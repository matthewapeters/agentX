# Cascade Classifier — Intent Classification and Tool-Call Routing (Family A)

Last updated: 2026-07-04
Status: Design draft (pre-implementation)
Owner: Runtime / Orchestration

## Why this doc exists

A live in-app session (`vivid-willow`, local `ornith:latest`) was asked to create
a markdown file. The model generated the entire file's content — then wrapped it in
a fenced `cat > file << 'EOF' … "Created …"` block and **never called the write
tool**. The artifact existed only as chat text; nothing hit disk. The single tool
call in the whole session was one `list_dir`.

The root cause is not "small model is bad at tools." It is an architecture that asks
**one overloaded generative pass to converse, classify intent, and select tools at
the same time**. As conversational context grows, the salience of a simple imperative
("write the file") decays against thousands of tokens of discussion, and the turn is
never recognized as actionable. The model didn't refuse to act — it never classified
the turn as an action.

This doc specifies the fix: **intent classification as a first-class, context-isolated
stage** that (a) runs a cheap coarse gate on almost every turn, (b) escalates to a
bounded self-consistency vote only when uncertain, (c) reconciles what the user asked
against what the model produced, and (d) emits a durable task record that a separate
executor drains. It is built on the fan-out primitive (`internal/llm/fanout`).

Family A only. This is the server's request-handling brain today; it is the seam that
later grows into the Family-B DAG orchestrator (§9).

## Principles

1. **Context isolation.** The classifier does not see the full transcript. It sees the
   latest turn (and a compact open-task digest), and answers a narrow question. Small
   models are reliable only when the question is narrow and the context is small.
2. **Externalize the task.** A recognized action becomes a durable `task_proposed`
   record in the event log — the generative model is never responsible for holding an
   action in its head across a long response.
3. **Converse ≠ execute.** Generation produces prose; a separate pass performs the
   tool call from the task record. A small model nails "here is a task: write X with
   content Y — emit `write_file`" in isolation and drowns on it inline.
4. **Accuracy over latency, but know when unsure.** A wrong classification of a write
   or command costs the user 2–3 redo cycles (or worse, a destructive action). Below a
   confidence floor the system **abstains and asks a one-line question** rather than
   guessing.
5. **Bounded fan-out.** Aggregate model throughput is flat past the server's
   parallel-slot count (see the fan-out concurrency spike). So width is bounded to
   ≈slots; accuracy is bought with *better* votes (constrained decoding, prompt
   engineering, quorum voting) and a **cascade**, not with wider N. AgentX runs one
   model — there is no stronger-model-per-vote lever.

## Pipeline

```
user turn
   │
   ├──▶ [A] CONVERSE          full context → generative model → prose response
   │
   ├──▶ [B] CLASSIFY (turn)   minimal context (this turn + open-task digest)
   │        cascade ▼         → { actionable?, task_type, goal, params, confidence }
   │
   ├──▶ [C] CLASSIFY (resp)   the model's own response
   │        cascade ▼         → { produced_action?, executed?, artifact, confidence }
   │
   ├──▶ [D] RECONCILE         fold [B]+[C] → route (§6)
   │
   └──▶ [E] EXECUTE + VERIFY  drain task record → tool call → confirm effect (§7)
```

[A] is unchanged. [B] and [C] are each a **cascade** (§4). [D] reconciles the two
signals into a routing decision. [E] performs and *verifies* the action.

A **stage 0 (relatedness triage)** runs *before* [B]/[C] and decides what context they
may see — closing the "who decides what's relevant?" question left open by the
minimal-context rule above. It is specified in
[`prompt_fan_groups.md`](prompt_fan_groups.md), which also defines the prompt corpus
(fan-groups) that all of [0]/[B]/[C] draw their prompts from.

## The cascade (applies to both [B] and [C])

Each classifier is a three-tier cascade so the common turn stays at R=1 (≈0.77s on the
measured box) and only ambiguous turns pay the vote tax.

| Tier | What runs | Cost | Emits |
|------|-----------|------|-------|
| **0 — prior** | Deterministic imperative pre-filter (regex/keywords: "create", "write", "run", "generate a file", …) | ~0 | a high-recall bias signal, not a decision |
| **1 — coarse gate** | One constrained-output classify invocation | **R=1** | `{actionable, task_type, confidence}` |
| **2 — bounded vote** | Fan-out self-consistency vote via `fanout.Pool` | R≈slots | measured verdict + agreement confidence, or **abstain** |

**Escalation trigger (Tier 1 → Tier 2).** Escalate when *any* of:
- Tier-1 self-reported `confidence` is below the escalation floor, **or**
- Tier-1 verdict disagrees with the Tier-0 prior (prior says imperative, gate says
  chat — or vice versa), **or**
- the task type is a **high-stakes** kind (file write, command exec) — those always
  escalate, because a false positive is destructive.

**Tier 2 semantics.** The vote is `N ≈ slots` invocations (diversity via jittered
temperature/seed/prompt template) folded by `fanout.MajorityVote` with quorum
`⌈N/2⌉+1` and an abstain threshold. Quorum early-exit cancels stragglers the moment a
verdict lands — which, past the knee, is exactly the queued tail. The vote's *agreement
share* is a measured confidence (self-consistency), stronger than any single model's
self-report. If the vote scatters below threshold → **abstain**.

