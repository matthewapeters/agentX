# Behavior — Clamping the Approval Panel to the Terminal's Actual Height

Status: **Implemented** (2026-08-02).

## Problem

Session `raw-interesting-elephant`: the last approval showed only "Approve
for this session" — every other option, and presumably the prompt, was
below the bottom of the visible viewport. A distinct bug from the two
already fixed (`docs/architecture/behavior/chat_width_overflow_clamp.feature.md`,
row *width*; `docs/architecture/behavior/approval_prompt_length_bound.feature.md`,
the approval widget's own *internal* row cap): `relayout()`
(`internal/surfaces/chat/chat.go`) computes `approvalH :=
m.approval.DesiredHeight()` and uses it unclamped. `outputHeight` is
floored at 0 when the budget goes negative, but `approvalH` itself is
never reduced to fit — so on a short terminal (or with several options and
a queued-preview section, `docs/architecture/behavior/
chat_pending_approval_batch_view.feature.md`), the approval panel can still
ask for more rows than `m.height - chrome - inputH - banner.Height()`
actually leaves, and nothing stops it from claiming that anyway. The panel
then renders every one of its rows — the terminal just can't display all of
them, so only what fits at the top (the prompt, then the first option) is
visible; the rest silently scrolls past the bottom, including the input
panel below it.

## Design

Two changes, working together — the panel must be told the truth about how
much room it actually has, and it must use that room wisely instead of
falling over when told less than it asked for.

**1. `relayout()` clamps `approvalH` to what's actually available before
using it anywhere** — the same floor `outputHeight` already gets, applied
symmetrically to the panel that was missing it:

```go
var approvalH int
if awaiting {
    m.approval.SetSize(innerW, 0) // 0 = query desired height, unconstrained
    desired := m.approval.DesiredHeight()
    maxAvailable := m.height - chrome - inputH - m.banner.FullHeight()
    if maxAvailable < 1 {
        maxAvailable = 1 // never collapse to 0 — see promptCap's own "0 = unconstrained" note below
    }
    approvalH = min(desired, maxAvailable)
    m.approval.SetSize(innerW, approvalH)
}
```

`maxAvailable` is floored at 1, not 0: `approval.Model` treats `SetSize`'s
height of exactly 0 as "not yet sized / unconstrained" (the same sentinel
meaning `SetSize(w, 0)` already had before this fix, and every existing
test that never calls `SetSize` with a real height relies on). Flooring at
1 keeps that sentinel unambiguous — a truly 0-row budget and "don't know
yet" would otherwise be indistinguishable.

**2. `approval.Model.View()` becomes height-aware, with a strict priority
order: options are never trimmed; the queued-preview section is dropped
first when tight; the prompt's visible window shrinks to whatever's left.**
Options are the one thing the user cannot make a decision without seeing —
sacrificing them to fit a long prompt is exactly the failure being fixed,
so they are unconditionally rendered in full, and everything else competes
for what remains:

```go
func (m *Model) View() string {
    queued := m.queuedLines()
    promptCap := maxPromptRows
    if m.height > 0 {
        budget := max(m.height-len(m.options), 0)
        if len(queued) > budget {
            queued = nil
        }
        promptCap = scrollutil.ClampInt(budget-len(queued), 0, maxPromptRows)
    }
    var rows []string
    rows = append(rows, m.promptRowsCapped(promptCap)...)
    // ...options loop, unchanged...
    rows = append(rows, queued...)
    ...
}
```

`promptLines()` (used by `DesiredHeight()`, which must stay a pure,
unconstrained "what would I like" query — `relayout()` needs that honest
number to decide how much to even try to clamp against) becomes a thin
wrapper: `promptRowsCapped(maxPromptRows)`. `promptRowsCapped(cap)` is the
existing wrap/window/scrollbar logic parameterized on the cap instead of
the fixed constant, so a shrunk prompt window still gets a correctly-sized
scrollbar (`track` in `scrollutil.ScrollbarCell` matches the actual window,
not a stale `maxPromptRows`) rather than a slice-after-the-fact truncation
that would misalign it.

The math is self-consistent in the common (unconstrained) case: when
`maxAvailable >= desired`, `relayout()` passes `approvalH == desired ==
len(promptLines())+len(options)+len(queuedLines())` straight through, so
`budget := m.height-len(options) == len(promptLines())+len(queuedLines())`
exactly — `queued` never gets dropped and `promptCap` resolves to exactly
`len(promptLines())`, identical to today's unconstrained rendering. Nothing
changes for a terminal with enough room; only a genuine squeeze degrades,
and degrades in the order that keeps the panel usable.

```
GIVEN a proposal with 4 options and a prompt long enough to want the full
      maxPromptRows, on a terminal too short to show all of it
WHEN  the approval panel renders
THEN  all 4 options are visible — never fewer — even though the prompt's
      visible window shrinks (and the queued-preview section, if present,
      is dropped) to make room.

GIVEN a terminal with ample room (maxAvailable >= DesiredHeight())
WHEN  the approval panel renders
THEN  output is byte-identical to before this fix — the clamp and the
      priority degradation are both no-ops in the common case.

GIVEN a queued-preview section that would fit but the prompt would not fit
      at all alongside it within the available budget
WHEN  the approval panel renders
THEN  the queued section is dropped entirely (not partially — a header row
      with no items under it, or a truncated item, would be more confusing
      than no section at all) before the prompt's own window shrinks.

GIVEN a terminal so short that even the options alone exceed m.height
WHEN  the approval panel renders
THEN  every option still renders in full (the panel may render slightly
      taller than its nominal budget in this pathological case) — an
      unusable, silently-incomplete option list is strictly worse than a
      panel a few rows taller than ideal.
```

## Tests

- `internal/surfaces/approval/approval_test.go` (extended): a height-
  constrained `SetSize` still renders every option; the queued section
  drops before the prompt's window shrinks; an ample-height case is
  byte-identical to the unconstrained render; `DesiredHeight()` stays
  unconstrained regardless of `SetSize`'s height argument.
- `internal/surfaces/chat/chat_test.go` (extended): `relayout()` on a short
  terminal with an oversized approval prompt never gives `approvalH` more
  than the terminal minus chrome/input/banner allows.
- Full existing suite / `make all` passes unchanged.
