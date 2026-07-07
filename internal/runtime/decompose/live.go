package decompose

import (
	"context"
	"encoding/json"

	"agentx/internal/prompting/planner"
)

// Chat completes a prompt against the model under an optional JSON-schema constraint
// (format; nil leaves output unconstrained), returning its text. It abstracts the
// inference client so the planner is testable with a stub and wired to Ollama in
// production.
type Chat func(ctx context.Context, prompt string, format json.RawMessage) (string, error)

// LLMPlanner is the production Planner: it renders the planner prompt for a goal, its
// branch context, and the tool catalog under planner.PlanSchema's JSON-schema constraint,
// asks the model, and parses the reply into namespaced child records.
type LLMPlanner struct {
	Chat Chat
	// Template is the planner system prompt (agentx-planner.md content). Empty falls
	// back to planner.DefaultPromptTemplate, mirroring classify.New's convention.
	Template string
	// Catalog is the compact tool catalog rendered once at construction (the set of
	// tools/args a "task" node may name) — not re-rendered per call, since it does not
	// change mid-session.
	Catalog string
}

// Plan renders, calls the model under the plan schema constraint, and parses. Parse
// errors (a malformed plan) propagate so the decomposer surfaces them rather than
// emitting a broken sub-DAG.
func (p LLMPlanner) Plan(ctx context.Context, parentID, goal, contextText string) (planner.Plan, error) {
	tmpl := p.Template
	if tmpl == "" {
		tmpl = planner.DefaultPromptTemplate
	}
	reply, err := p.Chat(ctx, planner.Render(tmpl, goal, contextText, p.Catalog), planner.PlanSchema())
	if err != nil {
		return planner.Plan{}, err
	}
	return planner.Parse(parentID, []byte(reply))
}
