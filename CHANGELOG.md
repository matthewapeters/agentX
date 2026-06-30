# Changelog

All notable changes to AgentX are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

## [Unreleased] - 2026-06-23

### Added

- Added an in-session launch-info widget so the surface-attach commands survive the
  alternate screen: the chat boot path now installs a collapsed, scrollable
  launch-info widget as the first output widget (after the banner, before the
  bootstrap response) instead of printing a startup hint that the alt-screen wiped.
  Expanding it lists the launchable surfaces by name; with the widget selected,
  pressing a digit `1..N` copies that surface's full `agentx surface launch <kind>`
  command to the clipboard via OSC 52 (`tea.SetClipboard`) and confirms by name. The
  attach command (and token) is never rendered — it only ever reaches the clipboard,
  so the token stays off-screen and no mouse capture is needed (native text selection
  is preserved). The widget is surface-local — never a session event — so it is not
  persisted and never appears on attached peer surfaces; it is omitted when the
  transport is disabled. New `output.Model.SetLaunchInfo`/`CopyCommand`; documented in
  `docs/ux/06_OUTPUT_WIDGET.md` (Launch-info widget). Replaces the dead `LaunchHint`
  stdout print.
- Added the context viewer surface (SS-3, completing M2's first peer surface): a new
  read-only `internal/surfaces/context` surface launched with
  `agentx surface launch context`. It projects the session event stream into the
  shared collapsible output renderer (reused from the chat output), intercepts
  processing-state events for a one-line status indicator, and exposes scroll/select
  navigation only (no prompt input). Registered in the launch dispatch
  (`surfaceModelFor`). Specced as `docs/ux/03_PANEL_DETAILS.md` PD-CTX (superseding
  the legacy GUI PD-03/PD-08 context affordances for the TUI). An end-to-end test
  attaches a context surface to a live orchestrator, seeds the prior exchange from
  the durable log, and renders live events streamed thereafter.
- Added the shared surface-client framework (SS-2, M2): a new
  `internal/surfaces/client` package with a `SurfaceModel` contract
  (`Apply`/`SetSize`/`Key`/`View`) and a Bubble Tea `Host` that drives the attach
  lifecycle — apply the durable seed before any live event, listen on the
  cursor-resumed stream, resize, and quit (invoking `POST /surface/{id}/shutdown`)
  on a quit key or a closed stream; non-quit keys forward to the surface.
  `client.Run` wires seed→subscribe→program, and `cli.RunSurface` makes
  `agentx surface launch <kind>` dispatch by kind to a TUI (or attach headless when
  a kind has no surface yet), kept in `internal/cli` to honor the import matrix.
  `transport/http.Client` gained `Shutdown`. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added the disk-seeded, cursor-resumed event stream for attaching surfaces (SS-1,
  first slice of M2): the event bus now stamps a per-session monotonic `ordinal` on
  every event at publish time (carried on the envelope so the live event and its
  persisted copy share one identity — a versioned addition to
  `event-envelope.schema.json`). A new `GET /sessions/current/events` returns the
  durable log as the seed (incl. each event's `enabled`), and `GET /events?after=<ordinal>`
  resumes the live stream after a cursor: the handler captures a boundary ordinal at
  subscribe time, serves `(after, boundary]` from the durable log and `(boundary, ∞)`
  live, so the seed→live handover has no gap and no duplicate with no client-side
  de-dup. `transport/http.Client` gained `Seed` and `Subscribe(after)`. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Wired the transport into the live runtime, completing M1 external-surface
  enablement (TRN-6): when `[agentx.transport] enabled` (default), the orchestrator
  allocates a loopback port and serves the HTTP/SSE server as part of `Start` (after
  the bus/processing/registry/recorder are live, before accepting prompts) and stops
  it on `Shutdown` (server first, then attached surfaces marked `stopped`, then the
  recorder drains); `enabled = false` keeps the pure in-process mode. The chat boot
  path prints the attach hint (endpoint + raw token + launchable kinds) for use in
  other terminals. The SSE handler now also returns on server shutdown so a
  long-lived stream cannot block the graceful drain. `*runtime.Orchestrator` carries
  a compile-time assertion that it satisfies the transport `Provider`. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added the `agentx surface launch` CLI (TRN-5): the canonical
  `agentx surface launch <kind> --session <s> --connect <ep> --token <t>` plus the
  `-l/--launch -s/--session -p/--port` compatibility alias (port mapped to a
  loopback endpoint; alias also accepts `-t/--token` since v1 requires one). It
  validates in order — known surface kind, non-empty session selector, local-safe
  (loopback) endpoint, endpoint reachability, session-selector match, then token —
  mapping each failure to a deterministic category (validation | auth | transport |
  conflict) and a non-zero exit. A new `transport/http.Client` performs the attach
  (`GET /sessions/current` + `POST /surface/register`), and `surfaces.KnownKind`
  gates the launchable surface set. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added transport port allocation and endpoint publication (TRN-4): a new
  `[agentx.transport]` config table (`enabled`, `host`, `port_start`, `port_end`,
  default loopback `127.0.0.1:8420-8460`); `transport/http.Allocate` binds the
  lowest free TCP port in the range ascending (the bind is the availability check,
  so concurrent agentx instances fall through and there is no TOCTOU gap) and
  returns the bound listener, failing with a range-exhausted error when the range
  is occupied; the resolved endpoint is published to session metadata as
  `sessions/<id>/transport.json` (`session.Store.WriteTransport`/`ReadTransport`),
  never carrying the raw attach token. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added the HTTP transport write endpoints (TRN-3): `POST /surface/register`
  (authorized by the attach token in the body; reason category maps to status —
  auth→401, validation→400, conflict→409), `POST /prompt` (bearer-authorized,
  gated by the orchestrator accepting state, runs the cycle async with events
  flowing back over SSE), `POST /tool/approval` (forwards the decision to the
  approval gate), `POST /surface/{id}/shutdown`, plus `POST /surface/{id}/command`
  and `POST /model/switch` reserved as `501 not_implemented` in v1. Non-register
  writes require an `Authorization: Bearer <attach-token>` header validated against
  the session token (`Registry.ValidateToken`). The `Provider` seam widened with
  `Submit`/`Resolve`/`Accepting`. Documented in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added the loopback HTTP/SSE transport read + streaming endpoints (TRN-2): a new
  `internal/transport/http` server adapts the orchestrator's canonical state behind a
  local `Provider` interface (so transport never imports runtime). `GET /health`,
  `GET /processing-state`, `GET /surfaces`, and `GET /sessions/current` return JSON
  snapshots; `GET /events` streams the event bus as Server-Sent Events, one
  independent bus subscription per connection so a slow surface never blocks others.
  Mechanics + GIVEN/WHEN/THEN contracts in
  `docs/implementation/02_surface_orchestration_http.md`.
- Added the surface registry and ephemeral attach token (TRN-1, first slice of the
  M1 external-surface enablement backlog): the orchestrator mints one per-session
  attach token at startup (raw value held in memory only) and owns an
  `internal/surfaces` registry that validates the token at registration, records the
  frozen `surface-registration` payload, manages surface lifecycle (ready/stopped),
  rejects bad tokens (`auth`) and id conflicts (`conflict`), and exposes only the
  non-secret token fingerprint. Registration mechanics documented in
  `docs/implementation/02_surface_orchestration_http.md`; backlog in
  `docs/build-plan/05_transport_backlog.md`.
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
