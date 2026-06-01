# Improvement and Enhancement Suggestions

## Document Purpose
This document identifies areas for improvement, enhancement opportunities, and technical debt to address during or after the integration. These suggestions are informed by the architectural review of both AgentX and Agentix.

---

## 1. High-Priority Improvements

### 1.1 Unified Ollama Client Interface

**Current Issue:**
- AgentX uses `ollama` Python library (synchronous, native streaming)
- Agentix uses `requests` library (REST, no native streaming)
- Inconsistent error handling, timeout configuration, and retry logic

**Proposed Enhancement:**

Create a unified client interface that abstracts the backend:

```python
# src/shared/ollama_client/interface.py

from abc import ABC, abstractmethod
from typing import AsyncIterator, Iterator, Optional
from dataclasses import dataclass


@dataclass
class OllamaMessage:
    role: str
    content: str
    thinking: Optional[str] = None
    tool_calls: Optional[list] = None


class IOllamaClient(ABC):
    """Unified interface for Ollama communication"""
    
    @abstractmethod
    def chat(
        self, 
        model: str, 
        messages: list[dict],
        stream: bool = True
    ) -> Iterator[OllamaMessage]:
        """Synchronous chat with optional streaming"""
        
    @abstractmethod
    async def chat_async(
        self,
        model: str,
        messages: list[dict],
        stream: bool = True
    ) -> AsyncIterator[OllamaMessage]:
        """Asynchronous chat with optional streaming"""
    
    @abstractmethod
    def list_models(self) -> list[dict]:
        """List available models"""
    
    @abstractmethod
    async def list_models_async(self) -> list[dict]:
        """List models asynchronously"""


# Implementation using REST (recommended for flexibility)
class RestOllamaClient(IOllamaClient):
    """REST-based implementation with httpx"""
    
    def __init__(self, host: str = "localhost:11434", timeout: int = 300):
        self.base_url = f"http://{host}"
        self.timeout = timeout
        
    async def chat_async(self, model, messages, stream=True):
        import httpx
        async with httpx.AsyncClient(timeout=self.timeout) as client:
            async with client.stream(
                "POST",
                f"{self.base_url}/api/chat",
                json={"model": model, "messages": messages, "stream": stream}
            ) as response:
                async for line in response.aiter_lines():
                    if line:
                        yield self._parse_response(line)
```

**Benefits:**
- Consistent behavior across both projects
- Easy to swap implementations
- Testable with mock implementations
- Future-proof for other backends (OpenAI, Anthropic)

---

### 1.2 Enhanced Error Handling

**Current Issue:**
- Basic try/catch with generic error messages
- No retry logic for transient failures
- Errors not surfaced to GUI appropriately

**Proposed Enhancement:**

```python
# src/shared/errors.py

from enum import Enum
from dataclasses import dataclass
from typing import Optional


class ErrorCategory(Enum):
    NETWORK = "network"          # Connection issues
    TIMEOUT = "timeout"          # Request timeout
    API = "api"                  # Invalid API response
    MODEL = "model"              # Model not found/loaded
    TOOL = "tool"                # Tool execution failure
    CONTEXT = "context"          # Context too large
    CLASSIFICATION = "classify"  # Intent classification failure


@dataclass
class AgentError(Exception):
    """Structured error for agent operations"""
    category: ErrorCategory
    message: str
    recoverable: bool
    details: Optional[dict] = None
    
    def to_user_message(self) -> str:
        """Format for user display"""
        messages = {
            ErrorCategory.NETWORK: "Unable to connect to AI service. Check your connection.",
            ErrorCategory.TIMEOUT: "Request timed out. The model may be loading.",
            ErrorCategory.MODEL: "Model not available. Try selecting a different model.",
            ErrorCategory.CONTEXT: "Conversation too long. Consider starting a new session.",
        }
        return messages.get(self.category, self.message)
    
    def should_retry(self) -> bool:
        """Determine if operation should be retried"""
        return self.recoverable and self.category in [
            ErrorCategory.NETWORK,
            ErrorCategory.TIMEOUT,
        ]


# Retry decorator
def with_retry(max_attempts: int = 3, backoff: float = 1.0):
    """Decorator for automatic retry on recoverable errors"""
    def decorator(func):
        async def wrapper(*args, **kwargs):
            last_error = None
            for attempt in range(max_attempts):
                try:
                    return await func(*args, **kwargs)
                except AgentError as e:
                    last_error = e
                    if not e.should_retry() or attempt == max_attempts - 1:
                        raise
                    await asyncio.sleep(backoff * (attempt + 1))
            raise last_error
        return wrapper
    return decorator
```

