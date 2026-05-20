// Package main core orchestrator.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

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
)

// AgentXCore orchestrates the tmux session, applets, and IPC.
type AgentXCore struct {
	Config          *Config
	SessionID       string
	tmuxSessionName string
	applets         map[string]*AppletProcess
	inputHistory    []string
	exitRequested   bool
	mu              sync.RWMutex
	startedAt       time.Time
	healthAddr      string // Address for health endpoint
	contextManager  *ContextManager
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
	Status     string `json:"status"`
	CrashCount int    `json:"crash_count"`
}

// HealthSnapshot captures runtime status for health endpoints.
type HealthSnapshot struct {
	Status        string         `json:"status"`
	SessionID     string         `json:"session_id"`
	UptimeSeconds int64          `json:"uptime_seconds"`
	Panes         []PaneStatus   `json:"panes"`
	Applets       []AppletStatus `json:"applets"`
}

// ChatTurn captures one persisted user/assistant exchange.
type ChatTurn struct {
	Prompt    string `json:"prompt"`
	Response  string `json:"response"`
	CreatedAt int64  `json:"created_at"`
}

// AppletProcess tracks a running Python applet.
type AppletProcess struct {
	Name         string
	PaneName     string
	Status       AppletLifecycleStatus
	LastError    string
	HandlePrompt func(context.Context, string) (string, error)
	Cmd          *exec.Cmd
	Cancel       context.CancelFunc
	DoneChan     chan error
	StartedAt    time.Time
	CrashCount   int
}

// NewAgentXCore creates a new orchestrator instance.
func NewAgentXCore(cfg *Config) *AgentXCore {
	sessionID := cfg.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("agentx_%d", time.Now().Unix())
	}

	core := &AgentXCore{
		Config:          cfg,
		SessionID:       sessionID,
		tmuxSessionName: fmt.Sprintf("agentx_%s_%s", cfg.Username, sessionID),
		applets:         make(map[string]*AppletProcess),
		inputHistory:    make([]string, 0),
		startedAt:       time.Now(),
		healthAddr:      "127.0.0.1:9876", // Default health endpoint
	}

	core.contextManager = NewContextManager(cfg.SessionContextDir(sessionID))
	core.contextManager.SetSessionMetadata(core.SessionID, core.startedAt)
	core.contextManager.SetSnapshotProvider(core.healthSnapshot)

	return core
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

	chatPaneTarget := ac.tmuxSessionName + ":0.0"
	if err := ac.runTmux(ctx, "select-pane", "-t", chatPaneTarget, "-T", "chat"); err != nil {
		return fmt.Errorf("failed to set chat pane title: %w", err)
	}

	inputPaneTarget, err := ac.runTmuxCapture(ctx, buildInputSplitCommand(chatPaneTarget)...)
	if err != nil {
		return fmt.Errorf("failed to split input pane: %w", err)
	}
	if err := ac.runTmux(ctx, "select-pane", "-t", inputPaneTarget, "-T", "input"); err != nil {
		return fmt.Errorf("failed to set input pane title: %w", err)
	}

	contextPaneTarget, err := ac.runTmuxCapture(ctx, buildContextSplitCommand(chatPaneTarget)...)
	if err != nil {
		return fmt.Errorf("failed to split context pane: %w", err)
	}
	if err := ac.runTmux(ctx, "select-pane", "-t", contextPaneTarget, "-T", "context"); err != nil {
		return fmt.Errorf("failed to set context pane title: %w", err)
	}

	if err := ac.runTmux(ctx, "new-window", "-t", ac.tmuxSessionName+":1", "-n", "logs"); err != nil {
		return fmt.Errorf("failed to create logs window: %w", err)
	}

	if err := ac.runTmux(ctx, "select-window", "-t", ac.tmuxSessionName+":0"); err != nil {
		return fmt.Errorf("failed to re-select primary window: %w", err)
	}

	for _, pane := range paneTargets(ac.tmuxSessionName, chatPaneTarget, inputPaneTarget, contextPaneTarget) {
		placeholderCmd := fmt.Sprintf("echo '🔶 Pane: %s (AgentX Core)'", pane.name)
		if err := ac.runTmux(ctx, "send-keys", "-t", pane.target, placeholderCmd, "Enter"); err != nil {
			return fmt.Errorf("failed to set placeholder in pane %s: %w", pane.name, err)
		}
	}

	log.Printf("[AgentX Core] tmux session '%s' initialized with layout: chat(80x80)|context(20x80) top, input(100x20) bottom, logs hidden", ac.tmuxSessionName)
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
		{name: "chat", target: chatTarget},
		{name: "input", target: inputTarget},
		{name: "context", target: contextTarget},
		{name: "logs", target: sessionName + ":1.0"},
	}
}

func buildNewSessionCommand(sessionName string) []string {
	return []string{"new-session", "-d", "-s", sessionName, "-n", "tui-chat", "-x", "120", "-y", "40"}
}

func buildInputSplitCommand(chatPaneTarget string) []string {
	return []string{"split-window", "-P", "-F", "#{pane_id}", "-t", chatPaneTarget, "-v", "-p", "20"}
}

