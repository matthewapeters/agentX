# AgentX — Demo Mode UX Contract

_Last updated: 2026-05-28 (v0.84.0)_

Demo mode is a pre-UAT validation surface for terminal UX and E2E behavior.
It is explicitly user-visible and interactive by design.

For applet-presence validation before the final frame layout lands, use the
optional startup mode documented in [06_TUI_MIRROR.md](06_TUI_MIRROR.md) and
[docs/architecture/runtime_split.md](../architecture/runtime_split.md). DemoMode
continues to govern the interactive test-review loop; it does not redefine the
startup topology contract.

---

## Purpose

Demo mode allows the UAT team to run the current E2E terminal sequence in a live tmux/terminal session and provide per-test feedback before formal UAT closure.

In this release, `agentx --demo` opens a split tmux view with a split-left controller workspace:

- left-top pane: stores (numbered Gherkin use-cases)
- left-bottom pane: testControler command prompt
- right pane: live AgentX core session (`output` / `system` / `input`)

### Authoritative Demo Pane Titles

The demo split pane-title contract is authoritative:

| Demo pane role | Required title |
|------|-------------|
| Story board pane | `stores` |
| Test control pane | `testControler` |
| Live runtime mirror pane | `liveCore` |

No additional demo pane titles may be introduced without first updating this document and matching tests.

The story browser is now a live status board with explicit per-test markers:

- `[ ]` pending/skip
- `[/]` active test
- `[P]` pass
- `[X]` fail

Navigation instructions are rendered in the lower testControler pane. The stores pane supports long-list navigation directly via pager controls:

- `Ctrl-b o` to focus stores pane
- arrow keys or `PgUp` / `PgDn` to scroll
- `R` to refresh content manually (status updates are also auto-refreshed)
- `Ctrl-b o` to return to testControler pane

The testControler submits prompts over the core `/submit` endpoint so the operator watches the actual running application respond in real time without replacing the split.

Command-line entry contract:

```bash
agentx --demo
```

Optional start-selection contract:

```bash
agentx --demo --demo-start <test-id-or-index>
```

The internal smoke gate uses `--demo-headless` to preserve deterministic artifact coverage without presenting the split-pane controller UI.

---

## Explicit Contract

### Scope

- Demo mode runs terminal E2E scenarios in a user-visible terminal.
- Demo mode is not a replacement for unit/integration/functional suites.
- Demo mode is a gate before UAT sign-off and after automated E2E checks.
- `--demo` is the interactive split-pane UX.
- `--demo-headless` is the internal non-interactive validation path used by smoke tests.

### Sequence Presentation

At demo start, the user must see:

- ordered list of E2E demo tests to be run
- stable test id and short human-readable title per test
- estimated duration per test (best-effort)
- selected starting point (default first test)
- Gherkin `GIVEN/WHEN/THEN` expectations for each test
- story browser pane (left-top) remains visible while the command prompt pane (left-bottom) accepts input
- live core session remains visible on the right for the duration of the run
- story browser lines include per-test status marker (`[ ]`, `[/]`, `[P]`, `[X]`) beside each test id

### Start Selection

The user must be able to choose where to start the sequence:

- start from test 1 (full review)
- start from any later test (targeted review)
- start from most recently added test group (if grouped/tagged)

### Per-Test Interaction (Required)

At the end of each test (not end of full sequence), control is returned to the user.

Prompt contract:

- `N` = mark current demo test as accepted and continue to next test
- `J <test number>` = jump ahead to a specific test number in the same run
- `X <feedback>` = mark current demo test as failed, optionally attach inline feedback, capture diagnostics, and stop sequence

In split mode, completion from the testControler pane now closes the full demo split session so the remaining mirror pane does not expand into a one-pane terminal.

In split mode, `Ctrl-C` cancellation now exits the testControler decision loop and triggers split-session teardown instead of leaving a re-prompting testControler pane.

In split mode, the testControler pane is refreshed between decisions so stale prompts/results are cleared and the current step remains legible.

Any other input must re-prompt without advancing.

Jump rules:

- jump target must be within valid range (`1..N`)
- jump target must be ahead of the current test (no backward jump)
- jumped-over tests are marked `SKIP` in the status ledger

### Failure Capture (`X`)

On `X`, demo mode must capture complete diagnostics for agent analysis:

- full `tmux capture-pane` dump for all panes in the active demo session
- pane metadata (`list-panes`, `display-message`) and active window/pane info
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
- explicit readiness statement: `Ready for UAT` only when all selected demo tests were accepted

---

## Affordance IDs (PD-17)

- `PD-17-AF-001` — `--demo` CLI flag enters demo mode
- `PD-17-AF-002` — demo sequence list is shown before execution
- `PD-17-AF-003` — user selects start test id/index before sequence begins
- `PD-17-AF-004` — split demo left workspace is split into story-browser (top) and prompt pane (bottom)
- `PD-17-AF-005` — per-test user feedback prompt accepts `N`, `J <num>`, `X <feedback>`
- `PD-17-AF-006` — `X` triggers full pane-dump diagnostics to log artifacts
- `PD-17-AF-007` — inline `X <feedback>` is persisted into diagnostics artifacts
- `PD-17-AF-008` — end-of-run summary and readiness result is displayed
- `PD-17-AF-009` — story-browser shows inline per-test status markers (`[ ]`, `[/]`, `[P]`, `[X]`)
- `PD-17-AF-010` — controller pane refreshes/clears between decisions to prevent muddled prompt history
- `PD-17-AF-011` — startup greeting parity criteria and demo story (`e2e-greet-001`) are defined
- `PD-17-AF-012` — full prompt lifecycle parity criteria and demo story (`e2e-cycle-001`) are defined
- `PD-17-AF-013` — system panel parity criteria and demo story (`e2e-system-001`) are defined
- `PD-17-AF-014` — system panel tab-tour parity criteria and demo story (`e2e-system-tour-001`) are defined

