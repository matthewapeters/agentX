// Package input is the chat surface's input panel: a multi-line prompt editor.
// It supports typing, Enter-to-submit (disabled while streaming), a soft-newline
// (Shift+Enter on terminals that disambiguate it, plus the terminal-agnostic
// Alt+Enter / Ctrl+J aliases), backspace, a stop/interrupt action while streaming,
// readline-style history seeding (↑/↓), a cursor with readline movement keys, and
// bracketed-paste insertion (routed in by the host from tea.PasteMsg, since that
// arrives as its own message type rather than a stream of KeyPressMsg).
//
// Long lines word-wrap to the panel width (no horizontal overflow) and the panel
// grows vertically with its content up to a configured cap (input_max_lines), beyond
// which it windows the content around the cursor with a right-gutter scrollbar. The
// host reads DesiredHeight to size the panel each layout.
//
// Source contract: docs/ux/03_PANEL_DETAILS.md PD-02 (re-authored for the TUI).
// Backlog task: CHT-B3.
package input

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Action is the outcome of handling a key, signaled to the host surface.
type Action int

const (
	// ActionNone means the key was handled internally (or ignored).
	ActionNone Action = iota
	// ActionSubmit means the user submitted a non-empty prompt.
	ActionSubmit
	// ActionStop means the user requested interruption of a streaming response.
	ActionStop
	// ActionHistoryBoundary means a history-navigation key hit an edge (older
	// than the oldest prompt, or newer than the in-progress draft); the buffer
	// did not change and the host should signal the boundary (e.g. a flash).
	ActionHistoryBoundary
)

// Model is the input panel state.
//
// It also keeps a readline-style history of prompts submitted during this run:
// ↑/↓ seed the editable buffer with a prior prompt (PD-02-AF-013…016). histIdx
// addresses history; histIdx == len(history) is the "present" line, whose
// in-progress text is stashed in draft while navigating older entries.
type Model struct {
	width     int
	height    int
	maxHeight int // cap on vertical growth before the panel scrolls (input_max_lines)
	value     string
	cursor    int // rune index into value; edits and movement are relative to it
	focused   bool
	streaming bool
	history   []string
	histIdx   int
	draft     string
	// newlineKey labels the soft-newline binding shown in the empty-buffer hint.
	// It defaults to the terminal-agnostic "alt+enter"; the host upgrades it to
	// "shift+enter" once the terminal reports key-disambiguation support.
	newlineKey string
}

// New returns a focused input panel that starts one row tall and grows with content.
func New() *Model { return &Model{focused: true, height: 1, maxHeight: 8, newlineKey: "alt+enter"} }

// SetNewlineKey sets the soft-newline key label shown in the placeholder hint. The
// host calls it with "shift+enter" when the terminal disambiguates modified keys,
// otherwise the terminal-agnostic default ("alt+enter") stands.
func (m *Model) SetNewlineKey(key string) {
	if key != "" {
		m.newlineKey = key
	}
}

// SetSize sets the panel's render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 0)
	if height > 0 {
		m.height = height
	}
}

// SetMaxHeight sets the cap on vertical growth (input_max_lines). Beyond it the
// panel scrolls with a gutter scrollbar.
func (m *Model) SetMaxHeight(n int) {
	if n > 0 {
		m.maxHeight = n
	}
}

// Height returns the panel's row count.
func (m *Model) Height() int { return m.height }

// DesiredHeight is the number of rows the panel wants at its current width: the
// wrapped visual-row count, clamped to [1, maxHeight].
func (m *Model) DesiredHeight() int {
	n := len(m.visualRows())
	if n < 1 {
		n = 1
	}
	if m.maxHeight > 0 && n > m.maxHeight {
		n = m.maxHeight
	}
	return n
}

// Focused reports whether the panel has input focus.
func (m *Model) Focused() bool { return m.focused }

// Focus gives the panel input focus.
func (m *Model) Focus() { m.focused = true }

// Blur removes input focus.
func (m *Model) Blur() { m.focused = false }

