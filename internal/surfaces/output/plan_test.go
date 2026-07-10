package output

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"

	"agentx/internal/state"
)

func planEv(ct state.ContentType, epoch int64, payload map[string]any) state.Event {
	return state.Event{Epoch: epoch, ContentType: ct, Payload: payload}
}

// renderedPlanBody renders a plan widget's current nested-box output as one string,
// for substring assertions — the widget's body is no longer baked at event time (it
// draws recursively at view time), so tests must render to see it (ADR 0009 §9c
// redesign).
func renderedPlanBody(m *Model, ps *planState) string {
	return strings.Join(m.renderWidget(ps.w, false), "\n")
}

// TestPlanWidgetLifecycle drives the mellow-meadow event shape through the widget: the
// entry exists BEFORE any execution, mutates in place per delta, shows per-step timing,
// and finalizes with blocked rows called out.
func TestPlanWidgetLifecycle(t *testing.T) {
	m := New()
	m.SetSize(100, 40)

	// Plan started: widget appears before anything runs.
	m.Apply(planEv(state.ContentTaskPlan, 1000, map[string]any{
		"root": "t", "goal": "review the project", "phase": "started",
		"nodes": []any{map[string]any{"task_id": "t", "goal": "review the project", "status": "proposed", "deps": []any{}}},
	}))
	ps := m.plans["t"]
	if ps == nil || len(m.widgets) == 0 {
		t.Fatal("no plan widget created at started snapshot")
	}
	if body := renderedPlanBody(m, ps); !strings.Contains(body, "• review the project") {
		t.Fatalf("pre-execution entry missing: %q", body)
	}
	widgets := len(m.widgets)

	// Root dispatched → running with live elapsed; then decomposed → children appear.
	m.Apply(planEv(state.ContentTaskNode, 2000, map[string]any{
		"root": "t", "task_id": "t", "event": "dispatched", "goal": "review the project", "depth": 0}))
	m.Apply(planEv(state.ContentTaskNode, 6000, map[string]any{
		"root": "t", "task_id": "t", "event": "decomposed", "children": []any{
			map[string]any{"task_id": "t-1", "goal": "list files", "deps": []any{}},
			map[string]any{"task_id": "t-2", "goal": "read docs", "deps": []any{}},
		}}))
	body := renderedPlanBody(m, ps)
	if !strings.Contains(body, "⑂ review the project") {
		t.Errorf("decompose cue missing: %q", body)
	}
	// Liveness: neither child has started yet, so the group stays auto-collapsed —
	// only the parent's own line shows (ADR 0009 §9c, user spec: auto-collapse
	// not-yet-begun steps).
	if strings.Contains(body, "list files") || strings.Contains(body, "read docs") {
		t.Errorf("not-yet-dispatched children must stay auto-collapsed: %q", body)
	}

	// Both children dispatched → parallel cue in the title, live elapsed on each,
	// and now visible (liveness expands the group while anything under it runs).
	m.Apply(planEv(state.ContentTaskNode, 7000, map[string]any{
		"root": "t", "task_id": "t-1", "event": "dispatched", "goal": "list files", "depth": 1}))
	m.Apply(planEv(state.ContentTaskNode, 8000, map[string]any{
		"root": "t", "task_id": "t-2", "event": "dispatched", "goal": "read docs", "depth": 1}))
	if !strings.Contains(ps.w.title, "2 running ∥") {
		t.Errorf("parallel cue missing from title: %q", ps.w.title)
	}
	body = renderedPlanBody(m, ps)
	// A running node's own spinner (independent per node, not the static ⏳ — ADR
	// 0009 §9c redesign) stands in for the glyph; its initial frame is deterministic
	// since nothing has ticked it yet in this synchronous test.
	if !strings.Contains(body, "list files (1.0s…)") {
		t.Errorf("live elapsed missing: %q", body)
	}
	if ps.nodes["t-1"].spin == nil {
		t.Error("running node should own a spinner")
	}

	// t-1 done (with duration): still visible, because its sibling t-2 is still
	// running — liveness propagates up to the shared parent, keeping the whole group
	// (including the now-finished t-1) expanded.
	m.Apply(planEv(state.ContentTaskNode, 9500, map[string]any{
		"root": "t", "task_id": "t-1", "event": "completed", "status": "done"}))
	body = renderedPlanBody(m, ps)
	if !strings.Contains(body, "✅ list files (2.5s)") {
		t.Errorf("done glyph/duration wrong (or sibling-still-running should keep it visible): %q", body)
	}

	// t-2 fails too: NOTHING under the root is live anymore, so the whole group
	// (t-1 and t-2 both) auto-collapses back out of view — only the root's own line
	// remains, per the user's liveness rule applied to a now-fully-quiet branch.
	m.Apply(planEv(state.ContentTaskNode, 12000, map[string]any{
		"root": "t", "task_id": "t-2", "event": "completed", "status": "failed"}))
	body = renderedPlanBody(m, ps)
	if strings.Contains(body, "list files") || strings.Contains(body, "read docs") {
		t.Errorf("fully-finished group must auto-collapse: %q", body)
	}

	m.Apply(planEv(state.ContentTaskPlan, 13000, map[string]any{
		"root": "t", "goal": "review the project", "phase": "ended", "executed": 2,
		"error": "plan blocked: 1 node never ran",
		"nodes": []any{
			map[string]any{"task_id": "t", "goal": "review the project", "status": "proposed", "deps": []any{}},
			map[string]any{"task_id": "t-1", "goal": "list files", "status": "done", "deps": []any{}},
			map[string]any{"task_id": "t-2", "goal": "read docs", "status": "failed", "deps": []any{}},
		}}))
	body = renderedPlanBody(m, ps)
	if !strings.Contains(body, "🚫 review the project") {
		t.Errorf("blocked cue missing on unfinished root: %q", body)
	}
	if !strings.Contains(body, "⚠ plan blocked") {
		t.Errorf("plan error line missing: %q", body)
	}
	if !strings.Contains(ps.w.title, "❌ 1/3 steps") {
		t.Errorf("final title wrong: %q", ps.w.title)
	}
	if len(m.widgets) != widgets {
		t.Errorf("widgets grew (%d → %d): deltas must mutate one widget, not append", widgets, len(m.widgets))
	}
}

