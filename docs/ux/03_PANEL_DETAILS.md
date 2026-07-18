# AgentX — Panel Details

> **Rewritten 2026-07-12.** This document previously specified a single-window
> Tkinter split-pane GUI (PD-01…PD-18, 112 affordances) that no longer exists in
> this codebase. It has been rewritten from scratch against the current
> **client-server** implementation on the `bubbletea` branch: a **chat surface**
> (output panel + input panel, launched together by `agentx`) plus a set of
> independently launchable **system surfaces** that attach over HTTP/SSE
> (`internal/surfaces/registry.go`). Nothing below describes Tkinter, Tcl
> bindings, or the old tabbed system panel — see the **Retired affordances**
> table at the end if you followed a `PD-xx` reference here from another doc
> (e.g. `UX_LIFECYCLE.md`) and it no longer resolves to a current section.

_Last updated: 2026-07-12._

Detailed affordance specifications for each current surface. Each section
documents the surface's purpose, real keybindings/controls as implemented, and
the package that owns them.

Authoritative contract rule (unchanged from the prior version of this doc):

- This document defines required user-facing affordances independent of delivery
  technology.
- Implementations may be GUI, TUI, or hybrid, but must satisfy these UX
  behaviors without weakening the contract.

Where a surface already has its own deep-dive spec (the output panel's widget
chrome, working memory's scroll/pin model, etc.), this document gives the
panel-level summary and points there rather than duplicating it — same as
before.

---

## Architecture overview

AgentX is a client-server app. The core server holds session state and routes
LLM inference; surfaces are separate client processes that attach to it. There
are two kinds:

- **The chat surface** — launched in-process together with the server by
  `agentx`. Two fixed, screen-pinned regions: an **output panel** (top, fills
  remaining space) and an **input panel** (bottom, sized to its content). A
  one-line **status bar** and a context-sensitive **hint row** sit between
  them; a third region (the **approval widget**) appears between output and
  input only while the run is awaiting an interactive decision. See
  `internal/surfaces/chat/chat.go` (`relayout`, `View`).
- **System surfaces** — independent processes, each launched separately
  (`agentx surface launch <kind>`) and attaching with an ephemeral token. The
  registry (`internal/surfaces/registry.go:47-53`) knows seven kinds — `chat`
  plus six external ones:

  | Kind | Package | Status |
  |------|---------|--------|
  | `context` | `internal/surfaces/context` | ✅ Implemented — [PD-CTX](#pd-ctx--context-surface-tui) |
  | `context-visualizer` | `internal/surfaces/contextviz` | ✅ Implemented — [PD-CTXVIZ](#pd-ctxviz--context-visualizer-tui) |
  | `working-memory` | `internal/surfaces/workmemory` | ✅ Implemented — [PD-WM](#pd-wm--working-memory-editor-tui) |
  | `files` | — | 📝 Registered, not yet implemented — [PD-FILES](#pd-files-registered-not-yet-implemented) |
  | `config` | — | 📝 Registered, not yet implemented — [PD-CONFIG](#pd-config-registered-not-yet-implemented) |
  | `context-history` | — | 📝 Registered, not yet implemented — [PD-CTXHIST](#pd-ctxhist-registered-not-yet-implemented) |

  The registry is open-ended: new kinds attach without changing existing
  surfaces. There is no tabbed "system panel" anymore — each surface is its
  own process, arranged by the user via a terminal multiplexer.

  There is also a checked-in **`ax` launcher** (`/ax`, repo root) that boots
  all of the above together: it mints a session name and runs
  `zellij --layout ~/.config/agentx/agentx.kdl`, whose tracked source is
  `config/seed/agentx.kdl` — currently three tabs (`agentX`: chat plus a
  `context`/`context-visualizer`/`working-memory` pane column; `editor`;
  `terminal`). [PD-LOGS](#pd-logs--logtrace-surface-proposed) proposes adding
  a fourth tab here.

---

## PD-01: Output Panel (chat surface)

**Purpose**: Fixed top region of the chat surface; streams the conversation —
user prompts, thinking, tool calls/results, assistant responses, plan
execution, errors, and system notices — as a scrollable transcript.

**Package**: `internal/surfaces/output`. **Authoritative spec**:
[`06_OUTPUT_WIDGET.md`](06_OUTPUT_WIDGET.md) — every entry type, its collapse
behavior, the box-border/inner-scroll/scrollbar chrome, the canonical emoji
set, the nested Plan widget, the pinned/collapsible/animated logo banner, and
the launch-info widget are all specified there in full GIVEN/WHEN/THEN detail.
This section is the panel-level summary only.

| Affordance | Where specified |
|------------|------------------|
| Collapsible entries (user/thinking/tool-call/tool-result/assistant/error/notice), one-line header + bounded scrollable body | `06_OUTPUT_WIDGET.md` §Anatomy, §Canonical emoji set |
| Plan widget (nested Step/Task tree, live status icons) | `06_OUTPUT_WIDGET.md` §Plan widget |
| Logo banner — pinned, content-based collapse to a one-row "AgentX - \<activity\>" label (`internal/surfaces/banner`), rainbow-wave animation while working | `06_OUTPUT_WIDGET.md` §Logo banner |
| Launch-info widget (attach commands for peer surfaces) | `06_OUTPUT_WIDGET.md` §Launch-info widget |
| Scroll/select keys (`j/k` scroll, `PgUp/PgDn` select, `Ctrl+O` expand) | hint row, `chat.go:629`; detailed in `06_OUTPUT_WIDGET.md` §Behaviour |

There is no clipboard context menu (the legacy Tkinter right-click "Copy"
popup, PD-01-AF-010, has no current equivalent) — the panel does not capture
the mouse, so copying text is the terminal emulator's native selection/copy,
outside the app entirely (see the "no mouse capture" preference this codebase
follows).

---

## PD-02: Input Panel (chat surface)

**Purpose**: Fixed bottom region of the chat surface; captures the next
prompt as free-text. During `state.StateAwaitingInput` the panel is inert and
the [approval widget](#pd-approval-approval-widget) takes over the same slot.

**Package**: `internal/surfaces/input`, wired through
`internal/surfaces/chat`.

### Submit, newline, and editing

| ID | Affordance | Trigger | Outcome |
|----|-----------|---------|---------|
| PD-02-AF-101 | Submit | `Enter` (buffer non-empty, not streaming) | Prompt is submitted; appended to in-memory history |
| PD-02-AF-102 | Soft newline | `Shift+Enter` (Kitty/modifyOtherKeys terminals only), or `Alt+Enter` / `Ctrl+J` (works on any terminal) | Inserts `\n` at the cursor |
| PD-02-AF-103 | Stop a running response | `Esc`, `Esc` (chord, only while streaming) | Sends the interrupt/stop action |
| PD-02-AF-104 | Backspace | `Backspace` | Deletes the rune before the cursor |

Source: `internal/surfaces/input/input.go:150-215` (`Update`). An empty input
shows a dim hint for whichever newline key applies, detected via
`tea.KeyboardEnhancementsMsg`.

There is **no attachment-chip system and no right-click clipboard context
menu** in the current implementation — both were Tkinter-only (legacy
PD-02-AF-005..012) and have not been carried forward. Paste is the terminal
emulator's native paste, outside the app.

### Prompt history seeding

> **TUI-native** (no Tkinter precedent). `↑`/`↓` *seed* the editable buffer
> with a prior submitted prompt for reuse — they copy it in, they don't edit
> the original. Submitting a seed (as-is or edited) creates a **new** history
> entry. Scope is the current process run only (in-memory).

| ID | Affordance | Trigger | Outcome |
|----|-----------|---------|---------|
| PD-02-AF-013 | History prev seeds the input | `↑` while input-focused, idle | Buffer replaced with the previous (older) submitted prompt; the in-progress draft is stashed on the first step back |
| PD-02-AF-014 | History next walks toward the present | `↓` while input-focused, idle | Buffer replaced with the next (newer) prompt; stepping past the newest restores the stashed draft |
| PD-02-AF-015 | Boundary flash | `↑` past the oldest prompt, or `↓` past the restored draft | Buffer unchanged; input frame flashes to signal the boundary |
| PD-02-AF-016 | Esc,Esc clears a seed | `Esc` then `Esc` while idle with a seed active | Buffer returns to empty (unseeded); history navigation resets to the present |

```gherkin
GIVEN the input has submitted prompts "first" then "second", buffer empty
WHEN  the user presses up twice
THEN  the input value goes "second" then "first"

GIVEN the input has submitted prompt "first" and the user typed "draft"
WHEN  the user presses up
THEN  the input value is "first" (the draft is stashed, not lost)

GIVEN the input has submitted prompt "first", value is "first" (one step back)
WHEN  the user presses up again
THEN  the value stays "first" and a history-boundary is reported
```

Note: `Esc` is heavily overloaded (history-clear when idle with a seed active,
vs. the interrupt chord while streaming, PD-02-AF-103) — the two conditions
(`streaming` vs. idle-with-seed) are mutually exclusive so this is unambiguous
at the input model, but worth knowing when reading `Update()`.

### Cursor & line editing

> **TUI-native** (no Tkinter precedent). Typing inserts at the cursor;
> Backspace deletes the rune before it. The buffer is one logical line
> (embedded newlines are ordinary characters), so `Ctrl+A`/`Ctrl+E` address the
> whole buffer. A *word* is a maximal run of non-space runes.

| ID | Affordance | Trigger | Outcome |
|----|-----------|---------|---------|
| PD-02-AF-017 | Left moves the cursor back one rune | `←` | Cursor index decremented, floored at 0 |
| PD-02-AF-018 | Right moves the cursor forward one rune | `→` | Cursor index incremented, capped at buffer length |
| PD-02-AF-019 | Jump to buffer start | `Ctrl+A` | Cursor index set to 0 |
| PD-02-AF-020 | Jump to buffer end | `Ctrl+E` | Cursor index set to buffer length |
| PD-02-AF-021 | Jump to start of prior word | `Alt+B` or `Ctrl+←` | Cursor moves left over spaces then the word, landing on its start |
| PD-02-AF-022 | Jump to start of next word | `Alt+F` or `Ctrl+→` | Cursor moves right over the current word then spaces, landing on the next word's start |
| PD-02-AF-024 | Cursor rendered while focused | panel focused | Reverse-video cell marks the cursor; absent when blurred |

**Multiplexer-safe aliases**: zellij binds `Alt-F` to toggle floating panes
and intercepts it before the app sees it, so word motion is also bound to
`Ctrl+←`/`Ctrl+→`, which zellij does not grab by default.

History seeding (PD-02-AF-013/014) places the cursor at the end of the seeded
text so the user can keep typing immediately.

---

## PD-STATUS: Status Bar (chat surface)

**Purpose**: One-line processing-state indicator between the output and input
panels. Replaces the legacy Tkinter `StatusTab` (PD-12) — there is no donut
chart, phase stepper, color-key legend, or dedicated interrupt button in the
current implementation; those pieces either moved elsewhere (the context
budget meter is now the separate [context-visualizer surface](#pd-ctxviz--context-visualizer-tui),
PD-10/PD-12's re-authored replacement) or don't exist (no per-phase stepper —
run state is the coarser `state.RunState` enum, not classify/think/tool/respond
steps).

**Package**: `internal/surfaces/chat/chat.go:665` (`statusBar`).

| ID | Affordance | Behavior |
|----|-----------|----------|
| PD-STATUS-AF-001 | State marker + label | Renders `<marker> <state>[ · <phase>]` left-aligned, padded to panel width with `─` |
| PD-STATUS-AF-002 | Marker reflects state | `○` idle; spinner glyph (falls back to `●`) while `StateWorking`; `●` otherwise (completed/failed/awaiting-input) |
| PD-STATUS-AF-003 | Interrupt | No dedicated button — `Esc`, `Esc` while streaming (PD-02-AF-103) stops the run; the hint row (`chat.go:615`) shows `esc → interrupt` / `esc again to confirm interrupt` |

---

## PD-APPROVAL: Approval Widget

**Purpose**: Generic interactive-decision prompt, swapped into the input
panel's slot in place of free-text entry whenever the run reaches
`state.StateAwaitingInput` — tool-execution approval, verb-continuation
approval, or any future decision kind, all rendered the same way (it never
hardcodes a per-kind option vocabulary). No legacy Tkinter equivalent — this
is a current-era addition.

**Package**: `internal/surfaces/approval/approval.go` (package doc comment,
lines 1-10).

| ID | Affordance | Trigger | Outcome |
|----|-----------|---------|---------|
| PD-APPROVAL-AF-001 | Navigate options | `↑`/`k`, `↓`/`j` | Highlighted-row cursor moves (`approval.go:79,83`) |
| PD-APPROVAL-AF-002 | Confirm | `Enter` | Highlighted option is chosen (`approval.go:87`) |
| PD-APPROVAL-AF-003 | Input stays visible, inert | decision pending | The approval widget renders as a third bordered region between output and input; input itself is not swapped away, so there's never a frame with neither visible (`chat.go:492-495`) |

The resulting decision and the pre-exec/approval/abort lifecycle it's part of
are specified in `06_OUTPUT_WIDGET.md` (the `kindApprovalRequest` /
`kindApprovalDecision` scrollback widgets).

---

## PD-WM — Working-Memory Editor (TUI)

> **TUI surface (M2, SS-6).** The first **read-write** peer surface, launched as a
> separate process (`agentx surface launch working-memory`). It lists the session's
> working-memory facts and lets the user curate what folds into the agent's context.
> It re-authors, for the TUI, the legacy GUI working-memory affordances (retired
> PD-14, PD-03 Working Memory section). See
> `docs/implementation/02_surface_orchestration_http.md` (Working-Memory CRUD SS-6).

### Behaviour

Working memory is a document (`working_memory.json`), not an event stream, so the
surface reads on attach (`GET /working-memory`), polls (~2s) for live refresh, and
mutates through dedicated token-gated endpoints. Each fact renders as
`<cursor> <●/○> key = value` (● enabled / ○ disabled; agent-owned facts are tagged).
Mutations persist and take effect on the **next** prompt's assembled context (only
enabled facts fold in). It is read-write but single-purpose: no prompt input.

#### Scroll & Collapse

The fact list is hosted in a scrollable viewport (`bubbles/v2/viewport`, the same primitive
`PD-01`'s TUI output panel uses — `docs/ux/06_OUTPUT_WIDGET.md`), with the
rightmost column reserved as a transcript-scrollbar gutter shown whenever facts
overflow the panel height. This surface fully mirrors the output panel's
established scroll/collapse taxonomy rather than inventing a new one, **including
its keybinding split** — `↑/↓`/`j`/`k` are repurposed from cursor movement (their
prior WM-only meaning) to **inner-scrolling the selected fact's value**, and
`PgUp`/`PgDn` become the cursor-movement keys (moving the selection exactly one
fact per press, matching the output panel's own `PgUp`/`PgDn` → `SelectUp`/
`SelectDown` behavior, not a true page jump). This is a **breaking change** to
this surface's previously-shipped `↑/↓` cursor binding, chosen deliberately for
cross-surface consistency over preserving the old binding.

A fact whose value word-wraps to more than one line (the `tree .`/multi-line
case this fixes) renders **collapsed by default**: only its first wrapped line
plus a `… (+N lines)` hint. Pressing **Enter** on the selected fact toggles it
between collapsed and expanded (a no-op on a single-line fact); expanded, the
value is capped to a per-fact row budget with its own scrollbar and inner-scroll
via `↑/↓`/`j`/`k`, exactly like an over-cap output-panel widget body. Moving the
selection (`PgUp`/`PgDn`) auto-scrolls the outer viewport to keep the newly
selected fact visible, the same `scrollSelectedIntoView` behavior the output
panel already has — no separate "jump to selection" key is needed. The owner/pin
annotation (` (agent)` / ` (pin ▶ live, 4s)`) always stays anchored to the fact's
first rendered row, never buried inside a wrapped or windowed body.

#### Pin (static / live)

A **pinned** fact (`Owner == pin`) is one created by the context surface's Pin
affordance (`p` on a selected tool-result, PD-CTX-AF-012) rather than typed by
hand — the durable, curated counterpart to the context surface's plain
enable/disable (PD-CTX-AF-011, which is session-scoped and applies to the exact
past event, not a copy). A pinned fact carries its source tool + args, so it can
be **re-run**, and one of two states:

- **static** (default at pin time): a frozen snapshot. The value never changes
  until the user edits or deletes it, exactly like a hand-typed fact.
- **live**: re-run before every turn's context assembly (`refreshLiveFacts`), so
  the value is always current — the mechanism for something like `tree .` or
  `date` that should never go stale. Only a tool that currently evaluates to
  policy `Allow` (no blacklist hit, no pending approval) can be set live; this
  prevents an unattended re-run of something that would otherwise need a human's
  sign-off, and avoids interrupting a turn on an approval prompt.

**Toggle key `l`** flips a pinned fact between live (▶) and static (⏸) — the
play/pause affordance — and immediately re-runs it once when switching to live,
so the action visibly does something rather than waiting for the next turn. It is
a no-op on a non-pinned fact. Each pinned fact's row shows its state and age
(`▶ live, 4s` / `⏸ static, 2m`) — `Age()` is measured from the last successful
refresh (live) or from when it was pinned (static), giving the user (and the
model, via the same annotation folded into the assembled context — see
`docs/implementation/03_configuration_and_storage.md`) a sense of how current the
value is.

**Unpinning** is the existing delete affordance (`d`) — no separate command. It
removes the WM fact only; it does **not** re-enable the source context element
(PD-CTX-AF-011/012) — that stays as the user left it. If the content is wanted
back in context, the user re-enables the original element directly in the
context surface.

**Value-sourced pins are static-only (ADR 0012 amendment).** A pin created from
a plan Step's own resolved value (PD-CTX-AF-014 — a decomposition engine's
synthesized fact, not a tool call, e.g. a wavefront Know) has no `ToolSource` at
all: there is nothing to re-run. It is a normal pinned fact in every other way
(editable, deletable, shows age from `PinnedAt`), but pressing `l` on it is a
no-op — the same refusal PD-WM-AF-009 already applies to a tool that isn't
currently policy-`Allow`, extended to the case of no tool at all.

### Affordance Inventory

| Affordance | ID | Status |
|-----------|-----|--------|
| List facts with enabled/disabled markers | PD-WM-AF-001 | ✅ |
| Navigate the selection cursor (`PgUp`/`PgDn`, one fact per press) | PD-WM-AF-002 | ✅ (was `↑/↓`/`j`/`k` — see PD-WM-AF-010) |
| Toggle enable/disable (space) | PD-WM-AF-003 | ✅ |
| Delete the selected fact (d) — also the unpin affordance for a pinned fact | PD-WM-AF-004 | ✅ |
| Add a fact (a → `key=value`, enter) | PD-WM-AF-005 | ✅ |
| Edit the selected value (e) / cancel (esc) | PD-WM-AF-006 | ✅ |
| A pinned fact's row shows its static/live state and age | PD-WM-AF-007 | ✅ |
| Toggle a pinned fact live/static (`l`), refreshing once immediately on live | PD-WM-AF-008 | ✅ |
| Setting a fact live is refused when its source tool is not currently policy-`Allow` | PD-WM-AF-009 | ✅ |
| Inner-scroll the selected fact's value (`↑/↓`, `j`/`k`) | PD-WM-AF-010 | ✅ |
| Expand/collapse the selected fact's multi-line value (Enter) | PD-WM-AF-011 | ✅ |
| Outer viewport auto-scrolls to keep the selection visible | PD-WM-AF-012 | ✅ |
| Transcript-style scrollbar in the reserved right gutter when facts overflow | PD-WM-AF-013 | ✅ |
| Setting a fact live is refused when it has no tool source at all (a plan-node value pin, ADR 0012 amendment) | PD-WM-AF-014 | ✅ |

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: A value-sourced pin can never go live (PD-WM-AF-014)

- GIVEN a working-memory fact pinned from a plan Step's own resolved value
  (PD-CTX-AF-014), which carries no `ToolSource`
- WHEN the user selects it and presses `l`
- THEN nothing happens — the same refusal PD-WM-AF-009 applies to a non-`Allow`
  tool, applied here because there is no tool at all to re-run

Use-case: A multi-line value collapses by default (PD-WM-AF-011)

- GIVEN a working-memory surface with a fact whose value word-wraps to more than
  one line (e.g. a pinned `tree .` snapshot)
- WHEN the fact list renders
- THEN the fact shows only its first wrapped line plus a `… (+N lines)` hint,
  and its owner/pin annotation stays on that first line

Use-case: Expand/collapse the selected fact (PD-WM-AF-011)

- GIVEN a working-memory surface with a collapsed multi-line fact selected
- WHEN the user presses Enter
- THEN the fact expands to show its full value, capped to a per-fact row budget
  with its own scrollbar when the value exceeds it
- WHEN the user presses Enter again
- THEN the fact re-collapses to its first-line preview

Use-case: Enter on a single-line fact is a no-op (PD-WM-AF-011)

- GIVEN a working-memory surface with a single-line-value fact selected
- WHEN the user presses Enter
- THEN nothing changes (there is nothing to expand)

Use-case: Inner-scroll an expanded, over-cap fact (PD-WM-AF-010)

- GIVEN a working-memory surface with an expanded fact whose value exceeds the
  per-fact row budget
- WHEN the user presses `↓`/`j`
- THEN the fact's body window scrolls down by one row and a scrollbar thumb
  reflects the new position

Use-case: Cursor movement auto-scrolls the outer list (PD-WM-AF-002 / PD-WM-AF-012)

- GIVEN a working-memory surface with more facts than fit in the panel height,
  and the selection at the bottom edge of the visible window
- WHEN the user presses `PgDn`
- THEN the selection moves to the next fact and the outer viewport scrolls just
  enough to keep it visible

Use-case: Transcript scrollbar reflects overflow (PD-WM-AF-013)

- GIVEN a working-memory surface whose facts (rendered, with any expansions)
  exceed the panel height
- WHEN the fact list renders
- THEN the reserved right gutter column shows a proportional scrollbar thumb
- GIVEN all facts fit within the panel height
- WHEN the fact list renders
- THEN the gutter column is blank (no thumb)

### Deferred (later slices)

Inline edit→clone, multi-select action bar, synthesize-via-LLM, system-prompt row
toggle, and click-to-navigate (the remainder of retired PD-14).

---

## PD-CTX — Context Surface (TUI)

> **TUI surface (M2, SS-3).** A read-only peer surface launched as a separate
> process (`agentx surface launch context`) that attaches over the transport and
> mirrors the session. It supersedes, for the TUI, the retired GUI context
> affordances (PD-03 SystemSurface — Context, PD-08 ContextRenderer). See
> `docs/build-plan/06_system_surfaces_backlog.md`.

### Behaviour

The surface seeds from the durable event log on attach (the full prior session),
then resumes the live stream by cursor and appends new events (SS-1). It is a
**navigable summary**: every element renders **collapsed by default** (titled border
+ preview), so the surface reads as a scannable list of conversation elements, not a
full transcript — expand one with Enter to read it.

It deals only in **complete conversation elements**: it never receives the live
`agent_delta` stream (that is the chat window's job); an agent turn appears as one
finished `agent_response` element. Streaming is watched in the main window.

Its **primary affordance is enable/disable** (not read-only): selecting a
user-prompt, agent-response, tool-call, or tool-result element and pressing
**space** toggles whether that element participates in the agent's upcoming
context. The toggle is sent to the orchestrator, which applies it in memory
(effective on the next prompt) and persists it in the element's event file. Each
toggleable element carries an **enabled checkbox to the left of its role emoji** —
`[x]` when it is in context, `[ ]` when disabled (re-authoring the retired
PD-03-AF-007 message-enabled checkbox). The checkbox is deliberately independent
of the selection border, so navigation and context-membership read as separate
cues. Thinking/classification/system-prompt/approval elements are display-only
and not toggleable, so they carry no checkbox. A one-line processing-state
indicator sits at the bottom. Quitting (`Ctrl-C`/`q`) marks the surface stopped.

A 🔧 tool-call or 📋 tool-result element (the flat, untagged `single_tool`-cycle
kind — a call folded into a plan step's Task node is display-only, unaffected)
starts **unchecked**: tool output normally scopes to the turn that produced it, so
by default it does not carry forward. Checking it is the same enable/disable
toggle as a user/agent element — nothing special about the word — applied to a
content class that starts off: its text folds into every subsequent turn's
assembled context until it is unchecked again. This is a session-scoped, one-off
inclusion of *that specific past event*; it is **not** the durable/curated
mechanism (that's Pin, below). An enabled tool element's bytes are counted under
the visualizer's `tools` 🔧 band (PD-CTXVIZ), which is otherwise always zero.

A selected 📋 tool-result element (only `tool_result`, not `tool_call` — the
result is the useful content) also has a distinct affordance: **`p` pins it to
working memory** (PD-WM). Pinning copies the element into a durable WM fact and
disables the source element here (`SetEventEnabled(ordinal, false)`), so the same
output is never represented twice — once as a raw context element, once as a WM
fact. Pin is a one-way handoff *out of* the context surface: the copy it creates
is managed from the working-memory surface from then on (static/live, age,
delete-to-unpin — PD-WM), not here. Unpinning does not restore the source
element's checkbox; if the user wants the raw event back in context, they
re-check it manually. See PD-WM's "Pin" affordances for the full design —
enable/disable here is the session-scoped mechanism, Pin is the durable one, and
they are deliberately different features that happen to share a source event.

**Pinning inside a plan (ADR 0012 amendment).** A call folded into a plan step's
Task node stays display-only for the enable/disable checkbox above — that part is
unchanged — but its own result, and a Step's own resolved value (a decomposition
engine's synthesized fact, not a tool call — e.g. a wavefront Know), are pinnable
too, via a node-level cursor inside the plan widget rather than the flat checkbox
model: `Tab`/`Shift+Tab` move the cursor to the next/previous node while the plan
widget is selected (rendered as a `›` prefix on the active node's title — see
`06_OUTPUT_WIDGET.md` §"Wavefront plans, Step values, and convergence"), and `p`
pins whatever the cursor is currently on:

- On a Task (or command-resolved) node whose result has arrived, `p` pins it
  exactly like a flat tool-result — same durable fact shape, same disable-the-
  source behavior.
- On a Step with a resolved value and no tool call behind it, `p` builds the WM
  fact directly from the node's own goal/value (there is no source event to
  disable). See PD-WM's "Pin" section for why such a fact can never go live.
- On a node with nothing resolved yet, `p` is a no-op.

### Affordance Inventory

| Affordance | ID | Status |
|-----------|-----|--------|
| Seed render: durable history on attach | PD-CTX-AF-001 | ✅ |
| Live tail: resumed complete events append after the seed cursor | PD-CTX-AF-002 | ✅ |
| Every element collapsed by default (navigable summary) | PD-CTX-AF-003 | ✅ |
| Navigation keys (scroll, page, select, expand/collapse) | PD-CTX-AF-004 | ✅ |
| Processing-state line reflects state · phase | PD-CTX-AF-005 | ✅ |
| Enable/disable the selected element (space) → context inclusion | PD-CTX-AF-006 | ✅ |
| Enabled checkbox (`[x]`/`[ ]`) left of the emoji, independent of selection | PD-CTX-AF-007 | ✅ |
| User/agent/flat-tool-call/flat-tool-result elements toggle; others are display-only | PD-CTX-AF-008 | ✅ |
| Complete agent responses only (no live `agent_delta` stream) | PD-CTX-AF-009 | ✅ |
| Title strip (`context · <session>`) via the surface host | PD-CTX-AF-010 | ✅ |
| Enabling a tool-call/tool-result folds its text into every subsequent turn's context (not just the turn that produced it), until disabled | PD-CTX-AF-011 | ✅ |
| Pin (`p`) a selected tool-result to working memory; disables it here (PD-WM) | PD-CTX-AF-012 | ✅ |
| `Tab`/`Shift+Tab` move the node-level pin cursor inside a selected plan widget | PD-CTX-AF-013 | ✅ |
| `p` pins the plan-node cursor's current node (Task result or Step value) to working memory (ADR 0012 amendment) | PD-CTX-AF-014 | ✅ |

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Seed then live (PD-CTX-AF-001 / PD-CTX-AF-002)

- GIVEN a session with a recorded exchange
- WHEN a context surface attaches
- THEN it renders the prior exchange as collapsed elements, and complete events
  stream in thereafter

Use-case: Enable/disable an element (PD-CTX-AF-006)

- GIVEN a context surface with a selected agent-response element
- WHEN the user presses space
- THEN the element's checkbox flips (`[x]`→`[ ]`) and the toggle is sent to the
  orchestrator (excluded from the next assembled context)

Use-case: Non-toggleable element (PD-CTX-AF-008)

- GIVEN a context surface with a selected thinking element
- WHEN the user presses space
- THEN nothing is toggled (thinking never enters context)

Use-case: Enable a tool-result element (PD-CTX-AF-011)

- GIVEN a context surface with a selected, unchecked tool-result element from an
  earlier turn (e.g. a `tree .` listing)
- WHEN the user presses space
- THEN the checkbox flips to `[x]` and the toggle is sent to the orchestrator; the
  result's text folds into the assembled context on the next prompt and every
  prompt after, until the element is unchecked again

Use-case: Pin a tool-result to working memory (PD-CTX-AF-012)

- GIVEN a context surface with a selected tool-result element
- WHEN the user presses `p`
- THEN a working-memory fact is created from the element's text and the source
  element's checkbox is unchecked (excluded from the next assembled context) — the
  content now folds into context through the WM fact instead

Use-case: Collapsed by default (PD-CTX-AF-003)

- GIVEN a context surface
- WHEN any element arrives
- THEN it renders collapsed until the user expands it

Use-case: Node-level pin cursor navigation (PD-CTX-AF-013)

- GIVEN a context surface with a plan widget selected
- WHEN the user presses `Tab`
- THEN the pin cursor moves to the next node in the plan and its title shows the
  `›` prefix (no other node's does)
- WHEN the user presses `Shift+Tab`
- THEN the cursor moves to the previous node instead
- GIVEN any other element is selected (not a plan widget)
- WHEN the user presses `Tab` or `Shift+Tab`
- THEN nothing happens

Use-case: Pin a plan node's result or value (PD-CTX-AF-014, ADR 0012 amendment)

- GIVEN a plan widget is selected and the pin cursor is on a Task node whose
  result has arrived
- WHEN the user presses `p`
- THEN a working-memory fact is created from the node's result, exactly like
  pinning a flat tool-result
- GIVEN the pin cursor is instead on a Step node with a resolved value and no
  backing tool call (e.g. a wavefront Know)
- WHEN the user presses `p`
- THEN a working-memory fact is created directly from the node's goal/value —
  there is no source event to disable
- GIVEN the pin cursor is on a node with nothing resolved yet
- WHEN the user presses `p`
- THEN nothing happens

---

## PD-CTXVIZ — Context Visualizer (TUI)

> **TUI surface (M2, SS-7).** A read-only peer surface launched as a separate
> process (`agentx surface launch context-visualizer`) that polls the assembled
> context window's composition and renders it as a budget meter. It re-authors the
> retired GUI ContextMeterWidget (PD-10) and ContextKeyWidget (PD-12) for the TUI.
> See `docs/build-plan/06_system_surfaces_backlog.md`.

### Behaviour

The surface polls `GET /context` (every 2 s) for a per-content-class breakdown of
the standing context window and draws one bar per class in the retired PD-10's
band order, using the app's content emoji so it reads consistently with the output
widgets: working memory 🧠, instructions 📜, user 👤, attachments 📎, thinking 💭,
assistant 🤖, tools 🔧. A remaining-capacity ghost band (`░`) and a total line
complete the meter, all measured against the model's context window — read from
Ollama's `/api/show` (`<architecture>.context_length`), the same window the
runtime requests as `num_ctx`. Token figures are a `chars ÷ 4` estimate (Ollama
exposes no universal local tokenizer), labelled "est.". When the model reports no
context length the meter drops the percentages and the ghost band.

It is **strictly read-only**: it holds an event stream only for connection presence
(SS-4) and performs no writes. The enable/disable-a-turn management affordance lives
on the context pane (PD-CTX); the meter only *hints* at what to prune. Classes not
yet fed into the assembled context (attachments, thinking today) render as zero
rather than being hidden, so the legend stays complete. `tools` renders as zero
too, until the context pane enables a tool-call/tool-result (PD-CTX-AF-011) —
from then on its bar reflects those bytes, same as any other class. A tool
element *pinned* to working memory (PD-CTX-AF-012 / PD-WM) instead counts under
`working-memory`, not `tools` — Pin hands the content off to WM entirely.
Quitting (`Ctrl-C`/`q`) marks the surface stopped.

### Affordance Inventory

| Affordance | ID | Status |
|-----------|-----|--------|
| Per-class bars in band order with content emoji | PD-CTXVIZ-AF-001 | ✅ |
| Bars sized against the model's context window | PD-CTXVIZ-AF-002 | ✅ |
| Remaining-capacity ghost band | PD-CTXVIZ-AF-003 | ✅ |
| Total line: est. tokens / window (percent) · model | PD-CTXVIZ-AF-004 | ✅ |
| Near-limit (≥80%) / full (≥100%) annotation | PD-CTXVIZ-AF-005 | ✅ |
| Graceful degrade when the window is unknown | PD-CTXVIZ-AF-006 | ✅ |
| Strictly read-only: no mutation affordance | PD-CTXVIZ-AF-007 | ✅ |
| Live refresh via poll (agent turns, WM edits appear) | PD-CTXVIZ-AF-008 | ✅ |

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Per-class meter (PD-CTXVIZ-AF-001 / PD-CTXVIZ-AF-002)

- GIVEN a context breakdown with working-memory and user contributions and a known window
- WHEN the visualizer renders
- THEN it shows a bar per content class and a remaining band against the window

Use-case: Window unknown (PD-CTXVIZ-AF-006)

- GIVEN a breakdown whose model reports no context length
- WHEN the visualizer renders
- THEN it shows "window unknown" and omits the percentages and ghost band

Use-case: Read-only (PD-CTXVIZ-AF-007)

- GIVEN a context visualizer
- WHEN a mutation key (e.g. `a`) is pressed
- THEN nothing changes — no editor opens; management is directed to the context pane

---

## PD-FILES: Registered, not yet implemented

`files` is a known launchable kind (`registry.go:48`), but no
`internal/surfaces/files` package exists — there is no implementation to
document. The retired Tkinter `FileBrowser` (PD-11) described a live GUI file
tree with nav buttons and right-click context menus; none of that carries
forward as-is. What *does* carry forward as a forward-looking, delivery-agnostic
target — because it was already written in TUI terms, not Tkinter terms — is
the parity contract for whatever eventually implements this surface:

| Requirement | Note |
|-------------|------|
| Large directory lists must remain navigable within the visible viewport; the selected row must stay visible while moving | not yet built |
| Accelerated navigation for long lists (`PageUp`/`PageDown`/`Home`/`End`) | not yet built |
| Deterministic overflow status (e.g. `showing X-Y of Z`) | not yet built |
| Arrow-key row navigation, `Space` soft-select, `Return` hard-select/activate | not yet built |

Attach-to-message (the retired PD-11 "Attach" context-menu item) has no
current mechanism either — recall PD-02's note that the input panel has no
attachment-chip system today.

---

## PD-CONFIG: Registered, not yet implemented

`config` is a known launchable kind (`registry.go:49`), but no
`internal/surfaces/config` package exists. Configuration today is a static
file, `agentx.toml` (project root; runtime copy under
`~/.config/agentx/`), hand-edited — `chat_backend`, `[agentx.theme]`,
`[agentx.ollama] host`/`model`, timeouts, applet port range, etc. There is no
in-app settings UI. The retired Tkinter `SettingsSurface` (PD-07) — live model
dropdown, restart-required tooltips, and so on — describes a design that does
not exist today and is not a confirmed target for this surface; treat it as
historical only.

---

## PD-CTXHIST: Registered, not yet implemented

`context-history` is a known launchable kind (`registry.go:51`), distinct from
[`working-memory`](#pd-wm--working-memory-editor-tui) (`registry.go:53`) —
these are two different registered surfaces, not two names for the same thing.
No `internal/surfaces/context-history` package exists yet. Earlier planning
(retired PD-18-AF-002) intended it to show ordered turn history with
deterministic truncation, but that was never built against the current
architecture, so it isn't restated here as settled behavior — a fresh spec is
needed before implementation.

---

## PD-LOGS — Log/Trace Surface (proposed)

> **Proposed, not yet built.** `logs` is not a known launchable kind —
> `internal/surfaces/registry.go:46-54`'s `knownKinds` map has exactly seven
> entries (`chat`, `files`, `config`, `context`, `context-history`,
> `context-visualizer`, `working-memory`), no `logs`. Nothing in the
> checked-in `ax` launcher (`config/seed/agentx.kdl`) opens a logs tab either.
> The retired Tkinter build did have a "Logs widget" (`UX_LIFECYCLE.md` §4,
> traceability row for `PD-17`/`e2e-logs-001`), but that was DemoMode's own
> diagnostics-capture artifact viewer (`docs/ux/07_DEMO_MODE.md`
> `PD-17-AF-006`), unrelated to reviewing session/runtime activity — there is
> no real precedent to re-author here. This section is a fresh spec, written
> in response to a request to let the user review backend functionality by
> browsing the current session's logs.
>
> **Implementation scoped**: `docs/build-plan/06_system_surfaces_backlog.md`
> Phase G (SS-8 host input-capture mode, SS-9 the logs surface itself).

### Purpose

Let the user review backend/session activity — every persisted `state.Event`
(`user_prompt`, `tool_call`, `tool_result`, `task_plan`, `task_node`,
`approval_request`/`approval_decision`, `thinking`, `classification`, etc.,
see `internal/state/event.go:9-55`) — as a searchable, scrollable, continuously
updating stream, instead of reading raw per-event JSON files by hand. The data
already exists: `internal/session/recorder.go` persists one JSON file per
event, append-only, under `<session-dir>/events/`, and `Recorder.Load()`
(`recorder.go:66-92`) already returns them in stable order. This surface is a
pure *read* layer on data that already exists — no event-model or persistence
work is required to build it.

### Required affordances

Per this document's own contract rule (line 22-25 above), these are specified
independent of delivery technology — see **Implementation approaches** below
for two ways to satisfy them.

| Affordance | ID |
|---|---|
| Full-tab placement — a dedicated zellij tab, not a shared pane column, so the view gets real screen real estate | PD-LOGS-AF-001 |
| Live streaming — new events appear as they're recorded, equivalent to `tail -f` | PD-LOGS-AF-002 |
| Incremental vi-style search (`/pattern`, `?pattern`, `n`/`N` to cycle matches) | PD-LOGS-AF-003 |
| Regex pattern support (vi/sed-style), not just literal substring matching | PD-LOGS-AF-004 |
| Matches are visually highlighted, not just jumped to | PD-LOGS-AF-005 |
| Line-wrapping — long lines wrap to the pane width; no horizontal scroll required | PD-LOGS-AF-006 |
| Vim-style jump to top (`gg`) / bottom (`G`) of the buffer | PD-LOGS-AF-007 |
| Strictly read-only — no affordance can write to, truncate, or reorder the underlying event files, and no escape hatch reaches a shell or an editor's save/write path | PD-LOGS-AF-008 |

### Implementation approaches considered

**A — CLI formatter piped into `less`, hosted as a plain zellij pane. Considered, not chosen for v1.**
Add an `agentx logs [--session <id>] [--follow]` subcommand that calls
`Recorder.Load()`, formats each `state.Event` as one line (e.g.
`epoch  content_type  [tool_name]  summary`), and either prints once or, with
`--follow`, watches the events directory for newly-written files and streams
them as they land. Run it as a fourth `config/seed/agentx.kdl` tab:
`agentx logs --session $AX_SESSION_STRING --follow | less +F -R`.

All eight affordances above fall out for free, because they're native `less`
behavior: `/pattern` / `?pattern` / `n` / `N` with highlighted matches, `g` /
`G` for top/bottom, wrapped lines by default (no `-S`), and `+F` for a
`tail -f`-equivalent follow mode that drops into ordinary paging the instant
the user presses a movement key. `less` has no write path at all, so
PD-LOGS-AF-008 is structural rather than something to enforce.

- Does **not** require a `registry.go`/`internal/surfaces` entry — it's a CLI
  subcommand plus a zellij pane, not an HTTP/SSE-attached surface.
- Caveat: `less` search regex is POSIX ERE-ish (via the system `regcomp`),
  close to but not byte-identical to `sed`'s BRE — fine for pattern search,
  just don't expect every `sed`-specific construct to carry over.
- **Rejected for v1**: `less`/zellij impose no governance over the pane once
  it's open. Pressing `q` (or any of `less`'s other exit paths) doesn't close
  a governed surface — it drops the user into a bare interactive shell,
  sitting in a tab labeled "logs," with no read-only boundary at all. That's
  a materially worse failure mode than anything option C's vim-escape-hatch
  concern raised: it's not a hard-to-lock-down editor, it's an *unrestricted*
  shell one keystroke away, indistinguishable from the rest of the ax layout.
- LOE: **Low** — one new CLI subcommand (reuses `Recorder.Load()` plus a small
  formatter and a directory-watch loop for `--follow`), one new tab in
  `config/seed/agentx.kdl`. Days, not weeks. Kept here as the cheap fallback
  if B's LOE proves too high in practice.

**B — Native Bubbletea surface (`internal/surfaces/logs`), matching `context`. Chosen for v1.**
A proper registered surface, and specifically a `client.SurfaceModel`
(`internal/surfaces/client`) — the same shared host framework `context`
already uses (SS-2/SS-3 in `docs/build-plan/06_system_surfaces_backlog.md`):
`Apply(state.Event)` folds each event into an internal line buffer,
`scrollutil` (`internal/surfaces/scrollutil` — already shared by `output` and
`workmemory`) supplies wrap/scrollbar math, and a hand-built `/pattern` search
overlay (Go `regexp`, highlighted matches) adds `n`/`N`, `gg`/`G`,
`ctrl-d`/`ctrl-u` paging on top. Live tail is natural since the host already
seeds from disk and resumes the session's live SSE stream by cursor — no new
transport work.

One real gap surfaced by scoping this: `client.Host` currently treats `"q"`
as an unconditional global quit before a key ever reaches the surface
(`client.go:155-161`), which would swallow `"q"` typed inside a search
pattern. Closing it is a small, isolated shared-framework change (see the
backlog's SS-8) — not a reason to reconsider option B, but worth flagging as
its own task rather than discovering it mid-implementation.

- Pro: consistent with the client-server surface model (this repo's core
  architecture — see `CLAUDE.md`), works without zellij or any multiplexer,
  and can render events with structural understanding (e.g. collapsing a
  `tool_call`/`tool_result` pair into one block) instead of one flat text
  line per event.
- Pro (deciding factor): the surface governs its own exit path. `q`/`Ctrl-C`
  stops the surface the same way it stops every other AgentX surface
  (`PD-CTXVIZ`'s "Quitting (`Ctrl-C`/`q`) marks the surface stopped" —
  `03_PANEL_DETAILS.md:598` — is the precedent to match). There is no key
  sequence that lands the user in a shell, because no shell is embedded — the
  process either renders the log view or exits cleanly. This is a strictly
  stronger read-only guarantee than option A's, not just a different one.
- Pro: room to grow deliberately — content-type filtering, per-class color
  (reusing the emoji/color vocabulary `PD-CTXVIZ` already established),
  collapsing tool call/result pairs, jumping straight to a specific
  correlation/task/node id — none of which `less` piping could ever offer,
  since it has no knowledge of `state.Event` structure.
- Con: reimplements search/highlight/navigation that `less` already provides,
  and Go's `regexp` (RE2) doesn't support backreferences, so "sed-style"
  search is *more* approximate here than in option A, not less.
- LOE: **Medium** — comparable to `PD-CTXVIZ`'s build, roughly 1-2 weeks for a
  solid v1, all net-new TUI code.

**C — Embed vim/neovim in a read-only mode inside the pane. Considered, not recommended.**
This was the "slick" option raised alongside the exact right concern: how do
you stop the user from saving, or exiting into a shell? That concern is why
it's disproportionate to pursue:

- Plain `vim -R` is not actually safe — `:w!` force-writes, `:!<cmd>` and
  `:r !<cmd>` reach an arbitrary shell, `Ctrl-Z` suspends. Locking it down
  needs restricted mode (`vim -Z -R -u NONE`) plus remapped `:q` / `:wq` /
  `ZZ`, and correctness then depends on those flags being reproduced exactly
  on every launch — a fragile safety boundary to maintain.
- A genuinely safe embedding means driving Neovim over its msgpack-RPC API
  (`nvim --embed`) as a library and translating grid updates into the pane's
  rendering — effectively building a small Neovim GUI client from scratch.
- Either path costs materially more than A or B while buying nothing A
  doesn't already provide — `less` already has vi-style search/navigation and
  zero write path by construction. Shelved as a "someday" idea, not a v1
  candidate.

### Recommendation

**Decision: ship B.** Option A's LOE advantage doesn't outweigh the gap it
leaves in PD-LOGS-AF-008: it can't stop `q` (or any other `less` exit key)
from surfacing a raw, unrestricted shell in a pane the user has every reason
to treat as inert output. A surface-owned pager closes that gap by
construction — there's no shell to fall through to — and buys room for
structure-aware rendering and filtering that piping flat text through `less`
never could. Option A stays documented above as the cheap fallback if B's
LOE turns out to be materially higher than estimated. Option C remains
shelved per the reasoning above.

`logs` becomes an eighth entry in `internal/surfaces/registry.go`'s
`knownKinds`, gets an `internal/surfaces/logs` package following the
`contextviz` package's shape (HTTP/SSE attach, `agentx surface launch logs`),
and — same as `context`/`context-visualizer`/`working-memory` today — the
user places it in their own zellij tab via `config/seed/agentx.kdl` (a
dedicated fourth tab, per PD-LOGS-AF-001) rather than the surface owning tab
placement itself.

### Open questions

- **OQ-LOGS-02** — Verbosity of one log line per event: full JSON payload
  inline, a truncated summary with a way to expand, or per-content-type
  formatting (e.g. `tool_call` shows `tool_name` + args, `thinking` shows the
  first N characters)?
- **OQ-LOGS-03** — Live-tail transport: the session's existing SSE event
  stream (consistent with how `context` seeds+subscribes today) versus a
  poll against `Recorder.Load()` (simpler, matches `context-visualizer`'s 2 s
  poll, but doesn't reuse the live bus)?
- **OQ-LOGS-04** — Should the vim-style search/nav keys (`/`, `?`, `n`, `N`,
  `g`, `G`, `ctrl-d`/`ctrl-u`) be verified against the project's zellij
  keymap audit (`docs/reference/zellij/options.md`)? That audit only checked
  AgentX's existing bindings (`Alt+f`, `Ctrl+o`) for collisions — low risk
  (none are zellij default-mode shortcuts) but unconfirmed for a new set of
  bindings.
- **OQ-LOGS-05** — Scope of "eventual customization" for v1 versus later:
  candidates are content-type filtering/toggling (reusing the
  `DefaultEnabled`-adjacent on/off vocabulary from `PD-CTX`), per-class color
  keyed to `PD-CTXVIZ`'s emoji legend, and jump-to-correlation/task/node-id.
  None of these block a v1 ship; flagged so `internal/surfaces/logs` is
  structured (e.g. one `state.Event` → one renderable row, not a
  pre-flattened string) to make adding them later cheap rather than a rewrite.

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Full-tab placement (PD-LOGS-AF-001)

- GIVEN the user launches `ax`
- WHEN the zellij session opens
- THEN a dedicated "logs" tab exists, sized to the full terminal, alongside `agentX`/`editor`/`terminal`

Use-case: Live streaming (PD-LOGS-AF-002)

- GIVEN the logs tab is open and scrolled to the bottom of the stream
- WHEN a new session event is recorded (e.g. a tool call)
- THEN the new line appears without the user taking any action, same as `tail -f`

Use-case: Search and highlight (PD-LOGS-AF-003 / PD-LOGS-AF-005)

- GIVEN a logs buffer containing multiple `tool_result` lines
- WHEN the user types `/tool_result` and presses Enter
- THEN the view jumps to the next match and every match in the buffer is visually highlighted

Use-case: Line wrap (PD-LOGS-AF-006)

- GIVEN a line longer than the pane's width (e.g. a large `tool_result` payload)
- WHEN it renders
- THEN it wraps onto additional visual lines rather than being cut off or requiring horizontal scroll

Use-case: Vim-style jump (PD-LOGS-AF-007)

- GIVEN the user is mid-buffer
- WHEN they press `g` twice (`gg`) or `G`
- THEN the view jumps to the first or last line of the buffer respectively

Use-case: Read-only (PD-LOGS-AF-008)

- GIVEN the logs surface
- WHEN the user presses `q` or `Ctrl-C`
- THEN the surface process stops cleanly (same contract as `PD-CTXVIZ-AF-007`) — no shell, editor, or write path is ever reachable, because none is embedded

---

## Retired affordances

Legacy `PD-xx` IDs from the prior version of this document that have no
current-implementation equivalent and are not restated above. Listed so a
reference from elsewhere (e.g. `UX_LIFECYCLE.md`'s traceability matrix, which
has not yet been reconciled to this rewrite) has somewhere to land.

| Retired ID | Was | Disposition |
|------------|-----|--------------|
| PD-03 | SystemSurface (tabbed system panel) | Gone — replaced by independent surface processes (see Architecture overview) |
| PD-04 | ModelSelector (live dropdown) | Gone — model is a static `agentx.toml` field, no runtime UI |
| PD-05 | PlanView (plan tab in OutputSurface) | Superseded — Plan widget lives inside the output panel; see `06_OUTPUT_WIDGET.md` §Plan widget |
| PD-06 | ResynthesisDialog (modal) | Gone — zero references anywhere in `internal/` |
| PD-08 | ContextRenderer (Tkinter widget factory) | Superseded by [PD-CTX](#pd-ctx--context-surface-tui)'s actual behavior |
| PD-09 | CollapsibleSection (shared Tkinter widget) | Superseded — collapse/expand is now documented per-surface (output panel: `06_OUTPUT_WIDGET.md`; working memory: PD-WM-AF-011) |
| PD-10 | ContextMeterWidget (donut chart) | Superseded by [PD-CTXVIZ](#pd-ctxviz--context-visualizer-tui) |
| PD-11 | FileBrowser | Superseded by [PD-FILES](#pd-files-registered-not-yet-implemented) (parity requirements carried forward; Tkinter specifics dropped) |
| PD-12 | StatusTab (donut + phase stepper + interrupt button) | Split — status line is [PD-STATUS](#pd-status-status-bar-chat-surface); donut moved to PD-CTXVIZ; no phase stepper or dedicated interrupt button exist today |
| PD-13 | ToolPanel (per-session tool enable/disable) | Gone — no current equivalent |
| PD-14 | ContextPanelWidget (spec-only, never implemented) | Superseded by [PD-CTX](#pd-ctx--context-surface-tui); unported ideas listed under PD-WM's "Deferred" |
| PD-17 | DemoMode | Moved to [`07_DEMO_MODE.md`](07_DEMO_MODE.md) — note that doc is itself still mid-migration, not yet a fully reconciled current-state reference |
| PD-18 | SystemAppletSuite | Superseded by the surface-registry model itself (Architecture overview) and the individual surface sections above |
| PD-01-AF-010 | Output panel right-click "Copy" popup | Gone — no mouse capture; terminal-native selection/copy |
| PD-02-AF-001..012 | InputSurface Ctrl+Enter submit, attachment chips, right-click copy/paste popup | Gone/changed — see PD-02 above (submit is now plain `Enter`; no chips; no popups) |
