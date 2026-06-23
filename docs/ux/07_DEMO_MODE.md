# AgentX — Demo Mode Testing Contract

_Last updated: 2026-06-23_

Demo mode is an interactive validation surface for end-to-end behavior testing.
It is explicitly user-visible and allows manual testing and feedback before formal acceptance.

Demo mode provides an interactive test harness for validating application behavior across all surfaces (output, input, system, logs, etc.).

---

## Purpose

Demo mode runs pre-scripted E2E scenarios that validate application behavior across all surfaces. It presents test cases in sequence with interactive per-test feedback from the operator.

In this release, `agentx --demo` launches a split workspace view with:

- Left-top surface: test story board (numbered test cases)
- Left-bottom surface: test control/command interface
- Right surface: live running application session (output/system/input/logs surfaces)

### Authoritative Demo Surface Titles

The demo split surface-title contract is authoritative:

| Demo surface role | Required title |
|------|-------------|
| Story board surface | `stores` |
| Test control surface | `testControler` |
| Live runtime mirror surface | `liveCore` |

No additional demo surface titles may be introduced without first updating this document and matching tests.

The story board is a live status board with explicit per-test markers:

- `[ ]` pending/skip
- `[/]` active test
- `[P]` pass
- `[X]` fail

Navigation instructions are rendered in the lower test control surface. The story board supports navigation directly via standard controls (no multiplexer-specific keybindings required):

- Focus story board surface to view test list
- Scroll keys or `PgUp` / `PgDn` to navigate
- `R` to refresh content manually (status updates are also auto-refreshed)
- Focus back to test control surface for command entry

The test control surface submits test commands over the application's `/submit` endpoint so the operator watches the running application respond in real time.

Command-line entry contract:

```bash
agentx --demo
```

Optional start-selection contract:

```bash
agentx --demo --demo-start <test-id-or-index>
```

The internal smoke gate uses `--demo-headless` to preserve deterministic artifact coverage without presenting the split-surface control interface.

---

## Explicit Contract

### Scope

- Demo mode runs pre-scripted E2E test scenarios in an interactive, operator-controlled environment.
- Demo mode is not a replacement for unit/integration/functional suites.
- Demo mode is a validation gate before formal acceptance sign-off and after automated E2E checks.
- `--demo` is the interactive split-surface control interface.
- `--demo-headless` is the non-interactive automated validation path used by smoke tests.

### Sequence Presentation

At demo start, the user must see:

- ordered list of E2E test scenarios to be run
- stable test id and short human-readable title per test
- estimated duration per test (best-effort)
- selected starting point (default first test)
- Test expectations (GIVEN/WHEN/THEN) for each scenario
- story board surface remains visible while the test control surface accepts commands
- live application session remains visible for the duration of the run
- story board lines include per-test status marker (`[ ]`, `[/]`, `[P]`, `[X]`) beside each test id

### Start Selection

The user must be able to choose where to start the sequence:

- start from test 1 (full review)
- start from any later test (targeted review)
- start from most recently added test group (if grouped/tagged)

### Per-Test Interaction (Required)

At the end of each test (not end of full sequence), control is returned to the user.

Command contract:

- `N` = mark current demo test as accepted and continue to next test
- `J <test number>` = jump ahead to a specific test number in the same run
- `X <feedback>` = mark current demo test as failed, optionally attach inline feedback, capture diagnostics, and stop sequence

In interactive mode, completion from the test control surface closes the full demo session so remaining surfaces do not remain orphaned.

In interactive mode, `Ctrl-C` cancellation exits the test control loop and triggers session teardown instead of leaving a re-prompting control surface.

In interactive mode, the test control surface is refreshed between commands so stale output is cleared and the current step remains legible.

Any other input must re-prompt without advancing.

Jump rules:

- jump target must be within valid range (`1..N`)
- jump target must be ahead of the current test (no backward jump)
- jumped-over tests are marked `SKIP` in the status ledger

### Failure Capture (`X`)

On `X`, demo mode must capture complete diagnostics for analysis:

- complete application state dump and surface snapshots for all active surfaces in the session
- surface metadata and active surface info
- executed test id/title and timestamp
- session id and runtime config snapshot (safe, non-secret fields)
- optional inline feedback text from `X <feedback>`

Artifacts must be written to a deterministic log path under repository `logs/`.

If inline feedback is provided, it must be persisted to:

- `metadata.json` (`failure_feedback` field)
- `demo_feedback.txt`

### Success Progression (`N` / `J`)

On `N`, demo mode must:

- persist per-test pass record
- advance to next test in sequence
- preserve cumulative pass/fail summary

On `J <test number>`, demo mode must:

- confirm jump target and print jump confirmation
- continue execution from selected test number
- preserve prior statuses and keep skipped intervening tests as `SKIP`

### End-of-Run Summary

At sequence end (or stop on `X`), demo mode must print:

- total tests run
- accepted tests count
- failed test id/title (if any)
- paths to captured artifacts
- explicit readiness statement: `Ready for acceptance` only when all selected demo tests were accepted

---

## Affordance IDs (PD-17)

