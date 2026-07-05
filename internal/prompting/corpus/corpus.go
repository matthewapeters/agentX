// Package corpus loads and renders the fan-group prompt corpus (prompts.toml):
// the machine-readable source of truth the cascade classifier votes with. A
// fan-group is a set of prompt variants plus one shared output contract that fan
// out and vote on a single question; this package parses that corpus, validates
// it structurally, and renders a group into the []fanout.Invocation the pool runs.
//
// Design: docs/architecture/prompt_fan_groups.md. Behavior contract:
// tests/features/prompting/fan_group_corpus.feature.
package corpus

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"agentx/internal/llm/fanout"
)

// knownPlaceholders are the template substitutions the builder fills at render
// time; any other {{token}} in a template is a corpus authoring error.
var knownPlaceholders = map[string]bool{
	"turn":           true,
	"session_digest": true,
	"open_tasks":     true,
	"context":        true,
}

var placeholderRE = regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)

// Corpus is the parsed prompt corpus: fan-groups keyed by id.
type Corpus struct {
	Groups map[string]*FanGroup `toml:"fangroup"`
}

// FanGroup is a set of prompt variants (+ one shared output contract) that vote
// on a single classification question.
type FanGroup struct {
	Name                string         `toml:"-"` // filled from the map key
	Stage               string         `toml:"stage"`
	Purpose             string         `toml:"purpose"`
	Width               int            `toml:"width"`
	CoarseVariant       string         `toml:"coarse_variant"`
	Quorum              int            `toml:"quorum"`
	AbstainBelow        float64        `toml:"abstain_below"`
	AlwaysEscalateTypes []string       `toml:"always_escalate_types"`
	OutputContract      OutputContract `toml:"output_contract"`
	Variants            []Variant      `toml:"variant"`
}

// OutputContract is the answer schema every variant in the group must return, so
// their votes are comparable. It compiles to a fanout.Contract (fan-in validation)
// and, later, to the model's constrained-decoding format.
type OutputContract struct {
	Require  []string            `toml:"require"`
	Enum     map[string][]string `toml:"enum"`
	MaxWords int                 `toml:"max_words"`
	MaxItems int                 `toml:"max_items"`
}

// Variant is one way of asking the group's question.
type Variant struct {
	ID          string  `toml:"id"`
	Axis        string  `toml:"axis"`
	Temperature float64 `toml:"temperature"`
	Seed        int     `toml:"seed"`
	Template    string  `toml:"template"`
}

// Load reads and validates a corpus file. On any structural error it returns the
// error; the classifier layer falls back to the shipped default with a warning.
func Load(path string) (*Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read corpus: %w", err)
	}
	return Parse(data)
}

// Parse decodes and validates a corpus from bytes.
func Parse(data []byte) (*Corpus, error) {
	var c Corpus
	if _, err := toml.Decode(string(data), &c); err != nil {
		return nil, fmt.Errorf("parse corpus: %w", err)
	}
	for name, g := range c.Groups {
		g.Name = name
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Group returns a fan-group by id.
func (c *Corpus) Group(name string) (*FanGroup, bool) {
	g, ok := c.Groups[name]
	return g, ok
}

// Validate checks the corpus is structurally coherent. It does not check model
// output (that is the fan-in Contract's job); it checks the corpus can run.
func (c *Corpus) Validate() error {
	if len(c.Groups) == 0 {
		return fmt.Errorf("corpus has no fan-groups")
	}
	// Deterministic order so errors are stable.
	names := make([]string, 0, len(c.Groups))
	for name := range c.Groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := c.Groups[name].validate(); err != nil {
			return fmt.Errorf("fan-group %q: %w", name, err)
		}
	}
	return nil
}

func (g *FanGroup) validate() error {
	if g.Width < 1 {
		return fmt.Errorf("width must be >= 1")
	}
	if g.Quorum < 1 {
		return fmt.Errorf("quorum must be >= 1")
	}
	if g.Quorum > g.Width {
		return fmt.Errorf("quorum %d exceeds width %d", g.Quorum, g.Width)
	}
	if len(g.Variants) == 0 {
		return fmt.Errorf("has no variants")
	}
	if len(g.OutputContract.Require) == 0 {
		return fmt.Errorf("output_contract.require must name at least one field")
	}
	ids := make(map[string]bool, len(g.Variants))
	for i, v := range g.Variants {
		if strings.TrimSpace(v.ID) == "" {
			return fmt.Errorf("variant %d has no id", i)
		}
		if ids[v.ID] {
			return fmt.Errorf("duplicate variant id %q", v.ID)
		}
		ids[v.ID] = true
		if strings.TrimSpace(v.Template) == "" {
			return fmt.Errorf("variant %q has an empty template", v.ID)
		}
		for _, ph := range placeholderRE.FindAllStringSubmatch(v.Template, -1) {
			if !knownPlaceholders[ph[1]] {
				return fmt.Errorf("variant %q uses unknown placeholder {{%s}}", v.ID, ph[1])
			}
		}
	}
	if !ids[g.CoarseVariant] {
		return fmt.Errorf("coarse_variant %q is not a defined variant", g.CoarseVariant)
	}
	return nil
}

// Contract compiles the group's output contract into a fanout.Contract used to
// quarantine non-conforming votes at fan-in.
func (g *FanGroup) Contract() fanout.Contract {
	return fanout.Contract{
		RequireFields: g.OutputContract.Require,
		MaxWords:      g.OutputContract.MaxWords,
		MaxMilestones: g.OutputContract.MaxItems,
	}
}

// Aggregator builds the majority-vote fold from the group's quorum and abstain
// threshold.
func (g *FanGroup) Aggregator() *fanout.MajorityVote {
	return fanout.NewMajorityVote(
		fanout.WithQuorum(g.Quorum),
		fanout.WithAbstainBelow(g.AbstainBelow),
	)
}

// Render fans the group out into Width invocations, one per variant, cycling with
// a jittered seed when Width exceeds the variant count. Templates have their
// placeholders substituted from vars; every invocation carries the shared contract.
func (g *FanGroup) Render(vars map[string]string) []fanout.Invocation {
	n := g.Width
	if n < 1 {
		n = 1
	}
	contract := g.Contract()
	invs := make([]fanout.Invocation, 0, n)
	for i := 0; i < n; i++ {
		v := g.Variants[i%len(g.Variants)]
		tag := g.Name + "/" + v.ID
		seed := v.Seed
		if i >= len(g.Variants) {
			tag = fmt.Sprintf("%s#%d", tag, i)
			seed = v.Seed + i // vary the seed on padded repeats so votes still diverge
		}
		invs = append(invs, fanout.Invocation{
			Tag:      tag,
			Prompt:   renderTemplate(v.Template, vars),
			Params:   fanout.Params{Temperature: v.Temperature, Seed: seed},
			Contract: contract,
		})
	}
	return invs
}

func renderTemplate(tmpl string, vars map[string]string) string {
	out := tmpl
	for k, val := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", val)
	}
	return strings.TrimSpace(out)
}
