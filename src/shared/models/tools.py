"""
Tool interfaces for client-side and server-side tool execution.

Tools are classified by their execution context:
- Client-side: Execute on AgentX client (file ops, user context)
- Server-side: Execute on Agentix server (DB ops, API calls, compute)

The tool system supports both local and remote Agentix servers.
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Optional, Protocol
import json


class ToolExecutionContext(str, Enum):
    """
    Where a tool should be executed.
    
    CLIENT: Execute on the AgentX client
        - File system operations (read, write, list)
        - User environment access (clipboard, selection)
        - Local code analysis
        
    SERVER: Execute on the Agentix server
        - Database operations
        - External API calls
        - Compute-intensive tasks
        - Operations requiring server credentials
        
    EITHER: Can execute on either client or server
        - Generic utilities
        - Depends on where resources are located
    """
    CLIENT = "client"
    SERVER = "server"
    EITHER = "either"


@dataclass
class ToolDefinition:
    """
    Definition of a tool that can be called by the LLM.
    
    This follows the OpenAI function calling format for compatibility
    with Ollama and other LLM providers.
    
    Attributes:
        name: Unique identifier for the tool
        description: Human-readable description for the LLM
        parameters: JSON Schema for the tool's parameters
        execution_context: Where the tool should be executed
        returns: JSON Schema for the return type (optional)
    """
    
    name: str
    description: str
    parameters: dict = field(default_factory=lambda: {"type": "object", "properties": {}})
    execution_context: ToolExecutionContext = ToolExecutionContext.CLIENT
    returns: Optional[dict] = None
    
    def to_openai_format(self) -> dict:
        """
        Convert to OpenAI function calling format.
        
        Used when sending tool definitions to the LLM.
        """
        return {
            "type": "function",
            "function": {
                "name": self.name,
                "description": self.description,
                "parameters": self.parameters,
            }
        }
    
    def to_dict(self) -> dict:
        """Serialize for transmission/storage."""
        return {
            "name": self.name,
            "description": self.description,
            "parameters": self.parameters,
            "execution_context": self.execution_context.value,
            "returns": self.returns,
        }
    
    @classmethod
    def from_dict(cls, data: dict) -> "ToolDefinition":
        """Create ToolDefinition from dictionary."""
        return cls(
            name=data.get("name", ""),
            description=data.get("description", ""),
            parameters=data.get("parameters", {"type": "object", "properties": {}}),
            execution_context=ToolExecutionContext(data.get("execution_context", "client")),
            returns=data.get("returns"),
        )


@dataclass
class ToolRequest:
    """
    Request to execute a tool.
    
    Sent from AgentX client to Agentix server for server-side tools,
    or processed locally for client-side tools.
    
    Attributes:
        tool_name: Name of the tool to execute
        arguments: Arguments to pass to the tool
        request_id: Unique identifier for tracking
        context_snapshot: Relevant context for server-side execution
    """
    
    tool_name: str
    arguments: dict = field(default_factory=dict)
    request_id: Optional[str] = None
    context_snapshot: Optional[dict] = None
    tool_input: Optional[dict] = None

    def __post_init__(self):
        if self.tool_input is not None and not self.arguments:
            self.arguments = self.tool_input
    
    def to_dict(self) -> dict:
        """Serialize for transmission."""
        data = {
            "tool_name": self.tool_name,
            "arguments": self.arguments,
        }
        if self.tool_input:
            data["tool_input"] = self.tool_input
        if self.request_id:
            data["request_id"] = self.request_id
        if self.context_snapshot:
            data["context_snapshot"] = self.context_snapshot
        return data
    
    @classmethod
    def from_dict(cls, data: dict) -> "ToolRequest":
        """Create ToolRequest from dictionary."""
        return cls(
            tool_name=data.get("tool_name", ""),
            arguments=data.get("arguments", data.get("tool_input", {})),
            request_id=data.get("request_id"),
            context_snapshot=data.get("context_snapshot"),
            tool_input=data.get("tool_input"),
        )
    
    @classmethod
    def from_llm_tool_call(cls, tool_call: dict) -> "ToolRequest":
        """
        Create ToolRequest from LLM tool call format.
        
        Handles both OpenAI and Ollama tool call formats.
        """
        # OpenAI format
        if "function" in tool_call:
            func = tool_call["function"]
            args = func.get("arguments", {})
            if isinstance(args, str):
                args = json.loads(args)
            return cls(
                tool_name=func.get("name", ""),
                arguments=args,
                request_id=tool_call.get("id"),
            )
        
        # Ollama format
        return cls(
            tool_name=tool_call.get("name", ""),
            arguments=tool_call.get("arguments", {}),
        )


@dataclass
class ToolResponse:
    """
    Response from tool execution.
    
    Returned from tool execution (either client-side or server-side)
    and stored in the conversation context.
    
    Attributes:
        success: Whether execution succeeded
        output: The tool's output (any JSON-serializable value)
        error: Error message if execution failed
        request_id: Matches the request_id from ToolRequest
        execution_time_ms: How long execution took
        add_to_context: Whether to add this result to conversation context
    """
    
    success: bool
    output: Any = None
    error: Optional[str] = None
    request_id: Optional[str] = None
    execution_time_ms: Optional[int] = None
    add_to_context: bool = True
    result: Any = None

    def __post_init__(self):
        if self.result is not None and self.output is None:
            self.output = self.result
    
    def to_dict(self) -> dict:
        """Serialize for transmission/storage."""
        data = {
            "success": self.success,
            "output": self.output,
            "add_to_context": self.add_to_context,
        }
        if self.output is not None:
            data["result"] = self.output
        if self.error:
            data["error"] = self.error
        if self.request_id:
            data["request_id"] = self.request_id
        if self.execution_time_ms is not None:
            data["execution_time_ms"] = self.execution_time_ms
        return data
    
    @classmethod
    def from_dict(cls, data: dict) -> "ToolResponse":
        """Create ToolResponse from dictionary."""
        return cls(
            success=data.get("success", False),
            output=data.get("output"),
            error=data.get("error"),
            request_id=data.get("request_id"),
            execution_time_ms=data.get("execution_time_ms"),
            add_to_context=data.get("add_to_context", True),
        )
    
    @classmethod
    def success_response(cls, output: Any, request_id: Optional[str] = None) -> "ToolResponse":
        """Create a successful response."""
        return cls(success=True, output=output, request_id=request_id)
    
    @classmethod
    def error_response(cls, error: str, request_id: Optional[str] = None) -> "ToolResponse":
        """Create an error response."""
        return cls(success=False, error=error, request_id=request_id)
    
    def to_llm_format(self) -> str:
        """Format for inclusion in LLM context."""
        if self.success:
            if isinstance(self.output, (dict, list)):
                return json.dumps(self.output, indent=2)
            return str(self.output)
        else:
            return f"Error: {self.error}"


class ITool(Protocol):
    """
    Protocol for tool implementations.
    
    Tools can be implemented as classes that follow this protocol.
    Both client-side and server-side tools use the same interface.
    """
    
    @property
    def definition(self) -> ToolDefinition:
        """Return the tool's definition."""
        ...
    
    async def execute(self, **kwargs) -> ToolResponse:
        """Execute the tool with the given arguments."""
        ...
    
    def validate_input(self, **kwargs) -> bool:
        """Validate input parameters before execution."""
        ...


