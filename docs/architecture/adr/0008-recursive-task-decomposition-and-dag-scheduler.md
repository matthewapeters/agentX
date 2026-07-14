# ADR 0008: Recursive Task Decomposition and the DAG Scheduler

Status: Proposed; **amended 2026-07-07** (typed DAG nodes — see amendment at the end)
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
   re-synthesis; this ADR defers. **Resolved by ADR 0010:** only a `refuted` assertion
   outcome (real evidence of failure) licenses re-decomposition; an `abstained` outcome
   (inconclusive evidence) may only retry the check, never rework the plan.
3. **Dynamic/TBD steps.** The prior doc's TBD steps (resolve a step after its prerequisites
   complete) fit naturally as NEEDS-DECOMPOSE nodes whose expansion is deferred until deps
   are `done`. Adopt as-is, or require concrete steps up front? Leaning adopt.
4. **Approval granularity.** Is approval per atomic leaf, per plan, or a budget the user
   grants the whole DAG? Ties into the workdir-confinement/approval work already landed.
5. **Slot fairness.** With one shared budget, does decomposition get a reserved minimum so a
   wide execution frontier can't starve planning (deadlocking progress)? Likely yes.

---

## Amendment (2026-07-07): Typed DAG Nodes — Step vs. Task

**Supersedes:** §2 ("The atomicity oracle") as the *primary* dispatch mechanism, and the
`decompose.ForceRoot` / `HeuristicOneStep` / echo-similarity guards built as inference-time
patches around it (session `tidy-cove`, `nimble-otter`). Those guards existed because
atomicity was *inferred* from goal text after generation. This amendment has the planner
*declare* it at generation time instead, which removes the inference problem — and the
whole class of guard it required — by construction.

### Context for the amendment

Three sessions (`tidy-cove`, `mellow-meadow`, `nimble-otter`) each exposed a variant of the
same root cause: the scheduler asks an LLM-backed Oracle "is this atomic?" for *every* node,
and a wrong answer either spirals (non-atomic verdict on an already-atomic goal → re-plan
forever, caught by the echo guard) or under-decomposes (atomic verdict on a compound goal
because a lexicon-based heuristic missed a verb, caught by `ForceRoot`). Each fix added
another inference-time patch. The user's proposal (2026-07-07) inverts the mechanism:
**the planner labels every node it emits as a Step or a Task. The scheduler dispatches on
that label — it never asks an Oracle to guess.**

### Decision

**A DAG node is one of two kinds**, replacing the single untyped `task.Record` used as
both a plan node and an executable leaf. Every node in the graph — root included — points
to either a Step or a Task; there is no third kind and no bare/untyped node:

- **Step** (plan node) — `Decompose()` asks the LLM to break the goal into **at most 5**
  child units, **preferring Task children where the child is already a single concrete
  action, and only using a child Step where it genuinely isn't** — a Step never executes
  directly. This preference is what keeps plans low-resolution at each level: a node only
  stays a Step, and drills down further, when it truly cannot yet resolve to one tool call.
- **Task** (tool node) — `Execute()`s exactly once, via the existing proposer/executor
  path, exposes its outcome through `Results()`, and **never decomposes** — there is no
  decompose operation for a Task to call. This is what makes a tool-call loop structurally
  impossible, not merely guarded against.

`task.Record` gains a `Kind` field:

```go
type Kind string

const (
    KindStep Kind = "step" // plan node: Decompose() only
    KindTask Kind = "task" // tool node: Execute()/Results() only
)
```

This is the wire/persisted shape — unchanged JSON envelope, one new field, so the existing
graph, session-event persistence, and plan-widget rendering (ADR 0009 §9c) all carry over
without a schema break.

**Two Go interfaces**, both embedding a common `Node` accessor shape, formalize the
contract for anything that consumes a typed node (the scheduler, a future WM-promotion
step, the plan widget):

```go
// Node is the read-only shape every DAG entry exposes, regardless of kind.
type Node interface {
    ID() string
    Goal() string
    Kind() task.Kind
    Deps() []string
    Depth() int
}

// Step is a plan node: decomposes into ≤5 children, never executes.
type Step interface {
    Node
    Decompose(ctx context.Context) (children []task.Record, synthesis string, err error)
}

// Task is a tool node: a leaf that never decomposes.
type Task interface {
    Node
    Execute(ctx context.Context) (executor.Outcome, error)
    Results() (executor.Outcome, bool) // ok=false before Execute has run
}
```

The scheduler's dispatch stays a cheap switch on `rec.Kind` — it does not need to construct
a `Step`/`Task` value just to decide which collaborator to call. The interfaces exist for
code that *holds* a typed node afterward (the WM-promotion path below is the first
consumer) rather than for the dispatch loop itself.

