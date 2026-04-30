"""Unit tests for WorkingMemory widget callbacks, PD-03-AF-011..014.

Units under test:
  - ``agentx.gui.context_renderer.ContextRenderer.render_working_memory_widget``
  - ``agentx.gui.context_renderer.ContextRenderer._render_working_memory_row``
  - ``agentx.gui.context_renderer.ContextRenderer._confirm_promote``

Affordance IDs: PD-03-AF-011, PD-03-AF-012, PD-03-AF-013, PD-03-AF-014

PD-03-AF-011 — Toggle checkbox calls the on_toggle callback with (compound_key, bool).
PD-03-AF-012 — Delete button calls the on_delete callback when the user confirms.
PD-03-AF-013 — Promote button calls the on_promote callback when the user confirms.
PD-03-AF-014 — Add-fact form calls on_user_add(key, value) when submitted.
"""

from __future__ import annotations

import os
import sys
import tkinter as tk
import unittest
from unittest.mock import MagicMock, patch

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agentx.gui.context_renderer import ContextRenderer
from agentx.gui.gui_config import GUIConfig
from agentx.gui.gui_manager import GUIManager
from shared.models.working_memory import FactEntry, FactOwner, WorkingMemory

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_gui(root: tk.Tk) -> GUIManager:
    """Build a GUIManager (no layout required — testing ContextRenderer directly)."""
    config = GUIConfig.from_dict(
        {
            "ollama_host": "localhost",
            "ollama_model": "test-model",
            "ollama_timeout": 30,
        }
    )
    return GUIManager(
        root=root,
        config=config,
        on_submit=MagicMock(),
        on_interrupt=MagicMock(),
        on_attachment_toggle=MagicMock(),
    )


def _make_fact(
    key: str = "my_key",
    value: str = "my_value",
    owner: FactOwner = FactOwner.AGENT,
    enabled: bool = True,
) -> FactEntry:
    """Create a minimal FactEntry for test use."""
    return FactEntry(owner=owner, key=key, value=value, enabled=enabled)


def _make_wm(*facts: FactEntry) -> WorkingMemory:
    """Create a WorkingMemory pre-populated with the given facts."""
    wm = WorkingMemory()
    for f in facts:
        wm.add_fact(f.owner, f.key, f.value, enabled=f.enabled)
    return wm


def _find_checkbutton_in_row(row_frame: tk.Frame) -> tk.Checkbutton | None:
    """Return the Checkbutton gridded in column 0 of a row_frame (the toggle control)."""
    for w in row_frame.winfo_children():
        info = w.grid_info()
        if info and int(info.get("column", -1)) == 0 and isinstance(w, tk.Checkbutton):
            return w
    return None


def _find_button_by_text(frame: tk.Frame, text: str) -> tk.Button | None:
    """Return the first tk.Button in *frame* whose text matches *text*."""
    for w in frame.winfo_children():
        if isinstance(w, tk.Button) and w.cget("text") == text:
            return w
    return None


def _find_first_row_frame(outer: tk.Frame) -> tk.Frame | None:
    """Return the first tk.Frame child of outer (the first fact row_frame)."""
    for w in outer.winfo_children():
        if isinstance(w, tk.Frame):
            return w
    return None


def _find_add_frame(outer: tk.Frame) -> tk.Frame | None:
    """Return the add_frame (last tk.Frame child of outer)."""
    frames = [w for w in outer.winfo_children() if isinstance(w, tk.Frame)]
    return frames[-1] if frames else None


def _find_entry_by_textvar(frame: tk.Frame, var: tk.StringVar) -> tk.Entry | None:
    """Find an Entry widget bound to a specific StringVar (by name)."""
    var_name = str(var)
    for w in frame.winfo_children():
        if isinstance(w, tk.Entry) and str(w.cget("textvariable")) == var_name:
            return w
    return None


