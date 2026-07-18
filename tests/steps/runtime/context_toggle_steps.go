package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/classify"
	"agentx/internal/prompting"
	"agentx/internal/runtime"
	"agentx/internal/state"
)

// contextToggleWorld drives enable/disable of a conversation element (the context
// surface's management path) end to end: it submits turns, toggles an element, and
// inspects the captured context of the next turn plus the persisted event state.
type contextToggleWorld struct {
	dir       string
	orc       *runtime.Orchestrator
	captured []prompting.Message
	toggled  uint64 // ordinal of the element last toggled (checked for persistence)
}

func registerContextToggleSteps(sc *godog.ScenarioContext) {
	w := &contextToggleWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.orc != nil {
			shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = w.orc.Shutdown(shutCtx)
			cancel()
		}
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = contextToggleWorld{}
		return ctx, nil
	})

	sc.Step(`^an orchestrator that captures context and answers "([^"]*)"$`, w.start)
	sc.Step(`^the prompt "([^"]*)" is submitted for toggling$`, w.submit)
	sc.Step(`^the last agent response is disabled$`, w.disableLastResponse)
	sc.Step(`^the last agent response is re-enabled$`, w.enableLastResponse)
	sc.Step(`^the captured context includes "([^"]*)"$`, w.capturedIncludes)
	sc.Step(`^the captured context omits "([^"]*)"$`, w.capturedOmits)
	sc.Step(`^the recorded agent response is persisted disabled$`, w.persistedDisabled)
}

func (w *contextToggleWorld) start(reply string) error {
	dir, err := os.MkdirTemp("", "agentx-ct-")
	if err != nil {
		return err
	}
	w.dir = dir
	classifierChat := func(context.Context, []prompting.Message) (string, error) {
		return `{"route": "respond_directly"}`, nil
	}
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: dir, OllamaModel: "stub", Instructions: "Be brief."},
		runtime.WithModel(stubModel{deltas: []string{reply}, captured: &w.captured}),
		runtime.WithClassifier(classify.New("", 0, classifierChat)),
	)
	return w.orc.Start()
}

func (w *contextToggleWorld) submit(text string) error {
	return w.orc.Submit(context.Background(), text)
}

// lastResponseOrdinal returns the ordinal of the most recent complete agent_response,
// polling briefly since the recorder persists off the hot path (the event is on the
// bus the moment Submit returns but may not be on disk yet).
func (w *contextToggleWorld) lastResponseOrdinal() (uint64, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		hist, err := w.orc.History()
		if err != nil {
			return 0, err
		}
		var ord uint64
		for _, ev := range hist {
			if ev.ContentType == state.ContentAgentResponse {
				ord = ev.Ordinal
			}
		}
		if ord != 0 {
			return ord, nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("no agent_response recorded")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (w *contextToggleWorld) disableLastResponse() error {
	ord, err := w.lastResponseOrdinal()
	if err != nil {
		return err
	}
	w.toggled = ord
	return w.orc.SetEventEnabled(ord, false)
}

func (w *contextToggleWorld) enableLastResponse() error {
	if w.toggled == 0 {
		return fmt.Errorf("no element has been disabled to re-enable")
	}
	return w.orc.SetEventEnabled(w.toggled, true)
}

func (w *contextToggleWorld) capturedText() string {
	var b string
	for _, m := range w.captured {
		b += m.Role + ": " + m.Content + "\n"
	}
	return b
}

func (w *contextToggleWorld) capturedIncludes(sub string) error {
	if !strings.Contains(w.capturedText(), sub) {
		return fmt.Errorf("captured context omits %q; got:\n%s", sub, w.capturedText())
	}
	return nil
}

func (w *contextToggleWorld) capturedOmits(sub string) error {
	if strings.Contains(w.capturedText(), sub) {
		return fmt.Errorf("captured context unexpectedly includes %q; got:\n%s", sub, w.capturedText())
	}
	return nil
}

func (w *contextToggleWorld) persistedDisabled() error {
	hist, err := w.orc.History()
	if err != nil {
		return err
	}
	for _, ev := range hist {
		if ev.Ordinal == w.toggled {
			if ev.Enabled {
				return fmt.Errorf("recorded agent_response ordinal %d is still enabled on disk", w.toggled)
			}
			return nil
		}
	}
	return fmt.Errorf("recorded agent_response ordinal %d not found", w.toggled)
}
