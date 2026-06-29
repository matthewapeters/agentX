package surfacesteps

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/state"
	"agentx/internal/surfaces/chat"
)

type roundTripWorld struct {
	bus        *state.Bus
	sub        *state.Subscription
	proc       *state.ProcessingPublisher
	procCh     <-chan state.ProcessingState
	procCancel func()
	model      chat.Model
	lastSubmit string
}

// registerRoundTripSteps wires the chat round-trip steps (CHT-B5).
func registerRoundTripSteps(sc *godog.ScenarioContext) {
	w := &roundTripWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.sub != nil {
			w.sub.Close()
		}
		if w.procCancel != nil {
			w.procCancel()
		}
		*w = roundTripWorld{}
		return ctx, err
	})

	sc.Step(`^a chat surface wired to a stub runtime$`, w.wiredSurface)
	sc.Step(`^the user submits "([^"]*)"$`, w.submit)
	sc.Step(`^the runtime received prompt "([^"]*)"$`, w.runtimeReceived)
	sc.Step(`^the output eventually contains "([^"]*)"$`, w.outputEventually)
	sc.Step(`^the chat status eventually shows "([^"]*)"$`, w.statusEventually)
}

func (w *roundTripWorld) wiredSurface() error {
	w.bus = state.NewBus()
	w.sub = w.bus.Subscribe()
	w.proc = state.NewProcessingPublisher("stub")
	w.procCh, w.procCancel = w.proc.Subscribe()

	bridge := chat.Bridge{
		Submit: func(text string) {
			w.lastSubmit = text
			w.bus.Publish(state.Event{
				Epoch: 1, SessionID: "stub", EventType: "USER_MESSAGE",
				ContentType: state.ContentUserPrompt, Payload: map[string]any{"text": text},
			})
			w.proc.Set(state.StateWorking, state.PhaseRespond)
			w.bus.Publish(state.Event{
				Epoch: 2, SessionID: "stub", EventType: "AGENT_CONTENT",
				ContentType: state.ContentAgentResponse, Payload: map[string]any{"text": "pong"},
			})
			w.proc.Set(state.StateCompleted, state.PhaseNone)
		},
		Events:     w.sub.C,
		Processing: w.procCh,
	}
	w.model = chat.NewWithBridge(bridge)
	w.updateModel(tea.WindowSizeMsg{Width: 40, Height: 16})
	return nil
}

func (w *roundTripWorld) updateModel(msg tea.Msg) tea.Cmd {
	m, cmd := w.model.Update(msg)
	w.model = m.(chat.Model)
	return cmd
}

func (w *roundTripWorld) submit(text string) error {
	for _, r := range text {
		w.updateModel(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if cmd := w.updateModel(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		cmd() // run the submit command synchronously (publishes stub events)
	}
	return nil
}

func (w *roundTripWorld) runtimeReceived(want string) error {
	if w.lastSubmit != want {
		return fmt.Errorf("runtime received %q, want %q", w.lastSubmit, want)
	}
	return nil
}

func (w *roundTripWorld) outputEventually(want string) error {
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(w.model.Output().View(), want) {
			return nil
		}
		select {
		case ev := <-w.sub.C:
			w.updateModel(chat.EventMsg(ev))
		case <-deadline:
			return fmt.Errorf("output never contained %q", want)
		}
	}
}

func (w *roundTripWorld) statusEventually(want string) error {
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(w.model.View().Content, want) {
			return nil
		}
		select {
		case ps := <-w.procCh:
			w.updateModel(chat.ProcessingStateMsg(ps))
		case <-deadline:
			return fmt.Errorf("status never showed %q", want)
		}
	}
}
