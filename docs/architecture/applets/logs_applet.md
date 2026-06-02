# Logs Applet Contract

Last updated: 2026-06-01
Applet ID: `logs`
Runtime entry: `agentx-core --logs-widget`

## UX Anchors

- `PD-17` demo/system parity coverage for logs startup lifecycle
- `e2e-logs-001` logs-pane startup and lifecycle parity story (`docs/ux/07_DEMO_MODE.md` usage)

## Owned Widget/Surface Inventory

- Startup status lines (`[startup] ...`)
- Lifecycle event rows (`[lifecycle] ...`)
- Bridge/runtime diagnostics emitted by core

## UX Flow Coverage

- Logs visibility and startup lifecycle parity in demo/UAT flow.

Evidence anchors:

- `cmd/agentx-core/logs_widget.go`
- `cmd/agentx-core/logs_widget_test.go`
- `cmd/agentx-core/demo_harness.go` (`e2e-logs-001`)
- `cmd/agentx-core/demo_harness_test.go`
- `tests/test_demo_ux_use_cases_headless.sh`

## Launch Contract

### Launch order

Applet process launch index: `4`.

### Dedicated tmux target

- Default startup mode: `<session>:1.0`
- Visible-windows startup mode: `<session>:1.0` (window `logs`)

## Core Integration Contract

- Core writes startup/lifecycle status to logs pane target.
- Logs applet is the presentation sink for runtime diagnostics.

## Open Parity Notes

- Keep logs acceptance criteria aligned with demo parity stories and startup bootstrap telemetry expectations.
