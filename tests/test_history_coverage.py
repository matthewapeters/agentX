"""Coverage uplift tests for agentx/history.py.

Covers:
- History.__init__ with valid sessions (messages loaded)
- History.__init__ with OSError on listdir → early return
- History.__init__ with exclude_session skipping the current session
- History.__init__ with load_from_dir OSError → continue to next session
- History.__init__ with empty context (no messages → not appended)
- History.open_task_tree — file not found → None
- History.open_task_tree — file present, valid tree
- History.open_task_tree — corrupt file → None
- History.get_enabled_messages
"""

import json
import os
import tempfile
import unittest
from datetime import datetime, timezone
from unittest.mock import patch

from agentx.history import History
from shared.models.message import Message, MessageRole


def _write_message_file(context_dir: str, content: str = "hello") -> None:
    """Write a valid message JSON file into `context_dir`."""
    os.makedirs(context_dir, exist_ok=True)
    msg = Message(role="user", content=content)
    ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    path = os.path.join(context_dir, f"{ts}_msg.json")
    with open(path, "w") as f:
        json.dump(msg.to_dict(), f)


class TestHistoryInit(unittest.TestCase):
    def test_empty_directory_has_no_sessions(self):
        with tempfile.TemporaryDirectory() as tmp:
            h = History(tmp)
            self.assertEqual(h.sessions, [])

    def test_session_with_messages_is_loaded(self):
        with tempfile.TemporaryDirectory() as tmp:
            session_dir = os.path.join(tmp, "session_001")
            ctx_dir = os.path.join(session_dir, "context")
            _write_message_file(ctx_dir)
            h = History(tmp)
            self.assertEqual(len(h.sessions), 1)
            self.assertEqual(h.sessions[0].session_id, "session_001")

    def test_session_without_messages_is_not_included(self):
        with tempfile.TemporaryDirectory() as tmp:
            # Create session dir but no message files
            os.makedirs(os.path.join(tmp, "empty_session", "context"), exist_ok=True)
            h = History(tmp)
            self.assertEqual(h.sessions, [])

    def test_oserror_on_listdir_returns_empty_history(self):
        h = History("/nonexistent/path/that/does/not/exist/xyz")
        self.assertEqual(h.sessions, [])

    def test_exclude_session_is_skipped(self):
        with tempfile.TemporaryDirectory() as tmp:
            session_dir = os.path.join(tmp, "session_current")
            ctx_dir = os.path.join(session_dir, "context")
            _write_message_file(ctx_dir)
            h = History(tmp, exclude_session=session_dir)
            self.assertEqual(h.sessions, [])

    def test_exclude_session_only_skips_one(self):
        with tempfile.TemporaryDirectory() as tmp:
            s1 = os.path.join(tmp, "session_001")
            s2 = os.path.join(tmp, "session_002")
            _write_message_file(os.path.join(s1, "context"))
            _write_message_file(os.path.join(s2, "context"), content="world")
            h = History(tmp, exclude_session=s1)
            self.assertEqual(len(h.sessions), 1)
            self.assertEqual(h.sessions[0].session_id, "session_002")

    def test_oserror_on_load_from_dir_continues_to_next_session(self):
        with tempfile.TemporaryDirectory() as tmp:
            # Create a valid second session
            s2 = os.path.join(tmp, "session_b")
            _write_message_file(os.path.join(s2, "context"))
            # Create a first session dir where context/ doesn't exist (load_from_dir raises)
            os.makedirs(os.path.join(tmp, "session_a"), exist_ok=True)
            # Don't create context/ under session_a → load_from_dir raises ValueError or OSError

            # Patch load_from_dir to raise OSError on first session
            original_load = type(None)
            call_count = [0]

            from shared.models.context import Context

            original_load_from_dir = Context.load_from_dir

            def patched_load(self_ctx, path=None):
                call_count[0] += 1
                if call_count[0] == 1:
                    raise OSError("simulated disk error")
                return original_load_from_dir(self_ctx, path)

            with patch.object(Context, "load_from_dir", patched_load):
                h = History(tmp)
            # First session errored, second should succeed
            self.assertEqual(len(h.sessions), 1)
            self.assertEqual(h.sessions[0].session_id, "session_b")

    def test_sessions_collapsed_by_default(self):
        with tempfile.TemporaryDirectory() as tmp:
            session_dir = os.path.join(tmp, "session_001")
            _write_message_file(os.path.join(session_dir, "context"))
            h = History(tmp)
            self.assertFalse(h.sessions[0].expanded)


class TestHistoryOpenTaskTree(unittest.TestCase):
    def test_returns_none_when_no_task_tree_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = History.open_task_tree(tmp)
            self.assertIsNone(result)

    def test_returns_none_for_corrupt_task_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            bad_file = os.path.join(tmp, "task_tree.json")
            with open(bad_file, "w") as f:
                f.write("{not valid json{{")
            result = History.open_task_tree(tmp)
            self.assertIsNone(result)

    def test_returns_task_tree_for_valid_file(self):
        from shared.models.task_node import TaskTree

        with tempfile.TemporaryDirectory() as tmp:
            tree = TaskTree(session_id="test-session")
            tree.save(tmp)
            result = History.open_task_tree(tmp)
            self.assertIsNotNone(result)
            self.assertEqual(result.session_id, "test-session")


class TestHistoryGetEnabledMessages(unittest.TestCase):
    def test_returns_empty_for_empty_history(self):
        with tempfile.TemporaryDirectory() as tmp:
            h = History(tmp)
            self.assertEqual(h.get_enabled_messages(), [])

    def test_returns_enabled_messages(self):
        with tempfile.TemporaryDirectory() as tmp:
            session_dir = os.path.join(tmp, "session_001")
            ctx_dir = os.path.join(session_dir, "context")
            _write_message_file(ctx_dir, content="enabled msg")
            h = History(tmp)
            # Messages start as disabled after load
            # Enable one manually via the context
            for ctx in h.sessions:
                for entry in ctx.messages:
                    entry.message.enabled = True
            msgs = h.get_enabled_messages()
            self.assertEqual(len(msgs), 1)

    def test_disabled_messages_not_returned(self):
        with tempfile.TemporaryDirectory() as tmp:
            session_dir = os.path.join(tmp, "session_001")
            _write_message_file(os.path.join(session_dir, "context"), content="disabled msg")
            h = History(tmp)
            # Leave messages disabled (default after load)
            msgs = h.get_enabled_messages()
            self.assertEqual(msgs, [])
