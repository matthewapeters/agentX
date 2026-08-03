// Package toolsteps holds Godog step definitions for the tools domain (TOOL-1+).
package toolsteps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/cucumber/godog"

	"agentx/internal/tools"
)

type policyWorld struct {
	reg     *tools.Registry
	pol     *tools.Policy
	verdict tools.Verdict
	src     *tools.Policy // pre-persistence source policy
	dir     string        // temp dir for persisted policy files
}

// InitializeScenario registers the tools-domain steps.
func InitializeScenario(sc *godog.ScenarioContext) {
	registerExecSteps(sc)
	registerOutputSizeSteps(sc)

	w := &policyWorld{}

	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if w.dir != "" {
			_ = os.RemoveAll(w.dir)
		}
		w.dir = ""
		return ctx, nil
	})

	sc.Step(`^a tool policy$`, w.newPolicy)
	sc.Step(`^the tool "([^"]*)" is blacklisted$`, w.blacklistTool)
	sc.Step(`^the tool "([^"]*)" is denied when an argument matches "([^"]*)"$`, w.blacklistMatch)
	sc.Step(`^"([^"]*)" is approved for the "([^"]*)" with:$`, w.approve)
	sc.Step(`^"([^"]*)" is approved for the plan "([^"]*)" with:$`, w.approvePlan)
	sc.Step(`^"([^"]*)" is plan-approved for "([^"]*)" with:$`, w.assertPlanApproved)
	sc.Step(`^"([^"]*)" is not plan-approved for "([^"]*)" with:$`, w.assertPlanNotApproved)
	sc.Step(`^the plan "([^"]*)" ends$`, w.expirePlan)
	sc.Step(`^"([^"]*)" is evaluated with:$`, w.evaluate)
	sc.Step(`^"([^"]*)" is evaluated with no arguments$`, w.evaluateNoArgs)
	sc.Step(`^the decision is "([^"]*)"$`, w.decisionIs)
	sc.Step(`^the reason is "([^"]*)"$`, w.reasonIs)

	sc.Step(`^a global approval of "([^"]*)" with:$`, w.globalApproval)
	sc.Step(`^the approvals are saved and reloaded into a fresh policy$`, w.reloadApprovals)
	sc.Step(`^a blacklist file denying tool "([^"]*)" matching "([^"]*)"$`, w.blacklistFile)
	sc.Step(`^the blacklist is loaded into a fresh policy$`, w.loadBlacklist)
}

func (w *policyWorld) tempDir() (string, error) {
	if w.dir != "" {
		return w.dir, nil
	}
	d, err := os.MkdirTemp("", "agentx-policy-")
	if err != nil {
		return "", err
	}
	w.dir = d
	return d, nil
}

func (w *policyWorld) globalApproval(id string, table *godog.Table) error {
	w.reg = tools.DefaultRegistry()
	d, ok := w.reg.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	w.src = tools.NewPolicy()
	w.src.Approve(tools.ScopeGlobal, d, tableArgs(table))
	return nil
}

func (w *policyWorld) reloadApprovals() error {
	dir, err := w.tempDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "agentx-tool-approvals.toml")
	if err := tools.SaveApprovals(path, w.src.GlobalApprovals()); err != nil {
		return err
	}
	entries, err := tools.LoadApprovals(path)
	if err != nil {
		return err
	}
	w.pol = tools.NewPolicy()
	w.pol.LoadGlobal(entries)
	return nil
}

func (w *policyWorld) blacklistFile(id, pattern string) error {
	dir, err := w.tempDir()
	if err != nil {
		return err
	}
	content := fmt.Sprintf("[[rule]]\ntool = %q\npattern = %q\nreason = \"sensitive_path\"\n", id, pattern)
	return os.WriteFile(filepath.Join(dir, "agentx-tool-blacklist.toml"), []byte(content), 0o644)
}

func (w *policyWorld) loadBlacklist() error {
	rules, err := tools.LoadBlacklist(filepath.Join(w.dir, "agentx-tool-blacklist.toml"))
	if err != nil {
		return err
	}
	w.reg = tools.DefaultRegistry()
	w.pol = tools.NewPolicy(rules...)
	return nil
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

func (w *policyWorld) approvePlan(id, root string, table *godog.Table) error {
	d, ok := w.reg.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	w.pol.ApprovePlan(root, d, tableArgs(table))
	return nil
}

func (w *policyWorld) assertPlanApproved(id, root string, table *godog.Table) error {
	d, ok := w.reg.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	if !w.pol.PlanApproved(root, d, tableArgs(table)) {
		return fmt.Errorf("PlanApproved(%q) = false for %q, want true", root, id)
	}
	return nil
}

func (w *policyWorld) assertPlanNotApproved(id, root string, table *godog.Table) error {
	d, ok := w.reg.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown tool %q", id)
	}
	if w.pol.PlanApproved(root, d, tableArgs(table)) {
		return fmt.Errorf("PlanApproved(%q) = true for %q, want false", root, id)
	}
	return nil
}

func (w *policyWorld) expirePlan(root string) error {
	w.pol.ExpirePlan(root)
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
