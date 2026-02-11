# Phased Integration Plan: AgentX + Agentix

## Overview

This document provides a detailed, systematic, and phased approach to integrating AgentX (GUI frontend) with Agentix (agent middleware). Each phase builds upon the previous one and can be independently tested.

**Total Estimated Duration:** 4-6 weeks (depending on team size)

---

## Phase 0: Foundation & Preparation

### Objectives
- Establish shared infrastructure
- Create unified configuration
- Set up testing framework for integration

### Duration: 3-5 days

### Tasks

#### 0.1 Create Shared Module Structure
```
src/shared/
├── __init__.py
├── models/
│   ├── __init__.py
│   ├── message.py          # Unified Message model
│   ├── context.py          # Unified Context interface
│   ├── attachment.py       # Unified Attachment model
│   └── response.py         # Response chunk model
├── config/
│   ├── __init__.py
│   └── unified_config.py   # Combined AgentX + Agentix config
└── interfaces/
    ├── __init__.py
    └── protocols.py        # Protocol definitions
```

#### 0.2 Unify Configuration Loading

**Current Files:**
- `src/agentx/config.py` - Loads `agentx.toml`
- `src/agentix/agentix_config.py` - CLI args + TOML loading

**Action Items:**
```python
# src/shared/config/unified_config.py

@dataclass
class UnifiedConfig:
    """Combined configuration for AgentX and Agentix"""
    
    # AgentX GUI settings
    ollama_host: str = "localhost:11434"
    ollama_model: str = "llama3.2"
    screen_side: str = "left"
    
    # Agentix middleware settings
    classify_prompts: bool = True
    default_system_prompts: list[str] = field(default_factory=list)
    available_tools: list[str] = field(default_factory=lambda: ["cst", "ast"])
    
    # Integration settings
    show_classification: bool = True
    show_tool_calls: bool = True
    
    @classmethod
    def from_toml(cls, path: str) -> "UnifiedConfig":
        """Load from TOML file"""
        
    @classmethod
    def merge(cls, cli: dict, file: dict) -> "UnifiedConfig":
        """Merge CLI args with file config"""
```

#### 0.3 Create Integration Test Scaffold

**File:** `tests/integration/test_agentx_agentix.py`

```python
import pytest
from unittest.mock import AsyncMock, MagicMock

class TestAgentXAgentixIntegration:
    """Integration tests for AgentX + Agentix"""
    
    @pytest.fixture
    def mock_ollama_server(self):
        """Mock Ollama server responses"""
        
    @pytest.fixture
    def agentx_session(self, mock_ollama_server):
        """Create AgentX session with mocked backend"""
        
    def test_prompt_flows_through_agentix_classification(self):
        """Verify user prompts are classified by Agentix"""
        
    def test_tool_calls_rendered_in_gui(self):
        """Verify tool calls appear in AgentX output"""
        
    def test_model_list_populated_from_agentix(self):
        """Verify model selector uses Agentix model list"""
```

### Deliverables
- [ ] `src/shared/` module structure created
- [ ] Unified configuration class implemented
- [ ] Integration test scaffold in place
- [ ] Both projects import from shared module successfully

### Verification
```bash
# Run from project root
python -c "from shared.config import UnifiedConfig; print('OK')"
python -c "from agentx.main import main; print('AgentX OK')"
python -c "from agentix.main import main; print('Agentix OK')"
pytest tests/integration/ -v
```

---

## Phase 1: Agentix Programmatic API

### Objectives
- Expose Agentix functionality as a programmatic API (not just CLI)
- Add streaming support to API client
- Create bridge interface for AgentX consumption

### Duration: 5-7 days

### Tasks

#### 1.1 Create Agentix Bridge Class

**File:** `src/agentix/bridge.py`

