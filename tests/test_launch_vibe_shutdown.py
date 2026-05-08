"""Unit tests for launch_vibe lifecycle shutdown and recovery flows."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path

import pytest


@pytest.mark.unit
def test_stop_gracefully_stops_agentx_and_editor(tmp_path: Path) -> None:
    """GIVEN a running tmux session WHEN launch_vibe.sh stop is called THEN AgentX and neovim receive graceful stop signals before session kill. [PD-15-AF-008]"""
    fake_bin = _create_fake_bin(tmp_path)
    log_path = tmp_path / "tmux.log"

    result = _run_launcher(
        ["stop"],
        fake_bin,
        log_path,
        {
            "TMUX_HAS_SESSION": "1",
            "TMUX_WINDOWS": "editor,agent-bg,agentx-log",
        },
    )

    assert result.returncode == 0, result.stderr
    log = log_path.read_text(encoding="utf-8")
    assert "send-keys\t-t\tagentx:agentx-log.0\tC-c" in log
    assert "send-keys\t-t\tagentx:editor.0\tC-c" in log
    assert "send-keys\t-t\tagentx:editor.0\t:qa!\tEnter" in log
    assert "kill-session\t-t\tagentx" in log


@pytest.mark.unit
def test_stop_is_noop_when_session_missing(tmp_path: Path) -> None:
    """GIVEN no running tmux session WHEN launch_vibe.sh stop is called THEN command exits successfully without kill-session. [PD-15-AF-008]"""
    fake_bin = _create_fake_bin(tmp_path)
    log_path = tmp_path / "tmux.log"

    result = _run_launcher(
        ["stop"],
        fake_bin,
        log_path,
        {
            "TMUX_HAS_SESSION": "0",
        },
    )

    assert result.returncode == 0, result.stderr
    assert "No active tmux session 'agentx' to stop." in result.stdout
    log = log_path.read_text(encoding="utf-8")
    assert "kill-session\t-t\tagentx" not in log


@pytest.mark.unit
def test_recover_editor_recreates_window_and_relaunches_nvim(tmp_path: Path) -> None:
    """GIVEN editor window is missing WHEN launch_vibe.sh recover-editor runs THEN the editor window is recreated and neovim relaunches in pane 0. [PD-14-AF-008]"""
    fake_bin = _create_fake_bin(tmp_path)
    log_path = tmp_path / "tmux.log"
    project_dir = tmp_path / "project"
    project_dir.mkdir(parents=True, exist_ok=True)

    result = _run_launcher(
        ["recover-editor", str(project_dir)],
        fake_bin,
        log_path,
        {
            "TMUX_HAS_SESSION": "1",
            "TMUX_WINDOWS": "agent-bg,agentx-log",
            "AGENTX_NVIM_SOCKET": str(tmp_path / "agentx.nvim.sock"),
            "AGENTX_SAVES_FIFO": str(tmp_path / "agentx_saves.fifo"),
        },
    )

    assert result.returncode == 0, result.stderr
    log = log_path.read_text(encoding="utf-8")
    assert f"new-window\t-d\t-t\tagentx\t-n\teditor\t-c\t{project_dir}" in log
    assert "send-keys\t-t\tagentx:editor.0\tC-c" in log
    assert "send-keys\t-t\tagentx:editor.0\tnvim" in log
    assert "--listen" in log
    assert (project_dir / ".nvimrc.agentx").exists()


@pytest.mark.unit
def test_start_launches_agentx_with_gui_exit_shutdown_hook(tmp_path: Path) -> None:
    """GIVEN a fresh session WHEN launch_vibe.sh start runs THEN AgentX command in window 2 includes session kill hook after GUI exits. [PD-15-AF-008]"""
    fake_bin = _create_fake_bin(tmp_path)
    log_path = tmp_path / "tmux.log"
    project_dir = tmp_path / "project"
    project_dir.mkdir(parents=True, exist_ok=True)

    result = _run_launcher(
        ["start", str(project_dir)],
        fake_bin,
        log_path,
        {
            "TMUX_HAS_SESSION": "0",
            "TMUX_WINDOWS": "editor,agent-bg,agentx-log",
            "TMUX_PANE_COMMAND": "nvim",
            "AGENTX_NVIM_SOCKET": str(tmp_path / "agentx.nvim.sock"),
            "AGENTX_SAVES_FIFO": str(tmp_path / "agentx_saves.fifo"),
            "AGENTX_SOCKET_WAIT_LOOPS": "1",
            "AGENTX_SOCKET_WAIT_SEC": "0",
        },
    )

    assert result.returncode == 0, result.stderr
    log = log_path.read_text(encoding="utf-8")
    assert "send-keys\t-t\tagentx:editor.0\tnvim" in log
    assert "send-keys\t-t\tagentx:agentx-log" in log
    assert "tmux\tkill-session\t-t\t'agentx'" in log


@pytest.mark.unit
def test_multiple_sessions_use_scoped_sockets(tmp_path: Path) -> None:
    """GIVEN multiple launch_vibe.sh sessions with different AGENTX_TMUX_SESSION values WHEN started with default socket paths THEN each uses its own scoped socket and FIFO without collision. [PD-15-AF-009]"""
    fake_bin = _create_fake_bin(tmp_path)
    project_dir = tmp_path / "project"
    project_dir.mkdir(parents=True, exist_ok=True)

    # Session A: default session name 'agentx'
    log_a = tmp_path / "tmux_a.log"
    result_a = _run_launcher(
        ["start", str(project_dir)],
        fake_bin,
        log_a,
        {
            "TMUX_HAS_SESSION": "0",
            "TMUX_WINDOWS": "editor,agent-bg,agentx-log",
            "TMUX_PANE_COMMAND": "nvim",
            "AGENTX_TMUX_SESSION": "agentx",
            # Do NOT override socket/FIFO — let them be scoped by session ID
            "AGENTX_SOCKET_WAIT_LOOPS": "1",
            "AGENTX_SOCKET_WAIT_SEC": "0",
        },
    )

    # Session B: different session name 'agentx-user2'
    log_b = tmp_path / "tmux_b.log"
    result_b = _run_launcher(
        ["start", str(project_dir)],
        fake_bin,
        log_b,
        {
            "TMUX_HAS_SESSION": "0",
            "TMUX_WINDOWS": "editor,agent-bg,agentx-log",
            "TMUX_PANE_COMMAND": "nvim",
            "AGENTX_TMUX_SESSION": "agentx-user2",
            # Do NOT override socket/FIFO — let them be scoped by session ID
            "AGENTX_SOCKET_WAIT_LOOPS": "1",
            "AGENTX_SOCKET_WAIT_SEC": "0",
        },
    )

    assert result_a.returncode == 0, result_a.stderr
    assert result_b.returncode == 0, result_b.stderr

    log_a_text = log_a.read_text(encoding="utf-8")
    log_b_text = log_b.read_text(encoding="utf-8")

    # Session A should use agentx-scoped paths (session ID = 'agentx')
    assert "agentx_agentx.nvim.sock" in log_a_text
    assert "agentx_agentx.saves.fifo" in log_a_text

    # Session B should use agentx-user2-scoped paths (session ID = 'agentx-user2')
    assert "agentx_agentx-user2.nvim.sock" in log_b_text
    assert "agentx_agentx-user2.saves.fifo" in log_b_text

    # Verify they are NOT using the same paths
    assert "agentx-user2" not in log_a_text  # A uses agentx, not agentx-user2
    assert "agentx_agentx.nvim.sock" not in log_b_text  # B uses agentx-user2, not plain agentx


def _run_launcher(
    args: list[str],
    fake_bin: Path,
    log_path: Path,
    extra_env: dict[str, str],
) -> subprocess.CompletedProcess[str]:
    """Run launch_vibe.sh with a fake tmux/nvim toolchain for hermetic tests."""
    env = os.environ.copy()
    env["PATH"] = f"{fake_bin}:{env.get('PATH', '')}"
    env["TMUX_LOG"] = str(log_path)
    env.update(extra_env)

    script_path = Path(__file__).resolve().parents[1] / "launch_vibe.sh"
    return subprocess.run(
        ["bash", str(script_path), *args],
        cwd=Path(__file__).resolve().parents[1],
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )


def _create_fake_bin(tmp_path: Path) -> Path:
    """Create fake tmux/nvim/python executables that log invocations."""
    fake_bin = tmp_path / "fake-bin"
    fake_bin.mkdir(parents=True, exist_ok=True)

    tmux_path = fake_bin / "tmux"
    tmux_path.write_text(
        """#!/usr/bin/env bash
