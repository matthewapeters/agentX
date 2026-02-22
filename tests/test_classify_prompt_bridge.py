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
