# ADR 0001: Orchestrator Control, Execution, and Data Planes

Status: Accepted
Date: 2026-06-14
Deciders: AgentX architecture owners

## Context

AgentX needs a practical orchestration model that keeps the current simple Q/A fast path while scaling to multi-step DAG execution and richer tool use. Existing architecture documents establish runtime split and channel boundaries, but orchestration responsibilities are still described across multiple files and plans.

Without explicit plane separation, ownership drift appears in three areas:

- Control flow decisions (routing, prioritization, retries) are mixed with execution details.
- Execution workers can accidentally mutate shared orchestration state.
- Data products (trace, context, memory snapshots) are inconsistently captured and hard to replay.

## Decision

Adopt a three-plane orchestration model with strict ownership and interfaces:

1. Control Plane

- Single source of truth for task lifecycle, route selection, DAG state transitions, cancellation, deadlines, and quality gate checks.
- Implemented in orchestrator logic owned by Go core runtime policy.
- Emits deterministic state transitions and dispatch intents over named channels.

1. Execution Plane

- Runs units of work selected by the control plane: LLM calls, tool calls, applet actions, and adapters.
- Execution workers are side-effect constrained: no direct mutation of control-plane graph state.
- Returns normalized outcomes: success, retriable failure, terminal failure, timeout, canceled.

1. Data Plane

- Captures immutable execution evidence and derived state artifacts.
- Includes prompt/response metadata, tool I/O envelopes, context snapshots, memory mutations, and lifecycle events.
- Provides replay-ready records and gate-evidence summaries.

Required invariants:

- Fast path remains: direct Q/A classification can execute as a degenerate one-node DAG.
- Control plane is the only owner of orchestration state transitions.
- Data plane writes are append-oriented and correlation-ID keyed.

## Consequences

Positive:

- Enables phased rollout from direct flows to complex DAGs without replacing runtime split contracts.
- Makes failure handling and replay deterministic by separating decision and execution concerns.
- Improves debuggability because every transition has evidence.

Trade-offs:

- Requires interface contracts and adapters for legacy components that currently blend concerns.
- Adds up-front schema discipline for events and trace payloads.

Operational implications:

- Channel registry must map plane-specific event types and ownership.
- Runtime policy docs must treat control-plane state as authoritative.

## Next Steps

1. Define control-plane event/state schema and ID propagation contract.
2. Add execution outcome normalization wrapper for existing tool and LLM calls.
3. Define data-plane append schema for replay and gate evidence.
4. Migrate current fast path to explicit one-node DAG representation without behavior change.
5. Add architecture conformance checks to PR review templates.
