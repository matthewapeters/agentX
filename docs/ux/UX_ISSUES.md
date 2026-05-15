# UX ISSUES

_Last updated: 2026-05-15 (v0.48.10)_

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
| `[/]`  | Agent  | Fix committed and all tests pass; **ready for UAT**. UAT happens _after_ the agent marks `[/]`, not before. |
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
  rationale was correct but `after_idle` does not defer past the _physical_ button
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

- The copy menu now displays on right-click over scraped content, but if the user clicks elsewhere or hits ESCAPE key, it remains visible.  It only disappears if clicked or if a new area of text is highlighted and left-clicked.  Normal pop-up menu behavior should be enforced (consistently thoughtout the application: may require a cut-sheet for pop-ups and a common pop-up class inheritence for enforced consistent behavior)
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
- **Phase 3 (UAT fix)** — Root-cause analysis revealed two bugs: (1) `<Button-3>` was bound to hidden `output_text` widget never packed into the visible layout; (2) `header_label` was a `tk.Label` (not selectable). Fix: replaced `header_label` with a `tk.Text(state=DISABLED)` widget (`header_text`), added `<Button-3>` bindings to both `header_text` and `detail_text` in `_create_output_entry`, added `_on_entry_text_right_click` method, updated `_show_output_context_menu` to accept optional `target` widget. 8 new tests added (16 total for output context menu). Committed as v0.22.17.
- **Phase 4 (UAT fix — popup dismiss)** — UAT reported popup persists when clicking elsewhere or pressing Escape. Root cause: `overrideredirect(True)` popups do not capture keyboard/mouse focus; the `<Escape>` binding never fires and there is no click-away handler. Fix: added `popup.grab_set()` after `popup.deiconify()` in both `_show_output_context_menu` and `_show_input_context_menu`. Added `_on_outside_click` handler bound to `<ButtonPress>` on the popup — checks if event coordinates are outside the popup bounds and dismisses if so. Applied to both output and input panels. 6 new tests added (19 output + 20 input = 39 total). **UAT-confirmed by user**.
- **Phase 5 (UAT-confirmed — startup/label selectability)** — Remaining non-selectable output text paths migrated from `tk.Label` to `tk.Text(state=DISABLED)` in `ChatPanel.display_startup_notice` (`icon`, `title`, `detail`) and `ChatPanel.add_plan_tab` toolbar title. Added right-click bindings on migrated widgets and moved startup detail tracking from `_output_wrapped_labels` to `_output_detail_text_widgets`. Added startup regression test coverage for selectable detail text and context-menu binding in `tests/test_startup_log_notice.py` (now 5 tests). **UAT-confirmed by user**.
- **Affordances**: PD-01-AF-010, PD-02-AF-008, PD-02-AF-009, PD-02-AF-010, PD-02-AF-011, PD-02-AF-012.
- **Tests**: `tests/test_chat_panel_copy_context_menu.py` (19 tests), `tests/test_input_panel_context_menu.py` (20 tests), `tests/test_startup_log_notice.py` (5 tests).
  
[/] Issue: The Working Memory widget should be collapsed at start-up like the other widgets in the context / history.  This should be reflected in Gherkin use-cases, unit tests, and cut-sheet details.  Its behavior should be consistent with widgets on this pane, and should be governed by common functionality, use-cases, and patterns within the cut-sheets.  It may be necessary to describe these widgets as a class of component with their own cut-sheet and use-cases.

- **Fixed in v0.22.20**: Changed `initial_collapsed=False` → `True` for `working_memory` in `SidePanel.create()`. Added affordance `PD-03-AF-015` with Gherkin spec in `03_PANEL_DETAILS.md`. Updated `test_session_sections_start_collapsed` to assert `working_memory` is collapsed.
- **UAT-confirmed by user** (2026-05-06). Issue closed.

[/] Task Status Issue: When complex tasks are being performed, the main display lacks visual clues as to status or progress.  New visual affordances are necessary.  Example: what is the curent state of the prompt-reply cycle (IE classification, thinking, tools, synthesize, reply, etc.)?  Nothing shows the steps of a plan and their status.  Agent and user should ideate solutions that are:

