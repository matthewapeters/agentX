package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

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
}

// Orchestrator owns the per-process runtime: session, event bus, processing
// state, and persistence.
type Orchestrator struct {
	settings Settings

	store     *session.Store
	id        session.Identity
	bus       *state.Bus
	proc      *state.ProcessingPublisher
	model     Model
	assembler *prompting.Assembler
	recDone   chan error
	recSub    *state.Subscription

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
	o.assembler = prompting.New(prompting.DefaultSystemPrompt)

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
// processing-state. ctx cancellation terminates the in-flight model call.
func (o *Orchestrator) Submit(ctx context.Context, text string) error {
	o.mu.Lock()
	ready := o.started && o.accepting
	model := o.model
	assembler := o.assembler
	o.mu.Unlock()
	if !ready {
		return fmt.Errorf("orchestrator not accepting prompts")
	}

	o.setProcessing(state.StateWorking, state.PhaseRespond)
	o.publish("USER_PROMPT", state.ContentUserPrompt, map[string]any{"text": text})

	messages := assembler.Assemble(text)
	_, err := model.Chat(ctx, o.settings.OllamaModel, messages, func(delta string) {
		o.publish("AGENT_CONTENT", state.ContentAgentResponse, map[string]any{"text": delta})
	})
	if err != nil {
		o.publish("ERROR", state.ContentAgentResponse, map[string]any{"text": err.Error()})
		o.setProcessing(state.StateFailed, state.PhaseNone)
		return err
	}

	o.setProcessing(state.StateCompleted, state.PhaseNone)
	return nil
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
