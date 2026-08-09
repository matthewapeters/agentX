Implement Phase 3 of ADR 0014 (Tidal): the `continue_investigating` native-tool
wrapper around `wavefront.Scheduler`.

No behavior doc exists yet for this phase — ADR 0014's Phased Build Plan is
explicit that each phase's GIVEN/WHEN/THEN behavior doc is written immediately
before that phase starts, not up front. Write
docs/architecture/behavior/adr/0014_tidal_continue_investigating.feature.md
first, following that same repo convention and the same level of rigor as
Phase 1's doc (docs/architecture/behavior/adr/0014_tidal_schema.feature.md),
then implement against it.

Read first:
- docs/architecture/adr/0014-tidal-hypothesis-grounded-consolidation.md's
  Phased Build Plan steps 3–5 (Tier 1 wrapper + Tier 2 stall gate +
  ConsolidatorHook wire).
- internal/runtime/wavefront/scheduler.go — `wavefront.Scheduler`, its `Run()`,
  and its `ErrStalled` / `ErrCancelled` signals.
- internal/prompting/task/types.go and hypothesis.go — the schema this wrapper
  consumes (Status, NodesResolved, etc. must align with existing Record/Graph
  types).

Scope: a new `tidal.Wrapper` in `internal/runtime/tidal` that wraps an already-
configured `*wavefront.Scheduler` and exposes a zero-arg `Run(ctx context.Context) Status` method.
The wrapper translates `scheduler.Run()`'s return into a small structured
`Status` (NodesResolved int, Status RunStatus, Error string) and is the single
Tier 1 entry point the model-facing tool call will invoke. No new scheduler
methods. No Tier 2 operations. No hook wiring. No conversation-core
registration (that's phase 6).

One design decision ADR 0014 explicitly leaves open for this phase to settle,
not before: how the `Status` struct's fields are shaped and which scheduler
error maps to which `RunStatus` value (done / stalled / error). Decide it, write
it into the behavior doc's scenarios as concrete GIVEN/WHEN/THEN, and implement
exactly that — do not leave it ambiguous or implicit in the code.

No LLM call anywhere in this phase — `Wrapper.Run()` is a thin adapter around
`wavefront.Scheduler.Run()` with no inference logic. Unit-testable against stub
collaborators with fixed graph fixtures and exact expected `Status` fields, no
Chat stub needed.

Do not implement anything beyond what your own behavior doc's scenarios require
— no input-parameter shaping (phase 6's concern), no model-facing rendering of
`Status` (phase 6), no changes to `task.Graph` / `task.Record` / `task.Kind`,
no `ConsolidatorHook`, no `hooks.SyncHook`.

make all must pass when you're done. Use the run_checks tool to verify this
yourself before reporting the phase complete — don't just tell the user to run
it themselves.
