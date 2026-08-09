# Behavior — Tidal Render: Five-Section Template (ADR 0014 Phase 2)

Status: Implemented. Realizes ADR 0014's Phased Build Plan step 2. This phase
adds a pure `Render(g *task.Graph) string` function in `internal/runtime/tidal/`
that produces the five-section consolidation template (§1 of ADR 0014) from a
`task.Graph`. No Tier 1/2 operations, no hook, no LLM call. Everything it
consumes is the schema already landed in Phase 1 (ADR 0014).

## Problem

ADR 0014 §1 defines a context schema consumed by every classify/consolidation
call — the five-section template (PROBLEM, RESOLUTION CRITERIA, HYPOTHESES,
KNOWN, NEED TO KNOW) that folds the entire task graph into a single string
before any LLM judgment. Phase 1 makes the schema exist; Phase 2 makes it
readable. The render is the single read seam Tier 1's `continue_investigating`
wrapper (phase 3), Tier 2's operations (phase 4), and `ConsolidatorHook`
(phase 5) all share — a bug here corrupts every downstream LLM call. It must be
pure, deterministic, and fully unit-testable with fixed graph fixtures and
exact expected strings.

**Design decision left open by ADR §1 for this phase to settle:** how
Impossible-likelihood hypotheses are treated in the rendered HYPOTHESES
section. ADR §1 says they "drop out of the rendered `HYPOTHESES` section once
retired (collapsed to a one-line note, or omitted from the prompt-facing render
and kept only in the stored graph for audit)". This phase must decide and
encode that decision as a concrete GIVEN/WHEN/THEN, then implement exactly that
— never leave it ambiguous or implicit in the code.

**Decision: omit Impossible-likelihood hypotheses entirely from the render.**
Rationale: `Impossible` means "decisively refuted by observation" — the model
cannot act on a refuted hypothesis, and including it even as a one-line note
wastes tokens the Context Curation discipline is specifically designed to save.
The hypothesis stays in the graph for audit and for later phases that may
reference it, but the prompt-facing render drops it the same way it drops empty
sections. A future phase may retroactively surface it in a "refuted hypotheses"
appendix if product judgment warrants that trade-off.

**Empty sections:** if a section has no entries to render, omit its header and
all its content (no blank lines in place of missing sections). The render
outputs only sections that have something to show. This keeps the template
compact for early investigation where most sections are empty, and avoids the
"all sections rendered with blank bodies" pattern that wastes tokens.

**Evidence resolution:** for each `Evidence` entry on a hypothesis, look up the
referenced `NodeID` in the graph. Render the fact's `Goal`, plus `Value` if
Status is Done or `Error` if Failed. If the `NodeID` is absent from the graph
(theoretically a schema violation since Evidence must reference existing nodes,
but possible during migration or partial-graph tests), skip that evidence entry
silently — never panic, never include a broken reference.

**Resolution criteria formatting:**
- Not yet judged (Outcome empty) or OutcomeAbstained: `[ ] {Text}`
- OutcomeSatisfied: `[x] {Text}` — satisfied: {cited evidence}
- OutcomeRefuted: `[ ] {Text}` — refuted: {cited evidence}
  Where "cited evidence" lists each Evidence's NodeID's Goal (and Value/Error).

**Hypothesis formatting:**
```
## {hyp.Goal} — likelihood (H/M/L/I): {hyp.Likelihood}
### Evidence
- [supports] {fact.Goal}: {fact.Value | fact.Error}
- [refutes]  {fact.Goal}: {fact.Error}
```
(Higher-likelihood hypotheses listed first — H, M, L — in first-seen order
within each likelihood tier.)

**Known formatting:**
```
- {fact.Goal}: {fact.Value | fact.Error}
```
Every Done KindTask record, regardless of evidence linking.

**Need to Know formatting:** split into two subsections. Open records with
`Deferred == false` go under "diagnostic"; open records with `Deferred == true`
go under "deferred". Open means Status ∉ {Done, Failed, Denied, Cancelled}.
KindHypothesis records are excluded from NEED TO KNOW — they appear in
HYPOTHESES instead.

