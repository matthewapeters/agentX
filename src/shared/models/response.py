"""
Response chunk models for streaming responses.

ResponseChunks are used for streaming data from Agentix server to AgentX client.
They represent incremental pieces of the LLM response, tool calls, and other events.
"""

from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Optional


class ChunkType(str, Enum):
    """
    Types of response chunks in the streaming protocol.
    
    Content types:
        CONTENT: Regular assistant response text
        THINKING: LLM reasoning/thinking text
        
    Tool types:
        TOOL_CALL: Request to execute a tool
        TOOL_RESULT: Result from tool execution
        
    Metadata types:
        CLASSIFICATION: Intent classification result
        MODEL_INFO: Information about the model being used
        
    Control types:
        ERROR: Error message
        DONE: Stream completion signal
    """
    # Content types
    CONTENT = "content"
    THINKING = "thinking"
    
    # Tool types
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"
    
    # Metadata types
    CLASSIFICATION = "classification"
    MODEL_INFO = "model_info"
    
    # Control types
    ERROR = "error"
    DONE = "done"


@dataclass
class ResponseChunk:
    """
    A chunk of streaming response from Agentix server.
    
    ResponseChunks are yielded by the Agentix API during streaming responses.
    The AgentX client processes these to update the GUI in real-time.
    
    Attributes:
        type: The type of this chunk
        content: Text content (for CONTENT, THINKING, ERROR types)
        
        # Tool-specific fields
        tool_name: Name of the tool (for TOOL_CALL, TOOL_RESULT)
        tool_input: Arguments for the tool (for TOOL_CALL)
        tool_output: Result from tool execution (for TOOL_RESULT)
        tool_execution_context: Where tool should execute (for TOOL_CALL)
        
        # Classification fields
        classification: Intent classification dict (for CLASSIFICATION)
        
        # Metadata
        model: Model name (for MODEL_INFO)
        done_reason: Reason for completion (for DONE)
    """
    
    type: Optional[ChunkType] = None
    content: str = ""
    chunk_type: Optional[ChunkType] = None
    
    # Tool-specific fields
    tool_name: Optional[str] = None
    tool_input: Optional[dict] = None
    tool_output: Optional[Any] = None
    tool_execution_context: Optional[str] = None  # "client" or "server"
    tool_id: Optional[str] = None
    round_index: Optional[int] = None  # which tool-loop round emitted this chunk
    
    # Classification fields
    classification: Optional[dict] = None
    
    # Metadata
    model: Optional[str] = None
    done_reason: Optional[str] = None
    error_code: Optional[str] = None
    
    def __post_init__(self):
        """Ensure type is ChunkType enum."""
        if self.chunk_type and self.type is None:
            self.type = self.chunk_type
        if self.type is None:
            self.type = ChunkType.CONTENT
        if isinstance(self.type, str):
            self.type = ChunkType(self.type)
        self.chunk_type = self.type
    
    def to_dict(self) -> dict:
        """
        Serialize chunk for transmission.
        
        Used when streaming over HTTP/WebSocket.
        """
        data = {
            "type": self.type.value,
            "content": self.content,
        }
        
        # Include optional fields if present
        if self.tool_name:
            data["tool_name"] = self.tool_name
        if self.tool_input:
            data["tool_input"] = self.tool_input
        if self.tool_output is not None:
            data["tool_output"] = self.tool_output
        if self.tool_execution_context:
            data["tool_execution_context"] = self.tool_execution_context
        if self.tool_id:
            data["tool_id"] = self.tool_id
        if self.classification:
            data["classification"] = self.classification
        if self.model:
            data["model"] = self.model
        if self.done_reason:
            data["done_reason"] = self.done_reason
        if self.error_code:
            data["error_code"] = self.error_code
            
        return data
    
    @classmethod
    def from_dict(cls, data: dict) -> "ResponseChunk":
        """Create ResponseChunk from dictionary."""
        return cls(
            type=ChunkType(data.get("type", "content")),
            content=data.get("content", ""),
            chunk_type=ChunkType(data.get("type", "content")),
            tool_name=data.get("tool_name"),
            tool_input=data.get("tool_input"),
            tool_output=data.get("tool_output"),
            tool_execution_context=data.get("tool_execution_context"),
            tool_id=data.get("tool_id"),
            classification=data.get("classification"),
            model=data.get("model"),
            done_reason=data.get("done_reason"),
            error_code=data.get("error_code"),
        )
    
    @classmethod
    def from_ollama_chunk(cls, ollama_chunk: dict) -> "ResponseChunk":
        """
        Create ResponseChunk from Ollama streaming response.
        
        Maps Ollama's response format to our unified chunk format.
        """
        message = ollama_chunk.get("message", {})
        
        # Determine chunk type based on content
        if message.get("thinking"):
            return cls(
                type=ChunkType.THINKING,
                content=message.get("thinking", ""),
            )
        elif message.get("tool_calls"):
            tool_call = message["tool_calls"][0]  # Handle first tool call
            return cls(
                type=ChunkType.TOOL_CALL,
                tool_name=tool_call.get("function", {}).get("name"),
                tool_input=tool_call.get("function", {}).get("arguments", {}),
            )
        elif message.get("content"):
            return cls(
                type=ChunkType.CONTENT,
                content=message.get("content", ""),
            )
        elif ollama_chunk.get("done"):
            return cls(
                type=ChunkType.DONE,
                done_reason=ollama_chunk.get("done_reason", "stop"),
            )
        else:
            return cls(type=ChunkType.CONTENT, content="")
    
    # Convenience properties
    
    @property
    def is_content(self) -> bool:
        """Check if this is a content chunk."""
        return self.type in (ChunkType.CONTENT, ChunkType.THINKING)
    
    @property
    def is_tool(self) -> bool:
        """Check if this is a tool-related chunk."""
        return self.type in (ChunkType.TOOL_CALL, ChunkType.TOOL_RESULT)
    
    @property
    def is_error(self) -> bool:
        """Check if this is an error chunk."""
        return self.type == ChunkType.ERROR
    
    @property
    def is_done(self) -> bool:
        """Check if this signals stream completion."""
        return self.type == ChunkType.DONE


