package runtimesteps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"

	"agentx/internal/classify"
	"agentx/internal/prompting"
)

type classifyWorld struct {
	retries int
	verdict classify.Verdict
}

// registerClassificationSteps wires the classifier steps (CHT-D3).
func registerClassificationSteps(sc *godog.ScenarioContext) {
	w := &classifyWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		*w = classifyWorld{}
		return ctx, err
	})

	sc.Step(`^a classifier with retries (\d+)$`, w.classifierWithRetries)
	sc.Step(`^the classifier model returns:$`, w.modelReturns)
	sc.Step(`^the classified route is "([^"]*)"$`, w.routeIs)
}

func (w *classifyWorld) classifierWithRetries(retries int) error {
	w.retries = retries
	return nil
}

// modelReturns builds a classifier whose model always returns the doc-string body
// and runs a classification, capturing the verdict.
func (w *classifyWorld) modelReturns(raw *godog.DocString) error {
	chat := func(context.Context, []prompting.Message) (string, error) {
		return raw.Content, nil
	}
	c := classify.New("", w.retries, chat)
	w.verdict = c.Classify(context.Background(), "some prompt")
	return nil
}

func (w *classifyWorld) routeIs(want string) error {
	if got := string(w.verdict.Route); got != want {
		return fmt.Errorf("classified route = %q, want %q", got, want)
	}
	return nil
}
