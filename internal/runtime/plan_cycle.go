package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"agentx/internal/executor"
	"agentx/internal/llm/ollama"
	"agentx/internal/planfindings"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/decompose"
	"agentx/internal/runtime/wavefront"
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

// outputSummaryThreshold is the character length above which a captured step's real
// findings are summarized rather than used as-is (ADR 0012 §6, Phase 3) — most tool
// output is small and stays under this, matching totAlX's own "common case" design.
// A character count, not a line count: a single very long line (a minified file, a
// raw JSON blob) can blow this budget while still being "one line" under
// findingsLines' 200-line read window — the two caps address different failure
// shapes and both stay in place. Ported from totAlX's own measurement as an
// illustrative starting point, not a tuned agentX constant.
const outputSummaryThreshold = 2000

// outputSummaryTargetChars is the requested size of a summarized finding (guidance
// handed to the model, not a hard cap) and the size a fallback truncation is cut to.
const outputSummaryTargetChars = 1200

// summarizeFunc condenses text (a step's real, oversized findings) toward
// outputSummaryTargetChars, given the general-to-specific chain of questions that led
// to it (root goal first, the step's own goal last when they differ) — targeted at
// the most specific item only, per ADR 0012 §6. A nil summarizeFunc falls back to
// plain truncation in capturingExec.condense.
type summarizeFunc func(ctx context.Context, chain []string, text string) (string, error)

// capturingExec wraps the task executor to record every leaf outcome as the scheduler
// drains a plan, so the plan cycle can fold the real findings into the answer. The
// scheduler runs Execute on worker goroutines, so it locks.
type capturingExec struct {
	inner taskExecutor
	// reader fetches the widened findings text from the artifact store; nil (no store, or
	// a lookup failure at construction) falls back to the step's UI preview — degraded,
	// but never a hard failure over an auxiliary capability.
	reader artifactReader
	// rootGoal is the plan's top-level question — the general end of the chain handed
	// to summarize when a step's real output needs condensing (ADR 0012 §6).
	rootGoal string
	// summarize condenses oversized findings; nil falls back to plain truncation, same
	// degrade-gracefully posture as a nil reader above.
	summarize summarizeFunc
	mu        sync.Mutex
	steps     []capturedStep
}

type capturedStep struct {
	goal    string
	outcome executor.Outcome
	// findings is what actually reaches the synthesis prompt: up to findingsLines of the
	// real result, read back from the artifact store (not the UI's tight preview),
	// summarized down from that if it exceeded outputSummaryThreshold.
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
	if len(findings) > outputSummaryThreshold {
		findings = c.condense(ctx, rec.Goal, findings)
	}
	c.mu.Lock()
	c.steps = append(c.steps, capturedStep{goal: rec.Goal, outcome: out, findings: findings})
	c.mu.Unlock()
	return out
}

// condense summarizes oversized findings via c.summarize, chain-aware (root goal then
// this step's own goal — the "most specific" item summarize must target, per ADR 0012
// §6), falling back to plain truncation if summarize is nil or the call itself fails —
// worse than a targeted summary, but strictly better than letting an unbounded payload
// back into the findings pipeline (the lively-raven failure mode this path closes).
func (c *capturingExec) condense(ctx context.Context, goal, text string) string {
	if c.summarize != nil {
		chain := []string{c.rootGoal}
		if goal != c.rootGoal {
			chain = append(chain, goal)
		}
		if summary, err := c.summarize(ctx, chain, text); err == nil {
			if summary = strings.TrimSpace(summary); summary != "" {
				return fmt.Sprintf("[summarized from %d chars] %s", len(text), summary)
			}
		}
	}
	return truncateFindings(text)
}

