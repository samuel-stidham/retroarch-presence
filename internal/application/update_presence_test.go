package application_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/application"
	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

// The rate limit from the spec: at most one SET_ACTIVITY per five seconds.
const publishGap = 5 * time.Second

const ocarinaFile = "Legend of Zelda, The - Ocarina of Time (USA)"

var (
	ocarina = domain.Status{State: domain.Playing, SystemID: "nintendo_64", Basename: ocarinaFile, CRC32: 0xcd16c529}
	dw4     = domain.Status{State: domain.Playing, SystemID: "nes", Basename: "Dragon Warrior IV (USA)", CRC32: 0x1f2b2c3d}
	menu    = domain.Status{State: domain.Contentless}
)

func paused(s domain.Status) domain.Status {
	s.State = domain.Paused
	return s
}

// fakeRetroArch stands in for the UDP side.
type fakeRetroArch struct {
	status domain.Status
	err    error
}

func (f *fakeRetroArch) Status(context.Context) (domain.Status, error) { return f.status, f.err }

type fakeCatalog struct {
	entries []domain.CatalogEntry
	err     error
	reads   int
}

func (f *fakeCatalog) Entries() ([]domain.CatalogEntry, error) {
	f.reads++
	return f.entries, f.err
}

// fakeDiscord stands in for the IPC side. Discord keeps a presence only while
// the connection lives, so a publish connects and dropping the flag models a
// Discord restart.
type fakeDiscord struct {
	connected bool
	err       error
	published []*domain.Presence
}

func (f *fakeDiscord) Publish(_ context.Context, p *domain.Presence) error {
	if f.err != nil {
		return f.err
	}
	f.connected = true
	f.published = append(f.published, p)
	return nil
}

func (f *fakeDiscord) Connected() bool { return f.connected }

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

type harness struct {
	retroarch *fakeRetroArch
	catalog   *fakeCatalog
	discord   *fakeDiscord
	clock     *fakeClock
	start     time.Time
	use       *application.UpdatePresence
}

func newHarness() *harness {
	start := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	h := &harness{
		retroarch: &fakeRetroArch{},
		catalog:   &fakeCatalog{},
		discord:   &fakeDiscord{},
		clock:     &fakeClock{now: start},
		start:     start,
	}
	settings := application.Settings{
		Systems:     domain.DefaultSystems(),
		Pins:        map[string]string{ocarinaFile: "Ocarina of Time"},
		PausedImage: "pause",
	}
	h.use = application.NewUpdatePresence(h.retroarch, h.catalog, h.discord, h.clock, settings, slog.New(slog.DiscardHandler))
	return h
}

