"""
Unit tests for the event-broker pub-sub system.

Tests verify that:
  - EventBroker publishes events to all subscribers
  - TUIEventSubscriber buffers and formats events correctly
  - Events flow from StreamingController → Broker → TUI Subscriber → FIFO
"""

import pytest
import time
import threading
from unittest.mock import MagicMock, patch

from agentx.event_broker import EventBroker, EventType, Event
from agentx.integration.tui_event_subscriber import TUIEventSubscriber
from agentx.streaming_controller import StreamingController


class TestEventBrokerPubSub:
    """Tests for EventBroker pub-sub system."""

    def test_event_broker_basic_publish(self):
        """
        GIVEN: An EventBroker with a subscriber
        WHEN: An event is published
        THEN: The subscriber receives the event
        """
        broker = EventBroker()
        events_received = []

        def handler(event: Event):
            events_received.append(event)

        broker.subscribe(EventType.AGENT_CONTENT, handler)
        broker.publish(EventType.AGENT_CONTENT, {"text": "Hello"})

        time.sleep(0.1)  # Wait for async dispatch
        assert len(events_received) == 1
        assert events_received[0].data["text"] == "Hello"

    def test_event_broker_multiple_subscribers(self):
        """
        GIVEN: An EventBroker with multiple subscribers to same event
        WHEN: An event is published
        THEN: All subscribers receive the event
        """
        broker = EventBroker()
        events1 = []
        events2 = []

        broker.subscribe(EventType.AGENT_CONTENT, lambda e: events1.append(e))
        broker.subscribe(EventType.AGENT_CONTENT, lambda e: events2.append(e))

        broker.publish(EventType.AGENT_CONTENT, {"text": "Broadcast"})
        time.sleep(0.1)

        assert len(events1) == 1
        assert len(events2) == 1
        assert events1[0].data["text"] == "Broadcast"
        assert events2[0].data["text"] == "Broadcast"

    def test_event_broker_unsubscribe(self):
        """
        GIVEN: A subscriber to EventBroker
        WHEN: The unsubscribe function is called
        THEN: The subscriber stops receiving events
        """
        broker = EventBroker()
        events = []

        unsub = broker.subscribe(EventType.AGENT_CONTENT, lambda e: events.append(e))
        broker.publish(EventType.AGENT_CONTENT, {"text": "Before"})
        time.sleep(0.1)

        unsub()
        broker.publish(EventType.AGENT_CONTENT, {"text": "After"})
        time.sleep(0.1)

        assert len(events) == 1
        assert events[0].data["text"] == "Before"

    def test_event_broker_slow_subscriber(self):
        """
        GIVEN: An EventBroker with a subscriber that blocks
        WHEN: Multiple events are published rapidly
        THEN: Events are not dropped (each subscriber has its own queue)
        """
        broker = EventBroker()
        events = []
        processed_count = [0]

        def slow_handler(event: Event):
            events.append(event)
            processed_count[0] += 1
            time.sleep(0.05)  # Simulate slow processing

        broker.subscribe(EventType.AGENT_CONTENT, slow_handler)

        # Publish multiple events rapidly
        for i in range(5):
            broker.publish(EventType.AGENT_CONTENT, {"text": f"Event {i}"})

        # Wait for all events to be processed
        time.sleep(1.0)

        assert len(events) == 5
        assert processed_count[0] == 5

    def test_event_broker_preserves_order_for_single_subscriber(self):
        """
        GIVEN: One subscriber receiving many events
        WHEN: Events are published in sequence
        THEN: Callback observes the same event order
        """
        broker = EventBroker()
        received: list[int] = []

        def handler(event: Event) -> None:
            received.append(int(event.data["index"]))

        broker.subscribe(EventType.AGENT_CONTENT, handler)

        count = 200
        for i in range(count):
            broker.publish(EventType.AGENT_CONTENT, {"index": i})

        deadline = time.time() + 2.0
        while len(received) < count and time.time() < deadline:
            time.sleep(0.01)

        assert len(received) == count
        assert received == list(range(count))

    def test_event_broker_no_drop_when_subscriber_is_busy(self):
        """
        GIVEN: A slow subscriber and rapid publish bursts
        WHEN: A large stream of events is published
        THEN: Subscriber still receives every event
        """
        broker = EventBroker()
        received: list[int] = []
        lock = threading.Lock()

        def handler(event: Event) -> None:
            time.sleep(0.001)
            with lock:
                received.append(int(event.data["index"]))

        broker.subscribe(EventType.AGENT_CONTENT, handler, queue_size=1)

        count = 1200
        for i in range(count):
            broker.publish(EventType.AGENT_CONTENT, {"index": i})

        deadline = time.time() + 8.0
        while True:
            with lock:
                done = len(received) >= count
            if done or time.time() >= deadline:
                break
            time.sleep(0.01)

        with lock:
            assert len(received) == count


