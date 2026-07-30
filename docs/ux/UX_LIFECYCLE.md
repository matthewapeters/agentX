# AgentX — UX Lifecycle Reference

<!-- markdownlint-disable MD036 MD040 MD047 MD051 MD060 -->

> **⚠️ Architecture migration (2026-06-26), not yet reconciled.** The traceability
> matrix below maps affordances (PD-01…PD-17) to the prior single-window split-pane
> GUI. AgentX is now a **client-server** app: the chat surface has **two panels
> (output + input)** and the former "system" surface is now **multiple independent,
> separately launchable surfaces**. `03_PANEL_DETAILS.md` was rewritten to the current
> implementation (2026-07-12) and its **"Retired affordances"** table maps each
> legacy `PD-xx` ID below to its current disposition — consult that table when a row
> here points at something that no longer exists. This matrix itself has not yet had
> the same rewrite pass (tracked in `nits.md`); treat surface/host columns and any
> row citing a nonexistent file/target as **stale** until that reconciliation lands.
> See [`../architecture/00_ARCHITECTURE_RECONCILIATION.md`](../architecture/00_ARCHITECTURE_RECONCILIATION.md).

_Last updated: 2026-06-01 (v0.85.0)_
**Purpose**: Single source of truth for the complete lifecycle of every user-facing
UI feature — from first written description through code implementation, hermetic
testing, and as-built reconciliation.  Both the developer and the AI agent refer to
this document when specifying, building, changing, or removing UI affordances.

Authoritative contract rule:

- UX requirements in this document are implementation-agnostic.
- Delivery technology (GUI, TUI, hybrid) is an implementation concern and must
  conform to UX requirements, not redefine them.

---

## Tab/Surface Parity Matrix (Source of Truth)

The rows below define required parity outcomes for user-visible surfaces.
Implementation details are supportive metadata for as-built reconciliation.

|Feature|Output Surface|Input Surface|System Surface|Status|
|---|---|---|---|---|
|Chat Messages|Primary output|N/A|N/A|Implemented|
|Tool Processing|Primary output|N/A|N/A|Implemented|
|File Display|Primary output|N/A|N/A|Implemented|
|Context Visualization|N/A|N/A|Primary display|Implemented|
|Files Navigation|N/A|N/A|Primary display|Implemented|
|Context History|N/A|N/A|Primary display|Implemented|
|Configuration|N/A|N/A|Primary display|Implemented|

**Design Principle:** All surfaces are equally important and must meet the same UX quality standards.

---

---

## Table of Contents

