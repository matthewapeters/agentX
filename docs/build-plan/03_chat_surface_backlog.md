# Chat Surface Backlog — First Vertical Slice

Last updated: 2026-06-26
Status: Execution-ready backlog
Owner: Delivery Lead
Scope: The minimal end-to-end slice that lands a working `agentx` chat interface.

## Context

`docs/build-plan/01_comprehensive_build_plan.md` defines milestones M0–M4 but stops at
capability headings; it has no task-level backlog. This document is that missing tier
for **one vertical slice**: the default chat surface launched by `agentx`. It cuts a
thin path through M1 (runtime), the chat portion of M2 (UX), and the streaming portion
of M3a (LLM) — and explicitly defers everything else.

See `../architecture/00_ARCHITECTURE_RECONCILIATION.md` (Family A is the target) and
`../architecture/runtime_contracts/` (frozen Phase 0 contracts) for the baseline.

## Goal (definition of done for the slice)

Running `agentx` starts the orchestrator and a two-panel chat surface. The user types a
prompt, presses Enter, the prompt is sent to Ollama, the response streams into the
output panel, the processing-state indicator reflects idle→working→completed, and the
exchange is persisted as JSON events. `make all` stays green throughout.

## Architecture of the slice

```
agentx (one process)
├── internal/app           composition root
├── internal/runtime       orchestrator lifecycle + prompt cycle
├── internal/state         canonical event bus + processing-state (subscribe iface)
├── internal/session       session identity + append-only JSON event persistence
├── internal/config        config resolution + first-run seeding
├── internal/prompting     minimal prompt assembly (system + user)
├── internal/llm/ollama     streaming Ollama adapter
└── internal/surfaces
    ├── chat               Bubbletea host: 2-panel layout, in-process bus subscriber
    ├── output             output panel renderer (event entry types)
    └── input              input panel (textarea, submit, stop)
```

The default chat surface is an **in-process** subscriber to `internal/state` (it boots
with the server). The HTTP/SSE transport, surface registration, and attach tokens are
**not** needed for this slice — they exist only to serve *external* surfaces and are
deferred (see Out of Scope).

Every task carries the cross-cutting contract obligations: a GIVEN/WHEN/THEN behavior
doc before implementation, a Godog feature + steps under the tag scheme, and AC→test
traceability (`01_comprehensive_build_plan.md` §3).

---

## Phase A — Runtime foundation (M1 slice)

### CHT-A1 · Config resolution + first-run seeding · S
- **Target**: `internal/config/`
- **Source**: `implementation/03_configuration_and_storage.md`
- **Behavior**: resolve effective config (deployment `~/.config/agentx/agentx.toml`
  first, then project `.agentx/.agentx.toml`); seed defaults on first launch; expose
  `ollama_host` and `ollama_model`.
- **Feature**: `tests/features/runtime/config_resolution.feature`
- **Done**: precedence + first-run seeding verified; missing-config path seeds and loads.

### CHT-A2 · Session identity + storage init · S
- **Target**: `internal/session/` (identity)
- **Deps**: CHT-A1
- **Source**: `implementation/03` (Session Storage Root), `01_runtime_blueprint.md`
- **Behavior**: create `session_id` (canonical) + adjective-noun `session_name` with
  deterministic collision suffixing; create `~/.config/agentx/sessions/<id>/` with
  `session.json` metadata.
- **Feature**: `tests/features/session/session_identity.feature`
- **Done**: id/name created, name collisions resolved, session dir + metadata written.

### CHT-A3 · Event bus + processing-state · M
- **Target**: `internal/state/`
- **Source**: `architecture/channel_registry.md`,
  `architecture/runtime_contracts/processing-state.schema.json`, `event-envelope.schema.json`
- **Behavior**: in-process publish/subscribe for the event channels (stream_start,
  agent_header, agent_content, thinking_*, tool_*, error, stream_end) with per-subscriber
  queues (slow subscriber must not block others); a session-level processing-state feed
  (`idle|working|completed|failed` × `classify|thinking|tool|respond|none`).
