package domain

import (
	"path"
	"strings"
)

// CatalogEntry is one game RetroArch knows about from a playlist.
type CatalogEntry struct {
	Path    string
	Label   string
	CRC32   uint32 // 0 when the playlist has none
	History bool   // from a builtin playlist such as history, which ranks lower
}

// Game is the resolved identity of the loaded content.
type Game struct {
	Title     string
	Extension string // lowercase, no dot, from the matched playlist path. Empty without a match.
}

// ResolveGame names the loaded content. A pin for the basename wins. Otherwise
// the matching playlist entry's label is used, and failing that the basename.
// Both are cleaned. The extension comes from the playlist match even when a
// pin supplies the title, because label refinement needs it.
func ResolveGame(status Status, pins map[string]string, catalog []CatalogEntry) Game {
	game := Game{Title: CleanTitle(status.Basename)}
	if entry, ok := bestMatch(status, catalog); ok {
		game.Title = CleanTitle(entry.Label)
		game.Extension = contentExtension(entry.Path)
	}
	if pin, ok := pins[status.Basename]; ok {
		game.Title = pin
	}
	return game
}

// bestMatch prefers entries whose file has the same basename. Among those, a
// CRC match beats a system playlist, which beats history. Without a basename
// match it falls back to the CRC alone. A zero CRC never matches, because
// RetroArch sends zero when it did not compute one.
func bestMatch(status Status, catalog []CatalogEntry) (CatalogEntry, bool) {
	crcMatches := func(e CatalogEntry) bool { return status.CRC32 != 0 && e.CRC32 == status.CRC32 }
	rank := func(e CatalogEntry) int {
		score := 0
		if crcMatches(e) {
			score += 2
		}
		if !e.History {
			score++
		}
		return score
	}

	pick := func(eligible func(CatalogEntry) bool) (CatalogEntry, bool) {
		best, bestRank, found := CatalogEntry{}, -1, false
		for _, e := range catalog {
			if e.Label == "" || !eligible(e) {
				continue
			}
			if r := rank(e); r > bestRank {
				best, bestRank, found = e, r, true
			}
		}
		return best, found
	}

	if e, ok := pick(func(e CatalogEntry) bool { return contentBasename(e.Path) == status.Basename }); ok {
		return e, true
	}
	return pick(crcMatches)
}

// contentBasename mirrors how RetroArch names content in GET_STATUS: the file
// name without its extension. For "a.zip#b.sfc" that is the file inside the
// archive, "b".
func contentBasename(p string) string {
	name := archiveMember(p)
	return strings.TrimSuffix(name, path.Ext(name))
}

func contentExtension(p string) string {
	return strings.ToLower(strings.TrimPrefix(path.Ext(archiveMember(p)), "."))
}

func archiveMember(p string) string {
	if _, member, ok := strings.Cut(p, "#"); ok {
		return path.Base(member)
	}
	return path.Base(p)
}

// articles that No-Intro names move to the end, as in "Legend of Zelda, The".
var articles = []string{"The", "A", "An"}

// CleanTitle removes every (...) and [...] group, which covers region tags such
// as (USA) and dump tags such as [!]. It then moves a trailing article back to
// the front. A title that would clean to nothing is returned as written.
func CleanTitle(title string) string {
	var b strings.Builder
	depth := 0
	for _, r := range title {
		switch {
		case r == '(' || r == '[':
			depth++
		case (r == ')' || r == ']') && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	cleaned := strings.Join(strings.Fields(b.String()), " ")
	if cleaned == "" {
		return strings.TrimSpace(title)
	}

	head, rest, hasSubtitle := strings.Cut(cleaned, " - ")
	for _, article := range articles {
		if trimmed, ok := strings.CutSuffix(head, ", "+article); ok {
			head = article + " " + trimmed
			break
		}
	}
	if hasSubtitle {
		return head + " - " + rest
	}
	return head
}
