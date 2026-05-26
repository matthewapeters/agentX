"""
Integration layer between AgentX GUI and Agentix middleware.

This module provides adapters and handlers to connect AgentX's
tkinter-based GUI with Agentix's async streaming API.
"""

from ..gui.model_selector import ModelSelector
from ..gui.progress_widgets import (
    ProgressIndicator,
    ProgressPanel,
    ResultStreamWidget,
    StreamingExecutionUI,
)
from .agentix_bridge_adapter import AgentixBridgeAdapter
from .client_tool_executor import ClientToolExecutor
from .code_analysis import (
    CodeAnalyzer,
    execute_analyze_syntax,
    execute_find_classes,
    execute_find_functions,
    execute_find_imports,
    execute_suggest_refactoring,
)
from .response_handler import ResponseHandler
from .server_tool_executor import AdvancedToolRegistry, CodeAnalysisTool, ServerToolExecutor
from .streaming_executor import (
    ProgressTracker,
    ProgressType,
    ProgressUpdate,
    StreamingExecutor,
    StreamingToolChain,
    create_progress_stream,
)
from .terminal_bridge import PermissionDecision, PermissionLayer, TerminalBridge, TerminalResult
from .tui_bridge import TuiBridge

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
    "StreamingExecutor",
    "ProgressUpdate",
    "ProgressType",
    "ProgressTracker",
    "StreamingToolChain",
    "create_progress_stream",
    "ProgressIndicator",
    "ProgressPanel",
    "ResultStreamWidget",
    "StreamingExecutionUI",
    "TerminalBridge",
    "PermissionLayer",
    "PermissionDecision",
    "TerminalResult",
    "TuiBridge",
]
