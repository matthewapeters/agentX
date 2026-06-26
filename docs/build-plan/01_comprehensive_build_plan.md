# AgentX Comprehensive Build Plan

Last updated: 2026-06-23
Status: Execution-ready
Scope: Repository-wide implementation planning (no code changes)

## 1. Planning Objectives

- Deliver a coherent implementation path aligned with current contracts.
- Make sequencing explicit across runtime, UX parity, tooling, persistence, and
  quality enforcement.
- Ensure every phase points to concrete source documentation.
- Define clear checkpoints, acceptance criteria, and risk controls.

## 2. Execution Principles

- Contract-first: architecture and UX docs define target behavior.
- Documentation-first: behavior expectations and traceability precede coding.
- Gate-based execution: no phase is complete until quality gates pass.
- Evidence-based progression: each checkpoint produces runnable or inspectable
  output and updated traceability.

## 3. Cross-Milestone Mandatory Controls

### 3.2 Mandatory AC-To-Test-Case Traceability

Every milestone checkpoint must include an AC coverage table with this format:

| AC ID | Acceptance criterion (measurable) | Test case ID(s) | Test type | Evidence link/artifact | Result |
| --- | --- | --- | --- | --- | --- |

Exit gate rule:

- All AC rows must map to at least one test case ID.
- Any unmapped AC is fail-close unless explicitly deferred with owner,
  due milestone, and tracking reference.

### 3.3 Mandatory Documentation-First GIVEN/WHEN/THEN Gate

For each milestone, every touched function/flow must have a documentation-first
behavior contract in GIVEN/WHEN/THEN format before implementation is claimed
complete.

### 3.4 Mandatory Regression Control

Each milestone must define:

- Protected scenarios
- Rerun minimum for flaky-suspect paths
- Fail-close policy for regressions

### 3.5 Mandatory Checkpoint Evidence Package

Every milestone checkpoint must publish an evidence package containing:

- gate command outputs
- AC coverage table
- regression and negative-path results (where required)
- unresolved decision log
- runtime compatibility delta
- evidence document based on `docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md`

### 3.6 Ownership Requirements

- Milestone owner and gate owners are required fields.
- Ownership assignment timing: no later than milestone kickoff.
- Missing owner assignment is fail-close.

## 4. Milestone Plan

### M0: Contract Alignment Baseline

Milestone owner: Delivery Lead
Gate owners: Architecture=Architecture Reviewer, UX=UX Reviewer, QA=QA Lead

Goal:

- Lock execution assumptions and remove ambiguities before implementation.

Primary dependencies:

- Existing implementation contract docs
- Open questions register

Activities:

- Validate that the build sequence aligns with:
  - docs/implementation/06_delivery_plan.md
  - docs/implementation/07_test_and_documentation_contract.md
  - docs/implementation/09_makefile_and_quality_gate_contract.md
- Reconfirm unresolved decisions from docs/implementation/90_open_questions.md.
- Confirm current branch constraints from CLAUDE.md.

Milestone deliverables:

- Finalized execution assumptions per phase.
- Updated issue/risk list tied to open questions.
- Confirmed quality-gate command set per phase.

Acceptance criteria:

- AC-M0-1: 0 unresolved high-severity contract conflicts remain open at M0 exit.
- AC-M0-2: 100 percent of open decision dependencies are either resolved or
  deferred with owner, due milestone, and tracking reference.
- AC-M0-3: 100 percent of M0 AC rows map to at least one test case ID in the
  AC coverage table.

Documentation-first GIVEN/WHEN/THEN gate:

- GIVEN any M0-touched planning function or decision flow,
  WHEN the checkpoint is reviewed,
  THEN a corresponding GIVEN/WHEN/THEN behavior contract exists and is linked.

Regression control:

- Protected scenarios: contract precedence resolution, deferment path, and
  checkpoint decision recording.
- Rerun minimum: 2 consecutive reruns for M0 gate checks.
- Fail-close policy: any mismatch in precedence application or defer metadata
  blocks progression.

AC coverage table requirement:

| AC ID | Acceptance criterion (measurable) | Test case ID(s) | Test type | Evidence link/artifact | Result |
| --- | --- | --- | --- | --- | --- |

Quality gate:

- Documentation coherence review completed (architecture + UX + implementation
  references valid and non-contradictory).

### M1: Runtime Skeleton And Surface Registration Foundation

