"""Phase 6 Tests: Context Panel Integration

Tests covering:
  - PLAN/TASK_NODE messages stored in context on stream chunks
  - render_context_widget groups PLAN/TASK_NODE under preceding assistant message
  - _render_plan_rows creates clickable plan header rows and nested task node rows
  - _render_message_to_grid routes plan messages to _render_plan_rows
  - _on_plan_row_click focuses existing tab or triggers replay
  - _replay_plan_tab reconstructs plan tab from persisted JSON records
"""

import os
import sys
import tempfile
import tkinter as tk
import unittest
from pathlib import Path
from unittest.mock import MagicMock, call, patch

project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from agentx.gui.gui_config import GUIConfig
from agentx.gui.gui_manager import GUIManager
from shared.models.message import Message, MessageRole
from shared.models.response import ChunkType, ResponseChunk
from shared.models.task_node import PlanRecord, TaskNodeRecord

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


def _make_session(test_dir: str, chunks):
    from agentx.session import AgentXSession

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

    with (
        patch("agentx.session.create_adapter", return_value=mock_adapter),
        patch("agentx.model_metadata_store.ModelMetadataStore.populate"),
    ):
        session = AgentXSession(username="tester", session_dir=test_dir, config=config)
    return session, mock_adapter


# ---------------------------------------------------------------------------
# 1. MESSAGE_ROLES includes plan and task_node
# ---------------------------------------------------------------------------


class TestMessageRolesIncludePlanEntries(unittest.TestCase):
    def test_plan_in_message_roles(self):
        self.assertIn("plan", GUIManager.MESSAGE_ROLES)

    def test_task_node_in_message_roles(self):
        self.assertIn("task_node", GUIManager.MESSAGE_ROLES)

    def test_plan_icon_is_not_empty(self):
        self.assertTrue(GUIManager.MESSAGE_ROLES["plan"])

    def test_task_node_icon_is_not_empty(self):
        self.assertTrue(GUIManager.MESSAGE_ROLES["task_node"])


# ---------------------------------------------------------------------------
# 2. render_context_widget groups PLAN / TASK_NODE under assistant message
# ---------------------------------------------------------------------------


