# Build Plan Phase Reference Matrix

Last updated: 2026-06-23
Status: Active

## Purpose

Map each build milestone to concrete source documents in architecture, UX, and
implementation folders so builders can navigate quickly and avoid contract drift.

## Standard Role Mapping (Fail-Close)

- Milestone owner: Delivery Lead
- Gate owners: Architecture Reviewer, UX Reviewer, QA Lead
- Security gate owner (required for M1, M3b, M4): Security Reviewer

Missing role assignment at checkpoint kickoff is fail-close.

## M0: Contract Alignment Baseline

Primary references:

- docs/implementation/00_index.md
- docs/implementation/06_delivery_plan.md
- docs/implementation/07_test_and_documentation_contract.md
- docs/implementation/09_makefile_and_quality_gate_contract.md
- docs/implementation/08_go_module_layout.md
- docs/implementation/90_open_questions.md
- AGENTS.md

Builder use:

- Confirm baseline assumptions and unresolved decisions before any coding slice.

Mandatory gate requirements:

- AC-to-test-case traceability table required at checkpoint exit.
- Documentation-first GIVEN/WHEN/THEN gate required for touched planning
  functions/flows.
- Checkpoint evidence package must use
  docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md.

## M1: Runtime Skeleton And Surface Registration Foundation

Architecture references:

- docs/architecture/channel_registry.md
- docs/architecture/adr/0001-orchestrator-control-execution-data-planes.md

Implementation references:

- docs/implementation/01_runtime_blueprint.md
- docs/implementation/02_surface_orchestration_http.md
- docs/implementation/03_configuration_and_storage.md
- docs/implementation/06_delivery_plan.md
- docs/implementation/09_makefile_and_quality_gate_contract.md

UX references:

- docs/ux/00_INDEX.md (for parity implications during bootstrap)

Builder use:

- Build runtime lifecycle, registration, and transport foundations with
  contract-safe startup/shutdown behavior.

Mandatory gate requirements:

- Start-of-milestone preflight: `test -d cmd/agentx-core`.
- If preflight fails, switch to contract-validation-only mode and prohibit
  executable Go-core claims.
- AC-to-test-case traceability table and regression control evidence required.
- Documentation-first GIVEN/WHEN/THEN gate required for touched functions.
- Checkpoint evidence package must use
  docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md.

## M2: UX Surface Parity Baseline (TUI + System Surfaces)

UX references:

- docs/ux/00_INDEX.md
- docs/ux/UX_LIFECYCLE.md
- docs/ux/03_PANEL_DETAILS.md
- docs/ux/01_MAIN_LAYOUT.md
- docs/ux/02_USER_FLOWS.md

Architecture references:

- docs/architecture/channel_registry.md (processing_state and event channel use)

Implementation references:

- docs/implementation/06_delivery_plan.md
- docs/implementation/07_test_and_documentation_contract.md

Builder use:

- Implement and reconcile user-visible affordances and lifecycle traceability.

Mandatory gate requirements:

- Start-of-milestone preflight: `test -d cmd/agentx-core`.
- AC-to-test-case traceability table and regression control evidence required.
- Documentation-first GIVEN/WHEN/THEN gate required for touched flows.
- Checkpoint evidence package must use
  docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md.

## M3a: LLM Prompt Stack And Model Behavior

Implementation references:

- docs/implementation/04_llm_prompt_tooling_runtime.md
- docs/implementation/06_delivery_plan.md
- docs/implementation/07_test_and_documentation_contract.md

Architecture references:

- docs/architecture/channel_registry.md
- docs/architecture/adr/0004-persona-skill-policy-and-security-boundaries.md
- docs/architecture/design/06_persona_skill_and_tools_canon.md

UX references:

- docs/ux/UX_LIFECYCLE.md (for any user-visible control/feedback updates)

Builder use:

- Deliver classify/prompt/model behavior deterministically before policy-loop
  coupling.

Mandatory gate requirements:

- Start-of-milestone preflight: `test -d cmd/agentx-core`.
- AC-to-test-case traceability table and regression control evidence required.
- Documentation-first GIVEN/WHEN/THEN gate required for touched functions.
- Checkpoint evidence package must use
  docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md.

## M3b: Tool Runtime And Policy Enforcement

Implementation references:

- docs/implementation/05_security_approvals_and_command_policy.md
- docs/implementation/06_delivery_plan.md
- docs/implementation/07_test_and_documentation_contract.md

Architecture references:

- docs/architecture/channel_registry.md
- docs/architecture/adr/0004-persona-skill-policy-and-security-boundaries.md
- docs/architecture/design/06_persona_skill_and_tools_canon.md
- docs/architecture/design/04_policy_boundary_authorization_contracts.md

