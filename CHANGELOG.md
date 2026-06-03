# Changelog

All notable changes to AgentX are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Code Changes

#### Changed

- Upgraded the Go filesystem applet row rendering in `cmd/agentx-core/filesystem_widget.go` with semantic visual styling:
  - directories now use reverse-video styling,
  - parent directory (`..`) row now uses a dedicated background highlight,
  - entry kind markers now use emoji (`⤴`, `📁`, `📄`, `❓`) instead of `[F|D]`,
  - file-type color classes were added for hidden files, config files, and code families (Go, Python, JS/TS, C/C++, other common code files).
- Updated filesystem row layout handling to preserve alignment with ANSI-styled content via visible-width padding.

### Test Changes

#### Added

- Added style-contract coverage in `cmd/agentx-core/filesystem_widget_test.go` (`TestFilesystemWidgetRender_StylesFolderHiddenConfigAndCodeFiles`) validating folder reverse-style, parent-row highlight, emoji markers, and file-type color mappings.

#### Changed

- Updated filesystem rendering assertions in `cmd/agentx-core/filesystem_widget_test.go` to validate emoji-based row rendering semantics instead of legacy `[F|D]` markers.

### Documentation Changes

#### Changed

- Updated `docs/architecture/applets/filesystem_applet.md` to authoritative current-state contract language:
  - refreshes ownership/runtime state descriptions,
  - captures the implemented visual style and classification decisions,
  - points evidence anchors to `filesystem_widget.go` / `filesystem_widget_test.go`,
  - removes stale transitional framing for the core filesystem widget behavior.

## [1.0.2] - 2026-06-01

### Code Changes

#### Added

- Added native Go pane/widget surfaces in `cmd/agentx-core` for chat/output, input, logs, and system/context rendering.
- Added Go-side system applet host scaffolding and working-memory system applet surface.
- Added deterministic classify-phase implementation and prompt-cycle state-machine coverage.

#### Changed

- Promoted checked-in chat runtime behavior to the Go-native path with direct deterministic fallback semantics in Go-runtime scenarios.
- Extended startup topology support with `--startup-mode visible-windows` and safe default-layout fallback behavior.
- Hardened demo submit retry behavior and surfaced retry observability in runtime/demo summaries and health payloads.
- Serialized `hybrid-parity-gate` execution to prevent concurrent parity-run interference.

### Test Changes

#### Added

- Added focused Go tests for widgets, classify/routing state machine, startup mode, system renderer host routing, and launch/supervisor contracts.
- Added/expanded headless parity scripts for demo UX use cases, system panel tab-tour coverage, routing/layout checks, and attached runtime layout checks.

#### Changed

- Updated integration feature scenarios and Godog wiring for direct Go fallback and delayed-backend recovery ordering.

### Documentation Changes

#### Added

- Added active migration ledger and checkpoints in `docs/hybrid_remaining_work.md`.
- Added system panel tab mapping and startup mode architecture references in `docs/architecture/system_panel_tab_mapping.md` and `docs/architecture/startup_modes.md`.
- Added archived planning set under `docs/archive/` for superseded integration planning material.

#### Changed

- Updated architecture and UX references to reflect Go-current ownership for chat/output, logs, and system/context pane surfaces.
- Consolidated active planning references and removed superseded hybrid/integration plan documents from active paths.

## [1.0.1] - 2026-05-28

### Code Changes

#### Added

- Added DemoMode system-panel tab-tour parity story `e2e-system-tour-001` in `cmd/agentx-core/demo_harness.go`.
- Added dedicated headless coverage script `tests/test_demo_system_panel_tour_headless.sh` to validate full system tab traversal from tour start.

#### Changed

- Updated DemoMode startup greeting validator to align with core-owned startup bootstrap behavior while preserving input-pane command-entry assertions.
- Hardened system-panel tour assertions against tmux line wrapping by validating compacted latest-snapshot pane captures.
- Added a shared core activity-state endpoint (`GET /activity`) derived from prompt-cycle state for applet-consistent busy/completed/failed affordances.
- Updated Go input widget to consume core activity state and render non-blocking visual prompt cues while preserving command semantics.

### Test Changes

#### Changed

- Consolidated Wave 4 regression pack executed across parity layers (unit, Go package, headless parity scripts).
- Updated demo UX headless scripts and smoke markers to include the new tour story in ordered sequence coverage.

### Documentation Changes

#### Changed

- Updated `docs/hybrid_ux_parity_execution_plan.md` to mark H1/H2/H3 completed with evidence.
- Updated `docs/ux/07_DEMO_MODE.md` with `PD-17-AF-014` and a Wave 4 UAT checklist referencing all parity stories and pass criteria.
- Updated `docs/ux/UX_LIFECYCLE.md` PD-17 matrix to reflect implemented and validated parity affordances.
- Updated `docs/architecture/channel_registry.md` and `docs/architecture/runtime_split.md` with the authoritative shared activity-state direction (`/activity`) and multi-applet consumer policy.
- Updated `docs/ux/06_TUI_MIRROR.md` and `docs/ux/UX_LIFECYCLE.md` with PD-16 activity affordance coverage for the input widget and context-aligned state semantics.

## [1.0.0] - 2026-05-24

### Code Changes

#### Changed

- All output/system panel logic, context visualization, and deterministic formatting are now fully Go core-owned. Python applet is a pure thin renderer.
- All output/system panels are Go-driven, with deterministic rendering and agentic orchestration.
- All blockers for Go-driven UX parity are resolved.

### Documentation Changes

#### Changed

- Updated `docs/ux/UX_LIFECYCLE.md` to mark all output/system panels as Go-owned and tested, and checklist as complete.
- Updated `docs/architecture.md` and all architectural diagrams to reflect Go-driven ownership and parity.

### Test Changes

#### Changed

- All tests updated and pass for Go-driven output/system panels. No regressions or coverage gaps remain for migrated surfaces.

### Version

- Bumped project version to 1.0.0 (major milestone: 100% Go-driven output/system panel migration, all blockers resolved, all tests pass, full UX parity achieved).

### Code Changes

#### Changed

- Refactored `applets/template.py` to act as a thin renderer: all output/system panel logic, context visualization, and deterministic formatting removed from Python applet layer.
- Output/system panel logic, context visualization, and deterministic formatting are now owned by the Go core (see runtime split doc).
- Python applet now receives rendering instructions/data from Go core via IPC and prints them directly.

### Documentation Changes

#### Changed

- Updated `docs/architecture.md` and `docs/architecture/runtime_split.md` revision stamps and project version to v0.85.0 to reflect the thin-renderer migration.
- Updated `docs/ux/UX_LIFECYCLE.md` revision stamp to v0.85.0 for traceability.

### Test Changes

#### Changed

- All tests re-run and pass after migration to thin-renderer model. No test failures or regressions.

### Version

- Bumped project version to 0.85.0 (minor, non-breaking feature addition: output/system panel logic migrated to Go core, Python applet is now a thin renderer).

## [0.84.1] - 2026-05-16 (continued)

### Documentation Changes

#### Added

- Authored authoritative runtime split doc: `docs/architecture/runtime_split.md` (Go core vs Python applet, IPC contract, migration phases, thin-GUI contract).

#### Changed

- Updated `docs/architecture.md`, `docs/ux/UX_LIFECYCLE.md`, and `docs/hybrid_ux_parity_execution_plan.md` to trace and mark runtime split doc as completed.

## [0.84.1] - 2026-05-16

### Documentation Changes

#### Added

- Authored authoritative channel registry: `docs/architecture/channel_registry.md` (channel names, schemas, pub/sub wiring, policy, code links).
- Added channel registry to architecture index and traceability matrix.

#### Changed

- Updated `docs/ux/UX_LIFECYCLE.md` and `docs/hybrid_ux_parity_execution_plan.md` next steps to mark channel registry doc as completed and traceable.
- Updated `docs/architecture.md` revision stamp and index for channel registry doc.

### Version

- Bumped project version to 0.84.1 (minor, non-breaking feature addition).

## [0.84.0] - 2026-05-23

### Code Changes

#### Added

- Added hybrid runtime shutdown endpoint plumbing (`/shutdown`) with core shutdown-provider wiring so runtime-initiated exit requests can terminate the full session.
- Added pre-bound dynamic health-listener allocation and applet endpoint propagation (`AGENTX_CORE_HTTP`) to avoid fixed-port collisions across parallel/runtime-residual sessions.
- Added attached-runtime E2E harness `tests/test_tmux_attached_runtime_headless.sh` and Makefile target `test-tmux-attached-runtime-headless`.
- Added deterministic system-pane rendering slice in `applets/template.py` with five GUI-mapped sections:
  - `FILES`
  - `CONFIGURATION`
  - `CONTEXT`
  - `CONTEXT HISTORY`
  - `CONTEXT VISUALIZER`

#### Changed

- Updated core applet launch environment wiring to include project/session context (`AGENTX_PROJECT_DIR`, `AGENTX_USERNAME`) for system-pane data sourcing.
- Updated demo split controller argument plumbing to pass explicit health address (`--health-addr`) from live core session.
- Updated startup flow to prepare the health listener before applet launch and then focus input pane deterministically.

#### Fixed

- Fixed health endpoint contract so `/shutdown` acknowledges first and performs shutdown asynchronously, preventing client-side hangs during self-termination.

### Test Changes

#### Added

- Added shutdown endpoint tests in `cmd/agentx-core/core_health_endpoint_test.go` covering:
  - method guard behavior for `/shutdown`
  - async shutdown-provider trigger contract

#### Changed

- Updated `tests/test_tmux_pane_affordances_headless.sh` to validate the new five-section system-pane contract while tolerating tmux hard-wrap behavior in captured text.
- Updated `cmd/agentx-core/demo_split_mode_test.go` for new demo-controller health-address argument expectations.

## [0.83.1] - 2026-05-23

### Code Changes

#### Fixed

- Added an authoritative Go pane-title registry for hybrid runtime and demo panes so runtime titles must match documented UX names.
- Renamed hybrid core pane titles to align with GUI expectations:
  - `output`
  - `system`
  - `input`
  - `logs`
- Renamed demo split pane titles to the approved authoritative names:
  - `stores`
  - `testControler`
  - `liveCore`
- Updated tmux pane-target resolution to preserve runtime behavior while exposing the approved public pane titles.
- Updated startup layout behavior to keep the interactive cursor focused on the input pane before attach.

#### Added

- Added Go contract coverage in `cmd/agentx-core/pane_titles_docs_contract_test.go` to fail when a registered pane title is not documented in UX specs.

### Test Changes

#### Changed

- Updated Go and shell headless tmux tests to validate the approved pane-title contract across:
  - startup layout
  - split demo layout
  - pane affordances

### Documentation Changes

#### Changed

- Updated `docs/ux/06_TUI_MIRROR.md` and `docs/ux/07_DEMO_MODE.md` with authoritative pane-title contracts.
- Refreshed `docs/ux/UX_LIFECYCLE.md §7` with a new hybrid gap chart including a `Newly Discovered In This Pass` column.
- Updated `docs/ux/00_INDEX.md` and `tests/test_tmux_layout_headless.md` to reflect the aligned pane vocabulary.

## [0.83.0] - 2026-05-23

### Code Changes

#### Added

- Added deterministic hybrid lifecycle observability hooks in the Go core runtime for:
  - `startup_greeting`
  - `submitted`
  - `classified`
  - `thinking`
  - `tool`
  - `final_response`
- Added lifecycle event sequencing and details rendering to the logs pane for contract verification during demo/runtime execution.
- Added lifecycle observability tests in `cmd/agentx-core/core_lifecycle_observability_test.go` covering:
  - one-shot startup lifecycle emission
  - deterministic prompt lifecycle ordering

### Test Changes

#### Added

- Added unit/integration coverage for lifecycle observability ordering contract in Go core test suite.

### Documentation Changes

#### Changed

- Updated `docs/hybrid_ux_parity_execution_plan.md`:
  - marked W0.2 complete
  - added W0.2 evidence and session log entry

## [0.82.1] - 2026-05-23

### Code Changes

#### Fixed

- Updated Go hybrid core runtime config resolution so chat backend and Ollama host/model are sourced from `agentx.toml` before bridge launch, with environment variables still taking precedence.
- Removed hardcoded-only backend/host/model fallback wiring in chat bridge process setup by routing through resolved runtime config.

#### Added

- Added Go unit coverage for runtime config precedence in `cmd/agentx-core/config_runtime_test.go`:
  - TOML values are loaded from `agentx.toml`
  - `AGENTX_*` environment overrides win over TOML
  - default values are used when neither TOML nor env overrides are present

### Documentation Changes

#### Changed

- Accepted and included latest update in `docs/ux/07_DEMO_MODE.md` in this commit.

## [0.82.0] - 2026-05-22

### Code Changes

#### Added

- Added hybrid UX parity placeholder demo stories to the DemoMode sequence:
  - `e2e-greet-001` (startup greeting parity baseline)
  - `e2e-cycle-001` (prompt lifecycle parity baseline)
  - `e2e-system-001` (system panel parity baseline)

#### Changed

- Updated readiness-path demo harness test to target the new final placeholder story ID.
- Established W0.1 parity acceptance criteria in UX docs and traceability matrix:
  - Flow A startup greeting parity
  - Flow B full lifecycle parity
  - Flow C system panel parity

### Test Changes

#### Changed

- Updated `demo_harness_test.go` expectations for extended demo sequence ending in `e2e-system-001`.

### Documentation Changes

#### Added

- Marked W0.1 complete with session evidence in `docs/hybrid_ux_parity_execution_plan.md`.

#### Changed

- Added `PD-17-AF-011..013` traceability rows as spec-baseline affordances (`📝`) in `docs/ux/UX_LIFECYCLE.md`.
- Updated `docs/ux/00_INDEX.md` status snapshot totals for new spec-only parity affordances.

## [0.81.4.post1] - 2026-05-22

### Code Changes

#### Changed

- Added a new multi-session execution plan document for hybrid UX parity work:
  - `docs/hybrid_ux_parity_execution_plan.md`
- Plan includes explicit status markers per step (`[ ]`, `[/]`, `[X]`), demo-pass completion criteria, and a required session handoff protocol for reliable cross-session execution.

### Test Changes

#### Changed

- No code/test logic changes in this release; documentation-only planning update.

## [0.81.4] - 2026-05-22

### Code Changes

#### Fixed

- Updated split DemoMode pane orchestration to capture tmux pane IDs (`-P -F #{pane_id}`) for stories, live-core, and controller panes, then apply titles/focus by pane ID rather than positional indexes.
- Ensures controller pane is deterministically the active pane at demo attach, fixing cases where cursor focus landed in output pane due to pane-index drift.

### Test Changes

#### Changed

- Updated split-mode fake tmux harness and assertions for pane-id-based split/selection commands.

## [0.81.3] - 2026-05-22

### Code Changes

#### Fixed

- Updated split DemoMode pre-attach focus behavior so the live-core chat pane is focused in the mirrored right session, reducing right-side input cursor dominance and preserving left/bottom controller as the operator interaction focus.

### Test Changes

#### Changed

- Updated split-mode tests to assert live-core chat pre-focus before nested attach.

## [0.81.2] - 2026-05-22

### Code Changes

#### Fixed

- Updated split DemoMode stories pane from passive tail rendering to pager-backed rendering so arrow keys and page keys scroll directly in the top pane.
- Added stories-pane refresh hook (`R` key injection) when status-board content is rewritten, keeping visible content aligned with live test status updates.
- Updated lower controller-pane navigation instructions to match direct scrolling controls.

### Test Changes

#### Changed

- Updated split-mode tests to assert pager-based stories pane command.
- Updated harness tests for revised stories navigation guidance.

## [0.81.1] - 2026-05-22

### Code Changes

#### Fixed

- Updated DemoMode pending status marker from `[S]` to `[ ]` for not-yet-executed tests.
- Moved stories-pane navigation instructions into the lower controller pane as the primary operator guidance.
- Refined split controller pane clearing to use tmux-native clear/reset commands (`clear-history`, `send-keys -R`, `C-u`) with ANSI fallback to reduce resize-related display artifacts.

### Test Changes

#### Changed

- Updated DemoMode harness tests for `[ ]` pending marker expectations.
- Updated split-view controller tests to assert in-pane navigation guidance and non-duplicated sequence rendering.

## [0.81.0] - 2026-05-22

### Code Changes

#### Added

- Expanded DemoMode story manifest from 3 to 6 E2E stories, including harness-focused stories (jump semantics, diagnostics capture behavior, controller readability).
- Added split-controller internal wiring for a shared stories board file (`--demo-split`, `--demo-stories-file`) so top-pane content and controller progress stay synchronized.

#### Changed

- Top-left split pane now renders a status board with inline markers next to each test:
  - `[S]` pending/skip
  - `[/]` active
  - `[P]` pass
  - `[X]` fail
- Story browser content is isolated to use-cases/status (no duplicate command-loop output from controller).
- Bottom-left controller pane now clears/refreshes between decision steps to reduce muddled prompt history.
- Split stories pane now tails a board file so status updates remain visible while preserving tmux copy-mode scrolling.

### Test Changes

#### Added

- Added `demo_harness_test.go` coverage for split-view controller suppression/refresh behavior.
- Added `demo_harness_test.go` coverage for status-marker rendering in story board output.

#### Changed

- Updated split-mode tests for new internal split flags and stories-board file handoff.
- Updated readiness test start selector to align with expanded 6-test demo manifest.

## [0.80.0] - 2026-05-22

### Code Changes

#### Added

- Implemented DemoMode split-left workspace in `agentx --demo`:
  - left-top `stories` pane renders ordered Gherkin use-cases
  - left-bottom `controller` pane runs the prompt loop
  - right `live-core` pane remains attached to the running application
- Added `make` target `test-demo-split-layout-headless` for dedicated split-layout UX validation.

#### Changed

- Updated `verify-tmux-layout` to include split-demo headless validation.
- Reconciled UX lifecycle/index documentation to mark `PD-17-AF-004` as implemented.

### Test Changes

#### Added

- Added `tests/test_demo_split_layout_headless.sh` to validate DemoMode split pane titles, geometry, and active controller-pane affordance.

#### Changed

- Updated split-mode command assertions in `demo_split_mode_test.go` for controller command invocation under `go test` dynamic binary paths.
- Updated `tests/test_tmux_layout_headless.md` to document the new split-layout contract test.

## [0.79.19] - 2026-05-22

### Code Changes

#### Added

- Enhanced DemoMode decision prompt commands:
  - `N` run next
  - `J <test number>` jump ahead to a specific test during the same run
  - `X <feedback>` fail current test with optional inline user feedback

#### Changed

- Demo diagnostics collector now stores inline failure feedback in artifacts (`metadata.json` and `demo_feedback.txt`).
- Demo summary now includes captured feedback when a run is failed.

### Test Changes

#### Added

- Added coverage for jump-ahead command behavior and inline failure-feedback propagation.

#### Changed

- Updated decision prompt/validation tests for the new command grammar.

## [0.79.18] - 2026-05-22

### Code Changes

#### Fixed

- Suppressed core runtime logger output during interactive split DemoMode attach to prevent core log bleed into the left controller pane.

## [0.79.17] - 2026-05-22

### Code Changes

#### Fixed

- Fixed left-pane whitespace bleed in controller result lines by removing carriage-return (`\r`) prefix from DemoMode PASS/FAIL output rendering.

## [0.79.16] - 2026-05-22

### Code Changes

#### Fixed

- Reduced left-pane wrap/whitespace bleed by shortening controller run-step and Gherkin expectation lines.
- Updated `:clear` live-core behavior to avoid visible control-glyph artifacts in pane output:
  - use tmux pane reset (`send-keys -R`) after history clear instead of `C-l`.

### Test Changes

#### Changed

- Updated input command contract tests to assert tmux reset-based clear behavior.

## [0.79.15] - 2026-05-22

### Code Changes

#### Fixed

- Improved DemoMode controller output formatting by sanitizing result text to a single concise line, reducing wrap/indent artifacts in the left pane.
- Fixed `:clear` command behavior in live core panes:
  - no longer sends literal `clear` text into the Chat pane.
  - now clears live-core Chat and Input pane history/display using tmux control commands.

#### Changed

- Updated Gherkin expectation text to use explicit live-core pane terminology (`Chat`, `Context`, `Input`) for clearer UAT assertions.

### Test Changes

#### Changed

- Updated input command contract tests to validate tmux-based clear behavior and prevent regression to literal `clear` text injection.

## [0.79.14] - 2026-05-22

### Code Changes

#### Changed

- Refined DemoMode left-pane readability with structured Gherkin sequence output (`GIVEN/WHEN/THEN`) to reduce noisy wrapped lines and clarify expected assertions.
- Updated split-pane sizing to keep more width available for the controller pane.

#### Added

- Final E2E shutdown behavior now closes the live core session on successful `e2e-003` (`:q`) in controller mode.
- Right pane now shows explicit completion guidance after the live session exits:
  - `"[AgentX Demo] demo complete. Press N or X to exit"`

### Test Changes

#### Changed

- Updated split-mode tests for weighted split command and right-pane completion guidance script.
- Updated demo harness tests for Gherkin sequence rendering.

## [0.79.13] - 2026-05-22

### Code Changes

#### Changed

- Polished split DemoMode left-pane readability by biasing the split width toward the controller pane (`split-window -h -p 45`) to reduce hard wraps.

#### Added

- Added explicit per-test status ledger in DemoMode summary:
  - `N` accepted test => `PASS`
  - `X` marked test => `FAIL`
  - non-executed tests (including pre-start or trailing tests) => `SKIP`

### Test Changes

#### Changed

- Updated split-mode command assertions for weighted controller-pane split.
- Added/updated demo harness tests for PASS/FAIL/SKIP status-ledger semantics.

## [0.79.12] - 2026-05-22

### Code Changes

#### Fixed

- Balanced split DemoMode nested live view to keep system/context visibility while preserving input visibility:
  - normalize core window to `tiled` layout before nested attach in split mode.
  - retain minimum input pane height enforcement and input-pane focus.

### Test Changes

#### Changed

- Updated split-mode tests to assert core tiled-layout normalization before nested attach.

## [0.79.11] - 2026-05-22

### Code Changes

#### Fixed

- Hardened split DemoMode live-core presentation so input is visible on entry:
  - enforce minimum input pane height of 3 rows before nested attach.
  - focus the input pane before opening the right-pane nested client.

### Test Changes

#### Changed

- Updated split-mode tests to validate minimum input height enforcement and input-pane focus before nested attach.

## [0.79.10] - 2026-05-22

### Code Changes

#### Added

- Enhanced DemoMode failure diagnostics (`X` path) to capture pane/window artifacts across both sessions:
  - `core` session (live app panes)
  - `split` session (controller + nested live pane)
- Added expanded capture depth (`capture-pane -S -200`) for richer screen-scrape context.

#### Changed

- Preserved legacy artifact filenames while introducing prefixed multi-session artifacts for better traceability.

### Test Changes

#### Changed

- Updated diagnostics unit test coverage for multi-session tmux artifact capture.
- Updated headless demo smoke test to validate pane artifacts dynamically from pane metadata instead of fixed pane IDs.

## [0.79.9] - 2026-05-22

### Code Changes

#### Fixed

- Normalized core tmux window state before opening split DemoMode nested live view:
  - force core window sizing policy to `smallest` so the nested client adapts to split-pane dimensions.
  - clear accidental zoomed window state before attach.
  - verify input pane height target exists and remains visible in the live pane session.

### Test Changes

#### Changed

- Expanded split-mode tmux orchestration tests to cover pre-attach core-window normalization commands.

## [0.79.8] - 2026-05-22

### Code Changes

#### Fixed

- Switched split DemoMode right pane from scripted snapshot rendering to a native nested tmux attach (`TMUX` unset, read-only attach).
- This makes the right side display the actual live application panes instead of synthesized mirror frames.

### Test Changes

#### Changed

- Updated split-mode tests to assert native read-only nested attach command wiring.

## [0.79.7] - 2026-05-22

### Code Changes

#### Fixed

- Polished split DemoMode interaction and shutdown UX:
  - per-test `N/X` decision prompt now always starts on a new line so cursor placement is deterministic.
  - split-session cleanup now uses captured tmux kill output, suppressing noisy `can't find session` terminal bleed when the demo session is already closed.

## [0.79.6] - 2026-05-22

### Code Changes

#### Fixed

- Improved split DemoMode right-pane live display readability:
  - replaced hardcoded chat/context mapping with pane-index snapshot rendering sourced from tmux pane metadata.
  - mirror now renders concise non-empty pane tails and avoids mislabeled pane content.

### Test Changes

#### Changed

- Updated split-mode tests to assert pane-index snapshot mirror behavior and newline-decoded frame rendering.

## [0.79.5] - 2026-05-22

### Code Changes

#### Fixed

- Fixed split DemoMode interruption behavior:
  - decision prompt now respects cancellation context, so `Ctrl-C` exits the controller loop instead of re-prompting indefinitely.
  - controller interruption/error path now attempts to close the full split tmux session before returning.

### Test Changes

#### Added

- Added cancellation regression coverage in `demo_harness_test.go` to ensure demo decision loop exits on cancelled context.

## [0.79.4] - 2026-05-22

### Code Changes

#### Fixed

- Fixed DemoMode completion behavior after `X` in split mode:
  - controller now closes the whole demo split session on completion so tmux does not expand the remaining mirror pane.
- Fixed right-pane mirror rendering quality:
  - pane titles are resolved dynamically (`chat` / `context`) instead of relying on fixed pane indexes.
  - mirror frames now render true newlines (no literal `\n` text output).

### Test Changes

#### Changed

- Updated split-mode tests to assert title-based pane resolution and newline-safe mirror frame generation.

## [0.79.3] - 2026-05-22

### Code Changes

#### Fixed

- Fixed DemoMode right-pane mirror UX instability:
  - removed fast clear/redraw loop that caused flashing and made screen capture difficult.
  - switched to change-detected live monitoring for chat/context captures to keep the pane stable.

### Test Changes

#### Changed

- Updated split-mode mirror command assertions to match the stable change-detected monitoring loop.

## [0.79.2] - 2026-05-22

### Code Changes

#### Fixed

- Fixed DemoMode diagnostics capture quality:
  - session-scoped pane capture now targets only the active demo core session.
  - `tmux display-message` invocation now uses a valid target argument ordering.
- Improved the split DemoMode right pane:
  - live-core mirror now renders all primary core panes (chat/context/input) instead of a single clipped pane snapshot.

### Test Changes

#### Changed

- Updated split-pane orchestration assertions to validate multi-pane live-core mirror command behavior.

### Documentation Changes

#### Changed

- Refreshed DemoMode notes to reflect multi-pane live-core mirror behavior and improved diagnostics scope.

## [0.79.1] - 2026-05-22

### Code Changes

#### Fixed

- Fixed the DemoMode split-pane right side so it mirrors the live core session instead of launching a nested tmux attach that collapsed the outer split.

### Test Changes

#### Changed

- Updated split-pane orchestration coverage to assert the right pane runs a passive live-core mirror loop instead of a nested tmux client.

### Documentation Changes

#### Changed

- Reworded the DemoMode contract and user-facing notes to describe the right pane as a live-core mirror pane.

## [0.79.0] - 2026-05-22

### Code Changes

#### Added

- Added a split-pane interactive DemoMode controller:
  - `agentx --demo` now opens an outer tmux session with a left controller pane and a right live-core pane.
  - the controller submits test prompts to the live core via its `/submit` endpoint so the user can watch the real app respond while reviewing the sequence.
  - `--demo-headless` preserves the internal non-interactive smoke path for deterministic artifact checks.

### Test Changes

#### Added

- Added split-pane orchestration coverage in `cmd/agentx-core/demo_split_mode_test.go`:
  - GIVEN a live core and fake tmux WHEN split demo mode launches THEN the controller pane and live-core attach pane are created.
  - GIVEN a live health endpoint WHEN the controller submits a prompt THEN the routed response is returned.

### Documentation Changes

#### Changed

- Updated the DemoMode contract, lifecycle matrix, migration notes, and README to describe the split-pane controller layout and hidden headless smoke path.

### Build and Test Changes

#### Changed

- `make demo-smoke` now exercises `--demo-headless` so the smoke gate remains deterministic while the user-facing `--demo` path becomes split-pane interactive.

## [0.78.0] - 2026-05-22

### Code Changes

#### Added

- Added a headless DemoMode smoke gate:
  - `tests/test_demo_smoke_headless.sh` runs the built binary in `--demo` mode against a fake tmux executable.
  - verifies `X` writes deterministic diagnostics artifacts under `logs/demo/<session>/<test>/`.
  - `Makefile` target `demo-smoke` invokes the smoke gate after building `bin/agentx`.

### Documentation Changes

#### Changed

- Updated DemoMode UX / migration docs and README to reflect D4 completion and the new `make demo-smoke` workflow:
  - `docs/ux/00_INDEX.md`
  - `docs/ux/03_PANEL_DETAILS.md`
  - `docs/ux/07_DEMO_MODE.md`
  - `docs/ux/UX_LIFECYCLE.md`
  - `docs/HYBRID_MIGRATION_PLAN.md`
  - `README.md`

### Test Changes

#### Added

- Added the DemoMode headless smoke test script coverage for the artifact bundle path and required log files.

## [0.77.0] - 2026-05-22

### Code Changes

#### Added

- Implemented DemoMode D3 failure diagnostics capture in Go core:
  - `cmd/agentx-core/demo_harness.go`
    - added diagnostics collector flow on `X` decisions,
    - persists deterministic artifacts under `logs/demo/<session>/<test>/`,
    - captures metadata, tmux pane listings, display-message snapshot, and pane capture outputs,
    - reports artifact paths in end-of-run summary.
  - `cmd/agentx-core/main.go`
    - added `--demo-tmux-session` (or `AGENTX_DEMO_TMUX_SESSION`) to target diagnostics capture against a specific tmux session.

### Test Changes

#### Added

- Expanded DemoMode unit coverage in `cmd/agentx-core/demo_harness_test.go`:
  - GIVEN custom diagnostics collector WHEN user enters `X` THEN artifact path is shown in run summary.
  - GIVEN fake tmux executable WHEN default diagnostics collector runs THEN expected artifact files are created.

### Documentation Changes

#### Changed

