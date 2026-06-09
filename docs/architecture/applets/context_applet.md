# Context Applet Contract

Last updated: 2026-06-04
Applet ID: `context`
Runtime entry: `agentx-core --context-widget`

## UX Anchors

Primary ownership is SidePanel Session-context composition from UX:

- `PD-03` SidePanel Session tab context surface (`docs/ux/03_PANEL_DETAILS.md`)
- `PD-03` SidePanel Session tab working-memory section (`docs/ux/03_PANEL_DETAILS.md`)
- `PD-08` ContextRenderer behavior (`docs/ux/03_PANEL_DETAILS.md`)

System-pane runtime tab parity anchors:

- `PD-17-AF-013` system panel parity (`docs/ux/07_DEMO_MODE.md`)
- `PD-17-AF-014` system panel tab tour parity (`docs/ux/07_DEMO_MODE.md`)
- `PD-18-AF-001..007` SystemAppletSuite (`docs/ux/03_PANEL_DETAILS.md`)

## Owned Widget/Surface Inventory

The context applet owns the system/session context surfaces as one applet contract.

1. Context surface (current session/context window summary)
2. Context-history surface
3. Working-memory surface
4. Context-visualizer surface

Transitional render-host surfaces (not authoritative ownership):

1. Files surface (transitional host for first-class `filesystem` applet)
2. Configuration surface (transitional host for first-class `system-settings` applet)

## Affordance List

- `PD-18-AF-001`: system frame binding by semantic title/role.
- `PD-18-AF-002`: context-history rendering contract.
- `PD-18-AF-005`: working-memory summary rendering contract.
- `PD-18-AF-006`: context visualizer capacity/prompt-cycle rendering contract.
- `PD-18-AF-007`: participates in visible-windows startup topology.
- `PD-03` / `PD-08` alignment: session context and working-memory composition
  behavior remains authoritative for this applet contract.

## Command/Input Model

- Terminal widget role: interactive context-feedback surface with keyboard-first
  navigation and selection behavior.
- Direct user command parser: implemented. Supports raw-key mode (TTY) and
  line/prompt mode using the shared applet input-reader contract.

### Two-mode Navigation (authoritative)

The widget operates in two exclusive modes that govern what all navigation keys
do. The current mode is indicated visually by section-border brightness.

| Mode | How to enter | How to exit | Border color |
|------|-------------|------------|-------------|
| **Outside section** | Initial state; `Shift-Tab` from section root | `Tab` | Dim (all sections) |
| **Inside section** | `Tab` while outside | `Shift-Tab` | Bright (active section only) |

### Keyboard contract (authoritative)

**Outside-section mode (header navigation):**

- `Up` / `Down` (`k` / `j`): move the section-header cursor through the ordered
  section list: `context-history` → `working-memory` → `current-context`.
  Cursor clamps at both ends.
- `Space`: expand or collapse the currently focused section.
- `Tab`: drill into the focused section by one level (switch to inside-section
  mode), expanding the target section if needed and placing focus on the
  expanded target.
- `Enter`: action-only; no section drill-in and no expand/collapse behavior.

**Inside-section mode (row navigation):**

- `Up` / `Down` (`k` / `j`): move the active row within the section.
- `Left` / `Right` (`h` / `l`): horizontal sibling movement (current
  prompt/response and prior-session group/item transitions).
- `Space`: perform peek/expand on the focused section or node; toggle the
  focused node branch visibility without moving focus.
- `Enter`: perform action only on the focused element/cell:
  - enable or disable an element,
  - commit a cell value and advance to the next cell,
  - save the Working Memory key/value pair when focus is on the Save cell.
- `PageDown` / `PageUp`: scroll text content when the active row is an expanded
  `current-context` entry; otherwise page the row cursor by 5.
- Section-specific note: for `context-history` and `working-memory`,
  `PageDown` / `PageUp` always page row selection by 5 (no separate textbox
  scroll mode).
- `Tab`: drill one level deeper from the focused target and move focus to the
  expanded target.
- `Shift-Tab`: back out one level and collapse the exited node.

**Textbox-scroll mode (inside section, expanded text row):**

- `Up` / `Down`: scroll one line.
- `PageUp` / `PageDown`: scroll five lines.
- `Tab`: exit textbox mode and remain inside the active section.

Rule: when `Shift-Tab` backs out from the section root, focus returns to
outside-section mode and the exited section is collapsed.

### Section list (authoritative order)

1. `context-history` — prior sessions, collapsed by default
2. `working-memory` — working memory facts, collapsed by default
3. `current-context` — active session turns, expanded by default

All three sections are rendered even when collapsed (title bar visible, no
row content).

### Expanded textbox contract

- Context prompt/response text is wrapped.
- Expanded viewport shows up to 5 lines.
- Scrolling indicator `↕ scroll N/M (PgUp/PgDn to scroll)` is rendered for
  overflow content.

## Visual Design Decisions (Authoritative)

- Context-feedback sections use IBM box-drawing style for expanded/structured
  blocks.
- Section titles use reverse-video treatment to improve scanability.
  - When the section-header cursor rests on a section (outside-section mode),
    the header gains a `▶` prefix and cyan highlight.
