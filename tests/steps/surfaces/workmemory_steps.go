package surfacesteps

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/session"
	"agentx/internal/surfaces/workmemory"
)

type wmWorld struct {
	model workmemory.Model
	cmd   tea.Cmd
}

// registerWorkMemorySteps wires the working-memory editor surface steps (SS-6).
func registerWorkMemorySteps(sc *godog.ScenarioContext) {
	w := &wmWorld{}

	sc.Step(`^a working memory surface$`, w.surface)
	sc.Step(`^the surface loads:$`, w.loadFacts)
	sc.Step(`^the surface receives key "([^"]*)"$`, w.receiveKey)
	sc.Step(`^the surface types "([^"]*)"$`, w.typeText)
	sc.Step(`^the working memory view shows "([^"]*)"$`, w.viewShows)
	sc.Step(`^the working memory view does not show "([^"]*)"$`, w.viewOmits)
	sc.Step(`^the working memory view highlights "([^"]*)"$`, w.viewHighlights)
	sc.Step(`^the surface issued a command$`, w.issuedCommand)
}

func (w *wmWorld) update(msg tea.Msg) {
	model, cmd := w.model.Update(msg)
	w.model = model.(workmemory.Model)
	w.cmd = cmd
}

func (w *wmWorld) surface() error {
	w.model = workmemory.New(nil, workmemory.Options{SessionName: "calm-otter"})
	w.update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return nil
}

func (w *wmWorld) loadFacts(table *godog.Table) error {
	var facts []session.Fact
	for _, row := range table.Rows[1:] {
		facts = append(facts, session.Fact{
			Key:     row.Cells[0].Value,
			Value:   row.Cells[1].Value,
			Owner:   session.OwnerUser,
			Enabled: row.Cells[2].Value == "true",
		})
	}
	w.update(workmemory.FactsMsg(facts))
	return nil
}

func (w *wmWorld) receiveKey(name string) error {
	w.update(keyMsg(name))
	return nil
}

func (w *wmWorld) typeText(text string) error {
	for _, r := range text {
		w.update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return nil
}

// keyMsg maps a step key name to a KeyPressMsg.
func keyMsg(name string) tea.KeyPressMsg {
	switch name {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		r := []rune(name)[0]
		return tea.KeyPressMsg{Code: r, Text: name}
	}
}

func (w *wmWorld) viewShows(want string) error {
	if !strings.Contains(w.model.View().Content, want) {
		return fmt.Errorf("working memory view does not show %q", want)
	}
	return nil
}

func (w *wmWorld) viewOmits(unwanted string) error {
	if strings.Contains(w.model.View().Content, unwanted) {
		return fmt.Errorf("working memory view unexpectedly shows %q", unwanted)
	}
	return nil
}

// viewHighlights asserts the selection cursor sits on the line containing key.
func (w *wmWorld) viewHighlights(key string) error {
	for _, line := range strings.Split(w.model.View().Content, "\n") {
		if strings.HasPrefix(line, "› ") && strings.Contains(line, key) {
			return nil
		}
	}
	return fmt.Errorf("working memory view does not highlight %q", key)
}

func (w *wmWorld) issuedCommand() error {
	if w.cmd == nil {
		return fmt.Errorf("expected the surface to issue a command, got none")
	}
	return nil
}
