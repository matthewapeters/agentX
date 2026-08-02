# ADR 0014: Tidal — Hypothesis-Grounded Consolidation over Wavefront Dispatch

Status: Proposed
Date: 2026-08-02
Deciders: AgentX architecture owners
Depends on: ADR 0008 (`task.Graph`/`Kind`/`Deps` substrate — Tidal extends `Kind`
and adds a sibling edge type, reusing the graph rather than building a new
structure), ADR 0009 (execution visibility — plan-widget rendering precedent),
ADR 0010 (tri-state assertion judgment — `satisfied`/`refuted`/`abstained` — and
its mechanical-precondition discipline; **Status: Proposed, unimplemented as of
this ADR** — Tidal extends its vocabulary from per-node to root-level rather than
inventing new terminology), ADR 0012 (wavefront's continuous dispatch,
graph-as-blackboard, `ErrNoProgress` — Tidal's Tier 1 *is* wavefront, reused
unchanged, not reimplemented), ADR 0013 (`ConversationCore` — the sub-session
substrate Tidal's Tier 2 attaches to via `hooks.SyncHook`, without modifying it)

Scope: names and designs the hybrid problem-solving strategy — combining
wavefront's grounded continuous dispatch, abductive hypothesis-tracking with
likelihood, and a non-pruning consolidation cycle inspired by but structurally
distinct from Tree-of-Thought — plus the context schema it operates over and how
its periodic phase is realized against `ConversationCore` with zero changes to
that loop. Does **not** scope: the `complex_task`/planning-tool's full native-tool
registration or its relationship to `plan_task` (ADR 0013 Open Question 4,
still open), or the sub-session `ContextStore`'s exact seeding mechanism (ADR
0013 Open Question 2, still open) — this design assumes both exist but doesn't
resolve either here.

## Context

Wavefront (ADR 0012) closes a specific failure mode: a node committing to a
guessed argument before the evidence to ground it exists. It does not close a
different, later-discovered failure mode: a classify call can only propose
`Need`s for directions that already occur to it as relevant, with no internal
signal for "we might be missing an entire category of fact" — `ErrNoProgress`
catches *stalling*, not *incompleteness*. A wavefront run can derive many true,
verified facts every round and still terminate having never asked about the
thing that actually mattered.

Tree-of-Thought's generate-multiple-candidates-and-evaluate pattern is a partial
answer to that gap (independent resampling surfaces angles a single pass might
miss; backtracking gives an actual recovery path) but imports a worse problem:
its evaluator is an LLM judgment that can irrecoverably prune the correct branch,
and its speculative "thoughts" have no grounding discipline at all — exactly the
hazard wavefront's `Need`/`Know` split was built to eliminate.

Working through a hybrid surfaced a cleaner synthesis than "alternate between the
two engines wholesale": decouple *hypotheses* (candidate explanations, tracked
with a likelihood that can rise or fall as evidence accumulates, never pruned,
only re-ranked) from *facts* (execution-verified, never colored by any attached
narrative). This is structurally closer to classical abductive
diagnosis — generate candidate explanations from observations, score
plausibility, select tests that discriminate between live candidates, update as
evidence arrives — than to either wavefront's forward-chaining lineage or ToT's
game-tree lineage. Critically, a bad likelihood judgment under this scheme costs
*investigation priority*, never *correctness*: a hypothesis can never become a
fact just by being believed, only by an actual command executing and producing a
real result. That is a categorically safer failure mode than ToT's evaluator,
and it required no round-synchronization or dedicated search machinery to build.

## Decision

### 1. The context schema

`task.Record.Kind` gains a third value, `KindHypothesis`, alongside `Task`/`Step`
— reusing the existing graph substrate rather than adding a parallel data
structure, the same instinct ADR 0012's amendment already applied when it
retired a standalone `Blackboard` type in favor of the graph itself.