Milestone owner: Delivery Lead
Gate owners: Architecture=Architecture Reviewer, UX=UX Reviewer, QA=QA Lead,
Security=Security Reviewer

Goal:

- Establish stable runtime bootstrap, session initialization, and
  surface-registration lifecycle.

Primary dependencies:

- M0 complete
- channel registry contract and implementation runtime blueprint

Activities:

- Build according to docs/implementation/06_delivery_plan.md Phase 1 scope.
- Enforce channel/event expectations from docs/architecture/channel_registry.md.
- Follow runtime/process guidance from docs/implementation/01_runtime_blueprint.md
  and docs/implementation/02_surface_orchestration_http.md.
- Apply config/session path expectations from docs/implementation/03_configuration_and_storage.md.

Milestone deliverables:

- Stable startup/shutdown lifecycle.
- Health and surface endpoints operational.
- Attach-token registration path functional and constrained.

Acceptance criteria:

- AC-M1-1: 0 collisions across 10 repeated startup attempts for local multi-instance
  runtime initialization in the selected execution mode.
- AC-M1-2: 100 percent of invalid attach-token registration attempts are denied;
  100 percent of valid token attempts succeed in scope-defined tests.
- AC-M1-3: Session/config root initialization path matches contract-defined
  location semantics in 10 out of 10 attempts.
- AC-M1-4: 100 percent of M1 AC rows map to at least one test case ID in the
  AC coverage table.

Documentation-first GIVEN/WHEN/THEN gate:

- GIVEN any touched startup/registration function,
  WHEN checkpoint exit is proposed,
  THEN GIVEN/WHEN/THEN behavior docs for the touched function are present and
  linked in evidence.

Regression control:

- Protected scenarios: startup/shutdown lifecycle, attach-token validation,
  surface registration deny/allow behavior.
- Rerun minimum: 3 reruns for startup and registration scenarios.
- Fail-close policy: any regression in protected scenarios blocks milestone exit.

AC coverage table requirement:

| AC ID | Acceptance criterion (measurable) | Test case ID(s) | Test type | Evidence link/artifact | Result |
| --- | --- | --- | --- | --- | --- |

Quality gate:

- Baseline `make all` behavior defined in docs/implementation/09_makefile_and_quality_gate_contract.md passes.
- Contract tests for startup, shutdown, and registration are present and green.

### M2: UX Surface Parity Baseline (TUI + System Surfaces)

Milestone owner: Delivery Lead
Gate owners: Architecture=Architecture Reviewer, UX=UX Reviewer, QA=QA Lead

Goal:

- Reach user-observable baseline parity for required surface affordances and
  processing-state visibility.

Primary dependencies:

- M1 complete
- UX lifecycle matrix and panel-level specs

Activities:

- Implement priority UX affordances from docs/ux/00_INDEX.md queue and
  docs/ux/UX_LIFECYCLE.md traceability matrix.
- Ensure processing-state rendering conforms to channel registry
  processing_state contract.
- Reconcile feature behavior in docs/ux/03_PANEL_DETAILS.md and
  docs/ux/UX_LIFECYCLE.md as implementation lands.

Milestone deliverables:

- Prompt submit cycle visible and stable in primary surfaces.
- Processing state reflected across required surfaces without drift.
- Affordance traceability rows reconciled to implemented status.

Acceptance criteria:

- AC-M2-1: 100 percent of in-scope affordance IDs are mapped to tested status
  in UX lifecycle traceability at checkpoint exit.
- AC-M2-2: 0 unresolved critical drift items between implemented surface behavior
  and documented transitions for in-scope affordances.
- AC-M2-3: 100 percent of M2 AC rows map to at least one test case ID in the
  AC coverage table.

Documentation-first GIVEN/WHEN/THEN gate:

- GIVEN any touched UX flow or panel behavior,
  WHEN milestone close is requested,
  THEN GIVEN/WHEN/THEN behavior docs exist for the touched flows and are linked.

Regression control:

- Protected scenarios: submit lifecycle, processing-state display, panel
  transition continuity.
- Rerun minimum: 3 reruns for user-observable lifecycle scenarios.
- Fail-close policy: any protected scenario drift or failure blocks progression.

AC coverage table requirement:

| AC ID | Acceptance criterion (measurable) | Test case ID(s) | Test type | Evidence link/artifact | Result |
| --- | --- | --- | --- | --- | --- |

