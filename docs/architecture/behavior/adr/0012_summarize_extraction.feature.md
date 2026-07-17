# Behavior — Summarization Mechanics Move Into `wavefront` (ADR 0012 Phase 7a)

Status: **Implemented** (2026-07-17). Realizes ADR 0012's same-day addendum finding
#3 and Phased Build Plan (amendment) step 7a. Behavior-preserving refactor, not a
new capability — a prerequisite for Phase 7 (`wavefront.Scheduler` needs this logic
for command-Need results and cannot reach it where it lived).

## Problem

`internal/runtime/plan_cycle.go` has imported `internal/runtime/wavefront` since
Phase 3 (for the Phase 2 prompt scaffolding). Go forbids the reverse import, so
`wavefront.Scheduler`'s future command-execution path cannot call into
`plan_cycle.go`'s `outputSummaryThreshold`/`condense`/`truncateFindings` as they
stood — not eventually, today. Separately, `Value`/`Error` now living directly on
`task.Record` (Phases 4-5) means wavefront never needed `capturingExec`'s parallel
bookkeeping in the first place. The fix is moving the mechanics into `wavefront`
itself, co-located with the prompt templates they already render.

## Design

### 1. What moves, what stays

Moves from `internal/runtime/plan_cycle.go` into `internal/runtime/wavefront/summarize.go`:

- `outputSummaryThreshold` → `wavefront.OutputSummaryThreshold` (exported)
- `outputSummaryTargetChars` → `wavefront.OutputSummaryTargetChars` (exported)
- `truncateFindings` → `wavefront.TruncateFindings` (exported, unchanged logic)
- `renderChain` → unexported `wavefront.renderChain` (internal helper, no external
  caller needs it directly)
- `newOutputSummarizer`'s prompt-rendering + threshold logic → `wavefront.NewCondenser`

Stays in `plan_cycle.go`, because it's genuinely about the continuous engine's own
bookkeeping, not summarization mechanics: `capturingExec.condense`'s chain-building
(`[]string{c.rootGoal}`, collapsed when the step *is* the root) — this is specific
to how the continuous engine tracks its plan's root/step relationship via
`capturingExec.rootGoal`, not something `wavefront` needs to know about.

### 2. `CondenseFunc`'s signature drops the error return

```go
// Before (plan_cycle.go): type summarizeFunc func(ctx, chain, text) (string, error)
// After  (wavefront):      type CondenseFunc  func(ctx, chain, text) string
```

`NewCondenser`'s returned `CondenseFunc` now handles its own chat-error and
empty-response fallback internally (falling back to `TruncateFindings`), rather than
surfacing an error for the caller to handle. This mirrors `capturingExec.condense`'s
actual pre-existing behavior exactly (it already did `if err == nil { ... } return
truncateFindings(text)` — the error was never propagated past `condense` anyway) —
collapsing that dead code path rather than preserving it as a no-op parameter.

```
GIVEN capturingExec.summarize is nil
WHEN  Execute condenses oversized findings
THEN  it falls back to wavefront.TruncateFindings directly — this is capturingExec's
      own responsibility, unchanged from before the move.

GIVEN capturingExec.summarize is a wavefront.CondenseFunc
WHEN  Execute condenses oversized findings
THEN  capturingExec builds the chain and calls it, storing whatever comes back
      verbatim — it never sees or handles a chat failure itself anymore, since
      CondenseFunc always returns usable (summarized or truncated) text.

GIVEN a chat call fails or returns only whitespace
WHEN  wavefront.NewCondenser's CondenseFunc runs
THEN  it falls back to TruncateFindings — this fallback now lives and is tested at
      the wavefront level, not the capturingExec level.
```

### 3. `classifier_pipeline.go`'s construction becomes a thin adapter

`newOutputSummarizer` (a `plan_cycle.go`-local function building a full `Complete`-backed
closure) is replaced by a small `summarizeChat` adapter (matching `wavefront.Chat`'s
signature) plus a call to `wavefront.NewCondenser(summarizeChat, template)` — the
same pattern already used for the planner's own `chat` closure a few lines above,
minus the thinking-budget wrapper (summarization still runs with `Think` always
unset, unchanged from before the move).

## Tests

- `internal/runtime/wavefront/summarize_test.go` (new) — `NewCondenser`'s five
  scenarios (successful chat, chat error, empty chat response, default template,
  custom template override) plus `TruncateFindings`'s rune-safety and
  below-target-unchanged cases, moved from `plan_cycle_test.go` where they
  previously tested the same logic through `capturingExec`.
- `internal/runtime/plan_cycle_test.go` (trimmed) — `capturingExec`'s own remaining
  responsibilities: skip-below-threshold, chain-building (general case and the
  root-collapse case), and the nil-summarizer fallback. The chat-error-fallback
  test that used to live here moved to `wavefront` per §2, since `capturingExec` no
  longer has that code path to test.
- Full suite and `-race` on `runtime`/`decompose`/`scheduler`/`wavefront` all clean
  — confirms this stayed behavior-preserving.