## Hybrid UX Parity Criteria (W0.1 Baseline)

The following acceptance criteria define parity targets for the hybrid architecture and are represented by placeholder demo stories.

### Flow A - Startup Greeting Parity (`PD-17-AF-011`)

- At startup, the default assistant greeting is visible without requiring a manual user prompt.
- Greeting appears once per session start and is persisted in session context.
- Demo coverage placeholder: `e2e-greet-001`.

### Flow B - Prompt Lifecycle Parity (`PD-17-AF-012`)

- A representative prompt must visibly traverse all lifecycle stages:
  - submitted
  - classified
  - thinking
  - tool activity (when applicable)
  - final response
- Lifecycle ordering must be deterministic for test assertions.
- Demo coverage placeholder: `e2e-cycle-001`.

### Flow C - System Panel Parity (`PD-17-AF-013`)

- Hybrid runtime must provide functional parity for system tabs:
  - files
  - configuration
  - context
  - context history
  - context visualizer
- Tab navigation and state rendering must be deterministic and testable.
- Demo coverage placeholders: `e2e-system-001`, `e2e-system-tour-001`.

### Flow C.1 - System Panel Tab Tour Parity (`PD-17-AF-014`)

- A single demo run starting at `e2e-system-tour-001` must validate all system tabs in order:
  - files
  - configuration
  - context
  - context history
  - context visualizer
- Each tab must render expected section content and must not leak unrelated sections in the active snapshot.
- Demo coverage placeholder: `e2e-system-tour-001`.

## Wave 4 UAT Checklist

- Story set and pass criteria:
  - `e2e-system-tour-001`: all five system tabs validated in a single run.
  - `e2e-greet-001`: startup greeting contract passes and input remains command-entry only.
  - `e2e-cycle-001`: prompt lifecycle rows and user/agent turn contract passes.
  - `e2e-system-001`: system-pane context visualization contract passes.
- Required validation commands:
  - `tests/test_demo_system_panel_tour_headless.sh`
  - `tests/test_demo_ux_use_cases_headless.sh`
  - `tests/test_demo_ux_use_cases_layout_headless.sh`
- Readiness gate:
  - `Ready for UAT` is valid only when selected stories pass and no failure artifact path is produced.

---

## Implementation Plan

### Phase D1 — Contract Scaffolding

Status: COMPLETE in `cmd/agentx-core`.

1. Add CLI flags in Go core entry:
   - `--demo`
   - `--demo-start`
2. Introduce `DemoHarness` orchestration unit (Go core side).
3. Define stable demo test manifest format:
   - id, title, command, expected pane checks, tags, approximate duration.

Exit criteria:

- `agentx --demo` presents ordered sequence before interactive test execution.
- `agentx --demo --demo-start` validates test id/index and begins from the selected start point.

### Phase D2 — Interactive Execution Loop

Status: COMPLETE in `cmd/agentx-core/demo_harness.go`.

1. Execute selected sequence starting at chosen test.
2. For each test:
   - run E2E action(s)
   - show result summary
   - return terminal control to user prompt (`N`/`X`).
3. Block advancement on invalid input.

Exit criteria:

- per-test prompt appears every time and only accepts `N`/`X`.
- starting from arbitrary index/id is functional.

### Phase D3 — Failure Diagnostics

Status: COMPLETE in `cmd/agentx-core/demo_harness.go`.

1. On `X`, run pane capture bundle:
   - `tmux list-panes`, `tmux display-message`, `tmux capture-pane` for each pane.
2. Persist artifacts under `logs/demo/<session-id>/<test-id>/`.
3. Print artifact paths in terminal summary.

Exit criteria:

- one-command retrieval of full failure context is possible from logs.

### Phase D4 — Test and Gate Integration

Status: COMPLETE in `tests/test_demo_smoke_headless.sh` and `Makefile`.

1. Add hermetic unit tests for manifest parsing, start-selection logic, and prompt-state machine.
2. Add headless integration tests for diagnostics artifact creation.
3. Add terminal E2E tests for demo interaction semantics where feasible.
4. Add a dedicated make target:
   - `make demo-smoke` (non-interactive subset via `--demo-headless`)

Exit criteria:

- demo-mode control logic covered by automated tests.
- docs and matrix status updated from `📝` to `✅` as implementations land.

## D4 Smoke Gate

- `make demo-smoke` runs the headless artifact-capture smoke path.
- the script launches `agentx --demo-headless`, sends `X` at the first prompt, and verifies the bundle under `logs/demo/<session>/<test>/`.

---

## UX Notes

Demo mode is a UX surface. It must be:

- organized: clear sequence and per-test status
- clean: no unsanctioned operational noise in user-facing panes
- navigable: pane capture artifacts are easy to locate for analysis
- deterministic: repeated runs produce comparable logs and prompts