**Lean (placement):** these interfaces live in `internal/runtime/scheduler` (next to the
existing `Oracle`/`Decomposer`/`Executor` collaborator interfaces they replace), not in
`internal/prompting/task` — that package stays pure data model (`Record`, `Kind`, `Graph`).
Open for the user to override.

### What retires from the primary path

The atomicity **Oracle is no longer called in the scheduler loop.** `PipelineActions`,
`HeuristicOneStep`, and `decompose.ForceRoot` are retired as gating mechanisms — the
planner's declared `Kind` *is* the atomicity decision now, decided once at generation
time instead of re-inferred at every dispatch. (`HeuristicOneStep`'s clause-detection logic
may be repurposed as a non-blocking lint on the planner's own output — flag a suspicious
Task goal in the widget — but it no longer decides what runs.) This is a real complexity
reduction: three files, two prompt-tuning surfaces, and the whole echo-similarity guard
collapse into one field.

The classifier's action-type resolution (`Type`: artifact/command/query) is unaffected — it
still stamps a `Type` on records for policy/display purposes, orthogonal to `Kind`.

### Constraints, enforced in `planner.Parse`

- Every step **must** declare `kind` (`"step"` or `"task"`) — no default. An omitted or
  unknown kind is a parse error (same tolerant-JSON path via `internal/jsonx`, same retry
  budget as today).
- **≤ 5 children** per decomposition, counting Steps and Tasks together. A 6th step is a
  parse error, not a silent truncation (dropping the 6th could drop the synthesis-critical
  step).
- A Task's `Deps` are set once at creation for ordering (a Task can still wait on a sibling)
  and are **never appended to** — only a Step gains additional `Deps` entries, and only via
  its own `Decompose` (parent-as-join, unchanged from the original decision). The scheduler
  enforces this structurally by never invoking `Decompose` on a `KindTask` record — not by
  checking after the fact.

### Degrade-on-violation (replaces `ErrNoProgress`)

A `Decompose` call that violates a constraint (>5 children, missing `kind`, or the
echo/no-progress case from `tidy-cove` — now just one instance of "decompose failed",
not a special mechanism) gets **one retry**, with the specific violation named in the
regenerated prompt. A second failure **demotes the node to a Task for a single execution
attempt** via the proposer — never a hard `Failed` on decomposition trouble alone, and
never a third attempt. This subsumes and generalizes the old echo-guard fallback under one
rule, and preserves the anti-spiral guarantee: at most one retry + one execute attempt, per
node, ever. The plan widget (ADR 0009 §9c) renders a degraded node truthfully — 🔧 instead
of ⑂, with the demotion reason in its body — so the visibility contract holds even on the
failure path.

### Root node

The request classifier already asserted "multi-step" to route `invoke_planner` in the
first place, so the plan-cycle root is constructed as `KindStep` directly — no wrapper
needed. This retires `decompose.ForceRoot` entirely rather than keeping it as a shim.

### Prompt changes (planner)

The planner prompt must teach the Step/Task distinction with a worked example, not just
state it — "decompose vs. execute" is exactly the ambiguity that caused `tidy-cove` and
`nimble-otter`. It must also state the drill-down preference explicitly as a rule, not
leave it implicit in the example: **for each child, prefer `kind: "task"` — a single
concrete action — and only use `kind: "step"` when the child genuinely cannot yet resolve
to one tool call.** A plan that is all Steps has deferred every real decision to a later
round; a plan that is all Tasks when the goal is still coarse will under-decompose (the
`nimble-otter` failure). The preference or bias must run *toward* Task at every level —
drilling down only happens where it's actually still needed.

Two examples, both drawn from this project's own review fixture so they are concrete
rather than abstract:

**Example A — a plan (Step), low resolution, 5 nodes:**

> Goal: "review the current project and suggest one feature improvement"
>
> ```json
> {"plan_name": "Project Review",
>  "synthesis": "Investigate structure, docs, and source; the assistant recommends an improvement from these findings.",
>  "steps": [
>    {"id": "s1", "kind": "task", "goal": "List the top-level directory structure", "deps": [], "cost": 1},
>    {"id": "s2", "kind": "task", "goal": "Read the project README", "deps": [], "cost": 1},
>    {"id": "s3", "kind": "step", "goal": "Review source code for existing feature coverage and quality issues", "deps": ["s1"], "cost": 3},
>    {"id": "s4", "kind": "task", "goal": "Check test coverage across modules", "deps": ["s1"], "cost": 2},
>    {"id": "s5", "kind": "task", "goal": "Read the CHANGELOG for recently shipped work", "deps": [], "cost": 1}
>  ]}
> ```
>
> Four leaves resolve to one tool call each (`kind: "task"`) — no further planning needed.
> `s3` is still coarse ("review source code" spans many files) and is `kind: "step"`: once
> `s1` completes, it will itself decompose into ≤5 further children. Note there is **no**
> "recommend the feature" node — that synthesis happens after the plan drains, over the
> gathered findings; it is not a DAG node at all.

