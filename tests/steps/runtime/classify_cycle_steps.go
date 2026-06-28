package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/classify"
	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/session"
	"agentx/internal/state"
)

type classifyCycleWorld struct {
	dir string
	orc *runtime.Orchestrator
}

// registerClassifyCycleSteps wires the classify-respond cycle steps (CHT-D4).
func registerClassifyCycleSteps(sc *godog.ScenarioContext) {
	w := &classifyCycleWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = classifyCycleWorld{}
		return ctx, err
	})

	sc.Step(`^a started orchestrator that classifies as "([^"]*)" and replies "([^"]*)"$`, w.started)
	sc.Step(`^the prompt "([^"]*)" runs the full cycle$`, w.runCycle)
	sc.Step(`^the cycle's content events are, in order:$`, w.contentEvents)
	sc.Step(`^the recorded classification route is "([^"]*)"$`, w.classificationRoute)
	sc.Step(`^the cycle's final state is "([^"]*)"$`, w.finalState)
}

func (w *classifyCycleWorld) started(route, reply string) error {
	dir, err := os.MkdirTemp("", "agentx-clscycle-")
	if err != nil {
		return err
	}
	w.dir = dir
	// Classifier and response model are independent stubs.
	classifierChat := func(context.Context, []prompting.Message) (string, error) {
		return fmt.Sprintf(`{"route": %q}`, route), nil
	}
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: dir, OllamaModel: "stub"},
		runtime.WithModel(stubModel{deltas: []string{reply}}),
		runtime.WithClassifier(classify.New("", 0, classifierChat)),
	)
	return w.orc.Start()
}

func (w *classifyCycleWorld) runCycle(text string) error {
	if err := w.orc.Submit(context.Background(), text); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return w.orc.Shutdown(ctx)
}

func (w *classifyCycleWorld) timeline() ([]state.Event, error) {
	store := session.NewStore(w.dir)
	return store.Recorder(w.orc.Session().ID).Load()
}

func (w *classifyCycleWorld) contentEvents(table *godog.Table) error {
	events, err := w.timeline()
	if err != nil {
		return err
	}
	var got []state.Event
	for _, ev := range events {
		if ev.ContentType == state.ContentProcessingState {
			continue
		}
		got = append(got, ev)
	}
	want := table.Rows[1:]
	if len(got) != len(want) {
		return fmt.Errorf("recorded %d content events, want %d", len(got), len(want))
	}
	for i, row := range want {
		if string(got[i].ContentType) != row.Cells[0].Value {
			return fmt.Errorf("event %d content_type = %q, want %q", i, got[i].ContentType, row.Cells[0].Value)
		}
	}
	return nil
}

func (w *classifyCycleWorld) classificationRoute(want string) error {
	events, err := w.timeline()
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.ContentType != state.ContentClassification {
			continue
		}
		if p, ok := ev.Payload.(map[string]any); ok {
			if route, _ := p["route"].(string); route == want {
				return nil
			}
			return fmt.Errorf("classification route = %v, want %q", p["route"], want)
		}
	}
	return fmt.Errorf("no classification event recorded")
}

func (w *classifyCycleWorld) finalState(want string) error {
	if got := string(w.orc.Processing().Current().State); got != want {
		return fmt.Errorf("final processing state = %q, want %q", got, want)
	}
	return nil
}
