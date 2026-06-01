# Hybrid Runtime Split: Go Core and Go Applets

_Last updated: 2026-05-28 (v1.0.1)_

## Purpose

This document defines the authoritative contract and migration plan for the AgentX hybrid runtime: the split between the Go core orchestrator and Go applet layer. It governs the division of responsibilities, IPC boundaries, migration phases, and the thin-GUI contract. All architectural and UX parity claims must reference this document.

---

## 1. Runtime Split Overview

AgentX hybrid runtime is decomposed into two principal layers:

- **Go Core (Orchestrator):**
  - Owns tmux session, pane/window orchestration, applet lifecycle, IPC routing, and session state.
  - Responsible for launching, monitoring, and restarting Go applets.
  - Maintains authoritative pane/channel registry and runtime policy.
  - Owns prompt-classification-thinking-tool-response orchestration and applet communications.

- **Go Applets (Shared Base Architecture):**
  - Each applet is a single-purpose Go process (e.g., chat/output, input, logs,
    and the system/context widget surfaces for files, configuration, context,
    context history, working memory, and context visualizer views).
  - All applets use a shared Go base architecture for lifecycle hooks, IPC contract, health signaling, and logging behavior.
  - Applets communicate with Go core via versioned IPC (FIFO/socket/pipe).
  - Applets must be validated against UX specs via unit, integration, and functional tests.

- **GUI Surface (Secondary):**
  - GUI remains secondary and back-burnered until TUI parity completion gates are met.
  - GUI may consume core state channels but does not set runtime orchestration priority.

### 1.1 Topology Modes and Startup Surfaces

The runtime split also defines two presentation topologies for tmux-based startup.
These modes do not change orchestration ownership, IPC contracts, or the applet
runtime model.

The authoritative mode and switch catalog is maintained in
[startup_modes.md](startup_modes.md).

- **Default frame-based runtime (production / steady state):**
  - Core owns the named tmux windows and binds panes by semantic title.
  - `output`, `system`, `input`, and `logs` remain the routing contract.
  - tmuxp overlays may reshape the layout, but core re-resolves owned surfaces by
    title after startup.

- **UAT-visible startup mode (opt-in validation surface):**
  - The proposed startup switch is `--startup-mode visible-windows`, with an
    environment fallback of `AGENTX_STARTUP_MODE=visible-windows`.
  - Core starts the initial session as separate top-level applet windows instead
    of nested frames so UAT can confirm that each applet is present, running,
    and functionally responsive before frame layout work is enabled.
  - This mode is presentation-only. It must not alter applet identity, IPC
    schemas, health signaling, or state ownership.
  - If the visible-windows mode cannot be established, startup must fall back to
    the default frame-based runtime rather than blocking the session.

Implementation policy for both modes:

- Each runtime applet still uses the shared Go base architecture and must be
  validated against its UX specification before parity can be claimed.
- Applet implementation is not complete until each applet has unit coverage,
  integration coverage, and a UX traceability row that is reconciled to the
  as-built state.
- The visible-windows mode is a temporary UAT affordance, not a new runtime
  ownership model.

---

## 2. Migration Phases

| Phase | Output/System Panel Owner    | IPC Contract         | Go Core Role                                  | Applet Role                            |
| ----- | ---------------------------- | -------------------- | --------------------------------------------- | -------------------------------------- |
| 1     | Transitional mixed ownership | FIFO v2              | Orchestrate, relay, monitor                   | Mixed Python/Go implementations        |
| 2     | Go (current default path)    | FIFO v2+             | Orchestrate, own logic, own state transitions | Go applets on shared base architecture |
| 3     | Go (final)                   | IPC v3 (socket/pipe) | Orchestrate, own logic, own state             | Go applets, UX-spec parity validated   |

- **Current:** Transitional (parity incomplete)
- **Target:** Go core + Go applets, TUI-first parity complete

---

## 3. Applet and GUI Contract

- **Applet contract:**
  - Applet is a Go application leveraging the shared base architecture.
  - Applet participates in runtime flows defined by Go core orchestration.
  - Applet must satisfy UX spec behavior for its surface and pass integration + functional tests.
  - Applet must follow shared IPC/lifecycle/health/logging contracts.

- **GUI contract:**
  - GUI is secondary while TUI parity is incomplete.
  - GUI must not displace TUI-first implementation priority.
  - GUI state and affordances remain mapped to Go-core channels.

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

### 4.1 Shared Activity-State Direction (Authoritative)

To keep applet UX consistent without duplicating orchestration state, AgentX defines a shared core-owned activity-state contract.

- Source of truth: Go core prompt-cycle state.
- Session-level endpoint: `GET /activity`.
- Required semantics:
  - `state`: `idle` | `working` | `completed` | `failed`
  - `phase`: `classify` | `thinking` | `tool` | `respond` | `none`
  - `prompt_cycle`: full deterministic cycle payload.

Consumer policy:

- Input affordance widgets must use this state only for visual guidance; they must not take ownership of orchestration state transitions.
- Context-visualization surfaces should render the same state source, either directly via `/activity` or via a synchronized mirror in `/context`.
- Multiple applets may consume the same session-level activity contract concurrently.

Transport policy:

- Keep traffic session-scoped and lightweight.
- Favor one shared channel/feed over per-applet status channels.

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

## 8. Current Decision Snapshot

- The shared activity-state contract is now a required architectural direction for hybrid applet UX.
- Input widget and context visualizer are first-class consumers of the same core activity source.
- Future applets should compose onto this contract rather than introducing independent working-state protocols.
