package runtimesteps

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/state"
)

type stateWorld struct {
	bus  *state.Bus
	subs []*state.Subscription
	proc *state.ProcessingPublisher
}

// registerStateSteps wires the event-bus and processing-state steps (CHT-A3).
func registerStateSteps(sc *godog.ScenarioContext) {
	w := &stateWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		for _, s := range w.subs {
			s.Close()
		}
		w.subs = nil
		w.bus = nil
		w.proc = nil
		return ctx, nil
	})

	sc.Step(`^a running event bus$`, w.runningBus)
	sc.Step(`^two subscribers are attached$`, w.twoSubscribers)
	sc.Step(`^a fast subscriber and a slow subscriber are attached$`, w.twoSubscribers)
	sc.Step(`^(\d+) ordered events are published$`, w.publishOrdered)
	sc.Step(`^each subscriber receives all (\d+) events in published order$`, w.eachReceivesInOrder)
	sc.Step(`^the fast subscriber receives all (\d+) events without waiting for the slow one$`, w.fastReceives)
	sc.Step(`^an agent_content event is published for the active session$`, w.publishAgentContent)
	sc.Step(`^the received event has a session id, event type, content type, and payload$`, w.receivedEnvelopeValid)

	sc.Step(`^a processing-state publisher for the active session$`, w.processingPublisher)
	sc.Step(`^the current state is "([^"]*)" with phase "([^"]*)"$`, w.currentStateIs)
	sc.Step(`^the state is set to working in phase "([^"]*)"$`, w.setWorking)
	sc.Step(`^the state is set to completed$`, w.setCompleted)
}

func (w *stateWorld) runningBus() error {
	w.bus = state.NewBus()
	return nil
}

func (w *stateWorld) twoSubscribers() error {
	if w.bus == nil {
		return fmt.Errorf("event bus not running")
	}
	w.subs = []*state.Subscription{w.bus.Subscribe(), w.bus.Subscribe()}
	return nil
}

func (w *stateWorld) publishOrdered(n int) error {
	if w.bus == nil {
		return fmt.Errorf("event bus not running")
	}
	for i := 1; i <= n; i++ {
		w.bus.Publish(state.Event{
			Epoch:       int64(i),
			SessionID:   "active",
			EventType:   "AGENT_CONTENT",
			ContentType: state.ContentAgentResponse,
			Payload:     map[string]any{"seq": i},
		})
	}
	return nil
}

func (w *stateWorld) eachReceivesInOrder(n int) error {
	for i, sub := range w.subs {
		got, err := readEvents(sub, n, time.Second)
		if err != nil {
			return fmt.Errorf("subscriber %d: %w", i, err)
		}
		for j, ev := range got {
			if ev.Epoch != int64(j+1) {
				return fmt.Errorf("subscriber %d event %d out of order: epoch=%d", i, j, ev.Epoch)
			}
		}
	}
	return nil
}

func (w *stateWorld) fastReceives(n int) error {
	if len(w.subs) < 1 {
		return fmt.Errorf("no subscribers attached")
	}
	// Deliberately never read from the slow subscriber (w.subs[1]); the fast
	// subscriber must still receive everything.
	got, err := readEvents(w.subs[0], n, time.Second)
	if err != nil {
		return fmt.Errorf("fast subscriber blocked by slow one: %w", err)
	}
	if len(got) != n {
		return fmt.Errorf("fast subscriber got %d events, want %d", len(got), n)
	}
	return nil
}

func (w *stateWorld) publishAgentContent() error {
	if w.bus == nil {
		return fmt.Errorf("event bus not running")
	}
	w.subs = []*state.Subscription{w.bus.Subscribe()}
	w.bus.Publish(state.Event{
		Epoch:       1,
		SessionID:   "active",
		EventType:   "AGENT_CONTENT",
		ContentType: state.ContentAgentResponse,
		Payload:     map[string]any{"text": "hello"},
	})
	return nil
}

func (w *stateWorld) receivedEnvelopeValid() error {
	got, err := readEvents(w.subs[0], 1, time.Second)
	if err != nil {
		return err
	}
	return got[0].Validate()
}

func (w *stateWorld) processingPublisher() error {
	w.proc = state.NewProcessingPublisher("active")
	return nil
}

func (w *stateWorld) currentStateIs(wantState, wantPhase string) error {
	cur := w.proc.Current()
	if err := cur.Validate(); err != nil {
		return err
	}
	if string(cur.State) != wantState || string(cur.Phase) != wantPhase {
		return fmt.Errorf("state=%q phase=%q, want state=%q phase=%q",
			cur.State, cur.Phase, wantState, wantPhase)
	}
	return nil
}

func (w *stateWorld) setWorking(phase string) error {
	w.proc.Set(state.StateWorking, state.Phase(phase))
	return nil
}

func (w *stateWorld) setCompleted() error {
	w.proc.Set(state.StateCompleted, state.PhaseNone)
	return nil
}

func readEvents(sub *state.Subscription, n int, timeout time.Duration) ([]state.Event, error) {
	got := make([]state.Event, 0, n)
	deadline := time.After(timeout)
	for len(got) < n {
		select {
		case ev := <-sub.C:
			got = append(got, ev)
		case <-deadline:
			return got, fmt.Errorf("timed out after %d/%d events", len(got), n)
		}
	}
	return got, nil
}
