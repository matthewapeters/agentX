package surfacesteps

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/surfaces/input"
)

type inputWorld struct {
	panel  *input.Model
	action input.Action
}

// registerInputSteps wires the input-panel control steps (CHT-B3).
func registerInputSteps(sc *godog.ScenarioContext) {
	w := &inputWorld{}

	sc.Step(`^a focused input panel$`, w.focusedPanel)
	sc.Step(`^the user types "([^"]*)"$`, w.types)
	sc.Step(`^the user presses enter$`, w.pressEnter)
	sc.Step(`^the user presses shift\+enter$`, w.pressShiftEnter)
	sc.Step(`^the user presses esc$`, w.pressEsc)
	sc.Step(`^the input is set streaming$`, w.setStreaming)
	sc.Step(`^the input value is "([^"]*)"$`, w.valueIs)
	sc.Step(`^the input reports a submit action$`, w.reportsSubmit)
	sc.Step(`^the input reports no action$`, w.reportsNone)
	sc.Step(`^the input reports a stop action$`, w.reportsStop)
}

func (w *inputWorld) focusedPanel() error {
	w.panel = input.New()
	w.action = input.ActionNone
	return nil
}

func (w *inputWorld) types(text string) error {
	for _, r := range text {
		w.panel.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return nil
}

func (w *inputWorld) pressEnter() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return nil
}

func (w *inputWorld) pressShiftEnter() error {
	w.panel.Update(tea.KeyPressMsg{Mod: tea.ModShift, Code: tea.KeyEnter})
	return nil
}

func (w *inputWorld) pressEsc() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	return nil
}

func (w *inputWorld) setStreaming() error {
	w.panel.SetStreaming(true)
	return nil
}

func (w *inputWorld) valueIs(want string) error {
	want = strings.ReplaceAll(want, `\n`, "\n")
	if got := w.panel.Value(); got != want {
		return fmt.Errorf("input value = %q, want %q", got, want)
	}
	return nil
}

func (w *inputWorld) reportsSubmit() error { return wantAction(w.action, input.ActionSubmit, "submit") }
func (w *inputWorld) reportsNone() error   { return wantAction(w.action, input.ActionNone, "none") }
func (w *inputWorld) reportsStop() error   { return wantAction(w.action, input.ActionStop, "stop") }

func wantAction(got, want input.Action, name string) error {
	if got != want {
		return fmt.Errorf("input action = %d, want %s", got, name)
	}
	return nil
}
