# Behavior — ContextStore's Orchestrator-Side Implementation Consolidated (ADR 0013 Phase 4)

Status: **Implemented** (2026-08-01). Realizes ADR 0013's Phased Build Plan step 4.

## This phase's real scope, found by checking before moving anything

The ADR's Phased Build Plan named this phase "move `withContext`/`workingMemoryMessage`/
`historyMessages`/`recordTurn` into the Orchestrator-side `ContextStore`
implementation." Checking what that actually requires, ahead of touching anything,
found two things worth stating plainly rather than assuming:

**1. The behavioral goal was already met by Phases 1 and 2.** `Augment`/`Record`
(`ContextStore`'s `Orchestrator`-side implementation) have been pure pass-throughs to
`withContext`/`recordTurn` since Phase 1, and `ConversationCore.RunPrompt` has called
`c.convo.Augment`/`c.convo.Record` — never `o.withContext`/`o.recordTurn` directly —
since Phase 2. There is no remaining behavioral gap for this phase to close; "into the
... implementation" was already true in substance.

**2. These five functions must stay `Orchestrator` methods — moving them onto
`ConversationCore` itself would be architecturally wrong, not just unnecessary.**
`recordTurn`/`historyMessages` read and write `o.history` under `o.mu`; `withContext`/
`workingMemoryMessage`/`workingMemoryFacts` read `o.store`. Both `o.mu` and `o.store`
are named explicitly in ADR 0013 §"What stays on `Orchestrator`, unconditionally."
Giving `ConversationCore` direct access to either would be the exact anti-pattern the
whole ADR exists to eliminate — Core is supposed to reach session state only through
the three interfaces, never by reaching back into `Orchestrator`'s private fields.
So "moved into the ContextStore implementation" correctly meant (and was already
correctly designed to mean, in the original ADR text) *co-located with Augment/Record
as Orchestrator methods*, not *relocated onto ConversationCore*.

Given both of those, this phase's genuine remaining work was: (a) confirm there's no
latent correctness gap around `o.history`'s locking discipline before relocating
anything near it, and (b) do the file-organization move for cohesion — the same
treatment Phase 3 gave `runNativeToolCall`/`streamResponse` (`core_tools.go`/
`core_respond.go`).

## Due diligence: `o.history`'s locking discipline, checked before moving

Every touch point was checked directly, not assumed:

- `recordTurn` (write) and `historyMessages` (read) both take `o.mu.Lock()`.
- `ContextBreakdown` (`orchestrator.go`, context-visualizer surface) copies
  `o.history` under `o.mu.Lock()` before iterating — deliberately reading the raw
  slice instead of `historyMessages()` (a pinned "tool" entry must land in its own
  "tools" byte-count band, not get folded into "user" the way `historyMessages`
  folds it for the model).
- `SetEventEnabled` mutates `o.history[i].enabled` under `o.mu.Lock()`.
- `historyEvents` (`classifier_pipeline.go`) is documented "Caller holds o.mu" and
  is only ever called from sites that do.

No gap found — `o.history` is consistently guarded everywhere it's touched. Nothing
needed fixing here, unlike Phase 3's live-reload snapshot bug; this is recorded as a
verified-clean finding, not a silent assumption.

## What moved, and what deliberately did not

**Moved to `internal/runtime/core_context.go` (new):** the `turnMsg` type,
`recordTurn`, `historyMessages`, `withContext`, `mergeSystemMessages`,
`workingMemoryMessage`, `workingMemoryFacts`, `pinAnnotatedValue`, and `Augment`/
`Record` (moved from `core.go`, where Phase 1 first placed them, to sit next to what
they wrap). All as `Orchestrator` methods — see "must stay" above.

**Did not move, deliberately:** the `history []turnMsg` field itself stays declared in
`Orchestrator`'s struct literal in `orchestrator.go` (Go doesn't require a field's type
to be declared in the same file as the struct; moving `turnMsg` next to its real users
is fine). `continuation.go:117-118` and `classifier_pipeline.go:436,438` still call
`o.withContext`/`o.recordTurn` directly by those names, unchanged — same treatment as
Phase 3's `streamResponse`/`finishCycle`: these two call sites are outside this ADR's
scope to touch, and the method names didn't change, only their file.

```
GIVEN Augment(base) is called on Orchestrator
WHEN  it delegates to withContext(base)
THEN  the result is identical to calling withContext directly — unchanged from
      Phase 1's original guarantee, now just implemented in a different file.

GIVEN recordTurn is called (directly, or via Record)
WHEN  o.history is mutated
THEN  the mutation happens under o.mu, exactly as before relocation — verified by
      the due-diligence pass above, not just assumed from the move being "just a
      file change."

GIVEN continuation.go and classifier_pipeline.go's direct calls to
      o.withContext/o.recordTurn
WHEN  this phase lands
THEN  neither call site changes — same method names, same behavior, only the
      method bodies' file location differs.
```

## Tests

- No new tests: this phase is a pure relocation with no behavior change, and the
  existing Phase 1 tests (`TestOrchestratorAugmentDelegatesToWithContext`,
  `TestOrchestratorRecordDelegatesToRecordTurn`, `internal/runtime/core_test.go`)
  already assert exactly the equivalence this phase must preserve — they now
  exercise code that lives in `core_context.go` instead of `orchestrator.go`,
  unchanged otherwise, and pass unchanged.
- Full existing suite (`go test ./...`, `-race` on `internal/runtime/...`) and
  `make all` (vet + full suite + build) pass unchanged.
