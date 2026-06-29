# System Surfaces Backlog — First Peer Surface (M2)

Last updated: 2026-06-29
Status: Execution-ready backlog
Owner: Delivery Lead
Scope: The shared **surface-client framework** plus the first **independent
rendering surface** (the context viewer) — a Bubble Tea v2 client process that
attaches to a running orchestrator over the M1 transport and renders the session.

## Context

`docs/build-plan/01_comprehensive_build_plan.md` defines milestone **M2 (UX Surface
Parity Baseline — TUI + System Surfaces)** at the capability level. The chat slice
(`03_chat_surface_backlog.md`) delivered the two-panel chat surface in-process, and
the transport slice (`05_transport_backlog.md`, TRN-1…6) made the orchestrator a real
client-server hub: external surfaces can register with an attach token, read snapshots
and stream events over HTTP/SSE, and be launched with `agentx surface launch`.

This document is the missing task tier for the first **rendering** peer surface.

> **The legacy UX counts are not parity targets.** `docs/ux/00_INDEX.md` shows
> PD-01…PD-17 with 112 "tested" affordances — those belong to the prior Python/Tkinter
> GUI, which does **not** exist on the `bubbletea` branch. On this branch no system
> surface exists yet. M2 builds them fresh as Bubble Tea TUI surfaces, the way CHT-*
> re-authored PD-01/PD-02 for the TUI. See
> `../architecture/00_ARCHITECTURE_RECONCILIATION.md`.

## Goal (definition of done for the slice)

Running `agentx surface launch context --session <s> --connect <ep> --token <t>` in a
second terminal attaches a **context viewer**: it registers with the orchestrator,
hydrates the session's prior events from the persisted log, live-tails new events over
SSE, renders them with the existing collapsible output widgets, and reflects
processing-state — a read-only mirror of the conversation, arranged by the user beside
the chat surface in their multiplexer. Quitting marks the surface `stopped`. `make all`
stays green throughout.

## Locked design decisions

1. **Surfaces are separate Bubble Tea v2 client processes** launched by `agentx
   surface launch`, attaching over the M1 transport. The chat surface stays
   in-process; this is the first *external* rendering surface.
2. **First surface = context viewer, read-only.** It consumes only what the transport
   already serves (events + processing-state) and introduces no new write paths —
   the lowest-risk way to prove the shared client framework.
3. **Thin slice: framework + one surface.** Files, config, context-history, and
   context-visualizer are future increments that reuse the framework (see Future
   surfaces).
4. **Hydrate-then-tail.** The event bus has no replay, so a surface attaching
   mid-session would otherwise miss prior events. A surface first fetches the
   persisted event history (new read endpoint), renders it, then live-tails
   `GET /events`. New events observed during hydration are de-duplicated by event
   identity (epoch + content type + ordinal) so nothing is dropped or doubled.
5. **Reuse the chat output renderer.** The context surface renders the event stream
   with `internal/surfaces/output.Model` (collapsible boxed widgets, viewport scroll,
   `Apply(state.Event)`) — the same renderer the chat output panel uses — so rendering
   parity is automatic.
6. **Re-author UX specs per surface for the TUI.** Each surface gets a fresh
   `docs/ux/03_PANEL_DETAILS.md` section with new `PD-<surface>-AF-NNN` affordance IDs
   and Gherkin contracts, and a row in the lifecycle matrix; legacy GUI affordances
   are not carried over verbatim.
7. **Quit shuts the surface down cleanly.** Ctrl-C / `ESC,q` POSTs
   `/surface/{id}/shutdown` (lifecycle → `stopped`) before the process exits.

## Architecture of the slice

```
agentx (orchestrator + in-process chat)            second terminal
  internal/transport/http                            agentx surface launch context …
    GET /sessions/current/events  (history, new)  ◀──┐
    GET /events                   (SSE live tail) ◀──┤  internal/surfaces/client
    POST /surface/register / shutdown             ◀──┤    attach (TRN-5 Launch)
                                                      │    hydrate + tail → tea.Msg
  internal/runtime.Orchestrator                       │    SurfaceModel host loop
    History() []state.Event  (reads persisted log)    │
                                                      └─ internal/surfaces/context
                                                            SurfaceModel: projects
                                                            events → output.Model
```

Import direction (`08_go_module_layout.md`): `internal/surfaces/*` may import
`internal/state`, `internal/session`, `internal/transport`; surfaces may reuse sibling
surface packages (as `chat` already imports `output`/`input`). `internal/cli` may
import `internal/surfaces` and `internal/transport`.

Every task carries the cross-cutting obligations: a GIVEN/WHEN/THEN behavior doc
before implementation, a Godog feature + steps under the tag scheme (+ `@ux:<id>` /
`@arch:surface-client`), and AC→test traceability.

---

## Phase F — First peer surface (M2 slice)

