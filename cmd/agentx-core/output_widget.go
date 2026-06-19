package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	appstate "github.com/matthewapeters/agentX/cmd/agentx-core/internal/state"
)

type outputWidgetSnapshot struct {
	SessionID       string            `json:"session_id"`
	TurnCount       int               `json:"turn_count"`
	Turns           []ChatTurn        `json:"turns"`
	ThinkingContent string            `json:"thinking_content,omitempty"`
	PromptCycle     PromptCycleStatus `json:"prompt_cycle"`
}

type outputWidgetViewState struct {
	turnExpanded      map[int]bool
	turnScroll        map[int]int
	collapsedThinking map[int]bool
	collapsedEntries  map[string]bool
	sessionID         string
	focusedTurn       int
	focusedEntry      string
	entryFocusMode    bool
	showHelp          bool
	showClipboard     bool
	clipboard         string
	clipboardSource   string
	statusLine        string
	statusUntil       time.Time
	lastTurnCount     int
	lastLatestSession string
	lastLatestTurn    int
	lastLatestReply   string
	lastPaneHeight    int
}

const outputWidgetPageReserveRows = 4

func outputWidgetEntryOrder() []string {
	return []string{"classification", "thinking", "response"}
}

func (state *outputWidgetViewState) normalizedFocusedEntry() string {
	if state == nil {
		return "response"
	}
	entry := normalizeOutputEntry(state.focusedEntry)
	if entry == "" || entry == "user" {
		return "response"
	}
	return entry
}

func outputWidgetEntryIndex(entry string) int {
	normalized := normalizeOutputEntry(entry)
	for idx, candidate := range outputWidgetEntryOrder() {
		if candidate == normalized {
			return idx
		}
	}
	return 0
}

func outputWidgetCycleEntry(entry string, step int) string {
	entries := outputWidgetEntryOrder()
	if len(entries) == 0 {
		return "response"
	}
	index := outputWidgetEntryIndex(entry)
	index = (index + step) % len(entries)
	if index < 0 {
		index += len(entries)
	}
	return entries[index]
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (state *outputWidgetViewState) pageStep() int {
	if state == nil {
		return 1
	}
	rows := state.lastPaneHeight - outputWidgetPageReserveRows
	if rows < 1 {
		rows = 1
	}
	return rows
}

func newOutputWidgetViewState() *outputWidgetViewState {
	return &outputWidgetViewState{
		turnExpanded:      make(map[int]bool),
		turnScroll:        make(map[int]int),
		collapsedThinking: make(map[int]bool),
		collapsedEntries:  make(map[string]bool),
		focusedEntry:      "response",
		entryFocusMode:    false,
	}
}

func (state *outputWidgetViewState) exportPersistedState() *appstate.OutputAppletState {
	persisted := appstate.NewOutputAppletState()
	if state == nil {
		return persisted
	}

	for turnIndex, expanded := range state.turnExpanded {
		if turnIndex < 1 {
			continue
		}
		if !expanded {
			persisted.CollapsedTurns[turnIndex] = true
		}
	}

	if state.focusedTurn > 0 {
		persisted.FocusedTurnIdx = state.focusedTurn
	}

	if entry := state.normalizedFocusedEntry(); entry != "" {
		persisted.EntryFocusPath = []string{entry}
	}

	return persisted
}

func (state *outputWidgetViewState) applyPersistedState(persisted *appstate.OutputAppletState, turnCount int) {
	if state == nil || persisted == nil {
		return
	}

	for turnIndex, collapsed := range persisted.CollapsedTurns {
		if !collapsed || turnIndex < 1 {
			continue
		}
		if turnCount > 0 && turnIndex > turnCount {
			continue
		}
		state.setTurnExpanded(turnIndex, false)
	}

	if persisted.FocusedTurnIdx > 0 {
		state.focusedTurn = persisted.FocusedTurnIdx
	}

	if len(persisted.EntryFocusPath) > 0 {
		if entry := normalizeOutputEntry(persisted.EntryFocusPath[0]); entry != "" {
			state.focusedEntry = entry
		}
	}

	state.normalize(state.sessionID, turnCount)
}

func outputWidgetPersistedStateEqual(left *appstate.OutputAppletState, right *appstate.OutputAppletState) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	if left.FocusedTurnIdx != right.FocusedTurnIdx {
		return false
	}
	if len(left.EntryFocusPath) != len(right.EntryFocusPath) {
		return false
	}
	for idx := range left.EntryFocusPath {
		if left.EntryFocusPath[idx] != right.EntryFocusPath[idx] {
			return false
		}
	}
	if len(left.CollapsedTurns) != len(right.CollapsedTurns) {
		return false
	}
	for key, value := range left.CollapsedTurns {
		if right.CollapsedTurns[key] != value {
			return false
		}
	}
	return true
}

