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

func TestRenderContextFeedbackSections_DefaultCollapsedOrderAndMinimalHeader(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{
		SessionID: "sess-order",
		Turns:     []ChatTurn{{Prompt: "p1", Response: "r1"}},
	}
	history := []contextHistorySession{{SessionID: "s-prev", Turns: []ChatTurn{{Prompt: "old", Response: "resp"}}}}

	rendered := strings.Join(renderContextFeedbackSections(snapshot, history, state), "\n")

	historyIdx := strings.Index(rendered, "CONTEXT HISTORY")
	wmIdx := strings.Index(rendered, "WORKING MEMORY")
	currentIdx := strings.Index(rendered, "CURRENT CONTEXT")
	if historyIdx == -1 || wmIdx == -1 || currentIdx == -1 {
		t.Fatalf("expected all context-feedback section titles, got:\n%s", rendered)
	}
	if !(historyIdx < wmIdx && wmIdx < currentIdx) {
		t.Fatalf("expected section order history -> working memory -> current context, got:\n%s", rendered)
	}

	if strings.Contains(rendered, "Controls: type help") || strings.Contains(rendered, "Status:") {
		t.Fatalf("expected clutter header lines removed, got:\n%s", rendered)
	}

	wmBlock := rendered[wmIdx:currentIdx]
	if strings.Contains(wmBlock, "fact_count:") || strings.Contains(wmBlock, "session_id:") {
		t.Fatalf("expected collapsed working memory to render title only by default, got:\n%s", wmBlock)
	}

	if !strings.Contains(rendered, "prompt: \x1b[33m[collapsed]\x1b[0m") || !strings.Contains(rendered, "response: \x1b[33m[collapsed]\x1b[0m") {
		t.Fatalf("expected current-context prompt/response tiles collapsed by default, got:\n%s", rendered)
	}
}

func TestNormalizeContextWidgetControlCommand(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "colon help", raw: ":help", want: "help"},
		{name: "question alias", raw: ":?", want: "help"},
		{name: "colon quit", raw: ":q", want: "q"},
		{name: "exit alias", raw: ":exit", want: "quit"},
		{name: "refresh alias", raw: ":refresh", want: "r"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeContextWidgetControlCommand(tc.raw); got != tc.want {
				t.Fatalf("normalizeContextWidgetControlCommand(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestRenderContextWidget_ClipsToViewport(t *testing.T) {
	snapshot := contextWidgetSnapshot{
		SessionID: "sess-1",
		TurnCount: 3,
		Turns: []ChatTurn{
			{Prompt: "first prompt", Response: "first response"},
			{Prompt: "second prompt", Response: "second response"},
			{Prompt: "third prompt", Response: "third response"},
		},
		PromptCycle: PromptCycleStatus{
			Classify: PromptCyclePhase{State: "done", ElapsedMs: 2},
			Thinking: PromptCyclePhase{State: "done", ElapsedMs: 10},
			Tool:     PromptCyclePhase{State: "done", ElapsedMs: 1},
			Respond:  PromptCyclePhase{State: "done", ElapsedMs: 1},
		},
	}

	render := renderContextWidget(snapshot, "context-visualizer", "qwen3.6:latest", "ollama", 8, 55)
	lines := strings.Split(render, "\n")
	if len(lines) > 8 {
		t.Fatalf("expected at most 8 lines, got %d\n%s", len(lines), render)
	}
	for _, line := range lines {
		if len([]rune(line)) > 55 {
			t.Fatalf("expected line width <= 55, got %d for %q", len([]rune(line)), line)
		}
	}
	if !strings.Contains(render, "... (") {
		t.Fatalf("expected truncation marker in clipped render, got:\n%s", render)
	}
}

func TestRunContextWidgetLoop_SkipsDuplicateFrames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/context" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(contextWidgetSnapshot{
			SessionID: "sess-1",
			TurnCount: 1,
			Turns:     []ChatTurn{{Prompt: "hello", Response: "world"}},
			PromptCycle: PromptCycleStatus{
				Classify: PromptCyclePhase{State: "done", ElapsedMs: 2},
				Thinking: PromptCyclePhase{State: "done", ElapsedMs: 5},
				Tool:     PromptCyclePhase{State: "done", ElapsedMs: 1},
				Respond:  PromptCyclePhase{State: "done", ElapsedMs: 1},
			},
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	output := &bytes.Buffer{}
	if err := runContextWidgetLoop(ctx, server.URL, output, 20*time.Millisecond); err != nil {
		t.Fatalf("runContextWidgetLoop returned error: %v", err)
	}

	widgetOutput := output.String()
	if got := strings.Count(widgetOutput, "\x1b[H\x1b[2J"); got != 1 {
		t.Fatalf("expected one redraw for unchanged payload, got %d\noutput:\n%s", got, widgetOutput)
	}
}

func TestRunContextWidgetLoopWithInput_QuitTokenStopsLoop(t *testing.T) {
	runHeadlessWidgetLoopScript(t, "q\n", func(ctx context.Context, in io.Reader, out io.Writer) error {
		return runContextWidgetLoopWithInput(ctx, "http://127.0.0.1:0", in, out, 100*time.Millisecond)
	})
}

func TestFetchContextWidgetSnapshot_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/context" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(contextWidgetSnapshot{
			SessionID: "sess-2",
			TurnCount: 1,
			Turns:     []ChatTurn{{Prompt: "p", Response: "r"}},
		})
	}))
	defer server.Close()

	snapshot, err := fetchContextWidgetSnapshot(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetchContextWidgetSnapshot returned error: %v", err)
	}
	if snapshot.SessionID != "sess-2" {
		t.Fatalf("expected session_id sess-2, got %q", snapshot.SessionID)
	}
	if snapshot.TurnCount != 1 {
		t.Fatalf("expected turn_count 1, got %d", snapshot.TurnCount)
	}
}

