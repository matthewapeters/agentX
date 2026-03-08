"""
Tests for Ollama streaming tool-call detection in AgentixBridge.

Covers Phase 2 (stream detection) and Phase 3.1 (execute_tool) of the
tool usage plan (docs/tool_usage_plan.md).

All tests use recorded/mocked Ollama responses — no live service required.
Tests that DO need a live Ollama instance are marked @pytest.mark.live.
"""

from __future__ import annotations

import json
import unittest
from unittest.mock import MagicMock, patch

from agentix.agentix_config import AgentixConfig
from agentix.bridge.bridge import AgentixBridge
from shared.models.context import Context
from shared.models.response import ChunkType, ResponseChunk


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_bridge() -> AgentixBridge:
    config = AgentixConfig(model="test-model", tools=[], classify_prompts=False)
    return AgentixBridge(config)


def _make_context() -> Context:
    ctx = MagicMock(spec=Context)
    ctx.get_enabled_messages.return_value = []
    return ctx


def _openai_content_chunk(text: str, finish: str | None = None) -> dict:
    """Build an OpenAI-compat streaming content delta."""
    choice: dict = {"delta": {"content": text}}
    if finish:
        choice["finish_reason"] = finish
    return {"choices": [choice]}


def _openai_tool_call_start(index: int, call_id: str, name: str, args_fragment: str = "") -> dict:
    """First delta for a tool call — includes id and function name."""
    return {
        "choices": [{
            "delta": {
                "tool_calls": [{
                    "index": index,
                    "id": call_id,
                    "type": "function",
                    "function": {"name": name, "arguments": args_fragment},
                }]
            },
            "finish_reason": None,
        }]
    }


def _openai_tool_call_args(index: int, args_fragment: str) -> dict:
    """Subsequent delta with more argument characters."""
    return {
        "choices": [{
            "delta": {
                "tool_calls": [{"index": index, "function": {"arguments": args_fragment}}]
            },
            "finish_reason": None,
        }]
    }


def _openai_finish_tool_calls() -> dict:
    """Final delta signalling tool_calls completion."""
    return {"choices": [{"delta": {}, "finish_reason": "tool_calls"}]}


def _openai_finish_stop(text: str = "") -> dict:
    choice: dict = {"delta": {"content": text}, "finish_reason": "stop"}
    return {"choices": [choice]}


# ---------------------------------------------------------------------------
# Phase 2 — Tool call detection in streaming
# ---------------------------------------------------------------------------

class TestToolCallDetectionInStream(unittest.TestCase):

    def _collect_chunks(self, raw_chunks: list[dict]) -> list[ResponseChunk]:
        bridge = _make_bridge()
        with patch("agentix.bridge.bridge.query_api_streaming", return_value=iter(raw_chunks)):
            return list(bridge._iter_llm_chunks([{"role": "user", "content": "hi"}]))

    def test_plain_content_stream_no_tool_calls(self):
        """Content-only stream produces CONTENT chunks and a DONE chunk."""
        raw = [
            _openai_content_chunk("Hello "),
            _openai_content_chunk("world"),
            _openai_finish_stop(),
        ]
        chunks = self._collect_chunks(raw)
        types = [c.type for c in chunks]
        self.assertIn(ChunkType.CONTENT, types)
        self.assertIn(ChunkType.DONE, types)
        self.assertNotIn(ChunkType.TOOL_CALL, types)
        content = "".join(c.content for c in chunks if c.type == ChunkType.CONTENT)
        self.assertEqual(content, "Hello world")

    def test_single_tool_call_detected(self):
        """A tool call spread across multiple deltas is emitted as one TOOL_CALL chunk."""
        raw = [
            _openai_tool_call_start(0, "call_abc", "read_file", '{"path":'),
            _openai_tool_call_args(0, ' "/tmp/x.txt"}'),
            _openai_finish_tool_calls(),
        ]
        chunks = self._collect_chunks(raw)
        tool_chunks = [c for c in chunks if c.type == ChunkType.TOOL_CALL]
        self.assertEqual(len(tool_chunks), 1)
        tc = tool_chunks[0]
        self.assertEqual(tc.tool_name, "read_file")
        self.assertEqual(tc.tool_input, {"path": "/tmp/x.txt"})
        self.assertEqual(tc.tool_id, "call_abc")

    def test_multiple_tool_calls_in_one_round(self):
        """Two parallel tool calls are both emitted."""
        raw = [
            _openai_tool_call_start(0, "call_1", "add", '{"a": 1,'),
            _openai_tool_call_start(1, "call_2", "multiply", '{"a": 3,'),
            _openai_tool_call_args(0, ' "b": 2}'),
            _openai_tool_call_args(1, ' "b": 4}'),
            _openai_finish_tool_calls(),
        ]
        chunks = self._collect_chunks(raw)
        tool_chunks = [c for c in chunks if c.type == ChunkType.TOOL_CALL]
        self.assertEqual(len(tool_chunks), 2)
        names = {tc.tool_name for tc in tool_chunks}
        self.assertEqual(names, {"add", "multiply"})

    def test_tool_call_with_malformed_json_args_degrades(self):
        """Malformed JSON arguments are stored as _raw rather than crashing."""
        raw = [
            _openai_tool_call_start(0, "call_x", "broken_tool", "not-json"),
            _openai_finish_tool_calls(),
        ]
        chunks = self._collect_chunks(raw)
        tool_chunks = [c for c in chunks if c.type == ChunkType.TOOL_CALL]
        self.assertEqual(len(tool_chunks), 1)
        self.assertIn("_raw", tool_chunks[0].tool_input)

    def test_thinking_chunks_emitted(self):
        """Reasoning/thinking deltas are emitted as THINKING chunks."""
        raw = [
            {"choices": [{"delta": {"reasoning": "Let me think..."}, "finish_reason": None}]},
            _openai_finish_stop("answer"),
        ]
        chunks = self._collect_chunks(raw)
        thinking = [c for c in chunks if c.type == ChunkType.THINKING]
        self.assertTrue(len(thinking) > 0)
        self.assertIn("Let me think", thinking[0].content)

    def test_api_error_yields_error_chunk(self):
        """A chunk with an error key yields an ERROR chunk."""
        raw = [{"error": "model not found"}]
        chunks = self._collect_chunks(raw)
        self.assertEqual(chunks[0].type, ChunkType.ERROR)
        self.assertIn("model not found", chunks[0].content)


