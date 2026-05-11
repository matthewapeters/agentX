"""Tool registry for managing available tools in AgentX.

Provides dynamic loading of tools from agentx_tools.toml, enable/disable
toggling, and built-in tools for tool management (reload_tools, register_tool).
"""

import json
from pathlib import Path
from typing import Any, Optional
import toml


class ToolRegistry:
    """
    Manages available tools for AgentX.

    Loads tool definitions from agentx_tools.toml, maintains enabled/disabled state,
    and provides built-in tools for tool management.

    Example:
        registry = ToolRegistry(config_path="agentx_tools.toml")
        all_tools = registry.get_all_tools()
        enabled = registry.get_enabled_tools()
        registry.toggle_tool("cst", enabled=False)
        registry.register_tool("my_tool", {"description": "Custom tool"})
        registry.reload_tools()
    """

    def __init__(self, config_path: Optional[str] = None):
        """
        Initialize tool registry.

        Args:
            config_path: Path to agentx_tools.toml. If None, uses default location.
        """
        self.config_path = Path(config_path or "agentx_tools.toml")
        self.tools: dict[str, dict[str, Any]] = {}
        self.enabled_state: dict[str, bool] = {}
        self._load_from_config()

    def _load_from_config(self) -> None:
        """Load tool definitions from agentx_tools.toml.

        Raises:
            FileNotFoundError: If config file doesn't exist.
            ValueError: If config format is invalid.
        """
        if not self.config_path.exists():
            raise FileNotFoundError(f"Tool registry config not found: {self.config_path}")

        try:
            config = toml.load(self.config_path)
        except toml.TomlDecodeError as e:
            raise ValueError(f"Invalid TOML in {self.config_path}: {e}") from e

        tools_section = config.get("tools", {})
        if not isinstance(tools_section, dict):
            raise ValueError("[tools] section must be a table")

        self.tools.clear()
        self.enabled_state.clear()

        for tool_name, tool_def in tools_section.items():
            if not isinstance(tool_def, dict):
                raise ValueError(f"Tool {tool_name} must be a table")

            self.tools[tool_name] = {
                "name": tool_name,
                "description": tool_def.get("description", ""),
                "category": tool_def.get("category", "user"),
            }
            self.enabled_state[tool_name] = bool(tool_def.get("enabled", True))

    def get_all_tools(self) -> list[dict[str, Any]]:
        """Get all tools (enabled and disabled).

        Returns:
            List of tool definitions with name, description, category, and enabled status.
        """
        return [{**tool, "enabled": self.enabled_state.get(name, True)} for name, tool in self.tools.items()]

    def get_enabled_tools(self) -> list[dict[str, Any]]:
        """Get only enabled tools.

        Returns:
            List of enabled tool definitions.
        """
        return [{**tool, "enabled": True} for name, tool in self.tools.items() if self.enabled_state.get(name, True)]

    def get_enabled_tool_names(self) -> list[str]:
        """Get list of enabled tool names.

        Returns:
            List of tool names that are currently enabled.
        """
        return [name for name, enabled in self.enabled_state.items() if enabled]

    def toggle_tool(self, tool_name: str, enabled: bool) -> bool:
        """Enable or disable a tool.

        Args:
            tool_name: Name of the tool to toggle.
            enabled: Whether to enable (True) or disable (False).

        Returns:
            True if the tool state was changed, False if tool doesn't exist.
        """
        if tool_name not in self.tools:
            return False
        self.enabled_state[tool_name] = enabled
        return True

    def register_tool(
        self,
        tool_name: str,
        description: str = "",
        category: str = "user",
        enabled: bool = True,
    ) -> bool:
        """Register a new tool dynamically.

        Args:
            tool_name: Unique name for the tool.
            description: Human-readable description.
            category: Tool category (e.g., "user", "filesystem", "code-analysis").
            enabled: Whether the tool is enabled by default.

        Returns:
            True if the tool was registered, False if it already exists.
        """
        if tool_name in self.tools:
            return False

        self.tools[tool_name] = {
            "name": tool_name,
            "description": description,
            "category": category,
        }
        self.enabled_state[tool_name] = enabled
        return True

    def reload_tools(self) -> None:
        """Reload tool definitions from config file.

        Resets enabled/disabled state to config defaults.
        Any dynamically registered tools are lost.

        Raises:
            FileNotFoundError: If config file doesn't exist.
            ValueError: If config format is invalid.
        """
        self._load_from_config()

    def save_enabled_state(self, output_path: Optional[Path] = None) -> None:
        """Save current enabled/disabled state to a file.

        Useful for persisting user preferences across sessions.

        Args:
            output_path: Path to save state to. If None, uses registry config path.
        """
        output = output_path or self.config_path
        state = {"tools": {name: {"enabled": self.enabled_state.get(name, True)} for name in self.tools}}
        with open(output, "w") as f:
            toml.dump(state, f)

    def to_dict(self) -> dict[str, Any]:
        """Export registry state as a dictionary.

        Returns:
            Dictionary containing all tools and their enabled state.
        """
        return {
            "tools": self.get_all_tools(),
            "enabled_tools": self.get_enabled_tool_names(),
        }
