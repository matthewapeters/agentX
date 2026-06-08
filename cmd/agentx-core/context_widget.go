package main

import (
	"bufio"
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

	"github.com/mattn/go-runewidth"
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
	Username    string
	SessionID   string
	Turns       []ChatTurn
	LastUpdated time.Time
}

type contextHistoryUser struct {
	Username string
	Sessions []contextHistorySession
}

type nodeKind string

const (
	nodeKindSection nodeKind = "section"
	nodeKindUser    nodeKind = "user"
	nodeKindSession nodeKind = "session"
	nodeKindTurn    nodeKind = "turn"
	nodeKindWM      nodeKind = "working-memory"
	nodeKindWmCell  nodeKind = "working-memory-cell"
	nodeKindEntry   nodeKind = "entry"
)

type nodeID struct {
	Kind    nodeKind
	Section string
	User    string
	Session string
	Turn    int
	Entry   string
	Cell    string
}

func (id nodeID) String() string {
	switch id.Kind {
	case nodeKindSection:
		return "section:" + strings.TrimSpace(id.Section)
	case nodeKindUser:
		return "user:" + strings.TrimSpace(id.User)
	case nodeKindSession:
		return "session:" + strings.TrimSpace(id.User) + ":" + strings.TrimSpace(id.Session)
	case nodeKindTurn:
		return fmt.Sprintf("turn:%s:%s:%d", strings.TrimSpace(id.User), strings.TrimSpace(id.Session), id.Turn)
	case nodeKindWM:
		return "wm"
	case nodeKindWmCell:
		return "wm:editor:" + strings.TrimSpace(id.Cell)
	case nodeKindEntry:
		return fmt.Sprintf("entry:%s:%s:%d:%s", strings.TrimSpace(id.User), strings.TrimSpace(id.Session), id.Turn, strings.TrimSpace(id.Entry))
	default:
		return "unknown"
	}
}

func (id nodeID) Parent() (nodeID, bool) {
	switch id.Kind {
	case nodeKindTurn:
		return nodeID{Kind: nodeKindSession, User: id.User, Session: id.Session}, true
	case nodeKindEntry:
		return nodeID{Kind: nodeKindTurn, User: id.User, Session: id.Session, Turn: id.Turn}, true
	case nodeKindSession:
		return nodeID{Kind: nodeKindUser, User: id.User}, true
	case nodeKindUser:
		return nodeID{Kind: nodeKindSection, Section: "context-history"}, true
	case nodeKindWmCell:
		return nodeID{Kind: nodeKindWM}, true
	case nodeKindWM:
		return nodeID{Kind: nodeKindSection, Section: "working-memory"}, true
	case nodeKindSection:
		return nodeID{Kind: nodeKindSection, Section: "root"}, true
	default:
		return nodeID{}, false
	}
}

type focusPath []nodeID

func (path focusPath) Clone() focusPath {
	if len(path) == 0 {
		return focusPath{}
	}
	clone := make(focusPath, len(path))
	copy(clone, path)
	return clone
}

func (path focusPath) Tail() nodeID {
	if len(path) == 0 {
		return nodeID{}
	}
	return path[len(path)-1]
}

func (path focusPath) HasFocus(id nodeID) bool {
	return len(path) > 0 && path.Tail().String() == id.String()
}

func (path focusPath) Pop() (focusPath, nodeID, bool) {
	if len(path) == 0 {
		return path, nodeID{}, false
	}
	parent := path[:len(path)-1]
	return parent, path[len(path)-1], true
}

func (path focusPath) Push(id nodeID) focusPath {
	clone := path.Clone()
	clone = append(clone, id)
	return clone
}

func commonPathPrefix(a focusPath, b focusPath) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i].String() != b[i].String() {
			return i
		}
	}
	return limit
}

func nodeIDForRowKey(section string, rowKey string) (nodeID, bool) {
	key := strings.TrimSpace(rowKey)
	switch strings.TrimSpace(section) {
	case "context-history":
		if strings.HasPrefix(key, "user:") {
			return nodeID{Kind: nodeKindUser, User: strings.TrimPrefix(key, "user:")}, true
		}
		if strings.HasPrefix(key, "session:") {
			parts := strings.SplitN(strings.TrimPrefix(key, "session:"), ":", 2)
			if len(parts) == 2 {
				return nodeID{Kind: nodeKindSession, User: parts[0], Session: parts[1]}, true
			}
		}
		if strings.HasPrefix(key, "history:") {
			parts := strings.Split(strings.TrimPrefix(key, "history:"), ":")
			if len(parts) == 3 {
				turn, err := strconv.Atoi(parts[2])
				if err == nil {
					return nodeID{Kind: nodeKindTurn, User: parts[0], Session: parts[1], Turn: turn}, true
				}
			}
		}
	case "working-memory":
		if strings.HasPrefix(key, "wm:editor:") {
			return nodeID{Kind: nodeKindWmCell, Cell: strings.TrimPrefix(key, "wm:editor:")}, true
		}
		if strings.HasPrefix(key, "wm:") {
			return nodeID{Kind: nodeKindWM, Entry: key}, true
		}
	case "current-context":
		if strings.HasPrefix(key, "current:") {
			parts := strings.Split(strings.TrimPrefix(key, "current:"), ":")
			if len(parts) == 2 {
				turn, err := strconv.Atoi(parts[0])
				if err == nil {
					return nodeID{Kind: nodeKindEntry, Turn: turn, Entry: parts[1]}, true
				}
			}
		}
	}
	return nodeID{}, false
}

const (
	contextHistorySortAscending = "ascending"
	contextHistorySortDescending = "descending"
)

