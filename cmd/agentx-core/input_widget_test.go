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

func TestSubmitPromptToCore_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}
		if req.Prompt != "hello" {
			t.Fatalf("expected prompt hello, got %q", req.Prompt)
		}

		_ = json.NewEncoder(w).Encode(submitResponse{Response: "Echo: hello"})
	}))
	defer server.Close()

	response, err := submitPromptToCore(context.Background(), server.URL, "hello")
	if err != nil {
		t.Fatalf("submitPromptToCore returned error: %v", err)
	}
	if response != "Echo: hello" {
		t.Fatalf("expected response Echo: hello, got %q", response)
	}
}

func TestRunInputWidgetCommand_SubmitsAndQuits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}

		response := "ok"
		if req.Prompt == ":q" {
			response = "quit"
		}
		_ = json.NewEncoder(w).Encode(submitResponse{Response: response})
	}))
	defer server.Close()

	input := bytes.NewBufferString("hello\n:q\n")
	output := &bytes.Buffer{}

	exitCode := runInputWidgetCommand(server.URL, input, output)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}

	widgetOutput := output.String()
	if !strings.Contains(widgetOutput, "\x1b[1A\x1b[2K\r") {
		t.Fatalf("expected input clear escape sequence before submit feedback, got output:\n%s", widgetOutput)
	}
	if strings.Contains(widgetOutput, "Submitted.") {
		t.Fatalf("did not expect submitted status line in input pane, got output:\n%s", widgetOutput)
	}
	if !strings.Contains(widgetOutput, "Session shutdown requested.") {
		t.Fatalf("expected shutdown requested status, got output:\n%s", widgetOutput)
	}
}

func TestFetchWidgetActivitySnapshot_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/activity" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(widgetActivitySnapshot{SessionID: "sess-1", State: "working", Phase: "thinking"})
	}))
	defer server.Close()

	snapshot, err := fetchWidgetActivitySnapshot(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetchWidgetActivitySnapshot returned error: %v", err)
	}
	if snapshot.SessionID != "sess-1" {
		t.Fatalf("expected session_id sess-1, got %q", snapshot.SessionID)
	}
	if snapshot.State != "working" {
		t.Fatalf("expected state working, got %q", snapshot.State)
	}
	if snapshot.Phase != "thinking" {
		t.Fatalf("expected phase thinking, got %q", snapshot.Phase)
	}
}

func TestWidgetActivityState_PromptLabelTransitions(t *testing.T) {
	state := newWidgetActivityState()

	if got := state.promptLabel(); got != "agentx" {
		t.Fatalf("expected default prompt label agentx, got %q", got)
	}

	state.update(widgetActivitySnapshot{State: "working", Phase: "tool"})
	if got := state.promptLabel(); got != "agentx[tool]" {
		t.Fatalf("expected working prompt label, got %q", got)
	}

	state.update(widgetActivitySnapshot{State: "completed", Phase: "none"})
	if got := state.promptLabel(); got != "agentx[done]" {
		t.Fatalf("expected done prompt label, got %q", got)
	}

	time.Sleep(1300 * time.Millisecond)
	if got := state.promptLabel(); got != "agentx" {
		t.Fatalf("expected prompt label to return to agentx, got %q", got)
	}
}

func TestRunInputWidgetCommand_ClearDoesNotPrintClearedLine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/submit" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var req submitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode submit request: %v", err)
		}

		response := "ok"
		if req.Prompt == ":clear" {
			response = "cleared"
		}
		if req.Prompt == ":q" {
			response = "quit"
		}
		_ = json.NewEncoder(w).Encode(submitResponse{Response: response})
	}))
	defer server.Close()

	input := bytes.NewBufferString(":clear\n:q\n")
	output := &bytes.Buffer{}

	exitCode := runInputWidgetCommand(server.URL, input, output)
	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}

	widgetOutput := output.String()
	if strings.Contains(widgetOutput, "Cleared.") {
		t.Fatalf("did not expect cleared status line in input pane, got output:\n%s", widgetOutput)
	}
	if !strings.Contains(widgetOutput, "\x1b[1A\x1b[2K\r") {
		t.Fatalf("expected input clear escape sequence for submitted commands, got output:\n%s", widgetOutput)
	}
}
