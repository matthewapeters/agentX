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

## Classification Cycle (v1, Stage 1)

Every user prompt is first **classified** for intent, then routed, then answered.
This is the `classify → respond` slice of the documented `classify → think → tool
→ respond` cycle (`06_delivery_plan.md`); `think`/`tool`/planning routes are
reserved and fall back to `respond_directly` until their executors land (M3b+).

### Tunable classification prompt

```
GIVEN a file at ~/.config/agentx/agentx-classification.md
  AND an application `agentx`
WHEN the application classifies a user prompt
THEN the contents of agentx-classification.md are used as the system prompt for
     the classification call (its role for the classify step; agentx-instructions.md
     remains the system prompt for the respond step)
  AND the user's prompt is the user message of the classification call.
```

- The classification prompt is the place to describe the breadth of agentic
  workflows so ambiguous prompts route well. It is **tunable without recompiling**.
- A built-in default is seeded on first launch (see *Default classification
  prompt* below). Absent/empty → the default is used.
- It does **not** replace `agentx-instructions.md`; the two play different roles
  (classify system prompt vs. respond system prompt).

### Routable taxonomy (fixed contract)

The runtime owns the set of routes it can execute; the prompt tunes how prompts
are matched to them, but may only emit a route from this set:

| Route | v1 behaviour |
|-------|--------------|
| `respond_directly` | Stream a conversational answer (the only executable route in v1). |
| `single_tool` | **Reserved** — falls back to `respond_directly` until the tool runtime lands (M3b). |
| `invoke_planner` | **Reserved** — falls back to `respond_directly` until the planner lands. |

An unrecognised or unexecutable route degrades to `respond_directly`.

### Output contract (strict JSON)

The classifier must return a single JSON object:

```json
{ "route": "respond_directly", "confidence": 0.0, "rationale": "short" }
```

- `route` (required) — one of the routable taxonomy values.
- `confidence` (optional, 0–1) and `rationale` (optional, terse) are advisory.
- Parsing is **tolerant of surrounding prose/fences**: the first balanced `{…}`
  object in the response is extracted, then strictly validated against this schema
  and the route enum. The call uses a low temperature.

### Retry and fallback

```
GIVEN a configured retry budget agentx.classification.retries = N
WHEN a classification response cannot be parsed/validated
THEN the runtime retries the classification up to N times
  AND if all attempts fail it falls back to route `respond_directly`
  AND records the fallback (so the cycle never stalls on a malformed verdict).
```

> Genuine ambiguity (the prompt is parseable but unclear) and the user-clarification
> flow (offer K interpretations; user picks one; append and resubmit) are **Stage 2**
> — see `90_open_questions.md` and the Stage-2 backlog. Stage 1 always resolves to a
> route (real or fallback).

### Events and ordering

- A new event content type `classification` carries the verdict (`route`,
  `confidence`, `rationale`); it is persisted and rendered as the greyed `⚙️
  intent → route` line directly under the user prompt (see `ux/06_OUTPUT_WIDGET.md`).
- Adding `classification` to the event envelope is a **versioned change** to the
  frozen contract (`architecture/runtime_contracts/event-envelope.schema.json`) and
  requires a `CHANGELOG.md` entry.
- Processing-state phase transitions: `idle → working/classify → working/respond →
  completed` (the `classify` phase already exists in the phase contract).

### Default classification prompt (seeded)

```markdown
You are AgentX's prompt classifier. Read the user's message and decide how the
assistant should handle it. Reply with ONE JSON object and nothing else:

{"route": "<route>", "confidence": <0..1>, "rationale": "<≤10 words>"}

Routes:
- respond_directly — conversation, questions, explanations, or anything answerable
  without running tools or a multi-step plan.
- single_tool — the request needs exactly one tool/command (e.g. read or edit a
  file, run a command).
- invoke_planner — a complex, multi-step task needing decomposition into a plan.

Prefer respond_directly when unsure. Output only the JSON object.
```

## Thinking Pass-through (v1)

When `[agentx.thinking] enabled` is true (the default), the **respond** phase
requests model reasoning and streams it ahead of the answer:

- The Ollama adapter sets `"think": true` on the chat request and reads the
  separate `message.thinking` field from each stream chunk (distinct from
  `message.content`). Classification never thinks (it needs a fast strict-JSON
  verdict).
- The orchestrator publishes reasoning chunks as `thinking` content events, then
  switches `working/thinking → working/respond` on the first content delta.
- The output panel coalesces the reasoning into a single, collapsed `💭 thinking`
  widget rendered above the `🤖` answer (see `ux/06_OUTPUT_WIDGET.md`).
- Per-turn ordering becomes: `user_prompt → classification → thinking →
  agent_response`. Models without thinking support simply emit no `thinking`
  field and the cycle proceeds straight to the answer.

### Tuning thinking toward the sweet spot

Three composing levers keep thinking *useful but bounded*:

- **Route-aware depth.** The classification verdict decides whether a turn reasons
  at all. `[agentx.thinking.routes]` enables thinking per route (defaults:
  `respond_directly` off, `single_tool`/`invoke_planner` on). The classified route
  is also injected into the thinking prompt as a calibration hint, so the model
  scales its reasoning to the task.
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

### The `single_tool` cycle (v1)

The `single_tool` classification route triggers one tool call before the answer
(`classify → PhaseTool → respond`). Multi-step tool use belongs to `invoke_planner`,
later.

- **Curated descriptors, not a generic runner.** The runtime exposes a fixed set of
  tools (read/search, write/modify, network) defined in `internal/tools`. The
  LLM-facing catalog is `~/.config/agentx/agentx-shell-commands.md` (default
  `tools.DefaultCatalog`), injected into the proposal context only when a turn routes
  to `single_tool`.
- **Strict-JSON proposal, one call per turn.** Reusing the classification pattern
  (strict JSON → tolerant extraction → retry → fallback), the model replies with
  `{"tool": "<id>", "args": {...}}` or `{"tool": "none"}`. Parse failure or `none`
  falls back to a direct response.
- **argv, no shell.** Commands run as an argv vector via `os/exec` — never `sh -c`. No
  pipes, redirects, globs, or expansion. File content and patches are passed inline in
  the JSON and delivered via process **stdin** or a Go built-in, so untrusted arguments
  are never shell-interpolated (see `05_security_approvals_and_command_policy.md`).
- **Events and ordering.** `user_prompt → classification → tool_call → tool_result →
  agent_response`; processing-state moves `classify → tool → respond`. A call needing
  approval inserts an `awaiting_input` pause (see doc 05).

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
