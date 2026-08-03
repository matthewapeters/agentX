# Behavior — Chat TUI: Pending-Approval Count on the Approval Panel Title

Status: **Implemented** (2026-08-02).

## Problem

Read-only mode's removal made "an approval blocks indefinitely with no human
present" the normal, accepted failure mode rather than an edge case (per the
product decision — see `docs/architecture/behavior/
tool_policy_read_only_removal.feature.md`). Today's approval panel
(`internal/surfaces/approval`, shown by `chat.Model` while
`state.StateAwaitingInput`) already displays the current request's
prompt+options under a fixed, attention-colored title
(`"AgentX Needs Your Input"`), and Phase A's clamp already guarantees it can't
scroll off-screen. What it does not communicate: `decisionGate`
(`internal/runtime/gate.go`) already serializes concurrent requests into a
FIFO queue and shows only the front — a user seeing one prompt has no way to
know whether it's the only pending decision or the first of several. That
matters specifically because a multi-step plan can generate several
approval-needing calls in sequence; discovering "3 more after this one" only
by resolving them one at a time is exactly the kind of surprise a persistent,
honest indicator should prevent.

## Design

`gate[Req, Resp]` gains a read-only length accessor (mutex-protected, matching
`enqueue`/`dequeue`/`deliver`'s existing locking discipline):

```go
// Len reports the current queue depth, including the front-of-queue request
// currently shown to the surface.
func (g *gate[Req, Resp]) Len() int {
    g.mu.Lock()
    defer g.mu.Unlock()
    return len(g.pending)
}
```

`publishApprovalPrompt` (`internal/runtime/decision.go`) gains a `pending int`
parameter, threaded from both existing call sites in `RequestDecision` via
`o.gate.Len()` (read immediately before publishing, so it reflects the queue
depth at the moment this specific prompt becomes the one shown):

```go
func (o *Orchestrator) publishApprovalPrompt(prompt string, options []state.ApprovalOption, pending int) {
    o.publish("APPROVAL_REQUEST", state.ContentApprovalRequest, map[string]any{
        "prompt": prompt, "options": options, "pending": pending,
    })
}
```

The chat surface reads `pending` off the existing `APPROVAL_REQUEST` event
payload (same dual-mode int-or-float64 coercion `state.DecodeApprovalOptions`
already demonstrates for the sibling `options` field — the payload may arrive
as a native Go value, in-process, or JSON-decoded, over the transport) and
builds the panel's title dynamically instead of using the fixed
`approvalTitle` constant: `"AgentX Needs Your Input"` when `pending <= 1`,
`"AgentX Needs Your Input (1 of N)"` when `pending > 1` — always position 1,
since the gate only ever shows the front of its FIFO queue.

```
GIVEN exactly one approval request is pending
WHEN  the panel renders
THEN  its title is the unchanged "AgentX Needs Your Input" — no count clutter
      for the common, single-pending case.

GIVEN three approval requests are enqueued in sequence, all before any is
      resolved (a plan proposing several write calls in a row)
WHEN  the first becomes front-of-queue and its APPROVAL_REQUEST event
      publishes
THEN  the event's pending field is 3, and the panel's title reads
      "AgentX Needs Your Input (1 of 3)".

GIVEN the front request is resolved and the queue advances to the next one
WHEN  that next request's APPROVAL_REQUEST event publishes (RequestDecision's
      defer/dequeue branch)
THEN  its pending field reflects the new, now-smaller queue depth (2, not 3)
      — the count is read fresh at publish time, not carried stale from the
      original enqueue.

GIVEN the APPROVAL_REQUEST payload arrives with pending as either a native
      int (in-process chat surface) or a JSON-decoded float64 (a remote
      surface attached over transport)
WHEN  chat.Model handles the event
THEN  both decode correctly to the same displayed count.
```

## Tests

- `internal/runtime/gate_test.go` (new, or extended if one exists):
  `TestGateLenReflectsQueueDepth` — enqueue several requests, assert `Len()`
  at each step; dequeue and confirm it decrements.
- `internal/runtime/decision_test.go` (extended if one exists, else new):
  a test driving `RequestDecision` with overlapping concurrent calls,
  asserting the published `APPROVAL_REQUEST` events carry the expected
  `pending` counts in sequence.
- `internal/surfaces/chat/chat_test.go` (extended): `TestApprovalPanelTitleShowsCount`
  covering both the `pending <= 1` (unchanged title) and `pending > 1`
  ("1 of N") cases, plus a case constructing the event payload with `pending`
  as `float64` to prove the transport-decoded path works identically to the
  native-int path.
- Full existing suite / `make all` passes unchanged.
