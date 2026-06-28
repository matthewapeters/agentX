# AgentX — Collapsible Output Widget (TUI)

_Status: Spec (Stage-1 target). Re-authors the legacy **PD-09 CollapsibleSection**
and the PD-01 message-entry rendering for the Bubble Tea chat surface._

> This supersedes the GTK/Tkinter rendering in `03_PANEL_DETAILS.md` (PD-01, PD-09)
> **for the chat output panel only**. Entry types, emojis, and turn ordering are
> inherited from PD-01; this document defines the *widget chrome* (border, line
> cap, inner scroll, scrollbar) that PD-01/PD-09 do not specify.

## Purpose

Every renderable element in the output panel — user prompt, classification,
thinking, tool call, tool result, assistant response, error, system notice — is a
**collapsible output widget**. The widget guarantees a constant, scannable
transcript: a one-line header is always visible, and any longer body is collapsed
by default, bounded to a configurable height, scrollable in place, and framed by a
box border.

## Component mapping (Bubble Tea / Bubbles / Lipgloss)

A suitable composition exists in the vendored components; no new external
dependency is required.

| Concern | Component / API |
|---------|-----------------|
| Box border | `lipgloss.NormalBorder()` applied with `lipgloss.NewStyle().Border(...)` (IBM single-line `┌─┐│└┘`) |
| Body height cap + inner scroll | `charm.land/bubbles/v2/viewport` (`SetHeight`, `SetWidth`, `SetContent`, `SoftWrap=true`) |
| Scroll position metrics | `viewport.ScrollPercent()`, `viewport.TotalLineCount()`, `viewport.VisibleLineCount()` |
| Proportional scrollbar thumb | derived (see below); composed with `lipgloss.JoinHorizontal` |
| Header word-break truncation | `ansi.Wrap(header, width, " -")` → first line |

**Scrollbar thumb derivation** (Bubbles has no built-in thumb — the pager example
only prints a percentage, so we compute it):

```
track  = body viewport height (rows)
total  = viewport.TotalLineCount()
visible= viewport.VisibleLineCount()
thumb  = max(1, round(track * visible / total))     // size
top    = round((track - thumb) * viewport.ScrollPercent())  // offset
```

Render a one-column track to the right of the body: `█` for thumb rows, `░` for
track rows. The scrollbar is shown only when `total > visible`.

## Anatomy

```
 ┌─ 💭 thinking  ────────────────────────────────────────┐   ← header row (always visible)
 │ The user is asking about parser internals, so I    █ │   ← body (viewport), capped
 │ should inspect parser.go before answering. The     █ │     at max_widget_lines;
 │ relevant function is Parse(), which …              ░ │     right column = scrollbar
 │ …                                                  ░ │
 └────────────────────────────────────────────────────┘
```

When collapsed, only the header row inside the top border is shown (no body, no
scrollbar):

```
 ┌─ 💭 thinking  (expand: enter) ─────────────────────────┐
 └────────────────────────────────────────────────────┘
```

Streaming entries (assistant response) render expanded and auto-follow the bottom
while tokens arrive.

## Canonical emoji set (output panel)

Inherited from PD-01, with one collision resolved (PD-01 assigns `⚙️` to
classification, but the current code uses `⚙` for system notices):

| Entry | Emoji | Collapsed by default |
|-------|-------|----------------------|
| User prompt | 👤 | no |
| Classification | ⚙️ | no (single greyed `intent → route` line) |
| Thinking | 💭 | **yes** |
| Tool call | 🔧 | no |
| Tool result | 📋 | **yes** |
| Assistant response | 🤖 | no (streams, then static) |
| Error | ⚠ | no |
| System notice | 📜 | no |

> The PhaseStepper widget (PD-05) uses a *different* set (`🤔` classify, `✍️`
> respond); that is the plan/cycle stepper, not these output entries — do not
> conflate them.

## Configuration

`~/.config/agentx/agentx.toml`:

```toml
[agentx.output]
max_widget_lines = 20   # max body rows before a widget scrolls internally
```

- Absent/zero → default `20`.
- The cap bounds the *visible* body; longer bodies scroll within the cap.

## Behaviour

### Header is always visible and word-break truncated

```gherkin
GIVEN an output widget with header "🔧 read_file /very/long/path/to/some/file.go"
  AND a panel width of 30 columns
WHEN the widget renders
THEN the header occupies exactly one row
  AND the text is truncated at the last whole word that fits the inner width
  AND a trailing ellipsis "…" marks truncation
  AND the leading emoji is always preserved.
```

### Collapse / expand

```gherkin
GIVEN a thinking widget (collapsed by default)
WHEN the widget first renders
THEN only its header row is visible inside the border
  AND the body is not rendered.

GIVEN a collapsed widget is focused
WHEN the user presses Enter (or Space)
THEN the widget expands and its body becomes visible (bounded by max_widget_lines)
WHEN the user presses Enter again
THEN the widget collapses to its header row.
```

### Height cap and inner scroll

```gherkin
GIVEN max_widget_lines = 20
  AND an expanded widget whose body is 8 wrapped lines
WHEN the widget renders
THEN the box height is 8 body rows (it shrinks to fit) and no scrollbar is shown.

GIVEN max_widget_lines = 20
  AND an expanded widget whose body is 50 wrapped lines
WHEN the widget renders
THEN exactly 20 body rows are visible
  AND a vertical scrollbar is shown in the right column
  AND the widget is internally scrollable.
```

### Proportional scrollbar thumb

```gherkin
GIVEN an expanded widget with 50 body lines shown in a 20-row viewport
WHEN the scrollbar renders
THEN the thumb height is proportional to visible/total (≈ 8 of 20 rows)
  AND the thumb sits at the top while scrolled to the top
  AND the thumb sits at the bottom while scrolled to the bottom
  AND intermediate scroll positions place the thumb proportionally between.
```

### Focus and scrolling

```gherkin
GIVEN multiple widgets in the output transcript
WHEN the user moves the selection (up/down) to a widget
THEN that widget is the focused widget (a subtle focus affordance on its border).

GIVEN a focused, expanded widget whose body exceeds max_widget_lines
WHEN the user scrolls (PgUp/PgDn or j/k)
THEN only that widget's inner viewport scrolls
  AND the surrounding transcript does not move
WHEN the body is at its last line and the user scrolls down
THEN scrolling passes back to the transcript.
```

### Box border

```gherkin
GIVEN any output widget
WHEN it renders
THEN it is framed by a single-line box border (lipgloss NormalBorder)
  AND the header text sits on the top border row
  AND the border reflows to the current panel width on resize.
```

## Ordering (inherited from PD-01)

Within one turn the widgets always render top-to-bottom in this order, below the
user prompt:

```
👤 user prompt
  ⚙️ classification        (single greyed line: intent → route)
  💭 thinking              (collapsed)
  🔧 tool call / 📋 result (collapsed result; repeats per tool round)
  🤖 assistant response    (streams last)
```

## Out of scope (this spec)

- Mouse-driven scroll/selection (keyboard-first; mouse capture stays off to keep
  native terminal selection — see chat-surface UX notes).
- Markdown/syntax rendering inside bodies (plain wrapped text for v1).
- Copy/selection affordances (PD-01-AF-010, deferred).
