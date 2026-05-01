# Changelog

All notable changes to AgentX are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

---

## [0.22.6] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py`: Changed right-click trigger binding from
  `<ButtonRelease-3>` to `<Button-3>` (press).  Root cause identified: when
  `menu.post()` is called from a `<ButtonRelease-3>` handler it creates the menu window
  at `(x_root, y_root)` — directly under the cursor — so the X server sends an `<Enter>`
  event to the new menu window.  The Tk `Menu` class has a generic `<ButtonRelease>`
  class binding (`tk::MenuInvoke`) that fires when any button is released over the menu;
  with no active item it calls `unpost()`.  Result: menu appeared and immediately
  vanished.  With `<Button-3>` (press) and `menu.post()` (no grab), the subsequent
  `<ButtonRelease-3>` goes to whichever window the cursor is over at release time — the
  menu (item invoked ✓) or the treeview (ignored ✓) — never triggering the auto-unpost.
  `<Control-ButtonRelease-1>` similarly changed to `<Control-Button-1>`.

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`:
  - Renamed `test_right_click_bound_to_button_release_not_press` →
    `test_right_click_bound_to_button_press_not_release`; assertion now verifies
    `<Button-3>` is bound and `<ButtonRelease-3>` is NOT.
  - Updated module docstring to document full root cause history (v0.22.1–0.22.6).
  - All 15 tests pass.

---

## [0.22.5] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py` `_on_right_click()`: Replaced `tk_popup()` with
  `menu.post()`.  All previous fixes (v0.22.1–0.22.4) were workarounds to symptoms of
  `tk_popup()`'s internal `grab` command.  On any modern Linux compositor the WM cancels
  Tk's grab immediately after `tk_popup()` sets it — there is no reliable way to keep the
  grab.  `menu.post()` displays the menu without setting any grab.  Tk's native
  root-window `<ButtonPress>` binding handles auto-dismiss; `<Escape>` remains bound.
  Removed the now-unnecessary `after_idle(grab_release)` call.

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`: Updated file and folder right-click tests
  to assert `menu.post()` is called and `tk_popup()` is NOT called.  Added docstring
  note documenting the inherent limitation of unit tests for this behavior (compositor
  grab conflicts cannot be detected headlessly; manual UAT is required).

---

## [0.22.4] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py` `_on_right_click()`: Replaced synchronous `try/finally`
  `grab_release()` with `menu.after_idle(menu.grab_release)` and added `return "break"`.
  The synchronous call fired before Tk drained its event queue — queued grab-related events
  could still cause `unpost()` afterward, producing the intermittent 1-in-12 success rate.
  `after_idle` defers the release until the queue is empty.  `return "break"` stops the
  `<ButtonRelease-3>` event from propagating to parent widgets or root-window bindings that
  could also close the menu.

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`: Updated file and folder right-click tests
  to assert `after_idle(grab_release)` is scheduled and handler returns `"break"`,
  replacing the previous synchronous `grab_release` assertions.

---

## [0.22.3] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py` `_on_right_click()`: Added `try/finally` block that calls
  `menu.grab_release()` immediately after `tk_popup()`.  On Linux with modern compositors
  (GNOME/Mutter, KWin, Wayland/XWayland), `tk_popup()` sets a server-side passive grab
  that conflicts with the WM's own grab.  The WM resolves the conflict by cancelling Tk's
  grab, which causes Tk to unpost the menu immediately.  `grab_release()` frees the
  conflicting grab while leaving the menu posted; Tk's native `<Leave>` and root
  `ButtonPress` bindings still handle auto-dismiss correctly.
  This is root cause #3; causes #1 (FocusOut) and #2 (ButtonRelease binding) were fixed
  in v0.22.1 and v0.22.2 respectively.

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`: Updated `test_right_click_file_calls_tk_popup_on_file_menu`
  and `test_right_click_directory_calls_tk_popup_on_folder_menu` to also assert
  `grab_release()` is called after `tk_popup()`.

### Documentation Changes

#### Changed

- `docs/ux/UX_ISSUES.md`: Added root cause #3 and side-effect explanation (accidental
  Attach trigger); updated attempt count to 3.
- `docs/ux/03_PANEL_DETAILS.md` PD-11: Extended dismiss-behaviour note to cover
  `grab_release()` rationale.

---

## [0.22.2] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py`: Changed right-click binding from `<Button-3>` to
  `<ButtonRelease-3>` (and `<Control-Button-1>` → `<Control-ButtonRelease-1>`).
  On Linux/X11, `tk_popup()` sets up an internal grab when it opens.  When the binding
  was on the button-press event, the corresponding button-release event was captured by
  that grab and immediately dismissed the menu.  Binding to the release event means the
  release has already been consumed before the popup opens, so the menu stays visible.
  This is the second fix for UX_ISSUES.md issue #1 (first fix in v0.22.1 removed the
  FocusOut binding).

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`: Added
  `test_right_click_bound_to_button_release_not_press` to `TestFileContextMenu` — asserts
  `<ButtonRelease-3>` is bound and `<Button-3>` is absent.  15 tests total (was 14).

### Documentation Changes

#### Changed

- `docs/ux/UX_ISSUES.md`: Issue #1 updated with second root-cause and marked `[/]`.
- `docs/ux/03_PANEL_DETAILS.md` PD-11: Updated the dismiss-behaviour note to explain
  both the ButtonRelease and FocusOut rationale.

---

## [0.22.1] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py`: Removed the `<FocusOut>` binding from the treeview
  in `to_gui()`.  The binding was calling `_dismiss_popup_menu()` the instant
  `tk_popup()` stole focus from the tree, causing context menus to appear and
  immediately vanish on Linux/X11.  `<Escape>` and clicking outside the menu remain
  as the natural dismiss paths.  Fixes UX_ISSUES.md issue #1.

### Test Changes

#### Added

- `tests/test_file_explorer_context_menu.py` — 14 `@pytest.mark.unit` tests across
  three classes covering the new affordances:

  `TestFileContextMenu` (PD-11-AF-008):
  - GIVEN file row right-clicked THEN file menu posted, folder menu not.
  - GIVEN empty area right-clicked THEN no menu posted.
  - GIVEN widget created THEN `<FocusOut>` is NOT bound on the tree.
  - GIVEN file selected WHEN Attach activated THEN on_attach called with correct path.
  - GIVEN file selected WHEN Edit activated THEN on_edit called with correct path.
  - GIVEN no on_attach callback THEN no exception raised on activation.

  `TestFolderContextMenu` (PD-11-AF-009):
  - GIVEN directory row right-clicked THEN folder menu posted, file menu not.
  - GIVEN directory selected WHEN "Add full path" activated THEN callback with abs path.
  - GIVEN directory selected WHEN "Add relative path" activated THEN callback with rel path.
  - GIVEN no callback THEN no exception raised on activation.

  `TestDismissContextMenu` (PD-11-AF-010):
  - GIVEN widget created THEN `<Key-Escape>` is bound on the tree.
  - GIVEN _dismiss_popup_menu called THEN unpost called on both menus.
  - GIVEN no event arg THEN _dismiss_popup_menu does not raise.
  - GIVEN synthetic event THEN _dismiss_popup_menu does not raise.

### Documentation Changes

#### Added

- `docs/ux/UX_ISSUES.md`: Updated issue #1 to `[/]` with root-cause and fix notes.

#### Changed

- `docs/ux/03_PANEL_DETAILS.md` PD-11 section:
  - Corrected "Escape / focus lost" dismiss interaction to "Escape only" with an
    explanatory note about why FocusOut is not used.
  - Added Gherkin use-cases for PD-11-AF-008, PD-11-AF-009, PD-11-AF-010.
- `docs/ux/UX_LIFECYCLE.md` PD-11 matrix: added rows for PD-11-AF-008, PD-11-AF-009,
  PD-11-AF-010 (all ✅).
- `docs/ux/00_INDEX.md`: PD-11 row `7✅` → `10✅`; totals `61✅` → `64✅`.

---

## [0.22.0] - 2026-05-01

### Code Changes

#### Added

- `src/agentx/gui/model_selector.py`: Added `on_refresh` constructor parameter,
  `refresh_btn` (`ttk.Button` with glyph `⟳`), `set_refresh_callback()`, and
  `_on_refresh()`. Pressing the button invokes the registered callback, which is
  wired through the full chain to reload the Ollama model list. Implements PD-04-AF-004.
- `src/agentx/gui/side_panel.py`: Added `set_refresh_models_callback()` — forwards
  the callback down to `ModelSelector`.
- `src/agentx/gui/gui_manager.py`: Added `set_refresh_models_callback()` delegate.
- `src/agentx/igui_manager.py`: Added `set_refresh_models_callback()` Protocol method.
- `src/agentx/session.py`: Added `_refresh_models()` inner function and
  `gui.set_refresh_models_callback(_refresh_models)` call in `_setup_agentix_ui()`.

