package main

import (
	"strings"
	"testing"
)

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
