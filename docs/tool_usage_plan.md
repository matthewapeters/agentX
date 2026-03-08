# Agentix Tool Usage Path — Implementation Plan

## Overview

This document tracks the phased implementation of a complete, agentic tool-usage pipeline for
the agentix middleware layer. The design is inspired by the MIT-licensed
[lmstudio-python](https://github.com/lmstudio-ai/lmstudio-python) SDK, which provides a clean
reference for: function-to-schema conversion, mid-stream tool-call detection, parallel tool
execution, and multi-round LLM feedback loops.

Steps use the following status markers:
- `[ ]` — Not yet started
- `[/]` — In progress
- `[X]` — Failed / blocked (see note)

Assume each **Phase** may span one or more agent sessions. Read this file at the start of each
session to orient yourself before implementing.

---

## Reference: Key Findings

### Current agentix gaps (as of 2026-03-07)

| Component | File | Status | Problem |
|-----------|------|--------|---------|
| Single-tool handler | `src/agentix/next_steps/single_tool.py` | ❌ Empty stub | Function body is `pass` |
| Planner handler | `src/agentix/next_steps/invoke_planner.py` | ❌ Incomplete | Cuts off with no return |
| Escalate handler | `src/agentix/next_steps/escalate.py` | ❌ Empty stub | |
| Respond-directly handler | `src/agentix/next_steps/respond_directly.py` | ❌ Empty stub | |
| Tool streaming route | `src/agentix/bridge/bridge.py` `_stream_tool_response()` | ❌ Stub | Returns placeholder, falls through to direct response |
| Planning streaming route | `src/agentix/bridge/bridge.py` `_stream_planned_response()` | ❌ Stub | Returns placeholder, falls through |
| Tool execution method | `src/agentix/bridge/bridge.py` | ❌ Missing | `execute_tool()` does not exist; `ServerToolExecutor` calls it |
| Tool enablement | `src/shared/models/tools.py` `set_enabled_tools()` | ❌ No-op | Stub with `pass`, no persistence |
| Multi-turn feedback loop | `src/agentx/session.py` `handle_tool_call()` | ❌ Missing | Results never re-submitted to LLM |
| Tool ID tracking | `src/shared/models/tools.py` | ⚠️ Unused | IDs assigned but never correlated |

### Key patterns to borrow from lmstudio-python (MIT)

- **Function → JSON schema**: `inspect.signature()` + `get_type_hints()` + docstring → OpenAI tool schema
  (`json_api.py:1144-1252`)
- **Mid-stream tool call detection**: Tool call events arrive as discrete WebSocket messages during
  token generation; detect them as they arrive, not after completion (`json_api.py:1386-1430`)
- **Parallel tool execution**: Submit each tool call to a `ThreadPoolExecutor` immediately on
  detection; collect results with `as_completed()` after streaming ends (`sync_api.py:1326-1510`)
- **Error-as-result**: Tool execution failures are serialized to JSON and sent to the LLM as
  `tool_result` messages — the loop never crashes
- **Multi-round loop**: After collecting results, append an `assistant` message (with tool calls)
  and a `tool` message (with results) to chat history, then resubmit; loop ends when no tool
  calls are detected or `max_rounds` is reached
- **Message roles**: `assistant` (contains `tool_call` items) → `tool` (contains `tool_result`
  items keyed by `tool_call_id`) — matches OpenAI function-calling wire format

---

## Phase 1 — Tool Schema & Registration

**Goal:** Every tool registered in `ToolRegistry` can produce a valid OpenAI-compatible JSON
schema from a plain Python function signature. No external dependencies beyond stdlib and
`inspect`.

### Step 1.1 — Implement function-to-schema conversion
- [/] 1.1.1  Create `src/agentix/tools/schema.py` with:
  - `extract_tool_schema(fn: Callable) -> dict` — uses `inspect.signature()`,
    `typing.get_type_hints()`, and `fn.__doc__` to produce an OpenAI `function` schema object
  - Map Python primitives: `str → "string"`, `int → "integer"`, `float → "number"`,
    `bool → "boolean"`, `list → "array"`, `dict → "object"`
  - Support `Optional[T]` (not required) and default values (omit from `required`)
  - Raise `SchemaGenerationError` (define in same file) if docstring is missing
- [/] 1.1.2  Write unit tests in `tests/test_tool_schema.py` covering:
  - Simple function with positional args
  - Optional parameters and defaults
  - Missing docstring raises error
  - Complex types (`list[str]`, `dict[str, int]`) degrade gracefully

### Step 1.2 — Wire schema generation into ToolDefinition
- [/] 1.2.1  In `src/shared/models/tools.py`, add `ToolDefinition.from_callable(fn)` class method
  that calls `extract_tool_schema()` and populates `name`, `description`, `parameters`
- [/] 1.2.2  In `ToolRegistry.register()`, accept a raw `Callable` as well as `ToolDefinition`;
  auto-wrap via `from_callable()` using the new `_CallableTool` wrapper class
- [/] 1.2.3  Add `ToolRegistry.to_llm_tools() -> list[dict]` returning only enabled tools in
  OpenAI format (respects the enabled-tools filter)

### Step 1.3 — Implement tool enablement
- [/] 1.3.1  Replace the `set_enabled_tools()` no-op in `tools.py` with a proper implementation
  that stores enabled names in an `_enabled: set[str] | None` instance field; `None` means all
  enabled. Added `enable_all_tools()` to clear restrictions. All three methods consistent.
- [/] 1.3.2  Persist per-session enabled-tool overrides via bridge. The GUI checkbox toggle
  in `session.py` now calls `agentix_adapter.set_enabled_tools(enabled_tools)` which
  delegates to `bridge.set_enabled_tools()`. The bridge filters `_extra_tool_schemas` by
  name so `get_available_tools()` (called inside `_run_tool_loop`) only returns tools the
  user has enabled. `self.config["agentix"]["available_tools"]` is also kept in sync.

---

## Phase 2 — Tool Call Detection in Streaming

**Goal:** When Ollama (or any backend) returns a streaming response containing a tool call,
the system detects it immediately — not only after the stream is complete.

> **Context:** Ollama's streaming format can include `tool_calls` in a `message` chunk
> before the final `done: true` chunk. The current `ResponseHandler` has chunk types for
> `TOOL_CALL` and `TOOL_RESULT` but the bridge never emits them from real streams.

### Step 2.1 — Map Ollama's streaming tool-call format
- [/] 2.1.1  Read Ollama's `/api/chat` streaming spec (or test against a live model with
  `tool_calls` in the schema) and document the exact JSON shape of a tool-call chunk
  — Bridge uses the OpenAI-compat `/v1/chat/completions` endpoint. Tool calls arrive as
  `choices[0].delta.tool_calls` deltas with incremental `arguments` strings; completed by
  `finish_reason == "tool_calls"`. Documented in `_iter_llm_chunks()` docstring with examples.
