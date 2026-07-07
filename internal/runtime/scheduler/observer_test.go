package scheduler

import (
	"context"
	"fmt"
	"testing"

	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
)

// recObserver records lifecycle callbacks in order. Callbacks fire on the scheduler's main
// loop, so no locking is needed.
type recObserver struct{ events []string }

func (r *recObserver) NodeDispatched(rec task.Record, depth int) {
	r.events = append(r.events, fmt.Sprintf("dispatch:%s@%d", rec.ID, depth))
}
func (r *recObserver) NodeDecomposed(parent task.Record, children []task.Record) {
	r.events = append(r.events, fmt.Sprintf("decompose:%s→%d", parent.ID, len(children)))
}
func (r *recObserver) NodeCompleted(id string, status task.Status) {
	r.events = append(r.events, fmt.Sprintf("complete:%s=%s", id, status))
}

type oneSplit struct{}

func (oneSplit) Decompose(_ context.Context, rec task.Record) (branch.Result, error) {
	return branch.Result{Records: []task.Record{
		{ID: "root-1", Goal: "leaf one", Type: task.Query, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
		{ID: "root-2", Goal: "leaf two", Type: task.Query, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
	}}, nil
}

// TestObserverStreamsLifecycle: every transition is reported as it happens (ADR 0009 §9a) —
// the root dispatch, its decomposition, both leaf completions, and the root join.
func TestObserverStreamsLifecycle(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "root", Goal: "compound", Type: task.Query, Kind: task.KindStep, Status: task.Proposed, Deps: []string{}})
	obs := &recObserver{}
	s := New(g, oneSplit{}, okExec{}, 1, 10, WithObserver(obs))
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := map[string]bool{
		"dispatch:root@0": true, "decompose:root→2": true,
		"dispatch:root-1@1": true, "complete:root-1=done": true,
		"dispatch:root-2@1": true, "complete:root-2=done": true,
		"complete:root=done": true,
	}
	if len(obs.events) != len(want) {
		t.Fatalf("events = %v, want %d entries", obs.events, len(want))
	}
	for _, e := range obs.events {
		if !want[e] {
			t.Errorf("unexpected event %q in %v", e, obs.events)
		}
	}
	// Ordering invariants: dispatch precedes decompose precedes leaf activity; root join last.
	if obs.events[0] != "dispatch:root@0" || obs.events[1] != "decompose:root→2" {
		t.Errorf("head = %v, want dispatch:root then decompose:root", obs.events[:2])
	}
	if obs.events[len(obs.events)-1] != "complete:root=done" {
		t.Errorf("tail = %q, want complete:root=done (join last)", obs.events[len(obs.events)-1])
	}
}

// echoDecomp simulates the tidy-cove spiral trigger: the decomposer refuses (ErrNoProgress)
// because the plan echoed the parent goal.
type echoDecomp struct{}

func (echoDecomp) Decompose(context.Context, task.Record) (branch.Result, error) {
	return branch.Result{}, fmt.Errorf("child echoes parent: %w", ErrNoProgress)
}

// TestNoProgressExecutesInsteadOfRecursing: the anti-spiral fallback — a non-progress
// decomposition executes the node as a Task (Done), never recurses, never fails it.
func TestNoProgressExecutesInsteadOfRecursing(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{ID: "n", Goal: "run ls -la", Type: task.Query, Kind: task.KindStep, Status: task.Proposed, Deps: []string{}})
	s := New(g, echoDecomp{}, okExec{}, 1, 10)
	if err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n, _ := g.Node("n"); n.Status != task.Done {
		t.Fatalf("n status = %s, want done (executed via no-progress fallback)", n.Status)
	}
	if got := len(s.DispatchOrder()); got != 1 {
		t.Fatalf("dispatches = %d, want 1 (no recursion)", got)
	}
}
