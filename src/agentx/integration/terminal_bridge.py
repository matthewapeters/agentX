"""tmux-backed terminal execution bridge for AgentX.

This module provides a minimal, testable scaffold for tmux command dispatch with
permission checks and audit logging. It is intentionally synchronous and
conservative: command execution defaults to supervised mode and requires explicit
approval for confirm-list commands.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass
from datetime import UTC, datetime
import json
from pathlib import Path
import shlex
import shutil
import subprocess
import time
from typing import Callable, Mapping
import uuid

_CAPTURE_SENTINEL_PREFIX = "__AGENTX_DONE__"
_DEFAULT_POLL_INTERVAL = 0.5  # seconds between capture-pane polls

DEFAULT_ALLOW_PREFIXES: list[str] = [
    "pwd",
    "ls",
    "git status",
    "git diff",
    "python -m pytest",
]

DEFAULT_CONFIRM_PREFIXES: list[str] = [
    "git commit",
    "git push",
    "pip install",
    "uv add",
    "uv pip install",
]

DEFAULT_DENY_PREFIXES: list[str] = [
    "rm ",
    "sudo ",
    "chmod 777",
    "mkfs",
]


@dataclass(slots=True)
class PermissionDecision:
    """Represents the permission verdict for a terminal command.

    Args:
        verdict: One of "allowed", "requires_approval", or "denied".
        reason: Human-readable explanation for the decision.
        list_name: The matching source list name.
    """

    verdict: str
    reason: str
    list_name: str


@dataclass(slots=True)
class TerminalResult:
    """Represents the terminal execution result payload.

    Args:
        pane_id: tmux pane id target used for dispatch.
        exit_code: Process exit status. Uses -1 for denied/rejected/timeouts.
        stdout: Human-readable result message or captured output.
        timed_out: True when command exceeded timeout.
        decision: Decision label for auditing and UI rendering.
        original_command: Command received from caller.
        executed_command: Command actually dispatched after edits/approval.
    """

    pane_id: str
    exit_code: int
    stdout: str
    timed_out: bool
    decision: str
    original_command: str
    executed_command: str


class PermissionLayer:
    """Classifies terminal commands against allow/confirm/deny policy.

    The first matching list wins in this order: deny -> allow -> confirm.
    Unknown commands default to requires_approval.
    """

    def __init__(
        self,
        mode: str = "supervised",
        allow: list[str] | None = None,
        confirm: list[str] | None = None,
        deny: list[str] | None = None,
    ) -> None:
        """Initialize command permission lists and execution mode.

        Args:
            mode: "supervised" or "autonomous".
            allow: Allowed command prefixes.
            confirm: Confirm-required command prefixes.
            deny: Denied command prefixes.
        """

        self._mode = mode
        self._allow = allow[:] if allow is not None else DEFAULT_ALLOW_PREFIXES[:]
        self._confirm = confirm[:] if confirm is not None else DEFAULT_CONFIRM_PREFIXES[:]
        self._deny = deny[:] if deny is not None else DEFAULT_DENY_PREFIXES[:]

    @property
    def mode(self) -> str:
        """Return the current execution mode."""

        return self._mode

    def set_mode(self, mode: str) -> None:
        """Update execution mode.

        Args:
            mode: Expected values are "supervised" or "autonomous".
        """

        if mode not in {"supervised", "autonomous"}:
            raise ValueError(f"Unsupported execution mode: {mode}")
        self._mode = mode

    def reload_from_config(self, config: Mapping[str, object]) -> None:
        """Reload permission settings from config mapping.

        Args:
            config: Configuration dictionary with optional `terminal` section.
        """

        terminal = config.get("terminal") if isinstance(config, Mapping) else None
        if not isinstance(terminal, Mapping):
            return

        allow = terminal.get("allow")
        confirm = terminal.get("confirm")
        deny = terminal.get("deny")
        mode = terminal.get("exec_mode")

        if isinstance(allow, list):
            self._allow = [str(v) for v in allow]
        if isinstance(confirm, list):
            self._confirm = [str(v) for v in confirm]
        if isinstance(deny, list):
            self._deny = [str(v) for v in deny]
        if isinstance(mode, str):
            self.set_mode(mode)

    def check_command(self, command: str) -> PermissionDecision:
        """Classify a command by list membership and mode.

        Args:
            command: Shell command string.

        Returns:
            PermissionDecision describing verdict and reason.
        """

        normalized = command.strip()

        for prefix in self._deny:
            if normalized.startswith(prefix):
                return PermissionDecision(
                    verdict="denied",
                    reason=f"Command matches deny-list prefix '{prefix}'.",
                    list_name="deny",
                )

        for prefix in self._allow:
            if normalized.startswith(prefix):
                return PermissionDecision(
                    verdict="allowed",
                    reason=f"Command matches allow-list prefix '{prefix}'.",
                    list_name="allow",
                )

        for prefix in self._confirm:
            if normalized.startswith(prefix):
                if self._mode == "autonomous":
                    return PermissionDecision(
                        verdict="allowed",
                        reason=f"Autonomous mode allows confirm-list prefix '{prefix}'.",
                        list_name="confirm",
                    )
                return PermissionDecision(
                    verdict="requires_approval",
                    reason=f"Command matches confirm-list prefix '{prefix}'.",
                    list_name="confirm",
                )

        return PermissionDecision(
            verdict="requires_approval",
            reason="No list matched; defaulting to supervised confirmation.",
            list_name="default_confirm",
        )

    def check_paths(self, command: str, project_roots: list[str]) -> bool:
        """Validate absolute file paths in command stay within project roots.

        Args:
            command: Shell command string.
            project_roots: Allowed project root directories.

        Returns:
            True when all parsed absolute paths are in-bounds, otherwise False.
        """

        roots = [Path(root).resolve() for root in project_roots]
        if not roots:
            return True

        try:
            tokens = shlex.split(command)
        except ValueError:
            return False

        for token in tokens:
            if not token.startswith("/"):
                continue
            candidate = Path(token).resolve()
            if any(_is_within(candidate, root) for root in roots):
                continue
            return False

        return True


class TerminalBridge:
    """Dispatches shell commands into tmux panes with policy enforcement."""

    def __init__(
        self,
        config: Mapping[str, object],
        session_id: str,
        project_roots: list[str] | None = None,
        approval_callback: Callable[[str, str], tuple[bool, str | None]] | None = None,
        audit_log_path: str | None = None,
    ) -> None:
        """Initialize terminal bridge state.

        Args:
            config: Agent configuration mapping.
            session_id: tmux session id.
            project_roots: Allowed roots for path checks.
            approval_callback: Optional callback for approval flow.
            audit_log_path: Optional explicit audit log path.
        """

        self._config = config
        self._session_id = session_id
        self._project_roots = project_roots[:] if project_roots is not None else [str(Path.cwd())]
        self._approval_callback = approval_callback

        self._permission_layer = PermissionLayer()
        self._permission_layer.reload_from_config(config)

        self._terminal_config = config.get("terminal") if isinstance(config, Mapping) else None
        if not isinstance(self._terminal_config, Mapping):
            self._terminal_config = {}

        self._audit_log_path = (
            Path(audit_log_path)
            if audit_log_path is not None
            else Path(self._terminal_config.get("audit_log_path", "sessions/_logs/terminal_audit.jsonl"))
        )

    @property
    def permission_layer(self) -> PermissionLayer:
        """Return the permission layer instance."""

        return self._permission_layer

    def set_exec_mode(self, mode: str) -> None:
        """Set execution mode on permission layer.

        Args:
            mode: "supervised" or "autonomous".
        """

        self._permission_layer.set_mode(mode)

    def get_exec_mode(self) -> str:
        """Get current execution mode value."""

        return self._permission_layer.mode

    def set_approval_callback(
        self,
        callback: Callable[[str, str], tuple[bool, str | None]] | None,
    ) -> None:
        """Set callback used for supervised approval decisions."""

        self._approval_callback = callback

    def is_tmux_available(self) -> bool:
        """Return True when tmux binary is available in PATH."""

        return shutil.which("tmux") is not None

    def is_session_active(self) -> bool:
        """Return True when target tmux session exists."""

        if not self.is_tmux_available():
            return False

        result = subprocess.run(
            ["tmux", "has-session", "-t", self._session_id],
            capture_output=True,
            text=True,
            check=False,
        )
        return result.returncode == 0

    def run_command(
        self,
        command: str,
        context: str = "",
        visible: bool = True,
        auto_close: bool = True,
        timeout_sec: int = 60,
    ) -> TerminalResult:
        """Execute command in tmux according to permission policy.

        Args:
            command: Shell command to run.
            context: Human-readable reason for approval dialog.
            visible: If True, run in ephemeral pane; else in persistent pane.
            auto_close: Whether to auto-close ephemeral pane after execution.
            timeout_sec: Maximum execution timeout in seconds.

        Returns:
            TerminalResult with dispatch metadata and decision outcome.
        """

        original_command = command
        decision = self._permission_layer.check_command(command)

        if not self._permission_layer.check_paths(command, self._project_roots):
            result = TerminalResult(
                pane_id="",
                exit_code=-1,
                stdout="DENIED: command path is outside allowed project roots.",
                timed_out=False,
                decision="path_violation",
                original_command=original_command,
                executed_command=command,
            )
            self._append_audit(result)
            return result

        if decision.verdict == "denied":
            result = TerminalResult(
                pane_id="",
                exit_code=-1,
                stdout=f"DENIED: {decision.reason}",
                timed_out=False,
                decision="denied",
                original_command=original_command,
                executed_command=command,
            )
            self._append_audit(result)
            return result

        if decision.verdict == "requires_approval":
            approved = False
            edited_command = command
            if self._approval_callback is not None:
                approved, maybe_edited = self._approval_callback(command, context)
                if approved and maybe_edited:
                    edited_command = maybe_edited
            if not approved:
                result = TerminalResult(
                    pane_id="",
                    exit_code=-1,
                    stdout="REJECTED: command requires approval in supervised mode.",
                    timed_out=False,
                    decision="rejected",
                    original_command=original_command,
                    executed_command=command,
                )
                self._append_audit(result)
                return result
            command = edited_command

        if not self.is_tmux_available():
            result = TerminalResult(
                pane_id="",
                exit_code=-1,
                stdout="tmux is not available.",
                timed_out=False,
                decision="denied",
                original_command=original_command,
                executed_command=command,
            )
            self._append_audit(result)
            return result

        if not self.is_session_active():
            result = TerminalResult(
                pane_id="",
                exit_code=-1,
                stdout=f"tmux session '{self._session_id}' is not active.",
                timed_out=False,
                decision="denied",
                original_command=original_command,
                executed_command=command,
            )
            self._append_audit(result)
            return result

        pane_target = self._create_visible_pane() if visible else f"{self._session_id}:1.0"

        # Generate a unique sentinel for this invocation so concurrent or
        # back-to-back commands on the persistent pane cannot cross-match.
        run_id = uuid.uuid4().hex
        sentinel = f"{_CAPTURE_SENTINEL_PREFIX}{run_id}__"
        sentinel_cmd = f"{command}; echo {sentinel}$?"
        self._tmux_send_keys(pane_target, sentinel_cmd)

        timed_out, exit_code, stdout_text = self._wait_for_completion(
            pane_target, timeout_sec, sentinel=sentinel, visible=visible
        )

        if visible and auto_close and not timed_out:
            try:
                self._run_tmux(["kill-pane", "-t", pane_target])
            except RuntimeError:
                pass  # pane already closed

        result = TerminalResult(
            pane_id=pane_target,
            exit_code=exit_code,
            stdout=stdout_text,
            timed_out=timed_out,
            decision="approved" if decision.verdict == "requires_approval" else "allowed",
            original_command=original_command,
            executed_command=command,
        )
        self._append_audit(result)
        return result

    def _wait_for_completion(
        self,
        pane_target: str,
        timeout_sec: int,
        sentinel: str,
        visible: bool = True,
    ) -> tuple[bool, int, str]:
        """Poll ``capture-pane`` until the unique sentinel line appears or timeout.

        Each invocation of ``run_command()`` generates a distinct sentinel of the
        form ``__AGENTX_DONE__<uuid>__<exit_code>`` so back-to-back or concurrent
        commands on the same persistent pane cannot cross-match each other's output.

        On timeout, visible panes are killed (``kill-pane``); persistent panes
        receive ``Ctrl+C`` to preserve the shell for subsequent commands.

        Args:
            pane_target: tmux pane target string.
            timeout_sec: Maximum wait in seconds.
            sentinel: Unique per-invocation sentinel prefix string to match.
            visible: True for ephemeral panes; False for persistent pane (1.0).

        Returns:
            Tuple of (timed_out, exit_code, stdout_text).
        """

        deadline = time.monotonic() + timeout_sec

        while time.monotonic() < deadline:
            time.sleep(_DEFAULT_POLL_INTERVAL)
            try:
                output = self._run_tmux(["capture-pane", "-p", "-t", pane_target])
            except RuntimeError:
                # Pane closed before we could capture; treat as completed without exit code.
                return False, -1, "(pane closed before output captured)"

            for line in output.splitlines():
                if sentinel in line:
                    try:
                        exit_code = int(line.split(sentinel)[-1].strip())
                    except (ValueError, IndexError):
                        exit_code = 0
                    clean = "\n".join(ln for ln in output.splitlines() if sentinel not in ln)
                    return False, exit_code, clean.strip()

        # Timeout reached.
        if visible:
            try:
                self._run_tmux(["kill-pane", "-t", pane_target])
            except RuntimeError:
                pass
        else:
            # Interrupt command without destroying the persistent shell pane.
            try:
                self._run_tmux(["send-keys", "-t", pane_target, "C-c"])
            except RuntimeError:
                pass

        return True, -1, f"TIMEOUT after {timeout_sec}s"

    def kill_pane(self, pane_id: str) -> None:
        """Kill a tmux pane by id.

        Args:
            pane_id: tmux pane id or pane target string.
        """

        self._run_tmux(["kill-pane", "-t", pane_id])

    def list_active_panes(self) -> list[str]:
        """List pane ids in target tmux session.

        Returns:
            List of pane ids.
        """

        output = self._run_tmux(["list-panes", "-t", self._session_id, "-F", "#{pane_id}"])
        return [line.strip() for line in output.splitlines() if line.strip()]

    def _create_visible_pane(self) -> str:
        """Create an ephemeral pane in window 1 and return pane id."""

        return self._run_tmux(
            [
                "new-window",
                "-P",
                "-F",
                "#{pane_id}",
                "-t",
                f"{self._session_id}:1",
                "-d",
            ]
        ).strip()

    def _tmux_send_keys(self, pane_target: str, command: str) -> None:
        """Send shell command to tmux pane target.

        Args:
            pane_target: Pane target string.
            command: Shell command text.
        """

        self._run_tmux(["send-keys", "-t", pane_target, command, "Enter"])

    def _run_tmux(self, args: list[str]) -> str:
        """Run a tmux subprocess command and return stdout.

        Args:
            args: tmux argument list excluding executable.

        Returns:
            stdout text.

        Raises:
            RuntimeError: When tmux exits non-zero.
        """

        completed = subprocess.run(
            ["tmux", *args],
            capture_output=True,
            text=True,
            check=False,
        )
        if completed.returncode != 0:
            stderr = completed.stderr.strip() or "tmux command failed"
            raise RuntimeError(stderr)
        return completed.stdout

    def _append_audit(self, result: TerminalResult) -> None:
        """Append terminal decision/execution record to audit log.

        Args:
            result: TerminalResult to serialize.
        """

        payload = {
            "timestamp": datetime.now(UTC).isoformat(),
            "session_id": self._session_id,
            **asdict(result),
        }

        self._audit_log_path.parent.mkdir(parents=True, exist_ok=True)
        with self._audit_log_path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(payload, ensure_ascii=True) + "\n")


def _is_within(candidate: Path, root: Path) -> bool:
    """Return True when candidate path is within root path.

    Args:
        candidate: Candidate resolved path.
        root: Root resolved path.
    """

    try:
        candidate.relative_to(root)
        return True
    except ValueError:
        return False


# ---------------------------------------------------------------------------
# Tool wrappers for Agentix tool-loop registration
# ---------------------------------------------------------------------------

_terminal_bridge: "TerminalBridge | None" = None


def configure_terminal_bridge(
    config: Mapping[str, object],
    session_id: str,
    project_roots: list[str] | None = None,
    audit_log_path: str | None = None,
    approval_callback: Callable[[str, str], tuple[bool, str | None]] | None = None,
) -> TerminalBridge:
    """Configure and cache a singleton TerminalBridge for tool wrappers.

    Args:
        config: Agent configuration mapping.
        session_id: tmux session id.
        project_roots: Allowed project root paths for path checks.
        audit_log_path: Optional explicit JSONL audit log path.

    Returns:
        Configured TerminalBridge singleton.
    """

    global _terminal_bridge
    _terminal_bridge = TerminalBridge(
        config=config,
        session_id=session_id,
        project_roots=project_roots,
        audit_log_path=audit_log_path,
        approval_callback=approval_callback,
    )
    return _terminal_bridge


def _get_terminal_bridge() -> TerminalBridge:
    """Return configured TerminalBridge singleton.

    Raises:
        RuntimeError: If bridge has not been configured.
    """

    if _terminal_bridge is None:
        raise RuntimeError("TerminalBridge is not configured. Call configure_terminal_bridge(...) first.")
    return _terminal_bridge


def terminal_run(
    command: str,
    context: str = "",
    visible: bool | None = None,
    auto_close: bool | None = None,
    timeout_sec: int | None = None,
) -> str:
    """Run a shell command in tmux through the terminal permission layer.

    Args:
        command: Shell command to execute.
        context: Human-readable reason string for approvals.
        visible: Run in an ephemeral visible pane when True.
        auto_close: Close ephemeral pane after command exits when True.
        timeout_sec: Maximum timeout in seconds.

    Returns:
        JSON string with ``TerminalResult`` fields.
    """

    bridge = _get_terminal_bridge()
    terminal_cfg = bridge._config.get("terminal") if isinstance(bridge._config, Mapping) else None
    if not isinstance(terminal_cfg, Mapping):
        terminal_cfg = {}

    resolved_visible = visible if visible is not None else bool(terminal_cfg.get("terminal_visible", True))
    resolved_auto_close = auto_close if auto_close is not None else bool(terminal_cfg.get("terminal_auto_close", True))
    resolved_timeout = timeout_sec
    if resolved_timeout is None:
        try:
            resolved_timeout = int(terminal_cfg.get("terminal_timeout_sec", 60))
        except (TypeError, ValueError):
            resolved_timeout = 60

    result = bridge.run_command(
        command=command,
        context=context,
        visible=resolved_visible,
        auto_close=resolved_auto_close,
        timeout_sec=resolved_timeout,
    )
    return json.dumps(asdict(result), ensure_ascii=True)


def terminal_kill_pane(pane_id: str) -> str:
    """Kill a tmux pane by id.

    Args:
        pane_id: Pane id or tmux target string.

    Returns:
        Success message.
    """

    _get_terminal_bridge().kill_pane(pane_id)
    return f"Killed pane: {pane_id}"


def terminal_list_active_panes() -> str:
    """List active pane ids in the configured tmux session.

    Returns:
        JSON array string of pane ids.
    """

    panes = _get_terminal_bridge().list_active_panes()
    return json.dumps(panes, ensure_ascii=True)


TERMINAL_TOOL_FUNCTIONS = {
    "terminal_run": terminal_run,
    "terminal_kill_pane": terminal_kill_pane,
    "terminal_list_active_panes": terminal_list_active_panes,
}


def get_terminal_tool_implementations() -> dict[str, Callable[..., str]]:
    """Return terminal tool name-to-callable mapping for Agentix bridge."""

    return dict(TERMINAL_TOOL_FUNCTIONS)


def get_terminal_tool_schemas() -> list[dict[str, object]]:
    """Return OpenAI-function schemas for terminal tool wrappers."""

    from agentix.tools.schema import extract_tool_schema, SchemaGenerationError

    schemas: list[dict[str, object]] = []
    for fn in TERMINAL_TOOL_FUNCTIONS.values():
        try:
            schemas.append(extract_tool_schema(fn))
        except SchemaGenerationError:
            pass
    return schemas


def set_terminal_exec_mode(mode: str) -> bool:
    """Set execution mode on configured bridge.

    Args:
        mode: ``supervised`` or ``autonomous``.

    Returns:
        True when a bridge is configured and updated, else False.
    """

    if _terminal_bridge is None:
        return False
    _terminal_bridge.set_exec_mode(mode)
    return True


def get_terminal_exec_mode(default: str = "supervised") -> str:
    """Get current execution mode from configured bridge.

    Args:
        default: Value returned when no bridge is configured.
    """

    if _terminal_bridge is None:
        return default
    return _terminal_bridge.get_exec_mode()


def set_terminal_approval_callback(callback: Callable[[str, str], tuple[bool, str | None]]) -> bool:
    """Attach approval callback to configured bridge.

    Args:
        callback: Approval callback receiving ``(command, context)``.

    Returns:
        True when callback is attached, else False.
    """

    if _terminal_bridge is None:
        return False
    _terminal_bridge.set_approval_callback(callback)
    return True


def reload_terminal_config(config: Mapping[str, object]) -> bool:
    """Reload terminal permission configuration on the configured bridge.

    Args:
        config: Application config mapping containing ``terminal`` settings.

    Returns:
        True when bridge exists and reload occurs, else False.
    """

    if _terminal_bridge is None:
        return False
    _terminal_bridge._config = config
    _terminal_bridge._permission_layer.reload_from_config(config)
    return True