```python
"""Programmatic API for Agentix consumption by AgentX"""

from typing import AsyncIterator, Optional
from dataclasses import dataclass

from .agentix_config import AgentixConfig
from .api_client import query_api_streaming
from .context.sessions import assemble_classification_prompt, manage_sessions
from .models import get_models, get_model
from .prompt_classification_response import PromptClassificationResponse
from shared.models import Context, Message, ResponseChunk


class AgentixBridge:
    """Bridge between AgentX GUI and Agentix middleware"""
    
    def __init__(self, config: AgentixConfig):
        self.config = config
        self._model_cache: Optional[list[dict]] = None
        
    async def classify_prompt(
        self, 
        prompt: str, 
        context: Context
    ) -> PromptClassificationResponse:
        """
        Classify user intent before processing.
        
        Args:
            prompt: User's input text
            context: Current conversation context
            
        Returns:
            Classification with intent and next_step
        """
        # Convert context to history format
        history = self._context_to_history(context)
        
        # Use existing classification logic
        classification_payload = assemble_classification_prompt(
            self.config, history, self._get_max_tokens()
        )
        
        result = query_api(self.config, classification_payload)
        return PromptClassificationResponse(**result)
    
    async def process_prompt_streaming(
        self,
        prompt: str,
        context: Context,
        classification: PromptClassificationResponse
    ) -> AsyncIterator[ResponseChunk]:
        """
        Process prompt through appropriate handler with streaming.
        
        Yields:
            ResponseChunk objects for GUI rendering
        """
        match classification.next_step:
            case NextStep.respond_directly:
                async for chunk in self._stream_direct_response(prompt, context):
                    yield chunk
            case NextStep.single_tool:
                async for chunk in self._stream_tool_response(prompt, context):
                    yield chunk
            case NextStep.invoke_planner:
                async for chunk in self._stream_planned_response(prompt, context):
                    yield chunk
            case NextStep.escalate:
                yield ResponseChunk(type="error", content="Escalation required")
    
    def get_available_models(self) -> list[dict]:
        """Fetch available models from Ollama"""
        if self._model_cache is None:
            self._model_cache = get_models(self.config)
        return self._model_cache
    
    def get_available_tools(self) -> list[dict]:
        """Return available MCP tools with metadata"""
        from .tools import extract_cst_tools
        from .tools.describe_tools import extract_tools_from_file, to_openai_tools
        
        tools = []
        for tool_name in self.config.tools or []:
            # ... extract tool definitions
        return tools
    
    def _context_to_history(self, context: Context) -> list[Message]:
        """Convert AgentX Context to Agentix history format"""
        return [msg for _, msg in context.messages if msg.enabled]
```

#### 1.2 Add Streaming Support to API Client

**File:** `src/agentix/api_client.py` (modifications)

```python
async def query_api_streaming(
    args: AgentixConfig, 
    payload: QueryPayload
) -> AsyncIterator[dict]:
    """
    Stream responses from Ollama API.
    
    Yields:
        Response chunks as dictionaries
    """
    import httpx
    
    headers = {"Content-Type": "application/json"}
    payload_dict = payload.to_dict()
    payload_dict["stream"] = True
    
    async with httpx.AsyncClient(timeout=300) as client:
        async with client.stream(
            "POST",
            f"{OLLAMA_API_BASE}{OLLAMA_CHAT_ENDPOINT}",
            headers=headers,
            json=payload_dict
        ) as response:
            async for line in response.aiter_lines():
                if line.strip():
                    yield json.loads(line)
```

#### 1.3 Create Response Chunk Model

**File:** `src/shared/models/response.py`

```python
from dataclasses import dataclass
from typing import Optional, Any
from enum import Enum


class ChunkType(Enum):
    CONTENT = "content"
    THINKING = "thinking"
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"
    CLASSIFICATION = "classification"
    ERROR = "error"


@dataclass
class ResponseChunk:
    """A chunk of streaming response for GUI consumption"""
    
    type: ChunkType
    content: str
    
    # Tool-specific fields
    tool_name: Optional[str] = None
    tool_input: Optional[dict] = None
    tool_output: Optional[Any] = None
    
    # Classification-specific fields
    classification: Optional[dict] = None
    
    def to_gui_dict(self) -> dict:
        """Format for GUI rendering"""
        return {
            "type": self.type.value,
            "content": self.content,
            "tool_name": self.tool_name,
            "tool_input": self.tool_input,
            "tool_output": self.tool_output,
        }
```

### Deliverables
- [ ] `AgentixBridge` class implemented with core methods
- [ ] Streaming API client (`query_api_streaming`) functional
- [ ] `ResponseChunk` model defined in shared module
- [ ] Unit tests for bridge methods

