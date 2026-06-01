package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var systemTabAliases = map[string]string{
	"full":               "full",
	"all":                "full",
	"files":              "files",
	"working-memory":     "working-memory",
	"working_memory":     "working-memory",
	"configuration":      "configuration",
	"config":             "configuration",
	"context":            "context",
	"context-history":    "context-history",
	"context_history":    "context-history",
	"history":            "context-history",
	"context-visualizer": "context-visualizer",
	"context_visualizer": "context-visualizer",
	"visualizer":         "context-visualizer",
}

func normalizeSystemTab(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return ""
	}
	return systemTabAliases[normalized]
}

func (ac *AgentXCore) resolveSelectedSystemTab() string {
	statePath := filepath.Join(ac.Config.ProjectDir, ".agentx", "system-panel-tab.txt")
	if data, err := os.ReadFile(statePath); err == nil {
		if tab := normalizeSystemTab(string(data)); tab != "" {
			return tab
		}
	}
	return "full"
}

func trimSingleLine(value string, limit int) string {
	if limit <= 0 {
		return "none"
	}
	singleLine := strings.Join(strings.Fields(value), " ")
	if singleLine == "" {
		return "none"
	}
	if len(singleLine) <= limit {
		return singleLine
	}
	if limit < 4 {
		return singleLine[:limit]
	}
	return strings.TrimSpace(singleLine[:limit-3]) + "..."
}

func safeListDir(path string) []string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return []string{}
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func (ac *AgentXCore) renderSystemSurface(activeTab string) string {
	turns := ac.ContextTurnsSnapshot()
	turnCount := len(turns)
	promptCycle := ac.promptCycleSnapshot()
	selectedTab := normalizeSystemTab(activeTab)
	if selectedTab == "" {
		selectedTab = "full"
	}

	modelName := trimSingleLine(ac.runtimeConfig.OllamaModel, 24)
	backend := trimSingleLine(ac.runtimeConfig.ChatBackend, 12)
	ollamaHost := trimSingleLine(ac.runtimeConfig.OllamaHost, 40)

	entries := safeListDir(ac.Config.ProjectDir)
	preview := []string{"- none"}
	if len(entries) > 0 {
		preview = []string{}
		for _, name := range entries[:minInt(3, len(entries))] {
			entryPath := filepath.Join(ac.Config.ProjectDir, name)
			entryType := "other"
			if info, err := os.Stat(entryPath); err == nil {
				if info.IsDir() {
					entryType = "dir"
				} else {
					entryType = "file"
				}
			}
			preview = append(preview, fmt.Sprintf("- %s: %s", entryType, trimSingleLine(name, 36)))
		}
	}

	lastPrompt := "none"
	lastResponse := "none"
	recentPrompt := "none"
	recentResponse := "none"
	if turnCount > 0 {
		lastPrompt = trimSingleLine(turns[turnCount-1].Prompt, 56)
		lastResponse = trimSingleLine(turns[turnCount-1].Response, 56)
	}
	if turnCount > 1 {
		recentPrompt = trimSingleLine(turns[turnCount-2].Prompt, 56)
		recentResponse = trimSingleLine(turns[turnCount-2].Response, 56)
	}

	userTokens := 0
	assistantTokens := 0
	for _, turn := range turns {
		userTokens += len(strings.Fields(turn.Prompt))
		assistantTokens += len(strings.Fields(turn.Response))
	}
	maxTokens := maxInt(8192, userTokens+assistantTokens)
	consumedPct := int(float64(userTokens+assistantTokens) / float64(maxTokens) * 100)
	remaining := maxInt(0, maxTokens-(userTokens+assistantTokens))

	cycleRows := []string{
		fmt.Sprintf("○ 🤔 Classify %s", formatCycleElapsed(promptCycle.Classify)),
		fmt.Sprintf("○ 💭 Think    %s", formatCycleElapsed(promptCycle.Thinking)),
		fmt.Sprintf("○ 🔧 Tool     %s", formatCycleElapsed(promptCycle.Tool)),
		fmt.Sprintf("○ 🤖 Respond  %s", formatCycleElapsed(promptCycle.Respond)),
	}

	sections := map[string][]string{
		"files": {
			"== FILES ==",
			fmt.Sprintf("project_dir: %s", trimSingleLine(ac.Config.ProjectDir, 64)),
			fmt.Sprintf("entry_count: %d", len(entries)),
			"preview:",
		},
		"configuration": {
			"== CONFIGURATION ==",
			fmt.Sprintf("model: %s", modelName),
			fmt.Sprintf("backend: %s", backend),
			fmt.Sprintf("ollama_host: %s", ollamaHost),
		},
		"context": {
			"== CONTEXT ==",
			fmt.Sprintf("session_id: %s", trimSingleLine(ac.SessionID, 36)),
			fmt.Sprintf("turn_count: %d", turnCount),
			fmt.Sprintf("last_user: %s", lastPrompt),
			fmt.Sprintf("last_agent: %s", lastResponse),
		},
		"working-memory": {
			"== WORKING MEMORY ==",
		},
		"context-visualizer": {
			"== CONTEXT WINDOW ==",
			fmt.Sprintf("model: %s | backend: %s", modelName, backend),
			fmt.Sprintf("consumed: %d%% (%d/%d)", consumedPct, userTokens+assistantTokens, maxTokens),
			"Top Contributors:",
			fmt.Sprintf("  1. 👤 User Prompts    %d%%", int(float64(userTokens)/float64(maxTokens)*100)),
			fmt.Sprintf("  2. 🤖 Agent Response %d%%", int(float64(assistantTokens)/float64(maxTokens)*100)),
			"== CONTEXT VISUALIZER ==",
			fmt.Sprintf("💾 Working Memory      [..................] 0 (0%%)"),
			fmt.Sprintf("🧠 System Prompts      [..................] 0 (0%%)"),
			fmt.Sprintf("👤 User Prompts        [################..] %d (%d%%)", userTokens, int(float64(userTokens)/float64(maxTokens)*100)),
			fmt.Sprintf("📎 Attachments         [..................] 0 (0%%)"),
			fmt.Sprintf("🤔 Thinking            [..................] 0 (0%%)"),
			fmt.Sprintf("🤖 Agent Response      [################..] %d (%d%%)", assistantTokens, int(float64(assistantTokens)/float64(maxTokens)*100)),
			fmt.Sprintf("🔧 Tool Calls          [..................] 0 (0%%)"),
			fmt.Sprintf("░ Remaining            [################..] %d (%d%%)", remaining, int(float64(remaining)/float64(maxTokens)*100)),
			"== PROMPT CYCLE ==",
		},
	}
	if applet, ok := ac.systemAppletHost.Resolve("context-history"); ok {
		sections["context-history"] = applet.RenderCore(SystemAppletCoreContext{
			SessionDir: ac.Config.SessionDataDir(ac.SessionID),
			SessionID: ac.SessionID,
			TurnCount: turnCount,
			Turns:     turns,
		})
	} else {
		sections["context-history"] = []string{
			"== CONTEXT HISTORY ==",
			fmt.Sprintf("history_context_count: %d", turnCount),
			fmt.Sprintf("recent_prompt: %s", recentPrompt),
			fmt.Sprintf("recent_response: %s", recentResponse),
		}
	}
	if applet, ok := ac.systemAppletHost.Resolve("working-memory"); ok {
		sections["working-memory"] = applet.RenderCore(SystemAppletCoreContext{
			SessionDir: ac.Config.SessionDataDir(ac.SessionID),
			SessionID:  ac.SessionID,
		})
	} else {
		sections["working-memory"] = []string{
			"== WORKING MEMORY ==",
			"No facts stored yet.",
		}
	}
	sections["files"] = append(sections["files"], preview...)
	sections["context-visualizer"] = append(sections["context-visualizer"], cycleRows...)

	lines := []string{"[SYSTEM]", fmt.Sprintf("[SYSTEM TAB] active=%s", selectedTab)}
	if selectedTab == "full" {
		for _, tab := range []string{"files", "configuration", "context", "context-history", "working-memory", "context-visualizer"} {
			lines = append(lines, sections[tab]...)
		}
	} else {
		if section, ok := sections[selectedTab]; ok {
			lines = append(lines, section...)
		} else {
			lines = append(lines, sections["context-visualizer"]...)
		}
	}
	lines = append(lines, fmt.Sprintf("turn_count: %d", turnCount))
	return strings.Join(lines, "\n")
}

func formatCycleElapsed(phase PromptCyclePhase) string {
	state := strings.ToLower(strings.TrimSpace(phase.State))
	if state != "running" && state != "done" && state != "failed" {
		return "--:--:--.---"
	}
	elapsed := maxInt64(0, phase.ElapsedMs)
	totalSeconds := elapsed / 1000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	millis := elapsed % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// emitSystemPanelRender builds and emits the system panel string as a JSON line to the context applet (thin renderer).
func (ac *AgentXCore) emitSystemPanelRender(ctx context.Context) error {
	ac.mu.RLock()
	contextApplet, exists := ac.applets["context"]
	ac.mu.RUnlock()
	if !exists {
		return fmt.Errorf("context applet not available or not running")
	}

	target := ac.paneTargetForName(PaneTitleSystem)
	renderText := ac.renderSystemSurface(ac.resolveSelectedSystemTab())
	renderText = clipRenderToPaneHeight(renderText, ac.paneHeight(ctx, target)-1)

	ac.systemRenderMu.Lock()
	if renderText == ac.lastSystemRender {
		ac.systemRenderMu.Unlock()
		return nil
	}
	ac.lastSystemRender = renderText
	ac.systemRenderMu.Unlock()

	if contextApplet.Runtime == appletRuntimeGo || contextApplet.BridgeStdin == nil {
		renderCmd := fmt.Sprintf("printf %s %s", shellSingleQuote("\033[H\033[2J%s\n"), shellSingleQuote(renderText))
		if err := ac.runTmux(ctx, "send-keys", "-t", target, renderCmd, "Enter"); err != nil {
			return fmt.Errorf("failed to emit system panel render via go runtime: %w", err)
		}
		return nil
	}

	payload := map[string]string{"render": renderText}
	enc := json.NewEncoder(contextApplet.BridgeStdin)
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("failed to emit system panel render: %w", err)
	}
	return nil
}

type tmuxPaneTarget struct {
	name   string
	target string
}

// AppletLifecycleStatus represents applet runtime state.
type AppletLifecycleStatus string

const (
	AppletStatusStarting AppletLifecycleStatus = "starting"
	AppletStatusReady    AppletLifecycleStatus = "ready"
	AppletStatusRunning  AppletLifecycleStatus = "running"
	AppletStatusStopped  AppletLifecycleStatus = "stopped"
	AppletStatusCrashed  AppletLifecycleStatus = "crashed"

	tmuxSessionPrefix = "agentx"
	tmuxPrimaryWindow = "tui-chat"
	tmuxLogsWindow    = PaneTitleLogs

	lifecycleStageStartupGreeting = "startup_greeting"
	lifecycleStageSubmitted       = "submitted"
	lifecycleStageClassified      = "classified"
	lifecycleStageThinking        = "thinking"
	lifecycleStageTool            = "tool"
	lifecycleStageFinalResponse   = "final_response"

	startupBootstrapPromptRelativePath = ".agentx/bootstrap-prompt.md"
)

