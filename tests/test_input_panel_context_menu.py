"""Unit tests for InputPanel right-click context menu affordances.

Units under test:
  - ``agentx.gui.input_panel.InputPanel._on_input_right_click``        (PD-02-AF-008)
  - ``agentx.gui.input_panel.InputPanel._dismiss_input_context_popup`` (PD-02-AF-008)
  - ``agentx.gui.input_panel.InputPanel._clipboard_has_content``       (PD-02-AF-010)
  - ``agentx.gui.input_panel.InputPanel._show_input_context_menu``     (PD-02-AF-008..012)
  - ``agentx.gui.input_panel.InputPanel._on_input_context_copy``       (PD-02-AF-011)
  - ``agentx.gui.input_panel.InputPanel._on_input_context_paste``      (PD-02-AF-012)

Affordance IDs: PD-02-AF-008, PD-02-AF-009, PD-02-AF-010, PD-02-AF-011, PD-02-AF-012

All filesystem, networking, and external service access is mocked.  The Tk root
is created and destroyed per-test to ensure hermeticity.
"""

from __future__ import annotations

import tkinter as tk
import unittest
from unittest.mock import MagicMock

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


def _put_on_clipboard(root: tk.Tk, text: str) -> None:
    """Place *text* on the Tk clipboard."""
    root.clipboard_clear()
    root.clipboard_append(text)


def _clear_clipboard(root: tk.Tk) -> None:
    """Empty the Tk clipboard (clipboard_get() will raise TclError afterwards)."""
    root.clipboard_clear()


# ---------------------------------------------------------------------------
# PD-02-AF-008 — right-click opens popup
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestInputPanelRightClickPopup(unittest.TestCase):
    """Tests for PD-02-AF-008: right-click opens a Wayland-safe context popup.

    Units under test: ``InputPanel._on_input_right_click``,
    ``InputPanel._show_input_context_menu``,
    ``InputPanel._dismiss_input_context_popup``.

    [PD-02-AF-008]
    """

    def setUp(self) -> None:
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        # 0 ms delay so tests don't need real wall-clock waits
        self.gui._input_panel._MENU_POST_DELAY_MS = 0

    def tearDown(self) -> None:
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_right_click_binding_registered(self) -> None:
        """<Button-3> binding must be present on the user_input_text widget.

        GIVEN InputPanel.create() has been called
        WHEN we query the bindings on user_input_text
        THEN a Button-3 binding is present

        [PD-02-AF-008]
        """
        widget = self.gui.widgets.user_input_text
        assert widget is not None
        assert "<Button-3>" in widget.bind()

    def test_right_click_with_selection_creates_popup(self) -> None:
        """Right-clicking with selected text must create a popup.

        GIVEN the input widget contains 'hello world' with 'hello' selected
        WHEN _on_input_right_click is called and the after() delay fires
        THEN _input_context_popup is a live tk.Toplevel

        [PD-02-AF-008]
        """
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello world")
        widget.tag_add(tk.SEL, "1.0", "1.5")

        event = MagicMock()
        event.x = 5
        event.y = 5
        self.gui._input_panel._on_input_right_click(event)
        self.root.update()

        popup = self.gui._input_panel._input_context_popup
        assert popup is not None
        assert popup.winfo_exists()

    def test_escape_dismisses_popup(self) -> None:
        """Pressing Escape (dismiss handler) must destroy the popup.

        GIVEN the input context popup is visible
        WHEN _dismiss_input_context_popup is called
        THEN the popup is destroyed and _input_context_popup is None

        [PD-02-AF-008]
        """
        _put_on_clipboard(self.root, "clipboard content")
        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        assert self.gui._input_panel._input_context_popup is not None

        self.gui._input_panel._dismiss_input_context_popup()
        assert self.gui._input_panel._input_context_popup is None

    def test_second_right_click_replaces_popup(self) -> None:
        """A second right-click must destroy the first popup and create a fresh one.

        GIVEN an input context popup is already visible
        WHEN _on_input_right_click is called again and the delay fires
        THEN the first popup no longer exists
          AND a new popup is present

        [PD-02-AF-008]
        """
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello world")
        widget.tag_add(tk.SEL, "1.0", "1.5")

        event = MagicMock()
        event.x = 5
        event.y = 5
        self.gui._input_panel._on_input_right_click(event)
        self.root.update()
        first_popup = self.gui._input_panel._input_context_popup
        assert first_popup is not None

        self.gui._input_panel._on_input_right_click(event)
        self.root.update()
        second_popup = self.gui._input_panel._input_context_popup
        assert second_popup is not None
        assert first_popup is not second_popup

    def test_popup_background_matches_input_bg(self) -> None:
        """The popup Toplevel background must match the configured input_bg colour.

        GIVEN the active theme has a known input_bg colour
        WHEN _show_input_context_menu is called (with clipboard content so popup shows)
        THEN the Toplevel's 'bg' configuration equals input_bg

        [PD-02-AF-008]
        """
        _put_on_clipboard(self.root, "some text")
        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        popup = self.gui._input_panel._input_context_popup
        assert popup is not None
        assert popup.cget("bg") == self.gui.config.input_bg

    def test_no_popup_when_nothing_applicable(self) -> None:
        """If neither Copy nor Paste is applicable the popup must not be created.

        GIVEN the input widget has no selection
          AND the clipboard is empty
        WHEN _show_input_context_menu is called
        THEN no popup is created (_input_context_popup is None)

        [PD-02-AF-008]
        """
        _clear_clipboard(self.root)
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello world")
        # Ensure no selection
        widget.tag_remove(tk.SEL, "1.0", tk.END)

        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        assert self.gui._input_panel._input_context_popup is None


