from unittest.mock import patch

import pytest

from agentix.agentix_config import AgentixConfig
from agentix.bridge.classify_prompt import _format_working_memory_for_classification, classify_prompt
from shared.models.context import Context
from shared.models.message import Message, MessageRole
from shared.models.working_memory import FactOwner, WorkingMemory

DEFAULT_RESULT = {
    "intent": "conversation",
    "needs_clarification": False,
    "missing_fields": [],
    "reasoning_summary": "ok",
    "next_step": "respond_directly",
}


def test_classify_prompt_uses_submitted_prompt_in_payload():
    config = AgentixConfig(
        model="gpt-oss",
        user=["stale-config-user"],
        temperature=0.5,
        classification_max_tokens=42,
    )
    context = Context()
    captured = {}

    def fake_assemble(args, history, max_tokens, response_max_tokens=None):
        captured["args"] = args
        captured["history"] = history
        captured["max_tokens"] = max_tokens
        captured["response_max_tokens"] = response_max_tokens
        return {"messages": [{"role": "user", "content": "payload"}]}

    with (
        patch("agentix.bridge.classify_prompt.assemble_prompts", side_effect=fake_assemble),
        patch("agentix.bridge.classify_prompt.query_classification", return_value=DEFAULT_RESULT),
    ):
        classify_prompt(
            config=config,
            prompt="live user prompt",
            context=context,
            history=[],
            max_tokens=1000,
        )

    assert captured["args"].user == ["live user prompt"]
    assert captured["response_max_tokens"] == 42


def test_classify_prompt_uses_context_when_history_not_provided():
    config = AgentixConfig(model="gpt-oss", temperature=0.5)
    context = Context()
    context.add_message(Message(role=MessageRole.USER, content="previous message"))
    captured = {}

    def fake_assemble(args, history, max_tokens, response_max_tokens=None):
        captured["history"] = history
        return {"messages": [{"role": "user", "content": "payload"}]}

    with (
        patch("agentix.bridge.classify_prompt.assemble_prompts", side_effect=fake_assemble),
        patch("agentix.bridge.classify_prompt.query_classification", return_value=DEFAULT_RESULT),
    ):
        classify_prompt(
            config=config,
            prompt="new prompt",
            context=context,
            history=None,
            max_tokens=1000,
        )

    assert len(captured["history"]) == 1
    assert captured["history"][0].content == "previous message"


# ---------------------------------------------------------------------------
# Bridge: CLASSIFICATION chunk emission tests
# ---------------------------------------------------------------------------


def test_process_prompt_streaming_emits_classification_chunk_first():
    """process_prompt_streaming yields a CLASSIFICATION chunk as the first chunk."""
    from unittest.mock import MagicMock

    from agentix.bridge.bridge import AgentixBridge
    from agentix.prompt_classification_response import (
        Intent,
        NextStep,
        PromptClassificationResponse,
    )
    from shared.models.response import ChunkType

    config = AgentixConfig(model="gpt-oss", classify_prompts=False)
    bridge = AgentixBridge(config)

    classification = PromptClassificationResponse(
        intent=Intent.simple_action,
        next_step=NextStep.respond_directly,
        reasoning_summary="Direct answer is sufficient.",
        needs_clarification=False,
        missing_fields=[],
    )

    # Stub the underlying stream so it produces no extra chunks
    bridge._stream_direct_response = MagicMock(return_value=iter([]))

    chunks = list(bridge.process_prompt_streaming("hello", Context(), classification=classification))

    assert len(chunks) >= 1
    first = chunks[0]
    assert first.type == ChunkType.CLASSIFICATION
    assert first.classification is not None
    assert first.classification["intent"] == "simple_action"
    assert first.classification["next_step"] == "respond_directly"
    assert first.classification["reasoning_summary"] == "Direct answer is sufficient."
    assert first.classification["needs_clarification"] is False
    assert first.classification["missing_fields"] == []