### Test Changes

#### Added

- `tests/test_model_selector_refresh.py` — 8 `@pytest.mark.unit` tests in
  `TestModelSelectorRefreshButton` covering PD-04-AF-004:
  - GIVEN ModelSelector created THEN refresh_btn is a ttk.Button.
  - GIVEN ModelSelector created THEN refresh_btn is packed inside the widget frame.
  - GIVEN ModelSelector created THEN refresh_btn text is "⟳".
  - GIVEN refresh callback registered WHEN button clicked THEN callback invoked once.
  - GIVEN no callback registered WHEN button clicked THEN no exception raised.
  - GIVEN existing callback WHEN set_refresh_callback() called with new callback THEN only new callback invoked.
  - GIVEN no callback WHEN set_refresh_callback() called THEN late-registered callback works on next click.
  - GIVEN callback set WHEN set_refresh_callback(None) called THEN subsequent click does not raise.

- `tests/test_plan_tree_affordances.py` — 16 `@pytest.mark.unit` tests across three
  classes covering PD-05-AF-004, PD-05-AF-005, PD-05-AF-006:

  `TestNodeStatusIconReflectsState` (PD-05-AF-006):
  - GIVEN pending status WHEN update_node_status called THEN icon is ○.
  - GIVEN running status WHEN update_node_status called THEN icon is ●.
  - GIVEN done status WHEN update_node_status called THEN icon is ✓.
  - GIVEN needs_review status WHEN update_node_status called THEN icon is ?.
  - GIVEN failed status WHEN update_node_status called THEN icon is ✗.
  - GIVEN unknown status WHEN update_node_status called THEN fallback bullet shown, no exception.
  - GIVEN missing task_id WHEN update_node_status called THEN no exception.
  - GIVEN multiple status transitions THEN each call updates icon correctly.

  `TestResynthButtonInSynthesisBlock` (PD-05-AF-004):
  - GIVEN on_resynth callback provided WHEN add_synthesis_to_node called THEN Re-synth button present.
  - GIVEN no on_resynth callback WHEN add_synthesis_to_node called THEN no Re-synth button.
  - GIVEN on_resynth callback THEN callback stored on synthesis widget for later invocation.
  - GIVEN missing task_id with on_resynth callback THEN no exception.

  `TestExportButtonInPlanTab` (PD-05-AF-005):
  - GIVEN plan tab created THEN Export button present in toolbar.
  - GIVEN plan tab with on_export callback WHEN Export clicked THEN callback invoked once.
  - GIVEN plan tab with no on_export callback THEN Export button still present.
  - GIVEN no on_export callback WHEN Export clicked THEN no exception.

### Documentation Changes

#### Changed

- `docs/ux/UX_LIFECYCLE.md`:
  - PD-04-AF-004 📝 → ✅; corrected source from non-existent `_on_refresh()` (method
    now added) and added test file/class refs.
  - PD-05-AF-004 📝 → ✅; corrected source from `_add_resynth_button()` to actual
    `_create_synthesis_block()`; added test file/class refs.
  - PD-05-AF-005 📝 → ✅; corrected source from `_on_export()` to `ChatPanel.add_plan_tab()`
    and `AgentXSession._export_task_tree()`; added test file/class refs.
  - PD-05-AF-006 📝 → ✅; corrected source from `_node_icon()` to `update_node_status()`
    and `_STATUS_ICONS`; added test file/class refs.
- `docs/ux/00_INDEX.md`:
  - PD-04 row: `3✅·1📝` → `4✅·0📝`.
  - PD-05 row: `3✅·3📝` → `6✅·0📝`.
  - Totals: `57✅·4📝` → `61✅·0📝`.
  - Priority Work Queue: added 4 `[/]` completed entries for PD-04-AF-004, PD-05-AF-004/005/006.
- `docs/ux/03_PANEL_DETAILS.md`:
  - PD-04 section: added Gherkin use-cases for PD-04-AF-004.
  - PD-05 Controls section: added Gherkin use-cases for PD-05-AF-004, PD-05-AF-005, PD-05-AF-006.

---

## [0.21.0] - 2026-04-30

### Code Changes

#### Added

- `src/agentx/gui/input_panel.py`: Implemented `_on_shift_return()` method and wired a
  `<Shift-Return>` binding on `user_input_text` in `InputPanel.create()`. Pressing
  Shift+Enter now inserts a newline at the cursor position and returns `"break"` to
  suppress Tkinter's default key handling. Implements PD-02-AF-002.

### Test Changes

#### Added

- `tests/test_input_panel_keyboard.py` — 5 `@pytest.mark.unit` tests in
  `TestShiftEnterInsertsNewline` covering PD-02-AF-002:
  - GIVEN input text is empty WHEN Shift+Enter pressed THEN widget contains a newline.
  - GIVEN input contains "hello" WHEN Shift+Enter pressed THEN content is "hello\\n".
  - GIVEN InputPanel created WHEN _on_shift_return called THEN return value is "break".
  - GIVEN InputPanel created WHEN bindings queried THEN Shift+Return binding present.
  - GIVEN input contains "ab" with cursor mid-word WHEN Shift+Enter pressed THEN content is "a\\nb".

### Documentation Changes

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: Added 5 Gherkin use-cases for PD-02-AF-002 and 5
  test-mapping rows for `test_input_panel_keyboard.py` in the PD-02 cut-sheet.
- `docs/ux/UX_LIFECYCLE.md`: PD-02-AF-002 📝 → ✅; corrected source method reference from
  non-existent `_bind_keys()` to the actual `_on_shift_return()` implementation.
- `docs/ux/00_INDEX.md`: PD-02 InputPanel row updated (4✅·3⚠️·0📝), totals
  (57✅·4📝), queue item `PD-02-AF-002` appended and marked complete.

---

## [0.20.3] - 2026-05-02

### Code Changes

#### Fixed

- `src/agentx/gui/settings_tab.py`: `_add_text_entry()` and `_add_spinbox()` now correctly
  append `RESTART_ICON` to the label when `restart=True` is passed, matching the existing
  behaviour of `_add_checkbox()`. Previously the `restart` parameter was accepted but silently
  ignored in both helpers, causing the 🔁 icon to be absent from Ollama Host and Load timeout
  labels. Callers that were compensating by manually including `RESTART_ICON` in the label
  string (`Agentix Host`, `Torch model`, `Torch device`) have had the explicit suffix removed
  to avoid duplication.

### Test Changes

#### Added

- `tests/test_settings_tab_sections.py` — 19 `@pytest.mark.unit` tests across 2 classes
  covering PD-07-AF-002 and PD-07-AF-003:
  - `TestSettingsTabSectionCollapseDefaults` (5 tests — PD-07-AF-002):
    - GIVEN SettingsTab constructed WHEN 🎨 Appearance section inspected THEN expanded=True.
    - GIVEN SettingsTab constructed WHEN 🤖 Ollama section inspected THEN expanded=True.
    - GIVEN SettingsTab constructed WHEN 🧠 Agentix section inspected THEN expanded=True.
    - GIVEN SettingsTab constructed WHEN 📊 Classification Display inspected THEN expanded=False.
    - GIVEN SettingsTab constructed WHEN 🏛️ Working Memory inspected THEN expanded=False.
  - `TestRestartIconInLabels` (14 tests — PD-07-AF-003):
    - GIVEN SettingsTab constructed WHEN RESTART_ICON constant read THEN equals `🔁`.
    - GIVEN SettingsTab constructed WHEN Theme mode label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Host labels located THEN at least one contains 🔁.
    - GIVEN SettingsTab constructed WHEN Load timeout label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Screen side label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Default model label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Enabled (WM) label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Torch model label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Torch device label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Classify prompts label located THEN no 🔁.
    - GIVEN SettingsTab constructed WHEN Debug logging label located THEN no 🔁.
    - GIVEN SettingsTab constructed WHEN Backend label located THEN no 🔁.
    - GIVEN SettingsTab constructed WHEN Inject into LLM context label located THEN no 🔁.
    - GIVEN SettingsTab constructed WHEN Max facts label located THEN no 🔁.

### Documentation Changes

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: Added PD-07-AF-002 (section collapse defaults) and
  PD-07-AF-003 (restart-required icon in label text) cut-sheet blocks in the PD-07 section.
- `docs/ux/UX_LIFECYCLE.md`: PD-07-AF-002 📝 → ✅, PD-07-AF-003 📝 → ✅; corrected
  PD-07-AF-003 source method reference from non-existent `_make_restart_tooltip()` to the
  actual implementation (`RESTART_ICON` class constant + widget factory helpers); removed both
  from the Medium Priority gaps section.