- Updated UX and architecture docs to mark DemoMode D3 complete and PD-17 fully implemented:
  - `docs/ux/00_INDEX.md`
  - `docs/ux/03_PANEL_DETAILS.md`
  - `docs/ux/07_DEMO_MODE.md`
  - `docs/ux/UX_LIFECYCLE.md`
  - `docs/architecture/HYBRID_ARCHITECTURE.md`
  - `docs/HYBRID_MIGRATION_PLAN.md`

## [0.76.0] - 2026-05-22

### Code Changes

#### Added

- Implemented DemoMode D2 interactive review loop in Go core:
  - `cmd/agentx-core/demo_harness.go`
    - runs selected demo tests in sequence from `--demo-start` selector,
    - prompts for per-test user feedback (`N`/`X`) after each test,
    - re-prompts on invalid input,
    - prints end-of-run readiness summary with artifact-path placeholder (`D3 diagnostics pending`).
  - `cmd/agentx-core/main.go`
    - `--demo` now executes the interactive DemoMode loop (instead of scaffold-only exit).

### Test Changes

#### Added

- Expanded DemoMode unit coverage in `cmd/agentx-core/demo_harness_test.go`:
  - GIVEN start selector by id WHEN user enters `X` THEN sequence stops and summary shows not-ready.
  - GIVEN invalid decision input WHEN prompt is shown THEN loop re-prompts until `N` or `X`.
  - GIVEN final-test start and `N` decision WHEN run completes THEN summary shows ready-for-UAT.

### Documentation Changes

#### Changed

- Updated DemoMode UX and architecture documentation to reflect D2 completion and remaining D3 diagnostics work:
  - `docs/ux/00_INDEX.md`
  - `docs/ux/03_PANEL_DETAILS.md`
  - `docs/ux/07_DEMO_MODE.md`
  - `docs/ux/UX_LIFECYCLE.md`
  - `docs/architecture/HYBRID_ARCHITECTURE.md`
  - `docs/HYBRID_MIGRATION_PLAN.md`

## [0.75.0] - 2026-05-22

### Code Changes

#### Added

- Added DemoMode D1 CLI scaffolding in Go core:
  - `cmd/agentx-core/main.go`: new `--demo` and `--demo-start` flags with validation (`--demo-start` requires `--demo`).
  - `cmd/agentx-core/demo_harness.go`: stable ordered demo sequence manifest, start-selector resolution (id or 1-based index), and scaffold output rendering.

### Test Changes

#### Added

- Added `cmd/agentx-core/demo_harness_test.go` unit coverage for D1 contracts:
  - GIVEN empty selector WHEN resolving start THEN first test is selected.
  - GIVEN id selector WHEN resolving start THEN matching test index is selected.
  - GIVEN 1-based index selector WHEN resolving start THEN normalized zero-based index is selected.
  - GIVEN invalid selector WHEN resolving start THEN error is returned.
  - GIVEN `--demo-start` with scaffolding WHEN rendering output THEN sequence heading and selected start marker are shown.

### Documentation Changes

#### Changed

- Updated DemoMode UX and architecture docs to reflect implemented D1 behavior and remaining planned phases D2-D4:
  - `docs/ux/07_DEMO_MODE.md`
  - `docs/ux/03_PANEL_DETAILS.md`
  - `docs/ux/UX_LIFECYCLE.md`
  - `docs/ux/00_INDEX.md`
  - `docs/architecture/HYBRID_ARCHITECTURE.md`
  - `docs/HYBRID_MIGRATION_PLAN.md`

## [0.74.4.post2] - 2026-05-22

### Documentation Changes

#### Added

- Added `docs/ux/07_DEMO_MODE.md` as the authoritative DemoMode UX contract and implementation plan.
  - defines `agentx --demo` and `--demo-start <id-or-index>` command contract (planned).
  - defines per-test feedback semantics (`N`/`X`) at the end of each test.
  - defines deterministic pane-dump diagnostics artifact contract on `X`.

#### Changed

- Updated UX authority docs to make DemoMode concrete and traceable:
  - `docs/ux/00_INDEX.md`
  - `docs/ux/README.md`
  - `docs/ux/03_PANEL_DETAILS.md`
  - `docs/ux/02_USER_FLOWS.md`
  - `docs/ux/UX_LIFECYCLE.md`
- Added `PD-17 DemoMode` affordance mapping (`PD-17-AF-001..006`) as spec-only rows in lifecycle traceability matrix.
- Updated migration and architecture docs with explicit pre-UAT DemoMode plan and CLI contract notes:
  - `docs/HYBRID_MIGRATION_PLAN.md`
  - `docs/architecture/HYBRID_ARCHITECTURE.md`

## [0.74.4.post1] - 2026-05-22

### Documentation Changes

#### Changed

- Updated `.github/copilot-instructions.md` to define the build/clean contract:
  - `make build` must remain the canonical wrapper for the most complete supported build sequence.
  - narrower build targets may exist for targeted workflows, but they must not replace the default full build.
  - `make clean` must be updated whenever new build artifacts are introduced so the workspace returns to a clean post-build state.
- Added the expectation that build/clean contract changes stay synchronized with `Makefile`, `CHANGELOG.md`, and project versioning.

## [0.74.4] - 2026-05-22

### Code Changes

#### Fixed

- Sanitized interactive pane UX output so user-facing panes no longer include startup command traces or IPC diagnostics.
  - `cmd/agentx-core/core.go`:
    - pane applets now launch via `tmux respawn-pane` instead of shell `send-keys` command injection.
    - core-to-pane direct rendering is now opt-in via `AGENTX_PANE_RENDER_MODE=core` (used in fake-tmux tests), with runtime default delegated to pane applets.
  - `applets/template.py`:
    - input/chat/context pane loops now emit plain, sanctioned UX lines.
    - non-bridge interactive panes clear startup noise on entry and suppress READY/IPC diagnostics.
    - fixed `SyntaxWarning` path by removing `return` from `finally` block.

### Test Changes

#### Added

- Added headless pane-affordance UX contract test: `tests/test_tmux_pane_affordances_headless.sh`.
  - GIVEN a live headless core WHEN a prompt is sent through the input pane THEN chat/context panes must show sanctioned outputs and exclude operational noise.
- Added `@e2e` GoDog scenario in `cmd/agentx-core/features/e2e.feature` for prompt-routing flow with response + persisted context assertions.

#### Changed

- Updated startup integration expectations in `cmd/agentx-core/features/integration.feature` to reflect clean initialization without placeholder `send-keys` injection.
- Updated fake-tmux harnesses (`cmd/agentx-core/core_chat_pipeline_test.go`, `cmd/agentx-core/godog_test.go`) to enable deterministic core-render mode only in test environments.
- Updated `tests/test_tmux_layout_headless.sh` and `tests/test_tmux_layout_headless.md` to align with clean-pane startup and expanded UX contract validation.

### Documentation Changes

#### Changed

- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` with explicit interactive-pane output contract and headless validation coverage.
- Updated `docs/HYBRID_MIGRATION_PLAN.md` with clean-pane contract milestone and pane-affordance test coverage.
- Updated `Makefile` verification flow to include `test-tmux-pane-affordances-headless` and run `build-core` before gate checks.

## [0.74.3] - 2026-05-22

### Code Changes

#### Fixed

- Fixed pane affordance behavior so TUI panes expose role-appropriate UX instead of only passive logs.
  - `cmd/agentx-core/core.go`: added `/submit` endpoint plumbing via `ContextManager` submit provider.
  - `applets/template.py`: added role-specific non-bridge loops:
    - `input` pane: line-based submit workflow posting prompts to `/submit`.
    - `chat` pane: `/context` polling and agent output display.
    - `context` pane: `/context` polling for turn metadata display.

### Test Changes

#### Added

- Added submit endpoint coverage in `cmd/agentx-core/core_health_endpoint_test.go`:
  - successful prompt submit routing,
  - request validation failures,
  - method constraint enforcement.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` with interactive pane affordance milestone.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to describe role-specific pane behavior and submit endpoint flow.

## [0.74.2] - 2026-05-22

### Code Changes

#### Fixed

- Fixed TUI pane startup behavior in `cmd/agentx-core/core.go`:
  - `StartAppletSupervisor` now launches live applet processes into the primary panes (`chat`, `context`, `input`) when tmux is initialized.
  - Pane launch is best-effort and skipped when the template applet script is unavailable.

### Test Changes

#### Added

- Added `TestStartAppletSupervisor_LaunchesPaneAppletProcesses` in `cmd/agentx-core/core_applet_supervisor_test.go`.
  - GIVEN initialized tmux and template applet WHEN supervisor starts THEN pane applet launch commands are sent for chat/context/input.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` with pane-process launch milestone.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to document supervisor pane applet startup behavior.

## [0.74.1] - 2026-05-22

### Code Changes

#### Fixed

- Updated launch UX in `cmd/agentx-core/main.go` so `./bin/agentx` attaches to the tmux TUI by default.
- Headless operation remains available via `-attach=false` for automation or non-interactive runs.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` to note the default attach launch behavior.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to document user-facing launch semantics.

## [0.74.0] - 2026-05-22

### Code Changes

#### Added

- Expanded backend streaming edge-case coverage in:
  - `cmd/agentx-core/core_phase2_chat_bridge_test.go`
  - `cmd/agentx-core/features/integration.feature`
  - `cmd/agentx-core/godog_test.go`

### Test Changes

#### Added

- Added route-level test: empty chunk frames are ignored while final response persists and `bridge_response_ok` is emitted without `bridge_chunk` events.
- Added parser-level tests for deterministic stream semantics:
  - duplicate response frames use the first response as authoritative,
  - malformed frames are ignored when valid frames follow,
  - late error frame after terminal response is ignored within that parse cycle.
- Added GoDog integration scenario for empty-chunk behavior with observability assertions (`bridge_response_ok` present; `bridge_chunk`/stream render absent).
- Added BDD fixture support for empty-chunk bridge applet.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` with backend-specific streaming edge-case milestone.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` with parser-level duplicate/late-frame handling notes.

## [0.73.0] - 2026-05-22

### Code Changes

#### Added

- Expanded fault-injection coverage for bridge protocol handling in:
  - `cmd/agentx-core/core_phase2_chat_bridge_test.go`
  - `cmd/agentx-core/features/integration.feature`
  - `cmd/agentx-core/godog_test.go`

### Test Changes

#### Added

- Added unit tests for bridge protocol fault permutations:
  - malformed JSON frames are tolerated and successful responses still render without fallback.
  - explicit bridge error frames trigger fallback on first request and recovery on subsequent request.
- Added GoDog integration scenarios for:
  - malformed frame tolerance with `bridge_response_ok` and no fallback event.
  - error-frame fallback followed by restart/recovery with ordered lifecycle event assertions.
- Added GoDog fixture helpers for malformed and error-frame bridge applets and a negative tmux assertion step (`tmux commands should not include ...`).

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` with completed malformed/error-frame fault-injection milestone.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` with bridge frame fault-handling observability coverage.

## [0.72.0] - 2026-05-22

### Code Changes

#### Added

- Expanded BDD observability coverage and helpers in:
  - `cmd/agentx-core/features/integration.feature`
  - `cmd/agentx-core/godog_test.go`

### Test Changes

#### Added

- Added integration scenario: `Bridge lifecycle events reflect timeout fallback and subsequent recovery`.
  - GIVEN a flaky bridge applet and tight timeout WHEN first prompt routes THEN timeout/fallback events are emitted with deterministic fallback response.
  - GIVEN immediate second prompt WHEN bridge restarts THEN recovery response is emitted and lifecycle sequence assertions pass.
- Added GoDog helper steps for observability assertions:
  - Configure core to use prepared bridge script with timeout.
  - Assert tmux command snippets ordering.
  - Assert tmux command occurrence counts (at least N times).
- Added flaky bridge applet fixture that persists first-time timeout state across process restarts to faithfully validate restart behavior.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` with completed lifecycle-sequencing observability milestone.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to document timeout/fallback/restart/recovery sequencing validation.

## [0.71.0] - 2026-05-22

### Code Changes

#### Added

- Added richer context-pane fidelity verification logic across unit and integration BDD coverage.

### Test Changes

#### Added

- Added unit tests in `cmd/agentx-core/core_context_persistence_test.go`:
  - `TestTrimForPaneSummary_BoundsLength` validates bounded truncation behavior for context summaries.
  - `TestRouteInputPrompt_ContextSummaryOrderingAndTruncation` validates multi-turn context summary ordering and truncation markers in tmux output.
- Expanded GoDog integration coverage:
  - New scenario in `cmd/agentx-core/features/integration.feature` validates context-pane summary ordering (`turn=1` before `turn=2`) and bounded formatting signal (`...`).
  - New GoDog step in `cmd/agentx-core/godog_test.go`: `tmux command snippet "..." should appear before "..."`.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` with completed context-pane ordering/bounded-format assertion milestone.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to reflect summary truncation and ordering validation coverage.

## [0.70.0] - 2026-05-22

### Code Changes

#### Added

- Expanded GoDog integration coverage artifacts for Phase 2 bridge streaming behavior in:
  - `cmd/agentx-core/features/integration.feature`
  - `cmd/agentx-core/godog_test.go`

### Test Changes

#### Added

- Added integration scenario: `Python bridge streaming renders chunks and persists final turn`.
  - GIVEN template chat bridge streaming output WHEN routing a prompt THEN tmux commands include assistant stream rendering and final assistant response rendering.
  - GIVEN completed routing WHEN context turns snapshot is captured THEN persisted turn includes expected prompt and expected response.
- Added GoDog step assertion for persisted response content: `context turns should include response "..."`.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` to mark BDD streaming assertions milestone complete.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` with BDD coverage statement for streaming + persistence integrity.

## [0.69.0] - 2026-05-22

### Code Changes

#### Added

- Hardened mid-stream cancellation semantics in `cmd/agentx-core/core.go`.
  - Cancellation/deadline errors from bridge routing are now propagated (no deterministic echo fallback on canceled contexts).
  - Cancellation telemetry events are emitted with a live background context so observability is preserved even when request context is canceled.

### Test Changes

#### Added

- Added `TestRouteInputPrompt_CanceledMidStreamRecoversOnImmediateRetry` in `cmd/agentx-core/core_phase2_chat_bridge_test.go`.
  - GIVEN chunked bridge output with delayed completion WHEN first route is canceled mid-stream THEN cancellation error is returned and no turn is persisted.
  - GIVEN immediate retry after cancellation WHEN prompt is routed again THEN bridge restarts and successful response is rendered/persisted.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` with cancellation/retry hardening milestone completion.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to document cancellation propagation and bridge restart behavior.

## [0.68.0] - 2026-05-22

### Code Changes

#### Added

- Added context-pane fidelity updates in `cmd/agentx-core/core.go`.
  - `RouteInputPrompt` now emits a compact per-turn context summary after successful turn persistence.
  - New context summary output format: `[context] turn=N prompt=... response=...`.

### Test Changes

#### Added

- Extended `cmd/agentx-core/core_context_persistence_test.go` to assert context-pane update rendering.
  - GIVEN successful prompt routing WHEN turn is persisted THEN tmux commands include context-pane summary output.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` with completed context-pane summary milestone.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to document post-persistence context-pane updates.

## [0.67.0] - 2026-05-22

### Code Changes

#### Added

- Added structured bridge lifecycle logging to logs pane in `cmd/agentx-core/core.go`.
  - Bridge now emits `[bridge] event=...` records for start, chunk receipt, response success, timeout, request/response errors, and fallback.
  - Logging path is best-effort (does not block prompt routing on log render failures).

### Test Changes

#### Added

- Extended `cmd/agentx-core/core_phase2_chat_bridge_test.go` to assert logs-pane bridge lifecycle events.
  - GIVEN streaming Ollama backend routing WHEN prompt runs THEN `bridge_start`, `bridge_chunk`, and `bridge_response_ok` records are emitted.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot to include logs-pane bridge lifecycle instrumentation.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to document bridge event mirroring into logs pane.

## [0.66.0] - 2026-05-21

### Code Changes

#### Added

- Replaced simulated chunk generation for Ollama backend with true backend streaming in `applets/template.py`.
  - Bridge now consumes streamed `/api/chat` JSON lines from Ollama when `AGENTX_CHAT_BACKEND=ollama`.
  - Streamed deltas are emitted as bridge `chunk` events in real-time before final `response` envelope.
  - Existing deterministic fallback path remains for unavailable/invalid Ollama responses.

### Test Changes

#### Added

- Extended `cmd/agentx-core/core_phase2_chat_bridge_test.go` with backend-stream integration coverage.
  - GIVEN a fake streaming Ollama endpoint WHEN prompt routing runs with `AGENTX_CHAT_BACKEND=ollama` THEN streamed chunk lines and final consolidated response are rendered.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot to record true backend-sourced streaming chunks.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to reflect bridge chunk/response protocol in backend streaming flow.

## [0.65.0] - 2026-05-21

### Code Changes

#### Added

- Added chunked bridge response protocol support for hybrid chat path.
  - `applets/template.py` now emits `{"type":"chunk","delta":"..."}` events before final `response` in bridge modes.
  - `cmd/agentx-core/core.go` now parses chunk events and renders incremental stream lines to chat pane while preserving final consolidated response handling.

### Test Changes

#### Added

- Extended `cmd/agentx-core/core_phase2_chat_bridge_test.go` with stream rendering coverage.
  - GIVEN chunk-emitting bridge responses WHEN prompt is routed THEN stream chunk lines and final consolidated assistant response are rendered.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` to reflect streamed bridge response handling in Phase 2.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` bridge message contract to include `chunk` events.

## [0.64.0] - 2026-05-21

### Code Changes

#### Added

- Added bounded response wait handling for persistent chat bridge in `cmd/agentx-core/core.go`.
  - Introduced `AGENTX_CHAT_BRIDGE_RESPONSE_TIMEOUT_SEC` configuration for bridge response timeout control.
  - Prompt routing now times out and tears down hung bridge process instead of blocking indefinitely.
  - Public prompt route path continues to fall back to deterministic handler when bridge timeout occurs.

### Test Changes

#### Added

- Extended `cmd/agentx-core/core_phase2_chat_bridge_test.go` with timeout reliability coverage.
  - GIVEN a hanging bridge process WHEN direct bridge route is called THEN timeout error is returned.
  - GIVEN a hanging bridge process WHEN `RouteInputPrompt` is called THEN deterministic fallback response is returned.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot for bounded bridge response waits.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` env var contract to include bridge response timeout control.

## [0.63.0] - 2026-05-21

### Code Changes

#### Added

- Added initial LLM-backed response capability for persistent hybrid chat bridge in `applets/template.py`.
  - Introduced backend selection via `AGENTX_CHAT_BACKEND` (`echo` or `ollama`).
  - Added Ollama request path using `AGENTX_OLLAMA_HOST` and `AGENTX_OLLAMA_MODEL` in bridge server mode.
  - Added resilient deterministic fallback to `Echo:` responses if Ollama is unavailable.
- Updated `cmd/agentx-core/core.go` to pass chat backend/model/host settings into the Python bridge process environment.

### Test Changes

#### Added

- Extended `cmd/agentx-core/core_phase2_chat_bridge_test.go` with fallback coverage.
  - GIVEN `AGENTX_CHAT_BACKEND=ollama` with unreachable host WHEN prompt routing occurs THEN deterministic echo fallback is returned without routing failure.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot to reflect initial LLM-backed bridge support with safe fallback behavior.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` revision stamp and environment-variable contract for bridge backend selection.

## [0.62.0.post1] - 2026-05-21

### Documentation Changes

#### Changed

- Established a single authoritative migration status source in `docs/HYBRID_MIGRATION_PLAN.md`.
- Updated revision stamps and wording in hybrid docs to reflect current bridge mode naming.

#### Removed

- Removed stale duplicate branch status document `HYBRID_STATUS.md` to eliminate split-brain status drift.

## [0.62.0] - 2026-05-20

### Code Changes

#### Added

- Implemented persistent Phase 2 Python chat bridge lifecycle in `cmd/agentx-core/core.go`.
  - Chat routing now starts a long-lived `template.py --bridge-chat-server` process on first use.
  - Subsequent prompts reuse the same Python bridge process instead of spawning one process per prompt.
  - Added synchronized request/response handling and process teardown/recovery path on bridge failure.
- Updated `applets/template.py` with persistent server mode (`--bridge-chat-server`) while retaining one-shot mode.

### Test Changes

#### Added

- Extended `cmd/agentx-core/core_phase2_chat_bridge_test.go` with process reuse validation.
  - GIVEN template applet is staged WHEN two prompts are routed THEN the chat applet process PID remains stable.
- Added GoDog integration coverage in `cmd/agentx-core/features/integration.feature` and `cmd/agentx-core/godog_test.go`.
  - GIVEN template applet is staged WHEN two prompts are routed THEN tracked chat applet PID remains unchanged.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot to capture persistent bridge process reuse behavior.

## [0.61.0] - 2026-05-20

### Code Changes

#### Added

- Began Phase 2 chat integration by wiring a Python bridge path for chat prompt handling.
  - Updated `cmd/agentx-core/core.go` to route `chat` prompts through `applets/template.py --bridge-chat` when available.
  - Added resilient fallback to deterministic in-process routing if bridge script/runtime is unavailable.
  - Added bridge request/response parsing for JSONL stdin/stdout applet communication.
- Updated `applets/template.py` with `--bridge-chat` mode to process one prompt from stdin and return a JSON response.

### Test Changes

#### Added

- Added `cmd/agentx-core/core_phase2_chat_bridge_test.go`.
  - GIVEN a project with template applet available WHEN chat prompt is routed THEN Python bridge response is returned deterministically.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot to record Phase 2 bridge kickoff.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` capability checklist to include Python chat bridge handoff.

## [0.60.0.post1] - 2026-05-20

### Documentation Changes

#### Changed

- Aligned runtime documentation with implemented hybrid-branch behavior.
  - Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to reflect deterministic in-process routing and turn persistence via `/context`.
  - Removed stale claims that Python applet process launch/`READY` signaling is already active runtime behavior.
  - Updated endpoint examples to include current `/context` payload shape.
- Updated `README.md` Go-core section to clearly distinguish implemented deterministic runtime path from still-pending full Python applet/LLM wiring.

## [0.60.0] - 2026-05-20

### Code Changes

#### Added

- Implemented Sprint B4 merge-readiness gate workflow.
  - Added `hybrid-merge-gate` target in `Makefile` to enforce required pre-merge checks (`go-test`, `verify-tmux-layout`, `build-core`).
  - Added CI job `Hybrid Merge Readiness Gate` in `.github/workflows/hybrid-go-godog.yml` to run `make hybrid-merge-gate` on push/PR.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` with an authoritative B4 merge-readiness checklist and single gate command.
- Updated project version metadata for B4 delivery.

## [0.59.0] - 2026-05-20

### Code Changes

#### Added

- Implemented Sprint B3 minimal turn persistence in `cmd/agentx-core/core.go`.
  - Added persisted turn model and JSONL turn logging under session context directory.
  - Added core/context snapshot access for persisted turns.
  - Added `/context` endpoint returning `session_id`, `turn_count`, and persisted turns.
  - Wired prompt routing to persist completed user/assistant turns.

### Test Changes

#### Added

- Added persistence tests in `cmd/agentx-core/core_context_persistence_test.go`.
  - GIVEN a routed prompt WHEN route completes THEN turn is persisted and queryable via core snapshot.
  - GIVEN a reconstructed core with same session id WHEN turns are queried THEN persisted turns are reloaded.
- Extended endpoint tests in `cmd/agentx-core/core_health_endpoint_test.go` for `/context` payload and method constraints.
- Added integration scenario and steps in `cmd/agentx-core/features/integration.feature` and `cmd/agentx-core/godog_test.go`.
  - GIVEN routed prompt and reconstructed core WHEN context turns snapshot is captured THEN persisted prompt remains available.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot for delivered B3 persisted-turn behavior.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` capability checklist to include context turn persistence and context endpoint querying.

## [0.58.0] - 2026-05-20

### Code Changes

#### Added

- Implemented Sprint B2 input command contract in `cmd/agentx-core/core.go`.
  - Added `HandleInputLine` with command parsing for `:clear`, `:q`, and normal prompt forwarding.
  - Added deterministic input history tracking via `InputHistorySnapshot`.
  - Added exit-request state handling for quit command contract.

### Test Changes

#### Added

- Added focused command-dispatch tests in `cmd/agentx-core/core_input_contract_test.go`.
  - GIVEN `:clear` WHEN input line is handled THEN chat pane clear command is emitted.
  - GIVEN `:q` WHEN input line is handled THEN exit flag is set without prompt routing.
  - GIVEN normal prompt WHEN input line is handled THEN prompt forwarding and chat rendering remain intact.
  - GIVEN mixed command/prompt sequence WHEN inputs are handled THEN history ordering is deterministic.
- Added integration scenario coverage in `cmd/agentx-core/features/integration.feature` with step bindings in `cmd/agentx-core/godog_test.go`.
  - GIVEN fake tmux and applet supervisor WHEN clear/quit/prompt inputs are handled THEN command parsing and dispatch behave deterministically.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot to include delivered B2 input command contract behavior.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` capability checklist to include input command parsing and prompt forwarding support.

## [0.57.0] - 2026-05-19

### Code Changes

#### Added

- Implemented Sprint B1 prompt routing MVP in `cmd/agentx-core/core.go`.
  - Added `RouteInputPrompt` to route deterministic prompts through tracked chat applet handling.
  - Added chat-pane rendering via tmux send-keys (`[assistant] <response>` output contract).
  - Added deterministic default prompt handler wiring for supervised applets.
  - Added actionable failure behavior for missing chat applet and render/handler failures.

### Test Changes

#### Added

- Added deterministic unit coverage in `cmd/agentx-core/core_chat_pipeline_test.go`.
  - GIVEN initialized core and applet supervisor WHEN routing prompt `hello from input` THEN deterministic response is returned and rendered in chat pane.
  - GIVEN missing chat applet WHEN routing a prompt THEN deterministic registration error is returned.
- Added GoDog integration scenario in `cmd/agentx-core/features/integration.feature` and step bindings in `cmd/agentx-core/godog_test.go`.
  - GIVEN fake tmux bootstrap and applet supervisor WHEN routing input prompt THEN prompt pipeline completes and rendered response command is emitted.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot for delivered B1 prompt ingress MVP.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` capability checklist to include deterministic prompt routing/rendering support.

## [0.56.1] - 2026-05-19

### Test Changes

#### Changed

- Expanded headless tmux UX validation in `tests/test_tmux_layout_headless.sh`.
  - Added active primary window assertion (`0:tui-chat`).
  - Added logs window presence/inactive assertion (`1:logs`).
  - Added strict pane title/index assertions (`chat`, `context`, `input`) alongside geometry and placeholder checks.
- Updated `tests/test_tmux_layout_headless.md` to document the expanded active-window and pane-order checks.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot to reflect completed Sprint A4-style headless UX assertions.

## [0.56.0] - 2026-05-19

### Code Changes

#### Added

- Hardened applet supervision lifecycle in `cmd/agentx-core/core.go`.
  - Added tracked applet runtime states: `starting`, `ready`, `running`, `stopped`, `crashed`.
  - Added crash accounting updates on state transitions.
  - Updated health snapshots to surface pane/app applet state from runtime supervision data.
  - Updated supervisor bootstrap to seed tracked pane applets and shutdown to mark applets stopped.

### Test Changes

#### Added

- Added deterministic unit lifecycle tests in `cmd/agentx-core/core_applet_supervisor_test.go`.
  - GIVEN a new core WHEN applet supervisor starts THEN default pane applets are tracked as ready.
  - GIVEN a tracked applet WHEN it transitions through crashed then stopped THEN status and crash count are reflected in snapshots.
- Added functional GoDog scenario coverage in `cmd/agentx-core/features/functional.feature` with step bindings in `cmd/agentx-core/godog_test.go`.
  - GIVEN a tracked applet WHEN it is marked crashed THEN health snapshot reports `crashed` status and crash count.

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot to reflect applet lifecycle supervision implementation.
- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` to document lifecycle/crash visibility in health snapshot behavior.

## [0.55.0] - 2026-05-19

### Code Changes

#### Added

- Implemented runtime health endpoint payloads in `cmd/agentx-core/core.go`.
  - Added JSON handlers for `GET /health`, `GET /panes`, and `GET /applets`.
  - Added runtime snapshot model wiring from `AgentXCore` into `ContextManager`.
  - Added session metadata and snapshot provider hooks for deterministic health reporting.

### Test Changes

#### Added

- Added hermetic endpoint tests in `cmd/agentx-core/core_health_endpoint_test.go`.
  - GIVEN configured runtime snapshot WHEN `/health`, `/panes`, and `/applets` are queried THEN payloads include session and runtime state.
  - GIVEN unsupported HTTP method WHEN posting to `/health` THEN handler returns method-not-allowed.
- Added functional Gherkin coverage for health snapshots in `cmd/agentx-core/features/functional.feature` with step bindings in `cmd/agentx-core/godog_test.go`.

### Documentation Changes

#### Changed

- Updated `docs/architecture/HYBRID_ARCHITECTURE.md` health endpoint examples to match implemented payload shape.
- Updated `docs/HYBRID_MIGRATION_PLAN.md` status snapshot to reflect delivered Sprint A2 health endpoint work.

## [0.54.3.post1] - 2026-05-19

### Documentation Changes

#### Changed

- Updated `docs/HYBRID_MIGRATION_PLAN.md` with a current status snapshot and a concrete two-sprint execution board.
  - Added Sprint A (foundation stabilization and observability hardening) with acceptance criteria and validation gates.
  - Added Sprint B (chat/input vertical slice) with deterministic completion criteria and required test commands.

## [0.54.3] - 2026-05-19

### Code Changes

#### Fixed

- Fixed Go core tmux startup default-window behavior in `cmd/agentx-core/core.go`.
  - Session bootstrap now names window `0` as `tui-chat`.
  - Startup now re-selects window `0` after creating the `logs` window so attach lands on the primary UX surface.

### Test Changes

#### Changed

- Updated `cmd/agentx-core/core_layout_test.go` expected `new-session` command arguments to include `-n tui-chat`.
- Re-ran issue #10 regression suite; unit, hermetic integration, and GoDog integration scenarios now pass.