type contextFeedbackViewState struct {
	showHelp                bool
	collapsedContextHistory bool
	collapsedEntries        map[string]bool
	disabledEntries         map[string]bool
	selectedEntries         map[string]bool
	orderedRowKeys          []string
	activeRow               int
	focusTextBox            bool
	textScroll              map[string]int
	showWorkingMemory       bool
	collapsedWorkingMemory  bool
	collapsedCurrentContext bool
	statusLine              string
	statusUntil             time.Time
	// Section-level navigation state.
	// activeSection is the section header the cursor rests on when insideSection=false.
	// insideSection=true means the cursor is navigating rows within the active section.
	activeSection string
	insideSection bool
	focusPath     focusPath
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
	ansiDim     = "\033[2m"
)

func newContextFeedbackViewState() *contextFeedbackViewState {
	return &contextFeedbackViewState{
		collapsedEntries:        make(map[string]bool),
		disabledEntries:         make(map[string]bool),
		selectedEntries:         make(map[string]bool),
		orderedRowKeys:          []string{},
		textScroll:              make(map[string]int),
		showWorkingMemory:       true,
		collapsedContextHistory: true,
		collapsedWorkingMemory:  true,
		activeSection:           "current-context",
		insideSection:           false,
		focusPath:               focusPath{nodeID{Kind: nodeKindSection, Section: "current-context"}},
	}
}

// moveSectionHeader advances the section-header cursor by delta within the ordered
// section list [context-history, working-memory, current-context].
func (state *contextFeedbackViewState) moveSectionHeader(delta int) {
	if state == nil {
		return
	}
	order := []string{"context-history", "working-memory", "current-context"}
	idx := 0
	for i, s := range order {
		if s == state.activeSection {
			idx = i
			break
		}
	}
	next := idx + delta
	if next < 0 {
		next = 0
	}
	if next >= len(order) {
		next = len(order) - 1
	}
	state.activeSection = order[next]
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
	if node, ok := nodeIDForRowKey(state.activeSection, state.activeRowKey()); ok {
		state.setFocusPath(focusPathForNode(state.activeSection, node))
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
		if node, ok := nodeIDForRowKey(state.activeSection, state.activeRowKey()); ok {
			state.setFocusPath(focusPathForNode(state.activeSection, node))
		}
	}
	return changed
}

func rowBelongsToSection(rowKey string, section string) bool {
	key := strings.TrimSpace(rowKey)
	switch strings.TrimSpace(section) {
	case "context-history":
		return strings.HasPrefix(key, "user:") || strings.HasPrefix(key, "session:") || strings.HasPrefix(key, "history:")
	case "working-memory":
		return strings.HasPrefix(key, "wm:")
	case "current-context":
		return strings.HasPrefix(key, "current:")
	default:
		return false
	}
}

func (state *contextFeedbackViewState) ensureSectionRowFocus() bool {
	if state == nil || len(state.orderedRowKeys) == 0 {
		return false
	}
	if rowBelongsToSection(state.activeRowKey(), state.activeSection) {
		return true
	}
	for idx, key := range state.orderedRowKeys {
		if !rowBelongsToSection(key, state.activeSection) {
			continue
		}
		state.activeRow = idx
		return true
	}
	return false
}

func (state *contextFeedbackViewState) moveRowInActiveSection(delta int) bool {
	if state == nil || len(state.orderedRowKeys) == 0 {
		return false
	}
	indices := make([]int, 0, len(state.orderedRowKeys))
	for idx, key := range state.orderedRowKeys {
		if rowBelongsToSection(key, state.activeSection) {
			indices = append(indices, idx)
		}
	}
	if len(indices) == 0 {
		return false
	}
	current := -1
	for pos, idx := range indices {
		if idx == state.activeRow {
			current = pos
			break
		}
	}
	if current == -1 {
		state.activeRow = indices[0]
		state.focusTextBox = false
		if node, ok := nodeIDForRowKey(state.activeSection, state.activeRowKey()); ok {
			state.setFocusPath(focusPathForNode(state.activeSection, node))
		}
		return true
	}
	next := current + delta
	if next < 0 {
		next = 0
	}
	if next >= len(indices) {
		next = len(indices) - 1
	}
	changed := state.activeRow != indices[next]
	state.activeRow = indices[next]
	if changed {
		state.focusTextBox = false
		if node, ok := nodeIDForRowKey(state.activeSection, state.activeRowKey()); ok {
			state.setFocusPath(focusPathForNode(state.activeSection, node))
		}
	}
	return changed
}

func (state *contextFeedbackViewState) setFocusPath(path focusPath) {
	if state == nil {
		return
	}
	state.focusPath = path.Clone()
	if len(state.focusPath) == 0 {
		state.activeSection = "current-context"
		state.insideSection = false
		return
	}
	if state.focusPath[0].Kind == nodeKindSection && strings.TrimSpace(state.focusPath[0].Section) != "" {
		state.activeSection = state.focusPath[0].Section
	}
	tail := state.focusPath.Tail()
	if tail.Kind == nodeKindSection {
		state.insideSection = false
		return
	}
	state.insideSection = true
}

func (state *contextFeedbackViewState) pushFocus(id nodeID) {
	if state == nil {
		return
	}
	state.focusPath = state.focusPath.Push(id)
	state.setFocusPath(state.focusPath)
}

func (state *contextFeedbackViewState) popFocus() bool {
	if state == nil || len(state.focusPath) <= 1 {
		return false
	}
	parent, _, ok := state.focusPath.Pop()
	if !ok {
		return false
	}
	state.setFocusPath(parent)
	return true
}

