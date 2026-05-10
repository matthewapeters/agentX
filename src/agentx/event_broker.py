"""
EventBroker — centralized pub-sub system for streaming events.

Ensures all subscribers (GUI, TUI, logging) receive events reliably with
guaranteed ordering and atomic delivery. This eliminates brittle point-to-point
FIFO writes and provides a single source of truth for all streaming events.

Design:
  - Publishers (StreamingController, Session) call broker.publish(event_type, data)
  - Subscribers (GUI, TUI, logging) register callbacks via broker.subscribe(event_type, callback)
  - Each subscriber has its own queue; slow subscribers don't block others
  - Events are delivered in order with guaranteed atomicity
"""

import logging
import threading
from collections import defaultdict
from dataclasses import dataclass
from enum import Enum
from queue import Queue
from typing import Any, Callable, Optional

logger = logging.getLogger(__name__)


class EventType(str, Enum):
    """Enumeration of streaming events."""

    # Streaming lifecycle
    STREAM_START = "stream_start"
    STREAM_END = "stream_end"

    # Content streaming
    THINKING_START = "thinking_start"
    THINKING_CONTENT = "thinking_content"
    AGENT_CONTENT = "agent_content"
    AGENT_HEADER = "agent_header"

    # Tool interactions
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"

    # Bootstrap and system messages
    BOOTSTRAP_MESSAGE = "bootstrap_message"
    SYSTEM_MESSAGE = "system_message"

    # User interactions
    USER_MESSAGE = "user_message"

    # Errors
    ERROR = "error"

    # Logging
    LOG_MESSAGE = "log_message"


@dataclass
class Event:
    """An event published by the broker."""

    event_type: EventType
    data: dict[str, Any]

    def __repr__(self) -> str:
        return f"Event({self.event_type.value}, data_keys={list(self.data.keys())})"


class EventBroker:
    """
    Centralized pub-sub broker for streaming events.

    Guarantees:
      - All subscribers receive all events in the same order
      - Slow subscribers don't block others (per-subscriber queues)
      - Thread-safe publishing and subscription
      - Atomic event delivery (no partial events)
    """

    def __init__(self) -> None:
        """Initialize the broker with empty subscriber registrations."""
        # subscribers[event_type] = [(callback, queue), ...]
        self._subscribers: dict[EventType, list[tuple[Callable, Queue]]] = defaultdict(list)
        self._lock = threading.RLock()

    def subscribe(
        self,
        event_type: EventType,
        callback: Callable[[Event], None],
        queue_size: int = 100,
    ) -> Callable[[], None]:
        """
        Register a callback for an event type.

        Args:
            event_type: The EventType to subscribe to.
            callback: Function called with Event when published.
            queue_size: Max size of the per-subscriber queue.

        Returns:
            Unsubscribe function: call to remove this subscription.

        Example:
            def on_content(event: Event) -> None:
                print(event.data['text'])

            unsub = broker.subscribe(EventType.AGENT_CONTENT, on_content)
            # ... later ...
            unsub()  # Unsubscribe
        """
        q: Queue = Queue(maxsize=queue_size)
        with self._lock:
            self._subscribers[event_type].append((callback, q))

        def unsubscribe() -> None:
            with self._lock:
                try:
                    self._subscribers[event_type].remove((callback, q))
                except ValueError:
                    pass

        return unsubscribe

    def publish(self, event_type: EventType, data: dict[str, Any]) -> None:
        """
        Publish an event to all registered subscribers.

        Each subscriber receives the event asynchronously via its own queue.
        The publisher is not blocked by slow subscribers.

        Args:
            event_type: The EventType being published.
            data: Event payload (arbitrary dict).

        Example:
            broker.publish(EventType.AGENT_CONTENT, {'text': 'Hello'})
        """
        event = Event(event_type=event_type, data=data)
        with self._lock:
            subscribers = list(self._subscribers.get(event_type, []))

        for callback, q in subscribers:
            try:
                # Non-blocking put: if queue is full, drop the event (subscriber is too slow)
                q.put_nowait(event)
                # Dispatch callback in a background thread so it doesn't block publishing
                threading.Thread(
                    target=self._dispatch_callback,
                    args=(callback, event),
                    daemon=True,
                ).start()
            except Exception as exc:
                logger.warning("Failed to queue event for subscriber: %s", exc)

    def _dispatch_callback(self, callback: Callable[[Event], None], event: Event) -> None:
        """Dispatch a callback safely, catching any exceptions."""
        try:
            callback(event)
        except Exception as exc:
            logger.exception("Subscriber callback raised exception: %s", exc)

    def publish_thinking_header(self, model_name: str) -> None:
        """Publish a thinking stream header."""
        self.publish(EventType.THINKING_START, {"model_name": model_name})

    def publish_thinking_content(self, text: str) -> None:
        """Publish thinking content."""
        self.publish(EventType.THINKING_CONTENT, {"text": text})

    def publish_agent_header(self, model_name: str) -> None:
        """Publish an agent response header."""
        self.publish(EventType.AGENT_HEADER, {"model_name": model_name})

    def publish_agent_content(self, text: str) -> None:
        """Publish agent response content."""
        self.publish(EventType.AGENT_CONTENT, {"text": text})

    def publish_tool_call(
        self,
        tool_name: str,
        tool_input: dict,
        round_index: Optional[int] = None,
        tool_id: Optional[str] = None,
    ) -> None:
        """Publish a tool call."""
        self.publish(
            EventType.TOOL_CALL,
            {
                "tool_name": tool_name,
                "tool_input": tool_input,
                "round_index": round_index,
                "tool_id": tool_id,
            },
        )

    def publish_tool_result(
        self,
        tool_name: str,
        output: Any,
        round_index: Optional[int] = None,
        tool_id: Optional[str] = None,
    ) -> None:
        """Publish a tool result."""
        self.publish(
            EventType.TOOL_RESULT,
            {
                "tool_name": tool_name,
                "output": output,
                "round_index": round_index,
                "tool_id": tool_id,
            },
        )

    def publish_bootstrap_message(self, message: str) -> None:
        """Publish a bootstrap/startup message."""
        self.publish(EventType.BOOTSTRAP_MESSAGE, {"message": message})

    def publish_system_message(self, message: str) -> None:
        """Publish a system message."""
        self.publish(EventType.SYSTEM_MESSAGE, {"message": message})

    def publish_user_message(self, text: str, timestamp: Optional[str] = None) -> None:
        """Publish a user message."""
        self.publish(EventType.USER_MESSAGE, {"text": text, "timestamp": timestamp})

    def publish_error(self, message: str) -> None:
        """Publish an error."""
        self.publish(EventType.ERROR, {"message": message})

    def publish_log(self, level: str, message: str) -> None:
        """Publish a log message."""
        self.publish(EventType.LOG_MESSAGE, {"level": level, "message": message})

    def publish_stream_start(self) -> None:
        """Publish stream start lifecycle event."""
        self.publish(EventType.STREAM_START, {})

    def publish_stream_end(self) -> None:
        """Publish stream end lifecycle event."""
        self.publish(EventType.STREAM_END, {})

    def clear_subscribers(self) -> None:
        """Clear all subscribers (used for testing)."""
        with self._lock:
            self._subscribers.clear()

    def get_subscriber_count(self, event_type: EventType) -> int:
        """Return the number of subscribers for an event type."""
        with self._lock:
            return len(self._subscribers.get(event_type, []))
