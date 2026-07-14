# ADR 0010 — Task Assertions, Outcome Grounding, and Plan Continuity in Working Memory

Status: Proposed
Date: 2026-07-13
Deciders: AgentX architecture owners
Depends on: ADR 0008 (recursive task decomposition + DAG scheduler, incl. 2026-07-07
typed-node amendment), ADR 0009 (plan & tool execution visibility)
Resolves: ADR 0008 Open Questions 2 and 6 (amendment); realizes ADR 0009 Phase 9e

## Context

Session `clever-otter` (2026-07-13, ~3 hours, 964 events, 5 plan roots) was reviewed after
the user observed the plan tool degrading over a long working session. The forensic review
found the scheduler and decomposition machinery (ADR 0008) work correctly *within* a single
plan, but nothing survives *across* plans or turns:

- Seven user prompts produced five independent plan roots with no reference between them.
  When `task-841` failed to locate `invoke_planner`, there was no "resume task-841" —
  the user had to retype the failure as a new goal string for `task-1381`.
- `working_memory.json` held ten static bootstrap facts (`cwd`, `os`, `git_status`, …)
  across the entire session; zero facts ever reflected plan state. Every new
  classification/decomposition call was blind to everything an earlier plan had already
  established or attempted.
- A root node's `Status` never reflected its children's failure — it stayed `"decomposed"`
  permanently, even when a child was `failed` or `abstained`.
- The one real remediation attempt (`task-2513-5`, a `write_file` to a wrong-cased path)
  exited non-zero, was marked `failed`, and nothing retried it or surfaced it — the root
  stayed `"decomposed"` as if nothing were wrong.
- The agent twice narrated performing a write in prose with no corresponding `tool_call`
  event — a hallucinated action that nothing in the plan tool cross-checked against what
  actually ran.
- The session's last plan ended `status: "abstained"`, `error: "plan incomplete: ... 1
  abstained ..."`, and the event log simply stopped. No escalation, no recovery path.

None of this is a new failure class for this project. It is the same root cause the
project has already named twice, from different angles:

- ADR 0009 §9b, on its own persistence layer: *"schema is resumability-friendly but
  write-only by design."* Plans are written faithfully; nothing reads them back into a
  later call.
- ADR 0008's Open Questions, left explicitly deferred:
  - **OQ2 (Re-decomposition on failure):** *"When an atomic leaf fails, does its parent
    re-decompose with the failure as context, or does the plan surface the failure?...
    this ADR defers."*
  - **OQ6 (Promotion policy, amendment):** *"What decides a Task's `Results()` gets
    written to WM with a TTL... Deferred; needs product judgment this amendment doesn't
    presume."*

Two more pieces of existing code independently anticipate this design without completing
it:

- `task.Done`'s own doc comment already promises what the code does not do:
  *"executed and the effect verified; only now may success be reported"*
  (`internal/prompting/task/task.go:53`). `Scheduler.execute`
  (`internal/runtime/scheduler/scheduler.go:255-264`) maps `executor.Executed` straight to
  `task.Done` — no verification happens. The comment describes this ADR; the code doesn't
  do it yet.
- `task.Status.Failed` is documented as covering two different things at once:
  *"execution errored **or** the effect could not be verified"* (task.go:56). Those are
  not the same claim — one is evidence of failure, the other is absence of evidence — and
  the project has already drawn exactly this distinction once before, for a different pair
  of causes: `Denied` was split out from `Failed` specifically so *"a plan's terminal
  report can tell 'someone said no' apart from 'this is broken'"* (RCA: session
  `nimble-pebble-2`, TOOL-7). This ADR applies the same reasoning to the `Failed`/evidence
  conflation the comment already confesses to.

## Decision

### 1. Every node declares a falsifiable assertion, not just a goal

`taskPayload` and `stepPayload` (`internal/prompting/planner/planner.go`) and `task.Record`
(`internal/prompting/task/task.go`) gain a required field:

