"""
Unified Context model for AgentX and Agentix.

Context represents a conversation session's message history. It is stored
client-side in the AgentX session folder and passed to Agentix server
in request payloads.
"""

import json
import logging
import os
import threading
from dataclasses import dataclass, field
from datetime import datetime
from glob import glob
from pathlib import Path
from typing import Iterator, Optional

logger = logging.getLogger(__name__)

from ..token_utils import chars_per_token, estimate_text_tokens
from .message import Message, MessageRole, is_valid_message_id, tool_call_message, tool_result_message
from .task_node import PlanRecord, TaskNodeRecord, TaskTree


@dataclass
class MessageEntry:
    """Compatibility wrapper for context message storage."""

    timestamp: datetime
    message: Message

    def __iter__(self):
        yield self.timestamp
        yield self.message

    def __getattr__(self, name: str):
        return getattr(self.message, name)

    def __setattr__(self, name: str, value):
        if name in {"timestamp", "message"}:
            object.__setattr__(self, name, value)
            return
        if hasattr(self.__dict__.get("message", None), name):
            setattr(self.message, name, value)
        else:
            object.__setattr__(self, name, value)


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
    messages: list[MessageEntry] = field(default_factory=list)
    metadata: dict = field(default_factory=dict)
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

        if not message.message_id or not is_valid_message_id(message.message_id):
            raise ValueError(f"Invalid message_id on add_message: {message.message_id!r}")
        if self.get_message_by_id(message.message_id) is not None:
            raise ValueError(f"Duplicate message_id in context: {message.message_id}")

        # Save to disk if path is set (safe from any thread — writes unique filenames)
        if self.path and message.file_path is None:
            message.save(self.path)

        self.messages.append(MessageEntry(timestamp=timestamp, message=message))

    def get_messages(self, enabled_only: bool = True) -> list[Message]:
        """
        Get messages from context.

        Args:
            enabled_only: If True, only return enabled messages

        Returns:
            List of messages
        """
        if enabled_only:
            return [entry.message for entry in self.messages if entry.enabled]
        return [entry.message for entry in self.messages]

    def get_enabled_messages(self) -> list[Message]:
        """Return enabled messages as a list."""
        return [entry.message for entry in self.messages if entry.enabled]

    def to_llm_messages(self) -> list[dict]:
        """
        Format all enabled messages for LLM API.

        Internal task-execution roles (PLAN, TASK_NODE, SYNTHESIS, ASSERTION)
        are automatically excluded — they must not reach the LLM API.

        Returns:
            List of message dicts suitable for Ollama/OpenAI
        """
        _internal = {MessageRole.PLAN, MessageRole.TASK_NODE, MessageRole.SYNTHESIS, MessageRole.ASSERTION}
        return [msg.to_llm_dict() for msg in self.get_enabled_messages() if msg.role not in _internal]

    def to_dict(self) -> dict:
        """
        Serialize context for transmission to Agentix server.

        Returns:
            Dictionary containing serialized context
        """
        return {
            "session_id": self.session_id,
            "metadata": self.metadata,
            "messages": [entry.message.to_dict() for entry in self.messages],
        }

    def to_payload(
        self,
        model: Optional[str] = None,
        stream: Optional[bool] = None,
        options: Optional[dict] = None,
        enabled_only: bool = True,
    ) -> dict:
        """Create payload for Agentix server request."""
        messages = self.get_messages(enabled_only=enabled_only)
        payload = {
            "messages": [msg.to_llm_dict() for msg in messages],
        }
        if model is not None:
            payload["model"] = model
        if stream is not None:
            payload["stream"] = stream
        if options is not None:
            payload["options"] = options
        return payload

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

        context.metadata = data.get("metadata", {})

        for msg_data in data.get("messages", []):
            msg = Message.from_dict(msg_data)
            if context.get_message_by_id(msg.message_id) is not None:
                raise ValueError(f"Duplicate message_id in serialized context: {msg.message_id}")
            context.messages.append(MessageEntry(timestamp=msg.timestamp, message=msg))

        return context

    def load_from_dir(self, path: Optional[str] = None) -> None:
        """Load messages from a context directory on disk."""
        load_path = path or self.path
        if not load_path:
            raise ValueError("No path specified for loading context")

        self.path = load_path
        self.messages = []

        pattern = os.path.join(load_path, "*.json")
        files = sorted(glob(pattern))

        for file_path in files:
            try:
                msg = Message.load(file_path)
                if not msg.message_id or not is_valid_message_id(msg.message_id):
                    raise ValueError(f"Invalid message_id in file {file_path}: {msg.message_id!r}")
                if self.get_message_by_id(msg.message_id) is not None:
                    raise ValueError(f"Duplicate message_id while loading context dir: {msg.message_id}")
                msg.enabled = False
                for att in msg.attachments:
                    att.enabled = False
                self.messages.append(MessageEntry(timestamp=msg.timestamp, message=msg))
            except ValueError as exc:
                if "message_id" in str(exc):
                    raise
                logger.warning("Could not load message from %s: %s", file_path, exc)
            except Exception as exc:
                logger.warning("Could not load message from %s: %s", file_path, exc)

    @classmethod
    def load(cls, path: str) -> "Context":
        """Load context from a JSON file or context directory."""
        if os.path.isdir(path):
            context = cls(path=path)
            context.load_from_dir(path)
            return context
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
        return cls.from_dict(data)

    def save(self, path: Optional[str] = None) -> None:
        """Save context to a directory or JSON file."""
        target = path or self.path
        if not target:
            raise ValueError("No path specified for saving context")

        if os.path.isdir(target) or not os.path.splitext(target)[1]:
            os.makedirs(target, exist_ok=True)
            for entry in self.messages:
                if entry.message.file_path is None:
                    entry.message.save(target)
            self.path = target
            return

        with open(target, "w", encoding="utf-8") as f:
            json.dump(self.to_dict(), f, indent=2)

    def clear(self) -> None:
        """Clear all messages from context (does not delete files)."""
        self.messages = []

    def __len__(self) -> int:
        """Return number of messages in context."""
        return len(self.messages)

    def __iter__(self) -> Iterator[Message]:
        """Iterate over message entries."""
        return iter(self.messages)

    # Utility methods for message filtering

    def get_user_messages(self) -> list[Message]:
        """Get all user messages."""
        return [entry.message for entry in self.messages if entry.message.role == MessageRole.USER]

    def get_assistant_messages(self) -> list[Message]:
        """Get all assistant messages."""
        return [entry.message for entry in self.messages if entry.message.role == MessageRole.ASSISTANT]

    def add_tool_call_message(
        self,
        tool_name: str,
        tool_input: dict,
        tool_id: Optional[str] = None,
    ) -> Message:
        """
        Add an assistant tool-call message to the context.

        Produces the correct Ollama wire format (``role: "assistant"`` +
        ``tool_calls`` array) when serialized via ``to_llm_messages()``.

        Args:
            tool_name: Name of the tool being called.
            tool_input: Arguments dict to pass to the tool.
            tool_id: Optional tool-call ID from the LLM for correlation.

        Returns:
            The created Message instance.
        """
        msg = tool_call_message(tool_name, tool_input, tool_id=tool_id)
        self.add_message(msg)
        return msg

    def add_tool_result_message(
        self,
        tool_name: str,
        tool_output,
        tool_id: Optional[str] = None,
        success: bool = True,
    ) -> Message:
        """
        Add a tool-result message to the context.

        Produces the correct Ollama wire format (``role: "tool"`` +
        ``tool_call_id``) when serialized via ``to_llm_messages()``.

        Args:
            tool_name: Name of the tool that was executed.
            tool_output: Result data (dict, list, or str).
            tool_id: Tool-call ID to correlate with the original call.
            success: Whether the tool executed successfully.

        Returns:
            The created Message instance.
        """
        msg = tool_result_message(
            tool_id=tool_id,
            tool_name=tool_name,
            tool_output=tool_output,
            success=success,
        )
        self.add_message(msg)
        return msg

    def get_tool_messages(self) -> list[Message]:
        """Get all tool-related messages."""
        return [
            entry.message
            for entry in self.messages
            if entry.message.role in (MessageRole.TOOL_CALL, MessageRole.TOOL_RESULT)
        ]

    def index_by_message_id(self) -> dict[str, Message]:
        """Return a message index keyed by ``message_id``."""
        return {entry.message.message_id: entry.message for entry in self.messages}

    def get_message_by_id(self, message_id: str) -> Optional[Message]:
        """Return message by ``message_id`` or ``None`` if missing."""
        return self.index_by_message_id().get(message_id)

    def require_message_by_id(self, message_id: str) -> Message:
        """Return message by ID or raise ``KeyError`` if not found."""
        message = self.get_message_by_id(message_id)
        if message is None:
            raise KeyError(f"message_id not found in context: {message_id}")
        return message

    def supersede_message(self, original_id: str, replacement_id: str) -> None:
        """Mark an original message as superseded by a replacement message."""
        original = self.require_message_by_id(original_id)
        self.require_message_by_id(replacement_id)
        original.superseded_by = replacement_id
        original.enabled = False
        if original.file_path:
            original.save(os.path.dirname(original.file_path))

    def get_ancestry(self, message_id: str) -> list[Message]:
        """Return replay ancestry from root original to the specified message."""
        lineage: list[Message] = []
        visited: set[str] = set()
        current = self.get_message_by_id(message_id)
        while current is not None and current.message_id not in visited:
            lineage.append(current)
            visited.add(current.message_id)
            if not current.cloned_from:
                break
            next_message = self.get_message_by_id(current.cloned_from)
            if next_message is None:
                logger.warning("Ancestry chain missing cloned_from target: %s", current.cloned_from)
                break
            current = next_message
        lineage.reverse()
        return lineage

    def get_last_user_message(self) -> Optional[Message]:
        """Get the most recent user message."""
        for entry in reversed(self.messages):
            if entry.message.role == MessageRole.USER:
                return entry.message
        return None

    def get_last_assistant_message(self) -> Optional[Message]:
        """Get the most recent assistant message."""
        for entry in reversed(self.messages):
            if entry.message.role == MessageRole.ASSISTANT:
                return entry.message
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

    @staticmethod
    def _chars_per_token(model_name: str) -> float:
        """Return the TOK-02 char/token ratio for a model name.

        .. deprecated::
            Use :func:`shared.token_utils.chars_per_token` directly.
            This shim is retained for backward compatibility with code that
            calls ``Context._chars_per_token(model)``.
        """
        return chars_per_token(model_name)

    @staticmethod
    def _estimate_text_tokens(text: str, ratio: float) -> int:
        """Estimate token count for ``text``.

        .. deprecated::
            Use :func:`shared.token_utils.estimate_text_tokens` directly.
            This shim is retained for backward compatibility.
        """
        return estimate_text_tokens(text, ratio)

    def token_breakdown(self, model_name: str = "") -> dict[str, int]:
        """Return estimated token counts split by context-meter categories.

        Args:
            model_name: Model identifier used to select the TOK-02
                chars-per-token ratio (e.g. ``"llama3.2"``).  Defaults to an
                empty string which maps to the generic 4.0 ratio.

        Returns:
            Dict with keys ``working_memory``, ``system``, ``user``,
            ``attachments``, ``thinking``, ``assistant``, and ``tool``.
        """
        ratio = chars_per_token(model_name)
        breakdown = {
            "working_memory": 0,
            "system": 0,
            "user": 0,
            "attachments": 0,
            "thinking": 0,
            "assistant": 0,
            "tool": 0,
        }

        for msg in self.get_enabled_messages():
            content_tokens = estimate_text_tokens(msg.content, ratio)

            if msg.role == MessageRole.SYSTEM:
                if msg.metadata.get("is_working_memory", False):
                    breakdown["working_memory"] += content_tokens
                else:
                    breakdown["system"] += content_tokens
            elif msg.role == MessageRole.USER:
                breakdown["user"] += content_tokens
            elif msg.role == MessageRole.THINKING:
                breakdown["thinking"] += content_tokens
            elif msg.role in (MessageRole.ASSISTANT, MessageRole.SYNTHESIS):
                # SYNTHESIS is an agent-generated response variant; count it
                # in the assistant band so the meter reflects all LLM output.
                breakdown["assistant"] += content_tokens
            elif msg.role in (MessageRole.TOOL_CALL, MessageRole.TOOL_RESULT):
                breakdown["tool"] += content_tokens

            for attachment in msg.attachments:
                if attachment.enabled:
                    breakdown["attachments"] += estimate_text_tokens(attachment.content, ratio)

        return breakdown

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
            entry.message
            for entry in self.messages
            if entry.message.role == MessageRole.SYSTEM and entry.message.enabled
        ]
        other_msgs = [
            entry.message
            for entry in self.messages
            if entry.message.role != MessageRole.SYSTEM and entry.message.enabled
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

    # ---------------------------------------------------------------------------
    # Hierarchical task execution persistence (Phase 1)
    # ---------------------------------------------------------------------------

    @property
    def _session_root(self) -> Optional[str]:
        """Parent directory of the context dir (the session root)."""
        if self.path is None:
            return None
        return os.path.dirname(self.path)

    @property
    def _plans_dir(self) -> Optional[str]:
        """Path to the ``plans/`` sub-directory inside the session root."""
        root = self._session_root
        return os.path.join(root, "plans") if root else None

    @property
    def _task_nodes_dir(self) -> Optional[str]:
        """Path to the ``task_nodes/`` sub-directory inside the session root."""
        root = self._session_root
        return os.path.join(root, "task_nodes") if root else None

    @property
    def _scratch_dir(self) -> Optional[str]:
        """Path to the ``scratch/`` sub-directory inside the session root."""
        root = self._session_root
        return os.path.join(root, "scratch") if root else None

    def get_scratch_dir(self) -> str:
        """Return (and create) the scratch directory for this session."""
        scratch = self._scratch_dir
        if scratch is None:
            raise ValueError("Context has no path — cannot determine scratch dir")
        os.makedirs(scratch, exist_ok=True)
        return scratch

    def save_plan(self, plan: PlanRecord) -> str:
        """Persist *plan* to the session ``plans/`` directory.

        Returns the file path written.
        """
        plans_dir = self._plans_dir
        if plans_dir is None:
            raise ValueError("Context has no path — cannot persist PlanRecord")
        return plan.save(plans_dir)

    def load_plans(self) -> list[PlanRecord]:
        """Load all PlanRecord files from the session ``plans/`` directory.

        Returns an empty list if the directory does not yet exist.
        """
        plans_dir = self._plans_dir
        if plans_dir is None or not os.path.isdir(plans_dir):
            return []
        records: list[PlanRecord] = []
        for filename in sorted(os.listdir(plans_dir)):
            if filename.endswith(".json"):
                try:
                    records.append(PlanRecord.load(os.path.join(plans_dir, filename)))
                except Exception as exc:
                    logger.warning("Could not load plan %s: %s", filename, exc)
        return records

    def save_task_node(self, node: TaskNodeRecord) -> str:
        """Persist *node* to the session ``task_nodes/`` directory.

        Returns the file path written.
        """
        task_nodes_dir = self._task_nodes_dir
        if task_nodes_dir is None:
            raise ValueError("Context has no path — cannot persist TaskNodeRecord")
        return node.save(task_nodes_dir)

    def load_task_nodes(self) -> list[TaskNodeRecord]:
        """Load all TaskNodeRecord files from the session ``task_nodes/`` directory.

        Returns an empty list if the directory does not yet exist.
        """
        task_nodes_dir = self._task_nodes_dir
        if task_nodes_dir is None or not os.path.isdir(task_nodes_dir):
            return []
        records: list[TaskNodeRecord] = []
        for filename in sorted(os.listdir(task_nodes_dir)):
            if filename.endswith(".json"):
                try:
                    records.append(TaskNodeRecord.load(os.path.join(task_nodes_dir, filename)))
                except Exception as exc:
                    logger.warning("Could not load task node %s: %s", filename, exc)
        return records

    def save_task_tree(self, tree: TaskTree) -> str:
        """Persist *tree* to ``<session_root>/task_tree.json``.

        Returns the file path written.
        """
        root = self._session_root
        if root is None:
            raise ValueError("Context has no path — cannot persist TaskTree")
        return tree.save(root)

    def load_task_tree(self) -> Optional[TaskTree]:
        """Load the TaskTree from ``<session_root>/task_tree.json``.

        Returns ``None`` if the file does not yet exist.
        """
        root = self._session_root
        if root is None:
            return None
        file_path = os.path.join(root, "task_tree.json")
        if not os.path.isfile(file_path):
            return None
        try:
            return TaskTree.load(root)
        except Exception as exc:
            logger.warning("Could not load task_tree.json: %s", exc)
            return None

    # Backward compatibility aliases

    def load_messages(self) -> None:
        """Alias for loading messages from context directory."""
        self.load_from_dir(self.path)
