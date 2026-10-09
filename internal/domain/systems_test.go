package domain_test

import (
	"testing"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

func TestSystemsLookup(t *testing.T) {
	defaults := domain.DefaultSystems()
	custom := defaults.WithOverrides(map[string]domain.System{
		"nintendo_64": {Label: "Nintendo 64", Image: "n64"},
		"super_nes":   {Image: "snes"},
		"game_boy":    {Image: "gb"},
		"vircon32":    {Label: "V32"},
	})

	cases := []struct {
		name      string
		systems   domain.Systems
		systemID  string
		ext       string
		wantLabel string
		wantImage string
		wantKnown bool
	}{
		{"given a Nintendo 64 core, the label is N64", defaults, "nintendo_64", "z64", "N64", "", true},
		{"given a SNES core, the label is SNES", defaults, "super_nes", "sfc", "SNES", "", true},
		{"given a NES core, the label is NES", defaults, "nes", "nes", "NES", "", true},
		{"given a PlayStation core, the label is PS1", defaults, "playstation", "chd", "PS1", "", true},
		{"given Gambatte running a .gbc file, the label is GBC", defaults, "game_boy", "gbc", "GBC", "", true},
		{"given Gambatte running a .gb file, the label is GB", defaults, "game_boy", "gb", "GB", "", true},
		{"given mGBA running a .gb file, the label is GB", defaults, "game_boy_advance", "gb", "GB", "", true},
		{"given mGBA running a .gba file, the label is GBA", defaults, "game_boy_advance", "gba", "GBA", "", true},
		{"given Genesis Plus GX running a Game Gear file, the label is GG", defaults, "mega_drive", "gg", "GG", "", true},
		{"given Genesis Plus GX running a Genesis file, the label is Genesis", defaults, "mega_drive", "md", "Genesis", "", true},
		{"given an upper case extension, the refinement still applies", defaults, "game_boy", "GBC", "GBC", "", true},
		{"given no playlist match, the base label applies", defaults, "game_boy", "", "GB", "", true},
		{"given an unknown system id, the label is the id in upper case", defaults, "fantasy_console", "", "FANTASY_CONSOLE", "", false},
		{"given a library name instead of an id, the label is that name in upper case", defaults, "Beetle PSX HW", "", "BEETLE PSX HW", "", false},
		{"given a config label for a system, it replaces the default", custom, "nintendo_64", "", "Nintendo 64", "n64", true},
		{"given a config image without a label, the default label stays", custom, "super_nes", "", "SNES", "snes", true},
		{"given an extension refinement, its label wins and the base image fills in", custom, "game_boy", "gbc", "GBC", "gb", true},
		{"given a config entry for a new system, it becomes known", custom, "vircon32", "", "V32", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, known := tc.systems.Lookup(tc.systemID, tc.ext)
			if got.Label != tc.wantLabel {
				t.Errorf("label = %q, want %q", got.Label, tc.wantLabel)
			}
			if got.Image != tc.wantImage {
				t.Errorf("image = %q, want %q", got.Image, tc.wantImage)
			}
			if known != tc.wantKnown {
				t.Errorf("known = %v, want %v", known, tc.wantKnown)
			}
		})
	}
}

func TestSystemsOverridesLeaveDefaultsAlone(t *testing.T) {
	defaults := domain.DefaultSystems()
	defaults.WithOverrides(map[string]domain.System{"nintendo_64": {Label: "Nintendo 64"}})

	t.Run("given an override was applied to a copy, the defaults are unchanged", func(t *testing.T) {
		got, _ := defaults.Lookup("nintendo_64", "")
		if got.Label != "N64" {
			t.Errorf("label = %q, want N64", got.Label)
		}
	})
}
