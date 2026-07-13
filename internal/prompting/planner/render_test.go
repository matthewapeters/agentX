package planner

import (
	"strings"
	"testing"
)

// ADR 0011: the system and user messages must partition cleanly — the catalog only
// ever in the system output, the goal/context only ever in the user output — so a
// caller can never accidentally flatten them back into one message.
func TestRenderSystemUserPartition(t *testing.T) {
	sys := RenderSystem(DefaultPromptTemplate, "- list_dir: args {path} (read)\n")
	usr := RenderUser(DefaultUserTemplate, "review the project", "project: agentX\n")

	if !strings.Contains(sys, "list_dir: args {path}") {
		t.Errorf("system message missing the catalog: %q", sys)
	}
	if strings.Contains(sys, "review the project") || strings.Contains(sys, "project: agentX") {
		t.Errorf("system message unexpectedly contains goal/context: %q", sys)
	}
	if !strings.Contains(usr, "review the project") || !strings.Contains(usr, "project: agentX") {
		t.Errorf("user message missing goal/context: %q", usr)
	}
	if strings.Contains(usr, "list_dir: args {path}") {
		t.Errorf("user message unexpectedly contains the catalog: %q", usr)
	}
	if strings.Contains(sys, "{{") || strings.Contains(usr, "{{") {
		t.Errorf("unfilled template placeholder left in output: sys=%q usr=%q", sys, usr)
	}
}

// The listing-bias guidance (vivid-beacon-2) must be conditional, not absolute — a
// regression guard against re-introducing the unconditional wording.
func TestSystemTemplateListingBiasIsConditional(t *testing.T) {
	if !strings.Contains(DefaultPromptTemplate, "UNLESS a listing of") {
		t.Errorf("listing-bias guidance is not conditioned on already-known directory contents")
	}
}