- `docs/ux/00_INDEX.md`: PD-07 SettingsTab row updated (2✅·1⚠️·0📝), totals
  (56✅·5📝), queue item `PD-07-AF-002..003` marked complete; last-updated date refreshed.

---

## [0.20.2.post7] - 2026-04-30

### Code Changes

#### Added

- `tests/test_working_memory_widget_callbacks.py`: 17 unit tests covering PD-03-AF-011..014.

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: Added full cut-sheet sections for PD-03-AF-011 (toggle), PD-03-AF-012 (delete), PD-03-AF-013 (promote), PD-03-AF-014 (add-fact form) in the Working Memory sub-section of PD-03.
- `docs/ux/UX_LIFECYCLE.md`: PD-03-AF-011..014 📝 → ✅, source method refs corrected, removed from Medium Priority gaps.
- `docs/ux/00_INDEX.md`: PD-03 Working Memory row updated (4✅/0📝), totals (54✅/0❌), queue item marked complete.

### Test Changes

#### Added

- `tests/test_working_memory_widget_callbacks.py` — 17 `@pytest.mark.unit` tests across 4 classes:
  - `TestWorkingMemoryToggle` (5 tests — PD-03-AF-011):
    - GIVEN fact(enabled=True) WHEN rendered THEN Checkbutton variable is True.
    - GIVEN fact(enabled=False) WHEN rendered THEN Checkbutton variable is False.
    - GIVEN fact(enabled=True) WHEN invoked THEN on_toggle called with (key, False).
    - GIVEN fact(enabled=False) WHEN invoked THEN on_toggle called with (key, True).
    - GIVEN on_toggle=None WHEN invoked THEN no exception raised.
  - `TestWorkingMemoryDelete` (4 tests — PD-03-AF-012):
    - GIVEN AGENT fact WHEN ✕ clicked + confirmed THEN on_delete called.
    - GIVEN AGENT fact WHEN ✕ clicked + cancelled THEN on_delete NOT called.
    - GIVEN USER fact WHEN rendered THEN no ✕ button present.
    - GIVEN on_delete=None WHEN confirmed THEN no exception raised.
  - `TestWorkingMemoryPromote` (4 tests — PD-03-AF-013):
    - GIVEN AGENT fact WHEN icon clicked + confirmed THEN on_promote called.
    - GIVEN AGENT fact WHEN icon clicked + cancelled THEN on_promote NOT called.
    - GIVEN USER fact WHEN rendered THEN owner icon is Label not Button.
    - GIVEN on_promote=None WHEN confirmed THEN no exception raised.
  - `TestWorkingMemoryAddFact` (4 tests — PD-03-AF-014):
    - GIVEN key+value entered WHEN ‘Add 👤’ clicked THEN on_user_add called with (key, value).
    - GIVEN key+value entered WHEN submitted THEN both entries cleared.
    - GIVEN empty key WHEN ‘Add 👤’ clicked THEN on_user_add NOT called.
    - GIVEN on_user_add=None WHEN submitted THEN no exception raised.

---

## [0.20.2.post6] - 2026-04-30

### Code Changes

#### Added

- `tests/test_context_renderer_message_enabled.py`: 4 unit tests for PD-03-AF-007 (checkbox initial state true/false, uncheck→False, check→True).

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: PD-03 Context section expanded with full PD-03-AF-007 cut-sheet entry — behaviour table, Gherkin use-cases, test mapping.
- `docs/ux/UX_LIFECYCLE.md`: PD-03-AF-007 📝 → ✅; removed from Medium Priority gaps list.
- `docs/ux/00_INDEX.md`: PD-03 Context row updated (7✅/0📝), totals (50✅/0❌), PD-03-AF-007 marked complete.

### Test Changes

#### Added

- `tests/test_context_renderer_message_enabled.py` — 4 `@pytest.mark.unit` tests:
  - GIVEN `enabled=True` WHEN row rendered THEN Checkbutton variable is True. (`test_enabled_checkbox_initial_true` — PD-03-AF-007)
  - GIVEN `enabled=False` WHEN row rendered THEN Checkbutton variable is False. (`test_enabled_checkbox_initial_false` — PD-03-AF-007)
  - GIVEN `enabled=True` WHEN Checkbutton invoked THEN `message.enabled` is False. (`test_uncheck_sets_message_enabled_false` — PD-03-AF-007)
  - GIVEN `enabled=False` WHEN Checkbutton invoked THEN `message.enabled` is True. (`test_check_sets_message_enabled_true` — PD-03-AF-007)

---

## [0.20.2.post5] - 2026-04-30

### Code Changes

#### Added

- `tests/test_input_panel_attachment_chips.py`: 9 unit tests covering PD-02-AF-005..007 (chip render with filename/icon, toggle callback, rebuild clears chips).

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: PD-02 (InputPanel) fully backfilled to cut-sheet standard — placement diagram, internal structure, behaviour inventory (7 affordances AF-001..007), Gherkin use-cases, test mapping table, code references. Corrected inaccurate chip description (was `[×]` remove button, now reflects actual toggle-checkbutton implementation).
- `docs/ux/UX_LIFECYCLE.md`: PD-02-AF-005..007 rows updated from 3 📝 (referencing non-existent methods) to 3 ✅ (referencing actual `_create_attachment_widget` / `update_attachment_bar`); High Priority gaps section cleared.
- `docs/ux/00_INDEX.md`: PD-02 row updated (3✅/3⚠️/1📝/0❌), totals updated (49✅/0❌), PD-02-AF-005..007 marked complete in Priority Work Queue.

### Test Changes

#### Added

- `tests/test_input_panel_attachment_chips.py` — 9 `@pytest.mark.unit` tests:
  - GIVEN `display_name="parser.py"` and `is_from_history=False` WHEN `update_attachment_bar([info], [])` THEN chip label contains `"parser.py"`. (`test_current_attachment_chip_shows_filename` — PD-02-AF-005)
  - GIVEN current-turn chip WHEN rendered THEN Checkbutton text starts with `"\ud83d\udcc1"`. (`test_current_attachment_chip_uses_folder_icon` — PD-02-AF-005)
  - GIVEN `is_from_history=True` WHEN rendered THEN text contains `"old_file.txt"` and `"history"`. (`test_history_attachment_chip_shows_filename_and_history_suffix` — PD-02-AF-005)
  - GIVEN history chip WHEN rendered THEN text starts with `"\ud83d\udcdc"`. (`test_history_attachment_chip_uses_scroll_icon` — PD-02-AF-005)
  - GIVEN two infos WHEN `update_attachment_bar([a, b], [])` THEN two chip frames. (`test_multiple_chips_rendered_in_order` — PD-02-AF-005)
  - GIVEN `enabled=True`, `attachment_id="att-x"` WHEN checkbox invoked THEN `on_toggle("att-x", False)`. (`test_uncheck_calls_on_attachment_toggle_false` — PD-02-AF-006)
  - GIVEN `enabled=False`, `attachment_id="att-y"` WHEN checkbox invoked THEN `on_toggle("att-y", True)`. (`test_check_after_uncheck_calls_toggle_true` — PD-02-AF-006)
  - GIVEN 1 chip rendered WHEN `update_attachment_bar([], [])` THEN `attachment_labels` empty. (`test_empty_update_clears_all_chips` — PD-02-AF-007)
  - GIVEN `"old.py"` chip WHEN rebuilt with `"new.py"` THEN only `"new.py"` chip present. (`test_rebuild_replaces_existing_chips` — PD-02-AF-007)

---

## [0.20.2.post4] - 2026-04-29

### Code Changes

#### Added

- `tests/test_resynthesis_dialog.py`: 7 unit tests covering all 5 PD-06 affordances (title, cancel, confirm with/without hint, WM section visibility, Add WM hint callback).

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: PD-06 (ResynthesisDialog) fully backfilled to cut-sheet standard — placement diagram, internal structure diagram, behaviour inventory (5 affordances: AF-001..005), Gherkin use-cases, test mapping, code references.
- `docs/ux/UX_LIFECYCLE.md`: PD-06 matrix expanded from 3 rows (❌) to 5 rows (✅); PD-06-AF-004..005 added for WM hint section and callback; removed from §7 Known Coverage Gaps.
- `docs/ux/00_INDEX.md`: PD-06 status row updated (5✅/0❌), totals updated (46✅/0❌), PD-06 removed from Priority Work Queue.

### Test Changes

#### Added

