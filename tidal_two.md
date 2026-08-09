Implement Phase 2 of ADR 0014 (Tidal): the render function.

No behavior doc exists yet for this phase — ADR 0014's Phased Build Plan is
explicit that each phase's GIVEN/WHEN/THEN behavior doc is written immediately
before that phase starts, not up front (Phase 1's is already written and
merged: docs/architecture/behavior/adr/0014_tidal_schema.feature.md). Write
docs/architecture/behavior/adr/0014_tidal_render.feature.md first, following
that same repo convention and the same level of rigor as Phase 1's doc, then
implement against it.

Read first: docs/architecture/adr/0014-tidal-hypothesis-grounded-consolidation.md's
Decision §1 (the context schema and the five-section render template) and
Phased Build Plan step 2. Also read the already-landed schema itself
(internal/prompting/task/hypothesis.go, and Kind/Record in task.go) — this
phase only reads that schema, it does not change it.

Scope: a new internal/runtime/tidal package containing a pure
`Render(g *task.Graph) string` function producing the five-section template
(PROBLEM, RESOLUTION CRITERIA, HYPOTHESES, KNOWN, NEED TO KNOW) over
`task.Graph.Nodes()`, grouped by Kind/Status/Deferred exactly as ADR 0014 §1
specifies:
- PROBLEM and RESOLUTION CRITERIA come from the graph's single root record.
- HYPOTHESES from every Kind == KindHypothesis record, with each Evidence
  entry resolved against its referenced NodeID.
- KNOWN from every Done KindTask record.
- NEED TO KNOW from every still-open record, split into "diagnostic" and
  "deferred" by the Deferred field.

One design decision ADR 0014 explicitly leaves open for this phase to settle,
not before: how Impossible-likelihood hypotheses are treated in the rendered
HYPOTHESES section (collapsed to a one-line note, or omitted from the render
entirely and kept only in the stored graph for audit). Decide it, write it into
the behavior doc's scenarios as a concrete GIVEN/WHEN/THEN, and implement
exactly that — do not leave it ambiguous or implicit in the code.

No LLM call anywhere in this phase — Render is pure and unit-testable with
fixed graph fixtures and exact expected output strings, no stub Chat function
needed. Do not implement anything beyond what your own behavior doc's
scenarios require, and do not anticipate later phases (the Tier 1
continue_investigating wrapper, Tier 2 operations, or ConsolidatorHook) — those
are separate phases 3-5, each gets its own behavior doc when it starts.

make all must pass when you're done. Use the run_checks tool to verify this
yourself before reporting the phase complete — don't just tell the user to run
it themselves.
