# Hybrid Go/Python Migration Plan

_Last updated: 2026-05-25 (v0.79.21)_

## Overview

This document tracks the migration from pure-Python TUI to hybrid Go-core + Python-applets architecture.

Authoritative status source: this file is the single source of truth for hybrid migration status and checkpoints.

Authoritative execution source: this file is also the single source of truth for TUI integration completion scope, sprint backlog, issue ownership, and dependency ordering.

**Branch:** `feat/hybrid-go-core-tui-migration`

## Current Sprint Rollup

Current sprint: Sprint 3 — Demo-Harness E2E Proof + UAT Gate.

Current objective: extend demo-harness parity stories and evidence checks to support UAT-ready proof bundles.

Active focus:

- Sprint 3 execution has started from a green Sprint 2 gate baseline.
- HX-301 is in progress with parity story guardrails anchored in the default demo manifest.
- HX-302 evidence-capture hardening is complete with deterministic pane/window metadata assertions in smoke and unit diagnostics tests.
- HX-303 smoke gate now asserts parity evidence markers for startup/lifecycle/system-panel stories and fails on missing markers.
- HX-304 summary evidence checks are now wired into smoke validation for readiness state, failed test id, and artifact path output markers.
- HX-305 merge-gate alignment is complete: reconciled parity checks are now enforced by the hybrid readiness gate.

Recently completed:

- `HX-101` Freeze TUI/GUI parity matrix.
- `HX-102` Canonical event lifecycle contract.
- `HX-103` Subscriber formatting parity.
- `HX-104` Session-mode coexistence contract.
- `HX-105` Parity regression suite for semantic lifecycle.
- `HX-201` Deterministic TUI startup/shutdown wiring.
- `HX-202` Bridge failure-mode handling (timeout/retry/disconnect).
- `HX-203` Integration path: StreamingController -> broker -> TUI subscriber.
- `HX-204` Integration path: input submit -> session -> response stream.

Current blockers:

- No issues are currently marked `Blocked` in the authoritative backlog.

Immediate exit criteria:

- GUI/TUI parity matrix is agreed and linked from UX docs.
- One canonical lifecycle contract exists for startup, turn streaming, tool activity, errors, interrupts, and turn completion.
- TUI subscriber/output formatting matches the canonical contract.
- Regression tests prove no visible event loss, duplication, or reordering for the semantic lifecycle.

## User-Perceivable Value Anchor (Sprint Planning Standard)

Each sprint must end with at least one demonstrable user-perceivable deliverable that can be shown live in less than five minutes.

Canonical example for Sprint 2 value framing:

- Reliable interaction path from input to response.
- Enter prompt in TUI input.
- See a complete streamed response lifecycle in TUI output.
- Turn completion is explicit and consistent every time.

Standard for upcoming sprints:

- Every sprint plan must include a "What the user can experience/demo" statement.
- Every acceptance table must map at least one issue to that statement.
- Every sprint review must include a pass/fail demo script tied to that statement.

Documentation-first workflow standard:

- Planning and user-facing docs are updated before implementation starts for a scoped change.
- Code changes are not considered complete until related docs are updated and reviewed in the same branch state.
- Build/test gate updates must be reflected in both developer docs and sprint execution docs before closure.

## Reconciled Plan Scope (Single Source)

This plan now absorbs the TUI integration completion scope previously tracked in `docs/TUI_INTEGRATION_COMPLETION_PLAN.md`.

Completion remains blocked until all of the following are true:

- TUI mirrors GUI semantics for startup, input, response, thinking, tool call/result, error, interrupt, and end-of-turn lifecycle.
- TUI exposes critical user-facing state currently visible in GUI (context/history summaries, attachment state summary, plan/status visibility).
- Coverage is demonstrable across hermetic unit tests, integration tests, and tmux/demo-harness E2E paths.
- Demo harness outputs are sufficient for UAT evidence without requiring source-level debugging.

Delivery model for this reconciled scope is the authoritative sprintized backlog in the next section.

## Sprintized Backlog (Authoritative)

Legend:

- Owner uses role ownership to avoid single-person bottlenecks.
- Dependencies use issue IDs in this section.
- Status values: `Ready`, `In Progress`, `Blocked`, `Done`.

### Sprint 1 — Parity Contract + Semantic Closure

