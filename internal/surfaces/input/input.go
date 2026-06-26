// Package input is the chat surface's input panel: a multi-line prompt editor.
// It supports typing, Enter-to-submit (disabled while streaming), Shift+Enter
// newline, backspace, and a stop/interrupt action while streaming.
//
// Source contract: docs/ux/03_PANEL_DETAILS.md PD-02 (re-authored for the TUI).
// Backlog task: CHT-B3.
package input

import (
	"strings"

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
)

// Model is the input panel state.
type Model struct {
	width     int
	height    int
	value     string
	focused   bool
	streaming bool
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

// Reset clears the input text.
func (m *Model) Reset() { m.value = "" }

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
		return ActionSubmit
	case "shift+enter":
		if !m.streaming {
			m.value += "\n"
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
			m.value += msg.Text
		}
		return ActionNone
	}
}

func (m *Model) backspace() {
	if m.value == "" {
		return
	}
	r := []rune(m.value)
	m.value = string(r[:len(r)-1])
}

// View renders exactly Height rows: the (multi-line) input with a prompt marker,
// bottom-aligned, padded with blanks.
func (m *Model) View() string {
	if m.height == 0 {
		return ""
	}
	valueLines := strings.Split(m.value, "\n")
	rows := make([]string, 0, len(valueLines))
	for i, l := range valueLines {
		prefix := "  "
		if i == 0 {
			prefix = "> "
		}
		rows = append(rows, fitWidth(prefix+l, m.width))
	}
	if len(rows) > m.height {
		rows = rows[len(rows)-m.height:]
	}
	for len(rows) < m.height {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
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
