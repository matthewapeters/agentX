# Orchestration Schema Index (Authoritative)

Last updated: 2026-06-15
Owner: Architecture
Status: Authoritative schema home for orchestration design and behavior contracts.

## Purpose

This folder is the single source of truth for orchestration schemas introduced from ADR 0001-0005.
All architecture design documents and behavior specs must link to schema documents in this folder.

## Schema Set

- `request-envelope.schema.json` - Control-plane request ingress contract.
- `compiled-dag.schema.json` - Canonical compiled DAG and node taxonomy representation.
- `execution-outcome.schema.json` - Normalized execution outcomes returned by workers.
- `policy-decision.schema.json` - Hard-boundary authorization decision contract.
- `trace-event.schema.json` - Canonical traceability event envelope.
- `replay-bundle.schema.json` - Deterministic replay corpus envelope.
- `quality-gate-report.schema.json` - Gate A-E evidence and pass/fail report.

## Governance

- Changes to taxonomy fields must be reflected in the corresponding ADR-linked design docs.
- Schema changes are backward compatible unless explicitly version-bumped.
- Architecture docs under `docs/architecture/design/` must reference schema filenames and key fields.
- Behavior specs under `docs/architecture/behavior/` must map scenarios to schema fields.

Compatibility guidance:

- Contract tightening that adds new required fields or stricter conditional requirements is a breaking change for existing producers and consumers.
- For breaking schema-tightening updates, either publish a new schema ID/version path or coordinate a single cutover where all producers/validators upgrade together.
- Consumer validators SHOULD be upgraded before producer enforcement when staged rollout is required.

## Decision Notes

- Persistence direction (approved): local append-first data plane, behind an interface boundary.
- Remaining operational thresholds (SLO/fairness bounds) may be provisional until explicitly ratified.

## Contract Clarifications (2026-06-15)

- `compiled-dag.schema.json` requires both `correlation_id` and `task_id`.
- `compiled-dag.schema.json` requires canonical deterministic `graph_hash` (SHA-256 hex) for machine-checkable DAG determinism.
- `compiled-dag.schema.json` treats `metadata.compiled_at` as optional, non-semantic timestamp excluded from determinism hashes.
- `compiled-dag.schema.json` requires `reduce_contract` metadata when node `kind` is `reduce`.
- `policy-decision.schema.json` requires `constraints` when `decision=allow_with_constraints` and includes explicit `policy_default`.
- `policy-decision.schema.json` defines destructive confirmation evidence semantics and fail-closed deny behavior when required evidence is missing.
- `trace-event.schema.json` defines constrained `redaction` metadata fields and records execution-outcome to trace-status mapping.
- `replay-bundle.schema.json` includes canonical divergence report path via `divergence_report_ref`.
- `quality-gate-report.schema.json` requires explicit non-empty `thresholds` and `observed` objects for every gate entry and enforces exactly one entry for each gate ID A-E.
- `quality-gate-report.schema.json` applies gate_id-conditional required metric keys for `thresholds` and `observed` (A-E); required key set is normative while numeric threshold values may remain provisional.
- `execution-outcome.schema.json` applies status-conditional required fields (for example, `error` on failure statuses and `completed_at` on terminalized statuses).
- Terminology mapping: Gate A `fast-path` corresponds to request envelope `intent=direct`.
