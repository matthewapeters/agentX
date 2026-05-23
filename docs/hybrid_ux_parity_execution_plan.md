# Hybrid UX Parity Execution Plan

_Last updated: 2026-05-23 (v0.83.0)_

## Purpose

Track and execute UX parity work between established UX flows and the hybrid architecture in a way that is reliable across multiple agent sessions.

## Tab/Surface Parity Matrix (Source of Truth)

| GUI Tab / Surface         | TUI Analog / Surface      | Owner (Go/Py) | Test / Contract                        | Status   |
|--------------------------|---------------------------|---------------|----------------------------------------|----------|
| Chat (Output)            | Output Pane (chat)        | Py (now), Go (target) | test_chat_panel_turn_rendering.py      | ⚠️ Py-owned, Go TODO |
| Tool Processing (Output) | Output Pane (tool)        | Py (now), Go (target) | test_chat_panel_turn_rendering.py      | ⚠️ Py-owned, Go TODO |
| File Edit (Output)       | Output Pane (file edit)   | Py (now), Go (target) | test_chat_panel_turn_rendering.py      | ⚠️ Py-owned, Go TODO |
| Context Visualization    | System Pane (context viz) | Py (now), Go (target) | test_status_tab.py, test_context_meter_widget.py | ⚠️ Py-owned, Go TODO |
| Files Navigation         | System Pane (files)       | Py (now), Go (target) | test_file_explorer_coverage.py         | ⚠️ Py-owned, Go TODO |
| Context History/Current  | System Pane (context hist)| Py (now), Go (target) | test_phase6_context_panel.py           | ⚠️ Py-owned, Go TODO |
| Configuration            | System Pane (config)      | Py (now), Go (target) | test_settings_tab_sections.py           | ⚠️ Py-owned, Go TODO |

**Blockers:** All output/system tabs still Py-owned. No Go-driven agentic orchestration. No TUI analogs for tab navigation. All must migrate to Go for true parity. All unmapped = blocker.

**Links:**

- [docs/ux/UX_LIFECYCLE.md](docs/ux/UX_LIFECYCLE.md) — traceability, affordance IDs, test mapping
- [docs/architecture.md](docs/architecture.md) — module map, runtime split, channel registry (to add)

## Next Steps Checklist (until 100% parity)

1. [ ] Ratify and commit this matrix in plan and UX_LIFECYCLE.
2. [ ] For each tab/surface, define TUI analog and Go owner in code/spec.
3. [ ] Implement Go-driven output loop (input/classify/think/tool/respond) for all output tabs.
4. [ ] Implement Go-driven system panel routing and data providers for all system tabs.
5. [/] Author channel registry doc: channel name, schema, pub/sub, policy, code link. ✓ See [docs/architecture/channel_registry.md](docs/architecture/channel_registry.md)
6. [ ] Author runtime split doc: Python applet vs Go routine, migration phases, thin-GUI contract.
7. [ ] Update all tests to cover Go-driven path; mark blockers in UX_LIFECYCLE.
8. [ ] No parity claim until all blockers resolved and all tests pass.

---

This plan is execution-first:

- each step has an explicit status marker
- each step has objective completion criteria
- a step is only marked complete after its demo scenario passes

## Status Markers

- `[ ]` Not started
- `[/]` Complete (demo scenario passed)
- `[X]` Blocked/failed (requires follow-up)

## Session Handoff Protocol (Required)

At the start of every session:

1. Open this plan and find the first `[ ]` step.
2. Read the previous step notes and verify prerequisites are still true.
3. Implement code/tests for only the selected step scope.
4. Run unit + integration + E2E/demo checks for that step.
5. If demo scenario passes, mark the step `[/]` and append evidence.
6. If blocked, mark the step `[X]` and add a short blocker note.

At the end of every session:

1. Update the step status marker.
2. Record executed commands and pass/fail summary in the step notes.
3. Commit all plan/code/test/doc changes together.

## Execution Waves

## Wave 0 - Baseline and Instrumentation

- [/] W0.1 Define parity acceptance criteria for Flow A/B/C in docs and traceability matrix. ✓ completed 2026-05-22
  - Complete when:
    - Acceptance criteria are documented and mapped to affordance IDs.
    - Demo harness has placeholder stories for each flow.
  - Evidence:
    - Acceptance criteria documented in `docs/ux/07_DEMO_MODE.md` under "Hybrid UX Parity Criteria (W0.1 Baseline)".
    - Traceability matrix rows added: `PD-17-AF-011`, `PD-17-AF-012`, `PD-17-AF-013` in `docs/ux/UX_LIFECYCLE.md`.
    - Placeholder stories added in `cmd/agentx-core/demo_harness.go`: `e2e-greet-001`, `e2e-cycle-001`, `e2e-system-001`.

- [/] W0.2 Add observability hooks for lifecycle events (startup greeting, classify, thinking, tool, final response). ✓ completed 2026-05-23
  - Complete when:
    - Events are emitted in deterministic order in logs.
    - Unit tests verify event ordering contract.
  - Evidence:
    - Lifecycle hooks added in `cmd/agentx-core/core.go` for `startup_greeting`, `submitted`, `classified`, `thinking`, `tool`, and `final_response` stages.
    - Deterministic ordering contract validated in `cmd/agentx-core/core_lifecycle_observability_test.go`.

