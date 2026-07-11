package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"agentx/internal/executor"
	"agentx/internal/planfindings"
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

// midDrainFindingsLines caps how much of each step's findings feed BACK into later
// decompose/proposal calls during the SAME drain — deliberately tighter than
// findingsLines (the one-shot final synthesis budget), since this gets re-injected
// into every subsequent LLM call for the rest of the plan, not read once at the end.
const midDrainFindingsLines = 20

// withComposedFindings attaches cap's live findings to ctx, composed underneath any
// findings source ctx already carries — so a nested/continuation plan phase (the
// verb-continuation follow-up round, ADR 0009-adjacent) sees an EARLIER round's
// findings as a base layer, not just its own new cap's, without runPlanPhase or
// runDecomposition needing a special "prior findings" parameter. Snapshotting the
// prior source once here (rather than calling it fresh each time) is correct because
// the outer plan phase, if any, has already finished by the time an inner one starts.
func withComposedFindings(ctx context.Context, cap *capturingExec) context.Context {
	prior := planfindings.From(ctx)
	if prior == "" {
		return planfindings.WithSource(ctx, cap.Findings)
	}
	return planfindings.WithSource(ctx, func() string {
		return prior + "\n" + cap.Findings()
	})
}

// Findings renders a live, monotonically-growing summary of every leaf this plan has
// completed so far — safe to call concurrently while the scheduler keeps draining
// (it locks the same mutex Execute appends under, so the read is a consistent
// snapshot). This is the read side of the sibling/dependency-context gap
// (mellow-meadow, lively-raven, quiet-cove, amber-quartz): a `tree` call that already
// ran and found real paths was invisible to a sibling Task resolved moments later,
// which then hallucinated `cat src/main.py` in a Go project. Threaded to the
// decomposer and tool proposer via internal/planfindings (context-scoped, not a
// struct field — see that package's doc comment for why).
func (c *capturingExec) Findings() string {
	c.mu.Lock()
	steps := append([]capturedStep(nil), c.steps...)
	c.mu.Unlock()
	if len(steps) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[This plan's findings so far — earlier steps in THIS investigation, not the final answer. Use these instead of guessing a path or re-discovering what's already here.]\n")
	for _, s := range steps {
		fmt.Fprintf(&b, "- %s → %s", s.goal, s.outcome.Status)
		if f := firstLines(s.findings, midDrainFindingsLines); f != "" {
			fmt.Fprintf(&b, ": %s", f)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// firstLines returns the first n lines of s, appending an ellipsis marker if more
// followed — a compact preview, not the full widened text findingsLines allows for
// the one-shot final synthesis.
func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + "\n…"
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
		"goal": rec.Goal, "depth": depth, "kind": string(rec.Kind),
	})
	p.o.planTrees.dispatched(p.root, rec, p.o.store, p.o.id.ID)
}

func (p *planObserver) NodeDecomposed(parent task.Record, children []task.Record) {
	kids := make([]map[string]any, 0, len(children))
	for _, c := range children {
		kids = append(kids, map[string]any{
			"task_id": c.ID, "goal": c.Goal, "deps": c.Deps, "kind": string(c.Kind),
		})
	}
	p.o.publish("TASK_NODE", state.ContentTaskNode, map[string]any{
		"root": p.root, "task_id": parent.ID, "event": "decomposed", "children": kids,
		"kind": string(parent.Kind),
	})
	p.o.planTrees.decomposed(p.root, parent, children, p.o.store, p.o.id.ID)
}

func (p *planObserver) NodeCompleted(id string, status task.Status) {
	p.o.publish("TASK_NODE", state.ContentTaskNode, map[string]any{
		"root": p.root, "task_id": id, "event": "completed", "status": string(status),
	})
	p.o.planTrees.completed(p.root, id, status, p.o.store, p.o.id.ID)
}

// runPlanPhase drains an imperative through the decomposition scheduler before the model
// answers: it decomposes the goal into a plan, executes the leaves (real read tools), and
// returns the plan + findings as context to ground the response. This is the wiring that
// makes the invoke_planner route actually act instead of free-forming. It returns
// handled=false (fall through to a direct answer) when nothing was investigated. rootID
// must be unique per plan within the session — an explicit parameter (rather than derived
// internally from userOrd) so a verb-continuation follow-up round (maybeContinuePlan) can
// drain a second, distinct plan tree for the SAME turn without colliding with the first
// round's root id.
func (o *Orchestrator) runPlanPhase(ctx context.Context, text, rootID string) (string, bool, error) {
	o.setProcessing(state.StateWorking, state.PhasePlanning)

	root := task.Record{
		ID:     rootID,
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
		"nodes": []map[string]any{{
			"task_id": root.ID, "goal": root.Goal, "status": string(root.Status),
			"deps": root.Deps, "kind": string(root.Kind),
		}},
	})
	cap := &capturingExec{inner: o.taskExec, reader: o.artifactStore()}
	ctx = withComposedFindings(ctx, cap)
	out, derr := decompose.DrainPlan(ctx, root, o.taskDecomp, cap,
		decompose.DefaultSlots, decompose.DefaultMaxDepth, &planObserver{o: o, root: root.ID})
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}

	o.publishPlan(root, out, len(cap.steps), derr)

	proceed, note, cerr := o.confirmPlanIncomplete(ctx, root, out.Nodes)
	if cerr != nil {
		return "", false, cerr
	}
	if !proceed {
		return planStoppedContext(root), true, nil
	}

	planCtx := planContext(cap.steps) + note
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

