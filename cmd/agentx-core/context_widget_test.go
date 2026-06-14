package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
)

func TestRenderContextFeedbackSections_DefaultCollapsedOrderAndMinimalHeader(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{
		SessionID: "sess-order",
		Turns:     []ChatTurn{{Prompt: "p1", Response: "r1"}},
	}
	history := []contextHistorySession{{SessionID: "s-prev", Turns: []ChatTurn{{Prompt: "old", Response: "resp"}}}}

	rendered := strings.Join(renderContextFeedbackSections(snapshot, history, state, 120), "\n")

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
	// Collapsed sections should NOT show freeform summary lines.
	if strings.Contains(rendered, "users ") || strings.Contains(rendered, "sessions ") {
		t.Fatalf("expected no freeform summary line in collapsed context-history, got:\n%s", rendered)
	}
	if strings.Contains(wmBlock, "facts ") {
		t.Fatalf("expected no freeform summary line in collapsed working-memory, got:\n%s", wmBlock)
	}
	// Current context should render items with emoji state icons, not legacy [collapsed]/[enabled] text.
	if strings.Contains(rendered, "[collapsed]") || strings.Contains(rendered, "[enabled]") {
		t.Fatalf("expected no legacy [collapsed]/[enabled] text in current-context, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "👤") || !strings.Contains(rendered, "🤖") {
		t.Fatalf("expected prompt/response emoji icons in current-context, got:\n%s", rendered)
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

func TestNormalizeContextWidgetCommand_PreservesLiteralKeysAndBackspace(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "backspace token", raw: "backspace", want: "backspace"},
		{name: "literal j remains j", raw: "j", want: "j"},
		{name: "literal h remains h", raw: "h", want: "h"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeContextWidgetCommand(tc.raw); got != tc.want {
				t.Fatalf("normalizeContextWidgetCommand(%q) = %q, want %q", tc.raw, got, tc.want)
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
		if got := runewidth.StringWidth(stripAnsi(line)); got > 55 {
			t.Fatalf("expected display width <= 55, got %d for %q", got, line)
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

	applyContextWidgetCommand(state, "c 1 p", "http://127.0.0.1:0", snapshot, "", "")

	if !state.collapsedEntries[contextEntryKey("current", 1, "prompt")] {
		t.Fatalf("expected prompt entry to be collapsed via hotkey alias")
	}
}

func TestContextWidgetCommandAliases_HelpToggleWithoutColon(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "sess-hotkeys"}

	applyContextWidgetCommand(state, "?", "http://127.0.0.1:0", snapshot, "", "")
	if !state.showHelp {
		t.Fatalf("expected help to be shown via '?' alias")
	}

	applyContextWidgetCommand(state, "hide-help", "http://127.0.0.1:0", snapshot, "", "")
	if state.showHelp {
		t.Fatalf("expected help to be hidden via hide-help command")
	}
}

func TestContextWidgetCommandAliases_ColonPrefixedHelpOutsideEditor(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "sess-hotkeys"}

	applyContextWidgetCommand(state, ":help", "http://127.0.0.1:0", snapshot, "", "")
	if !state.showHelp {
		t.Fatalf("expected colon-prefixed help to work outside the working-memory editor")
	}
}

func TestContextWidgetCommandAliases_WorkingMemoryToggleWithoutColon(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "sess-hotkeys"}
	initialCollapsed := state.collapsedWorkingMemory

	applyContextWidgetCommand(state, "m", "http://127.0.0.1:0", snapshot, "", "")
	if state.collapsedWorkingMemory == initialCollapsed {
		t.Fatalf("expected working memory collapse state to toggle with 'm'")
	}

	applyContextWidgetCommand(state, "m show", "http://127.0.0.1:0", snapshot, "", "")
	if !state.showWorkingMemory || state.collapsedWorkingMemory {
		t.Fatalf("expected working memory section visible and expanded after 'm show'")
	}
}

func TestContextWidgetKeyboard_SpaceAndEnterActionOwnership(t *testing.T) {
	state := newContextFeedbackViewState()
	state.insideSection = true
	state.activeSection = "current-context"
	state.updateOrderedRows([]string{"current:1:prompt"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys", Turns: []ChatTurn{{Prompt: "p1", Response: "r1"}}}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")
	if !state.collapsedEntries["current:1:prompt"] {
		t.Fatalf("expected space to toggle current-context row collapse")
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	if !state.disabledEntries["current:1:prompt"] {
		t.Fatalf("expected enter to toggle current-context row enabled/disabled state")
	}
}

// TestContextWidgetKeyboard_TabSectionToggle verifies that TAB drills into a
// section and remains inside until Shift-Tab exits.
func TestContextWidgetKeyboard_TabSectionToggle(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	if state.insideSection {
		t.Fatalf("expected insideSection to be false initially")
	}
	if state.activeSection != "current-context" {
		t.Fatalf("expected default activeSection to be current-context, got %q", state.activeSection)
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "current-context" {
		t.Fatalf("expected startup contract to point at current-context header while outside sections, got %#v", got)
	}

	// First TAB enters the active section.
	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	if !state.insideSection {
		t.Fatalf("expected first tab to enter section (insideSection=true)")
	}
	if got := state.statusLine; got != "Entered section: current-context." {
		t.Fatalf("expected normalized tab enter status, got %q", got)
	}

	// Second TAB remains inside the section (drill-in semantics).
	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	if !state.insideSection {
		t.Fatalf("expected second tab to keep section focus (insideSection=true)")
	}
	if got := state.statusLine; got != "Entered section: current-context." {
		t.Fatalf("expected normalized repeated tab status, got %q", got)
	}

	// Shift-Tab exits the section.
	applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot, "", "")
	if state.insideSection {
		t.Fatalf("expected shift-tab to exit section (insideSection=false)")
	}
	if got := state.statusLine; got != "Exited section: current-context." {
		t.Fatalf("expected normalized shift-tab status, got %q", got)
	}
}

func TestContextWidgetKeyboard_StartupHeaderPointerTabIntoContextHistoryFocusesFirstUser(t *testing.T) {
	projectDir := t.TempDir()
	user := "alpha-user"
	sessionID := "2026-06-09 10:30:00"
	turnPath := filepath.Join(projectDir, "sessions", user, sessionID, "context", "turns.jsonl")
	if err := os.MkdirAll(filepath.Dir(turnPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	payload, err := json.Marshal(ChatTurn{Prompt: "hello", Response: "world", CreatedAt: 1_700_000_000_000})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := os.WriteFile(turnPath, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "current-user",
	})

	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "current-session", Turns: []ChatTurn{{Prompt: "current", Response: "turn"}}}

	if state.insideSection {
		t.Fatalf("precondition: startup should be outside sections")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "current-context" {
		t.Fatalf("precondition: startup should point at current-context header, got %#v", got)
	}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeSection; got != "context-history" {
		t.Fatalf("expected pointer to move to context-history header, got %q", got)
	}

	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	if !state.insideSection {
		t.Fatalf("expected TAB from context-history header to enter section")
	}
	if state.collapsedContextHistory {
		t.Fatalf("expected TAB to expand context-history section")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != user {
		t.Fatalf("expected TAB to focus first context-history user container, got %#v", got)
	}

	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if got := state.activeRowKey(); got != "user:"+user {
		t.Fatalf("expected render cycle to map focused user container to active row, got %q", got)
	}
}

func TestContextWidgetKeyboard_ShiftTabExitThenHeaderTabReentersContextHistoryFirstUser(t *testing.T) {
	projectDir := t.TempDir()
	user := "alpha-user"
	sessionID := "2026-06-09 10:30:00"
	turnPath := filepath.Join(projectDir, "sessions", user, sessionID, "context", "turns.jsonl")
	if err := os.MkdirAll(filepath.Dir(turnPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	payload, err := json.Marshal(ChatTurn{Prompt: "hello", Response: "world", CreatedAt: 1_700_000_000_000})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := os.WriteFile(turnPath, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "current-user",
	})

	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "current-session", Turns: []ChatTurn{{Prompt: "current", Response: "turn"}}}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if got := state.activeRowKey(); got != "user:"+user {
		t.Fatalf("expected first TAB to place focus on first user row, got %q", got)
	}

	applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot, "", "")
	if state.insideSection {
		t.Fatalf("expected shift-tab from user to exit context-history section")
	}
	if got := state.activeSection; got != "context-history" {
		t.Fatalf("expected section pointer to remain on context-history header after exit, got %q", got)
	}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeSection; got != "current-context" {
		t.Fatalf("expected pointer to move away to current-context, got %q", got)
	}
	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeSection; got != "context-history" {
		t.Fatalf("expected pointer to return to context-history header, got %q", got)
	}

	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	if !state.insideSection {
		t.Fatalf("expected re-entry TAB to enter context-history")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != user {
		t.Fatalf("expected re-entry TAB to focus first user container, got %#v", got)
	}

	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if got := state.activeRowKey(); got != "user:"+user {
		t.Fatalf("expected re-entry render cycle to keep first user row active, got %q", got)
	}
}

func TestContextWidgetKeyboard_TabFromContextHistoryHeaderWithNoRows_StaysOutsideAndReportsNoRows(t *testing.T) {
	projectDir := t.TempDir()
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "current-user",
	})

	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "current-session", Turns: []ChatTurn{{Prompt: "current", Response: "turn"}}}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeSection; got != "context-history" {
		t.Fatalf("expected pointer to move to context-history header, got %q", got)
	}

	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if state.insideSection {
		t.Fatalf("precondition: expected startup pointer mode outside section")
	}
	if got := state.activeSection; got != "context-history" {
		t.Fatalf("precondition: expected active section pointer on context-history header, got %q", got)
	}

	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	if state.insideSection {
		t.Fatalf("expected TAB with empty history to remain outside section mode")
	}
	if got := state.statusLine; got != "No context-history rows." {
		t.Fatalf("expected no-rows TAB status, got %q", got)
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "context-history" {
		t.Fatalf("expected TAB no-rows path to remain on context-history header, got %#v", got)
	}

	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if state.insideSection {
		t.Fatalf("expected post-render no-rows invariant to remain outside section mode")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "context-history" {
		t.Fatalf("expected post-render no-rows invariant to preserve section header focus, got %#v", got)
	}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeSection; got != "working-memory" {
		t.Fatalf("expected down after no-rows TAB to navigate section headers, got %q", got)
	}
	if got := state.statusLine; got != "Section: working-memory" {
		t.Fatalf("expected outside-mode section-navigation status, got %q", got)
	}
}

func TestContextWidgetKeyboard_StartupHeaderPointerTabIntoContextHistoryFocusesAlphabeticalFirstUserAndReentry(t *testing.T) {
	projectDir := t.TempDir()
	writeHistoryTurn := func(user string, sessionID string, createdAt int64) {
		t.Helper()
		turnPath := filepath.Join(projectDir, "sessions", user, sessionID, "context", "turns.jsonl")
		if err := os.MkdirAll(filepath.Dir(turnPath), 0o755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}
		payload, err := json.Marshal(ChatTurn{Prompt: "hello", Response: "world", CreatedAt: createdAt})
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if err := os.WriteFile(turnPath, append(payload, '\n'), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}

	writeHistoryTurn("zulu-user", "2026-06-09 10:35:00", 1_700_000_001_000)
	writeHistoryTurn("alpha-user", "2026-06-09 10:30:00", 1_700_000_000_000)

	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "current-user",
	})

	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "current-session", Turns: []ChatTurn{{Prompt: "current", Response: "turn"}}}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeSection; got != "context-history" {
		t.Fatalf("expected pointer to move to context-history header, got %q", got)
	}

	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	if !state.insideSection {
		t.Fatalf("expected TAB from context-history header to enter section")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != "alpha-user" {
		t.Fatalf("expected first TAB to focus alphabetical first user, got %#v", got)
	}
	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if got := state.activeRowKey(); got != "user:alpha-user" {
		t.Fatalf("expected render cycle to map focus to alphabetical first user row, got %q", got)
	}

	applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot, "", "")
	if state.insideSection {
		t.Fatalf("expected shift-tab from user node to exit context-history section")
	}
	if got := state.activeSection; got != "context-history" {
		t.Fatalf("expected section pointer to remain on context-history header, got %q", got)
	}

	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	if !state.insideSection {
		t.Fatalf("expected re-entry TAB to enter context-history")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != "alpha-user" {
		t.Fatalf("expected re-entry TAB to focus same alphabetical first user, got %#v", got)
	}
	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if got := state.activeRowKey(); got != "user:alpha-user" {
		t.Fatalf("expected re-entry render cycle to keep alphabetical first user row active, got %q", got)
	}
}

// TestContextWidgetKeyboard_SpaceCollapsesSection verifies that SPACE when
// outside a section toggles the section's collapsed state.
func TestContextWidgetKeyboard_SpaceCollapsesSection(t *testing.T) {
	state := newContextFeedbackViewState()
	// History starts collapsed; SPACE should expand it.
	state.activeSection = "context-history"
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	initiallyCollapsed := state.collapsedContextHistory
	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")
	if state.collapsedContextHistory == initiallyCollapsed {
		t.Fatalf("expected SPACE to toggle context-history collapsed state")
	}

	// Second SPACE collapses it again.
	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")
	if state.collapsedContextHistory != initiallyCollapsed {
		t.Fatalf("expected second SPACE to restore original collapsed state")
	}
}

// TestContextWidgetKeyboard_ArrowMovesSectionHeader verifies that Up/Down when
// outside a section move the activeSection cursor through the section list.
func TestContextWidgetKeyboard_ArrowMovesSectionHeader(t *testing.T) {
	state := newContextFeedbackViewState()
	// Start at current-context (bottom of ordered list).
	state.activeSection = "current-context"
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if state.activeSection != "working-memory" {
		t.Fatalf("expected up to move to working-memory, got %q", state.activeSection)
	}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if state.activeSection != "context-history" {
		t.Fatalf("expected second up to move to context-history, got %q", state.activeSection)
	}

	// Up at the top should stay at context-history (clamp at start).
	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if state.activeSection != "context-history" {
		t.Fatalf("expected up at top to stay at context-history, got %q", state.activeSection)
	}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")
	if state.activeSection != "working-memory" {
		t.Fatalf("expected down to move to working-memory, got %q", state.activeSection)
	}
}

func TestContextWidgetKeyboard_SelectionAndBoundsStatusVocabulary(t *testing.T) {
	state := newContextFeedbackViewState()
	state.insideSection = true
	state.activeSection = "current-context"
	state.updateOrderedRows([]string{"current:1:prompt", "current:1:response"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys", Turns: []ChatTurn{{Prompt: "p1", Response: "r1"}}}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Selection moved." {
		t.Fatalf("expected normalized selection-moved status, got %q", got)
	}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Selection at last row." {
		t.Fatalf("expected normalized lower-bound status, got %q", got)
	}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Selection moved." {
		t.Fatalf("expected normalized selection-moved status after up, got %q", got)
	}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Selection at first row." {
		t.Fatalf("expected normalized upper-bound status, got %q", got)
	}
}

func TestContextWidgetKeyboard_HistoryArrowNavigationDoesNotExpandNodes(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.collapsedContextHistory = false
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-prev"})
	state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}})
	state.insideSection = true
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")

	if got := state.activeRowKey(); got != "user:mpeters" {
		t.Fatalf("expected selection to stay on only user sibling, got %q", got)
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "context-history" {
		t.Fatalf("expected history focus path to remain at section root after arrow navigation, got %#v", got)
	}
	if focusPathHasNode(state.focusPath, nodeID{Kind: nodeKindUser, User: "mpeters"}) {
		t.Fatalf("expected user node not to auto-expand on arrow navigation")
	}
	if got := state.statusLine; got != "Selection at last row." {
		t.Fatalf("expected boundary status when no user siblings exist, got %q", got)
	}
}

func TestContextWidgetKeyboard_HistoryArrowNavigationSurvivesRenderWithoutExpansion(t *testing.T) {
	projectDir := t.TempDir()
	turnPath := filepath.Join(projectDir, "sessions", "mpeters", "s-prev", "context", "turns.jsonl")
	if err := os.MkdirAll(filepath.Dir(turnPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	payload, err := json.Marshal(ChatTurn{Prompt: "hello", Response: "world", CreatedAt: 1_700_000_000_000})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := os.WriteFile(turnPath, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "current-user",
	})

	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.collapsedContextHistory = false
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-prev"})
	state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}})
	state.insideSection = true

	snapshot := contextWidgetSnapshot{SessionID: "current-session"}
	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")

	if got := state.activeRowKey(); got != "user:mpeters" {
		t.Fatalf("expected selection to remain on user row before render, got %q", got)
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "context-history" {
		t.Fatalf("expected history focus path to remain at section root before render, got %#v", got)
	}

	_ = renderContextFeedbackSections(snapshot, nil, state, 120)

	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "context-history" {
		t.Fatalf("expected render pass not to auto-expand history path, got %#v", got)
	}
	if focusPathHasNode(state.focusPath, nodeID{Kind: nodeKindUser, User: "mpeters"}) {
		t.Fatalf("expected render pass not to auto-expand user node")
	}
}

