"""
Programmatic API bridge for Agentix.

This module provides a clean programmatic interface to Agentix functionality
that can be consumed by AgentX GUI without going through the CLI.
"""

import json
import sys
from typing import AsyncIterator, Iterator, Optional

# Direct imports to avoid circular dependencies
from agentix.agentix_config import AgentixConfig
from agentix.api_client import query_api, query_api_streaming
from agentix.models import get_models, get_model
from agentix.prompt_classification_response import (
    PromptClassificationResponse,
    Intent,
    NextStep,
)
from agentix.query_payload import QueryPayload

# Import shared models
import os

# Add parent dir to path for shared imports
parent_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if parent_dir not in sys.path:
    sys.path.insert(0, parent_dir)

from shared.models.message import Message, MessageRole
from shared.models.context import Context
from shared.models.response import ResponseChunk, ChunkType


class AgentixBridge:
    """
    Bridge between AgentX GUI and Agentix middleware.
    
    Provides a programmatic interface to:
    - Prompt classification
    - Model management
    - Tool discovery
    - Streaming response generation
    
    Example usage:
        config = AgentixConfig(model="llama3.2", debug=False)
        bridge = AgentixBridge(config)
        
        # Classify prompt
        classification = bridge.classify_prompt("Read file.py", context)
        
        # Process with streaming
        for chunk in bridge.process_prompt_streaming(prompt, context, classification):
            print(chunk.content)
    """
    
    def __init__(self, config: AgentixConfig):
        """
        Initialize AgentixBridge with configuration.
        
        Args:
            config: AgentixConfig instance with model, tools, etc.
        """
        self.config = config
        self._model_cache: Optional[list[dict]] = None
        self._max_tokens: Optional[int] = None
        
    def classify_prompt(
        self,
        prompt: str,
        context: Context,
    ) -> PromptClassificationResponse:
        """
        Classify user intent before processing.
        
        Analyzes the user's prompt to determine:
        - Intent type (conversation, simple_action, complex_action, safety_issue)
        - Whether clarification is needed
        - Next step to take (respond_directly, single_tool, invoke_planner, escalate)
        
        Args:
            prompt: User's input text
            context: Current conversation context
            
        Returns:
            PromptClassificationResponse with intent and next_step
        """
        # Convert shared Context to Agentix format
        history = self._context_to_history(context)
        
        # Build classification prompt using Agentix logic
        from .context.sessions import assemble_classification_prompt
        
        classification_payload = assemble_classification_prompt(
            self.config,
            history,
            self._get_max_tokens(),
        )
        
        # Query API for classification
        result = query_api(self.config, classification_payload)
        
        # Parse result into PromptClassificationResponse
        return PromptClassificationResponse(
            intent=Intent[result.get("intent", "conversation")],
            needs_clarification=result.get("needs_clarification", False),
            missing_fields=result.get("missing_fields", []),
            reasoning_summary=result.get("reasoning_summary", ""),
            next_step=NextStep[result.get("next_step", "respond_directly")],
        )
    
    def process_prompt_streaming(
        self,
        prompt: str,
        context: Context,
        classification: Optional[PromptClassificationResponse] = None,
    ) -> Iterator[ResponseChunk]:
        """
        Process prompt through appropriate handler with streaming.
        
        This routes the request based on classification:
        - respond_directly: Direct LLM response
        - single_tool: Execute one tool then respond
        - invoke_planner: Multi-step planning
        - escalate: Human intervention required
        
        Args:
            prompt: User's input text
            context: Current conversation context
            classification: Optional pre-computed classification
            
        Yields:
            ResponseChunk objects for GUI rendering
        """
        # Auto-classify if not provided
        if classification is None:
            yield ResponseChunk(
                type=ChunkType.THINKING,
                content="Classifying prompt...",
            )
            classification = self.classify_prompt(prompt, context)
        
        # Emit classification for GUI display
        if self.config.debug:
            yield ResponseChunk(
                type=ChunkType.CLASSIFICATION,
                classification={
                    "intent": classification.intent.name,
                    "next_step": classification.next_step.name,
                    "reasoning": classification.reasoning_summary,
                },
            )
        
        # Route based on next_step
        match classification.next_step:
            case NextStep.respond_directly:
                yield from self._stream_direct_response(prompt, context)
            
            case NextStep.single_tool:
                yield from self._stream_tool_response(prompt, context, classification)
            
            case NextStep.invoke_planner:
                yield from self._stream_planned_response(prompt, context)
            
            case NextStep.escalate:
                yield ResponseChunk(
                    type=ChunkType.ERROR,
                    content="This request requires human assistance or involves safety concerns.",
                )
    
    def get_available_models(self) -> list[dict]:
        """
        Fetch available models from Ollama.
        
        Returns cached results if available.
        
        Returns:
            List of model dictionaries with name, size, details
        """
        if self._model_cache is None:
            # Don't filter - we want all models for the UI dropdown
            self._model_cache = get_models(self.config, filter_by_model=False)
        return self._model_cache
    
    def get_available_tools(self) -> list[dict]:
        """
        Return available MCP tools with metadata.
        
        Tools are defined in Agentix and include:
        - CST (Concrete Syntax Tree) analysis
        - AST (Abstract Syntax Tree) analysis
        - Code search and manipulation
        
        Returns:
            List of tool definitions in OpenAI tools format
        """
        from .tools.describe_tools import extract_tools_from_file, to_openai_tools
        
        tools = []
        for tool_name in self.config.tools or []:
            if tool_name == "cst":
                from .tools import extract_cst_tools
                cst_tools = extract_cst_tools()
                tools.extend(cst_tools)
            elif tool_name == "ast":
                # AST tools would be added here
                pass
        
        return to_openai_tools(tools) if tools else []
    
    def _context_to_history(self, context: Context) -> list[Message]:
        """
        Convert AgentX Context to Agentix history format.
        
        Args:
            context: AgentX Context with messages
            
        Returns:
            List of enabled Message dictionaries (Agentix format)
        """
        # Note: Return type annotation says list[Message] but Agentix
        # actually expects list[dict]. Converting to dicts here.
        return [msg.to_dict() for msg in context.get_enabled_messages()]
    
    def _get_max_tokens(self) -> int:
        """
        Get max tokens for current model.
        
        Caches the result after first call.
        
        Returns:
            Maximum token count for model
        """
        if self._max_tokens is None:
            self._max_tokens = get_model(self.config)
        return self._max_tokens
    
    def _stream_direct_response(
        self,
        prompt: str,
        context: Context,
    ) -> Iterator[ResponseChunk]:
        """
        Stream a direct LLM response without tools.
        
        Args:
            prompt: User prompt
            context: Conversation context
            
        Yields:
            Content chunks from LLM
        """
        # Build payload from context
        history = self._context_to_history(context)
        
        # Convert to Ollama message format
        messages = [
            {"role": msg.role.value, "content": msg.content}
            for msg in history
        ]
        
        # Add current prompt
        messages.append({"role": "user", "content": prompt})
        
        payload = {
            "model": self.config.model,
            "messages": messages,
            "temperature": self.config.temperature,
        }
        
        # Stream from API
        try:
            for chunk in query_api_streaming(self.config, payload):
                if chunk.get("error"):
                    yield ResponseChunk(
                        type=ChunkType.ERROR,
                        content=chunk["error"],
                    )
                    break
                
                # Extract content from chunk
                message = chunk.get("message", {})
                content = message.get("content", "")
                
                if content:
                    yield ResponseChunk(
                        type=ChunkType.CONTENT,
                        content=content,
                    )
                
                if chunk.get("done"):
                    break
        except Exception as e:
            yield ResponseChunk(
                type=ChunkType.ERROR,
                content=f"Error generating response: {str(e)}",
            )
    
    def _stream_tool_response(
        self,
        prompt: str,
        context: Context,
        classification: PromptClassificationResponse,
    ) -> Iterator[ResponseChunk]:
        """
        Stream response involving a single tool call.
        
        Note: Full tool execution not yet implemented.
        Returns placeholder for now.
        
        Args:
            prompt: User prompt
            context: Conversation context
            classification: Classification result
            
        Yields:
            Tool call, tool result, and response chunks
        """
        # TODO: Implement actual tool execution
        yield ResponseChunk(
            type=ChunkType.CONTENT,
            content="Tool execution coming soon. Processing as direct response for now...",
        )
        
        # Fall back to direct response
        yield from self._stream_direct_response(prompt, context)
    
    def _stream_planned_response(
        self,
        prompt: str,
        context: Context,
    ) -> Iterator[ResponseChunk]:
        """
        Stream response using multi-step planning.
        
        Note: Full planner not yet implemented.
        Returns placeholder for now.
        
        Args:
            prompt: User prompt
            context: Conversation context
            
        Yields:
            Planning, tool calls, and response chunks
        """
        # TODO: Implement actual planner
        yield ResponseChunk(
            type=ChunkType.THINKING,
            content="Multi-step planning coming soon. Processing as direct response for now...",
        )
        
        # Fall back to direct response
        yield from self._stream_direct_response(prompt, context)


# Convenience function for quick usage
def create_bridge(
    model: str = "llama3.2",
    tools: Optional[list[str]] = None,
    debug: bool = False,
) -> AgentixBridge:
    """
    Create an AgentixBridge with sensible defaults.
    
    Args:
        model: Model name (default: llama3.2)
        tools: List of tool names (default: ["cst", "ast"])
        debug: Enable debug output
        
    Returns:
        Configured AgentixBridge instance
    """
    config = AgentixConfig(
        model=model,
        tools=tools or ["cst", "ast"],
        debug=debug,
    )
    return AgentixBridge(config)
