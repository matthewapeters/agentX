# Output Applet Contract

Last updated: 2026-06-04
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
- **Compact non-focused entries**: turns and entries that are not focused show
  only a single summary line (role prefix + first ~60 chars).  The focused
  turn/entry is rendered in full.
- **Pane-level space reclamation** (e.g., hide pane, resize window) is a
  tmux/core concern and is outside the applet's responsibility.

---

## Navigation / Input Model

The output applet is an **interactive read-only navigator**; it accepts no
text input but handles the following keys:

| Key(s) | Action |
| --- | --- |
| `↑` / `k` | Move focus to the previous (older) turn |
| `↓` / `j` | Move focus to the next (newer) turn |
| `←` / `h` | Collapse the focused entry / step focus to parent turn |
| `→` / `l` | Expand the focused entry / step focus to first child entry |
| `Enter` / `Space` | Toggle expand/compact on the focused turn or entry |
| `PgUp` | Scroll up one page within the expanded focused entry |
| `PgDn` | Scroll down one page within the expanded focused entry |
| `Home` | Jump to the oldest turn |
| `End` | Jump to the newest turn (restores default latest-first view) |
| `?` | Toggle inline help overlay showing the keymap |
| `q` | Close / detach the output widget |

Navigation ergonomics match familiar terminal conventions (`vi`-style j/k,
h/l for hierarchy) so users need no relearning from other applets.

---

## Turn and Entry Rendering Contract

- A **turn** is one complete user↔assistant exchange (may contain multiple
  entries: prompt, tool calls, tool results, assistant reply).
- An **entry** is one atomic block within a turn (e.g., a single tool result).
- Compact form: `[role] <summary line>` — one terminal line maximum.
- Expanded form: full content with syntax/markdown rendering where supported.
- The most recently received turn starts expanded; all others start compacted.
- After manual expand/compact, the applet preserves the user-set state until
  navigation focus leaves that turn; returning focus restores the user-set
  state for that turn within the session.

---

## UX Anchors

- `PD-01` ChatPanel output surface (`docs/ux/03_PANEL_DETAILS.md`)
- `PD-17-AF-011` startup greeting parity (`docs/ux/07_DEMO_MODE.md`)
- `PD-17-AF-012` prompt lifecycle parity (`docs/ux/07_DEMO_MODE.md`)
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

| ID | Affordance | Status |
| --- | --- | --- |
| `PD-17-AF-011` | Startup greeting appears in output at session start | Built |
| `PD-17-AF-012` | Prompt lifecycle rows stream in deterministic order | Built |
| `OUT-AF-001` | Latest-first default: newest turn auto-expanded on arrival | Planned (Phase 1) |
| `OUT-AF-002` | Focus navigation with `j/k` / arrows across turns | Planned (Phase 1) |
| `OUT-AF-003` | Per-entry collapse/expand with `h/l` / Enter / Space | Planned (Phase 1) |
| `OUT-AF-004` | PgUp/PgDn scrolling within expanded entry | Planned (Phase 1) |
| `OUT-AF-005` | Home/End jump to oldest/newest turn | Planned (Phase 1) |
| `OUT-AF-006` | Inline keymap help overlay via `?` | Planned (Phase 2) |
| `OUT-AF-007` | Compact non-focused turns to preserve working space | Planned (Phase 1) |
| `PD-01` | Per-entry interactive controls (collapse/expand, TUI copy) | Planned (Phase 1–2) |

---

## Owned Data/State

- Pane-local view state: focus position, per-turn expand/compact flag.
- Does **not** own: prompt orchestration state, tool execution state, context
  lifecycle transitions, or turn/entry content (sourced from core).

---

## UX Flow Coverage

- Flow A: startup greeting parity
- Flow B: prompt lifecycle parity
- Flow C: interactive navigation (focus, collapse/expand) — **Planned**

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

### Phase 1 — Core interactive navigation (OUT-AF-001 through OUT-AF-005, OUT-AF-007)

1. Add pane-local view-state struct (`focusIndex int`, `expandedMap map[int]bool`).
2. Implement latest-first default: on new turn IPC message, set `focusIndex` to
   newest turn and `expandedMap[newest] = true`; compact all prior.
3. Implement `j/k` / arrow turn navigation: move `focusIndex`, trigger redraw.
4. Implement `h/l` / Enter / Space: toggle `expandedMap[focusIndex]` and redraw.
5. Implement PgUp/PgDn: scroll within the viewport of the focused expanded entry.
6. Implement Home/End: jump `focusIndex` to first/last turn.
7. Render loop: expanded entry = full content; compact entry = single summary line.

### Phase 2 — Help overlay and copy affordances (OUT-AF-006, PD-01 copy)

1. Implement `?` toggle: render keymap table as an overlay; dismiss on any key.
2. Implement TUI copy action for focused entry content (explicit app-level action).

### Phase 3 — Parity sign-off

1. Map all `PD-01` interactive affordances to Phase 1/2 deliverables.
2. Close `OUT-AF-*` rows in the UX affordance ownership matrix.
3. Add regression tests for each affordance under `output.feature`.

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

- Phase 1 navigation affordances (`OUT-AF-001` through `OUT-AF-005`,
  `OUT-AF-007`) are **not yet implemented**; Gherkin scenarios in
  `output.feature` are expected to fail until Phase 1 is complete.
- UX contract coverage is strong for startup/lifecycle rows; interactive
  per-entry controls are now tracked as explicit implementation work items
  (not open direction ambiguity).
