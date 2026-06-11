package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
		{name: "collapse token", raw: "h", want: ":collapse"},
		{name: "expand token", raw: "l", want: ":expand"},
		{name: "toggle token", raw: "space", want: ":toggle"},
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
		"[OUTPUT]",
		"Chat ready.",
		"User: what is 2+2?",
		"⚙️ Classification:",
		"💭 [thinking block - done (00:00:00.011)]",
		"Response: Echo: what is 2+2?",
	} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
}

func TestRenderOutputWidgetWithViewState_LatestFirstFocusExpandsNewest(t *testing.T) {
	view := newOutputWidgetViewState()
	render := renderOutputWidgetWithViewState(testOutputWidgetSnapshot(), 80, 200, view)

	if view.focusedTurn != 2 {
		t.Fatalf("expected newest turn focused by default, got %d", view.focusedTurn)
	}
	for _, fragment := range []string{
		"[assistant] all green",
		"Response: 4",
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

	if !strings.Contains(render, "* [assistant] all green") {
		t.Fatalf("expected compacted focused turn pointer, got:\n%s", render)
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
	if strings.Contains(render, "* [assistant] all green") {
		t.Fatalf("did not expect non-focused compact turn pointer, got:\n%s", render)
	}
	if !strings.Contains(render, "* [assistant] 4") {
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
	if !strings.Contains(collapsedRender, "* [assistant] partial") {
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

	view.applyCommand("space", snapshot)
	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "[assistant] Detailed response.") {
		t.Fatalf("expected compact summary after space, got:\n%s", render)
	}
	if strings.Contains(render, "Response: Detailed response.") {
		t.Fatalf("did not expect full turn content after collapsing, got:\n%s", render)
	}

	view.applyCommand("enter", snapshot)
	render = renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	if !strings.Contains(render, "Response: Detailed response.") {
		t.Fatalf("expected full turn content after enter, got:\n%s", render)
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
		"j / ↓   next turn",
		"k / ↑   previous turn",
		"Enter   expand/toggle focused turn",
		"Space   collapse/toggle focused turn",
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
