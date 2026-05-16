# AgentX Tool System Architecture Diagrams

## 1. COMPLETE TOOL EXECUTION FLOW (as implemented)

```
┌─────────────────┐
│  User Input     │
│   "Read file.py"│
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│  AgentXSession._stream_via_agentix()            │
│  ├─ Capture user input                          │
│  ├─ Build shared context from history           │
│  └─ Call agentix_adapter.process_prompt_generator()
└────────┬────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│  AgentixBridgeAdapter.process_prompt_generator()│
│  └─ Yield from bridge.process_prompt_streaming()
└────────┬────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────┐
│  AgentixBridge.process_prompt_streaming()       │
│  ├─ Auto-classify if enabled                    │
│  └─ Route based on next_step                    │
└────────┬────────────────────────────────────────┘
         │
    ┌────┴────────────────────────────────┐
    │ ROUTE BASED ON CLASSIFICATION       │
    └────┬────────────────┬──────────┬─────┘
         │                │          │
    RESPOND_DIRECTLY  SINGLE_TOOL INVOKE_PLANNER
         │                │          │
         ▼                ▼          ▼
    ┌────────────┐   ┌────────┐   ┌──────────┐
    │ STREAM LLM │   │ STUB   │   │  STUB    │
    │  RESPONSE  │   │ TODO:  │   │  TODO:   │
    │    ✅      │   │ Impl.  │   │  Impl.   │
    │            │   │ Tools  │   │ Planning │
    │            │   │   ✗    │   │    ✗     │
    └────────────┘   └────────┘   └──────────┘
         │                │          │
         └────────┬───────┴──────────┘
                  │
                  ▼
    ┌─────────────────────────────────────────┐
    │  Iterator[ResponseChunk]                │
    ├─ THINKING chunks                        │
    ├─ CONTENT chunks                         │
    ├─ TOOL_CALL chunks (from LLM)           │
    ├─ TOOL_RESULT chunks (NONE - see gap)   │
    ├─ CLASSIFICATION chunks                 │
    ├─ ERROR chunks                          │
    └─ DONE chunk                            │
    └─────────────────────────────────────────┘
         │
         ▼
    ┌──────────────────────────────────────────┐
    │  ResponseHandler.process_chunk()         │
    │  ├─ on_content() → display text          │
    │  ├─ on_thinking() → display reasoning    │
    │  ├─ on_tool_call() → EXECUTE             │
    │  ├─ on_tool_result() → display result    │
    │  ├─ on_classification() → display        │
    │  └─ on_error() → display error           │
    └──────────┬───────────────────────────────┘
               │
         ┌─────┴───────┐
         │ TOOL CALL?  │
         └─────┬───────┘
          YES  │  NO
              │
              ▼
        ┌──────────────────────────────────┐
        │ Session.handle_tool_call()       │
        │ ├─ Store TOOL_CALL message       │
        │ ├─ Execute tool (route by type)  │
        │ ├─ Store TOOL_RESULT message     │
        │ ├─ Display results in GUI        │
        │ └─ ❌ MISSING: Re-invoke LLM     │
        └──────────────────────────────────┘
               │
    ┌──────────┴──────────────┐
    │ EXECUTE TOOL            │
    └──────────┬──────────────┘
               │
    ┌──────────┴──────────────────────────┐
    │ Which tool type?                    │
    └──────────┬──────────────────────────┘
               │
    ┌──────────┼──────────────┐
    │          │              │
CLIENT_TOOLS  │         SERVER_TOOLS
    │          │              │
    ▼          ▼              ▼
┌────────┐  ┌──────┐  ┌────────────────┐
│ CLIENT │  │OTHER │  │ SERVER TOOL    │
│EXECUTOR│  │      │  │ EXECUTOR       │
│        │  │      │  │ ├─ Code Anal.  │
│• read  │  │      │  │ ├─ API calls   │
│• write │  │      │  │ └─ Compute     │
│• list  │  │      │  │                │
│• search│  │      │  │                │
└────────┘  └──────┘  └────────────────┘
    │          │              │
    └──────────┴──────────────┘
               │
               ▼
        ┌────────────────┐
        │ Result String  │
        │ (tool output)  │
        └────────────────┘
               │
               ▼
        ┌────────────────────────────┐
        │ Store in Context:          │
        │ - TOOL_RESULT message      │
        │ - role: MessageRole.TOOL_R │
        │ - content: result          │
        │ - enabled: true            │
        └────────────────────────────┘
               │
               ▼
        ┌────────────────────────────┐
        │ ❌ MISSING STEP            │
        │                            │
        │ Should:                    │
        │ 1. Build new request:      │
        │    [history] +             │
        │    [tool result]           │
        │ 2. Call LLM again          │
        │ 3. Let LLM reason about    │
        │    tool output             │
        │ 4. Loop if more tools      │
        │    needed                  │
        └────────────────────────────┘
               │
               ▼
        ┌────────────────────────────┐
        │ What Actually Happens:     │
        │ STOP - Stream ends ✗       │
        │                            │
        │ User sees:                 │
        │ - Tool executed            │
        │ - Result displayed         │
        │ - But LLM never saw it!    │
        └────────────────────────────┘
```