### Documentation Changes

#### Changed

- Updated `README.md` Go core run section to document that attached startup opens on `tui-chat` (window `0`) with logs in a background window.

## [0.54.2] - 2026-05-19

### Test Changes

#### Added

- Added unit regression coverage in `cmd/agentx-core/core_layout_test.go` for primary tmux window naming.
  - GIVEN tmux session startup command generation WHEN window arguments are built THEN window `0` must be named `tui-chat`.
- Added hermetic integration regression coverage in `cmd/agentx-core/core_tmux_startup_integration_test.go` using a fake `tmux` executable.
  - GIVEN core startup with command capture WHEN tmux bootstrap runs THEN startup names window `0` as `tui-chat` and re-selects window `0` after creating logs.
- Added integration Gherkin regression scenarios in `cmd/agentx-core/features/integration.feature` and step bindings in `cmd/agentx-core/godog_test.go`.
  - Added happy path command bootstrap scenario.
  - Added defect-path scenario asserting missing `tui-chat` startup naming.
  - Added boundary scenario asserting explicit selection of window `0` after logs creation.

#### Changed

- Updated GoDog integration step definitions to support hermetic command-capture assertions for tmux startup behavior.

## [0.54.1] - 2026-05-18

### Test Changes

#### Changed

- Wrapped long test docstrings in `tests/test_tui_bridge_output.py` and
  `tests/test_active_model_meter_wiring.py` to restore line-length lint compliance.
- Revalidated targeted TUI suites after docstring cleanup; all targeted tests pass.

## [0.54.0] - 2026-05-18

### Code Changes

#### Added

- Added TUI context visualization renderer for PD-16-AF-009 in `src/agentx/integration/tui_bridge.py`.
  - Added color-band main context bar rendering with ANSI segments.
  - Added Top Contributors rows with emoji labels and color-matched bars.
  - Added single-character ASCII fallback symbols for non-color terminals.
- Added session-side TUI context visualization publishing in `src/agentx/session.py` from meter redraw events.
  - Emits `###CONTEXT` blocks through the event broker as raw TUI records.
  - Includes deduplication to avoid re-emitting unchanged context snapshots.

### Test Changes

#### Added

- Added unit tests in `tests/test_tui_bridge_output.py` for PD-16-AF-009 rendering behavior.
  - GIVEN color-enabled rendering WHEN context is formatted THEN ANSI band colors and Top Contributors output are present.
  - GIVEN color-disabled rendering WHEN context is formatted THEN ASCII fallback uses single-character band symbols.
- Added unit test in `tests/test_active_model_meter_wiring.py` for session event publication deduplication.
  - GIVEN repeated meter redraws with identical payload WHEN schedule is called THEN a single TUI context event is published.

### Documentation Changes

#### Changed

- Updated `docs/ux/06_TUI_MIRROR.md` with as-built PD-16-AF-009 status and corrected PD-16 section structure.
- Updated `docs/ux/UX_LIFECYCLE.md` traceability matrix to mark PD-16-AF-009 as implemented and tested.

## [0.53.1.post3] - 2026-05-17

### Code Changes

#### Changed

- Updated `.gitignore` to ignore generated build and report artifacts:
  - `bin/`
  - `junit/test-results.xml*`

### Test Changes

#### Changed

- Stopped tracking generated junit report output file in git index to keep repository state deterministic across local/CI test runs.

## [0.53.1.post2] - 2026-05-17

### Code Changes

#### Changed

- Added Make-driven tmux layout verification flow in `Makefile` for consistent local and CI execution.
  - Added `go-test-pane-layout`, `test-tmux-layout-headless`, and `verify-tmux-layout` targets.
- Updated `.github/workflows/hybrid-go-godog.yml` tmux job to call `make verify-tmux-layout` after installing tmux and Go dependencies.

### Test Changes

#### Changed

- Standardized pane-layout unit tests and headless tmux UX checks behind a single Make target to reduce command drift.

## [0.53.1.post1] - 2026-05-17

### Code Changes

#### Changed

- Added CI workflow coverage for terminal UX layout validation in `.github/workflows/hybrid-go-godog.yml`.
  - Added a `tmux-layout-headless` job that installs tmux in CI, runs pane-layout unit tests, and executes `tests/test_tmux_layout_headless.sh`.

### Test Changes

#### Changed

- Enforced automated headless tmux layout checks in CI so pane-position regressions fail the pipeline.

## [0.53.1] - 2026-05-17

### Code Changes

#### Fixed

- Fixed tmux pane identity handling in `cmd/agentx-core/core.go` by replacing brittle pane-index assumptions with pane-ID capture from `split-window -P -F "#{pane_id}"`.
- Updated layout initialization to set pane titles and placeholders using captured pane IDs so context/input placement remains correct regardless of tmux pane reindexing.

### Test Changes

#### Added

- Added deterministic Go unit tests in `cmd/agentx-core/core_layout_test.go` for tmux command builders and pane-target mapping.
- Added headless terminal UX validation script in `tests/test_tmux_layout_headless.sh` that verifies pane geometry, placeholder content, and hidden logs window presence.

### Documentation Changes

#### Added

- Added `tests/test_tmux_layout_headless.md` to document programmatic tmux layout validation approach.

#### Changed

- Updated `.github/copilot-instructions.md` to require headless terminal/tmux UX layout validation and CI-failing assertions for layout drift.

## [0.53.0] - 2026-05-17

### Code Changes

#### Changed

- Refactored tmux pane layout in Go core to match UX design specification.
  - Top panes (80% height): Chat (80% width left) | Context (20% width right) with vertical split.
  - Bottom pane (20% height): Input spanning full width.
  - Logs moved to separate hidden window (navigable via `ctrl-b w` / window switcher).
  - Updated `InitializeTmuxSession()` in `cmd/agentx-core/core.go` to construct layout with explicit split sequence.
  - Updated `DefaultPaneLayout()` in `cmd/agentx-core/config.go` to document new layout proportions.

## [0.52.0] - 2026-05-17

### Code Changes

#### Added

- Added tmux attach mode for the Go core runtime.
  - Added `--attach` flag in `cmd/agentx-core/main.go` to auto-attach after startup.
  - Added `AttachTmuxSession` in `cmd/agentx-core/core.go` for interactive tmux client attachment.
  - Added `run-attached` target in `Makefile` for one-command startup + attach.

### Documentation Changes

#### Changed

- Updated `README.md` Go core run instructions with attached-mode workflows.

## [0.51.1.post1] - 2026-05-17

### Documentation Changes

#### Added

- Expanded `README.md` with a dedicated "Go Core (Hybrid) Build and Test" section.
  - Documented Make-based workflows for build, run, and split GoDog test suites.
  - Added direct Go command alternatives for test execution from both root and module paths.

## [0.51.1] - 2026-05-17

### Code Changes

#### Added

- Added repository root `Makefile` with consistent hybrid process targets.
  - Added build and run targets: `build`, `build-core`, `build-applets`, `run`, `run-with-applets`, `clean`.
  - Added Go test targets: `go-test`, `go-test-unit`, `go-test-integration`, `go-test-functional`, `go-test-e2e`.

### Test Changes

#### Changed

- Standardized local test invocation through Make targets so GoDog suite execution is consistent with CI split-suite behavior.

## [0.51.0] - 2026-05-17

### Code Changes

#### Added

- Added GoDog-based Gherkin testing foundation for the Go core in `cmd/agentx-core`.
  - Added tagged feature suites: `@unit`, `@integration`, `@functional`, `@e2e` in `cmd/agentx-core/features/`.
  - Added GoDog scenario runners and step definitions in `cmd/agentx-core/godog_test.go`.
  - Added dedicated GitHub Actions workflow `.github/workflows/hybrid-go-godog.yml` to run each GoDog suite separately.

#### Changed

- Updated `.github/copilot-instructions.md` to require GoDog (`github.com/cucumber/godog`) for Go Gherkin tests and define hermetic test scope expectations.
- Updated `cmd/agentx-core/ipc.go` FIFO creation to use a portable mkfifo syscall path.

### Test Changes

#### Added

- Added hermetic GoDog scenarios for:
  - Core configuration/layout primitives (unit)
  - IPC FIFO provisioning (integration)
  - Core lifecycle/context cancellation behavior (functional)
  - Core shutdown path with empty applet registry (e2e)

## [0.50.0] - 2026-05-17

### Code Changes

#### Added

- Added TUI graceful quit affordance `PD-16-AF-008` using a new quit sentinel pipeline.
  - Added `QUIT_SENTINEL` handling to `src/agentx/integration/tui_bridge.py` with `on_quit` callback dispatch.
  - Wired TUI quit callback in `src/agentx/session.py` to interrupt streaming and request Tk mainloop shutdown.
  - Updated launcher-generated Lua in `agentx` to provide `<leader>q` and `:AgentXQuit` for application quit.

### Test Changes

#### Added

- Added unit coverage in `tests/test_tui_bridge_output.py` for quit sentinel dispatch and mixed submit/quit parsing.
- Added integration coverage in `tests/test_session_gui_disabled.py` for session-level quit callback behavior.
- Extended launcher assertions in `tests/test_launch_vibe_shutdown.py` for generated quit command and keymap.

### Documentation Changes

#### Changed

- Updated TUI mirror affordance spec in `docs/ux/06_TUI_MIRROR.md` with `PD-16-AF-008`.
- Updated UX traceability matrix in `docs/ux/UX_LIFECYCLE.md` with `PD-16-AF-008` source/test linkage and status.
- Updated `docs/ux/00_INDEX.md` status snapshot totals for the new tested affordance.

## [0.49.1] - 2026-05-16

### Code Changes

#### Added

- Added deterministic Issue #9 verify-release harness: `docs/validation/verify_issue9_wide.sh`.
  - Uses fixed tmux geometry profile (`ISSUE9_TMUX_WIDTH`, `ISSUE9_TMUX_HEIGHT`; default `200x60`).
  - Writes unique per-run evidence under `/tmp/issue9_verify_profile.*`.
  - Uses explicit per-trial files and summary parsing (no wildcard ambiguity).
  - Emits a Markdown report (`report.md`) and CSV summary (`summary.csv`) for issue comments.
  - Full documentation with usage examples: [docs/validation/01_ISSUE_9_WIDE_PROFILE_VERIFICATION.md](docs/validation/01_ISSUE_9_WIDE_PROFILE_VERIFICATION.md).

### Documentation Changes

#### Changed

- Updated `docs/ux/UX_ISSUES.md` Issue #9 notes with deterministic wide-profile verification status.

## [0.49.0] - 2026-05-16

### Code Changes

#### Added

- Added deterministic tmux startup sizing for launcher-driven TUI sessions.
  - `agentx` now supports `AGENTX_TMUX_WIDTH` and `AGENTX_TMUX_HEIGHT`.
  - When set to positive integers, the launcher passes `-x/-y` to `tmux new-session` so headless and CI startup dimensions are stable.
  - Invalid values are ignored with a warning, preserving existing behavior.

### Test Changes

#### Added

- Added unit coverage in `tests/test_launch_vibe_shutdown.py`:
  - `test_start_uses_configured_tmux_dimensions_for_new_session`

## [0.48.14] - 2026-05-15

### Code Changes

#### Fixed

- Fixed intermittent TUI startup ENTER wait prompt by removing command-line startup notification from generated `agentx_tui.lua`.
  - Updated `agentx` Lua template to append startup guidance directly into the TUI output buffer as a `###SYSTEM` message.
  - This avoids Neovim command-line paging behavior (`Press ENTER or type command to continue`) during startup.

### Test Changes

#### Added

- Added issue #9 regression tests in `tests/test_launch_vibe_shutdown.py`:
  - `test_generated_tui_lua_uses_nonblocking_startup_hint_path` (unit)
  - `test_generated_tui_lua_writes_ready_hint_into_output_buffer` (integration)

### Documentation Changes

#### Changed

- Updated TUI contract documentation in `docs/ux/06_TUI_MIRROR.md` to reflect non-blocking startup guidance in output buffer.
- Updated `docs/ux/UX_ISSUES.md` with issue #9 intake, reproduction verdict, regression evidence, and latest fix-candidate status.

## [0.48.13.post1] - 2026-05-15

### Documentation Changes

#### Changed

- Documented the planned TUI-first default migration with GUI opt-in launch behavior.
  - Added a dedicated planning section in `docs/ux/06_TUI_MIRROR.md` covering target state, phased rollout, parity strategy, and docs/ux impact checklist.
  - Updated `docs/ux/00_INDEX.md` to link the migration plan and add a PD-16 migration queue item.
  - Updated `docs/ux/UX_LIFECYCLE.md` with a PD-16 planning cross-reference to the migration plan.

## [0.48.13] - 2026-05-15

### Code Changes

#### Fixed

- Hardened TUI thinking visibility for turns without explicit THINKING chunks.
  - Updated `src/agentx/streaming_controller.py` to emit `###THINKING 💭` from the classification callback when `tui.show_thinking=true`.
  - Added per-turn de-duplication so TUI thinking marker is emitted at most once per turn.

### Test Changes

#### Added

- Added regression test in `tests/test_tui_bridge_output.py` ensuring classification callback emits both thinking marker and classification block in TUI when thinking mirroring is enabled.

## [0.48.12] - 2026-05-15

### Code Changes

#### Fixed

- Fixed TUI classification mirroring when GUI classification panel rendering is disabled.
  - Updated `src/agentx/streaming_controller.py` so classification callback always emits TUI classification output (`###CLASSIFICATION 🤔`) while only gating GUI rendering behind `classification_display.enabled`.

### Test Changes

#### Added

- Added regression test in `tests/test_tui_bridge_output.py` to ensure TUI classification output is still emitted when GUI classification display is disabled.

## [0.48.11] - 2026-05-15

### Code Changes

#### Fixed

- Added missing TUI thinking emoji marker output and classification output mirroring.
  - Updated `src/agentx/streaming_controller.py` to emit `###THINKING 💭` in TUI when thinking mirroring is enabled.
  - Updated `src/agentx/streaming_controller.py` classification callback to emit a `###CLASSIFICATION 🤔` block to TUI.
  - Updated `src/agentx/integration/tui_event_subscriber.py` to normalize legacy `###THINKING` records with `💭` for parity.

### Test Changes

#### Changed

- Updated and extended TUI output tests in `tests/test_tui_bridge_output.py`.
  - Thinking marker assertion now validates `###THINKING 💭`.
  - Added regression coverage that classification callback emits a TUI classification block with emoji and key fields.

## [0.48.10.post1] - 2026-05-15

### Documentation Changes

#### Changed

- Completed Issue #8 verify-release audit trail and closure traceability.
  - Added release-like verification evidence link in `docs/ux/UX_ISSUES.md`.
  - Added final closure comment link and closed-unverified rationale in `docs/ux/UX_ISSUES.md`.
  - Recorded reopen trigger context and workflow transition to `issue-intake`.

## [0.48.10] - 2026-05-15

### Code Changes

#### Fixed

- Refined Issue #8 TUI emoji parity by decorating raw streamed TUI records in `src/agentx/integration/tui_event_subscriber.py`.
  - Adds `🤖` to raw `###AGENT` headers.
  - Adds `👤` prefix to raw `###USER` prompt lines when not already present.

### Test Changes

#### Fixed

- Stabilized nonblocking FIFO reader behavior in `tests/test_tui_prompt_flow_variants.py` by handling `BlockingIOError` during read loops.

## [0.48.9] - 2026-05-15

### Code Changes

#### Fixed

- Fixed Issue #8 TUI emoji parity by adding role emoji indicators to streamed and bootstrap TUI output records.
  - Updated `src/agentx/streaming_controller.py` to emit emoji-enhanced markers for streamed user and agent headers.
  - Updated `src/agentx/integration/tui_event_subscriber.py` role formatting for agent/user event payloads.
  - Updated `src/agentx/session.py` bootstrap TUI mirror output to include agent emoji marker.

### Test Changes

#### Changed

- Updated TUI regression and compatibility assertions to validate emoji-enhanced markers while preserving `###` framing semantics.
  - `tests/test_tui_emoji_regression.py`
  - `tests/test_tui_bridge_output.py`
  - `tests/test_session_gui_integration.py`

### Documentation Changes

#### Changed

- Updated Issue #8 status in `docs/ux/UX_ISSUES.md` to fix-complete and advanced the workflow prompt to verify-release.

## [0.48.8] - 2026-05-15

### Test Changes

#### Added

- Added Issue #8 TUI emoji regression coverage in `tests/test_tui_emoji_regression.py`.
  - `test_tui_event_formatting_includes_role_emoji`
    - GIVEN TUI role events (happy/defect/boundary)
    - WHEN events are formatted for output
    - THEN role emoji indicators (`👤`, `🤖`) are present.
  - `test_tui_submit_output_includes_user_and_agent_emojis`
    - GIVEN a hermetic TUI FIFO prompt flow across `AgentXSession` + `StreamingController` + `EventBroker` + `TUIEventSubscriber`
    - WHEN a prompt is submitted and response is streamed
    - THEN mirrored TUI output includes user/agent emoji indicators.

#### Changed

- Updated `docs/ux/UX_ISSUES.md` Issue #8 checklist and linked reproduction/regression evidence comments.

## [0.48.7.post1] - 2026-05-15

### Documentation Changes

#### Changed

- Completed Issue #6 verify-release documentation and closure traceability updates.
  - Recorded release/build verification evidence and closure decision links in `docs/ux/UX_ISSUES.md`.
  - Captured closed-unverified state rationale (live reporter GUI UAT unavailable in-session) and reopen criteria references.

## [0.48.7] - 2026-05-15

### Code Changes

#### Fixed

- Fixed Issue #6 approval exception handling to prevent GUI-side approval callback failures from propagating and leaving terminal workflows in an unstable state.
  - Updated `AgentXSession._request_terminal_approval` to fail closed (reject) when dialog execution raises unexpectedly and to always signal worker completion.
  - Updated `TerminalBridge.run_command` and `evaluate_terminal_policy` to catch approval callback exceptions and return deterministic rejected decisions.

### Test Changes

#### Changed

- Re-ran Issue #6 regression suite after fix implementation; defect-encoding tests now pass.
  - `tests/test_terminal_mode_and_approval.py` (7 passed)
  - `tests/test_terminal_bridge.py` (12 passed)

## [0.48.6] - 2026-05-15

### Test Changes

#### Added

- Added Issue #6 regression tests to encode approval-boundary crash/orphan risk paths.
  - `tests/test_terminal_mode_and_approval.py::test_request_terminal_approval_dialog_exception_rejects_without_propagating`
    - GIVEN approval callback path [PD-15-AF-006]
    - WHEN dialog callback raises unexpectedly
    - THEN approval should reject safely without exception propagation.
  - `tests/test_terminal_mode_and_approval.py::test_request_terminal_approval_timeout_falls_back_to_rejected`
    - GIVEN approval callback completion signal missing [PD-15-AF-006]
    - WHEN timeout fallback is used
    - THEN approval returns rejected and preserves original command.
  - `tests/test_terminal_bridge.py::test_run_command_approval_callback_exception_is_rejected`
    - GIVEN terminal bridge approval interaction boundary [PD-15-AF-006]
    - WHEN approval callback raises
    - THEN command execution should reject without propagating crash.

#### Changed

- Updated `docs/ux/UX_ISSUES.md` Issue #6 checklist and linked regression evidence comment.

## [0.48.5.post1] - 2026-05-15

### Documentation Changes

#### Changed

- Updated local UX issue tracking to reflect final resolved-verified closure for Issue #7.
  - Added final verify-release gate evidence link (v0.48.5, 5/5 startup passes).
  - Added closure comment link with reopen criteria.
  - Updated status from open investigation to closed/resolved-verified in `docs/ux/UX_ISSUES.md`.

## [0.48.5] - 2026-05-15

### Code Changes

#### Fixed

- Hardened Ollama launcher preflight behavior in `agentx` for heavy model startup paths.
  - Added `AGENTX_OLLAMA_PREFLIGHT_TIMEOUT_SEC` (default: `20`) so startup can tolerate slower first-token latency.
  - Reduced preflight generation work by sending chat probe option `"num_predict": 1`.
  - Added OOM-aware diagnostics when preflight response indicates CUDA/VRAM memory pressure.

### Test Changes

#### Added

- `tests/test_launch_vibe_shutdown.py::test_start_preflight_surfaces_gpu_oom_guidance`
  - GIVEN preflight returns HTTP 500 with CUDA out-of-memory details [PD-15-AF-010]
  - WHEN launcher starts
  - THEN launcher exits with OOM-specific guidance.

#### Changed

- `tests/test_launch_vibe_shutdown.py::test_start_preflight_uses_configured_model_payload`
  - Added assertion that preflight payload uses lightweight probe option `"num_predict": 1`.

## [0.48.4] - 2026-05-15

### Code Changes

#### Fixed

- `agentx` now retries Ollama chat-model preflight before failing startup for transient failures.
  - Added bounded retry/backoff controls:
    - `AGENTX_OLLAMA_PREFLIGHT_ATTEMPTS` (default: `2`)
    - `AGENTX_OLLAMA_PREFLIGHT_RETRY_SEC` (default: `1`)
  - Root cause: a single-shot preflight probe (`/api/chat`) could fail with transient timeout (`HTTP 000`) even when subsequent probes succeed, causing false-negative launcher aborts.

### Test Changes

#### Added

- `tests/test_launch_vibe_shutdown.py::test_start_preflight_timeout_defect_requires_retry`
  - GIVEN transient preflight timeout sequence (`000,200`) [PD-15-AF-010]
  - WHEN launcher starts with qwen model
  - THEN launcher should retry preflight and continue startup.
- `tests/test_launch_vibe_shutdown.py::test_start_fails_cleanly_when_preflight_stays_http_000`
  - GIVEN repeated timeout (`HTTP 000`) [PD-15-AF-010]
  - WHEN launcher starts
  - THEN launcher exits with clear preflight failure signature.
- `tests/test_launch_vibe_shutdown.py::test_start_preflight_uses_configured_model_payload`
  - GIVEN project config sets `qwen3.6:latest` [PD-15-AF-010]
  - WHEN launcher preflight runs
  - THEN request payload contains configured model and `/api/chat` endpoint.

---

## [0.48.3.post1] - 2025-05-13

### Documentation Changes

#### Added

- `.github/prompts/issue-intake.prompt.md` — triage, duplicate detection, reopen vs new logic
- `.github/prompts/issue-reproduce-evidence.prompt.md` — structured reproduction, artifacts, reproducibility matrix
- `.github/prompts/issue-investigate.prompt.md` — root-cause analysis, blast radius, fix strategy selection
- `.github/prompts/issue-regression-tests.prompt.md` — Gherkin scenario authoring, failing regression test baseline
- `.github/prompts/issue-fix-close.prompt.md` — fix code, quality gates, changelog, version, issue close
- `.github/prompts/issue-pr-handoff.prompt.md` — push branch, create PR, checklist, hand off to post-merge
- `.github/prompts/issue-verify-release.prompt.md` — post-merge verification, durable audit trail, reopen criteria
- `.github/prompts/00_INDEX.md` — indexed folder index with lifecycle pipeline diagram

#### Removed

- `.github/prompts/issue-intake-legacy.prompt.md` (renamed `new-issue.prompt.md`) — superseded by the seven-stage prompt suite above

---

## [0.48.3] - 2026-05-13

### Code Changes

#### Fixed

- `_request_terminal_approval` crash when the approval callback fires without `_enable_gui_chat` set.
  - Root cause: the test for `PD-15-AF-006` used `object.__new__(AgentXSession)` to bypass `__init__`, so `_enable_gui_chat` was never initialised. When `_enable_gui_chat` was added to `_request_terminal_approval` (headless-mode guard, commit `1f9cdd6`), the test started raising `AttributeError` — and this same path would crash in any code that constructs a session-like object without calling `__init__`.
  - Fix: set `session._enable_gui_chat = True` in the test setup so the unit exercises the correct GUI-enabled code path.

### Test Changes

#### Fixed

- `tests/test_terminal_mode_and_approval.py::test_request_terminal_approval_delegates_to_dialog_on_main_thread`
  - GIVEN a terminal command in supervised mode [PD-15-AF-006] WHEN approval requested on main thread THEN delegates to dialog — was failing with `AttributeError: 'AgentXSession' object has no attribute '_enable_gui_chat'`.
  - Added `session._enable_gui_chat = True` to test setup.

#### Added

- `tests/test_terminal_mode_and_approval.py::test_request_terminal_approval_returns_false_when_gui_disabled`
  - GIVEN a terminal command with GUI chat disabled (headless mode) [PD-15-AF-006] WHEN approval is requested THEN returns denied without opening any dialog.

---

## [0.48.2] - 2026-05-12

### Code Changes

#### Fixed

- Reordered tmux launcher windows for TUI-first workflow when TUI is enabled.
  - [agentx](agentx)
    - Changed startup window order to `0=tui-chat`, `1=editor`, `2=agent-bg`, `3=agentx-log` when `[tui].enable=true`.
    - Set explicit pre-attach window selection to make `tui-chat` the default visible pane when enabled.
    - Updated editor pane resolution fallback order to prefer named `editor` window before generic first-pane fallback.

### Documentation Changes

#### Changed

- Updated vibe-launch architecture and UX issue tracking for TUI-first pane ordering.
  - [docs/ux/05_VIBE_CODING.md](docs/ux/05_VIBE_CODING.md)
  - [docs/ux/UX_ISSUES.md](docs/ux/UX_ISSUES.md)

### Test Changes

#### Changed

- Updated launcher lifecycle tests to validate TUI-first ordering semantics.
  - [tests/test_launch_vibe_shutdown.py](tests/test_launch_vibe_shutdown.py)
    - Added assertions for `tui-chat` as session window `0` and `select-window -t agentx:0` before attach.

## [0.48.1] - 2026-05-12

### Code Changes

#### Fixed

- Improved launcher startup UX by adding an Ollama model preflight before tmux session creation.
  - [agentx](agentx)
    - Added config-driven preflight using `[agentx].ollama_host` and `[agentx].ollama_model`.
    - Added fast `/api/chat` validation with actionable guidance when a non-chat model is configured.
    - Added clear failure output to avoid misleading "launcher crashed" behavior when AgentX would otherwise exit immediately.

### Test Changes

#### Changed

- Performed manual launcher smoke validation for preflight success path and startup flow.

## [0.48.0] - 2026-05-11

### Code Changes

#### Added

- Added sandboxed editor-assist tooling integrated with terminal approval policy.
  - [src/agentx/integration/vim_bridge.py](src/agentx/integration/vim_bridge.py)
    - Added `editor_action()` supporting `show_symbol_help`, `autocomplete_assist`, and `propose_edit`.
    - Added payload sanitization and subprocess stdout/stderr capture for action results.
  - [src/agentx/integration/terminal_bridge.py](src/agentx/integration/terminal_bridge.py)
    - Added `evaluate_terminal_policy()` helper to reuse terminal allow/confirm/deny + approval callback flow for non-shell actions.
  - [src/agentx/integration/tool_registry_manager.py](src/agentx/integration/tool_registry_manager.py)
    - Added built-in `editor_action` tool wrapper with argument validation.
  - [src/agentx/integration/agentix_bridge_adapter.py](src/agentx/integration/agentix_bridge_adapter.py)
    - Registered `editor_action` tool schema for bridge invocation.
  - [src/agentx/tool_registry.py](src/agentx/tool_registry.py)
    - Added `editor_action` to default `system_tools`.
  - [src/agentx/integration/registry_tools.py](src/agentx/integration/registry_tools.py)
    - Added built-in documentation stub for `editor_action`.

#### Changed

- Updated tool issue tracker for final editor-tools sequence.
  - [docs/tools/tools_issues.md](docs/tools/tools_issues.md)
    - Marked `P6-001` complete with implementation and test references.

### Test Changes

#### Added

- [tests/test_tool_registry_manager.py](tests/test_tool_registry_manager.py)
  - Added `editor_action` tests for required argument validation and success dispatch.
- [tests/test_vim_bridge_gui.py](tests/test_vim_bridge_gui.py)
  - Added `editor_action` tests for unsupported actions, policy rejection, sandbox validation, and propose-edit success path.

---

## [0.47.0] - 2026-05-11

### Code Changes

#### Added

- Added vibe-editor diff tool support for side-by-side file comparisons.
  - [src/agentx/integration/vim_bridge.py](src/agentx/integration/vim_bridge.py)
    - Added `diff_files(left_file, right_file)` using neovim `--server` commands with vimdiff semantics.
    - Added `diff_files_from_context(left_file, right_file)` and Ex-command path escaping helper.
  - [src/agentx/integration/tool_registry_manager.py](src/agentx/integration/tool_registry_manager.py)
    - Added `builtin_diff_files_in_editor(left_file, right_file)` with argument validation and JSON results.
  - [src/agentx/integration/agentix_bridge_adapter.py](src/agentx/integration/agentix_bridge_adapter.py)
    - Registered `diff_files_in_editor` schema for bridge tool invocation.
  - [src/agentx/tool_registry.py](src/agentx/tool_registry.py)
    - Added `diff_files_in_editor` to default `system_tools`.
  - [src/agentx/integration/registry_tools.py](src/agentx/integration/registry_tools.py)
    - Added built-in documentation stub for `diff_files_in_editor`.

#### Changed

- Updated tool issue tracker for editor-tool sequence progress.
  - [docs/tools/tools_issues.md](docs/tools/tools_issues.md)
    - Marked `P5-001` complete and documented implementation + tests.

### Test Changes

#### Added

- [tests/test_tool_registry_manager.py](tests/test_tool_registry_manager.py)
  - Added `diff_files_in_editor` tests for required input validation.
  - Added success path test asserting `VimBridge.diff_files_from_context()` invocation.
  - Added failure path test for unavailable editor bridge.

---

## [0.46.0] - 2026-05-11

### Code Changes

#### Added

