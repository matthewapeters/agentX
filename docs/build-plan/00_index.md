# AgentX Build Plan Index

Last updated: 2026-06-23
Status: Active planning baseline
Owner: Delivery

## Authority And Precedence

When guidance conflicts, use this authority order:

1. Architecture contracts
2. UX behavior contracts
3. Implementation mechanics

If ambiguity remains after applying the order above, escalate before proceeding
to implementation claims.

## Purpose

This folder provides a developer-ready execution plan that translates existing
architecture, UX, and implementation contracts into sequenced build slices.

This plan is additive. It does not replace existing contracts in:

- ../implementation/
- ../ux/
- ../architecture/

## How To Use This Folder

1. Start with `01_comprehensive_build_plan.md` for sequencing and phase gates.
2. Use `02_phase_reference_matrix.md` during execution to find source contracts.
3. At each phase gate, validate measurable acceptance criteria, mandatory
  AC-to-test-case traceability, and quality gates before moving to the next
  phase.
4. At M1-M4 start, run the mandatory Go-core preflight decision gate from
  `01_comprehensive_build_plan.md` before any executable claim is made.

## Document Map

1. [01_comprehensive_build_plan.md](01_comprehensive_build_plan.md)
2. [02_phase_reference_matrix.md](02_phase_reference_matrix.md)

## Cadence And Governance

- Weekly execution cadence: one phase checkpoint per week unless a phase is
  explicitly split into multiple checkpoints.
- Gate reviews: architecture + UX + QA checks at each phase exit.
- Evidence packaging is mandatory at each checkpoint and must include:
  - gate command outputs
  - AC coverage table with test case IDs
  - unresolved decisions and risk delta
  - runtime compatibility delta (if any)
- Change control: if contract conflicts are found, resolve by updating
  implementation guidance and recording unresolved items in
  ../implementation/90_open_questions.md.

## Required Ownership Timing

- Every milestone and gate must have named owners assigned no later than
  checkpoint kickoff for that milestone.
- Missing owner assignment at checkpoint kickoff is a fail-close condition.

## Authoritative Inputs

- ../implementation/00_index.md
- ../implementation/06_delivery_plan.md
- ../implementation/07_test_and_documentation_contract.md
- ../implementation/09_makefile_and_quality_gate_contract.md
- ../implementation/90_open_questions.md
- ../ux/00_INDEX.md
- ../ux/UX_LIFECYCLE.md
- ../architecture/channel_registry.md
- ../../AGENTS.md
