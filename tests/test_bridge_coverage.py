"""
Coverage uplift tests for src/agentix/bridge/bridge.py.

Targets the 71% → 90% uplift by covering:
  - process_prompt_streaming: auto-classify path, debug print, escalate case
  - _stream_direct_response: body (normal + tool-call-fallback paths)
  - get_available_models / get_available_tools
  - retrigger_synthesis_streaming: epoch_set branch
  - Various exception-path fallbacks
"""

import sys
import os
from pathlib import Path
from unittest.mock import MagicMock, patch, PropertyMock

project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from agentix.agentix_config import AgentixConfig
from agentix.bridge.bridge import AgentixBridge
from agentix.prompt_classification_response import (
    Intent,
    NextStep,
    PromptClassificationResponse,
)
from shared.models.context import Context
from shared.models.response import ChunkType, ResponseChunk

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_bridge(classify: bool = False, debug: bool = False) -> AgentixBridge:
    config = AgentixConfig(model="test-model", tools=[], debug=debug)
    config.classify_prompts = classify
    return AgentixBridge(config)


def _make_classification(next_step: NextStep) -> PromptClassificationResponse:
    return PromptClassificationResponse(
        intent=Intent.conversation,
        needs_clarification=False,
        missing_fields=[],
        reasoning_summary="test",
        next_step=next_step,
    )


def _content_done(*texts) -> list[ResponseChunk]:
    chunks = [ResponseChunk(type=ChunkType.CONTENT, content=t) for t in texts]
    chunks.append(ResponseChunk(type=ChunkType.DONE, done_reason="stop"))
    return chunks


# ---------------------------------------------------------------------------
# process_prompt_streaming — auto-classify path (lines 128-131)
# ---------------------------------------------------------------------------


class TestProcessPromptStreamingAutoClassify:
    def test_auto_classify_when_classification_is_none(self):
        """When classification=None and classify_prompts=True, auto-classifies (lines 128-131)."""
        bridge = _make_bridge(classify=True)
        mock_cls = _make_classification(NextStep.respond_directly)
        with (
            patch.object(bridge, "classify_prompt", return_value=mock_cls),
            patch.object(bridge, "_stream_direct_response", return_value=iter([])),
        ):
            chunks = list(bridge.process_prompt_streaming("hello", Context(), classification=None))
        # CLASSIFICATION chunk should be yielded
        types = [c.type for c in chunks]
        assert ChunkType.CLASSIFICATION in types

    def test_no_auto_classify_when_classify_disabled(self):
        """When classify_prompts=False, no auto-classification (line 126 branch not taken)."""
        bridge = _make_bridge(classify=False)
        with patch.object(bridge, "_stream_direct_response", return_value=iter(_content_done("hi"))):
            chunks = list(bridge.process_prompt_streaming("hello", Context(), classification=None))
        types = [c.type for c in chunks]
        assert ChunkType.CLASSIFICATION not in types

    def test_debug_print_when_debug_and_classification(self):
        """debug=True prints classification info (line 158)."""
        bridge = _make_bridge(classify=True, debug=True)
        mock_cls = _make_classification(NextStep.respond_directly)
        with (
            patch.object(bridge, "classify_prompt", return_value=mock_cls),
            patch.object(bridge, "_stream_direct_response", return_value=iter([])),
        ):
            # Should not raise
            list(bridge.process_prompt_streaming("hello", Context(), classification=None))

    def test_escalate_yields_error_chunk(self):
        """NextStep.escalate yields ERROR chunk (lines 163-164)."""
        bridge = _make_bridge(classify=False)
        classification = _make_classification(NextStep.escalate)
        chunks = list(bridge.process_prompt_streaming("help me hack", Context(), classification=classification))
        types = [c.type for c in chunks]
        assert ChunkType.ERROR in types
        error_chunk = next(c for c in chunks if c.type == ChunkType.ERROR)
        assert "safety" in error_chunk.content.lower() or "human" in error_chunk.content.lower()

    def test_single_tool_classification_calls_stream_tool_response(self):
        """NextStep.single_tool calls _stream_tool_response."""
        bridge = _make_bridge(classify=False)
        classification = _make_classification(NextStep.single_tool)
        with patch.object(bridge, "_stream_tool_response", return_value=iter(_content_done("result"))):
            chunks = list(bridge.process_prompt_streaming("do tool thing", Context(), classification=classification))
        contents = [c.content for c in chunks if c.type == ChunkType.CONTENT]
        assert "result" in contents

    def test_invoke_planner_calls_stream_planned_response(self):
        """NextStep.invoke_planner calls _stream_planned_response."""
        bridge = _make_bridge(classify=False)
        classification = _make_classification(NextStep.invoke_planner)
        with patch.object(bridge, "_stream_planned_response", return_value=iter(_content_done("plan done"))):
            chunks = list(bridge.process_prompt_streaming("make a plan", Context(), classification=classification))
        contents = [c.content for c in chunks if c.type == ChunkType.CONTENT]
        assert "plan done" in contents