- Added built-in editor-open tool for vibe-editor integration.
  - [src/agentx/integration/tool_registry_manager.py](src/agentx/integration/tool_registry_manager.py)
    - Added `builtin_open_file_in_editor(file_path, line)` routed through `VimBridge.open_file_from_context()`.
    - Added JSON success/error responses for required-path validation and editor availability.
  - [src/agentx/integration/agentix_bridge_adapter.py](src/agentx/integration/agentix_bridge_adapter.py)
    - Registered `open_file_in_editor` tool schema with bridge-exposed built-ins.
    - Passed runtime config through to `ToolRegistryManager` for neovim socket resolution.
  - [src/agentx/tool_registry.py](src/agentx/tool_registry.py)
    - Added `open_file_in_editor` to default `system_tools` template.
  - [src/agentx/integration/registry_tools.py](src/agentx/integration/registry_tools.py)
    - Added built-in tool documentation stub for `open_file_in_editor`.

#### Changed

- Updated tool issue tracker for editor-tool sequence progress.
  - [docs/tools/tools_issues.md](docs/tools/tools_issues.md)
    - Marked `P4-001` complete and documented implementation + tests.

### Test Changes

#### Added

- [tests/test_tool_registry_manager.py](tests/test_tool_registry_manager.py)
  - Added `open_file_in_editor` tests for required input validation.
  - Added success path test asserting `VimBridge.open_file_from_context()` invocation.
  - Added failure path test for unavailable editor bridge.

---

## [0.45.0] - 2026-05-11

### Code Changes

#### Added

- Added deterministic vibe-editor intent routing in classification.
  - [src/agentix/bridge/classify_prompt.py](src/agentix/bridge/classify_prompt.py)
    - Added explicit phrase detection for requests such as "open <file> in vibe editor" and editor-first phrasing.
    - Added short-circuit classification route to `simple_action` + `single_tool` for reliable editor tool routing.

#### Changed

- Updated tool issues tracker status for Phase 3.
  - [docs/tools/tools_issues.md](docs/tools/tools_issues.md)
    - Marked `P3-001` complete and documented implementation/tests.

### Test Changes

#### Added

- [tests/test_classify_prompt_bridge.py](tests/test_classify_prompt_bridge.py)
  - Added deterministic-routing tests for vibe-editor intent override.
  - Added guard test confirming non-editor prompts still use normal LLM classification flow.

---

## [0.43.0.post1] - 2026-05-11

### Code Changes

#### Changed

- Documentation governance update only.
  - [.github/copilot-instructions.md](.github/copilot-instructions.md)
    - Added required anti-drift session closure policy.
    - Added mandatory quality-gate-before-commit requirements.
    - Added explicit rule: local commit required for completed change sets; never push unless user asks.
    - Added commit hygiene workflow to prevent session-owned changes from being treated as unrelated drift.

### Test Changes

#### Changed

- No code-path changes; no test behavior changes expected.

---

## [0.44.0] - 2026-05-11

### Code Changes

#### Added

- Added baked-in default tool-registry template and automatic regeneration when `agentx_tools.toml` is missing.
  - [src/agentx/tool_registry.py](src/agentx/tool_registry.py)
    - Added `DEFAULT_AGENTX_TOOLS_TOML` template stored in application code.
    - Added config-path resolution that auto-creates missing registry file from baked defaults.
    - Added support for search-path lookup from `agentx.toml` `[tool_registry].search_paths`.
- Added configurable tool-registry path management in application config.
  - [src/agentx/config.py](src/agentx/config.py)
    - Added `[tool_registry].search_paths` defaults.
    - Added validation for non-empty path list.
  - [agentx.toml](agentx.toml)
    - Added `[tool_registry]` section with default search paths.
- Added Settings UI exposure for tool-registry search paths.
  - [src/agentx/gui/settings_tab.py](src/agentx/gui/settings_tab.py)
    - Added `🧰 Tool Registry` section.
    - Added editable one-path-per-line search path list with Save/Reset actions.

#### Changed

- [src/agentx/tool_registry.py](src/agentx/tool_registry.py)
  - Clarified reload semantics: persisted dynamic registrations remain after reload.

### Test Changes

#### Added

- [tests/test_tool_registry.py](tests/test_tool_registry.py)
  - Added test for missing registry auto-regeneration using baked-in template.
  - Added test for search-path resolution from `agentx.toml` and default-file creation.
- [tests/test_config_tui_phase1.py](tests/test_config_tui_phase1.py)
  - Added validation test for `[tool_registry].search_paths` integrity.

#### Changed

- Existing focused suites run and pass with updated registry and config behavior.

---

## [0.42.0] - 2026-05-11

### Code Changes

#### Added

- Extended dynamic tool registry schema to support executable external-tool metadata.
  - [src/agentx/tool_registry.py](src/agentx/tool_registry.py)
    - Added support for extended fields: scope, source_path, runtime, entrypoint, input_schema, output_schema.
    - Added schema normalization for input/output metadata.
    - Added persistent registration path so new tools can be written directly to [agentx_tools.toml](agentx_tools.toml).
  - [src/agentx/integration/tool_registry_manager.py](src/agentx/integration/tool_registry_manager.py)
    - Expanded builtin register_tool parameters to accept full metadata for external scripts/functions.
    - Register flow now persists tool entries to TOML by default.
  - [src/agentx/integration/agentix_bridge_adapter.py](src/agentx/integration/agentix_bridge_adapter.py)
    - Expanded register_tool schema exposed to bridge with scope, source_path, runtime, entrypoint, input_schema, output_schema.
  - [agentx_tools.toml](agentx_tools.toml)
    - Added schema_version and default scoped tool paths (universal, user, project, session).
    - Added example external-tool registration block with input/output schema structure.

### Test Changes

#### Added

- [tests/test_tool_registry.py](tests/test_tool_registry.py)
  - Added tests for loading extended schema fields.
  - Added tests for persistent register_tool behavior with full metadata.
- [tests/test_tool_registry_manager.py](tests/test_tool_registry_manager.py)
  - Added tests for builtin registration with external script metadata persistence.

#### Changed

- [tests/test_tool_registry_integration.py](tests/test_tool_registry_integration.py)
  - Updated reload expectation to reflect persisted registration behavior.

---

## [0.41.0] - 2026-05-11

### Code Changes

#### Added

- Added built-in `diagnose_tools()` integration for agent-invoked tool pipeline diagnostics.
  - `src/agentx/integration/tool_registry_manager.py`
    - Added `builtin_diagnose_tools()`.
    - Added optional `bridge` dependency so diagnostics can execute against live bridge state.
    - Registered `diagnose_tools` in `get_builtin_tool_implementations()`.
  - `src/agentx/integration/agentix_bridge_adapter.py`
    - Passes bridge reference to `ToolRegistryManager`.
    - Registers `diagnose_tools` schema with the bridge.
  - `src/agentx/integration/registry_tools.py`
    - Added built-in tool documentation and stub for `diagnose_tools()`.

#### Changed

- `docs/tools/tools_issues.md`
  - Marked Phase 2 / P2-001 as complete (`[/]`) with implementation details and test status.

### Test Changes

#### Added

- `tests/test_tool_registry_manager.py`
  - Added coverage for `builtin_diagnose_tools()`:
    - returns error when bridge is unavailable
    - returns full 4-phase diagnostic report when bridge is configured
  - Updated built-in implementation-map assertions to include `diagnose_tools`.

---

## [0.40.1] - 2026-05-12

### Code Changes

#### Added

- **Tool Execution Diagnostics (P2 Foundation)**: Comprehensive diagnostic suite for verifying tool pipeline health.
  - `src/agentx/tool_diagnostics.py` — ToolDiagnostics class with 4-phase diagnostic suite:
    - Phase 1: Registry check — verify tools loaded from config
    - Phase 2: Bridge registration — confirm tools registered with bridge
    - Phase 3: Tool availability — check if LLM can see critical tools (read_file, write_file)
    - Phase 4: End-to-end execution — test actual tool invocation
  - Explicit error reporting for missing tools, schema issues, and execution failures.
  - JSON export and human-readable summary generation.
  - Factory function `create_tool_diagnostics()` for easy instantiation.

### Test Changes

#### Added

- `tests/test_tool_diagnostics.py` — 16 unit tests covering:
  - ToolDiagnostics initialization with and without registry manager.
  - Registry diagnostics phase (inventory, state verification).
  - Bridge diagnostics phase (registration status).
  - Availability diagnostics phase (critical tool visibility).
  - Execution diagnostics phase (read_file/write_file end-to-end).
  - Full diagnostic suite compilation with issue detection.
  - JSON export validation and factory function.

---

## [0.40.0] - 2026-05-11

### Code Changes

#### Added

- **Dynamic Tool Registry (P1 Implementation)**: Complete tool lifecycle management system.
  - `src/agentx/tool_registry.py` — ToolRegistry class: loads tools from `agentx_tools.toml`, maintains in-memory set, provides toggle/register/reload operations.
  - `src/agentx/integration/tool_registry_manager.py` — ToolRegistryManager: bridges registry with agent bridge and UI; provides built-in tools `reload_tools()` and `register_tool()`.
  - `src/agentx/integration/registry_tools.py` — Built-in tool implementations for tool discovery and registration.
  - `agentx_tools.toml` — New config file defining available tools (cst, ast, read_file, write_file, list_directory, get_file_info, search_files) with enable/disable state.
  - `AgentixBridgeAdapter._register_registry_tools()` — Registers built-in tools with bridge so agent can invoke `reload_tools()` and `register_tool()`.
  - Session layout update: populates ToolPanel with registry tools and wires toggle callback to registry manager.

#### Changed

- ToolPanel (`src/agentx/gui/tool_panel.py`) now receives tools from registry with enabled/disabled state instead of hardcoded list.
- Session tool population now uses registry manager for UI and bridge state synchronization.

### Test Changes

#### Added

- `tests/test_tool_registry.py` — 15 unit tests covering ToolRegistry load, toggle, register, reload, and state persistence.
- `tests/test_tool_registry_manager.py` — 18 unit tests covering ToolRegistryManager delegation, built-in tool implementations, and callback invocation.
- `tests/test_tool_registry_integration.py` — 6 integration tests verifying registry → bridge → UI pipeline (toggle, register, reload, state consistency).

---

## [0.39.3.post1] - 2026-05-11

### Code Changes

#### Changed

- Documentation curation only: split tool/tooling backlog from UX backlog to reduce triage drift.
- Added indexed tools docs entry point in `docs/tools/00_INDEX.md` and normalized `docs/tools/tools_issues.md` structure/status semantics.
- Updated UX docs cross-reference so tooling items are tracked in tools docs while UX items remain in the UX lifecycle tracker.

### Test Changes

#### Changed

- No code-path changes; no test behavior changes expected.

## [0.39.3] - 2026-05-10

### Code Changes

#### Fixed

- `src/agentx/event_broker.py`: replaced per-publish fire-and-forget callback threads + never-consumed queues with per-subscriber worker threads that actively drain queued events. This prevents silent delivery stalls after repeated prompt streams.
- `src/agentx/integration/tui_event_subscriber.py`: removed bounded `deque(maxlen=10000)` truncation and corrected writer-loop lock usage so idle waits do not occur while holding the queue lock.
- `agentx`: fixed generated `agentx_tui.lua` submit framing to use real newline sentinel (`"\n---SUBMIT---\n"`) and real newline join for input text, matching Python submit parsing.
- `agentx` / `agentx_tui.lua`: updated output pane append logic to coalesce partial stream chunks onto the current line and only emit new lines on explicit separators, preventing token-per-line rendering during streaming.

### Test Changes

#### Added

- `tests/test_event_broker_pubsub.py`:
  - Added order-preservation test for a single subscriber under burst publish load.
  - Added no-drop regression test for rapid publish against a busy subscriber.

#### Changed

- `tests/test_event_broker_pubsub.py`:
  - Replaced bounded-queue truncation assertion in TUI subscriber coverage with retention coverage for large queued event sets.

---

## [0.39.2] - 2026-05-09

### Code Changes

#### Fixed

- `agentx`: updated generated TUI Lua output reader command from one-shot FIFO read to persistent reopen loop (`while true; do cat <fifo>; done`) so the TUI output mirror does not die after writer-side EOF.
- `src/agentx/integration/tui_bridge.py`: changed input FIFO reader behavior to keep the read fd open across transient EOF, avoiding readerless reopen gaps that can stall or miss TUI submit writes.

### Test Changes

#### Added

- `tests/test_tui_bridge_output.py`:
  - Added regression test validating `_input_reader_loop` handles transient EOF and still dispatches later submit payloads without reopening fd.

#### Changed

- `tests/test_launch_vibe_shutdown.py`:
  - Added assertion that generated `agentx_tui.lua` uses persistent FIFO read loop for output mirror continuity.

---

## [0.39.1] - 2026-05-09

### Code Changes

#### Changed

- Updated TUI bridge output tests to validate real FIFO behavior so `write_output()` is a directly testable unit instead of only a mocked call path.
- Updated streaming-controller TUI emission assertions to validate `EventBroker.publish(...)` records (current architecture) instead of legacy direct bridge-write calls.

### Test Changes

#### Added

- `tests/test_tui_bridge_output.py`:
  - Added real FIFO reader/writer test for successful `write_output()` delivery.
  - Added disabled-bridge and empty-record unit cases for `write_output()` guard behavior.
  - Added large payload coverage for multi-write delivery semantics.
  - Added unicode payload coverage for encoded write/read behavior.

#### Changed

- `tests/test_tui_bridge_output.py`:
  - Streaming-controller integration assertions now validate broker-published payloads with `EventType.AGENT_CONTENT`.

#### Fixed

- Restored failing streaming-controller TUI tests after broker migration by asserting the active publish path.

---

## [0.39.0] - 2026-05-09

### Code Changes

#### Added

