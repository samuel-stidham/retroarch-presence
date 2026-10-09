package retroarch

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

// maxReply is the size of RetroArch's reply buffer in command_get_status.
const maxReply = 4096

// UDPSource asks RetroArch for its status over the network command interface.
type UDPSource struct {
	Address string
	Timeout time.Duration
}

// Status sends one GET_STATUS. Each call uses a fresh socket, so a reply that
// arrives after a timeout can never be read as the answer to the next poll.
// When nothing listens, the kernel reports the port unreachable and the read
// fails at once instead of waiting out the timeout.
func (s UDPSource) Status(ctx context.Context) (domain.Status, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "udp", s.Address)
	if err != nil {
		return domain.Status{}, fmt.Errorf("dial RetroArch: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(s.Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return domain.Status{}, err
	}

	if _, err := conn.Write([]byte("GET_STATUS\n")); err != nil {
		return domain.Status{}, fmt.Errorf("send GET_STATUS: %w", err)
	}
	buf := make([]byte, maxReply)
	n, err := conn.Read(buf)
	if err != nil {
		return domain.Status{}, fmt.Errorf("read GET_STATUS reply: %w", err)
	}
	return ParseStatus(string(buf[:n]))
}
