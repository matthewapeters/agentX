"""Unit tests for terminal mode toggling and approval callback behavior.

Units under test:
- ``AgentXSession._handle_terminal_mode_toggle`` [PD-15-AF-005]
- ``AgentXSession._request_terminal_approval`` [PD-15-AF-006]
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

from agentx.session import AgentXSession


@pytest.mark.unit
def test_handle_terminal_mode_toggle_requires_confirmation_for_autonomous() -> None:
    """GIVEN supervised mode [PD-15-AF-005]

    WHEN mode toggle is clicked and user confirms
    THEN target mode is set to autonomous.
    """
    session = object.__new__(AgentXSession)
    session.root = MagicMock()
    session.config = {"terminal": {"exec_mode": "supervised"}}
    session._get_terminal_exec_mode = MagicMock(return_value="supervised")
    session._apply_terminal_exec_mode = MagicMock()

    with patch("agentx.session.messagebox.askyesno", return_value=True):
        session._handle_terminal_mode_toggle()

    session._apply_terminal_exec_mode.assert_called_once_with("autonomous")


@pytest.mark.unit
def test_handle_terminal_mode_toggle_cancel_keeps_supervised() -> None:
    """GIVEN supervised mode [PD-15-AF-005]

    WHEN mode toggle is clicked and user rejects confirmation
    THEN mode-apply callback is not invoked.
    """
    session = object.__new__(AgentXSession)
    session.root = MagicMock()
    session.config = {"terminal": {"exec_mode": "supervised"}}
    session._get_terminal_exec_mode = MagicMock(return_value="supervised")
    session._apply_terminal_exec_mode = MagicMock()

    with patch("agentx.session.messagebox.askyesno", return_value=False):
        session._handle_terminal_mode_toggle()

    session._apply_terminal_exec_mode.assert_not_called()


@pytest.mark.unit
def test_request_terminal_approval_delegates_to_dialog_on_main_thread() -> None:
    """GIVEN a terminal command in supervised mode [PD-15-AF-006]

    WHEN approval is requested on main thread
    THEN session delegates to the approval dialog and returns its result.
    """
    session = object.__new__(AgentXSession)
    session._show_terminal_approval_dialog = MagicMock(return_value=(True, "git status"))

    approved, command = session._request_terminal_approval("git status", "check repo state")

    assert approved is True
    assert command == "git status"
    session._show_terminal_approval_dialog.assert_called_once_with("git status", "check repo state")


@pytest.mark.unit
def test_on_setting_change_terminal_lists_reload_runtime_config() -> None:
    """GIVEN terminal allow-list update [PD-15-AF-007]

    WHEN settings handler processes terminal key change
    THEN runtime bridge config reload helper is invoked.
    """
    session = object.__new__(AgentXSession)
    session.config = {"terminal": {"allow": ["ls"]}}
    session.gui = MagicMock()
    session.gui.config = MagicMock()
    session._update_terminal_status_strip = MagicMock()

    with (
        patch("agentx.session.save_config"),
        patch(
            "agentx.integration.terminal_bridge.reload_terminal_config",
            return_value=True,
        ) as mocked_reload,
    ):
        session._on_setting_change(["terminal", "allow"], ["pwd", "ls"])

    mocked_reload.assert_called_once_with(session.config)
