package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/runtime"
	"agentx/internal/session"
	"agentx/internal/state"
)

type lifecycleWorld struct {
	dir string
	orc *runtime.Orchestrator
}

// registerLifecycleSteps wires the orchestrator-lifecycle steps (CHT-A5).
func registerLifecycleSteps(sc *godog.ScenarioContext) {
	w := &lifecycleWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = lifecycleWorld{}
		return ctx, nil
	})

	sc.Step(`^orchestrator settings with a temp session root$`, w.settings)
	sc.Step(`^the orchestrator starts$`, w.start)
	sc.Step(`^a started orchestrator$`, w.startedOrchestrator)
	sc.Step(`^the processing state is idle$`, w.processingIdle)
	sc.Step(`^the orchestrator has an active session$`, w.hasSession)
	sc.Step(`^the orchestrator is accepting prompts$`, w.accepting)
	sc.Step(`^the orchestrator is not accepting prompts$`, w.notAccepting)
	sc.Step(`^a user_prompt event is published and the orchestrator shuts down$`, w.publishAndShutdown)
	sc.Step(`^the session timeline includes the user_prompt event$`, w.timelineHasUserPrompt)
	sc.Step(`^the session timeline includes a processing_state snapshot$`, w.timelineHasProcessingState)
}

func (w *lifecycleWorld) settings() error {
	dir, err := os.MkdirTemp("", "agentx-rt-")
	if err != nil {
		return err
	}
	w.dir = dir
	w.orc = runtime.New(runtime.Settings{SessionRoot: dir})
	return nil
}

func (w *lifecycleWorld) start() error {
	return w.orc.Start()
}

func (w *lifecycleWorld) startedOrchestrator() error {
	if err := w.settings(); err != nil {
		return err
	}
	return w.start()
}

func (w *lifecycleWorld) processingIdle() error {
	cur := w.orc.Processing().Current()
	if cur.State != state.StateIdle || cur.Phase != state.PhaseNone {
		return fmt.Errorf("processing state = %s/%s, want idle/none", cur.State, cur.Phase)
	}
	return nil
}

func (w *lifecycleWorld) hasSession() error {
	if w.orc.Session().ID == "" {
		return fmt.Errorf("orchestrator has no active session")
	}
	return nil
}

func (w *lifecycleWorld) accepting() error {
	if !w.orc.Accepting() {
		return fmt.Errorf("orchestrator is not accepting prompts")
	}
	return nil
}

func (w *lifecycleWorld) notAccepting() error {
	if w.orc.Accepting() {
		return fmt.Errorf("orchestrator is still accepting prompts")
	}
	return nil
}

func (w *lifecycleWorld) publishAndShutdown() error {
	w.orc.Bus().Publish(state.Event{
		Epoch:       time.Now().UnixMilli(),
		SessionID:   w.orc.Session().ID,
		EventType:   "USER_MESSAGE",
		ContentType: state.ContentUserPrompt,
		Payload:     map[string]any{"text": "hello"},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return w.orc.Shutdown(ctx)
}

func (w *lifecycleWorld) timeline() ([]state.Event, error) {
	store := session.NewStore(w.dir)
	return store.Recorder(w.orc.Session().ID).Load()
}

func (w *lifecycleWorld) timelineHasContent(ct state.ContentType) error {
	events, err := w.timeline()
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.ContentType == ct {
			return nil
		}
	}
	return fmt.Errorf("timeline (%d events) has no %s event", len(events), ct)
}

func (w *lifecycleWorld) timelineHasUserPrompt() error {
	return w.timelineHasContent(state.ContentUserPrompt)
}

func (w *lifecycleWorld) timelineHasProcessingState() error {
	return w.timelineHasContent(state.ContentProcessingState)
}
