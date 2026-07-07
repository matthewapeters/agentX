package decompose

import (
	"context"
	"strings"

	"agentx/internal/llm/fanout"
	"agentx/internal/prompting/pipeline"
	"agentx/internal/prompting/planner"
)

// PipelineActions is the production ActionClassifier: it runs the tuned action_classify
// fan-group over a bare goal (no session history) and returns its verdict. This is the
// "type resolves" half of the atomicity oracle.
type PipelineActions struct {
	P *pipeline.Pipeline
}

// ClassifyAction classifies a sub-goal's action. It passes no events — a decomposition
// child is judged on its own goal, not the parent conversation.
func (a PipelineActions) ClassifyAction(ctx context.Context, goal string) (fanout.Decision, error) {
	res, err := a.P.Classify(ctx, nil, goal)
	if err != nil {
		return fanout.Decision{}, err
	}
	return res.Action, nil
}

// HeuristicOneStep is the interim one-step check (ADR 0008 Phase 4e): a goal is treated as
// multi-step — and therefore non-atomic even when its action resolves — when it chains two
// actions (" then ", ";", " and <verb>") or runs long. It is a cheap, deterministic
// stand-in for a dedicated one-step fan-group. Two hardenings from the tidy-cove spiral
// (ADR 0009): result-plumbing clauses ("and capture its output") are stripped before
// judging — the executor returns results automatically, so plumbing is not a second step —
// and a bare " and " between nouns ("files and directories") does not count as chaining.
type HeuristicOneStep struct {
	MaxWords int // 0 ⇒ 16
}

// OneStep reports whether goal is satisfiable in a single executor step.
func (h HeuristicOneStep) OneStep(_ context.Context, goal string) (bool, error) {
	action := stripResultPlumbing(goal)
	if coordinatesClauses(action) {
		return false, nil
	}
	max := h.MaxWords
	if max <= 0 {
		max = 16
	}
	if len(strings.Fields(action)) > max {
		return false, nil
	}
	return true, nil
}

// Chat completes a prompt against the model, returning its text. It abstracts the inference
// client so the planner is testable with a stub and wired to Ollama in production.
type Chat func(ctx context.Context, prompt string) (string, error)

// LLMPlanner is the production Planner: it renders the planner prompt for a goal and its
// branch context, asks the model, and parses the reply into namespaced child records.
type LLMPlanner struct {
	Chat Chat
}

// Plan renders, calls the model, and parses. Parse errors (a malformed plan) propagate so
// the decomposer surfaces them rather than emitting a broken sub-DAG.
func (p LLMPlanner) Plan(ctx context.Context, parentID, goal, contextText string) (planner.Plan, error) {
	reply, err := p.Chat(ctx, planner.Render(goal, contextText))
	if err != nil {
		return planner.Plan{}, err
	}
	return planner.Parse(parentID, []byte(reply))
}
