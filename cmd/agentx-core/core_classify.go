package main

import (
	"strings"
	"unicode"
)

// ClassifyIntent is the intent category assigned during the classify phase.
type ClassifyIntent string

const (
	ClassifyIntentConversation  ClassifyIntent = "conversation"
	ClassifyIntentSimpleAction  ClassifyIntent = "simple_action"
	ClassifyIntentComplexAction ClassifyIntent = "complex_action"
	ClassifyIntentSafetyIssue   ClassifyIntent = "safety_issue"
)

// ClassifyNextStep is the routing decision produced by the classify phase.
type ClassifyNextStep string

const (
	ClassifyNextStepRespondDirectly ClassifyNextStep = "respond_directly"
	ClassifyNextStepSingleTool      ClassifyNextStep = "single_tool"
	ClassifyNextStepInvokePlanner   ClassifyNextStep = "invoke_planner"
	ClassifyNextStepEscalate        ClassifyNextStep = "escalate"
)

// ClassifyResult holds the deterministic output of the classify phase.
type ClassifyResult struct {
	Intent   ClassifyIntent   `json:"intent"`
	NextStep ClassifyNextStep `json:"next_step"`
}

// classifyPrompt applies deterministic, rule-based classification to a prompt.
// Evaluation order follows the system-prompt contract:
//  1. Safety check
//  2. Action complexity check
//  3. Routing assignment
//
// This function is synchronous and never calls the LLM.
func classifyPrompt(prompt string) ClassifyResult {
	lower := strings.ToLower(strings.TrimSpace(prompt))

	// Empty prompt → caller needs clarification.
	if lower == "" {
		return ClassifyResult{
			Intent:   ClassifyIntentComplexAction,
			NextStep: ClassifyNextStepInvokePlanner,
		}
	}

	// Safety check evaluated first per system-prompt contract.
	if hasSafetyPattern(lower) {
		return ClassifyResult{
			Intent:   ClassifyIntentSafetyIssue,
			NextStep: ClassifyNextStepEscalate,
		}
	}

	// Built-in commands (":clear", ":help", etc.) are conversation.
	if strings.HasPrefix(lower, ":") {
		return ClassifyResult{
			Intent:   ClassifyIntentConversation,
			NextStep: ClassifyNextStepRespondDirectly,
		}
	}

	// Complex-action heuristics: multi-sentence, long prompts, or planning keywords.
	if hasComplexActionPattern(lower) {
		return ClassifyResult{
			Intent:   ClassifyIntentComplexAction,
			NextStep: ClassifyNextStepInvokePlanner,
		}
	}

	// Simple-action heuristics: imperative verb at prompt start.
	if hasSimpleActionPattern(lower) {
		return ClassifyResult{
			Intent:   ClassifyIntentSimpleAction,
			NextStep: ClassifyNextStepSingleTool,
		}
	}

	// Default: treat as conversational exchange.
	return ClassifyResult{
		Intent:   ClassifyIntentConversation,
		NextStep: ClassifyNextStepRespondDirectly,
	}
}

// safetyPatterns are checked before any other routing decision.
var safetyPatterns = []string{
	"how to make a bomb",
	"how to kill",
	"synthesize drugs",
	"create malware",
	"hack into",
	"bypass security",
	"child exploitation",
}

func hasSafetyPattern(lower string) bool {
	for _, p := range safetyPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// complexActionKeywords trigger complex_action classification.
var complexActionKeywords = []string{
	"plan ",
	"plan my",
	"help me plan",
	"organize ",
	"multiple ",
	"step by step",
	"all of the",
	"entire ",
	"comprehensive ",
	"set up everything",
}

func hasComplexActionPattern(lower string) bool {
	for _, kw := range complexActionKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// Multi-sentence prompts are likely complex.
	sentences := 0
	for _, r := range lower {
		if r == '.' || r == '!' || r == '?' {
			sentences++
		}
	}
	if sentences >= 2 {
		return true
	}
	// Very long prompts are likely complex.
	return len(strings.Fields(lower)) > 30
}

// simpleActionVerbs are imperative verbs that signal a single-tool action.
var simpleActionVerbs = []string{
	"create", "add", "delete", "remove", "update", "rename",
	"move", "copy", "open", "close", "run", "execute", "start", "stop",
	"show", "list", "find", "search", "read", "write", "save",
	"remind", "schedule", "set", "toggle",
}

func hasSimpleActionPattern(lower string) bool {
	first := firstWord(lower)
	for _, v := range simpleActionVerbs {
		if first == v {
			return true
		}
	}
	return false
}

func firstWord(s string) string {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	idx := strings.IndexFunc(s, unicode.IsSpace)
	if idx < 0 {
		return s
	}
	return s[:idx]
}
