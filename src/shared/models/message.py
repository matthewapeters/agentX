"""
Unified Message model for AgentX and Agentix.

Messages are the core unit of conversation context. They are stored client-side
in the AgentX session folder and passed to Agentix server in request payloads.
"""

from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any, Optional
import json
import os
import re
import uuid

from .attachment import Attachment


class MessageRole(Enum):
    """
    Roles for messages in the conversation.

    Standard roles:
        USER: User input
        ASSISTANT: LLM response
        SYSTEM: System prompts

    Extended roles for agent functionality:
        THINKING: LLM reasoning (may be hidden from user)
        TOOL_CALL: Request to execute a tool
        TOOL_RESULT: Result from tool execution
    """

    USER = "user"
    ASSISTANT = "assistant"
    SYSTEM = "system"
    THINKING = "thinking"
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"

    # Hierarchical task execution roles (Phase 1)
    PLAN = "plan"  # Top-level plan record
    TASK_NODE = "task_node"  # Individual task/sub-task node
    SYNTHESIS = "synthesis"  # Versioned synthesis attempt
    ASSERTION = "assertion"  # Per-assertion verdict record

    def __eq__(self, other):
        if isinstance(other, str):
            return self.value == other
        return Enum.__eq__(self, other)

    def __hash__(self) -> int:
        return Enum.__hash__(self)

    def upper(self) -> str:
        return self.value.upper()


# Role display icons for GUI
ROLE_ICONS = {
    MessageRole.USER: "👤",
    MessageRole.ASSISTANT: "🤖",
    MessageRole.SYSTEM: "⚙️",
    MessageRole.THINKING: "💭",
    MessageRole.TOOL_CALL: "🔧",
    MessageRole.TOOL_RESULT: "📋",
    MessageRole.PLAN: "📋",
    MessageRole.TASK_NODE: "🌿",
    MessageRole.SYNTHESIS: "💡",
    MessageRole.ASSERTION: "✅",
}


MESSAGE_ID_PATTERN = re.compile(r"^msg_[0-9a-f]{32}$")


def _new_message_id() -> str:
    """Generate a new message identifier."""
    return f"msg_{uuid.uuid4().hex}"


def is_valid_message_id(message_id: str) -> bool:
    """Return True when ``message_id`` matches the expected format."""
    return bool(MESSAGE_ID_PATTERN.match(message_id))


