// Package domain holds the rules for turning what RetroArch is running into
// what Discord shows. It does no I/O.
package domain

// PlayState is what RetroArch is doing with its content.
type PlayState int

const (
	Contentless PlayState = iota
	Playing
	Paused
)

// Status is one answer from RetroArch about what it has loaded.
type Status struct {
	State    PlayState
	SystemID string
	Basename string // content file name without directory or extension
	CRC32    uint32 // 0 when RetroArch did not compute one
}

// SameContent reports whether two statuses describe the same loaded content,
// ignoring whether it is paused.
func (s Status) SameContent(other Status) bool {
	return s.SystemID == other.SystemID && s.Basename == other.Basename && s.CRC32 == other.CRC32
}
