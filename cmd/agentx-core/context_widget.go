package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type contextWidgetActivity struct {
	State string `json:"state"`
	Phase string `json:"phase"`
}

type contextWidgetSnapshot struct {
	SessionID   string                `json:"session_id"`
	TurnCount   int                   `json:"turn_count"`
	Turns       []ChatTurn            `json:"turns"`
	PromptCycle PromptCycleStatus     `json:"prompt_cycle"`
	Activity    contextWidgetActivity `json:"activity"`
}

type contextHistorySession struct {
	SessionID   string
	Turns       []ChatTurn
	LastUpdated time.Time
}

type contextFeedbackViewState struct {
	showHelp                bool
	collapsedContextHistory bool
	collapsedPriorSessions  map[string]bool
	collapsedEntries        map[string]bool
	disabledEntries         map[string]bool
	selectedEntries         map[string]bool
	orderedRowKeys          []string
	activeRow               int
	focusTextBox            bool
	textScroll              map[string]int
	showWorkingMemory       bool
	collapsedWorkingMemory  bool
	statusLine              string
	statusUntil             time.Time
}

const (
	ansiReset   = "\033[0m"
	ansiReverse = "\033[7m"
	ansiCyan    = "\033[36m"
	ansiBlue    = "\033[34m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiRed     = "\033[31m"
	ansiMagenta = "\033[35m"
)

func newContextFeedbackViewState() *contextFeedbackViewState {
	return &contextFeedbackViewState{
		collapsedPriorSessions:  make(map[string]bool),
		collapsedEntries:        make(map[string]bool),
		disabledEntries:         make(map[string]bool),
		selectedEntries:         make(map[string]bool),
		orderedRowKeys:          []string{},
		textScroll:              make(map[string]int),
		showWorkingMemory:       true,
		collapsedContextHistory: true,
		collapsedWorkingMemory:  true,
	}
}

func (state *contextFeedbackViewState) updateOrderedRows(keys []string) {
	if state == nil {
		return
	}
	state.orderedRowKeys = append([]string{}, keys...)
	if len(state.orderedRowKeys) == 0 {
		state.activeRow = 0
		state.focusTextBox = false
		return
	}
	if state.activeRow < 0 {
		state.activeRow = 0
	}
	if state.activeRow >= len(state.orderedRowKeys) {
		state.activeRow = len(state.orderedRowKeys) - 1
	}
}

func (state *contextFeedbackViewState) activeRowKey() string {
	if state == nil || len(state.orderedRowKeys) == 0 {
		return ""
	}
	if state.activeRow < 0 || state.activeRow >= len(state.orderedRowKeys) {
		return ""
	}
	return state.orderedRowKeys[state.activeRow]
}

func (state *contextFeedbackViewState) moveRow(delta int) bool {
	if state == nil || len(state.orderedRowKeys) == 0 {
		return false
	}
	next := state.activeRow + delta
	if next < 0 {
		next = 0
	}
	if next >= len(state.orderedRowKeys) {
		next = len(state.orderedRowKeys) - 1
	}
	changed := next != state.activeRow
	state.activeRow = next
	if changed {
		state.focusTextBox = false
	}
	return changed
}

func (state *contextFeedbackViewState) setActiveRowByKey(target string) bool {
	if state == nil || strings.TrimSpace(target) == "" {
		return false
	}
	for idx, key := range state.orderedRowKeys {
		if key != target {
			continue
		}
		changed := state.activeRow != idx
		state.activeRow = idx
		if changed {
			state.focusTextBox = false
		}
		return changed
	}
	return false
}

func (state *contextFeedbackViewState) moveHorizontal(direction string) bool {
	if state == nil {
		return false
	}
	current := state.activeRowKey()
	if strings.TrimSpace(current) == "" {
		return false
	}

	if strings.HasPrefix(current, "current:") {
		parts := strings.Split(current, ":")
		if len(parts) != 3 {
			return false
		}
		turn := strings.TrimSpace(parts[1])
		entry := strings.TrimSpace(parts[2])
		switch direction {
		case "right":
			if entry == "prompt" {
				return state.setActiveRowByKey("current:" + turn + ":response")
			}
		case "left":
			if entry == "response" {
				return state.setActiveRowByKey("current:" + turn + ":prompt")
			}
		}
		return false
	}

	if strings.HasPrefix(current, "session:") {
		if direction != "right" {
			return false
		}
		sessionID := strings.TrimPrefix(current, "session:")
		prefix := "history:" + sessionID + ":"
		for _, key := range state.orderedRowKeys {
			if strings.HasPrefix(key, prefix) {
				return state.setActiveRowByKey(key)
			}
		}
		return false
	}

	if strings.HasPrefix(current, "history:") {
		if direction != "left" {
			return false
		}
		parts := strings.Split(current, ":")
		if len(parts) < 3 {
			return false
		}
		sessionID := strings.TrimSpace(parts[1])
		return state.setActiveRowByKey("session:" + sessionID)
	}

	return false
}

func contextEntryKey(scope string, turnIndex int, entry string) string {
	return fmt.Sprintf("%s:%d:%s", strings.TrimSpace(scope), turnIndex, strings.ToLower(strings.TrimSpace(entry)))
}

func normalizeContextEntry(entry string) string {
	switch strings.ToLower(strings.TrimSpace(entry)) {
	case "prompt", "user":
		return "prompt"
	case "response", "assistant", "agent":
		return "response"
	case "both", "all":
		return "both"
	default:
		return ""
	}
}

func (state *contextFeedbackViewState) setStatus(status string) {
	if state == nil {
		return
	}
	state.statusLine = strings.TrimSpace(status)
	state.statusUntil = time.Now().Add(8 * time.Second)
}

func (state *contextFeedbackViewState) activeStatus() string {
	if state == nil {
		return ""
	}
	if strings.TrimSpace(state.statusLine) == "" {
		return ""
	}
	if time.Now().After(state.statusUntil) {
		state.statusLine = ""
		return ""
	}
	return state.statusLine
}

func runContextWidgetCommand(coreHTTP string, out io.Writer) int {
	return runContextWidgetCommandWithInput(coreHTTP, nil, out)
}

func runContextWidgetCommandWithInput(coreHTTP string, in io.Reader, out io.Writer) int {
	baseURL := strings.TrimSpace(coreHTTP)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AGENTX_CORE_HTTP"))
	}
	if baseURL == "" {
		fmt.Fprintln(out, "Context widget failed: missing core HTTP base URL")
		return 1
	}

	if err := runContextWidgetLoopWithInput(context.Background(), strings.TrimRight(baseURL, "/"), in, out, 300*time.Millisecond); err != nil {
		fmt.Fprintf(out, "Context widget failed: %v\n", err)
		return 1
	}
	return 0
}

