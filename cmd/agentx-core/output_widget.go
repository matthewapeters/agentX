package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type outputWidgetSnapshot struct {
	SessionID   string           `json:"session_id"`
	TurnCount   int              `json:"turn_count"`
	Turns       []ChatTurn       `json:"turns"`
	PromptCycle PromptCycleStatus `json:"prompt_cycle"`
}

type outputWidgetViewState struct {
	collapsedThinking map[int]bool
	focusedTurn       int
	showHelp          bool
	statusLine        string
	statusUntil       time.Time
}

func newOutputWidgetViewState() *outputWidgetViewState {
	return &outputWidgetViewState{collapsedThinking: make(map[int]bool)}
}

func (state *outputWidgetViewState) thinkingCollapsed(turnIndex int) bool {
	if state == nil {
		return false
	}
	return state.collapsedThinking[turnIndex]
}

func (state *outputWidgetViewState) setStatus(status string) {
	if state == nil {
		return
	}
	state.statusLine = strings.TrimSpace(status)
	state.statusUntil = time.Now().Add(8 * time.Second)
}

func (state *outputWidgetViewState) activeStatus() string {
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

func (state *outputWidgetViewState) normalize(turnCount int) {
	if state == nil {
		return
	}
	if turnCount <= 0 {
		state.focusedTurn = 0
		state.collapsedThinking = make(map[int]bool)
		return
	}
	if state.focusedTurn <= 0 {
		state.focusedTurn = turnCount
	}
	if state.focusedTurn > turnCount {
		state.focusedTurn = turnCount
	}
	for idx := range state.collapsedThinking {
		if idx < 1 || idx > turnCount {
			delete(state.collapsedThinking, idx)
		}
	}
}

func (state *outputWidgetViewState) applyCommand(raw string, turnCount int) {
	if state == nil {
		return
	}
	state.normalize(turnCount)
	line := strings.TrimSpace(raw)
	if line == "" {
		return
	}
	if !strings.HasPrefix(line, ":") {
		state.setStatus("Output widget controls must start with ':' (use :help).")
		return
	}

	args := strings.Fields(strings.TrimPrefix(line, ":"))
	if len(args) == 0 {
		return
	}
	cmd := strings.ToLower(args[0])

	parseTurnArg := func(arg string) (int, bool) {
		value, err := strconv.Atoi(strings.TrimSpace(arg))
		if err != nil || value < 1 || value > turnCount {
			return 0, false
		}
		return value, true
	}

	switch cmd {
	case "help", "controls":
		state.showHelp = true
		state.setStatus("Output controls visible.")
		return
	case "hide-help":
		state.showHelp = false
		state.setStatus("Output controls hidden.")
		return
	case "next":
		if turnCount == 0 {
			state.setStatus("No turns to focus.")
			return
		}
		state.focusedTurn++
		if state.focusedTurn > turnCount {
			state.focusedTurn = 1
		}
		state.setStatus(fmt.Sprintf("Focused turn %d.", state.focusedTurn))
		return
	case "prev":
		if turnCount == 0 {
			state.setStatus("No turns to focus.")
			return
		}
		state.focusedTurn--
		if state.focusedTurn < 1 {
			state.focusedTurn = turnCount
		}
		state.setStatus(fmt.Sprintf("Focused turn %d.", state.focusedTurn))
		return
	case "focus", "toggle", "collapse", "expand":
		if len(args) < 2 {
			state.setStatus(fmt.Sprintf("Usage: :%s <turn-number|all>", cmd))
			return
		}
		target := strings.ToLower(strings.TrimSpace(args[1]))
		if target == "all" {
			if turnCount == 0 {
				state.setStatus("No turns to update.")
				return
			}
			switch cmd {
			case "collapse":
				for i := 1; i <= turnCount; i++ {
					state.collapsedThinking[i] = true
				}
				state.setStatus("Collapsed thinking blocks for all turns.")
			case "expand":
				for i := 1; i <= turnCount; i++ {
					delete(state.collapsedThinking, i)
				}
				state.setStatus("Expanded thinking blocks for all turns.")
			case "toggle":
				for i := 1; i <= turnCount; i++ {
					state.collapsedThinking[i] = !state.collapsedThinking[i]
				}
				state.setStatus("Toggled thinking blocks for all turns.")
			case "focus":
				state.focusedTurn = turnCount
				state.setStatus(fmt.Sprintf("Focused turn %d.", state.focusedTurn))
			}
			return
		}

		turnIndex, ok := parseTurnArg(target)
		if !ok {
			state.setStatus(fmt.Sprintf("Invalid turn number %q.", target))
			return
		}

		switch cmd {
		case "focus":
			state.focusedTurn = turnIndex
			state.setStatus(fmt.Sprintf("Focused turn %d.", turnIndex))
		case "collapse":
			state.collapsedThinking[turnIndex] = true
			state.setStatus(fmt.Sprintf("Collapsed thinking block for turn %d.", turnIndex))
		case "expand":
			delete(state.collapsedThinking, turnIndex)
			state.setStatus(fmt.Sprintf("Expanded thinking block for turn %d.", turnIndex))
		case "toggle":
			state.collapsedThinking[turnIndex] = !state.collapsedThinking[turnIndex]
			if state.collapsedThinking[turnIndex] {
				state.setStatus(fmt.Sprintf("Collapsed thinking block for turn %d.", turnIndex))
			} else {
				state.setStatus(fmt.Sprintf("Expanded thinking block for turn %d.", turnIndex))
			}
		}
		return
	default:
		state.setStatus("Unknown output command (use :help).")
	}
}

func startOutputWidgetCommandReader(ctx context.Context, in io.Reader) <-chan string {
	commands := make(chan string, 16)
	if in == nil {
		close(commands)
		return commands
	}

	go func() {
		defer close(commands)
		scanner := bufio.NewScanner(in)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case commands <- scanner.Text():
			}
		}
	}()

	return commands
}

