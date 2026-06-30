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

[agentx.theme]
active_border_color   = "cyan"        # focused panel + selected widget
inactive_border_color = "dark gray"   # unfocused panel + other widgets
```

- Absent/zero `max_widget_lines` → default `20`. The cap bounds the *visible*
  body; longer bodies scroll within the cap.
- Border colors accept a name (`cyan`, `dark gray`, …), an ANSI-256 index
  (`"240"`), or a hex value (`"#00afaf"`). Absent → cyan / dark gray.

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

### Collapse / expand (uniform across widget kinds)

Every widget that carries a body — **user, thinking, tool, and assistant** alike —
is collapsible with identical mechanics: `Enter` (or `^o`) toggles the selected
widget, and the expanded body is bounded by `max_widget_lines` regardless of kind.
The label header (`👤 You`, `💭 thinking`, `🔧 <tool>`, `🤖 AgentX`) stays visible
when collapsed. Defaults differ only in noise level: thinking and tool *results*
start collapsed; user, tool calls, and the assistant answer start expanded.

```gherkin
GIVEN any widget with a body (user, thinking, tool, or assistant)
WHEN the widget is selected and the user presses Enter (or ^o)
THEN the widget toggles between showing only its header and showing its body
  AND an expanded body is bounded by max_widget_lines (scrolling within the cap).

GIVEN a thinking or tool-result widget
WHEN it first renders
THEN only its header row is visible (collapsed by default).

GIVEN a user or assistant widget
WHEN it first renders
THEN its body is visible (expanded by default), bounded by max_widget_lines.
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

### Focus & keymap (CHT-D5)

The chat surface tracks which **panel** holds focus (input or output). ESC is a
**leader/chord** key, not a persistent mode:

| Gesture | Effect |
|---------|--------|
| `ESC,q` | quit |
| `ESC,↑` | move focus to the OUTPUT panel |
| `ESC,↓` | move focus to the INPUT panel |
| `ESC,ESC` | interrupt the in-flight response (only while working) |
| `PgUp` / `PgDn` | auto-focus OUTPUT and move the widget selection |
| `j` / `↓`, `k` / `↑` | scroll the selected widget (OUTPUT focus only) |
| `^o` / `Enter` | collapse/expand the selected widget (OUTPUT focus only) |

A pending chord is advertised in the hint strip; an unrecognized follow-up key
cancels it.

**Border hierarchy.** The focused panel renders a **bold** border in
`active_border_color`; the unfocused panel renders an un-bold border in
`inactive_border_color`. Inside the OUTPUT panel the *selected* widget keeps its
heavy border and lights up in the active color **only while OUTPUT is focused**;
every other widget (and the selection when OUTPUT is unfocused) uses the inactive
color.

```gherkin
GIVEN the input panel has focus
WHEN the surface renders
THEN the input border is bold in the active color
  AND the output border is un-bold in the inactive color.

GIVEN the input panel has focus
WHEN the user presses ESC then ↑
THEN focus moves to the output panel
  AND the output border becomes bold in the active color.

GIVEN the output panel has focus and a selected, expanded widget over the cap
WHEN the user presses j (or ↓)
THEN only that widget's inner viewport scrolls
  AND the surrounding transcript does not move.

GIVEN the output panel has focus
WHEN the user presses ESC then ↓
THEN focus returns to the input panel.
```

### Box border

```gherkin
GIVEN any output widget
WHEN it renders
THEN it is framed by a single-line box border (lipgloss NormalBorder)
  AND the header text sits on the top border row
  AND the border reflows to the current panel width on resize.
```

## Logo banner (bootstrap)

The output panel renders an optional **logo banner** as the very first element of
the transcript, above every widget. It is a pre-rendered, ANSI-colored block of
text (the application logo) supplied at startup. Its purpose is a bootstrap-time
visual signal that the application is running while the bootstrap prompt is being
processed — it appears on the first render, before any response arrives, and then
remains pinned at the top of the transcript for the session.

The banner is rendered verbatim except that each line is clipped (ANSI-aware) to
the current panel width, so its embedded color sequences are preserved while the
art never soft-wraps into garbage on a narrow terminal. The banner is not a
widget: it has no border, header, selection, or collapse, and it does not shift
the widget selection or scroll-pinning behavior.

The banner content is the build artifact embedded from `logo/agentx.logo` (see
`docs/implementation/09_makefile_and_quality_gate_contract.md` for the build-time
sync); the surface is given the content at startup via `SetBanner`.

```gherkin
GIVEN an output panel with a logo banner set
WHEN the panel renders before any event is applied
THEN the rendered transcript begins with the banner content

GIVEN an output panel with a logo banner set
WHEN a user_prompt widget is applied
THEN the banner still precedes the widget in the rendered transcript

GIVEN an output panel sized narrower than the banner's widest line
WHEN the panel renders the banner
THEN no rendered line is wider than the panel width
```

## Launch-info widget (attach surfaces)

The chat surface boots the server with an HTTP/SSE transport that external surfaces
attach to (M1). The attach endpoint and the session's ephemeral attach token are
needed to launch a peer surface — but the chat surface runs in the alternate screen,
so anything printed to stdout before the program starts is wiped and not scrollable.
To make the attach information durably available during the session, the output panel
renders a **launch-info widget**.

Placement and lifecycle:

- It is the **first widget** of the transcript — rendered after the logo banner and
  before the bootstrap response — so a user can always scroll to the top to find it.
- It is **collapsed by default** to keep the bootstrap view clean (and to avoid
  casually exposing the token); the header shows the endpoint and an expand hint, and
  expanding reveals one **full, copy-pasteable** `agentx surface launch <kind> …`
  command per launchable surface kind.
- It is **surface-local**, injected via `SetLaunchInfo` at startup — it is *not* a
  session event: it is never persisted to the event log and never appears on attached
  peer surfaces (which render only bus events). It exists solely on the hub chat
  surface that knows how to launch peers.
- It is omitted entirely when the transport is disabled (no endpoint).

The widget reuses the standard collapsible-widget machinery (selectable, Enter
toggles, body capped/scrolled like any other), so it shifts selection/scroll exactly
as a normal widget. The displayed token is the session's raw attach token; this is
local and loopback-only, and collapsed-by-default limits incidental exposure (e.g. a
screen-share).

```gherkin
GIVEN an output panel with launch info set for 2 surface kinds
WHEN the panel renders before any event is applied
THEN the launch-info widget is the first widget and is collapsed

GIVEN an output panel with launch info set
WHEN the launch-info widget is expanded
THEN it shows a full "agentx surface launch <kind>" command for each kind

GIVEN an output panel with launch info set
WHEN a user_prompt widget is applied
THEN the launch-info widget still precedes it in the transcript
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
