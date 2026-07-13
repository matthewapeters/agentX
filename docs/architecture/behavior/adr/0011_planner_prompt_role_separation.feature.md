# Behavior — Planner Prompt Role Separation + Conditional Directory-Listing Bias

Status: **SHIPPED 2026-07-13**. Follow-up to session `vivid-beacon-2`
(2026-07-13): the planner decomposed "find and read the documentation, identify three
unique features" into a `list_dir` (`ls -la`) leaf even though the full, accurate,
552-line project tree was already present in the planner's own context. Investigation
confirmed this was **not** a context-assembly gap — the tree fact genuinely was in the
prompt — it was (a) a literal, unconditional instruction telling the model to prefer
listing before reading, with no carve-out for "unless already known," and (b) a prompt
structure with no role separation at all, which independently increases the odds that
data gets under-weighted relative to instructions.

## Problem

### No system/user role separation for the planner call
`decompose.Chat` (`internal/runtime/decompose/live.go:14`) is a flat-string seam:
`func(ctx context.Context, prompt string, format json.RawMessage) (string, error)`.
`LLMPlanner.Plan` (`live.go:33-43`) renders the **entire** `agentx-planner.md` template —
durable behavioral rules, the tool catalog, "What you know" (working-memory facts,
including the 552-line `tree` pin), the goal, and the reply-format JSON spec — into one
string, which `classifier_pipeline.go:160-166` wraps as a single
`ollama.Message{Role: "user", Content: prompt}`. There is no `Role: "system"` message at
all for this call.

This is the outlier in the codebase, not the norm: `prompting.Assembler.Assemble`
(`internal/prompting/prompting.go:74-79`), which builds the respond-path messages,
already does a proper split — `{Role: "system", Content: a.systemPrompt}` then
`{Role: "user", Content: userText}`. `internal/llm/ollama/ollama.go:160-193` confirms
`Complete` sends `req.Messages` (a real `{role, content}` array) straight through as the
JSON `"messages"` field to Ollama's `/api/chat` endpoint — Ollama applies the *model's own
chat template* per role there (whatever the model's Modelfile defines, typically special
tokens marking system vs. user turns). That is the actual mechanism that makes a model
weight "authoritative instructions" differently from "the task at hand" — it is not
something achievable by inserting text labels (e.g. `[SYSTEM]`/`[DATA]`/`[USER]`) into a
single already-flattened user-role string, since that string is still just more undifferentiated
tokens to the model, with none of its role-based, chat-template-level training engaged.

### Position effects compound the problem
Within that single flattened string, today's order is:

```
[instructions incl. "prefer a task that lists a directory... before... reads one"]
[What you know: {{context}}]   ← the 552-line tree sits here, sandwiched in the middle
[Goal: {{goal}}]
[reply-format JSON instructions]
```

Two independent, well-documented effects compound here: (1) "lost in the middle" —
content sandwiched between two other blocks is systematically under-attended relative to
content at the start or end of a long context; (2) instruction-before-data ordering — the
model reads the "prefer listing" bias *before* it has seen what it already knows, so it
may anchor on "list a directory" as the default move before ever checking whether the
fact already answers the question.

### The bias instruction itself is unconditional
`config/seed/agentx-planner.md` states: *"Only use paths/facts you actually know from
'What you know' below. Do not invent a path that isn't given to you — prefer a task that
lists a directory to discover real filenames before a task that reads one."* Nothing tells
the model to skip listing when a directory's contents are already given. The model
followed this literally — the generated goal text, *"discover all top-level docs files
available to read,"* essentially restates the instruction's own suggested pattern rather
than reasoning about whether it was necessary.

## Design

### 1. Split the planner prompt into a real system message and a real user message
Partition `agentx-planner.md`'s current content by what is durable-across-calls
(behavioral rules) versus per-call data:

- **System part** (durable; becomes the new `agentx-planner.md` content): the DAG/node
  semantics (task vs. step, ids, deps), "never restate the goal as a node," the tool
  catalog (`{{catalog}}`), the no-shell-syntax constraint, and the directory-listing
  guidance — **reworded to be conditional**: *"...prefer a task that lists a directory to
  discover real filenames before a task that reads one — unless a listing of that
  directory is already present in the working-memory context you are given, in which case
  use it directly instead of re-listing."*
