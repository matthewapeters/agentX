# Feature: ADR 0008 Phase 4 — Decomposition Integration (the live wiring)

Status: **Implemented** (2026-07-06) — all five slices landed; production decomposes live.
This was the largest phase and the first to touch the live LLM path. Six leans accepted
(async scheduler, scheduler-driven root, produced-majority discriminator, shared budget,
read-executor-now/UI-later, planner schema).

- **4a — Decompose route: DONE.** `reconcile.Decompose` + `ResponseSignal.LeansProduced` +
  the produced-majority discriminator (`responseSignal` spread test) + the `maybeEmitTask`
  route case (records "decompose"; scheduler wiring is 4d). Tests: reconcile.feature
  TC-RECON-007/008. The canonical fixture now routes to Decompose, not Ask.
- **4b — planner: DONE.** `internal/prompting/planner` — the plan prompt template + `Parse`
  (plan JSON → child `task.Record`s, local ids remapped to session-unique `parentID-<n>`,
  deps rewritten, malformed plans rejected). Not a corpus fan-group (a generator, not a
  vote). Tests: planner.feature UC-PLAN-001..003. Prompt tuned against the fixture in 4e.
- **4c — oracle + decomposer adapters: DONE.** `internal/runtime/decompose` — `Oracle`
  (atomic ⟺ action resolves ∧ one-step, via narrow `ActionClassifier`/`OneStepChecker`
  seams) and `Decomposer` (forks a Phase-2 branch, runs an injected `Planner` with the
  branch's WM-snapshot context, seals a `branch.Result`). Tests:
  task_decompose_adapters.feature UC-DEC-ORACLE-* / UC-DEC-DECOMPOSER (P4-005/006).
- **4d — pipeline/scheduler wiring: DONE.** `ContentTaskPlan` event; `WithDecomposition`
  option (injects `scheduler.Oracle` + `scheduler.Decomposer`); `maybeEmitTask` Decompose
  case runs `runDecomposition` in the background (async — turn not blocked) when wired, else
  records the route; `decompose.DrainPlan` is the pure seed-graph→run-scheduler seam,
  emitting `task_plan` + per-leaf `task_result`. Tested: drain_test.go (compound root →
  decompose → execute → join → all done). Note: an orchestrator-level end-to-end Decompose
  test is deferred — the fixed-verdict classifier harness produces unanimous votes and
  cannot synthesize the abstain-leaning-produced signal; the path is covered piecewise
  (4a route + 4c adapters + 4d drain). Per-node streaming (a scheduler observer) is a later
  refinement; 4d emits on completion.
- **4e — live wiring: DONE.** `decompose.PipelineActions` (action classifier over the live
  pipeline), `decompose.HeuristicOneStep` (interim one-step check — coordinated clauses /
  length; replaceable by a one-step fan-group), `decompose.LLMPlanner` (renders the planner
  prompt → Ollama `Complete` → `planner.Parse`). `Orchestrator.buildDecomposition()` wires
  these at Start (needs classifier + executor; planner `Facts` come from the session WM), so
  a Decompose-routed turn now runs a live plan. Tests: live_test.go (heuristic cases + LLM
  planner parses/namespaces with a stub chat). The `Planner` interface gained `parentID` so
  children are namespaced under the parent.

**Deferred (not blocking; explicitly out of Phase 4):** the interactive out-of-cwd read
approval UI (surface); re-decomposition on failure (ADR OQ2); cost-weighted critical-path
ordering (scheduler v1.5); per-node streaming (scheduler observer); a one-step fan-group to
replace the heuristic; a live-Ollama fixture eval as a gated test (kept as documented
contract in `task_decomposition.feature`, since the gate has no model).

Realizes **Phase 4** of `docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md`:
*"A prompt fan-group that emits child sub-goals + Deps + per-step cost … wire the
invoke_planner / Decompose route to it; the classifier resolves atomicity of each child."*

Phases 1–3 built the substrate (DAG, branch context + read-grants, scheduler) entirely
LLM-free, behind injected interfaces. Phase 4 makes the injected seams **real** and wires
them into `maybeEmitTask`, turning "review the project and propose a feature" from a
dead-ended `Ask` into an executed plan.

Schema / source links:

- `internal/prompting/reconcile/reconcile.go` (route fold — gains `Decompose`)
- `internal/runtime/classifier_pipeline.go` (`maybeEmitTask`, `buildTaskExecutor`)
- `internal/runtime/scheduler` (Phase 3 `Oracle`, `Decomposer`, `Executor`, `Scheduler`)
- `internal/runtime/branch`, `internal/session/readgrant.go` (Phase 2)
- `config/seed/prompts.toml` (fan-groups — gains `decompose_plan`)
- Fixture: `[[canonical-project-review-test-prompt]]` (the driving use-case)

## Scope

**In scope (Phase 4 — make the seams real and wire them in):**

1. **The `Decompose` route** (`reconcile`). A turn that is actionable and non-abstained but
   whose response scatters toward `produced` (the model narrated a *multi-step*
   investigation) folds to **`Decompose`**, a generalization of `Reify`: reify a *plan*, not
   one tool call. Genuine ambiguity (scatter including `none`/low confidence) stays `Ask`; a
   single `produced` stays `Reify`. (ADR §"Where it plugs in"; OQ1 discriminator.)
2. **The classifier-backed `Oracle`.** `Atomic(rec)` runs the tuned `action_classify` on the
   node's goal and the one-step check on its response: atomic iff it type-resolves AND is
   one-step (ADR §2). Reuses the existing pipeline classifier — no new model.
3. **The branch-backed `Decomposer`.** `Decompose(rec)` forks a Phase-2 investigating branch
   (read-restricted catalog + working-dir confinement + read-grants), runs the planner
   fan-group inside it, and seals a `branch.Result` of child records + synthesis.
4. **The `decompose_plan` fan-group** (`prompts.toml`). Emits a plan: ordered child
   sub-goals, `deps` edges (minimized for parallelism), an optional per-step cost, and a
   short plan synthesis. Child ids follow the Phase-1 `task-<turn>-<n>` scheme.
5. **Pipeline wiring.** On `Decompose`, `maybeEmitTask` seeds a `task.Graph` with the root
   record and runs the `Scheduler` with the real oracle/decomposer/executor and a slot
   budget — the scheduler drives decomposition→execution, emitting `task_proposed`,
   `task_result`, and a `task_plan` (synthesis + DAG) event stream.

**Out of scope (still deferred):** the interactive three-way out-of-cwd read prompt on the
surface (the *decision* + WM persistence exist from Phase 2; only the surface UI is
deferred); re-decomposition on failure (ADR OQ2); cost-weighted critical-path ordering
(scheduler v1.5); mid-run resume/persistence of scheduler progress.

## Testing split (non-negotiable for this phase)

Per `[[canonical-project-review-test-prompt]]` and `[[single-model-prompt-engineering]]`:

- **Deterministic contract tests (`@unit`/`@integration`, stubbed verdicts).** Inject
  classification vectors and planner output; assert *routing, oracle decisions, decomposer
  output, and scheduler drainage*. This is where the wiring is pinned.
- **Tolerant real-LLM evals (real Ollama, run-tag withheld until wired).** Assert *shape over
  N runs* — a plan with >1 step, ≥2 independent steps, read-tools engaged — never a single
  verdict. These activate the long-dormant scenarios in
  `tests/features/runtime/task_decomposition.feature`.

## Contract

Interfaces realized (Phase-3 seams, now concrete):

- `Oracle` → a classifier adapter over `pipeline` (action resolve ∧ response one-step).
- `Decomposer` → a branch adapter: fork → planner fan-group → seal `branch.Result`.
- `Executor` → the existing `buildTaskExecutor` executor, unchanged.

Reconcile fold gains one route:

```
turn actionable, non-abstained:
  response executed            → Verify
  response produced (single)   → Reify        (reify one tool call)
  response abstains → produced → Decompose    (reify a plan)   ← NEW
  response dropped             → Redispatch
turn/response ambiguous        → Ask
```

Invariants:

- **Atomic-before-undertake** holds end-to-end: only the scheduler's RUNNABLE path reaches
  the executor, and only after the real oracle calls a node atomic.
- **Plan-only branches**: decomposition runs in a read-restricted branch; no mutation
  occurs during planning (Phase 2 guarantee, now exercised with the real catalog).
- **Best-effort, non-disruptive**: like the rest of `maybeEmitTask`, a decomposition
  failure emits a diagnostic and never disturbs the prompt cycle.
- **Bounded**: recursion by `max_task_depth`; concurrency by the shared slot budget.

## Behavior

### Deterministic contract (stubbed verdicts)

```gherkin
@runtime @task-decomposition @arch:adr-0008 @integration
Scenario: ADR-0008-P4-001 A compound turn folds to Decompose
  Given a turn classified action "query" non-abstained
  And a response classifier that abstains toward "produced"
  When the turn is reconciled
  Then the route is "decompose"

@integration
Scenario: ADR-0008-P4-002 A single produced action stays Reify, not Decompose
  Given a turn classified action "artifact" non-abstained
  And a response classifier that reports a single "produced"
  When the turn is reconciled
  Then the route is "reify"

@integration
Scenario: ADR-0008-P4-003 Genuine ambiguity stays Ask, not Decompose
  Given a turn whose action classifier abstains with a scatter including "none"
  When the turn is reconciled
  Then the route is "ask"

@integration
Scenario: ADR-0008-P4-004 The Decompose route seeds the scheduler and drains the plan
  Given a compound turn routed to "decompose"
  And a stub decomposer expanding the goal into atomic leaves "l1" and "l2"
  And the executor reports every leaf executed
  When the decomposition runs
  Then a task_plan event is emitted with a synthesis
  And "l1" and "l2" each reach status "done"
  And the root goal reaches status "done"

@integration
Scenario: ADR-0008-P4-005 The classifier-backed oracle requires resolve AND one-step
  Given a node whose action classifier resolves to a single type
  And whose response classifier does not call it one-step
  When the oracle evaluates the node
  Then the node is judged not atomic

@integration
Scenario: ADR-0008-P4-006 The decomposer plans inside a read-restricted branch
  Given a decomposer over a stub planner returning two sub-goals
  When a non-atomic goal is decomposed
  Then the result carries two child records and a synthesis
  And no mutating tool is available to the branch

@unit
Scenario: ADR-0008-P4-007 The planner fan-group parses a plan into child records
  Given a planner response listing sub-goals "g1" and "g2" where "g2" depends on "g1"
  When the plan is parsed
  Then it yields child records "g1" and "g2"
  And "g2" depends on "g1"
```

### Tolerant real-LLM eval (activates task_decomposition.feature; assert shape over N)

```gherkin
@functional  # real Ollama; run as an eval, not a gate — assert shape, never one verdict
Scenario: ADR-0008-P4-100 The canonical review prompt decomposes into an investigating plan
  Given the fixture prompt against the running project
  When the turn is classified and routed
  Then in most runs it routes to "decompose"
  And the resulting plan has more than one step
  And at least two steps share no dependency path
  And at least one leaf engages a read tool on the project
```

## Phased sub-build (this phase is large — land in slices)

- **4a — Decompose route.** Add `Decompose` to `reconcile`, the fold rule, and the Ask/
  Decompose discriminator (OQ1). Deterministic tests P4-001..003.
- **4b — planner fan-group.** Add `decompose_plan` to `prompts.toml` + a parser to child
  records. Test P4-007 with canned responses.
- **4c — oracle + decomposer adapters.** Wrap the classifier as `Oracle`; wrap branch +
  planner as `Decomposer`. Tests P4-005, P4-006.
- **4d — pipeline wiring.** Dispatch `Decompose` in `maybeEmitTask` into a `Scheduler` run;
  emit `task_plan`/`task_result` events. Test P4-004.
- **4e — real-LLM eval.** Wire the planner prompt against Ollama; activate P4-100 and the
  dormant `task_decomposition.feature` scenarios as evals; tune the prompt on the fixture.

## Open points to settle before building

1. **Inline vs. async scheduler run.** A full decomposition is many LLM calls — running it
   inline would block the turn for a long time. Lean: **async** — the `Decompose` route
   kicks off a background `Scheduler` run that streams `task_plan`/`task_result` events; the
   turn returns immediately (the plan becomes a first-class, surfaceable entity, per the
   prior doc). Confirm — this shapes the pipeline change most.
2. **Ask vs. Decompose discriminator (OQ1).** Concrete rule: turn actionable + non-abstained
   + response abstains with a `produced`-majority spread → Decompose; a scatter including
   `none`/low confidence → Ask. Confirm the exact spread test using the fixture datapoint.
3. **Root handling.** Does the scheduler drive from the single compound root (it sees
   non-atomic → decompose → children), or does the route decompose once eagerly then
   schedule children? Lean: **scheduler-driven from the root** — uniform, one loop. Confirm.
4. **Oracle call budget.** The oracle classifies each candidate node — many fan-group calls
   over a DAG. Confirm it reuses the one shared slot budget (no separate pool) and the
   existing single model.
5. **Read-restricted branch executor now; surface prompt later.** Build the branch's
   read-restricted executor (read catalog + `WMReadGrants` + confinement) in 4c, but keep
   the interactive three-way approval UI deferred to a surface phase. Confirm.
6. **Planner output schema.** Steps as `{id, goal, deps[], cost?}`, ids `task-<turn>-<n>`,
   plus a plan synthesis/name. Confirm the contract so 4b and 4c agree.
