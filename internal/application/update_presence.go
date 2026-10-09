package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

const (
	// minPublishGap keeps SET_ACTIVITY well under Discord's limit of five
	// updates per twenty seconds.
	minPublishGap = 5 * time.Second
	// missesBeforeGone tolerates one lost UDP reply before deciding that
	// RetroArch has closed.
	missesBeforeGone = 2
)

// Settings are the user's choices that shape every presence.
type Settings struct {
	Systems     domain.Systems
	Pins        map[string]string // basename to exact title
	PausedImage string
}

// UpdatePresence keeps Discord in step with RetroArch. Each Tick polls once.
type UpdatePresence struct {
	source    StatusSource
	catalog   GameCatalog
	publisher PresencePublisher
	clock     Clock
	settings  Settings
	log       *slog.Logger

	misses     int
	hasContent bool
	content    domain.Status // the loaded content that system and game describe
	system     domain.System
	game       domain.Game
	session    domain.Session
	shown      *domain.Presence // what Discord is showing, nil for nothing
	lastSend   time.Time
}

func NewUpdatePresence(source StatusSource, catalog GameCatalog, publisher PresencePublisher, clock Clock, settings Settings, log *slog.Logger) *UpdatePresence {
	return &UpdatePresence{
		source:    source,
		catalog:   catalog,
		publisher: publisher,
		clock:     clock,
		settings:  settings,
		log:       log,
	}
}

// Run ticks every interval until ctx ends. Publish errors are logged, each
// distinct one once, so a persistent fault does not flood the journal.
func (u *UpdatePresence) Run(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	lastErr := ""
	for {
		err := u.Tick(ctx)
		switch {
		case err == nil:
			lastErr = ""
		case err.Error() == lastErr:
		case errors.Is(err, ErrPublisherOffline):
			u.log.Debug("waiting for Discord", "err", err)
		default:
			u.log.Error("could not update Discord", "err", err)
		}
		if err != nil {
			lastErr = err.Error()
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Tick polls RetroArch once and publishes if Discord needs to change. It
// returns an error only when publishing fails.
func (u *UpdatePresence) Tick(ctx context.Context) error {
	status, err := u.source.Status(ctx)
	if err != nil {
		u.misses++
		u.log.Debug("no reply from RetroArch", "err", err, "misses", u.misses)
		if u.misses < missesBeforeGone {
			return nil
		}
		u.forgetContent()
		return u.show(ctx, nil)
	}
	u.misses = 0

	if status.State == domain.Contentless {
		u.forgetContent()
		return u.show(ctx, nil)
	}
	p := u.presenceFor(status)
	return u.show(ctx, &p)
}

func (u *UpdatePresence) presenceFor(status domain.Status) domain.Presence {
	if !u.hasContent || !u.content.SameContent(status) {
		u.resolve(status)
	}
	p := domain.BuildPresence(u.system, u.game, status.State, u.settings.PausedImage)
	u.session = u.session.Advance(p.Text, u.clock.Now())
	p.Since = u.session.Since
	return p
}

// resolve names newly loaded content. It rereads the playlists each time, so
// games scanned since the last load are found.
func (u *UpdatePresence) resolve(status domain.Status) {
	entries, err := u.catalog.Entries()
	if err != nil {
		u.log.Warn("some playlists could not be read", "err", err)
	}
	u.game = domain.ResolveGame(status, u.settings.Pins, entries)

	system, known := u.settings.Systems.Lookup(status.SystemID, u.game.Extension)
	if !known {
		u.log.Info("unknown system id, add a label for it under [systems] in config", "system_id", status.SystemID)
	}
	u.system = system
	u.content = status
	u.hasContent = true
	u.log.Info("content loaded", "system_id", status.SystemID, "basename", status.Basename, "title", u.game.Title)
}

func (u *UpdatePresence) forgetContent() {
	u.hasContent = false
	u.session = domain.Session{}
}

// show publishes desired unless Discord already shows it or the rate limit
// window is still open. A change held back by the window is not queued. The
// next tick recomputes the state, so the latest state is what goes out.
func (u *UpdatePresence) show(ctx context.Context, desired *domain.Presence) error {
	if !u.publisher.Connected() {
		u.shown = nil
	}
	if samePresence(u.shown, desired) {
		return nil
	}
	now := u.clock.Now()
	if !u.lastSend.IsZero() && now.Sub(u.lastSend) < minPublishGap {
		return nil
	}

	// Failed attempts count toward the window too, so a struggling Discord is
	// never hit more often than a healthy one.
	u.lastSend = now
	if err := u.publisher.Publish(ctx, desired); err != nil {
		return fmt.Errorf("publish presence: %w", err)
	}
	u.shown = desired
	if desired == nil {
		u.log.Info("presence cleared")
	} else {
		u.log.Info("presence updated", "text", desired.Text, "paused", desired.Paused)
	}
	return nil
}

func samePresence(a, b *domain.Presence) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
