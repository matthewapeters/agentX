# Tool Runtime Backlog — First Built-in Tool (Command Line)

Last updated: 2026-06-28
Status: Execution-ready backlog
Owner: Delivery Lead
Scope: The minimal end-to-end slice that lets the agent run **curated, policy-gated
command-line tools**.

> **⚠️ Wire mechanism superseded (2026-07-31).** This backlog was written and shipped
> against the classify-gated `single_tool` route with a hand-rolled strict-JSON tool
> proposal (`internal/tools.Proposer`/`ParseProposal`). That convention is retired —
> tools are now advertised to the model as native tool-calling schemas
> (`internal/tools.Registry.ToolSchemas`) and called any number of times per turn in
> the flat loop (`internal/runtime/loop.go`), not gated by a classifier route. The
> descriptor/policy/execution machinery this backlog built (TOOL-1, 2, 3, 5, 6, 7, 8)
> is still live and accurate as shipped — only the proposal/dispatch wire format and
> the `classify → single_tool` framing below are stale. See
> `../implementation/04_llm_prompt_tooling_runtime.md` ("Native tool calls (v2)").

## Context

`docs/build-plan/01_comprehensive_build_plan.md` defines milestone **M3b (Tool Runtime
And Policy Enforcement)** at the capability level (AC-M3b-1…4) but has no task-level
backlog. This document is that missing tier for the first built-in tool family.

It builds on the chat slice (`03_chat_surface_backlog.md`) and the classify cycle
(CHT-D4) and implements the contracts in:

- `docs/implementation/04_llm_prompt_tooling_runtime.md` (Tool Runtime Overview, the
  `single_tool` cycle, tool catalog injection)
- `docs/implementation/05_security_approvals_and_command_policy.md` (policy layers,
  evaluation order, descriptor contract, execution safety, auditing, output artifacts)

## Locked design decisions

1. **Curated descriptor set** (not a generic `run_command`). Initial tools: read/search
   (`read_file`, `list_dir`, `find_path`, `read_output`), write/modify (`write_file`,
   `apply_patch`, `edit_file`), network (`http_get`, `download`).
2. ~~**Strict-JSON proposal, one tool call per turn**~~ — superseded: the model now
   issues native tool calls (provider-parsed, no `internal/classify`-style JSON
   extraction), any number per turn, bounded by `Settings.MaxToolIterationsPerTurn`.
3. **argv, no shell** — commands execute as an argv vector via `os/exec` (never
   `sh -c`); no pipes/redirects/globs/expansion. File content and patches are passed
   inline (JSON) and delivered to the process via **stdin** or a Go built-in — no shell
   interpolation of untrusted arguments (per doc 05).
4. **Read-only tier ships first**; write/modify and network tiers are enabled
   progressively by configuration.
5. **Output persisted as session artifacts.** Full stdout/stderr is written to the
   session; the model receives a compact `tool_result` (preview + size + `ref`) and
   pages through the full output on demand via `read_output`. (Doc 05 audit already
   specifies "output_digest or size metadata," not full output.)
6. **Approval round-trip** — the documented deny / approve_session / approve_global
   flow needs a new processing state and a surface affordance (see TOOL-3). This is the
   same machinery as the deferred Stage-2 clarification flow.

## Architecture of the slice

```
(as originally built — see the superseded-mechanism note above for the current wire format)
classify ──route=single_tool──▶ PhaseTool
   propose   (TOOL-4)  model → {"tool","args"}  [catalog injected from agentx-shell-commands.md]
   evaluate  (TOOL-1)  policy: blacklist → global wl → session wl → approval
   approve   (TOOL-3)  awaiting_input → user: session | global | deny
   execute   (TOOL-2)  argv/no-shell (+stdin) → capture → session artifact (ref)
   publish              🔧 tool_call,  📋 tool_result (preview + ref + audit)
   respond   (TOOL-4)  feed preview+ref to model → 🤖 final answer
```

Current (2026-07-31): `native tool call (any count/turn) → evaluate (TOOL-1) →
approve (TOOL-3) → execute (TOOL-2) → publish → fold result back → loop`, no
classify gate — see `internal/runtime/loop.go`.

New package `internal/tools` (reserved in `08_go_module_layout.md`): descriptors,
policy, executor, artifact store.

---

