package tools

import (
	"context"
	"testing"

	"agentx/internal/planfindings"
	"agentx/internal/prompting"
)

// TestProposeGroundsInFacts: a Proposer with Facts set folds a working-memory message
// between the catalog system message and the user message — the fix for the ungrounded
// single_tool path (clever-raven-3: "did you try tree?" resolved to read_file@/app with no
// cwd fact in sight).
func TestProposeGroundsInFacts(t *testing.T) {
	var sawMsgs []prompting.Message
	chat := func(_ context.Context, msgs []prompting.Message) (string, error) {
		sawMsgs = msgs
		return `{"tool":"tree","args":{"path":"/Projects/agentX"}}`, nil
	}
	p := NewProposer("CATALOG TEXT", 0, chat)
	p.Facts = func() []prompting.Fact {
		return []prompting.Fact{{Key: "cwd", Value: "/Projects/agentX"}}
	}

	prop, ok := p.Propose(context.Background(), "did you try tree?")
	if !ok || prop.Tool != "tree" {
		t.Fatalf("Propose = %+v, ok=%v", prop, ok)
	}
	if len(sawMsgs) != 3 {
		t.Fatalf("messages = %+v, want 3 (catalog system, facts system, user)", sawMsgs)
	}
	if sawMsgs[0].Content != "CATALOG TEXT" {
		t.Errorf("msg[0] = %+v, want the catalog system message first", sawMsgs[0])
	}
	if sawMsgs[1].Role != "system" || sawMsgs[1].Content == "" {
		t.Errorf("msg[1] = %+v, want a facts system message", sawMsgs[1])
	}
	if sawMsgs[2].Role != "user" || sawMsgs[2].Content != "did you try tree?" {
		t.Errorf("msg[2] = %+v, want the user message last, unchanged", sawMsgs[2])
	}
}

// TestProposeUngroundedWithoutFacts: Facts unset (nil) behaves exactly as before —
// no grounding message, no ripple to callers that don't set it.
func TestProposeUngroundedWithoutFacts(t *testing.T) {
	var sawMsgs []prompting.Message
	chat := func(_ context.Context, msgs []prompting.Message) (string, error) {
		sawMsgs = msgs
		return `{"tool":"none"}`, nil
	}
	p := NewProposer("CATALOG", 0, chat)
	_, _ = p.Propose(context.Background(), "hello")
	if len(sawMsgs) != 2 {
		t.Fatalf("messages = %+v, want 2 (catalog system, user) with Facts unset", sawMsgs)
	}
}

// TestProposeGroundsWithNoCatalogSystemMessage: an empty catalog (falls back to
// DefaultCatalog, still a non-empty system message) — insertFacts's "no leading system
// message" branch is exercised directly here since Assemble never actually omits it in
// practice (DefaultCatalog is always non-empty), guarding against future changes.
func TestInsertFactsNoLeadingSystem(t *testing.T) {
	msgs := []prompting.Message{{Role: "user", Content: "hi"}}
	out := insertFacts(msgs, func() []prompting.Fact {
		return []prompting.Fact{{Key: "project", Value: "agentX"}}
	})
	if len(out) != 2 || out[0].Role != "system" || out[1].Role != "user" {
		t.Fatalf("out = %+v, want [system(facts), user]", out)
	}
}

// TestProposeGroundsInPlanFindings: the amber-quartz fix — a tool-call resolution
// mid-decomposition sees what THIS plan has already found (via internal/planfindings,
// context-scoped), as a message separate from working-memory facts. Without this, a
// sibling's successful `tree` output was invisible to the very next Task's proposal,
// which hallucinated `cat src/main.py` in a Go project instead of using what tree had
// already found.
func TestProposeGroundsInPlanFindings(t *testing.T) {
	var sawMsgs []prompting.Message
	chat := func(_ context.Context, msgs []prompting.Message) (string, error) {
		sawMsgs = msgs
		return `{"tool":"read_file","args":{"path":"cmd/agentx/main.go"}}`, nil
	}
	p := NewProposer("CATALOG TEXT", 0, chat)
	p.Facts = func() []prompting.Fact {
		return []prompting.Fact{{Key: "cwd", Value: "/Projects/agentX"}}
	}

	ctx := planfindings.WithSource(context.Background(), func() string {
		return "[This plan's findings so far]\n- tree the project → done: cmd/agentx/main.go"
	})
	prop, ok := p.Propose(ctx, "Read the main entry point")
	if !ok || prop.Args["path"] != "cmd/agentx/main.go" {
		t.Fatalf("Propose = %+v, ok=%v, want it grounded in the real path from plan findings", prop, ok)
	}
	if len(sawMsgs) != 4 {
		t.Fatalf("messages = %+v, want 4 (catalog, facts, plan findings, user)", sawMsgs)
	}
	if sawMsgs[2].Role != "system" || sawMsgs[2].Content != "[This plan's findings so far]\n- tree the project → done: cmd/agentx/main.go" {
		t.Errorf("msg[2] = %+v, want the plan-findings system message after facts, before the user message", sawMsgs[2])
	}
	if sawMsgs[3].Role != "user" || sawMsgs[3].Content != "Read the main entry point" {
		t.Errorf("msg[3] = %+v, want the user message last, unchanged", sawMsgs[3])
	}
}

// TestProposeWithoutPlanFindings: no plan in flight (bare context, e.g. the
// single_tool cycle) — no plan-findings message, ripple-free.
func TestProposeWithoutPlanFindings(t *testing.T) {
	var sawMsgs []prompting.Message
	chat := func(_ context.Context, msgs []prompting.Message) (string, error) {
		sawMsgs = msgs
		return `{"tool":"none"}`, nil
	}
	p := NewProposer("CATALOG", 0, chat)
	_, _ = p.Propose(context.Background(), "hello")
	if len(sawMsgs) != 2 {
		t.Fatalf("messages = %+v, want 2 (catalog system, user) with no plan in flight", sawMsgs)
	}
}

func TestInsertPlanFindingsAfterFactsMessage(t *testing.T) {
	msgs := []prompting.Message{
		{Role: "system", Content: "catalog"},
		{Role: "system", Content: "facts"},
		{Role: "user", Content: "hi"},
	}
	out := insertPlanFindings(msgs, "plan findings text")
	if len(out) != 4 {
		t.Fatalf("out = %+v, want 4 messages", out)
	}
	if out[2].Role != "system" || out[2].Content != "plan findings text" {
		t.Errorf("out[2] = %+v, want the plan-findings message after both leading system messages", out[2])
	}
	if out[3].Role != "user" {
		t.Errorf("out[3] = %+v, want the user message last", out[3])
	}
}

func TestInsertPlanFindingsEmptyIsNoop(t *testing.T) {
	msgs := []prompting.Message{{Role: "system", Content: "catalog"}, {Role: "user", Content: "hi"}}
	out := insertPlanFindings(msgs, "")
	if len(out) != 2 {
		t.Fatalf("out = %+v, want unchanged 2 messages for empty findings", out)
	}
}
