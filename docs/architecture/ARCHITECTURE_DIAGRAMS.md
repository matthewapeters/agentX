# AgentX Tool System Architecture Diagrams

## 1. COMPLETE TOOL EXECUTION FLOW

```
┌─────────────────┐
│  User Input     │
│   "Read file.go"│
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│  Orchestrator                           │
│  ├─ Capture user input                  │
│  ├─ Build context from session history  │
│  └─ Submit to LLMBridge                 │
└────────┬────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│  LLMBridge                              │
│  └─ Stream response from model backend  │
└────────┬────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│  Ollama (model backend)                 │
│  ├─ Classify / route prompt             │
│  └─ Stream response chunks              │
└────────┬────────────────────────────────┘
         │
    ┌────┴────────────────────────────────┐
    │ ROUTE BASED ON PROMPT INTENT        │
    └────┬────────────────┬──────────┬────┘
         │                │          │
    RESPOND_DIRECTLY  SINGLE_TOOL INVOKE_PLANNER
         │                │          │
         ▼                ▼          ▼
    ┌────────────┐   ┌────────────┐  ┌──────────┐
    │ STREAM LLM │   │ TOOL EXEC  │  │ PLANNING │
    │  RESPONSE  │   │  + LOOP    │  │  + LOOP  │
    └────────────┘   └────────────┘  └──────────┘
         │                │          │
         └────────┬───────┴──────────┘
                  │
                  ▼
    ┌─────────────────────────────────────────┐
    │  ResponseChunk stream                   │
    ├─ THINKING chunks                        │
    ├─ CONTENT chunks                         │
    ├─ TOOL_CALL chunks                       │
    ├─ TOOL_RESULT chunks                     │
    ├─ ERROR chunks                           │
    └─ DONE chunk                             │
    └─────────────────────────────────────────┘
         │
         ▼
    ┌──────────────────────────────────────────┐
    │  SurfaceManager / Event Coordination     │
    │  ├─ on_content()  → OutputSurface        │
    │  ├─ on_thinking() → OutputSurface        │
    │  ├─ on_tool_call() → ToolDispatcher      │
    │  ├─ on_tool_result() → OutputSurface     │
    │  └─ on_error()    → OutputSurface        │
    └──────────┬───────────────────────────────┘
               │
         ┌─────┴───────┐
         │ TOOL CALL?  │
         └─────┬───────┘
          YES  │  NO → surface renders response
              │
              ▼
        ┌──────────────────────────────────┐
        │ ToolDispatcher                   │
        │ ├─ Store TOOL_CALL event         │
        │ ├─ Evaluate policy               │
        │ ├─ Execute tool (with approval)  │
        │ ├─ Store TOOL_RESULT event       │
        │ └─ Re-invoke LLMBridge          │
        └──────────────────────────────────┘
               │
    ┌──────────┴──────────────────────────┐
    │ EXECUTE TOOL                        │
    └──────────┬──────────────────────────┘
               │
    ┌──────────┼──────────────┐
    │          │              │
 LOCAL TOOLS   │          REMOTE TOOLS
    │          │              │
    ▼          ▼              ▼
┌────────┐  ┌──────┐  ┌──────────────────┐
│ LOCAL  │  │OTHER │  │ REMOTE TOOL      │
│EXECUTOR│  │      │  │ EXECUTOR         │
│        │  │      │  │ ├─ Code analysis  │
│• read  │  │      │  │ ├─ API calls      │
│• write │  │      │  │ └─ Compute        │
│• list  │  │      │  │                  │
│• search│  │      │  │                  │
└────────┘  └──────┘  └──────────────────┘
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
        │ Persist to session events  │
        │ - TOOL_RESULT event JSON   │
        │ - epoch + session_id       │
        │ - tool name + output       │
        └────────────────────────────┘
               │
               ▼
        ┌────────────────────────────┐
        │ Re-invoke LLMBridge with   │
        │ updated context (agentic   │
        │ loop continues until DONE) │
        └────────────────────────────┘
```

---

## 2. TOOL EXECUTION CONTEXT ROUTING

