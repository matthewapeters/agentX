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
[/] Right-clicking files in file browser do not cause menu pop-up - so they cannot be attached to the context - either individually or as a group.  Sporadic menus pop up but disappear immediately.  This is a major UX issue.  Although attempted to be fixed, UAT shows the issue ramains.  The pop-up shows up about once for every 12 clicks - it should be 100% reliable.  (count of attempted fixes: 10 — latest attempted fix in v0.22.9; pending user UAT confirmation)

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
- **Affordances**: PD-11-AF-008, PD-11-AF-009, PD-11-AF-010.
- **Tests**: `tests/test_file_explorer_context_menu.py` (17 unit tests);
  `tests/test_file_explorer_menu_coordinates.py` (7 functional tests — all pass).
- **Committed**: v0.22.9
- **UAT Status**: User-owned verification required; agent does not claim definitive UX resolution.

[ ] Log files appear empty.  Startup output should show the path to the log files.- **Fix**: `log.py` → `logger.info()` → `logger.debug()`.