```
# NEED TO KNOW — diagnostic
- {open question}

# NEED TO KNOW — deferred
- {open question}
```

## Design

### 1. `internal/runtime/tidal/render.go`

```go
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
func Render(g *task.Graph) string {
	// ... implementation
}
```

### 2. `internal/runtime/tidal/render_test.go`

Unit tests with fixed graph fixtures and exact expected output strings. No LLM
stub needed — Render is pure and fully deterministic.

## Behavior Scenarios

```
GIVEN a graph with only a root record (KindTask, Status Proposed, Goal "Fix
      the login endpoint")
WHEN  Render is called
THEN  output is exactly:
      "# PROBLEM\nFix the login endpoint"
      (no other sections, no trailing newline, no blank lines)

GIVEN a graph with:
      - root record: KindTask, Goal "Find why CI is flaky", ResolutionCriteria
        with two assertions: "Logs show OOM" (OutcomeSatisfied, with Evidence
        linking to node "logs" which is Done with Value "OOM at 2GB limit")
        and "Disk full" (no outcome set)
      - a Done KindTask node "logs" with Value "OOM at 2GB limit"
      - a Done KindTask node "disk_check" with Value "80% used"
WHEN  Render is called
THEN  output is exactly:
      "# PROBLEM\nFind why CI is flaky\n\n# RESOLUTION CRITERIA (any one satisfies)\n[x] Logs show OOM — satisfied: logs\n[ ] Disk full\n\n# KNOWN\n- logs: OOM at 2GB limit\n- disk_check: 80% used"

GIVEN a graph with:
      - root record: Goal "Diagnose high CPU"
      - KindHypothesis node "h1" Goal "Application leak", Likelihood High,
        Evidence linking to Done node "heap_dump" with StanceSupports
        and Done node "cpu_profile" with StanceRefutes
      - KindHypothesis node "h2" Goal "Kernel bug", Likelihood Medium, no
        Evidence
      - KindHypothesis node "h3" Goal "Sensor malfunction", Likelihood
        Impossible, no Evidence (the node exists in the graph but must not
        appear in render output)
      - Done KindTask node "heap_dump" with Value "400MB leaked in handler"
      - Done KindTask node "cpu_profile" with Error "profile timeout"
WHEN  Render is called
THEN  output is exactly:
      "# PROBLEM\nDiagnose high CPU\n\n# HYPOTHESES\n## Application leak — likelihood (H/M/L/I): H\n### Evidence\n- [supports] heap_dump: 400MB leaked in handler\n- [refutes]  cpu_profile: profile timeout\n\n## Kernel bug — likelihood (H/M/L/I): M\n### Evidence\n(No evidence linked)"

      Note: h3 ("Sensor malfunction", Impossible) is entirely absent from the
      output. The render includes NO mention of it.

GIVEN a graph with:
      - root record: Goal "Investigate"
      - KindTask node "q1" Status Proposed, Goal "Check disk usage",
        Deferred false
      - KindTask node "q2" Status Proposed, Goal "Review historical logs",
        Deferred true
      - KindStep node "q3" Status Ready, Goal "Run benchmark", Deferred false
WHEN  Render is called
THEN  output is exactly:
      "# PROBLEM\nInvestigate\n\n# NEED TO KNOW — diagnostic\n- Check disk usage\n- Run benchmark\n\n# NEED TO KNOW — deferred\n- Review historical logs"

      q1 and q3 are diagnostic (Deferred=false); q2 is deferred
      (Deferred=true). Order within each subsection follows first-seen order
      in the graph.

GIVEN a graph with a KindHypothesis node whose Evidence references a NodeID
      that does not exist in the graph (a schema violation that should not
      occur in practice, but must not panic)
WHEN  Render is called
THEN  that evidence entry is silently skipped — the hypothesis is still
      rendered (if not Impossible) with only its valid evidence entries
      shown; no error, no crash, no nil-pointer dereference.

GIVEN a graph with a KindHypothesis node whose Evidence references a Done
      node that has Value set (the normal happy path)
WHEN  Render is called
THEN  the evidence line shows the fact's Goal and its Value:
      "- [supports] {goal}: {value}"

GIVEN a graph with a KindHypothesis node whose Evidence references a Failed
      node that has Error set
WHEN  Render is called
THEN  the evidence line shows the fact's Goal and its Error:
      "- [refutes]  {goal}: {error}"
      (Note the two-space indent after "refutes" to align with "supports",
      matching ADR §1's template)

GIVEN a graph with a KindHypothesis node whose Evidence references a node
      with Status Proposed (not Done/Failed — no Value or Error set)
WHEN  Render is called
THEN  the evidence line shows only the fact's Goal, with no Value/Error
      appended:
      "- [supports] {goal}"
      (This represents an evidence entry linked to an open question — the
      hypothesis is reasoning about something not yet resolved)

GIVEN a graph with a root record that has ResolutionCriteria containing a
      Refuted criterion with Evidence
WHEN  Render is called
THEN  the criterion is shown as unchecked with refuted annotation:
      "[ ] {Text} — refuted: {evidence_goal}"

GIVEN a graph with a KindHypothesis node with Likelihood Low and Evidence
      referencing a Done node
WHEN  Render is called
THEN  the hypothesis appears in the HYPOTHESES section with its likelihood
      shown as "L" and its evidence listed — Low-likelihood hypotheses are
      fully rendered, only Impossible is omitted.

GIVEN a graph with multiple hypotheses of the same likelihood
WHEN  Render is called
THEN  they appear in first-seen order (graph.Nodes() order) within their
      likelihood tier — no secondary sort by ID or Goal.

GIVEN a graph with no nodes at all (an empty Graph)
WHEN  Render is called
THEN  output is exactly "" (empty string, no header, no whitespace)

GIVEN a graph with a root record whose ResolutionCriteria is empty (no criteria
      declared)
WHEN  Render is called
THEN  RESOLUTION CRITERIA section is omitted (no entries → no header)

GIVEN a graph where all open records are Deferred == true
WHEN  Render is called
THEN  "NEED TO KNOW — diagnostic" subsection is omitted (no diagnostic
      entries); "NEED TO KNOW — deferred" is present with all entries
GIVEN a graph where all open records are Deferred == false
WHEN  Render is called
THEN  "NEED TO KNOW — deferred" subsection is omitted (no deferred entries);
      "NEED TO KNOW — diagnostic" is present with all entries

GIVEN a graph with a Done KindTask that has both Value and Error set (should
      not happen in practice — Value is set on Done, Error on Failed — but
      validate() doesn't enforce mutual exclusion)
WHEN  Render is called
THEN  the KNOWN entry uses Value (the primary resolution signal for a Done
      node): "- {goal}: {value}"
```

