package wavefront

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Mirrors decompose.TestLLMPlannerParsesReply: the ADR 0011 system/user partition
// regression guard, checked at the full LLMClassifier.Classify call level rather
// than just the template-render level prompts_test.go already covers.
func TestLLMClassifierRendersAndParses(t *testing.T) {
	var sawSystem, sawUser string
	var sawFormat json.RawMessage
	chat := func(_ context.Context, systemPrompt, userPrompt string, format json.RawMessage) (string, error) {
		sawSystem = systemPrompt
		sawUser = userPrompt
		sawFormat = format
		return `{"classification": [
			{"KNOW": {"name": "language", "value": "Go"}},
			{"TOOL": {"name": "entry point", "tool": "read_file", "args": {"path": "cmd/agentx/main.go"}}}
		]}`, nil
	}
	c := LLMClassifier{Chat: chat, Catalog: "- read_file: args {path} (read)\n"}
	res, err := c.Classify(context.Background(), "cwd: /demo\n", "what does this project do?")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	if !strings.Contains(sawUser, "what does this project do?") || !strings.Contains(sawUser, "cwd: /demo") {
		t.Errorf("user message missing working memory/question: %q", sawUser)
	}
	if !strings.Contains(sawSystem, "read_file: args {path}") {
		t.Errorf("system message missing the catalog: %q", sawSystem)
	}
	if strings.Contains(sawSystem, "what does this project do?") || strings.Contains(sawSystem, "cwd: /demo") {
		t.Errorf("system message unexpectedly contains per-call working memory/question: %q", sawSystem)
	}
	if strings.Contains(sawUser, "read_file: args {path}") {
		t.Errorf("user message unexpectedly contains the catalog: %q", sawUser)
	}
	if len(sawFormat) == 0 {
		t.Error("Classify did not pass a schema (Format) to Chat")
	}

	if len(res.Knows) != 1 || res.Knows[0].Value != "Go" {
		t.Fatalf("Knows = %+v, want one {language, Go}", res.Knows)
	}
	if len(res.Tools) != 1 || res.Tools[0].Command.Tool != "read_file" {
		t.Fatalf("Tools = %+v, want one resolved read_file command", res.Tools)
	}
}

func TestLLMClassifierDefaultsTemplateWhenEmpty(t *testing.T) {
	var sawSystem string
	chat := func(_ context.Context, systemPrompt, userPrompt string, format json.RawMessage) (string, error) {
		sawSystem = systemPrompt
		return `{"classification": []}`, nil
	}
	c := LLMClassifier{Chat: chat} // Template left empty
	if _, err := c.Classify(context.Background(), "wm", "q"); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if !strings.Contains(sawSystem, "KNOW") || !strings.Contains(sawSystem, "NEED") {
		t.Errorf("empty Template did not fall back to DefaultClassifyPromptTemplate: %q", sawSystem)
	}
}

func TestLLMClassifierPropagatesChatError(t *testing.T) {
	wantErr := "model unavailable"
	chat := func(context.Context, string, string, json.RawMessage) (string, error) {
		return "", errString(wantErr)
	}
	c := LLMClassifier{Chat: chat}
	_, err := c.Classify(context.Background(), "wm", "q")
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Errorf("err = %v, want it to propagate %q", err, wantErr)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