- `tests/test_resynthesis_dialog.py` — 7 `@pytest.mark.unit` tests for ResynthesisDialog:
  - GIVEN `task_id="step-42"` WHEN dialog created THEN title is `"Re-synthesise — step-42"`. (`test_title_includes_task_id` — PD-06-AF-001)
  - GIVEN mock `on_confirm` WHEN Cancel invoked THEN `on_confirm` not called AND window destroyed. (`test_cancel_destroys_dialog_without_confirm` — PD-06-AF-002)
  - GIVEN hint text `"focus on error handling"` WHEN Re-synthesise invoked THEN `on_confirm("focus on error handling")` called AND window destroyed. (`test_confirm_calls_on_confirm_with_hint` — PD-06-AF-003)
  - GIVEN empty hint WHEN Re-synthesise invoked THEN `on_confirm("")` called. (`test_confirm_with_empty_hint_passes_empty_string` — PD-06-AF-003)
  - GIVEN `on_add_wm_hint=None` WHEN dialog created THEN no "Add WM hint" button present. (`test_wm_section_hidden_without_callback` — PD-06-AF-004)
  - GIVEN `on_add_wm_hint` provided WHEN dialog created THEN "Add WM hint" button is present. (`test_wm_section_visible_with_callback` — PD-06-AF-004)
  - GIVEN key="style" value="concise" WHEN Add WM hint invoked THEN callback called, fields cleared, dialog open. (`test_add_wm_hint_calls_callback_and_clears_fields` — PD-06-AF-005)

---

## [0.20.2.post3] - 2026-04-29

### Code Changes

#### Added

- `tests/test_chat_panel_collapse_defaults.py`: 3 unit tests locking down PD-01-AF-005..007 (thinking collapsed, tool call collapsed, assistant response expanded on initial render).

#### Changed

- `docs/ux/UX_LIFECYCLE.md`: PD-01-AF-005..007 matrix rows updated 📝 → ✅ with test file and test function references; rows removed from §7 Known Coverage Gaps.
- `docs/ux/00_INDEX.md`: PD-01 status row updated (7✅/1⚠️/0📝), totals updated (41✅/15📝), PD-01-AF-005..007 removed from Priority Work Queue.

### Test Changes

#### Added

- `tests/test_chat_panel_collapse_defaults.py` — 3 `@pytest.mark.unit` tests for ChatPanel entry collapse defaults.
  - GIVEN a turn started WHEN `display_agent_thinking()` called THEN thinking entry has `expanded=False` and `detail_text` not visible. (`test_thinking_entry_collapsed_by_default` — PD-01-AF-005)
  - GIVEN a turn started WHEN a `[🔧 Calling tool` line received via `display_agent_response()` THEN tool_call entry has `expanded=False` and `detail_text` not visible. (`test_tool_call_entry_collapsed_by_default` — PD-01-AF-006)
  - GIVEN a turn started WHEN `display_agent_response()` streams content THEN assistant entry has `expanded=True` and `detail_text` is visible. (`test_assistant_response_entry_expanded_by_default` — PD-01-AF-007)

---

## [0.20.2.post2] - 2026-04-29

### Code Changes

#### Added

- `docs/ux/04_COMPONENT_CUT_SHEET_TEMPLATE.md`: reusable component cut-sheet standard with sections for placement diagram, internal structure diagram, behaviour inventory table, Gherkin use-cases, test mapping table, and code/configuration references.

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: PD-09 (CollapsibleSection) backfilled to full cut-sheet exemplar — placement diagram, internal structure diagram, behaviour inventory (4 affordances), Gherkin scenarios, test mapping, code references.
- `docs/ux/UX_LIFECYCLE.md`: PD-09 traceability matrix rows all updated from 📝 → ✅ with concrete test file and test name references; PD-09-AF-001..004 removed from §7 Known Coverage Gaps.
- `docs/ux/00_INDEX.md`: status snapshot updated (38✅ total), requirement intake 5-step process added, cut-sheet template linked, PD-09 removed from Priority Work Queue.
- `docs/ux/README.md`: `04_COMPONENT_CUT_SHEET_TEMPLATE.md` added to contents table.

### Test Changes

#### Added

- `tests/test_collapsible_section.py`: 4 hermetic unit tests locking down all PD-09 affordances.
  - GIVEN a CollapsibleSection with `initial_collapsed=True` WHEN created THEN `is_expanded()` is False and content_container has no geometry manager. (`test_initial_collapsed_state_hides_content_container` — PD-09-AF-001)
  - GIVEN a CollapsibleSection with `initial_collapsed=False` WHEN created THEN `is_expanded()` is True and content_container is managed by pack. (`test_initial_expanded_state_shows_content_container` — PD-09-AF-002)
  - GIVEN a CollapsibleSection WHEN `toggle()` is called THEN expanded state flips and content_container visibility toggles accordingly. (`test_toggle_flips_state_and_visibility` — PD-09-AF-003)
  - GIVEN a CollapsibleSection with existing content WHEN `set_content()` is called with a new widget THEN previous widget is destroyed and only the new widget remains. (`test_set_content_replaces_previous_widget` — PD-09-AF-004)

---

## [0.20.2.post1] - 2026-04-29

### Documentation Changes

#### Added

- `docs/ux/00_INDEX.md` — session entry point for UX work; contains the Status
  Snapshot table, Priority Work Queue, and agile process flow diagram.  Both
  the developer and the agent open this file at the start of every UX session.
- `.github/prompts/ux-review.prompt.md` — `/ux-review` slash-command prompt
  implementing an 8-phase TDD review loop: Baseline → Specify → Cut-Sheet
  Verify → Gherkin Verify → Test-First Update → Iterative Code Fix →
  Reconcile → Commit Gate.
- `docs/ux/UX_LIFECYCLE.md` — single-source traceability hub; affordance ID
  scheme, complete traceability matrix for all 11 panels, change checklists,
  headless Tkinter testing primer, gap inventory, and audit commands.
  (File was created at end of prior session; captured in this post-release.)

#### Changed

- `docs/ux/README.md` — `00_INDEX.md` added as the top entry; `UX_LIFECYCLE.md`
  listed as lifecycle rules document.
- `.github/copilot-instructions.md` — UX section updated to require agent opens
  `00_INDEX.md` first (Status Snapshot + Priority Work Queue) before any
  `src/agentx/gui/` change.

### Code Changes

#### Fixed

- `src/agentx/gui/context_renderer._render_message_to_grid()` — regression where
  plain messages (no tools, attachments, or plans) received `is_expandable=False`,
  rendering an empty placeholder `Label` in the expand column instead of a toggle
  `Button`.  This made the full message content inaccessible in the Context panel.
  Fix: removed the `is_expandable` conditional entirely.  Every message now always
  gets an expand/collapse `Button` (col 0) and a hidden full-content `tk.Label`
  detail row that is revealed on toggle.

### Test Changes

#### Added

- `tests/test_phase6_context_panel.py` — new `TestRenderMessageAlwaysExpandable`
  class (7 unit tests):
  - `test_plain_user_message_has_expand_button` — GIVEN a plain user message WHEN
    rendered THEN a Button is placed in the exp_button column.
  - `test_plain_assistant_message_has_expand_button` — same for assistant role.
  - `test_plain_system_message_has_expand_button` — same for system role.
  - `test_full_content_row_created_and_hidden_by_default` — GIVEN a 200-char message
    WHEN rendered THEN a hidden full-content Label exists.
  - `test_full_content_row_becomes_visible_on_toggle` — GIVEN a plain message WHEN
    expand button is clicked THEN the full-content label becomes visible.
  - `test_message_with_tool_still_has_expand_button` — GIVEN a message with a tool
    call WHEN rendered THEN the expand button is still present.
  - `test_empty_content_message_has_expand_button_no_detail_row` — GIVEN an empty
    message WHEN rendered THEN expand button exists but no hidden detail row is added.

#### Fixed

- `tests/test_phase6_context_panel.py` — pre-existing SIGABRT race: background
  `ModelMetadataStore.populate` threads (spawned by `AgentXSession.__init__`) made
  HTTP calls during test teardown, sometimes hitting a destroyed socket/GC and
  aborting the entire process.  Fixed by adding
  `patch("agentx.model_metadata_store.ModelMetadataStore.populate")` to `_make_session`.

---

## [0.20.1] - 2026-04-28

### Code Changes

#### Fixed

- `src/agentx/gui/gui_manager.py` — `update_context_meter()` had regressed to the no-op
  stub after the editor saved a stale in-memory buffer over the committed fix. Re-applied
  the real delegation: `self._input_panel.context_meter.update(max_tokens, breakdown)`.
  This was the root cause of the context visualiser remaining at 0% throughout a session.
- `src/agentx/model_metadata_store.get_context_length()` — added `:latest` tag fallback so
  bare model names (e.g. `gpt-oss`) resolve to their tagged equivalent (`gpt-oss:latest`).
  Ollama always appends `:latest` implicitly, causing spurious *"missing from metadata store"*
  warnings and returning `FALLBACK_CONTEXT_WINDOW` instead of the real context length.
