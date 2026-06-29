# Orchestration Traceability Matrix

Last updated: 2026-06-15

| Scenario ID | ADR | Primary Component | Primary Schema | Gate |
| --- | --- | --- | --- | --- |
| ADR-0001-AF-001 | 0001 | Control Plane | docs/architecture/schemas/compiled-dag.schema.json | E |
| ADR-0001-AF-002 | 0001 | Control Plane | docs/architecture/schemas/trace-event.schema.json | E |
| ADR-0001-AF-003 | 0001 | Execution Plane | docs/architecture/schemas/execution-outcome.schema.json | E |
| ADR-0001-AF-004 | 0001 | Control/Execution | docs/architecture/schemas/execution-outcome.schema.json | E |
| ADR-0001-AF-005 | 0001 | Control Plane | docs/architecture/schemas/trace-event.schema.json | E |
| ADR-0001-AF-006 | 0001 | Data Plane | docs/architecture/schemas/trace-event.schema.json | E |
| ADR-0002-AF-001 | 0002 | Pattern Compiler | docs/architecture/schemas/compiled-dag.schema.json | D/E |
| ADR-0002-AF-002 | 0002 | Pattern Compiler | docs/architecture/schemas/compiled-dag.schema.json | D/E |
| ADR-0002-AF-003 | 0002 | Pattern Compiler | docs/architecture/schemas/compiled-dag.schema.json | D/E |
| ADR-0002-AF-004 | 0002 | Pattern Compiler | docs/architecture/schemas/compiled-dag.schema.json | D |
| ADR-0002-AF-005 | 0002 | Pattern Compiler | docs/architecture/schemas/compiled-dag.schema.json (graph_hash determinism) | D |
| ADR-0002-AF-006 | 0002 | Pattern Compiler | docs/architecture/schemas/compiled-dag.schema.json | D |
| ADR-0002-AF-007 | 0002 | Reduce Node | docs/architecture/schemas/compiled-dag.schema.json (reduce_contract) + docs/architecture/schemas/trace-event.schema.json (conflict trace evidence) | D/E |
| ADR-0003-AF-001 | 0003 | Dispatcher | docs/architecture/schemas/quality-gate-report.schema.json | B |
| ADR-0003-AF-002 | 0003 | Dispatcher | docs/architecture/schemas/quality-gate-report.schema.json | B |
| ADR-0003-AF-003 | 0003 | Dispatcher | docs/architecture/schemas/trace-event.schema.json | B/E |
| ADR-0003-AF-004 | 0003 | Dispatcher | docs/architecture/schemas/trace-event.schema.json | B/E |
| ADR-0003-AF-005 | 0003 | Dispatcher | docs/architecture/schemas/execution-outcome.schema.json | B/E |
| ADR-0003-AF-006 | 0003 | Dispatcher/Policy | docs/architecture/schemas/policy-decision.schema.json | B/C |
| ADR-0003-AF-007 | 0003 | Dispatcher | docs/architecture/schemas/replay-bundle.schema.json | D |
| ADR-0004-AF-001 | 0004 | Policy Engine | docs/architecture/schemas/policy-decision.schema.json | C |
| ADR-0004-AF-002 | 0004 | Policy Boundary | docs/architecture/schemas/policy-decision.schema.json | C |
| ADR-0004-AF-003 | 0004 | Policy Boundary | docs/architecture/schemas/policy-decision.schema.json | C |
| ADR-0004-AF-004 | 0004 | Policy Boundary | docs/architecture/schemas/policy-decision.schema.json | C |
| ADR-0004-AF-005 | 0004 | Policy/Data Plane | docs/architecture/schemas/trace-event.schema.json | C/E |
| ADR-0004-AF-006 | 0004 | Policy Boundary | docs/architecture/schemas/policy-decision.schema.json | C |
| ADR-0005-AF-001 | 0005 | Trace Pipeline | docs/architecture/schemas/trace-event.schema.json | E |
| ADR-0005-AF-002 | 0005 | Replay Harness | docs/architecture/schemas/replay-bundle.schema.json | D |
| ADR-0005-AF-003 | 0005 | Replay Harness | docs/architecture/schemas/replay-bundle.schema.json | D |
| ADR-0005-AF-004 | 0005 | Gate Runner | docs/architecture/schemas/quality-gate-report.schema.json | B |
| ADR-0005-AF-005 | 0005 | Gate Runner | docs/architecture/schemas/quality-gate-report.schema.json | A |
| ADR-0005-AF-006 | 0005 | Gate Runner | docs/architecture/schemas/quality-gate-report.schema.json | C |
| ADR-0006-AF-001 | 0006 | Orchestration Persona Router | docs/architecture/design/06_persona_skill_and_tools_canon.md | E |
| ADR-0006-AF-002 | 0006 | Orchestration Persona Router | docs/architecture/design/06_persona_skill_and_tools_canon.md | E |
| ADR-0006-AF-003 | 0006 | Context Loader | docs/architecture/design/06_persona_skill_and_tools_canon.md | E |
| ADR-0006-AF-004 | 0006 | Context Loader | docs/architecture/design/06_persona_skill_and_tools_canon.md | D/E |
| ADR-0006-AF-005 | 0006 | Context Loader | docs/architecture/design/06_persona_skill_and_tools_canon.md | D |
| ADR-0006-AF-006 | 0006 | Policy Boundary | docs/architecture/schemas/policy-decision.schema.json | C |
| ADR-0006-AF-007 | 0006 | Skill Router | docs/architecture/design/06_persona_skill_and_tools_canon.md | E |
| ADR-0006-AF-008 | 0006 | Skill Router | docs/architecture/design/06_persona_skill_and_tools_canon.md | E |
| ADR-0006-AF-009 | 0006 | Tools Reference | docs/architecture/design/06_persona_skill_and_tools_canon.md | E |
| ADR-0006-AF-010 | 0006 | Tools Availability | docs/architecture/design/06_persona_skill_and_tools_canon.md | E |
| ADR-0006-AF-011 | 0006 | Expert Return | docs/architecture/design/06_persona_skill_and_tools_canon.md + docs/architecture/schemas/trace-event.schema.json | E |
| ADR-0006-AF-012 | 0006 | Orchestration Persona Router | docs/architecture/design/06_persona_skill_and_tools_canon.md | E |
| ADR-0006-AF-013 | 0006 | Instruction Loader | docs/architecture/design/06_persona_skill_and_tools_canon.md + `.agentx/instructions/README.md` | E |
| ADR-0006-AF-014 | 0006 | Context Assembly | docs/architecture/design/06_persona_skill_and_tools_canon.md (Layer 0) | D/E |
| ADR-0006-AF-015 | 0006 | Fingerprinting | docs/architecture/design/06_persona_skill_and_tools_canon.md + docs/architecture/schemas/trace-event.schema.json | D |
| ADR-0006-AF-016 | 0006 | Instruction Loader | docs/architecture/design/06_persona_skill_and_tools_canon.md | C |
| ADR-0006-AF-017 | 0006 | Determinism | docs/architecture/design/06_persona_skill_and_tools_canon.md | D |

| ADR-0006-AF-018 | 0006 | Instruction Router | docs/architecture/design/06_persona_skill_and_tools_canon.md (Priority Table) | D |
| ADR-0006-AF-019 | 0006 | Fingerprinting | docs/architecture/design/06_persona_skill_and_tools_canon.md (Explicit Algorithm) | D |
| ADR-0006-AF-020 | 0006 | Immutability Guard | docs/architecture/design/06_persona_skill_and_tools_canon.md (Layer 0 Enforcement) | C |
