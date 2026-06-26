// Package surfacesteps holds Godog step definitions for the surfaces domain.
package surfacesteps

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/surfaces/chat"
)

type chatWorld struct {
	model chat.Model
	cmd   tea.Cmd
}

// InitializeScenario registers the surfaces-domain steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	registerChatSteps(sc)
}

// registerChatSteps wires the chat-surface layout steps (CHT-B1).
func registerChatSteps(sc *godog.ScenarioContext) {
	w := &chatWorld{}

	sc.Step(`^a new chat surface$`, w.newSurface)
	sc.Step(`^the terminal size is (\d+) by (\d+)$`, w.resize)
	sc.Step(`^the rendered view has (\d+) rows$`, w.viewRows)
	sc.Step(`^the output region is (\d+) rows$`, w.outputRows)
	sc.Step(`^the input region is (\d+) rows$`, w.inputRows)
	sc.Step(`^the input panel is focused$`, w.inputFocused)
	sc.Step(`^the user presses ctrl\+c$`, w.pressCtrlC)
	sc.Step(`^the surface requests quit$`, w.requestsQuit)
}

func (w *chatWorld) newSurface() error {
	w.model = chat.New()
	w.cmd = nil
	return nil
}

func (w *chatWorld) update(msg tea.Msg) {
	m, cmd := w.model.Update(msg)
	w.model = m.(chat.Model)
	w.cmd = cmd
}

func (w *chatWorld) resize(width, height int) error {
	w.update(tea.WindowSizeMsg{Width: width, Height: height})
	return nil
}

func (w *chatWorld) viewRows(want int) error {
	rows := strings.Count(w.model.View().Content, "\n") + 1
	if rows != want {
		return fmt.Errorf("rendered view has %d rows, want %d", rows, want)
	}
	return nil
}

func (w *chatWorld) outputRows(want int) error {
	if got := w.model.Output().Height(); got != want {
		return fmt.Errorf("output region is %d rows, want %d", got, want)
	}
	return nil
}

func (w *chatWorld) inputRows(want int) error {
	if got := w.model.Input().Height(); got != want {
		return fmt.Errorf("input region is %d rows, want %d", got, want)
	}
	return nil
}

func (w *chatWorld) inputFocused() error {
	if !w.model.Input().Focused() {
		return fmt.Errorf("input panel is not focused")
	}
	return nil
}

func (w *chatWorld) pressCtrlC() error {
	w.update(tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 'c'})
	return nil
}

func (w *chatWorld) requestsQuit() error {
	if w.cmd == nil {
		return fmt.Errorf("no command returned for ctrl+c")
	}
	if _, ok := w.cmd().(tea.QuitMsg); !ok {
		return fmt.Errorf("ctrl+c did not request quit")
	}
	return nil
}
