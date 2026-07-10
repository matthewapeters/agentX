package decompose

import (
	"context"
	"strings"
	"testing"

	"agentx/internal/planfindings"
	"agentx/internal/prompting/planner"
	"agentx/internal/prompting/task"
	"agentx/internal/session"
)

// capturingPlanner records the contextText it was called with and returns a fixed,
// non-echoing plan — just enough to prove what Decompose actually sent to the LLM.
type capturingPlanner struct {
	gotContext string
}

func (p *capturingPlanner) Plan(_ context.Context, parentID, _, contextText string) (planner.Plan, error) {
	p.gotContext = contextText
	return planner.Plan{Records: []task.Record{
		{ID: parentID + "-1", Goal: "a concrete, unrelated child goal", Type: task.Query,
			Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
	}}, nil
}

// TestDecomposeGroundsInPlanFindings: the amber-quartz fix — a Step's children are
// generated with visibility into what this plan has already found, not just the
// parent's own goal text. Without this, "list the project root" was regenerated
// three separate times across one plan because each decompose call never saw that an
// earlier sibling had already done it.
func TestDecomposeGroundsInPlanFindings(t *testing.T) {
	p := &capturingPlanner{}
	d := Decomposer{Planner: p, SessionID: "s", MaxDepth: 10,
		Facts: func() []session.Fact { return []session.Fact{{Key: "cwd", Value: "/Projects/agentX"}} }}

	ctx := planfindings.WithSource(context.Background(), func() string {
		return "[This plan's findings so far]\n- list project root → done: go.mod, cmd/, internal/"
	})
	if _, err := d.Decompose(ctx, task.Record{ID: "task-1", Kind: task.KindStep, Goal: "review the project"}); err != nil {
		t.Fatalf("Decompose: %v", err)
	}

	if !strings.Contains(p.gotContext, "cwd: /Projects/agentX") {
		t.Errorf("contextText = %q, want the WM facts still present", p.gotContext)
	}
	if !strings.Contains(p.gotContext, "list project root → done: go.mod, cmd/, internal/") {
		t.Errorf("contextText = %q, want this plan's findings-so-far folded in", p.gotContext)
	}
}

// TestDecomposeWithoutPlanFindings: no plan-findings source attached (bare context) —
// contextText is exactly the WM facts, unchanged from before this fix.
func TestDecomposeWithoutPlanFindings(t *testing.T) {
	p := &capturingPlanner{}
	d := Decomposer{Planner: p, SessionID: "s", MaxDepth: 10,
		Facts: func() []session.Fact { return []session.Fact{{Key: "cwd", Value: "/Projects/agentX"}} }}

	if _, err := d.Decompose(context.Background(), task.Record{ID: "task-1", Kind: task.KindStep, Goal: "review the project"}); err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if p.gotContext != "cwd: /Projects/agentX\n" {
		t.Errorf("contextText = %q, want just the WM facts line with no plan-findings source attached", p.gotContext)
	}
}
