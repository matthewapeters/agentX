"""
Shared models for AgentX and Agentix integration.
"""

from .attachment import Attachment
from .context import Context
from .message import Message, MessageRole
from .response import ChunkType, ResponseChunk
from .task_node import AssertionRecord, PlanRecord, PlanStep, SynthesisAttempt, TaskNodeRecord, TaskTree
from .tools import ToolDefinition, ToolExecutionContext, ToolRequest, ToolResponse

__all__ = [
    "Attachment",
    "Context",
    "Message",
    "MessageRole",
    "ResponseChunk",
    "ChunkType",
    "ToolDefinition",
    "ToolRequest",
    "ToolResponse",
    "ToolExecutionContext",
    # Hierarchical task execution
    "AssertionRecord",
    "PlanRecord",
    "PlanStep",
    "SynthesisAttempt",
    "TaskNodeRecord",
    "TaskTree",
]
