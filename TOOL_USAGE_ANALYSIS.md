# AgentX Tool Usage Path - Comprehensive Analysis

## EXECUTIVE SUMMARY

The tool usage path in AgentX is **partially implemented with critical gaps**. While the plumbing for client-side and server-side tool execution exists, **multi-turn tool use (feeding tool results back to the LLM for continued reasoning) is NOT implemented**. The system currently handles tool execution and storage but does not re-invoke the LLM with tool results for continued conversation.

---

## 1. FULL TOOL CALL FLOW END-TO-END

### Flow Diagram
```
User Input
    ↓
[AgentXSession._stream_via_agentix()]
    ↓
[Prompt Classification] → intent + next_step
    ↓
MATCH next_step:
    ├─ respond_directly → Direct LLM response ✓
    ├─ single_tool → Tool execution (INCOMPLETE) ✗
    ├─ invoke_planner → Multi-step planning (INCOMPLETE) ✗
    └─ escalate → Safety error
    ↓
[ResponseChunk Stream]
    ├─ THINKING chunks
    ├─ CONTENT chunks
    ├─ TOOL_CALL chunks (parsed from LLM)
    ├─ TOOL_RESULT chunks (LOCAL only, NOT from LLM)
    └─ DONE chunk
    ↓
[ResponseHandler processes chunks]
    ├─ on_content() → Display
    ├─ on_thinking() → Display
    ├─ on_tool_call() → Execute tool
    ├─ on_tool_result() → Display
    └─ Accumulate for persistence
    ↓
[Tool Results Stored in Context]
    ├─ TOOL_CALL message (role: "tool_call")
    ├─ TOOL_RESULT message (role: "tool_result")
    └─ Marked with tool_name, tool_input, enabled=true
    ↓
[MISSING STEP: Multi-turn tool use]
    ✗ Tool results NOT re-sent to LLM
    ✗ LLM does NOT continue reasoning with tool output
    ✗ No agentic loop for complex tasks
```

### Current State: INCOMPLETE MULTI-TURN SUPPORT
- ✅ Tool execution works
- ✅ Results stored in context
- ✅ Results displayed in GUI
- ❌ Results NOT fed back to LLM in new request
- ❌ LLM can't reason about tool outputs for follow-up actions

---

## 2. FILES IN src/agentix/ AND THEIR PURPOSES

### Directory Structure
```
/Projects/agentX/src/agentix/
├── __init__.py                      # Package exports
├── __main__.py                      # CLI entry point
├── main.py                          # Main processing pipeline
├── agent.py                         # Agent implementation
├── api_client.py                    # Ollama API client
├── agentix_config.py               # Configuration dataclass
├── models.py                        # Model management (get_models, get_model)
├── constants.py                     # Constants (OLLAMA_API_BASE, etc)
├── server.py                        # FastAPI server (if used)
├── file_utils.py                    # File utilities
├── local_classifier.py              # Local prompt classification (fallback)
├── prompt_classification_response.py # Response model (Intent, NextStep, etc)
├── query_payload.py                 # Request payload builder
├── transforms.py                    # Transform utilities
│
├── bridge/
│   ├── __init__.py
│   ├── bridge.py                    # AgentixBridge: main programmatic API
│   └── classify_prompt.py           # Prompt classification logic
│
├── context/
│   ├── __init__.py
│   ├── context.py                   # Context management
│   ├── message.py                   # Message models
│   ├── prompts.py                   # System prompts
│   └── sessions.py                  # Session management
│
├── next_steps/
│   ├── __init__.py
│   ├── respond_directly.py          # Route: direct response (stub)
│   ├── single_tool.py               # Route: single tool (stub - NOT IMPLEMENTED)
│   ├── invoke_planner.py            # Route: planner (stub - NOT IMPLEMENTED)
│   ├── escalate.py                  # Route: escalation (stub)
│   ├── plan_steps.py                # Step planning logic
│   └── take_steps.py                # Step execution (NOT IMPLEMENTED)
│
└── tools/
    ├── __init__.py
    ├── codeagent.py                 # Code analysis agent
    ├── ast_tools.py                 # AST analysis tools
    ├── cst_tools.py                 # CST analysis tools
    └── describe_tools/
        ├── __init__.py
        ├── tools.py                 # Tool definitions
        ├── tool_collector.py        # Tool discovery
        ├── tool_extractor.py        # Tool extraction from code
        ├── tool_spec.py             # Tool specification
        └── utils.py                 # Utilities
```

