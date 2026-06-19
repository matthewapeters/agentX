# ADR 0002: DAG Node Taxonomy and Pattern Compiler

Status: Accepted
Date: 2026-06-14
Deciders: AgentX architecture owners

## Context

AgentX supports both direct responses and orchestrated multi-step work. Existing planning artifacts describe hierarchical execution ideas, but there is no single actionable taxonomy for node behavior, retries, fan-out/fan-in, or policy hooks.

A fixed node taxonomy and compiler from intent to executable DAG are required to avoid ad hoc orchestration logic and to preserve compatibility with fast-path Q/A.

## Decision

Introduce a canonical node taxonomy and a pattern compiler that maps classified intent into executable DAGs.

Node taxonomy:

- Input Node: validates and normalizes request/context.
- Classify Node: maps request to route and complexity profile.
- Plan Node: emits executable subgraph for moderate/complex work.
- LLM Node: executes model call with policy-checked prompt envelope.
- Tool Node: executes registered tool action with typed arguments.
- Reduce Node: merges branch outputs and resolves conflicts.
- Guard Node: evaluates policy, budget, and quality assertions.
- Persist Node: writes stable artifacts and trace summaries.
- Respond Node: emits user-visible output.

Pattern compiler behavior:

- Fast path pattern: Input -> Classify -> LLM -> Respond.
- Tool-assisted pattern: Input -> Classify -> Plan -> (LLM/Tool branches) -> Reduce -> Respond.
- High-assurance pattern: includes Guard and Persist nodes before final Respond.
- Every compiled DAG must include correlation IDs, deterministic node IDs, timeout policy, and retry class per node.

Ownership:

- Compiler output is owned by control plane.
- Execution semantics are owned by execution plane workers.
- Node traces are written by data plane.

## Consequences

Positive:

- Preserves current direct flow while introducing a uniform DAG substrate.
- Makes orchestration extensible: new node kinds can be introduced with explicit contracts.
- Simplifies policy placement by using Guard nodes instead of scattered checks.

Trade-offs:

- Requires migration of existing implicit sequencing logic into explicit node definitions.
- Compiler defects can impact many routes at once; test coverage must be broad.

Operational implications:

- Need golden test fixtures for compiled DAGs per prompt class.
- Need compatibility mode where legacy paths produce equivalent compiled graphs.

## Next Steps

1. Define node interface contract (inputs, outputs, retries, timeout class).
2. Implement compiler v1 for direct and tool-assisted patterns only.
3. Add golden DAG snapshot tests for representative prompt classes.
4. Add Guard node policy checks for budget, safety, and tool allowlist.
5. Roll out by route: start with simple Q/A, then add complex task classes.
