package domain

import "time"

// Presence is what Discord should show while a game is loaded.
type Presence struct {
	Text       string // "[N64] Ocarina of Time"
	Paused     bool
	Since      time.Time // session start, zero until a session is attached
	LargeImage string
	LargeText  string
	SmallImage string
}

// Equal compares presences, treating the same instant in different time zones
// as equal.
func (p Presence) Equal(q Presence) bool {
	return p.Text == q.Text &&
		p.Paused == q.Paused &&
		p.Since.Equal(q.Since) &&
		p.LargeImage == q.LargeImage &&
		p.LargeText == q.LargeText &&
		p.SmallImage == q.SmallImage
}

// BuildPresence assembles the presence for a game. The session start is left
// zero because it depends on the text built here.
func BuildPresence(system System, game Game, state PlayState, pausedImage string) Presence {
	p := Presence{
		Text:   "[" + system.Label + "] " + game.Title,
		Paused: state == Paused,
	}
	if system.Image != "" {
		p.LargeImage = system.Image
		p.LargeText = system.Label
	}
	if p.Paused {
		p.SmallImage = pausedImage
	}
	return p
}

// Session tracks how long the current text has been showing.
type Session struct {
	Text  string
	Since time.Time
}

// Advance keeps the session while the text is unchanged and starts a new one
// when it changes. Pausing does not change the text, so it keeps the session.
func (s Session) Advance(text string, now time.Time) Session {
	if text == s.Text && !s.Since.IsZero() {
		return s
	}
	return Session{Text: text, Since: now}
}