# ---------------------------------------------------------------------------
# _stream_direct_response — body (lines 440-458)
# ---------------------------------------------------------------------------


class TestStreamDirectResponse:
    def test_normal_content_is_buffered_and_yielded(self):
        """LLM returns CONTENT+DONE → buffered and yielded (lines 440-450, 457-458)."""
        bridge = _make_bridge()
        content_chunks = _content_done("Hello there")
        with patch.object(bridge, "_iter_llm_chunks", return_value=iter(content_chunks)):
            chunks = list(bridge._stream_direct_response("hi", Context()))
        contents = [c.content for c in chunks if c.type == ChunkType.CONTENT]
        assert "Hello there" in contents

    def test_tool_call_in_direct_response_falls_back_to_tool_loop(self):
        """If LLM returns TOOL_CALL in direct-response mode, fall back to _run_tool_loop (lines 451-453)."""
        bridge = _make_bridge()
        tool_chunk = ResponseChunk(type=ChunkType.TOOL_CALL, tool_name="read_file", tool_input={}, tool_id="c1")
        done_chunk = ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")
        tool_loop_result = _content_done("tool result")

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter([tool_chunk, done_chunk])),
            patch.object(bridge, "_run_tool_loop", return_value=iter(tool_loop_result)),
        ):
            chunks = list(bridge._stream_direct_response("read a file", Context()))
        contents = [c.content for c in chunks if c.type == ChunkType.CONTENT]
        assert "tool result" in contents


# ---------------------------------------------------------------------------
# get_available_models — cache miss (lines 181-184)
# ---------------------------------------------------------------------------


class TestGetAvailableModels:
    def test_populates_cache_on_first_call(self):
        """First call fetches models and caches them (lines 181-184)."""
        bridge = _make_bridge()
        mock_models = [{"name": "llama3"}, {"name": "mistral"}]
        with patch("agentix.bridge.bridge.get_models", return_value=mock_models):
            result = bridge.get_available_models()
        assert result == mock_models
        assert bridge._model_cache == mock_models

    def test_returns_cached_models_on_second_call(self):
        """Subsequent calls use cache without re-fetching."""
        bridge = _make_bridge()
        bridge._model_cache = [{"name": "cached"}]
        with patch("agentix.bridge.bridge.get_models") as mock_get:
            result = bridge.get_available_models()
        mock_get.assert_not_called()
        assert result == [{"name": "cached"}]


# ---------------------------------------------------------------------------
# get_available_tools (line 214)
# ---------------------------------------------------------------------------


class TestGetAvailableTools:
    def test_delegates_to_tool_runner(self):
        """get_available_tools delegates to _tool_runner (line 214)."""
        bridge = _make_bridge()
        mock_tools = [{"type": "function", "function": {"name": "read_file"}}]
        bridge._tool_runner.get_available_tools = MagicMock(return_value=mock_tools)
        result = bridge.get_available_tools()
        assert result == mock_tools


# ---------------------------------------------------------------------------
# set_enabled_tools (lines 221-225)
# ---------------------------------------------------------------------------


class TestSetEnabledTools:
    def test_delegates_to_tool_runner(self):
        bridge = _make_bridge()
        bridge._tool_runner.set_enabled_tools = MagicMock()
        bridge.set_enabled_tools(["read_file"])
        bridge._tool_runner.set_enabled_tools.assert_called_once_with(["read_file"])


# ---------------------------------------------------------------------------
# _stream_planned_response — plan=None fallback (lines 524-525)
# ---------------------------------------------------------------------------


class TestStreamPlannedResponse:
    def test_fallback_to_tool_loop_when_plan_is_none(self):
        """When _create_plan returns (None, None), falls back to tool loop (lines 524-525)."""
        bridge = _make_bridge()
        fallback_chunks = _content_done("fallback answer")
        with (
            patch.object(bridge, "_create_plan", return_value=(None, None)),
            patch.object(bridge, "_run_tool_loop", return_value=iter(fallback_chunks)),
        ):
            chunks = list(bridge._stream_planned_response("do complex thing", Context()))
        contents = [c.content for c in chunks if c.type == ChunkType.CONTENT]
        assert "fallback answer" in contents


# ---------------------------------------------------------------------------
# retrigger_synthesis_streaming — epoch_set path (line 273-280)
# ---------------------------------------------------------------------------


class TestRetriggerSynthesisStreaming:
    def test_epoch_set_path_filters_messages(self):
        """epoch_set branch filters task messages by epoch (lines 273-280)."""
        bridge = _make_bridge()
        ctx = Context()
        from shared.models.task_node import TaskNodeRecord

        node = MagicMock()
        node.task_id = "step-1"
        node.plan_id = "plan-1"
        node.parent_task_id = None
        node.depth = 0
        node.child_message_epochs = [1000.0, 2000.0]
        node.assertions = []
        node.synthesis_attempts = []

        tree = MagicMock()
        tree.nodes = {}

        synthesis_chunks = _content_done("synthesized result")
        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(synthesis_chunks)),
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
        ):
            chunks = list(bridge.retrigger_synthesis_streaming(node, ctx, tree, hint=""))
        # Should produce TASK_NODE_END
        types = [c.type for c in chunks]
        assert ChunkType.TASK_NODE_END in types


