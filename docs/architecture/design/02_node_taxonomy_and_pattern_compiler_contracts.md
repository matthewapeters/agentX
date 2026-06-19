# Design 02: Node Taxonomy and Pattern Compiler Contracts

Last updated: 2026-06-15
ADR linkage: 0002

## Schema Links (Authoritative)

- `docs/architecture/schemas/compiled-dag.schema.json`
- `docs/architecture/schemas/request-envelope.schema.json`

## Goal

Define canonical node taxonomy and deterministic pattern compiler interfaces that transform classified intent into executable DAGs.

## Node Taxonomy

- `input`
- `classify`
- `plan`
- `llm`
- `tool`
- `reduce`
- `guard`
- `persist`
- `respond`

## Core Types

```go
type NodeKind string

type NodeSpec struct {
    NodeID       NodeID
    Kind         NodeKind
    RetryClass   RetryClass
    TimeoutClass TimeoutClass
    PolicyProfile string
    Inputs       []string
    Outputs      []string
}

type PatternCompiler interface {
    Compile(intent IntentProfile, ctx CompileContext) (CompiledDAG, error)
    FastPath(ctx CompileContext) (CompiledDAG, error)
    ToolAssisted(ctx CompileContext) (CompiledDAG, error)
    HighAssurance(ctx CompileContext) (CompiledDAG, error)
}
```

## Compiler Output Requirements

Every compiled DAG must include:

- Deterministic node IDs for same intent+config seed.
- Retry class and timeout class per node.
- Correlation and task identifiers.
- Canonical graph hash field: `graph_hash` (SHA-256 hex, 64 chars).
- Edge conditions for branch and reduce behavior.
- Reduce-node conflict metadata contract keys when kind is `reduce`:
  - `strategy`
  - `tie_breaker`
  - `conflict_keys`
  - `unresolved_policy`
  - `emit_conflict_trace`

Determinism contract details:

- Determinism hash MUST be computed from semantic fields only (`dag_id`, `correlation_id`, `task_id`, `nodes`, `edges`, and deterministic metadata like `compiler_version` and `seed`).
- Producer MUST persist this value in `graph_hash` and validators MUST compare `graph_hash` across deterministic replay/fixture scenarios.
- Hash input MUST exclude `graph_hash` itself to avoid self-reference cycles.
- `metadata.compiled_at` is explicitly optional and non-semantic; when present, it MUST be excluded from determinism comparisons.

## Pattern Guarantees

- Fast path: `Input -> Classify -> LLM -> Respond`
- Tool-assisted path: includes `Plan`, branch nodes, `Reduce`, `Respond`
- High-assurance path: includes `Guard` and `Persist` before `Respond`

## Invariants

- Compiler rejects cyclic graphs and unknown node kinds.
- Compiler output validates against `compiled-dag.schema.json`.
- Guard/Persist are mandatory for high-assurance patterns.

## Conformance Checklist

- Golden DAG fixtures exist per major intent class.
- Determinism test verifies stable graph hash.
- Determinism test verifies stable `graph_hash` equality for semantically equivalent DAG compilations.
- Each node includes timeout/retry metadata.
- Compiled DAG includes both `correlation_id` and `task_id`.
- Reduce nodes include conflict metadata keys and emit conflict trace records when `emit_conflict_trace=true`.