### Verification
```python
# Test script
from agentix.bridge import AgentixBridge
from agentix.agentix_config import AgentixConfig
from shared.models import Context

bridge = AgentixBridge(AgentixConfig())
models = bridge.get_available_models()
print(f"Found {len(models)} models")

# Test classification (requires running Ollama)
import asyncio
context = Context()
result = asyncio.run(bridge.classify_prompt("Hello, how are you?", context))
print(f"Classification: {result}")
```

---

## Phase 2: AgentX Integration Layer

### Objectives
- Integrate AgentixBridge into AgentXSession
- Route user prompts through Agentix classification
- Handle streaming responses through bridge

### Duration: 5-7 days

### Tasks

#### 2.1 Create Integration Module in AgentX

**File:** `src/agentx/integration/__init__.py`

```python
from .agentix_bridge_adapter import AgentixBridgeAdapter
from .response_handler import ResponseHandler

__all__ = ["AgentixBridgeAdapter", "ResponseHandler"]
```

**File:** `src/agentx/integration/agentix_bridge_adapter.py`

```python
"""Adapter to use AgentixBridge within AgentX session"""

import asyncio
from typing import Optional, AsyncIterator
from agentix.bridge import AgentixBridge
from agentix.agentix_config import AgentixConfig
from shared.models import Context, ResponseChunk


class AgentixBridgeAdapter:
    """Adapts AgentixBridge for use in AgentX's threaded model"""
    
    def __init__(self, config: dict):
        # Convert AgentX config to AgentixConfig
        agentix_config = self._convert_config(config)
        self.bridge = AgentixBridge(agentix_config)
        self._loop: Optional[asyncio.AbstractEventLoop] = None
        
    def _get_loop(self) -> asyncio.AbstractEventLoop:
        """Get or create event loop for async operations"""
        if self._loop is None or self._loop.is_closed():
            self._loop = asyncio.new_event_loop()
        return self._loop
    
    def classify_prompt_sync(self, prompt: str, context: Context):
        """Synchronous wrapper for classify_prompt"""
        loop = self._get_loop()
        return loop.run_until_complete(
            self.bridge.classify_prompt(prompt, context)
        )
    
    def process_prompt_generator(
        self, 
        prompt: str, 
        context: Context,
        classification
    ):
        """
        Generator that yields ResponseChunks.
        Compatible with AgentX's streaming loop.
        """
        loop = self._get_loop()
        
        async def collect_chunks():
            chunks = []
            async for chunk in self.bridge.process_prompt_streaming(
                prompt, context, classification
            ):
                chunks.append(chunk)
            return chunks
        
        # For now, collect all chunks then yield
        # TODO: True streaming with threading
        chunks = loop.run_until_complete(collect_chunks())
        for chunk in chunks:
            yield chunk
    
    def get_models(self) -> list[dict]:
        """Get available models"""
        return self.bridge.get_available_models()
    
    def _convert_config(self, agentx_config: dict) -> AgentixConfig:
        """Convert AgentX config dict to AgentixConfig"""
        return AgentixConfig(
            model=agentx_config.get("agentx", {}).get("ollama_model"),
            debug=agentx_config.get("debug", False),
            # Map other fields as needed
        )
```

#### 2.2 Modify AgentXSession to Use Bridge

**File:** `src/agentx/session.py` (modifications)

