# Output Applet Contract

Last updated: 2026-06-11
Applet ID: `chat/output`
Runtime entry: `agentx-core --output-widget`

> **Canonical source of truth** for the Output applet UX/architecture contract.
> Cross-references in `docs/architecture.md` and
> `docs/architecture/agentx_tui_hybrid_architecture.md` point here; do not
> duplicate design decisions in those docs.

---

## Ownership Boundary

| Concern | Owner |
| --- | --- |
| Conversation turn data (turns, entries, content) | **Core** |
| tmux pane geometry (splits, resize, window layout) | **Core** |
| Pane-local view state (focus position, expand/compact per entry) | **Output applet** |
| Keymap / navigation input handling within the pane | **Output applet** |

The applet is a render consumer and interactive navigator; it does not own
prompt orchestration, tool execution state, or context lifecycle transitions.

---

## Default View Behavior

- **Latest-first, focus-expands**: on any new turn arriving, the widget
  automatically scrolls to and expands the newest turn; all prior turns are
  compacted to a one-line summary.
- **Newest-turn pointer is always visible**: the newest turn is marked with a
  persistent visual indicator (`LATEST`) plus a high-contrast focus style when
  selected so users can immediately identify the most recent response.
- **Compact non-focused entries**: turns and entries that are not focused show
  only a single summary line (role prefix + first ~60 chars).  The focused
  turn/entry is rendered in full.
- **Pane-level space reclamation** (e.g., hide pane, resize window) is a
  tmux/core concern and is outside the applet's responsibility.

---

## Navigation / Input Model

The output applet is an **interactive read-only navigator**; it accepts no
text input but handles the following keys:

All navigation keys below MUST be handled in single-keystroke mode with no
line buffering and no Enter confirmation requirement.

| Key(s) | Action |
| --- | --- |
| `↑` / `k` | Move focus to the previous (older) turn |
| `↓` / `j` | Move focus to the next (newer) turn |
| `Tab` | Drill into focused turn container (entry focus mode) |
| `Shift-Tab` | Drill out to focused turn container mode |
| `h` / `l` | Collapse/expand the focused target (turn in container mode, entry in entry mode) |
| `Enter` / `Space` | Toggle expand/compact on the focused target (turn or entry by mode) |
| `←` / `→` | Previous/next turn navigation (non-toggle, no drill conflict) |
| `PgUp` | Scroll up one page within the expanded focused entry |
| `PgDn` | Scroll down one page within the expanded focused entry |
| `Home` | Jump to the oldest turn |
| `End` | Jump to the newest turn (restores default latest-first view) |
| `?` | Toggle inline help overlay showing the keymap |
| `q` | Close / detach the output widget |

Navigation ergonomics preserve single-keystroke responsiveness while making
container-vs-entry intent explicit via drill in/out.

---

## Turn and Entry Rendering Contract

- A **turn** is one complete user↔assistant exchange (may contain multiple
  entries: prompt, tool calls, tool results, assistant reply).
- An **entry** is one atomic block within a turn (e.g., a single tool result).
- Compact form: `[role] <summary line>` — one terminal line maximum.
  For collapsed turn containers, the summary line is sourced from the first
  words of the user prompt (falling back to response only when prompt is empty).
- Expanded form: full content with syntax/markdown rendering where supported.
- The most recently received turn starts expanded; all others start compacted.
- Focus is two-level per focused turn:
  - container focus mode uses marker `↳` on the user row.
  - entry focus mode uses marker `▶` on the focused entry row.
- After manual expand/compact, the applet preserves the user-set state until
  navigation focus leaves that turn; returning focus restores the user-set
  state for that turn within the session.
- Expandable content must provide explicit affordances (`[+]` collapsed,
  `[-]` expanded) and remain keyboard discoverable via `Tab` and `?` help.
- Expanded container renders a visible box around turn entries.
- Collapsed container renders an empty box stub (top/bottom border) so users
  can still see container presence in compact mode.
- User prompt rows include a person emoji prefix (`👤 User:`) to improve
  role-scannability in dense output streams.

---

## UX Anchors