- **NEW EVENT-BROKER PUB-SUB ARCHITECTURE** — centralized pub-sub system for streaming:
  - `src/agentx/event_broker.py`: `EventBroker` class with guaranteed event delivery to all subscribers.
  - `src/agentx/event_broker.py`: `EventType` enum for all streaming events (thinking, content, tool calls, errors, etc.).
  - `src/agentx/integration/tui_event_subscriber.py`: `TUIEventSubscriber` for reliable TUI output handling via event broker.
    - Maintains bounded event queue (maxlen=10000)
    - Background writer thread for FIFO writes with retry/backoff
    - Formats events into TUI protocol (###THINKING, ###AGENT, ###TOOL_CALL, etc.)
  - `tests/test_event_broker_pubsub.py`: 11 comprehensive unit tests covering pub-sub system, TUI subscriber, and end-to-end data flow.
  - `docs/event_broker_pubsub.md`: Complete architecture documentation with examples and diagrams.

#### Changed

- `src/agentx/streaming_controller.py`:
  - `_write_tui_output()` now publishes to EventBroker instead of direct FIFO writes.
  - No longer silently drops data if FIFO unavailable.
  - Guaranteed delivery to TUI via pub-sub.
- `src/agentx/session.py`:
  - Added `event_broker: EventBroker` initialization.
  - Created and wired `tui_event_subscriber: TUIEventSubscriber` to all event types.
  - Subscribers started/stopped with session lifecycle.

#### Fixed

- **TUI output data loss** — Previous non-blocking FIFO writes silently dropped data if reader unavailable.
  Now uses pub-sub guarantees with per-subscriber queues and retry/backoff.
- **TUI input processing** — Data flow now guaranteed end-to-end via event broker.

### Test Changes

#### Added

- `tests/test_event_broker_pubsub.py`: Full test suite with 11 unit tests (11/11 passing):
  - `TestEventBrokerPubSub`: Basic publish/subscribe, multiple subscribers, unsubscribe, slow subscribers.
  - `TestTUIEventSubscriber`: Event formatting, buffering, bounded queue, writer thread.
  - `TestStreamingControllerPubSub`: Publishing events, graceful handling of missing broker.
  - `TestEndToEndDataFlow`: Full chain from StreamingController → EventBroker → TUI Subscriber.

### Architecture

- Replaced brittle point-to-point FIFO writes with centralized event broker.
- Each subscriber gets its own queue; slow subscribers don't block publishers.
- Events buffered and retried with backoff if FIFO unavailable.
- Design supports future subscribers (logging, monitoring, bidirectional UI sync).
- See `docs/event_broker_pubsub.md` for complete architecture and migration guide.

---

## [0.38.5] - 2026-05-09

### Code Changes

#### Added

- `src/agentx/gui/settings_tab.py`: added a new `🪟 TUI Mirror` Settings section
  exposing `[tui]` keys:
  `enable`, `output_split_ratio`, `write_timeout_sec`, and `show_thinking`.
- `agentx.toml`: added explicit `agentx.auto_stop_tmux_on_gui_exit = true`
  so launcher auto-stop behavior is visible in project config by default.

#### Changed

- `agentx`: added support for `AGENTX_TUI_OUTPUT_SPLIT_RATIO` and
  `[tui].output_split_ratio` from `agentx.toml`.
- `agentx` generated `agentx_tui.lua`: now enforces output-on-top/input-on-bottom
  with `belowright split` and computes input-pane height from configurable
  output ratio.
- `agentx_tui.lua`: aligned checked-in script with launcher-generated behavior
  (split ratio env support + explicit split orientation).

### Test Changes

#### Added

- `tests/test_launch_vibe_shutdown.py`:
  - `test_start_reads_tui_split_ratio_from_project_toml`
    - GIVEN `agentx.toml` has `tui.output_split_ratio`
    - WHEN launcher starts
    - THEN TUI launch command includes `AGENTX_TUI_OUTPUT_SPLIT_RATIO`.

#### Changed

- `tests/test_launch_vibe_shutdown.py`:
  - strengthened TUI startup assertions to verify split-ratio env propagation
    and generated Lua wiring for split ratio + explicit split direction.

## [0.38.4] - 2026-05-09

### Code Changes

#### Fixed

- `agentx`: fixed tmux window allocation to use the next available
  explicit window index for `editor` recovery, `agent-bg`, `agentx-log`, and
  `tui-chat`, eliminating startup failures such as
  `create window failed: index <N> in use`.
- `agentx`: fixed generated `agentx_tui.lua` heredoc quoting so
  launcher no longer fails under `set -u` with unbound `AGENTX_TUI_*`
  variables while writing the Lua file.

### Test Changes

#### Changed

- `tests/test_launch_vibe_shutdown.py`:
  - updated tmux command-log assertions to support explicit
    `session:index` window targets.
  - hardened default TUI status test with explicit `AGENTX_TUI_ENABLE=false`
    to avoid environmental coupling.

## [0.38.3] - 2026-05-09

### Code Changes

#### Fixed

- `agentx`: fixed stale editor pane targeting by retrying target
  resolution when an initial `tmux send-keys` fails, preventing start failures
  like `can't find pane: %0`.
- `agentx`: restored default tmux auto-shutdown on AgentX/GUI exit and
  made it configurable via `AGENTX_AUTO_STOP_ON_EXIT` and
  `[agentx].auto_stop_tmux_on_gui_exit`.
- `agentx`: now reads `[tui]` defaults from project `agentx.toml`
  (`enable`, `socket`, `output_fifo`, `input_fifo`) when launcher env overrides
  are not provided.
- `agentx`: fixed `status`/`stop` early-exit regression under `set -e`
  in TOML default loading logic.

### Test Changes

#### Added

- `tests/test_launch_vibe_shutdown.py`:
  - `test_start_reads_tui_enable_from_project_toml`
    - GIVEN `agentx.toml` with `tui.enable=true`
    - WHEN launcher starts without `AGENTX_TUI_ENABLE`
    - THEN `tui-chat` window is created.
  - `test_start_reads_auto_stop_override_from_project_toml`
    - GIVEN `agentx.toml` with `agentx.auto_stop_tmux_on_gui_exit=false`
    - WHEN launcher starts
    - THEN AgentX command does not include tmux kill-session hook.

#### Changed

- `tests/test_launch_vibe_shutdown.py`:
  - restored start lifecycle assertion to validate default tmux auto-kill hook
    remains present when no override is configured.

## [0.38.2.post1] - 2026-05-09

### Code Changes

#### Changed

- `docs/ux/06_TUI_MIRROR.md`: reconciled `PD-16-AF-006` status to
  implemented/tested and refreshed revision stamp.
- `docs/ux/UX_LIFECYCLE.md`: reconciled PD-16 source mapping, added full
  PD-16 traceability matrix rows (`PD-16-AF-001..007`) with test references,
  and refreshed revision stamp.
- `docs/ux/00_INDEX.md`: updated status snapshot and totals to reflect PD-16
  completion and moved PD-16 queue item to completed.
- `pyproject.toml`: bumped version to `0.38.2.post1` for a docs-only release.

## [0.38.2] - 2026-05-09

### Test Changes

#### Added

- `test_status_reports_tui_disabled_by_default`
  - GIVEN default launcher settings
  - WHEN `launch_vibe.sh status` runs
  - THEN launcher reports `TUI : disabled` and omits TUI path lines.
- `test_restart_with_tui_enabled_recreates_tui_lifecycle`
  - GIVEN an existing TUI-enabled launcher session
  - WHEN `launch_vibe.sh restart` runs
  - THEN stop/start lifecycle re-creates TUI FIFOs and relaunches `tui-chat`
    with `agentx_tui.lua` wiring.

#### Changed

- `docs/ux/06_TUI_MIRROR.md`: updated revision stamp and added `TUI-FT-004`
  and `TUI-FT-005` scenarios for default status and restart lifecycle coverage.

## [0.38.1] - 2026-05-09

### Test Changes

#### Added

- `test_start_with_tui_disabled_does_not_launch_tui_window_or_lua`
  - GIVEN default launcher settings
  - WHEN `launch_vibe.sh start` runs
  - THEN no `tui-chat` window is created and `agentx_tui.lua` is not generated.
- `test_tui_enabled_status_and_stop_report_and_cleanup`
  - GIVEN a TUI-enabled launcher session
  - WHEN `status` and `stop` run after startup
  - THEN TUI state lines are reported and TUI FIFOs are removed.

#### Changed

- `docs/ux/06_TUI_MIRROR.md`: updated revision stamp and added `TUI-FT-003`
  scenario for TUI status/stop lifecycle coverage.

## [0.38.0] - 2026-05-09

### Code Changes

#### Added

- `agentx`: added runtime generation of `agentx_tui.lua` in the project
  root with split layout, FIFO tailing, and keymaps (`<leader>s`, `<leader>c`,
  `<leader>o`, `<leader>i`) for the TUI mirror workflow.

#### Changed

- `agentx`:
  - TUI startup now launches neovim with `--cmd 'luafile .../agentx_tui.lua'`
    and scoped TUI FIFO environment variables.
  - adds generated `agentx_tui.lua` to `.gitignore` when the project already
    has a `.gitignore` file.
- `tests/test_launch_vibe_shutdown.py`:
  - extended TUI-enabled launcher test to validate `luafile` startup command
    wiring and generated `agentx_tui.lua` file creation.
- `docs/ux/06_TUI_MIRROR.md`: updated revision stamp and marked PD-16 launcher
  and Lua affordance statuses as implemented.

### Test Changes

#### Changed

- `test_start_with_tui_enabled_launches_tui_window_and_env`
  - GIVEN TUI mode enabled under fake tmux
  - WHEN launcher starts session
  - THEN `tui-chat` launch command includes `luafile` wiring and generated
    `agentx_tui.lua` exists in the project directory.

## [0.37.0] - 2026-05-09

### Code Changes

#### Added

- `src/agentx/integration/tui_bridge.py`: added input FIFO reader support with
  submit-sentinel parsing (`\n---SUBMIT---\n`), callback dispatch, daemon
  reader lifecycle, and graceful stop handling.
- `tests/test_tui_bridge_output.py`: added hermetic FIFO input tests for submit
  parsing and whitespace-only discard behavior.
- `tests/test_launch_vibe_shutdown.py`: added hermetic launcher test validating
  TUI-enabled startup wiring (`tui-chat` window + AgentX TUI env vars).

#### Changed

- `src/agentx/session.py`:
  - wires TUI input FIFO path resolution via config/env/session defaults
  - routes TUI submit callbacks to the Tk main thread (`_safe_root_after`)
  - preserves injected pending prompts in `stream_ollama_response` instead of
    always overriding with GUI input.
- `agentx`:
  - adds optional TUI lifecycle variables (`AGENTX_TUI_ENABLE`,
    `AGENTX_TUI_OUTPUT_FIFO`, `AGENTX_TUI_INPUT_FIFO`, `AGENTX_TUI_SOCKET`)
  - creates/removes TUI FIFOs and socket during start/stop cleanup
  - launches optional `tui-chat` tmux window when TUI mode is enabled
  - includes TUI status lines in `status` output.
- `docs/ux/06_TUI_MIRROR.md`: updated revision stamp and marked Phase 3 complete
  with partial Phase 4 progress.

### Test Changes

#### Added

- `test_tui_bridge_reads_submit_messages_from_input_fifo`
  - GIVEN a running TUI input reader
  - WHEN FIFO payloads contain submit sentinels
  - THEN callback receives trimmed prompt texts in order.
- `test_tui_bridge_ignores_empty_submit_messages`
  - GIVEN a running TUI input reader
  - WHEN FIFO payload is whitespace-only before submit sentinel
  - THEN callback is not invoked for empty content.
- `test_start_with_tui_enabled_launches_tui_window_and_env`
  - GIVEN TUI mode enabled in launcher environment
  - WHEN `launch_vibe.sh start` runs under fake tmux
  - THEN `tui-chat` window launch and TUI env wiring are present.

## [0.36.0] - 2026-05-09

### Code Changes

#### Added

- `src/agentx/integration/tui_bridge.py`: added `TuiBridge` with non-blocking FIFO
  writes (`write_output`), runtime lifecycle controls (`start`/`stop`), and
  `is_enabled` status gating.
- `tests/test_tui_bridge_output.py`: new hermetic tests covering bridge write
  behavior and streaming-controller TUI record emission.

#### Changed

- `src/agentx/session.py`:
  - initializes optional `self.tui_bridge` when `tui.enable=true`
  - resolves default output FIFO path via `AGENTX_TMUX_SESSION` session scope
  - stops the bridge during session shutdown.
- `src/agentx/streaming_controller.py`:
  - emits TUI records for USER / AGENT / TOOL_CALL / TOOL_RESULT / DONE / ERROR
  - supports optional THINKING emission via `tui.show_thinking`.
- `src/agentx/integration/__init__.py`: exports `TuiBridge`.
- `docs/ux/06_TUI_MIRROR.md`: Phase 2 checklist items marked complete.

### Test Changes

#### Added

- `test_tui_bridge_write_output_success`
  - GIVEN writable FIFO
  - WHEN `write_output` runs
  - THEN payload is written and descriptor is closed.
- `test_tui_bridge_write_output_drops_when_no_reader`
  - GIVEN no FIFO reader
  - WHEN `write_output` runs
  - THEN write is dropped without raising.
- `test_streaming_controller_writes_agent_and_tool_records_to_tui`
  - GIVEN streaming/tool events
  - WHEN controller handlers run
  - THEN TUI records are emitted with expected prefixes.
- `test_streaming_controller_respects_show_thinking_flag_for_tui`
  - GIVEN `show_thinking=false`
  - WHEN thinking renders
  - THEN no THINKING records are emitted.
- `test_streaming_controller_writes_thinking_when_enabled`
  - GIVEN `show_thinking=true`
  - WHEN thinking renders
  - THEN THINKING records are emitted.

## [0.35.0] - 2026-05-09

### Code Changes

#### Added

- `src/agentx/igui_manager.py`: added `NullGUIManager`, a headless no-op GUI implementation
  for `enable_gui_chat=false` mode. It preserves the session-facing interface while avoiding
  widget creation.
- `tests/test_session_gui_disabled.py`: new hermetic tests for GUI-disabled runtime behavior.

#### Changed

- `src/agentx/session.py`:
  - wired `enable_gui_chat=false` path to instantiate `NullGUIManager` instead of `GUIManager`
  - uses `tk.Tk(useTk=False)` when no root is provided and GUI is disabled
  - skips GUI layout work when GUI chat is disabled
  - denies terminal approval requests in headless mode (no modal UI available)
  - ensures `gui.destroy()` is called safely during shutdown
- `docs/ux/06_TUI_MIRROR.md`: Phase 1 checklist updated; NullGUI and headless wiring items are now complete.

### Test Changes

#### Added

- `test_session_uses_null_gui_manager_when_gui_disabled`
  - GIVEN `enable_gui_chat=false`
  - WHEN session initializes
  - THEN `NullGUIManager` is used and `GUIManager` is not constructed.
- `test_terminal_approval_is_denied_in_headless_mode`
  - GIVEN headless mode
  - WHEN terminal approval is requested
  - THEN approval is denied and original command is returned.
- `test_layout_skips_gui_setup_when_disabled`
  - GIVEN headless mode
  - WHEN `session.layout()` is called
  - THEN GUI layout creation is not invoked.

#### Changed

- Re-validated existing Phase 1 config tests and GUI-enabled integration smoke test after headless path wiring.

## [0.34.0] - 2026-05-09

### Code Changes

#### Added

- `src/agentx/config.py`: added Phase 1 TUI/GUI configuration groundwork:
  - `ConfigurationError` for config-constraint failures
  - `apply_config_defaults(config)` to inject defaults for `agentx.enable_gui_chat`
    and `[tui]` settings (`enable`, `socket`, `output_fifo`, `input_fifo`,
    `output_split_ratio`, `write_timeout_sec`, `show_thinking`)
  - environment overrides for TUI runtime values:
    `AGENTX_TUI_ENABLE`, `AGENTX_TUI_OUTPUT_FIFO`, `AGENTX_TUI_INPUT_FIFO`,
    `AGENTX_TUI_SOCKET`
- `tests/test_config_tui_phase1.py`: new hermetic unit tests for Phase 1 config
  defaults, validation, and environment override behavior.

#### Changed

- `src/agentx/config.py`: `load_config()` now runs defaulting + validation before
  returning config.
- `src/agentx/session.py`: `AgentXSession.__init__` now applies and validates config
  defaults even when tests or callers inject config dicts directly (without calling
  `load_config()`).
- `docs/ux/06_TUI_MIRROR.md`: Phase 1 checklist updated to reflect completed config
  groundwork and remaining NullGUI-specific tasks.

### Test Changes

#### Added

- `test_load_config_applies_gui_and_tui_defaults`
  - GIVEN missing `enable_gui_chat` / `[tui]` keys
  - WHEN config is loaded
  - THEN defaults are applied.
- `test_load_config_rejects_both_gui_and_tui_disabled`
  - GIVEN `enable_gui_chat=false` and `tui.enable=false`
  - WHEN config is loaded
  - THEN `ConfigurationError` is raised.
- `test_load_config_accepts_headless_mode_when_tui_enabled`
  - GIVEN `enable_gui_chat=false` and `tui.enable=true`
  - WHEN config is loaded
  - THEN config is accepted.
- `test_load_config_rejects_invalid_tui_split_ratio`
  - GIVEN invalid `output_split_ratio`
  - WHEN config is loaded
  - THEN `ConfigurationError` is raised.
- `test_load_config_applies_tui_env_overrides`
  - GIVEN AGENTX_TUI_* env vars
  - WHEN config is loaded
  - THEN env values override TOML values.

#### Changed

- Session integration smoke test continues to pass with the new config defaulting path:
  `tests/test_session_gui_integration.py::TestAgentXSessionGUIIntegration::test_session_has_valid_configuration`.

## [0.33.1.post1] - 2026-05-09

### Code Changes

#### Added

- `docs/ux/06_TUI_MIRROR.md` — new UX specification and implementation plan for the
  optional TUI mirror chat pane (PD-16), including architecture, IPC contract,
  configuration toggles (`tui.enable`, `enable_gui_chat`), user flows, test scenarios,
  and phased rollout checklist.

#### Changed

- `docs/ux/00_INDEX.md` — registered TUI mirror spec in the navigation table, added
  PD-16 status row, updated UX totals, and queued PD-16 implementation in the priority list.
- `docs/ux/05_VIBE_CODING.md` — linked OQ-10 to the dedicated TUI mirror specification.
- `docs/ux/UX_LIFECYCLE.md` — added PD-16 panel reference to the lifecycle quick-reference map.

### Test Changes

#### Added

- No runtime code changes in this release.
- Added documented test-plan scenarios in `docs/ux/06_TUI_MIRROR.md` (unit,
  integration, functional) with Gherkin-style GIVEN/WHEN/THEN expectations for
  upcoming implementation work.

#### Changed

- No existing executable tests modified.

#### Fixed

- No failing executable tests addressed in this release.

#### Removed

- No tests removed.

## [0.33.1] - 2026-05-09

### Code Changes

#### Fixed

- `VimBridge` socket path resolution: the hardcoded default `/tmp/agentx.nvim.sock` did not
  match the session-scoped path created by `agentx`.  Resolution now mirrors the
  shell script exactly (highest priority first):
  1. `AGENTX_NVIM_SOCKET` environment variable
  2. `config["neovim"]["socket"]` from the runtime config dict
  3. `/tmp/agentx_<AGENTX_TMUX_SESSION>.nvim.sock` (default session name: `agentx`)
- Added `_resolve_default_socket()` module-level helper (also exported for testability).
- `VimBridge.__init__` `socket_path` parameter type changed from `str` (with default) to
  `str | None` (explicit override only); callers that omit it now get the correct env-based path.

### Test Changes

#### Added

- `TestVimBridgeConfig` expanded with 4 new env-var-aware tests replacing the 2 old static
  default tests:
  - `test_default_socket_path_uses_env_var_formula`
  - `test_agentx_nvim_socket_env_overrides_default`
  - `test_agentx_tmux_session_env_scopes_socket_path`
  - `test_explicit_socket_path_overrides_config`
  - `test_config_without_neovim_key_falls_through_to_env_formula`

---

## [0.33.0] - 2026-05-09

### Code Changes

#### Added

- `src/agentx/integration/vim_bridge.py` — new `VimBridge` class that opens files in a running
  neovim instance via `nvim --server <socket> --remote <path>`.  No new Python dependencies;
  uses neovim's built-in CLI RPC client.  Socket path defaults to `/tmp/agentx.nvim.sock` and
  is configurable via `config["neovim"]["socket"]`.  [PD-14-AF-002]
- `AgentXSession.vim_bridge` — `VimBridge` instance created at session startup and held on the
  session for testability and future extension.
- `AgentXSession._open_file_in_editor(file_path)` — callback that delegates to
  `vim_bridge.open_file_from_context()`.  Logs a warning when neovim is not connected.

#### Changed

- `AgentXSession.refresh_files_gui()` — `on_edit=None` placeholder replaced with
  `on_edit=self._open_file_in_editor`, wiring the FileExplorer "Edit" context-menu entry to
  the new neovim integration.

### Test Changes

#### Added

- `tests/test_vim_bridge_gui.py` — 16 hermetic unit and integration tests for `VimBridge` and
  session wiring (PD-14-AF-002).
  - `TestVimBridgeIsConnected`: 3 tests — socket exists (True), missing (False), regular file (False)
  - `TestVimBridgeConfig`: 4 tests — default path, explicit path, config override, no neovim key
  - `TestVimBridgeOpenFile`: 6 tests — dispatches correct command, line-number prefix, not
    connected, nvim not on PATH, non-zero exit, OSError
  - `TestVimBridgeOpenFileFromContext`: 3 tests — relative path resolved, absolute unchanged, line forwarded
  - `TestSessionOpenFileInEditor`: 2 integration tests — delegates to vim_bridge, graceful on disconnected

---

## [0.32.1] - 2026-05-10

### Code Changes

#### Changed

- `_CAPTURE_SENTINEL` renamed to `_CAPTURE_SENTINEL_PREFIX` — it is now a prefix, not the full
  sentinel string.  Each `run_command()` invocation generates a unique sentinel
  `__AGENTX_DONE__<uuid>__` via `uuid.uuid4().hex`, eliminating cross-invocation contamination
  on the persistent pane (`session:1.0`) where back-to-back commands share the same scrollback.
- `_wait_for_completion()` now accepts an explicit `sentinel: str` parameter rather than reading
  the module-level constant, and matches only its own unique sentinel string.
- Added `import uuid` to `terminal_bridge.py`.

### Test Changes

#### Added

- `test_run_command_persistent_pane_timeout_sends_ctrl_c_not_kill` — verifies that on timeout with
  `visible=False`, `send-keys C-c` is sent and `kill-pane` is **not** invoked.  [PD-15-AF-009]
  - GIVEN `visible=False` persistent pane, `timeout_sec=0`
  - WHEN `_wait_for_completion` times out
  - THEN `send-keys C-c` is called; `kill-pane` is absent from tmux calls
- `test_run_command_pane_closed_early_returns_gracefully` — verifies that a `RuntimeError` from
  `capture-pane` (pane already gone) returns `(timed_out=False, exit_code=-1, "pane closed …")`.  [PD-15-AF-009]
  - GIVEN ephemeral pane closes before first poll
  - WHEN `capture-pane` raises `RuntimeError`
  - THEN result carries `exit_code=-1` and `"pane closed"` in stdout, `timed_out=False`

#### Changed

- All `fake_run` closures in existing tests replaced with `_StatefulFakeTmux` helper class that
  automatically extracts the per-invocation UUID sentinel from the dispatched `send-keys` command
  and echoes it back in the `capture-pane` response — tests no longer rely on a fixed sentinel
  string and therefore remain correct regardless of the UUID value.
- `_CAPTURE_SENTINEL` import in test file updated to `_CAPTURE_SENTINEL_PREFIX`.

---

## [0.32.0] - 2026-05-09

### Code Changes

#### Added

- `TerminalBridge._wait_for_completion()` — polls `tmux capture-pane` at 0.5 s intervals until
  `__AGENTX_DONE__<exit_code>` sentinel is detected in pane scrollback or `timeout_sec` elapses.
- Real exit-code capture: `run_command()` now wraps dispatched commands with the sentinel and
  returns the actual exit code from the process via `TerminalResult.exit_code`.
- Timeout enforcement: on deadline exceeded, visible panes are killed (`kill-pane`); persistent
  pane (1.0) receives `Ctrl+C` to preserve the shell.  `TerminalResult.timed_out` is set to `True`
  and `exit_code` to `-1` on timeout.
- `_CAPTURE_SENTINEL` and `_DEFAULT_POLL_INTERVAL` module-level constants exported for test use.

#### Changed

- `run_command()` no longer returns a static `"DISPATCHED …"` stdout; actual captured pane output
  (excluding the sentinel line) is returned in `TerminalResult.stdout`.
- Auto-close of ephemeral panes now happens via `kill-pane` after capture, not via a shell
  `exit` injected into the dispatch command.

### Test Changes

#### Added

- `test_run_command_captures_exit_code_from_sentinel` — verifies exit code 42 propagated from
  sentinel, stdout cleaned of sentinel line. [PD-15-AF-009]

  GIVEN active tmux session WHEN capture-pane returns sentinel `__AGENTX_DONE__42` THEN `exit_code == 42` and sentinel absent from stdout.

- `test_run_command_timeout_sets_timed_out_flag_and_kills_pane` — `timeout_sec=0` triggers
  timeout path: `timed_out=True`, `exit_code=-1`, `kill-pane` called. [PD-15-AF-009]

  GIVEN active session WHEN timeout_sec=0 THEN poll loop skipped, `timed_out=True`, `kill-pane` invoked.

- `test_run_command_edited_command_is_dispatched` — approval callback returns edited command;
  `executed_command` and `send-keys` payload reflect the edit. [PD-15-AF-006]

  GIVEN supervised mode with confirm-list command WHEN approval callback edits command THEN `executed_command` and `send-keys` carry the edited string.

#### Changed

- Existing dispatch tests (`visible_creates_ephemeral_pane`, `appends_audit_log_entry`,
  `confirm_command_dispatches_when_approved`) updated to handle `capture-pane` in `fake_run`
  and patch `time.sleep` for speed.

---

## [0.31.0] - 2026-05-09

### Code Changes

#### Added

- `src/agentx/streaming_controller.py`: added terminal `tool_result` decision badges (allowed/approved/denied/rejected) and exit-code annotations in rendered tool-result rows.

#### Changed

- `src/agentx/integration/terminal_bridge.py`: `terminal_run(...)` now resolves `visible`, `auto_close`, and `timeout_sec` from runtime terminal settings when explicit arguments are omitted.

### Test Changes

#### Added

- `tests/test_terminal_bridge.py`: added unit coverage for wrapper default forwarding and supervised confirm-list dispatch approval path.
- `tests/test_terminal_streaming_controller.py`: added unit coverage asserting terminal decision badge and exit-code rendering in streamed tool-result rows.

#### Changed

- Re-ran focused terminal feature regression suite covering bridge policy/dispatch, session approval hooks, settings editor behavior, and tool-result UI wiring.

---

## [0.30.0] - 2026-05-09

### Code Changes

#### Added

- `src/agentx/gui/settings_tab.py`: added terminal permission-list editor controls (Allow/Confirm/Deny), save action, and reset-to-defaults action for PD-15-AF-007.
- `src/agentx/integration/terminal_bridge.py`: added `reload_terminal_config(...)` helper so runtime permission-layer prefixes can be reloaded from updated config without restarting.

#### Changed

- `src/agentx/session.py`: terminal settings updates now trigger bridge config reload and status refresh in `_on_setting_change(...)`.

### Test Changes

#### Added

- `tests/test_terminal_settings_editor.py`: added unit coverage for terminal permission-list save and reset flows.

#### Changed

- `tests/test_terminal_mode_and_approval.py`: added unit coverage asserting terminal settings updates call runtime reload hook.

---

## [0.29.0] - 2026-05-09

### Code Changes

#### Added

- `src/agentx/gui/input_panel.py`: added terminal mode toggle button in the input status strip and callback routing for user mode changes (PD-15-AF-005).
- `src/agentx/session.py`: added terminal exec-mode toggle handling with confirmation gate for autonomous mode, runtime/app-config synchronization, and a supervised command approval dialog flow (PD-15-AF-005/006).
- `src/agentx/integration/terminal_bridge.py`: added runtime helpers for setting/getting execution mode and attaching approval callbacks to the configured bridge singleton.
- `src/agentx/gui/settings_tab.py`: added a Terminal Execution section with `exec_mode`, visibility, auto-close, and timeout settings.

#### Changed

- `src/agentx/gui/gui_manager.py`, `src/agentx/igui_manager.py`, and `src/agentx/widget_registry.py`: extended GUI protocol and widget registry for terminal mode toggling.

### Test Changes

#### Added

- `tests/test_terminal_mode_and_approval.py`: added unit coverage for autonomous-mode confirmation and session approval-request delegation.

#### Changed

- `tests/test_terminal_pane_gui.py`: added integration coverage for terminal mode-button callback invocation.

---

## [0.28.0] - 2026-05-08

### Code Changes

#### Added

- `src/agentx/gui/input_panel.py`: added terminal activity status strip rendering and `set_terminal_status(active_panes, exec_mode)` for PD-15-AF-003.
- `src/agentx/gui/chat_panel.py`: added per-entry action button wiring and `set_tool_result_kill_action(...)` to expose kill-pane actions on terminal tool-result rows (PD-15-AF-004).
- `src/agentx/session.py`: added tracked active-pane state, terminal status strip refresh hook, and kill-pane callback handler for GUI action buttons.
- `src/agentx/streaming_controller.py`: added terminal-aware tool-result parsing to track active panes, refresh status strip, and wire kill actions for successful `terminal_run` calls.
- `src/agentx/gui/gui_manager.py`, `src/agentx/igui_manager.py`, and `src/agentx/widget_registry.py`: added protocol/registry/delegation support for terminal status updates and tool-result kill actions.

### Documentation Changes

#### Changed

- `docs/ux/UX_LIFECYCLE.md`: updated revision stamp and added PD-15 traceability rows for AF-003 and AF-004 with ✅ status.
- `docs/ux/00_INDEX.md`: reconciled PD-15 status snapshot totals and marked the PD-15-AF-003..004 queue item complete.
- `docs/ux/05_VIBE_CODING.md`: updated revision stamp to reflect the latest terminal GUI wiring milestone.

### Test Changes

#### Added

- `tests/test_terminal_pane_gui.py`: added integration coverage for terminal status strip text updates and kill-pane action callback invocation.
- `tests/test_terminal_streaming_controller.py`: added unit coverage for terminal pane tracking and kill-action wiring/removal through `_display_tool_result` terminal branches.

#### Changed

- Re-ran focused terminal/tmux regression set including adapter wiring, bridge permission/dispatch, and launcher lifecycle tests.

## [0.27.0] - 2026-05-08

### Code Changes

#### Added

- `src/agentx/integration/terminal_bridge.py`: added Agentix tool-loop wrapper functions and schema/export helpers:
  - `configure_terminal_bridge(...)`
  - `terminal_run(...)`
  - `terminal_kill_pane(...)`
  - `terminal_list_active_panes(...)`
  - `get_terminal_tool_implementations()` / `get_terminal_tool_schemas()`
- `src/agentx/integration/agentix_bridge_adapter.py`: added `_register_terminal_tools()` and automatic registration during adapter initialization, using `AGENTX_TMUX_SESSION` for session binding.

### Test Changes

#### Added

- `tests/test_agentix_bridge_adapter_coverage.py`: added terminal registration coverage tests for success and exception-swallow paths.

#### Changed

- Validated combined tmux feature tests: launcher lifecycle, permission layer, terminal bridge dispatch, and adapter wiring.

---

## [0.26.0] - 2026-05-08

### Code Changes

#### Added

- `src/agentx/integration/terminal_bridge.py`: added a tmux-backed `TerminalBridge` scaffold with:
  - `run_command(...)` dispatch for visible ephemeral panes and persistent pane targeting,
  - `PermissionLayer` (allow/confirm/deny precedence, supervised/autonomous mode),
  - project-root path checks for absolute command paths,
  - JSONL terminal audit logging via `TerminalResult` records.
- `src/agentx/integration/__init__.py`: exported `TerminalBridge`, `PermissionLayer`, `PermissionDecision`, and `TerminalResult`.

### Documentation Changes

#### Changed

- `docs/ux/05_VIBE_CODING.md`: updated revision stamp to reflect the new bridge scaffold milestone.
- `docs/ux/00_INDEX.md`: updated status snapshot metadata and PD-15 progress counts to reflect tested tmux lifecycle and permission-layer foundation work.

### Test Changes

#### Added

- `tests/test_permission_layer.py`: added 7 hermetic unit tests for allow/confirm/deny matching, autonomous mode behavior, and root path checks.
- `tests/test_terminal_bridge.py`: added 5 hermetic unit tests for tmux dispatch semantics, approval/rejection behavior, deny/path-violation short-circuiting, and audit log writes.

#### Changed

- Continued validation of launcher lifecycle behavior through `tests/test_launch_vibe_shutdown.py` alongside new bridge tests.

---

## [0.25.3] - 2026-05-08

### Code Changes

#### Fixed

- `agentx`: launch `agentx` before starting neovim, then start neovim last so the editor pane target is stable during retries.
- `agentx`: disable tmux `automatic-rename` on the editor window and trust the cached pane id directly so window renames no longer break editor recovery or shutdown.

### Test Changes

#### Changed

- `tests/test_launch_vibe_shutdown.py`: updated the recover-editor expectation to match the pane-id-returning `new-window` invocation.

---

## [0.25.2] - 2026-05-08

### Code Changes

#### Fixed

- `agentx`: fixed editor launch/recovery when tmux rejects named pane targets like `session:editor.0`.
  - Added robust window resolution helpers to map stable window names (`editor`, `agent-bg`, `agentx-log`) to numeric tmux targets at runtime.
  - All `send-keys` and pane command probes now use resolved numeric pane targets (`session:<index>.0`) while preserving name-based intent.
  - Added explicit error messages and hard-fail behavior when required windows cannot be resolved after creation.

### Test Changes

#### Changed

- `tests/test_launch_vibe_shutdown.py`: updated fake tmux harness to be stateful across invocations in a launcher run (session/window creation, kill-session, format-aware list-windows output).
- `tests/test_launch_vibe_shutdown.py`: adjusted assertions to allow dynamic editor index during recover flow while still verifying neovim launch and control signals.

---

## [0.25.1] - 2026-05-08

### Code Changes

#### Fixed

- `agentx`: fixed tmux window targeting to support non-default `base-index` settings.
  - Replaced hardcoded numeric targets (`:0`, `:1`, `:2`) with named targets (`:editor`, `:agent-bg`, `:agentx-log`) so startup and recovery work when tmux window numbering starts at `1`.
  - Prevented startup abort under `set -euo pipefail` when optional window index lookup returns empty.
  - Updated launcher status/help messages to reference named windows and dynamic shortcuts.

### Test Changes

#### Changed

- `tests/test_launch_vibe_shutdown.py`: updated hermetic tmux expectations from numeric window targets to named window targets to validate base-index-safe behavior.

---

## [0.25.0] - 2026-05-08

### Code Changes

#### Added

- **Multi-session socket scoping**: `agentx` now automatically scopes neovim RPC socket and save-notification FIFO paths by tmux session name to prevent collisions between concurrent/sequential sessions.
  - **Path scoping**: defaults changed from `/tmp/agentx.nvim.sock` and `/tmp/agentx_saves.fifo` to `/tmp/agentx_<SESSION_ID>.nvim.sock` and `/tmp/agentx_<SESSION_ID>.saves.fifo`.
  - **SESSION_ID derivation**: defaults to `AGENTX_TMUX_SESSION` (default: `agentx`) for deterministic, user-customizable scoping.
  - **Stale file cleanup**: new `_cleanup_stale_sockets()` function detects and removes stale/orphaned socket and FIFO files on startup, recovering from incomplete prior shutdowns.

#### Fixed

- **Socket collision issue (UAT finding v0.24.1)**: pane 0 failures caused by multiple sessions colliding on hardcoded `/tmp/` paths now resolved through session-scoped IPC paths and automatic stale file cleanup.

### Documentation Changes

#### Changed

- `docs/ux/05_VIBE_CODING.md`: updated Environment Variables table to reflect session-scoped socket/FIFO defaults; added "Multi-Session Collision Prevention" section with examples and explanation of stale file cleanup.
- `agentx` header: updated environment override documentation to explain session scoping and added multi-session support section.

### Test Changes

#### Added

- `tests/test_launch_vibe_shutdown.py`: added `test_multiple_sessions_use_scoped_sockets` unit test verifying that sessions with different `AGENTX_TMUX_SESSION` values use unique scoped socket/FIFO paths without collision.

---

## [0.24.1] - 2026-05-08

### Code Changes

#### Fixed

- `agentx`: resolved UAT regressions from v0.24.0 lifecycle rollout.
  - **Editor pane reliability**: added editor health-check verification and auto-recovery on `start`/reattach path when pane `0.0` is not running `nvim`.
  - **GUI-exit teardown**: AgentX runtime command in window `2` now includes a post-exit hook that tears down the tmux session, preventing orphaned panes after GUI close.
  - **Startup reliability tuning**: added `AGENTX_SOCKET_WAIT_LOOPS` and `AGENTX_SOCKET_WAIT_SEC` to control socket polling behavior in start/recovery paths.

### Documentation Changes

#### Changed

- `docs/ux/UX_ISSUES.md`: recorded UAT failure symptoms for v0.24.0 and added v0.24.1 as the latest fix candidate for the launch shutdown issue.
- `docs/ux/05_VIBE_CODING.md`: updated lifecycle behavior to reflect GUI-exit teardown hook and revised permutation outcomes.

### Test Changes

#### Added

- `tests/test_launch_vibe_shutdown.py`: added `test_start_launches_agentx_with_gui_exit_shutdown_hook` to validate session teardown hook is present in window `2` launch command.

#### Changed

- `tests/test_launch_vibe_shutdown.py`: fake tmux harness extended to report `#{pane_current_command}` for editor-health checks.

---

## [0.24.0] - 2026-05-08

### Code Changes

#### Added

- `agentx`: added explicit lifecycle command surface for consistent session management:
  - `start` (default), `stop`, `status`, `recover-editor`, and `restart`.
  - Introduced deterministic shutdown path (`stop`) that gracefully signals AgentX runtime and neovim before killing tmux session and cleaning stale socket.
  - Introduced editor recovery path (`recover-editor`) that recreates window 0 if missing and relaunches neovim in pane `0.0`.
  - Added status reporting for session/socket/FIFO/window state.

#### Changed

- `agentx`: startup flow refactored into command-oriented helper functions to separate dependency checks, start/stop operations, and editor recovery logic.
- Startup banner now includes explicit lifecycle command hints:
  - `./launch_vibe.sh stop`
  - `./launch_vibe.sh recover-editor`

### Documentation Changes

#### Changed

- `docs/ux/05_VIBE_CODING.md`:
  - Launch Architecture updated to reflect command-based lifecycle model.
  - Added explicit shutdown/recovery permutation matrix.
  - Added affordances `PD-14-AF-008` (recover-editor) and `PD-15-AF-008` (graceful stop command).
  - Added corresponding test traceability entries in Test Scenarios.
- `docs/ux/UX_ISSUES.md`:
  - Marked launch_vibe shutdown lifecycle issue as `[/]` with fix summary and UAT-ready status.

### Test Changes

#### Added

- `tests/test_launch_vibe_shutdown.py` (3 unit tests):
  - Graceful stop signals AgentX and editor before session kill.
  - Stop command is safe no-op when no session exists.
  - Recover-editor recreates missing editor window and relaunches neovim.

#### Changed

- Test harness for launcher lifecycle tests uses hermetic fake `tmux`/`nvim` executables (no real tmux/neovim dependency).

---

## [0.23.2] - 2026-05-06

### Code Changes

#### Fixed

- `src/agentix/bridge/tool_loop.py`: hardened history conversion for LLM calls by filtering internal task-execution roles (`PLAN`, `TASK_NODE`, `SYNTHESIS`, `ASSERTION`) in `_context_to_history()`.
  - Prevents runtime failures when bridge callers serialize history messages with `to_llm_dict()`.
  - Resolves errors of the form: "Message with role MessageRole.PLAN is an internal task-execution record and must not be serialised for the LLM API".

### Test Changes

#### Changed

- Re-ran bridge and loop regression tests after internal-role history filtering.
  - `tests/test_bridge_coverage.py`: pass
  - `tests/test_agentic_loop.py`: pass

## [0.23.1] - 2026-05-06

### Code Changes

#### Fixed

- `src/agentx/streaming_controller.py`: wired live prompt-cycle phase transitions into runtime streaming flow so StatusTab now updates in real time.
  - `classify` now transitions `RUNNING -> DONE` around synchronous prompt classification.
  - `think` transitions to `RUNNING` on THINKING chunks and finalizes when response/tool execution begins.
  - `tool` transitions to `RUNNING` on all TOOL_CALL chunks (with active tool name) and to `DONE` on TOOL_RESULT.
  - `respond` transitions to `RUNNING` on first CONTENT chunk and finalizes when streaming ends.
  - Running phases finalize to `FAILED` on interruption/error and to `DONE` on normal completion.
- `src/agentx/gui/status_tab.py` and `src/agentx/gui/context_meter_widget.py`: improved ContextMeter sizing in StatusTab by hosting donut canvas in a dedicated expanding frame and allowing placement overrides in `ContextMeterWidget.create()`.

### Test Changes

#### Changed

- Re-ran affected GUI and agentic-loop regression tests after runtime phase and sizing fixes.
  - `tests/test_status_tab.py`: pass
  - `tests/test_context_meter_widget.py`: pass
  - `tests/test_agentic_loop.py`: pass

## [0.23.0] - 2026-05-08

### Code Changes

#### Added

- **PD-12 StatusTab implementation**: Real-time prompt-cycle streaming status panel with live phase tracking, context meter relocation, and interrupt control.
  - New `src/agentx/gui/status_tab.py` module: `StatusTab`, `PhaseRow`, `ContextKeyWidget` classes; 11 affordances (AF-001 to AF-011).
  - Auto-activates on stream start; phase rows show Classify / Think / Tool / Respond with state icons (○ / ↻ / ✓ / ✗) and elapsed timers (HH:MM:SS).
  - Interrupt button (`Ctrl+Space`) enabled while streaming.
  - Context meter relocated from `InputPanel` to StatusTab `ContextWindowSection`.
  - `ContextKeyWidget`: colour-key legend synced to meter bands.

#### Changed

- `src/agentx/gui/input_panel.py`: Removed `user_break` button, context meter, and `Ctrl+Space` binding (relocated to StatusTab). Submit button now occupies slim right-column strip (relx=0.97, relwidth=0.03).
- `src/agentx/gui/side_panel.py`: Status tab inserted as first notebook tab via `SidePanel.show_status_tab()` and related delegation methods.
- `src/agentx/gui/gui_manager.py`: `set_streaming_state()` now delegates to both InputPanel and StatusTab; `update_context_meter()` rerouted to StatusTab; new public methods `show_status_tab()`, `reset_status_tab()`, `set_status_phase()`.
- `src/agentx/igui_manager.py` (Protocol): Added `show_status_tab()`, `reset_status_tab()`, `set_status_phase()` method signatures.
- `src/agentx/streaming_controller.py`: `_on_stream_start()` now calls `gui.show_status_tab()` and `gui.reset_status_tab()` for auto-switch and phase reset.

### Test Changes

#### Added

- `tests/test_status_tab.py` (40 unit tests): Full traceability matrix for PD-12 affordances AF-001 through AF-011.
  - `TestFormatElapsed`: 9 parameterized tests for elapsed timer formatting.
  - `TestPhaseRow`: 6 tests for phase row state machine (PENDING / RUNNING / DONE / FAILED transitions, reset, tick).
  - `TestContextKeyWidget`: 2 tests for colour-key legend rendering.
  - `TestStatusTabCreate`: 3 tests for StatusTab instantiation and attribute presence.
  - `TestStatusTabAutoSwitch`: 1 test for `show()` notebook selection.
  - `TestStatusTabInterruptButton`: 4 tests for button state management and callback.
  - `TestStatusTabPhaseReset`: 1 test for phase row reset.
  - `TestStatusTabSetPhase`: 13 parameterized tests for state transitions across all phases and tool-name injection.
  - All tests are hermetic (hidden Tk root, mocked GUIManager) and marked with `@pytest.mark.unit`.

---

## [0.22.20.post2] - 2026-05-07

### Documentation Changes

#### Added

- `docs/ux/03_PANEL_DETAILS.md` — **PD-12: StatusTab** full cut-sheet appended after PD-11. Includes:
  - ASCII placement diagram with three sub-sections (ContextWindowSection, PhaseStepperWidget, InterruptButton)
  - Sub-widget specs: `ContextKeyWidget` (colour-key legend), `ContextMeterWidget` relocation, `PhaseStepperWidget` (vertical phase rows with status icon + elapsed timer), `InterruptButton`
  - Phase step table (Classify / Think / Tool / Respond), status icon table (○ / ↻ / ✓ / ✗), elapsed timer design (1-second `after()` loop, `start_time: float` per row)
  - `IGUIManager` additions: `show_status_tab()`, `set_status_phase()`, `reset_status_tab()`
  - 11 affordances PD-12-AF-001 through PD-12-AF-011 with Gherkin use-cases
  - Full cross-reference table: PD-02, PD-10, PD-03, `StreamingController`
  - Test mapping table (all 11 affordances `📝 Spec Only`)

- `docs/ux/UX_LIFECYCLE.md` — Added PD-12 to panel registry table and traceability matrix (§4) with all 11 affordances at status `📝`.

- `docs/ux/00_INDEX.md` — Added PD-12 StatusTab row (11 spec-only affordances); updated totals; added PD-12 implementation item to Priority Work Queue.

#### Changed

- `docs/ux/03_PANEL_DETAILS.md` — PD-02 InputPanel:
  - `PD-02-AF-004` annotated as ⚠️ Relocated to PD-12-AF-003.
  - Keyboard Shortcuts and Button State tables annotated with pending PD-12 migration note for `Ctrl+Space` binding and `user_break` button.
  - Added "PD-12 layout change" forward-reference note (submit shrinks, text area expands, donut removed from right-column).

- `docs/ux/UX_LIFECYCLE.md` — PD-10 ContextMeterWidget section header annotated with relocation note referencing PD-12-AF-011.

- `docs/ux/UX_ISSUES.md` — "Task Status Issue" updated from `[ ]` to `[/]` (spec complete, awaiting implementation + UAT). Added summary referencing PD-12-AF-001..011.

---

## [0.22.20.post1] - 2026-05-06

### Documentation Changes

#### Changed

- `docs/ux/UX_ISSUES.md`: Recorded user UAT approval for Working Memory collapse-at-startup fix (PD-03-AF-015, v0.22.20). Issue closed.

---

## [0.22.20] - 2026-05-06

### Code Changes

#### Fixed

- `src/agentx/gui/side_panel.py` — `SidePanel.create()`: changed `initial_collapsed=False` to `True` for the `working_memory` session section so it starts collapsed at startup, consistent with History, Available Tools, and Context sections. [PD-03-AF-015]

### Test Changes

#### Changed

- `tests/test_gui_manager_integration.py` — `test_session_sections_start_collapsed`:
  - Added assertion that `working_memory` section `is_expanded() == False` at startup.
  - Updated docstring with Gherkin use-case referencing PD-03-AF-015.
  - **Gherkin**: `GIVEN a freshly created SidePanel / WHEN SidePanel.create() runs / THEN history, tools, working_memory, and context sections are all collapsed`

### Documentation Changes

#### Added

- `docs/ux/03_PANEL_DETAILS.md` — Added affordance spec `PD-03-AF-015` (Working Memory section starts collapsed at startup) with Gherkin use-cases and test mapping.

#### Changed

- `docs/ux/UX_LIFECYCLE.md` — Added `PD-03-AF-015` row to the PD-03 SidePanel affordance matrix.
- `docs/ux/UX_ISSUES.md` — Marked Working Memory collapse-at-startup issue `[/]` (attempted fix; ready for UAT).

---

## [0.22.19.post1] - 2026-05-05

### Documentation Changes

#### Changed

- `docs/ux/UX_ISSUES.md`:
  - Recorded user UAT approval for Phase 5 startup/label selectability screen-scrape fixes.
- `docs/ux/00_INDEX.md`:
  - Updated status note to reflect v0.22.19 startup/label fixes are UAT-confirmed.

---

## [0.22.19] - 2026-05-05

### Code Changes

#### Fixed

- `src/agentx/gui/chat_panel.py` — addressed remaining non-selectable output text paths by replacing `tk.Label` widgets with selectable `tk.Text(state=DISABLED)` widgets.
  - `display_startup_notice`: migrated icon/title/detail widgets to `tk.Text` and bound right-click on each so screen-scrape copy works consistently across startup notice content.
  - `add_plan_tab`: migrated plan title from toolbar `tk.Label` to `tk.Text` with right-click binding.
  - Removed dead `_output_wrapped_labels` tracking path now that output wrapping is handled by tracked text widgets (`_output_detail_text_widgets`).

### Test Changes

#### Added

- `tests/test_startup_log_notice.py`:
  - Added regression test validating startup notice detail body is a disabled selectable `tk.Text` widget with `<Button-3>` binding for context copy behavior.

#### Changed

- `tests/test_startup_log_notice.py`:
  - Updated startup icon test to validate the migrated `tk.Text` icon widget instead of the old `tk.Label` implementation.

### Documentation Changes

#### Changed

- `docs/ux/UX_ISSUES.md`:
  - Recorded user UAT confirmation for the popup dismiss fix from v0.22.18.
  - Added latest fix candidate details for startup/label selectability migration (v0.22.19, ready for UAT).

---

## [0.22.18] - 2026-05-05

### Code Changes

#### Fixed

- `src/agentx/gui/chat_panel.py` — PD-01-AF-010: popup now dismisses on click-away and Escape.
  - Root cause: `overrideredirect(True)` Toplevels do not capture focus, so `<Escape>` never fired and there was no click-away handler.
  - Added `popup.grab_set()` after `popup.deiconify()` so the popup receives all keyboard and mouse events.
  - Added `_on_outside_click` inner function bound to `<ButtonPress>` — checks if `event.x`/`event.y` fall outside the popup dimensions and calls `_dismiss_output_context_popup()` if so.

- `src/agentx/gui/input_panel.py` — PD-02-AF-008: same fix applied to input panel popup.
  - Added `popup.grab_set()` after `popup.deiconify()`.
  - Added `_on_outside_click` inner function bound to `<ButtonPress>`.

### Test Changes

#### Added

- `tests/test_chat_panel_copy_context_menu.py` — `TestOutputPopupDismissBehavior` (3 new tests):
  - GIVEN popup visible WHEN ButtonPress at x=-50, y=-50 THEN popup dismissed
  - GIVEN popup visible WHEN ButtonPress inside bounds THEN popup NOT dismissed
  - GIVEN popup visible WHEN grab_current() queried THEN returns popup Toplevel

- `tests/test_input_panel_context_menu.py` — `TestInputPopupDismissBehavior` (3 new tests, identical structure):
  - GIVEN popup visible WHEN ButtonPress at x=-50, y=-50 THEN popup dismissed
  - GIVEN popup visible WHEN ButtonPress inside bounds THEN popup NOT dismissed
  - GIVEN popup visible WHEN grab_current() queried THEN returns popup Toplevel

---

## [0.22.17] - 2026-05-03

### Code Changes

#### Fixed

- `src/agentx/gui/chat_panel.py` — PD-01-AF-010: fixed two UAT-reported regressions:
  1. **Right-click popup never appeared**: `<Button-3>` was bound to the hidden `output_text` widget (inside `_hidden_text_container`, never packed into the visible layout). Fixed by adding `<Button-3>` bindings directly to `header_text` and `detail_text` widgets inside `_create_output_entry`.
  2. **Header text not selectable**: `header_label` was a `tk.Label` (no text selection). Replaced with `header_text = tk.Text(state=DISABLED)` to enable mouse-drag selection and Ctrl-C copy. Added `StringVar.trace_add("write", ...)` callback to keep `header_text` content in sync when `header_var.set(...)` is called. Added `header_text` to `_output_detail_text_widgets` for auto-height management.
  - New method `_on_entry_text_right_click(event, target)` — passes the specific entry widget to `_show_output_context_menu`.
  - Updated `_show_output_context_menu(x_root, y_root, target=None)` — accepts optional `target: Optional[tk.Text]`; uses `copy_target = target if target is not None else self._widgets.output_text`; `_do_copy()` calls `copy_target.event_generate("<<Copy>>")`.

### Test Changes

#### Added

- `tests/test_chat_panel_copy_context_menu.py` — 8 new tests in `TestEntryLevelRightClickCopy`:
  - GIVEN an entry is created via `_create_output_entry` WHEN we inspect `header_text` THEN it is a `tk.Text` (not `tk.Label`)
  - GIVEN an entry WHEN `header_var.set(...)` is called THEN `header_text` content is updated
  - GIVEN `header_text` WHEN bindings inspected THEN `<Button-3>` is present
  - GIVEN `detail_text` WHEN bindings inspected THEN `<Button-3>` is present
  - GIVEN `detail_text` WHEN `_on_entry_text_right_click` called THEN popup is created
  - GIVEN `header_text` WHEN `_on_entry_text_right_click` called THEN popup is created
  - GIVEN `detail_text` with selection WHEN Copy button clicked via `_show_output_context_menu(..., target=detail_text)` THEN `<<Copy>>` generated on `detail_text` not on hidden `output_text`
  - GIVEN `header_text` WHEN state inspected THEN state is `disabled`

---

## [0.22.16] - 2026-05-03

### Code Changes

#### Added

- `src/agentx/gui/chat_panel.py` — PD-01-AF-010: right-click context menu on output panel.
  - `_MENU_POST_DELAY_MS` class attribute (default 100 ms, overridable to 0 in tests).
  - `_on_output_right_click()` — schedules popup via `after()` and returns `"break"`.
  - `_use_wayland_popup()` — detects Wayland via `XDG_SESSION_TYPE`.
  - `_dismiss_output_context_popup()` — safe Toplevel destroy.
  - `_show_output_context_menu()` — fresh `tk.Toplevel(overrideredirect=True)` with "Copy" button; calls `output.event_generate("<<Copy>>")` on click; themed with `output_bg`/`ui_fg`/`muted_fg`.
  - `<Button-3>` binding added in `_bind_output_text_shortcuts()`.

- `src/agentx/gui/input_panel.py` — PD-02-AF-008..012: right-click context menu on user input widget.
  - `_MENU_POST_DELAY_MS` class attribute (default 100 ms, overridable to 0 in tests).
  - `_on_input_right_click()` — schedules popup via `after()` and returns `"break"`.
  - `_dismiss_input_context_popup()` — safe Toplevel destroy.
  - `_clipboard_has_content()` — `try/except tk.TclError` guard for portable empty-clipboard detection.
  - `_on_input_context_copy()` — `event_generate("<<Copy>>")` + dismiss.
  - `_on_input_context_paste()` — explicit `delete(SEL_FIRST, SEL_LAST)` + `mark_set(INSERT, sel_start)` + `insert(INSERT, text)` for deterministic paste-replace behaviour; `try/except tk.TclError` guard for empty clipboard.
  - `_show_input_context_menu()` — conditional "Copy" (if SEL) and "Paste" (if clipboard non-empty); no popup when neither applies; fresh Toplevel per invocation; themed with `input_bg`/`input_fg`/`muted_fg`.
  - `<Button-3>` binding added in `create()`.

#### Fixed

- `src/agentx/gui/input_panel.py` — paste insert-point bug: after `delete(SEL_FIRST, SEL_LAST)`, Tk drifts INSERT to end of remaining text; fixed by explicitly `mark_set(INSERT, sel_start)` before the insert call, ensuring pasted text lands at the deletion point.

### Test Changes

#### Added

- `tests/test_chat_panel_copy_context_menu.py` — 8 hermetic unit tests for PD-01-AF-010:
  - GIVEN `<Button-3>` binding registered WHEN queried THEN present on `output_text`.
  - GIVEN right-click fires WHEN delay fires THEN `_output_context_popup` is a live Toplevel.
  - GIVEN popup created WHEN inspected THEN contains "Copy" button.
  - GIVEN SEL set WHEN "Copy" invoked THEN `<<Copy>>` generated on `output_text`.
  - GIVEN "Copy" clicked WHEN handler runs THEN popup is dismissed.
  - GIVEN Escape handler called WHEN runs THEN popup is None.
  - GIVEN first popup visible WHEN second right-click fires THEN first popup replaced.
  - GIVEN theme set WHEN popup created THEN `bg == config.output_bg`.

- `tests/test_input_panel_context_menu.py` — 17 hermetic unit tests for PD-02-AF-008..012:
  - `TestInputPanelRightClickPopup` (6 tests): binding registered; popup created with selection; Escape dismisses; stale popup replaced; themed bg; no popup when neither Copy nor Paste applicable.
  - `TestInputCopyMenuVisibility` (2 tests): "Copy" present with SEL, absent without.
  - `TestInputPasteMenuVisibility` (4 tests): "Paste" present with clipboard, absent without; `_clipboard_has_content` returns False on empty, True when filled.
  - `TestInputCopyAction` (2 tests): copies selection to clipboard; dismisses popup.
  - `TestInputPasteAction` (3 tests): replaces selection with clipboard text; inserts at cursor when no selection; dismisses popup.

### Documentation Changes

#### Changed

- `docs/ux/UX_ISSUES.md`:
  - Recorded user UAT approval for the startup log-location notice issue.
  - Updated UAT status from pending verification to user-approved.
- `docs/ux/00_INDEX.md`:
  - Updated status snapshot to reflect UAT-approved state for PD-01 startup notice.

---

## [0.22.15] - 2026-05-02

### Code Changes

#### Changed

- Startup log-location notice icon styling in output window:
  - Updated icon to `ⓘ` per UAT preference.
  - Rendered startup icon in bold and slightly larger font for improved visibility.

### Test Changes

#### Added

- `tests/test_startup_log_notice.py`:
  - Added regression test validating `ⓘ` icon presence and bold/larger startup icon styling.

### Documentation Changes

- `docs/ux/UX_ISSUES.md`: Recorded UAT nit attempted fix for startup icon visibility.
- `docs/ux/00_INDEX.md`: Updated status snapshot to v0.22.15 candidate.

---

## [0.22.14] - 2026-05-02

### Code Changes

#### Added

- Startup log-location notice in the output window before first agent response.
  - `AgentXSession._show_startup_log_locations_notice_if_enabled()` now renders
    friendly paths for session/runtime logs during layout.
  - New config gate: `agentx.show_log_locations_on_startup` in `agentx.toml`
    (default: `true`), allowing users to suppress the notice.

#### Changed

- `ChatPanel`/`GUIManager` now support `display_startup_notice()` for system-style,
  non-agent informational startup messages.

### Test Changes

#### Added

- `tests/test_startup_log_notice.py`:
  - startup notice displays by default and includes expected log paths.
  - startup notice suppression when config flag is false.
  - layout order guarantees notice displays before bootstrap response rendering.

### Documentation Changes

- Added UX use-cases and traceability for startup log-location messaging:
  - `docs/ux/03_PANEL_DETAILS.md` (PD-01-AF-009 + Gherkin stories)
  - `docs/ux/02_USER_FLOWS.md` (UF-13)
  - `docs/ux/UX_LIFECYCLE.md` (traceability matrix row)
  - `docs/ux/UX_ISSUES.md` (issue moved to ready-for-UAT)
  - `docs/ux/00_INDEX.md` (status snapshot)

---

## [0.22.13.post1] - 2026-05-02

### Documentation Changes

#### Changed

- `docs/ux/UX_ISSUES.md`:
  - Recorded user UAT approval for PD-11 right-click popup fixes.
  - Updated UAT status from pending verification to user-approved.
- `docs/ux/00_INDEX.md`:
  - Updated status snapshot to reflect UAT-approved state for PD-11.
- `docs/ux/02_USER_FLOWS.md`:
  - Added `UF-12` main-window popup rendering invariants for Wayland fallback.
  - Fixed malformed section structure around `UF-11`/`UF-12` so sequence diagrams
    render correctly and remain traceable for regression prevention.

---

## [0.22.13] - 2026-05-02

### Code Changes

#### Changed

- `src/agentx/file_explorer.py` Wayland fallback popup rendering:
  - Applied themed `panel_bg` color directly to the popup `tk.Toplevel` at creation.
  - Set `borderwidth=0` and `highlightthickness=0` on the `Toplevel` to avoid default
    light window styling during first compositor frame.

### Test Changes

#### Added

- `tests/test_file_explorer_context_menu.py`: Added a unit regression test asserting
  the Wayland popup `Toplevel` background is initialized from theme color.

### Documentation Changes

- `docs/ux/UX_ISSUES.md`: Added RC14 attempted fix notes and increased attempt count to 14.
- `docs/ux/00_INDEX.md`: Updated Last updated status to v0.22.13 candidate.

---

## [0.22.12] - 2026-05-02

### Code Changes

#### Changed

- `src/agentx/file_explorer.py` Wayland fallback popup lifecycle:
  - Explicitly dismisses any active fallback popup before scheduling a new right-click popup.
  - Added `_destroy_wayland_popup()` and now recreates a fresh `tk.Toplevel`
    surface on each `_show_wayland_popup()` call.
  - This avoids intermittent non-visual-but-clickable stale popup state seen in UAT.

### Test Changes

#### Added

- `tests/test_file_explorer_context_menu.py`: Added a regression test that verifies
  consecutive Wayland popup shows recreate the popup window.

### Documentation Changes

- `docs/ux/UX_ISSUES.md`: Added RC13 attempted fix notes and increased attempt count to 13.
- `docs/ux/00_INDEX.md`: Updated Last updated status to v0.22.12 candidate.

---

## [0.22.11] - 2026-05-02

### Code Changes

#### Changed

- `src/agentx/file_explorer.py` Wayland fallback popup behavior:
  - Removed `<FocusOut>` auto-dismiss binding from the fallback `tk.Toplevel` popup.
  - Removed `focus_force()` on popup show to avoid immediate focus churn.
  - Stabilized popup geometry before mapping (`update_idletasks()` + explicit
    width/height in geometry string) to reduce transient oversized flash artifacts.

### Documentation Changes

- `docs/ux/UX_ISSUES.md`: Added RC12 as latest attempted fix candidate, updated
  attempt count to 12, and preserved UAT ownership.

---

## [0.22.10] - 2026-05-02

### Code Changes

#### Changed

- `src/agentx/file_explorer.py`: Added a Wayland-specific context popup fallback.
  When `XDG_SESSION_TYPE=wayland` (or forced in tests), right-click now opens an
  in-app `tk.Toplevel` popup with button actions instead of Tk menu windows.
- Existing delayed scheduling remains in place (`_MENU_POST_DELAY_MS`).
- Existing non-Wayland behavior remains unchanged (`tk_popup` path with
  verification/retry logic).

### Test Changes

#### Added

- `tests/test_file_explorer_context_menu.py`: Added `TestWaylandPopupFallback`
  with two unit tests:
  - verifies forced Wayland mode routes to `_show_wayland_popup()` and bypasses
    `_post_menu()`.
  - verifies `_dismiss_popup_menu()` withdraws the Wayland fallback popup.

#### Changed

- Updated tk-popup assertions in existing tests to force non-Wayland mode
  (`fe._FORCE_WAYLAND_POPUP = False`) so test intent is explicit under Wayland CI.

### Documentation Changes

- `docs/ux/UX_ISSUES.md`: Added RC11 as the latest attempted fix candidate,
  updated attempt count to 11, and kept UAT ownership explicit.
- `docs/ux/00_INDEX.md`: Updated status snapshot line for v0.22.10.

---

## [0.22.9] - 2026-05-02

### Code Changes

#### Changed

- `src/agentx/file_explorer.py` `_post_menu()`: changed popup primitive from
  `menu.post()` to `menu.tk_popup()` with guarded `menu.grab_release()` in
  `finally`.
- Kept delayed scheduling (`_MENU_POST_DELAY_MS`) and generation-aware visibility
  verification (`_MENU_POST_VERIFY_DELAY_MS`, `_menu_post_generation`) so retry
  behavior remains bounded and stale clicks cannot re-open older menus.

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`:
  - Renamed file/folder right-click tests to assert `tk_popup()` behavior.
  - Updated mocks to verify `grab_release()` is called after popup display.
- `tests/test_file_explorer_menu_coordinates.py`:
  - Updated coordinate assertion test to validate `tk_popup()` receives safe
    coordinates rather than `post()`.

### Documentation Changes

- `docs/ux/UX_ISSUES.md`: Added RC10 as latest attempted fix candidate and
  updated attempt count to 10, preserving user-owned UAT status.

---

## [0.22.8] - 2026-05-02

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py` `_on_right_click()`: Replaced `after_idle` with
  `after(_MENU_POST_DELAY_MS)` (default 100 ms).  Root cause (RC9): `after_idle`
  fires **before** the button is physically released — the `<Button-3>` press event
  schedules the idle callback, the callback fires while the user still holds the
  button, the menu posts, and the subsequent `<ButtonRelease-3>` lands on the
  newly-posted menu window.  `tk::MenuInvoke` finds no active item and calls
  `unpost()` immediately, so the menu disappears in < 1 frame.  With `after(100)` the
  menu posts 100 ms after the press, by which time the button has always been
  released on the treeview (no binding there → no unpost).

#### Architecture / Design

- Added `_MENU_POST_DELAY_MS: int = 100` class attribute to `FileExplorer`.
  Set to `0` in unit tests combined with `root.update()` for zero-latency test runs.

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`: Both right-click tests now set
  `fe._MENU_POST_DELAY_MS = 0` and call `root.update()` (was `update_idletasks()`)
  to fire `after(0)` callbacks.
- `tests/test_file_explorer_menu_coordinates.py`: Same change to
  `test_on_right_click_uses_safe_coords_for_post`.

### Documentation Changes

- `docs/ux/UX_LIFECYCLE.md §6`: Corrected subsection title and content from
  `after_idle` to `after(100)`; added test pattern for `_MENU_POST_DELAY_MS = 0`.
- `docs/ux/UX_ISSUES.md`: RC9 documented; issue fix count updated to 9; committed
  version updated to v0.22.8.
- `docs/ux/00_INDEX.md`: Last-updated date entry updated.

---

## [0.22.7] - 2026-05-02

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py` `_on_right_click()`: Replaced `event.x_root` /
  `event.y_root` with `self.tree.winfo_rootx() + event.x` / `self.tree.winfo_rooty() +
  event.y` for the coordinates passed to `_post_menu()`.  Root cause (RC8): under
  Wayland/XWayland the raw X11 event coordinates are in physical-pixel virtual-screen
  space.  On a HiDPI or multi-monitor setup this can place `menu.post()` far outside any
  visible monitor region (UAT observation: `x_root=3753`).  Tk's `winfo_rootx/y()`
  queries the widget's logical on-screen anchor; adding the widget-relative `event.x/y`
  offsets produces a coordinate that is always inside the visible window.

#### Architecture / Design

- **Wayland/XWayland coordinate safety rule** documented as a standing platform design
  principle in `docs/ux/UX_LIFECYCLE.md §6`.  All future affordances that call
  `menu.post()`, `wm_geometry()`, or any absolute-position popup must follow this rule.

### Test Changes

#### Added

- `tests/test_file_explorer_menu_coordinates.py` — 7 new `pytest.mark.functional` tests
  in `TestMenuCoordinateSafety` covering affordance PD-11-AF-008:
  - GIVEN winfo_rootx + event.x / WHEN widget is on-screen / THEN coords within screen bounds
  - GIVEN raw XWayland x_root values (parametrized: 3753/500, 5000/3000, 0/0, 100/200) /
    WHEN compared against screen dimensions / THEN safe strategy always in bounds
  - GIVEN safe coordinates / WHEN _post_menu() called with real menu.post() /
    THEN menu.winfo_ismapped()=1 AND menu position within screen bounds
  - GIVEN synthetic event with bad x_root=9999 / WHEN_on_right_click() fires /
    THEN menu.post() called with winfo_rootx+event.x NOT with raw x_root (regression guard)

### Documentation Changes

- `docs/ux/UX_LIFECYCLE.md`: New subsection **"Platform Design Principle: Wayland /
  XWayland Coordinate Safety"** at the top of §6.  Covers: background on XWayland
  virtual framebuffer, the rule (never use `event.x_root/y_root` for popup placement),
  the correct pattern, why `after_idle` is also required, and testing guidelines.
- `docs/ux/UX_ISSUES.md`: Right-click issue marked `[/]`; RC8 fully documented alongside
  RC1–RC7 history.
- `docs/ux/00_INDEX.md`: Last-updated date bumped to 2026-05-02.

---

## [0.22.6] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py`: Changed right-click trigger binding from
  `<ButtonRelease-3>` to `<Button-3>` (press).  Root cause identified: when
  `menu.post()` is called from a `<ButtonRelease-3>` handler it creates the menu window
  at `(x_root, y_root)` — directly under the cursor — so the X server sends an `<Enter>`
  event to the new menu window.  The Tk `Menu` class has a generic `<ButtonRelease>`
  class binding (`tk::MenuInvoke`) that fires when any button is released over the menu;
  with no active item it calls `unpost()`.  Result: menu appeared and immediately
  vanished.  With `<Button-3>` (press) and `menu.post()` (no grab), the subsequent
  `<ButtonRelease-3>` goes to whichever window the cursor is over at release time — the
  menu (item invoked ✓) or the treeview (ignored ✓) — never triggering the auto-unpost.
  `<Control-ButtonRelease-1>` similarly changed to `<Control-Button-1>`.

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`:
  - Renamed `test_right_click_bound_to_button_release_not_press` →
    `test_right_click_bound_to_button_press_not_release`; assertion now verifies
    `<Button-3>` is bound and `<ButtonRelease-3>` is NOT.
  - Updated module docstring to document full root cause history (v0.22.1–0.22.6).
  - All 15 tests pass.

---

## [0.22.5] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py` `_on_right_click()`: Replaced `tk_popup()` with
  `menu.post()`.  All previous fixes (v0.22.1–0.22.4) were workarounds to symptoms of
  `tk_popup()`'s internal `grab` command.  On any modern Linux compositor the WM cancels
  Tk's grab immediately after `tk_popup()` sets it — there is no reliable way to keep the
  grab.  `menu.post()` displays the menu without setting any grab.  Tk's native
  root-window `<ButtonPress>` binding handles auto-dismiss; `<Escape>` remains bound.
  Removed the now-unnecessary `after_idle(grab_release)` call.

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`: Updated file and folder right-click tests
  to assert `menu.post()` is called and `tk_popup()` is NOT called.  Added docstring
  note documenting the inherent limitation of unit tests for this behavior (compositor
  grab conflicts cannot be detected headlessly; manual UAT is required).

---

## [0.22.4] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py` `_on_right_click()`: Replaced synchronous `try/finally`
  `grab_release()` with `menu.after_idle(menu.grab_release)` and added `return "break"`.
  The synchronous call fired before Tk drained its event queue — queued grab-related events
  could still cause `unpost()` afterward, producing the intermittent 1-in-12 success rate.
  `after_idle` defers the release until the queue is empty.  `return "break"` stops the
  `<ButtonRelease-3>` event from propagating to parent widgets or root-window bindings that
  could also close the menu.

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`: Updated file and folder right-click tests
  to assert `after_idle(grab_release)` is scheduled and handler returns `"break"`,
  replacing the previous synchronous `grab_release` assertions.

---

## [0.22.3] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py` `_on_right_click()`: Added `try/finally` block that calls
  `menu.grab_release()` immediately after `tk_popup()`.  On Linux with modern compositors
  (GNOME/Mutter, KWin, Wayland/XWayland), `tk_popup()` sets a server-side passive grab
  that conflicts with the WM's own grab.  The WM resolves the conflict by cancelling Tk's
  grab, which causes Tk to unpost the menu immediately.  `grab_release()` frees the
  conflicting grab while leaving the menu posted; Tk's native `<Leave>` and root
  `ButtonPress` bindings still handle auto-dismiss correctly.
  This is root cause #3; causes #1 (FocusOut) and #2 (ButtonRelease binding) were fixed
  in v0.22.1 and v0.22.2 respectively.

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`: Updated `test_right_click_file_calls_tk_popup_on_file_menu`
  and `test_right_click_directory_calls_tk_popup_on_folder_menu` to also assert
  `grab_release()` is called after `tk_popup()`.

### Documentation Changes

#### Changed

- `docs/ux/UX_ISSUES.md`: Added root cause #3 and side-effect explanation (accidental
  Attach trigger); updated attempt count to 3.
- `docs/ux/03_PANEL_DETAILS.md` PD-11: Extended dismiss-behaviour note to cover
  `grab_release()` rationale.

---

## [0.22.2] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py`: Changed right-click binding from `<Button-3>` to
  `<ButtonRelease-3>` (and `<Control-Button-1>` → `<Control-ButtonRelease-1>`).
  On Linux/X11, `tk_popup()` sets up an internal grab when it opens.  When the binding
  was on the button-press event, the corresponding button-release event was captured by
  that grab and immediately dismissed the menu.  Binding to the release event means the
  release has already been consumed before the popup opens, so the menu stays visible.
  This is the second fix for UX_ISSUES.md issue #1 (first fix in v0.22.1 removed the
  FocusOut binding).

