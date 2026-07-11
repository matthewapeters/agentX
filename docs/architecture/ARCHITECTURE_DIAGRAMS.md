# AgentX Architecture Diagrams

Diagrams reflect the current Go/bubbletea implementation on `bubbletea`.
For narrative context see `CLAUDE.md`, `docs/implementation/01_runtime_blueprint.md`,
and the ADRs under `docs/architecture/adr/`.

## 1. Client-Server / Surface Topology

```
┌───────────────────────────────────────────┐         HTTP/SSE          ┌────────────────────┐
│  agentx  (server + bundled chat surface)   │◄─────────────────────────►│  other surfaces     │
│                                             │                          │  (separate processes,│
│  internal/app        — composition/wiring  │                          │  launched via        │
│  internal/runtime    — Orchestrator        │                          │  `agentx surface     │
│  internal/transport/http — Provider/Server │                          │  launch <kind>`)      │
│  internal/state      — Bus, event model    │                          │                       │
│  internal/session    — identity, persistence│                         │  files · config ·     │
│                                             │                          │  context ·            │
│  internal/surfaces/chat  — output + input  │                          │  context-history ·    │
│    panels, bundled with the server process │                          │  context-visualizer · │
└───────────────────────────────────────────┘                          │  working-memory       │
                                                                          └────────────────────┘
```

The server holds canonical session/event state and exposes it over HTTP/SSE
(`internal/transport/http`). Each surface is a separate client process that
attaches with an ephemeral attach token; the surface registry
(`internal/surfaces/registry.go`) is open-ended — new surface kinds attach
without changing existing ones. Tool execution and approvals happen
server-side regardless of which surface is attached.

---

## 2. Prompt Cycle: Classify → Route → Respond

```
User prompt
     │
     ▼
Orchestrator.Submit (internal/runtime/orchestrator.go)
     │
     ▼
classify (internal/classify) ──► route: one of
     │
     ├─ respond_directly ──────────────────────────► streamResponse ──► agent_response
     │
     ├─ single_tool ───────► runToolPhase (tool_cycle.go)
     │                          │
     │                          ├─ tools.Proposer.Propose  (pick one tool call)
     │                          ├─ tools.Policy.Evaluate   (Allow | Deny | NeedsApproval)
     │                          ├─ RequestApproval, if needed (see §3)
     │                          ├─ tools.Executor.Run
     │                          └─ tool_call / tool_result events published
     │                          ▼
     │                     fold result into context ──► streamResponse ──► agent_response
     │
     └─ invoke_planner ────► runPlanPhase (plan_cycle.go)
                                │
                                ├─ decompose.LLMPlanner.Plan — DAG of ≤5 nodes
                                │   (task: a resolved tool call | step: coarse
                                │    sub-goal, decomposed further on dispatch)
                                ├─ scheduler drains the DAG, dependency-ordered,
                                │   executing ready leaves (task_node events
                                │   stream live as nodes dispatch/decompose/complete)
                                └─ findings fold into context
                                ▼
                          streamResponse (may run one bounded verb-continuation
                          round first — see maybeContinuePlan) ──► agent_response
```

`streamResponse` assembles the prompt (`internal/prompting`), calls the
configured `Model.Chat` (`internal/llm/ollama` in production), and — when
thinking is enabled — separates reasoning (`thinking` events) from the final
answer, which streams as transient `agent_delta` events and is published once,
complete, as `agent_response`.

---

## 3. Interactive Decisions: The Shared Approval Gate

```
Any routine needing a user decision
(tool-execution approval, verb-continuation approval, ...)
     │
     ▼
Orchestrator.RequestDecision (internal/runtime/decision.go)
     │
     ├─ enqueue onto the shared decisionGate (internal/runtime/gate.go) —
     │    a FIFO queue; concurrent requests of ANY kind serialize, one shown
     │    at a time
     ├─ if now at the front: publish an approval_request event
     │    {prompt, options: [{label, decision}, ...]}, set
     │    processing_state = awaiting_input
     │
     ▼
Chat surface (internal/surfaces/chat) swaps a THIRD panel in between output
and input — internal/surfaces/approval — bordered "AgentX Needs Your Input":
a navigable list of the options above (↑/↓ or j/k moves the cursor, Enter
confirms). Input stays visible but inert; the approval widget owns all keys
until resolved.
     │
     ▼
Bridge.Approve(decision string) ──► Orchestrator.Resolve ──► gate.deliver
     │
     ├─ the blocked RequestDecision call returns the decision string to its
     │    caller (RequestApproval / RequestVerbApproval), which applies its
     │    own kind-specific persistence (policy scope, verb allow/deny list)
     └─ an approval_decision audit event is published: {prompt, chosen_label,
          decision} — persisted like any event, but Enabled=false, so it is
          excluded from assembled LLM context. The chat surface renders it as
          a fixed-gray, one-line scrollback record.
```

