package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type outputWidgetSnapshot struct {
	SessionID   string            `json:"session_id"`
	TurnCount   int               `json:"turn_count"`
	Turns       []ChatTurn        `json:"turns"`
	PromptCycle PromptCycleStatus `json:"prompt_cycle"`
}

type outputWidgetViewState struct {
	collapsedThinking map[int]bool
	collapsedEntries  map[string]bool
	focusedTurn       int
	focusedEntry      string
	showHelp          bool
	showClipboard     bool
	clipboard         string
	clipboardSource   string
	statusLine        string
	statusUntil       time.Time
}

func newOutputWidgetViewState() *outputWidgetViewState {
	return &outputWidgetViewState{
		collapsedThinking: make(map[int]bool),
		collapsedEntries:  make(map[string]bool),
		focusedEntry:      "response",
	}
}

func outputEntryAliases() map[string]string {
	return map[string]string{
		"user":           "user",
		"prompt":         "user",
		"classification": "classification",
		"classify":       "classification",
		"thinking":       "thinking",
		"think":          "thinking",
		"response":       "response",
		"agent":          "response",
		"assistant":      "response",
	}
}

func normalizeOutputEntry(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	if normalized, ok := outputEntryAliases()[value]; ok {
		return normalized
	}
	return ""
}

func outputEntryCollapsedKey(turnIndex int, entry string) string {
	return fmt.Sprintf("%d:%s", turnIndex, entry)
}

func (state *outputWidgetViewState) entryCollapsed(turnIndex int, entry string) bool {
	if state == nil {
		return false
	}
	if normalizeOutputEntry(entry) == "thinking" {
		return state.collapsedThinking[turnIndex]
	}
	return state.collapsedEntries[outputEntryCollapsedKey(turnIndex, entry)]
}

func (state *outputWidgetViewState) setEntryCollapsed(turnIndex int, entry string, collapsed bool) {
	if state == nil {
		return
	}
	normalized := normalizeOutputEntry(entry)
	if normalized == "" || normalized == "user" {
		return
	}
	if normalized == "thinking" {
		if collapsed {
			state.collapsedThinking[turnIndex] = true
		} else {
			delete(state.collapsedThinking, turnIndex)
		}
		return
	}
	key := outputEntryCollapsedKey(turnIndex, normalized)
	if collapsed {
		state.collapsedEntries[key] = true
	} else {
		delete(state.collapsedEntries, key)
	}
}

func (state *outputWidgetViewState) setClipboard(source string, value string) {
	if state == nil {
		return
	}
	state.clipboardSource = strings.TrimSpace(source)
	state.clipboard = value
}

func (state *outputWidgetViewState) clipboardSummary() string {
	if state == nil || strings.TrimSpace(state.clipboard) == "" {
		return "empty"
	}
	source := strings.TrimSpace(state.clipboardSource)
	if source == "" {
		source = "selection"
	}
	return fmt.Sprintf("%s (%d chars)", source, len(state.clipboard))
}

func renderOutputWidgetClipboard(snapshot outputWidgetSnapshot, turnIndex int, entry string) (string, bool) {
	if turnIndex < 1 || turnIndex > len(snapshot.Turns) {
		return "", false
	}
	turn := snapshot.Turns[turnIndex-1]
	prompt := strings.TrimSpace(turn.Prompt)
	response := strings.TrimSpace(turn.Response)
	classification := classifyPrompt(prompt)
	thinking := formatOutputWidgetPhase(snapshot.PromptCycle.Thinking)

	switch normalizeOutputEntry(entry) {
	case "user":
		return prompt, true
	case "classification":
		return fmt.Sprintf("%s -> %s", classification.Intent, classification.NextStep), true
	case "thinking":
		return thinking, true
	case "response":
		return response, true
	case "":
		return "", false
	default:
		return "", false
	}
}

