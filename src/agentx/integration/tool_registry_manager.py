"""Tool registry manager for AgentX integration.

Bridges the ToolRegistry with the agent bridge, providing built-in tools
for dynamic tool management and registering all available tools with the bridge.
"""

import json
from typing import Any, Callable, Optional

from src.agentx.integration.vim_bridge import VimBridge
from src.agentx.tool_diagnostics import create_tool_diagnostics
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
        bridge: Optional[Any] = None,
        config: Optional[dict[str, Any]] = None,
    ):
        """
        Initialize tool registry manager.

        Args:
            config_path: Path to agentx_tools.toml. If None, uses default location.
            on_registry_change: Callback invoked when registry state changes
                (tool toggled, registered, or reloaded). Used to update bridge
                and UI.
            bridge: Optional AgentixBridge instance used by builtin_diagnose_tools.
            config: Optional AgentX runtime config used by editor bridge tools.
        """
        self.registry = ToolRegistry(config_path)
        self.on_registry_change = on_registry_change or (lambda: None)
        self.bridge = bridge
        self.config = config or {}

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
        scope: str = "project",
        source_path: str = "",
        runtime: str = "python",
        entrypoint: str = "",
        input_schema: Optional[dict[str, Any]] = None,
        output_schema: Optional[dict[str, Any]] = None,
    ) -> str:
        """Built-in tool: register a new tool dynamically.

        Args:
            tool_name: Unique name for the tool.
            description: Human-readable description.
            category: Tool category.
            enabled: Whether the tool is enabled by default.
            scope: Tool scope (universal, user, project, session).
            source_path: Path to the tool implementation file.
            runtime: Runtime used to execute the tool implementation.
            entrypoint: Callable/function entrypoint.
            input_schema: JSON-schema-like tool input schema.
            output_schema: JSON-schema-like tool output schema.

        Returns:
            JSON result with status and updated tool list.
        """
        if not tool_name:
            return json.dumps({"status": "error", "message": "tool_name is required"})

        success = self.registry.register_tool(
            tool_name,
            description=description,
            category=category,
            enabled=enabled,
            scope=scope,
            source_path=source_path or None,
            runtime=runtime,
            entrypoint=entrypoint or None,
            input_schema=input_schema,
            output_schema=output_schema,
            persist=True,
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

    def builtin_diagnose_tools(self) -> str:
        """Built-in tool: diagnose tool pipeline health.

        Runs a full diagnostic suite that verifies registry state, bridge
        registration, LLM tool visibility, and end-to-end tool execution.

        Returns:
            JSON diagnostic report.
        """
        if self.bridge is None:
            return json.dumps(
                {
                    "status": "error",
                    "message": "Bridge is not configured for diagnostics",
                }
            )

        diagnostics = create_tool_diagnostics(self.bridge, self)
        report = diagnostics.run_full_diagnostic()
        return json.dumps(report)

    def builtin_open_file_in_editor(self, file_path: str, line: Optional[int] = None) -> str:
        """Built-in tool: open a file in the running vibe editor.

        Args:
            file_path: Absolute or relative path to the file to open.
            line: Optional 1-based line number for cursor placement.

        Returns:
            JSON result indicating success or failure.
        """
        if not file_path or not str(file_path).strip():
            return json.dumps({"status": "error", "message": "file_path is required"})

        bridge = VimBridge(config=self.config)
        success = bridge.open_file_from_context(file_path, line=line)
        if not success:
            return json.dumps(
                {
                    "status": "error",
                    "message": "Could not open file in vibe editor",
                    "file_path": file_path,
                    "line": line,
                }
            )

        return json.dumps(
            {
                "status": "success",
                "message": "Opened file in vibe editor",
                "file_path": file_path,
                "line": line,
            }
        )

    def builtin_diff_files_in_editor(self, left_file: str, right_file: str) -> str:
        """Built-in tool: open a side-by-side diff in the running vibe editor.

        Args:
            left_file: Left-hand file path, absolute or relative.
            right_file: Right-hand file path, absolute or relative.

        Returns:
            JSON result indicating success or failure.
        """
        if not left_file or not str(left_file).strip():
            return json.dumps({"status": "error", "message": "left_file is required"})
        if not right_file or not str(right_file).strip():
            return json.dumps({"status": "error", "message": "right_file is required"})

        bridge = VimBridge(config=self.config)
        success = bridge.diff_files_from_context(left_file, right_file)
        if not success:
            return json.dumps(
                {
                    "status": "error",
                    "message": "Could not diff files in vibe editor",
                    "left_file": left_file,
                    "right_file": right_file,
                }
            )

        return json.dumps(
            {
                "status": "success",
                "message": "Opened diff in vibe editor",
                "left_file": left_file,
                "right_file": right_file,
            }
        )

    def builtin_editor_action(
        self,
        action: str,
        file_path: str,
        line: Optional[int] = None,
        payload: str = "",
    ) -> str:
        """Built-in tool: run a sandboxed editor-assist action.

        Args:
            action: Action name (show_symbol_help, autocomplete_assist, propose_edit).
            file_path: Target file path.
            line: Optional 1-based line number.
            payload: Optional text payload (used by propose_edit).

        Returns:
            JSON result indicating success or failure.
        """
        if not action or not str(action).strip():
            return json.dumps({"status": "error", "message": "action is required"})
        if not file_path or not str(file_path).strip():
            return json.dumps({"status": "error", "message": "file_path is required"})

        bridge = VimBridge(config=self.config)
        result = bridge.editor_action(action=action, file_path=file_path, line=line, payload=payload)
        return json.dumps(result)

    def get_builtin_tool_implementations(self) -> dict[str, Callable]:
        """Get built-in tool implementations for bridge registration.

        Returns:
            Dictionary mapping tool names to implementation functions.
        """
        return {
            "reload_tools": self.builtin_reload_tools,
            "register_tool": self.builtin_register_tool,
            "diagnose_tools": self.builtin_diagnose_tools,
            "open_file_in_editor": self.builtin_open_file_in_editor,
            "diff_files_in_editor": self.builtin_diff_files_in_editor,
            "editor_action": self.builtin_editor_action,
        }