@dataclass
class Message:
    """
    Represents a message in the conversation context.

    Messages are stored client-side in JSON files within the session folder.
    When sending context to Agentix server, messages are serialized and
    included in the request payload.

    Attributes:
        role: The role of the message sender
        content: The text content of the message
        attachments: List of file attachments
        enabled: Whether this message is included in LLM context
        timestamp: When the message was created
        file_path: Path to the persisted JSON file (client-side only)

        # Tool-specific fields
        tool_name: Name of the tool (for TOOL_CALL and TOOL_RESULT)
        tool_input: Arguments passed to the tool (for TOOL_CALL)
        tool_output: Result from tool execution (for TOOL_RESULT)

        # Classification metadata (for USER messages)
        classification: Intent classification from Agentix
    """

    role: MessageRole
    content: str
    attachments: list[Attachment] = field(default_factory=list)
    enabled: bool = True
    timestamp: datetime = field(default_factory=datetime.now)
    file_path: Optional[str] = None

    # Tool-specific fields
    tool_name: Optional[str] = None
    tool_input: Optional[dict] = None
    tool_output: Optional[Any] = None
    tool_id: Optional[str] = None

    # Classification metadata
    classification: Optional[dict] = None

    # Plan / task-node metadata (Phase 1)
    plan_id: Optional[str] = None
    plan_name: Optional[str] = None
    task_id: Optional[str] = None
    parent_task_id: Optional[str] = None
    task_depth: Optional[int] = None
    task_data: Optional[dict] = None  # full PlanRecord or TaskNodeRecord dict
    message_id: Optional[str] = None
    cloned_from: Optional[str] = None
    superseded_by: Optional[str] = None
    synthesis_of: list[str] = field(default_factory=list)

    def __post_init__(self):
        """Ensure role and message identity fields are valid."""
        if isinstance(self.role, str):
            self.role = MessageRole(self.role)

        if self.message_id is None:
            self.message_id = _new_message_id()

        if not is_valid_message_id(self.message_id):
            raise ValueError(f"Invalid message_id format: {self.message_id!r}")

        if not isinstance(self.synthesis_of, list):
            raise ValueError(
                f"synthesis_of must be a list[str], got {type(self.synthesis_of).__name__!r}: {self.synthesis_of!r}"
            )

    @property
    def icon(self) -> str:
        """Get display icon for this message's role."""
        return ROLE_ICONS.get(self.role, "❓")

    @property
    def epoch(self) -> float:
        """Get timestamp as epoch float."""
        return self.timestamp.timestamp()

    def attach(self, file_path: str) -> None:
        """
        Attach a file to this message.

        Args:
            file_path: Path to the file to attach
        """
        attachment = Attachment.from_file(file_path)
        self.attachments.append(attachment)
        self._auto_save()

    def detach(self, file_path: str) -> None:
        """
        Remove an attachment from this message.

        Args:
            file_path: Path of the attachment to remove
        """
        self.attachments = [a for a in self.attachments if a.file_path != file_path]
        self._auto_save()

    def _auto_save(self) -> None:
        """Save message if it has a file path."""
        if self.file_path:
            self.save(os.path.dirname(self.file_path))

    def to_dict(self) -> dict:
        """
        Serialize message for storage or transmission.

        This format is used for:
        - Persisting to JSON files (client-side)
        - Sending to Agentix server in request payloads
        """
        data = {
            "role": self.role.value,
            "content": self.content,
            "enabled": self.enabled,
            "epoch": self.epoch,
            "attachments": [a.to_dict() for a in (self.attachments or [])],
            "message_id": self.message_id,
            "synthesis_of": self.synthesis_of,
        }

        # Include optional fields if present
        if self.file_path:
            data["file"] = self.file_path
        if self.tool_name:
            data["tool_name"] = self.tool_name
        if self.tool_input:
            data["tool_input"] = self.tool_input
        if self.tool_output is not None:
            data["tool_output"] = self.tool_output
        if self.tool_id:
            data["tool_id"] = self.tool_id
        if self.classification:
            data["classification"] = self.classification
        if self.plan_id:
            data["plan_id"] = self.plan_id
        if self.plan_name:
            data["plan_name"] = self.plan_name
        if self.task_id:
            data["task_id"] = self.task_id
        if self.parent_task_id:
            data["parent_task_id"] = self.parent_task_id
        if self.task_depth is not None:
            data["task_depth"] = self.task_depth
        if self.task_data is not None:
            data["task_data"] = self.task_data
        if self.cloned_from:
            data["cloned_from"] = self.cloned_from
        if self.superseded_by:
            data["superseded_by"] = self.superseded_by

        return data

    # Alias for backward compatibility with AgentX
    def serialize(self) -> dict:
        """Alias for to_dict() for backward compatibility."""
        return self.to_dict()

    @classmethod
    def from_dict(cls, data: dict, file_path: Optional[str] = None) -> "Message":
        """
        Create Message from dictionary.

        Args:
            data: Dictionary containing message data
            file_path: Optional file path override

        Returns:
            Message instance
        """
        # Parse attachments (guard against null stored in JSON)
        attachments = []
        for a in data.get("attachments") or []:
            if isinstance(a, dict):
                attachments.append(Attachment.from_dict(a))
            elif isinstance(a, Attachment):
                attachments.append(a)

        # Parse timestamp
        epoch = data.get("epoch", 0)
        timestamp = datetime.fromtimestamp(epoch) if epoch else datetime.now()

        message_id = data.get("message_id")
        if not isinstance(message_id, str):
            raise ValueError("Missing required message_id in serialized message")

        return cls(
            role=MessageRole(data.get("role", "user")),
            content=data.get("content", ""),
            attachments=attachments,
            enabled=data.get("enabled", True),
            timestamp=timestamp,
            file_path=file_path or data.get("file"),
            message_id=message_id,
            tool_name=data.get("tool_name"),
            tool_input=data.get("tool_input"),
            tool_output=data.get("tool_output"),
            tool_id=data.get("tool_id"),
            classification=data.get("classification"),
            plan_id=data.get("plan_id"),
            plan_name=data.get("plan_name"),
            task_id=data.get("task_id"),
            parent_task_id=data.get("parent_task_id"),
            task_depth=data.get("task_depth"),
            task_data=data.get("task_data"),
            cloned_from=data.get("cloned_from"),
            superseded_by=data.get("superseded_by"),
            synthesis_of=data.get("synthesis_of") if isinstance(data.get("synthesis_of"), list) else [],
        )

    def to_llm_dict(self) -> dict:
        """
        Format message for LLM API (Ollama/OpenAI wire format).

        TOOL_CALL messages serialize as the OpenAI assistant message with a
        ``tool_calls`` array (not plain text). TOOL_RESULT messages use role
        ``"tool"`` with ``tool_call_id``.  All other roles follow the standard
        role mapping.

        Returns:
            Dictionary suitable for Ollama/OpenAI chat API
        """
        # --- Tool call: assistant message with tool_calls array ---
        if self.role == MessageRole.TOOL_CALL:
            return {
                "role": "assistant",
                "content": self.content or "",
                "tool_calls": [
                    {
                        "id": self.tool_id or "call_0",
                        "type": "function",
                        "function": {
                            "name": self.tool_name or "",
                            "arguments": json.dumps(self.tool_input or {}),
                        },
                    }
                ],
            }

        # --- Tool result: "tool" role with tool_call_id ---
        if self.role == MessageRole.TOOL_RESULT:
            output = self.tool_output
            if isinstance(output, (dict, list)):
                content = json.dumps(output)
            elif output is not None:
                content = str(output)
            else:
                content = self.content or ""
            return {
                "role": "tool",
                "content": content,
                "tool_call_id": self.tool_id or "call_0",
            }

        # --- Internal-only roles: must never reach the LLM API ---
        _internal_roles = {MessageRole.PLAN, MessageRole.TASK_NODE, MessageRole.SYNTHESIS, MessageRole.ASSERTION}
        if self.role in _internal_roles:
            raise ValueError(
                f"Message with role {self.role!r} is an internal task-execution record "
                "and must not be serialised for the LLM API. "
                "Filter these messages before calling to_llm_dict()."
            )

        # --- Standard roles (user / assistant / system / thinking) ---
        role_mapping = {
            MessageRole.THINKING: "assistant",
        }
        api_role = role_mapping.get(self.role, self.role.value)

        # Build content with enabled attachments
        full_content = self.content

        enabled_attachments = [a for a in self.attachments if a.enabled]
        if enabled_attachments:
            full_content += "\n\n--- Attached Files ---"
            for attachment in enabled_attachments:
                full_content += attachment.to_llm_format()

        return {
            "role": api_role,
            "content": full_content,
        }

    # Alias for backward compatibility with AgentX
    def llm_message_dict(self) -> dict:
        """Alias for to_llm_dict() for backward compatibility."""
        return self.to_llm_dict()

    def save(self, context_path: str) -> None:
        """
        Save message to JSON file in context folder.

        Args:
            context_path: Directory to save the message file
        """
        if self.file_path is None:
            filename = f"{self.epoch}_{self.role.value}_{self.message_id}.json"
            self.file_path = os.path.join(context_path, filename)

        os.makedirs(context_path, exist_ok=True)

        with open(self.file_path, "w", encoding="utf-8") as f:
            json.dump(self.to_dict(), f, indent=2)

    @classmethod
    def load(cls, file_path: str) -> "Message":
        """
        Load message from JSON file.

        Args:
            file_path: Path to the JSON file

        Returns:
            Message instance
        """
        with open(file_path, "r", encoding="utf-8") as f:
            data = json.load(f)
        return cls.from_dict(data, file_path=file_path)


