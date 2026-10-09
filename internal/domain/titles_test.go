package domain_test

import (
	"testing"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

func TestCleanTitle(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"given a region tag, it is removed", "Dragon Warrior IV (USA)", "Dragon Warrior IV"},
		{"given GoodTools tags, they are removed", "Super Mario World (U) [!]", "Super Mario World"},
		{"given several tags, all are removed", "Final Fantasy VII (USA) (Disc 1)", "Final Fantasy VII"},
		{"given a hack credit in brackets, it is removed", "Banzai Mario World [Hack by GbreezeSunset]", "Banzai Mario World"},
		{"given a trailing article before a subtitle, it moves to the front", "Legend of Zelda, The - Ocarina of Time (USA)", "The Legend of Zelda - Ocarina of Time"},
		{"given a trailing article at the end, it moves to the front", "Legend of Zelda, The (USA)", "The Legend of Zelda"},
		{"given the article A, it moves to the front", "Boy and His Blob, A - Trouble on Blobolonia (USA)", "A Boy and His Blob - Trouble on Blobolonia"},
		{"given a hyphenated title, the hyphen stays", "Pokemon - Red Version (USA, Europe) (SGB Enhanced)", "Pokemon - Red Version"},
		{"given doubled spaces left by a tag, they collapse", "Contra  (USA)", "Contra"},
		{"given a plain title, it is unchanged", "Tetris", "Tetris"},
		{"given a title that is only tags, it is kept as written", "(Homebrew)", "(Homebrew)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.CleanTitle(tc.in); got != tc.want {
				t.Errorf("CleanTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveGame(t *testing.T) {
	const (
		ocarina = "Legend of Zelda, The - Ocarina of Time (USA)"
		dw4     = "Dragon Warrior IV (USA)"
		links   = "Legend of Zelda, The - Link's Awakening DX (USA)"
	)
	catalog := []domain.CatalogEntry{
		// The history copy comes first to prove that rank beats file order.
		{Path: "/roms/nes/" + dw4 + ".nes", Label: "History Label", CRC32: 0x1f2b2c3d, History: true},
		{Path: "/roms/n64/" + ocarina + ".z64", Label: ocarina, CRC32: 0xcd16c529},
		{Path: "/roms/nes/" + dw4 + ".nes", Label: dw4, CRC32: 0x1f2b2c3d},
		{Path: "/roms/gbc/Zelda DX.zip#" + links + ".gbc", Label: "Link's Awakening DX", CRC32: 0x2a7b2d4f},
		{Path: "/roms/snes/smw-renamed.sfc", Label: "Super Mario World (USA)", CRC32: 0xb19ed489},
		{Path: "/roms/hacks/Kaizo.sfc", Label: "Kaizo Mario World 1", CRC32: 0x11111111},
		{Path: "/roms/hacks2/Kaizo.sfc", Label: "Kaizo Mario World 2", CRC32: 0x22222222},
		{Path: "/roms/gba/Metroid Fusion (USA).gba", Label: "", CRC32: 0x6c75479c},
		{Path: "/roms/unknown/Mystery.bin", Label: "Never Matched By Zero", CRC32: 0},
	}

	cases := []struct {
		name    string
		status  domain.Status
		pins    map[string]string
		catalog []domain.CatalogEntry
		want    string
		wantExt string
	}{
		{
			name:    "given a pin for the basename, the pin is used exactly as written",
			status:  domain.Status{Basename: ocarina, CRC32: 0xcd16c529},
			pins:    map[string]string{ocarina: "Ocarina of Time"},
			catalog: catalog,
			want:    "Ocarina of Time",
			wantExt: "z64",
		},
		{
			name:    "given a playlist entry with the same basename, its cleaned label is used",
			status:  domain.Status{Basename: ocarina, CRC32: 0xcd16c529},
			catalog: catalog,
			want:    "The Legend of Zelda - Ocarina of Time",
			wantExt: "z64",
		},
		{
			name:    "given two entries share the basename, the CRC match wins",
			status:  domain.Status{Basename: "Kaizo", CRC32: 0x22222222},
			catalog: catalog,
			want:    "Kaizo Mario World 2",
			wantExt: "sfc",
		},
		{
			name:    "given two entries share the basename and the CRC is zero, the first one wins",
			status:  domain.Status{Basename: "Kaizo", CRC32: 0},
			catalog: catalog,
			want:    "Kaizo Mario World 1",
			wantExt: "sfc",
		},
		{
			name:    "given no basename match, a CRC match is used",
			status:  domain.Status{Basename: "smw", CRC32: 0xb19ed489},
			catalog: catalog,
			want:    "Super Mario World",
			wantExt: "sfc",
		},
		{
			name:    "given a CRC of zero, CRC matching is skipped",
			status:  domain.Status{Basename: "Unknown Game (Europe)", CRC32: 0},
			catalog: catalog,
			want:    "Unknown Game",
			wantExt: "",
		},
		{
			name:    "given a history entry and a system playlist entry, the system playlist wins",
			status:  domain.Status{Basename: dw4, CRC32: 0x1f2b2c3d},
			catalog: catalog,
			want:    "Dragon Warrior IV",
			wantExt: "nes",
		},
		{
			name:    "given an archive path, the file inside the archive is matched",
			status:  domain.Status{Basename: links, CRC32: 0x2a7b2d4f},
			catalog: catalog,
			want:    "Link's Awakening DX",
			wantExt: "gbc",
		},
		{
			name:    "given an entry with an empty label, it is skipped",
			status:  domain.Status{Basename: "Metroid Fusion (USA)", CRC32: 0x6c75479c},
			catalog: catalog,
			want:    "Metroid Fusion",
			wantExt: "",
		},
		{
			name:    "given no playlists at all, the basename is cleaned",
			status:  domain.Status{Basename: dw4, CRC32: 0x1f2b2c3d},
			catalog: nil,
			want:    "Dragon Warrior IV",
			wantExt: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.ResolveGame(tc.status, tc.pins, tc.catalog)
			if got.Title != tc.want {
				t.Errorf("title = %q, want %q", got.Title, tc.want)
			}
			if got.Extension != tc.wantExt {
				t.Errorf("extension = %q, want %q", got.Extension, tc.wantExt)
			}
		})
	}
}
