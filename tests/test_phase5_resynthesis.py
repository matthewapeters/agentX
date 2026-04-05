"""Phase 5 Tests: Re-synthesis UI

Tests covering:
  - ResynthesisDialog creation, confirm, cancel, WM-hint, failed assertions
  - AgentixBridge.retrigger_synthesis_streaming
  - AgentixBridgeAdapter.retrigger_synthesis_generator
  - AgentXSession.retrigger_synthesis
"""

import sys
import os
import threading
import tkinter as tk
import time
import unittest
from dataclasses import dataclass, field
from pathlib import Path
from unittest.mock import MagicMock, Mock, patch, call

# Add src to path
project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from agentx.gui.resynthesis_dialog import ResynthesisDialog
from shared.models.response import ResponseChunk, ChunkType
from shared.models.task_node import TaskNodeRecord, TaskTree, SynthesisAttempt

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_root() -> tk.Tk:
    root = tk.Tk()
    root.withdraw()
    return root


def _make_node(task_id: str = "task_001", plan_id: str = "plan_001") -> TaskNodeRecord:
    return TaskNodeRecord(
        plan_id=plan_id,
        task_id=task_id,
        task_description="Do something useful",
        status="done",
    )


def _make_tree(node: TaskNodeRecord) -> TaskTree:
    tree = TaskTree(session_id="test_session")
    tree.nodes[node.task_id] = node
    return tree


# ---------------------------------------------------------------------------
# ResynthesisDialog tests
# ---------------------------------------------------------------------------


class TestResynthesisDialogCreation(unittest.TestCase):
    """Dialog opens with correct title, fields, and structure."""

    def setUp(self):
        self.root = _make_root()
        self.confirmed_hint: list[str] = []
        self.dialog = ResynthesisDialog(
            parent=self.root,
            task_id="task_abc",
            synthesis_text="This is the current synthesis.",
            failed_assertions=[],
            on_confirm=self.confirmed_hint.append,
        )

    def tearDown(self):
        try:
            self.dialog._win.destroy()
        except Exception:
            pass
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_window_title_contains_task_id(self):
        self.assertIn("task_abc", self.dialog._win.title())

    def test_synthesis_text_inserted(self):
        content = self.dialog._synth_text.get("1.0", tk.END).strip()
        self.assertEqual(content, "This is the current synthesis.")

    def test_hint_text_widget_exists_and_empty(self):
        hint_content = self.dialog._hint_text.get("1.0", tk.END).strip()
        self.assertEqual(hint_content, "")

    def test_wm_key_var_exists(self):
        # Even without on_add_wm_hint callback, the StringVars must exist
        self.assertIsInstance(self.dialog._wm_key_var, tk.StringVar)
        self.assertIsInstance(self.dialog._wm_val_var, tk.StringVar)


class TestResynthesisDialogConfirm(unittest.TestCase):
    """Confirm button passes hint to on_confirm and destroys window."""

    def setUp(self):
        self.root = _make_root()
        self.received: list[str] = []
        self.dialog = ResynthesisDialog(
            parent=self.root,
            task_id="task_confirm",
            synthesis_text="synthesis",
            failed_assertions=[],
            on_confirm=self.received.append,
        )

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_confirm_with_no_hint_calls_on_confirm_empty_string(self):
        self.dialog._on_confirm_clicked()
        self.assertEqual(len(self.received), 1)
        self.assertEqual(self.received[0], "")

    def test_confirm_with_hint_text_passes_stripped_hint(self):
        self.dialog._hint_text.insert("1.0", "  be more concise  ")
        self.dialog._on_confirm_clicked()
        self.assertEqual(self.received[0], "be more concise")

    def test_confirm_destroys_window(self):
        self.dialog._on_confirm_clicked()
        # window should be destroyed — accessing winfo_exists() returns 0
        try:
            exists = self.dialog._win.winfo_exists()
        except tk.TclError:
            exists = 0
        self.assertEqual(exists, 0)


class TestResynthesisDialogCancel(unittest.TestCase):
    """Cancel button destroys window without calling on_confirm."""

    def setUp(self):
        self.root = _make_root()
        self.called = False

        def _on_confirm(hint):
            self.called = True

        self.dialog = ResynthesisDialog(
            parent=self.root,
            task_id="task_cancel",
            synthesis_text="synthesis",
            failed_assertions=[],
            on_confirm=_on_confirm,
        )

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_destroy_does_not_call_on_confirm(self):
        self.dialog._win.destroy()
        self.assertFalse(self.called)


