"""
Docstring for agentix.context.context
"""

import json
import os
from datetime import UTC, datetime
from glob import glob
import sys

from agentix.agentix_config import AgentixConfig
from agentix.constants import SESSIONS_DIR, SESSIONS_METADATA_FILE, PROMPT_CLASSIFICATION
from agentix.file_utils import get_attachments
from agentix.query_payload import QueryPayload

from .message import Message
from .prompts import get_system_prompt, get_tools_prompt, get_user_prompt


class Context:
    """
    A class representing the context for Agentix operations.
    """

    def __init__(self, user_id: str, session_id: str):
        """
        Initialize the Context with user and session identifiers.

        :param user_id: The identifier for the user.
        :param session_id: The identifier for the session.
        """
        self.user_id = user_id
        self.session_id = session_id
        self.history: list[Message] = []

    def get_user_id(self) -> str:
        """
        Get the user identifier.

        :return: The user identifier.
        """
        return self.user_id

    def get_session_id(self) -> str:
        """
        Get the session identifier.

        :return: The session identifier.
        """
        return self.session_id

    def assemble_prompts(self, args: AgentixConfig, max_tokens: int) -> QueryPayload:
        """Construct API request payload with messages and configuration."""

        # add system prompts if provided
        if args.system:
            self.history.append(Message(role="system", content=get_system_prompt(args)))
        if args.tools:
            self.history.append(
                Message(role="tool_calls", content=get_tools_prompt(args))
            )
        if args.user or args.file_path:
            # add user prompts if provided
            role = ("user",)
            content = None
            attachment = None
            if args.user:
                content = get_user_prompt(args)
            if args.file_path:
                attachment = get_attachments(args)
            self.history.append(
                Message(role=role, content=content, attachments=attachment)
            )

        # Trim context based on max_tokens
        contextual_messages = self.trim_context(args, self.history, max_tokens)

        return QueryPayload(
            model=args.model,
            messages=contextual_messages,
            temperature=args.temperature,
        )

    def trim_context(
        self, args: AgentixConfig, messages: list[Message], max_tokens: int
    ) -> list[Message]:
        """Handle message history with token-based trimming."""

        # save the untrimmed history first
        os.makedirs(f"{SESSIONS_DIR}{args.session}", exist_ok=True)
        # Checkpoint history
        ts = datetime.now(UTC).strftime("%Y%m%dT%H%M%SZ")
        with open(
            f"{SESSIONS_DIR}{args.session}/{ts}.json", "w", encoding="utf-8"
        ) as f:
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

    def manage_sessions(self, args: AgentixConfig) -> list[Message]:
        """Create, retrieve, and manage session state."""
        # if no specific session is requested, (session == agentix_session) then summarize
        # the prompt and add it to the sessions metadata file
        history = []
        print((f"Managing session: {args.session}"), file=sys.stderr)
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
                    "Debug: Calling summarize_user_prompt with args:",
                    args,
                    file=sys.stderr,
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
                            "Continuing session:",
                            sessions["sessions"][-1],
                            file=sys.stderr,
                        )
                    args.session = sessions["sessions"][-1]["session_id"]
                    # Continue with the same model if not specified
                    if not args.model:
                        args.model = sessions["sessions"][-1]["model"]
                    history = get_session_history(args)
        print(f"Debug: args.session = {args.session}", file=sys.stderr)
        print(f"Debug: args = {args}", file=sys.stderr)
        return history

    def update_session(self, args: AgentixConfig, history: list[Message], response: str):
        """Update session history with the latest interaction."""
        session_dir = f"{SESSIONS_DIR}{args.session}"
        os.makedirs(session_dir, exist_ok=True)

        # Save each message in the history that hasn't been saved yet
        for message in history:
            if not message.filename:  # Only save messages without a filename
                timestamp = datetime.now(UTC).strftime(
                    "%Y%m%d%H%M%S%f"
                )  # Microsecond precision
                filename = f"{timestamp}_{message.role}.json"
                filepath = os.path.join(session_dir, filename)

                with open(filepath, "w", encoding="utf-8") as f:
                    json.dump(
                        {
                            "role": message.role,
                            "content": message.content,
                            "attachments": message.attachments,
                        },
                        f,
                        indent=2,
                    )

                message.filename = filename  # Assign the filename to the message

    def get_session_history(self, args: AgentixConfig) -> list[Message]:
        """Retrieve session history JSON from timestamped files."""
        session_dir = f"{SESSIONS_DIR}{args.session}"
        os.makedirs(session_dir, exist_ok=True)

        # Load all message files and sort them by timestamp
        message_files = glob(os.path.join(session_dir, "*.json"))
        message_files.sort()

        history = []
        for filepath in message_files:
            with open(filepath, "r", encoding="utf-8") as f:
                data = json.load(f)
                message = Message(
                    role=data["role"],
                    content=data["content"],
                    attachments=data.get("attachments"),
                )
                message.filename = os.path.basename(
                    filepath
                )  # Assign the filename to the message
                history.append(message)

        return history
