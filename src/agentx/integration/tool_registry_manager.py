"""Tool registry manager for AgentX integration.

Bridges the ToolRegistry with the agent bridge, providing built-in tools
for dynamic tool management and registering all available tools with the bridge.
"""

import json
from typing import Any, Callable, Optional

from src.agentx.tool_registry import ToolRegistry


class ToolRegistryManager:
    """
    Manages tool registry integration with the agent bridge.

    Owns a ToolRegistry instance, provides built-in tool implementations
    (reload_tools, register_tool), and exposes available tools for bridge
    registration.

    Example:
        manager = ToolRegistryManager(config_path="agentx_tools.toml")
        tools = manager.get_available_tools()
        impls = manager.get_builtin_tool_implementations()
        bridge.register_tool_implementations(impls, tools)
    """

    def __init__(
        self,
        config_path: Optional[str] = None,
        on_registry_change: Optional[Callable[[], None]] = None,
    ):
        """
        Initialize tool registry manager.

        Args:
            config_path: Path to agentx_tools.toml. If None, uses default location.
            on_registry_change: Callback invoked when registry state changes
                (tool toggled, registered, or reloaded). Used to update bridge
                and UI.
        """
        self.registry = ToolRegistry(config_path)
        self.on_registry_change = on_registry_change or (lambda: None)

    def get_available_tools(self) -> list[dict[str, Any]]:
        """Get all tools (both enabled and disabled).

        Returns:
            List of tool definitions with metadata.
        """
        return self.registry.get_all_tools()

    def get_enabled_tools(self) -> list[dict[str, Any]]:
        """Get only enabled tools.

        Returns:
            List of enabled tool definitions.
        """
        return self.registry.get_enabled_tools()

    def get_enabled_tool_names(self) -> list[str]:
        """Get list of enabled tool names for bridge API.

        Returns:
            List of enabled tool names.
        """
        return self.registry.get_enabled_tool_names()

    def toggle_tool(self, tool_name: str, enabled: bool) -> bool:
        """Toggle a tool's enabled state.

        Called by UI (ToolPanel) when user checks/unchecks a tool.

        Args:
            tool_name: Name of the tool to toggle.
            enabled: Whether to enable (True) or disable (False).

        Returns:
            True if the tool was toggled, False if it doesn't exist.
        """
        result = self.registry.toggle_tool(tool_name, enabled)
        if result:
            self.on_registry_change()
        return result

    # Built-in tool implementations
    # These are registered with the bridge so the agent can invoke them

    def builtin_reload_tools(self) -> str:
        """Built-in tool: reload tools from config.

        Reloads tool definitions from agentx_tools.toml, resetting enabled/disabled
        state to defaults.

        Returns:
            JSON result with loaded tools list.
        """
        self.registry.reload_tools()
        tools = self.registry.get_all_tools()
        self.on_registry_change()

        return json.dumps(
            {
                "status": "success",
                "message": "Tools reloaded from config",
                "tools_loaded": len(tools),
                "tools": tools,
            }
        )

    def builtin_register_tool(
        self,
        tool_name: str,
        description: str = "",
        category: str = "user",
        enabled: bool = True,
    ) -> str:
        """Built-in tool: register a new tool dynamically.

        Args:
            tool_name: Unique name for the tool.
            description: Human-readable description.
            category: Tool category.
            enabled: Whether the tool is enabled by default.

        Returns:
            JSON result with status and updated tool list.
        """
        if not tool_name:
            return json.dumps({"status": "error", "message": "tool_name is required"})

        success = self.registry.register_tool(
            tool_name, description=description, category=category, enabled=enabled
        )

        if not success:
            return json.dumps(
                {
                    "status": "error",
                    "message": f"Tool '{tool_name}' already exists",
                }
            )

        tools = self.registry.get_all_tools()
        self.on_registry_change()

        return json.dumps(
            {
                "status": "success",
                "message": f"Tool '{tool_name}' registered",
                "tool_registered": tool_name,
                "tools": tools,
            }
        )

    def get_builtin_tool_implementations(self) -> dict[str, Callable]:
        """Get built-in tool implementations for bridge registration.

        Returns:
            Dictionary mapping tool names to implementation functions.
        """
        return {
            "reload_tools": self.builtin_reload_tools,
            "register_tool": self.builtin_register_tool,
        }
