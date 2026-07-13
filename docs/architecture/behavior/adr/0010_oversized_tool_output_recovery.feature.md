# Behavior — Oversized Tool Output Recovery (post-`output_max_bytes` decision point)

Status: **PROPOSED — scoped, not yet built**. Follow-up to the `nimble-pebble-2` RCA fix
(0009-adjacent; see `CHANGELOG.md` "Fixed" entry dated 2026-07-12 and
`internal/tools/executor.go`). That fix made truncation honest (labeled, never silent)
but still just *proceeds* on a partial result. This spec covers what happens instead of
silently proceeding.

## Problem

`tools.Executor.Run` and `runBuiltin` (`internal/tools/executor.go:83-149`) cap captured
output at `maxBytes` (`output_max_bytes`, default 65536) as a memory/runaway-process
safety net. When that cap triggers, `Result.Truncated = true` and `Result.Preview` gets an
honest trailer noting the cut (`internal/tools/executor.go:109-113`). Today that is the
end of the story: `runToolPhase` (`internal/runtime/tool_cycle.go:80-122`) publishes the
labeled-but-partial result and the turn continues. Two things are missing per user
direction (2026-07-12 design discussion): a way to **ask a human** whether to accept the
truncated result or capture more, and a way to **ask the agent** to retry with a
narrower, self-authored command (`rg` over `cat`, `-maxdepth`, a scoped path) instead of
just eating a partial result.

## Design summary — reuse the existing decision-gate, not the pre-execution policy gate

`tools.Policy.Evaluate` (`internal/tools/policy.go:236-260`) is a **pre-execution** gate —
it decides `Allow`/`Deny`/`NeedsApproval` before `runner.Run` is ever called. Truncation is
only known **after** `runner.Run` returns, so this is a structurally different concern and
must not be bolted onto `tools.Verdict`. Instead it reuses `Orchestrator.RequestDecision`
(`internal/runtime/decision.go:42-63`) — the same generic gate/`awaiting_input` machinery
already used by both the tool-approval round-trip (`RequestApproval`,
`internal/runtime/approval.go:45-64`) and the continuation-verb round-trip
(`RequestVerbApproval`, `internal/runtime/continuation.go:48-65`). Both existing call sites
prove the shape: a `state.Phase`, a fixed option set, and (for continuation verbs) an
"always" decision persisted to a small config file loaded up front and consulted before
ever prompting again (`continuation.LoadVerbs`/`AppendVerb`,
`internal/prompting/continuation/continuation.go:103-145`). This spec follows that second
pattern closely, since "always accept truncation from tool X" / "always give tool X more
room" is the same shape as "always allow this verb to continue."

## New surface

### `state.PhaseOutputSize` (new `state.Phase` value)
- GIVEN a tool result comes back `Truncated` WHEN no remembered decision exists for that
  tool ID THEN processing enters `StateAwaitingInput` / `PhaseOutputSize` — visible to the
  user exactly like a tool-approval or verb-approval pause (never a silent internal
  branch); this satisfies the existing tool-execution-visibility invariant
  (`docs/…` / CLAUDE.md: all tool execution is user-visible, pre-exec widget through
  ✅/❌ cues).

### Decision options (`outputSizeOptions`, mirrors `toolApprovalOptions`/`verbApprovalOptions`)
- `use_truncated_once` — accept the labeled, partial result for this call only (today's
  only behavior).
- `use_truncated_always` — same, and remember it for this tool ID (persisted; future
  truncations from the same tool skip the prompt).
- `expand_once` — re-run the exact same call once more with a larger capture cap, for
  this invocation only.
- `expand_always` — same, and persist a per-tool cap override.
- `refine` — do not re-run the same command; hand the model the failure context (tool ID,
  args, bytes captured, bytes over) and let it propose a *different*, presumably narrower,
  tool call instead (Phase B, below). Offered only on the *first* truncation for a given
  call chain — see the refine-loop guard.
- `abort` — treat like a policy denial: publish a `denied`-shaped result
  (`toolDeniedContext`-style) and let the turn continue without this tool's output.

### Persisted per-tool override (new config surface)
- New file `agentx-tool-output-overrides.toml` under `~/.config/agentx/` (sibling of
  `agentx-tool-approvals.toml`), TOML list keyed by tool ID:
  `{ tool = "read_file", decision = "expand", cap_bytes = 262144 }`. Structured (not a flat
  verb-per-line file like the continuation lists) because it carries a numeric cap, not
  just a boolean.
