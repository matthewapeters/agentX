# Design 05: Traceability, Replay, and Quality Gate Schemas

Last updated: 2026-06-15
ADR linkage: 0005

## Schema Links (Authoritative)

- `docs/architecture/schemas/trace-event.schema.json`
- `docs/architecture/schemas/replay-bundle.schema.json`
- `docs/architecture/schemas/quality-gate-report.schema.json`

## Goal

Define schema-driven traceability, deterministic replay, and quality gate evidence contracts.

## Trace Requirements

Every orchestration stage emits trace events with required linkage:

- `correlation_id`
- `task_id`
- `node_id`
- `run_id`
- `attempt`
- `stage`
- `status`

## Replay Contract

Replay uses `replay-bundle.schema.json` with:

- Config fingerprint
- Seed
- Ordered event stream
- Expected final state
- Canonical divergence report location via `divergence_report_ref`
- Optional provisional drift allowlist for non-functional fields

Replay divergence reporting:

- Divergence artifacts are part of the replay bundle contract and MUST be referenced by `replay-bundle.schema.json#/properties/divergence_report_ref`.
- `divergence_report_ref` is repository-relative and stable for CI artifact publishing.

Execution outcome to trace status mapping:

- `ExecutionOutcome.status=success` -> `TraceEvent.status=completed`
- `ExecutionOutcome.status=retriable_failure` -> `TraceEvent.status=failed`
- `ExecutionOutcome.status=terminal_failure` -> `TraceEvent.status=failed`
- `ExecutionOutcome.status=timeout` -> `TraceEvent.status=timed_out`
- `ExecutionOutcome.status=canceled` -> `TraceEvent.status=canceled`

## Quality Gate Contract

`quality-gate-report.schema.json` must record:

- Gate A: fast-path latency/success SLO
- Gate B: dispatcher starvation/fairness
- Gate C: policy deny/allow/constraints matrix
- Gate D: replay parity
- Gate E: trace completeness/linkage integrity

Enforceable completeness rules:

- `gates` MUST contain exactly 5 entries.
- Gate IDs MUST be unique and include each of `A`, `B`, `C`, `D`, and `E` exactly once.
- For every gate entry, both `thresholds` and `observed` objects are required and non-empty.

Gate-specific metric key contracts (schema-enforced):

- Gate A (`fast-path` / `intent=direct`):
  - `thresholds` requires `latency_p95_ms`, `success_rate_min`
  - `observed` requires `latency_p95_ms`, `success_rate`
- Gate B (dispatcher fairness/starvation):
  - `thresholds` requires `max_wait_ms_p95`, `min_service_share`, `starvation_max_consecutive_misses`, `retry_jitter_ratio_max`
  - `observed` requires `max_wait_ms_p95`, `service_share`, `starvation`, `retry`
- Gate C (policy boundary):
  - `thresholds` requires `deny_rate_max`, `constraint_coverage_min`
  - `observed` requires `deny_rate`, `constraint_coverage`
- Gate D (replay parity):
  - `thresholds` requires `parity_rate_min`, `divergence_count_max`
  - `observed` requires `parity_rate`, `divergence_count`
- Gate E (trace linkage integrity):
  - `thresholds` requires `required_linkage_fields`, `max_missing_linkage`
  - `observed` requires `linkage_coverage`, `missing_linkage_count`

Provisional status:

- Required metric keys are normative (machine-checked by schema).
- Numeric threshold values remain provisional until ratified; producers MUST still emit explicit numeric values per gate run.

## Terminology Mapping

- Gate A `fast-path` coverage maps to `request-envelope.schema.json.intent=direct` traffic.
- Reports may continue to label Gate A as `fast-path` for continuity, but schema-linked classification authority is `intent=direct`.

## Local Append-First Evidence Flow

- Trace events are append-written locally first.
- Replay bundles and gate reports are generated from local evidence.
- Interface boundary keeps storage backend replaceable later.

## Conformance Checklist

- Required trace fields present and linked.
- Replay determinism checked against expected final state.
- Gate report has explicit threshold and observed fields for all gates.
- Replay bundle includes `divergence_report_ref` and parity diagnostics resolve via canonical path.
