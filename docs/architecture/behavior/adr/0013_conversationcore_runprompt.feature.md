# Behavior — ConversationCore.RunPrompt: Extracting the Loop Body (ADR 0013 Phase 2)

Status: **Implemented** (2026-08-01). Realizes ADR 0013's Phased Build Plan step 2.
Highest-risk phase named in the ADR — this is where `RunOptions`/`RunOutcome`
settled, and `runPrompt`'s body (formerly `internal/runtime/loop.go:24-105`) moved
onto the new `ConversationCore.RunPrompt` (`internal/runtime/core_loop.go`).
Behavior-preserving for the single-session path: `Orchestrator.runPrompt` keeps its
exact external signature and error-return contract for its two existing callers
(`orchestrator.go:569,584`) — confirmed by the full pre-existing suite and `-race`
passing unchanged. `o.core` is built by the new `buildCore` (`core_loop.go`), called
from `Start()` right after `o.hooks` is built.

## Problem

Phase 1 (`0013_conversationcore_seams.feature.md`) added `ApprovalSeeker`,
`EventSink`, `ContextStore` and `Orchestrator`'s pass-through implementations, but
nothing calls them yet — `runPrompt` still reaches `o.bus`/`o.store`/`o.history`
directly. Phase 2 moves the loop's own skeleton (submit → hooks → stream/tool-call
round trip → hooks → loop) onto a new `ConversationCore` type, routing through the
Phase-1 interfaces instead of direct field access.

## Design

### Not every dependency `runPrompt` has today is ready to move in this phase

Five things `runPrompt` calls into are themselves still Orchestrator-only methods
(`runNativeToolCall`/`streamResponse`/`finishCycle`/`availableToolSchemas`/
`maxToolIterations`, plus `runToolOrPlan`'s plan_task-vs-generic dispatch) — moving
their *bodies* onto `ConversationCore` is Phase 3's job (already scoped separately
in ADR 0013 because it touches `o.gate`/`o.publishEv` call sites *inside* those
methods, a materially different, riskier change than moving the outer loop shape).
Phase 2 therefore injects them as plain closures on `ConversationCore`, bound to
the existing unchanged `Orchestrator` methods by the thin delegator built in this
phase. Four of these five closures (`streamFn`, `finishFn`, `toolSchemasFn`,
`maxIterFn`) are a deliberate, temporary stepping stone — Phase 3 replaces each
with a direct call to a method that by then lives natively on `ConversationCore`,
and these closure fields are removed.

The fifth, `execTool` (bound to `runToolOrPlan`), is **not** a temporary stepping
stone — it is permanent. ADR 0013 §"Explicitly not decided here" scopes
`runToolOrPlan`'s plan_task interception out of this ADR entirely (Open Question
4): whether a future planning tool becomes a `ConversationCore` consumer is
undecided, so `runToolOrPlan` — which depends on Orchestrator-only state
(`taskDecomp`, `wavefrontClassifier`, `planReady()`) never destined to move — stays
on `Orchestrator` forever under this design. `ConversationCore` depending on an
injected "run one model-issued tool call" function, rather than owning tool
dispatch outright, is therefore the ADR's real, permanent shape for this seam, not
an artifact of sequencing.

`refreshLiveFacts` (`orchestrator.go:923`) and the `o.mu`/`started`/`accepting`
readiness check are **not** moved at all, in this phase or a later one currently
scoped: both are session-lifecycle/session-store concerns (`refreshLiveFacts`
reads/writes live working-memory facts via `o.store` directly) that don't fit any
of the three Phase-1 interfaces as defined, and extending `ContextStore`'s
contract to cover fact-refreshing is a real design decision this ADR doesn't make.
They stay as pre-flight steps `Orchestrator.runPrompt`'s thin delegator performs
before calling `core.RunPrompt` — called out explicitly here so the omission reads
as a decision, not an oversight.

### Processing-state reporting gets a minimal, deliberately uncommitted closure

`runPrompt`'s own body (not a helper) calls `o.setProcessing` directly at two
points (hook-chain failure, and a user interrupt during tool approval). ADR 0013
Open Question 3 leaves "does processing-state reporting generalize to a nested
loop, or does a nested loop report through the plan-visibility `Observer` channel
instead" explicitly unscoped. Phase 2 cannot leave the session path's existing
spinner/phase UI broken while that question stays open, so it adds the smallest
possible placeholder: a nil-safe closure field, `reportState func(state.RunState,
state.Phase)`, called through a `report` helper that no-ops when unset. This is
deliberately a raw closure, not a fourth named interface — inventing
`ProcessingReporter` as real API surface is exactly the kind of premature
commitment Open Question 3 defers; a closure the session wires to `setProcessing`
and a future consumer can simply leave nil keeps the door open without design work
this phase doesn't need to do.

