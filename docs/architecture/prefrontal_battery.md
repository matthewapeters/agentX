# The Pre-Frontal Battery — coordinate probes, cross-check fusion, decisiveness (Family A)

Last updated: 2026-07-05
Status: Design draft (pre-implementation)
Owner: Runtime / Orchestration

## Why this exists

The classifier's job is not to *answer* — it is to hand the "dumb" programmatic
harness a small, structured **pre-response** that says *what kind of thing this text
is*, so the harness switches into the right processing mode before it does anything
fuzzy. It is easier to parse a job request when you are already prepared to receive
one; the battery is how a fuzzy model is made to emit **crisp, machine-readable bits
up front**. This is the agent's pre-frontal cortex: decide *how* to engage before
engaging.

This doc consolidates the model that evolved past
[`cascade_classifier.md`](cascade_classifier.md) and
[`prompt_fan_groups.md`](prompt_fan_groups.md): the battery of coordinate probes, the
cross-check fusion that reads them, the subsystems each axis triggers, and the
configurable flow-vs-ask policy.

## The battery: three overlapping coordinate systems

The text's true nature is a hidden variable. Each probe is a cheap, noisy measurement
of one 3-axis slice of it. We ask three probes whose axes **overlap on purpose**, so
the overlaps become cross-checks. Axes are graded (a position on a range), not binary.

**System A — what kind of job**
- **Transactional ↔ Informational** — is there an action, or just talk/answer?
- **Atomic ↔ Composite** — one unit, or many parts?
- **Explicit ↔ Implicit** — stated outright, or leaning on unstated context?

**System B — risk / shape / dependency**
- **Constructive ↔ Destructive** — reversibility and stakes.
- **Simplicity ↔ Complexity** — effort and depth.
- **Requires-context ↔ Self-evident** — dependency on recall.

**System C — intent-grammar / done-condition / scope**
- **Imperative ↔ Interrogative** — expects me to *act* or to *speak*?
- **Bounded ↔ Open-ended** — is there a clear done-condition?
- **Local ↔ Global** — blast radius: one thing, the project, or the world?

## Triangulation: which axis cross-checks which

The whole point of overlapping systems is that every routing-critical axis is measured
by two or more differently-framed questions. Agreement across framings is far stronger
evidence than the same prompt voted N ways.

| Routing axis | Read by | Cross-checked by |
|---|---|---|
| Transactional (act vs. answer) | A | C: Imperative↔Interrogative |
| Atomic ↔ Composite (plan?) | A | B: Simplicity↔Complexity **and** C: Bounded↔Open-ended |
| Explicit ↔ Implicit (recall?) | A | B: Requires-context↔Self-evident |
| Destructive / stakes | B | C: Local↔Global (blast radius) |

Cost note: three probes × 1 invocation ≈ one question voted 3×, so the whole battery
fits in one fan-out wave within the Ollama slot budget — but yields ~9 axis readings
*with* built-in cross-checks instead of 3 readings of one thing.

## Each axis is a subsystem trigger

The dials do not just label the text — each turns on part of the machine. The harness
routes by region:

| Axis at its "high" end | Engages |
|---|---|
| **Transactional** | the **executor** (act) rather than answer |
| **Composite** | the **planner / DAG orchestrator** (Family B) rather than a single tool |
| **Implicit** | the **memory / recall** subsystem rather than proceed-now |
| **Destructive / Global** | the **confirmation gate** (see decisiveness) |

So Atomic↔Composite is literally the **Family-A/Family-B switch**, and Implicit is the
trigger that gates the (expensive) retrieval subsystem — you only pay for recall when
the turn actually leans on unstated context.

This shapes the emitted **task record** ([`task_record.md`](task_record.md)) directly:
composite → a task with `deps[]` (a plan); implicit → a resolve/recall step before
`ready`; destructive/global → requires confirmation.

## Cross-check fusion (the aggregator)

The battery replaces `MajorityVote` with a **cross-check fusion** aggregator. Given the
axis readings:

1. **Per-axis confidence** = tightness of the readings for that axis (spread across the
   probes that measure it) and distance from the axis midpoint. **Mid-axis = ambiguous
   = an ask signal**, for free — no separate "clarity" probe needed.
2. **Agreement on a correlated pair** → corroboration; route by the consensus position.
3. **Disagreement on a correlated pair is signal, not just error.** Correlation ≠
   identity: the off-diagonal corners are real —
   - **Atomic + Complex** ("prove the Riemann hypothesis") — one call, deep reasoning.
   - **Composite + Simple** ("lowercase these 5 filenames") — decompose, don't overthink.

   So the fusion must **not average** (that erases the corners). Strong agreement gives
   a confident consensus; a disagreeing pair means *either* an off-diagonal special case
   *or* genuine model confusion — and both route to the same safe place (look closer /
   ask), so the harness is safe even when it cannot tell which.

This generalizes `abstain_below` from "how scattered can one vote be" up to "how much
cross-check disagreement before we ask."

## Decisiveness: the flow-vs-ask policy

How much wobble the harness tolerates before asking is the agent's temperament, and it
is configurable.

- **User-facing dial:** `[classifier] decisiveness` in `agentx.toml`, a single number
  (0 = ask readily / cautious; 1 = proceed on strong agreement / decisive). This is the
  one human-legible knob.
- **It scales the *soft* axes only.** Relation, atomic/composite, speech-act, etc.
  loosen or tighten with the dial.
- **Hard safety floor — non-negotiable.** **Destructive** and **Global/irreversible**
  axes always confirm on doubt, *regardless of the dial.* `decisiveness = 1.0` must not
  be able to fire an irreversible, wide-blast action on a shaky read. The dial tunes
  flow; it never tunes away safety.

Which axes are hard vs. soft is a structural property of the axis definitions (with the
fusion), not a user tuning knob — the user gets the single global dial; the safety floor
is fixed.

## Relationship to the cascade and the fan-group model

- **Coarse gate (Tier 1):** run the whole battery once — three probes, one wave — for a
  full fingerprint at R=1. Good enough for the common turn.
- **Escalate (Tier 2):** re-ask only the shaky or hard-gated axes as focused, *voted*
  fan-groups. Spend accuracy exactly where the read wobbled.

This is an evolution of the fan-group concept, not a replacement: a **self-consistency
fan-group** (variants = paraphrases, aggregator = `MajorityVote`) still fits a
single-verdict question; the **pre-frontal battery** is a fan-group whose variants are
the three coordinate probes and whose aggregator is cross-check fusion. Same
`fanout.Pool` primitive, different variant shape and aggregator — the primitive already
takes a pluggable `Aggregator`.

Relatedness triage still runs **upstream** of the battery: it decides what context the
probes see (it is not a fourth axis). See [`prompt_fan_groups.md`](prompt_fan_groups.md).

## Deferred / open questions

- The exact fusion arithmetic (per-axis confidence, correlated-pair reconciliation,
  off-diagonal detection) — its own aggregator + behavior feature.
- Axis scales: graded floats vs. a coarse N-level scale the small model can emit
  reliably under constrained decoding.
- Corpus representation of a multi-axis probe (each probe returns several fields, not a
  single verdict) — extends the `output_contract` / `vote_on` model.
- `decisiveness` default and calibration; per-axis hard/soft designation location.
- Session-boundary and retrieval-subsystem contracts the Implicit/Global triggers hand
  off to.
