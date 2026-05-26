"""
Unit tests for the AgentixBridge agentic tool loop (_run_tool_loop).

All tests use mocked _iter_llm_chunks and execute_tool so no live Ollama
or Agentix service is required.  These tests verify the multi-turn
feedback loop behaviour described in docs/tool_usage_plan.md Phase 4.
"""

import os
import sys
from pathlib import Path
from unittest.mock import MagicMock, patch

project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from agentix.agentix_config import AgentixConfig
from agentix.bridge.bridge import AgentixBridge
from shared.models.context import Context
from shared.models.response import ChunkType, ResponseChunk
from shared.models.tools import ToolResponse

# ─── Helpers ────────────────────────────────────────────────────────────────


def _make_bridge(tools: dict[str, callable] | None = None) -> AgentixBridge:
    """Create a bridge with optional tool implementations pre-registered."""
    config = AgentixConfig(model="test-model", tools=[], debug=False)
    bridge = AgentixBridge(config)
    if tools:
        schemas = [
            {
                "type": "function",
                "function": {
                    "name": name,
                    "description": f"Test tool {name}",
                    "parameters": {"type": "object", "properties": {}},
                },
            }
            for name in tools
        ]
        bridge.register_tool_implementations(tools, schemas)
    return bridge


def _make_context() -> Context:
    return Context()


def _content_chunks(*texts: str) -> list[ResponseChunk]:
    chunks = [ResponseChunk(type=ChunkType.CONTENT, content=t) for t in texts]
    chunks.append(ResponseChunk(type=ChunkType.DONE, done_reason="stop"))
    return chunks


def _tool_call_chunk(name: str, args: dict, tool_id: str = "call_1") -> ResponseChunk:
    return ResponseChunk(
        type=ChunkType.TOOL_CALL,
        tool_name=name,
        tool_input=args,
        tool_id=tool_id,
    )


def _finish_tool_calls_then_content(tool_chunks: list[ResponseChunk], content: str) -> list[ResponseChunk]:
    """First yield finishes with tool calls, second yields text content."""
    round1 = tool_chunks + [ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")]
    round2 = _content_chunks(content)
    return round1, round2


def _collect(chunks) -> list[ResponseChunk]:
    return list(chunks)


# ─── Tests: No tool calls ────────────────────────────────────────────────────


class TestNoToolCalls:
    """Loop exits on first round when LLM answers directly."""

    def test_direct_content_response(self):
        bridge = _make_bridge()
        ctx = _make_context()

        with patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_chunks("Hello!"))):
            chunks = _collect(bridge._run_tool_loop("Hi", ctx, max_rounds=5))

        content = [c for c in chunks if c.type == ChunkType.CONTENT]
        done = [c for c in chunks if c.type == ChunkType.DONE]
        assert any("Hello!" in c.content for c in content)
        assert len(done) >= 1

    def test_thinking_chunk_passed_through(self):
        bridge = _make_bridge()
        ctx = _make_context()
        response = [
            ResponseChunk(type=ChunkType.THINKING, content="reasoning…"),
            ResponseChunk(type=ChunkType.CONTENT, content="Answer"),
            ResponseChunk(type=ChunkType.DONE, done_reason="stop"),
        ]
        with patch.object(bridge, "_iter_llm_chunks", return_value=iter(response)):
            chunks = _collect(bridge._run_tool_loop("Q", ctx))

        thinking = [c for c in chunks if c.type == ChunkType.THINKING]
        assert len(thinking) == 1
        assert thinking[0].content == "reasoning…"

    def test_max_rounds_zero_still_produces_synthesis(self):
        """max_rounds=0 means skip the tool loop entirely and go straight to synthesis."""
        bridge = _make_bridge()
        ctx = _make_context()
        # With max_rounds=0 range(0) is empty; since no tools were called,
        # the generator just yields a DONE chunk after the synthesis guard.
        with patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_chunks("Direct"))):
            chunks = _collect(bridge._run_tool_loop("Q", ctx, max_rounds=0))

        done = [c for c in chunks if c.type == ChunkType.DONE]
        assert len(done) >= 1


# ─── Tests: Single tool call round ──────────────────────────────────────────


