package main

import (
	"context"
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