- [/] 2.1.2  Create `tests/integration/test_ollama_tool_stream.py` with a recorded fixture
  of an Ollama streaming response containing a tool call (use `unittest.mock` if no live
  service is available); mark with `@pytest.mark.live` for runs requiring a real Ollama

### Step 2.2 — Emit TOOL_CALL chunks from the bridge
- [/] 2.2.1  In `src/agentix/bridge/bridge.py`, added `_iter_llm_chunks(messages, tools)` —
  the shared low-level streaming method that accumulates tool call argument fragments and
  emits `ResponseChunk(type=ChunkType.TOOL_CALL, ...)` when `finish_reason == "tool_calls"`
- [/] 2.2.2  `tool_call_id` from the Ollama/OpenAI response is preserved in `chunk.tool_id`
- [/] 2.2.3  `_iter_llm_chunks` handles all chunk types; `_stream_direct_response` delegates
  to it. `ResponseHandler` downstream already accumulated tool calls via `tool_name`/`tool_input`.

---

## Phase 3 — Single Tool Execution Path (End-to-End)

**Goal:** When the classifier routes a prompt to `NextStep.SINGLE_TOOL`, the correct tool is
selected, executed, and its result is returned as a `TOOL_RESULT` chunk.

### Step 3.1 — Add `execute_tool()` to AgentixBridge
- [/] 3.1.1  In `src/agentix/bridge/bridge.py`, implemented:
  ```python
  def execute_tool(self, tool_name: str, arguments: dict, tool_id=None) -> ToolResponse
  ```
  Looks up tool in `_get_tool_implementations()` (auto-discovers functions from cst_tools/ast_tools
  modules), calls it, returns `ToolResponse`. Also added `_get_tool_implementations()` with caching.
