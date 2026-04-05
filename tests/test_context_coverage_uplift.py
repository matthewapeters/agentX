"""Targeted coverage uplift tests for shared/models/context.py.

Covers paths not exercised by existing tests:
- MessageEntry iteration and attribute proxy
- add_message with/without path, enabled messages
- get_messages(enabled_only=False)
- to_llm_messages, to_dict, to_payload, from_dict
- load() from file vs directory
- save() to file vs directory
- clear(), __len__, __iter__
- get_user_messages, get_assistant_messages
- estimate_tokens, trim_to_tokens
- Context task-node helpers: save_plan, load_plans, save_task_node, load_task_nodes,
  save_task_tree, load_task_tree, get_scratch_dir
"""

import json
import os
import tempfile
import unittest
from datetime import datetime, timezone

from shared.models.context import Context, MessageEntry
from shared.models.message import Message, MessageRole
from shared.models.task_node import PlanRecord, PlanStep, TaskNodeRecord, TaskTree

# ── helpers ──────────────────────────────────────────────────────────────────


def _user_msg(text: str = "hello") -> Message:
    return Message(role="user", content=text)


def _assistant_msg(text: str = "hi") -> Message:
    return Message(role="assistant", content=text)


def _make_plan() -> PlanRecord:
    return PlanRecord(
        plan_id="plan_0",
        plan_name="Test plan",
        steps=[PlanStep(step_id="s0", description="Do it")],
        root_task_ids=["task_0"],
    )


def _make_node() -> TaskNodeRecord:
    return TaskNodeRecord(plan_id="plan_0", task_id="task_0", task_description="Do it")


# ── MessageEntry ──────────────────────────────────────────────────────────────


class TestMessageEntry(unittest.TestCase):
    def _entry(self) -> MessageEntry:
        msg = _user_msg("test")
        return MessageEntry(timestamp=msg.timestamp, message=msg)

    def test_iter_yields_timestamp_and_message(self):
        entry = self._entry()
        ts, msg = entry
        self.assertIsInstance(ts, datetime)
        self.assertIsInstance(msg, Message)

    def test_getattr_proxies_to_message(self):
        entry = self._entry()
        self.assertEqual(entry.content, "test")
        self.assertEqual(entry.role, "user")

    def test_setattr_proxies_content_to_message(self):
        entry = self._entry()
        entry.content = "updated"
        self.assertEqual(entry.message.content, "updated")


# ── add_message ───────────────────────────────────────────────────────────────


