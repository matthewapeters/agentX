# Behavior — Expanding Tabs Before Any Width Measurement or Wrapping

Status: **Implemented** (2026-08-03).

## Problem

Sessions `raw-interesting-elephant`/`naive-stunning-eagle`: after hitting the
per-turn tool-call limit, the terminal's bottom rows (including the input
panel) were pushed off-screen — with no live approval pending, so neither
of the two previously-fixed causes (`docs/architecture/behavior/
approval_prompt_length_bound.feature.md`,
`docs/architecture/behavior/approval_panel_height_budget.feature.md`)
applies. Root cause, confirmed by direct reproduction
(`internal/surfaces/output`, isolated `viewport.Model` test): a proposed
`write_file` call's `content` argument is real Go source, which uses tab
characters for indentation. `ansi.StringWidth("a\tb")` measures a tab as a
single column (`= 2` for that example) — the width `scrollutil.WrapLines`
and `renderBody`'s per-widget row-count bookkeeping is built on. But
`lipgloss.NewStyle().Width(w).Render(...)` — what `output.Model`'s
underlying `viewport.Model.View()` actually calls to produce the final,
height-constrained string — expands a raw tab to its terminal tab-stop
width when rendering, which is wider. A line `ansi.StringWidth` measured as
fitting exactly at the configured width is, by lipgloss's own measure,
*not* fitting — so lipgloss soft-wraps it into two physical rows. Nothing
in `renderBody`'s or `relayout()`'s row-count math anticipates this: it all
trusts `ansi.StringWidth`'s count of the input lines, not what the terminal
(via lipgloss) actually produces. Two tab-containing lines → two untracked
extra rows → the exact overflow observed.

## Design

Eliminate the ambiguity at its source instead of trying to reconcile two
width algorithms that disagree specifically about tabs: expand every tab to
plain spaces before ANY width measurement or wrapping happens. Plain spaces
have unambiguous, universally-agreed width — once no tab characters remain
in content by the time it reaches `ansi.StringWidth`, `WrapLines`, or
lipgloss's `Render()`, all three measure the identical thing.

`scrollutil.WrapLines` — the single shared entry point every relevant
rendering path (`internal/surfaces/output.Model.renderBody`,
`internal/surfaces/approval.Model.promptRowsCapped`,
`internal/surfaces/workmemory`) already wraps content through before it's
padded/boxed/handed to any viewport — expands tabs first, so every caller
is fixed by one change:

```go
// tabWidth is the fixed number of spaces a tab expands to before any
// width measurement or wrapping — not an attempt at true tab-stop
// semantics (which depend on cursor column), just enough to guarantee
// ansi.StringWidth and lipgloss's own internal measurement always agree,
// since plain spaces have unambiguous width and a raw tab does not
// (docs/architecture/behavior/scrollutil_tab_width_disagreement.feature.md).
const tabWidth = 4

func expandTabs(s string) string {
    if !strings.Contains(s, "\t") {
        return s
    }
    return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth))
}

func WrapLines(s string, w int) []string {
    s = expandTabs(s)
    ...
}
```

`proposalText`'s own truncation
(`docs/architecture/behavior/approval_prompt_length_bound.feature.md`)
still bounds the raw character count fed to `WrapLines` in the first
place — this is a second, independent layer: even a short, already-
truncated string can still contain a tab, and that tab alone is what
breaks the row-count invariant, regardless of overall length.

```
GIVEN a string containing a tab character, at a width where ansi.StringWidth
      says it fits exactly
WHEN  WrapLines wraps it
THEN  the tab is expanded to spaces first, so the wrapped result's row count
      matches what lipgloss's own Render() at the same width would produce —
      no silent extra row.

GIVEN a widget body containing real Go source (tab-indented) long enough to
      approach a panel's row cap
WHEN  the output panel (or the approval panel's prompt window) renders it
THEN  the actual rendered row count never exceeds what relayout()'s height
      budget allocated — the confirmed overflow this closes.

GIVEN content with no tab characters at all
WHEN  WrapLines wraps it
THEN  output is byte-identical to before this fix (expandTabs is a cheap
      strings.Contains check, no-op when there's nothing to expand).
```

## Tests

- `internal/surfaces/scrollutil/scrollutil_test.go` (new, or extended):
  `WrapLines` on a tab-containing line produces the same row count lipgloss
  itself would produce at the same width; a tab-free string is unchanged.
- `internal/surfaces/output/output_test.go` (extended): a widget body
  containing tabs, wide enough to approach `renderBody`'s cap, never
  produces more rendered rows than `m.maxBody` allows, and the FULL output
  panel's `View()` never exceeds its configured height.
- `internal/surfaces/chat/chat_test.go` (extended): a regression test
  reproducing the reported scenario — many resolved approval cycles with
  tab-containing (real Go source) proposal content, ending in a
  "stopped: tool-call limit" response — asserting `View()`'s total row
  count never exceeds `m.height`, across a range of realistic and short
  terminal sizes.
- Full existing suite / `make all` passes unchanged.
