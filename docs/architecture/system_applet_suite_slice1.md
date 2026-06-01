# System Applet Suite Slice-1 (PD-18)

_Last updated: 2026-05-31_

## Purpose

Define the first executable implementation slice for the PD-18 SystemAppletSuite,
including code scope, test scope, acceptance gates, and rollback criteria.

This packet is the execution guide for `HYB-06` in
`docs/hybrid_remaining_work.md`.

## Slice Objective

Deliver the first production-grade vertical slice for system applets by shipping:

1. title-bound system frame host routing contract (PD-18-AF-001)
2. context-history applet implementation (PD-18-AF-002)
3. baseline startup-mode validation hooks required to support visible-windows
   rollout later (PD-18-AF-007 pre-work only)

## In Scope

- Introduce a system applet host abstraction in Go core (in-process).
- Route current system tab selection through host lookup instead of ad hoc
  branching for the context-history path.
- Implement context-history applet render contract from core context state.
- Add deterministic tests for rendering, routing, and tab-state behavior.
- Keep existing behavior for non-slice applets unchanged.

## Out of Scope

- Full visible-windows startup mode implementation (`--startup-mode`).
- Working-memory mutation workflows.
- New tmux topology changes beyond current title-based routing.
- GUI surface changes.

## Traceability Mapping

| Affordance ID | Slice-1 status | Notes |
| --- | --- | --- |
| PD-18-AF-001 | In scope | Title-based system frame host binding |
| PD-18-AF-002 | In scope | Context-history applet render + tests |
| PD-18-AF-003 | Out of scope | Configuration applet in next slice |
| PD-18-AF-004 | Out of scope | File-selection applet in next slice |
| PD-18-AF-005 | Out of scope | Working-memory applet in next slice |
| PD-18-AF-006 | Out of scope | Context visualizer applet hardening next |
| PD-18-AF-007 | Partial | Startup-mode pre-work only (state and validation hooks) |

## Implementation Plan

### Step 1: System host interfaces

Create host contracts in `cmd/agentx-core`:

- `system_applet_host.go`
- `system_applet_registry.go`

Minimum interface set:

- `type SystemApplet interface { ID() string; Render(ctx ContextSnapshot) []string }`
- `type SystemAppletHost interface { Resolve(tab string) (SystemApplet, bool) }`

Rules:

- Host resolution is by semantic tab key.
- Host integration must preserve title-based system-pane contract.
- Existing fallback behavior remains active for unknown tabs.

### Step 2: Context-history applet

Implement context-history applet:

- `cmd/agentx-core/system_applet_context_history.go`

Render contract:

- deterministic header block
- turn count summary
- recent prompt/response excerpts with bounded truncation
- empty-state message when no turns exist

Data source:

- existing core `/context` snapshot data model (no duplicate state owner)

### Step 3: Wire host into system rendering path

Integration target:

- `cmd/agentx-core/core.go`

Requirements:

- system tab selection resolves through host registry first
- if host has no applet for tab, existing behavior remains unchanged
- preserve dedupe and viewport clipping safeguards in system rendering path

### Step 4: Startup-mode pre-work hooks

Add non-invasive pre-work only:

- config parse scaffold for startup mode enum/string (default frame mode)
- validation path for supported values (`default`, `visible-windows`)
- no topology behavior changes yet

Suggested files:

- `cmd/agentx-core/config.go`
- `cmd/agentx-core/main.go`

## Test Plan

### Unit tests (required)

1. host resolution and fallback:
   - `cmd/agentx-core/system_applet_host_test.go`
2. context-history rendering:
   - empty history
   - single turn
   - multiple turns with truncation
   - deterministic ordering
3. startup-mode parse validation:
   - accepted values
   - invalid value error path

### Integration/package tests (required)

1. system tab routing selects context-history applet when active tab is
   `context-history`
2. title-based routing contract remains intact after layout overlay
3. existing non-context-history tabs are unaffected by this slice

Suggested target files:

- `cmd/agentx-core/core_system_renderer_test.go`
- `cmd/agentx-core/core_layout_overlay_test.go`
- `cmd/agentx-core/context_widget_test.go`

### Demo/UAT validations (required)

1. run system-tour coverage and verify context-history snapshot remains
   deterministic
2. verify parity gate remains green after slice merge

Commands:

```bash
cd /Projects/agentX/cmd/agentx-core && go test ./...
make -C /Projects/agentX test-demo-system-panel-tour-headless
make -C /Projects/agentX test-demo-ux-use-cases-headless
make -C /Projects/agentX hybrid-parity-gate
```

## Acceptance Criteria

Slice-1 is complete only when all are true:

1. host abstraction is in place and used for context-history resolution
2. context-history applet renders from core state with deterministic output
3. unit and integration tests for slice scope are added and passing
4. full parity gate remains green
5. PD-18-AF-001 and PD-18-AF-002 move from planned to tested in
   `docs/ux/UX_LIFECYCLE.md`

## Rollback Criteria

Rollback or revert slice if any of the following occurs:

- regression in tab-tour/system-pane demo stories
- parity gate fails due to system tab routing changes
- host integration breaks title-based pane routing expectations

Rollback approach:

- disable host path behind temporary feature flag if needed
- revert context-history applet routing while retaining isolated tests
- keep startup-mode parse validation only if non-breaking

## Follow-on Slice Preview

After Slice-1 completes:

- Slice-2: configuration applet (PD-18-AF-003)
- Slice-3: file-selection applet (PD-18-AF-004)
- Slice-4: working-memory applet (PD-18-AF-005)
- Slice-5: context-visualizer applet hardening (PD-18-AF-006)
- Slice-6: visible-windows startup mode behavior (PD-18-AF-007)