# ---------------------------------------------------------------------------
# replay_task_node_streaming — lines 311-313
# ---------------------------------------------------------------------------


class TestReplayTaskNodeStreaming:
    def test_delegates_to_run_task_node(self):
        """replay calls _run_task_node with node properties (lines 311-313)."""
        bridge = _make_bridge()
        node = MagicMock()
        node.task_id = "step-1"
        node.plan_id = "plan-1"
        node.parent_task_id = None
        node.depth = 0
        node.description = "do something"

        result_chunks = _content_done("replay result")
        tree = MagicMock()
        tree.nodes = {}

        with patch.object(bridge, "_run_task_node", return_value=iter(result_chunks)):
            chunks = list(bridge.replay_task_node_streaming(node, Context(), tree))
        contents = [c.content for c in chunks if c.type == ChunkType.CONTENT]
        assert "replay result" in contents


# ---------------------------------------------------------------------------
# Delegation helpers — _get_tool_implementations (line 407), _iter_llm_chunks (line 415)
# ---------------------------------------------------------------------------


class TestDelegationMethods:
    def test_get_tool_implementations_delegates(self):
        bridge = _make_bridge()
        bridge._tool_runner._get_tool_implementations = MagicMock(return_value={"read_file": None})
        result = bridge._get_tool_implementations()
        assert "read_file" in result

    def test_iter_llm_chunks_delegates(self):
        bridge = _make_bridge()
        expected = iter([])
        bridge._tool_runner._iter_llm_chunks = MagicMock(return_value=expected)
        result = bridge._iter_llm_chunks([])
        assert result is expected


# ---------------------------------------------------------------------------
# _stream_tool_response body (line 483)
# ---------------------------------------------------------------------------


class TestStreamToolResponseBody:
    def test_body_delegates_to_run_tool_loop(self):
        """_stream_tool_response body calls _run_tool_loop (line 483)."""
        bridge = _make_bridge()
        classification = _make_classification(NextStep.single_tool)
        with patch.object(bridge, "_run_tool_loop", return_value=iter(_content_done("tool-result"))):
            chunks = list(bridge._stream_tool_response("do tool", Context(), classification))
        assert any(c.content == "tool-result" for c in chunks if c.type == ChunkType.CONTENT)


# ---------------------------------------------------------------------------
# _stream_planned_response exception paths (lines 499-500, 524-525, 530-531, 538-539)
# ---------------------------------------------------------------------------


class TestStreamPlannedResponseMorePaths:
    def test_load_plans_exception_caught(self):
        """Lines 499-500: load_plans raises → existing_plans = []."""
        bridge = _make_bridge()
        ctx = MagicMock()
        ctx.load_plans.side_effect = Exception("db error")
        with (
            patch.object(bridge, "_create_plan", return_value=(None, None)),
            patch.object(bridge, "_run_tool_loop", return_value=iter(_content_done("fallback"))),
        ):
            chunks = list(bridge._stream_planned_response("prompt", ctx))
        assert any(c.type == ChunkType.CONTENT for c in chunks)

    def test_get_system_prompt_exception_and_save_exceptions(self):
        """Lines 524-525, 530-531, 538-539: get_system_prompt raises; save_plan raises."""
        from shared.models.task_node import PlanRecord, PlanStep, TaskTree as TaskTreeModel

        bridge = _make_bridge()
        ctx = MagicMock()
        ctx.load_plans.return_value = []
        ctx.session_id = "test"
        ctx.save_plan.side_effect = Exception("save error")

        step = PlanStep(step_id="s1", description="do task", tbd=False, depends_on=[])
        plan = PlanRecord(
            plan_id="p_test",
            plan_name="My Plan",
            session_plan_index=0,
            steps=[step],
            root_task_ids=["s1"],
            status="pending",
            epoch=1.0,
        )
        tree = TaskTreeModel(session_id="test", created_epoch=1.0, last_updated_epoch=1.0)

        with (
            patch.object(bridge, "_create_plan", return_value=(plan, tree)),
            patch("agentix.context.prompts.get_system_prompt", side_effect=Exception("no prompt")),
            patch.object(bridge, "_run_plan", return_value=iter(_content_done("plan done"))),
        ):
            chunks = list(bridge._stream_planned_response("do complex", ctx))
        assert any(c.type == ChunkType.CONTENT for c in chunks)


# ---------------------------------------------------------------------------
# retrigger_synthesis_streaming — more assertion paths (lines 275-280, 311-313, 321-323)
# ---------------------------------------------------------------------------


