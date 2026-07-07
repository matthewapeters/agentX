package scheduler

import (
	"context"
	"testing"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
)

// TestTaskNodeHasNoDecompose proves the compile-time separation: a Task value's concrete
// type does not satisfy Step (no Decompose method exists to call), and Execute/Results
// behave correctly.
func TestTaskNodeHasNoDecompose(t *testing.T) {
	rec := task.Record{ID: "t1", Goal: "run it", Kind: task.KindTask, Status: task.Proposed, Deps: []string{}}
	tk, err := NewTask(rec, 2, okExec{})
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	if _, ok := any(tk).(Step); ok {
		t.Error("Task value unexpectedly satisfies Step — no compile-time separation")
	}
	if tk.ID() != "t1" || tk.Kind() != task.KindTask || tk.Depth() != 2 {
		t.Errorf("accessors wrong: id=%s kind=%s depth=%d", tk.ID(), tk.Kind(), tk.Depth())
	}
	if _, ok := tk.Results(); ok {
		t.Error("Results() ok=true before Execute ran")
	}
	out, err := tk.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Status != executor.Executed {
		t.Fatalf("Execute status = %s, want executed", out.Status)
	}
	got, ok := tk.Results()
	if !ok || got.Status != executor.Executed {
		t.Errorf("Results() after Execute = %+v, ok=%v", got, ok)
	}
}

// TestStepNodeHasNoExecute is the symmetric proof: a Step value's concrete type does not
// satisfy Task, and Decompose behaves correctly.
func TestStepNodeHasNoExecute(t *testing.T) {
	rec := task.Record{ID: "s1", Goal: "review the project", Kind: task.KindStep, Status: task.Proposed, Deps: []string{}}
	st, err := NewStep(rec, 0, stubDecomposerFn(func(context.Context, task.Record) (branch.Result, error) {
		return branch.Result{
			Records:   []task.Record{{ID: "s1-1", Goal: "leaf", Kind: task.KindTask}},
			Synthesis: "investigate then decide",
		}, nil
	}))
	if err != nil {
		t.Fatalf("NewStep: %v", err)
	}
	if _, ok := any(st).(Task); ok {
		t.Error("Step value unexpectedly satisfies Task — no compile-time separation")
	}
	if st.ID() != "s1" || st.Kind() != task.KindStep {
		t.Errorf("accessors wrong: id=%s kind=%s", st.ID(), st.Kind())
	}
	children, synthesis, err := st.Decompose(context.Background())
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if len(children) != 1 || children[0].ID != "s1-1" {
		t.Errorf("children = %+v, want one record s1-1", children)
	}
	if synthesis != "investigate then decide" {
		t.Errorf("synthesis = %q", synthesis)
	}
}

// NewTask/NewStep reject a record whose Kind doesn't match.
func TestNewTaskRejectsWrongKind(t *testing.T) {
	rec := task.Record{ID: "s1", Kind: task.KindStep}
	if _, err := NewTask(rec, 0, okExec{}); err == nil {
		t.Error("NewTask accepted a Step-kind record")
	}
}

func TestNewStepRejectsWrongKind(t *testing.T) {
	rec := task.Record{ID: "t1", Kind: task.KindTask}
	if _, err := NewStep(rec, 0, noDecomp{}); err == nil {
		t.Error("NewStep accepted a Task-kind record")
	}
}

// stubDecomposerFn adapts a func to the Decomposer interface for a one-off test case.
type stubDecomposerFn func(context.Context, task.Record) (branch.Result, error)

func (f stubDecomposerFn) Decompose(ctx context.Context, rec task.Record) (branch.Result, error) {
	return f(ctx, rec)
}
