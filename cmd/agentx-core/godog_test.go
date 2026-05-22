package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	tmuxCommands string
	snapshot     HealthSnapshot
	routedResp   string
	inputResp    string
	inputExit    bool
	contextTurns []ChatTurn
	chatPID      int
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
	s.tmuxCommands = ""
	s.snapshot = HealthSnapshot{}
	s.routedResp = ""
	s.inputResp = ""
	s.inputExit = false
	s.contextTurns = nil
	s.chatPID = 0
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
	s.fakeTmuxLog = logPath

	if err := os.Setenv("PATH", s.tmpDir+":"+s.oldPath); err != nil {
		return fmt.Errorf("failed to set PATH for fake tmux: %w", err)
	}
	if err := os.Setenv("TMUX_LOG", s.fakeTmuxLog); err != nil {
		return fmt.Errorf("failed to set TMUX_LOG for fake tmux: %w", err)
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
		if state.tmpDir != "" {
			_ = os.RemoveAll(state.tmpDir)
		}
		return ctx, nil
	})

	ctx.Step(`^a temporary project directory$`, state.iHaveATemporaryProjectDirectory)
	ctx.Step(`^the project contains template chat applet$`, state.theProjectContainsTemplateChatApplet)
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
