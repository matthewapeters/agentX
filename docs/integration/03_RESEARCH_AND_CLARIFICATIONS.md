# Research Areas and Clarification Requests

## Document Purpose
This document identifies areas requiring further research, technical investigation, or clarification from the user before proceeding with certain aspects of the integration.

---

## 1. Clarifications Needed from User

### 1.1 Session Storage Strategy

**Question:** Should Agentix sessions be migrated to AgentX's session structure, or should we maintain backward compatibility?

**Current State:**
- AgentX: `sessions/<user>/session_<timestamp>/context/*.json`
- Agentix: `~/.agentix/sessions/<session_id>/<timestamp>.json`

**Options:**
| Option | Pros | Cons |
|--------|------|------|
| A. AgentX structure only | Single source of truth, simpler | Breaking change for Agentix CLI users |
| B. Dual storage | Backward compatible | Sync complexity, data duplication |
| C. Configurable | Flexibility | More configuration to manage |

> ✅ **DECISION: Option A - AgentX Structure Only**
>
> **Rationale:** User confirmed AgentX is better structured (designed and built after Agentix). AgentX's session structure becomes the canonical format. Agentix server operates statelessly, receiving context from AgentX client per-request.
>
> **Migration:** Create one-time migration script for any valuable Agentix session data.

**Implementation Notes:**
- Context and session management remain **client-side only** (AgentX)
- Agentix server receives context as request payload, does not persist
- Session folder structure: `sessions/<user>/session_<timestamp>/context/*.json`

---

### 1.2 Ollama Communication Strategy

**Question:** Should we standardize on REST or Python library for Ollama communication?

**Current State:**
- AgentX: Uses `ollama` Python library (`from ollama import Client`)
- Agentix: Uses REST API via `requests` library

**Analysis:**

| Approach | Streaming | Error Handling | Type Safety | OpenAI Compatibility |
|----------|-----------|----------------|-------------|----------------------|
| ollama library | Native | Good | Good | Limited |
| REST API | Requires async | Manual | Manual | Excellent |
| Hybrid | Complex | Complex | Mixed | Good |

> ✅ **DECISION: Hybrid Approach with Clear Separation**
>
> **Rationale:** Use the right tool for each job. The hybrid approach provides optimal performance and flexibility.
>
> | Component | Communication Method | Reason |
> |-----------|---------------------|--------|
> | **AgentX → Local Ollama** | `ollama` Python library | Best streaming UX, native Python types, simpler code |
> | **AgentX → Remote Agentix** | REST API (httpx) | Network transport, async support, JSON payloads |
> | **Agentix Server → Ollama** | REST API | Server-side, OpenAI compatibility, flexibility |

**Implementation Pattern:**
```python
class OllamaClientFactory:
    """Factory for appropriate Ollama client based on context"""
    
    @staticmethod
    def create(config: dict) -> IOllamaClient:
        if config.get("agentix_remote"):
            # Remote Agentix server handles Ollama communication
            return AgentixProxyClient(config["agentix_url"])
        else:
            # Direct local Ollama via Python library
            return LocalOllamaClient(config["ollama_host"])
```

---

### 1.3 Tool Execution Environment

**Question:** Where should MCP tools execute - client-side (AgentX/Agentix) or server-side (Ollama)?

**Current State:**
- Agentix tools (AST, CST) are defined but execution is not fully implemented
- Ollama supports tool definitions but relies on client-side execution

**Options:**
| Option | Security | Complexity | Performance |
|--------|----------|------------|-------------|
| Client-side only | Higher | Lower | Varies |
| Server-side only | Lower | Higher | Better |
| Hybrid | Configurable | Highest | Best |

> ✅ **DECISION: Both Client-Side and Server-Side Tool Execution**
>
> **Rationale:** Agentix server may be remote from AgentX client. Different tools require different execution contexts.
>
> **Architecture:**
> ```
> ┌─────────────────────────────────────────────────────────────┐
> │                     AgentX Client                           │
> │  ┌─────────────────┐  ┌────────────────────────────────┐   │
> │  │ Client Tools    │  │ Session/Context (client-side)  │   │
> │  │ - File access   │  │ - Message history              │   │
> │  │ - Local scripts │  │ - Attachments                  │   │
> │  │ - User prompts  │  │ - Tool results                 │   │
> │  └─────────────────┘  └────────────────────────────────┘   │
> └─────────────────────────────────────────────────────────────┘
>                              │
>                              │ REST API (context in payload)
>                              ▼
> ┌─────────────────────────────────────────────────────────────┐
> │                   Agentix Server (Remote)                   │
> │  ┌─────────────────┐  ┌────────────────────────────────┐   │
> │  │ Server Tools    │  │ Stateless Processing           │   │
> │  │ - Database ops  │  │ - Intent classification        │   │
> │  │ - API calls     │  │ - Tool orchestration           │   │
> │  │ - Compute tasks │  │ - LLM communication            │   │
> │  └─────────────────┘  └────────────────────────────────┘   │
> └─────────────────────────────────────────────────────────────┘
> ```

