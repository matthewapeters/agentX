"""
Server-side tool executor for AgentX.

Routes tool execution requests to the Agentix server for tools that
require server resources or advanced analysis capabilities.

Supported tool categories:
- Code Analysis (CST/AST) - Code structure analysis, refactoring
- Advanced (Server resources) - API calls, complex operations
"""

import json
import logging
from typing import Any, Callable, Optional

logger = logging.getLogger(__name__)

from .code_analysis import (
    execute_analyze_syntax,
    execute_find_classes,
    execute_find_functions,
    execute_find_imports,
    execute_suggest_refactoring,
)

# Registry mapping tool name → handler function; add new code-analysis tools here.
_CODE_ANALYSIS_DISPATCH: dict[str, "Callable[[str], dict]"] = {
    "analyze_syntax": execute_analyze_syntax,
    "find_functions": execute_find_functions,
    "find_classes": execute_find_classes,
    "find_imports": execute_find_imports,
    "suggest_refactoring": execute_suggest_refactoring,
}


class ServerToolExecutor:
    """
    Executes tools on the Agentix server.

    Handles routing of tool requests to Agentix and formatting results
    for display/storage in conversation context.

    Supported server tools will be discovered from Agentix and include:
    - CST (Concrete Syntax Tree) tools for code analysis
    - AST (Abstract Syntax Tree) tools for semantic analysis
    - Future: API integration tools, database tools, etc.
    """

    def __init__(self, agentix_bridge):
        """
        Initialize server tool executor.

        Args:
            agentix_bridge: AgentixBridge instance for communication
        """
        self.agentix_bridge = agentix_bridge
        self._tool_cache = None

    def get_available_tools(self) -> list[dict]:
        """
        Get list of available server tools from Agentix.

        Returns:
            List of tool definitions in OpenAI format
        """
        if not self.is_available():
            return []

        if self._tool_cache is None:
            try:
                self._tool_cache = self.agentix_bridge.get_available_tools()
            except Exception as e:
                logger.error("Error fetching server tools: %s", e)
                return []

        return self._tool_cache

    def is_available(self) -> bool:
        """Return True when Agentix bridge is available."""
        return self.agentix_bridge is not None

    def execute(self, tool_name: str, arguments: dict, context: Optional[dict] = None) -> str:
        """
        Execute a server-side tool through Agentix.

        Args:
            tool_name: Name of the tool to execute
            arguments: Tool arguments
            context: Optional context snapshot for server execution

        Returns:
            Tool execution result as string

        Raises:
            ValueError: If tool not found or not available
        """
        # Try to execute as code analysis tool first (local execution)
        if CodeAnalysisTool.is_code_analysis_tool(tool_name):
            return self._execute_code_analysis_tool(tool_name, arguments)

        if not self.is_available():
            raise ValueError("Server tools not available - Agentix not connected")

        try:
            # Check if tool is available
            available_tools = self.get_available_tools()
            tool_names = [self._extract_tool_name(t) for t in available_tools]

            if tool_name not in tool_names:
                raise ValueError(f"Unknown server tool: {tool_name}")

            # Build tool request
            tool_request = {"name": tool_name, "arguments": arguments, "context": context or {}}

            # Execute through Agentix bridge
            result = self._execute_via_agentix(tool_request)

            # Format result for context/display
            formatted = self._format_tool_result(
                tool_name=tool_name,
                status="success" if result.get("success", False) else "error",
                message=result.get("message", ""),
                result=result.get("result", {}),
            )

            return formatted

        except Exception as e:
            return f"Error executing server tool '{tool_name}': {str(e)}"

    def _execute_code_analysis_tool(self, tool_name: str, arguments: dict) -> str:
        """
        Execute code analysis tool locally.

        Args:
            tool_name: Name of the code analysis tool
            arguments: Tool arguments (must include 'code')

        Returns:
            Formatted tool result
        """
        try:
            if "code" not in arguments:
                return self._format_tool_result(
                    tool_name=tool_name, status="error", message="Missing required argument: 'code'"
                )

            code = arguments["code"]

            # Dispatch to the appropriate code analysis handler.
            handler = _CODE_ANALYSIS_DISPATCH.get(tool_name)
            if handler is None:
                return self._format_tool_result(
                    tool_name=tool_name, status="error", message=f"Unknown code analysis tool: {tool_name}"
                )
            result = handler(code)

            # Format successful result
            return self._format_tool_result(
                tool_name=tool_name,
                status="success" if result.get("success") else "error",
                message="Code analysis completed" if result.get("success") else "Code analysis failed",
                result=result.get("data", {}),
            )

        except Exception as e:
            return self._format_tool_result(
                tool_name=tool_name, status="error", message=f"Code analysis error: {str(e)}"
            )

    def _execute_via_agentix(self, tool_request: dict) -> dict:
        """
        Execute tool request through Agentix API.

        Args:
            tool_request: Tool request with name, arguments, context

        Returns:
            Result dictionary with success, message, and result fields
        """
        try:
            # Call Agentix bridge to execute tool
            # The bridge handles streaming and response formatting
            response = self.agentix_bridge.execute_tool(
                tool_name=tool_request["name"], arguments=tool_request["arguments"]
            )

            # Handle streamed response if applicable
            if hasattr(response, "__iter__") and not isinstance(response, (str, dict)):
                # Collect streamed chunks
                chunks = []
                for chunk in response:
                    if isinstance(chunk, dict):
                        chunks.append(chunk)

                return {
                    "success": True,
                    "message": "Tool executed successfully (streamed)",
                    "result": {"chunks": chunks, "chunk_count": len(chunks)},
                }

            # Handle direct response
            if isinstance(response, dict):
                return response

            return {"success": True, "message": "Tool executed successfully", "result": {"output": str(response)}}

        except Exception as e:
            return {"success": False, "message": f"Tool execution failed: {str(e)}", "result": {}}

    def _extract_tool_name(self, tool_def: dict) -> str:
        """Extract tool name from OpenAI format tool definition."""
        if "function" in tool_def:
            return tool_def["function"].get("name", "unknown")
        return tool_def.get("name", "unknown")

    def _format_tool_result(self, tool_name: str, status: str, message: str = "", result: Optional[Any] = None) -> str:
        """
        Format tool result for storage and display.

        Args:
            tool_name: Name of the tool
            status: Execution status (success, pending, error)
            message: Status message
            result: Tool result data

        Returns:
            Formatted result string
        """
        output = {
            "tool": tool_name,
            "status": status,
            "message": message,
        }

        if result is not None:
            output["result"] = result

        return json.dumps(output, indent=2)


