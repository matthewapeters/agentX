package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

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

	store   *session.Store
	id      session.Identity
	bus     *state.Bus
	proc    *state.ProcessingPublisher
	recDone chan error
	recSub  *state.Subscription

	mu        sync.Mutex
	started   bool
	accepting bool
}

// New returns an unstarted Orchestrator for the given settings.
func New(s Settings) *Orchestrator { return &Orchestrator{settings: s} }

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
