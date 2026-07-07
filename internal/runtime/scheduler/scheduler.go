// Package scheduler is the DAG scheduler of ADR 0008 Phase 3. It walks a live
// task.Graph and drives every node through one derived state machine — DONE / FAILED /
// BLOCKED / IN-FLIGHT / RUNNABLE / NEEDS-DECOMPOSE / NEEDS-CLARIFY — so that lazy
// decomposition and interleaved execution fall out of a single loop. Treating "not yet
// atomic" as just another not-ready state is what unifies the two.
//
// The scheduler is itself LLM-free: the atomicity test (Oracle), the decomposition
// (Decomposer), and the execution (Executor) are injected, so the orchestration logic
// is unit-testable with deterministic stubs. Real classifier/decomposer wiring is Phase 4.
//
// Concurrency model: continuous. Up to `slots` workers run as goroutines; the graph is
// mutated only on the main loop goroutine (workers report results over a channel), so the
// graph needs no lock. Decomposition uses parent-as-join: a decomposed node gains its
// children as dependencies and completes (Done) once they are all done, without executing.
//
// Termination is guaranteed without a timer: each node is dispatched at most once, so the
// loop either makes progress (a distinct node per dispatch, bounded by the finite,
// depth-capped node set) or returns ErrStalled; a caller may also bound Run via ctx.
//
// Design: docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md.
// Behavior: docs/architecture/behavior/adr/0008_task_scheduler.feature.md.
package scheduler

import (
	"context"
	"errors"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
)

// ErrStalled is returned when the scheduler would dispatch a node it has already
// dispatched. Every node is dispatched at most once (execute, or decompose-then-join), so
// a re-dispatch means a completion failed to move the node to a terminal/blocked state —
// a logic bug. Failing closed here makes an infinite loop impossible: the scheduler either
// makes progress (a new node each dispatch) or returns this error, never spins.
var ErrStalled = errors.New("scheduler: stalled — node re-dispatched without progress")

// ErrNoProgress is the contract error a Decomposer returns (wrapped) when a decomposition
// would not advance the plan — e.g. the model echoed the parent goal back as its only
// child. The scheduler then treats the node as atomic and executes it instead of recursing,
// which is what broke the tidy-cove spiral (ten re-plannings of the same ls -la).
var ErrNoProgress = errors.New("scheduler: decomposition made no progress")

// Observer receives plan lifecycle callbacks as the scheduler drains the graph. All
// callbacks fire on the main loop goroutine (never concurrently), immediately as each
// transition happens — the seam that makes execution user-visible while it runs
// (ADR 0009 §9a) instead of batch-reported at completion. A nil observer is a no-op.
type Observer interface {
	// NodeDispatched fires when a node is handed to a worker (before its oracle call).
	NodeDispatched(rec task.Record, depth int)
	// NodeDecomposed fires when a decomposition lands: parent became a join over children.
	NodeDecomposed(parent task.Record, children []task.Record)
	// NodeCompleted fires when a node reaches a terminal status (done/failed/abstained).
	NodeCompleted(id string, status task.Status)
}

// Option configures a Scheduler.
type Option func(*Scheduler)

// WithObserver attaches a lifecycle observer (nil is allowed and means no-op).
func WithObserver(obs Observer) Option {
	return func(s *Scheduler) { s.observer = obs }
}

// Oracle decides whether a task is atomic — it type-resolves AND is one-step (the ADR §2
// refinement). Classifier-backed in production; stubbed in tests.
type Oracle interface {
	Atomic(ctx context.Context, rec task.Record) (bool, error)
}

// Decomposer expands a non-atomic node into child records + a synthesis, in a branch.
type Decomposer interface {
	Decompose(ctx context.Context, rec task.Record) (branch.Result, error)
}

// Executor drains an atomic leaf into a verified effect (the existing executor seam).
type Executor interface {
	Execute(ctx context.Context, rec task.Record) executor.Outcome
}

// Scheduler runs a task.Graph to completion over its injected collaborators.
type Scheduler struct {
	graph      *task.Graph
	oracle     Oracle
	decomposer Decomposer
	executor   Executor
	slots      int
	maxDepth   int
	observer   Observer // nil ⇒ silent

	// observability, written only on the main loop goroutine.
	dispatchOrder []string
	peak          int
}