// incompleteNodes tallies a drained plan's non-Done statuses and collects a few example
// goals worth surfacing to the user — a flat "4 never ran" count doesn't say what was
// actually missed, and the missed node is often buried deep in the tree.
func incompleteNodes(recs []task.Record) (failed, abstained, neverRan int, examples []string) {
	const maxExamples = 3
	for _, n := range recs {
		switch n.Status {
		case task.Done:
			// complete
		case task.Failed:
			failed++
		case task.Abstained:
			abstained++
			if len(examples) < maxExamples {
				examples = append(examples, truncateGoal(n.Goal))
			}
		default: // proposed / ready / in_progress: stranded behind a failed/abstained dep
			neverRan++
		}
	}
	return failed, abstained, neverRan, examples
}

// truncateGoal caps a goal string for a compact clarify prompt.
func truncateGoal(goal string) string {
	const maxLen = 80
	if len(goal) <= maxLen {
		return goal
	}
	return goal[:maxLen] + "…"
}

// planSummary builds the terminal task_plan payload, deriving an "incomplete" error line
// from the node statuses when the drain error alone would under-report.
func planSummary(root task.Record, recs []task.Record, executed int, derr error) map[string]any {
	nodes := make([]map[string]any, 0, len(recs))
	for _, n := range recs {
		nodes = append(nodes, map[string]any{
			"task_id": n.ID, "goal": n.Goal, "status": string(n.Status), "deps": n.Deps,
			"kind": string(n.Kind),
		})
	}
	failed, abstained, neverRan, _ := incompleteNodes(recs)
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

// planClarifyOptions is the fixed two-way choice offered when a drained plan didn't
// finish: answer with the partial findings, or stop rather than let the model silently
// paper over the gap.
var planClarifyOptions = []state.ApprovalOption{
	{Label: "Answer with what I found", Decision: "answer"},
	{Label: "Stop here", Decision: "stop"},
}

// confirmPlanIncomplete asks the user how to proceed when a drained plan has any
// failed/abstained/never-ran node — the scheduler's own NEEDS-CLARIFY state
// (scheduler.go: a Step at max depth "fails to Ask") has nowhere else to surface, and
// previously it didn't: the plan-incomplete signal reached only the task_plan event
// (excluded from context), never the model's response prompt, so the model answered as
// if nothing were missing (witty-falcon: a plan that never reached its file-write step
// still got narrated back as "done"). A fully-done plan returns proceed=true with no
// prompt — this only interrupts when something actually didn't finish. On "answer", note
// is a context line grounding the response in the gap so the model cannot claim to have
// completed steps that never ran. On "stop", proceed is false and the caller should use
// planStoppedContext instead of planContext.
func (o *Orchestrator) confirmPlanIncomplete(ctx context.Context, root task.Record, recs []task.Record) (proceed bool, note string, err error) {
	failed, abstained, neverRan, examples := incompleteNodes(recs)
	if failed+abstained+neverRan == 0 {
		return true, "", nil
	}
	prompt := fmt.Sprintf(
		"My plan for %q didn't fully complete — %d step(s) failed, %d abstained, %d never ran.",
		root.Goal, failed, abstained, neverRan)
	if len(examples) > 0 {
		prompt += " Stuck on: " + strings.Join(examples, "; ") + "."
	}
	prompt += " Answer with what I found so far, or stop here?"

	dec, derr := o.RequestDecision(ctx, state.PhaseClarify, prompt, planClarifyOptions)
	if derr != nil {
		return false, "", derr
	}
	if dec != "answer" {
		return false, "", nil
	}
	note = fmt.Sprintf(
		"\n\n[Plan incomplete: %d failed, %d abstained, %d never ran. Answer ONLY using the findings above — do not claim to have completed steps that never ran.]",
		failed, abstained, neverRan)
	return true, note, nil
}

// planStoppedContext grounds the response when the user chose to stop rather than have
// the model answer over an incomplete plan (confirmPlanIncomplete) — mirrors
// toolDeniedContext's shape (tool_cycle.go) for a declined action.
func planStoppedContext(root task.Record) string {
	return fmt.Sprintf(
		"\n\n[Investigation of %q stopped at your request before finishing. Say what you'd like to do next rather than assuming it was completed.]",
		root.Goal)
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