class TestContextAddMessage(unittest.TestCase):
    def test_add_message_no_path(self):
        ctx = Context()
        msg = _user_msg()
        ctx.add_message(msg)
        self.assertEqual(len(ctx.messages), 1)

    def test_add_message_with_explicit_timestamp(self):
        ctx = Context()
        msg = _user_msg()
        ts = datetime(2024, 1, 1, tzinfo=timezone.utc)
        ctx.add_message(msg, ts=ts)
        self.assertEqual(ctx.messages[0].timestamp, ts)

    def test_add_message_swapped_args_raises(self):
        """Passing (datetime, Message) in wrong order raises AttributeError — no silent swap."""
        ctx = Context()
        msg = _user_msg()
        ts = datetime.now(timezone.utc)
        with self.assertRaises((AttributeError, TypeError)):
            ctx.add_message(ts, msg)  # type: ignore[arg-type]

    def test_add_message_saves_to_disk_when_path_set(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx = Context(path=tmp)
            msg = _user_msg("saved")
            ctx.add_message(msg)
            files = [f for f in os.listdir(tmp) if f.endswith(".json")]
            self.assertEqual(len(files), 1)


# ── get_messages ──────────────────────────────────────────────────────────────


class TestContextGetMessages(unittest.TestCase):
    def _ctx_with_two(self) -> Context:
        ctx = Context()
        m1 = _user_msg("a")
        m1.enabled = True
        m2 = _user_msg("b")
        m2.enabled = False
        ctx.add_message(m1)
        ctx.add_message(m2)
        return ctx

    def test_get_messages_enabled_only_default(self):
        ctx = self._ctx_with_two()
        msgs = ctx.get_messages()
        self.assertEqual(len(msgs), 1)
        self.assertEqual(msgs[0].content, "a")

    def test_get_messages_not_enabled_only(self):
        ctx = self._ctx_with_two()
        msgs = ctx.get_messages(enabled_only=False)
        self.assertEqual(len(msgs), 2)

    def test_get_user_messages(self):
        ctx = Context()
        ctx.add_message(_user_msg("u"))
        ctx.add_message(_assistant_msg("a"))
        users = ctx.get_user_messages()
        self.assertEqual(len(users), 1)
        self.assertEqual(users[0].content, "u")

    def test_get_assistant_messages(self):
        ctx = Context()
        ctx.add_message(_user_msg("u"))
        ctx.add_message(_assistant_msg("a"))
        assistants = ctx.get_assistant_messages()
        self.assertEqual(len(assistants), 1)
        self.assertEqual(assistants[0].content, "a")

    def test_get_last_user_message(self):
        ctx = Context()
        ctx.add_message(_user_msg("first"))
        ctx.add_message(_user_msg("last"))
        self.assertEqual(ctx.get_last_user_message().content, "last")

    def test_get_last_user_message_none_when_empty(self):
        self.assertIsNone(Context().get_last_user_message())

    def test_get_last_assistant_message(self):
        ctx = Context()
        ctx.add_message(_assistant_msg("first"))
        ctx.add_message(_assistant_msg("last"))
        self.assertEqual(ctx.get_last_assistant_message().content, "last")


# ── to_llm_messages / to_dict / to_payload ───────────────────────────────────


class TestContextSerialization(unittest.TestCase):
    def test_to_llm_messages_empty(self):
        self.assertEqual(Context().to_llm_messages(), [])

    def test_to_llm_messages_excludes_disabled(self):
        ctx = Context()
        m = _user_msg("hi")
        m.enabled = True
        ctx.add_message(m)
        m2 = _user_msg("hidden")
        m2.enabled = False
        ctx.add_message(m2)
        llm_msgs = ctx.to_llm_messages()
        self.assertEqual(len(llm_msgs), 1)
        self.assertEqual(llm_msgs[0]["content"], "hi")

    def test_to_llm_messages_excludes_internal_roles(self):
        ctx = Context()
        plan_msg = Message(role=MessageRole.PLAN, content="plan")
        plan_msg.enabled = True
        ctx.add_message(plan_msg)
        user_msg = _user_msg("user text")
        user_msg.enabled = True
        ctx.add_message(user_msg)
        llm_msgs = ctx.to_llm_messages()
        roles = [m["role"] for m in llm_msgs]
        self.assertNotIn("plan", roles)
        self.assertIn("user", roles)

    def test_to_dict_includes_messages(self):
        ctx = Context(session_id="sid-1")
        ctx.add_message(_user_msg("hi"))
        d = ctx.to_dict()
        self.assertEqual(d["session_id"], "sid-1")
        self.assertEqual(len(d["messages"]), 1)

    def test_to_payload_includes_model(self):
        ctx = Context()
        payload = ctx.to_payload(model="llama3", stream=True)
        self.assertEqual(payload["model"], "llama3")
        self.assertTrue(payload["stream"])

    def test_from_dict_round_trip(self):
        ctx = Context(session_id="sess-abc")
        ctx.add_message(_user_msg("hello"))
        d = ctx.to_dict()
        ctx2 = Context.from_dict(d)
        self.assertEqual(ctx2.session_id, "sess-abc")
        self.assertEqual(len(ctx2.messages), 1)


# ── load() / save() ───────────────────────────────────────────────────────────


class TestContextPersistence(unittest.TestCase):
    def test_load_from_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx = Context(path=tmp)
            ctx.add_message(_user_msg("persisted"))
            ctx2 = Context.load(tmp)
            self.assertEqual(len(ctx2.messages), 1)

    def test_load_from_json_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx = Context(session_id="s1")
            ctx.add_message(_user_msg("json"))
            file_path = os.path.join(tmp, "context.json")
            ctx.save(file_path)
            ctx2 = Context.load(file_path)
            self.assertEqual(ctx2.session_id, "s1")
            self.assertEqual(len(ctx2.messages), 1)

    def test_save_to_directory(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx = Context()
            ctx.add_message(_user_msg("dir-save"))
            ctx.save(tmp)
            files = [f for f in os.listdir(tmp) if f.endswith(".json")]
            self.assertGreater(len(files), 0)

    def test_load_from_dir_handles_corrupt_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            with open(os.path.join(tmp, "bad.json"), "w") as f:
                f.write("not json{{{{")
            ctx = Context(path=tmp)
            ctx.load_from_dir(tmp)  # Should not raise
            self.assertEqual(len(ctx.messages), 0)

    def test_load_from_dir_raises_without_path(self):
        ctx = Context()
        with self.assertRaises(ValueError):
            ctx.load_from_dir()

    def test_save_raises_without_path(self):
        ctx = Context()
        ctx.add_message(_user_msg("x"))
        with self.assertRaises(ValueError):
            ctx.save()


# ── clear / __len__ / __iter__ ────────────────────────────────────────────────


class TestContextDataModel(unittest.TestCase):
    def test_clear_removes_messages(self):
        ctx = Context()
        ctx.add_message(_user_msg())
        ctx.clear()
        self.assertEqual(len(ctx), 0)

    def test_len(self):
        ctx = Context()
        ctx.add_message(_user_msg())
        ctx.add_message(_user_msg())
        self.assertEqual(len(ctx), 2)

    def test_iter(self):
        ctx = Context()
        ctx.add_message(_user_msg("a"))
        ctx.add_message(_user_msg("b"))
        entries = list(ctx)
        self.assertEqual(len(entries), 2)


# ── estimate_tokens / trim_to_tokens ─────────────────────────────────────────


class TestContextTokens(unittest.TestCase):
    def test_estimate_tokens_empty(self):
        self.assertEqual(Context().estimate_tokens(), 0)

    def test_estimate_tokens_counts_content(self):
        ctx = Context()
        msg = _user_msg("a" * 100)
        msg.enabled = True
        ctx.add_message(msg)
        tokens = ctx.estimate_tokens()
        self.assertGreater(tokens, 0)

    def test_trim_to_tokens_keeps_recent(self):
        ctx = Context()
        for i in range(10):
            m = _user_msg(f"message {i} " + "x" * 100)
            m.enabled = True
            ctx.add_message(m)
        trimmed = ctx.trim_to_tokens(50)
        self.assertLess(len(trimmed), 10)

    def test_trim_to_tokens_always_keeps_system(self):
        ctx = Context()
        sys_msg = Message(role=MessageRole.SYSTEM, content="System prompt " + "s" * 100)
        sys_msg.enabled = True
        ctx.add_message(sys_msg)
        other = _user_msg("x" * 1000)
        other.enabled = True
        ctx.add_message(other)
        trimmed = ctx.trim_to_tokens(50)
        roles = [m.role for m in trimmed]
        self.assertIn(MessageRole.SYSTEM, roles)


# ── task-node/tree helpers ────────────────────────────────────────────────────


class TestContextTaskHelpers(unittest.TestCase):
    def test_plans_dir_none_without_path(self):
        ctx = Context()
        self.assertIsNone(ctx._plans_dir)

    def test_task_nodes_dir_none_without_path(self):
        ctx = Context()
        self.assertIsNone(ctx._task_nodes_dir)

    def test_get_scratch_dir_creates_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx_path = os.path.join(tmp, "context")
            os.makedirs(ctx_path)
            ctx = Context(path=ctx_path)
            scratch = ctx.get_scratch_dir()
            self.assertTrue(os.path.isdir(scratch))

    def test_get_scratch_dir_raises_without_path(self):
        ctx = Context()
        with self.assertRaises(ValueError):
            ctx.get_scratch_dir()

    def test_save_plan_raises_without_path(self):
        ctx = Context()
        with self.assertRaises(ValueError):
            ctx.save_plan(_make_plan())

    def test_save_and_load_plans(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx_path = os.path.join(tmp, "context")
            os.makedirs(ctx_path)
            ctx = Context(path=ctx_path)
            ctx.save_plan(_make_plan())
            loaded = ctx.load_plans()
            self.assertEqual(len(loaded), 1)
            self.assertEqual(loaded[0].plan_id, "plan_0")

    def test_load_plans_empty_when_no_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx_path = os.path.join(tmp, "context")
            os.makedirs(ctx_path)
            ctx = Context(path=ctx_path)
            self.assertEqual(ctx.load_plans(), [])

    def test_save_and_load_task_nodes(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx_path = os.path.join(tmp, "context")
            os.makedirs(ctx_path)
            ctx = Context(path=ctx_path)
            ctx.save_task_node(_make_node())
            loaded = ctx.load_task_nodes()
            self.assertEqual(len(loaded), 1)
            self.assertEqual(loaded[0].task_id, "task_0")

    def test_load_task_nodes_empty_when_no_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx_path = os.path.join(tmp, "context")
            os.makedirs(ctx_path)
            ctx = Context(path=ctx_path)
            self.assertEqual(ctx.load_task_nodes(), [])

    def test_save_and_load_task_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx_path = os.path.join(tmp, "context")
            os.makedirs(ctx_path)
            ctx = Context(path=ctx_path)
            tree = TaskTree(session_id="s1")
            tree.add_plan(_make_plan())
            ctx.save_task_tree(tree)
            loaded = ctx.load_task_tree()
            self.assertIsNotNone(loaded)
            self.assertIn("plan_0", loaded.plans)

    def test_load_task_tree_returns_none_when_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx_path = os.path.join(tmp, "context")
            os.makedirs(ctx_path)
            ctx = Context(path=ctx_path)
            self.assertIsNone(ctx.load_task_tree())

    def test_load_task_tree_returns_none_without_path(self):
        ctx = Context()
        self.assertIsNone(ctx.load_task_tree())

    def test_save_task_tree_raises_without_path(self):
        ctx = Context()
        tree = TaskTree(session_id="s1")
        with self.assertRaises(ValueError):
            ctx.save_task_tree(tree)

    def test_load_plans_skips_corrupt_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx_path = os.path.join(tmp, "context")
            os.makedirs(ctx_path)
            ctx = Context(path=ctx_path)
            plans_dir = ctx._plans_dir
            os.makedirs(plans_dir, exist_ok=True)
            with open(os.path.join(plans_dir, "bad.json"), "w") as f:
                f.write("not json{{{{")
            result = ctx.load_plans()
            self.assertEqual(result, [])

    def test_load_task_nodes_skips_corrupt_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx_path = os.path.join(tmp, "context")
            os.makedirs(ctx_path)
            ctx = Context(path=ctx_path)
            nodes_dir = ctx._task_nodes_dir
            os.makedirs(nodes_dir, exist_ok=True)
            with open(os.path.join(nodes_dir, "bad.json"), "w") as f:
                f.write("not json{{{{")
            result = ctx.load_task_nodes()
            self.assertEqual(result, [])

    def test_load_task_tree_returns_none_on_corrupt(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx_path = os.path.join(tmp, "context")
            os.makedirs(ctx_path)
            ctx = Context(path=ctx_path)
            # Write a corrupt task_tree.json in session root
            root = ctx._session_root
            with open(os.path.join(root, "task_tree.json"), "w") as f:
                f.write("{{not valid json")
            result = ctx.load_task_tree()
            self.assertIsNone(result)

    def test_load_messages_alias(self):
        with tempfile.TemporaryDirectory() as tmp:
            ctx = Context(path=tmp)
            ctx.add_message(_user_msg("alias"))
            ctx2 = Context(path=tmp)
            ctx2.load_messages()
            self.assertEqual(len(ctx2.messages), 1)


class TestContextToolMessages(unittest.TestCase):
    def test_add_tool_call_message(self):
        ctx = Context()
        msg = ctx.add_tool_call_message("my_tool", {"arg": "val"}, tool_id="tc-1")
        self.assertEqual(msg.role, MessageRole.TOOL_CALL)
        self.assertIn("my_tool", msg.content)

    def test_add_tool_result_message(self):
        ctx = Context()
        msg = ctx.add_tool_result_message("my_tool", {"result": 42}, tool_id="tc-1", success=True)
        self.assertEqual(msg.role, MessageRole.TOOL_RESULT)

    def test_save_task_node_raises_without_path(self):
        ctx = Context()
        with self.assertRaises(ValueError):
            ctx.save_task_node(_make_node())


class TestContextMiscCoverage(unittest.TestCase):
    """Additional tests for remaining uncovered lines."""

    def test_setattr_non_message_attr_sets_on_self(self):
        entry = MessageEntry(timestamp=datetime.now(timezone.utc), message=_user_msg())
        # Setting an attribute that is NOT on message uses object.__setattr__
        entry.custom_attr = "value"
        self.assertEqual(entry.custom_attr, "value")
        # message should not have gained the attribute
        self.assertFalse(hasattr(entry.message, "custom_attr"))

    def test_to_payload_with_options(self):
        ctx = Context()
        payload = ctx.to_payload(model="llama3", stream=True, options={"temperature": 0.5})
        self.assertEqual(payload["options"]["temperature"], 0.5)

    def test_get_tool_messages(self):
        ctx = Context()
        tc = Message(role=MessageRole.TOOL_CALL, content="call")
        tc.enabled = True
        tr = Message(role=MessageRole.TOOL_RESULT, content="result")
        tr.enabled = True
        ctx.add_message(_user_msg("u"))
        ctx.add_message(tc)
        ctx.add_message(tr)
        tools = ctx.get_tool_messages()
        self.assertEqual(len(tools), 2)

    def test_get_last_assistant_message_returns_none_when_empty(self):
        self.assertIsNone(Context().get_last_assistant_message())

    def test_estimate_tokens_counts_enabled_attachments(self):
        from shared.models.attachment import Attachment

        ctx = Context()
        msg = _user_msg("short")
        msg.enabled = True
        # Attach 400-char content -> ~100 tokens
        att = Attachment(file_path="f.txt", content="a" * 400)
        att.enabled = True
        msg.attachments.append(att)
        ctx.add_message(msg)
        tokens = ctx.estimate_tokens()
        self.assertGreaterEqual(tokens, 100)

    def test_trim_drops_message_that_exceeds_budget(self):
        ctx = Context()
        # Add a large message that won't fit
        big = _user_msg("x" * 4000)
        big.enabled = True
        ctx.add_message(big)
        # Tiny budget
        trimmed = ctx.trim_to_tokens(5)
        # big message had ~1000 tokens — won't fit in 5 token budget
        big_contents = [m.content for m in trimmed if len(m.content) > 100]
        self.assertEqual(big_contents, [])

    def test_load_from_dir_skips_message_with_exception(self):
        with tempfile.TemporaryDirectory() as tmp:
            # Write a file with bad JSON to trigger the except branch
            with open(os.path.join(tmp, "20240101T000000Z_bad.json"), "w") as f:
                f.write("{invalid json}")
            ctx = Context(path=tmp)
            ctx.load_from_dir(tmp)  # Should not raise, should skip file
            self.assertEqual(len(ctx.messages), 0)
