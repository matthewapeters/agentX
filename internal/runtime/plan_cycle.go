package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/decompose"
	"agentx/internal/state"
)

// planReady reports whether the decomposition substrate is wired (branch decomposer and
// executor). buildDecomposition sets these at Start.
func (o *Orchestrator) planReady() bool {
	return o.taskDecomp != nil && o.taskExec != nil
}

// findingsLines bounds how much of a tool result's real output feeds the plan's synthesis
// — deliberately much larger than the executor's UI-preview cap (20 lines). Those two
// numbers serve different audiences (Context Curation, CLAUDE.md): the collapsed tool
// widget is a human glance, expandable on demand; the synthesis prompt is the model's ONE
// chance to see what a step actually found, with no way to ask for more. Sharing one small
// cap between them (the lively-raven bug) meant `tree`'s entire depth-3 structure was
// discarded down to its first ~20 alphabetical entries before synthesis ever saw it — a
// stray `agentx.egg-info` and a failed guessed path made it in; `go.mod`/`cmd/`/`internal/`
// did not, and the model concluded "Python-based" from what little survived.
const findingsLines = 200

// artifactReader reads back a tool result's full stored output by ref (satisfied by
// *session.Artifacts) — the same store the executor already writes every result to,
// regardless of how short its UI preview is.
type artifactReader interface {
	Read(ref string, offset, limit int) ([]byte, error)
}

// capturingExec wraps the task executor to record every leaf outcome as the scheduler
// drains a plan, so the plan cycle can fold the real findings into the answer. The
// scheduler runs Execute on worker goroutines, so it locks.
type capturingExec struct {
	inner taskExecutor
	// reader fetches the widened findings text from the artifact store; nil (no store, or
	// a lookup failure at construction) falls back to the step's UI preview — degraded,
	// but never a hard failure over an auxiliary capability.
	reader artifactReader
	mu     sync.Mutex
	steps  []capturedStep
}

type capturedStep struct {
	goal    string
	outcome executor.Outcome
	// findings is what actually reaches the synthesis prompt: up to findingsLines of the
	// real result, read back from the artifact store — not the UI's tight preview.
	findings string
}

func (c *capturingExec) Execute(ctx context.Context, rec task.Record) executor.Outcome {
	out := c.inner.Execute(ctx, rec)
	findings := strings.TrimSpace(out.Result.Preview)
	if c.reader != nil && out.Result.Ref != "" {
		if data, err := c.reader.Read(out.Result.Ref, 0, findingsLines); err == nil && len(data) > 0 {
			findings = strings.TrimSpace(string(data))
		}
	}
	c.mu.Lock()
	c.steps = append(c.steps, capturedStep{goal: rec.Goal, outcome: out, findings: findings})
	c.mu.Unlock()
	return out
}

// planObserver streams scheduler lifecycle transitions to the session bus as they happen
// (ADR 0009 §9a) — the seam that retired batch-emit-at-completion. Callbacks arrive on the
// scheduler's main loop, and publish is bus-safe, so no locking is needed here.
type planObserver struct {
	o    *Orchestrator
	root string
}

func (p *planObserver) NodeDispatched(rec task.Record, depth int) {
	p.o.publish("TASK_NODE", state.ContentTaskNode, map[string]any{
		"root": p.root, "task_id": rec.ID, "event": "dispatched",
		"goal": rec.Goal, "depth": depth,
	})
}

func (p *planObserver) NodeDecomposed(parent task.Record, children []task.Record) {
	kids := make([]map[string]any, 0, len(children))
	for _, c := range children {
		kids = append(kids, map[string]any{"task_id": c.ID, "goal": c.Goal, "deps": c.Deps})
	}
	p.o.publish("TASK_NODE", state.ContentTaskNode, map[string]any{
		"root": p.root, "task_id": parent.ID, "event": "decomposed", "children": kids,
	})
}

func (p *planObserver) NodeCompleted(id string, status task.Status) {
	p.o.publish("TASK_NODE", state.ContentTaskNode, map[string]any{
		"root": p.root, "task_id": id, "event": "completed", "status": string(status),
	})
}

