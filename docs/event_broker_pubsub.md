# Event-Broker Pub-Sub Architecture

_Last updated: 2026-06-23 (v0.39.4)_

## Overview

AgentX uses a **centralized event coordination layer** for streaming data distribution. This replaces point-to-point coupling with a robust pub-sub pattern that guarantees delivery to all subscribers.

### Problem Solved

**Previous Architecture (Issues):**

- Response streaming directly called individual surface writers (non-blocking FIFO writes)
- If a surface reader was unavailable, writes were silently dropped
- Surface output was fragile and unreliable
- No buffering if a surface wasn't ready

**New Architecture (Robust):**

- Response stream coordinator publishes events to the coordination layer
- Each subscriber (output surface, input surface, system surface, etc.) gets its own queue
- Events are buffered and retried with backoff
- Guaranteed delivery: no data is dropped
- Slow subscribers don't block the response stream

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                   Session Orchestrator                      │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │   Event Coordination Layer (central pub-sub hub)     │   │
│  │  - Maintains subscriber lists per event type        │   │
│  │  - Publishes events to all subscribers              │   │
│  │  - Thread-safe with concurrent access control       │   │
│  └──────────────┬─────────────────────────────────────┬┘   │
│                 │                                     │     │
│   ┌─────────────▼────────────┐     ┌────────────────┬┴────┐
│   │ Response Stream          │     │  Output        │ Logging │
│   │ Coordinator              │     │  Surface       │ Consumer │
│   │                          │     │                │        │
│   │ - publish() to layer     │     │  - Buffers     │        │
│   │ - background thread      │     │  - Processes   │        │
│   │ - manages flow control   │     │  - Displays    │        │
│   └──────────────────────────┘     └────────────────┘        │
│                                                             │
│   ┌──────────────────────────┐     ┌─────────────────────┐ │
│   │ Session Manager          │     │ Input Surface       │ │
│   │                          │     │ Subscriber          │ │
│   │ - create coord. layer    │     │                     │ │
│   │ - wire subscribers       │     │ - Buffers events    │ │
│   │ - manage all surfaces    │     │ - Processes input   │ │
│   └──────────────────────────┘     │ - Handles backoff   │ │
│                                    └─────────────────────┘ │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Components

### 1. Event Coordination Layer

**Purpose:** Centralized pub-sub hub for all streaming events

**Key Capabilities:**

- Register callbacks for event types
- Publish events to all registered subscribers
- Maintain per-subscriber event queues
- Guarantee ordered delivery
- Handle backoff and retry on subscriber saturation

**Guarantees:**

- **Ordered delivery:** Events dispatched in publish order
- **No dropped events:** Each subscriber has its own queue
- **Thread-safe:** Concurrent access control
- **Non-blocking publishers:** Publish returns immediately after enqueueing

### 2. Event Types

Standardized event types for streaming communication:

- Response stream start/end
- Response thinking markers
- Response content (agent messages, tool calls/results)
- Session markers (bootstrap, user input, system messages)
- Error events
- Logging events
- Processing state updates

See [Channel Registry](architecture/channel_registry.md) for complete event schema definitions.

## Event Flow

### Publish Path

1. Response coordinator generates events
2. Publishes to event coordination layer
3. Coordination layer enqueues per-subscriber worker
4. Returns immediately (non-blocking)
5. Per-subscriber workers dequeue and invoke callbacks

### Subscriber Path

1. Subscriber callback is invoked by coordination layer
2. Processes and buffers event
3. Returns immediately
4. Background consumer thread pops from queue
5. Formats and delivers to target surface with retry/backoff on failure

## Integration Patterns

### Event Publisher

- Generate events and publish to coordination layer
- Publisher never blocks waiting for subscriber delivery
- Coordination layer handles buffering and retry logic

### Event Subscriber

- Register callback for specific event types
- Callback invoked immediately when event is published
- Subscriber processes event and returns quickly
- Optional: Use background worker for I/O-heavy processing (e.g., FIFO writes)

### Error Handling

- If subscriber callback throws error: log and skip (don't block publisher)
- If background worker fails: retry with exponential backoff
- Slow subscribers don't affect other subscribers or publishers

## Testing and Validation

Event coordination layer should support:

- Unit tests for pub-sub ordering
- Subscriber buffer overflow scenarios
- Multi-publisher concurrent access
- Delivery guarantee verification
- Backoff/retry behavior

### Canonical Gate (Release Criterion)

Use this as the single release-gate command for this document's delivery guarantees:

```bash
uv run pytest tests/test_event_broker_pubsub.py -q
```

Expected pass criteria:

- Exit code is `0`.
- Artifact evidence is a `passed` result for `tests/test_event_broker_pubsub.py` in pytest output.
- Any non-zero exit code or failed test in that file fails the gate.

### Runnable Verification Map

| Guarantee / Claim | Runnable Check | Expected Evidence |
|---|---|---|
| Ordered delivery for a subscriber | `uv run pytest tests/test_event_broker_pubsub.py -k preserves_order_for_single_subscriber -q` | Test passes; received event indices are monotonic and complete |
| No dropped events when subscriber is busy | `uv run pytest tests/test_event_broker_pubsub.py -k no_drop_when_subscriber_is_busy -q` | Test passes; published count equals consumed count |
| Slow subscriber does not block publisher path | `uv run pytest tests/test_event_broker_pubsub.py -k slow_subscriber -q` | Test passes; publish path completes while handler sleeps |
| Canonical streaming event sequence survives broker path | `uv run pytest tests/test_event_broker_pubsub.py -k canonical_order -q` | Test passes; sequence ordering matches expected stream protocol |
| Event types and channel definitions stay aligned | `rg -n "STREAM_START|THINKING_START|agent_content|processing_state" docs/architecture/channel_registry.md tests/test_event_broker_pubsub.py` | Matching event names appear in both architecture contract and tests |

Threshold for this doc's reliability claims: all mapped checks above pass in the same change set before release.

## Migration from Old Architecture

### Previous Approach (Issues)

- Response coordinator directly called surface writers (non-blocking, data could be dropped)
- If a surface reader was unavailable or slow, messages were lost
- No buffering or retry logic
- Tight coupling between coordinator and surfaces
- Poor scalability for new subscribers

### Improved Approach (Pub-Sub)

- Response coordinator publishes events to coordination layer
- Coordination layer manages all subscriber queues
- Guaranteed delivery with buffering and retry
- Loose coupling: new subscribers can be added without changing publisher
- Scalable to many subscribers

## Benefits

✅ **No data loss:** Events queued per subscriber, never dropped
✅ **Scalable:** Easy to add new subscribers (logging, monitoring, metrics, etc.)
✅ **Decoupled:** Publishers don't know about subscribers
✅ **Thread-safe:** Concurrent access control on all shared state
✅ **Backpressure:** Slow surfaces don't block the response stream
✅ **Buffering:** Events buffered if a surface temporarily unavailable
✅ **Retry logic:** Writes retry with exponential backoff
✅ **Testable:** All components unit-tested for ordering, buffering, error handling

## Future Enhancements

1. **Logging Subscriber:** Wire logging system as subscriber for audit trail
2. **Metrics Subscriber:** Collect throughput and latency metrics
3. **Event Persistence:** Persist all events for session replay
4. **Event Filtering:** Allow subscribers to filter by event type or source
5. **Bidirectional Communication:** Support events flowing in both directions
6. **Priority Queues:** Support high-priority events (errors, critical updates)

## See Also

- [Channel Registry](architecture/channel_registry.md) — Event schema and registry
- [UX Lifecycle](ux/UX_LIFECYCLE.md) — User interface specifications