- Section borders encode navigation state:
  - **Dim** (`\033[2m`) — the default; section is not currently active for
    inside-section row navigation.
  - **Bright** (section-specific accent color) — the section the user is
    currently navigating inside:
    - `current-context` → cyan
    - `working-memory` → green
    - `context-history` → magenta
- Semantic color + emoji markers differentiate entry types and state:
  - user vs agent rows,
  - selected vs active row markers (suppressed when in outside-section mode),
  - collapsed/disabled status and action affordances.
- ANSI-aware width handling is required so styled rows remain aligned.
- Terminal cursor is hidden (`\033[?25l`) for the duration of the widget
  lifetime and restored on exit (`\033[?25h`) to prevent cursor flicker.

### TUI protocol-line filter (authoritative)

The underlying render pipeline embeds machine-readable protocol lines used by
httptest consumers and system-surface integration tests:

```
[SYSTEM]
[SYSTEM TAB] active=<tab>
== CONTEXT HISTORY ==
history_context_count: N
recent_prompt: ...
recent_response: ...
turn_count: N
```

These lines are **filtered from TUI terminal output** by
`filterContextWidgetTUILines()` applied in the render loop. They are **not**
removed from the `renderContextWidgetWithState()` return value so that
`core_system_renderer_test.go` and HTTP-path consumers continue to work
unchanged.

## Owned Data/State

- Owns context/session presentation state for:
  - context history/current context summary,
  - working-memory summary rows,
  - context visualizer meter/status rows,
  - interactive row state (active row, selection set, collapsed set,
    disabled set),
  - per-row scroll offsets,
  - section-navigation state: `activeSection` (focused section header),
    `insideSection` (mode flag), `focusTextBox` (textbox-scroll sub-mode),
  - per-section collapsed state: `collapsedContextHistory`,
    `collapsedWorkingMemory`, `collapsedCurrentContext`,
  - transitional files/configuration render-host routing only.
- Does not own persistence for working memory or context message storage.

Notes:

- Working Memory belongs to the Session-tab context composition, not a separate top-level runtime applet.
- Files and Configuration may be rendered in context-app tabs in the current Go
  runtime, but authoritative ownership remains with first-class
  `filesystem`/`system-settings` applet contracts.
- File System and System Settings split-vs-compose decision is finalized as
  first-class runtime applets; current context-tab ownership is transitional
  until split runtimes land and tmuxp composition wiring is complete.

## UX Flow Coverage

- Flow C system panel parity (`files`, `configuration`, `context`, `context history`, `context visualizer`)
- Flow C.1 system panel tour parity (deterministic tab sequence)
- Session context and working-memory behavior from PD-03/PD-08/UX lifecycle mappings

Evidence anchors:

- `cmd/agentx-core/context_widget.go`
- `cmd/agentx-core/context_widget_test.go`
- `cmd/agentx-core/filesystem_widget.go` (shared command reader contract)
- `cmd/agentx-core/core_system_renderer_test.go`
- `cmd/agentx-core/system_applet_host.go`
- `cmd/agentx-core/system_applet_working_memory.go`
- `cmd/agentx-core/demo_harness.go` (`e2e-system-001`, `e2e-system-tour-001`)
- `cmd/agentx-core/demo_harness_test.go`
- `tests/test_demo_system_panel_tour_headless.sh`

## Launch Contract

### Launch order

Applet process launch index: `2`.

### Dedicated tmux target

- Default startup mode: dynamic pane id in window `0` with pane title `system` (initial split target starts at `<session>:0.1`)
- Visible-windows startup mode: `<session>:3.0` (window `system`)

## Core Integration Contract

- Context applet renders from core context snapshot (`/context`) and related session state.
- Tab resolution uses system applet host routing for specialized surfaces.
- Context applet remains the single runtime applet host for these surfaces in current topology.

## Integration Touchpoints

- `input` applet: context-file registration and prompt activity signals affect
  context summary and prompt-cycle visibility.
- `filesystem` applet (target split): files-tab responsibilities are transitional
  here as render-host behavior until first-class split and tmuxp composition are complete.
- `system-settings` applet (target split): configuration-tab responsibilities are
  transitional here as render-host behavior until first-class split and tmuxp composition are complete.
- `output`/`logs` applets: context lifecycle state must remain consistent with
  rendered prompt lifecycle and diagnostics.

## Test Evidence Targets

- Unit anchors:
  - `cmd/agentx-core/context_widget_test.go`
  - `cmd/agentx-core/core_system_renderer_test.go`
  - `cmd/agentx-core/system_applet_working_memory.go` coverage via core tests
- Integration/functional anchors:
  - `cmd/agentx-core/demo_harness.go` (`e2e-system-001`, `e2e-system-tour-001`)
  - `cmd/agentx-core/demo_harness_test.go`
  - `tests/test_demo_system_panel_tour_headless.sh`

## Open Parity Notes

- `PD-18` traceability remains re-opened while system-surface scope alignment is being reconciled in active migration work.
- First-class split delivery for File System and System Settings must preserve
  this UX ownership map at the composed view level and be orchestrated through
  tmuxp layout composition.

## Gap Note

- This contract is sufficient for current implementation ownership, but it is
  intentionally transitional for files/configuration surfaces while first-class
  `filesystem` and `system-settings` runtime split work remains open.
