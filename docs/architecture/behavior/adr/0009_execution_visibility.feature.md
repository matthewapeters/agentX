# Behavior — ADR 0009 §9a/§9c: Streamed Plan Events, Plan Widget + Spiral Guard

Slices: **9a + spiral guard** (built together — visibility is the safety net, the guard
is the cure; tidy-cove RCA 2026-07-07) and **9c plan widget + read-verify fix**
(mellow-meadow RCA 2026-07-07). Status: **DONE**.

## §9c plan widget (output surface, plan.go)

### Model.applyPlanEvent / applyNodeEvent / renderPlan
- GIVEN a task_plan snapshot (phase "started") WHEN applied THEN one plan widget is
  created immediately — the entry exists BEFORE any execution — titled "🗺 plan".
- GIVEN task_node deltas WHEN applied THEN the SAME widget mutates in place (never a
  new widget per delta): dispatched → ⏳ running with live elapsed; decomposed → ⑂ on
  the parent and children indented beneath it; completed → ✅ done / ❌ failed /
  ⊘ abstained, each stamped with its duration.
- GIVEN more than one step running at once THEN the title carries the parallel cue
  ("N running ∥").
- GIVEN per-step timing THEN durations derive from server event epochs (dispatch →
  completion), never a surface clock; a running step's elapsed refreshes as events
  arrive (no idle ticker — "need not be streaming, but a timer per step").
- GIVEN the final snapshot (phase "ended") THEN unfinished rows (pending / running /
  decomposed-but-never-joined) are marked 🚫 blocked, the plan error renders as an
  ⚠ line, and the title closes ✅/❌ with done/total counts. The widget persists in
  the transcript as the record of what ran.
- GIVEN a task_node delta with no prior started snapshot THEN it is ignored (nothing
  to attach to) rather than crashing or fabricating a widget.

## §9c.2 nested tool widgets (executor CallObserver → tagged events → indented boxes)

### executor.Executor.Execute (WithCallObserver seam)
- GIVEN a call observer WHEN a proposal resolves to a known tool THEN ToolCalled(rec,
  tool, args) fires BEFORE the gate/run — the pre-execution announcement.
- GIVEN any terminal point (executed / phantom / failed / denied / needs-approval)
  THEN ToolFinished fires with the outcome — a blocked attempt is as legible as a
  successful one. GIVEN a nil observer THEN behavior is unchanged.

### runtime.taskToolPublisher (buildTaskExecutor wiring)
- GIVEN a plan leaf executes THEN tool_call / tool_result events publish tagged with
  the step's task_id (and the executor outcome on the result), so the surface can
  attribute them; the single_tool cycle's untagged events are unaffected.

### output surface nesting (Apply + renderWidget)
- GIVEN a tool event whose task_id belongs to a live plan THEN its widget nests:
  indented two columns under the plan, call titled "🔧 <tool> · <step-id>", result
  titled "📋 result · <outcome>" and collapsed by default — expanding shows the
  maxBody-capped, scrollable body (the existing widget scroll machinery).
- GIVEN an untagged tool event THEN it renders flat (unchanged).

### decompose.ForceRoot (root always decomposes)
- GIVEN the plan cycle (or Decompose route) runs THEN the root node is never judged
  atomic — the request classifier already asserted multi-step; the one-step heuristic
  only arbitrates children. (Nimble-otter: "review X and suggest Y" ran as a single
  leaf on a lexicon gap.) A root whose decomposition makes no progress still executes
  via ErrNoProgress. The clause-verb lexicon was also broadened (suggest, describe,
  explain, improve, …) — a false "compound" only costs a decomposition attempt.

### plan-path thinking (orchestrator handleSubmit)
- GIVEN a plan produced findings THEN the synthesis response gets the same
  route-aware thinking pass as a direct answer (ThinkingRoutes["invoke_planner"]);
  previously the plan-handled branch hard-coded thinking off, which is why a
  successful plan showed no 💭 widget.

