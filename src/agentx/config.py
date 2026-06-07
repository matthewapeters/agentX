import os
from importlib.resources import files
from typing import Any

import toml

DEFAULT_CONFIG = "agentx.toml"
DEFAULT_TOOL_REGISTRY_SEARCH_PATHS = [
    "./agentx_tools.toml",
    "~/.agentx/agentx_tools.toml",
]
DEFAULT_CONTEXT_HISTORY_SESSION_SORT = "Ascending"


def normalize_context_history_session_sort(value: Any) -> str:
    """Normalize the presentation-only context-history session sort order."""
    if isinstance(value, str):
        normalized = value.strip().lower()
        if normalized in {"ascending", "asc"}:
            return "Ascending"
        if normalized in {"descending", "desc"}:
            return "Descending"
    raise ConfigurationError(
        f"Invalid value for [agentx].context_history_session_sort: {value!r} (expected Ascending or Descending)"
    )


class ConfigurationError(ValueError):
    """Raised when AgentX configuration is structurally invalid."""


def _parse_bool_env(var_name: str, value: str) -> bool:
    """Parse a boolean environment variable value.

    Args:
        var_name: Environment variable name.
        value: Raw environment variable value.

    Returns:
        Parsed boolean value.

    Raises:
        ConfigurationError: If the value is not a recognized boolean literal.
    """
    normalized = value.strip().lower()
    if normalized in {"1", "true", "yes", "on"}:
        return True
    if normalized in {"0", "false", "no", "off"}:
        return False
    raise ConfigurationError(f"Invalid boolean value for {var_name}: {value!r}")


def apply_config_defaults(config: dict[str, Any]) -> dict[str, Any]:
    """Apply in-memory defaults for optional AgentX configuration keys.

    Args:
        config: Parsed configuration dictionary.

    Returns:
        The same dictionary instance with missing optional keys populated.
    """
    agentx = config.setdefault("agentx", {})
    if not isinstance(agentx, dict):
        raise ConfigurationError("[agentx] section must be a table")

    agentx.setdefault("enable_gui_chat", True)
    agentx["context_history_session_sort"] = normalize_context_history_session_sort(
        agentx.get("context_history_session_sort", DEFAULT_CONTEXT_HISTORY_SESSION_SORT)
    )

    tui = config.setdefault("tui", {})
    if not isinstance(tui, dict):
        raise ConfigurationError("[tui] section must be a table")

    tui.setdefault("enable", False)
    tui.setdefault("socket", "")
    tui.setdefault("output_fifo", "")
    tui.setdefault("input_fifo", "")
    tui.setdefault("output_split_ratio", 0.70)
    tui.setdefault("write_timeout_sec", 0.1)
    tui.setdefault("show_thinking", False)

    tool_registry = config.setdefault("tool_registry", {})
    if not isinstance(tool_registry, dict):
        raise ConfigurationError("[tool_registry] section must be a table")
    tool_registry.setdefault("search_paths", list(DEFAULT_TOOL_REGISTRY_SEARCH_PATHS))

    # Environment variable overrides for TUI runtime control.
    env_enable = os.getenv("AGENTX_TUI_ENABLE")
    if env_enable is not None:
        tui["enable"] = _parse_bool_env("AGENTX_TUI_ENABLE", env_enable)

    env_output_fifo = os.getenv("AGENTX_TUI_OUTPUT_FIFO")
    if env_output_fifo:
        tui["output_fifo"] = env_output_fifo

    env_input_fifo = os.getenv("AGENTX_TUI_INPUT_FIFO")
    if env_input_fifo:
        tui["input_fifo"] = env_input_fifo

    env_socket = os.getenv("AGENTX_TUI_SOCKET")
    if env_socket:
        tui["socket"] = env_socket

    return config


def validate_config(config: dict[str, Any]) -> None:
    """Validate cross-field configuration constraints.

    Args:
        config: Parsed and defaulted configuration dictionary.

    Raises:
        ConfigurationError: If a required constraint is violated.
    """
    agentx = config.get("agentx", {})
    tui = config.get("tui", {})
    tool_registry = config.get("tool_registry", {})

    normalize_context_history_session_sort(agentx.get("context_history_session_sort", "Ascending"))

    enable_gui_chat = bool(agentx.get("enable_gui_chat", True))
    enable_tui = bool(tui.get("enable", False))
    output_split_ratio = float(tui.get("output_split_ratio", 0.70))

    if not 0.0 < output_split_ratio < 1.0:
        raise ConfigurationError("[tui].output_split_ratio must be between 0 and 1")

    if not enable_gui_chat and not enable_tui:
        raise ConfigurationError("Invalid config: enable_gui_chat=false requires tui.enable=true")

    search_paths = tool_registry.get("search_paths", [])
    if (
        not isinstance(search_paths, list)
        or not search_paths
        or any(not isinstance(path, str) or not path.strip() for path in search_paths)
    ):
        raise ConfigurationError("[tool_registry].search_paths must be a non-empty list of non-empty strings")


def load_config(config_path=DEFAULT_CONFIG):
    with open(config_path, "r") as f:
        config = toml.loads(f.read())

    apply_config_defaults(config)
    validate_config(config)

    return config


def save_config(config, config_path=DEFAULT_CONFIG):
    with open(config_path, "w") as f:
        f.write(toml.dumps(config))


def get_icon_path(icon_name):
    """
    Retrieve the full path to an SVG icon by name.

    Args:
        icon_name (str): The name of the icon file (e.g., 'smiley.svg').

    Returns:
        str: The full path to the icon file.
    """
    base_path = files("agentx").joinpath("assets/icons/opemmoji-svg-color")
    return os.path.join(base_path, icon_name)
