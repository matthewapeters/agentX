# AgentX — Collapsible Output Widget (TUI)

_Status: Spec (Stage-1 target). Re-authors the legacy **PD-09 CollapsibleSection**
and the PD-01 message-entry rendering for the Bubble Tea chat surface._

> This supersedes the GTK/Tkinter rendering in `03_PANEL_DETAILS.md` (PD-01, PD-09)
> **for the chat output panel only**. Entry types, emojis, and turn ordering are
> inherited from PD-01; this document defines the *widget chrome* (border, line
> cap, inner scroll, scrollbar) that PD-01/PD-09 do not specify.

## Purpose

Every renderable element in the output panel — user prompt, thinking, tool call,
tool result, assistant response, error, system notice — is a **collapsible output
widget**. The widget guarantees a constant, scannable transcript: a one-line header
is always visible, and any longer body is collapsed by default, bounded to a
configurable height, scrollable in place, and framed by a box border.

**Classification is the one exception:** its payload is always a single line of
metadata (`intent → route`), so it renders **flat** — `⚙ classification · <intent →
route>` on one row, no box — rather than paying for a three-row frame. It is still
selectable (tinted by selection like a border).

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

The widget kind (emoji + type label) is rendered **in the top border**, not as a body
row, so every visible inner row is content:

```
 ┌─ 💭 thinking ──────────────────────────────────────────┐   ← title in the border
 │ The user is asking about parser internals, so I    █ │   ← body (viewport), capped
 │ should inspect parser.go before answering. The     █ │     at max_widget_lines;
 │ relevant function is Parse(), which …              ░ │     right column = scrollbar
 │ …                                                  ░ │
 └────────────────────────────────────────────────────┘
```

Collapse behaviour depends on the kind, so collapsing a verbose box hides it while a
narrative box still shows a one-line teaser:

- **Narrative** boxes (user prompt, assistant response, tool call) collapse to the
  titled border plus the **first content line** (with an `…` when there is more),
  so the gist stays visible:

  ```
   ┌─ 🤖 AgentX ─────────────────────────────────────────┐
   │ Here is the answer to your question about parser…   │
   └────────────────────────────────────────────────────┘
  ```

- **Noise** boxes (thinking, tool result) collapse to the **titled border only** — the
  label in the border says what it is, the content is hidden until expanded:

  ```
   ┌─ 💭 thinking ───────────────────────────────────────┐
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

There are two scrollbars: a **per-widget** thumb inside a box whose body exceeds
`max_widget_lines`, and a **transcript** thumb in a reserved right-gutter column of
the panel that shows where the visible window sits within the whole transcript. The
transcript gutter is blank when everything fits and shows a proportional thumb once
the content overflows the viewport; content renders one column narrower to make room.

```gherkin
GIVEN an expanded widget with 50 body lines shown in a 20-row viewport
WHEN the scrollbar renders
THEN the thumb height is proportional to visible/total (≈ 8 of 20 rows)
  AND the thumb sits at the top while scrolled to the top
  AND the thumb sits at the bottom while scrolled to the bottom
  AND intermediate scroll positions place the thumb proportionally between.

GIVEN more widgets than fit the viewport
WHEN the panel renders
THEN a transcript scrollbar thumb appears in the right gutter

GIVEN content that fits the viewport
WHEN the panel renders
THEN the right gutter is blank (no transcript scrollbar)
```

Emoji titles use glyphs with a deterministic single- or double-column width (e.g. the
plain `⚙`, not the VS16 `⚙️`), so the titled border's right corner stays aligned even
on terminals that render emoji-presentation selectors as one column.

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

## Plan widget (nested Step/Task DAG)

A decomposed plan (ADR 0009 §9c, redesigned 2026-07-08) gets **one** live widget per
root, created at the `task_plan` "started" snapshot and mutated in place as
`task_node`/`tool_call`/`tool_result` deltas arrive — never appended to, never
replaced. Unlike every other widget kind, it does not render from a static `body`
string: it draws its nested boxes recursively at *view* time (`renderPlanWidget` in
`internal/surfaces/output/plan.go`), so a terminal resize is correct for free.

The outer widget box *is* the root node's own box — its title bar is the plan's
summary line (`🗺 plan · N/M steps [· K running ∥]`); its first content row is the
root's own status line (glyph + goal + timing), followed by its children.

```gherkin
GIVEN a plan node that is a Step (kind "step")
WHEN it renders
THEN its own status line is its box's first content row
  AND, if it has decomposed, each child renders as its own fully nested box inside it,
      in order, recursively at whatever depth the plan actually reaches