// AgentXCore orchestrates the tmux session, applets, and IPC.
type AgentXCore struct {
	Config                    *Config
	runtimeConfig             CoreRuntimeConfig
	systemAppletHost          SystemAppletHost
	SessionID                 string
	tmuxSessionName           string
	coreExecutablePath        string
	tmuxInitialized           bool
	applets                   map[string]*AppletProcess
	pythonExecutable          string
	chatAppletScript          string
	chatBridgeResponseTimeout time.Duration
	inputHistory              []string
	exitRequested             bool
	mu                        sync.RWMutex
	startedAt                 time.Time
	healthAddr                string // Address for health endpoint
	healthListener            net.Listener
	contextManager            *ContextManager
	lifecycleEventCounter     int
	startupLifecycleEmitted   bool
	paneTargetByName          map[string]string
	shutdownProvider          func()
	lastPromptCycle           PromptCycleStatus
	classifyPhaseStartedAt    time.Time
	thinkingPhaseStartedAt    time.Time
	toolPhaseStartedAt        time.Time
	respondPhaseStartedAt     time.Time
	contextRenderLoopStarted  bool
	systemRenderMu            sync.Mutex
	lastSystemRender          string
}

type submitRequest struct {
	Prompt string `json:"prompt"`
}

type submitResponse struct {
	Response string `json:"response"`
}

// PaneStatus reports pane-level runtime state.
type PaneStatus struct {
	Name   string `json:"name"`
	Applet string `json:"applet"`
	Status string `json:"status"`
}

// AppletStatus reports applet-level runtime state.
type AppletStatus struct {
	Name       string `json:"name"`
	Pane       string `json:"pane"`
	Runtime    string `json:"runtime"`
	Status     string `json:"status"`
	CrashCount int    `json:"crash_count"`
}

// HealthSnapshot captures runtime status for health endpoints.
type HealthSnapshot struct {
	Status        string            `json:"status"`
	SessionID     string            `json:"session_id"`
	UptimeSeconds int64             `json:"uptime_seconds"`
	SubmitRetries uint64            `json:"submit_retries"`
	Panes         []PaneStatus      `json:"panes"`
	Applets       []AppletStatus    `json:"applets"`
	PromptCycle   PromptCycleStatus `json:"prompt_cycle"`
}

// ChatTurn captures one persisted user/assistant exchange.
type ChatTurn struct {
	Prompt    string `json:"prompt"`
	Response  string `json:"response"`
	CreatedAt int64  `json:"created_at"`
}

type PromptCyclePhase struct {
	State     string `json:"state"`
	ElapsedMs int64  `json:"elapsed_ms"`
}

type PromptCycleStatus struct {
	Classify       PromptCyclePhase `json:"classify"`
	ClassifyResult *ClassifyResult  `json:"classify_result,omitempty"`
	Thinking       PromptCyclePhase `json:"thinking"`
	Tool           PromptCyclePhase `json:"tool"`
	Respond        PromptCyclePhase `json:"respond"`
}

// ActivitySnapshot reports session-level activity state for lightweight UI affordances.
type ActivitySnapshot struct {
	SessionID   string            `json:"session_id"`
	State       string            `json:"state"`
	Phase       string            `json:"phase"`
	PromptCycle PromptCycleStatus `json:"prompt_cycle"`
}

func deriveActivityState(promptCycle PromptCycleStatus) (state string, phase string) {
	if promptCycle.Respond.State == "running" {
		return "working", "respond"
	}
	if promptCycle.Tool.State == "running" {
		return "working", "tool"
	}
	if promptCycle.Thinking.State == "running" {
		return "working", "thinking"
	}
	if promptCycle.Classify.State == "running" {
		return "working", "classify"
	}

	if promptCycle.Respond.State == "failed" || promptCycle.Tool.State == "failed" || promptCycle.Thinking.State == "failed" || promptCycle.Classify.State == "failed" {
		return "failed", "none"
	}

	if promptCycle.Respond.State == "done" {
		return "completed", "none"
	}

	return "idle", "none"
}

type chatBridgeRequest struct {
	Type   string `json:"type"`
	Prompt string `json:"prompt"`
}

type chatBridgeResponse struct {
	Type     string `json:"type"`
	Response string `json:"response,omitempty"`
	Delta    string `json:"delta,omitempty"`
	Error    string `json:"error,omitempty"`
}

type appletBaseRuntimeConfig struct {
	SessionID            string
	CoreHTTP             string
	ProjectDir           string
	Username             string
	ChatBackend          string
	OllamaHost           string
	OllamaModel          string
	SubmitTimeoutSeconds string
}

type appletRuntimeKind string

const (
	appletRuntimePython appletRuntimeKind = "python"
	appletRuntimeGo     appletRuntimeKind = "go"
)

type appletRuntimeSpec struct {
	Name     string
	PaneName string
	Runtime  appletRuntimeKind
}

func resolveChatRuntimeKind(raw string) appletRuntimeKind {
	runtime := strings.ToLower(strings.TrimSpace(raw))
	if runtime == "go" {
		return appletRuntimeGo
	}
	return appletRuntimePython
}

func (ac *AgentXCore) defaultAppletRuntimeSpecs() []appletRuntimeSpec {
	specs := make([]appletRuntimeSpec, 0, len(DefaultPaneLayout()))
	chatRuntime := resolveChatRuntimeKind(ac.runtimeConfig.ChatRuntime)
	for _, pane := range DefaultPaneLayout() {
		runtime := appletRuntimePython
		if pane.Name == "input" || pane.Name == "context" || pane.Name == "logs" {
			runtime = appletRuntimeGo
		}
		if pane.Name == "chat" {
			runtime = chatRuntime
		}
		specs = append(specs, appletRuntimeSpec{
			Name:     pane.Name,
			PaneName: pane.Name,
			Runtime:  runtime,
		})
	}
	return specs
}

// AppletProcess tracks a runtime-managed applet process and status.
type AppletProcess struct {
	Name         string
	PaneName     string
	Runtime      appletRuntimeKind
	Status       AppletLifecycleStatus
	LastError    string
	HandlePrompt func(context.Context, string) (string, error)
	Cmd          *exec.Cmd
	Cancel       context.CancelFunc
	DoneChan     chan error
	BridgeStdin  io.WriteCloser
	BridgeStdout *bufio.Scanner
	BridgeMu     sync.Mutex
	StartedAt    time.Time
	CrashCount   int
}

// NewAgentXCore creates a new orchestrator instance.
func NewAgentXCore(cfg *Config) *AgentXCore {
	sessionID := cfg.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("agentx_%d", time.Now().Unix())
	}
	runtimeConfig := resolveCoreRuntimeConfig(cfg.ProjectDir)
	coreExecutablePath := "agentx-core"
	if resolvedPath, err := os.Executable(); err == nil && strings.TrimSpace(resolvedPath) != "" {
		coreExecutablePath = resolvedPath
	}

	core := &AgentXCore{
		Config:                    cfg,
		runtimeConfig:             runtimeConfig,
		systemAppletHost:          newSystemAppletHost(),
		SessionID:                 sessionID,
		tmuxSessionName:           buildTmuxSessionName(cfg.Username, sessionID),
		coreExecutablePath:        coreExecutablePath,
		applets:                   make(map[string]*AppletProcess),
		pythonExecutable:          "python3",
		chatAppletScript:          filepath.Join(cfg.ProjectDir, "applets", "template.py"),
		chatBridgeResponseTimeout: runtimeConfig.ChatBridgeResponseTimeout,
		inputHistory:              make([]string, 0),
		startedAt:                 time.Now(),
		healthAddr:                "127.0.0.1:0",
		paneTargetByName:          make(map[string]string),
		lastPromptCycle: PromptCycleStatus{
			Classify: PromptCyclePhase{State: "pending", ElapsedMs: 0},
			Thinking: PromptCyclePhase{State: "pending", ElapsedMs: 0},
			Tool:     PromptCyclePhase{State: "idle", ElapsedMs: 0},
			Respond:  PromptCyclePhase{State: "pending", ElapsedMs: 0},
		},
	}
	core.contextManager = NewContextManager(cfg.SessionContextDir(sessionID))
	core.contextManager.SetSessionMetadata(core.SessionID, core.startedAt)
	core.contextManager.SetSnapshotProvider(core.healthSnapshot)
	core.contextManager.SetSubmitExecutionTimeout(runtimeConfig.SubmitExecutionTimeout)
	core.contextManager.SetSubmitProvider(func(ctx context.Context, prompt string) (string, error) {
		response, shouldExit, err := core.HandleInputLine(ctx, prompt)
		if shouldExit {
			go core.requestRuntimeShutdown()
		}
		return response, err
	})
	core.contextManager.SetShutdownProvider(func(context.Context) error {
		core.requestRuntimeShutdown()
		return nil
	})

	return core
}

func (ac *AgentXCore) SetShutdownProvider(provider func()) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.shutdownProvider = provider
}

func (ac *AgentXCore) requestRuntimeShutdown() {
	ac.mu.RLock()
	provider := ac.shutdownProvider
	ac.mu.RUnlock()
	if provider != nil {
		provider()
	}
	if err := ac.runTmux(context.Background(), "kill-session", "-t", ac.tmuxSessionName); err != nil && !isTmuxMissingSessionError(err) {
		log.Printf("[AgentX Core] Runtime shutdown session kill failed: %v", err)
	}
}

func (ac *AgentXCore) FocusInputPane(ctx context.Context) error {
	return ac.runTmux(ctx, "select-pane", "-t", ac.paneTargetForName(PaneTitleInput))
}

