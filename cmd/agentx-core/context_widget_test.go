package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
			Turns: []ChatTurn{{Prompt: "hello", Response: "world"}},
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

func TestFetchContextWidgetSnapshot_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/context" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(contextWidgetSnapshot{
			SessionID: "sess-2",
			TurnCount: 1,
			Turns: []ChatTurn{{Prompt: "p", Response: "r"}},
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
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "alpha.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.Mkdir(filepath.Join(projectDir, "beta"), 0o755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	oldProjectDir, hadProjectDir := os.LookupEnv("AGENTX_PROJECT_DIR")
	defer func() {
		if hadProjectDir {
			_ = os.Setenv("AGENTX_PROJECT_DIR", oldProjectDir)
		} else {
			_ = os.Unsetenv("AGENTX_PROJECT_DIR")
		}
	}()
	if err := os.Setenv("AGENTX_PROJECT_DIR", projectDir); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}

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
	oldHost, hadHost := os.LookupEnv("AGENTX_OLLAMA_HOST")
	defer func() {
		if hadHost {
			_ = os.Setenv("AGENTX_OLLAMA_HOST", oldHost)
		} else {
			_ = os.Unsetenv("AGENTX_OLLAMA_HOST")
		}
	}()
	if err := os.Setenv("AGENTX_OLLAMA_HOST", "localhost:11434"); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}

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
