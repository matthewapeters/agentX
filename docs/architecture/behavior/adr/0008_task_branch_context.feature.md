# Feature: ADR 0008 Phase 2 — Task Branch Context

Status: **Implemented** (2026-07-06). Branch is an **investigating branch** (Model I): it
may run read-only tools via a read-restricted catalog, and out-of-cwd reads are gated by an
infra approval with a session-scoped grant. Code: `internal/runtime/branch/branch.go`,
`internal/session/readgrant.go`, `internal/executor` (`ReadGrants`/`WithReadGrants`).
Runnable contract: `tests/features/runtime/task_branch.feature` (@unit, UC-RTBRANCH-001…009).
Open points 2–5 were resolved with the accepted leans. **Integration seam not yet wired**
(lands with the Phase-4 decomposer): the orchestrator building a branch's read-restricted
executor, and the interactive three-way (No / Once / Session) approval prompt on the surface
— the decision + WM persistence (`session.ApplyGrant`) is in place and unit-tested; only the
surface prompt is deferred.

Realizes **Phase 2** of `docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md`:
*"A forked sub-session (own event log + working memory) that runs a node and returns only
a synthesis + child records. Enforce plan-only (no side effects)."*

Like Phase 1, this phase is **LLM-free**: it delivers the *container* and its *boundary*,
not the decomposition algorithm. The decomposer that runs *inside* a branch (an LLM
fan-group emitting child records) is Phase 4. Phase 2 proves that a branch is isolated,
plan-only, and returns nothing to the parent except an explicit result.

Schema / source links:

- `internal/session/session.go`, `recorder.go` (append-only event log, `Store`/`Recorder`)
- `internal/session/workmemory.go` (`WorkingMemory`, `Fact`, `Enabled()`, `BootstrapFacts`)
- `internal/prompting/task/graph.go` (Phase 1 `Graph` — the merge target)
- `internal/state` (`Event`, bus/`Subscription` — the parent conversation stream)

## Scope

**In scope (Phase 2 — the branch container and its boundary):**

- **Fork.** A branch is created from a parent, inheriting a *read-only snapshot* of the
  parent's **enabled** working-memory facts (so a decomposer can see `cwd`, `project`,
  `repo_root`), plus a parent link and a depth counter.
- **Isolation.** A branch has its own event sink; branch events never reach the parent's
  conversation bus (this is the ADR's "context compression"). Branch-local working-memory
  changes never propagate to the parent.
- **Plan-only (investigating branch — Model I, chosen).** A branch may run **read-only**
  tools to plan from evidence rather than assumption, but cannot cause a mutating side
  effect. This is enforced by a **read-restricted catalog**: the branch's executor is
  offered only descriptors with `Risk == read` (`internal/tools`); `write`/`network`
  descriptors are simply absent, so a mutation cannot be issued — the boundary is the
  *catalog*, not a call-time denial. Read tools run under the same working-directory
  confinement as the parent.
- **Result + merge.** A branch's only output is a `Result = { records, synthesis }`.
  Merging a Result into the parent adds its records to the parent task `Graph` (Phase 1)
  and surfaces the synthesis. Nothing else — no branch fact, no branch event — crosses back.
- **Bounded depth.** A branch forked from a branch increments depth; forking at
  `max_task_depth` is refused (the node fails to `Ask` per the ADR, not silently).
- **Disposability.** A branch discarded without sealing leaves the parent byte-for-byte
  unchanged — no partial-state leak.

**Out of scope (later phases):** the decomposition algorithm and any LLM call (Phase 4);
acting on the merged DAG (`Ready` → executor, Phase 3); per-plan on-disk logs / a plan tab
(surface phase). Phase 2 proves the *container* is watertight so the decomposer can be
dropped into it safely.

## Contract

Working API sketch (behavior-level, not frozen):

- `Fork(parent) (Branch, error)` — snapshots `parent.WorkingMemory.Enabled()`, sets
  `depth = parent.depth + 1`, allocates an isolated event sink. Refuses at `max_task_depth`.
- `Branch.Facts() []Fact` — the inherited read-only snapshot. There is **no** setter that
  writes through to the parent.
- `Branch.Emit(event)` — records to the branch's own sink only.
- `Branch.Executor()` — the real executor wired to a **read-restricted catalog** (only
  `Risk == read` descriptors), under the parent's working-directory confinement. This is
  how a branch investigates without being able to mutate.
- **Out-of-cwd read grant (general executor infra, consumed here).** Confinement consults a
  `ReadGrants` set before prompting: a path inside the cwd *or* inside a granted path runs
  without a prompt. On a miss the `Approver` returns a **scope** — `Deny` / `Once` /
  `Session` (not a bare bool). `Session` persists the path to working memory as a
  first-class, user-revocable `read_path:<abs>` fact and adds it to the live grant set;
  `Once` allows only the current call; `Deny` refuses. This is an infra approval, never a
  conversation turn. Helpers: `session.GrantReadPath`, `session.PermittedReadPaths`,
  `session.ReadAllowed(root, granted, path)`; executor `WithReadGrants(ReadGrants)`.
- `Branch.Add(rec) / Branch.Plan() *Graph` — the branch accumulates child records in its
  own plan `Graph`; integrity is enforced exactly as in Phase 1.
- `Branch.Seal() Result` — returns `{ Records []task.Record, Synthesis string }`.
- `MergeBranch(parent, Result) error` — `Graph.Add`s each record into the parent DAG and
  emits the synthesis as a durable parent event; idempotent-safe on integrity errors.

Invariants:

- **Read-only inheritance.** The branch sees enabled parent facts and never the disabled
  ones; it cannot mutate parent WM.
- **One-way result.** The parent changes *only* through `MergeBranch`, and *only* by the
  records and synthesis in the Result. The branch's raw investigation (tool calls +
  results) stays in the branch log and is compressed to the synthesis on return.