UX references:

- docs/ux/UX_LIFECYCLE.md (for policy/approval feedback behavior)

Builder use:

- Deliver tool runtime execution, policy enforcement, and approval behavior with
  required negative-path resilience.

Mandatory gate requirements:

- Start-of-milestone preflight: `test -d cmd/agentx-core`.
- AC-to-test-case traceability table and regression control evidence required.
- Negative-path matrix is mandatory.
- Documentation-first GIVEN/WHEN/THEN gate required for touched functions.
- Checkpoint evidence package must use
  docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md.

## M4: Persistence, Replay, And Operational Hardening

Architecture references:

- docs/architecture/adr/0005-traceability-replay-and-quality-gates.md
- docs/architecture/behavior/TRACEABILITY_MATRIX.md
- docs/architecture/behavior/00_INDEX.md

Implementation references:

- docs/implementation/03_configuration_and_storage.md
- docs/implementation/06_delivery_plan.md
- docs/implementation/07_test_and_documentation_contract.md
- docs/implementation/09_makefile_and_quality_gate_contract.md

UX references:

- docs/ux/UX_LIFECYCLE.md (status reconciliation and parity confirmation)

Builder use:

- Finalize persistence/replay, validate deterministic behavior, and pass release
  quality gates.

Mandatory gate requirements:

- Start-of-milestone preflight: `test -d cmd/agentx-core`.
- AC-to-test-case traceability table and regression control evidence required.
- Negative-path matrix is mandatory.
- Documentation-first GIVEN/WHEN/THEN gate required for touched functions.
- Checkpoint evidence package must use
  docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md.

## Cross-Phase Quality Gate References

- docs/implementation/07_test_and_documentation_contract.md
- docs/implementation/09_makefile_and_quality_gate_contract.md
- docs/implementation/90_open_questions.md
- docs/architecture/behavior/TRACEABILITY_MATRIX.md
- docs/ux/UX_LIFECYCLE.md

## Gate Command And Evidence Matrix (Mandatory)

| Milestone | Gate commands (minimum set) | Mandatory evidence |
| --- | --- | --- |
| M0 | `rg -n "^Milestone owner:|^Gate owners:" docs/build-plan/01_comprehensive_build_plan.md` plus `rg -n "^\| AC ID \|" docs/build-plan/01_comprehensive_build_plan.md docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md` plus `rg -n "^\| Decision/AC ID \|" docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md` | AC table, owner assignments, defer metadata fields (owner/due/tracking reference), conflict/defer list, runtime compatibility delta |
| M1 | `test -d cmd/agentx-core` plus milestone gate commands from docs/implementation/09_makefile_and_quality_gate_contract.md | preflight result, AC table, regression evidence, GIVEN/WHEN/THEN links |
| M2 | `test -d cmd/agentx-core` plus milestone gate commands from docs/implementation/09_makefile_and_quality_gate_contract.md | AC table, regression evidence, UX lifecycle reconciliation, GIVEN/WHEN/THEN links |
| M3a | `test -d cmd/agentx-core` plus milestone gate commands from docs/implementation/09_makefile_and_quality_gate_contract.md | AC table, regression evidence, prompt/model determinism evidence, GIVEN/WHEN/THEN links |
| M3b | `test -d cmd/agentx-core` plus milestone gate commands from docs/implementation/09_makefile_and_quality_gate_contract.md | AC table, regression evidence, negative-path matrix, policy deny/allow evidence, GIVEN/WHEN/THEN links |
| M4 | `test -d cmd/agentx-core` plus milestone gate commands from docs/implementation/09_makefile_and_quality_gate_contract.md | AC table, regression evidence, negative-path matrix, replay determinism evidence, GIVEN/WHEN/THEN links |

M0 evidence normalization requirements (applies to each listed command, including silent commands such as `test -d` when used):

- Record one normalized proof row per command with fields:
  - command
  - exit_code
  - timestamp (UTC ISO-8601)
  - operator
  - artifact_link (or `none` when no stdout/stderr artifact exists)
- A non-zero exit code requires explicit hold/remediate or approved defer entry.

Canonical evidence artifact convention:

- Base path: `docs/validation/evidence/<checkpoint_id>/`
- File naming: `<checkpoint_id>_<artifact_type>_<YYYYMMDD-HHMMSS>.<ext>`

## Cross-Phase Review Checkpoint Inputs

For every phase checkpoint, collect:

- Scope slice mapped to milestone and source docs above
- Updated risk list and open-question delta
- Runtime compatibility delta
- Test and quality-gate evidence
- Traceability updates across implementation, UX, and architecture refs
