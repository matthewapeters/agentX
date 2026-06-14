package main

import (
	"fmt"
	"strings"
	"testing"
)

type testOutputResponseFormatter struct {
	responseParts []string
	preview       string
}

type trackingOutputResponseFormatter struct {
	responseCalls int
	previewCalls  int
	marker        string
}

func (f *trackingOutputResponseFormatter) FormatResponse(raw string, width int) []string {
	f.responseCalls++
	return []string{fmt.Sprintf("%s: %s", f.marker, strings.TrimSpace(raw))}
}

func (f *trackingOutputResponseFormatter) FormatCollapsedPreview(raw string, width int) string {
	f.previewCalls++
	return f.marker
}

func (f testOutputResponseFormatter) FormatResponse(raw string, width int) []string {
	if f.responseParts != nil {
		return f.responseParts
	}
	return []string{""}
}

func (f testOutputResponseFormatter) FormatCollapsedPreview(raw string, width int) string {
	return f.preview
}

func TestNewOutputTurnRenderer_InvalidIndex(t *testing.T) {
	snapshot := outputWidgetSnapshot{Turns: []ChatTurn{{Prompt: "p", Response: "r"}}}
	if renderer, ok := newOutputTurnRenderer(snapshot, 0, 80, nil); ok || renderer != nil {
		t.Fatal("expected invalid turn index to return no renderer")
	}
}

func TestOutputTurnRenderer_RenderIncludesUserHeaderAndResponse(t *testing.T) {
	snapshot := outputWidgetSnapshot{
		SessionID: "sess",
		Turns:     []ChatTurn{{Prompt: "hello", Response: "world"}},
		PromptCycle: PromptCycleStatus{
			Thinking: PromptCyclePhase{State: "done"},
		},
	}
	view := newOutputWidgetViewState()
	view.normalize(snapshot.SessionID, len(snapshot.Turns))
	renderer, ok := newOutputTurnRenderer(snapshot, 1, 100, view)
	if !ok {
		t.Fatal("expected valid renderer")
	}
	lines := renderer.render()
	render := strings.Join(lines, "\n")
	if !strings.Contains(render, "👤 User: hello") {
		t.Fatalf("expected user header in render, got:\n%s", render)
	}
	if !strings.Contains(render, "🤖 Response: world") {
		t.Fatalf("expected response row in render, got:\n%s", render)
	}
}

func TestOutputTurnRenderer_DefaultFormatterMatchesPlainTextResponsePath(t *testing.T) {
	snapshot := outputWidgetSnapshot{
		SessionID: "sess",
		Turns:     []ChatTurn{{Prompt: "hello", Response: "response line one\nresponse line two"}},
	}
	view := newOutputWidgetViewState()
	view.normalize(snapshot.SessionID, len(snapshot.Turns))

	renderer, ok := newOutputTurnRendererWithFormatter(snapshot, 1, 80, view, nil)
	if !ok {
		t.Fatal("expected valid renderer")
	}
	render := strings.Join(renderer.render(), "\n")

	if !strings.Contains(render, "🤖 Response: response line one") {
		t.Fatalf("expected first wrapped response line in render, got:\n%s", render)
	}
	if !strings.Contains(render, "response line two") {
		t.Fatalf("expected second wrapped response line in render, got:\n%s", render)
	}
}

func TestOutputTurnRenderer_CustomFormatterAffectsResponseOnly(t *testing.T) {
	snapshot := outputWidgetSnapshot{
		SessionID: "sess",
		Turns:     []ChatTurn{{Prompt: "hello", Response: "original response"}},
	}
	view := newOutputWidgetViewState()
	view.normalize(snapshot.SessionID, len(snapshot.Turns))

	formatter := testOutputResponseFormatter{responseParts: []string{"formatted response"}}
	renderer, ok := newOutputTurnRendererWithFormatter(snapshot, 1, 80, view, formatter)
	if !ok {
		t.Fatal("expected valid renderer")
	}
	render := strings.Join(renderer.render(), "\n")

	if !strings.Contains(render, "🤖 Response: formatted response") {
		t.Fatalf("expected formatted response line in render, got:\n%s", render)
	}
	if strings.Contains(render, "Classification: formatted response") || strings.Contains(render, "Thinking: formatted response") {
		t.Fatalf("expected formatter to affect response entry only, got:\n%s", render)
	}
}

