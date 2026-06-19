# Feature: ADR 0001 Control, Execution, and Data Planes

Schema links:

- `docs/architecture/schemas/compiled-dag.schema.json`
- `docs/architecture/schemas/request-envelope.schema.json`
- `docs/architecture/schemas/execution-outcome.schema.json`
- `docs/architecture/schemas/trace-event.schema.json`

```gherkin
@adr0001 @control_plane @positive
Scenario: ADR-0001-AF-001 Fast path compiles and executes as degenerate DAG
  Given a request envelope classified as direct
  When the control plane compiles and dispatches the request
  Then the execution order is Input then Classify then LLM then Respond
  And no Plan, Reduce, Guard, or Persist node is required
  And compiled DAG output validates against docs/architecture/schemas/compiled-dag.schema.json

@adr0001 @control_plane @negative
Scenario: ADR-0001-AF-002 Execution worker cannot mutate control state
  Given a running node worker
  When the worker attempts a direct graph-state mutation
  Then the mutation is rejected
  And trace event captures stage dispatch, status failed, and non-empty reason_codes
  And only control-plane transition APIs can change task lifecycle

@adr0001 @execution_plane @positive
Scenario: ADR-0001-AF-003 Worker outcomes are normalized
  Given heterogeneous runtime failures and successes from workers
  When the execution plane normalizes results
  Then status is one of success, retriable_failure, terminal_failure, timeout, or canceled

@adr0001 @timeout
Scenario: ADR-0001-AF-004 Timeout transitions are deterministic
  Given a node with timeout class exceeded
  When control plane receives timeout outcome
  Then outcome status is timeout in docs/architecture/schemas/execution-outcome.schema.json
  And mapped trace-event status is timed_out in docs/architecture/schemas/trace-event.schema.json
  And next transition is deterministic from node retry_class and attempt budget

@adr0001 @negative
Scenario: ADR-0001-AF-005 Cancellation supersedes queued retries
  Given a task with retriable failure queued for retry
  When cancel is requested before retry dispatch
  Then task transitions to canceled
  And execution-outcome status is canceled
  And mapped trace-event status is canceled
  And retry attempt count does not increment after cancellation
  And no additional node dispatch occurs

@adr0001 @data_plane @positive
Scenario: ADR-0001-AF-006 Data plane writes are append-oriented
  Given multiple events for one correlation id
  When events are persisted
  Then records are append-only and immutable
  And each record includes required linkage keys correlation_id, task_id, node_id, and run_id
```
