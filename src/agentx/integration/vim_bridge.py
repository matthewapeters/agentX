"""VimBridge — lightweight adapter for opening files in a running neovim instance.

This module communicates with neovim via its built-in ``--server`` / ``--remote``
CLI flags, which are available in every modern neovim distribution (≥ 0.5).  No
``pynvim`` or msgpack dependency is required.

Socket path resolution (highest priority first):

1. ``AGENTX_NVIM_SOCKET`` environment variable (mirrors ``launch_vibe.sh``).
2. ``config["neovim"]["socket"]`` from the runtime config dict.
3. ``/tmp/agentx_<session>.nvim.sock`` where ``<session>`` is
   ``AGENTX_TMUX_SESSION`` (default ``agentx``) — the same formula as
   ``launch_vibe.sh``.

Example usage::

    bridge = VimBridge(config=config)
    if bridge.is_connected():
        bridge.open_file("/path/to/file.py", line=42)

Affordances implemented here:

    PD-14-AF-002  — open file in running neovim instance from FileExplorer
"""

from __future__ import annotations

import logging
import os
import re
import shutil
import subprocess
from pathlib import Path

_logger = logging.getLogger(__name__)
_CONTROL_CHAR_PATTERN = re.compile(r"[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]")


def _resolve_default_socket() -> str:
    """Compute the default neovim socket path, mirroring ``launch_vibe.sh``.

    Resolution order:

    1. ``AGENTX_NVIM_SOCKET`` environment variable.
    2. ``/tmp/agentx_<session>.nvim.sock`` using ``AGENTX_TMUX_SESSION``
       (default ``agentx``) — the same formula as ``launch_vibe.sh``.

    Returns:
        str: Resolved socket path.
    """
    from_env = os.environ.get("AGENTX_NVIM_SOCKET", "")
    if from_env:
        return from_env
    session = os.environ.get("AGENTX_TMUX_SESSION", "agentx")
    return f"/tmp/agentx_{session}.nvim.sock"