### Key Components

**1. bridge/bridge.py - AgentixBridge (Main API)**
- Classification: `classify_prompt(prompt, context) → PromptClassificationResponse`
- Streaming: `process_prompt_streaming(prompt, context, classification) → Iterator[ResponseChunk]`
- Models: `get_available_models() → list[dict]`
- Tools: `get_available_tools() → list[dict]` (returns OpenAI format)
- Routes based on NextStep enum

**2. next_steps/ - Route Handlers (MOSTLY STUBS)**
- `respond_directly.py`: Works ✓
- `single_tool.py`: Returns `pass  # NOT IMPLEMENTED` ✗
- `invoke_planner.py`: Returns placeholder ✗
- `escalate.py`: Works ✓

**3. tools/ - Code Analysis Tools**
- CST/AST extraction for code analysis
- Tool discovery from codebase
- Conversion to OpenAI format

---

## 3. FILES IN src/shared/models/tools.py

### Tool Data Models

**ToolExecutionContext (Enum)**
```python
CLIENT = "client"     # Execute on AgentX client
SERVER = "server"     # Execute on Agentix server
EITHER = "either"     # Can execute anywhere
```

**ToolDefinition**
- `name`: Tool identifier
- `description`: LLM description
- `parameters`: JSON Schema
- `execution_context`: Where to execute
- `returns`: Optional return schema
- Methods: `to_openai_format()`, `to_dict()`, `from_dict()`

**ToolRequest**
- `tool_name`: Which tool to call
- `arguments`: Tool parameters
- `request_id`: Tracking ID
- `context_snapshot`: Optional context
- Methods: `from_llm_tool_call()`

**ToolResponse**
- `success`: bool
- `output`: Any (tool result)
- `error`: Optional error message
- `request_id`: Matches request
- `execution_time_ms`: Timing
- `add_to_context`: bool (whether to store in conversation)
- Methods: `success_response()`, `error_response()`, `to_llm_format()`

**ToolRegistry**
- `register(tool)`: Add tool
- `get(name)`: Retrieve tool
- `list_definitions()`: List all tools
- `list_by_context()`: Filter by execution context
- `get_client_tools()`: Client-side tools
- `get_server_tools()`: Server-side tools
- `to_openai_format()`: For LLM
- `async execute(request)`: Execute with validation
- Missing methods:
  - `get_enabled_tools()` (stub)
  - `set_enabled_tools()` (stub)
  - `is_tool_enabled()` (stub)

**Tool Enablement Management (INCOMPLETE)**
```python
def get_enabled_tools(self) -> list[str]:
    """Return a list of enabled tool names. Default: all tools enabled."""
    return list(self._tools.keys())

def set_enabled_tools(self, enabled_tools: list[str]) -> None:
    """Set which tools are enabled (stub for config/session integration)."""
    pass  # Placeholder - no-op

def is_tool_enabled(self, name: str) -> bool:
    """Return True if the tool is enabled."""
    return name in self.get_enabled_tools()
```

**BaseTool (Abstract Base Class)**
- `definition` property
- `execute()` method (raises NotImplementedError by default)
- `validate_input()` method
- `name` property
- `execution_context` property

**Global Registries**
```python
client_tool_registry = ToolRegistry()
server_tool_registry = ToolRegistry()
```

---

## 4. integration/server_tool_executor.py

### Purpose
Execute tools on the Agentix server (DB ops, API calls, compute-intensive tasks)

### ServerToolExecutor Class

**Methods:**
- `get_available_tools()`: Fetch from Agentix
- `is_available()`: Check bridge connectivity
- `execute(tool_name, arguments, context)`: Execute tool
- `_execute_code_analysis_tool()`: Local code analysis
- `_execute_via_agentix()`: Remote tool execution
- `_format_tool_result()`: Format for display
- `_extract_tool_name()`: Parse OpenAI format

