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
		{name: "top token", raw: "top", want: ":focus 1"},
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

func TestRenderOutputWidgetWithViewState_CollapsesThinkingBlock(t *testing.T) {
	view := newOutputWidgetViewState()
	view.applyCommand(":collapse 1", outputWidgetSnapshot{Turns: []ChatTurn{{Prompt: "status?", Response: "all green"}}})

	render := renderOutputWidgetWithViewState(outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "status?", Response: "all green"}},
		PromptCycle: PromptCycleStatus{
			Thinking: PromptCyclePhase{State: "done", ElapsedMs: 11},
		},
	}, 80, 200, view)

	if !strings.Contains(render, "💭 [thinking block - collapsed]") {
		t.Fatalf("expected collapsed thinking block marker, got:\n%s", render)
	}
	if strings.Contains(render, "💭 [thinking block - done (00:00:00.011)]") {
		t.Fatalf("did not expect expanded thinking marker after collapse, got:\n%s", render)
	}
}

func TestOutputWidgetViewState_HelpAndFocusCommands(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()
	view.applyCommand(":help", snapshot)
	if !view.showHelp {
		t.Fatal("expected help panel to be visible after :help")
	}

	view.applyCommand(":focus 1", snapshot)
	if view.focusedTurn != 1 {
		t.Fatalf("expected focused turn 1, got %d", view.focusedTurn)
	}

	view.applyCommand(":next", snapshot)
	if view.focusedTurn != 2 {
		t.Fatalf("expected focused turn 2 after :next, got %d", view.focusedTurn)
	}
}

func TestRenderOutputWidgetWithViewState_CollapsesClassificationAndResponseEntries(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()

	view.applyCommand(":collapse classification 1", snapshot)
	view.applyCommand(":collapse response 1", snapshot)
	view.applyCommand(":focus 1 response", snapshot)

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)

	if !strings.Contains(render, "⚙️ [classification entry - collapsed]") {
		t.Fatalf("expected collapsed classification marker, got:\n%s", render)
	}
	if !strings.Contains(render, "🤖 [response entry - collapsed]") {
		t.Fatalf("expected collapsed response marker, got:\n%s", render)
	}
	if strings.Contains(render, "Response: all green") {
		t.Fatalf("did not expect response line after collapsing response entry, got:\n%s", render)
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

func TestRenderOutputWidgetWithViewState_HelpIncludesCopyAndEntryCommands(t *testing.T) {
	view := newOutputWidgetViewState()
	snapshot := testOutputWidgetSnapshot()
	view.applyCommand(":help", snapshot)

	render := renderOutputWidgetWithViewState(snapshot, 80, 200, view)
	for _, fragment := range []string{
		":entry <entry>",
		":collapse [entry] <n|all>",
		":copy <entry|turn>",
	} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected help render to contain %q, got:\n%s", fragment, render)
		}
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