func TestContextWidgetKeyboard_HistoryDownOnOnlyUserRowIsNoOp(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.collapsedContextHistory = false
	state.updateOrderedRows([]string{"user:mpeters"})
	state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}})
	state.insideSection = true
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")

	if got := state.activeRowKey(); got != "user:mpeters" {
		t.Fatalf("expected only user row to remain active, got %q", got)
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "context-history" {
		t.Fatalf("expected no implicit descend into sessions, got %#v", got)
	}
	if got := state.statusLine; got != "Selection at last row." {
		t.Fatalf("expected lower-bound no-op status, got %q", got)
	}
}

func TestContextWidgetKeyboard_HistoryDownOnSingleUserWithVisibleSessionsIsNoOp(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.collapsedContextHistory = false
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-prev", "session:mpeters:s-older"})
	state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}})
	state.insideSection = true
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")

	if got := state.activeRowKey(); got != "user:mpeters" {
		t.Fatalf("expected down to remain on user row when no sibling users exist, got %q", got)
	}
	if got := state.statusLine; got != "Selection at last row." {
		t.Fatalf("expected lower-bound no-op status, got %q", got)
	}
}

func TestContextWidgetKeyboard_SpaceHistoryNodePeeksWithoutSelectionSemantics(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.collapsedContextHistory = false
	state.insideSection = true
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-prev"})
	state.selectedEntries["user:mpeters"] = true
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")

	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != "mpeters" {
		t.Fatalf("expected space to expand history node via focus path, got %#v", got)
	}
	if state.selectedEntries["user:mpeters"] {
		t.Fatalf("expected space on history row to avoid selected/enabled row semantics")
	}
	if marker := stripAnsi(rowMarker(state, "user:mpeters")); strings.Contains(marker, "●") {
		t.Fatalf("expected history row marker without selection dot, got %q", marker)
	}
	if got := state.statusLine; got != "History node expanded." {
		t.Fatalf("expected history expand status, got %q", got)
	}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "context-history" {
		t.Fatalf("expected second space to collapse back to section, got %#v", got)
	}
	if got := state.statusLine; got != "History node collapsed." {
		t.Fatalf("expected history collapse status, got %q", got)
	}
}

