package wavefront

import (
	"context"
	"encoding/json"
)

// Chat completes a system+user prompt pair against the model under an optional
// JSON-schema constraint (format; nil leaves output unconstrained), returning its
// text. Mirrors decompose.Chat's exact signature — a real system message (durable
// rules + catalog) and a real user message (working memory + question + reply
// format) as distinct roles, ADR 0011 — so the classifier is testable with a stub
// and wired to Ollama in production the same way the planner is. Not shared as one
// type across engines: each constructs its own closure, per the ADR's risk-isolation
// posture, even though the shape is identical enough that unifying construction
// later would not change either engine's behavior.
type Chat func(ctx context.Context, systemPrompt, userPrompt string, format json.RawMessage) (string, error)

// LLMClassifier is the production Classifier: renders the classify system prompt
// (rules + tool catalog) and user prompt (working memory + question + reply format)
// under ClassifySchema's constraint, asks the model, and parses the reply — mirrors
// decompose.LLMPlanner's construction exactly.
type LLMClassifier struct {
	Chat Chat
	// Template is the classify system prompt (agentx-wavefront-classify.md
	// content). Empty falls back to DefaultClassifyPromptTemplate.
	Template string
	// Catalog is the compact tool catalog rendered once at construction, same
	// convention as decompose.LLMPlanner.Catalog.
	Catalog string
}

// Classify renders the system and user messages, calls the model under the classify
// schema constraint, and parses. Parse errors propagate so the caller (the future
// wavefront scheduler) surfaces them rather than acting on a broken classification.
func (c LLMClassifier) Classify(ctx context.Context, wm, question string) (Result, error) {
	tmpl := c.Template
	if tmpl == "" {
		tmpl = DefaultClassifyPromptTemplate
	}
	sys := RenderClassifySystem(tmpl, c.Catalog)
	usr := RenderClassifyUser(DefaultClassifyUserTemplate, wm, question)
	reply, err := c.Chat(ctx, sys, usr, ClassifySchema())
	if err != nil {
		return Result{}, err
	}
	return Parse([]byte(reply))
}
