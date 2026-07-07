package decompose

import (
	"context"
	"testing"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
)

type decompFunc func(task.Record) branch.Result

func (f decompFunc) Decompose(_ context.Context, r task.Record) (branch.Result, error) {
	return f(r), nil
}

type execFunc func(task.Record) executor.Outcome

func (f execFunc) Execute(_ context.Context, r task.Record) executor.Outcome { return f(r) }

// TestDrainPlanCompound: a compound root (Kind: Step) decomposes into task leaves, all of
// which the scheduler drains to done via parent-as-join. This is the pipeline seam.
func TestDrainPlanCompound(t *testing.T) {
	root := task.Record{ID: "goal", Goal: "review the project", Type: task.Query, Kind: task.KindStep, Status: task.Proposed, Deps: []string{}}
	dec := decompFunc(func(task.Record) branch.Result {
		return branch.Result{Synthesis: "investigate then propose", Records: []task.Record{
			{ID: "l1", Goal: "read packages", Type: task.Query, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
			{ID: "l2", Goal: "pick sparsest", Type: task.Query, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
		}}
	})
	ex := execFunc(func(task.Record) executor.Outcome { return executor.Outcome{Status: executor.Executed} })

	out, err := DrainPlan(context.Background(), root, dec, ex, 4, 10, nil)
	if err != nil {
		t.Fatalf("DrainPlan: %v", err)
	}
	if len(out.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3 (root + 2 leaves)", len(out.Nodes))
	}
	for _, n := range out.Nodes {
		if n.Status != task.Done {
			t.Errorf("node %q status = %s, want done", n.ID, n.Status)
		}
	}
	if out.Root.ID != "goal" || out.Root.Status != task.Done {
		t.Errorf("root = %+v, want goal/done", out.Root)
	}
}
