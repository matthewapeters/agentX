# Tool Usage Path - Critical Gaps with Code Examples

## CRITICAL GAP #1: Multi-Turn Tool Use Not Implemented

### The Problem
Tool execution completes but results are NEVER fed back to the LLM for continued reasoning. The system stops after tool execution.

### Current Broken Flow

**What Happens Now (INCOMPLETE):**
```
User: "Read file.py and refactor it"
    ↓
Classify: "simple_action" → single_tool route
    ↓
Bridge._stream_tool_response() [line 315 in bridge.py]
    ↓
    # TODO: Implement actual tool execution
    yield ResponseChunk(type=ChunkType.CONTENT,
        content="Tool execution coming soon...")
    yield from self._stream_direct_response()  # Falls back to no tools!
    ↓
Result: User gets generic response, NO tool was actually called
```

**Code Location:** `/Projects/agentX/src/agentix/bridge/bridge.py:295-322`

```python
def _stream_tool_response(
    self,
    prompt: str,
    context: Context,
    classification: PromptClassificationResponse,
) -> Iterator[ResponseChunk]:
    """
    Stream response involving a single tool call.

    Note: Full tool execution not yet implemented.
    Returns placeholder for now.

    Args:
        prompt: User prompt
        context: Conversation context
        classification: Classification result

    Yields:
        Tool call, tool result, and response chunks
    """
    # TODO: Implement actual tool execution  ← CRITICAL GAP
    yield ResponseChunk(
        type=ChunkType.CONTENT,
        content="Tool execution coming soon. Processing as direct response for now...",
    )

    # Fall back to direct response
    yield from self._stream_direct_response(prompt, context)
```

### What Should Happen

```
User: "Read file.py and refactor it"
    ↓
Classify: "simple_action" → single_tool route
    ↓
1. Select tool: read_file("file.py")
   ↓
   Yield TOOL_CALL chunk
   ↓
   Execute tool → "class Foo: def bar()..."
   ↓
   Yield TOOL_RESULT chunk
   ↓

2. Re-invoke LLM with:
   [previous messages] + 
   [tool_call: read_file] +
   [tool_result: "class Foo..."]
   ↓
   LLM now reasons about the code
   ↓
   LLM output: "Here's the refactored version..."
   ↓
   Yield CONTENT chunk
   ↓

3. If LLM needs more tools, loop back to step 1

Result: CORRECT - Tool was used, LLM reasoned about it
```

### The Missing Code

**File:** `/Projects/agentX/src/agentix/next_steps/single_tool.py`
**Current State:**
```python
def single_tool(
    args: AgentixConfig, next_step: NextStep, history: list[Message], max_tokens: int
):
    """Handle the single tool next step."""
    pass  # ← LITERALLY EMPTY
```

**What it should do:**
```python
def single_tool(
    args: AgentixConfig, 
    next_step: NextStep, 
    history: list[Message], 
    max_tokens: int
) -> Iterator[ResponseChunk]:
    """Handle single tool selection and execution."""
    
    # 1. Build first prompt to get tool call
    messages = [msg.to_llm_dict() for msg in history]
    messages.append({"role": "user", "content": prompt})
    
    # 2. Get tools in system prompt
    tools = get_available_tools()  # From registry
    
    # 3. Call LLM with tool calling enabled
    for chunk in query_api_streaming(args, {
        "model": args.model,
        "messages": messages,
        "tools": tools,  # ← KEY: Send tools to LLM
    }):
        # 4. Parse tool calls from response
        if tool_call_detected(chunk):
            tool_name, tool_args = parse_tool_call(chunk)
            yield ResponseChunk(
                type=ChunkType.TOOL_CALL,
                tool_name=tool_name,
                tool_input=tool_args,
            )
            
            # 5. Execute tool
            tool_result = execute_tool(tool_name, tool_args)
            yield ResponseChunk(
                type=ChunkType.TOOL_RESULT,
                tool_name=tool_name,
                tool_output=tool_result,
            )
            
            # 6. BUILD NEW REQUEST WITH RESULT
            messages.append({"role": "assistant", "content": chunk.content})
            messages.append({
                "role": "user",  # Tool result as user message
                "content": f"Tool result: {tool_result}"
            })
            
            # 7. Call LLM AGAIN to reason about result
            for follow_up_chunk in query_api_streaming(args, {
                "model": args.model,
                "messages": messages,
                "tools": tools,
            }):
                # 8. Either yield content or detect more tools
                yield follow_up_chunk
        else:
            # No tool call detected, just stream content
            yield ResponseChunk(type=ChunkType.CONTENT, content=chunk.content)
```