- `src/agentx/model_metadata_store.get_metadata()` — applied the same `:latest` tag
  fallback for consistency with `get_context_length()`.

### Test Changes

#### Added

- `tests/test_model_metadata_store.py` — 6 new unit tests:
  - `test_get_context_length_latest_tag_fallback` (×4 parametrized):
    - GIVEN model stored as `gpt-oss:latest` / WHEN looked up as `gpt-oss` / THEN returns real capacity
    - GIVEN model stored as `llama3.2:latest` / WHEN looked up as `llama3.2` / THEN returns real capacity
    - GIVEN exact tagged lookup `gpt-oss:latest` / WHEN looked up directly / THEN returns real capacity
    - GIVEN completely unknown model / WHEN looked up / THEN returns FALLBACK_CONTEXT_WINDOW
  - `test_get_metadata_latest_tag_fallback` (×2 parametrized):
    - GIVEN model stored as `gpt-oss:latest` / WHEN metadata requested as `gpt-oss` / THEN returns correct dict
    - GIVEN model stored as `llama3.2:latest` / WHEN metadata requested as `llama3.2` / THEN returns correct dict

---

## [0.20.0] - 2026-04-28

### Code Changes

#### Added

- `src/agentx/gui/context_meter_widget.py` — `ContextMeterWidget` (ARCH-04): donut chart Tkinter widget showing LLM context window utilisation by category. Features: seven band arcs (BAND-01–07), ghost arc for remaining capacity (ENH-02), border ring with three risk states (default / warning ≥80% / critical ≥100%, ENH-16), center percentage label with matching risk color, hover tooltips via `tk.Toplevel` (ENH-06), thread-safe `update()` via `canvas.after(0, ...)` (ARCH-06).
- `src/agentx/widget_registry.py` — added `context_meter_canvas: Optional[tk.Canvas]` field and corresponding `destroy_all()` teardown.

#### Changed

- `src/agentx/gui/input_panel.py` — imports and instantiates `ContextMeterWidget`, calls `create()` inside `InputPanel.create()`, and registers `context_meter_canvas` in the widget registry.
- `src/agentx/gui/gui_manager.py` — replaced `update_context_meter()` stub with real delegation to `self._input_panel.context_meter.update(max_tokens, breakdown)`.

### Test Changes

#### Added

- `tests/test_context_meter_widget.py` — 39 hermetic unit tests covering:
  - GIVEN new widget / WHEN `create()` called / THEN canvas placed at correct geometry
  - GIVEN `update()` on background thread / WHEN called / THEN `canvas.after(0, ...)` invoked (thread safety)
  - GIVEN each of seven band categories / WHEN `_render()` / THEN PIESLICE arc drawn with correct fill color (parameterized × 7)
  - GIVEN 50% usage / WHEN `_render()` / THEN ghost arc drawn with `_GHOST_COLOR`
  - GIVEN 100% usage / WHEN `_render()` / THEN no ghost arc drawn
  - GIVEN empty breakdown / WHEN `_render()` / THEN only ghost arc fills ring
  - GIVEN usage at each risk threshold / WHEN `_render()` / THEN border ring has correct color and width (parameterized × 8: 0%, 50%, 79%, 80%, 95%, 99%, 100%, 120%)
  - GIVEN usage at each risk threshold / WHEN `_render()` / THEN center label has correct fill color (parameterized × 4)
  - GIVEN 40% usage / WHEN `_render()` / THEN center label text is `"40%"`
  - GIVEN absurdly large token count / WHEN `_render()` / THEN center label capped at `"999%"`
  - GIVEN `max_tokens=0` / WHEN `_render()` / THEN no crash
  - GIVEN canvas not yet laid out / WHEN `_render()` / THEN retries via `canvas.after(50, ...)`
  - GIVEN each band / WHEN tooltip hover / THEN tooltip text contains band label and percentage (parameterized × 5)
  - GIVEN 50% usage / WHEN ghost arc hover / THEN tooltip shows "Remaining capacity" and token count
  - GIVEN destroyed Toplevel / WHEN `_hide_tooltip()` / THEN TclError silently swallowed
  - GIVEN no active tooltip / WHEN `_hide_tooltip()` / THEN no exception

## [0.19.3] - 2026-04-27

### Code Changes

#### Changed

- `src/agentix/models.py` now returns supplied cached `max_tokens` before any live model discovery so cached context-length lookups remain usable even when Ollama tag enumeration is unavailable.
- `src/agentix/bridge/tool_loop.py`, `src/agentix/bridge/bridge.py`, `src/agentx/integration/agentix_bridge_adapter.py`, and `src/agentx/session.py` now explicitly invalidate the bridge max-token cache when the active model changes, keeping prompt trimming aligned with the selected model.

#### Fixed

- Fixed the review regression where `get_model(..., max_tokens=...)` still hit `/api/tags` before honoring the cached value.
- Fixed stale tool-loop max-token caching after model switches, which could leave Agentix trimming against the previous model's context window.

### Test Changes

#### Added

- Hermetic regression tests proving cached `max_tokens` bypasses live model discovery and proving model changes invalidate the bridge/tool-loop max-token cache.

#### Fixed

- Targeted regression coverage for the corrected model-selection path remains at 98% for `agentix.models` with new cache-invalidation behaviors covered by hermetic unit tests.

## [0.19.2] - 2026-04-27

### Code Changes

#### Added

- `src/shared/providers/` introducing a shared provider boundary so `agentix` and `agentx` can both consume the same `ILLMServiceProvider`, constants, and Ollama adapter without reverse imports.
- `tests/test_agentix_models.py` and `tests/test_tool_loop_max_tokens.py` covering model-selection failures, cached max-token reuse, and tool-loop max-token wiring.

#### Changed

- `src/agentix/models.py` now hardens Ollama model enumeration, rejects malformed payloads, raises a clear error when no models match, and uses cached `max_tokens` when supplied.
- `src/agentix/bridge/tool_loop.py`, `src/agentix/agentix_config.py`, and `src/agentx/session.py` now propagate cached context-length values into the tool loop so Agentix avoids redundant live lookups when AgentX already knows model capacity.
- `src/agentx/protocols.py`, `src/agentx/session.py`, and `src/agentx/streaming_controller.py` now expose public context-meter protocol methods while keeping compatibility wrappers for existing call sites.
- `src/agentx/model_metadata_store.py` now exposes `population_failed` alongside `populated` so callers can distinguish completion from successful population.
- `src/agentx/providers/*` now act as compatibility wrappers over the shared provider implementation.

#### Fixed

- Removed the reverse dependency from `agentix` into `agentx.providers`, eliminating the reviewed layering violation.
- Fixed unhandled request/JSON failures and malformed response handling in Ollama model discovery.
- Fixed weak parameter-size validation and empty-model handling in Agentix model selection.
- Fixed the bridge max-token path so cached context lengths are actually consumed by the tool loop.

### Test Changes

#### Added

- Hermetic unit coverage for malformed Ollama payloads, fallback provider paths, model metadata cache failure semantics, public/compatibility meter APIs, and cached max-token routing into the tool loop.

#### Fixed

- Targeted hermetic coverage for the repaired core modules now reaches 98% (`agentix.models`, `agentx.model_metadata_store`, `shared.providers.ollama_provider`).

## [0.19.1] - 2026-04-27

### Code Changes

#### Added

- `src/agentx/providers/constants.py` introducing provider-scoped constants (`OLLAMA_MODELS_ENDPOINT`, `OLLAMA_SHOW_ENDPOINT`, `FALLBACK_CONTEXT_WINDOW`) to remove cross-tree imports.
- `src/agentx/protocols.py` introducing runtime-checkable `IMeterSession` for explicit context-meter contracts.
- `src/shared/token_utils.py` with module-level `chars_per_token()` and `estimate_text_tokens()` utilities.

#### Changed

- `src/agentx/providers/base.py` adds required `provider_id` contract to `ILLMServiceProvider`.
- `src/agentx/providers/ollama_provider.py` now exposes `provider_id = "ollama"`, accepts optional host values, and normalizes `None`/empty hosts safely.
- `src/agentx/model_metadata_store.py` now uses provider `provider_id` in cache payloads, exposes `populated: threading.Event`, unifies cache parsing with `_parse_cache_data()`, and adds `invalidate(model_name: str | None = None)` background refresh support.
- `src/agentx/session.py` now imports provider constants from `agentx.providers.constants`, starts model-store population asynchronously at startup, adds `on_context_assembled(shared_context)`, and simplifies `_context_meter_payload()` error-handling and model-name fallback semantics.
- `src/agentx/streaming_controller.py` replaces `hasattr` meter guards with `isinstance(..., IMeterSession)` checks and delegates assembled-context meter redraw via `on_context_assembled()`.
- `src/shared/models/context.py` now routes `MessageRole.SYNTHESIS` into the assistant meter band and uses shared token utilities while retaining compatibility shims.
- `src/agentix/models.py` adds optional fast-path `max_tokens` argument to avoid redundant live context-length HTTP calls.
- `pyproject.toml` test config now includes `pythonpath = ["src"]` to eliminate per-test `sys.path` mutation.

