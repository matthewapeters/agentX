# ADR 0008: Recursive Task Decomposition and the DAG Scheduler

Status: Proposed
Date: 2026-07-06
Deciders: AgentX architecture owners
Supersedes (in part): `docs/architecture/hierarchical_task_execution_plan.md` (2026-03-22 Draft) —
this ADR keeps that document's provenance/context-compression bones and replaces its
eager, LLM-driven `run_subtask` recursion with scheduler-driven **lazy** expansion.

## Context

The task classifier now emits durable, DAG-node-shaped `task.Record`s and reconciles a
turn to a route (`internal/prompting/reconcile`). Two routes are live end-to-end
(`Reify`, `Redispatch` → executor); `Verify`/`Confirm`/`None` are handled; and one route
— `Ask` — fires whenever a fan-group **abstains** but has nowhere to go yet. The reserved
`invoke_planner`/Plan-node path from ADR 0002 has no realization.

That abstain is not just noise. The action classifier was tuned (3-of-4 supermajority →
2-of-3 majority, `abstain_below = 0.6`) precisely so that a *genuinely* actionable turn
resolves and a *genuinely* complex one still scatters. A scattering turn — "review the
project and propose a feature" — is the classifier telling us the goal is **not atomic**:
it is not one artifact/command/query, it is many. Today that signal dead-ends at `Ask`.

We need a way to take a non-atomic goal and break it down until every leaf is something
the existing reconcile→executor path can run — without (a) undertaking any step before it
is understood, (b) blocking the whole plan on planning the whole plan, or (c) polluting
the main conversation context with the exploration.

Three constraints from the design owner shape the decision:

1. **Atomic-before-undertake.** A step is never executed until it has been decomposed to
   the atomic level. Planning is a precondition of running a node, not of the plan.
2. **Lazy + interleaved.** Decomposition is progressive — expand a node only when it
   becomes a candidate to run. A ready leaf may execute while its siblings are still being
   decomposed, provided no dependency (critical-path) edge is violated.
3. **Parallelism is a goal, not a byproduct.** The planner is pushed to emit independent
   sub-goals (minimize `Deps`) and the scheduler actively exploits available width.

## Decision

Introduce **recursive task decomposition** as the realization of the `invoke_planner`
route, structured as **three cooperating parts over a live `task.Record` DAG**, driven by
a scheduler rather than by in-context LLM tool calls.

### 1. The `decompose` node (planner), run in a branch context

`decompose(node)` is an internal, agentic node — not a user-facing tool and not a leaf
executor. It takes one non-atomic goal and emits **child `task.Record`s plus dependency
edges** (`Deps`). It runs in a **branch context**: an isolated sub-session with its own
event log and working memory, forked from the parent. Only the resulting plan fragment
(the child records + a short synthesis) returns to the parent; the branch's exploration
does **not** enter the main conversation context. This is the prior doc's
"context compression," applied to planning rather than execution.

The branch is **plan-only**: decomposition causes no *mutating* side effects. It runs as an
**investigating branch** (Phase 2, Model I) — it may run read-only tools to plan from
evidence, enforced by a **read-restricted catalog** (only `Risk == read` descriptors; write
and network tools are absent, not merely denied), under the executor's existing
workdir-confinement. Only atomic *leaves* mutate, and only through the full executor
(workdir-confinement + approval gate) once the plan merges back. See
`docs/architecture/behavior/adr/0008_task_branch_context.feature.md`.

The planner is prompted to **maximize independence** — emit steps with the fewest
dependency edges that correctness allows — so the scheduler has width to exploit. It emits
a rough per-step cost/priority so the scheduler can find the critical path (see part 3).

### 2. The atomicity oracle (type-resolution AND one-step satisfiability)

A first draft of this ADR said a node is atomic ⟺ `action_classify` resolves it
(non-abstained) to a single `task_type`. **The canonical review fixture
(`[[canonical-project-review-test-prompt]]`) disproves that.** "Review the current project
and identify one under-developed feature" classifies as `query` at **95% confidence,
non-abstained** — yet it is plainly compound (a multi-file investigation, not a lookup).
`query` is a defensible *type* but the wrong *atomicity*: the enum
`{artifact, command, query, none}` has no "compound" verdict, so a multi-step goal is
squashed into the nearest atomic label with false confidence. **Type-resolution is
necessary but not sufficient.**