func renderOutputWidgetTurnClipboard(snapshot outputWidgetSnapshot, turnIndex int) (string, bool) {
	if turnIndex < 1 || turnIndex > len(snapshot.Turns) {
		return "", false
	}
	turn := snapshot.Turns[turnIndex-1]
	prompt := strings.TrimSpace(turn.Prompt)
	response := strings.TrimSpace(turn.Response)
	classification := classifyPrompt(prompt)
	thinking := formatOutputWidgetPhase(snapshot.PromptCycle.Thinking)

	lines := []string{
		fmt.Sprintf("User: %s", prompt),
		fmt.Sprintf("Classification: %s -> %s", classification.Intent, classification.NextStep),
		fmt.Sprintf("Thinking: %s", thinking),
		fmt.Sprintf("Response: %s", response),
	}
	return strings.Join(lines, "\n"), true
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
		state.focusedEntry = "response"
		state.collapsedThinking = make(map[int]bool)
		state.collapsedEntries = make(map[string]bool)
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
	for key := range state.collapsedEntries {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			delete(state.collapsedEntries, key)
			continue
		}
		turnIndex, err := strconv.Atoi(parts[0])
		if err != nil || turnIndex < 1 || turnIndex > turnCount {
			delete(state.collapsedEntries, key)
			continue
		}
		if normalizeOutputEntry(parts[1]) == "" || normalizeOutputEntry(parts[1]) == "thinking" || normalizeOutputEntry(parts[1]) == "user" {
			delete(state.collapsedEntries, key)
		}
	}
	if normalizeOutputEntry(state.focusedEntry) == "" {
		state.focusedEntry = "response"
	}
}

