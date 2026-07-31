# LLM, Prompt, and Tooling Runtime

## Default Model Service

Default adapter:

- Local Ollama runtime

Model config behavior:

- active model stored in runtime config
- model switch permitted without application restart
- switch flow must verify model readiness before accepting new prompt work

Suggested model switch flow:

1. user requests switch
2. runtime prompts user for in-flight prompt handling choice
3. runtime probes target model availability
4. runtime warms model if needed
5. runtime marks switch success/failure
6. processing state and UI feedback updated

## Prompt Stack Model

Prompt categories:

- System prompts (user-configurable selection)
- Persona prompts
- Skills prompts
- Commands prompts
- Procedural system prompts (internal, non-user-facing)

Behavior requirements:

- user may enable prompts one-time or sticky
- sticky selection persisted in ~/.config/agentx/agentx.toml
- shipped default prompt bundles seeded at deployment
- user may add or modify user-facing prompt files

## Instructions and Bootstrap Prompts (v1)

Two user-editable Markdown files in the deployment config directory
(`~/.config/agentx/`) shape the prompt stack at the v1 level. Both are optional;
absent or empty files are no-ops.

| File | Role |
|------|------|
| `agentx-instructions.md` | Standing user instructions prefixed to **every** LLM context (the user-facing system prompt). |
| `bootstrap-prompt.md` | A prompt submitted **automatically at startup** so the session opens with an agent response. |

### Story: instructions prefix every context

```
GIVEN a file at ~/.config/agentx/agentx-instructions.md
  AND an application `agentx`
  AND an Input affordance
  AND a User
WHEN the User submits a prompt through the Input affordance
THEN `agentx` prefixes every context with the contents of
     ~/.config/agentx/agentx-instructions.md
  AND finishes every context with the User's submitted prompt
     before passing the context to the LLM.
```

Behavior:

- The instructions file content is loaded at startup and used as the system
  message that leads every assembled context (see `internal/prompting.Assemble`).
- When the file is absent or empty, the built-in `DefaultSystemPrompt` is used.
- Executable contract: `tests/features/runtime/instructions_prompt.feature`.

### Story: bootstrap prompt at startup

```
GIVEN a file at ~/.config/agentx/bootstrap-prompt.md
  AND a file at ~/.config/agentx/agentx-instructions.md
  AND an application `agentx`
WHEN the application `agentx` is started
THEN the contents of ~/.config/agentx/agentx-instructions.md and the contents
     of ~/.config/agentx/bootstrap-prompt.md are submitted to the LLM
     automatically when the application starts
  AND the response is the first thing displayed in the `agentx` output display.
```

Behavior:

- At startup, if `bootstrap-prompt.md` is non-empty, the runtime submits it through
  the normal prompt cycle with `agentx-instructions.md` prefixed, exactly as if the
  user had typed it.
- The bootstrap prompt itself is **not** rendered as a user entry, so the model's
  response is the first thing shown in the output panel.
- Executable contract: `tests/features/runtime/bootstrap_prompt.feature`.

## Context Continuity (v1)

Each turn is assembled with the prior conversation folded in, so the model has
multi-turn continuity rather than seeing only the current prompt. The assembled
context is layered, in order:

1. **Instructions** (Layer 0) — `agentx-instructions.md` / `DefaultSystemPrompt`.
2. **Working memory** (band 0) — enabled facts from `working_memory.json`, re-read
   fresh each turn (see WM-1).
3. **Enabled history** — the prior turns' messages.
4. **Current user prompt** — finishes the context.

### Enabled-by-content-type

Whether an event participates in assembled context is carried by an `enabled` flag
on the event envelope, defaulted by `content_type` (`internal/state.DefaultEnabled`):

| Content type | Default | In context |
|--------------|---------|------------|
| `user_prompt`, `agent_response`, `attachments` | **enabled** | yes |
| `thinking`, `tool_call`, `tool_result` | **disabled** | retained, off by default |
| `classification`, `system_prompt`, `processing_state` | n/a | never context |

