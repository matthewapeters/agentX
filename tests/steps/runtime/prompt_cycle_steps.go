package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/session"
	"agentx/internal/state"
)

// stubModel is a deterministic Model: it emits a fixed set of deltas, fails with
// a fixed error, or (when block is set) streams nothing and waits for ctx
// cancellation — so the prompt cycle can be exercised without a live Ollama.
type stubModel struct {
	deltas []string
	err    error
	block  bool
}

func (s stubModel) Chat(ctx context.Context, _ string, _ []prompting.Message, onDelta func(string)) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	for _, d := range s.deltas {
		onDelta(d)
	}
	if s.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return strings.Join(s.deltas, ""), nil
}

func (s stubModel) Ready(context.Context, string) error { return s.err }

type promptCycleWorld struct {
	dir string
	orc *runtime.Orchestrator
}

// registerPromptCycleSteps wires the prompt-cycle orchestration steps (CHT-C3).
func registerPromptCycleSteps(sc *godog.ScenarioContext) {
	w := &promptCycleWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = promptCycleWorld{}
		return ctx, err
	})

	sc.Step(`^a started orchestrator with a stub model that streams "([^"]*)", "([^"]*)"$`, w.startedWithStreaming)
	sc.Step(`^a started orchestrator with a stub model that fails with "([^"]*)"$`, w.startedWithFailure)
	sc.Step(`^the prompt "([^"]*)" is submitted$`, w.submit)
	sc.Step(`^the orchestrator is shut down$`, w.shutdown)
	sc.Step(`^the recorded content events are, in order:$`, w.recordedContentEvents)
	sc.Step(`^an error event with "([^"]*)" is recorded$`, w.errorEventRecorded)
	sc.Step(`^the final processing state is "([^"]*)"$`, w.finalProcessingState)
}

func (w *promptCycleWorld) startWith(model runtime.Model) error {
	dir, err := os.MkdirTemp("", "agentx-pc-")
	if err != nil {
		return err
	}
	w.dir = dir
	w.orc = runtime.New(runtime.Settings{SessionRoot: dir, OllamaModel: "stub"}, runtime.WithModel(model))
	return w.orc.Start()
}

func (w *promptCycleWorld) startedWithStreaming(a, b string) error {
	return w.startWith(stubModel{deltas: []string{a, b}})
}

func (w *promptCycleWorld) startedWithFailure(msg string) error {
	return w.startWith(stubModel{err: fmt.Errorf("%s", msg)})
}

func (w *promptCycleWorld) submit(text string) error {
	// A model error is an expected outcome under test, not a step failure; the
	// resulting error event and failed state are asserted separately.
	_ = w.orc.Submit(context.Background(), text)
	return nil
}

func (w *promptCycleWorld) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return w.orc.Shutdown(ctx)
}

func (w *promptCycleWorld) timeline() ([]state.Event, error) {
	store := session.NewStore(w.dir)
	return store.Recorder(w.orc.Session().ID).Load()
}

func (w *promptCycleWorld) recordedContentEvents(table *godog.Table) error {
	events, err := w.timeline()
	if err != nil {
		return err
	}
	// Project to the renderable content events (drop processing-state snapshots).
	var got []state.Event
	for _, ev := range events {
		if ev.ContentType == state.ContentProcessingState {
			continue
		}
		got = append(got, ev)
	}

	want := table.Rows[1:] // skip the header row
	if len(got) != len(want) {
		return fmt.Errorf("recorded %d content events, want %d", len(got), len(want))
	}
	for i, row := range want {
		wantCT := row.Cells[0].Value
		wantText := row.Cells[1].Value
		if string(got[i].ContentType) != wantCT {
			return fmt.Errorf("event %d content_type = %q, want %q", i, got[i].ContentType, wantCT)
		}
		if text := eventText(got[i]); text != wantText {
			return fmt.Errorf("event %d text = %q, want %q", i, text, wantText)
		}
	}
	return nil
}

func (w *promptCycleWorld) errorEventRecorded(want string) error {
	events, err := w.timeline()
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.EventType == "ERROR" && strings.Contains(eventText(ev), want) {
			return nil
		}
	}
	return fmt.Errorf("no ERROR event containing %q in %d events", want, len(events))
}

func (w *promptCycleWorld) finalProcessingState(want string) error {
	cur := w.orc.Processing().Current()
	if string(cur.State) != want {
		return fmt.Errorf("final processing state = %q, want %q", cur.State, want)
	}
	return nil
}

// eventText extracts the "text" payload field of a recorded event.
func eventText(ev state.Event) string {
	if p, ok := ev.Payload.(map[string]any); ok {
		if t, ok := p["text"].(string); ok {
			return t
		}
	}
	return fmt.Sprintf("%v", ev.Payload)
}