// TestTaskCommandAndResultFolded: a tool event tagged with a plan step's task id folds
// into that Task node's own command/result fields — no separate widget is created (ADR
// 0009 §9c redesign, superseding the old flat nested-widget mechanism). An untagged
// tool event (single_tool cycle) still renders flat as before.
func TestTaskCommandAndResultFolded(t *testing.T) {
	m := New()
	m.SetSize(100, 40)
	m.Apply(planEv(state.ContentTaskPlan, 1, map[string]any{
		"root": "t", "goal": "review", "phase": "started",
		"nodes": []any{map[string]any{"task_id": "t", "goal": "review", "status": "proposed"}}}))
	m.Apply(planEv(state.ContentTaskNode, 2, map[string]any{
		"root": "t", "task_id": "t", "event": "decomposed", "kind": "step", "children": []any{
			map[string]any{"task_id": "t-1", "goal": "list files", "kind": "task", "deps": []any{}}}}))

	widgets := len(m.widgets)

	// Tagged call + result → fold into the node, no new widget.
	call := state.Event{Epoch: 3, ContentType: state.ContentToolCall, ToolName: "list_dir",
		Payload: map[string]any{"text": "list_dir path=.", "task_id": "t-1"}}
	m.Apply(call)
	res := state.Event{Epoch: 4, ContentType: state.ContentToolResult, ToolName: "list_dir",
		Payload: map[string]any{"text": "cmd/ docs/ internal/", "task_id": "t-1", "outcome": "executed"}}
	m.Apply(res)

	if len(m.widgets) != widgets {
		t.Errorf("widgets grew (%d → %d): tagged tool events must fold into the node, not append", widgets, len(m.widgets))
	}
	node := m.plans["t"].nodes["t-1"]
	if node.command != "list_dir path=." {
		t.Errorf("node.command = %q, want %q", node.command, "list_dir path=.")
	}
	if node.resultText != "cmd/ docs/ internal/" || node.resultOutcome != "executed" {
		t.Errorf("node result = (%q, %q), want (%q, %q)", node.resultText, node.resultOutcome, "cmd/ docs/ internal/", "executed")
	}

	// Untagged call → flat, unaffected.
	m.Apply(state.Event{Epoch: 5, ContentType: state.ContentToolCall, ToolName: "read_file",
		Payload: map[string]any{"text": "read_file path=go.mod"}})
	flat := m.widgets[len(m.widgets)-1]
	if flat.nested {
		t.Error("untagged tool call must render flat")
	}
	if flat.kind != kindToolCall || flat.body != "read_file path=go.mod" {
		t.Errorf("untagged widget wrong: kind=%v body=%q", flat.kind, flat.body)
	}
}

