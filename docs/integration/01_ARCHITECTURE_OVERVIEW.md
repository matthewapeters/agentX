# AgentX-Agentix Integration Architecture Overview

## Document Purpose
This document provides architectural guidance for agents and developers implementing the integration between AgentX (GUI frontend) and Agentix (agent middleware). It describes the current state of each system, their responsibilities, and the target integrated architecture.

---

## 1. Current System Analysis

### 1.1 AgentX (GUI Frontend)

**Purpose:** A tkinter-based GUI chat application providing superior session management and user experience.

**Core Components:**

| Component | File | Responsibility |
|-----------|------|----------------|
| `AgentXSession` | `src/agentx/session.py` | Orchestrates session lifecycle, manages context, coordinates GUI and Ollama communication |
| `GUIManager` | `src/agentx/gui_manager.py` | Presentation logic, widget management, separation from business logic |
| `Context` | `src/agentx/context.py` | Maintains conversation context, message history per session |
| `Message` | `src/agentx/message.py` | Message data structure with attachments, serialization, LLM formatting |
| `History` | `src/agentx/history.py` | Cross-session history management, enables message reuse |
| `FileExplorer` | `src/agentx/file_explorer.py` | File browsing and attachment capabilities |

**Key Characteristics:**
- Uses Python `ollama` library directly for LLM communication
- Session-based folder structure (`sessions/<user>/session_<timestamp>/`)
- Supports streaming responses with interrupt capability
- Message enable/disable for context management
- Attachment system for file context injection

**Current Data Flow:**
```
User Input → GUIManager → AgentXSession → ollama.Client.chat() → Stream Response → GUIManager
                              ↓
                          Context (save messages)
```

### 1.2 Agentix (Agent Middleware)

**Purpose:** Middleware providing prompt classification, intent analysis, MCP tooling, and REST-based Ollama communication.

**Core Components:**

| Component | File | Responsibility |
|-----------|------|----------------|
| `agentix()` | `src/agentix/agent.py` | Main agent entry point, orchestrates classification and next steps |
| `AgentixConfig` | `src/agentix/agentix_config.py` | Configuration management via CLI/TOML |
| `api_client` | `src/agentix/api_client.py` | REST-based Ollama API communication |
| `sessions.py` | `src/agentix/context/sessions.py` | Session management, context assembly, token trimming |
| `prompts.py` | `src/agentix/context/prompts.py` | System/user/tool prompt management |
| `PromptClassificationResponse` | `src/agentix/prompt_classification_response.py` | Intent classification data structure |
| `next_steps/` | `src/agentix/next_steps/` | Action handlers based on classification |
| `tools/` | `src/agentix/tools/` | MCP tooling (AST, CST analysis) |
| `server.py` | `src/agentix/server.py` | FastAPI server for OpenAI-compatible API |

**Key Characteristics:**
- REST-based communication with Ollama (`requests` library)
- Prompt classification before processing (intent detection)
- Supports multiple next-step actions: `respond_directly`, `single_tool`, `invoke_planner`, `escalate`
- Tool extraction and OpenAI-compatible tool formatting
- FastAPI server for API compatibility layer
- Token-based context trimming

**Current Data Flow:**
```
CLI Args → AgentixConfig → agentix() → manage_sessions() → assemble_classification_prompt()
                                              ↓
                                       query_api() (REST)
                                              ↓
                                  PromptClassificationResponse
                                              ↓
                                       take_steps() → [respond_directly | single_tool | invoke_planner | escalate]
```

---

## 2. Integration Architecture

### 2.1 Target Architecture Vision

> **Key Principle:** AgentX is the better-structured project. AgentX owns all client-side concerns (session, context, GUI). Agentix server is stateless middleware that may be local or remote.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        AgentX Client (All State Lives Here)                  │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │ GUIManager  │  │FileExplorer │  │   History   │  │   System Status Bar │ │
│  │ - Input     │  │ - Browse    │  │ - Sessions  │  │  - Model Selection  │ │
│  │ - Output    │  │ - Attach    │  │ - Context   │  │  - Connection Status│ │
│  │ - Render    │  │             │  │             │  │  - Tool Status      │ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────────┘ │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                   Session & Context (Client-Side Only)               │   │
│  │  - Message history persisted locally                                 │   │
│  │  - Attachments and file references                                   │   │
│  │  - Tool results stored as messages                                   │   │
│  │  - Canonical storage: sessions/<user>/session_<timestamp>/           │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      Client-Side Tools                               │   │
│  │  - File operations (read, write, list)                               │   │
│  │  - Local script execution                                            │   │
│  │  - User environment access                                           │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
                           │                              │
                           │ REST API                     │ ollama library
                           │ (context in payload)         │ (local only)
                           ▼                              ▼