- GIVEN a persisted `use_truncated_always`/`expand_always` entry for a tool ID WHEN that
  tool truncates again THEN the decision applies **without re-prompting** — but the
  applied override is still stated in the published result text (e.g. "(auto-expanded to
  262144 bytes — remembered preference for `read_file`, set 2026-07-12)"), never silently
  swapped. Visibility survives even when the human isn't re-asked.

### Hard absolute ceiling (safety-net-for-the-safety-net)
- GIVEN `expand_once`/`expand_always` (interactively chosen or remembered) THEN the
  effective cap is `min(requested_cap, ToolOutputAbsoluteMaxBytes)` — a new config value,
  default suggested 2 MiB — so no interactive choice or remembered preference can produce
  an unbounded capture. This must be enforced at the point the ad-hoc higher-cap re-run is
  constructed, not left to the requester's judgment.

## Sequencing (in `runToolPhase`, after `runner.Run` returns)

1. `res, err := o.runner.Run(ctx, d, prop.Args)` (unchanged).
2. GIVEN `!res.Truncated` THEN publish and continue exactly as today — no behavior change
   for the common case.
3. GIVEN `res.Truncated` AND a persisted override exists for `d.ID` THEN apply it directly
   (re-run with the (ceiling-clamped) expanded cap, or accept truncated) and publish with
   the "remembered preference" note from above — no prompt.
4. GIVEN `res.Truncated` AND no override exists THEN call
   `RequestDecision(ctx, PhaseOutputSize, prompt, outputSizeOptions)` — prompt text
   includes tool ID, args, bytes captured vs. cap.
5. Apply the resolved decision:
   - `use_truncated_once` / `use_truncated_always` → publish as today; on `_always`,
     persist the override.
   - `expand_once` / `expand_always` → re-run via a **separate ad-hoc executor
     instance** constructed with the ceiling-clamped cap (`tools.NewExecutor(art,
     clampedCap)`) — the shared `o.runner` keeps the configured default; only this one
     call gets a bigger allowance. On `_always`, persist the override.
   - `refine` → see Phase B.
   - `abort` → publish a denied-shaped result; fold a `toolDeniedContext`-style note into
     the turn (same shape as a policy `Deny`).

## Phase B — LLM self-refinement (`refine`)

### `tools.Proposer.ProposeRefinement` (new method, sibling of `Propose`)
- `Propose` (`internal/tools/proposer.go` — retries only on parse/transport failure, no
  feedback about *why* a prior call was rejected) gets a new sibling:
  `ProposeRefinement(ctx, userText string, prior tools.Result, reason string) (Proposal, bool)`
  — same retry/parse machinery, but the first message explicitly states the prior tool
  call, how much it captured, and that it should be narrowed (`rg` over `cat`, `-maxdepth`,
  a scoped path, `head`/`tail`) rather than repeated as-is.
- GIVEN `refine` is chosen WHEN `ProposeRefinement` returns a new proposal THEN
  `runToolPhase` runs it exactly like a fresh proposal (policy → execute → this same
  post-execution check) — recursion, not a special case.
- GIVEN the refined proposal **also** truncates THEN `refine` is not offered a second time
  for this call chain — escalate straight to the human decision menu (options above, minus
  `refine`). Two failed narrowing attempts means the model isn't converging; don't loop
  silently or repeatedly bother it.

## Scope boundaries / open questions (flag before implementing, not decided here)

1. **Live working-memory pin refresh** (`refreshLiveFacts`, `internal/runtime/
   orchestrator.go:789-824`) shares the same executor. An interactive approval prompt
   firing on an unattended, once-per-turn background pin refresh would be jarring — this
   spec recommends pin refreshes always resolve as `use_truncated_once` silently (no
   gate), and only the interactive `single_tool` cycle gets the full menu. Needs an
   explicit "interactive vs. background" flag threaded to wherever this check lives.
2. ~~Plan-step tool calls... blocking a plan step on human approval is likely wrong~~ —
   **retracted, 2026-07-13**. `executor.Execute` already blocks a plan leaf synchronously
   on the exact same decision gate (`Orchestrator.RequestApproval`/`RequestDecision`) when
   policy returns `NeedsApproval` (`internal/executor/executor.go:254-265`,
   `internal/runtime/classifier_pipeline.go:83-101` wires the approver) — sibling nodes
   keep running in parallel (`internal/runtime/scheduler.go:182`, one goroutine per node);
   only the blocked node's goroutine waits. This is the proven pattern and `PhaseOutputSize`
   should follow it exactly: block the plan leaf on the same gate, no special-casing for
   "unattended" execution. See **Prerequisite** below for the real gap this surfaced.
3. **Cap value for `expand_always`** — per-tool (recommended: a `find` on one path may
   legitimately need more room than a `cat` elsewhere) vs. one global bumped default.