---

## CRITICAL GAP #2: Tool Results Not in LLM Context

### The Problem

Even if tools execute, their results are stored locally but NOT sent back to the LLM.

**File:** `src/agentx/session.py:355`

```python
def handle_tool_call(self, tool_name: str, tool_input: dict) -> None:
    """
    Handle a tool call from the LLM response.

    This method:
    1. Stores the TOOL_CALL message in context ✓
    2. Executes the tool ✓
    3. Stores the TOOL_RESULT message in context ✓
    4. Displays both in the GUI ✓
    5. ❌ DOES NOT RE-INVOKE LLM WITH RESULT
    
    Missing: After step 4, should loop back to LLM with result
    """
    try:
        # ... execute tool ...
        result = self.execute_tool(tool_name, tool_input)
        
        # ... store result ...
        tool_result_msg = Message(...)
        self.add_message_to_context(tool_result_msg)
        
        # ... display ...
        self.gui.display_agent_response(...)
        
        # ❌ MISSING: Re-invoke LLM
        # Should do:
        # new_prompt = None  # Tool result is implicit in context
        # self._stream_via_agentix(new_prompt)  # Continue with tool result in context
```

### Message Format for Tool Results

**File:** `src/shared/models/message.py:79-82`

```python
def to_llm_dict(self) -> dict:
    """
    Format message for LLM API (Ollama/OpenAI).
    """
    role_mapping = {
        MessageRole.THINKING: "assistant",
        MessageRole.TOOL_CALL: "assistant",
        MessageRole.TOOL_RESULT: "user",  # ← Tool results map to "user"
    }
```

**This is correct!** Tool results ARE formatted as "user" role for LLM API.

**But:** They're never sent in a NEW request after tool execution.

### The Fix

After tool execution in session.py (line 365), should be:

```python
# Store TOOL_RESULT message
tool_result_msg = Message(role=MessageRole.TOOL_RESULT, content=result)
self.add_message_to_context(tool_result_msg)

# Display tool result
self.gui.display_agent_response(...)

# ← ADD THIS: Continue LLM reasoning with tool result
self._continue_stream_with_context()  # New method needed
```

---

## CRITICAL GAP #3: Planning/Multi-Step Execution Disabled

### The Problem

Prompts classified as "complex_action" route to `invoke_planner`, but the planner is completely unimplemented.

**File:** `src/agentix/bridge/bridge.py:324-349`

```python
def _stream_planned_response(
    self,
    prompt: str,
    context: Context,
) -> Iterator[ResponseChunk]:
    """
    Stream response using multi-step planning.

    Note: Full planner not yet implemented.
    Returns placeholder for now.
    """
    # TODO: Implement actual planner  ← CRITICAL GAP
    yield ResponseChunk(
        type=ChunkType.THINKING,
        content="Multi-step planning coming soon. Processing as direct response for now...",
    )

    # Fall back to direct response
    yield from self._stream_direct_response(prompt, context)
```

### Example: What Should Happen

```
User: "Help me organize my project. List all Python files, analyze them, 
       and create a summary of what needs to be refactored."
    ↓
Classify: "complex_action" → invoke_planner route
    ↓
1. Generate Plan:
   LLM: "I need to:
        - Step 1: Find all Python files (search_files)
        - Step 2: Analyze each file (analyze_syntax)
        - Step 3: Compile results"
    ↓
   Yield THINKING chunk with plan
    ↓

2. Execute Plan:
   
   Step 1: search_files(pattern="*.py")
   → Results: ["src/main.py", "src/utils.py", ...]
   → Yield TOOL_CALL, TOOL_RESULT chunks
    ↓
   
   Step 2: For each file:
           analyze_syntax(file_content)
   → Results: {"functions": [...], "classes": [...]}
   → Yield TOOL_CALL, TOOL_RESULT chunks
    ↓

3. Compile Results:
   LLM: "Based on analysis, here are refactoring recommendations..."
   → Yield CONTENT chunk
```

### Missing Code