# ---------------------------------------------------------------------------
# PD-03-AF-011 — Toggle checkbox
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestWorkingMemoryToggle(unittest.TestCase):
    """PD-03-AF-011: Toggle checkbox invokes on_toggle(compound_key, bool).

    Units under test:
      - ContextRenderer._render_working_memory_row
      - ContextRenderer.render_working_memory_widget

    Affordance ID: PD-03-AF-011
    """

    def setUp(self) -> None:
        """GIVEN a headless Tk root and a ContextRenderer."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.renderer = self.gui._context_renderer

    def tearDown(self) -> None:
        self.root.destroy()

    def test_toggle_calls_on_toggle_with_false(self) -> None:
        """PD-03-AF-011: unchecking a fact row fires on_toggle(compound_key, False).

        GIVEN a WorkingMemory with one agent fact (enabled=True)
        WHEN the toggle Checkbutton is invoked (unchecking it)
        THEN on_toggle is called with (compound_key, False).
        """
        fact = _make_fact(key="k1", enabled=True)
        wm = _make_wm(fact)
        on_toggle = MagicMock()

        outer = self.renderer.render_working_memory_widget(wm, self.root, on_toggle=on_toggle)

        row_frame = _find_first_row_frame(outer)
        self.assertIsNotNone(row_frame, "No row_frame found inside outer")

        cb = _find_checkbutton_in_row(row_frame)
        self.assertIsNotNone(cb, "No Checkbutton found in row_frame column 0")

        cb.invoke()

        on_toggle.assert_called_once_with(fact.compound_key, False)

    def test_toggle_calls_on_toggle_with_true(self) -> None:
        """PD-03-AF-011: checking a fact row fires on_toggle(compound_key, True).

        GIVEN a WorkingMemory with one agent fact (enabled=False)
        WHEN the toggle Checkbutton is invoked (checking it)
        THEN on_toggle is called with (compound_key, True).
        """
        fact = _make_fact(key="k2", enabled=False)
        wm = _make_wm(fact)
        on_toggle = MagicMock()

        outer = self.renderer.render_working_memory_widget(wm, self.root, on_toggle=on_toggle)

        row_frame = _find_first_row_frame(outer)
        cb = _find_checkbutton_in_row(row_frame)
        self.assertIsNotNone(cb)

        cb.invoke()

        on_toggle.assert_called_once_with(fact.compound_key, True)

    def test_toggle_initial_checked_state_matches_fact_enabled(self) -> None:
        """PD-03-AF-011: Checkbutton initial state reflects fact.enabled.

        GIVEN a WorkingMemory with one agent fact (enabled=True)
        WHEN the widget is rendered
        THEN the Checkbutton variable is True.
        """
        fact = _make_fact(key="k3", enabled=True)
        wm = _make_wm(fact)

        outer = self.renderer.render_working_memory_widget(wm, self.root)

        row_frame = _find_first_row_frame(outer)
        cb = _find_checkbutton_in_row(row_frame)
        self.assertIsNotNone(cb)

        var_value = self.root.getvar(str(cb.cget("variable")))
        self.assertTrue(self.root.tk.getboolean(var_value))

    def test_toggle_initial_unchecked_state_matches_fact_disabled(self) -> None:
        """PD-03-AF-011: Checkbutton initial state reflects fact.enabled=False.

        GIVEN a WorkingMemory with one agent fact (enabled=False)
        WHEN the widget is rendered
        THEN the Checkbutton variable is False.
        """
        fact = _make_fact(key="k4", enabled=False)
        wm = _make_wm(fact)

        outer = self.renderer.render_working_memory_widget(wm, self.root)

        row_frame = _find_first_row_frame(outer)
        cb = _find_checkbutton_in_row(row_frame)
        self.assertIsNotNone(cb)

        var_value = self.root.getvar(str(cb.cget("variable")))
        self.assertFalse(self.root.tk.getboolean(var_value))

    def test_toggle_no_callback_does_not_raise(self) -> None:
        """PD-03-AF-011: invoking toggle with no on_toggle callback does not raise.

        GIVEN a WorkingMemory with one fact and on_toggle=None
        WHEN the toggle Checkbutton is invoked
        THEN no exception is raised.
        """
        fact = _make_fact(key="k5")
        wm = _make_wm(fact)

        outer = self.renderer.render_working_memory_widget(wm, self.root, on_toggle=None)

        row_frame = _find_first_row_frame(outer)
        cb = _find_checkbutton_in_row(row_frame)
        self.assertIsNotNone(cb)

        # Should not raise
        cb.invoke()


# ---------------------------------------------------------------------------
# PD-03-AF-012 — Delete button
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestWorkingMemoryDelete(unittest.TestCase):
    """PD-03-AF-012: Delete button on agent facts fires on_delete(compound_key).

    Units under test:
      - ContextRenderer._render_working_memory_row

    Affordance ID: PD-03-AF-012
    """

    def setUp(self) -> None:
        """GIVEN a headless Tk root and a ContextRenderer."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.renderer = self.gui._context_renderer

    def tearDown(self) -> None:
        self.root.destroy()

    def test_delete_button_calls_on_delete_when_confirmed(self) -> None:
        """PD-03-AF-012: clicking ✕ and confirming calls on_delete(compound_key).

        GIVEN a WorkingMemory with one AGENT fact
        WHEN the ✕ button is clicked and the messagebox returns True
        THEN on_delete is called with the fact's compound_key.
        """
        fact = _make_fact(key="del_key", owner=FactOwner.AGENT)
        wm = _make_wm(fact)
        on_delete = MagicMock()

        outer = self.renderer.render_working_memory_widget(wm, self.root, on_delete=on_delete)

        row_frame = _find_first_row_frame(outer)
        self.assertIsNotNone(row_frame)

        delete_btn = _find_button_by_text(row_frame, "✕")
        self.assertIsNotNone(delete_btn, "No ✕ button found in agent fact row")

        with patch("agentx.gui.context_renderer.tk_messagebox.askyesno", return_value=True):
            delete_btn.invoke()

        on_delete.assert_called_once_with(fact.compound_key)

    def test_delete_button_not_called_when_cancelled(self) -> None:
        """PD-03-AF-012: clicking ✕ and cancelling does NOT call on_delete.

        GIVEN a WorkingMemory with one AGENT fact
        WHEN the ✕ button is clicked and the messagebox returns False
        THEN on_delete is NOT called.
        """
        fact = _make_fact(key="del_cancel", owner=FactOwner.AGENT)
        wm = _make_wm(fact)
        on_delete = MagicMock()

        outer = self.renderer.render_working_memory_widget(wm, self.root, on_delete=on_delete)

        row_frame = _find_first_row_frame(outer)
        delete_btn = _find_button_by_text(row_frame, "✕")
        self.assertIsNotNone(delete_btn)

        with patch("agentx.gui.context_renderer.tk_messagebox.askyesno", return_value=False):
            delete_btn.invoke()

        on_delete.assert_not_called()

    def test_delete_button_absent_for_user_fact(self) -> None:
        """PD-03-AF-012: USER-owned facts have no ✕ delete button.

        GIVEN a WorkingMemory with one USER fact
        WHEN the widget is rendered
        THEN no ✕ button is present in the row_frame.
        """
        fact = _make_fact(key="user_fact", owner=FactOwner.USER)
        wm = _make_wm(fact)

        outer = self.renderer.render_working_memory_widget(wm, self.root)

        row_frame = _find_first_row_frame(outer)
        delete_btn = _find_button_by_text(row_frame, "✕")
        self.assertIsNone(delete_btn, "USER fact should not have a ✕ delete button")

    def test_delete_no_callback_does_not_raise(self) -> None:
        """PD-03-AF-012: clicking ✕ confirmed with on_delete=None does not raise.

        GIVEN a WorkingMemory with one AGENT fact and on_delete=None
        WHEN the ✕ button is clicked and confirmed
        THEN no exception is raised.
        """
        fact = _make_fact(key="del_no_cb", owner=FactOwner.AGENT)
        wm = _make_wm(fact)

        outer = self.renderer.render_working_memory_widget(wm, self.root, on_delete=None)

        row_frame = _find_first_row_frame(outer)
        delete_btn = _find_button_by_text(row_frame, "✕")
        self.assertIsNotNone(delete_btn)

        with patch("agentx.gui.context_renderer.tk_messagebox.askyesno", return_value=True):
            delete_btn.invoke()  # Should not raise


