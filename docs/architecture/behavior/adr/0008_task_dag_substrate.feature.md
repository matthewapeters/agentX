# Feature: ADR 0008 Phase 1 — Task DAG Substrate

Status: **Implemented** (2026-07-06). Code: `internal/prompting/task/graph.go`.
Runnable contract: `tests/features/prompting/task_dag.feature` (@unit, UC-RTDAG-001…009).
The four open points below were resolved with the accepted leans (pure `task.Graph` fold,
no cache, per-node `Add`/`Update`, `task-<turn>-<n>` ids); live multi-node emission wiring
lands with the first producer (Phase 4 decomposer).

Realizes **Phase 1** of `docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md`:
*"Activate `task.Record.Deps` end-to-end: emit edges, persist the node set + a `task_tree`
index, load/replay it."*

Schema / source links:

- `docs/architecture/task-record.schema.json` (`deps[]` field — the edge list)
- `docs/architecture/task_record.md` (record shape, append-only lifecycle)
- `internal/prompting/task/task.go` (`Record`, `FromAction`)
- `internal/runtime/classifier_pipeline.go` (`maybeEmitTask` — current single-task emit)

## Scope

**In scope (Phase 1, pure data plumbing — no LLM, no scheduler, no branch context):**

- A record may carry a non-empty `deps` edge list, and that edge list survives persistence.
- The task DAG is a **projection over the append-only event log** — the log is the source
  of truth; any `task_tree.json` is a rebuildable cache, never authoritative.
- Reconstructing the DAG from events is **deterministic** (replay yields an identical graph).
- The projection exposes the read-only queries the Phase 3 scheduler will consume:
  `roots`, `dependents(id)`, and `ready` (all deps `done`). Read-only here — nothing runs.
- Structural integrity is enforced at admission: **no dangling edges, no cycles, unique ids.**

**Out of scope (later phases):** where deps *come from* (the decomposer, Phase 4); acting on
`ready` (the scheduler, Phase 3); branch context (Phase 2). Phase 1 proves that *given* a set
of records with deps, the substrate stores, reloads, queries, and guards them faithfully.

## Contract

- **Backward compatible.** A standalone Family-A task keeps `deps: []` and its emit/persist
  path is unchanged. The single-task scenarios in `task_record.feature` and
  `task_classifier.feature` must stay green.
- **Id scheme.** Ids are session-unique across *many* nodes in one turn (today's
  `task-<turn>` becomes `task-<turn>-<n>` or equivalent), because a plan emits several nodes
  per turn. `deps` entries are these ids.
- **Edges reference nodes.** Every id in a record's `deps` must resolve to a node already
  known in the session. A dangling edge is a structural error, refused at admission.
- **Acyclic.** A record whose `deps` would close a cycle is refused; the DAG stays a DAG.
- **Append-only, latest-wins.** A status change is a new `task_updated` event; the node's
  current state is the latest event for its id. Reconstruction folds the log; prior events
  are retained for replay/audit.

## Behavior

```gherkin
@adr0008 @phase1 @dag @positive
Scenario: ADR-0008-P1-001 A standalone task round-trips with empty deps
  Given a proposed task "t1" with no dependencies
  When the session events are persisted and reloaded
  Then the reconstructed DAG has one node "t1"
  And node "t1" has an empty deps edge list
  And the reload is byte-identical to a Family-A single-task session

@adr0008 @phase1 @dag @positive
Scenario: ADR-0008-P1-002 A record set with edges reconstructs the same DAG
  Given proposed tasks "a", "b", and "c" where "c" deps on "a" and "b"
  When the session events are persisted and reloaded
  Then the reconstructed DAG has nodes "a", "b", "c"
  And the edge set is exactly {a -> c, b -> c}
  And "a" and "b" are roots

@adr0008 @phase1 @dag @determinism
Scenario: ADR-0008-P1-003 The DAG is reconstructed from the event log, not the cache
  Given a persisted session with a task_tree.json cache present
  When the cache is discarded and the DAG is rebuilt from the raw event log alone
  Then the rebuilt DAG is identical to the cached DAG
  And the event log is treated as the source of truth

@adr0008 @phase1 @dag @query
Scenario: ADR-0008-P1-004 Roots are the nodes with no dependencies
  Given a DAG with nodes "a", "b", "c" and edges {a -> c, b -> c}
  When the roots are queried
  Then the roots are exactly "a" and "b"

@adr0008 @phase1 @dag @query
Scenario: ADR-0008-P1-005 A node is ready only when every dependency is done
  Given a DAG with edges {a -> c, b -> c}
  And "a" is done and "b" is proposed
  When the ready set is queried
  Then "c" is not ready
  When "b" transitions to done
  And the ready set is queried again
  Then "c" is ready

@adr0008 @phase1 @dag @negative
Scenario: ADR-0008-P1-006 An edge to an unknown node is refused
  Given a proposed task "x" that deps on unknown id "ghost"
  When the task is admitted to the DAG
  Then admission fails with a dangling-dependency error
  And no node "x" is added to the DAG

@adr0008 @phase1 @dag @negative
Scenario: ADR-0008-P1-007 A dependency cycle is refused
  Given nodes "p" and "q" where "p" deps on "q"
  When a record makes "q" dep on "p"
  Then admission fails with a cycle error
  And the DAG remains acyclic

@adr0008 @phase1 @dag @negative
Scenario: ADR-0008-P1-008 Node ids are unique within a session
  Given a node "dup" already exists in the session
  When a second record claims id "dup"
  Then admission fails with a duplicate-id error

@adr0008 @phase1 @dag @positive
Scenario: ADR-0008-P1-009 A status update supersedes append-only, latest wins
  Given a node "t1" with status "proposed"
  When a task_updated event sets "t1" to "done"
  Then the reconstructed node "t1" has status "done"
  And the prior "proposed" event is retained in the log for replay
```

## Open points to settle before building

1. **Where the DAG projection lives.** A new `internal/runtime` (or `internal/prompting/task`)
   type that folds events → graph, vs. extending the session store. Leaning: a pure
   `task.Graph` fold in the `task` package (no I/O), fed by the session event stream — keeps
   it unit-testable and matches `FromAction`'s "depends only on fanout" discipline.
2. **`task_tree.json` — cache now or defer.** The contract only needs *rebuild-from-events*.
   The cache is an optimization; we can ship Phase 1 with rebuild-only and add the cache when
   replay cost warrants it. Recommend: defer the cache, keep P1-003 as the guard that the log
   stays authoritative.
3. **Admission point.** Integrity checks (dangling/cycle/dup) belong at the moment a record
   enters the graph. Is that a `Graph.Add(rec)` returning an error, or a validating
   constructor over a whole record set? Leaning `Graph.Add` so it composes with lazy,
   one-node-at-a-time emission in later phases.
4. **Id scheme concretely.** `task-<turn>-<n>` is the obvious minimal change; confirm it
   won't collide across turns and that existing single-task ids (`task-<turn>`) either migrate
   or remain a valid degenerate form.