GIVEN a plan node that is a Task (kind "task")
WHEN it renders
THEN its resolved tool command renders as one reverse-video row inside its box
  AND that row renders regardless of collapse state
  AND its result, if present and the node is expanded, renders as a further-nested
      collapsible box one level inside the command row

GIVEN a plan node
WHEN it or the plan overall is running
THEN its border and title glyph pick up an amber tint that overrides its kind color
  AND a running Task's status glyph is its own independently-animated spinner frame,
      not a static glyph — concurrently-running nodes animate independently, not in
      lockstep
```

**Color** differentiates Kind, not selection: Step boxes are blue (`38;5;75`), Task
boxes are tan (`38;5;180`), a running node of either kind is amber (`38;5;214`) —
selection is conveyed by border *heaviness* (the pre-existing thin/heavy convention),
not by switching to the active/inactive selection colors every other widget kind uses.

**Liveness-propagating auto-collapse** governs whether a node's children (if a Step)
or result (if a Task) render at all **while the plan is still running**: a node's own
children/result show exactly while it — or *any* descendant, at any depth — is
running. This is an all-or-nothing gate on the whole children group per level, not per
individual child: while a group is live, already-finished or not-yet-started siblings
in that same group render too (useful context, still bounded — each of *those*
siblings' own deeper content stays independently governed by its own liveness). If
nothing anywhere in the tree is currently live, the whole thing collapses uniformly to
just the root's own line — including mid-plan, between two bursts of activity.

**Once the plan has ended, liveness stops gating anything and the full structure
always shows**, unconditionally. A real fast tool call (`ls`, `tree`, `grep` — the
overwhelming common case) dispatches and completes in single-digit milliseconds, far
under one terminal frame; gating past "ended" the same way meant the "live" window was
in practice never rendered at all, and — since there is no manual per-node expand,
deliberately — a finished plan had no way back to showing its steps (session
`brave-fjord-2`, 2026-07-08). Liveness exists only to bound clutter on a plan that is
still actively producing more content; that concern doesn't exist once it's over, and
the widget's stated job is to be "the record of what ran."

```gherkin
GIVEN a Step that has decomposed but none of its children have started, plan still running
WHEN the plan renders
THEN the Step's own line shows, but its children do not

GIVEN a Step whose one running child has a finished sibling in the same group
WHEN the plan renders
THEN both the running child and its finished sibling render
  AND the finished sibling's own deeper content (further children, or a Task's
      result) stays collapsed regardless

GIVEN a plan in which every node has finished or never started, but the plan itself
      has not yet received its "ended" snapshot
WHEN the plan renders
THEN only the root's own status line shows — every child group is collapsed

