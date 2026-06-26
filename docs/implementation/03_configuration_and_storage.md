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
