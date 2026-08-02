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
  - system prompt for the `internal/classify` pipeline's route verdict. That
    pipeline is disconnected from the live prompt/response loop (unwired, not
    deleted — see `04_llm_prompt_tooling_runtime.md`, "Legacy: classify /
    continuation / task-classifier pipeline"); this file has no effect on
    current runtime behavior until/unless the pipeline returns as a hook or
    tool (`90_open_questions.md`, D.5).
- ~/.config/agentx/agentx-thinking.md
  - thinking guidance folded into the respond system prompt when thinking; steers
    reasoning toward the bounded "sweet spot" (built-in default; tunable)
  - see `04_llm_prompt_tooling_runtime.md` (Thinking Pass-through)
- ~/.config/agentx/agentx-shell-commands.md
  - superseded: tools are now advertised to the model via native tool-calling
    schemas generated from each `Descriptor` (`internal/tools.Registry.ToolSchemas`),
    not injected as a catalog document — there is no LLM-facing catalog file
    anymore. See `04_llm_prompt_tooling_runtime.md` ("Native tool calls (v2)").

### Runtime tables (`agentx.toml`)

In addition to `[agentx.ollama]`, the following nested tables tune v1 behaviour:

```toml
[agentx.classification]     # vestigial: only consulted by the disconnected
retries = 2                 # internal/classify pipeline (see agentx-classification.md
clarification_options = 3   # above); has no effect on the live loop today.

[agentx.output]
max_widget_lines = 20     # max body rows before an output widget scrolls in place
input_max_lines = 8       # max rows the input panel grows to before it scrolls

[agentx.thinking]
enabled = true              # master switch for reasoning during respond (💭 widget); absent → on
time_budget_seconds = 180   # wall-clock cap on thinking; on expiry, fall back to a direct answer

[agentx.thinking.routes]    # vestigial: loaded and live-reloadable but never consulted
respond_directly = false    # by the live loop, which applies `enabled` above uniformly,
single_tool = true          # once per turn, with no route to key off (route-aware depth
invoke_planner = true       # was retired with the classifier — see 04_, "Thinking Pass-through (v2)")

[agentx.theme]
active_border_color   = "cyan"        # focused panel + selected output widget (bold)
inactive_border_color = "dark gray"   # unfocused panel + other widgets

[agentx.tools]                        # planned (M3b); see 04_/05_ and build-plan/04
enabled          = true               # master switch for tool execution
timeout_seconds  = 30                 # per-command execution timeout
output_max_bytes = 65536              # output cap before truncation (full output → artifact)
absolute_max_bytes = 2097152           # ceiling on the oversized-output recovery gate's "capture more" (TOOL-6)
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
  - accept an explicit name via `agentx --session <name>` (for scripted
    multiplexer layouts); absent the flag, generate the default.
  - ensure uniqueness with deterministic suffixing when needed (applies to both
    generated and explicitly provided names).

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
- agent_delta — a **transient** streaming chunk of the agent's answer, published
  on the in-process bus for the chat window's live typing effect only. Deltas are
  **never persisted** and **never streamed to external surfaces** — they carry no
  durable identity. The complete answer is emitted once as `agent_response`.
- agent_response — the **complete**, durable agent answer for a turn, published
  once when the response finishes assembling. This is the canonical conversation
  element: persisted, seeded/streamed to surfaces, and the unit the context
  surface toggles (one event, one ordinal, one file — see Enabled semantics).
- attachments
- tool_call
- tool_result
- processing_state

> **Streaming vs. durable (agent_delta vs agent_response).** The chat window, which
> subscribes to the in-process bus, renders `agent_delta` chunks live and finalizes
> the widget when the complete `agent_response` arrives. The recorder and every
> external surface (context viewer, context-visualizer) deal only in the complete
> `agent_response`, so the durable log holds one event per conversation element
> rather than a fragmented stream. Errors are emitted directly as a complete
> `agent_response`.

## Enabled Semantics

`enabled` on the envelope controls whether a conversation element participates in
the **assembled LLM context** of subsequent turns (`runtime.withContext`).
User-prompt and agent-response elements default enabled. Thinking, system-prompt,
and approval events are display-only and never enter context — `enabled` on them
is inert. Tool-call and tool-result events (the flat, untagged native-tool-call
kind; a plan step's tagged call is unaffected) default
**disabled**: their text already folds into the turn that produced them via the
respond-turn context block (`toolResultContext`/`toolDeniedContext`), so nothing
extra happens by default — but unlike thinking/classification, `enabled` on a
tool event is **not** inert: flipping it on folds the element into every
subsequent turn's assembled context too (`orchestrator.recordTurn`'s `toolPin`
registration; sent to the model as a tagged user-role message — see
`historyMessages`), until it is flipped off again.

This is a **session-scoped** mechanism — it applies to *that exact past event*,
toggled by hand, one at a time. It is deliberately distinct from **Pin** (below),
which copies the element into working memory as a durable, potentially
self-refreshing fact. The two do not compose automatically: pinning an event to
WM disables it here (so the content is never represented twice), but re-enabling
it here after that does not touch the WM copy, and deleting the WM copy does not
re-enable it here. See `docs/ux/03_PANEL_DETAILS.md` PD-CTX-AF-011/012 and PD-WM.

- The **context surface** is the management affordance: toggling an element
  (space) flips its `enabled`.
- The orchestrator applies the toggle **in memory** (so the next prompt's context
  reflects it immediately) and **persists it in the element's event file** (so a
  re-attaching surface seeds the correct state). Because each element is a single
  event, this is a single-file rewrite.
- An enabled tool element's bytes are reported under the `tools` class in
  `Orchestrator.ContextBreakdown`, so the context-visualizer's `tools` 🔧 band
  (otherwise always zero) reflects it. A tool element pinned to WM instead counts
  under `working-memory` (see below).

## Pinning to Working Memory

**Pin** (PD-CTX-AF-012 / PD-WM) is the durable, curated counterpart to plain
`enabled`: pressing `p` on a selected `tool_result` element in the context
surface copies it into a `session.Fact` (`Owner: pin`) and disables the source
event (`SetEventEnabled(ordinal, false)`) so the content is represented exactly
once, never twice.

A pinned fact carries a `Source` (`{Tool, Args}`, captured from the `tool_result`
event's payload) and a `Live bool`:

- **static** (`Live: false`, the default at pin time): the value is a frozen
  snapshot, edited/deleted like any other fact — never re-run.
- **live** (`Live: true`, set from the working-memory surface's `l` play/pause
  key, PD-WM-AF-008): re-run via `Orchestrator.refreshLiveFacts`, called once at
  the top of every `runPrompt`, before `withContext` assembles the turn. A
  refresh failure keeps the stale value rather than failing the turn (WM
  degrades gracefully, the same posture as the context-visualizer's
  unknown-window handling). Setting a fact live is refused unless
  `policy.Evaluate(descriptor, args)` currently returns `Allow` for its source
  tool — pin-live must never silently execute something that would otherwise
  need approval, and it must never block a turn on an approval prompt.

`Fact.Age()` (since the last successful refresh for a live fact, since `PinnedAt`
for a static one) is folded into the value text sent to the model
(`pinAnnotatedValue` in `orchestrator.go`), so the model has the same staleness
signal the WM surface shows the user.

**Unpinning** is the existing WM delete affordance — no separate command. It
removes the fact only; it does not restore the source context event's `enabled`
state, which the user controls independently in the context surface.

## Persistence Behavior

- writes should be append-oriented and crash-safe
- avoid mutable rewrite of prior events except explicit maintenance operations —
  toggling an element's `enabled` from the context surface is such an explicit
  maintenance operation (a single-field rewrite of one event file)
- maintain chronological ordering by epoch and filename prefix
- persist enough metadata for replay and audit

## Network Bind Defaults

- browser-capable surfaces use configurable bind address
- default bind address is localhost

Surface launch defaults:

- runtime should present endpoint-based launch commands by default.
- raw port launch form remains supported as compatibility alias.
