# Surface Orchestration and HTTP Transport

## Objective

Define how runtime surfaces are created, addressed, and synchronized using an initial HTTP transport model.

## Surface Model

Surface means a user-facing runtime area such as:

- Primary TUI (Bubbletea)
- Optional system surfaces (files, context, settings, logs)
- Future browser-based surfaces

Implementation guidance:

- Treat each surface as a client of canonical runtime state
- Keep one shared event coordination layer and processing state source
- Avoid per-surface private orchestration logic
- Run surfaces as separate processes managed by orchestrator (orchestrator provides startup script with orchestrator and surface endpoint and session info)
- Support external terminal-session launch and runtime registration for child surfaces

## Port Allocation

Runtime behavior:

- agentx allocates ports at startup for enabled surfaces from configured ranges
- allocated ports are registered in runtime memory and published to local metadata
- transport failures on required surfaces block startup
- every candidate port is checked for availability before assignment
- allocator must tolerate concurrent AgentX instances on same host

Suggested allocation policy:

- configurable port range map in runtime config
- deterministic preference by surface name where possible
- conflict-aware fallback to next available port in allowed range

## HTTP API Baseline (v1)

CLI launch contract (v1):

- canonical:
  - agentx surface launch <surface-name> --session <session-name-or-id> --connect <endpoint> --token <attach-token>
- compatibility alias:
  - agentx --launch|-l <surface-name> --session|-s <session-name-or-id> --port|-p <port>

Launch behavior:

- main session prints launch command strings for user execution in other terminal sessions.
- <endpoint> should be preferred over raw port for forward compatibility.
- if only port is provided, runtime maps it to local endpoint.

## Normative CLI Specification (v1)

This section is normative for implementation and QA.

### Command Forms

Canonical launch form:

- agentx surface launch <surface-name> --session <session-name-or-id> --connect <endpoint> --token <attach-token>

Compatibility alias form:

- agentx --launch|-l <surface-name> --session|-s <session-name-or-id> --port|-p <port>

### Argument Contract

Required arguments (canonical):

- surface-name
- session (name or id)
- connect (endpoint)
- token (ephemeral attach token)

Optional behavior:

- session may be either session_name or session_id
- port may be used only in compatibility alias form

Validation rules:

1. surface-name must match a known surface type in runtime registry.
2. session must resolve to exactly one active session.
3. connect endpoint must use local-safe transport address in v1 policy.
4. token must pass orchestrator attach-token validation.
5. alias form with port must map to endpoint before registration request.

### Success and Failure Outcomes

Success criteria:

- command exits with status 0
- surface process starts and registers with orchestrator
- surface appears in GET /surfaces with lifecycle_state ready

Failure criteria:

- invalid arguments or unresolved session: non-zero exit
- invalid or missing token: non-zero exit and registration rejected
- endpoint unreachable: non-zero exit with connect failure message
- duplicate/conflicting attach attempt: non-zero exit with conflict message

### Required Operator Messages

On success:

- print surface_id
- print resolved session_id and session_name
- print connected endpoint

On failure:

- print deterministic reason category (validation, auth, transport, conflict)
- print actionable remediation hint

### Example Outcomes

Success example:

- Input: canonical launch with valid session, endpoint, and token
- Output: surface registration accepted and lifecycle transitions to ready

Failure example A:

- Input: canonical launch with invalid token
- Output: registration rejected with auth category and non-zero exit

Failure example B:

- Input: alias launch with unknown session name
- Output: session resolution failure with validation category and non-zero exit

### Gherkin Behavior Contracts

Use-case: Launch child surface with canonical command

- GIVEN an active orchestrator session with valid attach token
- WHEN user runs canonical surface launch command in a new terminal session
- THEN surface registers to that session and reports lifecycle_state ready

Variant: Launch child surface with compatibility alias

- GIVEN an active orchestrator session and a valid port mapping
- WHEN user runs compatibility alias launch command
- THEN runtime resolves endpoint and registers surface successfully

Use-case: Reject attach without valid token

- GIVEN an active session and an invalid or missing attach token
- WHEN user runs surface launch command
- THEN orchestrator rejects registration and process exits non-zero

Use-case: Reject ambiguous session selector