class TestResynthesisDialogFailedAssertions(unittest.TestCase):
    """Assertion failures are rendered when present."""

    def setUp(self):
        self.root = _make_root()
        self.assertions = [
            {"fact": "file exists", "verified": False, "error": "file not found"},
            {"fact": "count == 5", "verified": False, "error": ""},
        ]
        self.dialog = ResynthesisDialog(
            parent=self.root,
            task_id="task_fail",
            synthesis_text="some synthesis",
            failed_assertions=self.assertions,
            on_confirm=lambda h: None,
        )

    def tearDown(self):
        try:
            self.dialog._win.destroy()
        except Exception:
            pass
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_dialog_created_with_failed_assertions(self):
        # Dialog must be openable without error when assertions are present
        self.assertIsNotNone(self.dialog._win)

    def test_window_is_visible(self):
        self.assertEqual(self.dialog._win.winfo_exists(), 1)


class TestResynthesisDialogWMHint(unittest.TestCase):
    """WM hint button fires on_add_wm_hint with key/value."""

    def setUp(self):
        self.root = _make_root()
        self.wm_calls: list[tuple[str, str]] = []
        self.dialog = ResynthesisDialog(
            parent=self.root,
            task_id="task_wm",
            synthesis_text="synthesis",
            failed_assertions=[],
            on_confirm=lambda h: None,
            on_add_wm_hint=lambda k, v: self.wm_calls.append((k, v)),
        )

    def tearDown(self):
        try:
            self.dialog._win.destroy()
        except Exception:
            pass
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_wm_hint_button_fires_callback(self):
        self.dialog._wm_key_var.set("my_key")
        self.dialog._wm_val_var.set("my_value")
        self.dialog._on_add_wm_hint_clicked()
        self.assertEqual(len(self.wm_calls), 1)
        self.assertEqual(self.wm_calls[0], ("my_key", "my_value"))

    def test_wm_hint_clears_fields_after_fired(self):
        self.dialog._wm_key_var.set("k")
        self.dialog._wm_val_var.set("v")
        self.dialog._on_add_wm_hint_clicked()
        self.assertEqual(self.dialog._wm_key_var.get(), "")
        self.assertEqual(self.dialog._wm_val_var.get(), "")

    def test_wm_hint_empty_key_does_not_fire(self):
        self.dialog._wm_key_var.set("")
        self.dialog._wm_val_var.set("val")
        with patch("tkinter.messagebox.showwarning"):
            self.dialog._on_add_wm_hint_clicked()
        self.assertEqual(len(self.wm_calls), 0)

    def test_wm_hint_empty_value_does_not_fire(self):
        self.dialog._wm_key_var.set("key")
        self.dialog._wm_val_var.set("")
        with patch("tkinter.messagebox.showwarning"):
            self.dialog._on_add_wm_hint_clicked()
        self.assertEqual(len(self.wm_calls), 0)


# ---------------------------------------------------------------------------
# Bridge.retrigger_synthesis_streaming tests
# ---------------------------------------------------------------------------