```go
const KindHypothesis Kind = "hypothesis"

// Likelihood is always an LLM judgment — advisory investigation priority, never
// treated as confirmed fact. Impossible is a terminal state distinct in kind
// from Low, not merely its bottom rung: it requires a specific Evidence entry
// with StanceRefutes that directly, decisively contradicts the hypothesis, not
// "no supporting evidence found yet" (that stays Low). "Impossible" names a
// decisively-refuted-by-observation state, not logical impossibility the way
// Game-of-24's evaluator vocabulary can claim — real-world evidence rarely
// proves anything that strongly.
type Likelihood string

const (
    LikelihoodHigh       Likelihood = "H"
    LikelihoodMedium     Likelihood = "M"
    LikelihoodLow        Likelihood = "L"
    LikelihoodImpossible Likelihood = "I"
)

// Stance is how one piece of evidence bears on a hypothesis. Required on every
// link — an unstanced "this fact is relevant" list is exactly the ambiguity
// that let a worked example in this design's own drafting mislink "file not
// found" as support for "config is malformed" when it more plausibly refutes
// it (a missing file is a different failure category than a malformed one).
type Stance string

const (
    StanceSupports Stance = "supports"
    StanceRefutes  Stance = "refutes"
)

// Evidence links a Hypothesis-kind node to a fact-node that bears on it.
// NodeID references an existing task.Record.ID — no new identity scheme. This
// is deliberately its own field, not a reuse of Record.Deps: Deps is a
// scheduling precondition (blocks dispatch until resolved); Evidence is a
// retrospective, non-blocking relationship. Conflating the two risks a
// hypothesis accidentally gating dispatch order the way a real dependency
// does.
type Evidence struct {
    NodeID string
    Stance Stance
}

// AssertionOutcome reuses ADR 0010's tri-state vocabulary verbatim — this ADR
// extends that judgment from per-node to root-level, not a new vocabulary.
type AssertionOutcome string

const (
    OutcomeSatisfied AssertionOutcome = "satisfied"
    OutcomeRefuted   AssertionOutcome = "refuted"
    OutcomeAbstained AssertionOutcome = "abstained"
)

// ResolutionAssertion is a falsifiable claim whose satisfaction resolves
// PROBLEM. At least one is required, declared at generation time (ADR 0010's
// discipline: an assertion is a constraint the planner supplies before
// investigation begins, never invented retroactively to match whatever was
// found). Multiple form a disjunction — satisfying any one ends the
// investigation, since most real problems admit more than one valid shape of
// "done." Additional criteria may be proposed mid-investigation only through
// the same disciplined, evidence-grounded mechanism a new Hypothesis is
// proposed through — Declared distinguishes "initial" from "added round N" so
// goalpost-movement is auditable, not silent.
type ResolutionAssertion struct {
    Text     string
    Outcome  AssertionOutcome
    Evidence []Evidence
    Declared string
}

// Additive on task.Record, meaningful only when Kind == KindHypothesis:
//   Likelihood Likelihood
//   Evidence   []Evidence
```

**Mechanical precondition, reused without modification from ADR 0010:** a
`ResolutionAssertion` may only be judged `Satisfied` or `Refuted` when every
cited `Evidence` entry is itself a cleanly-resolved (non-abstained) fact. Root-
level resolution judgment is the highest-inference, hence highest-hallucination-
risk call in the whole pipeline — it's judged against an aggregate of
accumulated context, not one tool's output — so it gets the *strictest*
application of this rule, not a relaxed one. When cited evidence doesn't cleanly
settle it, the outcome is `Abstained` — investigation continues — never a
confident answer manufactured from ambiguous grounds.

The corresponding render, consumed by every classify/consolidation call:

```
# PROBLEM
{root.Goal}

# RESOLUTION CRITERIA (any one satisfies)
- [ ] {assertion.Text}
- [x] {assertion.Text} — satisfied: {cited Evidence}

# HYPOTHESES
## {hyp.Goal} — likelihood (H/M/L/I): {hyp.Likelihood}
### Evidence
- [supports] {fact.Goal}: {fact.Value}
- [refutes]  {fact.Goal}: {fact.Error}

# KNOWN
- {fact.Goal}: {fact.Value | fact.Error}
  (every Done Task-kind node, regardless of whether linked as Evidence anywhere
  — an unlinked fact here is the abduction-trigger signal, §2)

# NEED TO KNOW — diagnostic
- {open question bearing directly on PROBLEM}

# NEED TO KNOW — deferred
- {open question adjacent but not required to resolve PROBLEM}
```

`Impossible`-ranked hypotheses drop out of the rendered `HYPOTHESES` section
once retired (collapsed to a one-line note, or omitted from the prompt-facing
render and kept only in the stored graph for audit) — the same token-cost
discipline Context Curation applies everywhere else; `Impossible` retires
immediately on assignment, `Low` retires after some number of stale rounds with
no new evidence.