// Value returns the current input text.
func (m *Model) Value() string { return m.value }

// Cursor returns the cursor position as a rune index into the value.
func (m *Model) Cursor() int { return m.cursor }

// Reset clears the input text and returns history navigation to the present.
func (m *Model) Reset() {
	m.value = ""
	m.cursor = 0
	m.draft = ""
	m.histIdx = len(m.history)
}

// Seeded reports whether the buffer is currently showing a history entry (a
// prompt was seeded via ↑/↓ and not yet cleared or submitted).
func (m *Model) Seeded() bool { return m.histIdx < len(m.history) }

// ClearSeed abandons a history seed: the buffer returns to empty and navigation
// resets to the present. Bound to the idle Esc,Esc chord by the host surface.
func (m *Model) ClearSeed() {
	m.value = ""
	m.cursor = 0
	m.draft = ""
	m.histIdx = len(m.history)
}

// SetStreaming toggles streaming mode (submit disabled, stop enabled).
func (m *Model) SetStreaming(streaming bool) { m.streaming = streaming }

// Streaming reports whether a response is currently streaming.
func (m *Model) Streaming() bool { return m.streaming }

// Update handles a key press and returns the resulting Action.
func (m *Model) Update(msg tea.KeyPressMsg) Action {
	switch msg.String() {
	case "enter":
		if m.streaming || strings.TrimSpace(m.value) == "" {
			return ActionNone
		}
		m.history = append(m.history, m.value)
		m.histIdx = len(m.history)
		m.draft = ""
		return ActionSubmit
	case "shift+enter", "alt+enter", "ctrl+j":
		// Soft-newline. Shift+Enter only reaches us on terminals that disambiguate
		// modified keys (Kitty protocol / modifyOtherKeys); Alt+Enter and Ctrl+J are
		// the terminal-agnostic aliases that work everywhere.
		if !m.streaming {
			m.insert("\n")
		}
		return ActionNone
	case "up":
		if m.streaming {
			return ActionNone
		}
		return m.historyPrev()
	case "down":
		if m.streaming {
			return ActionNone
		}
		return m.historyNext()
	case "left":
		if !m.streaming && m.cursor > 0 {
			m.cursor--
		}
		return ActionNone
	case "right":
		if !m.streaming && m.cursor < m.runeLen() {
			m.cursor++
		}
		return ActionNone
	case "ctrl+a":
		if !m.streaming {
			m.cursor = 0
		}
		return ActionNone
	case "ctrl+e":
		if !m.streaming {
			m.cursor = m.runeLen()
		}
		return ActionNone
	case "alt+b", "ctrl+left":
		// Alt-B is the readline binding; Ctrl-Left is the multiplexer-safe
		// alias (zellij grabs Alt-F for floating panes).
		if !m.streaming {
			m.cursor = prevWordStart([]rune(m.value), m.cursor)
		}
		return ActionNone
	case "alt+f", "ctrl+right":
		if !m.streaming {
			m.cursor = nextWordStart([]rune(m.value), m.cursor)
		}
		return ActionNone
	case "esc", "escape":
		if m.streaming {
			return ActionStop
		}
		return ActionNone
	case "backspace":
		if !m.streaming {
			m.backspace()
		}
		return ActionNone
	default:
		if !m.streaming && msg.Text != "" {
			m.insert(msg.Text)
		}
		return ActionNone
	}
}

// Paste inserts bracketed-paste content at the cursor, ignoring it while
// streaming (consistent with every other edit path). Pasted text commonly
// spans multiple lines; those become soft newlines exactly like Alt+Enter,
// since visualRows already wraps on "\n".
func (m *Model) Paste(s string) {
	if m.streaming || s == "" {
		return
	}
	m.insert(s)
}

// historyPrev seeds the buffer with the previous (older) submitted prompt. On
// the first step back it stashes the in-progress draft. Returns
// ActionHistoryBoundary when already at the oldest prompt (or history is empty).
func (m *Model) historyPrev() Action {
	if m.histIdx == 0 {
		return ActionHistoryBoundary
	}
	if m.histIdx == len(m.history) {
		m.draft = m.value
	}
	m.histIdx--
	m.value = m.history[m.histIdx]
	m.cursor = m.runeLen()
	return ActionNone
}