```python
# Add to imports
from .integration import AgentixBridgeAdapter

class AgentXSession:
    def __init__(self, root: tk.Tk, config: dict[str, Any]):
        # ... existing init code ...
        
        # Initialize Agentix bridge
        self.agentix_bridge = AgentixBridgeAdapter(config)
    
    def stream_ollama_response_worker(self):
        """Enhanced worker that routes through Agentix"""
        
        # ... existing setup code ...
        
        prompt = self.gui.get_user_input()
        
        if self.agentix_bridge:
            # Route through Agentix classification
            classification = self.agentix_bridge.classify_prompt_sync(
                prompt, self.context
            )
            
            # Display classification in GUI (optional)
            if self.config.get("agentix", {}).get("show_classification", True):
                self.gui.display_classification(classification)
            
            # Process through Agentix
            for chunk in self.agentix_bridge.process_prompt_generator(
                prompt, self.context, classification
            ):
                if not self._is_streaming.is_set():
                    break
                self._handle_response_chunk(chunk)
        else:
            # Fall back to direct Ollama communication
            # ... existing ollama.Client code ...
    
    def _handle_response_chunk(self, chunk: ResponseChunk):
        """Handle a response chunk from Agentix"""
        match chunk.type:
            case ChunkType.CONTENT:
                self.gui.display_agent_response(chunk.content)
            case ChunkType.THINKING:
                self.gui.display_agent_thinking(chunk.content)
            case ChunkType.TOOL_CALL:
                self.gui.display_tool_call(
                    chunk.tool_name, 
                    chunk.tool_input
                )
            case ChunkType.TOOL_RESULT:
                self.gui.display_tool_result(
                    chunk.tool_name,
                    chunk.tool_output
                )
            case ChunkType.ERROR:
                self.gui.display_error(chunk.content)
```

#### 2.3 Add GUI Methods for New Display Types

**File:** `src/agentx/gui_manager.py` (additions)

```python
class GUIManager(IGUIManager):
    # ... existing code ...
    
    def display_classification(self, classification) -> None:
        """Display intent classification in status area"""
        intent_text = f"Intent: {classification.intent} | Next: {classification.next_step}"
        # Update status bar or info area
        self._update_status_bar(intent_text)
    
    def display_tool_call(self, tool_name: str, tool_input: dict) -> None:
        """Display a tool invocation in the output"""
        tool_header = f"\n{self.MESSAGE_ROLES['tools']}\t[Calling: {tool_name}]\n"
        self._append_to_output(tool_header, tag="tool_call")
        
        # Show input as collapsible
        input_text = json.dumps(tool_input, indent=2)
        self._append_collapsible(f"Input: {input_text}", tag="tool_input")
    
    def display_tool_result(self, tool_name: str, tool_output) -> None:
        """Display tool execution result"""
        result_header = f"[Result from: {tool_name}]\n"
        self._append_to_output(result_header, tag="tool_result")
        
        # Format output based on type
        if isinstance(tool_output, dict):
            output_text = json.dumps(tool_output, indent=2)
        else:
            output_text = str(tool_output)
        
        self._append_collapsible(output_text, tag="tool_output")
```

### Deliverables
- [ ] `AgentixBridgeAdapter` implemented
- [ ] `AgentXSession.stream_ollama_response_worker()` modified to use bridge
- [ ] New GUI methods for tool display
- [ ] Configuration flag to enable/disable Agentix integration

### Verification
```bash
# Launch AgentX with Agentix integrated
python -m agentx

# Type a prompt and verify:
# 1. Classification appears in status bar
# 2. Response streams through Agentix
# 3. No regressions in basic chat functionality
```

---

## Phase 3: Model Selection & Tool Display

### Objectives
- Add model selector to system status bar
- Display available tools with enable/disable toggles
- Render tool calls as message objects in context

### Duration: 4-5 days

### Tasks

#### 3.1 Create Model Selector Widget

**File:** `src/agentx/integration/model_selector.py`

```python
"""Model selector widget backed by Agentix"""

import tkinter as tk
from tkinter import ttk
from typing import Callable, Optional


class ModelSelector:
    """Dropdown for selecting Ollama model"""
    
    def __init__(
        self, 
        parent: tk.Widget,
        on_model_change: Callable[[str], None],
        initial_model: str = ""
    ):
        self.parent = parent
        self.on_model_change = on_model_change
        self.current_model = tk.StringVar(value=initial_model)
        
        self.frame = ttk.Frame(parent)
        self.label = ttk.Label(self.frame, text="Model:")
        self.dropdown = ttk.Combobox(
            self.frame,
            textvariable=self.current_model,
            state="readonly",
            width=20
        )
        self.dropdown.bind("<<ComboboxSelected>>", self._on_selection)
        
        # Layout
        self.label.pack(side=tk.LEFT, padx=(0, 5))
        self.dropdown.pack(side=tk.LEFT)
    
    def populate(self, models: list[dict]) -> None:
        """Populate dropdown with model names"""
        model_names = [m.get("name", str(m)) for m in models]
        self.dropdown["values"] = model_names
        
        # Select first if no current selection
        if not self.current_model.get() and model_names:
            self.current_model.set(model_names[0])
    
    def _on_selection(self, event) -> None:
        """Handle model selection change"""
        selected = self.current_model.get()
        self.on_model_change(selected)
    
    def get_widget(self) -> tk.Widget:
        """Return the widget for packing"""
        return self.frame
```

