package runtimesteps

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/cucumber/godog"

	"agentx/internal/classify"
	"agentx/internal/executor"
	"agentx/internal/llm/fanout"
	"agentx/internal/prompting"
	"agentx/internal/prompting/cascade"
	"agentx/internal/prompting/corpus"
	"agentx/internal/prompting/pipeline"
	"agentx/internal/prompting/task"
	"agentx/internal/runtime"
	"agentx/internal/session"
	"agentx/internal/state"
	"agentx/internal/tools"
)

// stubTaskExec is an injected executor that reports a fixed outcome, so the
// orchestrator's reconcile->execute->task_result wiring is exercised without a
// real tool run.
type stubTaskExec struct{ status executor.Status }

func (s stubTaskExec) Execute(context.Context, task.Record) executor.Outcome {
	return executor.Outcome{Status: s.status, Result: tools.Result{ToolID: "write_file"}}
}

type taskClassifierWorld struct {
	orc        *runtime.Orchestrator
	dir        string
	execStatus executor.Status
	respAction string
}

func registerTaskClassifierSteps(sc *godog.ScenarioContext) {
	w := &taskClassifierWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		*w = taskClassifierWorld{}
		return ctx, nil
	})

	sc.Step(`^the task executor reports "([^"]*)"$`, w.executorReports)
	sc.Step(`^the model's response shows an action$`, w.responseShowsAction)
	sc.Step(`^a started orchestrator whose classifier calls the turn "([^"]*)"$`, w.startWithClassifier)
	sc.Step(`^the classifier turn "([^"]*)" is submitted$`, w.submit)
	sc.Step(`^the session timeline contains a task_proposed event$`, w.hasTaskEvent)
	sc.Step(`^the session timeline contains no task_proposed event$`, w.noTaskEvent)
	sc.Step(`^the task_proposed event records type "([^"]*)"$`, w.taskEventType)
	sc.Step(`^the session timeline contains a task_result event$`, w.hasResultEvent)
	sc.Step(`^the session timeline contains no task_result event$`, w.noResultEvent)
	sc.Step(`^the task_result event records status "([^"]*)"$`, w.resultEventStatus)
	sc.Step(`^the task_result event records route "([^"]*)"$`, w.resultEventRoute)
	sc.Step(`^the session timeline contains a task_diagnostic event$`, w.hasDiagEvent)
	sc.Step(`^the task_diagnostic event records the triage, action, and response scores$`, w.diagHasScores)
	sc.Step(`^the task_diagnostic event outcome is "([^"]*)"$`, w.diagOutcome)
	sc.Step(`^the task_diagnostic event reason is not empty$`, w.diagReasonNotEmpty)
}

func (w *taskClassifierWorld) executorReports(status string) error {
	w.execStatus = executor.Status(status)
	return nil
}

func (w *taskClassifierWorld) responseShowsAction() error {
	w.respAction = "produced"
	return nil
}

// fixedInvoker answers every triage probe "continuation" and every action probe
// with taskType, both at high confidence — so the pipeline runs its full chain
// without a live model.
type fixedInvoker struct{ taskType, responseAction string }

