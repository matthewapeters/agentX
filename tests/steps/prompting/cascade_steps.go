package promptingsteps

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/llm/fanout"
	"agentx/internal/prompting/cascade"
	"agentx/internal/prompting/corpus"
)

type scriptResp struct {
	verdict string
	conf    float64
}

// scriptedInvoker returns a canned verdict+confidence per variant id (extracted
// from the invocation tag), building a Response that conforms to the group's
// contract. It exercises the runner's cascade logic without a live model.
type scriptedInvoker struct {
	responses map[string]scriptResp
}

func (s scriptedInvoker) Invoke(_ context.Context, inv fanout.Invocation) (fanout.Response, error) {
	id := variantID(inv.Tag)
	r, ok := s.responses[id]
	if !ok {
		return fanout.Response{}, fmt.Errorf("unscripted variant %q (tag %q)", id, inv.Tag)
	}
	fields := map[string]string{"confidence": strconv.FormatFloat(r.conf, 'g', -1, 64)}
	if inv.VerdictField != "" {
		fields[inv.VerdictField] = r.verdict
	}
	return fanout.Response{Verdict: r.verdict, Confidence: r.conf, Fields: fields}, nil
}

func variantID(tag string) string {
	id := tag
	if slash := strings.LastIndexByte(id, '/'); slash >= 0 {
		id = id[slash+1:]
	}
	if hash := strings.IndexByte(id, '#'); hash >= 0 {
		id = id[:hash]
	}
	return id
}

type cascadeWorld struct {
	group     *corpus.FanGroup
	responses map[string]scriptResp
	result    cascade.Result
	err       error
}

func registerCascadeSteps(sc *godog.ScenarioContext) {
	w := &cascadeWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		*w = cascadeWorld{}
		return ctx, nil
	})

	sc.Step(`^a "triage" cascade group$`, w.triageGroup)
	sc.Step(`^a high-stakes cascade group escalating "([^"]*)"$`, w.stakesGroup)
	sc.Step(`^the coarse gate returns "([^"]*)" at confidence ([\d.]+)$`, w.coarseReturns)
	sc.Step(`^the vote agrees on "([^"]*)"$`, w.voteAgrees)
	sc.Step(`^the vote scatters across "([^"]*)", "([^"]*)", "([^"]*)"$`, w.voteScatters)
	sc.Step(`^the cascade runs$`, w.cascadeRuns)
	sc.Step(`^the cascade does not escalate$`, w.notEscalated)
	sc.Step(`^the cascade escalates$`, w.escalated)
	sc.Step(`^the cascade verdict is "([^"]*)"$`, w.verdict)
	sc.Step(`^the cascade abstains$`, w.abstains)
}

func cascadeGroupTOML(name, voteField, alwaysEscalate string) string {
	return fmt.Sprintf(`
[fangroup.%s]
stage = "s"
purpose = "p"
width = 4
coarse_variant = "direct"
vote_on = %q
quorum = 3
abstain_below = 0.6
%s
  [fangroup.%s.output_contract]
  require = [%q, "confidence"]
  max_words = 40
  [[fangroup.%s.variant]]
  id = "direct"
  temperature = 0.2
  template = "{{turn}} reply json"
  [[fangroup.%s.variant]]
  id = "v1"
  temperature = 0.4
  template = "{{turn}} reply json"
  [[fangroup.%s.variant]]
  id = "v2"
  temperature = 0.6
  template = "{{turn}} reply json"
  [[fangroup.%s.variant]]
  id = "v3"
  temperature = 0.8
  template = "{{turn}} reply json"
`, name, voteField, alwaysEscalate, name, voteField, name, name, name, name)
}

func (w *cascadeWorld) loadGroup(src, name string) error {
	c, err := corpus.Parse([]byte(src))
	if err != nil {
		return err
	}
	g, ok := c.Group(name)
	if !ok {
		return fmt.Errorf("no group %q", name)
	}
	w.group = g
	w.responses = map[string]scriptResp{}
	return nil
}

func (w *cascadeWorld) triageGroup() error {
	return w.loadGroup(cascadeGroupTOML("triage", "relation", ""), "triage")
}

func (w *cascadeWorld) stakesGroup(escalate string) error {
	line := fmt.Sprintf("always_escalate_types = [%q]", escalate)
	return w.loadGroup(cascadeGroupTOML("stakes", "task_type", line), "stakes")
}

func (w *cascadeWorld) coarseReturns(verdict string, conf float64) error {
	w.responses["direct"] = scriptResp{verdict: verdict, conf: conf}
	return nil
}

func (w *cascadeWorld) voteAgrees(verdict string) error {
	for _, id := range []string{"v1", "v2", "v3"} {
		w.responses[id] = scriptResp{verdict: verdict, conf: 0.9}
	}
	return nil
}

func (w *cascadeWorld) voteScatters(a, b, c string) error {
	w.responses["v1"] = scriptResp{verdict: a, conf: 0.7}
	w.responses["v2"] = scriptResp{verdict: b, conf: 0.7}
	w.responses["v3"] = scriptResp{verdict: c, conf: 0.7}
	return nil
}

func (w *cascadeWorld) cascadeRuns() error {
	runner := cascade.NewRunner(scriptedInvoker{responses: w.responses}, fanout.WithConcurrency(8))
	w.result, w.err = runner.Run(context.Background(), w.group, map[string]string{"turn": "do the thing"})
	return w.err
}

func (w *cascadeWorld) notEscalated() error {
	if w.result.Escalated {
		return fmt.Errorf("cascade escalated, want coarse acceptance")
	}
	return nil
}

func (w *cascadeWorld) escalated() error {
	if !w.result.Escalated {
		return fmt.Errorf("cascade did not escalate")
	}
	return nil
}

func (w *cascadeWorld) verdict(want string) error {
	if w.result.Decision.Abstained {
		return fmt.Errorf("cascade abstained (%s), want verdict %q", w.result.Decision.Reason, want)
	}
	if w.result.Decision.Verdict != want {
		return fmt.Errorf("cascade verdict = %q, want %q", w.result.Decision.Verdict, want)
	}
	return nil
}

func (w *cascadeWorld) abstains() error {
	if !w.result.Decision.Abstained {
		return fmt.Errorf("cascade did not abstain (verdict %q)", w.result.Decision.Verdict)
	}
	return nil
}