func (state *contextFeedbackViewState) branchTransition(next focusPath) {
	if state == nil {
		return
	}
	current := state.focusPath.Clone()
	keep := commonPathPrefix(current, next)
	if keep < len(current) {
		current = current[:keep]
	}
	state.setFocusPath(append(current, next[keep:]...))
}

func focusPathForNode(section string, id nodeID) focusPath {
	base := focusPath{{Kind: nodeKindSection, Section: strings.TrimSpace(section)}}
	switch id.Kind {
	case nodeKindUser:
		return append(base, nodeID{Kind: nodeKindUser, User: id.User})
	case nodeKindSession:
		return append(base,
			nodeID{Kind: nodeKindUser, User: id.User},
			nodeID{Kind: nodeKindSession, User: id.User, Session: id.Session},
		)
	case nodeKindTurn:
		return append(base,
			nodeID{Kind: nodeKindUser, User: id.User},
			nodeID{Kind: nodeKindSession, User: id.User, Session: id.Session},
			nodeID{Kind: nodeKindTurn, User: id.User, Session: id.Session, Turn: id.Turn},
		)
	case nodeKindWmCell:
		return append(base,
			nodeID{Kind: nodeKindWM},
			nodeID{Kind: nodeKindWmCell, Cell: id.Cell},
		)
	case nodeKindWM:
		return append(base, nodeID{Kind: nodeKindWM})
	case nodeKindEntry:
		return append(base, nodeID{Kind: nodeKindEntry, Turn: id.Turn, Entry: id.Entry})
	case nodeKindSection:
		return focusPath{{Kind: nodeKindSection, Section: id.Section}}
	default:
		return base
	}
}

func focusPathHasNode(path focusPath, id nodeID) bool {
	target := id.String()
	for _, node := range path {
		if node.String() == target {
			return true
		}
	}
	return false
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
		sessionRef := strings.TrimPrefix(current, "session:")
		prefix := "history:" + sessionRef + ":"
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
		if len(parts) == 3 {
			sessionID := strings.TrimSpace(parts[1])
			return state.setActiveRowByKey("session:" + sessionID)
		}
		username := strings.TrimSpace(parts[1])
		sessionID := strings.TrimSpace(parts[2])
		return state.setActiveRowByKey(historySessionRowKey(username, sessionID))
	}

	if strings.HasPrefix(current, "wm:editor:") {
		parts := strings.Split(current, ":")
		if len(parts) != 3 {
			return false
		}
		cell := strings.TrimSpace(parts[2])
		switch direction {
		case "right":
			switch cell {
			case "key":
				return state.setActiveRowByKey("wm:editor:value")
			case "value":
				return state.setActiveRowByKey("wm:editor:save")
			}
		case "left":
			switch cell {
			case "save":
				return state.setActiveRowByKey("wm:editor:value")
			case "value":
				return state.setActiveRowByKey("wm:editor:key")
			}
		}
		return false
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
	// Hide the terminal cursor for the duration of the widget loop to prevent
	// cursor flicker at the end of rendered content.
	fmt.Fprint(out, "\033[?25l")
	defer fmt.Fprint(out, "\033[?25h")

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
			currentLines := filterContextWidgetTUILines(filesystemWidgetFrameLines(render))
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
	sortOrder := resolveContextHistorySessionSort(projectDir)

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
		history = append(history, contextHistorySession{Username: username, SessionID: sessionID, Turns: turns, LastUpdated: lastUpdated})
	}

	sort.SliceStable(history, func(i, j int) bool {
		if history[i].LastUpdated.Equal(history[j].LastUpdated) {
			if sortOrder == contextHistorySortAscending {
				return history[i].SessionID < history[j].SessionID
			}
			return history[i].SessionID > history[j].SessionID
		}
		if sortOrder == contextHistorySortAscending {
			return history[i].LastUpdated.Before(history[j].LastUpdated)
		}
		return history[i].LastUpdated.After(history[j].LastUpdated)
	})
	return history
}

func discoverContextHistoryUsers(currentSessionID string) []contextHistoryUser {
	projectDir := strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR"))
	if projectDir == "" {
		return []contextHistoryUser{}
	}
	sortOrder := resolveContextHistorySessionSort(projectDir)

	usersRoot := filepath.Join(projectDir, "sessions")
	userEntries, err := os.ReadDir(usersRoot)
	if err != nil {
		return []contextHistoryUser{}
	}

	users := make([]contextHistoryUser, 0, len(userEntries))
	for _, userEntry := range userEntries {
		if !userEntry.IsDir() {
			continue
		}
		username := strings.TrimSpace(userEntry.Name())
		if username == "" {
			continue
		}
		sessionsRoot := filepath.Join(usersRoot, username)
		sessionEntries, err := os.ReadDir(sessionsRoot)
		if err != nil {
			continue
		}
		sessions := make([]contextHistorySession, 0, len(sessionEntries))
		for _, entry := range sessionEntries {
			if !entry.IsDir() {
				continue
			}
			sessionID := strings.TrimSpace(entry.Name())
			if sessionID == "" {
				continue
			}
			if username == strings.TrimSpace(os.Getenv("AGENTX_USERNAME")) && sessionID == strings.TrimSpace(currentSessionID) {
				continue
			}
			turnPath := filepath.Join(sessionsRoot, sessionID, "context", "turns.jsonl")
			turns, lastUpdated, ok := loadSessionTurns(turnPath)
			if !ok {
				continue
			}
			sessions = append(sessions, contextHistorySession{Username: username, SessionID: sessionID, Turns: turns, LastUpdated: lastUpdated})
		}
		if len(sessions) == 0 {
			continue
		}
		sort.SliceStable(sessions, func(i, j int) bool {
			if sessions[i].LastUpdated.Equal(sessions[j].LastUpdated) {
				if sortOrder == contextHistorySortAscending {
					return sessions[i].SessionID < sessions[j].SessionID
				}
				return sessions[i].SessionID > sessions[j].SessionID
			}
			if sortOrder == contextHistorySortAscending {
				return sessions[i].LastUpdated.Before(sessions[j].LastUpdated)
			}
			return sessions[i].LastUpdated.After(sessions[j].LastUpdated)
		})
		users = append(users, contextHistoryUser{Username: username, Sessions: sessions})
	}

	sort.SliceStable(users, func(i, j int) bool {
		return strings.ToLower(users[i].Username) < strings.ToLower(users[j].Username)
	})
	return users
}

