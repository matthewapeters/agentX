package runtime

import (
	"testing"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/session"
	"agentx/internal/tools"
)

func TestPlanTreeRegistryDispatchDecomposeComplete(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)
	id, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	r := newPlanTreeRegistry()
	root := task.Record{ID: "task-1", Goal: "review the project", Kind: task.KindStep}
	r.dispatched("task-1", root, store, id.ID)

	children := []task.Record{
		{ID: "task-1-1", Goal: "list files", Kind: task.KindTask, Type: task.Command, Deps: nil,
			Provenance: task.Provenance{Source: "planner", Origin: "action"}},
		{ID: "task-1-2", Goal: "read docs", Kind: task.KindTask, Type: task.Query, Deps: []string{"task-1-1"},
			Provenance: task.Provenance{Source: "wavefront", Origin: "need"}},
	}
	r.decomposed("task-1", root, children, store, id.ID)

	tree := r.trees["task-1"]
	if tree == nil {
		t.Fatal("tree not created")
	}
	if tree.Goal != "review the project" {
		t.Errorf("tree.Goal = %q, want %q", tree.Goal, "review the project")
	}
	parent := tree.Nodes["task-1"]
	if parent == nil || parent.Status != "decomposed" {
		t.Fatalf("parent node = %+v, want status=decomposed", parent)
	}
	if len(parent.Children) != 2 || parent.Children[0] != "task-1-1" || parent.Children[1] != "task-1-2" {
		t.Errorf("parent.Children = %v, want [task-1-1 task-1-2]", parent.Children)
	}

	child1 := tree.Nodes["task-1-1"]
	if child1 == nil || len(child1.Deps) != 0 {
		t.Errorf("child1.Deps = %v, want empty (no sibling wait)", child1.Deps)
	}
	child2 := tree.Nodes["task-1-2"]
	if child2 == nil || len(child2.Deps) != 1 || child2.Deps[0] != "task-1-1" {
		t.Errorf("child2.Deps = %v, want [task-1-1] — sibling waits-on must not be conflated with Children", child2.Deps)
	}

	// Source/Origin mirror each child's own Provenance at decompose time — the
	// signal that makes a durable plan snapshot self-describing about which
	// engine produced a node and whether it's a chain-of-thought (step/action) or
	// tree-of-thought (know/need) move, not just the live event stream (ADR 0012).
	if child1.Source != "planner" || child1.Origin != "action" {
		t.Errorf("child1 Source/Origin = %q/%q, want planner/action", child1.Source, child1.Origin)
	}
	if child2.Source != "wavefront" || child2.Origin != "need" {
		t.Errorf("child2 Source/Origin = %q/%q, want wavefront/need", child2.Source, child2.Origin)
	}
	// The root carries no Provenance in this test (mirroring the continuous
	// engine's own plan root in production) — Source/Origin must stay empty
	// rather than default to some stray non-zero value.
	if parent.Source != "" || parent.Origin != "" {
		t.Errorf("root Source/Origin = %q/%q, want empty/empty", parent.Source, parent.Origin)
	}

	// A leaf's own dispatch must not re-parent it: Children/Deps set by decompose stay put.
	r.dispatched("task-1", children[0], store, id.ID)
	if tree.Nodes["task-1-1"].Status != "dispatched" {
		t.Errorf("child1 status after its own dispatch = %q, want dispatched", tree.Nodes["task-1-1"].Status)
	}

	r.completed("task-1", "task-1-1", task.Done, "42", "", store, id.ID)
	if got := tree.Nodes["task-1-1"].Status; got != string(task.Done) {
		t.Errorf("child1 status after completion = %q, want %q", got, task.Done)
	}
	if tree.Nodes["task-1-1"].CompletedAt == 0 {
		t.Error("child1 CompletedAt not set")
	}
	if got := tree.Nodes["task-1-1"].Value; got != "42" {
		t.Errorf("child1 Value after completion = %q, want %q", got, "42")
	}

	// The completed leaf's own Children/Deps from decompose must be untouched by NodeCompleted.
	if len(tree.Nodes["task-1-2"].Deps) != 1 {
		t.Errorf("unrelated sibling's Deps mutated by completed(): %v", tree.Nodes["task-1-2"].Deps)
	}
}

