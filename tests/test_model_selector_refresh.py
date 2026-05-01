"""Unit tests for ModelSelector refresh button (PD-04-AF-004).

Tests:
- ModelSelector.__init__ creates a refresh button widget.
- Clicking refresh invokes the registered on_refresh callback.
- Clicking refresh with no callback registered does not raise.
- set_refresh_callback() replaces the callback.
- on_refresh parameter wired at construction time is called.
"""

from __future__ import annotations

import os
import sys
import tkinter as tk
import unittest
from tkinter import ttk
from unittest.mock import MagicMock

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agentx.gui.model_selector import ModelSelector

# ── Helpers ────────────────────────────────────────────────────────────────────


def _make_root() -> tk.Tk:
    root = tk.Tk()
    root.withdraw()
    return root


def _make_selector(root: tk.Tk, on_model_change=None, on_refresh=None) -> ModelSelector:
    """Construct a ModelSelector with sensible defaults."""
    return ModelSelector(
        parent=root,
        on_model_change=on_model_change or MagicMock(),
        initial_model="",
        on_refresh=on_refresh,
    )


# ── Tests ──────────────────────────────────────────────────────────────────────


@pytest.mark.unit
class TestModelSelectorRefreshButton(unittest.TestCase):
    """
    Unit tests for PD-04-AF-004: ModelSelector refresh button.

    Unit under test: ``ModelSelector`` (``src/agentx/gui/model_selector.py``)
    """

    def setUp(self) -> None:
        self.root = _make_root()

    def tearDown(self) -> None:
        self.root.destroy()

    # ------------------------------------------------------------------
    # Button existence
    # ------------------------------------------------------------------

    def test_refresh_button_is_created(self) -> None:
        """GIVEN a ModelSelector is constructed
        WHEN the widget is inspected
        THEN a refresh_btn attribute exists as a ttk.Button.
        """
        selector = _make_selector(self.root)
        self.assertIsInstance(selector.refresh_btn, ttk.Button)

    def test_refresh_button_is_packed_inside_frame(self) -> None:
        """GIVEN a ModelSelector is constructed
        WHEN the frame children are listed
        THEN the refresh button is present among them.
        """
        selector = _make_selector(self.root)
        children = selector.frame.winfo_children()
        self.assertIn(selector.refresh_btn, children)

    def test_refresh_button_label_is_refresh_glyph(self) -> None:
        """GIVEN a ModelSelector is constructed
        WHEN the refresh button text is read
        THEN it is the '⟳' glyph.
        """
        selector = _make_selector(self.root)
        self.assertEqual(selector.refresh_btn.cget("text"), "⟳")

    # ------------------------------------------------------------------
    # Callback invocation
    # ------------------------------------------------------------------

    def test_on_refresh_callback_called_when_button_clicked(self) -> None:
        """GIVEN a ModelSelector constructed with an on_refresh callback
        WHEN _on_refresh() is called (simulating a button click)
        THEN the callback is invoked exactly once.
        """
        cb = MagicMock()
        selector = _make_selector(self.root, on_refresh=cb)
        selector._on_refresh()
        cb.assert_called_once()

    def test_no_callback_does_not_raise(self) -> None:
        """GIVEN a ModelSelector constructed without an on_refresh callback
        WHEN _on_refresh() is called
        THEN no exception is raised.
        """
        selector = _make_selector(self.root, on_refresh=None)
        # Must not raise
        selector._on_refresh()

    # ------------------------------------------------------------------
    # set_refresh_callback
    # ------------------------------------------------------------------

    def test_set_refresh_callback_replaces_previous(self) -> None:
        """GIVEN a ModelSelector with an initial on_refresh callback
        WHEN set_refresh_callback() is called with a new callback
        THEN only the new callback is invoked on the next _on_refresh().
        """
        old_cb = MagicMock()
        new_cb = MagicMock()
        selector = _make_selector(self.root, on_refresh=old_cb)
        selector.set_refresh_callback(new_cb)
        selector._on_refresh()
        old_cb.assert_not_called()
        new_cb.assert_called_once()

    def test_set_refresh_callback_enables_refresh_after_none(self) -> None:
        """GIVEN a ModelSelector constructed without a refresh callback
        WHEN set_refresh_callback() is called
        THEN the new callback is invoked on the next _on_refresh().
        """
        cb = MagicMock()
        selector = _make_selector(self.root, on_refresh=None)
        selector.set_refresh_callback(cb)
        selector._on_refresh()
        cb.assert_called_once()

    def test_set_refresh_callback_to_none_then_no_raise(self) -> None:
        """GIVEN a ModelSelector with a registered callback
        WHEN set_refresh_callback(None) is called and _on_refresh() fires
        THEN no exception is raised.
        """
        cb = MagicMock()
        selector = _make_selector(self.root, on_refresh=cb)
        selector.set_refresh_callback(None)  # type: ignore[arg-type]
        selector._on_refresh()  # must not raise


if __name__ == "__main__":
    unittest.main()
