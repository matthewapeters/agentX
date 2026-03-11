"""
Handler for processing ResponseChunks and updating AgentX GUI.

Converts ResponseChunk objects from Agentix into GUI updates that
AgentX can display to the user.
"""

import sys
from pathlib import Path
from typing import Callable, Optional

# Add parent directories to path
parent_dir = str(Path(__file__).parent.parent.parent)
if parent_dir not in sys.path:
    sys.path.insert(0, parent_dir)

from shared.models.response import ResponseChunk, ChunkType
from shared.models.message import Message, MessageRole


class ResponseHandler:
    """
    Handles ResponseChunk processing and converts to GUI operations.
    
    This class acts as a translator between Agentix's ResponseChunk
    format and AgentX's GUI display needs.
    
    Example usage:
        handler = ResponseHandler(
            on_content=lambda text: gui.append_output(text),
            on_thinking=lambda text: gui.show_thinking(text),
            on_tool_call=lambda name, args: gui.show_tool(name, args),
        )
        
        for chunk in stream:
            handler.process_chunk(chunk)
    """
    
    def __init__(
        self,
        on_content: Optional[Callable[[str], None]] = None,
        on_thinking: Optional[Callable[[str], None]] = None,
        on_tool_call: Optional[Callable[[str, dict, Optional[int]], None]] = None,
        on_tool_result: Optional[Callable[[str, str, Optional[int], Optional[str]], None]] = None,
        on_classification: Optional[Callable[[dict], None]] = None,
        on_error: Optional[Callable[[str, str], None]] = None,
        on_done: Optional[Callable[[], None]] = None,
    ):
        """
        Initialize handler with callback functions.
        
        Args:
            on_content: Called with content text for display
            on_thinking: Called with thinking/reasoning text
            on_tool_call: Called with (tool_name, tool_input)
            on_tool_result: Called with (tool_name, result_output, round_index, tool_id)
            on_classification: Called with classification metadata
            on_error: Called with (error_message, error_code)
            on_done: Called when stream is complete
        """
        self.on_content = on_content or (lambda text: None)
        self.on_thinking = on_thinking or (lambda text: None)
        self.on_tool_call = on_tool_call or (lambda name, args, round_i=None: None)
        self.on_tool_result = on_tool_result or (lambda name, result, round_i=None, tool_id=None: None)
        self.on_classification = on_classification or (lambda meta: None)
        self.on_error = on_error or (lambda msg, code: None)
        self.on_done = on_done or (lambda: None)
        
        # Accumulators for building complete messages
        self.content_buffer = []
        self.thinking_buffer = []
        self.tool_calls = []
        self.tool_results = []
        
    def process_chunk(self, chunk: ResponseChunk) -> None:
        """
        Process a single ResponseChunk and trigger appropriate callbacks.
        
        Args:
            chunk: ResponseChunk from Agentix stream
        """
        match chunk.type:
            case ChunkType.CONTENT:
                self._handle_content(chunk)
            
            case ChunkType.THINKING:
                self._handle_thinking(chunk)
            
            case ChunkType.TOOL_CALL:
                self._handle_tool_call(chunk)
            
            case ChunkType.TOOL_RESULT:
                self._handle_tool_result(chunk)
            
            case ChunkType.CLASSIFICATION:
                self._handle_classification(chunk)
            
            case ChunkType.ERROR:
                self._handle_error(chunk)
            
            case ChunkType.DONE:
                self._handle_done(chunk)
    
    def _handle_content(self, chunk: ResponseChunk) -> None:
        """Handle content chunk - main assistant response."""
        if chunk.content:
            self.content_buffer.append(chunk.content)
            self.on_content(chunk.content)
    
    def _handle_thinking(self, chunk: ResponseChunk) -> None:
        """Handle thinking chunk - internal reasoning."""
        if chunk.content:
            self.thinking_buffer.append(chunk.content)
            self.on_thinking(chunk.content)
    
    def _handle_tool_call(self, chunk: ResponseChunk) -> None:
        """Handle tool call chunk."""
        if chunk.tool_name:
            self.tool_calls.append({
                "name": chunk.tool_name,
                "input": chunk.tool_input or {},
            })
            self.on_tool_call(chunk.tool_name, chunk.tool_input or {}, chunk.round_index)
    
    def _handle_tool_result(self, chunk: ResponseChunk) -> None:
        """Handle tool result chunk."""
        output = chunk.tool_output
        if output is None:
            output = chunk.content or ""
        self.tool_results.append({
            "name": chunk.tool_name,
            "result": output,
        })
        tool_id = chunk.tool_id
        tool_name = chunk.tool_name or "unknown"
        self.on_tool_result(tool_name, output, chunk.round_index, tool_id)
    
    def _handle_classification(self, chunk: ResponseChunk) -> None:
        """Handle classification metadata chunk."""
        if chunk.classification:
            self.on_classification(chunk.classification)
    
    def _handle_error(self, chunk: ResponseChunk) -> None:
        """Handle error chunk."""
        # ERROR chunks only have content, no separate error_code attribute
        error_code = "ERROR"
        self.on_error(chunk.content, error_code)
    
    def _handle_done(self, chunk: ResponseChunk) -> None:
        """Handle completion chunk."""
        self.on_done()
    
    def get_complete_content(self) -> str:
        """
        Get accumulated content as a single string.
        
        Returns:
            Complete content text
        """
        return "".join(self.content_buffer)
    
    def get_complete_thinking(self) -> str:
        """
        Get accumulated thinking as a single string.
        
        Returns:
            Complete thinking text
        """
        return "".join(self.thinking_buffer)
    
    def to_message(self) -> Message:
        """
        Convert accumulated chunks to a Message object.
        
        Returns:
            Message with role=ASSISTANT and accumulated content
        """
        from shared.models.message import assistant_message
        
        content = self.get_complete_content()
        msg = assistant_message(content)
        
        # Add metadata if we have tool calls
        if self.tool_calls:
            msg.metadata = msg.metadata or {}
            msg.metadata["tool_calls"] = self.tool_calls
            msg.metadata["tool_results"] = self.tool_results
        
        return msg
    
    def reset(self) -> None:
        """Reset all buffers for a new streaming session."""
        self.content_buffer.clear()
        self.thinking_buffer.clear()
        self.tool_calls.clear()
        self.tool_results.clear()
