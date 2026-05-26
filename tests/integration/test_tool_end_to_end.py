"""
Live end-to-end integration tests for the agentix tool pipeline.

These tests require a running Ollama instance with a model that supports
tool calls (e.g. llama3.1, qwen2.5-coder, mistral-nemo).

Run with:
    pytest tests/integration/test_tool_end_to_end.py -m live -v

Skip in CI (no Ollama available):
    pytest tests/integration/ --ignore=tests/integration/test_tool_end_to_end.py
    # or simply don't pass -m live
"""

import json
import sys
from pathlib import Path
from typing import Optional
from unittest.mock import MagicMock, patch

import pytest

from agentix.bridge.bridge import AgentixBridge
from shared.models.context import Context
from shared.models.response import ChunkType, ResponseChunk

# ──────────────────────────────────────────────────────────────────────────────
# Test fixture helpers
# ──────────────────────────────────────────────────────────────────────────────


def _chunks_by_type(chunks: list[ResponseChunk], chunk_type: ChunkType) -> list[ResponseChunk]:
    return [c for c in chunks if c.type == chunk_type]


# ──────────────────────────────────────────────────────────────────────────────
# Non-live unit tests (always run in CI — no Ollama required)
# ──────────────────────────────────────────────────────────────────────────────


class TestRoundIndexPropagation:
    """Verify round_index is set on TOOL_CALL and TOOL_RESULT chunks."""

    def test_round_index_on_tool_call_chunk(self):
        chunk = ResponseChunk(
            type=ChunkType.TOOL_CALL,
            tool_name="read_file",
            tool_input={"path": "main.py"},
            tool_id="call_001",
            round_index=0,
        )
        assert chunk.round_index == 0

    def test_round_index_on_tool_result_chunk(self):
        chunk = ResponseChunk(
            type=ChunkType.TOOL_RESULT,
            tool_name="read_file",
            tool_output="file contents here",
            tool_id="call_001",
            round_index=0,
        )
        assert chunk.round_index == 0

    def test_round_index_none_by_default(self):
        chunk = ResponseChunk(type=ChunkType.CONTENT, content="hello")
        assert chunk.round_index is None

    def test_round_index_increments_across_rounds(self):
        """round_index should be 0 for first round, 1 for second, etc."""
        chunks = [
            ResponseChunk(type=ChunkType.TOOL_CALL, tool_name="f", round_index=0),
            ResponseChunk(type=ChunkType.TOOL_RESULT, tool_name="f", round_index=0),
            ResponseChunk(type=ChunkType.TOOL_CALL, tool_name="g", round_index=1),
            ResponseChunk(type=ChunkType.TOOL_RESULT, tool_name="g", round_index=1),
            ResponseChunk(type=ChunkType.CONTENT, content="done"),
            ResponseChunk(type=ChunkType.DONE),
        ]
        tool_calls = _chunks_by_type(chunks, ChunkType.TOOL_CALL)
        assert tool_calls[0].round_index == 0
        assert tool_calls[1].round_index == 1


