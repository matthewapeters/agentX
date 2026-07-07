package runtimesteps

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/prompting/planner"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
	"agentx/internal/runtime/decompose"
	"agentx/internal/runtime/scheduler"
	"agentx/internal/session"
)

// --- stubs ---------------------------------------------------------------------

// stubPlanner. mode "" returns records/synthesis (the happy-path fixture). Mode
// "echoOnce" echoes the parent goal back on the first call only, then returns a distinct
// valid plan — exercising the retry-then-succeed path. Mode "alwaysEcho" echoes on every
// call, exhausting the retry (scheduler.ErrNoProgress).
type stubPlanner struct {
	records   []task.Record
	synthesis string
	sawCtx    string
	mode      string
	calls     int
}

func (s *stubPlanner) Plan(_ context.Context, parentID, goal, contextText string) (planner.Plan, error) {
	s.calls++
	s.sawCtx = contextText
	echo := func() planner.Plan {
		return planner.Plan{Records: []task.Record{
			{ID: parentID + "-1", Goal: goal, Type: task.Query, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
		}}
	}
	switch s.mode {
	case "echoOnce":
		if s.calls == 1 {
			return echo(), nil
		}
		return planner.Plan{Records: []task.Record{
			{ID: parentID + "-1", Goal: "list the project directory", Type: task.Query, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
		}}, nil
	case "alwaysEcho":
		return echo(), nil
	default:
		return planner.Plan{Records: s.records, Synthesis: s.synthesis}, nil
	}
}

// --- world ---------------------------------------------------------------------

type decAdaptWorld struct {
	planner *stubPlanner
	facts   []session.Fact
	result  branch.Result
	decErr  error
}

func registerDecomposeAdapterSteps(sc *godog.ScenarioContext) {
	w := &decAdaptWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = decAdaptWorld{planner: &stubPlanner{}}
		return ctx, nil
	})

	sc.Step(`^a stub planner returning sub-goals "([^"]*)" and "([^"]*)" with synthesis "([^"]*)"$`, w.stubPlan)
	sc.Step(`^a stub planner that echoes the parent goal on the first attempt only$`, w.stubEchoOnce)
	sc.Step(`^a stub planner that always echoes the parent goal$`, w.stubAlwaysEcho)
	sc.Step(`^the parent has an enabled fact "([^"]*)" = "([^"]*)"$`, w.parentFact)
	sc.Step(`^the goal "([^"]*)" is decomposed$`, w.decompose)
	sc.Step(`^the result carries (\d+) child records$`, w.resultChildCount)
	sc.Step(`^the decomposition synthesis is "([^"]*)"$`, w.resultSynthesisIs)
	sc.Step(`^the planner saw the parent fact "([^"]*)" in its context$`, w.plannerSawFact)
	sc.Step(`^the decomposition succeeds$`, w.decompositionSucceeds)
	sc.Step(`^the retry context named the violation$`, w.retryContextNamedViolation)
	sc.Step(`^the decomposition reports no progress$`, w.decompositionNoProgress)
}

// --- decomposer steps ----------------------------------------------------------

func (w *decAdaptWorld) stubPlan(a, b, synth string) error {
	w.planner.records = []task.Record{
		{ID: "task-1-1", Goal: a, Type: task.Query, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
		{ID: "task-1-2", Goal: b, Type: task.Query, Kind: task.KindTask, Status: task.Proposed, Deps: []string{}},
	}
	w.planner.synthesis = synth
	return nil
}

func (w *decAdaptWorld) stubEchoOnce() error  { w.planner.mode = "echoOnce"; return nil }
func (w *decAdaptWorld) stubAlwaysEcho() error { w.planner.mode = "alwaysEcho"; return nil }

func (w *decAdaptWorld) parentFact(key, value string) error {
	w.facts = append(w.facts, session.Fact{Key: key, Value: value, Owner: session.OwnerUser, Enabled: true})
	return nil
}

func (w *decAdaptWorld) decompose(goal string) error {
	d := decompose.Decomposer{
		Planner:   w.planner,
		SessionID: "s1",
		MaxDepth:  10,
		Facts:     func() []session.Fact { return w.facts },
	}
	w.result, w.decErr = d.Decompose(context.Background(), task.Record{ID: "task-1", Kind: task.KindStep, Goal: goal})
	return nil
}

func (w *decAdaptWorld) resultChildCount(n int) error {
	if got := len(w.result.Records); got != n {
		return fmt.Errorf("result child records = %d, want %d", got, n)
	}
	return nil
}

func (w *decAdaptWorld) resultSynthesisIs(want string) error {
	if w.result.Synthesis != want {
		return fmt.Errorf("result synthesis = %q, want %q", w.result.Synthesis, want)
	}
	return nil
}

func (w *decAdaptWorld) plannerSawFact(key string) error {
	if !strings.Contains(w.planner.sawCtx, key) {
		return fmt.Errorf("planner context %q did not include fact %q", w.planner.sawCtx, key)
	}
	return nil
}

func (w *decAdaptWorld) decompositionSucceeds() error {
	if w.decErr != nil {
		return fmt.Errorf("decompose failed: %v", w.decErr)
	}
	return nil
}

func (w *decAdaptWorld) retryContextNamedViolation() error {
	if !strings.Contains(w.planner.sawCtx, "invalid") {
		return fmt.Errorf("retry context %q did not name the violation", w.planner.sawCtx)
	}
	return nil
}

func (w *decAdaptWorld) decompositionNoProgress() error {
	if !errors.Is(w.decErr, scheduler.ErrNoProgress) {
		return fmt.Errorf("err = %v, want ErrNoProgress", w.decErr)
	}
	return nil
}