func (state *outputWidgetViewState) applyCommand(raw string, snapshot outputWidgetSnapshot) {
	if state == nil {
		return
	}
	turnCount := len(snapshot.Turns)
	state.normalize(turnCount)
	line := normalizeOutputWidgetControlCommand(raw)
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
		target := strings.ToLower(strings.TrimSpace(arg))
		if target == "focused" || target == "current" {
			if state.focusedTurn < 1 || state.focusedTurn > turnCount {
				return 0, false
			}
			return state.focusedTurn, true
		}
		value, err := strconv.Atoi(strings.TrimSpace(arg))
		if err != nil || value < 1 || value > turnCount {
			return 0, false
		}
		return value, true
	}

	parseCollapseArgs := func() (entry string, target string, ok bool) {
		if len(args) < 2 {
			return "", "", false
		}
		if len(args) >= 3 {
			first := normalizeOutputEntry(args[1])
			second := normalizeOutputEntry(args[2])
			if first != "" {
				return first, strings.ToLower(strings.TrimSpace(args[2])), true
			}
			if second != "" {
				return second, strings.ToLower(strings.TrimSpace(args[1])), true
			}
		}
		return "thinking", strings.ToLower(strings.TrimSpace(args[1])), true
	}

	setCollapseForAll := func(entry string, collapsed bool) {
		if turnCount == 0 {
			state.setStatus("No turns to update.")
			return
		}
		for i := 1; i <= turnCount; i++ {
			state.setEntryCollapsed(i, entry, collapsed)
		}
		if collapsed {
			state.setStatus(fmt.Sprintf("Collapsed %s entries for all turns.", entry))
		} else {
			state.setStatus(fmt.Sprintf("Expanded %s entries for all turns.", entry))
		}
	}

	setCollapseForTurn := func(entry string, turnIndex int, collapsed bool, toggled bool) {
		state.setEntryCollapsed(turnIndex, entry, collapsed)
		if toggled {
			if state.entryCollapsed(turnIndex, entry) {
				state.setStatus(fmt.Sprintf("Collapsed %s entry for turn %d.", entry, turnIndex))
			} else {
				state.setStatus(fmt.Sprintf("Expanded %s entry for turn %d.", entry, turnIndex))
			}
			return
		}
		if collapsed {
			state.setStatus(fmt.Sprintf("Collapsed %s entry for turn %d.", entry, turnIndex))
		} else {
			state.setStatus(fmt.Sprintf("Expanded %s entry for turn %d.", entry, turnIndex))
		}
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
			state.setStatus(fmt.Sprintf("Usage: :%s <turn|all|entry turn>", cmd))
			return
		}
		if cmd == "focus" {
			if strings.EqualFold(strings.TrimSpace(args[1]), "all") {
				if turnCount == 0 {
					state.setStatus("No turns to focus.")
					return
				}
				state.focusedTurn = turnCount
				state.setStatus(fmt.Sprintf("Focused turn %d.", state.focusedTurn))
				return
			}
			turnIndex, ok := parseTurnArg(args[1])
			if !ok {
				state.setStatus(fmt.Sprintf("Invalid turn number %q.", args[1]))
				return
			}
			state.focusedTurn = turnIndex
			if len(args) >= 3 {
				entry := normalizeOutputEntry(args[2])
				if entry == "" {
					state.setStatus(fmt.Sprintf("Unknown entry %q.", args[2]))
					return
				}
				state.focusedEntry = entry
				state.setStatus(fmt.Sprintf("Focused turn %d %s entry.", turnIndex, entry))
				return
			}
			state.setStatus(fmt.Sprintf("Focused turn %d.", turnIndex))
			return
		}

		entry, target, ok := parseCollapseArgs()
		if !ok {
			state.setStatus(fmt.Sprintf("Usage: :%s <entry> <turn|all>", cmd))
			return
		}
		if entry == "" || entry == "user" {
			state.setStatus("Entry is not collapsible.")
			return
		}

		if target == "all" {
			switch cmd {
			case "collapse":
				setCollapseForAll(entry, true)
			case "expand":
				setCollapseForAll(entry, false)
			case "toggle":
				if turnCount == 0 {
					state.setStatus("No turns to update.")
					return
				}
				for i := 1; i <= turnCount; i++ {
					state.setEntryCollapsed(i, entry, !state.entryCollapsed(i, entry))
				}
				state.setStatus(fmt.Sprintf("Toggled %s entries for all turns.", entry))
			}
			return
		}

		turnIndex, validTurn := parseTurnArg(target)
		if !validTurn {
			state.setStatus(fmt.Sprintf("Invalid turn number %q.", target))
			return
		}

		switch cmd {
		case "collapse":
			setCollapseForTurn(entry, turnIndex, true, false)
		case "expand":
			setCollapseForTurn(entry, turnIndex, false, false)
		case "toggle":
			setCollapseForTurn(entry, turnIndex, !state.entryCollapsed(turnIndex, entry), true)
		}
		return
	case "entry", "focus-entry", "select":
		if len(args) < 2 {
			state.setStatus("Usage: :entry <user|classification|thinking|response>")
			return
		}
		entry := normalizeOutputEntry(args[1])
		if entry == "" {
			state.setStatus(fmt.Sprintf("Unknown entry %q.", args[1]))
			return
		}
		state.focusedEntry = entry
		if state.focusedTurn > 0 {
			state.setStatus(fmt.Sprintf("Focused %s entry on turn %d.", entry, state.focusedTurn))
		} else {
			state.setStatus(fmt.Sprintf("Focused %s entry.", entry))
		}
		return
	case "copy":
		if len(args) < 2 {
			state.setStatus("Usage: :copy <entry|turn|clipboard|show|hide|clear> [turn|focused]")
			return
		}
		sub := strings.ToLower(strings.TrimSpace(args[1]))
		switch sub {
		case "focused", "focus":
			targetTurn := state.focusedTurn
			if targetTurn <= 0 {
				targetTurn = turnCount
			}
			if targetTurn < 1 || targetTurn > turnCount {
				state.setStatus("No turns to copy from.")
				return
			}
			entry := normalizeOutputEntry(state.focusedEntry)
			if entry == "" {
				entry = "response"
			}
			value, ok := renderOutputWidgetClipboard(snapshot, targetTurn, entry)
			if !ok {
				state.setStatus(fmt.Sprintf("Unable to copy focused %s for turn %d.", entry, targetTurn))
				return
			}
			state.setClipboard(fmt.Sprintf("turn %d %s", targetTurn, entry), value)
			state.setStatus(fmt.Sprintf("Copied focused %s for turn %d (%d chars) to output clipboard.", entry, targetTurn, len(value)))
			return
		case "show":
			state.showClipboard = true
			state.setStatus(fmt.Sprintf("Clipboard preview enabled: %s.", state.clipboardSummary()))
			return
		case "hide":
			state.showClipboard = false
			state.setStatus("Clipboard preview hidden.")
			return
		case "clear":
			state.setClipboard("", "")
			state.setStatus("Clipboard cleared.")
			return
		case "clipboard":
			state.setStatus(fmt.Sprintf("Clipboard: %s.", state.clipboardSummary()))
			return
		}

		targetTurn := state.focusedTurn
		if targetTurn <= 0 {
			targetTurn = turnCount
		}
		if len(args) >= 3 {
			parsed, ok := parseTurnArg(args[2])
			if !ok {
				state.setStatus(fmt.Sprintf("Invalid turn number %q.", args[2]))
				return
			}
			targetTurn = parsed
		}
		if targetTurn < 1 || targetTurn > turnCount {
			state.setStatus("No turns to copy from.")
			return
		}

		if strings.ToLower(sub) == "turn" {
			value, ok := renderOutputWidgetTurnClipboard(snapshot, targetTurn)
			if !ok {
				state.setStatus("Unable to copy turn.")
				return
			}
			state.setClipboard(fmt.Sprintf("turn %d", targetTurn), value)
			state.setStatus(fmt.Sprintf("Copied turn %d (%d chars) to output clipboard.", targetTurn, len(value)))
			return
		}

		entry := normalizeOutputEntry(sub)
		if entry == "" {
			state.setStatus(fmt.Sprintf("Unknown copy target %q.", sub))
			return
		}
		value, ok := renderOutputWidgetClipboard(snapshot, targetTurn, entry)
		if !ok {
			state.setStatus(fmt.Sprintf("Unable to copy %s for turn %d.", entry, targetTurn))
			return
		}
		state.setClipboard(fmt.Sprintf("turn %d %s", targetTurn, entry), value)
		state.setStatus(fmt.Sprintf("Copied %s for turn %d (%d chars) to output clipboard.", entry, targetTurn, len(value)))
		return
	default:
		state.setStatus("Unknown output command (use :help).")
	}
}

