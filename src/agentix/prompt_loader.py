"""
agentix.prompt_loader
~~~~~~~~~~~~~~~~~~~~~
Centralised loader for system prompt files.

Prompts are stored as plain text files (any extension) inside a single
directory.  A prompt is addressed by its *stem* — the filename without
extension (e.g. ``"planner_prompt"`` resolves to ``planner_prompt.md``
inside the configured directory).

This module is the single place where prompt-file I/O happens.  All other
modules that need to read a system prompt should import :class:`PromptLoader`
instead of calling ``open()`` or ``glob.glob()`` directly.
"""

import glob as _glob
import logging
import os
from typing import Optional

logger = logging.getLogger(__name__)


class PromptLoader:
    """
    Load and list system prompt files from a configurable directory.

    Args:
        prompts_dir: Absolute (or ``~``-prefixed) path to the directory that
            contains prompt files.  When ``None`` the value of
            :data:`agentix.constants.SYSTEM_PROMPTS_DIR` is used as the
            default, preserving the existing env-var-driven behaviour.
    """

    def __init__(self, prompts_dir: Optional[str] = None) -> None:
        if prompts_dir is None:
            from agentix.constants import SYSTEM_PROMPTS_DIR

            prompts_dir = SYSTEM_PROMPTS_DIR
        self._dir = os.path.expanduser(prompts_dir)

    # ------------------------------------------------------------------
    # Core I/O
    # ------------------------------------------------------------------

    def load(self, name: str) -> Optional[str]:
        """Load a prompt by stem name (no extension).

        Globs for ``<dir>/<name>.*`` and reads the first match.

        Returns:
            File contents as a string, or ``None`` if not found or unreadable.
        """
        matches = _glob.glob(os.path.join(self._dir, f"{name}.*"))
        if not matches:
            logger.debug("Prompt %r not found in %s", name, self._dir)
            return None
        try:
            with open(matches[0], "r", encoding="utf-8") as fh:
                return fh.read()
        except OSError as exc:
            logger.warning("Could not read prompt %r from %s: %s", name, matches[0], exc)
            return None

    # ------------------------------------------------------------------
    # Discovery helpers
    # ------------------------------------------------------------------

    def list_available(self) -> dict[str, str]:
        """Return ``{stem: absolute_path}`` for every file in the prompt directory."""
        result: dict[str, str] = {}
        for path in _glob.glob(os.path.join(self._dir, "*.*")):
            stem = os.path.splitext(os.path.basename(path))[0]
            result[stem] = path
        return result

    def preview(self, n_lines: int = 2) -> dict[str, list[str]]:
        """Return the first *n_lines* non-blank lines of each prompt.

        Useful for listing available prompts in a UI without loading full content.
        """
        result: dict[str, list[str]] = {}
        for stem, path in self.list_available().items():
            try:
                with open(path, "r", encoding="utf-8") as fh:
                    lines = [ln for ln in fh.readlines() if ln.strip()]
                result[stem] = lines[:n_lines]
            except OSError as exc:
                logger.warning("Could not preview prompt %r: %s", stem, exc)
        return result

    # ------------------------------------------------------------------
    # Formatting helper
    # ------------------------------------------------------------------

    def get_formatted_system_prompt(self, names: list[str], debug: bool = False) -> str:
        """Load and concatenate *names*, wrapped in ``[SYSTEM] … [END SYSTEM]`` tags.

        Prompts that are not found are silently skipped (a debug log is emitted).

        Args:
            names: Ordered list of prompt stems to include.
            debug: When ``True`` emit a debug log for each loaded prompt.

        Returns:
            A string of the form ``"[SYSTEM]\\n<content>\\n[END SYSTEM]\\n\\n"``.
        """
        parts: list[str] = []
        for name in names:
            text = self.load(name)
            if text is not None:
                if debug:
                    logger.debug("Loaded system prompt: %s", name)
                parts.append(text)
        combined = "".join(parts)
        return f"[SYSTEM]\n{combined}\n[END SYSTEM]\n\n"