class TestRetriggerMorePaths:
    def _make_retrigger_node(self, with_epochs=True):
        node = MagicMock()
        node.task_id = "sn-1"
        node.plan_id = "p-1"
        node.parent_task_id = None
        node.depth = 0
        node.child_message_epochs = [9000.0] if with_epochs else []
        node.assertions = []
        node.synthesis_attempts = []
        return node

    def test_assertion_loop_body_covered(self):
        """Lines 311-313: extract_assertions returns one assertion → body executes."""
        from shared.models.task_node import AssertionRecord

        bridge = _make_bridge()
        ctx = MagicMock()
        ctx.to_llm_messages.return_value = []
        ctx.get_messages.return_value = []
        ctx.save_task_node.return_value = None
        ctx.save_task_tree.return_value = None
        tree = MagicMock()
        tree.nodes = {}

        good_assertion = AssertionRecord(fact="Result is non-empty", type="post", verified=True)
        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("great synthesis"))),
            patch("agentix.bridge.bridge.extract_assertions", return_value=[good_assertion]),
            patch("agentix.bridge.bridge.verify_assertion", return_value=None),
        ):
            chunks = list(bridge.retrigger_synthesis_streaming(self._make_retrigger_node(), ctx, tree, hint=""))
        assert any(c.type == ChunkType.ASSERTION_RESULT for c in chunks)

    def test_assertion_loop_exception_covered(self):
        """Lines 321-323: extract_assertions raises → caught, status=accepted."""
        bridge = _make_bridge()
        ctx = MagicMock()
        ctx.to_llm_messages.return_value = []
        ctx.get_messages.return_value = []
        ctx.save_task_node.return_value = None
        ctx.save_task_tree.return_value = None
        tree = MagicMock()
        tree.nodes = {}

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("synthesis"))),
            patch("agentix.bridge.bridge.extract_assertions", side_effect=RuntimeError("extract failed")),
        ):
            chunks = list(bridge.retrigger_synthesis_streaming(self._make_retrigger_node(), ctx, tree, hint=""))
        assert any(c.type == ChunkType.TASK_NODE_END for c in chunks)

    def test_epoch_set_messages_body_covered(self):
        """Lines 275-280: epoch_set non-empty + matching messages → loop body executed."""
        bridge = _make_bridge()
        ctx = MagicMock()
        ctx.to_llm_messages.return_value = []

        # Message whose epoch matches → to_llm_dict() called
        mock_msg = MagicMock()
        mock_msg.epoch = 9000.0
        mock_msg.to_llm_dict.return_value = {"role": "user", "content": "msg with epoch"}
        ctx.get_messages.return_value = [mock_msg]
        ctx.save_task_node.return_value = None
        ctx.save_task_tree.return_value = None
        tree = MagicMock()
        tree.nodes = {}

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("synthesis with context"))),
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
        ):
            chunks = list(
                bridge.retrigger_synthesis_streaming(self._make_retrigger_node(with_epochs=True), ctx, tree, hint="")
            )
        assert any(c.type == ChunkType.TASK_NODE_END for c in chunks)

    def test_epoch_set_message_to_llm_dict_exception_caught(self):
        """Lines 279-280: msg.to_llm_dict raises → caught, message skipped."""
        bridge = _make_bridge()
        ctx = MagicMock()
        ctx.to_llm_messages.return_value = []

        bad_msg = MagicMock()
        bad_msg.epoch = 9000.0
        bad_msg.to_llm_dict.side_effect = RuntimeError("serialize error")
        ctx.get_messages.return_value = [bad_msg]
        ctx.save_task_node.return_value = None
        ctx.save_task_tree.return_value = None
        tree = MagicMock()
        tree.nodes = {}

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("synthesis"))),
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
        ):
            chunks = list(
                bridge.retrigger_synthesis_streaming(self._make_retrigger_node(with_epochs=True), ctx, tree, hint="")
            )
        assert any(c.type == ChunkType.TASK_NODE_END for c in chunks)


# ---------------------------------------------------------------------------
# replay_task_node_streaming — except block (lines 375-376)
# ---------------------------------------------------------------------------


class TestReplayTaskNodeMorePaths:
    def test_message_to_llm_dict_exception_is_caught(self):
        """Lines 375-376: msg.to_llm_dict() raises → caught, message skipped."""
        bridge = _make_bridge()

        bad_msg = MagicMock()
        bad_msg.epoch = None
        bad_msg.to_llm_dict.side_effect = RuntimeError("cannot serialize")
        ctx = MagicMock()
        ctx.get_messages.return_value = [bad_msg]

        node = MagicMock()
        node.task_id = "n1"
        node.plan_id = "p1"
        node.parent_task_id = None
        node.depth = 0
        node.plan_step_index = 0
        node.tbd_resolved_description = None
        node.task_description = "do task"
        node.child_message_epochs = []

        tree = MagicMock()
        tree.nodes = {}

        with patch.object(bridge, "_run_task_node", return_value=iter(_content_done("replay result"))):
            chunks = list(bridge.replay_task_node_streaming(node, ctx, tree))
        assert any(c.type == ChunkType.CONTENT for c in chunks)


# ---------------------------------------------------------------------------
# _load_prompt_file (lines 547-556)
# ---------------------------------------------------------------------------


