package promptingsteps

import (
	"context"
	"fmt"
	"strconv"

	"github.com/cucumber/godog"

	"agentx/internal/llm/fanout"
	"agentx/internal/prompting/cascade"
	"agentx/internal/prompting/corpus"
	"agentx/internal/prompting/pipeline"
	"agentx/internal/state"
)

// pipeInvoker scripts responses per classify stage, keyed by the invocation's
// VerdictField ("relation" for triage, "task_type" for action). A per-variant
// override (keyed "field/variantID") lets a stage's vote scatter, exercising the
// abstain path.
type pipeInvoker struct {
	byField   map[string]scriptResp
	byVariant map[string]scriptResp
}

func (p pipeInvoker) Invoke(_ context.Context, inv fanout.Invocation) (fanout.Response, error) {
	if r, ok := p.byVariant[inv.VerdictField+"/"+variantID(inv.Tag)]; ok {
		return respond(inv, r), nil
	}
	if r, ok := p.byField[inv.VerdictField]; ok {
		return respond(inv, r), nil
	}
	return fanout.Response{}, fmt.Errorf("unscripted field %q (tag %q)", inv.VerdictField, inv.Tag)
}

func respond(inv fanout.Invocation, r scriptResp) fanout.Response {
	fields := map[string]string{"confidence": strconv.FormatFloat(r.conf, 'g', -1, 64)}
	if inv.VerdictField != "" {
		fields[inv.VerdictField] = r.verdict
	}
	return fanout.Response{Verdict: r.verdict, Confidence: r.conf, Fields: fields}
}

type pipelineWorld struct {
	inv     pipeInvoker
	corpus  *corpus.Corpus
	events  []state.Event
	next    uint64
	result  pipeline.Result
	respDec fanout.Decision
	respOK  bool
	err     error
}

func registerPipelineSteps(sc *godog.ScenarioContext) {
	w := &pipelineWorld{}

	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = pipelineWorld{
			inv: pipeInvoker{byField: map[string]scriptResp{}, byVariant: map[string]scriptResp{}},
		}
		return ctx, nil
	})

	sc.Step(`^the classify pipeline corpus is loaded$`, w.corpusLoaded)
	sc.Step(`^the session has a prior turn "([^"]*)"$`, w.priorTurn)
	sc.Step(`^triage returns "([^"]*)" at confidence ([0-9.]+)$`, w.triageReturns)
	sc.Step(`^action returns "([^"]*)" at confidence ([0-9.]+)$`, w.actionReturns)
	sc.Step(`^response classify returns "([^"]*)" at confidence ([0-9.]+)$`, w.responseReturns)
	sc.Step(`^triage variant "([^"]*)" returns "([^"]*)" at confidence ([0-9.]+)$`, w.triageVariant)
	sc.Step(`^the pipeline classifies "([^"]*)"$`, w.classify)
	sc.Step(`^the pipeline classifies the response "([^"]*)"$`, w.classifyResponse)
	sc.Step(`^the response classifier verdict is "([^"]*)"$`, w.responseVerdict)
	sc.Step(`^the triage relation is "([^"]*)"$`, w.triageRelation)
	sc.Step(`^the action task type is "([^"]*)"$`, w.actionTaskType)
	sc.Step(`^the directive relation is "([^"]*)"$`, w.directiveRelation)
	sc.Step(`^the directive context is empty$`, w.directiveContextEmpty)
	sc.Step(`^the directive context is not empty$`, w.directiveContextNotEmpty)
	sc.Step(`^the directive is cautious$`, w.directiveCautious)
	sc.Step(`^the directive abstained$`, w.directiveAbstained)
}

// corpus is stashed on the world so classify can build the pipeline with it.
func (w *pipelineWorld) corpusLoaded() error {
	c, err := corpus.Parse([]byte(pipelineCorpusTOML))
	if err != nil {
		return fmt.Errorf("parse pipeline corpus: %w", err)
	}
	w.corpus = c
	return nil
}

func (w *pipelineWorld) priorTurn(text string) error {
	w.next++
	w.events = append(w.events, state.Event{
		ContentType: state.ContentUserPrompt,
		Payload:     map[string]any{"text": text},
		Enabled:     true,
		Ordinal:     w.next,
	})
	return nil
}

func (w *pipelineWorld) triageReturns(verdict string, conf float64) error {
	w.inv.byField["relation"] = scriptResp{verdict: verdict, conf: conf}
	return nil
}

func (w *pipelineWorld) actionReturns(verdict string, conf float64) error {
	w.inv.byField["task_type"] = scriptResp{verdict: verdict, conf: conf}
	return nil
}

func (w *pipelineWorld) responseReturns(verdict string, conf float64) error {
	w.inv.byField["response_action"] = scriptResp{verdict: verdict, conf: conf}
	return nil
}

func (w *pipelineWorld) classifyResponse(response string) error {
	runner := cascade.NewRunner(w.inv)
	p := pipeline.New(runner, w.corpus, 10)
	w.respDec, w.respOK = p.ClassifyResponse(context.Background(), response)
	return nil
}

func (w *pipelineWorld) responseVerdict(want string) error {
	if !w.respOK {
		return fmt.Errorf("response classification did not run (no response_classify group?)")
	}
	if got := w.respDec.Verdict; got != want {
		return fmt.Errorf("response verdict = %q, want %q", got, want)
	}
	return nil
}

func (w *pipelineWorld) triageVariant(id, verdict string, conf float64) error {
	w.inv.byVariant["relation/"+id] = scriptResp{verdict: verdict, conf: conf}
	return nil
}

func (w *pipelineWorld) classify(turn string) error {
	runner := cascade.NewRunner(w.inv)
	p := pipeline.New(runner, w.corpus, 10)
	w.result, w.err = p.Classify(context.Background(), w.events, turn)
	return w.err
}

func (w *pipelineWorld) triageRelation(want string) error {
	if got := w.result.Triage.Verdict; got != want {
		return fmt.Errorf("triage relation = %q, want %q", got, want)
	}
	return nil
}

func (w *pipelineWorld) actionTaskType(want string) error {
	if got := w.result.Action.Verdict; got != want {
		return fmt.Errorf("action task_type = %q, want %q", got, want)
	}
	return nil
}

func (w *pipelineWorld) directiveRelation(want string) error {
	if got := w.result.Directive.Relation; got != want {
		return fmt.Errorf("directive relation = %q, want %q", got, want)
	}
	return nil
}

func (w *pipelineWorld) directiveContextEmpty() error {
	if got := w.result.Directive.Context; got != "" {
		return fmt.Errorf("directive context should be empty, got %q", got)
	}
	return nil
}

func (w *pipelineWorld) directiveContextNotEmpty() error {
	if w.result.Directive.Context == "" {
		return fmt.Errorf("directive context should not be empty")
	}
	return nil
}

func (w *pipelineWorld) directiveCautious() error {
	if !w.result.Directive.Cautious {
		return fmt.Errorf("directive should be cautious")
	}
	return nil
}

func (w *pipelineWorld) directiveAbstained() error {
	if !w.result.Directive.Abstained {
		return fmt.Errorf("directive should have abstained")
	}
	return nil
}

const pipelineCorpusTOML = `
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
