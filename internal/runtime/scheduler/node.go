package scheduler

import (
	"context"
	"fmt"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
)

// Node is the read-only shape every DAG entry exposes, regardless of kind (ADR 0008
// amendment). A DAG node is always one of Step or Task — there is no bare Node value in
// production; the interface exists so code holding either can read common identity/
// structure without a type switch.
type Node interface {
	ID() string
	Goal() string
	Kind() task.Kind
	Deps() []string
	Depth() int
}

// Step is a plan node: it decomposes into at most 5 children (Step or Task) and never
// executes — there is no Execute method on this interface to call.
type Step interface {
	Node
	Decompose(ctx context.Context) (children []task.Record, synthesis string, err error)
}

// Task is a tool node: a leaf that never decomposes — there is no Decompose method on this
// interface to call. Results reads back the last Execute outcome (ok=false before Execute
// has run), so a later consumer (e.g. a WM-promotion step) can inspect what a Task produced
// without re-running it.
type Task interface {
	Node
	Execute(ctx context.Context) (executor.Outcome, error)
	Results() (executor.Outcome, bool)
}

// node is the common accessor implementation shared by stepNode/taskNode.
type node struct {
	rec   task.Record
	depth int
}

func (n node) ID() string      { return n.rec.ID }
func (n node) Goal() string    { return n.rec.Goal }
func (n node) Kind() task.Kind { return n.rec.Kind }
func (n node) Deps() []string  { return n.rec.Deps }
func (n node) Depth() int      { return n.depth }

// stepNode adapts a Decomposer collaborator to the Step interface for one record.
type stepNode struct {
	node
	dec Decomposer
}

// NewStep wraps rec as a Step, validating its declared Kind. dec is the collaborator that
// actually performs the decomposition (the same Decomposer the scheduler already uses).
// Not called by the scheduler's own dispatch loop (that stays a bare switch on rec.Kind,
// per the ADR's reasoning); this exists for code that needs to hold a typed node.
func NewStep(rec task.Record, depth int, dec Decomposer) (Step, error) {
	if rec.Kind != task.KindStep {
		return nil, fmt.Errorf("scheduler: NewStep: record %q has kind %q, want %q", rec.ID, rec.Kind, task.KindStep)
	}
	return stepNode{node: node{rec: rec, depth: depth}, dec: dec}, nil
}

// Decompose runs the wrapped Decomposer and unpacks its branch.Result into the Step
// interface's plain return shape.
func (s stepNode) Decompose(ctx context.Context) ([]task.Record, string, error) {
	res, err := s.dec.Decompose(ctx, s.rec)
	if err != nil {
		return nil, "", err
	}
	return res.Records, res.Synthesis, nil
}

// taskNode adapts an Executor collaborator to the Task interface for one record, caching
// its outcome so Results() can be read back after Execute without re-running.
type taskNode struct {
	node
	ex      Executor
	outcome executor.Outcome
	ran     bool
}

// NewTask wraps rec as a Task, validating its declared Kind. ex is the collaborator that
// actually runs the tool call (the same Executor the scheduler already uses). Not called
// by the scheduler's own dispatch loop; see NewStep.
func NewTask(rec task.Record, depth int, ex Executor) (Task, error) {
	if rec.Kind != task.KindTask {
		return nil, fmt.Errorf("scheduler: NewTask: record %q has kind %q, want %q", rec.ID, rec.Kind, task.KindTask)
	}
	return &taskNode{node: node{rec: rec, depth: depth}, ex: ex}, nil
}

// Execute runs the wrapped Executor once, caching the outcome for Results(). The Executor
// collaborator has no error return (its Outcome already encodes failure — Phantom/Denied/
// Failed/etc., exactly as the scheduler's own dispatch treats it), so this always returns a
// nil error; it exists on the interface for a future collaborator that might fail outright.
func (t *taskNode) Execute(ctx context.Context) (executor.Outcome, error) {
	t.outcome = t.ex.Execute(ctx, t.rec)
	t.ran = true
	return t.outcome, nil
}

// Results returns the last Execute outcome; ok is false until Execute has run.
func (t *taskNode) Results() (executor.Outcome, bool) {
	return t.outcome, t.ran
}
