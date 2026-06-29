// Package input is the chat surface's input panel: a multi-line prompt editor.
// It supports typing, Enter-to-submit (disabled while streaming), Shift+Enter
// newline, backspace, a stop/interrupt action while streaming, readline-style
// history seeding (↑/↓), and a cursor with readline movement keys.
//
// Source contract: docs/ux/03_PANEL_DETAILS.md PD-02 (re-authored for the TUI).
// Backlog task: CHT-B3.
package input

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
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
	value     string
	cursor    int // rune index into value; edits and movement are relative to it
	focused   bool
	streaming bool
	history   []string
	histIdx   int
	draft     string
}

// New returns an input panel, focused, with a default height of 3 rows.
func New() *Model { return &Model{focused: true, height: 3} }

// SetSize sets the panel's render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 0)
	if height > 0 {
		m.height = height
	}
}

// Height returns the panel's row count.
func (m *Model) Height() int { return m.height }

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
	case "shift+enter":
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
	case "alt+b":
		if !m.streaming {
			m.cursor = prevWordStart([]rune(m.value), m.cursor)
		}
		return ActionNone
	case "alt+f":
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

// cursorLineCol maps the rune cursor to a (line, column) within value, where
// column is the rune offset on that line (excluding the render prefix).
func (m *Model) cursorLineCol() (int, int) {
	line, col := 0, 0
	r := []rune(m.value)
	for i := 0; i < m.cursor && i < len(r); i++ {
		if r[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return line, col
}

// View renders exactly Height rows: the (multi-line) input with a prompt marker,
// bottom-aligned, padded with blanks. While focused it overlays a reverse-video
// cursor cell at the cursor position (a virtual cell at end-of-line).
func (m *Model) View() string {
	if m.height == 0 {
		return ""
	}
	curLine, curCol := m.cursorLineCol()
	valueLines := strings.Split(m.value, "\n")
	rows := make([]string, 0, len(valueLines))
	for i, l := range valueLines {
		prefix := "  "
		if i == 0 {
			prefix = "> "
		}
		row := fitWidth(prefix+l, m.width)
		if m.focused && i == curLine {
			row = overlayCursor(row, len([]rune(prefix))+curCol)
		}
		rows = append(rows, row)
	}
	if len(rows) > m.height {
		rows = rows[len(rows)-m.height:]
	}
	for len(rows) < m.height {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

// overlayCursor wraps the rune at col in reverse video, extending the row with a
// blank virtual cell when the cursor sits at or past the line's end.
func overlayCursor(row string, col int) string {
	r := []rune(row)
	for len(r) <= col {
		r = append(r, ' ')
	}
	return string(r[:col]) + "\x1b[7m" + string(r[col]) + "\x1b[0m" + string(r[col+1:])
}

func fitWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width])
}
