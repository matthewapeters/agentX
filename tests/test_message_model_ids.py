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


# ---------------------------------------------------------------------------
# F-4 / T-5: synthesis_of boolean corruption rejected at deserialization
# ---------------------------------------------------------------------------


@pytest.mark.unit
@pytest.mark.parametrize(
    "bad_synthesis_of",
    [
        True,
        False,
        "msg_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
        42,
        {"key": "value"},
    ],
)
def test_from_dict_coerces_non_list_synthesis_of_to_empty_list(bad_synthesis_of) -> None:
    """GIVEN a JSON message payload where synthesis_of contains a non-list truthy value.

    WHEN Message.from_dict is called with that payload.

    THEN message.synthesis_of equals [] (coerced to empty list, not storing the bad value),
    AND isinstance(message.synthesis_of, list) is True.

    Gherkin:
    GIVEN payload = {"role": "assistant", "content": "...", "message_id": "msg_abc...",
                     "synthesis_of": <bad_value>}
    WHEN Message.from_dict(payload)
    THEN resulting message.synthesis_of == []
     AND isinstance(message.synthesis_of, list) is True

    Permutations:
        - bad_synthesis_of=True  (boolean True from F-2 scenario E corruption)
        - bad_synthesis_of=False (boolean False from F-2 scenario D corruption)
        - bad_synthesis_of="msg_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4" (stray string instead of list)
        - bad_synthesis_of=42 (integer)
        - bad_synthesis_of={"key": "value"} (dict instead of list)
    """
    payload = {
        "role": "assistant",
        "content": "answer",
        "message_id": "msg_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
        "synthesis_of": bad_synthesis_of,
    }

    msg = Message.from_dict(payload)

    assert (
        msg.synthesis_of == []
    ), f"Expected synthesis_of=[] when input is {bad_synthesis_of!r}, got {msg.synthesis_of!r}"
    assert isinstance(msg.synthesis_of, list)


@pytest.mark.unit
def test_post_init_rejects_non_list_synthesis_of() -> None:
    """GIVEN a Message is constructed with a non-list synthesis_of value.

    WHEN the dataclass __post_init__ runs.

    THEN a ValueError is raised.

    Gherkin:
    GIVEN synthesis_of = True (a boolean, not a list)
    WHEN Message(role=..., content=..., synthesis_of=True)
    THEN ValueError is raised with a message about synthesis_of type
    """
    with pytest.raises(ValueError, match="synthesis_of"):
        Message(
            role=MessageRole.ASSISTANT,
            content="bad",
            synthesis_of=True,  # type: ignore[arg-type]
        )