**Tool Classification:**

| Tool Type | Execution | Examples | Reason |
|-----------|-----------|----------|--------|
| **File Operations** | Client | Read file, list dir | Access to local filesystem |
| **User Context** | Client | Clipboard, selection | Access to user environment |
| **Database Ops** | Server | Query, insert | Server has DB credentials |
| **External APIs** | Server | REST calls, webhooks | Server has API keys |
| **Compute Heavy** | Server | ML inference, analysis | Server has resources |
| **Code Analysis** | Either | AST/CST parsing | Depends on file location |

**Protocol:**
```python
@dataclass
class ToolDefinition:
    name: str
    description: str
    parameters: dict
    execution_context: Literal["client", "server", "either"]
    
@dataclass  
class ToolRequest:
    tool_name: str
    arguments: dict
    # Client sends context needed for server-side execution
    context_snapshot: Optional[dict] = None
    
@dataclass
class ToolResponse:
    success: bool
    result: Any
    # Server returns result to be stored client-side
    add_to_context: bool = True
```

---

### 1.4 Multi-Model Support

**Question:** Should the integration support concurrent models or session-specific model binding?

**Scenarios:**
1. **Single model per session:** User selects model at session start, all messages use same model
2. **Per-message model:** Each message can specify a different model
3. **Multi-agent:** Multiple models collaborate on responses

**User Input Needed:**
- What is the primary use case for model selection?
- Is multi-agent collaboration a future requirement?

---

### 1.5 Classification Display Verbosity

**Question:** How verbose should intent classification display be in the GUI?

**Options:**
| Level | Display |
|-------|---------|
| Minimal | Icon only (e.g., 💬 for conversation) |
| Summary | Icon + one-line summary |
| Detailed | Full classification with reasoning |
| Debug | Raw JSON response |

**User Input Needed:**
- Who is the target user? (Developer, analyst, end-user)
- Should classification be collapsible/expandable?

---

## 2. Technical Research Areas

### 2.1 Async/Threading Architecture

**Issue:** AgentX uses threading (`threading.Thread`) for non-blocking GUI updates, while Agentix's REST client would benefit from async (`asyncio`).

**Research Needed:**
- Best practices for integrating asyncio with tkinter
- Thread-safe patterns for GUI updates from async code
- Performance implications of sync wrappers around async code

**Current Understanding:**
```python
# Pattern being considered
class AsyncBridgeAdapter:
    def __init__(self):
        self._loop = asyncio.new_event_loop()
        
    def run_async(self, coro):
        """Run async code from sync context"""
        return self._loop.run_until_complete(coro)
```

**Alternatives to Research:**
1. `asyncio.run()` per call (overhead)
2. Dedicated async thread with queue
3. `concurrent.futures` with async support

---

### 2.2 MCP Tool Protocol Implementation

**Issue:** Agentix has tool extraction (`describe_tools`) but limited MCP protocol support.

**Research Needed:**
- Full MCP protocol specification
- Existing MCP Python implementations
- Integration patterns with Ollama tool calling

**Current State in Agentix:**
```python
# tools/describe_tools/__init__.py
def extract_tools_from_file(file_path, debug=False, return_dicts=False):
    """Extract tool definitions from Python file"""
    
def to_openai_tools(tool_data):
    """Convert to OpenAI tool format"""
```

**Questions:**
- Should we implement full MCP server/client?
- Or focus on OpenAI-compatible tool format?

---

### 2.3 Context Window Management

**Issue:** Both projects have token/context trimming, but approaches differ.

**AgentX Approach:**
```python
# session.py - builds llm_messages from enabled messages
llm_messages = []
for _, msg in self.context.messages:
    if getattr(msg, "enabled", False):
        llm_messages.append(msg.llm_message_dict())
```

**Agentix Approach:**
```python
# sessions.py - token-based trimming
def trim_context(args, messages, max_tokens):
    """Trim based on estimated token count"""
    total_tokens = 0
    for message in reversed(messages):
        message_tokens = len(message["content"]) // 4  # Rough estimate
        if total_tokens + message_tokens > max_tokens:
            break
```

**Research Needed:**
- Accurate token counting per model
- Smart trimming strategies (keep system prompts, recent messages)
- Integration with enable/disable UI

---

### 2.4 Streaming Response Handling

**Issue:** Need consistent streaming for all response types (thinking, content, tool calls).

**Current Ollama Stream Format:**
```python
for part in client.chat(model, messages, stream=True):
    # part.message.thinking
    # part.message.content
    # part.message.tool_calls
```

**Research Needed:**
- Full Ollama streaming response schema
- Handling partial tool calls in stream
- Proper sequencing of thinking → tool calls → content

---

### 2.5 Error Handling and Recovery

**Issue:** Need consistent error handling across the integration.

**Error Categories:**
| Category | Source | Current Handling |
|----------|--------|------------------|
| Network | Ollama connection | Basic try/catch |
| API | Invalid responses | JSON parse errors |
| Tool | Execution failures | Not implemented |
| Classification | Invalid format | Parse exception |

