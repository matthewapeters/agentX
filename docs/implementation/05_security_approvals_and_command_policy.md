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
- the surface renders the approval affordance on the `tool_call` (🔧) widget:
  `[a] approve session · [g] approve global · [d] deny`
- the runtime resumes on the decision (channel), persists the chosen scope, then
  executes or aborts
- the same `awaiting_input` mechanism backs Stage-2 classification clarification

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

## Future Security Extensions

- optional signed policy bundles
- role-based policy profiles
- restricted execution sandboxes
- HTTPS and local auth for HTTP control paths
