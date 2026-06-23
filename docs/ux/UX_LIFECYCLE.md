# AgentX — UX Lifecycle Reference

<!-- markdownlint-disable MD036 MD040 MD047 MD051 MD060 -->

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

The hybrid runtime's system applets are treated as first-class UX surfaces even
when they are implemented as runtime applets rather than GUI widgets.

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
  PD-01-AF-001   ChatPanel — turn order invariant (user entry above children)
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
| Node status icon reflects task state | PD-05-AF-006 | `PlanTreeWidget.update_node_status()` / `_STATUS_ICONS` | `test_plan_tree_affordances.py` | `TestNodeStatusIconReflectsState` | ✅ |

### PD-06 — ResynthesisDialog

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Dialog title includes task_id | PD-06-AF-001 | `ResynthesisDialog.__init__()` | `test_resynthesis_dialog.py` | Module-level pytest tests | ✅ |
| Cancel closes dialog without calling on_confirm | PD-06-AF-002 | `ResynthesisDialog._win.destroy` | `test_resynthesis_dialog.py` | Module-level pytest tests | ✅ |
| Re-synthesise calls on_confirm with hint text | PD-06-AF-003 | `ResynthesisDialog._on_confirm_clicked()` | `test_resynthesis_dialog.py` | Module-level pytest tests | ✅ |
| WM hint section hidden/visible based on callback | PD-06-AF-004 | `ResynthesisDialog.__init__()` | `test_resynthesis_dialog.py` | Module-level pytest tests | ✅ |
| Add WM hint calls callback and clears fields | PD-06-AF-005 | `ResynthesisDialog._on_add_wm_hint_clicked()` | `test_resynthesis_dialog.py` | Module-level pytest tests | ✅ |

### PD-07 — SettingsTab

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Theme toggle persists to agentx.toml | PD-07-AF-001 | `SettingsTab._on_theme_change()` | `test_gui_manager_integration.py` | `TestGUIManagerSettingsTheme` | ⚠️ |
| Settings sections collapsed/expanded correctly | PD-07-AF-002 | `SettingsTab.__init__()` / `SettingsTab._make_section()` | `test_settings_tab_sections.py` | `TestSettingsTabSectionCollapseDefaults` | ✅ |
| Restart-required fields show 🔁 icon in label | PD-07-AF-003 | `SettingsTab.RESTART_ICON` / `_add_checkbox()` / `_add_text_entry()` / `_add_spinbox()` | `test_settings_tab_sections.py` | `TestRestartIconInLabels` | ✅ |

### PD-08 — ContextRenderer (factory methods)

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Expand button always created for every message | PD-08-AF-001 | `_render_message_to_grid()` | `test_phase6_context_panel.py` | `TestRenderMessageAlwaysExpandable` | ✅ |
| Tool rows appended to collapsible list | PD-08-AF-002 | `_render_tool_rows()` | `test_phase6_context_panel.py` | `TestRenderMessageToGridPlanSplit` | ✅ |
| Plan/task_node rows appended to collapsible list | PD-08-AF-003 | `_render_plan_rows()` | `test_phase6_context_panel.py` | `TestRenderPlanRows` | ✅ |
| Empty-content message has button but no detail row | PD-08-AF-004 | `_render_message_to_grid()` | `test_phase6_context_panel.py` | `TestRenderMessageAlwaysExpandable` | ✅ |

### PD-09 — CollapsibleSection

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Section starts collapsed when default_open=False | PD-09-AF-001 | `CollapsibleSection.__init__()` | `test_collapsible_section.py` | Module-level pytest tests | ✅ |
| Section starts expanded when default_open=True | PD-09-AF-002 | `CollapsibleSection.__init__()` | `test_collapsible_section.py` | Module-level pytest tests | ✅ |
| Header click toggles content visibility | PD-09-AF-003 | `CollapsibleSection.toggle()` | `test_collapsible_section.py` | Module-level pytest tests | ✅ |
| set_content replaces previous content | PD-09-AF-004 | `CollapsibleSection.set_content()` | `test_collapsible_section.py` | Module-level pytest tests | ✅ |

### PD-10 — ContextMeterWidget