---

### 1.3 Proper Token Counting

**Current Issue:**
- Agentix uses rough estimate: `len(content) // 4`
- No model-specific tokenization
- May truncate context unnecessarily or overflow

**Proposed Enhancement:**

```python
# src/shared/tokenizer.py

from typing import Protocol, Optional
import tiktoken  # For OpenAI-compatible counting


class ITokenizer(Protocol):
    """Interface for token counting"""
    
    def count_tokens(self, text: str) -> int:
        """Count tokens in text"""
        ...
    
    def count_messages(self, messages: list[dict]) -> int:
        """Count tokens in message list"""
        ...


class OllamaTokenizer:
    """Token counter for Ollama models"""
    
    # Approximate characters per token by model family
    MODEL_RATIOS = {
        "llama": 3.5,
        "mistral": 3.8,
        "phi": 4.0,
        "default": 4.0,
    }
    
    def __init__(self, model: str):
        self.model = model
        self.ratio = self._get_ratio(model)
    
    def _get_ratio(self, model: str) -> float:
        for family, ratio in self.MODEL_RATIOS.items():
            if family in model.lower():
                return ratio
        return self.MODEL_RATIOS["default"]
    
    def count_tokens(self, text: str) -> int:
        return int(len(text) / self.ratio)
    
    def count_messages(self, messages: list[dict]) -> int:
        total = 0
        for msg in messages:
            # Add overhead for message structure
            total += 4  # role, content keys
            total += self.count_tokens(msg.get("content", ""))
            total += self.count_tokens(msg.get("role", ""))
        return total


class SmartContextTrimmer:
    """Intelligent context trimming that preserves important messages"""
    
    def __init__(self, tokenizer: ITokenizer, max_tokens: int):
        self.tokenizer = tokenizer
        self.max_tokens = max_tokens
    
    def trim(self, messages: list[dict]) -> list[dict]:
        """
        Trim messages to fit within token limit.
        
        Strategy:
        1. Always keep system messages
        2. Always keep most recent user message
        3. Trim from middle, keeping recent context
        """
        if not messages:
            return messages
            
        # Separate by priority
        system_msgs = [m for m in messages if m.get("role") == "system"]
        other_msgs = [m for m in messages if m.get("role") != "system"]
        
        # Calculate system token budget
        system_tokens = self.tokenizer.count_messages(system_msgs)
        available = self.max_tokens - system_tokens
        
        # Trim other messages from oldest, keeping newest
        trimmed = []
        current_tokens = 0
        
        for msg in reversed(other_msgs):
            msg_tokens = self.tokenizer.count_messages([msg])
            if current_tokens + msg_tokens <= available:
                trimmed.insert(0, msg)
                current_tokens += msg_tokens
            else:
                break
        
        return system_msgs + trimmed
```

---

## 2. Medium-Priority Improvements

### 2.1 Plugin Architecture for Tools

**Current Issue:**
- Tools are hardcoded in `agentix/tools/`
- Adding new tools requires code changes
- No standard tool interface

**Proposed Enhancement:**

