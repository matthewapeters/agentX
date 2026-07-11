package runtime

import (
	"strings"
	"testing"

	"agentx/internal/tools"
)

// TestToolResultContextDirectsAnAnswerFromTheResult reproduces the proud-pebble fix
// directly: a successful tool result must tell the model to answer from it, not just
// dump the preview and a paging hint — without a directive (mirroring plan_cycle.go's
// planContext), a small local model regressed to proposing the command again instead of
// reporting what it found.
func TestToolResultContextDirectsAnAnswerFromTheResult(t *testing.T) {
	d := tools.Descriptor{ID: "tree"}
	res := tools.Result{Status: "ok", Exit: 0, Lines: 543, Preview: "00_START_HERE.md\nAGENTS.md\n…", Ref: "artifacts/1.txt"}

	got := toolResultContext(d, res)

	if !strings.Contains(got, res.Preview) {
		t.Errorf("toolResultContext = %q, want the preview still present", got)
	}
	if !strings.Contains(got, "already ran") {
		t.Errorf("toolResultContext = %q, want an explicit \"already ran\" directive", got)
	}
	if !strings.Contains(strings.ToLower(got), "do not propose running it") {
		t.Errorf("toolResultContext = %q, want an explicit instruction not to re-propose the command", got)
	}
}

// TestToolResultContextDirectiveSurvivesNoRef: a result with no artifact ref (e.g. a
// builtin with nothing to page) still gets the directive.
func TestToolResultContextDirectiveSurvivesNoRef(t *testing.T) {
	d := tools.Descriptor{ID: "list_dir"}
	res := tools.Result{Status: "ok", Exit: 0, Lines: 3, Preview: "a\nb\nc"}

	got := toolResultContext(d, res)
	if !strings.Contains(got, "already ran") {
		t.Errorf("toolResultContext = %q, want the directive even with no ref", got)
	}
}