#### Fixed

- Addressed all 15 PR #5 review findings (A1-A7, P1-P8), including provider abstraction, cache semantics, meter protocol boundaries, startup threading behavior, and context-meter correctness.

### Test Changes

#### Added

- `tests/test_token_utils.py` (Unit):
  - GIVEN model-name families and text samples
  - WHEN token utility helpers run
  - THEN family ratios and ceiling token estimates are validated.
- `tests/test_protocols.py` (Unit):
  - GIVEN full and partial structural implementations
  - WHEN runtime protocol checks execute
  - THEN `IMeterSession` compatibility is correctly enforced.

#### Changed

- `tests/test_llm_service_provider.py`:
  - Removed `sys.path.insert` usage and switched constants import to `agentx.providers.constants`.
  - Added `provider_id` assertion and parametrized host-normalization coverage (`None`, empty, bare host, http/https variants).
- `tests/test_model_metadata_store.py`:
  - Removed `sys.path.insert`, updated constants import, and added `provider_id` to test provider.
  - Added coverage for `populated` event behavior, failure-path event setting, `invalidate()` single/all flows, provider-id cache serialization, and `_parse_cache_data()`.
- `tests/test_context_token_breakdown.py`:
  - Removed `sys.path.insert`.
  - Added explicit `SYNTHESIS` role routing assertions into `assistant` band.
- `tests/test_active_model_meter_wiring.py`:
  - Removed `sys.path.insert`, updated constants import.
  - Added `on_context_assembled()` meter-redraw behavior coverage.

#### Fixed

- New/updated targeted tests for PR #5 review scope now pass (`50 passed, 0 failed`).

## [0.19.0] - 2026-04-26

### Code Changes

#### Added

- `src/agentx/providers/base.py` with `ILLMServiceProvider` protocol.
- `src/agentx/providers/ollama_provider.py` implementing model enumeration and context-length lookup via Ollama endpoints.
- `src/agentx/model_metadata_store.py` for startup-populated, disk-backed model capacity metadata (`sessions/_model_cache.json`).

#### Changed

- `src/agentix/constants.py` adds `OLLAMA_SHOW_ENDPOINT` and `FALLBACK_CONTEXT_WINDOW`.
- `src/agentix/models.py` now derives `max_tokens` from provider context-length lookup instead of `parameter_size` proxy.
- `src/agentx/session.py` now initializes provider/store at startup, tags working-memory system messages via metadata, adds meter payload/scheduling helpers, and triggers redraw on model change, submit, and attachment-toggle events.
- `src/agentx/streaming_controller.py` now schedules meter redraw on submit-context assembly and after stream completion.
- `src/agentx/igui_manager.py` and `src/agentx/gui/gui_manager.py` now include `update_context_meter(max_tokens, breakdown)` contract/stub.
- `src/shared/models/message.py` now includes serializable `metadata` map.
- `src/shared/models/context.py` adds `token_breakdown(model_name)` with TOK-02 model-family ratios.
- `docs/context_size_prerequisite_plan.md` updated with Phase 1 audit findings and implementation progress tracking.
- `docs/ux/context_visualizer.md` marks PRE-02 complete and updates dynamic-context definition notes.
- `docs/architecture.md` module map updated with provider abstraction and metadata-store runtime flow.

#### Fixed

- Context-meter denominator source now aligns with active-model context window semantics instead of parameter-size heuristics.

### Test Changes

#### Added

- `tests/test_llm_service_provider.py` (Unit):
  - GIVEN Ollama tag/show payloads
  - WHEN provider methods are called
  - THEN model names, key-probe ordering, and fallback behavior are validated.
- `tests/test_model_metadata_store.py` (Unit):
  - GIVEN cache/no-cache startup conditions
  - WHEN `populate()` executes
  - THEN fetch/cached/fallback behavior is validated.
- `tests/test_context_token_breakdown.py` (Unit):
  - GIVEN enabled context messages and attachments
  - WHEN `token_breakdown()` executes
  - THEN role-band and attachment token estimates are correctly routed.
- `tests/test_active_model_meter_wiring.py` (Integration):
  - GIVEN active-model changes in session
  - WHEN setter logic runs
  - THEN GUI meter updates receive denominator and breakdown, including no-op and fallback paths.

#### Changed

- No existing tests removed; new PRE-02 tests run alongside the existing suite.

#### Fixed

- N/A

#### Removed

- N/A

## [0.18.26] - 2026-04-26

### Code Changes

#### Fixed

- **F-1 (HIGH)** `src/agentx/streaming_controller.py` — replay candidate lists (`original_tool_calls`, `original_tool_results`, `original_assistants`) now scoped to the replaying `task_id` via `and msg.task_id == _tid` filter, preventing cross-turn message contamination.
- **F-1 (HIGH)** `src/agentx/streaming_controller.py` — `_display_tool_call` and `_display_tool_result` now accept `task_id: str | None = None` and stamp it on persisted messages; streaming path passes the current task_id via a mutable box (`_current_task_id`); replay path passes `_tid` explicitly.
- **F-2 (HIGH)** `src/agentx/session.py` — `_persist_stream_messages` delegate restored missing `synthesis_of: list[str] | None = None` parameter and switched to keyword-based delegation to avoid positional argument corruption (the `refresh_gui` bool was being received as `synthesis_of`).
- **F-3 (MEDIUM)** `src/shared/models/message.py` — `from_dict` coerces non-list `synthesis_of` values to `[]` instead of storing them; `__post_init__` raises `ValueError` if `synthesis_of` is not a `list`.

### Test Changes

#### Added

- **Integration** `tests/test_replay_message_lineage_e2e.py` — T-1: `test_replay_does_not_supersede_prior_turn_messages` — verifies replay of a second task leaves prior-turn messages untouched.

  ```gherkin
  GIVEN a context with a prior turn (task_id=prior) and a replay target (task_id=target)
  WHEN the replay worker runs for task_id=target
  THEN messages from the prior turn (task_id=prior) are not superseded or modified
  ```

- **Integration** `tests/test_replay_message_lineage_e2e.py` — T-2: `test_replay_does_not_supersede_concurrent_plan_step_messages` — verifies replay of Step A in a two-step plan leaves Step B messages untouched.

  ```gherkin
  GIVEN a context with two plan steps (step_a_task_id, step_b_task_id)
  WHEN the replay worker runs for step_a_task_id
  THEN messages belonging to step_b_task_id are not superseded or modified
  ```

- **Integration** `tests/test_streaming_message_id_integration.py` — T-3: `test_persist_stream_messages_synthesis_of_is_list_type` — verifies the assistant message created by `_persist_stream_messages` has `isinstance(synthesis_of, list) == True`.

  ```gherkin
  GIVEN a StreamingController with a real Context
  WHEN _persist_stream_messages is called with synthesis_of=["msg_abc..."]
  THEN the persisted ASSISTANT message has synthesis_of as a list type
  ```

- **Integration** `tests/test_streaming_message_id_integration.py` — T-4: `test_session_delegate_forwards_synthesis_of_by_keyword` — verifies `session._persist_stream_messages` forwards `synthesis_of` and `refresh_gui` as keyword arguments to the controller.

  ```gherkin
  GIVEN a partially-initialized AgentXSession with a spy StreamingController
  WHEN session._persist_stream_messages(thinking, content, synthesis_of=[...], refresh_gui=False) is called
  THEN the spy records synthesis_of=[...] and refresh_gui=False (not positionally swapped)
  ```

- **Unit** `tests/test_message_model_ids.py` — T-5 (parametrized × 5): `test_from_dict_coerces_non_list_synthesis_of_to_empty_list` — verifies `Message.from_dict` coerces `True`, `False`, a bare string, `42`, and a dict to `[]`.

  ```gherkin
  GIVEN a message payload with synthesis_of=<non-list value>
  WHEN Message.from_dict(payload)
  THEN message.synthesis_of == [] and isinstance(message.synthesis_of, list) is True
  Permutations: True, False, "msg_a1b2c3...", 42, {"key": "value"}
  ```