> ⚠️ **Relocation pending (PD-12 implementation)**: `ContextMeterWidget.create()` will be called from `StatusTab` instead of `InputPanel`. All PD-10 affordances are unchanged; only the host frame changes. See `PD-12-AF-011`.

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Meter creates canvas on first create() call | PD-10-AF-001 | `ContextMeterWidget.create()` | `test_context_meter_widget.py` | — | ✅ |
| Arc slices sized proportionally to token counts | PD-10-AF-002 | `ContextMeterWidget._draw_arcs()` | `test_context_meter_widget.py` | — | ✅ |
| Ghost arc shows remaining capacity | PD-10-AF-003 | `ContextMeterWidget._draw_arcs()` | `test_context_meter_widget.py` | — | ✅ |
| Border turns warning-red at 80% | PD-10-AF-004 | `ContextMeterWidget._risk_state()` | `test_context_meter_widget.py` | — | ✅ |
| Border turns critical-red at 100% | PD-10-AF-005 | `ContextMeterWidget._risk_state()` | `test_context_meter_widget.py` | — | ✅ |
| update() is thread-safe via after() | PD-10-AF-006 | `ContextMeterWidget.update()` | `test_context_meter_widget.py` | — | ✅ |
| max_tokens=0 does not crash | PD-10-AF-007 | `ContextMeterWidget._draw_arcs()` | `test_context_meter_widget.py` | — | ✅ |

### PD-11 — FileExplorer

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| List directory populates widget | PD-11-AF-001 | `FileExplorer.list_directory()` | `test_file_explorer_coverage.py` | `TestListDirectory` | ✅ |
| Change directory navigates and lists | PD-11-AF-002 | `FileExplorer.change_directory()` | `test_file_explorer_coverage.py` | `TestChangeDirectory` | ✅ |
| Back/Forward navigate history | PD-11-AF-003 | `FileExplorer.navigate_back/forward()` | `test_file_explorer_coverage.py` | `TestNavigateBackForward` | ✅ |
| Home button navigates to home dir | PD-11-AF-004 | `FileExplorer.navigate_home()` | `test_file_explorer_coverage.py` | `TestNavigateHome` | ✅ |
| Parent button navigates up one level | PD-11-AF-005 | `FileExplorer.navigate_parent()` | `test_file_explorer_coverage.py` | `TestNavigateParent` | ✅ |
| Open file triggers callback | PD-11-AF-006 | `FileExplorer.open_file()` | `test_file_explorer_coverage.py` | `TestOpenFile` | ✅ |
| Theme applies correct colours | PD-11-AF-007 | `FileExplorer._apply_theme()` | `test_file_explorer_theme.py` | — | ✅ |
| Right-click on file shows file context menu | PD-11-AF-008 | `FileExplorer._on_right_click()` | `test_file_explorer_context_menu.py` | `TestFileContextMenu` | ✅ |
| Right-click on directory shows folder context menu | PD-11-AF-009 | `FileExplorer._on_right_click()` | `test_file_explorer_context_menu.py` | `TestFolderContextMenu` | ✅ |
| Escape dismisses context menu | PD-11-AF-010 | `FileExplorer._dismiss_popup_menu()` | `test_file_explorer_context_menu.py` | `TestDismissContextMenu` | ✅ |
| Overflow navigation keeps selected row visible in terminal viewport | PD-11-AF-011 | `filesystemWidgetState.ensureSelectionVisible()` + overflow render contract in `filesystemWidgetState.render()` | `filesystem_widget_test.go` | `TestFilesystemWidgetRender_OverflowOrientationAndSelectionVisibility` | ✅ |
| Overflow navigation supports PageUp/PageDown/Home/End (or equivalent) | PD-11-AF-012 | `filesystemWidgetState.handleCommand()` (`pgup`/`pgdn`/`top`/`end`) | `filesystem_widget_test.go` | `TestFilesystemWidgetHandleCommand_PageNavigation` | ✅ |
| Overflow status exposes visible range orientation (`showing X-Y of Z`) | PD-11-AF-013 | `filesystemWidgetState.render()` visible range header | `filesystem_widget_test.go` | `TestFilesystemWidgetRender_OverflowOrientationAndSelectionVisibility` | ✅ |
| Files parity sign-off requires overflow-list executable evidence | PD-11-AF-014 | `hybrid-parity-gate` blocker gate execution path + filesystem widget contracts | `filesystem_widget_test.go`, `Makefile`, `tests/test_demo_system_panel_tour_headless.sh` | package-level tests + blocker gate evidence | ✅ |
| TUI files applet supports arrow-key row navigation | PD-11-AF-015 | `normalizeFilesystemWidgetCommand()` maps `Up`/`Down` to row navigation | `filesystem_widget_test.go` | `TestNormalizeFilesystemWidgetCommand` | ✅ |
| TUI files applet supports `Space` soft-select semantics with visible state | PD-11-AF-016 | `filesystemWidgetState.toggleSoftSelection()` + soft-select rendering + multi-action compatibility | `filesystem_widget_test.go` | `TestFilesystemWidgetHandleCommand_SoftSelectToggleVisibleInRender`, `TestFilesystemWidgetHandleCommand_AttachUsesSoftSelectedSetStatus`, `TestFilesystemWidgetHandleCommand_EditUsesSoftSelectedSetInViewOrder` | ✅ |
| TUI files applet supports `Return` hard-select primary action semantics | PD-11-AF-017 | `filesystemWidgetState.activateSelection()` via `enter` command path | `filesystem_widget_test.go` | `TestFilesystemWidgetHandleCommand_ReturnHardSelectActivates` | ✅ |
| No-obvious-path affordances require explicit user case-by-case decision before closure | PD-11-AF-018 | Path-A decision log + applet-governance escalation contract | `docs/architecture/applets/README.md`, `00_START_HERE.md` | decision-log governance evidence | ✅ |