## TOOL-1 · Descriptors + command policy · M
- **Target**: `internal/tools/`
- **Deps**: CHT-D4
- **Behavior**: descriptor contract (`id, command, allowed_args schema, risk_level,
  requires_approval, timeout_seconds, working_directory_policy, output_capture_policy`);
  policy store with blacklist (static + config), global whitelist (persisted), session
  whitelist (in-memory). `Evaluate(id, args) → allow | deny(reason) | needsApproval` in
  the documented order (blacklist always wins); approvals keyed by command + validated
  args; arg-schema validation rejecting shell-escaping/destructive flags
  (`find -exec`/`-delete`, recursive force-delete, …).
- **Feature**: `tests/features/tools/command_policy.feature` (`@unit`)
- **Done**: eval order + precedence + deny reason codes + arg-schema rejection covered.
- **Maps to**: AC-M3b-1.

## TOOL-2 · Executor + session artifact store · M
- **Target**: `internal/tools/`, `internal/session/`
- **Deps**: TOOL-1
- **Behavior**: execute an approved descriptor as an argv vector (no shell), optional
  stdin payload (content/patch), context timeout, capture stdout/stderr/exit, truncate
  to an output cap. Write full output to a session **artifact** (`sessions/<id>/
  artifacts/<seq>.txt`) and return `{exit, status, bytes, line_count, preview, ref}`.
  Artifact store supports read-back by `ref` with offset/limit (backs `read_output`).
- **Feature**: `tests/features/tools/executor.feature` (`@integration`, safe commands
  e.g. `echo`/`ls` only); `tests/features/tools/artifacts.feature` (`@unit`)
- **Done**: timeout, output cap, exit/stderr capture, no-shell argv, artifact write +
  ranged read verified.
- **Maps to**: AC-M3b-2, AC-M3b-3.

## TOOL-3 · Approval round-trip · M
- **Target**: `internal/state/`, `internal/runtime/`, `internal/surfaces/chat/`
- **Deps**: TOOL-1
- **Behavior**: add `RunState = awaiting_input` (**versioned** change to
  `processing-state.schema.json` → `CHANGELOG.md`). The cycle pauses on a decision
  channel; `Bridge.Approve(decision)` + an approval affordance on the 🔧 widget
  (`[a] session · [g] global · [d] deny`). Decision persists per scope (TOOL-1) and
  resumes or aborts the cycle.
- **Feature**: `tests/features/tools/approval.feature` (`@functional`)
- **Done**: pending→approve(session/global)→execute and pending→deny→aborted, with
  persisted scope; interrupt while awaiting handled cleanly.
- **Maps to**: AC-M3b-1, AC-M3b-2.

## TOOL-4 · Proposal + cycle integration + respond shaping · M
- **Target**: `internal/runtime/`, `internal/prompting/`, `internal/tools/`
- **Deps**: TOOL-2, TOOL-3
- **Behavior** *(as originally built; see superseded-mechanism note above)*: on
  `route == single_tool`, enter `PhaseTool`; inject the tool catalog
  (`agentx-shell-commands.md`, default `tools.DefaultCatalog`) and parse one strict-JSON
  tool call (reuse classify parse + retry; `{"tool":"none"}` or parse failure → fall
  back to respond). Run TOOL-1→3→2, publish `tool_call`/`tool_result`, then a respond
  turn whose context carries the **preview + ref** (never the full artifact).
  `read_output` is exposed as a read-tier descriptor so the model can page the artifact.
  *Current (2026-07-31)*: the model issues native tool calls directly (no catalog
  injection, no classify gate, no JSON parse/retry); `PhaseTool` and TOOL-1→3→2 still
  apply per call, any number of times per turn — see `internal/runtime/tool_cycle.go`.
- **Feature**: `tests/features/tools/tool_cycle.feature` (`@e2e`, stub executor)
- **Done**: `classify → tool → respond` with deterministic event ordering
  (`user_prompt → classification → tool_call → tool_result → agent_response`); result +
  ref injected — **as originally shipped; current ordering has no `classification`
  event** (`user_prompt → tool_call → tool_result → ... → agent_response`).
  *Revised 2026-07-12*: the original "preview + ref, never the full
  artifact" design silently line-truncated durable facts too (RCA: session
  `nimble-pebble-2`, see `CHANGELOG.md`); context now always carries the full captured
  result, bounded only by `output_max_bytes`, and truncation is labeled, never silent.
  See TOOL-6 for the recovery gate this opened up.
- **Maps to**: AC-M3b-2, AC-M3b-4.