| Issue | Title | Owner | Dependencies | Status | Acceptance |
|------|-------|-------|--------------|--------|------------|
| HX-101 | Freeze TUI/GUI parity matrix | UX + Product | - | Done | Matrix defines 1:1 vs simplified affordances and is linked from UX docs |
| HX-102 | Canonical event lifecycle contract | Python Runtime | HX-101 | Done | One lifecycle contract for both GUI and TUI (startup -> done/error) |
| HX-103 | Subscriber formatting parity | Python Runtime | HX-102 | Done | `tui_event_subscriber` emits canonical headers/markers for all contract events |
| HX-104 | Session-mode coexistence contract (GUI-only/TUI-only/dual) | Application Architecture | HX-101 | Done | Runtime mode decision logic and user-visible behavior documented and tested |
| HX-105 | Parity regression suite for semantic lifecycle | QA/Automation | HX-102, HX-103, HX-104 | Done | Tests prove ordering and no event loss/regression across turn lifecycle |

### Sprint 2 — Runtime Determinism + Integration Hardening

| Issue | Title | Owner | Dependencies | Status | Acceptance |
|------|-------|-------|--------------|--------|------------|
| HX-201 | Deterministic TUI startup/shutdown wiring | Go Core | HX-104 | Ready | Session startup/stop leaves no orphaned readers, FIFOs, or panes |
| HX-202 | Bridge failure-mode handling (timeout/retry/disconnect) | Python Runtime | HX-103 | Done | Deterministic behavior for slow/unavailable reader and reconnect paths |
| HX-203 | Integration path: StreamingController -> broker -> TUI subscriber | QA/Automation | HX-103, HX-202 | Done | Integration tests verify end-to-end event delivery in-process |
| HX-204 | Integration path: input submit -> session -> response stream | QA/Automation | HX-201, HX-202 | Done | Integration tests prove deterministic prompt round-trip |
| HX-205 | Mode-switch reliability and no-duplication guarantee | Application Architecture | HX-104, HX-201 | Done | Coexistence runs do not duplicate, drop, or reorder visible output |

### Sprint 3 — Demo-Harness E2E Proof + UAT Gate

| Issue | Title | Owner | Dependencies | Status | Acceptance |
|------|-------|-------|--------------|--------|------------|
| HX-301 | Extend demo stories for parity lifecycle cases | Demo Harness | HX-105, HX-204 | Done | Demo sequence includes startup/lifecycle/system-panel parity scenarios |
| HX-302 | Headless tmux evidence capture hardening | QA/Automation | HX-201, HX-301 | Done | Pane titles/geometry/affordance assertions are deterministic and actionable |
| HX-303 | Demo smoke gate includes parity evidence checks | CI/CD | HX-301, HX-302 | Done | `make demo-smoke` fails on missing parity evidence markers |
| HX-304 | UAT-ready report bundle generation | Demo Harness | HX-303 | Done | Demo summary includes readiness, failed cases, and artifact paths |
| HX-305 | Merge-gate update for reconciled parity scope | CI/CD + Product | HX-303, HX-304 | Done | `make hybrid-merge-gate` enforces parity + demo evidence checks |

### Cross-Sprint Foundational Test Backlog (Always-On)

| Issue | Title | Owner | Dependencies | Status | Acceptance |
|------|-------|-------|--------------|--------|------------|
| HX-T01 | Hermetic bridge writer coverage expansion | QA/Automation | HX-102 | Done | Covers normal/timeout/disabled/empty/unicode/large payload cases |
| HX-T02 | Input reader and sentinel framing coverage | QA/Automation | HX-102 | Ready | Covers submit/whitespace/quit/malformed framing and graceful stop |
| HX-T03 | Subscriber queue ordering/retry coverage | QA/Automation | HX-103 | Ready | Proves queue ordering and retry semantics under failure and recovery |
| HX-T04 | Session wiring coverage for runtime modes | QA/Automation | HX-104 | Ready | Proves GUI-only/TUI-only/dual wiring behavior without manual inspection |

## Status Snapshot (2026-05-21)

Current state relative to recent issue verification and regression work:

- Go core build/test workflow is stable from repo root (`make build-core`, GoDog split suites).
- Startup now deterministically lands on window `0:tui-chat` with `logs` in background.
- Regression coverage exists for startup window naming and window reselection behavior.
- Health endpoint now returns runtime JSON payloads for `/health`, `/panes`, and `/applets`.
- Applet supervision now tracks lifecycle state transitions (`starting`, `ready`, `running`, `stopped`, `crashed`) and crash counts.
- Headless tmux layout validation now asserts active window selection (`0:tui-chat`), logs window presence (`1:logs`), and pane title/index ordering.
- Prompt ingress MVP is now wired from input routing through chat applet handling with deterministic chat-pane rendering.
- Input command contract now handles `:clear`, `:q`, and normal prompt forwarding with deterministic history tracking in tests.
- Completed turns are now persisted in session context with reload support across core reconstruction and query via `/context` endpoint.
- B4 merge-readiness gate is now codified as a single command (`make hybrid-merge-gate`) and enforced in CI.
- Phase 2 now keeps a persistent Python chat bridge process (`template.py --bridge-chat-server`) and reuses it across prompts.
- Phase 2 now includes initial LLM-backed bridge support via `AGENTX_CHAT_BACKEND=ollama` with deterministic fallback when Ollama is unavailable.
- Phase 2 now enforces bounded bridge response waits (`AGENTX_CHAT_BRIDGE_RESPONSE_TIMEOUT_SEC`) to prevent hung prompt routes.
- Phase 2 bridge now supports chunked response events for incremental chat-pane streaming while preserving final turn persistence.
- Phase 2 now sources chunk events from true Ollama backend streaming (not only local response tokenization) when `AGENTX_CHAT_BACKEND=ollama`.
- Phase 2 now emits bridge lifecycle events into the logs pane (`bridge_start`, `bridge_chunk`, `bridge_response_ok`, timeout/error/fallback paths).
- Phase 2 now emits compact per-turn context summaries to the context pane after successful turn persistence.
- Phase 2 mid-stream cancellation/retry flow is hardened: canceled routes propagate cancellation (no echo fallback), bridge teardown/restart is validated, and immediate retry succeeds.
- GoDog integration coverage now includes a streaming scenario that asserts chunk rendering and persisted final-turn prompt/response integrity.
- Context-pane fidelity assertions now verify bounded summary formatting and deterministic multi-turn ordering (unit + GoDog).
- GoDog observability assertions now validate bridge lifecycle sequencing across timeout, fallback, restart, and recovery.
- Fault-injection permutations now cover malformed JSON bridge frames and explicit error-frame behavior with lifecycle observability assertions (unit + GoDog).
- Backend-specific streaming edge-case assertions now cover empty chunk handling and parser-level duplicate/late-frame semantics.
- Launch UX fix: `./bin/agentx` now attaches to the tmux TUI by default for user-facing runs (`-attach=false` remains available for headless mode).
- Demo UX refinement: `./bin/agentx --demo` now opens a split tmux controller with a live core multi-pane mirror on the right; the smoke gate uses `--demo-headless` to keep artifact validation deterministic.
- Pane UX fix: applet supervisor now launches live pane applet processes for `chat`, `context`, and `input` so panes are not idle shell sessions at startup.
- Pane affordance fix: role-specific pane behavior now routes user input through core `/submit`, displays agent output in chat pane, and surfaces context metadata in context pane.
- Pane UX contract fix: interactive panes now suppress operational noise (READY payloads, IPC path diagnostics, shell echo traces) in favor of sanctioned user-facing output.
- Headless UX coverage now includes pane-affordance behavior validation (`tests/test_tmux_pane_affordances_headless.sh`) in addition to structural layout validation.
- Remaining applet fidelity work is focused on potential richer context-pane metadata views and any additional backend compatibility permutations requested.

## DemoMode Implementation Plan (Pre-UAT Gate)

Goal: provide a user-visible terminal harness (`agentx --demo`) that executes E2E scenarios and captures structured feedback after each test.

### D0 — Contract Freeze

- Define DemoMode UX contract in `docs/ux/07_DEMO_MODE.md`.
- Trace affordances in `UX_LIFECYCLE.md` as `PD-17-AF-001..006`.

### D1 — CLI Surface

- Add `--demo` flag to `agentx` command.
- Add `--demo-start <id-or-index>` selector.
- At startup, print ordered demo test sequence and active start point.

Status: COMPLETE (implemented in `cmd/agentx-core/main.go` and `cmd/agentx-core/demo_harness.go`).

### D2 — Per-Test User Feedback Loop

- Run demo tests sequentially in visible terminal.
- At end of each test, prompt user for:
  - `N` = accept and continue
  - `X` = fail and stop
- Invalid input must re-prompt without advancing.

Status: COMPLETE (implemented in `cmd/agentx-core/demo_harness.go` with unit tests in `cmd/agentx-core/demo_harness_test.go`).

### D3 — Failure Diagnostics

- On `X`, dump:
  - all pane captures
  - pane/window metadata
  - test id/title and timestamp
- Persist under deterministic `logs/demo/<session>/<test>/` artifact paths.

Status: COMPLETE (implemented in `cmd/agentx-core/demo_harness.go` with diagnostics artifact persistence).

### D4 — Automation and Readiness

- Add unit tests for selector and prompt-state logic.
- Add headless integration tests for artifact bundle creation.
- Add end summary with run totals and readiness outcome.

