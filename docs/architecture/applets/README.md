# Applet Contracts (Authoritative)

Last updated: 2026-06-01

## Purpose

This directory defines the authoritative runtime contract for each active Go-runtime applet.
Each applet contract must:

- map to UX panel/affordance sources,
- list the widget/surface inventory it owns,
- map to UX flow coverage,
- define launch order and dedicated tmux `session:window.pane` ownership.

Primary UX sources:

- `docs/ux/03_PANEL_DETAILS.md`
- `docs/ux/02_USER_FLOWS.md`
- `docs/ux/UX_LIFECYCLE.md`
- `docs/ux/07_DEMO_MODE.md`

Primary runtime sources:

- `cmd/agentx-core/core.go`
- `cmd/agentx-core/config.go`
- `cmd/agentx-core/context_widget.go`
- `cmd/agentx-core/input_widget.go`
- `cmd/agentx-core/logs_widget.go`
- `cmd/agentx-core/output_widget.go`

## Runtime Applet Set

1. `chat/output` applet: `output_applet.md`
2. `input` applet: `input_applet.md`
3. `logs` applet: `logs_applet.md`
4. `context` applet (session/context surfaces and transitional host for system tabs): `context_applet.md`
5. `filesystem` applet (first-class target, tmuxp-composed): `filesystem_applet.md`
6. `system-settings` applet (first-class target, tmuxp-composed): `system_settings_applet.md`

## UX Affordance Ownership Matrix

This is the compact ownership map from UX affordance IDs to runtime applets.

| UX affordance / flow | UX source | Owning runtime applet | Current implementation status | Test status |
| --- | --- | --- | --- | --- |
| `PD-17-AF-011` startup greeting parity | `docs/ux/07_DEMO_MODE.md` | `chat/output` | Built | Tested |
| `PD-17-AF-012` prompt lifecycle parity | `docs/ux/07_DEMO_MODE.md` | `chat/output` | Built | Tested |
| `PD-16-AF-010` input widget contract | `docs/ux/06_TUI_MIRROR.md` | `input` | Built | Tested |
| `PD-17-AF-005` prompt-entry/decision behavior | `docs/ux/07_DEMO_MODE.md` | `input` | Built | Tested |
| `e2e-logs-001` logs lifecycle parity | `docs/ux/07_DEMO_MODE.md` | `logs` | Built | Tested |
| `PD-18-AF-002` context-history surface | `docs/ux/03_PANEL_DETAILS.md` | `context` (system tab) | Built (tab surface) | Tested |
| `PD-18-AF-003` configuration surface | `docs/ux/03_PANEL_DETAILS.md` | First-class System Settings applet (target), transitional `context` tab owner | Transitional tab surface exists; first-class split decision finalized | Partial (needs split-runtime validation) |
| `PD-18-AF-004` files surface | `docs/ux/03_PANEL_DETAILS.md` | First-class File System applet (target), transitional `context` tab owner | Transitional tab surface exists; first-class split decision finalized | Partial (needs split-runtime validation) |
| `PD-18-AF-005` working-memory surface | `docs/ux/03_PANEL_DETAILS.md` | `context` (session/system composition) | Built (surface), not split as first-class applet process | Partially reconciled |
| `PD-18-AF-006` context-visualizer surface | `docs/ux/03_PANEL_DETAILS.md` | `context` (system tab) | Built (tab surface) | Tested |
| `PD-18-AF-007` visible-windows startup topology | `docs/ux/03_PANEL_DETAILS.md` | Core startup contract across all applets | Built | Tested |

### Important interpretation note

- Working Memory is specified in UX as part of Session-tab context composition
  (`PD-03` / `PD-08`) and is therefore owned by the `context` applet contract
  unless UX is revised to require a separate first-class runtime applet.
- File System and System Settings ownership is finalized as first-class runtime
  applets; tmuxp composition logic assembles them into the composed system UX.
- Current open migration work is primarily about runtime inventory/traceability
  alignment and split-runtime completion for these applets, while preserving UX
  behavior parity during transition.

## Launch Order (Applet Processes)

Process launch order is defined by `DefaultPaneLayout()` iteration and `launchPaneAppletProcesses()`:

1. `chat`
2. `context`
3. `input`
4. `logs`

Notes:

- Default layout pane creation order is distinct from process launch order.
- Default layout pane creation sequence is: output pane, input split, system split, logs window.

## Dedicated tmux Targets

### Default startup mode

- `chat/output` -> `<session>:0.0`
- `context` -> dynamic pane id in window `0` (initially right split, title=`system`)
- `input` -> dynamic pane id in window `0` (initially bottom split, title=`input`)
- `logs` -> `<session>:1.0`

### Visible-windows startup mode

- `chat/output` -> `<session>:0.0` (window `output`)
- `logs` -> `<session>:1.0` (window `logs`)
- `input` -> `<session>:2.0` (window `input`)
- `context` -> `<session>:3.0` (window `system`)

## Governance

A parity-closed claim for any applet requires:

1. UX spec anchors are explicit and current.
2. Widget/surface inventory is complete for that applet.
3. UX flows are mapped to executable tests.
4. Launch contract (`order` + `session:window`) is documented and validated.
