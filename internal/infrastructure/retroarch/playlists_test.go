package retroarch_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
	"github.com/samuel-stidham/retroarch-presence/internal/infrastructure/retroarch"
)

const snesPlaylist = `{
  "version": "1.5",
  "items": [
    {
      "path": "/roms/snes/Banzai Mario World.sfc",
      "label": "Banzai Mario World [Hack by GbreezeSunset]",
      "core_path": "DETECT",
      "core_name": "DETECT",
      "crc32": "922D3660|crc",
      "db_name": "Nintendo - Super Nintendo Entertainment System.lpl"
    },
    {
      "path": "/roms/snes/Unscanned.sfc",
      "label": "Unscanned",
      "crc32": "DETECT"
    }
  ]
}`

const historyPlaylist = `{
  "version": "1.5",
  "items": [
    {"path": "/roms/nes/Dragon Warrior IV (USA).nes", "label": "", "crc32": "00000000|crc"}
  ]
}`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func flatpakConfigDir(home string) string {
	return filepath.Join(home, ".var/app/org.libretro.RetroArch/config/retroarch")
}

func TestPlaylistCatalog(t *testing.T) {
	t.Run("given the Flatpak config points playlist_directory at a ~ path, entries come from there", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(flatpakConfigDir(home), "retroarch.cfg"),
			"menu_driver = \"ozone\"\nplaylist_directory = \"~/games/playlists\"\n")
		writeFile(t, filepath.Join(home, "games/playlists/Nintendo - SNES.lpl"), snesPlaylist)
		writeFile(t, filepath.Join(home, "games/playlists/builtin/content_history.lpl"), historyPlaylist)

		catalog := retroarch.PlaylistCatalog{ConfigHome: filepath.Join(home, ".config"), Home: home}
		got, err := catalog.Entries()
		if err != nil {
			t.Fatalf("Entries: %v", err)
		}

		want := []domain.CatalogEntry{
			{Path: "/roms/snes/Banzai Mario World.sfc", Label: "Banzai Mario World [Hack by GbreezeSunset]", CRC32: 0x922d3660},
			{Path: "/roms/snes/Unscanned.sfc", Label: "Unscanned", CRC32: 0},
			{Path: "/roms/nes/Dragon Warrior IV (USA).nes", Label: "", CRC32: 0, History: true},
		}
		if len(got) != len(want) {
			t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("given an explicit config dir whose playlist_directory is default, its playlists folder is used", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "retroarch.cfg"), "playlist_directory = \"default\"\n")
		writeFile(t, filepath.Join(dir, "playlists/Nintendo - SNES.lpl"), snesPlaylist)

		catalog := retroarch.PlaylistCatalog{ConfigDir: dir, Home: t.TempDir()}
		got, err := catalog.Entries()
		if err != nil {
			t.Fatalf("Entries: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("got %d entries, want 2", len(got))
		}
	})

	t.Run("given both a native and a Flatpak install, the native config wins", func(t *testing.T) {
		home := t.TempDir()
		configHome := filepath.Join(home, ".config")
		writeFile(t, filepath.Join(configHome, "retroarch/retroarch.cfg"), "")
		writeFile(t, filepath.Join(configHome, "retroarch/playlists/native.lpl"), historyPlaylist)
		writeFile(t, filepath.Join(flatpakConfigDir(home), "retroarch.cfg"), "")
		writeFile(t, filepath.Join(flatpakConfigDir(home), "playlists/flatpak.lpl"), snesPlaylist)

		catalog := retroarch.PlaylistCatalog{ConfigHome: configHome, Home: home}
		got, err := catalog.Entries()
		if err != nil {
			t.Fatalf("Entries: %v", err)
		}
		if len(got) != 1 || got[0].Path != "/roms/nes/Dragon Warrior IV (USA).nes" {
			t.Errorf("got %+v, want only the native playlist", got)
		}
	})

	t.Run("given a malformed playlist beside a good one, the good entries come back with an error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "retroarch.cfg"), "")
		writeFile(t, filepath.Join(dir, "playlists/broken.lpl"), "{not json")
		writeFile(t, filepath.Join(dir, "playlists/good.lpl"), snesPlaylist)

		catalog := retroarch.PlaylistCatalog{ConfigDir: dir, Home: t.TempDir()}
		got, err := catalog.Entries()
		if err == nil {
			t.Error("Entries returned no error, want one naming broken.lpl")
		}
		if len(got) != 2 {
			t.Errorf("got %d entries, want the 2 from good.lpl", len(got))
		}
	})

	t.Run("given no RetroArch config anywhere, it returns an error", func(t *testing.T) {
		home := t.TempDir()
		catalog := retroarch.PlaylistCatalog{ConfigHome: filepath.Join(home, ".config"), Home: home}
		if _, err := catalog.Entries(); err == nil {
			t.Error("Entries returned no error, want one")
		}
	})
}
