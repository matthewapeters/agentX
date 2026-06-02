# Applet Contracts (Authoritative)

Last updated: 2026-06-02

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
- `cmd/agentx-core/filesystem_widget.go`
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

## Ownership Model (Normative)

To remove cross-doc ambiguity, ownership is defined using two distinct terms:

- Authoritative owner: the applet contract responsible for UX requirement
  closure, acceptance evidence, and sign-off state for a surface.
- Transitional render host: a temporary runtime host that may render the
  surface while migration is in progress.

Single authoritative ownership per system surface:

- Files surface (`PD-18-AF-004`) -> `filesystem` applet (authoritative owner)
- Configuration surface (`PD-18-AF-003`) -> `system-settings` applet
  (authoritative owner)
- Context-history surface (`PD-18-AF-002`) -> `context` applet
  (authoritative owner)
- Working-memory surface (`PD-18-AF-005`) -> `context` applet
  (authoritative owner, unless UX spec changes)
- Context-visualizer surface (`PD-18-AF-006`) -> `context` applet
  (authoritative owner)

Transitional hosting note:

- `context` may temporarily host Files/Configuration rendering during migration,
  but this does not transfer authoritative ownership from
  `filesystem` / `system-settings`.

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

## Sign-off Signal Policy

To remove contradictory "closed vs reopened" language:

- "Closed" is valid only when the authoritative owner for a surface has
  current unit + integration/functional evidence and traceability rows are
  reconciled in UX lifecycle artifacts.
- If migration or inventory drift is discovered later, that surface status must
  be explicitly marked "Re-opened" with the reason, and all prior global
  closure language must be treated as historical context rather than current
  state.
- A transitional render host cannot be used to claim closure for a surface it
  does not authoritatively own.

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

Additional governance directives (2026-06-02):

- All interactive UX affordances must be implemented in the TUI runtime path.
- TUIs must meet UX features, affordances, and functionality defined by
  authoritative UX docs.
- If no obvious implementation path exists for an affordance, escalate to user
  decision as a case-by-case contract review; do not silently de-scope.
- Definition of Done for each applet is UX-criteria complete with executable
  evidence, not implementation convenience.

## Contract Completeness Standard

Each applet contract in this folder must include the following implementation
handoff sections so parallel experts can execute without ambiguity:

1. Affordance list (mapped to UX IDs, especially PD-11 and PD-18 where relevant).
2. Command/input model (for terminal widgets: accepted commands, key inputs, or
   explicit read-only behavior).
3. Owned data/state (what state the applet owns vs consumes from core).
4. Launch/runtime contract (entrypoint, launch order, tmux target).
5. Integration touchpoints with other applets (especially input/context coupling).
6. Explicit test evidence targets (unit + integration/functional anchors).
7. Gap note when UX contract expectations exceed current runtime implementation.

## Sufficiency Decision (2026-06-02)

Decision: applet docs are now sufficient for parallel implementation handoff,
with explicit gap notes preserved where runtime split and parity closure are
still in progress.

Primary open implementation gaps captured in applet docs:

- First-class split runtime for `filesystem` and `system-settings` remains
  transitional through `context` tab ownership.
- `PD-18` rows are documented as UX-tested at the spec layer, but active runtime
  inventory/ownership reconciliation remains open in
  `docs/hybrid_remaining_work.md`.

## Applet Readiness Review (2026-06-02)

This review answers four required questions for each runtime applet:

1. Is this at a professional level of implementation?
2. Is the user experience as clear and uncluttered as it can be?
3. Are all UX and UX flow requirements met?
4. Are there ambiguities between requirements and implementation that must be resolved?

| Applet | Professional level | UX clarity | UX requirements met | Ambiguities requiring resolution | Required follow-up |
| --- | --- | --- | --- | --- | --- |
| `chat/output` | Partial | Good | Partial | Yes | Implement `PD-01` interactive affordances in TUI; if any affordance lacks an obvious implementation path, escalate to user case-by-case decision before closure. |
| `input` | Partial | Good | Partial | Yes | Close `PD-02` parity deltas for command discoverability and non-submit interaction parity under TUI-first runtime. |
| `logs` | Yes (for current role) | Good | Mostly | No material ambiguity | Keep as read-only diagnostics surface; maintain lifecycle signal coverage and formatting readability. |
| `context` | Partial | Mixed (surface multiplexing increases cognitive load) | Partial | Yes | Resolve ownership boundary between context-owned surfaces and first-class split applets, then reconcile naming/inventory docs. |
| `filesystem` | No (not yet) | Partial (long-list usability gap) | No | Yes | Implement overflow viewport/paging requirements (`PD-11-AF-011..014`), plus keyboard affordances (`arrow`, `space` soft-select, `return` hard-select), and close `UF-11`/`UF-12` parity evidence in TUI runtime. |
| `system-settings` | Partial | Good | Partial | Yes | Finalize dedicated runtime ownership and acceptance criteria parity against `PD-07` and `PD-18-AF-003`. |

Concrete ambiguity example (must be resolved explicitly):

- Requirement-side expectation: `PD-01` includes interactive per-entry controls
  (collapse/expand and context actions).
- Implementation-side behavior: current TUI output surface is primarily a render
  sink without equivalent per-entry interactive controls.
- Required resolution: either implement equivalent TUI controls or obtain a
  user-approved, explicit requirement update for this specific affordance.

Review summary counts:

- Applets requiring additional work: 5 of 6
- Applets with ambiguities requiring resolution: 5 of 6

## Decision Log (2026-06-02)

User-selected direction: `A` for previously listed conflict options.

Applied contract decisions:

1. Output panel parity (`PD-01`): implement full per-entry interactive controls
  in TUI (not command-only parity).
2. Output copy parity (`PD-01-AF-010`): implement explicit app-level TUI copy
  actions with deterministic behavior.
3. Input multiline parity (`PD-02`): prioritize richer multiline key handling
  for TUI input behavior.
4. Filesystem soft-select semantics (`PD-11-AF-016`): adopt persistent
  multi-select semantics with visible selection state.

Implication:

- These are implementation work items, not open ambiguity items.
- Any residual no-obvious-path affordance still follows case-by-case user
  escalation before closure.
