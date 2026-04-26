"""Unit tests for Context message ID indexing and lineage helpers."""

import json

import pytest

from shared.models.context import Context
from shared.models.message import Message, MessageRole


@pytest.mark.unit
def test_context_index_by_message_id_returns_all_messages() -> None:
    """GIVEN unique messages in context WHEN indexing by ID THEN all messages are addressable."""
    ctx = Context()
    m1 = Message(role=MessageRole.USER, content="u")
    m2 = Message(role=MessageRole.ASSISTANT, content="a")
    ctx.add_message(m1)
    ctx.add_message(m2)

    index = ctx.index_by_message_id()

    assert index[m1.message_id] is m1
    assert index[m2.message_id] is m2


@pytest.mark.unit
def test_context_rejects_duplicate_message_ids() -> None:
    """GIVEN duplicate message IDs WHEN adding second message THEN context raises an error."""
    ctx = Context()
    msg_id = "msg_0123456789abcdef0123456789abcdef"
    ctx.add_message(Message(role=MessageRole.USER, content="one", message_id=msg_id))

    with pytest.raises(ValueError, match="Duplicate message_id"):
        ctx.add_message(Message(role=MessageRole.ASSISTANT, content="two", message_id=msg_id))


@pytest.mark.unit
def test_get_ancestry_returns_ordered_chain() -> None:
    """GIVEN replay-linked messages WHEN ancestry requested THEN result is ordered from root to leaf."""
    ctx = Context()
    root = Message(role=MessageRole.TOOL_CALL, content="root")
    child = Message(role=MessageRole.TOOL_CALL, content="child", cloned_from=root.message_id)
    leaf = Message(role=MessageRole.TOOL_CALL, content="leaf", cloned_from=child.message_id)
    ctx.add_message(root)
    ctx.add_message(child)
    ctx.add_message(leaf)

    ancestry = ctx.get_ancestry(leaf.message_id)

    assert [m.message_id for m in ancestry] == [root.message_id, child.message_id, leaf.message_id]


@pytest.mark.unit
def test_supersede_message_sets_replacement_and_disables_original() -> None:
    """GIVEN original and replacement messages WHEN superseded THEN original points to replacement and is disabled."""
    ctx = Context()
    original = Message(role=MessageRole.TOOL_RESULT, content="old")
    replacement = Message(role=MessageRole.TOOL_RESULT, content="new")
    ctx.add_message(original)
    ctx.add_message(replacement)

    ctx.supersede_message(original.message_id, replacement.message_id)

    assert original.superseded_by == replacement.message_id
    assert original.enabled is False


@pytest.mark.integration
def test_context_load_from_dir_rejects_legacy_message_without_id(tmp_path) -> None:
    """GIVEN a legacy context JSON without message_id WHEN loading from directory THEN load aborts with validation error."""
    legacy = {
        "role": "user",
        "content": "legacy",
        "enabled": True,
        "epoch": 1,
        "attachments": [],
    }
    (tmp_path / "1_user.json").write_text(json.dumps(legacy), encoding="utf-8")

    ctx = Context(path=str(tmp_path))

    with pytest.raises(ValueError, match="message_id"):
        ctx.load_from_dir(str(tmp_path))
