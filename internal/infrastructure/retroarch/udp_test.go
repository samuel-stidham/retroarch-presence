package retroarch_test

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
	"github.com/samuel-stidham/retroarch-presence/internal/infrastructure/retroarch"
)

// fakeRetroArch listens like RetroArch's network command interface. It
// answers each datagram with reply, or stays silent when reply is empty.
func fakeRetroArch(t *testing.T, reply string) (addr string, requests <-chan string) {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	got := make(chan string, 8)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			got <- string(buf[:n])
			if reply != "" {
				conn.WriteTo([]byte(reply), from)
			}
		}
	}()
	return conn.LocalAddr().String(), got
}

// closedPort returns a loopback UDP address with nothing listening on it.
func closedPort(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()
	return addr
}

func TestUDPSource(t *testing.T) {
	const timeout = 300 * time.Millisecond

	t.Run("given RetroArch answers, it sends GET_STATUS and parses the reply", func(t *testing.T) {
		addr, requests := fakeRetroArch(t, "GET_STATUS PLAYING nes,Tetris,crc32=ff\n")
		source := retroarch.UDPSource{Address: addr, Timeout: timeout}

		got, err := source.Status(t.Context())
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		want := domain.Status{State: domain.Playing, SystemID: "nes", Basename: "Tetris", CRC32: 0xff}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
		if req := <-requests; strings.TrimSpace(req) != "GET_STATUS" {
			t.Errorf("request = %q, want GET_STATUS", req)
		}
	})

	t.Run("given RetroArch never answers, it gives up after the timeout", func(t *testing.T) {
		addr, _ := fakeRetroArch(t, "")
		source := retroarch.UDPSource{Address: addr, Timeout: timeout}

		began := time.Now()
		if _, err := source.Status(t.Context()); err == nil {
			t.Fatal("Status succeeded, want a timeout error")
		}
		if waited := time.Since(began); waited > 3*timeout {
			t.Errorf("waited %v, want about %v", waited, timeout)
		}
	})

	t.Run("given RetroArch is not running, it returns an error without hanging", func(t *testing.T) {
		source := retroarch.UDPSource{Address: closedPort(t), Timeout: timeout}

		began := time.Now()
		if _, err := source.Status(t.Context()); err == nil {
			t.Fatal("Status succeeded, want an error")
		}
		if waited := time.Since(began); waited > 3*timeout {
			t.Errorf("waited %v, want at most about %v", waited, timeout)
		}
	})
}
