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

type promptCancelWorld struct {
	dir    string
	orc    *runtime.Orchestrator
	cancel context.CancelFunc
	done   chan struct{}
}

// registerPromptCancelSteps wires the prompt-interruption steps (CHT-C3 cancel).
func registerPromptCancelSteps(sc *godog.ScenarioContext) {
	w := &promptCancelWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.cancel != nil {
			w.cancel()
		}
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = promptCancelWorld{}
		return ctx, err
	})

	sc.Step(`^a started orchestrator with a stub model that blocks until canceled$`, w.startedBlocking)
	sc.Step(`^the prompt "([^"]*)" is submitted in the background$`, w.submitBackground)
	sc.Step(`^the in-flight prompt is interrupted$`, w.interrupt)
	sc.Step(`^the session is flushed to disk$`, w.shutdown)
	sc.Step(`^no error event is recorded$`, w.noErrorEvent)
	sc.Step(`^the interrupted cycle ends "([^"]*)"$`, w.finalState)
}

func (w *promptCancelWorld) startedBlocking() error {
	dir, err := os.MkdirTemp("", "agentx-cancel-")
	if err != nil {
		return err
	}
	w.dir = dir
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: dir, OllamaModel: "stub"},
		runtime.WithModel(stubModel{block: true}),
	)
	return w.orc.Start()
}

func (w *promptCancelWorld) submitBackground(text string) error {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		_ = w.orc.Submit(ctx, text)
	}()
	// Let the cycle reach the blocking model call before interrupting.
	if err := waitFor(func() bool {
		return w.orc.Processing().Current().State == state.StateWorking
	}); err != nil {
		return fmt.Errorf("prompt did not reach working state: %w", err)
	}
	return nil
}

func (w *promptCancelWorld) interrupt() error {
	w.cancel()
	select {
	case <-w.done:
		return nil
	case <-time.After(2 * time.Second):
		return fmt.Errorf("prompt did not return after interrupt")
	}
}

func (w *promptCancelWorld) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return w.orc.Shutdown(ctx)
}

func (w *promptCancelWorld) noErrorEvent() error {
	store := session.NewStore(w.dir)
	events, err := store.Recorder(w.orc.Session().ID).Load()
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.EventType == "ERROR" {
			return fmt.Errorf("unexpected ERROR event recorded after interrupt")
		}
	}
	return nil
}

func (w *promptCancelWorld) finalState(want string) error {
	if got := string(w.orc.Processing().Current().State); got != want {
		return fmt.Errorf("final processing state = %q, want %q", got, want)
	}
	return nil
}

// waitFor polls cond up to ~2s, returning an error if it never becomes true.
func waitFor(cond func() bool) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("condition not met within timeout")
}