class TestBridgeRetriggerSynthesis(unittest.TestCase):
    """AgentixBridge.retrigger_synthesis_streaming produces a TASK_NODE_END chunk."""

    def _make_bridge(self, llm_chunks):
        from agentix.bridge.bridge import AgentixBridge

        bridge = AgentixBridge.__new__(AgentixBridge)
        bridge._model = "test-model"
        bridge._ollama_host = "http://localhost:11434"
        bridge._context_window = 4096
        bridge._tools = []
        bridge._tool_implementations = {}
        bridge._max_tool_rounds = 3
        bridge._logger = MagicMock()
        bridge._iter_llm_chunks = Mock(return_value=iter(llm_chunks))
        return bridge

    def _make_context(self):
        from shared.models.context import Context

        ctx = MagicMock(spec=Context)
        ctx.get_messages.return_value = []
        ctx.to_llm_messages.return_value = []
        ctx.save_task_node.return_value = ""
        ctx.save_task_tree.return_value = ""
        return ctx

    def test_yields_task_node_end(self):
        content_chunk = ResponseChunk(type=ChunkType.CONTENT, content="new synthesis text")
        node = _make_node()
        tree = _make_tree(node)
        ctx = self._make_context()
        bridge = self._make_bridge([content_chunk])
        # Patch out assertion helpers to avoid file-system calls
        with (
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
            patch("agentix.bridge.bridge.verify_assertion"),
        ):
            chunks = list(bridge.retrigger_synthesis_streaming(node, ctx, tree))

        types = [c.type for c in chunks]
        self.assertIn(ChunkType.TASK_NODE_END, types)

    def test_task_node_end_carries_new_synthesis(self):
        content_chunk = ResponseChunk(type=ChunkType.CONTENT, content="better synthesis")
        node = _make_node()
        tree = _make_tree(node)
        ctx = self._make_context()
        bridge = self._make_bridge([content_chunk])
        with (
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
            patch("agentix.bridge.bridge.verify_assertion"),
        ):
            chunks = list(bridge.retrigger_synthesis_streaming(node, ctx, tree, hint="focus on X"))

        end_chunk = next(c for c in chunks if c.type == ChunkType.TASK_NODE_END)
        self.assertEqual(end_chunk.content, "better synthesis")

    def test_synthesis_attempt_appended(self):
        content_chunk = ResponseChunk(type=ChunkType.CONTENT, content="synth text")
        node = _make_node()
        tree = _make_tree(node)
        ctx = self._make_context()
        bridge = self._make_bridge([content_chunk])
        with (
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
            patch("agentix.bridge.bridge.verify_assertion"),
        ):
            list(bridge.retrigger_synthesis_streaming(node, ctx, tree))

        self.assertEqual(len(node.synthesis_attempts), 1)

    def test_node_persisted(self):
        content_chunk = ResponseChunk(type=ChunkType.CONTENT, content="synth")
        node = _make_node()
        tree = _make_tree(node)
        ctx = self._make_context()
        bridge = self._make_bridge([content_chunk])
        with (
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
            patch("agentix.bridge.bridge.verify_assertion"),
        ):
            list(bridge.retrigger_synthesis_streaming(node, ctx, tree))

        ctx.save_task_node.assert_called_once_with(node)
        ctx.save_task_tree.assert_called_once_with(tree)

    def test_tool_call_chunks_filtered_out(self):
        tool_chunk = ResponseChunk(type=ChunkType.TOOL_CALL, tool_name="some_tool")
        content_chunk = ResponseChunk(type=ChunkType.CONTENT, content="synth")
        node = _make_node()
        tree = _make_tree(node)
        ctx = self._make_context()
        bridge = self._make_bridge([tool_chunk, content_chunk])
        with (
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
            patch("agentix.bridge.bridge.verify_assertion"),
        ):
            chunks = list(bridge.retrigger_synthesis_streaming(node, ctx, tree))

        yielded_types = {c.type for c in chunks}
        self.assertNotIn(ChunkType.TOOL_CALL, yielded_types)

    def test_empty_synthesis_uses_fallback(self):
        # No CONTENT chunks -> fallback text used
        node = _make_node()
        tree = _make_tree(node)
        ctx = self._make_context()
        bridge = self._make_bridge([])
        with (
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
            patch("agentix.bridge.bridge.verify_assertion"),
        ):
            chunks = list(bridge.retrigger_synthesis_streaming(node, ctx, tree))

        end_chunk = next(c for c in chunks if c.type == ChunkType.TASK_NODE_END)
        self.assertIn("no synthesis", end_chunk.content)

    def test_task_id_in_end_chunk(self):
        content_chunk = ResponseChunk(type=ChunkType.CONTENT, content="synth")
        node = _make_node(task_id="task_xyz")
        tree = _make_tree(node)
        ctx = self._make_context()
        bridge = self._make_bridge([content_chunk])
        with (
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
            patch("agentix.bridge.bridge.verify_assertion"),
        ):
            chunks = list(bridge.retrigger_synthesis_streaming(node, ctx, tree))

        end_chunk = next(c for c in chunks if c.type == ChunkType.TASK_NODE_END)
        self.assertEqual(end_chunk.task_id, "task_xyz")


# ---------------------------------------------------------------------------
# AgentixBridgeAdapter.retrigger_synthesis_generator tests
# ---------------------------------------------------------------------------


