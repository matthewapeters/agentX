"""
SessionState — mutable session data model.

Owns all non-GUI, non-IO session data: active model, history reference, current
message, pending prompt, and the startup metadata (session id, folder paths, user).
Also provides the three pure file-reading utilities that load per-project config
files from disk.
"""

import logging
import os
import subprocess
from datetime import UTC, datetime
from typing import Optional

from .history import History

logger = logging.getLogger(__name__)


class SessionState:
    """
    Mutable session data owned by AgentXSession.

    Separates the data-model concerns from the streaming/GUI coordination that
    `AgentXSession` handles.  All attributes here are safe to read from any
    thread (they are either immutable after construction or only written from
    the main thread).
    """

    def __init__(
        self,
        config: dict,
        session_id: str,
        session_folder: str,
        context_folder: str,
        session_log_path: str,
        user: str,
        user_history_folder: str,
    ) -> None:
        from shared.models.message import Message

        self.session_id = session_id
        self.session_folder = session_folder
        self.context_folder = context_folder
        self.user = user
        self.user_history_folder = user_history_folder
        self._session_log_path = session_log_path
        self.start_time = datetime.now(UTC).strftime("%Y-%m-%d %H:%M:%S UTC")
        self._config = config

        # Runtime-mutable state
        self._active_model: str = config["agentx"]["ollama_model"]
        self._history: Optional[History] = None
        self.message = Message(role="user", content="")
        self.enabled_history_attachments: list = []
        self._pending_prompt: Optional[str] = None

    # ------------------------------------------------------------------
    # Active model property
    # ------------------------------------------------------------------

    @property
    def active_model(self) -> str:
        """Currently active Ollama model — single source of truth."""
        return self._active_model

    @active_model.setter
    def active_model(self, model: str) -> None:
        self._active_model = model
        self._config["agentx"]["ollama_model"] = model

    # ------------------------------------------------------------------
    # History property (lazily loaded)
    # ------------------------------------------------------------------

    @property
    def history(self) -> "History":
        if self._history is None:
            self._history = History(
                user_history_path=self.user_history_folder,
                exclude_session=self.context_folder,
            )
        return self._history

    @history.setter
    def history(self, value: "History") -> None:
        self._history = value

    # ------------------------------------------------------------------
    # Pure file-reading utilities (no I/O state side-effects)
    # ------------------------------------------------------------------

    @staticmethod
    def detect_git_project_name(cwd: str) -> Optional[str]:
        """Return repo name if *cwd* is inside a git worktree, otherwise None."""
        try:
            result = subprocess.run(
                ["git", "-C", cwd, "rev-parse", "--show-toplevel"],
                capture_output=True,
                text=True,
                check=False,
                timeout=2,
            )
        except (OSError, subprocess.SubprocessError):
            return None

        if result.returncode != 0:
            return None

        repo_root = (result.stdout or "").strip()
        if not repo_root:
            return None
        return os.path.basename(repo_root).lower()

    @staticmethod
    def load_agentx_instructions(cwd: str) -> Optional[str]:
        """Load ``.agentx/agentx-instructions.md`` contents when present."""
        instructions_path = os.path.join(cwd, ".agentx", "agentx-instructions.md")
        if not os.path.isfile(instructions_path):
            return None
        try:
            with open(instructions_path, "r", encoding="utf-8") as f:
                return f.read()
        except OSError:
            return None

    @staticmethod
    def load_bootstrap_prompt(cwd: str) -> Optional[str]:
        """Load ``.agentx/bootstrap-prompt.md`` contents when present."""
        prompt_path = os.path.join(cwd, ".agentx", "bootstrap-prompt.md")
        if not os.path.isfile(prompt_path):
            return None
        try:
            with open(prompt_path, "r", encoding="utf-8") as f:
                content = f.read().strip()
        except OSError:
            return None
        return content or None