**Research Needed:**
- Retry strategies for transient errors
- User-facing error messages
- Error logging and debugging

---

## 3. Architectural Decisions Pending

### 3.1 Shared Module Location

**Options:**
1. `src/shared/` - Sibling to agentx and agentix
2. `src/agentx/shared/` - Under agentx
3. Separate package - `agentx-common`

**Consideration:** Package distribution and import paths

---

### 3.2 Configuration Hierarchy

**Question:** How should configuration be layered?

```
Default values
    ↓
TOML file (agentx.toml)
    ↓
Environment variables
    ↓
CLI arguments
    ↓
Session-specific overrides
```

**Decision Needed:** Which layer takes precedence for which settings?

---

### 3.3 Testing Strategy

**Options:**
1. Unit tests only with mocks
2. Integration tests with Ollama emulator
3. Full end-to-end with real Ollama

**Consideration:** CI/CD pipeline requirements, test reliability

---

## 4. Prototype Experiments Needed

### 4.1 Async GUI Integration

**Experiment:** Build minimal prototype of async streaming with tkinter

```python
# prototype/async_tk.py
import asyncio
import tkinter as tk
import threading

class AsyncTkApp:
    """Prototype for async integration"""
    
    def __init__(self):
        self.root = tk.Tk()
        self._async_thread = None
        
    def start_async_task(self, coro):
        """Start async task from GUI"""
        # Test different patterns here
```

**Success Criteria:**
- GUI remains responsive
- Streaming updates appear smoothly
- No deadlocks or race conditions

---

### 4.2 Tool Call Rendering

**Experiment:** Prototype tool call display in tkinter

```python
# prototype/tool_display.py
class ToolCallWidget:
    """Expandable tool call display"""
    
    def render_tool_call(self, name, input_data, output_data):
        """Render with collapsible sections"""
```

**Success Criteria:**
- Collapsible input/output sections
- Code formatting for JSON
- Copy-to-clipboard functionality

---

### 4.3 Model Switching

**Experiment:** Test model switching mid-session

**Questions to Answer:**
- Does context work across models?
- How to handle model-specific system prompts?
- Performance impact of model switching

---

## 5. External Dependencies to Evaluate

### 5.1 Token Counting Library

**Candidates:**
| Library | Models Supported | Accuracy |
|---------|------------------|----------|
| tiktoken | OpenAI models | High |
| transformers | Most models | High |
| Simple heuristic | All | Low |

**Recommendation:** Research Ollama-specific token counting

---

### 5.2 Async HTTP Client

**Candidates:**
| Library | Streaming | Performance |
|---------|-----------|-------------|
| httpx | Excellent | Good |
| aiohttp | Good | Excellent |
| requests-async | Deprecated | - |

**Recommendation:** httpx for consistency and SSE support

---

### 5.3 Configuration Management

**Candidates:**
| Library | TOML | CLI | Env |
|---------|------|-----|-----|
| tomli/tomllib | Yes | No | No |
| pydantic-settings | Yes | No | Yes |
| dynaconf | Yes | No | Yes |

**Recommendation:** Evaluate pydantic-settings for validation

---

## 6. User Feedback Status

### ✅ Resolved - Critical Decisions Made

| Item | Decision | Status |
|------|----------|--------|
| **1.1** Session storage | AgentX structure only | ✅ Resolved |
| **1.2** Ollama communication | Hybrid approach | ✅ Resolved |
| **1.3** Tool execution | Both client and server | ✅ Resolved |

### Remaining Items (Lower Priority)

| Item | Question | Default if Not Specified |
|------|----------|-------------------------|
| **1.4** Multi-model support | Per-session or per-message? | Per-session |
| **1.5** Classification verbosity | How detailed in GUI? | Summary (icon + one-line) |
| **3.2** Config hierarchy | Which layer takes precedence? | TOML < Env < CLI |

### Optional Context (Nice to Have)
- [ ] Target deployment environment (local/enterprise/cloud)
- [ ] Primary user personas (developer/analyst/end-user)
- [ ] Timeline constraints or hard deadlines

---

## Summary of Decisions

| ID | Question | Decision | Status |
|----|----------|----------|--------|
| 1.1 | Session storage strategy | AgentX structure only | ✅ Resolved |
| 1.2 | Ollama communication method | Hybrid (library local, REST remote) | ✅ Resolved |
| 1.3 | Tool execution environment | Both client and server | ✅ Resolved |
| 1.4 | Multi-model support | TBD (default: per-session) | ⏳ Optional |
| 1.5 | Classification verbosity | TBD (default: summary) | ⏳ Optional |
| 3.2 | Config hierarchy | TBD (default: TOML < Env < CLI) | ⏳ Optional |

**All blocking decisions have been resolved. Implementation can proceed.**

---

## Next Document

See `04_IMPROVEMENT_SUGGESTIONS.md` for enhancement recommendations.
