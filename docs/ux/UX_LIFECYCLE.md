# AgentX — UX Lifecycle Reference

_Last updated: 2026-05-22 (v0.77.0)_
**Purpose**: Single source of truth for the complete lifecycle of every user-facing
UI feature — from first written description through code implementation, hermetic
testing, and as-built reconciliation.  Both the developer and the AI agent refer to
this document when specifying, building, changing, or removing UI affordances.

---

## Table of Contents

1. [Why UI Features Drift](#1-why-ui-features-drift)
2. [The 4-Phase Lifecycle](#2-the-4-phase-lifecycle)
3. [Affordance ID Scheme](#3-affordance-id-scheme)
4. [Traceability Matrix (As-Built)](#4-traceability-matrix-as-built)
5. [Change Workflow Checklists](#5-change-workflow-checklists)
6. [Headless Tkinter Testing Primer](#6-headless-tkinter-testing-primer)
7. [Known Coverage Gaps](#7-known-coverage-gaps)
8. [Keeping This Document Current](#8-keeping-this-document-current)

---

## 1. Why UI Features Drift

A UI affordance (a button, a keyboard shortcut, a collapse animation, a colour change)
exists in three independent places at once:

| Layer | Where it lives | Who can change it alone |
|-------|---------------|------------------------|
| **Specification** | `docs/ux/*.md` | Documentation edit |
| **Code** | `src/agentx/gui/*.py` | Code refactor |
| **Test** | `tests/test_*.py` | Test refactor |

When a change is made in one layer without updating the others, the layers diverge:

- Code change without spec update → spec becomes stale ("as-designed" ≠ "as-built").
- Code change without test update → the test either breaks (visible) or silently stops
  testing the right thing (invisible — the worst kind of drift).
- Spec change without code or test → a description of desired future state that looks
  indistinguishable from a description of current state.

The solution is a **traceability chain**: every affordance carries an ID that appears
in the spec document, in the source code docstring, and in the test docstring.  When
any one layer changes you can immediately find and update the others by searching for
that ID.

---

## 2. The 4-Phase Lifecycle

```
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
- Write what the affordance **does** (not how it is implemented):
  - What widget appears, where, and with what label.
  - What happens when the user activates it.
  - What the disabled/default/edge-case states look like.
- Add an ASCII or Mermaid mockup.
- Add a row to the [Traceability Matrix](#4-traceability-matrix-as-built) with status `📝 Spec Only`.

**Output**: A committed update to `docs/ux/03_PANEL_DETAILS.md` and this file.

### Phase 2 — Code

Implement the widget.

- Reference the Affordance ID in the class or method docstring:

  ```python
  def _render_expand_button(self, parent: tk.Frame) -> tk.Button:
      """Create the message collapse/expand toggle.  [PD-08-AF-001]"""
  ```

- If the implementation diverges from the spec (for any reason), update the spec first.
  Never silently let code and spec disagree.
- Do not add placeholder behaviour you intend to implement later.  Mark those with the
  `TODO(PD-XX-AF-YYY):` prefix so they are findable.

**Output**: Committed source changes with spec IDs in docstrings.

### Phase 3 — Test

Write hermetic unit tests before or alongside the code (see §6 for the test pattern).

- Reference the Affordance ID in the test docstring:

  ```python
  def test_expand_button_in_col_0(self):
      """GIVEN any message row [PD-08-AF-001]
      WHEN rendered via _render_message_to_grid
      THEN a Button widget exists in column 0 of the grid.
      """
  ```

- Every user-visible **state change** triggered by an affordance needs at least one test.
  Minimum coverage:
  - Default/initial state is correct.
  - Primary user action produces the expected state change.
  - Edge cases (empty content, disabled state, overflow).
- Tests must be hermetic: no live Ollama, no file system I/O, no real network calls.
  Mock `ModelMetadataStore.populate` in any test that constructs `AgentXSession`
  (prevents background threads from crashing teardown — see §6).

**Output**: Committed tests that all pass.  Coverage must remain ≥ 98%.

### Phase 4 — Reconcile (As-Built Update)

Update this document to reflect what was built and tested.

- Change the traceability row status from `📝 Spec Only` to `✅ Tested`.
- If the code differs from the spec in any way, update the spec to match the code and
  note the change in `CHANGELOG.md`.
- Commit `docs/ux/UX_LIFECYCLE.md` alongside the code and test changes.

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

| PD | Panel | Source File |
|----|-------|-------------|
| PD-01 | ChatPanel | `src/agentx/gui/chat_panel.py` |
| PD-02 | InputPanel | `src/agentx/gui/input_panel.py` |
| PD-03 | SidePanel | `src/agentx/gui/side_panel.py` |
| PD-04 | ModelSelector | `src/agentx/gui/model_selector.py` |
| PD-05 | PlanTreeWidget | `src/agentx/gui/plan_tree_widget.py` |
| PD-06 | ResynthesisDialog | `src/agentx/gui/resynthesis_dialog.py` |
| PD-07 | SettingsTab | `src/agentx/gui/settings_tab.py` |
| PD-08 | ContextRenderer | `src/agentx/gui/context_renderer.py` |
| PD-09 | CollapsibleSection | `src/agentx/gui/collapsible_section.py` |
| PD-10 | ContextMeterWidget | `src/agentx/gui/context_meter_widget.py` |
| PD-11 | FileExplorer | `src/agentx/file_explorer.py` |
| PD-12 | StatusTab | `src/agentx/gui/status_tab.py` |
| PD-13 | ToolPanel | `src/agentx/gui/tool_panel.py` |
| PD-14 | VimBridge GUI | `src/agentx/integration/vim_bridge.py` |
| PD-15 | TerminalPane GUI | `src/agentx/integration/terminal_bridge.py` + `src/agentx/gui/` |
| PD-16 | TuiMirror | `src/agentx/integration/tui_bridge.py`, `launch_vibe.sh` |
| PD-17 | DemoMode | `cmd/agentx-core/main.go`, demo harness package (planned) |

When a new panel or top-level widget is added, assign the next available PD number,
add a row to this table, and create a section in `03_PANEL_DETAILS.md`.

---

## 4. Traceability Matrix (As-Built)

This table is the **as-built record**.  It maps each spec section to the code that
implements it and the test that validates it.  Status legend:

| Symbol | Meaning |
|--------|---------|
| ✅ | Spec, code, and tests all exist and agree |
| ⚠️ | Tests exist but cover only a subset of the spec affordances |
| 📝 | Spec exists; no tests yet written |
| ❌ | Known gap — either no spec, no code, or no tests |

### PD-01 — ChatPanel

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Turn order: user entry packed before children frame | PD-01-AF-001 | `ChatPanel._ensure_turn_started()` | `test_chat_panel_turn_rendering.py` | `TestConversationTurnRenderingOrder` | ✅ |
| Collapse user entry hides children frame | PD-01-AF-002 | `ChatPanel._toggle_turn_children()` | `test_chat_panel_turn_rendering.py` | `TestConversationTurnRenderingOrder` | ✅ |
| Expand re-packs children frame below user entry | PD-01-AF-003 | `ChatPanel._toggle_turn_children()` | `test_chat_panel_turn_rendering.py` | `TestConversationTurnRenderingOrder` | ✅ |
| Multiple turns maintain independent order | PD-01-AF-004 | `ChatPanel` | `test_chat_panel_turn_rendering.py` | `TestMultipleTurnsRenderingOrder` | ✅ |
| Thinking block collapsed by default | PD-01-AF-005 | `ChatPanel.display_agent_thinking()` | `test_chat_panel_collapse_defaults.py` | `test_thinking_entry_collapsed_by_default` | ✅ |
| Tool call collapsed by default | PD-01-AF-006 | `ChatPanel._display_tool_call()` | `test_chat_panel_collapse_defaults.py` | `test_tool_call_entry_collapsed_by_default` | ✅ |
| Assistant response expanded by default | PD-01-AF-007 | `ChatPanel.display_agent_response()` | `test_chat_panel_collapse_defaults.py` | `test_assistant_response_entry_expanded_by_default` | ✅ |
| Markdown rendered after DONE chunk | PD-01-AF-008 | `ChatPanel.finalize_current_turn_markdown()` | `test_markdown_rendering.py` | — | ⚠️ |
| Startup log-location notice shown before first agent response (config-gated) | PD-01-AF-009 | `AgentXSession._show_startup_log_locations_notice_if_enabled()` | `test_startup_log_notice.py` | `TestStartupLogNotice` | ✅ |
| Right-click on output panel opens copy context menu | PD-01-AF-010 | `ChatPanel._show_output_context_menu()` | `test_chat_panel_copy_context_menu.py` | `TestOutputPanelRightClickCopy` | ✅ |

### PD-02 — InputPanel

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Enter key submits message | PD-02-AF-001 | `InputPanel._bind_keys()` | `test_gui_manager_integration.py` | `TestGUIManagerInputMethods` | ⚠️ |
| Shift+Enter inserts newline | PD-02-AF-002 | `InputPanel._on_shift_return()` | `test_input_panel_keyboard.py` | `TestShiftEnterInsertsNewline` | ✅ |
| Send disabled during streaming | PD-02-AF-003 | `InputPanel.set_streaming_state()` | `test_gui_manager_integration.py` | `TestGUIManagerInputMethods` | ⚠️ |
| Stop enabled during streaming | PD-02-AF-004 | `InputPanel.set_streaming_state()` | `test_gui_manager_integration.py` | `TestGUIManagerInputMethods` | ⚠️ |
| Attachment chip rendered with filename and icon | PD-02-AF-005 | `InputPanel._create_attachment_widget()` | `test_input_panel_attachment_chips.py` | `TestAttachmentChipRender` | ✅ |
| Toggle chip calls on_attachment_toggle callback | PD-02-AF-006 | `InputPanel._create_attachment_widget()` | `test_input_panel_attachment_chips.py` | `TestAttachmentChipToggle` | ✅ |
| Rebuild with empty lists clears all chips | PD-02-AF-007 | `InputPanel.update_attachment_bar()` | `test_input_panel_attachment_chips.py` | `TestAttachmentBarClear` | ✅ |
| Right-click opens Wayland-safe context popup on input widget | PD-02-AF-008 | `InputPanel._show_input_context_menu()` | `test_input_panel_context_menu.py` | `TestInputPanelRightClickPopup` | ✅ |
| Input context menu shows "Copy" only when text is selected | PD-02-AF-009 | `InputPanel._show_input_context_menu()` | `test_input_panel_context_menu.py` | `TestInputCopyMenuVisibility` | ✅ |
| Input context menu shows "Paste" only when clipboard is non-empty | PD-02-AF-010 | `InputPanel._show_input_context_menu()` | `test_input_panel_context_menu.py` | `TestInputPasteMenuVisibility` | ✅ |
| "Copy" in input context menu copies selected text to clipboard | PD-02-AF-011 | `InputPanel._on_input_context_copy()` | `test_input_panel_context_menu.py` | `TestInputCopyAction` | ✅ |
| "Paste" in input context menu replaces selection or inserts at cursor | PD-02-AF-012 | `InputPanel._on_input_context_paste()` | `test_input_panel_context_menu.py` | `TestInputPasteAction` | ✅ |

### PD-03 — SidePanel / Session Tab — Context Section

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Every message row has expand/collapse button | PD-03-AF-001 | `ContextRenderer._render_message_to_grid()` | `test_phase6_context_panel.py` | `TestRenderMessageAlwaysExpandable` | ✅ |
| Full content hidden by default | PD-03-AF-002 | `ContextRenderer._render_message_to_grid()` | `test_phase6_context_panel.py` | `TestRenderMessageAlwaysExpandable` | ✅ |
| Full content visible after expand click | PD-03-AF-003 | `ContextRenderer.collapse_expand_button()` | `test_phase6_context_panel.py` | `TestRenderMessageAlwaysExpandable` | ✅ |
| Plan rows grouped under preceding assistant message | PD-03-AF-004 | `ContextRenderer.render_context_widget()` | `test_phase6_context_panel.py` | `TestRenderContextWidgetGrouping` | ✅ |
| Plan header row is clickable when on_plan_click provided | PD-03-AF-005 | `ContextRenderer._render_plan_rows()` | `test_phase6_context_panel.py` | `TestRenderPlanRows` | ✅ |
| Plan/task_node rows excluded from LLM messages | PD-03-AF-006 | `Context.to_llm_messages()` | `test_phase6_context_panel.py` | `TestPlanMessagesExcludedFromLLM` | ✅ |
| Message enabled checkbox wired to message.enabled | PD-03-AF-007 | `ContextRenderer._render_message_to_grid()` | `test_context_renderer_message_enabled.py` | `TestMessageEnabledCheckbox` | ✅ |

### PD-03 — SidePanel / Session Tab — Working Memory Section

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Fact row rendered per WM fact | PD-03-AF-010 | `ContextRenderer.render_working_memory_widget()` | `test_gui_manager_integration.py` | `TestGUIManagerPanelMethods` | ⚠️ |
| Toggle checkbox calls on_toggle callback | PD-03-AF-011 | `ContextRenderer._render_working_memory_row()` | `test_working_memory_widget_callbacks.py` | `TestWorkingMemoryToggle` | ✅ |
| Delete button calls on_delete callback | PD-03-AF-012 | `ContextRenderer._render_working_memory_row()` | `test_working_memory_widget_callbacks.py` | `TestWorkingMemoryDelete` | ✅ |
| Promote button calls on_promote callback | PD-03-AF-013 | `ContextRenderer._render_working_memory_row()` | `test_working_memory_widget_callbacks.py` | `TestWorkingMemoryPromote` | ✅ |
| Add-fact form submits user-provided key/value | PD-03-AF-014 | `ContextRenderer.render_working_memory_widget()` | `test_working_memory_widget_callbacks.py` | `TestWorkingMemoryAddFact` | ✅ |
| Working Memory section starts collapsed at startup | PD-03-AF-015 | `SidePanel.create()` | `test_gui_manager_integration.py` | `test_session_sections_start_collapsed` | ✅ |

### PD-04 — ModelSelector

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Selecting model updates active_model | PD-04-AF-001 | `ModelSelector._on_selection_change()` | `test_active_model.py` | `TestActiveModelProperty` | ✅ |
| Model change triggers context meter redraw | PD-04-AF-002 | `SessionState.active_model` setter | `test_active_model_meter_wiring.py` | — | ✅ |
| Bare model name resolves via :latest fallback | PD-04-AF-003 | `ModelMetadataStore.get_context_length()` | `test_model_metadata_store.py` | — | ✅ |
| Refresh button reloads model list | PD-04-AF-004 | `ModelSelector._on_refresh()` | `test_model_selector_refresh.py` | `TestModelSelectorRefreshButton` | ✅ |

### PD-05 — PlanTreeWidget

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|------------|----|---------------------|-----------|------------|--------|
| Plan header row rendered with plan name | PD-05-AF-001 | `ContextRenderer._render_plan_rows()` | `test_phase6_context_panel.py` | `TestRenderPlanRows` | ✅ |
| Task node rows indented under plan | PD-05-AF-002 | `ContextRenderer._render_plan_rows()` | `test_phase6_context_panel.py` | `TestRenderPlanRows` | ✅ |
| Step count badge shown in plan header | PD-05-AF-003 | `ContextRenderer._render_plan_rows()` | `test_phase6_context_panel.py` | `TestRenderPlanRows` | ✅ |
| Re-synth button opens ResynthesisDialog | PD-05-AF-004 | `PlanTreeWidget._create_synthesis_block()` | `test_plan_tree_affordances.py` | `TestResynthButtonInSynthesisBlock` | ✅ |
| Export button writes and opens export file | PD-05-AF-005 | `ChatPanel.add_plan_tab()` / `AgentXSession._export_task_tree()` | `test_plan_tree_affordances.py` | `TestExportButtonInPlanTab` | ✅ |
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
| TUI context visualization renders color-band meter and top-contributor bars with ASCII fallback | PD-16-AF-009 | `TuiBridge.render_context_visualization()` + `AgentXSession.schedule_meter_redraw()` | `test_tui_bridge_output.py`, `test_active_model_meter_wiring.py` | module-level tests | ✅ |

### PD-17 — DemoMode

| Affordance | ID | Source Class/Method | Test File | Test Class | Status |
|---|---|---|---|---|---|
| `--demo` enters DemoMode execution path | PD-17-AF-001 | `main.go` flag parse + `runDemoMode` entry path | `demo_harness_test.go` | package-level tests | ✅ |
| Demo sequence list shown before execution | PD-17-AF-002 | `renderDemoSequence()` ordered sequence renderer | `demo_harness_test.go` | package-level tests | ✅ |
| Start test selection by id/index (`--demo-start`) | PD-17-AF-003 | `resolveDemoStartIndex()` selector parser | `demo_harness_test.go` | package-level tests | ✅ |
| Per-test user feedback prompt (`N`/`X`) | PD-17-AF-004 | `readDemoDecision()` with invalid-input re-prompt in `runDemoMode()` | `demo_harness_test.go` | package-level tests | ✅ |
| `X` triggers pane/metadata dump artifact bundle | PD-17-AF-005 | `defaultDemoDiagnosticsCollector()` tmux capture + metadata/log persistence | `demo_harness_test.go` | package-level tests | ✅ |
| End-of-run readiness and artifact summary output | PD-17-AF-006 | `renderDemoSummary()` readiness and artifact-path line | `demo_harness_test.go` | package-level tests | ✅ |

Planned follow-up for PD-16 default-behavior migration is documented in
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

## 6. Headless Tkinter Testing Primer

Tkinter requires a display server, but it can be driven completely in memory under the
`Xvfb` virtual framebuffer or the standard X session of a desktop environment.  Tests
run in CI and locally without a visible window.

---

### Platform Design Principle: Wayland / XWayland Coordinate Safety

> **This rule applies to every affordance that calls `menu.post()`, `wm_geometry()`,
> or any other function that places a popup or floating window at an absolute screen
> position.**

#### Background

AgentX runs on Ubuntu with a GNOME/Mutter Wayland compositor (`XDG_SESSION_TYPE=wayland`).
Tkinter itself is an X11 application and is surfaced through **XWayland** — an X11
compatibility layer embedded inside the Wayland session.

XWayland exposes a single large _virtual framebuffer_ whose pixel dimensions are the
union of all physical monitors at their native (physical) resolutions, including any
HiDPI scaling.  X11 event coordinates (`event.x_root`, `event.y_root`) are reported in
this physical-pixel virtual space.  Under a multi-monitor or HiDPI setup the virtual
framebuffer can be thousands of pixels wider/taller than any single visible monitor,
and these coordinates may therefore describe a point that is off-screen from the user's
perspective.

**UAT evidence (v0.22.7):** a right-click event yielded `x_root=3753` on a system with
no single monitor wider than ~3840 physical pixels, placing the menu window completely
off-screen.

#### The Rule

**Never pass `event.x_root` / `event.y_root` directly to `menu.post()` or any
window-positioning call.**

Instead, derive the screen position from the widget's own geometry:

```python
# CORRECT — always visible under Wayland/XWayland
x = widget.winfo_rootx() + event.x
y = widget.winfo_rooty() + event.y
menu.post(x, y)
```

`widget.winfo_rootx()` / `winfo_rooty()` query Tk's own geometry system, which tracks
the widget's logical on-screen anchor.  `event.x` / `event.y` are cursor offsets
_relative to the widget_ — always non-negative and bounded by the widget's own
dimensions.  The sum is therefore always within the widget's footprint and, by
extension, always within a visible monitor.

```python
# WRONG — raw X11 physical-pixel coords, off-screen under XWayland
menu.post(event.x_root, event.y_root)
```

#### Why `after_idle` is insufficient and `after(100)` is required

The `<Button-3>` press event fires synchronously.  Creating the menu window inside
the press handler causes the _subsequent_ `<ButtonRelease-3>` to be delivered to the
newly-visible menu window, where the Tk `Menu` class's built-in `<ButtonRelease>`
binding (`tk::MenuInvoke`) immediately calls `unpost()` — dismissing the menu before
the user can interact.

The naive fix is `after_idle` — but **`after_idle` fires before the physical button
release**.  The idle callback runs while the user is still holding the button down,
so the menu posts, and the release still arrives at the menu window.

The correct fix is `after(100)` — a 100 ms timer.  100 ms is longer than any
physical click duration, so by the time the callback fires the button has been
released on the treeview (where there is no `<ButtonRelease-3>` binding) and
the popup shows cleanly and stays open:

```python
_MENU_POST_DELAY_MS: int = 100  # class attribute; set to 0 in tests

def _on_right_click(self, event) -> None:
    x = self.tree.winfo_rootx() + event.x   # safe coords (Wayland rule)
    y = self.tree.winfo_rooty() + event.y
  self.tree.after(self._MENU_POST_DELAY_MS, lambda: menu.tk_popup(x, y))
```

**In unit tests** set `fe._MENU_POST_DELAY_MS = 0` and call `root.update()`
(not `root.update_idletasks()`) to flush the timer callback without real latency:

```python
fe._MENU_POST_DELAY_MS = 0
fe._on_right_click(ev)
root.update()   # fires after(0) callbacks
mock_post.assert_called_once()
```

#### Wayland fallback when Tk menus are mapped but invisible

If diagnostics repeatedly show `ismapped=1, viewable=1` with valid in-bounds
coordinates but the user still sees no popup, treat this as a compositor-level
rendering/stacking issue for Tk menu windows on Wayland/XWayland.

In that case, use a **Wayland-specific in-app popup fallback**:

1. Render a borderless `tk.Toplevel(overrideredirect=True)` anchored at safe
  coordinates (`winfo_rootx/y + event.x/y`).
2. Populate it with explicit `tk.Button` actions that map 1:1 to the context
  menu commands.
3. Keep existing non-Wayland behavior unchanged.
4. In tests, gate behavior with a force flag (e.g. `_FORCE_WAYLAND_POPUP`) so
  both branches are hermetically tested.

#### Testing the coordinate strategy

Functional tests for popup coordinates must:

1. Use a real `tk.Tk()` root (geometry queries do not work on `MagicMock`).
2. Assert that `winfo_rootx() + event.x` fits within `winfo_screenwidth()`.
3. Optionally supply a synthetic `event.x_root` with an out-of-bounds value
   (e.g. `9999`) and assert it is **not** what reaches `menu.post()`.

See `tests/test_file_explorer_menu_coordinates.py::TestMenuCoordinateSafety` for the
reference implementation (affordance PD-11-AF-008).

---

### Why it works

Tkinter's geometry managers (`pack`, `grid`) and widget state (`cget`, `grid_info`) are
all accessible without a visible window.  `root.withdraw()` hides the window; the widget
tree is still fully functional.

### Standard test scaffold

```python
import tkinter as tk
import unittest
from unittest.mock import MagicMock

from agentx.gui.gui_config import GUIConfig
from agentx.gui.gui_manager import GUIManager


def _make_root() -> tk.Tk:
    """Create a hidden Tk root for unit tests."""
    root = tk.Tk()
    root.withdraw()   # hide — no window appears on screen
    return root


def _make_gui(root: tk.Tk) -> GUIManager:
    """Build a fully-initialized GUIManager for testing."""
    config = GUIConfig.from_dict({
        "ollama_host": "localhost",
        "ollama_model": "test-model",
        "ollama_timeout": 30,
    })
    return GUIManager(
        root=root,
        config=config,
        on_submit=MagicMock(),
        on_interrupt=MagicMock(),
        on_attachment_toggle=MagicMock(),
    )


class TestMyAffordance(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()
        self.gui = _make_gui(self.root)

    def tearDown(self):
        try:
            self.root.destroy()   # always clean up the widget tree
        except Exception:
            pass
```

### Querying widget state

```python
# Is a widget visible? (grid geometry manager)
widget.grid_info()          # returns {} if grid_remove()'d, dict if visible

# Is a widget visible? (pack geometry manager)
widget.pack_info()          # same pattern

# What column is a widget in?
int(widget.grid_info().get("column", -1))

# What text does a widget display?
widget.cget("text")

# Trigger a button as if clicked:
button.invoke()

# List all children of a frame:
frame.winfo_children()
```

### Critical: mock ModelMetadataStore.populate

Any test that constructs an `AgentXSession` must mock `populate` to prevent background
HTTP threads from crashing teardown:

```python
from unittest.mock import patch

with patch("agentx.session.create_adapter", return_value=mock_adapter), \
     patch("agentx.model_metadata_store.ModelMetadataStore.populate"):
    session = AgentXSession(username="tester", session_dir=test_dir, config=config)
```

Without this mock the background thread makes a real HTTP call to Ollama.  When the
test's Tk root is destroyed before the thread finishes, the socket teardown triggers
`SIGABRT` and the whole test process dies.

### Running UI tests

```bash
# All UI tests
python -m pytest tests/test_chat_panel_turn_rendering.py \
                 tests/test_context_meter_widget.py \
                 tests/test_phase6_context_panel.py \
                 tests/test_file_explorer_coverage.py \
                 tests/test_file_explorer_theme.py \
                 tests/test_gui_manager_integration.py \
                 -v

# Only tests that do NOT need a live Ollama instance
python -m pytest -m "not live" -v
```

---

## 7. Known Coverage Gaps

The following affordance groups are **spec'd but untested** (📝) or **not yet spec'd**
(❌).  They represent the highest risk for silent drift.  Prioritise these when adding
tests.

### High Priority (visible to users, behaviour is non-trivial)

_(No outstanding high-priority gaps.)_

### Medium Priority (settings / configuration)

_All medium-priority items have been addressed._

### Low Priority (modal dialogs, less frequent use)

| Affordance ID | Description |
|---------------|-------------|
| PD-05-AF-004–006 | PlanTreeWidget Re-synth, Export, node status icons |

---

## 8. Keeping This Document Current

### Agent responsibility

When the AI agent makes any code change to `src/agentx/gui/`:

1. Search the Traceability Matrix for the affected affordance.
2. Update the `Status` column if tests were added or removed.
3. Add new rows for any new affordances introduced.
4. Remove rows for any affordances deleted.
5. Commit `docs/ux/UX_LIFECYCLE.md` as part of the same commit.

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
