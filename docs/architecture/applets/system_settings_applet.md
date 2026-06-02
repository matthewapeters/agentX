# System Settings Applet Contract

Last updated: 2026-06-01
Applet ID: `system-settings`
Runtime entry (target): `agentx-core --settings-widget` (planned)

## Purpose

Define the first-class System Settings runtime applet contract and its UX
traceability. This applet is composed into the system experience through tmuxp
composition logic while preserving UX behavior contracts.

## UX Anchors

- `PD-18-AF-003` configuration applet renders runtime config (`docs/ux/03_PANEL_DETAILS.md`)
- `PD-07` SettingsTab detail references (`docs/ux/03_PANEL_DETAILS.md`)
- System panel parity flows (`docs/ux/07_DEMO_MODE.md`):
  - `PD-17-AF-013` Flow C
  - `PD-17-AF-014` Flow C.1

## Owned Widget/Surface Inventory

- Runtime configuration summary surface (`model`, `backend`, `ollama_host` parity block)
- Effective settings/override display contract required by system-tab validations
- Deterministic settings section framing used in system-tour assertions

## UX Flow Coverage

- Flow C system panel parity includes `configuration` tab expectations
- Flow C.1 system tab tour validates `configuration` in deterministic order

Primary evidence targets:

- `cmd/agentx-core/context_widget.go` (transitional source until split runtime lands)
- `cmd/agentx-core/context_widget_test.go`
- `cmd/agentx-core/core_system_renderer_test.go`
- `cmd/agentx-core/demo_harness.go` (`e2e-system-001`, `e2e-system-tour-001`)
- `tests/test_demo_system_panel_tour_headless.sh`

## Launch and Composition Contract

### Launch order

Target launch order after first-class split lands: after first-class file-system
applet registration in tmuxp-composed system assembly (final order to be fixed
in startup composition spec).

### Dedicated tmux target

- Default startup mode (target): dedicated first-class pane/window composed by
  tmuxp into system layout.
- Visible-windows startup mode (target): dedicated first-class window.

### Composition rule

- tmuxp owns composition of first-class applet windows/panes into the final
  operator-facing system layout.
- Go core retains orchestration, IPC, and health ownership.

## Current State

- Transitional implementation is currently rendered via `context` applet tabs.
- First-class runtime split is planned/required by active remaining-work plan.

## Done Criteria

1. Dedicated runtime entrypoint and lifecycle registration exist.
2. tmuxp composition wiring places the settings surface in system UX layout.
3. Unit + integration + functional parity evidence passes.
4. Traceability rows are reconciled as tested in UX lifecycle docs.