```go
// task.Record
Assertion string `json:"assertion"` // a checkable claim this node's result must satisfy
```

A **Task**'s assertion is checked against its own tool result once it executes (e.g.
"README.md is present in the repo root"). A **Step**'s assertion is checked against the
synthesis of its children once they resolve (e.g. "the source tree's existing feature
coverage has been identified") — this is what finally gives a Step's `Deliverable` field
(already present, currently only planner guidance text — planner.go:112) a use: the
deliverable states what the assertion is checked against.

`Explanation` (Task) and `Deliverable` (Step) are **not replaced**. Assertion sits
alongside them: explanation is "why," the goal is "what," assertion is "how we'll know it
worked." Collapsing them would lose the pre-hoc justification the pre-execution widget
(ADR 0009 §9c) already shows the user before a node runs.

**Enforcement reuses the existing degrade-on-violation mechanism**, not new machinery. The
typed-node amendment already established the pattern for "the planner must declare
something at generation time, and a violation gets one retry then degrades"
(`kind` in `planner.Parse`; `SimilarGoals`'s echo check in
`internal/runtime/decompose/decompose.go`'s `attemptPlan`). A missing or non-falsifiable
assertion is one more checked violation in that same loop: named in the retry prompt
(`decompose.go:80`, `"Your previous attempt was invalid: %s. Fix this and reply again."`),
demoted to a single execute attempt on a second failure — never a third attempt, never a
silent pass-through with no assertion at all.

### 2. Outcome grounding is a mechanical precondition gating an inference judgment

A node's assertion is evaluated against its result in exactly one of three outcomes:

- **satisfied** — positive evidence confirms the assertion.
- **refuted** — positive evidence confirms the assertion is false. A clean negative result
  (`grep` ran correctly and found nothing) is real evidence, not an absence of it.
- **abstained** — the evidence is insufficient to conclude either way (the tool errored,
  timed out, hit a permission wall, or returned something ambiguous).

**The precondition is mechanical and non-negotiable: only a check that itself executed
cleanly may produce `satisfied` or `refuted`.** A broken or errored check is always
`abstained`, regardless of what its raw output looks like — the judgment step never gets
the chance to promote a broken check into a confident negative. This is what stops the
exact failure the session log showed: the agent must not hallucinate that a negative or
inconclusive result licenses a false claim about system state.

Judgment itself may require inference (most assertions do — "confirm X is present" is
mechanical, "confirm the README explains installation clearly" is not), so this is a real
LLM cost, not free. A phased mechanical fast-path is scoped in §6/Phased Build Plan step 4
to bound that cost: an assertion with an eligible `MechanicalCheck` (§6) skips the judge
entirely, since the execution *is* the check.

**When inference is required, it is not one call — it is a fan-out vote, reusing
`internal/llm/fanout` rather than inventing new voting infrastructure.** A single judge
call self-reporting "satisfied / refuted / abstained" has the same failure mode as the
rejected self-declared-mechanical boolean in §6: an LLM's single-shot self-assessed
uncertainty is exactly the thing that isn't trustworthy on its own. Instead, each
invocation answers a **binary** verdict only (`satisfied` | `refuted`) with a short
rationale, and `abstained` is never self-reported — it *emerges* from the ensemble:

- `fanout.Contract{RequireFields: []string{"verdict", "rationale"}, MaxWords: 40}` keeps
  each invocation's prompt and output small and structured — a tight, narrowly-scoped call
  (assertion + tool result only, per this project's context-curation discipline), not a
  free-form judgment. A malformed response is quarantined out of the vote, not counted.
- `fanout.NewMajorityVote(WithQuorum(3), WithAbstainBelow(2.0/3.0))` — the same aggregator
  shape already tuned for the action classifier (3-of-4 → 2-of-3 supermajority,
  `abstain_below`, per ADR 0008's context). A 2-of-3 agreement yields `satisfied`/`refuted`;
  a split vote, or too many quarantined/malformed responses, yields
  `Decision{Abstained: true}` structurally — not because any single call said "I don't
  know," but because the ensemble couldn't agree. This is a strictly harder-to-game
  implementation of "insufficient evidence" than trusting one call's self-assessment, and
  it mirrors §2's own mechanical precondition one layer up: just as a broken check can
  never be promoted to a confident negative, a **non-quorum vote can never be promoted to a
  confident verdict**.

This does raise the cost of the inference path from one call to up to three, run within
the same shared slot budget ADR 0008 already established (fan-out width clamped to slot
count, no independent pool) — making §6's mechanical fast-path a harder requirement to
land before this ships broadly, not a nice-to-have. See Consequences.

**This redefines what `Done`/`Failed`/`Abstained` mean for a Task node.** Today
`Scheduler.execute` derives status purely from `executor.Outcome.Status`
(`executor.Executed → task.Done`, `Denied/NeedsApproval → task.Denied`,
default → `task.Failed`). This ADR inserts the assertion judgment between "executed" and
the terminal status: `executor.Executed` no longer maps straight to `task.Done` — it maps
to *evaluate the assertion*, whose outcome then supplies the terminal status
(`satisfied → Done`, `refuted → Failed`, `abstained → Abstained`). `Denied`/`NeedsApproval`
are unaffected — a policy decision is orthogonal to evidence and stays exactly as it is
today (TOOL-7's distinction holds).

`Abstained` already exists as a task status, but today it is purely a *classification*-time
concept — *"classification was not confident enough (scattered vote)"* (task.go:46-47).
Reusing it here for a *post-execution* "insufficient evidence" outcome is deliberate, not
overloading: both are the same epistemic shape (not enough confidence to commit either
way), just at different pipeline stages. `PlanTreeNode.Status`
(`internal/session/plans.go`) already carries `abstained` in its (separate, smaller) enum
for exactly this reason — this ADR is the first thing to actually route into it on
purpose.

`PlanTreeNode` gains the assertion and the judge's grounds:

```go
// PlanTreeNode
Assertion string `json:"assertion,omitempty"`
Rationale string `json:"rationale,omitempty"` // judge's evidence-grounded explanation
```

A root or Step node's `Status` **rolls up** from its children instead of freezing at
`"decomposed"`: any child `failed` → parent `failed` (once its own assertion is evaluated
against the partial synthesis); any child `abstained` and none `failed` → parent
`abstained`; all children `done` → parent's own assertion is evaluated against the
synthesis, per above.

### 3. Only a refutation licenses rework — resolving OQ2

This is the guardrail the user raised directly, made structural rather than advisory:

- **`refuted`** → the reconsider/iterate loop may run: the node's assertion, its command,
  its result, and the judge's rationale are fed back to the planner to revise the
  *approach*. This is the "OQ2: does the parent re-decompose with the failure as context"
  question — answered yes, but gated strictly on `refuted`, never on `abstained`.
- **`abstained`** → may only trigger a retry of the *same assertion's evidence-gathering*
  (same objective, a different check method) — never a plan rewrite, never a claim about
  system state. Bounded retry (budget TBD, Open Questions below); on exhaustion, the node
  becomes a terminal, user-visible escalation rather than the session simply stopping the
  way `clever-otter`'s did. No new terminal state is needed — `abstained` already is one;
  what was missing was anything downstream of it.

### 4. Plan continuity becomes a curated Working Memory entry — resolving OQ6, realizing 9e

`session.Fact` (`internal/session/workmemory.go`) already carries a structured sidecar
pattern for non-plain facts — `Source *ToolSource`, `Live`, `PinnedAt` for pins. This ADR
adds a parallel sidecar rather than restructuring `Fact` or promoting `WorkingMemory` to a
nested map, since the existing `[]Fact{Key, Value string}` shape stays the thing every
context-assembly call site already knows how to read:

```go
type PlanRef struct {
    RootID    string `json:"root_id"`
    Status    string `json:"status"`     // rolled-up status per §2
    UpdatedAt int64  `json:"updated_at"`
}

// Fact
Plan *PlanRef `json:"plan,omitempty"`
```

`Fact.Value` becomes a **curated, bounded text summary** of the plan — e.g. `"Plan
'reading-progress-tracker': 3/5 done, refuted on step 4 (wrong path
/home/mpeters/projects vs. actual cwd), retrying"` — regenerated on every rollup change,
under `Key: "plan:<name>"`. This is the tier-2 fix: the classifier and the next
decomposition call read this like any other fact, so "proceed" after a plan resolves is
groundable against real state instead of falling to a content-free `respond_directly`
classification, and a plan that spans a multi-hour gap (`clever-otter`'s 15:06→17:42 gap)
resumes from what's actually recorded rather than starting blind. `Fact.Plan.RootID` is
the pointer into the full `plans/<rootID>.json` tree for anything that needs to reconcile
structurally rather than read the prose summary — the plan tree itself is not inlined into
WM, closing the unbounded-growth problem (`task-2513.json` hit 59KB from an inlined 41.6KB
raw file dump in one node's `result_text`; WM must never repeat that mistake).

**Promotion policy (closes OQ6):** a plan's WM entry is written/updated only on `satisfied`
or `refuted` node outcomes — both are evidenced conclusions worth remembering. `abstained`
outcomes update the plan's `Status` (so continuity still reflects "this is stuck") but
never promote a claim as fact.

### 5. Plan-scoped facts expire — building the amendment's deferred `ExpiresAt`

The 2026-07-07 typed-node amendment proposed `Fact.ExpiresAt *time.Time` as a "small,
additive" WM TTL primitive but left it unimplemented and unused. This ADR is its first
consumer: a plan's `Fact` (`Key: "plan:<name>"`) carries `ExpiresAt` set at plan-close
time (a short grace window after the root reaches a terminal rolled-up status), so a
closed plan's WM footprint self-removes instead of accumulating indefinitely across a long
session — directly answering the user's "removed when no longer needed."

```go
// Fact — implements the amendment's deferred field
ExpiresAt *time.Time `json:"expires_at,omitempty"` // nil = no expiry
```

`WorkingMemory.Enabled()` (workmemory.go:142) filters expired facts at read time; nothing
else needs to change to make expiry effective.

### 6. A mechanical check vocabulary — first cut, not a closed question

The fast-path in §2/Phase 4 needs a way to decide, per node, whether the judge call can be
skipped. Two shapes were considered:

- **Always infer**, with a tight judge prompt (intent + tool call/result + assertion →
  up/down). Safe, but pays an LLM call on every node — it doesn't address the cost §2/
  Consequences flags as needing an offset.
- **The planner self-declares** a node "mechanical" or "inferred" as a trusted boolean.
  Rejected: a false "mechanical" means *nothing checks it* — silent, and strictly worse
  than always inferring, since it looks grounded without being grounded.

The way out is not a more-accurate classifier; it's removing the boolean's ability to be
*trusted* at all. The planner doesn't get to assert "this is mechanical" — it has to
produce an actually-executable predicate, and the system runs it or doesn't, deterministically:

```go
// MechanicalCheck is an optional, planner-emitted predicate over a Task's raw
// executor.Outcome. Presence is a claim, not a trust grant — the fast path only fires
// if Kind is recognized and every field it references exists on the tool's Outcome
// shape. Anything that fails to parse or doesn't apply falls through to AssertionJudge
// automatically; a bad or unparseable Check costs one extra inference call, never a
// silently ungrounded pass.
type MechanicalCheck struct {
    Kind string `json:"kind"` // "exit_status" | "stderr_class" | "line_count" | "pattern_match"

    WantExit *int `json:"want_exit,omitempty"` // exit_status

    // stderr_class — a closed vocabulary for *why* a nonzero exit happened, so a real
    // negative (not_found) is distinguishable from an inconclusive one (permission_denied,
    // timeout, other) without inference. This is what makes §2's mechanical precondition
    // ("only a check that executed cleanly may refute") actually implementable for tools
    // whose informative signal *is* a nonzero exit — read_file (cat) and list_dir (ls)
    // both fail with "No such file or directory" on a legitimate absence, which is real
    // evidence, not an error to abstain on.
    WantClass string `json:"want_class,omitempty"` // "not_found" | "permission_denied" | "other"

    // line_count / pattern_match — evaluated against stdout or stderr text.
    Stream       string `json:"stream,omitempty"` // "stdout" (default) | "stderr"
    Pattern      string `json:"pattern,omitempty"` // RE2 syntax (Go-safe, no backtracking)
    MinMatches   *int   `json:"min_matches,omitempty"`
    MaxMatches   *int   `json:"max_matches,omitempty"`
    ExactMatches *int   `json:"exact_matches,omitempty"`
}
```

`task.Record`/`taskPayload` gain `Check *MechanicalCheck json:"check,omitempty"` alongside
`Assertion` — optional; its absence just means "always infer for this node," the safe
default.

**Grounded against the actual catalog** (`internal/tools/descriptors.go`), not proposed in
the abstract:

- `exit_status` is universal — every tool's `executor.Outcome` carries one.
- `find_path` (`find {root} -name {name}`) returns exit 0 with **empty stdout** on zero
  matches — GNU `find` does not error on "nothing found." A presence/absence assertion
  against it is a `line_count` check (`max_matches: 0` for absence), **not**
  `stderr_class` — the tool's own success/failure semantics don't line up with the
  assertion's semantics, and a check that assumed otherwise would silently misfire. This
  is exactly the kind of edge the vocabulary has to get right per-tool, not just per-kind.
- `read_file` (`cat`) and `list_dir` (`ls -la`) do error on a genuine absence
  (`No such file or directory`), so `stderr_class: not_found` applies to them.
- `pattern_match` against file *content* only works today by pairing it with `read_file`
  (run `read_file`, apply the pattern to its stdout) — it cannot search *across* files,
  because **the current catalog has no content-search tool** (`find_path` matches
  filenames only). A `grep`-shaped descriptor is a prerequisite for that class of
  assertion to be mechanically checkable at all, not a vocabulary gap — this ADR doesn't
  scope adding one, but Phase 4 should note it as a real, named limitation rather than
  something the vocabulary can paper over.

**The finite-vocabulary-vs-long-tail tension is real and unresolved here — this is a first
cut, not a closed design.** A handful of kinds cover the catalog's read-only tools
reasonably; they will not cover everything a planner reasonably wants to assert, and the
temptation will be to keep adding kinds indefinitely. The reason this is safe to leave
open rather than solved up front: **every kind the vocabulary doesn't yet support is
already handled correctly — by falling through to inference.** The vocabulary's
completeness is a cost lever, not a correctness requirement; it can ship with four kinds
and grow strictly additively, one at a time, each new kind only ever making *more* nodes
eligible for the cheap path, never changing what a missing/unrecognized kind does (falls
through, same as today). What's still genuinely open: who decides when a new `Kind` earns
its way into the vocabulary, whether eligibility should eventually be declared per-tool on
`Descriptor` itself (so `find_path` could assert "I don't support `stderr_class`" instead
of relying on every check-author to know that), and how a new kind's execution logic gets
tested before it's trusted to run unattended. None of that is resolved by this ADR.

### Architecture — insertion points

- `internal/prompting/planner/planner.go` — `taskPayload`/`stepPayload` gain `Assertion`;
  `DefaultPromptTemplate`/`DefaultUserTemplate` teach it with a worked example, same
  pattern as the Kind amendment's Step-vs-Task worked example.
- `internal/runtime/decompose/decompose.go` — `attemptPlan` gains an assertion-presence/
  falsifiability check alongside `SimilarGoals`, feeding the existing one-retry-then-degrade
  loop.
- `internal/runtime/scheduler/scheduler.go` — `Scheduler.execute` (currently
  executor-status → task-status, direct) gains the judgment step between "executed" and
  "terminal status": first an eligible `MechanicalCheck` (§6) if present, else the
  fan-out judge (§2). A new collaborator interface, `AssertionJudge`, sits next to the
  existing `Oracle`/`Decomposer`/`Executor` seams the Kind amendment retired the first of:

  ```go
  type AssertionJudge interface {
      Judge(ctx context.Context, assertion string, result executor.Outcome) (
          outcome task.Status, rationale string, err error)
  }
  ```

  Its implementation wraps `fanout.Pool.Fold` with `fanout.NewMajorityVote` (§2) — no new
  voting primitive, only a new `fanout.Invoker`/prompt for this call shape.

- `internal/session/plans.go` — `PlanTreeNode` gains `Assertion`, `Rationale`; root/Step
  rollup logic per §2.
- `internal/session/workmemory.go` — `Fact` gains `Plan *PlanRef` and `ExpiresAt`;
  `WorkingMemory.Enabled()` filters expired facts.
- `internal/prompting/task/task.go` — `Record` gains `Assertion`; `Status.Failed`'s doc
  comment is corrected to drop the "or the effect could not be verified" clause (that's
  `Abstained`'s job now), matching the precedent already set for `Denied`.
- ADR 0009's status-cue set (`⊘` currently covers both `blocked` and `abstained` — ADR
  0009 line 34-35) needs to split: `abstained`-with-retry-budget-remaining is distinct
  from `abstained`-exhausted-and-escalated, and neither is the same as a node genuinely
  `BLOCKED` on an unfinished dependency. Left to the widget implementation phase below
  rather than specified glyph-by-glyph here.

## Consequences

Positive:

- Closes ADR 0008 OQ2 and OQ6 with a mechanism, not just a policy statement — and both
  answers fall out of the *same* tri-state judgment rather than needing separate designs.
- Fulfills `task.Done`'s own doc comment ("effect verified") for the first time.
- Extends, rather than replaces, four patterns already trusted in this codebase: the
  Kind amendment's declare-at-generation-time + degrade-on-violation loop; the
  Denied/Failed evidence-vs-decision split (TOOL-7); the Fact sidecar pattern already
  used for pins; and `internal/llm/fanout`'s quorum/abstain-threshold voting, already
  tuned and load-bearing for the action classifier — the judge is a new *caller* of it,
  not new voting machinery.
- Makes plan intent legible to the user in plan-objective terms ("what must be true"), not
  system-call terms — the assertion text is a strictly more informative pre/post-execution
  label than the raw command already shown in ADR 0009's widget.
- Bounded WM growth: a plan's context-surface footprint is a regenerated summary string
  plus a pointer, never the raw tree — cannot repeat `task-2513.json`'s 59KB inlining.

Trade-offs:

- **Reintroduces per-node LLM calls the Kind amendment specifically removed — and as a
  3-call vote, not a single call, it is a larger reintroduction than a first read
  suggests** (the amendment retired the atomicity Oracle for a real latency win — "roughly
  half of `mellow-meadow`'s per-node ~10s was oracle voting"). This is not the same
  call — it buys grounded correctness on a judgment that a single self-reported answer
  can't be trusted for, not a guess at atomicity — but the cost is real, shares the same
  slot budget as everything else (no independent pool), and **must** be offset by the
  mechanical fast-path (§6/Phased step 4) landing before this ships broadly, not as an
  optional follow-on.
- `task.Status` (8 values, task.go) and `PlanTreeNode.Status` (6 values, plans.go) are
  already two separate, not-quite-aligned enums; this ADR adds meaning to `abstained` in
  both without unifying them. Reconciling the two enums is explicitly out of scope here
  (Open Questions).
- The retry budget for `abstained` re-checks needs a real number, not "bounded" — picking
  it wrong repeats either `clever-otter`'s silent stall (too low/no escalation) or a new
  spiral (too high, no escalation ceiling).

## Phased Build Plan

1. **Assertion field + generation-time enforcement.** `Record`/`taskPayload`/
   `stepPayload` gain `Assertion`; `planner.Parse` and `decompose.go`'s `attemptPlan` treat
   a missing/non-falsifiable assertion as a violation under the existing retry-then-degrade
   loop. No judgment yet — this phase only makes assertions mandatory and visible.
2. **Outcome grounding.** `AssertionJudge` collaborator, backed by a `fanout.Pool` +
   `MajorityVote` (§2), wired into `Scheduler.execute`; `task.Status`/`PlanTreeNode.Status`
   semantics updated per §2. Two behavior docs are the highest-value contracts to pin here,
   mirroring how ADR 0008 called out its own state machine as the priority: the
   mechanical-precondition rule (a broken check must never produce `refuted`) and the
   vote's non-quorum-never-promotes-to-a-verdict rule (the same guarantee, one layer up).
3. **Rollup + reconsider/abstain guardrail.** Root/Step status rollup; the `refuted`→
   reconsider vs. `abstained`→recheck-only branch (§3); bounded retry + escalation on
   `abstained` exhaustion.
4. **Mechanical fast-path.** Skip the judge call when an assertion is checkable directly
   from the executing tool's own exit/output shape — the cost mitigation the trade-offs
   section requires before this ships broadly, not an optional nice-to-have.
5. **WM plan continuity.** `Fact.Plan`/`PlanRef`, curated summary regeneration on rollup
   change, `ExpiresAt` implemented and enforced in `WorkingMemory.Enabled()`. Realizes ADR
   0009 Phase 9e.
6. **Widget + status-cue split.** ADR 0009 §9c's glyph set gains distinct cues for
   blocked / abstained-retrying / abstained-escalated, and renders the assertion text
   (not just the goal) as the node's user-facing objective line.

Every phase's touched functions need a GIVEN/WHEN/THEN behavior doc before implementation,
per repo invariant — phase 2's judgment rule and phase 3's guardrail are the two contracts
most worth writing first, since they are exactly the two places a bug reintroduces the
`clever-otter` failure mode.

## Open Questions

1. **Retry budget for `abstained` re-checks.** A specific number (and whether it's fixed or
   scales with plan depth/cost) is needed before phase 3 ships; picking it is product
   judgment, not architecture.
2. **`task.Status` / `PlanTreeNode.Status` reconciliation.** Two separate enums for
   overlapping concepts predates this ADR; whether to unify them is a larger refactor this
   ADR deliberately does not scope.
3. **Escalation surface.** When `abstained` exhausts its retry budget, does it surface as a
   new `Ask`-route question (ADR 0008's existing escalation path) or a distinct
   plan-level UI affordance? Leaning `Ask` — reuses an existing, understood mechanism —
   but not decided here.
4. **Mechanical-fast-path classification.** First cut in §6: the planner emits an optional
   `MechanicalCheck`, trusted only if it parses and applies — never a trusted label.
   Genuinely still open within that: vocabulary growth/governance (who adds a new `Kind`,
   whether eligibility should eventually be declared per-tool on `Descriptor`), and how a
   new kind's execution logic gets validated before it runs unattended.
5. **Vote quorum/threshold tuning.** §2 starts from the action classifier's existing
   2-of-3-of-3 shape (`WithQuorum(3)`, `AbstainBelow(2.0/3.0)`) as the obvious reuse, but
   whether assertion judgment should share that exact tuning or needs its own — the two
   calls are answering different kinds of questions (turn classification vs. evidence
   grounding) — is an empirical/product question, not decided here.