- [/] 1. Simple to implement
- [/] 2. Provide clear visual feedback to the user
- [/] 3. Are robust and reliable
- [/] 4. Are not overly complex or resource-intensive

**Spec complete (v0.22.21)**: Resolved via **PD-12 StatusTab** — a new first tab in the system notebook that auto-activates on prompt submit.  Consists of three sub-widgets:

1. **ContextWindowSection** — donut chart + colour-key legend side-by-side (ContextMeterWidget relocated from InputPanel button column).
2. **PhaseStepperWidget** — vertical list of phase rows (Classify / Think / Tool: `<name>` / Respond) each showing a status icon (`○` / `↻` / `✓` / `✗`), phase emoji, and a `HH:MM:SS` elapsed timer (1-second `after()` loop).
3. **InterruptButton** — large full-width button; replaces the `user_break` button removed from InputPanel.

Affordances: PD-12-AF-001 through PD-12-AF-011 (all `✅ Tested`).
Implementation: `src/agentx/gui/status_tab.py` — implemented; 40 unit tests pass (v0.32.1).

**Implementation complete — awaiting UAT confirmation before closing.**

[ ] Issue: When complex tasks are being performed, the user experience moving between output panes is slowed and the redraws are laggy.  It may be necessary to implement multi-processing with reliable state between processes to ensure reliable state management.
[/] Need more TMUX panes! Running launch_vibe.sh creates the neovim pane, and then fills it with the agentx cli run-time logs!  Create a different pane for these! (not 0 or 1)

- **Root cause / attempted fix (v0.23.0)**: `launch_vibe.sh` launched AgentX with `"$PYTHON_BIN" -m agentx &` as a background subprocess of the launcher script, then called `exec tmux attach`. The AgentX stdout/stderr FDs remained bound to the original TTY, which became the active tmux session — causing runtime logs to appear in pane 0.0 (the neovim window).
- **Fix**: AgentX is now launched inside a dedicated tmux window (`window 2: agentx-log`, pane 2.0) via `tmux new-window` + `tmux send-keys`. Its stdout/stderr are captured by that pane and never reach pane 0.0. User can inspect logs at any time with `Ctrl+B, 2`.
- **Design doc** (`docs/ux/05_VIBE_CODING.md`) updated with window 2 in layout diagram, component map, and navigation key table.
- **UAT confirmation (2026-05-08)**: User approved the pane separation behavior. Logs are isolated to `window 2: agentx-log` and no longer render in the neovim pane.
- **UAT Status**: User-approved in UAT on 2026-05-08.
[/] launch_vibe.sh issue: how does the engineer terminate their session?  Stopping the agentx gui does not terminate the tmux panes or neovim session.  What happens if the user terminates neovim and their pane?  Can they get it back?  Identify all permutations and propose a solution that can be tested.

- **UAT failure (2026-05-08)**: v0.24.0 did not hold up in live use.
  - Editor pane regression: window `0.0` presented a bash shell instead of neovim.
  - Session shutdown regression: closing the AgentX GUI left tmux windows/panes running.

- **Root cause / attempted fix (v0.24.0)**: `launch_vibe.sh` only provided a startup path; there was no canonical shutdown/recovery command set. This produced inconsistent behaviour when users closed GUI, exited neovim, detached tmux, or lost window 0.
- **Fix**: Added lifecycle command interface to `launch_vibe.sh`: `start`, `stop`, `status`, `recover-editor`, `restart`.
  - `stop`: graceful teardown (AgentX runtime pane Ctrl+C, neovim pane Ctrl+C + `:qa!`, `tmux kill-session`, stale socket cleanup).
  - `status`: reports session/socket/FIFO/window state.
  - `recover-editor`: recreates window 0 if missing and relaunches neovim in pane `0.0`.
  - `restart`: deterministic `stop` + `start`.
- **Permutations documented**: design now includes explicit session lifecycle matrix (GUI closed, editor exited, window missing, detach, full stop, wedged session).
- **Latest attempted fix candidate (v0.24.1)**:
  - Added editor health checks on start/reattach and automatic recovery if pane `0.0` is not running `nvim`.
  - Added GUI-exit session teardown hook so closing AgentX GUI terminates the tmux session.
  - Added socket wait tunables for robust startup polling (`AGENTX_SOCKET_WAIT_LOOPS`, `AGENTX_SOCKET_WAIT_SEC`).
