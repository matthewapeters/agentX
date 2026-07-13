package decompose

import (
	"context"
	"encoding/json"

	"agentx/internal/prompting/planner"
)

// Chat completes a system+user prompt pair against the model under an optional
// JSON-schema constraint (format; nil leaves output unconstrained), returning its text.
// Two parameters, not one flattened string: the planner sends a real system message
// (durable rules + catalog) and a real user message (working memory + goal + reply
// format) as distinct roles — ADR 0011, mirroring prompting.Assembler.Assemble's
// existing system/user split for the respond path. It abstracts the inference client so
// the planner is testable with a stub and wired to Ollama in production.
type Chat func(ctx context.Context, systemPrompt, userPrompt string, format json.RawMessage) (string, error)

// LLMPlanner is the production Planner: it renders the planner system prompt (rules +
// tool catalog) and user prompt (working memory + goal) under
// planner.PlanSchema's JSON-schema constraint, asks the model, and parses the reply into
// namespaced child records.
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

// Plan renders the system and user messages, calls the model under the plan schema
// constraint, and parses. Parse errors (a malformed plan) propagate so the decomposer
// surfaces them rather than emitting a broken sub-DAG.
func (p LLMPlanner) Plan(ctx context.Context, parentID, goal, contextText string) (planner.Plan, error) {
	tmpl := p.Template
	if tmpl == "" {
		tmpl = planner.DefaultPromptTemplate
	}
	sys := planner.RenderSystem(tmpl, p.Catalog)
	usr := planner.RenderUser(planner.DefaultUserTemplate, goal, contextText)
	reply, err := p.Chat(ctx, sys, usr, planner.PlanSchema())
	if err != nil {
		return planner.Plan{}, err
	}
	return planner.Parse(parentID, []byte(reply))
}
