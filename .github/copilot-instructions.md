# AgentX – Copilot Instructions

## What This Project Is

AgentX is a local-first AI agent framework with a Tkinter GUI. It connects to **Ollama** (LLM inference) and optionally **Agentix** (code-analysis middleware). Users interact through the GUI; the app streams LLM responses, executes tools, and persists conversations to `sessions/` on disk.

## Commands

```bash
# Install (Python 3.12 only — enforced by pyproject.toml)
uv sync

# Run the app
python main.py
# or
python -m agentx

# Run health checks (verifies Ollama/Agentix reachability)
python agentx_diagnostics.py

# Tests
python -m pytest                              # all tests
python -m pytest tests/test_active_model.py  # single file
python -m pytest tests/test_active_model.py::TestActiveModelProperty::test_active_model_initialized_from_config -v  # single test
python -m pytest -m "not live"               # skip integration tests that need external services

# Lint / format
black src/ tests/ --line-length=120
isort src/ tests/ --profile=black --line-length=120
flake8 src/ tests/                            # config in .flake8
mypy src/
```

## Architecture

### Startup flow

```
main.py → src/agentx/main.py
  → AgentXSession.__init__()
      ├── ServiceManager   (Ollama + Agentix health checks / subprocess launch)
      ├── GUIManager       (Tkinter widgets, implements IGUIManager protocol)
      ├── FileExplorer     (file navigation panel)
      ├── AgentixBridgeAdapter  (wraps async Agentix with sync/generator API)
      └── ClientToolExecutor / ServerToolExecutor
  → session.perform_service_handshake()
  → session.layout()          (builds all Tkinter widgets)
  → root.mainloop()
```

### Key module map

| Module | Role |
|--------|------|
| `src/agentx/session.py` | Central orchestrator — wires everything together, drives streaming |
| `src/agentx/service_manager.py` | Manages external service lifecycle (Ollama, Agentix) |
| `src/agentx/gui/gui_manager.py` | Implements `IGUIManager`; all Tkinter widget logic lives here |
| `src/agentx/igui_manager.py` | `Protocol` defining the GUI boundary — business logic talks only to this |
| `src/agentx/file_explorer.py` | File navigation widget (list, open, history traversal) |
| `src/agentx/history.py` | Loads prior sessions from disk |
| `src/agentx/widget_registry.py` | Centralised widget lifecycle and cleanup |
| `src/agentx/integration/agentix_bridge_adapter.py` | Converts async Agentix calls → sync / streaming generators for Tkinter thread model |
| `src/agentx/integration/streaming_executor.py` | Background-thread streaming with progress tracking |
| `src/agentx/integration/code_analysis.py` | CST/AST-based code analysis tools |
| `src/shared/models/context.py` | Conversation history — single source of truth, synced to disk |
| `src/shared/models/message.py` | `Message` dataclass with `MessageRole` enum |
| `src/shared/models/response.py` | `ResponseChunk` enum (CONTENT, THINKING, TOOL_CALL, TOOL_RESULT, …) |
| `src/shared/models/tools.py` | Tool definitions and schemas |
| `src/agentix/bridge/bridge.py` | `AgentixBridge` — tool loop, streaming, tool execution |
| `src/agentix/tools/schema.py` | `extract_tool_schema(fn)` — Python function → OpenAI JSON schema |
| `src/agentx/integration/client_tool_executor.py` | File system tools (read/write/search) wired into bridge |
| `system_prompts/` | Markdown prompt files loaded at runtime (planner, python_coder, tool_use, classification) |

### Tool pipeline

The agentic tool loop lives in `src/agentix/bridge/bridge.py`:

```
AgentixBridge.process_prompt_streaming()
  ├── RESPOND_DIRECTLY → _stream_direct_response()
  ├── SINGLE_TOOL      → _stream_tool_response()   ──┐
  └── INVOKE_PLANNER   → _stream_planned_response() ──┤
                                                       │
          _run_tool_loop(max_rounds=N) ────────────────┘
            ├── _iter_llm_chunks()   — accumulates OpenAI streaming deltas
            ├── execute_tool()       — dispatches by name (CST, AST, file tools)
            └── ThreadPoolExecutor  — runs multiple tool calls in parallel
```