// InitializeTmuxSession creates the tmux session and panes with the designed layout:
// Top (80% height): Chat (80% width left) | Context (20% width right)
// Bottom (20% height): Input (full width)
// Separate window: Logs (hidden, navigable via ctrl-b)
func (ac *AgentXCore) InitializeTmuxSession(ctx context.Context) error {
	// Ensure session directories exist.
	if err := ac.Config.EnsureSessionDirs(ac.SessionID); err != nil {
		return err
	}

	if err := ac.runTmux(ctx, buildNewSessionCommand(ac.tmuxSessionName)...); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	if ac.Config.StartupMode == visibleWindowsStartupMode {
		if err := ac.initializeVisibleWindowsTmuxLayout(ctx); err == nil {
			if err := ac.runTmux(ctx, "select-window", "-t", ac.tmuxSessionName+":2"); err != nil {
				return fmt.Errorf("failed to select input window in visible-windows mode: %w", err)
			}
			if err := ac.runTmux(ctx, "select-pane", "-t", ac.paneTargetForName(PaneTitleInput)); err != nil {
				return fmt.Errorf("failed to focus input pane in visible-windows mode: %w", err)
			}

			log.Printf("[AgentX Core] tmux session '%s' initialized in startup mode: visible-windows", ac.tmuxSessionName)
			ac.tmuxInitialized = true
			return nil
		}

		log.Printf("[AgentX Core] startup mode '%s' failed; falling back to default layout", visibleWindowsStartupMode)
		if killErr := ac.runTmux(ctx, "kill-session", "-t", ac.tmuxSessionName); killErr != nil && !isTmuxMissingSessionError(killErr) {
			return fmt.Errorf("failed to reset tmux session after visible-windows startup failure: %w", killErr)
		}
		if err := ac.runTmux(ctx, buildNewSessionCommand(ac.tmuxSessionName)...); err != nil {
			return fmt.Errorf("failed to recreate tmux session for default layout fallback: %w", err)
		}
	}

	if err := ac.initializeDefaultTmuxLayout(ctx); err != nil {
		return err
	}

	// Keep startup cursor in the interactive input pane.
	if err := ac.runTmux(ctx, "select-pane", "-t", ac.paneTargetForName(PaneTitleInput)); err != nil {
		return fmt.Errorf("failed to focus input pane: %w", err)
	}

	log.Printf("[AgentX Core] tmux session '%s' initialized with layout: chat(80x80)|context(20x80) top, input(100x20) bottom, logs hidden", ac.tmuxSessionName)
	ac.tmuxInitialized = true
	return nil
}

func (ac *AgentXCore) initializeVisibleWindowsTmuxLayout(ctx context.Context) error {
	if err := ac.setPaneTitle(ctx, ac.tmuxSessionName+":0.0", PaneTitleOutput); err != nil {
		return fmt.Errorf("failed to set output pane title: %w", err)
	}

	if err := ac.runTmux(ctx, "new-window", "-t", ac.tmuxSessionName+":1", "-n", tmuxLogsWindow); err != nil {
		return fmt.Errorf("failed to create logs window in visible-windows mode: %w", err)
	}
	if err := ac.setPaneTitle(ctx, ac.tmuxSessionName+":1.0", PaneTitleLogs); err != nil {
		return fmt.Errorf("failed to set logs pane title in visible-windows mode: %w", err)
	}

	if err := ac.runTmux(ctx, "new-window", "-t", ac.tmuxSessionName+":2", "-n", PaneTitleInput); err != nil {
		return fmt.Errorf("failed to create input window in visible-windows mode: %w", err)
	}
	if err := ac.setPaneTitle(ctx, ac.tmuxSessionName+":2.0", PaneTitleInput); err != nil {
		return fmt.Errorf("failed to set input pane title in visible-windows mode: %w", err)
	}

	if err := ac.runTmux(ctx, "new-window", "-t", ac.tmuxSessionName+":3", "-n", PaneTitleSystem); err != nil {
		return fmt.Errorf("failed to create system window in visible-windows mode: %w", err)
	}
	if err := ac.setPaneTitle(ctx, ac.tmuxSessionName+":3.0", PaneTitleSystem); err != nil {
		return fmt.Errorf("failed to set system pane title in visible-windows mode: %w", err)
	}

	ac.paneTargetByName[PaneTitleOutput] = ac.tmuxSessionName + ":0.0"
	ac.paneTargetByName["chat"] = ac.tmuxSessionName + ":0.0"
	ac.paneTargetByName[PaneTitleLogs] = ac.tmuxSessionName + ":1.0"
	ac.paneTargetByName["logs"] = ac.tmuxSessionName + ":1.0"
	ac.paneTargetByName[PaneTitleInput] = ac.tmuxSessionName + ":2.0"
	ac.paneTargetByName[PaneTitleSystem] = ac.tmuxSessionName + ":3.0"
	ac.paneTargetByName["context"] = ac.tmuxSessionName + ":3.0"

	if strings.TrimSpace(ac.Config.LayoutFile) != "" {
		log.Printf("[AgentX Core] startup mode '%s': layout overlay is ignored", visibleWindowsStartupMode)
	}

	return nil
}

func (ac *AgentXCore) initializeDefaultTmuxLayout(ctx context.Context) error {

	chatPaneTarget := ac.tmuxSessionName + ":0.0"
	if err := ac.setPaneTitle(ctx, chatPaneTarget, PaneTitleOutput); err != nil {
		return fmt.Errorf("failed to set chat pane title: %w", err)
	}

	inputPaneTarget, err := ac.runTmuxCapture(ctx, buildInputSplitCommand(chatPaneTarget)...)
	if err != nil {
		return fmt.Errorf("failed to split input pane: %w", err)
	}
	if err := ac.setPaneTitle(ctx, inputPaneTarget, PaneTitleInput); err != nil {
		return fmt.Errorf("failed to set input pane title: %w", err)
	}

	contextPaneTarget, err := ac.runTmuxCapture(ctx, buildContextSplitCommand(chatPaneTarget)...)
	if err != nil {
		return fmt.Errorf("failed to split context pane: %w", err)
	}
	if err := ac.setPaneTitle(ctx, contextPaneTarget, PaneTitleSystem); err != nil {
		return fmt.Errorf("failed to set context pane title: %w", err)
	}

	if err := ac.runTmux(ctx, "new-window", "-t", ac.tmuxSessionName+":1", "-n", tmuxLogsWindow); err != nil {
		return fmt.Errorf("failed to create logs window: %w", err)
	}

	ac.paneTargetByName[PaneTitleOutput] = chatPaneTarget
	ac.paneTargetByName["chat"] = chatPaneTarget
	ac.paneTargetByName[PaneTitleInput] = inputPaneTarget
	ac.paneTargetByName[PaneTitleSystem] = contextPaneTarget
	ac.paneTargetByName["context"] = contextPaneTarget
	ac.paneTargetByName[PaneTitleLogs] = ac.tmuxSessionName + ":1.0"
	ac.paneTargetByName["logs"] = ac.tmuxSessionName + ":1.0"

	if err := ac.applyOptionalLayoutOverlay(ctx); err != nil {
		return fmt.Errorf("failed to apply optional layout overlay: %w", err)
	}

	if err := ac.runTmux(ctx, "select-window", "-t", ac.tmuxSessionName+":0"); err != nil {
		return fmt.Errorf("failed to re-select primary window: %w", err)
	}

	if strings.TrimSpace(ac.Config.LayoutFile) != "" {
		if err := ac.refreshPaneTargetsFromTitles(ctx); err != nil {
			log.Printf("[AgentX Core] Warning: could not refresh pane targets after layout overlay: %v", err)
		}
	}

	return nil
}

func (ac *AgentXCore) runTmux(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	return cmd.Run()
}

func (ac *AgentXCore) runTmuxCapture(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func paneTargets(sessionName, chatTarget, inputTarget, contextTarget string) []tmuxPaneTarget {
	return []tmuxPaneTarget{
		{name: PaneTitleOutput, target: chatTarget},
		{name: PaneTitleInput, target: inputTarget},
		{name: PaneTitleSystem, target: contextTarget},
		{name: PaneTitleLogs, target: sessionName + ":1.0"},
	}
}

func buildNewSessionCommand(sessionName string) []string {
	return []string{"new-session", "-d", "-s", sessionName, "-n", tmuxPrimaryWindow, "-x", "120", "-y", "40"}
}

func buildTmuxSessionName(username string, sessionID string) string {
	usernamePart := sanitizeTmuxNameComponent(strings.TrimSpace(username), "user")
	sessionPart := sanitizeTmuxNameComponent(strings.TrimSpace(sessionID), "session")
	return fmt.Sprintf("%s_%s_%s", tmuxSessionPrefix, usernamePart, sessionPart)
}

func sanitizeTmuxNameComponent(raw string, fallback string) string {
	cleaned := strings.ToLower(strings.TrimSpace(raw))
	if cleaned == "" {
		return fallback
	}

	var builder strings.Builder
	builder.Grow(len(cleaned))
	lastDash := false
	for _, r := range cleaned {
		isLetter := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if isLetter || isDigit || r == '_' || r == '-' {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteRune('-')
			lastDash = true
		}
	}

	result := strings.Trim(builder.String(), "-_")
	if result == "" {
		return fallback
	}
	return result
}

func buildInputSplitCommand(chatPaneTarget string) []string {
	return []string{"split-window", "-P", "-F", "#{pane_id}", "-t", chatPaneTarget, "-v", "-p", "20"}
}

func buildContextSplitCommand(chatPaneTarget string) []string {
	return []string{"split-window", "-P", "-F", "#{pane_id}", "-t", chatPaneTarget, "-h", "-p", "20"}
}

func (ac *AgentXCore) applyOptionalLayoutOverlay(ctx context.Context) error {
	layoutFile := strings.TrimSpace(ac.Config.LayoutFile)
	if layoutFile == "" {
		return nil
	}

	if _, err := os.Stat(layoutFile); err != nil {
		log.Printf("[AgentX Core] Layout overlay skipped: file not found (%s)", layoutFile)
		return nil
	}

	tmuxpPath, err := exec.LookPath("tmuxp")
	if err != nil {
		log.Printf("[AgentX Core] Layout overlay skipped: tmuxp not found in PATH")
		return nil
	}

	cmd := exec.CommandContext(ctx, tmuxpPath, "load", "-y", "-d", layoutFile)
	cmd.Env = append(os.Environ(), "SESSION="+ac.tmuxSessionName)
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		log.Printf("[AgentX Core] Layout overlay failed, continuing with default layout: %v | output: %s", runErr, strings.TrimSpace(string(output)))
		return nil
	}

	if err := ac.ensureOwnedWindowNames(ctx); err != nil {
		log.Printf("[AgentX Core] Layout overlay applied but could not re-assert owned window names: %v", err)
	}

	log.Printf("[AgentX Core] Layout overlay applied from %s", layoutFile)
	return nil
}

func (ac *AgentXCore) ensureOwnedWindowNames(ctx context.Context) error {
	if err := ac.runTmux(ctx, "rename-window", "-t", ac.tmuxSessionName+":0", tmuxPrimaryWindow); err != nil {
		return err
	}

	if err := ac.runTmux(ctx, "rename-window", "-t", ac.tmuxSessionName+":1", tmuxLogsWindow); err != nil {
		if createErr := ac.runTmux(ctx, "new-window", "-t", ac.tmuxSessionName+":1", "-n", tmuxLogsWindow); createErr != nil {
			return createErr
		}
	}

	return nil
}

func (ac *AgentXCore) refreshPaneTargetsFromTitles(ctx context.Context) error {
	output, err := ac.runTmuxCapture(ctx, "list-panes", "-t", ac.tmuxSessionName+":0", "-F", "#{pane_id}|#{pane_title}")
	if err != nil {
		return err
	}

	resolved := map[string]string{}
	for _, rawLine := range strings.Split(strings.TrimSpace(output), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		paneID := strings.TrimSpace(parts[0])
		title := strings.TrimSpace(parts[1])
		if paneID == "" || title == "" {
			continue
		}
		resolved[title] = paneID
	}

	for _, required := range []string{PaneTitleOutput, PaneTitleInput, PaneTitleSystem} {
		if strings.TrimSpace(resolved[required]) == "" {
			return fmt.Errorf("pane title %q not found", required)
		}
	}

	ac.paneTargetByName[PaneTitleOutput] = resolved[PaneTitleOutput]
	ac.paneTargetByName["chat"] = resolved[PaneTitleOutput]
	ac.paneTargetByName[PaneTitleInput] = resolved[PaneTitleInput]
	ac.paneTargetByName[PaneTitleSystem] = resolved[PaneTitleSystem]
	ac.paneTargetByName["context"] = resolved[PaneTitleSystem]
	ac.paneTargetByName[PaneTitleLogs] = ac.tmuxSessionName + ":1.0"
	ac.paneTargetByName["logs"] = ac.tmuxSessionName + ":1.0"

	return nil
}