---

## 2. TOOL EXECUTION CONTEXT ROUTING

```
┌──────────────────────────────────────────────────────┐
│  Tool Execution Router                               │
│  (Session.execute_tool)                              │
└──────┬───────────────────────────────────────────────┘
       │
       ▼
    ┌─────────────────────────┐
    │ Tool Name?              │
    └──┬───────────────┬──────┘
       │               │
   CLIENT_TOOLS    SERVER_TOOLS
       │               │
       ├─read_file     ├─ analyze_syntax
       ├─write_file    ├─ find_functions
       ├─list_dir      ├─ find_classes
       ├─get_file_info ├─ find_imports
       ├─search_files  └─ suggest_refactor
       │
       ▼
    ┌─────────────────────────────────────────────┐
    │ ClientToolExecutor                          │
    │ ├─ Base path security check                 │
    │ ├─ Execute tool                             │
    │ ├─ Max 50KB output                          │
    │ └─ Return result as string                  │
    └─────────────────────────────────────────────┘
       │
       │                  ┌──────────────────────────┐
       │                  │ ServerToolExecutor       │
       │                  │ ├─ Code analysis (local) │
       │                  │ ├─ Server tools (remote) │
       │                  │ └─ Format for display    │
       │                  └──────────────────────────┘
       │                      │
       └──────────────────────┘
       │
       ▼
    ┌─────────────────────────────────────────────┐
    │ Tool Result (String)                        │
    │ ├─ Success: tool output                     │
    │ ├─ Error: error message                     │
    │ └─ Limit: 50KB max                          │
    └─────────────────────────────────────────────┘
```

---

## 3. MESSAGE ROLE MAPPING FOR LLM

```
┌──────────────────────────────────────────────────────┐
│ Message Role → LLM API Role Mapping                  │
│ (from Message.to_llm_dict)                           │
└──────────────────────────────────────────────────────┘

MessageRole.USER        → "user"
MessageRole.ASSISTANT   → "assistant"
MessageRole.SYSTEM      → "system"
MessageRole.THINKING    → "assistant"    (internal reasoning)
MessageRole.TOOL_CALL   → "assistant"    (model calls tool)
MessageRole.TOOL_RESULT → "user"         (tool result as input)
MessageRole.TOOL_RESULT → "user"         (not sent back! ❌)

Example conversation:

1. User: "Read file.py"
   {"role": "user", "content": "Read file.py"}
    
2. Assistant: (LLM response) "I'll read the file"
   {"role": "assistant", "content": "I'll read the file"}
    
3. Tool Call: read_file("file.py")
   Stored as: {"role": "assistant", "content": "..."}
    
4. Tool Result: "class Foo: ..."
   Stored as: {"role": "user", "content": "class Foo: ..."}
   
5. ❌ MISSING: Re-invoke LLM with messages 1-4
   Should send: {"messages": [1, 2, 3, 4], "model": "llama3.2"}
   Currently: Stream just stops!
```