## Tests

- `internal/runtime/tidal/render_test.go` (new) — one test per scenario above,
  plus an additional composite scenario that builds a realistic mixed graph
  (root + criteria + hypotheses with various likelihoods + Done tasks + open
  diagnostic/deferred records) and asserts the exact rendered string.
- Each test constructs a `task.Graph` directly via `task.NewGraph()` + `Add`,
  populating nodes with the exact shape needed for the scenario. No LLM stub,
  no fake Chat, no external dependencies.
- `internal/runtime/tidal/` package has no tests yet (it's new); this file is
  the first.
- Full existing suite must pass unchanged: `go test ./...` plus `go vet ./...`
  and `gofmt -l` clean.
- `make all` must pass before this phase is considered done.

## Explicitly out of scope for this phase

- No `internal/runtime/tidal` package beyond `render.go` and its tests.
- No `ConsolidatorHook`, no Tier 1/2 operations, no `continue_investigating`
  native tool (phases 3–5).
- No modification to `task.Graph`, `task.Record`, or `task.Kind` (Phase 1).
- No LLM prompts, no Chat interface, no stub dependencies.
- No "rendered to JSON" or other serialization — this phase produces plain
  text only.
- No section ordering changes, no template parameterization — the five
  sections render in the exact ADR §1 order, always.
