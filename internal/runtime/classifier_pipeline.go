package runtime

import (
	"context"
	"fmt"

	"agentx/internal/llm/fanout"
	"agentx/internal/llm/invoke"
	"agentx/internal/llm/ollama"
	"agentx/internal/prompting/cascade"
	"agentx/internal/prompting/corpus"
	"agentx/internal/prompting/pipeline"
	"agentx/internal/prompting/task"
	"agentx/internal/state"
)

// digestMaxTurns bounds the v1 session digest window fed to the classifier.
const digestMaxTurns = 12

// WithTaskClassifier injects the classifier pipeline (for tests). Without it, the
// orchestrator builds a live Ollama-backed pipeline at Start when a prompt corpus is
// configured (Settings.PromptCorpus).
func WithTaskClassifier(p *pipeline.Pipeline) Option {
	return func(o *Orchestrator) { o.taskPipeline = p }
}

// buildTaskClassifier constructs a live pipeline from settings when a prompt corpus
// is configured and none was injected. The feature is presence-gated and default
// off: with no corpus the orchestrator's prompt cycle is unchanged. A malformed
// corpus simply leaves the classifier off rather than failing startup. Caller holds
// o.mu.
func (o *Orchestrator) buildTaskClassifier() {
	if o.taskPipeline != nil || o.settings.PromptCorpus == "" {
		return
	}
	c, err := corpus.Parse([]byte(o.settings.PromptCorpus))
	if err != nil {
		return // opt-in feature: a bad corpus stays off, prompt cycle unaffected
	}
	client := ollama.New(o.settings.OllamaHost)
	inv := invoke.NewOllama(client, o.settings.OllamaModel, "")
	runner := cascade.NewRunner(inv, fanout.WithServerDefaults())
	o.taskPipeline = pipeline.New(runner, c, digestMaxTurns)
}

// maybeEmitTask runs the classifier pipeline over the session's prior turns and this
// turn, publishing a task_proposed event when the turn is actionable. It is a
// best-effort observer: it runs only on a cleanly completed, recorded turn, and any
// classification error yields no task and never disturbs the prompt cycle. It is
// called before recordTurn, so the digest reflects prior history (not this turn,
// which is passed explicitly).
func (o *Orchestrator) maybeEmitTask(ctx context.Context, cycleErr error, record bool, userOrd uint64, turn string) {
	if !record || cycleErr != nil {
		return
	}
	o.mu.Lock()
	p := o.taskPipeline
	events := o.historyEvents()
	o.mu.Unlock()
	if p == nil {
		return
	}
	res, err := p.Classify(ctx, events, turn)
	if err != nil {
		return
	}
	rec, ok := task.FromAction(fmt.Sprintf("task-%d", userOrd), turn, res.Action, res.Escalated)
	if !ok {
		return
	}
	o.publish("TASK_PROPOSED", state.ContentTaskProposed, rec)
}

// historyEvents projects the in-memory conversation history into events for the
// digest builder, preserving each turn's enabled state and ordinal. Caller holds
// o.mu.
func (o *Orchestrator) historyEvents() []state.Event {
	events := make([]state.Event, 0, len(o.history))
	for _, h := range o.history {
		ct := state.ContentUserPrompt
		if h.role == "assistant" {
			ct = state.ContentAgentResponse
		}
		events = append(events, state.Event{
			ContentType: ct,
			Payload:     map[string]any{"text": h.content},
			Enabled:     h.enabled,
			Ordinal:     h.ordinal,
		})
	}
	return events
}
