// Package logs is the log/trace surface (PD-LOGS): a read-only, searchable,
// live-tailing viewer over the session's complete event log. Unlike the
// context surface, it renders every persisted state.Event — including
// ephemeral ones — as one wrapped, timestamped line, so it doubles as a way
// to review backend activity that never reaches the conversation view.
// Vi-style /pattern and ?pattern search highlight every match with n/N to
// cycle them, and gg/G jump to the buffer's ends. Strictly read-only: there
// is no transport dependency, unlike the context surface's toggle/pin POSTs.
// Launched with `agentx surface launch logs`.
package logs

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"agentx/internal/state"
	"agentx/internal/surfaces/scrollutil"
)

var (
	matchStyle       = lipgloss.NewStyle().Reverse(true)
	activeMatchStyle = lipgloss.NewStyle().Reverse(true).Bold(true)
)

// entry is one applied event's formatted (pre-wrap) line.
type entry struct {
	line string
}

// Model is the read-only logs surface.
type Model struct {
	entries []entry
	wrapped []string // cached word-wrapped display lines for the current width

	width  int
	height int // rows available for content; one row is reserved for the footer

	offset int  // index into wrapped of the first visible line
	follow bool // true = pinned to the bottom, tracking new events like tail -f

	pendingG bool // true right after a lone "g", awaiting a second "g" for gg

	searching   bool // true while a /pattern or ?pattern prompt is being typed
	searchDir   rune // '/' forward, '?' backward — which way a fresh search jumps first
	searchInput string
	searchErr   string

	pattern  *regexp.Regexp
	matches  []matchLoc
	matchIdx int // index into matches of the active match, -1 if none
}

// New returns an empty logs surface. It has no transport dependency — the
// view is strictly read-only, so unlike the context surface there is
// nothing to POST back.
func New() *Model {
	return &Model{follow: true, matchIdx: -1}
}

// Apply appends one event as a formatted line. Unlike the context surface,
// ephemeral events are not skipped here — this is a full activity log, not a
// conversation view, so the startup bootstrap exchange and similar
// session-internal events are exactly the kind of backend activity a user
// reviewing logs wants to see.
func (m *Model) Apply(ev state.Event) {
	e := entry{line: formatEvent(ev)}
	m.entries = append(m.entries, e)
	start := len(m.wrapped)
	newLines := scrollutil.WrapLines(e.line, m.wrapWidth())
	m.wrapped = append(m.wrapped, newLines...)
	m.appendMatches(start, newLines)
	if m.follow {
		m.scrollToBottom()
	}
}

// SetSize sets the render area. One row is reserved for the footer status
// line. A width change invalidates the wrap cache, since every line's wrap
// points depend on it.
func (m *Model) SetSize(width, height int) {
	if width != m.width {
		m.width = width
		m.rewrapAll()
	}
	m.height = max(0, height-1)
	m.clampOffset()
}

// CapturesKeys reports whether the surface is mid-search-input — while true,
// the host framework (SS-8) forwards "q" as a literal character instead of
// treating it as quit, since a search pattern may contain one.
func (m *Model) CapturesKeys() bool { return m.searching }

// Key handles all logs-surface navigation and search. Quit ("q"/ctrl+c) is
// handled by the host, never here.
func (m *Model) Key(msg tea.KeyPressMsg) tea.Cmd {
	if m.searching {
		m.searchKey(msg)
		return nil
	}

	key := msg.String()
	if key == "g" {
		if m.pendingG {
			m.pendingG = false
			m.jumpTop()
		} else {
			m.pendingG = true
		}
		return nil
	}
	m.pendingG = false

	switch key {
	case "j", "down":
		m.scrollBy(1)
	case "k", "up":
		m.scrollBy(-1)
	case "ctrl+d":
		m.scrollBy(m.halfPage())
	case "ctrl+u":
		m.scrollBy(-m.halfPage())
	case "pgdown", "ctrl+f":
		m.scrollBy(m.pageSize())
	case "pgup", "ctrl+b":
		m.scrollBy(-m.pageSize())
	case "G":
		m.jumpBottom()
	case "/":
		m.startSearch('/')
	case "?":
		m.startSearch('?')
	case "n":
		m.gotoMatch(1)
	case "N":
		m.gotoMatch(-1)
	case "esc":
		m.clearSearch()
	}
	return nil
}

