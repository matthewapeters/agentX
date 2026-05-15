"""Unit tests for TUI output bridge and streaming output integration."""

from __future__ import annotations

import errno
import os
import time
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest

from agentx.event_broker import EventType
from agentx.integration.tui_bridge import SUBMIT_SENTINEL, TuiBridge
from agentx.streaming_controller import StreamingController


class _Msg:
    """Minimal persisted message stub used by streaming-controller tests."""

    def __init__(self, message_id: str) -> None:
        self.message_id = message_id
        self.file_path = None

    def save(self, _path: str) -> None:
        """Persist message metadata (unused in these tests)."""
        return None


def _build_session(show_thinking: bool = False) -> SimpleNamespace:
    """Build a minimal session stub for StreamingController tests.

    GIVEN a controller method that writes to GUI/context/loggers
    WHEN called with this stub session
    THEN controller behavior can be verified hermetically.
    """
    gui = MagicMock()
    context = MagicMock()
    context.add_tool_call_message.return_value = _Msg("tool-call-1")
    context.add_tool_result_message.return_value = _Msg("tool-result-1")

    session = SimpleNamespace()
    session.gui = gui
    session.config = {"tui": {"show_thinking": show_thinking}}
    session.active_model = "gpt-oss:latest"
    session._assistant_header_shown = False
    session._thinking_header_shown = False
    session._safe_root_after = lambda callback: callback()
    session._write_log = MagicMock()
    session._output_logger = MagicMock()
    session.context = context
    session.refresh_working_memory_gui = MagicMock()
    session._active_terminal_panes = set()
    session.tui_bridge = MagicMock()
    session.event_broker = MagicMock()
    return session


@pytest.mark.unit
def test_tui_bridge_write_output_success_with_reader(tmp_path: Path) -> None:
    """GIVEN active FIFO reader WHEN write_output is called THEN full payload reaches reader."""
    output_fifo = tmp_path / "output.fifo"
    os.mkfifo(output_fifo)

    received: list[bytes] = []

    def read_fifo() -> None:
        """Background thread to read FIFO data."""
        try:
            with open(output_fifo, "rb") as f:
                while True:
                    chunk = f.read(4096)
                    if not chunk:
                        break
                    received.append(chunk)
        except Exception:
            pass

    import threading

    reader_thread = threading.Thread(target=read_fifo, daemon=True)
    reader_thread.start()
    time.sleep(0.05)  # Let reader open FIFO

    bridge = TuiBridge(output_fifo=str(output_fifo), enabled=True, write_timeout_sec=0.5)
    bridge.start()

    try:
        ok = bridge.write_output("hello world")
        assert ok is True

        # Give reader time to process
        deadline = time.time() + 1.0
        while not received and time.time() < deadline:
            time.sleep(0.01)

        assert received, "Data did not reach FIFO reader"
        assert b"hello world" in b"".join(received)
    finally:
        bridge.stop()
        reader_thread.join(timeout=1.0)


@pytest.mark.unit
def test_tui_bridge_write_output_drops_when_no_reader(tmp_path: Path) -> None:
    """GIVEN no FIFO reader WHEN write_output is called THEN write is dropped safely."""
    output_fifo = tmp_path / "output.fifo"
    os.mkfifo(output_fifo)

    bridge = TuiBridge(output_fifo=str(output_fifo), enabled=True, write_timeout_sec=0.05)
    bridge.start()

    try:
        # No reader thread opened the FIFO, so write should fail
        ok = bridge.write_output("hello")
        # Non-blocking write with no reader should be False or timeout
        assert ok is False
    finally:
        bridge.stop()


@pytest.mark.unit
def test_tui_bridge_write_output_returns_false_when_disabled(tmp_path: Path) -> None:
    """GIVEN bridge disabled WHEN write_output is called THEN returns False immediately."""
    output_fifo = tmp_path / "output.fifo"
    os.mkfifo(output_fifo)

    bridge = TuiBridge(output_fifo=str(output_fifo), enabled=False)
    bridge.start()

    try:
        ok = bridge.write_output("hello")
        assert ok is False
    finally:
        bridge.stop()


@pytest.mark.unit
def test_tui_bridge_write_output_returns_false_for_empty_record(tmp_path: Path) -> None:
    """GIVEN empty record WHEN write_output is called THEN returns False immediately."""
    output_fifo = tmp_path / "output.fifo"
    os.mkfifo(output_fifo)

    bridge = TuiBridge(output_fifo=str(output_fifo), enabled=True)
    bridge.start()

    try:
        ok = bridge.write_output("")
        assert ok is False
    finally:
        bridge.stop()