```
┌──────────────────────────────────────────────────────┐
│  Tool Execution Router                               │
│  (ToolDispatcher / internal/tools)                   │
└──────┬───────────────────────────────────────────────┘
       │
       ▼
    ┌─────────────────────────┐
    │ Tool Name?              │
    └──┬───────────────┬──────┘
       │               │
  LOCAL TOOLS    REMOTE/REGISTERED TOOLS
       │               │
       ├─ read_file     ├─ code_analysis
       ├─ write_file    ├─ find_symbols
       ├─ list_dir      ├─ find_imports
       ├─ get_file_info └─ (extensible)
       └─ search_files
       │
       ▼
    ┌─────────────────────────────────────────────┐
    │ LocalToolExecutor (internal/tools)          │
    │ ├─ Base path security check                 │
    │ ├─ Policy evaluation (allow/confirm/deny)   │
    │ ├─ Execute tool with timeout                │
    │ ├─ Max output size enforcement              │
    │ └─ Return result as structured response     │
    └─────────────────────────────────────────────┘
       │
       │                  ┌──────────────────────────┐
       │                  │ RemoteToolExecutor       │
       │                  │ ├─ Tool descriptor lookup │
       │                  │ ├─ Remote invocation      │
       │                  │ └─ Format for display     │
       │                  └──────────────────────────┘
       │                      │
       └──────────────────────┘
       │
       ▼
    ┌─────────────────────────────────────────────┐
    │ Tool Result                                 │
    │ ├─ Success: tool output                     │
    │ ├─ Error: error message                     │
    │ ├─ Policy denied: rejection notice          │
    │ └─ Timeout: timeout notice                  │
    └─────────────────────────────────────────────┘
```

---

## 3. MESSAGE ROLE MAPPING FOR LLM

```
┌──────────────────────────────────────────────────────┐
│ Message Role → LLM API Role Mapping                  │
│ (session event → Ollama API format)                  │
└──────────────────────────────────────────────────────┘

Role.USER        → "user"
Role.ASSISTANT   → "assistant"
Role.SYSTEM      → "system"
Role.THINKING    → "assistant"    (internal reasoning)
Role.TOOL_CALL   → "assistant"    (model calls tool)
Role.TOOL_RESULT → "user"         (tool result as input)

Example conversation:

1. User: "Read file.go"
   {"role": "user", "content": "Read file.go"}

2. Assistant: (LLM response) "I'll read the file"
   {"role": "assistant", "content": "I'll read the file"}

3. Tool Call: read_file("file.go")
   Stored as: {"role": "assistant", "content": "<tool_call>"}

4. Tool Result: "package main..."
   Stored as: {"role": "user", "content": "package main..."}

5. Re-invoke LLM with messages 1-4
   {"messages": [1, 2, 3, 4], "model": "<configured model>"}
   LLM reasons about tool output and responds or calls another tool
```

---

## 4. PROMPT ROUTING → EXECUTION

```
┌────────────────────────────────────────────────────┐
│ Prompt Router                                      │
│ (internal/prompting or LLMBridge classify step)    │
└────────┬──────────────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────────────┐
    │ PromptRoute:                             │
    │ ├─ intent: (conversation | action | plan)│
    │ ├─ needs_clarification: bool             │
    │ └─ route: (direct | tool_use | planning) │
    └────┬──────────────────────────────────────┘
         │
         ▼
    ┌─────────────────────────────────────────┐
    │ ROUTE OPTIONS                           │
    └────┬────────┬────────────┬──────────────┘
         │        │            │
    RESPOND_  TOOL_USE   INVOKE_
    DIRECTLY           PLANNER
         │        │            │
         ▼        ▼            ▼
    ┌────┐   ┌────────┐   ┌──────────┐
    │    │   │ TOOL   │   │ PLANNING │
    │    │   │ EXEC   │   │ + DAG    │
    │    │   │ LOOP   │   │ EXEC     │
    └────┘   └────────┘   └──────────┘
     STREAM   TOOL-USE    HIERARCHICAL
    RESPONSE  AGENTIC     TASK
              LOOP        EXECUTION

Example Intent → Route Mapping:

"conversation"   → respond_directly
"simple_action"  → tool_use
"complex_action" → invoke_planner
```

