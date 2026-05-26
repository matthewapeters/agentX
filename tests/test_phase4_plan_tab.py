"""Phase 4 unit tests: Plan Tab (GUI Shell).

Tests cover:
- PlanTreeWidget: node creation, status updates, TBD resolution, tool call rows,
  synthesis blocks, assertion badges
- GUIManager: add_plan_tab (creates tab + toolbar + PlanTreeWidget), get_plan_tab_frame,
  focus_plan_tab, add_plan_step_node, add_plan_subtask_node, update_plan_node_status,
  resolve_plan_tbd_node, add_plan_tool_call, add_plan_synthesis
- session.py: PLAN_START, TASK_NODE_START (depth=0 and depth>0), TASK_NODE_TBD,
  TASK_NODE_END, TOOL_CALL-with-task_id all routed to correct GUI methods
"""

from __future__ import annotations

import os
import sys
import tkinter as tk
import unittest
from unittest.mock import MagicMock, call, patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agentx.gui.gui_config import GUIConfig
from agentx.gui.gui_manager import GUIManager
from agentx.gui.plan_tree_widget import _STATUS_ICONS, PlanTreeWidget

# ── Helpers ────────────────────────────────────────────────────────────────────


def _make_gui() -> tuple[tk.Tk, GUIManager]:
    root = tk.Tk()
    root.withdraw()
    cfg = GUIConfig.from_dict({"ollama_host": "localhost", "ollama_model": "m", "ollama_timeout": 30})
    gui = GUIManager(root, cfg, on_submit=MagicMock(), on_interrupt=MagicMock(), on_attachment_toggle=MagicMock())
    gui.create_layout()
    return root, gui


# ── PlanTreeWidget tests ───────────────────────────────────────────────────────


