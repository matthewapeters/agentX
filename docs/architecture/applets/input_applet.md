# Input Applet Contract

Last updated: 2026-06-03
Applet ID: `input`
Runtime entry: `agentx-core --input-widget`

## UX Anchors

- `PD-02` InputPanel command-entry and submit behavior (`docs/ux/03_PANEL_DETAILS.md`)
- `PD-16-AF-010` input runtime behavior in TUI mirror (`docs/ux/06_TUI_MIRROR.md`)
- `PD-17-AF-005` demo prompt decision/entry behavior (`docs/ux/07_DEMO_MODE.md`)

## Owned Widget/Surface Inventory

- Scrollable multiline input frame (IBM-style bordered viewport)
- Control-entry frame (vim-style command lane activated via `ESC`)
- Input command handling (`:clear`, `:q`, context commands)
- Activity hint rendering sourced from core activity state

## Affordance List

- `PD-16-AF-010`: terminal input widget behavior for prompt submission.
- `PD-17-AF-005`: prompt entry and decision behavior used in demo/runtime flow.
- `PD-02` alignment target: command-entry and submit semantics stay compatible
  with InputPanel interaction intent.

## Command/Input Model

- Primary input mode is multiline compose with a visible cursor.
- Default focus is the input frame; `ESC` toggles focus between input and
  control-entry frames.
- Border color indicates focus owner:
  - focused frame: accent border
  - unfocused frame: muted border
- `Enter` in input focus inserts line breaks (does not submit).
- `Tab` in input focus inserts a literal tab character.
- Cursor vs viewport movement split:
  - Arrow keys move the text cursor
  - `Shift` + Arrow keys pan the input viewport
- Overflow behavior:
  - Right-side vertical scrollbar appears when input height exceeds viewport
  - Bottom horizontal scrollbar appears when line width exceeds viewport
  - Scrollbar thumb position/size reflects relative viewport position/coverage
- Key help is hidden by default and is toggled by entering `:?` in the
  control-entry frame.
- Control-entry submit behavior (`Enter` while control-entry has focus):
  - `:q` submits quit command and closes widget after successful submit
  - `:?` toggles help locally
  - Other commands (e.g., `:clear`, `:context-add ...`, `:ctx-add ...`) are
    submitted to core `/submit`
  - Empty command (`:` only) submits current multiline input buffer
- Prompt label includes most recent context-file signal when available from
  `/activity` snapshot metadata.

### Terminal portability note

- Many terminals do not emit a distinct `Shift+Enter` keycode from `Enter`.
  The widget treats `Enter` as newline insertion in input focus; there is no
  separate `Shift+Enter` submit path.
- Shift+arrow escape encodings vary by terminal; widget decoding accepts both
  CSI-style (`\x1b[1;2A` etc.) and lowercase xterm-style (`\x1b[a` etc.)
  variants for viewport panning.

## Owned Data/State

- Owns input-surface state: current prompt line and local command parsing result.
- Owns prompt-label rendering state, including context-file hint display.
- Does not own orchestration phase transitions or context persistence state.

## UX Flow Coverage

- Prompt submit and command-entry behavior in TUI runtime
- Input clear parity (`e2e-input-001`)

Evidence anchors:

- `cmd/agentx-core/input_widget.go`
- `cmd/agentx-core/input_widget_test.go`
- `cmd/agentx-core/demo_harness.go` (`e2e-input-001`)
- `cmd/agentx-core/demo_harness_test.go`
- `tests/test_demo_ux_use_cases_headless.sh`

## Launch Contract

### Launch order

Applet process launch index: `3`.

### Dedicated tmux target

- Default startup mode: dynamic pane id in window `0` with pane title `input` (initial split target starts at `<session>:0.2`)
- Visible-windows startup mode: `<session>:2.0` (window `input`)

## Core Integration Contract

- Submits prompt input to core `/submit` contract.
- Consumes core `/activity` state for visual guidance only.
- Does not own lifecycle phase transitions.

## Integration Touchpoints

- `output` applet: prompt submissions drive output lifecycle rows.
- `context` applet: `:context-add`/`:ctx-add` must register context-file state
  that is visible in context-driven runtime surfaces.
- `logs` applet: invalid command handling and submit failures should be
  diagnosable through core lifecycle/log telemetry.

## Test Evidence Targets

- Unit anchors:
  - `cmd/agentx-core/input_widget_test.go` — state machine, compose transitions, help gate, multiline submit/cancel/discard, context-add command, scrollbar, activity label
  - `cmd/agentx-core/widget_input_test.go` — shared alias policy, debug toggle
  - `cmd/agentx-core/widget_harness_test.go` — shared test helpers (env, project dir, script runners) available to input tests
- Integration/functional anchors:
  - `cmd/agentx-core/demo_harness.go` (`e2e-input-001`)
  - `cmd/agentx-core/demo_harness_test.go`
  - `tests/test_demo_ux_use_cases_headless.sh`
  - `TestHandleInputLine_ContextAddCommand`
  - `TestWidgetActivityState_PromptLabelIncludesContextFile`

## Open Parity Notes

- Keep command semantics and clear behavior aligned with `PD-02` and `PD-16`.
- Preserve `:clear`/`:q` command compatibility through control-entry submit.

## Gap Note

- Input command contract now includes context-file registration commands, but
  broad GUI-level parity for all `PD-02` affordances is still tracked as part
  of overall hybrid parity completion rather than claimed closed here.
