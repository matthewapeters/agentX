package output

import (
	"fmt"
	"strings"

	"agentx/internal/state"
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

// planNode is one row of the plan tree.
type planNode struct {
	id           string
	goal         string
	depth        int
	status       string // pending | running | decomposed | done | failed | abstained | blocked
	dispatchedAt int64  // epoch ms; 0 = never dispatched
	completedAt  int64  // epoch ms; 0 = not finished
	childCount   int    // set when this node decomposed
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
			w: &widget{kind: kindPlan, previewWhenCollapsed: true}}
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
		node := ps.ensure(id, str(nm["goal"]), 0)
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
			}
		}
	}
	m.renderPlan(ps)
}

// applyNodeEvent folds a task_node delta (dispatched / decomposed / completed).
func (m *Model) applyNodeEvent(ev state.Event) {
	p, _ := ev.Payload.(map[string]any)
	ps := m.plans[str(p["root"])]
	if ps == nil {
		return // delta without a started snapshot: nothing to attach to
	}
	ps.lastEpoch = ev.Epoch
	id := str(p["task_id"])
	switch str(p["event"]) {
	case "dispatched":
		n := ps.ensure(id, str(p["goal"]), intOf(p["depth"]))
		n.status = "running"
		n.dispatchedAt = ev.Epoch
	case "decomposed":
		parent := ps.ensure(id, "", 0)
		children := anySlice(p["children"])
		parent.status = "decomposed"
		parent.childCount = len(children)
		for _, c := range children {
			cm, _ := c.(map[string]any)
			ps.ensure(str(cm["task_id"]), str(cm["goal"]), parent.depth+1)
		}
	case "completed":
		n := ps.ensure(id, "", 0)
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
}

// ensure returns the node, creating it in display order if new (goal/depth fill in only
// when previously unknown, so a dispatched delta can enrich a snapshot row).
func (ps *planState) ensure(id, goal string, depth int) *planNode {
	if n, ok := ps.nodes[id]; ok {
		if n.goal == "" {
			n.goal = goal
		}
		if n.depth == 0 && depth > 0 {
			n.depth = depth
		}
		return n
	}
	n := &planNode{id: id, goal: goal, depth: depth, status: "pending"}
	ps.nodes[id] = n
	ps.order = append(ps.order, id)
	return n
}

// renderPlan rebuilds the widget's title and body from the plan state and refreshes.
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

	var b strings.Builder
	for _, id := range ps.order {
		n := ps.nodes[id]
		fmt.Fprintf(&b, "%s%s %s%s\n", strings.Repeat("  ", n.depth), glyph(n.status),
			n.goal, planTiming(n, ps.lastEpoch))
	}
	if ps.ended && ps.errText != "" {
		fmt.Fprintf(&b, "⚠ %s\n", ps.errText)
	}
	ps.w.body = strings.TrimRight(b.String(), "\n")

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

func anySlice(v any) []any {
	s, _ := v.([]any)
	return s
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
// the widget to nest under.
func (m *Model) planFor(taskID string) *planState {
	for _, ps := range m.plans {
		if _, ok := ps.nodes[taskID]; ok {
			return ps
		}
	}
	return nil
}
