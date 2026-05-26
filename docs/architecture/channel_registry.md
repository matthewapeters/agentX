# Channel Registry Architecture

_Last updated: 2026-05-16 (v0.39.4)_

## Purpose

The **Channel Registry** is the authoritative mapping and policy layer for all event-driven communication channels in AgentX. It defines, documents, and governs the set of named channels (event types), their schemas, pub/sub wiring, and delivery policies. This registry is the single source of truth for channel semantics, ensuring architectural traceability and parity across GUI, TUI, and logging surfaces.

## Channel Table

| Channel Name         | EventType Enum         | Schema (data keys)         | Publisher(s)                | Subscriber(s)                | Delivery Policy                | Code Link |
|---------------------|-----------------------|----------------------------|-----------------------------|------------------------------|-------------------------------|-----------|
| stream_start        | STREAM_START          | -                          | StreamingController         | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| stream_end          | STREAM_END            | -                          | StreamingController         | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| thinking_start      | THINKING_START        | model_name                 | StreamingController         | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| thinking_content    | THINKING_CONTENT      | text                       | StreamingController         | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| agent_header        | AGENT_HEADER          | model_name                 | StreamingController         | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| agent_content       | AGENT_CONTENT         | text, is_raw_tui?          | StreamingController, Session| GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| tool_call           | TOOL_CALL             | tool_name, tool_input, ... | StreamingController, Bridge | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| tool_result         | TOOL_RESULT           | tool_name, output, ...     | StreamingController, Bridge | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| bootstrap_message   | BOOTSTRAP_MESSAGE     | message                    | Session                     | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| system_message      | SYSTEM_MESSAGE        | message                    | Session                     | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| user_message        | USER_MESSAGE          | text, timestamp            | Session                     | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| error               | ERROR                 | message                    | Any                         | GUI, TUI, logging            | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |
| log_message         | LOG_MESSAGE           | message, level, ...        | Any                         | Logging                      | Ordered, atomic, all receive  | [event_broker.py](../../src/agentx/event_broker.py) |

## Channel Schema and Policy

- **Schema**: Each channel (EventType) has a defined payload schema (see table above). All publishers must conform to this schema.
- **Policy**: All channels are delivered to all registered subscribers, in order, with atomic delivery. Slow or blocked subscribers do not affect others (per-subscriber queues).
- **Extensibility**: New channels must be registered here, with schema, publisher, subscriber, and policy documented before implementation.

## Pub/Sub Wiring

- **Publishers**: Components that emit events (StreamingController, Session, Bridge, etc.)
- **Subscribers**: GUI, TUI (TUIEventSubscriber), logging, and any future consumers.
- **Wiring**: Subscribers register callbacks for specific EventTypes via EventBroker.subscribe().

## Code Links

- EventType enum and broker: [src/agentx/event_broker.py](../../src/agentx/event_broker.py)
- TUI subscriber: [src/agentx/integration/tui_event_subscriber.py](../../src/agentx/integration/tui_event_subscriber.py)
- StreamingController: [src/agentx/streaming_controller.py](../../src/agentx/streaming_controller.py)

## Change Policy

- All changes to channel definitions, schemas, or policies must be reflected in this document and in the EventType enum.
- This doc is the single source of truth for channel registry and must be kept in sync with code and tests.