# ---------------------------------------------------------------------------
# PD-02-AF-009 — Copy item visibility
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestInputCopyMenuVisibility(unittest.TestCase):
    """Tests for PD-02-AF-009: 'Copy' item visible only when text is selected.

    [PD-02-AF-009]
    """

    def setUp(self) -> None:
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.gui._input_panel._MENU_POST_DELAY_MS = 0

    def tearDown(self) -> None:
        try:
            self.root.destroy()
        except Exception:
            pass

    def _popup_button_labels(self) -> list[str]:
        popup = self.gui._input_panel._input_context_popup
        if popup is None:
            return []
        frame = popup.winfo_children()[0]
        return [w.cget("text") for w in frame.winfo_children() if isinstance(w, tk.Button)]

    def test_copy_item_present_when_text_selected(self) -> None:
        """'Copy' must appear in popup when the SEL tag exists.

        GIVEN user_input_text contains 'hello world' with 'hello' selected
        WHEN _show_input_context_menu is called
        THEN the popup contains a 'Copy' button

        [PD-02-AF-009]
        """
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello world")
        widget.tag_add(tk.SEL, "1.0", "1.5")

        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        assert "Copy" in self._popup_button_labels()

    def test_copy_item_absent_when_no_selection(self) -> None:
        """'Copy' must NOT appear in popup when no text is selected.

        GIVEN user_input_text has no SEL tag
          AND the clipboard is non-empty (so popup still shows)
        WHEN _show_input_context_menu is called
        THEN the popup does NOT contain a 'Copy' button

        [PD-02-AF-009]
        """
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello world")
        widget.tag_remove(tk.SEL, "1.0", tk.END)
        _put_on_clipboard(self.root, "something")

        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        assert "Copy" not in self._popup_button_labels()