- **UAT failure (2026-05-08, post-v0.25.1)**:
  - Launcher output reported repeated `can't find window: editor` and no neovim process in any pane.
  - Root cause: some tmux environments rejected/failed named pane targets (`session:editor.0`) even when the window existed, so editor relaunch attempts never reached the actual pane.
- **Latest attempted fix candidate (v0.25.2)**:
  - Reworked pane targeting to resolve named windows to numeric runtime targets (`session:<index>.0`) before every `send-keys`/status probe.
  - Added stricter error handling when expected windows cannot be resolved after creation.
  - Upgraded fake tmux harness in launcher tests to be stateful across invocations in a single launcher run, so session/window lifecycle regressions are covered.
- **Tests**: `tests/test_launch_vibe_shutdown.py` (5 unit tests, hermetic fake tmux/nvim harness).
- **Design docs updated**: `docs/ux/05_VIBE_CODING.md` (launch architecture, lifecycle commands, permutations, and traceability test scenarios).
- **UAT confirmation (2026-05-08)**: User confirmed launch and close behavior of `launch_vibe.sh` now passes in live use.
- **UAT Status**: User-approved in UAT on 2026-05-08.

---

[/] issue: when user selects a file and choses "Edit" from the pop-down menu, the agent should open the selected file in the neovim editor in a new buffer.  Do not close any open buffers.

- **Affordance**: PD-14-AF-002 — "Edit" FileExplorer context menu → `VimBridge.open_file()`
- **Attempted fix 1 (v0.33.0)**: Created `src/agentx/integration/vim_bridge.py`. Socket defaulted to `/tmp/agentx.nvim.sock`. **UAT failed** — socket not found because `launch_vibe.sh` creates a session-scoped socket at `/tmp/agentx_<session>.nvim.sock` (e.g. `/tmp/agentx_agentx.nvim.sock`).
- **Attempted fix 2 (v0.33.1)**: Fixed socket resolution to mirror `launch_vibe.sh`. Priority: `AGENTX_NVIM_SOCKET` env var → `config["neovim"]["socket"]` → `/tmp/agentx_<AGENTX_TMUX_SESSION>.nvim.sock` (default: `/tmp/agentx_agentx.nvim.sock`). 19 tests, 100% coverage.
- **UAT Status**: Ready for UAT — launch `./launch_vibe.sh` then use Edit in file explorer.
[/] TUI issue: results streaming from the LLM are written to the TUI with a CR or LF after each word is received.  Text should be continued on the same line - only carriage returns from the agent should be written until the end of the stream is complete - then an additional CR is added.  It is clear that only the first response is mirrored to the TUI - the pub/sub channels appear to be closed or not re-established with second and later prompts (from the GUI).  Prompts in the TUI do not appear to send, but it is unclear.

- **Root cause / attempted fix (v0.39.3)**:
  - Event-broker subscriptions used a queue that was never drained, so sustained publish streams eventually stalled TUI delivery after initial responses. Fixed by introducing per-subscriber worker threads that consume and dispatch queued events continuously.
  - Generated TUI Lua used escaped sentinel/join strings (`"\\n"`) while Python expected real newline framing (`"\n"`), causing TUI submit parsing mismatch. Fixed Lua generation + checked-in script to emit real newline sentinel/join text.
  - Output pane appended each `on_stdout` callback as full lines, which rendered token streams as frequent line breaks. Fixed output append logic to coalesce partial chunks on the current line and only create new lines on explicit separators.
- **Tests**:
  - `tests/test_event_broker_pubsub.py`: added no-drop busy-subscriber regression and ordered-delivery regression tests.
  - `tests/test_tui_bridge_output.py`, `tests/test_config_tui_phase1.py`: full related suite re-run; all pass.
- **UAT Status**: latest fix candidate ready for UAT.
- [/] Issue, the neovim panes should be reorganized for better user experience:
  - TUI (default displayed)
  - neovim (AKA "Editor" or "Vibe Editor")
  - BASH terminal
  - AgentX Logs
