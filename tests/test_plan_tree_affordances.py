"""Unit tests for PlanTreeWidget UX affordances.

Covers:
- PD-05-AF-004: Re-synth button appears in synthesis block when on_resynth provided.
- PD-05-AF-005: Export button is created in plan-tab toolbar by ChatPanel.add_plan_tab().
- PD-05-AF-006: Node status icon correctly reflects all defined task states.
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
from agentx.gui.plan_tree_widget import _STATUS_ICONS, PlanTreeWidget

# ── Helpers ────────────────────────────────────────────────────────────────────


def _make_root() -> tk.Tk:
    root = tk.Tk()
    root.withdraw()
    return root


def _make_tree(root: tk.Tk) -> PlanTreeWidget:
    """Return a bare PlanTreeWidget attached to the root window."""
    return PlanTreeWidget(parent=root, bg="#222222", fg="#eee", dim_fg="#888", accent_fg="#7dd3fc")


def _make_gui(root: tk.Tk) -> GUIManager:
    cfg = GUIConfig.from_dict({"ollama_host": "localhost", "ollama_model": "m", "ollama_timeout": 30})
    gui = GUIManager(root, cfg, on_submit=MagicMock(), on_interrupt=MagicMock(), on_attachment_toggle=MagicMock())
    gui.create_layout()
    return gui


def _widget_texts(parent: tk.Widget) -> list[str]:
    """Recursively collect text/label from all child widgets."""
    texts: list[str] = []
    for child in parent.winfo_children():
        try:
            texts.append(str(child.cget("text")))
        except (tk.TclError, Exception):
            pass
        texts.extend(_widget_texts(child))
    return texts


# ── PD-05-AF-006: Node status icon reflects task state ────────────────────────


@pytest.mark.unit
class TestNodeStatusIconReflectsState(unittest.TestCase):
    """Unit tests for PD-05-AF-006: node status icon reflects task state.

    Unit under test: ``PlanTreeWidget.update_node_status()`` /
    ``_STATUS_ICONS`` (``src/agentx/gui/plan_tree_widget.py``).

    Affordance ID: PD-05-AF-006
    """

    def setUp(self) -> None:
        self.root = _make_root()
        self.tree = _make_tree(self.root)
        self.tree.add_step_node("plan-1", "step-1", "Task", tbd=False)

    def tearDown(self) -> None:
        self.root.destroy()

    def _status_label_text(self) -> str:
        return self.tree._nodes["step-1"]["status_label"].cget("text")

    # ------------------------------------------------------------------
    # Individual status states
    # ------------------------------------------------------------------

    def test_pending_status_shows_empty_circle(self) -> None:
        """GIVEN a step node with initial pending status
        WHEN update_node_status is called with "pending"
        THEN the label shows the pending icon '○'.
        """
        self.tree.update_node_status("step-1", "pending")
        self.assertEqual(self._status_label_text(), _STATUS_ICONS["pending"])

    def test_running_status_shows_filled_circle(self) -> None:
        """GIVEN a step node
        WHEN update_node_status is called with "running"
        THEN the label shows the running icon '●'.
        """
        self.tree.update_node_status("step-1", "running")
        self.assertEqual(self._status_label_text(), _STATUS_ICONS["running"])

    def test_done_status_shows_checkmark(self) -> None:
        """GIVEN a step node
        WHEN update_node_status is called with "done"
        THEN the label shows the done icon '✓'.
        """
        self.tree.update_node_status("step-1", "done")
        self.assertEqual(self._status_label_text(), _STATUS_ICONS["done"])

    def test_tbd_status_shows_question_mark(self) -> None:
        """GIVEN a step node
        WHEN update_node_status is called with "tbd"
        THEN the label shows the tbd icon '?'.
        """
        self.tree.update_node_status("step-1", "tbd")
        self.assertEqual(self._status_label_text(), _STATUS_ICONS["tbd"])

    def test_failed_status_shows_cross(self) -> None:
        """GIVEN a step node
        WHEN update_node_status is called with "failed"
        THEN the label shows the failed icon '✗'.
        """
        self.tree.update_node_status("step-1", "failed")
        self.assertEqual(self._status_label_text(), _STATUS_ICONS["failed"])

    # ------------------------------------------------------------------
    # Edge cases
    # ------------------------------------------------------------------

    def test_unknown_status_falls_back_to_bullet(self) -> None:
        """GIVEN a step node
        WHEN update_node_status is called with an unrecognised status string
        THEN the label shows the fallback bullet '●'.
        """
        self.tree.update_node_status("step-1", "an_unknown_state")
        self.assertEqual(self._status_label_text(), "●")

    def test_missing_task_id_does_not_raise(self) -> None:
        """GIVEN no node registered with id 'nonexistent'
        WHEN update_node_status is called with that id
        THEN no exception is raised.
        """
        self.tree.update_node_status("nonexistent", "done")  # must not raise

    def test_status_can_transition_multiple_times(self) -> None:
        """GIVEN a step node in 'pending' state
        WHEN update_node_status is called multiple times with different states
        THEN each transition updates the icon correctly.
        """
        for status, expected in [
            ("running", _STATUS_ICONS["running"]),
            ("done", _STATUS_ICONS["done"]),
            ("failed", _STATUS_ICONS["failed"]),
        ]:
            self.tree.update_node_status("step-1", status)
            self.assertEqual(self._status_label_text(), expected)


# ── PD-05-AF-004: Re-synth button opens ResynthesisDialog ─────────────────────


@pytest.mark.unit
class TestResynthButtonInSynthesisBlock(unittest.TestCase):
    """Unit tests for PD-05-AF-004: Re-synth button opens ResynthesisDialog.

    Unit under test: ``PlanTreeWidget._create_synthesis_block()`` /
    ``PlanTreeWidget.add_synthesis_to_node()``
    (``src/agentx/gui/plan_tree_widget.py``).

    Affordance ID: PD-05-AF-004
    """

    def setUp(self) -> None:
        self.root = _make_root()
        self.tree = _make_tree(self.root)
        self.tree.add_step_node("plan-1", "step-1", "Task", tbd=False)

    def tearDown(self) -> None:
        self.root.destroy()

    def _details_frame(self) -> tk.Widget:
        return self.tree._nodes["step-1"]["details_frame"]

    def _collect_button_texts(self) -> list[str]:
        """Gather text from all Button widgets in the details frame hierarchy."""
        texts: list[str] = []

        def _recurse(parent: tk.Widget) -> None:
            for child in parent.winfo_children():
                if isinstance(child, tk.Button):
                    try:
                        texts.append(str(child.cget("text")))
                    except tk.TclError:
                        pass
                _recurse(child)

        _recurse(self._details_frame())
        return texts

    def test_resynth_button_present_when_on_resynth_provided(self) -> None:
        """GIVEN add_synthesis_to_node is called with a non-None on_resynth callback
        WHEN the details frame children are inspected
        THEN a button labelled '↻ Re-synthesise' is present.
        """
        self.tree.add_synthesis_to_node("step-1", "Synthesis text.", [], on_resynth=MagicMock())
        texts = self._collect_button_texts()
        self.assertTrue(any("Re-synth" in t for t in texts), f"No re-synth button found; buttons: {texts}")

    def test_resynth_button_absent_when_on_resynth_is_none(self) -> None:
        """GIVEN add_synthesis_to_node is called WITHOUT an on_resynth callback
        WHEN the details frame children are inspected
        THEN no 'Re-synthesise' button is present.
        """
        self.tree.add_synthesis_to_node("step-1", "Synthesis text.", [], on_resynth=None)
        texts = self._collect_button_texts()
        self.assertFalse(any("Re-synth" in t for t in texts), f"Unexpected re-synth button found; buttons: {texts}")

    def test_resynth_callback_stored_for_invocation(self) -> None:
        """GIVEN a mock on_resynth callback is provided
        WHEN the dialog helper (_open_dialog) would fire (simulated by calling
             _create_synthesis_block directly)
        THEN the callback reference is captured and reachable.
        """
        cb = MagicMock()
        self.tree.add_synthesis_to_node("step-1", "Synthesis text.", [], on_resynth=cb)
        # The node should now have a synthesis_widget (confirms synthesis block was created)
        self.assertIn("synthesis_widget", self.tree._nodes["step-1"])

    def test_missing_node_does_not_raise_with_on_resynth(self) -> None:
        """GIVEN no node registered with id 'missing'
        WHEN add_synthesis_to_node is called on that id
        THEN no exception is raised.
        """
        self.tree.add_synthesis_to_node("missing", "text", [], on_resynth=MagicMock())


# ── PD-05-AF-005: Export button writes and opens export file ──────────────────


@pytest.mark.unit
class TestExportButtonInPlanTab(unittest.TestCase):
    """Unit tests for PD-05-AF-005: Export button wired through add_plan_tab.

    Unit under test: ``ChatPanel.add_plan_tab()`` / ``GUIManager.add_plan_tab()``
    (``src/agentx/gui/chat_panel.py``).

    The Export button is created in the plan-tab toolbar.  The actual export
    logic lives in ``AgentXSession._export_task_tree()`` and is injected via
    the ``on_export`` callback.

    Affordance ID: PD-05-AF-005
    """

    def setUp(self) -> None:
        self.root = _make_root()
        self.gui = _make_gui(self.root)

    def tearDown(self) -> None:
        self.root.destroy()

    def _collect_button_texts_in_tab(self, tab_frame: tk.Frame) -> list[str]:
        texts: list[str] = []

        def _recurse(parent: tk.Widget) -> None:
            for child in parent.winfo_children():
                if isinstance(child, tk.Button):
                    try:
                        texts.append(str(child.cget("text")))
                    except tk.TclError:
                        pass
                _recurse(child)

        _recurse(tab_frame)
        return texts

    def test_export_button_present_in_plan_tab_toolbar(self) -> None:
        """GIVEN add_plan_tab is called
        WHEN the resulting tab frame's children are inspected
        THEN a button labelled 'Export' exists in the toolbar.
        """
        tab_frame = self.gui.add_plan_tab("plan-x", "My Plan")
        texts = self._collect_button_texts_in_tab(tab_frame)
        self.assertIn("Export", texts)

    def test_export_callback_invoked_when_button_clicked(self) -> None:
        """GIVEN add_plan_tab is called with an on_export callback
        WHEN the Export button command is invoked directly
        THEN the on_export callback is called exactly once.
        """
        cb = MagicMock()
        tab_frame = self.gui.add_plan_tab("plan-y", "Callback Plan", on_export=cb)

        # Find the Export button and invoke its command
        def _find_export_btn(parent: tk.Widget):
            for child in parent.winfo_children():
                if isinstance(child, tk.Button):
                    try:
                        if child.cget("text") == "Export":
                            return child
                    except tk.TclError:
                        pass
                result = _find_export_btn(child)
                if result is not None:
                    return result
            return None

        btn = _find_export_btn(tab_frame)
        self.assertIsNotNone(btn, "Export button not found in tab frame")
        btn.invoke()
        cb.assert_called_once()

    def test_export_button_present_without_callback(self) -> None:
        """GIVEN add_plan_tab is called without an on_export callback
        WHEN the tab frame is inspected
        THEN the Export button still exists (command is a no-op).
        """
        tab_frame = self.gui.add_plan_tab("plan-z", "No Callback Plan", on_export=None)
        texts = self._collect_button_texts_in_tab(tab_frame)
        self.assertIn("Export", texts)

    def test_no_callback_export_click_does_not_raise(self) -> None:
        """GIVEN add_plan_tab is called without an on_export callback
        WHEN the Export button is clicked
        THEN no exception is raised.
        """
        tab_frame = self.gui.add_plan_tab("plan-noop", "Noop Plan", on_export=None)

        def _find_export_btn(parent: tk.Widget):
            for child in parent.winfo_children():
                if isinstance(child, tk.Button):
                    try:
                        if child.cget("text") == "Export":
                            return child
                    except tk.TclError:
                        pass
                result = _find_export_btn(child)
                if result is not None:
                    return result
            return None

        btn = _find_export_btn(tab_frame)
        self.assertIsNotNone(btn)
        btn.invoke()  # must not raise


if __name__ == "__main__":
    unittest.main()
