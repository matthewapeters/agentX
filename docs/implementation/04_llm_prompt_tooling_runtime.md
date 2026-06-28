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
