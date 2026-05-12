"""Built-in tools for managing the tool registry.

These tools are always available to the agent for discovering and managing
available tools dynamically.

Built-in tools:
- reload_tools(): Reload tool definitions from configuration
- register_tool(): Dynamically register a new tool definition
- diagnose_tools(): Run pipeline diagnostics for tool availability and execution
- open_file_in_editor(): Open a file in the running vibe editor
- diff_files_in_editor(): Open two files in side-by-side diff mode in vibe editor
- editor_action(): Run sandboxed editor-assist actions in vibe editor
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


def diagnose_tools() -> str:
    """Run diagnostics for the tool pipeline.

    Verifies that tools are discoverable in the registry, registered with the
    bridge, visible to the LLM, and executable end-to-end.

    Returns:
        JSON string with diagnostic report and identified issues.
    """
    return '{"status": "ok", "summary": "Tool diagnostics completed"}'


def open_file_in_editor(file_path: str, line: int | None = None) -> str:
    """Open a file in the running vibe editor.

    Args:
        file_path: Absolute or relative path to the file to open.
        line: Optional 1-based line number to focus after opening.

    Returns:
        JSON string with operation status.
    """
    return '{"status": "success", "message": "File opened in editor"}'


def diff_files_in_editor(left_file: str, right_file: str) -> str:
    """Open two files in a side-by-side diff view in vibe editor.

    Args:
        left_file: Left-hand file path.
        right_file: Right-hand file path.

    Returns:
        JSON string with operation status.
    """
    return '{"status": "success", "message": "Diff opened in editor"}'


def editor_action(action: str, file_path: str, line: int | None = None, payload: str = "") -> str:
    """Run an editor-assist action in vibe editor.

    Args:
        action: Action name (show_symbol_help, autocomplete_assist, propose_edit).
        file_path: Target file path.
        line: Optional 1-based line number.
        payload: Optional text payload for propose_edit.

    Returns:
        JSON string with operation status.
    """
    return '{"status": "success", "message": "Editor action executed"}'
