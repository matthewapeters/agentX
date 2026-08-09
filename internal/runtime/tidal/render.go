package tidal

import (
	"agentx/internal/prompting/task"
	"fmt"
	"strings"
)

// Render produces the five-section consolidation template from a task.Graph.
// Pure function — no LLM calls, no side effects, deterministic output for a
// given graph state. Consumed by every downstream Tidal component (continue_
// investigating wrapper, Tier 2 operations, ConsolidatorHook).
//
// Output sections, in order, only when populated:
//   # PROBLEM
//   # RESOLUTION CRITERIA
//   # HYPOTHESES (Impossible-likelihood hypotheses omitted)
//   # KNOWN
//   # NEED TO KNOW — diagnostic
//   # NEED TO KNOW — deferred
//
// Empty sections are omitted entirely — no header, no blank lines in their
// place. Sections are separated by a single blank line.
//
// Design decision (ADR 0014 Phase 2): Impossible-likelihood hypotheses are
// entirely absent from the render output. They remain in the graph for audit
// and for later phases that may reference them, but the prompt-facing render
// drops them the same way it drops any other empty section. Rationale:
// Impossible means "decisively refuted by observation" — the model cannot act
// on a refuted hypothesis, and including it even as a one-line note wastes
// tokens the Context Curation discipline is specifically designed to save.
func Render(g *task.Graph) string {
	var sections []string

	if g == nil {
		return ""
	}

	root, hasRoot := findRoot(g)

	// --- PROBLEM ---
	if hasRoot && root.Goal != "" {
		sections = append(sections, "# PROBLEM\n"+root.Goal)
	}

	// --- RESOLUTION CRITERIA ---
	if hasRoot && len(root.ResolutionCriteria) > 0 {
		sections = append(sections, "# RESOLUTION CRITERIA (any one satisfies)\n"+renderResolutionCriteria(root, g))
	}

	// --- HYPOTHESES ---
	if h := renderHypotheses(g); h != "" {
		sections = append(sections, "# HYPOTHESES\n"+h)
	}

	// --- KNOWN ---
	if k := renderKnown(g); k != "" {
		sections = append(sections, "# KNOWN\n"+k)
	}

	// --- NEED TO KNOW ---
	if n := renderNeedToKnow(g, rootID(g)); n != "" {
		sections = append(sections, n)
	}

	return strings.Join(sections, "\n\n")
}

// findRoot returns the first KindTask root node, if any.
func findRoot(g *task.Graph) (task.Record, bool) {
	for _, id := range g.Roots() {
		rec, ok := g.Node(id)
		if ok && rec.Kind == task.KindTask {
			return rec, true
		}
	}
	return task.Record{}, false
}

// rootID returns the ID of the graph's investigation root (first KindTask root),
// or "" if there is none.
func rootID(g *task.Graph) string {
	for _, id := range g.Roots() {
		rec, ok := g.Node(id)
		if ok && rec.Kind == task.KindTask {
			return id
		}
	}
	return ""
}

