from unittest.mock import patch

from agentix.agentix_config import AgentixConfig
from agentix.bridge.classify_prompt import classify_prompt
from shared.models.context import Context
from shared.models.message import Message, MessageRole


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