- A later context surface (PD-03 Context section / `context-history`) toggles the
  flag per message; disabled messages are excluded from the next call.
- `thinking` defaults off (it is the model's scratch reasoning); the user may enable
  it later. `tool_call`/`tool_result` are retained for audit and may be enabled to
  feed prior tool output back, but default off to keep context lean.
- Adding `enabled` to the frozen event-envelope is a **versioned change** to the
  contract (`architecture/runtime_contracts/event-envelope.schema.json`); absent is
  treated as `false`.

The **bootstrap exchange is excluded** from context. The bootstrap prompt and its
response engage the session at startup but are irrelevant to the user's intent, so
the bootstrap turn is not added to history (it already skips the `user_prompt`
record). Live continuity therefore starts from the first real user turn.

> v1 builds the live history in memory from completed turns (consolidating streamed
> `agent_response` deltas into one assistant message). Reconstructing history from
> the persisted event log on session reload — which must likewise exclude the
> bootstrap exchange (its persisted `agent_response` is still flagged enabled on
> disk) — and deterministic token-budget trimming (persona canon Layer 4) are
> follow-ups.

## The Prompt/Response Loop (v2 — native tool-calling)

Superseded design note: v1 routed every prompt through a semantic classifier
(`classify → respond_directly | single_tool | invoke_planner`) before acting.
That classifier, its `continuation` follow-up detector, and a second
independent task-classifier pipeline (`prompting/pipeline`, `cascade`,
`reconcile`, `corpus`) added up to four separate "should this turn act"
decision layers, each assembling its own context — the opposite of this
project's Context Curation motto (`CLAUDE.md`). The loop below replaces all of
that with one flat loop built on the model's own native tool-calling.

```
GIVEN a started orchestrator with native tool-calling wired
WHEN a user submits a prompt
THEN the orchestrator runs registered synchronous hooks, then asynchronous hooks
  AND submits the prompt (plus history) to the model, advertising the available
      tool schemas (subprocess/builtin tools + plan_task)
  AND detects, from the model's response, tool calls vs a chat response
  AND for a chat response, streams and publishes it, ending the turn
  AND for tool calls, executes each (policy/approval-gated) or runs plan_task,
      folds the results back as tool-role messages, runs the hook points again,
      and loops back to the model
  AND stops looping and answers with whatever it has if a per-turn tool-call
      budget (Settings.MaxToolIterationsPerTurn, default 25) is reached.
```

- Implementation: `internal/runtime/loop.go` (`Orchestrator.runPrompt`),
  `internal/runtime/tool_cycle.go` (`streamResponse`, `runNativeToolCall`).
- Executable contract: `tests/features/runtime/prompt_loop.feature`.

### Native tool-calling wire format

The model advertises tools via the provider's own structured tool-calling API
(Ollama's `tools` request field / `message.tool_calls` response field; the
OpenAI-compatible equivalent for llama.cpp) rather than a hand-rolled
JSON-in-text convention. `internal/tools.Registry.ToolSchemas` builds each
tool's schema from its `Descriptor` (`Description` + JSON Schema generated from
`Args []ArgSpec`); `internal/runtime/model.go` maps that provider-agnostically
between `tools.ToolSchema` and each client's own `Tool`/`ToolCall` wire types
(`internal/llm/ollama`, `internal/llm/llamacpp`, `internal/llm/provider`) — per
the import-direction matrix (`08_go_module_layout.md`), `internal/llm/*` must
not import `internal/tools`, so this mapping lives in `internal/runtime`.
`internal/prompting.Message` carries the shared, provider-agnostic shape
(`ToolCalls`, `ToolCallID`) the loop accumulates within one turn.

### The `plan_task` tool

