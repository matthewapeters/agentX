# AgentX ADR Index

Last updated: 2026-07-17
Owner: Architecture

This folder captures architecture decisions for orchestration design, rollout sequencing, and quality governance. These ADRs are implementation-oriented and aligned with:

- docs/architecture/design/00_INDEX.md
- docs/architecture/channel_registry.md
- docs/implementation/00_index.md

## ADRs

- [0001 - Orchestrator Control, Execution, and Data Planes](0001-orchestrator-control-execution-data-planes.md)
- [0002 - DAG Node Taxonomy and Pattern Compiler](0002-dag-node-taxonomy-and-pattern-compiler.md)
- [0003 - LLM Dispatcher Priority Heap and Fairness](0003-llm-dispatcher-priority-heap-and-fairness.md)
- [0004 - Persona, Skill Policy, and Security Boundaries](0004-persona-skill-policy-and-security-boundaries.md)
- [0005 - Traceability, Replay, and Quality Gates](0005-traceability-replay-and-quality-gates.md)
- [0006 - Persona Identification and Context Loading](0006-persona-identification-and-context-loading.md)
- [0007 - Output-Panel Markdown Rendering (Dual Renderer)](0007-output-panel-markdown-rendering.md) — Family A surface concern
- [0008 - Recursive Task Decomposition and the DAG Scheduler](0008-recursive-task-decomposition-and-dag-scheduler.md) — realizes `invoke_planner`; supersedes the eager `run_subtask` model
- [0009 - Plan & Tool Execution Visibility and Control](0009-plan-and-tool-execution-visibility.md) — all tool execution user-visible: streamed plan events, approval/abort, plan JSON persistence, Context surface
- [0010 - Task Assertions, Outcome Grounding, and Plan Continuity in Working Memory](0010-task-assertions-outcome-grounding-and-plan-continuity.md) — every node's result is judged against a declared assertion (satisfied/refuted/abstained); resolves ADR 0008 OQ2/OQ6 and realizes ADR 0009 Phase 9e
- [0012 - Wavefront-Grounded Decomposition and the Shared Blackboard](0012-wavefront-grounded-decomposition-and-shared-blackboard.md) — a second, continuously-dispatched decomposition engine (`internal/runtime/wavefront`) selectable alongside ADR 0008's continuous scheduler; closes the hallucinated-plan-argument gap by never letting a node's tool args be decided ahead of the evidence they depend on. Extends, does not supersede, ADR 0008. **Amended 2026-07-17**: no standalone blackboard type (the graph *is* the blackboard — `task.Record` gains `Value`/`Error`/`Seq`), no round-synchronization, and engine interleaving flagged as an explicitly open future direction the shared schema already supports.
- [0013 - ConversationCore: Extracting the Prompt/Tool/Hook Loop from Orchestrator](0013-conversationcore-extraction.md) — **Implemented 2026-08-01, all 5 phases.** Carves the prompt/tool-call/hook loop out of `Orchestrator` into an independently-constructible `ConversationCore` (`internal/runtime/core*.go`), behind three seams (`ApprovalSeeker`, `EventSink`, `ContextStore`) that abstract approval-gating, event publishing, and context assembly/turn recording. Proven standalone- and concurrent-runnable with zero `Orchestrator` involvement (Phase 5). Precondition for any future nested/sub-session conversational loop (motivated by, but does not design, a future planning-tool consumer — that remains open, see the ADR's Open Questions).

**Numbering gap:** "ADR 0011" (system/user prompt role separation in the planner) is
cited throughout the codebase and has a behavior doc on disk, but no `0011-*.md` file
was ever written in this folder. Not backfilled as part of adding 0012 — noted here so
the gap isn't mistaken for a missing link, and remains available as a small follow-up.

## Reading Order

1. Read ADR 0001 for system boundaries and ownership.
2. Read ADR 0002 for orchestration graph semantics.
3. Read ADR 0003 for dispatch scheduling and fairness.
4. Read ADR 0004 for policy and security controls.
5. Read ADR 0005 for evidence, replay, and release gates.
6. Read ADR 0006 for persona routing and deterministic context assembly.

## Implementation Companions

- Detailed design docs: `docs/architecture/design/00_INDEX.md`
- Behavior use cases (Gherkin): `docs/architecture/behavior/00_INDEX.md`
- Authoritative schemas: `docs/architecture/schemas/00_INDEX.md`

## Change Policy

- New architecture decisions that alter orchestration ownership or policy must add a new ADR file in this folder.
- Existing ADRs are immutable in intent. If intent changes materially, supersede with a new ADR and reference the prior one.
- Implementation PRs should cite the ADR ID in commit messages, PR descriptions, or both.
