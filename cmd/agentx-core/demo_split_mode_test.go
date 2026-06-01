package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net/http"
	"net/http/httptest"
)

func TestBuildDemoControllerArgs_IncludesControllerFlags(t *testing.T) {
	// GIVEN a split-pane demo controller launch plan
	// WHEN controller arguments are built
	// THEN the binary should receive the live core session and demo flags.
	args := buildDemoControllerArgs(
		"/tmp/agentx",
		&Config{ProjectDir: "/proj", Username: "tester", SessionID: "sess-1", StartupMode: visibleWindowsStartupMode},
		"sess-1",
		"e2e-002",
		"agentx_tester_sess-1",
		"127.0.0.1:34567",
		"/proj/logs/demo/sess-1/stories_board.txt",
	)

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "/tmp/agentx") {
		t.Fatalf("expected executable path in args, got %q", joined)
	}
	if !strings.Contains(joined, "--demo-controller") {
		t.Fatalf("expected demo controller flag in args, got %q", joined)
	}
	if !strings.Contains(joined, "--demo-split") {
		t.Fatalf("expected demo split flag in args, got %q", joined)
	}
	if !strings.Contains(joined, "--startup-mode visible-windows") {
		t.Fatalf("expected startup mode propagation in args, got %q", joined)
	}
	if !strings.Contains(joined, "--demo-core-session agentx_tester_sess-1") {
		t.Fatalf("expected core session flag in args, got %q", joined)
	}
	if !strings.Contains(joined, "--health-addr 127.0.0.1:34567") {
		t.Fatalf("expected health address flag in args, got %q", joined)
	}
	if !strings.Contains(joined, "--demo-stories-file /proj/logs/demo/sess-1/stories_board.txt") {
		t.Fatalf("expected stories board path in args, got %q", joined)
	}
	if !strings.Contains(joined, "--demo-start e2e-002") {
		t.Fatalf("expected demo start selector in args, got %q", joined)
	}
}

func TestSubmitDemoPrompt_PostsPromptToHealthEndpoint(t *testing.T) {
	// GIVEN a live core health endpoint with a submit handler
	// WHEN the controller posts a demo prompt
	// THEN the submit response should be returned to the caller.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/submit" {
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("failed to decode submit payload: %v", err)
			}
			if payload["prompt"] != "hello from split demo" {
				t.Fatalf("unexpected prompt payload: %v", payload)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"response": "Echo: hello from split demo"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	response, err := submitDemoPrompt(context.Background(), server.URL, "hello from split demo")
	if err != nil {
		t.Fatalf("expected submit helper to succeed, got %v", err)
	}
	if response != "Echo: hello from split demo" {
		t.Fatalf("expected routed response, got %q", response)
	}
}

