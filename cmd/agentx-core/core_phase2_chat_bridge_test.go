package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func stageTemplateApplet(t *testing.T, projectDir string) {
	t.Helper()

	templatePath := filepath.Join("..", "..", "applets", "template.py")
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("failed to read template applet: %v", err)
	}

	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		t.Fatalf("failed to create applets dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appletsDir, "template.py"), templateContent, 0o755); err != nil {
		t.Fatalf("failed to write template applet: %v", err)
	}
}

func stageHangingBridgeApplet(t *testing.T, projectDir string) string {
	t.Helper()

	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		t.Fatalf("failed to create applets dir: %v", err)
	}

	scriptPath := filepath.Join(appletsDir, "hanging_bridge.py")
	script := `#!/usr/bin/env python3
import argparse
import json
import sys
import time

parser = argparse.ArgumentParser()
parser.add_argument("--bridge-chat-server", action="store_true")
args = parser.parse_args()

print("READY " + json.dumps({"type": "ready", "applet": "chat", "session": "test"}))
sys.stdout.flush()

for raw_line in sys.stdin:
    if not raw_line.strip():
        continue
    time.sleep(5)
`

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write hanging bridge applet: %v", err)
	}

	return scriptPath
}

func stageSlowChunkBridgeApplet(t *testing.T, projectDir string) string {
	t.Helper()

	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		t.Fatalf("failed to create applets dir: %v", err)
	}

	scriptPath := filepath.Join(appletsDir, "slow_chunk_bridge.py")
	script := `#!/usr/bin/env python3
import argparse
import json
import sys
import time

parser = argparse.ArgumentParser()
parser.add_argument("--bridge-chat-server", action="store_true")
args = parser.parse_args()

print("READY " + json.dumps({"type": "ready", "applet": "chat", "session": "test"}))
sys.stdout.flush()

for raw_line in sys.stdin:
    if not raw_line.strip():
        continue
    req = json.loads(raw_line)
    if req.get("type") != "prompt":
        continue

    print(json.dumps({"type": "chunk", "delta": "partial"}))
    sys.stdout.flush()
    time.sleep(1.0)
    print(json.dumps({"type": "response", "response": f"Echo: {req.get('prompt', '')}"}))
    sys.stdout.flush()
`

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write slow chunk bridge applet: %v", err)
	}

	return scriptPath
}

func stageMalformedBridgeApplet(t *testing.T, projectDir string) string {
	t.Helper()

	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		t.Fatalf("failed to create applets dir: %v", err)
	}

	scriptPath := filepath.Join(appletsDir, "malformed_bridge.py")
	script := `#!/usr/bin/env python3
import argparse
import json
import sys

parser = argparse.ArgumentParser()
parser.add_argument("--bridge-chat-server", action="store_true")
args = parser.parse_args()

print("READY " + json.dumps({"type": "ready", "applet": "chat", "session": "test"}))
sys.stdout.flush()

for raw_line in sys.stdin:
	if not raw_line.strip():
		continue
	req = json.loads(raw_line)
	if req.get("type") != "prompt":
		continue

	print("not-json")
	sys.stdout.flush()
	prompt = req.get("prompt", "")
	print(json.dumps({"type": "chunk", "delta": "Noisy"}))
	sys.stdout.flush()
	print(json.dumps({"type": "response", "response": f"Noisy recovered: {prompt}"}))
	sys.stdout.flush()
`

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write malformed bridge applet: %v", err)
	}

	return scriptPath
}