The gate is generic (`gate[Req, Resp]`) — a third decision kind needs no new
UI, only a caller that builds `{prompt, options}` and calls
`RequestDecision`.

---

## 4. Tool Policy Evaluation

```
tools.Policy.Evaluate(descriptor, args)   (internal/tools/policy.go)
     │
     ├─ blacklist match?              ──► Deny (reason: blacklist / rule-specific)
     ├─ already approved (session or  ──► Allow
     │  global whitelist, keyed by
     │  tool id + canonical args)?
     ├─ descriptor.RequiresApproval?  ──► NeedsApproval  (→ RequestApproval, §3)
     └─ else                          ──► Allow

Read-only mode (agentx.toml [agentx.tools] read_only = true, default on):
  any non-read-risk tool is denied outright in the single_tool cycle,
  bypassing the approval gate entirely — a stricter, separate check in
  tool_cycle.go, ahead of policy evaluation.

Approval scopes:
  session — in-memory only, this session
  global  — persisted to ~/.config/agentx/agentx-tool-approvals.toml,
            survives restarts, keyed by tool id + canonical args
```

---

## 5. Session Event Persistence

```
Every published event (internal/state.Event) carries:
  epoch, session_id, event_type, content_type, payload, enabled, ordinal, ...

ContentType (internal/state/event.go) includes:
  user_prompt · system_prompt · classification · thinking · agent_delta*
  agent_response · attachments · tool_call · tool_result · processing_state
  task_proposed · task_result · task_diagnostic · task_plan · task_node
  approval_request · approval_decision

  * agent_delta is transient — never persisted, never sent to other surfaces;
    only the complete agent_response is durable.

DefaultEnabled(content_type) decides whether an event folds into assembled
LLM context by default:
  true  → user_prompt, agent_response, attachments
  false → everything else (persisted regardless, but excluded from context
          unless explicitly toggled on via the context surface)

internal/session.Store persists every non-agent_delta event as one JSON file
under sessions/<id>/events/, independent of its Enabled flag — persistence
and context-inclusion are orthogonal. internal/prompting/digest assembles
context by filtering on Enabled at build time.
```

---

## 6. Key Package Cross-Reference

```
cmd/agentx/                    — runtime entrypoint (boots server + chat surface)

internal/app/                  — composition: wires Orchestrator, Bridge, transport
internal/runtime/              — Orchestrator: classify/route, tool_cycle.go,
                                  plan_cycle.go, decision.go, gate.go,
                                  classifier_pipeline.go
internal/runtime/decompose/    — LLMPlanner: DAG decomposition
internal/runtime/scheduler/    — dependency-ordered DAG execution
internal/cli/                  — command-line parsing (`agentx`, `agentx surface
                                  launch <kind>`, `agentx session new-name`)

internal/transport/http/       — Provider interface + Server: HTTP/SSE endpoints
                                  external surfaces attach to
internal/surfaces/              — registry.go (open-ended surface kinds:
                                  files, config, context, context-history,
                                  context-visualizer, working-memory); chat/
                                  (output + input panels, bundled with the
                                  server); approval/ (the shared decision
                                  widget, a third panel chat/ swaps in);
                                  output/, input/ (chat's two panels); client/
                                  (shared Bubble Tea host every independently
                                  launched surface attaches through — seed →
                                  live-stream → resize → quit); context/,
                                  contextviz/, workmemory/ (dedicated surface
                                  implementations); files and config are
                                  simpler surfaces wired directly in
                                  internal/cli/surface_launch.go

internal/tools/                — Descriptor, Policy (blacklist/whitelist/
                                  approval), Executor, Proposer
internal/llm/ollama/           — Ollama streaming client, model listing,
                                  context-length lookup
internal/prompting/            — prompt assembly, digest (context filtering),
                                  classify, continuation (stated-intent
                                  detection), planner (DAG prompt/schema)

internal/session/               — session identity, append-only JSON event
                                  persistence, replay
internal/state/                 — Event/ContentType model, Bus (pub-sub),
                                  ProcessingState
internal/config/                — agentx.toml loading, seeded prompt/policy
                                  files under ~/.config/agentx/
```
