# Hybrid Runtime Split: Go Core vs Python Applet

_Last updated: 2026-05-16 (v0.84.1)_

## Purpose

This document defines the authoritative contract and migration plan for the AgentX hybrid runtime: the split between the Go core orchestrator and Python applet layer. It governs the division of responsibilities, IPC boundaries, migration phases, and the thin-GUI contract. All architectural and UX parity claims must reference this document.

---

## 1. Runtime Split Overview

AgentX hybrid runtime is decomposed into two principal layers:

- **Go Core (Orchestrator):**
  - Owns tmux session, pane/window orchestration, applet lifecycle, IPC routing, and session state.
  - Responsible for launching, monitoring, and restarting Python applets.
  - Maintains authoritative pane/channel registry and runtime policy.
  - Will own all output/system panel logic after migration.

- **Python Applets:**
  - Each applet is a single-purpose TUI or GUI process (e.g., chat/output, input, system/logs, context viz, navigation).
  - Communicate with Go core via IPC (FIFO, socket, or pipe; protocol versioned).
  - Initially own all output/system panel logic (pre-migration), but will become thin renderers only.
  - Tkinter GUI applet is a singleton, relaunchable, and independent of TUI applets.

---

## 2. Migration Phases

| Phase | Output/System Panel Owner | IPC Contract | Go Core Role | Python Applet Role |
|-------|--------------------------|--------------|--------------|--------------------|
| 1     | Python                   | FIFO v1      | Orchestrate, relay | Full logic, rendering |
| 2     | Python                   | FIFO v2      | Orchestrate, relay, monitor | Full logic, rendering |
| 3     | Go (target)              | FIFO v2+     | Orchestrate, own logic, relay | Thin renderer only |
| 4     | Go (final)               | IPC v3 (socket/pipe) | Orchestrate, own logic, own state | Thin renderer only |

- **Current:** Phase 2 (Python owns all output/system logic; Go orchestrates, relays, and monitors applets)
- **Target:** Phase 3+ (Go core owns all output/system logic; Python applets are thin renderers)

---

## 3. Thin-GUI/Applet Contract

- **Applet contract:**
  - Applet receives only minimal rendering instructions and data from Go core.
  - No business logic, state management, or tool execution in applet.
  - All event-driven logic, tool orchestration, and state transitions are owned by Go core.
  - Applet must render exactly as instructed and emit only user input events.

- **GUI contract:**
  - Tkinter GUI applet is launched, monitored, and (if needed) relaunched by Go core.
  - GUI applet must not persist state outside of rendering; all state is owned by Go core.
  - GUI affordances (tabs, panels, context, etc.) are mapped 1:1 to Go core channels.

---

## 4. IPC Boundary and Protocol

- **IPC channel:** Each applet has a dedicated IPC channel (FIFO, socket, or pipe).
- **Protocol:**
  - Versioned, line-oriented or framed (TBD in v3)
  - All messages are JSON objects with `type`, `payload`, and `channel` fields
  - All event types and schemas are governed by the [channel registry](channel_registry.md)
- **Upgrade path:**
  - FIFO v1: unidirectional, no framing, no versioning (legacy)
  - FIFO v2: bidirectional, versioned, JSON-framed (current)
  - IPC v3: socket/pipe, multiplexed, versioned, JSON-framed (target)

---

## 5. Migration Policy and Traceability

- All migration steps must be reflected in this document, the channel registry, and the architecture index.
- No output/system panel logic may be migrated to Go core without updating this doc and the channel registry.
- All tests must be updated to cover the Go-driven path before any parity claim.
- All GUI affordances must be mapped to Go core channels and documented in the channel registry.

---

## 6. Code Links

- Go core orchestrator: `cmd/agentx-core/`
- Applet template: `applets/template.py`
- Channel registry: [channel_registry.md](channel_registry.md)
- TUI event subscriber: `src/agentx/integration/tui_event_subscriber.py`
- Event broker: `src/agentx/event_broker.py`

---

## 7. Change Policy

- This document is the single source of truth for runtime split and migration status.
- All changes must be reflected in the architecture index and referenced in the UX_LIFECYCLE and plan docs.
- No code or test may claim Go-driven parity until this doc, the channel registry, and all tests are updated.
