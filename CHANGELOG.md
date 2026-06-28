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
- Added a synthesized remediation brief artifact capturing the documentation triad review outcomes under `.subutai/runs/2026-06-23-doc-review-triad/`.
- Added ADR-0006 and indexed it in the ADR navigation so architecture decisions referenced by implementation docs are discoverable.

### Changed

- Documented branch triad independent reviews (Go, architect, and SDET) and aligned downstream documentation updates with the consolidated findings.
- Corrected documentation link/path references to match current repository structure and avoid stale or non-resolvable paths.
- Aligned reason-code and determinism contract documentation language across planning, execution, and validation references.
- Clarified event-broker gate criteria documentation so promotion/acceptance checks are explicit and testable.
- Clarified consistency-audit historical disposition language to distinguish prior findings from currently active remediation items.
