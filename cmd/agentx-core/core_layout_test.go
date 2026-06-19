package main

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
)

// GIVEN tmux session startup is building the initial command
// WHEN the command is generated for first attach view
// THEN window 0 should be explicitly named tui-chat for UX parity.
func TestBuildNewSessionCommand_NamesPrimaryWindowTUIChat(t *testing.T) {
	session := "agentx_test"
	got := buildNewSessionCommand(defaultMultiplexerBackend, session, 120, 40, "")

	found := false
	for i := 0; i < len(got)-1; i++ {
		if got[i] == "-n" && got[i+1] == tmuxPrimaryWindow {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected new session command to include primary window name '-n %s', got %v", tmuxPrimaryWindow, got)
	}
}

func TestBuildNewSessionCommand(t *testing.T) {
	session := "agentx_test"
	got := buildNewSessionCommand(defaultMultiplexerBackend, session, 120, 40, "")
	want := []string{"new-session", "-d", "-s", session, "-n", tmuxPrimaryWindow, "-x", "120", "-y", "40"}

	if len(got) != len(want) {
		t.Fatalf("new session command length mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("new session command arg %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestBuildNewSessionCommand_ZellijIgnoresLayout(t *testing.T) {
	session := "agentx_test"
	layout := "/tmp/zellij-layout.kdl"
	got := buildNewSessionCommand("zellij", session, 120, 40, layout)
	// zellij 0.40+: background session creation; layout not supported via attach --create-background
	want := []string{"attach", "--create-background", session}

	if len(got) != len(want) {
		t.Fatalf("zellij new session command length mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("zellij new session command arg %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestBuildKillSessionCommand_TmuxUsesTargetFlag(t *testing.T) {
	got := buildKillSessionCommand(defaultMultiplexerBackend, "agentx_test")
	want := []string{"kill-session", "-t", "agentx_test"}
	if len(got) != len(want) {
		t.Fatalf("tmux kill session command length mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tmux kill session command arg %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestBuildKillSessionCommand_ZellijUsesDeleteSession(t *testing.T) {
	got := buildKillSessionCommand("zellij", "agentx_test")
	want := []string{"delete-session", "agentx_test"}
	if len(got) != len(want) {
		t.Fatalf("zellij kill session command length mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("zellij kill session command arg %d mismatch: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestLaunchZellijPaneApplets_AppliesLayoutOncePerCoreInstance(t *testing.T) {
	driver := &stubMultiplexerDriver{backendName: "zellij"}
	core := &AgentXCore{
		multiplexerDriver: driver,
		Config: &Config{
			ProjectDir: t.TempDir(),
			Username:   "tester",
		},
		runtimeConfig: CoreRuntimeConfig{
			ChatBackend:   "echo",
			OllamaHost:    "http://localhost:11434",
			OllamaModel:   "llama3.2",
			SubmitTimeout: 30,
		},
		SessionID:          "sess-zellij-layout-once",
		tmuxSessionName:    "agentx_tester_sess-zellij-layout-once",
		coreExecutablePath: "agentx-core",
		healthAddr:         "127.0.0.1:7777",
	}

	if err := core.launchZellijPaneApplets(context.Background()); err != nil {
		t.Fatalf("first layout apply failed: %v", err)
	}
	if err := core.launchZellijPaneApplets(context.Background()); err != nil {
		t.Fatalf("second layout apply should be a no-op, got: %v", err)
	}

	if len(driver.runCalls) != 1 {
		t.Fatalf("expected exactly one zellij layout apply call, got %d (%v)", len(driver.runCalls), driver.runCalls)
	}
	if len(driver.runCalls[0]) != 4 {
		t.Fatalf("expected zellij layout apply arg length 4, got %d (%v)", len(driver.runCalls[0]), driver.runCalls[0])
	}
	if driver.runCalls[0][0] != "--session" || driver.runCalls[0][1] != core.tmuxSessionName || driver.runCalls[0][2] != "--layout" {
		t.Fatalf("unexpected zellij layout apply args: %v", driver.runCalls[0])
	}
}

type blockingRunDriver struct {
	backendName string

	mu          sync.Mutex
	runCalls    [][]string
	runStarted  chan struct{}
	releaseRun  chan struct{}
	runErr      error
	startedOnce sync.Once
}

func (d *blockingRunDriver) BackendName() string {
	if d.backendName == "" {
		return "zellij"
	}
	return d.backendName
}

func (d *blockingRunDriver) Run(_ context.Context, args ...string) error {
	d.mu.Lock()
	d.runCalls = append(d.runCalls, append([]string(nil), args...))
	d.mu.Unlock()

	d.startedOnce.Do(func() {
		close(d.runStarted)
	})
	<-d.releaseRun

	return d.runErr
}

func (d *blockingRunDriver) RunCombined(_ context.Context, _ ...string) (string, error) {
	return "", nil
}

func (d *blockingRunDriver) Capture(_ context.Context, _ ...string) (string, error) {
	return "", nil
}

func (d *blockingRunDriver) AttachSession(_ context.Context, _ string, _ io.Reader, _ io.Writer, _ io.Writer) error {
	return nil
}

func (d *blockingRunDriver) runCallCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.runCalls)
}

func TestLaunchZellijPaneApplets_ConcurrentCallsApplyLayoutOnlyOnce(t *testing.T) {
	driver := &blockingRunDriver{
		backendName: "zellij",
		runStarted:  make(chan struct{}),
		releaseRun:  make(chan struct{}),
	}

	core := &AgentXCore{
		multiplexerDriver: driver,
		Config: &Config{
			ProjectDir: t.TempDir(),
			Username:   "tester",
		},
		runtimeConfig: CoreRuntimeConfig{
			ChatBackend:   "echo",
			OllamaHost:    "http://localhost:11434",
			OllamaModel:   "llama3.2",
			SubmitTimeout: 30,
		},
		SessionID:          "sess-zellij-layout-concurrent",
		tmuxSessionName:    "agentx_tester_sess-zellij-layout-concurrent",
		coreExecutablePath: "agentx-core",
		healthAddr:         "127.0.0.1:7777",
	}

	errCh := make(chan error, 2)
	start := make(chan struct{})

	for i := 0; i < 2; i++ {
		go func() {
			<-start
			errCh <- core.launchZellijPaneApplets(context.Background())
		}()
	}

	close(start)
	<-driver.runStarted

	if got := driver.runCallCount(); got != 1 {
		t.Fatalf("expected exactly one in-flight zellij layout apply call, got %d", got)
	}

	close(driver.releaseRun)

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent launch returned error: %v", err)
		}
	}

	if got := driver.runCallCount(); got != 1 {
		t.Fatalf("expected exactly one total zellij layout apply call, got %d", got)
	}
}

func TestBuildZellijAppletLayout_PlacesInputInBottomSplit(t *testing.T) {
	cmds := map[string]string{
		"chat":    "chat-command",
		"context": "context-command",
		"input":   "input-command",
		"logs":    "logs-command",
	}

	layout := buildZellijAppletLayout(cmds)

	if !strings.Contains(layout, "pane split_direction=\"horizontal\"") {
		t.Fatalf("expected top-level horizontal split for top/bottom layout, got:\n%s", layout)
	}
	if !strings.Contains(layout, "pane split_direction=\"vertical\"") {
		t.Fatalf("expected nested vertical split for chat/context side-by-side layout, got:\n%s", layout)
	}
	if !strings.Contains(layout, "pane name=\"input\" size=\"20%\"") {
		t.Fatalf("expected dedicated input pane in layout, got:\n%s", layout)
	}
	if !strings.Contains(layout, "floating_panes") || !strings.Contains(layout, "pane name=\"logs\"") {
		t.Fatalf("expected floating logs pane block in layout, got:\n%s", layout)
	}
}

func TestResolveStartupWindowSize_UsesEnvironmentWithMinimums(t *testing.T) {
	t.Setenv("LINES", "28")
	t.Setenv("COLUMNS", "92")

	width, height := resolveStartupWindowSize(nil)
	if width != 92 {
		t.Fatalf("expected startup width 92, got %d", width)
	}
	if height != 28 {
		t.Fatalf("expected startup height 28, got %d", height)
	}
}

func TestResolveStartupWindowSize_ClampsMinimums(t *testing.T) {
	t.Setenv("LINES", "12")
	t.Setenv("COLUMNS", "60")

	width, height := resolveStartupWindowSize(nil)
	if width != minStartupWindowWidth {
		t.Fatalf("expected startup width clamp %d, got %d", minStartupWindowWidth, width)
	}
	if height != minStartupWindowHeight {
		t.Fatalf("expected startup height clamp %d, got %d", minStartupWindowHeight, height)
	}
}

func TestBuildTmuxSessionName_SanitizesComponents(t *testing.T) {
	got := buildTmuxSessionName("User Name/Team", "Session 42@Dev")
	want := "agentx_user-name-team_session-42-dev"
	if got != want {
		t.Fatalf("tmux session name mismatch: got %q want %q", got, want)
	}
}

func TestBuildTmuxSessionName_UsesFallbacksForEmptyComponents(t *testing.T) {
	got := buildTmuxSessionName("   ", "$$$")
	want := "agentx_user_session"
	if got != want {
		t.Fatalf("tmux session fallback mismatch: got %q want %q", got, want)
	}
}

func TestSplitCommandsUsePaneIDCapture(t *testing.T) {
	chatPane := "agentx_test:0.0"

	inputSplit := buildInputSplitCommand(chatPane)
	contextSplit := buildContextSplitCommand(chatPane)

	if len(inputSplit) < 4 || inputSplit[1] != "-P" || inputSplit[2] != "-F" || inputSplit[3] != "#{pane_id}" {
		t.Fatalf("input split command must capture pane_id, got %v", inputSplit)
	}
	if len(contextSplit) < 4 || contextSplit[1] != "-P" || contextSplit[2] != "-F" || contextSplit[3] != "#{pane_id}" {
		t.Fatalf("context split command must capture pane_id, got %v", contextSplit)
	}
}

func TestPaneTargets_MapsAllPanesCorrectly(t *testing.T) {
	targets := paneTargets("agentx_test", "%1", "%3", "%4")
	if len(targets) != 4 {
		t.Fatalf("expected 4 pane targets, got %d", len(targets))
	}

	want := map[string]string{
		PaneTitleOutput: "%1",
		PaneTitleInput:  "%3",
		PaneTitleSystem: "%4",
		PaneTitleLogs:   "agentx_test:1.0",
	}

	for _, target := range targets {
		wantTarget, ok := want[target.name]
		if !ok {
			t.Fatalf("unexpected pane target name %q", target.name)
		}
		if target.target != wantTarget {
			t.Fatalf("pane %q target mismatch: got %q want %q", target.name, target.target, wantTarget)
		}
	}
}
