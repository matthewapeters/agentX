package tidal

import (
	"agentx/internal/prompting/task"
	"strings"
	"testing"
)

func TestRender_EmptyGraph(t *testing.T) {
	g := task.NewGraph()
	got := Render(g)
	want := ""
	if got != want {
		t.Errorf("Render(empty) = %q, want %q", got, want)
	}
}

func TestRender_NilGraph(t *testing.T) {
	got := Render(nil)
	want := ""
	if got != want {
		t.Errorf("Render(nil) = %q, want %q", got, want)
	}
}

// Scenario 1: graph with only a root record (KindTask, Status Proposed, Goal
// "Fix the login endpoint") — only PROBLEM section rendered.
func TestRender_OnlyRoot(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:     "root",
		Kind:   task.KindTask,
		Status: task.Proposed,
		Goal:   "Fix the login endpoint",
		Deps:   []string{},
	})
	got := Render(g)
	want := "# PROBLEM\nFix the login endpoint"
	if got != want {
		t.Errorf("Render:\n%s\n\ngot != want:\n%s", got, want)
	}
}

// Scenario 2: root with ResolutionCriteria, Done tasks in KNOWN.
func TestRender_ResolutionCriteriaAndKnown(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		// Status can be anything; it's the root.
		Goal: "Find why CI is flaky",
		ResolutionCriteria: []task.ResolutionAssertion{
			{
				Text:    "Logs show OOM",
				Outcome: task.OutcomeSatisfied,
				Evidence: []task.Evidence{
					{NodeID: "logs", Stance: task.StanceSupports},
				},
			},
			{Text: "Disk full"},
		},
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:     "logs",
		Kind:   task.KindTask,
		Status: task.Done,
		Goal:   "logs",
		Value:  "OOM at 2GB limit",
		Deps:   []string{},
	})
	_ = g.Add(task.Record{
		ID:     "disk_check",
		Kind:   task.KindTask,
		Status: task.Done,
		Goal:   "disk_check",
		Value:  "80% used",
		Deps:   []string{},
	})

	got := Render(g)
	want := `# PROBLEM
Find why CI is flaky

# RESOLUTION CRITERIA (any one satisfies)
[x] Logs show OOM — satisfied: logs: OOM at 2GB limit
[ ] Disk full

# KNOWN
- logs: OOM at 2GB limit
- disk_check: 80% used`
	if got != want {
		t.Errorf("Render:\n%s\n\ngot != want:\n%s", got, want)
	}
}

// Scenario 3: hypotheses with various likelihoods including Impossible.
func TestRender_Hypotheses(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Diagnose high CPU",
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:         "h1",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "Application leak",
		Likelihood: task.LikelihoodHigh,
		Evidence: []task.Evidence{
			{NodeID: "heap_dump", Stance: task.StanceSupports},
			{NodeID: "cpu_profile", Stance: task.StanceRefutes},
		},
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:         "h2",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "Kernel bug",
		Likelihood: task.LikelihoodMedium,
		Deps:       []string{},
	})
	_ = g.Add(task.Record{
		ID:         "h3",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "Sensor malfunction",
		Likelihood: task.LikelihoodImpossible,
		Deps:       []string{},
	})
	_ = g.Add(task.Record{
		ID:     "heap_dump",
		Kind:   task.KindTask,
		Status: task.Done,
		Goal:   "heap_dump",
		Value:  "400MB leaked in handler",
		Deps:   []string{},
	})
	_ = g.Add(task.Record{
		ID:     "cpu_profile",
		Kind:   task.KindTask,
		Status: task.Failed,
		Goal:   "cpu_profile",
		Error:  "profile timeout",
		Deps:   []string{},
	})

	got := Render(g)
	want := `# PROBLEM
Diagnose high CPU

# HYPOTHESES
## Application leak — likelihood (H/M/L/I): H
### Evidence
- [supports] heap_dump: 400MB leaked in handler
- [refutes]  cpu_profile: profile timeout

## Kernel bug — likelihood (H/M/L/I): M
### Evidence
(No evidence linked)

# KNOWN
- heap_dump: 400MB leaked in handler`

	if got != want {
		t.Errorf("Render:\n%s\n\ngot != want:\n%s", got, want)
	}

	// Verify Impossible hypothesis is entirely absent.
	if strings.Contains(got, "Sensor malfunction") {
		t.Error("Impossible hypothesis must not appear in render output")
	}
}