- **No mutating side effects.** The branch catalog is restricted to `Risk == read`;
  `write`/`network` descriptors are not offered, so a mutation cannot occur. Read tools run
  under the same working-directory confinement as the parent executor.
- **Provenance.** Every record a branch emits carries its origin (parent id, branch id,
  depth), so a plan is answerable from the log — consistent with `task.Provenance`.

## Behavior

```gherkin
@adr0008 @phase2 @branch @positive
Scenario: ADR-0008-P2-001 A branch inherits a read-only snapshot of enabled parent WM
  Given a parent working memory with enabled facts "cwd" and "project"
  And a disabled parent fact "secret"
  When a branch is forked from the parent
  Then the branch sees facts "cwd" and "project"
  And the branch does not see "secret"

@adr0008 @phase2 @branch @isolation
Scenario: ADR-0008-P2-002 Branch working-memory changes do not reach the parent
  Given a branch forked from a parent
  When the branch records a local fact "language" = "go"
  Then the parent working memory has no "language" fact

@adr0008 @phase2 @branch @isolation
Scenario: ADR-0008-P2-003 Branch events are isolated from the parent conversation
  Given a parent conversation bus and a branch forked from the parent
  When the branch emits a planning event
  Then the parent conversation receives no branch event
  And the branch's own log contains the planning event

@adr0008 @phase2 @branch @plan-only
Scenario: ADR-0008-P2-004 A branch cannot issue a mutating tool call
  Given a branch forked from a parent
  When the branch requests a "write"-risk tool
  Then the tool is not in the branch catalog
  And no file is written and no command is run on the branch's behalf

@adr0008 @phase2 @branch @plan-only @positive
Scenario: ADR-0008-P2-004b A branch may run a read-only tool to investigate
  Given a branch forked from a parent
  When the branch runs a "read"-risk tool within the working directory
  Then the tool runs and returns a result
  And the result stays in the branch log, not the parent conversation

@adr0008 @phase2 @branch @plan-only @confinement
Scenario: ADR-0008-P2-004c An out-of-cwd read is permitted only with prior approval
  Given a branch forked from a parent with no granted read paths
  When the branch runs a "read"-risk tool outside the working directory
  Then the read does not run
  And the call is surfaced for approval with reason "outside working directory"

@adr0008 @phase2 @branch @grant @positive
Scenario: ADR-0008-P2-004d A session-scoped grant is remembered in working memory
  Given a branch whose out-of-cwd read to "/data/logs" is approved for the session
  Then working memory records a permitted read path "/data/logs"
  When a later read under "/data/logs" is attempted
  Then the read runs without another approval prompt

@adr0008 @phase2 @branch @grant @positive
Scenario: ADR-0008-P2-004e A one-time grant is not remembered
  Given a branch whose out-of-cwd read to "/data/logs" is approved once
  Then working memory records no permitted read path
  When a later read under "/data/logs" is attempted
  Then the call is surfaced for approval again

@adr0008 @phase2 @branch @result
Scenario: ADR-0008-P2-005 A branch returns only child records and a synthesis
  Given a branch that has added child records "s1" and "s2"
  And the branch has set a synthesis "two independent investigation steps"
  When the branch is sealed
  Then the result records are exactly "s1, s2"
  And the result synthesis is "two independent investigation steps"

@adr0008 @phase2 @branch @merge
Scenario: ADR-0008-P2-006 Merging a branch result adds its records to the parent DAG
  Given a parent task DAG containing node "root"
  And a branch result with records "s1" and "s2" each depending on "root"
  When the result is merged into the parent
  Then the parent DAG has nodes "root", "s1", "s2"
  And the parent edge set includes "root->s1" and "root->s2"
  And the parent synthesis event records "two independent investigation steps"

@adr0008 @phase2 @branch @disposable
Scenario: ADR-0008-P2-007 A discarded branch leaves the parent unchanged
  Given a parent with a task DAG of 1 node and a working memory of 2 facts
  And a branch that has added records and local facts
  When the branch is discarded without sealing
  Then the parent task DAG still has 1 node
  And the parent working memory still has 2 facts

@adr0008 @phase2 @branch @depth @negative
Scenario: ADR-0008-P2-008 Branch depth increments and is bounded
  Given a branch at depth one below max_task_depth
  When it forks a child branch
  Then the child branch depth equals max_task_depth
  When the child branch attempts to fork again
  Then the fork is refused with a max-depth error

@adr0008 @phase2 @branch @provenance
Scenario: ADR-0008-P2-009 A branch and its records carry parent provenance
  Given a parent session "p1"
  When a branch is forked from the parent
  Then the branch provenance records parent "p1"
  And a child record added in the branch carries the branch's provenance
```

