package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// TestStartAppletSupervisorTracksDefaultPanes validates that supervisor initialization
// seeds tracked applets for each default pane with ready status.
//
// GIVEN a new core with no tracked applets
// WHEN StartAppletSupervisor is invoked
// THEN each default pane applet is tracked with ready status.
func TestStartAppletSupervisorTracksDefaultPanes(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-app"}
	core := NewAgentXCore(cfg)

	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor returned error: %v", err)
	}

	snapshot := core.healthSnapshot()
	expected := len(core.defaultAppletRuntimeSpecs())
	if len(snapshot.Applets) != expected {
		t.Fatalf("expected %d applets, got %d", expected, len(snapshot.Applets))
	}

	for _, applet := range snapshot.Applets {
		if applet.Status != string(AppletStatusReady) {
			t.Fatalf("expected applet %s to be ready, got %s", applet.Name, applet.Status)
		}
		if applet.Name == "input" && applet.Runtime != string(appletRuntimeGo) {
			t.Fatalf("expected input applet runtime %q, got %q", appletRuntimeGo, applet.Runtime)
		}
		if applet.Name == "context" && applet.Runtime != string(appletRuntimeGo) {
			t.Fatalf("expected context applet runtime %q, got %q", appletRuntimeGo, applet.Runtime)
		}
		if applet.Name == "logs" && applet.Runtime != string(appletRuntimeGo) {
			t.Fatalf("expected logs applet runtime %q, got %q", appletRuntimeGo, applet.Runtime)
		}
		if isDedicatedSystemAppletTab(applet.Name) && applet.Runtime != string(appletRuntimeGo) {
			t.Fatalf("expected dedicated applet %s runtime %q, got %q", applet.Name, appletRuntimeGo, applet.Runtime)
		}
	}
}

// TestMarkAppletStatusTracksCrashLifecycle validates crash transitions and accounting.
//
// GIVEN a tracked applet
// WHEN it is marked crashed and then stopped
// THEN health snapshot reflects status transitions and crash count increment.
func TestMarkAppletStatusTracksCrashLifecycle(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-crash"}
	core := NewAgentXCore(cfg)

	core.mu.Lock()
	core.applets["chat"] = &AppletProcess{Name: "chat", PaneName: "chat", Status: AppletStatusReady}
	core.mu.Unlock()

	core.markAppletStatus("chat", AppletStatusCrashed, errors.New("boom"))
	snapshot := core.healthSnapshot()
	if len(snapshot.Applets) != 1 {
		t.Fatalf("expected 1 applet, got %d", len(snapshot.Applets))
	}
	if snapshot.Applets[0].Status != string(AppletStatusCrashed) {
		t.Fatalf("expected crashed status, got %s", snapshot.Applets[0].Status)
	}
	if snapshot.Applets[0].CrashCount != 1 {
		t.Fatalf("expected crash count 1, got %d", snapshot.Applets[0].CrashCount)
	}

	core.markAppletStatus("chat", AppletStatusStopped, nil)
	snapshot = core.healthSnapshot()
	if snapshot.Applets[0].Status != string(AppletStatusStopped) {
		t.Fatalf("expected stopped status, got %s", snapshot.Applets[0].Status)
	}
	if snapshot.Applets[0].CrashCount != 1 {
		t.Fatalf("expected crash count to stay 1, got %d", snapshot.Applets[0].CrashCount)
	}
}