@pytest.mark.unit
def test_tui_bridge_write_output_multipart_payload(tmp_path: Path) -> None:
    """GIVEN large payload requiring multiple writes WHEN write_output is called THEN full data reaches reader."""
    output_fifo = tmp_path / "output.fifo"
    os.mkfifo(output_fifo)

    received: list[bytes] = []

    def read_fifo() -> None:
        """Background thread to read FIFO data."""
        try:
            with open(output_fifo, "rb") as f:
                while True:
                    chunk = f.read(4096)
                    if not chunk:
                        break
                    received.append(chunk)
        except Exception:
            pass

    import threading

    reader_thread = threading.Thread(target=read_fifo, daemon=True)
    reader_thread.start()
    time.sleep(0.05)

    bridge = TuiBridge(output_fifo=str(output_fifo), enabled=True, write_timeout_sec=0.5)
    bridge.start()

    try:
        # Large payload that may require multiple os.write calls
        large_payload = "x" * 10000
        ok = bridge.write_output(large_payload)
        assert ok is True

        deadline = time.time() + 1.0
        while not received and time.time() < deadline:
            time.sleep(0.01)

        combined = b"".join(received)
        assert len(combined) >= len(large_payload)
        assert large_payload.encode() in combined
    finally:
        bridge.stop()
        reader_thread.join(timeout=1.0)


@pytest.mark.unit
def test_tui_bridge_write_output_handles_unicode(tmp_path: Path) -> None:
    """GIVEN unicode payload WHEN write_output is called THEN payload is encoded and reaches reader."""
    output_fifo = tmp_path / "output.fifo"
    os.mkfifo(output_fifo)

    received: list[bytes] = []

    def read_fifo() -> None:
        """Background thread to read FIFO data."""
        try:
            with open(output_fifo, "rb") as f:
                while True:
                    chunk = f.read(4096)
                    if not chunk:
                        break
                    received.append(chunk)
        except Exception:
            pass

    import threading

    reader_thread = threading.Thread(target=read_fifo, daemon=True)
    reader_thread.start()
    time.sleep(0.05)

    bridge = TuiBridge(output_fifo=str(output_fifo), enabled=True, write_timeout_sec=0.5)
    bridge.start()

    try:
        unicode_payload = "Hello 世界 🚀 Привет"
        ok = bridge.write_output(unicode_payload)
        assert ok is True

        deadline = time.time() + 1.0
        while not received and time.time() < deadline:
            time.sleep(0.01)

        combined = b"".join(received).decode("utf-8", errors="replace")
        assert "Hello" in combined
        assert "🚀" in combined
    finally:
        bridge.stop()
        reader_thread.join(timeout=1.0)


def test_streaming_controller_writes_agent_and_tool_records_to_tui() -> None:
    """GIVEN TUI bridge enabled WHEN stream/tool events render THEN TUI records are emitted."""
    session = _build_session(show_thinking=False)
    controller = StreamingController(session)

    controller._handle_stream_content("hi")
    controller._display_tool_call("read_file", {"path": "src/app.py"})
    controller._display_tool_result("read_file", "ok")

    calls = [
        (call.args[0], call.args[1].get("text", ""))
        for call in session.event_broker.publish.call_args_list
        if len(call.args) >= 2
    ]

    assert any(event == EventType.AGENT_CONTENT and record.startswith("###AGENT") for event, record in calls)
    assert any(event == EventType.AGENT_CONTENT and record == "hi" for event, record in calls)
    assert any(
        event == EventType.AGENT_CONTENT and record.startswith("###TOOL_CALL read_file") for event, record in calls
    )
    assert any(
        event == EventType.AGENT_CONTENT and record.startswith("###TOOL_RESULT read_file") for event, record in calls
    )


def test_streaming_controller_respects_show_thinking_flag_for_tui() -> None:
    """GIVEN show_thinking=false WHEN thinking text renders THEN no TUI thinking record is emitted."""
    session = _build_session(show_thinking=False)
    controller = StreamingController(session)

    controller._display_thinking("internal")

    calls = [
        call.args[1].get("text", "")
        for call in session.event_broker.publish.call_args_list
        if len(call.args) >= 2 and call.args[0] == EventType.AGENT_CONTENT
    ]
    assert not any(record.startswith("###THINKING") for record in calls)
    assert "internal" not in calls


def test_streaming_controller_writes_thinking_when_enabled() -> None:
    """GIVEN show_thinking=true WHEN thinking text renders THEN TUI receives thinking records."""
    session = _build_session(show_thinking=True)
    controller = StreamingController(session)

    controller._display_thinking("internal")

    calls = [
        call.args[1].get("text", "")
        for call in session.event_broker.publish.call_args_list
        if len(call.args) >= 2 and call.args[0] == EventType.AGENT_CONTENT
    ]
    assert any(record == "###THINKING 💭\n" for record in calls)
    assert any(record == "internal" for record in calls)


