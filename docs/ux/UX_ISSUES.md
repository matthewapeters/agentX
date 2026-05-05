# UX ISSUES

This file is the bug-tracking log for user-reported UX defects in AgentX.

## How to Use This File

**User**: Add a new issue entry below the `----` divider.  Use the format:

```
[ ] <Short description of the problem>.  <Observed behaviour>.  <Expected behaviour>.
```

**Semaphore semantics** — the status markers are a handshake between agent and user:

| Marker | Set by | Meaning |
|--------|--------|---------|
| `[ ]`  | User   | Issue reported; agent has not yet applied a fix. |
| `[/]`  | Agent  | Fix committed and all tests pass; **ready for UAT**. UAT happens *after* the agent marks `[/]`, not before. |
| `[X]`  | Either | Fix attempted but failed or blocked; needs follow-up. |

When the user performs UAT and the fix is confirmed: leave `[/]` in place (it means resolved).
When the user performs UAT and the fix still fails: change `[/]` back to `[ ]` and note the failure count.

**Agent**: For each `[ ]` entry:

1. **Locate the affordance** — Open [docs/ux/00_INDEX.md](00_INDEX.md) and find
   the relevant panel.  Then look up the Affordance ID in the
   [Traceability Matrix](UX_LIFECYCLE.md#4-traceability-matrix-as-built) (§4).

2. **Check the spec** — Read the affordance cut-sheet in
   [docs/ux/03_PANEL_DETAILS.md](03_PANEL_DETAILS.md).  If the component is not yet
   documented, follow the **Add a new affordance** checklist in
   [UX_LIFECYCLE.md §5.1](UX_LIFECYCLE.md#51-adding-a-new-affordance) to spec it first
   (Gherkin use-cases required).

3. **Diagnose the defect** — Identify whether the bug is a code defect, a missing or
   wrong Gherkin expectation, or a spec/code/test drift.  Use the drift-detection
   commands in [UX_LIFECYCLE.md §5.4](UX_LIFECYCLE.md#54-detecting-drift-without-a-planned-change)
   if needed.

4. **Fix and test** — Apply the fix, then write or update hermetic unit tests.  All
   tests must carry the Affordance ID in their docstring.  Follow the
   [Headless Tkinter Testing Primer](UX_LIFECYCLE.md#6-headless-tkinter-testing-primer)
   (§6) for GUI test patterns.

5. **Reconcile the matrix** — Update the `Status` column in the Traceability Matrix
   and any affected rows in [00_INDEX.md](00_INDEX.md) and
   [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md).

6. **Mark complete** — Change `[ ]` to `[/]` once committed.  This signals the user
   that the fix is ready for UAT.

---

## Issues

----
[/] Right-clicking files in file browser do not cause menu pop-up - so they cannot be attached to the context - either individually or as a group.  Sporadic menus pop up but disappear immediately.  This is a major UX issue.  Although attempted to be fixed, UAT shows the issue ramains.  The pop-up shows up about once for every 12 clicks - it should be 100% reliable.  (count of attempted fixes: 14 — latest attempted fix in v0.22.13)

- **UAT confirmation (2026-05-02)**: User approved the fixes for this issue,
  including reliability and minor pre-render palette behavior.

- **Root cause 14 / attempted fix (v0.22.13)**: UAT confirms popup reliability,
  but a minor light-colored pre-render flash remains before the dark themed popup
  buttons appear. Cause: fallback popup `tk.Toplevel` was created without an
  explicit themed background, so compositor briefly displayed default light window
  styling. Fix: apply `panel_bg` directly to the `Toplevel` at creation
  (`popup.configure(bg=panel_bg, borderwidth=0, highlightthickness=0)`) so the
  first visible frame uses the selected palette.

- **Root cause 13 / attempted fix (v0.22.12)**: Wayland fallback popup reused one
  `overrideredirect` `tk.Toplevel` via repeated `withdraw()/deiconify()`. UAT still
  showed alternating invisible/visible popups where clicks near expected popup
  coordinates triggered actions without any rendered menu, indicating stale
  compositor surface state. Fix: force dismissal before scheduling each right-click
  popup and recreate the fallback `Toplevel` for every show
  (`_destroy_wayland_popup()` + fresh `_ensure_wayland_popup()`), so each invocation
  gets a new mapped surface.

- **Root cause 12 / attempted fix (v0.22.11)**: Wayland fallback popup displayed once,
  then immediately self-dismissed on subsequent attempts due to the popup's own
  `<FocusOut>` binding firing as soon as focus changed.  This made later right-clicks
  appear as "no popup" despite `_show_wayland_popup(... mapped=1)` diagnostics.
  Fix: removed popup `<FocusOut>` auto-dismiss and removed forced focus capture;
  popup now dismisses via explicit actions/Escape/dismiss handler only.
  Also precomputes requested popup geometry (`update_idletasks()` + fixed width/height)
  before map to reduce transient oversized flash artifacts observed in UAT.

- **Root cause 11 / attempted fix (v0.22.10)**: Even with `tk_popup()`, UAT showed
  no visible menus while diagnostics reported `ismapped=1, viewable=1`.  This implies
  compositor-level rendering/stacking behavior for Tk menu windows on Wayland/XWayland.
  Added a Wayland-specific fallback path that bypasses Tk menu windows entirely:
  `_show_wayland_popup()` now uses an in-app `tk.Toplevel(overrideredirect=True)`
  popup with explicit buttons for file/folder actions.  Non-Wayland sessions continue
  using native Tk menu behavior.  This is the latest fix candidate and requires UAT.

- **Root cause 10 / attempted fix (v0.22.9)**: Menu popup windows remained
  `ismapped=1` / `viewable=1` in diagnostics but still did not appear in live UAT.
  Updated popup primitive from `menu.post()` to `menu.tk_popup()` (with a guarded
  `grab_release()` in `finally`) while keeping delayed scheduling and generation-aware
  verification.  Rationale: `tk_popup()` uses Tk's native popup semantics and is more
  reliable for compositor stacking/focus behavior on some Wayland/XWayland sessions.
  This is a **latest fix candidate** and requires user UAT confirmation.

- **Root cause 9 / attempted fix (v0.22.8)**: `after_idle` fires **before the button
  is physically released**.  The menu posts while Button-3 is still held; the subsequent
  `<ButtonRelease-3>` lands on the newly-posted menu window; `tk::MenuInvoke` finds no
  active item and calls `unpost()` — menu disappears before the user sees it.  All four
  UAT right-clicks in v0.22.7 exhibited `ismapped=1` immediately after post and then
  vanished.  Fix: replace `after_idle` with `after(100)` so the button is guaranteed
  to have been released before the menu posts (100 ms > any physical click duration).
  The release fires on the treeview (no binding → no action) and the menu stays open.
  `_MENU_POST_DELAY_MS = 100` class attribute allows tests to set the delay to `0`
  without real wall-clock latency.  **This pattern applies to all future right-click
  context menus in this codebase** — see
  [UX_LIFECYCLE.md §6](UX_LIFECYCLE.md#why-after_idle-is-insufficient-and-after100-is-required).
- **Root cause 8 / partial fix (v0.22.7)**: XWayland virtual-screen coordinate mismatch
  (`event.x_root/y_root` in physical X11 pixels → replaced with `winfo_rootx/y() +
  event.x/y`).  Menus posted at correct on-screen coordinates after this fix, but still
  dismissed instantly by button-release (RC9 above).
- **Root cause 7 (v0.22.7)**: `after_idle` added to defer past `<ButtonRelease-3>` —
  rationale was correct but `after_idle` does not defer past the *physical* button
  release when binding is `<Button-3>` (press).  Superseded by `after(100)` in RC9.
- **Root cause 6 (v0.22.6)**: `<ButtonRelease-3>` binding + `menu.post()` → changed
  to `<Button-3>`.  Correct change; retained.
- **Root cause 5 (v0.22.5)**: Replaced `tk_popup()` with `menu.post()`.
- **Root cause 1–4 (v0.22.1–0.22.4)**: See history below.
- **History of all root causes**:
  - RC1 (v0.22.1): `<FocusOut>` bound to `_dismiss_popup_menu()` — removed.
  - RC2 (v0.22.2): `<Button-3>` press → changed to `<ButtonRelease-3>`.
  - RC3 (v0.22.3): Synchronous `grab_release()` after `tk_popup()`.
  - RC4 (v0.22.4): `grab_release()` before queue drained → moved to `after_idle`.
  - RC5 (v0.22.5): `tk_popup()` itself → replaced with `menu.post()`.
  - RC6 (v0.22.6): `<ButtonRelease-3>` binding → changed to `<Button-3>`.
  - RC7 (v0.22.7): `after_idle` added (correct intent, wrong primitive).
  - RC8 (v0.22.7): XWayland coordinates → `winfo_rootx/y() + event.x/y`.
  - RC9 (v0.22.8): `after_idle` → `after(100)` to survive button-release race.
  - RC10 (v0.22.9): switched popup primitive to `tk_popup()` + safe `grab_release()`.
  - RC11 (v0.22.10): Wayland fallback to in-app `tk.Toplevel` popup.
  - RC12 (v0.22.11): removed Wayland popup FocusOut auto-dismiss + stabilized geometry.
  - RC13 (v0.22.12): recreate Wayland popup per show + pre-dismiss stale popup state.
  - RC14 (v0.22.13): theme popup Toplevel background to reduce light pre-render flicker.
- **Affordances**: PD-11-AF-008, PD-11-AF-009, PD-11-AF-010.
- **Tests**: `tests/test_file_explorer_context_menu.py` (19 unit tests);
  `tests/test_file_explorer_menu_coordinates.py` (7 functional tests — all pass).
- **Committed**: v0.22.13
- **UAT Status**: User-approved in UAT on 2026-05-02.

[/] Log files appear empty. Startup output should show where log files are written before the agent's first response so users can verify the expected locations. (latest attempted fix in v0.22.15)

- **UAT confirmation (2026-05-02)**: User approved the startup notice behavior,
  including icon visibility and readability updates.

- **UAT nit / attempted fix (v0.22.15)**: Startup notice icon refined per user
  preference to use `ⓘ` and increase visibility by rendering it bold and slightly
  larger than notice body text.

- **Root cause / attempted fix (v0.22.14)**: No startup affordance informed users where
  logs are written, so expected locations appeared ambiguous or empty. Added a
  startup output notice that lists session/runtime log locations and clarifies that
  some files appear only after first write. The notice is controlled by a new
  `agentx.show_log_locations_on_startup` boolean setting in `agentx.toml`
  (default: `true`).

- **Affordance**: PD-01-AF-009.
- **Tests**: `tests/test_startup_log_notice.py` (3 unit tests — all pass).
- **Committed**: v0.22.15
- **UAT Status**: User-approved in UAT on 2026-05-02.
  
[/] Issue: output pane text cannot be screen-scraped or coppied - strategies and behavior tests needed.

- The user can scrape content in the Output panel, ctrl-C copies to pasteboard, and ctrl-v to the user-input panel successfully pastes the content. ✅ (already implemented)
- **Right-click to copy from Output panel** — right-clicking highlighted text in the Output panel should produce a popup showing "Copy"; selecting "Copy" adds the highlighted content to the clipboard. Affordance: **PD-01-AF-010**.
- **Right-click context menu on User Input panel** — right-clicking in the user input widget should display a popup with:
  - "Copy" (visible only when text is selected) — copies selected text to clipboard. Affordance: **PD-02-AF-009 / PD-02-AF-011**.
  - "Paste" (visible only when clipboard is non-empty) — replaces selected text with clipboard content, or inserts at cursor if nothing is selected. Affordance: **PD-02-AF-010 / PD-02-AF-012**.
- All popups use the Wayland-safe `tk.Toplevel(overrideredirect=True)` pattern established by the FileExplorer context menu.
- Paste reliability: explicit `delete(SEL_FIRST, SEL_LAST)` + `mark_set(INSERT, sel_start)` before insert ensures correct cursor placement; verified hermetically in unit tests.
- Clipboard emptiness check uses `try/except tk.TclError` guard around `clipboard_get()`.
- **Phase 1 (documentation)** — Complete as of v0.22.15.post2.
- **Phase 2 (implementation)** — Code + 25 hermetic unit tests implemented and passing. Complete as of v0.22.16.
- **Phase 3 (UAT fix)** — Root-cause analysis revealed two bugs: (1) `<Button-3>` was bound to hidden `output_text` widget never packed into the visible layout; (2) `header_label` was a `tk.Label` (not selectable). Fix: replaced `header_label` with a `tk.Text(state=DISABLED)` widget (`header_text`), added `<Button-3>` bindings to both `header_text` and `detail_text` in `_create_output_entry`, added `_on_entry_text_right_click` method, updated `_show_output_context_menu` to accept optional `target` widget. 8 new tests added (16 total for output context menu). Latest fix candidate as of v0.22.17 — ready for UAT.
- **Affordances**: PD-01-AF-010, PD-02-AF-008, PD-02-AF-009, PD-02-AF-010, PD-02-AF-011, PD-02-AF-012.
- **Tests**: `tests/test_chat_panel_copy_context_menu.py` (16 tests), `tests/test_input_panel_context_menu.py` (17 tests).

[ ] Issue: The Working Memory widget should be collapsed at start-up like the other widgets in the context / history.  This should be reflected in Gherkin use-cases, unit tests, and cut-sheet details.

[ ] Issue: When complex tasks are being performed, the main display lacks visual clues as to status or progress.  New visual affordances are necessary.

[ ] Issue: When complex tasks are being performed, the user experience moving between output panes is slowed and the redraws are laggy.  It may be necessary to implement multi-processing with reliable state between processes to ensure reliable state management.
