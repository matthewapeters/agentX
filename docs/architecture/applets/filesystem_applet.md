# File System Applet Contract

Last updated: 2026-06-03
Applet ID: `filesystem`
Runtime entry: `agentx-core --filesystem-widget` (implemented)

## Purpose

Define the first-class File System runtime applet contract and its UX
traceability. This document is authoritative for filesystem applet behavior,
state ownership, and render semantics implemented in
`cmd/agentx-core/filesystem_widget.go`.

## UX Anchors

- `PD-18-AF-004` file-selection applet renders project file navigation (`docs/ux/03_PANEL_DETAILS.md`)
- `PD-11` FileExplorer behavior references (`docs/ux/03_PANEL_DETAILS.md`)
- System panel parity flows (`docs/ux/07_DEMO_MODE.md`):
  - `PD-17-AF-013` Flow C
  - `PD-17-AF-014` Flow C.1

## Owned Widget/Surface Inventory

- Dedicated filesystem TUI widget process with adaptive viewport rendering.
- Directory listing surface with persistent selection state and deterministic
  visible-range status (`showing X-Y of Z`).
- File actions integrated with core submit path (`:context-add`) and editor
  launch path (tmux `new-window`).

## Affordance List

- `PD-18-AF-004`: files surface renders project navigation/selection summary for
  system parity review.
- `PD-11` alignment target: navigation and file/directory action behavior stays
  consistent with FileExplorer intent.
- `PD-11-AF-008..010` alignment target: right-click file/folder action model and
  popup dismissal semantics remain the UX reference.
- `UF-11`: file explorer navigation and attach/edit/folder-memory flows.
- `UF-12`: context popup rendering visibility and palette-first invariants.

## Visual Style Contract (Authoritative)

The filesystem applet render style is intentionally semantic and file-type aware.

- Kind markers use emoji, not `[F|D]` markers:
  - parent directory: `⤴`
  - directory: `📁`
  - file: `📄`
  - missing/unresolvable entry: `❓`
- Directory names use reverse-video ANSI styling (`\033[7m`) for fast visual
  differentiation from files.
- Parent directory row (`..`) uses dedicated background + foreground styling
  (`\033[48;5;238m\033[97m`) for strong navigation affordance.
- File color classes:
  - hidden files (`.` prefix): magenta
  - config files (`.ini`, `.toml`, `.yaml`, `.yml`, `.json`, `.xml`, `.conf`,
    `.cfg`): yellow
  - Go files: cyan
  - Python files: blue
  - JavaScript/TypeScript family (`.js`, `.jsx`, `.mjs`, `.cjs`, `.ts`, `.tsx`): yellow
  - C/C++ family (`.c`, `.h`, `.cc`, `.cpp`, `.cxx`, `.hpp`, `.hh`, `.hxx`): green
  - other common code files (`.java`, `.kt`, `.rs`, `.rb`, `.php`, `.cs`,
    `.swift`, shell scripts): green

Implementation anchors:

- `filesystemWidgetState.formatEntryRow()`
- `filesystemEntryIcon()`
- `filesystemEntryStyle()`
- `isFilesystemConfigFile()`
- `filesystemCodeFileStyle()`

## Command/Input Model

- Current mode: first-class command parser exists in dedicated filesystem
  process.
- Input-reader contract is shared with `context` and `system-settings` applets;
  key normalization behavior in this module is authoritative for common
  navigation keys in raw-terminal mode.
- Supported navigation/action keys:
  - Focused key-monitor mode: single-keystroke actions (no command + Enter)
  - Synthetic `..` parent entry appears at top of non-root directories; selecting it and pressing `Enter` navigates up one level
  - `Up`/`Down` as primary row navigation keys (`k`/`j` retained for compatibility)
  - `Left`/`Right` escape-key normalization is provided by the shared reader
    contract for sibling navigation semantics in applets that choose to use it.
  - `Tab` key normalization is provided by the shared reader contract for
    focus-mode switching semantics in applets that choose to use it.
  - `PageUp`/`PageDown` move viewport by one full page; `Home`/`End` jump to top/bottom
  - Arrow navigation applies line-buffer viewport scrolling when selection crosses viewport edge
  - Viewport adapts to active terminal pane dimensions (rows and width) on focused render cycles
  - `u` (parent), `b` (back), `f` (forward), `h` (home), `r` (refresh)
  - `enter`/`l` (open dir or attach file), `a` (attach), `e` (edit), `q` (quit)
  - Boundary attempt feedback: terminal bell (`\a`) when navigation hits top/bottom limits