**Supported Code Analysis Tools:**
- `analyze_syntax`: CST analysis
- `find_functions`: Function discovery
- `find_classes`: Class discovery
- `find_imports`: Import discovery
- `suggest_refactoring`: Refactoring suggestions

### AdvancedToolRegistry Class

**Purpose:** Manage available tools from Agentix

**Methods:**
- `initialize()`: Load tools from Agentix bridge
- `get_tool_info(name)`: Retrieve tool metadata
- `list_tools(category)`: List with optional filtering
- `_extract_name()` and `_extract_description()`: Parse OpenAI format

**Current State:**
- Loads tools from bridge on first access
- Caches tools
- Marks code analysis tools separately

---

## 5. integration/client_tool_executor.py

### Purpose
Execute tools on the client side (file operations, local analysis)

### ClientToolExecutor Class

**Supported Tools:**
1. `read_file`: Read file contents (50KB limit)
2. `write_file`: Create/overwrite files
3. `list_directory`: List directory contents
4. `get_file_info`: Get file metadata (JSON)
5. `search_files`: Search by pattern

**Security Features:**
- Base path restriction (prevents directory traversal)
- Symlink handling for macOS /var issue
- Path validation before operations
- Size limits on output (50KB)

**Implementation:**
- All methods return strings for LLM consumption
- Error messages on failures
- JSON output for complex results

---

## 6. integration/agentix_bridge_adapter.py

### Purpose
Bridge between AgentX (tkinter/threading) and Agentix (async/await)

### AgentixBridgeAdapter Class

**Methods:**
- `classify_prompt_sync(prompt, context)`: Blocking classification
- `process_prompt_generator(prompt, context, classification)`: Generator-based streaming
- `get_models()`: List available models
- `get_tools()`: List available tools
- `_convert_config()`: AgentX config → AgentixConfig

**Key Features:**
- Converts between config formats
- Synchronous wrappers for blocking calls
- Generator-based streaming compatible with tkinter
- Thread-safe for background threads

**Config Mapping:**
```python
agentx.ollama_model → model
agentx.ollama_host → ollama_host
agentx.temperature → temperature
agentix.available_tools → tools
agentix.classify_prompts → classify_prompts
```

---

## 7. integration/response_handler.py

### Purpose
Process ResponseChunks from Agentix and trigger GUI callbacks

### ResponseHandler Class

**Constructor Callbacks:**
- `on_content(text)`: Display content
- `on_thinking(text)`: Display thinking
- `on_tool_call(name, input)`: Handle tool request
- `on_tool_result(id, result)`: Display tool result
- `on_classification(meta)`: Display classification
- `on_error(msg, code)`: Display error
- `on_done()`: Stream complete

**Processing:**
- `process_chunk(chunk)`: Route chunk to handler
- Accumulates content/thinking/tool_calls in buffers
- `get_complete_content()`: Retrieve accumulated content
- `get_complete_thinking()`: Retrieve accumulated thinking
- `to_message()`: Convert to Message with metadata
- `reset()`: Clear buffers for new session

**Current State:**
- ✅ Handles all chunk types
- ✅ Stores metadata
- ⚠️ `tool_id` not available in ResponseChunk (uses tool_name as fallback)

---

## 8. ResponseChunk Types for Tools

### ChunkType Enum (src/shared/models/response.py)

```python
class ChunkType(str, Enum):
    # Content types
    CONTENT = "content"          # Assistant response text
    THINKING = "thinking"        # LLM reasoning

    # Tool types
    TOOL_CALL = "tool_call"      # Request to execute tool
    TOOL_RESULT = "tool_result"  # Result from execution

    # Metadata types
    CLASSIFICATION = "classification"
    MODEL_INFO = "model_info"

    # Control types
    ERROR = "error"
    DONE = "done"
```

### ResponseChunk Dataclass