def test_process_prompt_streaming_classification_chunk_has_all_keys():
    """CLASSIFICATION chunk dict contains all five expected keys."""
    from unittest.mock import MagicMock

    from agentix.bridge.bridge import AgentixBridge
    from agentix.prompt_classification_response import (
        Intent,
        NextStep,
        PromptClassificationResponse,
    )

    config = AgentixConfig(model="gpt-oss", classify_prompts=False)
    bridge = AgentixBridge(config)

    classification = PromptClassificationResponse(
        intent=Intent.complex_action,
        next_step=NextStep.invoke_planner,
        reasoning_summary="Needs planning.",
        needs_clarification=True,
        missing_fields=["file_path"],
    )

    bridge._stream_planned_response = MagicMock(return_value=iter([]))

    chunks = list(bridge.process_prompt_streaming("do something complex", Context(), classification=classification))

    classification_chunk = chunks[0]
    meta = classification_chunk.classification
    assert set(meta.keys()) >= {"intent", "next_step", "reasoning_summary", "needs_clarification", "missing_fields"}
    assert meta["needs_clarification"] is True
    assert meta["missing_fields"] == ["file_path"]


def test_process_prompt_streaming_no_classification_chunk_when_unclassified():
    """Without a classification, the stream starts directly with content (no CLASSIFICATION chunk)."""
    from unittest.mock import MagicMock, patch

    from agentix.bridge.bridge import AgentixBridge
    from shared.models.response import ChunkType, ResponseChunk, content_chunk

    config = AgentixConfig(model="gpt-oss", classify_prompts=False)
    bridge = AgentixBridge(config)
    bridge._stream_direct_response = MagicMock(return_value=iter([content_chunk("hi")]))

    chunks = list(bridge.process_prompt_streaming("hello", Context(), classification=None))

    types = [c.type for c in chunks]
    assert ChunkType.CLASSIFICATION not in types
    assert ChunkType.CONTENT in types


# ---------------------------------------------------------------------------
# Coverage uplift: history truncation, missing fields, exception paths
# ---------------------------------------------------------------------------


def _make_config():
    return AgentixConfig(model="gpt-oss", temperature=0.5)


def test_classify_prompt_truncates_long_history():
    """History with > 2 entries is truncated to the last 2 before classification."""
    config = _make_config()
    context = Context()
    captured = {}

    def fake_assemble(args, history, max_tokens, response_max_tokens=None):
        captured["history_len"] = len(history)
        return {"messages": []}

    messages = [Message(role=MessageRole.USER, content=f"msg {i}") for i in range(5)]

    with (
        patch("agentix.bridge.classify_prompt.assemble_prompts", side_effect=fake_assemble),
        patch("agentix.bridge.classify_prompt.query_classification", return_value=DEFAULT_RESULT),
    ):
        classify_prompt(config=config, prompt="new", context=context, history=messages, max_tokens=1000)

    # History is truncated to CLASSIFICATION_HISTORY_LIMIT (2) entries before being
    # passed to assemble_prompts — the new prompt adds one more user message on top.
    assert captured["history_len"] <= 3  # 2 truncated + 1 new user message


def test_classify_prompt_raises_on_non_dict_result():
    """raise ValueError if LLM returns a non-dict result."""
    config = _make_config()

    with (
        patch("agentix.bridge.classify_prompt.assemble_prompts", return_value={"messages": []}),
        patch("agentix.bridge.classify_prompt.query_classification", return_value=["not", "a", "dict"]),
    ):
        with pytest.raises(ValueError, match="must be a dict"):
            classify_prompt(config=config, prompt="hi", context=Context(), history=[], max_tokens=1000)


def test_classify_prompt_raises_on_missing_required_fields():
    """raise ValueError if required fields are absent from LLM result."""
    config = _make_config()
    bad_result = {"intent": "conversation"}  # missing next_step and reasoning_summary

    with (
        patch("agentix.bridge.classify_prompt.assemble_prompts", return_value={"messages": []}),
        patch("agentix.bridge.classify_prompt.query_classification", return_value=bad_result),
    ):
        with pytest.raises(ValueError, match="incomplete JSON"):
            classify_prompt(config=config, prompt="hi", context=Context(), history=[], max_tokens=1000)