func TestOutputTurnRenderer_CollapsedResponseUsesFormatterPreview(t *testing.T) {
	snapshot := outputWidgetSnapshot{
		SessionID: "sess",
		Turns:     []ChatTurn{{Prompt: "hello", Response: "original response"}},
	}
	view := newOutputWidgetViewState()
	view.normalize(snapshot.SessionID, len(snapshot.Turns))
	view.collapsedEntries[outputEntryCollapsedKey(1, "response")] = true

	formatter := testOutputResponseFormatter{preview: "custom preview"}
	renderer, ok := newOutputTurnRendererWithFormatter(snapshot, 1, 80, view, formatter)
	if !ok {
		t.Fatal("expected valid renderer")
	}
	render := strings.Join(renderer.render(), "\n")

	if !strings.Contains(render, "[+] 🤖 Response: custom preview") {
		t.Fatalf("expected collapsed response preview from formatter, got:\n%s", render)
	}
}

func TestDefaultOutputResponseFormatter_EnforcesWidthAsHardBudget(t *testing.T) {
	formatter := DefaultOutputResponseFormatter()
	parts := formatter.FormatResponse("alpha beta gamma delta", 12)
	if len(parts) == 0 {
		t.Fatal("expected formatted response parts")
	}
	for _, part := range parts {
		if visibleDisplayWidth(part) > 12 {
			t.Fatalf("expected part width <= 12, got %d for %q", visibleDisplayWidth(part), part)
		}
	}
}

func TestDefaultOutputResponseFormatter_CollapsedPreviewWhitespaceIsNone(t *testing.T) {
	formatter := DefaultOutputResponseFormatter()
	preview := formatter.FormatCollapsedPreview("  \n\t  ", 20)
	if preview != "none" {
		t.Fatalf("expected whitespace preview to be none, got %q", preview)
	}
}

func TestDefaultOutputResponseFormatter_IsDeterministicAndIdempotent(t *testing.T) {
	formatter := DefaultOutputResponseFormatter()
	raw := "line one\nline two"
	width := 24

	first := formatter.FormatResponse(raw, width)
	second := formatter.FormatResponse(raw, width)
	if strings.Join(first, "\n") != strings.Join(second, "\n") {
		t.Fatalf("expected deterministic response formatting, got %q vs %q", first, second)
	}

	previewOne := formatter.FormatCollapsedPreview(raw, width)
	previewTwo := formatter.FormatCollapsedPreview(raw, width)
	if previewOne != previewTwo {
		t.Fatalf("expected deterministic collapsed preview, got %q vs %q", previewOne, previewTwo)
	}
}

func TestMarkdownOutputFormatter_TransformsMarkdownToTerminalSafeText(t *testing.T) {
	formatter := MarkdownOutputFormatter()
	raw := "# Heading\n* bullet\n1. numbered\n```go\nfmt.Println(`x`)\n```\n[docs](https://example.com) **bold** _italic_"
	parts := formatter.FormatResponse(raw, 80)
	rendered := strings.Join(parts, "\n")

	for _, want := range []string{
		"Heading",
		"- bullet",
		"1. numbered",
		"    fmt.Println(x)",
		"docs (https://example.com) bold italic",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected markdown render to contain %q, got:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "```") {
		t.Fatalf("expected code fences stripped from markdown render, got:\n%s", rendered)
	}
}

func TestMarkdownOutputFormatter_CollapsedPreviewUsesNormalizedMarkdown(t *testing.T) {
	formatter := MarkdownOutputFormatter()
	preview := formatter.FormatCollapsedPreview("# Title\n**bold** words", 20)
	if strings.Contains(preview, "#") || strings.Contains(preview, "**") {
		t.Fatalf("expected collapsed preview without markdown markers, got %q", preview)
	}
	if preview == "none" {
		t.Fatalf("expected non-empty preview for markdown content")
	}
}

func TestMarkdownOutputFormatter_EnforcesWidthAsHardBudget(t *testing.T) {
	formatter := MarkdownOutputFormatter()
	parts := formatter.FormatResponse("## Title\n- alpha beta gamma delta epsilon", 16)
	if len(parts) == 0 {
		t.Fatal("expected markdown formatted response parts")
	}
	for _, part := range parts {
		if visibleDisplayWidth(part) > 16 {
			t.Fatalf("expected markdown part width <= 16, got %d for %q", visibleDisplayWidth(part), part)
		}
	}
}

func TestResolveOutputResponseFormatterFromEnv_DefaultPlain(t *testing.T) {
	setWidgetTestEnv(t, map[string]string{"AGENTX_OUTPUT_FORMATTER": ""})
	formatter := ResolveOutputResponseFormatterFromEnv()
	if _, ok := formatter.(plainTextOutputFormatter); !ok {
		t.Fatalf("expected default env formatter to be plain text, got %T", formatter)
	}
}