### PD-12 — StatusTab

| Affordance | ID | Source | Test File | Test Class | Status |
|---|---|---|---|---|---|
| Status tab is first in system notebook | PD-12-AF-001 | `SidePanel.create()` | `test_status_tab.py` | `TestStatusTabCreate` | ✅ |
| Auto-switch to Status tab on prompt submit | PD-12-AF-002 | `StreamingController._on_stream_start()` | `test_status_tab.py` | `TestStatusTabAutoSwitch` | ✅ |
| Interrupt button enables/disables with streaming | PD-12-AF-003 | `StatusTab.set_streaming_state()` | `test_status_tab.py` | `TestStatusTabInterruptButton` | ✅ |
| Interrupt button invokes callback | PD-12-AF-004 | `StatusTab` `_interrupt_btn` command | `test_status_tab.py` | `TestStatusTabInterruptButton` | ✅ |
| Phase rows reset at stream start | PD-12-AF-005 | `StatusTab.reset()` | `test_status_tab.py` | `TestStatusTabPhaseReset` | ✅ |
| Phase row transitions to RUNNING / starts timer | PD-12-AF-006 | `StatusTab.set_phase()` | `test_status_tab.py` | `TestStatusTabSetPhase` | ✅ |
| Phase row transitions to DONE / freezes timer | PD-12-AF-007 | `StatusTab.set_phase()` | `test_status_tab.py` | `TestStatusTabSetPhase` | ✅ |
| Phase row transitions to FAILED | PD-12-AF-008 | `StatusTab.set_phase()` | `test_status_tab.py` | `TestStatusTabSetPhase` | ✅ |
| Tool step label updates with active tool name | PD-12-AF-009 | `StatusTab.set_phase()` | `test_status_tab.py` | `TestStatusTabSetPhase` | ✅ |
| Colour-key legend rows match donut bands | PD-12-AF-010 | `ContextKeyWidget` | `test_status_tab.py` | `TestContextKeyWidget` | ✅ |
| ContextMeterWidget hosted in StatusTab | PD-12-AF-011 | `StatusTab.create()` | `test_status_tab.py` | `TestStatusTabCreate` | ✅ |

### PD-13 — ToolPanel

> ⚠️ **Note**: ToolPanel was previously numbered PD-10 in `03_PANEL_DETAILS.md` — renumbered to PD-13 to resolve conflict with ContextMeterWidget (PD-10 in `UX_LIFECYCLE.md`). ToolPanel has no affordance IDs yet.

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| Checkbox per tool toggles enabled state | PD-13-AF-001 | `ToolPanel.on_tool_toggle()` | — | — | 📝 |
| Expand/collapse panel header | PD-13-AF-002 | `ToolPanel` header toggle | — | — | 📝 |