class TestLoadPromptFile:
    def test_returns_none_when_no_matches(self):
        """No glob matches → None."""
        bridge = _make_bridge()
        with patch("agentix.prompt_loader._glob.glob", return_value=[]):
            result = bridge._load_prompt_file("nonexistent_prompt")
        assert result is None

    def test_returns_file_content_when_found(self):
        """File found → reads and returns content."""
        import unittest.mock as um

        bridge = _make_bridge()
        mock_open = um.mock_open(read_data="prompt text")
        with (
            patch("agentix.prompt_loader._glob.glob", return_value=["/fake/prompt.md"]),
            patch("builtins.open", mock_open),
        ):
            result = bridge._load_prompt_file("task_execution")
        assert result == "prompt text"

    def test_returns_none_on_exception(self):
        """Exception from glob → None."""
        bridge = _make_bridge()
        with patch("agentix.prompt_loader._glob.glob", side_effect=Exception("glob error")):
            result = bridge._load_prompt_file("bad_prompt")
        assert result is None


# ---------------------------------------------------------------------------
# _create_plan — save exception (lines 645-646)
# ---------------------------------------------------------------------------


class TestCreatePlanSaveException:
    def test_save_plan_exception_is_silenced(self):
        """Lines 645-646: context.save_plan raises → caught and silenced, plan still returned."""
        import json as _json

        bridge = _make_bridge()
        ctx = MagicMock()
        ctx.get_messages.return_value = []
        ctx.session_id = "test"
        ctx.save_plan.side_effect = Exception("save error")
        ctx.save_task_tree.side_effect = Exception("save error")

        valid_plan_json = _json.dumps(
            {
                "plan_name": "My Plan",
                "steps": [{"id": "s1", "description": "do the task", "tbd": False, "depends_on": []}],
            }
        )
        content_chunk = ResponseChunk(type=ChunkType.CONTENT, content=valid_plan_json)
        done_chunk = ResponseChunk(type=ChunkType.DONE, done_reason="stop")

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter([content_chunk, done_chunk])),
            patch.object(bridge, "_load_prompt_file", return_value="Be a planner."),
            patch.object(bridge, "_context_to_history", return_value=[]),
        ):
            plan, tree = bridge._create_plan("do complex task", ctx)
        assert plan is not None
        assert plan.plan_name == "My Plan"


# ---------------------------------------------------------------------------
# _resolve_tbd_step (lines 657-669)
# ---------------------------------------------------------------------------


class TestResolveTbdStep:
    def test_resolve_tbd_step_returns_llm_content(self):
        """Lines 657-669: calls LLM and returns resolved description."""
        from shared.models.task_node import PlanStep

        bridge = _make_bridge()
        step = PlanStep(step_id="s1", description="TBD: analyze something", tbd=True, depends_on=["s0"])
        content_chunks = [
            ResponseChunk(type=ChunkType.CONTENT, content="Analyze the log files"),
            ResponseChunk(type=ChunkType.DONE, done_reason="stop"),
        ]
        with patch.object(bridge, "_iter_llm_chunks", return_value=iter(content_chunks)):
            result = bridge._resolve_tbd_step(step, ["prior step result"], "My Plan")
        assert result == "Analyze the log files"

    def test_resolve_tbd_step_falls_back_to_description_when_empty(self):
        """_resolve_tbd_step falls back to step.description when LLM gives no content."""
        from shared.models.task_node import PlanStep

        bridge = _make_bridge()
        step = PlanStep(step_id="s1", description="TBD original", tbd=True, depends_on=[])
        done_chunk = ResponseChunk(type=ChunkType.DONE, done_reason="stop")
        with patch.object(bridge, "_iter_llm_chunks", return_value=iter([done_chunk])):
            result = bridge._resolve_tbd_step(step, [], "My Plan")
        assert result == "TBD original"


# ---------------------------------------------------------------------------
# _run_task_node — various uncovered paths
# ---------------------------------------------------------------------------