// historyNext walks back toward the present, restoring the stashed draft when it
// steps past the newest prompt. Returns ActionHistoryBoundary when already at the
// present (draft) line.
func (m *Model) historyNext() Action {
	if m.histIdx >= len(m.history) {
		return ActionHistoryBoundary
	}
	m.histIdx++
	if m.histIdx == len(m.history) {
		m.value = m.draft
	} else {
		m.value = m.history[m.histIdx]
	}
	m.cursor = m.runeLen()
	return ActionNone
}

// runeLen is the buffer length in runes (the cursor's upper bound).
func (m *Model) runeLen() int { return len([]rune(m.value)) }

// insert writes s at the cursor and advances the cursor past it.
func (m *Model) insert(s string) {
	r := []rune(m.value)
	ins := []rune(s)
	out := make([]rune, 0, len(r)+len(ins))
	out = append(out, r[:m.cursor]...)
	out = append(out, ins...)
	out = append(out, r[m.cursor:]...)
	m.value = string(out)
	m.cursor += len(ins)
}

// backspace deletes the rune before the cursor.
func (m *Model) backspace() {
	if m.cursor == 0 {
		return
	}
	r := []rune(m.value)
	r = append(r[:m.cursor-1], r[m.cursor:]...)
	m.value = string(r)
	m.cursor--
}

