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
seeds the session's prior events from the persisted log, resumes the live event stream
by cursor, renders them with the existing collapsible output widgets, and reflects
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
4. **Disk-seeded, then cursor-resumed live stream.** The durable append-only log —
   not the in-memory bus — is the source of truth, including each event's `enabled`
   state. A surface **seeds** from the persisted log (read server-side in a
   goroutine via `session.Recorder.Load`), renders it, and tracks the last consumed
   event **ordinal**; it then opens the live stream declaring that cursor, and the
   server delivers only events after it. The handover is exact — no gap, no
   duplicate — because the cursor is the stamped envelope ordinal (decision 8), not a
   fuzzy identity match; this replaces the earlier "hydrate-then-tail with
   client-side de-dup" idea, whose weakness was that bus events carry no `seq`.
8. **Monotonic ordinal on the envelope.** The recorder's per-event `seq` currently
   lives only in the persisted filename, so live bus events can't be reconciled
   against the disk log. SS-1 stamps a monotonic `ordinal` onto the `state.Event`
   envelope **at publish time**, so the same identity is carried by the live bus
   event and its persisted file. The ordinal is both the canonical total order and
   the resume cursor. This adds a field to
   `docs/architecture/runtime_contracts/event-envelope.schema.json` (a versioned
   contract change, coordinated per the freeze rules).
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
    GET /sessions/current/events  (disk seed)     ◀──┐
    GET /events?after=<ordinal>   (live, cursor)  ◀──┤  internal/surfaces/client
    POST /surface/register / shutdown             ◀──┤    seed (disk) → last ordinal
                                                      │    → subscribe(after) → tea.Msg
  internal/runtime.Orchestrator                       │    SurfaceModel host loop
    History() []state.Event  (disk, goroutine)        │
    publish() stamps ordinal on the envelope          │
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

### SS-1 · Disk-seeded event stream with cursor resume · M
- **Target**: `internal/state` (envelope `ordinal`), `internal/session` (recorder
  orders by it), `internal/runtime` (`Orchestrator.History`, stamp ordinal at
  publish), `internal/transport/http` (seed endpoint + cursor stream + `Provider`),
  `internal/transport/http.Client` (seed + resume),
  `docs/architecture/runtime_contracts/event-envelope.schema.json`
- **Source**: `02_surface_orchestration_http.md` (Read/Streaming endpoints), `03`
  (persistence), `event-envelope.schema.json`
- **Behavior**:
  1. Stamp a monotonic `ordinal` on the `state.Event` envelope at publish time (so
     the live bus event and its persisted file share one identity); the recorder
     uses the envelope ordinal for naming/ordering.
  2. `GET /sessions/current/events` returns the disk-persisted log
     (`Orchestrator.History` via `session.Recorder.Load`, read off the hot path) —
     the authoritative seed, carrying each event's `enabled` and `ordinal`.
  3. `GET /events?after=<ordinal>` (SSE) streams only events after the cursor: the
     server attaches to the live bus first, backfills from disk any
     `(after, first-live]` gap, then continues live — spliced exactly by ordinal, so
     the seed→live handover has no gap and no duplicate. `after=0` yields the full
     stream (seed + live) for clients that prefer one connection.
  4. `transport/http.Client` exposes `Seed(ctx)` and `Subscribe(ctx, after)`; the
     client seeds, takes the last ordinal, then subscribes — no client-side de-dup.
- **Feature**: `tests/features/transport/event_seed_stream.feature`
  (`@integration @arch:transport`)
- **Done**: the seed returns the recorded events ordered by ordinal, including
  `enabled`; subscribing with the seed's last ordinal yields exactly the subsequent
  events — verified with no gap or duplicate even when events are written during the
  handover; `after=0` yields the full stream.

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