### Test Changes

#### Changed

- `tests/test_file_explorer_context_menu.py`: Added
  `test_right_click_bound_to_button_release_not_press` to `TestFileContextMenu` — asserts
  `<ButtonRelease-3>` is bound and `<Button-3>` is absent.  15 tests total (was 14).

### Documentation Changes

#### Changed

- `docs/ux/UX_ISSUES.md`: Issue #1 updated with second root-cause and marked `[/]`.
- `docs/ux/03_PANEL_DETAILS.md` PD-11: Updated the dismiss-behaviour note to explain
  both the ButtonRelease and FocusOut rationale.

---

## [0.22.1] - 2026-05-01

### Code Changes

#### Fixed

- `src/agentx/file_explorer.py`: Removed the `<FocusOut>` binding from the treeview
  in `to_gui()`.  The binding was calling `_dismiss_popup_menu()` the instant
  `tk_popup()` stole focus from the tree, causing context menus to appear and
  immediately vanish on Linux/X11.  `<Escape>` and clicking outside the menu remain
  as the natural dismiss paths.  Fixes UX_ISSUES.md issue #1.

### Test Changes

#### Added

- `tests/test_file_explorer_context_menu.py` — 14 `@pytest.mark.unit` tests across
  three classes covering the new affordances:

  `TestFileContextMenu` (PD-11-AF-008):
  - GIVEN file row right-clicked THEN file menu posted, folder menu not.
  - GIVEN empty area right-clicked THEN no menu posted.
  - GIVEN widget created THEN `<FocusOut>` is NOT bound on the tree.
  - GIVEN file selected WHEN Attach activated THEN on_attach called with correct path.
  - GIVEN file selected WHEN Edit activated THEN on_edit called with correct path.
  - GIVEN no on_attach callback THEN no exception raised on activation.

  `TestFolderContextMenu` (PD-11-AF-009):
  - GIVEN directory row right-clicked THEN folder menu posted, file menu not.
  - GIVEN directory selected WHEN "Add full path" activated THEN callback with abs path.
  - GIVEN directory selected WHEN "Add relative path" activated THEN callback with rel path.
  - GIVEN no callback THEN no exception raised on activation.

  `TestDismissContextMenu` (PD-11-AF-010):
  - GIVEN widget created THEN `<Key-Escape>` is bound on the tree.
  - GIVEN _dismiss_popup_menu called THEN unpost called on both menus.
  - GIVEN no event arg THEN _dismiss_popup_menu does not raise.
  - GIVEN synthetic event THEN _dismiss_popup_menu does not raise.

### Documentation Changes

#### Added

- `docs/ux/UX_ISSUES.md`: Updated issue #1 to `[/]` with root-cause and fix notes.

#### Changed

- `docs/ux/03_PANEL_DETAILS.md` PD-11 section:
  - Corrected "Escape / focus lost" dismiss interaction to "Escape only" with an
    explanatory note about why FocusOut is not used.
  - Added Gherkin use-cases for PD-11-AF-008, PD-11-AF-009, PD-11-AF-010.
- `docs/ux/UX_LIFECYCLE.md` PD-11 matrix: added rows for PD-11-AF-008, PD-11-AF-009,
  PD-11-AF-010 (all ✅).
- `docs/ux/00_INDEX.md`: PD-11 row `7✅` → `10✅`; totals `61✅` → `64✅`.

---

## [0.22.0] - 2026-05-01

### Code Changes

#### Added

- `src/agentx/gui/model_selector.py`: Added `on_refresh` constructor parameter,
  `refresh_btn` (`ttk.Button` with glyph `⟳`), `set_refresh_callback()`, and
  `_on_refresh()`. Pressing the button invokes the registered callback, which is
  wired through the full chain to reload the Ollama model list. Implements PD-04-AF-004.