// truncateFindings is the last-resort fallback when summarization is unavailable or
// fails: a prefix cut, positionally arbitrary (may cut off exactly the part that
// mattered) but strictly better than an unbounded payload reaching every subsequent
// call in the plan. Rune-safe: a naive byte-index slice can split a multi-byte UTF-8
// rune at the boundary and produce invalid text.
func truncateFindings(text string) string {
	runes := []rune(text)
	if len(runes) <= outputSummaryTargetChars {
		return text
	}
	kept := string(runes[:outputSummaryTargetChars])
	return fmt.Sprintf("%s... [truncated, %d more chars]", kept, len(runes)-outputSummaryTargetChars)
}

// newOutputSummarizer builds a summarizeFunc backed by client/model (ADR 0012 §6),
// reusing Phase 2's wavefront summary prompt scaffolding — the first real consumer of
// it. template overrides wavefront.DefaultSummaryPromptTemplate when non-empty
// (Settings.WavefrontSummaryPrompt). Runs with Think unset: this is schema-free plain
// prose with nothing structured to break, exactly the case ADR 0012 §7 says is safe
// (and preferable, for speed) to run without reasoning.
func newOutputSummarizer(client *ollama.Client, model, template string) summarizeFunc {
	sysTemplate := template
	if sysTemplate == "" {
		sysTemplate = wavefront.DefaultSummaryPromptTemplate
	}
	return func(ctx context.Context, chain []string, text string) (string, error) {
		sys := wavefront.RenderSummarySystem(sysTemplate, outputSummaryTargetChars)
		usr := wavefront.RenderSummaryUser(wavefront.DefaultSummaryUserTemplate, renderChain(chain), text)
		return client.Complete(ctx, ollama.CompleteRequest{
			Model: model,
			Messages: []ollama.Message{
				{Role: "system", Content: sys},
				{Role: "user", Content: usr},
			},
		})
	}
}

// renderChain numbers a general-to-specific question chain for the summarization
// prompt's user message, matching totAlX's own "1. ... 2. ..." chain format.
func renderChain(chain []string) string {
	var b strings.Builder
	for i, step := range chain {
		fmt.Fprintf(&b, "%d. %s\n", i+1, step)
	}
	return strings.TrimRight(b.String(), "\n")
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
	cap := &capturingExec{
		inner: o.taskExec, reader: o.artifactStore(),
		rootGoal: root.Goal, summarize: o.outputSummarizer,
	}
	ctx = withComposedFindings(ctx, cap)
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
// the terminal summary. An incomplete plan — anything failed, denied, abstained, or
// stranded behind an unresolved dependency — is reported loudly, never silently
// (mellow-meadow: one failed leaf silently stranded five nodes). Denied is counted
// separately from failed: a policy/user decision is not a bug (TOOL-7).
func (o *Orchestrator) publishPlan(root task.Record, out decompose.PlanOutcome, executed int, derr error) {
	payload := planSummary(root, out.Nodes, executed, derr)
	o.publish("TASK_PLAN", state.ContentTaskPlan, payload)
}

// planSummary builds the terminal task_plan payload, deriving an "incomplete" error line
// from the node statuses when the drain error alone would under-report.
func planSummary(root task.Record, recs []task.Record, executed int, derr error) map[string]any {
	nodes := make([]map[string]any, 0, len(recs))
	var failed, denied, abstained, neverRan int
	for _, n := range recs {
		nodes = append(nodes, map[string]any{
			"task_id": n.ID, "goal": n.Goal, "status": string(n.Status), "deps": n.Deps,
			"kind": string(n.Kind),
		})
		switch n.Status {
		case task.Done:
			// complete
		case task.Failed:
			failed++
		case task.Denied:
			denied++
		case task.Abstained:
			abstained++
		default: // proposed / ready / in_progress: stranded behind an unresolved dep
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
	case failed+denied+abstained+neverRan > 0:
		payload["error"] = fmt.Sprintf(
			"plan incomplete: %d failed, %d denied (needs approval), %d abstained, %d never ran (blocked behind them) of %d nodes",
			failed, denied, abstained, neverRan, len(recs))
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
