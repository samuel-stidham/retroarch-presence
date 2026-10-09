// Package config loads config.toml.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
	"github.com/samuel-stidham/retroarch-presence/internal/infrastructure/discord"
)

const (
	defaultPollInterval     = 5 * time.Second
	minPollInterval         = time.Second
	defaultRetroArchAddress = "127.0.0.1:55355"
	// Quiet by default: a healthy daemon logs nothing.
	defaultLogLevel = slog.LevelError
)

type Config struct {
	ApplicationID      string
	PollInterval       time.Duration
	RetroArchAddress   string
	RetroArchConfigDir string // empty means auto-detect
	Display            discord.DisplayMode
	LogLevel           slog.Level
	PausedImage        string
	Systems            map[string]domain.System // keyed like the default table
	Titles             map[string]string        // basename to exact title
}

// DefaultPath is $XDG_CONFIG_HOME/retroarch-presence/config.toml, with
// XDG_CONFIG_HOME falling back to ~/.config.
func DefaultPath(getenv func(string) string) string {
	base := getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(getenv("HOME"), ".config")
	}
	return filepath.Join(base, "retroarch-presence", "config.toml")
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("no config at %s. Copy config.example.toml there and set application_id", path)
	}
	if err != nil {
		return Config{}, err
	}
	cfg, err := Parse(string(raw))
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func Parse(src string) (Config, error) {
	tables, err := parseTOML(src)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		PollInterval:     defaultPollInterval,
		RetroArchAddress: defaultRetroArchAddress,
		Display:          discord.DisplayDetails,
		LogLevel:         defaultLogLevel,
		Systems:          map[string]domain.System{},
		Titles:           map[string]string{},
	}
	for _, t := range tables {
		if err := cfg.apply(t); err != nil {
			return Config{}, err
		}
	}

	if cfg.ApplicationID == "" {
		return Config{}, errors.New("application_id is required. Create an application at https://discord.com/developers/applications and copy its id")
	}
	return cfg, nil
}

func (cfg *Config) apply(t *table) error {
	switch {
	case len(t.path) == 0:
		return cfg.applyRoot(t.values)
	case len(t.path) == 1 && t.path[0] == "titles":
		for _, v := range t.values {
			cfg.Titles[v.key] = v.text
		}
		return nil
	case len(t.path) == 2 && t.path[0] == "systems":
		return cfg.applySystem(t.path[1], t.values)
	default:
		return fmt.Errorf("line %d: unknown table [%s]. Expected [systems.<id>] or [titles]", t.line, strings.Join(t.path, "."))
	}
}

func (cfg *Config) applyRoot(values []value) error {
	for _, v := range values {
		var err error
		switch v.key {
		case "application_id":
			if !isDigits(v.text) {
				err = errors.New("application_id must be the numeric id from the Discord developer portal")
			}
			cfg.ApplicationID = v.text
		case "poll_interval":
			cfg.PollInterval, err = time.ParseDuration(v.text)
			if err == nil && cfg.PollInterval < minPollInterval {
				err = fmt.Errorf("poll_interval must be at least %v", minPollInterval)
			}
		case "retroarch_address":
			cfg.RetroArchAddress = v.text
		case "retroarch_config_dir":
			cfg.RetroArchConfigDir = v.text
		case "display":
			cfg.Display, err = discord.ParseDisplayMode(v.text)
		case "log_level":
			if uerr := cfg.LogLevel.UnmarshalText([]byte(v.text)); uerr != nil {
				err = fmt.Errorf("log_level must be debug, info, warn, or error, not %q", v.text)
			}
		case "paused_image":
			cfg.PausedImage = v.text
		default:
			err = fmt.Errorf("unknown key %q", v.key)
		}
		if err != nil {
			return fmt.Errorf("line %d: %w", v.line, err)
		}
	}
	return nil
}

func (cfg *Config) applySystem(id string, values []value) error {
	sys := cfg.Systems[id]
	for _, v := range values {
		switch v.key {
		case "label":
			sys.Label = v.text
		case "image":
			sys.Image = v.text
		default:
			return fmt.Errorf("line %d: unknown key %q in [systems.%s]. Expected label or image", v.line, v.key, id)
		}
	}
	cfg.Systems[id] = sys
	return nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