4. Whether the existing generic approval UI component can render this prompt as-is
   (byte counts in prose) or needs bespoke treatment (e.g. a size slider) — v1 should
   reuse the generic component; no new UI work assumed here.

## Prerequisite (pre-existing gap, not new to this feature): task.Status can't tell "blocked on a decision" from "genuinely broken" — SHIPPED 2026-07-13 as TOOL-7

Surfaced while correcting open question #2, 2026-07-13. `scheduler.execute`
(`internal/runtime/scheduler/scheduler.go:250-256`, before the fix) was:

```go
func (s *Scheduler) execute(ctx context.Context, rec task.Record) task.Status {
    out := s.executor.Execute(ctx, rec)
    if out.Status == executor.Executed { return task.Done }
    return task.Failed
}
```

`executor.Outcome.Status` (`internal/executor/executor.go:78-102`) already distinguishes
`Executed` / `Phantom` / `Denied` / `NeedsApproval` / `NoTool` / `Failed` — a user's
explicit decline is a *different, meaningful* outcome from a crash, a bad exit code, or a
timeout. But `task.Status` (`internal/prompting/task/task.go:41-56`, only
`Proposed`/`Ready`/`Abstained`/`Done`/`Failed`) has no value for it, so `scheduler.execute`
collapses all five non-`Executed` outcomes into the same `task.Failed`. This is exactly
the ambiguity the `nimble-pebble-2` RCA hit: `task-565-1`'s `git_status` call came back
`outcome: "denied"`, but the plan's terminal report just said "1 failed, 0 abstained, 1
never ran... of 3 nodes" — nothing distinguished a policy decision from a bug. It is a
**pre-existing bug in the already-shipped TOOL-3/TOOL-4 approval integration**,
independent of whether TOOL-6 ever ships — and TOOL-6's `PhaseOutputSize` gate would
inherit the identical collapse on its `abort` path without a fix.

**Shipped fix**: added `task.Denied` meaning "did not complete because of an explicit
policy/user decision — declined approval, denied by blacklist" — distinct from
`task.Failed` (a genuine execution error: crash, unexpected non-zero exit, timeout,
phantom no-op). `scheduler.execute` maps `executor.Denied`/`executor.NeedsApproval` to
`task.Denied`; `Phantom`/`NoTool`/`Failed` remain `task.Failed`. The plan-completion
error string (`"plan incomplete: N failed, M abstained, K never ran"`, seen in the RCA)
grew a fourth bucket: `"...N denied (needs approval)..."`. The plan widget
(`internal/surfaces/output/plan.go`) renders `task.Denied` with its own 🔒 glyph, never
the same ❌ as `task.Failed` — the widget-layer switches had their own separate
default-to-"failed" fallback that needed the same fix. Tracked as **TOOL-7** in
`docs/build-plan/04_tool_runtime_backlog.md`, landed ahead of TOOL-6 Phase A per the
dependency noted there (Phase A's `abort` option would otherwise inherit the same
collapse this fixed).

## Suggested delivery split

- **TOOL-7 (prerequisite, S) — SHIPPED 2026-07-13**: `task.Denied` status + the
  `scheduler.execute` mapping fix + plan-completion error string update + widget
  glyph. Fixed a real, already-shipped gap on its own merits (RCA-visible today)
  independent of TOOL-6.
- **Phase A** (S–M): decision gate only (`use_truncated_*`, `expand_*`, `abort`), reusing
  `RequestDecision` wholesale — new `state.PhaseOutputSize`, new persisted override
  file + loader (mirrors `continuation.LoadVerbs`/`AppendVerb` but with a numeric cap
  instead of a bare verb), the absolute ceiling constant, wiring in both `runToolPhase`
  and the plan-leaf path (`internal/executor/executor.go`'s `Execute`, alongside its
  existing approval check — same gate, same seam, no plan/interactive special-casing
  per the retracted open question #2 above). No model changes.
- **Phase B** (M–L, do after A lands and the gate's shape is proven): `refine` +
  `Proposer.ProposeRefinement`, the bounded one-shot refinement guard, and the
  interaction with Phase A's menu (dropping `refine` after one failed attempt).

## Tests (to write alongside Phase A / B, not yet written)

- `tests/features/tools/output_size_recovery.feature` (`@integration`,
  `@arch:output-size-recovery`): the four Phase-A resolutions, the ceiling clamp, and the
  "remembered override still visibly labeled" case.
- `tests/features/tools/output_size_recovery.feature` additions for Phase B: refine
  succeeds narrower; refine still truncates → escalates to human menu without `refine`.
- Unit coverage for `continuation`-style load/append helpers backing the new override
  file, mirroring `internal/prompting/continuation`'s existing test shape.
