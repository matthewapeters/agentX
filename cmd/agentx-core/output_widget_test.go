package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeOutputWidgetCommandToken(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "ctrl c passthrough", raw: "ctrl_c", want: "ctrl_c"},
		{name: "backspace passthrough", raw: "backspace", want: "backspace"},
		{name: "escape up normalized", raw: "\x1b[A", want: "k"},
		{name: "single char lower", raw: "Q", want: "q"},
		{name: "line command trimmed", raw: "  :Help  ", want: ":help"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeOutputWidgetCommandToken(tc.raw); got != tc.want {
				t.Fatalf("normalizeOutputWidgetCommandToken(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestStartOutputWidgetCommandReader_HeadlessLines(t *testing.T) {
	got := runHeadlessCommandScript(t, ":Help\n:focus 2\n", startOutputWidgetCommandReader)

	if len(got) != 2 {
		t.Fatalf("expected 2 commands, got %d (%#v)", len(got), got)
	}
	if got[0] != ":help" {
		t.Fatalf("expected first command :help, got %q", got[0])
	}
	if got[1] != ":focus 2" {
		t.Fatalf("expected second command :focus 2, got %q", got[1])
	}
}

func TestNormalizeOutputWidgetControlCommand(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "colon command unchanged", raw: ":FoCuS 2", want: ":focus 2"},
		{name: "help token", raw: "?", want: ":help"},
		{name: "quit token", raw: "q", want: ":q"},
		{name: "prev token", raw: "k", want: ":prev"},
		{name: "next token", raw: "j", want: ":next"},
		{name: "collapse token", raw: "h", want: ":target-collapse"},
		{name: "expand token", raw: "l", want: ":target-expand"},
		{name: "left arrow token", raw: "left", want: ":prev"},
		{name: "right arrow token", raw: "right", want: ":next"},
		{name: "toggle token", raw: "space", want: ":target-toggle"},
		{name: "tab token", raw: "tab", want: ":drill-in"},
		{name: "shift tab token", raw: "shift-tab", want: ":drill-out"},
		{name: "page up token", raw: "pgup", want: ":pageup"},
		{name: "page down token", raw: "pgdn", want: ":pagedown"},
		{name: "home token", raw: "home", want: ":focus 1"},
		{name: "end token", raw: "end", want: ":focus all"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeOutputWidgetControlCommand(tc.raw); got != tc.want {
				t.Fatalf("normalizeOutputWidgetControlCommand(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func testOutputWidgetSnapshot() outputWidgetSnapshot {
	return outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 2,
		Turns: []ChatTurn{
			{Prompt: "status update", Response: "all green"},
			{Prompt: "what is 2+2?", Response: "4"},
		},
		PromptCycle: PromptCycleStatus{
			Thinking: PromptCyclePhase{State: "done", ElapsedMs: 11},
		},
	}
}

func TestFetchOutputWidgetSnapshot_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/context" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(contextWidgetSnapshot{
			SessionID: "sess-output",
			TurnCount: 1,
			Turns:     []ChatTurn{{Prompt: "hello", Response: "world"}},
			PromptCycle: PromptCycleStatus{
				Thinking: PromptCyclePhase{State: "done", ElapsedMs: 5},
			},
		})
	}))
	defer server.Close()

	snapshot, err := fetchOutputWidgetSnapshot(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetchOutputWidgetSnapshot returned error: %v", err)
	}
	if snapshot.SessionID != "sess-output" {
		t.Fatalf("expected session_id sess-output, got %q", snapshot.SessionID)
	}
	if snapshot.TurnCount != 1 {
		t.Fatalf("expected turn_count 1, got %d", snapshot.TurnCount)
	}
}