func TestRenderContextWidget_UsesTotalCapacityPercentages(t *testing.T) {
	snapshot := contextWidgetSnapshot{
		SessionID: "sess-capacity",
		TurnCount: 2,
		Turns: []ChatTurn{
			{Prompt: "what is 5 plus 6", Response: "11"},
			{Prompt: "identify yourself", Response: "I am Agent X"},
		},
		PromptCycle: PromptCycleStatus{
			Classify: PromptCyclePhase{State: "done", ElapsedMs: 2},
			Thinking: PromptCyclePhase{State: "done", ElapsedMs: 5},
			Tool:     PromptCyclePhase{State: "done", ElapsedMs: 1},
			Respond:  PromptCyclePhase{State: "done", ElapsedMs: 1},
		},
	}

	render := renderContextWidget(snapshot, "context-visualizer", "qwen3.6:latest", "ollama", 80, 200)

	if !strings.Contains(render, "consumed: 0.") {
		t.Fatalf("expected consumed percentage to be rendered with decimal precision against total capacity, got:\n%s", render)
	}
	if !strings.Contains(render, "👤 User Prompts") || !strings.Contains(render, "🤖 Agent Response") {
		t.Fatalf("expected user/assistant rows in render, got:\n%s", render)
	}
	if strings.Contains(render, "👤 User Prompts        [################") {
		t.Fatalf("expected user prompts bar to reflect total-capacity ratio (not full bar), got:\n%s", render)
	}
	if !strings.Contains(render, "░ Remaining") {
		t.Fatalf("expected remaining row in render, got:\n%s", render)
	}
}

func TestRenderContextWidget_ContextHistoryUsesSystemAppletHost(t *testing.T) {
	snapshot := contextWidgetSnapshot{
		SessionID: "sess-history",
		TurnCount: 3,
		Turns: []ChatTurn{
			{Prompt: "first prompt", Response: "first response"},
			{Prompt: "second prompt", Response: "second response"},
			{Prompt: "third prompt", Response: "third response"},
		},
	}

	render := renderContextWidget(snapshot, "context-history", "qwen3.6:latest", "ollama", 80, 200)
	for _, fragment := range []string{"== CONTEXT HISTORY ==", "history_context_count: 3", "recent_prompt: second prompt", "recent_response: second response"} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
}

func TestRenderContextWidget_FilesTabContract(t *testing.T) {
	projectDir := createWidgetTestProjectDir(t, []string{"alpha.txt"}, []string{"beta"})
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
	})

	render := renderContextWidget(contextWidgetSnapshot{}, "files", "qwen3.6:latest", "ollama", 80, 200)
	for _, fragment := range []string{"== FILES ==", "project_dir:", "entry_count: 2"} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
	if strings.Contains(render, "preview:") {
		t.Fatalf("files widget tab should remain compact (no preview block), got:\n%s", render)
	}
}

func TestRenderContextWidget_ConfigurationTabContract(t *testing.T) {
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_OLLAMA_HOST": "localhost:11434",
	})

	render := renderContextWidget(contextWidgetSnapshot{}, "configuration", "qwen3.6:latest", "ollama", 80, 200)
	for _, fragment := range []string{"== CONFIGURATION ==", "model:", "backend:", "ollama_host:"} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
	if strings.Contains(render, "preview:") {
		t.Fatalf("configuration widget tab should not include file preview rows, got:\n%s", render)
	}
}

func TestResolveContextWidgetTab_UsesEnvironmentOverride(t *testing.T) {
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR":        t.TempDir(),
		"AGENTX_CONTEXT_WIDGET_TAB": "files",
	})

	if got := resolveContextWidgetTab(); got != "files" {
		t.Fatalf("expected files tab from environment override, got %q", got)
	}
}

