# Behavior — ConversationCore Seams: ApprovalSeeker, EventSink, ContextStore (ADR 0013 Phase 1)

Status: **Implemented** (2026-08-01). Realizes ADR 0013's Phased Build Plan step 1:
define the three interfaces and `Orchestrator`-side implementations, wired but unused
by the existing loop. Zero behavior change to the single-session path — confirmed by
the full pre-existing suite (`go test ./...`, `-race` on `internal/runtime/...`)
passing unchanged. Built in `internal/runtime/core.go`;
`TestOrchestratorPublishDelegatesToPublishEv`/`Augment.../Record...` in
`internal/runtime/core_test.go` cover the three GIVEN/WHEN/THEN wrapper-equivalence
scenarios below (the interface-satisfaction scenario is proven by the `var _`
assertions in `core.go` failing the build otherwise — no separate runtime test
needed for it).

## Problem

`ConversationCore` (ADR 0013) cannot be extracted from `Orchestrator` until something
abstracts three things `runNativeToolCall`/`streamResponse`/`withContext`/`recordTurn`
currently reach for directly and concretely: the approval gate (`o.gate`), the event
bus (`o.bus`+`o.id`), and the session's context store (`o.store`+`o.history`). Phase 1
introduces that abstraction without moving any existing call site — the loop keeps
calling its current methods directly; only the new interfaces and `Orchestrator`'s
implementations of them are added, and nothing yet depends on them. This is the
"prove the seam compiles against real usage before anything depends on it" step named
in ADR 0013's Phased Build Plan.

## Design

Three interfaces, added to `internal/runtime/core.go`:

```go
type ApprovalSeeker interface {
    RequestApproval(ctx context.Context, d tools.Descriptor, args map[string]string, pol *tools.Policy) (tools.Verdict, error)
    RequestOutputSizeDecision(ctx context.Context, d tools.Descriptor, args map[string]string, res tools.Result) (tools.Result, bool, error)
}

type EventSink interface {
    Publish(eventType string, ct state.ContentType, payload any, ephemeral bool) uint64
}

type TurnRecord struct {
    Err      error
    Record   bool
    UserOrd  uint64
    UserText string
    RespOrd  uint64
    Response string
    Pins     []*toolPin
}

type ContextStore interface {
    Augment(base []prompting.Message) []prompting.Message
    Record(entry TurnRecord)
}
```

`ApprovalSeeker` needs no new method on `Orchestrator` — its existing
`RequestApproval` (`approval.go:39`) and `RequestOutputSizeDecision`
(`output_size.go:39`) already match the interface's method signatures exactly, so
`Orchestrator` satisfies `ApprovalSeeker` as-is. `EventSink` and `ContextStore` each
get one or two new, deliberately trivial wrapper methods on `Orchestrator` that
delegate to what already exists:

```go
func (o *Orchestrator) Publish(eventType string, ct state.ContentType, payload any, ephemeral bool) uint64 {
    return o.publishEv(eventType, ct, payload, ephemeral)
}

func (o *Orchestrator) Augment(base []prompting.Message) []prompting.Message {
    return o.withContext(base)
}

func (o *Orchestrator) Record(entry TurnRecord) {
    o.recordTurn(entry.Err, entry.Record, entry.UserOrd, entry.UserText, entry.RespOrd, entry.Response, entry.Pins)
}
```

Compile-time assertions pin all three:

```go
var _ ApprovalSeeker = (*Orchestrator)(nil)
var _ EventSink = (*Orchestrator)(nil)
var _ ContextStore = (*Orchestrator)(nil)
```

```
GIVEN the ApprovalSeeker, EventSink, and ContextStore interfaces defined in core.go
WHEN  the package is built
THEN  *Orchestrator satisfies all three (compile-time var _ assertions fail the build
      otherwise) — RequestApproval/RequestOutputSizeDecision satisfy ApprovalSeeker
      with no new code; Publish/Augment/Record are new thin wrappers.

GIVEN an Orchestrator wired with a bus, processing publisher, and identity
WHEN  Publish(eventType, ct, payload, ephemeral) is called
THEN  it returns the same ordinal and publishes the identical event to the bus that
      calling publishEv directly would — Publish is a pure pass-through, not new logic.

GIVEN an Orchestrator wired with a session store and no prior history
WHEN  Augment(base) is called with a message list
THEN  it returns exactly what withContext(base) would return directly — same message
      set, same ordering.

GIVEN an Orchestrator wired with a session store
WHEN  Record(entry) is called with a populated TurnRecord
THEN  o.history is mutated exactly as calling recordTurn(entry.Err, entry.Record,
      entry.UserOrd, entry.UserText, entry.RespOrd, entry.Response, entry.Pins)
      directly would — Record is a pure field-unpacking pass-through.

GIVEN the existing prompt loop (runPrompt, runNativeToolCall, streamResponse,
      withContext, recordTurn)
WHEN  Phase 1 lands
THEN  none of their call sites change — they keep calling o.gate/o.publishEv/
      o.withContext/o.recordTurn directly, not through the new interfaces. The full
      existing test suite passes unchanged, confirming zero observable behavior
      change in this phase.
```

## Tests

- `internal/runtime/core_test.go` (new) — `TestOrchestratorSatisfiesConversationCoreSeams`
  is a placeholder if the compile-time assertions alone don't need a runtime test;
  `TestOrchestratorPublishDelegatesToPublishEv`,
  `TestOrchestratorAugmentDelegatesToWithContext`, and
  `TestOrchestratorRecordDelegatesToRecordTurn` each directly compare the wrapper's
  result/side-effect against calling the underlying method directly with identical
  inputs.
- Full existing `internal/runtime` suite must pass unchanged — this phase adds code,
  it does not modify any existing call site.
