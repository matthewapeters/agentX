"""Unit tests for ChatPanel output-panel right-click copy context menu.

Units under test:
  - ``agentx.gui.chat_panel.ChatPanel._on_output_right_click``        (PD-01-AF-010)
  - ``agentx.gui.chat_panel.ChatPanel._show_output_context_menu``     (PD-01-AF-010)
  - ``agentx.gui.chat_panel.ChatPanel._dismiss_output_context_popup`` (PD-01-AF-010)
    - ``agentx.gui.chat_panel.ChatPanel._on_entry_text_right_click``   (PD-01-AF-010)
    - ``agentx.gui.chat_panel.ChatPanel._create_output_entry``         (PD-01-AF-010)
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
        """<Button-3> legacy binding must be present on the output_text widget (backward-compat).

        GIVEN ChatPanel has been created with create_layout()
        WHEN we query the bindings on output_text
        THEN a Button-3 binding is present (legacy fallback handler)

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


# ---------------------------------------------------------------------------
# PD-01-AF-010 — entry-level right-click (visible widgets)
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestEntryLevelRightClickCopy(unittest.TestCase):
    """Unit tests for PD-01-AF-010: right-click on visible entry Text widgets.

    Verifies that ``_create_output_entry`` produces ``detail_text`` and
    ``header_text`` widgets that:
    - are ``tk.Text`` instances (selectable, not ``tk.Label``)
    - carry a ``<Button-3>`` binding that calls ``_on_entry_text_right_click``
    - trigger ``_show_output_context_menu`` with the entry widget as target

    Units under test:
      - ``ChatPanel._create_output_entry``       (PD-01-AF-010)
      - ``ChatPanel._on_entry_text_right_click`` (PD-01-AF-010)
      - ``ChatPanel._show_output_context_menu``  (PD-01-AF-010)
    """

    def setUp(self) -> None:
        """Create a headless Tk root and fully laid-out GUIManager."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.gui._chat_panel._MENU_POST_DELAY_MS = 0
        parent = self.gui.widgets.output_entries_frame
        self.entry = self.gui._create_output_entry(
            parent=parent,
            role_label="Agent",
            icon="🤖",
            content="hello world",
            expanded=True,
        )

    def tearDown(self) -> None:
        """Destroy root after each test."""
        try:
            self.root.destroy()
        except Exception:
            pass

    def _make_entry(self) -> dict:
        """Helper – returns the setUp entry state dict."""
        return self.entry

    # -- header_text is a selectable Text widget ------------------------------

    def test_header_text_is_tk_text_widget(self) -> None:
        """header_text must be a tk.Text widget, not a tk.Label.

        GIVEN an entry is created via _create_output_entry
        WHEN we inspect the 'header_text' key of the state dict
        THEN it is an instance of tk.Text (enables mouse text selection)

        [PD-01-AF-010]
        """
        header_text = self.entry["header_text"]
        assert isinstance(header_text, tk.Text), f"Expected tk.Text, got {type(header_text).__name__}"

    def test_header_text_state_is_disabled(self) -> None:
        """header_text must be read-only (state=DISABLED).

        GIVEN an entry is created via _create_output_entry
        WHEN we inspect the state of header_text
        THEN the state is 'disabled' (content is read-only but still selectable)

        [PD-01-AF-010]
        """
        header_text = self.entry["header_text"]
        assert str(header_text.cget("state")) == "disabled"

    def test_header_text_synced_via_header_var(self) -> None:
        """header_var.set() must update the content of header_text.

        GIVEN an entry is created via _create_output_entry
        WHEN header_var.set('new content') is called
        THEN header_text contains the updated text

        [PD-01-AF-010]
        """
        self.entry["header_var"].set("new content")
        self.root.update()
        content = self.entry["header_text"].get("1.0", tk.END).strip()
        assert content == "new content"

    # -- Button-3 bindings ----------------------------------------------------

    def test_header_text_has_right_click_binding(self) -> None:
        """header_text must have a <Button-3> binding for the copy context menu.

        GIVEN an entry is created via _create_output_entry
        WHEN we inspect bindings on header_text
        THEN '<Button-3>' is present

        [PD-01-AF-010]
        """
        header_text = self.entry["header_text"]
        assert "<Button-3>" in header_text.bind()

    def test_detail_text_has_right_click_binding(self) -> None:
        """detail_text must have a <Button-3> binding for the copy context menu.

        GIVEN an entry is created via _create_output_entry
        WHEN we inspect bindings on detail_text
        THEN '<Button-3>' is present

        [PD-01-AF-010]
        """
        detail_text = self.entry["detail_text"]
        assert "<Button-3>" in detail_text.bind()

    # -- _on_entry_text_right_click creates popup targeting entry widget ------

    def test_right_click_on_detail_text_creates_popup(self) -> None:
        """Right-clicking detail_text must create a popup via _on_entry_text_right_click.

        GIVEN a detail_text widget from a created entry
        WHEN _on_entry_text_right_click is called with that widget
        THEN _output_context_popup is a live tk.Toplevel

        [PD-01-AF-010]
        """
        detail_text: tk.Text = self.entry["detail_text"]
        event = MagicMock()
        event.widget = detail_text
        event.x = 5
        event.y = 5
        self.gui._chat_panel._on_entry_text_right_click(event, detail_text)
        self.root.update()

        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None
        assert popup.winfo_exists()

    def test_right_click_on_header_text_creates_popup(self) -> None:
        """Right-clicking header_text must create a popup via _on_entry_text_right_click.

        GIVEN a header_text widget from a created entry
        WHEN _on_entry_text_right_click is called with that widget
        THEN _output_context_popup is a live tk.Toplevel

        [PD-01-AF-010]
        """
        header_text: tk.Text = self.entry["header_text"]
        event = MagicMock()
        event.widget = header_text
        event.x = 5
        event.y = 5
        self.gui._chat_panel._on_entry_text_right_click(event, header_text)
        self.root.update()

        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None
        assert popup.winfo_exists()

    def test_copy_from_entry_targets_detail_text(self) -> None:
        """Copy action invoked from entry popup must call <<Copy>> on detail_text.

        GIVEN a detail_text widget with selected text
        WHEN _show_output_context_menu is called with detail_text as target
          AND the Copy button is clicked
        THEN <<Copy>> is generated on detail_text (not the hidden output_text)

        [PD-01-AF-010]
        """
        detail_text: tk.Text = self.entry["detail_text"]
        detail_text.config(state=tk.NORMAL)
        detail_text.tag_add(tk.SEL, "1.0", "1.5")
        detail_text.config(state=tk.DISABLED)

        generated_events: list[str] = []
        original_generate = detail_text.event_generate

        def _capture(event_name: str, **kwargs: object) -> None:
            generated_events.append(event_name)
            original_generate(event_name, **kwargs)

        detail_text.event_generate = _capture  # type: ignore[method-assign]

        self.gui._chat_panel._show_output_context_menu(0, 0, target=detail_text)
        self.root.update()

        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None
        for btn in popup.winfo_children()[0].winfo_children():
            if isinstance(btn, tk.Button) and btn.cget("text") == "Copy":
                btn.invoke()
                break

        assert "<<Copy>>" in generated_events


# ---------------------------------------------------------------------------
# PD-01-AF-010 — click-away and grab behaviour
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestOutputPopupDismissBehavior(unittest.TestCase):
    """Unit tests for popup auto-dismiss: outside-click and grab (PD-01-AF-010).

    Verifies that the output context popup is dismissed when the user clicks
    outside it, and that it holds the Tk event grab so keyboard events reach it.

    Units under test:
      - ``ChatPanel._show_output_context_menu`` (PD-01-AF-010)
      - ``ChatPanel._dismiss_output_context_popup`` (PD-01-AF-010)
    """

    def setUp(self) -> None:
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.gui._chat_panel._MENU_POST_DELAY_MS = 0

    def tearDown(self) -> None:
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_outside_click_dismisses_popup(self) -> None:
        """A ButtonPress event outside popup bounds must dismiss it.

        GIVEN the output context popup is visible
        WHEN a ButtonPress is generated at coordinates outside the popup
        THEN the popup is destroyed and _output_context_popup is None

        [PD-01-AF-010]
        """
        self.gui._chat_panel._show_output_context_menu(0, 0)
        self.root.update()
        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None

        popup.event_generate("<ButtonPress>", x=-50, y=-50)
        self.root.update()

        assert self.gui._chat_panel._output_context_popup is None

    def test_inside_click_does_not_dismiss_popup(self) -> None:
        """A ButtonPress event inside popup bounds must NOT dismiss it.

        GIVEN the output context popup is visible
        WHEN a ButtonPress is generated at coordinates inside the popup
        THEN the popup remains present

        [PD-01-AF-010]
        """
        self.gui._chat_panel._show_output_context_menu(0, 0)
        self.root.update()
        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None

        popup.update_idletasks()
        pw = max(popup.winfo_width(), 10)
        ph = max(popup.winfo_height(), 10)
        popup.event_generate("<ButtonPress>", x=pw // 2, y=ph // 2)
        self.root.update()

        assert self.gui._chat_panel._output_context_popup is not None

    def test_popup_holds_grab(self) -> None:
        """The popup must hold the Tk event grab after being shown.

        GIVEN the output context popup is visible
        WHEN we query grab_current() on the root
        THEN it returns the popup Toplevel

        [PD-01-AF-010]
        """
        self.gui._chat_panel._show_output_context_menu(0, 0)
        self.root.update()
        popup = self.gui._chat_panel._output_context_popup
        assert popup is not None
        assert self.root.grab_current() is popup
