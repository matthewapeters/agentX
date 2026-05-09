"""
Integration layer between AgentX GUI and Agentix middleware.

This module provides adapters and handlers to connect AgentX's
tkinter-based GUI with Agentix's async streaming API.
"""

from .agentix_bridge_adapter import AgentixBridgeAdapter
from .response_handler import ResponseHandler
from ..gui.model_selector import ModelSelector
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
from .streaming_executor import (
    StreamingExecutor,
    ProgressUpdate,
    ProgressType,
    ProgressTracker,
    StreamingToolChain,
    create_progress_stream,
)
from ..gui.progress_widgets import (
    ProgressIndicator,
    ProgressPanel,
    ResultStreamWidget,
    StreamingExecutionUI,
)
from .terminal_bridge import TerminalBridge, PermissionLayer, PermissionDecision, TerminalResult
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