#### 3.2 Add Tool Management Panel

**File:** `src/agentx/integration/tool_panel.py`

```python
"""Panel for viewing and toggling available tools"""

import tkinter as tk
from tkinter import ttk
from typing import Callable


class ToolPanel:
    """Display available tools with enable/disable toggles"""
    
    def __init__(
        self,
        parent: tk.Widget,
        on_tool_toggle: Callable[[str, bool], None]
    ):
        self.parent = parent
        self.on_tool_toggle = on_tool_toggle
        self.tool_vars: dict[str, tk.BooleanVar] = {}
        
        self.frame = ttk.LabelFrame(parent, text="Available Tools")
        self.tools_container = ttk.Frame(self.frame)
        self.tools_container.pack(fill=tk.BOTH, expand=True, padx=5, pady=5)
    
    def populate(self, tools: list[dict]) -> None:
        """Populate with tool definitions"""
        # Clear existing
        for widget in self.tools_container.winfo_children():
            widget.destroy()
        self.tool_vars.clear()
        
        for tool in tools:
            name = tool.get("name", "Unknown")
            description = tool.get("description", "")
            
            var = tk.BooleanVar(value=True)  # Enabled by default
            self.tool_vars[name] = var
            
            row = ttk.Frame(self.tools_container)
            
            checkbox = ttk.Checkbutton(
                row,
                text=name,
                variable=var,
                command=lambda n=name, v=var: self.on_tool_toggle(n, v.get())
            )
            
            desc_label = ttk.Label(
                row, 
                text=f"  - {description[:50]}...",
                foreground="gray"
            )
            
            checkbox.pack(side=tk.LEFT)
            desc_label.pack(side=tk.LEFT)
            row.pack(fill=tk.X, anchor=tk.W)
    
    def get_enabled_tools(self) -> list[str]:
        """Return list of enabled tool names"""
        return [name for name, var in self.tool_vars.items() if var.get()]
    
    def get_widget(self) -> tk.Widget:
        return self.frame
```

#### 3.3 Integrate into GUIManager

**File:** `src/agentx/gui_manager.py` (modifications)

```python
from .integration.model_selector import ModelSelector
from .integration.tool_panel import ToolPanel

class GUIManager(IGUIManager):
    def __init__(self, ...):
        # ... existing init ...
        
        # New components (initialized in create_layout)
        self.model_selector: Optional[ModelSelector] = None
        self.tool_panel: Optional[ToolPanel] = None
    
    def create_layout(self):
        """Enhanced layout with model selector and tool panel"""
        # ... existing layout code ...
        
        # Add to system status bar
        self.model_selector = ModelSelector(
            self.widgets.get("status_frame"),
            on_model_change=self._on_model_change
        )
        self.model_selector.get_widget().pack(side=tk.LEFT, padx=10)
        
        # Add tool panel to Session tab
        self.tool_panel = ToolPanel(
            self.widgets.get("session_tab"),
            on_tool_toggle=self._on_tool_toggle
        )
        self.tool_panel.get_widget().pack(fill=tk.X, pady=5)
    
    def populate_model_selector(self, models: list[dict]) -> None:
        """Populate model selector with available models"""
        if self.model_selector:
            self.model_selector.populate(models)
    
    def populate_tool_panel(self, tools: list[dict]) -> None:
        """Populate tool panel with available tools"""
        if self.tool_panel:
            self.tool_panel.populate(tools)
```

#### 3.4 Store Tool Calls as Messages

**File:** `src/agentx/message.py` (extensions)

