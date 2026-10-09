package discord

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

func TestFrameEncoding(t *testing.T) {
	t.Run("given a frame, the header is the opcode then the length, both little endian", func(t *testing.T) {
		var buf bytes.Buffer
		if err := writeFrame(&buf, opFrame, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
		want := []byte{1, 0, 0, 0, 2, 0, 0, 0, '{', '}'}
		if !bytes.Equal(buf.Bytes(), want) {
			t.Errorf("bytes = %v, want %v", buf.Bytes(), want)
		}

		op, payload, err := readFrame(&buf)
		if err != nil || op != opFrame || string(payload) != `{}` {
			t.Errorf("readFrame = %d, %q, %v", op, payload, err)
		}
	})
}

// fakeDiscordServer speaks the IPC protocol over a unix socket the way the
// Discord client does, closely enough to exercise the client's behaviour.
type fakeDiscordServer struct {
	path     string
	behavior serverBehavior

	handshakes chan map[string]any
	requests   chan map[string]any
	pongs      chan string
	conns      chan net.Conn
}

type serverBehavior struct {
	rejectApp bool // answer the handshake with a CLOSE frame
	rejectSet bool // answer SET_ACTIVITY with an ERROR event
}

func newFakeDiscordServer(t *testing.T, behavior serverBehavior) *fakeDiscordServer {
	t.Helper()
	s := &fakeDiscordServer{
		path:       filepath.Join(shortTempDir(t), "discord-ipc-0"),
		behavior:   behavior,
		handshakes: make(chan map[string]any, 8),
		requests:   make(chan map[string]any, 8),
		pongs:      make(chan string, 8),
		conns:      make(chan net.Conn, 8),
	}
	ln, err := net.Listen("unix", s.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			s.conns <- conn
			go s.serve(conn)
		}
	}()
	return s
}

func (s *fakeDiscordServer) serve(conn net.Conn) {
	defer conn.Close()
	_, payload, err := readFrame(conn)
	if err != nil {
		return
	}
	var hello map[string]any
	json.Unmarshal(payload, &hello)
	s.handshakes <- hello
	if s.behavior.rejectApp {
		writeFrame(conn, opClose, []byte(`{"code":4000,"message":"Invalid Client ID"}`))
		return
	}
	writeFrame(conn, opFrame, []byte(`{"cmd":"DISPATCH","evt":"READY","data":{"v":1}}`))

	for {
		op, payload, err := readFrame(conn)
		if err != nil {
			return
		}
		if op == opPong {
			s.pongs <- string(payload)
			continue
		}
		var req map[string]any
		json.Unmarshal(payload, &req)
		s.requests <- req
		reply := map[string]any{"cmd": req["cmd"], "nonce": req["nonce"], "evt": nil, "data": map[string]any{}}
		if s.behavior.rejectSet {
			reply["evt"] = "ERROR"
			reply["data"] = map[string]any{"code": 4000, "message": `child "activity" fails because [child "details" fails]`}
		}
		raw, _ := json.Marshal(reply)
		writeFrame(conn, opFrame, raw)
	}
}

func (s *fakeDiscordServer) dialer(dials *atomic.Int32) func() (net.Conn, error) {
	return func() (net.Conn, error) {
		if dials != nil {
			dials.Add(1)
		}
		return net.Dial("unix", s.path)
	}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the fake Discord server")
		panic("unreachable")
	}
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition never became true")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

const appID = "123456789012345678"

var ocarina = &domain.Presence{Text: "[N64] Ocarina of Time", Since: time.Unix(1791583114, 0)}

func newTestClient(server *fakeDiscordServer, dials *atomic.Int32) *Client {
	return NewClient(appID, DisplayDetails, server.dialer(dials), slog.New(slog.DiscardHandler))
}

