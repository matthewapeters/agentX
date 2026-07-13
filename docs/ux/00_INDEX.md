# AgentX UX — Working Index

> **This is the entry point for every UX work session — human or agent.**
> Open this file first, then follow the links below for the detail you need.

_Last updated: 2026-07-13_

AgentX is a Go **client-server** app (see `CLAUDE.md`): a core orchestration
**server**, and **surfaces** as separate client processes attaching over HTTP/SSE.
The chat surface has **two panels** (output + input); the former single "system"
panel is now a set of **independent, separately launchable surfaces** (files,
config, context, context-history, context-visualizer) that the user arranges via a
terminal multiplexer. See
[`../architecture/00_ARCHITECTURE_RECONCILIATION.md`](../architecture/00_ARCHITECTURE_RECONCILIATION.md)
for the Family A (build now) vs. Family B (future) split.

## Reconciliation status

[`03_PANEL_DETAILS.md`](03_PANEL_DETAILS.md) was rewritten to the current
implementation (2026-07-12) and is the authoritative per-surface reference. The
other UX documents below are at varying points in that same migration — each
carries its own banner stating whether it's reconciled or still describes the
prior single-window split-pane GUI:

| Document | Status |
|----------|--------|
| [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md) | Reconciled — current implementation |
| [UX_LIFECYCLE.md](UX_LIFECYCLE.md) | Not yet reconciled — traceability matrix still keyed to legacy `PD-xx` IDs; see its banner and `03_PANEL_DETAILS.md`'s "Retired affordances" table for the ID mapping |
| [07_DEMO_MODE.md](07_DEMO_MODE.md) | Not yet reconciled — still targets the prior GUI |
| [01_MAIN_LAYOUT.md](01_MAIN_LAYOUT.md), [02_USER_FLOWS.md](02_USER_FLOWS.md) | Mixed — some diagrams predate the client-server split |

## How to Use This Page

| You want to… | Go to |
|--------------|-------|
| See a surface's widget details, state fields, diagrams | [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md) |
| Understand the affordance lifecycle and ID scheme | [UX_LIFECYCLE.md](UX_LIFECYCLE.md) |
| Create or update a component cut-sheet | [04_COMPONENT_CUT_SHEET_TEMPLATE.md](04_COMPONENT_CUT_SHEET_TEMPLATE.md) |
| Understand the window layout and zone map | [01_MAIN_LAYOUT.md](01_MAIN_LAYOUT.md) |
| Follow a user interaction end-to-end | [02_USER_FLOWS.md](02_USER_FLOWS.md) |
| Vibe coding (editor + terminal integration) | [05_VIBE_CODING.md](05_VIBE_CODING.md) |
| Output-panel widget contract (Plan widget, collapsing, etc.) | [06_OUTPUT_WIDGET.md](06_OUTPUT_WIDGET.md) |
| Demo mode contract and harness plan | [07_DEMO_MODE.md](07_DEMO_MODE.md) |
| Report or look up a UX bug | [UX_ISSUES.md](UX_ISSUES.md) |
| Tool/tooling backlog (non-UX) | [../tools/tools_issues.md](../tools/tools_issues.md) |

---

## Document Map

```
docs/ux/
├── 00_INDEX.md                        ← YOU ARE HERE — session entry point
├── README.md                          ← Overview and quick-reference tables
├── UX_LIFECYCLE.md                    ← Lifecycle rules, affordance IDs, traceability matrix
├── 01_MAIN_LAYOUT.md                  ← Window zones, geometry
├── 02_USER_FLOWS.md                   ← End-to-end user interaction flows (Mermaid)
├── 03_PANEL_DETAILS.md                ← Per-surface: affordance tables, state fields, diagrams
├── 04_COMPONENT_CUT_SHEET_TEMPLATE.md ← Blank template for new component cut-sheets
├── 05_VIBE_CODING.md                  ← Editor + terminal integration UX contract
├── 06_OUTPUT_WIDGET.md                ← Output-panel widget contract
├── 07_DEMO_MODE.md                    ← Demo mode UX contract and implementation plan
└── UX_ISSUES.md                       ← Bug-tracking log for user-reported UX defects
```

---

## Conventions Reminder

| Topic | Rule |
|-------|------|
| Affordance ID format | `PD-<panel>-AF-<NNN>` (zero-padded three digits) |
| Spec freeze | Spec must be committed before code changes |
| Behavior contract | Every touched function requires a GIVEN/WHEN/THEN behavior doc before implementation (`CLAUDE.md` key invariants) |
| Test tagging | Godog tests use the `@unit`, `@integration`, `@functional`, `@e2e` tag scheme |
| Regression gate | `make all` must pass before merge; no commit if a previously-passing test now fails |

---

## Requirement Intake (Single Mechanism)

Use this exact sequence for every new UX requirement.

1. Start spec-first in [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md) using
    [04_COMPONENT_CUT_SHEET_TEMPLATE.md](04_COMPONENT_CUT_SHEET_TEMPLATE.md).
2. If new behavior is introduced, assign new `PD-XX-AF-NNN` IDs.
3. Write the GIVEN/WHEN/THEN behavior doc and the corresponding Godog scenario
   before code changes.
4. Run tests red, implement code, run tests green.
5. Update the affected surface's section in `03_PANEL_DETAILS.md` and, once
   reconciled, its row in `UX_LIFECYCLE.md`.
