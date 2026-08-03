# Behavior — `frame()` No Longer Renders a Phantom Row for Empty Panel Content

Status: **Implemented** (2026-08-03).

## Problem

Found while reproducing sessions `raw-interesting-elephant`/
`naive-stunning-eagle`'s overflow at very short terminal heights (e.g.
40×8), after fixing the tab-width disagreement
(`docs/architecture/behavior/scrollutil_tab_width_disagreement.feature.md`)
that was the primary cause — a second, independent overflow source that
only shows up once `relayout()` genuinely clamps `outputHeight` to 0 (a
short terminal with heavy scrollback). Every panel's `View()` returns the
empty string `""` specifically to mean "zero content rows" (`output.Model`:
`if m.height == 0 { return "" }`; `input.Model` and `approval.Model` follow
the same convention). `chat.go`'s `frame(content, ...)` renders that content
via `for _, line := range strings.Split(content, "\n") { ... }` — but
`strings.Split("", "\n")` returns `[""]`, a slice of length **1**, not 0.
So an intentionally-empty panel still gets one phantom blank content row
rendered inside its border, on top of the fixed 2-row border `relayout()`'s
`chrome` accounting already budgeted for — an extra row nothing in the
layout math anticipated, pushing everything below it (down to the input
panel) one row further than the terminal actually has.

## Design

`frame()` skips the split-and-loop entirely when `content == ""`, so an
empty panel contributes exactly its 2 border rows, matching what
`relayout()`'s `chrome` constant already assumes:

```go
out := []string{paint(titledTopBorder(title, innerW))}
if content != "" {
    for _, line := range strings.Split(content, "\n") {
        out = append(out, paint("│")+padCells(line, innerW)+paint("│"))
    }
}
out = append(out, paint("└"+strings.Repeat("─", innerW)+"┘"))
```

This is a `strings.Split` footgun independent of any specific panel — it
would recur for the input or approval panel too if either were ever legally
sized to 0 content rows (today they aren't, since both floor their desired
height at 1, but `frame()` itself has no business assuming that about every
possible future caller).

```
GIVEN a panel's View() returns "" (0 content rows — e.g. the output panel
      when relayout() clamps outputHeight to exactly 0 on a very short
      terminal)
WHEN  frame() renders it
THEN  the result is exactly 2 rows (top + bottom border), not 3 — no
      phantom blank content row.

GIVEN a panel's View() returns ordinary non-empty content
WHEN  frame() renders it
THEN  behavior is unchanged from before this fix — the empty-content
      special case is a no-op for every other input.
```

## Tests

- `internal/surfaces/chat/chat_test.go` (extended): `frame("", ...)`
  produces exactly 2 rows; `frame("one line", ...)` is unchanged (3 rows);
  the end-to-end short-terminal regression test
  (`TestViewNeverExceedsHeightWithTabContentInScrollback`) covers the
  40×8 case where this phantom row was actually observed.
- Full existing suite / `make all` passes unchanged.
