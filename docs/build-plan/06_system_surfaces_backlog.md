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
  CLI launch); a `06_TUI_MIRROR.md` legacy affordance-intent doc was referenced here
  historically but is not present in this repo
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

---

## Phase G — Logs surface (M2+)

Scope: a searchable, live-tailing viewer over the full session event log —
`docs/ux/03_PANEL_DETAILS.md` PD-LOGS. Decided against a `less`-in-a-zellij-pane
v1 (rejected: no governance over the pane once `q` is pressed — see PD-LOGS'
"Implementation approaches" §A) in favor of a native `client.SurfaceModel`,
same family as SS-3's context surface. Reuses SS-1/SS-2 as-is (disk seed +
cursor-resumed live stream, `client.Host` lifecycle) — no transport or
persistence changes.

### SS-8 · Host input-capture mode · S

- **Target**: `internal/surfaces/client/client.go`; `internal/surfaces/context/context.go` (one-line interface conformance)
- **Deps**: SS-2 (already landed)
- **Source**: this scoping — no prior doc. `client.Host.Update` currently
  intercepts `q`/`ctrl+c` as a global quit for every `SurfaceModel`
  (`isQuit`, `client.go:155-161`) before the key ever reaches
  `surface.Key`. That's fine for every `SurfaceModel` built so far because
  none of them capture free-form text — but PD-LOGS' `/pattern` search does,
  and "q" is a plausible character inside a search pattern (e.g. `/request`).
  As written, typing it would quit the whole surface mid-search.
- **Behavior**: add a `CapturesKeys() bool` method to the `SurfaceModel`
  interface. When the current surface returns `true`, `Host.Update` skips the
  `"q"` branch of `isQuit` and forwards the key untouched to `surface.Key`;
  `ctrl+c` still always quits, unconditionally — a hard-abort escape hatch
  that works even mid-search-input, so a governed surface is never
  unrecoverable. `context.Model` (the only existing `SurfaceModel`) gets a
  trivial `func (m *Model) CapturesKeys() bool { return false }` to keep
  satisfying the interface; its behavior is unchanged.
- **Feature**: extend `tests/features/surfaces/surface_client.feature`
  (`@functional @arch:surface-client`) with a scenario: a `CapturesKeys()==true`
  double receives `"q"` as an ordinary key while capturing, and `ctrl+c` still
  quits it.
- **Done**: `context.Model` unaffected (`make all` green, existing
  `context_surface.feature` scenarios unchanged); a capturing double proves
  `"q"` reaches `Key` and `ctrl+c` still quits.

### SS-9 · Logs surface · M

- **Target**: `internal/surfaces/logs/` (new — `logs.go` model, `format.go`
  event→row rendering, `search.go` regex search/highlight/nav),
  `internal/surfaces/registry.go` (`knownKinds["logs"]`),
  `internal/cli/surface_launch.go` (`surfaceModelFor` case `"logs"`),
  `config/seed/agentx.kdl` (new `logs` tab), `docs/ux/UX_LIFECYCLE.md`
  (traceability row)
- **Deps**: SS-8
- **Source**: `docs/ux/03_PANEL_DETAILS.md` PD-LOGS (PD-LOGS-AF-001..008 +
  GIVEN/WHEN/THEN contracts — already written)
