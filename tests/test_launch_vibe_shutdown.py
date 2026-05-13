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
    assert "send-keys\t-t\t%2\tC-c" in log
    assert "send-keys\t-t\t%0\tC-c" in log
    assert "send-keys\t-t\t%0\t:qa!\tEnter" in log
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
    assert "new-window\t-P\t-F\t#{pane_id}\t-d\t-t\tagentx:" in log
    assert "\t-n\teditor\t-c\t" in log
    assert "send-keys\t-t\t%2\tC-c" in log
    assert "send-keys\t-t\t%2\tnvim" in log
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
    assert "send-keys\t-t\t%0\tnvim" in log
    assert "send-keys\t-t\tagentx:2" in log
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


@pytest.mark.unit
def test_start_with_tui_enabled_launches_tui_window_and_env(tmp_path: Path) -> None:
    """GIVEN TUI mode enabled WHEN launch_vibe.sh start runs THEN tui-chat is window 0 and receives TUI env wiring. [PD-16-AF-003]"""
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
            "AGENTX_TUI_ENABLE": "true",
            "AGENTX_TUI_OUTPUT_FIFO": str(tmp_path / "agentx.tui.output.fifo"),
            "AGENTX_TUI_INPUT_FIFO": str(tmp_path / "agentx.tui.input.fifo"),
            "AGENTX_TUI_SOCKET": str(tmp_path / "agentx.tui.sock"),
            "AGENTX_SOCKET_WAIT_LOOPS": "1",
            "AGENTX_SOCKET_WAIT_SEC": "0",
        },
    )

    assert result.returncode == 0, result.stderr
    log = log_path.read_text(encoding="utf-8")
    assert "new-session\t-d\t-P\t-F\t#{pane_id}\t-s\tagentx\t-n\ttui-chat" in log
    assert "AGENTX_TUI_OUTPUT_FIFO='" in log
    assert "nvim\t--listen" in log
    assert "--cmd\t'luafile" in log
    assert "agentx_tui.lua'" in log
    assert "send-keys\t-t\tagentx:3" in log
    assert "select-window\t-t\tagentx:0" in log
    assert "AGENTX_TUI_ENABLE='true'" in log
    assert "AGENTX_TUI_OUTPUT_FIFO='" in log
    assert "AGENTX_TUI_INPUT_FIFO='" in log
    assert "AGENTX_TUI_OUTPUT_SPLIT_RATIO='" in log
    assert (project_dir / "agentx_tui.lua").exists()
    lua_text = (project_dir / "agentx_tui.lua").read_text(encoding="utf-8")
    assert 'local output_ratio = tonumber(vim.fn.expand("$AGENTX_TUI_OUTPUT_SPLIT_RATIO")) or 0.70' in lua_text
    assert 'vim.cmd("belowright split")' in lua_text
    assert "while true; do cat %q; done" in lua_text
    assert 'nvim_create_user_command("AgentXSubmit"' in lua_text
    assert 'vim.keymap.set({ "n", "i" }, "<leader>s", submit_input' in lua_text
    assert 'vim.keymap.set("n", "<CR>", submit_input' in lua_text
    assert "AgentX TUI ready." in lua_text


@pytest.mark.unit
def test_start_with_tui_disabled_does_not_launch_tui_window_or_lua(tmp_path: Path) -> None:
    """GIVEN default launcher settings WHEN launch_vibe.sh start runs THEN no tui-chat window or TUI Lua file is created. [PD-16-AF-003]"""
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
            "AGENTX_SOCKET_WAIT_LOOPS": "1",
            "AGENTX_SOCKET_WAIT_SEC": "0",
        },
    )

    assert result.returncode == 0, result.stderr
    log = log_path.read_text(encoding="utf-8")
    assert "-n\ttui-chat" not in log
    assert "AGENTX_TUI_ENABLE='true'" not in log
    assert "agentx_tui.lua" not in log
    assert not (project_dir / "agentx_tui.lua").exists()


