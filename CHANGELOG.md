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