**Fields:**
```python
type: Optional[ChunkType]              # Chunk type
content: str                           # Text content

# Tool-specific
tool_name: Optional[str]               # Tool name
tool_input: Optional[dict]             # Tool arguments
tool_output: Optional[Any]             # Tool result
tool_execution_context: Optional[str]  # "client" or "server"
tool_id: Optional[str]                 # Tool instance ID

# Classification
classification: Optional[dict]         # Intent classification

# Metadata
model: Optional[str]                   # Model name
done_reason: Optional[str]             # Completion reason
error_code: Optional[str]              # Error code
```

**Tool-Related Properties:**
- `is_content`: bool (CONTENT or THINKING)
- `is_tool`: bool (TOOL_CALL or TOOL_RESULT)
- `is_error`: bool (ERROR)
- `is_done`: bool (DONE)

**Factory Functions:**
```python
tool_call_chunk(tool_name, tool_input, execution_context)
tool_result_chunk(tool_name, tool_output, tool_id)
```

### Current Tool Chunk Gaps

**Missing:** `tool_id` in ResponseChunk
- Response handler uses `tool_name` as ID fallback
- Should be unique per tool invocation for multi-tool requests

---

## 9. session.py - Tool Coordination

### Tool Execution Flow

**Key Method: `handle_tool_call(tool_name, tool_input)`**
1. Create TOOL_CALL message with metadata
2. Add to context
3. Display in GUI
4. Execute tool (route: client vs server)
5. Create TOOL_RESULT message
6. Add to context
7. Display in GUI

**Code Snippet (lines 321-379):**
```python
def handle_tool_call(self, tool_name: str, tool_input: dict) -> None:
    """Handle a tool call from the LLM response."""
    try:
        # Store TOOL_CALL message
        tool_call_msg = Message(
            role=MessageRole.TOOL_CALL,
            content=f"Calling tool: {tool_name}",
        )
        tool_call_msg.tool_name = tool_name
        tool_call_msg.tool_input = tool_input
        tool_call_msg.enabled = True
        self.add_message_to_context(tool_call_msg)
        
        # Execute the tool
        result = self.execute_tool(tool_name, tool_input)
        
        # Store TOOL_RESULT message
        tool_result_msg = Message(
            role=MessageRole.TOOL_RESULT,
            content=result,
        )
        tool_result_msg.tool_name = tool_name
        tool_result_msg.enabled = True
        self.add_message_to_context(tool_result_msg)
        
        # Display tool result in GUI
        self.gui.display_agent_response(...)
    except Exception as e:
        self.gui.display_error(f"Error handling tool call: {e}")
```

### Tool Execution Routing: `execute_tool(tool_name, tool_input)`

**Strategy:**
1. Check if CLIENT tool → ClientToolExecutor
2. Check if CODE_ANALYSIS → ServerToolExecutor
3. Check if SERVER tool → ServerToolExecutor
4. Else → Unknown tool error

**Supported CLIENT Tools:**
- read_file, list_directory, write_file, get_file_info, search_files

**Code Analysis Tools (SERVER):**
- analyze_syntax, find_functions, find_classes, find_imports, suggest_refactoring

### ResponseHandler Integration (lines 588-622)

```python
handler = ResponseHandler(
    on_content=lambda text: self._handle_stream_content(text),
    on_thinking=lambda text: self._display_thinking(text),
    on_tool_call=lambda name, args: self.handle_tool_call(name, args),  # ← HERE
    on_tool_result=lambda id, result: self.gui.display_agent_response(...),
    on_error=lambda msg, code: self.gui.display_error(...),
)

# Stream through Agentix
for chunk in self.agentix_adapter.process_prompt_generator(
    prompt, shared_context, classification
):
    handler.process_chunk(chunk)  # Routes to callbacks
```

### Callbacks/Handlers Defined

1. **`_handle_submit()`**: User clicked submit → `stream_ollama_response()`
2. **`_handle_interrupt()`**: User clicked interrupt → `interrupt_streaming()`
3. **`_handle_attachment_toggle()`**: Attachment checkbox → track enabled files
4. **`on_history_attachment_toggle()`**: History attachment → update enabled list
5. **`_handle_stream_content()`**: Ensure header before content
6. **`_display_thinking()`**: Display thinking with header on first call
7. **Tool-related callbacks (see ResponseHandler above)**

