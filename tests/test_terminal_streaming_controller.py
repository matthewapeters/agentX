"""Unit tests for terminal-aware tool-result handling in StreamingController.

Units under test:
- ``StreamingController._display_tool_result`` terminal branches [PD-15-AF-003]
- ``StreamingController._display_tool_result`` kill-action wiring [PD-15-AF-004]
"""

from __future__ import annotations

import json
from types import SimpleNamespace
from unittest.mock import MagicMock

import pytest

from agentx.streaming_controller import StreamingController


class _DummyContext:
    """Minimal context double for _display_tool_result tests."""

    def __init__(self) -> None:
        self.add_tool_result_message = MagicMock(
            return_value=SimpleNamespace(file_path=None, message_id="msg_terminal_stub")
        )


class _DummySession:
    """Minimal session double exposing attributes StreamingController needs."""

    def __init__(self) -> None:
        self.gui = MagicMock()
        self.context = _DummyContext()
        self._output_logger = MagicMock()
        self._active_terminal_panes: set[str] = set()
        self._handle_terminal_kill_pane = MagicMock()
        self._update_terminal_status_strip = MagicMock()
        self.refresh_working_memory_gui = MagicMock()
        self._safe_root_after = lambda callback: callback()
        self._write_log = MagicMock()


@pytest.mark.unit
def test_terminal_run_result_tracks_active_pane_and_binds_kill_action() -> None:
    """GIVEN an allowed terminal_run result [PD-15-AF-003, PD-15-AF-004]

    WHEN _display_tool_result processes the JSON payload
    THEN pane id is tracked as active
     AND kill-action wiring is requested on the GUI row.
    """
    session = _DummySession()
    controller = StreamingController(session)

    payload = json.dumps(
        {
            "pane_id": "%14",
            "decision": "allowed",
            "stdout": "DISPATCHED",
            "exit_code": 0,
        }
    )
    controller._display_tool_result("terminal_run", payload)

    assert "%14" in session._active_terminal_panes
    session._update_terminal_status_strip.assert_called_once()
    session.gui.set_tool_result_kill_action.assert_called_once()


@pytest.mark.unit
def test_terminal_kill_result_removes_tracked_pane() -> None:
    """GIVEN a tracked terminal pane [PD-15-AF-003]

    WHEN terminal_kill_pane output is processed
    THEN the pane id is removed from active tracking
     AND the status strip is refreshed.
    """
    session = _DummySession()
    session._active_terminal_panes.add("%33")
    controller = StreamingController(session)

    controller._display_tool_result("terminal_kill_pane", "Killed pane: %33")

    assert "%33" not in session._active_terminal_panes
    session._update_terminal_status_strip.assert_called_once()


@pytest.mark.unit
def test_terminal_run_result_includes_decision_badge() -> None:
    """GIVEN an approved terminal_run result [PD-15-AF-003]

    WHEN _display_tool_result renders the row
    THEN the rendered tool-result line includes the decision badge and exit code.
    """
    session = _DummySession()
    controller = StreamingController(session)

    payload = json.dumps(
        {
            "pane_id": "%21",
            "decision": "approved",
            "stdout": "DISPATCHED",
            "exit_code": 0,
        }
    )
    controller._display_tool_result("terminal_run", payload)

    displayed = session.gui.display_agent_response.call_args[0][0]
    assert "approved" in displayed
    assert "exit 0" in displayed
