# Test and Documentation Contract (v1)

## Purpose

Define mandatory engineering quality gates for behavior-driven testing and documentation-first delivery.

## v1 Policy Decisions

1. Godog is the required testing framework for v1.
2. All tests must be Gherkin-based and written as behavioral scenarios.
3. Every function must have explicit behavior expectations documented using GIVEN/WHEN/THEN language before implementation.
4. CI must block merges when behavior traceability or required Gherkin contracts are missing.

## Godog Testing Standard

Required for all test suites:

- Use Godog feature files for behavior specifications.
- Each scenario must identify:
  - use-case ID
  - variant ID (if applicable)
  - related UX or architecture contract reference
- Scenario naming must be deterministic and searchable.

Required tags (minimum):

- @unit, @integration, @e2e, or @contract
- @ux:<affordance-id> when linked to UX behavior
- @arch:<contract-id> when linked to architecture contract

## Documentation-First Contract

Before writing code:

1. Document expected behavior in implementation docs using GIVEN/WHEN/THEN.
2. Link behavior to source contracts in:
   - ../ux/
   - ../architecture/
   - ../implementation/
3. Define acceptable variants and failure paths.

Function-level expectation policy (v1):

- All functions must have documented behavior expectations.
- Internal helper functions are not exempt.
- Ambiguity is treated as a defect in specification quality.

## Traceability Requirements

Every implemented behavior must be traceable across:

- UX behavior contract
- Architecture/system contract
- Implementation contract
- Godog feature/scenario

Minimum traceability fields:

- behavior_id
- source_contract_ref
- feature_file
- scenario_name
- implementation_ref

## CI Enforcement (Merge Blocking)

CI must fail when any of the following is true:

- Behavior-changing code has no linked Gherkin scenario.
- New/changed scenario has no linked implementation reference.
- Missing GIVEN/WHEN/THEN expectation docs for new functions.
- Broken contract references to UX/architecture docs.

## Documentation Density Expectation

Engineering expectation:

- Documentation should be at least as detailed as implementation for behavior-critical code paths.
- Documentation volume target is subordinate to clarity, traceability, and testability.

## Inputs for Documentation Authors

Required source material for behavior and contract writing:

- ../ux/
- ../architecture/
- ../implementation/

Authors should prioritize contract consistency over implementation shortcuts.
