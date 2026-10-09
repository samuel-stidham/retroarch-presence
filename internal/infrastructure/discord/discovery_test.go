package discord

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/samuel-stidham/retroarch-presence/internal/application"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestSocketCandidates(t *testing.T) {
	t.Run("given XDG_RUNTIME_DIR, it lists native, then Flatpak, then Snap paths for ids 0 to 9", func(t *testing.T) {
		got := socketCandidates(env(map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000", "TMPDIR": "/ignored"}))
		if len(got) != 30 {
			t.Fatalf("got %d candidates, want 30", len(got))
		}
		want := map[int]string{
			0:  "/run/user/1000/discord-ipc-0",
			9:  "/run/user/1000/discord-ipc-9",
			10: "/run/user/1000/app/com.discordapp.Discord/discord-ipc-0",
			20: "/run/user/1000/snap.discord/discord-ipc-0",
		}
		for i, path := range want {
			if got[i] != path {
				t.Errorf("candidate %d = %q, want %q", i, got[i], path)
			}
		}
	})

	t.Run("given no runtime dir, it falls back to TMPDIR", func(t *testing.T) {
		got := socketCandidates(env(map[string]string{"TMPDIR": "/var/tmp"}))
		if got[0] != "/var/tmp/discord-ipc-0" {
			t.Errorf("first candidate = %q", got[0])
		}
	})

	t.Run("given no temp variables at all, it uses /tmp", func(t *testing.T) {
		got := socketCandidates(env(nil))
		if got[0] != "/tmp/discord-ipc-0" {
			t.Errorf("first candidate = %q", got[0])
		}
	})
}

func TestSocketDialer(t *testing.T) {
	t.Run("given a directory squatting discord-ipc-0, it skips it and dials discord-ipc-1", func(t *testing.T) {
		dir := shortTempDir(t)
		if err := os.Mkdir(filepath.Join(dir, "discord-ipc-0"), 0o755); err != nil {
			t.Fatal(err)
		}
		ln, err := net.Listen("unix", filepath.Join(dir, "discord-ipc-1"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })

		conn, err := SocketDialer(env(map[string]string{"XDG_RUNTIME_DIR": dir}))()
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		if got := conn.RemoteAddr().String(); got != filepath.Join(dir, "discord-ipc-1") {
			t.Errorf("connected to %q, want discord-ipc-1", got)
		}
	})

	t.Run("given no socket anywhere, it reports the publisher as offline", func(t *testing.T) {
		dir := shortTempDir(t)
		_, err := SocketDialer(env(map[string]string{"XDG_RUNTIME_DIR": dir}))()
		if !errors.Is(err, application.ErrPublisherOffline) {
			t.Errorf("err = %v, want ErrPublisherOffline", err)
		}
	})
}

// shortTempDir keeps socket paths under the 108 byte limit for unix sockets,
// which t.TempDir can exceed with long test names.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "rp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