// StartAppletSupervisor launches Python applets in goroutines.
func (ac *AgentXCore) StartAppletSupervisor(ctx context.Context) error {
	ac.mu.Lock()
	shouldLaunchPaneApplets := ac.tmuxInitialized
	ac.mu.Unlock()

	for _, spec := range ac.defaultAppletRuntimeSpecs() {
		ac.mu.Lock()
		applet := ac.ensureTrackedAppletLocked(spec.Name, spec.PaneName)
		if applet.HandlePrompt != nil {
			ac.mu.Unlock()
			continue
		}

		handler := defaultPromptHandler(spec.Name)
		if spec.Name == "chat" {
			if spec.Runtime == appletRuntimeGo {
				handler = ac.goChatPromptHandler()
			} else {
				handler = ac.pythonChatPromptHandler()
			}
		}

		applet.Runtime = spec.Runtime
		applet.HandlePrompt = handler
		applet.Status = AppletStatusReady
		ac.mu.Unlock()
	}

	if shouldLaunchPaneApplets {
		if err := ac.launchPaneAppletProcesses(ctx); err != nil {
			return err
		}
	} else {
		ac.startContextRenderLoopIfNeeded(ctx)
	}

	ac.mu.RLock()
	trackedApplets := len(ac.applets)
	ac.mu.RUnlock()

	ac.emitStartupGreetingLifecycleEvent(ctx)
	log.Printf("[AgentX Core] Applet supervisor ready (%d tracked applets)", trackedApplets)
	return nil
}

func (ac *AgentXCore) startContextRenderLoopIfNeeded(ctx context.Context) {
	ac.mu.Lock()
	if ac.contextRenderLoopStarted {
		ac.mu.Unlock()
		return
	}

	contextApplet, exists := ac.applets["context"]
	if !exists || contextApplet.Runtime != appletRuntimeGo {
		ac.mu.Unlock()
		return
	}

	ac.contextRenderLoopStarted = true
	ac.mu.Unlock()

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		_ = ac.emitSystemPanelRender(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = ac.emitSystemPanelRender(ctx)
			}
		}
	}()
}

func clipRenderToPaneHeight(renderText string, paneHeight int) string {
	if paneHeight <= 0 {
		return renderText
	}

	lines := strings.Split(renderText, "\n")
	if len(lines) <= paneHeight {
		return renderText
	}

	if paneHeight <= 4 {
		return strings.Join(lines[:paneHeight], "\n")
	}

	headCount := paneHeight - 3
	if headCount < 2 {
		headCount = 2
	}
	tailCount := 2
	if headCount+tailCount+1 > paneHeight {
		tailCount = maxInt(1, paneHeight-headCount-1)
	}

	overflow := len(lines) - (headCount + tailCount)
	if overflow < 0 {
		overflow = 0
	}

	clipped := make([]string, 0, paneHeight)
	clipped = append(clipped, lines[:headCount]...)
	clipped = append(clipped, fmt.Sprintf("... (%d lines truncated to fit pane)", overflow))
	clipped = append(clipped, lines[len(lines)-tailCount:]...)
	return strings.Join(clipped, "\n")
}

func (ac *AgentXCore) paneHeight(ctx context.Context, paneTarget string) int {
	output, err := ac.runTmuxCapture(ctx, "display-message", "-p", "-t", paneTarget, "#{pane_height}")
	if err != nil {
		return 40
	}
	height, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil || height <= 0 {
		return 40
	}
	return height
}

func promptLifecycleStages() []string {
	return []string{
		lifecycleStageSubmitted,
		lifecycleStageClassified,
		lifecycleStageThinking,
		lifecycleStageTool,
		lifecycleStageFinalResponse,
	}
}

func lifecycleEventDetails(stage string, prompt string, response string) string {
	switch stage {
	case lifecycleStageSubmitted, lifecycleStageClassified, lifecycleStageThinking:
		return fmt.Sprintf("prompt_chars=%d", len(strings.TrimSpace(prompt)))
	case lifecycleStageTool:
		return "tool_activity=none"
	case lifecycleStageFinalResponse:
		return fmt.Sprintf("response_chars=%d", len(strings.TrimSpace(response)))
	default:
		return ""
	}
}

func (ac *AgentXCore) emitStartupGreetingLifecycleEvent(ctx context.Context) {
	ac.mu.Lock()
	if ac.startupLifecycleEmitted {
		ac.mu.Unlock()
		return
	}
	ac.startupLifecycleEmitted = true
	ac.mu.Unlock()

	ac.emitLifecycleEvent(ctx, lifecycleStageStartupGreeting, "hook=runtime_ready")
}

func (ac *AgentXCore) emitLifecycleEvent(ctx context.Context, stage string, details string) {
	trimmedStage := strings.TrimSpace(stage)
	if trimmedStage == "" {
		return
	}

	ac.mu.Lock()
	ac.lifecycleEventCounter++
	seq := ac.lifecycleEventCounter
	ac.mu.Unlock()

	message := fmt.Sprintf("[lifecycle] seq=%d stage=%s", seq, trimmedStage)
	trimmedDetails := strings.TrimSpace(details)
	if trimmedDetails != "" {
		message += " details=" + trimmedDetails
	}

	if err := ac.renderStartupStatus(ctx, message); err != nil {
		log.Printf("[AgentX Core] Lifecycle log render failed: %v", err)
	}
}

func (ac *AgentXCore) renderStartupStatus(ctx context.Context, message string) error {
	renderCmd := fmt.Sprintf("echo %s", shellSingleQuote(message))
	if err := ac.runTmux(ctx, "send-keys", "-t", ac.paneTargetForName(PaneTitleLogs), renderCmd, "Enter"); err != nil {
		trimmedMessage := strings.TrimSpace(message)
		if trimmedMessage == "" {
			trimmedMessage = "startup status unavailable"
		}
		log.Printf("[AgentX Core] Logs pane unavailable for startup status: %s (err=%v)", trimmedMessage, err)
		return err
	}
	return nil
}

func (ac *AgentXCore) loadStartupBootstrapPrompt() (string, bool) {
	promptPath := filepath.Join(ac.Config.ProjectDir, startupBootstrapPromptRelativePath)
	data, err := os.ReadFile(promptPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[AgentX Core] Startup bootstrap prompt read failed: %v", err)
		}
		return "", false
	}

	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", false
	}

	return prompt, true
}

// RunStartupBootstrap routes an optional startup prompt through the standard input pipeline.
// The bootstrap executes once per session by skipping when persisted turns already exist.
func (ac *AgentXCore) RunStartupBootstrap(ctx context.Context) error {
	if len(ac.ContextTurnsSnapshot()) > 0 {
		_ = ac.renderStartupStatus(ctx, "[startup] bootstrap skipped: existing session turns")
		return nil
	}

	bootstrapPrompt, exists := ac.loadStartupBootstrapPrompt()
	if !exists {
		_ = ac.renderStartupStatus(ctx, "[startup] bootstrap skipped: no bootstrap prompt configured")
		return nil
	}

	_ = ac.renderStartupStatus(ctx, "[startup] bootstrap starting")
	response, err := ac.RouteInputPrompt(ctx, bootstrapPrompt)
	if err != nil {
		_ = ac.renderStartupStatus(ctx, fmt.Sprintf("[startup] bootstrap failed: %s", err.Error()))
		return err
	}

	_ = ac.renderStartupStatus(ctx, fmt.Sprintf("[startup] bootstrap complete response_chars=%d", len(strings.TrimSpace(response))))
	return nil
}

func (ac *AgentXCore) launchPaneAppletProcesses(ctx context.Context) error {
	base := ac.buildAppletBaseRuntimeConfig()

	for _, spec := range ac.defaultAppletRuntimeSpecs() {
		if spec.Runtime == appletRuntimeGo && spec.Name != "input" && spec.Name != "context" && spec.Name != "chat" && spec.Name != "logs" {
			continue
		}
		if spec.Runtime == appletRuntimePython {
			if _, err := os.Stat(ac.chatAppletScript); err != nil {
				log.Printf("[AgentX Core] Pane applet launch skipped for %s (template unavailable): %v", spec.Name, err)
				continue
			}
		}

		launchCmd := ac.buildPaneAppletLaunchCommand(spec, base)

		if err := ac.runTmux(ctx, "respawn-pane", "-k", "-t", ac.paneTargetForName(spec.PaneName), launchCmd); err != nil {
			return fmt.Errorf("failed launching %s pane applet: %w", spec.Name, err)
		}
	}

	return nil
}

func (ac *AgentXCore) buildAppletBaseRuntimeConfig() appletBaseRuntimeConfig {
	chatBackend := strings.TrimSpace(ac.runtimeConfig.ChatBackend)
	if chatBackend == "" {
		chatBackend = defaultChatBackend
	}
	ollamaHost := strings.TrimSpace(ac.runtimeConfig.OllamaHost)
	if ollamaHost == "" {
		ollamaHost = defaultOllamaHost
	}
	ollamaModel := strings.TrimSpace(ac.runtimeConfig.OllamaModel)
	if ollamaModel == "" {
		ollamaModel = defaultOllamaModel
	}

	return appletBaseRuntimeConfig{
		SessionID:            ac.SessionID,
		CoreHTTP:             "http://" + ac.healthAddr,
		ProjectDir:           ac.Config.ProjectDir,
		Username:             ac.Config.Username,
		ChatBackend:          chatBackend,
		OllamaHost:           ollamaHost,
		OllamaModel:          ollamaModel,
		SubmitTimeoutSeconds: strconv.FormatInt(int64(ac.runtimeConfig.SubmitTimeout/time.Second), 10),
	}
}

func shellEnvPrefix(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, shellSingleQuote(env[key])))
	}
	return strings.Join(parts, " ")
}