func TestContextWidgetKeyboard_SpaceOnSessionRowPeeksWithoutSelectionSemantics(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.collapsedContextHistory = false
	state.insideSection = true
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-prev"})
	state.selectedEntries["session:mpeters:s-prev"] = true
	if !state.setActiveRowByKey("session:mpeters:s-prev") {
		t.Fatalf("expected session row activation to succeed")
	}
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")

	if got := state.focusPath.Tail(); got.Kind != nodeKindSession || got.User != "mpeters" || got.Session != "s-prev" {
		t.Fatalf("expected space to expand focused session node, got %#v", got)
	}
	if state.selectedEntries["session:mpeters:s-prev"] {
		t.Fatalf("expected session row space to avoid selected/enabled semantics")
	}
	if marker := stripAnsi(rowMarker(state, "session:mpeters:s-prev")); strings.Contains(marker, "●") {
		t.Fatalf("expected session row marker without selection dot, got %q", marker)
	}
	if got := state.statusLine; got != "History node expanded." {
		t.Fatalf("expected history expand status for session row, got %q", got)
	}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != "mpeters" {
		t.Fatalf("expected second space on focused session row to collapse to parent user, got %#v", got)
	}
	if got := state.statusLine; got != "History node collapsed." {
		t.Fatalf("expected history collapse status for session row, got %q", got)
	}
}

func TestContextWidgetKeyboard_ViewportStatusVocabulary(t *testing.T) {
	state := newContextFeedbackViewState()
	state.insideSection = true
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	state.activeSection = "context-history"
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-prev"})
	applyContextWidgetCommand(state, "pgdn", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Selection moved." {
		t.Fatalf("expected context-history pgdn to page rows, got %q", got)
	}

	applyContextWidgetCommand(state, "pgup", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Selection moved." {
		t.Fatalf("expected context-history pgup to page rows, got %q", got)
	}

	state.activeSection = "working-memory"
	state.updateOrderedRows([]string{"wm:editor:key", "wm:editor:value", "wm:editor:save", "wm:user:current_user"})
	applyContextWidgetCommand(state, "pgdn", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Selection moved." {
		t.Fatalf("expected working-memory pgdn to page rows, got %q", got)
	}

	applyContextWidgetCommand(state, "pgup", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Selection moved." {
		t.Fatalf("expected working-memory pgup to page rows, got %q", got)
	}
}

func TestContextWidgetKeyboard_EnterTabShiftTabTransitionVocabulary(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Enter has no action." {
		t.Fatalf("expected enter action-only status outside section, got %q", got)
	}
	if state.insideSection {
		t.Fatalf("expected enter not to drill into section")
	}

	state.insideSection = false
	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Entered section: current-context." {
		t.Fatalf("expected tab transition status, got %q", got)
	}

	applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Exited section: current-context." {
		t.Fatalf("expected shift-tab transition status, got %q", got)
	}
}

// TestContextWidgetKeyboard_PgDnScrollsExpandedRow verifies that PgDn when
// inside a section and the active row is an expanded current-context entry
// scrolls the text content instead of moving rows.
func TestContextWidgetKeyboard_PgDnScrollsExpandedRow(t *testing.T) {
	state := newContextFeedbackViewState()
	state.insideSection = true
	state.activeSection = "current-context"
	state.updateOrderedRows([]string{"current:1:prompt"})
	// Mark the row as explicitly expanded (not collapsed).
	state.collapsedEntries["current:1:prompt"] = false
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys", Turns: []ChatTurn{{Prompt: "one two three four five six seven eight nine ten eleven twelve", Response: "ok"}}}

	applyContextWidgetCommand(state, "pgdn", "http://127.0.0.1:0", snapshot, "", "")
	if state.textScroll["current:1:prompt"] == 0 {
		t.Fatalf("expected pgdn to scroll expanded text row when inside section")
	}
}