@pytest.mark.unit
def test_tui_enabled_status_and_stop_report_and_cleanup(tmp_path: Path) -> None:
    """GIVEN TUI-enabled session WHEN status and stop run THEN TUI state is reported and TUI FIFOs are cleaned up. [PD-16-AF-003]"""
    fake_bin = _create_fake_bin(tmp_path)
    log_path = tmp_path / "tmux.log"
    state_path = tmp_path / "tmux.state"
    project_dir = tmp_path / "project"
    project_dir.mkdir(parents=True, exist_ok=True)
    output_fifo = tmp_path / "agentx.tui.output.fifo"
    input_fifo = tmp_path / "agentx.tui.input.fifo"

    shared_env = {
        "TMUX_HAS_SESSION": "0",
        "TMUX_WINDOWS": "editor,agent-bg,agentx-log",
        "TMUX_PANE_COMMAND": "nvim",
        "TMUX_STATE_FILE": str(state_path),
        "AGENTX_TUI_ENABLE": "true",
        "AGENTX_TUI_OUTPUT_FIFO": str(output_fifo),
        "AGENTX_TUI_INPUT_FIFO": str(input_fifo),
        "AGENTX_TUI_SOCKET": str(tmp_path / "agentx.tui.sock"),
        "AGENTX_SOCKET_WAIT_LOOPS": "1",
        "AGENTX_SOCKET_WAIT_SEC": "0",
    }

    start_result = _run_launcher(["start", str(project_dir)], fake_bin, log_path, shared_env)
    assert start_result.returncode == 0, start_result.stderr
    assert output_fifo.exists()
    assert input_fifo.exists()

    status_result = _run_launcher(["status", str(project_dir)], fake_bin, log_path, shared_env)
    assert status_result.returncode == 0, status_result.stderr
    assert "TUI       : enabled" in status_result.stdout
    assert "TUI out" in status_result.stdout
    assert "TUI in" in status_result.stdout

    stop_result = _run_launcher(["stop", str(project_dir)], fake_bin, log_path, shared_env)
    assert stop_result.returncode == 0, stop_result.stderr
    assert not output_fifo.exists()
    assert not input_fifo.exists()


@pytest.mark.unit
def test_status_reports_tui_disabled_by_default(tmp_path: Path) -> None:
    """GIVEN default launcher settings WHEN status runs THEN it reports TUI disabled without TUI path lines. [PD-16-AF-003]"""
    fake_bin = _create_fake_bin(tmp_path)
    log_path = tmp_path / "tmux.log"

    result = _run_launcher(
        ["status"],
        fake_bin,
        log_path,
        {
            "TMUX_HAS_SESSION": "0",
            "AGENTX_TUI_ENABLE": "false",
        },
    )

    assert result.returncode == 0, result.stderr
    assert "TUI       : disabled" in result.stdout
    assert "TUI out" not in result.stdout
    assert "TUI in" not in result.stdout


@pytest.mark.unit
def test_restart_with_tui_enabled_recreates_tui_lifecycle(tmp_path: Path) -> None:
    """GIVEN an existing TUI-enabled session WHEN restart runs THEN stop/start lifecycle keeps tui-chat as window 0 and relaunches it. [PD-16-AF-003]"""
    fake_bin = _create_fake_bin(tmp_path)
    log_path = tmp_path / "tmux.log"
    project_dir = tmp_path / "project"
    project_dir.mkdir(parents=True, exist_ok=True)
    output_fifo = tmp_path / "agentx.restart.tui.output.fifo"
    input_fifo = tmp_path / "agentx.restart.tui.input.fifo"

    result = _run_launcher(
        ["restart", str(project_dir)],
        fake_bin,
        log_path,
        {
            "TMUX_HAS_SESSION": "1",
            "TMUX_WINDOWS": "editor,agent-bg,agentx-log,tui-chat",
            "TMUX_PANE_COMMAND": "nvim",
            "AGENTX_TUI_ENABLE": "true",
            "AGENTX_TUI_OUTPUT_FIFO": str(output_fifo),
            "AGENTX_TUI_INPUT_FIFO": str(input_fifo),
            "AGENTX_TUI_SOCKET": str(tmp_path / "agentx.restart.tui.sock"),
            "AGENTX_SOCKET_WAIT_LOOPS": "1",
            "AGENTX_SOCKET_WAIT_SEC": "0",
        },
    )

    assert result.returncode == 0, result.stderr
    log = log_path.read_text(encoding="utf-8")
    assert "kill-session\t-t\tagentx" in log
    assert "new-session\t-d\t-P\t-F\t#{pane_id}\t-s\tagentx\t-n\ttui-chat" in log
    assert "AGENTX_TUI_OUTPUT_FIFO='" in log
    assert "select-window\t-t\tagentx:0" in log
    assert "agentx_tui.lua" in log
    assert output_fifo.exists()
    assert input_fifo.exists()


