# Open Implementation Questions

Purpose: collect implementation decisions required to finalize execution planning.

## Resolved Decisions (2026-06-23)

1. Surfaces run as separate processes and communicate over HTTP with bidirectional semantics.
2. Users can launch child surfaces from separate terminal sessions; orchestrator registers and manages them.
3. Ports are assigned from configured ranges in runtime config, with availability checks for each assignment.
4. Runtime must tolerate multiple concurrent AgentX instances on the same host.
5. Deployed runtime config path is ~/.config/agentx/agentx.toml.
6. Session storage root is ~/.config/agentx/sessions/.
7. Command approvals are keyed by command with optional args; blacklist can still deny specific dangerous forms.
8. Model switch behavior prompts the user each time to decide handling of in-flight work.
9. Terminology can use both surface and applet interchangeably.
10. Browser-capable surfaces default to localhost with configurable bind address.
11. Session persistence format is one JSON file per event.
12. Config strategy is hybrid: main runtime file with optional domain override files.
13. Default retention keeps all sessions until manual cleanup.
14. Required v1 default surface is the primary TUI output/input shell.
15. Event stream protocol for v1 is SSE; alternatives will be evaluated later if needed.
16. Each session has both session_id (canonical internal) and session_name (human-readable).
17. Main runtime prints launch commands for child surfaces to run in separate terminal sessions.
18. Surface launch canonical shape uses subcommand form with endpoint and attach token.
19. Legacy launch flag form remains as compatibility alias in v1.
20. Surface registration requires valid ephemeral attach token.
21. Godog is mandatory for v1 tests using Gherkin-based behavior scenarios.
22. All functions require pre-implementation GIVEN/WHEN/THEN behavior expectations.
23. CI blocks merges when behavior traceability or required Gherkin contracts are missing.
24. Go module folder structure is standardized with cmd/agentx entrypoint and documented internal/tests layout.
25. make all is canonical baseline command and aliases make clean then make build.
26. Engineers must fix failing tests before merge, including failures from downstream or indirect impact.
27. Any time dependencies change, go mod tidy and go mod vendor are required.
28. Any semantic version change requires CHANGELOG.md update in same change set.
29. Quality gate severities: blockers and warnings must be addressed; nits may be ignored.
30. Ownership for these policy decisions is the user/product owner.

## Remaining Open Questions

### B. Configuration Management

1. First-run seeding update policy when upstream defaults evolve.

### C. Command Policy Depth

1. Session whitelist expiration policy (if any).
2. Forbidden command matching mode: exact only or pattern support.
3. Per-command working-directory restrictions in v1.

### D. Prompt Runtime

1. Procedural prompts shipped as compiled assets or versioned files.
2. Namespaced user prompt packs by profile/persona.
3. Internal prompt integrity checks (hash or signature).
4. Classification Stage 2 — user clarification on ambiguity (open). The classifier
   offers K interpretations (`agentx.classification.clarification_options`); the user
   selects one (number-press), which is appended to the prompt and resubmitted.
   Undecided: (a) the new `clarification` event content type + whether a
   `awaiting_input` processing state is added (both touch frozen contracts +
   CHANGELOG); (b) how the number-select affordance coexists with the input/command
   modes; (c) whether ambiguity is a first-class classifier outcome or only a
   post-retry fallback. See `04_llm_prompt_tooling_runtime.md` (Classification Cycle)
   and `../build-plan/03_chat_surface_backlog.md` (Phase D / Stage 2).

### E. Persistence and Replay

1. Attachment persistence model: metadata-only versus copied artifacts.
2. Replay mode defaults: chronological only versus prompt-cycle grouped views.

### F. UX Scope

1. Where processing_state is shown by default: all headers or selected surfaces.

## Decision Log

Add accepted decisions below with date and rationale.

- 2026-06-23: Applied initial runtime decisions from stakeholder responses (see Resolved Decisions section).
- 2026-06-23: Locked SSE as the v1 event-stream protocol; defer protocol expansion until post-v1 evaluation.
- 2026-06-23: Locked session identity dual-model (session_id plus session_name) and secure child surface launch contract for v1.
- 2026-06-23: Locked Godog-only v1 testing, mandatory function-level Gherkin expectations, and CI merge blocking for missing traceability.
- 2026-06-23: Added canonical Go module folder layout standard to reduce repository-structure ambiguity.
- 2026-06-23: Added make all baseline contract and mandatory failing-test ownership policy.
- 2026-06-23: Locked dependency hygiene, semver/changelog coupling, gate severity handling, and ownership model.
- 2026-06-26: Locked surface model — the chat (human-agent) surface has exactly two
  panels (output + input). The former single "system" panel is retired in favor of
  multiple independent, separately launchable surfaces (files, config, context,
  context-history, context-visualizer). Surface registry is open-ended for future
  surface kinds. "applet" terminology (decision #9) is retired in favor of "surface".
- 2026-06-26: Locked launch model — `agentx` boots the core server and the chat
  surface together; a server-only launch mode is deferred (out of scope, relevant to a
  future web client).
- 2026-06-26: Recorded A-now / B-later architecture split (see
  docs/architecture/00_ARCHITECTURE_RECONCILIATION.md). Near-term build is the Family A
  client-server runtime; the multi-expert DAG orchestrator (Family B,
  docs/architecture/design/ + docs/architecture/schemas/) is the server's future
  orchestration brain behind the surface/transport boundary.
- 2026-06-26: Froze Family A Phase 0 contracts under
  docs/architecture/runtime_contracts/ (index + event-envelope, processing-state, and
  surface-registration JSON schemas).