Status: COMPLETE (smoke gate implemented via `tests/test_demo_smoke_headless.sh` and `Makefile`).

## Execution Board (Legacy)

The legacy execution board content has been superseded by the authoritative sprintized issue backlog above.
Keep this heading only for historical continuity with prior references.

## B4 Hybrid Merge-Readiness Checklist (Authoritative)

Before opening or approving default-branch promotion for hybrid core, all items must be true in the same branch state:

- [ ] `make hybrid-merge-gate` passes locally with no modifications afterward.
- [ ] CI workflow `Hybrid Merge Readiness Gate` passes for the same commit.
- [ ] No open P1/P2 blockers remain for startup determinism, prompt path, or observability path.
- [ ] `CHANGELOG.md` and `pyproject.toml` reflect the latest delivered scope.

Authoritative gate command:

```bash
make hybrid-merge-gate
```

## Phase Checklist

### Phase 1: Foundation & MVP ✅ IN PROGRESS

Goal: Establish Go core orchestration with placeholder applets.

- [ ] **P1.1** Go core compiles and runs without errors
  - [ ] go.mod and dependencies resolved
  - [ ] main.go entry point works
  - [ ] Build script (`build_core.sh`) produces binary
  
- [ ] **P1.2** tmux session creation and layout
  - [ ] Go core creates tmux session with configurable name
  - [ ] Creates 5 panes: chat, logs, input, context, system
  - [ ] Each pane displays placeholder with pane name + emoji
  - [ ] Pane layout matches UX specification
  
- [ ] **P1.3** Python applet template
  - [ ] Template applet sends `READY` signal on startup
  - [ ] Receives environment variables correctly
  - [ ] Handles SIGTERM gracefully
  - [ ] Can be instantiated for each pane
  
- [ ] **P1.4** Graceful shutdown
  - [ ] Go core handles SIGTERM/SIGINT
  - [ ] Applet processes receive shutdown signal
  - [ ] tmux session is killed cleanly
  - [ ] Return to shell without orphaned processes
  
- [ ] **P1.5** Health endpoint
  - [ ] HTTP server on 127.0.0.1:9876
  - [ ] GET /health returns session status
  - [ ] GET /panes returns active pane list
  - [ ] GET /applets returns running applets
  
- [ ] **P1.6** Documentation
  - [ ] HYBRID_ARCHITECTURE.md complete
  - [ ] Python applet template documented
  - [ ] IPC protocol documented
  - [ ] Build instructions provided

---

### Phase 2: LLM Integration

Goal: Migrate agent logic from Python core to chat applet.

- [ ] **P2.1** Chat applet foundation
  - [ ] Chat applet receives user prompts via IPC
  - [ ] Chat applet uses OllamaClient to query LLM
  - [ ] Chat applet streams responses to Go core
  - [ ] Responses rendered in tmux chat pane
  
- [ ] **P2.2** Context & session state
  - [ ] Context manager loads/saves messages
  - [ ] Chat applet accesses session context
  - [ ] Tool definitions loaded from config
  - [ ] Conversation history persisted
  
- [ ] **P2.3** Agent loop
  - [ ] Tool invocation from chat applet
  - [ ] Tool results streamed back to chat
  - [ ] Classification (intent/thinking) displayed
  - [ ] Multi-turn agentic workflows supported

---

### Phase 3: Input & Output

Goal: Wire user input and system logs.

- [ ] **P3.1** Input applet
  - [ ] Input pane uses prompt_toolkit
  - [ ] User prompts sent to chat applet via IPC
  - [ ] Input history available (arrow keys)
  - [ ] Special commands (`:clear`, `:models`, `:q`)
  
- [ ] **P3.2** Logs applet
  - [ ] Logs pane displays system events
  - [ ] Applet crashes logged
  - [ ] IPC errors logged
  - [ ] User-visible error messages
  
- [ ] **P3.3** Context visualizer
  - [ ] Text-based context display (tables, trees, emoji)
  - [ ] Shows message count, token usage, tools
  - [ ] Pane updates on each turn
  - [ ] Color-coded for clarity

---

### Phase 4: GUI as Optional Applet

Goal: Run Tkinter GUI as independent process.

- [ ] **P4.1** GUI applet wrapper
  - [ ] Tkinter GUI runs in separate applet
  - [ ] Receives context updates via IPC
  - [ ] Can be launched from input pane
  - [ ] Can be closed without stopping agent
  
- [ ] **P4.2** GUI/TUI sync
  - [ ] Chat updates in GUI reflected in TUI
  - [ ] User input in GUI sent to agent
  - [ ] Singleton: only one GUI instance per session
  - [ ] GUI crash does not crash core
  