func TestPlanTreeRegistryToolEventsFoldIntoOwningNode(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)
	id, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	r := newPlanTreeRegistry()
	root := task.Record{ID: "task-1", Goal: "review the project", Kind: task.KindStep}
	r.dispatched("task-1", root, store, id.ID)
	leaf := task.Record{ID: "task-1-1", Goal: "list files", Kind: task.KindTask, Type: task.Command}
	r.decomposed("task-1", root, []task.Record{leaf}, store, id.ID)

	desc := tools.Descriptor{ID: "list_dir", Argv: []string{"ls", "-la"}}
	r.toolCalled(leaf, desc, map[string]string{}, store, id.ID)

	node := r.trees["task-1"].Nodes["task-1-1"]
	if node.Command == "" {
		t.Fatal("Command not set by toolCalled")
	}

	res := tools.Result{ToolID: "list_dir", Status: "ok", Ref: "artifacts/000000000001.txt", Preview: "total 0"}
	r.toolFinished(leaf, res, executor.Executed, store, id.ID)

	if node.ResultOutcome != string(executor.Executed) {
		t.Errorf("ResultOutcome = %q, want %q", node.ResultOutcome, executor.Executed)
	}
	if node.ResultRef != "artifacts/000000000001.txt" {
		t.Errorf("ResultRef = %q, want the tool result's ref", node.ResultRef)
	}
}

func TestPlanTreeRegistryToolEventForUnknownTaskIsANoop(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)
	id, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	r := newPlanTreeRegistry()
	// No dispatched/decomposed call for this task id — simulates an untagged, non-plan
	// tool call (the single_tool cycle) reaching the same taskToolPublisher.
	stray := task.Record{ID: "stray-1"}
	r.toolCalled(stray, tools.Descriptor{ID: "list_dir"}, nil, store, id.ID)
	r.toolFinished(stray, tools.Result{}, executor.Executed, store, id.ID)

	if len(r.trees) != 0 {
		t.Errorf("expected no tree created for an unowned task id, got %d trees", len(r.trees))
	}
}

func TestPlanTreeRegistryRootDegradesToTaskGetsOwnedForToolEvents(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)
	id, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	r := newPlanTreeRegistry()
	// Scheduler retry-then-degrade: a root Step that never decomposes, executing
	// directly as a Task instead. Only dispatched() ever fires for it.
	root := task.Record{ID: "task-1", Goal: "run ls -la", Kind: task.KindStep}
	r.dispatched("task-1", root, store, id.ID)

	r.toolCalled(root, tools.Descriptor{ID: "list_dir"}, nil, store, id.ID)
	node := r.trees["task-1"].Nodes["task-1"]
	if node.Command == "" {
		t.Fatal("degraded root's own tool_call did not fold in — ownerOf must self-register on dispatch")
	}
}

// GIVEN a leaf task registered under a plan root (via dispatched/decomposed)
// WHEN rootOf looks it up
// THEN it reports that root — the lookup RequestApproval's plan-scoped option
// depends on (docs/architecture/behavior/tool_policy_plan_scoped_approval.feature.md)
// — and reports ok=false for an id that was never registered, rather than
// panicking or silently returning a zero-value root.
func TestPlanTreeRegistryRootOf(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(dir)
	id, err := store.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	r := newPlanTreeRegistry()
	root := task.Record{ID: "task-1", Goal: "review the project", Kind: task.KindStep}
	r.dispatched("task-1", root, store, id.ID)
	children := []task.Record{
		{ID: "task-1-1", Goal: "list files", Kind: task.KindTask, Type: task.Command},
	}
	r.decomposed("task-1", root, children, store, id.ID)

	if got, ok := r.rootOf("task-1-1"); !ok || got != "task-1" {
		t.Errorf("rootOf(%q) = (%q, %v), want (%q, true)", "task-1-1", got, ok, "task-1")
	}
	if _, ok := r.rootOf("never-registered"); ok {
		t.Error("rootOf(unregistered id) reported ok=true, want false")
	}
}