---

## 4. CLASSIFICATION → ROUTING → EXECUTION

```
┌────────────────────────────────────────────────────┐
│ User Prompt Classification                         │
│ (AgentixBridge.classify_prompt)                    │
└────────┬──────────────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────────────┐
    │ PromptClassificationResponse:            │
    │ ├─ intent: Intent enum                   │
    │ ├─ needs_clarification: bool             │
    │ ├─ missing_fields: [str]                 │
    │ ├─ reasoning_summary: str                │
    │ └─ next_step: NextStep enum              │
    └────┬──────────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────────────┐
    │ NEXT STEP ROUTES                        │
    └────┬────────┬────────────┬──────────────┘
         │        │            │
    respond_  single_  invoke_      escalate
    directly   tool    planner
         │        │            │          │
         ▼        ▼            ▼          ▼
    ┌────┐   ┌────┐       ┌────┐    ┌──────┐
    │ ✅ │   │❌ │       │❌ │    │ ✅  │
    │    │   │    │       │    │    │      │
    └────┘   └────┘       └────┘    └──────┘
     IMPL    STUB         STUB       ERROR
     
    DIRECT   TOOL_EXEC    PLANNING   SAFETY
    RESPONSE NOT IMPL     NOT IMPL   ESCALATE


Example Intent → NextStep Mapping:

"conversation"      → respond_directly     ✅
"simple_action"     → single_tool          ❌ (stub)
"complex_action"    → invoke_planner       ❌ (stub)
"safety_issue"      → escalate             ✅
```

---

## 5. RESPONSE HANDLER ARCHITECTURE

```
┌──────────────────────────────────────────────────┐
│  AgentXSession._stream_via_agentix()             │
└──┬───────────────────────────────────────────────┘
   │
   ▼
┌──────────────────────────────────────────────────┐
│ ResponseHandler(                                 │
│   on_content=λ display_content,                  │
│   on_thinking=λ display_thinking,                │
│   on_tool_call=λ handle_tool_call,       ← KEY  │
│   on_tool_result=λ display_result,               │
│   on_classification=λ display_classification,    │
│   on_error=λ display_error,                      │
│   on_done=λ cleanup                              │
│ )                                                │
└──┬───────────────────────────────────────────────┘
   │
   ▼
┌──────────────────────────────────────────────────┐
│  For each ResponseChunk from Agentix:            │
│  handler.process_chunk(chunk)                    │
└──┬───────────────────────────────────────────────┘
   │
   ├─ CONTENT chunk
   │  └─ on_content(chunk.content)
   │     └─ GUI displays text
   │
   ├─ THINKING chunk
   │  └─ on_thinking(chunk.content)
   │     └─ GUI displays reasoning
   │
   ├─ TOOL_CALL chunk                    ← HERE
   │  └─ on_tool_call(chunk.tool_name, chunk.tool_input)
   │     └─ Session.handle_tool_call()
   │        ├─ Store TOOL_CALL message
   │        ├─ Execute tool
   │        ├─ Store TOOL_RESULT message
   │        └─ ❌ MISSING: Re-invoke LLM
   │
   ├─ TOOL_RESULT chunk (never comes from Agentix ✗)
   │  └─ on_tool_result(id, result)
   │     └─ GUI displays result
   │
   ├─ CLASSIFICATION chunk
   │  └─ on_classification(classification)
   │     └─ GUI displays metadata
   │
   ├─ ERROR chunk
   │  └─ on_error(content, code)
   │     └─ GUI displays error
   │
   └─ DONE chunk
      └─ on_done()
         └─ Stream complete

Accumulators:
├─ content_buffer: [str]
├─ thinking_buffer: [str]
├─ tool_calls: [{"name": str, "input": dict}]
└─ tool_results: [{"name": str, "result": str}]

Final: to_message() → Message with metadata
```