Quality gate:

- Godog/Gherkin behavior coverage exists for implemented affordances per
  docs/implementation/07_test_and_documentation_contract.md.
- No unresolved UX spec/implementation drift at checkpoint close.

### M3a: LLM Prompt Stack And Model Behavior

Milestone owner: Delivery Lead
Gate owners: Architecture=Architecture Reviewer, UX=UX Reviewer, QA=QA Lead

Goal:

- Deliver deterministic classify/prompt/model-switch behavior without tool
  policy enforcement scope.

Primary dependencies:

- M2 complete
- prompt runtime contracts

Activities:

- Implement docs/implementation/06_delivery_plan.md LLM/prompt slices.
- Align prompt assembly and model-switch behavior with
  docs/implementation/04_llm_prompt_tooling_runtime.md.
- Validate event publication for model/prompt processing updates
  against channel registry.

Milestone deliverables:

- Deterministic prompt assembly and response behavior per contract.
- Clean handling for model-switch and in-flight prompt decisions.
- Traceable model behavior evidence without policy-loop coupling.

Acceptance criteria:

- AC-M3a-1: 100 percent of in-scope classify/prompt flows complete with
  deterministic event ordering under repeated runs.
- AC-M3a-2: Model switching behavior is recoverable in 10 out of 10
  interruption tests for in-scope paths.
- AC-M3a-3: 100 percent of M3a AC rows map to at least one test case ID in
  the AC coverage table.

Documentation-first GIVEN/WHEN/THEN gate:

- GIVEN any touched LLM/prompt function,
  WHEN M3a checkpoint is evaluated,
  THEN GIVEN/WHEN/THEN docs for that function are present and linked.

Regression control:

- Protected scenarios: prompt assembly determinism, model-switch recoverability,
  processing-state continuity.
- Rerun minimum: 3 reruns for determinism-sensitive scenarios.
- Fail-close policy: nondeterministic or nonrecoverable behavior blocks exit.

AC coverage table requirement:

| AC ID | Acceptance criterion (measurable) | Test case ID(s) | Test type | Evidence link/artifact | Result |
| --- | --- | --- | --- | --- | --- |

Quality gate:

- Contract and integration scenarios for prompt/model paths pass.
- Traceability links (UX/architecture/implementation/test) are complete.

### M3b: Tool Runtime And Policy Enforcement

Milestone owner: Delivery Lead
Gate owners: Architecture=Architecture Reviewer, UX=UX Reviewer, QA=QA Lead,
Security=Security Reviewer

Goal:

- Deliver tool runtime execution loop with approval and policy enforcement,
  including negative-path resilience.

Primary dependencies:

- M3a complete
- policy and security contracts

Activities:

- Implement docs/implementation/06_delivery_plan.md tooling/policy slices.
- Enforce tool approval and command policy from
  docs/implementation/05_security_approvals_and_command_policy.md.
- Validate event publication for tool_call/tool_result and processing updates
  against channel registry and policy boundary contracts.

Milestone deliverables:

- End-to-end tool invocation with structured results and audit-safe logging.
- Approval UX and policy enforcement for risky commands.
- Negative-path resilience evidence for denied, malformed, timed-out,
  and interrupted tool flows.

Acceptance criteria:

- AC-M3b-1: 100 percent of disallowed command attempts are denied and logged
  with policy reason codes.
- AC-M3b-2: 100 percent of approved tool calls return structured tool_result or
  explicit bounded failure outcomes.
- AC-M3b-3: 0 unresolved critical negative-path defects from required matrix.
- AC-M3b-4: 100 percent of M3b AC rows map to at least one test case ID in the
  AC coverage table.

Documentation-first GIVEN/WHEN/THEN gate:

- GIVEN any touched tool-policy function,
  WHEN M3b gate is reviewed,
  THEN GIVEN/WHEN/THEN behavior docs for touched functions are present and linked.

Regression control:

- Protected scenarios: policy allow/deny, approval handoff, tool timeout and
  interruption recovery.
- Rerun minimum: 4 reruns for policy and interruption paths.
- Fail-close policy: any policy-bypass or critical resilience regression blocks exit.

Required negative-path matrix (M3b):

