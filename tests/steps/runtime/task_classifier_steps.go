package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/classify"
	"agentx/internal/llm/fanout"
	"agentx/internal/prompting"
	"agentx/internal/prompting/cascade"
	"agentx/internal/prompting/corpus"
	"agentx/internal/prompting/pipeline"
	"agentx/internal/runtime"
	"agentx/internal/session"
	"agentx/internal/state"
)

type taskClassifierWorld struct {
	orc *runtime.Orchestrator
	dir string
}

func registerTaskClassifierSteps(sc *godog.ScenarioContext) {
	w := &taskClassifierWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = taskClassifierWorld{}
		return ctx, err
	})

	sc.Step(`^a started orchestrator whose classifier calls the turn "([^"]*)"$`, w.startWithClassifier)
	sc.Step(`^the classifier turn "([^"]*)" is submitted$`, w.submit)
	sc.Step(`^the session timeline contains a task_proposed event$`, w.hasTaskEvent)
	sc.Step(`^the session timeline contains no task_proposed event$`, w.noTaskEvent)
	sc.Step(`^the task_proposed event records type "([^"]*)"$`, w.taskEventType)
}

// fixedInvoker answers every triage probe "continuation" and every action probe
// with taskType, both at high confidence — so the pipeline runs its full chain
// without a live model.
type fixedInvoker struct{ taskType string }

func (f fixedInvoker) Invoke(_ context.Context, inv fanout.Invocation) (fanout.Response, error) {
	verdict := "continuation"
	if inv.VerdictField == "task_type" {
		verdict = f.taskType
	}
	fields := map[string]string{
		"confidence":     strconv.FormatFloat(0.95, 'g', -1, 64),
		inv.VerdictField: verdict,
	}
	return fanout.Response{Verdict: verdict, Confidence: 0.95, Fields: fields}, nil
}

func (w *taskClassifierWorld) startWithClassifier(taskType string) error {
	dir, err := os.MkdirTemp("", "agentx-taskcls-")
	if err != nil {
		return err
	}
	w.dir = dir

	c, err := corpus.Parse([]byte(taskClassifierCorpusTOML))
	if err != nil {
		return fmt.Errorf("parse corpus: %w", err)
	}
	runner := cascade.NewRunner(fixedInvoker{taskType: taskType})
	p := pipeline.New(runner, c, 10)

	classifierChat := func(context.Context, []prompting.Message) (string, error) {
		return `{"route": "respond_directly"}`, nil
	}
	w.orc = runtime.New(
		runtime.Settings{SessionRoot: dir, OllamaModel: "stub"},
		runtime.WithModel(stubModel{deltas: []string{"ok"}}),
		runtime.WithClassifier(classify.New("", 0, classifierChat)),
		runtime.WithTaskClassifier(p),
	)
	return w.orc.Start()
}

func (w *taskClassifierWorld) submit(text string) error {
	if err := w.orc.Submit(context.Background(), text); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return w.orc.Shutdown(ctx)
}

func (w *taskClassifierWorld) timeline() ([]state.Event, error) {
	store := session.NewStore(w.dir)
	return store.Recorder(w.orc.Session().ID).Load()
}

func (w *taskClassifierWorld) taskEvents() ([]state.Event, error) {
	events, err := w.timeline()
	if err != nil {
		return nil, err
	}
	var out []state.Event
	for _, ev := range events {
		if ev.ContentType == state.ContentTaskProposed {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (w *taskClassifierWorld) hasTaskEvent() error {
	evs, err := w.taskEvents()
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		return fmt.Errorf("expected a task_proposed event, found none")
	}
	return nil
}

func (w *taskClassifierWorld) noTaskEvent() error {
	evs, err := w.taskEvents()
	if err != nil {
		return err
	}
	if len(evs) != 0 {
		return fmt.Errorf("expected no task_proposed event, found %d", len(evs))
	}
	return nil
}

func (w *taskClassifierWorld) taskEventType(want string) error {
	evs, err := w.taskEvents()
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		return fmt.Errorf("no task_proposed event to inspect")
	}
	payload, ok := evs[0].Payload.(map[string]any)
	if !ok {
		return fmt.Errorf("task_proposed payload is %T, want map", evs[0].Payload)
	}
	if got, _ := payload["type"].(string); got != want {
		return fmt.Errorf("task_proposed type = %q, want %q", got, want)
	}
	return nil
}

const taskClassifierCorpusTOML = `
[fangroup.relatedness_triage]
stage          = "triage"
purpose        = "test"
width          = 2
coarse_variant = "direct"
vote_on        = "relation"
quorum         = 2
abstain_below  = 0.6

  [fangroup.relatedness_triage.output_contract]
  require       = ["relation", "confidence"]
  enum.relation = ["continuation", "new", "orthogonal", "related_aside"]

  [[fangroup.relatedness_triage.variant]]
  id          = "direct"
  axis        = "template"
  temperature = 0.2
  template     = "Relate {{turn}} to {{session_digest}}. Reply JSON {relation, confidence}."

  [[fangroup.relatedness_triage.variant]]
  id          = "reframed"
  axis        = "context_reframe"
  temperature = 0.6
  template     = "Given {{session_digest}}, classify {{turn}}. Reply JSON {relation, confidence}."

[fangroup.action_classify]
stage          = "classify_turn"
purpose        = "test"
width          = 2
coarse_variant = "direct"
vote_on        = "task_type"
quorum         = 2
abstain_below  = 0.6

  [fangroup.action_classify.output_contract]
  require        = ["task_type", "confidence"]
  enum.task_type = ["artifact", "command", "query", "none"]

  [[fangroup.action_classify.variant]]
  id          = "direct"
  axis        = "template"
  temperature = 0.2
  template     = "Does {{turn}} ask for an action? Context {{context}}. Reply JSON {task_type, confidence}."

  [[fangroup.action_classify.variant]]
  id          = "reframed"
  axis        = "context_reframe"
  temperature = 0.6
  template     = "Classify {{turn}} given {{context}}. Reply JSON {task_type, confidence}."
`
