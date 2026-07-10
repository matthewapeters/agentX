// Package decompose adapts the real branch and planner to the scheduler's injected
// Decomposer seam (ADR 0008, amended 2026-07-07). The scheduler stays LLM-free; this
// adapter is where its Decomposer becomes concrete.
//
// Decomposer forks a Phase-2 investigating branch (isolated, WM-snapshotted, read-
// restricted), runs the planner inside it, and seals a branch.Result of child records +
// synthesis. The planner declares each child's Kind (Step/Task) at generation time — the
// amendment retired the separate "atomicity oracle" that used to vote on this per node;
// Decomposer's only remaining judgment call is the echo check (a child that just restates
// the parent's goal), which a JSON-schema constraint cannot catch since it's semantic, not
// structural. A violation gets one retry with the problem named, then the node degrades to
// executing as a Task (scheduler.ErrNoProgress) rather than recursing.
//
// Design: docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md
// (amendment section). Behavior: docs/architecture/behavior/adr/0008_task_decomposition_integration.feature.md.
package decompose

import (
	"context"
	"fmt"
	"strings"

	"agentx/internal/planfindings"
	"agentx/internal/prompting/planner"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
	"agentx/internal/runtime/scheduler"
	"agentx/internal/session"
)

// Planner renders a goal (with the branch's investigation context) into a parsed plan.
// parentID namespaces the child ids so the sub-plan is session-unique (Phase-1 DAG).
type Planner interface {
	Plan(ctx context.Context, parentID, goal, contextText string) (planner.Plan, error)
}

// Decomposer is the scheduler's decomposer: it plans a Step's goal in an isolated,
// read-restricted branch and returns only the plan.
type Decomposer struct {
	Planner   Planner
	SessionID string
	MaxDepth  int
	// Facts supplies the parent's enabled working-memory facts to snapshot into the branch
	// (so the planner can see cwd/project/repo_root). nil ⇒ no inherited facts.
	Facts func() []session.Fact
}

// Decompose forks an investigating branch, runs the planner inside it with the inherited
// working-memory context, records the child plan in the branch, and seals a Result. The
// branch is plan-only (Phase 2), so decomposition cannot mutate. The scheduler has already
// bounded recursion depth before calling this, so the branch fork here starts a fresh
// isolated context.
//
// A plan whose only defect is a child echoing the parent's goal (SimilarGoals — the one
// thing a JSON-schema constraint cannot catch, since it's semantic) gets one retry, with
// the violation named in the regenerated context, before Decompose gives up and returns an
// error wrapping scheduler.ErrNoProgress — the scheduler then executes the node as a Task
// for one attempt rather than recurse (tidy-cove's anti-spiral guarantee, generalized).
func (d Decomposer) Decompose(ctx context.Context, rec task.Record) (branch.Result, error) {
	var facts []session.Fact
	if d.Facts != nil {
		facts = d.Facts()
	}
	b, err := branch.Fork(branch.Parent{SessionID: d.SessionID, Depth: 0, Facts: facts}, d.MaxDepth)
	if err != nil {
		return branch.Result{}, err
	}

	ctxText := factsContext(b.Facts())
	if findings := planfindings.From(ctx); findings != "" {
		// What this plan's earlier siblings/dependencies have already found — ground the
		// next children in it instead of re-discovering or guessing (amber-quartz: "list
		// the project root" was generated three separate times across one plan because
		// each decompose call only ever saw the parent's own goal text, never what an
		// earlier step had already turned up).
		ctxText += "\n\n" + findings
	}
	plan, violation := d.attemptPlan(ctx, rec, ctxText)
	if violation != "" {
		retryCtx := ctxText + "\n\nYour previous attempt was invalid: " + violation + ". Fix this and reply again."
		plan, violation = d.attemptPlan(ctx, rec, retryCtx)
		if violation != "" {
			return branch.Result{}, fmt.Errorf("decompose %q: %s (after retry): %w", rec.ID, violation, scheduler.ErrNoProgress)
		}
	}

	for _, child := range plan.Records {
		if err := b.Add(child); err != nil {
			return branch.Result{}, err
		}
	}
	b.SetSynthesis(plan.Synthesis)
	return b.Seal(), nil
}

// attemptPlan calls the planner once and classifies the result: a valid plan with an empty
// violation, or a violation description the caller can feed back into a retry prompt. Most
// structural violations (too many children, a node missing/mixing task-or-step) are caught
// by planner.Parse (itself a backstop behind PlanSchema's JSON-schema constraint) and
// surface here as the Plan error's text; the echo check is Decomposer's own, since it needs
// the parent's goal, which Parse never sees.
func (d Decomposer) attemptPlan(ctx context.Context, rec task.Record, contextText string) (planner.Plan, string) {
	plan, err := d.Planner.Plan(ctx, rec.ID, rec.Goal, contextText)
	if err != nil {
		return planner.Plan{}, err.Error()
	}
	for _, child := range plan.Records {
		if SimilarGoals(rec.Goal, child.Goal) {
			return planner.Plan{}, fmt.Sprintf("child %q echoes the parent goal", child.ID)
		}
	}
	return plan, ""
}

// factsContext renders the branch's inherited facts as "key: value" lines for the planner
// prompt, so the plan is grounded in the project (cwd, language, repo_root, …).
func factsContext(facts []session.Fact) string {
	var b strings.Builder
	for _, f := range facts {
		b.WriteString(f.Key)
		b.WriteString(": ")
		b.WriteString(f.Value)
		b.WriteString("\n")
	}
	return b.String()
}
