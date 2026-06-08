package main

import "strings"

type contextHistoryNodeKind string

const (
	contextHistoryNodeKindSection contextHistoryNodeKind = "section"
	contextHistoryNodeKindUser    contextHistoryNodeKind = "user"
	contextHistoryNodeKindSession contextHistoryNodeKind = "session"
	contextHistoryNodeKindTurn    contextHistoryNodeKind = "turn"
)

// contextHistoryNodeID is applet-owned and supports tree/DAG ancestry via ParentKeys.
type contextHistoryNodeID interface {
	Kind() contextHistoryNodeKind
	Key() string
	ParentKeys() []string
}

type contextHistoryNodeRef struct {
	kind    contextHistoryNodeKind
	user    string
	session string
	turn    int
}

func (id contextHistoryNodeRef) Kind() contextHistoryNodeKind {
	return id.kind
}

func (id contextHistoryNodeRef) Key() string {
	switch id.kind {
	case contextHistoryNodeKindUser:
		return "user:" + id.user
	case contextHistoryNodeKindSession:
		if id.user == "" {
			return "session:" + id.session
		}
		return "session:" + id.user + ":" + id.session
	case contextHistoryNodeKindTurn:
		if id.user == "" {
			return "history:" + id.session + ":" + itoa(id.turn)
		}
		return "history:" + id.user + ":" + id.session + ":" + itoa(id.turn)
	default:
		return "section:context-history"
	}
}

func (id contextHistoryNodeRef) ParentKeys() []string {
	switch id.kind {
	case contextHistoryNodeKindUser:
		return []string{"section:context-history"}
	case contextHistoryNodeKindSession:
		if id.user == "" {
			return []string{"section:context-history"}
		}
		return []string{"user:" + id.user}
	case contextHistoryNodeKindTurn:
		if id.user == "" {
			return []string{"session:" + id.session}
		}
		return []string{"session:" + id.user + ":" + id.session}
	default:
		return []string{}
	}
}

type contextHistoryModel interface {
	NodeForRowKey(rowKey string) (contextHistoryNodeID, bool)
	EnsureRowFocus(state *contextFeedbackViewState) bool
	MoveVertical(state *contextFeedbackViewState, delta int) bool
	MoveHorizontal(state *contextFeedbackViewState, direction string) bool
	TogglePeek(state *contextFeedbackViewState) (string, bool)
	EnterSection(state *contextFeedbackViewState, forceExpand bool)
	ExitSection(state *contextFeedbackViewState)
}

type contextHistoryTreeModel struct {
	rowKeys []string
}

func newContextHistoryTreeModel(rowKeys []string) contextHistoryModel {
	clone := append([]string{}, rowKeys...)
	return &contextHistoryTreeModel{rowKeys: clone}
}

func (m *contextHistoryTreeModel) NodeForRowKey(rowKey string) (contextHistoryNodeID, bool) {
	return parseContextHistoryNode(strings.TrimSpace(rowKey))
}

func (m *contextHistoryTreeModel) EnsureRowFocus(state *contextFeedbackViewState) bool {
	if state == nil || len(m.rowKeys) == 0 {
		return false
	}
	active := state.activeRowKey()
	if isContextHistoryRowKey(active) {
		return true
	}
	for idx, key := range m.rowKeys {
		if !isContextHistoryRowKey(key) {
			continue
		}
		state.activeRow = idx
		return true
	}
	return false
}

func (m *contextHistoryTreeModel) MoveVertical(state *contextFeedbackViewState, delta int) bool {
	if state == nil || len(m.rowKeys) == 0 {
		return false
	}
	type historyRow struct {
		idx  int
		node contextHistoryNodeID
	}
	rows := make([]historyRow, 0, len(m.rowKeys))
	for idx, key := range m.rowKeys {
		node, ok := m.NodeForRowKey(key)
		if !ok {
			continue
		}
		rows = append(rows, historyRow{idx: idx, node: node})
	}
	if len(rows) == 0 {
		return false
	}
	current := -1
	for pos, row := range rows {
		if row.idx == state.activeRow {
			current = pos
			break
		}
	}
	if current == -1 {
		state.activeRow = rows[0].idx
		state.focusTextBox = false
		return true
	}
	currentNode := rows[current].node
	siblings := make([]int, 0, len(rows))
	for pos, row := range rows {
		if row.node.Kind() != currentNode.Kind() {
			continue
		}
		if !sharesParentIdentity(row.node, currentNode) {
			continue
		}
		siblings = append(siblings, pos)
	}
	if len(siblings) == 0 {
		return false
	}
	currentSibling := -1
	for pos, siblingPos := range siblings {
		if siblingPos == current {
			currentSibling = pos
			break
		}
	}
	if currentSibling == -1 {
		return false
	}
	next := currentSibling + delta
	if next < 0 {
		next = 0
	}
	if next >= len(siblings) {
		next = len(siblings) - 1
	}
	target := rows[siblings[next]].idx
	changed := state.activeRow != target
	state.activeRow = target
	if changed {
		state.focusTextBox = false
	}
	return changed
}

