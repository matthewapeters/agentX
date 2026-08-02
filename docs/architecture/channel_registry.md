# Channel Registry Architecture

_Last updated: 2026-05-28 (v1.0.1)_

## Purpose

The **Channel Registry** is the authoritative mapping and policy layer for all event-driven communication channels in AgentX. It defines, documents, and governs the set of named channels (event types), their schemas, pub/sub wiring, and delivery policies. This registry is the single source of truth for channel semantics, ensuring architectural traceability across all runtime surfaces.

## Channel Table

| Channel Name         | Event Type             | Schema (data keys)         | Publisher(s)                | Subscriber(s)                | Delivery Policy                |
|---------------------|-----------------------|----------------------------|-----------------------------|------------------------------|-------------------------------|-----------|
| stream_start        | STREAM_START          | -                          | Response stream coordinator | All runtime surfaces         | Ordered, atomic, all receive  |
| stream_end          | STREAM_END            | -                          | Response stream coordinator | All runtime surfaces         | Ordered, atomic, all receive  |
| thinking_start      | THINKING_START        | model_name                 | Response stream coordinator | All runtime surfaces         | Ordered, atomic, all receive  |
| thinking_content    | THINKING_CONTENT      | text                       | Response stream coordinator | All runtime surfaces         | Ordered, atomic, all receive  |
| agent_header        | AGENT_HEADER          | model_name                 | Response stream coordinator | All runtime surfaces         | Ordered, atomic, all receive  |
| agent_content       | AGENT_CONTENT         | text                       | Response stream coordinator, Session | All runtime surfaces         | Ordered, atomic, all receive  |
| tool_call           | TOOL_CALL             | tool_name, tool_input, ... | Response stream coordinator | All runtime surfaces         | Ordered, atomic, all receive  |
| tool_result         | TOOL_RESULT           | tool_name, output, ...     | Response stream coordinator | All runtime surfaces         | Ordered, atomic, all receive  |
| bootstrap_message   | BOOTSTRAP_MESSAGE     | message                    | Session                     | All runtime surfaces         | Ordered, atomic, all receive  |
| system_message      | SYSTEM_MESSAGE        | message                    | Session                     | All runtime surfaces         | Ordered, atomic, all receive  |
| user_message        | USER_MESSAGE          | text, timestamp            | Session                     | All runtime surfaces         | Ordered, atomic, all receive  |
| error               | ERROR                 | message                    | Any                         | All runtime surfaces         | Ordered, atomic, all receive  |
| log_message         | LOG_MESSAGE           | message, level, ...        | Any                         | Logging                      | Ordered, atomic, all receive  |
| processing_state    | PROCESSING_STATE      | session_id, state, phase, prompt_cycle | Runtime orchestrator | All runtime surfaces         | Session-scoped, read-mostly, low-frequency updates |

## Channel Schema and Policy

- **Schema**: Each channel (EventType) has a defined payload schema (see table above). All publishers must conform to this schema.
- **Policy**: All channels are delivered to all registered subscribers, in order, with atomic delivery. Slow or blocked subscribers do not affect others (per-subscriber queues).
- **Extensibility**: New channels must be registered here, with schema, publisher, subscriber, and policy documented before implementation.

### Processing State Contract (Authoritative)

The runtime must expose a shared processing-state contract so all surfaces can render consistent "working" indicators without owning independent orchestration logic.

Processing state schema requirements:

- `session_id`: active runtime session id.
- `state`: one of `idle`, `working`, `awaiting_input`, `completed`, `failed`.
- `phase`: one of `thinking`, `tool`, `respond`, `planning`, `verb`,
  `output_size`, `none` (`classify` remains a valid enum value but the live
  loop, `internal/runtime/loop.go`, never sets it — see
  `../implementation/04_llm_prompt_tooling_runtime.md`).
- `prompt_cycle`: full phase object for deterministic consumers.

Traffic policy requirements:

- Avoid per-surface bespoke state streams for basic processing indication.
- Prefer one session-level feed consumed by multiple surfaces and components.
- Keep update frequency low and payload compact to minimize surface overhead.

## Pub/Sub Wiring

- **Publishers**: Components that emit events (response coordinator, session, etc.)
- **Subscribers**: All runtime surfaces (output, input, system, logs, etc.), logging, and any future consumers.
- **Wiring**: Subscribers register callbacks for specific event types.

## Implementation References

For implementation details, see the dedicated implementation documentation folder.

## Change Policy

- All changes to channel definitions, schemas, or policies must be reflected in this document.
- This doc is the single source of truth for channel registry and must be kept in sync with runtime implementation.