@pytest.mark.unit
def test_start_reads_tui_enable_from_project_toml(tmp_path: Path) -> None:
    """GIVEN project config with tui.enable=true WHEN launch_vibe.sh start runs without AGENTX_TUI_ENABLE THEN tui-chat starts as window 0. [PD-16-AF-003]"""
    fake_bin = _create_fake_bin(tmp_path)
    log_path = tmp_path / "tmux.log"
    project_dir = tmp_path / "project"
    project_dir.mkdir(parents=True, exist_ok=True)
    (project_dir / "agentx.toml").write_text(
        """
[agentx]
enable_gui_chat = true

[tui]
enable = true
""".strip() + "\n",
        encoding="utf-8",
    )

    result = _run_launcher(
        ["start", str(project_dir)],
        fake_bin,
        log_path,
        {
            "TMUX_HAS_SESSION": "0",
            "TMUX_WINDOWS": "editor,agent-bg,agentx-log",
            "TMUX_PANE_COMMAND": "nvim",
            "AGENTX_SOCKET_WAIT_LOOPS": "1",
            "AGENTX_SOCKET_WAIT_SEC": "0",
        },
    )

    assert result.returncode == 0, result.stderr
    log = log_path.read_text(encoding="utf-8")
    assert "new-session\t-d\t-P\t-F\t#{pane_id}\t-s\tagentx\t-n\ttui-chat" in log
    assert "select-window\t-t\tagentx:0" in log


@pytest.mark.unit
def test_start_reads_tui_split_ratio_from_project_toml(tmp_path: Path) -> None:
    """GIVEN project config with tui.output_split_ratio WHEN launch_vibe.sh start runs THEN TUI launch command includes AGENTX_TUI_OUTPUT_SPLIT_RATIO. [PD-16-AF-003]"""
    fake_bin = _create_fake_bin(tmp_path)
    log_path = tmp_path / "tmux.log"
    project_dir = tmp_path / "project"
    project_dir.mkdir(parents=True, exist_ok=True)
    (project_dir / "agentx.toml").write_text(
        """
[agentx]
enable_gui_chat = true

[tui]
enable = true
output_split_ratio = 0.62
""".strip() + "\n",
        encoding="utf-8",
    )

    result = _run_launcher(
        ["start", str(project_dir)],
        fake_bin,
        log_path,
        {
            "TMUX_HAS_SESSION": "0",
            "TMUX_WINDOWS": "editor,agent-bg,agentx-log",
            "TMUX_PANE_COMMAND": "nvim",
            "AGENTX_SOCKET_WAIT_LOOPS": "1",
            "AGENTX_SOCKET_WAIT_SEC": "0",
        },
    )

    assert result.returncode == 0, result.stderr
    log = log_path.read_text(encoding="utf-8")
    assert "AGENTX_TUI_OUTPUT_SPLIT_RATIO='0.62'" in log


@pytest.mark.unit
def test_start_reads_auto_stop_override_from_project_toml(tmp_path: Path) -> None:
    """GIVEN project config with agentx.auto_stop_tmux_on_gui_exit=false WHEN launch_vibe.sh start runs THEN AgentX command omits tmux kill-session hook. [PD-15-AF-008]"""
    fake_bin = _create_fake_bin(tmp_path)
    log_path = tmp_path / "tmux.log"
    project_dir = tmp_path / "project"
    project_dir.mkdir(parents=True, exist_ok=True)
    (project_dir / "agentx.toml").write_text(
        """
[agentx]
enable_gui_chat = true
auto_stop_tmux_on_gui_exit = false
""".strip() + "\n",
        encoding="utf-8",
    )

    result = _run_launcher(
        ["start", str(project_dir)],
        fake_bin,
        log_path,
        {
            "TMUX_HAS_SESSION": "0",
            "TMUX_WINDOWS": "editor,agent-bg,agentx-log",
            "TMUX_PANE_COMMAND": "nvim",
            "AGENTX_SOCKET_WAIT_LOOPS": "1",
            "AGENTX_SOCKET_WAIT_SEC": "0",
        },
    )

    assert result.returncode == 0, result.stderr
    log = log_path.read_text(encoding="utf-8")
    assert "send-keys\t-t\tagentx:2" in log
    assert "tmux\tkill-session\t-t\t'agentx'" not in log


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
    env["TMUX_STATE_FILE"] = f"{log_path}.state"
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

