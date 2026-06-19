# ADR 0004: Persona, Skill Policy, and Security Boundaries

Status: Accepted
Date: 2026-06-14
Deciders: AgentX architecture owners

## Context

AgentX supports role/persona specialization and skill-based execution. This increases delivery quality but also expands policy and security risk if persona customization can bypass hard controls.

Architecture needs a clear distinction between customizable behavior and non-bypassable policy enforcement. Existing docs mention constraints but do not define an explicit hard boundary contract for orchestration.

## Decision

Adopt a two-layer behavior model with hard policy boundaries.

Layer A: Persona and skill customization (soft behavior)

- Allows task decomposition style, explanation style, planning depth, and specialist routing preferences.
- Can propose tool usage but cannot directly invoke blocked operations.
- Can be versioned and swapped per mode.

Layer B: Policy and security boundary (hard behavior)

- Non-bypassable controls executed before and during action dispatch.
- Enforces tool allow/deny lists, path and workspace boundaries, secret handling rules, destructive-action confirmation requirements, and sensitive-output controls.
- Owns final authorization decision for every execution request.

Boundary contract:

- Persona/skill layer outputs intent plus rationale.
- Policy layer returns allow, deny, or allow-with-constraints plus audit reason.
- Execution only proceeds on allow states.

Security principles:

- Least privilege for tool and file access.
- Explicit confirmation for irreversible destructive actions.
- Full audit trail for denied and constrained actions.

## Consequences

Positive:

- Enables rich persona customization without weakening security posture.
- Makes policy outcomes transparent and auditable.
- Reduces risk of prompt-level policy drift across modes.

Trade-offs:

- Some user requests may feel slower due to authorization checkpoints.
- Requires upfront policy rule definitions and lifecycle ownership.

Operational implications:

- Need policy test matrix for each persona mode.
- Need stable audit schema tying policy outcomes to trace IDs.

## Next Steps

1. Publish policy decision table (action, scope, preconditions, decision).
2. Implement policy enforcement middleware on dispatcher ingress.
3. Add deny-path and allow-with-constraints tests for high-risk tools.
4. Add audit reason codes and operator-facing diagnostics.
5. Add persona-mode conformance checks in architecture reviews.