func resolveOutputWidgetAppletStateDir(sessionID string) string {
	projectDir := strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR"))
	username := strings.TrimSpace(os.Getenv("AGENTX_USERNAME"))
	trimmedSessionID := strings.TrimSpace(sessionID)
	if projectDir == "" || username == "" || trimmedSessionID == "" {
		return ""
	}
	cfg := &Config{ProjectDir: projectDir, Username: username}
	return cfg.AppletStateDir(trimmedSessionID)
}

func saveOutputWidgetViewStateIfChanged(viewState *outputWidgetViewState, sessionID string, appletStateDir string, lastPersisted *appstate.OutputAppletState) *appstate.OutputAppletState {
	trimmedSessionID := strings.TrimSpace(sessionID)
	trimmedStateDir := strings.TrimSpace(appletStateDir)
	if viewState == nil || trimmedSessionID == "" || trimmedStateDir == "" {
		return lastPersisted
	}

	persisted := viewState.exportPersistedState()
	if outputWidgetPersistedStateEqual(lastPersisted, persisted) {
		return lastPersisted
	}

	if err := appstate.SaveOutputAppletState(trimmedSessionID, trimmedStateDir, persisted); err != nil {
		log.Printf("[output_widget] Warning: failed to save state: %v", err)
		return lastPersisted
	}

	return persisted
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

func (state *outputWidgetViewState) turnHasExplicitState(turnIndex int) (bool, bool) {
	if state == nil {
		return false, false
	}
	value, ok := state.turnExpanded[turnIndex]
	return value, ok
}

func (state *outputWidgetViewState) turnExpandedState(turnIndex int) bool {
	if state == nil {
		return true
	}
	if explicit, ok := state.turnHasExplicitState(turnIndex); ok {
		return explicit
	}
	return turnIndex == state.focusedTurn
}

func (state *outputWidgetViewState) setTurnExpanded(turnIndex int, expanded bool) {
	if state == nil || turnIndex < 1 {
		return
	}
	state.turnExpanded[turnIndex] = expanded
}

func (state *outputWidgetViewState) clearTurnExpanded(turnIndex int) {
	if state == nil || turnIndex < 1 {
		return
	}
	delete(state.turnExpanded, turnIndex)
}

func (state *outputWidgetViewState) turnScrollOffset(turnIndex int) int {
	if state == nil {
		return 0
	}
	if offset := state.turnScroll[turnIndex]; offset > 0 {
		return offset
	}
	return 0
}

func (state *outputWidgetViewState) setTurnScrollOffset(turnIndex int, offset int) {
	if state == nil || turnIndex < 1 {
		return
	}
	if offset < 0 {
		offset = 0
	}
	state.turnScroll[turnIndex] = offset
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
	thinking := renderOutputWidgetThinking(snapshot)

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
	thinking := renderOutputWidgetThinking(snapshot)

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

func (state *outputWidgetViewState) resetTurnViewState() {
	if state == nil {
		return
	}
	state.focusedTurn = 0
	state.turnExpanded = make(map[int]bool)
	state.turnScroll = make(map[int]int)
	state.collapsedThinking = make(map[int]bool)
	state.collapsedEntries = make(map[string]bool)
	state.lastTurnCount = 0
	state.lastLatestTurn = 0
	state.lastLatestReply = ""
}

func (state *outputWidgetViewState) normalize(sessionID string, turnCount int) {
	if state == nil {
		return
	}
	normalizedSessionID := strings.TrimSpace(sessionID)
	if normalizedSessionID != state.sessionID {
		state.sessionID = normalizedSessionID
		state.lastLatestSession = normalizedSessionID
		state.resetTurnViewState()
	}
	if turnCount <= 0 {
		state.resetTurnViewState()
		state.focusedEntry = "response"
		state.entryFocusMode = false
		return
	}
	if turnCount > state.lastTurnCount {
		state.focusedTurn = turnCount
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
	for idx := range state.turnExpanded {
		if idx < 1 || idx > turnCount {
			delete(state.turnExpanded, idx)
		}
	}
	for idx := range state.turnScroll {
		if idx < 1 || idx > turnCount {
			delete(state.turnScroll, idx)
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
	state.lastTurnCount = turnCount
}

func (state *outputWidgetViewState) maybeExpandLatestTurn(snapshot outputWidgetSnapshot) {
	if state == nil {
		return
	}
	sessionID := strings.TrimSpace(snapshot.SessionID)
	if sessionID != state.lastLatestSession {
		state.lastLatestSession = sessionID
		state.lastLatestTurn = 0
		state.lastLatestReply = ""
	}
	turnCount := len(snapshot.Turns)
	if turnCount <= 0 {
		state.lastLatestTurn = 0
		state.lastLatestReply = ""
		return
	}
	latestReply := strings.TrimSpace(snapshot.Turns[turnCount-1].Response)
	if state.lastLatestTurn == 0 && state.lastLatestReply == "" {
		state.lastLatestTurn = turnCount
		state.lastLatestReply = latestReply
		return
	}
	changed := turnCount != state.lastLatestTurn || latestReply != state.lastLatestReply
	if changed {
		state.focusedTurn = turnCount
		state.setTurnExpanded(turnCount, true)
		state.setTurnScrollOffset(turnCount, 0)
		state.setEntryCollapsed(turnCount, "response", false)
	}
	state.lastLatestTurn = turnCount
	state.lastLatestReply = latestReply
}

func (state *outputWidgetViewState) applyCommand(raw string, snapshot outputWidgetSnapshot) {
	if state == nil {
		return
	}
	turnCount := len(snapshot.Turns)
	state.normalize(snapshot.SessionID, turnCount)
	line := normalizeOutputWidgetControlCommand(raw)
	if line == "" {
		return
	}
	if !strings.HasPrefix(line, ":") {
		state.setStatus("Output widget controls must start with ':' (use :help).")
		return
	}
	if state.showHelp && line != ":help" {
		state.showHelp = false
		state.setStatus("Output controls hidden.")
		return
	}

	args := strings.Fields(strings.TrimPrefix(line, ":"))
	if len(args) == 0 {
		return
	}
	cmd := strings.ToLower(args[0])

	parseTurnArg := func(arg string) (int, bool) {
		target := strings.ToLower(strings.TrimSpace(arg))
		switch target {
		case "focused", "current":
			if state.focusedTurn < 1 || state.focusedTurn > turnCount {
				return 0, false
			}
			return state.focusedTurn, true
		case "oldest", "first", "top":
			if turnCount < 1 {
				return 0, false
			}
			return 1, true
		case "newest", "last", "all", "end":
			if turnCount < 1 {
				return 0, false
			}
			return turnCount, true
		}
		value, err := strconv.Atoi(strings.TrimSpace(arg))
		if err != nil || value < 1 || value > turnCount {
			return 0, false
		}
		return value, true
	}

	toggleFocusedEntry := func(targetTurn int) {
		entry := state.normalizedFocusedEntry()
		collapsed := state.entryCollapsed(targetTurn, entry)
		state.setEntryCollapsed(targetTurn, entry, !collapsed)
		state.setTurnExpanded(targetTurn, true)
		if !collapsed {
			state.setStatus(fmt.Sprintf("Collapsed %s entry on turn %d.", entry, targetTurn))
		} else {
			state.setStatus(fmt.Sprintf("Expanded %s entry on turn %d.", entry, targetTurn))
		}
	}

	setFocusedEntryCollapsed := func(targetTurn int, collapsed bool) {
		entry := state.normalizedFocusedEntry()
		state.setEntryCollapsed(targetTurn, entry, collapsed)
		state.setTurnExpanded(targetTurn, true)
		if collapsed {
			state.setStatus(fmt.Sprintf("Collapsed %s entry on turn %d.", entry, targetTurn))
		} else {
			state.setStatus(fmt.Sprintf("Expanded %s entry on turn %d.", entry, targetTurn))
		}
	}

	switch cmd {
	case "help", "controls":
		state.showHelp = !state.showHelp
		if state.showHelp {
			state.setStatus("Output controls visible.")
		} else {
			state.setStatus("Output controls hidden.")
		}
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
		if state.entryFocusMode {
			state.focusedEntry = outputWidgetCycleEntry(state.focusedEntry, 1)
			state.setStatus(fmt.Sprintf("Focused %s entry on turn %d.", state.focusedEntry, state.focusedTurn))
			return
		}
		if state.focusedTurn < turnCount {
			state.focusedTurn++
		}
		state.setStatus(fmt.Sprintf("Focused turn %d.", state.focusedTurn))
		return
	case "prev":
		if turnCount == 0 {
			state.setStatus("No turns to focus.")
			return
		}
		if state.entryFocusMode {
			state.focusedEntry = outputWidgetCycleEntry(state.focusedEntry, -1)
			state.setStatus(fmt.Sprintf("Focused %s entry on turn %d.", state.focusedEntry, state.focusedTurn))
			return
		}
		if state.focusedTurn > 1 {
			state.focusedTurn--
		}
		state.setStatus(fmt.Sprintf("Focused turn %d.", state.focusedTurn))
		return
	case "drill-in":
		if turnCount == 0 {
			state.setStatus("No turns to focus.")
			return
		}
		state.entryFocusMode = true
		state.focusedEntry = state.normalizedFocusedEntry()
		state.setStatus(fmt.Sprintf("Focused %s entry on turn %d.", state.focusedEntry, state.focusedTurn))
		return
	case "drill-out":
		if turnCount == 0 {
			state.setStatus("No turns to focus.")
			return
		}
		state.entryFocusMode = false
		state.setStatus(fmt.Sprintf("Focused turn %d container.", state.focusedTurn))
		return
	case "entry-toggle", "entry-collapse", "entry-expand":
		targetTurn := state.focusedTurn
		if targetTurn < 1 || targetTurn > turnCount {
			state.setStatus("No turns to update.")
			return
		}
		switch cmd {
		case "entry-toggle":
			toggleFocusedEntry(targetTurn)
		case "entry-collapse":
			setFocusedEntryCollapsed(targetTurn, true)
		case "entry-expand":
			setFocusedEntryCollapsed(targetTurn, false)
		}
		return
	case "focus":
		if turnCount == 0 {
			state.setStatus("No turns to focus.")
			return
		}
		if len(args) < 2 {
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
		}
		state.setStatus(fmt.Sprintf("Focused turn %d.", state.focusedTurn))
		return
	case "toggle", "collapse", "expand":
		targetTurn := state.focusedTurn
		if len(args) >= 2 {
			if parsed, ok := parseTurnArg(args[1]); ok {
				targetTurn = parsed
			}
		}
		if targetTurn < 1 || targetTurn > turnCount {
			state.setStatus("No turns to update.")
			return
		}
		switch cmd {
		case "toggle":
			state.setTurnExpanded(targetTurn, !state.turnExpandedState(targetTurn))
		case "collapse":
			state.setTurnExpanded(targetTurn, false)
		case "expand":
			state.setTurnExpanded(targetTurn, true)
		}
		if targetTurn == state.focusedTurn {
			state.setTurnScrollOffset(targetTurn, 0)
		}
		if state.turnExpandedState(targetTurn) {
			state.setStatus(fmt.Sprintf("Expanded turn %d.", targetTurn))
		} else {
			state.setStatus(fmt.Sprintf("Collapsed turn %d.", targetTurn))
		}
		return
	case "target-toggle":
		targetTurn := state.focusedTurn
		if targetTurn < 1 || targetTurn > turnCount {
			state.setStatus("No turns to update.")
			return
		}
		if state.entryFocusMode {
			toggleFocusedEntry(targetTurn)
			return
		}
		state.setTurnExpanded(targetTurn, !state.turnExpandedState(targetTurn))
		state.setTurnScrollOffset(targetTurn, 0)
		if state.turnExpandedState(targetTurn) {
			state.setStatus(fmt.Sprintf("Expanded turn %d.", targetTurn))
		} else {
			state.setStatus(fmt.Sprintf("Collapsed turn %d.", targetTurn))
		}
		return
	case "target-collapse":
		targetTurn := state.focusedTurn
		if targetTurn < 1 || targetTurn > turnCount {
			state.setStatus("No turns to update.")
			return
		}
		if state.entryFocusMode {
			setFocusedEntryCollapsed(targetTurn, true)
			return
		}
		state.setTurnExpanded(targetTurn, false)
		state.setTurnScrollOffset(targetTurn, 0)
		state.setStatus(fmt.Sprintf("Collapsed turn %d.", targetTurn))
		return
	case "target-expand":
		targetTurn := state.focusedTurn
		if targetTurn < 1 || targetTurn > turnCount {
			state.setStatus("No turns to update.")
			return
		}
		if state.entryFocusMode {
			setFocusedEntryCollapsed(targetTurn, false)
			return
		}
		state.setTurnExpanded(targetTurn, true)
		state.setTurnScrollOffset(targetTurn, 0)
		state.setStatus(fmt.Sprintf("Expanded turn %d.", targetTurn))
		return
	case "pageup", "pagedown":
		targetTurn := state.focusedTurn
		if targetTurn < 1 || targetTurn > turnCount {
			state.setStatus("No focused turn to scroll.")
			return
		}
		delta := state.pageStep()
		if cmd == "pageup" {
			delta = -delta
		}
		state.setTurnScrollOffset(targetTurn, state.turnScrollOffset(targetTurn)+delta)
		state.setStatus(fmt.Sprintf("Adjusted %s scroll for turn %d by %d rows.", state.focusedEntry, targetTurn, absInt(delta)))
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
		state.entryFocusMode = true
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
			entry := state.normalizedFocusedEntry()
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
	case "k", "up":
		return ":prev"
	case "j", "down":
		return ":next"
	case "h":
		return ":target-collapse"
	case "l":
		return ":target-expand"
	case "left":
		return ":prev"
	case "right":
		return ":next"
	case "enter", "space":
		return ":target-toggle"
	case "tab":
		return ":drill-in"
	case "shift-tab":
		return ":drill-out"
	case "pgup":
		return ":pageup"
	case "pgdn":
		return ":pagedown"
	case "top", "home":
		return ":focus 1"
	case "end":
		return ":focus all"
	default:
		return command
	}
}

func isOutputWidgetImmediateToken(token string) bool {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "?", "q", "ctrl_c", "k", "j", "h", "l", "up", "down", "left", "right", "home", "end", "enter", "space", "tab", "shift-tab", "pgup", "pgdn":
		return true
	default:
		return false
	}
}

func outputWidgetRawTokenStep(current []rune, token string) ([]rune, []string) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return current, nil
	}

	emit := func(command string) ([]rune, []string) {
		command = strings.TrimSpace(command)
		if command == "" {
			return current, nil
		}
		return current, []string{command}
	}

	inColonMode := len(current) > 0 && current[0] == ':'
	if inColonMode {
		switch trimmed {
		case "enter":
			line := strings.TrimSpace(string(current))
			return current[:0], []string{line}
		case "backspace":
			if len(current) > 0 {
				return current[:len(current)-1], nil
			}
			return current, nil
		case "ctrl_c":
			return current[:0], []string{"q"}
		case "space":
			return append(current, ' '), nil
		case "tab":
			return append(current, '\t'), nil
		}
		if len(trimmed) == 1 {
			return append(current, []rune(trimmed)...), nil
		}
		if isOutputWidgetImmediateToken(trimmed) {
			return current[:0], []string{trimmed}
		}
		return append(current, []rune(trimmed)...), nil
	}

	switch trimmed {
	case ":":
		return []rune{':'}, nil
	case "backspace":
		return current, nil
	case "ctrl_c":
		return emit("q")
	}
	if isOutputWidgetImmediateToken(trimmed) {
		return emit(trimmed)
	}
	if strings.HasPrefix(trimmed, ":") {
		return emit(trimmed)
	}
	return emit(trimmed)
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

			next, outgoing := outputWidgetRawTokenStep(current, cmd)
			current = next
			for _, command := range outgoing {
				if !emit(command) {
					return
				}
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
	ctx, cancel := widgetCommandContext()
	defer cancel()
	stopWatchdog := startWidgetCoreWatchdog(resolveWidgetCorePIDFromEnv(), 500*time.Millisecond, os.Stderr, cancel)
	defer stopWatchdog()

	if err := runOutputWidgetLoopWithInput(ctx, strings.TrimRight(baseURL, "/"), in, out, 300*time.Millisecond); err != nil {
		fmt.Fprintf(out, "Output widget failed: %v\n", err)
		return 1
	}
	return 0
}

func runOutputWidgetLoop(ctx context.Context, baseURL string, out io.Writer, refreshInterval time.Duration) error {
	return runOutputWidgetLoopWithInput(ctx, baseURL, nil, out, refreshInterval)
}

func runOutputWidgetLoopWithInput(ctx context.Context, baseURL string, in io.Reader, out io.Writer, refreshInterval time.Duration) error {
	hideTerminalCursor(out)
	defer showTerminalCursor(out)

	if refreshInterval <= 0 {
		refreshInterval = 300 * time.Millisecond
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	commands := startOutputWidgetCommandReader(ctx, in)
	viewState := newOutputWidgetViewState()
	snapshot := outputWidgetSnapshot{}
	lastPersistedState := (*appstate.OutputAppletState)(nil)
	loadedPersistedSession := ""
	startupHeight, startupWidth := resolveWidgetPaneSizeAtStartup(out)
	firstRender := true

	previousLines := []string(nil)
	for {
		for {
			select {
			case cmd, ok := <-commands:
				if !ok {
					commands = nil
					break
				}
				normalized := normalizeOutputWidgetControlCommand(cmd)
				if viewState.showHelp && normalized != ":help" {
					viewState.showHelp = false
					viewState.setStatus("Output controls hidden.")
					continue
				}
				action := handleWidgetLoopControlCommand(normalized, widgetLoopControlHandlers{
					QuitTokens: []string{":q", ":quit"},
					HelpTokens: []string{":help"},
					OnHelp: func() {
						viewState.showHelp = !viewState.showHelp
						if viewState.showHelp {
							viewState.setStatus("Output controls visible.")
						} else {
							viewState.setStatus("Output controls hidden.")
						}
					},
				})
				if action == widgetLoopControlQuit {
					return nil
				}
				if action == widgetLoopControlHandled {
					continue
				}
				viewState.applyCommand(normalized, snapshot)
				if sessionID := strings.TrimSpace(snapshot.SessionID); sessionID != "" {
					lastPersistedState = saveOutputWidgetViewStateIfChanged(viewState, sessionID, resolveOutputWidgetAppletStateDir(sessionID), lastPersistedState)
				}
			default:
				goto commandsDrained
			}
		}
	commandsDrained:

		updatedSnapshot, err := fetchOutputWidgetSnapshot(ctx, baseURL)
		if err == nil {
			snapshot = updatedSnapshot
			viewState.normalize(snapshot.SessionID, len(snapshot.Turns))
			if sessionID := strings.TrimSpace(snapshot.SessionID); sessionID != "" && sessionID != loadedPersistedSession {
				loadedPersistedSession = sessionID
				lastPersistedState = nil
				if appletStateDir := resolveOutputWidgetAppletStateDir(sessionID); appletStateDir != "" {
					persistedState, loadErr := appstate.LoadOutputAppletState(sessionID, appletStateDir)
					if loadErr != nil {
						log.Printf("[output_widget] Warning: failed to load state: %v", loadErr)
					} else {
						viewState.applyPersistedState(persistedState, len(snapshot.Turns))
						lastPersistedState = viewState.exportPersistedState()
					}
				}
			}
			height, width := resolveWidgetPaneSizeForWriter(out)
			if firstRender {
				height, width = startupHeight, startupWidth
				firstRender = false
			}
			render := renderOutputWidgetWithViewState(snapshot, height, width, viewState)
			currentLines := filesystemWidgetFrameLines(render)
			if len(previousLines) == 0 || strings.Join(previousLines, "\n") != strings.Join(currentLines, "\n") {
				if err := writeFilesystemWidgetFrameDiff(out, previousLines, currentLines); err != nil {
					return err
				}
				previousLines = currentLines
			}
			if sessionID := strings.TrimSpace(snapshot.SessionID); sessionID != "" {
				lastPersistedState = saveOutputWidgetViewStateIfChanged(viewState, sessionID, resolveOutputWidgetAppletStateDir(sessionID), lastPersistedState)
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
		SessionID:       ctxSnapshot.SessionID,
		TurnCount:       ctxSnapshot.TurnCount,
		Turns:           ctxSnapshot.Turns,
		ThinkingContent: strings.TrimSpace(ctxSnapshot.ThinkingContent),
		PromptCycle:     ctxSnapshot.PromptCycle,
	}, nil
}

func renderOutputWidgetTurnSummary(turn ChatTurn, focused bool) string {
	prompt := strings.TrimSpace(turn.Prompt)
	summary := prompt
	role := "user"
	if summary == "" {
		summary = strings.TrimSpace(turn.Response)
		role = "assistant"
	}
	if summary == "" {
		summary = "none"
	}
	pointer := " "
	if focused {
		pointer = "↳"
	}
	return fmt.Sprintf("%s [+] [%s] %s", pointer, role, trimSingleLine(summary, 60))
}

func renderOutputWidgetCollapsedPreview(content string, limit int) string {
	if limit <= 0 {
		return "none"
	}
	singleLine := strings.Join(strings.Fields(content), " ")
	if singleLine == "" {
		return "none"
	}
	if renderStringWidth(singleLine) <= limit {
		return singleLine
	}
	if limit <= 3 {
		return strings.Repeat(".", limit)
	}

	wordBudget := limit - 3
	words := strings.Fields(singleLine)
	builder := strings.Builder{}
	for _, word := range words {
		candidate := word
		if builder.Len() > 0 {
			candidate = builder.String() + " " + word
		}
		if renderStringWidth(candidate) > wordBudget {
			break
		}
		builder.Reset()
		builder.WriteString(candidate)
	}

	prefix := strings.TrimSpace(builder.String())
	if prefix == "" {
		runes := []rune(singleLine)
		clipped := make([]rune, 0, len(runes))
		for _, r := range runes {
			next := append(clipped, r)
			if renderStringWidth(string(next)) > wordBudget {
				break
			}
			clipped = next
		}
		prefix = strings.TrimSpace(string(clipped))
		if prefix == "" {
			return "..."
		}
	}

	return prefix + " ..."
}

func renderOutputWidgetThinking(snapshot outputWidgetSnapshot) string {
	phase := formatOutputWidgetPhase(snapshot.PromptCycle.Thinking)
	content := strings.TrimSpace(snapshot.ThinkingContent)
	if content == "" {
		return phase
	}
	return fmt.Sprintf("%s | %s", phase, content)
}

func outputWidgetTurnBoxInnerWidth(paneWidth int) int {
	inner := outputWidgetContentBudget(paneWidth, "  │ ")
	if inner < 12 {
		inner = 12
	}
	return inner
}

func outputWidgetContentBudget(paneWidth int, linePrefix string) int {
	screen := NewOutputWidgetScreenState(paneWidth)
	return screen.ContentBudget(linePrefix)
}

func wrapOutputWidgetContent(content string, width int) []string {
	if width < 12 {
		width = 12
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return []string{""}
	}
	chunks := strings.Split(trimmed, "\n")
	wrapped := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		line := strings.TrimSpace(chunk)
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, wrapTextLines(line, width)...)
	}
	if len(wrapped) == 0 {
		return []string{""}
	}
	return wrapped
}

func renderOutputWidgetTurnDetails(snapshot outputWidgetSnapshot, turnIndex int, paneWidth int, viewState *outputWidgetViewState) []string {
	return renderOutputWidgetTurnDetailsWithFormatter(snapshot, turnIndex, paneWidth, viewState, nil)
}

func renderOutputWidgetTurnDetailsWithFormatter(snapshot outputWidgetSnapshot, turnIndex int, paneWidth int, viewState *outputWidgetViewState, formatter OutputResponseFormatter) []string {
	renderer, ok := newOutputTurnRendererWithFormatter(snapshot, turnIndex, paneWidth, viewState, formatter)
	if !ok {
		return nil
	}
	return renderer.render()
}

func renderOutputWidget(snapshot outputWidgetSnapshot, paneHeight int, paneWidth int) string {
	return renderOutputWidgetWithViewState(snapshot, paneHeight, paneWidth, nil)
}

func renderOutputWidgetWithViewState(snapshot outputWidgetSnapshot, paneHeight int, paneWidth int, viewState *outputWidgetViewState) string {
	return renderOutputWidgetWithViewStateAndFormatter(snapshot, paneHeight, paneWidth, viewState, nil)
}

func renderOutputWidgetWithViewStateAndFormatter(snapshot outputWidgetSnapshot, paneHeight int, paneWidth int, viewState *outputWidgetViewState, formatter OutputResponseFormatter) string {
	if formatter == nil {
		formatter = ResolveOutputResponseFormatterFromEnv()
	}
	if viewState != nil {
		viewState.normalize(snapshot.SessionID, len(snapshot.Turns))
		viewState.maybeExpandLatestTurn(snapshot)
		viewState.lastPaneHeight = paneHeight
	}

	lines := []string{}
	if viewState != nil {
		if viewState.showHelp {
			lines = append(lines,
				"Output keymap:",
				"  j / ↓   next turn (container) or entry (entry mode)",
				"  k / ↑   previous turn (container) or entry (entry mode)",
				"  Tab     drill in to entry focus",
				"  S-Tab   drill out to container focus",
				"  l / h   expand/collapse focused target",
				"  Enter   toggle focused target",
				"  Space   toggle focused target",
				"  PgUp    page up focused entry",
				"  PgDn    page down focused entry",
				"  Home    jump to oldest turn",
				"  End     jump to newest turn",
				"  ?       toggle help",
				"  q       close widget",
			)
		}
		if viewState.showClipboard && strings.TrimSpace(viewState.clipboard) != "" {
			lines = append(lines,
				"Clipboard payload:",
				viewState.clipboard,
			)
		}
	}
	for turnIndex := 1; turnIndex <= len(snapshot.Turns); turnIndex++ {
		turn := snapshot.Turns[turnIndex-1]
		if strings.TrimSpace(turn.Prompt) == "" && strings.TrimSpace(turn.Response) == "" {
			continue
		}
		if viewState != nil && !viewState.turnExpandedState(turnIndex) {
			summary := renderOutputWidgetTurnSummary(turn, viewState.focusedTurn == turnIndex && !viewState.entryFocusMode)
			if turnIndex == len(snapshot.Turns) {
				summary += " [LATEST]"
			}
			lines = append(lines, summary)
			stubWidth := outputWidgetTurnBoxInnerWidth(paneWidth)
			lines = append(lines,
				fmt.Sprintf("  ┌%s┐", strings.Repeat("─", stubWidth+2)),
				fmt.Sprintf("  └%s┘", strings.Repeat("─", stubWidth+2)),
			)
			continue
		}
		lines = append(lines, renderOutputWidgetTurnDetailsWithFormatter(snapshot, turnIndex, paneWidth, viewState, formatter)...)
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