# ---------------------------------------------------------------------------
# Phase 3.1 — execute_tool
# ---------------------------------------------------------------------------

class TestExecuteTool(unittest.TestCase):

    def setUp(self):
        self.bridge = _make_bridge()

    def test_execute_known_tool_succeeds(self):
        """execute_tool calls a registered implementation and returns success."""
        def my_tool(x: int, y: int) -> int:
            return x + y

        self.bridge._tool_impl_cache = {"my_tool": my_tool}
        result = self.bridge.execute_tool("my_tool", {"x": 3, "y": 4})
        self.assertTrue(result.success)
        self.assertEqual(result.output, 7)

    def test_execute_unknown_tool_returns_error(self):
        """Unknown tool name returns error response without raising."""
        self.bridge._tool_impl_cache = {}
        result = self.bridge.execute_tool("nonexistent", {})
        self.assertFalse(result.success)
        self.assertIn("nonexistent", result.error)

    def test_execute_tool_exception_returns_error(self):
        """If the tool implementation raises, the error is caught and returned."""
        def exploding_tool(**kwargs):
            raise RuntimeError("boom")

        self.bridge._tool_impl_cache = {"exploding_tool": exploding_tool}
        result = self.bridge.execute_tool("exploding_tool", {})
        self.assertFalse(result.success)
        self.assertIn("boom", result.error)

    def test_execute_tool_bad_args_returns_error(self):
        """Wrong argument names result in a TypeError caught as error response."""
        def strict(a: int, b: int) -> int:
            return a + b

        self.bridge._tool_impl_cache = {"strict": strict}
        result = self.bridge.execute_tool("strict", {"wrong_param": 1})
        self.assertFalse(result.success)

    def test_execute_tool_preserves_tool_id(self):
        """The tool_id is stored in the response request_id for correlation."""
        def echo(msg: str) -> str:
            return msg

        self.bridge._tool_impl_cache = {"echo": echo}
        result = self.bridge.execute_tool("echo", {"msg": "hi"}, tool_id="call_42")
        self.assertTrue(result.success)
        self.assertEqual(result.request_id, "call_42")


# ---------------------------------------------------------------------------
# Phase 3.3 — _stream_tool_response routes through _run_tool_loop
# ---------------------------------------------------------------------------

class TestStreamToolResponse(unittest.TestCase):

    def test_stream_tool_response_with_no_tool_calls_falls_back_to_content(self):
        """When the LLM returns no tool calls, content reaches the caller."""
        bridge = _make_bridge()
        ctx = _make_context()
        classification = MagicMock()

        raw = [
            _openai_content_chunk("Direct answer"),
            _openai_finish_stop(),
        ]
        with patch("agentix.bridge.bridge.query_api_streaming", return_value=iter(raw)):
            bridge._tool_impl_cache = {}
            chunks = list(bridge._stream_tool_response("Q?", ctx, classification))

        content = "".join(c.content for c in chunks if c.type == ChunkType.CONTENT)
        self.assertIn("Direct answer", content)

    def test_stream_tool_response_executes_tool_and_continues(self):
        """When LLM calls a tool, the result is fed back and a final answer follows."""
        bridge = _make_bridge()
        ctx = _make_context()
        classification = MagicMock()

        # Round 1: LLM calls add(1, 2)
        round1 = [
            _openai_tool_call_start(0, "call_1", "add", '{"a": 1, "b": 2}'),
            _openai_finish_tool_calls(),
        ]
        # Round 2: LLM gives final answer after receiving tool result
        round2 = [
            _openai_content_chunk("The answer is 3"),
            _openai_finish_stop(),
        ]

        def add(a: int, b: int) -> int:
            return a + b

        bridge._tool_impl_cache = {"add": add}
        bridge.config.tools = []

        call_count = [0]
        def fake_streaming(config, payload):
            call_count[0] += 1
            if call_count[0] == 1:
                return iter(round1)
            return iter(round2)

        with patch("agentix.bridge.bridge.query_api_streaming", side_effect=fake_streaming):
            chunks = list(bridge._stream_tool_response("What is 1+2?", ctx, classification))

        tool_calls = [c for c in chunks if c.type == ChunkType.TOOL_CALL]
        tool_results = [c for c in chunks if c.type == ChunkType.TOOL_RESULT]
        content_chunks = [c for c in chunks if c.type == ChunkType.CONTENT]

        self.assertEqual(len(tool_calls), 1)
        self.assertEqual(tool_calls[0].tool_name, "add")
        self.assertEqual(len(tool_results), 1)
        self.assertEqual(tool_results[0].tool_output, 3)
        self.assertTrue(any("answer is 3" in c.content for c in content_chunks))


if __name__ == "__main__":
    unittest.main()
