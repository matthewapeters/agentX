# Makefile and Quality Gate Contract (v1)

## Purpose

Define the required build and test invocation contract for all engineers.

## Normative Build Command

Primary command:

- make all

Required behavior:

- make all is the canonical local and CI entrypoint for baseline project verification.
- make all aliases the sequence:
  - make clean
  - make build

Alias semantics:

- if clean fails, build must not execute.
- overall command exits non-zero on any failure.

## Expected Coverage of make all

v1 expectation from Make targets:

- clean build artifacts and stale generated outputs
- run build flow that includes Go core validation and compilation
- prepare required runtime artifacts bundled by build

Implementation note:

- project-specific Makefile may include additional checks during build.
- make all is the minimum required verification gate before merge and before release candidate handoff.

## Dependency Hygiene Contract

Dependency-change policy (v1):

- Any time dependencies change, engineers must run and commit the outputs of:
  - go mod tidy
  - go mod vendor

CI expectation:

- CI should fail if dependency-affecting changes are present and module/vendor state is inconsistent.

## Engineer Ownership Policy for Failing Tests

Mandatory policy:

- Engineers must fix broken tests encountered during their change workflow, even when the break is not obviously caused by files they edited.
- Downstream and cross-module effects are considered part of delivery responsibility.
- Ownership for this policy is the user/product owner.

Required response when tests fail:

1. Investigate impact and root cause.
2. Fix code or contracts causing failure.
3. Update Gherkin scenarios and documentation if behavior changed.
4. Re-run make all until green.

Non-compliance:

- Merges should be blocked when baseline test/build expectations fail.

## Changelog Contract

Required changelog update trigger (v1):

- Any time semantic version changes, CHANGELOG.md must be updated in the same change set.

Minimum changelog entry requirements:

- version section
- concise summary of major changes
- behavior or contract implications when relevant

## Quality Gate Severity Policy

Severity handling (v1):

- Blockers: must be fixed before merge.
- Warnings: must be addressed before merge.
- Nits: may be deferred or ignored at reviewer discretion.

## Recommended Engineering Workflow

1. Update docs and Gherkin contracts first.
2. Implement code changes.
3. Run make all.
4. Resolve all failing tests.
5. Re-run make all and attach output evidence in review context.

## Gherkin Behavior Contract

Use-case: Engineer validates baseline before merge

- GIVEN a local checkout with proposed changes
- WHEN engineer runs make all
- THEN clean and build stages execute in order and command exits zero only when successful

Use-case: Engineer encounters unrelated-seeming test failure

- GIVEN failing tests during make all
- WHEN engineer evaluates failures
- THEN engineer fixes the failures or their root contract issues before merge

## CI Policy Alignment

CI should enforce:

- make all must pass on merge-request pipelines
- failures are hard blockers
- no exception path for "unrelated" failing tests without explicit documented waiver

## Related Contracts

- 07_test_and_documentation_contract.md
- 08_go_module_layout.md
- 06_delivery_plan.md