---

## 6. CONTEXT AND MESSAGE PERSISTENCE

```
┌────────────────────────────────────────────────────┐
│  AgentX Context (shared.models.context)            │
│  ├─ messages: [(timestamp, Message)]               │
│  ├─ path: context_folder (for persistence)         │
│  └─ Methods:                                       │
│     ├─ add_message(msg, ts)                        │
│     ├─ get_enabled_messages()                      │
│     └─ to_llm_messages() → LLM API format          │
└────┬──────────────────────────────────────────────┘
     │
     ▼
┌────────────────────────────────────────────────────┐
│  Message Types in Context                          │
│                                                    │
│  1. USER: User input                               │
│     role="user"                                    │
│     content="Read file.py"                         │
│                                                    │
│  2. ASSISTANT: LLM response                        │
│     role="assistant"                               │
│     content="Here's the file..."                   │
│                                                    │
│  3. THINKING: LLM reasoning (if supported)         │
│     role="thinking"                                │
│     content="I need to read..."                    │
│     enabled=false (usually hidden)                 │
│                                                    │
│  4. TOOL_CALL: Tool invocation                     │
│     role="tool_call"                               │
│     tool_name="read_file"                          │
│     tool_input={"path": "file.py"}                 │
│     enabled=true                                   │
│                                                    │
│  5. TOOL_RESULT: Tool execution result             │
│     role="tool_result"                             │
│     tool_name="read_file"                          │
│     content="class Foo: ..."                       │
│     enabled=true                                   │
│                                                    │
│  6. CLASSIFICATION: Classification metadata        │
│     role="classification" (internal)               │
│     metadata={intent, next_step, ...}              │
│     enabled=false                                  │
│                                                    │
└────────────────────────────────────────────────────┘

Persistence Flow:

Message created
├─ on_context_add
├─ save(context_path/TIMESTAMP_ROLE.json)
├─ refresh_gui
└─ Available for next request

✅ Messages ARE saved
✅ Messages ARE loaded from history
❌ Messages NOT fed back to LLM in agentic loop
```

---

## 7. TOOL REGISTRY AND DISCOVERY

```
┌────────────────────────────────────────────────────┐
│  Tool Discovery and Registration                   │
└────┬─────────────────────────────────────────────┘
     │
     ├─ ClientToolRegistry
     │  └─ Registered by: (hardcoded)
     │     ├─ read_file
     │     ├─ write_file
     │     ├─ list_directory
     │     ├─ get_file_info
     │     └─ search_files
     │
     ├─ ServerToolRegistry
     │  └─ Populated from: AgentixBridge
     │     ├─ CST tools (code analysis)
     │     ├─ AST tools (semantic analysis)
     │     └─ (Future: API integration tools)
     │
     └─ AdvancedToolRegistry
        ├─ Wraps ServerToolRegistry
        ├─ Caches tool info
        ├─ Filters by category
        └─ Initialized on demand

Methods:
├─ register(tool)
├─ get(name)
├─ list_definitions() → [ToolDefinition]
├─ list_by_context(client/server/either)
├─ get_enabled_tools()          ← STUB
├─ set_enabled_tools()          ← STUB
├─ is_tool_enabled()            ← STUB
├─ to_openai_format() → [dict]
└─ async execute(request) → ToolResponse

ToolDefinition Format (OpenAI):

{
  "type": "function",
  "function": {
    "name": "read_file",
    "description": "Read file contents",
    "parameters": {
      "type": "object",
      "properties": {
        "path": {"type": "string", "description": "..."}
      },
      "required": ["path"]
    }
  }
}
```

---

## 8. DATA FLOW: FROM USER PROMPT TO CONTEXT STORAGE

