# AgentX Implementation Index

Last updated: 2026-06-23
Status: Draft for architecture reconciliation

## Purpose

This folder translates UX requirements and architecture contracts into implementation guidance for developers.

Primary source inputs:

- ../ux/*.md
- ../architecture/*.md
- ../event_broker_pubsub.md

## Scope

In scope:

- Runtime architecture and process boundaries
- Surface orchestration and transport contracts
- Configuration and persistence contracts
- Tool execution safety and approval policy
- Prompt stack and LLM runtime behavior
- Delivery phases and acceptance gates

Out of scope:

- UX behavior definition (owned by ../ux)
- Architecture intent and naming contracts (owned by ../architecture)
- Deep package-level code details (to be added as implementation evolves)

## Document Map

1. [01_runtime_blueprint.md](01_runtime_blueprint.md)
2. [02_surface_orchestration_http.md](02_surface_orchestration_http.md)
3. [03_configuration_and_storage.md](03_configuration_and_storage.md)
4. [04_llm_prompt_tooling_runtime.md](04_llm_prompt_tooling_runtime.md)
5. [05_security_approvals_and_command_policy.md](05_security_approvals_and_command_policy.md)
6. [06_delivery_plan.md](06_delivery_plan.md)
7. [07_test_and_documentation_contract.md](07_test_and_documentation_contract.md)
8. [08_go_module_layout.md](08_go_module_layout.md)
9. [09_makefile_and_quality_gate_contract.md](09_makefile_and_quality_gate_contract.md)
10. [90_open_questions.md](90_open_questions.md)

## Reconciliation Rules

- UX documents describe what users should experience.
- Architecture documents describe stable contracts and boundaries.
- Implementation docs describe exactly how developers will build to satisfy both.

If conflict is discovered:

1. Preserve UX and architecture intent.
2. Record the implementation decision in these docs.
3. Track unresolved choices in [90_open_questions.md](90_open_questions.md).

## Terminology Baseline

- Surface/Applet: interchangeable terms for user-facing runtime areas
- Processing state: canonical runtime status model shared across surfaces
- Session: filesystem-backed interaction timeline and artifacts
- Runtime orchestrator: process that coordinates model, tools, state, and surfaces

## Immediate Next Actions

- Confirm unanswered decisions in [90_open_questions.md](90_open_questions.md).
- Freeze v1 API and file schemas before coding.
- Start phase-by-phase execution from [06_delivery_plan.md](06_delivery_plan.md).