---

## 10. System Prompts Referencing Tools

### Location: `/Projects/agentX/system_prompts/`

**prompt_classification.md** (Main classification system prompt)
- Classifies intent into: conversation, simple_action, complex_action, safety_issue
- Routes to: respond_directly, single_tool, invoke_planner, escalate
- Does NOT directly use tools
- Used to route to next_step handlers

**Other System Prompts:**
- `planner_prompt.md`: Planning prompt (if used)
- `python_coder.md`: Code generation (if used)
- `structured_response.md`: Response format (if used)
- `modifier_class_decorator.md`: Decorator for modification (if used)

**Current State:**
- Classification determines whether tools are needed
- But single_tool and invoke_planner routes are NOT IMPLEMENTED
- So tool selection doesn't happen

---

## 11. Tests for Tool-Related Functionality

### Test Files

**1. `/Projects/agentX/tests/test_phase4_tool_handling.py`**
- Tool call message storage
- Tool result message storage
- Tool execution in session
- Status: ✅ Basic tests pass

**2. `/Projects/agentX/tests/test_phase5_tool_execution.py`**
- ClientToolExecutor tests:
  - `test_read_file()`: ✅
  - `test_write_file()`: ✅
  - `test_list_directory()`: ✅
  - `test_get_file_info()`: ✅
  - `test_search_files()`: ✅
  - `test_path_security()`: ✅
- Status: ✅ All passing

**3. `/Projects/agentX/tests/test_code_analysis_tools.py`**
- Code analysis tool tests (CST/AST)
- Status: Likely exists

**4. `/Projects/agentX/tests/test_phase6_advanced_tools.py`**
- Advanced tool registry tests
- Server tool executor tests
- Status: Likely exists

**5. `/Projects/agentX/tests/test_phase7_streaming.py`**
- Streaming with tool calls
- ResponseHandler processing
- Status: Exists but coverage unclear

**Test Coverage Gaps:**
- ❌ Multi-turn tool use NOT tested
- ❌ Tool results fed back to LLM NOT tested
- ❌ Agentic loops NOT tested
- ❌ Complex multi-step reasoning with tools NOT tested

---

## 12. MISSING OR INCOMPLETE IMPLEMENTATION

### Critical Gaps

**1. MULTI-TURN TOOL USE (HIGHEST PRIORITY)**

**Problem:** Tool results are NOT fed back to LLM for continued reasoning

**Current Flow:**
```
LLM generates TOOL_CALL
→ Tool executes
→ Result stored in context
→ Result displayed in GUI
→ STOP ✗ (Should continue)
```

**Expected Flow:**
```
LLM generates TOOL_CALL
→ Tool executes
→ Result stored in context
→ Result displayed in GUI
→ Build new request: [history] + [tool result]
→ Send to LLM again
→ LLM reasons about result
→ Repeat if more tools needed
```

**Missing Code:**
- No loop to re-invoke LLM after tool execution
- `single_tool` next_step handler is just `pass`
- `invoke_planner` next_step handler returns placeholder
- Tool results not included in new LLM requests

**Where it should happen:**
- `src/agentix/next_steps/single_tool.py` (line 10-14)
- `src/agentix/next_steps/invoke_planner.py` (incomplete)
- `src/agentix/bridge/bridge.py` (lines 315-322) - Tool execution is stubbed

---

**2. TOOL SELECTION (SECONDARY PRIORITY)**

**Current State:**
- Prompt is classified (conversation, simple_action, complex_action)
- Classification suggests single_tool route
- But NO IMPLEMENTATION to select which tool to use

**Missing:**
- Tool selection logic based on prompt
- Tool availability checking
- Parameter validation/extraction
- Fallback if tool fails

**Where it should happen:**
- `src/agentix/next_steps/single_tool.py`
- Bridge should extract tool calls from LLM

---

**3. PLANNING AND MULTI-STEP EXECUTION (TERTIARY)**