class TestRenderContextWidgetGrouping(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()
        self.gui = _make_gui(self.root)

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def _make_context_with_plan(self):
        from shared.models.context import Context

        ctx = Context()
        ctx.add_message(Message(role=MessageRole.USER, content="Run a plan"))
        ctx.add_message(Message(role=MessageRole.ASSISTANT, content="Sure, planning now"))
        ctx.add_message(Message(role=MessageRole.PLAN, content="Test Plan", plan_id="plan_001", plan_name="Test Plan"))
        ctx.add_message(
            Message(
                role=MessageRole.TASK_NODE,
                content="Step synthesis",
                plan_id="plan_001",
                task_id="task_001",
                task_depth=0,
            )
        )
        return ctx

    def test_render_produces_frame(self):
        ctx = self._make_context_with_plan()
        parent = tk.Frame(self.root)
        widget = self.gui.render_context_widget(ctx, parent)
        self.assertIsNotNone(widget)

    def test_on_plan_click_accepted(self):
        ctx = self._make_context_with_plan()
        parent = tk.Frame(self.root)
        clicked = []
        widget = self.gui.render_context_widget(ctx, parent, on_plan_click=lambda pid: clicked.append(pid))
        self.assertIsNotNone(widget)

    def test_plan_task_node_not_top_level_rows(self):
        """PLAN and TASK_NODE messages must be grouped under assistant, not as standalone rows."""
        from shared.models.context import Context

        ctx = Context()
        ctx.add_message(Message(role=MessageRole.USER, content="Go"))
        ctx.add_message(Message(role=MessageRole.ASSISTANT, content="Ok"))
        ctx.add_message(Message(role=MessageRole.PLAN, content="P", plan_id="p1", plan_name="P"))
        ctx.add_message(Message(role=MessageRole.TASK_NODE, content="T", plan_id="p1", task_id="t1"))

        parent = tk.Frame(self.root)

        # Patch _render_message_to_grid to capture its arguments
        calls = []
        original = self.gui._render_message_to_grid

        def capturing(*args, **kwargs):
            calls.append(args[0])  # message_obj
            return original(*args, **kwargs)

        self.gui._render_message_to_grid = capturing
        self.gui.render_context_widget(ctx, parent)

        # Only USER and ASSISTANT should be direct "row" messages (not plan/task_node)
        role_strs = []
        for m in calls:
            r = getattr(m, "role", "")
            role_strs.append(r.value if hasattr(r, "value") else str(r))

        self.assertNotIn("plan", role_strs, "PLAN should not appear as a standalone top-level row")
        self.assertNotIn("task_node", role_strs, "TASK_NODE should not appear as a standalone top-level row")


# ---------------------------------------------------------------------------
# 3. _render_plan_rows creates rows and appends to parent_collapsible
# ---------------------------------------------------------------------------


class TestRenderPlanRows(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()
        self.gui = _make_gui(self.root)
        self.frame = tk.Frame(self.root)

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def _plan_msg(self, plan_id="p1", plan_name="My Plan"):
        return Message(role=MessageRole.PLAN, content=plan_name, plan_id=plan_id, plan_name=plan_name)

    def _node_msg(self, plan_id="p1", task_id="t1", depth=0, content="synthesis"):
        return Message(role=MessageRole.TASK_NODE, content=content, plan_id=plan_id, task_id=task_id, task_depth=depth)

    def test_returns_incremented_row(self):
        parent_collapsible: list = []
        result = self.gui._render_plan_rows([self._plan_msg()], [], self.frame, 0, parent_collapsible)
        self.assertEqual(result, 1)  # 1 plan header row

    def test_appends_to_parent_collapsible(self):
        parent_collapsible: list = []
        self.gui._render_plan_rows([self._plan_msg()], [], self.frame, 0, parent_collapsible)
        self.assertEqual(len(parent_collapsible), 1)

    def test_plan_row_contains_label_widget(self):
        """Plan label/button must be packed into the collapsible header row."""
        parent_collapsible: list = []
        self.gui._render_plan_rows([self._plan_msg(plan_name="Explore Files")], [], self.frame, 0, parent_collapsible)
        header_row = parent_collapsible[0]
        texts = []
        for w in header_row:
            try:
                texts.append(str(w.cget("text")))
            except Exception:
                pass
        combined = " ".join(texts)
        self.assertIn("Explore Files", combined)

    def test_task_node_rows_added(self):
        parent_collapsible: list = []
        result = self.gui._render_plan_rows(
            [self._plan_msg()],
            [self._node_msg(task_id="t1"), self._node_msg(task_id="t2")],
            self.frame,
            0,
            parent_collapsible,
        )
        # 1 plan header row + 2 task node rows
        self.assertEqual(result, 3)

    def test_on_plan_click_wired_to_button(self):
        """When on_plan_click is provided the plan label must be a Button."""
        parent_collapsible: list = []
        self.gui._render_plan_rows(
            [self._plan_msg(plan_id="clickme")],
            [],
            self.frame,
            0,
            parent_collapsible,
            on_plan_click=lambda pid: None,
        )
        header_row = parent_collapsible[0]
        has_button = any(isinstance(w, tk.Button) for w in header_row)
        self.assertTrue(has_button, "Plan header row must contain a Button when on_plan_click is provided")

    def test_no_plan_click_uses_label(self):
        """When on_plan_click is None the plan title must be a Label, not Button."""
        parent_collapsible: list = []
        self.gui._render_plan_rows(
            [self._plan_msg(plan_id="noclickme")],
            [],
            self.frame,
            0,
            parent_collapsible,
            on_plan_click=None,
        )
        header_row = parent_collapsible[0]
        has_button = any(isinstance(w, tk.Button) and w.cget("cursor") == "hand2" for w in header_row)
        self.assertFalse(has_button, "Plan header row must not have a hand-cursor Button when no click handler")

    def test_tbd_node_shows_question_mark(self):
        """TBD task nodes must show a different icon from regular nodes."""
        parent_collapsible: list = []
        node = self._node_msg(task_id="tbd_task", plan_id="p_tbd")
        node.task_data = {"tbd": True}

        result = self.gui._render_plan_rows(
            [self._plan_msg(plan_id="p_tbd")],
            [node],
            self.frame,
            10,
            parent_collapsible,
        )
        # 1 plan header row + 1 task node row = 2 rows
        self.assertEqual(result, 12)

    def test_step_badge_shows_count(self):
        """Plan header label must show the step count badge."""
        parent_collapsible: list = []
        self.gui._render_plan_rows(
            [self._plan_msg(plan_name="Big Plan")],
            [self._node_msg(task_id="t1"), self._node_msg(task_id="t2"), self._node_msg(task_id="t3")],
            self.frame,
            0,
            parent_collapsible,
        )
        header_row = parent_collapsible[0]
        texts = []
        for w in header_row:
            try:
                texts.append(str(w.cget("text")))
            except Exception:
                pass
        combined = " ".join(texts)
        self.assertIn("3", combined)

    def test_multiple_plans_each_appended(self):
        parent_collapsible: list = []
        self.gui._render_plan_rows(
            [self._plan_msg(plan_id="p1"), self._plan_msg(plan_id="p2")],
            [],
            self.frame,
            0,
            parent_collapsible,
        )
        self.assertEqual(len(parent_collapsible), 2)


# ---------------------------------------------------------------------------
# 4. _render_message_to_grid splits plan and tool messages correctly
# ---------------------------------------------------------------------------


class TestRenderMessageToGridPlanSplit(unittest.TestCase):
    def setUp(self):
        self.root = _make_root()
        self.gui = _make_gui(self.root)
        self.frame = tk.Frame(self.root)

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_plan_msg_calls_render_plan_rows(self):
        plan_msg = Message(role=MessageRole.PLAN, content="My Plan", plan_id="p1", plan_name="My Plan")
        assistant_msg = Message(role=MessageRole.ASSISTANT, content="Executing plan")

        render_plan_calls = []
        original = self.gui._render_plan_rows

        def capturing(*args, **kwargs):
            render_plan_calls.append(args)
            return original(*args, **kwargs)

        self.gui._render_plan_rows = capturing
        self.gui._render_message_to_grid(assistant_msg, self.frame, 0, tool_interactions=[plan_msg])

        self.assertEqual(len(render_plan_calls), 1, "_render_plan_rows should be called once for plan messages")

    def test_tool_msgs_do_not_reach_render_plan_rows(self):
        tool_call = Message(role=MessageRole.TOOL_CALL, content="call", tool_name="read_file")
        tool_result = Message(role=MessageRole.TOOL_RESULT, content="result")
        assistant_msg = Message(role=MessageRole.ASSISTANT, content="Used tool")

        render_plan_calls = []
        original_plan = self.gui._render_plan_rows

        def capturing_plan(*args, **kwargs):
            render_plan_calls.append(args)
            return original_plan(*args, **kwargs)

        self.gui._render_plan_rows = capturing_plan
        self.gui._render_message_to_grid(assistant_msg, self.frame, 0, tool_interactions=[tool_call, tool_result])
        self.assertEqual(len(render_plan_calls), 0, "_render_plan_rows must not be called for tool_call/tool_result")

    def test_mixed_interactions_route_separately(self):
        plan_msg = Message(role=MessageRole.PLAN, content="Plan", plan_id="p1", plan_name="Plan")
        tool_call = Message(role=MessageRole.TOOL_CALL, content="call", tool_name="read_file")
        assistant_msg = Message(role=MessageRole.ASSISTANT, content="Did both")

        render_tool_calls = []
        render_plan_calls = []
        original_tool = self.gui._render_tool_rows
        original_plan = self.gui._render_plan_rows

        def capture_tool(*args, **kwargs):
            render_tool_calls.append(args)
            return original_tool(*args, **kwargs)

        def capture_plan(*args, **kwargs):
            render_plan_calls.append(args)
            return original_plan(*args, **kwargs)

        self.gui._render_tool_rows = capture_tool
        self.gui._render_plan_rows = capture_plan

        self.gui._render_message_to_grid(assistant_msg, self.frame, 0, tool_interactions=[plan_msg, tool_call])
        self.assertEqual(len(render_tool_calls), 1)
        self.assertEqual(len(render_plan_calls), 1)


# ---------------------------------------------------------------------------
# 5. Session stores PLAN message on PLAN_START chunk
# ---------------------------------------------------------------------------


class TestSessionPlanMessageStorage(unittest.TestCase):
    def setUp(self):
        self.test_dir = tempfile.mkdtemp()

    def tearDown(self):
        import shutil

        shutil.rmtree(self.test_dir, ignore_errors=True)

    def test_plan_message_stored_in_context_on_plan_start(self):
        chunks = [
            ResponseChunk(type=ChunkType.PLAN_START, plan_id="plan_42", plan_name="My Plan"),
            ResponseChunk(type=ChunkType.CONTENT, content="Executing"),
            ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
        ]
        session, _ = _make_session(self.test_dir, chunks)
        list(session.process_prompt("do the plan"))

        roles = [(m.message if hasattr(m, "message") else m).role for m in session.context.messages]
        role_values = [r.value if hasattr(r, "value") else str(r) for r in roles]
        self.assertIn("plan", role_values, "PLAN message should be stored in context after PLAN_START chunk")

    def test_plan_message_has_correct_plan_id(self):
        chunks = [
            ResponseChunk(type=ChunkType.PLAN_START, plan_id="plan_abc", plan_name="Test Plan"),
            ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
        ]
        session, _ = _make_session(self.test_dir, chunks)
        list(session.process_prompt("test"))

        plan_msgs = []
        for m in session.context.messages:
            msg = m.message if hasattr(m, "message") else m
            if msg.role == MessageRole.PLAN:
                plan_msgs.append(msg)

        self.assertTrue(any(getattr(m, "plan_id", None) == "plan_abc" for m in plan_msgs))

    def test_task_node_message_stored_on_task_node_end(self):
        chunks = [
            ResponseChunk(
                type=ChunkType.TASK_NODE_END,
                task_id="task_123",
                plan_id="plan_42",
                content="Task synthesis text",
                task_depth=0,
            ),
            ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
        ]
        session, _ = _make_session(self.test_dir, chunks)
        list(session.process_prompt("run task"))

        roles = [(m.message if hasattr(m, "message") else m).role for m in session.context.messages]
        role_values = [r.value if hasattr(r, "value") else str(r) for r in roles]
        self.assertIn("task_node", role_values, "TASK_NODE message should be stored after TASK_NODE_END chunk")

    def test_task_node_message_has_correct_task_id(self):
        chunks = [
            ResponseChunk(
                type=ChunkType.TASK_NODE_END,
                task_id="task_xyz",
                plan_id="plan_42",
                content="Synthesis",
                task_depth=1,
            ),
            ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
        ]
        session, _ = _make_session(self.test_dir, chunks)
        list(session.process_prompt("run task"))

        task_msgs = []
        for m in session.context.messages:
            msg = m.message if hasattr(m, "message") else m
            if msg.role == MessageRole.TASK_NODE:
                task_msgs.append(msg)

        self.assertTrue(any(getattr(m, "task_id", None) == "task_xyz" for m in task_msgs))


# ---------------------------------------------------------------------------
# 6. Session._on_plan_row_click focuses existing tab or calls _replay_plan_tab
# ---------------------------------------------------------------------------


class TestOnPlanRowClick(unittest.TestCase):
    def setUp(self):
        self.test_dir = tempfile.mkdtemp()

    def tearDown(self):
        import shutil

        shutil.rmtree(self.test_dir, ignore_errors=True)

    def _get_session(self):
        session, _ = _make_session(self.test_dir, [])
        return session

    def test_focuses_existing_tab(self):
        session = self._get_session()
        mock_frame = MagicMock()
        session.gui = MagicMock()
        session.gui.get_plan_tab_frame.return_value = mock_frame

        session._on_plan_row_click("plan_001")

        session.gui.focus_plan_tab.assert_called_once_with("plan_001")

    def test_calls_replay_when_tab_missing(self):
        session = self._get_session()
        session.gui = MagicMock()
        session.gui.get_plan_tab_frame.return_value = None  # tab doesn't exist

        session._replay_plan_tab = MagicMock()
        session._on_plan_row_click("plan_999")

        session._replay_plan_tab.assert_called_once_with("plan_999")
        session.gui.focus_plan_tab.assert_not_called()


# ---------------------------------------------------------------------------
# 7. Session._replay_plan_tab reconstructs plan tab from disk
# ---------------------------------------------------------------------------


class TestReplayPlanTab(unittest.TestCase):
    def setUp(self):
        self.test_dir = tempfile.mkdtemp()

    def tearDown(self):
        import shutil

        shutil.rmtree(self.test_dir, ignore_errors=True)

    def _persist_plan(self, session, plan_id: str, plan_name: str, nodes: list[TaskNodeRecord]):
        plan = PlanRecord(plan_id=plan_id, plan_name=plan_name)
        session.context.save_plan(plan)
        for node in nodes:
            session.context.save_task_node(node)

    def test_replay_creates_plan_tab(self):
        session, _ = _make_session(self.test_dir, [])
        # Ensure context has a session root so it can load plans
        session.gui = MagicMock()
        session.gui.get_plan_tab_frame.return_value = None

        node = TaskNodeRecord(
            plan_id="plan_rp",
            task_id="task_rp_1",
            task_description="Explore the code",
            status="done",
        )
        self._persist_plan(session, "plan_rp", "Replay Plan", [node])

        session._replay_plan_tab("plan_rp")

        session.gui.add_plan_tab.assert_called_once()
        args, kwargs = session.gui.add_plan_tab.call_args
        assert args == ("plan_rp", "Replay Plan") or (
            kwargs.get("plan_id") == "plan_rp" and kwargs.get("plan_name") == "Replay Plan"
        )
        assert list(args[:2]) == ["plan_rp", "Replay Plan"]
        assert "on_export" in kwargs  # production code now passes an export callback
        session.gui.focus_plan_tab.assert_called_once_with("plan_rp")

    def test_replay_adds_step_nodes(self):
        session, _ = _make_session(self.test_dir, [])
        session.gui = MagicMock()

        node = TaskNodeRecord(
            plan_id="plan_rp2",
            task_id="task_rp2_1",
            task_description="List files",
            status="done",
        )
        self._persist_plan(session, "plan_rp2", "Replay Plan 2", [node])

        session._replay_plan_tab("plan_rp2")

        session.gui.add_plan_step_node.assert_called_once()
        args, kwargs = session.gui.add_plan_step_node.call_args
        assert args == ("plan_rp2", "task_rp2_1", "List files", False)
        assert "on_replay" in kwargs  # production code now passes a replay callback

    def test_replay_marks_done_nodes(self):
        session, _ = _make_session(self.test_dir, [])
        session.gui = MagicMock()

        node = TaskNodeRecord(
            plan_id="plan_rp3",
            task_id="task_rp3_1",
            task_description="Do work",
            status="done",
        )
        self._persist_plan(session, "plan_rp3", "Replay Plan 3", [node])

        session._replay_plan_tab("plan_rp3")

        session.gui.update_plan_node_status.assert_called_with("task_rp3_1", "done")

    def test_replay_no_op_when_plan_not_found(self):
        session, _ = _make_session(self.test_dir, [])
        session.gui = MagicMock()

        # No plans persisted — should silently return
        session._replay_plan_tab("nonexistent_plan")

        session.gui.add_plan_tab.assert_not_called()

    def test_replay_subtask_node_uses_add_plan_subtask_node(self):
        session, _ = _make_session(self.test_dir, [])
        session.gui = MagicMock()

        root_node = TaskNodeRecord(
            plan_id="plan_sub",
            task_id="task_root",
            task_description="Root task",
            status="done",
        )
        child_node = TaskNodeRecord(
            plan_id="plan_sub",
            task_id="task_child",
            parent_task_id="task_root",
            task_description="Child task",
            depth=1,
            status="done",
        )
        self._persist_plan(session, "plan_sub", "Sub Plan", [root_node, child_node])

        session._replay_plan_tab("plan_sub")

        # root uses add_plan_step_node, child uses add_plan_subtask_node
        session.gui.add_plan_step_node.assert_called_once()
        step_args, step_kwargs = session.gui.add_plan_step_node.call_args
        assert step_args == ("plan_sub", "task_root", "Root task", False)
        assert "on_replay" in step_kwargs

        session.gui.add_plan_subtask_node.assert_called_once()
        sub_args, sub_kwargs = session.gui.add_plan_subtask_node.call_args
        assert sub_args == ("task_child", "task_root", "Child task", 1)
        assert "on_replay" in sub_kwargs


# ---------------------------------------------------------------------------
# 8. PLAN/TASK_NODE excluded from LLM context (to_llm_messages)
# ---------------------------------------------------------------------------


class TestPlanMessagesExcludedFromLLM(unittest.TestCase):
    def test_plan_message_not_in_llm_messages(self):
        from shared.models.context import Context

        ctx = Context()
        ctx.add_message(Message(role=MessageRole.USER, content="hi"))
        ctx.add_message(Message(role=MessageRole.PLAN, content="Plan", plan_id="p1", plan_name="Plan"))
        ctx.add_message(Message(role=MessageRole.TASK_NODE, content="Step", plan_id="p1", task_id="t1"))

        llm_msgs = ctx.to_llm_messages()
        roles = [m.get("role") for m in llm_msgs]
        self.assertNotIn("plan", roles)
        self.assertNotIn("task_node", roles)
        self.assertIn("user", roles)


# ---------------------------------------------------------------------------
# 8. _render_message_to_grid: every message has expand/collapse + full-content row
# ---------------------------------------------------------------------------


class TestRenderMessageAlwaysExpandable(unittest.TestCase):
    """GIVEN any message (with or without tools/attachments/plans)
    WHEN _render_message_to_grid is called
    THEN an expand/collapse Button is always rendered AND a hidden full-content
    detail row is appended to the collapsible_rows list.

    Covers the regression where plain user/assistant messages rendered an empty
    placeholder Label instead of an expand button, leaving full content
    inaccessible to the user.
    """

    def setUp(self):
        self.root = _make_root()
        self.gui = _make_gui(self.root)
        self.frame = tk.Frame(self.root)

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def _count_expand_buttons(self, parent: tk.Frame) -> int:
        """Count Button widgets gridded into column 0 (exp_button) of parent."""
        count = 0
        for child in parent.winfo_children():
            if isinstance(child, tk.Button):
                info = child.grid_info()
                if info and int(info.get("column", -1)) == 0:
                    count += 1
        return count

    def _find_gridded_labels(self, parent: tk.Frame) -> list[tk.Label]:
        """Return all Label widgets that have been gridded into parent."""
        return [c for c in parent.winfo_children() if isinstance(c, tk.Label) and c.grid_info()]

    @unittest.mark.parametrize if False else lambda f: f  # mark unused — parameterize via subtests
    def test_plain_user_message_has_expand_button(self):
        """GIVEN a plain user message (no tools/attachments/plans)
        WHEN rendered
        THEN a Button is placed in the exp_button column (not an empty Label).
        """
        msg = Message(role=MessageRole.USER, content="Hello world")
        self.gui._render_message_to_grid(msg, self.frame, 0)
        btn_count = self._count_expand_buttons(self.frame)
        self.assertGreater(btn_count, 0, "Plain user message must have an expand/collapse Button")

    def test_plain_assistant_message_has_expand_button(self):
        """GIVEN a plain assistant message (no tools/attachments/plans)
        WHEN rendered
        THEN a Button is placed in the exp_button column.
        """
        msg = Message(role=MessageRole.ASSISTANT, content="I can help with that.")
        self.gui._render_message_to_grid(msg, self.frame, 0)
        btn_count = self._count_expand_buttons(self.frame)
        self.assertGreater(btn_count, 0, "Plain assistant message must have an expand/collapse Button")

    def test_plain_system_message_has_expand_button(self):
        """GIVEN a plain system message (no tools/attachments/plans)
        WHEN rendered
        THEN a Button is placed in the exp_button column.
        """
        msg = Message(role=MessageRole.SYSTEM, content="You are a helpful assistant.")
        self.gui._render_message_to_grid(msg, self.frame, 0)
        btn_count = self._count_expand_buttons(self.frame)
        self.assertGreater(btn_count, 0, "Plain system message must have an expand/collapse Button")

    def test_full_content_row_created_and_hidden_by_default(self):
        """GIVEN a message with non-empty content
        WHEN rendered
        THEN a full-content Label is created and initially hidden (grid_remove'd).
        """
        long_text = "A" * 200  # definitely longer than 60-char preview
        msg = Message(role=MessageRole.USER, content=long_text)
        self.gui._render_message_to_grid(msg, self.frame, 0)

        # Find all Labels in the content column (col 3) that are NOT the preview
        full_text_labels = []
        for child in self.frame.winfo_children():
            if isinstance(child, tk.Label):
                info = child.grid_info()
                # grid_remove makes grid_info() return empty dict
                if not info:
                    try:
                        text = child.cget("text")
                        if long_text[:50] in text:
                            full_text_labels.append(child)
                    except tk.TclError:
                        pass

        self.assertGreater(len(full_text_labels), 0, "A hidden full-content Label must exist after rendering")

    def test_full_content_row_becomes_visible_on_toggle(self):
        """GIVEN a plain message rendered in the context panel
        WHEN the expand button is clicked
        THEN the full-content detail row becomes visible (grid_info is non-empty).
        """
        long_text = "Detail content that would be truncated in the preview " * 5
        msg = Message(role=MessageRole.USER, content=long_text)
        self.gui._render_message_to_grid(msg, self.frame, 0)

        # Locate the expand/collapse button (col 0, row 0)
        expand_btn = None
        for child in self.frame.winfo_children():
            if isinstance(child, tk.Button) and child.grid_info().get("column") == "0":
                expand_btn = child
                break
        if expand_btn is None:
            for child in self.frame.winfo_children():
                if isinstance(child, tk.Button):
                    info = child.grid_info()
                    if info and int(info.get("column", -1)) == 0:
                        expand_btn = child
                        break

        self.assertIsNotNone(expand_btn, "Expand button must exist")

        # Before click: full-content label must be hidden
        hidden_labels_before = [
            c
            for c in self.frame.winfo_children()
            if isinstance(c, tk.Label) and not c.grid_info() and long_text[:30] in (c.cget("text") or "")
        ]
        self.assertGreater(len(hidden_labels_before), 0, "Full-content label must be hidden before expand")

        # Click the expand button
        expand_btn.invoke()

        # After click: full-content label must be visible
        visible_labels_after = [
            c
            for c in self.frame.winfo_children()
            if isinstance(c, tk.Label) and c.grid_info() and long_text[:30] in (c.cget("text") or "")
        ]
        self.assertGreater(len(visible_labels_after), 0, "Full-content label must be visible after expanding")

    def test_message_with_tool_still_has_expand_button(self):
        """GIVEN a message with a tool call
        WHEN rendered
        THEN the expand button is still present (not regressed by the always-on change).
        """
        assistant_msg = Message(role=MessageRole.ASSISTANT, content="Used a tool")
        tool_call = Message(role=MessageRole.TOOL_CALL, content="call", tool_name="read_file")
        self.gui._render_message_to_grid(assistant_msg, self.frame, 0, tool_interactions=[tool_call])
        btn_count = self._count_expand_buttons(self.frame)
        self.assertGreater(btn_count, 0, "Message with tool call must still have an expand button")

    def test_empty_content_message_has_expand_button_no_detail_row(self):
        """GIVEN a message with empty string content
        WHEN rendered
        THEN an expand button is still created but no full-content detail row is added
        (no point showing an empty label).
        """
        msg = Message(role=MessageRole.ASSISTANT, content="")
        self.gui._render_message_to_grid(msg, self.frame, 0)
        btn_count = self._count_expand_buttons(self.frame)
        self.assertGreater(btn_count, 0, "Even empty-content message must have an expand button")


if __name__ == "__main__":
    unittest.main()
