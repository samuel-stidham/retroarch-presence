package config

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file parses the TOML subset config.toml needs, so the daemon stays on
// the standard library. Supported: comments, bare and quoted keys, [table] and
// [a."b.c"] headers, and basic or literal single line strings. Anything else is
// an error with a line number. If the config format grows past this, swap in a
// full TOML library rather than extending the subset.

// table is one [header] section, or the root section before any header.
type table struct {
	path   []string
	line   int // line of the header, 0 for the root
	values []value
}

type value struct {
	key  string
	text string
	line int
}

func parseTOML(src string) ([]*table, error) {
	root := &table{}
	tables := []*table{root}
	current := root
	seenTables := map[string]bool{}
	seenKeys := map[*table]map[string]bool{root: {}}

	for i, raw := range strings.Split(src, "\n") {
		lineNo := i + 1
		s := &scanner{src: strings.TrimRight(raw, "\r"), line: lineNo}
		s.skipSpace()
		if s.atEnd() {
			continue
		}

		if s.peek() == '[' {
			path, err := s.header()
			if err != nil {
				return nil, err
			}
			id := strings.Join(quoteAll(path), ".")
			if seenTables[id] {
				return nil, s.errorf("table [%s] is defined twice", id)
			}
			seenTables[id] = true
			current = &table{path: path, line: lineNo}
			tables = append(tables, current)
			seenKeys[current] = map[string]bool{}
			continue
		}

		key, err := s.key()
		if err != nil {
			return nil, err
		}
		s.skipSpace()
		if !s.consume('=') {
			return nil, s.errorf("expected = after key %q", key)
		}
		s.skipSpace()
		text, err := s.str()
		if err != nil {
			return nil, err
		}
		s.skipSpace()
		if !s.atEnd() {
			return nil, s.errorf("unexpected text after the value of %q", key)
		}
		if seenKeys[current][key] {
			return nil, s.errorf("key %q is set twice", key)
		}
		seenKeys[current][key] = true
		current.values = append(current.values, value{key: key, text: text, line: lineNo})
	}
	return tables, nil
}

// quoteAll quotes each segment so [a."b.c"] and [a.b.c] never collide.
func quoteAll(path []string) []string {
	quoted := make([]string, len(path))
	for i, p := range path {
		quoted[i] = strconv.Quote(p)
	}
	return quoted
}

// scanner walks one line.
type scanner struct {
	src  string
	pos  int
	line int
}

func (s *scanner) errorf(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", s.line, fmt.Sprintf(format, args...))
}

func (s *scanner) peek() byte {
	if s.pos >= len(s.src) {
		return 0
	}
	return s.src[s.pos]
}

func (s *scanner) consume(c byte) bool {
	if s.peek() == c && s.pos < len(s.src) {
		s.pos++
		return true
	}
	return false
}

func (s *scanner) skipSpace() {
	for s.pos < len(s.src) && (s.src[s.pos] == ' ' || s.src[s.pos] == '\t') {
		s.pos++
	}
}

// atEnd reports whether only a comment or nothing is left.
func (s *scanner) atEnd() bool {
	return s.pos >= len(s.src) || s.src[s.pos] == '#'
}

func (s *scanner) header() ([]string, error) {
	s.consume('[')
	if s.peek() == '[' {
		return nil, s.errorf("arrays of tables are not supported")
	}
	var path []string
	for {
		s.skipSpace()
		segment, err := s.key()
		if err != nil {
			return nil, err
		}
		path = append(path, segment)
		s.skipSpace()
		if s.consume(']') {
			break
		}
		if !s.consume('.') {
			return nil, s.errorf("expected . or ] in table header")
		}
	}
	s.skipSpace()
	if !s.atEnd() {
		return nil, s.errorf("unexpected text after table header")
	}
	return path, nil
}

func isBareKeyChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

func (s *scanner) key() (string, error) {
	if c := s.peek(); c == '"' || c == '\'' {
		return s.str()
	}
	start := s.pos
	for s.pos < len(s.src) && isBareKeyChar(s.src[s.pos]) {
		s.pos++
	}
	if s.pos == start {
		return "", s.errorf("expected a key")
	}
	return s.src[start:s.pos], nil
}

// str reads a basic "..." or literal '...' string.
func (s *scanner) str() (string, error) {
	switch s.peek() {
	case '\'':
		s.pos++
		end := strings.IndexByte(s.src[s.pos:], '\'')
		if end < 0 {
			return "", s.errorf("unterminated string")
		}
		text := s.src[s.pos : s.pos+end]
		s.pos += end + 1
		return text, nil
	case '"':
		s.pos++
		return s.basicString()
	default:
		return "", s.errorf("value must be a quoted string")
	}
}

func (s *scanner) basicString() (string, error) {
	if strings.HasPrefix(s.src[s.pos:], `""`) {
		return "", s.errorf("multi-line strings are not supported")
	}
	var b strings.Builder
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		s.pos++
		switch c {
		case '"':
			return b.String(), nil
		case '\\':
			if err := s.escape(&b); err != nil {
				return "", err
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", s.errorf("unterminated string")
}

var simpleEscapes = map[byte]byte{'b': '\b', 't': '\t', 'n': '\n', 'f': '\f', 'r': '\r', '"': '"', '\\': '\\'}

func (s *scanner) escape(b *strings.Builder) error {
	if s.pos >= len(s.src) {
		return s.errorf("unterminated string")
	}
	c := s.src[s.pos]
	s.pos++
	if r, ok := simpleEscapes[c]; ok {
		b.WriteByte(r)
		return nil
	}

	digits := map[byte]int{'u': 4, 'U': 8}[c]
	if digits == 0 {
		return s.errorf("unknown escape \\%c", c)
	}
	if s.pos+digits > len(s.src) {
		return s.errorf("short \\%c escape", c)
	}
	code, err := strconv.ParseUint(s.src[s.pos:s.pos+digits], 16, 32)
	if err != nil || !utf8.ValidRune(rune(code)) {
		return s.errorf("bad \\%c escape", c)
	}
	s.pos += digits
	b.WriteRune(rune(code))
	return nil
}
