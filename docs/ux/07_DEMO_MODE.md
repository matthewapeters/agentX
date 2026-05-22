# AgentX — Demo Mode UX Contract

_Last updated: 2026-05-22 (v0.74.4.post2)_

Demo mode is a pre-UAT validation surface for terminal UX and E2E behavior.
It is explicitly user-visible and interactive by design.

---

## Purpose

Demo mode allows the UAT team to run the current E2E terminal sequence in a live tmux/terminal session and provide per-test feedback before formal UAT closure.

Command-line entry contract (planned):

```bash
agentx --demo
```

Optional start-selection contract (planned):

```bash
agentx --demo --demo-start <test-id-or-index>
```

---

## Explicit Contract

### Scope

- Demo mode runs terminal E2E scenarios in a user-visible terminal.
- Demo mode is not a replacement for unit/integration/functional suites.
- Demo mode is a gate before UAT sign-off and after automated E2E checks.

### Sequence Presentation

At demo start, the user must see:

- ordered list of E2E demo tests to be run
- stable test id and short human-readable title per test
- estimated duration per test (best-effort)
- selected starting point (default first test)

### Start Selection

The user must be able to choose where to start the sequence:

- start from test 1 (full review)
- start from any later test (targeted review)
- start from most recently added test group (if grouped/tagged)

### Per-Test Interaction (Required)

At the end of each test (not end of full sequence), control is returned to the user.

Prompt contract:

- `N` = mark current demo test as accepted and continue to next test
- `X` = mark current demo test as failed, capture diagnostics, and stop sequence

Any other input must re-prompt without advancing.

### Failure Capture (`X`)

On `X`, demo mode must capture complete diagnostics for agent analysis:

- full `tmux capture-pane` dump for all panes in the active demo session
- pane metadata (`list-panes`, `display-message`) and active window/pane info
- executed test id/title and timestamp
- session id and runtime config snapshot (safe, non-secret fields)

Artifacts must be written to a deterministic log path under repository `logs/`.

### Success Progression (`N`)

On `N`, demo mode must:

- persist per-test pass record
- advance to next test in sequence
- preserve cumulative pass/fail summary

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
- `PD-17-AF-004` — per-test user feedback prompt accepts only `N` or `X`
- `PD-17-AF-005` — `X` triggers full pane-dump diagnostics to log artifacts
- `PD-17-AF-006` — end-of-run summary and readiness result is displayed

---

## Implementation Plan

### Phase D1 — Contract Scaffolding

1. Add CLI flags in Go core entry:
   - `--demo`
   - `--demo-start`
2. Introduce `DemoHarness` orchestration unit (Go core side).
3. Define stable demo test manifest format:
   - id, title, command, expected pane checks, tags, approximate duration.

Exit criteria:

- `agentx --demo` prints planned sequence and exits with clear "not yet implemented" if execution path not wired.
- `agentx --demo --demo-start` validates test id/index.

### Phase D2 — Interactive Execution Loop

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

1. On `X`, run pane capture bundle:
   - `tmux list-panes`, `tmux display-message`, `tmux capture-pane` for each pane.
2. Persist artifacts under `logs/demo/<session-id>/<test-id>/`.
3. Print artifact paths in terminal summary.

Exit criteria:

- one-command retrieval of full failure context is possible from logs.

### Phase D4 — Test and Gate Integration

1. Add hermetic unit tests for manifest parsing, start-selection logic, and prompt-state machine.
2. Add headless integration tests for diagnostics artifact creation.
3. Add terminal E2E tests for demo interaction semantics where feasible.
4. Add a dedicated make target:
   - `make demo-smoke` (non-interactive subset)

Exit criteria:

- demo-mode control logic covered by automated tests.
- docs and matrix status updated from `📝` to `✅` as implementations land.

---

## UX Notes

Demo mode is a UX surface. It must be:

- organized: clear sequence and per-test status
- clean: no unsanctioned operational noise in user-facing panes
- navigable: pane capture artifacts are easy to locate for analysis
- deterministic: repeated runs produce comparable logs and prompts
