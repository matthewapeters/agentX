# Design 01: Control, Execution, and Data Planes

Last updated: 2026-06-15
ADR linkage: 0001

## Schema Links (Authoritative)

- `docs/architecture/schemas/request-envelope.schema.json`
- `docs/architecture/schemas/execution-outcome.schema.json`
- `docs/architecture/schemas/trace-event.schema.json`

## Goal

Define object-oriented contracts for strict plane ownership where control plane owns lifecycle transitions, execution plane runs side-effect constrained work, and data plane persists append-only evidence.

## Core Objects and Responsibilities

- `PlaneCoordinator`
  - Ingress for requests and cancellation.
  - Owns orchestration tick loop and dispatch intents.
- `ControlPlane`
  - Owns DAG task state transitions.
  - Evaluates guards and accepts normalized outcomes only.
- `ExecutionPlane`
  - Runs node workers by node kind.
  - Produces normalized `ExecutionOutcome` only.
- `DataPlane`
  - Appends immutable trace/artifact records keyed by correlation ID.
  - Produces replay bundles and gate evidence summaries.

## Interface Contracts (Go-style)

```go
type PlaneCoordinator interface {
    Submit(req RequestEnvelope) (TaskID, error)
    Cancel(taskID TaskID, reason string) error
    Tick(now time.Time) ([]DispatchIntent, error)
    Snapshot(taskID TaskID) (ControlSnapshot, error)
}

type ControlPlane interface {
    Compile(req RequestEnvelope) (CompiledDAG, error)
    Transition(taskID TaskID, event ControlEvent) (ControlSnapshot, error)
    EvaluateGuards(taskID TaskID, nodeID NodeID) (GuardDecision, error)
    OnExecutionOutcome(taskID TaskID, outcome ExecutionOutcome) (ControlSnapshot, error)
}

type ExecutionPlane interface {
    Register(kind NodeKind, worker NodeWorker) error
    Dispatch(intent DispatchIntent) (DispatchReceipt, error)
    Poll(runID RunID) (ExecutionOutcome, bool, error)
    Cancel(runID RunID, reason string) error
}

type DataPlane interface {
    AppendTrace(evt TraceEvent) error
    AppendArtifact(artifact ArtifactEnvelope) error
    SnapshotContext(taskID TaskID) (ContextSnapshotRef, error)
    BuildReplayBundle(taskID TaskID) (ReplayBundleRef, error)
    GateEvidence(taskID TaskID) (GateEvidenceSummary, error)
}
```

## State Machine

- `pending -> ready -> running -> succeeded`
- `running -> retriable_failure -> ready`
- `running -> terminal_failure`
- `running -> timeout`
- `* -> canceled` (on control-plane cancel)

## Invariants

- Only control plane mutates orchestration state.
- Execution workers cannot mutate DAG state directly.
- Data plane is append-first and immutable per event record.
- Fast path is represented as a valid degenerate DAG route.

Execution outcome status mapping contract:

- `success` MUST map to trace status `completed`.
- `retriable_failure` and `terminal_failure` MUST map to trace status `failed`.
- `timeout` MUST map to trace status `timed_out`.
- `canceled` MUST map to trace status `canceled`.

This mapping is authoritative for control-plane transition ingestion and data-plane trace append behavior.

## Local Append-First Direction

Approved direction: data-plane persistence starts with local append-oriented storage behind interfaces.

## Conformance Checklist

- Request ingress validates `request-envelope.schema.json`.
- Worker results validate `execution-outcome.schema.json`.
- Persisted events validate `trace-event.schema.json`.
- Cancellation and timeout produce deterministic terminal states.