- **Feature**: `tests/features/runtime/event_bus.feature`, `processing_state.feature`
- **Done**: ordered atomic fan-out to multiple subscribers; processing-state validates
  against schema; payloads conform to event-envelope.

### CHT-A4 · Append-only session event persistence · M
- **Target**: `internal/session/` (store)
- **Deps**: CHT-A2, CHT-A3
- **Source**: `implementation/03` (Persistence Behavior), `event-envelope.schema.json`
- **Behavior**: subscribe to the event bus; write each event as
  `<epoch>_<content_type>.json` under the session's `events/`; append-only, crash-safe,
  ordered by epoch.
- **Feature**: `tests/features/session/event_persistence.feature`
- **Done**: a prompt cycle's events are recoverable from disk in epoch order.

### CHT-A5 · Orchestrator lifecycle · M
- **Target**: `internal/runtime/`
- **Deps**: CHT-A1, CHT-A3
- **Source**: `01_runtime_blueprint.md` (Runtime Lifecycle)
- **Behavior**: startup sequence (config → session → bus → state publisher → model
  readiness → serving loop); graceful shutdown (stop accepting prompts → drain in-flight
  model task → flush writes → final processing-state snapshot → exit).
- **Feature**: `tests/features/runtime/lifecycle.feature`
- **Done**: clean startup and shutdown with no dropped/!flushed events; idle state on boot.

### CHT-A6 · App composition + CLI wiring + entrypoint · S
- **Target**: `internal/app/`, `internal/cli/`, `cmd/agentx/main.go`
- **Deps**: CHT-A5
- **Source**: `08_go_module_layout.md` (import matrix)
- **Behavior**: `run()` parses args (keep `--version`), assembles the dependency graph in
  `internal/app`, starts the orchestrator and the chat surface in one process; honors the
  import-direction matrix.
- **Feature**: `tests/features/runtime/entrypoint.feature`
- **Done**: `agentx` boots the orchestrator + chat surface; `make all` green.

---

## Phase B — Chat surface (M2 slice)

### CHT-B1 · Bubbletea chat host: 2-panel layout · M
- **Target**: `internal/surfaces/chat/`
- **Deps**: CHT-A3 (subscribes to bus); wires bubbletea submodule (`replace charm.land/bubbletea/v2 => ./bubbletea`)
- **Source**: `01_runtime_blueprint.md` (Bubble Tea Adoption); replaces legacy PD-01/PD-02 geometry
- **Behavior**: a Bubbletea program with two panels — output (top, flex) and input
  (bottom, fixed) — and focus handling; resizes with the terminal.
- **Feature**: `tests/features/surfaces/chat_layout.feature` (`@ux:PD-01 @ux:PD-02`)
- **Done**: two panels render and reflow; input focused on start; Ctrl+C quits cleanly.

### CHT-B2 · Output panel: render event entry types · L
- **Target**: `internal/surfaces/output/`
- **Deps**: CHT-B1
- **Source**: `03_PANEL_DETAILS.md` PD-01 (message entry types) — re-authored for TUI
- **Behavior**: scrollable viewport rendering: user message, classification line,
  thinking block (collapsible, collapsed by default), assistant streaming response,
  tool_call + tool_result (collapsible), error. Turn order: response children below the
  user entry (PD-01-AF-001).
- **Feature**: `tests/features/surfaces/output_render.feature` (`@ux:PD-01`)
- **Done**: each entry type renders; streaming appends in place; scroll works; thinking/
  tool blocks expand/collapse. (tool entries render when emitted; execution is M3b.)

### CHT-B3 · Input panel: textarea + submit + stop · M
- **Target**: `internal/surfaces/input/`
- **Deps**: CHT-B1
- **Source**: PD-02-AF-001/002/003 + stop (AF-004) — re-authored for TUI
- **Behavior**: multi-line textarea; Enter submits (PD-02-AF-001), Shift+Enter inserts
  newline (PD-02-AF-002), submit disabled while streaming (PD-02-AF-003), stop/interrupt
  available while streaming (PD-02-AF-004).
