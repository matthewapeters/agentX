package output

import (
	"fmt"
	"reflect"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"agentx/internal/state"
	"agentx/internal/surfaces/scrollutil"
)

// Plan widget (ADR 0009 §9c): one live, mutable widget per plan root. It appears the
// moment the plan starts — before anything executes — and task_node deltas mutate it in
// place as the scheduler drains, so the user watches steps dispatch, decompose, run in
// parallel, and finish (✅/❌/⊘), each with an elapsed time from the event epochs. The
// widget persists in the transcript after completion: it is the record of what ran.
//
// Timing note: durations are computed from server event epochs (dispatch → completion),
// not a surface clock, so they are truthful even over a slow transport. A running step's
// elapsed refreshes whenever any plan event arrives (no idle ticker).

// planNode is one row of the plan tree. kind is a raw wire string ("step"/"task"),
// matching status's existing untyped-string convention — keeps this package free of
// any import from internal/prompting/task for a value it only ever displays.
type planNode struct {
	id           string
	goal         string
	kind         string // "step" | "task"
	parentID     string
	depth        int
	status       string   // pending | running | decomposed | done | failed | abstained | blocked
	dispatchedAt int64    // epoch ms; 0 = never dispatched
	completedAt  int64    // epoch ms; 0 = not finished
	childCount   int      // set when this node decomposed
	children     []string // ordered child ids, from decompose (containment, not deps)
	waitsOn      []string // sibling ids this node waits on, captured at decompose time

	// A Task's resolved tool call and result, folded in from the executor's tagged
	// tool_call/tool_result events instead of a separate nested widget (ADR 0009 §9c
	// redesign) — command renders even when the node is collapsed; the result does not.
	command       string
	resultText    string
	resultOutcome string

	// spin is this node's own independent spinner, live only while status ==
	// "running" — each concurrently-running node animates on its own (not in
	// lockstep), routed by Model.spinIndex keyed on the spinner's own unique ID.
	spin *spinner.Model
}

// planState tracks a plan's nodes in display order plus its summary line.
type planState struct {
	rootID    string
	goal      string
	order     []string
	nodes     map[string]*planNode
	lastEpoch int64 // newest event epoch, the "now" for running elapsed
	ended     bool
	executed  int
	errText   string
	w         *widget
}

// applyPlanEvent folds a task_plan snapshot (phase "started" creates the widget before any
// execution; phase "ended" finalizes it, marking never-ran nodes blocked).
func (m *Model) applyPlanEvent(ev state.Event) {
	p, _ := ev.Payload.(map[string]any)
	root := str(p["root"])
	if root == "" {
		return
	}
	ps := m.plans[root]
	if ps == nil {
		ps = &planState{rootID: root, goal: str(p["goal"]), nodes: map[string]*planNode{},
			w: &widget{kind: kindPlan, planRoot: root, previewWhenCollapsed: true, collapsible: true}}
		m.plans[root] = ps
		m.add(ps.w)
	}
	ps.lastEpoch = ev.Epoch
	for _, n := range anySlice(p["nodes"]) {
		nm, _ := n.(map[string]any)
		id := str(nm["task_id"])
		if id == "" {
			continue
		}
		node := ps.ensure(id, str(nm["goal"]), str(nm["kind"]), 0)
		// Snapshot statuses refine what deltas already told us; never regress a
		// terminal glyph back to pending.
		switch str(nm["status"]) {
		case "done":
			node.status = "done"
		case "failed":
			node.status = "failed"
		case "abstained":
			node.status = "abstained"
		}
	}
	if str(p["phase"]) == "ended" || ps.ended {
		ps.ended = true
		ps.executed = intOf(p["executed"])
		ps.errText = str(p["error"])
		for _, id := range ps.order {
			n := ps.nodes[id]
			// pending never ran; running never reported; a decomposed parent whose
			// join never resolved (a child failed) is equally unfinished.
			if n.status == "pending" || n.status == "running" || n.status == "decomposed" {
				n.status = "blocked" // plan over, node never finished: say so loudly
				m.stopSpin(n)        // defensive: the plan ended, nothing should still be ticking
			}
		}
	}
	m.renderPlan(ps)
}

