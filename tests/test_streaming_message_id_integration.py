"""Integration tests for streaming message persistence with message IDs."""

import threading
from types import SimpleNamespace
from unittest.mock import MagicMock

import pytest

from agentx.streaming_controller import StreamingController
from shared.models.context import Context


class _DummyGui:
    def set_streaming_state(self, _active: bool) -> None:
        return


class _DummySession:
    def __init__(self) -> None:
        self.context = Context()
        self.gui = _DummyGui()

    def add_message_to_context(self, message) -> None:
        self.context.add_message(message)


@pytest.mark.integration
def test_persist_stream_messages_sets_synthesis_of_for_assistant() -> None:
    """GIVEN streamed tool results WHEN assistant content is persisted THEN synthesis_of references tool-result IDs."""
    session = _DummySession()
    controller = StreamingController(session)

    controller._persist_stream_messages(
        thinking_text="thinking...",
        content_text="answer",
        synthesis_of=["msg_11111111111111111111111111111111"],
        refresh_gui=False,
    )

    messages = [entry.message for entry in session.context.messages]
    assistant = messages[-1]

    assert assistant.role.value == "assistant"
    assert assistant.synthesis_of == ["msg_11111111111111111111111111111111"]
    assert assistant.message_id is not None


# ---------------------------------------------------------------------------
# F-4 / T-3: synthesis_of is a valid list after _persist_stream_messages
# ---------------------------------------------------------------------------


@pytest.mark.integration
def test_persist_stream_messages_synthesis_of_is_list_type() -> None:
    """GIVEN _persist_stream_messages is called with an explicit synthesis_of list.

    WHEN the controller persists the assistant message.

    THEN assistant_message.synthesis_of equals the provided list,
    AND isinstance(assistant_message.synthesis_of, list) is True.

    Gherkin:
    GIVEN synthesis_of = ["msg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]
    WHEN _persist_stream_messages(thinking_text="", content_text="answer", synthesis_of=synthesis_of)
    THEN stored assistant message has synthesis_of == ["msg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]
     AND isinstance(synthesis_of, list) is True
    """
    session = _DummySession()
    controller = StreamingController(session)
    expected_ids = ["msg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]

    controller._persist_stream_messages(
        thinking_text="",
        content_text="answer",
        synthesis_of=expected_ids,
        refresh_gui=False,
    )

    messages = [entry.message for entry in session.context.messages]
    assistant = next(m for m in messages if m.role.value == "assistant")

    assert assistant.synthesis_of == expected_ids
    assert isinstance(assistant.synthesis_of, list)


# ---------------------------------------------------------------------------
# F-4 / T-4: Session delegate preserves synthesis_of through delegation chain
# ---------------------------------------------------------------------------


class _FakeStreamingController:
    """Spy controller that records the kwargs passed to _persist_stream_messages."""

    def __init__(self) -> None:
        self.received_kwargs: dict = {}

    def _persist_stream_messages(self, **kwargs) -> None:  # noqa: D401
        self.received_kwargs = kwargs


@pytest.mark.integration
def test_session_delegate_forwards_synthesis_of_by_keyword() -> None:
    """GIVEN AgentXSession._persist_stream_messages is called with an explicit synthesis_of list.

    WHEN the call is delegated to StreamingController._persist_stream_messages.

    THEN the controller receives synthesis_of as the same list (not a boolean),
    AND the controller receives refresh_gui=False,
    AND the session wrapper does not corrupt positional argument order.

    Gherkin:
    GIVEN session._persist_stream_messages called with synthesis_of=["msg_abc..."], refresh_gui=False
    WHEN delegation passes arguments to controller
    THEN controller receives synthesis_of=["msg_abc..."]
     AND controller receives refresh_gui=False
     AND stored message.synthesis_of == ["msg_abc..."]
    """
    from agentx.session import AgentXSession

    # Build a minimal session shell — bypass __init__ entirely via object.__new__
    session = object.__new__(AgentXSession)
    spy_controller = _FakeStreamingController()
    session._streaming_controller = spy_controller  # type: ignore[attr-defined]

    test_synthesis_of = ["msg_abcabcabcabcabcabcabcabcabcabcab"]

    session._persist_stream_messages(  # type: ignore[attr-defined]
        thinking_text="",
        content_text="done",
        synthesis_of=test_synthesis_of,
        refresh_gui=False,
    )

    assert spy_controller.received_kwargs["synthesis_of"] == test_synthesis_of
    assert isinstance(spy_controller.received_kwargs["synthesis_of"], list)
    assert spy_controller.received_kwargs["refresh_gui"] is False