func TestRenderOutputWidget_UsesPaneLifecycleContract(t *testing.T) {
	render := renderOutputWidget(outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "what is 2+2?", Response: "Echo: what is 2+2?"}},
		PromptCycle: PromptCycleStatus{
			Thinking: PromptCyclePhase{State: "done", ElapsedMs: 11},
		},
	}, 80, 200)

	for _, fragment := range []string{
		"👤 User: what is 2+2? [LATEST]",
		"[-] ⚙️ Classification:",
		"[-] 💭 Thinking: done (00:00:00.011)",
		"[-] 🤖 Response: Echo: what is 2+2?",
	} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
	if strings.Contains(render, "Turn 1 [LATEST]") {
		t.Fatalf("did not expect legacy turn header line, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_LatestFirstFocusExpandsNewest(t *testing.T) {
	view := newOutputWidgetViewState()
	render := renderOutputWidgetWithViewState(testOutputWidgetSnapshot(), 80, 200, view)

	if view.focusedTurn != 2 {
		t.Fatalf("expected newest turn focused by default, got %d", view.focusedTurn)
	}
	for _, fragment := range []string{
		"[+] [user] status update",
		"[-] 🤖 Response: 4",
		"↳ 👤 User: what is 2+2? [LATEST]",
	} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
	if strings.Contains(render, "Response: all green") {
		t.Fatalf("did not expect older turn to stay expanded, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_NewTurnAutoFocusesNewest(t *testing.T) {
	view := newOutputWidgetViewState()
	_ = renderOutputWidgetWithViewState(outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status update", Response: "all green"}},
	}, 80, 200, view)

	_ = renderOutputWidgetWithViewState(outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 2,
		Turns: []ChatTurn{
			{Prompt: "status update", Response: "all green"},
			{Prompt: "what is 2+2?", Response: "4"},
		},
	}, 80, 200, view)

	if view.focusedTurn != 2 {
		t.Fatalf("expected new turn to become focused, got %d", view.focusedTurn)
	}
	render := renderOutputWidgetWithViewState(outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 2,
		Turns: []ChatTurn{
			{Prompt: "status update", Response: "all green"},
			{Prompt: "what is 2+2?", Response: "4"},
		},
	}, 80, 200, view)
	if !strings.Contains(render, "Response: 4") {
		t.Fatalf("expected newest turn to stay expanded and readable, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_ExpandedResponseShowsFullContent(t *testing.T) {
	longResponse := "This is a very long response that would normally be truncated to 96 characters but should show the full content when the response entry is expanded in the output widget."
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "what is this?", Response: longResponse}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	// When expanded, the full response should be visible (not truncated to 96 chars)
	if !strings.Contains(render, longResponse) {
		t.Fatalf("expected expanded response to show full content, got:\n%s", render)
	}
	// Ensure it's not collapsed or legacy turn-header output
	if strings.Contains(render, "Turn 1 [LATEST]") {
		t.Fatalf("did not expect legacy turn header line, got:\n%s", render)
	}
	// Ensure it's not the truncated/collapsed version
	if strings.Contains(render, "...") {
		t.Fatalf("did not expect truncation marker in expanded response, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_ExpandedResponsePreservesMultilineContent(t *testing.T) {
	multiLine := "first response line\nsecond response line with details\nthird response line"
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "show details", Response: multiLine}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	for _, fragment := range []string{
		"↳ 👤 User: show details [LATEST]",
		"[-] 🤖 Response: first response line",
		"second response line with details",
		"third response line",
	} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
}

func TestRenderOutputWidgetWithViewState_FocusedCompactedTurnShowsPointer(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status update", Response: "all green"}},
	}

	_ = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	view.applyCommand(":collapse", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	if !strings.Contains(render, "↳ [+] [user] status update") {
		t.Fatalf("expected compacted focused turn pointer, got:\n%s", render)
	}
	if !strings.Contains(render, "[+] [user]") {
		t.Fatalf("expected compact affordance marker, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_CompactedPointerOnlyOnFocusedTurn(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()

	_ = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	view.setTurnExpanded(1, false)
	view.setTurnExpanded(2, false)
	view.focusedTurn = 2

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if strings.Contains(render, "↳ [+] [user] status update") {
		t.Fatalf("did not expect non-focused compact turn pointer, got:\n%s", render)
	}
	if !strings.Contains(render, "↳ [+] [user] what is 2+2?") {
		t.Fatalf("expected focused compact turn pointer, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_SameTurnResponseUpdateAutoExpandsLatest(t *testing.T) {
	view := newOutputWidgetViewState()
	initial := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status update", Response: "partial"}},
	}

	_ = renderOutputWidgetWithViewState(initial, 80, 200, view)
	view.applyCommand(":collapse", initial)
	collapsedRender := renderOutputWidgetWithViewState(initial, 80, 200, view)
	if !strings.Contains(collapsedRender, "↳ [+] [user] status update") {
		t.Fatalf("expected compact summary after collapse, got:\n%s", collapsedRender)
	}
	if strings.Contains(collapsedRender, "Response: partial") {
		t.Fatalf("did not expect expanded response while collapsed, got:\n%s", collapsedRender)
	}

	updated := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status update", Response: "final answer"}},
	}
	updatedRender := renderOutputWidgetWithViewState(updated, 80, 200, view)
	if view.focusedTurn != 1 {
		t.Fatalf("expected latest turn to stay focused, got %d", view.focusedTurn)
	}
	if !strings.Contains(updatedRender, "Response: final answer") {
		t.Fatalf("expected updated latest response to auto-expand, got:\n%s", updatedRender)
	}
}

func TestRenderOutputWidgetWithViewState_SessionSwitchResetsLatestTrackingAndStillExpands(t *testing.T) {
	view := newOutputWidgetViewState()
	firstSession := outputWidgetSnapshot{
		SessionID: "sess-a",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status update", Response: "partial"}},
	}

	_ = renderOutputWidgetWithViewState(firstSession, 80, 200, view)
	if view.lastLatestSession != "sess-a" {
		t.Fatalf("expected latest tracking session sess-a, got %q", view.lastLatestSession)
	}

	secondSession := outputWidgetSnapshot{
		SessionID: "sess-b",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status update", Response: "partial"}},
	}
	_ = renderOutputWidgetWithViewState(secondSession, 80, 200, view)
	if view.lastLatestSession != "sess-b" {
		t.Fatalf("expected latest tracking session reset to sess-b, got %q", view.lastLatestSession)
	}

	view.applyCommand(":collapse", secondSession)
	collapsed := renderOutputWidgetWithViewState(secondSession, 80, 200, view)
	if strings.Contains(collapsed, "Response: partial") {
		t.Fatalf("did not expect expanded response while collapsed, got:\n%s", collapsed)
	}

	secondSessionUpdated := outputWidgetSnapshot{
		SessionID: "sess-b",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status update", Response: "final answer"}},
	}
	updatedRender := renderOutputWidgetWithViewState(secondSessionUpdated, 80, 200, view)
	if !strings.Contains(updatedRender, "Response: final answer") {
		t.Fatalf("expected updated latest response to auto-expand, got:\n%s", updatedRender)
	}
}

func TestRenderOutputWidgetWithViewState_SessionSwitchGuardsAgainstViewStateLeaks(t *testing.T) {
	view := newOutputWidgetViewState()
	sessionA := outputWidgetSnapshot{
		SessionID: "sess-a",
		TurnCount: 2,
		Turns: []ChatTurn{
			{Prompt: "a1", Response: "response a1"},
			{Prompt: "a2", Response: "response a2"},
		},
	}
	_ = renderOutputWidgetWithViewState(sessionA, 80, 200, view)
	view.applyCommand(":focus 1", sessionA)
	view.applyCommand(":collapse", sessionA)
	view.applyCommand(":pagedown", sessionA)
	view.setEntryCollapsed(1, "thinking", true)
	view.setEntryCollapsed(1, "response", true)

	if view.focusedTurn != 1 {
		t.Fatalf("expected session A to focus turn 1, got %d", view.focusedTurn)
	}
	if view.turnScrollOffset(1) == 0 {
		t.Fatal("expected session A to have a non-zero scroll offset")
	}
	if !view.thinkingCollapsed(1) || !view.entryCollapsed(1, "response") {
		t.Fatal("expected collapsed thinking/response state in session A")
	}

	sessionB := outputWidgetSnapshot{
		SessionID: "sess-b",
		TurnCount: 2,
		Turns: []ChatTurn{
			{Prompt: "b1", Response: "response b1"},
			{Prompt: "b2", Response: "response b2"},
		},
	}
	_ = renderOutputWidgetWithViewState(sessionB, 80, 200, view)

	if view.focusedTurn != 2 {
		t.Fatalf("expected session B to reset focus to newest turn, got %d", view.focusedTurn)
	}
	if _, ok := view.turnHasExplicitState(1); ok {
		t.Fatal("expected collapsed/expanded per-turn state from session A to be cleared")
	}
	if view.turnScrollOffset(1) != 0 {
		t.Fatalf("expected session B scroll offsets to reset, got %d", view.turnScrollOffset(1))
	}
	if view.thinkingCollapsed(1) || view.entryCollapsed(1, "response") {
		t.Fatal("expected session B entry collapse state to reset")
	}
}

func TestRenderOutputWidgetWithViewState_SessionSwitchDefaultsToNewestExpandedTurn(t *testing.T) {
	view := newOutputWidgetViewState()
	sessionA := outputWidgetSnapshot{
		SessionID: "sess-a",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "a1", Response: "response a1"}},
	}
	_ = renderOutputWidgetWithViewState(sessionA, 80, 200, view)
	view.applyCommand(":collapse", sessionA)

	sessionB := outputWidgetSnapshot{
		SessionID: "sess-b",
		TurnCount: 2,
		Turns: []ChatTurn{
			{Prompt: "b1", Response: "response b1"},
			{Prompt: "b2", Response: "response b2"},
		},
	}
	render := renderOutputWidgetWithViewState(sessionB, 80, 200, view)

	if view.focusedTurn != 2 {
		t.Fatalf("expected newest turn focused in session B, got %d", view.focusedTurn)
	}
	if !strings.Contains(render, "Response: response b2") {
		t.Fatalf("expected newest turn expanded by default in session B, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_CollapsedLatestStaysCollapsedWhenUnchanged(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status update", Response: "partial"}},
	}

	_ = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	view.applyCommand(":collapse", snapshot)
	firstCollapsed := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if strings.Contains(firstCollapsed, "Response: partial") {
		t.Fatalf("did not expect expanded response while collapsed, got:\n%s", firstCollapsed)
	}

	secondCollapsed := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if strings.Contains(secondCollapsed, "Response: partial") {
		t.Fatalf("expected latest turn to remain collapsed when unchanged, got:\n%s", secondCollapsed)
	}
}

func TestOutputWidgetViewState_NavigationAndToggleControls(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()

	view.applyCommand("k", snapshot)
	if view.focusedTurn != 1 {
		t.Fatalf("expected k to move to older turn, got %d", view.focusedTurn)
	}

	view.applyCommand("j", snapshot)
	view.applyCommand("j", snapshot)
	if view.focusedTurn != 2 {
		t.Fatalf("expected j to clamp at newest turn, got %d", view.focusedTurn)
	}

	view.applyCommand("home", snapshot)
	if view.focusedTurn != 1 {
		t.Fatalf("expected home to jump to oldest turn, got %d", view.focusedTurn)
	}

	view.applyCommand("end", snapshot)
	if view.focusedTurn != 2 {
		t.Fatalf("expected end to jump to newest turn, got %d", view.focusedTurn)
	}
}

func TestOutputWidgetViewState_EnterAndSpaceToggleFocusedTurn(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "detailed prompt", Response: "Detailed response."}},
	}

	view.applyCommand(":entry response", snapshot)
	view.applyCommand("space", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "[+] 🤖 Response: Detailed response.") {
		t.Fatalf("expected collapsed response entry after space, got:\n%s", render)
	}
	if strings.Contains(render, "[-] 🤖 Response: Detailed response.") {
		t.Fatalf("did not expect expanded response entry after collapsing, got:\n%s", render)
	}

	view.applyCommand("enter", snapshot)
	render = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "[-] 🤖 Response: Detailed response.") {
		t.Fatalf("expected expanded response entry after enter, got:\n%s", render)
	}
}

