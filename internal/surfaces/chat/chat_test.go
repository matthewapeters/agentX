package chat

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"agentx/internal/state"
)

// GIVEN a chat surface fed a long unbroken token (no spaces) wider than any
// panel's wrap width — the adversarial case a real session hit — across a
// range of window sizes
// WHEN View() renders
// THEN every resulting row's display width is <= the window width, so no row
// can force the terminal to insert an extra physical row that pushes the
// input/approval panels off the visible viewport.
func TestViewNeverExceedsWidth(t *testing.T) {
	longToken := strings.Repeat("x", 500)

	for _, size := range []struct{ w, h int }{
		{80, 24}, {120, 40}, {40, 15}, {200, 50},
	} {
		m := New()
		m.width = size.w
		m.height = size.h
		m.relayout()

		m.output.Apply(state.Event{
			EventType:   "AGENT_RESPONSE",
			ContentType: state.ContentAgentResponse,
			Payload:     map[string]any{"text": "prefix " + longToken + " suffix"},
			Enabled:     true,
		})
		m.relayout()

		view := m.View()
		for i, row := range strings.Split(view.Content, "\n") {
			if w := ansi.StringWidth(row); w > size.w {
				t.Errorf("size %dx%d: row %d width = %d, want <= %d: %q", size.w, size.h, i, w, size.w, row)
			}
		}
	}
}

// GIVEN content that already fits well within the window
// WHEN View() renders
// THEN the injected text survives intact — proving the clamp is a no-op in
// the common path (nothing gets truncated) rather than just safe in the
// adversarial one.
func TestViewUnchangedWhenContentFits(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.relayout()

	const want = "a short reply that easily fits"
	m.output.Apply(state.Event{
		EventType:   "AGENT_RESPONSE",
		ContentType: state.ContentAgentResponse,
		Payload:     map[string]any{"text": want},
		Enabled:     true,
	})
	m.relayout()

	if view := m.View(); !strings.Contains(view.Content, want) {
		t.Errorf("View() = %q, want it to contain unmodified text %q", view.Content, want)
	}
}

// GIVEN a string containing ANSI styling codes (runes that contribute zero
// display width) — the case padLine's rune-counting predecessor got wrong,
// found by inspection: it measured len([]rune(s)), not display width, unlike
// its sibling padCells right next to it in this file
// WHEN padLine pads/truncates it to a target width
// THEN the result's actual display width (ansi.StringWidth) is exactly the
// target — not the target rune count, which would systematically undercount
// visible content whenever styling codes are present.
func TestPadLineMeasuresDisplayWidthNotRuneCount(t *testing.T) {
	styled := "\x1b[38;5;6m" + "hi" + "\x1b[0m" // 2 visible cells, many more runes
	const width = 10

	got := padLine(styled, width)
	if w := ansi.StringWidth(got); w != width {
		t.Errorf("padLine(%q, %d) display width = %d, want %d", styled, width, w, width)
	}

	long := "\x1b[38;5;6m" + strings.Repeat("x", 20) + "\x1b[0m"
	got = padLine(long, width)
	if w := ansi.StringWidth(got); w != width {
		t.Errorf("padLine(long styled, %d) display width = %d, want %d", width, w, width)
	}
}

// GIVEN a styled spinner frame reaches statusBar (the same class of input
// that exposed padLine's bug)
// WHEN statusBar renders
// THEN the result's display width is exactly the requested width, never more
// — proving the fix without depending on whether the live spinner style
// happens to emit ANSI codes today.
func TestStatusBarMeasuresDisplayWidthNotRuneCount(t *testing.T) {
	styled := "\x1b[38;5;6m⠋\x1b[0m"
	got := statusBar(state.ProcessingState{State: state.StateWorking}, styled, 40)
	if w := ansi.StringWidth(got); w != 40 {
		t.Errorf("statusBar display width = %d, want 40: %q", w, got)
	}
}

