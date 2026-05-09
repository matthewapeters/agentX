"""Tests for Phase 1 TUI/GUI configuration defaults and validation."""

from __future__ import annotations

from pathlib import Path

import pytest

from agentx.config import ConfigurationError, load_config


def _write_config(path: Path, content: str) -> None:
    """Write TOML test content to a temporary config path.

    Args:
        path: Destination file path.
        content: TOML content to write.
    """
    path.write_text(content, encoding="utf-8")


def test_load_config_applies_gui_and_tui_defaults(tmp_path: Path) -> None:
    """GIVEN a config without enable_gui_chat/tui keys WHEN loaded THEN defaults are applied."""
    config_path = tmp_path / "agentx.toml"
    _write_config(
        config_path,
        """
[agentx]
ollama_host = "localhost:11434"
ollama_model = "gpt-oss:latest"

[agentix]
host = "localhost:8000"
""".strip(),
    )

    config = load_config(str(config_path))

    assert config["agentx"]["enable_gui_chat"] is True
    assert config["tui"]["enable"] is False
    assert config["tui"]["output_split_ratio"] == 0.70
    assert config["tui"]["write_timeout_sec"] == 0.1
    assert config["tui"]["show_thinking"] is False


def test_load_config_rejects_both_gui_and_tui_disabled(tmp_path: Path) -> None:
    """GIVEN GUI and TUI disabled WHEN loaded THEN ConfigurationError is raised."""
    config_path = tmp_path / "agentx.toml"
    _write_config(
        config_path,
        """
[agentx]
ollama_host = "localhost:11434"
ollama_model = "gpt-oss:latest"
enable_gui_chat = false

[tui]
enable = false
""".strip(),
    )

    with pytest.raises(ConfigurationError, match="enable_gui_chat=false requires tui.enable=true"):
        load_config(str(config_path))


def test_load_config_accepts_headless_mode_when_tui_enabled(tmp_path: Path) -> None:
    """GIVEN GUI disabled and TUI enabled WHEN loaded THEN config is valid."""
    config_path = tmp_path / "agentx.toml"
    _write_config(
        config_path,
        """
[agentx]
ollama_host = "localhost:11434"
ollama_model = "gpt-oss:latest"
enable_gui_chat = false

[tui]
enable = true
output_split_ratio = 0.6
""".strip(),
    )

    config = load_config(str(config_path))

    assert config["agentx"]["enable_gui_chat"] is False
    assert config["tui"]["enable"] is True
    assert config["tui"]["output_split_ratio"] == 0.6


def test_load_config_rejects_invalid_tui_split_ratio(tmp_path: Path) -> None:
    """GIVEN output_split_ratio outside (0,1) WHEN loaded THEN ConfigurationError is raised."""
    config_path = tmp_path / "agentx.toml"
    _write_config(
        config_path,
        """
[agentx]
ollama_host = "localhost:11434"
ollama_model = "gpt-oss:latest"

[tui]
enable = true
output_split_ratio = 1.0
""".strip(),
    )

    with pytest.raises(ConfigurationError, match="output_split_ratio"):
        load_config(str(config_path))


def test_load_config_applies_tui_env_overrides(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    """GIVEN AGENTX_TUI_* env vars WHEN config is loaded THEN env values override [tui]."""
    config_path = tmp_path / "agentx.toml"
    _write_config(
        config_path,
        """
[agentx]
ollama_host = "localhost:11434"
ollama_model = "gpt-oss:latest"

[tui]
enable = false
output_fifo = "/tmp/from-config.output"
input_fifo = "/tmp/from-config.input"
socket = "/tmp/from-config.sock"
""".strip(),
    )

    monkeypatch.setenv("AGENTX_TUI_ENABLE", "true")
    monkeypatch.setenv("AGENTX_TUI_OUTPUT_FIFO", "/tmp/from-env.output")
    monkeypatch.setenv("AGENTX_TUI_INPUT_FIFO", "/tmp/from-env.input")
    monkeypatch.setenv("AGENTX_TUI_SOCKET", "/tmp/from-env.sock")

    config = load_config(str(config_path))

    assert config["tui"]["enable"] is True
    assert config["tui"]["output_fifo"] == "/tmp/from-env.output"
    assert config["tui"]["input_fifo"] == "/tmp/from-env.input"
    assert config["tui"]["socket"] == "/tmp/from-env.sock"
