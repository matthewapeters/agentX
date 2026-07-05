// Package promptingsteps implements the Godog steps for the prompting domain:
// the fan-group corpus loader/renderer (tests/features/prompting/fan_group_corpus.feature).
package promptingsteps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"agentx/internal/llm/fanout"
	"agentx/internal/prompting/corpus"
)

type corpusWorld struct {
	src    string
	corpus *corpus.Corpus
	err    error
	vars   map[string]string
	invs   []fanout.Invocation
}

// InitializeScenario registers the prompting-domain steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	w := &corpusWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		*w = corpusWorld{}
		return ctx, err
	})

	sc.Step(`^a corpus with a "([^"]*)" fan-group of width (\d+) quorum (\d+)$`, w.corpusWithGroup)
	sc.Step(`^a corpus whose coarse variant names a nonexistent variant$`, w.corpusBadCoarse)
	sc.Step(`^a corpus with an unknown placeholder in a template$`, w.corpusBadPlaceholder)
	sc.Step(`^a render context with turn "([^"]*)"$`, w.renderContext)

	sc.Step(`^the corpus is parsed$`, w.parse)
	sc.Step(`^the fan-group "([^"]*)" is rendered$`, w.render)
	sc.Step(`^the seed corpus at "([^"]*)" is loaded$`, w.loadSeed)

	sc.Step(`^the parse succeeds$`, w.parseOK)
	sc.Step(`^the load succeeds$`, w.parseOK)
	sc.Step(`^the parse fails mentioning "([^"]*)"$`, w.parseFails)
	sc.Step(`^the corpus has a fan-group "([^"]*)"$`, w.hasGroup)
	sc.Step(`^the fan-group "([^"]*)" has (\d+) variants$`, w.groupVariants)
	sc.Step(`^the fan-group "([^"]*)" contract requires "([^"]*)" and "([^"]*)"$`, w.contractRequires)
	sc.Step(`^the fan-group "([^"]*)" contract bounds words to (\d+)$`, w.contractWords)
	sc.Step(`^the fan-group "([^"]*)" aggregator has quorum (\d+)$`, w.aggQuorum)
	sc.Step(`^(\d+) invocations are produced$`, w.invCount)
	sc.Step(`^every invocation carries the compiled contract$`, w.invContract)
	sc.Step(`^every invocation votes on "([^"]*)"$`, w.invVotesOn)
	sc.Step(`^every invocation prompt substitutes the turn$`, w.invSubstitutes)
	sc.Step(`^no invocation prompt has an unfilled placeholder$`, w.invNoPlaceholder)
	sc.Step(`^the invocations carry more than one distinct temperature$`, w.invDistinctTemps)

	registerCascadeSteps(sc)
}

// ---- corpus fixtures ----

func triageCorpus(width, quorum int, coarse, tmplExtra string) string {
	return fmt.Sprintf(`
[fangroup.triage]
stage = "triage"
purpose = "decide relatedness"
width = %d
coarse_variant = %q
vote_on = "relation"
quorum = %d
abstain_below = 0.6
  [fangroup.triage.output_contract]
  require = ["relation", "confidence"]
  max_words = 40
  enum.relation = ["continuation", "new", "orthogonal", "related_aside"]
  [[fangroup.triage.variant]]
  id = "direct"
  axis = "template"
  temperature = 0.2
  template = "Session: {{session_digest}} Message: {{turn}} %s Reply JSON."
  [[fangroup.triage.variant]]
  id = "reframed"
  axis = "context_reframe"
  temperature = 0.5
  template = "Reframe: {{turn}} Reply JSON."
  [[fangroup.triage.variant]]
  id = "minimal"
  axis = "param"
  temperature = 0.8
  template = "{{turn}} Reply JSON."
`, width, coarse, quorum, tmplExtra)
}

func (w *corpusWorld) corpusWithGroup(_ string, width, quorum int) error {
	w.src = triageCorpus(width, quorum, "direct", "")
	return nil
}

func (w *corpusWorld) corpusBadCoarse() error {
	w.src = triageCorpus(3, 2, "nonexistent", "")
	return nil
}

func (w *corpusWorld) corpusBadPlaceholder() error {
	w.src = triageCorpus(3, 2, "direct", "{{bogus}}")
	return nil
}

func (w *corpusWorld) renderContext(turn string) error {
	w.vars = map[string]string{
		"turn":           turn,
		"session_digest": "(prior work on the classifier)",
		"open_tasks":     "(none)",
		"context":        "(prior work on the classifier)",
	}
	return nil
}

// ---- actions ----

func (w *corpusWorld) parse() error {
	w.corpus, w.err = corpus.Parse([]byte(w.src))
	return nil
}

