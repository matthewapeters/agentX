# Behavior — Chat TUI: Batch Preview of Queued Approvals

Status: **Implemented** (2026-08-02).

## Problem

Phase B put a "(1 of N)" count on the approval panel title; Phase C added
how long the front request has been waiting. Both tell the user *how many*
decisions are queued and *how stale* the current one is, but not *what* is
actually waiting behind it. A plan that issues several write/network calls
in a row (the exact case Phase B's problem statement called out) leaves the
user resolving one opaque prompt at a time, with no way to look ahead —
"is item 2 of 3 another `write_file` to the same path, or something I'd
want to review differently?" is unanswerable today without approving (or
denying) blind and finding out. A compact, read-only preview of what's
queued — reviewed together with the current prompt, not resolved together —
closes that gap without touching the gate's one-at-a-time decision
mechanics (a deliberate design choice; see `internal/runtime/gate.go`'s doc
comment on why decisions serialize).

## Design

`gate[Req, Resp]` gains a read-only snapshot accessor, alongside `Len()`:

```go
// Queued returns every payload currently in the queue, in FIFO order,
// including the one at the front (currently shown). Callers that only want
// what's waiting BEHIND the front slice off index 0 themselves — the gate
// has no opinion about what "front" means to a caller, same as Len().
func (g *gate[Req, Resp]) Queued() []Req {
    g.mu.Lock()
    defer g.mu.Unlock()
    out := make([]Req, len(g.pending))
    for i, r := range g.pending {
        out[i] = r.payload
    }
    return out
}
```

`decision.go` adds `queuedPrompts`, extracting the prompt text of everything
queued behind the front request:

```go
func queuedPrompts(all []approvalUIRequest) []string {
    if len(all) <= 1 {
        return nil
    }
    out := make([]string, 0, len(all)-1)
    for _, r := range all[1:] {
        out = append(out, r.prompt)
    }
    return out
}
```

`publishApprovalPrompt` gains a `queued []string` parameter, carried on the
existing `APPROVAL_REQUEST` payload as `"queued"` — both `RequestDecision`
call sites pass `queuedPrompts(o.gate.Queued())`, read fresh at publish
time (same freshness discipline Phase B's `pending`/Phase C's `since`
already established: never cached, always reflects the queue at the moment
this specific prompt becomes the one shown).

**Rendering.** `approval.Model` (`internal/surfaces/approval/approval.go`)
— the swapped-in widget that already owns the prompt/options display and
drives its own `DesiredHeight()`, which `chat.go`'s `relayout()` already
folds into the height budget generically — is the natural owner of this
preview too, rather than teaching `chat.go` about a second kind of content
inside the approval panel. `Set` gains a third parameter:

```go
func (m *Model) Set(prompt string, options []state.ApprovalOption, queued []string)
```

and a capped, truncated preview renders after the options:

```go
const maxQueuedPreview = 5

// queuedLines renders up to maxQueuedPreview queued prompts, each
// truncated (not wrapped) to one row — a compact glance, not full detail;
// the user resolves each one for the full prompt+options treatment when
// its turn comes. A "+N more" summary row covers anything beyond the cap,
// so the panel's height stays bounded regardless of how deep the queue
// gets (a plan dispatching a dozen write calls must not blow the height
// budget Phase A's clamp exists to protect).
func (m *Model) queuedLines() []string
```

Nothing renders when `len(queued) == 0` — the common case (one pending
decision) looks exactly as it did before this phase.

The chat surface decodes `queued` off the event payload with the same
dual-mode discipline every other `APPROVAL_REQUEST` field already uses
(native `[]string` in-process, JSON-decoded `[]any` of strings over
transport):

```go
func decodeStringSlice(v any) []string
```

```
GIVEN a single pending approval (the common case)
WHEN  the panel renders
THEN  no queued-preview section appears — unchanged from before this phase.

GIVEN three approval requests enqueued in sequence, all before any is
      resolved
WHEN  the first becomes front-of-queue and its APPROVAL_REQUEST event
      publishes
THEN  the event's queued field lists the OTHER two requests' prompts, in
      FIFO order, excluding the front request's own prompt (which is
      already shown as the main prompt, not duplicated in the list).

GIVEN more than maxQueuedPreview requests are queued behind the front one
WHEN  the panel renders
THEN  only the first maxQueuedPreview prompts are listed, followed by a
      "+N more" summary row — the panel's height never grows unbounded
      with queue depth.

GIVEN the front request resolves and the queue advances
WHEN  the next request's APPROVAL_REQUEST event publishes
THEN  its own queued field reflects the new, now-shorter list of what's
      still behind IT — read fresh, not carried over stale from the
      previous publish.

GIVEN the queued field arrives as either a native []string (in-process) or
      a JSON-decoded []any of strings (a remote surface attached over
      transport)
WHEN  chat.Model handles the event
THEN  both decode to the same displayed preview list.
```

## Tests

- `internal/runtime/gate_test.go`/`approval_test.go` (extended):
  `Queued()` returns payloads in FIFO order including the front; a test
  proving `queuedPrompts` excludes index 0 and returns nil for a
  queue of 0 or 1.
- `internal/surfaces/approval/approval_test.go` (extended): `DesiredHeight`
  grows with a non-empty `queued` list; `queuedLines` caps at
  `maxQueuedPreview` with a "+N more" row; an empty `queued` list renders
  nothing (byte-identical `View()` output to before this phase, for the
  common single-pending case).
- `internal/surfaces/chat/chat_test.go` (extended): the wire-shape test
  (native vs. JSON-decoded) extended to also cover `queued`.
- Full existing suite / `make all` passes unchanged.
