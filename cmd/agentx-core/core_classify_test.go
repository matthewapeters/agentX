package main

import "testing"

func TestClassifyPrompt_EmptyPromptIsComplexAction(t *testing.T) {
	result := classifyPrompt("")
	if result.Intent != ClassifyIntentComplexAction {
		t.Fatalf("expected complex_action for empty prompt, got %q", result.Intent)
	}
	if result.NextStep != ClassifyNextStepInvokePlanner {
		t.Fatalf("expected invoke_planner for empty prompt, got %q", result.NextStep)
	}
}

func TestClassifyPrompt_BuiltInCommandIsConversation(t *testing.T) {
	for _, cmd := range []string{":clear", ":help", ":reset"} {
		result := classifyPrompt(cmd)
		if result.Intent != ClassifyIntentConversation {
			t.Fatalf("expected conversation for %q, got %q", cmd, result.Intent)
		}
		if result.NextStep != ClassifyNextStepRespondDirectly {
			t.Fatalf("expected respond_directly for %q, got %q", cmd, result.NextStep)
		}
	}
}

func TestClassifyPrompt_SafetyPatternEscalates(t *testing.T) {
	result := classifyPrompt("how to make a bomb at home")
	if result.Intent != ClassifyIntentSafetyIssue {
		t.Fatalf("expected safety_issue, got %q", result.Intent)
	}
	if result.NextStep != ClassifyNextStepEscalate {
		t.Fatalf("expected escalate, got %q", result.NextStep)
	}
}

func TestClassifyPrompt_SimpleActionVerb(t *testing.T) {
	for _, prompt := range []string{"list the files here", "create a new task", "delete session"} {
		result := classifyPrompt(prompt)
		if result.Intent != ClassifyIntentSimpleAction {
			t.Fatalf("expected simple_action for %q, got %q", prompt, result.Intent)
		}
		if result.NextStep != ClassifyNextStepSingleTool {
			t.Fatalf("expected single_tool for %q, got %q", prompt, result.NextStep)
		}
	}
}

func TestClassifyPrompt_PlanningKeywordIsComplexAction(t *testing.T) {
	for _, prompt := range []string{"plan my week", "help me plan the sprint", "organize the backlog"} {
		result := classifyPrompt(prompt)
		if result.Intent != ClassifyIntentComplexAction {
			t.Fatalf("expected complex_action for %q, got %q", prompt, result.Intent)
		}
	}
}

func TestClassifyPrompt_QuestionIsConversation(t *testing.T) {
	result := classifyPrompt("what is the current session ID")
	if result.Intent != ClassifyIntentConversation {
		t.Fatalf("expected conversation for question, got %q", result.Intent)
	}
	if result.NextStep != ClassifyNextStepRespondDirectly {
		t.Fatalf("expected respond_directly for question, got %q", result.NextStep)
	}
}

func TestClassifyPrompt_LongPromptIsComplexAction(t *testing.T) {
	long := "this is a very long prompt that has many words in it and should therefore be classified as a complex action requiring the planner to decompose the request into multiple steps"
	result := classifyPrompt(long)
	if result.Intent != ClassifyIntentComplexAction {
		t.Fatalf("expected complex_action for long prompt, got %q", result.Intent)
	}
}

func TestClassifyPrompt_MultiSentenceIsComplexAction(t *testing.T) {
	result := classifyPrompt("Do this first. Then do that. Finally clean up.")
	if result.Intent != ClassifyIntentComplexAction {
		t.Fatalf("expected complex_action for multi-sentence prompt, got %q", result.Intent)
	}
}

func TestClassifyPrompt_ResultStoredInPromptCycle(t *testing.T) {
	core := NewAgentXCore(&Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-classify-1"})
	core.startPromptCycle()
	result := classifyPrompt("list the files here")
	core.finishClassifyPhase(result)

	cycle := core.promptCycleSnapshot()
	if cycle.ClassifyResult == nil {
		t.Fatal("expected classify result to be stored in prompt cycle, got nil")
	}
	if cycle.ClassifyResult.Intent != ClassifyIntentSimpleAction {
		t.Fatalf("expected simple_action in stored classify result, got %q", cycle.ClassifyResult.Intent)
	}
	if cycle.ClassifyResult.NextStep != ClassifyNextStepSingleTool {
		t.Fatalf("expected single_tool in stored classify result, got %q", cycle.ClassifyResult.NextStep)
	}
}