---

## 5. RESPONSE STREAM ARCHITECTURE

```
┌──────────────────────────────────────────────────┐
│  Orchestrator (session handling)                 │
└──┬───────────────────────────────────────────────┘
   │
   ▼
┌──────────────────────────────────────────────────┐
│ StreamCoordinator / Event Coordination Layer     │
│   on_content       → OutputSurface               │
│   on_thinking      → OutputSurface               │
│   on_tool_call     → ToolDispatcher      ← KEY   │
│   on_tool_result   → OutputSurface               │
│   on_error         → OutputSurface               │
│   on_done          → cleanup / persist           │
└──┬───────────────────────────────────────────────┘
   │
   ▼
┌──────────────────────────────────────────────────┐
│  For each ResponseChunk from LLMBridge:          │
│  coordinator.publish(chunk)                      │
└──┬───────────────────────────────────────────────┘
   │
   ├─ CONTENT chunk
   │  └─ OutputSurface displays text
   │
   ├─ THINKING chunk
   │  └─ OutputSurface displays reasoning (collapsed)
   │
   ├─ TOOL_CALL chunk
   │  └─ ToolDispatcher.execute(tool_name, tool_input)
   │     ├─ Policy check
   │     ├─ Execute tool
   │     ├─ Persist TOOL_RESULT event
   │     └─ Re-invoke LLMBridge with updated context
   │
   ├─ TOOL_RESULT chunk
   │  └─ OutputSurface displays result
   │
   ├─ ERROR chunk
   │  └─ OutputSurface displays error
   │
   └─ DONE chunk
      └─ Stream complete; finalize session events

Accumulators (per turn):
├─ content_buffer: []string
├─ thinking_buffer: []string
├─ tool_calls: []ToolCall
└─ tool_results: []ToolResult

Final: persisted as JSON events under session store
```

---

## 6. CONTEXT AND MESSAGE PERSISTENCE

```
┌────────────────────────────────────────────────────┐
│  Session Store (internal/session)                  │
│  ├─ session_id (canonical internal ID)             │
│  ├─ session_name (human-readable)                  │
│  ├─ events: append-only JSON event files           │
│  └─ Methods:                                       │
│     ├─ AppendEvent(event)                          │
│     ├─ EnabledEvents() → filtered for LLM          │
│     └─ ToLLMMessages() → model API format          │
└────┬──────────────────────────────────────────────┘
     │
     ▼
┌────────────────────────────────────────────────────┐
│  Event Types in Session                            │
│                                                    │
│  1. user_prompt: User input                        │
│     role="user"                                    │
│     content="Read file.go"                         │
│                                                    │
│  2. agent_response: LLM response                   │
│     role="assistant"                               │
│     content="Here's the file..."                   │
│                                                    │
│  3. thinking: LLM reasoning (if supported)         │
│     role="thinking"                                │
│     content="I need to read..."                    │
│     enabled=false (hidden from LLM context)        │
│                                                    │
│  4. tool_call: Tool invocation                     │
│     role="tool_call"                               │
│     tool_name="read_file"                          │
│     tool_input={"path": "file.go"}                 │
│     enabled=true                                   │
│                                                    │
│  5. tool_result: Tool execution result             │
│     role="tool_result"                             │
│     tool_name="read_file"                          │
│     content="package main..."                      │
│     enabled=true                                   │
│                                                    │
│  6. processing_state: Orchestrator status          │
│     metadata={phase, surface, ...}                 │
│     enabled=false (internal)                       │
│                                                    │
└────────────────────────────────────────────────────┘

Persistence Flow:

Event created
├─ Append to session event log
├─ Persist as epoch-prefixed JSON file
├─ Publish to Event Coordination Layer
└─ Available for context rebuild on next request

✅ Events ARE saved to disk
✅ Events ARE loaded for session replay
✅ Events ARE fed back to LLM in agentic loop (tool results → re-invoke)
```

---

## 7. TOOL REGISTRY AND DISCOVERY

