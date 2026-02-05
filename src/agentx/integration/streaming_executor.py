"""
Streaming execution layer for AgentX.

Provides real-time streaming of tool results with progress tracking
and status updates for long-running operations.

Features:
- Incremental result streaming
- Progress reporting with metrics
- Cancellation support
- Progress callbacks for UI updates
"""

import json
import time
from typing import Optional, Callable, Any, Generator, Dict
from dataclasses import dataclass, asdict
from enum import Enum
import threading
from queue import Queue


class ProgressType(Enum):
    """Types of progress updates."""
    STARTED = "started"
    PROGRESS = "progress"
    CHUNK = "chunk"
    COMPLETE = "complete"
    ERROR = "error"
    CANCELLED = "cancelled"


@dataclass
class ProgressUpdate:
    """Progress update for streaming operations."""
    type: str  # ProgressType
    tool_name: str
    current: int = 0
    total: Optional[int] = None
    message: str = ""
    percent: float = 0.0
    data: Optional[Any] = None
    timestamp: Optional[float] = None
    
    def __post_init__(self):
        if self.timestamp is None:
            self.timestamp = time.time()
    
    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            **asdict(self),
            "type": self.type if isinstance(self.type, str) else self.type.value,
            "timestamp": self.timestamp
        }
    
    def to_json(self) -> str:
        """Convert to JSON."""
        return json.dumps(self.to_dict())


class StreamingExecutor:
    """
    Execute tools with streaming result support.
    
    Provides callbacks for real-time progress tracking and chunk delivery.
    """
    
    def __init__(self, max_chunk_size: int = 1024):
        """
        Initialize streaming executor.
        
        Args:
            max_chunk_size: Maximum size of result chunks
        """
        self.max_chunk_size = max_chunk_size
        self._cancelled = False
        self._progress_queue: Optional[Queue] = None
    
    def execute_with_streaming(
        self,
        tool_executor: Any,
        tool_name: str,
        arguments: dict,
        progress_callback: Optional[Callable[[ProgressUpdate], None]] = None,
    ) -> Generator[ProgressUpdate, None, None]:
        """
        Execute tool with streaming progress updates.
        
        Args:
            tool_executor: Tool executor (ClientToolExecutor, ServerToolExecutor, etc.)
            tool_name: Name of tool to execute
            arguments: Tool arguments
            progress_callback: Optional callback for progress updates
            
        Yields:
            ProgressUpdate objects with streaming results
        """
        self._cancelled = False
        
        try:
            # Send started event
            start_update = ProgressUpdate(
                type=ProgressType.STARTED.value,
                tool_name=tool_name,
                message=f"Starting {tool_name}"
            )
            if progress_callback:
                progress_callback(start_update)
            yield start_update
            
            # Execute tool
            result = tool_executor.execute(tool_name, arguments)
            
            if self._cancelled:
                cancel_update = ProgressUpdate(
                    type=ProgressType.CANCELLED.value,
                    tool_name=tool_name,
                    message="Execution cancelled"
                )
                if progress_callback:
                    progress_callback(cancel_update)
                yield cancel_update
                return
            
            # Stream result in chunks
            if isinstance(result, str):
                result_bytes = result.encode('utf-8')
            else:
                result_bytes = json.dumps(result).encode('utf-8')
            
            total_size = len(result_bytes)
            chunks_sent = 0
            bytes_sent = 0
            
            for i in range(0, len(result_bytes), self.max_chunk_size):
                if self._cancelled:
                    break
                
                chunk = result_bytes[i:i + self.max_chunk_size]
                bytes_sent += len(chunk)
                chunks_sent += 1
                
                percent = (bytes_sent / total_size * 100) if total_size > 0 else 100
                
                chunk_update = ProgressUpdate(
                    type=ProgressType.CHUNK.value,
                    tool_name=tool_name,
                    current=bytes_sent,
                    total=total_size,
                    percent=percent,
                    message=f"Sent {chunks_sent} chunks",
                    data=chunk.decode('utf-8', errors='replace')
                )
                
                if progress_callback:
                    progress_callback(chunk_update)
                yield chunk_update
            
            # Send completion event
            complete_update = ProgressUpdate(
                type=ProgressType.COMPLETE.value,
                tool_name=tool_name,
                current=total_size,
                total=total_size,
                percent=100.0,
                message=f"Completed: {chunks_sent} chunks, {total_size} bytes"
            )
            
            if progress_callback:
                progress_callback(complete_update)
            yield complete_update
            
        except Exception as e:
            error_update = ProgressUpdate(
                type=ProgressType.ERROR.value,
                tool_name=tool_name,
                message=f"Error: {str(e)}"
            )
            
            if progress_callback:
                progress_callback(error_update)
            yield error_update
    
    def cancel_execution(self):
        """Cancel ongoing execution."""
        self._cancelled = True
    
    def is_cancelled(self) -> bool:
        """Check if execution was cancelled."""
        return self._cancelled


