"""
ToolDispatcher — routes tool calls to the correct executor.

Extracted from AgentXSession to give tool routing a clear single
responsibility.  AgentXSession keeps a thin delegation stub so the existing
public API is preserved.
"""

import logging

logger = logging.getLogger(__name__)


class ToolDispatcher:
    """
    Routes ``execute_tool`` requests to the correct executor.

    - Client-side file-system tools → ``ClientToolExecutor``
    - Code-analysis tools → ``ServerToolExecutor`` (Agentix)
    - All others → ``ServerToolExecutor`` if available, else 'unknown'
    """

    def __init__(self, client_tool_executor, server_tool_executor) -> None:
        self._client = client_tool_executor
        self._server = server_tool_executor

    def execute_tool(self, tool_name: str, tool_input: dict) -> str:
        """
        Execute a tool (either client-side or server-side).

        Routes to appropriate executor based on tool type and availability:
        - CLIENT tools: Execute via ClientToolExecutor
        - SERVER tools: Execute via ServerToolExecutor
        - CODE_ANALYSIS: Execute via ServerToolExecutor (Agentix)
        - EITHER: Try client first, fall back to server

        Args:
            tool_name: Name of the tool to execute
            tool_input: Arguments for the tool

        Returns:
            Tool execution result as string
        """
        try:
            from .integration import CodeAnalysisTool

            # Client-side tool names
            client_tool_names = {"read_file", "list_directory", "write_file", "get_file_info", "search_files"}

            if tool_name in client_tool_names:
                return self._client.execute(tool_name, tool_input)

            if CodeAnalysisTool.is_code_analysis_tool(tool_name):
                if self._server.is_available():
                    return self._server.execute(tool_name, tool_input)
                return f"Code analysis tool '{tool_name}' not available - Agentix not connected"

            if self._server.is_available():
                return self._server.execute(tool_name, tool_input)

            return f"Unknown tool: {tool_name}"

        except Exception as e:
            logger.exception("Error executing tool '%s'", tool_name)
            return f"Error executing tool '{tool_name}': {str(e)}"
