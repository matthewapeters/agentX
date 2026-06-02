# Hybrid Remaining Work Synthesis

<!-- markdownlint-disable MD036 MD060 MD047 -->

Last updated: 2026-06-02 (execution-enabled)

## Latest Runtime Delta (2026-06-02)

- Dedicated system applet windows are now created at startup so users can cycle
  through them with tmux window navigation (`C-b n`):
  - `files`
  - `configuration`
  - `context-history`
  - `working-memory`
- Core render loop now continuously updates those dedicated applet windows in
  addition to the composed system pane view.
- Input applet interaction contract now includes context-file registration via:
  - `:context-add <file-path>` (alias: `:ctx-add <file-path>`)
- Input applet prompt label now visualizes the most recent context file from
  `/activity` snapshot metadata (cross-applet UX signal).

Parallel Track Delta (A/B/C) on 2026-06-02:

- Track A (`output`): per-entry interactive commands were expanded with
  entry-focused collapse/expand/toggle behavior and deterministic app-level
  copy actions.
- Track B (`input`): multiline compose mode was implemented with explicit
  `:multiline`/`:submit`/`:cancel` flow and discoverability updates.
- Track C (`filesystem`): persistent soft-select set behavior was integrated
  with deterministic multi-file attach compatibility while preserving
  Enter/Return hard-select semantics.

Validation evidence:

- Focused tests:
  - `TestInitializeTmuxSession_PrimaryWindowSelectionRegression`
  - `TestInitializeTmuxSession_VisibleWindowsStartupMode`
  - `TestHandleInputLine_ContextAddCommand`
  - `TestWidgetActivityState_PromptLabelIncludesContextFile`
  - `TestFetchWidgetActivitySnapshot_Success`
- Full core module regression:
  - `cd /Projects/agentX/cmd/agentx-core && go test ./...`

## Purpose

This document is the working synthesis for what remains in the hybrid migration.
It is now the single planning reference for unfinished runtime and UX parity work.

Authoritative direction:

- UX documentation is authoritative for requirements and affordances. It is
  implementation-agnostic (GUI/TUI/hybrid) and must not be regressed to fit the
  current runtime.
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
- Migration slice is now captured in `CHANGELOG.md` (`1.0.2`) and consolidated
  in commit `dc90265` on `feat/hybrid-go-core-tui-migration`.
- Applet traceability was previously marked closed for the system-applet suite,
  but runtime inventory still tracks only `chat`, `input`, `logs`, and
  `context` pane applets. This mismatch is now treated as active remaining
  work.

Very brief remaining focus:

- Keep downstream docs/tests aligned with current Go-owned runtime claims and
  prevent stale migration wording from re-entering active paths.
- Keep blocker gate green after each incremental migration slice.

## Windowed Demo Validation Readiness

`agentx --demo --windowed` is now a valid UAT validation path for applet
surfaces.

Readiness signal:

- Run when `make -C /Projects/agentX hybrid-parity-gate` is green for the
  candidate commit.

Validation command:

- `agentx --demo --windowed --project-dir /Projects/agentX`

Applet-level UAT checklist for this run:

- Output/chat applet renders startup greeting + prompt lifecycle rows.
- Input applet accepts prompt and preserves output on `:clear`.
- Logs applet shows startup bootstrap and lifecycle telemetry markers.
- System applet coverage must explicitly include first-class File System,
  System Settings, and Context-family (current context + context history +
  working memory) surfaces per UX, not only tab-level rendering inside the
  context pane.

## Remaining Work

Parity is incomplete overall. The prior PD-18 "closed" claim is superseded:
runtime-level applet inventory and UX-required surfaces are not yet aligned.
Remaining work includes closing this applet-scope gap in addition to broader
runtime and UX parity gaps.

Remaining work is implementation-first and must converge to the UX contract;
requirements do not change to accommodate implementation shortcuts.

Go applet architecture policy:

- Each runtime applet is implemented as a Go application.
- Applets must leverage a shared Go base architecture (common lifecycle,
  IPC contracts, health signaling, logging, and runtime guards).
- Each applet must be validated against UX specifications.
- Validation requires unit tests plus integration and functional tests.
- Each applet must have a dedicated UX spec row and traceability row before it
  can be marked complete.
- Requirement-complete rule: no applet may be marked complete until all
  authoritative UX and UX-flow requirements for that applet are met in runtime
  behavior and backed by executable evidence.
