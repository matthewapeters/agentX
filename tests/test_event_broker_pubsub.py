"""
Unit tests for the event-broker pub-sub system.

Tests verify that:
  - EventBroker publishes events to all subscribers
  - TUIEventSubscriber buffers and formats events correctly
  - Events flow from StreamingController → Broker → TUI Subscriber → FIFO
"""

import threading
import time
from unittest.mock import MagicMock, patch

import pytest

from agentx.event_broker import Event, EventBroker, EventType
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

    def test_tui_subscriber_normalizes_raw_agent_header_to_include_robot_icon(self):
        """Raw AGENT marker should be normalized to canonical emoji header."""
        subscriber = TUIEventSubscriber()
        event = Event(
            event_type=EventType.AGENT_CONTENT,
            data={"text": "###AGENT\nhello", "is_raw_tui": True},
        )

        formatted = subscriber._format_event_for_tui(event)

        assert formatted.startswith("###AGENT 🤖")

    def test_tui_subscriber_normalizes_raw_user_record_to_include_user_icon(self):
        """Raw USER marker should include user icon on body line."""
        subscriber = TUIEventSubscriber()
        event = Event(
            event_type=EventType.AGENT_CONTENT,
            data={"text": "###USER 10:11:12\nhello\n\n", "is_raw_tui": True},
        )

        formatted = subscriber._format_event_for_tui(event)

        assert "###USER 10:11:12" in formatted
        assert "\n👤 hello\n" in formatted

    def test_tui_subscriber_normalizes_raw_done_marker_shape(self):
        """Raw completion records should collapse to one canonical done marker."""
        subscriber = TUIEventSubscriber()
        event = Event(
            event_type=EventType.AGENT_CONTENT,
            data={"text": "\n###DONE\n", "is_raw_tui": True},
        )

        formatted = subscriber._format_event_for_tui(event)

        assert formatted == "###DONE\n"

    def test_tui_subscriber_stream_end_marker_has_canonical_shape(self):
        """STREAM_END formatting should use canonical done marker without extra prefix newlines."""
        subscriber = TUIEventSubscriber()
        event = Event(event_type=EventType.STREAM_END, data={})

        formatted = subscriber._format_event_for_tui(event)

        assert formatted == "###DONE\n"

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

    def test_streaming_controller_broker_subscriber_writer_emits_canonical_order(self):
        """
        GIVEN: StreamingController publishes raw TUI records through EventBroker
        WHEN: TUIEventSubscriber writer loop drains events to bridge
        THEN: Bridge receives canonical records in publish order.
        """
        broker = EventBroker()

        class _CapturingBridge:
            def __init__(self) -> None:
                self.is_enabled = True
                self.records: list[str] = []

            def write_output(self, record: str) -> bool:
                self.records.append(record)
                return True

        bridge = _CapturingBridge()
        subscriber = TUIEventSubscriber(tui_bridge=bridge)
        subscriber.start()

        unsubscribers = [broker.subscribe(event_type, subscriber.handle_event) for event_type in EventType]

        try:
            session = MagicMock()
            session.event_broker = broker
            controller = StreamingController(session)

            controller._write_tui_output("###AGENT\n")
            controller._write_tui_output("hello")
            controller._write_tui_output("###DONE\n")

            deadline = time.time() + 1.5
            while len(bridge.records) < 3 and time.time() < deadline:
                time.sleep(0.01)

            assert bridge.records[:3] == ["###AGENT 🤖\n", "hello", "###DONE\n"]
        finally:
            for unsubscribe in unsubscribers:
                unsubscribe()
            subscriber.stop()

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

    def test_mode_switch_boundary_detach_reattach_keeps_single_markers(self):
        """
        GIVEN: A subscriber is detached and a new subscriber is attached at a turn boundary
        WHEN: Two turns are published before and after the switch
        THEN: AGENT and DONE markers are emitted once per turn without duplication or reordering
        """
        broker = EventBroker()

        class _CapturingBridge:
            def __init__(self) -> None:
                self.is_enabled = True
                self.records: list[str] = []

            def write_output(self, record: str) -> bool:
                self.records.append(record)
                return True

        bridge = _CapturingBridge()

        subscriber_one = TUIEventSubscriber(tui_bridge=bridge)
        subscriber_one.start()
        unsubscribers_one = [broker.subscribe(event_type, subscriber_one.handle_event) for event_type in EventType]

        session = MagicMock()
        session.event_broker = broker
        controller = StreamingController(session)

        try:
            controller._write_tui_output("###AGENT\n")
            controller._write_tui_output("first turn")
            controller._write_tui_output("###DONE\n")

            deadline = time.time() + 1.5
            while len(bridge.records) < 3 and time.time() < deadline:
                time.sleep(0.01)

            for unsubscribe in unsubscribers_one:
                unsubscribe()
            subscriber_one.stop()

            subscriber_two = TUIEventSubscriber(tui_bridge=bridge)
            subscriber_two.start()
            unsubscribers_two = [broker.subscribe(event_type, subscriber_two.handle_event) for event_type in EventType]
            try:
                controller._write_tui_output("###AGENT\n")
                controller._write_tui_output("second turn")
                controller._write_tui_output("###DONE\n")

                deadline = time.time() + 1.5
                while len(bridge.records) < 6 and time.time() < deadline:
                    time.sleep(0.01)
            finally:
                for unsubscribe in unsubscribers_two:
                    unsubscribe()
                subscriber_two.stop()

            assert bridge.records[:6] == [
                "###AGENT 🤖\n",
                "first turn",
                "###DONE\n",
                "###AGENT 🤖\n",
                "second turn",
                "###DONE\n",
            ]
        finally:
            # Defensive cleanup in case the first subscriber wasn't fully cleaned up above.
            subscriber_one.stop()

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

    def test_tui_lifecycle_marker_sequence_is_canonical(self):
        """
        GIVEN: Canonical lifecycle events for one turn
        WHEN: Events are formatted for TUI output
        THEN: Markers appear in canonical sequence without duplication.
        """
        subscriber = TUIEventSubscriber()

        lifecycle_events = [
            Event(EventType.BOOTSTRAP_MESSAGE, {"message": "startup"}),
            Event(EventType.USER_MESSAGE, {"text": "hello", "timestamp": "10:00:00"}),
            Event(EventType.AGENT_HEADER, {"model_name": "gpt"}),
            Event(EventType.THINKING_START, {"model_name": "gpt"}),
            Event(EventType.THINKING_CONTENT, {"text": "hmm"}),
            Event(EventType.AGENT_CONTENT, {"text": "answer"}),
            Event(EventType.TOOL_CALL, {"tool_name": "read_file", "tool_input": {"path": "a.py"}}),
            Event(EventType.TOOL_RESULT, {"tool_name": "read_file", "output": "ok"}),
            Event(EventType.STREAM_END, {}),
        ]

        rendered = [subscriber._format_event_for_tui(event) for event in lifecycle_events]
        combined = "".join(rendered)

        idx_system = combined.find("###SYSTEM Bootstrap")
        idx_user = combined.find("###USER 10:00:00")
        idx_agent = combined.find("###AGENT 🤖")
        idx_think = combined.find("###THINKING 💭")
        idx_tool_call = combined.find("###TOOL_CALL")
        idx_tool_result = combined.find("###TOOL_RESULT")
        idx_done = combined.find("###DONE")

        assert -1 not in [idx_system, idx_user, idx_agent, idx_think, idx_tool_call, idx_tool_result, idx_done]
        assert idx_system < idx_user < idx_agent < idx_think < idx_tool_call < idx_tool_result < idx_done
        assert combined.count("###AGENT 🤖") == 1
        assert combined.count("###THINKING 💭") == 1
        assert combined.count("###DONE") == 1


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
