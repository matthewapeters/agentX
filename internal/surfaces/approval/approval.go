// Package approval is the chat surface's swapped-in input-panel widget for
// interactive decisions: a prompt and a navigable list of options. It renders
// generically — it never hardcodes a per-kind option vocabulary — so it works
// identically for tool-execution approval, verb-continuation approval, or any
// future decision kind the runtime introduces. Up/Down (and j/k) move a
// highlighted-row cursor; Enter confirms the highlighted option.
//
// The host swaps this widget into the input panel's slot in place of the
// free-text input.Model while state.StateAwaitingInput holds, and swaps it
// back out once the decision resolves.
package approval

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"agentx/internal/state"
)

// Action is the outcome of handling a key, signaled to the host surface.
type Action int

const (
	// ActionNone means the key was handled internally (or ignored).
	ActionNone Action = iota
	// ActionConfirm means Enter confirmed the highlighted option; the host reads
	// Selected() for the chosen option.
	ActionConfirm
)

// Model is the approval widget state: a prompt and a navigable option list.
type Model struct {
	width, height int
	prompt        string
	options       []state.ApprovalOption
	cursor        int
}

// New returns an empty approval widget. Set populates it for a shown request.
func New() *Model { return &Model{} }

// Set (re)initializes the widget for a newly-shown request, resetting the
// cursor to the first option.
func (m *Model) Set(prompt string, options []state.ApprovalOption) {
	m.prompt = prompt
	m.options = options
	m.cursor = 0
}

// SetSize sets the panel's render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 0)
	m.height = max(height, 0)
}

// DesiredHeight is the number of rows the panel wants at its current width:
// the wrapped prompt's row count plus one row per option.
func (m *Model) DesiredHeight() int {
	return max(len(m.promptLines())+len(m.options), 1)
}

// Selected returns the currently-highlighted option. Zero value if there are
// no options (the host should not call this before Set).
func (m *Model) Selected() state.ApprovalOption {
	if m.cursor < 0 || m.cursor >= len(m.options) {
		return state.ApprovalOption{}
	}
	return m.options[m.cursor]
}

// Update handles a key press and returns the resulting Action.
func (m *Model) Update(msg tea.KeyPressMsg) Action {
	if len(m.options) == 0 {
		return ActionNone
	}
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.options)-1 {
			m.cursor++
		}
	case "enter":
		return ActionConfirm
	}
	return ActionNone
}

// promptLines word-wraps the prompt to the panel width, one entry per visual row.
func (m *Model) promptLines() []string {
	w := max(m.width, 1)
	if m.prompt == "" {
		return nil
	}
	return wrapText(m.prompt, w)
}

// View renders the prompt (wrapped to width) followed by one row per option,
// the highlighted row marked with a pointer glyph.
func (m *Model) View() string {
	var rows []string
	rows = append(rows, m.promptLines()...)
	for i, opt := range m.options {
		marker := "  "
		line := opt.Label
		if i == m.cursor {
			marker = "▸ "
			line = "\x1b[1m" + line + "\x1b[0m"
		}
		rows = append(rows, marker+line)
	}
	if len(rows) == 0 {
		return ""
	}
	return strings.Join(rows, "\n")
}

// wrapText greedily word-wraps s to w columns, preferring to break after a
// space and hard-breaking an over-long word.
func wrapText(s string, w int) []string {
	if w < 1 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := words[0]
	for _, word := range words[1:] {
		if ansi.StringWidth(cur)+1+ansi.StringWidth(word) <= w {
			cur += " " + word
		} else {
			lines = append(lines, cur)
			cur = word
		}
	}
	lines = append(lines, cur)
	return lines
}
