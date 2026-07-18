package wavefront

import "testing"

func TestParseKnowNeedAndToolItems(t *testing.T) {
	data := []byte(`{"classification": [
		{"KNOW": {"name": "language", "value": "Go"}},
		{"TOOL": {"name": "contents of main.go", "tool": "read_file", "args": {"path": "cmd/agentx/main.go"}}},
		{"NEED": {"name": "how the CLI is invoked"}}
	]}`)
	res, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Knows) != 1 || res.Knows[0].Name != "language" || res.Knows[0].Value != "Go" {
		t.Fatalf("Knows = %+v, want one {language, Go}", res.Knows)
	}
	if len(res.Tools) != 1 {
		t.Fatalf("Tools = %d, want 1", len(res.Tools))
	}
	tool := res.Tools[0]
	if tool.Command.Tool != "read_file" || tool.Command.Args["path"] != "cmd/agentx/main.go" {
		t.Errorf("Command = %+v, want read_file with path cmd/agentx/main.go", tool.Command)
	}
	if len(res.Needs) != 1 || res.Needs[0].Name != "how the CLI is invoked" {
		t.Fatalf("Needs = %+v, want one open question", res.Needs)
	}
}

func TestParseRejectsMoreOrFewerThanOneOfKnowNeedTool(t *testing.T) {
	cases := []string{
		`{"classification": [{}]}`,
		`{"classification": [{"KNOW": {"name": "a", "value": "b"}, "NEED": {"name": "c"}}]}`,
		`{"classification": [{"NEED": {"name": "c"}, "TOOL": {"name": "d", "tool": "read_file", "args": {}}}]}`,
	}
	for _, data := range cases {
		if _, err := Parse([]byte(data)); err == nil {
			t.Errorf("Parse(%s) succeeded, want an error (not exactly one of KNOW/NEED/TOOL)", data)
		}
	}
}

func TestParseRejectsToolWithNoTool(t *testing.T) {
	data := []byte(`{"classification": [
		{"TOOL": {"name": "x", "tool": "", "args": {}}}
	]}`)
	if _, err := Parse(data); err == nil {
		t.Error("Parse succeeded for a TOOL with no tool id, want an error")
	}
}

func TestParseRejectsEmptyName(t *testing.T) {
	cases := []string{
		`{"classification": [{"KNOW": {"name": "", "value": "x"}}]}`,
		`{"classification": [{"NEED": {"name": ""}}]}`,
		`{"classification": [{"TOOL": {"name": "", "tool": "read_file", "args": {}}}]}`,
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
	if len(res.Knows) != 0 || len(res.Needs) != 0 || len(res.Tools) != 0 {
		t.Errorf("Result = %+v, want empty", res)
	}
}
