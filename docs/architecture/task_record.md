# Task Record — the durable unit of work (Family A)

Last updated: 2026-07-04
Status: Design draft (pre-implementation)
Owner: Runtime / Orchestration

## Why this exists

The cascade classifier ([`cascade_classifier.md`](cascade_classifier.md)) turns a
recognized action into a **durable record**, not an in-context intention. This is the
fix for the `vivid-willow` failure at the storage layer: an action the model commits to
must survive outside the conversation, in a form a separate executor can drain — so it
cannot be lost to context-overload salience decay.

The record is **DAG-node-shaped from day one**. Today the classifier emits one task and
the executor runs it (Family A). The day it emits a *plan* — several tasks with `deps` —
the same records become a DAG and the executor becomes an orchestrator draining it in
topological order (Family B), with no schema change. **This record is the Family-A/B
seam.**

Machine schema: [`task-record.schema.json`](task-record.schema.json).

## The record

| Field | Role |
|-------|------|
| `id` | Session-unique key; the DAG node identity that `deps` reference. |
| `goal` | The extracted objective, natural language. |
| `type` | `artifact` \| `command` \| `query` — selects executor path + policy gate. |
| `status` | Lifecycle state (below). |
| `stakes` | `low` \| `high`; high (write/exec) always escalates + may need confirmation. |
| `params` | Type-specific (path/content, argv, query target); validated by the executor. |
| `deps[]` | Prerequisite task ids — empty in Family A, an edge list in Family B. |
| `provenance` | How it was classified: source (turn/response/reconciler), escalated, confidence, vote spread, model. |
| `verification` | Executor's observed effect; `done` requires `verified=true`. |
| `result_ref` | Link to the `tool_result` event this task produced. |
| `created_epoch` / `updated_epoch` | Temporal metadata, first-class. |

## Lifecycle

```
              ┌──────────── abstained ─────────── (classifier unsure → ask user)
              │
proposed ──▶ ready ──▶ in_progress ──▶ done      (verified effect)
   │           ▲                    └─▶ failed    (exec or verification failed)
   │           │
   └─▶ cancelled (superseded/withdrawn)
               └── deps satisfied ──┘
```

- **proposed** — the classifier emitted it (turn or response path).
- **ready** — the reconciler routed it for execution and its `deps` are all `done`.
- **in_progress** — the executor drained it and issued the tool call.
- **done** — executed **and** `verification.verified = true`. Only now may the surface
  report success (no "Created X" without an observed X).
- **failed** — execution errored, or the effect could not be verified.
- **abstained** — classification was not confident enough (scattered vote / below
  threshold); the turn is routed to a one-line clarifying question rather than a guess.
- **cancelled** — superseded by a later turn or explicitly withdrawn.

## Persistence — append-only event log, in-memory DAG

This is the design the earlier neo4j/Temporal discussion converged on: **separate the
DAG's runtime form from its persistence**, and reuse the event log AgentX already has.

- **Durability = the existing append-only event log.** A new task is a `task_proposed`
  event; every transition is a `task_updated` event. Both carry a TaskRecord payload.
  The **current state of a task is the latest event for its `id`** — no in-place
  mutation, full provenance of every change (the property neo4j temporal edges were
  wanted for, obtained for free over the log AgentX already persists).
- **Live plan = a plain Go in-memory DAG** rebuilt from those events: `map[id]*Task`
  with `deps` as slices. Sub-millisecond point lookups ("what's ready?"), freely
  mutable as ground truth is discovered. This is the "80% in Go" path.
- **No neo4j, no Temporal.** They stay deferred (neo4j only if cross-plan relational
  queries are ever needed — materialize a graph view from events then; Temporal only
  if a task becomes a long-running operational process needing restart recovery, e.g. a
  future FDE automation, not the core agent loop).

## Provenance and verification

- `provenance` records the classification path: whether it escalated to a Tier-2 vote,
  the measured confidence (vote agreement share), and the `vote_spread`
  (`fanout.Decision.Spread`). A classification is therefore fully answerable from the
  log — "why was this treated as a write?" is a query, not a guess.
- `verification` is the executor's close on the false-success class: after a
  `write_file` it stats the file; after a command it checks exit/output; it stamps
  `verified` + `detail` + `checked_epoch`. `done` is unreachable without it.

## Adoption notes (freeze boundary)

- The persisted **event-envelope `content_type` enum is frozen v1**
  (`runtime_contracts/event-envelope.schema.json`). Adopting task records adds
  `task_proposed` and `task_updated` to that enum — a **coordinated, breaking change**
  to a frozen contract (bump per the freeze semantics), tracked in
  `../implementation/90_open_questions.md`. Until then this schema is a **draft** and
  lives here, not in the frozen `runtime_contracts/` folder.
- On implementation, promote `task-record.schema.json` into `runtime_contracts/`, add
  it to that index, and give every touched function a GIVEN/WHEN/THEN feature first
  (doc-first invariant).

## Deferred / open questions

- Whether `task_updated` carries the full record or a status delta (log size vs.
  reconstruction cost).
- `params` sub-schemas per `type` (artifact/command/query) — validated by the executor
  now; may be frozen later.
- Compaction/retention of superseded (`cancelled`/`failed`) task events.
- The reconciler's exact mapping from the classifier routing matrix to `status`
  transitions (specified in `cascade_classifier.md` §6; the state-machine wiring is its
  own implementation feature).