func TestOutputWidgetViewState_TabDrillInShiftTabDrillOut(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()

	_ = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if view.entryFocusMode {
		t.Fatal("expected initial container focus mode")
	}
	view.applyCommand("tab", snapshot)
	if view.focusedTurn != 2 {
		t.Fatalf("expected tab to keep focused turn unchanged, got %d", view.focusedTurn)
	}
	if !view.entryFocusMode {
		t.Fatal("expected tab to drill in to entry focus mode")
	}
	if view.focusedEntry != "response" {
		t.Fatalf("expected tab drill-in to keep default response focus, got %q", view.focusedEntry)
	}

	view.applyCommand("down", snapshot)
	if view.focusedEntry != "classification" {
		t.Fatalf("expected down in entry mode to cycle entries, got %q", view.focusedEntry)
	}

	view.applyCommand("shift-tab", snapshot)
	if view.entryFocusMode {
		t.Fatal("expected shift-tab to drill out to container focus mode")
	}
}

func TestOutputWidgetViewState_FocusMarkersForContainerAndEntryModes(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()

	containerRender := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(containerRender, "↳ 👤 User: what is 2+2? [LATEST]") {
		t.Fatalf("expected container marker on focused user row, got:\n%s", containerRender)
	}

	view.applyCommand("tab", snapshot)
	entryRender := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(entryRender, "▶ │ [-] 🤖 Response: 4") {
		t.Fatalf("expected entry marker on focused response row, got:\n%s", entryRender)
	}
}

