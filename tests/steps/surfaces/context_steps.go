package surfacesteps

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cucumber/godog"

	"agentx/internal/state"
	contextsurface "agentx/internal/surfaces/context"
)

type contextWorld struct {
	model *contextsurface.Model
	ord   uint64
}

// registerContextSteps wires the context-viewer surface steps (SS-3).
func registerContextSteps(sc *godog.ScenarioContext) {
	w := &contextWorld{}

	sc.Step(`^a context surface sized (\d+) by (\d+)$`, w.sized)
	sc.Step(`^the context surface applies a user_prompt "([^"]*)"$`, w.applyUserPrompt)
	sc.Step(`^the context surface applies an agent_response "([^"]*)"$`, w.applyAgentResponse)
	sc.Step(`^the context surface applies an ephemeral agent_response "([^"]*)"$`, w.applyEphemeral)
	sc.Step(`^the context surface applies a thinking "([^"]*)"$`, w.applyThinking)
	sc.Step(`^the context surface applies processing-state "([^"]*)" "([^"]*)"$`, w.applyProcessing)
	sc.Step(`^the context view contains "([^"]*)"$`, w.viewContains)
	sc.Step(`^the context view does not contain "([^"]*)"$`, w.viewNotContains)
	sc.Step(`^the context status line shows "([^"]*)"$`, w.statusShows)
	sc.Step(`^the context view shows a selected object border$`, w.selectedBorder)
	sc.Step(`^the context surface receives key "([^"]*)"$`, w.receiveKey)
}

func (w *contextWorld) receiveKey(name string) error {
	if name == "space" {
		w.model.Key(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
		return nil
	}
	r := []rune(name)[0]
	w.model.Key(tea.KeyPressMsg{Code: r, Text: name})
	return nil
}

func (w *contextWorld) sized(width, height int) error {
	w.model = contextsurface.New(nil, "")
	w.model.SetSize(width, height)
	return nil
}

func (w *contextWorld) applyContent(et string, ct state.ContentType, text string) error {
	w.ord++
	w.model.Apply(state.Event{
		Epoch:       1,
		SessionID:   "s",
		EventType:   et,
		ContentType: ct,
		Ordinal:     w.ord,
		Payload:     map[string]any{"text": text},
		Enabled:     state.DefaultEnabled(ct),
	})
	return nil
}

func (w *contextWorld) applyUserPrompt(text string) error {
	return w.applyContent("USER_PROMPT", state.ContentUserPrompt, text)
}

func (w *contextWorld) applyAgentResponse(text string) error {
	return w.applyContent("AGENT_RESPONSE", state.ContentAgentResponse, text)
}

func (w *contextWorld) applyEphemeral(text string) error {
	w.model.Apply(state.Event{
		EventType:   "AGENT_RESPONSE",
		ContentType: state.ContentAgentResponse,
		Payload:     map[string]any{"text": text},
		Ephemeral:   true,
	})
	return nil
}

func (w *contextWorld) applyThinking(text string) error {
	return w.applyContent("THINKING", state.ContentThinking, text)
}

func (w *contextWorld) applyProcessing(runState, phase string) error {
	w.model.Apply(state.Event{
		Epoch:       1,
		SessionID:   "s",
		EventType:   "PROCESSING_STATE",
		ContentType: state.ContentProcessingState,
		Payload:     map[string]any{"state": runState, "phase": phase},
	})
	return nil
}

// statusLine returns the last rendered line (the processing-state line).
func (w *contextWorld) statusLine() string {
	lines := strings.Split(w.model.View(), "\n")
	return lines[len(lines)-1]
}

func (w *contextWorld) viewContains(want string) error {
	if !strings.Contains(w.model.View(), want) {
		return fmt.Errorf("context view does not contain %q", want)
	}
	return nil
}

func (w *contextWorld) viewNotContains(unwanted string) error {
	if strings.Contains(w.model.View(), unwanted) {
		return fmt.Errorf("context view unexpectedly contains %q", unwanted)
	}
	return nil
}

func (w *contextWorld) selectedBorder() error {
	if !strings.Contains(w.model.View(), "┏") {
		return fmt.Errorf("context view has no selected (heavy) object border; output not focused?")
	}
	return nil
}

func (w *contextWorld) statusShows(want string) error {
	if !strings.Contains(w.statusLine(), want) {
		return fmt.Errorf("status line %q does not show %q", w.statusLine(), want)
	}
	return nil
}