def test_classify_prompt_reraises_key_error_on_invalid_enum():
    """KeyError (bad intent/next_step value) propagates to caller."""
    config = _make_config()
    bad_enum_result = {
        "intent": "NOT_A_REAL_INTENT",
        "next_step": "respond_directly",
        "reasoning_summary": "ok",
        "needs_clarification": False,
        "missing_fields": [],
    }

    with (
        patch("agentix.bridge.classify_prompt.assemble_prompts", return_value={"messages": []}),
        patch("agentix.bridge.classify_prompt.query_classification", return_value=bad_enum_result),
    ):
        with pytest.raises(KeyError):
            classify_prompt(config=config, prompt="hi", context=Context(), history=[], max_tokens=1000)


def test_classify_prompt_reraises_generic_exception():
    """Unexpected exceptions from query_classification propagate to caller."""
    config = _make_config()

    with (
        patch("agentix.bridge.classify_prompt.assemble_prompts", return_value={"messages": []}),
        patch("agentix.bridge.classify_prompt.query_classification", side_effect=RuntimeError("network down")),
    ):
        with pytest.raises(RuntimeError, match="network down"):
            classify_prompt(config=config, prompt="hi", context=Context(), history=[], max_tokens=1000)


def test_format_working_memory_with_string_and_non_string_values():
    """_format_working_memory_for_classification covers lines 30-37 (loop body, non-string branch)."""
    wm = WorkingMemory()
    wm.add_fact(FactOwner.USER, "name", "Alice")
    wm.add_fact(FactOwner.AGENT, "count", 42)  # non-string value → str() branch

    result = _format_working_memory_for_classification(wm)

    assert "<working_memory>" in result
    assert "</working_memory>" in result
    assert "name: Alice" in result
    assert "count: 42" in result


def test_classify_prompt_injects_working_memory_into_prompt():
    """classify_prompt covers lines 90-92 (working_memory injection path)."""
    config = _make_config()
    wm = WorkingMemory()
    wm.add_fact(FactOwner.USER, "lang", "Python")

    with (
        patch("agentix.bridge.classify_prompt.assemble_prompts", return_value={"messages": []}),
        patch("agentix.bridge.classify_prompt.query_classification", return_value=DEFAULT_RESULT),
    ):
        result = classify_prompt(config=config, prompt="help", context=Context(), history=[], working_memory=wm)

    assert result is not None


def test_classify_prompt_routes_vibe_editor_open_intent_without_llm_call():
    """GIVEN a vibe-editor open request WHEN classify_prompt runs THEN it routes to single_tool without API classification."""
    config = _make_config()

    with (
        patch("agentix.bridge.classify_prompt.assemble_prompts") as assemble_mock,
        patch("agentix.bridge.classify_prompt.query_classification") as query_mock,
    ):
        result = classify_prompt(
            config=config,
            prompt="open src/agentx/session.py in vibe editor",
            context=Context(),
            history=[],
            max_tokens=1000,
        )

    assert result.intent.name == "simple_action"
    assert result.next_step.name == "single_tool"
    assemble_mock.assert_not_called()
    query_mock.assert_not_called()


def test_classify_prompt_non_editor_phrase_still_uses_llm_classification():
    """GIVEN a non-editor prompt WHEN classify_prompt runs THEN the normal LLM classification path is used."""
    config = _make_config()

    with (
        patch("agentix.bridge.classify_prompt.assemble_prompts", return_value={"messages": []}) as assemble_mock,
        patch("agentix.bridge.classify_prompt.query_classification", return_value=DEFAULT_RESULT) as query_mock,
    ):
        result = classify_prompt(
            config=config,
            prompt="open README.md",
            context=Context(),
            history=[],
            max_tokens=1000,
        )

    assert result.intent.name == "conversation"
    assemble_mock.assert_called_once()
    query_mock.assert_called_once()