state_file="${TMUX_STATE_FILE:-${TMUX_LOG:-/tmp/tmux}.state}"
if [[ ! -f "$state_file" ]]; then
    {
        echo "HAS_SESSION=${TMUX_HAS_SESSION:-1}"
        echo "WINDOWS=${TMUX_WINDOWS:-editor,agent-bg,agentx-log}"
        echo "OPT_AGENTX_EDITOR_PANE="
        echo "OPT_AGENTX_AGENT_BG_PANE="
        echo "OPT_AGENTX_AGENTX_LOG_PANE="
    } > "$state_file"
fi

# shellcheck disable=SC1090
source "$state_file"

save_state() {
    {
        echo "HAS_SESSION=$HAS_SESSION"
        echo "WINDOWS=$WINDOWS"
        echo "OPT_AGENTX_EDITOR_PANE=$OPT_AGENTX_EDITOR_PANE"
        echo "OPT_AGENTX_AGENT_BG_PANE=$OPT_AGENTX_AGENT_BG_PANE"
        echo "OPT_AGENTX_AGENTX_LOG_PANE=$OPT_AGENTX_AGENTX_LOG_PANE"
    } > "$state_file"
}

pane_id_for_index() {
    local idx="$1"
    echo "%${idx}"
}

window_index_for_name() {
    local name="$1"
    IFS=',' read -r -a wins <<< "${WINDOWS:-editor,agent-bg,agentx-log}"
    local i=0
    for w in "${wins[@]}"; do
        if [[ "$w" == "$name" ]]; then
            echo "$i"
            return 0
        fi
        i=$((i + 1))
    done
    echo ""
    return 1
}

parse_target_index() {
    local target="$1"
    if [[ "$target" == %* ]]; then
        echo "${target#%}"
        return 0
    fi
    if [[ "$target" == *":"* ]]; then
        local win_part="${target#*:}"
        echo "${win_part%%.*}"
        return 0
    fi
    echo ""
    return 1
}

if [[ -n "${TMUX_LOG:-}" ]]; then
    printf '%s\\n' "$*" | sed 's/ /\\t/g' >> "$TMUX_LOG"
fi

cmd="${1:-}"
if [[ "$cmd" == "has-session" ]]; then
    [[ "${HAS_SESSION:-1}" == "1" ]]
    exit $?
fi

if [[ "$cmd" == "list-windows" ]]; then
    [[ "${HAS_SESSION:-1}" == "1" ]] || exit 1
    IFS=',' read -r -a wins <<< "${WINDOWS:-editor,agent-bg,agentx-log}"
    format=""
    prev=""
    for a in "$@"; do
        if [[ "$prev" == "-F" ]]; then
            format="$a"
            break
        fi
        prev="$a"
    done

    if [[ -z "$format" || "$format" == "#{window_name}" ]]; then
        printf '%s\\n' "${wins[@]}"
    elif [[ "$format" == "#{window_index}:#{window_name}" ]]; then
        i=0
        for w in "${wins[@]}"; do
            printf '%s:%s\\n' "$i" "$w"
            i=$((i + 1))
        done
    elif [[ "$format" == "#{window_index}" ]]; then
        i=0
        for _w in "${wins[@]}"; do
            printf '%s\\n' "$i"
            i=$((i + 1))
        done
    else
        printf '%s\\n' "${wins[@]}"
    fi
    exit 0
fi

