package decompose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agentx/internal/prompting/planner"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/scheduler"
	"agentx/internal/session"
)

// The exact goal chain from the tidy-cove spiral: every rung is the same action wearing
// different result plumbing. SimilarGoals must see through all of it.
var tidyCoveChain = []string{
	"Run `ls -la` on /home/mpeters/Projects/agentX and capture its output",
	"Run `ls -la` on /home/mpeters/Projects/agentX and save output to $OUTPUT",
	"Execute `ls -la /home/mpeters/Projects/agentX` and save output to $OUTPUT",
	"Run `ls -la /home/mpeters/Projects/agentX` and write stdout to $OUTPUT",
	"Run `ls -la /home/mpeters/Projects/agentX` and capture stdout",
}

func TestSimilarGoalsCatchesTidyCoveEchoes(t *testing.T) {
	for i := 1; i < len(tidyCoveChain); i++ {
		if !SimilarGoals(tidyCoveChain[i-1], tidyCoveChain[i]) {
			t.Errorf("chain rung %d not judged similar:\n  %q\n  %q", i, tidyCoveChain[i-1], tidyCoveChain[i])
		}
	}
}

func TestSimilarGoalsAllowsRealDecomposition(t *testing.T) {
	parent := "review the current project and identify one feature that needs refinement"
	children := []string{
		"List project root and identify key directories",
		"Read README and docs to understand intended features",
		"Scan source code for the feature needing most refinement",
	}
	for _, c := range children {
		if SimilarGoals(parent, c) {
			t.Errorf("legitimate child judged an echo of parent:\n  parent %q\n  child  %q", parent, c)
		}
	}
}

// echoPlanner returns the parent goal back as the single child on every call — the spiral
// generator; ignores contextText, so it echoes on the retry too (proving Decomposer gives
// up after exactly one retry rather than looping).
type echoPlanner struct{ calls int }

func (p *echoPlanner) Plan(_ context.Context, parentID, goal, _ string) (planner.Plan, error) {
	p.calls++
	return planner.Plan{Records: []task.Record{
		{ID: parentID + "-1", Goal: goal + " and save output to $OUTPUT", Type: task.Query,
			Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
	}}, nil
}

// TestDecomposerRefusesEcho: a plan whose child echoes the parent goal, on both the first
// attempt and the retry, returns scheduler.ErrNoProgress so the scheduler executes instead
// of recursing — after exactly 2 planner calls (1 attempt + 1 retry), never more.
func TestDecomposerRefusesEcho(t *testing.T) {
	p := &echoPlanner{}
	d := Decomposer{Planner: p, SessionID: "s", MaxDepth: 10,
		Facts: func() []session.Fact { return nil }}
	_, err := d.Decompose(context.Background(), task.Record{
		ID: "task-1", Kind: task.KindStep, Goal: "Run `ls -la` on /home/mpeters/Projects/agentX and capture its output"})
	if !errors.Is(err, scheduler.ErrNoProgress) {
		t.Fatalf("err = %v, want ErrNoProgress", err)
	}
	if p.calls != 2 {
		t.Errorf("planner calls = %d, want 2 (1 attempt + 1 retry, then give up)", p.calls)
	}
}

// TestDecomposerRetriesThenSucceeds: a planner that echoes on the first attempt but
// produces a valid, distinct plan on the retry succeeds — proving the retry path itself
// (not just the give-up path) works, and that the retry's context named the violation.
func TestDecomposerRetriesThenSucceeds(t *testing.T) {
	var sawRetryContext string
	calls := 0
	rp := decomposerFn(func(_ context.Context, parentID, goal, contextText string) (planner.Plan, error) {
		calls++
		if calls == 1 {
			return planner.Plan{Records: []task.Record{
				{ID: parentID + "-1", Goal: goal, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
			}}, nil
		}
		sawRetryContext = contextText
		return planner.Plan{Records: []task.Record{
			{ID: parentID + "-1", Goal: "list the project directory", Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
		}}, nil
	})
	d := Decomposer{Planner: rp, SessionID: "s", MaxDepth: 10, Facts: func() []session.Fact { return nil }}
	res, err := d.Decompose(context.Background(), task.Record{
		ID: "task-2", Kind: task.KindStep, Goal: "review the project"})
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if calls != 2 {
		t.Fatalf("planner calls = %d, want 2", calls)
	}
	if len(res.Records) != 1 || res.Records[0].Goal != "list the project directory" {
		t.Errorf("result = %+v, want the retry's distinct plan", res.Records)
	}
	if !strings.Contains(sawRetryContext, "invalid") {
		t.Errorf("retry context did not name the violation: %q", sawRetryContext)
	}
}

// decomposerFn adapts a func to the Planner interface for a one-off test case.
type decomposerFn func(ctx context.Context, parentID, goal, contextText string) (planner.Plan, error)

func (f decomposerFn) Plan(ctx context.Context, parentID, goal, contextText string) (planner.Plan, error) {
	return f(ctx, parentID, goal, contextText)
}
