package discord

import (
	"fmt"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

// DisplayMode chooses which activity field carries the presence text.
type DisplayMode int

const (
	// DisplayDetails puts the text in details and has the status line show
	// details. It renders on every Discord build.
	DisplayDetails DisplayMode = iota
	// DisplayName puts the text in name, which replaces the application name
	// in the card header where Discord honours it. See docs/spec.md.
	DisplayName
)

func ParseDisplayMode(s string) (DisplayMode, error) {
	switch s {
	case "", "details":
		return DisplayDetails, nil
	case "name":
		return DisplayName, nil
	default:
		return 0, fmt.Errorf("display must be \"details\" or \"name\", not %q", s)
	}
}

const (
	activityTypePlaying = 0
	// status_display_type picks what the member list status line shows.
	statusDisplayName    = 0
	statusDisplayDetails = 2
	// Discord rejects name, details, and state longer than this.
	maxFieldRunes = 128
	pausedText    = "Paused"
)

type activity struct {
	Name              string      `json:"name,omitempty"`
	Type              int         `json:"type"`
	Details           string      `json:"details,omitempty"`
	State             string      `json:"state,omitempty"`
	StatusDisplayType int         `json:"status_display_type"`
	Timestamps        *timestamps `json:"timestamps,omitempty"`
	Assets            *assets     `json:"assets,omitempty"`
}

type timestamps struct {
	Start int64 `json:"start"` // Unix seconds
}

type assets struct {
	LargeImage string `json:"large_image,omitempty"`
	LargeText  string `json:"large_text,omitempty"`
	SmallImage string `json:"small_image,omitempty"`
	SmallText  string `json:"small_text,omitempty"`
}

// activityFor maps a presence onto Discord's activity object. A nil presence
// maps to no activity, which clears it.
func activityFor(p *domain.Presence, mode DisplayMode) *activity {
	if p == nil {
		return nil
	}

	a := &activity{Type: activityTypePlaying}
	text := truncate(p.Text, maxFieldRunes)
	if mode == DisplayName {
		a.Name = text
		a.StatusDisplayType = statusDisplayName
	} else {
		a.Details = text
		a.StatusDisplayType = statusDisplayDetails
	}
	if p.Paused {
		a.State = pausedText
	}
	if !p.Since.IsZero() {
		a.Timestamps = &timestamps{Start: p.Since.Unix()}
	}
	if p.LargeImage != "" || p.SmallImage != "" {
		a.Assets = &assets{LargeImage: p.LargeImage, LargeText: p.LargeText, SmallImage: p.SmallImage}
		if p.SmallImage != "" {
			a.Assets.SmallText = pausedText
		}
	}
	return a
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
