"""
TUI Event Subscriber — handles TUI output via event broker.

Subscribes to streaming events and writes them to the TUI output FIFO
with guaranteed delivery. Buffers output if FIFO isn't ready.
"""

import logging
import threading
import time
from collections import deque
from typing import Optional

from ..event_broker import Event, EventType
from .tui_bridge import TuiBridge

logger = logging.getLogger(__name__)


class TUIEventSubscriber:
    """
    Subscriber that handles all streaming events for TUI output.

    Guarantees:
      - All events are queued, never dropped
      - Events are written to FIFO in order
      - Slow FIFO doesn't block the event publisher
      - Failed FIFO writes are retried with backoff
    """

    def __init__(self, tui_bridge: Optional[TuiBridge] = None) -> None:
        """Initialize the TUI event subscriber.

        Args:
            tui_bridge: Optional TuiBridge for FIFO writing. If None, events are logged only.
        """
        self.tui_bridge = tui_bridge
        # Keep an unbounded queue so TUI output is not silently dropped.
        self._event_queue: deque = deque()
        self._writer_thread: Optional[threading.Thread] = None
        self._stop_event = threading.Event()
        self._lock = threading.Lock()

    def handle_event(self, event: Event) -> None:
        """Handle a streaming event from the event broker.

        Args:
            event: The Event to process.
        """
        with self._lock:
            self._event_queue.append(event)

    def start(self) -> None:
        """Start the background FIFO writer thread."""
        if self._writer_thread is not None and self._writer_thread.is_alive():
            return
        self._stop_event.clear()
        self._writer_thread = threading.Thread(target=self._writer_loop, daemon=True)
        self._writer_thread.start()

    def stop(self) -> None:
        """Stop the background FIFO writer thread."""
        self._stop_event.set()
        if self._writer_thread is not None and self._writer_thread.is_alive():
            self._writer_thread.join(timeout=2.0)

    def _writer_loop(self) -> None:
        """Background thread that writes queued events to the TUI FIFO."""
        while not self._stop_event.is_set():
            try:
                event: Event | None = None
                with self._lock:
                    if not self._event_queue:
                        event = None
                    else:
                        event = self._event_queue[0]  # Peek at the front

                if event is None:
                    # No events; wait a bit before checking again
                    time.sleep(0.01)
                    continue

                # Try to write the event
                if self._write_event_to_fifo(event):
                    with self._lock:
                        self._event_queue.popleft()  # Remove after successful write
                else:
                    # FIFO not ready; back off and retry
                    time.sleep(0.05)

            except Exception as exc:
                logger.exception("TUI writer loop failed: %s", exc)
                time.sleep(0.1)

    def _write_event_to_fifo(self, event: Event) -> bool:
        """Write an event to the TUI FIFO.

        Args:
            event: The Event to write.

        Returns:
            True if successfully written, False if FIFO is unavailable.
        """
        if self.tui_bridge is None or not self.tui_bridge.is_enabled:
            return True  # No FIFO; pretend success so queue drains

        try:
            formatted = self._format_event_for_tui(event)
            if not formatted:
                return True  # No output needed; pretend success

            return self.tui_bridge.write_output(formatted)

        except Exception as exc:
            logger.debug("Failed to write event to TUI FIFO: %s", exc)
            return False

    def _format_event_for_tui(self, event: Event) -> str:
        """Format an event for TUI output.

        Args:
            event: The Event to format.

        Returns:
            Formatted string for TUI, or empty string if event is not for TUI.
        """
        event_type = event.event_type
        data = event.data

        if event_type == EventType.THINKING_START:
            return "###THINKING\n"

        elif event_type == EventType.THINKING_CONTENT:
            return data.get("text", "")

        elif event_type == EventType.AGENT_HEADER:
            # Preserve marker semantics while adding GUI-parity role icon.
            return "###AGENT 🤖\n"

        elif event_type == EventType.AGENT_CONTENT:
            # Check if this is raw TUI output (from _write_tui_output)
            if data.get("is_raw_tui"):
                raw_text = data.get("text", "")
                if raw_text.startswith("###AGENT") and "🤖" not in raw_text.splitlines()[0]:
                    return raw_text.replace("###AGENT", "###AGENT 🤖", 1)
                if raw_text.startswith("###USER"):
                    lines = raw_text.splitlines(keepends=True)
                    if len(lines) >= 2 and not lines[1].startswith("👤"):
                        lines[1] = f"👤 {lines[1]}"
                        return "".join(lines)
                return raw_text
            return data.get("text", "")

        elif event_type == EventType.TOOL_CALL:
            tool_name = data.get("tool_name", "unknown")
            tool_input = data.get("tool_input", {})
            try:
                import json

                input_text = f"{tool_name}: {json.dumps(tool_input, ensure_ascii=False)}"
            except Exception:
                input_text = f"{tool_name}: {tool_input}"
            return f"###TOOL_CALL {input_text}\n"

        elif event_type == EventType.TOOL_RESULT:
            tool_name = data.get("tool_name", "unknown")
            output = data.get("output", "")
            if isinstance(output, str):
                preview = output[:100] + "..." if len(output) > 100 else output
            else:
                preview = str(output)[:100]
            return f"###TOOL_RESULT {tool_name}: {preview}\n"

        elif event_type == EventType.USER_MESSAGE:
            text = data.get("text", "")
            timestamp = data.get("timestamp", "")
            if timestamp:
                return f"###USER {timestamp}\n👤 {text}\n\n"
            return f"###USER\n👤 {text}\n\n"

        elif event_type == EventType.BOOTSTRAP_MESSAGE:
            return f"###SYSTEM Bootstrap\n{data.get('message', '')}\n"

        elif event_type == EventType.SYSTEM_MESSAGE:
            return f"###SYSTEM\n{data.get('message', '')}\n"

        elif event_type == EventType.ERROR:
            return f"###ERROR {data.get('message', '')}\n"

        elif event_type == EventType.STREAM_END:
            return "\n###DONE\n"

        # Other events: no TUI output
        return ""
