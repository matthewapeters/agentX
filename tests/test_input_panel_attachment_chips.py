"""Unit tests for InputPanel attachment chip affordances PD-02-AF-005..007.

Units under test:
  - ``agentx.gui.input_panel.InputPanel.update_attachment_bar``  (AF-005, AF-007)
  - ``agentx.gui.input_panel.InputPanel._create_attachment_widget``  (AF-005, AF-006)
  - ``agentx.attachment_info.AttachmentInfo``                        (AF-005, AF-006)

Affordance IDs: PD-02-AF-005, PD-02-AF-006, PD-02-AF-007
"""

from __future__ import annotations

import os
import sys
import tkinter as tk
import unittest
from unittest.mock import MagicMock

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agentx.attachment_info import AttachmentInfo
from agentx.gui.gui_config import GUIConfig
from agentx.gui.gui_manager import GUIManager

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_info(
    display_name: str = "test.py",
    attachment_id: str = "att-001",
    enabled: bool = True,
    is_from_history: bool = False,
) -> AttachmentInfo:
    """Build a minimal AttachmentInfo without a real Attachment object."""
    return AttachmentInfo(
        file_path=f"/tmp/{display_name}",
        display_name=display_name,
        enabled=enabled,
        is_from_history=is_from_history,
        attachment_id=attachment_id,
    )


def _make_gui(root: tk.Tk, on_toggle: MagicMock | None = None) -> GUIManager:
    """Build a fully laid-out GUIManager for headless testing."""
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
        on_attachment_toggle=on_toggle if on_toggle is not None else MagicMock(),
    )


def _find_checkbutton(widget: tk.Widget) -> tk.Checkbutton | None:
    """Recursively locate the first ``tk.Checkbutton`` inside *widget*."""
    if isinstance(widget, tk.Checkbutton):
        return widget
    for child in widget.winfo_children():
        found = _find_checkbutton(child)
        if found is not None:
            return found
    return None


# ---------------------------------------------------------------------------
# PD-02-AF-005 — Chip renders with filename
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestAttachmentChipRender(unittest.TestCase):
    """PD-02-AF-005: Attachment chip rendered with correct filename and icon.

    Unit under test: ``InputPanel._create_attachment_widget`` via
    ``InputPanel.update_attachment_bar``.
    """

    def setUp(self) -> None:
        """Set up a headless GUIManager with a real Tkinter root."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.on_toggle: MagicMock = MagicMock()
        self.gui = _make_gui(self.root, self.on_toggle)
        self.gui.create_layout()

    def tearDown(self) -> None:
        """Destroy Tkinter root after each test."""
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_current_attachment_chip_shows_filename(self) -> None:
        """Current-turn chip Checkbutton text contains the display_name.

        PD-02-AF-005

        GIVEN an AttachmentInfo with display_name="parser.py" and
              is_from_history=False
        WHEN  update_attachment_bar([info], []) is called
        THEN  attachment_labels has 1 entry
          AND the Checkbutton text contains "parser.py"
        """
        info = _make_info("parser.py", is_from_history=False)
        self.gui._input_panel.update_attachment_bar([info], [])

        chips = self.gui.widgets.attachment_labels
        self.assertEqual(len(chips), 1)
        cb = _find_checkbutton(chips[0])
        self.assertIsNotNone(cb, "Expected a Checkbutton inside the chip frame")
        self.assertIn("parser.py", cb.cget("text"))

    def test_current_attachment_chip_uses_folder_icon(self) -> None:
        """Current-turn chip Checkbutton text starts with the 📁 folder icon.

        PD-02-AF-005

        GIVEN an AttachmentInfo with display_name="main.py" and
              is_from_history=False
        WHEN  update_attachment_bar([info], []) is called
        THEN  the Checkbutton text begins with "📁"
        """
        info = _make_info("main.py", is_from_history=False)
        self.gui._input_panel.update_attachment_bar([info], [])

        cb = _find_checkbutton(self.gui.widgets.attachment_labels[0])
        self.assertTrue(cb.cget("text").startswith("📁"), f"Expected '📁' prefix, got: {cb.cget('text')!r}")

    def test_history_attachment_chip_shows_filename_and_history_suffix(self) -> None:
        """History chip Checkbutton text contains filename and "(history)".

        PD-02-AF-005

        GIVEN an AttachmentInfo with display_name="old_file.txt" and
              is_from_history=True
        WHEN  update_attachment_bar([], [info]) is called
        THEN  attachment_labels has 1 entry
          AND the Checkbutton text contains "old_file.txt"
          AND the Checkbutton text contains "(history)"
        """
        info = _make_info("old_file.txt", is_from_history=True)
        self.gui._input_panel.update_attachment_bar([], [info])

        chips = self.gui.widgets.attachment_labels
        self.assertEqual(len(chips), 1)
        cb = _find_checkbutton(chips[0])
        self.assertIsNotNone(cb)
        text = cb.cget("text")
        self.assertIn("old_file.txt", text)
        self.assertIn("history", text)

    def test_history_attachment_chip_uses_scroll_icon(self) -> None:
        """History chip Checkbutton text starts with the 📜 scroll icon.

        PD-02-AF-005

        GIVEN an AttachmentInfo with is_from_history=True
        WHEN  update_attachment_bar([], [info]) is called
        THEN  the Checkbutton text begins with "📜"
        """
        info = _make_info("ctx.txt", is_from_history=True)
        self.gui._input_panel.update_attachment_bar([], [info])

        cb = _find_checkbutton(self.gui.widgets.attachment_labels[0])
        self.assertTrue(cb.cget("text").startswith("📜"), f"Expected '📜' prefix, got: {cb.cget('text')!r}")

    def test_multiple_chips_rendered_in_order(self) -> None:
        """Multiple attachments produce one chip frame per attachment.

        PD-02-AF-005

        GIVEN two AttachmentInfos with display_names "a.py" and "b.py"
        WHEN  update_attachment_bar([info_a, info_b], []) is called
        THEN  attachment_labels has exactly 2 entries
        """
        info_a = _make_info("a.py", attachment_id="a")
        info_b = _make_info("b.py", attachment_id="b")
        self.gui._input_panel.update_attachment_bar([info_a, info_b], [])

        self.assertEqual(len(self.gui.widgets.attachment_labels), 2)


# ---------------------------------------------------------------------------
# PD-02-AF-006 — Toggle chip calls on_attachment_toggle callback
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestAttachmentChipToggle(unittest.TestCase):
    """PD-02-AF-006: Checking/unchecking a chip calls the toggle callback.

    Unit under test: the ``on_toggle`` closure inside
    ``InputPanel._create_attachment_widget``.
    """

    def setUp(self) -> None:
        """Set up a headless GUIManager with a captured on_toggle mock."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.on_toggle: MagicMock = MagicMock()
        self.gui = _make_gui(self.root, self.on_toggle)
        self.gui.create_layout()

    def tearDown(self) -> None:
        """Destroy Tkinter root after each test."""
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_uncheck_calls_on_attachment_toggle_false(self) -> None:
        """Unchecking an enabled chip calls on_attachment_toggle with False.

        PD-02-AF-006

        GIVEN a chip rendered with enabled=True and attachment_id="att-x"
        WHEN  the Checkbutton is invoked (checked → unchecked)
        THEN  on_attachment_toggle("att-x", False) is called exactly once
        """
        info = _make_info("code.py", attachment_id="att-x", enabled=True)
        self.gui._input_panel.update_attachment_bar([info], [])

        cb = _find_checkbutton(self.gui.widgets.attachment_labels[0])
        self.assertIsNotNone(cb)
        cb.invoke()  # True → False

        self.on_toggle.assert_called_once_with("att-x", False)

    def test_check_after_uncheck_calls_toggle_true(self) -> None:
        """Checking a disabled chip calls on_attachment_toggle with True.

        PD-02-AF-006

        GIVEN a chip rendered with enabled=False and attachment_id="att-y"
        WHEN  the Checkbutton is invoked (unchecked → checked)
        THEN  on_attachment_toggle("att-y", True) is called exactly once
        """
        info = _make_info("code.py", attachment_id="att-y", enabled=False)
        self.gui._input_panel.update_attachment_bar([info], [])

        cb = _find_checkbutton(self.gui.widgets.attachment_labels[0])
        self.assertIsNotNone(cb)
        cb.invoke()  # False → True

        self.on_toggle.assert_called_once_with("att-y", True)