**File:** `src/agentix/next_steps/invoke_planner.py`

```python
from agentix import AgentixConfig
from agentix.context.message import Message
from agentix.prompt_classification_response import NextStep


def invoke_planner(
    args: AgentixConfig, next_step: NextStep, history: list[Message], max_tokens: int
):
    """Handle the invoke planner next step."""
    pass  # ← EMPTY - Not implemented
```

**What it should contain:**

```python
async def invoke_planner(...) -> Iterator[ResponseChunk]:
    """Generate and execute multi-step plan."""
    
    # 1. Generate plan from prompt
    plan = await generate_plan(prompt, context)
    yield ResponseChunk(
        type=ChunkType.THINKING,
        content=f"Plan: {plan.summary}"
    )
    
    # 2. Execute each step
    for step in plan.steps:
        # Execute tool
        result = await execute_tool(step.tool_name, step.parameters)
        yield ResponseChunk(
            type=ChunkType.TOOL_RESULT,
            tool_name=step.tool_name,
            tool_output=result,
        )
    
    # 3. Generate final response
    final_response = await generate_response(results)
    yield ResponseChunk(
        type=ChunkType.CONTENT,
        content=final_response
    )
```

---

## GAP #4: Tool Selection Not Implemented

### The Problem

Classification says "use a tool" but there's no logic to select WHICH tool and extract its parameters.

### Current Code

**File:** `src/agentix/bridge/bridge.py:295-322`

The `_stream_tool_response()` method should:
1. Send tool definitions to LLM
2. Let LLM choose a tool
3. Extract tool name and parameters
4. Execute it

But it doesn't - it just falls back to direct response.

### Tools Available

**Client Tools** (file operations):
```python
ClientToolExecutor.tools = {
    "read_file": {...},
    "write_file": {...},
    "list_directory": {...},
    "get_file_info": {...},
    "search_files": {...},
}
```

**Server Tools** (code analysis):
```python
ServerToolExecutor.CodeAnalysisTool.TOOLS = {
    "analyze_syntax": {...},
    "find_functions": {...},
    "find_classes": {...},
    "find_imports": {...},
    "suggest_refactoring": {...},
}
```

**Both available via:**
```python
AgentixBridge.get_available_tools()  # Returns OpenAI format
```

### How Tool Selection Should Work

```python
def _stream_tool_response(self, prompt, context, classification):
    """Select and execute a tool."""
    
    # 1. Get available tools
    available_tools = self.get_available_tools()
    
    # 2. Build system prompt asking LLM to pick a tool
    system_prompt = f"""
    User's request: {prompt}
    
    Available tools:
    {json.dumps(available_tools, indent=2)}
    
    Select the most appropriate tool and provide its parameters.
    """
    
    # 3. Call LLM to select tool
    response = query_api(config, {
        "model": config.model,
        "messages": [
            {"role": "system", "content": system_prompt},
            {"role": "user", "content": prompt}
        ],
        "tools": available_tools,  # ← Tool schema
    })
    
    # 4. Parse tool selection from response
    tool_call = parse_tool_call(response)
    if tool_call:
        yield ResponseChunk(
            type=ChunkType.TOOL_CALL,
            tool_name=tool_call["name"],
            tool_input=tool_call["parameters"],
        )
        
        # 5. Execute
        result = execute_tool(tool_call["name"], tool_call["parameters"])
        
        # 6. Yield result
        yield ResponseChunk(
            type=ChunkType.TOOL_RESULT,
            tool_output=result,
        )
        
        # 7. Continue with LLM reasoning about result
        yield from self._continue_with_tool_result(result)
```

---

## GAP #5: Tool Enablement Management

### The Problem

The UI has a tool panel where users can enable/disable tools, but the implementation is just stubs.

**File:** `src/shared/models/tools.py:352-368`

```python
class ToolRegistry:
    # --- Enabled tools state management ---
    def get_enabled_tools(self) -> list[str]:
        """Return a list of enabled tool names. Default: all tools enabled."""
        # In a real app, this could be loaded from config/session
        # For now, all registered tools are enabled by default
        return list(self._tools.keys())

    def set_enabled_tools(self, enabled_tools: list[str]) -> None:
        """Set which tools are enabled (stub for config/session integration)."""
        # In a real app, store this in config/session
        # Here, just a placeholder (no-op)
        pass  # ← STUB - does nothing

    def is_tool_enabled(self, name: str) -> bool:
        """Return True if the tool is enabled."""
        return name in self.get_enabled_tools()
```

