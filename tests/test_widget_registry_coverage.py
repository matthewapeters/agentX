"""Coverage uplift tests for agentx/widget_registry.py.

All tests that create real Tkinter widgets must instantiate a tk.Tk root
(or use MagicMock for pure attribute tests) and destroy it properly.
"""

import tkinter as tk
import unittest
from unittest.mock import MagicMock

from agentx.widget_registry import WidgetRegistry


def _make_root() -> tk.Tk:
    root = tk.Tk()
    root.withdraw()  # Don't display the window
    return root


class TestWidgetRegistryInit(unittest.TestCase):
    def test_all_attrs_are_none_by_default(self):
        reg = WidgetRegistry()
        self.assertIsNone(reg.root)
        self.assertIsNone(reg.paned)
        self.assertIsNone(reg.output_display)
        self.assertIsNone(reg.user_input)
        self.assertIsNone(reg.user_submit)
        self.assertEqual(reg.attachment_labels, [])
        self.assertEqual(reg.plan_tabs, {})


class TestWidgetRegistryClearAttachments(unittest.TestCase):
    def test_clear_attachments_with_no_widgets(self):
        reg = WidgetRegistry()
        reg.clear_attachments()  # Should not raise with empty list
        self.assertEqual(reg.attachment_labels, [])

    def test_clear_attachments_destroys_widgets(self):
        root = _make_root()
        try:
            reg = WidgetRegistry()
            lbl1 = tk.Label(root, text="a")
            lbl2 = tk.Label(root, text="b")
            reg.attachment_labels = [lbl1, lbl2]
            reg.clear_attachments()
            self.assertEqual(reg.attachment_labels, [])
        finally:
            root.destroy()


class TestWidgetRegistryDestroyAll(unittest.TestCase):
    def test_destroy_all_with_all_none(self):
        """destroy_all should not raise when all attributes are None."""
        reg = WidgetRegistry()
        reg.destroy_all()  # Should not raise

    def test_destroy_all_destroys_real_widgets(self):
        root = _make_root()
        try:
            inner = tk.Tk()
            inner.withdraw()

            reg = WidgetRegistry()
            reg.root = inner
            reg.paned = tk.PanedWindow(inner)
            reg.output_display = tk.Frame(inner)
            reg.output_notebook = tk.ttk.Notebook(inner)
            reg.output_tab = tk.Frame(inner)
            reg.user_input = tk.Frame(inner)
            reg.user_input_text = tk.Text(inner)
            reg.user_submit = tk.Button(inner, text="Submit")
            reg.user_break = tk.Button(inner, text="Break")

            # Attach an attachment label
            lbl = tk.Label(inner, text="attach")
            reg.attachment_labels = [lbl]

            # Add a plan tab
            plan_frame = tk.Frame(inner)
            reg.plan_tabs["plan_0"] = plan_frame

            reg.destroy_all()

            # After destroy, root is destroyed too — plan_tabs should be cleared
            self.assertEqual(reg.plan_tabs, {})
            self.assertEqual(reg.attachment_labels, [])
        finally:
            try:
                root.destroy()
            except Exception:
                pass

    def test_destroy_all_with_dynamic_widgets(self):
        root = _make_root()
        try:
            reg = WidgetRegistry()
            reg.system_status_history = tk.Frame(root)
            reg.system_status_context = tk.Frame(root)
            reg.system_status_files = tk.Frame(root)
            reg.session_tab = tk.Frame(root)
            reg.files_tab = tk.Frame(root)
            reg.system_notebook = tk.ttk.Notebook(root)
            reg.system_status = tk.Frame(root)

            reg.destroy_all()
            # No exception = pass
        finally:
            try:
                root.destroy()
            except Exception:
                pass

    def test_destroy_all_with_output_panel_widgets(self):
        root = _make_root()
        try:
            reg = WidgetRegistry()
            reg.output_text = tk.Text(root)
            reg.output_scrollbar = tk.Scrollbar(root)
            reg.output_entries_frame = tk.Frame(root)
            reg.output_entries_canvas = tk.Canvas(root)
            reg.output_entries_scrollbar = tk.Scrollbar(root)
            reg.output_entries_container = tk.Frame(root)
            reg.output_notebook = tk.ttk.Notebook(root)
            reg.output_display = tk.Frame(root)

            reg.destroy_all()
        finally:
            try:
                root.destroy()
            except Exception:
                pass

    def test_destroy_all_skips_failed_plan_tab_destroy(self):
        """Plan tab destroy errors are caught and silently ignored."""
        reg = WidgetRegistry()
        bad_frame = MagicMock()
        bad_frame.destroy.side_effect = Exception("destroy failed")
        reg.plan_tabs["bad_plan"] = bad_frame
        reg.destroy_all()  # Should not raise
        self.assertEqual(reg.plan_tabs, {})

    def test_destroy_all_with_input_scrollbar_and_attachments_frame(self):
        root = _make_root()
        try:
            reg = WidgetRegistry()
            reg.input_scrollbar = tk.Scrollbar(root)
            reg.attachments_frame = tk.Frame(root)
            reg.destroy_all()
        finally:
            try:
                root.destroy()
            except Exception:
                pass