- [/] 3.1.2  Exceptions caught; returns `ToolResponse.error_response(str(exc))` — error-as-result pattern
- [/] 3.1.3  Tests in `tests/integration/test_ollama_tool_stream.py` (`TestExecuteTool` class)

### Step 3.2 — `single_tool.py` next-step handler
- ~~3.2.1~~ *(Removed — dead code path.)* `src/agentix/next_steps/single_tool.py` is part of
  the pre-bridge `agent.py → take_steps()` CLI path, which is never invoked by the AgentX GUI.
  The bridge handles `SINGLE_TOOL` end-to-end via `_stream_tool_response → _run_tool_loop`.
- ~~3.2.2~~ *(Removed — covered by bridge tests.)*

### Step 3.3 — Wire `_stream_tool_response()` in bridge.py
- [/] 3.3.1  `_stream_tool_response()` now delegates to `_run_tool_loop(max_rounds=1)` —
  detects tool calls, executes in parallel, feeds results back, gets final answer.
- [/] 3.3.2  Full path verified: `SINGLE_TOOL` → `_stream_tool_response` → `_run_tool_loop`
  → `_iter_llm_chunks` detects TOOL_CALL → `execute_tool` → TOOL_RESULT chunk → round 2
  → final CONTENT chunks. Covered by `TestStreamToolResponse` in the test file.

---

## Phase 4 — Multi-Turn Tool Feedback Loop

**Goal:** After a tool executes, its result is appended to the chat history and resubmitted to
the LLM, which can then reason over the result and either call another tool or produce a final
answer. This is the core "agentic loop".

> **Design reference:** lmstudio-python `sync_api.py:1326-1510` — the loop continues until
> no tool calls are present in the prediction OR `max_rounds` is hit.

### Step 4.1 — Extend chat history model for tool messages
- [/] 4.1.1  In `src/shared/models/message.py`, `to_llm_dict()` now emits correct Ollama
  wire format for tool roles:
  - `TOOL_CALL` → `{"role": "assistant", "content": "", "tool_calls": [{"id": ..., "type": "function", "function": {"name": ..., "arguments": "{...}"}}]}`
  - `TOOL_RESULT` → `{"role": "tool", "content": "...", "tool_call_id": "..."}`
  - `THINKING` → maps to `"assistant"` (unchanged)
- [/] 4.1.2  In `src/shared/models/context.py`, added `Context.add_tool_call_message()` and
  `Context.add_tool_result_message()` helpers that append correctly-typed `Message` objects
- [/] 4.1.3  `Context.to_llm_messages()` delegates to `Message.to_llm_dict()` which now
  emits correct wire format for all roles. Covered by 24 tests in
  `tests/test_message_wire_format.py`.

### Step 4.2 — Implement the agentic loop in the bridge
- [/] 4.2.1  `_run_tool_loop()` generator implemented in `src/agentix/bridge/bridge.py`.
  Iterates `max_rounds + 1` times; final round strips tools to force a direct answer. Each
  round streams via `_iter_llm_chunks`, collects TOOL_CALL chunks, executes tools in parallel,
  appends assistant + tool messages to local history, loops.
- [/] 4.2.2  `max_rounds` read from `getattr(config, "max_tool_rounds", 10)` in
  `_stream_planned_response`. Add `max_tool_rounds` to `agentx.toml` under `[agentix]` when
  wiring up config propagation (ties to step 4.4).
- [/] 4.2.3  `ResponseChunk(type=ChunkType.DONE, done_reason="stop")` yielded at loop end.

### Step 4.3 — Parallel tool execution in the loop
- [/] 4.3.1  `ThreadPoolExecutor(max_workers=min(N, 4))` with `as_completed()` — parallel
  execution when multiple tool calls arrive in a single round.
- [/] 4.3.2  Each result carries its `tool_call_id` via `ToolResponse.request_id` and the
  `tool` message's `tool_call_id` field.

