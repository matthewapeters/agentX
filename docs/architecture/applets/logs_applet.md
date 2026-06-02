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

## Affordance List

- `e2e-logs-001`: logs startup and lifecycle parity visibility.
- `PD-17` alignment target: logs visibility remains part of demo/runtime parity
  review loop.

## Command/Input Model

- Terminal widget role: read-only diagnostics surface.
- Direct user commands: none.
- Input source: core lifecycle/status emission stream.

## Owned Data/State

- Owns presentation state for visible startup and lifecycle diagnostic rows.
- Does not own log production, event classification, or orchestration state.

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

## Integration Touchpoints

- `output` applet: runtime incidents visible in logs should correspond to output
  lifecycle behavior.
- `input` applet: command and submission diagnostics originate from input-driven
  core events and are rendered here.
- `context` applet: context refresh/load failures should be observable here for
  cross-applet debugging.

## Test Evidence Targets

- Unit anchors:
  - `cmd/agentx-core/logs_widget_test.go`
- Integration/functional anchors:
  - `cmd/agentx-core/demo_harness.go` (`e2e-logs-001`)
  - `cmd/agentx-core/demo_harness_test.go`
  - `tests/test_demo_ux_use_cases_headless.sh`

## Open Parity Notes

- Keep logs acceptance criteria aligned with demo parity stories and startup bootstrap telemetry expectations.

## Gap Note

- Logs applet contract is sufficient for lifecycle visibility, but it is
  intentionally a presentation sink; ownership for remediation workflows remains
  in core and other applet contracts.