// Scenario 4: NEED TO KNOW — diagnostic and deferred.
func TestRender_NeedToKnow(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Investigate",
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:     "q1",
		Kind:   task.KindTask,
		Status: task.Proposed,
		Goal:   "Check disk usage",
		Deps:   []string{},
	})
	_ = g.Add(task.Record{
		ID:     "q2",
		Kind:   task.KindTask,
		Status: task.Proposed,
		Goal:   "Review historical logs",
		Deferred: true,
		Deps:   []string{},
	})
	_ = g.Add(task.Record{
		ID:     "q3",
		Kind:   task.KindStep,
		Status: task.Ready,
		Goal:   "Run benchmark",
		Deps:   []string{},
	})

	got := Render(g)
	want := `# PROBLEM
Investigate

# NEED TO KNOW — diagnostic
- Check disk usage
- Run benchmark

# NEED TO KNOW — deferred
- Review historical logs`

	if got != want {
		t.Errorf("Render:\n%s\n\ngot != want:\n%s", got, want)
	}
}

// Scenario 5: Evidence referencing a NodeID not in the graph — skip silently.
func TestRender_BrokenEvidenceReference(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Test",
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:         "h1",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "Bad ref hypothesis",
		Likelihood: task.LikelihoodMedium,
		Evidence: []task.Evidence{
			{NodeID: "missing_node", Stance: task.StanceSupports},
			{NodeID: "real_node", Stance: task.StanceSupports},
		},
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:     "real_node",
		Kind:   task.KindTask,
		Status: task.Done,
		Goal:   "real_node",
		Value:  "some fact",
		Deps:   []string{},
	})

	got := Render(g)

	if !strings.Contains(got, "[supports] real_node: some fact") {
		t.Errorf("valid evidence should be rendered:\n%s", got)
	}
	// No panic, no error — the broken reference is simply absent from output.
	if strings.Contains(got, "missing_node") {
		t.Errorf("broken evidence reference should not appear in output:\n%s", got)
	}
}

// Scenario 6: Evidence referencing a Done node with Value — normal happy path.
func TestRender_EvidenceWithDoneValue(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Test",
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:         "h1",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "h1",
		Likelihood: task.LikelihoodHigh,
		Evidence: []task.Evidence{
			{NodeID: "f1", Stance: task.StanceSupports},
		},
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:     "f1",
		Kind:   task.KindTask,
		Status: task.Done,
		Goal:   "f1",
		Value:  "answer",
		Deps:   []string{},
	})

	got := Render(g)
	want := "- [supports] f1: answer"
	if !strings.Contains(got, want) {
		t.Errorf("expected evidence line %q in:\n%s", want, got)
	}
}

// Scenario 7: Evidence referencing a Failed node with Error.
func TestRender_EvidenceWithFailedError(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Test",
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:         "h1",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "h1",
		Likelihood: task.LikelihoodHigh,
		Evidence: []task.Evidence{
			{NodeID: "f1", Stance: task.StanceRefutes},
		},
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:     "f1",
		Kind:   task.KindTask,
		Status: task.Failed,
		Goal:   "f1",
		Error:  "connection refused",
		Deps:   []string{},
	})

	got := Render(g)
	want := "- [refutes]  f1: connection refused"
	if !strings.Contains(got, want) {
		t.Errorf("expected evidence line %q in:\n%s", want, got)
	}
}

// Scenario 8: Evidence referencing an open (non-Done/Failed) node.
func TestRender_EvidenceWithOpenNode(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Test",
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:         "h1",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "h1",
		Likelihood: task.LikelihoodMedium,
		Evidence: []task.Evidence{
			{NodeID: "q1", Stance: task.StanceSupports},
		},
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:     "q1",
		Kind:   task.KindTask,
		Status: task.Proposed,
		Goal:   "q1",
		Deps:   []string{},
	})

	got := Render(g)
	want := "- [supports] q1"
	if !strings.Contains(got, want) {
		t.Errorf("expected evidence line %q in:\n%s", want, got)
	}
}

