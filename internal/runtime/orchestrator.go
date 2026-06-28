package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"agentx/internal/classify"
	"agentx/internal/prompting"
	"agentx/internal/session"
	"agentx/internal/state"
)

// Settings are the runtime inputs the composition root derives from configuration.
type Settings struct {
	// SessionRoot is the directory under which sessions are stored.
	SessionRoot string
	// OllamaHost and OllamaModel configure the model adapter (used by the prompt
	// cycle in CHT-C*).
	OllamaHost  string
	OllamaModel string
	// Instructions is the standing user-instructions text prefixed to every LLM
	// context (from ~/.config/agentx/agentx-instructions.md). Empty falls back to
	// the built-in default system prompt.
	Instructions string
	// BootstrapPrompt, when non-empty, is submitted automatically at startup
	// (from ~/.config/agentx/bootstrap-prompt.md).
	BootstrapPrompt string
	// ClassificationPrompt is the system prompt for the classify step (from
	// ~/.config/agentx/agentx-classification.md). Empty uses the built-in default.
	ClassificationPrompt string
	// ClassificationRetries is the classify-cycle retry budget.
	ClassificationRetries int
}

// Orchestrator owns the per-process runtime: session, event bus, processing
// state, and persistence.
type Orchestrator struct {
	settings Settings

	store      *session.Store
	id         session.Identity
	bus        *state.Bus
	proc       *state.ProcessingPublisher
	model      Model
	assembler  *prompting.Assembler
	classifier *classify.Classifier
	recDone    chan error
	recSub     *state.Subscription

	mu        sync.Mutex
	started   bool
	accepting bool
}

// Option configures an Orchestrator at construction time.
type Option func(*Orchestrator)

// WithModel overrides the LLM the prompt cycle drives. Without it the
// orchestrator builds a live Ollama adapter from its settings at Start.
func WithModel(m Model) Option {
	return func(o *Orchestrator) { o.model = m }
}

// WithClassifier overrides the prompt classifier. Without it the orchestrator
// builds one from its settings (using the model) at Start.
func WithClassifier(c *classify.Classifier) Option {
	return func(o *Orchestrator) { o.classifier = c }
}

// New returns an unstarted Orchestrator for the given settings.
func New(s Settings, opts ...Option) *Orchestrator {
	o := &Orchestrator{settings: s}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Start runs the startup sequence: create the session, start the bus and
// processing-state feed (idle), and begin draining events to disk.
func (o *Orchestrator) Start() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.started {
		return fmt.Errorf("orchestrator already started")
	}

	o.store = session.NewStore(o.settings.SessionRoot)
	id, err := o.store.Create()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	o.id = id

	o.bus = state.NewBus()
	o.proc = state.NewProcessingPublisher(id.ID)
	if o.model == nil {
		o.model = newOllamaModel(o.settings.OllamaHost)
	}
	instructions := o.settings.Instructions
	if instructions == "" {
		instructions = prompting.DefaultSystemPrompt
	}
	o.assembler = prompting.New(instructions)
	if o.classifier == nil {
		chat := func(ctx context.Context, msgs []prompting.Message) (string, error) {
			return o.model.Chat(ctx, o.settings.OllamaModel, msgs, func(string) {})
		}
		o.classifier = classify.New(o.settings.ClassificationPrompt, o.settings.ClassificationRetries, chat)
	}

	recorder := o.store.Recorder(id.ID)
	sub := o.bus.Subscribe()
	o.recSub = sub
	o.recDone = make(chan error, 1)
	go func() { o.recDone <- recorder.Run(sub) }()

	o.started = true
	o.accepting = true
	return nil
}

// Shutdown stops accepting prompts, persists a final processing-state snapshot,
// flushes the recorder, and returns. It respects ctx cancellation while waiting
// for the recorder to drain.
func (o *Orchestrator) Shutdown(ctx context.Context) error {
	o.mu.Lock()
	if !o.started {
		o.mu.Unlock()
		return fmt.Errorf("orchestrator not started")
	}
	o.accepting = false
	sub := o.recSub
	done := o.recDone
	o.mu.Unlock()

	// Persist a final processing-state snapshot before draining.
	o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   "PROCESSING_STATE",
		ContentType: state.ContentProcessingState,
		Payload:     o.proc.Current(),
	})

	sub.Close()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Accepting reports whether the orchestrator is accepting new prompts.
func (o *Orchestrator) Accepting() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.accepting
}

// Settings returns the settings the orchestrator was built with.
func (o *Orchestrator) Settings() Settings { return o.settings }

// Bus returns the canonical event bus (for surfaces and the prompt cycle).
func (o *Orchestrator) Bus() *state.Bus { return o.bus }

// Processing returns the processing-state publisher.
func (o *Orchestrator) Processing() *state.ProcessingPublisher { return o.proc }

// Session returns the active session identity.
func (o *Orchestrator) Session() session.Identity { return o.id }

