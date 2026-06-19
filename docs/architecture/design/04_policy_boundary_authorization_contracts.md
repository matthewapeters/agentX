# Design 04: Persona, Skill Policy, and Security Boundary Contracts

Last updated: 2026-06-15
ADR linkage: 0004

## Schema Links (Authoritative)

- `docs/architecture/schemas/policy-decision.schema.json`
- `docs/architecture/schemas/trace-event.schema.json`

## Goal

Define non-bypassable policy boundary interfaces that authorize or deny execution requests independent of persona/skill customization.

## Layer Separation

- Layer A (soft): persona and skill routing behavior.
- Layer B (hard): policy/security boundary authorization.

## Core Types

```go
type ActionRequest struct {
    CorrelationID string
    TaskID        TaskID
    NodeID        NodeID
    ActorPersona  string
    ActionType    string
    ResourceScope string
    PayloadMeta   map[string]any
}

type PolicyEngine interface {
    Authorize(req ActionRequest) (PolicyDecision, error)
    Reauthorize(req ActionRequest, prior PolicyDecision) (PolicyDecision, error)
}

type BoundaryMiddleware interface {
    CheckIngress(intent DispatchIntent) (PolicyDecision, error)
    CheckPreExecute(job DispatchJob) (PolicyDecision, error)
    RecordAudit(req ActionRequest, decision PolicyDecision) error
}
```

## Decision Contract

- `allow`
- `deny`
- `allow_with_constraints`

All outcomes must include reason codes and audit ID per `policy-decision.schema.json`.

`allow_with_constraints` semantics are strict:

- `constraints` MUST be present and non-empty when `decision=allow_with_constraints`.
- Execution MUST enforce each returned constraint before dispatch/execute transitions are accepted.
- If constraints are missing, malformed, or not enforceable, boundary middleware MUST downgrade decision to `deny` (fail closed).

Policy default behavior:

- Default policy for unresolved or ambiguous destructive authorization is `deny` unless explicitly overridden by deployment policy.
- Policy decisions MUST carry `policy_default` to make the applied default explicit in audit trails.

Destructive confirmation evidence contract:

- For destructive actions, `policy-decision.schema.json.destructive_confirmation.required` MUST be `true`.
- Confirmation evidence is valid only when `destructive_confirmation.evidence_present=true` and `evidence_ref`, `confirmed_at`, and `confirmed_by` are present.
- If destructive confirmation is required but evidence is absent or unverifiable, decision MUST be `deny` with reason code `destructive_confirmation_missing` (fail closed).
- Non-destructive actions MAY omit `destructive_confirmation`; if provided, it must remain internally consistent.

## Security Invariants

- Persona output never bypasses policy.
- Denied operations never dispatch.
- Constrained operations execute only under enforced constraints.
- Destructive operations require explicit confirmation evidence.
- Missing or ambiguous policy rule resolution defaults to deny unless override is explicitly configured.

## Conformance Checklist

- Allow/deny/constraint paths are covered.
- Audit records exist for each decision.
- Sensitive outputs are redaction-safe in trace events.
- `allow_with_constraints` paths assert non-empty constraints and enforcement success before execution.