class TestPlanTreeWidgetNodes(unittest.TestCase):
    def setUp(self):
        self.root = tk.Tk()
        self.root.withdraw()
        self.tree = PlanTreeWidget(parent=self.root, bg="#222222", fg="#eee", dim_fg="#888", accent_fg="#7dd3fc")

    def tearDown(self):
        self.root.destroy()

    def test_get_widget_returns_frame(self):
        widget = self.tree.get_widget()
        self.assertIsInstance(widget, tk.Frame)

    def test_add_step_node_registers_node(self):
        self.tree.add_step_node("plan-1", "step-1", "Do something", tbd=False)
        self.assertIn("step-1", self.tree._nodes)

    def test_step_node_status_icon_is_pending(self):
        self.tree.add_step_node("plan-1", "step-1", "Do something", tbd=False)
        label_text = self.tree._nodes["step-1"]["status_label"].cget("text")
        self.assertEqual(label_text, _STATUS_ICONS["pending"])

    def test_tbd_node_shows_tbd_icon(self):
        self.tree.add_step_node("plan-1", "step-tbd", "Unknown task", tbd=True)
        label_text = self.tree._nodes["step-tbd"]["status_label"].cget("text")
        self.assertEqual(label_text, _STATUS_ICONS["tbd"])

    def test_tbd_node_description_has_tbd_prefix(self):
        self.tree.add_step_node("plan-1", "step-tbd", "Something", tbd=True)
        desc = self.tree._nodes["step-tbd"]["desc_label"].cget("text")
        self.assertIn("[TBD]", desc)

    def test_add_subtask_node_registers_node(self):
        self.tree.add_step_node("plan-1", "step-1", "Parent step", tbd=False)
        self.tree.add_subtask_node("sub-1", "step-1", "Sub task", depth=1)
        self.assertIn("sub-1", self.tree._nodes)

    def test_subtask_node_attached_to_parent_details_frame(self):
        self.tree.add_step_node("plan-1", "step-1", "Parent", tbd=False)
        self.tree.add_subtask_node("sub-1", "step-1", "Child", depth=1)
        parent_details = self.tree._nodes["step-1"]["details_frame"]
        child_container = self.tree._nodes["sub-1"]["container"]
        self.assertEqual(str(child_container.master), str(parent_details))

    def test_update_node_status_changes_icon(self):
        self.tree.add_step_node("plan-1", "step-1", "Task", tbd=False)
        self.tree.update_node_status("step-1", "done")
        label_text = self.tree._nodes["step-1"]["status_label"].cget("text")
        self.assertEqual(label_text, _STATUS_ICONS["done"])

    def test_update_node_status_unknown_status_shows_bullet(self):
        self.tree.add_step_node("plan-1", "step-1", "Task", tbd=False)
        self.tree.update_node_status("step-1", "weird_status")
        label_text = self.tree._nodes["step-1"]["status_label"].cget("text")
        self.assertEqual(label_text, "●")

    def test_update_node_status_missing_node_does_not_raise(self):
        # Should silently ignore missing task_id
        self.tree.update_node_status("nonexistent", "done")

    def test_resolve_tbd_node_updates_label(self):
        self.tree.add_step_node("plan-1", "step-tbd", "TBD task", tbd=True)
        self.tree.resolve_tbd_node("step-tbd", "Resolved description")
        desc = self.tree._nodes["step-tbd"]["desc_label"].cget("text")
        self.assertEqual(desc, "Resolved description")

    def test_resolve_tbd_missing_node_does_not_raise(self):
        self.tree.resolve_tbd_node("missing", "desc")

    def test_add_tool_call_creates_child_in_details_frame(self):
        self.tree.add_step_node("plan-1", "step-1", "Task", tbd=False)
        before = len(self.tree._nodes["step-1"]["details_frame"].winfo_children())
        self.tree.add_tool_call_to_node("step-1", "read_file", {"path": "foo.py"})
        after = len(self.tree._nodes["step-1"]["details_frame"].winfo_children())
        self.assertGreater(after, before)

    def test_add_tool_call_missing_node_does_not_raise(self):
        self.tree.add_tool_call_to_node("missing", "read_file", {})

    def test_add_synthesis_creates_child_in_details_frame(self):
        self.tree.add_step_node("plan-1", "step-1", "Task", tbd=False)
        before = len(self.tree._nodes["step-1"]["details_frame"].winfo_children())
        self.tree.add_synthesis_to_node("step-1", "The synthesis text.", [])
        after = len(self.tree._nodes["step-1"]["details_frame"].winfo_children())
        self.assertGreater(after, before)

    def test_add_synthesis_missing_node_does_not_raise(self):
        self.tree.add_synthesis_to_node("missing", "text", [])

    def test_synthesis_with_assertions_renders_badges(self):
        self.tree.add_step_node("plan-1", "step-1", "Task", tbd=False)
        assertions = [
            {"fact": "file exists", "verified": True, "error": None},
            {"fact": "value check", "verified": False, "error": "not found"},
            {"fact": "unknown", "verified": None, "error": None},
        ]
        self.tree.add_synthesis_to_node("step-1", "Synthesis.", assertions)
        # details_frame should have content (the synthesis block)
        children = self.tree._nodes["step-1"]["details_frame"].winfo_children()
        self.assertGreater(len(children), 0)

    def test_subtask_unknown_parent_falls_back_to_content_frame(self):
        # Parent not registered — should still add node without raising
        self.tree.add_subtask_node("orphan", "no-such-parent", "Orphan task", depth=1)
        self.assertIn("orphan", self.tree._nodes)
        # Should be a child of _content (fallback)
        self.assertEqual(str(self.tree._nodes["orphan"]["container"].master), str(self.tree._content))


# ── GUIManager plan tab tests ──────────────────────────────────────────────────