```
User Prompt
   │
   ▼
AgentXSession._stream_via_agentix()
   │
   ├─1. Create user message
   │   └─ Message(role=USER, content=prompt, enabled=true)
   │
   ├─2. Add to context
   │   └─ context.add_message(user_msg)
   │      └─ Persisted to disk
   │
   ├─3. Build LLM request
   │   └─ messages = [history messages].to_llm_dict()
   │      └─ Includes TOOL_RESULT as "user" role ✓
   │
   ├─4. Stream from Agentix
   │   └─ ResponseChunk iterator
   │
   ├─5. Process chunks
   │   └─ ResponseHandler.process_chunk()
   │
   ├─6. On TOOL_CALL chunk
   │   ├─ Create tool call message
   │   ├─ Add to context
   │   ├─ Execute tool
   │   ├─ Create tool result message
   │   ├─ Add to context
   │   └─ ❌ STOP (should continue)
   │
   ├─7. On CONTENT chunk
   │   ├─ Create assistant message
   │   └─ Add to context
   │
   └─8. Finalize
       └─ All messages persisted
          ├─ User message
          ├─ Tool call messages (if any)
          ├─ Tool result messages (if any)
          ├─ Assistant message
          └─ Ready for next iteration

❌ MISSING: After step 6
   - Rebuild context with tool result
   - Send new request to LLM
   - LLM can reason about tool output
   - Close the agentic loop
```

---

## KEY FILES CROSS-REFERENCE

```
┌──────────────────────────────────────────┐
│  Tool Definition & Types                 │
│  src/shared/models/tools.py              │
│  ├─ ToolExecutionContext                 │
│  ├─ ToolDefinition                       │
│  ├─ ToolRequest                          │
│  ├─ ToolResponse                         │
│  ├─ ITool (protocol)                     │
│  ├─ BaseTool (abstract)                  │
│  └─ ToolRegistry                         │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Tool Execution (AgentX)                 │
│  src/agentx/integration/                 │
│  ├─ client_tool_executor.py              │
│  │  └─ File operations                   │
│  ├─ server_tool_executor.py              │
│  │  ├─ ServerToolExecutor                │
│  │  ├─ CodeAnalysisTool                  │
│  │  └─ AdvancedToolRegistry              │
│  ├─ response_handler.py                  │
│  │  └─ ResponseHandler                   │
│  └─ agentix_bridge_adapter.py            │
│     └─ AgentixBridgeAdapter              │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Tool Routing (Agentix)                  │
│  src/agentix/bridge/                     │
│  ├─ bridge.py                            │
│  │  ├─ AgentixBridge.classify_prompt()   │
│  │  ├─ process_prompt_streaming()        │
│  │  ├─ _stream_direct_response() ✅      │
│  │  ├─ _stream_tool_response() ❌        │
│  │  └─ _stream_planned_response() ❌     │
│  └─ classify_prompt.py                   │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Tool Steps (Agentix)                    │
│  src/agentix/next_steps/                 │
│  ├─ single_tool.py ❌ (stub)             │
│  ├─ invoke_planner.py ❌ (stub)          │
│  ├─ plan_steps.py (incomplete)           │
│  └─ take_steps.py (incomplete)           │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Response Types                          │
│  src/shared/models/response.py           │
│  ├─ ChunkType (enum)                     │
│  └─ ResponseChunk (dataclass)            │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Session Coordination                    │
│  src/agentx/session.py                   │
│  ├─ process_prompt()                     │
│  ├─ stream_ollama_response()             │
│  ├─ execute_tool()                       │
│  ├─ handle_tool_call()                   │
│  └─ _stream_via_agentix()                │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Message & Context                       │
│  src/shared/models/                      │
│  ├─ message.py                           │
│  │  └─ MessageRole (enum)                │
│  ├─ context.py                           │
│  │  └─ Context (manager)                 │
│  └─ response.py                          │
│     └─ ResponseChunk                     │
└──────────────────────────────────────────┘
```

