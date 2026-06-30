package surfacesteps

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
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
	sc.Step(`^the panel pages up$`, w.pageUp)
	sc.Step(`^the max widget lines is (\d+)$`, w.setMaxBody)
	sc.Step(`^a thinking event with (\d+) body lines is applied$`, w.applyThinkingLines)
	sc.Step(`^the selected widget is toggled$`, w.toggleSelected)
	sc.Step(`^the selected widget scrolls down by (\d+)$`, w.scrollSelected)
	sc.Step(`^the output view contains an unselected widget border$`, w.hasUnselectedBorder)
	sc.Step(`^the output view contains a selected widget border$`, w.hasSelectedBorder)
	sc.Step(`^the output view contains a scrollbar$`, w.hasScrollbar)
	sc.Step(`^the logo banner "([^"]*)" is set$`, w.setBanner)
	sc.Step(`^the logo banner precedes "([^"]*)" in the output$`, w.bannerPrecedes)
	sc.Step(`^the launch info is set for (\d+) surface kinds$`, w.setLaunchInfo)
	sc.Step(`^launch widget 0 is expanded$`, w.expandLaunch)
	sc.Step(`^the launch info precedes "([^"]*)" in the output$`, w.launchPrecedes)
	sc.Step(`^the output view contains "([^"]*)"$`, w.viewContains)
	sc.Step(`^the output view does not contain "([^"]*)"$`, w.viewNotContains)
	sc.Step(`^the output has (\d+) assistant entry$`, w.assistantEntries)
	sc.Step(`^no output line is wider than (\d+)$`, w.noLineWiderThan)
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

func (w *outputWorld) pageUp() error {
	w.panel.PageUp()
	return nil
}

func (w *outputWorld) setMaxBody(n int) error {
	w.panel.SetMaxBody(n)
	return nil
}

func (w *outputWorld) applyThinkingLines(n int) error {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%02d", i)
	}
	w.panel.Apply(state.Event{ContentType: state.ContentThinking, Payload: map[string]any{"text": strings.Join(lines, "\n")}})
	return nil
}

func (w *outputWorld) toggleSelected() error {
	w.panel.ToggleSelected()
	return nil
}

func (w *outputWorld) scrollSelected(n int) error {
	w.panel.ScrollSelected(n)
	return nil
}

func (w *outputWorld) hasUnselectedBorder() error {
	if !strings.Contains(w.panel.View(), "┌") {
		return fmt.Errorf("output view has no unselected (normal) widget border")
	}
	return nil
}

func (w *outputWorld) hasSelectedBorder() error {
	if !strings.Contains(w.panel.View(), "┏") {
		return fmt.Errorf("output view has no selected (heavy) widget border")
	}
	return nil
}

func (w *outputWorld) hasScrollbar() error {
	if !strings.Contains(w.panel.View(), "█") {
		return fmt.Errorf("output view has no scrollbar thumb")
	}
	return nil
}

func (w *outputWorld) setBanner(s string) error {
	w.panel.SetBanner(s)
	return nil
}

// launchMarker identifies the launch-info widget header in the rendered view.
const launchMarker = "Attach surfaces"

func (w *outputWorld) setLaunchInfo(n int) error {
	header := fmt.Sprintf("🔌 %s (%d) · http://127.0.0.1:8420 · enter to expand", launchMarker, n)
	lines := []string{"run in another terminal (token is for this session only):", ""}
	for i := 1; i <= n; i++ {
		lines = append(lines, fmt.Sprintf("agentx surface launch kind-%d --session s --connect http://127.0.0.1:8420 --token tkn", i))
	}
	w.panel.SetLaunchInfo(header, lines)
	return nil
}

func (w *outputWorld) expandLaunch() error {
	w.panel.ToggleCollapse(0)
	return nil
}

func (w *outputWorld) launchPrecedes(widgetText string) error {
	view := w.panel.View()
	li := strings.Index(view, launchMarker)
	wi := strings.Index(view, widgetText)
	if li < 0 {
		return fmt.Errorf("launch-info widget not found in output view")
	}
	if wi < 0 {
		return fmt.Errorf("widget text %q not found in output view", widgetText)
	}
	if li >= wi {
		return fmt.Errorf("launch info at %d does not precede widget %q at %d", li, widgetText, wi)
	}
	return nil
}

func (w *outputWorld) bannerPrecedes(widgetText string) error {
	view := w.panel.View()
	bi := strings.Index(view, "AGENTX-LOGO")
	wi := strings.Index(view, widgetText)
	if bi < 0 {
		return fmt.Errorf("banner not found in output view")
	}
	if wi < 0 {
		return fmt.Errorf("widget text %q not found in output view", widgetText)
	}
	if bi >= wi {
		return fmt.Errorf("banner at %d does not precede widget %q at %d", bi, widgetText, wi)
	}
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

func (w *outputWorld) noLineWiderThan(limit int) error {
	for _, line := range strings.Split(w.panel.View(), "\n") {
		if width := ansi.StringWidth(line); width > limit {
			return fmt.Errorf("line %q has display width %d, exceeds %d", line, width, limit)
		}
	}
	return nil
}

func (w *outputWorld) assistantEntries(want int) error {
	if got := w.panel.AssistantEntries(); got != want {
		return fmt.Errorf("assistant entries = %d, want %d", got, want)
	}
	return nil
}