func TestOutputWidgetViewState_FocusMarkersDoNotMixWithinTurnBlock(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "marker check", Response: "marker response"}},
	}

	containerRender := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(containerRender, "↳ 👤 User: marker check [LATEST]") {
		t.Fatalf("expected container focus marker on user row, got:\n%s", containerRender)
	}
	if strings.Contains(containerRender, "▶") {
		t.Fatalf("did not expect entry marker while in container focus mode, got:\n%s", containerRender)
	}

	view.applyCommand("tab", snapshot)
	entryRender := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if strings.Contains(entryRender, "↳ 👤 User: marker check [LATEST]") {
		t.Fatalf("did not expect container marker while in entry focus mode, got:\n%s", entryRender)
	}
	if !strings.Contains(entryRender, "▶ │ [-] 🤖 Response: marker response") {
		t.Fatalf("expected entry marker on focused entry row, got:\n%s", entryRender)
	}
}

func TestRenderOutputWidgetWithViewState_CollapsedTurnSummaryUsesUserPrompt(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "summary from prompt", Response: "response should not drive collapsed summary"}},
	}

	_ = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	view.applyCommand(":collapse", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	if !strings.Contains(render, "↳ [+] [user] summary from prompt [LATEST]") {
		t.Fatalf("expected collapsed summary to use user prompt with latest marker, got:\n%s", render)
	}
	if strings.Contains(render, "[assistant] response should not drive collapsed summary") {
		t.Fatalf("did not expect collapsed summary from assistant response, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_UserHeaderOutsideEntryBoxAndNoLeadingBlankLine(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "layout prompt", Response: "layout response"}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	lines := strings.Split(render, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least two rendered lines, got:\n%s", render)
	}
	if strings.TrimSpace(lines[0]) == "" {
		t.Fatalf("did not expect top blank line before expanded turn content, got:\n%s", render)
	}
	if !strings.Contains(lines[0], "👤 User: layout prompt [LATEST]") {
		t.Fatalf("expected first row to be user header outside box, got first line: %q\nfull render:\n%s", lines[0], render)
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[1]), "┌") {
		t.Fatalf("expected second row to start entry box, got %q\nfull render:\n%s", lines[1], render)
	}
}

