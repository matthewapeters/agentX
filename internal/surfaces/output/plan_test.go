package output

import (
	"strings"
	"testing"

	"agentx/internal/state"
)

func planEv(ct state.ContentType, epoch int64, payload map[string]any) state.Event {
	return state.Event{Epoch: epoch, ContentType: ct, Payload: payload}
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
	if !strings.Contains(ps.w.body, "• review the project") {
		t.Fatalf("pre-execution entry missing: %q", ps.w.body)
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
	if !strings.Contains(ps.w.body, "⑂ review the project") {
		t.Errorf("decompose cue missing: %q", ps.w.body)
	}
	if !strings.Contains(ps.w.body, "  • list files") || !strings.Contains(ps.w.body, "  • read docs") {
		t.Errorf("children not indented under parent: %q", ps.w.body)
	}

	// Both children dispatched → parallel cue in the title, live elapsed on each.
	m.Apply(planEv(state.ContentTaskNode, 7000, map[string]any{
		"root": "t", "task_id": "t-1", "event": "dispatched", "goal": "list files", "depth": 1}))
	m.Apply(planEv(state.ContentTaskNode, 8000, map[string]any{
		"root": "t", "task_id": "t-2", "event": "dispatched", "goal": "read docs", "depth": 1}))
	if !strings.Contains(ps.w.title, "2 running ∥") {
		t.Errorf("parallel cue missing from title: %q", ps.w.title)
	}
	if !strings.Contains(ps.w.body, "⏳ list files (1.0s…)") {
		t.Errorf("live elapsed missing: %q", ps.w.body)
	}

	// t-1 done (with duration), t-2 fails; root completes via join... but here the plan
	// ends with t-2 failed, so the final snapshot marks the never-finished root blocked.
	m.Apply(planEv(state.ContentTaskNode, 9500, map[string]any{
		"root": "t", "task_id": "t-1", "event": "completed", "status": "done"}))
	m.Apply(planEv(state.ContentTaskNode, 12000, map[string]any{
		"root": "t", "task_id": "t-2", "event": "completed", "status": "failed"}))
	if !strings.Contains(ps.w.body, "✅ list files (2.5s)") {
		t.Errorf("done glyph/duration wrong: %q", ps.w.body)
	}
	if !strings.Contains(ps.w.body, "❌ read docs (4.0s)") {
		t.Errorf("failed glyph/duration wrong: %q", ps.w.body)
	}

	m.Apply(planEv(state.ContentTaskPlan, 13000, map[string]any{
		"root": "t", "goal": "review the project", "phase": "ended", "executed": 2,
		"error": "plan blocked: 1 node never ran",
		"nodes": []any{
			map[string]any{"task_id": "t", "goal": "review the project", "status": "proposed", "deps": []any{}},
			map[string]any{"task_id": "t-1", "goal": "list files", "status": "done", "deps": []any{}},
			map[string]any{"task_id": "t-2", "goal": "read docs", "status": "failed", "deps": []any{}},
		}}))
	if !strings.Contains(ps.w.body, "🚫 review the project") {
		t.Errorf("blocked cue missing on unfinished root: %q", ps.w.body)
	}
	if !strings.Contains(ps.w.body, "⚠ plan blocked") {
		t.Errorf("plan error line missing: %q", ps.w.body)
	}
	if !strings.Contains(ps.w.title, "❌ 1/3 steps") {
		t.Errorf("final title wrong: %q", ps.w.title)
	}
	if len(m.widgets) != widgets {
		t.Errorf("widgets grew (%d → %d): deltas must mutate one widget, not append", widgets, len(m.widgets))
	}
}

// TestNestedToolWidgets: a tool event tagged with a plan step's task id nests (indented
// box) under the plan; the result arrives collapsed with its outcome in the title. An
// untagged tool event (single_tool cycle) renders flat as before.
func TestNestedToolWidgets(t *testing.T) {
	m := New()
	m.SetSize(100, 40)
	m.Apply(planEv(state.ContentTaskPlan, 1, map[string]any{
		"root": "t", "goal": "review", "phase": "started",
		"nodes": []any{map[string]any{"task_id": "t", "goal": "review", "status": "proposed"}}}))
	m.Apply(planEv(state.ContentTaskNode, 2, map[string]any{
		"root": "t", "task_id": "t", "event": "decomposed", "children": []any{
			map[string]any{"task_id": "t-1", "goal": "list files", "deps": []any{}}}}))

	// Tagged call + result → nested.
	call := state.Event{Epoch: 3, ContentType: state.ContentToolCall, ToolName: "list_dir",
		Payload: map[string]any{"text": "list_dir path=.", "task_id": "t-1"}}
	m.Apply(call)
	res := state.Event{Epoch: 4, ContentType: state.ContentToolResult, ToolName: "list_dir",
		Payload: map[string]any{"text": "cmd/ docs/ internal/", "task_id": "t-1", "outcome": "executed"}}
	m.Apply(res)

	callW := m.widgets[len(m.widgets)-2]
	resW := m.widgets[len(m.widgets)-1]
	if !callW.nested || !strings.Contains(callW.title, "🔧 list_dir · t-1") {
		t.Errorf("call widget not nested/titled: nested=%v title=%q", callW.nested, callW.title)
	}
	if !resW.nested || !resW.collapsed || !strings.Contains(resW.title, "📋 result · executed") {
		t.Errorf("result widget wrong: nested=%v collapsed=%v title=%q", resW.nested, resW.collapsed, resW.title)
	}
	// Nested boxes render indented two columns.
	lines := m.renderWidget(callW, false)
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "  ") {
		t.Errorf("nested widget not indented: %q", lines[0])
	}

	// Untagged call → flat.
	m.Apply(state.Event{Epoch: 5, ContentType: state.ContentToolCall, ToolName: "read_file",
		Payload: map[string]any{"text": "read_file path=go.mod"}})
	flat := m.widgets[len(m.widgets)-1]
	if flat.nested {
		t.Error("untagged tool call must render flat")
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