┌─────────────────────────────────────────┐    ┌─────────────────────────────┐
│      Agentix Server (Stateless)         │    │   Local Ollama Server       │
│  ┌─────────────┐  ┌─────────────────┐   │    │   (Direct connection for    │
│  │  Intent     │  │  Server-Side    │   │    │    best streaming UX)       │
│  │ Classifier  │  │  Tools          │   │    └─────────────────────────────┘
│  │             │  │  - DB queries   │   │
│  │             │  │  - API calls    │   │
│  │             │  │  - Compute      │   │
│  └─────────────┘  └─────────────────┘   │
│                                         │
│  ┌─────────────────────────────────┐   │
│  │    Ollama Communication         │   │
│  │    (REST, OpenAI-compatible)    │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
```

### 2.2 Key Integration Points

#### A. User Prompt Processing Pipeline

**Before Integration (AgentX):**
```python
# session.py - stream_ollama_response_worker()
prompt = self.gui.get_user_input()
client = Client(host=f"http://{ollama_host}")
for part in client.chat(model=ollama_model, messages=llm_messages, stream=True):
    # Process response
```

**After Integration:**
```python
# session.py - stream_ollama_response_worker()
prompt = self.gui.get_user_input()
classification = await self.agentix.classify_intent(prompt, context=self.context)
response_handler = self.agentix.get_handler(classification.next_step)
async for part in response_handler.execute(prompt, context=self.context, stream=True):
    # Process response with tool output integration
```

#### B. Context Unification

**Current State:**
- AgentX: `Context` class with `messages: list[Message]` stored in session folder
- Agentix: `Message` class stored in `~/.agentix/sessions/`

**Target State:**
- Unified `Context` class used by both systems
- Single session storage location (AgentX's `sessions/<user>/session_<timestamp>/`)
- Agentix references AgentX's context for classification and processing

#### C. Model Management

**Current State:**
- AgentX: Reads from `agentx.toml` configuration
- Agentix: Fetches from Ollama `/api/tags` endpoint

**Target State:**
- Agentix provides model list via `get_models()` 
- AgentX GUI displays model selector in system bar
- Model selection stored in session context

#### D. Tool Integration

**Current State:**
- Agentix: Extracts tools from Python files, formats for OpenAI compatibility
- AgentX: Prints tool calls to console (not integrated in GUI)

**Target State:**
- Tool calls rendered in AgentX output with expandable details
- Tool results added to context as message objects
- Tools can be enabled/disabled via GUI checkboxes

#### E. Client/Server Tool Architecture

**Key Insight:** Agentix server may be remote from AgentX client. Tools must be classified by execution context.

**Tool Execution Flow:**
```
┌───────────────────────────────────────────────────────────────────────────┐
│                              AgentX Client                                 │
│                                                                           │
│  1. LLM returns tool_call        4. Store result in context              │
│          │                                ▲                               │
│          ▼                                │                               │
│  2. Check tool.execution_context ─────────┤                               │
│          │                                │                               │
│    ┌─────┴─────┐                          │                               │
│    │           │                          │                               │
│    ▼           ▼                          │                               │
│ [client]    [server]                      │                               │
│    │           │                          │                               │
│    ▼           │                          │                               │
│ 3a. Execute    │                          │                               │
│ locally ───────┼──────────────────────────┘                               │
│                │                                                          │
└────────────────┼──────────────────────────────────────────────────────────┘
                 │
                 │ 3b. REST: POST /tools/execute
                 │     {tool_name, arguments, context_snapshot}
                 ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                           Agentix Server                                   │
│                                                                           │
│  Execute server-side tool → Return result                                 │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
```

**Tool Classification Table:**

| Tool Category | Execution | Examples | Rationale |
|---------------|-----------|----------|-----------|
| File System | Client | `read_file`, `write_file`, `list_dir` | Access to local files |
| User Context | Client | `get_clipboard`, `get_selection` | Access to user environment |
| Code Analysis | Client | `parse_ast`, `analyze_cst` | Files are on client |
| Database | Server | `query_db`, `insert_record` | Server has credentials |
| External APIs | Server | `call_rest_api`, `send_webhook` | Server has API keys |
| ML/Compute | Server | `run_inference`, `analyze_data` | Server has resources |
| Hybrid | Either | `search_code` | Depends on file location |

---

## 3. Component Responsibilities (Post-Integration)

### 3.1 AgentX Components (Client-Side)

| Component | New Responsibilities |
|-----------|---------------------|
| `AgentXSession` | Owns all state; coordinates with Agentix for LLM interactions |
| `GUIManager` | Render tool outputs, display model selector, show intent classification |
| `Context` | **Canonical context storage** - all messages, attachments, tool results |
| `Message` | Support tool role types, include classification metadata |
| `ClientToolExecutor` | Execute client-side tools, return results to context |

### 3.2 Agentix Components (Server-Side, Stateless)  

| Component | New Responsibilities |
|-----------|---------------------|
| `agentix()` | Provide programmatic API for AgentX (not just CLI) |
| `api_client` | Add streaming support for GUI integration |
| `sessions.py` | Reference AgentX session context (not duplicate) |
| `next_steps/` | Return structured results for GUI rendering |

### 3.3 New Integration Components

| Component | Responsibility |
|-----------|----------------|
| `AgentixBridge` | Adapter between AgentX session and Agentix middleware |
| `ToolRenderer` | Convert tool calls/results to GUI widgets |
| `ModelSelector` | GUI component backed by Agentix model management |
| `IntentIndicator` | Display classification result in status bar |

---

## 4. Data Models

### 4.1 Unified Message Model

```python
@dataclass
class UnifiedMessage:
    role: str  # user | assistant | system | thinking | tool_call | tool_result
    content: str
    attachments: list[Attachment]
    enabled: bool
    
    # New fields for integration
    classification: Optional[PromptClassificationResponse]  # For user messages
    tool_name: Optional[str]  # For tool_call/tool_result roles
    tool_input: Optional[dict]  # Arguments passed to tool
    tool_output: Optional[Any]  # Result from tool execution
    
    def to_llm_format(self) -> dict:
        """Format for Ollama/OpenAI API consumption"""
        
    def to_gui_format(self) -> dict:
        """Format for GUI rendering"""
