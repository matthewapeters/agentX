package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

type bddState struct {
	tmpDir       string
	cfg          *Config
	core         *AgentXCore
	layout       []PaneConfig
	router       *IPCRouter
	inputFIFO    string
	outputFIFO   string
	err          error
	contextMgr   *ContextManager
	healthErr    error
	cancelCtx    context.Context
	fakeTmuxLog  string
	oldPath      string
	oldTmuxLog   string
	oldPaneMode  string
	tmuxCommands string
	snapshot     HealthSnapshot
	routedResp   string
	inputResp    string
	inputExit    bool
	contextTurns []ChatTurn
	chatPID      int
	bridgeScript string
	originalChatRuntime string
	originalChatBackend string
	originalOllamaHost string
	backendStubServer *httptest.Server
}

func (s *bddState) reset() {
	s.tmpDir = ""
	s.cfg = nil
	s.core = nil
	s.layout = nil
	s.router = nil
	s.inputFIFO = ""
	s.outputFIFO = ""
	s.err = nil
	s.contextMgr = nil
	s.healthErr = nil
	s.cancelCtx = nil
	s.fakeTmuxLog = ""
	s.oldPath = ""
	s.oldTmuxLog = ""
	s.oldPaneMode = ""
	s.tmuxCommands = ""
	s.snapshot = HealthSnapshot{}
	s.routedResp = ""
	s.inputResp = ""
	s.inputExit = false
	s.contextTurns = nil
	s.chatPID = 0
	s.bridgeScript = ""
	s.originalChatRuntime = ""
	s.originalChatBackend = ""
	s.originalOllamaHost = ""
	s.backendStubServer = nil
}

func (s *bddState) iStartDelayedOllamaBackendWithStatus(delayMs, statusCode int) error {
	if delayMs < 0 {
		return fmt.Errorf("delay must be non-negative")
	}
	if statusCode < 100 || statusCode > 599 {
		return fmt.Errorf("invalid status code: %d", statusCode)
	}

	if s.backendStubServer != nil {
		s.backendStubServer.Close()
		s.backendStubServer = nil
	}

	delay := time.Duration(delayMs) * time.Millisecond
	s.backendStubServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		time.Sleep(delay)

		w.Header().Set("Content-Type", "application/json")
		if statusCode >= 200 && statusCode < 300 {
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte(`{"message":{"content":"Delayed backend reply"}}`))
			return
		}

		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(`{"error":"delayed backend failure"}`))
	}))

	if err := s.setOllamaHostOverride(s.backendStubServer.URL); err != nil {
		return err
	}
	return nil
}

func (s *bddState) iStartSequencedDelayedOllamaBackend(delayMs, firstStatusCode, secondStatusCode int) error {
	if delayMs < 0 {
		return fmt.Errorf("delay must be non-negative")
	}
	for _, code := range []int{firstStatusCode, secondStatusCode} {
		if code < 100 || code > 599 {
			return fmt.Errorf("invalid status code: %d", code)
		}
	}

	if s.backendStubServer != nil {
		s.backendStubServer.Close()
		s.backendStubServer = nil
	}

	delay := time.Duration(delayMs) * time.Millisecond
	var mu sync.Mutex
	requestCount := 0

	s.backendStubServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}

		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		time.Sleep(delay)

		mu.Lock()
		requestCount++
		activeStatus := secondStatusCode
		// First prompt now issues a single Go backend call before direct deterministic fallback.
		// Recover on the next prompt after the first failed backend attempt.
		if requestCount <= 1 {
			activeStatus = firstStatusCode
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if activeStatus >= 200 && activeStatus < 300 {
			w.WriteHeader(activeStatus)
			_, _ = w.Write([]byte(`{"message":{"content":"Delayed backend recovery reply"}}`))
			return
		}

		w.WriteHeader(activeStatus)
		_, _ = w.Write([]byte(`{"error":"sequenced delayed backend failure"}`))
	}))

	if err := s.setOllamaHostOverride(s.backendStubServer.URL); err != nil {
		return err
	}
	return nil
}

func (s *bddState) setChatRuntimeOverride(runtime string) error {
	if s.originalChatRuntime == "" {
		s.originalChatRuntime = os.Getenv("AGENTX_CHAT_RUNTIME")
	}
	return os.Setenv("AGENTX_CHAT_RUNTIME", strings.TrimSpace(runtime))
}

func (s *bddState) setChatBackendOverride(backend string) error {
	if s.originalChatBackend == "" {
		s.originalChatBackend = os.Getenv("AGENTX_CHAT_BACKEND")
	}
	return os.Setenv("AGENTX_CHAT_BACKEND", strings.TrimSpace(backend))
}

