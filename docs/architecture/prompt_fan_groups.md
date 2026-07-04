# Prompt Fan-Groups, the Corpus, and Relatedness Triage (Family A)

Last updated: 2026-07-04
Status: Design draft (pre-implementation)
Owner: Runtime / Orchestration

## Why this exists

AgentX runs **one model**, so prompts — not model selection — are the accuracy
lever ([[single-model-prompt-engineering]]). That makes the prompt set a
**first-class, user-exposed, versioned asset**, not buried literals. This doc
specifies:

1. **Relatedness triage** — a new pipeline **stage 0** that decides how an incoming
   turn relates to the session, and therefore what context the downstream
   classifiers may see. This closes the "who decides what context is relevant?" hole
   in [`cascade_classifier.md`](cascade_classifier.md).
2. **Fan-groups** — the organizing unit of the prompt corpus: the set of prompt
   variants (plus one shared output contract) that vote together on one question.
3. **The corpus** — a machine-readable `prompts.toml` (source of truth) plus a
   human-readable `PROMPTS.md` decision-tree readme (edification + transparency),
   both seeded to `~/.config/agentx/` via `make seed`.

It builds on the fan-out primitive (`internal/llm/fanout`).

## Pipeline, with triage inserted

Stage 0 runs before the turn/response classifiers and *produces the context they
consume* — it is not optional context-trimming bolted on, it is what decides context.

```
user turn
   │
   ├─▶ [0] RELATEDNESS TRIAGE   session digest + turn → { relation, context directive }
   │        (fan-group)          continuation | new | orthogonal | related_aside
   │                                     │ selects the context frame ▼
   ├─▶ [B] CLASSIFY (turn)   ─── minimal context per the triage directive ───┐
   ├─▶ [C] CLASSIFY (resp)   ─── (always-on; verifier/critic) ───────────────┤
   ├─▶ [D] RECONCILE ────────────────────────────────────────────────────────┘
   └─▶ [E] EXECUTE + VERIFY
```

[A] CONVERSE (unchanged, full context) runs in parallel; [B]–[E] are as specified in
`cascade_classifier.md`, now parameterized by the triage directive.

## Stage 0 — relatedness triage

**Question:** how does this turn relate to the session, and what context do I carry?

**Verdicts** (not binary):

