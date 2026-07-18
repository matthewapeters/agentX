# ADR 0012 — Wavefront-Grounded Decomposition and the Shared Blackboard

Status: Proposed; **amended 2026-07-17** (graph-as-blackboard, continuous
convergence, engine interleaving noted as future — see amendment at the end)
Date: 2026-07-16
Deciders: AgentX architecture owners
Depends on: ADR 0008 (recursive task decomposition + DAG scheduler, incl. 2026-07-07
typed-node amendment), ADR 0009 (plan & tool execution visibility), ADR 0010 (task
assertions, outcome grounding, plan continuity)
Extends (does not supersede): ADR 0008. See "Relationship to ADR 0008" below — this is
not a reversal of the Kind-at-generation-time amendment, and the two mechanisms are
meant to run side by side, not replace one another.

**Numbering note:** several files in this codebase (`planner.go`, `render_test.go`,
`decompose/live.go`) already cite "ADR 0011" for the system/user prompt role
separation decision, and its behavior doc
(`docs/architecture/behavior/adr/0011_planner_prompt_role_separation.feature.md`)
exists on disk — but no `0011-*.md` file exists in this folder. That decision shipped
without its ADR ever being written down. This document is numbered 0012 to avoid
colliding with that already-spoken-for slot; backfilling 0011 is a separate, smaller
piece of work not undertaken here.

## Context

### The observed failure

AgentX's decompose path (ADR 0008, as amended) produces plans that hallucinate: a
`Task` node reads a file that was never confirmed to exist, and the plan's final
answer is synthesized from that hallucinated read. This is not a tuning problem in the
prompt text — it is a structural gap in how a plan's arguments get decided.

`planner.Parse` (`internal/prompting/planner/planner.go:236-245`) binds every `Task`
node's `args` to literal strings **at plan-generation time**, inside the single model
call that also declares the node's `deps`. A dependency edge (`task.Graph.Ready()`,
`internal/prompting/task/graph.go:161-180`) gates **when** the scheduler dispatches a
node; nothing in `internal/executor` or `internal/prompting/task` binds a node's args
to a dependency's actual result. A plan can legally — and in practice does — contain:

```
s1: task(list_dir ".")
s2: task(cat_file "main.py"), deps: ["s1"]
```

`s2`'s path was decided in the same model call that decided to run `s1`, before `s1`
had produced anything. The `deps` edge makes this *look* like "discover, then read" —
the scheduler genuinely waits for `s1` before running `s2` — but no re-grounding
happens between those two decisions. If the real entry point is `cmd/agentx/main.go`,
`s2` was always going to hallucinate `main.py`, and no guard in the current design
catches it, because nothing here repeats or echoes — the existing `SimilarGoals`
guard (`internal/runtime/decompose/guard.go`) and the typed-node amendment's
degrade-on-violation rule both exist to catch a *spiral*. A single confident,
ungrounded guess on the first pass isn't a spiral. It's the failure mode neither
mechanism was built to catch.

### A companion investigation

A proof-of-concept in a sibling repository (`../totAlX`, reviewed alongside this ADR —
not vendored, not a dependency, purely a design reference) runs a structurally
different kind of decomposition against the same local Ollama substrate. Its
discipline: classify every open question into KNOW (already known, or derivable by
synthesis from what's known) and NEED (not known — either a further open question, or
a command that can resolve it), execute only what resolves against **currently
verified** facts, merge the real results into a shared Working Memory, and only then
classify the next wave. Its own transcripts show the self-correction this produces: a
guessed path (`cat .../README.md`) failed loudly in round 1; round 2's decision was
grounded in the real `ls -la` output already sitting in Working Memory from round 1,
and it read the file that actually existed. No node in that design can commit to an
argument it hasn't verified, because "not yet verified" and "ready to act on" are
different node states, not different phrasings of the same JSON array.

totAlX's own methodology notes (its README) document real, working lessons worth
importing selectively — round-synchronized grounding, a shared blackboard with
convergence, chain-aware output summarization in place of truncation, and the finding
that reasoning/"thinking" cost dominates latency far more than prompt size. It also
has real weaknesses agentX should not import: no schema-constrained decoding (fragile
fence-stripping and JSON-repair loops agentX's `Format`-constrained `Complete` already
avoids), fragile string-identity naming discipline (agentX's mechanically-namespaced
child IDs avoid this entirely), and — checked directly against its own transcript — an
inconsistent worked example in its own system prompt that shows a command's result as
already known in the same response that proposes running the command, which is an
instance of the exact hallucination pattern this ADR exists to close. This ADR takes
the grounding discipline, not the implementation.

### Relationship to ADR 0008 — extension, not reversal

The 2026-07-07 typed-node amendment retired a per-node atomicity Oracle. Every named
incident behind that retirement — `tidy-cove` (ten re-plannings of the same `ls -la`),
`mellow-meadow`, `amber-quartz` ("list the project root" generated three separate
times because a decompose call couldn't see what an earlier step had already found),
`vivid-beacon-2` (a listing re-derived even though it was already in Working Memory) —
is a call that produced **no new verified information**: an echo, a repeat, or a call
that ignored evidence already in hand. The amendment's actual target was never "fewer
calls" as an end in itself; it was calls that don't advance the plan.

State the test the amendment was already gesturing at, explicitly, as the principle
this ADR extends rather than departs from:

> **A call is judged by whether it is expected to yield new verified evidence, not by
> whether it is "another call."**

A wave-classify call under this ADR only fires after the previous wave's real command
results have been merged into the blackboard, so it always has strictly more grounding
than the call before it — it cannot repeat, because its inputs changed. That is not
the failure category the amendment retired the Oracle for; it is the category the
amendment's Kind-declaration alone still leaves open (a confident guess on the first,
only pass). No part of the typed-node amendment is undone here: `Kind` (Step vs. Task),
the ≤5-children cap, the one-retry-then-degrade rule, and `SimilarGoals` all keep
governing the existing continuous scheduler exactly as they do today. This ADR adds a
second, alternative decomposition engine that a plan can run under instead — selected
by settings, not a replacement of the first.

### The blackboard is a convergence mechanism, not a cost optimization

When independent branches of the same plan hinge on common data, sharing what's
discovered doesn't just save a call — it raises the odds that *some* branch converges
on a correct answer, including letting a branch that fails leave real evidence behind
for a sibling. What's already built gets most of the way there for **completed** work:
`internal/planfindings` (`Source func() string`, read fresh on every call) gives any
later decompose call in the same plan-drain live visibility into every already-completed
leaf, success or failure — `capturingExec.Execute` (`internal/runtime/plan_cycle.go:61-73`)
appends every outcome unconditionally, so a failed guess is already usable negative
evidence for siblings today.

What's missing is convergence across branches that are **concurrently in flight** —
two `Step` nodes dispatched at once, each forking an isolated branch, neither visible
to the other until one finishes. This is the gap totAlX's `THINGS TO BE CONSIDERED`
list closes: a currently-open, not-yet-resolved question that a second branch can
notice and converge onto instead of independently re-deriving.

**Scope of the pool, resolved by direct product input (not re-litigated here):** the
knowledge pool is scoped to one plan-drain — matching `capturingExec`'s existing
lifetime exactly. This is deliberately narrower than "persist across turns." A manual
override already exists and already solves that broader case: a user can pin a task
result to Working Memory from the context surface
(`internal/session/workmemory.go`, `internal/surfaces/workmemory`), outside any plan
machinery. Building automatic cross-drain persistence here would duplicate a path that
already works and is under the user's control. The one hard constraint this places on
the design below: the new blackboard is strictly additive alongside `branch.Fork`'s
existing `Facts()` seeding (`internal/runtime/decompose/decompose.go:60-64`) and must
never shadow or intercept a pinned fact's path into that seed.

## Decision