func (ac *AgentXCore) buildPaneAppletLaunchCommand(spec appletRuntimeSpec, base appletBaseRuntimeConfig) string {
	baseEnv := map[string]string{
		"AGENTX_APPLET_NAME":         spec.Name,
		"AGENTX_SESSION_ID":          base.SessionID,
		"AGENTX_CORE_HTTP":           base.CoreHTTP,
		"AGENTX_PROJECT_DIR":         base.ProjectDir,
		"AGENTX_USERNAME":            base.Username,
		"AGENTX_APPLET_RUNTIME":      string(spec.Runtime),
		"AGENTX_SUBMIT_TIMEOUT_SEC":  base.SubmitTimeoutSeconds,
		"AGENTX_CHAT_BACKEND":        base.ChatBackend,
		"AGENTX_OLLAMA_HOST":         base.OllamaHost,
		"AGENTX_OLLAMA_MODEL":        base.OllamaModel,
		"AGENTX_CORE_OWNS_STARTUP_BOOTSTRAP": "1",
	}

	if spec.Runtime == appletRuntimeGo {
		delete(baseEnv, "AGENTX_CORE_OWNS_STARTUP_BOOTSTRAP")
		if spec.Name == "chat" {
			return fmt.Sprintf(
				"%s %s --output-widget --core-http %s",
				shellEnvPrefix(baseEnv),
				shellSingleQuote(ac.coreExecutablePath),
				shellSingleQuote(base.CoreHTTP),
			)
		}
		if spec.Name == "logs" {
			return fmt.Sprintf(
				"%s %s --logs-widget --core-http %s",
				shellEnvPrefix(baseEnv),
				shellSingleQuote(ac.coreExecutablePath),
				shellSingleQuote(base.CoreHTTP),
			)
		}
		if spec.Name == "context" {
			return fmt.Sprintf(
				"%s %s --context-widget --core-http %s",
				shellEnvPrefix(baseEnv),
				shellSingleQuote(ac.coreExecutablePath),
				shellSingleQuote(base.CoreHTTP),
			)
		}
		return fmt.Sprintf(
			"%s %s --input-widget --core-http %s",
			shellEnvPrefix(baseEnv),
			shellSingleQuote(ac.coreExecutablePath),
			shellSingleQuote(base.CoreHTTP),
		)
	}

	return fmt.Sprintf(
		"%s %s %s",
		shellEnvPrefix(baseEnv),
		shellSingleQuote(ac.pythonExecutable),
		shellSingleQuote(ac.chatAppletScript),
	)
}

func (ac *AgentXCore) PrepareHealthEndpoint() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.healthListener != nil {
		return nil
	}

	listener, err := net.Listen("tcp", ac.healthAddr)
	if err != nil {
		return err
	}
	ac.healthListener = listener
	ac.healthAddr = listener.Addr().String()
	return nil
}

func defaultPromptHandler(appletName string) func(context.Context, string) (string, error) {
	return func(_ context.Context, prompt string) (string, error) {
		trimmedPrompt := strings.TrimSpace(prompt)
		if trimmedPrompt == "" {
			return "", fmt.Errorf("empty prompt for applet %s", appletName)
		}
		return fmt.Sprintf("Echo: %s", trimmedPrompt), nil
	}
}

func (ac *AgentXCore) pythonChatPromptHandler() func(context.Context, string) (string, error) {
	return func(ctx context.Context, prompt string) (string, error) {
		response, err := ac.routePromptViaPythonChatApplet(ctx, prompt)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				ac.emitBridgeLog(context.Background(), "bridge_fallback_skipped", err.Error())
				return "", err
			}
			ac.emitBridgeLog(ctx, "bridge_fallback", err.Error())
			return defaultPromptHandler("chat")(ctx, prompt)
		}
		return response, nil
	}
}

func (ac *AgentXCore) goChatPromptHandler() func(context.Context, string) (string, error) {
	return func(ctx context.Context, prompt string) (string, error) {
		response, err := ac.routePromptViaGoChatBackend(ctx, prompt)
		if err == nil {
			return response, nil
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			ac.emitBridgeLog(context.Background(), "go_chat_fallback_skipped", err.Error())
			return "", err
		}

		ac.emitBridgeLog(ctx, "go_chat_fallback", err.Error())
		return defaultPromptHandler("chat")(ctx, prompt)
	}
}

func normalizeOllamaBaseURL(hostOrURL string) string {
	value := strings.TrimSpace(hostOrURL)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return strings.TrimRight(value, "/")
	}
	return "http://" + strings.TrimRight(value, "/")
}

func (ac *AgentXCore) routePromptViaGoChatBackend(ctx context.Context, prompt string) (string, error) {
	trimmedPrompt := strings.TrimSpace(prompt)
	if trimmedPrompt == "" {
		return "", fmt.Errorf("empty prompt")
	}
	ac.emitBridgeLog(ctx, "go_chat_route_start", fmt.Sprintf("prompt_chars=%d", len(trimmedPrompt)))

	backend := strings.ToLower(strings.TrimSpace(ac.runtimeConfig.ChatBackend))
	if backend == "" || backend == "echo" || backend == "mock" {
		response := fmt.Sprintf("Echo: %s", trimmedPrompt)
		ac.emitBridgeLog(ctx, "go_chat_response_ok", fmt.Sprintf("response_chars=%d", len(response)))
		return response, nil
	}
	if backend != "ollama" {
		response := fmt.Sprintf("Echo: %s", trimmedPrompt)
		ac.emitBridgeLog(ctx, "go_chat_response_ok", fmt.Sprintf("response_chars=%d", len(response)))
		return response, nil
	}

	payload := map[string]interface{}{
		"model": ac.runtimeConfig.OllamaModel,
		"messages": []map[string]string{
			{"role": "user", "content": trimmedPrompt},
		},
		"stream": false,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	baseURL := normalizeOllamaBaseURL(ac.runtimeConfig.OllamaHost)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/chat", bytes.NewReader(payloadJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: ac.runtimeConfig.SubmitExecutionTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama chat request failed: status=%d", resp.StatusCode)
	}

	var decoded map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", err
	}

	if message, ok := decoded["message"].(map[string]interface{}); ok {
		if content, ok := message["content"].(string); ok {
			trimmed := strings.TrimSpace(content)
			if trimmed != "" {
				ac.emitBridgeLog(ctx, "go_chat_response_ok", fmt.Sprintf("response_chars=%d", len(trimmed)))
				return trimmed, nil
			}
		}
	}
	if response, ok := decoded["response"].(string); ok {
		trimmed := strings.TrimSpace(response)
		if trimmed != "" {
			ac.emitBridgeLog(ctx, "go_chat_response_ok", fmt.Sprintf("response_chars=%d", len(trimmed)))
			return trimmed, nil
		}
	}

	return "", fmt.Errorf("ollama response missing content")
}

func (ac *AgentXCore) routePromptViaPythonChatApplet(ctx context.Context, prompt string) (string, error) {
	ac.emitBridgeLog(ctx, "bridge_route_start", fmt.Sprintf("prompt_chars=%d", len(prompt)))

	ac.mu.RLock()
	chatApplet, exists := ac.applets["chat"]
	ac.mu.RUnlock()
	if !exists {
		ac.emitBridgeLog(ctx, "bridge_route_error", "chat applet not registered")
		return "", fmt.Errorf("chat applet is not registered")
	}

	chatApplet.BridgeMu.Lock()
	defer chatApplet.BridgeMu.Unlock()

	if err := ac.ensureChatBridgeProcessLocked(chatApplet); err != nil {
		ac.emitBridgeLog(ctx, "bridge_start_error", err.Error())
		return "", err
	}

	request := chatBridgeRequest{Type: "prompt", Prompt: prompt}
	if err := json.NewEncoder(chatApplet.BridgeStdin).Encode(request); err != nil {
		ac.emitBridgeLog(ctx, "bridge_request_error", err.Error())
		ac.teardownChatBridgeProcessLocked(chatApplet)
		return "", err
	}

	readResultChan := make(chan chatBridgeReadResult, 1)
	go func() {
		readResultChan <- readChatBridgeResponseFromScanner(chatApplet.BridgeStdout, func(delta string) error {
			return ac.renderChatStreamChunk(ctx, delta)
		})
	}()

	select {
	case readResult := <-readResultChan:
		if readResult.err != nil {
			ac.emitBridgeLog(ctx, "bridge_response_error", readResult.err.Error())
			ac.teardownChatBridgeProcessLocked(chatApplet)
			return "", readResult.err
		}
		ac.emitBridgeLog(ctx, "bridge_response_ok", fmt.Sprintf("response_chars=%d", len(readResult.response)))
		return readResult.response, nil
	case <-ctx.Done():
		ac.emitBridgeLog(context.Background(), "bridge_canceled", ctx.Err().Error())
		ac.teardownChatBridgeProcessLocked(chatApplet)
		return "", ctx.Err()
	case <-time.After(ac.chatBridgeResponseTimeout):
		ac.emitBridgeLog(ctx, "bridge_timeout", fmt.Sprintf("timeout=%s", ac.chatBridgeResponseTimeout))
		ac.teardownChatBridgeProcessLocked(chatApplet)
		return "", fmt.Errorf("chat bridge response timeout after %s", ac.chatBridgeResponseTimeout)
	}
}

func shouldRenderInteractivePanesViaCore() bool {
	mode := strings.TrimSpace(strings.ToLower(os.Getenv("AGENTX_PANE_RENDER_MODE")))
	if mode == "" {
		return false
	}
	return mode == "core" || mode == "1" || mode == "true"
}

func (ac *AgentXCore) renderChatStreamChunk(ctx context.Context, delta string) error {
	if !shouldRenderInteractivePanesViaCore() {
		return nil
	}

	trimmed := strings.TrimSpace(delta)
	if trimmed == "" {
		return nil
	}
	ac.emitBridgeLog(ctx, "bridge_chunk", fmt.Sprintf("chunk_chars=%d", len(trimmed)))
	renderCmd := fmt.Sprintf("echo %s", shellSingleQuote("[assistant-stream] "+trimmed))
	if err := ac.runTmux(ctx, "send-keys", "-t", ac.paneTargetForName(PaneTitleOutput), renderCmd, "Enter"); err != nil {
		return fmt.Errorf("failed rendering chat stream chunk: %w", err)
	}
	return nil
}

func (ac *AgentXCore) ensureChatBridgeProcessLocked(chatApplet *AppletProcess) error {
	if _, err := os.Stat(ac.chatAppletScript); err != nil {
		return fmt.Errorf("chat applet script unavailable: %w", err)
	}

	if chatApplet.Cmd != nil && chatApplet.BridgeStdin != nil && chatApplet.BridgeStdout != nil {
		if chatApplet.DoneChan != nil {
			select {
			case <-chatApplet.DoneChan:
				ac.teardownChatBridgeProcessLocked(chatApplet)
			default:
				return nil
			}
		} else {
			return nil
		}
	}

	processCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(processCtx, ac.pythonExecutable, ac.chatAppletScript, "--bridge-chat-server")
	chatBackend := ac.runtimeConfig.ChatBackend
	ollamaHost := ac.runtimeConfig.OllamaHost
	ollamaModel := ac.runtimeConfig.OllamaModel
	cmd.Env = append(os.Environ(),
		"AGENTX_APPLET_NAME=chat",
		"AGENTX_SESSION_ID="+ac.SessionID,
		"AGENTX_CHAT_BACKEND="+chatBackend,
		"AGENTX_OLLAMA_HOST="+ollamaHost,
		"AGENTX_OLLAMA_MODEL="+ollamaModel,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}

	chatApplet.Cmd = cmd
	chatApplet.Cancel = cancel
	chatApplet.BridgeStdin = stdin
	chatApplet.BridgeStdout = bufio.NewScanner(stdout)
	chatApplet.DoneChan = make(chan error, 1)
	ac.emitBridgeLog(context.Background(), "bridge_start", "backend="+chatBackend)
	doneChan := chatApplet.DoneChan

	go func(applet *AppletProcess, process *exec.Cmd, processStderr io.Reader) {
		_, _ = io.Copy(io.Discard, processStderr)
		err := process.Wait()
		doneChan <- err
	}(chatApplet, cmd, stderr)

	ac.markAppletStatus(chatApplet.Name, AppletStatusRunning, nil)
	return nil
}

