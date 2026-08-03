# Behavior — Chat TUI: Hard Width Clamp Prevents Input/Approval From Scrolling Off-Screen

Status: **Implemented** (2026-08-02).

**Honest note on root-causing:** the defensive clamp below is confirmed
effective regardless of cause. A specific, confirmed-real bug was also found
and fixed alongside it — `padLine` (used by `hintStrip`) and a duplicated
inline copy of the same logic in `statusBar` measured width by **rune count**
(`len([]rune(s))`), not display width, unlike their sibling `padCells` right
next to them in the same file. Rune count only equals display width for plain
ASCII with no ANSI styling; any styled content (a colored spinner frame, a
styled label) breaks that assumption, and naive rune-slicing can cut an ANSI
escape sequence mid-code, producing a malformed sequence the terminal may
render as literal, width-consuming garbage. This is a real, confirmed defect
— but neither `hintStrip`'s nor `statusBar`'s *current* content is styled or
unbounded in length, so it could not be confirmed as *the* exact cause of the
original session's overflow. It's fixed as a genuine correctness fix either
way; the clamp is what provides the actual guarantee.

## Problem

`chat.Model.View()` (`internal/surfaces/chat/chat.go`) composes a fixed-height
frame — banner, bordered output panel, status bar, hint row, optionally a
bordered approval panel, bordered input panel — with `tea.NewView`'s
`AltScreen: true`. The height budget (`relayout`) is internally
self-consistent: each panel is sized so the sum of all rows equals `m.height`.
But nothing guarantees every individual *row* is `<= m.width` display columns.
A single row wider than the terminal gets soft-wrapped by the terminal itself
into an extra physical row bubbletea's own height accounting never knows
about — pushing every row below it (including the input and approval panels)
down and off the visible viewport. A real session hit this: several output
lines exceeded the terminal's column width.

Width reduction happens across four nested layers in three different files
(chat.go's outer panel border, output.go's scrollbar gutter, each entry's own
per-widget border, depth-based indentation) — a mismatch anywhere in that
chain can produce an overlong row. Rather than chase the exact originating
layer as the sole fix, this phase adds a structural guarantee that makes the
whole *class* of bug impossible regardless of which upstream layer produced
the overlong row, per the two-part fix already scoped: this phase is part 1
(the clamp + its regression test); root-causing the specific mismatch is
separate, lower-urgency follow-up work this phase does not attempt.

## Design

`View()` gains a final pass, after all rows are assembled and before
`strings.Join`: every row is measured with `ansi.StringWidth` (the same
display-width-aware measurement already used throughout `scrollutil.go`, not
naive byte length) and, if it exceeds `m.width`, hard-truncated to fit via
`ansi.Truncate` — preserving ANSI styling codes correctly rather than cutting
mid-escape-sequence. This is a safety net, not a replacement for correct
upstream wrapping: it never *should* fire in the common case, and does not
change any row that already fits.

```go
func clampRowWidth(rows []string, width int) []string {
    if width <= 0 {
        return rows
    }
    for i, r := range rows {
        if ansi.StringWidth(r) > width {
            rows[i] = ansi.Truncate(r, width, "")
        }
    }
    return rows
}
```

Called as the last step in `View()`, immediately before `strings.Join(rows,
"\n")` — after the banner, output, status bar, hint, approval, and input rows
are all already appended, so it clamps regardless of source.

```
GIVEN every row View() assembles already fits within m.width
WHEN  the clamp pass runs
THEN  no row is modified — byte-for-byte identical output to today, for the
      entire existing test suite's worth of normal content.

GIVEN a row wider than m.width reaches the clamp pass (regardless of which
      upstream panel produced it — output content, status bar, hint strip,
      approval panel, or input)
WHEN  View() runs
THEN  that row is truncated to exactly m.width display columns, preserving
      ANSI styling rather than corrupting it, and every row after it in the
      final composed frame — critically, the input and approval panel rows —
      remains within the visible viewport, since no row can force the
      terminal to insert an unaccounted-for extra physical row.

GIVEN a single unbroken token (a long path or URL with no spaces) wider than
      the output panel's wrap width, injected as a tool result or agent
      response
WHEN  it flows through the existing wrap pipeline and then View()'s clamp
      pass
THEN  the final row is still <= m.width — the adversarial case the real
      session hit.

GIVEN wide/multi-byte (double-width or emoji) characters in content
WHEN  the clamp measures and truncates
THEN  ansi.StringWidth/ansi.Truncate's existing display-width awareness
      (already relied on elsewhere in this codebase, e.g. scrollutil.go)
      handles this correctly — the clamp does not need its own width logic,
      only to call the same measurement already trusted throughout.
```

## Tests

- `internal/surfaces/chat/chat_test.go` (new): `TestViewNeverExceedsWidth`
  drives `Model.View()` across a range of window sizes with adversarial
  content (a long unbroken token with no spaces, injected output content) and
  asserts every resulting row's `ansi.StringWidth` is `<= m.width`. A second
  case confirms normal, already-fitting content is byte-for-byte unchanged by
  the clamp (proving it's a no-op in the common path).
- Full existing suite (`go test ./...`, `make all`) passes unchanged.
