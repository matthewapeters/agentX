"""
Phase 2 unit tests: hierarchical task execution.

Tests cover:
- _extract_plan_json: valid JSON, embedded JSON, bad input
- _create_plan: successful parse → PlanRecord; fallback on bad LLM output
- _run_task_node: no-tool (pure content); regular tool call; run_subtask recursion
- Depth cap: run_subtask excluded from tool list at max_task_depth
- _stream_planned_response end-to-end: PLAN_START → TASK_NODE_* → DONE; fallback mode
"""

from __future__ import annotations

import json
from typing import Iterator
from unittest.mock import MagicMock, patch

import pytest

from agentix.bridge.bridge import AgentixBridge, run_subtask
from agentix.agentix_config import AgentixConfig
from shared.models.response import ChunkType, ResponseChunk
from shared.models.context import Context
from shared.models.task_node import PlanRecord, PlanStep, TaskNodeRecord, TaskTree

# ── Helpers ────────────────────────────────────────────────────────────────────


def make_bridge(max_task_depth: int = 10, max_tool_rounds: int = 5) -> AgentixBridge:
    config = AgentixConfig(
        model="llama3.2",
        max_task_depth=max_task_depth,
        max_tool_rounds=max_tool_rounds,
    )
    return AgentixBridge(config)


def make_context(tmp_path) -> Context:
    """Return a Context backed by a temp dir (path = <tmp>/context so session root = tmp)."""
    import os

    ctx_dir = os.path.join(str(tmp_path), "context")
    os.makedirs(ctx_dir, exist_ok=True)
    ctx = Context(session_id="test-session", path=ctx_dir)
    return ctx


def content_chunk(text: str) -> ResponseChunk:
    return ResponseChunk(type=ChunkType.CONTENT, content=text)


def done_chunk() -> ResponseChunk:
    return ResponseChunk(type=ChunkType.DONE, done_reason="stop")


def collect(gen: Iterator[ResponseChunk]) -> list[ResponseChunk]:
    return list(gen)


# ── _extract_plan_json ─────────────────────────────────────────────────────────


class TestExtractPlanJson:
    def test_direct_json(self):
        data = {"plan_name": "Foo", "steps": []}
        result = AgentixBridge._extract_plan_json(json.dumps(data))
        assert result == data

    def test_json_embedded_in_markdown(self):
        raw = 'Here is your plan:\n```json\n{"plan_name": "Bar", "steps": []}\n```'
        result = AgentixBridge._extract_plan_json(raw)
        assert result["plan_name"] == "Bar"

    def test_missing_json_raises(self):
        with pytest.raises(ValueError):
            AgentixBridge._extract_plan_json("No JSON here at all.")

    def test_invalid_json_raises(self):
        with pytest.raises((ValueError, json.JSONDecodeError)):
            AgentixBridge._extract_plan_json("{bad json!!}")


# ── _create_plan ───────────────────────────────────────────────────────────────


class TestCreatePlan:
    def _plan_json(self) -> str:
        return json.dumps(
            {
                "plan_name": "Test Plan",
                "steps": [
                    {"id": "step_0", "description": "Do first thing", "tbd": False, "depends_on": []},
                    {"id": "step_1", "description": "Do second thing", "tbd": False, "depends_on": ["step_0"]},
                ],
            }
        )

    def test_successful_plan_creation(self, tmp_path):
        bridge = make_bridge()
        ctx = make_context(tmp_path)

        with (
            patch.object(bridge, "_load_prompt_file", return_value="System prompt text"),
            patch.object(bridge, "_context_to_history", return_value=[]),
            patch.object(
                bridge,
                "_iter_llm_chunks",
                return_value=iter([content_chunk(self._plan_json()), done_chunk()]),
            ),
        ):
            plan, tree = bridge._create_plan("Do stuff", ctx)

        assert plan is not None
        assert isinstance(plan, PlanRecord)
        assert plan.plan_name == "Test Plan"
        assert len(plan.steps) == 2
        assert plan.steps[0].step_id == "step_0"
        assert plan.steps[1].depends_on == ["step_0"]
        assert tree is not None
        assert plan.plan_id in tree.plans

    def test_returns_none_when_prompt_file_missing(self, tmp_path):
        bridge = make_bridge()
        ctx = make_context(tmp_path)

        with patch.object(bridge, "_load_prompt_file", return_value=None):
            plan, tree = bridge._create_plan("Do stuff", ctx)

        assert plan is None
        assert tree is None

    def test_returns_none_on_bad_json(self, tmp_path):
        bridge = make_bridge()
        ctx = make_context(tmp_path)

        with (
            patch.object(bridge, "_load_prompt_file", return_value="Prompt"),
            patch.object(bridge, "_context_to_history", return_value=[]),
            patch.object(
                bridge,
                "_iter_llm_chunks",
                return_value=iter([content_chunk("not json at all"), done_chunk()]),
            ),
        ):
            plan, tree = bridge._create_plan("Do stuff", ctx)

        assert plan is None
        assert tree is None

    def test_returns_none_on_empty_steps(self, tmp_path):
        bridge = make_bridge()
        ctx = make_context(tmp_path)

        empty_plan_json = json.dumps({"plan_name": "Empty", "steps": []})
        with (
            patch.object(bridge, "_load_prompt_file", return_value="Prompt"),
            patch.object(bridge, "_context_to_history", return_value=[]),
            patch.object(
                bridge,
                "_iter_llm_chunks",
                return_value=iter([content_chunk(empty_plan_json), done_chunk()]),
            ),
        ):
            plan, tree = bridge._create_plan("Do stuff", ctx)

        assert plan is None


