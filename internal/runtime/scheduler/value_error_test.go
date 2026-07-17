package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
	"agentx/internal/tools"
)

// runAndWait drives s.Run to completion (or fails the test after a generous
// failsafe timeout — not part of the scheduler's own guarantee, which needs no
// timer at all per guard_test.go).
func runAndWait(t *testing.T, s *Scheduler) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not terminate")
		return nil
	}
}

type outcomeExec struct{ out executor.Outcome }

func (e outcomeExec) Execute(context.Context, task.Record) executor.Outcome { return e.out }

// TestExecuteSuccessSetsValue: a Task that executes successfully carries the
// executor's result preview as Record.Value, with Error left empty.
func TestExecuteSuccessSetsValue(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "a", Kind: task.KindTask, Status: task.Proposed})
	exec := outcomeExec{out: executor.Outcome{Status: executor.Executed, Result: tools.Result{Preview: "resolved value text"}}}
	s := New(g, noDecomp{}, exec, 4, 10)

	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := g.Node("a")
	if got.Status != task.Done {
		t.Fatalf("Status = %s, want Done", got.Status)
	}
	if got.Value != "resolved value text" {
		t.Errorf("Value = %q, want the executor's result preview", got.Value)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty on success", got.Error)
	}
}

// TestExecuteFailureSetsError: a Task whose execution fails carries the executor's
// Reason as Record.Error, with Value left empty.
func TestExecuteFailureSetsError(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "a", Kind: task.KindTask, Status: task.Proposed})
	exec := outcomeExec{out: executor.Outcome{Status: executor.Failed, Reason: "tool exited non-zero"}}
	s := New(g, noDecomp{}, exec, 4, 10)

	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := g.Node("a")
	if got.Status != task.Failed {
		t.Fatalf("Status = %s, want Failed", got.Status)
	}
	if got.Error != "tool exited non-zero" {
		t.Errorf("Error = %q, want the executor's Reason", got.Error)
	}
	if got.Value != "" {
		t.Errorf("Value = %q, want empty on failure", got.Value)
	}
}

// TestExecuteDeniedCarriesNeitherValueNorError: a deliberate, documented scoping
// choice — Denied is a policy/user decision, not a Failed transition, and does not
// (yet) get its reason written onto the durable record.
func TestExecuteDeniedCarriesNeitherValueNorError(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "a", Kind: task.KindTask, Status: task.Proposed})
	exec := outcomeExec{out: executor.Outcome{Status: executor.Denied, Reason: "outside cwd"}}
	s := New(g, noDecomp{}, exec, 4, 10)

	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := g.Node("a")
	if got.Status != task.Denied {
		t.Fatalf("Status = %s, want Denied", got.Status)
	}
	if got.Value != "" || got.Error != "" {
		t.Errorf("Value=%q Error=%q, want both empty for Denied", got.Value, got.Error)
	}
}

type decomposeErrFn func(context.Context, task.Record) (branch.Result, error)

func (f decomposeErrFn) Decompose(ctx context.Context, rec task.Record) (branch.Result, error) {
	return f(ctx, rec)
}

// TestDecomposeErrorSetsError: a Step whose Decompose call returns a real error
// (not ErrNoProgress) fails with that error's text on Record.Error.
func TestDecomposeErrorSetsError(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "a", Kind: task.KindStep, Status: task.Proposed})
	failDecomp := decomposeErrFn(func(context.Context, task.Record) (branch.Result, error) {
		return branch.Result{}, errors.New("branch fork failed")
	})
	s := New(g, failDecomp, okExec{}, 4, 10)

	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := g.Node("a")
	if got.Status != task.Failed {
		t.Fatalf("Status = %s, want Failed", got.Status)
	}
	if !strings.Contains(got.Error, "branch fork failed") {
		t.Errorf("Error = %q, want it to contain the decompose error", got.Error)
	}
}

