# File System Applet Contract

Last updated: 2026-06-01
Applet ID: `filesystem`
Runtime entry (target): `agentx-core --filesystem-widget` (planned)

## Purpose

Define the first-class File System runtime applet contract and its UX traceability.
This applet is composed into the system experience through tmuxp composition
logic while preserving UX behavior defined in panel and flow specs.

## UX Anchors

- `PD-18-AF-004` file-selection applet renders project file navigation (`docs/ux/03_PANEL_DETAILS.md`)
- `PD-11` FileExplorer behavior references (`docs/ux/03_PANEL_DETAILS.md`)
- System panel parity flows (`docs/ux/07_DEMO_MODE.md`):
  - `PD-17-AF-013` Flow C
  - `PD-17-AF-014` Flow C.1

## Owned Widget/Surface Inventory

- File-system listing/summary surface used by system tab-tour parity
- Deterministic project-root metadata block (`project_dir`, `entry_count` parity contract)
- Navigation/selection summary rows required by system-flow validation

## Affordance List

- `PD-18-AF-004`: files surface renders project navigation/selection summary for
  system parity review.
- `PD-11` alignment target: navigation and file/directory action behavior stays
  consistent with FileExplorer intent.
- `PD-11-AF-008..010` alignment target: right-click file/folder action model and
  popup dismissal semantics remain the UX reference.
- `UF-11`: file explorer navigation and attach/edit/folder-memory flows.
- `UF-12`: context popup rendering visibility and palette-first invariants.

## Command/Input Model

- Current transitional mode: no first-class command parser in a dedicated
  filesystem process; behavior is rendered through context-owned system tabs.
- Target first-class mode: file navigation/select actions exposed as runtime
  applet interactions compatible with `PD-11` and `UF-11`/`UF-12` contracts.

## Owned Data/State

- Current transitional ownership: files surface presentation state in context-tab
  system rendering.
- Target ownership after split: filesystem applet owns file listing/selection
  presentation state and navigation summary state for its dedicated runtime.

## UX Flow Coverage

- Flow C system panel parity includes `files` tab expectations
- Flow C.1 system tab tour validates `files` in deterministic order

Primary evidence targets:

- `cmd/agentx-core/context_widget.go` (transitional source until split runtime lands)
- `cmd/agentx-core/context_widget_test.go`
- `cmd/agentx-core/core_system_renderer_test.go`
- `cmd/agentx-core/demo_harness.go` (`e2e-system-001`, `e2e-system-tour-001`)
- `tests/test_demo_system_panel_tour_headless.sh`

## Launch and Composition Contract

### Launch order

Target launch order after first-class split lands: after `context` applet host
bootstrap and before `logs` applet in tmuxp-composed system assembly (final
order to be fixed in startup composition spec).

### Dedicated tmux target

- Default startup mode (target): dedicated first-class pane/window composed by
  tmuxp into system layout.
- Visible-windows startup mode (target): dedicated first-class window.

### Composition rule

- tmuxp owns the composition of first-class applet windows/panes into the
  operator-facing system layout.
- Core ownership and health/IPC contracts remain under Go core orchestration.

## Integration Touchpoints

- `context` applet: transitional host for files surface until split runtime is
  complete.
- `input` applet: file attach flows must integrate with input-side context and
  attachment intent (`UF-11`).
- `system-settings` applet: both participate in deterministic system tab-tour
  order (`PD-17-AF-014`).
- `output`/`logs` applets: file-action diagnostics and related lifecycle messages
  must be traceable through core runtime output/log channels.

## Test Evidence Targets

- Unit anchors (current transitional implementation):
  - `cmd/agentx-core/context_widget_test.go`
  - `cmd/agentx-core/core_system_renderer_test.go`
- Integration/functional anchors:
  - `cmd/agentx-core/demo_harness.go` (`e2e-system-001`, `e2e-system-tour-001`)
  - `tests/test_demo_system_panel_tour_headless.sh`
  - `tests/test_file_explorer_context_menu.py` (UX reference for `PD-11`)

## Current State

- Transitional implementation is currently rendered via `context` applet tabs.
- First-class runtime split is planned/required by active remaining-work plan.

## Gap Note

- UX contract expectations from `PD-11`, `PD-18-AF-004`, `UF-11`, and `UF-12`
  currently exceed the dedicated-runtime implementation state because files
  behavior remains transitional under context-tab ownership.

## Done Criteria

1. Dedicated runtime entrypoint and lifecycle registration exist.
2. tmuxp composition wiring places the surface in system UX layout.
3. Unit + integration + functional parity evidence passes.
4. Traceability rows are reconciled as tested in UX lifecycle docs.
