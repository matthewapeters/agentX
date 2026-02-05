"""
Integration layer between AgentX GUI and Agentix middleware.

This module provides adapters and handlers to connect AgentX's
tkinter-based GUI with Agentix's async streaming API.
"""

from .agentix_bridge_adapter import AgentixBridgeAdapter
from .response_handler import ResponseHandler
from .model_selector import ModelSelector
from .tool_panel import ToolPanel
from .client_tool_executor import ClientToolExecutor
from .server_tool_executor import ServerToolExecutor, AdvancedToolRegistry, CodeAnalysisTool
from .code_analysis import (
    CodeAnalyzer,
    execute_analyze_syntax,
    execute_find_functions,
    execute_find_classes,
    execute_find_imports,
    execute_suggest_refactoring,
)

__all__ = [
    "AgentixBridgeAdapter",
    "ResponseHandler",
    "ModelSelector",
    "ToolPanel",
    "ClientToolExecutor",
    "ServerToolExecutor",
    "AdvancedToolRegistry",
    "CodeAnalysisTool",
    "CodeAnalyzer",
    "execute_analyze_syntax",
    "execute_find_functions",
    "execute_find_classes",
    "execute_find_imports",
    "execute_suggest_refactoring",
]