func (h *harness) tick(t *testing.T) {
	t.Helper()
	if err := h.use.Tick(t.Context()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
}

func (h *harness) sends() int { return len(h.discord.published) }

func (h *harness) last(t *testing.T) *domain.Presence {
	t.Helper()
	if h.sends() == 0 {
		t.Fatal("nothing was published")
	}
	return h.discord.published[h.sends()-1]
}

func TestUpdatePresence(t *testing.T) {
	t.Run("given RetroArch is playing, it shows the label and title with the session start", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.tick(t)

		got := h.last(t)
		if got == nil || got.Text != "[N64] Ocarina of Time" {
			t.Fatalf("published %+v, want [N64] Ocarina of Time", got)
		}
		if !got.Since.Equal(h.start) || got.Paused {
			t.Errorf("since, paused = %v, %v, want %v, false", got.Since, got.Paused, h.start)
		}
	})

	t.Run("given nothing changed, it never resends", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		for range 4 {
			h.tick(t)
			h.clock.advance(10 * time.Second)
		}
		if h.sends() != 1 {
			t.Errorf("sends = %d, want 1", h.sends())
		}
	})

	t.Run("given the game pauses, it shows paused and keeps the session start", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.tick(t)
		h.clock.advance(10 * time.Second)
		h.retroarch.status = paused(ocarina)
		h.tick(t)

		got := h.last(t)
		if !got.Paused || got.SmallImage != "pause" {
			t.Errorf("paused, small image = %v, %q, want true, pause", got.Paused, got.SmallImage)
		}
		if !got.Since.Equal(h.start) {
			t.Errorf("since = %v, want %v", got.Since, h.start)
		}
	})

	t.Run("given the title changes, the session restarts", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.tick(t)
		h.clock.advance(10 * time.Second)
		h.retroarch.status = dw4
		h.tick(t)

		got := h.last(t)
		if got.Text != "[NES] Dragon Warrior IV" {
			t.Errorf("text = %q, want [NES] Dragon Warrior IV", got.Text)
		}
		if !got.Since.Equal(h.start.Add(10 * time.Second)) {
			t.Errorf("since = %v, want ten seconds after start", got.Since)
		}
	})

	t.Run("given changes inside the rate limit window, it waits and then sends only the latest", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.tick(t)

		h.clock.advance(2 * time.Second)
		h.retroarch.status = paused(ocarina)
		h.tick(t)
		h.clock.advance(1 * time.Second)
		h.retroarch.status = dw4
		h.tick(t)
		if h.sends() != 1 {
			t.Fatalf("sends inside the window = %d, want 1", h.sends())
		}

		h.clock.advance(publishGap - 3*time.Second)
		h.tick(t)
		if h.sends() != 2 {
			t.Fatalf("sends after the window = %d, want 2", h.sends())
		}
		if got := h.last(t); got.Text != "[NES] Dragon Warrior IV" || got.Paused {
			t.Errorf("published %q paused=%v, want the latest state", got.Text, got.Paused)
		}
	})

	t.Run("given RetroArch returns to the menu, it clears the presence", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.tick(t)
		h.clock.advance(publishGap)
		h.retroarch.status = menu
		h.tick(t)

		if got := h.last(t); got != nil {
			t.Errorf("published %+v, want a clear", got)
		}
	})

	t.Run("given nothing was shown, the menu sends nothing", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = menu
		h.tick(t)
		if h.sends() != 0 {
			t.Errorf("sends = %d, want 0", h.sends())
		}
	})

	t.Run("given one missed reply, it keeps the presence", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.tick(t)
		h.clock.advance(publishGap)
		h.retroarch.err = errors.New("read udp: i/o timeout")
		h.tick(t)
		if h.sends() != 1 {
			t.Errorf("sends = %d, want 1", h.sends())
		}
	})

	t.Run("given two missed replies in a row, it clears the presence", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.tick(t)
		h.retroarch.err = errors.New("read udp: connection refused")
		h.clock.advance(publishGap)
		h.tick(t)
		h.clock.advance(publishGap)
		h.tick(t)
		if got := h.last(t); got != nil {
			t.Errorf("published %+v, want a clear", got)
		}
	})

	t.Run("given RetroArch comes back after closing, the session starts fresh", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.tick(t)
		h.retroarch.err = errors.New("read udp: connection refused")
		h.clock.advance(publishGap)
		h.tick(t)
		h.clock.advance(publishGap)
		h.tick(t)
		h.retroarch.err = nil
		h.clock.advance(publishGap)
		h.tick(t)

		got := h.last(t)
		if got == nil || !got.Since.Equal(h.clock.now) {
			t.Errorf("published %+v, want a session starting now", got)
		}
	})

	t.Run("given Discord restarted, it resends the current presence", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.tick(t)
		h.clock.advance(publishGap)
		h.discord.connected = false
		h.tick(t)

		if h.sends() != 2 || h.last(t).Text != "[N64] Ocarina of Time" {
			t.Errorf("sends = %d, want the presence sent again", h.sends())
		}
	})

	t.Run("given Discord is not running, it reports the error and retries on a later tick", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.discord.err = application.ErrPublisherOffline
		if err := h.use.Tick(t.Context()); !errors.Is(err, application.ErrPublisherOffline) {
			t.Fatalf("Tick error = %v, want ErrPublisherOffline", err)
		}

		h.discord.err = nil
		h.clock.advance(publishGap)
		h.tick(t)
		if h.sends() != 1 {
			t.Errorf("sends = %d, want 1", h.sends())
		}
	})

	t.Run("given the same content across ticks, the playlists are read once per load", func(t *testing.T) {
		h := newHarness()
		h.retroarch.status = ocarina
		h.tick(t)
		h.retroarch.status = paused(ocarina)
		h.tick(t)
		h.retroarch.status = ocarina
		h.tick(t)
		if h.catalog.reads != 1 {
			t.Fatalf("reads = %d, want 1", h.catalog.reads)
		}

		h.retroarch.status = dw4
		h.tick(t)
		if h.catalog.reads != 2 {
			t.Errorf("reads after new content = %d, want 2", h.catalog.reads)
		}
	})

	t.Run("given the playlists cannot be read, it falls back to the cleaned file name", func(t *testing.T) {
		h := newHarness()
		h.catalog.err = errors.New("open playlists: permission denied")
		h.retroarch.status = dw4
		h.tick(t)

		if got := h.last(t); got.Text != "[NES] Dragon Warrior IV" {
			t.Errorf("text = %q, want [NES] Dragon Warrior IV", got.Text)
		}
	})

	t.Run("given a playlist match, the label is refined by the file extension", func(t *testing.T) {
		h := newHarness()
		h.catalog.entries = []domain.CatalogEntry{
			{Path: "/roms/gbc/Zelda DX.gbc", Label: "Link's Awakening DX (USA)", CRC32: 0x2a7b2d4f},
		}
		h.retroarch.status = domain.Status{State: domain.Playing, SystemID: "game_boy", Basename: "Zelda DX", CRC32: 0x2a7b2d4f}
		h.tick(t)

		if got := h.last(t); got.Text != "[GBC] Link's Awakening DX" {
			t.Errorf("text = %q, want [GBC] Link's Awakening DX", got.Text)
		}
	})
}
