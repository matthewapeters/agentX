"""
Integration layer between AgentX GUI and Agentix middleware.

This module provides adapters and handlers to connect AgentX's
tkinter-based GUI with Agentix's async streaming API.
"""

from .agentix_bridge_adapter import AgentixBridgeAdapter
from .response_handler import ResponseHandler

__all__ = ["AgentixBridgeAdapter", "ResponseHandler"]