// GIVEN a pending count of 0 or 1 (the common case — nothing else queued)
// WHEN approvalPanelTitle builds the panel title
// THEN it returns the unchanged base title, no "(1 of N)" clutter.
//
// GIVEN a pending count greater than 1
// WHEN approvalPanelTitle builds the panel title
// THEN it appends "(1 of N)" — always position 1, since decisionGate only
// ever shows the front of its FIFO queue.
func TestApprovalPanelTitleShowsCount(t *testing.T) {
	for _, tc := range []struct {
		pending int
		want    string
	}{
		{0, approvalTitleBase},
		{1, approvalTitleBase},
		{2, approvalTitleBase + " (1 of 2)"},
		{5, approvalTitleBase + " (1 of 5)"},
	} {
		if got := approvalPanelTitle(tc.pending, 0); got != tc.want {
			t.Errorf("approvalPanelTitle(%d, 0) = %q, want %q", tc.pending, got, tc.want)
		}
	}
}

// GIVEN various elapsed durations, alone and combined with a pending count
// WHEN approvalPanelTitle builds the panel title
// THEN it appends "waiting Xs"/"waiting Xm"/"waiting XhYYm" per
// formatPendingDuration's coarse boundaries, composing independently of the
// "(1 of N)" count suffix — an elapsed of 0 (no APPROVAL_REQUEST received
// yet) omits the duration suffix entirely.
func TestApprovalPanelTitleShowsDuration(t *testing.T) {
	for _, tc := range []struct {
		pending int
		elapsed time.Duration
		want    string
	}{
		{1, 0, approvalTitleBase},
		{1, 45 * time.Second, approvalTitleBase + " · waiting 45s"},
		{1, 90 * time.Second, approvalTitleBase + " · waiting 1m"},
		{1, 61 * time.Minute, approvalTitleBase + " · waiting 1h01m"},
		{3, 90 * time.Second, approvalTitleBase + " (1 of 3) · waiting 1m"},
	} {
		if got := approvalPanelTitle(tc.pending, tc.elapsed); got != tc.want {
			t.Errorf("approvalPanelTitle(%d, %v) = %q, want %q", tc.pending, tc.elapsed, got, tc.want)
		}
	}
}

// GIVEN an approvalAgeTickMsg
// WHEN proc.State is StateAwaitingInput
// THEN the tick loop reschedules itself (a non-nil Cmd) — and once
// proc.State leaves StateAwaitingInput, the same message produces no Cmd,
// the self-stopping contract the existing spinner ticker already has.
func TestApprovalAgeTickStopsWhenNoLongerAwaiting(t *testing.T) {
	m := New()
	m.proc = state.ProcessingState{State: state.StateAwaitingInput}
	if _, cmd := m.Update(approvalAgeTickMsg{}); cmd == nil {
		t.Error("Update(approvalAgeTickMsg{}) while awaiting input returned a nil Cmd, want the tick to reschedule")
	}

	m.proc = state.ProcessingState{State: state.StateIdle}
	if _, cmd := m.Update(approvalAgeTickMsg{}); cmd != nil {
		t.Error("Update(approvalAgeTickMsg{}) while idle rescheduled a tick, want nil (self-stopping)")
	}
}

// GIVEN a []string payload field (native, in-process) or a []any of strings
// (JSON-decoded, remote transport) — including a stray non-string element,
// which a remote surface should never send but a defensive decoder should
// not panic on
// WHEN decodeStringSlice reads it
// THEN both wire shapes decode to the same []string, and the non-string
// element is skipped rather than crashing the decode.
func TestDecodeStringSliceBothWireShapes(t *testing.T) {
	want := []string{"a", "b"}
	if got := decodeStringSlice([]string{"a", "b"}); !slices.Equal(got, want) {
		t.Errorf("decodeStringSlice([]string) = %v, want %v", got, want)
	}
	if got := decodeStringSlice([]any{"a", "b"}); !slices.Equal(got, want) {
		t.Errorf("decodeStringSlice([]any) = %v, want %v", got, want)
	}
	if got := decodeStringSlice([]any{"a", 42, "b"}); !slices.Equal(got, want) {
		t.Errorf("decodeStringSlice([]any with non-string) = %v, want %v (non-string skipped)", got, want)
	}
	if got := decodeStringSlice(nil); got != nil {
		t.Errorf("decodeStringSlice(nil) = %v, want nil", got)
	}
}

