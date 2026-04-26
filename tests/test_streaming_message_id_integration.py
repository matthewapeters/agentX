"""Integration tests for streaming message persistence with message IDs."""

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
