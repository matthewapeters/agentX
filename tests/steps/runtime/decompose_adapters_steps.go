package runtimesteps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/llm/fanout"
	"agentx/internal/prompting/planner"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime/branch"
	"agentx/internal/runtime/decompose"
	"agentx/internal/session"
)

// --- stubs ---------------------------------------------------------------------

type stubActionClassifier struct {
	decision fanout.Decision
}

func (s stubActionClassifier) ClassifyAction(context.Context, string) (fanout.Decision, error) {
	return s.decision, nil
}

type stubOneStep struct{ oneStep bool }

func (s stubOneStep) OneStep(context.Context, string) (bool, error) { return s.oneStep, nil }

type stubPlanner struct {
	records   []task.Record
	synthesis string
	sawCtx    string
}

func (s *stubPlanner) Plan(_ context.Context, _, _, contextText string) (planner.Plan, error) {
	s.sawCtx = contextText
	return planner.Plan{Records: s.records, Synthesis: s.synthesis}, nil
}

// --- world ---------------------------------------------------------------------

type decAdaptWorld struct {
	action    fanout.Decision
	oneStep   bool
	hasOne    bool
	atomic    bool
	planner   *stubPlanner
	facts     []session.Fact
	result    branch.Result
	decErr    error
}

func registerDecomposeAdapterSteps(sc *godog.ScenarioContext) {
	w := &decAdaptWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = decAdaptWorld{planner: &stubPlanner{}}
		return ctx, nil
	})

	sc.Step(`^the action classifier resolves the goal to "([^"]*)"$`, w.actionResolves)
	sc.Step(`^the action classifier abstains on the goal$`, w.actionAbstains)
	sc.Step(`^the one-step check reports one-step$`, w.oneStepYes)
	sc.Step(`^the one-step check reports not one-step$`, w.oneStepNo)
	sc.Step(`^the oracle evaluates the goal$`, w.oracleEval)
	sc.Step(`^the goal is judged atomic$`, w.judgedAtomic)
	sc.Step(`^the goal is judged not atomic$`, w.judgedNotAtomic)

	sc.Step(`^a stub planner returning sub-goals "([^"]*)" and "([^"]*)" with synthesis "([^"]*)"$`, w.stubPlan)
	sc.Step(`^the parent has an enabled fact "([^"]*)" = "([^"]*)"$`, w.parentFact)
	sc.Step(`^the goal "([^"]*)" is decomposed$`, w.decompose)
	sc.Step(`^the result carries (\d+) child records$`, w.resultChildCount)
	sc.Step(`^the decomposition synthesis is "([^"]*)"$`, w.resultSynthesisIs)
	sc.Step(`^the planner saw the parent fact "([^"]*)" in its context$`, w.plannerSawFact)
}

// --- oracle steps --------------------------------------------------------------

func (w *decAdaptWorld) actionResolves(verdict string) error {
	w.action = fanout.Decision{Verdict: verdict, Confidence: 0.9}
	return nil
}

func (w *decAdaptWorld) actionAbstains() error {
	w.action = fanout.Decision{Abstained: true}
	return nil
}

func (w *decAdaptWorld) oneStepYes() error { w.oneStep, w.hasOne = true, true; return nil }
func (w *decAdaptWorld) oneStepNo() error  { w.oneStep, w.hasOne = false, true; return nil }

func (w *decAdaptWorld) oracleEval() error {
	o := decompose.Oracle{Action: stubActionClassifier{decision: w.action}}
	if w.hasOne {
		o.OneStep = stubOneStep{oneStep: w.oneStep}
	}
	got, err := o.Atomic(context.Background(), task.Record{ID: "n", Goal: "g"})
	if err != nil {
		return err
	}
	w.atomic = got
	return nil
}

func (w *decAdaptWorld) judgedAtomic() error {
	if !w.atomic {
		return fmt.Errorf("goal judged not atomic, want atomic")
	}
	return nil
}

func (w *decAdaptWorld) judgedNotAtomic() error {
	if w.atomic {
		return fmt.Errorf("goal judged atomic, want not atomic")
	}
	return nil
}

// --- decomposer steps ----------------------------------------------------------

func (w *decAdaptWorld) stubPlan(a, b, synth string) error {
	w.planner.records = []task.Record{
		{ID: "task-1-1", Goal: a, Type: task.Query, Status: task.Proposed, Deps: []string{}},
		{ID: "task-1-2", Goal: b, Type: task.Query, Status: task.Proposed, Deps: []string{}},
	}
	w.planner.synthesis = synth
	return nil
}

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
	w.result, w.decErr = d.Decompose(context.Background(), task.Record{ID: "task-1", Goal: goal})
	return w.decErr
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