Multi-step investigation (what the old classifier's `invoke_planner` route
existed for) is now a tool the model calls at its own discretion, named
`plan_task(goal string)` — not a pre-classifier decision. Its description
carries the judgment the old classifier prompt used to make ahead of time
("review, audit, analyze, refactor... spans multiple steps or files").
Calling it runs the configured decomposition engine
(`internal/runtime/decompose.DrainPlan`, or `internal/runtime/wavefront`'s
round-free engine when `Settings.WavefrontEnabled`) to completion — both are
synchronous, so the tool call is a plain call-and-wait, no process spawn
needed — and returns the rendered findings as the tool result. Per-leaf tool
executions inside the plan still go through the same policy/approval gate as
any other native tool call. Implementation: `internal/runtime/plan_tool.go`.

### Hooks framework (present, empty)

`internal/runtime/hooks` is the loop's extension seam: a synchronous chain
(`SyncHook`, run serially in registration order against the live turn) and an
asynchronous fan-out (`AsyncHook`, run against a value-copy snapshot, fire-and-
forget, orthogonal to the loop — indexers, summarizers, telemetry). Both hook
points fire twice per loop iteration: right after a new prompt is recorded, and
after each round of tool execution. Registration is config-driven
(`Settings.HooksConfigPath`, a TOML file resolved against a compiled-in
`hooks.Available` factory registry) — **no hooks are registered in this pass**;
the framework exists so intent evaluation, decomposition-as-a-hook, or other
future extensions can register without changing the loop. A hook that spawns a
recursive loop instance (a sub-agent) must derive its context from
`hooks.NextSpawnContext` and check `hooks.CanSpawn` against a depth budget —
this guardrail exists even though nothing spawns yet.

### Legacy: classify / continuation / task-classifier pipeline (unwired)

`internal/classify`, `internal/runtime/continuation.go`, and
`internal/prompting/{pipeline,cascade,reconcile,corpus}` still exist and still
have working unit test coverage, but nothing in `Orchestrator.Start`/`runPrompt`
constructs or calls them anymore — they are disconnected from the main loop,
not deleted. They may return later as a hook (see above) or as a tool (the
same treatment `plan_task` got), once native tool-call detection has proven
reliable enough to judge whether a separate intent-evaluation layer is still
worth its cost. `docs/implementation/90_open_questions.md` tracks this.

## Thinking Pass-through (v2)

When `[agentx.thinking] enabled` is true, the loop's **first** model call of a
turn requests model reasoning and streams it ahead of the answer (subsequent
tool-round-trip calls within the same turn don't re-think — reasoning happens
once, before acting, not before every tool result):

- The Ollama adapter sets `"think": true` on the chat request and reads the
  separate `message.thinking` field from each stream chunk (distinct from
  `message.content`).
- The orchestrator publishes reasoning chunks as `thinking` content events, then
  switches `working/thinking → working/respond` on the first content delta.
- The output panel coalesces the reasoning into a single, collapsed `💭 thinking`
  widget rendered above the `🤖` answer (see `ux/06_OUTPUT_WIDGET.md`).
- Per-turn ordering: `user_prompt → thinking → agent_response` (a turn with no
  tool calls), or `user_prompt → thinking → tool_call → tool_result →
  agent_response` (a turn that called a tool) — no `classification` event (see
  "The Prompt/Response Loop" above). Models without thinking support simply
  emit no `thinking` field and the cycle proceeds straight to the answer.

### Tuning thinking toward the sweet spot

Two composing levers keep thinking *useful but bounded* (a third, route-aware
depth, existed in v1 but no longer applies — there are no classifier routes to
key off; `ThinkingEnabled` now applies uniformly):

- **Tunable guidance.** `~/.config/agentx/agentx-thinking.md` (built-in default in
  `prompting.DefaultThinkingPrompt`) is folded into the respond system prompt; it
  steers brevity and goal-direction without recompiling.
- **Hard wall-clock budget.** `[agentx.thinking] time_budget_seconds` (default
  `180`) caps the thinking phase. If it elapses before any content delta, the
  runtime cancels the stream (keeping the partial reasoning), records a
  `…(thinking budget reached — answering directly)` note, and re-asks **without**
  thinking so the turn still completes.

