package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type outputWidgetSnapshot struct {
	SessionID   string           `json:"session_id"`
	TurnCount   int              `json:"turn_count"`
	Turns       []ChatTurn       `json:"turns"`
	PromptCycle PromptCycleStatus `json:"prompt_cycle"`
}

func runOutputWidgetCommand(coreHTTP string, out io.Writer) int {
	baseURL := strings.TrimSpace(coreHTTP)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AGENTX_CORE_HTTP"))
	}
	if baseURL == "" {
		fmt.Fprintln(out, "Output widget failed: missing core HTTP base URL")
		return 1
	}

	if err := runOutputWidgetLoop(context.Background(), strings.TrimRight(baseURL, "/"), out, 300*time.Millisecond); err != nil {
		fmt.Fprintf(out, "Output widget failed: %v\n", err)
		return 1
	}
	return 0
}

func runOutputWidgetLoop(ctx context.Context, baseURL string, out io.Writer, refreshInterval time.Duration) error {
	if refreshInterval <= 0 {
		refreshInterval = 300 * time.Millisecond
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	lastRender := ""
	for {
		snapshot, err := fetchOutputWidgetSnapshot(ctx, baseURL)
		if err == nil {
			height, width := resolveWidgetPaneSize()
			render := renderOutputWidget(snapshot, height, width)
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

func fetchOutputWidgetSnapshot(ctx context.Context, baseURL string) (outputWidgetSnapshot, error) {
	ctxSnapshot, err := fetchContextWidgetSnapshot(ctx, baseURL)
	if err != nil {
		return outputWidgetSnapshot{}, err
	}
	return outputWidgetSnapshot{
		SessionID:   ctxSnapshot.SessionID,
		TurnCount:   ctxSnapshot.TurnCount,
		Turns:       ctxSnapshot.Turns,
		PromptCycle: ctxSnapshot.PromptCycle,
	}, nil
}

func renderOutputWidget(snapshot outputWidgetSnapshot, paneHeight int, paneWidth int) string {
	lines := []string{"[OUTPUT]", "Chat ready."}
	for _, turn := range snapshot.Turns {
		prompt := strings.TrimSpace(turn.Prompt)
		response := strings.TrimSpace(turn.Response)
		if prompt == "" && response == "" {
			continue
		}
		classify := classifyPrompt(prompt)
		lines = append(lines,
			"",
			fmt.Sprintf("User: %s", trimSingleLine(prompt, 96)),
			fmt.Sprintf("⚙️ Classification: %s -> %s", classify.Intent, classify.NextStep),
			fmt.Sprintf("Thinking: %s", formatOutputWidgetPhase(snapshot.PromptCycle.Thinking)),
			fmt.Sprintf("💭 [thinking block - %s]", formatOutputWidgetPhase(snapshot.PromptCycle.Thinking)),
			fmt.Sprintf("Response: %s", trimSingleLine(response, 96)),
			fmt.Sprintf("Agent: %s", trimSingleLine(response, 96)),
		)
	}
	if len(snapshot.Turns) == 0 {
		lines = append(lines, "No turns yet.")
	}

	lines = fitLinesToWidth(lines, paneWidth)
	lines = clipLinesForHeight(lines, paneHeight-1)
	return strings.Join(lines, "\n")
}

func formatOutputWidgetPhase(phase PromptCyclePhase) string {
	state := strings.TrimSpace(strings.ToLower(phase.State))
	if state == "" {
		state = "pending"
	}
	return fmt.Sprintf("%s (%s)", state, formatCycleElapsed(phase))
}