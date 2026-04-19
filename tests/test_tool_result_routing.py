"""
Tests for the tool-result routing fix (C2).

Verifies that tool_name and tool_id are kept as separate, distinct values
through the ResponseHandler → session._display_tool_result →
context.add_tool_result_message pipeline. The old bug passed tool_id as
tool_name to add_tool_result_message, corrupting context records.
"""

import sys
import os
from pathlib import Path
from unittest.mock import MagicMock, patch, call

import pytest

project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from shared.models.response import ResponseChunk, ChunkType
from agentx.integration.response_handler import ResponseHandler

# ---------------------------------------------------------------------------
# ResponseHandler tests
# ---------------------------------------------------------------------------


class TestResponseHandlerToolResult:
    """ResponseHandler must pass tool_name and tool_id as separate arguments."""

    def _make_chunk(self, tool_name=None, tool_id=None, tool_output="result"):
        return ResponseChunk(
            type=ChunkType.TOOL_RESULT,
            tool_name=tool_name,
            tool_id=tool_id,
            tool_output=tool_output,
        )

    def test_callback_receives_tool_name_as_first_arg(self):
        """on_tool_result first arg is tool_name, not tool_id."""
        received = {}
        handler = ResponseHandler(
            on_tool_result=lambda name, out, round_i=None, tool_id=None: received.update(
                {"name": name, "tool_id": tool_id}
            )
        )
        handler.process_chunk(self._make_chunk(tool_name="read_file", tool_id="call-abc"))
        assert received["name"] == "read_file"

    def test_callback_receives_tool_id_as_fourth_arg(self):
        """on_tool_result fourth arg is tool_id, distinct from tool_name."""
        received = {}
        handler = ResponseHandler(
            on_tool_result=lambda name, out, round_i=None, tool_id=None: received.update(
                {"name": name, "tool_id": tool_id}
            )
        )
        handler.process_chunk(self._make_chunk(tool_name="read_file", tool_id="call-abc"))
        assert received["tool_id"] == "call-abc"

    def test_tool_name_and_tool_id_are_different(self):
        """tool_name and tool_id must never be the same value when both are present."""
        received = {}
        handler = ResponseHandler(
            on_tool_result=lambda name, out, round_i=None, tool_id=None: received.update(
                {"name": name, "tool_id": tool_id}
            )
        )
        handler.process_chunk(self._make_chunk(tool_name="write_file", tool_id="call-xyz"))
        assert received["name"] != received["tool_id"]
        assert received["name"] == "write_file"
        assert received["tool_id"] == "call-xyz"

    def test_tool_name_falls_back_to_unknown_when_absent(self):
        """When tool_name is None the callback receives 'unknown', not the tool_id."""
        received = {}
        handler = ResponseHandler(
            on_tool_result=lambda name, out, round_i=None, tool_id=None: received.update(
                {"name": name, "tool_id": tool_id}
            )
        )
        handler.process_chunk(self._make_chunk(tool_name=None, tool_id="call-999"))
        assert received["name"] == "unknown"
        assert received["tool_id"] == "call-999"

    def test_tool_id_is_none_when_absent(self):
        """When the chunk carries no tool_id the callback receives None for tool_id."""
        received = {}
        handler = ResponseHandler(
            on_tool_result=lambda name, out, round_i=None, tool_id=None: received.update(
                {"name": name, "tool_id": tool_id}
            )
        )
        handler.process_chunk(self._make_chunk(tool_name="search", tool_id=None))
        assert received["name"] == "search"
        assert received["tool_id"] is None

    def test_output_passed_correctly(self):
        """Tool output is still passed as the second argument unchanged."""
        received = {}
        handler = ResponseHandler(
            on_tool_result=lambda name, out, round_i=None, tool_id=None: received.update({"out": out})
        )
        handler.process_chunk(self._make_chunk(tool_name="t", tool_output={"key": "value"}))
        assert received["out"] == {"key": "value"}

    def test_round_index_passed_correctly(self):
        """round_index is the third argument."""
        received = {}
        handler = ResponseHandler(
            on_tool_result=lambda name, out, round_i=None, tool_id=None: received.update({"round_i": round_i})
        )
        chunk = ResponseChunk(
            type=ChunkType.TOOL_RESULT,
            tool_name="t",
            tool_output="out",
            round_index=2,
        )
        handler.process_chunk(chunk)
        assert received["round_i"] == 2

    def test_regression_tool_name_not_used_as_tool_id(self):
        """Regression guard: tool_id arg must not equal tool_name when both provided."""
        bad_calls = []

        def on_result(name, out, round_i=None, tool_id=None):
            if name == tool_id:
                bad_calls.append((name, tool_id))

        handler = ResponseHandler(on_tool_result=on_result)
        handler.process_chunk(self._make_chunk(tool_name="read_file", tool_id="call-111"))
        assert bad_calls == [], f"tool_name was incorrectly used as tool_id: {bad_calls}"