**Example B — a single tool call (Task), from dispatch to Results():**

> `s1` above (`{"id": "s1", "kind": "task", "goal": "List the top-level directory structure"}`)
> is a leaf. The scheduler never calls Decompose on it — it goes straight to the existing
> proposer/executor path:
>
> ```
> Execute(s1) → proposer resolves {"tool": "list_dir", "args": {"path": "."}}
>            → policy gate → run → verify
>            → Outcome{Status: Executed, Result: {...}}
> Results()  → the same Outcome, readable again without re-running —
>              this is what a later WM-promotion step (below) reads from.
> ```

### Working-memory TTL (a smaller, concrete slice worth building now)

Some Task results are worth remembering across a session without re-fetching — "the
project's dominant language," "the top-level structure" — things that change rarely but
get referenced by many later Steps. `session.Fact` gains an optional expiry so this is
possible without a new storage layer:

```go
type Fact struct {
    Key       string
    Value     string
    Owner     Owner
    Enabled   bool
    ExpiresAt *time.Time `json:"expires_at,omitempty"` // nil = no expiry (all current facts)
}
```

Backward compatible (every existing fact has `ExpiresAt == nil`). WM read paths
(`Facts()`/`Fact()`) filter expired entries. This is small, additive, and useful on its own
(e.g., a read grant's Once/Session distinction could eventually use it) even before
anything promotes a Task's `Results()` into WM.

**Deferred — not scoped in this amendment:** the actual *promotion policy* (which results
qualify, who decides "doesn't change much but is referenced often," default TTL). That
needs product judgment this amendment doesn't presume; tracked as Open Question 6 below.

### Open Question (forward note, not scoped): WM as a pluggable backend

The user raised — correctly — that TTL-bearing facts start to look like a small KV cache,
and asked whether WM could eventually sit behind a `Get/Set/Delete` + TTL interface with a
Redis (or similar) backend instead of the current session-file store. This ADR does not
scope that: it's a storage-backend swap with no behavioral need yet (session-local files
are correct for a local-first, single-user app). Flagged here so the `ExpiresAt` field
above is *shaped* compatibly with that future (TTL as data, not as file-store-specific
logic) without committing to build the seam now.

### Consequences (amendment)

Positive:

- Removes an entire class of inference-time guard (echo-similarity, clause-verb lexicon,
  `ForceRoot`) by moving the decision to generation time, where the planner already has
  full context.
- Removes an LLM call (the Oracle) from every single dispatch — real latency win; roughly
  half of `mellow-meadow`'s per-node ~10s was oracle voting.
- The plan widget (ADR 0009 §9c) can render Step (⑂, will expand) vs. Task (🔧, will run)
  *before* either happens — strengthens the pre-execution visibility contract, doesn't just
  coexist with it.
- Bounded by construction: ≤5 children × `DefaultMaxDepth` (3) means a worst-case plan is
  small and every path provably terminates in a non-decomposable node.

Trade-offs:

- Correctness now depends on the planner's `kind` compliance rather than a separate
  verdict; mitigated by strict `Parse` validation + the one-retry-then-degrade rule, which
  bounds the damage of a misdeclared node to "one wasted attempt," never a loop.
- Two new interfaces plus a field is more ceremony than one field would be alone; justified
  by giving future consumers (WM promotion, the widget) a checked contract instead of a
  string tag to switch on.

### Open Questions (amendment)

6. **Promotion policy.** What decides a Task's `Results()` gets written to WM with a TTL —
   an explicit planner-declared flag (`"cacheable": true`), a fixed allowlist of goal
   shapes, or a later heuristic? Deferred; needs product input before building.
   **Resolved by ADR 0010:** promotion is gated on assertion outcome, not goal shape — a
   plan's WM entry updates only on `satisfied`/`refuted` (evidenced conclusions), never on
   `abstained`; `ExpiresAt` (this amendment's field) is set at plan-close time.
7. **Retry-prompt shape.** The one-retry-on-violation path needs the specific violation
   (">5 children" / "missing kind" / "echoes parent") fed back into the regenerated
   prompt — a small `Render` variant, not a new template. Implementation detail, not
   architecture; noted so it isn't lost.