- GIVEN multiple potential matches for provided session selector
- WHEN user runs surface launch command
- THEN command fails with deterministic validation error requiring explicit selector

Read endpoints:

- GET /health
- GET /processing-state
- GET /surfaces
- GET /sessions/current

Write endpoints:

- POST /prompt
- POST /surface/{id}/command
- POST /tool/approval
- POST /model/switch
- POST /surface/register
- POST /surface/{id}/shutdown

Streaming endpoints:

- GET /events (SSE for v1)

Transport directionality:

- Orchestrator and surfaces require bidirectional communication semantics.
- v1 uses SSE for orchestrator-to-surface event streaming.
- v1 uses HTTP request-response endpoints for surface-to-orchestrator commands and callbacks.
- upgraded streaming protocol can be introduced after v1 evaluation.

Protocol note:

- Bubble Tea does not prescribe a network transport protocol.
- AgentX standardizes on SSE in v1 and will evaluate alternatives post-v1 based on measured needs.

## Processing State Contract

Canonical fields (from architecture channel registry contract):

- session_id
- state: idle | working | completed | failed
- phase: classify | thinking | tool | respond | none
- prompt_cycle: structured phase details for deterministic consumers

Policy:

- publish one session-level processing-state stream
- all surfaces consume the same model
- low-frequency updates for status, high-frequency details via event stream

## Surface Registration Contract

Each surface should publish:

- surface_id
- surface_kind
- transport_address
- capabilities
- lifecycle_state
- session_id
- session_name
- attach_token_fingerprint

Suggested lifecycle states:

- provisioning
- ready
- degraded
- stopped

Attach security contract (v1):

- child surface registration requires an ephemeral attach token.
- token is generated by orchestrator per session and validated at registration.
- registration attempts without valid token are rejected.

## Registration Mechanics (TRN-1)

This section refines the Surface Registration Contract and Attach security contract
above with the implementation mechanics for the registry and attach token. It adds
detail; it does not change the frozen `surface-registration.schema.json` payload.

### Attach token

- The orchestrator **mints one ephemeral attach token per session at Start**, after
  the session identity exists and before prompts are accepted.
- The raw token is a high-entropy random string. It is **held only in orchestrator
  memory and never persisted** and never written to the event log or session
  metadata.
- Only the token **fingerprint** — `sha256` of the raw token, hex-encoded, truncated
  to a fixed-width prefix — is persistable/publishable. The registry stores the
  fingerprint (not the raw token) on each accepted registration, satisfying the
  `attach_token_fingerprint` field.
- Validation is constant-time comparison of the presented raw token against the
  session token; a mismatch or empty token is rejected.

### Registry API (`internal/surfaces/registry.go`)

- `Register(req)` validates the presented token, assigns/accepts the `surface_id`,
  records the 8 frozen fields, sets `lifecycle_state: ready`, and returns the stored
  record. A second registration for an existing, non-stopped `surface_id` is a
  **conflict** (rejected). Re-registering a `stopped` id is permitted.
- `Shutdown(surface_id)` transitions a registered surface to `stopped`.
- `List()` returns the current registrations (for `GET /surfaces`), ordered
  deterministically by `surface_id`.
- Rejections carry a deterministic reason category (`validation | auth | conflict`)
  so transport handlers (TRN-3) and the CLI (TRN-5) can map them to messages/exit
  codes.

### Lifecycle

```
register(valid token)  → ready
register(stopped id)   → ready        (re-attach allowed)
shutdown(id)           → stopped
register(invalid token)→ (rejected, auth)        no record created
register(dup ready id) → (rejected, conflict)    existing record unchanged
```

`provisioning` and `degraded` are reserved by the frozen schema for future
multi-step attach / health degradation; v1 registration transitions straight to
`ready` and does not emit `degraded`.

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Register a surface with a valid attach token

- GIVEN an orchestrator session with a minted attach token
- WHEN a surface registers presenting the valid token
- THEN the registry stores the record with `lifecycle_state: ready` and the
  `attach_token_fingerprint` matches the session token fingerprint

Use-case: Reject registration without a valid token

- GIVEN an orchestrator session with a minted attach token
- WHEN a surface registers presenting a missing or wrong token
- THEN registration is rejected with category `auth` and no record is created

Use-case: Reject a conflicting surface id