# Factory functions for common message types


def user_message(content: str, attachments: list[Any] = None) -> Message:
    """Create a user message with optional file attachments."""
    msg = Message(role=MessageRole.USER, content=content)
    for item in attachments or []:
        if isinstance(item, Attachment):
            msg.attachments.append(item)
        else:
            msg.attach(item)
    return msg


def assistant_message(content: str) -> Message:
    """Create an assistant message."""
    return Message(role=MessageRole.ASSISTANT, content=content)


def system_message(content: str) -> Message:
    """Create a system message."""
    return Message(role=MessageRole.SYSTEM, content=content)


def thinking_message(content: str) -> Message:
    """Create a thinking/reasoning message."""
    return Message(role=MessageRole.THINKING, content=content, enabled=False)


def tool_call_message(
    tool_name: str,
    tool_input: dict,
    tool_id: Optional[str] = None,
    content: Optional[str] = None,
) -> Message:
    """Create a tool call message."""
    return Message(
        role=MessageRole.TOOL_CALL,
        content=content or f"Calling {tool_name}",
        tool_name=tool_name,
        tool_input=tool_input,
        tool_id=tool_id,
    )


def tool_result_message(
    tool_id: Optional[str] = None,
    content: Optional[str] = None,
    tool_name: Optional[str] = None,
    tool_output: Any = None,
    success: bool = True,
) -> Message:
    """Create a tool result message."""
    if content is None and tool_name:
        content = f"Result from {tool_name}" if success else f"Error from {tool_name}"
    return Message(
        role=MessageRole.TOOL_RESULT,
        content=content or "",
        tool_name=tool_name,
        tool_output=tool_output,
        tool_id=tool_id,
    )