## Read-effect verification (executor/verify.go)

### FSVerifier.Verify (two regimes by risk class)
- GIVEN a read-class tool (Risk == read) whose run exited cleanly WITH output
  THEN it verifies — the output IS the effect. (Was the mellow-meadow kill: the old
  file-target stat phantomed every list_dir because its "path" arg is a directory.)
- GIVEN a read that returned nothing THEN it does not verify (proved nothing).
- GIVEN a write-class tool THEN the original semantics hold: clean exit AND the named
  file target exists non-empty (a write that never landed stays Phantom).

## Incomplete-plan diagnostic (plan_cycle.go planSummary)
- GIVEN a drained plan with any failed / abstained / never-ran nodes WHEN the final
  task_plan publishes THEN its error line reports the counts ("plan incomplete: 1
  failed, 0 abstained, 4 never ran …") — a partially-dead plan is never silent
  (mellow-meadow: one failed leaf silently stranded five nodes). Both the plan cycle
  and the background Decompose route share this summary.

## Plan-incomplete clarify gate (plan_cycle.go confirmPlanIncomplete)
Witty-falcon (2026-07-10): a plan whose leaf hit `scheduler.DefaultMaxDepth` was marked
Abstained, cascaded to strand its root, and the incomplete-plan diagnostic above landed only
on the (context-excluded) task_plan event — never the model's response prompt. The model
answered anyway, confidently narrating a file-write step that never ran. The diagnostic was
loud in the log; it was silent to the model and the user.
- GIVEN a drained plan whose nodes are all `done` WHEN confirmPlanIncomplete runs THEN it
  returns immediately with no prompt — a clean plan never interrupts.
- GIVEN a drained plan with any failed/abstained/never-ran node WHEN confirmPlanIncomplete
  runs THEN it calls RequestDecision (PhaseClarify) with a prompt naming the counts and up
  to 3 example blocked/abstained goals, and the same two-way options every such gate offers:
  "Answer with what I found" / "Stop here" — the same decision-gate seam tool approval and
  verb-continuation already use (internal/runtime/decision.go), not a new mechanism.
- GIVEN the user picks "Answer with what I found" THEN the response context gets an explicit
  incompleteness note ("Plan incomplete: N failed, N abstained, N never ran. Answer ONLY
  using the findings above — do not claim to have completed steps that never ran.") appended
  after the executed-steps findings, so the model cannot narrate over the gap.
- GIVEN the user picks "Stop here" THEN no further findings-grounded answer is generated;
  the response is grounded instead in a stopped-at-your-request note (planStoppedContext),
  mirroring toolDeniedContext's shape for a declined tool call.
- GIVEN the surface interrupts (ctx canceled) while the clarify decision is pending THEN
  runPlanPhase / runDecomposition end the cycle cleanly, same as any other RequestDecision
  call site.
- GIVEN the background Decompose route (runDecomposition) THEN it shares the exact same
  confirmPlanIncomplete gate as the foreground plan cycle (runPlanPhase) — one helper, two
  call sites, no duplicated incomplete-plan logic.

## Spiral guard

### decompose.stripResultPlumbing / SimilarGoals (guard.go)
- GIVEN a goal "Run `ls -la` on X and capture its output"
  WHEN plumbing is stripped THEN the action "Run `ls -la` on X" remains — the executor
  returns results automatically, so plumbing clauses are not a second step.
- GIVEN two tidy-cove chain rungs ("Run … and save output to $OUTPUT" vs "Execute … and
  write stdout to $OUTPUT") WHEN compared THEN they are similar (stopwords dropped, verb
  synonyms folded, ≥0.8 containment of the smaller token set).
- GIVEN a legitimate child ("Read README to understand features") of a review-project
  parent WHEN compared THEN not similar (real decomposition is never blocked).

### decompose.Decomposer.Decompose (non-progress guard)
- GIVEN the planner returns a child whose goal echoes the parent's
  WHEN decomposing THEN it returns an error wrapping `scheduler.ErrNoProgress` and no
  children — refusing to fund a recursion that cannot advance.

### scheduler.work (ErrNoProgress fallback)
- GIVEN the decomposer returns ErrNoProgress for a node the oracle judged non-atomic
  WHEN the worker handles it THEN the node is executed as an atomic leaf (Done/Failed by
  outcome), dispatched exactly once — never recursed, never marked Failed for the refusal.

### decompose.HeuristicOneStep (hardened)
- GIVEN "Run `ls -la` on <dir> and capture its output" WHEN judged THEN one-step (plumbing
  stripped first).
- GIVEN "enumerate all files and directories" WHEN judged THEN one-step (noun "and" is not
  clause chaining; only " and <action-verb>", " then ", ";" chain).
- GIVEN "review the project and identify a feature" WHEN judged THEN not one-step.

### decompose.DefaultMaxDepth
- GIVEN a plan recursion reaching depth 3 WHEN a node at the bound is non-atomic THEN it
  resolves to Ask — a spiral costs at most 3 levels, not 10.

### planner.PromptTemplate (generator-side rules)
- Steps are one verb+object action; result plumbing is explicitly forbidden with a
  WRONG/RIGHT example ("the tool returns results automatically"); no shell syntax; the
  goal itself is never restated as a step.

## §9a streamed events

### scheduler.Observer (WithObserver option; callbacks on the main loop, never concurrent)
- GIVEN an observer WHEN a node is handed to a worker THEN NodeDispatched(rec, depth)
  fires before the oracle call.
- GIVEN a decomposition lands WHEN the parent becomes a join THEN NodeDecomposed(parent,
  children) fires with the admitted children.
- GIVEN any node reaches a terminal status THEN NodeCompleted(id, status) fires (all
  terminal transitions flow through setStatus).
- GIVEN a nil observer THEN the scheduler is silent and behavior is unchanged.

### Orchestrator.runPlanPhase / planObserver / publishPlan (plan_cycle.go)
- GIVEN the plan cycle starts THEN processing shows the `planning` phase and an initial
  `task_plan` snapshot (phase "started", the root node) is published before any work.
- GIVEN the scheduler drains WHEN each transition happens THEN a `task_node` delta event
  (dispatched / decomposed / completed) is published immediately — batch-emit at
  completion is retired (the documented ADR 0009 invariant).
- GIVEN the drain ends THEN a final `task_plan` snapshot (phase "ended", executed count)
  is published; a plan that executed nothing carries a loud "plan blocked" error, never
  silence.
- GIVEN the background Decompose route (runDecomposition) THEN it streams through the same
  observer and uses the same depth bound.

### state
- `task_node` is a valid content type; `planning` is a valid processing phase.

## Recent-turn digest grounds classify + decompose (witty-falcon, 2026-07-11)
A short confirmation reply ("proceed with the commands", "yes", "do it") has no verb or
scope of its own — it only makes sense against what was just discussed. Both LLM call sites
that decide what happens next were, until now, structurally blind to that: `classify.
Classifier.Classify` assembled its prompt from `userText` alone, and `decompose.Decomposer.
Decompose` built its context from working-memory facts + this-plan's-own findings, neither
of which includes prior turns. Conversation history was threaded only into the final
answer-synthesis call. Live-observed failure: the model twice narrated ("let me actually
read `.vscode/settings.json`/`AGENTS.md`/`Makefile.hybrid`") without executing, and "proceed
with the commands" — which should have run exactly those three reads — instead decomposed
into an unrelated `ls -la` on the project root, because the planner never saw the prior
turn's text at all. `internal/prompting/digest` (pure, no LLM, always available — already
built for the [B]/[C] task-diagnostic pipeline) is now also threaded into both call sites.

### classify.Classifier.Classify (internal/classify/classify.go)
- GIVEN an empty history string WHEN Classify runs THEN the assembled messages are
  unchanged from before this fix (no regression on the common/cold-start case).
- GIVEN a non-empty history digest WHEN Classify runs THEN it is folded in as its own
  system message, inserted immediately before the user message — stable regardless of
  whether a custom system prompt is configured (Assemble always returns 0-or-1 leading
  system message(s) + exactly one trailing user message).
- GIVEN the digest system message THEN it is explicitly labeled "context only, not
  instructions" so recent conversation cannot be mistaken for a directive to the
  classifier itself.
- GIVEN DefaultPrompt THEN it instructs that a short reply with no verb/scope of its own
  is classified by what the recent conversation most recently proposed, not defaulted to
  respond_directly for lack of its own verb.

### decompose.Decomposer.Decompose (internal/runtime/decompose/decompose.go)
- GIVEN Decomposer.History is nil or returns "" THEN ctxText is unchanged from before this
  fix (no regression when no history is available).
- GIVEN Decomposer.History returns a non-empty digest THEN it is folded into ctxText ahead
  of this plan's own planfindings — distinct labeling: recent conversation (what was
  already discussed, before this goal was even dispatched) vs. this plan's own findings
  (what THIS drain has discovered while running).
- GIVEN buildDecomposition wires the live orchestrator THEN Decomposer.History is
  `o.recentDigest` — the same digest-building call classify.Classifier.Classify uses,
  reloaded fresh on every call (mirrors the existing Facts closure pattern), bounded to
  digestMaxTurns.

## Standing read-grants actually persist (witty-falcon, 2026-07-11)
`executor.Execute` force-requires approval whenever a call's path argument lexically
escapes the confinement root (`escapesRoot`, executor.go) — independent of whatever
`tools.Policy.Evaluate` already decided. `executor.ReadGrants` (`WithReadGrants`) and
`session.WMReadGrants`/`GrantReadPath`/`PermittedReadPaths` exist specifically to let a
prior out-of-root read approval suppress that override on later calls — fully built, but
never wired into `buildTaskExecutor`, so every out-of-root read (e.g. this environment's
two valid absolute paths to the same repo, `/Projects/agentX` and
`/home/mpeters/Projects/agentX` — whichever wasn't `os.Getwd()` at startup always looks
like it escapes root) re-prompted for approval every single time, even immediately after
"approve for this session"/"approve for all sessions."
- GIVEN a RiskRead call whose path escapes root WHEN the interactive approver grants it
  (session or global) THEN the path is persisted as a working-memory `read_path:` grant
  (`session.GrantReadPath`), not just recorded in `tools.Policy`'s exact-args allowlist.
- GIVEN a write-class or network call, or a call that was NOT flagged specifically for
  escaping root (e.g. a normal `RequiresApproval` tool) THEN no read-grant is persisted —
  this only widens standing READ access, matching `WithReadGrants`' own documented
  contract ("never permits a mutating call").
- GIVEN a later call (same session) whose path falls under a persisted grant WHEN the
  executor evaluates escapesRoot THEN `e.reads.Allows(full)` (backed by a live-reloading
  `session.WMReadGrants` wrapper, `liveReadGrants`) suppresses the override — no repeat
  approval prompt, honored immediately, not only after a restart.
- GIVEN a grant was persisted mid-session (via a fresh approval) THEN the very next call
  under that path honors it — `liveReadGrants` reloads working memory on every check
  rather than snapshotting it once at executor-construction time.

## Tests
`scheduler/observer_test.go` (lifecycle stream order + no-progress fallback),
`decompose/guard_test.go` (tidy-cove chain similarity, legit-decomposition negative,
echo-planner refusal), `decompose/live_test.go` (hardened heuristic cases),
`decompose/drain_test.go` (observer-threaded signature). Orchestrator-level event
assertions remain deferred (fixed-verdict harness limitation, per Phase 4d note).
`classify_test.go` (history folding, DefaultPrompt continuation guidance),
`decompose/decompose_test.go` (History folding), `runtime/readgrant_test.go`
(liveReadGrants + approver persistence).