GIVEN a plan that has received its "ended" snapshot
WHEN the plan renders
THEN every node's full content shows, regardless of how quickly each step ran
```

A width floor (`nodeBoxFloor`, 8 columns) bounds recursion depth on a narrow
terminal or a very deep plan: below it, a node falls back to a single flat
`glyph + goal` line instead of an unreadably narrow nested box — mirroring the
existing `innerW < 1` single-line fallback every other widget kind already has.

The full nested tree (kind, status, timing, command, result ref, explicit
parent/child containment distinct from sibling "waits-on" deps) is also persisted to
`sessions/<id>/plans/<root-id>.json` on every mutation (ADR 0009 §9b,
`internal/session/plans.go`) — the queryable, post-session-review companion to the
live rendering and the append-only event log.

### Wavefront plans, Step values, and convergence (ADR 0012 amendment)

AgentX's second decomposition engine (`internal/runtime/wavefront`, ADR 0012) is
**round-free and reuses this exact widget** — it is not a separate widget kind.
A wavefront plan's Know/open-Need/command-Need nodes are ordinary `KindStep`/
`KindTask` records dispatched through the same `NodeDispatched`/`NodeDecomposed`/
`NodeCompleted` observer callbacks the continuous engine uses, so everything
above (nesting, liveness, collapse, timing) applies unchanged. Three additions,
specific to what wavefront's nodes can carry that a continuous-engine node
today does not:

**A Step's resolved value or failure reason renders like a Task's result.** A
wavefront Know has no tool call behind it at all — its `Value` (or `Error`) is
its only content. Once such a Step is expanded, its value renders in a
collapsible `🧩 value` box (or `⚠ error`, taking precedence if both are somehow
set) one level deeper — the same `drawTextBox` machinery a Task's `📋 result`
box already uses, generalized rather than duplicated.

```gherkin
GIVEN a Step node (any engine) with a non-empty resolved Value and no Error
WHEN it is expanded
THEN a "🧩 value" box renders one level deeper, capped at maxResultLines like a
     Task's result

GIVEN a Step node with a non-empty Error
WHEN it is expanded
THEN it shows "⚠ error" instead of "🧩 value"
```

**Convergence renders as a reference annotation, never a duplicate box.**
Wavefront's merge step can fold a Need's edge onto an already-existing node
instead of creating a child (ADR 0012 §3) — a genuine cross-branch reference the
tree-shaped recursion above cannot express as a second nested box without either
duplicating content or breaking the "tree, not general graph" rendering model.
Instead, the *converging* node's content gains one line:

```gherkin
GIVEN parent P's classify response converges a Need onto an already-existing
      node E (found elsewhere in the graph, not created fresh)
WHEN the plan widget renders P expanded
THEN P's content includes "↳ converges onto: <E's goal>"
  AND E's own box — wherever its real, first owner rendered it — is unaffected;
      E's content is never drawn a second time under P
```

**A node-level pin cursor, for Context-surface Pin only.** A plan is one
top-level widget with many nodes inside it; pinning an individual node's
result/value (PD-CTX-AF-012, ADR 0012 amendment) needed *some* way to select one.
Rather than a new sub-widget selection framework, the plan widget gained a
minimal cursor — `Tab`/`Shift+Tab` in the context surface move it forward/back
through the plan's nodes (flat order, depth-agnostic) — that only exists, and
only renders (a `›` prefix on the active node's own title line), while the
owning plan widget is itself the selected top-level widget:

```gherkin
GIVEN a plan widget that is not the currently selected top-level widget
WHEN Tab/Shift+Tab is pressed
THEN nothing moves — the cursor only exists "inside" a selected plan widget

GIVEN a plan widget becomes selected for the first time
WHEN its node cursor is read
THEN it defaults to the plan's root node

