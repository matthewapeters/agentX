"""Unit tests for TerminalBridge permission policy decisions."""

from __future__ import annotations

import pytest

from agentx.integration.terminal_bridge import PermissionLayer


@pytest.mark.unit
def test_allow_list_command_is_allowed() -> None:
    """GIVEN allow prefixes include pytest WHEN command matches allow THEN verdict is allowed. [PD-15-AF-007]"""

    layer = PermissionLayer(allow=["python -m pytest"], confirm=[], deny=[])

    decision = layer.check_command("python -m pytest tests/")

    assert decision.verdict == "allowed"
    assert decision.list_name == "allow"


@pytest.mark.unit
def test_confirm_list_command_requires_approval_in_supervised() -> None:
    """GIVEN supervised mode and confirm prefixes WHEN command matches confirm THEN verdict requires approval. [PD-15-AF-006]"""

    layer = PermissionLayer(mode="supervised", allow=[], confirm=["git commit"], deny=[])

    decision = layer.check_command("git commit -m 'wip'")

    assert decision.verdict == "requires_approval"
    assert decision.list_name == "confirm"


@pytest.mark.unit
def test_confirm_list_command_is_allowed_in_autonomous() -> None:
    """GIVEN autonomous mode and confirm prefixes WHEN command matches confirm THEN verdict is allowed. [PD-15-AF-005]"""

    layer = PermissionLayer(mode="autonomous", allow=[], confirm=["git commit"], deny=[])

    decision = layer.check_command("git commit -m 'wip'")

    assert decision.verdict == "allowed"
    assert decision.list_name == "confirm"


@pytest.mark.unit
def test_deny_list_command_is_denied() -> None:
    """GIVEN deny prefixes include rm WHEN command matches deny THEN verdict is denied. [PD-15-AF-007]"""

    layer = PermissionLayer(allow=[], confirm=[], deny=["rm "])

    decision = layer.check_command("rm -rf build/")

    assert decision.verdict == "denied"
    assert decision.list_name == "deny"


@pytest.mark.unit
def test_unknown_command_defaults_to_requires_approval() -> None:
    """GIVEN no list match WHEN command is unknown THEN verdict defaults to requires approval. [PD-15-AF-006]"""

    layer = PermissionLayer(allow=[], confirm=[], deny=[])

    decision = layer.check_command("custom_tool --dry-run")

    assert decision.verdict == "requires_approval"
    assert decision.list_name == "default_confirm"


@pytest.mark.unit
def test_path_check_passes_for_in_bounds_absolute_path(tmp_path) -> None:
    """GIVEN project root path WHEN command references in-bounds absolute path THEN path check returns true. [PD-15-AF-007]"""

    root = tmp_path / "project"
    root.mkdir(parents=True, exist_ok=True)

    layer = PermissionLayer()
    result = layer.check_paths(f"cat {root}/file.py", [str(root)])

    assert result is True


@pytest.mark.unit
def test_path_check_blocks_out_of_bounds_absolute_path(tmp_path) -> None:
    """GIVEN project root path WHEN command references out-of-bounds absolute path THEN path check returns false. [PD-15-AF-007]"""

    root = tmp_path / "project"
    root.mkdir(parents=True, exist_ok=True)

    layer = PermissionLayer()
    result = layer.check_paths("cat /etc/passwd", [str(root)])

    assert result is False