func TestContextWidgetKeyboard_LeftRightCurrentTurnSibling(t *testing.T) {
	state := newContextFeedbackViewState()
	state.updateOrderedRows([]string{"current:1:prompt", "current:1:response"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys", Turns: []ChatTurn{{Prompt: "p1", Response: "r1"}}}

	if state.activeRowKey() != "current:1:prompt" {
		t.Fatalf("expected initial active row to be prompt")
	}

	applyContextWidgetCommand(state, "right", "http://127.0.0.1:0", snapshot, "", "")
	if state.activeRowKey() != "current:1:response" {
		t.Fatalf("expected right to move to response sibling, got %q", state.activeRowKey())
	}

	applyContextWidgetCommand(state, "left", "http://127.0.0.1:0", snapshot, "", "")
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

	applyContextWidgetCommand(state, "right", "http://127.0.0.1:0", snapshot, "", "")
	if state.activeRowKey() != "history:s-prev:1" {
		t.Fatalf("expected right to enter first session turn, got %q", state.activeRowKey())
	}

	applyContextWidgetCommand(state, "left", "http://127.0.0.1:0", snapshot, "", "")
	if state.activeRowKey() != "session:s-prev" {
		t.Fatalf("expected left to return to session row, got %q", state.activeRowKey())
	}
}

func TestContextWidgetKeyboard_EnterHistoryIsActionOnly(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.insideSection = true
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-prev"})
	state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.statusLine; got != "Enter has no action." {
		t.Fatalf("expected enter action-only status in context-history, got %q", got)
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "context-history" {
		t.Fatalf("expected enter to avoid history drill-in, got %#v", got)
	}
}

func TestContextWidgetKeyboard_SpaceInsideWorkingMemoryTargetsRowOnly(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "working-memory"
	state.insideSection = true
	state.collapsedWorkingMemory = false
	state.updateOrderedRows([]string{"wm:editor:key", "wm:user:current_user"})
	_ = state.setActiveRowByKey("wm:user:current_user")
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")

	if state.collapsedWorkingMemory {
		t.Fatalf("expected space inside working-memory to avoid section collapse")
	}
	if got := state.statusLine; got != "Row has no expand/collapse action." {
		t.Fatalf("expected focused-target no-op status, got %q", got)
	}
}

func TestContextWidgetKeyboard_EnterWorkingMemoryFactTogglesEnabledState(t *testing.T) {
	projectDir := t.TempDir()
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "mpeters",
		"AGENTX_SESSION_ID":  "sess-wm-enter",
	})

	sessionDir := filepath.Join(projectDir, "sessions", "mpeters", "sess-wm-enter")
	if err := saveWorkingMemoryPayload(sessionDir, map[string]workingMemoryFactSnapshot{
		"user:topic": {Owner: "user", Key: "topic", Value: "go", Enabled: true},
	}); err != nil {
		t.Fatalf("saveWorkingMemoryPayload failed: %v", err)
	}

	state := newContextFeedbackViewState()
	state.activeSection = "working-memory"
	state.insideSection = true
	state.updateOrderedRows([]string{"wm:user:topic"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	payload, err := loadWorkingMemoryPayload(sessionDir)
	if err != nil {
		t.Fatalf("loadWorkingMemoryPayload failed: %v", err)
	}
	if payload["user:topic"].Enabled {
		t.Fatalf("expected first enter to disable focused fact")
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	payload, err = loadWorkingMemoryPayload(sessionDir)
	if err != nil {
		t.Fatalf("loadWorkingMemoryPayload failed: %v", err)
	}
	if !payload["user:topic"].Enabled {
		t.Fatalf("expected second enter to re-enable focused fact")
	}
}

func TestContextWidgetKeyboard_EnterWorkingMemoryEditorCommitAndSave(t *testing.T) {
	projectDir := t.TempDir()
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "mpeters",
		"AGENTX_SESSION_ID":  "sess-wm-editor",
	})
	sessionDir := filepath.Join(projectDir, "sessions", "mpeters", "sess-wm-editor")

	state := newContextFeedbackViewState()
	state.activeSection = "working-memory"
	state.insideSection = true
	state.collapsedWorkingMemory = false
	state.updateOrderedRows([]string{"wm:editor:key", "wm:editor:value", "wm:editor:save"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "feature_flag", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.wmEditorDraftKey; got != "feature_flag" {
		t.Fatalf("expected key draft staged, got %q", got)
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeRowKey(); got != "wm:editor:value" {
		t.Fatalf("expected enter on key cell to advance to value cell, got %q", got)
	}

	applyContextWidgetCommand(state, "enabled", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.wmEditorDraftValue; got != "enabled" {
		t.Fatalf("expected value draft staged, got %q", got)
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeRowKey(); got != "wm:editor:save" {
		t.Fatalf("expected enter on value cell to advance to save cell, got %q", got)
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	payload, err := loadWorkingMemoryPayload(sessionDir)
	if err != nil {
		t.Fatalf("loadWorkingMemoryPayload failed: %v", err)
	}
	fact, ok := payload["user:feature_flag"]
	if !ok {
		t.Fatalf("expected save cell enter to persist feature_flag")
	}
	if fact.Value != "enabled" {
		t.Fatalf("expected persisted value enabled, got %#v", fact.Value)
	}
	if !fact.Enabled {
		t.Fatalf("expected persisted fact to be enabled")
	}
}

func TestContextWidgetKeyboard_WorkingMemoryEditorSavePreservesStartupDefaultsAcrossMultipleAdds(t *testing.T) {
	projectDir := t.TempDir()
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "mpeters",
		"AGENTX_SESSION_ID":  "sess-wm-defaults",
	})
	sessionDir := filepath.Join(projectDir, "sessions", "mpeters", "sess-wm-defaults")

	state := newContextFeedbackViewState()
	state.activeSection = "working-memory"
	state.insideSection = true
	state.collapsedWorkingMemory = false
	state.updateOrderedRows([]string{"wm:editor:key", "wm:editor:value", "wm:editor:save"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-wm-defaults"}

	applyContextWidgetCommand(state, "feature_flag", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "enabled", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")

	payload, err := loadWorkingMemoryPayload(sessionDir)
	if err != nil {
		t.Fatalf("loadWorkingMemoryPayload failed after first save: %v", err)
	}
	if _, ok := payload["user:current_user"]; !ok {
		t.Fatalf("expected startup default current_user to persist after first save")
	}
	if _, ok := payload["user:current_working_directory"]; !ok {
		t.Fatalf("expected startup default current_working_directory to persist after first save")
	}
	if fact, ok := payload["user:feature_flag"]; !ok || fact.Value != "enabled" {
		t.Fatalf("expected first user key to persist after first save, got %#v (present=%v)", fact, ok)
	}

	applyContextWidgetCommand(state, "release_channel", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "stable", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")

	payload, err = loadWorkingMemoryPayload(sessionDir)
	if err != nil {
		t.Fatalf("loadWorkingMemoryPayload failed after second save: %v", err)
	}
	if _, ok := payload["user:current_user"]; !ok {
		t.Fatalf("expected startup default current_user to persist after second save")
	}
	if _, ok := payload["user:current_working_directory"]; !ok {
		t.Fatalf("expected startup default current_working_directory to persist after second save")
	}
	if fact, ok := payload["user:feature_flag"]; !ok || fact.Value != "enabled" {
		t.Fatalf("expected first user key to remain after second save, got %#v (present=%v)", fact, ok)
	}
	if fact, ok := payload["user:release_channel"]; !ok || fact.Value != "stable" {
		t.Fatalf("expected second user key to persist after second save, got %#v (present=%v)", fact, ok)
	}
}

func TestContextWidgetKeyboard_SpaceOnWorkingMemoryHeaderTogglesOnly(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "working-memory"
	state.collapsedWorkingMemory = true
	state.updateOrderedRows([]string{"wm:editor:key", "wm:editor:value", "wm:editor:save", "wm:user:current_user"})
	if !state.setActiveRowByKey("wm:user:current_user") {
		t.Fatalf("expected active row setup for working-memory row to succeed")
	}
	state.insideSection = false
	activeBefore := state.activeRow
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")

	if state.collapsedWorkingMemory {
		t.Fatalf("expected SPACE on working-memory header to toggle expanded state")
	}
	if state.insideSection {
		t.Fatalf("expected SPACE on working-memory header to remain outside section")
	}
	if state.activeRow != activeBefore {
		t.Fatalf("expected SPACE on working-memory header not to advance row focus; got %d want %d", state.activeRow, activeBefore)
	}
}

// TestContextWidgetKeyboard_SpaceAfterExitWMSection_DoesNotReenterSection covers
// the exact regression where the user was inside the WM section (insideSection=true,
// activeRow pointing at a WM row), pressed Shift-Tab to exit, then pressed SPACE
// to expand the section. The subsequent updateOrderedRows call (simulating a render)
// previously called setFocusPath unconditionally, which re-set insideSection=true.
func TestContextWidgetKeyboard_SpaceAfterExitWMSection_DoesNotReenterSection(t *testing.T) {
	wmRows := []string{"wm:editor:key", "wm:editor:value", "wm:editor:save", "wm:user:project"}
	snapshot := contextWidgetSnapshot{SessionID: "sess-space-reenter"}

	state := newContextFeedbackViewState()
	state.activeSection = "working-memory"
	state.insideSection = true
	state.collapsedWorkingMemory = false
	state.updateOrderedRows(wmRows)
	_ = state.setActiveRowByKey("wm:editor:key")
	if got := state.activeRowKey(); got != "wm:editor:key" {
		t.Fatalf("expected wm:editor:key to be active inside section, got %q", got)
	}

	// Shift-Tab exits the section.
	applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot, "", "")
	if state.insideSection {
		t.Fatalf("expected shift-tab to set insideSection=false, got true")
	}

	// SPACE expands the section from outside.
	state.collapsedWorkingMemory = true // simulate collapsed before SPACE
	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")
	if state.collapsedWorkingMemory {
		t.Fatalf("expected SPACE to expand working-memory section")
	}
	if state.insideSection {
		t.Fatalf("expected SPACE to leave insideSection=false, got true (immediate check)")
	}

	// Simulate the render cycle: updateOrderedRows is called with WM rows visible
	// (WM is now expanded). This is the step that previously triggered the bug.
	state.updateOrderedRows(wmRows)
	if state.insideSection {
		t.Fatalf("expected updateOrderedRows after SPACE-expand not to set insideSection=true (regression: TAB effect)")
	}
	if state.activeSection != "working-memory" {
		t.Fatalf("expected activeSection to remain working-memory, got %q", state.activeSection)
	}
}

func TestContextWidgetKeyboard_WorkingMemoryEditorIncrementalTypingBackspaceAndLimits(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "working-memory"
	state.insideSection = true
	state.collapsedWorkingMemory = false
	state.updateOrderedRows([]string{"wm:editor:key", "wm:editor:value", "wm:editor:save"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	for _, token := range []string{"h", "j", "k", "l"} {
		applyContextWidgetCommand(state, token, "http://127.0.0.1:0", snapshot, "", "")
	}
	if got := state.wmEditorDraftKey; got != "hjkl" {
		t.Fatalf("expected incremental key typing to append hjkl, got %q", got)
	}
	if got := state.activeRowKey(); got != "wm:editor:key" {
		t.Fatalf("expected editor focus to stay on key while typing, got %q", got)
	}

	applyContextWidgetCommand(state, "backspace", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.wmEditorDraftKey; got != "hjk" {
		t.Fatalf("expected backspace to remove last key rune, got %q", got)
	}

	for i := 0; i < 80; i++ {
		applyContextWidgetCommand(state, "x", "http://127.0.0.1:0", snapshot, "", "")
	}
	if got := len([]rune(state.wmEditorDraftKey)); got != workingMemoryEditorKeyMaxChars {
		t.Fatalf("expected key to clamp at %d chars, got %d", workingMemoryEditorKeyMaxChars, got)
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeRowKey(); got != "wm:editor:value" {
		t.Fatalf("expected enter on key to advance to value, got %q", got)
	}

	for i := 0; i < 1100; i++ {
		applyContextWidgetCommand(state, "v", "http://127.0.0.1:0", snapshot, "", "")
	}
	if got := len(state.wmEditorDraftValue); got != workingMemoryEditorValueMaxBytes {
		t.Fatalf("expected value to clamp at %d bytes, got %d", workingMemoryEditorValueMaxBytes, got)
	}

	applyContextWidgetCommand(state, "backspace", "http://127.0.0.1:0", snapshot, "", "")
	if got := len(state.wmEditorDraftValue); got != workingMemoryEditorValueMaxBytes-1 {
		t.Fatalf("expected value backspace to remove one rune/byte, got len=%d", got)
	}
}

func TestContextWidgetKeyboard_WorkingMemoryEditorAllowsColonAndIgnoresHomeEnd(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "working-memory"
	state.insideSection = true
	state.collapsedWorkingMemory = false
	state.updateOrderedRows([]string{"wm:editor:key", "wm:editor:value", "wm:editor:save"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "feature:flag", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.wmEditorDraftKey; got != "feature:flag" {
		t.Fatalf("expected colon to stage in key draft, got %q", got)
	}

	applyContextWidgetCommand(state, "home", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.wmEditorDraftKey; got != "feature:flag" {
		t.Fatalf("expected home to be ignored in active key cell, got %q", got)
	}
	if got := state.activeRowKey(); got != "wm:editor:key" {
		t.Fatalf("expected home to leave focus on key cell, got %q", got)
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeRowKey(); got != "wm:editor:value" {
		t.Fatalf("expected enter to advance to value cell, got %q", got)
	}

	applyContextWidgetCommand(state, "value:enabled", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.wmEditorDraftValue; got != "value:enabled" {
		t.Fatalf("expected colon to stage in value draft, got %q", got)
	}

	applyContextWidgetCommand(state, "end", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.wmEditorDraftValue; got != "value:enabled" {
		t.Fatalf("expected end to be ignored in active value cell, got %q", got)
	}
	if got := state.activeRowKey(); got != "wm:editor:value" {
		t.Fatalf("expected end to leave focus on value cell, got %q", got)
	}
}

func TestRenderWorkingMemoryEditorCellViewport_TailWhenActiveHeadAfterCommit(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "working-memory"
	state.insideSection = true
	state.collapsedWorkingMemory = false
	state.updateOrderedRows([]string{"wm:editor:key", "wm:editor:value", "wm:editor:save"})
	state.wmEditorDraftKey = "abcdefghijklmnopqrstuvwxyz"
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	activeKey := strings.Join(renderWorkingMemoryEditorScaffold(state, nil, ansiBlue), "\n")
	if !strings.Contains(stripAnsi(activeKey), "│defghijklmnopqrstuvwxyz│") {
		t.Fatalf("expected active key viewport to show trailing 23 chars, got:\n%s", stripAnsi(activeKey))
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeRowKey(); got != "wm:editor:value" {
		t.Fatalf("expected enter to commit key and move to value, got %q", got)
	}

	committedKey := strings.Join(renderWorkingMemoryEditorScaffold(state, nil, ansiBlue), "\n")
	if !strings.Contains(stripAnsi(committedKey), "│abcdefghijklmnopqrstuvw│") {
		t.Fatalf("expected committed key viewport to show leading 23 chars, got:\n%s", stripAnsi(committedKey))
	}
}

func TestResolveContextHistorySessionSort_EnvOverride(t *testing.T) {
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_CONTEXT_HISTORY_SESSION_SORT": "Ascending",
	})

	if got := resolveContextHistorySessionSort(""); got != contextHistorySortAscending {
		t.Fatalf("expected env override to resolve ascending, got %q", got)
	}
}

func TestResolveContextHistorySessionSort_FromToml(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\ncontext_history_session_sort = \"Ascending\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if got := resolveContextHistorySessionSort(projectDir); got != contextHistorySortAscending {
		t.Fatalf("expected agentx.toml value to resolve ascending, got %q", got)
	}
}

func TestDiscoverContextHistorySessions_HonorsSortOrder(t *testing.T) {
	projectDir := t.TempDir()
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "tester",
	})

	writeTurns := func(sessionID string, createdAt int64) {
		t.Helper()
		turnPath := filepath.Join(projectDir, "sessions", "tester", sessionID, "context", "turns.jsonl")
		if err := os.MkdirAll(filepath.Dir(turnPath), 0o755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}
		turn := ChatTurn{Prompt: "p", Response: "r", CreatedAt: createdAt}
		payload, err := json.Marshal(turn)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if err := os.WriteFile(turnPath, append(payload, '\n'), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}

	writeTurns("older-session", 1_000)
	writeTurns("newer-session", 2_000)
	writeTurns("current-session", 3_000)

	setWidgetTestEnv(t, map[string]string{
		"AGENTX_CONTEXT_HISTORY_SESSION_SORT": "Descending",
	})
	desc := discoverContextHistorySessions("current-session")
	if len(desc) != 2 {
		t.Fatalf("expected 2 history sessions, got %d", len(desc))
	}
	if desc[0].SessionID != "newer-session" || desc[1].SessionID != "older-session" {
		t.Fatalf("expected descending order newer->older, got %q then %q", desc[0].SessionID, desc[1].SessionID)
	}

	setWidgetTestEnv(t, map[string]string{
		"AGENTX_CONTEXT_HISTORY_SESSION_SORT": "Ascending",
	})
	asc := discoverContextHistorySessions("current-session")
	if len(asc) != 2 {
		t.Fatalf("expected 2 history sessions, got %d", len(asc))
	}
	if asc[0].SessionID != "older-session" || asc[1].SessionID != "newer-session" {
		t.Fatalf("expected ascending order older->newer, got %q then %q", asc[0].SessionID, asc[1].SessionID)
	}
}

func TestNodeIDForRowKey_MapsCurrentHistoryAndWMNodes(t *testing.T) {
	tests := []struct {
		section string
		rowKey  string
		kind    nodeKind
	}{
		{section: "context-history", rowKey: "user:mpeters", kind: nodeKindUser},
		{section: "context-history", rowKey: "session:mpeters:session_1", kind: nodeKindSession},
		{section: "context-history", rowKey: "history:mpeters:session_1:2", kind: nodeKindTurn},
		{section: "working-memory", rowKey: "wm:editor:key", kind: nodeKindWmCell},
		{section: "current-context", rowKey: "current:1:prompt", kind: nodeKindEntry},
	}
	for _, tc := range tests {
		id, ok := nodeIDForRowKey(tc.section, tc.rowKey)
		if !ok {
			t.Fatalf("expected %q in %q to map to node id", tc.rowKey, tc.section)
		}
		if id.Kind != tc.kind {
			t.Fatalf("expected %q in %q to map to kind %q, got %q", tc.rowKey, tc.section, tc.kind, id.Kind)
		}
	}
}

func TestMoveRowInActiveSection_UpdatesFocusPath(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "current-context"
	state.insideSection = true
	state.updateOrderedRows([]string{"current:1:prompt", "current:1:response"})

	if !state.moveRowInActiveSection(1) {
		t.Fatalf("expected row move to succeed")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindEntry || got.Entry != "response" {
		t.Fatalf("expected focusPath tail to track current response entry, got %#v", got)
	}
}

func TestMoveRowInActiveSection_HistorySiblingSwitchDoesNotRewriteFocusPath(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.insideSection = true
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-1", "session:mpeters:s-2"})
	state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}, {Kind: nodeKindUser, User: "mpeters"}})

	if !state.moveRowInActiveSection(1) {
		t.Fatalf("expected move to first session row to succeed")
	}
	if got := state.activeRowKey(); got != "session:mpeters:s-1" {
		t.Fatalf("expected first session row to be active, got %q", got)
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != "mpeters" {
		t.Fatalf("expected focus path to remain on expanded user node, got %#v", got)
	}

	if !state.moveRowInActiveSection(1) {
		t.Fatalf("expected move to sibling session row to succeed")
	}
	if got := state.activeRowKey(); got != "session:mpeters:s-2" {
		t.Fatalf("expected second session row to be active, got %q", got)
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != "mpeters" {
		t.Fatalf("expected sibling move not to rewrite history focus path, got %#v", got)
	}
}

func TestUpdateOrderedRows_ContextHistoryPreservesFocusByStableKeyAndPath(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.insideSection = true
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-1", "session:mpeters:s-2"})
	if !state.setActiveRowByKey("session:mpeters:s-2") {
		t.Fatalf("expected initial session row activation to succeed")
	}
	state.setFocusPath(focusPath{
		{Kind: nodeKindSection, Section: "context-history"},
		{Kind: nodeKindUser, User: "mpeters"},
		{Kind: nodeKindSession, User: "mpeters", Session: "s-2"},
	})

	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-2"})
	if got := state.activeRowKey(); got != "session:mpeters:s-2" {
		t.Fatalf("expected stable row-key remap to keep focused session, got %q", got)
	}

	state.updateOrderedRows([]string{"user:mpeters"})
	if got := state.activeRowKey(); got != "user:mpeters" {
		t.Fatalf("expected focus-path fallback remap to nearest ancestor row, got %q", got)
	}
}

func TestContextWidgetKeyboard_ShiftTabPopsDeepHistoryPath(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.insideSection = true
	state.collapsedContextHistory = false
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-1", "history:mpeters:s-1:1"})
	state.setFocusPath(focusPath{
		{Kind: nodeKindSection, Section: "context-history"},
		{Kind: nodeKindUser, User: "mpeters"},
		{Kind: nodeKindSession, User: "mpeters", Session: "s-1"},
		{Kind: nodeKindTurn, User: "mpeters", Session: "s-1", Turn: 1},
	})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	if !state.setActiveRowByKey("history:mpeters:s-1:1") {
		t.Fatalf("expected turn row activation to succeed")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindTurn {
		t.Fatalf("expected deep history focus before shift-tab, got %#v", got)
	}

	applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot, "", "")
	if !state.insideSection {
		t.Fatalf("expected shift-tab from turn to stay inside section (step back to session)")
	}
	if state.collapsedContextHistory {
		t.Fatalf("expected shift-tab from turn to leave context-history expanded")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSession || got.Session != "s-1" {
		t.Fatalf("expected shift-tab to pop focus path to parent session node, got %#v", got)
	}
	if got := state.statusLine; got != "Stepped back in context-history." {
		t.Fatalf("expected step-back status from turn node, got %q", got)
	}
}

func TestContextHistoryTreeModel_MoveVerticalTraversesEntryLeavesAcrossTurns(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.insideSection = true
	state.updateOrderedRows([]string{
		"user:mpeters",
		"session:mpeters:s-1",
		"history:mpeters:s-1:1:prompt",
		"history:mpeters:s-1:1:response",
		"history:mpeters:s-1:2:prompt",
		"history:mpeters:s-1:2:response",
	})
	model := newContextHistoryTreeModel(state.orderedRowKeys)

	if !state.setActiveRowByKey("history:mpeters:s-1:1:response") {
		t.Fatalf("expected first response leaf activation to succeed")
	}
	if !model.MoveVertical(state, 1) {
		t.Fatalf("expected down from turn1 response leaf to move")
	}
	if got := state.activeRowKey(); got != "history:mpeters:s-1:2:prompt" {
		t.Fatalf("expected down to traverse turn1 response -> turn2 prompt, got %q", got)
	}

	if !model.MoveVertical(state, -1) {
		t.Fatalf("expected up from turn2 prompt leaf to move")
	}
	if got := state.activeRowKey(); got != "history:mpeters:s-1:1:response" {
		t.Fatalf("expected up to traverse turn2 prompt -> turn1 response, got %q", got)
	}
	if state.focusTextBox {
		t.Fatalf("expected vertical traversal to keep text focus disabled")
	}
}

func TestContextWidgetKeyboard_TabContextHistoryPopulatedRows_RenderCoupledProgressiveDrillIn(t *testing.T) {
	projectDir := t.TempDir()
	sessionID := "2026-06-06 23:20:18"
	turnPath := filepath.Join(projectDir, "sessions", "mpeters", sessionID, "context", "turns.jsonl")
	if err := os.MkdirAll(filepath.Dir(turnPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	payload, err := json.Marshal(ChatTurn{Prompt: "hello", Response: "world", CreatedAt: 1_700_000_000_000})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := os.WriteFile(turnPath, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "current-user",
	})

	snapshot := contextWidgetSnapshot{SessionID: "current-session", Turns: []ChatTurn{{Prompt: "q", Response: "a"}}}
	assertExpected := func(t *testing.T, state *contextFeedbackViewState, presses int) {
		t.Helper()
		if !state.insideSection {
			t.Fatalf("expected insideSection=true after %d tabs", presses)
		}
		if got := state.activeSection; got != "context-history" {
			t.Fatalf("expected activeSection=context-history after %d tabs, got %q", presses, got)
		}
		if state.collapsedContextHistory {
			t.Fatalf("expected collapsedContextHistory=false after %d tabs", presses)
		}
		wantRow := ""
		switch presses {
		case 1:
			wantRow = "user:mpeters"
		case 2:
			wantRow = "session:mpeters:" + sessionID
		case 3, 4, 5:
			wantRow = "history:mpeters:" + sessionID + ":1:prompt"
		default:
			t.Fatalf("unexpected presses=%d", presses)
		}
		if got := state.activeRowKey(); got != wantRow {
			t.Fatalf("expected activeRowKey %q after %d tabs, got %q", wantRow, presses, got)
		}
		tail := state.focusPath.Tail()
		switch presses {
		case 1:
			if tail.Kind != nodeKindUser || tail.User != "mpeters" {
				t.Fatalf("expected focusPath tail user:mpeters after %d tabs, got %#v", presses, tail)
			}
		case 2:
			if tail.Kind != nodeKindSession || tail.User != "mpeters" || tail.Session != sessionID {
				t.Fatalf("expected focusPath tail session:mpeters:%s after %d tabs, got %#v", sessionID, presses, tail)
			}
		case 3, 4, 5:
			if tail.Kind != nodeKindTurn || tail.User != "mpeters" || tail.Session != sessionID || tail.Turn != 1 {
				t.Fatalf("expected focusPath tail history:mpeters:%s:1 after %d tabs, got %#v", sessionID, presses, tail)
			}
		}
	}

	for presses := 1; presses <= 5; presses++ {
		presses := presses
		t.Run("render_coupled_tab_presses_"+itoa(presses), func(t *testing.T) {
			state := newContextFeedbackViewState()
			state.activeSection = "context-history"
			state.insideSection = false
			state.collapsedContextHistory = false
			state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}})

			_ = renderContextWidgetWithState(snapshot, "context-history", "qwen3.6:latest", "ollama", 80, 200, nil, state)
			if got := state.activeRowKey(); got != "user:mpeters" {
				t.Fatalf("expected render path to populate history user row before tab loop, got %q", got)
			}

			state.collapsedContextHistory = true
			state.insideSection = false
			state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}})
			if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "context-history" {
				t.Fatalf("expected precondition focusPath tail section:context-history, got %#v", got)
			}

			for i := 0; i < presses; i++ {
				applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
				_ = renderContextWidgetWithState(snapshot, "context-history", "qwen3.6:latest", "ollama", 80, 200, nil, state)
				assertExpected(t, state, i+1)
				if got := state.statusLine; got != "Entered section: context-history." {
					t.Fatalf("expected normalized tab-enter status after tab %d/%d, got %q", i+1, presses, got)
				}
			}
		})
	}
}

// TestContextWidgetKeyboard_TabNShiftTabNMinus1_StepsBackIncrementally verifies
// that pressing Shift-Tab (n-1) times after Tab×n steps back one level at a time
// rather than exiting the section completely on the first Shift-Tab press.
//
// State model:
//
//	S0 = outside section (insideSection=false, collapsed=true)
//	S1 = user row    (insideSection=true)
//	S2 = session row (insideSection=true)
//	S3 = turn row    (insideSection=true)
func TestContextWidgetKeyboard_TabNShiftTabNMinus1_StepsBackIncrementally(t *testing.T) {
	projectDir := t.TempDir()
	sessionID := "2026-06-06 23:20:18"
	user := "current-user"
	turnPath := filepath.Join(projectDir, "sessions", user, sessionID, "context", "turns.jsonl")
	if err := os.MkdirAll(filepath.Dir(turnPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	payload, err := json.Marshal(ChatTurn{Prompt: "hello", Response: "world", CreatedAt: 1_700_000_000_000})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := os.WriteFile(turnPath, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    user,
	})

	snapshot := contextWidgetSnapshot{SessionID: "current-session", Turns: []ChatTurn{{Prompt: "q", Response: "a"}}}
	userRowKey := "user:" + user
	sessionRowKey := historySessionRowKey(user, sessionID)
	turnRowKey := "history:" + user + ":" + sessionID + ":1:prompt"

	assertS0 := func(t *testing.T, state *contextFeedbackViewState, label string) {
		t.Helper()
		if state.insideSection {
			t.Fatalf("[%s] S0: expected insideSection=false, got true", label)
		}
		if !state.collapsedContextHistory {
			t.Fatalf("[%s] S0: expected collapsedContextHistory=true, got false", label)
		}
		if tail := state.focusPath.Tail(); tail.Kind != nodeKindSection {
			t.Fatalf("[%s] S0: expected focusPath tail nodeKindSection, got %#v", label, tail)
		}
	}
	assertS1 := func(t *testing.T, state *contextFeedbackViewState, label string) {
		t.Helper()
		if !state.insideSection {
			t.Fatalf("[%s] S1: expected insideSection=true, got false", label)
		}
		if state.collapsedContextHistory {
			t.Fatalf("[%s] S1: expected collapsedContextHistory=false, got true", label)
		}
		if got := state.activeRowKey(); got != userRowKey {
			t.Fatalf("[%s] S1: expected activeRowKey=%q, got %q", label, userRowKey, got)
		}
		if tail := state.focusPath.Tail(); tail.Kind != nodeKindUser || tail.User != user {
			t.Fatalf("[%s] S1: expected focusPath tail user:%s, got %#v", label, user, tail)
		}
	}
	assertS2 := func(t *testing.T, state *contextFeedbackViewState, label string) {
		t.Helper()
		if !state.insideSection {
			t.Fatalf("[%s] S2: expected insideSection=true, got false", label)
		}
		if state.collapsedContextHistory {
			t.Fatalf("[%s] S2: expected collapsedContextHistory=false, got true", label)
		}
		if got := state.activeRowKey(); got != sessionRowKey {
			t.Fatalf("[%s] S2: expected activeRowKey=%q, got %q", label, sessionRowKey, got)
		}
		if tail := state.focusPath.Tail(); tail.Kind != nodeKindSession || tail.User != user || tail.Session != sessionID {
			t.Fatalf("[%s] S2: expected focusPath tail session:%s:%s, got %#v", label, user, sessionID, tail)
		}
	}
	assertS3 := func(t *testing.T, state *contextFeedbackViewState, label string) {
		t.Helper()
		if !state.insideSection {
			t.Fatalf("[%s] S3: expected insideSection=true, got false", label)
		}
		if state.collapsedContextHistory {
			t.Fatalf("[%s] S3: expected collapsedContextHistory=false, got true", label)
		}
		if got := state.activeRowKey(); got != turnRowKey {
			t.Fatalf("[%s] S3: expected activeRowKey=%q, got %q", label, turnRowKey, got)
		}
		if tail := state.focusPath.Tail(); tail.Kind != nodeKindTurn || tail.User != user || tail.Session != sessionID || tail.Turn != 1 {
			t.Fatalf("[%s] S3: expected focusPath tail turn:%s:%s:1, got %#v", label, user, sessionID, tail)
		}
	}

	// tabStates[i] is the expected state after pressing Tab i times (1-indexed).
	tabStates := [6]func(*testing.T, *contextFeedbackViewState, string){
		nil,      // index 0 unused
		assertS1, // Tab 1 → S1
		assertS2, // Tab 2 → S2
		assertS3, // Tab 3 → S3
		assertS3, // Tab 4 → S3 (no-op at leaf)
		assertS3, // Tab 5 → S3 (no-op at leaf)
	}

	// shiftTabPaths[n] is the sequence of expected states after each Shift-Tab press
	// when the preceding Tab×n left us at the given state.
	shiftTabPaths := [6][]func(*testing.T, *contextFeedbackViewState, string){
		nil,                                      // n=0 unused
		{},                                       // n=1: 0 shift-tabs
		{assertS1},                               // n=2: S2→S1
		{assertS2, assertS1},                     // n=3: S3→S2→S1
		{assertS2, assertS1, assertS0},           // n=4: S3→S2→S1→S0
		{assertS2, assertS1, assertS0, assertS0}, // n=5: S3→S2→S1→S0→S0
	}

	finalStates := [6]func(*testing.T, *contextFeedbackViewState, string){
		nil, assertS1, assertS1, assertS1, assertS0, assertS0,
	}

	// Expected status messages after each Shift-Tab press (inside→"Stepped back", exit→"Exited section").
	shiftTabStatuses := [6][]string{
		nil,
		{},
		{"Stepped back in context-history."},
		{"Stepped back in context-history.", "Stepped back in context-history."},
		{"Stepped back in context-history.", "Stepped back in context-history.", "Exited section: context-history."},
		{"Stepped back in context-history.", "Stepped back in context-history.", "Exited section: context-history.", "Exited section: context-history."},
	}

	for n := 1; n <= 5; n++ {
		n := n
		t.Run("tab"+itoa(n)+"_shifttab"+itoa(n-1), func(t *testing.T) {
			state := newContextFeedbackViewState()
			state.activeSection = "context-history"
			state.insideSection = false
			state.collapsedContextHistory = false
			state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}})

			// Populate history rows via initial render (must be expanded to register rows).
			_ = renderContextWidgetWithState(snapshot, "context-history", "qwen3.6:latest", "ollama", 80, 200, nil, state)
			if got := state.activeRowKey(); got != userRowKey {
				t.Fatalf("precondition: expected render to populate history user row, got %q", got)
			}

			// Reset to S0 after render populates orderedRowKeys.
			state.collapsedContextHistory = true
			state.insideSection = false
			state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}})

			// Press Tab×n, asserting expected state after each press.
			for i := 0; i < n; i++ {
				applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
				_ = renderContextWidgetWithState(snapshot, "context-history", "qwen3.6:latest", "ollama", 80, 200, nil, state)
				tabStates[i+1](t, state, "after tab "+itoa(i+1)+"/"+itoa(n))
			}

			// Press Shift-Tab×(n-1), asserting expected state after each press.
			for i := 0; i < n-1; i++ {
				applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot, "", "")
				_ = renderContextWidgetWithState(snapshot, "context-history", "qwen3.6:latest", "ollama", 80, 200, nil, state)
				shiftTabPaths[n][i](t, state, "n="+itoa(n)+" after shift-tab "+itoa(i+1)+"/"+itoa(n-1))
				if got := state.statusLine; got != shiftTabStatuses[n][i] {
					t.Fatalf("[n=%d shift-tab %d] expected status %q, got %q",
						n, i+1, shiftTabStatuses[n][i], got)
				}
			}

			finalStates[n](t, state, "n="+itoa(n)+" final")
		})
	}
}

func TestRenderContextFeedbackSections_CollapsedSectionsRenderBoxStubs(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "sess-collapsed"}

	rendered := strings.Join(renderContextFeedbackSections(snapshot, nil, state, 120), "\n")
	if strings.Count(rendered, "┌") < 2 || strings.Count(rendered, "└") < 2 {
		t.Fatalf("expected collapsed sections to include visible box stubs, got:\n%s", rendered)
	}
}

func TestContextWidgetKeyboard_HistoryTargetSessionResponseExpansion_ShowsFullMultiline(t *testing.T) {
	projectDir := t.TempDir()
	targetSession := "2026-06-06 23:23:13"
	sessionOrder := []string{
		"2026-06-06 23:24:30",
		targetSession,
		"2026-06-06 23:22:10",
		"2026-06-06 23:21:05",
		"2026-06-06 23:20:18",
	}

	writeSessionTurns := func(sessionID string, turns []ChatTurn) {
		t.Helper()
		parsed, err := time.Parse("2006-01-02 15:04:05", sessionID)
		if err != nil {
			t.Fatalf("time.Parse failed for %q: %v", sessionID, err)
		}
		turnPath := filepath.Join(projectDir, "sessions", "mpeters", sessionID, "context", "turns.jsonl")
		if err := os.MkdirAll(filepath.Dir(turnPath), 0o755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}
		lines := make([][]byte, 0, len(turns))
		for idx, turn := range turns {
			if turn.CreatedAt == 0 {
				turn.CreatedAt = parsed.UnixMilli() + int64(idx)
			}
			payload, err := json.Marshal(turn)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			lines = append(lines, payload)
		}
		if err := os.WriteFile(turnPath, append(bytes.Join(lines, []byte{'\n'}), '\n'), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}

	for _, sessionID := range sessionOrder {
		turns := []ChatTurn{{
			Prompt:   "prompt for " + sessionID,
			Response: "short response " + sessionID,
		}}
		if sessionID == targetSession {
			turns = []ChatTurn{
				{
					Prompt: "prompt for " + sessionID,
					Response: strings.Join([]string{
						"SENTINEL-A start",
						"SENTINEL-B marker",
						"SENTINEL-C final marker",
					}, "\n"),
				},
				{
					Prompt:   "follow-up prompt for " + sessionID,
					Response: "follow-up response for " + sessionID,
				},
			}
		}
		writeSessionTurns(sessionID, turns)
	}

	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "current-user",
	})

	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "current-session", Turns: []ChatTurn{{Prompt: "current", Response: "turn"}}}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeSection; got != "context-history" {
		t.Fatalf("expected context-history section selected, got %q", got)
	}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")
	_ = renderContextFeedbackSections(snapshot, nil, state, 120)

	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if got := state.activeRowKey(); got != "user:mpeters" {
		t.Fatalf("expected user row focus after tab enter, got %q", got)
	}

	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if got := state.activeRowKey(); got != "session:mpeters:2026-06-06 23:20:18" {
		t.Fatalf("expected first session selected after drill-in tabs, got %q", got)
	}

	targetSessionRow := "session:mpeters:" + targetSession
	for steps := 0; steps < 8 && state.activeRowKey() != targetSessionRow; steps++ {
		applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")
	}
	if got := state.activeRowKey(); got != targetSessionRow {
		t.Fatalf("expected down navigation to reach target session %q, got %q", targetSession, got)
	}
	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if got := state.activeRowKey(); got != targetSessionRow {
		t.Fatalf("expected render cycle to preserve target session selection, got %q", got)
	}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")
	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	if got := state.focusPath.Tail(); got.Kind != nodeKindSession || got.Session != targetSession {
		t.Fatalf("expected target session expansion via space, got %#v", got)
	}

	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot, "", "")
	_ = renderContextFeedbackSections(snapshot, nil, state, 120)
	promptKey := "history:mpeters:" + targetSession + ":1:prompt"
	if got := state.activeRowKey(); got != promptKey {
		t.Fatalf("expected prompt leaf row after tab into session, got %q", got)
	}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")
	responseKey := "history:mpeters:" + targetSession + ":1:response"
	if got := state.activeRowKey(); got != responseKey {
		t.Fatalf("expected down to move prompt->response leaf, got %q", got)
	}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot, "", "")
	secondPromptKey := "history:mpeters:" + targetSession + ":2:prompt"
	if got := state.activeRowKey(); got != secondPromptKey {
		t.Fatalf("expected down to move turn1 response -> turn2 prompt, got %q", got)
	}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot, "", "")
	if got := state.activeRowKey(); got != responseKey {
		t.Fatalf("expected up to move turn2 prompt -> turn1 response, got %q", got)
	}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot, "", "")
	rendered := strings.Join(renderContextFeedbackSections(snapshot, nil, state, 120), "\n")
	if got := state.activeRowKey(); got != responseKey {
		t.Fatalf("expected render cycle to preserve expanded response leaf selection, got %q", got)
	}

	for _, sentinel := range []string{"SENTINEL-A start", "SENTINEL-B marker", "SENTINEL-C final marker"} {
		if !strings.Contains(rendered, sentinel) {
			t.Fatalf("expected expanded response to include %q, render:\n%s", sentinel, rendered)
		}
	}

	applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot, "", "")
	if !state.insideSection {
		t.Fatalf("expected shift-tab from response leaf to stay inside context-history")
	}
	if got := state.activeRowKey(); got != "session:mpeters:"+targetSession {
		t.Fatalf("expected shift-tab from response leaf to step out to session row, got %q", got)
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSession || got.Session != targetSession {
		t.Fatalf("expected shift-tab from response leaf to restore session focus, got %#v", got)
	}
	if got := state.statusLine; got != "Stepped back in context-history." {
		t.Fatalf("expected shift-tab step-back status from response leaf, got %q", got)
	}
}