### What Should Happen

```python
class ToolRegistry:
    def __init__(self):
        self._tools: dict[str, ITool] = {}
        self._enabled_tools: set[str] = set()  # ← Track enabled state
    
    def set_enabled_tools(self, enabled_tools: list[str]) -> None:
        """Update which tools are enabled."""
        self._enabled_tools = set(enabled_tools)
        # Also persist to config/session
        save_to_session({"enabled_tools": enabled_tools})
    
    def get_enabled_tools(self) -> list[str]:
        """Return only enabled tool names."""
        if not self._enabled_tools:
            # Load from session if not set
            self._enabled_tools = load_from_session().get("enabled_tools", set())
        return list(self._enabled_tools)
    
    def to_openai_format(self) -> list[dict]:
        """Get only ENABLED tools in OpenAI format."""
        return [
            tool.definition.to_openai_format() 
            for name, tool in self._tools.items()
            if self.is_tool_enabled(name)  # ← Filter by enabled
        ]
```

---

## GAP #6: Tool ID Tracking for Concurrency

### The Problem

ResponseChunk doesn't have a unique tool_id, so multiple concurrent tool calls can't be tracked.

**File:** `src/shared/models/response.py:80-86`

```python
@dataclass
class ResponseChunk:
    type: Optional[ChunkType] = None
    content: str = ""
    
    # Tool-specific fields
    tool_name: Optional[str] = None       # Not unique!
    tool_input: Optional[dict] = None
    tool_output: Optional[Any] = None
    tool_execution_context: Optional[str] = None
    tool_id: Optional[str] = None         # ← Exists but never set!
```

### Issue

If two tool calls happen:
```
Tool Call 1: read_file("a.py")
Tool Call 2: read_file("b.py")
↓
Tool Result 1: "contents of a.py"
Tool Result 2: "contents of b.py"
↓
Can't match which result belongs to which call!
```

### What Should Happen

```python
@dataclass
class ResponseChunk:
    ...
    tool_id: Optional[str] = None         # ← MUST be set for tool calls
    ...

# Usage:
yield ResponseChunk(
    type=ChunkType.TOOL_CALL,
    tool_id=uuid.uuid4(),  # ← Unique per call
    tool_name="read_file",
    tool_input={"path": "a.py"},
)

yield ResponseChunk(
    type=ChunkType.TOOL_RESULT,
    tool_id=uuid.uuid4(),  # ← Same ID as matching call
    tool_output="contents of a.py",
)

# Response handler can match them:
if chunk.type == ChunkType.TOOL_CALL:
    self.pending_tools[chunk.tool_id] = chunk
elif chunk.type == ChunkType.TOOL_RESULT:
    original_call = self.pending_tools[chunk.tool_id]
    print(f"Result for {original_call.tool_name}: {chunk.tool_output}")
```

---

## IMPLEMENTATION PRIORITY

### Priority 1 (BLOCKING everything else)
1. Implement `_stream_tool_response()` in bridge.py
2. Add loop to re-invoke LLM after tool execution
3. Feed tool results back into context for next request

### Priority 2 (Enables complex workflows)
4. Implement tool selection logic
5. Implement planning/multi-step execution

### Priority 3 (Quality of Life)
6. Implement tool enablement persistence
7. Add tool ID tracking for concurrency
8. Implement tool-specific error recovery

---

## CODE LOCATIONS SUMMARY

| Gap | File | Lines | Status |
|-----|------|-------|--------|
| Tool execution stub | bridge.py | 295-322 | TODO |
| Tool selection stub | bridge.py | 295-322 | TODO |
| Planning stub | bridge.py | 324-349 | TODO |
| Single tool route empty | single_tool.py | 10-14 | pass |
| Planning route empty | invoke_planner.py | entire file | pass |
| Multi-turn missing | session.py | 321-379 | Missing loop |
| Tool results not sent to LLM | session.py | 355-365 | Missing re-invoke |
| Enablement stub | tools.py | 360-364 | pass |
| BaseTool stub | tools.py | 326 | NotImplementedError |
| Tool ID not set | response.py | 85 | Not used |