- **Feature**: `tests/features/surfaces/input_controls.feature` (`@ux:PD-02`)
- **Done**: submit emits the prompt intent; Shift+Enter newline; disabled-while-streaming;
  stop cancels an in-flight cycle.

### CHT-B4 · Processing-state indicator · S
- **Target**: `internal/surfaces/chat/` (status line)
- **Deps**: CHT-B1, CHT-A3
- **Source**: `channel_registry.md` (Processing State Contract)
- **Behavior**: subscribe to the processing-state feed; show idle/working + phase without
  owning orchestration logic.
- **Feature**: `tests/features/surfaces/processing_state_indicator.feature` (`@arch:runtime-bootstrap`)
- **Done**: indicator tracks idle→working(phase)→completed with no drift.

### CHT-B5 · Submit → prompt → event-stream round trip · M
- **Target**: `internal/surfaces/chat/` ↔ `internal/runtime/`
- **Deps**: CHT-B2, CHT-B3, CHT-A6
- **Behavior**: chat host hands a submitted prompt to the orchestrator (in-process) and
  renders the resulting event stream + processing-state transitions back into the panels.
- **Feature**: `tests/features/surfaces/chat_round_trip.feature` (`@e2e`)
- **Done**: a submitted prompt produces rendered streamed output end-to-end (against a
  stub model; real model lands in Phase C).

---

## Phase C — LLM prompt loop (M3a slice)

### CHT-C1 · Ollama streaming adapter · M
- **Target**: `internal/llm/ollama/`
- **Source**: `04_llm_prompt_tooling_runtime.md` (Default Model Service)
- **Behavior**: stream chat completions from local Ollama using `ollama_host`/
  `ollama_model`; surface token deltas and completion/err; readiness probe.
- **Feature**: `tests/features/runtime/ollama_adapter.feature` (`@integration`)
- **Done**: streaming deltas delivered; model-not-ready and connection-error paths bounded.

### CHT-C2 · Minimal prompt assembly · S
- **Target**: `internal/prompting/`
- **Source**: `04` (Prompt Stack Model) — MVP subset
- **Behavior**: assemble a request from a system prompt + the user message (persona/
  skills/procedural/classification stages deferred).
- **Feature**: `tests/features/runtime/prompt_assembly.feature` (`@unit`)
- **Done**: deterministic assembled request for a given input.

### CHT-C3 · Prompt cycle orchestration · M
- **Target**: `internal/runtime/` (prompt cycle)
- **Deps**: CHT-A3, CHT-A5, CHT-C1, CHT-C2
- **Behavior**: on prompt submit, set processing-state working/respond, assemble prompt,
  stream from Ollama, publish stream_start → agent_header → agent_content* → stream_end,
  then processing-state completed; failure path → failed + error event.
- **Feature**: `tests/features/runtime/prompt_cycle.feature` (`@functional`)
- **Done**: classify→respond MVP (streaming respond) with deterministic event ordering;
  cancel (from CHT-B3 stop) terminates cleanly.

### CHT-C4 · Active-model config + readiness · S
- **Target**: `internal/config/`, `internal/llm/ollama/`
- **Source**: `04` (Model config behavior)
- **Behavior**: read active model from config; verify readiness at startup (CHT-A5);
  surface a clear error if unavailable. (Live model-switch workflow deferred.)
- **Feature**: `tests/features/runtime/model_readiness.feature` (`@integration`)
- **Done**: startup blocks/reports clearly when the configured model is unavailable.

---

## Phase D — Classification cycle (M3a)

> **⚠️ Superseded (2026-07-31).** CHT-D2–D4/D7 below shipped as written, then were
> replaced by the native tool-calling loop (`internal/runtime/loop.go`) — the
> classify→route architecture they describe (and the `classification` event,
> `PhaseClassify`, route-aware thinking) is disconnected from the live loop.
> CHT-D1/D5/D6's TUI/widget/thinking-pass-through work is still live, minus the
> classify-specific bits. See `../implementation/04_llm_prompt_tooling_runtime.md`
> ("The Prompt/Response Loop") for the current architecture and
> `../implementation/90_open_questions.md` (D.5) for whether/how classification
> returns as a hook or tool. This section is kept as a historical record of what
> was built and why, not as a current target.