func normalizeContextHistorySessionSort(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "asc", "ascending":
		return contextHistorySortAscending
	case "desc", "descending":
		return contextHistorySortDescending
	default:
		return contextHistorySortDescending
	}
}

func resolveContextHistorySessionSort(projectDir string) string {
	if override := strings.TrimSpace(os.Getenv("AGENTX_CONTEXT_HISTORY_SESSION_SORT")); override != "" {
		return normalizeContextHistorySessionSort(override)
	}
	root := strings.TrimSpace(projectDir)
	if root == "" {
		root = strings.TrimSpace(os.Getenv("AGENTX_PROJECT_DIR"))
	}
	if root == "" {
		return contextHistorySortDescending
	}

	configPath := filepath.Join(root, "agentx.toml")
	file, err := os.Open(configPath)
	if err != nil {
		return contextHistorySortDescending
	}
	defer file.Close()

	currentSection := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}
		if currentSection != "agentx" {
			continue
		}
		key, value, ok := parseTomlKeyValue(line)
		if !ok || key != "context_history_session_sort" {
			continue
		}
		return normalizeContextHistorySessionSort(value)
	}
	return contextHistorySortDescending
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
	effectiveTurns := filterRenderableTurns(snapshot.Turns)
	historyUsers := discoverContextHistoryUsers(snapshot.SessionID)
	lines := []string{
		"",
		reverseTitle("CONTEXT FEEDBACK APPLET"),
	}
	_ = history

	// Section order follows the UX contract:
	// 1) Context history (title only when collapsed)
	// 2) Working memory (title only when collapsed)
	// 3) Current context (expanded, element tiles collapsed by default)
	lines = append(lines, "", renderSectionHeader("CONTEXT HISTORY", "context-history", viewState))
	historyCollapsed := true
	if viewState != nil {
		historyCollapsed = viewState.collapsedContextHistory
	}
	historyBorder := sectionBorderColor("context-history", viewState)
	if historyCollapsed {
		lines = append(lines, renderCollapsedBoxStub(historyBorder, 42)...)
	} else {
		historyItems := make([]string, 0)
		if len(historyUsers) == 0 {
			historyItems = append(historyItems, "No prior sessions found on disk.")
		} else {
			for _, user := range historyUsers {
				userRowKey := "user:" + user.Username
				rowKeys = append(rowKeys, userRowKey)
				userNode := nodeID{Kind: nodeKindUser, User: user.Username}
				userCollapsed := !focusPathHasNode(viewState.focusPath, userNode)
				historyItems = append(historyItems, fmt.Sprintf("%s %s %s", rowMarker(viewState, userRowKey), styleToken("📂", ansiCyan), trimSingleLine(user.Username, 32)))
				if userCollapsed {
					// User collapsed: show a collapsed inner box stub representing user sessions.
					historyItems = append(historyItems, "  ┌──────────────────────────────────────┐", "  └──────────────────────────────────────┘")
					continue
				}
				// User expanded: wrap all sessions inside an inner user box.
				historyItems = append(historyItems, "  ┌──────────────────────────────────────┐")
				sessionsOffset := sectionViewportOffset(viewState, "context-history")
				visibleSessions := viewportSessions(user.Sessions, sessionsOffset, 4)
				for _, session := range visibleSessions {
					sessionRowKey := historySessionRowKey(session.Username, session.SessionID)
					rowKeys = append(rowKeys, sessionRowKey)
					sessionNode := nodeID{Kind: nodeKindSession, User: session.Username, Session: session.SessionID}
					sessionCollapsed := !focusPathHasNode(viewState.focusPath, sessionNode)
					startLabel := sessionStartLabel(session)
					historyItems = append(historyItems,
						fmt.Sprintf("  │ %s 📑 %s", rowMarker(viewState, sessionRowKey), trimSingleLine(startLabel, 30)),
						"  │   ┌────────────────────────────────────┐",
					)
					if sessionCollapsed {
						historyItems = append(historyItems, "  │   └────────────────────────────────────┘")
						continue
					}
					if len(session.Turns) == 0 {
						historyItems = append(historyItems, "  │   │ (empty session)                    │")
					} else {
						for idx, turn := range session.Turns {
							turnNumber := idx + 1
							turnKey := fmt.Sprintf("history:%s:%s:%d", session.Username, session.SessionID, turnNumber)
							rowKeys = append(rowKeys, turnKey)
							historyItems = append(historyItems,
								fmt.Sprintf("  │   │ %s 👤  %s", rowMarker(viewState, turnKey), trimSingleLine(turn.Prompt, 24)),
								fmt.Sprintf("  │   │   🤖  %s", trimSingleLine(turn.Response, 24)),
							)
						}
					}
					historyItems = append(historyItems,
						fmt.Sprintf("  │   │   %s include: i %s 1 b", styleToken("➕", ansiCyan), trimSingleLine(session.SessionID, 16)),
						"  │   └────────────────────────────────────┘",
					)
				}
				historyItems = append(historyItems, "  └──────────────────────────────────────┘")
			}
		}
		lines = append(lines, boxContainer(historyItems, historyBorder)...)
	}

	if viewState == nil || viewState.showWorkingMemory {
		lines = append(lines, "")
		lines = append(lines, renderWorkingMemoryFeedbackSection(viewState, &rowKeys)...)
	}

	lines = append(lines, "", renderSectionHeader("CURRENT CONTEXT", "current-context", viewState))
	currentBorder := sectionBorderColor("current-context", viewState)
	currentCollapsed := viewState != nil && viewState.collapsedCurrentContext
	if currentCollapsed {
		lines = append(lines, renderCollapsedBoxStub(currentBorder, 38)...)
	} else if len(effectiveTurns) == 0 {
		// Empty context: show an empty box stub (no text message).
		lines = append(lines, renderCollapsedBoxStub(currentBorder, 38)...)
	} else {
		turnItems := make([]string, 0)
		for idx, turn := range effectiveTurns {
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

			promptStateIcon := "✔️"
			if promptDisabled {
				promptStateIcon = "⬜"
			}
			responseStateIcon := "✔️"
			if responseDisabled {
				responseStateIcon = "⬜"
			}

			if promptCollapsed {
				turnItems = append(turnItems, fmt.Sprintf("%s %s 👤  %s", rowMarker(viewState, promptKey), promptStateIcon, trimSingleLine(turn.Prompt, 36)))
			} else {
				turnItems = append(turnItems, fmt.Sprintf("%s %s 👤", rowMarker(viewState, promptKey), promptStateIcon))
				turnItems = append(turnItems, renderWrappedTextBox(viewState, promptKey, turn.Prompt, ansiBlue)...)
			}
			if responseCollapsed {
				turnItems = append(turnItems, fmt.Sprintf("%s %s 🤖  %s", rowMarker(viewState, responseKey), responseStateIcon, trimSingleLine(turn.Response, 36)))
			} else {
				turnItems = append(turnItems, fmt.Sprintf("%s %s 🤖", rowMarker(viewState, responseKey), responseStateIcon))
				turnItems = append(turnItems, renderWrappedTextBox(viewState, responseKey, turn.Response, ansiGreen)...)
			}
		}
		lines = append(lines, boxContainer(turnItems, currentBorder)...)
	}
	if viewState != nil {
		viewState.updateOrderedRows(rowKeys)
	}

	return lines
}

