# Hybrid Remaining Work Synthesis

<!-- markdownlint-disable MD036 MD060 MD047 -->

Last updated: 2026-06-01 (execution-enabled)

## Purpose

This document is the working synthesis for what remains in the hybrid migration.
It is now the single planning reference for unfinished runtime and UX parity work.

Authoritative direction:

- Go core is the main runtime application and must orchestrate the full
  prompt-classification-thinking-tool-response cycle.
- Go core manages communications between applets and owns runtime state transitions.
- GUI is a secondary feature and remains back-burnered until TUI parity is complete.

## Current Progress Context

Completed and stable foundation work:

- Go core orchestrates tmux/session lifecycle, startup bootstrap, demo-mode flow,
  and core routing boundaries.
- System-panel mappings and contracts are documented and partially validated.
- Demo harness and hardening checks exist and provide baseline regression signals.
- Channel registry, runtime split, and demo-mode contracts are documented.

| Area          | Completed outcome                                                   | Primary evidence |
| ------------- | ------------------------------------------------------------------- | ----------------- |
| Runtime split | Go core and Go applet responsibilities are defined                   | [docs/architecture/runtime_split.md](architecture/runtime_split.md) |
| System tabs   | Tab/provider mapping and tab-tour contracts exist                    | [docs/architecture/system_panel_tab_mapping.md](architecture/system_panel_tab_mapping.md), [docs/ux/07_DEMO_MODE.md](ux/07_DEMO_MODE.md) |
| Event model   | Channel semantics and activity-state contract are documented         | [docs/architecture/channel_registry.md](architecture/channel_registry.md) |
| Demo/UAT      | Baseline demo scenarios and hardening checks exist                  | [docs/ux/07_DEMO_MODE.md](ux/07_DEMO_MODE.md) |

## Resume Checkpoint (Cumulative Progress)

Since this execution direction started (native Go ownership for chat/output and
logs with parity-first validation), cumulative progress is:

- Go chat routing is the checked-in default and now uses direct deterministic
  fallback in the Go path (no Python bridge fallback dependency).
- Output/chat pane rendering is Go-native via widget launch path.
- Logs pane is now Go-native via `--logs-widget` launch path.
- Integration, bootstrap, and supervisor contracts were updated to match the
  direct-go-fallback + native-logs behavior.
- Full blocker gate is currently green after the latest rerun
  (`hybrid parity blocker gate complete`).

Very brief remaining focus:

- Complete applet-by-applet UX traceability closure for parity sign-off.
- Retire stale migration claims in downstream docs/tests that still imply
  Python ownership for chat/logs/system-pane tab surfaces.
- Keep blocker gate green after each incremental migration slice.

## Remaining Work

Parity is incomplete overall, but the PD-18 system applet suite slices are now
implemented and validated. Remaining work now shifts to the broader runtime and
UX parity gaps outside the completed applet-suite slice.

Remaining work is implementation-first, not documentation-first.

Go applet architecture policy:

- Each runtime applet is implemented as a Go application.
- Applets must leverage a shared Go base architecture (common lifecycle,
  IPC contracts, health signaling, logging, and runtime guards).
- Each applet must be validated against UX specifications.
- Validation requires unit tests plus integration and functional tests.
- Each applet must have a dedicated UX spec row and traceability row before it
  can be marked complete.
- The optional UAT-visible startup mode is implemented as a validation surface,
  not a separate ad hoc demo path.

| Workstream                | Remaining work                                                                 | Why it matters                                                               | Status               |
| ------------------------- | ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- | -------------------- |
| Core orchestration completion | Complete Go-owned prompt-classification-thinking-tool-response runtime orchestration | This is the primary product behavior and must be deterministic in TUI         | In progress          |
| Applet parity implementation | Implement Go-created/runtime-managed applet flows to match UX specifications, one applet at a time, with individual tests and UX review sign-off | Current flows are incomplete and parity claims are premature                  | In progress          |
| Applet UX review and traceability | Add PD-18 panel details, individual affordance IDs, and UX lifecycle rows for each applet before marking parity complete | The team needs a reviewable spec/test chain for each applet                   | In progress          |
| UAT-visible startup mode | Optional startup switch now launches one visible window per applet before frame layout (`--startup-mode visible-windows`) | UAT can verify applet presence/function before frame layout work               | Completed            |
| TUI-first completion       | Finish TUI behavior and parity gates before advancing GUI feature work         | GUI is explicitly secondary until TUI completion                              | In progress          |
| Traceability correction    | Remove stale "complete" claims where implementation remains partial            | Keeps engineering state honest and executable                                 | In progress          |
| Code reality audit         | Verify still-stubbed logic in `cmd/agentx-core`, `applets/template.py`, and `src/agentx` | Confirms actual implementation coverage against UX contracts                   | Completed            |

