# AgentX-Agentix Integration Architecture Overview

> **Last updated**: 2026-04-19  
> For the full system architecture, see [../architecture.md](../architecture.md).

## Document Purpose

This document provides architectural guidance for developers and agents working on the
integration between AgentX (GUI frontend) and Agentix (agent middleware).

---

## 1. Current System Analysis

### 1.1 AgentX (GUI Frontend)

**Purpose:** A Tkinter-based GUI chat application with session management, working memory,
and hierarchical task execution.

**Core Components:**

| Component | File | Responsibility |
|-----------|------|----------------|
| `AgentXSession` | `src/agentx/session.py` | Thin coordinator — wires all subsystems |
| `SessionState` | `src/agentx/session_state.py` | Mutable session data (model, folder, user) |
| `StreamingController` | `src/agentx/streaming_controller.py` | All LLM streaming and display logic |
| `ToolDispatcher` | `src/agentx/tool_dispatcher.py` | Routes tool calls to client/server executor |
| `GUIManager` | `src/agentx/gui/gui_manager.py` | Thin coordinator; delegates to 4 panels |
| `IGUIManager` | `src/agentx/igui_manager.py` | `Protocol` interface — session depends only on this |
| `Context` | `src/shared/models/context.py` | Conversation history — single source of truth |
| `Message` | `src/shared/models/message.py` | Message dataclass with `MessageRole` enum |
| `WorkingMemory` | `src/shared/models/working_memory.py` | Per-session key-value fact store |
| `History` | `src/agentx/history.py` | Cross-session history management |
| `FileExplorer` | `src/agentx/file_explorer.py` | File browsing and attachment capabilities |
| `AgentixBridgeAdapter` | `src/agentx/integration/agentix_bridge_adapter.py` | Bridges async Agentix → sync/generator |
| `ClientToolExecutor` | `src/agentx/integration/client_tool_executor.py` | File-system tools (read/write/list/search) |
| `WorkingMemoryToolExecutor` | `src/agentx/integration/working_memory_tool_executor.py` | WM tools for the agent |
| `ResponseHandler` | `src/agentx/integration/response_handler.py` | Translates `ResponseChunk` → GUI callbacks |

**Key Characteristics:**

- Uses `AgentixBridgeAdapter` (not `ollama` directly) for all LLM calls
- Session-based folder structure (`sessions/<user>/<session_YYYY-MM-DD_HH-MM-SS>/`)
- Supports streaming responses with interrupt capability
- Message enable/disable for context management
- Attachment system for file context injection
- Working Memory with user/agent ownership enforcement
- Hierarchical Task Execution with PlanTreeWidget

**Current Data Flow:**

```
User Input → InputPanel → AgentXSession._handle_submit()
                              │
                              ├── Context.add_message(USER)
                              ├── GUIManager.display_user_message()
                              └── StreamingController.run_streaming_loop()
                                        │
                                        └── AgentixBridgeAdapter.process_prompt_generator()
                                                  │
                                                  └── AgentixBridge.process_prompt_streaming()
                                                            │
                                            ┌─────────────┴──────────────┐
                                     classify_prompt()        ToolLoopRunner
                                            │                      │
                                    Ollama REST API          Ollama REST API
```

### 1.2 Agentix (Agent Middleware)

**Purpose:** Middleware providing prompt classification, intent analysis, tool loop execution, and
REST-based Ollama communication.

**Core Components:**

| Component | File | Responsibility |
|-----------|------|----------------|
| `AgentixBridge` | `src/agentix/bridge/bridge.py` | Main entry point; orchestrates classify → route → stream |
| `ToolLoopRunner` | `src/agentix/bridge/tool_loop.py` | Core agentic loop (LLM chunks + tool dispatch) |
| `classify_prompt` | `src/agentix/bridge/classify_prompt.py` | Intent classification before routing |
| `assemble_prompts` | `src/agentix/bridge/prompt_assembly.py` | Builds messages list for API call |
| `AssertionChecker` | `src/agentix/bridge/assertion_checker.py` | Pre/post/invariant assertion verification |
| `AgentixConfig` | `src/agentix/agentix_config.py` | Configuration management |
| `ApiClient` | `src/agentix/api_client.py` | REST calls to `/v1/chat/completions` |
| `QueryPayload` | `src/agentix/query_payload.py` | API request model (emits `response_format: json_object`) |
| `PromptLoader` | `src/agentix/prompt_loader.py` | Loads system prompt `.md` files |
| `PromptClassificationResponse` | `src/agentix/prompt_classification_response.py` | Intent + next_step data model |
| `next_steps/` | `src/agentix/next_steps/` | Action handlers per `NextStep` variant |
| `tools/schema.py` | `src/agentix/tools/schema.py` | `extract_tool_schema()` — function → OpenAI schema |
| `tools/cst_tools.py` | `src/agentix/tools/cst_tools.py` | CST-based code analysis |
| `tools/ast_tools.py` | `src/agentix/tools/ast_tools.py` | AST-based code analysis |
| `context/sessions.py` | `src/agentix/context/sessions.py` | Session management, token trimming |

**Key Characteristics:**

- REST-based communication with Ollama (`requests` library via `api_client.py`)
- Prompt classification before processing (intent detection) with dedicated `phi4-mini` model
- Supports `respond_directly`, `single_tool`, `invoke_planner`, `escalate` next steps
- Tool schema extraction and OpenAI-compatible tool formatting
- Token-based context trimming
- `response_format: {"type": "json_object"}` enforced via `QueryPayload`

**Current Data Flow:**

```
AgentixBridge.process_prompt_streaming(prompt, context, …)
    │
    ├── classify_prompt() → PromptClassificationResponse
    │         {intent, next_step, needs_clarification}
    │
    └── route on next_step:
          respond_directly  → _stream_direct_response()
          single_tool       → _stream_tool_response()
          invoke_planner    → _stream_planned_response()
          escalate          → _stream_direct_response()
                │
                └── _run_tool_loop() via ToolLoopRunner
                      ├── _iter_llm_chunks()   OpenAI streaming
                      ├── execute_tool(name)   client + server tools
                      └── yields ResponseChunk objects
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