class TestSingleToolCall:
    """Loop executes one tool then gets final answer."""

    def _build_bridge_with_echo(self) -> tuple[AgentixBridge, list]:
        calls = []

        def echo_tool(message: str = "") -> str:
            calls.append(message)
            return f"echo:{message}"

        bridge = _make_bridge({"echo": echo_tool})
        return bridge, calls

    def _two_round_side_effect(self, tool_chunk, final_text):
        """Returns a side_effect callable for _iter_llm_chunks that serves two rounds."""
        round1 = [tool_chunk, ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")]
        round2 = _content_chunks(final_text)
        responses = iter([round1, round2])

        def side_effect(messages, tools=None):
            return iter(next(responses))

        return side_effect

    def test_tool_executed_once(self):
        bridge, calls = self._build_bridge_with_echo()
        ctx = _make_context()
        tc = _tool_call_chunk("echo", {"message": "hi"})

        with patch.object(
            bridge,
            "_iter_llm_chunks",
            side_effect=self._two_round_side_effect(tc, "Done"),
        ):
            _collect(bridge._run_tool_loop("Prompt", ctx))

        assert calls == ["hi"]

    def test_tool_result_chunk_yielded(self):
        bridge, _ = self._build_bridge_with_echo()
        ctx = _make_context()
        tc = _tool_call_chunk("echo", {"message": "x"})

        with patch.object(
            bridge,
            "_iter_llm_chunks",
            side_effect=self._two_round_side_effect(tc, "Final"),
        ):
            chunks = _collect(bridge._run_tool_loop("Prompt", ctx))

        tool_results = [c for c in chunks if c.type == ChunkType.TOOL_RESULT]
        assert len(tool_results) == 1
        assert "echo:x" in (tool_results[0].tool_output or "")

    def test_tool_call_chunk_yielded(self):
        bridge, _ = self._build_bridge_with_echo()
        ctx = _make_context()
        tc = _tool_call_chunk("echo", {"message": "x"}, tool_id="call_abc")

        with patch.object(
            bridge,
            "_iter_llm_chunks",
            side_effect=self._two_round_side_effect(tc, "Final"),
        ):
            chunks = _collect(bridge._run_tool_loop("Prompt", ctx))

        tool_calls = [c for c in chunks if c.type == ChunkType.TOOL_CALL]
        assert len(tool_calls) == 1
        assert tool_calls[0].tool_name == "echo"
        assert tool_calls[0].tool_id == "call_abc"

    def test_final_content_after_tool(self):
        bridge, _ = self._build_bridge_with_echo()
        ctx = _make_context()
        tc = _tool_call_chunk("echo", {"message": "ping"})

        with patch.object(
            bridge,
            "_iter_llm_chunks",
            side_effect=self._two_round_side_effect(tc, "Pong response"),
        ):
            chunks = _collect(bridge._run_tool_loop("Prompt", ctx))

        content = [c for c in chunks if c.type == ChunkType.CONTENT]
        assert any("Pong response" in c.content for c in content)

    def test_unknown_tool_returns_error_and_loop_continues(self):
        bridge = _make_bridge()  # no tools registered
        ctx = _make_context()
        tc = _tool_call_chunk("nonexistent", {})
        responses = [
            [tc, ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")],
            _content_chunks("I couldn't use that tool."),
        ]
        idx = [0]

        def side_effect(messages, tools=None):
            result = responses[idx[0]]
            idx[0] += 1
            return iter(result)

        with patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect):
            chunks = _collect(bridge._run_tool_loop("Prompt", ctx))

        tool_results = [c for c in chunks if c.type == ChunkType.TOOL_RESULT]
        assert len(tool_results) == 1
        assert "unknown" in (tool_results[0].tool_output or "").lower()

        content = [c for c in chunks if c.type == ChunkType.CONTENT]
        assert len(content) > 0  # loop continued to produce an answer


# ─── Tests: Tool execution errors ────────────────────────────────────────────


class TestToolErrors:
    """Error-as-result: tool failure never crashes the loop."""

    def test_exception_in_tool_becomes_result(self):
        def boom(**kwargs) -> str:
            raise RuntimeError("disk full")

        bridge = _make_bridge({"boom": boom})
        ctx = _make_context()
        tc = _tool_call_chunk("boom", {})
        responses = [
            [tc, ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")],
            _content_chunks("Sorry, error occurred."),
        ]
        idx = [0]

        def side_effect(messages, tools=None):
            result = responses[idx[0]]
            idx[0] += 1
            return iter(result)

        with patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect):
            chunks = _collect(bridge._run_tool_loop("Prompt", ctx))

        tool_results = [c for c in chunks if c.type == ChunkType.TOOL_RESULT]
        assert len(tool_results) == 1
        output = tool_results[0].tool_output or ""
        assert "disk full" in output.lower() or "error" in output.lower()

        # Loop did not crash and did produce a final answer
        done = [c for c in chunks if c.type == ChunkType.DONE]
        assert len(done) >= 1


# ─── Tests: Multi-round loop ─────────────────────────────────────────────────


class TestMultiRoundLoop:
    """Loop can chain multiple rounds of tool calls."""

    def test_two_rounds_of_tool_calls(self):
        step_log = []

        def step_a() -> str:
            step_log.append("a")
            return "result_a"

        def step_b() -> str:
            step_log.append("b")
            return "result_b"

        bridge = _make_bridge({"step_a": step_a, "step_b": step_b})
        ctx = _make_context()

        responses = [
            # Round 1: call step_a
            [
                _tool_call_chunk("step_a", {}, "call_1"),
                ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls"),
            ],
            # Round 2: call step_b
            [
                _tool_call_chunk("step_b", {}, "call_2"),
                ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls"),
            ],
            # Round 3: final answer
            _content_chunks("Both steps done."),
        ]
        idx = [0]

        def side_effect(messages, tools=None):
            result = responses[idx[0]]
            idx[0] += 1
            return iter(result)

        with patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect):
            chunks = _collect(bridge._run_tool_loop("Prompt", ctx, max_rounds=5))

        assert step_log == ["a", "b"]
        content = [c for c in chunks if c.type == ChunkType.CONTENT]
        assert any("Both steps done" in c.content for c in content)

    def test_max_rounds_triggers_synthesis(self):
        """When max_rounds is exhausted, a synthesis step runs without tools."""
        always_tool_calls = 0

        def loop_forever() -> str:
            nonlocal always_tool_calls
            always_tool_calls += 1
            return "partial"

        bridge = _make_bridge({"loop_forever": loop_forever})
        ctx = _make_context()

        def side_effect(messages, tools=None):
            if tools:
                # Still in the loop: return a tool call
                return iter(
                    [
                        _tool_call_chunk("loop_forever", {}, f"call_{always_tool_calls}"),
                        ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls"),
                    ]
                )
            else:
                # Synthesis round (tools=None): return a text answer
                return iter(_content_chunks("Synthesis answer."))

        with patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect):
            chunks = _collect(bridge._run_tool_loop("Prompt", ctx, max_rounds=3))

        # Tool was called max_rounds times
        assert always_tool_calls == 3

        # Synthesis produced content
        content = [c for c in chunks if c.type == ChunkType.CONTENT]
        assert any("Synthesis answer" in c.content for c in content)