## Pruned Objectives

These objectives are treated as obsolete in this synthesis and should not be
carried forward as active backlog items:

- The old Sprint 1/2/3 rollout structure from the hybrid migration plan.
- The separate Wave 1/2/3/4 execution board as a planning source of truth.
- The legacy AgentX/Agentix integration roadmap as an active implementation plan.

## Archived Planning Set

The AgentX/Agentix integration plan is archived for historical context and is no
longer part of active hybrid runtime planning.

Archive location:

- `docs/archive/agentx-agentix-integration/`

## Active Inconsistencies To Resolve

The two high-priority inconsistencies have been resolved by policy direction and
must now be enforced in implementation work.

| Inconsistency | Resolution policy | Enforcement expectation |
| ------------- | ----------------- | ----------------------- |
| Go-owned parity is claimed complete while applet/flow implementation remains incomplete | Applet/flow parity is explicitly incomplete until Go applets match UX specs. Each applet is a Go app on a shared base architecture and must pass integration + functional validation. | No parity-complete claim until applet-level UX spec validation passes. |
| TUI priority vs GUI positioning is inconsistent | TUI is first. GUI remains secondary/back-burnered until TUI completion gates are satisfied. | GUI features do not advance priority ahead of TUI parity blockers. |

## Next Step

Drive implementation and validation from this synthesis, with TUI-first completion and Go-core runtime ownership as the gating criteria for parity.
HYB-04 shared applet base scaffolding is now in place in Go core, and HYB-05 first
vertical slice is delivered by formalizing runtime-managed applet specs with the
input applet as an explicit Go-native runtime slice.
Go-native system/context rendering prerequisite is now implemented in core with
tab-aware parity-contract output and focused renderer tests.
Context runtime-kind migration from python to go is now complete and validated
through `make hybrid-parity-gate`.
Active next execution slices (parallel-ready):

- Slice A (Lane D): feature-flagged Go-native chat runtime path with direct
  deterministic Go fallback is now implemented and validated through blocker gates.
- Slice B (Lane E): renderer cadence/tab-switch regression expansion is complete
  (state-file tab routing + repeated-turn tab stability coverage).
- Slice C (Lane A): traceability cleanup for stale python-context assumptions is
  complete in architecture/runtime docs.

Dependency rule: completed slices remain integrated only when
`make hybrid-parity-gate` is green.

Active next parallel-ready slices:

- Slice D (Lane D): default-promotion control path for chat runtime is now
  implemented via `AGENTX_CHAT_RUNTIME_DEFAULT` (safe staged evidence path,
  explicit runtime override still wins).
- Slice E (Lane E): focused regressions for go-chat fallback telemetry and
  backend failure attribution are complete.

Dependency rule: completed slices remain integrated only when
`make hybrid-parity-gate` is green.

Active next parallel-ready slices:

- Slice F (Lane D): completed. Cutover criteria and bounded experiment window
  for default chat-runtime promotion are now fixed as policy:
  - Gate prerequisite: `make -C /Projects/agentX hybrid-parity-gate` must be green
    for the candidate commit.
  - Exposure window: 3 consecutive execution days in staged mode using
    `AGENTX_CHAT_RUNTIME_DEFAULT=go` with explicit override path retained.
  - Success threshold: zero blocker regressions and no net increase in
    `go_chat_fallback` attributable failures during the window.
  - Rollback trigger: any blocker-gate failure attributable to chat runtime
    default behavior or repeated bridge-unavailable fallback incidents.
  - Rollback action: revert staged default by unsetting
    `AGENTX_CHAT_RUNTIME_DEFAULT` (or setting to `python`) while preserving
    explicit override support (`AGENTX_CHAT_RUNTIME`).