- **Tool wire format**: `TOOL_CALL` chunks → Ollama `assistant` + `tool_calls[]`; `TOOL_RESULT` chunks → `tool` role with `tool_call_id`
- **Registering new tools**: call `bridge.register_tool_implementations(impls, schemas)` — `AgentixBridgeAdapter._register_client_tools()` is the reference implementation
- **Schema generation**: `extract_tool_schema(fn)` in `src/agentix/tools/schema.py` converts any Python function with a docstring into a valid OpenAI schema
- **Context persistence**: `session._display_tool_call()` / `_display_tool_result()` store tool interactions in `Context` using `add_tool_call_message()` / `add_tool_result_message()` — do not call `handle_tool_call()` directly (double-execution bug, now fixed)

### Threading model

Tkinter must run on the main thread. LLM streaming runs on background threads. `AgentixBridgeAdapter` wraps async Agentix methods with sync calls and returns generators so `streaming_executor.py` can iterate them safely. Use `threading.Event` for coordination (see `_is_streaming` in `session.py`).

### Persistence

Conversations are stored under `sessions/<session_id>/context/` as JSON. `Context` (in `shared/models/context.py`) is the authoritative in-memory model and handles load/save. Do not write session state anywhere except through `Context`.

## Conventions

### Style
- **Line length**: 120 characters (black + isort + flake8 all agree)
- **Type hints**: required everywhere; use Python 3.12+ syntax (`list[str]`, `dict[str, Any]`)
- **Dataclasses** for data models; **Enums** for typed constants; **Protocols** for interfaces

### Naming
- Classes: `PascalCase`; interface classes prefixed with `I` (e.g. `IGUIManager`)
- Methods/attributes: `snake_case`; private members: single leading underscore (`_active_model`)
- Constants: `UPPER_SNAKE_CASE`

### Patterns
- **Adapter** — `AgentixBridgeAdapter` is the canonical example; use adapters when bridging async↔sync or external API boundaries
- **Protocol** — define boundaries with `Protocol` classes before implementing; `session.py` depends on `IGUIManager`, never on `GUIManager` directly
- **Registry** — `WidgetRegistry` owns widget lifecycle; don't create/destroy widgets outside it
- **Markers** — tag tests that need live external services with `@pytest.mark.live`

### Configuration
Runtime config lives in `agentx.toml` (loaded by `src/agentx/config.py`). Never hard-code hostnames, model names, or timeouts — read from `AgentXConfig`. The GUI config subset lives in `gui/gui_config.py`.

## Plan Documents

Multi-session work is tracked in Markdown plan files (e.g. `docs/tool_usage_plan.md`). Each
step is a numbered checkbox. When working on a plan:

- **At session start:** read the plan file to find the next `[ ]` step
- **While working:** mark the step `[/]` (in progress) before starting it
- **On success:** replace `[/]` with `[ ]` and add a ✓ comment inline, **or** leave `[/]`
  permanently to indicate "done" — follow whichever convention the plan file uses
- **On failure:** mark the step `[X]` and append a brief inline note explaining why, so it
  can be revisited or corrected in a future session; report the failure explicitly in your
  response so the user is aware

Step status legend (applies to all plan files in this repo):

| Marker | Meaning |
|--------|---------|
| `[ ]` | Not yet started |
| `[/]` | Complete |
| `[X]` | Failed or blocked — needs follow-up |

Never silently skip a failed step. Always surface `[X]` items before closing out a session.

## Additional Resources

- `docs/architecture.md` — module index with retrieval keywords, designed for AI-assisted reasoning
- `docs/tool_usage_plan.md` — phased implementation plan for the agentix tool usage path
- `smolagentx.md` — SmolAgents integration PoC design and suggested agent prompts
- `agentx_diagnostics.py` — run this first when external service behaviour is unexpected