func (f fixedInvoker) Invoke(_ context.Context, inv fanout.Invocation) (fanout.Response, error) {
	verdict := "continuation"
	switch inv.VerdictField {
	case "task_type":
		verdict = f.taskType
	case "response_action":
		verdict = f.responseAction
		if verdict == "" {
			verdict = "none"
		}
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
	runner := cascade.NewRunner(fixedInvoker{taskType: taskType, responseAction: w.respAction})
	p := pipeline.New(runner, c, 10)

	classifierChat := func(context.Context, []prompting.Message) (string, error) {
		return `{"route": "respond_directly"}`, nil
	}
	opts := []runtime.Option{
		runtime.WithModel(stubModel{deltas: []string{"ok"}}),
		runtime.WithClassifier(classify.New("", 0, classifierChat)),
		runtime.WithTaskClassifier(p),
	}
	if w.execStatus != "" {
		opts = append(opts, runtime.WithTaskExecutor(stubTaskExec{status: w.execStatus}))
	}
	w.orc = runtime.New(runtime.Settings{SessionRoot: dir, OllamaModel: "stub"}, opts...)
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

func (w *taskClassifierWorld) resultEvents() ([]state.Event, error) {
	events, err := w.timeline()
	if err != nil {
		return nil, err
	}
	var out []state.Event
	for _, ev := range events {
		if ev.ContentType == state.ContentTaskResult {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (w *taskClassifierWorld) hasResultEvent() error {
	evs, err := w.resultEvents()
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		return fmt.Errorf("expected a task_result event, found none")
	}
	return nil
}

func (w *taskClassifierWorld) noResultEvent() error {
	evs, err := w.resultEvents()
	if err != nil {
		return err
	}
	if len(evs) != 0 {
		return fmt.Errorf("expected no task_result event, found %d", len(evs))
	}
	return nil
}

func (w *taskClassifierWorld) resultEventStatus(want string) error {
	evs, err := w.resultEvents()
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		return fmt.Errorf("no task_result event to inspect")
	}
	payload, ok := evs[0].Payload.(map[string]any)
	if !ok {
		return fmt.Errorf("task_result payload is %T, want map", evs[0].Payload)
	}
	if got, _ := payload["status"].(string); got != want {
		return fmt.Errorf("task_result status = %q, want %q", got, want)
	}
	return nil
}

func (w *taskClassifierWorld) resultEventRoute(want string) error {
	evs, err := w.resultEvents()
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		return fmt.Errorf("no task_result event to inspect")
	}
	payload, ok := evs[0].Payload.(map[string]any)
	if !ok {
		return fmt.Errorf("task_result payload is %T, want map", evs[0].Payload)
	}
	if got, _ := payload["route"].(string); got != want {
		return fmt.Errorf("task_result route = %q, want %q", got, want)
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

func (w *taskClassifierWorld) diagEvents() ([]state.Event, error) {
	events, err := w.timeline()
	if err != nil {
		return nil, err
	}
	var out []state.Event
	for _, ev := range events {
		if ev.ContentType == state.ContentTaskDiagnostic {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (w *taskClassifierWorld) hasDiagEvent() error {
	evs, err := w.diagEvents()
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		return fmt.Errorf("expected a task_diagnostic event, found none")
	}
	return nil
}

func (w *taskClassifierWorld) diagPayload() (map[string]any, error) {
	evs, err := w.diagEvents()
	if err != nil {
		return nil, err
	}
	if len(evs) == 0 {
		return nil, fmt.Errorf("no task_diagnostic event to inspect")
	}
	payload, ok := evs[len(evs)-1].Payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("task_diagnostic payload is %T, want map", evs[len(evs)-1].Payload)
	}
	return payload, nil
}

func (w *taskClassifierWorld) diagHasScores() error {
	payload, err := w.diagPayload()
	if err != nil {
		return err
	}
	for _, stage := range []string{"triage", "action", "response"} {
		if _, ok := payload[stage].(map[string]any); !ok {
			return fmt.Errorf("task_diagnostic missing %q stage score", stage)
		}
	}
	return nil
}

func (w *taskClassifierWorld) diagOutcome(want string) error {
	payload, err := w.diagPayload()
	if err != nil {
		return err
	}
	if got, _ := payload["outcome"].(string); got != want {
		return fmt.Errorf("task_diagnostic outcome = %q, want %q", got, want)
	}
	return nil
}

func (w *taskClassifierWorld) diagReasonNotEmpty() error {
	payload, err := w.diagPayload()
	if err != nil {
		return err
	}
	if got, _ := payload["reason"].(string); got == "" {
		return fmt.Errorf("task_diagnostic reason is empty")
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

[fangroup.response_classify]
stage          = "classify_response"
purpose        = "test"
width          = 2
coarse_variant = "direct"
vote_on        = "response_action"
quorum         = 2
abstain_below  = 0.6

  [fangroup.response_classify.output_contract]
  require              = ["response_action", "confidence"]
  enum.response_action = ["none", "produced", "executed"]

  [[fangroup.response_classify.variant]]
  id          = "direct"
  axis        = "template"
  temperature = 0.2
  template     = "What did {{response}} do? Reply JSON {response_action, confidence}."

  [[fangroup.response_classify.variant]]
  id          = "skeptical"
  axis        = "context_reframe"
  temperature = 0.5
  template     = "Skeptically classify {{response}}. Reply JSON {response_action, confidence}."
`
