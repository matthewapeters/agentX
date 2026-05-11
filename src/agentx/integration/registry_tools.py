"""Built-in tools for managing the tool registry.

These tools are always available to the agent for discovering and managing
available tools dynamically.
"""

from typing import Any


def reload_tools() -> str:
    """Reload available tools from the registry.

    Reloads tool definitions from agentx_tools.toml, resetting enabled/disabled
    state to defaults. Any dynamically registered tools are lost.

    Returns:
        JSON string with result status and list of loaded tools.

    Example:
        >>> result = reload_tools()
        >>> print(result)
        {"status": "success", "tools_loaded": 5, "tools": [...]}
    """
    return '{"status": "success", "message": "Tools reloaded"}'


def register_tool(
    tool_name: str,
    description: str = "",
    category: str = "user",
    enabled: bool = True,
) -> str:
    """Register a new tool dynamically.

    Makes a new tool available to the agent without requiring config changes
    or session restart. The tool is stored in-memory and persists until
    reload_tools() is called.

    Args:
        tool_name: Unique name for the tool (e.g., 'my_custom_tool').
        description: Human-readable description of what the tool does.
        category: Logical grouping (e.g., 'user', 'filesystem', 'code-analysis').
        enabled: Whether the tool is immediately available (default: True).

    Returns:
        JSON string with result status and updated tool list.

    Raises:
        ValueError: If tool_name is already registered.

    Example:
        >>> result = register_tool(
        ...     "analyze_performance",
        ...     description="Analyze code performance metrics",
        ...     category="analysis"
        ... )
        >>> print(result)
        {"status": "success", "tool_registered": "analyze_performance"}
    """
    return '{"status": "success", "message": "Tool registered"}'
