# ADR 0009 — Plan & Tool Execution Visibility and Control

Status: **Draft** (2026-07-07)
Depends on: ADR 0008 (recursive task decomposition + DAG scheduler)

## Context

Session `tidy-cove` (2026-07-07) ran a 123-second decomposition spiral — ten recursive
re-plannings of `ls -la` — during which **zero events were emitted**. The plan cycle
batch-published 15 events only after `DrainPlan` completed. The user, watching a silent
surface, had no way to notice the runaway pattern or abort it. The silence withheld the
kill switch.

This surfaced a previously undocumented **hard requirement**: *all tool execution must be
presented to the user in the Output surface* — before, during, and after it happens. The
user is a supervisor of execution, not a recipient of results.

## Decision

Execution becomes **observable, interruptible, and durable** end to end:

1. **Pre-execution callback** — every tool call (plan leaf, tool cycle, redispatch) is
   announced to the Output surface in a tools widget *before* it runs.
2. **Approval** — the existing approval gate (policy → `RequestApproval`) surfaces
   per-call in that widget; the Phase-2 out-of-cwd read grant (No / Once / Session) rides
   the same affordance.
3. **Abort** — the user can cancel a running plan; the scheduler already honors ctx
   cancellation, so abort = cancel the plan's context. Partial results are kept and
   reported.
4. **Results shown first** — a tool's result renders to the user (collapsed sub-widget,
   expandable) *before/independent of* what is folded into the agent's prompt.
5. **Structure cues** — the widget shows when a task decomposes (children indent under
   the parent) and when tasks run in parallel (multiple concurrent spinners).
6. **Status cues** — ❌ on failure, ✅ on success, spinner while running, ⊘ on
   blocked/abstained.
7. **Plan persistence** — the entire plan (DAG, statuses, tool calls, result refs) is
   persisted in the session as a standalone JSON document for later access, in addition
   to the append-only event stream.
8. **Context surface representation** — the plan and folded tool findings appear in the
   Context surface as manageable context items (the findings *are* prompt context).

### Architecture

**Observer seam (runtime).** `scheduler.Scheduler` gains an injected `Observer`
(nil ⇒ no-op) with lifecycle callbacks: `PlanStarted`, `NodeDispatched`,
`NodeDecomposed(parent, children)`, `NodeCompleted(id, status)`, `PlanEnded(outcome)`.
The scheduler stays LLM-free and bus-free; the observer is the seam.

**Event fan-out (orchestrator).** `runPlanPhase` wires an observer that publishes bus
events as the drain proceeds — `task_plan` on first decomposition, `task_node` updates
per lifecycle change, `tool_call`/`tool_result` around each leaf execution — replacing
the batch emit at completion. A new `planning` processing phase keeps the surface alive.

**Widget (output surface).** A plan widget renders the DAG as an indented tree with
per-node status glyphs, concurrent spinners for parallel nodes, and collapsed result
sub-widgets. Bubbles components; vertical scroll only (innerW−1 for the gutter).

**Abort (transport).** The existing stop affordance cancels the plan context when a plan
is active; the scheduler unwinds, in-flight leaves finish or cancel, statuses persist.

**Persistence.** On every plan mutation the orchestrator materializes
`sessions/<id>/plans/<root-id>.json` — final source of truth for "what ran, in what
order, with what result refs". The event stream remains the audit log; the plan file is
the queryable snapshot.

## Leans (decide before build)

1. **Event shape**: one `task_node` content type carrying `{id, status, phase}` deltas
   (lean) vs. re-emitting whole-plan `task_plan` snapshots per change. Lean: deltas +
   one initial snapshot; surfaces fold.
2. **Approval default for plan leaves**: read-only leaves auto-approved by policy
   (current tool-cycle behavior), mutating leaves always prompt. Lean: inherit the
   existing policy verdicts unchanged — the widget only *shows* what policy decides,
   plus interactive prompts where policy says `NeedsApproval`.
3. **Abort scope**: abort kills the whole plan (lean) vs. per-node abort. Per-node is a
   later refinement; the widget's first job is the emergency brake.
4. **Plan file write cadence**: rewrite-on-every-mutation (lean; file is always current,
   plans are small) vs. write-once at end (loses the crash/abort story).
5. **Context surface granularity**: one context item per plan (collapsed, with the
   folded findings as its body) (lean) vs. one item per leaf result. Per-leaf floods the
   pane on wide plans.
6. **Widget lifetime**: the plan widget persists in the transcript after completion
   (lean — it *is* the record of what ran) vs. collapsing to a one-line summary.

## Phases

