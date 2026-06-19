# ADR 0005: Traceability, Replay, and Quality Gates

Status: Accepted
Date: 2026-06-14
Deciders: AgentX architecture owners

## Context

AgentX orchestration is moving from mostly linear flows to graph execution with retries, channelized scheduling, and policy checks. Without rigorous traceability and replay, regressions are hard to diagnose and architecture claims become unverifiable.

Existing logs and diagnostics provide partial visibility, but gate criteria and replay expectations are not yet standardized as release requirements.

## Decision

Make traceability, deterministic replay, and quality gates first-class architecture requirements.

Traceability requirements:

- Every request and node action has a correlation ID and deterministic node/run IDs.
- Capture event stream covering classify, dispatch, execute, policy, persist, and respond stages.
- Store enough payload metadata to explain decisions without storing unnecessary sensitive content.

Replay requirements:

- Support offline replay of orchestration decisions using recorded events and normalized outcomes.
- Replay mode must reproduce node ordering, retries, policy outcomes, and final status transitions for the same seed/config snapshot.
- Replay output is used for regression detection and incident RCA.

Quality gates:

- Gate A: fast-path latency and success-rate SLOs remain within threshold.
- Gate B: no starvation violations in dispatcher fairness tests.
- Gate C: policy enforcement tests pass for deny/allow/constraint scenarios.
- Gate D: replay parity tests pass on representative scenario corpus.
- Gate E: trace completeness checks pass (required fields and linkage integrity).

## Consequences

Positive:

- Converts architecture decisions into testable release criteria.
- Improves incident triage speed and confidence in phased rollout.
- Provides auditable evidence for policy and scheduling behavior.

Trade-offs:

- Increased storage and test maintenance overhead.
- Replay determinism can constrain some implementation shortcuts.

Operational implications:

- Need retention policy for trace artifacts and a redaction strategy.
- Need scenario corpus curation process for replay parity gates.

## Next Steps

1. Define canonical trace schema and required field set.
2. Implement replay harness for compiled DAG scenarios.
3. Add CI gate checks for fairness, policy, replay parity, and trace completeness.
4. Publish SLO thresholds and exception handling process.
5. Add release checklist entries referencing all gate IDs.