func TestContextWidgetCommandAliases_HotkeyCollapseWithoutColon(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{
		SessionID: "sess-hotkeys",
		Turns: []ChatTurn{
			{Prompt: "p1", Response: "r1"},
		},
	}

	applyContextWidgetCommand(state, "c 1 p", "http://127.0.0.1:0", snapshot)

	if !state.collapsedEntries[contextEntryKey("current", 1, "prompt")] {
		t.Fatalf("expected prompt entry to be collapsed via hotkey alias")
	}
}

func TestContextWidgetCommandAliases_HelpToggleWithoutColon(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "sess-hotkeys"}

	applyContextWidgetCommand(state, "?", "http://127.0.0.1:0", snapshot)
	if !state.showHelp {
		t.Fatalf("expected help to be shown via '?' alias")
	}

	applyContextWidgetCommand(state, "hide-help", "http://127.0.0.1:0", snapshot)
	if state.showHelp {
		t.Fatalf("expected help to be hidden via hide-help command")
	}
}

func TestContextWidgetCommandAliases_WorkingMemoryToggleWithoutColon(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "sess-hotkeys"}

	applyContextWidgetCommand(state, "m", "http://127.0.0.1:0", snapshot)
	if !state.collapsedWorkingMemory {
		t.Fatalf("expected working memory to toggle collapsed with 'm'")
	}

	applyContextWidgetCommand(state, "m show", "http://127.0.0.1:0", snapshot)
	if !state.showWorkingMemory || state.collapsedWorkingMemory {
		t.Fatalf("expected working memory section visible and expanded after 'm show'")
	}
}

func TestContextWidgetKeyboard_SpaceSelectAndEnterCollapse(t *testing.T) {
	state := newContextFeedbackViewState()
	state.updateOrderedRows([]string{"current:1:prompt"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys", Turns: []ChatTurn{{Prompt: "p1", Response: "r1"}}}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot)
	if !state.selectedEntries["current:1:prompt"] {
		t.Fatalf("expected row to be selected by space key")
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot)
	if !state.collapsedEntries["current:1:prompt"] {
		t.Fatalf("expected row to be collapsed by enter key")
	}
}

func TestContextWidgetKeyboard_TabAndScrollTextBox(t *testing.T) {
	state := newContextFeedbackViewState()
	state.updateOrderedRows([]string{"current:1:prompt"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys", Turns: []ChatTurn{{Prompt: "one two three four five six seven eight nine ten", Response: "ok"}}}

	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot)
	if !state.focusTextBox {
		t.Fatalf("expected tab to enter textbox focus mode")
	}

	applyContextWidgetCommand(state, "pgdn", "http://127.0.0.1:0", snapshot)
	if state.textScroll["current:1:prompt"] == 0 {
		t.Fatalf("expected pgdn to scroll focused textbox")
	}

	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot)
	if state.focusTextBox {
		t.Fatalf("expected second tab to exit textbox focus mode")
	}
}

func TestContextWidgetKeyboard_LeftRightCurrentTurnSibling(t *testing.T) {
	state := newContextFeedbackViewState()
	state.updateOrderedRows([]string{"current:1:prompt", "current:1:response"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys", Turns: []ChatTurn{{Prompt: "p1", Response: "r1"}}}

	if state.activeRowKey() != "current:1:prompt" {
		t.Fatalf("expected initial active row to be prompt")
	}

	applyContextWidgetCommand(state, "right", "http://127.0.0.1:0", snapshot)
	if state.activeRowKey() != "current:1:response" {
		t.Fatalf("expected right to move to response sibling, got %q", state.activeRowKey())
	}

	applyContextWidgetCommand(state, "left", "http://127.0.0.1:0", snapshot)
	if state.activeRowKey() != "current:1:prompt" {
		t.Fatalf("expected left to move back to prompt sibling, got %q", state.activeRowKey())
	}
}

func TestContextWidgetKeyboard_LeftRightHistorySessionAndTurn(t *testing.T) {
	state := newContextFeedbackViewState()
	state.updateOrderedRows([]string{"session:s-prev", "history:s-prev:1"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	if state.activeRowKey() != "session:s-prev" {
		t.Fatalf("expected initial active row to be session row")
	}

	applyContextWidgetCommand(state, "right", "http://127.0.0.1:0", snapshot)
	if state.activeRowKey() != "history:s-prev:1" {
		t.Fatalf("expected right to enter first session turn, got %q", state.activeRowKey())
	}

	applyContextWidgetCommand(state, "left", "http://127.0.0.1:0", snapshot)
	if state.activeRowKey() != "session:s-prev" {
		t.Fatalf("expected left to return to session row, got %q", state.activeRowKey())
	}
}