class ProgressTracker:
    """
    Track execution progress for multiple concurrent operations.
    """
    
    def __init__(self):
        """Initialize progress tracker."""
        self._operations: Dict[str, ProgressUpdate] = {}
        self._callbacks: Dict[str, Callable] = {}
        self._lock = threading.Lock()
    
    def start_operation(self, operation_id: str, tool_name: str):
        """Start tracking an operation."""
        with self._lock:
            self._operations[operation_id] = ProgressUpdate(
                type=ProgressType.STARTED.value,
                tool_name=tool_name
            )
    
    def update_progress(self, operation_id: str, update: ProgressUpdate):
        """Update progress for an operation."""
        with self._lock:
            self._operations[operation_id] = update
            
            # Call registered callback if available
            if operation_id in self._callbacks:
                callback = self._callbacks[operation_id]
                callback(update)
    
    def get_progress(self, operation_id: str) -> Optional[ProgressUpdate]:
        """Get current progress for an operation."""
        with self._lock:
            return self._operations.get(operation_id)
    
    def get_all_progress(self) -> Dict[str, ProgressUpdate]:
        """Get progress for all operations."""
        with self._lock:
            return dict(self._operations)
    
    def complete_operation(self, operation_id: str):
        """Mark operation as complete."""
        with self._lock:
            if operation_id in self._operations:
                update = self._operations[operation_id]
                update.type = ProgressType.COMPLETE.value
    
    def register_callback(self, operation_id: str, callback: Callable):
        """Register callback for progress updates."""
        with self._lock:
            self._callbacks[operation_id] = callback
    
    def unregister_callback(self, operation_id: str):
        """Unregister callback."""
        with self._lock:
            if operation_id in self._callbacks:
                del self._callbacks[operation_id]


class StreamingToolChain:
    """
    Execute multiple tools in sequence with streaming results.
    """
    
    def __init__(self):
        """Initialize tool chain."""
        self._chain: list[tuple[str, dict]] = []
        self._results: Dict[str, Any] = {}
    
    def add_tool(self, tool_name: str, arguments: dict) -> "StreamingToolChain":
        """Add tool to chain."""
        self._chain.append((tool_name, arguments))
        return self
    
    def execute_chain(
        self,
        executor: StreamingExecutor,
        tool_executor: Any,
        progress_callback: Optional[Callable] = None,
    ) -> Generator[ProgressUpdate, None, None]:
        """
        Execute tools in chain with streaming.
        
        Yields:
            ProgressUpdate for each tool and chunk
        """
        for i, (tool_name, arguments) in enumerate(self._chain):
            if executor.is_cancelled():
                break
            
            tool_index = f"{i+1}/{len(self._chain)}"
            
            # Update message to show tool in chain
            original_callback = progress_callback
            
            def chain_callback(update: ProgressUpdate, idx=tool_index):
                update.message = f"[{idx}] {update.message}"
                if original_callback:
                    original_callback(update)
            
            # Execute tool and collect results
            results = []
            for update in executor.execute_with_streaming(
                tool_executor, tool_name, arguments, chain_callback
            ):
                yield update
                if update.type == ProgressType.CHUNK.value and update.data:
                    results.append(update.data)
            
            # Store results for next tool
            self._results[tool_name] = "".join(results)
    
    def get_results(self) -> Dict[str, Any]:
        """Get results from chain execution."""
        return self._results


def create_progress_stream(
    tool_executor: Any,
    tool_name: str,
    arguments: dict,
    max_chunk_size: int = 1024,
) -> Generator[ProgressUpdate, None, None]:
    """
    Convenience function to create streaming execution.
    
    Args:
        tool_executor: Tool executor instance
        tool_name: Name of tool to execute
        arguments: Tool arguments
        max_chunk_size: Maximum chunk size
        
    Yields:
        ProgressUpdate objects
    """
    executor = StreamingExecutor(max_chunk_size=max_chunk_size)
    yield from executor.execute_with_streaming(tool_executor, tool_name, arguments)
