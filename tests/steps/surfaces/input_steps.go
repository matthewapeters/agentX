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
	sc.Step(`^a focused input panel containing "([^"]*)" with the cursor at (\d+)$`, w.focusedContaining)
	sc.Step(`^the user types "([^"]*)"$`, w.types)
	sc.Step(`^the user presses enter$`, w.pressEnter)
	sc.Step(`^the user presses shift\+enter$`, w.pressShiftEnter)
	sc.Step(`^the user presses esc$`, w.pressEsc)
	sc.Step(`^the user presses up$`, w.pressUp)
	sc.Step(`^the user presses down$`, w.pressDown)
	sc.Step(`^the user presses backspace$`, w.pressBackspace)
	sc.Step(`^the user presses left$`, w.pressLeft)
	sc.Step(`^the user presses right$`, w.pressRight)
	sc.Step(`^the user presses ctrl\+a$`, w.pressCtrlA)
	sc.Step(`^the user presses ctrl\+e$`, w.pressCtrlE)
	sc.Step(`^the user presses alt\+b$`, w.pressAltB)
	sc.Step(`^the user presses alt\+f$`, w.pressAltF)
	sc.Step(`^the panel is blurred$`, w.blur)
	sc.Step(`^the input has submitted prompt "([^"]*)"$`, w.submittedPrompt)
	sc.Step(`^the input is set streaming$`, w.setStreaming)
	sc.Step(`^the input value is "([^"]*)"$`, w.valueIs)
	sc.Step(`^the cursor is at (\d+)$`, w.cursorAt)
	sc.Step(`^the rendered input shows a cursor cell$`, w.showsCursor)
	sc.Step(`^the rendered input shows no cursor cell$`, w.showsNoCursor)
	sc.Step(`^the input reports a submit action$`, w.reportsSubmit)
	sc.Step(`^the input reports no action$`, w.reportsNone)
	sc.Step(`^the input reports a stop action$`, w.reportsStop)
	sc.Step(`^the input reports a history boundary$`, w.reportsBoundary)
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

// focusedContaining builds a focused panel with the given text and positions the
// cursor at pos by jumping to the start and stepping right (no test-only API).
func (w *inputWorld) focusedContaining(text string, pos int) error {
	w.panel = input.New()
	w.action = input.ActionNone
	if err := w.types(text); err != nil {
		return err
	}
	w.panel.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'a'})
	for range pos {
		w.panel.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	return nil
}

func (w *inputWorld) pressUp() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	return nil
}

func (w *inputWorld) pressBackspace() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	return nil
}

func (w *inputWorld) pressLeft() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	return nil
}

func (w *inputWorld) pressRight() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	return nil
}

func (w *inputWorld) pressCtrlA() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'a'})
	return nil
}

func (w *inputWorld) pressCtrlE() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'e'})
	return nil
}

func (w *inputWorld) pressAltB() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Mod: tea.ModAlt, Code: 'b'})
	return nil
}

func (w *inputWorld) pressAltF() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Mod: tea.ModAlt, Code: 'f'})
	return nil
}

func (w *inputWorld) blur() error {
	w.panel.Blur()
	return nil
}

func (w *inputWorld) cursorAt(want int) error {
	if got := w.panel.Cursor(); got != want {
		return fmt.Errorf("cursor = %d, want %d", got, want)
	}
	return nil
}

func (w *inputWorld) showsCursor() error {
	if !strings.Contains(w.panel.View(), "\x1b[7m") {
		return fmt.Errorf("rendered input shows no cursor cell")
	}
	return nil
}

func (w *inputWorld) showsNoCursor() error {
	if strings.Contains(w.panel.View(), "\x1b[7m") {
		return fmt.Errorf("rendered input unexpectedly shows a cursor cell")
	}
	return nil
}

func (w *inputWorld) pressDown() error {
	w.action = w.panel.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	return nil
}

// submittedPrompt drives a full submit so the prompt enters history, mirroring
// the chat surface (read value, then Reset) so the buffer is clear afterwards.
func (w *inputWorld) submittedPrompt(text string) error {
	if err := w.types(text); err != nil {
		return err
	}
	if act := w.panel.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); act != input.ActionSubmit {
		return fmt.Errorf("submitting %q reported action %d, want submit", text, act)
	}
	w.panel.Reset()
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

func (w *inputWorld) reportsBoundary() error {
	return wantAction(w.action, input.ActionHistoryBoundary, "history boundary")
}

func wantAction(got, want input.Action, name string) error {
	if got != want {
		return fmt.Errorf("input action = %d, want %s", got, name)
	}
	return nil
}