- **Behavior**:
  1. `logs.Model` implements `client.SurfaceModel`. `Apply(ev state.Event)`
     formats each event to one logical (pre-wrap) line via `formatEvent` —
     `time.UnixMilli(ev.Epoch).Format("15:04:05.000")`, `ev.ContentType`,
     `ev.ToolName` when set, then a payload summary — and appends it to an
     internal entry buffer. Unlike the context surface, this view does **not**
     skip `ev.Ephemeral` events: it's a full activity log, not a conversation
     view.
  2. `SetSize(w, h)` recomputes the wrapped display lines for the new width
     via `scrollutil.WrapLines`; scroll-position/scrollbar math reuses
     `scrollutil.ClampInt`/`ScrollbarCell`/`PadTo` — the same primitives
     `output`/`workmemory` already share, no new wrap or scrollbar code.
  3. Auto-follow: while the scroll offset sits at the bottom, `Apply` keeps it
     pinned to the new bottom as events arrive (PD-LOGS-AF-002, the `tail -f`
     affordance); once the user scrolls up, new events still append to the
     buffer but the viewport doesn't jump — standard pager convention.
  4. `Key` handles `j`/`down`, `k`/`up` (line), `ctrl+d`/`ctrl+u` (half page),
     `pgdown`/`pgup` (full page), `g` `g` (`gg`, jump top — a one-shot
     pending-key flag reset on any other key), `G` (jump bottom, re-arms
     auto-follow), `/` and `?` (enter search-input mode, remembering
     direction), `n`/`N` (next/prev match, wrapping at the buffer ends),
     `esc` (clear the active search highlight, or cancel in-progress input).
     `CapturesKeys()` returns `true` exactly while `/`/`?` input is active
     (consumes SS-8).
  5. Search: pattern compiled with Go `regexp` (RE2 — the PD-LOGS-AF-004
     caveat already documents no backreference support); matches are computed
     over the *wrapped* display lines so highlighting and `n`/`N` line up with
     what's on screen. `View()` renders the visible slice plus a footer:
     `/pattern` while typing, `<match>/<total> matches` plus key hints
     (`/ search · n/N next/prev · gg/G top/bottom · q quit`) otherwise — same
     footer convention `contextviz` already uses.
  6. Wire-up: `registry.go`'s `knownKinds["logs"] = true`; a `case "logs":` in
     `surfaceModelFor` returning `logs.New(...)` with title `"logs"` (+
     session suffix per `LaunchTitleSession`, matching every other surface);
     a fourth `config/seed/agentx.kdl` tab running
     `agentx surface launch logs --session $AX_SESSION_STRING`.
- **Feature**: `tests/features/surfaces/logs_surface.feature`
  (`@functional @ux:PD-LOGS`), steps in `tests/steps/surfaces/logs_steps.go`
  mirroring `context_visualizer_steps.go`'s shape (build a `logs.Model`,
  `Apply` synthetic `state.Event`s, feed key presses, assert on `View()`)
  — one scenario per PD-LOGS-AF-002..008 (PD-LOGS-AF-001, full-tab placement,
  is a layout fact with no unit-level assertion, same treatment
  `context-visualizer`'s screen-real-estate affordance already gets).
- **Done**: applied events render as wrapped, timestamped lines; new events
  auto-follow until the user scrolls; `/pattern` and `?pattern` search
  highlights every match and `n`/`N` cycles them; `gg`/`G` jump to the buffer
  ends; `q`/`ctrl+c` quits cleanly, and mid-search-input only `ctrl+c` quits
  (`q` is a literal character); `make all` green.

### Sequencing

```
SS-8 (host input-capture mode) ─ SS-9 (logs surface) ─ INTEGRATION
```

Linear: the surface's search-input mode needs SS-8 before it can safely use
`"q"` inside a pattern.

---

## Future surfaces (out of scope for this slice)

Each reuses the SS-2 framework; each is its own increment with re-authored TUI specs.

| Surface | Legacy spec | Adds beyond the framework |
| --- | --- | --- |
| Files browser | PD-11 FileBrowser | filesystem read access; directory tree model |
| Config surface | PD-07 SettingsSurface | config read endpoint; later: write-back + restart-required semantics |
| Context-history | (new) | session-list + reload-from-log (ties to CTX-1 follow-up) |
| Context-visualizer (done, SS-7) | PD-10 ContextMeterWidget / PD-08 | read-only context-window accounting by content class; measured against the model's context length (`/api/show` → `num_ctx`). Enable/disable is **not** here — it belongs to the context pane (PD-CTX); the meter only hints. See PD-CTXVIZ. |
| Working-memory editor | PD-03 (Working Memory) | working_memory.json read/edit over the transport |
| Log/trace surface (scoped, Phase G above) | (new — no legacy precedent; see PD-LOGS) | search/highlight/nav over the full event log; `client.Host` gains input-capture mode (SS-8) |

Also deferred: surface→surface coordination, attachment chips, plan/DAG visualizer,
and any Family-B orchestration surfaces.

## Milestone / AC mapping

- Phase F → **M2** (`AC-M2-1` in-scope affordance IDs mapped to tested status for the
  context surface; `AC-M2-2` no critical drift between the surface's documented
  transitions and its behavior; processing-state rendered without drift).
- Each task's Godog feature provides the `TC-M2-<area>-<nnn>` test-case IDs that fill
  the M2 AC coverage table at the checkpoint.
