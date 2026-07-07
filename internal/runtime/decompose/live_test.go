package decompose

import (
	"context"
	"strings"
	"testing"
)

func TestHeuristicOneStep(t *testing.T) {
	h := HeuristicOneStep{}
	cases := map[string]bool{ // goal -> want one-step
		"read the go.mod file":                                                true,
		"list the packages":                                                   true,
		"review the current project and identify one under-developed feature": false, // " and <verb>" ⇒ compound
		"read every file then summarize the architecture":                     false, // " then "
		// tidy-cove spiral cases (ADR 0009): result plumbing is not a second step, and a
		// bare " and " between nouns is not clause chaining.
		"Run `ls -la` on /home/mpeters/Projects/agentX and capture its output": true,
		"List the files and save output to $OUTPUT":                           true,
		"enumerate all files and directories in the project root":             true,
		"do an exceedingly long thing with very many separate words that easily exceeds the generous default length limit here": false, // length
	}
	for goal, want := range cases {
		got, err := h.OneStep(context.Background(), goal)
		if err != nil {
			t.Fatalf("OneStep(%q): %v", goal, err)
		}
		if got != want {
			t.Errorf("OneStep(%q) = %v, want %v", goal, got, want)
		}
	}
}

func TestLLMPlannerParsesReply(t *testing.T) {
	var sawPrompt string
	chat := func(_ context.Context, prompt string) (string, error) {
		sawPrompt = prompt
		return `{"plan_name":"review","synthesis":"investigate then propose","steps":[
			{"id":"a","goal":"read packages","deps":[],"cost":1},
			{"id":"b","goal":"pick sparsest","deps":["a"],"cost":2}]}`, nil
	}
	p := LLMPlanner{Chat: chat}
	plan, err := p.Plan(context.Background(), "task-7", "review the project", "project: agentX\n")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !strings.Contains(sawPrompt, "review the project") || !strings.Contains(sawPrompt, "project: agentX") {
		t.Errorf("prompt did not include goal + context: %q", sawPrompt)
	}
	if len(plan.Records) != 2 {
		t.Fatalf("records = %d, want 2", len(plan.Records))
	}
	// ids namespaced under the parent, deps remapped
	if plan.Records[0].ID != "task-7-1" || plan.Records[1].ID != "task-7-2" {
		t.Errorf("ids = %s,%s want task-7-1,task-7-2", plan.Records[0].ID, plan.Records[1].ID)
	}
	if len(plan.Records[1].Deps) != 1 || plan.Records[1].Deps[0] != "task-7-1" {
		t.Errorf("b deps = %v, want [task-7-1]", plan.Records[1].Deps)
	}
	if plan.Synthesis != "investigate then propose" {
		t.Errorf("synthesis = %q", plan.Synthesis)
	}
}
