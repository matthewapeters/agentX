# AgentX Startup Modes (Authoritative)

Last updated: 2026-05-31

This document is the single source of truth for startup modes and startup switches in AgentX.

## Startup Mode Table

| mode | description | cli pattern |
| ---- | ---- | ---- |
| default | launch AgentX in standard interactive runtime layout (output/system/input + logs window) for normal user experience | `agentx [working-directory-path]` |
| default (selector alias) | explicitly select default startup topology without requiring demo mode | `agentx --default --project-dir <path>` |
| default (explicit project-dir) | launch standard runtime layout from an explicit project directory | `agentx --project-dir <path>` |
| windowed (`visible-windows`) | launch one visible window per applet for startup/UAT validation before frame-style pane layout | `agentx --startup-mode visible-windows --project-dir <path>` |
| windowed (selector alias) | explicitly select windowed startup topology without requiring demo mode | `agentx --windowed --project-dir <path>` |
| demo | launch DemoMode split-controller UX for interactive scenario review (`stores`, `testControler`, `liveCore`) | `agentx --demo --project-dir <path>` |
| demo-headless | run DemoMode sequence without split controller UI (automation/smoke path) | `agentx --demo-headless --project-dir <path>` |
| demo-controller (internal) | run controller-only process attached to an already-running demo core session | `agentx --demo-controller --demo-core-session <tmux-session> --health-addr <host:port> --project-dir <path>` |
| input-widget (internal) | run the native Go input widget process used by core runtime applet supervision | `agentx-core --input-widget --core-http <http://host:port>` |
| context-widget (internal) | run the native Go context/system widget process used by core runtime applet supervision | `agentx-core --context-widget --core-http <http://host:port>` |

## Startup Topology Switches

| switch | allowed values | effect |
| ---- | ---- | ---- |
| `--startup-mode` | `default`, `visible-windows` | selects startup topology mode for core runtime launch (`visible-windows` is the windowed mode) |
| `AGENTX_STARTUP_MODE` | `default`, `visible-windows` | environment default for startup topology; ignored when `--startup-mode` is provided |

## Related Startup Flags

| switch | purpose |
| ---- | ---- |
| `--project-dir` | sets project root for sessions/config/runtime files |
| `--session-id` | forces session id instead of auto-generation |
| `--user` | sets user namespace used in tmux/session paths |
| `--attach` | attach to tmux after startup (`true` by default; set `false` for headless runtime boot) |
| `--layout` | tmuxp composition file applied after owned windows are created (defaults to `.agentx/layouts/default-layout.yaml`) |
| `--layout-file` | legacy compatibility alias for `--layout` |
| `--dump-default-layout` | writes built-in default tmuxp composition to `<file>` or stdout (`-`) and exits |
| `--layout-template` | writes a starter tmuxp layout template and exits |

## Demo Startup Selectors

The demo launcher accepts explicit selectors that map to the same startup
topologies used by `--startup-mode`:

| demo flag | effect |
| ---- | ---- |
| `--demo --default` | launches demo mode with the default frame-based topology |
| `--demo --windowed` | launches demo mode with the windowed startup topology |

If neither demo selector is provided, demo mode uses the resolved startup mode
from `--startup-mode` or `AGENTX_STARTUP_MODE`.

Outside demo mode, `--default` and `--windowed` are valid startup-topology
selector aliases equivalent to `--startup-mode default` and
`--startup-mode visible-windows`.

## Notes

- `visible-windows` is a validation surface for applet presence and responsiveness; if setup fails, runtime must fall back to `default` startup topology.
- `demo-controller`, `input-widget`, and `context-widget` are internal/runtime wiring modes and are not standard end-user entry points.
- Topology mode selection (`--startup-mode`) is distinct from run mode selection (`--demo`, `--demo-headless`, etc.).
- Runtime startup now probes `tmux -V` and `tmuxp --version`; missing binaries or probe failures are treated as hard prerequisite errors before session initialization.