Introduce **wavefront-grounded decomposition** as a second decomposition engine,
implemented as a new package, selectable per session/settings alongside the existing
continuous scheduler (ADR 0008) — not a modification of it.

### 1. `internal/runtime/wavefront` — a new, self-contained package

Sibling to `internal/runtime/decompose` and `internal/runtime/scheduler`, not nested
inside either. It reuses rather than re-implements:

- `task.Graph` (`internal/prompting/task/graph.go`) **as-is** — `Add`, `Update`,
  `Ready()`, dangling-dep and cycle protection all carry over unchanged. A "leaf" for
  wavefront purposes is `Ready()` filtered to nodes not yet classified in the current
  round — a thin, round-local map the wavefront scheduler tracks itself, the same way
  `scheduler.Scheduler` already tracks `dispatched`/`inflight` locally
  (`internal/runtime/scheduler/scheduler.go:143-149`). **No new graph type and no
  mutating prune are needed** — `task.Graph` stays the append-only event-fold it's
  documented as (`graph.go:10-19`); "resolved and no longer a candidate" is a
  `Status` read, not a removal. This is materially less work than it looked like
  before checking: the graph substrate the scheduler already has is sufficient.
- `scheduler.Executor` (`internal/runtime/scheduler/scheduler.go:76-79`) — the exact
  same interface, same tool registry, same policy/approval gate. Immediate command
  resolution under wavefront calls `Execute` through this seam, unchanged. "The tools
  list is used as currently used in AgentX" is satisfied by reusing the type, not by
  re-describing it.
- `plannerCatalog` (`internal/runtime/classifier_pipeline.go:189-`) — the same
  drift-proof tool catalog rendering, reused verbatim for the new classify prompt's
  tool list.
- `branch.Fork`'s existing session-fact seeding (`internal/runtime/decompose/decompose.go:60-64`)
  — the "start from what's already known globally" requirement (cwd, project,
  repo_root) is already built; the blackboard below only adds the plan-scoped,
  round-accumulated layer on top.
- `decompose.Chat` (`internal/runtime/decompose/live.go:17`) — the same
  system/user-role `Chat` func type, so the new `Classifier` implementation is
  constructed the same way `LLMPlanner` is, against the same Ollama client.

### 2. The `Classifier` contract — wavefront's sibling to `Planner`

```go
// Know is a fact the model already has or can synthesize from the blackboard.
type Know struct{ Name, Value string }

// Need is something required but not yet known. A non-empty Command resolves it
// immediately and deterministically via the executor; an empty Command makes it a
// new open question — a child node for the next round.
type Need struct{ Name, Command string }

type Result struct {
    Knows []Know
    Needs []Need
}

// Classifier asks, for one open question against the current blackboard: what do we
// already know or can synthesize, and what's still needed? Unlike Planner.Plan, this
// never resolves a downstream argument that depends on a not-yet-executed sibling —
// a Need's Command, when present, must be answerable from wm alone.
type Classifier interface {
    Classify(ctx context.Context, wm, question string) (Result, error)
}
```

`LLMClassifier` (the production implementation) renders a new system/user template
pair under `planner.PlanSchema`-style JSON-schema-constrained decoding (reusing
`internal/jsonx` for fence-tolerant parsing as a backstop, same as `planner.Parse`) —
this is the one place agentX's design is strictly better-founded than totAlX's: totAlX
has no schema constraint on this call at all and defensively strips markdown fences;
agentX's `Complete` already supports `Format`, so the KNOW/NEED shape is constrained,
not just requested in prose.

### 3. The blackboard — per-drain, live, with in-flight convergence

```go
// Blackboard is one plan-drain's shared, mutable fact store plus its list of
// currently-open (not yet resolved) questions, for cross-branch convergence. Scoped
// to exactly one drain's lifetime — never persisted past it. Seeded from the same
// branch.Facts() session snapshot decompose.Decomposer already uses.
type Blackboard struct {
    mu          sync.Mutex
    facts       map[string]string
    considering map[string]string // normalized name -> owning node id
}

func (b *Blackboard) Know(name, value string)
func (b *Blackboard) Get(name string) (string, bool)
// Consider registers name as being worked on by nodeID. If another node already
// claimed the identical (normalized) name, it returns that node's id and false —
// the caller converges its edge onto the existing node instead of creating a
// duplicate, exactly totAlX's THINGS TO BE CONSIDERED convergence.
func (b *Blackboard) Consider(name, nodeID string) (owner string, isNew bool)
func (b *Blackboard) Resolve(name string) // drops it from "considering" once known
func (b *Blackboard) Render() string      // facts as prompt text for the next Classify call
```

Mutex-guarded for the same reason `capturingExec` already is: wavefront dispatches a
round's leaves concurrently (up to `slots`), and results merge back on completion.
Unlike `capturingExec`, whose `Findings()` only reflects **completed** steps,
`Consider`/`considering` is read and written by branches that are still **in flight** —
this is the actual gap being closed, not a restatement of what `planfindings` already
does.

### 4. The wavefront scheduler — round-synchronized, not continuous

```go
type Scheduler struct {
    graph      *task.Graph
    board      *Blackboard
    classifier Classifier
    executor   scheduler.Executor // the existing interface, reused
    slots      int
    maxDepth   int
    maxRounds  int
}

func (s *Scheduler) Run(ctx context.Context, root task.Record) (Outcome, error)
```

One round: compute this round's leaves (`Ready()`, not yet classified this pass),
dispatch `Classify` for each concurrently (bounded by `slots`), collect every result,
execute every command-bearing `Need` immediately through `executor.Execute`, merge all
real `Know`s and command results into the blackboard **only after the whole round's
work has returned** — deliberately synchronous at the round boundary, unlike the
continuous scheduler's immediate slot-backfill. A `Need` with no command either
converges onto an existing `considering` entry (edge added, no new node) or becomes a
new child node with a dependency edge back to its parent. The round terminates the
plan when a `Know`'s normalized name matches the root's own normalized goal
(`normalize`: trim, collapse whitespace, casefold — the same discipline totAlX
documents as necessary even for a compliant model); if the root's own leaves are
exhausted with no such match, one dedicated schema-free synthesis call runs (see
§6), mirroring totAlX's two-phase decompose-then-synthesize fix — but note **agentX's
Task/Step split already prevents the specific bug that fix was patching** (a plan node
never both proposes an action and claims to have already answered in the same
response), so this call exists to produce the final answer, not to recover a silently
discarded one.

**Termination is the productive/unproductive test made operational, not a bare
counter.** A round that yields zero new `Know`s and zero successfully-executed `Need`s
across every leaf it dispatched is `scheduler.ErrNoProgress` — reusing the existing
exported error rather than minting a duplicate, since it is the identical signal: this
call/round did not advance the plan. `maxRounds` and `maxDepth` remain as backstops
(matching totAlX's own `MAX_ITERATIONS`/`MAX_DEPTH`, and the continuous scheduler's
`DefaultMaxDepth`), never the primary stop condition — accuracy-first per the guiding
principle: a plan should stop because it stopped learning, not because it hit a count.

### 5. Immediate command execution reuses the executor and its guardrails as-is

