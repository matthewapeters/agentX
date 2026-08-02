# AgentX Sequence Diagrams: Prompt → Decomposition → Execution → Resolution

These diagrams trace what the code on `bubbletea` actually does, end to end, for a
single user turn — from the moment a prompt is submitted in the chat surface to the
moment the user sees a resolved answer. They complement the box-and-arrow diagrams in
`ARCHITECTURE_DIAGRAMS.md` (topology, policy, persistence) with the call-by-call
*sequence* of who talks to whom.

**Method.** Every arrow below was verified against the `.go` source (file:line noted
in each diagram's "Key call sites" table), not against design docs or ADRs describing
future work. Where a function exists but is not reachable from `cmd/agentx/main.go`,
it is called out explicitly rather than drawn as live. **Re-verified 2026-08-01**
against the native tool-calling loop shipped 2026-07-31 (commit `5283a766`) — Diagram
1 was rewritten, Diagrams 2/3 got entry-point/split notes, and Diagram 5 is now
flagged disconnected rather than merely gated-off; see each diagram's inline note.
Two more caveats surfaced earlier and are worth knowing before reading the diagrams:

- `internal/runtime/scheduler/node.go`'s `NewStep`/`NewTask` wrapper types are
  defined and unit-tested but **not used** by the scheduler's actual dispatch loop,
  which switches on `task.Record.Kind` directly (`scheduler.go:219`).
- `POST /prompt` (`internal/transport/http/write.go:57`) is fully wired
  server-side, but no shipped surface client calls it — the bundled chat surface
  reaches `Orchestrator.Submit` via a direct in-process closure instead
  (`internal/app/app.go:192`). The HTTP path only exists today for external
  surfaces/tests.

**Scope.** All diagrams assume the default launch: `agentx` boots the server and the
chat surface together, in one process (`internal/app/app.go:154 RunChat`). Diagram 4
shows where a second, separate surface process would plug in over HTTP/SSE instead.

**How to view.** Open this file in VS Code with a Mermaid preview extension (e.g.
"Markdown Preview Mermaid Support") and use the Markdown preview pane, or view it on
GitHub/GitLab, which render ```mermaid fences natively.

---

## Diagram 1 — Turn Overview: Native Tool-Calling Loop

> **Rewritten 2026-08-01.** This diagram originally showed a `classify → route →
> (single_tool | invoke_planner | respond_directly)` spine. Commit `5283a766`
> (2026-07-31, "Replace classify-routed prompt cycle with native tool-calling
> loop") replaced that with the flat loop below; `internal/classify`,
> `internal/runtime/continuation.go`, and the task-classifier pipeline
> (`internal/runtime/classifier_pipeline.go`) are disconnected from
> `Orchestrator.runPrompt` — unwired, not deleted (see
> `../implementation/90_open_questions.md`, D.5). Diagram 5 below documents that
> now-dead trigger for historical reference only.

This is the spine every prompt goes through: one loop, no upfront classify step.
Tool execution (native calls) and `plan_task` (multi-step investigation, the
model's own discretionary replacement for the old `invoke_planner` route) both
happen *inside* the loop, not as pre-classified branches.

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant Chat as Chat Surface<br/>(surfaces/chat)
    participant Orc as Orchestrator<br/>(runtime)
    participant Hooks as hooks.Registry<br/>(sync + async, empty today)
    participant LLM as Ollama/llama.cpp Model<br/>(native tool schemas advertised)
    participant Tool as runNativeToolCall<br/>/ plan_task [Diagram 2/3]
    participant Bus as Event Bus<br/>(state.Bus)

    User->>Chat: type prompt, submit
    Chat->>Orc: Submit(ctx, text)
    activate Orc
    Orc->>Bus: publish USER_PROMPT
    Orc->>Hooks: RunSync(turn); RunAsync(turn) — no-ops (empty registry)

    loop up to MaxToolIterationsPerTurn (default 25)
        Orc->>LLM: streamResponse(msgs, toolSchemas, think only on i==0)
        LLM-->>Orc: ChatResult{Content, ToolCalls}
        alt len(ToolCalls) == 0
            Note right of Orc: plain chat response — loop ends
        else ToolCalls present
            loop each call
                alt call.Name == "plan_task"
                    Orc->>Tool: runPlanTaskTool(ctx, goal)<br/>runs runPlanPhase/runWavefrontPhase to completion [Diagram 2]
                else any other tool
                    Orc->>Tool: runNativeToolCall(ctx, call) [Diagram 3]
                end
                Tool-->>Orc: result text (folded back as a tool-role message)
            end
            Orc->>Hooks: RunSync(turn); RunAsync(turn) — no-ops (empty registry)
            Note right of Orc: loop back to LLM with tool results appended
        end
    end

    Orc->>Bus: publish agent_delta* (streamed), then final agent_response
    Orc->>Orc: recordTurn(...); finishCycle(err)
    Orc->>Bus: publish PROCESSING_STATE (Completed | Failed)
    deactivate Orc
    Bus-->>Chat: event fan-out [Diagram 4]
    Chat-->>User: render streamed answer + tool/plan widgets
```

### Key call sites

| Step | File:line | Function |
|---|---|---|
| Submit | `internal/surfaces/chat/chat.go:467`, `internal/app/app.go:192` | `submitCmd` → `bridge.Submit` → `orc.Submit` (in-process) |
| Entry | `internal/runtime/orchestrator.go` | `Orchestrator.Submit` |
| Loop | `internal/runtime/loop.go:24` | `runPrompt` |
| Hooks | `internal/runtime/hooks/hooks.go` | `Registry.RunSync`, `Registry.RunAsync` |
| Model call | `internal/runtime/tool_cycle.go` (`streamResponse`) | native tool schemas advertised, `ChatResult{Content,ToolCalls}` returned |
| Tool-call dispatch | `internal/runtime/loop.go:112` | `runToolOrPlan` → `runNativeToolCall` or `runPlanTaskTool` |
| Iteration bound | `internal/runtime/loop.go:60` | `Settings.MaxToolIterationsPerTurn` (default 25) |
| Turn close | `internal/runtime/loop.go:103-104` | `recordTurn`, `finishCycle` |

---

## Diagram 2 — Plan Phase: Decomposition + Scheduler Drain

> **Entry point changed 2026-07-31**, mechanism unchanged: this used to run only
> when the classifier picked `invoke_planner`. It's now entered when the model
> calls the `plan_task` tool at its own discretion (Diagram 1) —
> `runPlanPhase`/`DrainPlan`/`Scheduler` below are reused as-is
> (`internal/runtime/plan_tool.go`).

This turns one goal into a dependency graph, recursively breaks down anything too
coarse to run directly, and executes the leaves. Tool leaf execution itself is
black-boxed to Diagram 3.

```mermaid
sequenceDiagram
    autonumber
    participant Orc as Orchestrator
    participant Drain as decompose.DrainPlan
    participant Sched as Scheduler
    participant Dec as Decomposer<br/>(LLMPlanner)
    participant LLM as Ollama Model
    participant Exec as Executor<br/>[Diagram 3]
    participant Bus as Event Bus

    Orc->>Drain: DrainPlan(root, decomposer, cappingExec,<br/>slots, maxDepth, observer)
    activate Drain
    Drain->>Drain: seed task.Graph with root node (Kind=Step)
    Drain->>Sched: New(graph, decomposer, executor, slots, maxDepth)
    Drain->>Sched: Run(ctx)
    activate Sched

    loop dispatch loop — until graph drained or stalled
        Sched->>Sched: dispatch every Ready() node,<br/>up to `slots` concurrent workers

        alt node.Kind == Step (too coarse to run directly)
            Sched->>Dec: Decompose(ctx, rec)
            activate Dec
            Dec->>Dec: fork isolated, read-restricted planning branch
            Dec->>LLM: LLMPlanner.Plan(goal, context)<br/>schema-constrained JSON, ≤5 child nodes
            LLM-->>Dec: candidate children
            opt a child goal echoes the parent's (SimilarGoals guard)
                Dec->>LLM: retry once, violation named in prompt
                LLM-->>Dec: revised children
            end
            Dec-->>Sched: branch.Result{children} OR ErrNoProgress
            deactivate Dec
            alt guard still failing after the one retry
                Sched->>Sched: demote Step → Task, execute as-is<br/>(anti-spiral fallback, no further recursion)
                Sched->>Exec: Execute(ctx, rec)
            else children returned
                Sched->>Sched: applyDecompose — add children to graph,<br/>parent becomes a join (Deps = children)
                Sched->>Bus: publish TASK_NODE "decomposed"
            end
        else node.Kind == Task (leaf, resolvable now)
            Sched->>Exec: Execute(ctx, rec)
            Exec-->>Sched: Outcome{Status, Result}
            Sched->>Sched: setStatus (Done|Failed|Denied);<br/>capturingExec records findings for synthesis
            Sched->>Bus: publish TASK_NODE "completed"
        end
    end

    Sched-->>Drain: PlanOutcome{nodes} (or ErrStalled / ctx.Err)
    deactivate Sched
    Drain-->>Orc: PlanOutcome, derr
    deactivate Drain

    Orc->>Orc: publishPlan — tally failed/denied/abstained/neverRan
    Orc->>Bus: publish TASK_PLAN "ended"
    Orc->>Orc: planContext(steps) → grounding text<br/>(empty ⇒ handled=false, caller answers ungrounded)
```

Depth is bounded (`decompose.DefaultMaxDepth = 3`): a `Step` that recurses past that
depth is not retried further — it degrades to an abstain/clarify outcome rather than
looping.

### Key call sites

| Step | File:line | Function |
|---|---|---|
| Entry | `internal/runtime/plan_cycle.go:183` | `runPlanPhase` |
| Drain | `internal/runtime/decompose/drain.go:34` | `DrainPlan` |
| Scheduler loop | `internal/runtime/scheduler/scheduler.go:138` | `Scheduler.Run` |
| Node dispatch | `internal/runtime/scheduler/scheduler.go:219` | `work` (switches on `rec.Kind`) |
| Decompose | `internal/runtime/decompose/decompose.go:59` | `Decomposer.Decompose` |
| LLM planning call | `internal/prompting/planner/planner.go` (via `decompose/live.go:37`) | `LLMPlanner.Plan` |
| Echo guard | `internal/runtime/decompose/guard.go:75` | `SimilarGoals` |
| Apply children | `internal/runtime/scheduler/scheduler.go:270` | `applyDecompose` |
| Execute leaf | `internal/runtime/scheduler/scheduler.go:255` | `execute` → `Executor.Execute` |
| Status mapping | `internal/runtime/scheduler/scheduler.go:304` | `setStatus` |
| Findings capture | `internal/runtime/plan_cycle.go:61` | `capturingExec.Execute` |
| Terminal summary | `internal/runtime/plan_cycle.go:225,232` | `publishPlan`, `planSummary` |

---

## Diagram 3 — Tool Execution Detail

> **Split 2026-07-31.** Before the native tool-calling loop, both a plan leaf and
> the classifier's `single_tool` route went through the same `Executor.Execute`,
> which fell back to an LLM-driven `Proposer` when a call wasn't already
> resolved. Now there are two distinct paths that only share the policy/approval
> primitives:
> - **Interactive native tool calls** (from Diagram 1's loop) go straight through
>   `runNativeToolCall` — no `Executor`, no `Proposer`, no post-run `Verify` step.
> - **Plan leaves** (from Diagram 2) still go through `internal/executor.Executor`,
>   including its post-run `FSVerifier.Verify` — but its `Proposer` fallback is now
>   wired to a `noProposer` stub (`internal/runtime/classifier_pipeline.go`) that
>   always returns "no tool"; live plan leaves always carry a pre-resolved call
>   from the decomposer, so that fallback path is dead in practice.

```mermaid
sequenceDiagram
    autonumber
    participant Loop as Orchestrator.runPrompt<br/>[Diagram 1]
    participant NTC as runNativeToolCall
    participant Sched as Scheduler leaf dispatch<br/>[Diagram 2]
    participant Exec as executor.Executor<br/>(plan leaves only)
    participant Pol as Policy
    participant Gate as Approval Gate<br/>(decision.go)
    participant Chat as Chat Surface
    actor User
    participant Run as tools.Runner.Run
    participant Ver as FSVerifier<br/>(plan leaves only)
    participant Bus as Event Bus

    par interactive native tool call
        Loop->>NTC: runNativeToolCall(ctx, call)
        activate NTC
        NTC->>Pol: Evaluate(descriptor, args)
    and plan leaf
        Sched->>Exec: Execute(ctx, rec)
        activate Exec
        Exec->>Exec: resolvedProposal(rec.Params)<br/>(always hits — Proposer fallback is a dead noProposer stub)
        Exec->>Pol: Evaluate(descriptor, args)
    end

    alt blacklist match
        Pol-->>NTC: Deny
    else already approved (session/global whitelist)
        Pol-->>NTC: Allow
    else descriptor.RequiresApproval OR path escapes working-dir root
        Pol-->>NTC: NeedsApproval
        NTC->>Gate: RequestDecision(...)
        activate Gate
        Gate->>Bus: publish approval_request;<br/>PROCESSING_STATE=awaiting_input
        Bus-->>Chat: approval_request event
        Chat->>User: swap in approval panel
        User->>Chat: pick option
        Chat->>Gate: Resolve(decision)
        Gate-->>NTC: decision
        deactivate Gate
        alt user denies
            NTC-->>Loop: tool-role message: denied
        end
    end

    NTC->>Run: Run(tool, args)
    Run-->>NTC: result (possibly Truncated)
    opt result truncated and a size-decision callback is wired
        NTC->>Gate: RequestDecision (accept preview / rerun wider / decline)
        Gate-->>NTC: decision
    end
    NTC->>Bus: publish tool_call / tool_result
    NTC-->>Loop: tool-role message: result text
    deactivate NTC

    Exec->>Ver: Verify(effect)
    alt verification fails
        Ver-->>Exec: Phantom
        Exec-->>Sched: Outcome{Failed/Phantom}
    else verified
        Ver-->>Exec: ok
        Exec-->>Sched: Outcome{Executed, Result}
    end
    deactivate Exec
```

### Key call sites

| Step | File:line | Function |
|---|---|---|
| Interactive entry | `internal/runtime/tool_cycle.go:101` | `runNativeToolCall` |
| Plan-leaf entry | `internal/executor/executor.go:247` | `Executor.Execute` |
| Dead Proposer fallback | `internal/runtime/classifier_pipeline.go:88` | `noProposer.Propose` (always returns "no tool") |
| Policy | `internal/tools/policy.go:236` | `Policy.Evaluate` |
| Confinement check | `internal/executor/executor.go:193` | `escapesRoot` |
| Approval | `internal/runtime/approval.go:45`, `decision.go:42` | `RequestApproval` → `RequestDecision` (FIFO gate) |
| Run tool | `internal/tools/executor.go:63` | `Runner.Run` |
| Verify (plan leaves only) | `internal/executor/verify.go:28` | `FSVerifier.Verify` |
| Status mapping (plan-leaf caller side) | `internal/runtime/scheduler/scheduler.go:257-264` | `Executed→Done`, `Denied/NeedsApproval→Denied`, else `Failed` |

---

## Diagram 4 — Event Delivery: Bus → Surfaces

How a published event actually reaches pixels. The bundled chat surface never goes
over HTTP; only independently launched surfaces do.

```mermaid
sequenceDiagram
    autonumber
    participant Orc as Orchestrator
    participant Bus as Event Bus (state.Bus)
    participant ChatSurf as Bundled Chat Surface
    participant HTTPServer as transport/http.Server
    participant ExtSurf as External Surface<br/>(e.g. context-visualizer, separate process)
    actor User

    Orc->>Bus: Publish(event) — stamps monotonic Ordinal

    par bundled chat surface (in-process, no HTTP hop)
        Bus->>ChatSurf: fan out on Subscribe() channel
        ChatSurf->>ChatSurf: listenEvents() → EventMsg →<br/>Model.Update → output.Apply(ev)
        alt ContentType == task_plan
            ChatSurf->>ChatSurf: applyPlanEvent — create/update/finalize plan widget
        else ContentType == task_node
            ChatSurf->>ChatSurf: applyNodeEvent — mutate node state, spinner
        else ContentType in {tool_call, tool_result} tagged with task_id
            ChatSurf->>ChatSurf: applyTaskToolEvent — nest under owning plan node
        else other content types
            ChatSurf->>ChatSurf: default rendering (agent_response, thinking, ...)
        end
        ChatSurf-->>User: render updated transcript
    and external surfaces (separate processes, optional, attach over HTTP/SSE)
        Bus->>HTTPServer: same event
        HTTPServer->>ExtSurf: SSE frame:<br/>"event: &lt;content_type&gt;\ndata: &lt;json&gt;\n\n"
        ExtSurf->>ExtSurf: Apply(ev) via SurfaceModel interface
    end
```

### Key call sites

| Step | File:line | Function |
|---|---|---|
| Publish | `internal/runtime/orchestrator.go:1069,1078` | `publish` → `publishEv` |
| Bus fan-out | `internal/state/bus.go:46` | `Bus.Publish` |
| Bundled subscribe | `internal/app/app.go:176,213` | `orc.Bus().Subscribe()` → `Bridge.Events` |
| Chat apply | `internal/surfaces/chat/chat.go:476`, `internal/surfaces/output/output.go:433` | `listenEvents`, `Model.Apply` |
| Plan/node rendering | `internal/surfaces/output/plan.go:69,123,610,228` | `applyPlanEvent`, `applyNodeEvent`, `applyTaskToolEvent`, `renderPlan` |
| External SSE endpoint | `internal/transport/http/server.go:178,246,274` | `handleEvents`, `historyThrough`, `writeEvent` |
| External client apply | `internal/surfaces/client/client.go:73,83,38` | `NewHost`, `Update`, `SurfaceModel.Apply` |

---

## Diagram 5 — Background Retroactive Decomposition (disconnected)

> **⚠️ Disconnected, not just "off by default" (2026-07-31).** This is the
> "second, independent task-classifier pipeline" commit `5283a766` disconnected
> from the main loop along with `internal/classify`. `Orchestrator.runPrompt`
> (`internal/runtime/loop.go`) never calls `maybeEmitTask` — this diagram is kept
> as a historical record, not a currently reachable (even if gated-off) code
> path. It may return as a hook (`internal/runtime/hooks`) — see
> `../implementation/90_open_questions.md`, D.5.

A second, entirely separate trigger for the same Plan Phase machinery, run *after*
any answer — including a plain `respond_directly` one — in case the response itself
narrated a multi-step action that should have been investigated. It was gated on an
operator-configured prompt corpus before being disconnected entirely.

```mermaid
sequenceDiagram
    autonumber
    participant Orc as Orchestrator
    participant TaskClf as Task Classifier<br/>(classifier_pipeline)
    participant Recon as reconcile.Reconcile
    participant Plan as Plan Phase<br/>(same DrainPlan as Diagram 2)
    participant Bus as Event Bus

    Orc->>TaskClf: maybeEmitTask(ctx, ..., resp)  — runs after every turn, any route
    alt taskPipeline == nil (no prompts.toml corpus configured — the default)
        TaskClf-->>Orc: no-op, returns immediately
    else corpus configured
        TaskClf->>TaskClf: Classify (triage+action fan-group vote)<br/>+ ClassifyResponse
        TaskClf->>Recon: Reconcile(votes)
        alt verdict == Decompose
            Recon-->>TaskClf: Decompose
            TaskClf->>Orc: go runDecomposition(ctx, rec, ex)  — detached goroutine
            Orc->>Plan: DrainPlan(root, ...)
            Plan-->>Orc: PlanOutcome
            Orc->>Orc: streamResponse — own follow-up synthesis
            Orc->>Orc: recordTurn / finishCycle — closes its own cycle
            Orc->>Bus: publish TASK_PLAN, agent_response, PROCESSING_STATE
        else verdict in {Confirm, Reify, Redispatch, Verify, None/Ask}
            Recon-->>TaskClf: other verdict
            Note right of TaskClf: surfaced for approval, drained directly<br/>without decomposing, or skipped — not this flow
        end
    end
```

### Key call sites

| Step | File:line | Function |
|---|---|---|
| Trigger | `internal/runtime/orchestrator.go:509` | `runPrompt` fallthrough → `maybeEmitTask` |
| Gate | `internal/runtime/classifier_pipeline.go:65-77,227-229` | `taskPipeline` built only if `Settings.PromptCorpus` set |
| Classify + reconcile | `internal/runtime/classifier_pipeline.go:218,261-292` | `maybeEmitTask`, `reconcile.Reconcile` branches |
| Background drain | `internal/runtime/classifier_pipeline.go:284,325,330` | `go runDecomposition` → `DrainPlan` |

---

## Cross-diagram decision-point summary

| Site (Diagram) | Condition | Outcome A | Outcome B |
|---|---|---|---|
| 1 | model response has tool calls | none → plain chat answer, loop ends | present → execute/`plan_task`, fold results back, loop |
| 1 | tool-iteration budget (`MaxToolIterationsPerTurn`) | reached → answer with whatever text is on hand | not reached → keep looping |
| 2 | node.Kind | `Step` → decompose | `Task` → execute |
| 2 | echo-guard after 1 retry | still failing → demote to Task, execute directly | children valid → recurse |
| 2 | `planContext` empty after drain | falls through to an *ungrounded* answer | grounded synthesis with findings |
| 3 | policy verdict | `Deny`/denied approval → denied result, tool never runs | `Allow` → runs |
| 3 | `Verify` after running (plan leaves only) | fails → `Phantom`, never reported as success | ok → `Executed` |
| 5 *(disconnected)* | `PromptCorpus` configured | no-op (default; also true always, now — see Diagram 5 note) | historical: classifier+reconcile could background-trigger Diagram 2 |

## Loop / iteration points, at a glance

1. Tool-call loop (Diagram 1) — bounded by `Settings.MaxToolIterationsPerTurn`
   (default 25); ends early as soon as a response has no tool calls.
2. Scheduler dispatch loop (Diagram 2) — runs until the DAG is drained or stalled;
   every node dispatches at most once, so it terminates structurally.
3. Decomposition echo-guard retry (Diagram 2) — exactly one retry, then gives up.
4. Recursive decomposition depth bound (Diagram 2) — capped at `DefaultMaxDepth = 3`.
5. Interactive decision gate wait (Diagram 3) — blocks on a per-request channel,
   FIFO-serialized against any other concurrent decision request.

Retired: classification retry (bounded `ClassificationRetries+1`), tool-proposal
retry, and the single-bounded continuation round all belonged to the disconnected
classify-routed cycle — see the note at the top of Diagram 1.