func (s *bddState) setOllamaHostOverride(host string) error {
	if s.originalOllamaHost == "" {
		s.originalOllamaHost = os.Getenv("AGENTX_OLLAMA_HOST")
	}
	return os.Setenv("AGENTX_OLLAMA_HOST", strings.TrimSpace(host))
}

func stageFlakyBridgeApplet(projectDir string) (string, error) {
	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create applets dir: %w", err)
	}

	scriptPath := filepath.Join(appletsDir, "flaky_bridge.py")
	markerPath := filepath.Join(appletsDir, ".flaky_once_marker")
	script := fmt.Sprintf(`#!/usr/bin/env python3
import argparse
import json
import os
import sys
import time

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

	if not os.path.exists(marker_path):
		with open(marker_path, "w", encoding="utf-8") as marker_file:
			marker_file.write("first-timeout-done\n")
		time.sleep(1.0)
		continue

	prompt = req.get("prompt", "")
	print(json.dumps({"type": "chunk", "delta": "Flaky"}))
	sys.stdout.flush()
	print(json.dumps({"type": "chunk", "delta": "recovered:"}))
	sys.stdout.flush()
	print(json.dumps({"type": "chunk", "delta": prompt}))
	sys.stdout.flush()
	print(json.dumps({"type": "response", "response": f"Flaky recovered: {prompt}"}))
	sys.stdout.flush()
`, markerPath)

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("failed to write flaky bridge applet: %w", err)
	}

	return scriptPath, nil
}

func stageMalformedBridgeAppletBDD(projectDir string) (string, error) {
	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create applets dir: %w", err)
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
	print(json.dumps({"type": "response", "response": f"Malformed recovered: {prompt}"}))
	sys.stdout.flush()
`

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("failed to write malformed bridge applet: %w", err)
	}

	return scriptPath, nil
}

func stageErrorFrameBridgeAppletBDD(projectDir string) (string, error) {
	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create applets dir: %w", err)
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
			marker_file.write("error-triggered\n")
		print(json.dumps({"type": "error", "error": "synthetic error frame"}))
		sys.stdout.flush()
		continue

	print(json.dumps({"type": "response", "response": f"Error recovered: {prompt}"}))
	sys.stdout.flush()
`, markerPath)

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("failed to write error-frame bridge applet: %w", err)
	}

	return scriptPath, nil
}