**Current State:**
- `invoke_planner` classification exists
- No planner implementation
- Returns placeholder chunk

**Missing:**
- Plan generation from prompt
- Step decomposition
- Step execution loop
- Result aggregation
- Error recovery

**Where it should happen:**
- `src/agentix/next_steps/invoke_planner.py`
- `src/agentix/next_steps/plan_steps.py`
- `src/agentix/next_steps/take_steps.py`

---

**4. TOOL ENABLEMENT MANAGEMENT**

**Current State:**
```python
def set_enabled_tools(self, enabled_tools: list[str]) -> None:
    """Set which tools are enabled (stub for config/session integration)."""
    pass  # In a real app, store this in config/session
```

**Missing:**
- Persistence of enabled tools
- Integration with GUI tool panel
- Per-session tool filtering
- Role-based tool access

---

**5. TOOL ID TRACKING**

**Issue:** `tool_id` not available in ResponseChunk
- Response handler uses `tool_name` as ID
- Fails with multiple concurrent tool calls
- No way to match results to calls

**Missing:**
- Unique `tool_id` generation
- Tool call tracking in context
- Result matching to specific tool invocation

---

### TODOs in Codebase

**1. Bridge tool execution (CRITICAL)**
```python
# src/agentix/bridge/bridge.py, line 315
def _stream_tool_response(...):
    """
    Stream response involving a single tool call.
    
    Note: Full tool execution not yet implemented.
    Returns placeholder for now.
    """
    # TODO: Implement actual tool execution
    yield ResponseChunk(...)
```

**2. Bridge planner (CRITICAL)**
```python
# src/agentix/bridge/bridge.py, line 342
def _stream_planned_response(...):
    """
    Stream response using multi-step planning.
    
    Note: Full planner not yet implemented.
    Returns placeholder for now.
    """
    # TODO: Implement actual planner
    yield ResponseChunk(...)
```

**3. Model hardcoding**
```python
# src/agentix/tools/codeagent.py (2x)
# TODO: need to get the model from the agentx config instead of hardcoding it here
```

**4. BaseTool stub**
```python
# src/shared/models/tools.py, line 326
async def execute(self, **kwargs) -> ToolResponse:
    """Execute the tool with the given arguments."""
    raise NotImplementedError("Tool execution not implemented")
```

**5. Tool enablement stub**
```python
# src/shared/models/tools.py, lines 360-364
def set_enabled_tools(self, enabled_tools: list[str]) -> None:
    """Set which tools are enabled (stub for config/session integration)."""
    pass  # In a real app, store this in config/session
```

**6. Empty next_step handlers**
```python
# src/agentix/next_steps/single_tool.py
def single_tool(...):
    """Handle the single tool next step."""
    pass  # NOT IMPLEMENTED
```

---

### NotImplementedError Instances

1. **BaseTool.execute()** (tools.py:326)
   - Raises NotImplementedError
   - Should be overridden by subclasses
   - OK for abstract base

---

## 13. AGENTIX MIDDLEWARE LAYER ARCHITECTURE

### Design Pattern

**Location:** `/Projects/agentX/src/agentix/`

**Pattern:** Classification → Routing → Execution

### Layer Components

**1. Classification Layer** (bridge/classify_prompt.py)
- Analyzes user intent
- Returns PromptClassificationResponse
- Non-blocking (uses LLM for classification)
- Routes to next_step handler

**2. Routing Layer** (bridge/bridge.py)
- Routes based on NextStep enum:
  - `respond_directly` → _stream_direct_response()
  - `single_tool` → _stream_tool_response() [STUB]
  - `invoke_planner` → _stream_planned_response() [STUB]
  - `escalate` → Error chunk
- Each route returns Iterator[ResponseChunk]

**3. Execution Layer**
- `_stream_direct_response()`: Query Ollama with context
- `_stream_tool_response()`: Execute tool and respond [NOT DONE]
- `_stream_planned_response()`: Multi-step execution [NOT DONE]

**4. Tool Discovery** (tools/describe_tools/)
- Extracts tools from codebase (CST/AST)
- Converts to OpenAI format
- Available via bridge.get_available_tools()

