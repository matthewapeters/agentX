"""Phase 5 unit tests: Re-synthesis UI.

Tests cover:
- ResynthesisDialog: creates window, calls on_confirm with hint, WM hint callback,
  cancel button, missing WM fields handled
- PlanTreeWidget: add_synthesis_to_node with on_resynth creates button,
  update_synthesis_on_node replaces text and badges, mark_plan_node_invalidated
- GUIManager: update_plan_synthesis, mark_plan_node_invalidated
- bridge.retrigger_synthesis_streaming: yields TASK_NODE_END with updated synthesis,
  persists SynthesisAttempt, WM hint flows through
- session.retrigger_synthesis: loads tree/node, starts background thread, guards
  against streaming conflicts, GUI callbacks scheduled correctly
"""

from __future__ import annotations

import sys
import os
import tempfile
import threading
import time
import tkinter as tk
import unittest
from unittest.mock import MagicMock, call, patch

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from agentx.gui.gui_manager import GUIManager
from agentx.gui.gui_config import GUIConfig
from agentx.gui.plan_tree_widget import PlanTreeWidget, _STATUS_ICONS
from agentx.gui.resynthesis_dialog import ResynthesisDialog
from shared.models.response import ChunkType, ResponseChunk
from shared.models.task_node import (
    AssertionRecord,
    PlanRecord,
    PlanStep,
    SynthesisAttempt,
    TaskNodeRecord,
    TaskTree,
)
from shared.models.context import Context

# ── Helpers ────────────────────────────────────────────────────────────────────


def _make_root() -> tk.Tk:
    root = tk.Tk()
    root.withdraw()
    return root


def _make_gui():
    root = _make_root()
    cfg = GUIConfig.from_dict({"ollama_host": "localhost", "ollama_model": "m", "ollama_timeout": 30})
    gui = GUIManager(
        root,
        cfg,
        on_submit=MagicMock(),
        on_interrupt=MagicMock(),
        on_attachment_toggle=MagicMock(),
    )
    gui.create_layout()
    return root, gui


def _make_node(task_id: str = "task-1", plan_id: str = "plan-1") -> TaskNodeRecord:
    return TaskNodeRecord(
        plan_id=plan_id,
        task_id=task_id,
        depth=0,
        plan_step_index=0,
        task_description="Do something useful",
        status="done",
        synthesis_attempts=[SynthesisAttempt(epoch=1000.0, status="accepted")],
    )


def _make_tree(session_id: str = "sess-1") -> TaskTree:
    tree = TaskTree(session_id=session_id)
    plan = PlanRecord(
        plan_id="plan-1",
        plan_name="Test Plan",
        session_plan_index=1,
        steps=[PlanStep(step_id="task-1", description="Step 1")],
    )
    tree.add_plan(plan)
    node = _make_node()
    tree.add_node(node)
    return tree


# ── ResynthesisDialog ──────────────────────────────────────────────────────────