```python
# src/shared/tools/interface.py

from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Any, Optional


@dataclass
class ToolDefinition:
    """Standard tool definition"""
    name: str
    description: str
    parameters: dict  # JSON Schema
    returns: dict     # JSON Schema for return type
    

@dataclass
class ToolResult:
    """Standard tool result"""
    success: bool
    output: Any
    error: Optional[str] = None
    execution_time_ms: Optional[int] = None


class ITool(ABC):
    """Interface for pluggable tools"""
    
    @property
    @abstractmethod
    def definition(self) -> ToolDefinition:
        """Return tool definition for LLM"""
        ...
    
    @abstractmethod
    async def execute(self, **kwargs) -> ToolResult:
        """Execute tool with given arguments"""
        ...
    
    @abstractmethod
    def validate_input(self, **kwargs) -> bool:
        """Validate input parameters"""
        ...


# Tool registry
class ToolRegistry:
    """Registry for discovering and managing tools"""
    
    _tools: dict[str, ITool] = {}
    
    @classmethod
    def register(cls, tool: ITool) -> None:
        """Register a tool"""
        cls._tools[tool.definition.name] = tool
    
    @classmethod
    def get(cls, name: str) -> Optional[ITool]:
        """Get tool by name"""
        return cls._tools.get(name)
    
    @classmethod
    def list_definitions(cls) -> list[ToolDefinition]:
        """List all tool definitions"""
        return [t.definition for t in cls._tools.values()]
    
    @classmethod
    def discover_plugins(cls, path: str) -> None:
        """Auto-discover tools from plugin directory"""
        # Load Python files, find ITool implementations
```

**Plugin Example:**

```python
# plugins/file_tools.py

from shared.tools.interface import ITool, ToolDefinition, ToolResult


class ReadFileTool(ITool):
    """Tool to read file contents"""
    
    @property
    def definition(self) -> ToolDefinition:
        return ToolDefinition(
            name="read_file",
            description="Read contents of a file",
            parameters={
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "File path"}
                },
                "required": ["path"]
            },
            returns={"type": "string"}
        )
    
    async def execute(self, path: str) -> ToolResult:
        try:
            with open(path, 'r') as f:
                return ToolResult(success=True, output=f.read())
        except Exception as e:
            return ToolResult(success=False, output=None, error=str(e))
    
    def validate_input(self, path: str = None) -> bool:
        return path is not None and isinstance(path, str)
```

---

### 2.2 Session Export/Import

**Current Issue:**
- Sessions are local files only
- No way to share or backup sessions
- Limited session metadata

**Proposed Enhancement:**

```python
# src/shared/session_io.py

import json
import zipfile
from pathlib import Path
from dataclasses import dataclass
from datetime import datetime


@dataclass
class SessionMetadata:
    """Metadata for session export"""
    session_id: str
    user: str
    created: datetime
    last_modified: datetime
    model: str
    message_count: int
    version: str = "1.0"


class SessionExporter:
    """Export sessions to portable format"""
    
    def export_to_zip(self, session_path: str, output_path: str) -> str:
        """
        Export session to ZIP archive.
        
        Contents:
        - metadata.json
        - messages/*.json
        - attachments/*
        """
        session = Path(session_path)
        
        with zipfile.ZipFile(output_path, 'w', zipfile.ZIP_DEFLATED) as zf:
            # Write metadata
            metadata = self._build_metadata(session)
            zf.writestr("metadata.json", json.dumps(metadata.__dict__, default=str))
            
            # Write messages
            for msg_file in (session / "context").glob("*.json"):
                zf.write(msg_file, f"messages/{msg_file.name}")
            
            # Write attachments (referenced in messages)
            # ...
        
        return output_path
    
    def export_to_markdown(self, session_path: str, output_path: str) -> str:
        """Export session as readable Markdown"""
        # Format as conversation transcript
        pass


class SessionImporter:
    """Import sessions from exported format"""
    
    def import_from_zip(self, zip_path: str, target_dir: str) -> str:
        """Import session from ZIP archive"""
        pass
    
    def validate_archive(self, zip_path: str) -> bool:
        """Validate archive format and version"""
        pass
```

---

### 2.3 Configuration Validation

**Current Issue:**
- Configuration loaded without validation
- Invalid values cause runtime errors
- No schema documentation

**Proposed Enhancement:**

