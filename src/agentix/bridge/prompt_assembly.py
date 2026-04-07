# Prompt assembly for the Agentix bridge layer
# Extracted from agentix.context.sessions to co-locate with the bridge code
# that consumes it (classify_prompt, bridge).

import logging

from agentix.context.message import Message
from agentix.agentix_config import AgentixConfig
from agentix.file_utils import get_attachments
from agentix.query_payload import QueryPayload
from agentix.context.prompts import get_system_prompt, get_tools_prompt, get_user_prompt

logger = logging.getLogger(__name__)


def assemble_prompts(
    args: AgentixConfig,
    history: list[Message],
    max_tokens: int,
    response_max_tokens: int | None = None,
) -> QueryPayload:
    """Construct API request payload with messages and configuration."""

    # add system prompts if provided
    if args.system:
        system_content = get_system_prompt(args)
        history.append(Message(role="system", content=system_content))
        if args.debug:
            logger.debug("Added system message to history (length: %d chars)", len(system_content))
    if args.tools:
        history.append(Message(role="system", content=get_tools_prompt(args)))
    if args.user or args.file_path:
        # add user prompts if provided
        role = "user"
        content = None
        attachment = None
        if args.user:
            content = get_user_prompt(args)
        if args.file_path:
            attachment = get_attachments(args)
        history.append(Message(role=role, content=content, attachments=attachment or []))

    # Convert Message objects to dicts for trim_context
    history_dicts = [msg.to_dict() if hasattr(msg, "to_dict") else msg for msg in history]

    if args.debug:
        logger.debug("History before trim_context: %d messages", len(history_dicts))
        for i, msg in enumerate(history_dicts):
            role = msg.get("role", "unknown")
            content_len = len(msg.get("content", "")) if msg.get("content") else 0
            logger.debug("  Message %d: role=%s, content_length=%d", i, role, content_len)

    # Trim context based on max_tokens
    contextual_messages = trim_context(args, history_dicts, max_tokens)

    if args.debug:
        logger.debug("History after trim_context: %d messages", len(contextual_messages))
        for i, msg in enumerate(contextual_messages):
            role = msg.get("role", "unknown")
            content_len = len(msg.get("content", "")) if msg.get("content") else 0
            logger.debug("  Message %d: role=%s, content_length=%d", i, role, content_len)

    # Add format='json' for classification to enforce JSON output
    format_constraint = getattr(args, "response_format", None)

    return QueryPayload(
        model=args.model,
        messages=contextual_messages,
        temperature=args.temperature,
        max_tokens=response_max_tokens,
        format=format_constraint,
    )


def trim_context(args: AgentixConfig, messages: list[Message], max_tokens: int) -> list[Message]:
    """Handle message history with token-based trimming.

    System messages are ALWAYS preserved as they contain critical instructions.
    """
    # Separate system messages from the rest
    system_messages = [msg for msg in messages if msg.get("role") == "system"]
    non_system_messages = [msg for msg in messages if msg.get("role") != "system"]

    # Trim history based on token limits (max_tokens)
    total_tokens = 0
    trimmed_history = []

    # Iterate over non-system messages from the most recent to the oldest
    for message in reversed(non_system_messages):
        # message.get("content") might be None, so handle that
        content = message.get("content") or ""
        # Estimate tokens for the current message
        # Assuming 1 token per 4 characters as a rough approximation
        message_tokens = len(content) // 4
        if "attachments" in message:
            for attachment in message["attachments"]:
                if isinstance(attachment, dict):
                    message_tokens += len(attachment.get("content", "")) // 4
                else:
                    message_tokens += len(attachment) // 4

        # Check if adding this message exceeds the token limit
        if total_tokens + message_tokens > max_tokens:
            break  # Stop adding messages if the limit is exceeded

        # Add the message to the trimmed history and update the token count
        trimmed_history.append(message)
        total_tokens += message_tokens

    # Reverse the trimmed history to maintain chronological order
    trimmed_history.reverse()

    # Prepend system messages at the beginning (they go first in the message list)
    return system_messages + trimmed_history