- Slice G (Lane E): completed. Integration-level telemetry ordering assertions
  are implemented for mixed go-chat fallback paths in
  `cmd/agentx-core/features/integration.feature` and step wiring in
  `cmd/agentx-core/godog_test.go`.
  - Additional negative-path coverage now includes intermittent bridge startup
    latency behavior with timeout-on-first-prompt and recovery-on-second-prompt
    ordering assertions under go-chat fallback.
  - Additional negative-path coverage now includes intermittent bridge
    start-error jitter behavior with start-error fallback on first prompt and
    bridge recovery ordering on second prompt.
  - Additional negative-path coverage now includes delayed backend error jitter
    behavior (delayed non-2xx backend response) with ordered go-chat fallback
    routing to direct deterministic fallback behavior.
  - Additional recovery-path coverage now includes delayed backend recovery
    transition behavior (first prompt fallback, second prompt direct backend
    success) with deterministic sequenced backend stub assertions.
- Slice H (Lane D/E): completed. Baseline logs-pane POC parity is now explicitly
  validated in demo UX flow execution.
  - Added `e2e-logs-001` (`Logs pane startup and bridge lifecycle parity`) to
    the default demo sequence in `cmd/agentx-core/demo_harness.go`.
  - Added logs-window capture/polling path for non-active window validation and
    stabilized matcher criteria for startup + bridge lifecycle signal presence.
  - Updated headless UX/tour scripts to include the logs parity use-case in
    acceptance sequencing:
    - `tests/test_demo_ux_use_cases_headless.sh`
    - `tests/test_demo_system_panel_tour_headless.sh`
  - Updated DemoMode parity expectation test coverage in
    `cmd/agentx-core/demo_harness_test.go`.
- Slice I (Lane D/E): completed. Input-command parity and system-tour renderer
  hardening are now integrated in the POC baseline flow.
  - Added `e2e-input-001` (`Input clear command parity preserves output pane`)
    to the default demo sequence in `cmd/agentx-core/demo_harness.go` with
    deterministic marker assertions for output persistence across `:clear`.
  - Hardened Go-runtime system-pane rendering command in
    `cmd/agentx-core/core.go` to avoid shell `printf` format interpretation on
    `%`-bearing context visualizer rows (`printf '%s' <payload>` pattern).
  - Verified the tour path no longer fails on context-visualizer tab checks when
    consumed-percentage rows are present.
- Slice J (Lane D/E): completed. System-pane tab-state stabilization is now
  enforced for prompt/system parity checks.
  - Updated `cmd/agentx-core/demo_harness.go` so `e2e-cycle-001` and
    `e2e-system-001` explicitly set system tab state to `context-visualizer`
    before validating prompt-cycle/context-usage contracts.
  - This removes cross-run dependence on persisted `system-panel-tab.txt` state
    (for example stale `files` tab selection) and stabilizes POC baseline checks.
- Slice K (Lane D/E): completed. Transient submit-timeout hardening is now
  enforced for input parity seeding.
  - Updated `cmd/agentx-core/demo_harness.go` so `e2e-input-001` seeds prior
    output with bounded retry handling for transient submit failures (`504`,
    `submit timed out`, `context deadline exceeded`) before asserting `:clear`
    output-preservation behavior.
  - This removes intermittent false negatives caused by one-off bridge/backend
    latency while preserving deterministic failure behavior for non-transient
    submit errors.
- Slice L (Lane E): completed. Regression coverage for transient submit-timeout
  hardening is now in place.
  - Added focused tests in `cmd/agentx-core/demo_harness_test.go` for:
    - transient error classification (`isTransientDemoSubmitError`)
    - retry success path (`submitDemoPromptWithRetry` retries a 504 then succeeds)
    - fail-fast behavior for non-transient submit failures
  - This locks the timeout-retry policy into automated coverage and guards
    against future regressions.
- Slice M (Lane D/E): completed. Clear-command submit path now uses the same
  transient retry policy as seed submit in input parity validation.
  - Updated `cmd/agentx-core/demo_harness.go` so `e2e-input-001` applies
    bounded transient retry handling to `:clear` submission, not just seed
    prompt submission.
  - This reduces intermittent false negatives when transient timeout behavior
    occurs during clear-command dispatch.
