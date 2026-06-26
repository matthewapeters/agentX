// Package output is the chat surface's output panel: a fixed-height region that
// renders conversation/event lines. CHT-B1 provides line storage and sizing;
// CHT-B2 adds event-entry rendering, scrolling, and collapsible blocks.
//
// Source contract: docs/ux/03_PANEL_DETAILS.md PD-01 (re-authored for the TUI).
package output

import "strings"

// Model is the output panel state.
type Model struct {
	width  int
	height int
	lines  []string
}

// New returns an empty output panel.
func New() *Model { return &Model{} }

// SetSize sets the panel's render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 0)
	m.height = max(height, 0)
}

// Height returns the panel's row count.
func (m *Model) Height() int { return m.height }

// Append adds a line to the panel.
func (m *Model) Append(line string) { m.lines = append(m.lines, line) }

// View renders exactly Height rows, showing the most recent lines and padding
// with blanks so the panel always fills its region.
func (m *Model) View() string {
	if m.height == 0 {
		return ""
	}
	start := 0
	if len(m.lines) > m.height {
		start = len(m.lines) - m.height
	}
	rows := make([]string, 0, m.height)
	for _, l := range m.lines[start:] {
		rows = append(rows, fitWidth(l, m.width))
	}
	for len(rows) < m.height {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

// fitWidth truncates s to width runes (width 0 leaves it unchanged).
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