func (ac *AgentXCore) emitBridgeLog(ctx context.Context, event string, details string) {
	trimmedEvent := strings.TrimSpace(event)
	if trimmedEvent == "" {
		trimmedEvent = "bridge_event"
	}
	trimmedDetails := strings.TrimSpace(details)
	if len(trimmedDetails) > 180 {
		trimmedDetails = trimmedDetails[:180]
	}

	message := "[bridge] event=" + trimmedEvent
	if trimmedDetails != "" {
		message += " details=" + trimmedDetails
	}

	renderCmd := fmt.Sprintf("echo %s", shellSingleQuote(message))
	if err := ac.runTmux(ctx, "send-keys", "-t", ac.paneTargetForName(PaneTitleLogs), renderCmd, "Enter"); err != nil {
		log.Printf("[AgentX Core] Bridge log render failed: %v", err)
	}
}

func (ac *AgentXCore) teardownChatBridgeProcessLocked(chatApplet *AppletProcess) {
	if chatApplet.BridgeStdin != nil {
		_ = chatApplet.BridgeStdin.Close()
	}
	if chatApplet.Cancel != nil {
		chatApplet.Cancel()
	}
	if chatApplet.Cmd != nil && chatApplet.Cmd.Process != nil {
		_ = chatApplet.Cmd.Process.Kill()
	}

	chatApplet.BridgeStdin = nil
	chatApplet.BridgeStdout = nil
	chatApplet.Cmd = nil
	chatApplet.Cancel = nil
	chatApplet.DoneChan = nil
}

type chatBridgeReadResult struct {
	response string
	streamed bool
	err      error
}

func readChatBridgeResponseFromScanner(scanner *bufio.Scanner, onChunk func(string) error) chatBridgeReadResult {
	var responseBuilder strings.Builder
	streamed := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "READY ") {
			continue
		}

		var response chatBridgeResponse
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			continue
		}

		switch response.Type {
		case "chunk":
			trimmedDelta := strings.TrimSpace(response.Delta)
			if trimmedDelta == "" {
				continue
			}
			if onChunk != nil {
				if err := onChunk(trimmedDelta); err != nil {
					return chatBridgeReadResult{err: err}
				}
			}
			if responseBuilder.Len() > 0 {
				responseBuilder.WriteString(" ")
			}
			responseBuilder.WriteString(trimmedDelta)
			streamed = true
			continue
		case "response":
			finalResponse := strings.TrimSpace(response.Response)
			if finalResponse == "" {
				finalResponse = strings.TrimSpace(responseBuilder.String())
			}
			if finalResponse == "" {
				return chatBridgeReadResult{err: fmt.Errorf("chat bridge returned empty response")}
			}
			return chatBridgeReadResult{response: finalResponse, streamed: streamed}
		case "error":
			if response.Error == "" {
				return chatBridgeReadResult{err: fmt.Errorf("chat bridge returned empty error")}
			}
			return chatBridgeReadResult{err: fmt.Errorf("chat bridge error: %s", response.Error)}
		}
	}

	if err := scanner.Err(); err != nil {
		return chatBridgeReadResult{err: err}
	}

	return chatBridgeReadResult{err: fmt.Errorf("chat bridge produced no response")}
}

func shellSingleQuote(input string) string {
	return "'" + strings.ReplaceAll(input, "'", "'\"'\"'") + "'"
}

func (ac *AgentXCore) paneTargetForName(paneName string) string {
	if target, ok := ac.paneTargetByName[paneName]; ok && strings.TrimSpace(target) != "" {
		return target
	}

	if paneName == PaneTitleLogs {
		return ac.tmuxSessionName + ":1.0"
	}

	return ac.tmuxSessionName + ":0." + map[string]string{
		PaneTitleOutput: "0",
		"chat":          "0",
		PaneTitleSystem: "1",
		"context":       "1",
		PaneTitleInput:  "2",
	}[paneName]
}

func (ac *AgentXCore) setPaneTitle(ctx context.Context, target string, title string) error {
	if err := validatePaneTitle(title); err != nil {
		return err
	}
	return ac.runTmux(ctx, "select-pane", "-t", target, "-T", title)
}

func (ac *AgentXCore) renderChatResponse(ctx context.Context, response string) error {
	if !shouldRenderInteractivePanesViaCore() {
		return nil
	}

	renderCmd := fmt.Sprintf("echo %s", shellSingleQuote("[assistant] "+response))
	if err := ac.runTmux(ctx, "send-keys", "-t", ac.paneTargetForName(PaneTitleOutput), renderCmd, "Enter"); err != nil {
		return fmt.Errorf("failed rendering chat response: %w", err)
	}
	return nil
}

func trimForPaneSummary(value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if maxLen <= 3 || len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen-3] + "..."
}

func (ac *AgentXCore) renderContextTurnSummary(ctx context.Context, turnIndex int, prompt string, response string) error {
	if !shouldRenderInteractivePanesViaCore() {
		return nil
	}

	summary := fmt.Sprintf(
		"[context] turn=%d prompt=%q response=%q",
		turnIndex,
		trimForPaneSummary(prompt, 48),
		trimForPaneSummary(response, 72),
	)
	renderCmd := fmt.Sprintf("echo %s", shellSingleQuote(summary))
	if err := ac.runTmux(ctx, "send-keys", "-t", ac.paneTargetForName(PaneTitleSystem), renderCmd, "Enter"); err != nil {
		return fmt.Errorf("failed rendering context turn summary: %w", err)
	}
	return nil
}

// RouteInputPrompt routes an input prompt through the tracked chat applet and renders the response.
func (ac *AgentXCore) RouteInputPrompt(ctx context.Context, prompt string) (string, error) {
	trimmedPrompt := strings.TrimSpace(prompt)
	if trimmedPrompt == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}

	ac.mu.RLock()
	chatApplet, exists := ac.applets["chat"]
	ac.mu.RUnlock()
	if !exists {
		err := fmt.Errorf("chat applet is not registered")
		log.Printf("[AgentX Core] Prompt routing failed: %v", err)
		return "", err
	}

	handler := chatApplet.HandlePrompt
	if handler == nil {
		handler = defaultPromptHandler(chatApplet.Name)
	}

	ac.startPromptCycle()
	ac.emitLifecycleEvent(ctx, lifecycleStageSubmitted, lifecycleEventDetails(lifecycleStageSubmitted, trimmedPrompt, ""))
	classResult := classifyPrompt(trimmedPrompt)

	// Safety escalation: abort before thinking begins.
	if classResult.NextStep == ClassifyNextStepEscalate {
		ac.failClassifyPhase()
		log.Printf("[AgentX Core] Prompt escalated (safety): %q", trimmedPrompt)
		return "", fmt.Errorf("prompt blocked: safety escalation")
	}

	ac.finishClassifyPhase(classResult)
	ac.emitLifecycleEvent(ctx, lifecycleStageClassified, lifecycleEventDetails(lifecycleStageClassified, trimmedPrompt, ""))
	ac.startThinkingPhase()
	ac.emitLifecycleEvent(ctx, lifecycleStageThinking, lifecycleEventDetails(lifecycleStageThinking, trimmedPrompt, ""))

	ac.markAppletStatus(chatApplet.Name, AppletStatusRunning, nil)
	response, err := handler(ctx, trimmedPrompt)
	if err != nil {
		ac.failThinkingPhase()
		ac.markAppletStatus(chatApplet.Name, AppletStatusCrashed, err)
		log.Printf("[AgentX Core] Prompt routing failed in chat applet: %v", err)
		return "", err
	}
	ac.finishThinkingPhase()

	// Route tool phase based on classify result:
	// respond_directly → skip (no tool call needed for conversational responses).
	// single_tool | invoke_planner → run tool phase (current behavior).
	if classResult.NextStep == ClassifyNextStepRespondDirectly {
		ac.skipToolPhase()
	} else {
		ac.startToolPhase()
		ac.emitLifecycleEvent(ctx, lifecycleStageTool, lifecycleEventDetails(lifecycleStageTool, trimmedPrompt, response))
		ac.finishToolPhase()
	}
	ac.startRespondPhase()

	if err := ac.renderChatResponse(ctx, response); err != nil {
		ac.failRespondPhase()
		ac.markAppletStatus(chatApplet.Name, AppletStatusCrashed, err)
		log.Printf("[AgentX Core] Prompt rendering failed: %v", err)
		return "", err
	}

	if err := ac.contextManager.RecordTurn(trimmedPrompt, response); err != nil {
		ac.failRespondPhase()
		ac.markAppletStatus(chatApplet.Name, AppletStatusCrashed, err)
		log.Printf("[AgentX Core] Prompt persistence failed: %v", err)
		return "", err
	}

	ac.emitLifecycleEvent(ctx, lifecycleStageFinalResponse, lifecycleEventDetails(lifecycleStageFinalResponse, trimmedPrompt, response))

	turnCount := len(ac.ContextTurnsSnapshot())
	if err := ac.renderContextTurnSummary(ctx, turnCount, trimmedPrompt, response); err != nil {
		log.Printf("[AgentX Core] Context pane update failed: %v", err)
	}
	ac.finishRespondPhase()

	ac.markAppletStatus(chatApplet.Name, AppletStatusReady, nil)
	return response, nil
}

// ContextTurnsSnapshot returns persisted chat turns for the current session.
func (ac *AgentXCore) ContextTurnsSnapshot() []ChatTurn {
	return ac.contextManager.TurnsSnapshot()
}

func (ac *AgentXCore) promptCycleSnapshot() PromptCycleStatus {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.currentPromptCycleLocked(time.Now())
}

func (ac *AgentXCore) currentPromptCycleLocked(now time.Time) PromptCycleStatus {
	cycle := ac.lastPromptCycle
	if cycle.Classify.State == "running" && !ac.classifyPhaseStartedAt.IsZero() {
		cycle.Classify.ElapsedMs = now.Sub(ac.classifyPhaseStartedAt).Milliseconds()
	}
	if cycle.Thinking.State == "running" && !ac.thinkingPhaseStartedAt.IsZero() {
		cycle.Thinking.ElapsedMs = now.Sub(ac.thinkingPhaseStartedAt).Milliseconds()
	}
	if cycle.Tool.State == "running" && !ac.toolPhaseStartedAt.IsZero() {
		cycle.Tool.ElapsedMs = now.Sub(ac.toolPhaseStartedAt).Milliseconds()
	}
	if cycle.Respond.State == "running" && !ac.respondPhaseStartedAt.IsZero() {
		cycle.Respond.ElapsedMs = now.Sub(ac.respondPhaseStartedAt).Milliseconds()
	}
	return cycle
}

