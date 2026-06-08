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
	initialCollapsed := state.collapsedWorkingMemory

	applyContextWidgetCommand(state, "m", "http://127.0.0.1:0", snapshot)
	if state.collapsedWorkingMemory == initialCollapsed {
		t.Fatalf("expected working memory collapse state to toggle with 'm'")
	}

	applyContextWidgetCommand(state, "m show", "http://127.0.0.1:0", snapshot)
	if !state.showWorkingMemory || state.collapsedWorkingMemory {
		t.Fatalf("expected working memory section visible and expanded after 'm show'")
	}
}

func TestContextWidgetKeyboard_SpaceSelectAndEnterCollapse(t *testing.T) {
	state := newContextFeedbackViewState()
	// Must be inside a section for SPACE to select rows (outside SPACE collapses the section).
	state.insideSection = true
	state.activeSection = "current-context"
	state.updateOrderedRows([]string{"current:1:prompt"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys", Turns: []ChatTurn{{Prompt: "p1", Response: "r1"}}}

	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot)
	if !state.selectedEntries["current:1:prompt"] {
		t.Fatalf("expected row to be selected by space key when inside section")
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot)
	if !state.collapsedEntries["current:1:prompt"] {
		t.Fatalf("expected row to be collapsed by enter key")
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

	// First TAB enters the active section.
	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot)
	if !state.insideSection {
		t.Fatalf("expected first tab to enter section (insideSection=true)")
	}
	if got := state.statusLine; got != "Entered section: current-context." {
		t.Fatalf("expected normalized tab enter status, got %q", got)
	}

	// Second TAB remains inside the section (drill-in semantics).
	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot)
	if !state.insideSection {
		t.Fatalf("expected second tab to keep section focus (insideSection=true)")
	}
	if got := state.statusLine; got != "Entered section: current-context." {
		t.Fatalf("expected normalized repeated tab status, got %q", got)
	}

	// Shift-Tab exits the section.
	applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot)
	if state.insideSection {
		t.Fatalf("expected shift-tab to exit section (insideSection=false)")
	}
	if got := state.statusLine; got != "Exited section: current-context." {
		t.Fatalf("expected normalized shift-tab status, got %q", got)
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
	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot)
	if state.collapsedContextHistory == initiallyCollapsed {
		t.Fatalf("expected SPACE to toggle context-history collapsed state")
	}

	// Second SPACE collapses it again.
	applyContextWidgetCommand(state, "space", "http://127.0.0.1:0", snapshot)
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

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot)
	if state.activeSection != "working-memory" {
		t.Fatalf("expected up to move to working-memory, got %q", state.activeSection)
	}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot)
	if state.activeSection != "context-history" {
		t.Fatalf("expected second up to move to context-history, got %q", state.activeSection)
	}

	// Up at the top should stay at context-history (clamp at start).
	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot)
	if state.activeSection != "context-history" {
		t.Fatalf("expected up at top to stay at context-history, got %q", state.activeSection)
	}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot)
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

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot)
	if got := state.statusLine; got != "Selection moved." {
		t.Fatalf("expected normalized selection-moved status, got %q", got)
	}

	applyContextWidgetCommand(state, "down", "http://127.0.0.1:0", snapshot)
	if got := state.statusLine; got != "Selection at last row." {
		t.Fatalf("expected normalized lower-bound status, got %q", got)
	}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot)
	if got := state.statusLine; got != "Selection moved." {
		t.Fatalf("expected normalized selection-moved status after up, got %q", got)
	}

	applyContextWidgetCommand(state, "up", "http://127.0.0.1:0", snapshot)
	if got := state.statusLine; got != "Selection at first row." {
		t.Fatalf("expected normalized upper-bound status, got %q", got)
	}
}

func TestContextWidgetKeyboard_ViewportStatusVocabulary(t *testing.T) {
	state := newContextFeedbackViewState()
	state.insideSection = true
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	state.activeSection = "context-history"
	applyContextWidgetCommand(state, "pgdn", "http://127.0.0.1:0", snapshot)
	if got := state.statusLine; got != "Viewport moved down: context history." {
		t.Fatalf("expected normalized context-history pgdn viewport status, got %q", got)
	}

	applyContextWidgetCommand(state, "pgup", "http://127.0.0.1:0", snapshot)
	if got := state.statusLine; got != "Viewport moved up: context history." {
		t.Fatalf("expected normalized context-history pgup viewport status, got %q", got)
	}

	state.activeSection = "working-memory"
	applyContextWidgetCommand(state, "pgdn", "http://127.0.0.1:0", snapshot)
	if got := state.statusLine; got != "Viewport moved down: working memory." {
		t.Fatalf("expected normalized working-memory pgdn viewport status, got %q", got)
	}

	applyContextWidgetCommand(state, "pgup", "http://127.0.0.1:0", snapshot)
	if got := state.statusLine; got != "Viewport moved up: working memory." {
		t.Fatalf("expected normalized working-memory pgup viewport status, got %q", got)
	}
}

func TestContextWidgetKeyboard_EnterTabShiftTabTransitionVocabulary(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot)
	if got := state.statusLine; got != "Entered section: current-context." {
		t.Fatalf("expected enter transition status, got %q", got)
	}

	state.insideSection = false
	applyContextWidgetCommand(state, "tab", "http://127.0.0.1:0", snapshot)
	if got := state.statusLine; got != "Entered section: current-context." {
		t.Fatalf("expected tab transition status to match enter, got %q", got)
	}

	applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot)
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

	applyContextWidgetCommand(state, "pgdn", "http://127.0.0.1:0", snapshot)
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

