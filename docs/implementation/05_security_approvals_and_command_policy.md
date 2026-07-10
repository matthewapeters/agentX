# Security, Approvals, and Command Policy

## Command Policy Layers

Three policy sets:

1. Blacklist (always forbidden)
2. Session whitelist (approved for current session)
3. Global whitelist (approved for all sessions)

Approval result semantics:

- deny: command blocked
- approve_session: add to session whitelist
- approve_global: add to global whitelist

Approval keying model:

- approvals are keyed by command plus optional validated args
- blacklist rules take precedence over any prior approval
- example: rm may be approved while rm -rf / remains forbidden

## Evaluation Order

1. Check blacklist
2. Check global whitelist
3. Check session whitelist
4. Prompt user for approval

If approved:

- persist approval per chosen scope
- execute command with captured context

## Command Descriptor Contract

Each tool command descriptor should include:

- id
- command
- allowed_args schema
- risk_level
- requires_approval (bool)
- timeout_seconds
- working_directory_policy
- output_capture_policy

## Execution Safety Requirements

- no shell interpolation for untrusted arguments
- enforce argument schema before execution
- enforce timeouts and output size limits
- capture stdout/stderr and exit code
- persist execution record to session events

## Approval Round-trip

The approval prompt is a mid-cycle request for user input, so the prompt cycle
pauses rather than blocks the UI:

- processing-state enters `awaiting_input` (a versioned addition to the
  `RunState` enum / `processing-state.schema.json`; requires a `CHANGELOG.md` entry)
- the runtime publishes a generic `approval_request` event — `{prompt, options}`,
  where each option is a `{label, decision}` pair — via the orchestrator's shared
  decision gate (`internal/runtime/decision.go`). The chat surface swaps a
  navigable-list widget into the input panel in place of the free-text input,
  bordered and titled "AgentX Needs Your Input": up/down (or j/k) moves a
  highlighted-row cursor, Enter confirms and sends the highlighted option's
  `decision` string back. The same widget and interaction model handle every
  decision kind (tool approval, verb-continuation approval, and any future kind)
  — the surface never hardcodes a per-kind option vocabulary or keymap.
- concurrent decisions of any kind serialize through one shared FIFO queue, so
  only one is ever shown at a time regardless of which kinds are pending
- the runtime resumes on the decision (per-request channel), persists any
  kind-specific side effect (policy scope, verb allow/deny list), then proceeds
- the same `awaiting_input` mechanism backs Stage-2 classification clarification

## Policy Persistence

The policy survives restarts (TOOL-5), under `~/.config/agentx/`:

- **blacklist** — loaded from `agentx-tool-blacklist.toml` (`[[rule]]` of
  `tool`/`pattern`/`reason`, RE2 patterns); a missing file means no rules. Seeded
  from `config/seed/agentx-tool-blacklist.toml`. This is the user's to edit.
- **global whitelist** — `agentx-tool-approvals.toml` (`[[approval]]` of `tool` +
  `args`). Runtime-managed: an "approve global" decision appends the entry and
  rewrites the file; it is reloaded into the policy at the next session's start. Keyed
  by tool id + canonical args, so the approval is scoped to that exact argument set.
- **session whitelist** — in-memory only, never persisted.

## Output Artifacts and Context Shaping

Full tool output is persisted but **not** fed wholesale to the model:

- the executor writes the full stdout/stderr to a session artifact
  (`sessions/<id>/artifacts/<seq>.txt`), persisted like any session record
- the `tool_result` carries a compact projection only — `exit`, `status`, `bytes`,
  `line_count`, a short `preview`, and an opaque `ref` (this is the
  "output_digest or size metadata" named under Auditing)
- the model reads more on demand via the `read_output` tool (`ref` + offset/limit)
- the human sees the full output in the 📋 result widget (scrollable)

This bounds context cost for large outputs and keeps a complete, auditable record.

## Auditing

Each tool execution record should include:

- epoch
- user_prompt_reference
- proposed_command
- approval_scope
- execution_status
- output_digest or size metadata (the `ref` + byte/line counts above)

Separately, every resolved interactive decision (not just tool-execution) is
recorded as an `approval_decision` event — the original prompt and the chosen
option's label as text (never the raw key) — via the same persist-everything
session-event mechanism as any other event, but with `Enabled: false` by
default so it never folds into assembled LLM context. The chat surface renders
it as a fixed-gray, one-line record in scrollback. This is the audit trail for
the decision itself, distinct from the tool-execution fields above.

## Future Security Extensions

- optional signed policy bundles
- role-based policy profiles
- restricted execution sandboxes
- HTTPS and local auth for HTTP control paths