1. [Why UI Features Drift](#1-why-ui-features-drift)
2. [The 4-Phase Lifecycle](#2-the-4-phase-lifecycle)
3. [Affordance ID Scheme](#3-affordance-id-scheme)
4. [Traceability Matrix (As-Built)](#4-traceability-matrix-as-built)
5. [Change Workflow Checklists](#5-change-workflow-checklists)
6. [Current Coverage Gaps](#6-current-coverage-gaps)
7. [Keeping This Document Current](#7-keeping-this-document-current)

---

## 1. Why UI Features Drift

A UI affordance (a button, a keyboard shortcut, a collapse animation, a colour change)
exists in three independent layers:

| Layer | Responsibility | Can change independently |
|-------|---|---|
| **Specification** | UX requirements and behavior | Yes (requires propagation) |
| **Implementation** | Runtime code/runtime logic | Yes (requires spec + test update) |
| **Testing** | Acceptance criteria and validation | Yes (requires spec + impl update) |

When a change is made in one layer without updating the others, drift occurs:

- Implementation change without spec update → spec becomes stale ("as-designed" ≠ "as-built").
- Implementation change without test update → tests stop validating correctly (invisible drift — worst case).
- Spec change without implementation or test → description of desired future state looks like current state.

The solution is a **traceability chain**: every affordance carries an ID that appears
in the spec document, in implementation docstrings, and in test docstrings. When
any one layer changes you can immediately find and update the others by searching for
that ID.

---

## 2. The 4-Phase Lifecycle

```text
 ┌────────────────────────────────────────────────────────────────────────┐
 │                      UX FEATURE LIFECYCLE                              │
 │                                                                        │
 │  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────────────┐ │
 │  │ 1. SPEC  │───▶│ 2. CODE  │───▶│ 3. TEST  │───▶│ 4. RECONCILE     │ │
 │  │          │    │          │    │          │    │ (As-Built Update) │ │
 │  └──────────┘    └──────────┘    └──────────┘    └──────────────────┘ │
 │       ▲                                                    │           │
 │       └────────────────────────────────────────────────────┘           │
 │                     (next iteration)                                   │
 └────────────────────────────────────────────────────────────────────────┘
```

### Phase 1 — Specify

Write or update the affordance description **before** writing any code.

- Use or create a Panel Detail section in [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md)
  (e.g. `PD-12: NewWidget`).
- Assign an Affordance ID for each distinct user-visible behaviour within that panel
  (e.g. `PD-12-AF-001`).  See §3 for the ID scheme.

### Phase 2 — Implementation

Implement the feature.

- Reference the Affordance ID in the implementation documentation.
- If the implementation diverges from the spec, update the spec first.
  Never silently let implementation and spec disagree.
- Do not add placeholder behavior intended for later.  Mark those with the
  `TODO(PD-XX-AF-YYY):` prefix so they are findable.

**Output**: Committed implementation changes with spec IDs in docstrings.

### Phase 3 — Testing

Write tests to validate the feature.

- Reference the Affordance ID in the test documentation.
- Every user-visible **state change** triggered by the affordance needs at least one test.
  Minimum coverage:
  - Default/initial state is correct.
  - Primary user action produces the expected state change.
  - Edge cases (empty content, disabled state, overflow).
- Tests should be deterministic and isolated (no external dependencies when possible).

**Output**: Committed tests that validate the affordance behavior.

### Phase 4 — Reconcile (As-Built Update)

Update this document to reflect what was built and tested.

- Change the traceability row status from `📝 Spec Only` to `✅ Tested`.
- If the implementation differs from the spec in any way, update the spec to match and
  note the change in `CHANGELOG.md`.
- Commit `docs/ux/UX_LIFECYCLE.md` alongside the implementation and test changes.

### Applet Review Gate

The current runtime's system applets — the independent, separately launchable
surfaces (files, config, context, context-history, context-visualizer) — are treated
as first-class UX surfaces even when they are implemented as TUI applets rather than
GUI widgets.

Before a parity claim is made for any system applet or startup topology change,
the following must be true:

- The applet or surface has a dedicated spec row in [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md).
- The same affordance ID appears in the traceability matrix below.
- The implementation has unit coverage for the default state and every user-
  visible state transition.
- The implementation has integration or functional coverage for launch,
  reattach, and session ownership behavior.
- The UX review has been completed against the authoritative spec, and the row
  status has been reconciled from `📝 Spec Only` to `✅ Tested`.

This gate applies to the system applet suite, to the visible-windows startup
mode, and to any future frame/container topology changes that affect what UAT can
see and validate.

---

## 3. Affordance ID Scheme

```
PD-<panel number>-AF-<three-digit sequence>

Examples:
  PD-01-AF-001   OutputSurface — turn order invariant (user entry above children)
  PD-08-AF-001   ContextRenderer — expand/collapse button in every message row
  PD-10-AF-003   ContextMeterWidget — border turns red at warning threshold
```

### Panel Numbers Quick Reference

| PD | Feature |
|----|---------|
| PD-01 | Chat/Output |
| PD-02 | User Input |
| PD-03 | Context/Navigation |
| PD-04 | Model Configuration |
| PD-05 | Plan Visualization |
| PD-06 | Dialog/Resynthesis |
| PD-07 | Settings |
| PD-08 | Context Rendering |
| PD-09 | Collapsible Sections |
| PD-10 | Context Metrics |
| PD-11 | File Navigation |
| PD-12 | Status Display |
| PD-13 | Tool Processing |
| PD-14 | Keyboard Integration |
| PD-15 | System Integration |
| PD-16 | Multi-Surface Sync |
| PD-17 | Demo Mode |
| PD-CTX | Context Surface (TUI) | New (M2) |
| PD-CTXVIZ | Context Visualizer (TUI) | New (M2) |
| PD-WM | Working Memory Editor (TUI) | New (M2) |
| PD-LOGS | Log/Trace Surface (TUI) | New (M2+) |
| PD-CONFIG | Configuration Surface (TUI) | Phase 1a complete (M2+, in progress) |

When a new surface feature is added, assign the next available PD number,
add a row to this table, and create a section in `03_PANEL_DETAILS.md`.

---

## 4. Traceability Matrix (As-Built)

This table is the **as-built record**.  It maps each spec section to the code that
implements it and the test that validates it.  Status legend:

| Symbol | Meaning |
|--------|---------|
| ✅ | Spec, implementation, and tests all exist and agree |
| ⚠️ | Implementation exists but tests cover only a subset of spec affordances |
| 📝 | Spec exists; no implementation yet |
| ❌ | Known gap — either no spec, no implementation, or no tests |

### PD-01 — Chat/Output

| Affordance | ID | Status |
|------------|----|--------|
| User entry messages packed in turn order | PD-01-AF-001 | ✅ |
| Collapse hides child messages | PD-01-AF-002 | ✅ |
| Expand shows child messages | PD-01-AF-003 | ✅ |
| Multiple turns maintain independent order | PD-01-AF-004 | ✅ |
| Thinking blocks collapsed by default | PD-01-AF-005 | ✅ |
| Tool calls collapsed by default | PD-01-AF-006 | ✅ |
| Assistant responses expanded by default | PD-01-AF-007 | ✅ |
| Markdown rendering after stream completion | PD-01-AF-008 | ⚠️ |
| Startup log-location notice shown (config-gated) | PD-01-AF-009 | ✅ |
| Right-click opens copy context menu on output | PD-01-AF-010 | ✅ |

### PD-02 — User Input

| Affordance | ID | Status |
|------------|----|--------|
| Enter key submits message | PD-02-AF-001 | ⚠️ |
| Shift+Enter inserts newline | PD-02-AF-002 | ✅ |
| Send button disabled during streaming | PD-02-AF-003 | ⚠️ |
| Stop button enabled during streaming | PD-02-AF-004 | ⚠️ |
| Attachment chip rendered with filename/icon | PD-02-AF-005 | ✅ |
| Toggle chip calls attachment callback | PD-02-AF-006 | ✅ |
| Clear attachments removes all chips | PD-02-AF-007 | ✅ |
| Right-click opens context menu on input | PD-02-AF-008 | ✅ |
| Copy menu item shown when text selected | PD-02-AF-009 | ✅ |
| Paste menu item shown when clipboard non-empty | PD-02-AF-010 | ✅ |
| Copy action copies selection to clipboard | PD-02-AF-011 | ✅ |
| Paste action replaces selection or inserts | PD-02-AF-012 | ✅ |

### PD-03 — Context/Navigation

| Affordance | ID | Status |
|------------|----|--------|
| Message row has expand/collapse button | PD-03-AF-001 | ✅ |
| Full content hidden by default | PD-03-AF-002 | ✅ |
| Full content visible after expand | PD-03-AF-003 | ✅ |
| Plan rows grouped under preceding message | PD-03-AF-004 | ✅ |
| Plan header clickable when callback provided | PD-03-AF-005 | ✅ |
| Plan/task rows excluded from LLM messages | PD-03-AF-006 | ✅ |
| Message enabled checkbox wired to state | PD-03-AF-007 | ✅ |
| Working memory fact row rendered per fact | PD-03-AF-010 | ⚠️ |
| Working memory toggle calls callback | PD-03-AF-011 | ✅ |
| Working memory delete calls callback | PD-03-AF-012 | ✅ |
| Working memory promote calls callback | PD-03-AF-013 | ✅ |
| Add-fact form submits user input | PD-03-AF-014 | ✅ |
| Working memory section starts collapsed | PD-03-AF-015 | ✅ |

### PD-04 — Model Configuration

| Affordance | ID | Status |
|------------|----|--------|
| Model selection updates active model | PD-04-AF-001 | ✅ |
| Model change triggers context meter redraw | PD-04-AF-002 | ✅ |
| Model name resolves via fallback | PD-04-AF-003 | ✅ |
| Refresh button reloads model list | PD-04-AF-004 | ✅ |

### PD-05 — Plan Visualization

| Affordance | ID | Status |
|------------|----|--------|
| Plan header row rendered with name | PD-05-AF-001 | ✅ |
| Task node rows indented under plan | PD-05-AF-002 | ✅ |
| Step count badge shown in header | PD-05-AF-003 | ✅ |
| Re-synthesize button opens dialog | PD-05-AF-004 | ✅ |
| Export button saves and opens file | PD-05-AF-005 | ✅ |
| Node status icon reflects task state | PD-05-AF-006 | `PlanView.UpdateNodeStatus()` | — | — | ✅ |

### PD-06 — ResynthesisDialog

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Dialog title includes task_id | PD-06-AF-001 | `ResynthesisDialog.Init()` | — | — | ✅ |
| Cancel closes dialog without calling on_confirm | PD-06-AF-002 | `ResynthesisDialog.Destroy()` | — | — | ✅ |
| Re-synthesise calls on_confirm with hint text | PD-06-AF-003 | `ResynthesisDialog.OnConfirmClicked()` | — | — | ✅ |
| WM hint section hidden/visible based on callback | PD-06-AF-004 | `ResynthesisDialog.Init()` | — | — | ✅ |
| Add WM hint calls callback and clears fields | PD-06-AF-005 | `ResynthesisDialog.OnAddWMHintClicked()` | — | — | ✅ |

### PD-07 — SettingsTab

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Theme toggle persists to agentx.toml | PD-07-AF-001 | `SettingsSurface.OnThemeChange()` | — | — | ⚠️ |
| Settings sections collapsed/expanded correctly | PD-07-AF-002 | `SettingsSurface.Init()` / `SettingsSurface.MakeSection()` | — | — | ✅ |
| Restart-required fields show 🔁 icon in label | PD-07-AF-003 | `SettingsSurface.RestartIcon` / `AddCheckbox()` / `AddTextEntry()` / `AddSpinbox()` | — | — | ✅ |

### PD-08 — ContextRenderer (factory methods)

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Expand button always created for every message | PD-08-AF-001 | `_render_message_to_grid()` | — | — | ✅ |
| Tool rows appended to collapsible list | PD-08-AF-002 | `_render_tool_rows()` | — | — | ✅ |
| Plan/task_node rows appended to collapsible list | PD-08-AF-003 | `_render_plan_rows()` | — | — | ✅ |
| Empty-content message has button but no detail row | PD-08-AF-004 | `_render_message_to_grid()` | — | — | ✅ |

### PD-09 — CollapsibleSection

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Section starts collapsed when default_open=False | PD-09-AF-001 | `CollapsibleSection.Init()` | — | — | ✅ |
| Section starts expanded when default_open=True | PD-09-AF-002 | `CollapsibleSection.Init()` | — | — | ✅ |
| Header click toggles content visibility | PD-09-AF-003 | `CollapsibleSection.Toggle()` | — | — | ✅ |
| set_content replaces previous content | PD-09-AF-004 | `CollapsibleSection.SetContent()` | — | — | ✅ |

### PD-10 — ContextMeterWidget

> ⚠️ **Relocation pending (PD-12 implementation)**: `ContextMeterWidget.create()` will be called from `StatusTab` instead of `InputPanel`. All PD-10 affordances are unchanged; only the host frame changes. See `PD-12-AF-011`.

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Meter creates canvas on first create() call | PD-10-AF-001 | `ContextMeterWidget.Create()` | — | — | ✅ |
| Arc slices sized proportionally to token counts | PD-10-AF-002 | `ContextMeterWidget.DrawArcs()` | — | — | ✅ |
| Ghost arc shows remaining capacity | PD-10-AF-003 | `ContextMeterWidget.DrawArcs()` | — | — | ✅ |
| Border turns warning-red at 80% | PD-10-AF-004 | `ContextMeterWidget.RiskState()` | — | — | ✅ |
| Border turns critical-red at 100% | PD-10-AF-005 | `ContextMeterWidget.RiskState()` | — | — | ✅ |
| update() is thread-safe | PD-10-AF-006 | `ContextMeterWidget.Update()` | — | — | ✅ |
| max_tokens=0 does not crash | PD-10-AF-007 | `ContextMeterWidget.DrawArcs()` | — | — | ✅ |

### PD-11 — FileExplorer

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| List directory populates widget | PD-11-AF-001 | `FileBrowser.ListDirectory()` | — | — | ✅ |
| Change directory navigates and lists | PD-11-AF-002 | `FileBrowser.ChangeDirectory()` | — | — | ✅ |
| Back/Forward navigate history | PD-11-AF-003 | `FileBrowser.NavigateBackForward()` | — | — | ✅ |
| Home button navigates to home dir | PD-11-AF-004 | `FileBrowser.NavigateHome()` | — | — | ✅ |
| Parent button navigates up one level | PD-11-AF-005 | `FileBrowser.NavigateParent()` | — | — | ✅ |
| Open file triggers callback | PD-11-AF-006 | `FileBrowser.OpenFile()` | — | — | ✅ |
| Theme applies correct colours | PD-11-AF-007 | `FileBrowser.ApplyTheme()` | — | — | ✅ |
| Right-click on file shows file context menu | PD-11-AF-008 | `FileBrowser.OnRightClick()` | — | — | ✅ |
| Right-click on directory shows folder context menu | PD-11-AF-009 | `FileBrowser.OnRightClick()` | — | — | ✅ |
| Escape dismisses context menu | PD-11-AF-010 | `FileBrowser.DismissPopupMenu()` | — | — | ✅ |
| Overflow navigation keeps selected row visible in terminal viewport | PD-11-AF-011 | `filesystemWidgetState.ensureSelectionVisible()` + overflow render contract in `filesystemWidgetState.render()` | `filesystem_widget_test.go` | `TestFilesystemWidgetRender_OverflowOrientationAndSelectionVisibility` | ✅ |
| Overflow navigation supports PageUp/PageDown/Home/End (or equivalent) | PD-11-AF-012 | `filesystemWidgetState.handleCommand()` (`pgup`/`pgdn`/`top`/`end`) | `filesystem_widget_test.go` | `TestFilesystemWidgetHandleCommand_PageNavigation` | ✅ |
| Overflow status exposes visible range orientation (`showing X-Y of Z`) | PD-11-AF-013 | `filesystemWidgetState.render()` visible range header | `filesystem_widget_test.go` | `TestFilesystemWidgetRender_OverflowOrientationAndSelectionVisibility` | ✅ |
| Files parity sign-off requires overflow-list executable evidence | PD-11-AF-014 | filesystem widget contracts (the `hybrid-parity-gate` Makefile target and `tests/test_demo_system_panel_tour_headless.sh` this row originally cited no longer exist — stale, unreconciled) | `filesystem_widget_test.go` | package-level tests | ⚠️ |
| TUI files applet supports arrow-key row navigation | PD-11-AF-015 | `normalizeFilesystemWidgetCommand()` maps `Up`/`Down` to row navigation | `filesystem_widget_test.go` | `TestNormalizeFilesystemWidgetCommand` | ✅ |
| TUI files applet supports `Space` soft-select semantics with visible state | PD-11-AF-016 | `filesystemWidgetState.toggleSoftSelection()` + soft-select rendering + multi-action compatibility | `filesystem_widget_test.go` | `TestFilesystemWidgetHandleCommand_SoftSelectToggleVisibleInRender`, `TestFilesystemWidgetHandleCommand_AttachUsesSoftSelectedSetStatus`, `TestFilesystemWidgetHandleCommand_EditUsesSoftSelectedSetInViewOrder` | ✅ |
| TUI files applet supports `Return` hard-select primary action semantics | PD-11-AF-017 | `filesystemWidgetState.activateSelection()` via `enter` command path | `filesystem_widget_test.go` | `TestFilesystemWidgetHandleCommand_ReturnHardSelectActivates` | ✅ |
| No-obvious-path affordances require explicit user case-by-case decision before closure | PD-11-AF-018 | Path-A decision log + applet-governance escalation contract | `docs/architecture/applets/README.md`, `00_START_HERE.md` | decision-log governance evidence | ✅ |

### PD-12 — StatusTab

| Affordance | ID | Source | Test File | Test Class | Status |
|---|---|---|---|---|---|
| Status tab is first in system notebook | PD-12-AF-001 | `SystemSurface.Create()` | — | — | ✅ |
| Auto-switch to Status tab on prompt submit | PD-12-AF-002 | `StreamingController.OnStreamStart()` | — | — | ✅ |
| Interrupt button enables/disables with streaming | PD-12-AF-003 | `StatusTab.SetStreamingState()` | — | — | ✅ |
| Interrupt button invokes callback | PD-12-AF-004 | `StatusTab` interrupt button command | — | — | ✅ |
| Phase rows reset at stream start | PD-12-AF-005 | `StatusTab.Reset()` | — | — | ✅ |
| Phase row transitions to RUNNING / starts timer | PD-12-AF-006 | `StatusTab.SetPhase()` | — | — | ✅ |
| Phase row transitions to DONE / freezes timer | PD-12-AF-007 | `StatusTab.SetPhase()` | — | — | ✅ |
| Phase row transitions to FAILED | PD-12-AF-008 | `StatusTab.SetPhase()` | — | — | ✅ |
| Tool step label updates with active tool name | PD-12-AF-009 | `StatusTab.SetPhase()` | — | — | ✅ |
| Colour-key legend rows match donut bands | PD-12-AF-010 | `ContextKeyWidget` | — | — | ✅ |
| ContextMeterWidget hosted in StatusTab | PD-12-AF-011 | `StatusTab.Create()` | — | — | ✅ |

### PD-13 — ToolPanel

> ⚠️ **Note**: ToolPanel was previously numbered PD-10 in `03_PANEL_DETAILS.md` — renumbered to PD-13 to resolve conflict with ContextMeterWidget (PD-10 in `UX_LIFECYCLE.md`). ToolPanel has no affordance IDs yet.

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| Checkbox per tool toggles enabled state | PD-13-AF-001 | `ToolPanel.on_tool_toggle()` | — | — | 📝 |
| Expand/collapse panel header | PD-13-AF-002 | `ToolPanel` header toggle | — | — | 📝 |

### PD-14 — VimBridge GUI

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| "Edit" context menu opens file in editor as new buffer; no buffers closed | PD-14-AF-002 | `EditorBridge.OpenFile()` + `Orchestrator.OpenFileInEditor()` + `FileBrowser.OnEditSelected()` | — | — | ✅ |
| is_connected() returns True when editor socket is available | PD-14-AF-002a | `EditorBridge.IsConnected()` | — | — | ✅ |
| Path resolution forwards absolute/relative paths correctly | PD-14-AF-002b | `EditorBridge.OpenFileFromContext()` | — | — | ✅ |
| Editor status bar shows connected state | PD-14-AF-001 | — | — | — | 📝 |
| Send to Editor button enabled when connected | PD-14-AF-003 | — | — | — | 📝 |
| Send to Editor button disabled when disconnected | PD-14-AF-003 | — | — | — | 📝 |
| Line navigation from error display opens file at line N | PD-14-AF-004 | — | — | — | 📝 |
| File-saved notification shown in OutputSurface | PD-14-AF-005 | — | — | — | 📝 |
| Recover-editor restores editing surface | PD-14-AF-008 | `EditorBridge` recovery path | — | — | ✅ |

### PD-15 — TerminalPane GUI

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| Active terminal pane indicator updates in input status strip | PD-15-AF-003 | `InputSurface.SetTerminalStatus()` + `StreamingController.HandleTerminalToolResult()` | — | — | ✅ |
| Tool-result row exposes kill-pane action and callback wiring | PD-15-AF-004 | `OutputSurface.SetToolResultKillAction()` + `Orchestrator.HandleTerminalKillPane()` | — | — | ✅ |
| Input-strip mode toggle switches supervised/autonomous with confirmation gate | PD-15-AF-005 | `InputSurface.OnTerminalModeToggle()` + `Orchestrator.HandleTerminalModeToggle()` | — | — | ✅ |
| Supervised confirm-list commands route through interactive approval dialog | PD-15-AF-006 | `Orchestrator.RequestTerminalApproval()` + `ShowTerminalApprovalDialog()` | — | — | ✅ |
| Settings editor updates allow/confirm/deny permission prefixes with save/reset controls | PD-15-AF-007 | `SettingsSurface.SaveTerminalPermissionLists()` + `ResetTerminalPermissionLists()` | — | — | ✅ |
| Terminal bridge stop signal gracefully shuts down active terminal surfaces | PD-15-AF-008 | `TerminalBridge` stop path | — | — | ✅ |
| `terminal_run()` wrapper resolves `visible`/`auto_close`/`timeout_sec` from config when caller omits them | PD-15-AF-009 | `TerminalBridge.Run()` defaults resolution | — | — | ✅ |
| Streamed tool-result rows for `terminal_run` include a decision badge (✅/⛔/🚫) and exit code | PD-15-AF-010 | `StreamingController.DisplayToolResult()` badge injection | — | — | ✅ |

### PD-16 — TuiMirror

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| Output writer emits USER/AGENT/TOOL/DONE records without blocking surface path | PD-16-AF-001 | `TuiBridge.WriteOutput()` + `StreamingController.Display*()` hooks | — | — | ✅ |
| Input reader parses submit sentinel and dispatches prompt callbacks | PD-16-AF-002 | `TuiBridge.InputReaderLoop()` | — | — | ✅ |
| Launcher creates TUI surface with env wiring | PD-16-AF-003 | TUI launch path | — | — | ✅ |
| TUI config is written and sourced at startup | PD-16-AF-004 | TUI launch configuration writer | — | — | ✅ |
| Submit keymap writes input text with submit sentinel | PD-16-AF-005 | TUI submit keymap | — | — | ✅ |
| `enable_gui_chat=false` mode uses headless surface manager and enforces config constraint | PD-16-AF-006 | `config.ValidateConfig()` + `Orchestrator` GUI-disabled path | — | — | ✅ |
| `tui.enable` controls `TuiBridge` lifecycle and guarded call-sites | PD-16-AF-007 | `Orchestrator.Init()` + `Close()` + streaming guards | — | — | ✅ |
| Quit keymap writes quit sentinel and triggers graceful application shutdown from TUI | PD-16-AF-008 | TUI quit keymap + `TuiBridge.InputReaderLoop()` + `Orchestrator.OnTuiQuit()` | — | — | ✅ |
| TUI context visualization renders color-band meter and top-contributor bars with ASCII fallback | PD-16-AF-009 | `TuiBridge.RenderContextVisualization()` + `Orchestrator.ScheduleMeterRedraw()` | — | — | ✅ |
| Go input widget consumes core activity state (`/activity`) and renders non-blocking busy/done/fail cues | PD-16-AF-010 | `input_widget.go` activity polling + prompt adornment (`fetchActivitySnapshot()` / `activityPromptLabel()`) + core `/activity` handler | `input_widget_test.go`, `core_health_endpoint_test.go` | package-level tests | ✅ |

### PD-17 — DemoMode

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| `--demo` enters DemoMode execution path | PD-17-AF-001 | `main.go` flag parse + `runDemoSplitMode()` / controller entry paths | `demo_split_mode_test.go`, `demo_harness_test.go` | package-level tests | ✅ |
| Demo sequence list shown before execution | PD-17-AF-002 | `renderDemoSequence()` ordered sequence renderer | `demo_harness_test.go` | package-level tests | ✅ |
| Start test selection by id/index (`--demo-start`) | PD-17-AF-003 | `resolveDemoStartIndex()` selector parser | `demo_harness_test.go` | package-level tests | ✅ |
| Split demo left workspace has story-browser (top) and prompt pane (bottom) | PD-17-AF-004 | `runDemoSplitMode()` pane layout orchestration | `demo_split_mode_test.go` | package-level tests | ✅ |
| Per-test prompt supports `N`, `J <num>`, `X <feedback>` with validation | PD-17-AF-005 | `readDemoDecision()` and `runDemoMode()` decision state machine | `demo_harness_test.go` | package-level tests | ✅ |
| `X` triggers pane/metadata dump artifact bundle | PD-17-AF-006 | `defaultDemoDiagnosticsCollector()` tmux capture + metadata/log persistence | `demo_harness_test.go` | package-level tests | ✅ |
| Inline `X <feedback>` persisted to artifact bundle (`metadata.json`, `demo_feedback.txt`) | PD-17-AF-007 | `defaultDemoDiagnosticsCollector()` feedback persistence | `demo_harness_test.go` | package-level tests | ✅ |
| End-of-run readiness and artifact summary output | PD-17-AF-008 | `renderDemoSummary()` readiness and artifact-path line | `demo_harness_test.go` | package-level tests | ✅ |
| Story-browser displays inline per-test status markers (`[ ]`, `[/]`, `[P]`, `[X]`) | PD-17-AF-009 | `renderDemoStoriesBoard()` status board renderer | `demo_harness_test.go` | package-level tests | ✅ |
| Split controller pane refreshes/clears between decisions to preserve readability | PD-17-AF-010 | `clearControllerPane()` and split-view `runDemoModeWithOptions()` flow | `demo_harness_test.go` | package-level tests | ✅ |
| Startup greeting parity criteria and demo story are defined and validated | PD-17-AF-011 | `defaultDemoSequence()` (`e2e-greet-001`) + `validateDemoStartupGreeting()` + `07_DEMO_MODE.md` Flow A contract | `demo_harness_test.go`, `tests/test_demo_ux_use_cases_headless.sh`, `tests/test_demo_ux_use_cases_layout_headless.sh` | package-level tests + headless e2e | ✅ |
| Prompt lifecycle parity criteria and demo story are defined and validated | PD-17-AF-012 | `defaultDemoSequence()` (`e2e-cycle-001`) + `validateDemoPromptLifecycle()` + `07_DEMO_MODE.md` Flow B contract | `demo_harness_test.go`, `tests/test_demo_ux_use_cases_headless.sh`, `tests/test_demo_ux_use_cases_layout_headless.sh` | package-level tests + headless e2e | ✅ |
| System panel parity criteria and demo story are defined and validated | PD-17-AF-013 | `defaultDemoSequence()` (`e2e-system-001`) + `validateDemoSystemPane()` + `07_DEMO_MODE.md` Flow C contract | `demo_harness_test.go`, `tests/test_demo_ux_use_cases_headless.sh`, `tests/test_demo_ux_use_cases_layout_headless.sh` | package-level tests + headless e2e | ✅ |
| System panel tab-tour parity criteria and demo story are defined and validated | PD-17-AF-014 | `defaultDemoSequence()` (`e2e-system-tour-001`) + `validateDemoSystemTour()` + `07_DEMO_MODE.md` Flow C.1 contract | `demo_harness_test.go`, `tests/test_demo_system_panel_tour_headless.sh` | package-level tests + headless e2e | ✅ |

### PD-18 — SystemAppletSuite

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| System frame binds by semantic title, not pane index | PD-18-AF-001 | `SystemAppletHost` / `newSystemAppletHost()` + `Resolve(tab)` | `system_applet_host_test.go`, `core_system_renderer_test.go` | package-level tests | ✅ |
| Context history applet renders recent turn history | PD-18-AF-002 | `contextHistorySystemApplet.RenderWidget()` via `SystemAppletHost.Resolve("context-history")` | `system_applet_host_test.go`, `context_widget_test.go` | package-level tests | ✅ |
| Configuration applet renders runtime config | PD-18-AF-003 | `renderContextWidget()` `configuration` case with `model:`, `backend:`, `ollama_host:` | `context_widget_test.go` (`TestRenderContextWidget_ConfigurationTabContract`) | package-level tests | ✅ |
| File-selection applet renders project file navigation | PD-18-AF-004 | `renderContextWidget()` `files` case with `project_dir:` + `entry_count:` from env/fs | `context_widget_test.go` (`TestRenderContextWidget_FilesTabContract`) | package-level tests | ✅ |
| Working-memory applet renders session facts | PD-18-AF-005 | `workingMemorySystemApplet.RenderCore()` via `SystemAppletHost.Resolve("working-memory")` | `system_applet_host_test.go`, `core_system_renderer_test.go` | package-level tests | ✅ |
| Context visualizer applet renders capacity and prompt-cycle status | PD-18-AF-006 | `renderContextWidget()` default case + `e2e-system-tour-001` live demo validator | `context_widget_test.go`, `tests/test_demo_system_panel_tour_headless.sh` | package-level tests + headless e2e | ✅ |
| Visible startup mode exposes one window per applet for UAT | PD-18-AF-007 | `main.go` `--startup-mode` flag + `config.go` `normalizeStartupMode()` | `config_startup_mode_test.go` | package-level tests | ✅ |

### Runtime Applet Sign-off Status (2026-06-01 baseline; later reopen applied)

This table records a 2026-06-01 sign-off baseline and the current reopened
state where applicable. It is not a blanket "all applets closed" statement.
Each runtime applet/surface row reflects current status as shown in `Sign-off`.
Evidence expectations remain:

- a dedicated PD affordance specification row,
- a matching traceability row in this matrix, and
- passing unit plus integration/functional evidence.

| Runtime applet/surface | UX spec anchor | Traceability anchor | Evidence | Sign-off |
|---|---|---|---|---|
| Chat/Output widget | `PD-17-AF-011`, `PD-17-AF-012` | `PD-17` | `demo_harness_test.go`, `tests/test_demo_ux_use_cases_headless.sh` | ✅ Closed |
| Input widget | `PD-16-AF-010`, `PD-17-AF-005` | `PD-16`, `PD-17` | `input_widget_test.go`, `demo_harness_test.go`, `tests/test_demo_ux_use_cases_headless.sh` | ✅ Closed |
| Logs widget | `PD-17` (`e2e-logs-001`) | `PD-17` | `demo_harness_test.go`, `tests/test_demo_ux_use_cases_headless.sh` | ✅ Closed |
| System applet suite (files/config/context-history/working-memory/context-visualizer) | `PD-18-AF-001..007` | `PD-18` | `system_applet_host_test.go`, `context_widget_test.go`, `core_system_renderer_test.go`, `tests/test_demo_system_panel_tour_headless.sh` | ⚠ Re-opened |
| UAT-visible startup topology | `PD-18-AF-007` | `PD-18` | `config_startup_mode_test.go`, `core_tmux_startup_integration_test.go` | ✅ Closed |
| Log/trace surface — session-event viewer (distinct from the retired "Logs widget" row above, which was DemoMode's own diagnostics-capture artifact viewer) | `PD-LOGS-AF-002..008` (AF-001, full-tab placement, is a layout fact with no unit-level test, same as `context-visualizer`'s) | `PD-LOGS` | `tests/steps/surfaces/logs_steps.go`, `tests/features/surfaces/logs_surface.feature` (`docs/build-plan/06_system_surfaces_backlog.md` Phase G, SS-8/SS-9) | ✅ Tested |
| Configuration surface — inspect and edit `agentx.toml` with live push to the orchestrator | `PD-CONFIG-AF-001..012` | `PD-CONFIG` | `tests/steps/transport/http_steps.go`, `tests/features/transport/config_surface_api.feature` | ✅ Phase 1a (read-only transport) |

**Pending:** PD-CTXHIST (context-history surface) — registered, not yet implemented, needs a fresh spec.

Active execution packet for first implementation slice: `docs/architecture/system_applet_suite_slice1.md` and `docs/ux/06_TUI_MIRROR.md` §12 (PD-16 default-behavior migration,
TUI-first default with `--gui` opt-in) were referenced here historically but neither
file is present in this repo.

---

## 5. Change Workflow Checklists

Use these checklists when modifying the UI.  Each item maps to the 4-phase lifecycle.

### 5.1 Adding a New Affordance

```
[ ] 1. Write the spec in 03_PANEL_DETAILS.md under the correct PD section.
[ ] 2. Assign an Affordance ID (PD-XX-AF-NNN).
[ ] 3. Add a row to the Traceability Matrix (§4) with status 📝 Spec Only.
[ ] 4. Implement the affordance; reference the ID in the method/class docstring.
[ ] 5. Write ≥ 1 unit test per state change; reference the ID in the test docstring.
[ ] 6. Run all tests; ensure ≥ 98% coverage is maintained.
[ ] 7. Update the Traceability Matrix row to ✅ Tested.
[ ] 8. Update CHANGELOG.md and bump the patch version.
[ ] 9. Commit everything together (spec + code + tests + matrix).
```

### 5.2 Modifying an Existing Affordance

```
[ ] 1. Find the Affordance ID in the Traceability Matrix (§4).
[ ] 2. Update the spec in 03_PANEL_DETAILS.md to describe the new behaviour.
[ ] 3. Update the code; keep the ID in the docstring (add a note if behaviour changed).
[ ] 4. Update or extend the existing tests to cover the new behaviour.
[ ] 5. Run all tests; fix any regressions in other tests caused by the change.
[ ] 6. Update the Traceability Matrix row if test file/class changed.
[ ] 7. Update CHANGELOG.md and bump the patch version.
[ ] 8. Commit everything together.
```

### 5.3 Removing an Affordance

```
[ ] 1. Find the Affordance ID in the Traceability Matrix (§4).
[ ] 2. Remove or strike-through the spec text in 03_PANEL_DETAILS.md.
[ ] 3. Delete the code (widget, callback, binding).
[ ] 4. Delete the tests that exclusively cover this affordance.
[ ] 5. Remove the row from the Traceability Matrix.
[ ] 6. Search for the Affordance ID across the whole repo (grep) to catch
       any remaining references.
[ ] 7. Update CHANGELOG.md and bump the patch version.
[ ] 8. Commit everything together.
```

### 5.4 Detecting Drift Without a Planned Change

Run this audit when the UI "looks wrong" but no deliberate change was made:

```bash
# 1. Find all Affordance IDs referenced in tests
grep -rn "PD-[0-9]\+-AF-[0-9]\+" tests/ internal/ | grep -oE 'PD-[0-9]+-AF-[0-9]+'

# 2. Find all Affordance IDs referenced in source code
grep -rn "PD-[0-9]\+-AF-[0-9]\+" cmd/ internal/ | grep -oE 'PD-[0-9]+-AF-[0-9]+'

# 3. Find all Affordance IDs declared in the spec
grep -rn "PD-[0-9]\+-AF-[0-9]\+" docs/ | grep -oE 'PD-[0-9]+-AF-[0-9]+'

# 4. Any ID that appears in only ONE of the three sets is a drift candidate.
```

---

## 6. Testing Strategy

Surface features must be validated through:

1. **Unit tests**: Test individual affordance behavior in isolation
2. **Integration tests**: Test affordance interaction with other features
3. **Acceptance tests**: Validate end-to-end user workflows

Test coverage requirements:

- Every user-visible state change must have at least one test
- Tests must be deterministic and independent
- Tests should avoid external dependencies where possible
- Mock external services (AI models, APIs, etc.)

---

## 7. Current Coverage Gaps

Specific surface features and workflows that need additional testing or implementation refinement.

---

## 8. Keeping This Document Current

### Developer responsibility

When making any change to surface features or behavior:

1. Search the Traceability Matrix for the affected affordance
2. Update the `Status` column if tests were added or modified
3. Add new rows for any new affordances introduced
4. Remove rows for any deleted affordances
5. Commit `docs/ux/UX_LIFECYCLE.md` as part of the same commit
6. If implementation diverges from spec, update the spec to match reality

When the AI agent makes any change to the system applet suite or the UAT-visible
startup mode:

1. Update the matching PD-18 row in [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md).
2. Add or update the traceability row in section §4.
3. Add at least one unit test and one integration/functional test for the
  affected applet or startup topology.
4. Reconcile the row status from `📝 Spec Only` to `✅ Tested` only after the
  implementation and tests pass.

### Developer responsibility

When you review a PR that touches UX surface code:

1. Check that `docs/ux/UX_LIFECYCLE.md` was updated.
2. Verify new affordances have both spec text and test coverage.
3. Verify removed affordances have no dangling test references.

### Audit command

```bash
# Quick consistency check — find all AF IDs in tests but not in UX_LIFECYCLE.md
grep -rho 'PD-[0-9]\+-AF-[0-9]\+' tests/ internal/ cmd/ | sort -u > /tmp/tested.txt
grep -oh 'PD-[0-9]\+-AF-[0-9]\+' docs/ux/UX_LIFECYCLE.md | sort -u > /tmp/specced.txt
comm -23 /tmp/tested.txt /tmp/specced.txt   # in tests but missing from matrix
comm -13 /tmp/tested.txt /tmp/specced.txt   # in matrix but no test yet (📝 / ❌)
```