- `src/agentx/gui/side_panel.py`: Added `set_refresh_models_callback()` — forwards
  the callback down to `ModelSelector`.
- `src/agentx/gui/gui_manager.py`: Added `set_refresh_models_callback()` delegate.
- `src/agentx/igui_manager.py`: Added `set_refresh_models_callback()` Protocol method.
- `src/agentx/session.py`: Added `_refresh_models()` inner function and
  `gui.set_refresh_models_callback(_refresh_models)` call in `_setup_agentix_ui()`.

### Test Changes

#### Added

- `tests/test_model_selector_refresh.py` — 8 `@pytest.mark.unit` tests in
  `TestModelSelectorRefreshButton` covering PD-04-AF-004:
  - GIVEN ModelSelector created THEN refresh_btn is a ttk.Button.
  - GIVEN ModelSelector created THEN refresh_btn is packed inside the widget frame.
  - GIVEN ModelSelector created THEN refresh_btn text is "⟳".
  - GIVEN refresh callback registered WHEN button clicked THEN callback invoked once.
  - GIVEN no callback registered WHEN button clicked THEN no exception raised.
  - GIVEN existing callback WHEN set_refresh_callback() called with new callback THEN only new callback invoked.
  - GIVEN no callback WHEN set_refresh_callback() called THEN late-registered callback works on next click.
  - GIVEN callback set WHEN set_refresh_callback(None) called THEN subsequent click does not raise.

- `tests/test_plan_tree_affordances.py` — 16 `@pytest.mark.unit` tests across three
  classes covering PD-05-AF-004, PD-05-AF-005, PD-05-AF-006:

  `TestNodeStatusIconReflectsState` (PD-05-AF-006):
  - GIVEN pending status WHEN update_node_status called THEN icon is ○.
  - GIVEN running status WHEN update_node_status called THEN icon is ●.
  - GIVEN done status WHEN update_node_status called THEN icon is ✓.
  - GIVEN needs_review status WHEN update_node_status called THEN icon is ?.
  - GIVEN failed status WHEN update_node_status called THEN icon is ✗.
  - GIVEN unknown status WHEN update_node_status called THEN fallback bullet shown, no exception.
  - GIVEN missing task_id WHEN update_node_status called THEN no exception.
  - GIVEN multiple status transitions THEN each call updates icon correctly.

  `TestResynthButtonInSynthesisBlock` (PD-05-AF-004):
  - GIVEN on_resynth callback provided WHEN add_synthesis_to_node called THEN Re-synth button present.
  - GIVEN no on_resynth callback WHEN add_synthesis_to_node called THEN no Re-synth button.
  - GIVEN on_resynth callback THEN callback stored on synthesis widget for later invocation.
  - GIVEN missing task_id with on_resynth callback THEN no exception.

  `TestExportButtonInPlanTab` (PD-05-AF-005):
  - GIVEN plan tab created THEN Export button present in toolbar.
  - GIVEN plan tab with on_export callback WHEN Export clicked THEN callback invoked once.
  - GIVEN plan tab with no on_export callback THEN Export button still present.
  - GIVEN no on_export callback WHEN Export clicked THEN no exception.

### Documentation Changes

#### Changed

- `docs/ux/UX_LIFECYCLE.md`:
  - PD-04-AF-004 📝 → ✅; corrected source from non-existent `_on_refresh()` (method
    now added) and added test file/class refs.
  - PD-05-AF-004 📝 → ✅; corrected source from `_add_resynth_button()` to actual
    `_create_synthesis_block()`; added test file/class refs.
  - PD-05-AF-005 📝 → ✅; corrected source from `_on_export()` to `ChatPanel.add_plan_tab()`
    and `AgentXSession._export_task_tree()`; added test file/class refs.
  - PD-05-AF-006 📝 → ✅; corrected source from `_node_icon()` to `update_node_status()`
    and `_STATUS_ICONS`; added test file/class refs.
- `docs/ux/00_INDEX.md`:
  - PD-04 row: `3✅·1📝` → `4✅·0📝`.
  - PD-05 row: `3✅·3📝` → `6✅·0📝`.
  - Totals: `57✅·4📝` → `61✅·0📝`.
  - Priority Work Queue: added 4 `[/]` completed entries for PD-04-AF-004, PD-05-AF-004/005/006.
- `docs/ux/03_PANEL_DETAILS.md`:
  - PD-04 section: added Gherkin use-cases for PD-04-AF-004.
  - PD-05 Controls section: added Gherkin use-cases for PD-05-AF-004, PD-05-AF-005, PD-05-AF-006.

---

## [0.21.0] - 2026-04-30

### Code Changes

#### Added

- `src/agentx/gui/input_panel.py`: Implemented `_on_shift_return()` method and wired a
  `<Shift-Return>` binding on `user_input_text` in `InputPanel.create()`. Pressing
  Shift+Enter now inserts a newline at the cursor position and returns `"break"` to
  suppress Tkinter's default key handling. Implements PD-02-AF-002.

### Test Changes

#### Added

- `tests/test_input_panel_keyboard.py` — 5 `@pytest.mark.unit` tests in
  `TestShiftEnterInsertsNewline` covering PD-02-AF-002:
  - GIVEN input text is empty WHEN Shift+Enter pressed THEN widget contains a newline.
  - GIVEN input contains "hello" WHEN Shift+Enter pressed THEN content is "hello\\n".
  - GIVEN InputPanel created WHEN _on_shift_return called THEN return value is "break".
  - GIVEN InputPanel created WHEN bindings queried THEN Shift+Return binding present.
  - GIVEN input contains "ab" with cursor mid-word WHEN Shift+Enter pressed THEN content is "a\\nb".

### Documentation Changes

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: Added 5 Gherkin use-cases for PD-02-AF-002 and 5
  test-mapping rows for `test_input_panel_keyboard.py` in the PD-02 cut-sheet.
- `docs/ux/UX_LIFECYCLE.md`: PD-02-AF-002 📝 → ✅; corrected source method reference from
  non-existent `_bind_keys()` to the actual `_on_shift_return()` implementation.
- `docs/ux/00_INDEX.md`: PD-02 InputPanel row updated (4✅·3⚠️·0📝), totals
  (57✅·4📝), queue item `PD-02-AF-002` appended and marked complete.

---

## [0.20.3] - 2026-05-02

### Code Changes

#### Fixed

- `src/agentx/gui/settings_tab.py`: `_add_text_entry()` and `_add_spinbox()` now correctly
  append `RESTART_ICON` to the label when `restart=True` is passed, matching the existing
  behaviour of `_add_checkbox()`. Previously the `restart` parameter was accepted but silently
  ignored in both helpers, causing the 🔁 icon to be absent from Ollama Host and Load timeout
  labels. Callers that were compensating by manually including `RESTART_ICON` in the label
  string (`Agentix Host`, `Torch model`, `Torch device`) have had the explicit suffix removed
  to avoid duplication.

### Test Changes

#### Added

- `tests/test_settings_tab_sections.py` — 19 `@pytest.mark.unit` tests across 2 classes
  covering PD-07-AF-002 and PD-07-AF-003:
  - `TestSettingsTabSectionCollapseDefaults` (5 tests — PD-07-AF-002):
    - GIVEN SettingsTab constructed WHEN 🎨 Appearance section inspected THEN expanded=True.
    - GIVEN SettingsTab constructed WHEN 🤖 Ollama section inspected THEN expanded=True.
    - GIVEN SettingsTab constructed WHEN 🧠 Agentix section inspected THEN expanded=True.
    - GIVEN SettingsTab constructed WHEN 📊 Classification Display inspected THEN expanded=False.
    - GIVEN SettingsTab constructed WHEN 🏛️ Working Memory inspected THEN expanded=False.
  - `TestRestartIconInLabels` (14 tests — PD-07-AF-003):
    - GIVEN SettingsTab constructed WHEN RESTART_ICON constant read THEN equals `🔁`.
    - GIVEN SettingsTab constructed WHEN Theme mode label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Host labels located THEN at least one contains 🔁.
    - GIVEN SettingsTab constructed WHEN Load timeout label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Screen side label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Default model label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Enabled (WM) label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Torch model label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Torch device label located THEN text contains 🔁.
    - GIVEN SettingsTab constructed WHEN Classify prompts label located THEN no 🔁.
    - GIVEN SettingsTab constructed WHEN Debug logging label located THEN no 🔁.
    - GIVEN SettingsTab constructed WHEN Backend label located THEN no 🔁.
    - GIVEN SettingsTab constructed WHEN Inject into LLM context label located THEN no 🔁.
    - GIVEN SettingsTab constructed WHEN Max facts label located THEN no 🔁.

### Documentation Changes

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: Added PD-07-AF-002 (section collapse defaults) and
  PD-07-AF-003 (restart-required icon in label text) cut-sheet blocks in the PD-07 section.
- `docs/ux/UX_LIFECYCLE.md`: PD-07-AF-002 📝 → ✅, PD-07-AF-003 📝 → ✅; corrected
  PD-07-AF-003 source method reference from non-existent `_make_restart_tooltip()` to the
  actual implementation (`RESTART_ICON` class constant + widget factory helpers); removed both
  from the Medium Priority gaps section.
- `docs/ux/00_INDEX.md`: PD-07 SettingsTab row updated (2✅·1⚠️·0📝), totals
  (56✅·5📝), queue item `PD-07-AF-002..003` marked complete; last-updated date refreshed.

---

## [0.20.2.post7] - 2026-04-30

### Code Changes

#### Added

- `tests/test_working_memory_widget_callbacks.py`: 17 unit tests covering PD-03-AF-011..014.

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: Added full cut-sheet sections for PD-03-AF-011 (toggle), PD-03-AF-012 (delete), PD-03-AF-013 (promote), PD-03-AF-014 (add-fact form) in the Working Memory sub-section of PD-03.
- `docs/ux/UX_LIFECYCLE.md`: PD-03-AF-011..014 📝 → ✅, source method refs corrected, removed from Medium Priority gaps.
- `docs/ux/00_INDEX.md`: PD-03 Working Memory row updated (4✅/0📝), totals (54✅/0❌), queue item marked complete.

### Test Changes

#### Added

- `tests/test_working_memory_widget_callbacks.py` — 17 `@pytest.mark.unit` tests across 4 classes:
  - `TestWorkingMemoryToggle` (5 tests — PD-03-AF-011):
    - GIVEN fact(enabled=True) WHEN rendered THEN Checkbutton variable is True.
    - GIVEN fact(enabled=False) WHEN rendered THEN Checkbutton variable is False.
    - GIVEN fact(enabled=True) WHEN invoked THEN on_toggle called with (key, False).
    - GIVEN fact(enabled=False) WHEN invoked THEN on_toggle called with (key, True).
    - GIVEN on_toggle=None WHEN invoked THEN no exception raised.
  - `TestWorkingMemoryDelete` (4 tests — PD-03-AF-012):
    - GIVEN AGENT fact WHEN ✕ clicked + confirmed THEN on_delete called.
    - GIVEN AGENT fact WHEN ✕ clicked + cancelled THEN on_delete NOT called.
    - GIVEN USER fact WHEN rendered THEN no ✕ button present.
    - GIVEN on_delete=None WHEN confirmed THEN no exception raised.
  - `TestWorkingMemoryPromote` (4 tests — PD-03-AF-013):
    - GIVEN AGENT fact WHEN icon clicked + confirmed THEN on_promote called.
    - GIVEN AGENT fact WHEN icon clicked + cancelled THEN on_promote NOT called.
    - GIVEN USER fact WHEN rendered THEN owner icon is Label not Button.
    - GIVEN on_promote=None WHEN confirmed THEN no exception raised.
  - `TestWorkingMemoryAddFact` (4 tests — PD-03-AF-014):
    - GIVEN key+value entered WHEN ‘Add 👤’ clicked THEN on_user_add called with (key, value).
    - GIVEN key+value entered WHEN submitted THEN both entries cleared.
    - GIVEN empty key WHEN ‘Add 👤’ clicked THEN on_user_add NOT called.
    - GIVEN on_user_add=None WHEN submitted THEN no exception raised.

---

## [0.20.2.post6] - 2026-04-30

### Code Changes

#### Added

- `tests/test_context_renderer_message_enabled.py`: 4 unit tests for PD-03-AF-007 (checkbox initial state true/false, uncheck→False, check→True).

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: PD-03 Context section expanded with full PD-03-AF-007 cut-sheet entry — behaviour table, Gherkin use-cases, test mapping.
- `docs/ux/UX_LIFECYCLE.md`: PD-03-AF-007 📝 → ✅; removed from Medium Priority gaps list.
- `docs/ux/00_INDEX.md`: PD-03 Context row updated (7✅/0📝), totals (50✅/0❌), PD-03-AF-007 marked complete.

### Test Changes

#### Added

- `tests/test_context_renderer_message_enabled.py` — 4 `@pytest.mark.unit` tests:
  - GIVEN `enabled=True` WHEN row rendered THEN Checkbutton variable is True. (`test_enabled_checkbox_initial_true` — PD-03-AF-007)
  - GIVEN `enabled=False` WHEN row rendered THEN Checkbutton variable is False. (`test_enabled_checkbox_initial_false` — PD-03-AF-007)
  - GIVEN `enabled=True` WHEN Checkbutton invoked THEN `message.enabled` is False. (`test_uncheck_sets_message_enabled_false` — PD-03-AF-007)
  - GIVEN `enabled=False` WHEN Checkbutton invoked THEN `message.enabled` is True. (`test_check_sets_message_enabled_true` — PD-03-AF-007)

---

## [0.20.2.post5] - 2026-04-30

### Code Changes

#### Added

- `tests/test_input_panel_attachment_chips.py`: 9 unit tests covering PD-02-AF-005..007 (chip render with filename/icon, toggle callback, rebuild clears chips).

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: PD-02 (InputPanel) fully backfilled to cut-sheet standard — placement diagram, internal structure, behaviour inventory (7 affordances AF-001..007), Gherkin use-cases, test mapping table, code references. Corrected inaccurate chip description (was `[×]` remove button, now reflects actual toggle-checkbutton implementation).
- `docs/ux/UX_LIFECYCLE.md`: PD-02-AF-005..007 rows updated from 3 📝 (referencing non-existent methods) to 3 ✅ (referencing actual `_create_attachment_widget` / `update_attachment_bar`); High Priority gaps section cleared.
- `docs/ux/00_INDEX.md`: PD-02 row updated (3✅/3⚠️/1📝/0❌), totals updated (49✅/0❌), PD-02-AF-005..007 marked complete in Priority Work Queue.

### Test Changes

#### Added

- `tests/test_input_panel_attachment_chips.py` — 9 `@pytest.mark.unit` tests:
  - GIVEN `display_name="parser.py"` and `is_from_history=False` WHEN `update_attachment_bar([info], [])` THEN chip label contains `"parser.py"`. (`test_current_attachment_chip_shows_filename` — PD-02-AF-005)
  - GIVEN current-turn chip WHEN rendered THEN Checkbutton text starts with `"\ud83d\udcc1"`. (`test_current_attachment_chip_uses_folder_icon` — PD-02-AF-005)
  - GIVEN `is_from_history=True` WHEN rendered THEN text contains `"old_file.txt"` and `"history"`. (`test_history_attachment_chip_shows_filename_and_history_suffix` — PD-02-AF-005)
  - GIVEN history chip WHEN rendered THEN text starts with `"\ud83d\udcdc"`. (`test_history_attachment_chip_uses_scroll_icon` — PD-02-AF-005)
  - GIVEN two infos WHEN `update_attachment_bar([a, b], [])` THEN two chip frames. (`test_multiple_chips_rendered_in_order` — PD-02-AF-005)
  - GIVEN `enabled=True`, `attachment_id="att-x"` WHEN checkbox invoked THEN `on_toggle("att-x", False)`. (`test_uncheck_calls_on_attachment_toggle_false` — PD-02-AF-006)
  - GIVEN `enabled=False`, `attachment_id="att-y"` WHEN checkbox invoked THEN `on_toggle("att-y", True)`. (`test_check_after_uncheck_calls_toggle_true` — PD-02-AF-006)
  - GIVEN 1 chip rendered WHEN `update_attachment_bar([], [])` THEN `attachment_labels` empty. (`test_empty_update_clears_all_chips` — PD-02-AF-007)
  - GIVEN `"old.py"` chip WHEN rebuilt with `"new.py"` THEN only `"new.py"` chip present. (`test_rebuild_replaces_existing_chips` — PD-02-AF-007)

---

## [0.20.2.post4] - 2026-04-29

### Code Changes

#### Added

- `tests/test_resynthesis_dialog.py`: 7 unit tests covering all 5 PD-06 affordances (title, cancel, confirm with/without hint, WM section visibility, Add WM hint callback).

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: PD-06 (ResynthesisDialog) fully backfilled to cut-sheet standard — placement diagram, internal structure diagram, behaviour inventory (5 affordances: AF-001..005), Gherkin use-cases, test mapping, code references.
- `docs/ux/UX_LIFECYCLE.md`: PD-06 matrix expanded from 3 rows (❌) to 5 rows (✅); PD-06-AF-004..005 added for WM hint section and callback; removed from §7 Known Coverage Gaps.
- `docs/ux/00_INDEX.md`: PD-06 status row updated (5✅/0❌), totals updated (46✅/0❌), PD-06 removed from Priority Work Queue.

### Test Changes

#### Added

- `tests/test_resynthesis_dialog.py` — 7 `@pytest.mark.unit` tests for ResynthesisDialog:
  - GIVEN `task_id="step-42"` WHEN dialog created THEN title is `"Re-synthesise — step-42"`. (`test_title_includes_task_id` — PD-06-AF-001)
  - GIVEN mock `on_confirm` WHEN Cancel invoked THEN `on_confirm` not called AND window destroyed. (`test_cancel_destroys_dialog_without_confirm` — PD-06-AF-002)
  - GIVEN hint text `"focus on error handling"` WHEN Re-synthesise invoked THEN `on_confirm("focus on error handling")` called AND window destroyed. (`test_confirm_calls_on_confirm_with_hint` — PD-06-AF-003)
  - GIVEN empty hint WHEN Re-synthesise invoked THEN `on_confirm("")` called. (`test_confirm_with_empty_hint_passes_empty_string` — PD-06-AF-003)
  - GIVEN `on_add_wm_hint=None` WHEN dialog created THEN no "Add WM hint" button present. (`test_wm_section_hidden_without_callback` — PD-06-AF-004)
  - GIVEN `on_add_wm_hint` provided WHEN dialog created THEN "Add WM hint" button is present. (`test_wm_section_visible_with_callback` — PD-06-AF-004)
  - GIVEN key="style" value="concise" WHEN Add WM hint invoked THEN callback called, fields cleared, dialog open. (`test_add_wm_hint_calls_callback_and_clears_fields` — PD-06-AF-005)

---

## [0.20.2.post3] - 2026-04-29

### Code Changes

#### Added

- `tests/test_chat_panel_collapse_defaults.py`: 3 unit tests locking down PD-01-AF-005..007 (thinking collapsed, tool call collapsed, assistant response expanded on initial render).

#### Changed

- `docs/ux/UX_LIFECYCLE.md`: PD-01-AF-005..007 matrix rows updated 📝 → ✅ with test file and test function references; rows removed from §7 Known Coverage Gaps.
- `docs/ux/00_INDEX.md`: PD-01 status row updated (7✅/1⚠️/0📝), totals updated (41✅/15📝), PD-01-AF-005..007 removed from Priority Work Queue.

### Test Changes

#### Added

- `tests/test_chat_panel_collapse_defaults.py` — 3 `@pytest.mark.unit` tests for ChatPanel entry collapse defaults.
  - GIVEN a turn started WHEN `display_agent_thinking()` called THEN thinking entry has `expanded=False` and `detail_text` not visible. (`test_thinking_entry_collapsed_by_default` — PD-01-AF-005)
  - GIVEN a turn started WHEN a `[🔧 Calling tool` line received via `display_agent_response()` THEN tool_call entry has `expanded=False` and `detail_text` not visible. (`test_tool_call_entry_collapsed_by_default` — PD-01-AF-006)
  - GIVEN a turn started WHEN `display_agent_response()` streams content THEN assistant entry has `expanded=True` and `detail_text` is visible. (`test_assistant_response_entry_expanded_by_default` — PD-01-AF-007)

---

## [0.20.2.post2] - 2026-04-29

### Code Changes

#### Added

- `docs/ux/04_COMPONENT_CUT_SHEET_TEMPLATE.md`: reusable component cut-sheet standard with sections for placement diagram, internal structure diagram, behaviour inventory table, Gherkin use-cases, test mapping table, and code/configuration references.

#### Changed

- `docs/ux/03_PANEL_DETAILS.md`: PD-09 (CollapsibleSection) backfilled to full cut-sheet exemplar — placement diagram, internal structure diagram, behaviour inventory (4 affordances), Gherkin scenarios, test mapping, code references.
- `docs/ux/UX_LIFECYCLE.md`: PD-09 traceability matrix rows all updated from 📝 → ✅ with concrete test file and test name references; PD-09-AF-001..004 removed from §7 Known Coverage Gaps.
- `docs/ux/00_INDEX.md`: status snapshot updated (38✅ total), requirement intake 5-step process added, cut-sheet template linked, PD-09 removed from Priority Work Queue.
- `docs/ux/README.md`: `04_COMPONENT_CUT_SHEET_TEMPLATE.md` added to contents table.

### Test Changes

#### Added

- `tests/test_collapsible_section.py`: 4 hermetic unit tests locking down all PD-09 affordances.
  - GIVEN a CollapsibleSection with `initial_collapsed=True` WHEN created THEN `is_expanded()` is False and content_container has no geometry manager. (`test_initial_collapsed_state_hides_content_container` — PD-09-AF-001)
  - GIVEN a CollapsibleSection with `initial_collapsed=False` WHEN created THEN `is_expanded()` is True and content_container is managed by pack. (`test_initial_expanded_state_shows_content_container` — PD-09-AF-002)
  - GIVEN a CollapsibleSection WHEN `toggle()` is called THEN expanded state flips and content_container visibility toggles accordingly. (`test_toggle_flips_state_and_visibility` — PD-09-AF-003)
  - GIVEN a CollapsibleSection with existing content WHEN `set_content()` is called with a new widget THEN previous widget is destroyed and only the new widget remains. (`test_set_content_replaces_previous_widget` — PD-09-AF-004)

---

## [0.20.2.post1] - 2026-04-29

### Documentation Changes

#### Added

- `docs/ux/00_INDEX.md` — session entry point for UX work; contains the Status
  Snapshot table, Priority Work Queue, and agile process flow diagram.  Both
  the developer and the agent open this file at the start of every UX session.
- `.github/prompts/ux-review.prompt.md` — `/ux-review` slash-command prompt
  implementing an 8-phase TDD review loop: Baseline → Specify → Cut-Sheet
  Verify → Gherkin Verify → Test-First Update → Iterative Code Fix →
  Reconcile → Commit Gate.
- `docs/ux/UX_LIFECYCLE.md` — single-source traceability hub; affordance ID
  scheme, complete traceability matrix for all 11 panels, change checklists,
  headless Tkinter testing primer, gap inventory, and audit commands.
  (File was created at end of prior session; captured in this post-release.)

#### Changed

- `docs/ux/README.md` — `00_INDEX.md` added as the top entry; `UX_LIFECYCLE.md`
  listed as lifecycle rules document.
- `.github/copilot-instructions.md` — UX section updated to require agent opens
  `00_INDEX.md` first (Status Snapshot + Priority Work Queue) before any
  `src/agentx/gui/` change.

### Code Changes

#### Fixed

- `src/agentx/gui/context_renderer._render_message_to_grid()` — regression where
  plain messages (no tools, attachments, or plans) received `is_expandable=False`,
  rendering an empty placeholder `Label` in the expand column instead of a toggle
  `Button`.  This made the full message content inaccessible in the Context panel.
  Fix: removed the `is_expandable` conditional entirely.  Every message now always
  gets an expand/collapse `Button` (col 0) and a hidden full-content `tk.Label`
  detail row that is revealed on toggle.

### Test Changes

#### Added

- `tests/test_phase6_context_panel.py` — new `TestRenderMessageAlwaysExpandable`
  class (7 unit tests):
  - `test_plain_user_message_has_expand_button` — GIVEN a plain user message WHEN
    rendered THEN a Button is placed in the exp_button column.
  - `test_plain_assistant_message_has_expand_button` — same for assistant role.
  - `test_plain_system_message_has_expand_button` — same for system role.
  - `test_full_content_row_created_and_hidden_by_default` — GIVEN a 200-char message
    WHEN rendered THEN a hidden full-content Label exists.
  - `test_full_content_row_becomes_visible_on_toggle` — GIVEN a plain message WHEN
    expand button is clicked THEN the full-content label becomes visible.
  - `test_message_with_tool_still_has_expand_button` — GIVEN a message with a tool
    call WHEN rendered THEN the expand button is still present.
  - `test_empty_content_message_has_expand_button_no_detail_row` — GIVEN an empty
    message WHEN rendered THEN expand button exists but no hidden detail row is added.

#### Fixed

- `tests/test_phase6_context_panel.py` — pre-existing SIGABRT race: background
  `ModelMetadataStore.populate` threads (spawned by `AgentXSession.__init__`) made
  HTTP calls during test teardown, sometimes hitting a destroyed socket/GC and
  aborting the entire process.  Fixed by adding
  `patch("agentx.model_metadata_store.ModelMetadataStore.populate")` to `_make_session`.

---

## [0.20.1] - 2026-04-28

### Code Changes

#### Fixed

- `src/agentx/gui/gui_manager.py` — `update_context_meter()` had regressed to the no-op
  stub after the editor saved a stale in-memory buffer over the committed fix. Re-applied
  the real delegation: `self._input_panel.context_meter.update(max_tokens, breakdown)`.
  This was the root cause of the context visualiser remaining at 0% throughout a session.
- `src/agentx/model_metadata_store.get_context_length()` — added `:latest` tag fallback so
  bare model names (e.g. `gpt-oss`) resolve to their tagged equivalent (`gpt-oss:latest`).
  Ollama always appends `:latest` implicitly, causing spurious *"missing from metadata store"*
  warnings and returning `FALLBACK_CONTEXT_WINDOW` instead of the real context length.
- `src/agentx/model_metadata_store.get_metadata()` — applied the same `:latest` tag
  fallback for consistency with `get_context_length()`.

### Test Changes

#### Added

- `tests/test_model_metadata_store.py` — 6 new unit tests:
  - `test_get_context_length_latest_tag_fallback` (×4 parametrized):
    - GIVEN model stored as `gpt-oss:latest` / WHEN looked up as `gpt-oss` / THEN returns real capacity
    - GIVEN model stored as `llama3.2:latest` / WHEN looked up as `llama3.2` / THEN returns real capacity
    - GIVEN exact tagged lookup `gpt-oss:latest` / WHEN looked up directly / THEN returns real capacity
    - GIVEN completely unknown model / WHEN looked up / THEN returns FALLBACK_CONTEXT_WINDOW
  - `test_get_metadata_latest_tag_fallback` (×2 parametrized):
    - GIVEN model stored as `gpt-oss:latest` / WHEN metadata requested as `gpt-oss` / THEN returns correct dict
    - GIVEN model stored as `llama3.2:latest` / WHEN metadata requested as `llama3.2` / THEN returns correct dict

---

## [0.20.0] - 2026-04-28

### Code Changes

#### Added

- `src/agentx/gui/context_meter_widget.py` — `ContextMeterWidget` (ARCH-04): donut chart Tkinter widget showing LLM context window utilisation by category. Features: seven band arcs (BAND-01–07), ghost arc for remaining capacity (ENH-02), border ring with three risk states (default / warning ≥80% / critical ≥100%, ENH-16), center percentage label with matching risk color, hover tooltips via `tk.Toplevel` (ENH-06), thread-safe `update()` via `canvas.after(0, ...)` (ARCH-06).
- `src/agentx/widget_registry.py` — added `context_meter_canvas: Optional[tk.Canvas]` field and corresponding `destroy_all()` teardown.

#### Changed

- `src/agentx/gui/input_panel.py` — imports and instantiates `ContextMeterWidget`, calls `create()` inside `InputPanel.create()`, and registers `context_meter_canvas` in the widget registry.
- `src/agentx/gui/gui_manager.py` — replaced `update_context_meter()` stub with real delegation to `self._input_panel.context_meter.update(max_tokens, breakdown)`.

### Test Changes

#### Added

- `tests/test_context_meter_widget.py` — 39 hermetic unit tests covering:
  - GIVEN new widget / WHEN `create()` called / THEN canvas placed at correct geometry
  - GIVEN `update()` on background thread / WHEN called / THEN `canvas.after(0, ...)` invoked (thread safety)
  - GIVEN each of seven band categories / WHEN `_render()` / THEN PIESLICE arc drawn with correct fill color (parameterized × 7)
  - GIVEN 50% usage / WHEN `_render()` / THEN ghost arc drawn with `_GHOST_COLOR`
  - GIVEN 100% usage / WHEN `_render()` / THEN no ghost arc drawn
  - GIVEN empty breakdown / WHEN `_render()` / THEN only ghost arc fills ring
  - GIVEN usage at each risk threshold / WHEN `_render()` / THEN border ring has correct color and width (parameterized × 8: 0%, 50%, 79%, 80%, 95%, 99%, 100%, 120%)
  - GIVEN usage at each risk threshold / WHEN `_render()` / THEN center label has correct fill color (parameterized × 4)
  - GIVEN 40% usage / WHEN `_render()` / THEN center label text is `"40%"`
  - GIVEN absurdly large token count / WHEN `_render()` / THEN center label capped at `"999%"`
  - GIVEN `max_tokens=0` / WHEN `_render()` / THEN no crash
  - GIVEN canvas not yet laid out / WHEN `_render()` / THEN retries via `canvas.after(50, ...)`
  - GIVEN each band / WHEN tooltip hover / THEN tooltip text contains band label and percentage (parameterized × 5)
  - GIVEN 50% usage / WHEN ghost arc hover / THEN tooltip shows "Remaining capacity" and token count
  - GIVEN destroyed Toplevel / WHEN `_hide_tooltip()` / THEN TclError silently swallowed
  - GIVEN no active tooltip / WHEN `_hide_tooltip()` / THEN no exception