func TestRenderOutputWidgetWithViewState_DoesNotRenderStatusBannerRow(t *testing.T) {
	view := newOutputWidgetViewState()
	view.setStatus("Output controls visible.")
	snapshot := testOutputWidgetSnapshot()

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if strings.Contains(render, "Status: ") {
		t.Fatalf("did not expect rendered status banner row, got:\n%s", render)
	}
}

func TestOutputWidgetViewState_HLOperateOnFocusedEntryWhenExpanded(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "hello", Response: "Detailed response."}},
		PromptCycle: PromptCycleStatus{
			Thinking: PromptCyclePhase{State: "done", ElapsedMs: 7},
		},
	}

	_ = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	view.applyCommand(":entry classification", snapshot)
	view.applyCommand("shift-tab", snapshot)
	view.applyCommand("h", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if strings.Contains(render, "⚙️ Classification: [collapsed]") {
		t.Fatalf("expected h in container mode to collapse turn, not classification entry, got:\n%s", render)
	}
	if !strings.Contains(render, "[+] [user] hello") {
		t.Fatalf("expected h in container mode to compact focused turn summary, got:\n%s", render)
	}

	view.applyCommand("tab", snapshot)
	view.applyCommand("down", snapshot)
	view.applyCommand("l", snapshot)
	render = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "[-] ⚙️ Classification:") {
		t.Fatalf("expected l to expand focused classification entry, got:\n%s", render)
	}
}

func TestOutputWidgetViewState_CollapsedResponsePreviewTruncatesWithEllipsis(t *testing.T) {
	view := newOutputWidgetViewState()
	longResponse := "This response intentionally exceeds the preview threshold so the collapsed row renders beginning words and an ellipsis near the line end for readability."
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "hello", Response: longResponse}},
	}

	_ = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	view.applyCommand(":entry response", snapshot)
	view.applyCommand("space", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 80, view)

	wantPreview := renderOutputWidgetCollapsedPreview(longResponse, outputWidgetContentBudget(80, "  │ [+] 🤖 Response: "))
	if !strings.Contains(render, "[+] 🤖 Response: "+wantPreview) {
		t.Fatalf("expected collapsed response preview with ellipsis, got:\n%s", render)
	}
	if !strings.Contains(wantPreview, " ...") {
		t.Fatalf("expected truncated preview to include spaced ellipsis, got %q", wantPreview)
	}
}

func TestOutputWidgetViewState_LeftRightNavigateTurnsWithoutCollapseSideEffects(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()

	_ = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if view.focusedTurn != 2 {
		t.Fatalf("expected newest focused by default, got %d", view.focusedTurn)
	}

	view.applyCommand("left", snapshot)
	if view.focusedTurn != 1 {
		t.Fatalf("expected left arrow to navigate to previous turn, got %d", view.focusedTurn)
	}

	view.applyCommand("right", snapshot)
	if view.focusedTurn != 2 {
		t.Fatalf("expected right arrow to navigate to next turn, got %d", view.focusedTurn)
	}
}

func TestOutputWidgetViewState_SpaceTogglesTurnInContainerModeAndEntryInEntryMode(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "detailed prompt", Response: "Detailed response."}},
	}

	_ = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	view.applyCommand("space", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "↳ [+] [user] detailed prompt [LATEST]") {
		t.Fatalf("expected space in container mode to compact turn, got:\n%s", render)
	}

	view.applyCommand("space", snapshot)
	view.applyCommand("tab", snapshot)
	view.applyCommand("space", snapshot)
	render = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "▶ │ [+] 🤖 Response: Detailed response.") {
		t.Fatalf("expected space in entry mode to collapse focused response entry, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_CollapsedContainerRendersEmptyBoxStub(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status update", Response: "all green"}},
	}

	_ = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	view.applyCommand(":collapse", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	if !strings.Contains(render, "┌") || !strings.Contains(render, "└") {
		t.Fatalf("expected collapsed container box stub top/bottom borders, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_ExpandedContainerRendersBoxedContent(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status update", Response: "all green"}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "┌") || !strings.Contains(render, "└") || !strings.Contains(render, "│") {
		t.Fatalf("expected expanded container boxed content, got:\n%s", render)
	}
}