class BaseTool(ABC):
    """
    Base class for tool implementations.
    
    Provides common functionality for tools. Subclass this to
    create new tools with proper definition and execution.
    
    Example:
        class ReadFileTool(BaseTool):
            @property
            def definition(self) -> ToolDefinition:
                return ToolDefinition(
                    name="read_file",
                    description="Read contents of a file",
                    parameters={
                        "type": "object",
                        "properties": {
                            "path": {"type": "string", "description": "File path"}
                        },
                        "required": ["path"]
                    },
                    execution_context=ToolExecutionContext.CLIENT,
                )
            
            async def execute(self, path: str) -> ToolResponse:
                try:
                    with open(path, 'r') as f:
                        return ToolResponse.success_response(f.read())
                except Exception as e:
                    return ToolResponse.error_response(str(e))
    """
    
    @property
    def definition(self) -> ToolDefinition:
        """Return the tool's definition."""
        parameters = getattr(self, "parameters", None)
        return ToolDefinition(
            name=getattr(self, "name", self.__class__.__name__),
            description=getattr(self, "description", ""),
            parameters=parameters or {"type": "object", "properties": {}},
            execution_context=getattr(
                self,
                "execution_context",
                ToolExecutionContext.CLIENT,
            ),
        )
    
    async def execute(self, **kwargs) -> ToolResponse:
        """Execute the tool with the given arguments."""
        raise NotImplementedError("Tool execution not implemented")
    
    def validate_input(self, **kwargs) -> bool:
        """
        Validate input parameters.
        
        Override this method to add custom validation logic.
        Default implementation checks required parameters.
        """
        required = self.definition.parameters.get("required", [])
        for param in required:
            if param not in kwargs:
                return False
        return True
    
    @property
    def name(self) -> str:
        """Convenience property for tool name."""
        return self.definition.name
    
    @property
    def execution_context(self) -> ToolExecutionContext:
        """Convenience property for execution context."""
        return self.definition.execution_context


