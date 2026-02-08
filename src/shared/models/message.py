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
}


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
    
    def __post_init__(self):
        """Ensure role is MessageRole enum."""
        if isinstance(self.role, str):
            self.role = MessageRole(self.role)
    
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
            "attachments": [a.to_dict() for a in self.attachments],
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
        # Parse attachments
        attachments = []
        for a in data.get("attachments", []):
            if isinstance(a, dict):
                attachments.append(Attachment.from_dict(a))
            elif isinstance(a, Attachment):
                attachments.append(a)
        
        # Parse timestamp
        epoch = data.get("epoch", 0)
        timestamp = datetime.fromtimestamp(epoch) if epoch else datetime.now()
        
        return cls(
            role=MessageRole(data.get("role", "user")),
            content=data.get("content", ""),
            attachments=attachments,
            enabled=data.get("enabled", True),
            timestamp=timestamp,
            file_path=file_path or data.get("file"),
            tool_name=data.get("tool_name"),
            tool_input=data.get("tool_input"),
            tool_output=data.get("tool_output"),
            tool_id=data.get("tool_id"),
            classification=data.get("classification"),
        )
    
    def to_llm_dict(self) -> dict:
        """
        Format message for LLM API (Ollama/OpenAI).
        
        Includes enabled attachment content inline in the content field.
        Handles role mapping for non-standard roles.
        
        Returns:
            Dictionary suitable for Ollama/OpenAI chat API
        """
        # Map non-standard roles to standard ones
        role_mapping = {
            MessageRole.THINKING: "assistant",
            MessageRole.TOOL_CALL: "assistant",
            MessageRole.TOOL_RESULT: "user",  # Tool results go back as user context
        }
        api_role = role_mapping.get(self.role, self.role.value)
        
        # Build content with enabled attachments
        full_content = self.content
        
        # Add tool info for tool messages
        if self.role == MessageRole.TOOL_CALL and self.tool_name:
            full_content = f"[Tool Call: {self.tool_name}]\nInput: {json.dumps(self.tool_input or {}, indent=2)}"
        elif self.role == MessageRole.TOOL_RESULT and self.tool_name:
            output_str = json.dumps(self.tool_output, indent=2) if isinstance(self.tool_output, (dict, list)) else str(self.tool_output)
            full_content = f"[Tool Result: {self.tool_name}]\n{output_str}"
        
        # Append enabled attachments
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
            filename = f"{self.epoch}_{self.role.value}.json"
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
    for item in (attachments or []):
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