// applyNodeEvent folds a task_node delta (dispatched / decomposed / completed) and
// returns the tea.Cmd that starts a newly-dispatched node's own spinner ticking (nil
// for the other two cases — see stopSpin).
func (m *Model) applyNodeEvent(ev state.Event) tea.Cmd {
	p, _ := ev.Payload.(map[string]any)
	ps := m.plans[str(p["root"])]
	if ps == nil {
		return nil // delta without a started snapshot: nothing to attach to
	}
	ps.lastEpoch = ev.Epoch
	id := str(p["task_id"])
	var cmd tea.Cmd
	switch str(p["event"]) {
	case "dispatched":
		n := ps.ensure(id, str(p["goal"]), str(p["kind"]), intOf(p["depth"]))
		n.status = "running"
		n.dispatchedAt = ev.Epoch
		cmd = m.startSpin(n)
	case "decomposed":
		parent := ps.ensure(id, "", str(p["kind"]), 0)
		m.stopSpin(parent)
		children := anySlice(p["children"])
		parent.status = "decomposed"
		parent.childCount = len(children)
		childIDs := make([]string, 0, len(children))
		for _, c := range children {
			cm, _ := c.(map[string]any)
			cid := str(cm["task_id"])
			if cid == "" {
				continue
			}
			child := ps.ensure(cid, str(cm["goal"]), str(cm["kind"]), parent.depth+1)
			child.parentID = id
			for _, dep := range anySlice(cm["deps"]) {
				if ds, ok := dep.(string); ok {
					child.waitsOn = append(child.waitsOn, ds)
				}
			}
			childIDs = append(childIDs, cid)
		}
		parent.children = childIDs
	case "completed":
		n := ps.ensure(id, "", "", 0)
		m.stopSpin(n)
		switch str(p["status"]) {
		case "done":
			n.status = "done"
		case "abstained":
			n.status = "abstained"
		default:
			n.status = "failed"
		}
		n.completedAt = ev.Epoch
	}
	m.renderPlan(ps)
	return cmd
}

// startSpin creates n's own spinner and indexes it for Update's TickMsg routing,
// returning the Cmd that starts (and, via each returned continuation, sustains) its
// ticking.
func (m *Model) startSpin(n *planNode) tea.Cmd {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	n.spin = &sp
	m.spinIndex[sp.ID()] = n
	return sp.Tick
}

// stopSpin removes n's spinner from the routing index — called on decompose
// (a Step stops animating itself; its liveness now comes from its children) and on
// completion, so a finished node's Tick chain has nothing left to update and quietly
// ends rather than leaking forever.
func (m *Model) stopSpin(n *planNode) {
	if n.spin == nil {
		return
	}
	delete(m.spinIndex, n.spin.ID())
	n.spin = nil
}

// ensure returns the node, creating it in display order if new (goal/kind/depth fill
// in only when previously unknown, so a dispatched delta can enrich a snapshot row).
func (ps *planState) ensure(id, goal, kind string, depth int) *planNode {
	if n, ok := ps.nodes[id]; ok {
		if n.goal == "" {
			n.goal = goal
		}
		if n.kind == "" {
			n.kind = kind
		}
		if n.depth == 0 && depth > 0 {
			n.depth = depth
		}
		return n
	}
	n := &planNode{id: id, goal: goal, kind: kind, depth: depth, status: "pending"}
	ps.nodes[id] = n
	ps.order = append(ps.order, id)
	return n
}

// renderPlan rebuilds the widget's summary title from the plan state and refreshes.
// The body is no longer built here: a kindPlan widget draws its nested Step/Task
// boxes recursively at view time (renderPlanWidget), not baked into w.body at event
// time — a resize would otherwise leave stale fixed-width boxes (ADR 0009 §9c
// redesign).
func (m *Model) renderPlan(ps *planState) {
	running, doneN, total := 0, 0, 0
	for _, id := range ps.order {
		switch ps.nodes[id].status {
		case "running":
			running++
		case "done":
			doneN++
		}
		total++
	}

	switch {
	case ps.ended && ps.errText != "":
		ps.w.title = fmt.Sprintf("🗺 plan · ❌ %d/%d steps", doneN, total)
	case ps.ended:
		ps.w.title = fmt.Sprintf("🗺 plan · ✅ %d/%d steps", doneN, total)
	case running > 1:
		// The parallel cue: more than one step is in flight right now.
		ps.w.title = fmt.Sprintf("🗺 plan · %d/%d steps · %d running ∥", doneN, total, running)
	default:
		ps.w.title = fmt.Sprintf("🗺 plan · %d/%d steps", doneN, total)
	}
	m.refresh(true)
}

