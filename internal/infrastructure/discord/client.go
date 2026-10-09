package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

const (
	ipcVersion       = 1
	handshakeTimeout = 5 * time.Second
	replyTimeout     = 5 * time.Second
)

// Client publishes presences over one IPC connection, opened lazily and
// reopened after Discord restarts. Discord clears the activity when the
// connection closes, so closing the client clears the presence.
type Client struct {
	appID string
	mode  DisplayMode
	dial  func() (net.Conn, error)
	log   *slog.Logger

	mu    sync.Mutex // serialises Publish and guards conn and nonce
	conn  *connection
	nonce uint64
}

func NewClient(appID string, mode DisplayMode, dial func() (net.Conn, error), log *slog.Logger) *Client {
	return &Client{appID: appID, mode: mode, dial: dial, log: log}
}

// Publish sends SET_ACTIVITY and waits for Discord's answer. A nil presence
// clears the activity. With no connection open there is nothing to clear,
// so a nil presence then sends nothing.
func (c *Client) Publish(ctx context.Context, p *domain.Presence) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil && !c.conn.alive() {
		c.dropLocked()
	}
	if c.conn == nil {
		if p == nil {
			return nil
		}
		conn, err := c.connect()
		if err != nil {
			return err
		}
		c.conn = conn
	}

	c.nonce++
	nonce := strconv.FormatUint(c.nonce, 10)
	req, err := json.Marshal(request{
		Cmd:   "SET_ACTIVITY",
		Nonce: nonce,
		Args:  setActivityArgs{PID: os.Getpid(), Activity: activityFor(p, c.mode)},
	})
	if err != nil {
		return err
	}
	if err := c.conn.write(opFrame, req); err != nil {
		c.dropLocked()
		return fmt.Errorf("send SET_ACTIVITY: %w", err)
	}

	r, err := c.conn.await(ctx, nonce)
	if err != nil {
		c.dropLocked()
		return fmt.Errorf("SET_ACTIVITY reply: %w", err)
	}
	if r.Evt == "ERROR" {
		return fmt.Errorf("discord rejected SET_ACTIVITY: %s", r.errorMessage())
	}
	return nil
}

// Connected reports whether the connection is still open. The reader notices
// Discord closing it within moments.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && c.conn.alive()
}

// Close drops the connection, which makes Discord clear the activity.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropLocked()
	return nil
}

func (c *Client) dropLocked() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// connect dials and performs the handshake. Discord answers a good handshake
// with a READY dispatch and a bad one, such as an unknown application id,
// with a CLOSE frame carrying the reason.
func (c *Client) connect() (*connection, error) {
	nc, err := c.dial()
	if err != nil {
		return nil, err
	}
	if err := c.handshake(nc); err != nil {
		nc.Close()
		return nil, err
	}

	conn := &connection{Conn: nc, replies: make(chan reply, 4), done: make(chan struct{})}
	go conn.readLoop(c.log)
	c.log.Debug("connected to Discord", "socket", nc.RemoteAddr().String())
	return conn, nil
}

func (c *Client) handshake(nc net.Conn) error {
	if err := nc.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return err
	}
	hello, err := json.Marshal(map[string]any{"v": ipcVersion, "client_id": c.appID})
	if err != nil {
		return err
	}
	if err := writeFrame(nc, opHandshake, hello); err != nil {
		return fmt.Errorf("discord handshake: %w", err)
	}

	op, payload, err := readFrame(nc)
	if err != nil {
		return fmt.Errorf("discord handshake: %w", err)
	}
	var r reply
	if err := json.Unmarshal(payload, &r); err != nil {
		return fmt.Errorf("discord handshake reply: %w", err)
	}
	if op == opClose {
		return fmt.Errorf("discord refused application id %s: %s", c.appID, r.closeMessage())
	}
	if op != opFrame || r.Evt != "READY" {
		return fmt.Errorf("discord handshake: unexpected reply %s", payload)
	}
	return nc.SetDeadline(time.Time{})
}

type request struct {
	Cmd   string          `json:"cmd"`
	Nonce string          `json:"nonce"`
	Args  setActivityArgs `json:"args"`
}

type setActivityArgs struct {
	PID      int       `json:"pid"`
	Activity *activity `json:"activity,omitempty"`
}

type reply struct {
	Cmd   string          `json:"cmd"`
	Evt   string          `json:"evt"`
	Nonce string          `json:"nonce"`
	Data  json.RawMessage `json:"data"`
	// CLOSE frames carry these at the top level.
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (r reply) errorMessage() string {
	var data struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(r.Data, &data) != nil || data.Message == "" {
		return string(r.Data)
	}
	return fmt.Sprintf("%s (code %d)", data.Message, data.Code)
}

func (r reply) closeMessage() string {
	return fmt.Sprintf("%s (code %d)", r.Message, r.Code)
}

// connection is one live IPC socket with a reader that answers pings and
// hands replies to the waiting publish.
type connection struct {
	net.Conn
	writeMu sync.Mutex
	replies chan reply
	done    chan struct{} // closed when the reader stops
}

func (c *connection) write(op uint32, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return writeFrame(c.Conn, op, payload)
}

func (c *connection) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *connection) readLoop(log *slog.Logger) {
	defer close(c.done)
	for {
		op, payload, err := readFrame(c.Conn)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Debug("Discord connection ended", "err", err)
			}
			return
		}
		switch op {
		case opFrame:
			var r reply
			if err := json.Unmarshal(payload, &r); err != nil {
				log.Debug("unreadable frame from Discord", "err", err)
				continue
			}
			// A reply nobody waits for, such as one that arrived after a
			// timeout, is dropped rather than blocking the reader.
			select {
			case c.replies <- r:
			default:
			}
		case opPing:
			if err := c.write(opPong, payload); err != nil {
				return
			}
		case opClose:
			log.Debug("Discord closed the connection", "payload", string(payload))
			return
		}
	}
}

// await returns the reply carrying nonce, skipping stale ones.
func (c *connection) await(ctx context.Context, nonce string) (reply, error) {
	timeout := time.NewTimer(replyTimeout)
	defer timeout.Stop()
	for {
		select {
		case r := <-c.replies:
			if r.Nonce == nonce {
				return r, nil
			}
		case <-c.done:
			// The reply may have landed just before Discord hung up.
			for {
				select {
				case r := <-c.replies:
					if r.Nonce == nonce {
						return r, nil
					}
				default:
					return reply{}, errors.New("connection closed")
				}
			}
		case <-timeout.C:
			return reply{}, errors.New("timed out")
		case <-ctx.Done():
			return reply{}, ctx.Err()
		}
	}
}