- **Unit** `tests/test_message_model_ids.py` — `test_post_init_rejects_non_list_synthesis_of` — verifies `Message(..., synthesis_of=True)` raises `ValueError` mentioning `synthesis_of`.

  ```gherkin
  GIVEN synthesis_of=True (a boolean, not a list)
  WHEN Message(role=..., content=..., synthesis_of=True)
  THEN ValueError is raised with a message about synthesis_of type
  ```

#### Changed

- `tests/test_replay_message_lineage_e2e.py` — `_make_original_group` now stamps `task_id="task-1"` on all original messages so the scoped replay filter matches them correctly.

---

## [0.18.25] - 2026-04-25

### Code Changes

#### Changed

- `src/agentx/streaming_controller.py` — replay worker now persists lineage for replayed messages: replay `TOOL_CALL`/`TOOL_RESULT` messages set `cloned_from`, replay synthesis `ASSISTANT` sets `cloned_from` and `synthesis_of`, and supersession is applied only after replay outputs are fully persisted.
- `src/agentx/streaming_controller.py` — replay completion now applies `Context.supersede_message(...)` mappings so originals are disabled and point to replacements via `superseded_by`.

### Test Changes

#### Added

- **Integration/Functional** `tests/test_replay_message_lineage_e2e.py`: 20 parametrized tests covering replay lineage and supersession behavior end-to-end.

  ```gherkin
  GIVEN a replayed message group
  WHEN replay succeeds
  THEN replacements set cloned_from and originals set superseded_by
  ```

  ```gherkin
  GIVEN a replay attempt fails at any persistence stage
  WHEN replay exits with error
  THEN originals remain enabled and no superseded_by links are applied
  ```

  ```gherkin
  GIVEN replay emits tool results
  WHEN replay synthesis is persisted
  THEN synthesis_of references replay TOOL_RESULT message_ids only
  ```

  ```gherkin
  GIVEN replay replacement generation is persisted
  WHEN supersession completes
  THEN originals are disabled and replacements are enabled
  ```

  ```gherkin
  GIVEN multi-generation replay lineage
  WHEN get_ancestry is requested for the latest generation
  THEN ancestry is root-to-leaf and complete
  ```

  ```gherkin
  GIVEN replay supersession updates under interleaving patterns
  WHEN mappings are applied
  THEN supersession targets remain deterministic and lineage-safe
  ```

## [0.18.24] - 2026-04-20

### Code Changes

#### Added

- `tests/test_chat_panel_turn_rendering.py`: 10 new integration tests verifying correct pack-order of conversation-turn widgets in `ChatPanel`.  Tests cover: first-render order, full turn sequence (user → classify → think → respond), collapse/expand cycle, and multiple consecutive turns (parametrized: 1, 2, 3 turns).

#### Fixed

- `src/agentx/gui/chat_panel.py` — `_ensure_turn_started()`: moved `children.pack(fill=tk.X, …)` to *after* `_create_output_entry()` so Tkinter packs the user-entry frame before the children frame.  Previously `children` was packed first (index 0 in the slave list), causing all classification/thinking/tool/assistant entries to render *above* the user prompt on first render.  Collapse → expand accidentally "fixed" this because `pack_forget()` + `pack()` appended children to the end of the list.
- `src/agentx/gui/markdown_renderer.py` — `markdown_to_html()`: added guard for `_md_lib is None` so the function produces valid HTML (`<pre>` fallback) even when the `markdown` package is not installed.  Previously any call with `MARKDOWN_AVAILABLE=True` but missing library raised `AttributeError`.
- `tests/test_markdown_rendering.py` — `test_full_path_with_mocked_html_frame`: patched `agentx.gui.chat_panel.markdown_to_html` with a stub that returns `<table>` HTML so the test is self-contained regardless of whether the `markdown` package is installed.
- `tests/test_gui_manager_integration.py` — `test_header_preview_not_driven_by_newline`: removed incorrect assertion that header preview text varies by pixel width; the `_header_preview()` method truncates by word count (>15 words), not pixel width.
- `tests/integration/test_bootstrap_e2e.py` — `TestBootstrapDefaults`: fixed 6 methods that referenced undefined bare variables (`agentx`, `cm`, `candidates`, `prompt_path`, `instructions_path`); replaced with correct `toml_config` fixture access.

#### Changed

- `pyproject.toml`: added pytest markers `unit`, `functional`, `integration` to the `[tool.pytest.ini_options]` markers list.

### Test Changes