class TestBridgeRunToolLoopRoundIndex:
    """Verify _run_tool_loop emits correct round_index on chunks."""

    def _make_bridge_with_mock(self, tool_call_rounds: int = 1) -> AgentixBridge:
        """
        Build an AgentixBridge whose _iter_llm_chunks is mocked to emit
        exactly `tool_call_rounds` rounds of tool calls before a final answer.
        """
        bridge = AgentixBridge.__new__(AgentixBridge)
        bridge._tool_impl_cache = {}
        bridge._extra_tool_schemas = []
        bridge.config = MagicMock()
        bridge.config.model = "mock-model"
        bridge.config.base_url = "http://localhost:11434"
        bridge.config.get.return_value = None

        # Build a fake echo tool
        def echo_impl(message: str) -> str:
            """Echo a message back."""
            return f"echo: {message}"

        bridge._tool_impl_cache = {"echo": echo_impl}
        bridge._extra_tool_schemas = [
            {
                "type": "function",
                "function": {
                    "name": "echo",
                    "description": "Echo a message back.",
                    "parameters": {
                        "type": "object",
                        "properties": {"message": {"type": "string"}},
                        "required": ["message"],
                    },
                },
            }
        ]

        def _mock_iter_llm_chunks(messages, tools=None, **kw):
            # On final round (tools=None) emit a CONTENT + DONE
            if tools is None:
                yield ResponseChunk(type=ChunkType.CONTENT, content="Final answer")
                yield ResponseChunk(type=ChunkType.DONE, done_reason="stop")
                return
            # Otherwise emit a tool call on the first `tool_call_rounds` rounds
            remaining = getattr(bridge, "_mock_rounds_remaining", tool_call_rounds)
            if remaining > 0:
                bridge._mock_rounds_remaining = remaining - 1
                yield ResponseChunk(
                    type=ChunkType.TOOL_CALL,
                    tool_name="echo",
                    tool_input={"message": "hello"},
                    tool_id=f"call_{tool_call_rounds - remaining}",
                )
                yield ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")
            else:
                yield ResponseChunk(type=ChunkType.CONTENT, content="Final answer")
                yield ResponseChunk(type=ChunkType.DONE, done_reason="stop")

        bridge._iter_llm_chunks = _mock_iter_llm_chunks
        # Stub out methods that _run_tool_loop calls internally
        bridge.get_available_tools = lambda: bridge._extra_tool_schemas
        bridge._context_to_history = lambda ctx: []
        return bridge

    def test_first_round_is_zero(self):
        bridge = self._make_bridge_with_mock(tool_call_rounds=1)
        chunks = list(
            bridge._run_tool_loop(
                prompt="call echo",
                context=Context(),
                max_rounds=3,
            )
        )
        tool_calls = _chunks_by_type(chunks, ChunkType.TOOL_CALL)
        assert tool_calls, "Expected at least one TOOL_CALL chunk"
        assert tool_calls[0].round_index == 0

    def test_tool_result_same_round_as_call(self):
        bridge = self._make_bridge_with_mock(tool_call_rounds=1)
        chunks = list(
            bridge._run_tool_loop(
                prompt="call echo",
                context=Context(),
                max_rounds=3,
            )
        )
        calls = _chunks_by_type(chunks, ChunkType.TOOL_CALL)
        results = _chunks_by_type(chunks, ChunkType.TOOL_RESULT)
        assert calls and results
        assert calls[0].round_index == results[0].round_index

    def test_second_round_index_is_one(self):
        bridge = self._make_bridge_with_mock(tool_call_rounds=2)
        chunks = list(
            bridge._run_tool_loop(
                prompt="call echo twice",
                context=Context(),
                max_rounds=5,
            )
        )
        tool_calls = _chunks_by_type(chunks, ChunkType.TOOL_CALL)
        if len(tool_calls) >= 2:
            assert tool_calls[1].round_index == 1


# ──────────────────────────────────────────────────────────────────────────────
# Live tests (require Ollama — skipped in CI)
# ──────────────────────────────────────────────────────────────────────────────