- Slice N (Lane E): completed. Lightweight retry-frequency observability signal
  is now wired into demo submit retry handling.
  - Added a process-local retry counter in `cmd/agentx-core/demo_harness.go`
    that increments on each transient submit retry.
  - Added tests in `cmd/agentx-core/demo_harness_test.go` to verify counter
    behavior for transient-retry and non-transient fail-fast paths.
- Slice O (Lane E): completed. Retry observability signal is now surfaced in
  DemoMode run summaries.
  - Updated `cmd/agentx-core/demo_harness.go` to print
    `Submit retries observed: <count>` in summary output and reset the counter
    at run start for per-run scoping.
  - This makes transient-retry activity visible in baseline parity executions
    without changing parity pass/fail criteria.
- Slice P (Lane E): completed. Parity-gate execution is now serialized to avoid
  concurrent-run interference.
  - Updated `Makefile` so `hybrid-parity-gate` acquires a lock directory and
    runs through `hybrid-parity-gate-inner` only when no active gate lock is
    present.
  - Concurrent invocation now fails fast with an explicit lock message instead
    of overlapping tmux-heavy acceptance runs.
- Slice Q (Lane E): completed. Submit-retry observability signal is now exposed
  through the machine-consumable health payload.
  - Updated `cmd/agentx-core/core.go` to include `submit_retries` in
    `HealthSnapshot` and in `/health` endpoint JSON output.
  - Added/updated endpoint assertions in
    `cmd/agentx-core/core_health_endpoint_test.go` to verify the field is
    surfaced.
- Slice R (Lane D/E): completed. System/context pane rendering now uses a
  dedicated Go context widget process instead of shell-echo repaint loops.
  - Added `--context-widget` mode in `cmd/agentx-core/main.go` and new widget
    implementation in `cmd/agentx-core/context_widget.go`.
  - Updated pane applet launch wiring in `cmd/agentx-core/core.go` so go-runtime
    context pane launches `agentx-core --context-widget --core-http ...`.
  - Disabled overlapping core-side context render loop when pane applets are
    active to avoid competing redraw paths.
  - Added widget tests in `cmd/agentx-core/context_widget_test.go` and updated
    launch-contract tests in `cmd/agentx-core/core_applet_supervisor_test.go`.
- HYB-06 / Slice S (Lane D/E): completed. PD-18 Slice-1: system applet host
  scaffolding, context-history applet, startup-mode scaffold, and all tab-widget
  live-contract fixes are now implemented and validated.
  - Added `cmd/agentx-core/system_applet_host.go`: `SystemApplet` interface,
    `SystemAppletHost`, `SystemAppletCoreContext`, `SystemAppletWidgetContext`,
    `systemAppletRegistry`, and `contextHistorySystemApplet`.
  - Updated `cmd/agentx-core/core.go`: `AgentXCore.systemAppletHost` field,
    `NewAgentXCore()` wiring, and `renderSystemSurface()` routing
    `context-history` through the host.
  - Updated `cmd/agentx-core/context_widget.go`: `context-history` tab now
    resolves through `newSystemAppletHost().Resolve(tab)`; `files` tab now
    emits `project_dir:` and `entry_count:` from env/fs; `configuration` tab now
    emits `ollama_host:` — all three fix the live demo-tour contract gap.
  - Updated `cmd/agentx-core/config.go` and `main.go`: `StartupMode` field +
    `--startup-mode` flag with `default|visible-windows` validation (scaffold
    only; layout behavior is not yet implemented).
  - Added focused tests:
    - `cmd/agentx-core/system_applet_host_test.go`
    - `cmd/agentx-core/config_startup_mode_test.go`
    - tests added to `cmd/agentx-core/context_widget_test.go` (host routing,
      files contract, configuration contract)
    - tests added to `cmd/agentx-core/core_system_renderer_test.go`
  - Repaired live system-tour regression: `files` and `configuration` tabs were
    failing the demo validator because they lacked `entry_count:` and
    `ollama_host:` fields respectively. Both fixed and re-validated.
