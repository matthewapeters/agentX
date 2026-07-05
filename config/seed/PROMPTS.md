# AgentX Prompts — the Decision Tree

This is the human-facing companion to [`prompts.toml`](prompts.toml). That file is
the machine-readable source of truth AgentX loads; this file explains how it works,
so you can understand, trust, and refine it.

AgentX runs **one local model**. That means **prompts are the accuracy lever** — how a
question is asked matters more than which model answers it. So the prompts are exposed
here for you to read and tune.

## How to read this

AgentX doesn't hand your message to the model in one big pass. It runs a small
**decision tree** of classifiers first, each of which is a **fan-group**: several
prompt *variants* that ask the same question different ways, then **vote**. Voting is
how a small model earns confidence — agreement across variants is trustworthy;
disagreement means "I'm not sure," and the system asks you rather than guessing.

Each fan-group runs as a **cascade**: one cheap prompt first (the *coarse gate*), and
only if that gate is unsure — or the stakes are high — does it escalate to the full
vote. Most turns settle at the cheap gate.

## The tree

```
                      your message
                            │
                 ┌──────────▼───────────┐
                 │  relatedness_triage  │   how does this relate to the session?
                 └──────────┬───────────┘
        continuation / new / orthogonal / related_aside
                            │  (decides what context to carry)
                 ┌──────────▼───────────┐
                 │   action_classify    │   are you asking me to DO something?
                 └──────────┬───────────┘
              actionable? ──┤
                   yes      │      no
                    │       └──────────────▶ just converse
                    ▼
             (produce a task; execute + verify)
```

> The graph above is hand-drawn today; it is intended to be **generated from
> `prompts.toml`** so it can never drift from the real groups. If you add or rename a
> group, update this section (or regenerate it).

## The fan-groups

### `relatedness_triage` — "how does this relate to what we were doing?"

Runs **first**, and its job is to decide **what context the next steps get to see**.
The verdicts:

| verdict | meaning | what context is carried |
|---------|---------|-------------------------|
| `continuation` | you're extending the current task | thread + task context |
| `new` | a fresh, unrelated request | thread context dropped |
| `orthogonal` | a side-quest | domain context, not task context |
| `related_aside` | *looks* new but is a lateral move in the same goal | thread context, carried carefully |

It is deliberately **conservative about dropping context** — wrongly forgetting what
you were doing is worse than carrying a little extra. `related_aside` is the tricky
case (a topic that *looks* new but isn't), which is exactly why this group votes
instead of trusting one prompt.

### `action_classify` — "are you asking me to do something?"

Decides whether your message is a request to **act** (`artifact` = produce/write,
`command` = run something, `query` = look something up) or just to converse (`none`).
A **file write or a command always escalates to a vote** — a false positive there is
destructive, so it never rides on the cheap gate alone. If the vote scatters, the
system asks you to confirm rather than acting on a guess.

## Editing safely

- **Your copy wins.** Edit `~/.config/agentx/prompts.toml`; `make seed` never
  overwrites it. (This file and the corpus ship as defaults you can always restore.)
- **Keep the answer schema.** Every variant in a group must still return the fields in
  that group's `[output_contract]` (e.g. `relation` + `confidence`). If an edited
  variant breaks the shape, AgentX **falls back to the shipped default and warns you**
  — it won't silently misclassify.
- **Vary the ask, not the answer.** Reword prompts, change framing, adjust
  temperature — but keep every variant answering the *same* question in the *same*
  JSON shape, or the vote can't compare them.
- **A group needs enough working variants to reach its `quorum`.** If too many are
  broken, that stage abstains and asks you.

## What you changed

AgentX can show which prompts differ from the shipped defaults (transparency cuts both
ways — if accuracy shifts after you tune, this is where to look). This lives in the
future prompt/fan-group inspector surface.