GIVEN the plan widget is selected and its cursor is on some node N
WHEN the widget renders
THEN N's title shows the "›" prefix, and no other node's does
```

See `docs/ux/03_PANEL_DETAILS.md` §PD-CTX for what pressing `p` does with the
cursor's current node, and §PD-WM for how the resulting pin behaves once it
reaches working memory.

## Logo banner (pinned, collapsible, animated)

The chat surface renders a **logo banner** in a fixed region above the output
viewport — the same kind of screen-pinned region the input panel already
occupies below it (chat surface `computeLayout`), not the first line of
scrollable transcript content. The banner never scrolls: growing the
transcript shrinks the output viewport's share of the remaining rows, exactly
as it already does to make room for the input panel, but the banner itself is
outside the scrollable region entirely. The banner is not a widget: it has no
border, header, or selection, and it does not shift widget selection.

The full-size banner's cell content is a build artifact, not raw ANSI text:
`cmd/logogen` converts an authored ANSI-colored source (`logo/agentx.logo`)
into a structured grid of `Cell{Rune, Color}` (rune + xterm-256 palette
index) — see `logo/README.md`. The collapsed row is *not* a build artifact:
its text varies with what the agent is currently doing, so
`internal/surfaces/banner` synthesizes its cells at runtime instead (see
"Collapsed row label" below), padded with blank cells out to the full panel
width — not just the label text's own length — so the row's gradient/
animation spans edge to edge like a status bar, with the text sitting near
the start. The surface picks which grid is active and how to color it
without needing further input from the caller — it reacts to its own
measured content height and to `RunState`/`Phase`.

### Collapse: content-based and sticky

The banner starts full-size. It collapses to the single-row label the first
time the applied transcript content's height — measured against the output
viewport height available *under the full-size banner* (a fixed budget, so
the trigger doesn't move once evaluated) — exceeds one screenful. Once
collapsed, it stays collapsed for the rest of the session: later shrinking
the transcript, or resizing the terminal taller, does not restore the full
banner. There is no reverse transition.

### Collapsed row label

The collapsed row reads `AgentX - <activity>`, where `<activity>` tracks the
run's current `state.RunState`/`state.Phase` (`internal/surfaces/chat`'s
`bannerLabel`, re-evaluated on every processing-state change):

| RunState | Phase | Label |
|----------|-------|-------|
| `Idle` / `Completed` / `Failed` | any | `Your Local Agent` |
| `AwaitingInput` | any | `Needs Input` |
| `Working` | `thinking`, `classify` | `Thinking` |
| `Working` | `tool` | `Working` |
| `Working` | `respond` | `Responding` |
| `Working` | `planning` | `Planning` |

`AwaitingInput` takes priority over phase — the user needs to know a decision
is pending, not what phase the run paused in. The label is a purely local
rendering choice: it is not a session event and is not persisted.

### Color-cycle animation ("rainbow wave")

Whichever grid is active (full or collapsed), the banner is subject to the
same color treatment, keyed off `RunState`:

- **Idle/Completed/Failed/AwaitingInput** — each cell renders with its
  originally authored grayscale color, exactly as today.
- **Working** — each cell's color animates as a rainbow hue that travels
  left to right across the banner over time, modulated by that cell's
  *original* grayscale value as its luminance (HSL: hue advances with column
  position and elapsed time; lightness comes from the source gradient). The
  banner's existing shading/shape is preserved — only hue is added and moves
  — and it reverts to the static original the moment the run leaves
  `StateWorking`.

The animation ticks at a bounded, modest rate (on the order of 10 frames/sec,
not a real-time high frame rate) so a long-running agent task doesn't impose
continuous high-frequency rendering. It is a local rendering effect only —
like the existing spinner, it is not a session event and is not persisted.

**Foreground vs. background.** "Each cell renders" means different things
for the two grids, because they're made of different glyphs. The full grid's
`█` block characters fill their whole cell, so coloring the *foreground* is
enough — the glyph itself is the visible swatch. The collapsed row's glyphs
are ordinary letters and spaces, mostly empty space within their cell, so it
instead colors the cell *background* and forces the glyph to black — the
same "solid colored strip" result, reached the way that's actually visible
for text rather than block art. The gradient/hue's brightness (`V` in HSV,
or the gray level when static) runs from pure white at the first column down
to pure black at the last — and because the row is padded to the full panel
width (not just the label text), that dark end falls in the padding past the
visible text, so the text itself only ever sits in the bright portion and
stays legible against black lettering regardless of how long the label is.

```gherkin
GIVEN a freshly booted chat surface with the full logo banner set
WHEN the surface renders
THEN the banner occupies a fixed region above the output viewport
  AND the banner is not part of the output viewport's scrollable content

GIVEN an output panel whose applied transcript content height, measured
      against the output viewport height available under the full-size
      banner, exceeds one screenful
WHEN the panel next renders
THEN the banner collapses to a single row reading "AgentX - <activity>"

GIVEN a banner that has collapsed
WHEN the transcript later shrinks, or the terminal is resized taller
THEN the banner remains collapsed for the rest of the session

GIVEN a collapsed banner
WHEN the run's state.RunState/state.Phase changes
THEN the collapsed row's label updates to match (see the mapping table above)

