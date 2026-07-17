# Behavior — Reasoning-Effort Control for `Complete` (ADR 0012 Phase 1)

Status: **Implemented** (2026-07-17). Realizes ADR 0012 §7 ("Reasoning effort,
constrained for fast rounds") and Phased Build Plan step 1. No wavefront dependency —
proven out against the existing decomposition planner's `Complete` call site
(`internal/runtime/classifier_pipeline.go`'s `buildDecomposition`).

Built exactly as scoped below. `completeWithThinkingBudget` (`classifier_pipeline.go`)
is package-level, not a method, specifically so a later wavefront classifier can reuse
it against its own budget setting. Tests: `internal/llm/ollama/ollama_test.go`
(payload wiring), `internal/runtime/classifier_pipeline_test.go` (all four scenarios
below). One test-hygiene lesson worth recording: the first version of the
timeout/retry test blocked its fake server handler on `<-r.Context().Done()` to
simulate a client giving up — over loopback, client-side context cancellation does
not reliably/promptly close the server-side connection, so `httptest.Server.Close()`
hung waiting for that handler goroutine. Fixed by having the handler sleep a bounded
duration comfortably longer than the budget instead of blocking on a signal that
might never arrive.

## Problem

`ollama.CompleteRequest` (`internal/llm/ollama/ollama.go:149-156`) has no `Think`
field — only `Temperature`, `Seed`, `Format`, `NumCtx`. Every `Complete`-based call
(the decomposition planner, the classifier fan-out) gets whatever reasoning behavior
Ollama defaults to for the configured model, unmeasured and uncontrollable from this
codebase. The streaming `Chat` path already has both halves of this problem solved —
`ChatRequest.Think bool` (`ollama.go:61`) plus an orchestrator-level budget/fallback
dance in `internal/runtime/tool_cycle.go:175-194` (a `time.AfterFunc` cancels a child
context if no content has started streaming by the budget's expiry; the caller then
retries once on the parent context with thinking off) — but neither half exists for
`Complete`. ADR 0012 cites a controlled, independent measurement (totAlX's README)
showing a >25x wall-time difference between reasoning-enabled and reasoning-disabled
calls, prompt size and server load held constant — the single highest-leverage lever
available for the planner's (and, later, wavefront's) round-trip-heavy latency.

## Design

### 1. `CompleteRequest.Think bool` — mechanical, mirrors `ChatRequest.Think` exactly

`Complete` sends `payload["think"] = true` as a **top-level** payload key when set —
not nested under `options` — matching `Chat`'s existing wire convention
(`ollama.go:84-86`) and Ollama's actual `/api/chat` contract. No other request shape
changes.

### 2. Budget enforcement lives at the orchestrator layer, not inside `ollama.Client`

This mirrors the existing precedent exactly: `Client.Chat` itself has no budget
parameter at all — the timer/cancel/retry logic lives entirely in the orchestrator
(`tool_cycle.go`), not the low-level HTTP client. `Client.Complete` stays a thin,
mechanical wrapper; the budget-then-retry-without-thinking policy is added to
`buildDecomposition`'s `chat` closure (`classifier_pipeline.go:160-169`), the single
call site `LLMPlanner.Chat` uses today.

New config surface, added to the *existing* `[agentx.thinking]` table rather than a
new table — this is planner-specific reasoning control, but it is still "reasoning
control," and keeping it alongside `TimeBudgetSeconds`/`Routes` avoids fragmenting
one concept across two TOML tables:

```go
// config.Thinking gains:
PlannerTimeBudgetSeconds int `toml:"planner_time_budget_seconds"`
```

```go
// Config.PlannerThinkingBudgetSeconds bounds the decomposition planner's own
// Complete-based reasoning phase (ADR 0012 Phase 1), independent of the respond
// path's ThinkingTimeBudgetSeconds. <= 0 (the default — deliberately unset, not
// defaulted to a positive number) disables thinking for planner Complete calls
// entirely: existing behavior is byte-identical unless explicitly configured.
func (c Config) PlannerThinkingBudgetSeconds() int {
    return c.Agentx.Thinking.PlannerTimeBudgetSeconds
}
```

`runtime.Settings` gains `PlannerThinkingBudget time.Duration`, wired in
`internal/app/app.go` the same way `ThinkingBudget` already is
(`time.Duration(cfg.PlannerThinkingBudgetSeconds()) * time.Second`).

### 3. The budget-then-retry wrapper

```
GIVEN Settings.PlannerThinkingBudget <= 0 (the default)
WHEN  the planner calls Complete
THEN  Think is never set; the request and its outcome are byte-identical to today.

GIVEN Settings.PlannerThinkingBudget > 0
WHEN  the planner's Complete call (Think: true) finishes before the budget elapses
THEN  its result is returned as-is; no retry occurs.

GIVEN Settings.PlannerThinkingBudget > 0
WHEN  the planner's Complete call has not returned when the budget elapses,
      AND the parent (caller-supplied) context is still live
THEN  the call is retried exactly once, on the parent context, with Think forced
      false, and that retry's result (or error) is what the caller receives.

GIVEN Settings.PlannerThinkingBudget > 0
WHEN  the parent context itself is cancelled (not just the budget context)
THEN  no retry is attempted — the cancellation propagates, mirroring the existing
      ctx.Err() == nil guard in tool_cycle.go's fallback branch.
```

Unlike the streaming `Chat` path, `Complete` has no partial content to preserve — a
budget timeout here always means "the whole call gets restarted without thinking,"
never "cut thinking short and keep the same generation." This is a deliberate,
narrower adaptation of the existing pattern for a non-streaming call, not an
oversight.

## Implementation shape

- `internal/llm/ollama/ollama.go` — `CompleteRequest.Think bool`; `Complete` sets
  `payload["think"] = true` when set. No budget/retry logic in this file — that stays
  at the orchestrator layer per §2.
- `internal/config/config.go` — `Thinking.PlannerTimeBudgetSeconds`;
  `Config.PlannerThinkingBudgetSeconds()`.
- `internal/runtime/orchestrator.go` — `Settings.PlannerThinkingBudget time.Duration`.
- `internal/app/app.go` — wire `cfg.PlannerThinkingBudgetSeconds()` into the new
  `Settings` field, same pattern as `ThinkingBudget`.
- `internal/runtime/classifier_pipeline.go` — `buildDecomposition`'s `chat` closure
  gains the budget-then-retry wrapper (§3), reusable as-is by wavefront's own
  `LLMClassifier` construction in a later phase (ADR 0012 §7 names a *separate*
  `WavefrontThinkingBudget` setting for that — this phase does not add it yet, since
  nothing consumes it before wavefront exists).

**Explicitly out of scope for this phase:** the classifier fan-out
(`internal/llm/fanout`) is a second existing `Complete` consumer ADR 0012 §7
mentions in passing ("existing planner/classifier `Complete` call sites"), but this
phase only wires the planner's site. The fan-out's voting mechanics are a separate,
more involved call shape; extending budget control to it is deferred as a distinct,
explicitly scoped-out follow-up rather than bundled in here.

## Tests

- `internal/llm/ollama/ollama_test.go` — `Think: true` produces a top-level
  `"think": true` in the request payload (not nested under `options`); `Think: false`
  (zero value) omits the key entirely, matching `Chat`'s existing test coverage shape.
- `internal/runtime/classifier_pipeline_test.go` (new or extended) — the
  budget-then-retry wrapper against a fake `Complete` that can be told to simulate
  "exceeds budget" (blocks past the deadline) vs. "returns in time": asserts exactly
  one retry, that the retry carries `Think: false`, and that a genuinely cancelled
  parent context short-circuits the retry per the fourth scenario above.

## Open questions

1. Should the fan-out's `Complete` call sites get the same `Think`+budget treatment,
   and on what timeline? Deferred per "Explicitly out of scope" above — revisit once
   this phase's wrapper has real usage to generalize from.
