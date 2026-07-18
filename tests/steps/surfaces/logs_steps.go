package surfacesteps

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/state"
	"agentx/internal/surfaces/logs"
)

// logsWorld drives the read-only logs surface (PD-LOGS, SS-9) by applying
// synthetic events, feeding key presses, and inspecting the rendered view.
type logsWorld struct {
	model *logs.Model
	seq   int
}

func registerLogsSteps(sc *godog.ScenarioContext) {
	w := &logsWorld{}

	sc.Step(`^a logs surface sized (\d+) by (\d+)$`, w.surface)
	sc.Step(`^a "([^"]*)" event with tool "([^"]*)" and payload "([^"]*)" is applied$`, w.applyEvent)
	sc.Step(`^(\d+) "([^"]*)" events are applied$`, w.applyN)
	sc.Step(`^(\d+) more "([^"]*)" events are applied$`, w.applyN)
	sc.Step(`^the logs surface receives key "([^"]*)"$`, w.key)
	sc.Step(`^the logs surface types "([^"]*)"$`, w.types)
	sc.Step(`^the logs view shows "([^"]*)"$`, w.viewShows)
	sc.Step(`^the logs view omits "([^"]*)"$`, w.viewOmits)
	sc.Step(`^the logs view has more than (\d+) line$`, w.viewHasMoreThanLines)
	sc.Step(`^the logs surface is capturing keys$`, w.capturingKeys)
}

func (w *logsWorld) surface(width, height int) error {
	w.model = logs.New()
	w.model.SetSize(width, height)
	w.seq = 0
	return nil
}

func (w *logsWorld) applyEvent(contentType, tool, payload string) error {
	w.model.Apply(state.Event{
		Epoch:       1,
		SessionID:   "s",
		EventType:   strings.ToUpper(contentType),
		ContentType: state.ContentType(contentType),
		Payload:     payload,
		ToolName:    tool,
	})
	return nil
}

// applyN applies n synthetic events of the given content type, each carrying
// a distinct "prompt-N" payload (a monotonic counter on the world, not per
// call) so later assertions can tell them apart.
func (w *logsWorld) applyN(n int, contentType string) error {
	for range n {
		w.seq++
		w.model.Apply(state.Event{
			Epoch:       1,
			SessionID:   "s",
			EventType:   strings.ToUpper(contentType),
			ContentType: state.ContentType(contentType),
			Payload:     fmt.Sprintf("prompt-%d", w.seq),
		})
	}
	return nil
}

func (w *logsWorld) key(name string) error {
	switch name {
	case "enter":
		w.model.Key(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "esc":
		w.model.Key(tea.KeyPressMsg{Code: tea.KeyEscape})
	case "backspace":
		w.model.Key(tea.KeyPressMsg{Code: tea.KeyBackspace})
	case "ctrl+c":
		w.model.Key(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	default:
		r := []rune(name)
		w.model.Key(tea.KeyPressMsg{Code: r[0], Text: name})
	}
	return nil
}

// types feeds text as a sequence of individual printable-character
// keypresses, the way a search pattern is typed one rune at a time.
func (w *logsWorld) types(text string) error {
	for _, r := range text {
		w.model.Key(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return nil
}

func (w *logsWorld) viewShows(want string) error {
	if !strings.Contains(w.model.View(), want) {
		return fmt.Errorf("logs view does not show %q (got %q)", want, w.model.View())
	}
	return nil
}

func (w *logsWorld) viewOmits(unwanted string) error {
	if strings.Contains(w.model.View(), unwanted) {
		return fmt.Errorf("logs view unexpectedly shows %q", unwanted)
	}
	return nil
}

// viewHasMoreThanLines counts content lines, excluding the trailing footer
// line, so a wrap assertion isn't muddied by the always-present footer.
func (w *logsWorld) viewHasMoreThanLines(n int) error {
	all := strings.Split(w.model.View(), "\n")
	content := all
	if len(content) > 0 {
		content = content[:len(content)-1]
	}
	if len(content) <= n {
		return fmt.Errorf("logs view has %d content lines, want more than %d", len(content), n)
	}
	return nil
}

func (w *logsWorld) capturingKeys() error {
	if !w.model.CapturesKeys() {
		return fmt.Errorf("logs surface is not capturing keys")
	}
	return nil
}
