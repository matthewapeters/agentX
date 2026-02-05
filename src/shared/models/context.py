"""
Unified Context model for AgentX and Agentix.

Context represents a conversation session's message history. It is stored
client-side in the AgentX session folder and passed to Agentix server
in request payloads.
"""

from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Iterator, Optional
import json
import os
from glob import glob

from .message import Message, MessageRole


@dataclass
class Context:
    """
    Represents a conversation context (session).
    
    Context is the canonical storage for all messages in a conversation.
    It is owned and persisted by the AgentX client. When making requests
    to the Agentix server, the relevant context is serialized and included
    in the request payload.
    
    Attributes:
        path: Directory path where context messages are stored
        session_id: Unique identifier for this session
        messages: List of (timestamp, message) tuples
        expanded: GUI state for expand/collapse (client-side only)
    """
    
    path: Optional[str] = None
    session_id: Optional[str] = None
    messages: list[tuple[datetime, Message]] = field(default_factory=list)
    expanded: bool = True  # GUI state
    
    def add_message(self, message: Message, ts: Optional[datetime] = None) -> None:
        """
        Add a message to the context.
        
        Args:
            message: The message to add
            ts: Optional timestamp (uses message.timestamp if not provided)
        """
        timestamp = ts or message.timestamp
        message.timestamp = timestamp
        
        # Save to disk if path is set
        if self.path and message.file_path is None:
            message.save(self.path)
        
        self.messages.append((timestamp, message))
    
    def get_messages(self, enabled_only: bool = True) -> list[Message]:
        """
        Get messages from context.
        
        Args:
            enabled_only: If True, only return enabled messages
            
        Returns:
            List of messages
        """
        if enabled_only:
            return [msg for _, msg in self.messages if msg.enabled]
        return [msg for _, msg in self.messages]
    
    def get_enabled_messages(self) -> Iterator[Message]:
        """
        Iterate over enabled messages.
        
        Yields:
            Enabled Message objects
        """
        for _, msg in self.messages:
            if msg.enabled:
                yield msg
    
    def to_llm_messages(self) -> list[dict]:
        """
        Format all enabled messages for LLM API.
        
        Returns:
            List of message dicts suitable for Ollama/OpenAI
        """
        return [msg.to_llm_dict() for msg in self.get_enabled_messages()]
    
    def to_dict(self) -> dict:
        """
        Serialize context for transmission to Agentix server.
        
        Returns:
            Dictionary containing serialized context
        """
        return {
            "session_id": self.session_id,
            "messages": [msg.to_dict() for _, msg in self.messages],
        }
    
    def to_payload(self, enabled_only: bool = True) -> list[dict]:
        """
        Create payload for Agentix server request.
        
        Args:
            enabled_only: If True, only include enabled messages
            
        Returns:
            List of message dicts for API payload
        """
        messages = self.get_messages(enabled_only=enabled_only)
        return [msg.to_dict() for msg in messages]
    
    @classmethod
    def from_dict(cls, data: dict, path: Optional[str] = None) -> "Context":
        """
        Create Context from dictionary (e.g., from Agentix server response).
        
        Args:
            data: Dictionary containing context data
            path: Optional path for persistence
            
        Returns:
            Context instance
        """
        context = cls(
            path=path,
            session_id=data.get("session_id"),
        )
        
        for msg_data in data.get("messages", []):
            msg = Message.from_dict(msg_data)
            context.messages.append((msg.timestamp, msg))
        
        return context
    
    def load(self, path: Optional[str] = None) -> None:
        """
        Load messages from disk.
        
        Args:
            path: Directory to load from (uses self.path if not provided)
        """
        load_path = path or self.path
        if not load_path:
            raise ValueError("No path specified for loading context")
        
        self.path = load_path
        self.messages = []
        
        # Find all JSON files in the context directory
        pattern = os.path.join(load_path, "*.json")
        files = sorted(glob(pattern))
        
        for file_path in files:
            try:
                msg = Message.load(file_path)
                # Default loaded messages to disabled (user must enable)
                msg.enabled = False
                for att in msg.attachments:
                    att.enabled = False
                self.messages.append((msg.timestamp, msg))
            except Exception as e:
                print(f"Warning: Could not load message from {file_path}: {e}")
    
    def save(self) -> None:
        """Save all messages to disk."""
        if not self.path:
            raise ValueError("No path specified for saving context")
        
        os.makedirs(self.path, exist_ok=True)
        
        for _, msg in self.messages:
            if msg.file_path is None:
                msg.save(self.path)
    
    def clear(self) -> None:
        """Clear all messages from context (does not delete files)."""
        self.messages = []
    
    def __len__(self) -> int:
        """Return number of messages in context."""
        return len(self.messages)
    
    def __iter__(self) -> Iterator[tuple[datetime, Message]]:
        """Iterate over (timestamp, message) tuples."""
        return iter(self.messages)
    
    # Utility methods for message filtering
    
    def get_user_messages(self) -> list[Message]:
        """Get all user messages."""
        return [msg for _, msg in self.messages if msg.role == MessageRole.USER]
    
    def get_assistant_messages(self) -> list[Message]:
        """Get all assistant messages."""
        return [msg for _, msg in self.messages if msg.role == MessageRole.ASSISTANT]
    
    def get_tool_messages(self) -> list[Message]:
        """Get all tool-related messages."""
        return [
            msg for _, msg in self.messages 
            if msg.role in (MessageRole.TOOL_CALL, MessageRole.TOOL_RESULT)
        ]
    
    def get_last_user_message(self) -> Optional[Message]:
        """Get the most recent user message."""
        for _, msg in reversed(self.messages):
            if msg.role == MessageRole.USER:
                return msg
        return None
    
    def get_last_assistant_message(self) -> Optional[Message]:
        """Get the most recent assistant message."""
        for _, msg in reversed(self.messages):
            if msg.role == MessageRole.ASSISTANT:
                return msg
        return None
    
    # Token management
    
    def estimate_tokens(self) -> int:
        """
        Estimate total tokens in enabled messages.
        
        Uses rough approximation of 1 token per 4 characters.
        For accurate counting, use a proper tokenizer.
        
        Returns:
            Estimated token count
        """
        total = 0
        for msg in self.get_enabled_messages():
            # Message content
            total += len(msg.content) // 4
            # Attachments
            for att in msg.attachments:
                if att.enabled:
                    total += len(att.content) // 4
        return total
    
    def trim_to_tokens(self, max_tokens: int) -> list[Message]:
        """
        Get messages that fit within token limit.
        
        Strategy:
        1. Always keep system messages
        2. Always keep most recent user message
        3. Include recent messages until limit reached
        
        Args:
            max_tokens: Maximum tokens to include
            
        Returns:
            List of messages within token budget
        """
        # Separate by priority
        system_msgs = [
            msg for _, msg in self.messages 
            if msg.role == MessageRole.SYSTEM and msg.enabled
        ]
        other_msgs = [
            msg for _, msg in self.messages 
            if msg.role != MessageRole.SYSTEM and msg.enabled
        ]
        
        # Calculate system token budget
        system_tokens = sum(len(m.content) // 4 for m in system_msgs)
        available = max_tokens - system_tokens
        
        # Add messages from most recent, respecting limit
        trimmed = []
        current_tokens = 0
        
        for msg in reversed(other_msgs):
            msg_tokens = len(msg.content) // 4
            for att in msg.attachments:
                if att.enabled:
                    msg_tokens += len(att.content) // 4
            
            if current_tokens + msg_tokens <= available:
                trimmed.insert(0, msg)
                current_tokens += msg_tokens
            else:
                break
        
        return system_msgs + trimmed