GIVEN the run state transitions to StateWorking
WHEN the banner (full or collapsed) next renders
THEN each cell's color animates as a left-to-right traveling rainbow hue,
     modulated by that cell's original grayscale luminance, at a bounded
     tick rate

GIVEN the run state leaves StateWorking (Completed, Failed, Idle, or
      AwaitingInput)
WHEN the banner next renders
THEN the banner returns to its static, original (non-animated) coloring

GIVEN an output panel sized narrower than the active banner's widest line
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
- It is **collapsed by default** to keep the bootstrap view clean; the header shows
  the endpoint and an expand hint, and expanding reveals a numbered list of the
  launchable surfaces, each shown as `<digit> <status> <name>` (e.g. `1 🔴 context`,
  `2 🟢 files`) plus a copy hint. The **status emoji** is 🟢 when at least one surface
  of that kind is currently attached and 🔴 otherwise; it updates live as surfaces
  attach and detach (see Connection status below).
- It is **surface-local**, injected via `SetLaunchInfo` at startup — it is *not* a
  session event: it is never persisted to the event log and never appears on attached
  peer surfaces (which render only bus events). It exists solely on the hub chat
  surface that knows how to launch peers.
- It is omitted entirely when the transport is disabled (no endpoint).

Copying / typing the command. The attach command is the **short** form
`agentx surface launch <kind>` — the peer resolves the endpoint and token from the
session directory on disk (SS-5), so no token appears in the command or on screen.
Because it is short and unwrapped, it can be **cleanly selected over SSH in any
terminal** (the original motivation: a wrapped, bordered command scraped border
characters). As a convenience where supported, with the launch-info widget selected,
pressing a **digit `1..N`** also copies that surface's command to the system clipboard
via the terminal's **OSC 52** sequence (`tea.SetClipboard`), and the widget body
confirms by name. OSC 52 is terminal-dependent (VTE-based terminals such as GNOME
Terminal/Terminator ignore clipboard writes), which is why the short, selectable
command — not the clipboard — is the load-bearing path. For exactly that reason the
expanded body ends with a **manual-invocation footer**
(`or run in another pane:  agentx surface launch <name> --session <this-session>`) so a
user whose terminal drops the clipboard write can just type the command, substituting a
listed name. The header and footer name the session because more than one agentx
session may be running, and the launcher disambiguates on `--session` (SS-5). The widget reuses the standard
collapsible-widget machinery (selectable, Enter toggles, body capped/scrolled), so it
shifts selection/scroll exactly as a normal widget.

Connection status. Each row carries a presence indicator so the user can see, at a
glance, which peer surfaces are live. "Connected" is defined by an **active event
stream**, not merely a registration: a surface is green only while it holds an open
SSE subscription (`GET /events?surface_id=…`). The orchestrator marks the surface
live when its stream opens and dead when it closes — which also covers a crashed or
killed surface, since its TCP connection drops and the stream ends. The chat surface
polls the orchestrator's connected-kinds snapshot on a short interval (~1s) and
re-renders the row emojis; it never blocks rendering on the network. See
`docs/implementation/02_surface_orchestration_http.md` (Connection liveness).

```gherkin
GIVEN an output panel with launch info set for 2 surface kinds
WHEN the panel renders before any event is applied
THEN the launch-info widget is the first widget and is collapsed

GIVEN an output panel with launch info set
WHEN the launch-info widget is expanded
THEN it lists each launchable surface by name with a status emoji
AND it does not render any attach command or token

GIVEN an output panel with launch info set and no surface attached
WHEN the launch-info widget is expanded
THEN every surface shows the disconnected (🔴) status

GIVEN an output panel with launch info set
WHEN a surface kind reports connected
THEN that surface's row shows the connected (🟢) status
AND the other surfaces stay disconnected (🔴)

GIVEN an output panel with launch info set and the widget selected
WHEN the user presses a digit for a listed surface
THEN that surface's attach command is copied to the clipboard
AND the widget confirms the copied surface by name

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
