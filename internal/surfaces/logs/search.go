package logs

import "regexp"

// matchLoc is one regex match location within the wrapped display buffer:
// which wrapped line it's on, and its byte offsets within that line's text.
// Matches are computed over wrapped lines, not logical entries, so
// highlighting and n/N navigation line up exactly with what's on screen
// (PD-LOGS-AF-003/AF-005) — the accepted trade-off is that a match spanning
// a wrap boundary is missed, the same limitation any line-oriented pager has.
type matchLoc struct {
	line       int
	start, end int
}

// compileSearch compiles pat as the active search pattern (Go regexp/RE2 —
// PD-LOGS-AF-004's documented caveat: no backreferences, unlike sed's BRE).
// An empty pattern clears the active search. An invalid regex leaves the
// previous pattern/matches untouched and returns the error for the footer to
// display.
func (m *Model) compileSearch(pat string) error {
	if pat == "" {
		m.pattern = nil
		m.matches = nil
		m.matchIdx = -1
		return nil
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return err
	}
	m.pattern = re
	m.recomputeMatches()
	return nil
}

// recomputeMatches rebuilds the match list against the current wrapped
// buffer for the active pattern. Called whenever the pattern changes or the
// buffer is fully rewrapped (resize).
func (m *Model) recomputeMatches() {
	m.matches = m.matches[:0]
	m.matchIdx = -1
	if m.pattern == nil {
		return
	}
	for i, line := range m.wrapped {
		for _, loc := range m.pattern.FindAllStringIndex(line, -1) {
			m.matches = append(m.matches, matchLoc{line: i, start: loc[0], end: loc[1]})
		}
	}
}

// appendMatches scans only the newly-wrapped lines (starting at start) for
// the active pattern, so a live-tailing buffer with an active search doesn't
// re-scan its entire history on every incoming event.
func (m *Model) appendMatches(start int, newLines []string) {
	if m.pattern == nil {
		return
	}
	for i, line := range newLines {
		for _, loc := range m.pattern.FindAllStringIndex(line, -1) {
			m.matches = append(m.matches, matchLoc{line: start + i, start: loc[0], end: loc[1]})
		}
	}
}

// gotoMatch moves the active match by delta (+1 for next/n, -1 for
// previous/N), wrapping around the match list, and scrolls it into view.
// With no active match yet, it picks the nearest match to the current
// viewport in the given direction. A no-op with zero matches.
func (m *Model) gotoMatch(delta int) {
	if len(m.matches) == 0 {
		return
	}
	if m.matchIdx < 0 {
		m.matchIdx = m.nearestMatchIndex(delta)
	} else {
		m.matchIdx = (m.matchIdx + delta + len(m.matches)) % len(m.matches)
	}
	m.follow = false
	m.scrollToLine(m.matches[m.matchIdx].line)
}

// nearestMatchIndex finds the first match at/after the viewport (delta >= 0)
// or the last match at/before it (delta < 0), wrapping to the opposite end
// when nothing qualifies.
func (m *Model) nearestMatchIndex(delta int) int {
	if delta >= 0 {
		for i, mt := range m.matches {
			if mt.line >= m.offset {
				return i
			}
		}
		return 0
	}
	for i := len(m.matches) - 1; i >= 0; i-- {
		if m.matches[i].line <= m.offset {
			return i
		}
	}
	return len(m.matches) - 1
}