// runPlanPhase drains an imperative through the decomposition scheduler before the model
// answers: it decomposes the goal into a plan, executes the leaves (real read tools), and
// returns the plan + findings as context to ground the response. This is the wiring that
// makes the invoke_planner route actually act instead of free-forming. It returns
// handled=false (fall through to a direct answer) when nothing was investigated.
func (o *Orchestrator) runPlanPhase(ctx context.Context, text string, userOrd uint64) (string, bool, error) {
	o.setProcessing(state.StateWorking, state.PhasePlanning)

	root := task.Record{
		ID:     fmt.Sprintf("task-%d", userOrd),
		Goal:   text,
		Type:   task.Query,
		Kind:   task.KindStep, // the invoke_planner route already judged this multi-step
		Status: task.Proposed,
		Deps:   []string{},
	}
	// Initial snapshot first, then per-node deltas stream as the plan drains — the user
	// sees the plan being worked, not a silent gap (tidy-cove, ADR 0009).
	o.publish("TASK_PLAN", state.ContentTaskPlan, map[string]any{
		"root": root.ID, "goal": root.Goal, "phase": "started",
		"nodes": []map[string]any{{"task_id": root.ID, "goal": root.Goal, "status": string(root.Status), "deps": root.Deps}},
	})
	cap := &capturingExec{inner: o.taskExec, reader: o.artifactStore()}
	out, derr := decompose.DrainPlan(ctx, root, o.taskDecomp, cap,
		decompose.DefaultSlots, decompose.DefaultMaxDepth, &planObserver{o: o, root: root.ID})
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}

	o.publishPlan(root, out, len(cap.steps), derr)
	planCtx := planContext(cap.steps)
	if strings.TrimSpace(planCtx) == "" {
		return "", false, nil // nothing investigated → answer directly
	}
	return planCtx, true, nil
}

// publishPlan records the drained plan (node goals, deps, final statuses) as the final
// task_plan snapshot. Per-node progress was already streamed as task_node deltas; this is
// the terminal summary. An incomplete plan — anything failed, abstained, or blocked
// behind a failed dependency — is reported loudly, never silently (mellow-meadow: one
// failed leaf silently stranded five nodes).
func (o *Orchestrator) publishPlan(root task.Record, out decompose.PlanOutcome, executed int, derr error) {
	payload := planSummary(root, out.Nodes, executed, derr)
	o.publish("TASK_PLAN", state.ContentTaskPlan, payload)
}

// planSummary builds the terminal task_plan payload, deriving an "incomplete" error line
// from the node statuses when the drain error alone would under-report.
func planSummary(root task.Record, recs []task.Record, executed int, derr error) map[string]any {
	nodes := make([]map[string]any, 0, len(recs))
	var failed, abstained, neverRan int
	for _, n := range recs {
		nodes = append(nodes, map[string]any{
			"task_id": n.ID, "goal": n.Goal, "status": string(n.Status), "deps": n.Deps,
		})
		switch n.Status {
		case task.Done:
			// complete
		case task.Failed:
			failed++
		case task.Abstained:
			abstained++
		default: // proposed / ready / in_progress: stranded behind a failed/abstained dep
			neverRan++
		}
	}
	payload := map[string]any{
		"root": root.ID, "goal": root.Goal, "phase": "ended", "nodes": nodes,
		"executed": executed,
	}
	switch {
	case derr != nil:
		payload["error"] = derr.Error()
	case failed+abstained+neverRan > 0:
		payload["error"] = fmt.Sprintf(
			"plan incomplete: %d failed, %d abstained, %d never ran (blocked behind them) of %d nodes",
			failed, abstained, neverRan, len(recs))
	}
	return payload
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
		if s.findings != "" {
			fmt.Fprintf(&b, ": %s", s.findings)
		}
		if s.outcome.Reason != "" {
			fmt.Fprintf(&b, " (%s)", s.outcome.Reason)
		}
		b.WriteByte('\n')
	}
	b.WriteString("Answer the request using only these findings.")
	return b.String()
}
