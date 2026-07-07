package decompose

import (
	"context"
	"errors"
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

// echoPlanner returns the parent goal back as the single step — the spiral generator.
type echoPlanner struct{}

func (echoPlanner) Plan(_ context.Context, parentID, goal, _ string) (planner.Plan, error) {
	return planner.Plan{Records: []task.Record{
		{ID: parentID + "-1", Goal: goal + " and save output to $OUTPUT", Type: task.Query,
			Status: task.Proposed, Deps: []string{}},
	}}, nil
}

// TestDecomposerRefusesEcho: a plan whose child echoes the parent goal returns
// scheduler.ErrNoProgress so the scheduler executes instead of recursing.
func TestDecomposerRefusesEcho(t *testing.T) {
	d := Decomposer{Planner: echoPlanner{}, SessionID: "s", MaxDepth: 10,
		Facts: func() []session.Fact { return nil }}
	_, err := d.Decompose(context.Background(), task.Record{
		ID: "task-1", Goal: "Run `ls -la` on /home/mpeters/Projects/agentX and capture its output"})
	if !errors.Is(err, scheduler.ErrNoProgress) {
		t.Fatalf("err = %v, want ErrNoProgress", err)
	}
}