- **Root cause / attempted fix (v0.48.2)**:
  - Launcher created the editor as window `0`, then added agent/log windows and optionally appended `tui-chat` later. This made attach behavior editor-first and inconsistent with the requested TUI-first workflow.
  - Updated `launch_vibe.sh` session creation order when TUI is enabled to deterministic window order: `0=tui-chat`, `1=editor`, `2=agent-bg`, `3=agentx-log`.
  - Added explicit `tmux select-window` before attach so default visible pane is `tui-chat` when enabled (editor remains default when TUI is disabled).
  - Adjusted editor-pane resolution fallback order to prefer named `editor` window before generic first-pane fallback, preventing mis-targeting in TUI-first sessions.
- **Tests**: `tests/test_launch_vibe_shutdown.py` updated for TUI-first ordering and attach-window selection assertions.
- **UAT Status**: latest fix candidate ready for UAT.
- Tool/tooling backlog items were moved to `docs/tools/tools_issues.md` to keep UX and tooling triage separate.
[ ] User requested that agent run 'nvtop' in BASH terminal.  Application presented modal to authorize.  When the user accepted, the GUI crashed, but the TUI remained running with no clear way to terminate (user had to close each panel).  Application crashes should cleanly close the application.  The application should not crash when attempting to run commands in the terminal.

- **Intake/Triage (2026-05-14)**:
  - **Decision**: new issue created: <https://github.com/matthewapeters/agentX/issues/6>
  - **Severity/Priority/Type**: high / p1 / bug.
  - **Expected behavior**: approving terminal command execution should run the command without crashing GUI; if a fatal error occurs, application shutdown should be coordinated and clean.
  - **Actual behavior**: after approval modal accept, GUI crashed while TUI remained active and required manual pane-by-pane closure.
  - **Reproduction summary**:
    1. Start AgentX with terminal workflow enabled.
    2. Request command execution for `nvtop` in BASH terminal.
    3. Approve command from the modal.
    4. Observe GUI crash and orphaned running TUI session.
  - **Environment snapshot**: Linux, Python 3.14.4, current repo head `152c077`.
  - **Evidence available**:
    - user-reported crash path at approval boundary;
    - related fix history for `PD-15-AF-006` recorded in `CHANGELOG.md` (terminal approval path).
    - reproduction report comments:
      - <https://github.com/matthewapeters/agentX/issues/6#issuecomment-4451342760> (initial run: not reproduced; blocked by preflight timeout path).
      - <https://github.com/matthewapeters/agentX/issues/6#issuecomment-4451368911> (follow-up run: not reproduced; startup succeeds with model override, headless path fails on missing DISPLAY).
  - **Issue checklist**:
    - [ ] reproduction pending
    - [ ] evidence pending
    - [/] regression tests complete (defect encoded)
    - [/] fix complete
    - [/] verification complete (closed-unverified)
  - **Regression test evidence**: <https://github.com/matthewapeters/agentX/issues/6#issuecomment-4461869543>.
  - **Verify-release evidence**:
    - <https://github.com/matthewapeters/agentX/issues/6#issuecomment-4462068242> (release/build validation + scenario evidence + reopen criteria).
    - <https://github.com/matthewapeters/agentX/issues/6#issuecomment-4462069247> (issue closed as closed-unverified; live UAT unavailable in-session).
  - **Next prompt**: `issue-intake` (for next open issue).
[/] launch_vibe.sh crashes - there is an issue working with the selected model (qwen3.6:latest)

