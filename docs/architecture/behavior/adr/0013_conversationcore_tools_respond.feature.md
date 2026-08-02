# Behavior — ConversationCore: Native Tool Execution and Response Streaming (ADR 0013 Phase 3)

Status: **Implemented** (2026-08-01). Realizes ADR 0013's Phased Build Plan step 3:
`runNativeToolCall`, `streamResponse`, `finishCycle`, `availableToolSchemas`, and
`maxToolIterations` move onto `ConversationCore`. Two corrections surfaced during
implementation that weren't anticipated when Phase 2 was scoped — both fixed here,
recorded below rather than silently patched over.

**Later removed (2026-08-02):** `ToolReadOnly`/`toolReadOnly`/`tools.read_only`,
described throughout this document as a live-reloadable setting, no longer
exist — read-only mode was removed entirely; approval-gating
(`Policy.Evaluate`/`RequestApproval`) is now the sole tool-execution gate. See
`docs/architecture/behavior/tool_policy_read_only_removal.feature.md`. This
document is left as-is below as the historical record of what Phase 3 actually
built and why (including the live-reload bug this phase fixed for that
now-removed setting) — not rewritten to erase that it existed.

## Correction 1: `toolSchemasFn` is permanent, not a Phase 3 cleanup target

Phase 2's plan assumed all four of `streamFn`/`finishFn`/`toolSchemasFn`/`maxIterFn`
would become native Core methods in Phase 3, retiring the closures. Building it
surfaced that `availableToolSchemas` isn't purely generic-catalog logic — it also
decides whether to advertise `plan_task`:

```go
func (o *Orchestrator) availableToolSchemas() []tools.ToolSchema {
	var out []tools.ToolSchema
	if o.toolsReady() {
		out = append(out, o.registry.ToolSchemas(o.settings.ToolReadOnly)...)
	}
	if o.planReady() {
		out = append(out, planTaskSchema())
	}
	return out
}
```

`planReady()` depends on Orchestrator-only state (`taskDecomp`, `wavefrontClassifier`)
that ADR 0013 §"Explicitly not decided here" keeps off `ConversationCore` for good.
Moving `availableToolSchemas` onto Core wholesale would have reintroduced exactly that
coupling. The fix: split it. Core gets a native `toolSchemas()` (the generic,
registry-driven catalog only — `core_tools.go`); `Orchestrator.availableToolSchemas`
becomes a thin wrapper (`o.core.toolSchemas()` + `plan_task` when `planReady()`), and
**stays** the thing `toolSchemasFn` is bound to. `toolSchemasFn` is therefore permanent,
exactly like `execTool` — both are entangled with plan_task, and both stay
Orchestrator-mediated for the same reason (Open Question 4).

## Correction 2: Settings must be read live, never snapshotted, on Core

Phase 2's `ConversationCore` snapshotted `thinkingEnabled bool` and
`thinkingPromptTemplate string` once, at `buildCore` time. Checking
`Orchestrator.applyLiveSettings` (`orchestrator.go:1851-1968`) while building this
phase found that `thinking.enabled`, `thinking.time_budget_seconds`, `tools.enabled`,
and `tools.read_only` are all **live-reloadable** — mutated directly on `o.settings`
under `o.mu`, without a restart, by `Orchestrator.SetConfig`/`applyLiveSettings`. A
value snapshotted once at `buildCore` time would have silently stopped tracking a
live-reload edit to any of these — a real regression Phase 2 had already introduced
undetected (no existing test exercises live-reload of `thinking.enabled` against the
extracted loop).

The fix, applied uniformly rather than field-by-field: every Settings-derived value
`ConversationCore` needs is read through a closure (`thinkingEnabled func() bool`,
`thinkingPromptText func() string`, `thinkingBudget func() time.Duration`,
`toolsEnabled func() bool`, `toolReadOnly func() bool`, `maxIterSetting func() int`),
bound by `buildCore` to read `o.settings.X` fresh on every call — mirroring the
pre-extraction code's own discipline (`doThink := o.settings.ThinkingEnabled` was read
inline inside `runPrompt`'s body on every call, never cached). None of these closures
take `o.mu` — the pre-extraction code didn't either at these read sites, and adding
locking here would be a behavior change beyond this ADR's extraction mandate, not a
faithful move.

The one exception is `modelName string`, which **is** snapshotted once — safe because
Provider/model changes are restart-required config keys (`SetConfig`'s
`restartRequiredKeys`), and a restart calls `Start()` again, which rebuilds `o.core`
via `buildCore` from scratch.

## Correction 3 (minor): `EventSink` needed a second method

Moving `runNativeToolCall` surfaced that Orchestrator's pre-extraction
`publishToolCall`/`publishToolResult` (formerly `approval.go`/`tool_cycle.go`) never
went through `publishEv`/`Publish` at all — they called `o.bus.Publish` directly with
an extra `ToolName` field `state.Event` carries but `EventSink.Publish`'s generic
signature has no room for. Fixed by adding `PublishTool(evt ToolEvent) uint64` to
`EventSink` (`core.go`) — a second interface method, added in this phase rather than
worked around with a permanent closure, since it's squarely "this loop's events," the
job `EventSink` exists for.

**Also preserved, not fixed:** `publishToolCall`/`publishToolResult` stamped
`ModelName: o.settings.OllamaModel` unconditionally — a pre-existing discrepancy from
every other event, which reads `o.modelName()` (provider-aware). `Orchestrator.PublishTool`
reproduces this exactly. ADR 0013 is an extraction, not a bugfix pass; flagged here for
visibility, not silently carried forward unremarked.

