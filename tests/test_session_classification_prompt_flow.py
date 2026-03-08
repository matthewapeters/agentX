import tempfile
import tkinter as tk
import unittest
from unittest.mock import patch

from agentx.session import AgentXSession
from shared.models.response import ChunkType, ResponseChunk


class TestSessionClassificationPromptFlow(unittest.TestCase):
    def setUp(self):
        self.root = tk.Tk()
        self.root.withdraw()
        self.temp_dir = tempfile.mkdtemp()
        self.config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "gpt-oss",
                "temperature": 0.7,
            },
            "agentix": {
                "classify_prompts": True,
                "classification_backend": "ollama",
            },
        }

    def tearDown(self):
        import os
        import shutil

        try:
            self.root.destroy()
        except Exception:
            pass

        if os.path.exists(self.temp_dir):
            shutil.rmtree(self.temp_dir)

    def test_process_prompt_classifies_submitted_prompt(self):
        captured = {}

        def fake_assemble(args, history, max_tokens, response_max_tokens=None):
            captured["classification_user"] = args.user
            captured["history_len"] = len(history)
            captured["max_tokens"] = max_tokens
            captured["response_max_tokens"] = response_max_tokens
            return {"messages": [{"role": "user", "content": "payload"}]}

        with (
            patch("agentix.bridge.classify_prompt.assemble_prompts", side_effect=fake_assemble),
            patch(
                "agentix.bridge.classify_prompt.query_classification",
                return_value={
                    "intent": "conversation",
                    "needs_clarification": False,
                    "missing_fields": [],
                    "reasoning_summary": "ok",
                    "next_step": "respond_directly",
                },
            ),
        ):
            session = AgentXSession(
                root=self.root,
                config=self.config,
                username="test_user",
                session_dir=self.temp_dir,
            )

            session.agentix_adapter.process_prompt_generator = (
                lambda *_args, **_kwargs: iter(
                    [
                        ResponseChunk(type=ChunkType.CONTENT, content="ok"),
                        ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
                    ]
                )
            )

            chunks = list(session.process_prompt("what model are you?"))

        self.assertEqual(captured["classification_user"], ["what model are you?"])
        self.assertIn("THINKING", [chunk.type.name for chunk in chunks])

    def test_classification_chunk_triggers_display_classification(self):
        """_make_classification_callback returns a callable that invokes gui.display_classification."""
        session = AgentXSession(
            root=self.root,
            config={
                **self.config,
                "agentix": {
                    **self.config["agentix"],
                    "classification_display": {"enabled": True},
                },
            },
            username="test_user",
            session_dir=self.temp_dir,
        )

        display_calls: list[dict] = []
        session.gui.display_classification = lambda meta: display_calls.append(meta)  # type: ignore[assignment]

        callback = session._make_classification_callback(session.config)
        callback({
            "intent": "simple_action",
            "next_step": "single_tool",
            "reasoning_summary": "One tool needed.",
            "needs_clarification": False,
            "missing_fields": [],
        })

        self.assertEqual(len(display_calls), 1)
        self.assertEqual(display_calls[0]["intent"], "simple_action")
        self.assertEqual(display_calls[0]["next_step"], "single_tool")

    def test_classification_display_disabled_skips_gui_call(self):
        """When classification_display.enabled is False, display_classification is never called."""
        session = AgentXSession(
            root=self.root,
            config={
                **self.config,
                "agentix": {
                    **self.config["agentix"],
                    "classification_display": {"enabled": False},
                },
            },
            username="test_user",
            session_dir=self.temp_dir,
        )

        display_calls: list = []
        session.gui.display_classification = lambda meta: display_calls.append(meta)  # type: ignore[assignment]

        callback = session._make_classification_callback(session.config)
        callback({
            "intent": "conversation",
            "next_step": "respond_directly",
            "reasoning_summary": "ok",
            "needs_clarification": False,
            "missing_fields": [],
        })

        self.assertEqual(len(display_calls), 0)