### PD-14 — VimBridge GUI

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| "Edit" context menu opens file in running neovim as new buffer; no buffers closed | PD-14-AF-002 | `VimBridge.open_file()` + `AgentXSession._open_file_in_editor()` + `FileExplorer._on_edit_selected()` | `test_vim_bridge_gui.py` | `TestVimBridgeOpenFile`, `TestSessionOpenFileInEditor` | ✅ |
| is_connected() returns True when socket file is a Unix socket | PD-14-AF-002a | `VimBridge.is_connected()` | `test_vim_bridge_gui.py` | `TestVimBridgeIsConnected` | ✅ |
| Path resolution forwards absolute/relative paths correctly | PD-14-AF-002b | `VimBridge.open_file_from_context()` | `test_vim_bridge_gui.py` | `TestVimBridgeOpenFileFromContext` | ✅ |
| Editor status bar shows connected state | PD-14-AF-001 | — | — | — | 📝 |
| Send to Editor button enabled when connected | PD-14-AF-003 | — | — | — | 📝 |
| Send to Editor button disabled when disconnected | PD-14-AF-003 | — | — | — | 📝 |
| Line navigation from error display opens file at line N | PD-14-AF-004 | — | — | — | 📝 |
| File-saved notification shown in ChatPanel | PD-14-AF-005 | — | — | — | 📝 |
| Recover-editor restores editing surface | PD-14-AF-008 | `launch_vibe.sh` recover-editor branch | `test_launch_vibe_shutdown.py` | module-level tests | ✅ |

### PD-15 — TerminalPane GUI

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| Active terminal pane indicator updates in input status strip | PD-15-AF-003 | `InputPanel.set_terminal_status()` + `StreamingController._handle_terminal_tool_result()` | `test_terminal_pane_gui.py`, `test_terminal_streaming_controller.py` | `TestTerminalPaneGuiAffordances` | ✅ |
| Tool-result row exposes kill-pane action and callback wiring | PD-15-AF-004 | `ChatPanel.set_tool_result_kill_action()` + `AgentXSession._handle_terminal_kill_pane()` | `test_terminal_pane_gui.py`, `test_terminal_streaming_controller.py` | `TestTerminalPaneGuiAffordances` | ✅ |
| Input-strip mode toggle switches supervised/autonomous with confirmation gate | PD-15-AF-005 | `InputPanel._on_terminal_mode_toggle()` + `AgentXSession._handle_terminal_mode_toggle()` | `test_terminal_pane_gui.py`, `test_terminal_mode_and_approval.py` | `TestTerminalPaneGuiAffordances` | ✅ |
| Supervised confirm-list commands route through interactive approval dialog | PD-15-AF-006 | `AgentXSession._request_terminal_approval()` + `_show_terminal_approval_dialog()` | `test_terminal_mode_and_approval.py` | module-level tests | ✅ |
| Settings editor updates allow/confirm/deny permission prefixes with save/reset controls | PD-15-AF-007 | `SettingsTab._save_terminal_permission_lists()` + `_reset_terminal_permission_lists()` | `test_terminal_settings_editor.py` | module-level tests | ✅ |
| `launch_vibe.sh stop` sends Ctrl+C to AgentX and nvim panes then kills the tmux session | PD-15-AF-008 | `launch_vibe.sh` stop branch | `test_launch_vibe_shutdown.py` | module-level tests | ✅ |
| `terminal_run()` wrapper resolves `visible`/`auto_close`/`timeout_sec` from `agentx.toml [terminal]` when caller omits them | PD-15-AF-009 | `terminal_bridge.terminal_run()` defaults resolution block | `test_terminal_bridge.py` | `test_terminal_run_wrapper_uses_config_defaults` | ✅ |
| Streamed tool-result rows for `terminal_run` include a decision badge (✅/⛔/🚫) and exit code | PD-15-AF-010 | `StreamingController._display_tool_result()` badge injection | `test_terminal_streaming_controller.py` | `test_terminal_run_result_includes_decision_badge` | ✅ |