// Node box colors: kind carries the Step/Task distinction, running overrides both so
// an active node is unmistakable regardless of its kind. Selection (border heaviness)
// is orthogonal — see renderPlanWidget.
const (
	colorStep    = "38;5;75"  // blue
	colorTask    = "38;5;180" // tan
	colorRunning = "38;5;214" // amber, overrides kind
)

// nestMargin is the fixed left margin each recursion level reserves for a child box —
// the same 2-column convention ADR 0009 §9c's original flat nested-widget indent used.
const nestMargin = 2

// nodeBoxFloor is the minimum width at which a node still renders as a titled box;
// below it, drawNode falls back to a single flat glyph+goal line (mirrors
// renderWidget's own innerW < 1 guard, scaled up for a box's larger fixed overhead).
const nodeBoxFloor = 8

// nodeColor picks a node's border color: running always wins (it is the most
// actionable state to notice), otherwise kind differentiates Step from Task.
func nodeColor(n *planNode) string {
	if n.status == "running" {
		return colorRunning
	}
	if n.kind == "step" {
		return colorStep
	}
	return colorTask
}

// nodeTitle renders a node's own status line, plus a "waits on: …" annotation when it
// has sibling dependencies captured at decompose time (the tree's real DAG shape is a
// parent-as-join tree with no cross-branch edges, so this short annotation — not a
// literal graph edge — is enough to convey ordering within one Step's children).
func nodeTitle(n *planNode, now int64, ps *planState) string {
	g := glyph(n.status)
	if n.status == "running" && n.spin != nil {
		g = n.spin.View() // independent per-node animation, not a static ⏳
	}
	t := g + " " + n.goal + planTiming(n, now)
	if len(n.waitsOn) == 0 {
		return t
	}
	goals := make([]string, 0, len(n.waitsOn))
	for _, wid := range n.waitsOn {
		if wn := ps.nodes[wid]; wn != nil {
			goals = append(goals, wn.goal)
		}
	}
	if len(goals) == 0 {
		return t
	}
	return t + " · waits on: " + strings.Join(goals, ", ")
}

// maxResultLines caps how many lines of a Task's result render inline — a large
// result (e.g. a wide `tree` dump) would otherwise blow up the whole plan box; the
// full text is always in the persisted plan JSON (ADR 0009 §9b) for anything cut off.
const maxResultLines = 10

// renderPlanWidget draws a plan's whole nested DAG at view time (not cached into
// w.body — see renderPlan), so a terminal resize is correct for free. The outer box
// IS the root node's own box: its title is the widget's summary line (w.title,
// computed by renderPlan), color/heaviness come from the root the same way any nested
// node's would, and its content rows are the root's own drawNodeContent — there is no
// separate wrapping frame.
func (m *Model) renderPlanWidget(w *widget, selected bool) []string {
	ps := m.plans[w.planRoot]
	if ps == nil {
		return nil
	}
	root := ps.nodes[ps.rootID]
	if root == nil {
		return nil
	}
	outerW := m.contentWidth() - 2
	if outerW < 1 {
		return []string{scrollutil.TruncateWord(w.title, m.contentWidth())}
	}
	if w.collapsed {
		row := scrollutil.PadTo(scrollutil.TruncateWord(nodeTitle(root, ps.lastEpoch, ps), outerW), outerW)
		return m.boxifyStyled(w.title, []string{row}, outerW, nodeColor(root), selected)
	}
	live := ps.computeLiveness()
	if ps.ended {
		// Liveness only exists to bound clutter on a big plan WHILE it runs — once
		// the whole plan has ended there is nothing left to protect against, and the
		// widget's own documented job is to be "the record of what ran" (see the
		// package doc comment above). A real fast tool call (ls/tree/grep, the
		// overwhelming common case) completes in single-digit milliseconds — far
		// under one terminal frame — so without this, the liveness window is
		// effectively never observed and a finished plan permanently collapses to
		// just its root line with no way back (there is no manual per-node expand).
		// Confirmed live in session brave-fjord-2: every child dispatched→completed
		// in ~4ms, so the "live" state was never actually visible.
		for id := range ps.nodes {
			live[id] = true
		}
	}
	// The outer title is the plan's summary (progress counts), not the root's own
	// status line — so, for parity with every other node (whose own status is always
	// visible as its own box's title), the root's line is the first content row here.
	rows := []string{scrollutil.PadTo(scrollutil.TruncateWord(nodeTitle(root, ps.lastEpoch, ps), outerW), outerW)}
	rows = append(rows, m.drawNodeContent(root, ps, outerW, live)...)
	if ps.ended && ps.errText != "" {
		for _, l := range scrollutil.WrapLines("⚠ "+ps.errText, outerW) {
			rows = append(rows, scrollutil.PadTo(l, outerW))
		}
	}
	return m.boxifyStyled(w.title, rows, outerW, nodeColor(root), selected)
}