- Long-list viewport/paging behavior for `PD-11-AF-011..018` is implemented and
  covered by executable tests.

## Owned Data/State

- Authoritative ownership: files surface UX closure, acceptance evidence, and
  sign-off state belong to this `filesystem` applet contract.
- Runtime-owned state in `filesystemWidgetState`:
  - current path and history (`currentDir`, `history`, `historyIndex`)
  - loaded entries (`entries`) and hard selection (`selected`)
  - viewport state (`viewOffset`, `viewportRows`, `viewportCols`)
  - persistent soft-select set (`softSelected`)
  - status/help/bell state (`status`, `showHelp`, `bellPending`)

## UX Flow Coverage

- Flow C system panel parity includes `files` tab expectations
- Flow C.1 system tab tour validates `files` in deterministic order

Primary evidence targets:

- `cmd/agentx-core/filesystem_widget.go`
- `cmd/agentx-core/filesystem_widget_test.go`
- `cmd/agentx-core/core_applet_supervisor_test.go`
- `cmd/agentx-core/demo_harness.go` (`e2e-system-001`, `e2e-system-tour-001`)
- `tests/test_demo_system_panel_tour_headless.sh`

## Launch and Composition Contract

### Launch order

Current launch order (runtime): managed by core applet host startup sequencing.
Filesystem applet is a first-class process entry (`--filesystem-widget`).

Target composition order in tmuxp remains an integration concern and must not
change this applet's authoritative ownership or widget contract.

### Dedicated tmux target

- Default startup mode (target): dedicated first-class pane/window composed by
  tmuxp into system layout.
- Visible-windows startup mode (target): dedicated first-class window.

### Composition rule

- tmuxp owns the composition of first-class applet windows/panes into the
  operator-facing system layout.
- Core ownership and health/IPC contracts remain under Go core orchestration.

## Integration Touchpoints

- `context` applet: integration peer for system-tab flow parity and lifecycle
  orchestration; no ownership transfer of files-surface contract.
- `input` applet: file attach flows must integrate with input-side context and
  attachment intent (`UF-11`).
- `system-settings` applet: both participate in deterministic system tab-tour
  order (`PD-17-AF-014`).
- `output`/`logs` applets: file-action diagnostics and related lifecycle messages
  must be traceable through core runtime output/log channels.

## Test Evidence Targets

- Unit anchors (authoritative filesystem runtime):
  - `cmd/agentx-core/filesystem_widget_test.go`
  - `cmd/agentx-core/core_applet_supervisor_test.go`
- Integration/functional anchors:
  - `cmd/agentx-core/demo_harness.go` (`e2e-system-001`, `e2e-system-tour-001`)
  - `tests/test_demo_system_panel_tour_headless.sh`
  - `tests/test_file_explorer_context_menu.py` (GUI UX reference for `PD-11-AF-008..010`)

## Current State

- First-class runtime entrypoint is implemented in Go and actively used as the
  authoritative filesystem applet implementation.
- Keyboard affordances and overflow behavior required by `PD-11-AF-011..018`
  are implemented with executable evidence.
- Styling decisions (emoji markers, reverse-video folders, parent-row
  highlighting, file-type color classes) are implemented in the filesystem
  widget render path.

## Gap Note

- Remaining gaps, if any, are composition/runtime-inventory alignment items
  outside this applet's core widget contract (for example tmuxp assembly and
  cross-doc traceability synchronization). The filesystem widget behavior itself
  is implemented and test-covered.

## Done Criteria

1. Dedicated runtime entrypoint and lifecycle registration exist.
2. tmuxp composition wiring places the surface in system UX layout.
3. Unit + integration + functional parity evidence passes.
4. Traceability rows are reconciled as tested in UX lifecycle docs.
5. Interactive TUI affordances meet `PD-11-AF-011..018`; if any affordance has
  no obvious implementation path, user-approved case-by-case decision is
  documented before closure.
