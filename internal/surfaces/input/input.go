// Package input is the chat surface's input panel: a fixed-height prompt region.
// CHT-B1 provides the buffer, focus, and sizing; CHT-B3 adds multi-line editing,
// submit/newline keys, and streaming-disabled/stop behavior.
//
// Source contract: docs/ux/03_PANEL_DETAILS.md PD-02 (re-authored for the TUI).
package input

import "strings"

// Model is the input panel state.
type Model struct {
	width   int
	height  int
	value   string
	focused bool
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

// View renders exactly Height rows: the prompt line plus blank padding.
func (m *Model) View() string {
	if m.height == 0 {
		return ""
	}
	rows := []string{fitWidth("> "+m.value, m.width)}
	for len(rows) < m.height {
		rows = append(rows, "")
	}
	return strings.Join(rows[:m.height], "\n")
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