- **User part** (per-call data; becomes a new Go template, not a seed file — it is pure
  mechanical scaffolding with nothing a user would tune): "What you know:\n{{context}}",
  "Goal:\n{{goal}}", and the reply-format JSON spec (kept adjacent to the goal
  deliberately — recency right before generation is *wanted* here, unlike the bias
  instruction's recency in the old layout, since you want the model to remember the exact
  output shape right as it responds).

GIVEN the planner call is made WHEN the prompt is assembled THEN the model receives two
messages — `{Role: "system", Content: <durable rules + catalog>}` and
`{Role: "user", Content: <working memory + goal + reply format>}` — never one flattened
string under a single role.

### 2. Working memory keeps its position immediately before the goal
No change needed here beyond the split itself: once the bias instruction moves to the
system message, the user message becomes exactly `[What you know] → [Goal] → [reply
format]` — working memory is no longer sandwiched between an instruction and the goal, it
leads directly into the goal it should inform.

### 3. Conditional directory-listing guidance
GIVEN a directory's contents are already present under "What you know" WHEN the planner
resolves a task that would otherwise list that directory THEN it should use the known
listing directly — the reworded instruction (above) states this explicitly rather than
leaving it to inference.

## Implementation shape — SHIPPED 2026-07-13

Built exactly as scoped:

- `internal/prompting/planner/planner.go`: `DefaultPromptTemplate` is now the system-only
  content; `DefaultUserTemplate` holds the user-part scaffolding. `Render` is replaced by
  `RenderSystem(template, catalog string) string` and
  `RenderUser(template, goal, context string) string` (kept symmetric/testable — both
  take an explicit template argument rather than closing over a constant).
- `internal/runtime/decompose/live.go`: `Chat`'s type is now
  `func(ctx context.Context, systemPrompt, userPrompt string, format json.RawMessage) (string, error)`;
  `LLMPlanner.Plan` renders both parts (`planner.RenderSystem`/`planner.RenderUser`,
  the latter always against `planner.DefaultUserTemplate`, per the "hardcoded, not
  configurable" scoping below) and passes both through.
- `internal/runtime/classifier_pipeline.go`'s `chat` closure now builds
  `[]ollama.Message{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}}`.
- `config/seed/agentx-planner.md`: reworded to system-only content; the "What you
  know"/"Goal"/reply-format sections moved to `DefaultUserTemplate`; the listing-bias
  guidance is now conditional ("...UNLESS a listing of that directory is already present
  in the working-memory context, in which case use it directly instead of re-listing").
- **Blast radius**: `internal/runtime/decompose/live_test.go` was the only other
  `decompose.Chat`/`LLMPlanner` call site; updated to the new two-string signature with
  added assertions that goal/context land in the user message and the catalog in the
  system message, never crossed.

## Tests

- `internal/prompting/planner/render_test.go` (new):
  `TestRenderSystemUserPartition` asserts the catalog only ever appears in
  `RenderSystem`'s output, goal/context only ever in `RenderUser`'s, and no unfilled
  `{{...}}` placeholder survives either; `TestSystemTemplateListingBiasIsConditional` is
  a regression guard against re-introducing the unconditional listing-bias wording.
- `internal/runtime/decompose/live_test.go`: updated `TestLLMPlannerParsesReply` for the
  new signature, with the system/user cross-contamination assertions above.
- **Not built**: the harder-to-pin-down live-model regression scenario (an actual
  decomposition call against a real model, asserting it does not re-list an
  already-known directory) — flagged in the original design as needing a `@manual` or
  documentation-only treatment if it can't be made deterministically reliable; not
  attempted this pass. The structural tests above verify the *mechanism* (role
  separation, conditional wording) is in place; they cannot verify a live model's actual
  behavior change, which was the original session's symptom.

## Open questions

1. Should the reply-format JSON spec move to the system message instead (durable across
   calls) rather than the user message? Kept in the user message deliberately per the
   design above (recency benefit right before generation), but worth revisiting if models
   in practice drift from the shape more when the "what you know" block grows very large
   between the schema reminder and the actual output.
2. Should `PlannerPrompt`/`Paths.PlannerPath()` (currently one config file) become two
   settable prompts (system + user), or should only the system part remain externally
   configurable (as scoped above) with the user part hardcoded? Scoped above as
   hardcoded/Go-constant for the user part, since it is mechanical scaffolding, not
   tunable guidance — revisit if a real need for tuning the user-part shape emerges.
