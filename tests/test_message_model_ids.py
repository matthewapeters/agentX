"""Unit tests for message identifier behavior."""

import os

import pytest

from shared.models.message import Message, MessageRole, is_valid_message_id


@pytest.mark.unit
def test_message_auto_generates_valid_message_id() -> None:
    """GIVEN a new message without an ID WHEN constructed THEN a valid message_id is generated."""
    msg = Message(role=MessageRole.USER, content="hello")

    assert msg.message_id is not None
    assert is_valid_message_id(msg.message_id)


@pytest.mark.unit
def test_message_preserves_explicit_message_id() -> None:
    """GIVEN an explicit message ID WHEN message is constructed THEN that ID is preserved."""
    explicit_id = "msg_0123456789abcdef0123456789abcdef"

    msg = Message(role=MessageRole.ASSISTANT, content="ok", message_id=explicit_id)

    assert msg.message_id == explicit_id


@pytest.mark.unit
def test_message_id_uniqueness_for_multiple_messages() -> None:
    """GIVEN many generated messages WHEN IDs are collected THEN all IDs are unique."""
    messages = [Message(role=MessageRole.USER, content=f"m{i}") for i in range(50)]
    ids = [m.message_id for m in messages]

    assert len(ids) == len(set(ids))


@pytest.mark.unit
def test_to_dict_includes_message_identity_fields() -> None:
    """GIVEN a message with identity metadata WHEN serialized THEN identity fields are present."""
    msg = Message(
        role=MessageRole.ASSISTANT,
        content="final",
        cloned_from="msg_11111111111111111111111111111111",
        superseded_by="msg_22222222222222222222222222222222",
        synthesis_of=["msg_33333333333333333333333333333333"],
    )

    data = msg.to_dict()

    assert data["message_id"] == msg.message_id
    assert data["cloned_from"] == msg.cloned_from
    assert data["superseded_by"] == msg.superseded_by
    assert data["synthesis_of"] == msg.synthesis_of


@pytest.mark.unit
def test_from_dict_requires_message_id() -> None:
    """GIVEN serialized message data without message_id WHEN loading THEN validation fails."""
    payload = {"role": "user", "content": "legacy"}

    with pytest.raises(ValueError, match="message_id"):
        Message.from_dict(payload)


@pytest.mark.unit
def test_from_dict_round_trips_identity_fields() -> None:
    """GIVEN serialized identity fields WHEN loaded THEN they round-trip unchanged."""
    payload = {
        "role": "assistant",
        "content": "done",
        "message_id": "msg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "cloned_from": "msg_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
        "superseded_by": "msg_cccccccccccccccccccccccccccccccc",
        "synthesis_of": ["msg_dddddddddddddddddddddddddddddddd"],
    }

    msg = Message.from_dict(payload)

    assert msg.message_id == payload["message_id"]
    assert msg.cloned_from == payload["cloned_from"]
    assert msg.superseded_by == payload["superseded_by"]
    assert msg.synthesis_of == payload["synthesis_of"]


@pytest.mark.unit
def test_save_uses_filename_with_message_id(tmp_path) -> None:
    """GIVEN a new message WHEN saved THEN filename contains epoch, role, and message_id."""
    msg = Message(role=MessageRole.USER, content="save me")

    msg.save(str(tmp_path))

    assert msg.file_path is not None
    filename = os.path.basename(msg.file_path)
    assert filename.endswith(f"_{msg.message_id}.json")
    assert f"_{MessageRole.USER.value}_" in filename