// renderResolutionCriteria formats the root's resolution criteria body (no
// header — caller prepends "# RESOLUTION CRITERIA ...").
func renderResolutionCriteria(root task.Record, g *task.Graph) string {
	var b strings.Builder
	for _, c := range root.ResolutionCriteria {
		switch c.Outcome {
		case task.OutcomeSatisfied:
			cited := citeEvidenceGraph(g, c.Evidence)
			fmt.Fprintf(&b, "[x] %s — satisfied: %s\n", c.Text, cited)
		case task.OutcomeRefuted:
			cited := citeEvidenceGraph(g, c.Evidence)
			fmt.Fprintf(&b, "[ ] %s — refuted: %s\n", c.Text, cited)
		default:
			// OutcomeAbstained, empty, or any other value — unchecked, no
			// annotation.
			fmt.Fprintf(&b, "[ ] %s\n", c.Text)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// citeEvidenceGraph looks up each Evidence's NodeID in the graph and renders
// the referenced fact's Goal (plus Value if Done or Error if Failed). Missing
// NodeIDs are skipped silently.
func citeEvidenceGraph(g *task.Graph, evidences []task.Evidence) string {
	var parts []string
	for _, ev := range evidences {
		fact, ok := g.Node(ev.NodeID)
		if !ok {
			continue
		}
		switch {
		case fact.Status == task.Done && fact.Value != "":
			parts = append(parts, fmt.Sprintf("%s: %s", fact.Goal, fact.Value))
		case fact.Status == task.Failed && fact.Error != "":
			parts = append(parts, fmt.Sprintf("%s: %s", fact.Goal, fact.Error))
		default:
			parts = append(parts, fact.Goal)
		}
	}
	return strings.Join(parts, ", ")
}

// renderHypotheses builds the HYPOTHESES section body. Impossible-likelihood
// hypotheses are excluded. Within each likelihood tier (H, M, L), hypotheses
// appear in first-seen graph order.
func renderHypotheses(g *task.Graph) string {
	// Likelihood tier ordering: H first, then M, then L. Impossible excluded.
	tiers := []task.Likelihood{task.LikelihoodHigh, task.LikelihoodMedium, task.LikelihoodLow}

	var entries []string
	for _, tier := range tiers {
		for _, rec := range g.Nodes() {
			if rec.Kind != task.KindHypothesis || rec.Likelihood != tier {
				continue
			}
			entries = append(entries, renderHypothesisEntry(rec, g))
		}
	}
	return strings.Join(entries, "\n\n")
}

// renderHypothesisEntry returns one hypothesis entry as a string (header +
// evidence block, no trailing newline).
func renderHypothesisEntry(hyp task.Record, g *task.Graph) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s — likelihood (H/M/L/I): %s", hyp.Goal, hyp.Likelihood)

	if len(hyp.Evidence) == 0 {
		b.WriteString("\n### Evidence\n(No evidence linked)")
		return b.String()
	}

	// Render evidence entries, skipping broken references.
	var lines []string
	for _, ev := range hyp.Evidence {
		fact, ok := g.Node(ev.NodeID)
		if !ok {
			// Broken reference — skip silently (schema violation).
			continue
		}
		switch {
		case fact.Status == task.Done && fact.Value != "":
			lines = append(lines, fmt.Sprintf("- [%s] %s: %s", ev.Stance, fact.Goal, fact.Value))
		case fact.Status == task.Failed && fact.Error != "":
			lines = append(lines, fmt.Sprintf("- [%s]  %s: %s", ev.Stance, fact.Goal, fact.Error))
		default:
			// Open node (no Value/Error) — show goal only.
			lines = append(lines, fmt.Sprintf("- [%s] %s", ev.Stance, fact.Goal))
		}
	}

	if len(lines) == 0 {
		// All evidence references were broken — render as empty.
		b.WriteString("\n### Evidence\n(No evidence linked)")
	} else {
		b.WriteString("\n### Evidence\n")
		b.WriteString(strings.Join(lines, "\n"))
	}

	return b.String()
}

// renderKnown builds the KNOWN section body. Every Done KindTask node is
// included regardless of evidence linkage.
func renderKnown(g *task.Graph) string {
	var lines []string
	for _, rec := range g.Nodes() {
		if rec.Kind != task.KindTask || rec.Status != task.Done {
			continue
		}
		if rec.Value != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s", rec.Goal, rec.Value))
		} else {
			lines = append(lines, fmt.Sprintf("- %s", rec.Goal))
		}
	}
	return strings.Join(lines, "\n")
}

// renderNeedToKnow builds the NEED TO KNOW section, split into diagnostic and
// deferred subsections. Excludes KindHypothesis records (they appear in
// HYPOTHESES instead), resolved nodes (Done/Failed/Denied/Cancelled), and the
// investigation root (identified by rootID).
func renderNeedToKnow(g *task.Graph, rootID string) string {
	var diag, deferred []string

	for _, rec := range g.Nodes() {
		if rec.ID == rootID {
			continue
		}
		if isResolved(rec) {
			continue
		}
		if rec.Kind == task.KindHypothesis {
			continue
		}
		line := "- " + rec.Goal
		if rec.Deferred {
			deferred = append(deferred, line)
		} else {
			diag = append(diag, line)
		}
	}

	var b strings.Builder

	if len(diag) > 0 {
		b.WriteString("# NEED TO KNOW — diagnostic\n")
		b.WriteString(strings.Join(diag, "\n"))
	}

	if len(deferred) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("# NEED TO KNOW — deferred\n")
		b.WriteString(strings.Join(deferred, "\n"))
	}

	return b.String()
}

// isResolved reports whether a node is in a terminal state.
func isResolved(rec task.Record) bool {
	switch rec.Status {
	case task.Done, task.Failed, task.Denied, task.Cancelled:
		return true
	}
	return false
}
