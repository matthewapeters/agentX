package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchOutputWidgetSnapshot_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/context" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(contextWidgetSnapshot{
			SessionID: "sess-output",
			TurnCount: 1,
			Turns: []ChatTurn{{Prompt: "hello", Response: "world"}},
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
		Turns: []ChatTurn{{Prompt: "what is 2+2?", Response: "Echo: what is 2+2?"}},
		PromptCycle: PromptCycleStatus{
			Thinking: PromptCyclePhase{State: "done", ElapsedMs: 11},
		},
	}, 80, 200)

	for _, fragment := range []string{
		"[OUTPUT]",
		"Chat ready.",
		"User: what is 2+2?",
		"⚙️ Classification:",
		"Thinking:",
		"💭 [thinking block - done (00:00:00.011)]",
		"Response: Echo: what is 2+2?",
		"Agent: Echo: what is 2+2?",
	} {
		if !strings.Contains(render, fragment) {
			t.Fatalf("expected render to contain %q, got:\n%s", fragment, render)
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
			Turns: []ChatTurn{{Prompt: "hello", Response: "Echo: hello"}},
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
	if !strings.Contains(widgetOutput, "Agent: Echo: hello") {
		t.Fatalf("expected rendered agent response in output widget, got:\n%s", widgetOutput)
	}
}
