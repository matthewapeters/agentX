# Architecture Reconciliation: Family A (now) vs Family B (future)

Last updated: 2026-08-01 (ADR 0006-0012 status clarified; originally 2026-06-26)
Status: Authoritative orientation note
Owner: Architecture

## Why this note exists

`docs/` contains two distinct architectures. The build plan
(`docs/build-plan/`) cross-references both as if they were one system, which makes
the "what to build now" ambiguous. This note fixes the canonical near-term target and
parks the rest, so planning and schema-freeze work have an unambiguous baseline.

## The two families

### Family A — client-server runtime with multiple surfaces (BUILD NOW)

The near-term product. A core **server** is the orchestration hub; **surfaces** are
separate client processes that attach over an HTTP/SSE transport with an ephemeral
attach token.

- `agentx` boots the **server and the human-agent chat surface together**. (A
  server-only launch mode is planned but out of scope now; it mainly matters for a
  future web client.)
- The **chat surface has two panels**: output and input.
- The former single "system" panel is now a set of **independent, separately
  launchable surfaces** (files, config, context, context-history,
  context-visualizer). The user arranges them via a multiplexer (tmux/screen/zellij)
  or separate windows.
- The **surface registry is open-ended**: new surface kinds attach without changing
  existing surfaces.

Authoritative sources:

- `docs/implementation/01_runtime_blueprint.md` … `09_makefile_and_quality_gate_contract.md`
- `docs/architecture/channel_registry.md`
- `docs/architecture/runtime_contracts/` (the frozen Family-A v1 contracts)
- `docs/architecture/adr/0006…0012` (persona/context loading, output-panel
  rendering, task decomposition + DAG scheduler, execution visibility, task
  assertions, wavefront-grounded decomposition) — **shipped Family-A build,
  not Family B.** ADR 0008's `decompose.DrainPlan` and ADR 0012's
  `internal/runtime/wavefront` scheduler are wired live into the loop via the
  `plan_task` tool (`internal/runtime/plan_tool.go`, 2026-07-31) — see
  `docs/implementation/04_llm_prompt_tooling_runtime.md`. `docs/architecture/adr/00_INDEX.md`
  carries per-ADR status annotations; this note's own list below (updated
  2026-06-26) predates 0006–0012 and did not originally cover them.

### Family B — multi-expert DAG orchestrator (FUTURE)

A richer server-side orchestration model: request envelope → compiled DAG → dispatcher
priority heap → execution outcomes, with Gate A–E quality reports and replay bundles.
This is the server's **future orchestration brain**. It sits *behind* the
surface/transport boundary, so it can land later without changing the surfaces.

Authoritative sources (treat as **future**, not the current build target):

- `docs/architecture/design/01_*.md` … `06_*.md` (ADR 0001–0005)
- `docs/architecture/schemas/*.schema.json` (request-envelope, compiled-dag,
  execution-outcome, policy-decision, trace-event, replay-bundle, quality-gate-report)
- `docs/architecture/adr/0001…0005` **only** — ADR 0006 and up are Family A (see
  above), not part of this future-orchestrator scope.

## The seam

The **surface/transport boundary is the seam** between A and B. Family A builds a
simple orchestrator behind that boundary now; Family B replaces/extends the
orchestrator behind the same boundary later. Surfaces should not encode orchestration
internals — they consume canonical state and events over the transport.

## Practical rules for builders

1. For current milestones (M1–M4 as scoped against the implementation docs), build
   **Family A**. Freeze and reference the contracts in
   `docs/architecture/runtime_contracts/`.
2. Do **not** implement Family B schemas/designs in the near-term build. When a build
   task references a Family-B doc, treat it as background/aspirational unless this note
   is updated to promote it.
3. When the orchestrator gains real complexity, introduce Family B behind the existing
   transport boundary and record the cutover in an ADR.
