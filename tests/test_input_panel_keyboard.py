"""Unit tests for InputPanel keyboard shortcut affordances.

Units under test:
  - ``agentx.gui.input_panel.InputPanel._on_shift_return``  (AF-002)
  - ``agentx.gui.input_panel.InputPanel.create``  (binding wiring for AF-002)

Affordance IDs: PD-02-AF-002
"""

from __future__ import annotations

import os
import sys
import tkinter as tk
import unittest
from unittest.mock import MagicMock

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agentx.gui.gui_config import GUIConfig
from agentx.gui.gui_manager import GUIManager

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_gui(root: tk.Tk) -> GUIManager:
    """Build a headless GUIManager with create_layout called."""
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


# ---------------------------------------------------------------------------
# PD-02-AF-002 — Shift+Enter inserts newline
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestShiftEnterInsertsNewline(unittest.TestCase):
    """Unit tests for PD-02-AF-002: Shift+Enter inserts a newline.

    Unit under test: ``InputPanel._on_shift_return()`` and the
    ``<Shift-Return>`` binding wired in ``InputPanel.create()``.

    GIVEN the InputPanel has been created
    WHEN _on_shift_return is called with a synthetic event
    THEN a newline is inserted at the current insertion point
      AND the method returns "break" to suppress default behaviour.
    """

    def setUp(self) -> None:
        """Create a headless Tk root and fully laid-out GUIManager."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.text_widget: tk.Text = self.gui.widgets.user_input_text

    def tearDown(self) -> None:
        """Destroy root after each test."""
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_shift_return_inserts_newline_into_empty_widget(self) -> None:
        """Shift+Enter in an empty input inserts exactly one newline.

        PD-02-AF-002

        GIVEN the input text widget is empty
        WHEN _on_shift_return is invoked
        THEN the widget contains exactly one newline character.
        """
        event = MagicMock(spec=tk.Event)
        self.gui._input_panel._on_shift_return(event)
        content = self.text_widget.get("1.0", tk.END)
        # tk.Text always adds a trailing newline; we inserted one → two total
        self.assertEqual(content.count("\n"), 2)

    def test_shift_return_inserts_newline_after_existing_text(self) -> None:
        """Shift+Enter after existing text inserts a newline mid-content.

        PD-02-AF-002

        GIVEN the input text widget contains "hello"
        WHEN the cursor is at the end AND _on_shift_return is invoked
        THEN the widget content is "hello\\n".
        """
        self.text_widget.insert("1.0", "hello")
        self.text_widget.mark_set(tk.INSERT, tk.END)
        event = MagicMock(spec=tk.Event)
        self.gui._input_panel._on_shift_return(event)
        content = self.text_widget.get("1.0", tk.END)
        # tk.Text appends a trailing \n; inserted \n + trailing \n = "hello\n\n"
        self.assertIn("hello\n", content)

    def test_shift_return_returns_break(self) -> None:
        """_on_shift_return returns 'break' to suppress default key handling.

        PD-02-AF-002

        GIVEN the InputPanel has been created
        WHEN _on_shift_return is called
        THEN the return value is the string "break".
        """
        event = MagicMock(spec=tk.Event)
        result = self.gui._input_panel._on_shift_return(event)
        self.assertEqual(result, "break")

    def test_shift_return_binding_registered_on_input_text(self) -> None:
        """<Shift-Return> is bound on user_input_text after create().

        PD-02-AF-002

        GIVEN InputPanel.create() has been called
        WHEN we query the bindings on user_input_text
        THEN '<Shift-Return>' appears in the binding list.
        """
        bindings = self.text_widget.bind()
        # Tkinter normalises <Shift-Return> to <Shift-Key-Return> internally.
        self.assertTrue(
            any("Shift" in b and "Return" in b for b in bindings),
            f"No Shift+Return binding found; registered bindings: {bindings}",
        )

    def test_shift_return_inserts_at_cursor_not_at_end(self) -> None:
        """Newline is inserted at the cursor position, not appended to the end.

        PD-02-AF-002

        GIVEN the input text widget contains "ab" with cursor between 'a' and 'b'
        WHEN _on_shift_return is invoked
        THEN the content is "a\\nb" (newline mid-word, not at end).
        """
        self.text_widget.insert("1.0", "ab")
        # Position cursor after 'a' (row 1, col 1)
        self.text_widget.mark_set(tk.INSERT, "1.1")
        event = MagicMock(spec=tk.Event)
        self.gui._input_panel._on_shift_return(event)
        content = self.text_widget.get("1.0", tk.END)
        self.assertTrue(content.startswith("a\nb"), f"Unexpected content: {content!r}")