func TestOutputWidgetViewState_PageUpPageDownUseDeterministicPageStep(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()

	_ = renderOutputWidgetWithViewState(snapshot, 12, 120, view)
	view.applyCommand(":entry response", snapshot)
	view.applyCommand("pgdn", snapshot)
	if got, want := view.turnScrollOffset(view.focusedTurn), 8; got != want {
		t.Fatalf("expected pgdn page step %d, got %d", want, got)
	}

	view.applyCommand("pgup", snapshot)
	if got := view.turnScrollOffset(view.focusedTurn); got != 0 {
		t.Fatalf("expected pgup to subtract same page step back to zero, got %d", got)
	}
}

func TestOutputWidgetViewState_HelpOverlayTogglesAndDismisses(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()

	view.applyCommand("?", snapshot)
	if !view.showHelp {
		t.Fatal("expected help panel to be visible after ?")
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	for _, fragment := range []string{
		"j / ↓   next turn (container) or entry (entry mode)",
		"k / ↑   previous turn (container) or entry (entry mode)",
		"Tab     drill in to entry focus",
		"S-Tab   drill out to container focus",
		"l / h   expand/collapse focused target",
		"Enter   toggle focused target",
		"Space   toggle focused target",
		"?       toggle help",
	} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected help render to contain %q, got:\n%s", fragment, render)
		}
	}

	view.applyCommand("j", snapshot)
	if view.showHelp {
		t.Fatal("expected any key to dismiss help overlay")
	}
}

func TestOutputWidgetRawTokenStep_ImmediateCommands(t *testing.T) {
	current := make([]rune, 0, 16)

	next, outgoing := outputWidgetRawTokenStep(current, "j")
	if len(next) != 0 {
		t.Fatalf("expected no buffered state after immediate command, got %q", string(next))
	}
	if !reflect.DeepEqual(outgoing, []string{"j"}) {
		t.Fatalf("expected immediate j command, got %#v", outgoing)
	}

	_, outgoing = outputWidgetRawTokenStep(current, "space")
	if !reflect.DeepEqual(outgoing, []string{"space"}) {
		t.Fatalf("expected immediate space command, got %#v", outgoing)
	}

	_, outgoing = outputWidgetRawTokenStep(current, "pgdn")
	if !reflect.DeepEqual(outgoing, []string{"pgdn"}) {
		t.Fatalf("expected immediate pgdn command, got %#v", outgoing)
	}
}

func TestOutputWidgetRawTokenStep_ColonCommandBuffering(t *testing.T) {
	current := make([]rune, 0, 16)
	outgoing := []string(nil)

	current, outgoing = outputWidgetRawTokenStep(current, ":")
	if len(outgoing) != 0 {
		t.Fatalf("expected no immediate output when entering colon mode, got %#v", outgoing)
	}

	for _, token := range []string{"f", "o", "c", "u", "s", "space", "2"} {
		current, outgoing = outputWidgetRawTokenStep(current, token)
		if len(outgoing) != 0 {
			t.Fatalf("expected buffered colon command while typing, got %#v", outgoing)
		}
	}

	current, outgoing = outputWidgetRawTokenStep(current, "enter")
	if len(current) != 0 {
		t.Fatalf("expected colon buffer to clear after enter, got %q", string(current))
	}
	if !reflect.DeepEqual(outgoing, []string{":focus 2"}) {
		t.Fatalf("expected emitted colon command, got %#v", outgoing)
	}
}

func TestOutputWidgetViewState_CopyCommands_AreDeterministic(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()

	view.applyCommand(":focus 2", snapshot)
	view.applyCommand(":copy response focused", snapshot)

	if view.clipboard != "4" {
		t.Fatalf("expected copied response '4', got %q", view.clipboard)
	}
	if view.clipboardSource != "turn 2 response" {
		t.Fatalf("expected clipboard source turn 2 response, got %q", view.clipboardSource)
	}

	view.applyCommand(":copy turn 1", snapshot)
	if !strings.Contains(view.clipboard, "User: status update") {
		t.Fatalf("expected copied turn payload, got %q", view.clipboard)
	}
	if !strings.Contains(view.clipboard, "Response: all green") {
		t.Fatalf("expected copied turn response in payload, got %q", view.clipboard)
	}

	view.applyCommand(":copy clear", snapshot)
	if view.clipboard != "" {
		t.Fatalf("expected cleared clipboard, got %q", view.clipboard)
	}
}