```python
# src/shared/config/validated_config.py

from pydantic import BaseModel, Field, validator
from typing import Optional, List
from enum import Enum


class ScreenSide(str, Enum):
    LEFT = "left"
    RIGHT = "right"


class AgentXConfig(BaseModel):
    """Validated AgentX configuration"""
    
    ollama_host: str = Field(
        default="localhost:11434",
        description="Ollama server host:port"
    )
    
    ollama_model: str = Field(
        default="llama3.2",
        description="Default model to use"
    )
    
    ollama_initial_load_timeout_seconds: int = Field(
        default=120,
        ge=10,
        le=600,
        description="Timeout for initial model load"
    )
    
    screen_side: ScreenSide = Field(
        default=ScreenSide.LEFT,
        description="Which side of screen to position window"
    )
    
    @validator('ollama_host')
    def validate_host(cls, v):
        if ':' not in v:
            raise ValueError("Host must include port (e.g., localhost:11434)")
        return v


class AgentixConfig(BaseModel):
    """Validated Agentix configuration"""
    
    enabled: bool = Field(
        default=True,
        description="Enable Agentix integration"
    )
    
    classify_prompts: bool = Field(
        default=True,
        description="Classify all prompts before processing"
    )
    
    available_tools: List[str] = Field(
        default=["cst", "ast"],
        description="Tools to make available"
    )
    
    show_classification: bool = Field(
        default=True,
        description="Show classification in GUI"
    )


class UnifiedConfig(BaseModel):
    """Complete validated configuration"""
    
    agentx: AgentXConfig = Field(default_factory=AgentXConfig)
    agentix: AgentixConfig = Field(default_factory=AgentixConfig)
    
    class Config:
        # Generate JSON schema for documentation
        schema_extra = {
            "title": "AgentX Configuration",
            "description": "Configuration for AgentX + Agentix integration"
        }
```

---

## 3. Lower-Priority Improvements (Nice-to-Have)

### 3.1 GUI Theme System

**Current Issue:**
- Colors hardcoded in GUIManager
- No light/dark mode toggle
- Not configurable

**Proposed Enhancement:**

```python
# src/agentx/themes.py

from dataclasses import dataclass


@dataclass
class Theme:
    """GUI color theme"""
    name: str
    
    # Backgrounds
    bg_primary: str
    bg_secondary: str
    bg_input: str
    
    # Text
    text_primary: str
    text_secondary: str
    text_error: str
    
    # Accents
    accent_primary: str
    accent_secondary: str
    
    # Message-specific
    user_message_bg: str
    assistant_message_bg: str
    thinking_text: str


DARK_THEME = Theme(
    name="dark",
    bg_primary="#222222",
    bg_secondary="#333333",
    bg_input="#222222",
    text_primary="#eeeeee",
    text_secondary="#cccccc",
    text_error="#ff4444",
    accent_primary="#3399ff",
    accent_secondary="#666666",
    user_message_bg="#333333",
    assistant_message_bg="#2a2a2a",
    thinking_text="#cccccc",
)

LIGHT_THEME = Theme(
    name="light",
    bg_primary="#ffffff",
    bg_secondary="#f5f5f5",
    bg_input="#ffffff",
    text_primary="#222222",
    text_secondary="#666666",
    text_error="#cc0000",
    accent_primary="#0066cc",
    accent_secondary="#cccccc",
    user_message_bg="#f0f0f0",
    assistant_message_bg="#fafafa",
    thinking_text="#888888",
)
```

---

### 3.2 Keyboard Shortcuts

**Current Issue:**
- Limited keyboard navigation
- No shortcut customization

**Proposed Enhancement:**

```python
# src/agentx/keybindings.py

DEFAULT_BINDINGS = {
    "submit": "<Return>",           # Submit on Enter
    "submit_multiline": "<Shift-Return>",  # Newline
    "interrupt": "<Escape>",        # Stop streaming
    "new_session": "<Control-n>",   # New session
    "toggle_sidebar": "<Control-b>", # Toggle file explorer
    "focus_input": "<Control-l>",   # Focus input field
}


class KeybindingManager:
    """Manage keyboard shortcuts"""
    
    def __init__(self, root, bindings=None):
        self.root = root
        self.bindings = bindings or DEFAULT_BINDINGS.copy()
        self.handlers = {}
    
    def bind(self, action: str, handler):
        """Bind action to handler"""
        self.handlers[action] = handler
        key = self.bindings.get(action)
        if key:
            self.root.bind(key, lambda e: handler())
    
    def rebind(self, action: str, new_key: str):
        """Change keybinding for action"""
        old_key = self.bindings.get(action)
        if old_key:
            self.root.unbind(old_key)
        self.bindings[action] = new_key
        if action in self.handlers:
            self.root.bind(new_key, lambda e: self.handlers[action]())
```

