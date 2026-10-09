package discord

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/application"
)

const (
	// Discord binds the first free discord-ipc-N for N from 0 to 9.
	maxSocketIndex = 9
	dialTimeout    = 2 * time.Second
)

// socketSubdirs are where each packaging of Discord puts its socket, relative
// to the temp base. Native first, then Flatpak, then Snap.
var socketSubdirs = []string{"", "app/com.discordapp.Discord", "snap.discord"}

// socketCandidates lists every path Discord might listen on, most likely
// first. The base dir follows Discord's own lookup order.
func socketCandidates(getenv func(string) string) []string {
	base := "/tmp"
	for _, key := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if v := getenv(key); v != "" {
			base = v
			break
		}
	}

	var paths []string
	for _, sub := range socketSubdirs {
		for i := 0; i <= maxSocketIndex; i++ {
			paths = append(paths, filepath.Join(base, sub, "discord-ipc-"+strconv.Itoa(i)))
		}
	}
	return paths
}

// SocketDialer returns a dialer that connects to the first live Discord
// socket. It skips paths that exist but are not sockets, such as the empty
// directory a Flatpak app's discord-ipc-0 grant can leave behind.
func SocketDialer(getenv func(string) string) func() (net.Conn, error) {
	return func() (net.Conn, error) {
		for _, path := range socketCandidates(getenv) {
			info, err := os.Stat(path)
			if err != nil || info.Mode()&os.ModeSocket == 0 {
				continue
			}
			conn, err := net.DialTimeout("unix", path, dialTimeout)
			if err != nil {
				continue
			}
			return conn, nil
		}
		return nil, fmt.Errorf("%w: no live discord-ipc socket", application.ErrPublisherOffline)
	}
}
