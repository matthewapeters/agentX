// Package pipeline chains the classifier stages over a session: it builds the
// v1 digest, runs relatedness triage through the cascade runner, turns that
// verdict into a context directive, and runs action classification scoped by the
// directive. This is the first place two cascade stages chain over real session
// state — the connective tissue between internal/prompting/digest,
// internal/prompting/cascade, and the corpus fan-groups.
//
// Design: docs/architecture/prompt_fan_groups.md (stage 0 relatedness triage +
// the context directive table), cascade_classifier.md. Behavior contract:
// tests/features/prompting/classify_pipeline.feature.
package pipeline

import (
	"context"
	"fmt"

	"agentx/internal/llm/fanout"
	"agentx/internal/prompting/cascade"
	"agentx/internal/prompting/corpus"
	"agentx/internal/prompting/digest"
	"agentx/internal/state"
)

// Fan-group names the pipeline chains, in order.
const (
	triageGroup = "relatedness_triage"
	actionGroup = "action_classify"
)

// Directive is the context-assembly decision produced by relatedness triage: the
// {{context}} the downstream turn classifier may see, and how cautiously to trust
// it. It is conservative about dropping context — only a confident, non-abstained
// "new" clears the thread; everything else carries it.
type Directive struct {
	Relation  string // continuation | new | orthogonal | related_aside ("" on abstain)
	Context   string // the assembled {{context}} for downstream stages
	Cautious  bool   // related_aside → carry the thread, but cautiously
	Abstained bool   // triage vote scattered → context kept, caller should ask
}

// Result is the outcome of the two-stage classify chain for one turn.
type Result struct {
	Directive Directive
	Triage    fanout.Decision // stage-0 relatedness verdict
	Action    fanout.Decision // action verdict, classified under the directive
	Escalated bool            // either stage escalated to a vote
}

// Pipeline runs the digest → triage → directive → action-classify chain.
type Pipeline struct {
	runner   *cascade.Runner
	corpus   *corpus.Corpus
	maxTurns int
}

// New builds a pipeline over a cascade runner and a loaded corpus. maxTurns bounds
// the digest window (<= 0 keeps all turns).
func New(runner *cascade.Runner, c *corpus.Corpus, maxTurns int) *Pipeline {
	return &Pipeline{runner: runner, corpus: c, maxTurns: maxTurns}
}

// Classify runs the chain for a new turn against a session's event log. The caller
// supplies the events (e.g. from session.Recorder.Load), keeping the pipeline
// decoupled from disk.
func (p *Pipeline) Classify(ctx context.Context, events []state.Event, turn string) (Result, error) {
	dig := digest.Build(events, p.maxTurns)

	// Stage 0 — relatedness triage over the digest.
	triage, ok := p.corpus.Group(triageGroup)
	if !ok {
		return Result{}, fmt.Errorf("corpus missing %q fan-group", triageGroup)
	}
	tres, err := p.runner.Run(ctx, triage, vars(turn, dig.Render(), ""))
	if err != nil {
		return Result{}, fmt.Errorf("relatedness triage: %w", err)
	}

	dir := deriveDirective(tres.Decision, dig)

	// Stage B — action classification, scoped by the triage directive.
	action, ok := p.corpus.Group(actionGroup)
	if !ok {
		return Result{}, fmt.Errorf("corpus missing %q fan-group", actionGroup)
	}
	ares, err := p.runner.Run(ctx, action, vars(turn, dig.Render(), dir.Context))
	if err != nil {
		return Result{}, fmt.Errorf("action classify: %w", err)
	}

	return Result{
		Directive: dir,
		Triage:    tres.Decision,
		Action:    ares.Decision,
		Escalated: tres.Escalated || ares.Escalated,
	}, nil
}

// deriveDirective maps a relatedness verdict to the context the downstream turn
// classifier may see (docs/architecture/prompt_fan_groups.md, stage-0 table).
// v1 has only the recent-turn tier, so the thread/task/domain distinctions among
// continuation/orthogonal/related_aside collapse to "carry the digest" — they
// sharpen when the open-task tier and rolling summary arrive. Wrongly dropping
// context is catastrophic, so an abstain keeps context and only a confident "new"
// clears it.
func deriveDirective(d fanout.Decision, dig digest.Digest) Directive {
	dir := Directive{Relation: d.Verdict, Context: dig.Render(), Abstained: d.Abstained}
	if d.Abstained {
		return dir // scattered vote → keep context, let the caller ask
	}
	switch d.Verdict {
	case "new":
		dir.Context = "" // fresh intent → drop the thread
	case "related_aside":
		dir.Cautious = true // looks new but isn't → carry, cautiously
	case "continuation", "orthogonal":
		// carry the thread (v1: the recent-turn tier)
	}
	return dir
}

// vars fills the corpus template placeholders. open_tasks is empty until the task
// tier lands; both stages receive every key so no template renders a stray
// placeholder.
func vars(turn, digestText, contextText string) map[string]string {
	return map[string]string{
		"turn":           turn,
		"session_digest": digestText,
		"context":        contextText,
		"open_tasks":     "",
	}
}