# Factory functions for common chunk types

def content_chunk(text: str) -> ResponseChunk:
    """Create a content chunk."""
    return ResponseChunk(type=ChunkType.CONTENT, content=text)


def thinking_chunk(text: str) -> ResponseChunk:
    """Create a thinking chunk."""
    return ResponseChunk(type=ChunkType.THINKING, content=text)


def tool_call_chunk(
    tool_name: str, 
    tool_input: dict, 
    execution_context: str = "client"
) -> ResponseChunk:
    """Create a tool call chunk."""
    return ResponseChunk(
        type=ChunkType.TOOL_CALL,
        tool_name=tool_name,
        tool_input=tool_input,
        tool_execution_context=execution_context,
    )


def tool_result_chunk(tool_name: str, tool_output: Any, tool_id: Optional[str] = None) -> ResponseChunk:
    """Create a tool result chunk."""
    return ResponseChunk(
        type=ChunkType.TOOL_RESULT,
        tool_name=tool_name,
        tool_output=tool_output,
        tool_id=tool_id,
    )


def classification_chunk(classification: dict) -> ResponseChunk:
    """Create a classification chunk."""
    return ResponseChunk(
        type=ChunkType.CLASSIFICATION,
        classification=classification,
    )


def error_chunk(message: str, error_code: Optional[str] = None) -> ResponseChunk:
    """Create an error chunk."""
    return ResponseChunk(type=ChunkType.ERROR, content=message, error_code=error_code)


def done_chunk(reason: str = "stop") -> ResponseChunk:
    """Create a done chunk."""
    return ResponseChunk(type=ChunkType.DONE, done_reason=reason)