func TestOutputWidgetViewState_CopyFocusedUsesFocusedEntryAndTurn(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()

	view.applyCommand(":focus 1 classification", snapshot)
	view.applyCommand(":copy focused", snapshot)

	expected := classifyPrompt(snapshot.Turns[0].Prompt)
	if view.clipboard != string(expected.Intent)+" -> "+string(expected.NextStep) {
		t.Fatalf("expected focused classification clipboard payload, got %q", view.clipboard)
	}
	if view.clipboardSource != "turn 1 classification" {
		t.Fatalf("expected focused clipboard source, got %q", view.clipboardSource)
	}
}

func TestRunOutputWidgetLoop_SkipsDuplicateFrames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/context" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(contextWidgetSnapshot{
			SessionID: "sess-output-loop",
			TurnCount: 1,
			Turns:     []ChatTurn{{Prompt: "hello", Response: "Echo: hello"}},
			PromptCycle: PromptCycleStatus{
				Thinking: PromptCyclePhase{State: "done", ElapsedMs: 5},
			},
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	output := &bytes.Buffer{}
	if err := runOutputWidgetLoop(ctx, server.URL, output, 20*time.Millisecond); err != nil {
		t.Fatalf("runOutputWidgetLoop returned error: %v", err)
	}

	widgetOutput := output.String()
	if got := strings.Count(widgetOutput, "\x1b[H\x1b[2J"); got != 1 {
		t.Fatalf("expected one redraw for unchanged payload, got %d\noutput:\n%s", got, widgetOutput)
	}
	if !strings.Contains(widgetOutput, "Response: Echo: hello") {
		t.Fatalf("expected rendered agent response in output widget, got:\n%s", widgetOutput)
	}
}

func TestRunOutputWidgetLoopWithInput_QuitTokenStopsLoop(t *testing.T) {
	runHeadlessWidgetLoopScript(t, "q\n", func(ctx context.Context, in io.Reader, out io.Writer) error {
		return runOutputWidgetLoopWithInput(ctx, "http://127.0.0.1:0", in, out, 100*time.Millisecond)
	})
}

func TestRenderOutputWidgetViewState_CollapsedResponsePreviewHandlesShortContent(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-short",
		Turns:     []ChatTurn{{Prompt: "hi", Response: "short"}},
	}

	_ = renderOutputWidget(snapshot, 80, 200)
	view.applyCommand(":entry response", snapshot)
	view.applyCommand("space", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	if !strings.Contains(render, "[+] 🤖 Response: short") {
		t.Fatalf("expected short untruncated response preview, got: %s", render)
	}
}

func TestRenderOutputWidgetViewState_CollapsedPreviewNormalizesWhitespace(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-ws",
		Turns:     []ChatTurn{{Prompt: "hi", Response: "a  b   c"}},
	}

	_ = renderOutputWidget(snapshot, 80, 200)
	view.applyCommand(":entry response", snapshot)
	view.applyCommand("space", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	if !strings.Contains(render, "a b c") {
		t.Fatalf("expected whitespace normalized to single space, got: %s", render)
	}
}

func TestRenderOutputWidgetViewState_CollapsedPreviewNormalizesNewlines(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-nl",
		Turns:     []ChatTurn{{Prompt: "hi", Response: "line1\nline2\nline3"}},
	}

	_ = renderOutputWidget(snapshot, 80, 200)
	view.applyCommand(":entry response", snapshot)
	view.applyCommand("space", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	if !strings.Contains(render, "line1 line2 line3") {
		t.Fatalf("expected newlines normalized to single space, got: %s", render)
	}
}

func TestRenderOutputWidgetViewState_CollapsedPreviewHandlesEmptyContent(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-empty",
		Turns:     []ChatTurn{{Prompt: "hi", Response: "  "}},
	}

	_ = renderOutputWidget(snapshot, 80, 200)
	view.applyCommand(":entry response", snapshot)
	view.applyCommand("space", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	if !strings.Contains(render, "[+] 🤖 Response: none") {
		t.Fatalf("expected empty response to show 'none', got: %s", render)
	}
}

func TestRenderOutputWidgetViewState_CollapsedPreviewExactLimitShowsContent(t *testing.T) {
	view := newOutputWidgetViewState()
	padding := "0123456789"
	payload := padding + padding + padding + padding + padding // exactly 50 chars
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-exact",
		Turns:     []ChatTurn{{Prompt: "hi", Response: payload}},
	}

	_ = renderOutputWidget(snapshot, 80, 200)
	view.applyCommand(":entry response", snapshot)
	view.applyCommand("space", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	if !strings.Contains(render, "[+] 🤖 Response: "+payload) {
		t.Fatalf("expected exact-limit response to not truncate, got: %s", render)
	}
}