A command-bearing `Need` is not a new kind of tool call — it dispatches through
`scheduler.Executor.Execute` exactly like a `Task` node does today, under the same
policy gate, approval flow, and workdir confinement (ADR 0008 §1, ADR 0009 §2). There
is no separate, weaker allowlist to build (totAlX's own regex allowlist is a strictly
worse mechanism than what agentX already has: agentX's model sees the actual permitted
tool catalog and cannot propose a disallowed command in the first place, rather than
guessing and getting silently blocked post-hoc, which wastes a round every time it
happens — visible directly in totAlX's own transcripts).

### 6. Output summarization replaces truncation for oversized findings

Independent of wavefront specifically, but motivated by the same grounding standard
and applicable to both decomposition engines: `plan_cycle.go`'s `findingsLines`/
`midDrainFindingsLines` (`internal/runtime/plan_cycle.go:31,79`) currently cap oversized
tool output by straight prefix truncation (`firstLines`). This is the same failure
class as the `lively-raven` incident already named in that file's own comments (a
`tree` call's real structure discarded to its first ~20 alphabetical entries, the model
concluding "Python-based" from what survived). Add a chain-aware, schema-free
summarization call — general-to-specific question chain as context, targeted at the
most specific item only, truncation kept strictly as the last-resort fallback if
summarization itself fails after retries. This is a direct, small port of totAlX's
`summarize_output`, and it benefits the existing continuous scheduler's findings
pipeline as much as wavefront's — it is not gated behind picking one engine over the
other.

### 7. Reasoning effort, constrained for fast rounds

`ollama.CompleteRequest` (`internal/llm/ollama/ollama.go:149-156`) currently has no
`Think` field at all — only `Temperature`, `Seed`, `Format`, `NumCtx`. The respond
path's `ThinkingEnabled`/`ThinkingBudget`/`ThinkingRoutes` machinery
(`internal/runtime/orchestrator.go:62-73`) already exists and is strictly better than
what totAlX had to work with (a hard timer via `time.AfterFunc`, not an unreliable
soft "keep reasoning under N characters" prompt instruction) — it is simply not wired
to `Complete`, the call `LLMPlanner`/`LLMClassifier`/the classifier fan-out all use.
Add `Think bool` to `CompleteRequest`, threaded the same way `Temperature`/`Seed`
already are (`Complete`'s `options` map), and give wavefront its own budget setting
(`WavefrontThinkingBudget`) rather than silently inheriting the respond path's —
wavefront's classify rounds are the highest round-trip-count path in the system, and
totAlX's controlled measurement (>25x wall-time difference from this one flag, prompt
size and server load held constant) makes this the single highest-leverage lever
available for wavefront's actual latency. A reasoning-budget instruction in the new
classify prompt is a reasonable belt-and-suspenders addition, but per totAlX's own
finding it must not be relied on alone — the hard timer is the mechanism that matters.

### 8. Prompts as seed config files, mirroring the existing convention exactly

New seed files under `config/seed/`, following `agentx-planner.md`'s existing
load/override/fallback pattern (`Settings.PlannerPrompt` → `planner.DefaultPromptTemplate`):

- `agentx-wavefront-classify.md` — the KNOW/NEED system prompt (rules + tool catalog
  placeholder), analogous to `DefaultPromptTemplate`.
