# Behavior — Chat TUI: Pending-Approval Duration on the Approval Panel Title

Status: **Implemented** (2026-08-02).

## Problem

Phase B (`docs/architecture/behavior/chat_pending_approval_count.feature.md`)
made the approval panel title show queue depth. It still doesn't answer the
question that matters most for the workflow this whole approval-loop effort
is aimed at — start a run, walk away, check back later: is this pending
decision one that just came up, or has it been sitting since before I left?
Today the only way to tell is cross-referencing event timestamps by hand
(exactly what manual session-log inspection had to do earlier in this
project). A glanceable "waiting 23m" on the panel itself closes that gap
directly.

## Design

`pendingRequest[Req, Resp]` (`internal/runtime/gate.go`) gains an immutable
creation timestamp, set once at construction — this is "since a decision
became necessary," not "since it became front-of-queue and visible," a
deliberate choice: time the system has been needing a decision is the honest
answer to "how long has this been waiting," even for the (currently rare)
case where a request sat queued behind another before ever being shown.

```go
type pendingRequest[Req any, Resp any] struct {
    payload    Req
    resp       chan Resp
    enqueuedAt time.Time
}

func newPendingRequest[Req any, Resp any](payload Req) *pendingRequest[Req, Resp] {
    return &pendingRequest[Req, Resp]{payload: payload, resp: make(chan Resp, 1), enqueuedAt: time.Now()}
}
```

No new `gate` method needed — `RequestDecision` already holds the exact
`*pendingRequest` (`req`, or `next` from `dequeue`) it's about to publish for,
so it reads `enqueuedAt` directly off that value. `publishApprovalPrompt`
gains a `since time.Time` parameter alongside Phase B's `pending int`,
carried on the same `APPROVAL_REQUEST` payload as `since.UnixMilli()` —
matching the millisecond-epoch convention every other timestamped event field
in this codebase already uses.

The chat surface decodes `since` (native `int64` in-process, JSON-decoded
`float64` over transport — a `decodeInt64` sibling to Phase B's `decodeInt`),
stores it as `time.Time`, and computes elapsed duration fresh at render time
— never cached, so it's always accurate regardless of when it's looked at.
`approvalPanelTitle` gains an `elapsed time.Duration` parameter (the caller's
job to compute `time.Since`, keeping the title-building function itself a
pure, easily-testable function of already-known values, not of wall-clock
time):

```go
func approvalPanelTitle(pending int, elapsed time.Duration) string
```

Coarse duration formatting, matching the tick interval's own granularity —
seconds under a minute, minutes under an hour, hours+minutes beyond that.

**Keeping the display live**: `View()` is a pure render of `Model`'s current
fields, and nothing was periodically re-invoking it while a decision sits
idle — the existing spinner ticker explicitly stops once
`proc.State != StateWorking` (`StateAwaitingInput` is not `StateWorking`), so
without a new periodic trigger, "waiting Xm" would freeze at whatever value
happened to be on screen when the panel last redrew for an unrelated reason.
A new self-sustaining tick loop (mirroring the existing `spinner.TickMsg`/
`banner.TickMsg` self-stopping pattern already in this file) fires only while
`StateAwaitingInput`, at a deliberately coarse 15s interval — a duration
display doesn't need per-second precision, and ticking is pure redraw
overhead while nothing else is happening.

```
GIVEN a single pending approval request, 90 seconds old
WHEN  the panel renders
THEN  the title includes "waiting 1m" (no queue-count suffix, since
      pending <= 1) — Phase B's suffix and this phase's duration compose
      independently, neither gates the other.

GIVEN three pending requests, the front one 3700 seconds old
WHEN  the panel renders
THEN  the title includes both "(1 of 3)" and "waiting 1h01m".

GIVEN the front request resolves and the queue advances to the next one
WHEN  that next request's APPROVAL_REQUEST event publishes
THEN  its since field is that specific request's own enqueuedAt — an
      older, previously-queued request shows its real, longer wait, not a
      value reset to "now" just because it became visible.

GIVEN the chat surface is sitting idle with a decision pending and no new
      events arriving
WHEN  15 seconds elapse
THEN  the panel redraws and the displayed duration has visibly advanced —
      proving the tick loop actually keeps the display live, not just
      correct-at-first-render.

GIVEN the decision resolves (proc.State leaves StateAwaitingInput)
WHEN  the next scheduled tick fires
THEN  the tick loop does not reschedule itself — it stops, the same
      self-stopping contract the existing spinner ticker already has.

GIVEN since arrives as either a native int64 (in-process) or a JSON-decoded
      float64 (remote transport)
WHEN  chat.Model handles the event
THEN  both decode to the same time.Time and the same displayed duration.
```

## Tests

- `internal/runtime/approval_test.go` (extended): a test proving
  `newPendingRequest` stamps a non-zero, roughly-`time.Now()` `enqueuedAt`,
  and that `publishApprovalPrompt` carries whatever `since` value it's given
  through to the event payload as `since.UnixMilli()` unchanged.
- `internal/surfaces/chat/chat_test.go` (extended): `TestApprovalPanelTitleShowsDuration`
  covering the formatting boundaries (seconds/minutes/hours), a case
  combining the count suffix and duration together, and a wire-shape test
  (int64 vs. float64) mirroring Phase B's pattern for `pending`. A separate
  test drives the tick message directly and confirms it does not reschedule
  once `proc.State` is no longer `StateAwaitingInput`.
- Full existing suite / `make all` passes unchanged.