```
┌────────────────────────────────────────────────────┐
│  Tool Registry (internal/tools)                    │
└────┬─────────────────────────────────────────────┘
     │
     ├─ BuiltinToolRegistry
     │  └─ Registered by config (tools.toml):
     │     ├─ read_file
     │     ├─ write_file
     │     ├─ list_directory
     │     ├─ get_file_info
     │     └─ search_files
     │
     ├─ RegisteredToolRegistry
     │  └─ Loaded from tool descriptor files:
     │     ├─ Code analysis tools
     │     ├─ API integration tools
     │     └─ (Extensible: new tool descriptors)
     │
     └─ PolicyEvaluator
        ├─ Blacklist (always forbidden)
        ├─ Session whitelist
        ├─ Global whitelist
        └─ Prompt user for approval on unknown

Methods:
├─ Register(tool)
├─ Get(name) → ToolDescriptor
├─ ListDefinitions() → []ToolDefinition
├─ GetEnabledTools() → filtered by policy
├─ SetEnabled(name, bool)
├─ EvaluatePolicy(cmd) → allow|confirm|deny
└─ Execute(request) → ToolResponse

ToolDefinition Format (OpenAI-compatible):

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
Orchestrator (internal/runtime)
   │
   ├─1. Create user_prompt event
   │   └─ {role: user, content: prompt, epoch: now}
   │
   ├─2. Persist event
   │   └─ session store append → disk
   │
   ├─3. Build LLM request
   │   └─ messages = session.EnabledEvents().ToLLMMessages()
   │      └─ Includes TOOL_RESULT as "user" role ✓
   │
   ├─4. Stream from LLMBridge → Ollama
   │   └─ ResponseChunk iterator
   │
   ├─5. Publish chunks to Event Coordination Layer
   │   └─ Per-subscriber delivery guaranteed
   │
   ├─6. On TOOL_CALL chunk
   │   ├─ Persist tool_call event
   │   ├─ Evaluate policy
   │   ├─ Execute tool (with approval if required)
   │   ├─ Persist tool_result event
   │   └─ Re-invoke LLMBridge with updated context
   │      └─ Agentic loop continues until DONE
   │
   ├─7. On CONTENT chunk
   │   ├─ Buffer content
   │   └─ Deliver to OutputSurface (streamed)
   │
   └─8. On DONE chunk
       └─ Persist agent_response event
          ├─ user_prompt event
          ├─ tool_call events (if any)
          ├─ tool_result events (if any)
          ├─ agent_response event
          └─ Ready for next turn
```

---

## KEY FILES CROSS-REFERENCE

```
┌──────────────────────────────────────────┐
│  Runtime Entrypoint                      │
│  cmd/agentx/                             │
│  └─ main.go (bootstrap + CLI wiring)     │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Orchestration                           │
│  internal/app/      (composition)        │
│  internal/runtime/  (lifecycle)          │
│  internal/cli/      (command parsing)    │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Surface Transport                       │
│  internal/transport/http/                │
│  ├─ SSE event stream handlers            │
│  ├─ Surface registration endpoints       │
│  └─ Submit endpoint (/submit)            │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Surface Implementations                 │
│  internal/surfaces/                      │
│  ├─ output/  (OutputSurface)             │
│  ├─ input/   (InputSurface)              │
│  └─ system/  (SystemSurface)             │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Tool Execution                          │
│  internal/tools/                         │
│  ├─ Registry + descriptor loader         │
│  ├─ Policy evaluator (allow/confirm/deny)│
│  ├─ Local executor (file ops)            │
│  └─ Remote executor (extensible)         │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  LLM Integration                         │
│  internal/llm/ollama/                    │
│  ├─ Ollama streaming client              │
│  ├─ Model listing (/api/tags)            │
│  └─ Context length lookup (/api/show)    │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Prompt Assembly                         │
│  internal/prompting/                     │
│  ├─ Prompt stage pipeline                │
│  └─ Procedural prompt loading            │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Session and Event Persistence           │
│  internal/session/                       │
│  ├─ Session identity (id + name)         │
│  ├─ Append-only JSON event writer        │
│  └─ Replay and context rebuild           │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│  Processing State                        │
│  internal/state/                         │
│  ├─ Canonical processing_state model     │
│  └─ Event coordination layer (pub-sub)   │
└──────────────────────────────────────────┘
```
