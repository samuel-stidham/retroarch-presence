// Package retroarch talks to RetroArch: its network command interface over
// UDP, and the playlists and config it keeps on disk.
package retroarch

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/samuel-stidham/retroarch-presence/internal/domain"
)

const (
	replyPrefix = "GET_STATUS "
	crcMarker   = ",crc32="
)

// ParseStatus reads a GET_STATUS reply. RetroArch 1.22 builds it in
// command_get_status as
//
//	GET_STATUS PLAYING <system_id>,<basename>,crc32=<%lx>
//
// with PAUSED in place of PLAYING, or as GET_STATUS CONTENTLESS. The basename
// can contain commas, so the system id ends at the first comma and the CRC
// starts after the last ",crc32=".
func ParseStatus(reply string) (domain.Status, error) {
	reply = strings.TrimRight(reply, "\r\n")
	rest, ok := strings.CutPrefix(reply, replyPrefix)
	if !ok {
		return domain.Status{}, fmt.Errorf("not a GET_STATUS reply: %q", reply)
	}
	if rest == "CONTENTLESS" {
		return domain.Status{State: domain.Contentless}, nil
	}

	stateWord, content, ok := strings.Cut(rest, " ")
	if !ok {
		return domain.Status{}, fmt.Errorf("GET_STATUS reply has no content fields: %q", reply)
	}
	var state domain.PlayState
	switch stateWord {
	case "PLAYING":
		state = domain.Playing
	case "PAUSED":
		state = domain.Paused
	default:
		return domain.Status{}, fmt.Errorf("unknown GET_STATUS state %q", stateWord)
	}

	systemID, tail, ok := strings.Cut(content, ",")
	if !ok {
		return domain.Status{}, fmt.Errorf("GET_STATUS reply has no basename: %q", reply)
	}
	marker := strings.LastIndex(tail, crcMarker)
	if marker < 0 {
		return domain.Status{}, fmt.Errorf("GET_STATUS reply has no crc32: %q", reply)
	}
	crc, err := strconv.ParseUint(tail[marker+len(crcMarker):], 16, 32)
	if err != nil {
		return domain.Status{}, fmt.Errorf("GET_STATUS crc32: %w", err)
	}

	return domain.Status{
		State:    state,
		SystemID: systemID,
		Basename: tail[:marker],
		CRC32:    uint32(crc),
	}, nil
}