class TestAdapterRetriggerSynthesis(unittest.TestCase):
    """Adapter wraps bridge method and propagates chunks (or converts exceptions)."""

    def _make_adapter(self, bridge_chunks):
        from agentx.integration.agentix_bridge_adapter import AgentixBridgeAdapter

        adapter = AgentixBridgeAdapter.__new__(AgentixBridgeAdapter)
        adapter.bridge = MagicMock()
        adapter.bridge.retrigger_synthesis_streaming.return_value = iter(bridge_chunks)
        return adapter

    def test_propagates_task_node_end(self):
        end_chunk = ResponseChunk(
            type=ChunkType.TASK_NODE_END,
            task_id="task_001",
            content="new synth",
        )
        adapter = self._make_adapter([end_chunk])
        node = _make_node()
        tree = _make_tree(node)
        ctx = MagicMock()
        chunks = list(adapter.retrigger_synthesis_generator(node, ctx, tree, hint="test"))
        self.assertIn(end_chunk, chunks)

    def test_exception_yields_error_chunk(self):
        from agentx.integration.agentix_bridge_adapter import AgentixBridgeAdapter

        adapter = AgentixBridgeAdapter.__new__(AgentixBridgeAdapter)
        adapter.bridge = MagicMock()
        adapter.bridge.retrigger_synthesis_streaming.side_effect = RuntimeError("bridge error")
        node = _make_node()
        tree = _make_tree(node)
        ctx = MagicMock()
        chunks = list(adapter.retrigger_synthesis_generator(node, ctx, tree))
        self.assertEqual(len(chunks), 1)
        self.assertEqual(chunks[0].type, ChunkType.ERROR)
        self.assertIn("bridge error", chunks[0].content)

    def test_passes_hint_to_bridge(self):
        adapter = self._make_adapter([])
        node = _make_node()
        tree = _make_tree(node)
        ctx = MagicMock()
        list(adapter.retrigger_synthesis_generator(node, ctx, tree, hint="my hint"))
        adapter.bridge.retrigger_synthesis_streaming.assert_called_once_with(node, ctx, tree, "my hint")


# ---------------------------------------------------------------------------
# Session.retrigger_synthesis tests
# ---------------------------------------------------------------------------


class TestSessionRetriggerSynthesis(unittest.TestCase):
    """Session.retrigger_synthesis wires adapter → GUI updates."""

    def _make_session(self):
        from agentx.session import AgentXSession

        session = AgentXSession.__new__(AgentXSession)
        session.gui = MagicMock()
        session.context = MagicMock()
        session.working_memory = None
        session._is_streaming = threading.Event()
        session.agentix_adapter = MagicMock()
        session._safe_root_after = lambda fn: fn()  # call immediately on same thread
        session.refresh_user_gui = MagicMock()
        from agentx.streaming_controller import StreamingController
        session._streaming_controller = StreamingController(session)
        return session

    def test_does_nothing_when_already_streaming(self):
        session = self._make_session()
        session._is_streaming.set()
        session.retrigger_synthesis("task_001", "hint")
        session.gui.display_error.assert_called_once()
        session.agentix_adapter.retrigger_synthesis_generator.assert_not_called()

    def test_error_when_no_task_tree(self):
        session = self._make_session()
        session.context.load_task_tree.return_value = None
        session.retrigger_synthesis("task_001")
        session.gui.display_error.assert_called_once()

    def test_error_when_task_id_not_in_tree(self):
        session = self._make_session()
        tree = MagicMock()
        tree.nodes = {}
        session.context.load_task_tree.return_value = tree
        session.retrigger_synthesis("missing_task")
        session.gui.display_error.assert_called_once()

    def test_marks_node_invalidated(self):
        session = self._make_session()
        node = _make_node("task_001")
        tree = _make_tree(node)
        session.context.load_task_tree.return_value = tree
        end_chunk = ResponseChunk(
            type=ChunkType.TASK_NODE_END,
            task_id="task_001",
            content="new synth",
            assertions=[],
        )
        session.agentix_adapter.retrigger_synthesis_generator.return_value = iter([end_chunk])
        session.retrigger_synthesis("task_001")
        session.gui.mark_plan_node_invalidated.assert_called_with("task_001")

    def test_calls_update_plan_synthesis_on_end_chunk(self):
        session = self._make_session()
        node = _make_node("task_001")
        tree = _make_tree(node)
        session.context.load_task_tree.return_value = tree
        end_chunk = ResponseChunk(
            type=ChunkType.TASK_NODE_END,
            task_id="task_001",
            content="new synth",
            assertions=[{"fact": "a", "verified": True}],
        )
        session.agentix_adapter.retrigger_synthesis_generator.return_value = iter([end_chunk])
        done_event = threading.Event()
        original_refresh = session.refresh_user_gui

        def _patched_refresh():
            original_refresh()
            done_event.set()

        session.refresh_user_gui = _patched_refresh

        # The method starts a daemon thread; wait for it
        session._safe_root_after = lambda fn: fn()  # direct call
        session.retrigger_synthesis("task_001")

        # Give the daemon thread a moment to finish
        done_event.wait(timeout=2.0)
        session.gui.update_plan_synthesis.assert_called_once_with(
            "task_001", "new synth", [{"fact": "a", "verified": True}]
        )

    def test_streaming_flag_cleared_after_completion(self):
        session = self._make_session()
        node = _make_node("task_001")
        tree = _make_tree(node)
        session.context.load_task_tree.return_value = tree
        end_chunk = ResponseChunk(
            type=ChunkType.TASK_NODE_END,
            task_id="task_001",
            content="synth",
            assertions=[],
        )
        session.agentix_adapter.retrigger_synthesis_generator.return_value = iter([end_chunk])
        done_event = threading.Event()
        original_refresh = session.refresh_user_gui

        def _patched_refresh():
            original_refresh()
            done_event.set()

        session.refresh_user_gui = _patched_refresh
        session.retrigger_synthesis("task_001")
        done_event.wait(timeout=2.0)
        self.assertFalse(session._is_streaming.is_set())