func runOutputWidgetCommand(coreHTTP string, in io.Reader, out io.Writer) int {
	baseURL := strings.TrimSpace(coreHTTP)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("AGENTX_CORE_HTTP"))
	}
	if baseURL == "" {
		fmt.Fprintln(out, "Output widget failed: missing core HTTP base URL")
		return 1
	}

	if err := runOutputWidgetLoopWithInput(context.Background(), strings.TrimRight(baseURL, "/"), in, out, 300*time.Millisecond); err != nil {
		fmt.Fprintf(out, "Output widget failed: %v\n", err)
		return 1
	}
	return 0
}

func runOutputWidgetLoop(ctx context.Context, baseURL string, out io.Writer, refreshInterval time.Duration) error {
	return runOutputWidgetLoopWithInput(ctx, baseURL, nil, out, refreshInterval)
}

func runOutputWidgetLoopWithInput(ctx context.Context, baseURL string, in io.Reader, out io.Writer, refreshInterval time.Duration) error {
	if refreshInterval <= 0 {
		refreshInterval = 300 * time.Millisecond
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	commands := startOutputWidgetCommandReader(ctx, in)
	viewState := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{}

	lastRender := ""
	for {
		for {
			select {
			case cmd, ok := <-commands:
				if !ok {
					commands = nil
					break
				}
				viewState.applyCommand(cmd, len(snapshot.Turns))
			default:
				goto commandsDrained
			}
		}
	commandsDrained:

		updatedSnapshot, err := fetchOutputWidgetSnapshot(ctx, baseURL)
		if err == nil {
			snapshot = updatedSnapshot
			viewState.normalize(len(snapshot.Turns))
			height, width := resolveWidgetPaneSize()
			render := renderOutputWidgetWithViewState(snapshot, height, width, viewState)
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
	return renderOutputWidgetWithViewState(snapshot, paneHeight, paneWidth, nil)
}

func renderOutputWidgetWithViewState(snapshot outputWidgetSnapshot, paneHeight int, paneWidth int, viewState *outputWidgetViewState) string {
	if viewState != nil {
		viewState.normalize(len(snapshot.Turns))
	}

	lines := []string{"[OUTPUT]", "Chat ready."}
	if viewState != nil {
		lines = append(lines, "Controls: :help for output affordances.")
		if status := viewState.activeStatus(); status != "" {
			lines = append(lines, fmt.Sprintf("Status: %s", status))
		}
		if viewState.showHelp {
			lines = append(lines,
				"Output commands:",
				"  :focus <n> | :next | :prev",
				"  :collapse <n|all> | :expand <n|all> | :toggle <n|all>",
				"  :hide-help",
			)
		}
	}
	for i := range snapshot.Turns {
		turn := snapshot.Turns[i]
		prompt := strings.TrimSpace(turn.Prompt)
		response := strings.TrimSpace(turn.Response)
		if prompt == "" && response == "" {
			continue
		}
		classify := classifyPrompt(prompt)
		prefix := " "
		isCollapsed := false
		if viewState != nil {
			turnNumber := i + 1
			if viewState.focusedTurn == turnNumber {
				prefix = ">"
			}
			isCollapsed = viewState.thinkingCollapsed(turnNumber)
		}
		lines = append(lines,
			"",
			fmt.Sprintf("%s User: %s", prefix, trimSingleLine(prompt, 96)),
			fmt.Sprintf("⚙️ Classification: %s -> %s", classify.Intent, classify.NextStep),
			fmt.Sprintf("Thinking: %s", formatOutputWidgetPhase(snapshot.PromptCycle.Thinking)),
		)
		if isCollapsed {
			lines = append(lines, "💭 [thinking block - collapsed]")
		} else {
			lines = append(lines, fmt.Sprintf("💭 [thinking block - %s]", formatOutputWidgetPhase(snapshot.PromptCycle.Thinking)))
		}
		lines = append(lines,
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