- GIVEN a surface already registered and `ready`
- WHEN a second registration presents the same `surface_id` with a valid token
- THEN registration is rejected with category `conflict` and the existing record is
  unchanged

Use-case: Shut a surface down

- GIVEN a registered, `ready` surface
- WHEN the orchestrator shuts that surface down
- THEN its `lifecycle_state` becomes `stopped` and it may re-register later

Use-case: Raw token is never exposed by the registry

- GIVEN an accepted registration
- WHEN the registry record is read back (e.g. via `GET /surfaces`)
- THEN it carries only the non-secret fingerprint, never the raw token

## Read & Streaming Server (TRN-2)

This section refines the HTTP API Baseline (read + streaming endpoints) and
Transport directionality above with the implementation mechanics for the loopback
server. It adds detail; it does not change the endpoint contract.

### Provider seam

The HTTP transport (`internal/transport/http`) must not import `internal/runtime`
(Import Direction Matrix). It depends on a local `Provider` interface describing the
orchestrator surface it adapts; the concrete `*runtime.Orchestrator` satisfies it and
is injected by `internal/app`:

```
Provider:
  Bus() *state.Bus                         // event fan-out for /events
  Processing() *state.ProcessingPublisher  // snapshot for /processing-state
  Session() session.Identity               // /sessions/current
  Registry() *surfaces.Registry            // /surfaces
```

The server owns no orchestration logic; every handler is a read/stream adapter over
the canonical state the orchestrator already publishes.

### Read endpoints

- `GET /health` → `200` with `{"status":"ok","session_id":"<id>"}`.
- `GET /processing-state` → JSON of `Processing().Current()` (conforms to
  `processing-state.schema.json`).
- `GET /surfaces` → JSON array of `Registry().List()` (each conforms to
  `surface-registration.schema.json`), deterministically ordered by `surface_id`.
- `GET /sessions/current` → JSON of the active `session.Identity`.

All read responses are `application/json`.

### Streaming endpoint (`GET /events`, SSE)

- Response is `text/event-stream` (`Cache-Control: no-cache`). The handler obtains a
  fresh bus subscription per connection, streams each event as an SSE frame
  (`event: <content_type>` + `data: <event-envelope JSON>`), and flushes after each.
- The connection ends when the client disconnects (`r.Context().Done()`) or the bus
  subscription closes; the subscription is always closed on return.
- **Fan-out guarantee:** each SSE connection is an independent bus subscriber with its
  own ordered queue, so a slow or stalled SSE consumer never blocks the publisher or
  other surfaces (the per-subscriber queue semantics in `internal/state`).

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Health check

- GIVEN a running transport server for a session
- WHEN a client GETs `/health`
- THEN the response is `200` with `status: ok` and the session id

Use-case: Read current processing state

- GIVEN a running transport server whose session is `working`/`respond`
- WHEN a client GETs `/processing-state`
- THEN the JSON reports `state: working` and `phase: respond`

Use-case: List attached surfaces

- GIVEN a running transport server with a registered surface
- WHEN a client GETs `/surfaces`
- THEN the JSON array includes that surface's `surface_id`

Use-case: Stream events over SSE

- GIVEN a running transport server with an open `/events` stream
- WHEN an event is published on the bus
- THEN the stream delivers that event as an SSE frame

Use-case: Concurrent streams both receive an event

- GIVEN two open `/events` streams on the same server
- WHEN an event is published
- THEN both streams deliver it (no consumer blocks another)

## Write Server (TRN-3)

This section refines the HTTP API Baseline (write endpoints) with the implementation
mechanics for surface-to-orchestrator commands. It adds detail; it does not change
the endpoint contract.

### Authorization

- `POST /surface/register` authorizes via the **attach token in the request body**
  (it is the attach handshake) — see Registration Mechanics (TRN-1).
- Every other write authorizes via an `Authorization: Bearer <attach-token>` header,
  validated against the session token (`Registry.ValidateToken`). A missing/invalid
  bearer token is rejected `401` with category `auth`.

### Endpoints

- `POST /surface/register` → `201` with the stored `Registration` JSON on success;
  on rejection, the registry's reason category maps to status: `auth → 401`,
  `validation → 400`, `conflict → 409`. The error body is
  `{"error":"<msg>","category":"<category>"}`.
