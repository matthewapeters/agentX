package decompose

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"agentx/internal/prompting/task"
)

func TestLLMPlannerParsesReply(t *testing.T) {
	var sawPrompt string
	var sawFormat json.RawMessage
	chat := func(_ context.Context, prompt string, format json.RawMessage) (string, error) {
		sawPrompt = prompt
		sawFormat = format
		return `{"plan":{"name":"review","objective":"investigate then propose","dag":[
			{"id":"a","deps":[],"task":{"tool":"list_dir","args":{"path":"."},"explanation":"read packages"}},
			{"id":"b","deps":["a"],"step":{"description":"pick sparsest","deliverable":"a chosen module"}}]}}`, nil
	}
	p := LLMPlanner{Chat: chat, Catalog: "- list_dir: args {path} (read)\n"}
	plan, err := p.Plan(context.Background(), "task-7", "review the project", "project: agentX\n")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !strings.Contains(sawPrompt, "review the project") || !strings.Contains(sawPrompt, "project: agentX") {
		t.Errorf("prompt did not include goal + context: %q", sawPrompt)
	}
	if !strings.Contains(sawPrompt, "list_dir: args {path}") {
		t.Errorf("prompt did not include the catalog: %q", sawPrompt)
	}
	if len(sawFormat) == 0 {
		t.Error("Plan did not pass a schema (Format) to Chat")
	}
	if len(plan.Records) != 2 {
		t.Fatalf("records = %d, want 2", len(plan.Records))
	}
	// ids namespaced under the parent, deps remapped
	if plan.Records[0].ID != "task-7-1" || plan.Records[1].ID != "task-7-2" {
		t.Errorf("ids = %s,%s want task-7-1,task-7-2", plan.Records[0].ID, plan.Records[1].ID)
	}
	if plan.Records[0].Kind != task.KindTask {
		t.Errorf("a.Kind = %s, want task", plan.Records[0].Kind)
	}
	if plan.Records[1].Kind != task.KindStep {
		t.Errorf("b.Kind = %s, want step", plan.Records[1].Kind)
	}
	if len(plan.Records[1].Deps) != 1 || plan.Records[1].Deps[0] != "task-7-1" {
		t.Errorf("b deps = %v, want [task-7-1]", plan.Records[1].Deps)
	}
	if plan.Synthesis != "investigate then propose" {
		t.Errorf("synthesis = %q", plan.Synthesis)
	}
}