func (w *corpusWorld) render(name string) error {
	if w.corpus == nil {
		w.corpus, w.err = corpus.Parse([]byte(w.src))
		if w.err != nil {
			return w.err
		}
	}
	g, ok := w.corpus.Group(name)
	if !ok {
		return fmt.Errorf("no fan-group %q", name)
	}
	w.invs = g.Render(w.vars)
	return nil
}

func (w *corpusWorld) loadSeed(path string) error {
	w.corpus, w.err = corpus.Load(path)
	return nil
}

// ---- assertions ----

func (w *corpusWorld) group(name string) (*corpus.FanGroup, error) {
	if w.corpus == nil {
		return nil, fmt.Errorf("no corpus parsed (err: %v)", w.err)
	}
	g, ok := w.corpus.Group(name)
	if !ok {
		return nil, fmt.Errorf("no fan-group %q", name)
	}
	return g, nil
}

func (w *corpusWorld) parseOK() error {
	if w.err != nil {
		return fmt.Errorf("parse/load failed: %v", w.err)
	}
	return nil
}

func (w *corpusWorld) parseFails(want string) error {
	if w.err == nil {
		return fmt.Errorf("expected parse to fail mentioning %q, but it succeeded", want)
	}
	if !strings.Contains(w.err.Error(), want) {
		return fmt.Errorf("error %q does not mention %q", w.err.Error(), want)
	}
	return nil
}

func (w *corpusWorld) hasGroup(name string) error {
	_, err := w.group(name)
	return err
}

func (w *corpusWorld) groupVariants(name string, n int) error {
	g, err := w.group(name)
	if err != nil {
		return err
	}
	if len(g.Variants) != n {
		return fmt.Errorf("fan-group %q has %d variants, want %d", name, len(g.Variants), n)
	}
	return nil
}

func (w *corpusWorld) contractRequires(name, a, b string) error {
	g, err := w.group(name)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, f := range g.Contract().RequireFields {
		have[f] = true
	}
	if !have[a] || !have[b] {
		return fmt.Errorf("contract requires %v, want %q and %q", g.Contract().RequireFields, a, b)
	}
	return nil
}

func (w *corpusWorld) contractWords(name string, n int) error {
	g, err := w.group(name)
	if err != nil {
		return err
	}
	if g.Contract().MaxWords != n {
		return fmt.Errorf("contract MaxWords = %d, want %d", g.Contract().MaxWords, n)
	}
	return nil
}

func (w *corpusWorld) aggQuorum(name string, n int) error {
	g, err := w.group(name)
	if err != nil {
		return err
	}
	if q := g.Aggregator().Quorum; q != n {
		return fmt.Errorf("aggregator quorum = %d, want %d", q, n)
	}
	return nil
}

func (w *corpusWorld) invCount(n int) error {
	if len(w.invs) != n {
		return fmt.Errorf("%d invocations produced, want %d", len(w.invs), n)
	}
	return nil
}

func (w *corpusWorld) invContract() error {
	g, err := w.group("triage")
	if err != nil {
		return err
	}
	want := strings.Join(g.Contract().RequireFields, ",")
	for _, inv := range w.invs {
		if strings.Join(inv.Contract.RequireFields, ",") != want {
			return fmt.Errorf("invocation %q carries contract %v, want %v", inv.Tag, inv.Contract.RequireFields, g.Contract().RequireFields)
		}
	}
	return nil
}

func (w *corpusWorld) invVotesOn(field string) error {
	for _, inv := range w.invs {
		if inv.VerdictField != field {
			return fmt.Errorf("invocation %q votes on %q, want %q", inv.Tag, inv.VerdictField, field)
		}
	}
	return nil
}

func (w *corpusWorld) invSubstitutes() error {
	turn := w.vars["turn"]
	for _, inv := range w.invs {
		if !strings.Contains(inv.Prompt, turn) {
			return fmt.Errorf("invocation %q prompt did not substitute the turn", inv.Tag)
		}
	}
	return nil
}

func (w *corpusWorld) invNoPlaceholder() error {
	for _, inv := range w.invs {
		if strings.Contains(inv.Prompt, "{{") {
			return fmt.Errorf("invocation %q prompt has an unfilled placeholder: %q", inv.Tag, inv.Prompt)
		}
	}
	return nil
}

func (w *corpusWorld) invDistinctTemps() error {
	seen := map[float64]bool{}
	for _, inv := range w.invs {
		seen[inv.Params.Temperature] = true
	}
	if len(seen) < 2 {
		return fmt.Errorf("invocations carry only %d distinct temperature(s)", len(seen))
	}
	return nil
}