### Data Flow Through Middleware

```
User Prompt (string)
    ↓ [Classification]
PromptClassificationResponse {
    intent: Intent enum,
    needs_clarification: bool,
    missing_fields: [str],
    reasoning_summary: str,
    next_step: NextStep enum
}
    ↓ [Routing]
NextStep matches:
    - respond_directly:
        Query LLM with context
        → Iterator[ResponseChunk]
    - single_tool:
        [NOT IMPLEMENTED]
    - invoke_planner:
        [NOT IMPLEMENTED]
    - escalate:
        → ERROR ResponseChunk
    ↓
Iterator[ResponseChunk] {
    type: ChunkType,
    content: str,
    tool_name: str (if TOOL_CALL/TOOL_RESULT),
    tool_input: dict (if TOOL_CALL),
    tool_output: Any (if TOOL_RESULT),
    classification: dict (if CLASSIFICATION),
    ...
}
    ↓ [AgentX Handler]
ResponseHandler processes each chunk
→ Callbacks trigger GUI updates
→ Tool calls execute via ClientToolExecutor/ServerToolExecutor
→ Results stored in context
```

### Integration with AgentX

**AgentXSession → AgentixBridgeAdapter → AgentixBridge**

```python
# session.py
self.agentix_adapter = create_adapter(config)  # AgentixBridgeAdapter

# _stream_via_agentix()
for chunk in self.agentix_adapter.process_prompt_generator(
    prompt, shared_context, classification
):
    handler.process_chunk(chunk)
```

---

## 14. SYSTEM DESIGN ISSUES AND RECOMMENDATIONS

### Issue 1: No Agentic Loop
**Problem:** System generates single responses without reasoning about tool outputs

**Recommendation:**
Implement loop in `_stream_tool_response()`:
```python
1. Get LLM response with tools available
2. If tool call detected:
   a. Execute tool
   b. Yield TOOL_RESULT chunk
   c. Re-invoke LLM with [messages] + [tool result]
   d. Loop back to step 2
3. If no more tools:
   a. Yield CONTENT chunk with final response
   b. Break
```

### Issue 2: Tool Selection Not Implemented
**Problem:** Classification says "use a tool" but no logic selects which tool

**Recommendation:**
- Implement tool selection in bridge based on prompt
- Extract tool capabilities into LLM system prompt
- Use LLM to select tool + parameters
- Validate before execution

### Issue 3: Multi-step Planning Disabled
**Problem:** Complex prompts classified as "invoke_planner" but planner not implemented

**Recommendation:**
- Implement planner in `invoke_planner.py`
- Generate execution plan
- Execute steps with feedback
- Refine plan if steps fail

### Issue 4: Tool Results Not in LLM Context
**Problem:** Tool results stored locally but not sent back to LLM

**Recommendation:**
- After tool execution, rebuild context with result
- Send new request to LLM with results
- LLM can reason about output
- Close the feedback loop

### Issue 5: Tool ID Tracking
**Problem:** Multiple concurrent tool calls not trackable

**Recommendation:**
- Add UUID to each tool call
- Track in ResponseChunk as `tool_id`
- Match results to calls using tool_id
- Handle out-of-order results

---

## SUMMARY TABLE

| Component | Status | Location | Issues |
|-----------|--------|----------|--------|
| Tool definitions | ✅ | shared/models/tools.py | Enablement management stubbed |
| Client executor | ✅ | agentx/integration/ | Works well |
| Server executor | ✅ | agentx/integration/ | Code analysis only |
| Classification | ✅ | agentix/bridge/ | Works well |
| Direct response | ✅ | agentix/bridge/ | Works well |
| Tool execution | ⚠️ | agentix/bridge/ | STUB - Not implemented |
| Planning | ❌ | agentix/next_steps/ | STUB - Not implemented |
| Multi-turn tool use | ❌ | session.py | NOT IMPLEMENTED |
| Tool feedback loop | ❌ | Everywhere | MISSING |
| Tests | ⚠️ | tests/ | No multi-turn tests |

