package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRenderSystemSurface_ContextVisualizerContract(t *testing.T) {
	projectDir := t.TempDir()
	core := NewAgentXCore(&Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-render-contract"})
	core.runtimeConfig.OllamaModel = "qwen3.6:latest"
	core.runtimeConfig.ChatBackend = "ollama"
	core.runtimeConfig.OllamaHost = "localhost:11434"

	if err := core.contextManager.RecordTurn("hello world", "hi there"); err != nil {
		t.Fatalf("RecordTurn failed: %v", err)
	}

	render := core.renderSystemSurface("context-visualizer")
	required := []string{
		"[SYSTEM TAB] active=context-visualizer",
		"== CONTEXT WINDOW ==",
		"consumed:",
		"Top Contributors:",
		"== CONTEXT VISUALIZER ==",
		"💾 Working Memory",
		"🧠 System Prompts",
		"👤 User Prompts",
		"📎 Attachments",
		"🤔 Thinking",
		"🤖 Agent Response",
		"🔧 Tool Calls",
		"░ Remaining",
		"== PROMPT CYCLE ==",
		"🤖 Respond",
	}
	for _, fragment := range required {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
	if strings.Contains(render, "== SESSION SNAPSHOT ==") {
		t.Fatalf("render should not include legacy session snapshot block:\n%s", render)
	}
	if strings.Contains(render, "/1)") {
		t.Fatalf("render should not include invalid denominator /1:\n%s", render)
	}
}

func TestRenderSystemSurface_TabContracts(t *testing.T) {
	projectDir := t.TempDir()
	core := NewAgentXCore(&Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-render-tabs"})
	core.runtimeConfig.OllamaModel = "qwen3.6:latest"
	core.runtimeConfig.ChatBackend = "ollama"
	core.runtimeConfig.OllamaHost = "localhost:11434"

	if err := core.contextManager.RecordTurn("system panel tour", "ack"); err != nil {
		t.Fatalf("RecordTurn failed: %v", err)
	}

	cases := []struct {
		tab      string
		required []string
		banned   []string
	}{
		{
			tab: "files",
			required: []string{"== FILES ==", "project_dir:", "entry_count:", "preview:"},
			banned:   []string{"== CONFIGURATION ==", "== CONTEXT ==", "== CONTEXT HISTORY ==", "== CONTEXT VISUALIZER =="},
		},
		{
			tab: "configuration",
			required: []string{"== CONFIGURATION ==", "model:", "backend:", "ollama_host:"},
			banned:   []string{"== FILES ==", "== CONTEXT ==", "== CONTEXT HISTORY ==", "== CONTEXT VISUALIZER =="},
		},
		{
			tab: "context",
			required: []string{"== CONTEXT ==", "session_id:", "last_user: system panel tour"},
			banned:   []string{"== FILES ==", "== CONFIGURATION ==", "== CONTEXT HISTORY ==", "== CONTEXT VISUALIZER =="},
		},
		{
			tab: "context-history",
			required: []string{"== CONTEXT HISTORY ==", "history_context_count:", "recent_prompt:", "recent_response:"},
			banned:   []string{"== FILES ==", "== CONFIGURATION ==", "== CONTEXT ==", "== CONTEXT VISUALIZER =="},
		},
		{
			tab: "working-memory",
			required: []string{"== WORKING MEMORY ==", "fact_count:"},
			banned:   []string{"== FILES ==", "== CONFIGURATION ==", "== CONTEXT ==", "== CONTEXT HISTORY ==", "== CONTEXT VISUALIZER =="},
		},
	}

	for _, tc := range cases {
		render := core.renderSystemSurface(tc.tab)
		for _, fragment := range tc.required {
			if !strings.Contains(render, fragment) {
				t.Fatalf("tab %q expected %q in render:\n%s", tc.tab, fragment, render)
			}
		}
		for _, fragment := range tc.banned {
			if strings.Contains(render, fragment) {
				t.Fatalf("tab %q should not include %q:\n%s", tc.tab, fragment, render)
			}
		}
	}
}

func TestResolveSelectedSystemTab_StateFile(t *testing.T) {
	projectDir := t.TempDir()
	core := NewAgentXCore(&Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-tab-state"})

	if got := core.resolveSelectedSystemTab(); got != "full" {
		t.Fatalf("expected default tab full, got %q", got)
	}

	stateDir := filepath.Join(projectDir, ".agentx")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "system-panel-tab.txt"), []byte("context_visualizer\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if got := core.resolveSelectedSystemTab(); got != "context-visualizer" {
		t.Fatalf("expected alias to resolve to context-visualizer, got %q", got)
	}
}

func TestResolveSelectedSystemTab_InvalidStateFallsBackToFull(t *testing.T) {
	projectDir := t.TempDir()
	core := NewAgentXCore(&Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-tab-invalid"})

	stateDir := filepath.Join(projectDir, ".agentx")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "system-panel-tab.txt"), []byte("unknown-tab\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if got := core.resolveSelectedSystemTab(); got != "full" {
		t.Fatalf("expected invalid tab state to fall back to full, got %q", got)
	}
}

func TestRenderSystemSurface_TabSwitchStableAcrossTurns(t *testing.T) {
	projectDir := t.TempDir()
	core := NewAgentXCore(&Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-tab-switch-stable"})
	core.runtimeConfig.OllamaModel = "qwen3.6:latest"
	core.runtimeConfig.ChatBackend = "ollama"
	core.runtimeConfig.OllamaHost = "localhost:11434"

	turns := []struct {
		prompt   string
		response string
	}{
		{prompt: "first prompt", response: "first response"},
		{prompt: "second prompt", response: "second response"},
		{prompt: "third prompt", response: "third response"},
	}
	for _, turn := range turns {
		if err := core.contextManager.RecordTurn(turn.prompt, turn.response); err != nil {
			t.Fatalf("RecordTurn failed: %v", err)
		}
	}

	tabChecks := []struct {
		tab      string
		required []string
		banned   []string
	}{
		{
			tab:      "files",
			required: []string{"[SYSTEM TAB] active=files", "== FILES ==", "project_dir:", "entry_count:", "preview:"},
			banned:   []string{"== CONFIGURATION ==", "== CONTEXT ==", "== CONTEXT HISTORY ==", "== CONTEXT VISUALIZER =="},
		},
		{
			tab:      "context",
			required: []string{"[SYSTEM TAB] active=context", "== CONTEXT ==", "last_user: third prompt"},
			banned:   []string{"== FILES ==", "== CONFIGURATION ==", "== CONTEXT HISTORY ==", "== CONTEXT VISUALIZER =="},
		},
		{
			tab:      "context-history",
			required: []string{"[SYSTEM TAB] active=context-history", "== CONTEXT HISTORY ==", "history_context_count: 3", "recent_prompt: second prompt", "recent_response: second response"},
			banned:   []string{"== FILES ==", "== CONFIGURATION ==", "== CONTEXT ==", "== CONTEXT VISUALIZER =="},
		},
		{
			tab:      "working-memory",
			required: []string{"[SYSTEM TAB] active=working-memory", "== WORKING MEMORY ==", "fact_count:"},
			banned:   []string{"== FILES ==", "== CONFIGURATION ==", "== CONTEXT ==", "== CONTEXT HISTORY ==", "== CONTEXT VISUALIZER =="},
		},
		{
			tab:      "context-visualizer",
			required: []string{"[SYSTEM TAB] active=context-visualizer", "== CONTEXT WINDOW ==", "consumed:", "Top Contributors:", "== PROMPT CYCLE ==", "🤖 Respond"},
			banned:   []string{"== FILES ==", "== CONFIGURATION ==", "== CONTEXT ==", "== CONTEXT HISTORY =="},
		},
		{
			tab:      "configuration",
			required: []string{"[SYSTEM TAB] active=configuration", "== CONFIGURATION ==", "model:", "backend:", "ollama_host:"},
			banned:   []string{"== FILES ==", "== CONTEXT ==", "== CONTEXT HISTORY ==", "== CONTEXT VISUALIZER =="},
		},
	}

	for _, tc := range tabChecks {
		render := core.renderSystemSurface(tc.tab)
		for _, fragment := range tc.required {
			if !strings.Contains(render, fragment) {
				t.Fatalf("tab %q expected %q in render:\n%s", tc.tab, fragment, render)
			}
		}
		for _, fragment := range tc.banned {
			if strings.Contains(render, fragment) {
				t.Fatalf("tab %q should not include %q:\n%s", tc.tab, fragment, render)
			}
		}
	}
}

func TestEmitSystemPanelRender_GoContextUsesStateFileTab(t *testing.T) {
	logPath := setupFakeTmux(t)
	projectDir := t.TempDir()
	core := NewAgentXCore(&Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-emit-system-state"})

	core.mu.Lock()
	core.applets["context"] = &AppletProcess{Name: "context", PaneName: "context", Runtime: appletRuntimeGo, Status: AppletStatusReady}
	core.mu.Unlock()

	stateDir := filepath.Join(projectDir, ".agentx")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "system-panel-tab.txt"), []byte("context-history\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := core.emitSystemPanelRender(context.Background()); err != nil {
		t.Fatalf("emitSystemPanelRender failed: %v", err)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "send-keys -t "+core.tmuxSessionName+":0.1") {
		t.Fatalf("expected system pane send-keys target, got:\n%s", commands)
	}
	if !strings.Contains(commands, "[SYSTEM TAB] active=context-history") {
		t.Fatalf("expected state-file-selected tab in render command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "== CONTEXT HISTORY ==") {
		t.Fatalf("expected context-history section in render command, got:\n%s", commands)
	}
}

func TestClipRenderToPaneHeight_TruncatesAndKeepsTail(t *testing.T) {
	lines := []string{}
	for i := 1; i <= 12; i++ {
		lines = append(lines, "line "+strconv.Itoa(i))
	}

	render := clipRenderToPaneHeight(strings.Join(lines, "\n"), 7)
	if !strings.Contains(render, "line 1") {
		t.Fatalf("expected clipped render to keep head lines, got:\n%s", render)
	}
	if !strings.Contains(render, "... (") {
		t.Fatalf("expected clipped render marker, got:\n%s", render)
	}
	if !strings.Contains(render, "line 11") || !strings.Contains(render, "line 12") {
		t.Fatalf("expected clipped render to keep tail lines, got:\n%s", render)
	}
}

func TestEmitSystemPanelRender_SkipsUnchangedRedraw(t *testing.T) {
	logPath := setupFakeTmux(t)
	projectDir := t.TempDir()
	core := NewAgentXCore(&Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-emit-system-dedupe"})

	core.mu.Lock()
	core.applets["context"] = &AppletProcess{Name: "context", PaneName: "context", Runtime: appletRuntimeGo, Status: AppletStatusReady}
	core.mu.Unlock()

	if err := core.emitSystemPanelRender(context.Background()); err != nil {
		t.Fatalf("first emitSystemPanelRender failed: %v", err)
	}
	if err := core.emitSystemPanelRender(context.Background()); err != nil {
		t.Fatalf("second emitSystemPanelRender failed: %v", err)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	target := "send-keys -t " + core.tmuxSessionName + ":0.1"
	if got := strings.Count(commands, target); got != 1 {
		t.Fatalf("expected exactly one system render send-keys for unchanged payload, got %d\ncommands:\n%s", got, commands)
	}
}
