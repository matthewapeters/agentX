package surfacesteps

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/session"
	"agentx/internal/surfaces/contextviz"
)

// ctxVizWorld drives the read-only context-visualizer surface (SS-7) by building
// a breakdown, applying it as a message, and inspecting the rendered meter.
type ctxVizWorld struct {
	model  contextviz.Model
	report session.ContextReport
}

func registerContextVizSteps(sc *godog.ScenarioContext) {
	w := &ctxVizWorld{}

	sc.Step(`^a context visualizer for session "([^"]*)"$`, w.surface)
	sc.Step(`^a visualizer window of (\d+) tokens for model "([^"]*)"$`, w.window)
	sc.Step(`^the visualizer class "([^"]*)" contributes (\d+) chars$`, w.contributes)
	sc.Step(`^the visualizer breakdown is applied$`, w.apply)
	sc.Step(`^the visualizer receives key "([^"]*)"$`, w.receiveKey)
	sc.Step(`^the visualizer view shows "([^"]*)"$`, w.viewShows)
	sc.Step(`^the visualizer view omits "([^"]*)"$`, w.viewOmits)
}

func (w *ctxVizWorld) update(msg tea.Msg) {
	m, _ := w.model.Update(msg)
	w.model = m.(contextviz.Model)
}

func (w *ctxVizWorld) surface(name string) error {
	w.model = contextviz.New(nil, contextviz.Options{SessionName: name})
	w.report = session.ContextReport{}
	w.update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return nil
}

func (w *ctxVizWorld) window(tokens int, model string) error {
	w.report.WindowTokens = tokens
	w.report.Model = model
	return nil
}

func (w *ctxVizWorld) contributes(class string, chars int) error {
	w.report.Components = append(w.report.Components, session.ContextComponent{Class: class, Chars: chars})
	return nil
}

func (w *ctxVizWorld) apply() error {
	w.update(contextviz.ReportMsg(w.report))
	return nil
}

// receiveKey feeds a single key. A read-only surface ignores mutation keys, so
// this is how the scenarios prove pressing them does not open an editor.
func (w *ctxVizWorld) receiveKey(name string) error {
	if name == "space" {
		w.update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
		return nil
	}
	r := []rune(name)[0]
	w.update(tea.KeyPressMsg{Code: r, Text: name})
	return nil
}

func (w *ctxVizWorld) viewShows(want string) error {
	if !strings.Contains(w.model.View().Content, want) {
		return fmt.Errorf("visualizer view does not show %q", want)
	}
	return nil
}

func (w *ctxVizWorld) viewOmits(unwanted string) error {
	if strings.Contains(w.model.View().Content, unwanted) {
		return fmt.Errorf("visualizer view unexpectedly shows %q", unwanted)
	}
	return nil
}