func TestBoxContainer_AlignsBordersWithEmojiRows(t *testing.T) {
	rows := []string{
		"👤 Prompt row",
		"🤖 Response row",
		styleToken("📑 Section marker", ansiCyan),
	}
	rendered := boxContainer(rows, ansiDim)
	assertBoxAlignment(t, rendered)
}

func TestBoxSection_AlignsBordersWithEmojiRows(t *testing.T) {
	rows := []string{
		"👤 Prompt row",
		"🤖 Response row",
	}
	rendered := boxSection("📑 CURRENT CONTEXT", rows, ansiDim)
	assertBoxAlignment(t, rendered)
}

func TestBoxSection_RenderWidthsStableAcrossLocales(t *testing.T) {
	locales := []string{"C.UTF-8", "ja_JP.UTF-8"}
	var baseline []int

	for i, locale := range locales {
		setWidgetTestEnv(t, map[string]string{
			"LANG":     locale,
			"LC_CTYPE": locale,
		})

		lines := boxSection("· TITLE 🙂", []string{"· ambiguous + 🙂 emoji row"}, ansiDim)
		widths := make([]int, len(lines))
		for j, line := range lines {
			widths[j] = renderStringWidth(stripAnsi(line))
		}

		if i == 0 {
			baseline = widths
			continue
		}

		if len(widths) != len(baseline) {
			t.Fatalf("expected %d box lines for locale %s, got %d", len(baseline), locale, len(widths))
		}
		for j := range widths {
			if widths[j] != baseline[j] {
				t.Fatalf("expected stable render width across locales at line %d: baseline=%d locale(%s)=%d", j, baseline[j], locale, widths[j])
			}
		}
	}
}