| Scenario ID | Negative condition | Expected bounded behavior | Test case ID(s) | Evidence | Result |
| --- | --- | --- | --- | --- | --- |
| NEG-M3b-1 | Disallowed command | Deny with reason code and no execution |  |  |  |
| NEG-M3b-2 | Missing/invalid approval | Block execution and request approval path |  |  |  |
| NEG-M3b-3 | Tool timeout | Emit bounded failure and recover loop state |  |  |  |
| NEG-M3b-4 | Malformed tool result | Safe parse failure handling and user-safe output |  |  |  |

AC coverage table requirement:

| AC ID | Acceptance criterion (measurable) | Test case ID(s) | Test type | Evidence link/artifact | Result |
| --- | --- | --- | --- | --- | --- |

Quality gate:

- Contract and integration scenarios for tool policy paths pass.
- Negative-path matrix results are complete and fail-close enforced.
- Traceability links (UX/architecture/implementation/test) are complete.

### M4: Persistence, Replay, And Operational Hardening

Milestone owner: Delivery Lead
Gate owners: Architecture=Architecture Reviewer, UX=UX Reviewer, QA=QA Lead,
Security=Security Reviewer

Goal:

- Guarantee durable session replay, deterministic event ordering, and release
  confidence through hardening gates.

Primary dependencies:

- M3b complete
- persistence/replay and quality-gate contracts

Activities:

- Implement docs/implementation/06_delivery_plan.md Phase 5 and 6 scope.
- Apply replay and traceability expectations in
  docs/architecture/behavior/TRACEABILITY_MATRIX.md and
  docs/architecture/adr/0005-traceability-replay-and-quality-gates.md.
- Run reliability/load/failure-injection coverage for model/tool/storage paths.
- Execute all required merge gates and changelog/semver policy checks.

Milestone deliverables:

- Recoverable session timelines from persisted events.
- Deterministic replay semantics for supported views.
- Release-candidate quality report with blocker/warning disposition.

Acceptance criteria:

- AC-M4-1: Replay outputs are byte-equivalent for 10 out of 10 identical-input
  replay runs.
- AC-M4-2: 100 percent of required CI quality gates pass for milestone scope.
- AC-M4-3: 0 open blockers and 0 open required warnings at merge decision.
- AC-M4-4: 100 percent of M4 AC rows map to at least one test case ID in the
  AC coverage table.

Documentation-first GIVEN/WHEN/THEN gate:

- GIVEN any touched persistence/replay function,
  WHEN M4 checkpoint exit is requested,
  THEN GIVEN/WHEN/THEN docs for touched functions are present and linked.

Regression control:

- Protected scenarios: replay determinism, event-order integrity, recovery after
  restart, and CI gate reproducibility.
- Rerun minimum: 4 reruns for replay and hardening-critical scenarios.
- Fail-close policy: determinism failure or gate instability blocks progression.

Required negative-path matrix (M4):

| Scenario ID | Negative condition | Expected bounded behavior | Test case ID(s) | Evidence | Result |
| --- | --- | --- | --- | --- | --- |
| NEG-M4-1 | Corrupted event payload | Reject with traceable error and preserve replay integrity |  |  |  |
| NEG-M4-2 | Missing event segment | Bounded failure with actionable recovery signal |  |  |  |
| NEG-M4-3 | Out-of-order event stream | Deterministic handling per replay contract |  |  |  |
| NEG-M4-4 | Storage read/write interruption | Safe retry/recovery or explicit bounded failure |  |  |  |

AC coverage table requirement:

| AC ID | Acceptance criterion (measurable) | Test case ID(s) | Test type | Evidence link/artifact | Result |
| --- | --- | --- | --- | --- | --- |

Quality gate:

- `make all` and required test suites pass in CI.
- Documentation and traceability gates pass (including GIVEN/WHEN/THEN contract
  checks and linked scenarios).

## 5. Sequencing And Dependency Graph

Sequential dependencies:

- M0 -> M1 -> M2 -> M3a -> M3b -> M4

Parallelizable work within milestones:

- M0: contract review and risk register curation can run in parallel.
- M1: runtime lifecycle scaffolding and endpoint contract tests can run in parallel.
- M2: affordance implementation and Gherkin authoring can run in parallel after
  affordance IDs are frozen.
- M3a: prompt behavior tests and model-switch handling can run in parallel once
  event schema assertions are fixed.
- M3b: tool policy evaluator and approval UX can run in parallel once shared
  policy schema is fixed.
- M4: persistence implementation and failure-injection harnessing can run in
  parallel after replay schema is fixed.