func normalizeOutputWidgetControlCommand(raw string) string {
	line := strings.TrimSpace(raw)
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, ":") {
		return strings.ToLower(line)
	}

	command := strings.ToLower(line)
	switch command {
	case "?", "help":
		return ":help"
	case "q", "quit", "ctrl_c":
		return ":q"
	case "k", "up", "left", "pgup":
		return ":prev"
	case "j", "down", "right", "pgdn":
		return ":next"
	case "top":
		return ":focus 1"
	case "end":
		return ":focus all"
	default:
		return command
	}
}

func startOutputWidgetCommandReader(ctx context.Context, in io.Reader) <-chan string {
	commands := make(chan string, 16)
	if in == nil {
		close(commands)
		return commands
	}
	commandReader, promptMode, cleanup := newWidgetCommandReader(in, normalizeOutputWidgetCommandToken)

	go func() {
		defer cleanup()
		defer close(commands)
		current := make([]rune, 0, 64)
		emit := func(command string) bool {
			trimmed := strings.TrimSpace(command)
			if trimmed == "" {
				return true
			}
			select {
			case <-ctx.Done():
				return false
			case commands <- trimmed:
				return true
			}
		}

		for {
			cmd, err := commandReader()
			if err != nil {
				return
			}

			if promptMode {
				if !emit(cmd) {
					return
				}
				continue
			}

			switch cmd {
			case "enter":
				if len(current) == 0 {
					continue
				}
				if !emit(string(current)) {
					return
				}
				current = current[:0]
			case "backspace":
				if len(current) > 0 {
					current = current[:len(current)-1]
				}
			case "ctrl_c":
				if !emit("q") {
					return
				}
			case "tab":
				current = append(current, '\t')
			case "space":
				current = append(current, ' ')
			default:
				current = append(current, []rune(cmd)...)
			}
		}
	}()

	return commands
}