func TestRunDemoSplitMode_UsesVerticalControllerAndLiveCorePanes(t *testing.T) {
	// GIVEN a live core with a reachable health endpoint and a fake tmux binary
	// WHEN split demo mode is launched
	// THEN the tmux layout should create a controller pane and a live core mirror pane.
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmuxPath := filepath.Join(tmpDir, "tmux")
	fakeTmuxScript := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"printf '%s\\n' \"$*\" >> \"${TMUX_LOG}\"\n" +
		"if [[ \"$1\" == \"new-session\" && \"$*\" == *\"-P -F #{pane_id}\"* ]]; then\n" +
		"  printf '%%1\\n'\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [[ \"$1\" == \"split-window\" && \"$*\" == *\"-P -F #{pane_id}\"* ]]; then\n" +
		"  if [[ \"$*\" == *\" -h \"* ]]; then\n" +
		"    printf '%%2\\n'\n" +
		"  else\n" +
		"    printf '%%3\\n'\n" +
		"  fi\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [[ \"$1\" == \"display-message\" ]]; then\n" +
		"  target=\"\"\n" +
		"  for ((i=1; i<=$#; i++)); do\n" +
		"    if [[ \"${!i}\" == \"-t\" ]]; then\n" +
		"      j=$((i+1))\n" +
		"      target=\"${!j}\"\n" +
		"      break\n" +
		"    fi\n" +
		"  done\n" +
		"  if [[ \"$target\" == *\":0\" ]]; then\n" +
		"    printf '0\\n'\n" +
		"    exit 0\n" +
		"  fi\n" +
		"  if [[ \"$target\" == *\":0.2\" ]]; then\n" +
		"    printf '1\\n'\n" +
		"    exit 0\n" +
		"  fi\n" +
		"fi\n" +
		"case \"$1\" in\n" +
		"  new-session|split-window|select-pane|select-layout|attach-session|kill-session|set-window-option|display-message|resize-pane) exit 0 ;;\n" +
		"  *) echo \"unexpected tmux args: $*\" >&2; exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(fakeTmuxPath, []byte(fakeTmuxScript), 0o755); err != nil {
		t.Fatalf("failed to write fake tmux script: %v", err)
	}

	oldPath := os.Getenv("PATH")
	oldTmuxLog := os.Getenv("TMUX_LOG")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	if err := os.Setenv("TMUX_LOG", logPath); err != nil {
		t.Fatalf("failed to set TMUX_LOG: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
		_ = os.Setenv("TMUX_LOG", oldTmuxLog)
	})

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer healthServer.Close()

	core := &AgentXCore{
		SessionID:       "sess-split",
		tmuxSessionName: "agentx_tester_sess-split",
		healthAddr:      strings.TrimPrefix(healthServer.URL, "http://"),
	}
	cfg := &Config{ProjectDir: tmpDir, Username: "tester", SessionID: "sess-split"}

	if err := runDemoSplitMode(context.Background(), cfg, core, "e2e-001"); err != nil {
		t.Fatalf("expected split demo mode to succeed, got %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read tmux log: %v", err)
	}
	commands := string(data)
	if !strings.Contains(commands, "new-session -d -P -F #{pane_id} -s agentx_tester_sess-split_demo -n demo-control bash -lc") {
		t.Fatalf("expected story-browser session bootstrap, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "less -R -c +g") {
		t.Fatalf("expected story board pager command in left-top pane bootstrap command, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "--demo-controller") {
		t.Fatalf("expected controller launch flag in prompt pane command, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "--demo-split") {
		t.Fatalf("expected split-view flag in prompt pane command, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "--demo-stories-file") {
		t.Fatalf("expected stories-board flag in prompt pane command, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "set-window-option -t agentx_tester_sess-split:0 window-size smallest") {
		t.Fatalf("expected core window to use smallest client sizing for split view, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "display-message -p -t agentx_tester_sess-split:0 #{window_zoomed_flag}") {
		t.Fatalf("expected zoom flag check for core window before nested attach, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "display-message -p -t agentx_tester_sess-split:0.2 #{pane_height}") {
		t.Fatalf("expected input pane height check before nested attach, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "resize-pane -t agentx_tester_sess-split:0.2 -y 3") {
		t.Fatalf("expected input pane minimum height enforcement before nested attach, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "select-layout -t agentx_tester_sess-split:0 tiled") {
		t.Fatalf("expected core tiled layout normalization before nested attach, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "select-pane -t agentx_tester_sess-split:0.0") {
		t.Fatalf("expected chat pane focus before nested attach so right-side cursor does not dominate, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "split-window -P -F #{pane_id} -h -p 45 -t %1 bash -lc") {
		t.Fatalf("expected weighted vertical split with live core attach command, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "split-window -P -F #{pane_id} -v -p 35 -t %1") {
		t.Fatalf("expected bottom-left prompt pane split from stories pane, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "--demo-controller") {
		t.Fatalf("expected prompt pane split to launch demo controller command, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "TMUX= tmux attach-session -r -t 'agentx_tester_sess-split'") {
		t.Fatalf("expected right pane to run nested read-only tmux attach before completion message, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "[AgentX Demo] demo complete. Press N or X to exit") {
		t.Fatalf("expected right pane completion guidance after live session exits, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "attach-session -t agentx_tester_sess-split_demo") {
		t.Fatalf("expected final attach to split demo session, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "select-pane -t %1 -T stores") {
		t.Fatalf("expected stories pane title assignment, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "select-pane -t %3 -T testControler") {
		t.Fatalf("expected controller pane title assignment on bottom-left pane, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "select-pane -t %3") {
		t.Fatalf("expected controller focus on bottom-left pane, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "kill-session -t agentx_tester_sess-split_demo") {
		t.Fatalf("expected demo session cleanup, commands:\n%s", commands)
	}
}
