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
[ ] Right-clicking files in file browser do not cause menu pop-up - so they cannot be attached to the context - either individually or as a group.  Sporadic menus pop up but disappear immediately.  This is a major UX issue.  Although attempted to be fixed, UAT shows the issue ramains.  The pop-up shows up about once for every 12 clicks - it should be 100% reliable.  (count of attempted fixes: 5)

- **Root cause 1–4 (v0.22.1–0.22.4)**: See history below — all were workarounds to
  `tk_popup()`'s internal `grab` command.  The WM kept cancelling Tk's grab.
- **Root cause 5 / structural fix (v0.22.5)**: All previous fixes treated symptoms of
  `tk_popup()`.  `tk_popup()` unconditionally calls Tcl's `grab` command, which on any
  modern Linux compositor (GNOME/Mutter, KWin, Wayland/XWayland) conflicts with the WM's
  own pointer grab — the WM cancels Tk's grab and the menu is immediately dismissed.
  There is no reliable way to keep the grab after `tk_popup()` on these compositors.
  Fix: replaced `tk_popup()` with `menu.post()`.  `post()` positions and displays the
  menu without setting any grab.  Tk's native root-window `<ButtonPress>` binding handles
  auto-dismiss when the user clicks outside; `<Escape>` is bound explicitly.
- **Test limitation note**: Unit tests verify that `menu.post()` is called and
  `tk_popup()` is not.  Whether the menu stays visible under a live compositor cannot
  be detected by headless unit tests — this requires manual UAT.
- **History of prior root causes**:
  - RC1 (v0.22.1): `<FocusOut>` bound to `_dismiss_popup_menu()` — removed.
  - RC2 (v0.22.2): `<Button-3>` press binding — changed to `<ButtonRelease-3>`.
  - RC3 (v0.22.3): Synchronous `grab_release()` after `tk_popup()` — added.
  - RC4 (v0.22.4): `grab_release()` fired before event queue drained — moved to
    `after_idle`; added `return "break"`.
- **Side-effect of RC2**: `<Button-3>` could accidentally trigger "Attach" via the WM's
  ButtonRelease landing on menu-item coordinates — fixed in v0.22.2.
- **Affordances**: PD-11-AF-008, PD-11-AF-009, PD-11-AF-010.
- **Tests**: `tests/test_file_explorer_context_menu.py` (15 unit tests, all pass).
- **Committed**: v0.22.5