// TestPlanWidgetHappyPathTitle: a fully drained plan closes with the ✅ summary title.
func TestPlanWidgetHappyPathTitle(t *testing.T) {
	m := New()
	m.SetSize(100, 40)
	m.Apply(planEv(state.ContentTaskPlan, 1, map[string]any{
		"root": "p", "goal": "g", "phase": "started",
		"nodes": []any{map[string]any{"task_id": "p", "goal": "g", "status": "proposed"}}}))
	m.Apply(planEv(state.ContentTaskNode, 2, map[string]any{
		"root": "p", "task_id": "p", "event": "dispatched", "goal": "g", "depth": 0}))
	m.Apply(planEv(state.ContentTaskNode, 3, map[string]any{
		"root": "p", "task_id": "p", "event": "completed", "status": "done"}))
	m.Apply(planEv(state.ContentTaskPlan, 4, map[string]any{
		"root": "p", "goal": "g", "phase": "ended", "executed": 1,
		"nodes": []any{map[string]any{"task_id": "p", "goal": "g", "status": "done"}}}))
	ps := m.plans["p"]
	if !strings.Contains(ps.w.title, "✅ 1/1 steps") {
		t.Errorf("title = %q, want ✅ 1/1 steps", ps.w.title)
	}
}

// TestEndedPlanShowsFullStructure reproduces session brave-fjord-2: a real fast tool
// call (ls/tree) dispatches and completes in single-digit milliseconds — no terminal
// frame could ever render the brief "live" window — so without this, a finished plan
// permanently collapsed to just its root line with no way to see the individual steps
// (and no manual per-node expand to bring them back). Liveness must only bound
// clutter WHILE a plan runs; once it has ended, the full structure always shows —
// matching the widget's own documented job of being "the record of what ran."
func TestEndedPlanShowsFullStructure(t *testing.T) {
	m := New()
	m.SetSize(100, 40)
	m.Apply(planEv(state.ContentTaskPlan, 1, map[string]any{
		"root": "r", "goal": "review the project", "phase": "started",
		"nodes": []any{map[string]any{"task_id": "r", "goal": "review the project", "status": "proposed"}}}))
	m.Apply(planEv(state.ContentTaskNode, 1, map[string]any{
		"root": "r", "task_id": "r", "event": "dispatched", "goal": "review the project", "kind": "step", "depth": 0}))
	m.Apply(planEv(state.ContentTaskNode, 1, map[string]any{
		"root": "r", "task_id": "r", "event": "decomposed", "kind": "step", "children": []any{
			map[string]any{"task_id": "r-1", "goal": "get full project tree", "kind": "task", "deps": []any{}}}}))
	m.Apply(planEv(state.ContentTaskNode, 1, map[string]any{
		"root": "r", "task_id": "r-1", "event": "dispatched", "goal": "get full project tree", "kind": "task", "depth": 1}))
	// Same epoch as dispatch — exactly what brave-fjord-2's real tool_call/tool_result
	// showed (4ms wall clock, same millisecond epoch).
	m.Apply(state.Event{Epoch: 1, ContentType: state.ContentToolCall, ToolName: "tree",
		Payload: map[string]any{"text": "tree -L 3 -- .", "task_id": "r-1"}})
	m.Apply(state.Event{Epoch: 1, ContentType: state.ContentToolResult, ToolName: "tree",
		Payload: map[string]any{"text": "./go.mod\n./cmd/\n./internal/", "task_id": "r-1", "outcome": "executed"}})
	m.Apply(planEv(state.ContentTaskNode, 1, map[string]any{
		"root": "r", "task_id": "r-1", "event": "completed", "status": "done"}))
	m.Apply(planEv(state.ContentTaskNode, 1, map[string]any{
		"root": "r", "task_id": "r", "event": "completed", "status": "done"}))

	ps := m.plans["r"]
	// Before the plan's own "ended" snapshot arrives, the now fully-quiet tree is
	// correctly collapsed per the ordinary liveness rule (nothing running anywhere).
	body := renderedPlanBody(m, ps)
	if strings.Contains(body, "get full project tree") {
		t.Errorf("before 'ended', a fully-quiet tree should still collapse normally: %q", body)
	}

	m.Apply(planEv(state.ContentTaskPlan, 1, map[string]any{
		"root": "r", "goal": "review the project", "phase": "ended", "executed": 1,
		"nodes": []any{
			map[string]any{"task_id": "r", "goal": "review the project", "status": "done", "deps": []any{"r-1"}},
			map[string]any{"task_id": "r-1", "goal": "get full project tree", "status": "done", "deps": []any{}},
		}}))
	body = renderedPlanBody(m, ps)
	if !strings.Contains(body, "get full project tree") {
		t.Errorf("an ended plan must show its full structure, not just the root line: %q", body)
	}
	if !strings.Contains(body, "tree -L 3 -- .") {
		t.Errorf("an ended plan's Task command must be visible: %q", body)
	}
}

