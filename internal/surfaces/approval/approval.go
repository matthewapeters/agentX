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
	"fmt"
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

// maxQueuedPreview bounds how many queued-behind prompts the panel lists
// individually before collapsing the rest into a "+N more" summary row —
// the panel's height must stay bounded regardless of queue depth (a plan
// dispatching a dozen write calls must not blow the height budget Phase A's
// clamp exists to protect); docs/architecture/behavior/
// chat_pending_approval_batch_view.feature.md.
const maxQueuedPreview = 5

// Model is the approval widget state: a prompt and a navigable option list.
type Model struct {
	width, height int
	prompt        string
	options       []state.ApprovalOption
	cursor        int
	// queued is the prompt text of every request waiting BEHIND this one —
	// a read-only preview, reviewed together with the current prompt but
	// resolved one at a time same as always (the gate's queue is unaffected).
	queued []string
}

// New returns an empty approval widget. Set populates it for a shown request.
func New() *Model { return &Model{} }

// Set (re)initializes the widget for a newly-shown request, resetting the
// cursor to the first option. queued lists what's waiting behind this
// request, if anything (nil in the common single-pending case).
func (m *Model) Set(prompt string, options []state.ApprovalOption, queued []string) {
	m.prompt = prompt
	m.options = options
	m.cursor = 0
	m.queued = queued
}

// SetSize sets the panel's render dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 0)
	m.height = max(height, 0)
}

// DesiredHeight is the number of rows the panel wants at its current width:
// the wrapped prompt's row count, plus one row per option, plus the
// queued-preview rows (zero when nothing is queued behind this request).
func (m *Model) DesiredHeight() int {
	return max(len(m.promptLines())+len(m.options)+len(m.queuedLines()), 1)
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

// maxPromptRows caps the rendered prompt at this many rows regardless of
// how long m.prompt is or how narrow the panel is — the defensive backstop
// on top of proposalText's own truncation (internal/runtime/approval.go):
// a widget that trusts every caller to have already bounded its input
// correctly is exactly the assumption a prior overflow bug argued against
// (docs/architecture/behavior/approval_prompt_length_bound.feature.md).
// Capping by RENDERED ROW COUNT, not character count, holds regardless of
// panel width — a narrower panel wraps more rows per character, which a
// char-count cap alone wouldn't protect against.
const maxPromptRows = 10

// promptLines word-wraps the prompt to the panel width, one entry per visual
// row, capped at maxPromptRows with a trailing "…" row if truncated.
func (m *Model) promptLines() []string {
	w := max(m.width, 1)
	if m.prompt == "" {
		return nil
	}
	lines := wrapText(m.prompt, w)
	if len(lines) > maxPromptRows {
		lines = append(lines[:maxPromptRows], "…")
	}
	return lines
}

// View renders the prompt (wrapped to width) followed by one row per option,
// the highlighted row marked with a pointer glyph, followed by a
// queued-preview section (nothing, when this request has nothing queued
// behind it).
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
	rows = append(rows, m.queuedLines()...)
	if len(rows) == 0 {
		return ""
	}
	return strings.Join(rows, "\n")
}

// queuedLines renders up to maxQueuedPreview queued prompts, each truncated
// (not wrapped) to one row — a compact glance, not full detail; the user
// gets the full prompt+options treatment when each one's own turn comes. A
// "+N more" summary row covers anything beyond the cap. Returns nil (no
// section at all) when nothing is queued behind this request — the common
// case renders identically to before this feature existed.
func (m *Model) queuedLines() []string {
	if len(m.queued) == 0 {
		return nil
	}
	w := max(m.width, 1)
	lines := []string{"Also waiting:"}
	show := min(len(m.queued), maxQueuedPreview)
	for _, p := range m.queued[:show] {
		lines = append(lines, "  "+ansi.Truncate(p, max(w-2, 1), "…"))
	}
	if more := len(m.queued) - show; more > 0 {
		lines = append(lines, fmt.Sprintf("  … and %d more", more))
	}
	return lines
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
