# Behavior — Wavefront `Classifier` Contract (ADR 0012 Phase 6)

Status: **Implemented** (2026-07-17). Realizes ADR 0012's Phased Build Plan
(amendment) step 6. First genuinely new LLM-facing logic in this effort — the
KNOW/NEED contract, schema-constrained prompt, parse path.

Built exactly as scoped below, including the Phase 2 prompt corrections (object-
wrapped wire shape, explicit reply-format spec in the user template) made as part of
this phase since building the actual consumer is what surfaced the gaps. Tests:
`classifier_test.go` (`Parse`'s six scenarios), `live_test.go`
(`LLMClassifier.Classify`'s system/user partition, default-template fallback, error
propagation), `prompts_test.go` (extended with the wrapper-key regression guard).
Full suite clean.

## Problem

Phase 2 shipped the classify prompt's *text* but not its consumer, and building the
consumer now surfaces two gaps worth fixing rather than carrying forward:

1. The prompt's worked example shows a bare JSON array as the response
   (`[{"KNOW":...}, {"NEED":...}]`). Every other JSON-producing call in this codebase
   (`planner.Parse`, the classifier fan-out) is object-wrapped at the top level and
   recovered via `internal/jsonx.FirstObject` — described in that package's own doc
   comment as "the single source of truth for this concern" specifically so no
   parser can drift back into fence-intolerance. A bare-array response would need
   either a new `jsonx.FirstArray` (a second, parallel "source of truth") or its own
   ad hoc fence-stripping — neither is worth it just to preserve an incidental choice
   made before the consumer existed.
2. `wavefront.DefaultClassifyUserTemplate` (Phase 2) has no reply-format spec at
   all — the response shape is only ever shown in the system prompt's worked
   example. `planner.DefaultUserTemplate` deliberately puts the reply-format spec in
   the *user* message, adjacent to the goal, for recency right before generation
   (ADR 0011) — the classify prompt should follow the same discipline, not skip it.

## Design

### 1. Wire shape: object-wrapped, reusing `jsonx.FirstObject` unchanged

```json
{"classification": [
  {"KNOW": {"name": "...", "value": "..."}},
  {"NEED": {"name": "...", "command": {"tool": "...", "args": {"...": "..."}}}},
  {"NEED": {"name": "..."}}
]}
```

A `NEED` with no `command` key is an open question (matches `planner`'s
oneOf-by-key-presence convention: a nil pointer means absent, not a `null`/empty
sentinel value). `config/seed/agentx-wavefront-classify.md`'s worked example and
`wavefront.DefaultClassifyUserTemplate` (which gains an explicit reply-format spec,
matching `planner.DefaultUserTemplate`'s shape) are updated to match — a correction
to already-shipped Phase 2 scaffolding, not a new decision; both were written before
this phase's consumer made the gap concrete.

### 2. Go types — `Command` is structured, not a raw shell string

```go
type Know struct{ Name, Value string }

// Command is a resolved tool call, matching the existing planner's task payload
// shape ({"tool","args"}) — the same catalog/executor path handles both engines'
// resolved calls identically; there is no separate "raw shell command" concept.
type Command struct {
	Tool string
	Args map[string]string
}

// Need is something required but not yet known. A non-nil Command resolves it
// immediately via the executor; nil makes it a new open question. Command.Args must
// come from wm alone — Classify must never propose an argument it expects a sibling,
// not-yet-executed Need to produce (ADR 0012's core grounding rule; this is the
// contract-level statement of it, not a runtime check — enforcement is prompt
// discipline plus the fact that nothing downstream binds an unresolved value into
// Args).
type Need struct {
	Name    string
	Command *Command
}

type Result struct {
	Knows []Know
	Needs []Need
}

// Classifier asks, for one open question against the current blackboard (rendered
// working-memory text — the graph, per the ADR amendment, not a separate struct),
// what's already known/synthesizable and what's still needed.
type Classifier interface {
	Classify(ctx context.Context, wm, question string) (Result, error)
}
```

### 3. `ClassifySchema` constrains decoding; `Parse` is a defensive backstop, not the primary guarantee

Mirrors `planner.PlanSchema`/`planner.Parse`'s posture exactly: the JSON schema
(`oneOf(KNOW, NEED)` per item, `command` optional within `NEED`) is handed to
Ollama's `Format` field so constrained decoding should prevent most malformed
shapes; `Parse` still validates independently (a node with both or neither of
KNOW/NEED, an empty name, a `command` present but missing `tool`) because a
model/Ollama version can fall back to unconstrained text, and `Parse` must not trust
the schema blindly.

```
GIVEN a classify response with a KNOW item and a NEED item (one command-valued, one
      open)
WHEN  Parse runs
THEN  Result.Knows has one entry and Result.Needs has two, the command-valued one's
      Command non-nil with Tool/Args populated, the open one's Command nil.

GIVEN an item with both KNOW and NEED set, or neither
WHEN  Parse runs
THEN  it returns an error naming the item's position, not a partial Result.

GIVEN a NEED item whose command is present but has an empty/missing tool
WHEN  Parse runs
THEN  it returns an error — a command-valued NEED with nothing to call is not a
      valid "resolves immediately" claim.

GIVEN the model wraps its reply in a ```json fence despite instructions not to
WHEN  Parse runs
THEN  it still succeeds — jsonx.FirstObject recovers the payload, unchanged
      behavior from every other consumer of that package.
```

### 4. `LLMClassifier` mirrors `decompose.LLMPlanner`'s construction exactly

Same `Chat` func type signature (system+user strings, optional JSON-schema
`format`), same `Template`/`Catalog` fields, same fallback-to-default-when-empty
convention. Deliberately not shared/reused as one type — each engine constructs its
own, per the ADR's stated risk-isolation posture (a wavefront regression must not
touch the continuous engine's already-proven `LLMPlanner`), even though the
underlying `Chat` closure shape is identical enough that a future refactor could
unify construction without changing either engine's behavior.

## Tests

- `internal/runtime/wavefront/classifier_test.go` (new):
  - `TestParseKnowAndNeedItems` — the four-scenario table above (mixed KNOW/NEED,
    both-or-neither rejected, command-with-no-tool rejected, fence-tolerant).
  - `TestParseOpenNeedHasNilCommand` — explicit nil-vs-populated `Command` check, since
    that's the discriminator the eventual scheduler (Phase 7) branches on.
  - `TestParseRejectsEmptyName` — both KNOW and NEED.
- `internal/runtime/wavefront/live_test.go` (new) — `TestLLMClassifierRendersAndParses`
  mirroring `decompose/live_test.go`'s `TestLLMPlannerParsesReply`: a stub `Chat` that
  captures the rendered system/user text and returns a fixed classify JSON reply;
  asserts the catalog only ever appears in the system message, `wm`/`question` only
  in the user message (the same ADR 0011 partition-cleanliness regression guard
  `planner.TestRenderSystemUserPartition`/`prompts_test.go`'s
  `TestRenderClassifySystemUserPartition` already established at the template level,
  now checked at the full call level).
- `internal/runtime/wavefront/prompts_test.go` (extended) — update the existing
  partition test's expectations for the new reply-format spec in
  `DefaultClassifyUserTemplate`; add a regression guard asserting the user template
  contains the `"classification"` wrapper key, not a bare-array shape.
