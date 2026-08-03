# Behavior — Tool-Call Iteration Limit Becomes a Continue/Stop Approval

Status: **Implemented** (2026-08-03).

## Problem

`ConversationCore.RunPrompt` bounds each turn to `maxToolIterations()`
(`Settings.MaxToolIterationsPerTurn`, default 25) native tool-call
round-trips — a runaway guard the old one-tool-per-turn cycle never needed.
Previously, hitting that budget without a final text answer was a hard
stop: the loop ended unconditionally and published a fixed
`"[stopped: reached the tool-call limit for this turn without a final
answer]"` response. Reported from a real session doing many individual
`sed`/`write_file` calls: this is genuinely limiting for legitimate
multi-step work (a large refactor, a long test-writing pass) that just
needs more than 25 round-trips, with no way to say "keep going" short of
re-prompting from scratch. Session `naive-stunning-eagle` additionally
surfaced this at the worst possible moment — layered under the (now-fixed)
approval-panel/scrollback overflow bugs, hitting the hard stop left no
visible way to resume at all.

## Design

Reuse the exact `RequestDecision` seam every other interactive decision in
this codebase already funnels through (tool approval, verb continuation,
output-size recovery) — a fourth decision kind, not a new mechanism:

```go
var toolLimitOptions = []state.ApprovalOption{
    {Label: "Continue working", Decision: "continue"},
    {Label: "Stop here", Decision: "stop"},
}

func (o *Orchestrator) RequestToolLimitApproval(ctx context.Context, used int) (bool, error) {
    prompt := fmt.Sprintf("Reached %d tool-call round-trips this turn without a final answer. Continue working?", used)
    dec, err := o.RequestDecision(ctx, state.PhaseIterationLimit, prompt, toolLimitOptions)
    if err != nil {
        return false, err
    }
    return dec == "continue", nil
}
```

Not scoped (session/global) the way tool approval is — "keep working a
while longer this turn" isn't a standing permission worth remembering, just
a one-off decision about how deep THIS turn goes. A new `state.Phase`
(`PhaseIterationLimit`) distinguishes it in the processing-state stream from
tool/verb/output-size decisions, though the chat surface's approval widget
is already fully generic (prompt + options, no Phase-specific rendering),
so no surface changes are needed to display it.

**`ApprovalSeeker` gains a third method** (`RequestToolLimitApproval`),
alongside `RequestApproval`/`RequestOutputSizeDecision` — `ConversationCore`
reaches it the same way it reaches the other two, through the same injected
interface.

**`RunPrompt`'s loop** changes from a bounded `for i := 0; i < maxIter;
i++` to an unbounded loop with an explicit, resettable counter:

```go
iter := 0        // resets to 0 on every "continue" decision
totalIter := 0   // never resets — the running total shown in the prompt
firstCall := true
declinedToContinue := false

for {
    if iter >= maxIter {
        cont, cerr := c.approvals.RequestToolLimitApproval(ctx, totalIter)
        if cerr != nil {
            c.report(state.StateCompleted, state.PhaseNone)
            return RunOutcome{Interrupted: true}, nil
        }
        if !cont {
            declinedToContinue = true
            break
        }
        iter = 0
    }
    iter++
    totalIter++

    result, respOrd, err = c.streamResponse(ctx, turn.Messages, fallback, toolSchemas, doThink && firstCall, opts.Ephemeral)
    firstCall = false
    // ... unchanged: tool-call detection, execution, hook points ...
}
```

Two counters, deliberately: `iter` is what actually gates the ask (reset to
0 on continue, so the SAME budget applies to every window); `totalIter` is
purely for the prompt's "reached N round-trips" text, so a turn extended
several times reports its true cumulative depth rather than repeating the
same number every time. `firstCall` (not `iter`) gates thinking — a
continuation reset must never re-trigger reasoning, which is meant to
happen exactly once, at the very start of the turn, regardless of how many
times the turn is later extended.

An interrupt while `RequestToolLimitApproval` is pending (ctx canceled) ends
the cycle the same way an interrupted tool approval already does:
`Interrupted: true`, nil error — no new interrupt-handling path, the
existing one generalizes. A decline still produces a non-empty final
response (never silence), just with updated wording distinguishing an
explicit decision from the old unconditional stop:
`"[stopped: declined to continue past the tool-call limit for this turn]"`.

```
GIVEN the model keeps issuing tool calls past maxToolIterations, never
      answering with plain text, and the user declines to continue when
      asked
WHEN  RunPrompt's loop exhausts the budget
THEN  RequestToolLimitApproval is asked exactly once (with the exhausted
      count), then the turn ends with the updated "[stopped: declined...]"
      text — never silence.

GIVEN the model hits the budget, the user approves continuing, and the
      model keeps issuing tool calls a while longer before finally
      answering with plain text
WHEN  RunPrompt runs
THEN  the per-window counter resets and the model makes MORE round-trips
      than maxToolIterations' original budget alone would have allowed —
      proving the reset actually happened, not just that the decision was
      consulted.

GIVEN the model hits the budget and RequestToolLimitApproval is interrupted
      (ctx canceled while the decision is pending)
WHEN  RunPrompt runs
THEN  it ends the cycle cleanly (Interrupted=true, empty Response, nil
      error) — the same posture as an interrupted tool approval.

GIVEN a turn that never hits the budget at all (a normal turn)
WHEN  RunPrompt runs
THEN  behavior is unchanged from before this fix — RequestToolLimitApproval
      is never consulted, and thinking still happens exactly once, on the
      turn's first model call.
```

## Tests

- `internal/runtime/core_loop_test.go` (extended):
  `TestConversationCoreRunPromptBudgetExhaustedDeclined` (renamed/updated
  from the old hard-stop test — decline path, updated message, asserts
  `RequestToolLimitApproval` was called once with the exhausted count),
  `TestConversationCoreRunPromptContinuesPastBudgetOnApproval` (continue
  path — proves the reset lets the turn exceed the original budget),
  `TestConversationCoreRunPromptInterruptedWhileAwaitingContinuation`
  (interrupt path).
- `internal/runtime/core_tools_test.go` (extended): `stubApprovalSeeker`
  gains `RequestToolLimitApproval` plus scriptable
  `continueOnLimit`/`continueErr` fields and call-tracking, matching its
  existing `RequestApproval` pattern.
- `internal/state/processing.go`: `PhaseIterationLimit` added to
  `validPhases`.
- Full existing suite / `make all` passes unchanged.