## Open points to settle before building

1. **RESOLVED — Model I (investigating branch).** The branch may run read-only tools to
   plan from evidence, preventing the assumptions/hallucinations a blind planner would make.
   The risk this raised — "plan-only becomes a policy check, not an absence of capability" —
   is largely retired by the tools registry already tiering descriptors by `Risk ∈ {read,
   write, network}`. The branch is offered a **read-restricted catalog** (`Risk == read`
   only), so mutating tools are *absent*, not merely denied: the boundary stays structural,
   reusing existing risk tiers and working-directory confinement — no new tool metadata and
   no new policy code. Consequences folded into the scope/contract above and scenarios
   P2-004 / 004b / 004c. Cost (an investigating branch spends tokens + slots on real tool
   calls) is contained by the branch's own context compression — the reads bloat the branch,
   not the parent — and bounded by the shared slot budget (Phase 3). The prior doc's
   re-decomposition / dynamic-step mechanism remains available but is no longer *required*
   for the branch to see the project.

2. **In-memory branch vs. on-disk sub-session.** Lean: **in-memory** branch (ephemeral
   event sink + WM snapshot); persist only the merged Result into the parent session.
   Defer a real on-disk branch dir / per-plan replayable log to the surface phase, when a
   plan tab needs it. Keeps Phase 2 light and matches "returns only the plan."

3. **Where `Branch` lives.** It straddles `task` (plan/`Graph`, records) and `session`/
   `state` (WM snapshot, event sink). Lean: a small `internal/runtime/branch` (or
   `internal/session`) fork helper that composes a `task.Graph` for the plan, keeping the
   `task` package I/O-free as in Phase 1. Confirm the package boundary before coding.

4. **Synthesis representation on merge.** Plain string, a WM fact, or a dedicated event
   content type (like `ContentTaskDiagnostic`)? Lean: a dedicated `task_plan`/synthesis
   **event** so the surface can render it and replay reconstructs it. Defer the exact
   `ContentType` name.

5. **`max_task_depth` source.** From `agentx.toml` (prior doc default 10). Confirm the key
   name and whether it lives under a `[classification]`/`[tasks]` block; wire the default so
   an unset config still bounds recursion.
