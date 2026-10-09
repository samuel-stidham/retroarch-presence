package config_test

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
	"github.com/samuel-stidham/retroarch-presence/internal/infrastructure/config"
	"github.com/samuel-stidham/retroarch-presence/internal/infrastructure/discord"
)

const minimal = `application_id = "123456789012345678"`

func TestParse(t *testing.T) {
	t.Run("given only an application id, every other key takes its default", func(t *testing.T) {
		got, err := config.Parse(minimal)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if got.ApplicationID != "123456789012345678" {
			t.Errorf("application id = %q", got.ApplicationID)
		}
		if got.PollInterval != 5*time.Second {
			t.Errorf("poll interval = %v, want 5s", got.PollInterval)
		}
		if got.RetroArchAddress != "127.0.0.1:55355" {
			t.Errorf("address = %q", got.RetroArchAddress)
		}
		if got.Display != discord.DisplayDetails {
			t.Errorf("display = %v, want details", got.Display)
		}
		if got.LogLevel != slog.LevelError {
			t.Errorf("log level = %v, want error", got.LogLevel)
		}
	})

	t.Run("given every key, each one is read", func(t *testing.T) {
		src := `
# A comment line.
application_id = "123456789012345678"   # a trailing comment
poll_interval = "2s"
retroarch_address = '127.0.0.1:55400'
retroarch_config_dir = "~/.config/retroarch"
display = "name"
log_level = "debug"
paused_image = "pause"

[systems.nintendo_64]
label = "Nintendo 64"
image = "n64"

[ systems . "game_boy.gbc" ]
label = "GBC"

[titles]
"Legend of Zelda, The - Ocarina of Time (USA)" = "Ocarina of Time"
"Say \"hi\" \u00e9" = 'C:\literal'
`
		got, err := config.Parse(src)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if got.PollInterval != 2*time.Second || got.RetroArchAddress != "127.0.0.1:55400" {
			t.Errorf("poll, address = %v, %q", got.PollInterval, got.RetroArchAddress)
		}
		if got.RetroArchConfigDir != "~/.config/retroarch" || got.PausedImage != "pause" {
			t.Errorf("config dir, paused image = %q, %q", got.RetroArchConfigDir, got.PausedImage)
		}
		if got.Display != discord.DisplayName || got.LogLevel != slog.LevelDebug {
			t.Errorf("display, level = %v, %v", got.Display, got.LogLevel)
		}
		if got.Systems["nintendo_64"] != (domain.System{Label: "Nintendo 64", Image: "n64"}) {
			t.Errorf("nintendo_64 = %+v", got.Systems["nintendo_64"])
		}
		if got.Systems["game_boy.gbc"] != (domain.System{Label: "GBC"}) {
			t.Errorf("game_boy.gbc = %+v", got.Systems["game_boy.gbc"])
		}
		if got.Titles["Legend of Zelda, The - Ocarina of Time (USA)"] != "Ocarina of Time" {
			t.Errorf("titles = %v", got.Titles)
		}
		if got.Titles[`Say "hi" é`] != `C:\literal` {
			t.Errorf("escaped title = %v", got.Titles)
		}
	})
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantText string
	}{
		{"given no application id", `poll_interval = "5s"`, "application_id"},
		{"given an application id that is not a number", `application_id = "my-app"`, "application_id"},
		{"given a poll interval that is not a duration", minimal + "\npoll_interval = \"often\"", "line 2"},
		{"given a poll interval under one second", minimal + "\npoll_interval = \"100ms\"", "poll_interval"},
		{"given an unknown display mode", minimal + "\ndisplay = \"header\"", "display"},
		{"given an unknown log level", minimal + "\nlog_level = \"loud\"", "log_level"},
		{"given a misspelled key", minimal + "\npoll_intervall = \"5s\"", "line 2"},
		{"given a value that is not a string", minimal + "\npoll_interval = 5", "line 2"},
		{"given a duplicate key", minimal + "\n" + minimal, "line 2"},
		{"given an unterminated string", `application_id = "1234`, "line 1"},
		{"given an unknown table", minimal + "\n[server]\nport = \"1\"", "line 2"},
		{"given an unknown key inside a system", minimal + "\n[systems.nes]\ncolour = \"red\"", "line 3"},
		{"given an array of tables", minimal + "\n[[systems]]", "line 2"},
		{"given a duplicate table", minimal + "\n[titles]\n[titles]", "line 3"},
	}

	for _, tc := range cases {
		t.Run(tc.name+", it returns an error naming the problem", func(t *testing.T) {
			_, err := config.Parse(tc.src)
			if err == nil {
				t.Fatal("Parse succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("err = %q, want it to mention %q", err, tc.wantText)
			}
		})
	}
}

func TestExampleConfig(t *testing.T) {
	t.Run("given the documented example with an id filled in, it parses", func(t *testing.T) {
		raw, err := os.ReadFile("../../../config.example.toml")
		if err != nil {
			t.Fatal(err)
		}
		src := strings.Replace(string(raw), `application_id = ""`, minimal, 1)
		if _, err := config.Parse(src); err != nil {
			t.Errorf("config.example.toml does not parse: %v", err)
		}
	})
}

func TestDefaultPath(t *testing.T) {
	t.Run("given XDG_CONFIG_HOME, the config lives under it", func(t *testing.T) {
		got := config.DefaultPath(func(k string) string {
			return map[string]string{"XDG_CONFIG_HOME": "/xdg", "HOME": "/home/sam"}[k]
		})
		if got != "/xdg/retroarch-presence/config.toml" {
			t.Errorf("path = %q", got)
		}
	})

	t.Run("given no XDG_CONFIG_HOME, the config lives under ~/.config", func(t *testing.T) {
		got := config.DefaultPath(func(k string) string {
			return map[string]string{"HOME": "/home/sam"}[k]
		})
		if got != "/home/sam/.config/retroarch-presence/config.toml" {
			t.Errorf("path = %q", got)
		}
	})
}
