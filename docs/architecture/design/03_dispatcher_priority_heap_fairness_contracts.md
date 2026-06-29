# Design 03: Dispatcher Priority Heap and Fairness Contracts

Last updated: 2026-06-15
ADR linkage: 0003

## Schema Links (Authoritative)

- `docs/architecture/schemas/execution-outcome.schema.json`
- `docs/architecture/schemas/trace-event.schema.json`
- `docs/architecture/schemas/quality-gate-report.schema.json`

## Goal

Define scheduler interfaces and fairness behavior for channelized dispatch over a single-owner priority heap.

## Channels

- `interactive`
- `orchestration`
- `maintenance`

## Terminology Mapping

- `request-envelope.schema.json.intent=direct` maps to dispatcher `interactive` channel and Gate A fast-path SLO checks.
- Design shorthand `fast-path` is an execution-profile label for direct intent traffic; schema authority remains `intent=direct`.
- `tool_assisted` and `high_assurance` intents typically map to `orchestration` (or `maintenance` for deferred/background work) by policy and scheduler classification.

## Core Types

```go
type DispatchJob struct {
    JobID           string
    TaskID          TaskID
    NodeID          NodeID
    Channel         ChannelClass
    BasePriority    int
    EnqueueTime     time.Time
    Deadline        *time.Time
    Attempt         int
    EstimatedTokens int
}

type HeapScore struct {
    PriorityScore  float64
    AgeBoost       float64
    DeadlineBoost  float64
    AttemptPenalty float64
    Total          float64
}

type Scheduler interface {
    Enqueue(job DispatchJob) error
    Dequeue(now time.Time) (DispatchJob, HeapScore, bool)
    ObserveOutcome(jobID string, outcome ExecutionOutcome) error
    Snapshot() SchedulerSnapshot
}
```

## Fairness Rules

- Aging increases effective priority for waiting jobs.
- Each channel receives bounded service windows (no starvation).
- Retry jobs re-enter with jittered backoff and capped attempts.
- Budget guards cap channel-level concurrency and token use.

## Provisional Threshold Notes

Until ratified numerically, treat thresholds as provisional and keep them explicit in gate evidence (`quality-gate-report.schema.json`).

### Provisional Defaults (Pending Ratification)

These defaults are implementation-ready starting points and MUST be emitted in Gate B `thresholds` unless explicitly overridden by deployment policy:

Schema key contract alignment (`quality-gate-report.schema.json`):

- Gate B `thresholds.max_wait_ms_p95`, `thresholds.min_service_share`, `thresholds.starvation_max_consecutive_misses`, `thresholds.retry_jitter_ratio_max` are required.
- Gate B `observed.max_wait_ms_p95`, `observed.service_share`, `observed.starvation`, `observed.retry` are required.
- Nested channel/object fields shown below are canonical for producer interoperability.

| Metric | Provisional default | Gate field |
| --- | ---: | --- |
| Max wait p95 (interactive) | 250 ms | `observed.max_wait_ms_p95.interactive` |
| Max wait p95 (orchestration) | 2,000 ms | `observed.max_wait_ms_p95.orchestration` |
| Max wait p95 (maintenance) | 10,000 ms | `observed.max_wait_ms_p95.maintenance` |
| Min service share over rolling window (interactive/orchestration/maintenance) | 0.50 / 0.35 / 0.15 | `observed.service_share` |
| Starvation max consecutive misses per active channel | 3 windows | `observed.starvation.max_consecutive_misses` |
| Retry backoff jitter | +/-20% | `observed.retry.jitter_ratio` |

Override policy:

- Source of truth order: deployment policy config > environment override > provisional defaults in this document.
- Any override MUST be trace-visible in Gate B `thresholds` and include provenance metadata in gate evidence.
- If no explicit override is provided, scheduler MUST use the provisional defaults above.
- For ambiguous or missing threshold configuration, fail closed by using documented defaults; do not infer permissive values implicitly.

## Conformance Checklist

- Dispatch trace includes score components and selection reason.
- Fairness tests assert no starvation violations.
- Retry and budget behavior are trace-visible.
- Gate B records explicit `thresholds` and `observed` objects for every run.