// GIVEN an APPROVAL_REQUEST event's pending and since fields arrive as
// either native Go values (int, int64 — in-process chat surface) or
// JSON-decoded float64 (a remote surface attached over transport)
// WHEN chat.Model handles the event
// THEN both wire shapes decode to the same count and the same pendingSince
// — proving the transport-decoded path works identically to the native path,
// the same dual-mode robustness state.DecodeApprovalOptions already
// demonstrates for the sibling options field.
func TestApprovalRequestEventPendingDecodesBothWireShapes(t *testing.T) {
	wantSince := time.UnixMilli(1_700_000_000_000)
	for _, tc := range []struct {
		pending any
		since   any
		queued  any
	}{
		{3, wantSince.UnixMilli(), []string{"run http_get?"}},
		{float64(3), float64(wantSince.UnixMilli()), []any{"run http_get?"}},
	} {
		m := New()
		m.width = 80
		m.height = 24
		// relayout only sizes the approval widget while awaiting_input (it's
		// otherwise not shown) — set that first so approval.View() below
		// renders at a real width instead of its zero-value default.
		m.proc = state.ProcessingState{State: state.StateAwaitingInput}
		m.relayout()

		updated, _ := m.Update(EventMsg(state.Event{
			EventType:   "APPROVAL_REQUEST",
			ContentType: state.ContentApprovalRequest,
			Payload: map[string]any{
				"prompt":  "run write_file?",
				"options": []any{map[string]any{"label": "Deny", "decision": "deny"}},
				"pending": tc.pending,
				"since":   tc.since,
				"queued":  tc.queued,
			},
			Enabled: true,
		}))
		m = updated.(Model)

		if m.pending != 3 {
			t.Errorf("pending = %v: m.pending = %d, want 3", tc.pending, m.pending)
		}
		if !m.pendingSince.Equal(wantSince) {
			t.Errorf("since = %v: m.pendingSince = %v, want %v", tc.since, m.pendingSince, wantSince)
		}
		if view := m.approval.View(); !strings.Contains(view, "run http_get?") {
			t.Errorf("queued = %v: approval.View() = %q, want it to contain the queued prompt", tc.queued, view)
		}
	}
}

// GIVEN a decision is pending (StateAwaitingInput) and its prompt is long
// enough to have scrolled content of its own
// WHEN PgDown is pressed
// THEN it scrolls the approval widget's prompt instead of jumping chat
// focus to the output panel — the ordinary PgUp/PgDn-jumps-focus behavior
// is deliberately overridden while awaiting input (docs/architecture/
// behavior/approval_prompt_length_bound.feature.md), since those keys
// would otherwise never reach the approval widget at all.
func TestPgDownWhileAwaitingInputScrollsApprovalPromptNotFocus(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.proc = state.ProcessingState{State: state.StateAwaitingInput}
	m.relayout()

	words := make([]string, 200)
	for i := range words {
		words[i] = "word"
	}
	m.approval.Set(strings.Join(words, " "), []state.ApprovalOption{{Label: "Deny", Decision: "deny"}}, nil)
	m.relayout()
	before := m.approval.View()

	updated, _ := m.Update(tea.KeyPressMsg{Text: "pgdown", Code: tea.KeyPgDown})
	m2 := updated.(Model)

	if m2.focus == focusOutput {
		t.Error("pgdown while awaiting input jumped focus to output, want it to stay routed to the approval widget")
	}
	if after := m2.approval.View(); after == before {
		t.Error("approval.View() unchanged after pgdown while awaiting input, want the prompt to have scrolled")
	}
}

// GIVEN a deliberately short terminal, an oversized prompt, and 4 options
// WHEN relayout() computes the approval panel's height
// THEN every option is still visible in the rendered approval widget — the
// confirmed failure from session raw-interesting-elephant, where relayout()
// gave the approval panel its full unclamped DesiredHeight() regardless of
// how little of the terminal was actually left, so only the first option
// (everything the prompt didn't already consume) ever made it on-screen
// (docs/architecture/behavior/approval_panel_height_budget.feature.md).
func TestRelayoutClampsApprovalHeightToTerminal(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 8 // deliberately too short for prompt + options unclamped
	m.proc = state.ProcessingState{State: state.StateAwaitingInput}

	words := make([]string, 200)
	for i := range words {
		words[i] = "word"
	}
	opts := []state.ApprovalOption{
		{Label: "Approve for this session", Decision: "session"},
		{Label: "Approve for this plan", Decision: "plan"},
		{Label: "Approve for all sessions", Decision: "global"},
		{Label: "Deny", Decision: "deny"},
	}
	m.approval.Set(strings.Join(words, " "), opts, nil)
	m.relayout()

	view := m.approval.View()
	for _, opt := range opts {
		if !strings.Contains(view, opt.Label) {
			t.Errorf("approval.View() after relayout on a short terminal is missing option %q: %q", opt.Label, view)
		}
	}
}