// TestPerNodeSpinnerLifecycle drives two concurrent dispatches and verifies each gets
// its own independently-routed, independently-ticking spinner (ADR 0009 §9c redesign
// — not the old shared/lockstep single spinner), and that neither leaks once its node
// completes (spinIndex must not grow unboundedly across a session's plans).
func TestPerNodeSpinnerLifecycle(t *testing.T) {
	m := New()
	m.SetSize(100, 40)
	m.Apply(planEv(state.ContentTaskPlan, 1, map[string]any{
		"root": "r", "goal": "g", "phase": "started",
		"nodes": []any{map[string]any{"task_id": "r", "goal": "g", "status": "proposed"}}}))
	m.Apply(planEv(state.ContentTaskNode, 2, map[string]any{
		"root": "r", "task_id": "r", "event": "decomposed", "kind": "step", "children": []any{
			map[string]any{"task_id": "r-1", "goal": "one", "kind": "task", "deps": []any{}},
			map[string]any{"task_id": "r-2", "goal": "two", "kind": "task", "deps": []any{}},
		}}))

	cmd1 := m.Apply(planEv(state.ContentTaskNode, 3, map[string]any{
		"root": "r", "task_id": "r-1", "event": "dispatched", "goal": "one", "kind": "task", "depth": 1}))
	cmd2 := m.Apply(planEv(state.ContentTaskNode, 4, map[string]any{
		"root": "r", "task_id": "r-2", "event": "dispatched", "goal": "two", "kind": "task", "depth": 1}))
	if cmd1 == nil || cmd2 == nil {
		t.Fatal("dispatching a node must return a non-nil Tick cmd")
	}
	if len(m.spinIndex) != 2 {
		t.Fatalf("spinIndex = %d entries, want 2", len(m.spinIndex))
	}
	n1, n2 := m.plans["r"].nodes["r-1"], m.plans["r"].nodes["r-2"]
	if n1.spin == nil || n2.spin == nil {
		t.Fatal("both dispatched nodes should own a spinner")
	}
	if n1.spin.ID() == n2.spin.ID() {
		t.Error("concurrent nodes must have distinct spinner IDs")
	}

	// A tick for r-1's spinner only updates r-1, not r-2, and routes through
	// Model.Update (not Apply) — mirroring how the real bubbletea runtime delivers
	// the Cmd's resulting message back in.
	before2 := n2.spin.View()
	tickCmd := m.Update(spinner.TickMsg{ID: n1.spin.ID(), Time: time.Now()})
	if tickCmd == nil {
		t.Error("a tick for a still-running node's spinner should return a continuation cmd")
	}
	if n2.spin.View() != before2 {
		t.Error("a tick for r-1 must not affect r-2's independent spinner")
	}

	// A stray tick for an ID nothing owns is a harmless no-op.
	if cmd := m.Update(spinner.TickMsg{ID: 999999, Time: time.Now()}); cmd != nil {
		t.Error("an unrecognized spinner ID must not return a continuation cmd")
	}

	// A zero-value spinner.TickMsg{} (ID 0 — never a real spinner.Tick(), only a
	// hand-built test fixture) must also be a safe no-op. Forwarding one of these
	// unguarded from chat.go into here reproduced a genuine hang under the full
	// godog @functional suite's concurrent scenario execution (isolated runs never
	// hit it) — this is the defensive guard on the receiving side, independent of
	// chat.go's own ID>0 check before it ever forwards.
	if cmd := m.Update(spinner.TickMsg{}); cmd != nil {
		t.Error("a zero-value spinner.TickMsg must not return a continuation cmd")
	}

	// Both complete: their spinners must be torn down, not left ticking forever.
	m.Apply(planEv(state.ContentTaskNode, 5, map[string]any{
		"root": "r", "task_id": "r-1", "event": "completed", "status": "done"}))
	m.Apply(planEv(state.ContentTaskNode, 6, map[string]any{
		"root": "r", "task_id": "r-2", "event": "completed", "status": "done"}))
	if len(m.spinIndex) != 0 {
		t.Errorf("spinIndex leaked %d entries after both nodes completed", len(m.spinIndex))
	}
	if n1.spin != nil || n2.spin != nil {
		t.Error("completed nodes must not still hold a spinner reference")
	}
}

