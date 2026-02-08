"""AgentX Message compatibility shim."""

from shared.models.message import Message, MessageRole, ROLE_ICONS

USER = MessageRole.USER.value
ASSISTANT = MessageRole.ASSISTANT.value
SYSTEM = MessageRole.SYSTEM.value
THINKING = MessageRole.THINKING.value
TOOL_CALL = MessageRole.TOOL_CALL.value
TOOL_RESULT = MessageRole.TOOL_RESULT.value

__all__ = [
    "Message",
    "MessageRole",
    "ROLE_ICONS",
    "USER",
    "ASSISTANT",
    "SYSTEM",
    "THINKING",
    "TOOL_CALL",
    "TOOL_RESULT",
]