- **9a — observer seam + streamed events**: scheduler `Observer`, orchestrator fan-out,
  `planning` phase, delta events. (Also fixes tidy-cove defect #2.) **Landed.**
- **9b — plan persistence**: `plans/<root-id>.json` materialization + rewrite cadence.
  **Landed (2026-07-08)**, see amendment below — schema is resumability-friendly but
  write-only by design.
- **9c — output plan widget**: tree, spinners, ✅/❌/⊘, collapsed results,
  pre-execution announcement. **Landed (2026-07-08)** as a recursive nested-DAG
  rendering, substantially beyond the original indented-tree scope — see amendment.
- **9d — approval + abort wiring**: per-leaf approval surfacing (incl. read grants),
  stop-cancels-plan. Not yet built (the concurrent-approval-queue fix, vivid-raven, was
  a necessary precursor, already landed separately).
- **9e — context surface representation**: plan as context item, findings manageable.
  Not yet built. **Designed in ADR 0010**: a plan's rolled-up status becomes a curated,
  bounded `session.Fact` (`Key: "plan:<name>"`), not the raw plan tree — the mechanism
  that also closes ADR 0008 OQ2/OQ6.

## Consequences

- The tidy-cove failure class becomes user-visible within ~1 dispatch (seconds), and
  user-abortable.
- Batch-emit-at-completion is retired; any future execution path must emit through the
  observer seam (this is now the documented invariant).
- The plan JSON gives replay/traceability (ADR 0005) a concrete artifact.

## Related defects (from tidy-cove RCA, fixed alongside but not part of this ADR)

- Decomposer non-progress guard (single child ≈ parent ⇒ atomic) — prevents the spiral
  itself; this ADR makes any residual spiral visible and abortable.
- Tighter `DefaultMaxDepth`; loud diagnostic when a plan ends fully blocked.

## Amendment (2026-07-08): §9c redesign — nested-DAG rendering, §9b persistence landed

**Phases 9b and 9c are now built**, in a substantially richer form than originally
scoped: not an indented flat tree, but a recursive nested-box DAG — separate boxes per
node, color-differentiated by Kind (Step blue, Task tan, running amber overriding
both), a Task's resolved command always visible in reverse video even collapsed, an
independent per-node spinner (not the lockstep single spinner §9c's original text
implied), and a liveness-propagating auto-collapse rule *while a plan is running* (a
node's children group is visible exactly while it or any descendant is running —
collapses uniformly once fully quiet, including mid-plan between bursts of activity).
Once the plan has ended, liveness stops gating anything and the full structure always
shows — see the brave-fjord-2 correction below; a real fast tool call completes far
under one terminal frame, so gating past "ended" meant the live window was in practice
never observed at all.

**Tree, not general graph.** The scheduler's decomposition is strictly parent-as-join
— a Step's children are always private to it, no cross-branch dependency edges are
structurally possible. A recursive nested-box *tree* captures the real DAG shape
exactly; "clear expression of parallelism and dependencies" is satisfied by
containment (a Step's children ARE its nested boxes) plus a short "waits on: …"
annotation for sibling ordering, not literal graph-edge rendering — which would be
solving a problem this system's DAG shape doesn't actually have.

**9c.2's separate nested tool-call/tool-result widgets are retired**, superseded by
folding a `task_id`-tagged `tool_call`/`tool_result` event directly into the owning
`planNode`'s own `command`/`resultText` fields — command renders inside the Task's own
box regardless of collapse state; the result is what collapsing hides. Untagged
(single-tool-cycle) events are unaffected.

**9b's plan-JSON schema is deliberately resumability-friendly** (durable node states,
result refs, full parent/child tree + sibling-deps distinct from scheduler `Deps`)
**without building any reconstruction logic** — the user's own forward-looking
observation ("agentX currently lacks a session resumption — but it could have one")
is a named, explicitly deferred follow-on, not built in this pass. `internal/session/plans.go`
is write-only by design (no `Read`/reconstruction API) to keep that boundary honest.

**A real, non-obvious bug found and fixed along the way**: `internal/surfaces/output`'s
`anySlice` helper only matched JSON-unmarshaled `[]any` payloads. The bundled chat
surface's event delivery never crosses a JSON boundary (a direct in-process channel of
`state.Event`), so the server's native `[]map[string]any`/`[]string` payload values
(`plan_cycle.go` builds them as literals) silently failed the type assertion —
`task_plan`'s bulk node list and `task_node`'s decomposed-children list were empty in
the live surface the entire time ADR 0009 §9a/9c-v1 was live, masked because
individual `dispatched`/`completed` deltas (which never touch `anySlice`) kept
populating nodes one at a time regardless. Fixed by making `anySlice` tolerant of any
concrete slice type via `reflect`, with a regression test reproducing the exact native
payload shape.

See `internal/surfaces/output/plan.go` (`drawNode`/`drawNodeContent`/`computeLiveness`)
and `internal/session/plans.go`/`internal/runtime/plan_tree.go` for the implementation.

### Correction (2026-07-08, same day): liveness must not gate a finished plan

Live usage (session `brave-fjord-2`) surfaced that the auto-collapse rule as first
implemented gated a plan's structure by liveness *unconditionally*, including after
the plan had fully ended. Both of that session's plans ran their leaf tool calls
(`ls -la`, `tree`) in ~4ms — dispatch and completion landed in the same millisecond
epoch — so the "live, expanded" window was never actually rendered by any terminal
frame; by the time a redraw happened, the step had already finished and the group had
already collapsed. With no manual per-node expand (deliberately out of scope — see
above), the user had no way to ever see the individual steps, only the outer title's
counts. Fixed: `renderPlanWidget` now treats every node as live once `ps.ended` is
true, unconditionally showing the full final structure — liveness only ever bounded
clutter *while things were still happening*, and that concern doesn't exist once a
plan is over. `TestEndedPlanShowsFullStructure` reproduces the exact same-epoch
dispatch→completion timing.