func buildContextSplitCommand(chatPaneTarget string) []string {
	return []string{"split-window", "-P", "-F", "#{pane_id}", "-t", chatPaneTarget, "-h", "-p", "20"}
}

// StartAppletSupervisor launches Python applets in goroutines.
func (ac *AgentXCore) StartAppletSupervisor(ctx context.Context) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	for _, pane := range DefaultPaneLayout() {
		if _, exists := ac.applets[pane.Name]; exists {
			continue
		}

		ac.applets[pane.Name] = &AppletProcess{
			Name:         pane.Name,
			PaneName:     pane.Name,
			Status:       AppletStatusReady,
			HandlePrompt: defaultPromptHandler(pane.Name),
			StartedAt:    time.Now(),
		}
	}

	log.Printf("[AgentX Core] Applet supervisor ready (%d tracked applets)", len(ac.applets))
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

func shellSingleQuote(input string) string {
	return "'" + strings.ReplaceAll(input, "'", "'\"'\"'") + "'"
}

func (ac *AgentXCore) paneTargetForName(paneName string) string {
	if paneName == "logs" {
		return ac.tmuxSessionName + ":1.0"
	}
	return ac.tmuxSessionName + ":0." + map[string]string{
		"chat":    "0",
		"context": "1",
		"input":   "2",
	}[paneName]
}

func (ac *AgentXCore) renderChatResponse(ctx context.Context, response string) error {
	renderCmd := fmt.Sprintf("echo %s", shellSingleQuote("[assistant] "+response))
	if err := ac.runTmux(ctx, "send-keys", "-t", ac.paneTargetForName("chat"), renderCmd, "Enter"); err != nil {
		return fmt.Errorf("failed rendering chat response: %w", err)
	}
	return nil
}

// RouteInputPrompt routes an input prompt through the tracked chat applet and renders the response.
func (ac *AgentXCore) RouteInputPrompt(ctx context.Context, prompt string) (string, error) {
	trimmedPrompt := strings.TrimSpace(prompt)
	if trimmedPrompt == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}

	log.Printf("[AgentX Core] Routing input prompt to chat applet")

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

	ac.markAppletStatus(chatApplet.Name, AppletStatusRunning, nil)
	response, err := handler(ctx, trimmedPrompt)
	if err != nil {
		ac.markAppletStatus(chatApplet.Name, AppletStatusCrashed, err)
		log.Printf("[AgentX Core] Prompt routing failed in chat applet: %v", err)
		return "", err
	}

	if err := ac.renderChatResponse(ctx, response); err != nil {
		ac.markAppletStatus(chatApplet.Name, AppletStatusCrashed, err)
		log.Printf("[AgentX Core] Prompt rendering failed: %v", err)
		return "", err
	}

	if err := ac.contextManager.RecordTurn(trimmedPrompt, response); err != nil {
		ac.markAppletStatus(chatApplet.Name, AppletStatusCrashed, err)
		log.Printf("[AgentX Core] Prompt persistence failed: %v", err)
		return "", err
	}

	ac.markAppletStatus(chatApplet.Name, AppletStatusReady, nil)
	log.Printf("[AgentX Core] Prompt routed and response rendered")
	return response, nil
}

// ContextTurnsSnapshot returns persisted chat turns for the current session.
func (ac *AgentXCore) ContextTurnsSnapshot() []ChatTurn {
	return ac.contextManager.TurnsSnapshot()
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
//   - :clear clears chat pane output
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
			if err := ac.runTmux(ctx, "send-keys", "-t", ac.paneTargetForName("chat"), "clear", "Enter"); err != nil {
				return "", false, fmt.Errorf("failed to clear chat pane: %w", err)
			}
			log.Printf("[AgentX Core] Input command handled: :clear")
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
		Panes:         panes,
		Applets:       applets,
	}
}

// StartHealthEndpoint starts the health/status HTTP endpoint.
func (ac *AgentXCore) StartHealthEndpoint(ctx context.Context) error {
	go func() {
		if err := ac.contextManager.ServeHealth(ctx, ac.healthAddr); err != nil {
			log.Printf("[AgentX Core] Health endpoint error: %v", err)
		}
	}()
	log.Printf("[AgentX Core] Health endpoint listening on %s", ac.healthAddr)
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
	cmd := exec.CommandContext(ctx, "tmux", "kill-session", "-t", ac.tmuxSessionName)
	if err := cmd.Run(); err != nil {
		log.Printf("[AgentX Core] Warning: failed to kill tmux session: %v", err)
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
	turns            []ChatTurn
	turnsLoaded      bool
	mu               sync.RWMutex
}

// NewContextManager creates a new context manager.
func NewContextManager(contextDir string) *ContextManager {
	return &ContextManager{
		contextDir: contextDir,
		startedAt:  time.Now(),
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
		writeJSON(w, http.StatusOK, map[string]any{
			"session_id": snapshot.SessionID,
			"turn_count": len(turns),
			"turns":      turns,
		})
	})

	return mux
}

// ServeHealth starts an HTTP health endpoint.
func (cm *ContextManager) ServeHealth(ctx context.Context, addr string) error {
	server := &http.Server{
		Addr:    addr,
		Handler: cm.HealthHandler(),
	}

	errCh := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
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