**Abstention → ask, don't guess.** An abstained classification routes to a one-line
clarifying question ("Did you want me to write that to a file?"), never to a silent
best-guess action. This is the accuracy-first stance made mechanical.

## Reconciliation ([D])

The turn cascade answers *"did the user request an action?"*; the response cascade
answers *"did the model produce or execute one?"* Neither decides alone:

| Turn | Response | Meaning | Route |
|------|----------|---------|-------|
| action | executed | model did it | verify effect, done |
| **action** | **produced, not executed** | **the `vivid-willow` case** | **reify the artifact from the response → real tool call** |
| action | nothing | model dropped the action | re-dispatch the task record to the executor |
| no action | action | model volunteered an action | gate via command policy / confirm |
| no action | nothing | pure conversation | no task |
| *any abstain* | *any* | uncertain | ask one clarifying question |

The response cascade is a **verifier/critic on the generator** (the reflexion pattern):
it catches "the model committed to an action then didn't take it," which the
single-pass design could not.

**[C] runs always-on** — on every turn, independent of whether [B] found an action.
Accuracy-first: it must catch *volunteered* actions (the model does or produces
something the user never explicitly asked for, as in `vivid-willow`), which a
[B]-gated [C] would miss. The cost is a second classify cascade per turn, but [C]'s
own Tier 1 keeps the common case at R=1 — a pure-conversation response confidently
classifies as "no action" and never escalates.

## Output contracts (per classifier tier)

Every classify invocation carries a `fanout.Contract` so results are comparable
(votable) and bounded. Constrained/JSON-schema decoding enforces structure at
generation; the contract quarantines anything that slips through — a malformed vote
never poisons the fold, and if too few conform to reach quorum the vote abstains.

- **Structure**: required fields (`verdict`, `confidence`, `task_type`) — `RequireField`.
- **Length**: bounded rationale (e.g. ≤ 60 words) — `MaxWords`, so a vote can't run away.
- **Cardinality**: for extraction/decomposition, ≤ N params/milestones — `MaxMilestones`.

## Task record ([B] output, [E] input)

A recognized action is emitted as a `task_proposed` event, decoupled from the turn:

```
task_proposed { id, goal, type: artifact|command|query, params, status, deps[], provenance }
```

It is **DAG-node-shaped from day one** (`deps[]`, `provenance`) so today's "extract one
task, execute it" grows into Family B's "extract a plan DAG, orchestrate it" with no
redesign. The full schema is specified separately (task-record schema doc, TBD); this
doc only fixes that the classifier's output *is* a durable record, not an in-context
intention. Provenance carries the vote spread from Tier 2 so a classification is later
answerable from the event log.

## Execute + verify ([E])

The executor drains the task record with a tight, tool-only prompt ("here is a task —
emit the tool call"), gated by the existing command-policy layer (autonomous vs.
confirm per tool). Then it **verifies the effect** before the surface reports success:
stat the file, check the command's exit and output. The response never says "Created X"
unless X demonstrably exists. This permanently retires the phantom-success class that
started this whole thread.

## Latency and cost

Total token throughput is fixed (~23–34 tok/s), so the cascade is the primary
efficiency lever — pay the vote tax only on the ambiguous minority.

| Path | Requests | Approx wall | When |
|------|----------|-------------|------|
| Coarse gate only | R=1 | ~0.77s | confident, low-stakes turns (the common case) |
| Escalated vote | R≈slots (4–5) | ~2.6s (≈3× baseline at the knee) | uncertain or high-stakes turns |
| Abstain → ask | R=1 + user turn | — | vote scattered; cheaper than a wrong action |

Escalating on *every* turn would pay ~3× latency for zero throughput gain — the whole
point of Tier 1 is to keep that off the hot path.

## Failure modes and mitigations

- **False negative (missed action).** Tier-0 imperative prior biases recall; the
  response cascade [C] is a second net (model produced an artifact → reify it).
- **False positive (phantom action).** High-stakes types always escalate to a vote;
  the command-policy gate can require confirmation; verify-effect bounds the blast.
- **Vote runaway.** `fanout` width budget (≈2× slots) refuses pathologically wide
  batches — the same guard that contains the 5.7GB-style blowups seen in testing.
- **Classifier context creep.** Enforce the minimal-context rule; if the open-task
  digest grows, summarize it — never fall back to feeding the full transcript.

## Mapping to code

- **Built:** `internal/llm/fanout` — the fan-out primitive (Pool, MajorityVote with
  quorum early-exit + abstention, Contract quarantine, `WithServerDefaults`).
- **Refit:** `internal/classify` + the sequential classify hook in the orchestrator
  become Tier 1/2 of the turn cascade over `fanout.Pool`, with a minimal-context prompt.
- **New:** Tier-0 imperative prior; the response cascade [C]; the reconciler [D]; the
  task-record emitter; the executor + verify-effect [E].

Every touched/new function gets a GIVEN/WHEN/THEN Gherkin feature before implementation
(`tests/features/…`), per the project's doc-first invariant.

## Deferred / open questions

- Variant-generation policy for Tier 2 (temperature/seed/prompt-template jitter) —
  how much diversity yields a meaningful agreement signal on one model.
- Escalation-floor and abstain-threshold calibration (empirical).
- The task-record schema (its own doc) — the Family-A/B seam.