Critical path:

- Contract freeze -> runtime skeleton -> UX parity baseline -> LLM/prompt
  behavior -> tool runtime/policy -> persistence/hardening.

## 6. Suggested Execution Cadence And Checkpoints

Suggested cadence:

- Weekly phase checkpoint cadence.
- Mid-week implementation sync for active phase.
- End-of-week gate review with evidence artifacts.

Checkpoint template (for each milestone):

Use `docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md` as the required base
template for M0-M4 checkpoint packages.

- Checkpoint input:
  - scope slice and linked contracts
  - open risk delta
  - runtime compatibility delta
- Checkpoint evidence:
  - test/gate outputs
  - AC coverage table (mandatory)
  - negative-path matrix results for M3b and M4 (mandatory)
  - updated traceability references
  - unresolved decision log
- Checkpoint decision:
  - proceed
  - proceed with risk acceptance
  - hold and remediate

Review checkpoints:

- Architecture checkpoint: contract alignment and channel/policy correctness.
- UX checkpoint: affordance parity and lifecycle matrix reconciliation.
- QA checkpoint: Gherkin coverage, make/CI gate outcomes, regression status.

## 7. Risks And Mitigations

Risk: Contract drift between implementation, UX, and architecture docs.

- Mitigation: enforce phase-exit traceability review and documentation gate.

Risk: Open questions in configuration/policy/replay block later phases.

- Mitigation: resolve high-impact items in M0; defer only with explicit owner and
  target checkpoint.

Risk: Tooling loop complexity causes regressions in user-visible flow.

- Mitigation: stage M3 with contract tests first and explicit fallback behavior.

Risk: Quality-gate fatigue slows delivery.

- Mitigation: keep baseline gate set stable; automate repetitive checks in CI;
  treat warnings as merge blockers per contract.

Risk: Snapshot mismatch between expected Go-core paths and current workspace.

- Mitigation: run branch preflight checks from AGENTS.md before Go-core-specific
  execution and document exceptions in checkpoint notes.

## 8. Artifact Inputs And Outputs By Milestone

M0:

- Produced: execution assumptions, owner assignment list, AC template, risk delta.
- Consumed: implementation contracts, open questions, AGENTS reality check.

M1:

- Produced: startup/registration evidence, AC mapping, regression results.
- Consumed: M0 assumptions, runtime and channel contracts.

M2:

- Produced: UX parity evidence, lifecycle traceability updates, AC mapping.
- Consumed: M1 runtime baseline, UX contracts.

M3a:

- Produced: prompt/model determinism evidence, AC mapping, regression results.
- Consumed: M2 baseline, LLM runtime contracts.

M3b:

- Produced: policy enforcement evidence, negative-path matrix, AC mapping.
- Consumed: M3a behavior baseline, security/policy contracts.

M4:

- Produced: replay/hardening evidence, negative-path matrix, release gate report.
- Consumed: M3b policy/runtime baseline, persistence/replay contracts.

## 9. Additional Suggestions

- Add a lightweight `docs/build-plan/CHANGELOG.md` only if this folder begins to
  change independently across multiple delivery cycles.
- Keep `docs/validation/02_CHECKPOINT_EVIDENCE_TEMPLATE.md` as the reusable
  checkpoint evidence baseline and revise only when gate requirements change.
- Add role-to-person mapping at milestone kickoff (for example, "QA Lead =
  <name/handle>") in checkpoint evidence so required roles stay unambiguous.
- Use a canonical evidence artifact path/name convention for checkpoint assets:
  `docs/validation/evidence/<checkpoint_id>/` with files named
  `<checkpoint_id>_<artifact_type>_<YYYYMMDD-HHMMSS>.<ext>`.
- Use a lightweight AC test case ID format convention:
  `TC-<milestone>-<area>-<nnn>` (example: `TC-M3b-policy-003`).
- Consider adding a machine-readable milestone manifest (YAML/JSON) after the
  first full execution cycle if automation/reporting is needed.

## 10. Completion Definition

This build plan is considered complete when:

- All milestones have either completed acceptance criteria or have explicit
  approved deferments.
- Required quality gates for each completed milestone pass.
- Traceability across UX, architecture, implementation, and tests is current.
- Open questions are either closed or scheduled with owner and checkpoint.
- Every checkpoint evidence package is present and includes mandatory AC
  traceability and required negative-path matrices.