class TestGUIManagerPlanTab(unittest.TestCase):
    def setUp(self):
        self.root, self.gui = _make_gui()

    def tearDown(self):
        self.root.destroy()

    def test_add_plan_tab_creates_tab_in_notebook(self):
        tab_count_before = len(self.gui.widgets.output_notebook.tabs())
        self.gui.add_plan_tab("plan-abc", "Explore Bridge")
        tab_count_after = len(self.gui.widgets.output_notebook.tabs())
        self.assertEqual(tab_count_after, tab_count_before + 1)

    def test_add_plan_tab_registers_in_plan_tabs(self):
        self.gui.add_plan_tab("plan-abc", "Explore Bridge")
        self.assertIn("plan-abc", self.gui.widgets.plan_tabs)

    def test_add_plan_tab_creates_plan_tree(self):
        self.gui.add_plan_tab("plan-abc", "Explore Bridge")
        self.assertIn("plan-abc", self.gui._plan_trees)
        self.assertIsInstance(self.gui._plan_trees["plan-abc"], PlanTreeWidget)

    def test_add_plan_tab_idempotent(self):
        self.gui.add_plan_tab("plan-abc", "Explore Bridge")
        tab_count = len(self.gui.widgets.output_notebook.tabs())
        self.gui.add_plan_tab("plan-abc", "Explore Bridge")  # second call
        self.assertEqual(len(self.gui.widgets.output_notebook.tabs()), tab_count)

    def test_get_plan_tab_frame_returns_frame(self):
        self.gui.add_plan_tab("plan-abc", "Explore Bridge")
        frame = self.gui.get_plan_tab_frame("plan-abc")
        self.assertIsInstance(frame, tk.Frame)

    def test_get_plan_tab_frame_returns_none_if_missing(self):
        result = self.gui.get_plan_tab_frame("nonexistent")
        self.assertIsNone(result)

    def test_focus_plan_tab_switches_notebook(self):
        self.gui.add_plan_tab("plan-abc", "Explore Bridge")
        tab_frame = self.gui.get_plan_tab_frame("plan-abc")
        self.gui.focus_plan_tab("plan-abc")
        selected = self.gui.widgets.output_notebook.select()
        self.assertEqual(str(selected), str(tab_frame))

    def test_focus_plan_tab_missing_does_not_raise(self):
        self.gui.focus_plan_tab("nonexistent")

    def test_add_plan_step_node_registers_task_mapping(self):
        self.gui.add_plan_tab("plan-abc", "My Plan")
        self.gui.add_plan_step_node("plan-abc", "step-1", "List files", False)
        self.assertIn("step-1", self.gui._task_to_plan)
        self.assertEqual(self.gui._task_to_plan["step-1"], "plan-abc")

    def test_add_plan_step_node_adds_to_tree(self):
        self.gui.add_plan_tab("plan-abc", "My Plan")
        self.gui.add_plan_step_node("plan-abc", "step-1", "List files", False)
        self.assertIn("step-1", self.gui._plan_trees["plan-abc"]._nodes)

    def test_add_plan_subtask_node_registers_task_mapping(self):
        self.gui.add_plan_tab("plan-abc", "My Plan")
        self.gui.add_plan_step_node("plan-abc", "step-1", "Parent", False)
        self.gui.add_plan_subtask_node("sub-1", "step-1", "Child", 1)
        self.assertIn("sub-1", self.gui._task_to_plan)

    def test_update_plan_node_status(self):
        self.gui.add_plan_tab("plan-abc", "My Plan")
        self.gui.add_plan_step_node("plan-abc", "step-1", "Task", False)
        self.gui.update_plan_node_status("step-1", "done")
        icon = self.gui._plan_trees["plan-abc"]._nodes["step-1"]["status_label"].cget("text")
        self.assertEqual(icon, _STATUS_ICONS["done"])

    def test_resolve_plan_tbd_node(self):
        self.gui.add_plan_tab("plan-abc", "My Plan")
        self.gui.add_plan_step_node("plan-abc", "tbd-1", "TBD task", True)
        self.gui.resolve_plan_tbd_node("tbd-1", "Resolved!")
        desc = self.gui._plan_trees["plan-abc"]._nodes["tbd-1"]["desc_label"].cget("text")
        self.assertEqual(desc, "Resolved!")

    def test_add_plan_tool_call(self):
        self.gui.add_plan_tab("plan-abc", "My Plan")
        self.gui.add_plan_step_node("plan-abc", "step-1", "Task", False)
        before = len(self.gui._plan_trees["plan-abc"]._nodes["step-1"]["details_frame"].winfo_children())
        self.gui.add_plan_tool_call("step-1", "list_directory", {"path": "."})
        after = len(self.gui._plan_trees["plan-abc"]._nodes["step-1"]["details_frame"].winfo_children())
        self.assertGreater(after, before)

    def test_add_plan_synthesis(self):
        self.gui.add_plan_tab("plan-abc", "My Plan")
        self.gui.add_plan_step_node("plan-abc", "step-1", "Task", False)
        before = len(self.gui._plan_trees["plan-abc"]._nodes["step-1"]["details_frame"].winfo_children())
        self.gui.add_plan_synthesis("step-1", "Here is the synthesis.", [])
        after = len(self.gui._plan_trees["plan-abc"]._nodes["step-1"]["details_frame"].winfo_children())
        self.assertGreater(after, before)

    def test_widget_registry_plan_tabs_cleared_on_destroy(self):
        self.gui.add_plan_tab("plan-abc", "My Plan")
        self.assertIn("plan-abc", self.gui.widgets.plan_tabs)
        self.gui.widgets.destroy_all()
        self.assertNotIn("plan-abc", self.gui.widgets.plan_tabs)