## Wave 1 - Flow A: Startup Greeting Parity

- [ ] A1 Implement startup greeting bootstrap in hybrid runtime (one-shot per session start).
  - Complete when:
    - Greeting appears at startup without manual user prompt.
    - Reload semantics are deterministic and documented.

- [ ] A2 Unit tests for greeting bootstrap and one-shot guard.
  - Complete when:
    - Unit suite validates greeting emission, persistence, and duplicate suppression.

- [ ] A3 Integration tests for startup greeting with mocked LLM backend.
  - Complete when:
    - Startup path receives and renders greeting via adapter boundary.

- [ ] A4 E2E + demo harness story for startup greeting.
  - Demo story: `e2e-greet-001`
  - Complete when:
    - Demo run shows startup greeting in expected pane(s).
    - Story passes in demo summary and artifacts are clean.

## Wave 2 - Flow B: Full Prompt Lifecycle Parity

- [ ] B1 Implement canonical lifecycle stage rendering in hybrid UI:
  - `submitted -> classified -> thinking -> tool activity -> final response`
  - Complete when:
    - All stages are visible in the UX for a representative prompt.

- [ ] B2 Unit tests for lifecycle state machine and transition validity.
  - Complete when:
    - Invalid transitions are rejected and tested.

- [ ] B3 Integration tests for classifier/thinking/tool/final-response orchestration.
  - Complete when:
    - End-to-end pipeline stages appear in the expected order under mocks.

- [ ] B4 E2E + demo harness story for lifecycle visibility.
  - Demo story: `e2e-cycle-001`
  - Complete when:
    - Demo scenario shows full cycle with stage evidence.
    - Story passes in demo summary.

## Wave 3 - Flow C: System Panel Parity

- [ ] C1 Define hybrid architecture mapping for System panel tabs:
  - files, configuration, context, context history, context visualizer
  - Complete when:
    - Mapping doc identifies source of truth and render adapter per tab.

- [ ] C2 Implement tab routing/state sync in hybrid runtime.
  - Complete when:
    - Tab switch behavior is deterministic and stateful.

- [ ] C3 Unit tests for tab router and per-tab adapter contracts.
  - Complete when:
    - Tab-level state and actions have coverage with deterministic fixtures.

- [ ] C4 Integration tests for data providers and tab rendering pipeline.
  - Complete when:
    - Each tab renders expected data from provider boundary.

- [ ] C5 E2E + demo harness system-panel tour scenario.
  - Demo story: `e2e-system-001`
  - Complete when:
    - Demo validates all five tabs and expected outputs.
    - Story passes in demo summary.

## Wave 4 - Hardening and Release Readiness

- [ ] H1 Regression pack for A/B/C flows (unit, integration, E2E).
  - Complete when:
    - CI/local run passes all targeted suites for parity flows.

- [ ] H2 Demo harness parity run containing all new UX-flow stories.
  - Complete when:
    - `e2e-greet-001`, `e2e-cycle-001`, and `e2e-system-001` all pass.

- [ ] H3 Documentation reconciliation and final UAT handoff.
  - Complete when:
    - UX docs, lifecycle matrix, and changelog are updated.
    - UAT checklist references all parity stories and pass criteria.

## Evidence Log (Append Per Session)

Use this lightweight template after each session:

- Date:
- Agent/session:
- Step(s) touched:
- Commands run:
- Test results:
- Demo story results:
- Status updates applied:
- Commit hash:

### Session 2026-05-22 (W0.1)

- Date: 2026-05-22
- Agent/session: Copilot (hybrid parity continuation)
- Step(s) touched: W0.1
- Commands run:
  - `cd cmd/agentx-core && gofmt -w demo_harness.go demo_harness_test.go && go test ./...`
  - `cd /Projects/agentX && make demo-smoke`
- Test results:
  - Go test suite: pass
  - Demo smoke: pass
- Demo story results:
  - Placeholder stories added to sequence (`e2e-greet-001`, `e2e-cycle-001`, `e2e-system-001`)
  - Story execution marked as placeholder-only baseline (full pass criteria deferred to A4/B4/C5)
- Status updates applied:
  - W0.1 set to `[/]`
- Commit hash:

### Session 2026-05-23 (W0.2)

- Date: 2026-05-23
- Agent/session: Copilot (hybrid parity continuation)
- Step(s) touched: W0.2
- Commands run:
  - `cd cmd/agentx-core && gofmt -w core.go core_lifecycle_observability_test.go`
  - `cd cmd/agentx-core && go test ./...`
  - `cd /Projects/agentX && make demo-smoke`
- Test results:
  - Go test suite: pass
  - Demo smoke: pass
- Demo story results:
  - Demo harness run remained stable after lifecycle hook insertion.
- Status updates applied:
  - W0.2 set to `[/]`
- Commit hash:
