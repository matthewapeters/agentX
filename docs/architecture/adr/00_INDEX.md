# AgentX ADR Index

Last updated: 2026-06-23
Owner: Architecture

This folder captures architecture decisions for orchestration design, rollout sequencing, and quality governance. These ADRs are implementation-oriented and aligned with:

- docs/architecture/design/00_INDEX.md
- docs/architecture/channel_registry.md
- docs/implementation/00_index.md

## ADRs

- [0001 - Orchestrator Control, Execution, and Data Planes](0001-orchestrator-control-execution-data-planes.md)
- [0002 - DAG Node Taxonomy and Pattern Compiler](0002-dag-node-taxonomy-and-pattern-compiler.md)
- [0003 - LLM Dispatcher Priority Heap and Fairness](0003-llm-dispatcher-priority-heap-and-fairness.md)
- [0004 - Persona, Skill Policy, and Security Boundaries](0004-persona-skill-policy-and-security-boundaries.md)
- [0005 - Traceability, Replay, and Quality Gates](0005-traceability-replay-and-quality-gates.md)
- [0006 - Persona Identification and Context Loading](0006-persona-identification-and-context-loading.md)
- [0007 - Output-Panel Markdown Rendering (Dual Renderer)](0007-output-panel-markdown-rendering.md) — Family A surface concern

## Reading Order

1. Read ADR 0001 for system boundaries and ownership.
2. Read ADR 0002 for orchestration graph semantics.
3. Read ADR 0003 for dispatch scheduling and fairness.
4. Read ADR 0004 for policy and security controls.
5. Read ADR 0005 for evidence, replay, and release gates.
6. Read ADR 0006 for persona routing and deterministic context assembly.

## Implementation Companions

- Detailed design docs: `docs/architecture/design/00_INDEX.md`
- Behavior use cases (Gherkin): `docs/architecture/behavior/00_INDEX.md`
- Authoritative schemas: `docs/architecture/schemas/00_INDEX.md`

## Change Policy

- New architecture decisions that alter orchestration ownership or policy must add a new ADR file in this folder.
- Existing ADRs are immutable in intent. If intent changes materially, supersede with a new ADR and reference the prior one.
- Implementation PRs should cite the ADR ID in commit messages, PR descriptions, or both.