class TestRunTaskNodeMorePaths:
    def _make_ctx(self, *, save_raises=False):
        ctx = MagicMock()
        ctx.get_messages.return_value = []
        ctx.session_id = "test"
        if save_raises:
            ctx.save_task_node.side_effect = Exception("save failed")
            ctx.save_task_tree.side_effect = Exception("save failed")
        else:
            ctx.save_task_node.return_value = None
            ctx.save_task_tree.return_value = None
        return ctx

    def _run_node(self, bridge, ctx, **kw):
        tree = MagicMock()
        tree.nodes = {}
        defaults = dict(
            plan_id="p1",
            task_id="t1",
            task_description="task",
            context=ctx,
            task_tree=tree,
            initial_messages=[],
        )
        defaults.update(kw)
        return list(bridge._run_task_node(**defaults))

    def test_initial_save_exception_silenced(self):
        """Lines 714-715: context.save_task_node raises at start → silenced."""
        bridge = _make_bridge()
        ctx = self._make_ctx()
        ctx.save_task_node.side_effect = [Exception("init fail"), None]

        tree = MagicMock()
        tree.nodes = {}
        tree.add_node.side_effect = Exception("add fail")

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("done"))),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
        ):
            chunks = list(
                bridge._run_task_node(
                    plan_id="p1",
                    task_id="t1",
                    task_description="task",
                    context=ctx,
                    task_tree=tree,
                    initial_messages=[],
                )
            )
        assert any(c.type == ChunkType.TASK_NODE_START for c in chunks)

    def test_task_exec_prompt_injected(self):
        """Lines 721-722: _load_prompt_file returns content → inserted into messages."""
        bridge = _make_bridge()
        ctx = self._make_ctx()

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("result"))),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_load_prompt_file", return_value="System at depth {depth} of {max_depth}."),
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
        ):
            chunks = self._run_node(bridge, ctx)
        assert any(c.type == ChunkType.CONTENT for c in chunks)

    def test_else_chunk_type_is_yielded(self):
        """Line 753: else branch yields chunk types other than TOOL_CALL/CONTENT/THINKING/DONE."""
        bridge = _make_bridge()
        ctx = self._make_ctx()

        error_chunk = ResponseChunk(type=ChunkType.ERROR, content="some error")
        done_chunk = ResponseChunk(type=ChunkType.DONE, done_reason="stop")
        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter([error_chunk, done_chunk])),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
        ):
            chunks = self._run_node(bridge, ctx)
        assert any(c.type == ChunkType.ERROR for c in chunks)

    def test_synthesis_when_tools_used_and_no_content(self):
        """Lines 871-891: tool called, no content → synthesis step triggered."""
        bridge = _make_bridge()
        ctx = self._make_ctx()

        tool_call = ResponseChunk(type=ChunkType.TOOL_CALL, tool_name="test_tool", tool_input={}, tool_id="c1")
        done_after_tool = ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")
        synthesis_content = ResponseChunk(type=ChunkType.CONTENT, content="synthesized result")
        synthesis_done = ResponseChunk(type=ChunkType.DONE, done_reason="stop")

        call_count = [0]

        def side_effect(messages, tools=None):
            call_count[0] += 1
            return (
                iter([tool_call, done_after_tool]) if call_count[0] == 1 else iter([synthesis_content, synthesis_done])
            )

        mock_result = MagicMock()
        mock_result.success = True
        mock_result.output = "tool result"
        mock_result.to_llm_format.return_value = "tool result"

        with (
            patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect),
            patch.object(bridge, "execute_tool", return_value=mock_result),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
        ):
            chunks = self._run_node(bridge, ctx, max_rounds=1)
        content_texts = [c.content for c in chunks if c.type == ChunkType.CONTENT]
        assert "synthesized result" in content_texts

    def test_synthesis_no_content_produced(self):
        """Lines 889-891: synthesis step yields no content → '(no synthesis produced)'."""
        bridge = _make_bridge()
        ctx = self._make_ctx()

        tool_call = ResponseChunk(type=ChunkType.TOOL_CALL, tool_name="test_tool", tool_input={}, tool_id="c2")
        done_after_tool = ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")
        # Synthesis returns only DONE (no content)
        synthesis_done = ResponseChunk(type=ChunkType.DONE, done_reason="stop")

        call_count = [0]

        def side_effect(messages, tools=None):
            call_count[0] += 1
            return iter([tool_call, done_after_tool]) if call_count[0] == 1 else iter([synthesis_done])

        mock_result = MagicMock()
        mock_result.success = True
        mock_result.output = "data"
        mock_result.to_llm_format.return_value = "data"

        with (
            patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect),
            patch.object(bridge, "execute_tool", return_value=mock_result),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
        ):
            chunks = self._run_node(bridge, ctx, max_rounds=1)
        end_chunks = [c for c in chunks if c.type == ChunkType.TASK_NODE_END]
        assert end_chunks
        assert "(no synthesis produced)" in (end_chunks[0].content or "")

    def test_assertion_loop_exception_silenced(self):
        """Lines 946-947: extract_assertions raises → caught, execution continues."""
        bridge = _make_bridge()
        ctx = self._make_ctx()

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("good content"))),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch("agentix.bridge.bridge.extract_assertions", side_effect=RuntimeError("assertion fail")),
        ):
            chunks = self._run_node(bridge, ctx)
        assert any(c.type == ChunkType.TASK_NODE_END for c in chunks)

    def test_save_task_node_exception_silenced_at_end(self):
        """Lines 955-956: context.save_task_node raises at node completion → silenced."""
        bridge = _make_bridge()
        ctx = self._make_ctx()
        # Let initial save succeed but fail at the end
        ctx.save_task_node.side_effect = [None, Exception("final save failed")]

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("content"))),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_load_prompt_file", return_value=None),
            patch("agentix.bridge.bridge.extract_assertions", return_value=[]),
        ):
            chunks = self._run_node(bridge, ctx)
        assert any(c.type == ChunkType.TASK_NODE_END for c in chunks)


# ---------------------------------------------------------------------------
# _execute_plan_steps (lines 976-1064)
# ---------------------------------------------------------------------------


