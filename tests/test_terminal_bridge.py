"""Unit tests for tmux dispatch behavior in TerminalBridge."""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import patch

import pytest

from agentx.integration.terminal_bridge import (
    TerminalBridge,
    TerminalResult,
    _CAPTURE_SENTINEL_PREFIX,
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


def _tmux_err(stderr: str = "error"):
    """Return a subprocess-like object simulating a failing tmux command."""

    class Result:
        returncode = 1

        def __init__(self, msg: str) -> None:
            self.stderr = msg
            self.stdout = ""

    return Result(stderr)


def _extract_sentinel(send_keys_cmd: list[str]) -> str | None:
    """Parse the unique sentinel prefix from a tmux send-keys command list.

    The shell command appended by ``run_command`` looks like::

        user_cmd; echo __AGENTX_DONE__<uuid>__$?

    This function extracts the ``__AGENTX_DONE__<uuid>__`` prefix so tests can
    construct the correct capture-pane response without knowing the UUID ahead
    of time.

    Args:
        send_keys_cmd: Full ``subprocess.run`` argument list for ``send-keys``.

    Returns:
        The sentinel prefix string, or ``None`` if not found.
    """

    if len(send_keys_cmd) < 5:
        return None
    shell_cmd = send_keys_cmd[4]
    for token in shell_cmd.split():
        if token.startswith(_CAPTURE_SENTINEL_PREFIX):
            return token.rstrip("$?")
    return None


class _StatefulFakeTmux:
    """Stateful fake ``subprocess.run`` for tmux that echoes the sentinel from send-keys.

    Extracts the unique per-invocation sentinel from the ``send-keys`` call and
    returns it in the subsequent ``capture-pane`` response, simulating real tmux
    pane scrollback.

    Args:
        pane_id: Pane id string returned by ``new-window``.
        exit_code: Simulated process exit code embedded in the sentinel reply.
        capture_raises: When ``True``, ``capture-pane`` raises ``RuntimeError``
            (simulates pane disappearing mid-poll).
    """

    def __init__(
        self,
        pane_id: str = "%7",
        exit_code: int = 0,
        capture_raises: bool = False,
    ) -> None:
        """Initialise fake tmux state."""

        self.calls: list[list[str]] = []
        self._pane_id = pane_id
        self._exit_code = exit_code
        self._capture_raises = capture_raises
        self._sentinel: str | None = None

    def __call__(self, cmd: list[str], capture_output: bool, text: bool, check: bool) -> object:
        """Dispatch fake tmux subcommands."""

        self.calls.append(cmd)

        if cmd[1:4] == ["has-session", "-t", "agentx"]:
            return _tmux_ok("")

        if cmd[1:6] == ["new-window", "-P", "-F", "#{pane_id}", "-t"]:
            return _tmux_ok(f"{self._pane_id}\n")

        if cmd[1] == "send-keys":
            self._sentinel = _extract_sentinel(cmd)
            return _tmux_ok("")

        if cmd[1:4] == ["capture-pane", "-p", "-t"]:
            if self._capture_raises:
                return _tmux_err("no pane")
            if self._sentinel:
                return _tmux_ok(f"some output\n{self._sentinel}{self._exit_code}\n")
            return _tmux_ok("")

        return _tmux_ok("")


@pytest.mark.unit
def test_run_command_visible_creates_ephemeral_pane_and_dispatches(tmp_path: Path) -> None:
    """GIVEN active tmux session WHEN visible command runs THEN new pane is created and send-keys dispatches command. [PD-15-AF-001]"""

    config = {"terminal": {"allow": ["pytest"], "confirm": [], "deny": []}}
    audit_path = tmp_path / "terminal_audit.jsonl"
    bridge = TerminalBridge(config=config, session_id="agentx", audit_log_path=str(audit_path))

    fake = _StatefulFakeTmux(pane_id="%7")

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake):
            with patch("agentx.integration.terminal_bridge.time.sleep"):
                result = bridge.run_command("pytest tests/", visible=True)

    assert result.exit_code == 0
    assert result.pane_id == "%7"
    assert any(c[1] == "new-window" for c in fake.calls)
    assert any(c[1] == "send-keys" and c[3] == "%7" for c in fake.calls)


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

    fake = _StatefulFakeTmux(pane_id="%2")

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake):
            with patch("agentx.integration.terminal_bridge.time.sleep"):
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

    fake = _StatefulFakeTmux(pane_id="%9")

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake):
            with patch("agentx.integration.terminal_bridge.time.sleep"):
                result = bridge.run_command("git commit -m 'wip'", context="save", visible=True)

    assert result.decision == "approved"
    assert any(c[1] == "send-keys" for c in fake.calls)


@pytest.mark.unit
def test_run_command_captures_exit_code_from_sentinel(tmp_path: Path) -> None:
    """GIVEN command exits with code 42 WHEN unique sentinel echoed in pane THEN exit_code is 42 and stdout is cleaned.

    GIVEN an active tmux session and a command that exits with code 42
    WHEN run_command dispatches the command and capture-pane returns the per-invocation
    sentinel followed by '42'
    THEN result.exit_code == 42 and the sentinel prefix is absent from result.stdout. [PD-15-AF-009]
    """

    config = {"terminal": {"allow": ["pytest"], "confirm": [], "deny": []}}
    bridge = TerminalBridge(config=config, session_id="agentx", audit_log_path=str(tmp_path / "audit.jsonl"))

    fake = _StatefulFakeTmux(pane_id="%3", exit_code=42)

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake):
            with patch("agentx.integration.terminal_bridge.time.sleep"):
                result = bridge.run_command("pytest tests/", visible=True, auto_close=False)

    assert result.exit_code == 42
    assert result.timed_out is False
    assert "some output" in result.stdout
    assert _CAPTURE_SENTINEL_PREFIX not in result.stdout