- HYB-03 / Slice T (Lane B): completed. Real deterministic classify phase is
  now implemented in Go core prompt-cycle state transitions.
  - Added `cmd/agentx-core/core_classify.go`: `ClassifyIntent`,
    `ClassifyNextStep`, `ClassifyResult` types + `classifyPrompt()` function
    using rule-based deterministic logic (no LLM): safety check first, then
    built-in command detection, then complex-action heuristics (keyword set,
    multi-sentence, long-prompt word count), then simple-action verb detection,
    then conversation default. Evaluation order mirrors the system-prompt
    contract in `system_prompts/prompt_classification.md`.
  - Updated `cmd/agentx-core/core.go`: `PromptCycleStatus.ClassifyResult`
    field added; `finishClassifyPhase` now accepts and stores a `ClassifyResult`;
    `RoutePrompt` calls `classifyPrompt(trimmedPrompt)` before finishing the
    classify phase so the result is always available in health snapshots and
    activity state before thinking begins.
  - Added `cmd/agentx-core/core_classify_test.go`: 8 focused unit tests
    covering empty prompt, built-in commands, safety escalation, simple-action
    verbs, planning keywords, question/conversation default, long-prompt
    heuristic, multi-sentence heuristic, and result storage in prompt cycle.
- HYB-03 / Slice U (Lane B): completed. `ClassifyResult.NextStep` now drives
  actual routing decisions in `RouteInputPrompt`.
  - Added `failClassifyPhase()`: marks classify phase as "failed" (from
    "running"); used for safety-escalation abort before thinking begins.
  - Added `skipToolPhase()`: marks tool phase as "skipped" (from "pending")
    allowing `startRespondPhase` to proceed without a tool cycle.
  - Updated `startRespondPhase` guard: accepts `tool.State == "skipped"` in
    addition to `"done"` so the `respond_directly` routing path can complete.
  - Updated `RouteInputPrompt` routing block:
    - `escalate` → `failClassifyPhase()` + return error (safety abort).
    - `respond_directly` → `skipToolPhase()` (no tool cycle for conversations).
    - `single_tool | invoke_planner` → full tool cycle (existing behavior).
  - Added 4 new state-machine tests in
    `core_prompt_cycle_state_machine_test.go`: skip-tool path completes,
    fail-classify sets state, skip from non-pending is rejected, respond
    reaches done after skip-tool.
  - Fixed `TestRouteInputPrompt_EmitsLifecycleStagesInOrder` and
    `TestRouteInputPrompt_PythonBridgeLifecycleStagesInOrder`: updated test
    prompts to `"list the files here"` (`simple_action`) so the tool-phase
    lifecycle marker (`stage=tool`) is still exercised.
- HYB-06 / Slice V (Lane D/E): completed. UAT-visible startup topology is now
  implemented with `visible-windows` behavior and safe fallback to default.
  - Updated `cmd/agentx-core/core.go`: `InitializeTmuxSession()` now applies
    `visible-windows` topology when `Config.StartupMode == visible-windows`,
    creating one window per applet surface (`output`, `logs`, `input`,
    `system`) with semantic title bindings; if mode initialization fails, core
    tears down and recreates the session in default layout.
  - Added startup-mode behavior coverage in
    `cmd/agentx-core/core_tmux_startup_integration_test.go`:
    `TestInitializeTmuxSession_VisibleWindowsStartupMode` validates no
    split-window usage, expected per-window creation commands, input-focus
    targeting, and pane-target map contracts.
  - Added authoritative startup-mode catalog document:
    `docs/architecture/startup_modes.md` and linked it from
    `docs/architecture.md`, `docs/architecture/runtime_split.md`, and
    `docs/ux/00_INDEX.md`.
- HYB-06 / Slice W (Lane D/E): completed. Working-memory applet parity is now
  delivered as a read-only Go-side working-memory surface.
  - Slice objective: render session facts deterministically from the active
    session directory without mutation controls.
  - PD-18 target: `PD-18-AF-005` in `docs/ux/03_PANEL_DETAILS.md` and the
    matching traceability row in `docs/ux/UX_LIFECYCLE.md`.
  - Validation expectation: unit tests plus system-panel coverage now pass.
- Slice X (Lane D): completed. Checked-in project default now promotes chat
  routing to the native Go path while preserving pane/demo parity contracts.
  - Updated `agentx.toml` to set `chat_runtime = "go"` as the repository
    default while keeping explicit env overrides authoritative.
  - Follow-on hardening replaced the chat pane template process with a native Go
    output widget so the visible output pane now launches through Go while
    preserving startup greeting and prompt-render parity.
  - Follow-on hardening removed the Python bridge from the checked-in Go chat
    backend recovery path; backend failures now fall back directly to the
    deterministic Go echo response instead of routing through the Python applet.
  - Added a native Go logs widget so the logs pane now launches through
    `agentx-core --logs-widget --core-http ...` rather than relying on template
    applet rendering for the pane process lifecycle.
  - Added native-Go routing telemetry markers (`go_chat_route_start`,
    `go_chat_response_ok`) so the logs pane continues to satisfy parity
    contracts without disturbing existing fallback ordering assertions.