---

### 3.3 Conversation Search

**Current Issue:**
- No way to search conversation history
- Cannot find messages across sessions

**Proposed Enhancement:**

```python
# src/agentx/search.py

from dataclasses import dataclass
from typing import List, Optional
import re


@dataclass
class SearchResult:
    """Search result with context"""
    session_id: str
    message_index: int
    role: str
    content_snippet: str
    match_positions: List[tuple[int, int]]
    timestamp: str


class ConversationSearcher:
    """Search through conversation history"""
    
    def search(
        self,
        query: str,
        sessions: List[str],
        role_filter: Optional[str] = None,
        regex: bool = False
    ) -> List[SearchResult]:
        """
        Search for query in sessions.
        
        Args:
            query: Search term or regex
            sessions: Session paths to search
            role_filter: Only search messages with this role
            regex: Treat query as regex
        """
        results = []
        
        pattern = re.compile(query, re.IGNORECASE) if regex else None
        
        for session_path in sessions:
            for msg_idx, message in enumerate(self._load_messages(session_path)):
                if role_filter and message.role != role_filter:
                    continue
                    
                matches = self._find_matches(message.content, query, pattern)
                if matches:
                    results.append(SearchResult(
                        session_id=session_path,
                        message_index=msg_idx,
                        role=message.role,
                        content_snippet=self._get_snippet(message.content, matches[0]),
                        match_positions=matches,
                        timestamp=str(message.timestamp)
                    ))
        
        return results
```

---

## 4. Technical Debt to Address

### 4.1 Duplicate Message Classes

**Issue:** Both projects have `Message` classes with overlapping functionality.

**Files Affected:**
- `src/agentx/message.py`
- `src/agentix/context/message.py`

**Action:** Consolidate into `src/shared/models/message.py` during Phase 4.

---

### 4.2 Inconsistent Imports

**Issue:** Mixed relative and absolute imports across projects.

**Example:**
```python
# agentix/context/sessions.py
from agentix import Message  # Absolute
from ..agentix_config import AgentixConfig  # Relative
```

**Action:** Standardize on relative imports within packages, absolute for cross-package.

---

### 4.3 Missing Type Hints

**Issue:** Incomplete type annotations, especially in AgentX.

**Action:** Add comprehensive type hints during refactoring.

---

### 4.4 Test Coverage Gaps

**Issue:** Limited unit tests, no integration tests.

**Current Test Files:**
- `test_gui_manager_integration.py`
- `test_session_gui_integration.py`
- `test_smoke_workflows.py`

**Action:** Expand test coverage to >80% during Phase 5.

---

## 5. Enhancement Roadmap

### Short-term (Weeks 1-2)
- [ ] Unified Ollama client interface (1.1)
- [ ] Enhanced error handling (1.2)
- [ ] Configuration validation (2.3)

### Medium-term (Weeks 3-4)
- [ ] Proper token counting (1.3)
- [ ] Plugin architecture for tools (2.1)
- [ ] Session export/import (2.2)

### Long-term (Post-Integration)
- [ ] GUI theme system (3.1)
- [ ] Keyboard shortcuts (3.2)
- [ ] Conversation search (3.3)

---

## 6. Metrics for Success

| Improvement | Metric | Target |
|-------------|--------|--------|
| Unified client | Lines of duplicate code | < 100 |
| Error handling | User-facing error rate | < 5% |
| Token counting | Context overflow errors | 0 |
| Configuration | Invalid config crashes | 0 |
| Tools plugin | Time to add new tool | < 30 min |
| Test coverage | Code coverage | > 80% |

---

## Summary

This document has identified:
- **3 high-priority improvements** essential for integration
- **3 medium-priority enhancements** for better maintainability
- **3 nice-to-have features** for improved UX
- **4 technical debt items** to address during refactoring

These improvements should be prioritized based on:
1. Impact on integration success
2. User-facing value
3. Development effort required

Recommended order: 1.1 → 1.2 → 1.3 → 2.3 → 2.1 → 2.2 → 3.x