Atomicity therefore has **two** conditions:

- **Type resolves** — `action_classify` is non-abstained on a single actionable type; and
- **One-step satisfiable** — that type can be discharged by a *single* executor action.

The **response classifier is the one-step detector.** Run speculatively on a proposed
leaf, it answers "would satisfying this be one tool call, or a narrated sequence of
steps?" In the fixture trace it abstained `{produced:2 · executed:1 · neither:1}` — the
model narrated a multi-step investigation instead of executing a single tool. That scatter
is the **decompose signal**, not an `Ask` signal (see the reconcile change below). Only a
node that passes *both* conditions is atomic and dispatchable to the executor.

Termination is threefold: a leaf terminates when it is atomic under *both* conditions; the
tree terminates at `max_task_depth` (guard rail, default 10, from the prior doc); and a
cycle/dep check rejects self- or back-references so recursion is bounded, not open-ended.
At `depth >= max_task_depth` a still-non-atomic node fails to `Ask` rather than recursing
— i.e. it surfaces a clarifying question instead of guessing.

### 3. The scheduler (readiness = deps-done ∧ atomic)

A single scheduling loop walks the DAG and drives every node through the **same** state
machine, treating "not yet atomic" as just another not-ready state. A node is:

- **BLOCKED** — some `Deps` entry is not yet `done`. Wait.
- **NEEDS-DECOMPOSE** — deps satisfied, but not atomic. Enqueue `decompose(node)` in a
  branch context; its children replace it as the frontier.
- **RUNNABLE** — deps satisfied **and** atomic. Dispatch through the existing
  reconcile→executor path.
- **DONE / FAILED** — terminal; unblocks dependents.

Because decomposition and execution are two transitions in one loop rather than two
phases, lazy + interleaved falls out for free: the scheduler expands a node only when it
reaches the frontier, and runs a ready leaf immediately even while siblings are mid-expand.

```
             ┌─────────────── scheduler loop (topological, live) ───────────────┐
 goal ──►    │  for each frontier node:                                          │
             │    deps not done         → BLOCKED (wait)                         │
             │    deps done, not atomic → NEEDS-DECOMPOSE ──► decompose(node)    │
             │                                                 (branch context)  │
             │                                                 ↳ children + Deps │
             │    deps done, atomic     → RUNNABLE ──► reconcile → executor      │
             │  fan out all RUNNABLE + NEEDS-DECOMPOSE nodes concurrently,       │
             │  bounded by the shared slot budget, critical path first           │
             └───────────────────────────────────────────────────────────────────┘
           atomicity test = action_classify resolves AND response says one-step (§ part 2)
```

**Concurrency budget is shared, not per-pool.** Per the Ollama fan-out curve
(`[[ollama-fanout-concurrency-curve]]`: throughput plateaus ~5–8 concurrent), the
scheduler's fan-out, the `decompose` planner's own reasoning, and the classifier
fan-groups all draw from **one** bounded slot budget. There is no third independent pool
to oversubscribe the local model. Fan-out width is clamped to slot count.

**Critical-path priority.** "Seek parallelization" and "respect the critical path" both
require the scheduler to know which chain is longest. v1: breadth-first, letting `Deps`
serialize what must serialize. v1.5: use the planner's per-step cost to prioritize the
longest dependency chain so it is never starved behind a wide-but-shallow branch.

### Where it plugs in

`decompose` is what the `invoke_planner` route dispatches to. The reconciler gains a
**`Decompose`** route, and — critically — it is reached on a path the current fold sends to
`Ask`. Today `reconcile.Reconcile` returns `Ask` the moment *either* signal abstains
(reconcile.go:54). The fixture shows why that is too blunt: an **actionable, non-abstained
turn** (`query@95%`) whose **response scatters** toward `produced` is not "uncertain" — it
is the model trying to do a *compound* thing in prose. That is the same insight as `Reify`
(model narrated instead of executing), generalized from one tool call to a plan:

- turn actionable + response `produced` (single) → **`Reify`** — reify one tool call.
- turn actionable + response abstains toward `produced` (multi-step narration) →
  **`Decompose`** — reify a *plan*.
- turn/response genuinely ambiguous (scatter including `none`/low confidence) → **`Ask`**.

