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
