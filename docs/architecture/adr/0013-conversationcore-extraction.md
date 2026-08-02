# ADR 0013: ConversationCore — Extracting the Prompt/Tool/Hook Loop from Orchestrator

Status: **Implemented** (2026-08-01) — all 5 phases of the Phased Build Plan shipped.
`ConversationCore` is independently instantiable and runnable with zero `Orchestrator`
involvement (Phase 5's standalone/concurrent tests). What consumes a second instance
(a sub-session, a planning tool) remains future work — see Open Questions, unchanged.
**Note (2026-08-02):** Phase 3's `toolReadOnly` closure (§"Architecture — insertion
points", the `ToolReadOnly`/`tools.read_only` live-reload example below) describes a
setting that has since been removed entirely — read-only mode is gone;
approval-gating is the sole tool-execution gate now. Left unedited below as the
historical record of Phase 3's actual design; see
`docs/architecture/behavior/tool_policy_read_only_removal.feature.md`.
Date: 2026-08-01
Deciders: AgentX architecture owners
Depends on: ADR 0008 (branch context — the existing in-memory sub-session precedent),
ADR 0009 (execution visibility), ADR 0012 (wavefront — whose "Future direction"
amendment names engine interleaving/sub-problem routing as open, unscoped future work
this ADR is a precondition for, not a continuation of)
Scope: this ADR decides the shape of `ConversationCore` and how it is carved out of
`Orchestrator`. It deliberately does **not** design a planning/decomposition tool, a
sub-session consumer, or anything that uses `ConversationCore` for nested execution —
those are future work, named only as motivation in Context and left as Open Questions.

## Context

`Orchestrator` (`internal/runtime/orchestrator.go`) currently owns two things that are
conceptually separate but structurally fused into one struct and one method body:

1. **The prompt/tool/hook loop** — `runPrompt` (`internal/runtime/loop.go:24-105`):
   submit → LLM (advertising tool schemas) → detect tool calls vs. a chat answer →
   execute tools/fold results back → loop, with `hooks.RunSync`/`RunAsync` firing at
   the two hook points, bounded by `maxToolIterations`.
2. **Session ownership** — the fact that there is exactly one canonical, durable,
   transport-attached conversation per `Orchestrator`: it satisfies
   `transporthttp.Provider` (`orchestrator.go:29`), owns a config file watcher, an
   event bus (`bus *state.Bus`) that attached surfaces subscribe to, on-disk session
   persistence (`store *session.Store`), and the concrete human-approval queue
   (`gate decisionGate`, `internal/runtime/decision.go:18` — a queue of
   `approvalUIRequest`s, not an abstraction).

These are fused today because there has only ever been one conversation per process.
That stops being true the moment anything needs a second, nested prompt/tool/hook loop
running inside a tool call — a documented, not-yet-built direction: `hooks.go` already
carries `SpawnDepth`/`CanSpawn`/`NextSpawnContext` (`internal/runtime/hooks/hooks.go:87-114`)
explicitly for "a hook [that] spawns a recursive loop instance (a sub-agent)," and ADR
0012's amendment names "engine-selection policy" and "hand-off protocol" as deliberately
unscoped future work once a sub-problem needs its own decomposition pass. Neither of
those can be acted on today without either (a) instantiating a second full
`Orchestrator` — which drags in transport, config-watching, and session persistence a
nested loop has no use for and no wiring to — or (b) hand-writing a bespoke second loop
that duplicates `runPrompt`'s logic and drifts from it over time.

This ADR's job is narrow: make the loop itself an independently-constructible unit, so
that question ("what does a nested loop actually need") has a real answer later, without
committing now to who calls it or what it's for.

### What's already cleanly separable (evidence, not assumption)

Grepped directly rather than assumed: `Model` (`internal/runtime/model.go:22`) and
`ToolRunner` (`internal/runtime/approval.go:15`) are already interfaces; `tools.Registry`,
`tools.Policy`, `tools.OutputOverrides`, and `hooks.Registry` are already independent
types held as plain `Orchestrator` fields, not methods baked into it. Pulling these into
a smaller struct is mostly regrouping, not new abstraction.

### What's currently fused and needs a seam

Three loop-body concerns reach directly into session-only state today and have no
abstraction between them:

- **Approval-seeking.** `runNativeToolCall` (`internal/runtime/tool_cycle.go:101-155`)
  calls `o.RequestApproval`/`o.RequestOutputSizeDecision` directly against the concrete
  `gate decisionGate`. There is no interface a nested loop could satisfy differently
  (e.g., delegate up to the same human, or run under a stricter no-mutation policy).
- **Event publishing.** `streamResponse`, `runNativeToolCall`, and `finishCycle`
  (`tool_cycle.go:171-240`) call `o.publishEv`/`o.publish`
  (`orchestrator.go:1216-1236`) directly against `o.bus` and the session's `id.ID`. A
  nested loop publishing here would leak into the live chat transcript — which ADR
  0008's `branch.Branch` already deliberately avoids for its own (non-conversational)
  sub-sessions: *"records events to its own log — never the parent conversation"*
  (`internal/runtime/branch/branch.go:5`, `Emit`/`Events` at lines 117-122). A
  conversational nested loop needs the equivalent for message-shaped output, and
  doesn't have it.
- **Context assembly and turn recording.** `withContext`
  (`orchestrator.go:1095-1115`) reads `o.store.LoadWorkingMemory` (on-disk, session-scoped)
  and `o.history` (an in-process `[]turnMsg`, `orchestrator.go:236`) to build the next
  model call's messages; `recordTurn` (`orchestrator.go:603-628`) writes back into that
  same `o.history`. Both are hardwired to *the* session's store and *the* session's
  history slice. `branch.Branch` already solved the read side of this differently for
  its own (non-loop) use — a read-only `Facts()` snapshot at fork time, never a live
  store read (`branch.go:37,94-98`) — but nothing generalizes that pattern to a full
  conversational context yet.

`o.mu`/`started`/`accepting` (session lifecycle: is this process currently accepting
external prompts) and `o.ctxWindow` (a cached, session-lifetime value) are session-only
by nature — a nested loop is constructed and run once, synchronously, inside a tool
call; it has no independent "accepting" state to guard.

## Decision

### 1. `ConversationCore` — a new type, same package, not a new subpackage

`ConversationCore` lives in `internal/runtime` (a new file, `core.go`), **not** a new
`internal/runtime/core` subpackage. Reasons: it needs `toolPin`, `ChatResult`, and the
`turnMsg`-shaped bookkeeping that are currently unexported `runtime`-package types, and
per `docs/implementation/08_go_module_layout.md:77-84`, `internal/runtime`'s existing
siblings (`decompose`, `scheduler`, `wavefront`) are for genuinely separable
*engines*, not for the central loop the package itself exists to hold. Cutting a new
subpackage boundary here would force exporting internal types purely to satisfy the
split, which is churn the go-module-layout doc's own guidance doesn't ask for. This
also means no "new top-level folder" Change Control question arises (mirroring how ADR
0012 §"Documentation" reasoned about `wavefront`'s siblinghood).

```go
// Illustrative — exact signatures settle during phase implementation, not here.
type ConversationCore struct {
    model             Model
    registry          *tools.Registry
    policy            *tools.Policy
    runner            ToolRunner
    outputOverrides   *tools.OutputOverrides
    hooks             *hooks.Registry
    approvals         ApprovalSeeker
    events            EventSink
    convo             ContextStore
    maxToolIterations int
    thinkingBudget    time.Duration
    thinkingPrompt    string
}

func (c *ConversationCore) RunPrompt(ctx context.Context, text string, opts RunOptions) (RunOutcome, error)
```

`RunPrompt` is `runPrompt`'s current body (`loop.go:24-105`), minus every direct
`o.bus`/`o.store`/`o.gate`/`o.mu`/`o.history` reach — those go through the three new
interfaces below instead. `runNativeToolCall`, `streamResponse`, `finishCycle`,
`availableToolSchemas`, `maxToolIterations` move onto `ConversationCore` largely as-is,
with their approval/publish call sites redirected to `c.approvals`/`c.events`.

### 2. Three new interfaces — the actual seam

```go
// ApprovalSeeker requests a policy decision for a proposed tool call or an
// oversized-output recovery choice. The session Orchestrator's implementation wraps
// its existing concrete decisionGate (UI round-trip); a future nested loop's
// implementation is an open question (Open Questions below) — options range from
// delegating to the same gate (a mutating tool inside a nested loop prompts the real
// human, same as today) to a stricter deny-by-default policy, mirroring ADR 0008's
// existing read-only branch model. Not designed here — only the seam is.
type ApprovalSeeker interface {
    RequestApproval(ctx context.Context, d tools.Descriptor, args map[string]string, p *tools.Policy) (tools.Verdict, error)
    RequestOutputSizeDecision(ctx context.Context, d tools.Descriptor, args map[string]string, res tools.Result) (tools.Result, bool, error)
}

// EventSink records this loop's events. The session Orchestrator's implementation
// wraps o.bus.Publish (live, transport-visible). A future nested loop's
// implementation is the ConversationCore-shaped equivalent of branch.Branch's
// Emit/Events — an isolated, in-memory log, never published to the parent bus.
type EventSink interface {
    Publish(eventType string, ct state.ContentType, payload any, ephemeral bool) uint64
}

// ContextStore supplies this loop's next-call context and records what happened.
// The session Orchestrator's implementation wraps o.store.LoadWorkingMemory +
// o.history. A future nested loop's implementation is the conversational analogue of
// branch.Branch's Facts()/SetLocalFact/LocalWM split: a read-only snapshot in, a
// local-only accumulation out, never merged back except through an explicit seal step.
type ContextStore interface {
    Augment(base []prompting.Message) []prompting.Message
    Record(entry TurnRecord)
}
```

These three interfaces are the entire seam. Everything else `ConversationCore` needs
(`Model`, `tools.Registry`/`Policy`/`OutputOverrides`, `hooks.Registry`,
`ToolRunner`) is already independently typed and just becomes a constructor
parameter — no new abstraction required, per the grep evidence in Context.

### 3. What stays on `Orchestrator`, unconditionally

Transport (`transporthttp.Provider`, `server`, `endpoint`, `serveDone`, `token`,
`surfaceReg`), config watching (`configWatcher`, `configWatcherStop`, `restartQueue`,
`liveReloadEnabled`), session persistence and identity (`store`, `id`, `bus`, `proc`,
`recDone`, `recSub`), the concrete `gate decisionGate` and its UI round-trip, `Settings`
loading and the `Start()` bootstrap sequence (classifier, `taskPipeline`, `taskDecomp`,
`wavefrontClassifier`/`wavefrontChat`, tool blacklist/approvals file I/O), `planTrees`
(UI plan-tree rendering), `ctxWindow` caching, and all theme/UI settings
(`ActiveBorderColor`, `MarkdownRenderer`, `MaxWidgetLines`, `InputMaxLines`). `Orchestrator`
becomes: *the thing that boots from `Settings`, owns transport/config/persistence, and
constructs+drives one `ConversationCore` as the canonical session* — its own
`ApprovalSeeker`/`EventSink`/`ContextStore` implementations are thin wrappers over what
it already owns (`gate`, `bus`+`id`, `store`+`history`), not new logic.

### 4. Explicitly not decided here

- `plan_task`'s special-casing in `runToolOrPlan` (`loop.go:107-118`) is unchanged.
  Whether `plan_task` (or any future planning tool) becomes a `ConversationCore`
  consumer is out of scope for this ADR by explicit instruction — noted only as the
  motivating future direction in Context.
- No sub-session/nested-loop consumer is designed here. `ApprovalSeeker`'s and
  `ContextStore`'s nested-loop implementations are named as open questions, not decided.
- No decision on whether `RunOutcome`/processing-state reporting (`setProcessing`,
  `state.RunState`/`state.Phase`) generalizes to a nested loop, or whether a nested
  loop instead reports through the existing `Observer`/`NodeDispatched`-style plan
  visibility channel (ADR 0009/0012). Flagged, not resolved.

## Architecture — insertion points

- **New:** `internal/runtime/core.go` — `ConversationCore`, `ApprovalSeeker`,
  `EventSink`, `ContextStore`, `RunOptions`, `RunOutcome`, `TurnRecord`.
- **Changed:** `internal/runtime/loop.go` — `runPrompt` becomes a thin method on
  `Orchestrator` that builds `RunOptions` (the `recordUserPrompt`/`ephemeral` flags
  that today gate `publishEv` calls) and delegates to `o.core.RunPrompt`.
- **New:** `internal/runtime/core_tools.go` — `ConversationCore.toolsReady`/
  `.toolSchemas`/`.runNativeToolCall`/`.publishToolCall`/`.publishToolResult`.
- **New:** `internal/runtime/core_respond.go` — `ConversationCore.maxToolIterations`/
  `.streamResponse`/`.finishCycle`.
- **Changed:** `internal/runtime/tool_cycle.go` — trimmed to `buildTools`,
  `Orchestrator.toolsReady` (kept, unchanged — `classifier_pipeline.go` still calls
  it), `toolPin`, the free render functions, and thin `availableToolSchemas`/
  `streamResponse`/`finishCycle` wrappers (the latter two kept because
  `continuation.go`/`classifier_pipeline.go` call them directly). `runNativeToolCall`
  fully retired (only caller was `loop.go`, now calling `o.core.runNativeToolCall`).
- **Changed:** `internal/runtime/approval.go` — `publishToolCall` retired (moved to
  `core_tools.go`, published via the new `EventSink.PublishTool`).
- **New:** `internal/runtime/core_context.go` — `Orchestrator.Augment`/`.Record`
  (moved from `core.go`), `turnMsg`, `.recordTurn`, `.historyMessages`,
  `.withContext`, `mergeSystemMessages`, `.workingMemoryMessage`,
  `.workingMemoryFacts`, `pinAnnotatedValue` — all still `Orchestrator` methods,
  relocated for cohesion, not moved onto `ConversationCore` (see Phase 4 below for
  why that would be wrong, not just unnecessary).
- **Changed:** `internal/runtime/orchestrator.go` — gains a `core *ConversationCore`
  field, built at `Start()` from the pieces `Orchestrator` already constructs
  (`registry`, `policy`, `runner`, `outputOverrides`, `hooks`, `model`); gains its own
  `ApprovalSeeker`/`EventSink`/`ContextStore` implementations wrapping `gate`,
  `bus`+`id`, and `store`+`history` respectively. `withContext`, `workingMemoryMessage`,
  `historyMessages`, `recordTurn` move (behavior-preserving) into the
  `Orchestrator`-side `ContextStore` implementation.
- **Unchanged:** everything under "What stays on `Orchestrator`, unconditionally" above.

## Consequences

Positive:

- `ConversationCore` becomes independently constructible and testable against fakes for
  all three new interfaces, without booting transport, config-watching, or session
  persistence — a real unit-testing win independent of any future nested-loop consumer.
- The three interfaces make explicit, at the type level, exactly what a future
  nested-loop consumer would need to supply — turning "can a sub-session run a real
  conversation loop" from an open architectural question into a concrete, boundable
  implementation task, without committing to that task now.
- No behavior change for the existing single-session path if each phase below is
  genuinely behavior-preserving — the session Orchestrator's `ApprovalSeeker`/
  `EventSink`/`ContextStore` implementations are wrappers over exactly what it does
  today, not new logic.

Trade-offs:

- Real, multi-site refactor of already-proven code (`loop.go`, `tool_cycle.go`,
  `orchestrator.go`'s context/history methods) — not a mechanical rename. Needs full
  regression coverage before and after each phase, matching the discipline ADR 0012's
  amendment applied to its own `scheduler.go` refactor.
- Three new interfaces are permanent surface area to keep in sync with `Orchestrator`'s
  own evolution (a new approval-decision shape, a new event-publishing concern) even
  before any second implementation of them exists.
- `RunOptions`/`RunOutcome`/`TurnRecord`'s exact shape is illustrative here and will
  need real design during Phase 2 below — this ADR fixes the seam's existence and
  location, not its final field list.

## Phased Build Plan

Each phase is behavior-preserving for the existing single-session path: existing tests
(`orchestrator_test.go`, `loop`/`tool_cycle`-adjacent tests) must pass unchanged after
every phase. Behavior docs are written immediately before each phase's implementation,
per repo convention (not written as part of this ADR):

1. **Implemented (2026-08-01).** Define the three interfaces + `Orchestrator`-side
   implementations, wired but unused. `ApprovalSeeker`/`EventSink`/`ContextStore` added
   to `internal/runtime/core.go`; `Orchestrator` grows thin wrapper methods
   (`Publish`/`Augment`/`Record`) satisfying each — `RequestApproval`/
   `RequestOutputSizeDecision` already matched `ApprovalSeeker` with no new code.
   `runNativeToolCall`/`streamResponse`/`withContext`/`recordTurn` kept calling
   `o.gate`/`o.bus`/`o.store`/`o.history` directly, unchanged. Zero behavior change,
   confirmed by the full pre-existing suite passing unchanged. Behavior doc:
   `docs/architecture/behavior/adr/0013_conversationcore_seams.feature.md`.
2. **Implemented (2026-08-01).** Extract `ConversationCore` and move the loop body.
   `runPrompt`'s logic moved to `ConversationCore.RunPrompt` (`core_loop.go`);
   `Orchestrator.runPrompt` (`loop.go`) is now a four-line delegator: the
   `o.mu`/`started`/`accepting` check, `refreshLiveFacts`, build `RunOptions`,
   delegate. `RunOptions`/`RunOutcome`/`TurnRecord`'s shapes settled as designed.
   Four of `ConversationCore`'s dependencies (`streamFn`/`finishFn`/`toolSchemasFn`/
   `maxIterFn`) are Phase 3's explicit stepping stone — closures bound to
   `Orchestrator`'s still-unmoved methods by the new `buildCore` (called from
   `Start()` after `o.hooks` is built). The fifth, `execTool` (bound to
   `runToolOrPlan`), is permanent, not temporary — see the behavior doc for why.
   Behavior doc: `docs/architecture/behavior/adr/0013_conversationcore_runprompt.feature.md`.
3. **Implemented (2026-08-01).** Move `runNativeToolCall`/`streamResponse`/
   `finishCycle`/`maxToolIterations` natively onto `ConversationCore`
   (`core_tools.go`, `core_respond.go`); their `o.gate`/`o.publishEv` call sites
   redirect to `c.approvals`/`c.events`. **Refined during implementation, not as
   originally scoped:** `availableToolSchemas` does not move wholesale —
   `toolSchemasFn` turned out to be permanent like `execTool`, both entangled with
   plan_task (see the behavior doc's Correction 1); only its generic-catalog logic
   moved, as Core's new native `toolSchemas()`. Two more corrections surfaced and
   were fixed in this phase, not deferred: Phase 2's `thinkingEnabled`/
   `thinkingPromptTemplate` fields were snapshotted values that would have silently
   stopped tracking live-reload config edits (`thinking.enabled`, `tools.enabled`,
   `tools.read_only`, `thinking.time_budget_seconds` are all live-reloadable without
   a restart) — fixed by making every Settings-derived value on Core a closure read
   fresh, never a snapshot (Correction 2); `EventSink` gained a second method,
   `PublishTool`, once moving `runNativeToolCall` showed the pre-extraction
   `publishToolCall`/`publishToolResult` carried a `ToolName` the generic `Publish`
   shape had no room for (Correction 3). Full regression pass, `-race`, and `make
   all` all clean. Behavior doc:
   `docs/architecture/behavior/adr/0013_conversationcore_tools_respond.feature.md`.
4. **Implemented (2026-08-01).** Move `withContext`/`workingMemoryMessage`/
   `historyMessages`/`recordTurn` into the `Orchestrator`-side `ContextStore`
   implementation (`core_context.go`, new). **Lower-risk than scoped, found by
   checking before moving:** the behavioral goal was already met by Phases 1-2
   (`Augment`/`Record` were already pure pass-throughs, already the only path
   `ConversationCore.RunPrompt` used since Phase 2) — this phase is a pure
   code-organization relocation, not a behavior change. Confirmed, not assumed,
   these five stay `Orchestrator` methods permanently: `o.mu`/`o.history`/`o.store`
   are named in §"What stays on `Orchestrator`, unconditionally" above, and giving
   `ConversationCore` direct access to them would be the exact anti-pattern this ADR
   exists to eliminate. A due-diligence pass over every `o.history` touch point
   (`recordTurn`, `historyMessages`, `ContextBreakdown`, `SetEventEnabled`,
   `classifier_pipeline.go`'s `historyEvents`) confirmed consistent `o.mu` locking
   throughout — no gap found, unlike Phase 3's live-reload catch. `continuation.go`/
   `classifier_pipeline.go`'s direct calls to `o.withContext`/`o.recordTurn` are
   unchanged (same treatment as Phase 3's `streamResponse`/`finishCycle`). Behavior
   doc: `docs/architecture/behavior/adr/0013_conversationcore_context_store.feature.md`.
5. **Implemented (2026-08-01).** Verify independent instantiability.
   `internal/runtime/core_standalone_test.go` (new):
   `TestConversationCoreRunsStandaloneEndToEnd` constructs a `ConversationCore`
   wired entirely to itself — `execTool`/`toolSchemasFn` bound to the same
   instance's own `runNativeToolCall`/`toolSchemas`, not an `Orchestrator`-bound
   closure — with a fake `ApprovalSeeker`/`EventSink`/`ContextStore`, real
   `tools.DefaultRegistry()`/`tools.NewPolicy()`, and a `stubModel`; drives a full
   turn (hooks → model call → approval-gated tool execution → second model call →
   final answer) with no `Orchestrator`, session store, or transport anywhere in
   the dependency graph. `TestTwoConversationCoresRunConcurrentlyWithoutInterference`
   goes further: two independently-constructed instances run concurrently
   (goroutines, `-race`), each ending with only its own recorded state — the
   concrete version of this ADR's founding motivation (a second, nested loop must
   not share mutable state with the first). Behavior doc:
   `docs/architecture/behavior/adr/0013_conversationcore_standalone.feature.md`.

Phase 5 was this ADR's actual finish line: a `ConversationCore` that provably runs
without an `Orchestrator` attached, alone or alongside a second instance. All 5
phases are now implemented. What consumes a second instance for real (a sub-session,
a planning tool) remains future work — see Open Questions below, unchanged by this
ADR's completion.

## Open Questions

1. **Nested-loop `ApprovalSeeker` policy.** Delegate to the same human via the parent's
   gate, or a stricter deny-by-default mirroring ADR 0008's read-only branch model?
   Explicitly unscoped — likely decided alongside whatever future ADR designs a
   concrete nested-loop consumer.
2. **Nested-loop `ContextStore` shape.** A `branch.Facts()`-style read-only snapshot in,
   local-only accumulation out (mirroring `SetLocalFact`/`LocalWM`), never merged back
   except through an explicit seal — plausible by analogy to `branch.Branch`, but not
   designed here.
3. **Processing-state reporting for a nested loop.** Generalize `setProcessing`/
   `state.RunState`/`Phase`, or report through the existing plan-visibility
   `Observer`/`NodeDispatched` channel instead? Unscoped.
4. **`plan_task`'s relationship to `ConversationCore`.** Does a future planning tool
   become a `ConversationCore` consumer, or stay structurally separate the way
   `decompose`/`wavefront` are today? Explicitly deferred — not this ADR's job.