# ---------------------------------------------------------------------------
# PD-02-AF-010 — Paste item visibility
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestInputPasteMenuVisibility(unittest.TestCase):
    """Tests for PD-02-AF-010: 'Paste' visible only when clipboard is non-empty.

    [PD-02-AF-010]
    """

    def setUp(self) -> None:
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.gui._input_panel._MENU_POST_DELAY_MS = 0

    def tearDown(self) -> None:
        try:
            self.root.destroy()
        except Exception:
            pass

    def _popup_button_labels(self) -> list[str]:
        popup = self.gui._input_panel._input_context_popup
        if popup is None:
            return []
        frame = popup.winfo_children()[0]
        return [w.cget("text") for w in frame.winfo_children() if isinstance(w, tk.Button)]

    def test_paste_item_present_when_clipboard_non_empty(self) -> None:
        """'Paste' must appear in popup when clipboard contains text.

        GIVEN the system clipboard contains 'world'
        WHEN _show_input_context_menu is called
        THEN the popup contains a 'Paste' button

        [PD-02-AF-010]
        """
        _put_on_clipboard(self.root, "world")
        # Add selection so popup renders
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello")
        widget.tag_add(tk.SEL, "1.0", "1.5")

        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        assert "Paste" in self._popup_button_labels()

    def test_paste_item_absent_when_clipboard_empty(self) -> None:
        """'Paste' must NOT appear in popup when clipboard is empty.

        GIVEN the system clipboard is empty (clipboard_get() raises TclError)
          AND the input has text selected (so popup still shows)
        WHEN _show_input_context_menu is called
        THEN the popup does NOT contain a 'Paste' button

        [PD-02-AF-010]
        """
        _clear_clipboard(self.root)
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello world")
        widget.tag_add(tk.SEL, "1.0", "1.5")

        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        assert "Paste" not in self._popup_button_labels()

    def test_clipboard_has_content_returns_false_on_empty(self) -> None:
        """_clipboard_has_content must return False when clipboard is empty.

        GIVEN the clipboard is empty
        WHEN _clipboard_has_content() is called
        THEN it returns False (no TclError propagated)

        [PD-02-AF-010]
        """
        _clear_clipboard(self.root)
        result = self.gui._input_panel._clipboard_has_content()
        assert result is False

    def test_clipboard_has_content_returns_true_when_filled(self) -> None:
        """_clipboard_has_content must return True when clipboard is non-empty.

        GIVEN the clipboard contains 'hello'
        WHEN _clipboard_has_content() is called
        THEN it returns True

        [PD-02-AF-010]
        """
        _put_on_clipboard(self.root, "hello")
        result = self.gui._input_panel._clipboard_has_content()
        assert result is True


# ---------------------------------------------------------------------------
# PD-02-AF-011 — Copy action
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestInputCopyAction(unittest.TestCase):
    """Tests for PD-02-AF-011: 'Copy' button copies selection to clipboard.

    [PD-02-AF-011]
    """

    def setUp(self) -> None:
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.gui._input_panel._MENU_POST_DELAY_MS = 0

    def tearDown(self) -> None:
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_copy_action_copies_selection_to_clipboard(self) -> None:
        """Invoking 'Copy' must place the selected text on the clipboard.

        GIVEN user_input_text contains 'hello world' with 'hello' selected
        WHEN the 'Copy' button is invoked from the context popup
        THEN the system clipboard contains 'hello'
          AND the input text is unchanged

        [PD-02-AF-011]
        """
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello world")
        widget.tag_add(tk.SEL, "1.0", "1.5")

        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()

        popup = self.gui._input_panel._input_context_popup
        assert popup is not None
        frame = popup.winfo_children()[0]
        for btn in frame.winfo_children():
            if isinstance(btn, tk.Button) and btn.cget("text") == "Copy":
                btn.invoke()
                break

        clipboard = self.root.clipboard_get()
        assert clipboard == "hello"
        assert widget.get("1.0", tk.END).strip() == "hello world"

    def test_copy_action_dismisses_popup(self) -> None:
        """Invoking 'Copy' must dismiss the popup.

        GIVEN the input context popup is visible
        WHEN the 'Copy' button is clicked
        THEN _input_context_popup is None

        [PD-02-AF-011]
        """
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello world")
        widget.tag_add(tk.SEL, "1.0", "1.5")

        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        popup = self.gui._input_panel._input_context_popup
        assert popup is not None

        frame = popup.winfo_children()[0]
        for btn in frame.winfo_children():
            if isinstance(btn, tk.Button) and btn.cget("text") == "Copy":
                btn.invoke()
                break

        assert self.gui._input_panel._input_context_popup is None