// prevWordStart returns the index of the start of the word at or before pos:
// skip any spaces to the left, then skip the word's runes.
func prevWordStart(r []rune, pos int) int {
	i := pos
	for i > 0 && unicode.IsSpace(r[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(r[i-1]) {
		i--
	}
	return i
}

// nextWordStart returns the index of the start of the next word after pos: skip
// the current word's runes, then skip any spaces.
func nextWordStart(r []rune, pos int) int {
	n := len(r)
	i := pos
	for i < n && !unicode.IsSpace(r[i]) {
		i++
	}
	for i < n && unicode.IsSpace(r[i]) {
		i++
	}
	return i
}

// wrapWidth is the number of text runes per visual row: the panel width minus the
// scrollbar gutter (1), the prompt prefix (2), and one column reserved for the
// end-of-line cursor cell.
func (m *Model) wrapWidth() int { return max(m.width-1-prefixLen-1, 1) }

const prefixLen = 2

// visualRow is one rendered line: its prompt prefix + wrapped text, and the range of
// value runes it covers (for cursor mapping). start/end are absolute rune offsets
// into value.
type visualRow struct {
	prefix string
	text   string
	start  int
	end    int
}

// visualRows word-wraps the value into rendered rows at the current width. Logical
// lines (split on "\n") each wrap independently; the prompt marker "> " leads the
// first row and continuations are indented. All runes are preserved so the cursor
// maps exactly.
func (m *Model) visualRows() []visualRow {
	w := m.wrapWidth()
	var rows []visualRow
	base := 0 // absolute offset of the current logical line's first rune
	for li, lstr := range strings.Split(m.value, "\n") {
		if li > 0 {
			base++ // account for the '\n' separator between logical lines
		}
		lr := []rune(lstr)
		for _, s := range wrapRunes(lr, w) {
			prefix := "  "
			if len(rows) == 0 {
				prefix = "> "
			}
			rows = append(rows, visualRow{prefix: prefix, text: string(lr[s.start:s.end]), start: base + s.start, end: base + s.end})
		}
		base += len(lr)
	}
	if len(rows) == 0 {
		rows = append(rows, visualRow{prefix: "> "})
	}
	return rows
}

// wrapSeg is a [start,end) rune range within a single logical line.
type wrapSeg struct{ start, end int }

// wrapRunes greedily word-wraps a logical line to w columns, preferring to break
// after a space and hard-breaking an over-long word. It partitions [0,len(line)]
// contiguously so every rune has a home (exact cursor mapping).
func wrapRunes(line []rune, w int) []wrapSeg {
	if w < 1 || len(line) == 0 {
		return []wrapSeg{{0, len(line)}}
	}
	var segs []wrapSeg
	start := 0
	for start < len(line) {
		end, lastBreak := start, -1
		for end < len(line) && end-start < w {
			end++
			if line[end-1] == ' ' {
				lastBreak = end
			}
		}
		if end < len(line) && lastBreak > start {
			end = lastBreak // break after the last space that fits
		}
		segs = append(segs, wrapSeg{start, end})
		start = end
	}
	return segs
}

// View renders exactly Height rows: the wrapped input, windowed to keep the cursor
// visible, with a reverse-video cursor overlay while focused and a right-gutter
// scrollbar when the content exceeds the panel height.
func (m *Model) View() string {
	if m.height == 0 {
		return ""
	}
	rows := m.visualRows()
	curRow, curCol := m.cursorVisual(rows)

	// Window: keep the cursor row within [top, top+height).
	top := 0
	if len(rows) > m.height {
		top = len(rows) - m.height
		if curRow < top {
			top = curRow
		}
	}
	rowW := max(m.width-1, 0)
	out := make([]string, 0, m.height)
	for i := range m.height {
		line := ""
		if vi := top + i; vi < len(rows) {
			line = rows[vi].prefix + rows[vi].text
			if m.focused && vi == curRow {
				line = overlayCursor(line, curCol)
			}
		}
		// Placeholder hint: a dim soft-newline reminder on the empty first row that
		// disappears as soon as the user types (and only while focused and idle).
		if top+i == 0 && m.focused && m.value == "" && !m.streaming && m.newlineKey != "" {
			line += dimHint(" " + m.newlineKey + " for newline")
		}
		gutter := " "
		if len(rows) > m.height {
			gutter = scrollbarCell(i, top, len(rows), m.height)
		}
		out = append(out, padTo(line, rowW)+gutter)
	}
	return strings.Join(out, "\n")
}

// cursorVisual maps the rune cursor to a (visual row, column-in-row) for the overlay.
func (m *Model) cursorVisual(rows []visualRow) (int, int) {
	for i, r := range rows {
		if m.cursor < r.end {
			return i, prefixLen + (m.cursor - r.start)
		}
		// At a row boundary the cursor belongs here when this is the last row or the
		// next row begins a new logical line (a '\n' gap); otherwise it is the start
		// of the wrapped continuation row.
		if m.cursor == r.end && (i == len(rows)-1 || rows[i+1].start > r.end) {
			return i, prefixLen + (m.cursor - r.start)
		}
	}
	return 0, prefixLen
}

// dimHint renders placeholder text in gray (SGR 90), matching the muted styling
// used for non-content affordances.
func dimHint(s string) string { return "\x1b[90m" + s + "\x1b[0m" }

// overlayCursor wraps the rune at col in reverse video, extending the row with a
// blank virtual cell when the cursor sits at or past the line's end.
func overlayCursor(row string, col int) string {
	r := []rune(row)
	for len(r) <= col {
		r = append(r, ' ')
	}
	return string(r[:col]) + "\x1b[7m" + string(r[col]) + "\x1b[0m" + string(r[col+1:])
}

// padTo pads (or ANSI-aware clips) a rendered row to w display columns.
func padTo(s string, w int) string {
	dw := ansi.StringWidth(s)
	switch {
	case dw < w:
		return s + strings.Repeat(" ", w-dw)
	case dw > w:
		return ansi.Truncate(s, w, "")
	default:
		return s
	}
}

// scrollbarCell returns the gutter glyph for visible row i given the scroll offset.
func scrollbarCell(i, offset, total, track int) string {
	thumb := max(track*track/total, 1)
	span := total - track
	top := 0
	if span > 0 {
		top = (track - thumb) * offset / span
	}
	if i >= top && i < top+thumb {
		return "█"
	}
	return "░"
}
