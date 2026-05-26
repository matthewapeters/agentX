#!/usr/bin/env python
"""
Functional test for end-to-end chat workflow.

This test covers the full chat process:
1. User sends a message
2. Agentix classifies the prompt (if configured)
3. System streams response
4. Output is displayed
"""

import os
import sys
import unittest
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from agentx.session import AgentXSession
from shared.models.context import Context, Message, MessageRole
from shared.models.response import ChunkType, ResponseChunk


class TestEndToEndChat(unittest.TestCase):
    """Test complete chat workflow from user input to response."""

    def setUp(self):
        """Set up test session."""
        # Use temp directory for test session
        import tempfile

        self.test_dir = tempfile.mkdtemp()

        # Configure session for predictable testing
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
        self._adapter_patcher = None
        self._mock_adapter = None

    def tearDown(self):
        """Clean up test directory."""
        import shutil

        if os.path.exists(self.test_dir):
            shutil.rmtree(self.test_dir)
        if self._adapter_patcher:
            self._adapter_patcher.stop()

    def _create_session(self, classify_prompts: bool = False) -> AgentXSession:
        from unittest.mock import MagicMock, patch

        self.config["agentix"]["classify_prompts"] = classify_prompts
        self._adapter_patcher = patch("agentx.session.create_adapter")
        mock_create = self._adapter_patcher.start()

        self._mock_adapter = MagicMock()
        self._mock_adapter.process_prompt_generator.side_effect = lambda *_args, **_kwargs: [
            ResponseChunk(type=ChunkType.CONTENT, content="Hello"),
            ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop"),
        ]
        if classify_prompts:
            from types import SimpleNamespace

            self._mock_adapter.classify_prompt_sync.return_value = SimpleNamespace(
                reasoning_summary="ok",
                intent="conversation",
                next_step="respond_directly",
            )
        else:
            self._mock_adapter.classify_prompt_sync.return_value = None

        mock_create.return_value = self._mock_adapter

        return AgentXSession(
            username="test_user",
            session_dir=self.test_dir,
            config=self.config,
        )

    def test_simple_question(self):
        """Test asking a simple question."""
        print("\n" + "=" * 60)
        print("TEST: Simple question")
        print("=" * 60)

        self.session = self._create_session(classify_prompts=False)
        prompt = "What is 2+2?"

        # Capture response chunks
        chunks = []
        for chunk in self.session.process_prompt(prompt):
            chunks.append(chunk)
            print(f"  [{chunk.type.name}] {chunk.content[:50] if chunk.content else ''}...")

        # Verify we got chunks
        self.assertGreater(len(chunks), 0, "Should receive response chunks")

        # Verify we got content
        content_chunks = [c for c in chunks if c.content]
        self.assertGreater(len(content_chunks), 0, "Should have content chunks")

        # Verify DONE chunk
        done_chunks = [c for c in chunks if c.done_reason]
        self.assertEqual(len(done_chunks), 1, "Should have exactly one DONE chunk")

        print(f"\n✅ Test passed: {len(chunks)} chunks received")

    def test_model_identification(self):
        """Test asking about the model."""
        print("\n" + "=" * 60)
        print("TEST: Model identification")
        print("=" * 60)

        self.session = self._create_session(classify_prompts=False)
        prompt = "what model are you?"

        # Track chunks
        content_parts = []
        error_chunks = []

        for chunk in self.session.process_prompt(prompt):
            print(f"  [{chunk.type.name}]", end="")
            if chunk.content:
                print(f" {chunk.content[:50]}...", end="")
                content_parts.append(chunk.content)
            if chunk.type.name == "ERROR":
                error_chunks.append(chunk)
                print(f" ERROR: {chunk.content}")
            print()

        # Verify no errors
        self.assertEqual(len(error_chunks), 0, f"Should not have errors. Got: {[e.content for e in error_chunks]}")

        # Verify we got content
        self.assertGreater(len(content_parts), 0, "Should have content")

        full_response = "".join(content_parts)
        print(f"\nFull response ({len(full_response)} chars):")
        print(full_response[:200] + "..." if len(full_response) > 200 else full_response)
        print(f"\n✅ Test passed: No errors, got response")

    def test_simple_question_with_classification(self):
        """Test asking a simple question with Agentix classification."""
        self.session = self._create_session(classify_prompts=True)

        print("\n" + "=" * 60)
        print("TEST: Simple question with Agentix classification")
        print("=" * 60)

        prompt = "what model are you?"

        # Track chunks
        chunks_by_type = {}
        errors = []

        for chunk in self.session.process_prompt(prompt):
            chunk_type = chunk.type.name
            if chunk_type not in chunks_by_type:
                chunks_by_type[chunk_type] = []
            chunks_by_type[chunk_type].append(chunk)

            print(f"  [{chunk_type}]", end="")
            if chunk.content:
                print(f" {chunk.content[:70]}...", end="")
            if chunk.classification:
                print(f" Classification: {chunk.classification}", end="")
            print()

            if chunk_type == "ERROR":
                errors.append(chunk.content)

        print(f"\nChunks by type: {list(chunks_by_type.keys())}")
        print(f"Total chunks: {sum(len(v) for v in chunks_by_type.values())}")

        # Verify no errors
        if errors:
            print(f"\n❌ ERRORS encountered:")
            for err in errors:
                print(f"  - {err}")
            self.fail(f"Should not have errors. Got: {errors}")

        # Verify we got thinking (classification)
        self.assertIn("THINKING", chunks_by_type, "Should have THINKING chunks from classification")

        # Verify we got content
        self.assertIn("CONTENT", chunks_by_type, "Should have CONTENT chunks")

        print(f"\n✅ Test passed: Agentix classification worked, got response")


if __name__ == "__main__":
    # Run with verbose output
    unittest.main(verbosity=2)
