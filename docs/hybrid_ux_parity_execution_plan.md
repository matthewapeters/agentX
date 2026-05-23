# Hybrid UX Parity Execution Plan

_Last updated: 2026-05-22 (v0.81.4.post1)_

## Purpose

Track and execute UX parity work between established UX flows and the hybrid architecture in a way that is reliable across multiple agent sessions.

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

- [ ] W0.1 Define parity acceptance criteria for Flow A/B/C in docs and traceability matrix.
  - Complete when:
    - Acceptance criteria are documented and mapped to affordance IDs.
    - Demo harness has placeholder stories for each flow.

- [ ] W0.2 Add observability hooks for lifecycle events (startup greeting, classify, thinking, tool, final response).
  - Complete when:
    - Events are emitted in deterministic order in logs.
    - Unit tests verify event ordering contract.

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