func TestClient(t *testing.T) {
	t.Run("given Discord is listening, it handshakes with the app id and sends SET_ACTIVITY", func(t *testing.T) {
		server := newFakeDiscordServer(t, serverBehavior{})
		client := newTestClient(server, nil)
		defer client.Close()

		if err := client.Publish(t.Context(), ocarina); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		hello := receive(t, server.handshakes)
		if hello["client_id"] != appID || hello["v"] != float64(1) {
			t.Errorf("handshake = %v", hello)
		}
		req := receive(t, server.requests)
		if req["cmd"] != "SET_ACTIVITY" || req["nonce"] == "" {
			t.Errorf("request = %v", req)
		}
		args, _ := req["args"].(map[string]any)
		activity, _ := args["activity"].(map[string]any)
		if activity["details"] != "[N64] Ocarina of Time" || args["pid"] == nil {
			t.Errorf("args = %v", args)
		}
		if !client.Connected() {
			t.Error("Connected = false after a publish")
		}
	})

	t.Run("given a nil presence while connected, it sends SET_ACTIVITY without an activity", func(t *testing.T) {
		server := newFakeDiscordServer(t, serverBehavior{})
		client := newTestClient(server, nil)
		defer client.Close()

		if err := client.Publish(t.Context(), ocarina); err != nil {
			t.Fatal(err)
		}
		receive(t, server.requests)
		if err := client.Publish(t.Context(), nil); err != nil {
			t.Fatalf("Publish(nil): %v", err)
		}
		args, _ := receive(t, server.requests)["args"].(map[string]any)
		if _, ok := args["activity"]; ok {
			t.Errorf("args = %v, want no activity", args)
		}
	})

	t.Run("given a nil presence before any connection, it does not connect", func(t *testing.T) {
		server := newFakeDiscordServer(t, serverBehavior{})
		var dials atomic.Int32
		client := newTestClient(server, &dials)
		defer client.Close()

		if err := client.Publish(t.Context(), nil); err != nil {
			t.Fatalf("Publish(nil): %v", err)
		}
		if dials.Load() != 0 {
			t.Errorf("dials = %d, want 0", dials.Load())
		}
	})

	t.Run("given Discord rejects the app id, Publish returns Discord's message", func(t *testing.T) {
		server := newFakeDiscordServer(t, serverBehavior{rejectApp: true})
		client := newTestClient(server, nil)
		defer client.Close()

		err := client.Publish(t.Context(), ocarina)
		if err == nil || !strings.Contains(err.Error(), "Invalid Client ID") {
			t.Errorf("err = %v, want it to carry Invalid Client ID", err)
		}
	})

	t.Run("given Discord answers with an ERROR event, Publish returns its message", func(t *testing.T) {
		server := newFakeDiscordServer(t, serverBehavior{rejectSet: true})
		client := newTestClient(server, nil)
		defer client.Close()

		err := client.Publish(t.Context(), ocarina)
		if err == nil || !strings.Contains(err.Error(), `child "details" fails`) {
			t.Errorf("err = %v, want Discord's message", err)
		}
	})

	t.Run("given Discord pings, the client answers with a pong carrying the same payload", func(t *testing.T) {
		server := newFakeDiscordServer(t, serverBehavior{})
		client := newTestClient(server, nil)
		defer client.Close()

		if err := client.Publish(t.Context(), ocarina); err != nil {
			t.Fatal(err)
		}
		conn := receive(t, server.conns)
		writeFrame(conn, opPing, []byte(`{"nonce":"ping-1"}`))
		if got := receive(t, server.pongs); got != `{"nonce":"ping-1"}` {
			t.Errorf("pong = %q", got)
		}
	})

	t.Run("given Discord drops the connection, Connected turns false and the next publish reconnects", func(t *testing.T) {
		server := newFakeDiscordServer(t, serverBehavior{})
		var dials atomic.Int32
		client := newTestClient(server, &dials)
		defer client.Close()

		if err := client.Publish(t.Context(), ocarina); err != nil {
			t.Fatal(err)
		}
		receive(t, server.conns).Close()
		eventually(t, func() bool { return !client.Connected() })

		if err := client.Publish(t.Context(), ocarina); err != nil {
			t.Fatalf("Publish after reconnect: %v", err)
		}
		if dials.Load() != 2 {
			t.Errorf("dials = %d, want 2", dials.Load())
		}
	})

	t.Run("given the dialer fails, Publish returns its error", func(t *testing.T) {
		offline := errors.New("no socket")
		client := NewClient(appID, DisplayDetails, func() (net.Conn, error) { return nil, offline }, slog.New(slog.DiscardHandler))
		if err := client.Publish(t.Context(), ocarina); !errors.Is(err, offline) {
			t.Errorf("err = %v, want the dialer's error", err)
		}
	})
}