- All interactive affordances must be implemented in TUI; if an affordance has
  no obvious implementation path, it must be escalated for user case-by-case
  decision before closure.
- The optional UAT-visible startup mode is implemented as a validation surface,
  not a separate ad hoc demo path.

| Workstream                | Remaining work                                                                 | Why it matters                                                               | Status               |
| ------------------------- | ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- | -------------------- |
| Core orchestration completion | Complete Go-owned prompt-classification-thinking-tool-response runtime orchestration | This is the primary product behavior and must be deterministic in TUI         | In progress          |
| Applet parity implementation | Implement Go-created/runtime-managed applet flows to match UX specifications, one applet at a time, with individual tests and UX review sign-off | Current flows are incomplete and parity claims are premature                  | In progress          |
| Missing UX-specified system applets | Deliver first-class File System and System Settings runtime applets and use tmuxp composition logic for assembled system layout | Aligns runtime architecture with planned first-class applet model while preserving UX-composed experience | In progress |
| Applet UX review and traceability | Re-open PD-18 traceability to align runtime applet inventory with UX-required applet surfaces before re-closing sign-off | Maintains an auditable spec/test/sign-off chain per applet and prevents false-complete claims | In progress |
| UAT-visible startup mode | Optional startup switch now launches one visible window per applet before frame layout (`--startup-mode visible-windows`) | UAT can verify applet presence/function before frame layout work               | Completed            |
| TUI-first completion       | Finish TUI behavior and parity gates before advancing GUI feature work         | GUI is explicitly secondary until TUI completion                              | In progress          |
| Traceability correction    | Remove stale "complete" claims where implementation remains partial            | Keeps engineering state honest and executable                                 | In progress          |
| Code reality audit         | Verify still-stubbed logic in `cmd/agentx-core`, `applets/template.py`, and `src/agentx` | Confirms actual implementation coverage against UX contracts                   | Completed            |
| Files overflow parity requirements | Implement `PD-11-AF-011..014` in TUI filesystem applet (viewport/paging/overflow status + evidence) | Professional-grade file navigation requires deterministic behavior beyond visible viewport | Completed |
| Files keyboard affordance parity | Implement `PD-11-AF-015..017` (arrow navigation, `Space` soft-select, `Return` hard-select) with visible selection-state feedback | User-expected TUI file navigation affordances must match UX contract | Completed |
| No-obvious-path escalation protocol | Enforce `PD-11-AF-018`: case-by-case user decision required when an affordance lacks clear TUI implementation path | Prevents silent de-scoping and protects UX-authoritative delivery | Completed |
| Output affordance parity scope closure | Reconcile `PD-01` interactive affordances with TUI output implementation and close approved parity scope | Prevents implicit de-scoping and contradictory sign-off language | Completed |
| Output per-entry interactive parity (Path A) | Implement full per-entry interactive controls in TUI output surface (`PD-01`) | User-selected direction requires feature-equivalent TUI interaction fidelity | Completed |
| Output explicit copy actions (Path A) | Implement deterministic app-level TUI copy actions for output (`PD-01-AF-010`) | Avoids terminal-dependent behavior drift and closes copy affordance parity | Completed |
| Input affordance parity closure | Close remaining `PD-02` parity deltas for command behavior/discoverability and acceptance evidence | Keeps prompt-entry UX consistent and auditable across runtimes | Completed |
| Input multiline parity (Path A) | Implement richer multiline key handling for TUI input (`PD-02`) | User-selected direction prioritizes multiline interaction fidelity over simplified single-line flow | Completed |
| Context ownership ambiguity closure | Resolve context vs split applet ownership boundaries and normalize naming across architecture/UX docs | Eliminates ambiguity that causes contradictory completion reporting | In progress |
| System settings parity closure | Finalize system-settings dedicated-runtime acceptance criteria and evidence mapping to `PD-07`/`PD-18-AF-003` | Required for professional-grade applet sign-off | In progress |
| Files persistent multi-select semantics (Path A) | Implement persistent `Space` soft-select set behavior with explicit visible state and action compatibility (`PD-11-AF-016`) | User-selected direction aligns with professional TUI file-manager patterns | In progress (persistent set + attach compatibility landed; remaining action semantics to finalize) |

