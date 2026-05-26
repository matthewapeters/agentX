# AgentX TUI Integration Completion Plan

_Last updated: 2026-05-24_

> Status: Supporting brief only.
>
> The authoritative execution plan, sprint backlog, owners, and dependency graph now live in `docs/HYBRID_MIGRATION_PLAN.md`.
> If any content here conflicts with that file, `docs/HYBRID_MIGRATION_PLAN.md` wins.

## Purpose

This plan closes the remaining gap between the hybrid Go-core runtime and the user-facing TUI experience. The current codebase already has a real bridge/subscriber path, tmux-based demo harnesses, and a split between GUI and TUI responsibilities, but the TUI still needs feature parity with the GUI and stronger evidence from hermetic tests, integration tests, and demo-driven E2E coverage.

Operational note: this document is intentionally non-authoritative and should be treated as context for implementation details. All status and planning updates must be made in `docs/HYBRID_MIGRATION_PLAN.md`.

## What Already Exists

- Go core orchestration and demo harness entry points in [cmd/agentx-core/main.go](../cmd/agentx-core/main.go) and [cmd/agentx-core/demo_harness.go](../cmd/agentx-core/demo_harness.go).
- TUI output plumbing in [src/agentx/integration/tui_bridge.py](../src/agentx/integration/tui_bridge.py) and event fan-out in [src/agentx/integration/tui_event_subscriber.py](../src/agentx/integration/tui_event_subscriber.py).
- GUI presentation surface and protocol boundary in [src/agentx/gui/gui_manager.py](../src/agentx/gui/gui_manager.py) and [src/agentx/igui_manager.py](../src/agentx/igui_manager.py).
- Core streaming and event publication in [src/agentx/streaming_controller.py](../src/agentx/streaming_controller.py).
- Existing test scaffolding in [tests/test_tui_bridge_output.py](../tests/test_tui_bridge_output.py), [tests/test_event_broker_pubsub.py](../tests/test_event_broker_pubsub.py), [tests/test_tui_emoji_regression.py](../tests/test_tui_emoji_regression.py), [tests/test_session_gui_integration.py](../tests/test_session_gui_integration.py), [tests/test_tmux_layout_headless.sh](../tests/test_tmux_layout_headless.sh), [tests/test_tmux_pane_affordances_headless.sh](../tests/test_tmux_pane_affordances_headless.sh), and [tests/test_demo_smoke_headless.sh](../tests/test_demo_smoke_headless.sh).

## Completion Criteria

The integration is complete only when all of the following are true:

1. The TUI mirrors the GUI at the semantic level for a full prompt lifecycle: startup, user input, assistant response, thinking, tool calls, tool results, errors, interrupts, and end-of-turn markers.
2. The TUI exposes the same actionable state as the GUI for the parts that matter to the user: context/history summaries, attachments, plan/status visibility, and prompt control affordances.
3. The TUI behavior is proven by hermetic unit tests, integration tests, and E2E tests that can be exercised through the demo harness.
4. Demo-mode output makes the TUI behavior demonstrable without manual inspection of implementation details.

## Workstreams

### 1. Semantic Parity Workstream

Goal: make the TUI behave like the GUI for the user-facing flow, even if the rendering primitives differ.

Scope:

- Align TUI output records with the same event model used by the GUI.
- Ensure the TUI subscriber formats the same turn lifecycle markers as the GUI display path.
- Close gaps in startup notices, user/assistant headers, thinking stream, tool call/result rendering, and turn completion.
- Decide the minimum viable representation for GUI-only concepts such as context/history summaries, attachments, and plan/status surface so the TUI is not missing critical information.

Deliverables:

- A documented TUI/GUI parity matrix that states what is mirrored 1:1 and what is intentionally simplified.
- A single canonical set of message and state transitions for both surfaces.
- A clear rule for what happens when TUI and GUI are both enabled at the same time.

Acceptance checks:

- A prompt can move through the same visible lifecycle in both surfaces.
- The TUI does not lose or reorder visible semantic events.
- The TUI exposes a consistent end-of-turn and error path.

### 2. Runtime and Wiring Workstream

Goal: make the hybrid runtime robust enough that the TUI is not just present, but actually integrated into session startup, shutdown, and control flow.

Scope:

- Confirm the Go core and Python session boundaries remain stable while the TUI is enabled.
- Ensure TUI startup, shutdown, and reconnect behavior are deterministic.
- Keep input/output FIFO or IPC wiring session-scoped and testable.
- Make launch behavior explicit so operators can tell whether they are in GUI-only, TUI-only, or dual-surface mode.

Deliverables:

- A single source of truth for the active runtime mode.
- Deterministic launch/stop behavior for TUI sessions.
- Clear runtime logging that identifies the TUI session lifecycle.

Acceptance checks:

- Starting a session creates the expected TUI surfaces and control channels.
- Stopping a session cleans up the TUI surfaces without orphaned panes or blocking readers/writers.
- Reconnect or restart behavior is predictable and documented.

### 3. Test Pyramid Workstream

Goal: build evidence in the right order, from cheap and hermetic to end-to-end.

#### 3.1 Hermetic Unit Tests

Add or expand unit tests for:

- TUI formatting of every event type that matters to the user.
- FIFO/IPC writer behavior under normal, timeout, disabled, empty, unicode, and large-payload conditions.
- Input reader behavior for submit, whitespace discard, quit, and malformed input.
- Event subscriber queue behavior, ordering, and retry semantics.
- Session wiring behavior when GUI is disabled, TUI is enabled, or both are enabled.

Primary targets:

- [tests/test_tui_bridge_output.py](../tests/test_tui_bridge_output.py)
- [tests/test_event_broker_pubsub.py](../tests/test_event_broker_pubsub.py)
- [tests/test_tui_emoji_regression.py](../tests/test_tui_emoji_regression.py)
- any focused session wiring tests that cover the runtime mode split

Exit criteria:

- The TUI bridge and subscriber are covered for the common and failure paths that can be tested without tmux or Neovim.
- The tests verify actual state transitions, not just that a method was called.

#### 3.2 Integration Tests

Add or expand tests that verify the TUI path across components:

- StreamingController to event broker to TUI subscriber.
- Session lifecycle to TUI bridge wiring.
- GUI and TUI coexistence when both are enabled.
- Prompt submission path from input surface to session and back out through the visible response stream.

Primary targets:

- [tests/test_session_gui_integration.py](../tests/test_session_gui_integration.py)
- [tests/test_tui_bridge_output.py](../tests/test_tui_bridge_output.py)
- [tests/test_event_broker_pubsub.py](../tests/test_event_broker_pubsub.py)

Exit criteria:

- The test suite proves that the stream reaches the TUI path end-to-end within the Python process boundary.
- Integration tests verify the parity contract, not just component-local behavior.

#### 3.3 E2E and Demo Harness Tests

Use tmux and the demo harness to prove the TUI path in a user-visible way.

Scope:

- Validate the tmux layout and pane titles by the authoritative runtime contract.
- Validate pane affordances from the outside in: prompt in, response out, navigation, cleanup, and restart.
- Extend the demo harness with stories that exercise the TUI-specific parity claims.
- Keep the demo harness output deterministic enough that failures are diagnosable from captured artifacts.

Primary targets:

- [tests/test_tmux_layout_headless.sh](../tests/test_tmux_layout_headless.sh)
- [tests/test_tmux_pane_affordances_headless.sh](../tests/test_tmux_pane_affordances_headless.sh)
- [tests/test_demo_smoke_headless.sh](../tests/test_demo_smoke_headless.sh)
- [cmd/agentx-core/demo_harness.go](../cmd/agentx-core/demo_harness.go)
- [cmd/agentx-core/demo_harness_test.go](../cmd/agentx-core/demo_harness_test.go)

Exit criteria:

- The demo harness can demonstrate the TUI flow without manual debugging of code paths.
- Headless tests prove the same behavior from tmux pane state and captured output.
- Demo failures produce actionable artifacts rather than ambiguous timeouts.

## Recommended Delivery Order

1. Freeze the parity contract first. Decide exactly which GUI affordances must appear in the TUI and which may be simplified.
2. Close any semantic gaps in the TUI subscriber and streaming controller before adding more tests.
3. Expand hermetic unit tests until the bridge and subscriber are stable under normal and failure conditions.
4. Add or tighten integration tests around session wiring and prompt lifecycle.
5. Extend the tmux and demo harness tests last, after the Python-side behavior is stable.
6. Use the demo harness as the final proof that the TUI is actually usable, not merely wired.

## Definition of Done

The migration is not complete until:

- The TUI reaches GUI-equivalent semantic behavior for the main chat workflow.
- The TUI exposes the user-visible state that matters for normal operation.
- Hermetic unit tests cover the TUI bridge, subscriber, and session wiring.
- Integration tests prove end-to-end delivery inside the Python runtime.
- E2E/demo tests prove the tmux-driven user experience and can be shown through the harness.

## Notes

- Prefer pane-title and geometry assertions over brittle pane-index assumptions in tmux tests.
- Keep the demo harness stories aligned with the user-visible parity matrix so the harness stays a proof tool, not just a smoke test.
- Update the authoritative migration docs if the runtime mode or parity contract changes.
