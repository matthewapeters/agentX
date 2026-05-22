# AgentX UX — Working Index

_Last updated: 2026-05-22 (v0.78.1)_

> **This is the entry point for every UX work session — human or agent.**
> Open this file first. It tells you exactly where things stand and what to do next.

**Last updated**: 2026-05-22 (v0.78.1 — DemoMode now exercises the live core session)
**Current version**: see `pyproject.toml`

> **Numbering note (resolved)**: ToolPanel was erroneously assigned PD-10 in `03_PANEL_DETAILS.md`; the canonical PD-10 is ContextMeterWidget (per `UX_LIFECYCLE.md` with 7 tested affordances). ToolPanel is now **PD-13** in both files.

---

## How to Use This Page

| You want to… | Go to |
|--------------|-------|
| Understand the full lifecycle process | [UX_LIFECYCLE.md §2](UX_LIFECYCLE.md#2-the-4-phase-lifecycle) |
| Create or update a component cut-sheet | [04_COMPONENT_CUT_SHEET_TEMPLATE.md](04_COMPONENT_CUT_SHEET_TEMPLATE.md) |
| Find the affordance ID for a widget | [UX_LIFECYCLE.md §4 — Traceability Matrix](UX_LIFECYCLE.md#4-traceability-matrix-as-built) |
| See a panel's widget details, state fields, diagrams | [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md) |
| Understand the window layout and zone map | [01_MAIN_LAYOUT.md](01_MAIN_LAYOUT.md) |
| Follow a user interaction end-to-end | [02_USER_FLOWS.md](02_USER_FLOWS.md) |
| Vibe coding (neovim + tmux integration) | [05_VIBE_CODING.md](05_VIBE_CODING.md) |
| TUI mirror chat pane (spec + plan) | [06_TUI_MIRROR.md](06_TUI_MIRROR.md) |
| Demo mode contract and harness plan | [07_DEMO_MODE.md](07_DEMO_MODE.md) |
| TUI-default migration plan and docs impact list | [06_TUI_MIRROR.md §12](06_TUI_MIRROR.md#12-tui-first-migration-plan-planned) |
| Tool/tooling backlog (non-UX) | [../tools/tools_issues.md](../tools/tools_issues.md) |
| Run the UX review+enforce cycle (agent slash-command) | `/ux-review` in Copilot Chat |
| See test coverage gaps | [UX_LIFECYCLE.md §7](UX_LIFECYCLE.md#7-known-coverage-gaps) |
| Detect spec/code/test drift | [UX_LIFECYCLE.md §5.4](UX_LIFECYCLE.md#54-detecting-drift-without-a-planned-change) |
| Add a new panel/affordance (checklist) | [UX_LIFECYCLE.md §5.1](UX_LIFECYCLE.md#51-adding-a-new-affordance) |
| Modify an existing affordance (checklist) | [UX_LIFECYCLE.md §5.2](UX_LIFECYCLE.md#52-modifying-an-existing-affordance) |
| Remove an affordance (checklist) | [UX_LIFECYCLE.md §5.3](UX_LIFECYCLE.md#53-removing-an-affordance) |

---

## Current Status Snapshot

> **Agent**: when starting a session, update this table from `UX_LIFECYCLE.md §4`.
> Run the drift-detection commands in `UX_LIFECYCLE.md §5.4` and record the result here.

| Panel | Fully Tested ✅ | Partial ⚠️ | Spec Only 📝 | Gap ❌ |
|-------|:--------------:|:----------:|:------------:|:------:|
| PD-01 ChatPanel | 9 | 1 | 0 | 0 |
| PD-02 InputPanel | 9 | 3 | 0 | 0 |
| PD-03 SidePanel — Context | 7 | 0 | 0 | 0 |
| PD-03 SidePanel — Working Memory | 4 | 1 | 0 | 0 |
| PD-04 ModelSelector | 4 | 0 | 0 | 0 |
| PD-05 PlanTreeWidget | 6 | 0 | 0 | 0 |
| PD-06 ResynthesisDialog | 5 | 0 | 0 | 0 |
| PD-07 SettingsTab | 2 | 1 | 0 | 0 |
| PD-08 ContextRenderer | 4 | 0 | 0 | 0 |
| PD-09 CollapsibleSection | 4 | 0 | 0 | 0 |
| PD-10 ContextMeterWidget | 7 | 0 | 0 | 0 |
| PD-11 FileExplorer | 10 | 0 | 0 | 0 |
| PD-12 StatusTab | 11 | 0 | 0 | 0 |
| PD-13 ToolPanel | 0 | 0 | 2 | 0 |
| PD-14 VimBridge GUI | 3 | 0 | 6 | 0 |
| PD-15 TerminalPane GUI | 8 | 0 | 0 | 0 |
| PD-16 TuiMirror | 8 | 0 | 0 | 0 |
| PD-17 DemoMode | 6 | 0 | 0 | 0 |

**Totals**: 110 ✅ · 6 ⚠️ · 8 📝 · 0 ❌

---

## Priority Work Queue

> Ordered by risk (high-visibility affordances with no test coverage first).
> When picking up work, take the **top unchecked item**.

- [/] **PD-02-AF-005..007** — Attachment chip: render, remove, clear-all
- [/] **PD-03-AF-007** — Message enabled checkbox wired to `message.enabled`
- [/] **PD-03-AF-011..014** — Working Memory callbacks (toggle, delete, promote, add)
- [/] **PD-07-AF-002..003** — Settings collapse state + restart-required icon
- [/] **PD-02-AF-002** — Shift+Enter inserts newline
- [/] **PD-12: StatusTab** — implemented: `status_tab.py`, donut + break button relocated, `StreamingController` phase events wired (11 affordances ✅)
- [/] **PD-15-AF-003..004** — active terminal indicator and kill-pane action wiring in GUI/tool-result rows
- [/] **PD-04-AF-004** — ModelSelector refresh button
- [/] **PD-05-AF-004** — Re-synth button in synthesis block
- [/] **PD-05-AF-005** — Export button in plan tab toolbar
- [/] **PD-05-AF-006** — Node status icon reflects task state
- [/] **PD-01-AF-010** — Output panel right-click copy context menu (Wayland-safe)
- [/] **PD-02-AF-008..012** — Input panel right-click context menu: popup, Copy/Paste visibility, Copy action, Paste action
- [/] **PD-16 (TuiMirror)** — implemented: config + NullGUI path, bridge I/O, launcher lifecycle, and traceability/test reconciliation completed
- [ ] **PD-16 migration (planned)** — invert launcher default to TUI-first, add `--gui` opt-in path, and reconcile docs/ux artifacts per `06_TUI_MIRROR.md §12.6`

---

## Agile Process: How a UX Change Flows

```
┌──────────────────────────────────────────────────────────────────────────┐
│  SESSION START                                                           │
│  1. Open this file (00_INDEX.md)                                         │
│  2. Check Status Snapshot — note any pre-existing ⚠️ or ❌              │
│  3. Pick top item from Priority Work Queue                               │
└────────────────────────┬─────────────────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  SPECIFY                                                                 │
│  - Write/update affordance in 03_PANEL_DETAILS.md                       │
│  - Assign PD-XX-AF-NNN if new                                            │
│  - Update ASCII/Mermaid diagram                                          │
│  - Add row to UX_LIFECYCLE.md §4 (status: 📝)                           │
│  - Commit: "spec(PD-XX): ..."                                            │
└────────────────────────┬─────────────────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  GHERKIN FIRST                                                           │
│  - Write GIVEN/WHEN/THEN in test docstring                               │
│  - Include [PD-XX-AF-NNN] in docstring                                   │
│  - Write assertions that will FAIL against current code                  │
│  - Run tests → confirm they fail (red)                                   │
└────────────────────────┬─────────────────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  CODE                                                                    │
│  - Implement minimum code to pass the failing tests                      │
│  - Reference affordance ID in source docstring                           │
│  - Run tests → all green, no regressions                                 │
└────────────────────────┬─────────────────────────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────────────────────────┐
│  RECONCILE                                                               │
│  - Update UX_LIFECYCLE.md §4 status → ✅                                │
│  - Remove from Priority Work Queue                                       │
│  - Bump version + update CHANGELOG.md                                    │
│  - Commit: "ux(PD-XX): ..."                                              │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Quick Drift Check

Run this at the start of any UX session to identify gaps immediately:

```bash
# IDs in tests but missing from UX_LIFECYCLE.md
grep -rho 'PD-[0-9]\+-AF-[0-9]\+' tests/ | sort -u > /tmp/tested.txt
grep -oh 'PD-[0-9]\+-AF-[0-9]\+' docs/ux/UX_LIFECYCLE.md | sort -u > /tmp/specced.txt
echo "=== Tested but not in matrix ==="
comm -23 /tmp/tested.txt /tmp/specced.txt
echo "=== In matrix but no tests yet ==="
comm -13 /tmp/tested.txt /tmp/specced.txt
```

---

## Document Map

```
docs/ux/
├── 00_INDEX.md                        ← YOU ARE HERE — session entry point
├── README.md                          ← Overview and navigation guide
├── UX_LIFECYCLE.md                    ← Lifecycle rules, affordance IDs, traceability matrix
├── 01_MAIN_LAYOUT.md                  ← Window zones, geometry
├── 02_USER_FLOWS.md                   ← End-to-end user interaction flows (Mermaid)
├── 03_PANEL_DETAILS.md                ← Per-panel: affordance tables, state fields, diagrams
├── 04_COMPONENT_CUT_SHEET_TEMPLATE.md ← Blank template for new component cut-sheets
├── 07_DEMO_MODE.md                    ← Demo mode UX contract and implementation plan
└── UX_ISSUES.md                       ← Bug-tracking log for user-reported UX defects

.github/prompts/
└── ux-review.prompt.md      ← /ux-review slash command (8-phase TDD review loop)
```

---

## Conventions Reminder

| Topic | Rule |
|-------|------|
| Affordance ID format | `PD-<panel>-AF-<NNN>` (zero-padded three digits) |
| Spec freeze | Spec must be committed before code changes |
| Gherkin oracle | GIVEN/WHEN/THEN drives assertions; update Gherkin first, code second |
| Regression gate | No commit if a test that previously passed now fails |
| Coverage floor | Unit test coverage must stay ≥ 98% |
| Source docstring | Must contain `[PD-XX-AF-NNN]` for every affordance implemented |
| Test docstring | Must contain `[PD-XX-AF-NNN]` and Gherkin use-case |

---

## Requirement Intake (Single Mechanism)

Use this exact sequence for every new UX requirement.

1. Start spec-first in [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md) using
    [04_COMPONENT_CUT_SHEET_TEMPLATE.md](04_COMPONENT_CUT_SHEET_TEMPLATE.md).
2. If new behavior is introduced, assign new `PD-XX-AF-NNN` IDs.
3. Add or update traceability row(s) in [UX_LIFECYCLE.md](UX_LIFECYCLE.md#4-traceability-matrix-as-built) with `📝` status.
4. Write/update Gherkin scenarios in tests before code changes.
5. Run tests red, implement code, run tests green.
6. Reconcile matrix status to `✅` and update this index status snapshot.