class ToolRegistry:
    # --- Enabled tools state management ---
    def get_enabled_tools(self) -> list[str]:
        """Return a list of enabled tool names. Default: all tools enabled."""
        # In a real app, this could be loaded from config/session
        # For now, all registered tools are enabled by default
        return list(self._tools.keys())

    def set_enabled_tools(self, enabled_tools: list[str]) -> None:
        """Set which tools are enabled (stub for config/session integration)."""
        # In a real app, store this in config/session
        # Here, just a placeholder (no-op)
        pass

    def is_tool_enabled(self, name: str) -> bool:
        """Return True if the tool is enabled."""
        return name in self.get_enabled_tools()
    """
    Registry for discovering and managing tools.
    
    Maintains a collection of available tools and provides
    lookup functionality for tool execution.
    """
    
    def __init__(self):
        self._tools: dict[str, ITool] = {}
    
    def register(self, tool: ITool) -> None:
        """Register a tool."""
        self._tools[tool.definition.name] = tool
    
    def unregister(self, name: str) -> None:
        """Unregister a tool by name."""
        if name in self._tools:
            del self._tools[name]
    
    def get(self, name: str) -> Optional[ITool]:
        """Get a tool by name."""
        return self._tools.get(name)
    
    def list_definitions(self) -> list[ToolDefinition]:
        """List all tool definitions."""
        return [tool.definition for tool in self._tools.values()]

    def list_tools(self) -> list[str]:
        """List registered tool names."""
        return list(self._tools.keys())

    def get_definitions(self) -> list[ToolDefinition]:
        """Alias for list_definitions (backward compatibility)."""
        return self.list_definitions()
    
    def list_by_context(self, context: ToolExecutionContext) -> list[ToolDefinition]:
        """List tools for a specific execution context."""
        return [
            tool.definition for tool in self._tools.values()
            if tool.definition.execution_context == context
            or tool.definition.execution_context == ToolExecutionContext.EITHER
        ]
    
    def get_client_tools(self) -> list[ToolDefinition]:
        """Get tools that can execute on the client."""
        return self.list_by_context(ToolExecutionContext.CLIENT)
    
    def get_server_tools(self) -> list[ToolDefinition]:
        """Get tools that can execute on the server."""
        return self.list_by_context(ToolExecutionContext.SERVER)
    
    def to_openai_format(self) -> list[dict]:
        """Get all tools in OpenAI function calling format."""
        return [tool.definition.to_openai_format() for tool in self._tools.values()]
    
    async def execute(self, request: ToolRequest) -> ToolResponse:
        """
        Execute a tool by name.
        
        Args:
            request: The tool request
            
        Returns:
            Tool execution response
        """
        tool = self.get(request.tool_name)
        if tool is None:
            return ToolResponse.error_response(
                f"Tool not found: {request.tool_name}",
                request_id=request.request_id
            )
        
        if not tool.validate_input(**request.arguments):
            return ToolResponse.error_response(
                f"Invalid arguments for tool: {request.tool_name}",
                request_id=request.request_id
            )
        
        import time
        start = time.time()
        
        try:
            response = await tool.execute(**request.arguments)
            response.request_id = request.request_id
            response.execution_time_ms = int((time.time() - start) * 1000)
            return response
        except Exception as e:
            return ToolResponse.error_response(
                str(e),
                request_id=request.request_id
            )


# Global registries for client and server tools
client_tool_registry = ToolRegistry()
server_tool_registry = ToolRegistry()