// New builds a scheduler. slots < 1 is clamped to 1. A negative maxDepth means "unset" and
// uses the branch default; maxDepth == 0 is a valid strict bound (depth-0 nodes may not
// decompose — they must be atomic or fail to Ask).
func New(g *task.Graph, o Oracle, d Decomposer, e Executor, slots, maxDepth int, opts ...Option) *Scheduler {
	if slots < 1 {
		slots = 1
	}
	if maxDepth < 0 {
		maxDepth = branch.DefaultMaxDepth
	}
	s := &Scheduler{graph: g, oracle: o, decomposer: d, executor: e, slots: slots, maxDepth: maxDepth}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// DispatchOrder returns the ids in the order the scheduler spawned work for them
// (deterministic: the main loop is single-threaded and reads the graph's first-seen order).
func (s *Scheduler) DispatchOrder() []string { return s.dispatchOrder }

// Peak is the maximum number of workers that were in flight at once (<= slots).
func (s *Scheduler) Peak() int { return s.peak }

type workKind int

const (
	doneExecute workKind = iota
	doneDecompose
	doneAsk
	doneError
)

type workDone struct {
	id     string
	kind   workKind
	status task.Status   // for doneExecute
	result branch.Result // for doneDecompose
}

// Run drives the graph to a terminal state: every node is done, failed, marked for
// clarification, or permanently blocked by a failed dependency. It returns when no node is
// runnable, needs decomposition, or is in flight.
func (s *Scheduler) Run(ctx context.Context) error {
	depth := map[string]int{}
	for _, n := range s.graph.Nodes() {
		depth[n.ID] = 0
	}
	inflight := map[string]bool{}
	decomposed := map[string]bool{}
	dispatched := map[string]bool{} // never cleared: a node is dispatched at most once
	// Buffered to slots so an in-flight worker can always report and exit even if Run
	// returns early on ctx cancellation — no leaked goroutine.
	done := make(chan workDone, s.slots)
	active := 0

	for {
		// Dispatch every dispatchable candidate up to the slot budget, resolving
		// decomposed joins inline (they complete without a slot). Repeat until a pass
		// makes no progress, so a join that unblocks new work is picked up immediately.
		for {
			progressed := false
			for _, id := range s.graph.Ready() {
				if inflight[id] {
					continue
				}
				if decomposed[id] {
					s.setStatus(id, task.Done) // join complete: children are all done
					progressed = true
					continue
				}
				if active >= s.slots {
					continue
				}
				if dispatched[id] {
					return ErrStalled // re-dispatch ⇒ a completion made no progress
				}
				rec, _ := s.graph.Node(id)
				inflight[id] = true
				dispatched[id] = true
				active++
				if active > s.peak {
					s.peak = active
				}
				s.dispatchOrder = append(s.dispatchOrder, id)
				if s.observer != nil {
					s.observer.NodeDispatched(rec, depth[id])
				}
				go s.work(ctx, rec, depth[id], done)
				progressed = true
			}
			if !progressed {
				break
			}
		}

		if active == 0 {
			return nil // nothing in flight and nothing dispatchable: terminal
		}

		var wd workDone
		select {
		case wd = <-done:
		case <-ctx.Done():
			return ctx.Err() // caller-bounded cancellation; buffered done drains workers
		}
		active--
		delete(inflight, wd.id)
		switch wd.kind {
		case doneExecute:
			s.setStatus(wd.id, wd.status)
		case doneDecompose:
			s.applyDecompose(wd, depth, decomposed)
		case doneAsk:
			s.setStatus(wd.id, task.Abstained)
		case doneError:
			s.setStatus(wd.id, task.Failed)
		}
	}
}

// work runs one candidate off the main loop: classify atomicity, then execute a leaf,
// decompose a non-atomic node, or (at max depth) fail to Ask. It reports back on done.
func (s *Scheduler) work(ctx context.Context, rec task.Record, d int, done chan<- workDone) {
	atomicNode, err := s.oracle.Atomic(ctx, rec)
	if err != nil {
		done <- workDone{id: rec.ID, kind: doneError}
		return
	}
	if atomicNode {
		out := s.executor.Execute(ctx, rec)
		st := task.Failed
		if out.Status == executor.Executed {
			st = task.Done
		}
		done <- workDone{id: rec.ID, kind: doneExecute, status: st}
		return
	}
	if d >= s.maxDepth {
		done <- workDone{id: rec.ID, kind: doneAsk} // bounded recursion → clarify
		return
	}
	res, err := s.decomposer.Decompose(ctx, rec)
	if errors.Is(err, ErrNoProgress) {
		// The decomposition would not advance the plan (echoed goal). The node is
		// effectively atomic: execute it rather than recurse — the anti-spiral fallback.
		out := s.executor.Execute(ctx, rec)
		st := task.Failed
		if out.Status == executor.Executed {
			st = task.Done
		}
		done <- workDone{id: rec.ID, kind: doneExecute, status: st}
		return
	}
	if err != nil {
		done <- workDone{id: rec.ID, kind: doneError}
		return
	}
	done <- workDone{id: rec.ID, kind: doneDecompose, result: res}
}

// applyDecompose merges a branch's children into the graph and makes the parent a join:
// it gains the children as dependencies and completes once they are all done (parent-as-
// join). Runs on the main loop, so graph access is single-threaded.
func (s *Scheduler) applyDecompose(wd workDone, depth map[string]int, decomposed map[string]bool) {
	var childIDs []string
	for _, c := range wd.result.Records {
		if _, exists := s.graph.Node(c.ID); exists {
			continue
		}
		if err := s.graph.Add(c); err != nil {
			s.setStatus(wd.id, task.Failed)
			return
		}
		depth[c.ID] = depth[wd.id] + 1
		childIDs = append(childIDs, c.ID)
	}
	parent, _ := s.graph.Node(wd.id)
	parent.Deps = append(parent.Deps, childIDs...)
	if err := s.graph.Update(parent); err != nil {
		s.setStatus(wd.id, task.Failed)
		return
	}
	decomposed[wd.id] = true
	if s.observer != nil {
		children := make([]task.Record, 0, len(childIDs))
		for _, id := range childIDs {
			if c, ok := s.graph.Node(id); ok {
				children = append(children, c)
			}
		}
		s.observer.NodeDecomposed(parent, children)
	}
}

// setStatus rewrites a node's status in place (deps unchanged, so Update re-validation is
// a no-op) and notifies the observer — every terminal transition flows through here.
// Main-loop only.
func (s *Scheduler) setStatus(id string, st task.Status) {
	rec, ok := s.graph.Node(id)
	if !ok {
		return
	}
	rec.Status = st
	_ = s.graph.Update(rec)
	if s.observer != nil {
		s.observer.NodeCompleted(id, st)
	}
}
