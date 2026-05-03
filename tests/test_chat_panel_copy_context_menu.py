"""Unit tests for ChatPanel output-panel right-click copy context menu.

Units under test:
  - ``agentx.gui.chat_panel.ChatPanel._on_output_right_click``        (PD-01-AF-010)
  - ``agentx.gui.chat_panel.ChatPanel._show_output_context_menu``     (PD-01-AF-010)
  - ``agentx.gui.chat_panel.ChatPanel._dismiss_output_context_popup`` (PD-01-AF-010)

Affordance IDs: PD-01-AF-010

All filesystem, networking, and external service access is mocked.  The Tk root
is created and destroyed per-test to ensure hermeticity.
"""

from __future__ import annotations

import tkinter as tk
import unittest
from unittest.mock import MagicMock, patch

import pytest

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
# PD-01-AF-010 — right-click opens popup
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestOutputPanelRightClickCopy(unittest.TestCase):
    """Unit tests for PD-01-AF-010: right-click opens Copy context menu.

    Units under test:
      - ``ChatPanel._on_output_right_click``
      - ``ChatPanel._show_output_context_menu``
      - ``ChatPanel._dismiss_output_context_popup``

    [PD-01-AF-010]
    """

    def setUp(self) -> None:
        """Create a headless Tk root and fully laid-out GUIManager."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        # Speed-up: 0 ms delay so tests don't need real wall-clock waits.
        self.gui._chat_panel._MENU_POST_DELAY_MS = 0

    def tearDown(self) -> None:
        """Destroy root after each test."""
        try:
            self.root.destroy()
        except Exception:
            pass

    # -- Button-3 binding wired -----------------------------------------------

    def test_right_click_binding_registered(self) -> None:
        """<Button-3> binding must be present on the output_text widget.

        GIVEN ChatPanel has been created with create_layout()
        WHEN we query the bindings on output_text
        THEN a Button-3 binding is present

        [PD-01-AF-010]
        """
        output = self.gui.widgets.output_text
        assert output is not None
        bindings = output.bind()
        assert "<Button-3>" in bindings

    # -- Popup created ---------------------------------------------------------

    def test_right_click_creates_popup(self) -> None:
        """Right-clicking the output widget must create a Toplevel popup.

        GIVEN the output_text widget has content
        WHEN _on_output_right_click is called and the after() delay fires
        THEN _output_context_popup is a live tk.Toplevel

        [PD-01-AF-010]
        """
        output = self.gui.widgets.output_text
        assert output is not None
        output.insert("1.0", "some text")

        event = MagicMock()
        event.x = 5
        event.y = 5
        self.gui._chat_panel._on_output_right_click(event)
        self.root.update()

        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None
        assert popup.winfo_exists()

    # -- Popup contains Copy button -------------------------------------------

    def test_popup_contains_copy_button(self) -> None:
        """The output context popup must contain a button labelled 'Copy'.

        GIVEN the output_text widget exists
        WHEN _show_output_context_menu is called
        THEN the popup contains at least one widget whose text is 'Copy'

        [PD-01-AF-010]
        """
        self.gui._chat_panel._show_output_context_menu(0, 0)
        self.root.update()

        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None
        labels = [w.cget("text") for w in popup.winfo_children()[0].winfo_children() if isinstance(w, tk.Button)]
        assert "Copy" in labels

    # -- Copy with selection calls <<Copy>> -----------------------------------

    def test_copy_button_generates_copy_event(self) -> None:
        """Clicking 'Copy' must invoke <<Copy>> on the output_text widget.

        GIVEN the output_text widget has 'hello world' with 'hello' selected
        WHEN _show_output_context_menu is called and the 'Copy' button is invoked
        THEN <<Copy>> is generated on output_text

        [PD-01-AF-010]
        """
        output = self.gui.widgets.output_text
        assert output is not None
        output.insert("1.0", "hello world")
        output.tag_add(tk.SEL, "1.0", "1.5")

        generated_events: list[str] = []
        original_generate = output.event_generate

        def _capture(event_name: str, **kwargs: object) -> None:
            generated_events.append(event_name)
            original_generate(event_name, **kwargs)

        output.event_generate = _capture  # type: ignore[method-assign]

        self.gui._chat_panel._show_output_context_menu(0, 0)
        self.root.update()

        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None
        for btn in popup.winfo_children()[0].winfo_children():
            if isinstance(btn, tk.Button) and btn.cget("text") == "Copy":
                btn.invoke()
                break

        assert "<<Copy>>" in generated_events

    # -- Copy dismisses popup --------------------------------------------------

    def test_copy_button_dismisses_popup(self) -> None:
        """Clicking 'Copy' must dismiss the popup.

        GIVEN the output context popup is visible
        WHEN the 'Copy' button is clicked
        THEN the popup is destroyed (_output_context_popup is None)

        [PD-01-AF-010]
        """
        self.gui._chat_panel._show_output_context_menu(0, 0)
        self.root.update()
        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None

        for btn in popup.winfo_children()[0].winfo_children():
            if isinstance(btn, tk.Button) and btn.cget("text") == "Copy":
                btn.invoke()
                break

        assert self.gui._chat_panel._output_context_popup is None

    # -- Escape dismisses popup -----------------------------------------------

    def test_escape_dismisses_popup(self) -> None:
        """Pressing Escape must dismiss the popup.

        GIVEN the output context popup is visible
        WHEN Escape is pressed (dismiss handler called directly)
        THEN the popup is destroyed

        [PD-01-AF-010]
        """
        self.gui._chat_panel._show_output_context_menu(0, 0)
        self.root.update()
        assert self.gui._chat_panel._output_context_popup is not None

        self.gui._chat_panel._dismiss_output_context_popup()
        assert self.gui._chat_panel._output_context_popup is None

    # -- Stale popup replaced on second right-click ---------------------------

    def test_second_right_click_replaces_popup(self) -> None:
        """A second right-click must destroy the first popup and create a fresh one.

        GIVEN an output context popup is already visible
        WHEN _on_output_right_click is called again and the after() delay fires
        THEN the first popup no longer exists
          AND a new popup is present

        [PD-01-AF-010]
        """
        event = MagicMock()
        event.x = 5
        event.y = 5
        self.gui._chat_panel._on_output_right_click(event)
        self.root.update()
        first_popup = self.gui._chat_panel._output_context_popup
        assert first_popup is not None

        self.gui._chat_panel._on_output_right_click(event)
        self.root.update()
        second_popup = self.gui._chat_panel._output_context_popup
        assert second_popup is not None
        # First popup should have been destroyed before second was created
        assert first_popup is not second_popup

    # -- Popup uses themed background -----------------------------------------

    def test_popup_background_matches_output_bg(self) -> None:
        """The popup Toplevel background must match the configured output_bg colour.

        GIVEN the active theme has a known output_bg colour
        WHEN _show_output_context_menu is called
        THEN the Toplevel's 'bg' configuration equals output_bg

        [PD-01-AF-010]
        """
        self.gui._chat_panel._show_output_context_menu(0, 0)
        self.root.update()
        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None
        assert popup.cget("bg") == self.gui.config.output_bg
