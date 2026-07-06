package runtimesteps

import (
	"context"
	"fmt"
	"os"

	"github.com/cucumber/godog"

	"agentx/internal/classify"
	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/session"
)

// contextBreakdownWorld drives Orchestrator.ContextBreakdown (SS-7): it starts an
// orchestrator with known instructions, seeds working memory, runs a turn to
// populate history, then inspects the per-class composition report.
type contextBreakdownWorld struct {
	dir    string
	orc    *runtime.Orchestrator
	report session.ContextReport
}

func registerContextBreakdownSteps(sc *godog.ScenarioContext) {
	w := &contextBreakdownWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = contextBreakdownWorld{}
		return ctx, err
	})

	sc.Step(`^an orchestrator for context breakdown with instructions "([^"]*)" answering "([^"]*)" with context window (\d+)$`, w.start)
	sc.Step(`^the context-breakdown fact "([^"]*)" is "([^"]*)"$`, w.setFact)
	sc.Step(`^the context-breakdown prompt "([^"]*)" is submitted$`, w.submit)
	sc.Step(`^the context breakdown is requested$`, w.request)
	sc.Step(`^the breakdown window is (\d+)$`, w.windowIs)
	sc.Step(`^the breakdown class "([^"]*)" has (\d+) chars$`, w.classChars)
	sc.Step(`^the breakdown includes class "([^"]*)"$`, w.includesClass)
	sc.Step(`^the breakdown omits class "([^"]*)"$`, w.omitsClass)
}

func (w *contextBreakdownWorld) start(instructions, reply string, window int) error {
	dir, err := os.MkdirTemp("", "agentx-cb-")
	if err != nil {
		return err
	}
	w.dir = dir
	classifierChat := func(context.Context, []prompting.Message) (string, error) {
		return `{"route": "respond_directly"}`, nil
	}
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: dir, OllamaModel: "stub", Instructions: instructions},
		runtime.WithModel(stubModel{deltas: []string{reply}, ctxLen: window}),
		runtime.WithClassifier(classify.New("", 0, classifierChat)),
	)
	return w.orc.Start()
}

func (w *contextBreakdownWorld) setFact(key, value string) error {
	return w.orc.SetFact(key, value)
}

func (w *contextBreakdownWorld) submit(text string) error {
	return w.orc.Submit(context.Background(), text)
}

func (w *contextBreakdownWorld) request() error {
	report, err := w.orc.ContextBreakdown()
	if err != nil {
		return err
	}
	w.report = report
	return nil
}

func (w *contextBreakdownWorld) windowIs(want int) error {
	if w.report.WindowTokens != want {
		return fmt.Errorf("breakdown window = %d, want %d", w.report.WindowTokens, want)
	}
	return nil
}

func (w *contextBreakdownWorld) component(class string) (session.ContextComponent, bool) {
	for _, c := range w.report.Components {
		if c.Class == class {
			return c, true
		}
	}
	return session.ContextComponent{}, false
}

func (w *contextBreakdownWorld) classChars(class string, want int) error {
	c, ok := w.component(class)
	if !ok {
		return fmt.Errorf("breakdown has no %q class", class)
	}
	if c.Chars != want {
		return fmt.Errorf("class %q = %d chars, want %d", class, c.Chars, want)
	}
	return nil
}

func (w *contextBreakdownWorld) includesClass(class string) error {
	c, ok := w.component(class)
	if !ok || c.Chars <= 0 {
		return fmt.Errorf("breakdown omits class %q (want a non-zero contribution)", class)
	}
	return nil
}

func (w *contextBreakdownWorld) omitsClass(class string) error {
	if _, ok := w.component(class); ok {
		return fmt.Errorf("breakdown unexpectedly includes class %q", class)
	}
	return nil
}