```python
# Add new roles
TOOL_CALL = "tool_call"
TOOL_RESULT = "tool_result"

ROLES = {
    USER: "👤",
    ASSISTANT: "🤖",
    SYSTEM: "⚙️",
    TOOL_CALL: "🔧",
    TOOL_RESULT: "📋",
}

@dataclass
class ToolCallMessage(Message):
    """Message representing a tool invocation"""
    
    tool_name: str = ""
    tool_input: dict = field(default_factory=dict)
    
    def __init__(self, tool_name: str, tool_input: dict, **kwargs):
        super().__init__(role=TOOL_CALL, content="", **kwargs)
        self.tool_name = tool_name
        self.tool_input = tool_input
        self.content = f"Calling {tool_name}"
    
    def serialize(self) -> dict:
        data = super().serialize()
        data["tool_name"] = self.tool_name
        data["tool_input"] = self.tool_input
        return data


@dataclass  
class ToolResultMessage(Message):
    """Message representing tool execution result"""
    
    tool_name: str = ""
    tool_output: Any = None
    
    def __init__(self, tool_name: str, tool_output: Any, **kwargs):
        super().__init__(role=TOOL_RESULT, content="", **kwargs)
        self.tool_name = tool_name
        self.tool_output = tool_output
        self.content = f"Result from {tool_name}"
```

### Deliverables
- [ ] `ModelSelector` widget implemented and integrated
- [ ] `ToolPanel` widget implemented and integrated
- [ ] `ToolCallMessage` and `ToolResultMessage` classes created
- [ ] Tool calls stored in session context
- [ ] Model selection persists to session configuration

### Verification
```bash
# Launch AgentX
python -m agentx

# Verify:
# 1. Model dropdown appears in status bar with available models
# 2. Tools panel shows available tools with descriptions
# 3. Tool toggles work (check console or debug output)
# 4. When a tool is called, it appears in the context panel
```

---

## Phase 4: Context Unification

### Objectives
- Unify Message and Context models between projects
- Ensure Agentix reads from AgentX session storage
- Implement cross-session context awareness

### Duration: 4-5 days

### Tasks

#### 4.1 Create Unified Message Model

**File:** `src/shared/models/message.py`

```python
"""Unified Message model for AgentX and Agentix"""

from dataclasses import dataclass, field
from datetime import datetime
from typing import Any, Optional
from enum import Enum
import json


class MessageRole(Enum):
    USER = "user"
    ASSISTANT = "assistant"
    SYSTEM = "system"
    THINKING = "thinking"
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"


@dataclass
class UnifiedMessage:
    """Message that works with both AgentX GUI and Agentix middleware"""
    
    role: MessageRole
    content: str
    enabled: bool = True
    
    # Metadata
    timestamp: datetime = field(default_factory=datetime.now)
    file_path: Optional[str] = None
    
    # Attachments
    attachments: list = field(default_factory=list)
    
    # Classification (for user messages)
    classification: Optional[dict] = None
    
    # Tool-specific
    tool_name: Optional[str] = None
    tool_input: Optional[dict] = None
    tool_output: Optional[Any] = None
    
    def to_llm_dict(self) -> dict:
        """Format for Ollama/OpenAI API"""
        msg = {
            "role": self.role.value if self.role != MessageRole.THINKING else "assistant",
            "content": self.content,
        }
        
        # Add attachment content
        if self.attachments:
            attachment_content = "\n\n".join(
                f"[Attachment: {a.file_path}]\n{a.content}"
                for a in self.attachments
                if a.enabled
            )
            msg["content"] = f"{msg['content']}\n\n{attachment_content}"
        
        return msg
    
    def to_gui_dict(self) -> dict:
        """Format for GUI rendering"""
        return {
            "role": self.role.value,
            "content": self.content,
            "enabled": self.enabled,
            "timestamp": self.timestamp.isoformat(),
            "attachments": [a.to_dict() for a in self.attachments],
            "tool_name": self.tool_name,
            "classification": self.classification,
        }
    
    def serialize(self) -> dict:
        """Serialize for file storage"""
        return {
            "role": self.role.value,
            "content": self.content,
            "enabled": self.enabled,
            "epoch": self.timestamp.timestamp(),
            "file": self.file_path,
            "attachments": [a.serialize() for a in self.attachments],
            "classification": self.classification,
            "tool_name": self.tool_name,
            "tool_input": self.tool_input,
            "tool_output": str(self.tool_output) if self.tool_output else None,
        }
    
    @classmethod
    def from_dict(cls, data: dict) -> "UnifiedMessage":
        """Deserialize from stored data"""
        return cls(
            role=MessageRole(data.get("role", "user")),
            content=data.get("content", ""),
            enabled=data.get("enabled", True),
            timestamp=datetime.fromtimestamp(data.get("epoch", 0)),
            file_path=data.get("file"),
            # ... other fields
        )
```

