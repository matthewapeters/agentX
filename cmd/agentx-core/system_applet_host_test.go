package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemAppletHost_ResolvesContextHistory(t *testing.T) {
	host := newSystemAppletHost()
	applet, ok := host.Resolve("context-history")
	if !ok {
		t.Fatalf("expected context-history applet to resolve")
	}
	if applet.ID() != "context-history" {
		t.Fatalf("expected context-history id, got %q", applet.ID())
	}
}

func TestSystemAppletHost_ResolvesWorkingMemory(t *testing.T) {
	host := newSystemAppletHost()
	applet, ok := host.Resolve("working-memory")
	if !ok {
		t.Fatalf("expected working-memory applet to resolve")
	}
	if applet.ID() != "working-memory" {
		t.Fatalf("expected working-memory id, got %q", applet.ID())
	}
}

func TestSystemAppletHost_ResolvesFiles(t *testing.T) {
	host := newSystemAppletHost()
	applet, ok := host.Resolve("files")
	if !ok {
		t.Fatalf("expected files applet to resolve")
	}
	if applet.ID() != "files" {
		t.Fatalf("expected files id, got %q", applet.ID())
	}
}

func TestSystemAppletHost_ResolvesConfiguration(t *testing.T) {
	host := newSystemAppletHost()
	applet, ok := host.Resolve("configuration")
	if !ok {
		t.Fatalf("expected configuration applet to resolve")
	}
	if applet.ID() != "configuration" {
		t.Fatalf("expected configuration id, got %q", applet.ID())
	}
}

func TestContextHistorySystemApplet_RenderCore(t *testing.T) {
	host := newSystemAppletHost()
	applet, ok := host.Resolve("context-history")
	if !ok {
		t.Fatalf("expected context-history applet to resolve")
	}
	lines := applet.RenderCore(SystemAppletCoreContext{
		SessionID: "sess-1",
		TurnCount: 3,
		Turns: []ChatTurn{
			{Prompt: "first prompt", Response: "first response"},
			{Prompt: "second prompt", Response: "second response"},
			{Prompt: "third prompt", Response: "third response"},
		},
	})
	render := strings.Join(lines, "\n")
	for _, fragment := range []string{"== CONTEXT HISTORY ==", "history_context_count: 3", "recent_prompt: second prompt", "recent_response: second response"} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
}

func TestWorkingMemorySystemApplet_RenderCore(t *testing.T) {
	sessionDir := t.TempDir()
	payload := map[string]workingMemoryFactSnapshot{
		"user:project": {Owner: "user", Key: "project", Value: "AgentX", Enabled: true},
		"agent:task":   {Owner: "agent", Key: "task", Value: "coding", Enabled: false},
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "working_memory.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	applet := workingMemorySystemApplet{}
	render := strings.Join(applet.RenderCore(SystemAppletCoreContext{SessionDir: sessionDir, SessionID: "sess-1"}), "\n")
	for _, fragment := range []string{"== WORKING MEMORY ==", "session_id: sess-1", "fact_count: 2", "enabled_fact_count: 1", "👤 project [enabled] = AgentX", "🤖 task [disabled] = coding"} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
}

func TestWorkingMemorySystemApplet_RenderCoreEmptyState(t *testing.T) {
	applet := workingMemorySystemApplet{}
	render := strings.Join(applet.RenderCore(SystemAppletCoreContext{SessionDir: t.TempDir(), SessionID: "sess-empty"}), "\n")
	for _, fragment := range []string{"== WORKING MEMORY ==", "fact_count: 0", "No facts stored yet."} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
}

func TestFilesSystemApplet_RenderCoreContractAndPreview(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "alpha.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.Mkdir(filepath.Join(projectDir, "beta"), 0o755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	applet := filesSystemApplet{}
	render := strings.Join(applet.RenderCore(SystemAppletCoreContext{ProjectDir: projectDir}), "\n")
	for _, fragment := range []string{"== FILES ==", "project_dir:", "entry_count: 2", "preview:", "- dir: beta", "- file: alpha.txt"} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
}

func TestFilesSystemApplet_RenderWidgetContractCompact(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "alpha.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	applet := filesSystemApplet{}
	render := strings.Join(applet.RenderWidget(SystemAppletWidgetContext{ProjectDir: projectDir}), "\n")
	for _, fragment := range []string{"== FILES ==", "project_dir:", "entry_count: 1"} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
		}
	}
	if strings.Contains(render, "preview:") {
		t.Fatalf("widget render should not include preview rows, got:\n%s", render)
	}
}

func TestConfigurationSystemApplet_RenderContracts(t *testing.T) {
	applet := configurationSystemApplet{}
	coreRender := strings.Join(applet.RenderCore(SystemAppletCoreContext{
		Model:      "qwen3.6:latest",
		Backend:    "ollama",
		OllamaHost: "localhost:11434",
	}), "\n")
	for _, fragment := range []string{"== CONFIGURATION ==", "model: qwen3.6:latest", "backend: ollama", "ollama_host: localhost:11434"} {
		if !strings.Contains(coreRender, fragment) {
			t.Fatalf("expected core render to contain %q, got:\n%s", fragment, coreRender)
		}
	}

	widgetRender := strings.Join(applet.RenderWidget(SystemAppletWidgetContext{
		Model:      "qwen3.6:latest",
		Backend:    "ollama",
		OllamaHost: "localhost:11434",
	}), "\n")
	for _, fragment := range []string{"== CONFIGURATION ==", "model: qwen3.6:latest", "backend: ollama", "ollama_host: localhost:11434"} {
		if !strings.Contains(widgetRender, fragment) {
			t.Fatalf("expected widget render to contain %q, got:\n%s", fragment, widgetRender)
		}
	}
}