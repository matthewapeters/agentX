# ADR 0006: Persona Identification and Context Loading

Status: Accepted
Date: 2026-06-23
Deciders: AgentX architecture owners

## Context

Behavior specifications and traceability already define ADR-0006 scenarios for persona identification, expert selection, and deterministic context loading.

In this branch snapshot, those behavior contracts existed without a corresponding ADR decision record, leaving governance traceability incomplete.

## Decision

Adopt persona identification and context loading as a first-class architecture decision, with deterministic loading and explicit fail-closed behavior.

Persona identification and expert selection:

- Persona IDs must resolve to explicit persona definitions in `.agentx/agents/`.
- Missing persona definitions are hard failures with deterministic reason codes.
- No silent persona invention or implicit expert substitution is allowed.

Instruction loading and context assembly:

- Required instruction files are loaded before expert invocation.
- Instruction content forms immutable Layer 0 context.
- Missing required instructions fail closed with explicit reason codes.

Deterministic context preparation:

- Only enabled user/assistant messages are included by default.
- System/internal messages are excluded unless explicitly required by policy.
- Token budget truncation and context fingerprints are deterministic for identical inputs.

Policy boundary coupling:

- Policy checks execute before expert invocation and before tool dispatch.
- Deny/constraint outcomes are recorded with explicit reason codes and audit linkage.

Traceability and replay evidence:

- Expert return packets include persona ID, skill ID (if any), context fingerprint, policy decision, and audit linkage fields.
- Instruction hash and context fingerprint are included in replay parity evidence.

## Consequences

Positive:

- Restores ADR-to-behavior traceability for ADR-0006 scenarios.
- Makes persona routing and context assembly auditable and replayable.
- Reduces risk of silent behavior drift across instruction/policy layers.

Trade-offs:

- More strict validation can convert previously permissive behavior into explicit failures.
- Requires maintenance of instruction/persona registries and reason-code taxonomy.

Operational implications:

- CI and release gates should continue validating deterministic context behavior.
- Instruction and persona assets must be versioned as architecture-significant inputs.

## Next Steps

1. Keep behavior scenarios and ADR references synchronized for ADR-0006 updates.
2. Maintain reason-code mappings for persona, instruction, and policy failures.
3. Validate replay parity with instruction hash + context fingerprint in gate runs.
4. Include ADR-0006 linkage in future persona/context-related implementation PRs.
