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

## Non-Goals for v1

- Public remote access over internet
- Multi-node distributed runtime
- Long-lived authenticated cross-machine surface sessions
