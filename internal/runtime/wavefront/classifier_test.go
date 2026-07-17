package wavefront

import "testing"

func TestParseKnowAndNeedItems(t *testing.T) {
	data := []byte(`{"classification": [
		{"KNOW": {"name": "language", "value": "Go"}},
		{"NEED": {"name": "contents of main.go", "command": {"tool": "read_file", "args": {"path": "cmd/agentx/main.go"}}}},
		{"NEED": {"name": "how the CLI is invoked"}}
	]}`)
	res, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Knows) != 1 || res.Knows[0].Name != "language" || res.Knows[0].Value != "Go" {
		t.Fatalf("Knows = %+v, want one {language, Go}", res.Knows)
	}
	if len(res.Needs) != 2 {
		t.Fatalf("Needs = %d, want 2", len(res.Needs))
	}
	commandNeed := res.Needs[0]
	if commandNeed.Command == nil {
		t.Fatal("first Need's Command is nil, want a resolved command")
	}
	if commandNeed.Command.Tool != "read_file" || commandNeed.Command.Args["path"] != "cmd/agentx/main.go" {
		t.Errorf("Command = %+v, want read_file with path cmd/agentx/main.go", commandNeed.Command)
	}
	openNeed := res.Needs[1]
	if openNeed.Command != nil {
		t.Errorf("second Need's Command = %+v, want nil (open question)", openNeed.Command)
	}
}

func TestParseRejectsBothOrNeitherOfKnowNeed(t *testing.T) {
	cases := []string{
		`{"classification": [{}]}`,
		`{"classification": [{"KNOW": {"name": "a", "value": "b"}, "NEED": {"name": "c"}}]}`,
	}
	for _, data := range cases {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("Parse(%s) succeeded, want an error (both/neither of KNOW/NEED)", data)
		}
	}
}

func TestParseRejectsCommandWithNoTool(t *testing.T) {
	data := []byte(`{"classification": [
		{"NEED": {"name": "x", "command": {"tool": "", "args": {}}}}
	]}`)
	if _, err := Parse(data); err == nil {
		t.Error("Parse succeeded for a command-valued NEED with no tool, want an error")
	}
}

func TestParseRejectsEmptyName(t *testing.T) {
	cases := []string{
		`{"classification": [{"KNOW": {"name": "", "value": "x"}}]}`,
		`{"classification": [{"NEED": {"name": ""}}]}`,
	}
	for _, data := range cases {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("Parse(%s) succeeded, want an error (empty name)", data)
		}
	}
}

// Fence-tolerant, matching every other jsonx.FirstObject consumer in this codebase.
func TestParseToleratesMarkdownFence(t *testing.T) {
	data := []byte("```json\n" + `{"classification": [{"KNOW": {"name": "a", "value": "b"}}]}` + "\n```")
	res, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Knows) != 1 {
		t.Fatalf("Knows = %d, want 1", len(res.Knows))
	}
}

func TestParseEmptyClassificationIsValid(t *testing.T) {
	res, err := Parse([]byte(`{"classification": []}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Knows) != 0 || len(res.Needs) != 0 {
		t.Errorf("Result = %+v, want empty", res)
	}
}