## TOOL-5 · Config, persisted policy, seeds · S
- **Target**: `internal/config/`, `config/seed/`
- **Deps**: TOOL-1
- **Behavior**: `[agentx.tools]` config (enable, tier enablement, default
  `timeout_seconds`, `output_max_bytes`); persisted blacklist + global whitelist under
  `~/.config/agentx/`. Add policy seed templates to `config/seed/`. *(The
  `agentx-shell-commands.md` catalog this originally loaded is retired — see
  superseded-mechanism note above.)*
- **Feature**: covered via TOOL-1/TOOL-4 config-driven scenarios (`@unit`)
- **Done**: tiers gate availability; policy persists across sessions; seeds mirror
  built-in defaults 1:1.
- **Maps to**: AC-M3b-1.

## TOOL-6 · Oversized-output recovery gate · S–M (Phase A) + M–L (Phase B) · post-M3b, RCA-driven · Phase A SHIPPED 2026-07-13
- **Target**: `internal/runtime/` (`runToolPhase`, plan-leaf path in
  `internal/executor/executor.go`), `internal/tools/` (`Proposer`, Phase B only),
  `internal/config/`
- **Deps**: TOOL-2 (executor truncation + honest labeling, already shipped), TOOL-3
  (proves the `RequestDecision`/gate/`awaiting_input` shape this reuses), **TOOL-7**
  (shipped — Phase A's `abort` path needed the `task.Denied` distinction to be
  meaningful, not just `task.Failed`)
- **Behavior**: full design in
  `docs/architecture/behavior/adr/0010_oversized_tool_output_recovery.feature.md`.
  Summary: when `tools.Result.Truncated` (the `output_max_bytes` safety net triggered),
  stop just publishing the labeled-but-partial result — offer a decision (reusing
  `RequestDecision`, a new `state.PhaseOutputSize`, mirroring the continuation-verb
  allow/deny/always shape): use the truncated result (once/always), re-run with a
  ceiling-clamped larger cap (once/always, persisted per tool ID), or (Phase B) let the
  agent self-refine the tool call (narrower command) before falling back to the human
  menu. Not part of original M3b acceptance criteria — opened up by the
  `nimble-pebble-2` RCA fix to TOOL-4/TOOL-2's truncation behavior.
- **Feature**: `tests/features/tools/output_size_recovery.feature` +
  `tests/steps/tools/output_size_recovery_steps.go` — seven scenarios (UC-OUTSIZE
  001–007), see the behavior doc's Tests section.
- **Done**: Phase A shipped 2026-07-13 — decision gate, persisted per-tool
  overrides, absolute-ceiling clamp, wired into both the interactive cycle and
  plan leaves via a shared `executor.OutputSizeDecider` seam. Phase B (LLM
  self-refinement) not started; see the behavior doc's "Suggested delivery
  split" and the remaining open scope questions (live-pin-refresh) before
  starting it. The plan-step-blocking open question was retracted 2026-07-13 —
  plan leaves already block correctly on the approval gate today
  (`internal/executor/executor.go:254-265`); `PhaseOutputSize` follows the same
  seam, no special-casing.
- **Maps to**: none (post-M3b addition; not required for original AC-M3b-1…4).