func TestFitLinesToWidth_ClipsAnsiEmojiRows(t *testing.T) {
	lines := []string{styleToken("🙂🙂🙂🙂🙂🙂🙂🙂", ansiCyan)}
	fitted := fitLinesToWidth(lines, 6)

	if len(fitted) != 1 {
		t.Fatalf("expected one fitted line, got %d", len(fitted))
	}
	if got := renderStringWidth(stripAnsi(fitted[0])); got > 6 {
		t.Fatalf("expected clipped width <= 6, got %d for %q", got, fitted[0])
	}
	if strings.IndexByte(fitted[0], '\x1b') != -1 {
		t.Fatalf("expected ANSI-clipped output to be plain text, got %q", fitted[0])
	}
}

func TestFitLinesToWidth_PreservesLeadingIndentWhenClipping(t *testing.T) {
	lines := []string{"  │   │   include details"}
	fitted := fitLinesToWidth(lines, 12)

	if len(fitted) != 1 {
		t.Fatalf("expected one fitted line, got %d", len(fitted))
	}
	if !strings.HasPrefix(fitted[0], "  ") {
		t.Fatalf("expected clipped line to preserve leading indent, got %q", fitted[0])
	}
	if !strings.Contains(fitted[0], "...") {
		t.Fatalf("expected clipped line to include truncation marker, got %q", fitted[0])
	}
	if got := renderStringWidth(fitted[0]); got > 12 {
		t.Fatalf("expected clipped width <= 12, got %d for %q", got, fitted[0])
	}
}