- `PD-17-AF-001` — `--demo` CLI flag enters demo mode
- `PD-17-AF-002` — demo sequence list is shown before execution
- `PD-17-AF-003` — user selects start test id/index before sequence begins
- `PD-17-AF-004` — split demo workspace is split into story-board surface (top) and test control surface (bottom)
- `PD-17-AF-005` — per-test user feedback prompt accepts `N`, `J <num>`, `X <feedback>`
- `PD-17-AF-006` — `X` triggers full diagnostics capture to log artifacts
- `PD-17-AF-007` — inline `X <feedback>` is persisted into diagnostics artifacts
- `PD-17-AF-008` — end-of-run summary and readiness result is displayed
- `PD-17-AF-009` — story-board shows inline per-test status markers (`[ ]`, `[/]`, `[P]`, `[X]`)
- `PD-17-AF-010` — test control surface refreshes/clears between commands to prevent muddled output history
- `PD-17-AF-011` — startup greeting validation criteria and test case (`e2e-greet-001`) are defined
- `PD-17-AF-012` — user interaction lifecycle validation criteria and test case (`e2e-cycle-001`) are defined
- `PD-17-AF-013` — system surface validation criteria and test case (`e2e-system-001`) are defined
- `PD-17-AF-014` — system surface tour validation criteria and test case (`e2e-system-tour-001`) are defined

## Generic Test Parity Criteria

The following acceptance criteria define key test scenarios for validating application behavior across all surfaces and are represented by test cases in the demo suite.

### Scenario A - Startup Validation

- At startup, the default system greeting is visible without requiring a manual user prompt.
- Greeting appears once per session start and is persisted in session context.
- Test case placeholder: `e2e-greet-001`.

### Scenario B - User Interaction Lifecycle

- A representative user input must visibly traverse all interaction stages:
  - submitted
  - classified
  - thinking (where applicable)
  - tool activity (when applicable)
  - final response
- Lifecycle ordering must be deterministic for test assertions.
- Test case placeholder: `e2e-cycle-001`.

### Scenario C - System Surface Validation

- Application must provide functional test coverage for system surfaces:
  - files
  - configuration
  - context
  - context history
  - context visualizer
- Surface navigation and state rendering must be deterministic and testable.
- Test case placeholders: `e2e-system-001`, `e2e-system-tour-001`.

### Scenario C.1 - System Surface Tour Validation

- A single demo run starting at `e2e-system-tour-001` must validate all system surfaces in order:
  - files
  - configuration
  - context
  - context history
  - context visualizer
- Each surface must render expected content and must not leak unrelated sections in the active snapshot.
- Test case placeholder: `e2e-system-tour-001`.

## Test Acceptance Checklist

- Test cases and pass criteria:
  - `e2e-system-tour-001`: all system surfaces validated in a single run.
  - `e2e-greet-001`: startup greeting contract passes and input remains command-entry only.
  - `e2e-cycle-001`: user interaction lifecycle and turn contract passes.
  - `e2e-system-001`: system-surface context validation contract passes.
- Required test harness commands:
  - `tests/test_demo_system_panel_tour_headless.sh`
  - `tests/test_demo_ux_use_cases_headless.sh`
  - `tests/test_demo_ux_use_cases_layout_headless.sh`
- Readiness gate:
  - "Ready for acceptance" is valid only when selected test cases pass and no failure artifact path is produced.

---

## Implementation Plan

### Phase D1 — Contract Scaffolding

Status: COMPLETE in `cmd/agentx-core`.

1. Add CLI flags in Go core entry:
   - `--demo`
   - `--demo-start`
2. Introduce `DemoHarness` orchestration unit.
3. Define stable demo test manifest format:
   - id, title, command, expected surface checks, tags, approximate duration.

Exit criteria:

- `agentx --demo` presents ordered sequence before interactive test execution.
- `agentx --demo --demo-start` validates test id/index and begins from the selected start point.

### Phase D2 — Interactive Execution Loop

Status: COMPLETE.

1. Execute selected sequence starting at chosen test.
2. For each test:
   - run E2E action(s)
   - show result summary
   - return control to user for next command (`N`/`X`).
3. Block advancement on invalid input.

Exit criteria:

- per-test prompt appears every time and only accepts `N`/`X`.
- starting from arbitrary index/id is functional.

### Phase D3 — Failure Diagnostics

Status: COMPLETE.

1. On `X`, capture complete application state bundle:
   - Surface snapshots and metadata for all active application surfaces.
2. Persist artifacts under `logs/demo/<session-id>/<test-id>/`.
3. Print artifact paths in output summary.

Exit criteria:

- one-command retrieval of full failure context is possible from logs.

### Phase D4 — Test and Gate Integration

Status: COMPLETE.

1. Add unit tests for manifest parsing, start-selection logic, and test state machine.
2. Add integration tests for diagnostics artifact creation.
3. Add E2E tests for demo interaction semantics where feasible.
4. Add a dedicated make target:
   - `make demo-smoke` (non-interactive subset via `--demo-headless`)

Exit criteria:

- demo-mode control logic covered by automated tests.
- docs and matrix status updated from `📝` to `✅` as implementations land.

## D4 Smoke Gate

- `make demo-smoke` runs the headless artifact-capture smoke test path.
- the script launches `agentx --demo-headless`, sends `X` at the first test, and verifies the bundle under `logs/demo/<session>/<test>/`.

---

## Test Interface Notes

Demo mode is a testing and validation surface. It must be:

- organized: clear sequence and per-test status
- clean: no unsanctioned operational noise in user-facing surfaces
- navigable: failure diagnostics are easy to locate for analysis
- deterministic: repeated runs produce comparable logs and prompts