func stageEmptyChunkBridgeAppletBDD(projectDir string) (string, error) {
	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create applets dir: %w", err)
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
	print(json.dumps({"type": "response", "response": f"Empty recovered: {prompt}"}))
	sys.stdout.flush()
`

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("failed to write empty-chunk bridge applet: %w", err)
	}

	return scriptPath, nil
}

func stageStartupLatencyBridgeAppletBDD(projectDir string) (string, error) {
	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create applets dir: %w", err)
	}

	scriptPath := filepath.Join(appletsDir, "startup_latency_bridge.py")
	markerPath := filepath.Join(appletsDir, ".startup_latency_once_marker")
	script := fmt.Sprintf(`#!/usr/bin/env python3
import argparse
import json
import os
import sys
import time

parser = argparse.ArgumentParser()
parser.add_argument("--bridge-chat-server", action="store_true")
args = parser.parse_args()

marker_path = %q

# Simulate intermittent process startup latency for the first bridge process only.
if not os.path.exists(marker_path):
	with open(marker_path, "w", encoding="utf-8") as marker_file:
		marker_file.write("slow-start-consumed\n")
	time.sleep(0.45)

print("READY " + json.dumps({"type": "ready", "applet": "chat", "session": "test"}))
sys.stdout.flush()

for raw_line in sys.stdin:
	if not raw_line.strip():
		continue
	req = json.loads(raw_line)
	if req.get("type") != "prompt":
		continue

	prompt = req.get("prompt", "")
	print(json.dumps({"type": "response", "response": f"Startup latency recovered: {prompt}"}))
	sys.stdout.flush()
`, markerPath)

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return "", fmt.Errorf("failed to write startup-latency bridge applet: %w", err)
	}

	return scriptPath, nil
}

func (s *bddState) theProjectContainsTemplateChatApplet() error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}

	templatePath := filepath.Join("..", "..", "applets", "template.py")
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed reading template applet: %w", err)
	}

	appletsDir := filepath.Join(s.tmpDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		return fmt.Errorf("failed creating applets directory: %w", err)
	}

	if err := os.WriteFile(filepath.Join(appletsDir, "template.py"), templateContent, 0o755); err != nil {
		return fmt.Errorf("failed writing template applet: %w", err)
	}

	return nil
}

func (s *bddState) iHaveATemporaryProjectDirectory() error {
	d, err := os.MkdirTemp("", "agentx-core-bdd-")
	if err != nil {
		return err
	}
	s.tmpDir = d
	return nil
}

func (s *bddState) theProjectContainsFlakyChatBridgeApplet() error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}

	scriptPath, err := stageFlakyBridgeApplet(s.tmpDir)
	if err != nil {
		return err
	}

	s.bridgeScript = scriptPath
	return nil
}

func (s *bddState) theProjectContainsMalformedChatBridgeApplet() error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}

	scriptPath, err := stageMalformedBridgeAppletBDD(s.tmpDir)
	if err != nil {
		return err
	}

	s.bridgeScript = scriptPath
	return nil
}

func (s *bddState) theProjectContainsErrorFrameChatBridgeApplet() error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}

	scriptPath, err := stageErrorFrameBridgeAppletBDD(s.tmpDir)
	if err != nil {
		return err
	}

	s.bridgeScript = scriptPath
	return nil
}

func (s *bddState) theProjectContainsEmptyChunkChatBridgeApplet() error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}

	scriptPath, err := stageEmptyChunkBridgeAppletBDD(s.tmpDir)
	if err != nil {
		return err
	}

	s.bridgeScript = scriptPath
	return nil
}

func (s *bddState) theProjectContainsStartupLatencyChatBridgeApplet() error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}

	scriptPath, err := stageStartupLatencyBridgeAppletBDD(s.tmpDir)
	if err != nil {
		return err
	}

	s.bridgeScript = scriptPath
	return nil
}

func (s *bddState) aConfigWithUsername(username string) error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}
	s.cfg = &Config{ProjectDir: s.tmpDir, Username: username, SessionID: ""}
	return nil
}

func (s *bddState) iEnsureSessionDirectoriesForSession(sessionID string) error {
	if s.cfg == nil {
		return errors.New("config not initialized")
	}
	s.err = s.cfg.EnsureSessionDirs(sessionID)
	return nil
}

func (s *bddState) theSessionDirectoryStructureShouldExist() error {
	if s.err != nil {
		return s.err
	}
	paths := []string{
		filepath.Join(s.tmpDir, "sessions", s.cfg.Username),
		filepath.Join(s.tmpDir, "sessions", s.cfg.Username, "s1", "logs"),
		filepath.Join(s.tmpDir, "sessions", s.cfg.Username, "s1", "context"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("expected path to exist: %s (%w)", p, err)
		}
	}
	return nil
}

func (s *bddState) theDefaultPaneLayout() error {
	s.layout = DefaultPaneLayout()
	return nil
}

func (s *bddState) paneNamesShouldInclude(name string) error {
	for _, pane := range s.layout {
		if pane.Name == name {
			return nil
		}
	}
	return fmt.Errorf("expected pane name %q in layout", name)
}

func (s *bddState) anIPCRouterForSession(sessionID string) error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}
	s.router = NewIPCRouter(sessionID, s.tmpDir)
	return nil
}

func (s *bddState) iCreateAnIPCFIFOPairForApplet(applet string) error {
	if s.router == nil {
		return errors.New("router not initialized")
	}
	in, out, err := s.router.CreateFIFOPair(applet)
	s.inputFIFO, s.outputFIFO, s.err = in, out, err
	return nil
}

func (s *bddState) bothFIFOFilesShouldExist() error {
	if s.err != nil {
		return s.err
	}
	for _, p := range []string{s.inputFIFO, s.outputFIFO} {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("expected fifo to exist: %s (%w)", p, err)
		}
	}
	return nil
}

func (s *bddState) fifoPathsShouldIncludeAppletName(applet string) error {
	if !strings.Contains(s.inputFIFO, applet) || !strings.Contains(s.outputFIFO, applet) {
		return fmt.Errorf("expected fifo paths to include applet name %q", applet)
	}
	return nil
}

func (s *bddState) aCoreConfigWithUsernameAndSession(username, session string) error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}
	s.cfg = &Config{ProjectDir: s.tmpDir, Username: username, SessionID: session}
	return nil
}

func (s *bddState) iConstructTheAgentXCore() error {
	if s.cfg == nil {
		return errors.New("config not initialized")
	}
	s.core = NewAgentXCore(s.cfg)
	return nil
}

func (s *bddState) iConfigureCoreChatBridgeToUsePreparedAppletScriptWithTimeoutMs(timeoutMs int) error {
	if s.core == nil {
		return errors.New("core not initialized")
	}
	if s.bridgeScript == "" {
		return errors.New("prepared bridge script not initialized")
	}

	s.core.chatAppletScript = s.bridgeScript
	s.core.chatBridgeResponseTimeout = time.Duration(timeoutMs) * time.Millisecond
	return nil
}

func (s *bddState) theCoreSessionIDShouldBeNonempty() error {
	if s.core == nil || s.core.SessionID == "" {
		return errors.New("expected non-empty session id")
	}
	return nil
}

func (s *bddState) theCoreTmuxSessionNameShouldIncludeUsername(username string) error {
	if s.core == nil {
		return errors.New("core not initialized")
	}
	if !strings.Contains(s.core.tmuxSessionName, username) {
		return fmt.Errorf("expected tmux session name to include %q", username)
	}
	return nil
}

func (s *bddState) aContextManagerWithATemporaryContextDirectory() error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}
	s.contextMgr = NewContextManager(filepath.Join(s.tmpDir, "ctx"))
	return nil
}

func (s *bddState) aCanceledContext() error {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.cancelCtx = ctx
	return nil
}

func (s *bddState) iRunTheHealthServeRoutine() error {
	if s.contextMgr == nil || s.cancelCtx == nil {
		return errors.New("context manager or canceled context not initialized")
	}
	s.healthErr = s.contextMgr.ServeHealth(s.cancelCtx, "127.0.0.1:0")
	return nil
}

func (s *bddState) theRoutineShouldReturnContextCanceled() error {
	if !errors.Is(s.healthErr, context.Canceled) {
		return fmt.Errorf("expected context canceled, got: %v", s.healthErr)
	}
	return nil
}

func (s *bddState) aConstructedAgentXCore() error {
	if s.cfg == nil {
		return errors.New("config not initialized")
	}
	s.core = NewAgentXCore(s.cfg)
	return nil
}

func (s *bddState) iInvokeCoreShutdown() error {
	if s.core == nil {
		return errors.New("core not initialized")
	}
	s.err = s.core.Shutdown(context.Background())
	return nil
}

func (s *bddState) shutdownShouldCompleteWithoutError() error {
	if s.err != nil {
		return s.err
	}
	return nil
}

func (s *bddState) aFakeTmuxExecutableThatRecordsCommands() error {
	if s.tmpDir == "" {
		return errors.New("temporary project directory not initialized")
	}

	logPath := filepath.Join(s.tmpDir, "tmux_commands.log")
	scriptPath := filepath.Join(s.tmpDir, "tmux")
	script := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"printf '%s\\n' \"$*\" >> \"${TMUX_LOG}\"\n" +
		"if [[ \"$1\" == \"split-window\" ]]; then\n" +
		"  if [[ \"$*\" == *\" -v \"* ]]; then echo \"%3\"; else echo \"%4\"; fi\n" +
		"fi\n"

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return fmt.Errorf("failed to write fake tmux executable: %w", err)
	}

	s.oldPath = os.Getenv("PATH")
	s.oldTmuxLog = os.Getenv("TMUX_LOG")
	s.oldPaneMode = os.Getenv("AGENTX_PANE_RENDER_MODE")
	s.fakeTmuxLog = logPath

	if err := os.Setenv("PATH", s.tmpDir+":"+s.oldPath); err != nil {
		return fmt.Errorf("failed to set PATH for fake tmux: %w", err)
	}
	if err := os.Setenv("TMUX_LOG", s.fakeTmuxLog); err != nil {
		return fmt.Errorf("failed to set TMUX_LOG for fake tmux: %w", err)
	}
	if err := os.Setenv("AGENTX_PANE_RENDER_MODE", "core"); err != nil {
		return fmt.Errorf("failed to set AGENTX_PANE_RENDER_MODE for fake tmux: %w", err)
	}

	return nil
}

func (s *bddState) iInitializeTheTmuxSession() error {
	if s.core == nil {
		return errors.New("core not initialized")
	}

	s.err = s.core.InitializeTmuxSession(context.Background())
	if s.fakeTmuxLog != "" {
		data, readErr := os.ReadFile(s.fakeTmuxLog)
		if readErr == nil {
			s.tmuxCommands = string(data)
		}
	}

	return nil
}

func (s *bddState) iStartTheAppletSupervisor() error {
	if s.core == nil {
		return errors.New("core not initialized")
	}

	s.err = s.core.StartAppletSupervisor(context.Background())
	return nil
}

func (s *bddState) iRouteInputPrompt(prompt string) error {
	if s.core == nil {
		return errors.New("core not initialized")
	}

	response, err := s.core.RouteInputPrompt(context.Background(), prompt)
	s.routedResp = response
	s.err = err

	if s.fakeTmuxLog != "" {
		data, readErr := os.ReadFile(s.fakeTmuxLog)
		if readErr == nil {
			s.tmuxCommands = string(data)
		}
	}

	return nil
}

func (s *bddState) promptRoutingShouldCompleteWithoutError() error {
	if s.err != nil {
		return s.err
	}
	return nil
}

func (s *bddState) routedResponseShouldEqual(expected string) error {
	if s.routedResp != expected {
		return fmt.Errorf("expected routed response %q, got %q", expected, s.routedResp)
	}
	return nil
}

func (s *bddState) tmuxShouldIncludeRenderedChatResponse(response string) error {
	expected := "echo '[assistant] " + response + "' Enter"
	if !strings.Contains(s.tmuxCommands, expected) {
		return fmt.Errorf("expected tmux render command %q, got:\n%s", expected, s.tmuxCommands)
	}
	return nil
}

func (s *bddState) iHandleInputLine(line string) error {
	if s.core == nil {
		return errors.New("core not initialized")
	}

	resp, shouldExit, err := s.core.HandleInputLine(context.Background(), line)
	s.inputResp = resp
	s.inputExit = shouldExit
	s.err = err

	if s.fakeTmuxLog != "" {
		data, readErr := os.ReadFile(s.fakeTmuxLog)
		if readErr == nil {
			s.tmuxCommands = string(data)
		}
	}

	return nil
}

func (s *bddState) inputHandlingShouldCompleteWithoutError() error {
	if s.err != nil {
		return s.err
	}
	return nil
}

func (s *bddState) inputResponseShouldEqual(expected string) error {
	if s.inputResp != expected {
		return fmt.Errorf("expected input response %q, got %q", expected, s.inputResp)
	}
	return nil
}

func (s *bddState) inputExitFlagShouldBe(expected string) error {
	expectedBool := expected == "true"
	if s.inputExit != expectedBool {
		return fmt.Errorf("expected input exit flag %t, got %t", expectedBool, s.inputExit)
	}
	return nil
}

func (s *bddState) iReconstructTheAgentXCoreWithTheSameConfig() error {
	if s.cfg == nil {
		return errors.New("config not initialized")
	}
	s.core = NewAgentXCore(s.cfg)
	return nil
}

func (s *bddState) iCaptureTheContextTurnsSnapshot() error {
	if s.core == nil {
		return errors.New("core not initialized")
	}
	s.contextTurns = s.core.ContextTurnsSnapshot()
	return nil
}

func (s *bddState) contextTurnsShouldHaveLength(expected int) error {
	if len(s.contextTurns) != expected {
		return fmt.Errorf("expected context turns length %d, got %d", expected, len(s.contextTurns))
	}
	return nil
}

func (s *bddState) contextTurnsShouldIncludePrompt(prompt string) error {
	for _, turn := range s.contextTurns {
		if turn.Prompt == prompt {
			return nil
		}
	}
	return fmt.Errorf("expected context turns to include prompt %q", prompt)
}

func (s *bddState) contextTurnsShouldIncludeResponse(response string) error {
	for _, turn := range s.contextTurns {
		if turn.Response == response {
			return nil
		}
	}
	return fmt.Errorf("expected context turns to include response %q", response)
}

func (s *bddState) iCaptureTheTrackedChatAppletProcessPID() error {
	if s.core == nil {
		return errors.New("core not initialized")
	}

	s.core.mu.RLock()
	defer s.core.mu.RUnlock()

	chatApplet, exists := s.core.applets["chat"]
	if !exists || chatApplet == nil || chatApplet.Cmd == nil || chatApplet.Cmd.Process == nil {
		return errors.New("tracked chat applet process unavailable")
	}

	s.chatPID = chatApplet.Cmd.Process.Pid
	return nil
}

func (s *bddState) trackedChatAppletProcessPIDShouldRemainTheSame() error {
	if s.core == nil {
		return errors.New("core not initialized")
	}

	s.core.mu.RLock()
	defer s.core.mu.RUnlock()

	chatApplet, exists := s.core.applets["chat"]
	if !exists || chatApplet == nil || chatApplet.Cmd == nil || chatApplet.Cmd.Process == nil {
		return errors.New("tracked chat applet process unavailable")
	}

	currentPID := chatApplet.Cmd.Process.Pid
	if s.chatPID == 0 {
		return errors.New("expected prior chat applet pid capture")
	}
	if s.chatPID != currentPID {
		return fmt.Errorf("expected persistent chat applet pid %d, got %d", s.chatPID, currentPID)
	}

	return nil
}

func (s *bddState) tmuxInitializationShouldCompleteWithoutError() error {
	if s.err != nil {
		return s.err
	}
	return nil
}

func (s *bddState) tmuxCommandsShouldInclude(substring string) error {
	if !strings.Contains(s.tmuxCommands, substring) {
		return fmt.Errorf("expected tmux commands to include %q, got:\n%s", substring, s.tmuxCommands)
	}
	return nil
}

func (s *bddState) tmuxCommandsShouldNotInclude(substring string) error {
	if strings.Contains(s.tmuxCommands, substring) {
		return fmt.Errorf("expected tmux commands to not include %q, got:\n%s", substring, s.tmuxCommands)
	}
	return nil
}

func (s *bddState) tmuxCommandSnippetShouldAppearBefore(first, second string) error {
	firstIdx := strings.Index(s.tmuxCommands, first)
	secondIdx := strings.Index(s.tmuxCommands, second)
	if firstIdx == -1 || secondIdx == -1 {
		return fmt.Errorf("expected tmux commands to include both %q and %q, got:\n%s", first, second, s.tmuxCommands)
	}
	if firstIdx >= secondIdx {
		return fmt.Errorf("expected %q before %q in tmux commands, got:\n%s", first, second, s.tmuxCommands)
	}
	return nil
}

func (s *bddState) tmuxCommandsShouldIncludeAtLeast(substring string, count int) error {
	actual := strings.Count(s.tmuxCommands, substring)
	if actual < count {
		return fmt.Errorf("expected tmux commands to include %q at least %d times, got %d; commands:\n%s", substring, count, actual, s.tmuxCommands)
	}
	return nil
}

func (s *bddState) startupShouldNameWindowZeroAs(windowName string) error {
	expected := "new-session -d -s " + s.core.tmuxSessionName + " -n " + windowName
	if !strings.Contains(s.tmuxCommands, expected) {
		return fmt.Errorf("expected startup to include %q, got:\n%s", expected, s.tmuxCommands)
	}
	return nil
}

func (s *bddState) startupShouldSelectWindowZero() error {
	expected := "select-window -t " + s.core.tmuxSessionName + ":0"
	if !strings.Contains(s.tmuxCommands, expected) {
		return fmt.Errorf("expected startup to include %q, got:\n%s", expected, s.tmuxCommands)
	}
	return nil
}

func (s *bddState) theCoreHasATrackedAppletOnPane(appletName, paneName string) error {
	if s.core == nil {
		return errors.New("core not initialized")
	}

	s.core.mu.Lock()
	defer s.core.mu.Unlock()

	s.core.applets[appletName] = &AppletProcess{
		Name:       appletName,
		PaneName:   paneName,
		Status:     AppletStatusReady,
		StartedAt:  time.Now(),
		CrashCount: 0,
	}

	return nil
}

func (s *bddState) iCaptureTheCoreHealthSnapshot() error {
	if s.core == nil {
		return errors.New("core not initialized")
	}

	s.snapshot = s.core.healthSnapshot()
	return nil
}

func (s *bddState) theHealthSnapshotShouldIncludeSessionID(sessionID string) error {
	if s.snapshot.SessionID != sessionID {
		return fmt.Errorf("expected snapshot session id %q, got %q", sessionID, s.snapshot.SessionID)
	}
	return nil
}

func (s *bddState) theHealthSnapshotShouldIncludePane(paneName string) error {
	for _, pane := range s.snapshot.Panes {
		if pane.Name == paneName {
			return nil
		}
	}
	return fmt.Errorf("expected snapshot to include pane %q", paneName)
}

func (s *bddState) theHealthSnapshotShouldIncludeApplet(appletName string) error {
	for _, applet := range s.snapshot.Applets {
		if applet.Name == appletName {
			return nil
		}
	}
	return fmt.Errorf("expected snapshot to include applet %q", appletName)
}

func (s *bddState) theAppletIsMarkedAsCrashed(appletName string) error {
	if s.core == nil {
		return errors.New("core not initialized")
	}

	s.core.markAppletStatus(appletName, AppletStatusCrashed, errors.New("simulated crash"))
	return nil
}

func (s *bddState) theHealthSnapshotShouldReportAppletStatus(appletName, status string) error {
	for _, applet := range s.snapshot.Applets {
		if applet.Name == appletName {
			if applet.Status != status {
				return fmt.Errorf("expected applet %q status %q, got %q", appletName, status, applet.Status)
			}
			return nil
		}
	}
	return fmt.Errorf("expected applet %q in snapshot", appletName)
}

func (s *bddState) theHealthSnapshotShouldReportAppletCrashCount(appletName string, count int) error {
	for _, applet := range s.snapshot.Applets {
		if applet.Name == appletName {
			if applet.CrashCount != count {
				return fmt.Errorf("expected applet %q crash count %d, got %d", appletName, count, applet.CrashCount)
			}
			return nil
		}
	}
	return fmt.Errorf("expected applet %q in snapshot", appletName)
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	state := &bddState{}

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		state.reset()
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if state.oldPath != "" {
			_ = os.Setenv("PATH", state.oldPath)
		}
		if state.oldTmuxLog != "" {
			_ = os.Setenv("TMUX_LOG", state.oldTmuxLog)
		}
		_ = os.Setenv("AGENTX_CHAT_RUNTIME", state.originalChatRuntime)
		_ = os.Setenv("AGENTX_CHAT_BACKEND", state.originalChatBackend)
		_ = os.Setenv("AGENTX_OLLAMA_HOST", state.originalOllamaHost)
		if state.backendStubServer != nil {
			state.backendStubServer.Close()
			state.backendStubServer = nil
		}
		_ = os.Setenv("AGENTX_PANE_RENDER_MODE", state.oldPaneMode)
		if state.tmpDir != "" {
			_ = os.RemoveAll(state.tmpDir)
		}
		return ctx, nil
	})

	ctx.Step(`^a temporary project directory$`, state.iHaveATemporaryProjectDirectory)
	ctx.Step(`^the project contains template chat applet$`, state.theProjectContainsTemplateChatApplet)
	ctx.Step(`^the project contains flaky chat bridge applet$`, state.theProjectContainsFlakyChatBridgeApplet)
	ctx.Step(`^the project contains malformed chat bridge applet$`, state.theProjectContainsMalformedChatBridgeApplet)
	ctx.Step(`^the project contains error-frame chat bridge applet$`, state.theProjectContainsErrorFrameChatBridgeApplet)
	ctx.Step(`^the project contains empty-chunk chat bridge applet$`, state.theProjectContainsEmptyChunkChatBridgeApplet)
	ctx.Step(`^the project contains startup-latency chat bridge applet$`, state.theProjectContainsStartupLatencyChatBridgeApplet)
	ctx.Step(`^I set chat runtime override to "([^"]*)"$`, state.setChatRuntimeOverride)
	ctx.Step(`^I set chat backend override to "([^"]*)"$`, state.setChatBackendOverride)
	ctx.Step(`^I set ollama host override to "([^"]*)"$`, state.setOllamaHostOverride)
	ctx.Step(`^I start a delayed ollama backend with delay (\d+) ms and status (\d+)$`, state.iStartDelayedOllamaBackendWithStatus)
	ctx.Step(`^I start a sequenced delayed ollama backend with delay (\d+) ms and statuses (\d+) then (\d+)$`, state.iStartSequencedDelayedOllamaBackend)
	ctx.Step(`^a config with username "([^"]*)"$`, state.aConfigWithUsername)
	ctx.Step(`^I ensure session directories for session "([^"]*)"$`, state.iEnsureSessionDirectoriesForSession)
	ctx.Step(`^the session directory structure should exist$`, state.theSessionDirectoryStructureShouldExist)
	ctx.Step(`^the default pane layout$`, state.theDefaultPaneLayout)
	ctx.Step(`^pane names should include "([^"]*)"$`, state.paneNamesShouldInclude)
	ctx.Step(`^an IPC router for session "([^"]*)"$`, state.anIPCRouterForSession)
	ctx.Step(`^I create an IPC FIFO pair for applet "([^"]*)"$`, state.iCreateAnIPCFIFOPairForApplet)
	ctx.Step(`^both FIFO files should exist$`, state.bothFIFOFilesShouldExist)
	ctx.Step(`^FIFO paths should include applet name "([^"]*)"$`, state.fifoPathsShouldIncludeAppletName)
	ctx.Step(`^a core config with username "([^"]*)" and session "([^"]*)"$`, state.aCoreConfigWithUsernameAndSession)
	ctx.Step(`^I construct the AgentX core$`, state.iConstructTheAgentXCore)
	ctx.Step(`^I configure core chat bridge to use prepared applet script with timeout (\d+) ms$`, state.iConfigureCoreChatBridgeToUsePreparedAppletScriptWithTimeoutMs)
	ctx.Step(`^the core session id should be non-empty$`, state.theCoreSessionIDShouldBeNonempty)
	ctx.Step(`^the core tmux session name should include username "([^"]*)"$`, state.theCoreTmuxSessionNameShouldIncludeUsername)
	ctx.Step(`^a context manager with a temporary context directory$`, state.aContextManagerWithATemporaryContextDirectory)
	ctx.Step(`^a canceled context$`, state.aCanceledContext)
	ctx.Step(`^I run the health serve routine$`, state.iRunTheHealthServeRoutine)
	ctx.Step(`^the routine should return context canceled$`, state.theRoutineShouldReturnContextCanceled)
	ctx.Step(`^a constructed AgentX core$`, state.aConstructedAgentXCore)
	ctx.Step(`^I invoke core shutdown$`, state.iInvokeCoreShutdown)
	ctx.Step(`^shutdown should complete without error$`, state.shutdownShouldCompleteWithoutError)
	ctx.Step(`^a fake tmux executable that records commands$`, state.aFakeTmuxExecutableThatRecordsCommands)
	ctx.Step(`^I initialize the tmux session$`, state.iInitializeTheTmuxSession)
	ctx.Step(`^I start the applet supervisor$`, state.iStartTheAppletSupervisor)
	ctx.Step(`^I route input prompt "([^"]*)"$`, state.iRouteInputPrompt)
	ctx.Step(`^I handle input line "([^"]*)"$`, state.iHandleInputLine)
	ctx.Step(`^I reconstruct the AgentX core with the same config$`, state.iReconstructTheAgentXCoreWithTheSameConfig)
	ctx.Step(`^I capture the context turns snapshot$`, state.iCaptureTheContextTurnsSnapshot)
	ctx.Step(`^tmux initialization should complete without error$`, state.tmuxInitializationShouldCompleteWithoutError)
	ctx.Step(`^tmux commands should include "([^"]*)"$`, state.tmuxCommandsShouldInclude)
	ctx.Step(`^tmux commands should not include "([^"]*)"$`, state.tmuxCommandsShouldNotInclude)
	ctx.Step(`^tmux command snippet "([^"]*)" should appear before "([^"]*)"$`, state.tmuxCommandSnippetShouldAppearBefore)
	ctx.Step(`^tmux commands should include "([^"]*)" at least (\d+) times$`, state.tmuxCommandsShouldIncludeAtLeast)
	ctx.Step(`^prompt routing should complete without error$`, state.promptRoutingShouldCompleteWithoutError)
	ctx.Step(`^routed response should equal "([^"]*)"$`, state.routedResponseShouldEqual)
	ctx.Step(`^tmux should include rendered chat response "([^"]*)"$`, state.tmuxShouldIncludeRenderedChatResponse)
	ctx.Step(`^input handling should complete without error$`, state.inputHandlingShouldCompleteWithoutError)
	ctx.Step(`^input response should equal "([^"]*)"$`, state.inputResponseShouldEqual)
	ctx.Step(`^input exit flag should be (true|false)$`, state.inputExitFlagShouldBe)
	ctx.Step(`^context turns should have length (\d+)$`, state.contextTurnsShouldHaveLength)
	ctx.Step(`^context turns should include prompt "([^"]*)"$`, state.contextTurnsShouldIncludePrompt)
	ctx.Step(`^context turns should include response "([^"]*)"$`, state.contextTurnsShouldIncludeResponse)
	ctx.Step(`^I capture the tracked chat applet process pid$`, state.iCaptureTheTrackedChatAppletProcessPID)
	ctx.Step(`^the tracked chat applet process pid should remain the same$`, state.trackedChatAppletProcessPIDShouldRemainTheSame)
	ctx.Step(`^startup should name window 0 as "([^"]*)"$`, state.startupShouldNameWindowZeroAs)
	ctx.Step(`^startup should select window 0$`, state.startupShouldSelectWindowZero)
	ctx.Step(`^the core has a tracked applet "([^"]*)" on pane "([^"]*)"$`, state.theCoreHasATrackedAppletOnPane)
	ctx.Step(`^I capture the core health snapshot$`, state.iCaptureTheCoreHealthSnapshot)
	ctx.Step(`^the health snapshot should include session id "([^"]*)"$`, state.theHealthSnapshotShouldIncludeSessionID)
	ctx.Step(`^the health snapshot should include pane "([^"]*)"$`, state.theHealthSnapshotShouldIncludePane)
	ctx.Step(`^the health snapshot should include applet "([^"]*)"$`, state.theHealthSnapshotShouldIncludeApplet)
	ctx.Step(`^the applet "([^"]*)" is marked as crashed$`, state.theAppletIsMarkedAsCrashed)
	ctx.Step(`^the health snapshot should report applet "([^"]*)" status "([^"]*)"$`, state.theHealthSnapshotShouldReportAppletStatus)
	ctx.Step(`^the health snapshot should report applet "([^"]*)" crash count (\d+)$`, state.theHealthSnapshotShouldReportAppletCrashCount)
}

func TestGoDogUnit(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format: "pretty",
			Paths:  []string{"features"},
			Tags:   "@unit",
		},
	}
	if suite.Run() != 0 {
		t.Fatal("unit godog suite failed")
	}
}

func TestGoDogIntegration(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format: "pretty",
			Paths:  []string{"features"},
			Tags:   "@integration",
		},
	}
	if suite.Run() != 0 {
		t.Fatal("integration godog suite failed")
	}
}

func TestGoDogFunctional(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format: "pretty",
			Paths:  []string{"features"},
			Tags:   "@functional",
		},
	}
	if suite.Run() != 0 {
		t.Fatal("functional godog suite failed")
	}
}

func TestGoDogE2E(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format: "pretty",
			Paths:  []string{"features"},
			Tags:   "@e2e",
		},
	}
	if suite.Run() != 0 {
		t.Fatal("e2e godog suite failed")
	}
}