Dependency rule: completed slices remain integrated only when
`make hybrid-parity-gate` stays green.

Latest validation snapshot (Slice X: native Go chat as checked-in default):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX test-demo-ux-use-cases-headless`: passing.

Latest validation snapshot (native Go output widget follow-on):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX test-demo-ux-use-cases-headless`: passing.

Latest validation snapshot (direct Go fallback + native Go logs widget):

- `cd /Projects/agentX/cmd/agentx-core && go test -run 'LogsWidget|BuildPaneAppletLaunchCommand|StartAppletSupervisor|GoChatRuntime|StartupBootstrap' ./...`: passing.
- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX test-demo-ux-use-cases-headless`: passing.

Latest validation snapshot (post-native-go-chat/logs full blocker gate rerun):

- `make -C /Projects/agentX hybrid-parity-gate`: passing (`hybrid parity blocker gate complete`).

Latest validation snapshot (post-recovery-slice hardening):

- `cd /Projects/agentX/cmd/agentx-core && go test -v -run TestGoDogIntegration ./...`:
  20 scenarios, 301 steps, all passing.
- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX hybrid-parity-gate`: passing (exit code 0).

Latest validation snapshot (post-logs-pane POC slice):

- `cd /Projects/agentX/cmd/agentx-core && go test -run 'TestDefaultDemoSequence_IncludesSprint3ParityStories|TestRunDemoMode_AllAcceptedShowsReadyForUAT' -v ./...`: passing.
- `make -C /Projects/agentX test-demo-ux-use-cases-headless`: passing (includes `e2e-logs-001`).
- `make -C /Projects/agentX test-demo-system-panel-tour-headless`: passing (includes `e2e-logs-001`).
- `make -C /Projects/agentX hybrid-parity-gate`: passing (exit code 0).

Latest validation snapshot (post-input-command + renderer-hardening slice):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX test-demo-ux-use-cases-headless`: passing (includes `e2e-input-001` + `e2e-logs-001`).
- `make -C /Projects/agentX test-demo-system-panel-tour-headless`: passing (includes `e2e-system-tour-001` + `e2e-input-001` + `e2e-logs-001`).
- `make -C /Projects/agentX hybrid-parity-gate`: passing (`hybrid parity blocker gate complete`).

Latest validation snapshot (post-system-tab stabilization slice):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX test-demo-ux-use-cases-headless`: passing.
- `make -C /Projects/agentX test-demo-system-panel-tour-headless`: passing.
- `make -C /Projects/agentX hybrid-parity-gate`: passing (`hybrid parity blocker gate complete`).

Latest validation snapshot (post-submit-timeout hardening slice):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX test-demo-ux-use-cases-headless`: passing (`e2e-input-001` PASS).
- `make -C /Projects/agentX hybrid-parity-gate`: passing (`hybrid parity blocker gate complete`, exit code 0).

Latest validation snapshot (post-health submit-retry exposure slice):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX hybrid-parity-gate`: attempted twice after change,
  both runs stalled at demo start of `e2e-system-tour-001` (lock held, no
  completion marker). Runs were terminated and lock was cleared.

Latest validation snapshot (post-context widget rendering slice):

- `cd /Projects/agentX/cmd/agentx-core && go test -run 'TestRunContextWidgetLoop_SkipsDuplicateFrames|TestRenderContextWidget_ClipsToViewport|TestBuildPaneAppletLaunchCommand_ContextPaneUsesNativeWidget|TestStartAppletSupervisor_LaunchesPaneAppletProcesses' ./...`: passing.
- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.

Latest validation snapshot (post-retry-regression-test slice):

- `cd /Projects/agentX/cmd/agentx-core && go test -run 'TestIsTransientDemoSubmitError|TestSubmitDemoPromptWithRetry_RetriesTransientFailureThenSucceeds|TestSubmitDemoPromptWithRetry_DoesNotRetryNonTransientFailure' ./...`: passing.
- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX hybrid-parity-gate`: passing (`hybrid parity blocker gate complete`).

Latest validation snapshot (post-clear-submit retry extension slice):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX hybrid-parity-gate`: passing (`hybrid parity blocker gate complete`).

