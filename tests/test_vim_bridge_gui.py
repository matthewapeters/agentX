"""Tests for VimBridge and its integration with the FileExplorer Edit action.

Units under test:
  - ``src/agentx/integration/vim_bridge.VimBridge``
  - ``src/agentx/session.AgentXSession._open_file_in_editor``

Affordance IDs: PD-14-AF-002
"""

from __future__ import annotations

import subprocess
from pathlib import Path
from typing import Any
from unittest.mock import MagicMock, call, patch

import pytest

from agentx.integration.vim_bridge import VimBridge, _resolve_default_socket

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_bridge(socket_path: str = "/tmp/agentx.nvim.sock") -> VimBridge:
    """Return a VimBridge pointed at *socket_path*."""
    return VimBridge(socket_path=socket_path)


def _make_bridge_from_config(cfg: dict) -> VimBridge:
    """Return a VimBridge constructed from a config dict."""
    return VimBridge(config=cfg)


# ---------------------------------------------------------------------------
# PD-14-AF-002: is_connected()
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestVimBridgeIsConnected:
    """Unit tests for ``VimBridge.is_connected()``.

    GIVEN a VimBridge pointing at a socket path
    WHEN ``is_connected()`` is called
    THEN it returns True iff the path exists and is a socket
    """

    def test_is_connected_true_when_socket_exists(self, tmp_path: Path) -> None:
        """GIVEN a real Unix socket at the configured path
        WHEN is_connected() is called
        THEN the method returns True.
        """
        import socket

        sock_path = tmp_path / "nvim.sock"
        srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        srv.bind(str(sock_path))
        srv.listen(1)
        try:
            bridge = _make_bridge(str(sock_path))
            assert bridge.is_connected() is True
        finally:
            srv.close()

    def test_is_connected_false_when_socket_missing(self, tmp_path: Path) -> None:
        """GIVEN no socket file at the configured path
        WHEN is_connected() is called
        THEN the method returns False.
        """
        bridge = _make_bridge(str(tmp_path / "no_such.sock"))
        assert bridge.is_connected() is False

    def test_is_connected_false_when_path_is_regular_file(self, tmp_path: Path) -> None:
        """GIVEN a regular file (not a socket) at the configured path
        WHEN is_connected() is called
        THEN the method returns False (path is not a socket).
        """
        regular = tmp_path / "regular.txt"
        regular.write_text("not a socket")
        bridge = _make_bridge(str(regular))
        assert bridge.is_connected() is False


