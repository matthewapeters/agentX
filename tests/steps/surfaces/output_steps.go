package surfacesteps

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/state"
	"agentx/internal/surfaces/output"
)

type outputWorld struct {
	panel *output.Model
}

// registerOutputSteps wires the output-panel rendering steps (CHT-B2).
func registerOutputSteps(sc *godog.ScenarioContext) {
	w := &outputWorld{}

	sc.Step(`^an output panel sized (\d+) by (\d+)$`, w.panelSized)
	sc.Step(`^a user_prompt event "([^"]*)" is applied$`, w.applyUserPrompt)
	sc.Step(`^an agent_response event "([^"]*)" is applied$`, w.applyAgentResponse)
	sc.Step(`^a thinking event "([^"]*)" is applied$`, w.applyThinking)
	sc.Step(`^a tool_call event for "([^"]*)" is applied$`, w.applyToolCall)
	sc.Step(`^(\d+) numbered user events are applied$`, w.applyNumbered)
	sc.Step(`^entry (\d+) is toggled$`, w.toggleEntry)
	sc.Step(`^the panel scrolls up by (\d+)$`, w.scrollUp)
	sc.Step(`^the output view contains "([^"]*)"$`, w.viewContains)
	sc.Step(`^the output view does not contain "([^"]*)"$`, w.viewNotContains)
	sc.Step(`^the output has (\d+) assistant entry$`, w.assistantEntries)
}

func (w *outputWorld) panelSized(width, height int) error {
	w.panel = output.New()
	w.panel.SetSize(width, height)
	return nil
}

func (w *outputWorld) applyUserPrompt(text string) error {
	w.panel.Apply(state.Event{ContentType: state.ContentUserPrompt, Payload: map[string]any{"text": text}})
	return nil
}

func (w *outputWorld) applyAgentResponse(text string) error {
	w.panel.Apply(state.Event{ContentType: state.ContentAgentResponse, Payload: map[string]any{"text": text}})
	return nil
}

func (w *outputWorld) applyThinking(text string) error {
	w.panel.Apply(state.Event{ContentType: state.ContentThinking, Payload: map[string]any{"text": text}})
	return nil
}

func (w *outputWorld) applyToolCall(tool string) error {
	w.panel.Apply(state.Event{ContentType: state.ContentToolCall, ToolName: tool, Payload: map[string]any{"text": "args"}})
	return nil
}

func (w *outputWorld) applyNumbered(n int) error {
	for i := 1; i <= n; i++ {
		w.panel.Apply(state.Event{
			ContentType: state.ContentUserPrompt,
			Payload:     map[string]any{"text": fmt.Sprintf("msg-%02d", i)},
		})
	}
	return nil
}

func (w *outputWorld) toggleEntry(i int) error {
	w.panel.ToggleCollapse(i)
	return nil
}

func (w *outputWorld) scrollUp(n int) error {
	w.panel.ScrollUp(n)
	return nil
}

func (w *outputWorld) viewContains(want string) error {
	if !strings.Contains(w.panel.View(), want) {
		return fmt.Errorf("output view does not contain %q", want)
	}
	return nil
}

func (w *outputWorld) viewNotContains(unwanted string) error {
	if strings.Contains(w.panel.View(), unwanted) {
		return fmt.Errorf("output view unexpectedly contains %q", unwanted)
	}
	return nil
}

func (w *outputWorld) assistantEntries(want int) error {
	if got := w.panel.AssistantEntries(); got != want {
		return fmt.Errorf("assistant entries = %d, want %d", got, want)
	}
	return nil
}
