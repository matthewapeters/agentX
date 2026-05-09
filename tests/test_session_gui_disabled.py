"""Tests for enable_gui_chat=false runtime path.

GIVEN an AgentX configuration with GUI disabled
WHEN AgentXSession initializes
THEN it uses NullGUIManager and remains operational for headless flows.
"""

from __future__ import annotations

import tkinter as tk
from unittest.mock import MagicMock, patch

from agentx.igui_manager import NullGUIManager
from agentx.session import AgentXSession


def _minimal_config(enable_gui_chat: bool, tui_enable: bool = True) -> dict:
    """Create a minimal valid session config for GUI toggle tests.

    Args:
        enable_gui_chat: Whether GUI chat is enabled.
        tui_enable: Whether TUI mode is enabled.

    Returns:
        Minimal config dictionary accepted by AgentXSession.
    """
    return {
        "agentx": {
            "ollama_model": "gpt-oss:latest",
            "ollama_host": "localhost:11434",
            "enable_gui_chat": enable_gui_chat,
        },
        "tui": {
            "enable": tui_enable,
        },
        "agentix": {
            "host": "localhost:8000",
        },
    }


def test_session_uses_null_gui_manager_when_gui_disabled(tmp_path) -> None:
    """GIVEN enable_gui_chat=false WHEN session initializes THEN NullGUIManager is used."""
    root = tk.Tk(useTk=False)
    config = _minimal_config(enable_gui_chat=False, tui_enable=True)

    with patch("agentx.session.GUIManager") as mock_gui_class:
        session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))

    assert isinstance(session.gui, NullGUIManager)
    assert session._enable_gui_chat is False
    mock_gui_class.assert_not_called()

    session.close()


def test_terminal_approval_is_denied_in_headless_mode(tmp_path) -> None:
    """GIVEN headless mode WHEN terminal approval is requested THEN command is denied."""
    root = tk.Tk(useTk=False)
    config = _minimal_config(enable_gui_chat=False, tui_enable=True)

    session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))
    approved, command = session._request_terminal_approval("git commit -m 'x'", "test")

    assert approved is False
    assert command == "git commit -m 'x'"

    session.close()


def test_layout_skips_gui_setup_when_disabled(tmp_path) -> None:
    """GIVEN headless mode WHEN layout is called THEN no GUI setup methods are invoked."""
    root = tk.Tk(useTk=False)
    config = _minimal_config(enable_gui_chat=False, tui_enable=True)

    session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))
    session.gui.create_layout = MagicMock()  # type: ignore[method-assign]

    session.layout()

    session.gui.create_layout.assert_not_called()
    session.close()