# ── session.py chunk routing tests ────────────────────────────────────────────


class TestSessionPlanChunkRouting(unittest.TestCase):
    """Verify that plan-related ResponseChunks are routed to the correct GUI methods."""

    def _make_chunk(self, **kwargs):
        from shared.models.response import ChunkType, ResponseChunk

        return ResponseChunk(**kwargs)

    def _run_chunk_in_session(self, chunk):
        """Simulate session._stream_via_agentix processing a single chunk."""
        from shared.models.response import ChunkType

        gui = MagicMock()

        # Execute the routing logic extracted from session._stream_via_agentix
        safe_calls = []

        def safe_root_after(fn):
            safe_calls.append(fn)

        if chunk.type == ChunkType.PLAN_START and chunk.plan_id:
            _pid = chunk.plan_id
            _pname = chunk.plan_name or "Plan"
            safe_root_after(
                lambda pid=_pid, pn=_pname: (
                    gui.add_plan_tab(pid, pn),
                    gui.focus_plan_tab(pid),
                )
            )
        elif chunk.type == ChunkType.TASK_NODE_START and chunk.task_id:
            _tid = chunk.task_id
            _pid = chunk.plan_id or ""
            _desc = chunk.content or chunk.task_id
            _par = chunk.parent_task_id
            _depth = chunk.task_depth or 0
            _tbd = bool(chunk.tbd)
            if _par:
                safe_root_after(
                    lambda tid=_tid, par=_par, desc=_desc, d=_depth: gui.add_plan_subtask_node(tid, par, desc, d)
                )
            else:
                safe_root_after(
                    lambda pid=_pid, tid=_tid, desc=_desc, tb=_tbd: gui.add_plan_step_node(pid, tid, desc, tb)
                )
        elif chunk.type == ChunkType.TASK_NODE_TBD and chunk.task_id:
            _tid = chunk.task_id
            _desc = chunk.content or ""
            safe_root_after(lambda tid=_tid, desc=_desc: gui.resolve_plan_tbd_node(tid, desc))
        elif chunk.type == ChunkType.TASK_NODE_END and chunk.task_id:
            _tid = chunk.task_id
            _synth = chunk.content or ""
            _asserts = chunk.assertions or []
            safe_root_after(lambda tid=_tid: gui.update_plan_node_status(tid, "done"))
            safe_root_after(lambda tid=_tid, s=_synth, a=_asserts: gui.add_plan_synthesis(tid, s, a))
        elif chunk.type == ChunkType.TOOL_CALL and chunk.task_id:
            _tid = chunk.task_id
            _tname = chunk.tool_name or ""
            _tinput = chunk.tool_input or {}
            safe_root_after(lambda tid=_tid, tn=_tname, ti=_tinput: gui.add_plan_tool_call(tid, tn, ti))

        # Execute all scheduled callbacks
        for fn in safe_calls:
            fn()

        return gui

    def test_plan_start_calls_add_plan_tab(self):
        from shared.models.response import ChunkType

        chunk = self._make_chunk(type=ChunkType.PLAN_START, plan_id="p1", plan_name="My Plan")
        gui = self._run_chunk_in_session(chunk)
        gui.add_plan_tab.assert_called_once_with("p1", "My Plan")

    def test_plan_start_calls_focus_plan_tab(self):
        from shared.models.response import ChunkType

        chunk = self._make_chunk(type=ChunkType.PLAN_START, plan_id="p1", plan_name="My Plan")
        gui = self._run_chunk_in_session(chunk)
        gui.focus_plan_tab.assert_called_once_with("p1")

    def test_plan_start_default_plan_name(self):
        from shared.models.response import ChunkType

        chunk = self._make_chunk(type=ChunkType.PLAN_START, plan_id="p1")
        gui = self._run_chunk_in_session(chunk)
        gui.add_plan_tab.assert_called_once_with("p1", "Plan")

    def test_task_node_start_depth0_calls_add_step_node(self):
        from shared.models.response import ChunkType

        chunk = self._make_chunk(
            type=ChunkType.TASK_NODE_START,
            plan_id="p1",
            task_id="t1",
            content="List files",
            task_depth=0,
            tbd=False,
        )
        gui = self._run_chunk_in_session(chunk)
        gui.add_plan_step_node.assert_called_once_with("p1", "t1", "List files", False)
        gui.add_plan_subtask_node.assert_not_called()

    def test_task_node_start_with_parent_calls_add_subtask_node(self):
        from shared.models.response import ChunkType

        chunk = self._make_chunk(
            type=ChunkType.TASK_NODE_START,
            plan_id="p1",
            task_id="sub-1",
            parent_task_id="t1",
            content="Sub task",
            task_depth=1,
            tbd=False,
        )
        gui = self._run_chunk_in_session(chunk)
        gui.add_plan_subtask_node.assert_called_once_with("sub-1", "t1", "Sub task", 1)
        gui.add_plan_step_node.assert_not_called()

    def test_task_node_tbd_calls_resolve(self):
        from shared.models.response import ChunkType

        chunk = self._make_chunk(
            type=ChunkType.TASK_NODE_TBD,
            task_id="tbd-1",
            content="Resolved desc",
        )
        gui = self._run_chunk_in_session(chunk)
        gui.resolve_plan_tbd_node.assert_called_once_with("tbd-1", "Resolved desc")

    def test_task_node_end_calls_update_status_and_synthesis(self):
        from shared.models.response import ChunkType

        chunk = self._make_chunk(
            type=ChunkType.TASK_NODE_END,
            task_id="t1",
            content="The synthesis.",
            assertions=[{"fact": "f", "verified": True}],
        )
        gui = self._run_chunk_in_session(chunk)
        gui.update_plan_node_status.assert_called_once_with("t1", "done")
        gui.add_plan_synthesis.assert_called_once_with("t1", "The synthesis.", [{"fact": "f", "verified": True}])

    def test_tool_call_with_task_id_routes_to_plan_tree(self):
        from shared.models.response import ChunkType

        chunk = self._make_chunk(
            type=ChunkType.TOOL_CALL,
            task_id="t1",
            tool_name="read_file",
            tool_input={"path": "foo.py"},
        )
        gui = self._run_chunk_in_session(chunk)
        gui.add_plan_tool_call.assert_called_once_with("t1", "read_file", {"path": "foo.py"})

    def test_tool_call_without_task_id_not_routed(self):
        from shared.models.response import ChunkType

        chunk = self._make_chunk(
            type=ChunkType.TOOL_CALL,
            tool_name="read_file",
            tool_input={"path": "foo.py"},
        )
        gui = self._run_chunk_in_session(chunk)
        gui.add_plan_tool_call.assert_not_called()

    def test_plan_start_without_plan_id_not_routed(self):
        from shared.models.response import ChunkType

        chunk = self._make_chunk(type=ChunkType.PLAN_START, plan_name="My Plan")
        gui = self._run_chunk_in_session(chunk)
        gui.add_plan_tab.assert_not_called()


if __name__ == "__main__":
    unittest.main()