So `Decompose` and `Ask` are siblings out of the abstain branch, discriminated by whether
the scatter looks like *compositeness* (bimodal across actionable outcomes → Decompose) or
*ambiguity* (→ Ask). Atomic leaves rejoin the existing `Reify`/`Redispatch`/`Verify`
routing unchanged — decomposition adds a front stage, it does not fork the executor path.

## Consequences

Positive:

- Realizes ADR 0002's Plan node and the reserved `invoke_planner` route on the current
  (bubbletea) architecture, reusing `task.Record.Deps` as the DAG substrate with no schema
  redesign — the Family A → Family B bridge the record was shaped for.
- Reuses the tuned classifier as the recursion terminator; no new "is it complex" model.
  The one-step condition (response classifier) closes the gap the review fixture exposed —
  a confident single-type verdict on a compound goal is no longer mistaken for atomic.
- Branch context keeps planning out of the main context window; the plan is auditable on
  its own, matching the prior doc's per-plan tab/named-entity model.
- Lazy expansion means large goals start executing early instead of paying a full
  plan-the-plan latency up front; parallelism is bounded by real slots, not wished-for.

Trade-offs:

- A live scheduler over a mutating DAG is materially more complex than the current linear
  pipeline; it needs its own state machine, cycle guard, and failure propagation.
- Shared slot budget means decomposition and execution contend; a decompose-heavy frontier
  can starve execution (and vice versa) without the critical-path priority in v1.5.
- Interleaving makes runs non-deterministic in ordering; replay/trace must key on the DAG,
  not wall-clock order, to stay reproducible.

Operational implications:

- Persist plan + node records per the prior doc's layout (`plan.json`, per-node records,
  `task_tree.json`) so a run is replayable from the DAG, not from ordering.
- The output surface needs a plan/DAG view (branch/join topology) — deferred; v1 can emit
  `task_diagnostic`-style events and render flat.
- Every touched function needs a GIVEN/WHEN/THEN behavior doc before implementation
  (repo invariant); the scheduler state machine is the highest-value contract to pin.

## Phased Build Plan

1. **DAG substrate.** Activate `task.Record.Deps` end-to-end: emit edges, persist the node
   set + a `task_tree` index, load/replay it. Behavior doc for record→DAG round-trip.
2. **Branch context.** A forked sub-session (own event log + working memory) that runs a
   node and returns only a synthesis + child records. Enforce plan-only (no side effects).
3. **Scheduler loop.** The readiness state machine (BLOCKED / NEEDS-DECOMPOSE / RUNNABLE /
   DONE / FAILED), single shared slot budget, cycle + depth guards, breadth-first v1.
4. **`decompose` planner fan-group.** A prompt fan-group (per `prompt_fan_groups.md`) that
   emits child sub-goals + `Deps` + per-step cost, pushed toward minimal edges. Wire the
   `invoke_planner`/`Decompose` route to it; classifier resolves atomicity of each child.
5. **Critical-path priority (v1.5).** Use per-step cost to prioritize the longest chain.
6. **Surface.** Plan/DAG view with branch/join topology; plan-as-context-panel-element.

## Open Questions

1. **Ask vs. Decompose discriminator.** How does the reconciler choose between "ambiguous →
   ask the user" and "compound → decompose"? Candidate: response-spread shape (bimodal
   toward `produced`/`executed` → decompose; scatter including `none`/low-confidence →
   ask), gated on an actionable, non-abstained turn. Needs data from real scattering turns.
   The canonical review fixture (`[[canonical-project-review-test-prompt]]`) is the first
   datapoint: `action=query@0.95`, `response=abstained{produced:2,executed:1,neither:1}`.
2. **Re-decomposition on failure.** When an atomic leaf fails, does its parent re-decompose
   with the failure as context, or does the plan surface the failure? Prior doc leans
   re-synthesis; this ADR defers.
3. **Dynamic/TBD steps.** The prior doc's TBD steps (resolve a step after its prerequisites
   complete) fit naturally as NEEDS-DECOMPOSE nodes whose expansion is deferred until deps
   are `done`. Adopt as-is, or require concrete steps up front? Leaning adopt.
4. **Approval granularity.** Is approval per atomic leaf, per plan, or a budget the user
   grants the whole DAG? Ties into the workdir-confinement/approval work already landed.
5. **Slot fairness.** With one shared budget, does decomposition get a reserved minimum so a
   wide execution frontier can't starve planning (deadlocking progress)? Likely yes.