if [[ "$cmd" == "list-panes" ]]; then
    [[ "${HAS_SESSION:-1}" == "1" ]] || exit 1

    if printf '%s\n' "$*" | grep -q -- "-a"; then
        IFS=',' read -r -a wins <<< "${WINDOWS:-editor,agent-bg,agentx-log}"
        if printf '%s\n' "$*" | grep -q -- "#{pane_id}"; then
            i=0
            for _w in "${wins[@]}"; do
                echo "$(pane_id_for_index "$i")"
                i=$((i + 1))
            done
            exit 0
        fi
        exit 0
    fi

    target=""
    prev=""
    for a in "$@"; do
        if [[ "$prev" == "-t" ]]; then
            target="$a"
            break
        fi
        prev="$a"
    done
    win="$(parse_target_index "$target" || true)"
    IFS=',' read -r -a wins <<< "${WINDOWS:-editor,agent-bg,agentx-log}"
    i=0
    for w in "${wins[@]}"; do
        if [[ "$w" == "$win" || "$i" == "$win" ]]; then
            exit 0
        fi
        i=$((i + 1))
    done
    exit 1
fi

if [[ "$cmd" == "new-session" ]]; then
    HAS_SESSION=1
    name="editor"
    prev=""
    for a in "$@"; do
        if [[ "$prev" == "-n" ]]; then
            name="$a"
            break
        fi
        prev="$a"
    done
    WINDOWS="$name"
    save_state
    if printf '%s\n' "$*" | grep -q -- "-P"; then
        echo "$(pane_id_for_index 0)"
    fi
    exit 0
fi

if [[ "$cmd" == "new-window" ]]; then
    name=""
    prev=""
    for a in "$@"; do
        if [[ "$prev" == "-n" ]]; then
            name="$a"
            break
        fi
        prev="$a"
    done
    if [[ -n "$name" ]]; then
        if [[ -z "$WINDOWS" ]]; then
            WINDOWS="$name"
        elif [[ ",$WINDOWS," != *",$name,"* ]]; then
            WINDOWS="$WINDOWS,$name"
        fi
        save_state
        if printf '%s\n' "$*" | grep -q -- "-P"; then
            idx="$(window_index_for_name "$name")"
            echo "$(pane_id_for_index "$idx")"
        fi
    fi
    exit 0
fi

if [[ "$cmd" == "set-option" ]]; then
    key=""
    value=""
    prev=""
    for a in "$@"; do
        if [[ "$a" == "-q" || "$a" == "-t" ]]; then
            prev="$a"
            continue
        fi
        if [[ "$prev" == "-t" ]]; then
            prev=""
            continue
        fi
        if [[ -z "$key" ]]; then
            key="$a"
        elif [[ -z "$value" ]]; then
            value="$a"
            break
        fi
    done
    case "$key" in
        @agentx_editor_pane) OPT_AGENTX_EDITOR_PANE="$value" ;;
        @agentx_agent_bg_pane) OPT_AGENTX_AGENT_BG_PANE="$value" ;;
        @agentx_agentx_log_pane) OPT_AGENTX_AGENTX_LOG_PANE="$value" ;;
    esac
    save_state
    exit 0
fi

if [[ "$cmd" == "show-options" ]]; then
    key="${@: -1}"
    case "$key" in
        @agentx_editor_pane) echo "$OPT_AGENTX_EDITOR_PANE" ;;
        @agentx_agent_bg_pane) echo "$OPT_AGENTX_AGENT_BG_PANE" ;;
        @agentx_agentx_log_pane) echo "$OPT_AGENTX_AGENTX_LOG_PANE" ;;
        *) echo "" ;;
    esac
    exit 0
fi

if [[ "$cmd" == "kill-session" ]]; then
    HAS_SESSION=0
    save_state
    exit 0
fi

if [[ "$cmd" == "display-message" ]]; then
    target=""
    format=""
    prev=""
    for a in "$@"; do
        if [[ "$prev" == "-t" ]]; then
            target="$a"
        fi
        if [[ "$prev" == "-p" ]]; then
            format="$a"
        fi
        prev="$a"
    done

    idx="$(parse_target_index "$target" || true)"
    if [[ -z "$idx" ]]; then
        idx="0"
    fi

    for a in "$@"; do
        if [[ "$a" == "#{pane_current_path}" ]]; then
            echo "${TMUX_PANE_PATH:-$PWD}"
            exit 0
        fi
        if [[ "$a" == "#{pane_current_command}" ]]; then
            echo "${TMUX_PANE_COMMAND:-bash}"
            exit 0
        fi
        if [[ "$a" == "#{pane_id}" ]]; then
            echo "$(pane_id_for_index "$idx")"
            exit 0
        fi
        if [[ "$a" == "#{session_name}:#{window_index}" ]]; then
            echo "agentx:${idx}"
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
