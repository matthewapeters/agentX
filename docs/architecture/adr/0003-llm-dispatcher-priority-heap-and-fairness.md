# ADR 0003: LLM Dispatcher Priority Heap and Fairness

Status: Accepted
Date: 2026-06-14
Deciders: AgentX architecture owners

## Context

As orchestration complexity increases, LLM calls compete with tool-bound tasks and interactive user requests. The dispatcher needs predictable latency for user-facing flows while preventing starvation of background work.

Current execution behavior is mostly sequential and lacks explicit fairness semantics. A queue-only model is insufficient once multiple channels and priority classes are active.

## Decision

Adopt channelized dispatch with a single-owner priority heap scheduler.

Channels:

- interactive: user-visible direct responses and final synthesis steps.
- orchestration: planner, decomposition, and reduce steps for active DAGs.
- maintenance: low-priority replay, indexing, summarization, and diagnostics.

Scheduler model:

- One dispatcher owner maintains a single priority heap over all ready jobs.
- Heap key includes: priority class, effective age, deadline proximity, and attempt count.
- Selection uses weighted fairness across channels so interactive work gets lower latency while orchestration and maintenance still make progress.

Fairness rules:

- No starvation: each channel receives service within bounded selection windows.
- Aging: waiting jobs gain effective priority over time.
- Budget guard: channel-level concurrency and token budgets limit runaway consumption.

Failure and retry policy:

- Retries are re-enqueued with jittered backoff and capped attempts.
- Terminal failures emit structured events for gate evaluation and replay.

## Consequences

Positive:

- Keeps simple Q/A responsive under load.
- Provides deterministic, debuggable scheduling behavior for complex DAG execution.
- Enables policy controls through channel budgets and priority classes.

Trade-offs:

- Introduces scheduler complexity and tuning requirements.
- Incorrect weighting can either starve maintenance work or degrade interactive latency.

Operational implications:

- Metrics required: queue depth, dispatch delay, per-channel throughput, starvation violations.
- Trace records must include dispatch reason and heap score components.

## Next Steps

1. Define channel taxonomy and default weights in configuration.
2. Implement single-owner heap dispatcher behind feature flag.
3. Add fairness invariants and starvation tests.
4. Add per-channel telemetry dashboards and alert thresholds.
5. Gradually shift existing LLM call sites to dispatcher API.
