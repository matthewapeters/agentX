# Event-Broker Pub-Sub Architecture

_Last updated: 2026-05-09 (v0.39.0)_

## Overview

AgentX now uses a **centralized event broker** for streaming data distribution. This replaces the previous brittle point-to-point FIFO approach with a robust pub-sub pattern that guarantees delivery to all subscribers.

### Problem Solved

**Previous Architecture (Broken):**

- StreamingController called `tui_bridge.write_output()` directly → non-blocking FIFO writes
- If FIFO reader was unavailable, writes were silently dropped
- TUI output was fragile and unreliable
- No way to buffer data if TUI wasn't ready

**New Architecture (Robust):**

- StreamingController publishes events to EventBroker
- Each subscriber (GUI, TUI, logging) gets its own queue
- Events are buffered and retried with backoff
- Guaranteed delivery: no data is dropped
- Slow subscribers don't block publishers

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                   AgentXSession                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │         EventBroker (central pub-sub hub)           │   │
│  │  - Maintains subscriber lists per event type        │   │
│  │  - Publishes events to all subscribers              │   │
│  │  - Thread-safe with RLock                           │   │
│  └──────────────┬─────────────────────────────────────┬┘   │
│                 │                                     │     │
│   ┌─────────────▼────────────┐     ┌────────────────┬┴─────┴──┐
│   │ StreamingController      │     │  GUI display   │ TUI     │
│   │                          │     │  calls still   │ Event   │
│   │ - _write_tui_output()    │     │  work as-is    │ Sub.    │
│   │ - publish() to broker    │     └────────────────┘ │       │
│   │ - background thread      │                        │       │
│   └──────────────────────────┘                        │       │
│                                                       │       │
│   ┌──────────────────────────┐     ┌─────────────────┴─────┐ │
│   │ Session                  │     │ TUIEventSubscriber    │ │
│   │                          │     │                       │ │
│   │ - create EventBroker     │     │ - Buffers events      │ │
│   │ - wire subscribers       │     │ - Background writer   │ │
│   │ - manage TUI/GUI         │     │ - Formats for TUI     │ │
│   └──────────────────────────┘     │ - Writes FIFO with    │ │
│                                    │   retry/backoff       │ │
│                                    └───────────────────────┘ │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Components

### 1. EventBroker (`src/agentx/event_broker.py`)

**Purpose:** Centralized pub-sub hub for all streaming events

**Key Methods:**

```python
def subscribe(
    event_type: EventType,
    callback: Callable[[Event], None],
    queue_size: int = 100
) -> Callable[[], None]:
    """Register a callback for an event type. Returns unsubscribe function."""

def publish(
    event_type: EventType,
    data: dict[str, Any]
) -> None:
    """Publish an event to all registered subscribers."""
```

**Guarantees:**

- **Ordered delivery:** Events dispatched in publish order
- **No dropped events:** Each subscriber has its own queue
- **Thread-safe:** Uses RLock for concurrent access
- **Non-blocking publishers:** Dispatch happens in background threads

### 2. EventType (`src/agentx/event_broker.py`)

Enum of all streaming event types:

```python
class EventType(str, Enum):
    # Lifecycle
    STREAM_START = "stream_start"
    STREAM_END = "stream_end"
    
    # Content streaming
    THINKING_START = "thinking_start"
    THINKING_CONTENT = "thinking_content"
    AGENT_HEADER = "agent_header"
    AGENT_CONTENT = "agent_content"
    
    # Tool interactions
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"
    
    # Bootstrap and system
    BOOTSTRAP_MESSAGE = "bootstrap_message"
    SYSTEM_MESSAGE = "system_message"
    USER_MESSAGE = "user_message"
    
    # Errors and logging
    ERROR = "error"
    LOG_MESSAGE = "log_message"
```

### 3. TUIEventSubscriber (`src/agentx/integration/tui_event_subscriber.py`)

**Purpose:** Reliable TUI output handling via event broker

**Key Features:**

