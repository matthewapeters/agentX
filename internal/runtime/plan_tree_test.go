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
		{ID: "task-1-1", Goal: "list files", Kind: task.KindTask, Type: task.Command, Deps: nil},
		{ID: "task-1-2", Goal: "read docs", Kind: task.KindTask, Type: task.Query, Deps: []string{"task-1-1"}},
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

	// A leaf's own dispatch must not re-parent it: Children/Deps set by decompose stay put.
	r.dispatched("task-1", children[0], store, id.ID)
	if tree.Nodes["task-1-1"].Status != "dispatched" {
		t.Errorf("child1 status after its own dispatch = %q, want dispatched", tree.Nodes["task-1-1"].Status)
	}

	r.completed("task-1", "task-1-1", task.Done, store, id.ID)
	if got := tree.Nodes["task-1-1"].Status; got != string(task.Done) {
		t.Errorf("child1 status after completion = %q, want %q", got, task.Done)
	}
	if tree.Nodes["task-1-1"].CompletedAt == 0 {
		t.Error("child1 CompletedAt not set")
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