def test_streaming_controller_writes_classification_record_to_tui() -> None:
    """GIVEN classification metadata WHEN callback runs THEN TUI receives a classification block with emoji."""
    session = _build_session(show_thinking=False)
    controller = StreamingController(session)

    callback = controller._make_classification_callback(
        {
            "agentix": {
                "classification_display": {
                    "enabled": True,
                    "show_intent": True,
                    "show_reasoning": True,
                    "show_clarification": True,
                    "show_next_step": True,
                }
            }
        }
    )
    callback(
        {
            "intent": "simple_action",
            "reasoning_summary": "Direct answer is sufficient.",
            "needs_clarification": False,
            "missing_fields": [],
            "next_step": "respond_directly",
        }
    )

    calls = [
        call.args[1].get("text", "")
        for call in session.event_broker.publish.call_args_list
        if len(call.args) >= 2 and call.args[0] == EventType.AGENT_CONTENT
    ]
    classification_records = [record for record in calls if record.startswith("###CLASSIFICATION")]
    assert classification_records, "Expected at least one TUI classification record"
    assert "###CLASSIFICATION 🤔" in classification_records[0]
    assert "intent: simple_action" in classification_records[0]
    assert "path: respond_directly" in classification_records[0]


@pytest.mark.unit
def test_tui_bridge_reads_submit_messages_from_input_fifo(tmp_path: Path) -> None:
    """GIVEN TUI input fifo payloads WHEN submit sentinel appears THEN callback receives trimmed prompts."""
    output_fifo = tmp_path / "output.fifo"
    input_fifo = tmp_path / "input.fifo"
    os.mkfifo(input_fifo)

    submitted: list[str] = []
    bridge = TuiBridge(
        output_fifo=str(output_fifo),
        input_fifo=str(input_fifo),
        on_submit=submitted.append,
        enabled=True,
    )
    bridge.start()
    try:
        _write_fifo_payload(str(input_fifo), f" first prompt {SUBMIT_SENTINEL}second{SUBMIT_SENTINEL}")
        deadline = time.time() + 1.5
        while len(submitted) < 2 and time.time() < deadline:
            time.sleep(0.01)
    finally:
        bridge.stop()

    assert submitted == ["first prompt", "second"]


@pytest.mark.unit
def test_tui_bridge_ignores_empty_submit_messages(tmp_path: Path) -> None:
    """GIVEN empty submit payloads WHEN sentinel is parsed THEN callback only receives non-empty prompts."""
    output_fifo = tmp_path / "output.fifo"
    input_fifo = tmp_path / "input.fifo"
    os.mkfifo(input_fifo)

    submitted: list[str] = []
    bridge = TuiBridge(
        output_fifo=str(output_fifo),
        input_fifo=str(input_fifo),
        on_submit=submitted.append,
        enabled=True,
    )
    bridge.start()
    try:
        _write_fifo_payload(str(input_fifo), f"   {SUBMIT_SENTINEL}real input{SUBMIT_SENTINEL}")
        deadline = time.time() + 1.5
        while len(submitted) < 1 and time.time() < deadline:
            time.sleep(0.01)
    finally:
        bridge.stop()

    assert submitted == ["real input"]


@pytest.mark.unit
def test_tui_bridge_input_reader_handles_transient_eof_without_reopen() -> None:
    """GIVEN FIFO read EOF WHEN later submit arrives THEN callback still receives message without fd reopen."""
    submitted: list[str] = []
    bridge = TuiBridge(
        output_fifo="/tmp/unused-output.fifo",
        input_fifo="/tmp/unused-input.fifo",
        on_submit=submitted.append,
        enabled=True,
    )

    read_sequence = [b"", f" hello {SUBMIT_SENTINEL}".encode("utf-8")]
    state = {"reads": 0}

    def _fake_read(_fd: int, _size: int) -> bytes:
        """Return EOF once, then a valid submit payload, then stop the loop."""
        idx = state["reads"]
        state["reads"] += 1
        if idx < len(read_sequence):
            return read_sequence[idx]
        bridge._stop_event.set()
        return b""

    with (
        patch("agentx.integration.tui_bridge.os.open", return_value=11) as mock_open,
        patch("agentx.integration.tui_bridge.select.select", return_value=([11], [], [])),
        patch("agentx.integration.tui_bridge.os.read", side_effect=_fake_read),
        patch("agentx.integration.tui_bridge.time.sleep"),
        patch("agentx.integration.tui_bridge.os.close"),
    ):
        bridge._input_reader_loop()

    assert submitted == ["hello"]
    assert mock_open.call_count == 1


def _write_fifo_payload(path: str, payload: str) -> None:
    """Write payload to a FIFO, retrying briefly while reader endpoint appears."""
    deadline = time.time() + 1.5
    while time.time() < deadline:
        try:
            fd = os.open(path, os.O_WRONLY | os.O_NONBLOCK)
            try:
                os.write(fd, payload.encode("utf-8"))
                return
            finally:
                os.close(fd)
        except OSError as exc:
            if exc.errno == errno.ENXIO:
                time.sleep(0.01)
                continue
            raise
    raise AssertionError(f"Timed out opening fifo for write: {path}")
