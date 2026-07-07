package decompose

import (
	"context"

	"agentx/internal/prompting/task"
	"agentx/internal/runtime/scheduler"
)

// DefaultSlots bounds concurrent scheduler dispatches. Clamped to the Ollama fan-out
// plateau; the classifier fan-out draws from the same budget in production.
const DefaultSlots = 4

// PlanOutcome is a drained decomposition plan: the root goal and every node's final state,
// ready to publish as task_plan / task_result events. It is the value the Phase-4d pipeline
// wiring turns into durable events.
type PlanOutcome struct {
	Root  task.Record
	Nodes []task.Record
}

// DrainPlan seeds a task DAG with a compound root and runs the scheduler to completion over
// the given oracle / decomposer / executor, returning the final node states. It is the pure
// bridge from the Decompose route to the scheduler — no bus, no I/O — so the wiring is
// unit-testable. slots < 1 and maxDepth are defaulted by scheduler.New.
func DrainPlan(ctx context.Context, root task.Record, oracle scheduler.Oracle, dec scheduler.Decomposer, ex scheduler.Executor, slots, maxDepth int) (PlanOutcome, error) {
	g := task.NewGraph()
	if err := g.Add(root); err != nil {
		return PlanOutcome{}, err
	}
	s := scheduler.New(g, oracle, dec, ex, slots, maxDepth)
	err := s.Run(ctx)

	out := PlanOutcome{Nodes: g.Nodes()}
	if r, ok := g.Node(root.ID); ok {
		out.Root = r
	}
	return out, err
}
