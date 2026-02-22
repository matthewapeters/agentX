import unittest
from unittest.mock import MagicMock, patch

from agentx.session import AgentXSession
from shared.models.message import MessageRole
from shared.models.response import ChunkType, ResponseChunk


class TestSessionStreamContextPersistence(unittest.TestCase):
    def setUp(self):
        import tempfile

        self.test_dir = tempfile.mkdtemp()
        self.config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "gpt-oss",
                "temperature": 0.7,
            },
            "agentix": {
                "classify_prompts": False,
            },
        }
        self._adapter_patcher = patch("agentx.session.create_adapter")
        self.mock_create = self._adapter_patcher.start()

    def tearDown(self):
        import os
        import shutil

        self._adapter_patcher.stop()
        if os.path.exists(self.test_dir):
            shutil.rmtree(self.test_dir)

    def _make_session_with_stream(self, chunks):
        mock_adapter = MagicMock()
        mock_adapter.process_prompt_generator.side_effect = lambda *_args, **_kwargs: iter(chunks)
        mock_adapter.classify_prompt_sync.return_value = None
        mock_adapter.get_models.return_value = []
        mock_adapter.get_tools.return_value = []
        self.mock_create.return_value = mock_adapter

        return AgentXSession(
            username="test_user",
            session_dir=self.test_dir,
            config=self.config,
        )

    def test_process_prompt_persists_thinking_and_assistant(self):
        session = self._make_session_with_stream(
            [
                ResponseChunk(type=ChunkType.THINKING, content="Thinking..."),
                ResponseChunk(type=ChunkType.CONTENT, content="Hello"),
                ResponseChunk(type=ChunkType.CONTENT, content=" world"),
                ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
            ]
        )

        list(session.process_prompt("hi"))

        messages = [entry.message if hasattr(entry, "message") else entry for entry in session.context.messages]
        roles = [msg.role for msg in messages]

        self.assertIn(MessageRole.USER, roles)
        self.assertIn(MessageRole.THINKING, roles)
        self.assertIn(MessageRole.ASSISTANT, roles)

        thinking_message = [msg for msg in messages if msg.role == MessageRole.THINKING][-1]
        assistant_message = [msg for msg in messages if msg.role == MessageRole.ASSISTANT][-1]

        self.assertEqual(thinking_message.content, "Thinking...")
        self.assertFalse(thinking_message.enabled)
        self.assertEqual(assistant_message.content, "Hello world")
        self.assertTrue(assistant_message.enabled)

    def test_gui_worker_persists_thinking_and_assistant(self):
        session = self._make_session_with_stream(
            [
                ResponseChunk(type=ChunkType.THINKING, content="Plan step 1."),
                ResponseChunk(type=ChunkType.THINKING, content=" Plan step 2."),
                ResponseChunk(type=ChunkType.CONTENT, content="Answer part A."),
                ResponseChunk(type=ChunkType.CONTENT, content=" Answer part B."),
                ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
            ]
        )

        session.gui.create_layout()
        session._pending_prompt = "run gui flow"
        session.stream_ollama_response_worker()

        messages = [entry.message if hasattr(entry, "message") else entry for entry in session.context.messages]

        thinking_message = [msg for msg in messages if msg.role == MessageRole.THINKING][-1]
        assistant_message = [msg for msg in messages if msg.role == MessageRole.ASSISTANT][-1]

        self.assertEqual(thinking_message.content, "Plan step 1. Plan step 2.")
        self.assertFalse(thinking_message.enabled)
        self.assertEqual(assistant_message.content, "Answer part A. Answer part B.")
        self.assertTrue(assistant_message.enabled)


if __name__ == "__main__":
    unittest.main(verbosity=2)
