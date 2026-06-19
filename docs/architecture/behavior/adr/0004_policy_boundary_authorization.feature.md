# Feature: ADR 0004 Policy Boundary Authorization

Schema links:

- `docs/architecture/schemas/policy-decision.schema.json`
- `docs/architecture/schemas/trace-event.schema.json`

```gherkin
@adr0004 @policy @positive
Scenario: ADR-0004-AF-001 Allowed action proceeds with audit evidence
  Given an allowed action within scope
  When policy boundary authorizes request
  Then decision is allow with non-empty reason_codes and audit_id
  And execution proceeds

@adr0004 @policy @negative
Scenario: ADR-0004-AF-002 Denied action never dispatches
  Given an action on deny list
  When persona layer proposes execution
  Then policy decision is deny
  And no execution dispatch occurs

@adr0004 @policy @positive
Scenario: ADR-0004-AF-003 Constrained action executes within constraints
  Given an action requiring bounded scope
  When policy returns allow_with_constraints
  Then execution is permitted only under returned constraints

@adr0004 @policy @negative
Scenario: ADR-0004-AF-004 Destructive action requires explicit confirmation
  Given a destructive action without confirmation artifact
  When policy evaluates request
  Then decision is deny with reason_codes containing destructive_confirmation_missing
  And no execution dispatch occurs

@adr0004 @policy @security
Scenario: ADR-0004-AF-005 Sensitive payload is redacted in traces
  Given execution payload includes sensitive material
  When trace events are persisted
  Then sensitive material is redacted per policy
  And redaction metadata includes applied, rule_set_version, field_count, and categories

@adr0004 @policy @negative
Scenario: ADR-0004-AF-006 Persona customization cannot bypass hard policy
  Given different persona or skill configurations
  When same prohibited action is evaluated
  Then policy outcome remains deny unless policy rules change
```
