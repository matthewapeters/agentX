"""Unit tests for TUI output bridge and streaming output integration."""

from __future__ import annotations

import errno
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from agentx.integration.tui_bridge import TuiBridge
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
    return session


def test_tui_bridge_write_output_success() -> None:
    """GIVEN writable FIFO WHEN write_output is called THEN full payload is written."""
    bridge = TuiBridge("/tmp/test-output.fifo", enabled=True, write_timeout_sec=0.1)
    bridge.start()

    with (
        patch("agentx.integration.tui_bridge.os.open", return_value=11),
        patch("agentx.integration.tui_bridge.select.select", return_value=([], [11], [])),
        patch("agentx.integration.tui_bridge.os.write", return_value=5) as mock_write,
        patch("agentx.integration.tui_bridge.os.close") as mock_close,
    ):
        ok = bridge.write_output("hello")

    assert ok is True
    mock_write.assert_called_once()
    mock_close.assert_called_once_with(11)


def test_tui_bridge_write_output_drops_when_no_reader() -> None:
    """GIVEN FIFO has no reader WHEN write_output is called THEN write is dropped safely."""
    bridge = TuiBridge("/tmp/test-output.fifo", enabled=True)
    bridge.start()

    with patch("agentx.integration.tui_bridge.os.open", side_effect=OSError(errno.ENXIO, "no reader")):
        ok = bridge.write_output("hello")

    assert ok is False


def test_streaming_controller_writes_agent_and_tool_records_to_tui() -> None:
    """GIVEN TUI bridge enabled WHEN stream/tool events render THEN TUI records are emitted."""
    session = _build_session(show_thinking=False)
    controller = StreamingController(session)

    controller._handle_stream_content("hi")
    controller._display_tool_call("read_file", {"path": "src/app.py"})
    controller._display_tool_result("read_file", "ok")

    calls = [str(call.args[0]) for call in session.tui_bridge.write_output.call_args_list]

    assert any(record == "###AGENT\n" for record in calls)
    assert any(record == "hi" for record in calls)
    assert any(record.startswith("###TOOL_CALL read_file") for record in calls)
    assert any(record.startswith("###TOOL_RESULT read_file") for record in calls)


def test_streaming_controller_respects_show_thinking_flag_for_tui() -> None:
    """GIVEN show_thinking=false WHEN thinking text renders THEN no TUI thinking record is emitted."""
    session = _build_session(show_thinking=False)
    controller = StreamingController(session)

    controller._display_thinking("internal")

    calls = [str(call.args[0]) for call in session.tui_bridge.write_output.call_args_list]
    assert not any(record.startswith("###THINKING") for record in calls)
    assert "internal" not in calls


def test_streaming_controller_writes_thinking_when_enabled() -> None:
    """GIVEN show_thinking=true WHEN thinking text renders THEN TUI receives thinking records."""
    session = _build_session(show_thinking=True)
    controller = StreamingController(session)

    controller._display_thinking("internal")

    calls = [str(call.args[0]) for call in session.tui_bridge.write_output.call_args_list]
    assert any(record == "###THINKING\n" for record in calls)
    assert any(record == "internal" for record in calls)