### 2. Two tiers, not one loop

**Tier 1 — continuous, cheap, per-item. This is wavefront (ADR 0012), unchanged,
not reimplemented:**

- **Execute** — resolve an open Need-to-Know's command into a `Known` fact.
- **Decompose** — classify a Need-to-Know or Hypothesis into child questions.
- **Link** (new, lightweight) — when a fact resolves, check it against
  currently-live hypotheses for an obvious, direct `Stance`. Single-fact-
  against-one-hypothesis, cheap, fans out fine.
- **Mechanical resolution check** (new, lightweight) — does this fact, on its
  own, directly satisfy or refute a `ResolutionAssertion` (ADR 0010's already-
  designed fast-path — skip the expensive judge call when the tool's own result
  shape settles it).

None of Tier 1 needs a gather step. Wavefront's continuous dispatch makes zero
LLM calls just to decide whether to act on a `Ready()` node — that property is
preserved exactly by keeping Tier 1 untouched.

**Tier 2 — periodic, gathered, comparative:**

- **Evaluate Hypotheses (comparative)** — re-rank all live hypotheses against
  each other, not just against new facts in isolation. Comparative judgment is
  more reliable than isolated scoring (the same lesson ToT's own literature
  draws from voting vs. absolute scoring); Tier 1's `Link` already catches the
  obvious per-fact cases cheaply, so this catches only the subtler ones that
  only show up when hypotheses are weighed against each other.
- **Abduce** — propose new hypotheses. Triggered specifically by any `Known`
  fact still unlinked to any hypothesis after `Link` has run (a cheap, structural
  graph query — not an LLM judgment call to detect), plus a periodic fallback
  even without one.
- **Check Resolution (comprehensive)** — judge each `ResolutionAssertion`
  against the *full* known set, catching satisfaction that only emerges from a
  combination of facts, none individually sufficient.
- **Propose New Resolution Criteria** — rare, gated specifically on Abduce
  having just produced a hypothesis that implies a different shape of "done"
  than currently declared, not run on a general cadence. Tagged
  `Declared: "added round N"`.

### 3. Tier 2 is realized as a `ConversationCore` `SyncHook` — zero changes to that loop

Tier 1 is wrapped as one native tool a sub-session's `ConversationCore` calls
repeatedly — `continue_investigating` — which runs wavefront's dispatch to its
own internal stall point (`ErrNoProgress`) and returns a compact status summary.
Tier 2 is a dedicated, named type (not an anonymous closure — its role as the
core consolidation algorithm should be discoverable from its name, not buried as
generic pluggable "extension" logic) satisfying `hooks.SyncHook`:

```go
type ConsolidatorHook struct {
    graph *task.Graph // shared with the continue_investigating tool
    chat  Chat        // the hook makes its own LLM calls directly — see below
}

func (h *ConsolidatorHook) Run(ctx context.Context, turn *hooks.Turn) error
```

Two design points settle a risk raised while working through the shape of this:

- **The hook performs Tier 2 directly rather than merely nudging the model
  toward it.** `Run` can hold its own `Model`/`Chat` reference and make real LLM
  calls inside itself, exactly as `wavefront.LLMClassifier` already does —
  nothing in `hooks.SyncHook`'s interface prevents this. This is materially more
  reliable than appending a "consider re-evaluating your hypotheses" message and
  hoping the model calls the right tool next round; the hook runs Tier 2
  deterministically when its cheap stall check says to, writes results onto the
  graph, and folds a summary into `turn.Messages` before the next model call.
- **This resolves ADR 0013's own earlier-flagged concern about non-fatal
  signaling** (`SyncHook`'s only return-value signal is fatal-abort, the wrong
  shape for a non-fatal strategy pivot). A pivot doesn't need a new control-flow
  signal at all — it's achieved entirely as a context-mutation side effect on
  `turn.Messages`, which `SyncHook` already fully supports.

The stall-detection gate itself is a cheap, deterministic graph query (compare
node/fact counts before and after the last `continue_investigating` round — the
same shape as `ErrNoProgress` already is), never an LLM call — keeping the hook
fast on every round where nothing needs consolidating.

Both existing hook points get used, for different checks:

- **Hook point 1** (before the first model call of a turn) — bootstrap check:
  first round, nothing decomposed yet → run initial abduction from the problem
  statement alone.