func sharesParentIdentity(a contextHistoryNodeID, b contextHistoryNodeID) bool {
	aParents := a.ParentKeys()
	bParents := b.ParentKeys()
	if len(aParents) == 0 && len(bParents) == 0 {
		return true
	}
	if len(aParents) == 0 || len(bParents) == 0 {
		return false
	}
	for _, parentA := range aParents {
		for _, parentB := range bParents {
			if parentA == parentB {
				return true
			}
		}
	}
	return false
}

func (m *contextHistoryTreeModel) MoveHorizontal(state *contextFeedbackViewState, direction string) bool {
	if state == nil {
		return false
	}
	current := strings.TrimSpace(state.activeRowKey())
	if current == "" {
		return false
	}
	if strings.HasPrefix(current, "session:") {
		if direction != "right" {
			return false
		}
		sessionRef := strings.TrimPrefix(current, "session:")
		prefix := "history:" + sessionRef + ":"
		for _, key := range m.rowKeys {
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
		node, ok := parseContextHistoryNode(current)
		if !ok {
			return false
		}
		if turn, ok := node.(contextHistoryNodeRef); ok {
			if turn.user != "" {
				return state.setActiveRowByKey(historySessionRowKey(turn.user, turn.session))
			}
			return state.setActiveRowByKey("session:" + turn.session)
		}
	}
	return false
}

func (m *contextHistoryTreeModel) TogglePeek(state *contextFeedbackViewState) (string, bool) {
	if state == nil {
		return "No expandable row.", false
	}
	rowKey := strings.TrimSpace(state.activeRowKey())
	if rowKey == "" {
		return "No expandable row.", false
	}
	if !isContextHistoryRowKey(rowKey) {
		return "Row has no expand/collapse action.", false
	}
	delete(state.selectedEntries, rowKey)
	id, ok := nodeIDForRowKey("context-history", rowKey)
	if !ok {
		return "Row has no expand/collapse action.", false
	}
	targetPath := focusPathForNode("context-history", id)
	if state.focusPath.HasFocus(id) && len(targetPath) > 1 {
		state.branchTransition(targetPath[:len(targetPath)-1])
		state.insideSection = true
		return "History node collapsed.", true
	}
	state.branchTransition(targetPath)
	state.insideSection = true
	return "History node expanded.", true
}

func (m *contextHistoryTreeModel) EnterSection(state *contextFeedbackViewState, forceExpand bool) {
	if state == nil {
		return
	}
	if forceExpand {
		state.collapsedContextHistory = false
	}
	state.insideSection = true
	m.EnsureRowFocus(state)
	if id, ok := nodeIDForRowKey("context-history", state.activeRowKey()); ok {
		state.setFocusPath(focusPathForNode("context-history", id))
		return
	}
	state.setFocusPath(focusPath{{Kind: nodeKindSection, Section: "context-history"}})
}

func (m *contextHistoryTreeModel) ExitSection(state *contextFeedbackViewState) {
	if state == nil {
		return
	}
	if len(state.focusPath) > 1 {
		state.popFocus()
	}
	state.collapsedContextHistory = true
	state.insideSection = false
}

func isContextHistoryRowKey(rowKey string) bool {
	key := strings.TrimSpace(rowKey)
	return strings.HasPrefix(key, "user:") || strings.HasPrefix(key, "session:") || strings.HasPrefix(key, "history:")
}

func parseContextHistoryNode(rowKey string) (contextHistoryNodeID, bool) {
	key := strings.TrimSpace(rowKey)
	if strings.HasPrefix(key, "user:") {
		return contextHistoryNodeRef{kind: contextHistoryNodeKindUser, user: strings.TrimPrefix(key, "user:")}, true
	}
	if strings.HasPrefix(key, "session:") {
		parts := strings.SplitN(strings.TrimPrefix(key, "session:"), ":", 2)
		if len(parts) == 2 {
			return contextHistoryNodeRef{kind: contextHistoryNodeKindSession, user: parts[0], session: parts[1]}, true
		}
		if len(parts) == 1 && strings.TrimSpace(parts[0]) != "" {
			return contextHistoryNodeRef{kind: contextHistoryNodeKindSession, session: parts[0]}, true
		}
		return contextHistoryNodeRef{}, false
	}
	if strings.HasPrefix(key, "history:") {
		parts := strings.Split(strings.TrimPrefix(key, "history:"), ":")
		if len(parts) == 3 {
			turn, ok := parseInt(parts[2])
			if !ok {
				return contextHistoryNodeRef{}, false
			}
			return contextHistoryNodeRef{kind: contextHistoryNodeKindTurn, user: parts[0], session: parts[1], turn: turn}, true
		}
		if len(parts) == 2 {
			turn, ok := parseInt(parts[1])
			if !ok {
				return contextHistoryNodeRef{}, false
			}
			return contextHistoryNodeRef{kind: contextHistoryNodeKindTurn, session: parts[0], turn: turn}, true
		}
	}
	return contextHistoryNodeRef{}, false
}

func parseInt(value string) (int, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, false
	}
	n := 0
	for _, r := range v {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = (n * 10) + int(r-'0')
	}
	return n, true
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	if value < 0 {
		return "-" + itoa(-value)
	}
	buf := [20]byte{}
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + (value % 10))
		value /= 10
	}
	return string(buf[i:])
}