func runContextWidgetLoop(ctx context.Context, baseURL string, out io.Writer, refreshInterval time.Duration) error {
	return runContextWidgetLoopWithInput(ctx, baseURL, nil, out, refreshInterval)
}

func runContextWidgetLoopWithInput(ctx context.Context, baseURL string, in io.Reader, out io.Writer, refreshInterval time.Duration) error {
	if refreshInterval <= 0 {
		refreshInterval = 300 * time.Millisecond
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	commandInput := in
	if commandInput == nil {
		commandInput = strings.NewReader("")
	}
	commandReader, promptMode, cleanup := newFilesystemWidgetCommandReader(commandInput)
	defer cleanup()
	commands := make(chan string, 16)
	go func() {
		defer close(commands)
		for {
			cmd, err := commandReader()
			if err != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case commands <- cmd:
			}
		}
	}()
	viewState := newContextFeedbackViewState()
	currentSnapshot := contextWidgetSnapshot{}
	var previousLines []string

	for {
		renderChanged := false
		for {
			select {
			case cmd, ok := <-commands:
				if !ok {
					return nil
				}
				normalized := normalizeContextWidgetControlCommand(cmd)
				action := handleWidgetLoopControlCommand(normalized, widgetLoopControlHandlers{
					QuitTokens:    []string{"q", "quit"},
					HelpTokens:    []string{"help"},
					RefreshTokens: []string{"r", "refresh"},
					OnHelp: func() {
						viewState.showHelp = true
						viewState.setStatus("Context feedback controls visible.")
					},
					OnRefresh: func() {
						viewState.setStatus("Refreshed.")
					},
				})
				if action == widgetLoopControlQuit {
					return nil
				}
				if action == widgetLoopControlHandled {
					renderChanged = true
					continue
				}
				applyContextWidgetCommand(viewState, cmd, baseURL, currentSnapshot)
				renderChanged = true
			default:
				goto commandQueueDrained
			}
		}
	commandQueueDrained:

		snapshot, err := fetchContextWidgetSnapshot(ctx, baseURL)
		if err == nil {
			currentSnapshot = snapshot
			height, width := resolveWidgetPaneSizeForWriter(out)
			tab := resolveContextWidgetTab()
			model := strings.TrimSpace(os.Getenv("AGENTX_OLLAMA_MODEL"))
			if model == "" {
				model = defaultOllamaModel
			}
			backend := strings.TrimSpace(os.Getenv("AGENTX_CHAT_BACKEND"))
			if backend == "" {
				backend = defaultChatBackend
			}
			history := discoverContextHistorySessions(snapshot.SessionID)

			render := renderContextWidgetWithState(snapshot, tab, model, backend, height, width, history, viewState)
			currentLines := filesystemWidgetFrameLines(render)
			if renderChanged || len(previousLines) == 0 || strings.Join(previousLines, "\n") != strings.Join(currentLines, "\n") {
				if err := writeFilesystemWidgetFrameDiff(out, previousLines, currentLines); err != nil {
					return err
				}
				previousLines = currentLines
				if promptMode {
					if _, writeErr := fmt.Fprint(out, "context> "); writeErr != nil {
						return writeErr
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func normalizeContextWidgetControlCommand(raw string) string {
	return normalizeWidgetControlCommand(raw, defaultWidgetControlAliases())
}

func discoverContextHistorySessions(currentSessionID string) []contextHistorySession {
	projectDir := strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR"))
	username := strings.TrimSpace(os.Getenv("AGENTX_USERNAME"))
	if projectDir == "" || username == "" {
		return []contextHistorySession{}
	}

	sessionsRoot := filepath.Join(projectDir, "sessions", username)
	entries, err := os.ReadDir(sessionsRoot)
	if err != nil {
		return []contextHistorySession{}
	}

	history := make([]contextHistorySession, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := strings.TrimSpace(entry.Name())
		if sessionID == "" || sessionID == strings.TrimSpace(currentSessionID) {
			continue
		}
		turnPath := filepath.Join(sessionsRoot, sessionID, "context", "turns.jsonl")
		turns, lastUpdated, ok := loadSessionTurns(turnPath)
		if !ok {
			continue
		}
		history = append(history, contextHistorySession{SessionID: sessionID, Turns: turns, LastUpdated: lastUpdated})
	}

	sort.SliceStable(history, func(i, j int) bool {
		if history[i].LastUpdated.Equal(history[j].LastUpdated) {
			return history[i].SessionID > history[j].SessionID
		}
		return history[i].LastUpdated.After(history[j].LastUpdated)
	})
	return history
}

func loadSessionTurns(turnPath string) ([]ChatTurn, time.Time, bool) {
	data, err := os.ReadFile(turnPath)
	if err != nil {
		return nil, time.Time{}, false
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return []ChatTurn{}, time.Time{}, true
	}

	lines := strings.Split(trimmed, "\n")
	turns := make([]ChatTurn, 0, len(lines))
	var lastUpdated time.Time
	for _, line := range lines {
		candidate := strings.TrimSpace(line)
		if candidate == "" {
			continue
		}
		var turn ChatTurn
		if err := json.Unmarshal([]byte(candidate), &turn); err != nil {
			continue
		}
		turns = append(turns, turn)
		if turn.CreatedAt > 0 {
			created := time.UnixMilli(turn.CreatedAt)
			if created.After(lastUpdated) {
				lastUpdated = created
			}
		}
	}
	if lastUpdated.IsZero() {
		if info, err := os.Stat(turnPath); err == nil {
			lastUpdated = info.ModTime()
		}
	}
	return turns, lastUpdated, true
}

func fetchContextWidgetSnapshot(ctx context.Context, baseURL string) (contextWidgetSnapshot, error) {
	var snapshot contextWidgetSnapshot
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/context", nil)
	if err != nil {
		return snapshot, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return snapshot, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return snapshot, fmt.Errorf("context failed with status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return snapshot, err
	}
	if snapshot.TurnCount == 0 {
		snapshot.TurnCount = len(snapshot.Turns)
	}
	return snapshot, nil
}

func resolveContextWidgetTab() string {
	override := normalizeSystemTab(strings.TrimSpace(os.Getenv("AGENTX_CONTEXT_WIDGET_TAB")))
	if override != "" && override != "full" {
		return override
	}

	projectDir := strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR"))
	if projectDir == "" {
		return "context-visualizer"
	}
	statePath := filepath.Join(projectDir, ".agentx", "system-panel-tab.txt")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return "context-visualizer"
	}
	tab := normalizeSystemTab(string(data))
	if tab == "" || tab == "full" {
		return "context-visualizer"
	}
	return tab
}

func renderContextWidget(snapshot contextWidgetSnapshot, tab string, model string, backend string, paneHeight int, paneWidth int) string {
	return renderContextWidgetWithState(snapshot, tab, model, backend, paneHeight, paneWidth, nil, nil)
}

func renderContextWidgetWithState(snapshot contextWidgetSnapshot, tab string, model string, backend string, paneHeight int, paneWidth int, history []contextHistorySession, viewState *contextFeedbackViewState) string {
	lines := []string{
		"[SYSTEM]",
		fmt.Sprintf("[SYSTEM TAB] active=%s", tab),
	}

	projectDir := strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR"))

	turnCount := snapshot.TurnCount
	if turnCount == 0 {
		turnCount = len(snapshot.Turns)
	}
	lastPrompt := "none"
	lastResponse := "none"
	if len(snapshot.Turns) > 0 {
		lastPrompt = trimSingleLine(snapshot.Turns[len(snapshot.Turns)-1].Prompt, 64)
		lastResponse = trimSingleLine(snapshot.Turns[len(snapshot.Turns)-1].Response, 64)
	}

	userTokens := 0
	assistantTokens := 0
	for _, turn := range snapshot.Turns {
		userTokens += len(strings.Fields(turn.Prompt))
		assistantTokens += len(strings.Fields(turn.Response))
	}
	total := userTokens + assistantTokens
	maxTokens := maxInt(8192, total)
	consumedPct := percentOfCapacity(total, maxTokens)
	userPct := percentOfCapacity(userTokens, maxTokens)
	assistantPct := percentOfCapacity(assistantTokens, maxTokens)
	remaining := maxInt(0, maxTokens-total)
	remainingPct := percentOfCapacity(remaining, maxTokens)

	switch tab {
	case "files":
		if applet, ok := newSystemAppletHost().Resolve(tab); ok {
			lines = append(lines, applet.RenderWidget(SystemAppletWidgetContext{
				SessionID:  snapshot.SessionID,
				ProjectDir: projectDir,
				TurnCount:  turnCount,
				Turns:      snapshot.Turns,
			})...)
		} else {
			lines = append(lines,
				"== FILES ==",
				fmt.Sprintf("project_dir: %s", trimSingleLine(projectDir, 40)),
				fmt.Sprintf("entry_count: %d", len(safeListDir(projectDir))),
			)
		}
	case "configuration":
		ollamaHost := strings.TrimSpace(os.Getenv("AGENTX_OLLAMA_HOST"))
		if ollamaHost == "" {
			ollamaHost = defaultOllamaHost
		}
		if applet, ok := newSystemAppletHost().Resolve(tab); ok {
			lines = append(lines, applet.RenderWidget(SystemAppletWidgetContext{
				SessionID:  snapshot.SessionID,
				ProjectDir: projectDir,
				Model:      model,
				Backend:    backend,
				OllamaHost: ollamaHost,
				TurnCount:  turnCount,
				Turns:      snapshot.Turns,
			})...)
		} else {
			lines = append(lines,
				"== CONFIGURATION ==",
				fmt.Sprintf("model: %s", trimSingleLine(model, 32)),
				fmt.Sprintf("backend: %s", trimSingleLine(backend, 20)),
				fmt.Sprintf("ollama_host: %s", trimSingleLine(ollamaHost, 32)),
			)
		}
	case "context":
		lines = append(lines,
			"== CONTEXT ==",
			fmt.Sprintf("session_id: %s", trimSingleLine(snapshot.SessionID, 40)),
			fmt.Sprintf("turn_count: %d", turnCount),
			fmt.Sprintf("last_user: %s", lastPrompt),
			fmt.Sprintf("last_agent: %s", lastResponse),
		)
	case "context-history":
		if applet, ok := newSystemAppletHost().Resolve(tab); ok {
			lines = append(lines, applet.RenderWidget(SystemAppletWidgetContext{
				SessionID:  snapshot.SessionID,
				ProjectDir: projectDir,
				TurnCount:  turnCount,
				Turns:      snapshot.Turns,
			})...)
		}
		lines = append(lines, renderContextFeedbackSections(snapshot, history, viewState)...)
	default:
		lines = append(lines,
			"== CONTEXT WINDOW ==",
			fmt.Sprintf("model: %s | backend: %s", trimSingleLine(model, 24), trimSingleLine(backend, 12)),
			fmt.Sprintf("consumed: %.1f%% (%d/%d)", consumedPct, total, maxTokens),
			"Top Contributors:",
			fmt.Sprintf("  1. 👤 User Prompts    %.1f%%", userPct),
			fmt.Sprintf("  2. 🤖 Agent Response %.1f%%", assistantPct),
			"== CONTEXT VISUALIZER ==",
			meterRow("💾 Working Memory", 0, maxTokens),
			meterRow("🧠 System Prompts", 0, maxTokens),
			meterRow("👤 User Prompts", userTokens, maxTokens),
			meterRow("📎 Attachments", 0, maxTokens),
			meterRow("🤔 Thinking", 0, maxTokens),
			meterRow("🤖 Agent Response", assistantTokens, maxTokens),
			meterRow("🔧 Tool Calls", 0, maxTokens),
			fmt.Sprintf("░ Remaining            %s %d (%.1f%%)", meterBar(remaining, maxTokens), remaining, remainingPct),
			"== PROMPT CYCLE ==",
			fmt.Sprintf("○ 🤔 Classify %s", formatCycleElapsed(snapshot.PromptCycle.Classify)),
			fmt.Sprintf("○ 💭 Think    %s", formatCycleElapsed(snapshot.PromptCycle.Thinking)),
			fmt.Sprintf("○ 🔧 Tool     %s", formatCycleElapsed(snapshot.PromptCycle.Tool)),
			fmt.Sprintf("○ 🤖 Respond  %s", formatCycleElapsed(snapshot.PromptCycle.Respond)),
		)
	}

	lines = append(lines, fmt.Sprintf("turn_count: %d", turnCount))
	lines = fitLinesToWidth(lines, paneWidth)
	lines = clipLinesForHeight(lines, paneHeight-1)
	return strings.Join(lines, "\n")
}

func renderContextFeedbackSections(snapshot contextWidgetSnapshot, history []contextHistorySession, viewState *contextFeedbackViewState) []string {
	rowKeys := make([]string, 0)
	lines := []string{
		"",
		reverseTitle("CONTEXT FEEDBACK"),
	}

	// Section order follows the UX contract:
	// 1) Context history (title only when collapsed)
	// 2) Working memory (title only when collapsed)
	// 3) Current context (expanded, element tiles collapsed by default)
	lines = append(lines, "", reverseTitle("CONTEXT HISTORY"))
	historyCollapsed := true
	if viewState != nil {
		historyCollapsed = viewState.collapsedContextHistory
	}
	if !historyCollapsed {
		if len(history) == 0 {
			lines = append(lines, "No prior sessions found on disk.")
		} else {
			for _, session := range history {
				sessionKey := "session:" + session.SessionID
				rowKeys = append(rowKeys, sessionKey)
				collapsed := viewState != nil && viewState.collapsedPriorSessions[session.SessionID]
				updatedAt := "unknown"
				if !session.LastUpdated.IsZero() {
					updatedAt = session.LastUpdated.Format(time.RFC3339)
				}
				sessionLines := []string{
					fmt.Sprintf("%s session: %s", rowMarker(viewState, sessionKey), trimSingleLine(session.SessionID, 48)),
					fmt.Sprintf("turns: %d | updated: %s", len(session.Turns), updatedAt),
					fmt.Sprintf("state: %s | Enter collapse/expand", mapCollapsedState(collapsed)),
				}
				if collapsed {
					lines = append(lines, boxSection("SESSION", sessionLines, ansiMagenta)...)
					continue
				}
				for idx, turn := range session.Turns {
					turnNumber := idx + 1
					turnKey := fmt.Sprintf("history:%s:%d", session.SessionID, turnNumber)
					rowKeys = append(rowKeys, turnKey)
					sessionLines = append(sessionLines,
						fmt.Sprintf("%s %s %d user: %s", rowMarker(viewState, turnKey), styleToken("👤", ansiBlue), turnNumber, trimSingleLine(turn.Prompt, 68)),
						fmt.Sprintf("%s %d agent: %s", styleToken("🤖", ansiGreen), turnNumber, trimSingleLine(turn.Response, 68)),
						fmt.Sprintf("%s include: i %s %d b | Space select", styleToken("➕", ansiCyan), session.SessionID, turnNumber),
					)
				}
				lines = append(lines, boxSection("SESSION", sessionLines, ansiMagenta)...)
			}
		}
	}

	if viewState == nil || viewState.showWorkingMemory {
		lines = append(lines, "")
		lines = append(lines, renderWorkingMemoryFeedbackSection(viewState, &rowKeys)...)
	}

	lines = append(lines, "", reverseTitle("CURRENT CONTEXT"))
	if len(snapshot.Turns) == 0 {
		lines = append(lines, "No current context elements.")
	} else {
		for idx, turn := range snapshot.Turns {
			turnNumber := idx + 1
			promptKey := contextEntryKey("current", turnNumber, "prompt")
			responseKey := contextEntryKey("current", turnNumber, "response")
			rowKeys = append(rowKeys, promptKey, responseKey)

			if viewState != nil {
				if _, exists := viewState.collapsedEntries[promptKey]; !exists {
					viewState.collapsedEntries[promptKey] = true
				}
				if _, exists := viewState.collapsedEntries[responseKey]; !exists {
					viewState.collapsedEntries[responseKey] = true
				}
			}

			promptCollapsed := viewState != nil && viewState.collapsedEntries[promptKey]
			responseCollapsed := viewState != nil && viewState.collapsedEntries[responseKey]
			promptDisabled := viewState != nil && viewState.disabledEntries[promptKey]
			responseDisabled := viewState != nil && viewState.disabledEntries[responseKey]

			turnLines := []string{fmt.Sprintf("%s turn %d", styleToken("↳", ansiMagenta), turnNumber)}
			if promptCollapsed {
				turnLines = append(turnLines, fmt.Sprintf("%s %s prompt: %s [%s]", rowMarker(viewState, promptKey), styleToken("👤", ansiBlue), styleToken("[collapsed]", ansiYellow), mapContextEntryState(promptDisabled)))
			} else {
				turnLines = append(turnLines, fmt.Sprintf("%s %s prompt [%s]", rowMarker(viewState, promptKey), styleToken("👤", ansiBlue), mapContextEntryState(promptDisabled)))
				turnLines = append(turnLines, renderWrappedTextBox(viewState, promptKey, turn.Prompt, ansiBlue)...)
			}
			if responseCollapsed {
				turnLines = append(turnLines, fmt.Sprintf("%s %s response: %s [%s]", rowMarker(viewState, responseKey), styleToken("🤖", ansiGreen), styleToken("[collapsed]", ansiYellow), mapContextEntryState(responseDisabled)))
			} else {
				turnLines = append(turnLines, fmt.Sprintf("%s %s response [%s]", rowMarker(viewState, responseKey), styleToken("🤖", ansiGreen), mapContextEntryState(responseDisabled)))
				turnLines = append(turnLines, renderWrappedTextBox(viewState, responseKey, turn.Response, ansiGreen)...)
			}
			lines = append(lines, boxSection(fmt.Sprintf("TURN %d", turnNumber), turnLines, ansiCyan)...)
		}
	}
	if viewState != nil {
		viewState.updateOrderedRows(rowKeys)
	}

	return lines
}

func renderWorkingMemoryFeedbackSection(viewState *contextFeedbackViewState, rowKeys *[]string) []string {
	lines := []string{reverseTitle("WORKING MEMORY")}
	if viewState != nil && viewState.collapsedWorkingMemory {
		return lines
	}
	sessionDir := resolveCurrentSessionDirFromEnv()
	sessionID := strings.TrimSpace(os.Getenv("AGENTX_SESSION_ID"))
	if sessionID != "" {
		lines = append(lines, fmt.Sprintf("session_id: %s", trimSingleLine(sessionID, 48)))
	}
	if strings.TrimSpace(sessionDir) == "" {
		return append(lines, "Session path unavailable.")
	}

	facts := loadWorkingMemoryFacts(sessionDir)
	lines = append(lines, fmt.Sprintf("fact_count: %d", len(facts)))
	if len(facts) == 0 {
		lines = append(lines, "No facts stored yet.")
	} else {
		factLines := make([]string, 0, len(facts)+1)
		for idx, fact := range facts {
			factKey := fmt.Sprintf("wm:%s:%s", fact.owner, fact.key)
			if rowKeys != nil {
				*rowKeys = append(*rowKeys, factKey)
			}
			status := "disabled"
			if fact.enabled {
				status = "enabled"
			}
			ownerIcon := styleToken("👤", ansiBlue)
			if strings.EqualFold(fact.owner, "agent") {
				ownerIcon = styleToken("🤖", ansiGreen)
			}
			statusLabel := status
			if status == "disabled" {
				statusLabel = styleToken(status, ansiRed)
			} else {
				statusLabel = styleToken(status, ansiGreen)
			}
			factLines = append(factLines, fmt.Sprintf("%s %d) %s %s:%s [%s] = %s", rowMarker(viewState, factKey), idx+1, ownerIcon, fact.owner, trimSingleLine(fact.key, 32), statusLabel, trimSingleLine(formatWorkingMemoryValue(fact.value), 72)))
		}
		lines = append(lines, boxSection("FACTS", factLines, ansiGreen)...)
	}
	lines = append(lines,
		"actions: mk <key> <value> | md <key> | mt <key>",
	)
	return lines
}

func reverseTitle(title string) string {
	label := strings.TrimSpace(title)
	if label == "" {
		return ""
	}
	return ansiReverse + " " + label + " " + ansiReset
}

func styleToken(text string, color string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	return color + text + ansiReset
}

func boxSection(title string, items []string, borderColor string) []string {
	cleanTitle := strings.TrimSpace(title)
	if len(items) == 0 {
		items = []string{"(empty)"}
	}
	maxWidth := len([]rune(cleanTitle))
	for _, item := range items {
		if width := len([]rune(stripAnsi(item))); width > maxWidth {
			maxWidth = width
		}
	}
	if maxWidth < 16 {
		maxWidth = 16
	}
	line := strings.Repeat("─", maxWidth+2)
	lines := []string{borderColor + "┌" + line + "┐" + ansiReset}
	header := reverseTitle(cleanTitle)
	lines = append(lines, borderColor+"│ "+padVisibleWidth(header, maxWidth)+" │"+ansiReset)
	for _, item := range items {
		lines = append(lines, borderColor+"│ "+padVisibleWidth(item, maxWidth)+" │"+ansiReset)
	}
	lines = append(lines, borderColor+"└"+line+"┘"+ansiReset)
	return lines
}

func stripAnsi(value string) string {
	if strings.IndexByte(value, '\x1b') == -1 {
		return value
	}
	var out strings.Builder
	inEsc := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if inEsc {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				inEsc = false
			}
			continue
		}
		if ch == 0x1b {
			inEsc = true
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func padVisibleWidth(value string, width int) string {
	visible := len([]rune(stripAnsi(value)))
	if visible >= width {
		return value
	}
	return value + strings.Repeat(" ", width-visible)
}

func mapCollapsedState(collapsed bool) string {
	if collapsed {
		return "collapsed"
	}
	return "expanded"
}

func mapContextEntryState(disabled bool) string {
	if disabled {
		return "disabled"
	}
	return "enabled"
}

func rowMarker(state *contextFeedbackViewState, rowKey string) string {
	if state == nil {
		return "  "
	}
	active := state.activeRowKey() == rowKey
	selected := state.selectedEntries[rowKey]
	switch {
	case active && selected:
		return styleToken("▶●", ansiCyan)
	case active:
		return styleToken("▶ ", ansiCyan)
	case selected:
		return styleToken(" ●", ansiGreen)
	default:
		return "  "
	}
}

func wrapTextLines(text string, width int) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return []string{"(empty)"}
	}
	if width < 12 {
		width = 12
	}
	words := strings.Fields(trimmed)
	if len(words) == 0 {
		return []string{"(empty)"}
	}
	lines := make([]string, 0)
	line := words[0]
	for i := 1; i < len(words); i++ {
		candidate := line + " " + words[i]
		if len([]rune(candidate)) <= width {
			line = candidate
			continue
		}
		lines = append(lines, line)
		line = words[i]
	}
	lines = append(lines, line)
	return lines
}

func renderWrappedTextBox(state *contextFeedbackViewState, rowKey string, text string, color string) []string {
	wrapped := wrapTextLines(text, 72)
	offset := 0
	if state != nil {
		offset = state.textScroll[rowKey]
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(wrapped)-1 {
		offset = maxInt(0, len(wrapped)-1)
	}
	end := offset + 5
	if end > len(wrapped) {
		end = len(wrapped)
	}
	visible := wrapped[offset:end]
	for i := range visible {
		visible[i] = styleToken(visible[i], color)
	}
	if len(wrapped) > 5 {
		visible = append(visible, fmt.Sprintf("%s scroll %d/%d (Tab to exit)", styleToken("↕", ansiYellow), offset+1, len(wrapped)-4))
	}
	return visible
}

func applyContextWidgetCommand(state *contextFeedbackViewState, raw string, baseURL string, snapshot contextWidgetSnapshot) {
	if state == nil {
		return
	}
	line := strings.TrimSpace(raw)
	if line == "" {
		return
	}
	args := strings.Fields(strings.TrimPrefix(line, ":"))
	if len(args) == 0 {
		return
	}
	args = normalizeContextCommandAliases(args)

	if handleContextKeyboardCommand(state, args, snapshot) {
		return
	}

	cmd := strings.ToLower(strings.TrimSpace(args[0]))
	switch cmd {
	case "help", "controls":
		state.showHelp = true
		state.setStatus("Context feedback controls visible.")
		return
	case "hide-help":
		state.showHelp = false
		state.setStatus("Context feedback controls hidden.")
		return
	case "toggle":
		if len(args) >= 3 && strings.EqualFold(args[1], "session") {
			sessionID := strings.TrimSpace(args[2])
			if sessionID == "" {
				state.setStatus("Usage: :toggle session <session_id>")
				return
			}
			state.collapsedPriorSessions[sessionID] = !state.collapsedPriorSessions[sessionID]
			state.setStatus(fmt.Sprintf("Session %s is now %s.", sessionID, mapCollapsedState(state.collapsedPriorSessions[sessionID])))
			return
		}
		if len(args) >= 2 && strings.EqualFold(args[1], "history") {
			state.collapsedContextHistory = !state.collapsedContextHistory
			state.setStatus(fmt.Sprintf("Context history is now %s.", mapCollapsedState(state.collapsedContextHistory)))
			return
		}
		if len(args) >= 2 && strings.EqualFold(args[1], "wm") {
			state.collapsedWorkingMemory = !state.collapsedWorkingMemory
			state.setStatus(fmt.Sprintf("Working memory is now %s.", mapCollapsedState(state.collapsedWorkingMemory)))
			return
		}
		state.setStatus("Usage: :toggle history | :toggle session <session_id> | :toggle wm")
		return
	case "collapse", "expand":
		if len(args) < 4 || !strings.EqualFold(args[1], "current") {
			state.setStatus(fmt.Sprintf("Usage: :%s current <turn> <prompt|response|both>", cmd))
			return
		}
		turnIndex, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil || turnIndex < 1 || turnIndex > len(snapshot.Turns) {
			state.setStatus("Invalid current-session turn index.")
			return
		}
		entry := normalizeContextEntry(args[3])
		if entry == "" {
			state.setStatus("Entry must be prompt, response, or both.")
			return
		}
		collapsed := strings.EqualFold(cmd, "collapse")
		if entry == "both" {
			state.collapsedEntries[contextEntryKey("current", turnIndex, "prompt")] = collapsed
			state.collapsedEntries[contextEntryKey("current", turnIndex, "response")] = collapsed
		} else {
			state.collapsedEntries[contextEntryKey("current", turnIndex, entry)] = collapsed
		}
		state.setStatus(fmt.Sprintf("Set turn %d %s to %s.", turnIndex, entry, mapCollapsedState(collapsed)))
		return
	case "disable":
		if len(args) < 4 || !strings.EqualFold(args[1], "current") {
			state.setStatus("Usage: :disable current <turn> <prompt|response|both> [on|off|toggle]")
			return
		}
		turnIndex, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil || turnIndex < 1 || turnIndex > len(snapshot.Turns) {
			state.setStatus("Invalid current-session turn index.")
			return
		}
		entry := normalizeContextEntry(args[3])
		if entry == "" {
			state.setStatus("Entry must be prompt, response, or both.")
			return
		}
		action := "toggle"
		if len(args) >= 5 {
			action = strings.ToLower(strings.TrimSpace(args[4]))
		}
		setDisabled := func(targetEntry string) {
			key := contextEntryKey("current", turnIndex, targetEntry)
			switch action {
			case "on", "true", "1", "enable=false":
				state.disabledEntries[key] = true
			case "off", "false", "0", "enable":
				delete(state.disabledEntries, key)
			default:
				if state.disabledEntries[key] {
					delete(state.disabledEntries, key)
				} else {
					state.disabledEntries[key] = true
				}
			}
		}
		if entry == "both" {
			setDisabled("prompt")
			setDisabled("response")
		} else {
			setDisabled(entry)
		}
		state.setStatus(fmt.Sprintf("Updated disabled state for turn %d %s.", turnIndex, entry))
		return
	case "include":
		if len(args) < 4 {
			state.setStatus("Usage: :include <session_id|current> <turn> <prompt|response|both>")
			return
		}
		source := strings.TrimSpace(args[1])
		turnIndex, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil || turnIndex < 1 {
			state.setStatus("Invalid include turn index.")
			return
		}
		entry := normalizeContextEntry(args[3])
		if entry == "" {
			state.setStatus("Entry must be prompt, response, or both.")
			return
		}

		var turn ChatTurn
		if strings.EqualFold(source, "current") {
			if turnIndex > len(snapshot.Turns) {
				state.setStatus("Current-session turn index out of range.")
				return
			}
			turn = snapshot.Turns[turnIndex-1]
			source = snapshot.SessionID
		} else {
			sessions := discoverContextHistorySessions(snapshot.SessionID)
			found := false
			for _, session := range sessions {
				if session.SessionID != source {
					continue
				}
				if turnIndex > len(session.Turns) {
					state.setStatus("Historical turn index out of range.")
					return
				}
				turn = session.Turns[turnIndex-1]
				found = true
				break
			}
			if !found {
				state.setStatus("Session not found in history.")
				return
			}
		}

		payload := []string{fmt.Sprintf("[context-import] source_session=%s turn=%d", source, turnIndex)}
		if entry == "prompt" || entry == "both" {
			payload = append(payload, fmt.Sprintf("prompt: %s", strings.TrimSpace(turn.Prompt)))
		}
		if entry == "response" || entry == "both" {
			payload = append(payload, fmt.Sprintf("response: %s", strings.TrimSpace(turn.Response)))
		}
		submitCtx, cancel := context.WithTimeout(context.Background(), resolveWidgetSubmitTimeout())
		defer cancel()
		if _, err := submitPromptToCore(submitCtx, baseURL, strings.Join(payload, "\n")); err != nil {
			state.setStatus(fmt.Sprintf("Include failed: %v", err))
			return
		}
		state.setStatus(fmt.Sprintf("Included %s turn %d (%s) into current context.", source, turnIndex, entry))
		return
	case "wm":
		applyWorkingMemoryCommand(state, args)
		return
	default:
		state.setStatus("Unknown context command (use help or ?).")
	}
}

func handleContextKeyboardCommand(state *contextFeedbackViewState, args []string, snapshot contextWidgetSnapshot) bool {
	if state == nil || len(args) == 0 {
		return false
	}
	cmd := strings.ToLower(strings.TrimSpace(args[0]))
	switch cmd {
	case "j", "down":
		if state.focusTextBox {
			rowKey := state.activeRowKey()
			state.textScroll[rowKey] = state.textScroll[rowKey] + 1
			state.setStatus("Scrolled text box down.")
			return true
		}
		if !state.moveRow(1) {
			state.setStatus("Bottom of list")
		} else {
			state.setStatus("Moved selection")
		}
		return true
	case "k", "up":
		if state.focusTextBox {
			rowKey := state.activeRowKey()
			state.textScroll[rowKey] = maxInt(0, state.textScroll[rowKey]-1)
			state.setStatus("Scrolled text box up.")
			return true
		}
		if !state.moveRow(-1) {
			state.setStatus("Top of list")
		} else {
			state.setStatus("Moved selection")
		}
		return true
	case "right", "l":
		if state.focusTextBox {
			state.setStatus("Press Tab to leave text-box scroll mode.")
			return true
		}
		if state.moveHorizontal("right") {
			state.setStatus("Moved right.")
		} else {
			state.setStatus("No right sibling.")
		}
		return true
	case "left", "h":
		if state.focusTextBox {
			state.setStatus("Press Tab to leave text-box scroll mode.")
			return true
		}
		if state.moveHorizontal("left") {
			state.setStatus("Moved left.")
		} else {
			state.setStatus("No left sibling.")
		}
		return true
	case "pgdn":
		if state.focusTextBox {
			rowKey := state.activeRowKey()
			state.textScroll[rowKey] = state.textScroll[rowKey] + 5
			state.setStatus("Paged text box down.")
			return true
		}
		state.moveRow(5)
		state.setStatus("Moved selection")
		return true
	case "pgup":
		if state.focusTextBox {
			rowKey := state.activeRowKey()
			state.textScroll[rowKey] = maxInt(0, state.textScroll[rowKey]-5)
			state.setStatus("Paged text box up.")
			return true
		}
		state.moveRow(-5)
		state.setStatus("Moved selection")
		return true
	case "tab":
		state.focusTextBox = !state.focusTextBox
		if state.focusTextBox {
			state.setStatus("Text-box focus enabled.")
		} else {
			state.setStatus("Text-box focus disabled.")
		}
		return true
	case "space":
		rowKey := state.activeRowKey()
		if rowKey == "" {
			state.setStatus("No selectable row.")
			return true
		}
		if state.selectedEntries[rowKey] {
			delete(state.selectedEntries, rowKey)
			state.setStatus("Deselected row.")
		} else {
			state.selectedEntries[rowKey] = true
			state.setStatus("Selected row.")
		}
		return true
	case "enter":
		rowKey := state.activeRowKey()
		if rowKey == "" {
			state.setStatus("No expandable row.")
			return true
		}
		if strings.HasPrefix(rowKey, "current:") {
			state.collapsedEntries[rowKey] = !state.collapsedEntries[rowKey]
			state.setStatus(fmt.Sprintf("Row %s is now %s.", rowKey, mapCollapsedState(state.collapsedEntries[rowKey])))
			return true
		}
		if strings.HasPrefix(rowKey, "session:") {
			sessionID := strings.TrimPrefix(rowKey, "session:")
			state.collapsedPriorSessions[sessionID] = !state.collapsedPriorSessions[sessionID]
			state.setStatus(fmt.Sprintf("Session %s is now %s.", sessionID, mapCollapsedState(state.collapsedPriorSessions[sessionID])))
			return true
		}
		if strings.HasPrefix(rowKey, "wm:") {
			state.collapsedWorkingMemory = !state.collapsedWorkingMemory
			state.setStatus(fmt.Sprintf("Working memory is now %s.", mapCollapsedState(state.collapsedWorkingMemory)))
			return true
		}
		_ = snapshot
		state.setStatus("Row has no expand/collapse action.")
		return true
	default:
		return false
	}
}

func normalizeContextCommandAliases(args []string) []string {
	if len(args) == 0 {
		return args
	}
	first := strings.ToLower(strings.TrimSpace(args[0]))
	switch first {
	case "?":
		return []string{"help"}
	case "x":
		if len(args) >= 2 {
			return []string{"toggle", "session", args[1]}
		}
		return []string{"toggle"}
	case "c":
		if len(args) >= 3 {
			return []string{"collapse", "current", args[1], expandContextEntryAlias(args[2])}
		}
	case "e":
		if len(args) >= 3 {
			return []string{"expand", "current", args[1], expandContextEntryAlias(args[2])}
		}
	case "d":
		if len(args) >= 3 {
			normalized := []string{"disable", "current", args[1], expandContextEntryAlias(args[2])}
			if len(args) >= 4 {
				normalized = append(normalized, expandToggleAlias(args[3]))
			}
			return normalized
		}
	case "i":
		if len(args) >= 4 {
			return []string{"include", args[1], args[2], expandContextEntryAlias(args[3])}
		}
	case "m":
		if len(args) == 1 {
			return []string{"wm", "toggle"}
		}
		return []string{"wm", strings.ToLower(strings.TrimSpace(args[1]))}
	case "mk":
		if len(args) >= 3 {
			return append([]string{"wm", "set", args[1]}, args[2:]...)
		}
	case "md":
		if len(args) >= 2 {
			return []string{"wm", "del", args[1]}
		}
	case "mt":
		if len(args) >= 2 {
			return []string{"wm", "toggle", args[1]}
		}
	}
	return args
}

func expandContextEntryAlias(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "p":
		return "prompt"
	case "r":
		return "response"
	case "b", "a":
		return "both"
	default:
		return value
	}
}

func expandToggleAlias(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "t":
		return "toggle"
	default:
		return value
	}
}

func resolveCurrentSessionDirFromEnv() string {
	projectDir := strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR"))
	username := strings.TrimSpace(os.Getenv("AGENTX_USERNAME"))
	sessionID := strings.TrimSpace(os.Getenv("AGENTX_SESSION_ID"))
	if projectDir == "" || username == "" || sessionID == "" {
		return ""
	}
	return filepath.Join(projectDir, "sessions", username, sessionID)
}

func applyWorkingMemoryCommand(state *contextFeedbackViewState, args []string) {
	if state == nil {
		return
	}
	if len(args) < 2 {
		state.setStatus("Usage: :wm show|hide|toggle|set|del|enable|disable")
		return
	}
	action := strings.ToLower(strings.TrimSpace(args[1]))
	sessionDir := resolveCurrentSessionDirFromEnv()
	if action == "show" {
		state.showWorkingMemory = true
		state.collapsedWorkingMemory = false
		state.setStatus("Working memory section shown.")
		return
	}
	if action == "hide" {
		state.showWorkingMemory = false
		state.setStatus("Working memory section hidden.")
		return
	}
	if action == "toggle" && len(args) == 2 {
		state.collapsedWorkingMemory = !state.collapsedWorkingMemory
		state.setStatus(fmt.Sprintf("Working memory details %s.", mapCollapsedState(state.collapsedWorkingMemory)))
		return
	}
	if strings.TrimSpace(sessionDir) == "" {
		state.setStatus("Working memory unavailable (missing session env).")
		return
	}

	payload, err := loadWorkingMemoryPayload(sessionDir)
	if err != nil {
		state.setStatus(fmt.Sprintf("Failed to load working memory: %v", err))
		return
	}

	switch action {
	case "set", "add", "edit":
		if len(args) < 4 {
			state.setStatus("Usage: :wm set <key> <value>")
			return
		}
		key := strings.TrimSpace(args[2])
		value := strings.TrimSpace(strings.Join(args[3:], " "))
		if key == "" {
			state.setStatus("Working-memory key cannot be empty.")
			return
		}
		compound := "user:" + key
		payload[compound] = workingMemoryFactSnapshot{Owner: "user", Key: key, Value: value, Enabled: true}
		if err := saveWorkingMemoryPayload(sessionDir, payload); err != nil {
			state.setStatus(fmt.Sprintf("Failed to persist working memory: %v", err))
			return
		}
		state.setStatus(fmt.Sprintf("Updated working-memory key %s.", key))
	case "del", "delete", "rm", "remove":
		if len(args) < 3 {
			state.setStatus("Usage: :wm del <key>")
			return
		}
		key := strings.TrimSpace(args[2])
		if key == "" {
			state.setStatus("Working-memory key cannot be empty.")
			return
		}
		delete(payload, "user:"+key)
		if err := saveWorkingMemoryPayload(sessionDir, payload); err != nil {
			state.setStatus(fmt.Sprintf("Failed to persist working memory: %v", err))
			return
		}
		state.setStatus(fmt.Sprintf("Deleted working-memory key %s.", key))
	case "enable", "disable", "toggle":
		if len(args) < 3 {
			state.setStatus(fmt.Sprintf("Usage: :wm %s <key>", action))
			return
		}
		key := strings.TrimSpace(args[2])
		compound := "user:" + key
		snapshot, ok := payload[compound]
		if !ok {
			state.setStatus(fmt.Sprintf("Key %s not found.", key))
			return
		}
		switch action {
		case "enable":
			snapshot.Enabled = true
		case "disable":
			snapshot.Enabled = false
		case "toggle":
			snapshot.Enabled = !snapshot.Enabled
		}
		payload[compound] = snapshot
		if err := saveWorkingMemoryPayload(sessionDir, payload); err != nil {
			state.setStatus(fmt.Sprintf("Failed to persist working memory: %v", err))
			return
		}
		state.setStatus(fmt.Sprintf("Updated working-memory key %s (%s).", key, mapContextEntryState(!snapshot.Enabled)))
	default:
		state.setStatus("Unknown :wm command.")
	}
}

func loadWorkingMemoryPayload(sessionDir string) (map[string]workingMemoryFactSnapshot, error) {
	payload := make(map[string]workingMemoryFactSnapshot)
	target := filepath.Join(sessionDir, "working_memory.json")
	raw, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return payload, nil
		}
		return nil, err
	}
	if strings.TrimSpace(string(raw)) == "" {
		return payload, nil
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func saveWorkingMemoryPayload(sessionDir string, payload map[string]workingMemoryFactSnapshot) error {
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sessionDir, "working_memory.json"), encoded, 0o644)
}

func percentOfCapacity(amount int, capacity int) float64 {
	if capacity <= 0 {
		return 0
	}
	if amount < 0 {
		amount = 0
	}
	return (float64(amount) / float64(capacity)) * 100.0
}

func meterBar(amount int, capacity int) string {
	const width = 18
	if capacity <= 0 {
		return "[..................]"
	}
	if amount < 0 {
		amount = 0
	}
	if amount > capacity {
		amount = capacity
	}
	filled := int((float64(amount) / float64(capacity)) * float64(width))
	if amount > 0 && filled == 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat(".", width-filled) + "]"
}

func meterRow(label string, amount int, capacity int) string {
	return fmt.Sprintf("%-22s %s %d (%.1f%%)", label, meterBar(amount, capacity), amount, percentOfCapacity(amount, capacity))
}

func fitLinesToWidth(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	fitted := make([]string, 0, len(lines))
	for _, line := range lines {
		if len([]rune(stripAnsi(line))) <= width {
			fitted = append(fitted, line)
			continue
		}
		if strings.IndexByte(line, '\x1b') != -1 {
			fitted = append(fitted, line)
			continue
		}
		runes := []rune(line)
		if width <= 3 {
			fitted = append(fitted, string(runes[:width]))
			continue
		}
		fitted = append(fitted, strings.TrimSpace(string(runes[:width-3]))+"...")
	}
	return fitted
}

func clipLinesForHeight(lines []string, height int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	if height <= 4 {
		return lines[:height]
	}
	headCount := height - 2
	if headCount < 2 {
		headCount = 2
	}
	truncated := len(lines) - headCount - 1
	if truncated < 0 {
		truncated = 0
	}
	clipped := make([]string, 0, height)
	clipped = append(clipped, lines[:headCount]...)
	clipped = append(clipped, fmt.Sprintf("... (%d lines truncated)", truncated))
	clipped = append(clipped, lines[len(lines)-1])
	return clipped
}
