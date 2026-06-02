# Context Applet Contract

Last updated: 2026-06-01
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

1. Files surface (File System summary/navigation surface)
2. Configuration surface (System Settings summary surface)
3. Context surface (current session/context window summary)
4. Context-history surface
5. Working-memory surface
6. Context-visualizer surface

## Affordance List

- `PD-18-AF-001`: system frame binding by semantic title/role.
- `PD-18-AF-002`: context-history rendering contract.
- `PD-18-AF-005`: working-memory summary rendering contract.
- `PD-18-AF-006`: context visualizer capacity/prompt-cycle rendering contract.
- `PD-18-AF-007`: participates in visible-windows startup topology.
- `PD-03` / `PD-08` alignment: session context and working-memory composition
  behavior remains authoritative for this applet contract.

## Command/Input Model

- Terminal widget role: primarily read-only context/session render surface.
- Direct user command parser: none in current contract.
- Input source: core context snapshots, active-session state, and system applet
  host routing.

## Owned Data/State

- Owns context/session presentation state for:
  - context history/current context summary,
  - working-memory summary rows,
  - context visualizer meter/status rows,
  - transitional files/configuration tab surfaces.
- Does not own persistence for working memory or context message storage.

Notes:

- Working Memory belongs to the Session-tab context composition, not a separate top-level runtime applet.
- Files and Configuration are represented as context-app applet surfaces/tabs in current Go runtime.
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
  here until first-class split and tmuxp composition are complete.
- `system-settings` applet (target split): configuration-tab responsibilities are
  transitional here until first-class split and tmuxp composition are complete.
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
