package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
)

type okExec struct{}

func (okExec) Execute(context.Context, task.Record) executor.Outcome {
	return executor.Outcome{Status: executor.Executed}
}

type noDecomp struct{}

func (noDecomp) Decompose(context.Context, task.Record) (branch.Result, error) {
	return branch.Result{}, nil
}

// TestRunTerminatesWithoutTimer is the anti-freeze guarantee: Run completes on its own —
// no wall-clock timer, no watchdog — because each node is dispatched at most once. The
// outer timeout here is only the test's own failsafe, not part of the scheduler.
func TestRunTerminatesWithoutTimer(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "a", Goal: "a", Type: task.Query, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}})
	s := New(g, noDecomp{}, okExec{}, 4, 10)

	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not terminate")
	}
	if n, _ := g.Node("a"); n.Status != task.Done {
		t.Fatalf("a status = %s, want done", n.Status)
	}
}

// TestCtxCancelStops proves a caller can bound Run via context without a timer inside.
func TestCtxCancelStops(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "a", Goal: "a", Type: task.Query, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	s := New(g, noDecomp{}, okExec{}, 4, 10)
	// With an already-cancelled ctx, Run must return promptly (either done or ctx.Err()).
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected err %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run ignored ctx cancellation")
	}
}