- **Hook point 2** (after each round of tool execution) — stall check: if the
  just-completed `continue_investigating` round made no progress, run full Tier
  2 consolidation before the next model call.

**Hook registration is additive, not exclusive.** The tool `RegisterSync`s its
`ConsolidatorHook` onto the *same* `hooks.Registry` `hooks.Build` populates from
`Settings.HooksConfigPath` — it does not construct or own a separate registry.
A user's own configured hooks and the tool's built-in consolidator coexist in
registration order.

### 4. Termination

- Any `ResolutionAssertion` reaches `Satisfied` → done.
- **Tier 2 itself stalling** (a consolidation pass producing no new hypotheses,
  no likelihood movement, no new Need-to-Knows, nothing newly satisfied) is a
  second-order `ErrNoProgress` — one level above wavefront's own — and
  escalates rather than looping forever, mirroring ADR 0010's `abstained`-
  exhaustion-escalates pattern (leaning toward the `Ask` route per ADR 0010's own
  Open Question 3, not decided here either).
- A Tier-2-cycle cap exists as a backstop only, never the primary stop
  condition — consistent with "a plan should stop because it stopped learning,
  not because it hit a count" (ADR 0012 §"Termination").

### 5. Naming: Tidal

Tier 1's continuous dispatch is a steady wave, no periodicity — wavefront,
unchanged. Tier 2 is a different motion on the same body of water: slower,
periodic, gathering everything before it moves — the relationship between tides
and surface waves in the literal physical sense. The name is chosen specifically
to signal *extension*, not *replacement*: Tidal runs wavefront as its Tier 1
engine rather than reimplementing it, the same relationship ADR 0012 held to
ADR 0008.

## Architecture — insertion points (none built yet; this ADR is design-only)

- **Changed:** `internal/prompting/task/task.go` — `Kind` gains `KindHypothesis`;
  `Record` gains `Likelihood`/`Evidence` (meaningful only for
  `Kind == KindHypothesis`); a new `internal/prompting/task` (or sibling)
  location for `Stance`, `Evidence`, `AssertionOutcome`, `ResolutionAssertion` —
  shared substrate both Tier 1 (wavefront, unchanged) and Tier 2 read/write.
- **New:** `internal/runtime/tidal/` — sibling to `decompose`/`scheduler`/
  `wavefront`, matching that established siblinghood (`docs/implementation/
  08_go_module_layout.md`'s existing `internal/runtime` guidance). Houses the
  render function (the `PROBLEM`/`RESOLUTION CRITERIA`/`HYPOTHESES`/`KNOWN`/
  `NEED TO KNOW` template), `ConsolidatorHook`, the `continue_investigating`
  tool's implementation (wrapping `wavefront.Scheduler` unchanged as Tier 1),
  and the Tier 2 operations (`EvaluateHypotheses`, `Abduce`, `CheckResolution`,
  `ProposeResolutionCriteria`).
- **Depends on, not yet resolved:** how `continue_investigating` and
  `ConsolidatorHook` attach to a *sub-session* `ConversationCore` specifically
  depends on ADR 0013's still-open Open Question 2 (the sub-session
  `ContextStore`'s shape) and Open Question 4 (whether a planning tool becomes a
  `ConversationCore` consumer at all). This ADR designs Tidal's mechanism
  assuming both land compatibly; it does not resolve either.

## Consequences

Positive:

- Closes the specific gap wavefront alone leaves open (incompleteness, not just
  stalling) without importing ToT's evaluator-can-irrecoverably-prune risk —
  nothing in Tidal is ever discarded, only re-ranked; a bad `Likelihood`
  judgment costs investigation priority, never correctness.
- Reuses proven substrate throughout rather than inventing new machinery: the
  graph (no new data structure), `ErrNoProgress` (reused as the Tier 1→Tier 2
  trigger, and generalized as the Tier 2 termination signal), ADR 0010's
  tri-state vocabulary and mechanical precondition (reused verbatim, extended
  from per-node to root-level), and `ConversationCore`'s existing hook seam
  (zero modification required — proven independently instantiable and testable
  by ADR 0013 Phase 5 before this ADR needed either property).
- The `Evidence`/`Stance` requirement is directly motivated by a real
  misattribution this design's own drafting produced by hand ("file not found"
  initially mislinked as support for "config malformed") — not a hypothetical
  risk invented for the ADR.