set -euo pipefail

if [[ -n "${TMUX_LOG:-}" ]]; then
    printf '%s\\n' "$*" | sed 's/ /\\t/g' >> "$TMUX_LOG"
fi

cmd="${1:-}"
if [[ "$cmd" == "has-session" ]]; then
    [[ "${TMUX_HAS_SESSION:-1}" == "1" ]]
    exit $?
fi

if [[ "$cmd" == "list-windows" ]]; then
    [[ "${TMUX_HAS_SESSION:-1}" == "1" ]] || exit 1
    IFS=',' read -r -a wins <<< "${TMUX_WINDOWS:-editor,agent-bg,agentx-log}"
    printf '%s\\n' "${wins[@]}"
    exit 0
fi

if [[ "$cmd" == "list-panes" ]]; then
    [[ "${TMUX_HAS_SESSION:-1}" == "1" ]] || exit 1
    target=""
    prev=""
    for a in "$@"; do
        if [[ "$prev" == "-t" ]]; then
            target="$a"
            break
        fi
        prev="$a"
    done
    win="editor"
    if [[ "$target" == *":"* ]]; then
        win_part="${target#*:}"
        win="${win_part%%.*}"
    fi
    IFS=',' read -r -a wins <<< "${TMUX_WINDOWS:-editor,agent-bg,agentx-log}"
    for w in "${wins[@]}"; do
        if [[ "$w" == "$win" ]]; then
            exit 0
        fi
    done
    exit 1
fi

if [[ "$cmd" == "display-message" ]]; then
    for a in "$@"; do
        if [[ "$a" == "#{pane_current_path}" ]]; then
            echo "${TMUX_PANE_PATH:-$PWD}"
            exit 0
        fi
        if [[ "$a" == "#{pane_current_command}" ]]; then
            echo "${TMUX_PANE_COMMAND:-bash}"
            exit 0
        fi
    done
fi

exit 0
""",
        encoding="utf-8",
    )
    tmux_path.chmod(0o755)

    nvim_path = fake_bin / "nvim"
    nvim_path.write_text(
        """#!/usr/bin/env bash
if [[ "$1" == "--version" ]]; then
  echo "NVIM v0.10.0"
  exit 0
fi
exit 0
""",
        encoding="utf-8",
    )
    nvim_path.chmod(0o755)

    python3_path = fake_bin / "python3"
    python3_path.write_text(
        """#!/usr/bin/env bash
exit 0
""",
        encoding="utf-8",
    )
    python3_path.chmod(0o755)

    return fake_bin