class TestExecutePlanSteps:
    def _make_ctx(self):
        ctx = MagicMock()
        ctx.get_messages.return_value = []
        ctx.session_id = "test"
        ctx.save_plan.return_value = None
        ctx.save_task_node.return_value = None
        ctx.save_task_tree.return_value = None
        return ctx

    def test_basic_single_step_plan(self):
        """Lines 976-1064: single step, then cross-step final synthesis."""
        from shared.models.task_node import PlanRecord, PlanStep

        bridge = _make_bridge()
        ctx = self._make_ctx()
        tree = MagicMock()
        tree.nodes = {}

        step = PlanStep(step_id="s1", description="Do the task", tbd=False, depends_on=[])
        plan = PlanRecord(
            plan_id="p1",
            plan_name="Test Plan",
            session_plan_index=0,
            steps=[step],
            root_task_ids=["s1"],
            status="running",
            epoch=1.0,
        )

        start_chunk = ResponseChunk(type=ChunkType.TASK_NODE_START, task_id="s1", content="Do the task", plan_id="p1")
        end_chunk = ResponseChunk(type=ChunkType.TASK_NODE_END, task_id="s1", content="step synthesis", plan_id="p1")

        with (
            patch.object(bridge, "_run_task_node", return_value=iter([start_chunk, end_chunk])),
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("final answer"))),
        ):
            chunks = list(bridge._run_plan(plan, ctx, tree, [], "What do you know?"))

        types = [c.type for c in chunks]
        assert ChunkType.TASK_NODE_START in types
        assert ChunkType.CONTENT in types

    def test_step_with_unsatisfied_dep_is_skipped(self):
        """Lines 984-989: step with unsatisfied deps → skipped."""
        from shared.models.task_node import PlanRecord, PlanStep

        bridge = _make_bridge()
        ctx = self._make_ctx()
        tree = MagicMock()

        step = PlanStep(step_id="s2", description="Depends on s1", tbd=False, depends_on=["s1"])
        plan = PlanRecord(
            plan_id="p1",
            plan_name="Dep Plan",
            session_plan_index=0,
            steps=[step],
            root_task_ids=[],
            status="running",
            epoch=1.0,
        )

        with patch.object(bridge, "_run_task_node", return_value=iter([])):
            chunks = list(bridge._run_plan(plan, ctx, tree, [], "prompt"))
        assert not any(c.type == ChunkType.TASK_NODE_START for c in chunks)

    def test_tbd_step_is_resolved(self):
        """Lines 997-1012: TBD step gets resolved at runtime."""
        from shared.models.task_node import PlanRecord, PlanStep

        bridge = _make_bridge()
        ctx = self._make_ctx()
        tree = MagicMock()
        tree.nodes = {}

        step = PlanStep(step_id="s1", description="TBD: figure out what to do", tbd=True, depends_on=[])
        plan = PlanRecord(
            plan_id="p1",
            plan_name="TBD Plan",
            session_plan_index=0,
            steps=[step],
            root_task_ids=["s1"],
            status="running",
            epoch=1.0,
        )

        tbd_chunk = ResponseChunk(type=ChunkType.TASK_NODE_TBD, task_id="s1", content="resolved desc", plan_id="p1")
        end_chunk = ResponseChunk(
            type=ChunkType.TASK_NODE_END, task_id="s1", content="resolved synthesis", plan_id="p1"
        )

        with (
            patch.object(bridge, "_resolve_tbd_step", return_value="resolved desc"),
            patch.object(bridge, "_run_task_node", return_value=iter([tbd_chunk, end_chunk])),
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("final synthesis"))),
        ):
            chunks = list(bridge._run_plan(plan, ctx, tree, [], "prompt"))
        assert any(c.type == ChunkType.TASK_NODE_TBD for c in chunks)


# ---------------------------------------------------------------------------
# _run_tool_loop — system prompt injection (lines 1113-1118), got_content (line 1142),
# synthesis content (line 1233), absolute fallback (line 1239)
# ---------------------------------------------------------------------------


