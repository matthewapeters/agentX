package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
	"agentx/internal/runtime/decompose"
	"agentx/internal/state"
)

// planReady reports whether the decomposition substrate is wired (classifier oracle,
// branch decomposer, and executor). buildDecomposition sets these at Start.
func (o *Orchestrator) planReady() bool {
	return o.taskOracle != nil && o.taskDecomp != nil && o.taskExec != nil
}

// capturingExec wraps the task executor to record every leaf outcome as the scheduler
// drains a plan, so the plan cycle can fold the real findings into the answer. The
// scheduler runs Execute on worker goroutines, so it locks.
type capturingExec struct {
	inner taskExecutor
	mu    sync.Mutex
	steps []capturedStep
}

type capturedStep struct {
	goal    string
	outcome executor.Outcome
}

func (c *capturingExec) Execute(ctx context.Context, rec task.Record) executor.Outcome {
	out := c.inner.Execute(ctx, rec)
	c.mu.Lock()
	c.steps = append(c.steps, capturedStep{goal: rec.Goal, outcome: out})
	c.mu.Unlock()
	return out
}

// runPlanPhase drains an imperative through the decomposition scheduler before the model
// answers: it decomposes the goal into a plan, executes the leaves (real read tools), and
// returns the plan + findings as context to ground the response. This is the wiring that
// makes the invoke_planner route actually act instead of free-forming. It returns
// handled=false (fall through to a direct answer) when nothing was investigated.
func (o *Orchestrator) runPlanPhase(ctx context.Context, text string, userOrd uint64) (string, bool, error) {
	o.setProcessing(state.StateWorking, state.PhaseTool)

	root := task.Record{
		ID:     fmt.Sprintf("task-%d", userOrd),
		Goal:   text,
		Type:   task.Query,
		Status: task.Proposed,
		Deps:   []string{},
	}
	cap := &capturingExec{inner: o.taskExec}
	out, derr := decompose.DrainPlan(ctx, root, o.taskOracle, o.taskDecomp, cap, decompose.DefaultSlots, branch.DefaultMaxDepth)
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}

	o.publishPlan(root, out, derr)
	planCtx := planContext(cap.steps)
	if strings.TrimSpace(planCtx) == "" {
		return "", false, nil // nothing investigated → answer directly
	}
	return planCtx, true, nil
}

// publishPlan records the drained plan (node goals, deps, final statuses) as a task_plan
// event and each executed leaf as a task_result, so the plan is durable and legible.
func (o *Orchestrator) publishPlan(root task.Record, out decompose.PlanOutcome, derr error) {
	nodes := make([]map[string]any, 0, len(out.Nodes))
	for _, n := range out.Nodes {
		nodes = append(nodes, map[string]any{
			"task_id": n.ID, "goal": n.Goal, "status": string(n.Status), "deps": n.Deps,
		})
		if n.ID != root.ID {
			o.publish("TASK_RESULT", state.ContentTaskResult, map[string]any{
				"task_id": n.ID, "route": "invoke_planner",
				"status": string(n.Status), "verified": n.Status == task.Done,
			})
		}
	}
	payload := map[string]any{"root": root.ID, "goal": root.Goal, "nodes": nodes}
	if derr != nil {
		payload["error"] = derr.Error()
	}
	o.publish("TASK_PLAN", state.ContentTaskPlan, payload)
}

// planContext renders the executed steps and their findings as prompt context, so the
// model answers grounded in what the tools actually read rather than assumptions.
func planContext(steps []capturedStep) string {
	if len(steps) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[Investigation — executed steps and findings]\n")
	for _, s := range steps {
		fmt.Fprintf(&b, "- %s → %s", s.goal, s.outcome.Status)
		if p := strings.TrimSpace(s.outcome.Result.Preview); p != "" {
			fmt.Fprintf(&b, ": %s", p)
		}
		if s.outcome.Reason != "" {
			fmt.Fprintf(&b, " (%s)", s.outcome.Reason)
		}
		b.WriteByte('\n')
	}
	b.WriteString("Answer the request using only these findings.")
	return b.String()
}
