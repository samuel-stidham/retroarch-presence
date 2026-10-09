package retroarch

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

// flatpakConfig is where the Flathub build of RetroArch keeps its config,
// relative to the home directory.
const flatpakConfig = ".var/app/org.libretro.RetroArch/config/retroarch"

// PlaylistCatalog reads the games RetroArch knows from its *.lpl playlists.
type PlaylistCatalog struct {
	ConfigDir  string // RetroArch's config dir. Empty means auto-detect.
	ConfigHome string // $XDG_CONFIG_HOME, where a native install keeps its config
	Home       string
}

// Entries returns system playlist entries first, then the builtin ones such as
// history. A playlist that cannot be parsed is skipped, and its error is
// returned beside the entries from the rest.
func (c PlaylistCatalog) Entries() ([]domain.CatalogEntry, error) {
	dir, err := c.playlistDir()
	if err != nil {
		return nil, err
	}

	var entries []domain.CatalogEntry
	var errs []error
	for _, group := range []struct {
		pattern string
		history bool
	}{
		{filepath.Join(dir, "*.lpl"), false},
		{filepath.Join(dir, "builtin", "*.lpl"), true},
	} {
		files, _ := filepath.Glob(group.pattern) // the pattern is always well formed
		for _, file := range files {
			found, err := readPlaylist(file, group.history)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			entries = append(entries, found...)
		}
	}
	return entries, errors.Join(errs...)
}

// playlistDir finds RetroArch's config dir, native first, then Flatpak, and
// reads playlist_directory from its retroarch.cfg.
func (c PlaylistCatalog) playlistDir() (string, error) {
	candidates := []string{filepath.Join(c.ConfigHome, "retroarch"), filepath.Join(c.Home, flatpakConfig)}
	if c.ConfigDir != "" {
		candidates = []string{c.expandHome(c.ConfigDir)}
	}

	for _, dir := range candidates {
		cfg := filepath.Join(dir, "retroarch.cfg")
		if _, err := os.Stat(cfg); err != nil {
			continue
		}
		value, err := readConfigValue(cfg, "playlist_directory")
		if err != nil {
			return "", err
		}
		// RetroArch writes "default" for an unset path, and a leading ':'
		// means relative to its executable, which only portable builds use.
		if value == "" || value == "default" || strings.HasPrefix(value, ":") {
			return filepath.Join(dir, "playlists"), nil
		}
		return c.expandHome(value), nil
	}
	return "", fmt.Errorf("no retroarch.cfg found in %s", strings.Join(candidates, " or "))
}

func (c PlaylistCatalog) expandHome(p string) string {
	if p == "~" {
		return c.Home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(c.Home, rest)
	}
	return p
}

// readConfigValue reads one key from a retroarch.cfg, whose lines look like
// key = "value".
func readConfigValue(path, key string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		k, v, ok := strings.Cut(scanner.Text(), "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"`), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return "", nil
}

type playlistFile struct {
	Items []struct {
		Path  string `json:"path"`
		Label string `json:"label"`
		CRC32 string `json:"crc32"`
	} `json:"items"`
}

func readPlaylist(path string, history bool) ([]domain.CatalogEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var playlist playlistFile
	if err := json.Unmarshal(raw, &playlist); err != nil {
		return nil, fmt.Errorf("parse playlist %s: %w", path, err)
	}

	entries := make([]domain.CatalogEntry, 0, len(playlist.Items))
	for _, item := range playlist.Items {
		entries = append(entries, domain.CatalogEntry{
			Path:    item.Path,
			Label:   item.Label,
			CRC32:   playlistCRC(item.CRC32),
			History: history,
		})
	}
	return entries, nil
}

// playlistCRC reads RetroArch's "922D3660|crc" form. "DETECT" and anything
// else unreadable mean unknown, which is zero.
func playlistCRC(field string) uint32 {
	hex, _, _ := strings.Cut(field, "|")
	crc, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0
	}
	return uint32(crc)
}
