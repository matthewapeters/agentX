// Package toolsteps holds Godog step definitions for the tools domain (TOOL-1+).
package toolsteps

import (
	"fmt"
	"regexp"

	"github.com/cucumber/godog"

	"agentx/internal/tools"
)

type policyWorld struct {
	reg     *tools.Registry
	pol     *tools.Policy
	verdict tools.Verdict
}

// InitializeScenario registers the tools-domain steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	w := &policyWorld{}

	sc.Step(`^a tool policy$`, w.newPolicy)
	sc.Step(`^the tool "([^"]*)" is blacklisted$`, w.blacklistTool)
	sc.Step(`^the tool "([^"]*)" is denied when an argument matches "([^"]*)"$`, w.blacklistMatch)
	sc.Step(`^"([^"]*)" is approved for the "([^"]*)" with:$`, w.approve)
	sc.Step(`^"([^"]*)" is evaluated with:$`, w.evaluate)
	sc.Step(`^"([^"]*)" is evaluated with no arguments$`, w.evaluateNoArgs)
	sc.Step(`^the decision is "([^"]*)"$`, w.decisionIs)
	sc.Step(`^the reason is "([^"]*)"$`, w.reasonIs)
}

func (w *policyWorld) newPolicy() error {
	w.reg = tools.DefaultRegistry()
	w.pol = tools.NewPolicy()
	w.verdict = tools.Verdict{}
	return nil
}

func (w *policyWorld) blacklistTool(id string) error {
	w.pol.AddRule(tools.Rule{Tool: id})
	return nil
}

func (w *policyWorld) blacklistMatch(id, pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	w.pol.AddRule(tools.Rule{Tool: id, Pattern: re})
	return nil
}

func (w *policyWorld) approve(id, scope string, table *godog.Table) error {
	d, ok := w.reg.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	s := tools.ScopeSession
	if scope == "global" {
		s = tools.ScopeGlobal
	}
	w.pol.Approve(s, d, tableArgs(table))
	return nil
}

func (w *policyWorld) evaluate(id string, table *godog.Table) error {
	d, ok := w.reg.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	w.verdict = w.pol.Evaluate(d, tableArgs(table))
	return nil
}

func (w *policyWorld) evaluateNoArgs(id string) error {
	d, ok := w.reg.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	w.verdict = w.pol.Evaluate(d, map[string]string{})
	return nil
}

func (w *policyWorld) decisionIs(want string) error {
	got := decisionName(w.verdict.Decision)
	if got != want {
		return fmt.Errorf("decision = %q, want %q", got, want)
	}
	return nil
}

func (w *policyWorld) reasonIs(want string) error {
	if w.verdict.Reason != want {
		return fmt.Errorf("reason = %q, want %q", w.verdict.Reason, want)
	}
	return nil
}

func decisionName(d tools.Decision) string {
	switch d {
	case tools.Allow:
		return "allow"
	case tools.Deny:
		return "deny"
	case tools.NeedsApproval:
		return "needs_approval"
	default:
		return "unknown"
	}
}

// tableArgs converts an arg|value table (header row skipped) to a map.
func tableArgs(table *godog.Table) map[string]string {
	args := map[string]string{}
	for _, row := range table.Rows[1:] {
		args[row.Cells[0].Value] = row.Cells[1].Value
	}
	return args
}