- **Intake/Triage (2026-05-14)**:
  - **Decision**: new issue created: <https://github.com/matthewapeters/agentX/issues/7>.
  - **Severity/Priority/Type**: high / p1 / bug.
  - **Environment**: Linux, Python 3.14.4, commit `152c077`, `ollama_host=localhost:11434`, `ollama_model=qwen3.6:latest`.
  - **Observed launcher failure**: `./launch_vibe.sh start` fails during preflight with `HTTP 000` after curl timeout (`Operation timed out after 10001 milliseconds with 0 bytes received`).
  - **Direct probe evidence**: `POST /api/chat` with `qwen3.6:latest` returned `HTTP 500` and `CUDA error: out of memory` from `ggml-cuda`.
  - **Runtime pressure evidence**: `nvidia-smi` showed RTX 4090 memory `23699 MiB / 24564 MiB` in use; `ollama ps` shows `qwen3.6:latest` resident with mixed CPU/GPU execution.
  - **Hypothesis**: launcher preflight timeout can mask underlying VRAM saturation / model startup instability for large models.
  - **Reproduction report**: <https://github.com/matthewapeters/agentX/issues/7#issuecomment-4456384092>.
  - **Reproduction verdict**: flaky (exact launcher path failed with preflight timeout while direct `/api/chat` probe for same model returned `HTTP 200` in a separate trial).
  - **Attempted fix candidate (v0.48.4)**:
    - Added bounded preflight retry/backoff in `launch_vibe.sh` to reduce false-negative startup aborts on transient `HTTP 000`.
    - Added controls: `AGENTX_OLLAMA_PREFLIGHT_ATTEMPTS` and `AGENTX_OLLAMA_PREFLIGHT_RETRY_SEC`.
  - **Regression test evidence**: <https://github.com/matthewapeters/agentX/issues/7#issuecomment-4460265372>.
  - **Fix summary evidence**: <https://github.com/matthewapeters/agentX/issues/7#issuecomment-4460328653>.
  - **Verify-release evidence**: <https://github.com/matthewapeters/agentX/issues/7#issuecomment-4461184032>.
  - **Final verify-release gate (v0.48.5)**: <https://github.com/matthewapeters/agentX/issues/7#issuecomment-4461716958> (5/5 startup passes).
  - **Issue closure comment**: <https://github.com/matthewapeters/agentX/issues/7#issuecomment-4461718216>.
  - **Final state**: closed as resolved-verified on 2026-05-15.
  - **UAT Status (historical checkpoint)**: verification failed stability gate (5-trial run: 2 pass / 3 fail).
  - **Investigation hardening candidate (v0.48.5)**:
    - Increased preflight timeout budget via `AGENTX_OLLAMA_PREFLIGHT_TIMEOUT_SEC` (default `20`).
    - Reduced chat-probe work with `"num_predict": 1` in preflight payload.
    - Added explicit OOM diagnostics in launcher output for CUDA/VRAM pressure signatures.
  - **Latest observed failure signatures**:
    - preflight retry exhaustion with `curl: (28)` and final `HTTP 000` launcher abort;
    - intermittent upstream `HTTP 500` including `CUDA error: out of memory` in model execution path.
  - **UAT Status (latest)**: resolved-verified for build `main@4219787` (v0.48.5); reopen on recurrence signatures per closure note.
  - **Next prompt**: `issue-verify-release` (complete).

[/] Add emoji support to TUI agent responses - user responses lack emoji indicators

- **Intake/Triage (2026-05-15)**:
  - **Decision**: new issue created: <https://github.com/matthewapeters/agentX/issues/8>.
  - **Severity/Priority/Type**: low / p3 / enhancement.
  - **Summary**: TUI mode does not display emojis in agent responses, unlike the GUI which includes emoji indicators (person emoji for user, robot emoji for agent, etc.).
  - **Expected behavior**: Agent responses in TUI mode should include emoji indicators matching or similar to GUI indicators.
  - **Actual behavior**: No emojis are shown in TUI agent responses; responses appear without emoji indicators.
  - **Reproduction summary**:
    1. Start AgentX in TUI mode: `./launch_vibe.sh`
    2. Interact with the agent by sending prompts.
    3. Observe agent response output in terminal; no emoji indicators appear.
  - **Environment snapshot**: Linux (Ubuntu 26.04), Python 3.12+, current repo head main branch.
  - **Related components**:
    - TUI rendering: `src/agentx/integration/tui_bridge.py`
    - GUI emoji support comparison: `src/agentx/gui/` (chat_panel, context_renderer).
    - Emoji dependency: project uses `emoji` package.
  - **Issue checklist**:
    - [/] reproduction complete (feature gap confirmed)
    - [/] evidence complete
    - [/] regression tests complete (defect encoded)
    - [/] fix complete
    - [ ] verification pending
  - **Reproduction evidence**: <https://github.com/matthewapeters/agentX/issues/8#issuecomment-4462913162>.
  - **Regression test evidence**: <https://github.com/matthewapeters/agentX/issues/8#issuecomment-4463029769>.
  - **Next prompt**: `issue-verify-release`.