Un-defers the `classify` phase. Specs: `../implementation/04_llm_prompt_tooling_runtime.md`
(Classification Cycle) and `../ux/06_OUTPUT_WIDGET.md` (collapsible output widget).

### CHT-D1 · Collapsible output widget (TUI) · L
- **Target**: `internal/surfaces/output/`
- **Source**: `ux/06_OUTPUT_WIDGET.md` (re-authors PD-01/PD-09 for the TUI)
- **Behavior**: per-entry widget — always-visible word-break-truncated header, collapse/
  expand, `[agentx.output] max_widget_lines` cap, inner viewport scroll with a
  proportional scrollbar thumb, `lipgloss.NormalBorder` box, focus/selection model.
- **Feature**: `tests/features/surfaces/output_widget.feature` (`@functional @ux:PD-01`)
- **Done**: header/cap/inner-scroll/scrollbar/border behave per spec; existing render
  scenarios still pass.

### CHT-D2 · Classification prompt + config · S
- **Target**: `internal/config/`, `internal/prompting/`
- **Source**: `04` (Classification Cycle), `03` (User prompt files / runtime tables)
- **Behavior**: seed `agentx-classification.md`; load it; add `[agentx.classification]`
  (`retries`, `clarification_options`).
- **Feature**: `tests/features/runtime/classification_config.feature` (`@integration`)
- **Done**: prompt + retry/clarification settings resolve with defaults.

### CHT-D3 · Classifier (classify → route) · M
- **Target**: `internal/classify/` (new)
- **Deps**: CHT-C1, CHT-C3, CHT-D2
- **Source**: `04` (routable taxonomy, strict-JSON contract, retry/fallback)
- **Behavior**: run the classification call, extract+validate JSON, retry up to N,
  fall back to `respond_directly`; emit the `classification` event.
- **Feature**: `tests/features/runtime/classification.feature` (`@functional`)
- **Done**: deterministic route from a parseable verdict; malformed → retry → fallback.

### CHT-D4 · Prompt cycle integration · M
- **Target**: `internal/runtime/`
- **Deps**: CHT-D3
- **Behavior**: insert `PhaseClassify` before `PhaseRespond`; route `respond_directly`
  (reserved routes fall back); render the greyed `⚙️ intent → route` line.
- **Feature**: `tests/features/runtime/classify_respond_cycle.feature` (`@functional`)
- **Done**: `idle → classify → respond → completed` with deterministic event ordering.

> Stage 2 (ambiguity + user clarification: K interpretations, number-select, append &
> resubmit; `clarification` content type; `awaiting_input` state) is a separate backlog
> increment once Stage 1 lands.

### CHT-D5 · Panel focus model + ESC chord + theming · M
- **Target**: `internal/surfaces/chat/`, `internal/surfaces/output/`, `internal/config/`
- **Deps**: CHT-D1
- **Behavior**: track input/output panel focus; ESC is a leader chord
  (`ESC,q` quit · `ESC,↑` focus output · `ESC,↓` focus input · `ESC,ESC` interrupt);
  PgUp/PgDn auto-focus output; `j/k` + arrows scroll the selected widget while output
  is focused. Focused panel renders a bold `active_border_color` border, the other an
  `inactive_border_color` border; the selected widget lights up only while output is
  focused. New `[agentx.theme]` config (name/ANSI-256/hex colors).
- **Feature**: `tests/features/surfaces/focus_navigation.feature`,
  `tests/features/surfaces/command_mode.feature` (`@functional`)
- **Done**: focus toggle + chord + scroll keys behave per spec; themed borders render;
  `make all` green.

