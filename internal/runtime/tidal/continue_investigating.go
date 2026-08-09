package tidal

import (
	"context"
	"errors"

	"agentx/internal/prompting/task"
	"agentx/internal/runtime/scheduler"
	"agentx/internal/runtime/wavefront"
)

// SnapshotFunc renders the five-section template from a post-run graph.
// Nil is allowed — the wrapper captures no snapshot in that case.
// Provided by ConsolidatorHook in phase 5 (uses tidal.Render internally).
// Zero-arg closure so the wrapper never reaches into the scheduler's private
// fields — the graph is captured in the surrounding scope.
type SnapshotFunc func() string

// Wrapper wraps an already-configured *wavefront.Scheduler. Zero-arg entry
// point for the model-facing native tool (parameter shaping is phase 6's
// concern). The scheduler must be fully wired before construction — this
// wrapper does not build or configure it. The graph reference is stored
// separately so countTerminal can read terminal nodes without reaching into
// the scheduler's private fields (preserving the "no new scheduler methods"
// constraint of this phase).
type Wrapper struct {
	scheduler  *wavefront.Scheduler
	graph      *task.Graph
	snapshotFn SnapshotFunc
}

// New builds a Wrapper around scheduler. snapshotFn may be nil. graph is the
// scheduler's underlying graph — stored separately so the wrapper can count
// terminal nodes without adding a Graph() accessor to wavefront.Scheduler.
func New(s *wavefront.Scheduler, graph *task.Graph, snapshotFn SnapshotFunc) *Wrapper {
	return &Wrapper{scheduler: s, graph: graph, snapshotFn: snapshotFn}
}

// Run drives the scheduler to a terminal state and returns structured status.
// Translates scheduler.Run()'s error into Status.RunStatus and optional Error
// text. Invokes snapshotFn (if non-nil) after the run completes.
//
// Error mapping:
//   - nil           ⇒ RunStatusDone
//   - scheduler.ErrStalled ⇒ RunStatusStalled
//   - anything else ⇒ RunStatusError with err.Error() in Status.Error
//
// Does not recover panics — panics propagate to the caller (the native-tool
// dispatcher, phase 6).
func (w *Wrapper) Run(ctx context.Context) Status {
	err := w.scheduler.Run(ctx)
	nodes := w.countTerminal()
	s := Status{NodesResolved: nodes}
	if err == nil {
		s.Status = RunStatusDone
	} else if errors.Is(err, scheduler.ErrStalled) {
		s.Status = RunStatusStalled
	} else {
		s.Status = RunStatusError
		s.Error = err.Error()
	}
	if w.snapshotFn != nil {
		s.Snapshot = w.snapshotFn()
	}
	return s
}

// countTerminal counts terminal nodes in g (Status ∈ {Done, Failed, Denied,
// Cancelled}). Used by Run to populate Status.NodesResolved.
//
// The wrapper holds graph separately from the scheduler to avoid needing a
// Graph() accessor on wavefront.Scheduler (preserving the "no new scheduler
// methods" constraint of this phase).
func (w *Wrapper) countTerminal() int {
	if w.graph == nil {
		return 0
	}
	count := 0
	for _, n := range w.graph.Nodes() {
		if n.Status == task.Done || n.Status == task.Failed ||
			n.Status == task.Denied || n.Status == task.Cancelled {
			count++
		}
	}
	return count
}
