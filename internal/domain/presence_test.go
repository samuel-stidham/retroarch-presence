package domain_test

import (
	"testing"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

func TestBuildPresence(t *testing.T) {
	n64 := domain.System{Label: "N64"}
	n64WithImage := domain.System{Label: "N64", Image: "n64"}
	ocarina := domain.Game{Title: "Ocarina of Time"}

	t.Run("given a game is playing, the text is the bracketed label then the title", func(t *testing.T) {
		got := domain.BuildPresence(n64, ocarina, domain.Playing, "")
		if got.Text != "[N64] Ocarina of Time" {
			t.Errorf("text = %q, want %q", got.Text, "[N64] Ocarina of Time")
		}
		if got.Paused {
			t.Error("paused = true, want false")
		}
	})

	t.Run("given a game is paused, the presence says so and shows the paused image", func(t *testing.T) {
		got := domain.BuildPresence(n64, ocarina, domain.Paused, "pause")
		if !got.Paused {
			t.Error("paused = false, want true")
		}
		if got.SmallImage != "pause" {
			t.Errorf("small image = %q, want pause", got.SmallImage)
		}
	})

	t.Run("given a game is playing, the paused image is not shown", func(t *testing.T) {
		got := domain.BuildPresence(n64, ocarina, domain.Playing, "pause")
		if got.SmallImage != "" {
			t.Errorf("small image = %q, want empty", got.SmallImage)
		}
	})

	t.Run("given the system has an image, it is the large image and the label is its hover text", func(t *testing.T) {
		got := domain.BuildPresence(n64WithImage, ocarina, domain.Playing, "")
		if got.LargeImage != "n64" || got.LargeText != "N64" {
			t.Errorf("large image, text = %q, %q, want n64, N64", got.LargeImage, got.LargeText)
		}
	})

	t.Run("given the system has no image, there is no large image or hover text", func(t *testing.T) {
		got := domain.BuildPresence(n64, ocarina, domain.Playing, "")
		if got.LargeImage != "" || got.LargeText != "" {
			t.Errorf("large image, text = %q, %q, want both empty", got.LargeImage, got.LargeText)
		}
	})
}

func TestSessionAdvance(t *testing.T) {
	start := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	later := start.Add(10 * time.Minute)

	t.Run("given no session yet, it starts now", func(t *testing.T) {
		got := domain.Session{}.Advance("[N64] Ocarina of Time", start)
		if !got.Since.Equal(start) {
			t.Errorf("since = %v, want %v", got.Since, start)
		}
	})

	t.Run("given the text is unchanged, the session keeps its start", func(t *testing.T) {
		s := domain.Session{}.Advance("[N64] Ocarina of Time", start)
		got := s.Advance("[N64] Ocarina of Time", later)
		if !got.Since.Equal(start) {
			t.Errorf("since = %v, want %v", got.Since, start)
		}
	})

	t.Run("given the text changes, the session restarts", func(t *testing.T) {
		s := domain.Session{}.Advance("[N64] Ocarina of Time", start)
		got := s.Advance("[NES] Dragon Warrior IV", later)
		if !got.Since.Equal(later) {
			t.Errorf("since = %v, want %v", got.Since, later)
		}
	})
}

func TestPresenceEqual(t *testing.T) {
	since := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	a := domain.Presence{Text: "[N64] Ocarina of Time", Since: since}

	t.Run("given the same instant in another time zone, presences are equal", func(t *testing.T) {
		b := a
		b.Since = since.In(time.FixedZone("EDT", -4*3600))
		if !a.Equal(b) {
			t.Error("Equal = false, want true")
		}
	})

	t.Run("given a different paused flag, presences differ", func(t *testing.T) {
		b := a
		b.Paused = true
		if a.Equal(b) {
			t.Error("Equal = true, want false")
		}
	})
}
