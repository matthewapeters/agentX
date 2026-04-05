"""Tests for src/agentx/gui/tool_panel.py — ToolPanel widget."""

import tkinter as tk
import unittest
from unittest.mock import MagicMock


def _make_root() -> tk.Tk:
    root = tk.Tk()
    root.withdraw()
    return root


_TOOLS = [
    {"name": "cst", "description": "Concrete Syntax Tree analysis"},
    {"name": "ast", "description": "Abstract Syntax Tree analysis"},
]


class TestToolPanelInit(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()

    def tearDown(self):
        self.root.destroy()

    def test_widget_created(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        widget = panel.get_widget()
        self.assertIsNotNone(widget)
        self.assertIsInstance(widget, tk.Widget)

    def test_initial_state_expanded(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        self.assertTrue(panel.expanded_var.get())

    def test_tool_vars_initially_empty(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        self.assertEqual(panel.tool_vars, {})


class TestToolPanelPopulate(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()

    def tearDown(self):
        self.root.destroy()

    def test_populate_creates_tool_vars(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel.populate(_TOOLS)
        self.assertIn("cst", panel.tool_vars)
        self.assertIn("ast", panel.tool_vars)

    def test_populate_enables_by_default(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel.populate(_TOOLS)
        for name in ["cst", "ast"]:
            self.assertTrue(panel.tool_vars[name].get(), f"{name} should be enabled by default")

    def test_populate_with_empty_list(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel.populate([])
        self.assertEqual(panel.tool_vars, {})

    def test_populate_replaces_previous_tools(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel.populate(_TOOLS)
        panel.populate([{"name": "only_tool", "description": "single"}])
        self.assertNotIn("cst", panel.tool_vars)
        self.assertIn("only_tool", panel.tool_vars)


class TestToolPanelGetEnabledTools(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()

    def tearDown(self):
        self.root.destroy()

    def test_all_enabled_by_default(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel.populate(_TOOLS)
        self.assertEqual(set(panel.get_enabled_tools()), {"cst", "ast"})

    def test_disabled_tool_excluded(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel.populate(_TOOLS)
        panel.tool_vars["cst"].set(False)
        self.assertNotIn("cst", panel.get_enabled_tools())
        self.assertIn("ast", panel.get_enabled_tools())

    def test_empty_panel_returns_empty_list(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        self.assertEqual(panel.get_enabled_tools(), [])


class TestToolPanelSetToolEnabled(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()

    def tearDown(self):
        self.root.destroy()

    def test_set_tool_enabled_false(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel.populate(_TOOLS)
        panel.set_tool_enabled("cst", False)
        self.assertFalse(panel.tool_vars["cst"].get())

    def test_set_tool_enabled_true(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel.populate(_TOOLS)
        panel.tool_vars["cst"].set(False)
        panel.set_tool_enabled("cst", True)
        self.assertTrue(panel.tool_vars["cst"].get())

    def test_set_unknown_tool_is_noop(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel.populate(_TOOLS)
        # Should not raise
        panel.set_tool_enabled("nonexistent", False)


class TestToolPanelOnToolToggleCallback(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()

    def tearDown(self):
        self.root.destroy()

    def test_callback_fired_with_correct_args(self):
        from agentx.gui.tool_panel import ToolPanel

        callback = MagicMock()
        panel = ToolPanel(self.root, on_tool_toggle=callback)
        panel.populate(_TOOLS)
        panel._on_tool_toggle("cst", False)
        callback.assert_called_with("cst", False)

    def test_callback_updates_var(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel.populate(_TOOLS)
        panel._on_tool_toggle("cst", False)
        self.assertFalse(panel.tool_vars["cst"].get())


class TestToolPanelExpandCollapse(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()

    def tearDown(self):
        self.root.destroy()

    def test_toggle_expand_collapses(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        self.assertTrue(panel.expanded_var.get())
        panel._toggle_expand()
        self.assertFalse(panel.expanded_var.get())

    def test_toggle_expand_re_expands(self):
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        panel._toggle_expand()
        panel._toggle_expand()
        self.assertTrue(panel.expanded_var.get())


# ---------------------------------------------------------------------------
# Coverage uplift: tooltip show/hide, populate with description
# ---------------------------------------------------------------------------


class TestToolPanelTooltip(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()

    def tearDown(self):
        self.root.destroy()

    def test_show_tooltip_creates_toplevel(self):
        """_show_tooltip creates a Tk Toplevel window."""
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        label = tk.Label(self.root, text="test")
        label.pack()
        self.root.update_idletasks()

        panel._show_tooltip(label, "Tooltip text")
        self.assertIsNotNone(panel._tooltip)
        self.assertIsInstance(panel._tooltip, tk.Toplevel)

    def test_hide_tooltip_destroys_toplevel(self):
        """_hide_tooltip destroys the Toplevel and sets _tooltip to None."""
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        label = tk.Label(self.root, text="test")
        label.pack()
        self.root.update_idletasks()

        panel._show_tooltip(label, "text")
        self.assertIsNotNone(panel._tooltip)
        panel._hide_tooltip()
        self.assertIsNone(panel._tooltip)

    def test_hide_tooltip_with_no_existing_tooltip_is_noop(self):
        """_hide_tooltip when no tooltip exists does not raise."""
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        # No _tooltip set yet — should be a no-op
        panel._hide_tooltip()

    def test_show_tooltip_replaces_existing(self):
        """Calling _show_tooltip twice replaces the old tooltip."""
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        label = tk.Label(self.root, text="test")
        label.pack()
        self.root.update_idletasks()

        panel._show_tooltip(label, "first")
        first = panel._tooltip
        panel._show_tooltip(label, "second")
        # A new Toplevel should have been created
        self.assertIsNotNone(panel._tooltip)
        # The first one is destroyed, a new object is in place
        self.assertIsNot(first, panel._tooltip)

    def test_populate_with_descriptions_adds_tooltips(self):
        """populate with descriptions binds tooltips to description labels."""
        from agentx.gui.tool_panel import ToolPanel

        panel = ToolPanel(self.root, on_tool_toggle=MagicMock())
        tools = [
            {"name": "read_file", "description": "Read file contents"},
            {"name": "write_file", "description": "Write to a file"},
        ]
        panel.populate(tools)
        # After populate, both tool labels exist in the panel
        self.assertEqual(len(panel.tool_vars), 2)