| relation | meaning | context directive |
|----------|---------|-------------------|
| `continuation` | extends the thread (pronouns, follow-up) | carry thread + task context |
| `new` | fresh, unrelated intent | drop thread context (may suggest a new session — a UX consumer, not triage's job) |
| `orthogonal` | a side-quest | carry domain context, not task context |
| `related_aside` | *looks* new but is a lateral move in the same goal | carry thread context, cautiously |

**Design stances:**

- **Conservative about discarding context.** Wrongly dropping relevant context is
  catastrophic (garbage classification); carrying a little extra is cheap. Bias toward
  keeping context when unsure; make the confident `new`/`orthogonal` call only at high
  confidence.
- **The hard case (`related_aside`) is why triage fans out.** A single cheap gate will
  *confidently* mis-call "looks-new-but-isn't." Multiple framings disagree on genuinely
  ambiguous input, and that disagreement is the signal to carry context cautiously or
  ask. So triage is itself a cascade (coarse gate → vote on ambiguity).
- **Bootstrapping + graceful cold-start.** Triage input is a compact **session digest**
  (topic + open tasks + recent turns), never the full transcript; its **output is the
  context-assembly directive** the rest of the pipeline consumes. On a brand-new
  session the digest is empty, triage returns `new`, and downstream runs context-free.

## Fan-groups

A **fan-group** is the set of prompt variants — plus one shared output contract — that
fan out and vote on a single question. It is the builder's unit of work and the
corpus's organizing unit. Each group binds to a pipeline stage.

**The load-bearing invariant: vary the *ask*, fix the *answer schema*.** Every variant
in a group targets the *same* output contract, or the votes aren't comparable and the
fold is theater. Variants differ only in how they ask, along three axes:

1. `param` — same template, jittered temperature/seed (cheapest diversity).
2. `template` — paraphrased instruction (catches prompt-brittleness).
3. `context_reframe` — same question, different framing/emphasis of the context
   (strongest signal, sharpest edge — hold the question and schema fixed).

A group runs as a cascade: its `coarse_variant` is the Tier-1 gate (R=1); on escalation
the full variant list is the Tier-2 vote (R = `width` ≈ slot count), folded by
`fanout.MajorityVote` with the group's `quorum` and `abstain_below`.

## The corpus — `prompts.toml` (machine-readable source of truth)

```toml
# ==========================================================================
# AgentX prompt corpus — fan-groups. Machine-readable source of truth.
# Human-facing walk-through: PROMPTS.md (the decision-tree readme).
# Edit with care: every variant must still satisfy its group's
# [output_contract]. A variant that fails validation on load falls back to the
# shipped default and logs a warning — it will not silently misclassify.
# Placeholders: {{turn}} {{session_digest}} {{open_tasks}} {{context}}
# ({{context}} is filled per the relatedness-triage directive.)
# ==========================================================================

[fangroup.relatedness_triage]
stage         = "triage"
purpose       = "Decide how the incoming turn relates to the session and what context to carry."
width         = 4          # Tier-2 vote width (≈ slot count)
coarse_variant = "direct"  # Tier-1 gate
quorum        = 3          # early-exit at ⌈N/2⌉+1 agreeing
abstain_below = 0.6        # scattered vote → abstain → ask

  [fangroup.relatedness_triage.output_contract]
  require        = ["relation", "confidence"]
  enum.relation  = ["continuation", "new", "orthogonal", "related_aside"]
  max_words      = 40      # bounded rationale, keeps a vote from running away

  [[fangroup.relatedness_triage.variant]]
  id    = "direct"
  axis  = "template"
  temperature = 0.2
  template = """
  Classify how a NEW message relates to an ongoing session.
  Session digest:
  {{session_digest}}
  New message:
  {{turn}}
  Reply JSON {relation, confidence, why}. relation is one of
  continuation | new | orthogonal | related_aside. why <= 40 words.
  """

  [[fangroup.relatedness_triage.variant]]
  id    = "reframed"
  axis  = "context_reframe"
  temperature = 0.6
  template = """
  A user just sent a message. Decide whether it CONTINUES what they were
  doing, starts something NEW, is an ORTHOGONAL aside, or looks new but is
  really a RELATED_ASIDE to the same goal.
  What they were doing:
  {{session_digest}}
  Message:
  {{turn}}
  Reply JSON {relation, confidence, why}.
  """

[fangroup.action_classify]
stage         = "classify_turn"
purpose       = "Decide whether the turn requests an action, and of what type."
width         = 5
coarse_variant = "direct"
quorum        = 3
abstain_below = 0.6
always_escalate_types = ["artifact", "command"]  # high-stakes never rides on the gate alone

  [fangroup.action_classify.output_contract]
  require          = ["actionable", "task_type", "confidence"]
  enum.task_type   = ["artifact", "command", "query", "none"]
  max_words        = 40

  [[fangroup.action_classify.variant]]
  id    = "direct"
  axis  = "template"
  temperature = 0.2
  template = """
  Does this message ask the assistant to DO something (produce a file,
  run a command, retrieve an answer) or just to converse?
  Context: {{context}}
  Message: {{turn}}
  Reply JSON {actionable, task_type, confidence, why}. task_type is one of
  artifact | command | query | none.
  """

  # ... further variants (paraphrase, context_reframe, param-jitter) ...
```

**Contract mapping.** A group's `[output_contract]` compiles to a `fanout.Contract`:
`require` → `RequireField`, `max_words` → `MaxWords`, list caps → `MaxMilestones`. The
`enum.*` verdict-domain check is a classifier-layer validation (a candidate `Contract`
extension). The *same* contract also drives **constrained decoding** on the request
side (Ollama `format`/grammar) — one schema, two uses: it tells the model what shape to
emit and quarantines anything that slips through.

## The decision-tree readme — `PROMPTS.md` (human-readable)

Primarily for the human: edification, safe revision, transparency. Sections:

1. **Overview** — one model, prompts are the lever, how to edit safely (user copy
   wins, validation + fallback).
2. **The decision tree** — the cascade flow as a **generated graph** (fan-group nodes,
   escalation/handoff edges: triage → action-classify → escalate-to-vote →
   reconcile → execute). Generated from `prompts.toml` so it cannot drift.
3. **Per fan-group** — hand-authored prose: what it decides, its verdicts, when it
   escalates, what its output contract requires.
4. **What you changed** — which prompts differ from the shipped default (transparency
   cuts both ways).

## Load + validation rules

- **Seed + user-wins.** Ship defaults in `config/seed/prompts.toml` +
  `config/seed/PROMPTS.md`; `make seed` copies them to `~/.config/agentx/` without
  clobbering an existing (user-tuned) copy.
- **Validate on load, fall back with a warning.** Each variant is checked against its
  group's output contract at load. A malformed variant is dropped back to the shipped
  default and logged — never a silent degrade ("know when it's broken").
- **Readme ↔ corpus sync.** Every fan-group named in `PROMPTS.md` must exist in
  `prompts.toml` and vice versa; the graph section is generated, so it stays linked.
- **A group needs ≥ its `quorum` valid variants**, else it cannot vote and the stage
  abstains (routes to a clarifying question).

## Wiring to code

- **Built:** `internal/llm/fanout` — Pool, MajorityVote (quorum/abstain), Contract.
- **New — the builder** (`internal/prompting`): loads + validates the corpus, and for a
  given fan-group + triage directive produces the `[]fanout.Invocation` (rendered
  templates, per-variant params, the shared contract) to hand to `Pool.Fold`. It also
  compiles a group's contract into the Ollama `format` schema.
- **New — triage stage** as fan-group `relatedness_triage`, feeding the context
  directive into `action_classify` / the response classifier.
- `Invocation.Prompt` likely graduates from a flat string to rendered messages plus a
  contract-derived format.

Every touched/new function gets a GIVEN/WHEN/THEN feature before implementation.

## Deferred / open questions

- Contract `enum` verdict-domain: classifier-layer check vs. a `fanout.Contract`
  extension.
- Session-digest construction (what/how much) — its own concern; triage consumes it.
- How the `context_reframe` axis renders differing framings without changing the
  question (template authoring guidance).
- Whether `PROMPTS.md`'s graph is regenerated on `make seed`, on load, or by a tool.
- Session-boundary UX when triage returns `new` (suggest/auto a fresh session).