#### 4.2 Create Unified Context Interface

**File:** `src/shared/models/context.py`

```python
"""Unified Context interface"""

from typing import Protocol, Iterator, Optional
from datetime import datetime
from .message import UnifiedMessage


class IContext(Protocol):
    """Interface for context management"""
    
    @property
    def messages(self) -> list[tuple[datetime, UnifiedMessage]]:
        """All messages in context"""
        ...
    
    def add_message(self, message: UnifiedMessage) -> None:
        """Add message to context"""
        ...
    
    def get_enabled_messages(self) -> Iterator[UnifiedMessage]:
        """Iterate over enabled messages"""
        ...
    
    def save(self) -> None:
        """Persist context to storage"""
        ...
    
    def load(self, path: str) -> None:
        """Load context from storage"""
        ...


class UnifiedContext:
    """Concrete implementation of unified context"""
    
    def __init__(self, path: Optional[str] = None):
        self._messages: list[tuple[datetime, UnifiedMessage]] = []
        self.path = path
        self.session_id: Optional[str] = None
        self.expanded: bool = True  # GUI state
    
    @property
    def messages(self) -> list[tuple[datetime, UnifiedMessage]]:
        return self._messages
    
    def add_message(self, message: UnifiedMessage) -> None:
        ts = message.timestamp
        self._messages.append((ts, message))
        if self.path:
            message.save(self.path)
    
    def get_enabled_messages(self) -> Iterator[UnifiedMessage]:
        for _, msg in self._messages:
            if msg.enabled:
                yield msg
    
    def to_llm_messages(self) -> list[dict]:
        """Format all enabled messages for LLM"""
        return [msg.to_llm_dict() for msg in self.get_enabled_messages()]
```

#### 4.3 Migrate AgentX to Use Unified Models

**File:** Update imports in `src/agentx/session.py`

```python
# Replace
from .message import Message
from .context import Context

# With
from shared.models import UnifiedMessage as Message, UnifiedContext as Context
```

#### 4.4 Configure Agentix to Reference AgentX Sessions

**File:** `src/agentix/context/sessions.py` (modifications)

```python
from shared.models import UnifiedContext

def load_agentx_session(session_path: str) -> UnifiedContext:
    """Load an AgentX session as a UnifiedContext"""
    context = UnifiedContext(path=session_path)
    context.load(session_path)
    return context


def manage_sessions(args: AgentixConfig) -> list[UnifiedMessage]:
    """
    Enhanced session management that can reference AgentX sessions.
    """
    # Check if we're working with a frontend (AgentX)
    if args.with_frontend:
        # Session path would be passed from AgentX
        if args.session_path:
            context = load_agentx_session(args.session_path)
            return list(context.get_enabled_messages())
    
    # Fall back to original CLI session management
    # ... existing code ...
```

### Deliverables
- [ ] `UnifiedMessage` model in shared module
- [ ] `UnifiedContext` class in shared module
- [ ] AgentX migrated to use shared models
- [ ] Agentix configured to load AgentX sessions
- [ ] All existing tests pass with new models

### Verification
```python
# Test unified models
from shared.models import UnifiedMessage, MessageRole, UnifiedContext

# Create message
msg = UnifiedMessage(
    role=MessageRole.USER,
    content="Test message"
)

# Verify LLM format
assert msg.to_llm_dict() == {"role": "user", "content": "Test message"}

# Test context
ctx = UnifiedContext()
ctx.add_message(msg)
assert len(list(ctx.get_enabled_messages())) == 1
```

---

## Phase 5: Testing & Refinement

### Objectives
- Comprehensive integration testing
- Performance optimization
- Documentation and code cleanup