@pytest.mark.unit
def test_run_command_timeout_sets_timed_out_flag_and_kills_pane(tmp_path: Path) -> None:
    """GIVEN command exceeds timeout WHEN timeout_sec=0 THEN timed_out=True, exit_code=-1, kill-pane called.

    GIVEN an active tmux session and timeout_sec=0 (deadline already passed)
    WHEN run_command dispatches and the poll loop never executes
    THEN result.timed_out is True, exit_code is -1, and kill-pane is invoked on the
    ephemeral pane. [PD-15-AF-009]
    """

    config = {"terminal": {"allow": ["pytest"], "confirm": [], "deny": []}}
    bridge = TerminalBridge(config=config, session_id="agentx", audit_log_path=str(tmp_path / "audit.jsonl"))

    fake = _StatefulFakeTmux(pane_id="%4")

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake):
            result = bridge.run_command("pytest tests/", visible=True, timeout_sec=0)

    assert result.timed_out is True
    assert result.exit_code == -1
    assert any(c[1] == "kill-pane" for c in fake.calls)


@pytest.mark.unit
def test_run_command_persistent_pane_timeout_sends_ctrl_c_not_kill(tmp_path: Path) -> None:
    """GIVEN persistent pane and timeout WHEN deadline exceeded THEN Ctrl+C sent, kill-pane NOT called.

    GIVEN visible=False (persistent pane) and timeout_sec=0
    WHEN the poll loop times out
    THEN send-keys with 'C-c' is called to interrupt the running command without
    destroying the persistent shell, and kill-pane is not invoked. [PD-15-AF-009]
    """

    config = {"terminal": {"allow": ["pytest"], "confirm": [], "deny": []}}
    bridge = TerminalBridge(config=config, session_id="agentx", audit_log_path=str(tmp_path / "audit.jsonl"))

    fake = _StatefulFakeTmux(pane_id="agentx:1.0")

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake):
            result = bridge.run_command("pytest tests/", visible=False, timeout_sec=0)

    assert result.timed_out is True
    assert result.exit_code == -1
    # Ctrl+C sent to persistent pane
    assert any(c[1] == "send-keys" and "C-c" in c for c in fake.calls)
    # kill-pane must NOT be called on timeout of persistent pane
    assert not any(c[1] == "kill-pane" for c in fake.calls)


@pytest.mark.unit
def test_run_command_pane_closed_early_returns_gracefully(tmp_path: Path) -> None:
    """GIVEN pane closes during polling WHEN capture-pane raises RuntimeError THEN graceful result returned.

    GIVEN a command dispatched to an ephemeral pane that closes before the first
    capture-pane poll (e.g. the command finishes and tmux auto-destroys the pane
    before our poll fires)
    WHEN _wait_for_completion calls capture-pane and gets a RuntimeError
    THEN timed_out is False, exit_code is -1, and stdout indicates the early close.
    [PD-15-AF-009]
    """

    config = {"terminal": {"allow": ["pytest"], "confirm": [], "deny": []}}
    bridge = TerminalBridge(config=config, session_id="agentx", audit_log_path=str(tmp_path / "audit.jsonl"))

    # _capture_raises=True makes capture-pane return a non-zero returncode,
    # causing _run_tmux to raise RuntimeError as it would in production.
    fake = _StatefulFakeTmux(pane_id="%6", capture_raises=True)

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake):
            with patch("agentx.integration.terminal_bridge.time.sleep"):
                result = bridge.run_command("pytest tests/", visible=True, auto_close=False)

    assert result.timed_out is False
    assert result.exit_code == -1
    assert "pane closed" in result.stdout


@pytest.mark.unit
def test_run_command_edited_command_is_dispatched(tmp_path: Path) -> None:
    """GIVEN approval callback returns edited command WHEN command approved THEN edited command is dispatched.

    GIVEN supervised mode, a confirm-list command, and an approval callback that edits the command
    WHEN run_command calls the callback and user edits the command before approving
    THEN result.executed_command reflects the edited command and send-keys carries it. [PD-15-AF-006]
    """

    config = {"terminal": {"allow": [], "confirm": ["git commit"], "deny": []}}
    edited = "git commit --amend -m 'fix: correct message'"
    bridge = TerminalBridge(
        config=config,
        session_id="agentx",
        approval_callback=lambda cmd, _ctx: (True, edited),
        audit_log_path=str(tmp_path / "audit.jsonl"),
    )

    fake = _StatefulFakeTmux(pane_id="%5")

    with patch("agentx.integration.terminal_bridge.shutil.which", return_value="/usr/bin/tmux"):
        with patch("agentx.integration.terminal_bridge.subprocess.run", side_effect=fake):
            with patch("agentx.integration.terminal_bridge.time.sleep"):
                result = bridge.run_command("git commit -m 'wip'", context="commit", visible=True)

    assert result.executed_command == edited
    assert result.decision == "approved"
    sent_keys_args = [c for c in fake.calls if c[1] == "send-keys"]
    assert any(edited in c[4] for c in sent_keys_args)


