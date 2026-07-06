package promptingsteps

import (
	"context"
	"fmt"
	"math"

	"github.com/cucumber/godog"

	"agentx/internal/llm/fanout"
	"agentx/internal/prompting/task"
)

type taskWorld struct {
	action    fanout.Decision
	escalated bool
	record    task.Record
	emitted   bool
}

func registerTaskSteps(sc *godog.ScenarioContext) {
	w := &taskWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = taskWorld{}
		return ctx, nil
	})

	sc.Step(`^an action decision "([^"]*)" at confidence ([0-9.]+)$`, w.actionDecision)
	sc.Step(`^an abstained action decision$`, w.abstainedDecision)
	sc.Step(`^the decision was escalated$`, w.wasEscalated)
	sc.Step(`^a task is derived with id "([^"]*)" goal "([^"]*)"$`, w.deriveTask)
	sc.Step(`^a task is emitted$`, w.taskEmitted)
	sc.Step(`^no task is emitted$`, w.noTaskEmitted)
	sc.Step(`^the task type is "([^"]*)"$`, w.taskType)
	sc.Step(`^the task status is "([^"]*)"$`, w.taskStatus)
	sc.Step(`^the task deps are empty$`, w.taskDepsEmpty)
	sc.Step(`^the task provenance confidence is ([0-9.]+)$`, w.provConfidence)
	sc.Step(`^the task provenance is escalated$`, w.provEscalated)
}

func (w *taskWorld) actionDecision(verdict string, conf float64) error {
	w.action = fanout.Decision{Verdict: verdict, Confidence: conf}
	return nil
}

func (w *taskWorld) abstainedDecision() error {
	w.action = fanout.Decision{Abstained: true, Reason: "scattered"}
	return nil
}

func (w *taskWorld) wasEscalated() error {
	w.escalated = true
	return nil
}

func (w *taskWorld) deriveTask(id, goal string) error {
	w.record, w.emitted = task.FromAction(id, goal, w.action, w.escalated)
	return nil
}

func (w *taskWorld) taskEmitted() error {
	if !w.emitted {
		return fmt.Errorf("expected a task to be emitted")
	}
	return nil
}

func (w *taskWorld) noTaskEmitted() error {
	if w.emitted {
		return fmt.Errorf("expected no task, got %+v", w.record)
	}
	return nil
}

func (w *taskWorld) taskType(want string) error {
	if got := string(w.record.Type); got != want {
		return fmt.Errorf("task type = %q, want %q", got, want)
	}
	return nil
}

func (w *taskWorld) taskStatus(want string) error {
	if got := string(w.record.Status); got != want {
		return fmt.Errorf("task status = %q, want %q", got, want)
	}
	return nil
}

func (w *taskWorld) taskDepsEmpty() error {
	if len(w.record.Deps) != 0 {
		return fmt.Errorf("task deps = %v, want empty", w.record.Deps)
	}
	return nil
}

func (w *taskWorld) provConfidence(want float64) error {
	if got := w.record.Provenance.Confidence; math.Abs(got-want) > 1e-9 {
		return fmt.Errorf("provenance confidence = %v, want %v", got, want)
	}
	return nil
}

func (w *taskWorld) provEscalated() error {
	if !w.record.Provenance.Escalated {
		return fmt.Errorf("expected provenance escalated")
	}
	return nil
}
