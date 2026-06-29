# Feature: ADR 0002 DAG Taxonomy and Pattern Compiler

Schema links:

- `docs/architecture/schemas/compiled-dag.schema.json`

```gherkin
@adr0002 @compiler @positive
Scenario: ADR-0002-AF-001 Fast-path DAG contains canonical nodes
  Given a direct intent profile
  When pattern compiler emits DAG
  Then node kind values are input, classify, llm, and respond
  And DAG validates against compiled-dag schema

@adr0002 @compiler @positive
Scenario: ADR-0002-AF-002 Tool-assisted DAG includes planning and reduction
  Given a tool-assisted intent profile
  When pattern compiler emits DAG
  Then DAG includes plan and reduce nodes
  And llm and tool branch nodes feed into reduce before respond

@adr0002 @compiler @positive
Scenario: ADR-0002-AF-003 High-assurance DAG includes Guard and Persist
  Given a high-assurance intent profile
  When pattern compiler emits DAG
  Then guard and persist are mandatory predecessors of respond

@adr0002 @compiler @negative
Scenario: ADR-0002-AF-004 Compiler rejects invalid taxonomy
  Given a compile rule producing unknown node kind or cycle
  When compile is requested
  Then compile fails with structured validation error
  And no executable DAG is produced

@adr0002 @compiler @determinism
Scenario: ADR-0002-AF-005 Compiler node ids are deterministic
  Given same intent profile, config fingerprint, and seed
  When DAG is compiled twice
  Then node ids and graph_hash are identical

@adr0002 @compiler @positive
Scenario: ADR-0002-AF-006 Retry and timeout metadata is complete
  Given a compiled DAG
  When each node is inspected
  Then each node has retry_class and timeout_class values

@adr0002 @compiler @negative
Scenario: ADR-0002-AF-007 Reduce conflict resolution is deterministic
  Given conflicting branch outputs entering Reduce node
  When Reduce executes
  Then conflict resolution follows deterministic reduce_contract.strategy and reduce_contract.tie_breaker
  And unresolved conflicts follow reduce_contract.unresolved_policy
  And trace metadata includes reduce_contract.strategy and reduce_contract.tie_breaker
```
