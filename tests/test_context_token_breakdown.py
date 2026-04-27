"""Unit tests for Context.token_breakdown.

GIVEN enabled context messages
WHEN token_breakdown is called
THEN role buckets and attachment totals are computed correctly.
"""

import pytest

from shared.models.attachment import Attachment
from shared.models.context import Context
from shared.models.message import Message, MessageRole


@pytest.mark.unit
def test_token_breakdown_empty_context() -> None:
    """GIVEN empty context WHEN token_breakdown runs THEN all buckets are zero."""
    ctx = Context()
    breakdown = ctx.token_breakdown(model_name="llama3.2")
    assert breakdown == {
        "working_memory": 0,
        "system": 0,
        "user": 0,
        "attachments": 0,
        "thinking": 0,
        "assistant": 0,
        "tool": 0,
    }


@pytest.mark.unit
def test_token_breakdown_working_memory_split() -> None:
    """GIVEN system and working-memory messages WHEN breakdown runs THEN buckets are split."""
    ctx = Context()

    wm = Message(role=MessageRole.SYSTEM, content="wm text")
    wm.metadata["is_working_memory"] = True
    normal_system = Message(role=MessageRole.SYSTEM, content="system text")
    user = Message(role=MessageRole.USER, content="hello user")

    ctx.add_message(wm)
    ctx.add_message(normal_system)
    ctx.add_message(user)

    breakdown = ctx.token_breakdown(model_name="mistral:7b")
    assert breakdown["working_memory"] > 0
    assert breakdown["system"] > 0
    assert breakdown["user"] > 0


@pytest.mark.unit
def test_token_breakdown_attachments_and_tool_roles() -> None:
    """GIVEN enabled attachments and tool messages WHEN breakdown runs THEN tool and attachment bands increase."""
    ctx = Context()

    call = Message(role=MessageRole.TOOL_CALL, content="calling")
    result = Message(role=MessageRole.TOOL_RESULT, content="done")
    user = Message(role=MessageRole.USER, content="ask")
    user.attachments.append(Attachment(file_path="a.txt", content="attachment body", enabled=True))

    ctx.add_message(call)
    ctx.add_message(result)
    ctx.add_message(user)

    breakdown = ctx.token_breakdown(model_name="phi3")
    assert breakdown["tool"] > 0
    assert breakdown["attachments"] > 0


@pytest.mark.unit
def test_token_breakdown_synthesis_counts_as_assistant() -> None:
    """GIVEN SYNTHESIS role messages WHEN breakdown runs THEN tokens are counted in assistant band.

    P3 fix verification: MessageRole.SYNTHESIS was previously unrouted and silently dropped.
    """
    ctx = Context()

    synthesis_msg = Message(role=MessageRole.SYNTHESIS, content="synthesised response text")
    ctx.add_message(synthesis_msg)

    breakdown = ctx.token_breakdown(model_name="llama3.2")
    assert breakdown["assistant"] > 0
    # All other bands should remain zero.
    for key in ("working_memory", "system", "user", "attachments", "thinking", "tool"):
        assert breakdown[key] == 0, f"Expected {key}=0 but got {breakdown[key]}"


@pytest.mark.unit
def test_token_breakdown_assistant_and_synthesis_both_count() -> None:
    """GIVEN both ASSISTANT and SYNTHESIS messages WHEN breakdown runs THEN both contribute to assistant band."""
    ctx = Context()

    assistant_msg = Message(role=MessageRole.ASSISTANT, content="standard assistant response")
    synthesis_msg = Message(role=MessageRole.SYNTHESIS, content="agent synthesis result")
    ctx.add_message(assistant_msg)
    ctx.add_message(synthesis_msg)

    breakdown = ctx.token_breakdown(model_name="mistral:7b")
    # Both should have increased the assistant bucket.
    assert breakdown["assistant"] > 0