func (ac *AgentXCore) startPromptCycle() {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.lastPromptCycle = PromptCycleStatus{
		Classify: PromptCyclePhase{State: "running", ElapsedMs: 0},
		Thinking: PromptCyclePhase{State: "pending", ElapsedMs: 0},
		Tool:     PromptCyclePhase{State: "pending", ElapsedMs: 0},
		Respond:  PromptCyclePhase{State: "pending", ElapsedMs: 0},
	}
	ac.classifyPhaseStartedAt = now
	ac.thinkingPhaseStartedAt = time.Time{}
	ac.toolPhaseStartedAt = time.Time{}
	ac.respondPhaseStartedAt = time.Time{}
}

func (ac *AgentXCore) finishClassifyPhase(result ClassifyResult) {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.lastPromptCycle.Classify.State != "running" {
		log.Printf("[AgentX Core] Ignoring invalid transition: finishClassifyPhase from %q", ac.lastPromptCycle.Classify.State)
		return
	}
	elapsed := int64(0)
	if !ac.classifyPhaseStartedAt.IsZero() {
		elapsed = now.Sub(ac.classifyPhaseStartedAt).Milliseconds()
	}
	ac.lastPromptCycle.Classify = PromptCyclePhase{State: "done", ElapsedMs: elapsed}
	ac.lastPromptCycle.ClassifyResult = &result
}

func (ac *AgentXCore) failClassifyPhase() {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.lastPromptCycle.Classify.State != "running" {
		log.Printf("[AgentX Core] Ignoring invalid transition: failClassifyPhase from %q", ac.lastPromptCycle.Classify.State)
		return
	}
	elapsed := int64(0)
	if !ac.classifyPhaseStartedAt.IsZero() {
		elapsed = now.Sub(ac.classifyPhaseStartedAt).Milliseconds()
	}
	ac.lastPromptCycle.Classify = PromptCyclePhase{State: "failed", ElapsedMs: elapsed}
}

func (ac *AgentXCore) startThinkingPhase() {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.lastPromptCycle.Classify.State != "done" || ac.lastPromptCycle.Thinking.State != "pending" {
		log.Printf("[AgentX Core] Ignoring invalid transition: startThinkingPhase classify=%q thinking=%q", ac.lastPromptCycle.Classify.State, ac.lastPromptCycle.Thinking.State)
		return
	}
	ac.lastPromptCycle.Thinking = PromptCyclePhase{State: "running", ElapsedMs: 0}
	ac.thinkingPhaseStartedAt = now
}

func (ac *AgentXCore) finishThinkingPhase() {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.lastPromptCycle.Thinking.State != "running" {
		log.Printf("[AgentX Core] Ignoring invalid transition: finishThinkingPhase from %q", ac.lastPromptCycle.Thinking.State)
		return
	}
	elapsed := int64(0)
	if !ac.thinkingPhaseStartedAt.IsZero() {
		elapsed = now.Sub(ac.thinkingPhaseStartedAt).Milliseconds()
	}
	ac.lastPromptCycle.Thinking = PromptCyclePhase{State: "done", ElapsedMs: elapsed}
}

func (ac *AgentXCore) failThinkingPhase() {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.lastPromptCycle.Thinking.State != "running" {
		log.Printf("[AgentX Core] Ignoring invalid transition: failThinkingPhase from %q", ac.lastPromptCycle.Thinking.State)
		return
	}
	elapsed := int64(0)
	if !ac.thinkingPhaseStartedAt.IsZero() {
		elapsed = now.Sub(ac.thinkingPhaseStartedAt).Milliseconds()
	}
	ac.lastPromptCycle.Thinking = PromptCyclePhase{State: "failed", ElapsedMs: elapsed}
}

func (ac *AgentXCore) startToolPhase() {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.lastPromptCycle.Thinking.State != "done" || ac.lastPromptCycle.Tool.State != "pending" {
		log.Printf("[AgentX Core] Ignoring invalid transition: startToolPhase thinking=%q tool=%q", ac.lastPromptCycle.Thinking.State, ac.lastPromptCycle.Tool.State)
		return
	}
	ac.lastPromptCycle.Tool = PromptCyclePhase{State: "running", ElapsedMs: 0}
	ac.toolPhaseStartedAt = now
}

func (ac *AgentXCore) finishToolPhase() {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.lastPromptCycle.Tool.State != "running" {
		log.Printf("[AgentX Core] Ignoring invalid transition: finishToolPhase from %q", ac.lastPromptCycle.Tool.State)
		return
	}
	elapsed := int64(0)
	if !ac.toolPhaseStartedAt.IsZero() {
		elapsed = now.Sub(ac.toolPhaseStartedAt).Milliseconds()
	}
	ac.lastPromptCycle.Tool = PromptCyclePhase{State: "done", ElapsedMs: elapsed}
}

// skipToolPhase marks the tool phase as skipped, allowing startRespondPhase to proceed
// when the classify result routes directly to respond (e.g. respond_directly).
func (ac *AgentXCore) skipToolPhase() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.lastPromptCycle.Tool.State != "pending" {
		log.Printf("[AgentX Core] Ignoring invalid transition: skipToolPhase from %q", ac.lastPromptCycle.Tool.State)
		return
	}
	ac.lastPromptCycle.Tool = PromptCyclePhase{State: "skipped", ElapsedMs: 0}
}

func (ac *AgentXCore) startRespondPhase() {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	toolDone := ac.lastPromptCycle.Tool.State == "done" || ac.lastPromptCycle.Tool.State == "skipped"
	if ac.lastPromptCycle.Thinking.State != "done" || !toolDone || ac.lastPromptCycle.Respond.State != "pending" {
		log.Printf("[AgentX Core] Ignoring invalid transition: startRespondPhase thinking=%q tool=%q respond=%q", ac.lastPromptCycle.Thinking.State, ac.lastPromptCycle.Tool.State, ac.lastPromptCycle.Respond.State)
		return
	}
	ac.lastPromptCycle.Respond = PromptCyclePhase{State: "running", ElapsedMs: 0}
	ac.respondPhaseStartedAt = now
}

func (ac *AgentXCore) finishRespondPhase() {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.lastPromptCycle.Respond.State != "running" {
		log.Printf("[AgentX Core] Ignoring invalid transition: finishRespondPhase from %q", ac.lastPromptCycle.Respond.State)
		return
	}
	elapsed := int64(0)
	if !ac.respondPhaseStartedAt.IsZero() {
		elapsed = now.Sub(ac.respondPhaseStartedAt).Milliseconds()
	}
	ac.lastPromptCycle.Respond = PromptCyclePhase{State: "done", ElapsedMs: elapsed}
}

func (ac *AgentXCore) failRespondPhase() {
	now := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if ac.lastPromptCycle.Respond.State != "running" {
		log.Printf("[AgentX Core] Ignoring invalid transition: failRespondPhase from %q", ac.lastPromptCycle.Respond.State)
		return
	}
	elapsed := int64(0)
	if !ac.respondPhaseStartedAt.IsZero() {
		elapsed = now.Sub(ac.respondPhaseStartedAt).Milliseconds()
	}
	ac.lastPromptCycle.Respond = PromptCyclePhase{State: "failed", ElapsedMs: elapsed}
}

func (ac *AgentXCore) appendInputHistory(input string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.inputHistory = append(ac.inputHistory, input)
}

// InputHistorySnapshot returns a deterministic copy of recorded input lines.
func (ac *AgentXCore) InputHistorySnapshot() []string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	history := make([]string, len(ac.inputHistory))
	copy(history, ac.inputHistory)
	return history
}

// HandleInputLine handles input command contract for the input applet.
// Supported commands:
//   - :clear clears input pane state only
//   - :q requests session exit
//
// Any non-command input is routed as a chat prompt.
func (ac *AgentXCore) HandleInputLine(ctx context.Context, line string) (response string, shouldExit bool, err error) {
	trimmedLine := strings.TrimSpace(line)
	if trimmedLine == "" {
		return "", false, nil
	}

	ac.appendInputHistory(trimmedLine)

	if strings.HasPrefix(trimmedLine, ":") {
		switch trimmedLine {
		case ":clear":
			inputPaneTarget := ac.paneTargetForName(PaneTitleInput)

			// Clear visible terminal state and scrollback for the live-core input pane only.
			if err := ac.runTmux(ctx, "clear-history", "-t", inputPaneTarget); err != nil {
				return "", false, fmt.Errorf("failed to clear live-core input pane history: %w", err)
			}
			if err := ac.runTmux(ctx, "send-keys", "-t", inputPaneTarget, "C-u"); err != nil {
				return "", false, fmt.Errorf("failed to reset live-core input line: %w", err)
			}
			if err := ac.runTmux(ctx, "send-keys", "-R", "-t", inputPaneTarget); err != nil {
				return "", false, fmt.Errorf("failed to reset live-core input pane display: %w", err)
			}
			return "cleared", false, nil
		case ":q":
			ac.mu.Lock()
			ac.exitRequested = true
			ac.mu.Unlock()
			log.Printf("[AgentX Core] Input command handled: :q")
			return "quit", true, nil
		default:
			return "", false, fmt.Errorf("unsupported command: %s", trimmedLine)
		}
	}

	routedResponse, routeErr := ac.RouteInputPrompt(ctx, trimmedLine)
	if routeErr != nil {
		return "", false, routeErr
	}

	return routedResponse, false, nil
}

func (ac *AgentXCore) ensureTrackedAppletLocked(appletName, paneName string) *AppletProcess {
	if applet, exists := ac.applets[appletName]; exists {
		if paneName != "" {
			applet.PaneName = paneName
		}
		if applet.StartedAt.IsZero() {
			applet.StartedAt = time.Now()
		}
		return applet
	}

	applet := &AppletProcess{
		Name:      appletName,
		PaneName:  paneName,
		Runtime:   appletRuntimePython,
		Status:    AppletStatusStarting,
		StartedAt: time.Now(),
	}
	ac.applets[appletName] = applet
	return applet
}

func (ac *AgentXCore) markAppletStatus(appletName string, status AppletLifecycleStatus, statusErr error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	applet := ac.ensureTrackedAppletLocked(appletName, appletName)
	applet.Status = status
	if statusErr != nil {
		applet.LastError = statusErr.Error()
	}
	if status == AppletStatusCrashed {
		applet.CrashCount++
	}
}

func resolveAppletStatus(applet *AppletProcess) AppletLifecycleStatus {
	if applet.Status != "" {
		return applet.Status
	}
	if applet.Cmd != nil && applet.Cmd.Process != nil {
		return AppletStatusRunning
	}
	return AppletStatusStopped
}

