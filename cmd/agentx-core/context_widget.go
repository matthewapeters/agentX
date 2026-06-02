package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type contextWidgetActivity struct {
	State string `json:"state"`
	Phase string `json:"phase"`
}

type contextWidgetSnapshot struct {
	SessionID   string               `json:"session_id"`
	TurnCount   int                  `json:"turn_count"`
	Turns       []ChatTurn           `json:"turns"`
	PromptCycle PromptCycleStatus    `json:"prompt_cycle"`
	Activity    contextWidgetActivity `json:"activity"`
}

func runContextWidgetCommand(coreHTTP string, out io.Writer) int {
	baseURL := strings.TrimSpace(coreHTTP)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AGENTX_CORE_HTTP"))
	}
	if baseURL == "" {
		fmt.Fprintln(out, "Context widget failed: missing core HTTP base URL")
		return 1
	}

	if err := runContextWidgetLoop(context.Background(), strings.TrimRight(baseURL, "/"), out, 300*time.Millisecond); err != nil {
		fmt.Fprintf(out, "Context widget failed: %v\n", err)
		return 1
	}
	return 0
}

func runContextWidgetLoop(ctx context.Context, baseURL string, out io.Writer, refreshInterval time.Duration) error {
	if refreshInterval <= 0 {
		refreshInterval = 300 * time.Millisecond
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	lastRender := ""
	for {
		snapshot, err := fetchContextWidgetSnapshot(ctx, baseURL)
		if err == nil {
			height, width := resolveWidgetPaneSize()
			tab := resolveContextWidgetTab()
			model := strings.TrimSpace(os.Getenv("AGENTX_OLLAMA_MODEL"))
			if model == "" {
				model = defaultOllamaModel
			}
			backend := strings.TrimSpace(os.Getenv("AGENTX_CHAT_BACKEND"))
			if backend == "" {
				backend = defaultChatBackend
			}

			render := renderContextWidget(snapshot, tab, model, backend, height, width)
			if render != lastRender {
				if _, writeErr := fmt.Fprintf(out, "\033[H\033[2J%s\n", render); writeErr != nil {
					return writeErr
				}
				lastRender = render
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
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

func resolveWidgetPaneSize() (height int, width int) {
	height = 40
	width = 100
	if raw := strings.TrimSpace(os.Getenv("LINES")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 4 {
			height = parsed
		}
	}
	if raw := strings.TrimSpace(os.Getenv("COLUMNS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 20 {
			width = parsed
		}
	}
	return height, width
}

func renderContextWidget(snapshot contextWidgetSnapshot, tab string, model string, backend string, paneHeight int, paneWidth int) string {
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
	recentPrompt := "none"
	if len(snapshot.Turns) > 0 {
		lastPrompt = trimSingleLine(snapshot.Turns[len(snapshot.Turns)-1].Prompt, 64)
		lastResponse = trimSingleLine(snapshot.Turns[len(snapshot.Turns)-1].Response, 64)
	}
	if len(snapshot.Turns) > 1 {
		recentPrompt = trimSingleLine(snapshot.Turns[len(snapshot.Turns)-2].Prompt, 64)
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
		} else {
			lines = append(lines,
				"== CONTEXT HISTORY ==",
				fmt.Sprintf("history_context_count: %d", turnCount),
				fmt.Sprintf("recent_prompt: %s", recentPrompt),
			)
		}
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
		runes := []rune(line)
		if len(runes) <= width {
			fitted = append(fitted, line)
			continue
		}
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
