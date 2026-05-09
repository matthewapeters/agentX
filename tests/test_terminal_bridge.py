"""Unit tests for tmux dispatch behavior in TerminalBridge."""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import patch

import pytest

from agentx.integration.terminal_bridge import (
    TerminalBridge,
    TerminalResult,
    configure_terminal_bridge,
    terminal_run,
)


def _tmux_ok(stdout: str = ""):
    class Result:
        returncode = 0
        stderr = ""

        def __init__(self, output: str) -> None:
            self.stdout = output

    return Result(stdout)


@pytest.mark.unit
def test_run_command_visible_creates_ephemeral_pane_and_dispatches(tmp_path: Path) -> None:
    """GIVEN active tmux session WHEN visible command runs THEN new pane is created and send-keys dispatches command. [PD-15-AF-001]"""

    config = {"terminal": {"allow": ["pytest"], "confirm": [], "deny": []}}
    audit_path = tmp_path / "terminal_audit.jsonl"
    bridge = TerminalBridge(config=config, session_id="agentx", audit_log_path=str(audit_path))

    calls: list[list[str]] = []

    def fake_run(cmd, capture_output, text, check):
        calls.append(cmd)
        if cmd[1:4] == ["has-session", "-t", "agentx"]:
            return _tmux_ok("")
        if cmd[1:6] == ["new-window", "-P", "-F", "#{pane_id}", "-t"]:
            return _tmux_ok("%7\n")
        return _tmux_ok("")

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake_run):
            result = bridge.run_command("pytest tests/", visible=True)

    assert result.exit_code == 0
    assert result.pane_id == "%7"
    assert any(c[1] == "new-window" for c in calls)
    assert any(c[1] == "send-keys" and c[3] == "%7" for c in calls)


@pytest.mark.unit
def test_run_command_requires_approval_without_callback_is_rejected(tmp_path: Path) -> None:
    """GIVEN supervised mode and confirm command WHEN no approval callback exists THEN command is rejected. [PD-15-AF-006]"""

    config = {"terminal": {"allow": [], "confirm": ["git commit"], "deny": []}}
    bridge = TerminalBridge(config=config, session_id="agentx", audit_log_path=str(tmp_path / "audit.jsonl"))

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        result = bridge.run_command("git commit -m 'wip'", context="save changes", visible=True)

    assert result.exit_code == -1
    assert result.decision == "rejected"


@pytest.mark.unit
def test_deny_list_blocks_command_before_tmux_call(tmp_path: Path) -> None:
    """GIVEN denied command WHEN run is requested THEN tmux is not called and command is denied. [PD-15-AF-007]"""

    config = {"terminal": {"allow": [], "confirm": [], "deny": ["rm "]}}
    bridge = TerminalBridge(config=config, session_id="agentx", audit_log_path=str(tmp_path / "audit.jsonl"))

    with patch("agentx.integration.terminal_bridge.subprocess.run") as mocked_run:
        result = bridge.run_command("rm -rf build/", visible=True)

    assert result.exit_code == -1
    assert result.decision == "denied"
    mocked_run.assert_not_called()


@pytest.mark.unit
def test_path_violation_blocks_before_tmux_call(tmp_path: Path) -> None:
    """GIVEN out-of-bounds path WHEN command runs THEN decision is path violation and tmux is not called. [PD-15-AF-007]"""

    project_root = tmp_path / "project"
    project_root.mkdir(parents=True, exist_ok=True)

    config = {"terminal": {"allow": ["cat"], "confirm": [], "deny": []}}
    bridge = TerminalBridge(
        config=config,
        session_id="agentx",
        project_roots=[str(project_root)],
        audit_log_path=str(tmp_path / "audit.jsonl"),
    )

    with patch("agentx.integration.terminal_bridge.subprocess.run") as mocked_run:
        result = bridge.run_command("cat /etc/passwd", visible=True)

    assert result.exit_code == -1
    assert result.decision == "path_violation"
    mocked_run.assert_not_called()


@pytest.mark.unit
def test_run_command_appends_audit_log_entry(tmp_path: Path) -> None:
    """GIVEN dispatched terminal command WHEN run completes THEN terminal audit jsonl entry is appended. [PD-15-AF-002]"""

    config = {"terminal": {"allow": ["pytest"], "confirm": [], "deny": []}}
    audit_path = tmp_path / "terminal_audit.jsonl"
    bridge = TerminalBridge(config=config, session_id="agentx", audit_log_path=str(audit_path))

    def fake_run(cmd, capture_output, text, check):
        if cmd[1:4] == ["has-session", "-t", "agentx"]:
            return _tmux_ok("")
        if cmd[1:6] == ["new-window", "-P", "-F", "#{pane_id}", "-t"]:
            return _tmux_ok("%2\n")
        return _tmux_ok("")

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake_run):
            bridge.run_command("pytest tests/", visible=True)

    lines = audit_path.read_text(encoding="utf-8").strip().splitlines()
    assert len(lines) == 1
    payload = json.loads(lines[0])
    assert payload["session_id"] == "agentx"
    assert payload["executed_command"] == "pytest tests/"
    assert payload["decision"] == "allowed"


@pytest.mark.unit
def test_terminal_run_wrapper_uses_config_defaults(tmp_path: Path) -> None:
    """GIVEN configured terminal defaults WHEN wrapper is called without explicit options THEN defaults are forwarded. [PD-15-AF-002]"""

    config = {
        "terminal": {
            "terminal_visible": False,
            "terminal_auto_close": False,
            "terminal_timeout_sec": 17,
            "allow": ["pytest"],
            "confirm": [],
            "deny": [],
        }
    }
    bridge = configure_terminal_bridge(
        config=config,
        session_id="agentx",
        project_roots=[str(tmp_path)],
        audit_log_path=str(tmp_path / "audit.jsonl"),
    )

    with patch.object(
        bridge,
        "run_command",
        return_value=TerminalResult(
            pane_id="",
            exit_code=0,
            stdout="ok",
            timed_out=False,
            decision="allowed",
            original_command="pytest tests/",
            executed_command="pytest tests/",
        ),
    ) as mocked_run:
        terminal_run("pytest tests/")

    _, kwargs = mocked_run.call_args
    assert kwargs["visible"] is False
    assert kwargs["auto_close"] is False
    assert kwargs["timeout_sec"] == 17


@pytest.mark.unit
def test_confirm_command_dispatches_when_approved(tmp_path: Path) -> None:
    """GIVEN supervised confirm-list command and approval callback WHEN approved THEN dispatch proceeds and decision is approved. [PD-15-AF-006]"""

    config = {"terminal": {"allow": [], "confirm": ["git commit"], "deny": []}}
    bridge = TerminalBridge(
        config=config,
        session_id="agentx",
        approval_callback=lambda cmd, _ctx: (True, cmd),
        audit_log_path=str(tmp_path / "audit.jsonl"),
    )

    calls: list[list[str]] = []

    def fake_run(cmd, capture_output, text, check):
        calls.append(cmd)
        if cmd[1:4] == ["has-session", "-t", "agentx"]:
            return _tmux_ok("")
        if cmd[1:6] == ["new-window", "-P", "-F", "#{pane_id}", "-t"]:
            return _tmux_ok("%9\n")
        return _tmux_ok("")

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake_run):
            result = bridge.run_command("git commit -m 'wip'", context="save", visible=True)

    assert result.decision == "approved"
    assert any(c[1] == "send-keys" for c in calls)
