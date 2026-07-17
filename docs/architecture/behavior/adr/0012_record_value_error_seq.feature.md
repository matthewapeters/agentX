# Behavior — `task.Record` Value/Error/Seq (ADR 0012 Phase 4, amendment)

Status: **Implemented** (2026-07-17). Realizes ADR 0012's 2026-07-17 amendment §2 and
Phased Build Plan (amendment) step 4. Schema-only: no engine wiring yet (that's step
5 for the continuous engine, step 7 for wavefront) — this phase only makes the
fields exist and behave correctly in isolation.

Built exactly as scoped below. Tests: `internal/prompting/task/graph_test.go` (new)
cover all three scenarios plus a repeated-update variant guarding that `Seq`
preservation holds across multiple updates to multiple nodes, not just one. Full
suite and `-race` on `task`/`scheduler`/`decompose` all clean — confirms the additive
fields don't disturb either engine's existing behavior.

## Problem

`task.Record` has no durable place for a resolved node's value or failure reason —
`Status` says *that* a node is Done or Failed, never *what* it resolved to or *why*
it failed. That information exists transiently today (`executor.Outcome`, discarded
`graph.Add`/`Update` errors) but never reaches the persisted graph. The ADR 0012
amendment's "graph is the blackboard" design depends on `Value` being durable — a
classify prompt's "working memory" is a render over `Status == Done` nodes' `Value`
fields, nothing else — so this has to be correct and race-free before anything reads
from it.

Separately, a serialized graph today has no way to show its own growth over time —
`Graph.Nodes()` returns first-seen order, but that's an iteration property of the
in-memory struct, not a durable field on the record itself, so it doesn't survive
serialization into a form that shows "node 7 existed before node 12."

## Design

### 1. Three new fields on `task.Record`, all additive

```go
Value string `json:"value,omitempty"` // resolved fact/answer, set once Status == Done
Error string `json:"error,omitempty"` // failure reason, set once Status == Failed
Seq   int    `json:"seq"`             // graph-assigned growth position
```

Engine-agnostic in meaning: both the continuous engine and wavefront populate them
under the identical rule ("write what this node's own processing actually produced;
leave empty otherwise"), never a different rule per engine. This phase does not wire
either engine to populate them — it only adds the fields and `Graph`'s bookkeeping
for `Seq`.

### 2. `Seq` is graph-assigned at `Add`, never caller-supplied

```
GIVEN a fresh Graph
WHEN  N records are Added in sequence
THEN  their Seq values are 0, 1, 2, ... N-1, in Add order, regardless of what Seq
      (if anything) the caller set on each Record before calling Add.
```

`Graph.Add` overwrites whatever `Seq` the caller passed, so ordering is authoritative
by construction and cannot be gamed or left inconsistent by a careless caller.

### 3. `Graph.Update` must preserve the existing node's `Seq` — never re-derive it from the incoming record

```
GIVEN a node already in the graph with Seq == 3
WHEN  Update is called for that node's id with a Record whose own Seq field is the
      zero value (unset, as almost every real call site's Record will be — Seq is
      not something callers are expected to set)
THEN  the stored node's Seq remains 3 after Update returns — never silently reset to
      0.
```

This is the one part of this phase most likely to be gotten wrong by omission (an
`Update` implementation that just stores the incoming record verbatim, the way it
does today for every other field, would silently corrupt every node's growth
position on its very first status-transition update) — it gets a dedicated
regression test, not just coverage via the general Update path.

### 4. `Value`/`Error` are plain pass-through fields at the `Graph` level

`Graph` does not validate, default, or interpret `Value`/`Error` — it stores whatever
the caller sets, exactly like `Goal`/`Params` today. Enforcing "only set on the
matching `Status`" is a caller discipline (steps 5 and 7), not something `Graph`
polices — consistent with `Graph`'s existing scope (referential integrity and
acyclicity only, per `validate`).

## Tests

- `internal/prompting/task/graph_test.go` (new or extended):
  - `TestAddAssignsMonotonicSeq` — several `Add` calls in sequence get `Seq` 0, 1, 2,
    ... regardless of any `Seq` value set on the input `Record`.
  - `TestUpdatePreservesSeq` — the regression guard for §3: `Add` a record, capture
    its `Seq`, `Update` it with a zero-value `Seq` on the incoming record, assert the
    stored node's `Seq` is unchanged.
  - `TestValueAndErrorPassThroughUnvalidated` — `Graph` stores whatever `Value`/
    `Error` a caller sets without interpreting them (e.g. a `Value` set alongside
    `Status: Failed`, or vice versa, is accepted — `Graph` does not police the
    pairing).