// Scenario 9: Refuted criterion with Evidence.
func TestRender_RefutedCriterion(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Test",
		ResolutionCriteria: []task.ResolutionAssertion{
			{
				Text:    "Something was wrong",
				Outcome: task.OutcomeRefuted,
				Evidence: []task.Evidence{
					{NodeID: "f1", Stance: task.StanceRefutes},
				},
			},
		},
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:     "f1",
		Kind:   task.KindTask,
		Status: task.Done,
		Goal:   "fact1",
		Value:  "it was fine",
		Deps:   []string{},
	})

	got := Render(g)
	want := "[ ] Something was wrong — refuted: fact1: it was fine"
	if !strings.Contains(got, want) {
		t.Errorf("expected criterion line %q in:\n%s", want, got)
	}
}

// Scenario 10: Low-likelihood hypothesis fully rendered.
func TestRender_LowLikelihoodHypothesis(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Test",
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:         "h1",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "Edge case",
		Likelihood: task.LikelihoodLow,
		Evidence: []task.Evidence{
			{NodeID: "f1", Stance: task.StanceSupports},
		},
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:     "f1",
		Kind:   task.KindTask,
		Status: task.Done,
		Goal:   "f1",
		Value:  "supporting fact",
		Deps:   []string{},
	})

	got := Render(g)
	if !strings.Contains(got, "likelihood (H/M/L/I): L") {
		t.Errorf("Low likelihood must be shown:\n%s", got)
	}
	if !strings.Contains(got, "Edge case") {
		t.Errorf("Low hypothesis must be rendered:\n%s", got)
	}
}

// Scenario 11: Multiple hypotheses same likelihood — first-seen order.
func TestRender_MultipleSameLikelihood(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Test",
		Deps: []string{},
	})
	// Add in order: C (M), A (M), B (M)
	_ = g.Add(task.Record{
		ID:         "c",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "C-hyp",
		Likelihood: task.LikelihoodMedium,
		Deps:       []string{},
	})
	_ = g.Add(task.Record{
		ID:         "a",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "A-hyp",
		Likelihood: task.LikelihoodMedium,
		Deps:       []string{},
	})
	_ = g.Add(task.Record{
		ID:         "b",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "B-hyp",
		Likelihood: task.LikelihoodMedium,
		Deps:       []string{},
	})

	got := Render(g)

	// Find positions — C must come before A, A before B.
	posC := strings.Index(got, "C-hyp")
	posA := strings.Index(got, "A-hyp")
	posB := strings.Index(got, "B-hyp")

	if posC >= posA || posA >= posB {
		t.Errorf("same-likelihood hypotheses must be in first-seen order (C, A, B);\ngot positions C=%d A=%d B=%d\n%s", posC, posA, posB, got)
	}
}

// Scenario 12: Empty ResolutionCriteria — section omitted.
func TestRender_EmptyResolutionCriteria(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:     "root",
		Kind:   task.KindTask,
		Status: task.Proposed,
		Goal:   "Just a goal",
		// ResolutionCriteria is nil/empty by default.
		Deps: []string{},
	})

	got := Render(g)
	if strings.Contains(got, "RESOLUTION CRITERIA") {
		t.Errorf("empty criteria should not produce a section:\n%s", got)
	}
	want := "# PROBLEM\nJust a goal"
	if got != want {
		t.Errorf("Render:\n%s\n\nwant:\n%s", got, want)
	}
}

// Scenario 13: All open records Deferred — diagnostic subsection omitted.
func TestRender_AllDeferred(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Test",
		Deps: []string{},
	})
	_ = g.Add(task.Record{
		ID:       "q1",
		Kind:     task.KindTask,
		Status:   task.Proposed,
		Goal:     "Q1",
		Deferred: true,
		Deps:     []string{},
	})
	_ = g.Add(task.Record{
		ID:       "q2",
		Kind:     task.KindTask,
		Status:   task.Ready,
		Goal:     "Q2",
		Deferred: true,
		Deps:     []string{},
	})

	got := Render(g)
	if strings.Contains(got, "NEED TO KNOW — diagnostic") {
		t.Errorf("diagnostic section should be omitted when no non-deferred records:\n%s", got)
	}
	if !strings.Contains(got, "NEED TO KNOW — deferred") {
		t.Errorf("deferred section should be present:\n%s", got)
	}
}

