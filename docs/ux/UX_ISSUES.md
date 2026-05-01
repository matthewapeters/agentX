# UX ISSUES

This file is the bug-tracking log for user-reported UX defects in AgentX.

## How to Use This File

**User**: Add a new issue entry below the `----` divider.  Use the format:

```
[ ] <Short description of the problem>.  <Observed behaviour>.  <Expected behaviour>.
```

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

6. **Mark complete** — Change `[ ]` to `[/]` once committed.

---

## Issues

----
[/] Right-clicking files in file browser do not cause menu pop-up - so they cannot be attached to the context - either individually or as a group.  Sporadic menus pop up but disappear immediately.  This is a major UX issue.
  - **Root cause**: `<FocusOut>` was bound to `_dismiss_popup_menu()` on the treeview.
    `tk_popup()` steals focus from the tree when it opens, immediately firing `FocusOut`
    and calling `unpost()` before the user sees the menu.
  - **Fix**: Removed the `<FocusOut>` binding from `FileExplorer.to_gui()`.  `<Escape>`
    and clicking elsewhere remain as the natural dismiss paths.
  - **Affordances added**: PD-11-AF-008, PD-11-AF-009, PD-11-AF-010.
  - **Tests**: `tests/test_file_explorer_context_menu.py` (14 unit tests, all pass).
  - **Committed**: v0.22.1