## TOOL-7 · Distinguish "blocked on a decision" from "genuinely failed" in task.Status · S · pre-existing gap, RCA-driven · SHIPPED 2026-07-13
- **Target**: `internal/prompting/task/task.go` (new `Status` value),
  `internal/runtime/scheduler/scheduler.go` (`execute`'s outcome mapping),
  `internal/runtime/plan_cycle.go` (plan-completion error string),
  `internal/surfaces/output/plan.go` (widget glyph)
- **Deps**: none — fixes already-shipped TOOL-3/TOOL-4 behavior
- **Behavior**: `scheduler.execute` mapped every `executor.Outcome.Status` that isn't
  `Executed` — `Denied`, `NeedsApproval`, `Phantom`, `NoTool`, `Failed` — to the same
  `task.Failed`. A user's explicit decline (or a blacklist denial) is a fundamentally
  different, meaningful outcome from a crash or bad exit code, but `task.Status` had no
  value for it. This is exactly the ambiguity the `nimble-pebble-2` RCA hit:
  `task-565-1`'s `git_status` call came back `outcome: "denied"`, but the plan's
  terminal report just said "1 failed... of 3 nodes" — indistinguishable from a bug.
  Added `task.Denied`; `scheduler.execute` maps `executor.Denied`/`executor.
  NeedsApproval` to it, leaving `Phantom`/`NoTool`/`Failed` as `task.Failed`. The
  plan-completion error string grew a fourth bucket ("...N denied (needs
  approval)..."). The plan widget renders a denied node with its own 🔒 glyph, never
  the same ❌ as a real failure.
- **Feature**: `tests/features/runtime/task_scheduler.feature` — "A denied leaf is
  distinguished from a genuine failure" (UC-RTSCHED-007b); native Go test
  `TestDeniedNodeRendersDistinctFromFailed` in
  `internal/surfaces/output/plan_test.go` for the widget glyph.
- **Done**: shipped. See
  `docs/architecture/behavior/adr/0010_oversized_tool_output_recovery.feature.md`
  ("Prerequisite" section) for the full writeup; `CHANGELOG.md` 2026-07-13.
- **Maps to**: none (bugfix to already-shipped AC-M3b-1/AC-M3b-2 reporting fidelity).

## TOOL-8 · Planner prompt role separation + conditional directory-listing bias · S–M · RCA-driven · SHIPPED 2026-07-13
- **Target**: `internal/prompting/planner/planner.go` (`Render` → `RenderSystem`/
  `RenderUser`), `internal/runtime/decompose/live.go` (`Chat` type, `LLMPlanner.Plan`),
  `internal/runtime/classifier_pipeline.go` (the `chat` closure), `config/seed/
  agentx-planner.md`
- **Deps**: none — a prompt-structure fix to the already-shipped ADR 0008 planner
- **Behavior**: full design in
  `docs/architecture/behavior/adr/0011_planner_prompt_role_separation.feature.md`.
  Summary: session `vivid-beacon-2` — the planner produced a `list_dir` (`ls -la`) leaf
  even though the full, accurate, untruncated 552-line working-memory `tree` fact was
  already in its context. Root cause: (1) the planner call sends everything —
  instructions, working memory, the goal — as a single flattened `role: "user"` message
  (`decompose.Chat` is a flat-string seam), unlike the respond path's proper
  `system`/`user` split (`prompting.Assembler.Assemble`); (2) within that flattened
  string, the working-memory fact sits sandwiched between an instruction and the goal
  ("lost in the middle"); (3) the instruction itself — "prefer a task that lists a
  directory... before... reads one" — is unconditional, with no carve-out for "unless
  already known." Fix: split the planner prompt into a real system message (durable
  rules + tool catalog + the listing guidance, reworded conditionally) and a real user
  message (working memory + goal + reply-format spec, adjacent with no instruction
  between them) — mirroring `Assembler.Assemble`'s existing pattern, sent as genuine
  `{role: "system"}`/`{role: "user"}` messages to Ollama's `/api/chat` (which applies the
  model's own chat template per role) rather than text-label markers inside one message,
  which would not engage that mechanism at all.
- **Feature**: `internal/prompting/planner/render_test.go` (new) — partitioning +
  conditional-wording regression guard; `internal/runtime/decompose/live_test.go`
  updated for the new `Chat` signature (its only other call site).
- **Done**: shipped. The mechanism (role separation, conditional wording) is verified by
  structural tests; the harder-to-pin-down live-model regression case (does a real model
  actually stop re-listing a known directory) was not attempted — see the behavior doc's
  Tests section.
- **Maps to**: none (prompt-quality fix, not an M3b acceptance criterion).

---

## Sequencing

```
CHT-D4 ─ TOOL-1 ─┬─ TOOL-2 ─┐
                 ├─ TOOL-3 ─┼─ TOOL-4 ── INTEGRATION (single_tool live)
                 └─ TOOL-5 ─┘
```

- TOOL-1 (pure policy) and TOOL-5 (config/seeds) can proceed first and in parallel.
- TOOL-2 (executor+artifacts) and TOOL-3 (approval round-trip) are independent; both
  converge at TOOL-4 for the end-to-end `single_tool` cycle.
- Read-only tier is the first thing wired end-to-end; write/modify and network tiers
  follow behind configuration once the loop is proven.

## Security gate (M3b)

Per `02_phase_reference_matrix.md`, M3b requires a Security Reviewer gate: deny/allow
evidence, the negative-path matrix (denied, malformed, timed-out, interrupted), and
policy reason-code logging. No shell interpolation; every mutating/network call is
approval-gated; all invocations and outputs are persisted to the session for audit.
