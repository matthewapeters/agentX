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

	"agentx/internal/surfaces/scrollutil"
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
	// promptOffset is the prompt's inner scroll offset (wrapped-line index)
	// once it exceeds maxPromptRows — mirrors the output panel's per-widget
	// body offset (internal/surfaces/output.Model's widget.offset) rather
	// than truncating, so a large proposed edit is pageable, not hidden
	// (docs/architecture/behavior/approval_prompt_length_bound.feature.md).
	promptOffset int
}

// New returns an empty approval widget. Set populates it for a shown request.
func New() *Model { return &Model{} }

// Set (re)initializes the widget for a newly-shown request, resetting the
// cursor to the first option and the prompt's scroll position to the top.
// queued lists what's waiting behind this request, if anything (nil in the
// common single-pending case).
func (m *Model) Set(prompt string, options []state.ApprovalOption, queued []string) {
	m.prompt = prompt
	m.options = options
	m.cursor = 0
	m.queued = queued
	m.promptOffset = 0
}

// ScrollPrompt scrolls the prompt's inner view by n rows (positive = down),
// clamped to its content — the same "scroll a capped body" contract
// internal/surfaces/output.Model.ScrollSelected already has for a widget
// body, reused here via the identical scrollutil windowing primitives
// promptLines applies.
func (m *Model) ScrollPrompt(n int) {
	lines := scrollutil.WrapLines(m.prompt, max(m.width, 1))
	maxOffset := max(len(lines)-maxPromptRows, 0)
	m.promptOffset = scrollutil.ClampInt(m.promptOffset+n, 0, maxOffset)
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

// Update handles a key press and returns the resulting Action. PgUp/PgDn
// page the prompt's scroll position regardless of whether there are any
// options yet (the guard below only gates option-cursor movement and
// confirmation); up/down/j/k move the option cursor, enter confirms.
func (m *Model) Update(msg tea.KeyPressMsg) Action {
	switch msg.String() {
	case "pgup":
		m.ScrollPrompt(-maxPromptRows)
		return ActionNone
	case "pgdown":
		m.ScrollPrompt(maxPromptRows)
		return ActionNone
	}
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

// maxPromptRows caps the prompt's VISIBLE rows at once — regardless of how
// long m.prompt is or how narrow the panel is, so relayout()'s height
// budget stays statically bounded (docs/architecture/behavior/
// approval_prompt_length_bound.feature.md). Content beyond the cap scrolls
// (PgUp/PgDn) rather than being truncated, reusing the exact wrap/window/
// scrollbar mechanics internal/surfaces/output.Model.renderBody already
// uses for a widget body — nothing about a proposed call is ever hidden
// from the user, only paged, and any future fix to that windowing logic
// benefits this widget too instead of a second, drifting copy.
const maxPromptRows = 10

// promptLines renders the prompt's current scroll window: every wrapped row
// when it fits within maxPromptRows, or a maxPromptRows-row window with a
// scrollbar column when it doesn't.
func (m *Model) promptLines() []string {
	w := max(m.width, 1)
	if m.prompt == "" {
		return nil
	}
	lines := scrollutil.WrapLines(m.prompt, w)
	if len(lines) <= maxPromptRows {
		m.promptOffset = 0
		return lines
	}

	// Over the cap: reserve a scrollbar column and re-wrap to the narrower width.
	bodyW := max(w-1, 1)
	lines = scrollutil.WrapLines(m.prompt, bodyW)
	total := len(lines)
	maxOffset := total - maxPromptRows
	m.promptOffset = scrollutil.ClampInt(m.promptOffset, 0, maxOffset)
	window := lines[m.promptOffset : m.promptOffset+maxPromptRows]

	out := make([]string, maxPromptRows)
	for i, l := range window {
		out[i] = scrollutil.PadTo(l, bodyW) + scrollutil.ScrollbarCell(i, m.promptOffset, total, maxPromptRows)
	}
	return out
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