## Applet Readiness Review (2026-06-02)

This review answered four questions per applet: professional implementation
level, UX clarity, requirements coverage, and unresolved ambiguity.

| Applet | Professional level | UX clarity | Requirements met | Ambiguity present | Queue action |
| --- | --- | --- | --- | --- | --- |
| `chat/output` | Yes (current scope) | Good | Mostly | No | Maintain regression coverage and lifecycle parity evidence. |
| `input` | Yes (current scope) | Good | Mostly | No | Maintain multiline/command discoverability regression coverage. |
| `logs` | Yes (current scope) | Good | Mostly | No | Maintain diagnostics readability and lifecycle evidence; no new blocker row. |
| `context` | Partial | Mixed | Partial | Yes | Complete ownership closure execution and keep PD-18 re-opened until evidence is complete. |
| `filesystem` | Yes (current scope) | Good | Mostly | No | Maintain viewport/select/action compatibility regression coverage. |
| `system-settings` | Partial | Good | Partial | No | Execute system-settings parity closure tied to `PD-07` and `PD-18-AF-003`. |

Review counts (after Path A direction selection):

- Applets requiring additional work: 2
- Ambiguities pending direction decision: 0

Resolved direction example:

- `PD-01` per-entry interactive output affordances are now directed as Path A
  implementation work (full TUI interaction), not deferred ambiguity.

## Applet Build/Test Plan (Open)

This section tracks applet surfaces with open ownership/evidence closure tasks.

Decision baseline (finalized):

- File System and System Settings are first-class runtime applets.
- tmuxp composition logic assembles first-class applets into the composed
  system UX layout.
- Working Memory remains composed under Session-tab context ownership unless UX
  is explicitly revised.

| Target applet surface | UX anchor(s) | Current state | Required build work | Required test work | Planned runtime target |
| --- | --- | --- | --- | --- | --- |
| File System applet (first-class, finalized) | `PD-18-AF-004`, Files panel references in `PD-11` | First-class host-owned files applet runtime is now implemented in Go (`SystemAppletHost` + dedicated files applet render path); tmuxp composition binding remains pending | Complete tmuxp composition binding and runtime inventory traceability alignment for files applet ownership | Unit: file listing/state behavior. Integration: startup/reattach + applet health. Functional: demo/system-tour parity | Dedicated first-class applet target; composed into system UX via tmuxp |
| System Settings applet (first-class, finalized) | `PD-18-AF-003`, `PD-07` settings behavior references | First-class host-owned configuration applet runtime is now implemented in Go (`SystemAppletHost` + dedicated configuration applet render path); tmuxp composition binding remains pending | Complete tmuxp composition binding and runtime inventory traceability alignment for settings applet ownership | Unit: config render/override behavior. Integration: state refresh/startup + applet health. Functional: system-tab tour assertions | Dedicated first-class applet target; composed into system UX via tmuxp |
| Context-history applet (first-class process) | `PD-18-AF-002` | Implemented through system applet host/tab rendering; first-class process decomposition not finalized | Decide if decomposition is required beyond current host-based surface; if yes, add first-class applet runtime process | Unit: history ordering/truncation. Integration: ownership/reattach. Functional: demo tour path | Same as above; depends on decomposition decision |
| Working-memory surface reconciliation | `PD-03`, `PD-08`, `PD-18-AF-005` | Surface implemented; traceability remains re-opened due inventory alignment drift | Keep under context applet ownership unless UX explicitly changes; or split to first-class applet by approved spec change | Unit: WM row/callback contracts. Integration: session ownership. Functional: explicit WM acceptance path in tour | Context applet-owned surface by default; split target only if UX contract changes |
| Context-current applet/surface reconciliation | `PD-18` `context` system tab + `PD-03` context section | Surface implemented in context widget; inventory naming and ownership still mixed in docs | Reconcile naming so one authoritative ownership model is used across runtime docs/UX lifecycle/plan | Regression sweep for context summary + prompt-cycle exposure consistency | Context applet-owned surface |

Execution order for open applet work:

1. File System tmuxp composition wiring + traceability closure.
2. System Settings tmuxp composition wiring + traceability closure.
3. Context-history decomposition decision and execution.
4. Working-memory ownership reconciliation closure.
5. Context-current naming/ownership reconciliation closure.

Completion criteria for this section:

1. File System and System Settings split-vs-compose decisions are finalized and documented as first-class applets composed via tmuxp.
2. Every accepted row has unit + integration + functional evidence.
3. `docs/ux/UX_LIFECYCLE.md` and this plan agree on final applet inventory status.

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

Latest validation snapshot (post-1.0.2 consolidation commit):

- `make -C /Projects/agentX hybrid-parity-gate`: passing (exit code 0,
  `hybrid parity blocker gate complete`).
- Release traceability updated in `CHANGELOG.md` (`1.0.2`) and committed as
  `dc90265`.

Latest validation snapshot (post-files-applet first-class host slice):

- `cd /Projects/agentX/cmd/agentx-core && go test ./... -run 'FilesSystemApplet|SystemAppletHost_ResolvesFiles|RenderContextWidget_FilesTabContract|RenderSystemSurface_TabContracts|RenderSystemSurface_TabSwitchStableAcrossTurns'`: passing.
- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- Blocker sub-gates passing:
  - `make -C /Projects/agentX go-test-functional`
  - `make -C /Projects/agentX go-test-integration`
  - `make -C /Projects/agentX go-test-e2e`
  - `make -C /Projects/agentX demo-smoke`
  - `make -C /Projects/agentX test-demo-ux-use-cases-headless`
  - `make -C /Projects/agentX test-demo-system-panel-tour-headless`

Latest validation snapshot (post-settings-applet first-class host slice):

- `cd /Projects/agentX/cmd/agentx-core && go test ./... -run 'ConfigurationSystemApplet|SystemAppletHost_ResolvesConfiguration|RenderContextWidget_ConfigurationTabContract|RenderSystemSurface_TabContracts|RenderSystemSurface_TabSwitchStableAcrossTurns'`: passing.
- `cd /Projects/agentX/cmd/agentx-core && go test ./...`: passing.
- Blocker sub-gates passing:
  - `make -C /Projects/agentX go-test-functional`
  - `make -C /Projects/agentX go-test-integration`
  - `make -C /Projects/agentX go-test-e2e`
  - `make -C /Projects/agentX demo-smoke`
  - `make -C /Projects/agentX test-demo-ux-use-cases-headless`
  - `make -C /Projects/agentX test-demo-system-panel-tour-headless`

Latest validation snapshot (post-layout-flag alignment + default-layout workflow):

- CLI/help alignment delivered:
  - `--layout` is now the primary composition flag (`--layout-file` retained as compatibility alias)
  - `--dump-default-layout <file|->` now exports the built-in default composition
  - implicit default composition path documented and exercised: `.agentx/layouts/default-layout.yaml`
- Blocker sub-gates passing:
  - `make -C /Projects/agentX go-test-functional`
  - `make -C /Projects/agentX go-test-integration`
  - `make -C /Projects/agentX go-test-e2e`
  - `make -C /Projects/agentX demo-smoke`
  - `make -C /Projects/agentX test-demo-ux-use-cases-headless`
  - `make -C /Projects/agentX test-demo-system-panel-tour-headless`

Latest validation snapshot (post-strict tmux/tmuxp probe hardening):

- Runtime prerequisite checks now include executable probes:
  - `tmux -V`
  - `tmuxp --version`
- Implicit default layout composition load (`.agentx/layouts/default-layout.yaml`)
  now fails fast on overlay load failure; explicit custom layout load failures
  remain non-fatal fallback paths.
- Focused + module tests:
  - `cd /Projects/agentX/cmd/agentx-core && go test ./... -run 'ValidateRuntimePrerequisites|ApplyOptionalLayoutOverlay'`
  - `cd /Projects/agentX/cmd/agentx-core && go test ./...`
- Blocker sub-gates passing:
  - `make -C /Projects/agentX go-test-functional`
  - `make -C /Projects/agentX go-test-integration`
  - `make -C /Projects/agentX go-test-e2e`
  - `make -C /Projects/agentX demo-smoke`
  - `make -C /Projects/agentX test-demo-ux-use-cases-headless`
  - `make -C /Projects/agentX test-demo-system-panel-tour-headless`
- Demo smoke harness compatibility note:
  - `tests/test_demo_smoke_headless.sh` now adds fake-tmux `-V` support and
    invokes `--layout <missing-custom-layout>` to exercise non-fatal custom
    overlay fallback while preserving artifact assertions.

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