> **Open (future):** model-native effort levels (`think: "low"|"medium"|"high"` on
> models that support them) and feeding partial reasoning into the budget-fallback
> answer are deferred; current wire format is boolean `think`.

## Procedural Prompts

Procedural prompts are internal orchestration instructions and should be versioned with application code.

Examples:

- classification prompt constrained to known classes
- tool-use planning prompt
- error-recovery prompt
- response-format guardrails

Implementation guardrails:

- procedural prompts not directly editable in normal user workflow
- strict output schema validation for procedural stages
- failure path when model output violates required schema

## Tool Runtime Overview

Tool registry source:

- local MCP-style command descriptors

Execution behavior:

- LLM proposes tool command + parameters
- runtime validates command plus optional args against policy and allow/deny lists
- runtime requests user approval when needed
- runtime executes approved command and returns structured result

Design principle:

- maximize utility of local CLI environment while preserving explicit user control and safety.

### Native tool calls (v2)

Any number of tool calls may happen per turn, in a loop, bounded by
`Settings.MaxToolIterationsPerTurn` (see "The Prompt/Response Loop" above) —
there is no longer a `single_tool`-route one-call-per-turn limit.

- **Curated descriptors, not a generic runner.** The runtime exposes a fixed set of
  tools (read/search, write/modify, network) defined in `internal/tools`,
  advertised to the model as native tool schemas (see "Native tool-calling wire
  format" above) — there is no LLM-facing catalog document to inject into a
  prompt anymore; each `Descriptor.Description` is the tool's model-facing text.
- **Model-issued, not proposed-and-parsed.** The provider's own structured
  tool-calling parses the call; AgentX no longer parses a `{"tool": ...}` JSON
  object out of free text (`internal/tools.Proposer`/`ParseProposal` were
  retired with the classifier).
- **argv, no shell.** Commands run as an argv vector via `os/exec` — never `sh -c`. No
  pipes, redirects, globs, or expansion. File content and patches are passed inline in
  the call's arguments and delivered via process **stdin** or a Go built-in, so
  untrusted arguments are never shell-interpolated (see
  `05_security_approvals_and_command_policy.md`).
- **Events and ordering.** `user_prompt → tool_call → tool_result → ... →
  agent_response` (no `classification` event; PhaseTool still brackets each
  call). A call needing approval inserts an `awaiting_input` pause (see doc 05).

### Output artifacts and context shaping

Bounding a tool result's size is the tool call's job (a narrower command — add
`-maxdepth`, pipe through `rg`/`head`, scope the path), not something this layer
does after the fact. A truncated-but-unlabeled result is a lie of omission: the
model would be reasoning over partial data believing it saw the whole thing (RCA:
session `nimble-pebble-2`, 2026-07-12 — a `tree` pin silently clipped to 22 of
547 lines, header still claiming the full count, misled a planner into thinking
the project had no Go source). So:

- The executor writes the **full** stdout/stderr to a **session artifact**
  (`sessions/<id>/artifacts/<seq>.txt`) — persisted like any other session record.
- The `tool_result` returned to the model carries the **full** captured text
  (`exit`, `status`, `bytes`, `line_count`, the full `preview` text, and an
  opaque `ref`) — never a further-truncated excerpt of it.
- The one exception is `output_max_bytes`, a capture-level safety net against a
  truly runaway process (e.g. `cat` on a multi-GB file). When it triggers, the
  result text says so explicitly (capped-at-N-bytes notice) — truncation is
  always visible, never silent.
- `read_output` (`ref` + optional offset/limit) lets the agent re-read a specific
  window on demand — a convenience for large-but-fully-captured results, not a
  workaround for a preview that hid something.

The human sees the same full output in the 📋 result widget (scrollable, capped
by `max_widget_lines` for *display* only); the model sees the same text via
`preview`. One stored artifact, one honest projection — this generalizes to any
bulky producer (planner, future experts).
