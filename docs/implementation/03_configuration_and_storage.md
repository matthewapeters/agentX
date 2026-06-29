# Configuration and Storage Contracts

## Configuration Source of Truth

Two operational config locations:

Development defaults:

- <project folder>/.agentx/.agentx.toml

Deployed runtime config:

- ~/.config/agentx/agentx.toml

Implementation rule:

- resolve effective configuration from deployment config first
- fall back to project local defaults if deployment config missing
- seed deployment config with defaults on first launch

## Additional Config Files

Config strategy:

- hybrid model: main runtime config in ~/.config/agentx/agentx.toml with optional domain overrides.

Tool configuration:

- ~/.config/agentx/tools.toml (or equivalent under same folder)

Command policy configuration:

- ~/.config/agentx/command-policy.toml
  - blacklist (forbidden commands)
  - global whitelist (approved for all sessions)

Prompt catalog configuration:

- ~/.config/agentx/prompts.toml (or retained in main agentx.toml)
  - enabled always-on prompt files
  - one-time selection behavior metadata

User prompt files (Markdown, optional):

- ~/.config/agentx/agentx-instructions.md
  - standing user instructions prefixed to every LLM context (respond step)
- ~/.config/agentx/bootstrap-prompt.md
  - prompt submitted automatically at startup; the response opens the session
  - see `04_llm_prompt_tooling_runtime.md` (Instructions and Bootstrap Prompts)
- ~/.config/agentx/agentx-classification.md
  - system prompt for the classification step; describes the agentic-workflow
    taxonomy used to route prompts (seeded with a default; tunable)
  - see `04_llm_prompt_tooling_runtime.md` (Classification Cycle)
- ~/.config/agentx/agentx-thinking.md
  - thinking guidance folded into the respond system prompt when thinking; steers
    reasoning toward the bounded "sweet spot" (built-in default; tunable)
  - see `04_llm_prompt_tooling_runtime.md` (Thinking Pass-through)
- ~/.config/agentx/agentx-shell-commands.md
  - LLM-facing catalog of curated tools, injected into context when a turn routes to
    `single_tool` (built-in default `tools.DefaultCatalog`; tunable)
  - see `04_llm_prompt_tooling_runtime.md` (The single_tool cycle) and
    `05_security_approvals_and_command_policy.md`

### Runtime tables (`agentx.toml`)

In addition to `[agentx.ollama]`, the following nested tables tune v1 behaviour:

```toml
[agentx.classification]
retries = 2               # re-attempts when a classification verdict won't parse
clarification_options = 3 # Stage-2: candidate interpretations offered on ambiguity

[agentx.output]
max_widget_lines = 20     # max body rows before an output widget scrolls in place

[agentx.thinking]
enabled = true              # master switch for reasoning during respond (💭 widget); absent → on
time_budget_seconds = 180   # wall-clock cap on thinking; on expiry, fall back to a direct answer

[agentx.thinking.routes]    # route-aware depth (which classification routes reason)
respond_directly = false    # plain conversation answers without thinking
single_tool = true
invoke_planner = true

[agentx.theme]
active_border_color   = "cyan"        # focused panel + selected output widget (bold)
inactive_border_color = "dark gray"   # unfocused panel + other widgets

[agentx.tools]                        # planned (M3b); see 04_/05_ and build-plan/04
enabled          = true               # master switch for tool execution
read_only        = true               # only read/search tools until the loop is proven
timeout_seconds  = 30                 # per-command execution timeout
output_max_bytes = 65536              # output cap before truncation (full output → artifact)
```

- Unknown keys are ignored by the decoder; absent values fall back to the defaults
  shown above.
- Theme colors accept a name (`cyan`, `dark gray`, …), an ANSI-256 index (`"240"`),
  or a hex value (`"#00afaf"`). See `docs/ux/06_OUTPUT_WIDGET.md` (Focus & keymap)
  for the focus/border model.

## Session Storage Root

All user/agent activity is persisted under:

- ~/.config/agentx/sessions/

Session folder organization:

- one folder per session id, derived from epoch or timestamp
- include explicit epoch in each JSON document for stable sorting
- name each event file `<epoch>_<arrival-seq>_<content_type>.json`; the
  zero-padded per-recorder arrival sequence breaks ties so events sharing a
  millisecond epoch still load back in write order
- persist both session_id and session_name in session metadata

Example layout:

- ~/.config/agentx/sessions/1719090012000/
  - session.json
  - events/
    - 1719090012450_000000_user_prompt.json
    - 1719090012670_000001_system_prompt.json
    - 1719090013300_000002_thinking.json
    - 1719090014100_000003_tool_call.json
    - 1719090014700_000004_tool_result.json
    - 1719090015200_000005_agent_response.json

  Session metadata requirements (session.json):

  - session_id (canonical internal identifier)
  - session_name (human-readable name)
  - created_epoch
  - orchestrator_endpoint
  - attach_token_issued_at (metadata only, not raw secret)

  Human-readable naming guidance (v1):

  - use adjective-noun generation for default names.
  - ensure uniqueness with deterministic suffixing when needed.

## JSON Event Envelope

Minimum required fields:

- epoch
- session_id
- event_type
- content_type
- payload

Suggested optional fields:

- ordinal — per-session monotonic sequence stamped by the event bus at publish
  time; the canonical total order and the resume cursor for surface attach
  (seed-then-subscribe). Carried on both the live event and its persisted copy.
- correlation_id
- parent_event_id
- surface_id
- tool_name
- model_name

Initial content_type values:

- user_prompt
- system_prompt
- thinking
- agent_response
- attachments
- tool_call
- tool_result
- processing_state

## Persistence Behavior

- writes should be append-oriented and crash-safe
- avoid mutable rewrite of prior events except explicit maintenance operations
- maintain chronological ordering by epoch and filename prefix
- persist enough metadata for replay and audit

## Network Bind Defaults

- browser-capable surfaces use configurable bind address
- default bind address is localhost

Surface launch defaults:

- runtime should present endpoint-based launch commands by default.
- raw port launch form remains supported as compatibility alias.