func stageErrorFrameBridgeApplet(t *testing.T, projectDir string) string {
	t.Helper()

	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		t.Fatalf("failed to create applets dir: %v", err)
	}

	scriptPath := filepath.Join(appletsDir, "error_frame_bridge.py")
	markerPath := filepath.Join(appletsDir, ".error_frame_once_marker")
	script := fmt.Sprintf(`#!/usr/bin/env python3
import argparse
import json
import os
import sys

parser = argparse.ArgumentParser()
parser.add_argument("--bridge-chat-server", action="store_true")
args = parser.parse_args()

marker_path = %q

print("READY " + json.dumps({"type": "ready", "applet": "chat", "session": "test"}))
sys.stdout.flush()

for raw_line in sys.stdin:
	if not raw_line.strip():
		continue
	req = json.loads(raw_line)
	if req.get("type") != "prompt":
		continue

	prompt = req.get("prompt", "")
	if not os.path.exists(marker_path):
		with open(marker_path, "w", encoding="utf-8") as marker_file:
			marker_file.write("error-frame-triggered\n")
		print(json.dumps({"type": "error", "error": "synthetic bridge error"}))
		sys.stdout.flush()
		continue

	print(json.dumps({"type": "response", "response": f"Error-frame recovered: {prompt}"}))
	sys.stdout.flush()
`, markerPath)

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write error-frame bridge applet: %v", err)
	}

	return scriptPath
}

func stageEmptyChunkBridgeApplet(t *testing.T, projectDir string) string {
	t.Helper()

	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		t.Fatalf("failed to create applets dir: %v", err)
	}

	scriptPath := filepath.Join(appletsDir, "empty_chunk_bridge.py")
	script := `#!/usr/bin/env python3
import argparse
import json
import sys

parser = argparse.ArgumentParser()
parser.add_argument("--bridge-chat-server", action="store_true")
args = parser.parse_args()

print("READY " + json.dumps({"type": "ready", "applet": "chat", "session": "test"}))
sys.stdout.flush()

for raw_line in sys.stdin:
    if not raw_line.strip():
        continue
    req = json.loads(raw_line)
    if req.get("type") != "prompt":
        continue

    prompt = req.get("prompt", "")
    print(json.dumps({"type": "chunk", "delta": "   "}))
    sys.stdout.flush()
    print(json.dumps({"type": "response", "response": f"Empty-chunk recovered: {prompt}"}))
    sys.stdout.flush()
`

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write empty-chunk bridge applet: %v", err)
	}

	return scriptPath
}

func stageDuplicateResponseBridgeApplet(t *testing.T, projectDir string) string {
	t.Helper()

	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		t.Fatalf("failed to create applets dir: %v", err)
	}

	scriptPath := filepath.Join(appletsDir, "duplicate_response_bridge.py")
	script := `#!/usr/bin/env python3
import argparse
import json
import sys

parser = argparse.ArgumentParser()
parser.add_argument("--bridge-chat-server", action="store_true")
args = parser.parse_args()

print("READY " + json.dumps({"type": "ready", "applet": "chat", "session": "test"}))
sys.stdout.flush()

for raw_line in sys.stdin:
    if not raw_line.strip():
        continue
    req = json.loads(raw_line)
    if req.get("type") != "prompt":
        continue

    prompt = req.get("prompt", "")
    print(json.dumps({"type": "response", "response": f"Primary response: {prompt}"}))
    sys.stdout.flush()
    print(json.dumps({"type": "response", "response": f"Secondary response: {prompt}"}))
    sys.stdout.flush()
`

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write duplicate-response bridge applet: %v", err)
	}

	return scriptPath
}

// GIVEN a project with Python template applet available
// WHEN a prompt is routed through the chat handler
// THEN the handler uses the Python bridge and returns a deterministic response.
func TestRouteInputPrompt_UsesPythonBridgeTemplate(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	projectDir := t.TempDir()
	stageTemplateApplet(t, projectDir)

	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "phase2 bridge")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Echo: phase2 bridge" {
		t.Fatalf("expected python bridge response, got %q", response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	if len(commandsRaw) == 0 {
		t.Fatal("expected tmux commands to be recorded")
	}
}

