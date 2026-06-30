// Package surfacesteps holds Godog step definitions for the surfaces domain.
package surfacesteps

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/state"
	"agentx/internal/surfaces/chat"
)

type chatWorld struct {
	model    chat.Model
	cmd      tea.Cmd
	stopped  *bool
	approval *string
}

// InitializeScenario registers the surfaces-domain steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	registerChatSteps(sc)
	registerOutputSteps(sc)
	registerInputSteps(sc)
	registerRoundTripSteps(sc)
	registerRegistrySteps(sc)
	registerClientSteps(sc)
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
	sc.Step(`^a new chat surface sized (\d+) by (\d+)$`, w.newSurfaceSized)
	sc.Step(`^the user types "([^"]*)" and submits$`, w.typesAndSubmits)
	sc.Step(`^the chat output contains "([^"]*)"$`, w.chatOutputContains)
	sc.Step(`^the processing state becomes working in phase "([^"]*)"$`, w.processingWorking)
	sc.Step(`^the chat status shows "([^"]*)"$`, w.statusShows)
	sc.Step(`^a spinner tick advances the indicator$`, w.spinnerTickAdvances)
	sc.Step(`^ESC is pressed$`, w.pressEsc)
	sc.Step(`^the "([^"]*)" key is pressed$`, w.pressKey)
	sc.Step(`^the chat hint shows "([^"]*)"$`, w.statusShows)
	sc.Step(`^the output panel has focus$`, w.outputFocused)
	sc.Step(`^the chat view shows the active border color$`, w.viewHasActiveColor)
	sc.Step(`^the chat view shows the inactive border color$`, w.viewHasInactiveColor)
	sc.Step(`^a chat surface that records interrupts sized (\d+) by (\d+)$`, w.recordingSurfaceSized)
	sc.Step(`^the interrupt is confirmed$`, w.confirmInterrupt)
	sc.Step(`^the runtime is interrupted$`, w.runtimeInterrupted)
	sc.Step(`^the processing state becomes awaiting input$`, w.processingAwaiting)
	sc.Step(`^a chat surface that records approvals sized (\d+) by (\d+)$`, w.recordingApprovalsSized)
	sc.Step(`^the approval decision is "([^"]*)"$`, w.approvalDecisionIs)
	sc.Step(`^the chat input value is "([^"]*)"$`, w.chatInputValueIs)
	sc.Step(`^the chat view shows the flash border color$`, w.viewHasFlashColor)
}

func (w *chatWorld) chatInputValueIs(want string) error {
	if got := w.model.Input().Value(); got != want {
		return fmt.Errorf("chat input value = %q, want %q", got, want)
	}
	return nil
}

func (w *chatWorld) viewHasFlashColor() error {
	if !strings.Contains(w.model.View().Content, "7;38;5;6") {
		return fmt.Errorf("chat view does not show the flash border color")
	}
	return nil
}

func (w *chatWorld) pressEsc() error {
	w.update(tea.KeyPressMsg{Code: tea.KeyEscape})
	return nil
}

// pressKey sends a named key (arrows, pgup/pgdown, enter, or a single rune).
func (w *chatWorld) pressKey(name string) error {
	var msg tea.KeyPressMsg
	switch name {
	case "up":
		msg = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		msg = tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgup":
		msg = tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		msg = tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "enter":
		msg = tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		r := []rune(name)[0]
		msg = tea.KeyPressMsg{Code: r, Text: name}
	}
	w.update(msg)
	return nil
}

func (w *chatWorld) outputFocused() error {
	if !w.model.Output().Focused() {
		return fmt.Errorf("output panel does not have focus")
	}
	return nil
}

func (w *chatWorld) viewHasActiveColor() error {
	if !strings.Contains(w.model.View().Content, "38;5;6") {
		return fmt.Errorf("chat view does not show the active border color")
	}
	return nil
}

func (w *chatWorld) viewHasInactiveColor() error {
	if !strings.Contains(w.model.View().Content, "38;5;240") {
		return fmt.Errorf("chat view does not show the inactive border color")
	}
	return nil
}

func (w *chatWorld) recordingSurfaceSized(width, height int) error {
	stopped := false
	w.stopped = &stopped
	bridge := chat.Bridge{Stop: func() { stopped = true }}
	w.model = chat.NewWithBridge(bridge)
	w.update(tea.WindowSizeMsg{Width: width, Height: height})
	return nil
}

func (w *chatWorld) processingAwaiting() error {
	w.update(chat.ProcessingStateMsg{State: state.StateAwaitingInput, Phase: state.PhaseTool})
	return nil
}

func (w *chatWorld) recordingApprovalsSized(width, height int) error {
	var decision string
	w.approval = &decision
	bridge := chat.Bridge{Approve: func(d string) { decision = d }}
	w.model = chat.NewWithBridge(bridge)
	w.update(tea.WindowSizeMsg{Width: width, Height: height})
	return nil
}

func (w *chatWorld) approvalDecisionIs(want string) error {
	if w.cmd != nil {
		w.cmd() // run the pending approveCmd so the bridge records the decision
	}
	if w.approval == nil || *w.approval != want {
		got := ""
		if w.approval != nil {
			got = *w.approval
		}
		return fmt.Errorf("approval decision = %q, want %q", got, want)
	}
	return nil
}

func (w *chatWorld) confirmInterrupt() error {
	if w.cmd != nil {
		w.cmd()
	}
	return nil
}

func (w *chatWorld) runtimeInterrupted() error {
	if w.stopped == nil || !*w.stopped {
		return fmt.Errorf("runtime was not interrupted")
	}
	return nil
}

// spinnerTickAdvances verifies the working spinner animates: a tick changes the
// rendered frame (only the spinner glyph differs between renders).
func (w *chatWorld) spinnerTickAdvances() error {
	before := w.model.View().Content
	w.update(spinner.TickMsg{})
	if after := w.model.View().Content; after == before {
		return fmt.Errorf("spinner did not advance on tick")
	}
	return nil
}

func (w *chatWorld) processingWorking(phase string) error {
	w.update(chat.ProcessingStateMsg{State: state.StateWorking, Phase: state.Phase(phase)})
	return nil
}

func (w *chatWorld) statusShows(want string) error {
	if !strings.Contains(w.model.View().Content, want) {
		return fmt.Errorf("chat status does not show %q", want)
	}
	return nil
}

func (w *chatWorld) newSurfaceSized(width, height int) error {
	w.model = chat.New()
	w.update(tea.WindowSizeMsg{Width: width, Height: height})
	return nil
}

func (w *chatWorld) typesAndSubmits(text string) error {
	for _, r := range text {
		w.update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	w.update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return nil
}

func (w *chatWorld) chatOutputContains(want string) error {
	if !strings.Contains(w.model.Output().View(), want) {
		return fmt.Errorf("chat output does not contain %q", want)
	}
	return nil
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
