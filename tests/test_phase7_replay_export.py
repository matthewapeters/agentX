"""Phase 7 Tests: Replay & Export

Tests covering:
  - replay_task_node_streaming in bridge delegates to _run_task_node with cleaned context
  - replay_task_node_generator in adapter delegates to bridge
  - PlanTreeWidget on_replay button appears per-node when callback provided
  - gui_manager.add_plan_tab wires on_export to Export button
  - gui_manager.add_plan_step_node/add_plan_subtask_node pass on_replay through
  - History.open_task_tree loads TaskTree from session path, returns None for missing
  - session._export_task_tree writes task_tree_export.md
  - session._replay_subtask updates GUI and calls adapter on background thread
  - session._replay_plan_tab passes on_export and on_replay callbacks
"""

import os
import sys
import tempfile
import threading
import tkinter as tk
import unittest
from datetime import datetime
from pathlib import Path
from unittest.mock import MagicMock, patch, call

project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from agentx.gui.gui_manager import GUIManager
from agentx.gui.gui_config import GUIConfig
from shared.models.message import Message, MessageRole
from shared.models.response import ResponseChunk, ChunkType
from shared.models.task_node import PlanRecord, PlanStep, TaskNodeRecord, TaskTree

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_root() -> tk.Tk:
    root = tk.Tk()
    root.withdraw()
    return root


def _make_gui(root: tk.Tk) -> GUIManager:
    config_dict = {
        "ollama_host": "localhost",
        "ollama_model": "test-model",
        "ollama_timeout": 30,
    }
    config = GUIConfig.from_dict(config_dict)
    return GUIManager(
        root=root,
        config=config,
        on_submit=MagicMock(),
        on_interrupt=MagicMock(),
        on_attachment_toggle=MagicMock(),
    )


def _make_session(test_dir: str, chunks=None):
    from agentx.session import AgentXSession

    if chunks is None:
        chunks = []
    config = {
        "agentx": {
            "ollama_host": "localhost:11434",
            "ollama_model": "gpt-oss",
        },
        "agentix": {
            "classify_prompts": False,
        },
    }

    mock_adapter = MagicMock()
    mock_adapter.process_prompt_generator.side_effect = lambda *a, **kw: iter(chunks)
    mock_adapter.classify_prompt_sync.return_value = None
    mock_adapter.get_models.return_value = []
    mock_adapter.get_tools.return_value = []

    with patch("agentx.session.create_adapter", return_value=mock_adapter):
        session = AgentXSession(username="tester", session_dir=test_dir, config=config)
    return session, mock_adapter


def _make_plan_and_nodes(plan_id="plan1", node_count=2):
    """Return a PlanRecord and list of TaskNodeRecords for testing."""
    plan = PlanRecord(
        plan_id=plan_id,
        plan_name="Test Plan",
        steps=[PlanStep(step_id=f"step_{i}", description=f"Step {i}") for i in range(node_count)],
        root_task_ids=[f"task_{i}" for i in range(node_count)],
        status="done",
    )
    nodes = [
        TaskNodeRecord(
            plan_id=plan_id,
            task_id=f"task_{i}",
            depth=0,
            plan_step_index=i,
            task_description=f"Step {i}",
            status="done",
            epoch=float(i),
        )
        for i in range(node_count)
    ]
    return plan, nodes


# ---------------------------------------------------------------------------
# 1. Bridge replay_task_node_streaming
# ---------------------------------------------------------------------------