// TestLivenessCollapse drives a two-level decompose (root → A,B; A → A1,A2) to verify
// the liveness rule as actually specified: a node's own children list is an
// ALL-OR-NOTHING gate on that node's own liveness (self-or-any-descendant running),
// not filtered per individual child — so a live grandchild expands the whole spine
// down to itself, including already-finished or not-yet-started siblings at each
// level (useful context, and still bounded: each of THOSE siblings' own deeper
// content stays independently collapsed). Once nothing anywhere is live, everything
// collapses uniformly to the root's own line — including mid-plan, between two
// bursts of activity, not just at the very end.
func TestLivenessCollapse(t *testing.T) {
	m := New()
	m.SetSize(100, 40)
	m.Apply(planEv(state.ContentTaskPlan, 1, map[string]any{
		"root": "r", "goal": "root goal", "phase": "started",
		"nodes": []any{map[string]any{"task_id": "r", "goal": "root goal", "status": "proposed"}}}))
	ps := m.plans["r"]

	// root → A (step), B (task). Neither has started: root itself isn't live yet, so
	// the whole group stays collapsed to just the root's own line.
	m.Apply(planEv(state.ContentTaskNode, 2, map[string]any{
		"root": "r", "task_id": "r", "event": "decomposed", "kind": "step", "children": []any{
			map[string]any{"task_id": "a", "goal": "step A", "kind": "step", "deps": []any{}},
			map[string]any{"task_id": "b", "goal": "task B", "kind": "task", "deps": []any{}},
		}}))
	body := renderedPlanBody(m, ps)
	if strings.Contains(body, "step A") || strings.Contains(body, "task B") {
		t.Errorf("nothing has started yet — root must stay collapsed to its own line: %q", body)
	}

	// B runs and finishes quickly, with nothing else yet started: root goes live
	// while B runs (A and B both become visible), then fully quiet again once B
	// finishes (A never having started) — the whole group collapses again, even
	// though the plan overall isn't done.
	m.Apply(planEv(state.ContentTaskNode, 3, map[string]any{
		"root": "r", "task_id": "b", "event": "dispatched", "goal": "task B", "kind": "task", "depth": 1}))
	body = renderedPlanBody(m, ps)
	if !strings.Contains(body, "task B") || !strings.Contains(body, "step A") {
		t.Errorf("root is live via running B — the whole child group should show: %q", body)
	}
	m.Apply(planEv(state.ContentTaskNode, 4, map[string]any{
		"root": "r", "task_id": "b", "event": "completed", "status": "done"}))
	body = renderedPlanBody(m, ps)
	if strings.Contains(body, "step A") || strings.Contains(body, "task B") {
		t.Errorf("quiet again mid-plan (B done, A not yet started) must re-collapse: %q", body)
	}

	// A → A1, A2 (both tasks); A1 dispatched (running), A2 still pending. Root is
	// live again (via A→A1), so its whole children group shows — including the
	// already-finished sibling B (useful context, bounded: B shows only its own
	// compact line, not any of its own deeper content) — and A's own children group
	// shows in turn (A is live via A1), including not-yet-started A2.
	m.Apply(planEv(state.ContentTaskNode, 5, map[string]any{
		"root": "r", "task_id": "a", "event": "decomposed", "kind": "step", "children": []any{
			map[string]any{"task_id": "a1", "goal": "grandchild one", "kind": "task", "deps": []any{}},
			map[string]any{"task_id": "a2", "goal": "grandchild two", "kind": "task", "deps": []any{}},
		}}))
	m.Apply(planEv(state.ContentTaskNode, 6, map[string]any{
		"root": "r", "task_id": "a1", "event": "dispatched", "goal": "grandchild one", "kind": "task", "depth": 2}))
	body = renderedPlanBody(m, ps)
	for _, want := range []string{"grandchild one", "step A", "task B", "grandchild two"} {
		if !strings.Contains(body, want) {
			t.Errorf("root live via a1 should expand the whole spine plus siblings at each level (missing %q): %q", want, body)
		}
	}

	// a1 finishes too: now nothing anywhere in the tree is live — the whole plan
	// collapses to just the root's own line.
	m.Apply(planEv(state.ContentTaskNode, 7, map[string]any{
		"root": "r", "task_id": "a1", "event": "completed", "status": "done"}))
	body = renderedPlanBody(m, ps)
	if strings.Contains(body, "step A") || strings.Contains(body, "grandchild one") {
		t.Errorf("fully-quiet tree must collapse to just the root line: %q", body)
	}
}

