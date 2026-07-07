package planner

import "testing"

// The calm-pebble failure: a valid plan wrapped in a ```json fence must parse.
func TestParseToleratesFence(t *testing.T) {
	raw := "```json\n" + `{"plan":{"name":"Review","objective":"look then pick","dag":[
		{"id":"s1","deps":[],"task":{"tool":"list_dir","args":{"path":"."},"explanation":"list project structure"}},
		{"id":"s2","deps":["s1"],"step":{"description":"pick weakest feature","deliverable":"a chosen feature"}}]}}` + "\n```"
	plan, err := Parse("task-9", []byte(raw))
	if err != nil {
		t.Fatalf("Parse fenced plan: %v", err)
	}
	if len(plan.Records) != 2 {
		t.Fatalf("records = %d, want 2", len(plan.Records))
	}
	if plan.Records[0].ID != "task-9-1" || plan.Records[1].Deps[0] != "task-9-1" {
		t.Errorf("namespacing/deps wrong: %+v", plan.Records)
	}
}