- `POST /prompt` (`{"text":"…"}`) → `202 {"status":"accepted"}` after handing the
  prompt to the orchestrator. The cycle runs asynchronously; its events and
  processing-state transitions flow back over `GET /events` and
  `GET /processing-state`. Empty text → `400 validation`. When the orchestrator is
  not accepting (`Accepting()` false) → `409 conflict`. (v1: an external surface
  cannot cancel an in-flight cycle; Stop remains an in-process chat affordance.)
- `POST /tool/approval` (`{"decision":"approve_session|approve_global|deny"}`) →
  `200 {"status":"resolved"}`; the decision is forwarded to the orchestrator's
  approval gate (`Resolve`). Empty decision → `400 validation`.
- `POST /surface/{id}/shutdown` → `200 {"status":"stopped"}`; unknown id → `404`.
- `POST /surface/{id}/command` → reserved; validates the surface is registered then
  returns `501 not_implemented` in v1 (there is no inbound channel to an external
  surface process yet).
- `POST /model/switch` → `501 not_implemented` in v1 (live model-switch is deferred).

### Provider seam (extended)

TRN-3 widens the `Provider` interface the transport depends on with the
orchestrator's write surface:

```
Submit(ctx, text) error   // POST /prompt (run async)
Resolve(decision)         // POST /tool/approval
Accepting() bool          // gate POST /prompt
```

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Register over HTTP with a valid token

- GIVEN a running transport server
- WHEN a surface POSTs `/surface/register` with the valid attach token
- THEN the response is `201` and the body reports `lifecycle_state: ready`

Use-case: Reject an unauthorized write

- GIVEN a running transport server
- WHEN a client POSTs `/prompt` without a valid bearer token
- THEN the response is `401` with category `auth`

Use-case: Prompt over HTTP drives a cycle

- GIVEN an authorized surface with an open `/events` stream
- WHEN it POSTs `/prompt` with text
- THEN the response is `202` and the resulting events arrive over the stream

Use-case: Reject a prompt when not accepting

- GIVEN an authorized surface and an orchestrator not accepting prompts
- WHEN it POSTs `/prompt`
- THEN the response is `409` with category `conflict`

Use-case: Approval over HTTP resolves the gate

- GIVEN an authorized surface
- WHEN it POSTs `/tool/approval` with a decision
- THEN the response is `200` and the orchestrator receives that decision

Use-case: Shut a surface down over HTTP

- GIVEN an authorized surface and a registered surface id
- WHEN it POSTs `/surface/{id}/shutdown`
- THEN the response is `200` and that surface's lifecycle becomes `stopped`

## Port Allocation & Endpoint Publication (TRN-4)

This section refines the Port Allocation policy above with the implementation
mechanics for the orchestrator's transport endpoint. In the reconciled Family A
model the orchestrator exposes **one** HTTP/SSE endpoint that external surfaces
attach to as clients (it does not allocate a port per surface).

### Configuration

A `[agentx.transport]` table controls the endpoint:

```toml
[agentx.transport]
enabled    = true
host       = "127.0.0.1"   # loopback only in v1
port_start = 8420
port_end   = 8460
```

- `enabled = false` keeps the pure in-process mode (no server bound); the escape
  hatch referenced by TRN-6.
- `host` is a local-safe address (loopback) in v1 (Non-Goals: no public access).
- `[port_start, port_end]` is the inclusive candidate range.

### Allocation

- Allocation **binds** the first available TCP port in the range ascending
  (deterministic preference = lowest free port), returning the bound listener. The
  bind itself is the availability check, so there is no time-of-check/time-of-use gap
  and concurrent `agentx` instances on the same host naturally fall through to the
  next free port.
- If every port in the range is occupied, allocation fails with a range-exhausted
  error; on a **required** transport this blocks startup with a clean error (TRN-6).
- The returned listener is handed to the server's `Serve` (TRN-6); nothing rebinds.

### Endpoint publication

- The resolved endpoint (`http://<host>:<port>`) is published to the session
  metadata as `sessions/<id>/transport.json` so tooling and launch flows can
  discover where to attach. The file carries the endpoint and session id only — the
  raw attach token is **never** written to metadata (it is printed to the operator's
  terminal by TRN-6).

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Allocate the lowest free port

