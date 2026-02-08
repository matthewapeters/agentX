# Session management for Agentix CLI

import glob
import json
import os
import sys
from datetime import UTC, datetime

from agentix.context.message import Message

from ..agentix_config import AgentixConfig
from ..api_client import summarize_user_prompt
from ..constants import (
    DEFAULT_SESSION_ID,
    PROMPT_CLASSIFICATION,
    SESSIONS_DIR,
    SESSIONS_METADATA_FILE,
)
from ..file_utils import get_attachments
from ..query_payload import QueryPayload
from .prompts import get_system_prompt, get_tools_prompt, get_user_prompt
from shared.models.context import Context


def _get_user_name() -> str:
    return os.getenv("USER") or os.getenv("USERNAME") or "User"


def _resolve_sessions_base() -> tuple[str, bool]:
    """Return (base_dir, is_agentx_mode)."""
    env_dir = os.getenv("AGENTX_SESSIONS_DIR")
    if env_dir:
        return env_dir, True

    cwd_sessions = os.path.join(os.getcwd(), "sessions")
    if os.path.isdir(cwd_sessions):
        return cwd_sessions, True

    return SESSIONS_DIR, False


def _ensure_session_context_dir(args: AgentixConfig) -> tuple[str, bool]:
    base_dir, agentx_mode = _resolve_sessions_base()

    if not agentx_mode:
        session_dir = f"{SESSIONS_DIR}{args.session}"
        os.makedirs(session_dir, exist_ok=True)
        return session_dir, agentx_mode

    user_dir = os.path.join(base_dir, _get_user_name())
    os.makedirs(user_dir, exist_ok=True)

    if args.session == DEFAULT_SESSION_ID:
        session_id = f"session_{datetime.now(UTC).strftime('%Y-%m-%d_%H-%M-%S')}"
        args.session = session_id

    session_dir = os.path.join(user_dir, args.session)
    context_dir = os.path.join(session_dir, "context")
    os.makedirs(context_dir, exist_ok=True)
    return context_dir, agentx_mode


def _get_latest_session_id(base_dir: str, user_name: str) -> str | None:
    user_dir = os.path.join(base_dir, user_name)
    if not os.path.isdir(user_dir):
        return None
    sessions = [
        d for d in os.listdir(user_dir)
        if os.path.isdir(os.path.join(user_dir, d)) and d.startswith("session_")
    ]
    if not sessions:
        return None
    sessions.sort(reverse=True)
    return sessions[0]


def assemble_classification_prompt(
    args: AgentixConfig, history: list[Message], max_tokens: int
) -> dict:
    """Construct API request payload with messages and configuration for classification prompts."""

    # Use the classification prompt to ask the LLM to classify the user input
    # and determine next steps.  We do this for all user prompts.
    # We do not include system prompts or tool prompts in this classification step.
    classification_config = AgentixConfig()
    classification_config.model = args.model
    classification_config.system = [PROMPT_CLASSIFICATION]
    classification_config.user = args.user
    classification_config.debug = args.debug

    return assemble_prompts(classification_config, history, max_tokens)


def assemble_prompts(
    args: AgentixConfig, history: list[Message], max_tokens: int
) -> QueryPayload:
    """Construct API request payload with messages and configuration."""

    # add system prompts if provided
    if args.system:
        history.append(Message(role="system", content=get_system_prompt(args)))
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
        history.append(Message(role=role, content=content, attachments=attachment))

    # Convert Message objects to dicts for trim_context
    history_dicts = [msg.to_dict() if hasattr(msg, "to_dict") else msg for msg in history]

    # Trim context based on max_tokens
    contextual_messages = trim_context(args, history_dicts, max_tokens)

    return QueryPayload(
        model=args.model,
        messages=contextual_messages,
        temperature=args.temperature,
    )


def trim_context(
    args: AgentixConfig, messages: list[Message], max_tokens: int
) -> list[Message]:
    """Handle message history with token-based trimming."""

    session_dir, agentx_mode = _ensure_session_context_dir(args)

    if not agentx_mode:
        # Checkpoint history
        ts = datetime.now(UTC).strftime("%Y%m%dT%H%M%SZ")
        with open(os.path.join(session_dir, f"{ts}.json"), "w", encoding="utf-8") as f:
            json.dump(messages, f, indent=2)

    # Trim history based on token limits (max_tokens)
    total_tokens = 0
    trimmed_history = []

    # Iterate over messages from the most recent to the oldest
    for message in reversed(messages):
        # Estimate tokens for the current message
        # Assuming 1 token per 4 characters as a rough approximation
        message_tokens = len(message["content"]) // 4
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

    return trimmed_history