### Step 4.4 — Update session.py to use the loop
- [/] 4.4.1  Fixed double-execution bug: `session.py` `on_tool_call` callback no longer
  calls the old `handle_tool_call()` (which re-executed the tool). Bridge's `_run_tool_loop`
  handles execution; the new `_display_tool_call(name, args)` method only displays + stores a
  TOOL_CALL message in context.
- [/] 4.4.2  New `_display_tool_result(tool_id, output)` method displays result in GUI and
  calls `context.add_tool_result_message()` so tool interactions persist across sessions.
  `on_tool_result` in `ResponseHandler` now correctly passes `chunk.tool_output` (not
  empty `chunk.content`).
- [/] 4.4.3  GUI receives THINKING / TOOL_CALL / TOOL_RESULT / CONTENT chunks in correct
  order as they arrive from `_run_tool_loop`. No display-layer changes needed; chunk routing
  via `ResponseHandler.process_chunk()` is already correct.

### Step 4.5 — Tests for the multi-turn loop
- [/] 4.5.1  `TestStreamToolResponse.test_stream_tool_response_executes_tool_and_continues`
  in `tests/integration/test_ollama_tool_stream.py` covers: single tool call → result →
  final answer. Additional scenarios (multi-round, max_rounds termination, error-as-result)
  are covered by the surrounding test classes in the same file.

---

## Phase 5 — Planning Route (Multi-Step Tool Chains)

**Goal:** When the classifier routes to `NextStep.INVOKE_PLANNER`, a plan is created (list of
`PlanStep` objects from `plan_steps.py`) and executed step by step, with results from each step
available to subsequent steps.

> **Status (2026-03-08):** Phase 5.4.1 is effectively done — `_stream_planned_response()`
> already delegates to `_run_tool_loop(max_rounds=config.max_tool_rounds)`, which lets the LLM
> decide which tools to call across up to N rounds before producing a final answer.  The old
> `invoke_planner.py` / `take_steps.py` / `escalate.py` handlers used an incompatible API
> (non-streaming, no bridge integration) and are superseded by this approach.  Steps 5.1–5.3
> remain as optional enhancements for structured plan inspection/reporting.

### Step 5.1 — Complete the planner handler
- ~~5.1.1~~ *(Removed — dead code path.)* `invoke_planner.py` is part of the pre-bridge
  `agent.py → take_steps()` CLI path, never called by the GUI bridge.
- ~~5.1.2~~ *(Removed.)*

### Step 5.2 — Implement step execution in `take_steps.py`
- ~~5.2.1~~ *(Removed — dead code path.)* `take_steps.py` is the old CLI router; the bridge
  handles all routing internally.
- ~~5.2.2~~ *(Removed.)*

### Step 5.3 — Implement `escalate.py`
- ~~5.3.1~~ *(Removed — dead code path.)* `escalate.py` is part of the same superseded CLI
  path. The bridge's `NextStep.escalate` branch falls through to `_stream_direct_response`
  which yields a graceful response.

### Step 5.4 — Wire `_stream_planned_response()` in bridge.py
- [/] 5.4.1  `_stream_planned_response()` delegates to `_run_tool_loop(max_rounds=config.max_tool_rounds)`.
  The LLM self-directs tool selection across multiple rounds (tool-use planning pattern).
  Formal `PlanStep`-based execution (steps 5.1-5.3) is an optional future enhancement.

---

## Phase 6 — Client-Side Tool Integration

**Goal:** The existing `ClientToolExecutor` (file read/write/search) is fully wired into the
tool loop so the LLM can invoke client-side tools the same way as server-side ones.

### Step 6.1 — Register client tools in ToolRegistry
- [/] 6.1.1  In `src/agentx/integration/client_tool_executor.py`, added five standalone
  wrapper functions (`read_file`, `write_file`, `list_directory`, `get_file_info`,
  `search_files`) with full type signatures and docstrings. Added
  `get_client_tool_implementations()` (name→callable dict) and
  `get_client_tool_schemas()` (auto-generates OpenAI schemas via `extract_tool_schema()`).
- [/] 6.1.2  `AgentixBridgeAdapter._register_client_tools()` is called from `__init__()`.
  It calls `bridge.register_tool_implementations(impls, schemas)` so both execution and LLM
  visibility are set up at startup.