# ---------------------------------------------------------------------------
# PD-02-AF-007 — Rebuild clears old chips
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestAttachmentBarClear(unittest.TestCase):
    """PD-02-AF-007: update_attachment_bar([], []) destroys all existing chips.

    Unit under test: ``InputPanel.update_attachment_bar`` delegating to
    ``WidgetRegistry.clear_attachments``.
    """

    def setUp(self) -> None:
        """Set up a headless GUIManager."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)
        self.gui.create_layout()

    def tearDown(self) -> None:
        """Destroy Tkinter root after each test."""
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_empty_update_clears_all_chips(self) -> None:
        """Calling update_attachment_bar with empty lists removes all chips.

        PD-02-AF-007

        GIVEN one chip already rendered
        WHEN  update_attachment_bar([], []) is called
        THEN  attachment_labels is empty
        """
        info = _make_info("file.py", attachment_id="c1")
        self.gui._input_panel.update_attachment_bar([info], [])
        self.assertEqual(len(self.gui.widgets.attachment_labels), 1)

        self.gui._input_panel.update_attachment_bar([], [])
        self.assertEqual(len(self.gui.widgets.attachment_labels), 0)

    def test_rebuild_replaces_existing_chips(self) -> None:
        """Rebuilding the bar destroys old chips and shows only the new ones.

        PD-02-AF-007

        GIVEN a chip for "old.py" already rendered
        WHEN  update_attachment_bar([new_info("new.py")], []) is called
        THEN  attachment_labels has exactly 1 entry
          AND the Checkbutton text contains "new.py"
        """
        info_old = _make_info("old.py", attachment_id="old")
        self.gui._input_panel.update_attachment_bar([info_old], [])

        info_new = _make_info("new.py", attachment_id="new")
        self.gui._input_panel.update_attachment_bar([info_new], [])

        chips = self.gui.widgets.attachment_labels
        self.assertEqual(len(chips), 1)
        cb = _find_checkbutton(chips[0])
        self.assertIsNotNone(cb)
        self.assertIn("new.py", cb.cget("text"))
        self.assertNotIn("old.py", cb.cget("text"))
