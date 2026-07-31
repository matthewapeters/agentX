package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"agentx/internal/tools"
)

// planTaskToolName is the native tool name exposed for decomposition/wavefront —
// the model calls this at its own discretion, replacing the old classifier's
// invoke_planner heuristic.
const planTaskToolName = "plan_task"

// planTaskSchema is plan_task's native tool-calling schema. Its description
// carries the "spans multiple steps" judgment the classifier used to make
// ahead of time (internal/classify's invoke_planner criteria) — that judgment
// now lives here, made by the model itself.
func planTaskSchema() tools.ToolSchema {
	return tools.ToolSchema{
		Name: planTaskToolName,
		Description: "Decompose and investigate a goal that spans multiple steps or files: review, audit, " +
			"analyze, refactor, build, or anything about \"this project\", \"this repo\", \"these files\", or " +
			"\"the current state\". Executes real read/write tools against the goal and returns grounded " +
			"findings from what was actually inspected. Call this instead of guessing or answering from " +
			"memory when the request needs broad or open-ended investigation rather than one concrete action.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"goal":{"type":"string","description":"the multi-step goal to investigate"}},"required":["goal"]}`),
	}
}

// runPlanTaskTool is the plan_task tool's implementation: it runs the
// configured decomposition engine (decompose.DrainPlan or wavefront.Scheduler,
// selected by Settings.WavefrontEnabled — see selectedEngine) to completion
// against goal and returns the rendered findings as the tool-result content.
// Both engines are synchronous/blocking, so this is a plain call-and-wait; no
// process spawn is needed. Per-leaf tool executions inside the plan go through
// the same policy/approval gate as any other native tool call.
func (o *Orchestrator) runPlanTaskTool(ctx context.Context, goal string) (string, error) {
	if !o.planReady() {
		return "", fmt.Errorf("plan_task is not available: no decomposition engine wired")
	}
	rootID := fmt.Sprintf("task-%d", atomic.AddUint64(&o.planSeq, 1))
	planCtx, handled, err := o.runPlanPhase(ctx, goal, rootID)
	if err != nil {
		return "", err
	}
	if !handled {
		return "No steps were executed for this goal.", nil
	}
	return planCtx, nil
}

// planTaskGoal extracts the goal argument from a native plan_task tool call.
func planTaskGoal(args map[string]any) string {
	return tools.StringifyArg(args["goal"])
}
