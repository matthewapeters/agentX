package decompose

import (
	"context"
	"strings"
	"testing"

	"agentx/internal/prompting/task"
	"agentx/internal/session"
)

// TestDecomposeGroundsInHistory reproduces the witty-falcon fix directly: a Step's
// children are generated with visibility into recent conversation, not just working-memory
// facts and this plan's own findings — without this, "proceed with the commands" had no
// way to resolve "the commands" to anything the model had actually just proposed.
func TestDecomposeGroundsInHistory(t *testing.T) {
	p := &capturingPlanner{}
	d := Decomposer{Planner: p, SessionID: "s", MaxDepth: 10,
		Facts:   func() []session.Fact { return []session.Fact{{Key: "cwd", Value: "/Projects/agentX"}} },
		History: func() string { return "user: yes, read any suspect files\nagent: let me run cat AGENTS.md" },
	}

	if _, err := d.Decompose(context.Background(), task.Record{ID: "task-1", Kind: task.KindStep, Goal: "proceed with the commands"}); err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if !strings.Contains(p.gotContext, "cwd: /Projects/agentX") {
		t.Errorf("contextText = %q, want the WM facts still present", p.gotContext)
	}
	if !strings.Contains(p.gotContext, "let me run cat AGENTS.md") {
		t.Errorf("contextText = %q, want the history digest folded in", p.gotContext)
	}
}

// TestDecomposeWithoutHistory: a nil History field leaves contextText exactly as it was
// before this fix — no regression for callers that don't wire it (e.g. existing tests).
func TestDecomposeWithoutHistory(t *testing.T) {
	p := &capturingPlanner{}
	d := Decomposer{Planner: p, SessionID: "s", MaxDepth: 10,
		Facts: func() []session.Fact { return []session.Fact{{Key: "cwd", Value: "/Projects/agentX"}} }}

	if _, err := d.Decompose(context.Background(), task.Record{ID: "task-1", Kind: task.KindStep, Goal: "review the project"}); err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if p.gotContext != "cwd: /Projects/agentX\n" {
		t.Errorf("contextText = %q, want just the WM facts line with History unset", p.gotContext)
	}
}

// TestDecomposeHistoryEmptyStringOmitted: a non-nil History that renders "" (cold start,
// no prior turns) contributes nothing — no empty "Recent conversation" block padding the
// prompt for no reason.
func TestDecomposeHistoryEmptyStringOmitted(t *testing.T) {
	p := &capturingPlanner{}
	d := Decomposer{Planner: p, SessionID: "s", MaxDepth: 10,
		Facts:   func() []session.Fact { return []session.Fact{{Key: "cwd", Value: "/Projects/agentX"}} },
		History: func() string { return "" },
	}

	if _, err := d.Decompose(context.Background(), task.Record{ID: "task-1", Kind: task.KindStep, Goal: "review the project"}); err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if p.gotContext != "cwd: /Projects/agentX\n" {
		t.Errorf("contextText = %q, want just the WM facts line when History renders empty", p.gotContext)
	}
}