class TestTUIEventSubscriber:
    """Tests for TUIEventSubscriber."""

    def test_tui_subscriber_event_formatting(self):
        """
        GIVEN: A TUIEventSubscriber
        WHEN: Events of different types are formatted
        THEN: They are formatted correctly for TUI output
        """
        subscriber = TUIEventSubscriber()

        test_cases = [
            (EventType.THINKING_START, {"model_name": "llama"}, "###THINKING"),
            (EventType.AGENT_HEADER, {"model_name": "llama"}, "###AGENT"),
            (EventType.AGENT_CONTENT, {"text": "Response"}, "Response"),
            (EventType.TOOL_CALL, {"tool_name": "read_file", "tool_input": {}}, "###TOOL_CALL"),
            (EventType.STREAM_END, {}, "###DONE"),
        ]

        for event_type, data, expected in test_cases:
            event = Event(event_type=event_type, data=data)
            formatted = subscriber._format_event_for_tui(event)
            assert expected in formatted, f"Expected {expected} in {formatted}"

    def test_tui_subscriber_buffers_events(self):
        """
        GIVEN: A TUIEventSubscriber with no FIFO
        WHEN: Multiple events are handled
        THEN: Events are queued in the event buffer
        """
        subscriber = TUIEventSubscriber()

        for i in range(5):
            event = Event(event_type=EventType.AGENT_CONTENT, data={"text": f"Event {i}"})
            subscriber.handle_event(event)

        assert len(subscriber._event_queue) == 5

    def test_tui_subscriber_retains_all_queued_events(self):
        """
        GIVEN: A TUIEventSubscriber queue
        WHEN: Many events are added before writer drains
        THEN: Queue retains all events without maxlen truncation
        """
        subscriber = TUIEventSubscriber()

        for i in range(12000):
            event = Event(event_type=EventType.AGENT_CONTENT, data={"text": f"Event {i}"})
            subscriber.handle_event(event)

        assert len(subscriber._event_queue) == 12000
        oldest_event = subscriber._event_queue[0]
        assert "Event 0" in oldest_event.data["text"]


class TestStreamingControllerPubSub:
    """Tests for StreamingController publishing events."""

    def test_streaming_controller_publishes_on_write_tui_output(self):
        """
        GIVEN: A StreamingController with event broker
        WHEN: _write_tui_output is called
        THEN: An event is published to the broker
        """
        broker = EventBroker()
        session = MagicMock()
        session.event_broker = broker

        events = []
        broker.subscribe(EventType.AGENT_CONTENT, lambda e: events.append(e))

        controller = StreamingController(session)
        controller._write_tui_output("Test output")

        time.sleep(0.1)
        assert len(events) == 1
        assert events[0].data["text"] == "Test output"

    def test_streaming_controller_no_broker_graceful(self):
        """
        GIVEN: A StreamingController with no event broker
        WHEN: _write_tui_output is called
        THEN: No exception is raised
        """
        session = MagicMock()
        session.event_broker = None

        controller = StreamingController(session)
        # Should not raise
        controller._write_tui_output("Test output")


class TestEndToEndDataFlow:
    """Integration tests for end-to-end data flow."""

    def test_full_chain_streaming_to_tui(self):
        """
        GIVEN: StreamingController, EventBroker, and TUIEventSubscriber
        WHEN: StreamingController publishes content events
        THEN: TUIEventSubscriber receives and formats them correctly
        """
        broker = EventBroker()
        subscriber = TUIEventSubscriber()
        session = MagicMock()
        session.event_broker = broker

        # Subscribe TUI subscriber to all relevant event types
        for event_type in [EventType.AGENT_CONTENT, EventType.THINKING_CONTENT, EventType.TOOL_CALL]:
            broker.subscribe(event_type, subscriber.handle_event)

        controller = StreamingController(session)

        # Simulate a response stream
        controller._write_tui_output("Content line 1")
        controller._write_tui_output("Content line 2")

        time.sleep(0.1)

        # Verify events were buffered by TUI subscriber
        assert len(subscriber._event_queue) == 2
        events = list(subscriber._event_queue)
        assert "Content line 1" in events[0].data["text"]
        assert "Content line 2" in events[1].data["text"]

    def test_tui_subscriber_writer_thread(self):
        """
        GIVEN: A TUIEventSubscriber with writer thread
        WHEN: Events are added to the queue and writer thread is started
        THEN: Events are processed and formatted
        """
        with patch.object(TUIEventSubscriber, "_write_event_to_fifo") as mock_write:
            mock_write.return_value = True

            subscriber = TUIEventSubscriber()
            subscriber.start()

            # Add an event
            event = Event(event_type=EventType.AGENT_CONTENT, data={"text": "Test"})
            subscriber.handle_event(event)

            time.sleep(0.2)

            # Stop and verify
            subscriber.stop()

            # Should have attempted to write
            assert mock_write.called


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
