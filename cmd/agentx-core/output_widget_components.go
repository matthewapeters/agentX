package main

import (
	"fmt"
	"strings"
)

// collapsedPreviewEmpty is the sentinel rendered when FormatCollapsedPreview returns
// an empty string (e.g. formatter received empty/whitespace content).
const collapsedPreviewEmpty = "none"

type outputTurnRenderer struct {
	snapshot   outputWidgetSnapshot
	turnIndex  int
	paneWidth  int
	viewState  *outputWidgetViewState
	formatter  OutputResponseFormatter
	turn       ChatTurn
	prompt     string
	response   string
	classify   ClassifyResult
	containerFocused bool
	entryFocused     bool
}

func newOutputTurnRenderer(snapshot outputWidgetSnapshot, turnIndex int, paneWidth int, viewState *outputWidgetViewState) (*outputTurnRenderer, bool) {
	return newOutputTurnRendererWithFormatter(snapshot, turnIndex, paneWidth, viewState, nil)
}

func newOutputTurnRendererWithFormatter(snapshot outputWidgetSnapshot, turnIndex int, paneWidth int, viewState *outputWidgetViewState, formatter OutputResponseFormatter) (*outputTurnRenderer, bool) {
	if turnIndex < 1 || turnIndex > len(snapshot.Turns) {
		return nil, false
	}
	if formatter == nil {
		formatter = DefaultOutputResponseFormatter()
	}
	turn := snapshot.Turns[turnIndex-1]
	prompt := strings.TrimSpace(turn.Prompt)
	response := strings.TrimSpace(turn.Response)
	containerFocused := viewState != nil && viewState.focusedTurn == turnIndex
	entryFocused := containerFocused && viewState != nil && viewState.entryFocusMode
	return &outputTurnRenderer{
		snapshot:         snapshot,
		turnIndex:        turnIndex,
		paneWidth:        paneWidth,
		viewState:        viewState,
		formatter:        formatter,
		turn:             turn,
		prompt:           prompt,
		response:         response,
		classify:         classifyPrompt(prompt),
		containerFocused: containerFocused,
		entryFocused:     entryFocused,
	}, true
}

func (r *outputTurnRenderer) userPrefix() string {
	if r.containerFocused && (r.viewState == nil || !r.viewState.entryFocusMode) {
		return "↳"
	}
	return " "
}

func (r *outputTurnRenderer) entryPrefix(entry string) string {
	if !r.entryFocused {
		return " "
	}
	if r.viewState.normalizedFocusedEntry() == normalizeOutputEntry(entry) {
		return "▶"
	}
	return " "
}

func (r *outputTurnRenderer) affordance(entry string) string {
	if r.viewState != nil && r.viewState.entryCollapsed(r.turnIndex, entry) {
		return "[+]"
	}
	return "[-]"
}

func (r *outputTurnRenderer) latestSuffix() string {
	if r.turnIndex == len(r.snapshot.Turns) {
		return " [LATEST]"
	}
	return ""
}

func (r *outputTurnRenderer) boxInnerWidth(prefix string) int {
	// Keep entry text width deterministic and derived from pane budget.
	inner := outputWidgetTurnBoxInnerWidth(r.paneWidth)
	if inner < 12 {
		inner = 12
	}
	return inner
}

func (r *outputTurnRenderer) formatBoxLine(prefix string, content string, width int) string {
	if width < 12 {
		width = 12
	}
	line := content
	if visibleDisplayWidth(line) > width {
		line = renderTruncate(stripAnsi(line), width, "...")
	}
	return fmt.Sprintf("%s │ %s │", prefix, padVisibleWidth(line, width))
}

func (r *outputTurnRenderer) appendEntry(prefix string, entry string, icon string, label string, content string, collapsed bool) []string {
	linePrefix := fmt.Sprintf("%s %s %s %s: ", prefix, r.affordance(entry), icon, label)
	innerWidth := r.boxInnerWidth(prefix)
	if collapsed {
		body := "[collapsed]"
		if normalizeOutputEntry(entry) == "response" {
			body = r.formatter.FormatCollapsedPreview(content, outputWidgetContentBudget(r.paneWidth, linePrefix))
			if body == "" {
				body = collapsedPreviewEmpty
			}
		}
		return []string{r.formatBoxLine(prefix, fmt.Sprintf("%s %s %s: %s", r.affordance(entry), icon, label, body), innerWidth)}
	}

	parts := wrapOutputWidgetContent(content, outputWidgetContentBudget(r.paneWidth, linePrefix))
	if normalizeOutputEntry(entry) == "response" {
		parts = r.formatter.FormatResponse(content, outputWidgetContentBudget(r.paneWidth, linePrefix))
	}
	if len(parts) == 0 {
		parts = []string{""}
	}
	entryLines := []string{r.formatBoxLine(prefix, fmt.Sprintf("%s %s %s: %s", r.affordance(entry), icon, label, parts[0]), innerWidth)}
	continuationPrefix := " "
	for _, part := range parts[1:] {
		entryLines = append(entryLines, r.formatBoxLine(continuationPrefix, part, innerWidth))
	}
	return entryLines
}

func (r *outputTurnRenderer) render() []string {
	lines := []string{}
	promptLines := strings.Split(r.prompt, "\n")
	if len(promptLines) == 0 {
		promptLines = []string{""}
	}
	lines = append(lines, fmt.Sprintf("%s 👤 User: %s%s", r.userPrefix(), promptLines[0], r.latestSuffix()))
	for _, promptLine := range promptLines[1:] {
		lines = append(lines, fmt.Sprintf("  %s", promptLine))
	}
	boxInnerWidth := r.boxInnerWidth(" ")
	lines = append(lines, fmt.Sprintf("  ┌%s┐", strings.Repeat("─", boxInnerWidth+2)))
	lines = append(lines, r.appendEntry(
		r.entryPrefix("classification"),
		"classification",
		"⚙️",
		"Classification",
		fmt.Sprintf("%s -> %s", r.classify.Intent, r.classify.NextStep),
		r.viewState != nil && r.viewState.entryCollapsed(r.turnIndex, "classification"),
	)...)
	lines = append(lines, r.appendEntry(
		r.entryPrefix("thinking"),
		"thinking",
		"💭",
		"Thinking",
		renderOutputWidgetThinking(r.snapshot),
		r.viewState != nil && r.viewState.entryCollapsed(r.turnIndex, "thinking"),
	)...)
	lines = append(lines, r.appendEntry(
		r.entryPrefix("response"),
		"response",
		"🤖",
		"Response",
		r.response,
		r.viewState != nil && r.viewState.entryCollapsed(r.turnIndex, "response"),
	)...)
	lines = append(lines, fmt.Sprintf("  └%s┘", strings.Repeat("─", boxInnerWidth+2)))
	if r.viewState != nil {
		if offset := r.viewState.turnScrollOffset(r.turnIndex); offset > 0 && len(lines) > 1 {
			content := lines[1:]
			if offset >= len(content) {
				lines = []string{lines[0]}
			} else {
				lines = append([]string{lines[0]}, content[offset:]...)
			}
		}
	}
	return lines
}
