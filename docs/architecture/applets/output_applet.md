# Output Applet Contract

Last updated: 2026-06-01
Applet ID: `chat/output`
Runtime entry: `agentx-core --output-widget`

## UX Anchors

- `PD-01` ChatPanel output surface (`docs/ux/03_PANEL_DETAILS.md`)
- `PD-17-AF-011` startup greeting parity (`docs/ux/07_DEMO_MODE.md`)
- `PD-17-AF-012` prompt lifecycle parity (`docs/ux/07_DEMO_MODE.md`)
- Traceability row: runtime applet closure table (`docs/ux/UX_LIFECYCLE.md`)

## Owned Widget/Surface Inventory

- Output conversation render stream (assistant/user/system/tool lifecycle text blocks)
- Startup greeting render surface
- Prompt lifecycle rows rendered during a prompt cycle

## UX Flow Coverage

- Flow A startup greeting parity
- Flow B prompt lifecycle parity

Evidence anchors:

- `cmd/agentx-core/demo_harness.go` (`e2e-greet-001`, `e2e-cycle-001`)
- `cmd/agentx-core/demo_harness_test.go`
- `tests/test_demo_ux_use_cases_headless.sh`

## Launch Contract

### Launch order

Applet process launch index: `1` (first applet process launched by core).

### Dedicated tmux target

- Default startup mode: `<session>:0.0`
- Visible-windows startup mode: `<session>:0.0` (window `output`)

## Core Integration Contract

- Prompt handling routes through Go core prompt pipeline.
- Output applet is a render consumer of core-owned prompt lifecycle and turn state.
- No ownership of orchestration state transitions.

## Open Parity Notes

- Keep aligned with `PD-01` output interaction affordances and any context-menu parity updates.
