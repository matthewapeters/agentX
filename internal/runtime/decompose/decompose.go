// Package decompose adapts the real classifier, branch, and planner to the scheduler's
// injected seams (ADR 0008 Phase 4c). The scheduler stays LLM-free; these adapters are
// where its Oracle and Decomposer become concrete.
//
//   - Oracle: a node is atomic iff the action classifier resolves its goal to a single
//     actionable type AND a one-step check passes (the ADR §2 refinement — the canonical
//     fixture proved type-resolution alone is not enough).
//   - Decomposer: forks a Phase-2 investigating branch (isolated, WM-snapshotted, read-
//     restricted), runs the planner inside it, and seals a branch.Result of child records
//     + synthesis.
//
// The classifier / one-step / planner collaborators are narrow interfaces, so the adapters
// are unit-testable with stubs and the live wiring (pipeline classifier, LLM planner) lands
// in Phase 4d/4e.
//
// Design: docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md.
// Behavior: docs/architecture/behavior/adr/0008_task_decomposition_integration.feature.md.
package decompose

import (
	"context"
	"strings"

	"agentx/internal/llm/fanout"
	"agentx/internal/prompting/planner"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
	"agentx/internal/session"
)

// ActionClassifier resolves a goal to an action verdict (the tuned action_classify group).
type ActionClassifier interface {
	ClassifyAction(ctx context.Context, goal string) (fanout.Decision, error)
}

// OneStepChecker answers whether a goal is satisfiable in a single executor step, rather
// than a narrated multi-step effort. It is the ADR §2 one-step detector.
type OneStepChecker interface {
	OneStep(ctx context.Context, goal string) (bool, error)
}

// Oracle is the scheduler's atomicity oracle: resolve AND one-step.
type Oracle struct {
	Action  ActionClassifier
	OneStep OneStepChecker // nil ⇒ resolution alone decides (degenerate; see note)
}

// Atomic reports whether a node is an atomic leaf. It is non-atomic when the action
// classifier abstains or resolves to "none"/"" (it does not name an actionable type), or
// when it resolves but the one-step check says the goal needs several steps. Only a node
// that both resolves and is one-step is handed to the executor.
func (o Oracle) Atomic(ctx context.Context, rec task.Record) (bool, error) {
	d, err := o.Action.ClassifyAction(ctx, rec.Goal)
	if err != nil {
		return false, err
	}
	if d.Abstained || d.Verdict == "" || d.Verdict == "none" {
		return false, nil // does not resolve to an actionable type ⇒ decompose
	}
	if o.OneStep == nil {
		return true, nil
	}
	return o.OneStep.OneStep(ctx, rec.Goal)
}

// Planner renders a goal (with the branch's investigation context) into a parsed plan.
// parentID namespaces the child ids so the sub-plan is session-unique (Phase-1 DAG).
type Planner interface {
	Plan(ctx context.Context, parentID, goal, contextText string) (planner.Plan, error)
}

// Decomposer is the scheduler's decomposer: it plans a non-atomic goal in an isolated,
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
func (d Decomposer) Decompose(ctx context.Context, rec task.Record) (branch.Result, error) {
	var facts []session.Fact
	if d.Facts != nil {
		facts = d.Facts()
	}
	b, err := branch.Fork(branch.Parent{SessionID: d.SessionID, Depth: 0, Facts: facts}, d.MaxDepth)
	if err != nil {
		return branch.Result{}, err
	}
	plan, err := d.Planner.Plan(ctx, rec.ID, rec.Goal, factsContext(b.Facts()))
	if err != nil {
		return branch.Result{}, err
	}
	for _, child := range plan.Records {
		if err := b.Add(child); err != nil {
			return branch.Result{}, err
		}
	}
	b.SetSynthesis(plan.Synthesis)
	return b.Seal(), nil
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