// drawNode returns id's complete boxed rendering — its own title/color/border — at
// exactly width display columns (borders included). Below nodeBoxFloor it degrades to
// a single flat line instead of a garbled, too-narrow box.
func (m *Model) drawNode(id string, ps *planState, width int, live map[string]bool) []string {
	n := ps.nodes[id]
	if n == nil {
		return nil
	}
	if width < nodeBoxFloor {
		return []string{scrollutil.PadTo(scrollutil.TruncateWord(glyph(n.status)+" "+n.goal, width), width)}
	}
	innerW := width - 2
	rows := m.drawNodeContent(n, ps, innerW, live)
	return m.boxifyStyled(nodeTitle(n, ps.lastEpoch, ps), rows, innerW, nodeColor(n), false)
}

// drawNodeContent returns id's content rows (no box of its own): a Task always shows
// its resolved command in reverse video, even collapsed — only the result is what
// collapsing hides; a Step shows each child recursively, in order, only while live
// (liveness-propagating auto-collapse — ADR 0009 §9c redesign, user spec).
func (m *Model) drawNodeContent(n *planNode, ps *planState, width int, live map[string]bool) []string {
	var rows []string
	expanded := live[n.id]

	if n.kind == "task" {
		cmd := n.command
		if cmd == "" {
			cmd = "(no command yet)"
		}
		rows = append(rows, scrollutil.PadTo(sgrCode+scrollutil.TruncateWord(cmd, width)+sgrReset, width))
		if expanded && n.resultText != "" {
			rows = append(rows, m.drawResultBox(n, width)...)
		}
	}

	if expanded {
		for _, cid := range n.children {
			child := ps.nodes[cid]
			if child == nil {
				continue
			}
			childW := width - nestMargin
			var box []string
			if childW < nodeBoxFloor {
				box = []string{scrollutil.PadTo(scrollutil.TruncateWord(glyph(child.status)+" "+child.goal, width), width)}
			} else {
				box = m.drawNode(cid, ps, childW, live)
				for i, l := range box {
					box[i] = scrollutil.PadTo(strings.Repeat(" ", nestMargin)+l, width)
				}
			}
			rows = append(rows, box...)
		}
	}
	return rows
}

// drawResultBox renders a Task's result as a nested box one level deeper — the
// collapsible sub-widget the user's spec asked for, now folded into the Task's own
// recursive rendering instead of a separate widget (ADR 0009 §9c redesign).
func (m *Model) drawResultBox(n *planNode, width int) []string {
	if width < nodeBoxFloor {
		return []string{scrollutil.PadTo(scrollutil.TruncateWord("📋 "+oneLine(n.resultText), width), width)}
	}
	innerW := width - 2
	lines := scrollutil.WrapLines(n.resultText, innerW)
	truncated := len(lines) > maxResultLines
	if truncated {
		lines = lines[:maxResultLines]
	}
	rows := make([]string, len(lines))
	for i, l := range lines {
		rows[i] = scrollutil.PadTo(l, innerW)
	}
	if truncated {
		rows = append(rows, scrollutil.PadTo(scrollutil.TruncateWord("… see plans/<root>.json for the full result", innerW), innerW))
	}
	title := "📋 result"
	if n.resultOutcome != "" {
		title += " · " + n.resultOutcome
	}
	box := m.boxifyStyled(title, rows, innerW, "", false)
	for i, l := range box {
		box[i] = scrollutil.PadTo(l, width)
	}
	return box
}

