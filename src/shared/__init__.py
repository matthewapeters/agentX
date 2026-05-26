"""
Shared module for AgentX and Agentix integration.

This module contains unified models, interfaces, and utilities used by both
the AgentX client and Agentix server components.

Architecture:
- AgentX (client): Owns all state - sessions, context, messages, attachments
- Agentix (server): Stateless middleware - classification, tool orchestration, LLM communication

Key Principles:
1. AgentX is the canonical source for all client-side data structures
2. Agentix server receives context in request payloads, does not persist state
3. Tools are classified as client-side or server-side based on execution requirements
"""

from .config import AgentixConfig, AgentXConfig, UnifiedConfig
from .models import (
    Attachment,
    ChunkType,
    Context,
    Message,
    MessageRole,
    ResponseChunk,
    ToolDefinition,
    ToolExecutionContext,
    ToolRequest,
    ToolResponse,
)

__all__ = [
    # Models
    "Attachment",
    "Context",
    "Message",
    "MessageRole",
    "ResponseChunk",
    "ChunkType",
    # Tools
    "ToolDefinition",
    "ToolRequest",
    "ToolResponse",
    "ToolExecutionContext",
    # Config
    "UnifiedConfig",
    "AgentXConfig",
    "AgentixConfig",
]
