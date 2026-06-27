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