### Step 6.2 — Route client vs. server tool execution
- [/] 6.2.1  `AgentixBridge.register_tool_implementations()` was added. It merges impls into
  `_tool_impl_cache` and schemas into `_extra_tool_schemas`. `get_available_tools()` now
  includes `_extra_tool_schemas`. `execute_tool()` dispatches to registered callables
  (client and server tools share the same lookup — no separate dispatch needed since they
  run in the same process).
- [/] 6.2.2  All client tools return strings; `execute_tool()` wraps in `ToolResponse`
  uniformly. Covered by 21 tests in `tests/test_client_tool_integration.py`.

---

## Phase 7 — Polish, Tests & Observability

**Goal:** The completed pipeline is robust, well-tested, and produces enough signal in the GUI
and logs for debugging.

### Step 7.1 — End-to-end integration tests
- [/] 7.1.1  Added `tests/integration/test_tool_end_to_end.py` — live tests marked `@pytest.mark.live`
  that test against a real Ollama instance; includes 6 live tests for tool call/result chunks,
  round_index propagation, and DONE chunk at end. Run with `pytest -m live --model llama3.1`.
- [/] 7.1.2  Non-live (CI) tests in same file — 7 tests using mocked `_iter_llm_chunks` to verify
  round_index on ResponseChunk fields and round_index propagation through `_run_tool_loop`.

### Step 7.2 — GUI observability
- [/] 7.2.1  Added `round_index: Optional[int]` field to `ResponseChunk`. Bridge's `_run_tool_loop`
  now sets `round_index` on every TOOL_CALL and TOOL_RESULT chunk. `ResponseHandler` passes it
  through `on_tool_call(name, args, round_i)` and `on_tool_result(id, output, round_i)`.
  Session's `_display_tool_call`/`_display_tool_result` show `[round N]` in the chat output.
- [/] 7.2.2  Cancel mechanism already works via Python generator protocol: breaking the
  `for chunk in process_prompt_generator(...)` loop (which `interrupt_streaming()` causes via
  `_is_streaming.clear()`) triggers `GeneratorExit` on `_run_tool_loop` at its next yield point.
  No additional cancel button code needed — the existing interrupt button already cancels tool loops.

### Step 7.3 — System prompt review
- [/] 7.3.1  `system_prompts/python_coder.md` does not reference tools (it's a code
  generation prompt, not an agent prompt). `system_prompts/planner_prompt.md` already
  instructs the LLM not to hallucinate tools and to use exact tool schema arguments.
  No changes needed for existing prompts.
- [/] 7.3.2  Created `system_prompts/tool_use.md` — explains available tool categories,
  call rules (always read before write, use tools for file inspection, chain naturally),
  and error handling. Prepend this when `tools` are enabled in `AgentixConfig.system`.

### Step 7.4 — Documentation update
- [/] 7.4.1  Updated `docs/architecture.md` with "Tool Pipeline" section: ASCII flow
  diagram, key file table, wire format examples, and list of new test files.
- [/] 7.4.2  Updated `.github/copilot-instructions.md`: added "Tool pipeline" subsection
  under Architecture (wire format, registration pattern, schema generation, context
  persistence), updated module map with `bridge.py`, `schema.py`, `client_tool_executor.py`.

---

## Suggested Session Starting Checklist

At the start of each new agent session working on this plan:

1. Read this file (`docs/tool_usage_plan.md`) to see which steps are `[ ]`
2. Read `docs/architecture.md` for the module map
3. Check `src/agentix/bridge/bridge.py` and `src/agentix/next_steps/` for current state
4. Run `python -m pytest -m "not live"` to confirm baseline test status
5. Pick the next uncompleted step, mark it `[/]`, implement, then mark `[/]` → `[ ]` (done
   means updating to a different indicator is intentional — re-read the marking legend above)

> **Marking reminder:** `[ ]` = not started, `[/]` = in progress, `[X]` = failed/blocked

---

## Dependency Graph (Phase Order)

```
Phase 1 (Schema & Registration)
    └── Phase 2 (Stream Detection)
            └── Phase 3 (Single Tool Execution)
                    └── Phase 4 (Multi-Turn Loop)  ← core milestone
                            ├── Phase 5 (Planning Route)
                            └── Phase 6 (Client Tools)
                                    └── Phase 7 (Polish & Tests)
```

Phases 5 and 6 can proceed in parallel once Phase 4 is complete.
