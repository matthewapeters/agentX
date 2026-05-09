"""Integration tests for TerminalPane GUI affordances.

Units under test:
- ``InputPanel.set_terminal_status`` [PD-15-AF-003]
- ``ChatPanel.set_tool_result_kill_action`` [PD-15-AF-004]

These tests verify that terminal state is visible in the input strip and that a
terminal tool-result row exposes an actionable kill button.
"""

from __future__ import annotations

import tkinter as tk
import unittest
from datetime import datetime
from unittest.mock import MagicMock

import pytest

from agentx.gui.gui_config import GUIConfig
from agentx.gui.gui_manager import GUIManager


def _make_gui(root: tk.Tk) -> GUIManager:
    """Build a fully laid-out GUIManager attached to *root*."""
    config = GUIConfig.from_dict(
        {
            "ollama_host": "localhost",
            "ollama_model": "test-model",
            "ollama_timeout": 30,
        }
    )
    gui = GUIManager(
        root=root,
        config=config,
        on_submit=MagicMock(),
        on_interrupt=MagicMock(),
        on_attachment_toggle=MagicMock(),
    )
    gui.create_layout()
    return gui


@pytest.mark.integration
class TestTerminalPaneGuiAffordances(unittest.TestCase):
    """Integration tests for PD-15 terminal GUI affordances.

    GIVEN a fully created GUIManager
    WHEN terminal state and tool-result updates are rendered
    THEN terminal affordances appear and invoke callbacks as specified.
    """

    def setUp(self) -> None:
        """Create a headless Tk root and GUI manager."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)

    def tearDown(self) -> None:
        """Destroy Tk root after each test."""
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_terminal_status_strip_reflects_mode_and_active_count(self) -> None:
        """GIVEN terminal status updates [PD-15-AF-003]

        WHEN the session reports supervised and autonomous modes
        THEN the status strip text reflects mode and active-pane count.
        """
        label = self.gui.widgets.terminal_status_label
        self.assertIsNotNone(label)

        self.gui.set_terminal_status(active_panes=2, exec_mode="supervised")
        supervised_text = label.cget("text")
        self.assertIn("Supervised", supervised_text)
        self.assertIn("2 active panes", supervised_text)

        self.gui.set_terminal_status(active_panes=1, exec_mode="autonomous")
        autonomous_text = label.cget("text")
        self.assertIn("Autonomous", autonomous_text)
        self.assertIn("1 active pane", autonomous_text)

    def test_tool_result_row_exposes_kill_button_and_invokes_callback(self) -> None:
        """GIVEN a terminal_run tool result in chat [PD-15-AF-004]

        WHEN the tool-result row is rendered with an allowed pane id
        THEN a kill button is attached to the row
         AND invoking the button calls the kill callback with that pane id.
        """
        on_kill = MagicMock()
        self.gui._on_terminal_kill_pane = on_kill

        self.gui.display_user_message("Run terminal command", attachments=[], timestamp=datetime.now())
        self.gui.display_agent_response('[📋 Tool result: {"pane_id": "%21", "decision": "allowed", "stdout": "ok"}]')

        tool_entry = self.gui._current_turn_entries.get("tool_result")
        self.assertIsNotNone(tool_entry)
        action_button = tool_entry.get("action_button")
        self.assertIsNotNone(action_button)

        action_button.invoke()
        on_kill.assert_called_once_with("%21")

    def test_terminal_mode_button_invokes_toggle_callback(self) -> None:
        """GIVEN terminal mode control button [PD-15-AF-005]

        WHEN the user clicks the mode button in the input strip
        THEN the registered mode-toggle callback is invoked once.
        """
        on_toggle = MagicMock()
        self.gui.set_terminal_mode_toggle_callback(on_toggle)

        button = self.gui.widgets.terminal_mode_button
        self.assertIsNotNone(button)
        button.invoke()

        on_toggle.assert_called_once_with()