class TestBridgeReplayTaskNodeStreaming(unittest.TestCase):
    """replay_task_node_streaming should call _run_task_node with excluded child msgs."""

    def _make_bridge(self):
        from agentix.bridge.bridge import AgentixBridge

        config = MagicMock()
        config.max_tool_rounds = 3
        config.max_task_depth = 10
        bridge = AgentixBridge.__new__(AgentixBridge)
        bridge.config = config
        bridge._tool_impls = {}
        bridge._tool_schemas = []
        return bridge

    def test_replay_calls_run_task_node(self):
        bridge = self._make_bridge()

        sentinel = [ResponseChunk(type=ChunkType.DONE, content="")]
        called_with = {}

        def fake_run_task_node(**kwargs):
            called_with.update(kwargs)
            yield from sentinel

        bridge._run_task_node = fake_run_task_node

        from shared.models.context import Context

        ctx = Context()
        tree = TaskTree(session_id="s1")
        node = TaskNodeRecord(
            plan_id="p1",
            task_id="t1",
            depth=0,
            plan_step_index=0,
            task_description="do something",
            status="done",
            child_message_epochs=[1.0, 2.0],
            epoch=1.0,
        )
        tree.add_node(node)
        tree.plans["p1"] = PlanRecord(plan_id="p1", plan_name="P", root_task_ids=["t1"], status="done")

        results = list(bridge.replay_task_node_streaming(node, ctx, tree))

        self.assertEqual(results, sentinel)
        self.assertEqual(called_with["task_id"], "t1")
        self.assertEqual(called_with["plan_id"], "p1")
        self.assertEqual(called_with["task_description"], "do something")
        self.assertEqual(called_with["depth"], 0)
        self.assertFalse(called_with["tbd"])

    def test_replay_excludes_child_message_epochs(self):
        """base_messages passed to _run_task_node must not include node's child epochs."""
        bridge = self._make_bridge()

        passed_messages = {}

        def fake_run_task_node(**kwargs):
            passed_messages["initial_messages"] = kwargs["initial_messages"]
            return iter([])

        bridge._run_task_node = fake_run_task_node

        from shared.models.context import Context
        from shared.models.message import Message, MessageRole

        ctx = Context()
        msg_keep = Message(role=MessageRole.USER, content="hello", timestamp=datetime.fromtimestamp(0.5))
        msg_exclude = Message(role=MessageRole.ASSISTANT, content="tool result", timestamp=datetime.fromtimestamp(1.0))
        ctx.add_message(msg_keep)
        ctx.add_message(msg_exclude)

        tree = TaskTree(session_id="s1")
        node = TaskNodeRecord(
            plan_id="p1",
            task_id="t1",
            depth=0,
            task_description="task",
            status="done",
            child_message_epochs=[1.0],
            epoch=0.0,
        )
        tree.add_node(node)
        tree.plans["p1"] = PlanRecord(plan_id="p1", plan_name="P", root_task_ids=["t1"], status="done")

        list(bridge.replay_task_node_streaming(node, ctx, tree))

        msgs = passed_messages.get("initial_messages", [])
        # Only msg_keep (epoch=0.5) should be included; msg_exclude (epoch=1.0) filtered out
        self.assertEqual(len(msgs), 1)

    def test_replay_uses_tbd_resolved_description(self):
        bridge = self._make_bridge()

        called_desc = {}

        def fake_run_task_node(**kwargs):
            called_desc["desc"] = kwargs["task_description"]
            return iter([])

        bridge._run_task_node = fake_run_task_node

        from shared.models.context import Context

        ctx = Context()
        tree = TaskTree(session_id="s1")
        node = TaskNodeRecord(
            plan_id="p1",
            task_id="t1",
            depth=0,
            task_description="[TBD] find something",
            tbd=True,
            tbd_resolved_description="find the config file",
            status="done",
            epoch=0.0,
        )
        tree.add_node(node)
        tree.plans["p1"] = PlanRecord(plan_id="p1", plan_name="P", root_task_ids=["t1"], status="done")

        list(bridge.replay_task_node_streaming(node, ctx, tree))

        self.assertEqual(called_desc["desc"], "find the config file")


# ---------------------------------------------------------------------------
# 2. Adapter replay_task_node_generator
# ---------------------------------------------------------------------------


class TestAdapterReplayTaskNodeGenerator(unittest.TestCase):
    def _make_adapter(self):
        from agentx.integration.agentix_bridge_adapter import AgentixBridgeAdapter

        adapter = AgentixBridgeAdapter.__new__(AgentixBridgeAdapter)
        adapter.bridge = MagicMock()
        return adapter

    def test_replay_generator_delegates_to_bridge(self):
        adapter = self._make_adapter()
        sentinel = [ResponseChunk(type=ChunkType.DONE, content="")]
        adapter.bridge.replay_task_node_streaming.return_value = iter(sentinel)

        node = MagicMock()
        ctx = MagicMock()
        tree = MagicMock()

        results = list(adapter.replay_task_node_generator(node, ctx, tree))

        adapter.bridge.replay_task_node_streaming.assert_called_once_with(node, ctx, tree)
        self.assertEqual(results, sentinel)

    def test_replay_generator_yields_error_on_exception(self):
        adapter = self._make_adapter()
        adapter.bridge.replay_task_node_streaming.side_effect = RuntimeError("bridge error")

        results = list(adapter.replay_task_node_generator(MagicMock(), MagicMock(), MagicMock()))

        self.assertEqual(len(results), 1)
        self.assertEqual(results[0].type, ChunkType.ERROR)
        self.assertIn("bridge error", results[0].content)


# ---------------------------------------------------------------------------
# 3. PlanTreeWidget on_replay button
# ---------------------------------------------------------------------------