- `agentx-wavefront-synthesis.md` — the schema-free "answer from the blackboard"
  prompt (§4's fallback synthesis call).
- `agentx-wavefront-summary.md` — the chain-aware output-summarization prompt (§6).

Each gets a `Settings` field (`WavefrontClassifyPrompt`, etc.), loaded and defaulted
the same way `PlannerPrompt`/`ThinkingPrompt` already are in `internal/app/app.go` and
`internal/config`. No new loading mechanism — this is the existing one, applied three
more times.

### 9. Selection: a second engine, not a switch inside the first

`runPlanPhase` (`internal/runtime/plan_cycle.go:183`) gains a settings-gated branch: a
new `Settings.WavefrontEnabled bool` (default `false`) routes to a new
`runWavefrontPhase`, constructed in a new `internal/runtime/wavefront_cycle.go`
mirroring `buildDecomposition`'s construction of today's `decompose.Decomposer` —
same `client.Complete` (now `Think`-capable per §7), same `plannerCatalog`, same
`o.taskExec`. Both engines coexist behind the same orchestrator entry point; a session
runs one or the other, never both on the same plan. This is what makes "A/B testing
prompts, settings" structural rather than a separate harness: swapping
`WavefrontEnabled`, or pointing `WavefrontClassifyPrompt` at a variant file, is the
entire mechanism. `decompose.DrainPlan`'s existing documented shape — "no bus, no I/O
... unit-testable" (`internal/runtime/decompose/drain.go:31-33`) — is mirrored by
wavefront's `Scheduler.Run`, so both are replayable against a fixed goal corpus by a
thin test harness with no product-code changes required to add that harness later.

### Open decision point: does wavefront need its own dedicated synthesis call?

§4's fallback synthesis call (used when no `Know` matches the root verbatim) is
wavefront-local. Separately, once a plan drains, its findings currently fold into the
**general respond call**'s prompt via `planContext` (`plan_cycle.go:270-288`) — a
single line of instruction ("Answer the request using only these findings") competing
for compliance against whatever else that assembled prompt carries (session history,
tool catalog awareness, etc.), rather than a dedicated, single-purpose call the way
totAlX's `synthesize_answer` is. Recommended default: wavefront's `Outcome` still
renders into the same `capturedStep`-compatible shape and reuses `planContext`/the
general respond call, for consistency with the existing engine and to avoid a second
code path for the same job. Whether to instead give wavefront a dedicated, always-on
schema-free final-answer call is left as an Open Question below rather than decided
here — it trades one more real LLM call for cleaner single-purpose grounding, and
should be evaluated with the eval harness (Phase 4) rather than assumed.

## Architecture — insertion points

- **New:** `internal/runtime/wavefront/` — `blackboard.go`, `classifier.go`,
  `scheduler.go`, `live.go` (the `LLMClassifier`/`Chat`-based production
  implementation, mirroring `decompose/live.go`'s split from `decompose/decompose.go`).
- **New:** `internal/runtime/wavefront_cycle.go` — `runWavefrontPhase`, construction
  wiring mirroring `buildDecomposition` (`classifier_pipeline.go:154-187`).
- **Changed:** `internal/llm/ollama/ollama.go` — `CompleteRequest` gains `Think bool`;
  `Complete`'s `options` map gains the `think` key conditionally, same pattern as
  `temperature`/`seed`/`num_ctx` (lines 161-170).
- **Changed:** `internal/runtime/orchestrator.go` — `Settings` gains
  `WavefrontEnabled`, `WavefrontMaxRounds`, `WavefrontThinkingBudget`,
  `WavefrontClassifyPrompt`, `WavefrontSynthesisPrompt`, `WavefrontSummaryPrompt`.
- **Changed:** `internal/runtime/plan_cycle.go` — `runPlanPhase` gains the
  `WavefrontEnabled` branch; `findingsLines`-based truncation (`firstLines`) is
  replaced by the summarization call from §6, benefiting both engines.
- **Changed:** `internal/app/app.go`, `internal/config` — load/default the three new
  seed files, same pattern as `plannerPrompt`/`thinkingPrompt`.
- **New:** `config/seed/agentx-wavefront-classify.md`,
  `agentx-wavefront-synthesis.md`, `agentx-wavefront-summary.md`.
- **Unchanged, reused as-is:** `internal/prompting/task` (Graph, Record, Kind,
  Status), `internal/runtime/scheduler` (Executor interface, ErrNoProgress),
  `internal/runtime/branch` (Fork, Facts snapshotting), `internal/tools` (registry,
  policy, catalog rendering), `internal/session/workmemory.go` (pin path — explicitly
  not touched; see Context).
- **Documentation:** `docs/implementation/08_go_module_layout.md`'s `internal/runtime`
  bullet gains a one-line mention of the new sibling subpackage — no new top-level
  folder, no new directory *pattern* (it is structurally identical to the existing
  `decompose`/`scheduler` siblings), so the Change Control process for a new top-level
  folder does not apply.

## Consequences

Positive:

- Closes the hallucinated-argument gap directly: no wavefront `Need` with a command
  can be emitted without the literal value already being in the blackboard, because
  the classify call only ever sees currently-verified facts — there is no JSON shape
  in which "resolve this later" and "resolve this now with a guessed literal" can be
  confused, the same way Task/Step already made "propose an action" and "claim an
  answer" structurally distinct.
- Extends ADR 0008's own stated principle (calls must be productive) rather than
  reopening its trade-offs; the generalized `ErrNoProgress` termination is a sharper,
  more accuracy-first stop condition than a bare iteration cap, consistent with
  "accuracy first, then performance."
- The blackboard's in-flight convergence closes a gap `planfindings` structurally
  cannot (visibility into work that hasn't completed yet), and does so without any
  new persistence layer — the scope is exactly what already exists, per direct product
  decision, and the existing manual pin path remains the sole mechanism for anything
  that must outlive a drain.
- Runs as a genuine second engine behind the same entry point, not a fork of the
  existing scheduler's internals — the continuous scheduler, its tests, and ADR
  0008/0010's guarantees are untouched. A regression in wavefront cannot corrupt the
  existing path, and either can be selected per session for direct comparison.
- Output summarization (§6) fixes a failure mode (`lively-raven`) already named and
  already hit in production, independent of which decomposition engine is active.

Trade-offs:

- **Round-synchronization costs concurrency efficiency relative to the continuous
  scheduler.** The continuous scheduler backfills a freed slot immediately with
  whatever's next-ready, regardless of "wave"; wavefront blocks an entire round on its
  slowest member before starting the next. totAlX's own numbers show this cost is
  real, not hypothetical — its `parallel(batch=1)` run (113s) was slower than
  `sequential(batch=1)` (69s) on an identical goal, purely from one straggler's
  reasoning length. This is accepted deliberately: grounding correctness is sequenced
  before dispatch efficiency, per the stated principle. A staggered-release variant
  (advance a lane once its own dependency chain clears, without waiting for the whole
  round) is a legitimate later optimization, scoped out here — see Open Questions.
- Two decomposition engines mean two things to reason about, test, and keep in sync
  with the tool registry/policy layer as it evolves — real, ongoing maintenance
  surface, not a one-time cost.
- The blackboard's `Consider`/convergence mechanism adds a genuinely new kind of
  cross-goroutine coordination beyond what `capturingExec` needed (append-only,
  read-after-write); it needs its own concurrency tests, not a reuse of the existing
  ones.
- A wavefront round's classify calls are real LLM calls, gated only by whether they're
  expected to be productive — this is by design, not a regression, but it means
  wavefront's total call count on a given goal is not obviously lower than the
  continuous scheduler's, and should be measured, not assumed.

## Phased Build Plan

1. **Reasoning-effort plumbing (no wavefront dependency).** `CompleteRequest.Think`,
   `Complete`'s options-map wiring, a `Settings.WavefrontThinkingBudget`-shaped knob
   proven out first against the *existing* planner/classifier `Complete` call sites
   (cheap, immediately useful regardless of what follows). Behavior doc:
   `docs/architecture/behavior/adr/0012_complete_thinking_control.feature.md`.
2. **Seed-file prompt templating.** The three new `agentx-wavefront-*.md` files, their
   `Settings` fields, load/default wiring — no runtime behavior yet, just the
   scaffolding proven against the existing `PlannerPrompt` pattern.
3. **Output summarization (§6).** Independent of wavefront; replaces `firstLines`
   truncation in `plan_cycle.go` with the chain-aware summarize call. Ships and is
   verifiable on its own, benefits the existing engine immediately.
4. **Blackboard.** `wavefront.Blackboard` — `Know`/`Get`/`Consider`/`Resolve`/`Render`,
   seeded from `branch.Facts()`, concurrency-tested for the in-flight `Consider` race
   specifically (two goroutines racing to claim the same normalized name). Behavior
   doc: `docs/architecture/behavior/adr/0012_blackboard_convergence.feature.md`.
5. **`Classifier` + `LLMClassifier`.** The KNOW/NEED contract, schema-constrained
   prompt, parse path (reusing `internal/jsonx` as backstop). Unit-testable with a
   stub `Classifier`, same posture as `decompose.Planner`'s test doubles.
6. **`wavefront.Scheduler`.** The round loop over `task.Graph` — leaf computation,
   concurrent dispatch bounded by `slots`, round-boundary merge, generalized
   `scheduler.ErrNoProgress` termination, `maxRounds`/`maxDepth` backstops, fallback
   synthesis call. This is the highest-risk, highest-value phase; behavior doc first:
   `docs/architecture/behavior/adr/0012_wavefront_scheduler.feature.md`, mirroring how
   ADR 0008 called out its own state machine as the priority contract to pin.
7. **Orchestrator wiring.** `wavefront_cycle.go`, the `WavefrontEnabled` branch in
   `runPlanPhase`, settings plumbing end to end.
8. **Eval harness.** A thin runner (Godog suite or `cmd/` tool) replaying a fixed goal
   corpus through both engines, logging outcome/round-or-node-count/latency —
   `decompose.DrainPlan` and `wavefront.Scheduler.Run`'s shared "no bus, no I/O"
   shape (§9) is what makes this cheap once phases 1-7 land, not before.

Every phase's touched functions need a GIVEN/WHEN/THEN behavior doc before
implementation, per repo invariant — phases 4 and 6 are the two contracts most worth
writing first, since they are where a bug would reintroduce either the hallucination
failure this ADR exists to close, or a new, wavefront-specific stall.

## Open Questions

1. **Dedicated wavefront synthesis call.** §"Open decision point" above — reuse
   `planContext`/the general respond call (recommended default) vs. an always-on
   schema-free final-answer call unique to wavefront. Evaluate with the Phase 8 eval
   harness rather than deciding now.
2. **`maxRounds` value.** Needs a real number, the same way ADR 0010 left the
   `abstained`-retry budget as product judgment rather than architecture. totAlX used
   25 against a much shallower, unconstrained-schema tree; agentX's schema-constrained
   classify calls likely converge faster per round, but this needs measurement against
   the eval harness, not a ported constant.
3. **Staggered round release.** A round need not be strict lockstep — a lane whose own
   dependency chain clears early could, in principle, start its next classify call
   without waiting for a slower sibling lane, recovering some of the continuous
   scheduler's efficiency without losing the grounding guarantee (each lane still only
   ever sees its own real, completed upstream results). Scoped out of this ADR
   deliberately — ship strict round-sync first, measure the actual cost, then decide
   if this is worth the added complexity.
4. **Retry-prompt shape for classify violations.** Mirrors ADR 0008's Open Question 7
   (never resolved as architecture, still an implementation detail): the specific
   violation (missing `name`, a `Need` with a command that isn't in the tool catalog)
   needs to be fed back into a regenerated prompt, not just a generic "invalid, retry."
5. **Does a `Need`'s command ever need multi-step shell syntax?** totAlX's own
   allowlist is regex-based and rejects shell control flow entirely (pipes,
   conditionals) — agentX's existing planner prompt already forbids this
   (`planner.go:64`, "no shell syntax in args"). Wavefront should inherit the same
   constraint by construction (the tool catalog it's shown has no shell-string
   parameter shape to begin with), but this should be confirmed, not assumed, once
   the catalog rendering is reused in Phase 5.

---

## Amendment (2026-07-17): Graph-as-Blackboard, Continuous Convergence, and Engine Interleaving as a Deliberately Open Future

**Supersedes:** §3 ("The blackboard — per-drain, live, with in-flight convergence")
and §4's round-synchronized dispatch model, in favor of a simpler mechanism that
needs neither a standalone data structure nor a wave boundary; §9's "a session runs
one or the other, never both on the same plan" framing, loosened to a stated,
deliberately deferred future direction rather than a structural limit. The
Consequences section's round-sync-vs-continuous-efficiency trade-off no longer
applies — there is no round to pay that cost for. Open Question 2 (`maxRounds`
value) is retired as moot. Original §1, §2, §5, §6, §7, §8 and the Phased Build Plan
steps 1-3 (already shipped) are unaffected and left as written below.

### Context for the amendment

Working through Phase 4's design before writing code surfaced that two things
originally scoped as necessary — a standalone `Blackboard` type with its own
`considering` map, and round-synchronized (bulk-synchronous) dispatch — were solving
problems a simpler mechanism already solves, once examined closely:

1. **Registration is consideration.** If a node's proposal only becomes real once
   it's registered against `task.Graph`, and every graph write already happens
   through the continuous scheduler's existing single-writer discipline ("the graph
   is mutated only on the main loop goroutine," `scheduler.go:14-17`), two branches
   concurrently proposing the same fact don't need a separate atomic `Consider()`
   primitive to resolve the race — whichever's merge step reaches the lock first
   creates the node; the second, moments later, finds it already there (an existence
   check by normalized name, the same shape `applyDecompose`'s existing
   `if _, exists := s.graph.Node(c.ID); exists { continue }` already has, just keyed
   differently) and links its own edge to it instead.
2. **Grounding never depended on round boundaries.** The rule that actually prevents
   hallucination — a classify call may only propose a command whose arguments are
   already true in its own snapshot at dispatch time — is a property of one call, not
   of scheduling cadence. Round-synchronization (wait for a whole wave before
   starting the next) was totAlX's mechanism for achieving this, not a requirement of
   the mechanism itself. Once separated out, wavefront's dispatch can be continuous —
   mirroring the existing scheduler's dispatch/channel/single-threaded-merge/
   dispatched-at-most-once loop exactly — without paying the concurrency-efficiency
   cost totAlX's own numbers already showed (`parallel(batch=1)` slower than
   `sequential(batch=1)`, purely from one straggler).

Separately, extending `task.Record` to carry a resolved node's value durably (rather
than a parallel blackboard structure) raised a question worth settling explicitly:
should the two engines populate that value differently? They shouldn't, and there is
no structural reason for them to. `task.Record` is already the one shared structure
of record for both; the two engines differ in dispatch and merge *logic* (Kind-XOR
dispatch vs. heterogeneous classify responses; no dedup needed vs. name-based
convergence), never in what a resolved node's value or failure reason *means*.
Treating them differently was hesitation about touching already-proven code in
`scheduler.go`, not a real design constraint — and that code has already absorbed a
comparably-sized change once before (ADR 0010's assertion-judgment insertion).

### Decision

**1. No standalone `Blackboard` type.** What was scoped as `wavefront.Blackboard`
(`facts`/`considering` maps, `Know`/`Get`/`Consider`/`Resolve`/`Render`) is retired
before being built. "Working memory" for a classify prompt is a pure rendering
function over `task.Graph.Nodes()`, filtered to `Status == Done`, formatted
`{Goal}: {Value}`. "Currently being considered" is the same query, filtered to
still-open nodes instead. Nothing is cached or duplicated outside the graph.

**2. `task.Record` gains `Value`, `Error`, and `Seq` — shared, engine-agnostic
fields:**

```go
// Value is the node's resolved fact or answer text, set once Status becomes Done.
// Makes the graph a complete, serializable record of what was learned — not just
// what was attempted — with no separate structure to keep in sync. Populated by
// whichever engine's processing actually produced a resolved value for this node
// (a Task's tool result today; a Step's synthesis-of-children once ADR 0010's judge
// phases supply one) — never engine-specific in meaning, only in which engine had
// something to write.
Value string `json:"value,omitempty"`

// Error is the failure reason, set once Status becomes Failed. Persisted alongside
// Value so a failed node's graph entry shows *why*, not just *that* — closing a
// real, pre-existing gap: today neither engine's Failed transitions carry the
// reason onto the durable record, only into ephemeral bookkeeping
// (executor.Outcome, discarded graph.Add/Update errors).
Error string `json:"error,omitempty"`

// Seq is the node's position in the graph's growth — a monotonic counter Graph.Add
// assigns at admission and Graph.Update never touches, independent of Deps depth or
// dispatch concurrency. Lets a serialized graph show its own growth over time, not
// just its final shape.
Seq int `json:"seq"`
```

`Graph.Add` assigns `Seq` (`rec.Seq = g.nextSeq; g.nextSeq++`) — never
caller-supplied, so ordering is authoritative by construction. `Graph.Update`
explicitly re-inherits the existing node's `Seq` before storing
(`rec.Seq = g.nodes[rec.ID].Seq`) — a status-transition update carrying a
zero-value `Record.Seq` must never silently reset a node's growth position back to
0. This needs its own regression test; it is exactly the kind of one-line omission
that would silently corrupt history months later.

**3. Convergence is a merge-time existence check, not a separate mechanism.**
Wavefront's merge step, on receiving a proposed Need with no command, normalizes its
name and scans currently-open graph nodes for a match (the same query as §1's
"considering" render) before deciding to `Add` a new node or wire an edge onto the
existing one. This runs single-threaded, at the same point graph mutation already
happens — no new lock, no new data structure.

**4. Wavefront's scheduler is continuous, not round-synchronized — a separate type,
mirroring `scheduler.Scheduler`'s skeleton, not a generalization of it.** Dispatch
whatever's `Ready()` and not yet classified, up to the slot budget; results return
over a channel whenever each call actually finishes; a single-threaded merge step
(per §3) applies each completion as it arrives. Kept as a genuinely separate type
from `scheduler.Scheduler` — copying the proven concurrency pattern deliberately
rather than generalizing the original to take a pluggable per-node handler — so a
wavefront regression cannot touch the continuous engine's already-proven dispatch
loop. The one place per-node logic is real, new work, not a simplification: a
classify response is heterogeneous (some Needs execute immediately, some spawn
children, a Know may self-resolve the node) in a way `Kind`-XOR dispatch never is.

**5. Termination needs no bespoke mechanism.** Depth cap × per-node children cap
bounds total node count, hence total dispatches; each node is classified at most
once (mirroring `ErrStalled`'s existing "dispatched at most once" guarantee); a node
resolved purely by convergence is not re-dispatched, and its parent's join either
completes when siblings resolve or the branch stalls the same way an over-depth Step
already fails to `Ask` today. `Settings.WavefrontMaxRounds` and Open Question 2 are
retired — there is no round to bound.

**6. §9's engine selection is a starting simplification, not a structural limit.**
The original text — "a session runs one or the other, never both on the same plan"
— describes the first cut this ADR's Phased Build Plan actually builds, not a
ceiling. See "Future direction" below.

### Continuous-engine refactor this creates — captured explicitly, scoped as its own phase

Wiring `Value`/`Error` onto every terminal transition is real work in `scheduler.go`,
not a byproduct of the schema change, and must not be left half-done:

- `Scheduler.execute` currently returns only a `task.Status`, discarding
  `executor.Outcome`'s result text and failure reason entirely once mapped. It needs
  to surface both so the caller can write them onto the record.
- `workDone` (`scheduler.go`'s internal struct) carries no error/value payload today
  — `doneError` is dispatched with nothing but `id`; the actual error from
  `s.decomposer.Decompose` is dropped on the floor in `s.work`'s `default` branch of
  the Kind switch, too. This needs a new field threaded through the struct and every
  site that constructs one.
- `applyDecompose`'s two failure branches (`graph.Add` and `graph.Update` returning
  an error) both currently do `s.setStatus(wd.id, task.Failed)` with the real error
  (`ErrDuplicateID`/`ErrDanglingDep`/`ErrCycle`/etc.) discarded immediately after
  being checked. Both need to carry that text onto the record.
- `setStatus` itself needs a variant (or an added parameter) that also writes
  `Value`/`Error` before calling `graph.Update`, not just `Status`.

None of this changes `Status` derivation, dispatch order, or any concurrency
guarantee — it is additive at the observable-behavior level, matching the discipline
every prior extension to this scheduler has followed. But it is a real, multi-site
touch to proven code, not a one-line change, and it deserves its own phase with full
regression coverage (`scheduler_test.go`, `guard_test.go`, `node_test.go` all passing
unchanged, plus new tests asserting `Value`/`Error` land correctly on every failure
path) before wavefront ever depends on the same discipline. Doing it here, first,
against the smaller and better-understood engine, is also the same "prove
infrastructure on the existing call site before wavefront needs it" sequencing Phase
1 already used for reasoning-budget plumbing.

### Future direction: conditional and interleaved engine selection (explicitly not scoped, explicitly not foreclosed)

There is a real possibility that the right unit of engine selection is neither "once
per session" nor even "once per plan," but something narrower and dynamic — routing
individual sub-problems to whichever engine suits their nature, and for a
sufficiently complex problem, alternating passes of each within a single plan's
lifetime (gather grounded facts, decompose efficiently over what's now solid, gather
again for what the decomposition surfaced as unknown, synthesize, repeat).

This ADR does not design that mechanism — a selection policy ("what makes a
sub-problem suited to which engine") and an explicit hand-off protocol are real,
unscoped work. But nothing in the design above forecloses it, and the
schema-unification decision in §2 is what makes it *feasible* rather than merely
imaginable:

- Because both engines read and write the same `task.Record`/`task.Graph`, a node
  one engine produced is immediately legible to the other's dispatch loop with no
  translation step. Interleaving two engines with divergent schemas would need a
  translation boundary every time control passed between them; unifying the schema
  removes that boundary entirely.
- `task.Record.Provenance.Source` (already populated — `decompose.go` already tags
  planner-produced nodes `"planner"`) already carries per-node engine attribution for
  free. A future router deciding "hand this branch to wavefront, that one to the
  continuous engine" needs exactly this signal, and it costs nothing new to have it —
  wavefront's future nodes need only tag `Source: "wavefront"` consistently.
- The continuous engine's `Decomposer` interface already accepts an inherited-facts
  snapshot at fork time (`branch.Fork`'s `Facts()`). A plausible concrete mechanism
  for "gather, then decompose" — not designed here, just noted as evidence the seam
  already exists — is wavefront grounding a Step's context first, then handing that
  same Step to the continuous engine's decomposer for efficient one-shot breakdown
  now that its inputs are verified, rather than either engine owning a Step
  exclusively end to end.

Recorded here so the Phased Build Plan below is read as a first, deliberately simple
cut — two complete, independently useful engines sharing one schema — not as a
decision that the two must stay permanently separate.

### Consequences (amendment)

Positive:

- Eliminates an entire data structure (`Blackboard`) and the synchronization surface
  between it and the graph before either was built.
- Removes the round-sync-vs-continuous-efficiency trade-off from the original
  Consequences section entirely, rather than accepting it — wavefront gets the
  continuous engine's dispatch efficiency and totAlX's grounding discipline, not a
  choice between them.
- `Value`/`Error`/`Seq` fix a real, pre-existing gap in the continuous engine's
  persisted plan output (a failed node's *reason* is not durable today), independent
  of wavefront ever shipping.
- Directly enables the future interleaving direction at no extra design cost now — a
  consequence of the schema-unification decision, not a separate investment.

Trade-offs:

- The continuous-engine refactor is real, multi-site work against already-proven
  code (`scheduler.go`), not free — mitigated by scoping it as its own phase with
  full regression coverage before anything depends on it.
- Two engines sharing one schema means a future interleaving/hand-off mechanism, if
  built, has to reason about a graph that may contain nodes from both engines' merge
  logic simultaneously — not a problem today (each plan-drain still selects one
  engine), but worth naming as complexity the "Future direction" section defers, not
  eliminates.

### Phased Build Plan (amendment) — supersedes original steps 4 onward

Steps 1-3 (reasoning-effort plumbing, seed-file prompt templating, output
summarization) are unchanged and already shipped.

4. **`task.Record`/`task.Graph` schema: `Value`, `Error`, `Seq`.** Additive fields
   only; `Graph.Add` assigns `Seq`, `Graph.Update` preserves it (dedicated regression
   test). No engine wiring yet — this phase only makes the fields exist and behave
   correctly in isolation. Behavior doc:
   `docs/architecture/behavior/adr/0012_record_value_error_seq.feature.md`.
5. **Wire `Value`/`Error` into the continuous engine.** The `scheduler.go` refactor
   named above — `workDone` gains an error/value payload, `setStatus`'s variant
   writes it, every `task.Failed`/relevant `task.Done` transition site threads real
   text through instead of discarding it. Full regression pass on existing scheduler
   tests plus new tests per failure path. Proves the population discipline on the
   smaller, better-understood engine before wavefront needs the same pattern.
   Behavior doc:
   `docs/architecture/behavior/adr/0012_scheduler_value_error_wiring.feature.md`.
6. **`Classifier` + `LLMClassifier`.** Unchanged in substance from the original step
   5 — the KNOW/NEED contract, schema-constrained prompt, parse path. **Implemented**
   — also corrected two Phase-2 prompt gaps found while building this phase's
   consumer (object-wrapped wire shape reusing `jsonx.FirstObject` unchanged; an
   explicit reply-format spec in the user template). Behavior doc:
   `docs/architecture/behavior/adr/0012_wavefront_classifier.feature.md`.
7a. **Extract output-summarization ownership into `wavefront`, out of `plan_cycle.go`.**
   Prep step, see the same-day addendum below for why this is necessary (not just
   tidy) before step 7: `internal/runtime` already imports
   `internal/runtime/wavefront` (for the Phase-2 prompt scaffolding, since Phase 3)
   — so `wavefront` cannot import `internal/runtime` back, ever, without a cycle.
   Phase 3's summarization mechanics (`outputSummaryThreshold`,
   `outputSummaryTargetChars`, the condense/truncate logic) move from
   `plan_cycle.go` into `wavefront` as `OutputSummaryThreshold`,
   `OutputSummaryTargetChars`, `TruncateFindings`, `CondenseFunc`, `NewCondenser` —
   co-located with the prompt templates they already render. `plan_cycle.go`'s
   `capturingExec`/`newOutputSummarizer` become thin callers. Behavior-preserving:
   Phase 3's existing tests move and must pass unchanged.
7. **`wavefront.Scheduler`.** Continuous dispatch mirroring `scheduler.Scheduler`'s
   skeleton (§4 above); merge-time convergence via normalized-name existence check
   (§3) for open-value Needs only (command-valued Needs execute unconditionally,
   per the same-day addendum's scope reduction); `Value`/`Error`/`Seq` population
   reusing the exact discipline step 5 proved out; per-node lifecycle per the
   same-day addendum's `classified`/`awaitingResolution` generalization of
   `scheduler.go`'s existing `dispatched`/`decomposed` pair. No round, no
   `maxRounds`. Highest-risk phase; behavior doc first:
   `docs/architecture/behavior/adr/0012_wavefront_scheduler.feature.md`.
8. **Orchestrator wiring.** `wavefront_cycle.go`, the `WavefrontEnabled` branch in
   `runPlanPhase`, settings plumbing end to end. (Unchanged from original step 7.)
9. **Eval harness.** Unchanged from original step 8.

### Open Questions (amendment)

Retired: Open Question 2 (`maxRounds` value) — no round exists to bound.

New:

7. **Engine-selection policy.** What makes a sub-problem "suited" to wavefront vs.
   the continuous engine, and what triggers a hand-off mid-plan? Explicitly unscoped
   (see "Future direction" above) — needs real usage data from both engines running
   independently first.
8. **Hand-off protocol.** If/when interleaving is pursued, does control pass at Step
   boundaries only (matching the continuous engine's existing `Decomposer` seam), or
   something finer-grained? Unscoped for the same reason as (7).

### Addendum (2026-07-17, same day): Phase 7 scheduler design — node lifecycle, scope reduction, summarization ownership

Worked out ahead of writing `wavefront.Scheduler`, in response to the question "what
in Phase 7 is genuinely risky, and is any of it dissolved by something already
built" — three findings, all resolved before code.

**1. A wavefront node's lifecycle looked like it needed three async stages; it
needs the same two `scheduler.go` already has.** A question can (a) get classified,
(b) if it spawned open Needs, wait on them exactly like a Step's join, and (c) once
those resolve, still need a self-match-or-synthesize check. That looked like a third
state the continuous engine never has. It isn't one: a node with *zero* spawned
Needs and no self-match is in exactly the same position as a node whose Needs have
*all* resolved — both are "`Ready()` again, not yet resolved, needs one more
check." So the state is the same pair `scheduler.go` already tracks
(`dispatched`/`decomposed`), renamed for wavefront (`classified`/
`awaitingResolution`) and generalized by exactly one change: instead of
`decomposed[id]` resolving for free once `Ready()` again, wavefront's join branch
resolves via a self-match check that's sometimes free (a Know already matches, no
call) and sometimes needs one bounded synthesis call (§4's fallback) — itself
tracked the same "at most once" way `classified` already is, so it cannot be
re-triggered. No new state machine; the existing one generalized by one optional
step.

**2. Command-valued Need convergence is deferred — it's a performance question, not
a correctness one.** The original §3/§4 design implied every Need, command-valued or
not, goes through the existence-check/convergence dance before acting. Re-examined
against actual stakes: two branches independently proposing the identical tool call
both succeed and produce the same value if both run — wasteful, never wrong. Only
open-value Need convergence is load-bearing for correctness (it's what keeps the
graph from filling with duplicate sub-trees for the same *unresolved* question). Per
"accuracy first, then performance," command-valued Needs execute directly and
unconditionally in this build — no pre-registration, no in-flight-execution
bookkeeping. Revisit as a real optimization once there's usage data showing
duplicate tool calls are common enough to matter (ties to Open Question 7's
eval-harness dependency).

**3. `wavefront` cannot import `internal/runtime` — already true today, not a
future Phase 8 concern — so Phase 3's summarization logic needs to move, not just
get reused.** Checked directly: `internal/runtime/plan_cycle.go` has imported
`internal/runtime/wavefront` since Phase 3 (for the Phase 2 prompt scaffolding). Go
forbids the reverse import regardless of when wavefront's own code would call it, so
`wavefront.Scheduler`'s command-execution path cannot call into
`plan_cycle.go`'s `outputSummaryThreshold`/`condense`/`truncateFindings` as they
stand — those are unreachable from `wavefront`, cycle or not. Since `Value`/`Error`
living directly on `task.Record` (Phase 4/5) also means wavefront never needed
`capturingExec`'s parallel bookkeeping in the first place — a resolved command's
result just writes onto its node's `Value` — the real fix is moving Phase 3's
summarization *mechanics* into `wavefront` itself, co-located with the prompt
templates they already render (which live there already), and having
`plan_cycle.go` become a thin caller instead of the owner. This is the one piece of
this addendum that touches already-shipped code — scoped as its own step (7a)
before step 7, mirroring how step 5's continuous-engine refactor was itself scoped
ahead of needing it, with Phase 3's existing tests required to pass unchanged after
the move.

## Amendment (2026-07-17): Surface Visibility — Chat Output, Context, Working Memory

**Status: Implemented.** A design for this surface work was drafted earlier the
same day, before Phases 7-9 (the wavefront scheduler, orchestrator wiring) existed
to build it against — that draft assumed the original round-synchronized,
standalone-`Blackboard` design the "Graph-as-Blackboard, Continuous Convergence"
amendment above superseded (rounds, a `🧠 blackboard` sub-widget, `wavefront_round`/
`wavefront_node` content types). This amendment replaces it with what the shipped,
continuous, graph-as-blackboard engine actually needed and what was actually built.

### The gap, found by reading the shipped engine against the shipped widget

`wavefront.Scheduler` (Phase 7) never called `scheduler.Observer.NodeDecomposed` —
only `NodeDispatched` and `NodeCompleted`. The output/context plan widget's nested-
box recursion (`internal/surfaces/output/plan.go`, ADR 0009 §9c) only ever descends
into a node's recorded `children`, and `children` is populated *exclusively* by the
`"decomposed"` event handler. The practical result, confirmed by reading the code
rather than assumed: **a wavefront plan rendered as its root box and nothing
else**, regardless of how many Know/Need nodes were dispatched, executed, and
resolved underneath it — the exact "no visibility into ToT progress" gap the
original design task set out to close, except worse than a UX gap: it was a
latent rendering bug in already-shipped code that happened to have no observable
symptom yet, because nothing had exercised the wavefront path through a live
surface until this pass looked for it.

Two further, smaller gaps, found the same way:

- `NodeCompleted(id, status)` never carried the `Value`/`Error` the "Graph-as-
  Blackboard" amendment had just added to `task.Record` (Phase 4) and wired into
  `setStatus` (Phase 5) — so even a node that *did* render had nothing to show for
  a Step's resolved fact (a Know has no tool call behind it at all; `Value` is its
  only content).
- Convergence (a Need's edge folding onto an already-existing node instead of
  creating a child, §3 above) is structurally a second parent referencing one
  child — exactly the cross-branch edge the plan widget's tree-shaped rendering
  (ADR 0009 amendment: "no cross-branch dependency edges are structurally
  possible") cannot express as a second nested box without either duplicating the
  node's content or breaking the recursion's tree assumption.

A fourth, pre-existing gap (not wavefront-specific, but sharpened by it):
`output.Model.SelectedToolEvent` explicitly excluded any plan-tagged tool_result
from Pin — "a plan-tagged tool_result folded into a Task node... is not pinnable."
Only the auto-generated `plan:<name>` rollup Fact (ADR 0010 §4) ever reached
Working Memory; no individual finding inside a plan did, for either engine.

### Decision

**1. No new content types.** `NodeDecomposed`'s existing wire shape (`TASK_NODE`
`"decomposed"`, `children: [{task_id, goal, deps, kind}]`) is reused as-is —
`applyClassify` (`internal/runtime/wavefront/merge.go`) now collects the *freshly
created* children from a classify response's Needs and reports them the same way
the continuous engine reports a real decomposition. This was simpler than the
original draft's two-new-`ContentType` proposal because the shipped schema
unification (§2 above: `Value`/`Error`/`Kind` all mean the same thing regardless
of which engine wrote them) means the existing `task_plan`/`task_node` wire shape
already fits wavefront's nodes exactly — Know/open-Need/command-Need are just
`KindStep`/`KindTask` records like any other, not a third shape needing a third
event type.

**2. `NodeCompleted` gains `value, errText string`.** `scheduler.Observer`'s
signature changes to `NodeCompleted(id string, status task.Status, value, errText
string)`, both engines' `setStatus` pass what they already write onto the record,
and `planObserver` folds them into the `task_node` `"completed"` payload and into
`session.PlanTreeNode` (which gains matching `Value`/`Error` fields). The output
widget's `planNode` gains the same two fields, and a Step with a resolved
value/error now renders it the same way a Task renders its result — a collapsible
box one level deeper (`🧩 value` / `⚠ error`), reusing the existing result-box
machinery (generalized to `drawTextBox`, parameterized by title/outcome/text
instead of reading `resultText`/`resultOutcome` directly).

**3. Convergence is a new optional `ConvergenceObserver` interface, not a
`NodeDecomposed` overload.** `scheduler.ConvergenceObserver` (`NodeConverged
(parentID string, existing task.Record)`) sits next to `Observer`, following Go's
standard optional-interface pattern (`io.ReaderFrom`) rather than growing the base
contract every consumer must implement — the continuous engine's decomposition is
strictly parent-as-join and never needs it. `wavefront.Scheduler` type-asserts its
own observer for it. `planObserver` implements it, publishing a `task_node`
`"converged"` event (`{task_id: <parent>, converges_onto: <existing>, goal:
<existing's goal>}`); the widget renders it as a one-line `↳ converges onto:
<goal>` annotation on the *converging* node, never a duplicate nested box — the
referenced node's own content stays exactly where its real (first) owner drew it,
preserving the tree-shaped recursion untouched. `session.PlanTreeNode` gains a
matching `ConvergesTo []string` for the persisted-tree counterpart.

**4. Plan-node pinning: a node-level cursor inside the plan widget, not a new
selection model.** Pin (`p`, PD-CTX-AF-012) already worked at the granularity of
one top-level widget; a plan is one widget with many nodes inside it, none of them
independently selectable. Rather than inventing a per-surface sub-widget selection
framework, the plan widget gained a minimal node-level cursor (`planState
.activeNode`, `output.Model.ActiveNodeNext`/`ActiveNodePrev` — `Tab`/`Shift+Tab` in
the context surface, free keys in both surfaces' existing keymaps) that only
moves, and only renders (the `›` prefix on the active node's title), while its
owning plan widget is the selected top-level widget. `SelectedPlanNode()` reports
what pinning the active node would do:

- A Task/command-Need node whose tagged `tool_result` has arrived already carries
  a real event ordinal (captured into `planNode.resultOrdinal`, previously
  discarded) — it reuses the **existing** `PinToolEvent(ordinal, live)` path
  unchanged. Checking the actual server-side contract confirmed this needed no
  server change at all: `PinToolEvent` only ever required `ContentType ==
  ContentToolResult`, and a task-tagged tool_result already satisfies that — the
  old exclusion comment described a client-side selection gap, not a real server
  restriction.
- A Step/Know node has no tool call behind it at all — no ordinal exists to pin
  by. A new path, `PinPlanNode(root, nodeID)`, reads the node's current
  `goal`/`value` from the durable plan-tree registry (the same authoritative
  server-side source `PinToolEvent` already reads via `o.History()`, not
  whatever text a client last rendered) and constructs a `session.Fact` directly.
  New transport: `POST /plans/{root}/nodes/{node}/pin`, `Client.PinPlanNode`,
  `Provider.PinPlanNode`.

**5. A value-sourced pin can never go live — enforced by construction, not a new
check.** `PinPlanNode`'s `session.Fact` sets no `Source` (there is no tool to
re-run). The existing live-toggle refusal (`SetFactLive`: "fact is not pinned to a
tool source" when `Source == nil`) and the working-memory surface's existing
client-side guard (`case "l": if f.Source != nil { ... }`, PD-WM-AF-009's pattern)
both already gate on exactly this — no new gating logic was needed, only a test
locking in that the existing gate actually covers this new fact shape.

### Architecture — insertion points (as built)

- `internal/runtime/scheduler/scheduler.go` — `Observer.NodeCompleted` gains
  `value, errText string`; new `ConvergenceObserver` interface.
- `internal/runtime/wavefront/merge.go` — `applyClassify`/
  `registerOrConvergeNeed` report fresh children via `NodeDecomposed` and
  converged edges via `NodeConverged` (type-asserted).
- `internal/runtime/wavefront/scheduler.go`, `internal/runtime/scheduler
  /scheduler.go` — both `setStatus` implementations pass `value, errText` to
  `NodeCompleted`.
- `internal/runtime/plan_cycle.go` — `planObserver.NodeCompleted` folds
  value/errText into the event payload; new `planObserver.NodeConverged`.
- `internal/runtime/plan_tree.go` — `completed` gains `value, errText`; new
  `converged` and `node` (read accessor for `PinPlanNode`) methods.
- `internal/session/plans.go` — `PlanTreeNode` gains `Value`, `Error`,
  `ConvergesTo`.
- `internal/runtime/orchestrator.go` — new `PinPlanNode`, `pinNodeFactKey`.
- `internal/transport/http/{server,context,client}.go` — new
  `POST /plans/{root}/nodes/{node}/pin` route + `Provider`/`Client` methods.
- `internal/surfaces/output/{output,plan}.go` — `planNode` gains
  `value`/`errText`/`convergesTo`/`resultOrdinal`; `drawResultBox` generalized to
  `drawTextBox` (shared by Task results and Step values); new `SelectedPlanNode`,
  `ActiveNodeNext`/`ActiveNodePrev`, `selectedPlanState`; `nodeTitle` gains the
  `›` cursor prefix.
- `internal/surfaces/context/context.go` — `Key` gains `tab`/`shift+tab`;
  `pinSelected` tries `SelectedPlanNode` before the flat-tool-result path.
- Behavior: `docs/architecture/behavior/adr/0012_surface_visibility.feature.md`.

### Consequences (amendment)

Positive:

- Fixes a real rendering bug in already-shipped code (wavefront nodes never
  nested), not just an enhancement — found by reading the engine and widget
  together rather than assumed from the original design pass.
- The pin-exclusion fix benefits the continuous engine equally; it was never
  wavefront-specific, only surfaced as urgent by wavefront's per-node grounding
  being the whole point of the engine.
- No new content types, no new widget kind, no new selection framework — every
  piece reuses an existing mechanism (`NodeDecomposed`'s wire shape, `PinToolEvent`
  and its exact server contract, PD-WM-AF-009's existing live-gate), consistent
  with this ADR's own repeated preference for reuse over parallel machinery.

Trade-offs:

- `ConvergenceObserver` is one more optional interface a future `Observer`
  implementer needs to know might exist, even though the base contract didn't
  change — a small ongoing discoverability cost, accepted for not forcing every
  `Observer` (including test stubs that will never see wavefront) to implement a
  method they can't produce a meaningful value for.
- The node-level pin cursor is a second, independent selection concept living
  inside one top-level widget selection — a real (if small) new mental model for
  a user navigating a plan, not free.

### Open Questions (amendment, continued)

9. **Node cursor order.** `ActiveNodeNext`/`Prev` walk `planState.order` (flat,
   depth-agnostic — simplest to implement first). A depth-first walk matching the
   widget's own visual nesting may be more intuitive once there is real usage to
   evaluate against; deferred, not decided here.
10. **Owner-side convergence annotation.** Same open question the original design
    pass raised: whether the *converged-onto* node should also show "N other
    lanes reference this," not just the converging side. Still deferred to real
    transcripts, not decided here.

### Addendum (2026-07-17, same day): the "🌊" provenance tag

The original design pass's chat widget draft (before the round-synchronized
design was superseded) proposed a "🌊" glyph as the whole wavefront widget's
title. That widget never got built — see the amendment above — but the
underlying signal it was reaching for (which engine produced a given node) was
real and already free: `task.Record.Provenance.Source` is `"wavefront"` for
every node wavefront's merge step creates (`internal/runtime/wavefront/merge.go`,
`wavefront_cycle.go`'s root), `"planner"` for the continuous engine's planner-
produced children, and empty for the continuous engine's own root — it just
never reached the wire. `NodeDispatched`/`NodeDecomposed`'s payloads and the
`task_plan` "started"/"ended" snapshots (`planSummary`, shared by both engines)
now include `"source"`; the widget's `planNode` gains a matching field and
`nodeTitle` prepends a `"🌊 "` tag when it reads `"wavefront"`. Deliberately
per-node rather than reviving a plan-level title marker: it is the signal that
stays meaningful if the "Future direction" section's interleaved/mixed-
provenance plans are ever built, at no cost while every plan still selects one
engine exclusively. Test: `internal/surfaces/output/plan_test.go`'s
`TestWavefrontSourceTag`.