// searchKey handles a keypress while a /pattern or ?pattern prompt is open.
func (m *Model) searchKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "enter":
		m.commitSearch()
	case "esc":
		m.searching = false
		m.searchInput = ""
	case "backspace":
		if n := len(m.searchInput); n > 0 {
			m.searchInput = m.searchInput[:n-1]
		}
	default:
		if msg.Text != "" {
			m.searchInput += msg.Text
		}
	}
}

func (m *Model) startSearch(dir rune) {
	m.searching = true
	m.searchDir = dir
	m.searchInput = ""
	m.searchErr = ""
}

// commitSearch compiles the typed pattern and jumps to the first match in
// the search's direction. An invalid regex reports the error in the footer
// and leaves any previously active pattern untouched.
func (m *Model) commitSearch() {
	m.searching = false
	if err := m.compileSearch(m.searchInput); err != nil {
		m.searchErr = err.Error()
		return
	}
	m.searchErr = ""
	if m.searchDir == '?' {
		m.gotoMatch(-1)
	} else {
		m.gotoMatch(1)
	}
}

// clearSearch drops the active pattern and its highlights.
func (m *Model) clearSearch() {
	m.pattern = nil
	m.matches = nil
	m.matchIdx = -1
	m.searchErr = ""
}

func (m *Model) scrollBy(n int) {
	if n < 0 {
		m.follow = false
	}
	m.offset = scrollutil.ClampInt(m.offset+n, 0, m.maxOffset())
}

func (m *Model) scrollToLine(line int) {
	if line < m.offset || line >= m.offset+m.height {
		m.offset = scrollutil.ClampInt(line-m.height/2, 0, m.maxOffset())
	}
}

func (m *Model) scrollToBottom() { m.offset = m.maxOffset() }

func (m *Model) jumpTop() {
	m.follow = false
	m.offset = 0
}

func (m *Model) jumpBottom() {
	m.follow = true
	m.offset = m.maxOffset()
}

func (m *Model) clampOffset() {
	m.offset = scrollutil.ClampInt(m.offset, 0, m.maxOffset())
}

func (m *Model) maxOffset() int { return max(0, len(m.wrapped)-m.height) }

func (m *Model) halfPage() int { return max(1, m.height/2) }
func (m *Model) pageSize() int { return max(1, m.height) }

func (m *Model) wrapWidth() int {
	if m.width <= 0 {
		return 80
	}
	return m.width
}

// rewrapAll rebuilds the entire wrap cache for the current width — only
// needed on resize, since Apply wraps and appends incrementally otherwise.
func (m *Model) rewrapAll() {
	wrapped := make([]string, 0, len(m.entries))
	for _, e := range m.entries {
		wrapped = append(wrapped, scrollutil.WrapLines(e.line, m.wrapWidth())...)
	}
	m.wrapped = wrapped
	m.recomputeMatches()
	if m.follow {
		m.scrollToBottom()
	} else {
		m.clampOffset()
	}
}

// View renders the visible slice of wrapped lines with search matches
// highlighted, followed by a footer status/hint line.
func (m *Model) View() string {
	var b strings.Builder
	start := scrollutil.ClampInt(m.offset, 0, len(m.wrapped))
	end := scrollutil.ClampInt(start+m.height, 0, len(m.wrapped))
	for i := start; i < end; i++ {
		b.WriteString(m.renderLine(i))
		b.WriteByte('\n')
	}
	b.WriteString(m.footer())
	return b.String()
}

// renderLine renders wrapped[i], wrapping each match in it with a highlight
// style (the active match bolded so n/N progress is visible at a glance).
func (m *Model) renderLine(i int) string {
	line := m.wrapped[i]
	if m.pattern == nil {
		return line
	}
	var b strings.Builder
	last := 0
	for mi, mt := range m.matches {
		if mt.line != i {
			continue
		}
		b.WriteString(line[last:mt.start])
		style := matchStyle
		if mi == m.matchIdx {
			style = activeMatchStyle
		}
		b.WriteString(style.Render(line[mt.start:mt.end]))
		last = mt.end
	}
	b.WriteString(line[last:])
	return b.String()
}

func (m *Model) footer() string {
	if m.searching {
		return string(m.searchDir) + m.searchInput
	}
	if m.searchErr != "" {
		return "search error: " + m.searchErr
	}
	if m.pattern != nil {
		if len(m.matches) == 0 {
			return "no matches · / ? search · gg/G top/bottom · q quit"
		}
		return fmt.Sprintf("%d/%d matches · n/N next/prev · gg/G top/bottom · q quit", m.matchIdx+1, len(m.matches))
	}
	return "/ ? search · gg/G top/bottom · q quit"
}
