package main

import (
	"fmt"
	"testing"
)

func TestVisualRenderOutput(t *testing.T) {
	snapshot := outputWidgetSnapshot{
		SessionID: "sess-output",
		TurnCount: 2,
		Turns: []ChatTurn{
			{Prompt: "what is status?", Response: "all systems green"},
			{Prompt: "what is 2+2?", Response: "The answer is 4 because when you add 2 and 2 together, mathematically they equal 4."},
		},
		PromptCycle: PromptCycleStatus{
			Thinking: PromptCyclePhase{State: "done", ElapsedMs: 11},
		},
	}

	view := newOutputWidgetViewState()
	render := renderOutputWidgetWithViewState(snapshot, 200, 200, view)
	fmt.Println("=== RENDERED OUTPUT ===")
	fmt.Println(render)
	fmt.Println("=== END OUTPUT ===")
}