func (ac *AgentXCore) healthSnapshot() HealthSnapshot {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	panes := make([]PaneStatus, 0, len(DefaultPaneLayout()))
	for _, pane := range DefaultPaneLayout() {
		paneApplet := pane.Name
		paneStatus := AppletStatusReady
		for _, applet := range ac.applets {
			if applet.PaneName == pane.Name {
				paneApplet = applet.Name
				paneStatus = resolveAppletStatus(applet)
				break
			}
		}

		panes = append(panes, PaneStatus{
			Name:   pane.Name,
			Applet: paneApplet,
			Status: string(paneStatus),
		})
	}
	applets := make([]AppletStatus, 0, len(ac.applets))
	for _, applet := range ac.applets {
		status := resolveAppletStatus(applet)
		applets = append(applets, AppletStatus{
			Name:       applet.Name,
			Pane:       applet.PaneName,
			Runtime:    string(applet.Runtime),
			Status:     string(status),
			CrashCount: applet.CrashCount,
		})
	}

	sort.Slice(applets, func(i, j int) bool {
		return applets[i].Name < applets[j].Name
	})

	return HealthSnapshot{
		Status:        "ok",
		SessionID:     ac.SessionID,
		UptimeSeconds: int64(time.Since(ac.startedAt).Seconds()),
		SubmitRetries: loadDemoSubmitRetryCounter(),
		Panes:         panes,
		Applets:       applets,
		PromptCycle:   ac.currentPromptCycleLocked(time.Now()),
	}
}

// StartHealthEndpoint starts the health/status HTTP endpoint.
func (ac *AgentXCore) StartHealthEndpoint(ctx context.Context) error {
	if err := ac.PrepareHealthEndpoint(); err != nil {
		return err
	}

	ac.mu.RLock()
	listener := ac.healthListener
	addr := ac.healthAddr
	ac.mu.RUnlock()

	go func() {
		if err := ac.contextManager.ServeHealthListener(ctx, listener); err != nil {
			log.Printf("[AgentX Core] Health endpoint error: %v", err)
		}
	}()
	log.Printf("[AgentX Core] Health endpoint listening on %s", addr)
	return nil
}

// AttachTmuxSession attaches the current terminal to the managed tmux session.
func (ac *AgentXCore) AttachTmuxSession(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "tmux", "attach-session", "-t", ac.tmuxSessionName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to attach tmux session: %w", err)
	}
	return nil
}

// Shutdown gracefully stops all goroutines, applets, and tmux session.
func (ac *AgentXCore) Shutdown(ctx context.Context) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Stop all applets.
	for name, applet := range ac.applets {
		applet.Status = AppletStatusStopped
		if applet.Cancel != nil {
			applet.Cancel()
		}
		if applet.Cmd != nil && applet.Cmd.Process != nil {
			applet.Cmd.Process.Kill()
		}
		log.Printf("[AgentX Core] Stopped applet: %s", name)
	}

	// Kill tmux session.
	tmuxKillCtx, tmuxKillCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer tmuxKillCancel()
	cmd := exec.CommandContext(tmuxKillCtx, "tmux", "kill-session", "-t", ac.tmuxSessionName)
	if err := cmd.Run(); err != nil {
		log.Printf("[AgentX Core] Warning: failed to kill tmux session: %v", err)
	}
	if ac.healthListener != nil {
		_ = ac.healthListener.Close()
		ac.healthListener = nil
	}

	log.Printf("[AgentX Core] Shutdown complete")
	return nil
}

// ContextManager handles session context (state, messages, tools).
type ContextManager struct {
	contextDir       string
	sessionID        string
	startedAt        time.Time
	snapshotProvider func() HealthSnapshot
	submitProvider   func(context.Context, string) (string, error)
	submitTimeout    time.Duration
	shutdownProvider func(context.Context) error
	turns            []ChatTurn
	turnsLoaded      bool
	mu               sync.RWMutex
}

// NewContextManager creates a new context manager.
func NewContextManager(contextDir string) *ContextManager {
	return &ContextManager{
		contextDir: contextDir,
		startedAt:  time.Now(),
		submitTimeout: 120 * time.Second,
		turns:      make([]ChatTurn, 0),
	}
}

func (cm *ContextManager) turnsFilePath() string {
	return filepath.Join(cm.contextDir, "turns.jsonl")
}

func (cm *ContextManager) loadTurnsLocked() error {
	if cm.turnsLoaded {
		return nil
	}

	cm.turns = make([]ChatTurn, 0)
	data, err := os.ReadFile(cm.turnsFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			cm.turnsLoaded = true
			return nil
		}
		return err
	}

	trimmedData := strings.TrimSpace(string(data))
	if trimmedData == "" {
		cm.turnsLoaded = true
		return nil
	}

	for _, line := range strings.Split(trimmedData, "\n") {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		var turn ChatTurn
		if err := json.Unmarshal([]byte(trimmedLine), &turn); err != nil {
			return fmt.Errorf("invalid turn record: %w", err)
		}
		cm.turns = append(cm.turns, turn)
	}

	cm.turnsLoaded = true
	return nil
}

// RecordTurn appends a completed chat turn to in-memory and persisted context.
func (cm *ContextManager) RecordTurn(prompt, response string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if err := cm.loadTurnsLocked(); err != nil {
		return err
	}

	if err := os.MkdirAll(cm.contextDir, 0o755); err != nil {
		return err
	}

	turn := ChatTurn{
		Prompt:    prompt,
		Response:  response,
		CreatedAt: time.Now().UnixMilli(),
	}

	encoded, err := json.Marshal(turn)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(cm.turnsFilePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}

	cm.turns = append(cm.turns, turn)
	return nil
}

// TurnsSnapshot returns a deterministic copy of persisted turns.
func (cm *ContextManager) TurnsSnapshot() []ChatTurn {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if err := cm.loadTurnsLocked(); err != nil {
		log.Printf("[AgentX Core] Failed to load turns snapshot: %v", err)
		return []ChatTurn{}
	}

	turns := make([]ChatTurn, len(cm.turns))
	copy(turns, cm.turns)
	return turns
}

// SetSessionMetadata updates session metadata used by health responses.
func (cm *ContextManager) SetSessionMetadata(sessionID string, startedAt time.Time) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.sessionID = sessionID
	cm.startedAt = startedAt
}

// SetSnapshotProvider configures a runtime snapshot function for endpoint payloads.
func (cm *ContextManager) SetSnapshotProvider(provider func() HealthSnapshot) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.snapshotProvider = provider
}

// SetSubmitProvider configures interactive prompt submission for /submit endpoint.
func (cm *ContextManager) SetSubmitProvider(provider func(context.Context, string) (string, error)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.submitProvider = provider
}

// SetSubmitExecutionTimeout configures /submit server-side execution timeout.
func (cm *ContextManager) SetSubmitExecutionTimeout(timeout time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if timeout > 0 {
		cm.submitTimeout = timeout
	}
}

// SetShutdownProvider configures graceful shutdown for /shutdown endpoint.
func (cm *ContextManager) SetShutdownProvider(provider func(context.Context) error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.shutdownProvider = provider
}

func (cm *ContextManager) snapshot() HealthSnapshot {
	cm.mu.RLock()
	provider := cm.snapshotProvider
	sessionID := cm.sessionID
	startedAt := cm.startedAt
	cm.mu.RUnlock()

	if provider != nil {
		return provider()
	}

	return HealthSnapshot{
		Status:        "ok",
		SessionID:     sessionID,
		UptimeSeconds: int64(time.Since(startedAt).Seconds()),
		Panes:         []PaneStatus{},
		Applets:       []AppletStatus{},
		PromptCycle: PromptCycleStatus{
			Classify: PromptCyclePhase{State: "pending", ElapsedMs: 0},
			Thinking: PromptCyclePhase{State: "pending", ElapsedMs: 0},
			Tool:     PromptCyclePhase{State: "idle", ElapsedMs: 0},
			Respond:  PromptCyclePhase{State: "pending", ElapsedMs: 0},
		},
	}
}

// HealthHandler returns the HTTP handler for runtime health endpoints.
func (cm *ContextManager) HealthHandler() http.Handler {
	mux := http.NewServeMux()

	writeJSON := func(w http.ResponseWriter, statusCode int, payload any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			log.Printf("[AgentX Core] Failed to encode health payload: %v", err)
		}
	}

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		snapshot := cm.snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":         snapshot.Status,
			"session_id":     snapshot.SessionID,
			"uptime_seconds": snapshot.UptimeSeconds,
			"submit_retries": snapshot.SubmitRetries,
			"pane_count":     len(snapshot.Panes),
			"applet_count":   len(snapshot.Applets),
		})
	})

	mux.HandleFunc("/panes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		snapshot := cm.snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": snapshot.SessionID,
			"panes":      snapshot.Panes,
		})
	})

	mux.HandleFunc("/applets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		snapshot := cm.snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": snapshot.SessionID,
			"applets":    snapshot.Applets,
		})
	})

	mux.HandleFunc("/context", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		snapshot := cm.snapshot()
		turns := cm.TurnsSnapshot()
		promptCycle := snapshot.PromptCycle
		activityState, activityPhase := deriveActivityState(promptCycle)
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id":   snapshot.SessionID,
			"turn_count":   len(turns),
			"turns":        turns,
			"prompt_cycle": promptCycle,
			"activity": map[string]any{
				"state": activityState,
				"phase": activityPhase,
			},
		})
	})

	mux.HandleFunc("/activity", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		snapshot := cm.snapshot()
		promptCycle := snapshot.PromptCycle
		activityState, activityPhase := deriveActivityState(promptCycle)
		writeJSON(w, http.StatusOK, ActivitySnapshot{
			SessionID:   snapshot.SessionID,
			State:       activityState,
			Phase:       activityPhase,
			PromptCycle: promptCycle,
		})
	})

	mux.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		cm.mu.RLock()
		submitProvider := cm.submitProvider
		submitTimeout := cm.submitTimeout
		cm.mu.RUnlock()
		if submitProvider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "submit unavailable"})
			return
		}

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed reading request body"})
			return
		}

		var req submitRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request json"})
			return
		}

		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt cannot be empty"})
			return
		}

		submitCtx, cancel := context.WithTimeout(context.Background(), submitTimeout)
		defer cancel()

		response, err := submitProvider(submitCtx, prompt)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "submit timed out"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, submitResponse{Response: response})
	})

	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		cm.mu.RLock()
		shutdownProvider := cm.shutdownProvider
		cm.mu.RUnlock()
		if shutdownProvider == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "shutdown unavailable"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "shutting_down"})
		go func() {
			if err := shutdownProvider(context.Background()); err != nil {
				log.Printf("[AgentX Core] Shutdown request failed: %v", err)
			}
		}()
	})

	return mux
}

// ServeHealth starts an HTTP health endpoint.

func (cm *ContextManager) ServeHealth(ctx context.Context, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return cm.ServeHealthListener(ctx, listener)
}

func (cm *ContextManager) ServeHealthListener(ctx context.Context, listener net.Listener) error {
	server := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: cm.HealthHandler(),
	}

	errCh := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		if err := <-errCh; err != nil {
			return err
		}
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