class VimBridge:
    """Thin adapter for issuing open-file commands to a running neovim process.

    VimBridge delegates to the ``nvim`` CLI rather than using ``pynvim`` RPC so
    that no additional Python dependency is required.  The communication path is:

    ::

        VimBridge.open_file(path)
          └── subprocess: nvim --server <socket> --remote <path>
                (or --remote +<line> <path> when line number is provided)

    The neovim socket is a Unix-domain socket created by ``launch_vibe.sh`` at::

        nvim --listen /tmp/agentx_agentx.nvim.sock

    where the trailing ``agentx`` is the tmux session name (``AGENTX_TMUX_SESSION``).

    Args:
        socket_path: Override the resolved socket path.  When omitted the path is
            derived from ``AGENTX_NVIM_SOCKET`` / ``AGENTX_TMUX_SESSION`` env vars
            (matching ``launch_vibe.sh``) or ``config["neovim"]["socket"]``.
        config: Optional mapping containing a ``[neovim]`` section whose ``socket``
            key overrides the env-var resolution.
    """

    def __init__(
        self,
        socket_path: str | None = None,
        config: dict | None = None,
    ) -> None:
        """Initialise VimBridge.

        Args:
            socket_path: Explicit socket path override.  When ``None`` the path is
                resolved from environment variables and then from ``config``.
            config: Optional config dict; ``config["neovim"]["socket"]`` takes
                priority over the env-var default when present.
        """
        resolved = _resolve_default_socket()
        if config:
            resolved = config.get("neovim", {}).get("socket", resolved)
        if socket_path is not None:
            resolved = socket_path
        self._socket_path: str = resolved

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    @property
    def socket_path(self) -> str:
        """Return the configured neovim socket path.

        Returns:
            str: Filesystem path to the neovim socket.
        """

        return self._socket_path

    def is_connected(self) -> bool:
        """Return ``True`` when the neovim socket file exists and is a socket.

        The ``nvim --listen`` process creates a Unix-domain socket at startup.
        If the file exists and is a socket (``stat.S_ISSOCK``), neovim is likely
        running and reachable.  If neovim has crashed, the socket may linger on
        disk — ``open_file`` will detect the failure via subprocess return code.

        Returns:
            bool: ``True`` if the socket file is present and is a socket.
        """

        path = Path(self._socket_path)
        return path.exists() and path.is_socket()

    def open_file(self, file_path: str, line: int | None = None) -> bool:
        """Open ``file_path`` in the running neovim instance as a new buffer.

        Uses ``nvim --server <socket> --remote <file>`` (or ``--remote +<line>
        <file>`` when a line number is provided).  The existing neovim session
        is not closed; any open buffers remain intact.  [PD-14-AF-002]

        Args:
            file_path: Absolute or relative path to the file to open.
            line: Optional 1-based line number; when provided, neovim's cursor
                is placed at that line after opening the file.

        Returns:
            bool: ``True`` if the ``nvim`` subprocess exited with code 0,
            ``False`` when neovim is not connected or the command failed.
        """

        if not self.is_connected():
            _logger.warning(
                "VimBridge.open_file: neovim socket not present at %s — file not sent.",
                self._socket_path,
            )
            return False

        nvim_bin = shutil.which("nvim")
        if nvim_bin is None:
            _logger.error("VimBridge.open_file: 'nvim' binary not found in PATH.")
            return False

        cmd: list[str] = [nvim_bin, "--server", self._socket_path]

        if line is not None:
            # ``--remote`` with a ``+<cmd>`` prefix positions the cursor.
            cmd += ["--remote", f"+{line}", file_path]
        else:
            cmd += ["--remote", file_path]

        _logger.debug("VimBridge.open_file: %s", " ".join(cmd))

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                check=False,
            )
        except OSError as exc:
            _logger.error("VimBridge.open_file: subprocess error — %s", exc)
            return False

        if result.returncode != 0:
            _logger.warning(
                "VimBridge.open_file: nvim exited %d — %s",
                result.returncode,
                result.stderr.strip(),
            )
            return False

        return True

    def open_file_from_context(self, file_path: str, line: int | None = None) -> bool:
        """Convenience wrapper resolving ``file_path`` to an absolute path before opening.

        If ``file_path`` is relative, it is resolved against the current working
        directory.  Passes through to ``open_file`` after resolution.  [PD-14-AF-002]

        Args:
            file_path: Absolute or relative path to the file to open.
            line: Optional 1-based line number.

        Returns:
            bool: Result of ``open_file``.
        """

        resolved = str(Path(file_path).resolve())
        return self.open_file(resolved, line=line)

    @staticmethod
    def _escape_ex_path(file_path: str) -> str:
        """Escape a path for safe use in neovim Ex commands.

        Args:
            file_path: Path to escape.

        Returns:
            str: Escaped path string suitable for ``:edit``/``:diffsplit``.
        """

        escaped = file_path.replace("\\", "\\\\")
        escaped = escaped.replace(" ", "\\ ")
        escaped = escaped.replace("|", "\\|")
        return escaped

    def diff_files(self, left_file: str, right_file: str) -> bool:
        """Open a side-by-side vimdiff view for two files in running neovim.

        The command opens ``left_file`` in a new tab, runs ``:vert diffsplit``
        for ``right_file``, and enables diff mode for both windows.

        Args:
            left_file: Left-hand file in the diff view.
            right_file: Right-hand file in the diff view.

        Returns:
            bool: ``True`` when neovim accepted both commands, else ``False``.
        """

        if not self.is_connected():
            _logger.warning(
                "VimBridge.diff_files: neovim socket not present at %s — diff not sent.",
                self._socket_path,
            )
            return False

        nvim_bin = shutil.which("nvim")
        if nvim_bin is None:
            _logger.error("VimBridge.diff_files: 'nvim' binary not found in PATH.")
            return False

        escaped_left = self._escape_ex_path(left_file)
        escaped_right = self._escape_ex_path(right_file)

        open_left_cmd = [
            nvim_bin,
            "--server",
            self._socket_path,
            "--remote-tab-silent",
            escaped_left,
        ]
        diff_cmd = [
            nvim_bin,
            "--server",
            self._socket_path,
            "--remote-send",
            f"<C-\\><C-N>:vert diffsplit {escaped_right}<CR>:windo diffthis<CR>",
        ]

        _logger.debug("VimBridge.diff_files: %s", " ".join(open_left_cmd))
        _logger.debug("VimBridge.diff_files: %s", " ".join(diff_cmd))

        try:
            open_left_result = subprocess.run(open_left_cmd, capture_output=True, text=True, check=False)
            if open_left_result.returncode != 0:
                _logger.warning(
                    "VimBridge.diff_files: nvim exited %d opening left file — %s",
                    open_left_result.returncode,
                    open_left_result.stderr.strip(),
                )
                return False

            diff_result = subprocess.run(diff_cmd, capture_output=True, text=True, check=False)
            if diff_result.returncode != 0:
                _logger.warning(
                    "VimBridge.diff_files: nvim exited %d sending diffsplit — %s",
                    diff_result.returncode,
                    diff_result.stderr.strip(),
                )
                return False
        except OSError as exc:
            _logger.error("VimBridge.diff_files: subprocess error — %s", exc)
            return False

        return True

    def diff_files_from_context(self, left_file: str, right_file: str) -> bool:
        """Resolve relative file paths and open a diff in the running editor.

        Args:
            left_file: Left-hand file path, absolute or relative.
            right_file: Right-hand file path, absolute or relative.

        Returns:
            bool: Result of ``diff_files``.
        """

        resolved_left = str(Path(left_file).resolve())
        resolved_right = str(Path(right_file).resolve())
        return self.diff_files(resolved_left, resolved_right)

    @staticmethod
    def _sanitize_editor_text(text: str, max_length: int = 4000) -> str:
        """Sanitize freeform editor text to prevent unsafe key-injection payloads.

        Args:
            text: Raw user/tool-provided text.
            max_length: Maximum accepted payload length.

        Returns:
            str: Sanitized text payload.

        Raises:
            ValueError: If payload is too long or contains unsafe control chars.
        """

        if len(text) > max_length:
            raise ValueError(f"payload exceeds max length ({max_length})")
        if _CONTROL_CHAR_PATTERN.search(text):
            raise ValueError("payload contains unsupported control characters")
        return text

    def editor_action(
        self,
        action: str,
        file_path: str,
        line: int | None = None,
        payload: str = "",
    ) -> dict[str, object]:
        """Perform sandboxed editor-assist actions in the running vibe editor.

        Supported actions:
        - ``show_symbol_help``: trigger keyword help (`K`)
        - ``autocomplete_assist``: trigger omnifunc completion (`<C-x><C-o>`)
        - ``propose_edit``: replace current line with sanitized payload text

        Args:
            action: Action name from the supported set above.
            file_path: Target file path to open before action.
            line: Optional 1-based line number.
            payload: Optional text payload, used by ``propose_edit``.

        Returns:
            dict[str, object]: Action result payload with status and captured output.
        """

        supported_actions = {"show_symbol_help", "autocomplete_assist", "propose_edit"}
        if action not in supported_actions:
            return {
                "status": "error",
                "message": f"Unsupported editor action: {action}",
                "action": action,
            }

        if not self.open_file_from_context(file_path, line=line):
            return {
                "status": "error",
                "message": "Could not open file in vibe editor",
                "action": action,
                "file_path": file_path,
                "line": line,
            }

        nvim_bin = shutil.which("nvim")
        if nvim_bin is None:
            return {
                "status": "error",
                "message": "nvim binary not found in PATH",
                "action": action,
            }

        try:
            from agentx.integration.terminal_bridge import evaluate_terminal_policy

            canonical_command = f"nvim --server {self._socket_path} --remote-send <editor_action:{action}>"
            allowed, _, decision = evaluate_terminal_policy(
                canonical_command,
                context=f"Editor assist action '{action}' on {file_path}",
            )
            if not allowed:
                return {
                    "status": "error",
                    "message": "Editor action rejected by terminal safety policy",
                    "action": action,
                    "decision": decision,
                }
        except Exception as exc:
            return {
                "status": "error",
                "message": f"Terminal policy unavailable: {exc}",
                "action": action,
            }

        if action == "show_symbol_help":
            key_sequence = "K"
        elif action == "autocomplete_assist":
            key_sequence = "<C-x><C-o>"
        else:
            try:
                safe_payload = self._sanitize_editor_text(payload)
            except ValueError as exc:
                return {
                    "status": "error",
                    "message": str(exc),
                    "action": action,
                }
            escaped_payload = safe_payload.replace("\\", "\\\\").replace("'", "''")
            key_sequence = f"<C-\\><C-N>:call setline('.', '{escaped_payload}')<CR>"

        command = [
            nvim_bin,
            "--server",
            self._socket_path,
            "--remote-send",
            key_sequence,
        ]

        try:
            result = subprocess.run(command, capture_output=True, text=True, check=False)
        except OSError as exc:
            return {
                "status": "error",
                "message": f"Editor action failed: {exc}",
                "action": action,
            }

        if result.returncode != 0:
            return {
                "status": "error",
                "message": "Editor action command failed",
                "action": action,
                "stderr": result.stderr.strip(),
                "exit_code": result.returncode,
            }

        return {
            "status": "success",
            "message": "Editor action executed",
            "action": action,
            "stdout": result.stdout.strip(),
            "stderr": result.stderr.strip(),
            "exit_code": result.returncode,
        }