class TestPlanTreeWidgetOnReplay(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def _make_tree(self):
        from agentx.gui.plan_tree_widget import PlanTreeWidget

        frame = tk.Frame(self.root)
        return PlanTreeWidget(parent=frame)

    def test_no_replay_button_when_no_callback(self):
        tree = self._make_tree()
        tree.add_step_node("p1", "t1", "Step 1", False)
        node = tree._nodes["t1"]
        row = node["row"]
        buttons = [w for w in row.winfo_children() if isinstance(w, tk.Button)]
        # Only the toggle button — no Replay button
        self.assertEqual(len(buttons), 1)

    def test_replay_button_added_when_callback_provided(self):
        tree = self._make_tree()
        callback = MagicMock()
        tree.add_step_node("p1", "t1", "Step 1", False, on_replay=callback)
        node = tree._nodes["t1"]
        row = node["row"]
        buttons = [w for w in row.winfo_children() if isinstance(w, tk.Button)]
        # Toggle button + Replay button = 2
        self.assertEqual(len(buttons), 2)

    def test_replay_button_calls_callback_with_task_id(self):
        tree = self._make_tree()
        received = []
        tree.add_step_node("p1", "t1", "Step 1", False, on_replay=lambda tid: received.append(tid))
        node = tree._nodes["t1"]
        row = node["row"]
        buttons = [w for w in row.winfo_children() if isinstance(w, tk.Button)]
        # Invoke the Replay button
        replay_btn = next(b for b in buttons if "Replay" in str(b.cget("text")))
        replay_btn.invoke()
        self.assertEqual(received, ["t1"])

    def test_replay_button_on_subtask_node(self):
        tree = self._make_tree()
        callback = MagicMock()
        tree.add_step_node("p1", "t_root", "Root step", False)
        tree.add_subtask_node("t_sub", "t_root", "Sub-task", 1, on_replay=callback)
        node = tree._nodes["t_sub"]
        row = node["row"]
        buttons = [w for w in row.winfo_children() if isinstance(w, tk.Button)]
        self.assertEqual(len(buttons), 2)


# ---------------------------------------------------------------------------
# 4. GUIManager.add_plan_tab wires Export button
# ---------------------------------------------------------------------------


class TestGUIManagerAddPlanTabExport(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()
        self.gui = _make_gui(self.root)

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def _setup_notebook(self):
        self.gui.create_layout()

    def test_export_callback_stored_and_callable(self):
        self._setup_notebook()
        called = []
        self.gui.add_plan_tab("p1", "My Plan", on_export=lambda: called.append(True))
        # Find the tab frame and look for the Export button
        tab_frame = self.gui.get_plan_tab_frame("p1")
        self.assertIsNotNone(tab_frame)
        # Toolbar is first child
        toolbar = tab_frame.winfo_children()[0]
        export_btn = next(
            w for w in toolbar.winfo_children() if isinstance(w, tk.Button) and "Export" in str(w.cget("text"))
        )
        export_btn.invoke()
        self.assertEqual(called, [True])

    def test_no_export_callback_button_is_noop(self):
        self._setup_notebook()
        self.gui.add_plan_tab("p2", "Plan 2")  # no on_export
        tab_frame = self.gui.get_plan_tab_frame("p2")
        toolbar = tab_frame.winfo_children()[0]
        export_btn = next(
            w for w in toolbar.winfo_children() if isinstance(w, tk.Button) and "Export" in str(w.cget("text"))
        )
        # Should not raise
        export_btn.invoke()


# ---------------------------------------------------------------------------
# 5. GUIManager add_plan_step_node / add_plan_subtask_node pass on_replay
# ---------------------------------------------------------------------------


class TestGUIManagerOnReplayPassthrough(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()
        self.gui = _make_gui(self.root)
        self.gui.create_layout()

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_add_plan_step_node_passes_on_replay(self):
        self.gui.add_plan_tab("p1", "Plan")
        received = []
        self.gui.add_plan_step_node("p1", "t1", "Step 1", False, on_replay=lambda tid: received.append(tid))
        tree = self.gui._plan_trees["p1"]
        node = tree._nodes.get("t1")
        self.assertIsNotNone(node)
        row = node["row"]
        buttons = [w for w in row.winfo_children() if isinstance(w, tk.Button)]
        self.assertEqual(len(buttons), 2)  # toggle + replay

    def test_add_plan_subtask_node_passes_on_replay(self):
        self.gui.add_plan_tab("p1", "Plan")
        self.gui.add_plan_step_node("p1", "t_root", "Root", False)
        received = []
        self.gui.add_plan_subtask_node("t_sub", "t_root", "Sub", 1, on_replay=lambda tid: received.append(tid))
        tree = self.gui._plan_trees["p1"]
        node = tree._nodes.get("t_sub")
        self.assertIsNotNone(node)
        row = node["row"]
        buttons = [w for w in row.winfo_children() if isinstance(w, tk.Button)]
        self.assertEqual(len(buttons), 2)  # toggle + replay


# ---------------------------------------------------------------------------
# 6. History.open_task_tree
# ---------------------------------------------------------------------------


class TestHistoryOpenTaskTree(unittest.TestCase):
    def test_returns_none_when_no_task_tree_json(self):
        from agentx.history import History

        with tempfile.TemporaryDirectory() as tmp:
            result = History.open_task_tree(tmp)
        self.assertIsNone(result)

    def test_loads_task_tree_from_valid_session(self):
        from agentx.history import History

        with tempfile.TemporaryDirectory() as tmp:
            tree = TaskTree(session_id="sess1")
            tree.add_plan(PlanRecord(plan_id="p1", plan_name="P", status="done"))
            tree.save(tmp)

            loaded = History.open_task_tree(tmp)

        self.assertIsNotNone(loaded)
        self.assertEqual(loaded.session_id, "sess1")
        self.assertIn("p1", loaded.plans)

    def test_returns_none_for_corrupt_json(self):
        from agentx.history import History

        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "task_tree.json")
            with open(path, "w") as fh:
                fh.write("not valid json {{{")

            result = History.open_task_tree(tmp)

        self.assertIsNone(result)


# ---------------------------------------------------------------------------
# 7. session._export_task_tree
# ---------------------------------------------------------------------------


class TestSessionExportTaskTree(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()

    def tearDown(self):
        import shutil

        shutil.rmtree(self.tmp, ignore_errors=True)

    def _build_session_with_plan(self):
        session, _ = _make_session(self.tmp)
        plan, nodes = _make_plan_and_nodes("plan_export", node_count=2)
        # Save plan and nodes into the session context
        session.context.save_plan(plan)
        for node in nodes:
            session.context.save_task_node(node)
        return session, plan, nodes

    def test_export_creates_markdown_file(self):
        session, plan, _ = self._build_session_with_plan()
        session.gui.display_error = MagicMock()
        session._export_task_tree("plan_export")
        export_path = os.path.join(session.session_folder, "task_tree_export.md")
        self.assertTrue(os.path.isfile(export_path))

    def test_export_markdown_contains_plan_name(self):
        session, plan, _ = self._build_session_with_plan()
        session.gui.display_error = MagicMock()
        session._export_task_tree("plan_export")
        export_path = os.path.join(session.session_folder, "task_tree_export.md")
        content = open(export_path).read()
        self.assertIn("Test Plan", content)

    def test_export_markdown_contains_node_descriptions(self):
        session, plan, nodes = self._build_session_with_plan()
        session.gui.display_error = MagicMock()
        session._export_task_tree("plan_export")
        export_path = os.path.join(session.session_folder, "task_tree_export.md")
        content = open(export_path).read()
        self.assertIn("Step 0", content)
        self.assertIn("Step 1", content)

    def test_export_missing_plan_calls_display_error(self):
        session, _ = _make_session(self.tmp)
        session.gui.display_error = MagicMock()
        session._export_task_tree("nonexistent_plan")
        session.gui.display_error.assert_called_once()
        args = session.gui.display_error.call_args[0]
        self.assertIn("nonexistent_plan", args[0])

    def test_export_nested_subtasks_indented(self):
        session, _ = _make_session(self.tmp)
        plan = PlanRecord(plan_id="p_nest", plan_name="Nested Plan", root_task_ids=["root1"], status="done")
        root_node = TaskNodeRecord(
            plan_id="p_nest", task_id="root1", depth=0, task_description="Root step", status="done", epoch=1.0
        )
        child_node = TaskNodeRecord(
            plan_id="p_nest",
            task_id="child1",
            parent_task_id="root1",
            depth=1,
            task_description="Child step",
            status="done",
            epoch=2.0,
        )
        session.context.save_plan(plan)
        session.context.save_task_node(root_node)
        session.context.save_task_node(child_node)
        session.gui.display_error = MagicMock()
        session._export_task_tree("p_nest")
        export_path = os.path.join(session.session_folder, "task_tree_export.md")
        content = open(export_path).read()
        lines = content.splitlines()
        root_line = next(l for l in lines if "Root step" in l)
        child_line = next(l for l in lines if "Child step" in l)
        # Child should be indented more than root
        root_indent = len(root_line) - len(root_line.lstrip(" "))
        child_indent = len(child_line) - len(child_line.lstrip(" "))
        self.assertGreater(child_indent, root_indent)


# ---------------------------------------------------------------------------
# 8. session._replay_subtask
# ---------------------------------------------------------------------------


class TestSessionReplaySubtask(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()

    def tearDown(self):
        import shutil

        shutil.rmtree(self.tmp, ignore_errors=True)

    def _build_session_with_tree(self):
        session, adapter = _make_session(self.tmp)
        tree = TaskTree(session_id=session.session_id)
        plan = PlanRecord(plan_id="p1", plan_name="P", root_task_ids=["t1"], status="done")
        node = TaskNodeRecord(plan_id="p1", task_id="t1", depth=0, task_description="do it", status="done", epoch=1.0)
        tree.add_plan(plan)
        tree.add_node(node)
        session.context.save_task_tree(tree)
        return session, adapter

    def test_replay_subtask_calls_adapter_generator(self):
        session, adapter = self._build_session_with_tree()
        done = threading.Event()

        def _side_effect(*a, **kw):
            done.set()
            return iter(
                [
                    ResponseChunk(
                        type=ChunkType.TASK_NODE_END, task_id="t1", plan_id="p1", content="synthesis", assertions=[]
                    ),
                ]
            )

        adapter.replay_task_node_generator.side_effect = _side_effect
        session.gui.update_plan_node_status = MagicMock()
        session.gui.update_plan_synthesis = MagicMock()
        session.gui.set_streaming_state = MagicMock()

        session._replay_subtask("t1")
        done.wait(timeout=2.0)

        adapter.replay_task_node_generator.assert_called_once()

    def test_replay_subtask_noop_when_streaming(self):
        session, adapter = self._build_session_with_tree()
        session._is_streaming.set()
        session.gui.display_error = MagicMock()

        session._replay_subtask("t1")

        adapter.replay_task_node_generator.assert_not_called()
        session.gui.display_error.assert_called_once()

    def test_replay_subtask_error_when_no_task_tree(self):
        session, _ = _make_session(self.tmp)
        session.gui.display_error = MagicMock()

        session._replay_subtask("nonexistent")

        session.gui.display_error.assert_called_once()

    def test_replay_subtask_error_when_task_id_not_in_tree(self):
        session, adapter = self._build_session_with_tree()
        session.gui.display_error = MagicMock()

        session._replay_subtask("unknown_task")

        session.gui.display_error.assert_called_once()
        adapter.replay_task_node_generator.assert_not_called()


# ---------------------------------------------------------------------------
# 9. session._replay_plan_tab passes callbacks
# ---------------------------------------------------------------------------


class TestReplayPlanTabCallbacks(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()

    def tearDown(self):
        import shutil

        shutil.rmtree(self.tmp, ignore_errors=True)

    def test_replay_plan_tab_passes_on_export_to_add_plan_tab(self):
        session, _ = _make_session(self.tmp)
        plan, nodes = _make_plan_and_nodes("plan_cb")
        session.context.save_plan(plan)
        for n in nodes:
            session.context.save_task_node(n)

        session.gui.add_plan_tab = MagicMock(return_value=MagicMock())
        session.gui.focus_plan_tab = MagicMock()
        session.gui.add_plan_step_node = MagicMock()
        session.gui.update_plan_node_status = MagicMock()

        session._replay_plan_tab("plan_cb")

        call_kwargs = session.gui.add_plan_tab.call_args[1]
        self.assertIn("on_export", call_kwargs)
        self.assertIsNotNone(call_kwargs["on_export"])

    def test_replay_plan_tab_passes_on_replay_to_add_plan_step_node(self):
        session, _ = _make_session(self.tmp)
        plan, nodes = _make_plan_and_nodes("plan_replay_cb")
        session.context.save_plan(plan)
        for n in nodes:
            session.context.save_task_node(n)

        session.gui.add_plan_tab = MagicMock(return_value=MagicMock())
        session.gui.focus_plan_tab = MagicMock()
        session.gui.add_plan_step_node = MagicMock()
        session.gui.update_plan_node_status = MagicMock()

        session._replay_plan_tab("plan_replay_cb")

        # Every add_plan_step_node call should have on_replay kwarg
        for c in session.gui.add_plan_step_node.call_args_list:
            self.assertIn("on_replay", c[1])
            self.assertIsNotNone(c[1]["on_replay"])


if __name__ == "__main__":
    unittest.main()