func renderWorkingMemoryFeedbackSection(viewState *contextFeedbackViewState, rowKeys *[]string) []string {
	lines := []string{renderSectionHeader("WORKING MEMORY", "working-memory", viewState)}
	wmBorder := sectionBorderColor("working-memory", viewState)
	if viewState != nil && viewState.collapsedWorkingMemory {
		lines = append(lines, renderCollapsedBoxStub(wmBorder, 54)...)
		return lines
	}
	// Single box for entire working memory section: editor scaffold + facts list.
	allItems := make([]string, 0)
	allItems = append(allItems, renderWorkingMemoryEditorScaffold(viewState, rowKeys, wmBorder)...)
	sessionDir := resolveCurrentSessionDirFromEnv()
	facts := loadWorkingMemoryFacts(sessionDir)
	facts = appendDefaultWorkingMemoryFacts(facts)
	facts = viewportFacts(facts, sectionViewportOffset(viewState, "working-memory"), 8)
	for _, fact := range facts {
		factKey := fmt.Sprintf("wm:%s:%s", fact.owner, fact.key)
		if rowKeys != nil {
			*rowKeys = append(*rowKeys, factKey)
		}
		stateIcon := "✔️"
		if !fact.enabled {
			stateIcon = "⬜"
		}
		line := fmt.Sprintf("%s %s: %s", stateIcon, trimSingleLine(fact.key, 32), trimSingleLine(formatWorkingMemoryValue(fact.value), 56))
		if viewState != nil && viewState.insideSection && viewState.activeRowKey() == factKey {
			line = ansiReverse + line + ansiReset
		}
		allItems = append(allItems, line)
	}
	lines = append(lines, boxContainer(allItems, wmBorder)...)
	return lines
}