# ---------------------------------------------------------------------------
# PD-02-AF-012 — Paste action
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestInputPasteAction(unittest.TestCase):
    """Tests for PD-02-AF-012: 'Paste' replaces selection or inserts at cursor.

    [PD-02-AF-012]
    """

    def setUp(self) -> None:
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.gui._input_panel._MENU_POST_DELAY_MS = 0

    def tearDown(self) -> None:
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_paste_replaces_selected_text(self) -> None:
        """Paste must replace selected text with clipboard content.

        GIVEN user_input_text contains 'hello world' with 'hello' selected
          AND the clipboard contains 'goodbye'
        WHEN the 'Paste' button is invoked from the context popup
        THEN the input widget contains 'goodbye world'
          AND the popup is dismissed

        [PD-02-AF-012]
        """
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello world")
        widget.tag_add(tk.SEL, "1.0", "1.5")
        _put_on_clipboard(self.root, "goodbye")

        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()

        popup = self.gui._input_panel._input_context_popup
        assert popup is not None
        frame = popup.winfo_children()[0]
        for btn in frame.winfo_children():
            if isinstance(btn, tk.Button) and btn.cget("text") == "Paste":
                btn.invoke()
                break

        result = widget.get("1.0", tk.END).strip()
        assert result == "goodbye world"
        assert self.gui._input_panel._input_context_popup is None

    def test_paste_inserts_at_cursor_when_no_selection(self) -> None:
        """Paste must insert clipboard text at cursor when nothing is selected.

        GIVEN user_input_text contains 'helo world' with the cursor after 'hel'
          AND the clipboard contains 'l'
          AND no text is selected
        WHEN _on_input_context_paste is called directly
        THEN the input widget contains 'hello world'

        [PD-02-AF-012]
        """
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "helo world")
        # Position cursor after 'hel' (index 1.3)
        widget.mark_set(tk.INSERT, "1.3")
        widget.tag_remove(tk.SEL, "1.0", tk.END)
        _put_on_clipboard(self.root, "l")

        self.gui._input_panel._on_input_context_paste(widget)

        result = widget.get("1.0", tk.END).strip()
        assert result == "hello world"

    def test_paste_action_dismisses_popup(self) -> None:
        """Invoking 'Paste' must dismiss the popup.

        GIVEN the input context popup is visible
        WHEN the 'Paste' button is clicked
        THEN _input_context_popup is None

        [PD-02-AF-012]
        """
        widget = self.gui.widgets.user_input_text
        widget.insert("1.0", "hello")
        widget.tag_add(tk.SEL, "1.0", "1.5")
        _put_on_clipboard(self.root, "world")

        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        popup = self.gui._input_panel._input_context_popup
        assert popup is not None

        frame = popup.winfo_children()[0]
        for btn in frame.winfo_children():
            if isinstance(btn, tk.Button) and btn.cget("text") == "Paste":
                btn.invoke()
                break

        assert self.gui._input_panel._input_context_popup is None


# ---------------------------------------------------------------------------
# PD-02-AF-008 — click-away and grab behaviour
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestInputPopupDismissBehavior(unittest.TestCase):
    """Unit tests for input popup auto-dismiss: outside-click and grab (PD-02-AF-008).

    Verifies that the input context popup is dismissed when the user clicks
    outside it, and that it holds the Tk event grab so keyboard events reach it.

    Units under test:
      - ``InputPanel._show_input_context_menu`` (PD-02-AF-008)
      - ``InputPanel._dismiss_input_context_popup`` (PD-02-AF-008)
    """

    def setUp(self) -> None:
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.gui._input_panel._MENU_POST_DELAY_MS = 0
        _put_on_clipboard(self.root, "content")

    def tearDown(self) -> None:
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_outside_click_dismisses_popup(self) -> None:
        """A ButtonPress event outside popup bounds must dismiss it.

        GIVEN the input context popup is visible
        WHEN a ButtonPress is generated at coordinates outside the popup
        THEN the popup is destroyed and _input_context_popup is None

        [PD-02-AF-008]
        """
        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        popup = self.gui._input_panel._input_context_popup
        assert popup is not None

        popup.event_generate("<ButtonPress>", x=-50, y=-50)
        self.root.update()

        assert self.gui._input_panel._input_context_popup is None

    def test_inside_click_does_not_dismiss_popup(self) -> None:
        """A ButtonPress event inside popup bounds must NOT dismiss it.

        GIVEN the input context popup is visible
        WHEN a ButtonPress is generated at coordinates inside the popup
        THEN the popup remains present

        [PD-02-AF-008]
        """
        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        popup = self.gui._input_panel._input_context_popup
        assert popup is not None

        popup.update_idletasks()
        pw = max(popup.winfo_width(), 10)
        ph = max(popup.winfo_height(), 10)
        popup.event_generate("<ButtonPress>", x=pw // 2, y=ph // 2)
        self.root.update()

        assert self.gui._input_panel._input_context_popup is not None

    def test_popup_holds_grab(self) -> None:
        """The popup must hold the Tk event grab after being shown.

        GIVEN the input context popup is visible
        WHEN we query grab_current() on the root
        THEN it returns the popup Toplevel

        [PD-02-AF-008]
        """
        self.gui._input_panel._show_input_context_menu(0, 0)
        self.root.update()
        popup = self.gui._input_panel._input_context_popup
        assert popup is not None
        assert self.root.grab_current() is popup
