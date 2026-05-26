# Session management for Agentix CLI

import logging
import os
from datetime import UTC, datetime

logger = logging.getLogger(__name__)

from agentix.context.message import Message
from shared.models.context import Context

from ..agentix_config import AgentixConfig

# Re-export from bridge layer — assemble_prompts and trim_context now live in
# agentix.bridge.prompt_assembly; keep these names here for backward compatibility.
from ..bridge.prompt_assembly import assemble_prompts, trim_context  # noqa: F401
from ..constants import (
    CLASSIFICATION_MAX_TOKENS,
    DEFAULT_SESSION_ID,
    PROMPT_CLASSIFICATION,
)


def _get_user_name() -> str:
    return os.getenv("USER") or os.getenv("USERNAME") or "User"


def _resolve_sessions_base() -> str:
    """Return base sessions directory for AgentX-compatible mode."""
    env_dir = os.getenv("AGENTX_SESSIONS_DIR")
    if env_dir:
        return env_dir

    cwd_sessions = os.path.join(os.getcwd(), "sessions")
    return cwd_sessions


def _ensure_session_context_dir(args: AgentixConfig) -> str:
    base_dir = _resolve_sessions_base()

    user_dir = os.path.join(base_dir, _get_user_name())
    os.makedirs(user_dir, exist_ok=True)

    if args.session == DEFAULT_SESSION_ID:
        session_id = f"session_{datetime.now(UTC).strftime('%Y-%m-%d_%H-%M-%S')}"
        args.session = session_id

    session_dir = os.path.join(user_dir, args.session)
    context_dir = os.path.join(session_dir, "context")
    os.makedirs(context_dir, exist_ok=True)
    return context_dir


def _get_latest_session_id(base_dir: str, user_name: str) -> str | None:
    user_dir = os.path.join(base_dir, user_name)
    if not os.path.isdir(user_dir):
        return None
    sessions = [
        d for d in os.listdir(user_dir) if os.path.isdir(os.path.join(user_dir, d)) and d.startswith("session_")
    ]
    if not sessions:
        return None
    sessions.sort(reverse=True)
    return sessions[0]


def assemble_classification_prompt(args: AgentixConfig, history: list[Message], max_tokens: int) -> dict:
    """Construct API request payload with messages and configuration for classification prompts."""

    # Use the classification prompt to ask the LLM to classify the user input
    # and determine next steps.  We do this for all user prompts.
    # We do not include system prompts or tool prompts in this classification step.
    classification_config = AgentixConfig()
    classification_config.model = args.classification_model or args.model
    classification_config.system = [PROMPT_CLASSIFICATION]
    classification_config.user = args.user
    classification_config.debug = args.debug
    # Reuse the active session ID so classification does not spawn a second
    # auto-generated session folder in AgentX mode.
    classification_config.session = args.session

    response_max_tokens = (
        args.classification_max_tokens if args.classification_max_tokens is not None else CLASSIFICATION_MAX_TOKENS
    )

    return assemble_prompts(
        classification_config,
        history,
        max_tokens,
        response_max_tokens=response_max_tokens,
    )


def manage_sessions(args: AgentixConfig) -> list[Message]:
    """Create, retrieve, and manage session state."""
    history = []
    logger.debug("Managing session: %s", args.session)

    base_dir = _resolve_sessions_base()

    if args.session == "__continue":
        latest = _get_latest_session_id(base_dir, _get_user_name())
        if latest:
            args.session = latest
            history = get_session_history(args)
    else:
        _ensure_session_context_dir(args)

    logger.debug("args.session = %s", args.session)
    logger.debug("args = %s", args)
    return history


def update_session(args: AgentixConfig, history: list[Message], response: str):
    """Update session history with the latest interaction."""
    session_dir = _ensure_session_context_dir(args)

    # Save each message in the history that hasn't been saved yet
    for message in history:
        if getattr(message, "file_path", None):
            continue
        message.save(session_dir)


def get_session_history(args: AgentixConfig) -> list[Message]:
    """Retrieve session history JSON from timestamped files."""
    session_dir = _ensure_session_context_dir(args)
    context = Context(path=session_dir, session_id=args.session)
    context.load_from_dir(session_dir)
    return [entry.message for entry in context.messages]