Latest validation snapshot (post-retry-observability slice):

- `cd /Projects/agentX/cmd/agentx-core && go test -run 'TestSubmitDemoPromptWithRetry_RetriesTransientFailureThenSucceeds|TestSubmitDemoPromptWithRetry_DoesNotRetryNonTransientFailure|TestIsTransientDemoSubmitError' ./...`: passing.
- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX hybrid-parity-gate`: passing (exit code 0).

Latest validation snapshot (post-retry-summary-signal slice):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- Full parity run showed Demo summary retry signal lines, for example:
  - `[AgentX Demo] Submit retries observed: 0`
- `make -C /Projects/agentX hybrid-parity-gate`: passing on clean run before a
  later process-interrupted rerun.

Latest validation snapshot (post-gate-serialization slice):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX hybrid-parity-gate`: completed with
  `hybrid parity blocker gate complete` on clean run.
- Forced-lock verification:
  - with lock directory pre-created, `make hybrid-parity-gate` fails fast with
    `hybrid-parity-gate already running`.

Latest validation snapshot (HYB-06 / PD-18 Slice-1: system applet host + live
widget contract repair):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing (19+ tests, 0 failed).
- `make -C /Projects/agentX test-demo-system-panel-tour-headless`: PASS —
  `e2e-system-tour-001` now covers all 5 tabs including repaired `files` and
  `configuration` contracts.
- `make -C /Projects/agentX hybrid-parity-gate-inner`: completed, exit code 0,
  `hybrid parity blocker gate complete`. All 6 sub-checks passed:
  - `go-test-functional` (20 scenarios, 301 steps)
  - `go-test-integration`
  - `go-test-e2e` (2 scenarios, 20 steps)
  - `demo-smoke` PASS
  - `test-demo-ux-use-cases-headless` PASS (greet/cycle/input/logs/system)
  - `test-demo-system-panel-tour-headless` PASS (system-tour + 5 other cases)

Latest validation snapshot (HYB-03 / Slice U: classify result drives routing):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing (all tests, 0 failed).
- `make -C /Projects/agentX hybrid-parity-gate-inner`: completed, exit code 0,
  `hybrid parity blocker gate complete`. All 6 sub-checks passed:
  - `go-test-functional` PASS
  - `go-test-integration` PASS
  - `go-test-e2e` PASS
  - `demo-smoke` PASS
  - `test-demo-ux-use-cases-headless` PASS
  - `test-demo-system-panel-tour-headless` PASS

Latest validation snapshot (HYB-03 / Slice T: real deterministic classify phase):

- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing (all tests, 0 failed).
- `make -C /Projects/agentX hybrid-parity-gate-inner`: completed, exit code 0,
  `hybrid parity blocker gate complete`. All 6 sub-checks passed:
  - `go-test-functional` PASS
  - `go-test-integration` PASS
  - `go-test-e2e` PASS
  - `demo-smoke` PASS
  - `test-demo-ux-use-cases-headless` PASS (5/5: greet/cycle/input/logs/system)
  - `test-demo-system-panel-tour-headless` PASS (system-tour + 5 other cases)

Latest validation snapshot (HYB-06 / Slice V: visible-windows startup mode):

- `cd /Projects/agentX/cmd/agentx-core && go test -run 'TestInitializeTmuxSession_PrimaryWindowSelectionRegression|TestInitializeTmuxSession_VisibleWindowsStartupMode|TestNormalizeStartupMode|TestResolveStartupModeDefault' ./...`: passing.
- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- `make -C /Projects/agentX hybrid-parity-gate`: passing (exit code 0).

Effective immediately, active execution priority shifts from promotion governance
to baseline applet functionality and POC validation.

Rationale:

- Backend/fallback stability confidence has increased through repeated green gates.
- Remaining value risk is now missing or unvalidated applet-by-applet UX-spec
  parity and startup visibility.
- Promotion-window governance is lower priority until baseline POC behavior is
  visibly functional and test-validated.

Updated parallel execution lanes:

- Lane D (POC applet functionality): implement/validate baseline end-to-end
  applet behavior first (chat/input/context/system/logs user-visible flow).