### PD-16 — TuiMirror

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| Output FIFO writer emits USER/AGENT/TOOL/DONE records without blocking GUI path | PD-16-AF-001 | `TuiBridge.write_output()` + `StreamingController._display_*()` hooks | `test_tui_bridge_output.py` | module-level tests | ✅ |
| Input FIFO reader parses submit sentinel and dispatches prompt callbacks | PD-16-AF-002 | `TuiBridge._input_reader_loop()` | `test_tui_bridge_output.py` | module-level tests | ✅ |
| Launcher creates optional `tui-chat` tmux window with TUI env wiring | PD-16-AF-003 | `launch_vibe.sh` start/restart branches | `test_launch_vibe_shutdown.py` | module-level tests | ✅ |
| Generated `agentx_tui.lua` config is written and sourced by TUI nvim startup | PD-16-AF-004 | `launch_vibe.sh` `_write_tui_lua()` + TUI launch command | `test_launch_vibe_shutdown.py` | module-level tests | ✅ |
| `<leader>s` submit keymap writes input text to FIFO with submit sentinel | PD-16-AF-005 | generated `agentx_tui.lua` submit keymap block | `test_launch_vibe_shutdown.py` | module-level tests | ✅ |
| `enable_gui_chat=false` mode uses headless `NullGUIManager` and enforces config constraint | PD-16-AF-006 | `config.validate_config()` + `AgentXSession` GUI-disabled path | `test_config_tui_phase1.py`, `test_session_gui_disabled.py` | module-level tests | ✅ |
| `tui.enable` controls `TuiBridge` lifecycle and guarded call-sites | PD-16-AF-007 | `AgentXSession.__init__()` + `close()` + streaming guards | `test_tui_bridge_output.py`, `test_launch_vibe_shutdown.py` | module-level tests | ✅ |
| `<leader>q` writes quit sentinel and triggers graceful application shutdown from TUI | PD-16-AF-008 | generated `agentx_tui.lua` quit keymap + `TuiBridge._input_reader_loop()` + `AgentXSession._on_tui_quit()` | `test_tui_bridge_output.py`, `test_session_gui_disabled.py`, `test_launch_vibe_shutdown.py` | module-level tests | ✅ |
| TUI context visualization renders color-band meter and top-contributor bars with ASCII fallback | PD-16-AF-009 | `TuiBridge.render_context_visualization()` + `AgentXSession.schedule_meter_redraw()` | `test_tui_bridge_output.py`, `test_active_model_meter_wiring.py`, `test_tmux_ux_flow_what_is_2_plus_2_headless.sh` | module-level tests + headless e2e | ✅ |
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

Active execution packet for first implementation slice:

- `docs/architecture/system_applet_suite_slice1.md`

Follow-up for PD-16 default-behavior migration is documented in
`docs/ux/06_TUI_MIRROR.md` §12 (TUI-first default with `--gui` opt-in).

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
grep -rn "PD-[0-9]\+-AF-[0-9]\+" tests/ | grep -oE 'PD-[0-9]+-AF-[0-9]+'

# 2. Find all Affordance IDs referenced in source code
grep -rn "PD-[0-9]\+-AF-[0-9]\+" src/  | grep -oE 'PD-[0-9]+-AF-[0-9]+'

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

When you review a PR that touches `src/agentx/gui/`:

1. Check that `docs/ux/UX_LIFECYCLE.md` was updated.
2. Verify new affordances have both spec text and test coverage.
3. Verify removed affordances have no dangling test references.

### Audit command

```bash
# Quick consistency check — find all AF IDs in tests but not in UX_LIFECYCLE.md
grep -rho 'PD-[0-9]\+-AF-[0-9]\+' tests/ | sort -u > /tmp/tested.txt
grep -oh 'PD-[0-9]\+-AF-[0-9]\+' docs/ux/UX_LIFECYCLE.md | sort -u > /tmp/specced.txt
comm -23 /tmp/tested.txt /tmp/specced.txt   # in tests but missing from matrix
comm -13 /tmp/tested.txt /tmp/specced.txt   # in matrix but no test yet (📝 / ❌)
```