- `PD-01` ChatPanel output surface (`docs/ux/03_PANEL_DETAILS.md`)
- `PD-17-AF-011` startup greeting parity (`docs/ux/07_DEMO_MODE.md`)
- `PD-17-AF-012` prompt lifecycle parity (`docs/ux/07_DEMO_MODE.md`)
- Context interaction parity (**normative**) (`docs/architecture/applets/context_applet.md`)
- Context History visual language (**informative**) (`context_history.md`)
- Traceability row: runtime applet closure table (`docs/ux/UX_LIFECYCLE.md`)

---

## Owned Widget/Surface Inventory

- Output conversation render stream (assistant/user/system/tool lifecycle text blocks)
- Per-turn compact/expand state (pane-local, not persisted across sessions)
- Startup greeting render surface
- Prompt lifecycle rows rendered during a prompt cycle
- Inline keymap help overlay

---

## Affordance List

Status taxonomy used in this table:

- `Priority`: `Required` (release-gating contract requirement) or `Advisory` (non-gating improvement).
- `Implementation state`: `Built`, `Partial`, or `Not built`.

| ID | Affordance | Priority | Implementation state | Acceptance criteria |
| --- | --- | --- | --- | --- |
| `PD-17-AF-011` | Output startup surface omits boilerplate chrome lines (`[OUTPUT]`, `Chat ready.`, `Controls...`, `Clipboard...`) | Required | Not built | On fresh launch before the first user prompt, the first rendered output row is a lifecycle row or conversation row; none of the four forbidden boilerplate strings appears in any rendered output row. |
| `PD-17-AF-012` | Prompt lifecycle rows stream in deterministic order | Required | Built | For one prompt cycle, lifecycle rows are ordered consistently with core lifecycle events and do not reorder on redraw. |
| `OUT-AF-001` | Latest-first default: newest turn auto-expanded on arrival | Required | Not built | On new turn arrival, newest turn is expanded and focused; previously focused older turn becomes compact. |
| `OUT-AF-002` | Focus navigation with `j/k` / arrows across turns (single-key responsive) | Required | Partial | Each single `j/k` or arrow keypress causes exactly one focus move with no Enter requirement; key events are not dropped or coalesced. |
| `OUT-AF-003` | Per-entry collapse/expand with `h/l` / Enter / Space (single-key responsive) | Required | Partial | `h/l`, `Enter`, and `Space` each toggle only the focused region, and the changed expanded/collapsed state is visible in the first frame rendered after that key event. |
| `OUT-AF-004` | PgUp/PgDn scrolling within expanded entry (single-key responsive) | Required | Partial | `PgUp/PgDn` changes viewport position immediately while retaining focus and without requiring Enter. |
| `OUT-AF-005` | Home/End jump to oldest/newest turn | Required | Partial | `Home` lands on oldest visible turn; `End` lands on newest turn and restores latest-first focus state. |
| `OUT-AF-006` | Inline keymap help overlay via `?` | Required | Built | `?` opens and closes help overlay; overlay includes all supported navigation keys including `Tab`. |
| `OUT-AF-007` | Compact non-focused turns to preserve working space | Required | Built | Only focused turn is expanded by default; non-focused turns render one-line summaries unless manually expanded. |
| `OUT-AF-008` | `Tab` navigation between expandable regions in focused turn | Required | Not built | Repeated `Tab` cycles through expandable controls/regions predictably and wraps within the focused turn. |
| `OUT-AF-009` | Clear visual pointer to most recent response | Required | Not built | Newest response always displays `LATEST` marker; marker remains visible even when focus moves elsewhere. |
| `OUT-AF-010` | Strong focus marker in compact and expanded modes | Required | Not built | Focused row/entry is distinguished by at least two cues present simultaneously in both compact and expanded modes: an explicit pointer marker and non-default ANSI style; non-focused rows must not use both cues together. |
| `OUT-AF-011` | Expand/collapse affordances are explicit and discoverable | Required | Not built | Each collapsible region shows `[+]` or `[-]` state icon and appears in `?` help legend. |
| `OUT-AF-012` | Interaction look/feel aligns with Context History applet | Required | Not built | Output applet matches `context_applet.md` keyboard contract timing semantics (single keypress action, no Enter gating for navigation keys) and uses the same expand/collapse affordance tokens (`[+]`/`[-]`) defined here and referenced by context-history visual conventions in `context_history.md`. |
| `PD-01` | Per-entry interactive controls (collapse/expand, TUI copy) | Required | Partial | Collapse/expand is keyboard-operable; copy action is visible and reachable for focused entry content. |