# ── _run_task_node ─────────────────────────────────────────────────────────────


class TestRunTaskNode:
    def _make_tree(self, plan_id: str) -> TaskTree:
        tree = TaskTree(session_id="test-session")
        plan = PlanRecord(
            plan_id=plan_id,
            plan_name="P",
            steps=[
                PlanStep(step_id="s0", description="step 0"),
            ],
        )
        tree.add_plan(plan)
        return tree

    def test_pure_content_node(self, tmp_path):
        """Node with no tool calls emits START → CONTENT → END."""
        bridge = make_bridge()
        ctx = make_context(tmp_path)
        tree = self._make_tree("plan_x")

        with (
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(
                bridge,
                "_iter_llm_chunks",
                return_value=iter([content_chunk("The answer is 42."), done_chunk()]),
            ),
        ):
            chunks = collect(
                bridge._run_task_node(
                    plan_id="plan_x",
                    task_id="s0",
                    task_description="What is 6×7?",
                    context=ctx,
                    task_tree=tree,
                    initial_messages=[],
                )
            )

        types = [c.type for c in chunks]
        assert ChunkType.TASK_NODE_START in types
        assert ChunkType.CONTENT in types
        assert ChunkType.TASK_NODE_END in types
        end = next(c for c in chunks if c.type == ChunkType.TASK_NODE_END)
        assert "42" in (end.content or "")

    def test_regular_tool_call_node(self, tmp_path):
        """Node that calls a regular tool emits TOOL_CALL and TOOL_RESULT."""
        from shared.models.tools import ToolResponse

        bridge = make_bridge()
        ctx = make_context(tmp_path)
        tree = self._make_tree("plan_y")

        tool_call_chunk = ResponseChunk(
            type=ChunkType.TOOL_CALL,
            tool_name="read_file",
            tool_input={"path": "/tmp/test.txt"},
            tool_id="call_001",
        )
        # First iter call: returns a tool call; second: returns content
        iter_responses = [
            iter([tool_call_chunk, done_chunk()]),
            iter([content_chunk("File content: hello world"), done_chunk()]),
        ]
        mock_tool_result = ToolResponse(success=True, output="hello world")

        with (
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch.object(bridge, "get_available_tools", return_value=[{"function": {"name": "read_file"}}]),
            patch.object(bridge, "_iter_llm_chunks", side_effect=iter_responses),
            patch.object(bridge, "execute_tool", return_value=mock_tool_result),
        ):
            chunks = collect(
                bridge._run_task_node(
                    plan_id="plan_y",
                    task_id="t0",
                    task_description="Read a file",
                    context=ctx,
                    task_tree=tree,
                    initial_messages=[],
                )
            )

        types = [c.type for c in chunks]
        assert ChunkType.TASK_NODE_START in types
        assert ChunkType.TOOL_CALL in types
        assert ChunkType.TOOL_RESULT in types
        assert ChunkType.TASK_NODE_END in types

    def test_run_subtask_recursion(self, tmp_path):
        """run_subtask calls trigger recursive _run_task_node invocation."""
        bridge = make_bridge()
        ctx = make_context(tmp_path)
        tree = self._make_tree("plan_z")

        subtask_call_chunk = ResponseChunk(
            type=ChunkType.TOOL_CALL,
            tool_name="run_subtask",
            tool_input={"task": "Investigate sub-problem"},
            tool_id="call_sub_01",
        )

        # Counter ensures only the very first LLM call emits the run_subtask
        # tool call.  Every subsequent call (parent round 1+, child rounds)
        # returns plain content so the loop terminates in O(1) rounds.
        call_count = [0]

        def fake_iter_llm(messages, tools=None):
            call_count[0] += 1
            if call_count[0] == 1:
                yield subtask_call_chunk
                yield done_chunk()
            else:
                yield content_chunk("Sub-task synthesis result")
                yield done_chunk()

        with (
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch.object(bridge, "get_available_tools", return_value=[{"function": {"name": "run_subtask"}}]),
            patch.object(bridge, "_iter_llm_chunks", side_effect=fake_iter_llm),
        ):
            chunks = collect(
                bridge._run_task_node(
                    plan_id="plan_z",
                    task_id="root_t",
                    task_description="Top-level task",
                    context=ctx,
                    task_tree=tree,
                    initial_messages=[],
                )
            )

        types = [c.type for c in chunks]
        # Should have START from root, START from subtask, TOOL_CALL, TOOL_RESULT, END × 2
        assert types.count(ChunkType.TASK_NODE_START) >= 2
        assert types.count(ChunkType.TASK_NODE_END) >= 2
        assert ChunkType.TOOL_CALL in types
        assert ChunkType.TOOL_RESULT in types

    def test_depth_cap_removes_run_subtask(self, tmp_path):
        """At max_task_depth, run_subtask is not in available tools."""
        bridge = make_bridge(max_task_depth=2)
        ctx = make_context(tmp_path)
        tree = self._make_tree("plan_a")

        calls_to_get_tools: list[list] = []

        original_iter = bridge._iter_llm_chunks

        def recording_iter(messages, tools=None):
            calls_to_get_tools.append(tools or [])
            yield content_chunk("done at depth")
            yield done_chunk()

        with (
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch.object(
                bridge,
                "get_available_tools",
                return_value=[
                    {"function": {"name": "run_subtask"}},
                    {"function": {"name": "read_file"}},
                ],
            ),
            patch.object(bridge, "_iter_llm_chunks", side_effect=recording_iter),
        ):
            collect(
                bridge._run_task_node(
                    plan_id="plan_a",
                    task_id="deep",
                    task_description="Deep task",
                    depth=2,  # equals max_task_depth
                    context=ctx,
                    task_tree=tree,
                    initial_messages=[],
                )
            )

        assert calls_to_get_tools, "Expected at least one LLM call"
        used_tools = calls_to_get_tools[0]
        tool_names = [t.get("function", {}).get("name") for t in used_tools]
        assert "run_subtask" not in tool_names
        assert "read_file" in tool_names