class TestRunToolLoopMorePaths:
    def test_system_prompt_injected_when_config_has_system(self):
        """Lines 1113-1118: config.system set → system prompt prepended."""
        bridge = _make_bridge()
        bridge.config.system = ["tool_use"]
        ctx = MagicMock()

        with (
            patch("agentix.context.prompts.get_system_prompt", return_value="You are helpful."),
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("response with sys"))),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_context_to_history", return_value=[]),
        ):
            chunks = list(bridge._run_tool_loop("do something", ctx))
        assert any(c.type == ChunkType.CONTENT for c in chunks)

    def test_got_content_set_on_content_chunk(self):
        """Line 1142: got_content=True when CONTENT chunk received."""
        bridge = _make_bridge()
        ctx = MagicMock()

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter(_content_done("hi there"))),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_context_to_history", return_value=[]),
        ):
            chunks = list(bridge._run_tool_loop("hi", ctx))
        assert any(c.content == "hi there" for c in chunks if c.type == ChunkType.CONTENT)

    def test_synthesis_content_in_tool_loop(self):
        """Line 1233: synthesis step produces content → got_content set True."""
        bridge = _make_bridge()
        ctx = MagicMock()

        tool_call = ResponseChunk(type=ChunkType.TOOL_CALL, tool_name="read_file", tool_input={}, tool_id="c1")
        done_after_tool = ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")
        synthesis_content = ResponseChunk(type=ChunkType.CONTENT, content="synthesized in tool loop")
        synthesis_done = ResponseChunk(type=ChunkType.DONE, done_reason="stop")

        call_count = [0]

        def side_effect(messages, tools=None):
            call_count[0] += 1
            return (
                iter([tool_call, done_after_tool]) if call_count[0] == 1 else iter([synthesis_content, synthesis_done])
            )

        mock_result = MagicMock()
        mock_result.success = True
        mock_result.output = "file content"
        mock_result.to_llm_format.return_value = "file content"

        with (
            patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect),
            patch.object(bridge, "execute_tool", return_value=mock_result),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_context_to_history", return_value=[]),
        ):
            chunks = list(bridge._run_tool_loop("read the file", ctx, max_rounds=1))
        assert any(c.content == "synthesized in tool loop" for c in chunks if c.type == ChunkType.CONTENT)

    def test_absolute_fallback_when_no_synthesis_content(self):
        """Line 1239: synthesis step produces no content → fallback newline yielded."""
        bridge = _make_bridge()
        ctx = MagicMock()

        tool_call = ResponseChunk(type=ChunkType.TOOL_CALL, tool_name="read_file", tool_input={}, tool_id="c1")
        done_after_tool = ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")
        synthesis_done = ResponseChunk(type=ChunkType.DONE, done_reason="stop")

        call_count = [0]

        def side_effect(messages, tools=None):
            call_count[0] += 1
            return iter([tool_call, done_after_tool]) if call_count[0] == 1 else iter([synthesis_done])

        mock_result = MagicMock()
        mock_result.success = True
        mock_result.output = "data"
        mock_result.to_llm_format.return_value = "data"

        with (
            patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect),
            patch.object(bridge, "execute_tool", return_value=mock_result),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_context_to_history", return_value=[]),
        ):
            chunks = list(bridge._run_tool_loop("do task", ctx, max_rounds=1))
        content_chunks = [c for c in chunks if c.type == ChunkType.CONTENT]
        assert any(c.content == "\n" for c in content_chunks)

    def test_else_chunk_type_in_main_loop(self):
        """Line 1142: else branch in main loop yields non-standard chunk types (e.g. ERROR)."""
        bridge = _make_bridge()
        ctx = MagicMock()

        # ERROR is not TOOL_CALL, DONE, CONTENT, or THINKING → hits else branch
        error_chunk = ResponseChunk(type=ChunkType.ERROR, content="stream error")
        done_chunk = ResponseChunk(type=ChunkType.DONE, done_reason="stop")

        with (
            patch.object(bridge, "_iter_llm_chunks", return_value=iter([error_chunk, done_chunk])),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_context_to_history", return_value=[]),
        ):
            chunks = list(bridge._run_tool_loop("hi", ctx))
        assert any(c.type == ChunkType.ERROR for c in chunks)

    def test_tool_call_in_synthesis_is_skipped(self):
        """Line 1233: TOOL_CALL in synthesis phase is filtered (continue)."""
        bridge = _make_bridge()
        ctx = MagicMock()

        tool_call = ResponseChunk(type=ChunkType.TOOL_CALL, tool_name="read_file", tool_input={}, tool_id="c1")
        done_after_tool = ResponseChunk(type=ChunkType.DONE, done_reason="tool_calls")

        call_count = [0]

        def side_effect(messages, tools=None):
            call_count[0] += 1
            # Both rounds return a TOOL_CALL → synthesis also returns TOOL_CALL (gets filtered)
            return iter([tool_call, done_after_tool])

        mock_result = MagicMock()
        mock_result.success = True
        mock_result.output = "res"
        mock_result.to_llm_format.return_value = "res"

        with (
            patch.object(bridge, "_iter_llm_chunks", side_effect=side_effect),
            patch.object(bridge, "execute_tool", return_value=mock_result),
            patch.object(bridge, "get_available_tools", return_value=[]),
            patch.object(bridge, "_context_to_history", return_value=[]),
        ):
            chunks = list(bridge._run_tool_loop("do task", ctx, max_rounds=1))
        # Synthesis TOOL_CALL is filtered; fallback newline is yielded
        assert any(c.content == "\n" for c in chunks if c.type == ChunkType.CONTENT)


# ---------------------------------------------------------------------------
# create_bridge convenience function (lines 1291-1296)
# ---------------------------------------------------------------------------


class TestCreateBridgeFunction:
    def test_create_bridge_returns_configured_instance(self):
        """Lines 1291-1296: create_bridge returns a properly configured AgentixBridge."""
        from agentix.bridge.bridge import create_bridge

        bridge = create_bridge(model="llama3.2", debug=True)
        assert isinstance(bridge, AgentixBridge)
        assert bridge.config.model == "llama3.2"
        assert bridge.config.debug is True