// GIVEN a project with Python template applet available
// WHEN two prompts are routed through the chat handler
// THEN the same persistent bridge process is reused for both prompts.
func TestRouteInputPrompt_PythonBridgeProcessReusedAcrossPrompts(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	projectDir := t.TempDir()
	stageTemplateApplet(t, projectDir)

	setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-reuse"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	firstResponse, err := core.RouteInputPrompt(context.Background(), "first bridge prompt")
	if err != nil {
		t.Fatalf("first RouteInputPrompt failed: %v", err)
	}
	if firstResponse != "Echo: first bridge prompt" {
		t.Fatalf("unexpected first response %q", firstResponse)
	}

	core.mu.RLock()
	chatApplet := core.applets["chat"]
	if chatApplet == nil || chatApplet.Cmd == nil || chatApplet.Cmd.Process == nil {
		core.mu.RUnlock()
		t.Fatal("expected tracked persistent chat process after first prompt")
	}
	firstPID := chatApplet.Cmd.Process.Pid
	core.mu.RUnlock()

	secondResponse, err := core.RouteInputPrompt(context.Background(), "second bridge prompt")
	if err != nil {
		t.Fatalf("second RouteInputPrompt failed: %v", err)
	}
	if secondResponse != "Echo: second bridge prompt" {
		t.Fatalf("unexpected second response %q", secondResponse)
	}

	core.mu.RLock()
	chatApplet = core.applets["chat"]
	if chatApplet == nil || chatApplet.Cmd == nil || chatApplet.Cmd.Process == nil {
		core.mu.RUnlock()
		t.Fatal("expected tracked persistent chat process after second prompt")
	}
	secondPID := chatApplet.Cmd.Process.Pid
	core.mu.RUnlock()

	if firstPID != secondPID {
		t.Fatalf("expected persistent chat process reuse, got pid change %d -> %d", firstPID, secondPID)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN Python bridge routing with mocked backend behavior
// WHEN a prompt is routed through RouteInputPrompt
// THEN submitted/classified/thinking/tool/final_response lifecycle stages appear in order.
func TestRouteInputPrompt_PythonBridgeLifecycleStagesInOrder(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	t.Setenv("AGENTX_CHAT_BACKEND", "mock")

	projectDir := t.TempDir()
	stageTemplateApplet(t, projectDir)

	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-lifecycle-order"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	if _, err := core.RouteInputPrompt(context.Background(), "list the files here"); err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)

	orderedStages := []string{
		"stage=submitted",
		"stage=classified",
		"stage=thinking",
		"stage=tool",
		"stage=final_response",
	}

	lastIndex := -1
	for _, marker := range orderedStages {
		idx := strings.Index(commands, marker)
		if idx == -1 {
			t.Fatalf("expected lifecycle marker %q in commands:\n%s", marker, commands)
		}
		if idx <= lastIndex {
			t.Fatalf("expected marker %q after previous lifecycle marker; commands:\n%s", marker, commands)
		}
		lastIndex = idx
	}

	if !strings.Contains(commands, "[bridge] event=bridge_response_ok") {
		t.Fatalf("expected bridge_response_ok in command log, got:\n%s", commands)
	}
}

// GIVEN chat backend is configured for Ollama with an unreachable host
// WHEN a prompt is routed through the persistent Python bridge
// THEN the applet falls back to deterministic echo response without failing routing.
func TestRouteInputPrompt_OllamaBackendFallsBackToEcho(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	t.Setenv("AGENTX_CHAT_BACKEND", "ollama")
	t.Setenv("AGENTX_OLLAMA_HOST", "127.0.0.1:1")
	t.Setenv("AGENTX_OLLAMA_MODEL", "llama3.2")

	projectDir := t.TempDir()
	stageTemplateApplet(t, projectDir)

	setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-ollama-fallback"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "ollama fallback prompt")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Echo: ollama fallback prompt" {
		t.Fatalf("expected fallback echo response, got %q", response)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN a hanging chat bridge applet process
// WHEN direct bridge routing is called
// THEN it returns a timeout error bounded by configured response timeout.
func TestRoutePromptViaPythonChatApplet_TimesOutOnNoResponse(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	projectDir := t.TempDir()
	hangingScript := stageHangingBridgeApplet(t, projectDir)

	setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-timeout-direct"}
	core := NewAgentXCore(cfg)
	core.chatAppletScript = hangingScript
	core.chatBridgeResponseTimeout = 150 * time.Millisecond

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	_, err := core.routePromptViaPythonChatApplet(context.Background(), "timeout direct")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "timeout") {
		t.Fatalf("expected timeout error, got %q", got)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN a hanging chat bridge applet process
// WHEN prompt routing occurs through the public RouteInputPrompt path
// THEN the chat handler falls back to deterministic echo response.
func TestRouteInputPrompt_HangingBridgeFallsBackToEcho(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	projectDir := t.TempDir()
	hangingScript := stageHangingBridgeApplet(t, projectDir)

	setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-timeout-fallback"}
	core := NewAgentXCore(cfg)
	core.chatAppletScript = hangingScript
	core.chatBridgeResponseTimeout = 150 * time.Millisecond

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "timeout fallback")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Echo: timeout fallback" {
		t.Fatalf("expected fallback echo response, got %q", response)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN template bridge emits chunk events for multi-token responses
// WHEN a prompt is routed through the chat path
// THEN stream chunk lines and final consolidated response are both rendered.
func TestRouteInputPrompt_RendersStreamChunksAndFinalResponse(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	projectDir := t.TempDir()
	stageTemplateApplet(t, projectDir)

	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-stream-render"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "stream chunk demo")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Echo: stream chunk demo" {
		t.Fatalf("unexpected response %q", response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "[assistant-stream] Echo:") {
		t.Fatalf("expected stream chunk render in tmux commands, got:\n%s", commands)
	}
	if !strings.Contains(commands, "[assistant] Echo: stream chunk demo") {
		t.Fatalf("expected final consolidated render in tmux commands, got:\n%s", commands)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN chat backend is configured for Ollama with a streaming endpoint
// WHEN a prompt is routed through the persistent bridge
// THEN chunk events are sourced from backend stream and final response is rendered.
func TestRouteInputPrompt_OllamaStreamingBackendRendersChunks(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"message":{"content":"Ollama"},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"message":{"content":"stream"},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"message":{"content":"reply"},"done":true}`)
	}))
	defer server.Close()

	t.Setenv("AGENTX_CHAT_BACKEND", "ollama")
	t.Setenv("AGENTX_OLLAMA_HOST", server.URL)
	t.Setenv("AGENTX_OLLAMA_MODEL", "test-model")

	projectDir := t.TempDir()
	stageTemplateApplet(t, projectDir)

	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-ollama-stream"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "ollama stream prompt")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Ollama stream reply" {
		t.Fatalf("expected ollama streamed response, got %q", response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "[assistant-stream] Ollama") {
		t.Fatalf("expected ollama stream chunk render in tmux log, got:\n%s", commands)
	}
	if !strings.Contains(commands, "[assistant] Ollama stream reply") {
		t.Fatalf("expected final ollama response render in tmux log, got:\n%s", commands)
	}
	if !strings.Contains(commands, "[bridge] event=bridge_start") {
		t.Fatalf("expected bridge_start event in logs pane commands, got:\n%s", commands)
	}
	if !strings.Contains(commands, "[bridge] event=bridge_chunk") {
		t.Fatalf("expected bridge_chunk event in logs pane commands, got:\n%s", commands)
	}
	if !strings.Contains(commands, "[bridge] event=bridge_response_ok") {
		t.Fatalf("expected bridge_response_ok event in logs pane commands, got:\n%s", commands)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN a bridge that emits one chunk and then stalls
// WHEN the first route is canceled mid-stream and a second prompt is routed immediately
// THEN cancellation is propagated and the next prompt succeeds via a restarted bridge process.
func TestRouteInputPrompt_CanceledMidStreamRecoversOnImmediateRetry(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	projectDir := t.TempDir()
	slowScript := stageSlowChunkBridgeApplet(t, projectDir)

	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-cancel-retry"}
	core := NewAgentXCore(cfg)
	core.chatAppletScript = slowScript
	core.chatBridgeResponseTimeout = 3 * time.Second

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	cancelCtx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err := core.RouteInputPrompt(cancelCtx, "cancel me")
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded error, got %v", err)
	}

	if turns := core.ContextTurnsSnapshot(); len(turns) != 0 {
		t.Fatalf("expected no persisted turns after canceled route, got %d", len(turns))
	}

	retryResponse, retryErr := core.RouteInputPrompt(context.Background(), "recover after cancel")
	if retryErr != nil {
		t.Fatalf("retry RouteInputPrompt failed: %v", retryErr)
	}
	if retryResponse != "Echo: recover after cancel" {
		t.Fatalf("unexpected retry response %q", retryResponse)
	}

	if turns := core.ContextTurnsSnapshot(); len(turns) != 1 {
		t.Fatalf("expected exactly one persisted turn after retry, got %d", len(turns))
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "[bridge] event=bridge_canceled") {
		t.Fatalf("expected bridge_canceled event in tmux log, got:\n%s", commands)
	}
	if strings.Count(commands, "[bridge] event=bridge_start") < 2 {
		t.Fatalf("expected bridge restart after cancellation, got commands:\n%s", commands)
	}
	if !strings.Contains(commands, "[assistant] Echo: recover after cancel") {
		t.Fatalf("expected successful retry render in tmux log, got:\n%s", commands)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN a bridge that emits malformed JSON before valid stream events
// WHEN a prompt is routed through the bridge
// THEN malformed frames are ignored and final response succeeds with response-ok observability.
func TestRouteInputPrompt_MalformedJSONIsIgnoredAndResponseSucceeds(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	projectDir := t.TempDir()
	scriptPath := stageMalformedBridgeApplet(t, projectDir)

	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-malformed-json"}
	core := NewAgentXCore(cfg)
	core.chatAppletScript = scriptPath

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "malformed input")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Noisy recovered: malformed input" {
		t.Fatalf("expected recovered response, got %q", response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "[bridge] event=bridge_response_ok") {
		t.Fatalf("expected bridge_response_ok event in tmux log, got:\n%s", commands)
	}
	if strings.Contains(commands, "[bridge] event=bridge_fallback") {
		t.Fatalf("did not expect fallback for malformed-json tolerant route, got:\n%s", commands)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN a bridge that emits an explicit error frame on first request
// WHEN prompts are routed twice
// THEN first route falls back and second route recovers after restart with response-ok observability.
func TestRouteInputPrompt_ErrorFrameFallbackThenRecovery(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	projectDir := t.TempDir()
	scriptPath := stageErrorFrameBridgeApplet(t, projectDir)

	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-error-frame"}
	core := NewAgentXCore(cfg)
	core.chatAppletScript = scriptPath

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	firstResponse, err := core.RouteInputPrompt(context.Background(), "first error-frame")
	if err != nil {
		t.Fatalf("first RouteInputPrompt failed: %v", err)
	}
	if firstResponse != "Echo: first error-frame" {
		t.Fatalf("expected fallback echo response, got %q", firstResponse)
	}

	secondResponse, err := core.RouteInputPrompt(context.Background(), "second error-frame")
	if err != nil {
		t.Fatalf("second RouteInputPrompt failed: %v", err)
	}
	if secondResponse != "Error-frame recovered: second error-frame" {
		t.Fatalf("expected recovered response, got %q", secondResponse)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "[bridge] event=bridge_response_error") {
		t.Fatalf("expected bridge_response_error in tmux log, got:\n%s", commands)
	}
	if !strings.Contains(commands, "[bridge] event=bridge_fallback") {
		t.Fatalf("expected bridge_fallback in tmux log, got:\n%s", commands)
	}
	if !strings.Contains(commands, "[bridge] event=bridge_response_ok") {
		t.Fatalf("expected bridge_response_ok in tmux log, got:\n%s", commands)
	}
	if strings.Count(commands, "[bridge] event=bridge_start") < 2 {
		t.Fatalf("expected bridge restart across fallback/recovery, got:\n%s", commands)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN a bridge response stream that includes empty chunk payloads
// WHEN a prompt is routed through the bridge
// THEN empty chunks do not render stream output while final response and persistence still succeed.
func TestRouteInputPrompt_EmptyChunkIgnoredWithPersistence(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	projectDir := t.TempDir()
	scriptPath := stageEmptyChunkBridgeApplet(t, projectDir)

	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-empty-chunk"}
	core := NewAgentXCore(cfg)
	core.chatAppletScript = scriptPath

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "empty chunk route")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Empty-chunk recovered: empty chunk route" {
		t.Fatalf("unexpected response %q", response)
	}

	turns := core.ContextTurnsSnapshot()
	if len(turns) != 1 {
		t.Fatalf("expected one persisted turn, got %d", len(turns))
	}
	if turns[0].Response != "Empty-chunk recovered: empty chunk route" {
		t.Fatalf("expected persisted response to match final response, got %q", turns[0].Response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if strings.Contains(commands, "[assistant-stream]") {
		t.Fatalf("did not expect assistant stream render for empty chunks, got:\n%s", commands)
	}
	if strings.Contains(commands, "[bridge] event=bridge_chunk") {
		t.Fatalf("did not expect bridge_chunk events for empty chunks, got:\n%s", commands)
	}
	if !strings.Contains(commands, "[bridge] event=bridge_response_ok") {
		t.Fatalf("expected bridge_response_ok event, got:\n%s", commands)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN go chat runtime with unreachable ollama backend
// WHEN a prompt is routed
// THEN go-runtime fallback telemetry is emitted and deterministic echo fallback is returned directly.
func TestRouteInputPrompt_GoChatRuntimeOllamaFailureFallsBackDirectlyToEcho(t *testing.T) {
	t.Setenv("AGENTX_CHAT_RUNTIME", "go")
	t.Setenv("AGENTX_CHAT_BACKEND", "ollama")
	t.Setenv("AGENTX_OLLAMA_HOST", "127.0.0.1:1")

	projectDir := t.TempDir()

	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-go-chat-fallback-direct"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "go chat fallback prompt")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Echo: go chat fallback prompt" {
		t.Fatalf("expected python bridge recovered echo response, got %q", response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "[bridge] event=go_chat_fallback") {
		t.Fatalf("expected go_chat_fallback telemetry, got:\n%s", commands)
	}
	if strings.Contains(commands, "[bridge] event=bridge_route_start") {
		t.Fatalf("did not expect python bridge route telemetry after direct go fallback, got:\n%s", commands)
	}
	if strings.Contains(commands, "[bridge] event=go_chat_bridge_fallback") {
		t.Fatalf("did not expect secondary go_chat_bridge_fallback when no Python bridge path is used, got:\n%s", commands)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN go chat runtime with unreachable ollama backend and no template staged
// WHEN a prompt is routed
// THEN deterministic echo fallback remains available without any Python bridge telemetry.
func TestRouteInputPrompt_GoChatRuntimeFallbackDoesNotDependOnTemplateScript(t *testing.T) {
	t.Setenv("AGENTX_CHAT_RUNTIME", "go")
	t.Setenv("AGENTX_CHAT_BACKEND", "ollama")
	t.Setenv("AGENTX_OLLAMA_HOST", "127.0.0.1:1")

	projectDir := t.TempDir()
	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-go-chat-fallback-echo"}
	core := NewAgentXCore(cfg)
	core.chatAppletScript = filepath.Join(projectDir, "applets", "missing_template.py")

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "go chat double fallback prompt")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Echo: go chat double fallback prompt" {
		t.Fatalf("expected deterministic echo fallback response, got %q", response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "[bridge] event=go_chat_fallback") {
		t.Fatalf("expected go_chat_fallback telemetry, got:\n%s", commands)
	}
	if strings.Contains(commands, "[bridge] event=go_chat_bridge_fallback") {
		t.Fatalf("did not expect go_chat_bridge_fallback telemetry, got:\n%s", commands)
	}
	if strings.Contains(commands, "[bridge] event=bridge_start_error") {
		t.Fatalf("did not expect bridge_start_error telemetry when Python bridge is not consulted, got:\n%s", commands)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN go chat runtime with a reachable backend
// WHEN a prompt is routed directly through the Go backend path
// THEN normalized route-start and success telemetry are emitted without fallback.
func TestRouteInputPrompt_GoChatRuntimeDirectSuccessEmitsTelemetry(t *testing.T) {
	t.Setenv("AGENTX_CHAT_RUNTIME", "go")
	t.Setenv("AGENTX_CHAT_BACKEND", "ollama")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"message":{"content":"Direct go reply"}}`)
	}))
	defer server.Close()
	t.Setenv("AGENTX_OLLAMA_HOST", strings.TrimPrefix(server.URL, "http://"))

	projectDir := t.TempDir()
	stageTemplateApplet(t, projectDir)
	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2-go-chat-direct-success"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "go chat direct success")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Direct go reply" {
		t.Fatalf("expected direct go reply, got %q", response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "[bridge] event=go_chat_route_start") {
		t.Fatalf("expected go_chat_route_start telemetry, got:\n%s", commands)
	}
	if !strings.Contains(commands, "[bridge] event=go_chat_response_ok") {
		t.Fatalf("expected go_chat_response_ok telemetry, got:\n%s", commands)
	}
	if strings.Contains(commands, "[bridge] event=go_chat_fallback") {
		t.Fatalf("did not expect go_chat_fallback on direct success, got:\n%s", commands)
	}

	if err := core.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

// GIVEN a bridge response stream with duplicate response frames
// WHEN parsing bridge output
// THEN the first response frame is authoritative.
func TestReadChatBridgeResponseFromScanner_DuplicateResponseUsesFirst(t *testing.T) {
	t.Parallel()

	scanner := bufio.NewScanner(strings.NewReader(strings.Join([]string{
		`READY {"type":"ready"}`,
		`{"type":"response","response":"primary"}`,
		`{"type":"response","response":"secondary"}`,
	}, "\n")))

	result := readChatBridgeResponseFromScanner(scanner, nil)
	if result.err != nil {
		t.Fatalf("expected no error, got %v", result.err)
	}
	if result.response != "primary" {
		t.Fatalf("expected primary response, got %q", result.response)
	}
}

// GIVEN a bridge stream containing malformed frames before valid frames
// WHEN parsing bridge output
// THEN malformed frames are ignored and valid chunk/response frames are processed.
func TestReadChatBridgeResponseFromScanner_MalformedFramesIgnored(t *testing.T) {
	t.Parallel()

	chunks := []string{}
	scanner := bufio.NewScanner(strings.NewReader(strings.Join([]string{
		`READY {"type":"ready"}`,
		`not-json`,
		`{"type":"chunk","delta":"valid"}`,
		`{"type":"response","response":"valid response"}`,
	}, "\n")))

	result := readChatBridgeResponseFromScanner(scanner, func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})
	if result.err != nil {
		t.Fatalf("expected no error, got %v", result.err)
	}
	if result.response != "valid response" {
		t.Fatalf("unexpected response %q", result.response)
	}
	if len(chunks) != 1 || chunks[0] != "valid" {
		t.Fatalf("expected one valid chunk callback, got %v", chunks)
	}
}

// GIVEN a bridge stream with a terminal response followed by late error frame
// WHEN parsing bridge output
// THEN terminal response is returned and late frames are ignored for that parse cycle.
func TestReadChatBridgeResponseFromScanner_LateErrorAfterResponseIgnored(t *testing.T) {
	t.Parallel()

	scanner := bufio.NewScanner(strings.NewReader(strings.Join([]string{
		`READY {"type":"ready"}`,
		`{"type":"response","response":"done"}`,
		`{"type":"error","error":"late"}`,
	}, "\n")))

	result := readChatBridgeResponseFromScanner(scanner, nil)
	if result.err != nil {
		t.Fatalf("expected no error, got %v", result.err)
	}
	if result.response != "done" {
		t.Fatalf("expected done response, got %q", result.response)
	}
}