// TestDecomposedChildrenSurviveNativePayload reproduces the exact shape the bundled
// chat surface actually receives in production: plan_cycle.go builds task_node/
// task_plan payloads as literal []map[string]any/[]string, and the chat surface's
// bridge is a direct in-process channel with no JSON round-trip — so a plain
// v.([]any) assertion (what anySlice used before this fix) silently drops every
// element. A standalone HTTP/SSE surface would see the JSON-unmarshaled []any form
// instead; both must populate identically.
func TestDecomposedChildrenSurviveNativePayload(t *testing.T) {
	m := New()
	m.SetSize(100, 40)
	m.Apply(state.Event{Epoch: 1, ContentType: state.ContentTaskPlan, Payload: map[string]any{
		"root": "t", "goal": "review", "phase": "started",
		"nodes": []map[string]any{{"task_id": "t", "goal": "review", "status": "proposed", "deps": []string{}}},
	}})
	m.Apply(state.Event{Epoch: 2, ContentType: state.ContentTaskNode, Payload: map[string]any{
		"root": "t", "task_id": "t", "event": "decomposed", "kind": "step",
		"children": []map[string]any{
			{"task_id": "t-1", "goal": "list files", "kind": "task", "deps": []string{}},
			{"task_id": "t-2", "goal": "read docs", "kind": "task", "deps": []string{"t-1"}},
		},
	}})

	ps := m.plans["t"]
	parent := ps.nodes["t"]
	if len(parent.children) != 2 || parent.children[0] != "t-1" || parent.children[1] != "t-2" {
		t.Fatalf("parent.children = %v, want [t-1 t-2] — native []map[string]any payload not tolerated", parent.children)
	}
	child2 := ps.nodes["t-2"]
	if child2 == nil || len(child2.waitsOn) != 1 || child2.waitsOn[0] != "t-1" {
		t.Fatalf("child2.waitsOn = %v, want [t-1] — native []string deps not tolerated", child2.waitsOn)
	}
	if child2.kind != "task" || child2.parentID != "t" {
		t.Errorf("child2 kind/parentID = %q/%q, want task/t", child2.kind, child2.parentID)
	}

	// Liveness gates whether children render at all (auto-collapse when nothing is
	// running — a separate concern from this test's actual target, the native-payload
	// tolerance above); dispatch t-1 so the tree is live and the render assertion
	// below isolates what it's meant to check.
	m.Apply(state.Event{Epoch: 3, ContentType: state.ContentTaskNode, Payload: map[string]any{
		"root": "t", "task_id": "t-1", "event": "dispatched", "goal": "list files", "kind": "task", "depth": 1}})
	body := renderedPlanBody(m, ps)
	if !strings.Contains(body, "list files") {
		t.Errorf("children never appeared in the rendered body: %q", body)
	}
}
