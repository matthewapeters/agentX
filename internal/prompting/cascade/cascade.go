// Package cascade runs a single fan-group as a cascade: a cheap coarse gate at
// R=1, escalating to a bounded self-consistency vote only when the gate is
// unsure or the verdict is high-stakes. It is context-agnostic — it takes a
// fan-group and an already-rendered vars map, so how that context was assembled
// (session digest, triage directive, cold start) is a separate concern above it.
//
// Design: docs/architecture/cascade_classifier.md, prompt_fan_groups.md.
// Behavior contract: tests/features/prompting/cascade_runner.feature.
package cascade

import (
	"context"

	"agentx/internal/llm/fanout"
	"agentx/internal/prompting/corpus"
)

// Result is a cascade outcome: the fold decision plus whether the coarse gate
// escalated to a vote.
type Result struct {
	Decision  fanout.Decision
	Escalated bool
}

// Runner runs fan-group cascades over a fan-out pool.
type Runner struct {
	pool *fanout.Pool
}

// NewRunner builds a cascade runner over the given invoker (WithServerDefaults or
// explicit options size the underlying pool).
func NewRunner(invoker fanout.Invoker, opts ...fanout.Option) *Runner {
	return &Runner{pool: fanout.New(invoker, opts...)}
}

// Run executes the cascade for one fan-group against the rendered context.
//
// Tier 1: the coarse gate runs a single invocation. If it conforms, is confident
// (>= the group's abstain threshold), and is not a high-stakes verdict, its
// answer is accepted as-is (Escalated=false).
//
// Tier 2: otherwise — unsure, high-stakes, or a malformed gate — the full group
// fans out and its majority-vote fold decides (Escalated=true), abstaining rather
// than guessing when the vote is thin or scattered.
func (r *Runner) Run(ctx context.Context, g *corpus.FanGroup, vars map[string]string) (Result, error) {
	if coarse, ok := g.RenderCoarse(vars); ok {
		results, err := r.pool.Run(ctx, []fanout.Invocation{coarse})
		if err != nil {
			return Result{}, err
		}
		if len(results) == 1 && results[0].Conforms() {
			resp := results[0].Response
			if !needsEscalation(g, resp) {
				return Result{
					Decision:  fanout.Decision{Verdict: resp.Verdict, Confidence: resp.Confidence},
					Escalated: false,
				}, nil
			}
		}
	}

	d, err := r.pool.Fold(ctx, g.Render(vars), g.Aggregator())
	if err != nil {
		return Result{}, err
	}
	return Result{Decision: d, Escalated: true}, nil
}

// needsEscalation reports whether a confident-looking coarse answer must still go
// to a vote: either its confidence is below the group's threshold, or its verdict
// is one the group marks high-stakes (a false positive there is destructive, so it
// never rides on the gate alone).
func needsEscalation(g *corpus.FanGroup, resp fanout.Response) bool {
	if resp.Confidence < g.AbstainBelow {
		return true
	}
	for _, t := range g.AlwaysEscalateTypes {
		if resp.Verdict == t {
			return true
		}
	}
	return false
}