# ---------------------------------------------------------------------------
# session._display_tool_result tests
# ---------------------------------------------------------------------------


class TestDisplayToolResult:
    """_display_tool_result must pass tool_name and tool_id correctly to context."""

    def _make_session(self):
        """Build a minimal mock session with the real _display_tool_result method."""
        from agentx.session import AgentXSession

        session = MagicMock(spec=AgentXSession)
        # Bind the real method to our mock
        session._display_tool_result = AgentXSession._display_tool_result.__get__(session)
        session._safe_root_after = MagicMock()
        session._write_log = MagicMock()
        session._output_logger = MagicMock()
        session.context = MagicMock()
        session.refresh_working_memory_gui = MagicMock()
        # _display_tool_result now delegates to _streaming_controller; wire it so
        # calls to context.add_tool_result_message are captured through the controller.
        sc = MagicMock()
        sc._display_tool_result.side_effect = lambda tool_name, output, round_index=None, tool_id=None: (
            session.context.add_tool_result_message(
                tool_name=tool_name,
                tool_output=output,
                round_index=round_index,
                tool_id=tool_id,
            )
        )
        session._streaming_controller = sc
        return session

    def test_add_tool_result_message_receives_correct_tool_name(self):
        """context.add_tool_result_message is called with the tool_name arg."""
        session = self._make_session()
        session._display_tool_result("read_file", "file contents", tool_id="call-1")
        session.context.add_tool_result_message.assert_called_once()
        _, kwargs = session.context.add_tool_result_message.call_args
        assert kwargs["tool_name"] == "read_file"

    def test_add_tool_result_message_receives_correct_tool_id(self):
        """context.add_tool_result_message is called with the tool_id arg."""
        session = self._make_session()
        session._display_tool_result("read_file", "file contents", tool_id="call-1")
        _, kwargs = session.context.add_tool_result_message.call_args
        assert kwargs["tool_id"] == "call-1"

    def test_tool_name_and_tool_id_are_distinct_in_context_call(self):
        """tool_name and tool_id passed to context are distinct values."""
        session = self._make_session()
        session._display_tool_result("write_file", "ok", tool_id="call-xyz")
        _, kwargs = session.context.add_tool_result_message.call_args
        assert kwargs["tool_name"] == "write_file"
        assert kwargs["tool_id"] == "call-xyz"
        assert kwargs["tool_name"] != kwargs["tool_id"]

    def test_tool_id_none_when_not_provided(self):
        """tool_id defaults to None when omitted."""
        session = self._make_session()
        session._display_tool_result("search", "results")
        _, kwargs = session.context.add_tool_result_message.call_args
        assert kwargs["tool_id"] is None

    def test_tool_output_passed_to_context(self):
        """The raw output is forwarded to context unchanged."""
        session = self._make_session()
        output = {"lines": ["a", "b"]}
        session._display_tool_result("list_dir", output, tool_id="c1")
        _, kwargs = session.context.add_tool_result_message.call_args
        assert kwargs["tool_output"] == output

    def test_regression_tool_id_not_used_as_tool_name(self):
        """Regression: tool_name arg to context must never equal tool_id string."""
        session = self._make_session()
        session._display_tool_result("read_file", "data", tool_id="call-abc")
        _, kwargs = session.context.add_tool_result_message.call_args
        # The old bug: tool_name=tool_id meaning both were "call-abc"
        assert kwargs["tool_name"] != kwargs["tool_id"]
        assert kwargs["tool_name"] == "read_file"
