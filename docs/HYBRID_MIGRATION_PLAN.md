# Hybrid Go/Python Migration Plan

_Last updated: 2026-05-22 (v0.69.0)_

## Overview

This document tracks the migration from pure-Python TUI to hybrid Go-core + Python-applets architecture.

Authoritative status source: this file is the single source of truth for hybrid migration status and checkpoints.

**Branch:** `feat/hybrid-go-core-tui-migration`

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
- Remaining applet fidelity work is focused on richer context detail views and expanded BDD streaming assertions.

## Execution Board (Next 2 Sprints)

This board converts roadmap phases into immediately actionable work with acceptance criteria and test gates.

### Sprint A (Stabilize Foundation + Observability)

1. **A1: Lock Phase-1 parity in docs**
   - Scope: Update architecture and migration docs to match current implemented startup/layout behavior.
   - Done when:
     - `docs/HYBRID_MIGRATION_PLAN.md` status and checkpoint language reflects current behavior.
     - `docs/architecture/HYBRID_ARCHITECTURE.md` pane/window descriptions match `cmd/agentx-core/core.go` behavior.
   - Verification:
     - Manual review + `git diff -- docs/`.

2. **A2: Complete health endpoint payloads**
   - Scope: Implement `GET /health`, `GET /panes`, `GET /applets` with real runtime state.
   - Done when:
     - Endpoints return deterministic JSON from active core state.
     - Error path and empty-state behavior are covered.
   - Verification:
     - `cd cmd/agentx-core && go test ./...`
     - Add/expand GoDog integration scenarios for endpoint responses.

3. **A3: Harden applet supervision lifecycle**
   - Scope: Track applet status transitions (starting/ready/stopped/crashed), expose in health, improve shutdown reliability.
   - Done when:
     - Crash and graceful-stop states are observable.
     - Shutdown leaves no orphaned applet/tmux processes in test harness.
   - Verification:
     - `cd cmd/agentx-core && go test ./...`
     - Add deterministic integration tests for crash/stop transitions.

4. **A4: Expand headless UX assertions**
   - Scope: Extend tmux headless validation to assert selected window and pane titles/ordering.
   - Done when:
     - Failure reproduces if selected window drifts from `tui-chat`.
     - CI/local `make verify-tmux-layout` enforces these assertions.
   - Verification:
     - `make verify-tmux-layout`

### Sprint B (Chat + Input Vertical Slice)

1. **B1: Chat applet request/response MVP**
   - Scope: Route one user prompt from input path to chat applet and render response in chat pane.
   - Done when:
     - Prompt ingress -> chat applet -> pane output works for a deterministic test prompt.
     - Failure paths emit actionable logs.
   - Verification:
     - `cd cmd/agentx-core && go test ./...`
     - Add GoDog integration scenario covering end-to-end prompt/response pipeline.

2. **B2: Input applet command contract**
   - Scope: Implement basic input command handling (`:clear`, `:q`) and normal prompt forwarding.
   - Done when:
     - Special commands are parsed and handled without breaking normal prompts.
     - Input history behavior is deterministic in tests.
   - Verification:
     - `cd cmd/agentx-core && go test ./...`
     - Add focused integration tests for command parsing and dispatch.

3. **B3: Context sync MVP for chat turns**
   - Scope: Persist and expose minimal conversation turn history for chat applet interactions.
   - Done when:
     - A completed turn is persisted and queryable via core state/endpoint path.
     - Restart path preserves previously written history.
   - Verification:
     - `cd cmd/agentx-core && go test ./...`
     - Add test fixture proving persistence across core restart.

4. **B4: Merge readiness gate for default-branch promotion**
   - Scope: Define and enforce a single checklist before hybrid default switch.
   - Done when:
     - Required checks are codified in docs and CI commands.
     - No open P1/P2 blockers remain for startup, prompt flow, and observability.
   - Verification:
     - `make go-test`
     - `make verify-tmux-layout`

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
