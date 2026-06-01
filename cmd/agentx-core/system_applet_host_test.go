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