## [0.19.3] - 2026-04-27

### Code Changes

#### Changed

- `src/agentix/models.py` now returns supplied cached `max_tokens` before any live model discovery so cached context-length lookups remain usable even when Ollama tag enumeration is unavailable.
- `src/agentix/bridge/tool_loop.py`, `src/agentix/bridge/bridge.py`, `src/agentx/integration/agentix_bridge_adapter.py`, and `src/agentx/session.py` now explicitly invalidate the bridge max-token cache when the active model changes, keeping prompt trimming aligned with the selected model.

#### Fixed

- Fixed the review regression where `get_model(..., max_tokens=...)` still hit `/api/tags` before honoring the cached value.
- Fixed stale tool-loop max-token caching after model switches, which could leave Agentix trimming against the previous model's context window.

### Test Changes

#### Added

- Hermetic regression tests proving cached `max_tokens` bypasses live model discovery and proving model changes invalidate the bridge/tool-loop max-token cache.

#### Fixed

- Targeted regression coverage for the corrected model-selection path remains at 98% for `agentix.models` with new cache-invalidation behaviors covered by hermetic unit tests.

## [0.19.2] - 2026-04-27

### Code Changes

#### Added

- `src/shared/providers/` introducing a shared provider boundary so `agentix` and `agentx` can both consume the same `ILLMServiceProvider`, constants, and Ollama adapter without reverse imports.
- `tests/test_agentix_models.py` and `tests/test_tool_loop_max_tokens.py` covering model-selection failures, cached max-token reuse, and tool-loop max-token wiring.

#### Changed

- `src/agentix/models.py` now hardens Ollama model enumeration, rejects malformed payloads, raises a clear error when no models match, and uses cached `max_tokens` when supplied.
- `src/agentix/bridge/tool_loop.py`, `src/agentix/agentix_config.py`, and `src/agentx/session.py` now propagate cached context-length values into the tool loop so Agentix avoids redundant live lookups when AgentX already knows model capacity.
- `src/agentx/protocols.py`, `src/agentx/session.py`, and `src/agentx/streaming_controller.py` now expose public context-meter protocol methods while keeping compatibility wrappers for existing call sites.
- `src/agentx/model_metadata_store.py` now exposes `population_failed` alongside `populated` so callers can distinguish completion from successful population.
- `src/agentx/providers/*` now act as compatibility wrappers over the shared provider implementation.

#### Fixed

- Removed the reverse dependency from `agentix` into `agentx.providers`, eliminating the reviewed layering violation.
- Fixed unhandled request/JSON failures and malformed response handling in Ollama model discovery.
- Fixed weak parameter-size validation and empty-model handling in Agentix model selection.
- Fixed the bridge max-token path so cached context lengths are actually consumed by the tool loop.

### Test Changes

#### Added

- Hermetic unit coverage for malformed Ollama payloads, fallback provider paths, model metadata cache failure semantics, public/compatibility meter APIs, and cached max-token routing into the tool loop.

#### Fixed

- Targeted hermetic coverage for the repaired core modules now reaches 98% (`agentix.models`, `agentx.model_metadata_store`, `shared.providers.ollama_provider`).

## [0.19.1] - 2026-04-27

### Code Changes

#### Added

- `src/agentx/providers/constants.py` introducing provider-scoped constants (`OLLAMA_MODELS_ENDPOINT`, `OLLAMA_SHOW_ENDPOINT`, `FALLBACK_CONTEXT_WINDOW`) to remove cross-tree imports.
- `src/agentx/protocols.py` introducing runtime-checkable `IMeterSession` for explicit context-meter contracts.
- `src/shared/token_utils.py` with module-level `chars_per_token()` and `estimate_text_tokens()` utilities.

#### Changed

- `src/agentx/providers/base.py` adds required `provider_id` contract to `ILLMServiceProvider`.
- `src/agentx/providers/ollama_provider.py` now exposes `provider_id = "ollama"`, accepts optional host values, and normalizes `None`/empty hosts safely.
- `src/agentx/model_metadata_store.py` now uses provider `provider_id` in cache payloads, exposes `populated: threading.Event`, unifies cache parsing with `_parse_cache_data()`, and adds `invalidate(model_name: str | None = None)` background refresh support.
- `src/agentx/session.py` now imports provider constants from `agentx.providers.constants`, starts model-store population asynchronously at startup, adds `on_context_assembled(shared_context)`, and simplifies `_context_meter_payload()` error-handling and model-name fallback semantics.
- `src/agentx/streaming_controller.py` replaces `hasattr` meter guards with `isinstance(..., IMeterSession)` checks and delegates assembled-context meter redraw via `on_context_assembled()`.
- `src/shared/models/context.py` now routes `MessageRole.SYNTHESIS` into the assistant meter band and uses shared token utilities while retaining compatibility shims.
- `src/agentix/models.py` adds optional fast-path `max_tokens` argument to avoid redundant live context-length HTTP calls.
- `pyproject.toml` test config now includes `pythonpath = ["src"]` to eliminate per-test `sys.path` mutation.

#### Fixed

- Addressed all 15 PR #5 review findings (A1-A7, P1-P8), including provider abstraction, cache semantics, meter protocol boundaries, startup threading behavior, and context-meter correctness.

### Test Changes

#### Added

- `tests/test_token_utils.py` (Unit):
  - GIVEN model-name families and text samples
  - WHEN token utility helpers run
  - THEN family ratios and ceiling token estimates are validated.
- `tests/test_protocols.py` (Unit):
  - GIVEN full and partial structural implementations
  - WHEN runtime protocol checks execute
  - THEN `IMeterSession` compatibility is correctly enforced.

#### Changed

- `tests/test_llm_service_provider.py`:
  - Removed `sys.path.insert` usage and switched constants import to `agentx.providers.constants`.
  - Added `provider_id` assertion and parametrized host-normalization coverage (`None`, empty, bare host, http/https variants).
- `tests/test_model_metadata_store.py`:
  - Removed `sys.path.insert`, updated constants import, and added `provider_id` to test provider.
  - Added coverage for `populated` event behavior, failure-path event setting, `invalidate()` single/all flows, provider-id cache serialization, and `_parse_cache_data()`.
- `tests/test_context_token_breakdown.py`:
  - Removed `sys.path.insert`.
  - Added explicit `SYNTHESIS` role routing assertions into `assistant` band.
- `tests/test_active_model_meter_wiring.py`:
  - Removed `sys.path.insert`, updated constants import.
  - Added `on_context_assembled()` meter-redraw behavior coverage.

#### Fixed

- New/updated targeted tests for PR #5 review scope now pass (`50 passed, 0 failed`).

## [0.19.0] - 2026-04-26

### Code Changes

#### Added

- `src/agentx/providers/base.py` with `ILLMServiceProvider` protocol.
- `src/agentx/providers/ollama_provider.py` implementing model enumeration and context-length lookup via Ollama endpoints.
- `src/agentx/model_metadata_store.py` for startup-populated, disk-backed model capacity metadata (`sessions/_model_cache.json`).

#### Changed

- `src/agentix/constants.py` adds `OLLAMA_SHOW_ENDPOINT` and `FALLBACK_CONTEXT_WINDOW`.
- `src/agentix/models.py` now derives `max_tokens` from provider context-length lookup instead of `parameter_size` proxy.
- `src/agentx/session.py` now initializes provider/store at startup, tags working-memory system messages via metadata, adds meter payload/scheduling helpers, and triggers redraw on model change, submit, and attachment-toggle events.
- `src/agentx/streaming_controller.py` now schedules meter redraw on submit-context assembly and after stream completion.
- `src/agentx/igui_manager.py` and `src/agentx/gui/gui_manager.py` now include `update_context_meter(max_tokens, breakdown)` contract/stub.
- `src/shared/models/message.py` now includes serializable `metadata` map.
- `src/shared/models/context.py` adds `token_breakdown(model_name)` with TOK-02 model-family ratios.
- `docs/context_size_prerequisite_plan.md` updated with Phase 1 audit findings and implementation progress tracking.
- `docs/ux/context_visualizer.md` marks PRE-02 complete and updates dynamic-context definition notes.
- `docs/architecture.md` module map updated with provider abstraction and metadata-store runtime flow.

#### Fixed

- Context-meter denominator source now aligns with active-model context window semantics instead of parameter-size heuristics.

### Test Changes

#### Added

- `tests/test_llm_service_provider.py` (Unit):
  - GIVEN Ollama tag/show payloads
  - WHEN provider methods are called
  - THEN model names, key-probe ordering, and fallback behavior are validated.
- `tests/test_model_metadata_store.py` (Unit):
  - GIVEN cache/no-cache startup conditions
  - WHEN `populate()` executes
  - THEN fetch/cached/fallback behavior is validated.
- `tests/test_context_token_breakdown.py` (Unit):
  - GIVEN enabled context messages and attachments
  - WHEN `token_breakdown()` executes
  - THEN role-band and attachment token estimates are correctly routed.
- `tests/test_active_model_meter_wiring.py` (Integration):
  - GIVEN active-model changes in session
  - WHEN setter logic runs
  - THEN GUI meter updates receive denominator and breakdown, including no-op and fallback paths.

#### Changed

- No existing tests removed; new PRE-02 tests run alongside the existing suite.

#### Fixed

- N/A

#### Removed

- N/A

## [0.18.26] - 2026-04-26

### Code Changes

#### Fixed

- **F-1 (HIGH)** `src/agentx/streaming_controller.py` — replay candidate lists (`original_tool_calls`, `original_tool_results`, `original_assistants`) now scoped to the replaying `task_id` via `and msg.task_id == _tid` filter, preventing cross-turn message contamination.
- **F-1 (HIGH)** `src/agentx/streaming_controller.py` — `_display_tool_call` and `_display_tool_result` now accept `task_id: str | None = None` and stamp it on persisted messages; streaming path passes the current task_id via a mutable box (`_current_task_id`); replay path passes `_tid` explicitly.
- **F-2 (HIGH)** `src/agentx/session.py` — `_persist_stream_messages` delegate restored missing `synthesis_of: list[str] | None = None` parameter and switched to keyword-based delegation to avoid positional argument corruption (the `refresh_gui` bool was being received as `synthesis_of`).
- **F-3 (MEDIUM)** `src/shared/models/message.py` — `from_dict` coerces non-list `synthesis_of` values to `[]` instead of storing them; `__post_init__` raises `ValueError` if `synthesis_of` is not a `list`.

### Test Changes

#### Added

- **Integration** `tests/test_replay_message_lineage_e2e.py` — T-1: `test_replay_does_not_supersede_prior_turn_messages` — verifies replay of a second task leaves prior-turn messages untouched.

  ```gherkin
  GIVEN a context with a prior turn (task_id=prior) and a replay target (task_id=target)
  WHEN the replay worker runs for task_id=target
  THEN messages from the prior turn (task_id=prior) are not superseded or modified
  ```

- **Integration** `tests/test_replay_message_lineage_e2e.py` — T-2: `test_replay_does_not_supersede_concurrent_plan_step_messages` — verifies replay of Step A in a two-step plan leaves Step B messages untouched.

  ```gherkin
  GIVEN a context with two plan steps (step_a_task_id, step_b_task_id)
  WHEN the replay worker runs for step_a_task_id
  THEN messages belonging to step_b_task_id are not superseded or modified
  ```

- **Integration** `tests/test_streaming_message_id_integration.py` — T-3: `test_persist_stream_messages_synthesis_of_is_list_type` — verifies the assistant message created by `_persist_stream_messages` has `isinstance(synthesis_of, list) == True`.

  ```gherkin
  GIVEN a StreamingController with a real Context
  WHEN _persist_stream_messages is called with synthesis_of=["msg_abc..."]
  THEN the persisted ASSISTANT message has synthesis_of as a list type
  ```

- **Integration** `tests/test_streaming_message_id_integration.py` — T-4: `test_session_delegate_forwards_synthesis_of_by_keyword` — verifies `session._persist_stream_messages` forwards `synthesis_of` and `refresh_gui` as keyword arguments to the controller.

  ```gherkin
  GIVEN a partially-initialized AgentXSession with a spy StreamingController
  WHEN session._persist_stream_messages(thinking, content, synthesis_of=[...], refresh_gui=False) is called
  THEN the spy records synthesis_of=[...] and refresh_gui=False (not positionally swapped)
  ```

- **Unit** `tests/test_message_model_ids.py` — T-5 (parametrized × 5): `test_from_dict_coerces_non_list_synthesis_of_to_empty_list` — verifies `Message.from_dict` coerces `True`, `False`, a bare string, `42`, and a dict to `[]`.

  ```gherkin
  GIVEN a message payload with synthesis_of=<non-list value>
  WHEN Message.from_dict(payload)
  THEN message.synthesis_of == [] and isinstance(message.synthesis_of, list) is True
  Permutations: True, False, "msg_a1b2c3...", 42, {"key": "value"}
  ```

- **Unit** `tests/test_message_model_ids.py` — `test_post_init_rejects_non_list_synthesis_of` — verifies `Message(..., synthesis_of=True)` raises `ValueError` mentioning `synthesis_of`.

  ```gherkin
  GIVEN synthesis_of=True (a boolean, not a list)
  WHEN Message(role=..., content=..., synthesis_of=True)
  THEN ValueError is raised with a message about synthesis_of type
  ```

#### Changed

- `tests/test_replay_message_lineage_e2e.py` — `_make_original_group` now stamps `task_id="task-1"` on all original messages so the scoped replay filter matches them correctly.

---

## [0.18.25] - 2026-04-25

### Code Changes

#### Changed

- `src/agentx/streaming_controller.py` — replay worker now persists lineage for replayed messages: replay `TOOL_CALL`/`TOOL_RESULT` messages set `cloned_from`, replay synthesis `ASSISTANT` sets `cloned_from` and `synthesis_of`, and supersession is applied only after replay outputs are fully persisted.
- `src/agentx/streaming_controller.py` — replay completion now applies `Context.supersede_message(...)` mappings so originals are disabled and point to replacements via `superseded_by`.

### Test Changes

#### Added

- **Integration/Functional** `tests/test_replay_message_lineage_e2e.py`: 20 parametrized tests covering replay lineage and supersession behavior end-to-end.

  ```gherkin
  GIVEN a replayed message group
  WHEN replay succeeds
  THEN replacements set cloned_from and originals set superseded_by
  ```

  ```gherkin
  GIVEN a replay attempt fails at any persistence stage
  WHEN replay exits with error
  THEN originals remain enabled and no superseded_by links are applied
  ```

  ```gherkin
  GIVEN replay emits tool results
  WHEN replay synthesis is persisted
  THEN synthesis_of references replay TOOL_RESULT message_ids only
  ```

  ```gherkin
  GIVEN replay replacement generation is persisted
  WHEN supersession completes
  THEN originals are disabled and replacements are enabled
  ```

  ```gherkin
  GIVEN multi-generation replay lineage
  WHEN get_ancestry is requested for the latest generation
  THEN ancestry is root-to-leaf and complete
  ```

  ```gherkin
  GIVEN replay supersession updates under interleaving patterns
  WHEN mappings are applied
  THEN supersession targets remain deterministic and lineage-safe
  ```

## [0.18.24] - 2026-04-20

### Code Changes

#### Added

- `tests/test_chat_panel_turn_rendering.py`: 10 new integration tests verifying correct pack-order of conversation-turn widgets in `ChatPanel`.  Tests cover: first-render order, full turn sequence (user → classify → think → respond), collapse/expand cycle, and multiple consecutive turns (parametrized: 1, 2, 3 turns).

#### Fixed

- `src/agentx/gui/chat_panel.py` — `_ensure_turn_started()`: moved `children.pack(fill=tk.X, …)` to *after* `_create_output_entry()` so Tkinter packs the user-entry frame before the children frame.  Previously `children` was packed first (index 0 in the slave list), causing all classification/thinking/tool/assistant entries to render *above* the user prompt on first render.  Collapse → expand accidentally "fixed" this because `pack_forget()` + `pack()` appended children to the end of the list.
- `src/agentx/gui/markdown_renderer.py` — `markdown_to_html()`: added guard for `_md_lib is None` so the function produces valid HTML (`<pre>` fallback) even when the `markdown` package is not installed.  Previously any call with `MARKDOWN_AVAILABLE=True` but missing library raised `AttributeError`.
- `tests/test_markdown_rendering.py` — `test_full_path_with_mocked_html_frame`: patched `agentx.gui.chat_panel.markdown_to_html` with a stub that returns `<table>` HTML so the test is self-contained regardless of whether the `markdown` package is installed.
- `tests/test_gui_manager_integration.py` — `test_header_preview_not_driven_by_newline`: removed incorrect assertion that header preview text varies by pixel width; the `_header_preview()` method truncates by word count (>15 words), not pixel width.
- `tests/integration/test_bootstrap_e2e.py` — `TestBootstrapDefaults`: fixed 6 methods that referenced undefined bare variables (`agentx`, `cm`, `candidates`, `prompt_path`, `instructions_path`); replaced with correct `toml_config` fixture access.

#### Changed

- `pyproject.toml`: added pytest markers `unit`, `functional`, `integration` to the `[tool.pytest.ini_options]` markers list.

### Test Changes

#### Added

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_user_entry_packed_before_children_frame_on_first_render`

  ```gherkin
  GIVEN a GUIManager with a chat panel
  WHEN a user message is sent (first render)
  THEN the user entry frame appears before the children frame in Tkinter's pack slave list
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_children_frame_packed_after_user_entry`

  ```gherkin
  GIVEN a conversation turn has been started
  WHEN inspecting pack order of turn_frame children
  THEN user entry pack-index < children frame pack-index
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_classify_entry_appended_to_children_not_turn_frame`

  ```gherkin
  GIVEN a user message has been displayed
  WHEN display_classify is called
  THEN the classification entry is a child of the children_frame, not the turn_frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_assistant_entry_appended_to_children_not_turn_frame`

  ```gherkin
  GIVEN a user message has been displayed
  WHEN display_agent_response is called
  THEN the assistant entry is parented to the children_frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_children_frame_is_indented`

  ```gherkin
  GIVEN a conversation turn
  WHEN inspecting the children_frame's pack configuration
  THEN padx has a non-zero left indent to visually nest responses under the user message
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_full_turn_sequence_pack_order`

  ```gherkin
  GIVEN a GUIManager with a fully laid-out chat panel
  WHEN a user message, classification, thinking, and agent response are all displayed
  THEN user_entry_frame is the first child of turn_frame, children_frame is the second
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_expand_after_collapse_preserves_correct_order`

  ```gherkin
  GIVEN a conversation turn has been rendered correctly
  WHEN the user entry is collapsed then expanded
  THEN the children_frame remains below the user_entry_frame in pack order
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestConversationTurnRenderingOrder::test_new_turn_starts_fresh_frame`

  ```gherkin
  GIVEN a first conversation turn is complete
  WHEN a second user message is displayed
  THEN a new turn_frame is created and the children_frame is correct within that new frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestMultipleTurnsRenderingOrder::test_multiple_turns_correct_order[1 turn]`

  ```gherkin
  GIVEN 1 user message and agent response
  WHEN rendered
  THEN user entry is before children frame in the turn's pack slave list
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestMultipleTurnsRenderingOrder::test_multiple_turns_correct_order[2 turns]`

  ```gherkin
  GIVEN 2 consecutive user messages each with an agent response
  WHEN rendered
  THEN every turn has user entry before children frame
  ```

- **Integration** `test_chat_panel_turn_rendering.py::TestMultipleTurnsRenderingOrder::test_multiple_turns_correct_order[3 turns]`

  ```gherkin
  GIVEN 3 consecutive user messages each with an agent response
  WHEN rendered
  THEN every turn has user entry before children frame
  ```

#### Fixed

- **Integration** `test_markdown_rendering.py::TestMarkdownRenderingHeadless::test_full_path_with_mocked_html_frame`

  ```gherkin
  BEFORE: AttributeError on _md_lib.markdown when markdown package is not installed
  AFTER:  markdown_to_html is patched so the test asserts <table> regardless of package availability
  ```

- **Integration** `test_gui_manager_integration.py::TestGUIManagerDisplay::test_header_preview_not_driven_by_newline`

  ```gherkin
  BEFORE: AssertionError — test incorrectly assumed header text varies by pixel width
  AFTER:  Test only asserts that newlines are condensed to spaces; truncation threshold is word-count-based
  ```

- **Integration** `tests/integration/test_bootstrap_e2e.py::TestBootstrapDefaults` (6 methods)

  ```gherkin
  BEFORE: NameError: name 'agentx' / 'cm' / 'candidates' / 'prompt_path' / 'instructions_path' is not defined
  AFTER:  All references replaced with correct toml_config fixture access
  ```

---

## [0.18.23] - 2026-04-19

### Code Changes

#### Fixed

- `pyproject.toml`: version tag format changed from PEP 440-incompatible `-letter` suffix (e.g. `0.18.22-i`) to PEP 440-compliant `.postN` post-release form (e.g. `0.18.22.post9`). The old format caused `uv build` to fail with a TOML parse error.
- `.github/copilot-instructions.md`: Semantic Versioning section updated — doc-only version examples changed from `2.3.1-a` / `2.3.1-b` to `2.3.1.post1` / `2.3.1.post2`. All references to the `-letter` alpha tag scheme replaced with `.postN` language.

---

## [0.18.22.post9] - 2026-04-19

### Code Changes

#### Added

- None

#### Changed

- None

#### Fixed

- None

### Documentation Changes

#### Added

- `docs/architecture.md`: new **§12 Context Construction Pipeline** section documenting all five filter layers applied before any message reaches the LLM API:
  - **Layer 0** — `_build_shared_context()` assembly order (WM injection, history, current session)
  - **Layer 1** — `Message.enabled` flag mechanics, including non-obvious `load_from_dir()` default of `False`, the `MessageEntry.__getattr__` proxy pattern, and per-role enabled defaults
  - **Layer 2** — `to_llm_messages()` internal-role exclusion set (`PLAN`, `TASK_NODE`, `SYNTHESIS`, `ASSERTION`) with per-role LLM-sent table
  - **Layer 3** — `to_llm_dict()` attachment filtering (`attachment.enabled == True`)
  - Full pipeline ASCII diagram showing the complete flow from `_build_shared_context` through wire-format output
  - Classification path note showing `classify_prompt()`'s independent `enabled`-only filter
- `docs/architecture.md`: Contents table updated with §12 entry; old §12–16 renumbered to §13–17

---

## [0.18.22.post8] - 2026-04-19

### Code Changes

#### Added

- None

#### Changed

- None

#### Fixed

- None

### Documentation Changes

#### Added

- `docs/ux/03_PANEL_DETAILS.md`: new **PD-11: FileExplorer** section documenting the Files tab widget — navigation bar (Back, Forward, Up, Home, Refresh), path label, three-column treeview, file/folder context menus (Attach, Edit, Add to memory), navigation history state, and related user flow references.
- `docs/ux/03_PANEL_DETAILS.md`: fully expanded **PD-07: SettingsTab** section — replaces the thin key-sections table with complete per-section widget inventories for all five collapsible groups (Appearance, Ollama, Agentix, Classification Display, Working Memory), ASCII mockup, and widget convention table.
- `docs/ux/02_USER_FLOWS.md`: **UF-11: File Explorer Navigation** sequence diagram covering directory navigation, file attach, file edit, folder pin to Working Memory, and history traversal (Back/Forward).
- `docs/ux/01_MAIN_LAYOUT.md`: Detail Diagram References table — added rows linking **Files Tab → PD-11** and **Settings Tab → PD-07**.
- `docs/ux/01_MAIN_LAYOUT.md`: Component Index — added `[→ PD-11]` and `[→ PD-07]` inline links on the Files tab and Settings tab rows.
- `docs/ux/03_PANEL_DETAILS.md`: PD-03 SidePanel — replaced the inline Files Tab and Settings Tab affordance tables with concise `[→ PD-11]` / `[→ PD-07]` forward-references to avoid duplication.

---

## [0.18.22.post7] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation / workspace config only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Added

- `.vscode/extensions.json` — workspace extension recommendations. Marks `vstirbu.vscode-mermaid-preview` and `mermaidchart.vscode-mermaid-chart` as `unwantedRecommendations` because both register Markdown preview renderers that conflict with `bierner.markdown-mermaid`, producing the double-nested "No diagram type detected" error in the Markdown preview. Only `bierner.markdown-mermaid` should be active in this workspace for Mermaid rendering.

---

## [0.18.22.post6] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10: rewrote self-referential arrow labels to remove leading underscores (`_interrupt_flag`, `_is_streaming`). Mermaid processes label text as Markdown, so `_word_` patterns are interpreted as italic markers, corrupting the token stream and causing "No diagram type detected". Replaced with plain descriptive text: `set interrupt flag = True` and `clear is_streaming event`. Also simplified the Note and partial-response label text.

---

## [0.18.22.post5] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10: replaced em dash `—` with a plain hyphen `-` in the `Note` text; the Unicode em dash caused "No diagram type detected" in the VS Code Mermaid preview renderer.

---

## [0.18.22.post4] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10 Mermaid parse error: added missing `participant Chat as ChatPanel` declaration and replaced `\n`-delimited `Note` text with a single-line string (inline `\n` escapes in `Note` blocks are not supported by all Mermaid versions and caused a NEWLINE parse error).

---

## [0.18.22.post3] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — replaced `actor User` with `participant User` in all 9 Mermaid sequence diagrams (UF-01 through UF-10 excluding UF-09). The `actor` keyword is not supported by the Mermaid version bundled with VS Code's markdown preview, causing "No diagram type detected" errors.

---

## [0.18.22.post2] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Window Layout Mockup**: ChatPanel is now shown on the **left (~66%)** and SidePanel on the **right (~34%)**, matching the actual `PanedWindow` widget order in `chat_panel.py` and `side_panel.py`.
- `docs/ux/01_MAIN_LAYOUT.md` — removed incorrect model-selector widget from the OS title bar in the mockup (it lives inside SidePanel, not the title bar).
- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Zone Map** sash table: left pane is ChatPanel (~66%), right pane is SidePanel (~34%); was previously inverted (25%/75%).
- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Component Index** `Left pane` / `Right pane` labels and percentages.
- `docs/ux/01_MAIN_LAYOUT.md` — corrected §4 SidePanel and §5 ChatPanel position strings to match actual sash proportions.
- `docs/ux/01_MAIN_LAYOUT.md` — added `screen_side` clarification note: this setting controls which side of the **monitor** the window is placed on, not the internal panel arrangement.
- `docs/ux/01_MAIN_LAYOUT.md` — added **Detail Diagram References** table beneath the mockup, providing `[→ PD-XX]` annotations on every generalised component and linking to corresponding sections in `03_PANEL_DETAILS.md` (quality gate: generalised components must carry detail diagram references).
- `docs/ux/03_PANEL_DETAILS.md` — corrected **PD-01 ChatPanel** position from "Right ~75%" to "Left ~66% (PanedWindow left pane)".
- `docs/ux/03_PANEL_DETAILS.md` — corrected **PD-03 SidePanel** position from "Left ~25%" to "Right ~34% (PanedWindow right pane)".

---

## [0.18.22.post1] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Added

- `docs/ux/README.md` — new UX documentation index with message-role icon legend and key UX principles.
- `docs/ux/01_MAIN_LAYOUT.md` — main window layout mockup with zone map, component index, and per-panel layout details (SidePanel, ChatPanel, InputPanel).
- `docs/ux/02_USER_FLOWS.md` — Mermaid sequence and flowchart diagrams for all 10 major user flows: basic chat, tool execution, hierarchical task execution, re-synthesis, file attachment, model switch, settings change, session history navigation, working memory management, and interrupt streaming.
- `docs/ux/03_PANEL_DETAILS.md` — per-panel affordance specifications for ChatPanel, InputPanel, SidePanel, ModelSelector, PlanTreeWidget, ResynthesisDialog, SettingsTab, ContextRenderer, CollapsibleSection, and ToolPanel.

#### Changed

- `docs/architecture.md` — full rewrite to reflect current codebase state (2026-04-19). Updated module map tables, class relationship Mermaid diagram, session decomposition (SessionState/StreamingController/ToolDispatcher), GUI decomposition (ChatPanel/InputPanel/SidePanel/ContextRenderer), classification pipeline (with phi4-mini, response_format fix, system-msg exclusion fix), tool pipeline, hierarchical task execution, working memory section, data model schemas, tool schema examples, threading model, persistence layout, configuration reference, and retrieval keywords index.
- `gui_manager.md` — replaced aspirational design doc with current-state documentation of GUIManager as a thin coordinator, describing the IGUIManager Protocol, panel decomposition, WidgetRegistry pattern, and achieved separation-of-concerns table.
- `docs/integration/01_ARCHITECTURE_OVERVIEW.md` — updated all stale module file paths (e.g. `src/agentx/gui_manager.py` → `src/agentx/gui/gui_manager.py`; `src/agentx/context.py` → `src/shared/models/context.py`). Updated component tables for both AgentX and Agentix subsystems to match current code structure. Updated data flow diagrams.

---

## [0.18.22] - 2026-04-18

### Code Changes

#### Fixed

- `src/agentix/query_payload.py` — emit `response_format: {"type": "json_object"}` (OpenAI-compat key) instead of Ollama-native `format: "json"` so classification calls receive structured JSON from all endpoints.
- `src/agentix/bridge/classify_prompt.py` — filter context to `user`/`assistant` roles only before classification (exclude `system` messages carrying working-memory identity to prevent persona contamination).
- `src/agentix/bridge/classify_prompt.py` — pass `system_prompts_dir` from `AgentixConfig` to `PromptLoader` so the classification system prompt is loaded correctly when invoked from the bridge.
- `agentx.toml` — set `agentix_bench_classification_model = "phi4-mini:3.8b"` (neutral model suitable for JSON classification; replaces agent-persona model).
- `system_prompts/prompt_classification.md` — updated to produce valid JSON output conforming to `PromptClassificationResponse` schema.

### Test Changes

#### Added

- `tests/integration/test_bootstrap_e2e.py` — 10 `@pytest.mark.live` end-to-end bootstrap tests verifying the full classification pipeline against a live Ollama instance.