# ── _stream_planned_response ───────────────────────────────────────────────────


class TestStreamPlannedResponse:
    def _plan_json(self) -> str:
        return json.dumps(
            {
                "plan_name": "My Plan",
                "steps": [
                    {"id": "s0", "description": "Step zero", "tbd": False, "depends_on": []},
                ],
            }
        )

    def test_plan_start_emitted(self, tmp_path):
        bridge = make_bridge()
        ctx = make_context(tmp_path)

        with (
            patch.object(ctx, "load_plans", return_value=[]),
            patch.object(bridge, "_load_prompt_file", return_value="Planner text"),
            patch.object(bridge, "_context_to_history", return_value=[]),
            patch.object(
                bridge,
                "_iter_llm_chunks",
                return_value=iter([content_chunk(self._plan_json()), done_chunk()]),
            ),
            patch.object(
                bridge,
                "_run_plan",
                return_value=iter(
                    [
                        ResponseChunk(
                            type=ChunkType.TASK_NODE_START, plan_id="plan_x", task_id="s0", content="Step zero"
                        ),
                        ResponseChunk(type=ChunkType.TASK_NODE_END, plan_id="plan_x", task_id="s0", content="Done"),
                    ]
                ),
            ),
        ):
            chunks = collect(bridge._stream_planned_response("Do the thing", ctx))

        types = [c.type for c in chunks]
        assert ChunkType.PLAN_START in types
        assert ChunkType.DONE in types
        plan_start = next(c for c in chunks if c.type == ChunkType.PLAN_START)
        assert plan_start.plan_name == "My Plan"

    def test_fallback_on_planner_failure(self, tmp_path):
        """When _create_plan fails, falls back to _run_tool_loop."""
        bridge = make_bridge()
        ctx = make_context(tmp_path)

        with (
            patch.object(ctx, "load_plans", return_value=[]),
            patch.object(bridge, "_create_plan", return_value=(None, None)),
            patch.object(
                bridge,
                "_run_tool_loop",
                return_value=iter([content_chunk("Fallback answer"), done_chunk()]),
            ),
        ):
            chunks = collect(bridge._stream_planned_response("Do the thing", ctx))

        types = [c.type for c in chunks]
        assert ChunkType.PLAN_START not in types
        content = "".join(c.content or "" for c in chunks if c.type == ChunkType.CONTENT)
        assert "Fallback answer" in content


# ── run_subtask module-level function ─────────────────────────────────────────


class TestRunSubtaskFunction:
    def test_raises_not_implemented(self):
        with pytest.raises(NotImplementedError):
            run_subtask("some task")

    def test_schema_extraction(self):
        """extract_tool_schema should produce a valid OpenAI schema for run_subtask."""
        from agentix.tools.schema import extract_tool_schema

        schema = extract_tool_schema(run_subtask)
        assert schema["type"] == "function"
        fn = schema["function"]
        assert fn["name"] == "run_subtask"
        params = fn["parameters"]["properties"]
        assert "task" in params
        assert "scratch_file" in params