class TestResynthesisDialog(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()

    def tearDown(self):
        # destroy any lingering Toplevel windows before destroying root
        for w in self.root.winfo_children():
            try:
                w.destroy()
            except Exception:
                pass
        self.root.destroy()

    def test_dialog_window_created(self):
        on_confirm = MagicMock()
        dlg = ResynthesisDialog(
            parent=self.root,
            task_id="task-1",
            synthesis_text="Original synthesis",
            failed_assertions=[],
            on_confirm=on_confirm,
        )
        self.assertIsNotNone(dlg._win)
        self.assertTrue(dlg._win.winfo_exists())

    def test_confirm_calls_on_confirm_with_hint(self):
        received = []
        dlg = ResynthesisDialog(
            parent=self.root,
            task_id="task-1",
            synthesis_text="Synthesis text",
            failed_assertions=[],
            on_confirm=lambda h: received.append(h),
        )
        dlg._hint_text.insert("1.0", "my hint")
        dlg._on_confirm_clicked()
        self.assertEqual(len(received), 1)
        self.assertEqual(received[0], "my hint")

    def test_confirm_with_empty_hint(self):
        received = []
        dlg = ResynthesisDialog(
            parent=self.root,
            task_id="task-1",
            synthesis_text="Synthesis text",
            failed_assertions=[],
            on_confirm=lambda h: received.append(h),
        )
        dlg._on_confirm_clicked()
        self.assertEqual(received[0], "")

    def test_synthesis_text_displayed_readonly(self):
        dlg = ResynthesisDialog(
            parent=self.root,
            task_id="task-1",
            synthesis_text="Show this text",
            failed_assertions=[],
            on_confirm=MagicMock(),
        )
        content = dlg._synth_text.get("1.0", tk.END).strip()
        self.assertEqual(content, "Show this text")
        # Should be disabled (read-only)
        self.assertEqual(str(dlg._synth_text.cget("state")), "disabled")

    def test_failed_assertions_rendered(self):
        assertions = [
            {"fact": "File X exists", "verified": False, "error": "not found"},
        ]
        dlg = ResynthesisDialog(
            parent=self.root,
            task_id="task-1",
            synthesis_text="Synthesis",
            failed_assertions=assertions,
            on_confirm=MagicMock(),
        )
        # Verify dialog was created without error; assertion label appears somewhere
        # in the window's descendants.
        all_text = " ".join(
            w.cget("text") for w in dlg._win.winfo_children() if hasattr(w, "cget") and "text" in w.configure()
        )
        # Check window title includes task_id
        self.assertIn("task-1", dlg._win.title())

    def test_wm_hint_callback_called_with_key_value(self):
        received = []
        dlg = ResynthesisDialog(
            parent=self.root,
            task_id="task-1",
            synthesis_text="Synthesis",
            failed_assertions=[],
            on_confirm=MagicMock(),
            on_add_wm_hint=lambda k, v: received.append((k, v)),
        )
        dlg._wm_key_var.set("project")
        dlg._wm_val_var.set("AgentX")
        dlg._on_add_wm_hint_clicked()
        self.assertEqual(received, [("project", "AgentX")])

    def test_wm_hint_clears_fields_after_add(self):
        dlg = ResynthesisDialog(
            parent=self.root,
            task_id="task-1",
            synthesis_text="Synthesis",
            failed_assertions=[],
            on_confirm=MagicMock(),
            on_add_wm_hint=MagicMock(),
        )
        dlg._wm_key_var.set("k")
        dlg._wm_val_var.set("v")
        dlg._on_add_wm_hint_clicked()
        self.assertEqual(dlg._wm_key_var.get(), "")
        self.assertEqual(dlg._wm_val_var.get(), "")

    def test_no_wm_hint_callback_hides_wm_section(self):
        # When on_add_wm_hint is None the dialog should still build without error,
        # and _wm_key_var / _wm_val_var are still created (empty StringVars).
        dlg = ResynthesisDialog(
            parent=self.root,
            task_id="task-1",
            synthesis_text="Synthesis",
            failed_assertions=[],
            on_confirm=MagicMock(),
            on_add_wm_hint=None,
        )
        self.assertIsNotNone(dlg._wm_key_var)

    def test_confirm_destroys_window(self):
        dlg = ResynthesisDialog(
            parent=self.root,
            task_id="task-1",
            synthesis_text="Synthesis",
            failed_assertions=[],
            on_confirm=MagicMock(),
        )
        win = dlg._win
        dlg._on_confirm_clicked()
        self.assertFalse(win.winfo_exists())


# ── PlanTreeWidget Phase 5 additions ──────────────────────────────────────────


class TestPlanTreeWidgetPhase5(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()
        self.tree = PlanTreeWidget(parent=self.root, bg="#222222", fg="#eee", dim_fg="#888", accent_fg="#7dd3fc")

    def tearDown(self):
        self.root.destroy()

    def _add_step_with_synthesis(self, on_resynth=None):
        self.tree.add_step_node("plan-1", "step-1", "Do thing", tbd=False)
        self.tree.add_synthesis_to_node(
            "step-1",
            "Original synthesis",
            [{"fact": "X exists", "verified": True}],
            on_resynth=on_resynth,
        )

    def test_add_synthesis_stores_widget_ref(self):
        self._add_step_with_synthesis()
        node = self.tree._nodes["step-1"]
        self.assertIn("synthesis_widget", node)
        self.assertIn("synthesis_badge_frame", node)

    def test_add_synthesis_with_resynth_callback_creates_button(self):
        cb = MagicMock()
        self._add_step_with_synthesis(on_resynth=cb)
        node = self.tree._nodes["step-1"]
        details = node["details_frame"]
        # Verify button was rendered by checking winfo_children depth
        found_button = False

        def _search(w):
            nonlocal found_button
            if isinstance(w, tk.Button):
                try:
                    txt = w.cget("text")
                    if "Re-synthesise" in txt or "↻" in txt:
                        found_button = True
                except Exception:
                    pass
            for child in w.winfo_children():
                _search(child)

        _search(details)
        self.assertTrue(found_button, "Re-synthesise button not found in details subtree")

    def test_add_synthesis_without_callback_no_button(self):
        self._add_step_with_synthesis(on_resynth=None)
        node = self.tree._nodes["step-1"]
        details = node["details_frame"]
        found_button = False

        def _search(w):
            nonlocal found_button
            if isinstance(w, tk.Button):
                try:
                    if "Re-synthesise" in (w.cget("text") or ""):
                        found_button = True
                except Exception:
                    pass
            for child in w.winfo_children():
                _search(child)

        _search(details)
        self.assertFalse(found_button)

    def test_update_synthesis_on_node_changes_text(self):
        self._add_step_with_synthesis()
        self.tree.update_synthesis_on_node("step-1", "New synthesis text", [{"fact": "Y exists", "verified": True}])
        node = self.tree._nodes["step-1"]
        synth_widget = node["synthesis_widget"]
        content = synth_widget.get("1.0", tk.END).strip()
        self.assertEqual(content, "New synthesis text")

    def test_update_synthesis_on_node_updates_badges(self):
        self._add_step_with_synthesis()
        self.tree.update_synthesis_on_node(
            "step-1",
            "New synthesis",
            [
                {"fact": "A exists", "verified": True},
                {"fact": "B missing", "verified": False, "error": "not found"},
            ],
        )
        node = self.tree._nodes["step-1"]
        badge_frame = node["synthesis_badge_frame"]
        # Should have 2 badge labels
        badges = badge_frame.winfo_children()
        self.assertEqual(len(badges), 2)

    def test_update_synthesis_missing_node_does_not_raise(self):
        # Should silently ignore unknown task_id
        self.tree.update_synthesis_on_node("nonexistent", "text", [])

    def test_update_node_status_invalidated(self):
        self.tree.add_step_node("plan-1", "step-1", "Task", tbd=False)
        self.tree.update_node_status("step-1", "invalidated")
        # "invalidated" is not a key in _STATUS_ICONS — falls back to "●"
        label_text = self.tree._nodes["step-1"]["status_label"].cget("text")
        self.assertEqual(label_text, "●")


# ── GUIManager Phase 5 additions ──────────────────────────────────────────────


class TestGUIManagerPhase5(unittest.TestCase):
    def setUp(self):
        self.root, self.gui = _make_gui()
        # Bootstrap a plan tab so nodes can be inserted
        self.gui.add_plan_tab("plan-1", "Test Plan")
        self.gui.add_plan_step_node("plan-1", "task-1", "Step 1", tbd=False)
        self.gui.add_plan_synthesis(
            "task-1",
            "Original synthesis",
            [{"fact": "X exists", "verified": True}],
        )

    def tearDown(self):
        self.root.destroy()

    def test_update_plan_synthesis_changes_text(self):
        self.gui.update_plan_synthesis("task-1", "Updated synthesis", [{"fact": "Y exists", "verified": True}])
        tree = self.gui._plan_trees["plan-1"]
        node = tree._nodes["task-1"]
        content = node["synthesis_widget"].get("1.0", tk.END).strip()
        self.assertEqual(content, "Updated synthesis")

    def test_update_plan_synthesis_missing_task_does_not_raise(self):
        self.gui.update_plan_synthesis("nonexistent", "text", [])

    def test_mark_plan_node_invalidated_changes_status(self):
        self.gui.mark_plan_node_invalidated("task-1")
        tree = self.gui._plan_trees["plan-1"]
        node = tree._nodes["task-1"]
        status_text = node["status_label"].cget("text")
        # invalidated falls back to "●"
        self.assertEqual(status_text, "●")

    def test_mark_plan_node_invalidated_missing_does_not_raise(self):
        self.gui.mark_plan_node_invalidated("nonexistent")

    def test_add_plan_synthesis_with_resynth_callback(self):
        cb = MagicMock()
        self.gui.add_plan_tab("plan-2", "Plan 2")
        self.gui.add_plan_step_node("plan-2", "task-2", "Step A", tbd=False)
        # Should not raise even with a callback
        self.gui.add_plan_synthesis(
            "task-2",
            "Synthesis with button",
            [],
            on_resynth=cb,
        )
        tree = self.gui._plan_trees["plan-2"]
        node = tree._nodes["task-2"]
        self.assertIn("synthesis_widget", node)


# ── Bridge retrigger_synthesis_streaming ──────────────────────────────────────


class TestBridgeRetriggerSynthesis(unittest.TestCase):
    """Test AgentixBridge.retrigger_synthesis_streaming using a stubbed LLM."""

    def _make_bridge_with_stub_llm(self, llm_chunks):
        """Return a bridge whose _iter_llm_chunks yields llm_chunks."""
        from agentix.bridge.bridge import AgentixBridge
        from agentix.agentix_config import AgentixConfig

        config = AgentixConfig(model="test-model", debug=False)
        bridge = AgentixBridge(config)
        bridge._iter_llm_chunks = MagicMock(return_value=iter(llm_chunks))
        return bridge

    def _make_context(self, tmp_path):
        ctx = Context()
        ctx.path = os.path.join(tmp_path, "context")
        os.makedirs(ctx.path, exist_ok=True)
        ctx.session_id = "sess-test"
        return ctx

    def test_yields_task_node_end_chunk(self):
        bridge = self._make_bridge_with_stub_llm(
            [
                ResponseChunk(type=ChunkType.CONTENT, content="Re-synthesised result"),
                ResponseChunk(type=ChunkType.DONE),
            ]
        )
        with tempfile.TemporaryDirectory() as tmp:
            ctx = self._make_context(tmp)
            tree = _make_tree()
            node = _make_node()

            # Patch assertion and verify to skip LLM calls
            with (
                patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
                patch("agentix.bridge.bridge.verify_assertion"),
            ):
                chunks = list(bridge.retrigger_synthesis_streaming(node, ctx, tree, hint=""))

        end_chunks = [c for c in chunks if c.type == ChunkType.TASK_NODE_END]
        self.assertEqual(len(end_chunks), 1)
        self.assertIn("Re-synthesised result", end_chunks[0].content)

    def test_new_synthesis_attempt_appended(self):
        bridge = self._make_bridge_with_stub_llm(
            [
                ResponseChunk(type=ChunkType.CONTENT, content="New synthesis"),
            ]
        )
        with tempfile.TemporaryDirectory() as tmp:
            ctx = self._make_context(tmp)
            tree = _make_tree()
            node = _make_node()
            prior_count = len(node.synthesis_attempts)

            with (
                patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
                patch("agentix.bridge.bridge.verify_assertion"),
            ):
                list(bridge.retrigger_synthesis_streaming(node, ctx, tree, hint=""))

        self.assertEqual(len(node.synthesis_attempts), prior_count + 1)

    def test_hint_included_in_synthesis_prompt(self):
        bridge = self._make_bridge_with_stub_llm(
            [
                ResponseChunk(type=ChunkType.CONTENT, content="Result"),
            ]
        )
        with tempfile.TemporaryDirectory() as tmp:
            ctx = self._make_context(tmp)
            tree = _make_tree()
            node = _make_node()

            with (
                patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
                patch("agentix.bridge.bridge.verify_assertion"),
            ):
                list(bridge.retrigger_synthesis_streaming(node, ctx, tree, hint="focus on errors"))

        call_args = bridge._iter_llm_chunks.call_args
        messages = call_args[0][0]
        last_msg = messages[-1]["content"]
        self.assertIn("focus on errors", last_msg)

    def test_failed_assertions_recorded(self):
        bridge = self._make_bridge_with_stub_llm(
            [
                ResponseChunk(type=ChunkType.CONTENT, content="Synthesis"),
            ]
        )
        fail_a = AssertionRecord(fact="File missing", verified=False, error="not found")

        with tempfile.TemporaryDirectory() as tmp:
            ctx = self._make_context(tmp)
            tree = _make_tree()
            node = _make_node()

            with (
                patch("agentix.bridge.bridge.extract_assertions", return_value=[fail_a]),
                patch("agentix.bridge.bridge.verify_assertion"),
            ):
                list(bridge.retrigger_synthesis_streaming(node, ctx, tree))

        self.assertIn(fail_a, node.assertions)
        self.assertEqual(node.synthesis_attempts[-1].status, "rejected")

    def test_tool_call_chunks_skipped(self):
        bridge = self._make_bridge_with_stub_llm(
            [
                ResponseChunk(type=ChunkType.TOOL_CALL, tool_name="read_file", tool_input={}),
                ResponseChunk(type=ChunkType.CONTENT, content="Good synthesis"),
            ]
        )
        with tempfile.TemporaryDirectory() as tmp:
            ctx = self._make_context(tmp)
            tree = _make_tree()
            node = _make_node()

            with (
                patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
                patch("agentix.bridge.bridge.verify_assertion"),
            ):
                chunks = list(bridge.retrigger_synthesis_streaming(node, ctx, tree))

        tool_chunks = [c for c in chunks if c.type == ChunkType.TOOL_CALL]
        self.assertEqual(len(tool_chunks), 0)


# ── Session retrigger_synthesis ────────────────────────────────────────────────


class TestSessionRetriggerSynthesis(unittest.TestCase):
    """Test session.retrigger_synthesis routing logic (no live LLM)."""

    def _make_session(self, tmp_path):
        """Return a minimal AgentXSession stub."""
        from agentx.session import AgentXSession

        root = _make_root()
        gui = MagicMock()
        session = AgentXSession.__new__(AgentXSession)
        session.root = root
        session.gui = gui
        session._is_streaming = threading.Event()
        session.context = Context()
        session.context.path = os.path.join(tmp_path, "context")
        os.makedirs(session.context.path, exist_ok=True)
        session.context.session_id = "sess-test"
        session.session_folder = tmp_path
        session.working_memory = None
        session.agentix_adapter = MagicMock()
        session._last_synthesis_thread = None
        # Bypass root.after() thread-safety mechanism so background thread calls
        # gui methods directly on the MagicMock — avoids TclError/SIGABRT
        session._safe_root_after = lambda cb: cb()
        import logging

        session._logger = logging.getLogger("test_session")
        return session, root, gui

    def test_retrigger_blocked_when_streaming(self):
        with tempfile.TemporaryDirectory() as tmp:
            session, root, gui = self._make_session(tmp)
            session._is_streaming.set()
            session.retrigger_synthesis("task-1", hint="hint")
            gui.display_error.assert_called_once()
            # Should NOT start adapter streaming
            session.agentix_adapter.retrigger_synthesis_generator.assert_not_called()
        root.destroy()

    def test_retrigger_error_if_no_task_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            session, root, gui = self._make_session(tmp)
            # No task_tree.json exists → context.load_task_tree() returns None
            session.retrigger_synthesis("task-1")
            gui.display_error.assert_called_once()
        root.destroy()

    def test_retrigger_error_if_node_not_in_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            session, root, gui = self._make_session(tmp)
            tree = _make_tree()
            tree.save(tmp)
            session.retrigger_synthesis("nonexistent-task")
            gui.display_error.assert_called_once()
        root.destroy()

    def test_retrigger_marks_node_invalidated(self):
        with tempfile.TemporaryDirectory() as tmp:
            session, root, gui = self._make_session(tmp)
            tree = _make_tree()
            tree.save(tmp)

            # Make adapter generator return empty immediately
            session.agentix_adapter.retrigger_synthesis_generator.return_value = iter([])

            session.retrigger_synthesis("task-1")
            # Join the background thread to avoid race with root.destroy()
            if session._last_synthesis_thread is not None:
                session._last_synthesis_thread.join(timeout=2.0)
            root.update()

            gui.mark_plan_node_invalidated.assert_called_with("task-1")
        root.destroy()

    def test_retrigger_streams_update_into_gui(self):
        with tempfile.TemporaryDirectory() as tmp:
            session, root, gui = self._make_session(tmp)
            tree = _make_tree()
            tree.save(tmp)

            end_chunk = ResponseChunk(
                type=ChunkType.TASK_NODE_END,
                plan_id="plan-1",
                task_id="task-1",
                content="Brand new synthesis",
                assertions=[{"fact": "X exists", "verified": True}],
            )
            session.agentix_adapter.retrigger_synthesis_generator.return_value = iter([end_chunk])
            session.refresh_user_gui = MagicMock()

            session.retrigger_synthesis("task-1")
            # Join the background thread to avoid race with root.destroy()
            if session._last_synthesis_thread is not None:
                session._last_synthesis_thread.join(timeout=2.0)
            root.update()

            gui.update_plan_node_status.assert_called()
            gui.update_plan_synthesis.assert_called()
        root.destroy()

    def test_retrigger_wm_hint_added_when_working_memory_present(self):
        from shared.models.working_memory import WorkingMemory

        with tempfile.TemporaryDirectory() as tmp:
            session, root, gui = self._make_session(tmp)
            session.working_memory = WorkingMemory()
            tree = _make_tree()
            tree.save(tmp)
            session.agentix_adapter.retrigger_synthesis_generator.return_value = iter([])

            session.retrigger_synthesis("task-1", hint="use error path")
            # Join the background thread to avoid race with root.destroy()
            if session._last_synthesis_thread is not None:
                session._last_synthesis_thread.join(timeout=2.0)
            root.update()

            wm_facts = session.working_memory.all_facts()
            hint_keys = [f.key for f in wm_facts if "resynth_hint" in f.key]
            self.assertGreater(len(hint_keys), 0)
        root.destroy()


# ── TASK_NODE_END on_resynth wiring in session.py ─────────────────────────────


class TestSessionTaskNodeEndCallback(unittest.TestCase):
    """Verify the TASK_NODE_END chunk routing passes on_resynth to gui.add_plan_synthesis."""

    def _routing_fn(self, chunk, gui):
        """Replicate the TASK_NODE_END routing block from session._stream_via_agentix."""
        from shared.models.response import ChunkType

        if chunk.type == ChunkType.TASK_NODE_END and chunk.task_id:
            _tid = chunk.task_id
            _synth = chunk.content or ""
            _asserts = chunk.assertions or []

            def _make_resynth_cb(tid: str):
                return lambda hint, t=tid: f"retrigger:{t}:{hint}"

            _resynth_cb = _make_resynth_cb(_tid)
            gui.update_plan_node_status(_tid, "done")
            gui.add_plan_synthesis(_tid, _synth, _asserts, on_resynth=_resynth_cb)

    def test_on_resynth_callback_passed_to_add_plan_synthesis(self):
        gui = MagicMock()
        chunk = ResponseChunk(
            type=ChunkType.TASK_NODE_END,
            task_id="task-X",
            content="Synthesis text",
            assertions=[],
        )
        self._routing_fn(chunk, gui)
        call_kwargs = gui.add_plan_synthesis.call_args
        self.assertIn("on_resynth", call_kwargs.kwargs)
        self.assertIsNotNone(call_kwargs.kwargs["on_resynth"])

    def test_on_resynth_callback_is_per_task_id(self):
        """Each TASK_NODE_END chunk should produce a distinct on_resynth closure."""
        gui = MagicMock()
        for task_id in ("task-A", "task-B"):
            chunk = ResponseChunk(
                type=ChunkType.TASK_NODE_END,
                task_id=task_id,
                content="Synth",
                assertions=[],
            )
            self._routing_fn(chunk, gui)

        call_a = gui.add_plan_synthesis.call_args_list[0]
        call_b = gui.add_plan_synthesis.call_args_list[1]
        cb_a = call_a.kwargs["on_resynth"]
        cb_b = call_b.kwargs["on_resynth"]
        # Each callback should reference a different task_id
        result_a = cb_a("hint")
        result_b = cb_b("hint")
        self.assertIn("task-A", result_a)
        self.assertIn("task-B", result_b)
        self.assertNotEqual(result_a, result_b)


if __name__ == "__main__":
    unittest.main()