func TestContextWidgetKeyboard_EnterHistoryUsesFocusPath(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.insideSection = true
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-prev"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot)
	if got := state.focusPath.Tail(); got.Kind != nodeKindSection || got.Section != "context-history" {
		t.Fatalf("expected enter on focused user row to collapse to section, got %#v", got)
	}
	if !state.insideSection {
		t.Fatalf("expected collapse to keep section navigation active")
	}
	if got := state.statusLine; got != "History node collapsed." {
		t.Fatalf("expected normalized collapse status, got %q", got)
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot)
	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != "mpeters" {
		t.Fatalf("expected second enter on user row to expand user node without re-entry step, got %#v", got)
	}
	if got := state.statusLine; got != "History node expanded." {
		t.Fatalf("expected normalized expand status, got %q", got)
	}

	if !state.moveRowInActiveSection(1) {
		t.Fatalf("expected move to session row to succeed")
	}
	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot)
	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != "mpeters" {
		t.Fatalf("expected enter on focused session row to collapse to parent user, got %#v", got)
	}
	if got := state.statusLine; got != "History node collapsed." {
		t.Fatalf("expected normalized collapse status from session row, got %q", got)
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot)
	if got := state.focusPath.Tail(); got.Kind != nodeKindSession || got.Session != "s-prev" {
		t.Fatalf("expected second enter on session row to focus session node, got %#v", got)
	}
	if got := state.statusLine; got != "History node expanded." {
		t.Fatalf("expected normalized expand status for session row, got %q", got)
	}

	applyContextWidgetCommand(state, "enter", "http://127.0.0.1:0", snapshot)
	if got := state.focusPath.Tail(); got.Kind != nodeKindUser || got.User != "mpeters" {
		t.Fatalf("expected second enter on focused session to collapse to parent user, got %#v", got)
	}
	if got := state.statusLine; got != "History node collapsed." {
		t.Fatalf("expected normalized collapse status for focused session, got %q", got)
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

func TestMoveRowInActiveSection_HistorySiblingSwitchReplacesLeafPath(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.insideSection = true
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-1", "session:mpeters:s-2"})

	if !state.moveRowInActiveSection(1) {
		t.Fatalf("expected move to first session row to succeed")
	}
	sessionOne := nodeID{Kind: nodeKindSession, User: "mpeters", Session: "s-1"}
	if !focusPathHasNode(state.focusPath, sessionOne) {
		t.Fatalf("expected focus path to include first session node")
	}

	if !state.moveRowInActiveSection(1) {
		t.Fatalf("expected move to sibling session row to succeed")
	}
	if focusPathHasNode(state.focusPath, sessionOne) {
		t.Fatalf("expected sibling move to replace old session leaf path")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSession || got.Session != "s-2" {
		t.Fatalf("expected focus path tail to track second session, got %#v", got)
	}
}

func TestContextWidgetKeyboard_ShiftTabPopsDeepHistoryPath(t *testing.T) {
	state := newContextFeedbackViewState()
	state.activeSection = "context-history"
	state.insideSection = true
	state.collapsedContextHistory = false
	state.updateOrderedRows([]string{"user:mpeters", "session:mpeters:s-1", "history:mpeters:s-1:1"})
	snapshot := contextWidgetSnapshot{SessionID: "sess-keys"}

	if !state.moveRowInActiveSection(2) {
		t.Fatalf("expected move to turn row to succeed")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindTurn {
		t.Fatalf("expected deep history focus before shift-tab, got %#v", got)
	}

	applyContextWidgetCommand(state, "shift-tab", "http://127.0.0.1:0", snapshot)
	if state.insideSection {
		t.Fatalf("expected shift-tab to exit section")
	}
	if !state.collapsedContextHistory {
		t.Fatalf("expected shift-tab to collapse context-history section")
	}
	if got := state.focusPath.Tail(); got.Kind != nodeKindSession || got.Session != "s-1" {
		t.Fatalf("expected shift-tab to pop focus path to parent session node, got %#v", got)
	}
	if got := state.statusLine; got != "Exited section: context-history." {
		t.Fatalf("expected normalized shift-tab status in deep history path, got %q", got)
	}
}

func TestRenderContextFeedbackSections_CollapsedSectionsRenderBoxStubs(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{SessionID: "sess-collapsed"}

	rendered := strings.Join(renderContextFeedbackSections(snapshot, nil, state), "\n")
	if strings.Count(rendered, "┌") < 2 || strings.Count(rendered, "└") < 2 {
		t.Fatalf("expected collapsed sections to include visible box stubs, got:\n%s", rendered)
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

func TestRenderContextFeedbackSections_FiltersEmptyTurnStubs(t *testing.T) {
	state := newContextFeedbackViewState()
	snapshot := contextWidgetSnapshot{
		SessionID: "sess-empty-turn",
		Turns: []ChatTurn{{Prompt: "", Response: ""}},
	}

	rendered := strings.Join(renderContextFeedbackSections(snapshot, nil, state), "\n")
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

func TestRenderWorkingMemoryFeedbackSection_ShowsEditorCells(t *testing.T) {
	state := newContextFeedbackViewState()
	state.collapsedWorkingMemory = false

	rendered := strings.Join(renderWorkingMemoryFeedbackSection(state, nil), "\n")
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
