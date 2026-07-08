package runtime

import (
	"context"
	"strings"
	"testing"

	"agentx/internal/executor"
	"agentx/internal/prompting/task"
	"agentx/internal/tools"
)

// stubTaskExec returns a fixed outcome regardless of the record.
type stubTaskExec struct{ out executor.Outcome }

func (s stubTaskExec) Execute(context.Context, task.Record) executor.Outcome { return s.out }

// stubArtifactReader maps a ref to its "full" stored content, mimicking the artifact store
// (whose Write always persists the complete output, independent of the tiny UI preview).
type stubArtifactReader map[string]string

func (r stubArtifactReader) Read(ref string, offset, limit int) ([]byte, error) {
	return []byte(r[ref]), nil
}

// TestCapturingExecWidensFindings reproduces the lively-raven bug directly: a result whose
// UI Preview is short (the executor's 20-line-style truncation) but whose full artifact
// content is long — findings must carry the WIDE content, not the narrow preview, or
// synthesis loses exactly the evidence (e.g. go.mod/cmd/ far down a tree listing) a short
// preview discards.
func TestCapturingExecWidensFindings(t *testing.T) {
	full := "00_START_HERE.md\nagentx.egg-info\n...\ngo.mod\ncmd/agentx/main.go\ninternal/\n"
	out := executor.Outcome{
		Status: executor.Executed,
		Result: tools.Result{Status: "ok", Preview: "00_START_HERE.md\nagentx.egg-info\n...", Ref: "ref-tree"},
	}
	c := &capturingExec{
		inner:  stubTaskExec{out: out},
		reader: stubArtifactReader{"ref-tree": full},
	}
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "get full recursive tree"})

	if len(c.steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(c.steps))
	}
	got := c.steps[0].findings
	if !strings.Contains(got, "go.mod") || !strings.Contains(got, "cmd/agentx/main.go") {
		t.Fatalf("findings did not carry the widened content: %q", got)
	}

	ctx := planContext(c.steps)
	if !strings.Contains(ctx, "go.mod") {
		t.Errorf("planContext output lost the widened findings: %q", ctx)
	}
}

// TestCapturingExecFallsBackToPreview: no reader, or an unknown ref, degrades to the UI
// preview rather than losing the finding entirely.
func TestCapturingExecFallsBackToPreview(t *testing.T) {
	out := executor.Outcome{
		Status: executor.Executed,
		Result: tools.Result{Status: "ok", Preview: "short preview only", Ref: "ref-missing"},
	}
	c := &capturingExec{inner: stubTaskExec{out: out}} // reader left nil
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "g"})

	if c.steps[0].findings != "short preview only" {
		t.Errorf("findings = %q, want the preview fallback", c.steps[0].findings)
	}
}

// TestCapturingExecNoRefFallsBackToPreview: a result with no Ref (e.g. a builtin that
// never wrote an artifact) also falls back cleanly.
func TestCapturingExecNoRefFallsBackToPreview(t *testing.T) {
	out := executor.Outcome{Status: executor.Executed, Result: tools.Result{Status: "ok", Preview: "preview text"}}
	c := &capturingExec{inner: stubTaskExec{out: out}, reader: stubArtifactReader{}}
	c.Execute(context.Background(), task.Record{ID: "t1", Goal: "g"})

	if c.steps[0].findings != "preview text" {
		t.Errorf("findings = %q, want the preview fallback", c.steps[0].findings)
	}
}