# ---------------------------------------------------------------------------
# PD-03-AF-013 — Promote button
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestWorkingMemoryPromote(unittest.TestCase):
    """PD-03-AF-013: Agent-fact owner icon is a promote button.

    Units under test:
      - ContextRenderer._render_working_memory_row
      - ContextRenderer._confirm_promote

    Affordance ID: PD-03-AF-013
    """

    def setUp(self) -> None:
        """GIVEN a headless Tk root and a ContextRenderer."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.renderer = self.gui._context_renderer

    def tearDown(self) -> None:
        self.root.destroy()

    def test_promote_calls_on_promote_when_confirmed(self) -> None:
        """PD-03-AF-013: clicking agent icon and confirming calls on_promote(compound_key).

        GIVEN a WorkingMemory with one AGENT fact
        WHEN the owner-icon button is clicked and the promote dialog returns True
        THEN on_promote is called with the fact's compound_key.
        """
        fact = _make_fact(key="promo_key", owner=FactOwner.AGENT)
        wm = _make_wm(fact)
        on_promote = MagicMock()

        outer = self.renderer.render_working_memory_widget(wm, self.root, on_promote=on_promote)

        row_frame = _find_first_row_frame(outer)
        self.assertIsNotNone(row_frame)

        # For agent facts, the owner icon (🤖) is a clickable Button in column 1
        promote_btn = _find_button_by_text(row_frame, fact.owner_icon)
        self.assertIsNotNone(promote_btn, "No owner-icon promote button found for AGENT fact")

        with patch("agentx.gui.context_renderer.tk_messagebox.askyesno", return_value=True):
            promote_btn.invoke()

        on_promote.assert_called_once_with(fact.compound_key)

    def test_promote_not_called_when_cancelled(self) -> None:
        """PD-03-AF-013: clicking agent icon and cancelling does NOT call on_promote.

        GIVEN a WorkingMemory with one AGENT fact
        WHEN the owner-icon button is clicked and the dialog returns False
        THEN on_promote is NOT called.
        """
        fact = _make_fact(key="promo_cancel", owner=FactOwner.AGENT)
        wm = _make_wm(fact)
        on_promote = MagicMock()

        outer = self.renderer.render_working_memory_widget(wm, self.root, on_promote=on_promote)

        row_frame = _find_first_row_frame(outer)
        promote_btn = _find_button_by_text(row_frame, fact.owner_icon)
        self.assertIsNotNone(promote_btn)

        with patch("agentx.gui.context_renderer.tk_messagebox.askyesno", return_value=False):
            promote_btn.invoke()

        on_promote.assert_not_called()

    def test_user_fact_owner_icon_is_label_not_button(self) -> None:
        """PD-03-AF-013: USER-owned fact owner icon is a Label (not a clickable Button).

        GIVEN a WorkingMemory with one USER fact
        WHEN the widget is rendered
        THEN the owner icon is a tk.Label, not a tk.Button.
        """
        fact = _make_fact(key="user_icon", owner=FactOwner.USER)
        wm = _make_wm(fact)

        outer = self.renderer.render_working_memory_widget(wm, self.root)

        row_frame = _find_first_row_frame(outer)

        # Should have a Label with the user icon, NOT a Button
        promote_btn = _find_button_by_text(row_frame, fact.owner_icon)
        self.assertIsNone(promote_btn, "USER fact should have Label, not promote Button")

        icon_label = None
        for w in row_frame.winfo_children():
            if isinstance(w, tk.Label) and w.cget("text") == fact.owner_icon:
                icon_label = w
                break
        self.assertIsNotNone(icon_label, "USER fact should have a Label with owner icon")

    def test_promote_no_callback_does_not_raise(self) -> None:
        """PD-03-AF-013: clicking promote icon confirmed with on_promote=None does not raise.

        GIVEN a WorkingMemory with one AGENT fact and on_promote=None
        WHEN the owner-icon promote button is clicked and confirmed
        THEN no exception is raised.
        """
        fact = _make_fact(key="promo_no_cb", owner=FactOwner.AGENT)
        wm = _make_wm(fact)

        outer = self.renderer.render_working_memory_widget(wm, self.root, on_promote=None)

        row_frame = _find_first_row_frame(outer)
        promote_btn = _find_button_by_text(row_frame, fact.owner_icon)
        self.assertIsNotNone(promote_btn)

        with patch("agentx.gui.context_renderer.tk_messagebox.askyesno", return_value=True):
            promote_btn.invoke()  # Should not raise


# ---------------------------------------------------------------------------
# PD-03-AF-014 — Add-fact form
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestWorkingMemoryAddFact(unittest.TestCase):
    """PD-03-AF-014: Add-fact form submits user-provided key/value via on_user_add.

    Units under test:
      - ContextRenderer.render_working_memory_widget (add_frame section)

    Affordance ID: PD-03-AF-014
    """

    def setUp(self) -> None:
        """GIVEN a headless Tk root and a ContextRenderer."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.renderer = self.gui._context_renderer

    def tearDown(self) -> None:
        self.root.destroy()

    def _render_and_get_add_widgets(self, on_user_add: MagicMock) -> tuple[tk.Frame, tk.Button, list[tk.Entry]]:
        """Render the WM widget and return (add_frame, add_button, [entry1, entry2])."""
        wm = WorkingMemory()  # empty — add form always present
        outer = self.renderer.render_working_memory_widget(wm, self.root, on_user_add=on_user_add)

        add_frame = _find_add_frame(outer)
        self.assertIsNotNone(add_frame, "No add_frame found in outer")

        add_btn = _find_button_by_text(add_frame, "Add 👤")
        self.assertIsNotNone(add_btn, "No 'Add 👤' button found in add_frame")

        entries = [w for w in add_frame.winfo_children() if isinstance(w, tk.Entry)]
        return add_frame, add_btn, entries

    def test_add_button_calls_on_user_add_with_key_and_value(self) -> None:
        """PD-03-AF-014: clicking 'Add 👤' with key and value fires on_user_add(key, value).

        GIVEN a rendered WM widget with on_user_add callback
        WHEN the key Entry contains "my_key", value Entry contains "my_val", and 'Add 👤' is clicked
        THEN on_user_add is called with ("my_key", "my_val").
        """
        on_user_add = MagicMock()
        add_frame, add_btn, entries = self._render_and_get_add_widgets(on_user_add)

        self.assertGreaterEqual(len(entries), 2, "Expected at least 2 Entry widgets in add_frame")
        entries[0].insert(0, "my_key")
        entries[1].insert(0, "my_val")

        add_btn.invoke()

        on_user_add.assert_called_once_with("my_key", "my_val")

    def test_add_button_clears_entries_after_submit(self) -> None:
        """PD-03-AF-014: after successful add, key and value Entry fields are cleared.

        GIVEN a rendered WM widget with on_user_add callback
        WHEN 'Add 👤' is clicked with non-empty key/value
        THEN both Entry fields are empty after the call.
        """
        on_user_add = MagicMock()
        add_frame, add_btn, entries = self._render_and_get_add_widgets(on_user_add)

        entries[0].insert(0, "clear_key")
        entries[1].insert(0, "clear_val")
        add_btn.invoke()

        self.assertEqual(entries[0].get(), "", "Key entry should be cleared after submit")
        self.assertEqual(entries[1].get(), "", "Value entry should be cleared after submit")

    def test_add_button_does_not_call_on_user_add_when_key_empty(self) -> None:
        """PD-03-AF-014: clicking 'Add 👤' with an empty key does NOT fire on_user_add.

        GIVEN a rendered WM widget with on_user_add callback
        WHEN 'Add 👤' is clicked with an empty key Entry (and any value)
        THEN on_user_add is NOT called.
        """
        on_user_add = MagicMock()
        add_frame, add_btn, entries = self._render_and_get_add_widgets(on_user_add)

        entries[0].delete(0, tk.END)  # empty key
        entries[1].insert(0, "some_val")

        add_btn.invoke()

        on_user_add.assert_not_called()

    def test_add_button_no_callback_does_not_raise(self) -> None:
        """PD-03-AF-014: clicking 'Add 👤' with on_user_add=None does not raise.

        GIVEN a rendered WM widget with on_user_add=None
        WHEN 'Add 👤' is clicked with a valid key/value
        THEN no exception is raised.
        """
        wm = WorkingMemory()
        outer = self.renderer.render_working_memory_widget(wm, self.root, on_user_add=None)

        add_frame = _find_add_frame(outer)
        add_btn = _find_button_by_text(add_frame, "Add 👤")
        entries = [w for w in add_frame.winfo_children() if isinstance(w, tk.Entry)]

        entries[0].insert(0, "key_no_cb")
        entries[1].insert(0, "val_no_cb")

        add_btn.invoke()  # Should not raise