@pytest.mark.live
class TestLiveToolCallEndToEnd:
    """
    End-to-end tests against a live Ollama instance.

    Require:
        - Ollama running at http://localhost:11434
        - A tool-capable model pulled (llama3.1, qwen2.5-coder, etc.)

    Usage:
        pytest tests/integration/test_tool_end_to_end.py -m live -v \\
               --model llama3.1
    """

    @pytest.fixture(autouse=True)
    def adapter(self, request):
        try:
            from agentix.integration.agentix_bridge_adapter import AgentixBridgeAdapter

            self._AgentixBridgeAdapter = AgentixBridgeAdapter
        except ImportError:
            pytest.skip("AgentixBridgeAdapter not importable — check sys.path")
        model = request.config.getoption("--model", default="llama3.1")
        try:
            self.adapter = self._AgentixBridgeAdapter(model=model)
        except Exception as e:
            pytest.skip(f"Could not connect to Ollama: {e}")

    def _collect(self, prompt: str) -> list[ResponseChunk]:
        ctx = Context()
        return list(self.adapter.process_prompt_generator(prompt, ctx))

    def test_file_read_produces_tool_call_and_result(self, tmp_path):
        """Agent should call read_file when asked to read a file."""
        target = tmp_path / "hello.txt"
        target.write_text("Hello from test!")

        chunks = self._collect(f"Read the file at {target} and tell me its contents.")

        tool_calls = _chunks_by_type(chunks, ChunkType.TOOL_CALL)
        tool_results = _chunks_by_type(chunks, ChunkType.TOOL_RESULT)
        content = _chunks_by_type(chunks, ChunkType.CONTENT)
        done = _chunks_by_type(chunks, ChunkType.DONE)

        assert tool_calls, "Expected at least one TOOL_CALL chunk"
        assert tool_results, "Expected at least one TOOL_RESULT chunk"
        assert content, "Expected content chunk with final answer"
        assert done, "Expected DONE chunk"

    def test_tool_call_has_round_index(self, tmp_path):
        """TOOL_CALL chunks emitted by the live bridge must carry round_index."""
        target = tmp_path / "data.txt"
        target.write_text("test data")

        chunks = self._collect(f"Read {target}")
        tool_calls = _chunks_by_type(chunks, ChunkType.TOOL_CALL)

        assert tool_calls, "Expected TOOL_CALL"
        for tc in tool_calls:
            assert tc.round_index is not None, "round_index must be set on live TOOL_CALL chunk"
            assert tc.round_index >= 0

    def test_tool_result_has_round_index(self, tmp_path):
        """TOOL_RESULT chunks emitted by the live bridge must carry round_index."""
        target = tmp_path / "data.txt"
        target.write_text("test data")

        chunks = self._collect(f"Read {target}")
        tool_results = _chunks_by_type(chunks, ChunkType.TOOL_RESULT)

        assert tool_results, "Expected TOOL_RESULT"
        for tr in tool_results:
            assert tr.round_index is not None, "round_index must be set on live TOOL_RESULT chunk"

    def test_call_and_result_share_round_index(self, tmp_path):
        """TOOL_CALL and TOOL_RESULT for the same round must share round_index."""
        target = tmp_path / "abc.txt"
        target.write_text("abc")

        chunks = self._collect(f"Read {target}")
        call_rounds = {c.round_index for c in _chunks_by_type(chunks, ChunkType.TOOL_CALL)}
        result_rounds = {c.round_index for c in _chunks_by_type(chunks, ChunkType.TOOL_RESULT)}

        # Each TOOL_RESULT round should match a TOOL_CALL round
        assert result_rounds.issubset(
            call_rounds
        ), f"TOOL_RESULT round indices {result_rounds} not all present in TOOL_CALL rounds {call_rounds}"

    def test_done_chunk_emitted_after_tool_loop(self, tmp_path):
        """A DONE chunk must always follow the agentic loop."""
        target = tmp_path / "x.txt"
        target.write_text("x")

        chunks = self._collect(f"Read {target}")
        assert chunks[-1].type == ChunkType.DONE, "Last chunk must be DONE"

    def test_list_directory_tool_call(self, tmp_path):
        """Agent should use list_directory when asked to list files."""
        (tmp_path / "a.txt").write_text("a")
        (tmp_path / "b.txt").write_text("b")

        chunks = self._collect(f"List the files in {tmp_path}")
        tool_calls = _chunks_by_type(chunks, ChunkType.TOOL_CALL)
        assert tool_calls, "Expected a tool call for listing files"


def pytest_addoption(parser):
    """Add --model CLI option for live tests."""
    try:
        parser.addoption("--model", default="llama3.1", help="Ollama model to use for live tests")
    except ValueError:
        pass  # already added by another conftest
