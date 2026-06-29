# Changelog

All notable changes to AgentX are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

## [Unreleased] - 2026-06-23

### Added

- Added the `classification` event content type to the frozen event-envelope
  contract (`docs/architecture/runtime_contracts/event-envelope.schema.json` and
  `internal/state`) for the Stage-1 prompt classification cycle (CHT-D4).
- Added panel focus model, ESC chord keymap, and themed focus borders to the chat
  surface, with new `[agentx.theme]` config (CHT-D5).
- Added thinking pass-through: the respond phase streams model reasoning as
  `thinking` events into a collapsed `💭` widget, gated by new `[agentx.thinking]`
  config (default on) (CHT-D6).
- Added thinking sweet-spot tuning: route-aware depth (`[agentx.thinking.routes]`),
  a tunable `agentx-thinking.md` guidance prompt, and a wall-clock
  `time_budget_seconds` (default 180) that falls back to a direct answer on expiry
  (CHT-D7).

- Added `internal/tools` command policy and curated descriptors (TOOL-1): argument
  schema validation plus blacklist → global → session → approval evaluation with
  reason codes, and the built-in curated toolset registry.
- Added the tool executor and session artifact store (TOOL-2): argv/no-shell
  execution with stdin, timeout, stdout/stderr/exit capture and an output cap;
  `read_file`/`write_file`/`read_output` built-ins; full output persisted to
  `sessions/<id>/artifacts/` with a compact preview + ref and line-windowed read-back.
- Added the tool approval round-trip (TOOL-3): new `awaiting_input` processing
  state, an orchestrator approval gate (`RequestApproval`/`Resolve`) that pauses the
  cycle and persists the approved scope, and a chat affordance mapping a/g/d to
  approve-session / approve-global / deny.
- Wired the end-to-end `single_tool` cycle (TOOL-4): a strict-JSON tool proposer
  (`tools.Proposer` + `DefaultCatalog`), `classify → tool → respond` integration
  with policy/approval and read-only gating, `tool_call`/`tool_result` events, and a
  respond turn that carries the result preview + ref (not the full artifact). New
  `[agentx.tools]` config and `agentx-shell-commands.md` catalog loading.
- Persisted the command policy across sessions (TOOL-5): the blacklist loads from
  `agentx-tool-blacklist.toml` and global approvals are written to / reloaded from
  `agentx-tool-approvals.toml` under `~/.config/agentx/`; executor output cap now
  honors `[agentx.tools] output_max_bytes`. New blacklist seed template.
- Added a bootstrap logo banner to the chat output surface: the application logo
  (`logo/agentx.logo`, ANSI-colored text) is embedded into the binary and rendered
  as the first element of the output transcript, pinned above all widgets, as a
  "running" signal while the bootstrap prompt is processed. Each banner line is
  clipped to the panel width so the art survives narrow terminals. The Makefile
  re-syncs the embedded copy (`cmd/agentx/assets/agentx.logo`) from the authored
  source whenever it changes. Documented in `docs/ux/06_OUTPUT_WIDGET.md` and
  `docs/implementation/09_makefile_and_quality_gate_contract.md`.
- Added a text cursor and readline-style line editing to the chat input panel:
  typing, Backspace, and Shift+Enter now act at the cursor, with Left/Right
  (char), Ctrl-A/Ctrl-E (buffer start/end), and Alt-B/Alt-F (word back/forward)
  movement; word motion is also bound to Ctrl-←/Ctrl-→ as a multiplexer-safe
  alias, since zellij intercepts Alt-F for its floating-pane toggle. History
  seeding leaves the cursor at the end of the seeded text. The
  cursor renders as a reverse-video cell while the panel is focused. Documented as
  `docs/ux/03_PANEL_DETAILS.md` PD-02-AF-017…024.
- Added readline-style prompt history seeding to the chat input panel: `↑`/`↓`
  seed the editable buffer with prior prompts submitted during the current run
  (the in-progress draft is stashed and restored at the present line), hitting a
  boundary flashes the input border instead of moving, and the idle Esc,Esc chord
  clears an active seed back to an empty prompt. Seeding copies a prompt for reuse —
  submitting (as-is or edited) always creates a new prompt. History is in-memory and
  current-run only; persisting across session reload is a follow-up. Documented as
  `docs/ux/03_PANEL_DETAILS.md` PD-02-AF-013…016.
- Added conversation context continuity (CTX-1): each turn is now assembled with
  the prior enabled turns folded in (instructions → working memory → enabled
  history → current user prompt), giving the model multi-turn continuity instead of
  the previous single-turn context. User prompts and agent responses are enabled by
  default; thinking and tool events are retained but disabled by default; the
  bootstrap prompt and its response are excluded from context after processing. Adds an
  `enabled` field to the frozen event-envelope contract
  (`docs/architecture/runtime_contracts/event-envelope.schema.json`) with
  per-content-type defaults (`state.DefaultEnabled`) — a versioned schema change.
- Added session working memory (WM-1): a per-session `working_memory.json` of
  user-controlled facts (`internal/session` `WorkingMemory`/`Fact`), bootstrap
  seeding of stable environment facts (`userid`, `cwd`, `project`, `home`, `os`,
  `arch`, and `repo_root` when in a git work tree) as user-owned facts when absent,
  and re-read-each-turn injection of enabled facts as a system message after the
  instruction layer. The TUI management surface is deferred.
- Captured the tool-runtime design (first built-in command-line tool) ahead of
  implementation: `docs/build-plan/04_tool_runtime_backlog.md` (TOOL-1…5 for M3b),
  the `single_tool` cycle + output-artifact/context-shaping notes in
  `docs/implementation/04`/`05`, `[agentx.tools]` config, and the
  `config/seed/agentx-shell-commands.md` tool catalog seed.

### Changed

- Made output-widget collapse uniform: user, thinking, tool, and assistant widgets
  are all collapsible (Enter/^o toggles the selection) with bodies bounded by
  `max_widget_lines`; the assistant answer and user prompts gained label headers.
- Added a synthesized remediation brief artifact capturing the documentation triad review outcomes under `.subutai/runs/2026-06-23-doc-review-triad/`.
- Added ADR-0006 and indexed it in the ADR navigation so architecture decisions referenced by implementation docs are discoverable.

### Changed

- Documented branch triad independent reviews (Go, architect, and SDET) and aligned downstream documentation updates with the consolidated findings.
- Corrected documentation link/path references to match current repository structure and avoid stale or non-resolvable paths.
- Aligned reason-code and determinism contract documentation language across planning, execution, and validation references.
- Clarified event-broker gate criteria documentation so promotion/acceptance checks are explicit and testable.
- Clarified consistency-audit historical disposition language to distinguish prior findings from currently active remediation items.