// GIVEN sessions raw-interesting-elephant/naive-stunning-eagle: many resolved
// approval request/decision/result cycles in scrollback whose proposal text
// is real, tab-indented Go source (a write_file content argument), ending
// in a "stopped: tool-call limit" agent_response, across a range of
// realistic and short terminal sizes
// WHEN the full chat surface renders
// THEN View()'s total row count never exceeds the terminal height — the
// confirmed overflow that pushed the input panel off-screen with no live
// approval pending (so neither of the two previous approval-panel-specific
// fixes applies): a tab-containing line ansi.StringWidth measured as
// fitting exactly still got soft-wrapped an extra row by lipgloss's actual
// renderer, an untracked row nothing in relayout()'s budget accounted for
// (docs/architecture/behavior/scrollutil_tab_width_disagreement.feature.md).
func TestViewNeverExceedsHeightWithTabContentInScrollback(t *testing.T) {
	tabBody := "write_file content=package task\n\nimport (\n\t\"encoding/json\"\n\n\t\"testing\"\n)\n\nfunc T… path=internal/prompting/task/hypothesis_test.go"

	for _, size := range []struct{ w, h int }{
		{100, 30}, {80, 24}, {60, 15}, {60, 10}, {40, 8},
	} {
		m := New()
		m.width = size.w
		m.height = size.h
		m.relayout()

		for i := 0; i < 15; i++ {
			m.output.Apply(state.Event{EventType: "TOOL_CALL", ContentType: state.ContentApprovalRequest,
				Payload: map[string]any{"prompt": tabBody}, Enabled: true})
			m.output.Apply(state.Event{EventType: "APPROVAL_DECISION", ContentType: state.ContentApprovalDecision,
				Payload: map[string]any{"prompt": tabBody, "chosen_label": "Approve for this session"}, Enabled: true})
			m.output.Apply(state.Event{EventType: "TOOL_RESULT", ContentType: state.ContentToolResult, ToolName: "write_file",
				Payload: map[string]any{"text": ""}, Enabled: true})
			m.relayout()
		}
		m.output.Apply(state.Event{EventType: "AGENT_RESPONSE", ContentType: state.ContentAgentResponse,
			Payload: map[string]any{"text": "[stopped: reached the tool-call limit for this turn without a final answer]"}, Enabled: true})
		m.relayout()

		rows := strings.Split(m.View().Content, "\n")
		if len(rows) > size.h {
			t.Errorf("size %dx%d: View() produced %d rows, want <= m.height (%d)", size.w, size.h, len(rows), size.h)
		}
	}
}

// GIVEN a panel's content is "" — the convention every panel's View() uses
// for zero content rows (e.g. the output panel when relayout() clamps its
// height to exactly 0 on a very short terminal)
// WHEN frame() renders it
// THEN the result is exactly 2 rows (top + bottom border) — not 3.
// strings.Split("", "\n") returns [""] (length 1, not 0), so naively
// looping over that split renders one phantom blank content row nothing in
// relayout()'s chrome budget accounted for (docs/architecture/behavior/
// chat_frame_empty_content_phantom_row.feature.md).
func TestFrameEmptyContentProducesNoPhantomRow(t *testing.T) {
	m := New()
	m.width = 40

	got := m.frame("", false, false, "")
	if len(got) != 2 {
		t.Fatalf("frame(\"\", ...) produced %d rows, want 2 (border only): %v", len(got), got)
	}
}

// GIVEN a panel's content is ordinary, non-empty text
// WHEN frame() renders it
// THEN behavior is unchanged from before this fix — the empty-content
// special case is a no-op for every other input.
func TestFrameNonEmptyContentUnaffected(t *testing.T) {
	m := New()
	m.width = 40

	got := m.frame("one line", false, false, "")
	if len(got) != 3 {
		t.Fatalf("frame(\"one line\", ...) produced %d rows, want 3 (border + 1 content row): %v", len(got), got)
	}
}