Trade-offs:

- Each Tier 1 *pass* (not each Need — one `continue_investigating` call, which
  may resolve many Needs internally) costs one real LLM round-trip for the
  sub-session's model to decide to keep investigating. This is unavoidable
  given the design constraint of routing through `ConversationCore`'s existing
  loop rather than a background loop outside any model turn — accepted
  deliberately, since it's a per-*pass* cost, not wavefront's per-*Need*
  dispatch cost, which stays entirely LLM-free as designed.
- Real, load-bearing logic (Tier 2's consolidation) lives inside a hook
  implementation, which reads as generic/optional extension machinery to
  someone unfamiliar with this ADR. Mitigated by naming (`ConsolidatorHook`, its
  own package) but not eliminated — worth remembering when documenting
  `ConversationCore` itself, not just this ADR.
- Depends on two of ADR 0013's Open Questions resolving compatibly
  (sub-session `ContextStore` shape, plan_task/planning-tool relationship to
  `ConversationCore`) — this ADR's mechanism is designed against an assumption
  about both, not a guarantee.

## Phased Build Plan

1. **Schema.** `Kind` gains `KindHypothesis`; `Likelihood`/`Evidence`/`Stance`/
   `AssertionOutcome`/`ResolutionAssertion` added to `internal/prompting/task`.
   Additive only — existing `task.Graph`/scheduler/wavefront tests must pass
   unchanged, mirroring the discipline ADR 0012's own `Value`/`Error`/`Seq`
   phase required.
2. **Render function.** The five-section template over `task.Graph.Nodes()`,
   filtered/grouped by `Kind`/`Status` — a pure function, unit-testable with
   fixed inputs and expected string outputs, same posture as wavefront's own
   `Render`-equivalent.
3. **Tier 1 wrapper.** `continue_investigating` as a native tool wrapping
   `wavefront.Scheduler` unchanged; behavior-preserving for wavefront itself
   (full existing wavefront suite passes unchanged).
4. **Tier 2 operations.** `EvaluateHypotheses`/`Abduce`/`CheckResolution`/
   `ProposeResolutionCriteria` as standalone, independently-testable functions
   against the schema — no hook wiring yet.
5. **`ConsolidatorHook`.** Wires phase 4's operations behind the stall-detection
   gate, satisfying `hooks.SyncHook`; tested standalone against a fake `Turn`
   and a stub graph, mirroring `core_loop_test.go`'s existing fake-driven
   pattern — no `ConversationCore` or sub-session wiring required to prove this
   phase.
6. **Sub-session wiring.** Attaching `continue_investigating` + `ConsolidatorHook`
   to a real sub-session `ConversationCore` — blocked on ADR 0013 OQ2/OQ4
   resolving; scoped as its own follow-on, not part of this build plan's first
   five phases.

Every phase's touched functions need a GIVEN/WHEN/THEN behavior doc before
implementation, per repo invariant — phase 1 (schema) and phase 5
(`ConsolidatorHook`'s stall-gate logic) are the two contracts most worth writing
first, since they're where a bug would either corrupt the graph's shared meaning
across both tiers or cause Tier 2 to never fire (silent incompleteness — the
exact failure this ADR exists to close) or fire needlessly (cost regression).

## Open Questions

1. **Retry budget for `Abstained` resolution criteria**, mirroring ADR 0010's
   own unresolved Open Question 1 for per-node assertions — a real number is
   product judgment, not architecture, and needed before phase 6.
2. **Hypothesis-list retirement cadence.** "Some number of stale rounds" (§1) is
   not yet a number; needs the same kind of measurement-before-deciding ADR
   0012 applied to its own `maxRounds` question before it was retired as moot.
3. **Mechanical-fast-path classification for `ResolutionAssertion`**, mirroring
   ADR 0010's own Open Question 4 exactly — declared by the planner at
   generation time vs. inferred from the assertion's shape at evaluation time.
   Deferred to phase 3/4 design, not decided here.
4. **Escalation surface for Tier-2 stall**, mirroring ADR 0010's own Open
   Question 3 — a new `Ask`-route question or a distinct UI affordance. Leaning
   `Ask`, not decided.
5. **Sub-session attachment mechanics** (ADR 0013 OQ2/OQ4) — this ADR's phase 6
   is explicitly blocked on these, not resolved here.