- GIVEN a free port and a range whose only member is that port
- WHEN the allocator runs
- THEN it binds a listener on that port

Use-case: Fall back past an occupied port

- GIVEN an occupied port and a range starting at it
- WHEN the allocator runs
- THEN it binds a listener on a different free port within the range

Use-case: Range exhausted

- GIVEN an occupied port and a single-port range at it
- WHEN the allocator runs
- THEN allocation fails with a range-exhausted error

Use-case: Publish the endpoint

- GIVEN a created session
- WHEN the transport endpoint is published
- THEN the session transport metadata reports that endpoint

## Launch Implementation (TRN-5)

This section refines the Normative CLI Specification above with the implementation
mechanics. It adds detail; it does not change the CLI contract.

### Command parsing

- Canonical: `agentx surface launch <surface-name> --session <s> --connect <ep>
  --token <t>` (positional kind after `launch`, then flags).
- Compatibility alias: `agentx -l|--launch <kind> -s|--session <session>
  -p|--port <port>`; the alias maps `port` → `http://127.0.0.1:<port>` **before**
  registration (validation rule 5). Because v1 registration requires an attach
  token, the alias also accepts `-t|--token <token>` (an extension to the historical
  alias, which predates the attach-token contract).

### Validation order and reason categories

`cli.Launch` validates in this order, mapping each failure to a deterministic
category (and a non-zero exit):

1. `surface-name` is a known surface kind (`surfaces.KnownKind`) → else `validation`.
2. `session` selector is non-empty → else `validation`.
3. `connect` endpoint is a local-safe (loopback) address → else `validation`.
4. Reach the endpoint (`GET /sessions/current`) → unreachable → `transport`; then the
   selector must equal the running session's id or name → mismatch → `validation`
   (rule 2: resolve to exactly one active session).
5. Register (`POST /surface/register` with the token). The orchestrator's registry
   decides: invalid/empty token → `auth`; duplicate id → `conflict`.

### Attach client

A small HTTP client (`transport/http.Client`) performs `GET /sessions/current` and
`POST /surface/register`, decoding `{error,category}` bodies into a typed
`AttachError{Category,Message}`; connection failures become category `transport`. A
headless launch sets `transport_address` to the endpoint it attached through.

### Operator output

On success `cli.Launch` returns the `surface_id`, resolved `session_id` /
`session_name`, and connected endpoint, which the command prints (exit 0). On failure
the error carries the category and a remediation hint (exit non-zero).

## Seed + Resume (SS-1)

This section refines the Read/Streaming endpoints with the attach protocol a
rendering surface uses to mirror a session: seed from the durable log, then resume
the live stream by cursor. It adds detail; it does not change the endpoint contract.

### Event ordinal

Every event carries a per-session monotonic `ordinal`, stamped by the bus at publish
time (not at disk write), so the live event and its persisted copy share one
identity. The ordinal is the canonical total order and the resume cursor. (See the
`ordinal` field in `event-envelope.schema.json`.)

### Endpoints

- `GET /sessions/current/events` → the persisted session event log
  (`Provider.History`), the **seed**: authoritative and durable, carrying each
  event's `enabled` and `ordinal`.
- `GET /events?after=<ordinal>` (SSE) → the live stream **after** the cursor. The
  handler:
  1. subscribes to the bus, then captures a `boundary = Bus.CurrentOrdinal()`;
  2. serves `(after, boundary]` from the durable log (polling briefly for the
     recorder to persist up to the boundary, so there is no gap);
  3. serves `(boundary, ∞)` from the subscription.

  Because the boundary partitions history vs. live by ordinal, the handover has **no
  gap and no duplicate** and needs no client-side de-duplication. `after=0` yields the
  full stream (seed + live) in one connection.

### Client attach sequence

`transport/http.Client` exposes `Seed(ctx)` and `Subscribe(ctx, after)`. A surface:
seeds → renders the snapshot → notes the last ordinal → `Subscribe(ctx, last)` →
applies live events thereafter.

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Seed returns the durable log

- GIVEN a session with recorded events
- WHEN a surface GETs `/sessions/current/events`
- THEN it receives the events in ordinal order, each carrying its `enabled` and
  `ordinal`