func normalizeOutputWidgetCommandToken(raw string) string {
	if raw == "ctrl_c" {
		return "ctrl_c"
	}
	if raw == "backspace" {
		return "backspace"
	}
	if command, ok := normalizeWidgetEscapeSequence(raw); ok {
		return command
	}
	if len(raw) == 1 {
		return strings.ToLower(raw)
	}
	return strings.ToLower(strings.TrimSpace(raw))
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
				normalized := normalizeOutputWidgetControlCommand(cmd)
				action := handleWidgetLoopControlCommand(normalized, widgetLoopControlHandlers{
					QuitTokens: []string{":q", ":quit"},
					HelpTokens: []string{":help"},
					OnHelp: func() {
						viewState.showHelp = true
						viewState.setStatus("Output controls visible.")
					},
				})
				if action == widgetLoopControlQuit {
					return nil
				}
				if action == widgetLoopControlHandled {
					continue
				}
				viewState.applyCommand(normalized, snapshot)
			default:
				goto commandsDrained
			}
		}
	commandsDrained:

		updatedSnapshot, err := fetchOutputWidgetSnapshot(ctx, baseURL)
		if err == nil {
			snapshot = updatedSnapshot
			viewState.normalize(len(snapshot.Turns))
			height, width := resolveWidgetPaneSizeForWriter(out)
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
				"  :focus <n> [entry] | :entry <entry> | :next | :prev",
				"  :collapse [entry] <n|all> | :expand [entry] <n|all> | :toggle [entry] <n|all>",
				"  :copy <entry|turn> [n|focused] | :copy clipboard|show|hide|clear",
				"  :hide-help",
			)
		}
		lines = append(lines, fmt.Sprintf("Clipboard: %s", viewState.clipboardSummary()))
		if viewState.showClipboard && strings.TrimSpace(viewState.clipboard) != "" {
			lines = append(lines,
				"Clipboard payload:",
				viewState.clipboard,
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
		isThinkingCollapsed := false
		isClassificationCollapsed := false
		isResponseCollapsed := false
		entryPrefix := func(turnNumber int, entry string) string {
			if viewState == nil {
				return " "
			}
			if viewState.focusedTurn == turnNumber && viewState.focusedEntry == entry {
				return ">"
			}
			return " "
		}
		if viewState != nil {
			turnNumber := i + 1
			if viewState.focusedTurn == turnNumber {
				prefix = "*"
			}
			isThinkingCollapsed = viewState.entryCollapsed(turnNumber, "thinking")
			isClassificationCollapsed = viewState.entryCollapsed(turnNumber, "classification")
			isResponseCollapsed = viewState.entryCollapsed(turnNumber, "response")
		}
		lines = append(lines,
			"",
			fmt.Sprintf("%s User: %s", prefix, trimSingleLine(prompt, 96)),
		)
		if isClassificationCollapsed {
			lines = append(lines, fmt.Sprintf("%s ⚙️ [classification entry - collapsed]", entryPrefix(i+1, "classification")))
		} else {
			lines = append(lines, fmt.Sprintf("%s ⚙️ Classification: %s -> %s", entryPrefix(i+1, "classification"), classify.Intent, classify.NextStep))
		}
		lines = append(lines,
			fmt.Sprintf("%s Thinking: %s", entryPrefix(i+1, "thinking"), formatOutputWidgetPhase(snapshot.PromptCycle.Thinking)),
		)
		if isThinkingCollapsed {
			lines = append(lines, fmt.Sprintf("%s 💭 [thinking block - collapsed]", entryPrefix(i+1, "thinking")))
		} else {
			lines = append(lines, fmt.Sprintf("%s 💭 [thinking block - %s]", entryPrefix(i+1, "thinking"), formatOutputWidgetPhase(snapshot.PromptCycle.Thinking)))
		}
		if isResponseCollapsed {
			lines = append(lines, fmt.Sprintf("%s 🤖 [response entry - collapsed]", entryPrefix(i+1, "response")))
		} else {
			lines = append(lines,
				fmt.Sprintf("%s Response: %s", entryPrefix(i+1, "response"), trimSingleLine(response, 96)),
				fmt.Sprintf("%s Agent: %s", entryPrefix(i+1, "response"), trimSingleLine(response, 96)),
			)
		}
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
