# Behavior — ConversationCore Runs Standalone (ADR 0013 Phase 5)

Status: **Implemented** (2026-08-01). Realizes ADR 0013's Phased Build Plan step 5 —
this ADR's actual finish line: concrete proof `ConversationCore` is independently
instantiable and runnable with zero `Orchestrator` involvement.

## Why this phase is more than "another unit test"

Every test added in Phases 2-4 already constructs a bare `ConversationCore` with no
`Orchestrator` — that part of "independent instantiability" has been true since Phase
2. But each of those tests deliberately isolates one seam at a time:
`core_loop_test.go`'s `RunPrompt` tests stub `execTool` directly, bypassing
`runNativeToolCall` entirely; `core_tools_test.go`'s `runNativeToolCall` tests never
go through `RunPrompt` at all. None of them prove the thing Phase 5 is actually for:
that a *second*, self-contained `ConversationCore` — wired to its own native
`toolSchemas`/`runNativeToolCall` rather than an `Orchestrator`-bound closure — can
run a complete turn (hooks → model call → approval-gated tool execution → second
model call → final answer) start to finish.

That distinction matters concretely: `Orchestrator`'s real wiring
(`buildCore`, `core_loop.go`) binds `execTool` to `o.runToolOrPlan` (plan_task-aware)
and `toolSchemasFn` to `o.availableToolSchemas` (also plan_task-aware) — both
permanent closures, by design, because plan_task stays off `ConversationCore` (Open
Question 4). A future minimal consumer that has no use for plan_task should be able to
wire `execTool` directly to the *same instance's* `runNativeToolCall` and
`toolSchemasFn` to its own `toolSchemas` — cutting the Orchestrator-mediation layer
out entirely. Phase 5 proves that shape actually works, not just that the pieces
compile in isolation.

## Design

`TestConversationCoreRunsStandaloneEndToEnd` builds one `ConversationCore` with:

- `execTool` set to `c.runNativeToolCall` itself (not a stub, not an
  Orchestrator-bound closure) — the self-wiring a minimal future consumer would use.
- `toolSchemasFn` set to `c.toolSchemas` itself, likewise.
- A real `hooks.Registry` with one sync hook that records it ran, proving hooks fire
  in a standalone instance exactly as they do in the session path.
- `tools.DefaultRegistry()`/`tools.NewPolicy()` (real, unmodified) plus a
  `stubToolRunner`/`stubApprovalSeeker` for the actual command execution and human
  decision — the only two things a real standalone consumer couldn't supply as real
  implementations without a live approval UI and a real subprocess executor.
- A `stubModel` scripted to issue one `write_file` tool call, then answer with final
  text — exercising the full `RunPrompt` round-trip, not a single-shot chat response.

No package in the test imports `agentx/internal/session`, `agentx/internal/transport`,
or references `Orchestrator` anywhere — the absence is the proof, not just an
assertion in the test body.

`TestTwoConversationCoresRunConcurrentlyWithoutInterference` goes one step further:
constructs two separate `ConversationCore` instances (separate fakes throughout, no
shared pointers) and drives both concurrently via goroutines under `-race`, then
asserts each instance's recorded events/history reflect only its own run. This is the
concrete version of the ADR's founding motivation (ADR 0013 Context: *"the moment
anything needs a second, nested prompt/tool/hook loop running inside a tool call"*) —
proving no hidden shared mutable state exists between instances, not just that one
instance works alone.

```
GIVEN a ConversationCore wired entirely to itself (execTool = c.runNativeToolCall,
      toolSchemasFn = c.toolSchemas) with no Orchestrator anywhere in its
      dependency graph
WHEN  RunPrompt drives a turn that issues a tool call requiring approval, then
      answers with final text
THEN  the sync hook fires, the approval seeker is consulted, the tool runs, the
      final answer is returned, and events publish in the sequence USER_PROMPT,
      TOOL_RESULT, AGENT_RESPONSE — no TOOL_CALL, since runNativeToolCall only
      calls publishToolCall on the no-approval-needed path (an approval-gated
      call's audit trail is the approval request/decision exchange instead,
      which a stub ApprovalSeeker doesn't publish — this is the same behavior
      the pre-extraction code documented in toolPin's own doc comment, not a
      gap introduced here).

GIVEN two independently-constructed ConversationCore instances, each with its own
      fakes
WHEN  both run a turn concurrently (goroutines, -race enabled)
THEN  neither instance's recorded state reflects the other's run — no shared
      mutable state, no race.
```

## Tests

- `internal/runtime/core_standalone_test.go` (new):
  `TestConversationCoreRunsStandaloneEndToEnd`,
  `TestTwoConversationCoresRunConcurrentlyWithoutInterference`.
- Full existing suite, `-race` (this phase's second test specifically requires it),
  and `make all` pass unchanged.