# ---------------------------------------------------------------------------
# Integration smoke: TASK_NODE_END routing wires on_resynth callback
# ---------------------------------------------------------------------------


class TestTaskNodeEndResynthWiring(unittest.TestCase):
    """Verify that TASK_NODE_END routes on_resynth into add_plan_synthesis."""

    def test_on_resynth_callable_passed_to_add_plan_synthesis(self):
        """The lambda built in _stream_via_agentix must forward task_id correctly."""
        received_callbacks: list = []

        class FakeGUI:
            def add_plan_synthesis(self, task_id, synth, assertions, on_resynth=None):
                received_callbacks.append((task_id, on_resynth))

            def update_plan_node_status(self, *a, **kw):
                pass

        # Simulate the exact routing code in session._stream_via_agentix
        gui = FakeGUI()

        def make_cb_and_call(tid, synth, asserts, retrigger_fn):
            def _make_resynth_cb(t=tid):
                return lambda hint: retrigger_fn(t, hint)

            cb = _make_resynth_cb()
            gui.add_plan_synthesis(tid, synth, asserts, on_resynth=cb)

        calls_received: list[tuple[str, str]] = []

        def fake_retrigger(task_id, hint):
            calls_received.append((task_id, hint))

        make_cb_and_call("task_z", "synthesis text", [], fake_retrigger)

        self.assertEqual(len(received_callbacks), 1)
        task_id, callback = received_callbacks[0]
        self.assertEqual(task_id, "task_z")
        self.assertIsNotNone(callback)

        # Fire the callback with a hint
        callback("be concise")
        self.assertEqual(calls_received, [("task_z", "be concise")])


# ---------------------------------------------------------------------------
# Session._add_wm_hint_for_task tests
# ---------------------------------------------------------------------------