def manage_sessions(args: AgentixConfig) -> list[Message]:
    """Create, retrieve, and manage session state."""
    # if no specific session is requested, (session == agentix_session) then summarize
    # the prompt and add it to the sessions metadata file
    history = []
    print((f"Managing session: {args.session}"), file=sys.stderr)

    base_dir, agentx_mode = _resolve_sessions_base()

    if agentx_mode:
        if args.session == "__continue":
            latest = _get_latest_session_id(base_dir, _get_user_name())
            if latest:
                args.session = latest
                history = get_session_history(args)
        else:
            _ensure_session_context_dir(args)
    else:
        match args.session:
            case "agentix_session":
                print("Creating new session...", file=sys.stderr)
                # summarize_user_prompt is called from api_client.py to avoid circular imports

                try:
                    with open(SESSIONS_METADATA_FILE, "r", encoding="utf-8") as f:
                        sessions = json.load(f)
                except FileNotFoundError:
                    sessions = {"sessions": []}

                print(
                    "Debug: Calling summarize_user_prompt with args:", args, file=sys.stderr
                )
                summarize_user_prompt(args)

                sessions["sessions"].append(
                    {
                        "session_id": args.session,
                        "model": args.model,
                        "created_at": datetime.now(UTC).isoformat(),
                    }
                )

                with open(SESSIONS_METADATA_FILE, "w", encoding="utf-8") as f:
                    json.dump(sessions, f, indent=2)
                    print(f"Session {args.session} created.", file=sys.stderr)
            case "__continue":
                # continue the session
                print("Continuing previous session...", file=sys.stderr)
                try:
                    with open(SESSIONS_METADATA_FILE, "r", encoding="utf-8") as f:
                        sessions = json.load(f)
                except FileNotFoundError:
                    print("No previous sessions found.", file=sys.stderr)
                    sessions = {"sessions": []}
                # get the last session
                if sessions["sessions"]:
                    if args.debug:
                        print(
                            "Continuing session:", sessions["sessions"][-1], file=sys.stderr
                        )
                    args.session = sessions["sessions"][-1]["session_id"]
                    # Continue with the same model if not specified
                    if not args.model:
                        args.model = sessions["sessions"][-1]["model"]
                    history = get_session_history(args)
    print(f"Debug: args.session = {args.session}", file=sys.stderr)
    print(f"Debug: args = {args}", file=sys.stderr)
    return history


def update_session(args: AgentixConfig, history: list[Message], response: str):
    """Update session history with the latest interaction."""
    session_dir, agentx_mode = _ensure_session_context_dir(args)

    # Save each message in the history that hasn't been saved yet
    for message in history:
        if getattr(message, "file_path", None):
            continue
        if agentx_mode:
            message.save(session_dir)
        else:
            timestamp = datetime.now(UTC).strftime(
                "%Y%m%d%H%M%S%f"
            )  # Microsecond precision
            filename = f"{timestamp}_{message.role}.json"
            filepath = os.path.join(session_dir, filename)

            with open(filepath, "w", encoding="utf-8") as f:
                json.dump(message.to_dict(), f, indent=2)

            message.file_path = filepath


def get_session_history(args: AgentixConfig) -> list[Message]:
    """Retrieve session history JSON from timestamped files."""
    session_dir, agentx_mode = _ensure_session_context_dir(args)

    if agentx_mode:
        context = Context(path=session_dir, session_id=args.session)
        context.load_from_dir(session_dir)
        return [entry.message for entry in context.messages]

    # Load all message files and sort them by timestamp
    message_files = glob.glob(os.path.join(session_dir, "*.json"))
    message_files.sort()

    history = []
    for filepath in message_files:
        with open(filepath, "r", encoding="utf-8") as f:
            data = json.load(f)
            if data.get("role") == "tool_calls":
                data["role"] = "system"
            message = Message.from_dict(data, file_path=filepath)
            history.append(message)

    return history