# ---------------------------------------------------------------------------
# PD-14-AF-002: socket_path property and config override
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestVimBridgeConfig:
    """Unit tests for VimBridge construction and config override.

    GIVEN various constructor arguments
    WHEN VimBridge is instantiated
    THEN socket_path reflects the expected value
    """

    def test_default_socket_path_uses_env_var_formula(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """GIVEN no AGENTX_NVIM_SOCKET env var and AGENTX_TMUX_SESSION=agentx (default)
        WHEN VimBridge is constructed with no arguments
        THEN socket_path is /tmp/agentx_agentx.nvim.sock (matching launch_vibe.sh).
        """
        monkeypatch.delenv("AGENTX_NVIM_SOCKET", raising=False)
        monkeypatch.delenv("AGENTX_TMUX_SESSION", raising=False)
        bridge = VimBridge()
        assert bridge.socket_path == "/tmp/agentx_agentx.nvim.sock"

    def test_agentx_nvim_socket_env_overrides_default(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """GIVEN AGENTX_NVIM_SOCKET is set in environment
        WHEN VimBridge is constructed with no arguments
        THEN socket_path equals the env var value.
        """
        monkeypatch.setenv("AGENTX_NVIM_SOCKET", "/env/override.sock")
        bridge = VimBridge()
        assert bridge.socket_path == "/env/override.sock"

    def test_agentx_tmux_session_env_scopes_socket_path(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """GIVEN AGENTX_TMUX_SESSION=myproject and no AGENTX_NVIM_SOCKET
        WHEN VimBridge is constructed with no arguments
        THEN socket_path is /tmp/agentx_myproject.nvim.sock.
        """
        monkeypatch.delenv("AGENTX_NVIM_SOCKET", raising=False)
        monkeypatch.setenv("AGENTX_TMUX_SESSION", "myproject")
        bridge = VimBridge()
        assert bridge.socket_path == "/tmp/agentx_myproject.nvim.sock"

    def test_explicit_socket_path_overrides_default(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """GIVEN an explicit socket_path argument
        WHEN VimBridge is constructed
        THEN socket_path equals the supplied value (env vars ignored).
        """
        monkeypatch.setenv("AGENTX_NVIM_SOCKET", "/env/override.sock")
        bridge = _make_bridge("/custom/nvim.sock")
        assert bridge.socket_path == "/custom/nvim.sock"

    def test_explicit_socket_path_overrides_config(self) -> None:
        """GIVEN both socket_path argument and config[neovim][socket]
        WHEN VimBridge is constructed
        THEN explicit socket_path wins.
        """
        bridge = VimBridge(socket_path="/explicit.sock", config={"neovim": {"socket": "/cfg.sock"}})
        assert bridge.socket_path == "/explicit.sock"

    def test_config_dict_overrides_env_default(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """GIVEN a config dict with neovim.socket key
        WHEN VimBridge is constructed with config=...
        THEN socket_path is taken from the config dict (overrides env-var default).
        """
        monkeypatch.delenv("AGENTX_NVIM_SOCKET", raising=False)
        cfg: dict[str, Any] = {"neovim": {"socket": "/cfg/nvim.sock"}}
        bridge = _make_bridge_from_config(cfg)
        assert bridge.socket_path == "/cfg/nvim.sock"

    def test_config_without_neovim_key_falls_through_to_env_formula(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """GIVEN a config dict with no neovim section and AGENTX_TMUX_SESSION=agentx
        WHEN VimBridge is constructed with config=...
        THEN socket_path falls back to /tmp/agentx_agentx.nvim.sock.
        """
        monkeypatch.delenv("AGENTX_NVIM_SOCKET", raising=False)
        monkeypatch.delenv("AGENTX_TMUX_SESSION", raising=False)
        bridge = _make_bridge_from_config({"agentx": {}})
        assert bridge.socket_path == "/tmp/agentx_agentx.nvim.sock"


# ---------------------------------------------------------------------------
# PD-14-AF-002: open_file()
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestVimBridgeOpenFile:
    """Unit tests for ``VimBridge.open_file()``.

    GIVEN a VimBridge
    WHEN open_file() is called
    THEN it dispatches (or declines) the correct nvim subprocess command
    """

    def test_open_file_dispatches_nvim_remote_when_connected(self, tmp_path: Path) -> None:
        """GIVEN VimBridge is connected (socket exists)
        AND nvim is on PATH
        WHEN open_file(path) is called without a line number
        THEN subprocess.run is called with nvim --server SOCKET --remote PATH
        AND the method returns True.
        """
        import socket as _socket

        sock_path = tmp_path / "nvim.sock"
        srv = _socket.socket(_socket.AF_UNIX, _socket.SOCK_STREAM)
        srv.bind(str(sock_path))
        srv.listen(1)
        target_file = str(tmp_path / "target.py")

        try:
            bridge = _make_bridge(str(sock_path))
            mock_result = MagicMock()
            mock_result.returncode = 0
            mock_result.stderr = ""

            with (
                patch("shutil.which", return_value="/usr/bin/nvim"),
                patch("subprocess.run", return_value=mock_result) as mock_run,
            ):
                result = bridge.open_file(target_file)

            assert result is True
            mock_run.assert_called_once_with(
                ["/usr/bin/nvim", "--server", str(sock_path), "--remote", target_file],
                capture_output=True,
                text=True,
                check=False,
            )
        finally:
            srv.close()

    def test_open_file_with_line_number_adds_plus_prefix(self, tmp_path: Path) -> None:
        """GIVEN VimBridge is connected
        AND a line number is provided
        WHEN open_file(path, line=42) is called
        THEN subprocess.run is called with --remote +42 PATH.
        """
        import socket as _socket

        sock_path = tmp_path / "nvim.sock"
        srv = _socket.socket(_socket.AF_UNIX, _socket.SOCK_STREAM)
        srv.bind(str(sock_path))
        srv.listen(1)
        target_file = str(tmp_path / "target.py")

        try:
            bridge = _make_bridge(str(sock_path))
            mock_result = MagicMock()
            mock_result.returncode = 0
            mock_result.stderr = ""

            with (
                patch("shutil.which", return_value="/usr/bin/nvim"),
                patch("subprocess.run", return_value=mock_result) as mock_run,
            ):
                result = bridge.open_file(target_file, line=42)

            assert result is True
            mock_run.assert_called_once_with(
                ["/usr/bin/nvim", "--server", str(sock_path), "--remote", "+42", target_file],
                capture_output=True,
                text=True,
                check=False,
            )
        finally:
            srv.close()

    def test_open_file_returns_false_when_not_connected(self, tmp_path: Path) -> None:
        """GIVEN VimBridge is not connected (socket path does not exist)
        WHEN open_file(path) is called
        THEN subprocess.run is NOT called
        AND the method returns False.
        """
        bridge = _make_bridge(str(tmp_path / "no_socket.sock"))
        with patch("subprocess.run") as mock_run:
            result = bridge.open_file("/some/file.py")
        assert result is False
        mock_run.assert_not_called()

    def test_open_file_returns_false_when_nvim_not_on_path(self, tmp_path: Path) -> None:
        """GIVEN VimBridge is connected but nvim is not on PATH
        WHEN open_file(path) is called
        THEN subprocess.run is NOT called
        AND the method returns False.
        """
        import socket as _socket

        sock_path = tmp_path / "nvim.sock"
        srv = _socket.socket(_socket.AF_UNIX, _socket.SOCK_STREAM)
        srv.bind(str(sock_path))
        srv.listen(1)

        try:
            bridge = _make_bridge(str(sock_path))
            with patch("shutil.which", return_value=None), patch("subprocess.run") as mock_run:
                result = bridge.open_file("/some/file.py")
            assert result is False
            mock_run.assert_not_called()
        finally:
            srv.close()

    def test_open_file_returns_false_when_nvim_exits_nonzero(self, tmp_path: Path) -> None:
        """GIVEN VimBridge is connected
        AND nvim exits with non-zero return code
        WHEN open_file(path) is called
        THEN the method returns False.
        """
        import socket as _socket

        sock_path = tmp_path / "nvim.sock"
        srv = _socket.socket(_socket.AF_UNIX, _socket.SOCK_STREAM)
        srv.bind(str(sock_path))
        srv.listen(1)

        try:
            bridge = _make_bridge(str(sock_path))
            mock_result = MagicMock()
            mock_result.returncode = 1
            mock_result.stderr = "ECONNREFUSED"

            with patch("shutil.which", return_value="/usr/bin/nvim"), patch("subprocess.run", return_value=mock_result):
                result = bridge.open_file("/some/file.py")

            assert result is False
        finally:
            srv.close()

    def test_open_file_returns_false_on_oserror(self, tmp_path: Path) -> None:
        """GIVEN VimBridge is connected
        AND subprocess.run raises OSError (e.g. nvim binary missing at runtime)
        WHEN open_file(path) is called
        THEN the method returns False without propagating the exception.
        """
        import socket as _socket

        sock_path = tmp_path / "nvim.sock"
        srv = _socket.socket(_socket.AF_UNIX, _socket.SOCK_STREAM)
        srv.bind(str(sock_path))
        srv.listen(1)

        try:
            bridge = _make_bridge(str(sock_path))
            with (
                patch("shutil.which", return_value="/usr/bin/nvim"),
                patch("subprocess.run", side_effect=OSError("Permission denied")),
            ):
                result = bridge.open_file("/some/file.py")

            assert result is False
        finally:
            srv.close()


# ---------------------------------------------------------------------------
# PD-14-AF-002: open_file_from_context() — path resolution
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestVimBridgeOpenFileFromContext:
    """Unit tests for ``VimBridge.open_file_from_context()``.

    GIVEN a VimBridge
    WHEN open_file_from_context(relative_path) is called
    THEN the path is resolved to an absolute path before dispatching
    """

    def test_relative_path_is_resolved_to_absolute(self, tmp_path: Path) -> None:
        """GIVEN a relative file path
        WHEN open_file_from_context is called
        THEN open_file receives the resolved absolute path.
        """
        bridge = _make_bridge("/tmp/agentx.nvim.sock")
        with patch.object(bridge, "open_file", return_value=True) as mock_open:
            bridge.open_file_from_context("relative/file.py")
        expected = str(Path("relative/file.py").resolve())
        mock_open.assert_called_once_with(expected, line=None)

    def test_absolute_path_is_passed_through_unchanged(self, tmp_path: Path) -> None:
        """GIVEN an absolute file path
        WHEN open_file_from_context is called
        THEN open_file receives the same absolute path.
        """
        bridge = _make_bridge("/tmp/agentx.nvim.sock")
        abs_path = "/absolute/path/to/file.py"
        with patch.object(bridge, "open_file", return_value=True) as mock_open:
            bridge.open_file_from_context(abs_path)
        mock_open.assert_called_once_with(abs_path, line=None)

    def test_line_number_forwarded(self) -> None:
        """GIVEN a file path and a line number
        WHEN open_file_from_context is called
        THEN open_file receives the resolved path and the line number.
        """
        bridge = _make_bridge("/tmp/agentx.nvim.sock")
        abs_path = "/absolute/path/to/file.py"
        with patch.object(bridge, "open_file", return_value=True) as mock_open:
            bridge.open_file_from_context(abs_path, line=10)
        mock_open.assert_called_once_with(abs_path, line=10)


# ---------------------------------------------------------------------------
# PD-14-AF-002: Session._open_file_in_editor integration test
# ---------------------------------------------------------------------------


@pytest.mark.integration
class TestSessionOpenFileInEditor:
    """Integration tests for ``AgentXSession._open_file_in_editor()``.

    Units under test: ``AgentXSession`` + ``VimBridge``

    GIVEN a session with a wired VimBridge
    WHEN _open_file_in_editor() is called
    THEN it delegates to VimBridge.open_file_from_context with the correct path
    """

    def _make_session(self, tmp_path: Path):
        """Return a minimal AgentXSession with Tkinter and external I/O mocked out."""
        import tkinter as tk

        from agentx.session import AgentXSession

        config: dict[str, Any] = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "test-model",
                "working_memory": {"enabled": False},
            },
            "agentix": {
                "system_prompts_dir": "system_prompts",
            },
        }

        root = tk.Tk()
        root.withdraw()

        patches = [
            patch("agentx.session.ServiceManager"),
            patch("agentx.session.GUIManager"),
            patch("agentx.session.create_adapter"),
            patch("agentx.session.ClientToolExecutor"),
            patch("agentx.session.ServerToolExecutor"),
            patch("agentx.session.ToolDispatcher"),
            patch("agentx.session.StreamingController"),
            patch("agentx.session.OllamaServiceProvider"),
            patch("agentx.session.ModelMetadataStore"),
            patch("threading.Thread"),
        ]
        for p in patches:
            p.start()

        session = AgentXSession(
            root=root,
            config=config,
            username="testuser",
            session_dir=str(tmp_path),
        )

        return session, root, patches

    def test_open_file_in_editor_calls_vim_bridge(self, tmp_path: Path) -> None:
        """GIVEN a session with a mocked VimBridge
        WHEN _open_file_in_editor('/path/to/file.py') is called
        THEN vim_bridge.open_file_from_context is called with that path.
        """
        session, root, patches = self._make_session(tmp_path)
        try:
            mock_vim_bridge = MagicMock()
            mock_vim_bridge.open_file_from_context.return_value = True
            session.vim_bridge = mock_vim_bridge

            session._open_file_in_editor("/path/to/file.py")

            mock_vim_bridge.open_file_from_context.assert_called_once_with("/path/to/file.py")
        finally:
            for p in patches:
                p.stop()
            root.destroy()

    def test_open_file_in_editor_logs_warning_when_disconnected(self, tmp_path: Path) -> None:
        """GIVEN a session whose VimBridge reports disconnected (returns False)
        WHEN _open_file_in_editor('/path/to/file.py') is called
        THEN no exception is raised (graceful degradation).
        """
        session, root, patches = self._make_session(tmp_path)
        try:
            mock_vim_bridge = MagicMock()
            mock_vim_bridge.open_file_from_context.return_value = False
            session.vim_bridge = mock_vim_bridge

            # Must not raise
            session._open_file_in_editor("/path/to/file.py")
        finally:
            for p in patches:
                p.stop()
            root.destroy()