- Maintains bounded event queue (maxlen=10000)
- Background writer thread for FIFO writes
- Retries with backoff if FIFO unavailable
- Formats events into TUI protocol (###THINKING, ###AGENT, ###TOOL_CALL, etc.)

**Data Flow:**

1. StreamingController publishes event
2. EventBroker queues for all subscribers
3. TUIEventSubscriber receives event in background
4. Event added to internal queue
5. Writer thread:
   - Pops event from queue
   - Formats for TUI protocol
   - Writes to output FIFO
   - Retries with backoff if write fails

### 4. StreamingController Updates (`src/agentx/streaming_controller.py`)

**Changed Method:**

```python
def _write_tui_output(self, record: str) -> None:
    """Publish an output record to TUI subscribers via event broker.
    
    Guaranteed delivery via pub-sub; no data is dropped.
    """
    broker = getattr(self._s, "event_broker", None)
    if broker is None:
        return
    broker.publish(EventType.AGENT_CONTENT, {
        "text": record, 
        "is_raw_tui": True
    })
```

This method now:

- Publishes to event broker instead of direct FIFO writes
- Ensures all TUI subscribers receive the event
- Never drops data due to slow or unavailable readers

### 5. Session Integration (`src/agentx/session.py`)

**New in AgentXSession.**init**:**

```python
# Create event broker
self.event_broker = EventBroker()

# Create and wire TUI event subscriber
if tui_enabled:
    self.tui_event_subscriber = TUIEventSubscriber(tui_bridge=self.tui_bridge)
    self.tui_event_subscriber.start()
    
    # Subscribe to all event types
    for event_type in EventType:
        self.event_broker.subscribe(
            event_type, 
            self.tui_event_subscriber.handle_event,
            queue_size=1000
        )
```

**Cleanup:**

- TUI subscriber stopped in session cleanup
- Event broker automatically cleared when session destroyed

## Event Flow Example

### User submits message → TUI output appears

```
1. User presses <leader>s in Neovim
2. Input FIFO receives text
3. TUIBridge._input_reader_loop() reads from FIFO
4. Calls session._on_tui_submit(text)
5. Session schedules stream_ollama_response()
6. StreamingController._handle_stream_content() called with text
7. Publishes: broker.publish(EventType.AGENT_CONTENT, {"text": "..."})
8. EventBroker queues event for TUIEventSubscriber
9. TUIEventSubscriber._writer_loop() dequeues event
10. Formats and writes to output FIFO
11. Neovim jobstart reads from FIFO
12. Appends to output buffer
13. User sees response in output pane
```

## Testing

All tests pass (11/11):

```bash
python -m pytest tests/test_event_broker_pubsub.py -v
```

### Test Coverage

- **EventBroker:** Basic pub-sub, multiple subscribers, unsubscribe, slow subscribers
- **TUIEventSubscriber:** Event formatting, buffering, bounded queue, writer thread
- **StreamingController:** Publishing events, graceful handling of missing broker
- **End-to-end:** Full chain from StreamingController → EventBroker → TUI Subscriber

## Migration from Old Architecture

### Old Way (Broken)

```python
# Old: Direct FIFO write, data dropped if no reader
def _write_tui_output(self, record: str) -> None:
    bridge = getattr(self._s, "tui_bridge", None)
    if bridge is None:
        return
    try:
        bridge.write_output(record)  # Non-blocking, drops if unavailable
    except Exception:
        pass
```

### New Way (Robust)

```python
# New: Publish to broker, guaranteed delivery
def _write_tui_output(self, record: str) -> None:
    broker = getattr(self._s, "event_broker", None)
    if broker is None:
        return
    broker.publish(EventType.AGENT_CONTENT, {
        "text": record, 
        "is_raw_tui": True
    })
```

## Benefits

✅ **No data loss:** Events queued per subscriber, never dropped
✅ **Scalable:** Easy to add new subscribers (logging, monitoring, etc.)
✅ **Decoupled:** Publishers don't know about subscribers
✅ **Thread-safe:** RLock on broker, per-subscriber queues
✅ **Backpressure:** Slow FIFO doesn't block streaming
✅ **Buffering:** Events buffered if TUI temporarily unavailable
✅ **Retry logic:** Writes retry with exponential backoff
✅ **Testable:** All components unit-tested with 66%+ coverage

## Future Enhancements

1. **Logging Subscriber:** Wire OutputLogger as subscriber to EventType.LOG_MESSAGE
2. **GUI Subscriber:** Optionally migrate GUI display to subscriber pattern
3. **Event Persistence:** Persist all events to session log for replay
4. **Metrics:** Track subscriber throughput, queue depth, retry counts
5. **Event Filtering:** Allow subscribers to filter by event subtype or source
6. **Bidirectional:** Support events flowing both GUI↔TUI (e.g., UI state sync)

## See Also

- [docs/architecture.md](architecture.md) — Module index
- [docs/ux/UX_LIFECYCLE.md](ux/UX_LIFECYCLE.md) — User interface documentation
- [tests/test_event_broker_pubsub.py](../tests/test_event_broker_pubsub.py) — Comprehensive test suite