func TestResolveOutputResponseFormatterFromEnv_Markdown(t *testing.T) {
	setWidgetTestEnv(t, map[string]string{"AGENTX_OUTPUT_FORMATTER": "markdown"})
	formatter := ResolveOutputResponseFormatterFromEnv()
	if _, ok := formatter.(markdownOutputFormatter); !ok {
		t.Fatalf("expected markdown env formatter, got %T", formatter)
	}
}

func TestRenderOutputWidgetWithViewStateAndFormatter_UsesInjectedFormatterInProductionPath(t *testing.T) {
	snapshot := outputWidgetSnapshot{
		SessionID: "sess",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "hello", Response: "world"}},
	}
	view := newOutputWidgetViewState()
	formatter := &trackingOutputResponseFormatter{marker: "custom-marker"}

	render := renderOutputWidgetWithViewStateAndFormatter(snapshot, 80, 120, view, formatter)
	if !strings.Contains(render, "🤖 Response: custom-marker: world") {
		t.Fatalf("expected injected formatter output in render, got:\n%s", render)
	}
	if formatter.responseCalls == 0 {
		t.Fatal("expected production render path to call custom response formatter")
	}
	if formatter.previewCalls != 0 {
		t.Fatalf("expected expanded default path to avoid preview formatter calls, got %d", formatter.previewCalls)
	}
}

func TestRenderOutputWidgetWithViewState_DefaultPathUnchangedWhenFormatterEnvDisabled(t *testing.T) {
	setWidgetTestEnv(t, map[string]string{"AGENTX_OUTPUT_FORMATTER": "plain"})
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "hello", Response: "**bold** text"}},
	}
	view := newOutputWidgetViewState()
	render := renderOutputWidgetWithViewStateAndFormatter(snapshot, 80, 120, view, nil)
	if !strings.Contains(render, "🤖 Response: **bold** text") {
		t.Fatalf("expected plain default response path when formatter env disabled, got:\n%s", render)
	}
}

func TestRenderOutputWidgetWithViewState_UsesMarkdownFormatterViaNormalEntryWhenEnabled(t *testing.T) {
	setWidgetTestEnv(t, map[string]string{"AGENTX_OUTPUT_FORMATTER": "markdown"})
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 1,
		Turns:     []ChatTurn{{Prompt: "hello", Response: "# Heading\n* item\n[site](https://example.com)"}},
	}
	view := newOutputWidgetViewState()
	render := renderOutputWidgetWithViewStateAndFormatter(snapshot, 80, 120, view, nil)

	for _, want := range []string{"🤖 Response: Heading", "- item", "site (https://example.com)"} {
		if !strings.Contains(render, want) {
			t.Fatalf("expected markdown formatter output %q, got:\n%s", want, render)
		}
	}
	if strings.Contains(render, "# Heading") || strings.Contains(render, "* item") {
		t.Fatalf("expected markdown markers removed in enabled path, got:\n%s", render)
	}
}

// Finding 2: regression test — fenced code at width=12 must never exceed 12 visible chars per line.
func TestMarkdownOutputFormatter_FencedCodeWidth12NeverExceedsBudget(t *testing.T) {
	formatter := MarkdownOutputFormatter()
	raw := "```go\nfunc longFunctionName(arg1 string, arg2 int) error {\n    return nil\n}\n```"
	parts := formatter.FormatResponse(raw, 12)
	if len(parts) == 0 {
		t.Fatal("expected non-empty parts for fenced code at width=12")
	}
	for _, part := range parts {
		if w := visibleDisplayWidth(part); w > 12 {
			t.Fatalf("finding 2: fenced code line exceeds width=12: got width=%d for %q", w, part)
		}
	}
}

// Finding 3: collapsed response fallback renders "none" when formatter returns empty string.
func TestOutputTurnRenderer_CollapsedResponseFallsBackToNoneWhenFormatterReturnsEmpty(t *testing.T) {
	snapshot := outputWidgetSnapshot{
		SessionID: "sess",
		Turns:     []ChatTurn{{Prompt: "hello", Response: "original"}},
	}
	view := newOutputWidgetViewState()
	view.normalize(snapshot.SessionID, len(snapshot.Turns))
	view.collapsedEntries[outputEntryCollapsedKey(1, "response")] = true

	// Formatter explicitly returns empty string to trigger the defensive fallback.
	formatter := testOutputResponseFormatter{preview: ""}
	renderer, ok := newOutputTurnRendererWithFormatter(snapshot, 1, 80, view, formatter)
	if !ok {
		t.Fatal("expected valid renderer")
	}
	render := strings.Join(renderer.render(), "\n")
	if !strings.Contains(render, "[+] 🤖 Response: none") {
		t.Fatalf("finding 3: expected collapsed empty preview fallback to render 'none', got:\n%s", render)
	}
}