// computeLiveness reports, for every node, whether it or any descendant is currently
// running ("running" already covers a decomposing Step too — its status only flips to
// "decomposed" once NodeDecomposed lands, so the whole in-between window reads as
// running). A node's effective collapsed state is exactly !live[id] — recomputed fresh
// on every render (plans are small; no invalidation bookkeeping needed).
func (ps *planState) computeLiveness() map[string]bool {
	live := make(map[string]bool, len(ps.order))
	var walk func(id string) bool
	walk = func(id string) bool {
		if v, ok := live[id]; ok {
			return v
		}
		n := ps.nodes[id]
		if n == nil {
			return false
		}
		l := n.status == "running"
		for _, cid := range n.children {
			if walk(cid) {
				l = true
			}
		}
		live[id] = l
		return l
	}
	for _, id := range ps.order {
		walk(id)
	}
	return live
}

// glyph maps a node status to its visual cue (ADR 0009 reqs 5–6).
func glyph(status string) string {
	switch status {
	case "running":
		return "⏳"
	case "decomposed":
		return "⑂"
	case "done":
		return "✅"
	case "failed":
		return "❌"
	case "abstained":
		return "⊘"
	case "blocked":
		return "🚫"
	default: // pending
		return "•"
	}
}

// planTiming renders a step's elapsed time: final duration once completed, a live
// "so far" figure while running (against the newest event epoch).
func planTiming(n *planNode, now int64) string {
	switch {
	case n.dispatchedAt == 0:
		return ""
	case n.completedAt > 0:
		return " (" + fmtElapsed(n.completedAt-n.dispatchedAt) + ")"
	case now > n.dispatchedAt:
		return " (" + fmtElapsed(now-n.dispatchedAt) + "…)"
	default:
		return ""
	}
}

// fmtElapsed renders a millisecond duration compactly: "0.4s", "12.3s", "2m05s".
func fmtElapsed(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	s := float64(ms) / 1000
	if s < 60 {
		return fmt.Sprintf("%.1fs", s)
	}
	return fmt.Sprintf("%dm%02ds", int(s)/60, int(s)%60)
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// intOf tolerates the JSON number forms a payload int arrives in (float64 over the
// wire, int when applied in-process).
func intOf(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	}
	return 0
}

// anySlice tolerates a payload slice value in either representation the bus can
// deliver it in: []any (a payload that crossed a JSON boundary, e.g. reaching a
// standalone HTTP/SSE surface) or a concrete slice type (an in-process delivery — the
// bundled chat surface's bridge is a direct Go channel of state.Event with no JSON
// round-trip, and plan_cycle.go/classifier_pipeline.go build payloads as literal
// []map[string]any/[]string). A plain `v.([]any)` assertion only ever matches the
// first case and silently returns nil for the second — the bug this generalizes past
// (task_plan's bulk "nodes" list and task_node's "decomposed" children list were both
// silently empty in the live chat surface; only per-node "dispatched"/"completed"
// deltas, which never touch this helper, ever actually populated anything).
func anySlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out
}

// taskIDOf extracts the owning plan-step id a tool event was tagged with (executor call
// observer), or "" for untagged (single_tool cycle) events.
func taskIDOf(ev state.Event) string {
	p, _ := ev.Payload.(map[string]any)
	return str(p["task_id"])
}

// resultOutcome extracts the executor outcome ("executed" / "denied" / "phantom" …) a
// nested tool_result carries, for the result box title.
func resultOutcome(ev state.Event) string {
	p, _ := ev.Payload.(map[string]any)
	return str(p["outcome"])
}

// planFor returns the plan that owns a step id, or nil — how a tagged tool event finds
// the node to fold into.
func (m *Model) planFor(taskID string) *planState {
	for _, ps := range m.plans {
		if _, ok := ps.nodes[taskID]; ok {
			return ps
		}
	}
	return nil
}

// applyTaskToolEvent folds a task_id-tagged tool_call/tool_result event straight into
// the owning Task node's own fields — no separate widget is created (superseding the
// old flat nested-widget mechanism). The command renders inside the Task's own box
// regardless of collapse state; the result is the part collapse actually hides.
func (m *Model) applyTaskToolEvent(ev state.Event, isResult bool) {
	ps := m.planFor(taskIDOf(ev))
	if ps == nil {
		return
	}
	n := ps.nodes[taskIDOf(ev)]
	if isResult {
		n.resultText = eventText(ev)
		n.resultOutcome = resultOutcome(ev)
	} else {
		n.command = eventText(ev)
	}
	m.renderPlan(ps)
}