```

### 4.2 Integration Configuration

```toml
# agentx.toml (extended)
[agentx]
ollama_host = "localhost:11435"
ollama_model = "gpt-oss"
ollama_initial_load_timeout_seconds = 120
screen_side = "left"

[agentix]
enabled = true
classify_all_prompts = true
default_system_prompts = ["prompt_classification"]
available_tools = ["cst", "ast"]
show_classification_in_gui = true
show_tool_calls_in_gui = true
```

---

## 5. Interface Contracts

### 5.1 AgentixBridge Interface

```python
class IAgentixBridge(Protocol):
    """Interface for Agentix integration with AgentX"""
    
    async def classify_prompt(
        self, 
        prompt: str, 
        context: Context
    ) -> PromptClassificationResponse:
        """Classify user intent before processing"""
        
    async def get_models(self) -> list[dict]:
        """Fetch available models from Ollama"""
        
    async def process_prompt(
        self,
        prompt: str,
        context: Context,
        model: str,
        stream: bool = True
    ) -> AsyncIterator[ResponseChunk]:
        """Process prompt through Agentix pipeline"""
        
    def get_available_tools(self) -> list[ToolDefinition]:
        """Return available MCP tools"""
```

### 5.2 ResponseChunk Model

```python
@dataclass
class ResponseChunk:
    """Streaming response chunk from Agentix"""
    type: str  # content | thinking | tool_call | tool_result | error
    content: str
    tool_name: Optional[str] = None
    tool_input: Optional[dict] = None
    tool_output: Optional[Any] = None
```

---

## 6. File Organization (Post-Integration)

```
src/
├── agentx/
│   ├── __init__.py
│   ├── main.py                    # Entry point
│   ├── session.py                 # AgentXSession (enhanced)
│   ├── gui_manager.py             # GUIManager (enhanced)
│   ├── context.py                 # Unified Context
│   ├── message.py                 # Unified Message
│   ├── integration/               # NEW: Integration layer
│   │   ├── __init__.py
│   │   ├── agentix_bridge.py      # Bridge to Agentix
│   │   ├── tool_renderer.py       # Tool output GUI rendering
│   │   └── model_selector.py      # Model selection component
│   └── ...
│
├── agentix/
│   ├── __init__.py
│   ├── agent.py                   # agentix() with programmatic API
│   ├── api_client.py              # Enhanced with streaming
│   ├── context/
│   │   ├── sessions.py            # References AgentX context
│   │   └── ...
│   ├── next_steps/                # Return structured results
│   └── tools/                     # MCP tooling
│
└── shared/                        # NEW: Shared models
    ├── __init__.py
    ├── message.py                 # Unified message model
    ├── response.py                # Response chunk model
    └── config.py                  # Unified configuration
```

---

## 7. Key Design Decisions

### Decision 1: AgentX as Primary Session Manager
**Rationale:** AgentX has superior session management with user folders, timestamps, and GUI integration. Agentix should reference AgentX sessions rather than maintain parallel storage.

### Decision 2: Agentix Provides Intent Classification
**Rationale:** Agentix's prompt classification infrastructure (intent detection, next-step routing) adds intelligence before response generation. All user prompts should pass through classification.

### Decision 3: REST with Streaming for LLM Communication
**Rationale:** Agentix uses REST for flexibility and OpenAI compatibility. Adding streaming support allows GUI responsiveness while maintaining API compatibility.

### Decision 4: Tool Outputs as First-Class Messages
**Rationale:** Tool calls and results should be rendered in the GUI and stored in context for transparency and replayability.

### Decision 5: Shared Data Models
**Rationale:** Create a `shared/` module with unified models to avoid duplication and ensure consistency.

---

## Next Steps

See `02_PHASED_INTEGRATION_PLAN.md` for detailed implementation phases.