### Duration: 3-4 days

### Tasks

#### 5.1 Integration Test Suite

**File:** `tests/integration/test_full_integration.py`

```python
import pytest
from unittest.mock import patch, MagicMock
import tkinter as tk

from agentx.session import AgentXSession
from agentx.config import load_config


class TestFullIntegration:
    """End-to-end integration tests"""
    
    @pytest.fixture
    def mock_ollama(self):
        """Mock Ollama server"""
        with patch("ollama.Client") as mock:
            yield mock
    
    @pytest.fixture
    def session(self, mock_ollama):
        """Create test session"""
        root = tk.Tk()
        config = load_config()
        return AgentXSession(root, config)
    
    def test_prompt_classification_flow(self, session, mock_ollama):
        """Test that prompts flow through classification"""
        # Simulate user input
        session.gui._widgets["input_text"].insert("1.0", "Hello world")
        
        # Trigger submission
        session._handle_submit()
        
        # Verify classification was called
        assert session.agentix_bridge.classify_prompt_sync.called
    
    def test_tool_calls_rendered(self, session, mock_ollama):
        """Test that tool calls appear in GUI"""
        # Setup mock to return tool call
        mock_ollama.return_value.chat.return_value = iter([
            MagicMock(message=MagicMock(
                tool_name="test_tool",
                tool_calls=[{"name": "test_tool", "arguments": {}}]
            ))
        ])
        
        # ... trigger and verify
    
    def test_model_selection(self, session):
        """Test model selection updates configuration"""
        session.gui.model_selector._on_selection(None)
        # Verify model changed
    
    def test_session_persistence(self, session, tmp_path):
        """Test sessions save and load correctly"""
        # Add messages
        # Save session
        # Create new session pointing to same path
        # Verify messages loaded
```

#### 5.2 Performance Testing

```python
# tests/integration/test_performance.py

import time
import pytest

class TestPerformance:
    """Performance benchmarks"""
    
    def test_classification_latency(self, session):
        """Classification should complete within 2 seconds"""
        start = time.time()
        session.agentix_bridge.classify_prompt_sync("Test", session.context)
        elapsed = time.time() - start
        assert elapsed < 2.0
    
    def test_gui_responsiveness(self, session):
        """GUI should remain responsive during streaming"""
        # Start streaming in background
        # Verify GUI events still process
```

#### 5.3 Documentation Updates

- Update README.md with integration information
- Document new configuration options
- Create user guide for integrated features

### Deliverables
- [ ] Full integration test suite passing
- [ ] Performance benchmarks documented
- [ ] README updated
- [ ] User guide created
- [ ] Code comments and docstrings complete

---

## Phase Summary

| Phase | Duration | Key Deliverables |
|-------|----------|------------------|
| 0 | 3-5 days | Shared module, unified config, test scaffold |
| 1 | 5-7 days | AgentixBridge, streaming API, ResponseChunk |
| 2 | 5-7 days | Integration layer, session modifications |
| 3 | 4-5 days | Model selector, tool panel, tool messages |
| 4 | 4-5 days | Unified models, context integration |
| 5 | 3-4 days | Testing, optimization, documentation |

**Total:** 24-33 days (~4-6 weeks)

---

## Dependencies Between Phases

```
Phase 0 ─────┐
             │
             ▼
Phase 1 ────────────┐
             │      │
             ▼      │
Phase 2 ◄──────────┘
             │
             ▼
Phase 3 ─────┐
             │
             ▼
Phase 4 ─────┐
             │
             ▼
Phase 5
```

- Phase 1 depends on Phase 0 (shared modules)
- Phase 2 depends on Phase 1 (bridge API)
- Phase 3 can partially parallel Phase 2
- Phase 4 depends on Phase 2 completion
- Phase 5 depends on all previous phases

---

## Risk Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| Async/sync mismatch | High | Adapter pattern with sync wrappers |
| Breaking existing functionality | High | Feature flags, comprehensive tests |
| Performance degradation | Medium | Benchmarking, caching |
| Configuration complexity | Medium | Unified config with sensible defaults |

---

## Next Document

See `03_RESEARCH_AND_CLARIFICATIONS.md` for areas requiring further research and user input.