---

## Owned Data/State

- Pane-local view state: focus position, per-turn expand/compact flag.
- Does **not** own: prompt orchestration state, tool execution state, context
  lifecycle transitions, or turn/entry content (sourced from core).

---

## UX Flow Coverage

- Flow A: startup greeting parity
- Flow B: prompt lifecycle parity
- Flow C: interactive navigation (focus, collapse/expand) — **Implemented with input responsiveness gaps**
- Flow D: most-recent-pointer and affordance discoverability parity with Context History — **Required**

Evidence anchors:

- `cmd/agentx-core/demo_harness.go` (`e2e-greet-001`, `e2e-cycle-001`)
- `cmd/agentx-core/demo_harness_test.go`
- `tests/test_demo_ux_use_cases_headless.sh`
- `cmd/agentx-core/features/output.feature` (acceptance scenarios for Flow C)

---

## Launch Contract

### Launch order

Applet process launch index: `1` (first applet process launched by core).

### Dedicated tmux target

- Default startup mode: `<session>:0.0`
- Visible-windows startup mode: `<session>:0.0` (window `output`)

---

## Core Integration Contract

- Prompt handling routes through Go core prompt pipeline.
- Output applet receives turn/entry data from core via IPC; renders it locally.
- Core notifies applet on new turn arrival; applet decides expand/compact
  rendering without further core involvement.
- No ownership of orchestration state transitions.

---

## Integration Touchpoints

- `input` applet: receives render updates for prompt submissions initiated by
  input command handling.
- `context` applet: prompt-cycle visibility and context-sensitive output rows
  must remain consistent with current context snapshot signals.
- `logs` applet: diagnostics for output lifecycle should be correlated via core
  runtime telemetry rather than duplicated output-side state.

---

## Phased Implementation Plan

### Remediation R1 — Input responsiveness and newest-turn correctness

1. Enforce raw/single-keystroke input handling for `Tab`, arrows, `h/j/k/l`,
  `Enter`, `Space`, `PgUp`, `PgDn`, `Home`, and `End`.
2. Remove key buffering behavior that waits for Enter before dispatch.
3. Reassert newest-turn auto-expand/focus on new turn arrival and persistent
  `LATEST` marker rendering.

### Remediation R2 — Discoverability and visual parity

1. Add explicit `[+]`/`[-]` affordances for all collapsible regions.
2. Strengthen focus styling in expanded mode to match compact mode clarity.
3. Align focus and affordance styling with Context History applet conventions.

### Remediation R3 — Startup output hygiene

1. Remove startup boilerplate rows (`[OUTPUT]`, `Chat ready.`, `Controls...`,
  `Clipboard...`) from the output surface.
2. Preserve lifecycle and conversation rows as first meaningful output.

---

## Test Evidence Targets

- Unit/integration anchors:
  - `cmd/agentx-core/demo_harness_test.go`
  - `cmd/agentx-core/output_widget_test.go` (target for Phase 1 unit coverage)
- Functional/UAT anchors:
  - `cmd/agentx-core/demo_harness.go` (`e2e-greet-001`, `e2e-cycle-001`)
  - `tests/test_demo_ux_use_cases_headless.sh`
  - `cmd/agentx-core/features/output.feature` (Gherkin acceptance scenarios)

---

## Gap Note

- Current implementation includes baseline interactive navigation, but key
  handling is not consistently single-keystroke responsive and may buffer until
  Enter (`OUT-AF-002` through `OUT-AF-005`).
- Newest-turn default behavior is currently inconsistent with contract intent;
  newest response may render collapsed (`OUT-AF-001`, `OUT-AF-009`).
- Expand/collapse affordance discoverability and focus prominence are below
  contract requirement (`OUT-AF-010`, `OUT-AF-011`).
- Startup boilerplate rows currently violate output hygiene requirement
  (`PD-17-AF-011`).
