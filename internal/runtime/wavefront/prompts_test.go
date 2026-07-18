package wavefront

import (
	"strconv"
	"strings"
	"testing"
)

// Mirrors planner.TestRenderSystemUserPartition (ADR 0011): system and user
// messages must partition cleanly, with no cross-contamination and no unfilled
// placeholder left in either.
func TestRenderClassifySystemUserPartition(t *testing.T) {
	sys := RenderClassifySystem(DefaultClassifyPromptTemplate, "- list_dir: args {path} (read)\n")
	usr := RenderClassifyUser(DefaultClassifyUserTemplate, "cwd: /demo\n", "what language is this?")

	if !strings.Contains(sys, "list_dir: args {path}") {
		t.Errorf("system message missing the catalog: %q", sys)
	}
	if strings.Contains(sys, "what language is this?") || strings.Contains(sys, "cwd: /demo") {
		t.Errorf("system message unexpectedly contains working memory/question: %q", sys)
	}
	if !strings.Contains(usr, "what language is this?") || !strings.Contains(usr, "cwd: /demo") {
		t.Errorf("user message missing working memory/question: %q", usr)
	}
	if strings.Contains(usr, "list_dir: args {path}") {
		t.Errorf("user message unexpectedly contains the catalog: %q", usr)
	}
	if strings.Contains(sys, "{{") || strings.Contains(usr, "{{") {
		t.Errorf("unfilled template placeholder left in output: sys=%q usr=%q", sys, usr)
	}
}

func TestRenderSynthesisUser(t *testing.T) {
	usr := RenderSynthesisUser(DefaultSynthesisUserTemplate, "language: Go\n", "what language is this?")
	if !strings.Contains(usr, "language: Go") || !strings.Contains(usr, "what language is this?") {
		t.Errorf("synthesis user message missing working memory/question: %q", usr)
	}
	if strings.Contains(usr, "{{") {
		t.Errorf("unfilled template placeholder left in output: %q", usr)
	}
}

func TestRenderSummarySystemFillsTargetChars(t *testing.T) {
	sys := RenderSummarySystem(DefaultSummaryPromptTemplate, 1500)
	if !strings.Contains(sys, strconv.Itoa(1500)) {
		t.Errorf("summary system prompt missing the target character budget: %q", sys)
	}
	if strings.Contains(sys, "{{") {
		t.Errorf("unfilled template placeholder left in output: %q", sys)
	}
}

func TestRenderSummaryUser(t *testing.T) {
	usr := RenderSummaryUser(DefaultSummaryUserTemplate, "1. top question\n2. this command", "raw tool output here")
	if !strings.Contains(usr, "top question") || !strings.Contains(usr, "raw tool output here") {
		t.Errorf("summary user message missing chain/output: %q", usr)
	}
	if strings.Contains(usr, "{{") {
		t.Errorf("unfilled template placeholder left in output: %q", usr)
	}
}

// Regression guard: the classify response is object-wrapped ({"classification": [...]})
// so it reuses jsonx.FirstObject unchanged, like every other JSON-producing call in
// this codebase — not a bare array, which would need a second, parallel
// fence-stripping mechanism just for this one prompt.
func TestClassifyUserTemplateWrapsResponseInObject(t *testing.T) {
	usr := RenderClassifyUser(DefaultClassifyUserTemplate, "wm", "q")
	if !strings.Contains(usr, `"classification"`) {
		t.Error("classify user template's reply-format spec must wrap the array under a \"classification\" key")
	}
}

// Regression guard: the classify prompt must never let a TOOL's command reference a
// value it hasn't actually got yet — this is the hallucination gap ADR 0012 exists to
// close, and the wording that prevents it must not silently erode.
func TestClassifyPromptForbidsGuessedCommandArgs(t *testing.T) {
	if !strings.Contains(DefaultClassifyPromptTemplate, "never from a fact you expect another item in this same response to produce") {
		t.Error("classify prompt no longer states the no-guessed-arguments rule")
	}
}

// Regression guard: the classify prompt must require every argument a tool needs, not
// just some of them — the incomplete-args gap that let list_dir/tree calls through with
// no "path" and get silently denied before any approval prompt.
func TestClassifyPromptRequiresCompleteToolArgs(t *testing.T) {
	if !strings.Contains(DefaultClassifyPromptTemplate, "supply every argument the\ntool requires") {
		t.Error("classify prompt no longer states the complete-arguments rule")
	}
}