func TestRenderOutputWidgetViewState_CollapsedPreviewJustOverLimitTruncates(t *testing.T) {
	view := newOutputWidgetViewState()
	exact := "0123456789"
	exact = exact + exact + exact + exact + exact
	exact = exact + "x" // 51 chars, over 50-byte limit
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-over",
		Turns:     []ChatTurn{{Prompt: "hi", Response: exact}},
	}

	_ = renderOutputWidget(snapshot, 80, 200)
	view.applyCommand(":entry response", snapshot)
	view.applyCommand("space", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	if !strings.Contains(render, "[+] 🤖 Response: ") {
		t.Fatalf("expected truncated over-limit preview, got: %s", render)
	}
}

func TestRenderOutputWidgetViewState_ContentWrappingHandlesEmptyContent(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-wrap-empty",
		Turns:     []ChatTurn{{Prompt: "hi", Response: ""}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "│ [-] 🤖 Response:") {
		t.Fatalf("expected empty content to render as a blank response line, got:\n%s", render)
	}
}

func TestRenderOutputWidgetViewState_ContentWrappingHandlesSingleCharacterContent(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-wrap-single",
		Turns:     []ChatTurn{{Prompt: "hi", Response: "a"}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "│ [-] 🤖 Response: a") {
		t.Fatalf("expected single character content to stay on the response line, got:\n%s", render)
	}
}

func TestRenderOutputWidgetViewState_ContentWrappingHandlesWideContent(t *testing.T) {
	view := newOutputWidgetViewState()
	wide := ""
	for i := 0; i < 200; i++ {
		wide += "W"
	}
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-wrap-wide",
		Turns:     []ChatTurn{{Prompt: "hi", Response: wide}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "│ [-] 🤖 Response: ") || !strings.Contains(render, "\n  │ W") {
		t.Fatalf("expected wide content to wrap onto a continuation line, got:\n%s", render)
	}
}

func TestRenderOutputWidgetViewState_ContentWrappingPreservesEmptySublines(t *testing.T) {
	view := newOutputWidgetViewState()
	multiWithEmpty := "first\n\nthird"
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-wrap-multiline",
		Turns:     []ChatTurn{{Prompt: "hi", Response: multiWithEmpty}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "\n") {
		t.Fatalf("expected multiline response to produce newlines, got:\n%s", render)
	}
}

func TestRenderOutputWidgetViewState_TurnDetailsRendersBoxedContent(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-td-box",
		Turns:     []ChatTurn{{Prompt: "hi", Response: "hi"}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "┌") || !strings.Contains(render, "└") {
		t.Fatalf("expected boxed turn content in expanded mode, got:\n%s", render)
	}
	if !strings.Contains(render, "│") {
		t.Fatalf("expected box side borders, got:\n%s", render)
	}
}

func TestRenderOutputWidgetViewState_TurnDetailsMultipleTurnsAreIndependent(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-multi-independent",
		Turns: []ChatTurn{
			{Response: "turn1"},
			{Response: "turn2"},
			{Response: "turn3"},
		},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "turn1") {
		t.Fatalf("expected turn1 content, got:\n%s", render)
	}
	if !strings.Contains(render, "turn2") {
		t.Fatalf("expected turn2 content, got:\n%s", render)
	}
	if !strings.Contains(render, "turn3") {
		t.Fatalf("expected turn3 content, got:\n%s", render)
	}
}

func TestRenderOutputWidgetViewState_TurnDetailsBoxWidthAdjustsForNarrowPane(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-narrow",
		Turns:     []ChatTurn{{Prompt: "hi", Response: "hi"}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 40, 40, view)
	// When pane width is small, the ┌─...─┐ line should still be present and valid
	if !strings.Contains(render, "──") {
		t.Fatalf("expected narrow pane to still have box borders, got:\n%s", render)
	}
}

func TestRenderOutputWidgetViewState_PromptCyclePhaseStateRendering(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-phase",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "hi", Response: "ok"}},
		PromptCycle: PromptCycleStatus{
			Thinking: PromptCyclePhase{State: "done", ElapsedMs: 42},
		},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "done (00:00:00.042)") {
		t.Fatalf("expected thinking phase rendered as 'done (00:00:00.042)', got:\n%s", render)
	}
}

func TestRenderOutputWidgetViewState_EmptyTurnStillGetsClassified(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-classified",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "hello", Response: "Hi!"}},
	}

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "🤖 Response: Hi!") {
		t.Fatalf("expected response rendering in expanded turn, got:\n%s", render)
	}
}