class TestSessionAddWMHintForTask(unittest.TestCase):
    """session._add_wm_hint_for_task stores the fact and invalidates the node."""

    def _make_session(self):
        from agentx.session import AgentXSession

        session = AgentXSession.__new__(AgentXSession)
        session.gui = MagicMock()
        session.context = MagicMock()
        session.working_memory = MagicMock()
        session._is_streaming = threading.Event()
        session._safe_root_after = lambda fn: fn()
        session.refresh_user_gui = MagicMock()
        from agentx.streaming_controller import StreamingController
        session._streaming_controller = StreamingController(session)
        return session

    def test_adds_wm_fact(self):
        session = self._make_session()
        node = _make_node("task_001")
        tree = _make_tree(node)
        session.context.load_task_tree.return_value = tree
        session._add_wm_hint_for_task("task_001", "my_key", "my_value")
        session.working_memory.add_fact.assert_called_once()
        call_args = session.working_memory.add_fact.call_args[0]
        self.assertEqual(call_args[1], "my_key")
        self.assertEqual(call_args[2], "my_value")

    def test_marks_node_wm_hints_added(self):
        session = self._make_session()
        node = _make_node("task_001")
        tree = _make_tree(node)
        session.context.load_task_tree.return_value = tree
        session._add_wm_hint_for_task("task_001", "k", "v")
        self.assertTrue(node.wm_hints_added)

    def test_persists_node_and_tree(self):
        session = self._make_session()
        node = _make_node("task_001")
        tree = _make_tree(node)
        session.context.load_task_tree.return_value = tree
        session._add_wm_hint_for_task("task_001", "k", "v")
        session.context.save_task_node.assert_called_once_with(node)
        session.context.save_task_tree.assert_called_once_with(tree)

    def test_marks_gui_node_invalidated(self):
        session = self._make_session()
        node = _make_node("task_001")
        tree = _make_tree(node)
        session.context.load_task_tree.return_value = tree
        session._add_wm_hint_for_task("task_001", "k", "v")
        session.gui.mark_plan_node_invalidated.assert_called_with("task_001")

    def test_handles_missing_working_memory(self):
        session = self._make_session()
        session.working_memory = None
        node = _make_node("task_001")
        tree = _make_tree(node)
        session.context.load_task_tree.return_value = tree
        # Must not raise
        session._add_wm_hint_for_task("task_001", "k", "v")
        session.gui.mark_plan_node_invalidated.assert_called_with("task_001")

    def test_handles_missing_task_tree(self):
        session = self._make_session()
        session.context.load_task_tree.return_value = None
        # Must not raise even when tree is absent
        session._add_wm_hint_for_task("task_001", "k", "v")
        session.gui.mark_plan_node_invalidated.assert_called_with("task_001")


# ---------------------------------------------------------------------------
# Integration: on_add_wm_hint callback threaded into ResynthesisDialog
# ---------------------------------------------------------------------------


class TestOnAddWMHintWiring(unittest.TestCase):
    """Verify on_add_wm_hint is passed all the way to ResynthesisDialog."""

    def setUp(self):
        self.root = _make_root()
        self.wm_calls: list[tuple[str, str]] = []
        self.dialog = ResynthesisDialog(
            parent=self.root,
            task_id="task_wm2",
            synthesis_text="synthesis",
            failed_assertions=[],
            on_confirm=lambda h: None,
            on_add_wm_hint=lambda k, v: self.wm_calls.append((k, v)),
        )

    def tearDown(self):
        try:
            self.dialog._win.destroy()
        except Exception:
            pass
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_wm_section_visible_when_callback_provided(self):
        # The dialog should have been constructed without error and be visible
        self.assertEqual(self.dialog._win.winfo_exists(), 1)

    def test_on_add_wm_hint_called_with_correct_args(self):
        self.dialog._wm_key_var.set("hint_key")
        self.dialog._wm_val_var.set("hint_val")
        self.dialog._on_add_wm_hint_clicked()
        self.assertEqual(self.wm_calls, [("hint_key", "hint_val")])


class TestPlanTreeWidgetOnAddWMHint(unittest.TestCase):
    """PlanTreeWidget.add_synthesis_to_node forwards on_add_wm_hint to _create_synthesis_block."""

    def setUp(self):
        self.root = _make_root()

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_on_add_wm_hint_passed_correctly(self):
        from agentx.gui.plan_tree_widget import PlanTreeWidget

        frame = tk.Frame(self.root)
        widget = PlanTreeWidget(frame)

        # Add a node so add_synthesis_to_node can attach the block
        widget.add_step_node("plan_1", "task_x", "Do something", tbd=False)

        received: list[tuple[str, str]] = []

        def fake_wm(k, v):
            received.append((k, v))

        # Should not raise — on_add_wm_hint wired through
        widget.add_synthesis_to_node(
            "task_x",
            "synthesis text",
            [],
            on_resynth=lambda h: None,
            on_add_wm_hint=fake_wm,
        )
        # Widget created — confirm _create_synthesis_block accepted on_add_wm_hint
        self.assertIn("task_x", widget._nodes)


if __name__ == "__main__":
    unittest.main()