#### Added

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_user_entry_packed_before_children_frame_on_first_render`

  ```gherkin
  GIVEN a GUIManager with a chat panel
  WHEN a user message is sent (first render)
  THEN the user entry frame appears before the children frame in Tkinter's pack slave list
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_children_frame_packed_after_user_entry`

  ```gherkin
  GIVEN a conversation turn has been started
  WHEN inspecting pack order of turn_frame children
  THEN user entry pack-index < children frame pack-index
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_classify_entry_appended_to_children_not_turn_frame`

  ```gherkin
  GIVEN a user message has been displayed
  WHEN display_classify is called
  THEN the classification entry is a child of the children_frame, not the turn_frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_assistant_entry_appended_to_children_not_turn_frame`

  ```gherkin
  GIVEN a user message has been displayed
  WHEN display_agent_response is called
  THEN the assistant entry is parented to the children_frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_children_frame_is_indented`

  ```gherkin
  GIVEN a conversation turn
  WHEN inspecting the children_frame's pack configuration
  THEN padx has a non-zero left indent to visually nest responses under the user message
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_full_turn_sequence_pack_order`

  ```gherkin
  GIVEN a GUIManager with a fully laid-out chat panel
  WHEN a user message, classification, thinking, and agent response are all displayed
  THEN user_entry_frame is the first child of turn_frame, children_frame is the second
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_expand_after_collapse_preserves_correct_order`

  ```gherkin
  GIVEN a conversation turn has been rendered correctly
  WHEN the user entry is collapsed then expanded
  THEN the children_frame remains below the user_entry_frame in pack order
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_new_turn_starts_fresh_frame`

  ```gherkin
  GIVEN a first conversation turn is complete
  WHEN a second user message is displayed
  THEN a new turn_frame is created and the children_frame is correct within that new frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestMultipleTurnsRenderingOrder::test_multiple_turns_correct_order[1 turn]`

  ```gherkin
  GIVEN 1 user message and agent response
  WHEN rendered
  THEN user entry is before children frame in the turn's pack slave list
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestMultipleTurnsRenderingOrder::test_multiple_turns_correct_order[2 turns]`

  ```gherkin
  GIVEN 2 consecutive user messages each with an agent response
  WHEN rendered
  THEN every turn has user entry before children frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestMultipleTurnsRenderingOrder::test_multiple_turns_correct_order[3 turns]`

  ```gherkin
  GIVEN 3 consecutive user messages each with an agent response
  WHEN rendered
  THEN every turn has user entry before children frame
  ```

#### Fixed

- **Integration** `test_markdown_rendering.py::TestMarkdownRenderingHeadless::test_full_path_with_mocked_html_frame`

  ```gherkin
  BEFORE: AttributeError on _md_lib.markdown when markdown package is not installed
  AFTER:  markdown_to_html is patched so the test asserts <table> regardless of package availability
  ```

- **Integration** `test_gui_manager_integration.py::TestGUIManagerDisplay::test_header_preview_not_driven_by_newline`

  ```gherkin
  BEFORE: AssertionError — test incorrectly assumed header text varies by pixel width
  AFTER:  Test only asserts that newlines are condensed to spaces; truncation threshold is word-count-based
  ```

- **Integration** `tests/integration/test_bootstrap_e2e.py::TestBootstrapDefaults` (6 methods)

  ```gherkin
  BEFORE: NameError: name 'agentx' / 'cm' / 'candidates' / 'prompt_path' / 'instructions_path' is not defined
  AFTER:  All references replaced with correct toml_config fixture access
  ```

---

## [0.18.23] - 2026-04-19

### Code Changes

#### Fixed

- `pyproject.toml`: version tag format changed from PEP 440-incompatible `-letter` suffix (e.g. `0.18.22-i`) to PEP 440-compliant `.postN` post-release form (e.g. `0.18.22.post9`). The old format caused `uv build` to fail with a TOML parse error.
- `.github/copilot-instructions.md`: Semantic Versioning section updated — doc-only version examples changed from `2.3.1-a` / `2.3.1-b` to `2.3.1.post1` / `2.3.1.post2`. All references to the `-letter` alpha tag scheme replaced with `.postN` language.

---

## [0.18.22.post9] - 2026-04-19

### Code Changes

#### Added

- None

#### Changed

- None

#### Fixed

- None

### Documentation Changes

#### Added

- `docs/architecture.md`: new **§12 Context Construction Pipeline** section documenting all five filter layers applied before any message reaches the LLM API:
  - **Layer 0** — `_build_shared_context()` assembly order (WM injection, history, current session)
  - **Layer 1** — `Message.enabled` flag mechanics, including non-obvious `load_from_dir()` default of `False`, the `MessageEntry.__getattr__` proxy pattern, and per-role enabled defaults
  - **Layer 2** — `to_llm_messages()` internal-role exclusion set (`PLAN`, `TASK_NODE`, `SYNTHESIS`, `ASSERTION`) with per-role LLM-sent table
  - **Layer 3** — `to_llm_dict()` attachment filtering (`attachment.enabled == True`)
  - Full pipeline ASCII diagram showing the complete flow from `_build_shared_context` through wire-format output
  - Classification path note showing `classify_prompt()`'s independent `enabled`-only filter
- `docs/architecture.md`: Contents table updated with §12 entry; old §12–16 renumbered to §13–17

---

## [0.18.22.post8] - 2026-04-19

### Code Changes

#### Added

- None

#### Changed

- None

#### Fixed

- None

### Documentation Changes

#### Added

- `docs/ux/03_PANEL_DETAILS.md`: new **PD-11: FileExplorer** section documenting the Files tab widget — navigation bar (Back, Forward, Up, Home, Refresh), path label, three-column treeview, file/folder context menus (Attach, Edit, Add to memory), navigation history state, and related user flow references.
- `docs/ux/03_PANEL_DETAILS.md`: fully expanded **PD-07: SettingsTab** section — replaces the thin key-sections table with complete per-section widget inventories for all five collapsible groups (Appearance, Ollama, Agentix, Classification Display, Working Memory), ASCII mockup, and widget convention table.
- `docs/ux/02_USER_FLOWS.md`: **UF-11: File Explorer Navigation** sequence diagram covering directory navigation, file attach, file edit, folder pin to Working Memory, and history traversal (Back/Forward).
- `docs/ux/01_MAIN_LAYOUT.md`: Detail Diagram References table — added rows linking **Files Tab → PD-11** and **Settings Tab → PD-07**.
- `docs/ux/01_MAIN_LAYOUT.md`: Component Index — added `[→ PD-11]` and `[→ PD-07]` inline links on the Files tab and Settings tab rows.
- `docs/ux/03_PANEL_DETAILS.md`: PD-03 SidePanel — replaced the inline Files Tab and Settings Tab affordance tables with concise `[→ PD-11]` / `[→ PD-07]` forward-references to avoid duplication.

---

## [0.18.22.post7] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation / workspace config only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Added

- `.vscode/extensions.json` — workspace extension recommendations. Marks `vstirbu.vscode-mermaid-preview` and `mermaidchart.vscode-mermaid-chart` as `unwantedRecommendations` because both register Markdown preview renderers that conflict with `bierner.markdown-mermaid`, producing the double-nested "No diagram type detected" error in the Markdown preview. Only `bierner.markdown-mermaid` should be active in this workspace for Mermaid rendering.

---

## [0.18.22.post6] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10: rewrote self-referential arrow labels to remove leading underscores (`_interrupt_flag`, `_is_streaming`). Mermaid processes label text as Markdown, so `_word_` patterns are interpreted as italic markers, corrupting the token stream and causing "No diagram type detected". Replaced with plain descriptive text: `set interrupt flag = True` and `clear is_streaming event`. Also simplified the Note and partial-response label text.

---

## [0.18.22.post5] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10: replaced em dash `—` with a plain hyphen `-` in the `Note` text; the Unicode em dash caused "No diagram type detected" in the VS Code Mermaid preview renderer.

---

## [0.18.22.post4] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10 Mermaid parse error: added missing `participant Chat as ChatPanel` declaration and replaced `\n`-delimited `Note` text with a single-line string (inline `\n` escapes in `Note` blocks are not supported by all Mermaid versions and caused a NEWLINE parse error).

---

## [0.18.22.post3] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — replaced `actor User` with `participant User` in all 9 Mermaid sequence diagrams (UF-01 through UF-10 excluding UF-09). The `actor` keyword is not supported by the Mermaid version bundled with VS Code's markdown preview, causing "No diagram type detected" errors.

---

## [0.18.22.post2] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Window Layout Mockup**: ChatPanel is now shown on the **left (~66%)** and SidePanel on the **right (~34%)**, matching the actual `PanedWindow` widget order in `chat_panel.py` and `side_panel.py`.
- `docs/ux/01_MAIN_LAYOUT.md` — removed incorrect model-selector widget from the OS title bar in the mockup (it lives inside SidePanel, not the title bar).
- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Zone Map** sash table: left pane is ChatPanel (~66%), right pane is SidePanel (~34%); was previously inverted (25%/75%).
- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Component Index** `Left pane` / `Right pane` labels and percentages.
- `docs/ux/01_MAIN_LAYOUT.md` — corrected §4 SidePanel and §5 ChatPanel position strings to match actual sash proportions.
- `docs/ux/01_MAIN_LAYOUT.md` — added `screen_side` clarification note: this setting controls which side of the **monitor** the window is placed on, not the internal panel arrangement.
- `docs/ux/01_MAIN_LAYOUT.md` — added **Detail Diagram References** table beneath the mockup, providing `[→ PD-XX]` annotations on every generalised component and linking to corresponding sections in `03_PANEL_DETAILS.md` (quality gate: generalised components must carry detail diagram references).
- `docs/ux/03_PANEL_DETAILS.md` — corrected **PD-01 ChatPanel** position from "Right ~75%" to "Left ~66% (PanedWindow left pane)".
- `docs/ux/03_PANEL_DETAILS.md` — corrected **PD-03 SidePanel** position from "Left ~25%" to "Right ~34% (PanedWindow right pane)".

---

## [0.18.22.post1] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Added

- `docs/ux/README.md` — new UX documentation index with message-role icon legend and key UX principles.
- `docs/ux/01_MAIN_LAYOUT.md` — main window layout mockup with zone map, component index, and per-panel layout details (SidePanel, ChatPanel, InputPanel).
- `docs/ux/02_USER_FLOWS.md` — Mermaid sequence and flowchart diagrams for all 10 major user flows: basic chat, tool execution, hierarchical task execution, re-synthesis, file attachment, model switch, settings change, session history navigation, working memory management, and interrupt streaming.
- `docs/ux/03_PANEL_DETAILS.md` — per-panel affordance specifications for ChatPanel, InputPanel, SidePanel, ModelSelector, PlanTreeWidget, ResynthesisDialog, SettingsTab, ContextRenderer, CollapsibleSection, and ToolPanel.

#### Changed

- `docs/architecture.md` — full rewrite to reflect current codebase state (2026-04-19). Updated module map tables, class relationship Mermaid diagram, session decomposition (SessionState/StreamingController/ToolDispatcher), GUI decomposition (ChatPanel/InputPanel/SidePanel/ContextRenderer), classification pipeline (with phi4-mini, response_format fix, system-msg exclusion fix), tool pipeline, hierarchical task execution, working memory section, data model schemas, tool schema examples, threading model, persistence layout, configuration reference, and retrieval keywords index.
- `gui_manager.md` — replaced aspirational design doc with current-state documentation of GUIManager as a thin coordinator, describing the IGUIManager Protocol, panel decomposition, WidgetRegistry pattern, and achieved separation-of-concerns table.
- `docs/integration/01_ARCHITECTURE_OVERVIEW.md` — updated all stale module file paths (e.g. `src/agentx/gui_manager.py` → `src/agentx/gui/gui_manager.py`; `src/agentx/context.py` → `src/shared/models/context.py`). Updated component tables for both AgentX and Agentix subsystems to match current code structure. Updated data flow diagrams.

---

## [0.18.22] - 2026-04-18

### Code Changes

#### Fixed

- `src/agentix/query_payload.py` — emit `response_format: {"type": "json_object"}` (OpenAI-compat key) instead of Ollama-native `format: "json"` so classification calls receive structured JSON from all endpoints.
- `src/agentix/bridge/classify_prompt.py` — filter context to `user`/`assistant` roles only before classification (exclude `system` messages carrying working-memory identity to prevent persona contamination).
- `src/agentix/bridge/classify_prompt.py` — pass `system_prompts_dir` from `AgentixConfig` to `PromptLoader` so the classification system prompt is loaded correctly when invoked from the bridge.
- `agentx.toml` — set `agentix_bench_classification_model = "phi4-mini:3.8b"` (neutral model suitable for JSON classification; replaces agent-persona model).
- `system_prompts/prompt_classification.md` — updated to produce valid JSON output conforming to `PromptClassificationResponse` schema.

### Test Changes

#### Added

- `tests/integration/test_bootstrap_e2e.py` — 10 `@pytest.mark.live` end-to-end bootstrap tests verifying the full classification pipeline against a live Ollama instance.
