// Package application holds the one use case, UpdatePresence, and the ports it
// drives. Adapters in infrastructure implement the ports.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

// StatusSource reports what RetroArch has loaded. Any error means RetroArch
// did not answer, which is normal while it is closed.
type StatusSource interface {
	Status(ctx context.Context) (domain.Status, error)
}

// GameCatalog lists the games RetroArch knows from its playlists. It may
// return usable entries together with an error about the files it skipped.
type GameCatalog interface {
	Entries() ([]domain.CatalogEntry, error)
}

// PresencePublisher shows a presence in Discord. A nil presence clears it.
type PresencePublisher interface {
	Publish(ctx context.Context, p *domain.Presence) error
	// Connected reports whether the last published presence can still be
	// showing. It turns false when Discord goes away, which also clears it.
	Connected() bool
}

// ErrPublisherOffline is wrapped by a PresencePublisher that cannot reach
// Discord at all. That is normal while Discord is closed.
var ErrPublisherOffline = errors.New("discord is not reachable")

// Clock tells the time.
type Clock interface {
	Now() time.Time
}