func assertBoxAlignment(t *testing.T, lines []string) {
	t.Helper()
	if len(lines) < 2 {
		t.Fatalf("expected boxed output with borders, got %v", lines)
	}
	top := runewidth.StringWidth(stripAnsi(lines[0]))
	for i, line := range lines[1:] {
		if got := runewidth.StringWidth(stripAnsi(line)); got != top {
			t.Fatalf("expected box line %d display width %d to match top border width %d; line=%q", i+1, got, top, line)
		}
	}
}

func TestRenderSectionHeader_ReservesPointerWhitespace(t *testing.T) {
	state := newContextFeedbackViewState()
	state.insideSection = false
	state.activeSection = "working-memory"

	active := stripAnsi(renderSectionHeader("WORKING MEMORY", "working-memory", state))
	inactive := stripAnsi(renderSectionHeader("CONTEXT HISTORY", "context-history", state))

	if !strings.HasPrefix(active, "▶ ") {
		t.Fatalf("expected active section prefix with pointer, got %q", active)
	}
	if !strings.HasPrefix(inactive, "  ") {
		t.Fatalf("expected inactive section to reserve pointer whitespace, got %q", inactive)
	}
}

func TestSectionBorderColor_ActiveSectionUsesSectionSpecificAccent(t *testing.T) {
	state := newContextFeedbackViewState()
	state.insideSection = true

	state.activeSection = "context-history"
	if got := sectionBorderColor("context-history", state); got != ansiCyan {
		t.Fatalf("expected context-history active border to be cyan, got %q", got)
	}

	state.activeSection = "working-memory"
	if got := sectionBorderColor("working-memory", state); got != ansiGreen {
		t.Fatalf("expected working-memory active border to be green, got %q", got)
	}

	state.activeSection = "current-context"
	if got := sectionBorderColor("current-context", state); got != ansiMagenta {
		t.Fatalf("expected current-context active border to be magenta, got %q", got)
	}

	state.insideSection = false
	if got := sectionBorderColor("working-memory", state); got != ansiDim {
		t.Fatalf("expected outside-section border to be dim, got %q", got)
	}
}