- [ ] **P4.3** Mode switching
  - [ ] User can toggle between pure TUI and GUI+TUI
  - [ ] State preserved during switch
  - [ ] No message loss or duplication

---

### Phase 5: Cleanup & Stabilization

Goal: Deprecate pure-Python mode.

- [ ] **P5.1** Backward compatibility
  - [ ] Old Python entry point still works (if needed)
  - [ ] Migration guide for users
  - [ ] Deprecation warnings added
  
- [ ] **P5.2** Performance optimization
  - [ ] Profile applet startup time
  - [ ] Reduce IPC latency
  - [ ] Optimize context serialization
  
- [ ] **P5.3** Testing & hardening
  - [ ] Unit tests for Go core components
  - [ ] Integration tests (core + applets)
  - [ ] Stress tests (long sessions, many turns)
  - [ ] Edge case handling (network loss, applet crash)

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Go binary** | Faster, distributable, minimal deps. Easy orchestration. |
| **Python applets** | Leverage LLM ecosystem, rapid prototyping, optional features. |
| **tmux for layout** | Terminal-native, no additional runtime, proven stability. |
| **context.Context for shutdown** | Go idiom, elegant broadcast cancellation, no goroutine leaks. |
| **FIFOs for IPC** | Simple, Unix-standard, works across processes/machines if needed. |
| **HTTP health endpoint** | Debuggable, applets can query status, decoupled from core. |

## Migration Checkpoints

Each checkpoint is a working, testable milestone:

### ✅ Checkpoint 0: Feature Branch Created

- Branch: `feat/hybrid-go-core-tui-migration`
- Go core scaffolding complete
- Python applet template created
- Architecture documented

### 🎯 Checkpoint 1: MVP Runs (Phase 1)

- Go binary compiles and runs
- tmux session creates 5 panes with placeholders
- Each applet sends `READY` signal
- Graceful shutdown works
- Health endpoint responds

**Merge to main?** No; keep on feature branch until at least Phase 2.

### 🎯 Checkpoint 2: Chat Applet Works (Phase 2)

- Chat applet integrated with OllamaClient
- User can ask questions via Go core input
- LLM responses appear in chat pane
- Context persisted

**Merge to main?** Possibly; depends on testing/stability.

### 🎯 Checkpoint 3: Full TUI Feature Parity (Phases 2-3)

- All UX affordances from existing TUI working in hybrid mode
- Classification, tool execution, context visualization
- Input pane functional
- No regressions vs current code

**Merge to main?** Yes, make this the new default.

### 🎯 Checkpoint 4: GUI Optional (Phase 4)

- Tkinter GUI runs as applet
- Can be toggled on/off without app crash
- State sync between GUI and TUI

**Merge to main?** Yes, mark GUI as experimental.

### 🎯 Checkpoint 5: Stable Release (Phase 5)

- All tests passing
- Performance acceptable
- Documentation complete
- Deprecation warnings added

**Merge to main?** Yes, release as v1.0.0-hybrid (or v0.51.0).

## Build & Test

### Building the Go Core

```bash
# First time
cd /Projects/agentX
go mod tidy
cd cmd/agentx-core
go mod download

# Build
./build_core.sh

# Run
../bin/agentx --project-dir . --user $USER
```

### Running Tests (as we add them)

```bash
# Unit tests
go test ./cmd/agentx-core/...

# Integration tests (Python applets)
python -m pytest tests/hybrid/
```

## Known Unknowns / Risks

1. **Applet startup order:** Do we need to wait for all applets before attaching to tmux?
   - Mitigation: Implement applet health polling in health endpoint.

2. **IPC reliability:** FIFOs can hang if reader/writer closes unexpectedly.
   - Mitigation: Implement timeout and retry logic.

3. **Python distribution:** How to ensure applets are found at runtime?
   - Mitigation: Package applets in Go binary (embed FS) or rely on $PATH.

4. **Context serialization:** Can context grow too large to sync via IPC?
   - Mitigation: Implement lazy loading, stream large payloads.

5. **Debugging:** How to debug crashes across Go/Python boundary?
   - Mitigation: Structured logging, central log aggregation, health endpoint.

## References

- Architecture diagram: `docs/architecture/agentx_tui_hybrid_architecture.md`
- Hybrid architecture spec: `docs/architecture/HYBRID_ARCHITECTURE.md`
- Python applet template: `applets/template.py`
- Go core main: `cmd/agentx-core/main.go`
- TUI-first design plan: `docs/ux/06_TUI_MIRROR.md`
- Existing Python code: `src/agentx/`, `src/agentix/` (reference)