### CHT-D6 · Thinking pass-through · S
- **Target**: `internal/llm/ollama/`, `internal/runtime/`, `internal/surfaces/output/`,
  `internal/config/`
- **Deps**: CHT-D1, CHT-D4
- **Behavior**: `Model.Chat` gains an `onThink` callback (non-nil ⇒ request thinking).
  The respond phase streams reasoning as `thinking` events ahead of the answer
  (`working/thinking → working/respond` on first content delta); classification never
  thinks. Output coalesces reasoning into one collapsed `💭` widget. New
  `[agentx.thinking] enabled` config (default on).
- **Feature**: `tests/features/runtime/classify_respond_cycle.feature`
  (thinking-streams-before-response variant) (`@functional`)
- **Done**: thinking widget renders; ordering `user_prompt → classification → thinking
  → agent_response`; `make all` green.

### CHT-D7 · Thinking sweet-spot tuning · M
- **Target**: `internal/runtime/`, `internal/config/`, `internal/prompting/`
- **Deps**: CHT-D6
- **Behavior**: route-aware thinking depth via `[agentx.thinking.routes]` (the
  classification verdict gates thinking and is injected as a calibration hint);
  tunable `agentx-thinking.md` guidance folded into the respond system prompt; a
  wall-clock `time_budget_seconds` (default 180) that, on expiry before any content,
  cancels the stream and falls back to a direct non-thinking answer.
- **Feature**: `tests/features/runtime/classify_respond_cycle.feature`
  (thinking-streams + budget-fallback variants) (`@functional`)
- **Done**: route gating, prompt injection, and budget fallback behave per spec;
  `make all` green.
- **Follow-up**: model-native effort levels; feed partial reasoning into the fallback.

---

## Sequencing

```
A1 ─┬─ A2 ─┬─ A4
    └─ A3 ─┼─ A5 ─ A6 ───────────────┐
           └─ (bus available) ─ B1 ─ B2 ─┐
                                  ├─ B3 ─┼─ B5 ── INTEGRATION ── slice done
                                  └─ B4 ─┘        │
C2 ─ C1 ─ C3 ─ C4 ──────────────────────────────┘
```

- Phase B can start as soon as **A3** (the bus) exists; panels develop against stubbed
  events until **B5** wires the real orchestrator.
- Phase C develops in parallel; it converges at **B5 + C3** for the end-to-end loop.
- Critical path: A1→A3→A5→A6→B5 and C1→C3→(B5/C3 integration).

## Out of scope (explicitly deferred)

| Deferred | Belongs to |
| --- | --- |
| HTTP/SSE transport, surface registration, attach tokens, port allocation, launch-command generation, multi-instance | M1 (external-surface enablement) |
| System surfaces: files, config, context, context-history, context-visualizer (PD-03/04/07/08/09/11/12) | M2 (separate surfaces) |
| Attachment chips (PD-02-AF-005..007) | later feature |
| Right-click copy/paste context menus (PD-01-AF-010, PD-02-AF-008..012) | later (TUI redesign) |
| Plan tabs / PlanView (PD-05), context-meter donut (PD-10) | later feature / separate surface |
| Tool runtime + policy enforcement (tool execution) | M3b |
| Classification cycle (classify → route → respond) | **Phase D (this doc)** |
| Classification Stage 2: ambiguity + user clarification flow | later M3a (Stage 2) |
| `think`/procedural prompt stages; tool/planner routes | later M3a/M3b (reserved, fall back now) |
| Live model-switch workflow | later M3a |
| Persistence replay & operational hardening | M4 |

## Milestone / AC mapping

- Phase A → M1 (`AC-M1-3` session/config init; lifecycle stability).
- Phase B → M2 (`AC-M2-1/2` in-scope affordances = PD-01/PD-02 chat subset, processing-
  state visibility without drift).
- Phase C → M3a (`AC-M3a-1` deterministic event ordering for the respond flow).

Each task's Godog feature provides the test-case IDs (`TC-<milestone>-<area>-<nnn>`) that
fill the AC coverage tables at the relevant checkpoint.
