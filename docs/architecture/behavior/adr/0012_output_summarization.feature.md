# Behavior — Chain-Aware Output Summarization Replaces Truncation (ADR 0012 Phase 3)

Status: **Implemented** (2026-07-17). Realizes ADR 0012 §6 and Phased Build Plan
step 3. Independent of wavefront — lives in `capturingExec.Execute`
(`internal/runtime/plan_cycle.go`), which both the existing continuous scheduler and
any future wavefront engine drive through, so this benefits whichever is active.

Built exactly as scoped below, reusing Phase 2's `wavefront` prompt scaffolding as
its first real consumer (`newOutputSummarizer`, wired in `buildDecomposition`
alongside the planner, off the same `ollama.Client`/model). Tests:
`internal/runtime/plan_cycle_test.go` (extended) cover all four scenarios in §5 plus
the chain-collapse case (§2) and `truncateFindings`'s UTF-8 safety directly; full
suite and `-race` both clean.

## Problem

`capturingExec.Execute` currently bounds a captured step's real findings by reading at
most `findingsLines` (200) lines from the artifact store, with no further protection
against a single line — or the 200-line window itself — being large. The mid-drain
"live findings" view (`Findings()`) additionally truncates by `firstLines` to
`midDrainFindingsLines` (20) lines. Both are positional-arbitrary prefix truncations:
the file's own comments already name the failure this causes — the `lively-raven`
incident, where `tree`'s real depth-3 structure was discarded down to its first ~20
alphabetical entries before synthesis ever saw it, and the model concluded
"Python-based" from what survived (a stray `agentx.egg-info` and a failed guessed
path made it in; `go.mod`/`cmd/`/`internal/` did not).

totAlX's own investigation hit the same failure class at a larger scale (a
~47,000-character tool result caused total response-format collapse, reproducing on
every run) and found a working fix: below a size threshold, use raw output as-is
(the common case); above it, run the output through a dedicated, chain-aware,
schema-free summarization call — targeted at the specific question that produced the
output, not the top-level goal — and fall back to plain truncation only if
summarization itself fails. In testing this reliably condensed the ~47,000-character
case to roughly 1,200-1,300 characters while preserving the actually relevant
content.

## Design

### 1. A character threshold, not a line count, decides summarize vs. use-as-is

```go
const outputSummaryThreshold = 2000   // chars
const outputSummaryTargetChars = 1200 // requested size, guidance not a hard cap
```

Deliberately a character count: a single very long line (a minified file, a raw JSON
blob) can blow a character budget while still being "one line" under `findingsLines`'
200-line window — the two caps address different failure shapes and both stay in
place. `outputSummaryThreshold`/`outputSummaryTargetChars` are illustrative starting
points from totAlX's own measurement, not tuned agentX constants — expect to revisit
once this has real usage.

### 2. Chain-aware summarization, targeted at the most specific item

The chain handed to the summarizer is `[rootGoal, stepGoal]` (collapsed to just
`[rootGoal]` when the step *is* the root — the plan's very first, still-atomic
question). This is a deliberately smaller chain than totAlX's own full ancestry walk
(root → every intermediate Step → the immediate NEED) — `capturingExec` does not
currently have access to the task graph's parent/child structure, only the flat list
of steps it has captured, so a full multi-level chain would need new plumbing beyond
this phase's scope. Two levels (general, specific) already carries the core insight —
don't summarize toward the top-level question, target the specific one, use the
general one only as relevance context — and is revisited as an open question below if
a deeper chain proves necessary in practice.

### 3. Summarization reuses Phase 2's prompt scaffolding directly

`wavefront.DefaultSummaryPromptTemplate`/`DefaultSummaryUserTemplate` and
`RenderSummarySystem`/`RenderSummaryUser` (built in Phase 2 as engine-agnostic
scaffolding) are the summarizer's prompt, with `Settings.WavefrontSummaryPrompt`
overriding the system part the same way `PlannerPrompt` does for the planner. This is
the first real consumer of that scaffolding — Phase 2 was scoped exactly so this
phase would not need to duplicate prompt content.

### 4. No reasoning for this call

The summarization call runs with `Think` unset (false) — no budget wrapper needed.
This is schema-free plain prose with nothing structured to break, which is exactly
the case ADR 0012 §7 (citing totAlX's own finding) says is safe to run without
thinking: the speed cost of reasoning has no corresponding format-safety benefit here,
unlike a JSON-schema-constrained call.

### 5. Truncation survives as the last-resort fallback only

```
GIVEN a step's real findings are <= outputSummaryThreshold characters
WHEN  Execute captures them
THEN  they are stored as-is; no summarization call is made.

GIVEN a step's real findings exceed outputSummaryThreshold characters,
      AND a summarizer is wired
WHEN  Execute captures them
THEN  a chain-aware summarization call runs (chain = [rootGoal, stepGoal], collapsed
      to [rootGoal] when the step is the root) and the condensed result — prefixed
      "[summarized from N chars]" so the provenance is never silently hidden — is
      stored instead of the raw text.

GIVEN a step's real findings exceed the threshold,
      AND either no summarizer is wired OR the summarization call fails
WHEN  Execute captures them
THEN  a plain, rune-safe prefix truncation (to outputSummaryTargetChars) is stored,
      clearly marked "[truncated, N more chars]" — never silently indistinguishable
      from a summary.
```

### 6. Scoped out: the mid-drain `Findings()` cap is untouched

`Findings()`'s `firstLines(f, midDrainFindingsLines)` (20 lines) addresses a
different concern — bounding the *cumulative* prompt size across every step's
findings fed back into later decompose/proposal calls during the same drain, not one
step's raw-output size. Once summarization runs at capture time, most findings never
approach that cap in the first place, but the cap itself stays as-is; changing it is
out of scope here.

## Implementation shape

- `internal/runtime/plan_cycle.go` — `outputSummaryThreshold`,
  `outputSummaryTargetChars`, `summarizeFunc` type, `capturingExec.rootGoal` and
  `capturingExec.summarize` fields, `capturingExec.condense`/`truncateFindings`
  helpers, `Execute` calling `condense` when the threshold is exceeded.
- `internal/runtime/plan_cycle.go` (`runPlanPhase`) or `classifier_pipeline.go` —
  the summarizer closure construction (an `ollama.Client.Complete` call, `Think:
  false`, rendered via `wavefront.RenderSummarySystem`/`RenderSummaryUser`), wired
  into `capturingExec` at construction alongside `rootGoal: root.Goal`.

## Tests

- `internal/runtime/plan_cycle_test.go` (new or extended) — `capturingExec` with a
  fake `summarize` function: below-threshold findings are stored verbatim with no
  summarizer call; above-threshold findings trigger exactly one summarizer call with
  the expected chain and are stored with the `"[summarized from"` prefix; a
  summarizer returning an error (or a nil summarizer) falls back to
  `truncateFindings`, marked `"[truncated,"`; `truncateFindings` itself is rune-safe
  against multi-byte input (a regression guard, since a naive byte-index slice can
  split a UTF-8 rune at the boundary).

## Open questions

1. **Deeper ancestry chain.** If a two-level `[rootGoal, stepGoal]` chain proves too
   coarse in practice (a deeply nested Step's summary needs its intermediate
   ancestors' context, not just the root), `capturingExec` needs access to the task
   graph's parent chain, not just its own flat captured-steps list — deferred until
   there's a concrete case motivating it.
2. **Threshold/target tuning.** `2000`/`1200` are ported from totAlX's own
   measurement on a different model/workload; revisit once this has real agentX
   usage to tune against.