### SS-1 · Event history endpoint + client hydrate-then-tail · M
- **Target**: `internal/runtime/` (`Orchestrator.History`), `internal/transport/http`
  (read endpoint + `Provider`), `internal/transport/http.Client` (history + events
  stream)
- **Source**: `02_surface_orchestration_http.md` (Read/Streaming endpoints), `03`
  (persistence), `event-envelope.schema.json`
- **Behavior**: add `GET /sessions/current/events` returning the persisted session
  event log (orchestrator reads it via `session.Recorder.Load`, exposed as
  `Provider.History`); extend `transport/http.Client` with `History(ctx)` and an
  `Events(ctx) (<-chan state.Event, …)` SSE consumer. A client helper hydrates from
  history then subscribes, de-duplicating the overlap by event identity.
- **Feature**: `tests/features/transport/event_history.feature`
  (`@integration @arch:transport`)
- **Done**: history endpoint returns the recorded events in order; the client yields
  history-then-live with no gap or duplicate across the handover.

### SS-2 · Surface-client framework (host + lifecycle) · L
- **Target**: `internal/surfaces/client/` (new), `internal/cli/` (launch-into-UI)
- **Deps**: SS-1
- **Source**: `01_runtime_blueprint.md` (Bubble Tea Adoption), `02` (Surface Model,
  CLI launch), `06_TUI_MIRROR.md` (legacy, for affordance intent)
- **Behavior**: a reusable Bubble Tea host that takes a registered attach
  (reusing TRN-5 `cli.Launch`) and a `SurfaceModel` (the per-surface contract:
  `Apply(state.Event)`, `SetProcessing(state.ProcessingState)`, `SetSize`, `View`,
  key handling), pumps the hydrate-then-tail stream into `tea.Msg`s, handles terminal
  resize and quit (POST `/surface/{id}/shutdown` then exit), and exits cleanly when
  the stream closes (orchestrator gone). `agentx surface launch <kind>` dispatches to
  the framework with the kind's `SurfaceModel`; a known kind with no UI yet reports a
  clear "not implemented" message.
- **Feature**: `tests/features/surfaces/surface_client.feature`
  (`@functional @arch:surface-client`)
- **Done**: the host applies streamed events to its model and renders; quit posts
  shutdown; stream close ends the program; the launch dispatch selects the right model.

### SS-3 · Context viewer surface · M
- **Target**: `internal/surfaces/context/` (new), `docs/ux/03_PANEL_DETAILS.md`,
  `docs/ux/UX_LIFECYCLE.md`
- **Deps**: SS-2
- **Source**: re-authors PD-03 (SystemSurface — Context) / PD-08 (ContextRenderer)
  for the TUI; `ux/06_OUTPUT_WIDGET.md`
- **Behavior**: a `SurfaceModel` that projects the event stream into
  `internal/surfaces/output.Model` (collapsible widgets, scroll), shows a
  `context · <session>` title and a processing-state line, and is read-only (no
  prompt input). New `PD-<context>-AF-NNN` affordance IDs + lifecycle rows.
- **Feature**: `tests/features/surfaces/context_surface.feature`
  (`@functional @ux:<context>`)
- **Done**: applied events render in order; thinking/tool widgets collapse; processing
  state displays; no input affordance; `make all` green.

> An `@e2e` scenario extends `transport_lifecycle.feature` (or a new
> `context_attach.feature`): a launched context surface hydrates + tails a real
> orchestrator's stream and renders a submitted prompt's response.

---

## Sequencing

```
SS-1 (history + client tail) ─ SS-2 (framework host) ─ SS-3 (context surface) ─ INTEGRATION
```

Critical path is linear: the surface needs the framework, which needs the
hydrate-then-tail client.

## Future surfaces (out of scope for this slice)

Each reuses the SS-2 framework; each is its own increment with re-authored TUI specs.

| Surface | Legacy spec | Adds beyond the framework |
| --- | --- | --- |
| Files browser | PD-11 FileBrowser | filesystem read access; directory tree model |
| Config surface | PD-07 SettingsSurface | config read endpoint; later: write-back + restart-required semantics |
| Context-history | (new) | session-list + reload-from-log (ties to CTX-1 follow-up) |
| Context-visualizer | PD-10 ContextMeterWidget / PD-08 | context-window accounting; enable/disable affordances |
| Working-memory editor | PD-03 (Working Memory) | working_memory.json read/edit over the transport |

Also deferred: surface→surface coordination, attachment chips, plan/DAG visualizer,
and any Family-B orchestration surfaces.

## Milestone / AC mapping

- Phase F → **M2** (`AC-M2-1` in-scope affordance IDs mapped to tested status for the
  context surface; `AC-M2-2` no critical drift between the surface's documented
  transitions and its behavior; processing-state rendered without drift).
- Each task's Godog feature provides the `TC-M2-<area>-<nnn>` test-case IDs that fill
  the M2 AC coverage table at the checkpoint.
