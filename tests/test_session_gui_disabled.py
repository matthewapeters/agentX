"""Tests for enable_gui_chat=false runtime path.

GIVEN an AgentX configuration with GUI disabled
WHEN AgentXSession initializes
THEN it uses NullGUIManager and remains operational for headless flows.
"""

from __future__ import annotations

import time
import tkinter as tk
from unittest.mock import MagicMock, patch

from agentx.event_broker import EventType
from agentx.igui_manager import NullGUIManager
from agentx.session import AgentXSession
from agentx.streaming_controller import StreamingController


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


def test_tui_quit_callback_requests_mainloop_shutdown(tmp_path) -> None:
    """GIVEN TUI quit affordance WHEN callback runs.

    THEN session interrupts streaming and requests root quit. [PD-16-AF-008]
    """
    root = tk.Tk(useTk=False)
    root.quit = MagicMock()  # type: ignore[method-assign]
    config = _minimal_config(enable_gui_chat=False, tui_enable=True)

    session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))
    session._safe_root_after = lambda callback: callback()
    session.tui_bridge = MagicMock()
    session._is_streaming.set()

    session._on_tui_quit()

    assert session._is_streaming.is_set() is False
    root.quit.assert_called_once()
    session.tui_bridge.write_output.assert_called_once()
    session.close()


def test_tui_enabled_session_stops_tui_resources_on_close(tmp_path) -> None:
    """GIVEN TUI enabled WHEN session closes THEN TUI runtime resources are stopped exactly once."""
    root = tk.Tk(useTk=False)
    config = _minimal_config(enable_gui_chat=False, tui_enable=True)

    with (
        patch("agentx.session.TuiBridge") as mock_bridge_class,
        patch("agentx.session.TUIEventSubscriber") as mock_subscriber_class,
    ):
        bridge = MagicMock()
        bridge.is_enabled = True
        subscriber = MagicMock()
        mock_bridge_class.return_value = bridge
        mock_subscriber_class.return_value = subscriber

        session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))
        session.close()

    mock_bridge_class.assert_called_once()
    mock_subscriber_class.assert_called_once()
    bridge.start.assert_called_once()
    subscriber.start.assert_called_once()
    bridge.stop.assert_called_once()
    subscriber.stop.assert_called_once()


def test_tui_submit_callback_schedules_stream_with_clean_prompt(tmp_path) -> None:
    """GIVEN TUI submit text WHEN callback runs THEN cleaned prompt is queued and stream path starts."""
    root = tk.Tk(useTk=False)
    config = _minimal_config(enable_gui_chat=False, tui_enable=True)

    session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))
    session._safe_root_after = lambda callback: callback()
    session.stream_ollama_response = MagicMock()

    session._on_tui_submit("  hello from tui  ")

    assert session._pending_prompt == "hello from tui"
    session.stream_ollama_response.assert_called_once()
    session.close()


def test_tui_submit_round_trip_emits_user_agent_and_done_records(tmp_path) -> None:
    """GIVEN a TUI submit WHEN session processes one turn THEN output contains user, agent, and done lifecycle markers."""
    root = tk.Tk(useTk=False)
    config = _minimal_config(enable_gui_chat=False, tui_enable=True)

    with patch("agentx.session.TuiBridge") as mock_bridge_class:
        bridge = MagicMock()
        bridge.is_enabled = True
        bridge.records = []

        def _capture(record: str) -> bool:
            bridge.records.append(record)
            return True

        bridge.write_output.side_effect = _capture
        mock_bridge_class.return_value = bridge

        session = AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))
        session._safe_root_after = lambda callback: callback()

        def _fake_stream_once() -> None:
            session.event_broker.publish(
                EventType.USER_MESSAGE, {"text": session._pending_prompt, "timestamp": "10:00:00"}
            )
            controller = StreamingController(session)
            controller._write_tui_output("###AGENT\n")
            controller._write_tui_output("echo response")
            controller._write_tui_output("###DONE\n")

        session.stream_ollama_response = MagicMock(side_effect=_fake_stream_once)

        session._on_tui_submit("  hello sprint2  ")

        deadline = time.time() + 1.5
        while len(bridge.records) < 4 and time.time() < deadline:
            time.sleep(0.01)

        combined = "".join(bridge.records)
        assert "###USER 10:00:00" in combined
        assert "hello sprint2" in combined
        assert "###AGENT 🤖\n" in combined
        assert "echo response" in combined
        assert "###DONE\n" in combined
        assert combined.find("###AGENT 🤖\n") < combined.find("###DONE\n")

        session.close()