- Lane E (POC validation): add/expand integration and demo assertions for
  baseline applet behavior and UX flow acceptance.
- Promotion governance work (staged-window telemetry automation) is parked
  until POC baseline criteria are met.

POC baseline completion criteria:

- Core applet user flows execute end-to-end in interactive/demo paths.
- UX flow expectations are validated for baseline applet behaviors.
- `make -C /Projects/agentX hybrid-parity-gate` remains green after each
  POC functionality increment.

## Parallel Execution Mode (Active)

This work is now explicitly parallelized with one orchestrator lane and four
specialist lanes.

Execution policy:

- One authoritative plan: this file.
- Implementation-first execution with strict WIP limits.
- No GUI-priority work until TUI parity blocker gates are green.
- No "parity complete" claim until blocker gates pass with evidence.

### Lane Ownership and WIP Limits

| Lane | Focus | Ownership boundary | WIP limit |
| ---- | ----- | ------------------ | --------- |
| Lane A | Control + code reality audit | Ticket sequencing, blockers, and reality checks in core/apply/runtime paths | 1 |
| Lane B | Core orchestration completion | `cmd/agentx-core` runtime state machine and prompt lifecycle ownership | 1 |
| Lane C | Shared Go applet base | Go applet lifecycle, IPC contract scaffolding, health/logging/guards | 1 |
| Lane D | Applet parity vertical slices | Applet behavior parity against UX contracts on shared base | 2 |
| Lane E | Validation + traceability | Blocker gates, parity evidence, and stale-claim cleanup | 1 |

### Week 1 Start Queue

Start these tickets in order and pull strictly by dependency:

| Ticket | Goal | Primary owner lane | Depends on |
| ------ | ---- | ------------------ | ---------- |
| HYB-01 | Code reality audit for `cmd/agentx-core`, `applets/template.py`, and `src/agentx` | Lane A | None |
| HYB-02 | Define and enforce TUI parity gate contract | Lane E | HYB-01 |
| HYB-03 | Complete deterministic core prompt lifecycle in Go | Lane B | HYB-01 |
| HYB-04 | Implement shared Go applet base architecture | Lane C | HYB-01 |
| HYB-05 | Deliver first applet parity vertical slice with integration + functional pass | Lane D | HYB-02, HYB-03, HYB-04 |
| HYB-06 | PD-18 Slice-1: system applet host + context-history applet + test plan execution | Lane D/E | HYB-02, HYB-04, HYB-05 |

### PD-18 Slice Packet (Archived)

Implementation and test execution for the first applet vertical slice was
defined in:

- `docs/architecture/system_applet_suite_slice1.md`

This packet remains the historical build-and-validate guide for HYB-06. It
defines:

- concrete implementation scope for the first system applet slice
- file-level delivery targets in `cmd/agentx-core`
- required unit, integration, and demo/UAT validation commands
- acceptance and rollback criteria tied to PD-18 affordance IDs

The active next work should now follow the remaining hybrid slices listed above,
with Slice W already delivered and validated.

### First Coding Objective (HYB-03)

The first coding objective for core runtime parity is:

- Replace synthetic classify completion with a real deterministic classify phase
  in Go core prompt-cycle state transitions.

Done criteria:

- `startPromptCycle` initializes classify as running, not pre-completed.
- Route lifecycle transitions explicitly through classify -> thinking -> tool ->
  respond.
- Health/activity snapshots reflect in-flight classify state truthfully.
- Prompt cycle and lifecycle ordering tests updated and passing.

### Blocker Gate Contract (Required)

All blocker gates must pass before parity status is advanced:

1. `make go-test-functional`
2. `make go-test-integration`
3. `make go-test-e2e`
4. `make demo-smoke`
5. `make test-demo-ux-use-cases-headless`
6. `./tests/test_demo_system_panel_tour_headless.sh`

Checkpoint policy:

- Any blocker failure keeps parity status open.
- Any stale "complete" claim must be removed when blockers fail.
- GUI-priority work is blocked while any TUI parity blocker remains red.

### Daily Cadence (Low Overhead)

Use lightweight synchronization only:

1. 09:00-09:12: pull next ticket + declare blockers
2. 13:00-13:10: dependency arbitration only
3. 16:30-16:50: parity gate review + demo readiness check