Use-case: Resume after the seed delivers only newer events

- GIVEN a surface that seeded through ordinal N
- WHEN it subscribes with `after=N` and new events are published
- THEN it receives exactly the events with ordinal > N, in order, with no gap or
  duplicate

Use-case: Full stream from zero

- GIVEN a session with recorded events and a live publisher
- WHEN a surface subscribes with `after=0`
- THEN it receives the seed events followed by live events, each exactly once

## Serve-Alongside Lifecycle (TRN-6)

This section refines the Surface Model ("run surfaces as separate processes managed
by orchestrator") and Launch behavior ("main session prints launch command strings")
with the runtime wiring that makes `agentx` serve the transport.

### Lifecycle integration

- When `[agentx.transport] enabled` (the default), the orchestrator allocates a
  loopback port (TRN-4) and serves the HTTP/SSE server (TRN-2/3) as part of `Start`,
  **after** the event bus, processing-state publisher, registry, and recorder are
  live and before prompts are accepted. A bind failure blocks startup with a clean
  error.
- On `Shutdown` the server is stopped **first** (so no new external request arrives
  mid-drain), every attached surface is marked `stopped`, and then the normal
  recorder drain proceeds.
- `enabled = false` keeps the pure in-process mode: no port is bound, no endpoint is
  published, and the default chat surface still works over the in-process bridge.

### Launch-command emission

The chat boot path prints the operator hint (`transport/http.LaunchHint`) — the
resolved endpoint, a copy-pasteable `agentx surface launch <kind> …` template
carrying the endpoint and **raw attach token**, and the list of launchable external
kinds (`surfaces.ExternalKinds`) — so the user can attach surfaces from other
terminals. The raw token appears only in this terminal output, never in persisted
metadata.

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Serve the transport alongside the chat surface

- GIVEN an orchestrator started with the transport enabled
- WHEN a client requests `/health` on the published endpoint
- THEN it responds ok

Use-case: A launched surface round-trips a prompt

- GIVEN a running orchestrator serving the transport
- WHEN a surface attaches with the launch CLI and submits a prompt over the transport
- THEN the response streams back over the surface's event stream

Use-case: Shutdown stops the transport

- GIVEN a running orchestrator serving the transport with an attached surface
- WHEN the orchestrator shuts down
- THEN the endpoint becomes unreachable and the attached surface is marked stopped

## Surface Host (SS-2)

This section documents the shared client-side host that turns an attach into a
running TUI surface. It is the reusable framework every rendering surface (context,
files, config, …) builds on.

### SurfaceModel contract

A concrete surface implements a small contract; the host owns everything else:

```
SurfaceModel:
  Apply(ev state.Event)          // fold one session event into the projection
  SetSize(width, height int)     // inner render area (inside the host title strip)
  Key(msg tea.KeyPressMsg)       // surface-specific keys (scroll, …)
  View() string                  // render the body
```

### Host lifecycle

`internal/surfaces/client.Host` is the Bubble Tea model wrapping a `SurfaceModel`:

1. **Seed**: `Init` applies the durable seed snapshot (decision SS-1) before any live
   event, so the surface renders the full session immediately.
2. **Live**: it then listens on the resumed stream (`Subscribe(after: lastOrdinal)`),
   applying each `EventMsg` and re-arming the listen command — the same channel-read
   idiom the chat surface uses.
3. **Resize**: `WindowSizeMsg` sets the surface's inner size.
4. **Quit**: a quit key (`Ctrl-C`/`q`) invokes the shutdown hook
   (`POST /surface/{id}/shutdown`, lifecycle → `stopped`) then quits; a closed stream
   (orchestrator gone) also quits.

`client.Run` wires the attach: it seeds, subscribes after the seed cursor, and runs
the program. Non-quit keys are forwarded to the surface.

### Launch dispatch

`agentx surface launch <kind>` (`cli.RunSurface`) registers via `cli.Launch` (TRN-5),
then dispatches by kind: a kind with a registered `SurfaceModel` runs its TUI;
a kind without one yet attaches headless and prints the registration. Surfaces
register in `surfaceModelFor` as they land (the context viewer in SS-3). The
dispatch lives in `internal/cli` (not `cmd/agentx`) to honor the import matrix.

## Connection liveness (SS-4)

The launch-info widget (and any future presence indicator) needs to know which peer
surfaces are *actually attached*, not merely which ever registered. Registration is
durable (added on `POST /surface/register`, removed only on explicit shutdown), so it
cannot answer "is it connected right now?" — a crashed surface would look attached
forever. Liveness is therefore tied to the **event stream**, the one long-lived link a
surface holds:

- The subscribe request carries the surface id: `GET /events?after=N&surface_id=<id>`.
- `handleEvents` calls `Registry.MarkLive(id)` when the stream opens and, via `defer`,
  `Registry.MarkDead(id)` when it closes. A clean quit, a crash, or a `kill -9` all end
  the HTTP request the same way (the TCP connection drops and the handler returns), so
  `defer` covers every disconnect without a heartbeat.
- The registry keeps a per-surface live-stream count (a surface may briefly hold two
  streams across a reconnect) and exposes `ConnectedKinds()` — the sorted, unique set
  of kinds with at least one live stream, intersected with current registrations.
- Consumers read the snapshot by polling (`ConnectedKinds()` is cheap and lock-guarded)
  rather than subscribing, so a stalled reader never affects the stream path. The chat
  surface polls on a ~1s tick and updates the launch-info row emojis.

`surface_id` is optional on `/events`; an omitted id (e.g. a seed-only attach) simply
isn't tracked for liveness. Loopback-only v1 does not authenticate the stream beyond
this, so liveness is advisory, not a security boundary.

## Flagless launch & token discovery (SS-5)

Because v1 is loopback-only, a peer surface always runs on the same machine as the
orchestrator, so both processes can read the session directory. The orchestrator
therefore publishes everything a peer needs to attach, and the launch CLI discovers
it — no copying a token between panes:

- `transport.json` (0644 metadata): session id + endpoint.
- `attach-token` (0600 secret): the raw ephemeral attach token, written beside it at
  startup and removed on shutdown. This lives in the same trust domain as the
  plaintext event log already in the session dir, so persisting the loopback token
  does not lower the security boundary — a reader of this `0600` file inside the
  `0700` session dir already has the user's UID.

`agentx surface launch <kind>` with no `--connect/--token` auto-resolves from the
session root on disk. Resolution considers only sessions whose server is reachable:

- `--session <name|id>` attaches to the **matching** session (so it is correct with
  several running); a non-match errors with the list of running sessions.
- With **no** selector, a single reachable session is unambiguous and is used; with
  **multiple**, resolution refuses to guess and errors (`pass --session <name>`) so a
  surface never silently attaches to the wrong session.
- Explicit `--connect`/`--token` still override everything.

`transport.json` therefore carries `session_name` (alongside `session_id` + endpoint)
so a launcher can match on the human-friendly name. The launch-info widget displays
the session name and advertises `agentx surface launch <kind> --session <name>` — still
token-free and SSH-typeable, but unambiguous when more than one agentx session runs.

## Working-Memory CRUD (WM surface, SS-6)

The working-memory editor is the first **read-write** surface. It reads and mutates
the session's `working_memory.json` through dedicated, typed endpoints (not the
reserved generic command relay):

- `GET /working-memory` → `{ "facts": [ {key, value, owner, enabled}, … ] }`. A
  loopback read, not token-gated (consistent with the other GET endpoints).
- `POST /working-memory/set` `{key, value}` — **upsert**: a new key is added
  (user-owned, enabled); an existing key's value is updated, preserving owner/enabled.
- `POST /working-memory/delete` `{key}` — remove (an unknown key is a no-op success).
- `POST /working-memory/enabled` `{key, enabled}` — enable/disable (unknown key → 404).

Mutations are token-gated (`Authorization: Bearer <attach-token>`) and persist to
`working_memory.json`. Because the orchestrator re-reads working memory when it
assembles each prompt (`withContext` → `workingMemoryMessage`), an edit or
enable/disable takes effect on the **next** prompt — only enabled facts fold into the
context. WM is a document, not an event stream, so the surface reads on attach and
polls for live refresh rather than subscribing.

## Non-Goals for v1

- Public remote access over internet
- Multi-node distributed runtime
- Long-lived authenticated cross-machine surface sessions