class CodeAnalysisTool:
    """
    Wrapper for code analysis tools (CST/AST).

    These tools analyze Python code structure and provide insights
    for refactoring, optimization, and understanding.
    """

    # Available code analysis tools
    TOOLS = {
        "analyze_syntax": {
            "description": "Analyze Python code syntax and structure using CST",
            "categories": ["syntax", "structure"],
        },
        "find_functions": {
            "description": "Find all functions in code and their signatures",
            "categories": ["functions", "definitions"],
        },
        "find_classes": {
            "description": "Find all classes and methods",
            "categories": ["classes", "definitions"],
        },
        "find_imports": {
            "description": "Find all imports and dependencies",
            "categories": ["imports", "dependencies"],
        },
        "suggest_refactoring": {
            "description": "Suggest code refactoring opportunities",
            "categories": ["refactoring", "optimization"],
        },
    }

    @classmethod
    def is_code_analysis_tool(cls, tool_name: str) -> bool:
        """Check if a tool is a code analysis tool."""
        return tool_name in cls.TOOLS

    @classmethod
    def get_description(cls, tool_name: str) -> Optional[str]:
        """Get description of a code analysis tool."""
        tool = cls.TOOLS.get(tool_name)
        return tool["description"] if tool else None


class AdvancedToolRegistry:
    """
    Registry of advanced tools available through Agentix.

    Tracks which tools are available and their capabilities.
    """

    def __init__(self, agentix_bridge):
        """Initialize registry."""
        self.agentix_bridge = agentix_bridge
        self._registry = {}
        self._initialized = False

    def initialize(self):
        """Load tools from Agentix."""
        if self._initialized:
            return

        try:
            tools = self.agentix_bridge.get_available_tools()
            for tool in tools:
                tool_name = self._extract_name(tool)
                tool_desc = self._extract_description(tool)
                self._registry[tool_name] = {
                    "name": tool_name,
                    "description": tool_desc,
                    "is_code_analysis": CodeAnalysisTool.is_code_analysis_tool(tool_name),
                }
        except Exception as e:
            logger.error("Error initializing advanced tool registry: %s", e)

        self._initialized = True

    def get_tool_info(self, tool_name: str) -> Optional[dict]:
        """Get information about a tool."""
        if not self._initialized:
            self.initialize()

        return self._registry.get(tool_name)

    def list_tools(self, category: Optional[str] = None) -> list[dict]:
        """
        List available tools.

        Args:
            category: Optional filter (e.g., "code_analysis")

        Returns:
            List of tool information dictionaries
        """
        if not self._initialized:
            self.initialize()

        tools = list(self._registry.values())

        if category == "code_analysis":
            tools = [t for t in tools if t.get("is_code_analysis")]

        return tools

    def _extract_name(self, tool_def: dict) -> str:
        """Extract tool name from OpenAI format."""
        if "function" in tool_def:
            return tool_def["function"].get("name", "unknown")
        return tool_def.get("name", "unknown")

    def _extract_description(self, tool_def: dict) -> str:
        """Extract tool description from OpenAI format."""
        if "function" in tool_def:
            return tool_def["function"].get("description", "")
        return tool_def.get("description", "")
