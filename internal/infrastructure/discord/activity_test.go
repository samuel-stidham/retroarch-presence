package discord

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

// activityJSON renders the activity the way it goes over the wire, so the
// tests pin field names as well as values.
func activityJSON(t *testing.T, p *domain.Presence, mode DisplayMode) map[string]any {
	t.Helper()
	raw, err := json.Marshal(activityFor(p, mode))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestActivityFor(t *testing.T) {
	since := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	playing := &domain.Presence{Text: "[N64] Ocarina of Time", Since: since}

	t.Run("given details mode, the text goes in details and the status line shows details", func(t *testing.T) {
		got := activityJSON(t, playing, DisplayDetails)
		if got["details"] != "[N64] Ocarina of Time" {
			t.Errorf("details = %v", got["details"])
		}
		if got["status_display_type"] != float64(2) {
			t.Errorf("status_display_type = %v, want 2", got["status_display_type"])
		}
		if _, ok := got["name"]; ok {
			t.Errorf("name = %v, want it absent", got["name"])
		}
	})

	t.Run("given name mode, the text goes in name and the status line shows the name", func(t *testing.T) {
		got := activityJSON(t, playing, DisplayName)
		if got["name"] != "[N64] Ocarina of Time" {
			t.Errorf("name = %v", got["name"])
		}
		if got["status_display_type"] != float64(0) {
			t.Errorf("status_display_type = %v, want 0", got["status_display_type"])
		}
		if _, ok := got["details"]; ok {
			t.Errorf("details = %v, want it absent", got["details"])
		}
	})

	t.Run("given any presence, the type is Playing and the start is in Unix seconds", func(t *testing.T) {
		got := activityJSON(t, playing, DisplayDetails)
		if got["type"] != float64(0) {
			t.Errorf("type = %v, want 0", got["type"])
		}
		timestamps, _ := got["timestamps"].(map[string]any)
		if timestamps["start"] != float64(since.Unix()) {
			t.Errorf("timestamps = %v, want start %d", got["timestamps"], since.Unix())
		}
	})

	t.Run("given a paused presence, the state reads Paused and the small image is set", func(t *testing.T) {
		p := *playing
		p.Paused = true
		p.SmallImage = "pause"
		got := activityJSON(t, &p, DisplayDetails)
		if got["state"] != "Paused" {
			t.Errorf("state = %v, want Paused", got["state"])
		}
		assets, _ := got["assets"].(map[string]any)
		if assets["small_image"] != "pause" || assets["small_text"] != "Paused" {
			t.Errorf("assets = %v, want small image pause with text Paused", got["assets"])
		}
	})

	t.Run("given a playing presence, there is no state line", func(t *testing.T) {
		got := activityJSON(t, playing, DisplayDetails)
		if _, ok := got["state"]; ok {
			t.Errorf("state = %v, want it absent", got["state"])
		}
	})

	t.Run("given a system image, it is the large image with the label as hover text", func(t *testing.T) {
		p := *playing
		p.LargeImage = "n64"
		p.LargeText = "N64"
		got := activityJSON(t, &p, DisplayDetails)
		assets, _ := got["assets"].(map[string]any)
		if assets["large_image"] != "n64" || assets["large_text"] != "N64" {
			t.Errorf("assets = %v", got["assets"])
		}
	})

	t.Run("given no images, the assets object is left out", func(t *testing.T) {
		got := activityJSON(t, playing, DisplayDetails)
		if _, ok := got["assets"]; ok {
			t.Errorf("assets = %v, want it absent", got["assets"])
		}
	})

	t.Run("given text longer than Discord allows, it is cut to 128 characters", func(t *testing.T) {
		p := *playing
		p.Text = "[N64] " + strings.Repeat("é", 200)
		got := activityJSON(t, &p, DisplayDetails)
		if n := len([]rune(got["details"].(string))); n != 128 {
			t.Errorf("details has %d characters, want 128", n)
		}
	})

	t.Run("given no presence, there is no activity, which clears it", func(t *testing.T) {
		if a := activityFor(nil, DisplayDetails); a != nil {
			t.Errorf("activity = %+v, want nil", a)
		}
	})
}

func TestParseDisplayMode(t *testing.T) {
	cases := []struct {
		in      string
		want    DisplayMode
		wantErr bool
	}{
		{"details", DisplayDetails, false},
		{"name", DisplayName, false},
		{"", DisplayDetails, false},
		{"header", 0, true},
	}
	for _, tc := range cases {
		t.Run("given "+tc.in, func(t *testing.T) {
			got, err := ParseDisplayMode(tc.in)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("ParseDisplayMode(%q) = %v, %v", tc.in, got, err)
			}
		})
	}
}