### `RunOptions`/`RunOutcome`

```go
type RunOptions struct {
    Text             string
    RecordUserPrompt bool
    Ephemeral        bool
}

type RunOutcome struct {
    Response    string // the final answer text, "" if the turn was interrupted
    Interrupted bool   // a user interrupt during tool approval ended the turn cleanly
}
```

`RunPrompt(ctx, opts) (RunOutcome, error)` mirrors `finishCycle`'s existing
nil-on-success-or-interrupt split: a hard error is the `error` return; an interrupt
is `RunOutcome{Interrupted: true}, nil`. `Orchestrator.runPrompt` keeps its current
`(ctx, text, recordUserPrompt, ephemeral) error` signature unchanged for its two
callers — it builds `RunOptions`, calls `o.core.RunPrompt`, and returns just the
error, discarding `RunOutcome` for now (nothing external consumes it yet; it
exists because Phase 2's job is to settle its shape, not because anything uses it
today).

```
GIVEN Orchestrator.runPrompt is called exactly as it is today (ctx, text,
      recordUserPrompt, ephemeral)
WHEN  it delegates to core.RunPrompt
THEN  the returned error is identical to what the pre-Phase-2 runPrompt body would
      have returned for the same inputs and the same mocked dependencies — same
      hook-error short-circuit, same interrupt-returns-nil behavior, same
      finishCycle mapping on a stream error or a full turn.

GIVEN a hook registered on ConversationCore's sync chain returns an error
WHEN  RunPrompt's first or a mid-loop RunSync call fails
THEN  RunPrompt returns that error unchanged, report(StateFailed, PhaseNone) is
      invoked exactly once (or not at all, if reportState is nil), and no further
      hooks/streaming/tool calls run for this turn.

GIVEN a tool call is interrupted while awaiting approval (execTool returns a
      non-nil error)
WHEN  RunPrompt handles that tool call
THEN  RunPrompt returns RunOutcome{Interrupted: true}, nil (not an error) and
      report(StateCompleted, PhaseNone) is invoked exactly once (or not at all, if
      reportState is nil) — matching the pre-Phase-2 body's "interrupted while
      awaiting approval: end the cycle cleanly" comment verbatim.

GIVEN the tool-iteration budget is exhausted without the model ever returning a
      plain-text answer
WHEN  RunPrompt's loop ends
THEN  RunOutcome.Response is the same "[stopped: reached the tool-call limit...]"
      fallback text the pre-Phase-2 body published, via events.Publish, not a
      direct bus call.

GIVEN refreshLiveFacts and the o.mu/started/accepting readiness check
WHEN  Phase 2 lands
THEN  neither moves onto ConversationCore — both remain steps
      Orchestrator.runPrompt's thin delegator performs before calling
      core.RunPrompt, unchanged in behavior from today.
```

## Tests

- `internal/runtime/core_loop_test.go` (new) — drives `ConversationCore.RunPrompt`
  directly against fake `EventSink`/`ContextStore` and stub
  `streamFn`/`execTool`/`finishFn`/`toolSchemasFn`/`maxIterFn` closures plus real
  `hooks.Registry` instances — no `Orchestrator`, no transport, no session store —
  covering all six scenarios above:
  `TestConversationCoreRunPromptChatResponse`,
  `TestConversationCoreRunPromptToolCallThenChatResponse`,
  `TestConversationCoreRunPromptHookFailureAtFirstPointAborts`,
  `TestConversationCoreRunPromptHookFailureMidLoopAborts`,
  `TestConversationCoreRunPromptInterruptDuringToolApproval`,
  `TestConversationCoreRunPromptBudgetExhausted`.
- No separate `Orchestrator.runPrompt`-level test was added: its body shrank to four
  mechanical lines (the readiness check, `refreshLiveFacts`, building `RunOptions`,
  delegating) with no branching logic of its own, so the `ConversationCore.RunPrompt`
  coverage above is where the real behavior lives. A full `Orchestrator`-level
  integration test (stubbed `Model`, live tool registry) remains a reasonable future
  addition but wasn't required to prove this phase correct — noted here rather than
  silently dropped from what was originally scoped.
- Full existing `internal/runtime` suite (`go test ./...`, `-race`), plus `make all`
  (vet + full suite + build), pass unchanged.
