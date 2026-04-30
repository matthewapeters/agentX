"""Unit tests for ContextRenderer message-enabled checkbox, PD-03-AF-007.

Units under test:
  - ``agentx.gui.context_renderer.ContextRenderer._render_message_to_grid``

The enabled checkbox (column 1 of every message row) must:
  1. Reflect the initial value of ``message.enabled``.
  2. Update ``message.enabled`` in-place when toggled.

Affordance ID: PD-03-AF-007
"""

from __future__ import annotations

import os
import sys
import tkinter as tk
import unittest

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agentx.gui.context_renderer import ContextRenderer
from agentx.gui.gui_config import GUIConfig
from agentx.gui.gui_manager import GUIManager
from shared.models.message import Message, MessageRole

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_gui(root: tk.Tk) -> GUIManager:
    """Build a GUIManager (no layout required — we test ContextRenderer directly)."""
    config = GUIConfig.from_dict(
        {
            "ollama_host": "localhost",
            "ollama_model": "test-model",
            "ollama_timeout": 30,
        }
    )
    from unittest.mock import MagicMock

    return GUIManager(
        root=root,
        config=config,
        on_submit=MagicMock(),
        on_interrupt=MagicMock(),
        on_attachment_toggle=MagicMock(),
    )


def _make_message(enabled: bool = True) -> Message:
    """Create a minimal user Message with the given enabled state."""
    return Message(role=MessageRole.USER, content="hello world", enabled=enabled)


def _find_checkbutton_in_column(frame: tk.Frame, col: int) -> tk.Checkbutton | None:
    """Return the first ``tk.Checkbutton`` gridded in *col* inside *frame*."""
    for widget in frame.winfo_children():
        info = widget.grid_info()
        if info and int(info.get("column", -1)) == col and isinstance(widget, tk.Checkbutton):
            return widget
    return None


# MESSAGE_COLUMNS["enabled"] is column 1
_ENABLED_COL = ContextRenderer.MESSAGE_COLUMNS["enabled"]


# ---------------------------------------------------------------------------
# PD-03-AF-007 tests
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestMessageEnabledCheckbox(unittest.TestCase):
    """PD-03-AF-007: Message enabled checkbox reflects and drives message.enabled.

    Unit under test: ``ContextRenderer._render_message_to_grid``.
    """

    def setUp(self) -> None:
        """Set up a headless Tkinter root and a bare parent frame."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.renderer: ContextRenderer = self.gui._context_renderer
        self.frame = tk.Frame(self.root)

    def tearDown(self) -> None:
        """Destroy the Tkinter root after each test."""
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_enabled_checkbox_initial_true(self) -> None:
        """Checkbox is checked when message.enabled is True at render time.

        PD-03-AF-007

        GIVEN a Message with enabled=True
        WHEN  the message row is rendered via _render_message_to_grid()
        THEN  a Checkbutton is present in the enabled column
          AND the Checkbutton variable reports True
        """
        msg = _make_message(enabled=True)
        self.renderer._render_message_to_grid(msg, self.frame, 0)

        cb = _find_checkbutton_in_column(self.frame, _ENABLED_COL)
        self.assertIsNotNone(cb, "Expected a Checkbutton in the enabled column")
        var = cb.cget("variable")
        self.assertTrue(self.frame.tk.getboolean(self.root.getvar(str(var))), "Checkbox should be checked (True)")

    def test_enabled_checkbox_initial_false(self) -> None:
        """Checkbox is unchecked when message.enabled is False at render time.

        PD-03-AF-007

        GIVEN a Message with enabled=False
        WHEN  the message row is rendered via _render_message_to_grid()
        THEN  the Checkbutton variable reports False
        """
        msg = _make_message(enabled=False)
        self.renderer._render_message_to_grid(msg, self.frame, 0)

        cb = _find_checkbutton_in_column(self.frame, _ENABLED_COL)
        self.assertIsNotNone(cb)
        var = cb.cget("variable")
        self.assertFalse(self.frame.tk.getboolean(self.root.getvar(str(var))), "Checkbox should be unchecked (False)")

    def test_uncheck_sets_message_enabled_false(self) -> None:
        """Invoking the checkbox (checked→unchecked) sets message.enabled=False.

        PD-03-AF-007

        GIVEN a Message with enabled=True rendered in a frame
        WHEN  the Checkbutton is invoked (checked → unchecked)
        THEN  message.enabled is False
        """
        msg = _make_message(enabled=True)
        self.renderer._render_message_to_grid(msg, self.frame, 0)

        cb = _find_checkbutton_in_column(self.frame, _ENABLED_COL)
        self.assertIsNotNone(cb)
        cb.invoke()  # True → False

        self.assertFalse(msg.enabled, "message.enabled should be False after unchecking")

    def test_check_sets_message_enabled_true(self) -> None:
        """Invoking the checkbox (unchecked→checked) sets message.enabled=True.

        PD-03-AF-007

        GIVEN a Message with enabled=False rendered in a frame
        WHEN  the Checkbutton is invoked (unchecked → checked)
        THEN  message.enabled is True
        """
        msg = _make_message(enabled=False)
        self.renderer._render_message_to_grid(msg, self.frame, 0)

        cb = _find_checkbutton_in_column(self.frame, _ENABLED_COL)
        self.assertIsNotNone(cb)
        cb.invoke()  # False → True

        self.assertTrue(msg.enabled, "message.enabled should be True after checking")
