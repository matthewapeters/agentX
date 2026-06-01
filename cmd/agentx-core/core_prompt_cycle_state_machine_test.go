package main

import "testing"

func TestPromptCycle_InvalidTransitionStartRespondRejected(t *testing.T) {
	core := NewAgentXCore(&Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-transition-1"})

	core.startPromptCycle()
	core.startRespondPhase()

	cycle := core.promptCycleSnapshot()
	if cycle.Respond.State != "pending" {
		t.Fatalf("expected respond to remain pending on invalid transition, got %q", cycle.Respond.State)
	}
}

func TestPromptCycle_InvalidTransitionFinishToolRejected(t *testing.T) {
	core := NewAgentXCore(&Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-transition-2"})

	core.startPromptCycle()
	core.finishToolPhase()

	cycle := core.promptCycleSnapshot()
	if cycle.Tool.State != "pending" {
		t.Fatalf("expected tool phase to remain pending on invalid finish transition, got %q", cycle.Tool.State)
	}
}

func TestPromptCycle_ValidTransitionSequenceCompletes(t *testing.T) {
	core := NewAgentXCore(&Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-transition-3"})

	core.startPromptCycle()
	core.finishClassifyPhase(ClassifyResult{Intent: ClassifyIntentConversation, NextStep: ClassifyNextStepRespondDirectly})
	core.startThinkingPhase()
	core.finishThinkingPhase()
	core.startToolPhase()
	core.finishToolPhase()
	core.startRespondPhase()
	core.finishRespondPhase()

	cycle := core.promptCycleSnapshot()
	if cycle.Classify.State != "done" {
		t.Fatalf("expected classify done, got %q", cycle.Classify.State)
	}
	if cycle.Thinking.State != "done" {
		t.Fatalf("expected thinking done, got %q", cycle.Thinking.State)
	}
	if cycle.Tool.State != "done" {
		t.Fatalf("expected tool done, got %q", cycle.Tool.State)
	}
	if cycle.Respond.State != "done" {
		t.Fatalf("expected respond done, got %q", cycle.Respond.State)
	}
}

func TestPromptCycle_StartHasClassifyRunning(t *testing.T) {
	core := NewAgentXCore(&Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-transition-4"})

	core.startPromptCycle()

	cycle := core.promptCycleSnapshot()
	if cycle.Classify.State != "running" {
		t.Fatalf("expected classify running at cycle start, got %q", cycle.Classify.State)
	}
	if cycle.Thinking.State != "pending" {
		t.Fatalf("expected thinking pending at cycle start, got %q", cycle.Thinking.State)
	}
}

func TestPromptCycle_SkipToolPhaseAllowsRespond(t *testing.T) {
	core := NewAgentXCore(&Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-skip-tool-1"})

	core.startPromptCycle()
	core.finishClassifyPhase(ClassifyResult{Intent: ClassifyIntentConversation, NextStep: ClassifyNextStepRespondDirectly})
	core.startThinkingPhase()
	core.finishThinkingPhase()
	core.skipToolPhase()
	core.startRespondPhase()
	core.finishRespondPhase()

	cycle := core.promptCycleSnapshot()
	if cycle.Tool.State != "skipped" {
		t.Fatalf("expected tool skipped, got %q", cycle.Tool.State)
	}
	if cycle.Respond.State != "done" {
		t.Fatalf("expected respond done after skip-tool path, got %q", cycle.Respond.State)
	}
}

func TestPromptCycle_FailClassifyPhaseSetsFailedState(t *testing.T) {
	core := NewAgentXCore(&Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-fail-classify-1"})

	core.startPromptCycle()
	core.failClassifyPhase()

	cycle := core.promptCycleSnapshot()
	if cycle.Classify.State != "failed" {
		t.Fatalf("expected classify failed, got %q", cycle.Classify.State)
	}
	// After classify fails, thinking must not start (guard protects it).
	core.startThinkingPhase()
	cycle = core.promptCycleSnapshot()
	if cycle.Thinking.State != "pending" {
		t.Fatalf("expected thinking to remain pending after classify failure, got %q", cycle.Thinking.State)
	}
}

func TestPromptCycle_SkipToolFromNonPendingIsRejected(t *testing.T) {
	core := NewAgentXCore(&Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-skip-tool-2"})

	core.startPromptCycle()
	core.finishClassifyPhase(ClassifyResult{Intent: ClassifyIntentConversation, NextStep: ClassifyNextStepRespondDirectly})
	core.startThinkingPhase()
	core.finishThinkingPhase()
	core.startToolPhase() // put tool into running state
	core.skipToolPhase()  // must be rejected

	cycle := core.promptCycleSnapshot()
	if cycle.Tool.State != "running" {
		t.Fatalf("expected tool to remain running after rejected skip, got %q", cycle.Tool.State)
	}
}