func viewportFacts(facts []workingMemoryFactLine, offset int, pageSize int) []workingMemoryFactLine {
	if len(facts) == 0 {
		return facts
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(facts) {
		offset = len(facts) - 1
	}
	end := offset + pageSize
	if end > len(facts) {
		end = len(facts)
	}
	return facts[offset:end]
}

func viewportSessions(sessions []contextHistorySession, offset int, pageSize int) []contextHistorySession {
	if len(sessions) == 0 {
		return sessions
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(sessions) {
		offset = len(sessions) - 1
	}
	end := offset + pageSize
	if end > len(sessions) {
		end = len(sessions)
	}
	return sessions[offset:end]
}

func sectionViewportOffset(state *contextFeedbackViewState, section string) int {
	if state == nil {
		return 0
	}
	key := "section:" + strings.TrimSpace(section)
	if state.textScroll[key] < 0 {
		state.textScroll[key] = 0
	}
	return state.textScroll[key]
}

func historySessionRowKey(username string, sessionID string) string {
	return "session:" + strings.TrimSpace(username) + ":" + strings.TrimSpace(sessionID)
}

func sessionStartLabel(session contextHistorySession) string {
	parts := strings.Split(strings.TrimSpace(session.SessionID), "_")
	if len(parts) >= 2 {
		if parsed, err := strconv.ParseInt(parts[len(parts)-1], 10, 64); err == nil && parsed > 0 {
			return time.Unix(parsed, 0).Format("2006-01-02 15:04:05")
		}
	}
	if !session.LastUpdated.IsZero() {
		return session.LastUpdated.Format("2006-01-02 15:04:05")
	}
	return "unknown"
}

func indentLines(items []string, prefix string) []string {
	if strings.TrimSpace(prefix) == "" || len(items) == 0 {
		return items
	}
	indented := make([]string, 0, len(items))
	for _, item := range items {
		indented = append(indented, prefix+item)
	}
	return indented
}

func renderWorkingMemoryEditorScaffold(viewState *contextFeedbackViewState, rowKeys *[]string, borderColor string) []string {
	keyRow := "wm:editor:key"
	valueRow := "wm:editor:value"
	saveRow := "wm:editor:save"
	if rowKeys != nil {
		*rowKeys = append(*rowKeys, keyRow, valueRow, saveRow)
	}
	keyCell := "                       "
	valueCell := "                       "
	saveLbl := " ↳OK "
	if viewState != nil && viewState.insideSection {
		if viewState.activeRowKey() == keyRow {
			keyCell = ansiReverse + keyCell + ansiReset
		}
		if viewState.activeRowKey() == valueRow {
			valueCell = ansiReverse + valueCell + ansiReset
		}
		if viewState.activeRowKey() == saveRow {
			saveLbl = ansiReverse + saveLbl + ansiReset
		}
	}
	return []string{
		"KEY                       VALUE",
		"┌───────────────────────┐ ┌───────────────────────┐ ┌─────┐",
		fmt.Sprintf("│%s│ │%s│ │%s│", keyCell, valueCell, saveLbl),
		"└───────────────────────┘ └───────────────────────┘ └─────┘",
	}
}

func appendDefaultWorkingMemoryFacts(facts []workingMemoryFactLine) []workingMemoryFactLine {
	if len(facts) > 0 {
		return facts
	}
	defaults := make([]workingMemoryFactLine, 0, 2)
	if user := strings.TrimSpace(os.Getenv("AGENTX_USERNAME")); user != "" {
		defaults = append(defaults, workingMemoryFactLine{owner: "user", key: "current_user", value: user, enabled: true})
	}
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		defaults = append(defaults, workingMemoryFactLine{owner: "user", key: "current_working_directory", value: cwd, enabled: true})
	}
	return defaults
}

func reverseTitle(title string) string {
	label := strings.TrimSpace(title)
	if label == "" {
		return ""
	}
	return ansiReverse + " " + label + " " + ansiReset
}

// renderSectionHeader renders a section title. When the cursor is on this
// section in outside-section mode, a ▶ indicator is prepended.
// When the cursor is inside the section, a ↳ indicator is prepended.
func renderSectionHeader(title string, sectionID string, viewState *contextFeedbackViewState) string {
	label := sectionHeaderLabel(strings.TrimSpace(title), sectionID)
	prefix := "  "
	if viewState != nil {
		if viewState.insideSection && viewState.activeSection == sectionID {
			prefix = styleToken("↳ ", ansiCyan)
		} else if !viewState.insideSection && viewState.activeSection == sectionID {
			prefix = styleToken("▶ ", ansiCyan)
		}
	}
	return prefix + reverseTitle(label)
}

func sectionHeaderLabel(title string, sectionID string) string {
	switch sectionID {
	case "context-history":
		return "🗄️ " + title
	case "working-memory":
		return "💾 " + title
	case "current-context":
		return "📑 " + title
	default:
		return title
	}
}

// sectionBorderColor returns a bright border color when the cursor is inside
// the given section, and a dim color otherwise.
func sectionBorderColor(sectionID string, viewState *contextFeedbackViewState) string {
	if viewState != nil && viewState.insideSection && viewState.activeSection == sectionID {
		return ansiCyan
	}
	return ansiDim
}

func boxContainer(items []string, borderColor string) []string {
	if len(items) == 0 {
		items = []string{"(empty)"}
	}
	maxWidth := 16
	for _, item := range items {
		if width := visibleDisplayWidth(item); width > maxWidth {
			maxWidth = width
		}
	}
	line := strings.Repeat("─", maxWidth+2)
	lines := []string{borderColor + "┌" + line + "┐" + ansiReset}
	for _, item := range items {
		lines = append(lines, borderColor+"│ "+padVisibleWidth(item, maxWidth)+" │"+ansiReset)
	}
	lines = append(lines, borderColor+"└"+line+"┘"+ansiReset)
	return lines
}

// filterContextWidgetTUILines strips machine-readable protocol lines that are
// embedded in the render output for test/HTTP consumers but should not be
// visible in the TUI terminal display.
func filterContextWidgetTUILines(lines []string) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		stripped := strings.TrimSpace(stripAnsi(line))
		if strings.HasPrefix(stripped, "[SYSTEM") ||
			stripped == "== CONTEXT HISTORY ==" ||
			strings.HasPrefix(stripped, "history_context_count:") ||
			strings.HasPrefix(stripped, "recent_prompt:") ||
			strings.HasPrefix(stripped, "recent_response:") ||
			strings.HasPrefix(stripped, "turn_count:") {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
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
	maxWidth := runewidth.StringWidth(cleanTitle)
	for _, item := range items {
		if width := visibleDisplayWidth(item); width > maxWidth {
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
	visible := visibleDisplayWidth(value)
	if visible >= width {
		return value
	}
	return value + strings.Repeat(" ", width-visible)
}

func visibleDisplayWidth(value string) int {
	return runewidth.StringWidth(stripAnsi(value))
}

func mapCollapsedState(collapsed bool) string {
	if collapsed {
		return "collapsed"
	}
	return "expanded"
}

func renderCollapsedBoxStub(borderColor string, width int) []string {
	if width < 16 {
		width = 16
	}
	line := strings.Repeat("─", width)
	return []string{
		borderColor + "┌" + line + "┐" + ansiReset,
		borderColor + "└" + line + "┘" + ansiReset,
	}
}

func filterRenderableTurns(turns []ChatTurn) []ChatTurn {
	if len(turns) == 0 {
		return turns
	}
	filtered := make([]ChatTurn, 0, len(turns))
	for _, turn := range turns {
		if strings.TrimSpace(turn.Prompt) == "" && strings.TrimSpace(turn.Response) == "" {
			continue
		}
		filtered = append(filtered, turn)
	}
	return filtered
}

func styleCollapsedStateLabel(collapsed bool) string {
	if collapsed {
		return styleToken("collapsed", ansiYellow)
	}
	return styleToken("expanded", ansiGreen)
}

func formatRelativeTime(value time.Time) string {
	if value.IsZero() {
		return "time unknown"
	}
	delta := time.Since(value)
	if delta < 0 {
		delta = -delta
	}
	if delta < time.Minute {
		return "just now"
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm ago", int(delta/time.Minute))
	}
	if delta < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(delta/time.Hour))
	}
	return fmt.Sprintf("%dd ago", int(delta/(24*time.Hour)))
}

func mapContextEntryState(disabled bool) string {
	if disabled {
		return "disabled"
	}
	return "enabled"
}

func rowMarker(state *contextFeedbackViewState, rowKey string) string {
	// Suppress row-level cursor when navigating between section headers.
	if state == nil || !state.insideSection {
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
		if runewidth.StringWidth(candidate) <= width {
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
		visible = append(visible, fmt.Sprintf("%s scroll %d/%d (PgUp/PgDn to scroll)", styleToken("↕", ansiYellow), offset+1, len(wrapped)-4))
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
			sessionRef := strings.TrimSpace(args[2])
			if sessionRef == "" {
				state.setStatus("Usage: :toggle session <session_id|user:session_id>")
				return
			}
			var target nodeID
			if parts := strings.SplitN(sessionRef, ":", 2); len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
				target = nodeID{Kind: nodeKindSession, User: strings.TrimSpace(parts[0]), Session: strings.TrimSpace(parts[1])}
			} else {
				matches := make([]nodeID, 0, 1)
				for _, user := range discoverContextHistoryUsers(snapshot.SessionID) {
					for _, session := range user.Sessions {
						if session.SessionID != sessionRef {
							continue
						}
						matches = append(matches, nodeID{Kind: nodeKindSession, User: user.Username, Session: session.SessionID})
					}
				}
				switch len(matches) {
				case 0:
					state.setStatus(fmt.Sprintf("Session %s not found in history.", sessionRef))
					return
				case 1:
					target = matches[0]
				default:
					state.setStatus(fmt.Sprintf("Session %s is ambiguous; use user:session.", sessionRef))
					return
				}
			}

			targetPath := focusPathForNode("context-history", target)
			if state.focusPath.HasFocus(target) {
				state.branchTransition(targetPath[:len(targetPath)-1])
				state.setStatus(fmt.Sprintf("Session %s collapsed.", target.Session))
				return
			}
			state.branchTransition(targetPath)
			state.setStatus(fmt.Sprintf("Session %s expanded.", target.Session))
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
		state.setStatus("Usage: :toggle history | :toggle session <session_id|user:session_id> | :toggle wm")
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
			users := discoverContextHistoryUsers(snapshot.SessionID)
			found := false
			for _, user := range users {
				for _, session := range user.Sessions {
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
				if found {
					break
				}
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
		if !state.insideSection {
			state.moveSectionHeader(1)
			state.setStatus(fmt.Sprintf("Section: %s", state.activeSection))
			return true
		}
		if !state.moveRowInActiveSection(1) {
			state.setStatus("Selection at last row.")
		} else {
			state.setStatus("Selection moved.")
		}
		return true
	case "k", "up":
		if state.focusTextBox {
			rowKey := state.activeRowKey()
			state.textScroll[rowKey] = maxInt(0, state.textScroll[rowKey]-1)
			state.setStatus("Scrolled text box up.")
			return true
		}
		if !state.insideSection {
			state.moveSectionHeader(-1)
			state.setStatus(fmt.Sprintf("Section: %s", state.activeSection))
			return true
		}
		if !state.moveRowInActiveSection(-1) {
			state.setStatus("Selection at first row.")
		} else {
			state.setStatus("Selection moved.")
		}
		return true
	case "right", "l":
		if state.moveHorizontal("right") {
			state.setStatus("Moved right.")
		} else {
			state.setStatus("No right sibling.")
		}
		return true
	case "left", "h":
		if state.moveHorizontal("left") {
			state.setStatus("Moved left.")
		} else {
			state.setStatus("No left sibling.")
		}
		return true
	case "pgdn":
		if !state.insideSection {
			return true
		}
		if state.activeSection == "context-history" {
			state.textScroll["section:context-history"] = state.textScroll["section:context-history"] + 1
			state.setStatus("Viewport moved down: context history.")
			return true
		}
		if state.activeSection == "working-memory" {
			state.textScroll["section:working-memory"] = state.textScroll["section:working-memory"] + 1
			state.setStatus("Viewport moved down: working memory.")
			return true
		}
		if state.focusTextBox {
			rowKey := state.activeRowKey()
			state.textScroll[rowKey] = state.textScroll[rowKey] + 5
			state.setStatus("Paged text box down.")
			return true
		}
		// Scroll textbox content when on an expanded text row; otherwise page rows.
		rowKey := state.activeRowKey()
		if rowKey != "" && strings.HasPrefix(rowKey, "current:") && !state.collapsedEntries[rowKey] {
			state.textScroll[rowKey] = state.textScroll[rowKey] + 5
			state.setStatus("Paged text box down.")
			return true
		}
		state.moveRowInActiveSection(5)
		state.setStatus("Selection moved.")
		return true
	case "pgup":
		if !state.insideSection {
			return true
		}
		if state.activeSection == "context-history" {
			state.textScroll["section:context-history"] = maxInt(0, state.textScroll["section:context-history"]-1)
			state.setStatus("Viewport moved up: context history.")
			return true
		}
		if state.activeSection == "working-memory" {
			state.textScroll["section:working-memory"] = maxInt(0, state.textScroll["section:working-memory"]-1)
			state.setStatus("Viewport moved up: working memory.")
			return true
		}
		if state.focusTextBox {
			rowKey := state.activeRowKey()
			state.textScroll[rowKey] = maxInt(0, state.textScroll[rowKey]-5)
			state.setStatus("Paged text box up.")
			return true
		}
		rowKey := state.activeRowKey()
		if rowKey != "" && strings.HasPrefix(rowKey, "current:") && !state.collapsedEntries[rowKey] {
			state.textScroll[rowKey] = maxInt(0, state.textScroll[rowKey]-5)
			state.setStatus("Paged text box up.")
			return true
		}
		state.moveRowInActiveSection(-5)
		state.setStatus("Selection moved.")
		return true
	case "tab":
		// TAB drills into the active section and keeps focus there.
		if state.focusTextBox {
			state.focusTextBox = false
			state.setStatus("Text-box scroll disabled.")
		} else {
			if state.activeSection == "" {
				state.activeSection = "current-context"
			}
			switch state.activeSection {
			case "context-history":
				state.collapsedContextHistory = false
			case "working-memory":
				state.collapsedWorkingMemory = false
			case "current-context":
				state.collapsedCurrentContext = false
			}
			state.insideSection = true
		}
		state.insideSection = true
		state.ensureSectionRowFocus()
		if node, ok := nodeIDForRowKey(state.activeSection, state.activeRowKey()); ok {
			state.setFocusPath(focusPathForNode(state.activeSection, node))
		} else {
			state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: state.activeSection}})
		}
		state.insideSection = true
		state.setStatus(fmt.Sprintf("Entered section: %s.", state.activeSection))
		return true
	case "shift-tab", "shift+tab", "s-tab", "backtab":
		if state.focusTextBox {
			state.focusTextBox = false
		}
		if state.insideSection {
			state.popFocus()
			switch state.activeSection {
			case "context-history":
				state.collapsedContextHistory = true
			case "working-memory":
				state.collapsedWorkingMemory = true
			case "current-context":
				state.collapsedCurrentContext = true
			}
			state.insideSection = false
			state.setStatus(fmt.Sprintf("Exited section: %s.", state.activeSection))
		}
		return true
	case "space":
		// Outside a section: SPACE expands/collapses the active section.
		// Inside a section: SPACE selects/deselects the active row.
		if !state.insideSection {
			switch state.activeSection {
			case "context-history":
				state.collapsedContextHistory = !state.collapsedContextHistory
				state.setStatus(fmt.Sprintf("Context history is now %s.", mapCollapsedState(state.collapsedContextHistory)))
			case "working-memory":
				state.collapsedWorkingMemory = !state.collapsedWorkingMemory
				state.setStatus(fmt.Sprintf("Working memory is now %s.", mapCollapsedState(state.collapsedWorkingMemory)))
			case "current-context":
				state.collapsedCurrentContext = !state.collapsedCurrentContext
				state.setStatus(fmt.Sprintf("Current context is now %s.", mapCollapsedState(state.collapsedCurrentContext)))
			default:
				state.setStatus("No section selected.")
			}
			return true
		}
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
		// Outside a section: ENTER enters the section (same as TAB).
		if !state.insideSection {
			if state.activeSection == "" {
				state.activeSection = "current-context"
			}
			state.insideSection = true
			state.setStatus(fmt.Sprintf("Entered section: %s.", state.activeSection))
			return true
		}
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
		if state.activeSection == "context-history" {
			if id, ok := nodeIDForRowKey(state.activeSection, rowKey); ok {
				targetPath := focusPathForNode(state.activeSection, id)
				if state.focusPath.HasFocus(id) && len(targetPath) > 1 {
					state.branchTransition(targetPath[:len(targetPath)-1])
					if state.focusPath.Tail().Kind == nodeKindSection {
						state.insideSection = true
					}
					state.setStatus("History node collapsed.")
					return true
				}
				state.branchTransition(targetPath)
				state.setStatus("History node expanded.")
				return true
			}
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
		if visibleDisplayWidth(line) <= width {
			fitted = append(fitted, line)
			continue
		}
		if strings.IndexByte(line, '\x1b') != -1 {
			fitted = append(fitted, line)
			continue
		}
		if width <= 3 {
			fitted = append(fitted, runewidth.Truncate(line, width, ""))
			continue
		}
		fitted = append(fitted, strings.TrimSpace(runewidth.Truncate(line, width, "...")))
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