// TestApplyDecomposeAddFailureSetsError: a child with a dangling dependency fails
// graph.Add inside applyDecompose — the discarded error (previously silent) now
// lands on the parent's Record.Error.
func TestApplyDecomposeAddFailureSetsError(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "parent", Kind: task.KindStep, Status: task.Proposed})
	badChild := decomposeErrFn(func(context.Context, task.Record) (branch.Result, error) {
		return branch.Result{Records: []task.Record{
			{ID: "child1", Kind: task.KindTask, Status: task.Proposed, Deps: []string{"does-not-exist"}},
		}}, nil
	})
	s := New(g, badChild, okExec{}, 4, 10)

	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := g.Node("parent")
	if got.Status != task.Failed {
		t.Fatalf("Status = %s, want Failed", got.Status)
	}
	if !strings.Contains(got.Error, "dangling dependency") {
		t.Errorf("Error = %q, want it to contain the graph.Add integrity error", got.Error)
	}
}

// TestApplyDecomposeUpdateFailureSetsError: a child that (via its own stored deps)
// closes a cycle only once the parent is updated to depend on it — graph.Update's
// discarded error now lands on the parent's Record.Error.
func TestApplyDecomposeUpdateFailureSetsError(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "parent", Kind: task.KindStep, Status: task.Proposed})
	cyclicChild := decomposeErrFn(func(context.Context, task.Record) (branch.Result, error) {
		return branch.Result{Records: []task.Record{
			// child1 depends on parent (valid on its own — parent has no deps yet);
			// once applyDecompose appends child1 as a dep OF parent, parent -> child1
			// -> parent closes a cycle, which only graph.Update's re-validation catches.
			{ID: "child1", Kind: task.KindTask, Status: task.Proposed, Deps: []string{"parent"}},
		}}, nil
	})
	s := New(g, cyclicChild, okExec{}, 4, 10)

	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := g.Node("parent")
	if got.Status != task.Failed {
		t.Fatalf("Status = %s, want Failed", got.Status)
	}
	if !strings.Contains(got.Error, "dependency cycle") {
		t.Errorf("Error = %q, want it to contain the graph.Update cycle error", got.Error)
	}
}

// TestInvalidKindSetsError: a node reaching the scheduler with no valid declared
// Kind fails loudly with a descriptive Error instead of a silent one.
func TestInvalidKindSetsError(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "a", Kind: task.Kind("bogus"), Status: task.Proposed})
	s := New(g, noDecomp{}, okExec{}, 4, 10)

	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := g.Node("a")
	if got.Status != task.Failed {
		t.Fatalf("Status = %s, want Failed", got.Status)
	}
	if !strings.Contains(got.Error, "no valid Kind") {
		t.Errorf("Error = %q, want it to describe the invalid Kind", got.Error)
	}
}

// TestJoinCompleteAndAbstainedLeaveValueAndErrorEmpty: neither a parent-as-join
// completion nor a bounded-recursion Abstain has anything to report yet — setStatus
// must not invent placeholder text for either.
func TestJoinCompleteAndAbstainedLeaveValueAndErrorEmpty(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "parent", Kind: task.KindStep, Status: task.Proposed})
	oneChild := decomposeErrFn(func(context.Context, task.Record) (branch.Result, error) {
		return branch.Result{Records: []task.Record{
			{ID: "child1", Kind: task.KindTask, Status: task.Proposed},
		}}, nil
	})
	s := New(g, oneChild, okExec{}, 4, 10)
	if err := runAndWait(t, s); err != nil {
		t.Fatalf("Run: %v", err)
	}
	parent, _ := g.Node("parent")
	if parent.Status != task.Done {
		t.Fatalf("parent Status = %s, want Done (join complete)", parent.Status)
	}
	if parent.Value != "" || parent.Error != "" {
		t.Errorf("parent Value=%q Error=%q, want both empty on join completion", parent.Value, parent.Error)
	}

	// Abstained: a Step at max depth fails to Ask rather than recurse.
	g2 := task.NewGraph()
	_ = g2.Add(task.Record{ID: "s", Kind: task.KindStep, Status: task.Proposed})
	s2 := New(g2, noDecomp{}, okExec{}, 4, 0) // maxDepth 0: a depth-0 Step can't decompose
	if err := runAndWait(t, s2); err != nil {
		t.Fatalf("Run: %v", err)
	}
	abstained, _ := g2.Node("s")
	if abstained.Status != task.Abstained {
		t.Fatalf("Status = %s, want Abstained", abstained.Status)
	}
	if abstained.Value != "" || abstained.Error != "" {
		t.Errorf("Value=%q Error=%q, want both empty for Abstained", abstained.Value, abstained.Error)
	}
}