// CheckModel verifies the configured model is available (CHT-C4). It is called
// after Start, before prompts are accepted, so an unavailable model is reported
// clearly rather than surfacing as a per-prompt failure. ctx bounds the probe.
func (o *Orchestrator) CheckModel(ctx context.Context) error {
	o.mu.Lock()
	model := o.model
	name := o.settings.OllamaModel
	o.mu.Unlock()
	if model == nil {
		return fmt.Errorf("orchestrator not started: no model")
	}
	if err := model.Ready(ctx, name); err != nil {
		return fmt.Errorf("model %q is not available: %w", name, err)
	}
	return nil
}

// Submit runs one prompt cycle (CHT-C3): it records the user prompt, drives the
// model through the respond phase streaming agent_response deltas onto the bus,
// and transitions processing-state idle→working→completed. A model error routes
// an error event and transitions to failed. Event ordering is deterministic:
// user_prompt, then agent_response deltas in stream order, then the terminal
// processing-state. Canceling ctx interrupts the in-flight model call: any
// partial response is kept, no error is recorded, and the cycle ends completed.
func (o *Orchestrator) Submit(ctx context.Context, text string) error {
	return o.runPrompt(ctx, text, true, true)
}

// SubmitBootstrap submits the configured bootstrap prompt at startup (story:
// bootstrap prompt). It runs the normal cycle with instructions prefixed but
// does not record a user-prompt entry, so the model response is the first thing
// shown. It is a no-op when no bootstrap prompt is configured.
func (o *Orchestrator) SubmitBootstrap(ctx context.Context) error {
	o.mu.Lock()
	text := o.settings.BootstrapPrompt
	o.mu.Unlock()
	if text == "" {
		return nil
	}
	// Bootstrap skips classification so the response is the first thing shown.
	return o.runPrompt(ctx, text, false, false)
}

// runPrompt drives one prompt cycle. When recordUserPrompt is false the user
// message is still sent to the model (so instructions + prompt reach the LLM) but
// no user_prompt event is published — used for the bootstrap prompt. When
// classifyPrompt is true the prompt is classified (and a classification event
// published) before the respond phase.
func (o *Orchestrator) runPrompt(ctx context.Context, text string, recordUserPrompt, classifyPrompt bool) error {
	o.mu.Lock()
	ready := o.started && o.accepting
	model := o.model
	assembler := o.assembler
	classifier := o.classifier
	o.mu.Unlock()
	if !ready {
		return fmt.Errorf("orchestrator not accepting prompts")
	}

	if classifyPrompt {
		o.setProcessing(state.StateWorking, state.PhaseClassify)
	} else {
		o.setProcessing(state.StateWorking, state.PhaseRespond)
	}
	if recordUserPrompt {
		o.publish("USER_PROMPT", state.ContentUserPrompt, map[string]any{"text": text})
	}

	if classifyPrompt && classifier != nil {
		verdict := classifier.Classify(ctx, text)
		o.publish("CLASSIFICATION", state.ContentClassification, map[string]any{
			"route":     string(verdict.Route),
			"rationale": verdict.Rationale,
			"text":      classificationText(verdict),
		})
		// v1: only respond_directly executes; reserved routes fall back to respond.
		o.setProcessing(state.StateWorking, state.PhaseRespond)
	}

	messages := assembler.Assemble(text)
	_, err := model.Chat(ctx, o.settings.OllamaModel, messages, func(delta string) {
		o.publish("AGENT_CONTENT", state.ContentAgentResponse, map[string]any{"text": delta})
	})
	switch {
	case err == nil:
		o.setProcessing(state.StateCompleted, state.PhaseNone)
		return nil
	case errors.Is(err, context.Canceled):
		// Interrupted by the user: not a failure. Keep the partial response and
		// end the cycle cleanly.
		o.setProcessing(state.StateCompleted, state.PhaseNone)
		return nil
	default:
		o.publish("ERROR", state.ContentAgentResponse, map[string]any{"text": err.Error()})
		o.setProcessing(state.StateFailed, state.PhaseNone)
		return err
	}
}

// classificationText renders the greyed "intent → route" line for the output
// panel (see ux/06_OUTPUT_WIDGET.md).
func classificationText(v classify.Verdict) string {
	if v.Rationale != "" {
		return fmt.Sprintf("%s → %s", v.Rationale, v.Route)
	}
	return fmt.Sprintf("→ %s", v.Route)
}

// publish stamps and fans an event out over the bus.
func (o *Orchestrator) publish(eventType string, ct state.ContentType, payload any) {
	o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   eventType,
		ContentType: ct,
		Payload:     payload,
		ModelName:   o.settings.OllamaModel,
	})
}

// setProcessing updates the live processing-state feed and persists a snapshot
// onto the bus so the transition is recoverable from the event log.
func (o *Orchestrator) setProcessing(s state.RunState, ph state.Phase) {
	o.proc.Set(s, ph)
	o.bus.Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   o.id.ID,
		EventType:   "PROCESSING_STATE",
		ContentType: state.ContentProcessingState,
		Payload:     o.proc.Current(),
	})
}