func TestRenderContextFeedbackSections_SectionSpecificActiveBorderColorsAppearInRenderedOutput(t *testing.T) {
	state := newContextFeedbackViewState()
	state.insideSection = true

	snapshot := contextWidgetSnapshot{SessionID: "sess-colors", Turns: []ChatTurn{{Prompt: "p1", Response: "r1"}}}

	state.activeSection = "context-history"
	state.collapsedContextHistory = false
	state.updateOrderedRows([]string{"user:mpeters"})
	renderedHistory := strings.Join(renderContextFeedbackSections(snapshot, nil, state, 120), "\n")
	if !strings.Contains(renderedHistory, ansiCyan) {
		t.Fatalf("expected context-history active border rendered with cyan accent in output")
	}

	state.activeSection = "working-memory"
	state.collapsedWorkingMemory = false
	renderedWM := strings.Join(renderContextFeedbackSections(snapshot, nil, state, 120), "\n")
	if !strings.Contains(renderedWM, ansiGreen) {
		t.Fatalf("expected working-memory active border rendered with green accent in output")
	}

	state.activeSection = "current-context"
	state.collapsedCurrentContext = false
	renderedCurrent := strings.Join(renderContextFeedbackSections(snapshot, nil, state, 120), "\n")
	if !strings.Contains(renderedCurrent, ansiMagenta) {
		t.Fatalf("expected current-context active border rendered with magenta accent in output")
	}

	state.insideSection = false
	renderedOutside := strings.Join(renderContextFeedbackSections(snapshot, nil, state, 120), "\n")
	if !strings.Contains(renderedOutside, ansiDim) {
		t.Fatalf("expected outside-section borders rendered with dim accent in output")
	}
}

func TestRenderContextFeedbackSections_FiltersEmptyTurnStubs(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{
		SessionID: "sess-empty-turn",
		Turns:     []ChatTurn{{Prompt: "", Response: ""}},
	}

	rendered := strings.Join(renderContextFeedbackSections(snapshot, nil, state, 120), "\n")
	if strings.Contains(rendered, "TURN 1") {
		t.Fatalf("expected empty turn placeholder to be filtered out, got:\n%s", rendered)
	}
	// Empty context renders a box stub, not a freeform text message.
	if strings.Contains(rendered, "No current context elements.") {
		t.Fatalf("expected empty context to render box stub not text message, got:\n%s", rendered)
	}
	if strings.Count(rendered, "┌") < 2 || strings.Count(rendered, "└") < 2 {
		t.Fatalf("expected box stubs in output for empty context, got:\n%s", rendered)
	}
}

func TestRenderContextFeedbackSections_HistoryIncludeArtifactIsConcise(t *testing.T) {
	projectDir := t.TempDir()
	turnPath := filepath.Join(projectDir, "sessions", "mpeters", "s-prev", "context", "turns.jsonl")
	if err := os.MkdirAll(filepath.Dir(turnPath), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	payload, err := json.Marshal(ChatTurn{Prompt: "hello", Response: "world", CreatedAt: 1_700_000_000_000})
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := os.WriteFile(turnPath, append(payload, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	setWidgetTestEnv(t, map[string]string{
		"AGENTX_PROJECT_DIR": projectDir,
		"AGENTX_USERNAME":    "current-user",
	})

	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.collapsedContextHistory = false
	state.setFocusPath(focusPath{
		{Kind: nodeKindSection, Section: "context-history"},
		{Kind: nodeKindUser, User: "mpeters"},
		{Kind: nodeKindSession, User: "mpeters", Session: "s-prev"},
	})

	rendered := stripAnsi(strings.Join(renderContextFeedbackSections(contextWidgetSnapshot{SessionID: "current-session"}, nil, state, 120), "\n"))
	if !strings.Contains(rendered, "include") {
		t.Fatalf("expected concise include hint line to render, got:\n%s", rendered)
	}
	if count := strings.Count(rendered, "include"); count != 1 {
		t.Fatalf("expected exactly one include hint artifact, got %d in:\n%s", count, rendered)
	}
	if strings.Contains(rendered, "include: i ") || strings.Contains(rendered, " 1 b") {
		t.Fatalf("expected include line not to render command token artifact, got:\n%s", rendered)
	}
}

func TestRenderWorkingMemoryFeedbackSection_ShowsEditorCells(t *testing.T) {
	state := newContextFeedbackViewState()
	state.collapsedWorkingMemory = false

	rendered := strings.Join(renderWorkingMemoryFeedbackSection(state, nil, 120), "\n")
	// Editor cells appear inside the single WM box.
	for _, fragment := range []string{
		"KEY                       VALUE",
		"┌───────────────────────┐ ┌───────────────────────┐ ┌─────┐",
		"│ ↳OK │",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("expected working-memory editor scaffold fragment %q, got:\n%s", fragment, rendered)
		}
	}
	// Facts appear in KEY: VALUE format with emoji state icons, not legacy [enabled]/[disabled].
	if strings.Contains(rendered, "[enabled]") || strings.Contains(rendered, "[disabled]") {
		t.Fatalf("expected no legacy [enabled]/[disabled] text in working memory, got:\n%s", rendered)
	}
	// No separate FACTS or WM EDITOR sub-section titles.
	if strings.Contains(stripAnsi(rendered), "FACTS") || strings.Contains(stripAnsi(rendered), "WM EDITOR") {
		t.Fatalf("expected no FACTS/WM EDITOR sub-section titles in working memory, got:\n%s", rendered)
	}
}