## What moved where

- **`core_tools.go` (new):** `ConversationCore.toolsReady`, `.toolSchemas`,
  `.runNativeToolCall`, `.publishToolCall`, `.publishToolResult` — moved verbatim in
  control flow from `tool_cycle.go`/`approval.go`. `o.gate` reaches become
  `c.approvals.RequestApproval`/`RequestOutputSizeDecision`; `o.publishToolCall`/
  `o.publishToolResult` become `c.publishToolCall`/`c.publishToolResult` via
  `EventSink.PublishTool`.
- **`core_respond.go` (new):** `ConversationCore.maxToolIterations`, `.streamResponse`,
  `.finishCycle` — moved verbatim from `tool_cycle.go`. `o.setProcessing`/
  `o.publishEv`/`o.model`/`o.modelName()` reaches become `c.report`/
  `c.events.Publish`/`c.model`/`c.modelName`.
- **`tool_cycle.go` (trimmed):** keeps `buildTools`, `Orchestrator.toolsReady`
  (unchanged — `classifier_pipeline.go`'s disconnected pipeline still calls it
  directly, deliberately not unified with Core's copy), `toolPin`, the free functions
  `toolResultText`/`toolResultContext`/`toolDeniedContext`. `availableToolSchemas`
  is now the thin wrapper described in Correction 1. `streamResponse`/`finishCycle`
  are now thin wrappers delegating to `o.core.streamResponse`/`o.core.finishCycle` —
  kept on `Orchestrator` because `continuation.go:119` and
  `classifier_pipeline.go:437,439` call them directly and were out of scope to touch.
  `runNativeToolCall` and `publishToolCall`/`publishToolResult` are fully retired from
  `Orchestrator` (their only caller, `loop.go`'s `runToolOrPlan`, now calls
  `o.core.runNativeToolCall` directly).
- **`core.go`:** `EventSink` gains `PublishTool`; `Orchestrator.PublishTool` added.
- **`core_loop.go`:** `ConversationCore` gains `registry`/`policy`/`runner`/
  `approvals`/`model`/`modelName` plus the six Settings closures; loses `streamFn`/
  `finishFn`/`maxIterFn` (now native calls); `RunPrompt` calls `c.streamResponse`/
  `c.maxToolIterations`/`c.finishCycle` directly instead of the retired closures.

```
GIVEN a live-reload config write flips thinking.enabled (or tools.enabled,
      tools.read_only, thinking.time_budget_seconds) while the orchestrator is
      running, with no restart
WHEN  the next RunPrompt call (or, for tools.*, the next runNativeToolCall) runs
THEN  it observes the new value immediately — ConversationCore's closures read
      o.settings fresh, exactly as the pre-extraction inline reads did.

GIVEN Settings.Provider or Settings.OllamaModel/LlamacppModel changes
WHEN  the change is applied
THEN  it requires a restart (SetConfig's restartRequiredKeys, unchanged by this
      phase); Core's modelName snapshot is safe because Start()/buildCore rebuild
      it from scratch on every restart.

GIVEN toolsReady() is false (tools disabled, or registry/policy/runner not built)
WHEN  toolSchemas() is called
THEN  it returns nil — plan_task's availability is decided independently by
      Orchestrator's planReady(), never gated on Core's own toolsReady().

GIVEN a native tool call that needs approval
WHEN  runNativeToolCall reaches the NeedsApproval branch
THEN  it calls c.approvals.RequestApproval — approved, it executes via c.runner;
      the approval-interrupt error propagates unchanged, never reaching c.runner.

GIVEN a tool call published as a tool_call/tool_result event
WHEN  publishToolCall/publishToolResult run
THEN  the published event's ModelName is settings.OllamaModel (preserved
      bug-for-bug from pre-extraction behavior — see Correction 3), not the
      dynamic, provider-aware modelName() every other event uses.
```

## Tests

- `internal/runtime/core_tools_test.go` (new): `TestConversationCoreToolsReady`,
  `TestConversationCoreToolSchemas` (including the plan_task-exclusion assertion),
  `TestConversationCoreRunNativeToolCallExecutesDirectly`,
  `TestConversationCoreRunNativeToolCallDeniesUnderReadOnly`,
  `TestConversationCoreRunNativeToolCallRequestsApproval` (against `write_file`, a
  real `RequiresApproval: true` descriptor in the default registry — not a synthetic
  rule),
  `TestConversationCoreRunNativeToolCallPropagatesApprovalInterrupt`.
- `internal/runtime/core_loop_test.go` (rewritten for Phase 3's shape): all six Phase
  2 scenarios now drive a `stubModel` (`Model` interface) instead of a bare `streamFn`
  closure, since `streamResponse` is native and calls `c.model.Chat` directly.
  `TestConversationCoreRunPromptInterruptDuringToolApproval`'s assertion changed from
  counting `finishFn` calls (no longer swappable) to asserting `reportState`'s call
  count/final state — `streamResponse` itself reports `StateWorking` before the model
  call, so the interrupt path now yields exactly two `report` calls, not one; a third
  would mean `finishCycle` also ran, which it must not.
- Full existing suite (`go test ./...`, `-race` on `internal/runtime/...`) and
  `make all` (vet + full suite + build) pass unchanged — including `tests/suites`'
  Godog run, which exercises `continuation.go`/`classifier_pipeline.go`'s direct
  `streamResponse`/`finishCycle` calls through the thin wrappers.