# ─── Tests: round_index propagation ─────────────────────────────────────────


class TestRoundIndexPropagation:
    """TOOL_CALL and TOOL_RESULT chunks carry correct round_index."""

    def test_round_index_on_tool_call_and_result(self):
        def noop() -> str:
            return "ok"

        bridge = _make_bridge({"noop": noop})
        ctx = _make_context()

        responses = [
            [
                _tool_call_chunk("noop", {}, "id1"),
                ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls"),
            ],
            _content_chunks("Done."),
        ]
        idx = [0]

        def side_effect(messages, tools=None):
            result = responses[idx[0]]
            idx[0] += 1
            return iter(result)

        with patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect):
            chunks = _collect(bridge._run_tool_loop("Prompt", ctx))

        tool_calls = [c for c in chunks if c.type == ChunkType.TOOL_CALL]
        tool_results = [c for c in chunks if c.type == ChunkType.TOOL_RESULT]

        assert len(tool_calls) == 1
        assert len(tool_results) == 1
        # Both should be round 0 (first loop iteration)
        assert tool_calls[0].round_index == 0
        assert tool_results[0].round_index == 0

    def test_round_index_increments_across_rounds(self):
        call_count = [0]

        def counter() -> str:
            call_count[0] += 1
            return f"count={call_count[0]}"

        bridge = _make_bridge({"counter": counter})
        ctx = _make_context()

        responses = [
            [_tool_call_chunk("counter", {}, "id0"), ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")],
            [_tool_call_chunk("counter", {}, "id1"), ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")],
            _content_chunks("Final."),
        ]
        idx = [0]

        def side_effect(messages, tools=None):
            result = responses[idx[0]]
            idx[0] += 1
            return iter(result)

        with patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect):
            chunks = _collect(bridge._run_tool_loop("Prompt", ctx, max_rounds=5))

        tool_calls = [c for c in chunks if c.type == ChunkType.TOOL_CALL]
        assert len(tool_calls) == 2
        assert tool_calls[0].round_index == 0
        assert tool_calls[1].round_index == 1