// TestStartAppletSupervisor_LaunchesPaneAppletProcesses validates pane process launch wiring.
//
// GIVEN initialized tmux session and project-local template applet
// WHEN StartAppletSupervisor is invoked
// THEN each primary pane receives a launched Python applet command.
func TestStartAppletSupervisor_LaunchesPaneAppletProcesses(t *testing.T) {
	t.Setenv("AGENTX_CHAT_BACKEND", "ollama")
	t.Setenv("AGENTX_OLLAMA_HOST", "test-host:11434")
	t.Setenv("AGENTX_OLLAMA_MODEL", "test-model")

	logPath := setupFakeTmux(t)
	projectDir := t.TempDir()
	stageTemplateApplet(t, projectDir)

	cfg := &Config{ProjectDir: projectDir, Username: "dev", SessionID: "s-launch-pane-applets"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor returned error: %v", err)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)

	if !strings.Contains(commands, "AGENTX_APPLET_NAME='chat'") {
		t.Fatalf("expected chat pane applet launch command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "AGENTX_APPLET_NAME='context'") {
		t.Fatalf("expected context pane applet launch command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "--context-widget --core-http") {
		t.Fatalf("expected native go context widget launch args for context pane, got:\n%s", commands)
	}
	if !strings.Contains(commands, "AGENTX_APPLET_NAME='input'") {
		t.Fatalf("expected input pane applet launch command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "--input-widget --core-http") {
		t.Fatalf("expected native go input widget launch args for input pane, got:\n%s", commands)
	}
	if !strings.Contains(commands, "AGENTX_APPLET_NAME='logs'") {
		t.Fatalf("expected logs pane applet launch command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "--logs-widget --core-http") {
		t.Fatalf("expected native go logs widget launch args for logs pane, got:\n%s", commands)
	}
	for _, tab := range dedicatedSystemAppletTabs {
		if !strings.Contains(commands, "AGENTX_APPLET_NAME='"+tab+"'") {
			t.Fatalf("expected dedicated applet launch for %s, got:\n%s", tab, commands)
		}
		if tab == "files" {
			if !strings.Contains(commands, "--filesystem-widget --core-http") {
				t.Fatalf("expected files applet launch args for filesystem widget, got:\n%s", commands)
			}
			continue
		}
		if tab == "configuration" {
			if !strings.Contains(commands, "--settings-widget --core-http") {
				t.Fatalf("expected configuration applet launch args for settings widget, got:\n%s", commands)
			}
			continue
		}
		if !strings.Contains(commands, "AGENTX_CONTEXT_WIDGET_TAB='"+tab+"'") {
			t.Fatalf("expected dedicated applet context widget tab env for %s, got:\n%s", tab, commands)
		}
	}
	if !strings.Contains(commands, "AGENTX_CHAT_BACKEND='ollama'") {
		t.Fatalf("expected backend env in pane launch command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "AGENTX_OLLAMA_HOST='test-host:11434'") {
		t.Fatalf("expected ollama host env in pane launch command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "AGENTX_OLLAMA_MODEL='test-model'") {
		t.Fatalf("expected ollama model env in pane launch command, got:\n%s", commands)
	}
}

// TestBuildPaneAppletLaunchCommand_InputPaneUsesNativeWidget verifies the shared
// applet base launch builder routes input pane through native Go widget mode.
func TestBuildPaneAppletLaunchCommand_InputPaneUsesNativeWidget(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-input-launch"}
	core := NewAgentXCore(cfg)
	core.healthAddr = "127.0.0.1:33333"
	core.runtimeConfig.SubmitTimeout = 10 * time.Second

	base := core.buildAppletBaseRuntimeConfig()
	cmd := core.buildPaneAppletLaunchCommand(appletRuntimeSpec{Name: "input", PaneName: "input", Runtime: appletRuntimeGo}, base, 37, 111)

	if !strings.Contains(cmd, "--input-widget --core-http") {
		t.Fatalf("expected native input widget launch args, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_APPLET_NAME='input'") {
		t.Fatalf("expected input applet env var, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_WIDGET_PANE_HEIGHT='37'") || !strings.Contains(cmd, "AGENTX_WIDGET_PANE_WIDTH='111'") {
		t.Fatalf("expected seeded pane dimensions in input launch command, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_APPLET_RUNTIME='go'") {
		t.Fatalf("expected go applet runtime env var, got:\n%s", cmd)
	}
	if strings.Contains(cmd, "template.py") {
		t.Fatalf("expected input pane not to launch python template applet, got:\n%s", cmd)
	}
}

func TestBuildPaneAppletLaunchCommand_ContextPaneUsesNativeWidget(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-context-launch"}
	core := NewAgentXCore(cfg)
	core.healthAddr = "127.0.0.1:33333"
	core.runtimeConfig.SubmitTimeout = 10 * time.Second

	base := core.buildAppletBaseRuntimeConfig()
	cmd := core.buildPaneAppletLaunchCommand(appletRuntimeSpec{Name: "context", PaneName: "context", Runtime: appletRuntimeGo}, base, 37, 111)

	if !strings.Contains(cmd, "--context-widget --core-http") {
		t.Fatalf("expected native context widget launch args, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_APPLET_NAME='context'") {
		t.Fatalf("expected context applet env var, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_WIDGET_PANE_HEIGHT='37'") || !strings.Contains(cmd, "AGENTX_WIDGET_PANE_WIDTH='111'") {
		t.Fatalf("expected seeded pane dimensions in context launch command, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_APPLET_RUNTIME='go'") {
		t.Fatalf("expected go applet runtime env var, got:\n%s", cmd)
	}
	if strings.Contains(cmd, "template.py") {
		t.Fatalf("expected context pane not to launch python template applet, got:\n%s", cmd)
	}
}

func TestBuildPaneAppletLaunchCommand_LogsPaneUsesNativeWidget(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-logs-launch"}
	core := NewAgentXCore(cfg)
	core.healthAddr = "127.0.0.1:33333"
	core.runtimeConfig.SubmitTimeout = 10 * time.Second

	base := core.buildAppletBaseRuntimeConfig()
	cmd := core.buildPaneAppletLaunchCommand(appletRuntimeSpec{Name: "logs", PaneName: "logs", Runtime: appletRuntimeGo}, base, 37, 111)

	if !strings.Contains(cmd, "--logs-widget --core-http") {
		t.Fatalf("expected native logs widget launch args, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_APPLET_NAME='logs'") {
		t.Fatalf("expected logs applet env var, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_WIDGET_PANE_HEIGHT='37'") || !strings.Contains(cmd, "AGENTX_WIDGET_PANE_WIDTH='111'") {
		t.Fatalf("expected seeded pane dimensions in logs launch command, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_APPLET_RUNTIME='go'") {
		t.Fatalf("expected go applet runtime env var, got:\n%s", cmd)
	}
	if strings.Contains(cmd, "template.py") {
		t.Fatalf("expected logs pane not to launch python template applet, got:\n%s", cmd)
	}
}

func TestBuildPaneAppletLaunchCommand_FilesAppletUsesFilesystemWidget(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-dedicated-launch"}
	core := NewAgentXCore(cfg)
	core.healthAddr = "127.0.0.1:33333"
	core.runtimeConfig.SubmitTimeout = 10 * time.Second

	base := core.buildAppletBaseRuntimeConfig()
	cmd := core.buildPaneAppletLaunchCommand(appletRuntimeSpec{Name: "files", PaneName: "files", Runtime: appletRuntimeGo}, base, 37, 111)

	if !strings.Contains(cmd, "--filesystem-widget --core-http") {
		t.Fatalf("expected filesystem widget launch args for files applet, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_APPLET_NAME='files'") {
		t.Fatalf("expected dedicated applet name env var, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_WIDGET_PANE_HEIGHT='37'") || !strings.Contains(cmd, "AGENTX_WIDGET_PANE_WIDTH='111'") {
		t.Fatalf("expected seeded pane dimensions in files launch command, got:\n%s", cmd)
	}
	if strings.Contains(cmd, "AGENTX_CONTEXT_WIDGET_TAB='files'") {
		t.Fatalf("expected files applet not to use context widget tab env var, got:\n%s", cmd)
	}
}

func TestBuildPaneAppletLaunchCommand_ConfigurationAppletUsesSettingsWidget(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-settings-launch"}
	core := NewAgentXCore(cfg)
	core.healthAddr = "127.0.0.1:33333"
	core.runtimeConfig.SubmitTimeout = 10 * time.Second

	base := core.buildAppletBaseRuntimeConfig()
	cmd := core.buildPaneAppletLaunchCommand(appletRuntimeSpec{Name: "configuration", PaneName: "configuration", Runtime: appletRuntimeGo}, base, 37, 111)

	if !strings.Contains(cmd, "--settings-widget --core-http") {
		t.Fatalf("expected settings widget launch args for configuration applet, got:\n%s", cmd)
	}
	if strings.Contains(cmd, "AGENTX_CONTEXT_WIDGET_TAB='configuration'") {
		t.Fatalf("expected configuration applet not to use context widget tab env var, got:\n%s", cmd)
	}
}

func TestDefaultAppletRuntimeSpecs_DedicatedSystemTabsUseGoRuntime(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-dedicated-runtime-specs"}
	core := NewAgentXCore(cfg)

	specs := core.defaultAppletRuntimeSpecs()
	byName := make(map[string]appletRuntimeSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}

	for _, tab := range dedicatedSystemAppletTabs {
		spec, ok := byName[tab]
		if !ok {
			t.Fatalf("expected dedicated runtime spec for tab %q", tab)
		}
		if spec.Runtime != appletRuntimeGo {
			t.Fatalf("expected dedicated tab %q runtime %q, got %q", tab, appletRuntimeGo, spec.Runtime)
		}
		if spec.PaneName != tab {
			t.Fatalf("expected dedicated tab %q pane name %q, got %q", tab, tab, spec.PaneName)
		}
	}
}

func TestDefaultAppletRuntimeSpecs_ChatAlwaysUsesGoRuntime(t *testing.T) {
	t.Setenv("AGENTX_CHAT_RUNTIME", "python")

	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-chat-runtime-forced-go"}
	core := NewAgentXCore(cfg)

	specs := core.defaultAppletRuntimeSpecs()
	for _, spec := range specs {
		if spec.Name != "chat" {
			continue
		}
		if spec.Runtime != appletRuntimeGo {
			t.Fatalf("expected chat applet runtime %q, got %q", appletRuntimeGo, spec.Runtime)
		}
		return
	}

	t.Fatalf("expected chat runtime spec to be present")
}

// TestBuildPaneAppletLaunchCommand_ChatPaneUsesPythonTemplate verifies the
// shared applet base launch builder routes chat pane through python applet mode.
func TestBuildPaneAppletLaunchCommand_ChatPaneUsesPythonTemplate(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-chat-launch"}
	core := NewAgentXCore(cfg)
	core.healthAddr = "127.0.0.1:33333"
	core.runtimeConfig.SubmitTimeout = 10 * time.Second
	core.chatAppletScript = "/tmp/template.py"

	base := core.buildAppletBaseRuntimeConfig()
	cmd := core.buildPaneAppletLaunchCommand(appletRuntimeSpec{Name: "chat", PaneName: "chat", Runtime: appletRuntimePython}, base, 0, 0)

	if !strings.Contains(cmd, "AGENTX_APPLET_NAME='chat'") {
		t.Fatalf("expected chat applet env var, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_APPLET_RUNTIME='python'") {
		t.Fatalf("expected python applet runtime env var, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_CORE_OWNS_STARTUP_BOOTSTRAP='1'") {
		t.Fatalf("expected startup bootstrap ownership env var, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "/tmp/template.py") {
		t.Fatalf("expected python template applet launch path, got:\n%s", cmd)
	}
	if strings.Contains(cmd, "--input-widget") {
		t.Fatalf("expected chat pane not to use native input widget args, got:\n%s", cmd)
	}
}

func TestBuildPaneAppletLaunchCommand_ChatPaneUsesNativeOutputWidget(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-chat-launch-go"}
	core := NewAgentXCore(cfg)
	core.healthAddr = "127.0.0.1:33333"
	core.runtimeConfig.SubmitTimeout = 10 * time.Second

	base := core.buildAppletBaseRuntimeConfig()
	cmd := core.buildPaneAppletLaunchCommand(appletRuntimeSpec{Name: "chat", PaneName: "chat", Runtime: appletRuntimeGo}, base, 37, 111)

	if !strings.Contains(cmd, "--output-widget --core-http") {
		t.Fatalf("expected native output widget launch args, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_APPLET_NAME='chat'") {
		t.Fatalf("expected chat applet env var, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_WIDGET_PANE_HEIGHT='37'") || !strings.Contains(cmd, "AGENTX_WIDGET_PANE_WIDTH='111'") {
		t.Fatalf("expected seeded pane dimensions in go chat launch command, got:\n%s", cmd)
	}
	if !strings.Contains(cmd, "AGENTX_APPLET_RUNTIME='go'") {
		t.Fatalf("expected go applet runtime env var, got:\n%s", cmd)
	}
	if strings.Contains(cmd, "template.py") {
		t.Fatalf("expected chat pane not to launch python template applet when runtime is go, got:\n%s", cmd)
	}
	if strings.Contains(cmd, "AGENTX_CORE_OWNS_STARTUP_BOOTSTRAP='1'") {
		t.Fatalf("expected native go output widget launch to omit python bootstrap env, got:\n%s", cmd)
	}
}

func TestStartAppletSupervisor_ChatRuntimeGoFromEnv(t *testing.T) {
	t.Setenv("AGENTX_CHAT_RUNTIME", "go")

	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-chat-runtime-go"}
	core := NewAgentXCore(cfg)

	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor returned error: %v", err)
	}

	snapshot := core.healthSnapshot()
	for _, applet := range snapshot.Applets {
		if applet.Name == "chat" && applet.Runtime != string(appletRuntimeGo) {
			t.Fatalf("expected chat applet runtime %q, got %q", appletRuntimeGo, applet.Runtime)
		}
	}
}

func TestStartAppletSupervisor_ChatRuntimeForcedGoWhenEnvRequestsPython(t *testing.T) {
	t.Setenv("AGENTX_CHAT_RUNTIME", "python")

	cfg := &Config{ProjectDir: t.TempDir(), Username: "dev", SessionID: "s-chat-runtime-forced-go-python-env"}
	core := NewAgentXCore(cfg)

	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor returned error: %v", err)
	}

	snapshot := core.healthSnapshot()
	for _, applet := range snapshot.Applets {
		if applet.Name == "chat" && applet.Runtime != string(appletRuntimeGo) {
			t.Fatalf("expected chat applet runtime %q, got %q", appletRuntimeGo, applet.Runtime)
		}
	}
}

func TestStartAppletSupervisor_LaunchesPaneAppletProcesses_GoChatUsesNativeOutputWidget(t *testing.T) {
	t.Setenv("AGENTX_CHAT_RUNTIME", "go")
	t.Setenv("AGENTX_CHAT_BACKEND", "ollama")
	t.Setenv("AGENTX_OLLAMA_HOST", "test-host:11434")
	t.Setenv("AGENTX_OLLAMA_MODEL", "test-model")

	logPath := setupFakeTmux(t)
	projectDir := t.TempDir()
	stageTemplateApplet(t, projectDir)

	cfg := &Config{ProjectDir: projectDir, Username: "dev", SessionID: "s-launch-pane-applets-go-chat"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor returned error: %v", err)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)

	if !strings.Contains(commands, "AGENTX_APPLET_NAME='chat'") {
		t.Fatalf("expected chat pane applet launch command even when runtime is go, got:\n%s", commands)
	}
	if !strings.Contains(commands, "AGENTX_APPLET_RUNTIME='go'") {
		t.Fatalf("expected chat pane launch to preserve go runtime marker, got:\n%s", commands)
	}
	if !strings.Contains(commands, "--output-widget --core-http") {
		t.Fatalf("expected native output widget launch args for go chat pane, got:\n%s", commands)
	}
	if !strings.Contains(commands, "respawn-pane -k -t "+core.tmuxSessionName+":0.0 AGENTX_APPLET_NAME='chat' AGENTX_APPLET_RUNTIME='go'") {
		t.Fatalf("expected chat pane respawn command to use native go runtime, got:\n%s", commands)
	}
	if !strings.Contains(commands, "AGENTX_APPLET_NAME='input'") {
		t.Fatalf("expected input pane applet launch command, got:\n%s", commands)
	}
}
