# Orchestration Design Index

Last updated: 2026-06-15
Owner: Architecture

## Purpose

This folder contains implementation-ready object-oriented design documents for the Go-based multi-expert backend aligned to ADR 0001-0005.

All design docs in this folder must link to authoritative schemas in:

- `docs/architecture/schemas/`

## Design Documents

- `01_control_execution_data_planes.md`
- `02_node_taxonomy_and_pattern_compiler_contracts.md`
- `03_dispatcher_priority_heap_fairness_contracts.md`
- `04_policy_boundary_authorization_contracts.md`
- `05_trace_replay_quality_gate_schemas.md`
- `06_persona_skill_and_tools_canon.md` — Canonical locations and loading pipeline for personas, skills, and tools

## ADR Alignment

- ADR 0001 -> `01_control_execution_data_planes.md`
- ADR 0002 -> `02_node_taxonomy_and_pattern_compiler_contracts.md`
- ADR 0003 -> `03_dispatcher_priority_heap_fairness_contracts.md`
- ADR 0004 -> `04_policy_boundary_authorization_contracts.md`
- ADR 0005 -> `05_trace_replay_quality_gate_schemas.md`
- ADR 0004 (supplement) -> `06_persona_skill_and_tools_canon.md` — Extends ADR 0004 with explicit directory contracts

## Canonical Runtime Locations

**Go experts and runtime developers must enforce these as source-of-truth locations:**

- `.agentx/agents/` — Authoritative home for expert persona definitions
- `.agentx/skills/` — Authoritative home for skill specifications
- `.agentx/tools/` — Reference documentation for available tools (for discovering and documenting capabilities)

See `06_persona_skill_and_tools_canon.md` for detailed loading pipeline and determinism contracts.