// Scenario 14: All open records non-deferred — deferred subsection omitted.
func TestRender_NoDeferred(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:     "root",
		Kind:   task.KindTask,
		Goal:   "Test",
		Deps:   []string{},
	})
	_ = g.Add(task.Record{
		ID:     "q1",
		Kind:   task.KindTask,
		Status: task.Proposed,
		Goal:   "Q1",
		Deps:   []string{},
	})
	_ = g.Add(task.Record{
		ID:     "q2",
		Kind:   task.KindTask,
		Status: task.Ready,
		Goal:   "Q2",
		Deps:   []string{},
	})

	got := Render(g)
	if strings.Contains(got, "NEED TO KNOW — deferred") {
		t.Errorf("deferred section should be omitted when no deferred records:\n%s", got)
	}
	if !strings.Contains(got, "NEED TO KNOW — diagnostic") {
		t.Errorf("diagnostic section should be present:\n%s", got)
	}
}

// Scenario 15: Done KindTask with both Value and Error set — uses Value.
func TestRender_DoneWithBothValueAndError(t *testing.T) {
	g := task.NewGraph()
	_ = g.Add(task.Record{
		ID:     "t1",
		Kind:   task.KindTask,
		Status: task.Done,
		Goal:   "t1",
		Value:  "resolved fact",
		Error:  "stale error",
		Deps:   []string{},
	})

	got := Render(g)
	if !strings.Contains(got, "- t1: resolved fact") {
		t.Errorf("Done node should use Value (primary): %s", got)
	}
}

// Composite scenario: realistic mixed graph.
func TestRender_Composite(t *testing.T) {
	g := task.NewGraph()

	// Root.
	_ = g.Add(task.Record{
		ID:   "root",
		Kind: task.KindTask,
		Goal: "Find why the cache layer is evicting too aggressively",
		ResolutionCriteria: []task.ResolutionAssertion{
			{Text: "Identify the eviction trigger threshold", Outcome: task.OutcomeSatisfied, Evidence: []task.Evidence{{NodeID: "threshold_probe", Stance: task.StanceSupports}}},
			{Text: "Confirm it's not a GC pause"},
		},
		Deps: []string{},
	})

	// Hypothesis (High).
	_ = g.Add(task.Record{
		ID:         "h1",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "Threshold too low in prod config",
		Likelihood: task.LikelihoodHigh,
		Evidence: []task.Evidence{
			{NodeID: "config_diff", Stance: task.StanceSupports},
		},
		Deps: []string{},
	})

	// Hypothesis (Medium).
	_ = g.Add(task.Record{
		ID:         "h2",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "Memory pressure from a leak",
		Likelihood: task.LikelihoodMedium,
		Deps:       []string{},
	})

	// Hypothesis (Impossible) — must NOT appear.
	_ = g.Add(task.Record{
		ID:         "h3",
		Kind:       task.KindHypothesis,
		Status:     task.Proposed,
		Goal:       "Network partition",
		Likelihood: task.LikelihoodImpossible,
		Deps:       []string{},
	})

	// Done fact.
	_ = g.Add(task.Record{
		ID:     "threshold_probe",
		Kind:   task.KindTask,
		Status: task.Done,
		Goal:   "threshold_probe",
		Value:  "prod threshold is 60%, staging is 80%",
		Deps:   []string{},
	})

	// Open diagnostic question.
	_ = g.Add(task.Record{
		ID:     "q1",
		Kind:   task.KindTask,
		Status: task.Proposed,
		Goal:   "Compare staging vs prod memory configs",
		Deps:   []string{},
	})

	// Open deferred question.
	_ = g.Add(task.Record{
		ID:       "q2",
		Kind:     task.KindTask,
		Status:   task.Proposed,
		Goal:     "Check if any recent config deploys",
		Deferred: true,
		Deps:     []string{},
	})

	got := Render(g)

	// Must contain expected sections.
	for _, need := range []string{
		"# PROBLEM",
		"# RESOLUTION CRITERIA (any one satisfies)",
		"# HYPOTHESES",
		"# NEED TO KNOW — diagnostic",
		"# NEED TO KNOW — deferred",
		"Threshold too low in prod config",
		"Memory pressure from a leak",
	} {
		if !strings.Contains(got, need) {
			t.Errorf("composite render missing %q:\n%s", need, got)
		}
	}

	// Impossible hypothesis must NOT appear.
	if strings.Contains(got, "Network partition") {
		t.Errorf("Impossible hypothesis 'Network partition' must not appear in render:\n%s", got)
	}

	// Resolution criteria satisfied annotation.
	if !strings.Contains(got, "Identify the eviction trigger threshold — satisfied: threshold_probe: prod threshold is 60%, staging is 80%") {
		t.Errorf("satisfied criterion must cite evidence:\n%s", got)
	}
}
