// Package branch is the isolated planning context of ADR 0008 Phase 2: a fork of a
// parent session that a decomposer investigates within, returning only a plan.
//
// A branch inherits a read-only snapshot of the parent's enabled working memory,
// accumulates child task records in its own task.Graph, and records events to its own
// log — never the parent conversation. It is plan-only (Model I): the orchestrator
// wires its executor to a read-restricted catalog under working-directory confinement,
// so a branch can read to plan from evidence but cannot mutate. The branch's only
// output is a Result; MergeResult is the one path by which it changes the parent.
//
// This package is I/O-free, mirroring the Phase-1 task.Graph discipline. The executor
// wiring, WM persistence of grants, and event emission live in the orchestrator.
//
// Design: docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md.
// Behavior: docs/architecture/behavior/adr/0008_task_branch_context.feature.md.
package branch

import (
	"errors"
	"fmt"

	"agentx/internal/prompting/task"
	"agentx/internal/session"
	"agentx/internal/state"
)

// DefaultMaxDepth bounds recursion when config supplies none (prior-doc default).
const DefaultMaxDepth = 10

// ErrMaxDepth — forking would exceed the configured recursion bound.
var ErrMaxDepth = errors.New("branch: max task depth exceeded")

// Parent is the minimal surface a branch forks from.
type Parent struct {
	SessionID string
	Depth     int
	Facts     []session.Fact // enabled facts to snapshot (read-only inheritance)
}

// Result is a sealed branch's only output: the child task records and a synthesis.
type Result struct {
	Records   []task.Record
	Synthesis string
}

// Branch is an isolated planning context forked from a parent.
type Branch struct {
	parentID  string
	depth     int
	maxDepth  int
	facts     []session.Fact         // read-only snapshot of the parent's enabled WM
	plan      *task.Graph            // the child records accumulated here
	local     *session.WorkingMemory // branch-local scratch WM (never merged back)
	events    []state.Event          // isolated branch log (never the parent bus)
	synthesis string
}

// Fork creates a child branch from parent, snapshotting its enabled facts and sitting
// one level deeper. It refuses to fork at or beyond maxDepth (<=0 uses DefaultMaxDepth),
// so recursion is bounded; the caller routes the refusal to Ask per the ADR.
func Fork(parent Parent, maxDepth int) (*Branch, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	if parent.Depth+1 > maxDepth {
		return nil, fmt.Errorf("%w: depth %d exceeds %d", ErrMaxDepth, parent.Depth+1, maxDepth)
	}
	snap := make([]session.Fact, len(parent.Facts))
	copy(snap, parent.Facts)
	return &Branch{
		parentID: parent.SessionID,
		depth:    parent.Depth + 1,
		maxDepth: maxDepth,
		facts:    snap,
		plan:     task.NewGraph(),
		local:    &session.WorkingMemory{},
	}, nil
}

// Fork spawns a deeper child branch from this one, carrying the current WM snapshot
// forward and bounded by the same maxDepth.
func (b *Branch) Fork() (*Branch, error) {
	return Fork(Parent{SessionID: b.parentID, Depth: b.depth, Facts: b.facts}, b.maxDepth)
}

// Depth is how deep this branch sits below the root turn.
func (b *Branch) Depth() int { return b.depth }

// ParentID is the session the branch was forked from.
func (b *Branch) ParentID() string { return b.parentID }

// Facts returns a copy of the inherited read-only snapshot. There is no setter that
// writes through to the parent.
func (b *Branch) Facts() []session.Fact {
	out := make([]session.Fact, len(b.facts))
	copy(out, b.facts)
	return out
}

// Fact returns the snapshot value for key, if present and inherited.
func (b *Branch) Fact(key string) (session.Fact, bool) {
	for _, f := range b.facts {
		if f.Key == key {
			return f, true
		}
	}
	return session.Fact{}, false
}

// SetLocalFact records a fact in the branch-local scratch WM only; it never reaches
// the parent working memory.
func (b *Branch) SetLocalFact(key, value string) { b.local.Set(key, value) }

// LocalWM exposes the branch-local scratch WM (for the decomposer's own use).
func (b *Branch) LocalWM() *session.WorkingMemory { return b.local }

// Emit records an event to the branch's own log; it is never published to the parent
// conversation bus.
func (b *Branch) Emit(ev state.Event) { b.events = append(b.events, ev) }

// Events returns the branch's isolated log.
func (b *Branch) Events() []state.Event { return b.events }

// Add admits a child task record into the branch plan, with Phase-1 DAG integrity.
func (b *Branch) Add(rec task.Record) error { return b.plan.Add(rec) }

// Plan is the branch's accumulating task DAG.
func (b *Branch) Plan() *task.Graph { return b.plan }

// SetSynthesis records the compressed summary the parent will see on merge.
func (b *Branch) SetSynthesis(s string) { b.synthesis = s }

// Seal returns the branch's Result: its child records and synthesis, and nothing else.
func (b *Branch) Seal() Result {
	return Result{Records: b.plan.Nodes(), Synthesis: b.synthesis}
}

// MergeResult applies a sealed Result to the parent plan graph — the one path by which
// a branch changes the parent. Each child record is added to the parent DAG (skipping
// ids already present, so re-merge is idempotent). It returns the synthesis for the
// caller to record as a durable parent event; nothing else from the branch crosses back.
func MergeResult(parent *task.Graph, res Result) (string, error) {
	for _, rec := range res.Records {
		if _, ok := parent.Node(rec.ID); ok {
			continue
		}
		if err := parent.Add(rec); err != nil {
			return "", err
		}
	}
	return res.Synthesis, nil
}
