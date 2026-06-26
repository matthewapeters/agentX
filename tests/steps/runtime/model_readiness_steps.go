package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/runtime"
)

type readinessWorld struct {
	dir      string
	orc      *runtime.Orchestrator
	checkErr error
}

// registerModelReadinessSteps wires the active-model readiness steps (CHT-C4).
func registerModelReadinessSteps(sc *godog.ScenarioContext) {
	w := &readinessWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = readinessWorld{}
		return ctx, err
	})

	sc.Step(`^a started orchestrator with model "([^"]*)" that is ready$`, w.startedReady)
	sc.Step(`^a started orchestrator with model "([^"]*)" that is unavailable$`, w.startedUnavailable)
	sc.Step(`^the model is checked$`, w.check)
	sc.Step(`^the model check passes$`, w.checkPasses)
	sc.Step(`^the model check fails clearly naming "([^"]*)"$`, w.checkFailsNaming)
}

func (w *readinessWorld) start(model string, stub stubModel) error {
	dir, err := os.MkdirTemp("", "agentx-ready-")
	if err != nil {
		return err
	}
	w.dir = dir
	w.orc = runtime.New(runtime.Settings{SessionRoot: dir, OllamaModel: model}, runtime.WithModel(stub))
	return w.orc.Start()
}

func (w *readinessWorld) startedReady(model string) error {
	return w.start(model, stubModel{})
}

func (w *readinessWorld) startedUnavailable(model string) error {
	return w.start(model, stubModel{err: fmt.Errorf("connection refused")})
}

func (w *readinessWorld) check() error {
	w.checkErr = w.orc.CheckModel(context.Background())
	return nil
}

func (w *readinessWorld) checkPasses() error {
	if w.checkErr != nil {
		return fmt.Errorf("model check failed: %w", w.checkErr)
	}
	return nil
}

func (w *readinessWorld) checkFailsNaming(name string) error {
	if w.checkErr == nil {
		return fmt.Errorf("model check passed, want failure")
	}
	if !strings.Contains(w.checkErr.Error(), name) {
		return fmt.Errorf("error %q does not name model %q", w.checkErr.Error(), name)
	}
	return nil
}
