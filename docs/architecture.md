
# AgentX — Architecture Reference

_Last updated: 2026-05-24 (v1.0.0)_
Version: 2026-05-24  
Branch: main  
Project version: 1.0.0

---

## Tab/Surface Parity Matrix (Architecture Anchor)

See [docs/hybrid_remaining_work.md](hybrid_remaining_work.md) and [docs/ux/UX_LIFECYCLE.md](ux/UX_LIFECYCLE.md) for the remaining-work synthesis and traceability matrix.

## Channel Registry (Authoritative)

See [docs/architecture/channel_registry.md](architecture/channel_registry.md) for the authoritative channel registry: channel names, schemas, pub/sub wiring, and policy. All changes to event channels must be reflected there.

## Runtime Split (Go Core and Go Applets)

See [docs/architecture/runtime_split.md](architecture/runtime_split.md) for the authoritative runtime split and migration plan: Go core and Go applets, IPC contract, migration phases, and thin-GUI contract. All parity and migration claims must reference this doc.

## Applet Contracts (Authoritative)

See [docs/architecture/applets/](architecture/applets/README.md) for the authoritative per-applet UX/architecture contracts.
The **Output applet** interactive design (navigation model, ownership boundary, phased implementation plan) is the canonical source of truth at [docs/architecture/applets/output_applet.md](architecture/applets/output_applet.md).

## Startup Modes (Authoritative)

See [docs/architecture/startup_modes.md](architecture/startup_modes.md) for the authoritative startup-mode catalog, startup switches, and launch patterns.

## Architecture Decisions (ADRs)

See [docs/architecture/adr/00_INDEX.md](architecture/adr/00_INDEX.md) for the orchestration architecture decision record (ADR) entrypoint and index of accepted decisions.

## Orchestration Design and Behavior Contracts

See [docs/architecture/design/00_INDEX.md](architecture/design/00_INDEX.md) for implementation-level object/interface design contracts derived from ADR 0001-0005.

See [docs/architecture/behavior/00_INDEX.md](architecture/behavior/00_INDEX.md) for detailed Gherkin behavior use-cases and traceability mapping.

See [docs/architecture/schemas/00_INDEX.md](architecture/schemas/00_INDEX.md) for the authoritative orchestration schema home.

| GUI Tab / Surface | TUI Analog / Surface | Owner (Go/Py) | Status |
| --- | --- | --- | --- |
| Chat (Output) | Output Pane (chat) | Go (current) | ✅ Implemented |
| Tool Processing (Output) | Output Pane (tool) | Go (current) | ✅ Implemented |
| File Edit (Output) | Output Pane (file edit) | Go (current) | ✅ Implemented |
| Logs Lifecycle | Logs Pane | Go (current) | ✅ Implemented |
| Context Visualization | System Pane (context viz) | Go (current) | ✅ Implemented |
| Files Navigation | System Pane (files) | Go (current) | ✅ Implemented |
| Context History/Current | System Pane (context hist) | Go (current) | ✅ Implemented |
| Configuration | System Pane (config) | Go (current) | ✅ Implemented |

**Blockers:** Parity is still governed by blocker gates. Current risk is now concentrated on end-to-end traceability/sign-off discipline and continued gate stability, not Python-owned pane runtime gaps.

**Next Steps:**

- Complete Go-owned runtime/apply flow implementations for TUI parity
- Keep GUI on secondary track until TUI parity completion gates are met
- Align UX lifecycle and architecture claims with implementation reality

---

## Contents

1. [System Overview](#1-system-overview)
2. [Startup Flow](#2-startup-flow)
3. [Module Map](#3-module-map)
4. [Class Relationships](#4-class-relationships)
5. [Session Decomposition](#5-session-decomposition)
6. [GUI Decomposition](#6-gui-decomposition)
7. [Classification Pipeline](#7-classification-pipeline)
8. [Tool Pipeline](#8-tool-pipeline)
9. [Hierarchical Task Execution](#9-hierarchical-task-execution)
10. [Working Memory](#10-working-memory)
11. [Data Models](#11-data-models)
12. [Context Construction Pipeline](#12-context-construction-pipeline)
13. [Tool Schemas and Usage](#13-tool-schemas-and-usage)
14. [Threading Model](#14-threading-model)
15. [Persistence Layout](#15-persistence-layout)
16. [Configuration Reference](#16-configuration-reference)

17. [Runtime Split (Go Core and Go Applets)](architecture/runtime_split.md)
18. [Startup Modes (Authoritative)](architecture/startup_modes.md)

---

## 1. System Overview

AgentX is a local-first AI agent framework with a Tkinter GUI.  It connects to
**Ollama** (LLM inference) and optionally **Agentix** (code-analysis
middleware).  Users interact through the GUI; the app streams LLM responses,
executes tools, and persists conversations to `sessions/` on disk.

```
┌──────────────────────────────────────────────────────────────┐
│  Tkinter GUI  (main thread)                                  │
│  ┌─────────────┐  ┌──────────────────────────────────────┐  │
│  │  SidePanel  │  │  ChatPanel  (output notebook)        │  │
│  │  - Model    │  │  - Chat tab  (streaming messages)     │  │
│  │  - Session  │  │  - Plan tabs (PlanTreeWidget × N)     │  │
│  │  - Files    │  └──────────────────────────────────────┘  │
│  │  - Settings │  ┌──────────────────────────────────────┐  │
│  └─────────────┘  │  InputPanel (text + buttons)         │  │
└──────────────────────────────────────────────────────────────┘
          │                          │
          │ IGUIManager interface    │
          ▼                          ▼
┌────────────────────────────────────────────────────────────────┐
│  AgentXSession  (main thread coordinator)                      │
│  ├─ SessionState        mutable session data                   │
│  ├─ StreamingController LLM stream + display logic             │
│  ├─ ToolDispatcher      routes tool calls                      │
│  ├─ Context             conversation history (disk-backed)     │
│  ├─ WorkingMemory       per-session key-value fact store       │
│  └─ AgentixBridgeAdapter  ──────────────────────────────────┐  │
└────────────────────────────────────────────────────────────┬┘  │
                                                             │    │
                     background thread ───────────────────────   │
                          │                                       │
          ┌───────────────▼──────────────┐                       │
          │  AgentixBridge               │                       │
          │  ├─ classify_prompt()        │                       │
          │  ├─ _stream_direct_response()│                       │
          │  ├─ _stream_tool_response()  │                       │
          │  ├─ _stream_planned_response │                       │
          │  └─ ToolLoopRunner           │                       │
          └──────────┬───────────────────┘                       │
                     │                                           │
          ┌──────────▼───────────┐   ┌────────────────────────┐ │
          │  Ollama              │   │  Agentix (optional)    │ │
          │  /v1/chat/completions│   │  code-analysis tools   │ │
          └──────────────────────┘   └────────────────────────┘ │
```

---

## 2. Startup Flow

```
main.py
  └── src/agentx/main.py → main()
        ├── load agentx.toml  (config.py)
        ├── AgentXSession(root, config)
        │     ├── SessionState(...)             mutable session data
        │     ├── Context(path, session_id)     load/create on-disk context
        │     ├── FileExplorer(start_path)
        │     ├── ServiceManager(config)        Ollama + Agentix health checks
        │     ├── GUIManager(root, config, …)   build panel objects (no widgets yet)
        │     ├── AgentixBridgeAdapter(config)  async→sync bridge, registers client tools
        │     ├── ClientToolExecutor            file-system tools
        │     ├── ServerToolExecutor            Agentix server tools
        │     ├── ToolDispatcher               routes tool calls
        │     ├── WorkingMemory.load()          load or create fact store
        │     └── StreamingController(self)     owns all streaming logic
        ├── session.perform_service_handshake()
        │     ├── ServiceManager.start_services()
        │     └── ServiceManager.wait_for_services()
        ├── session.layout()
        │     └── gui.create_layout()
        │           ├── _setup_fonts()
        │           ├── _setup_window_geometry()
        │           ├── ChatPanel.create()
        │           ├── SidePanel.create()
        │           └── InputPanel.create()
        └── root.mainloop()
```

---

## 3. Module Map

### src/agentx/ — Application layer

| Module | Path | Role |
|--------|------|------|
| `AgentXSession` | `session.py` | Central coordinator — wires all subsystems |
| `shared.providers` | `../src/shared/providers/` | Shared LLM provider contracts and Ollama implementation consumed by both AgentX and Agentix |
| `SessionState` | `session_state.py` | Mutable session data (model, history, message) |
| `StreamingController` | `streaming_controller.py` | All LLM streaming and display logic |
| `ToolDispatcher` | `tool_dispatcher.py` | Routes tool calls to client/server executor |
| `ServiceManager` | `service_manager.py` | External service lifecycle (Ollama, Agentix) |
| `IGUIManager` | `igui_manager.py` | `Protocol` defining the GUI boundary |
| `OutputLogger` | `output_logger.py` | Session transcript file writer |
| `IMeterSession` | `protocols.py` | Runtime-checkable protocol for context-meter callbacks |
| `AttachmentInfo` | `attachment_info.py` | File attachment metadata dataclass |
| `History` | `history.py` | Loads prior sessions from disk for GUI display |
| `FileExplorer` | `file_explorer.py` | File navigation widget (list, open, history) |
| `WidgetRegistry` | `widget_registry.py` | Centralised widget lifecycle and cleanup |
| `AgentXConfig` | `config.py` | Loads/saves `agentx.toml` |
| `ModelMetadataStore` | `model_metadata_store.py` | Startup-populated model capacity/metadata cache (memory + disk) |

### src/agentx/providers/ — LLM provider abstraction

| Module | Path | Role |
|--------|------|------|
| `ILLMServiceProvider` | `providers/base.py` | Provider protocol for model listing + context length lookup |
| `OllamaServiceProvider` | `providers/ollama_provider.py` | Ollama HTTP adapter for `/api/tags` + `/api/show` |

### src/agentx/gui/ — Presentation layer

| Module | Path | Role |
|--------|------|------|
| `GUIManager` | `gui/gui_manager.py` | Thin coordinator; delegates to four panel objects |
| `ChatPanel` | `gui/chat_panel.py` | Output notebook, structured entries, plan tree tabs |
| `InputPanel` | `gui/input_panel.py` | User text input, submit/interrupt buttons, attachment bar |
| `SidePanel` | `gui/side_panel.py` | System-status pane, model selector, tabbed notebook |
| `ContextRenderer` | `gui/context_renderer.py` | Stateless widget factory for context/history/WM |
| `PlanTreeWidget` | `gui/plan_tree_widget.py` | Scrollable collapsible plan-step/sub-task tree |
| `ResynthesisDialog` | `gui/resynthesis_dialog.py` | Modal dialog for re-synthesis with WM hint |
| `ModelSelector` | `gui/model_selector.py` | Toolbar combo for switching active Ollama model |
| `SettingsTab` | `gui/settings_tab.py` | Interactive `agentx.toml` editor inside a notebook tab |
| `ToolPanel` | `gui/tool_panel.py` | Collapsible list of tool toggles (enable/disable per tool) |
| `MarkdownRenderer` | `gui/markdown_renderer.py` | HTML/Markdown rendering via `tkinterweb` |
| `CollapsibleSection` | `gui/collapsible_section.py` | Expand/collapse wrapper widget |
| `ProgressWidgets` | `gui/progress_widgets.py` | Loading spinners and progress indicators |
| `GUIConfig` | `gui/gui_config.py` | Theme colours, fonts, layout ratios |

### src/agentx/integration/ — Bridge layer

| Module | Path | Role |
|--------|------|------|
| `AgentixBridgeAdapter` | `integration/agentix_bridge_adapter.py` | Converts async Agentix → sync/generator for Tkinter |
| `ClientToolExecutor` | `integration/client_tool_executor.py` | File-system tools (read/write/list/search) |
| `ServerToolExecutor` | `integration/server_tool_executor.py` | Agentix server-side code-analysis tools |
| `WorkingMemoryToolExecutor` | `integration/working_memory_tool_executor.py` | `remember_fact`, `forget_fact`, `list_facts` agent tools |
| `ResponseHandler` | `integration/response_handler.py` | Translates `ResponseChunk` → GUI callback calls |
| `StreamingExecutor` | `integration/streaming_executor.py` | Background-thread streaming with progress tracking |
| `CodeAnalysis` | `integration/code_analysis.py` | CST/AST-based code analysis helper |

### src/agentix/ — Agentix middleware

| Module | Path | Role |
|--------|------|------|
| `AgentixBridge` | `bridge/bridge.py` | Tool loop, streaming, tool execution orchestration |
| `ToolLoopRunner` | `bridge/tool_loop.py` | Core agentic loop (LLM chunks, tool dispatch) |
| `classify_prompt` | `bridge/classify_prompt.py` | Intent classification before routing |
| `assemble_prompts` | `bridge/prompt_assembly.py` | Builds message list for API call |
| `AssertionChecker` | `bridge/assertion_checker.py` | Pre/post/invariant assertion verification |
| `QueryPayload` | `query_payload.py` | API request model → `response_format: json_object` |
| `PromptLoader` | `prompt_loader.py` | Loads system-prompt `.md` files from `system_prompts/` |
| `AgentixConfig` | `agentix_config.py` | Runtime config (model, temperature, hosts) |
| `ApiClient` | `api_client.py` | REST calls to `/v1/chat/completions` |
| `LocalClassifier` | `local_classifier.py` | Optional Torch-based local classification |
| `CstTools` | `tools/cst_tools.py` | Concrete-Syntax-Tree code analysis tools |
| `AstTools` | `tools/ast_tools.py` | Abstract-Syntax-Tree code analysis tools |
| `extract_tool_schema` | `tools/schema.py` | Python function → OpenAI JSON schema |

### src/shared/models/ — Shared data models

| Module | Path | Role |
|--------|------|------|
| `Context` | `models/context.py` | Conversation history — single source of truth, disk-backed |
| `Message` | `models/message.py` | `Message` dataclass with `MessageRole` enum |
| `ResponseChunk` | `models/response.py` | `ResponseChunk` / `ChunkType` enum |
| `WorkingMemory` | `models/working_memory.py` | Per-session key-value fact store with ownership |
| `TaskNodeRecord` | `models/task_node.py` | `PlanRecord`, `TaskNodeRecord`, `TaskTree`, `SynthesisAttempt` |
| `ToolDefinition` | `models/tools.py` | `ToolDefinition`, `ToolRegistry`, `ToolResponse` |
| `Attachment` | `models/attachment.py` | File attachment dataclass |

### src/shared/ — Shared utilities

| Module | Path | Role |
|--------|------|------|
| `token_utils` | `token_utils.py` | TOK-02 token estimation helpers (`chars_per_token`, `estimate_text_tokens`) |

### PRE-02 runtime flow notes

- `AgentXSession.__init__` creates `OllamaServiceProvider` and `ModelMetadataStore`.
- `ModelMetadataStore.populate()` is started on a background daemon thread at startup.
- Cached metadata is persisted at `sessions/_model_cache.json` and reused when the
  model set from provider `list_models()` is unchanged.
- `ModelMetadataStore.populated` is a `threading.Event` that is set after populate
  completes (including failure paths), and `invalidate()` can trigger asynchronous
  selective/full refreshes mid-session.
- Meter redraw calls use `ModelMetadataStore.get_context_length()` and are scheduled on
  the Tk main thread via `root.after(0, ...)` through session helper methods.

### System prompts (`system_prompts/`)

| File | Purpose |
|------|---------|
| `tool_use.md` | Instructs LLM how to call tools in OpenAI wire format |
| `planner_prompt.md` | Instructs LLM to decompose requests into plan steps |
| `python_coder.md` | Instructs LLM for Python coding tasks |
| `prompt_classification.md` | **Classification-only** — JSON schema instruction for `classify_prompt()` |
| `task_execution.md` | Injected per-task by `_run_task_node`; defines scope and synthesis contract |
| `structured_response.md` | Instructs LLM to emit structured JSON (internal) |
| `modifier_class_decorator.md` | Code-gen helper for class decorator patterns |

---

## 4. Class Relationships

```mermaid
classDiagram
    class AgentXSession {
        +session_id: str
        +context: Context
        +working_memory: WorkingMemory
        +gui: IGUIManager
        +client_tool_executor: ClientToolExecutor
        +server_tool_executor: ServerToolExecutor
        +execute_tool(name, input)
        +_handle_submit()
    }

    class SessionState {
        +active_model: str
        +session_folder: str
        +user: str
    }

    class StreamingController {
        +_s: AgentXSession
        +run_streaming_loop(prompt, context)
        +_display_tool_call()
        +_display_tool_result()
    }

    class ToolDispatcher {
        +_client: ClientToolExecutor
        +_server: ServerToolExecutor
        +execute_tool(name, input)
    }

    class GUIManager {
        +config: GUIConfig
        +widgets: WidgetRegistry
        +create_layout()
        +display_user_message()
        +display_agent_response()
    }

    class ChatPanel {
        +_g: GUIManager
        +_plan_trees: dict
        +display_user_message()
        +display_agent_response()
    }

    class InputPanel {
        +_g: GUIManager
        +create()
        +get_user_input()
    }

    class SidePanel {
        +_g: GUIManager
        +model_selector: ModelSelector
        +create()
        +update_context_panel()
    }

    class ContextRenderer {
        +_g: GUIManager
        +render_context_widget()
        +render_working_memory_widget()
    }

    class AgentixBridgeAdapter {
        +bridge: AgentixBridge
        +process_prompt_generator()
    }

    class AgentixBridge {
        +config: AgentixConfig
        +classify_prompt()
        +process_prompt_streaming()
        +_run_tool_loop()
    }

    class ToolLoopRunner {
        +config: AgentixConfig
        +_tool_impl_cache: dict
        +_iter_llm_chunks()
        +execute_tool()
    }

    AgentXSession --> SessionState : owns
    AgentXSession --> StreamingController : owns
    AgentXSession --> ToolDispatcher : owns
    AgentXSession --> "IGUIManager" : uses
    AgentXSession --> Context : owns
    AgentXSession --> WorkingMemory : owns
    AgentXSession --> AgentixBridgeAdapter : owns
    GUIManager ..|> "IGUIManager" : implements
    GUIManager --> ChatPanel : owns
    GUIManager --> InputPanel : owns
    GUIManager --> SidePanel : owns
    GUIManager --> ContextRenderer : owns
    SidePanel --> ModelSelector : owns
    SidePanel --> SettingsTab : owns
    SidePanel --> ToolPanel : owns
    ChatPanel --> PlanTreeWidget : "one per plan"
    AgentixBridgeAdapter --> AgentixBridge : wraps
    AgentixBridge --> ToolLoopRunner : uses
    ToolDispatcher --> ClientToolExecutor : delegates
    ToolDispatcher --> ServerToolExecutor : delegates
```

---

## 5. Session Decomposition

`AgentXSession` was decomposed into three focused collaborators.  The session
retains thin delegation stubs so the existing public API is unchanged.

```
AgentXSession
  ├── SessionState          All mutable data (model, history, folder paths)
  │     ├── active_model    property with getter/setter
  │     ├── session_folder  str
  │     ├── user            str
  │     └── detect_git_project_name() / load_agentx_instructions()  (static)
  │
  ├── StreamingController   All LLM streaming and display logic
  │     ├── run_streaming_loop(prompt, context)
  │     ├── _display_thinking(text)
  │     ├── _display_assistant_header()
  │     ├── _display_tool_call(name, input, round)
  │     ├── _display_tool_result(name, output, round, tool_id)
  │     ├── _persist_stream_messages(thinking, content)
  │     ├── _run_bootstrap_prompt_if_present()
  │     └── Workers: retrigger_synthesis_streaming / replay_task_node_streaming
  │
  └── ToolDispatcher        Tool routing
        ├── execute_tool(name, input)
        │     ├── client tools  → ClientToolExecutor.execute()
        │     ├── code analysis → ServerToolExecutor.execute()
        │     └── other         → ServerToolExecutor (fallback)
        └── WM tools are registered on AgentixBridgeAdapter (not here)
```

**Back-reference pattern**: `StreamingController` holds `self._s` (the session)
and accesses all session state through it, so tests can patch individual
attributes on a partial session object.

---

## 6. GUI Decomposition

`GUIManager` is a thin coordinator that delegates all widget creation and state
management to four panel objects.  Each panel holds `self._g = gui_manager` and
reads shared config/widgets from there.

```
GUIManager (implements IGUIManager)
  │
  ├── ContextRenderer      Stateless widget factory
  │     render_context_widget()
  │     render_history_widget()
  │     render_working_memory_widget()
  │     _render_message_to_grid()
  │     _render_tool_rows()
  │     _render_plan_rows()
  │
  ├── ChatPanel            Output notebook + plan tabs
  │     create()           builds ttk.Notebook with Chat tab
  │     display_user_message()
  │     display_agent_thinking()
  │     display_classification()
  │     display_agent_response()
  │     display_error()
  │     finalize_current_turn_markdown()
  │     add_plan_tab(plan_id, name)    → inserts PlanTreeWidget tab
  │     add_plan_step_node(…)
  │     add_plan_subtask_node(…)
  │     update_plan_node_status(…)
  │
  ├── SidePanel            Left status pane
  │     create()           builds PanedWindow left sash
  │     ├── ModelSelector  toolbar combo (top of pane)
  │     └── ttk.Notebook   with three tabs:
  │           ├── Session tab
  │           │     ├── CollapsibleSection: Working Memory
  │           │     └── CollapsibleSection: Context (message list)
  │           ├── Files tab
  │           │     └── FileExplorer widget
  │           └── Settings tab
  │                 └── SettingsTab widget
  │
  └── InputPanel           Bottom input area
        create()
        ├── attachment bar (relx 0.001, rely 0.77, relheight 0.03)
        ├── text area      (relx 0.001, rely 0.80, relheight 0.20)
        └── submit / interrupt buttons
```

**Window geometry** (absolute placement via `.place(relx, rely, …)`):

| Zone | rely | relheight | Content |
|------|------|-----------|---------|
| Main paned area | 0.00 | 0.77 | PanedWindow: SidePanel + ChatPanel |
| Attachment bar | 0.77 | 0.03 | Current + history attachments |
| Input area | 0.80 | 0.20 | Text widget + submit/interrupt |

---

## 7. Classification Pipeline

Every user prompt is classified before routing.  The endpoint is
`/v1/chat/completions` (OpenAI-compatible).

```
User prompt
  │
  ▼
classify_prompt(config, prompt, context, history, wm)
  │
  ├── Filter context to user/assistant roles only
  │     (_CLASSIFY_ROLES = {"user", "assistant"} — system messages excluded
  │      to prevent working-memory identity from contaminating the call)
  │
  ├── assemble_prompts(classification_config, messages)
  │     └── Injects prompt_classification.md as [SYSTEM] block
  │         (loaded via PromptLoader from system_prompts_dir)
  │
  ├── QueryPayload(model=classification_model, format="json")
  │     └── to_dict() → {"response_format": {"type": "json_object"}, …}
  │         (OpenAI-compat key; Ollama-native "format" is ignored by endpoint)
  │
  └── Returns PromptClassificationResponse
        ├── intent:         conversation | simple_action | complex_action | safety_issue
        ├── next_step:      respond_directly | single_tool | invoke_planner | escalate
        ├── needs_clarification: bool
        └── reasoning_summary: str

Routing decision (AgentixBridge.process_prompt_streaming):
  respond_directly  → _stream_direct_response()
  single_tool       → _stream_tool_response()   ──┐
  invoke_planner    → _stream_planned_response()──┤
  escalate          → _stream_direct_response()   │
                                                   │
                    All tool routes call _run_tool_loop()

Classification model: agentix_bench_classification_model (agentx.toml)
  default: "phi4-mini:3.8b"  (neutral model; agent persona model unsuitable)
```

**Key files**:

- `src/agentix/bridge/classify_prompt.py` — `classify_prompt()` function
- `src/agentix/query_payload.py` — `QueryPayload.to_dict()` (response_format)
- `system_prompts/prompt_classification.md` — classification instruction
- `src/agentix/agentix_config.py` — `AgentixConfig.classification_model`

---

## 8. Tool Pipeline

```
AgentixBridge._run_tool_loop(max_rounds=N)
  │
  ├── ToolLoopRunner._iter_llm_chunks()
  │     └── query_api_streaming() → /v1/chat/completions (stream=True)
  │           Accumulates OpenAI delta chunks, yields ResponseChunk objects:
  │             ChunkType.CONTENT   → text fragment
  │             ChunkType.THINKING  → reasoning fragment
  │             ChunkType.TOOL_CALL → {tool_call_id, name, arguments_json}
  │             ChunkType.DONE
  │
  ├── On TOOL_CALL chunks:
  │     execute_tool(name, arguments_dict)
  │       ├── CST/AST tools     → ToolLoopRunner._tool_impl_cache[name]
  │       ├── Registered tools  → _extra_tool_schemas implementations
  │       └── Yields ChunkType.TOOL_RESULT
  │
  └── ThreadPoolExecutor — runs multiple tool calls in parallel within a round

Wire format (Ollama/OpenAI):
  TOOL_CALL:   role=assistant, tool_calls=[{id, function:{name, arguments}}]
  TOOL_RESULT: role=tool,      content="...", tool_call_id="..."

Registering client-side tools:
  AgentixBridgeAdapter._register_client_tools()
    └── bridge.register_tool_implementations(impls, schemas)
          ├── impls:   dict[str, Callable]  (e.g. read_file → fn)
          └── schemas: list[dict]           (OpenAI tool schema per tool)

Available client tools (ClientToolExecutor):
  read_file        Read file contents (path, optional start/end lines)
  list_directory   List directory contents (path, optional pattern)
  write_file       Write or append to a file (path, content, mode)
  get_file_info    Get file metadata (size, mtime, type)
  search_files     Glob search under a directory (path, pattern)

Available Working Memory tools (WorkingMemoryToolExecutor):
  remember_fact    Store/update an agent-owned fact (key, value)
  forget_fact      Disable an agent-owned fact (key)
  list_facts       List all facts with ownership and enabled status

Available CST/AST tools (src/agentix/tools/):
  Registered via extract_cst_tools() and to_openai_tools()
  (names depend on the CST/AST modules loaded at runtime)
```

**Schema generation**: `extract_tool_schema(fn)` in `src/agentix/tools/schema.py`
converts any Python function with a typed signature + docstring into a valid
OpenAI JSON schema automatically.

---

## 9. Hierarchical Task Execution

Implemented across Phases 1–7 of `docs/hierarchical_task_execution_plan.md`.

```
invoke_planner route:
  AgentixBridge._stream_planned_response(prompt, context, classification)
    │
    ├── _create_plan(prompt, context)
    │     └── LLM call → JSON plan {steps: [{description, tbd, depends_on}]}
    │         Stored as PlanRecord in sessions/<user>/<session>/plans/
    │
    └── _run_plan(plan, context)
          for each step:
            _run_task_node(step, plan_context)
              ├── Injects task_execution.md as system prompt
              ├── Runs tool loop for this node
              ├── Synthesises result → SynthesisAttempt
              ├── Checks assertions (AssertionChecker)
              └── Stores TaskNodeRecord in sessions/…/task_nodes/
```

**PlanTreeWidget**: Live tree rendered in the GUI as execution proceeds.

- Root steps at depth 0; spawned sub-tasks at depth 1–10
- Each node shows status icon, tool calls, and synthesis text
- [Re-synth] button opens `ResynthesisDialog` for re-synthesis with WM hint
- [Export] button exports the plan tree to markdown

**Session-folder layout (extended)**:

```
sessions/<user>/<session>/
  context/                 message JSON files (one per message)
  plans/                   <epoch>_<plan_id>.json
  task_nodes/              <epoch>_<task_id>.json
  task_tree.json           TaskTree index (single file, updated in-place)
  scratch/                 ephemeral per-task scratch files
  task_tree_export.md      generated by [Export] button
  working_memory.json      WorkingMemory fact store
  session.log              full session transcript
```

---

## 10. Working Memory

Per-session key-value fact store with ownership enforcement.

```
WorkingMemory
  ├── FactOwner.USER  — user can create/mutate; agent can read only
  └── FactOwner.AGENT — agent can create/update/disable; user can promote to USER

Facts injected at session start (SessionState._load_agentx_instructions):
  user:UserName          → OS username
  user:cwd               → current working directory
  user:project           → git project name (if in a git repo)
  user:agentx-instructions → .agentx/agentx-instructions.md (agent identity)

Serialisation to LLM:
  _build_shared_context() in session.py produces a SYSTEM message:
    <working_memory>
    👤 UserName: alice
    👤 cwd: /Projects/my-project
    🤖 current_task: implement login feature
    </working_memory>

Agent-callable tools (WorkingMemoryToolExecutor):
  remember_fact(key, value)   add/update agent-owned fact
  forget_fact(key)            disable agent-owned fact
  list_facts()                enumerate all facts with status
```

Persistence: `{session_dir}/working_memory.json` — loaded at startup, saved
after every mutation.

---

## 11. Data Models

### Message

```python
@dataclass
class Message:
    role: MessageRole        # USER | ASSISTANT | SYSTEM | THINKING | TOOL | PLAN | TASK_NODE
    content: str
    epoch: float             # creation timestamp (time.time())
    enabled: bool = True     # False → excluded from LLM context
    attachments: list[Attachment] = []
    tool_calls: list[dict] = []      # role=ASSISTANT tool_call wire format
    tool_call_id: str | None = None  # role=TOOL result wire format
    tool_name: str | None = None
    # Plan/task fields
    plan_id: str | None = None
    plan_name: str | None = None
    task_id: str | None = None

# Wire format helpers
to_llm_dict()   → {"role": "user"|"assistant"|"tool", "content": "…"}
user_message(content) → Message(role=USER, …)
```

### Context

```python
class Context:
    # Main API
    add_message(message)
    get_enabled_messages() → list[Message]
    to_llm_messages()      → list[dict]  (PLAN/TASK_NODE excluded)
    add_tool_call_message(tool_name, tool_input, tool_call_id, round_index)
    add_tool_result_message(tool_name, tool_output, tool_call_id, tool_id, round_index)
    # Plan/task persistence
    save_plan(plan_record)
    load_plans() → list[PlanRecord]
    save_task_node(node_record)
    load_task_nodes() → list[TaskNodeRecord]
```

### ResponseChunk

```python
class ChunkType(str, Enum):
    CONTENT         # text fragment
    THINKING        # reasoning fragment
    TOOL_CALL       # {tool_name, tool_input, tool_call_id, round_index}
    TOOL_RESULT     # {tool_name, tool_output, tool_id, round_index}
    CLASSIFICATION  # {intent, next_step, needs_clarification, …}
    PLAN            # {plan_id, plan_name, steps}
    TASK_NODE_START # {task_id, plan_id, description, depth}
    TASK_NODE_DONE  # {task_id, synthesis_text}
    SYNTHESIS       # {task_id, content}
    ASSERTION       # {fact, type, verified, error}
    ERROR           # {content}
    DONE            #
```

### TaskNodeRecord (task_node.json schema)

```json
{
  "plan_id":                  "string",
  "task_id":                  "string",
  "parent_task_id":           "string | null",
  "depth":                    0,
  "plan_step_index":          0,
  "task_description":         "string",
  "tbd":                      false,
  "tbd_resolved_description": "string | null",
  "status":                   "pending|running|done|failed",
  "child_message_epochs":     [],
  "child_task_ids":           [],
  "synthesis_epoch":          null,
  "scratch_file":             "string | null",
  "assertions": [
    {"fact": "…", "type": "pre|post|invariant", "check": null, "verified": null, "error": null}
  ],
  "synthesis_attempts": [
    {"epoch": 0.0, "status": "accepted|rejected|pending", "rejected_epochs": []}
  ],
  "wm_hints_added":           false,
  "epoch":                    0.0,
  "enabled":                  true
}
```

---

## 12. Context Construction Pipeline

Before any prompt reaches Ollama, a five-layer filter pipeline transforms the
raw `messages` list into the final wire-format array.  Understanding all five
layers is necessary to predict which messages a model actually sees.

### Layer 0 — Assembly (`_build_shared_context`)

`AgentXSession._build_shared_context()` (`session.py`) constructs a fresh
`Context` object from **three sources in priority order**:

```
_build_shared_context() → shared_context: Context
│
├── 1. Working Memory injection (optional)
│     Condition: working_memory.enabled AND working_memory.inject_into_context
│     Effect:    prepends a SYSTEM message containing all WM facts
│
├── 2. Prior-session history
│     Source:    self.history.get_enabled_messages()
│     Effect:    appends every *enabled* message from previous sessions
│
└── 3. Current-session messages
      Source:    self.context.messages  (current session on disk)
      Condition: msg.enabled == True  (checked inline — not via get_messages())
      Effect:    appends each enabled message of the live session
```

The assembled `shared_context` is then passed to `classify_prompt()` and
`process_prompt_generator()`.

### Layer 1 — `Message.enabled` flag

Every `Message` carries an `enabled: bool` field (default `True` for new
messages).

| Scenario | `enabled` at rest |
|----------|-------------------|
| Normal turn (user/assistant/tool) | `True` |
| Message loaded from disk | `False` ← **non-obvious default** |
| `THINKING` block | `False` (hidden by default) |
| `CLASSIFICATION` metadata | `False` |
| User soft-deletes via Context panel ☑ | toggled to `False` |

The `MessageEntry` wrapper (`context.py`) delegates attribute access to the
inner `Message` via `__getattr__`, so `entry.enabled` reads
`entry.message.enabled`.  This means `get_messages(enabled_only=True)` and
`get_enabled_messages()` both work through this proxy rather than a direct
field on `MessageEntry`.

> **On-disk load default**: `load_from_dir()` sets `msg.enabled = False` and
> `att.enabled = False` for every loaded message and attachment.  The caller
> (e.g. `_build_shared_context`) must explicitly re-enable messages it wants
> in context.  New messages added via `add_message()` are **not** touched —
> they keep whatever `enabled` value was set before calling `add_message()`.

### Layer 2 — Internal role exclusion (`to_llm_messages`)

`Context.to_llm_messages()` unconditionally strips four roles regardless of
their `enabled` flag:

```python
_internal = {MessageRole.PLAN, MessageRole.TASK_NODE,
             MessageRole.SYNTHESIS, MessageRole.ASSERTION}
return [msg.to_llm_dict()
        for msg in self.get_enabled_messages()
        if msg.role not in _internal]
```

| Role | Sent to LLM? | Reason |
|------|-------------|--------|
| `USER` | Yes | |
| `ASSISTANT` | Yes | |
| `SYSTEM` | Yes | |
| `THINKING` | Yes (as `"assistant"`) | role remapped in `to_llm_dict()` |
| `TOOL_CALL` | Yes (via `tool_calls[]`) | |
| `TOOL_RESULT` | Yes (role `"tool"`) | |
| `PLAN` | **No** | task-execution metadata only |
| `TASK_NODE` | **No** | task-execution metadata only |
| `SYNTHESIS` | **No** | task-execution metadata only |
| `ASSERTION` | **No** | task-execution metadata only |
| `CLASSIFICATION` | **No** | `enabled=False` by construction |

### Layer 3 — Attachment filtering (`to_llm_dict`)

Inside `Message.to_llm_dict()`, only attachments with `attachment.enabled ==
True` are appended to `content`:

```python
enabled_attachments = [a for a in self.attachments if a.enabled]
if enabled_attachments:
    full_content += "\n\n--- Attached Files ---"
    for attachment in enabled_attachments:
        full_content += attachment.to_llm_format()
```

Attachments loaded from disk start with `enabled = False` (see Layer 1).  A
user removes an attachment chip to set it to `False`; the history chip (greyed)
is informational and its attachment is also `False`.

### Full pipeline diagram

```
AgentXSession._build_shared_context()
│
│  [WM SYSTEM msg?]  [history enabled msgs]  [current session enabled msgs]
│        │                    │                          │
│        └────────────────────┴──────────────────────────┘
│                             ▼
│                     shared_context: Context
│
│                             │ .to_llm_messages()
│                             ▼
│             ┌───────────────────────────────┐
│             │  get_enabled_messages()        │ ← Layer 1: msg.enabled == True
│             └───────────────┬───────────────┘
│                             │
│             ┌───────────────▼───────────────┐
│             │  role not in _internal         │ ← Layer 2: strip PLAN/TASK_NODE/
│             └───────────────┬───────────────┘            SYNTHESIS/ASSERTION
│                             │
│             ┌───────────────▼───────────────┐
│             │  msg.to_llm_dict()             │ ← Layer 3: att.enabled == True
│             └───────────────┬───────────────┘
│                             │
│                     list[dict]  (wire format)
│                             │
└──────────── passed to Ollama /v1/chat/completions
```

### Classification path

`classify_prompt()` (`agentix/bridge/classify_prompt.py`) receives the same
`shared_context` but builds its own `effective_history`:

```python
effective_history = list(history) if history is not None \
                    else list(context.get_enabled_messages())
# Then applies a second enabled-only pass regardless:
effective_history = [m for m in effective_history if getattr(m, "enabled", True)]
```

This means classification sees only enabled messages and respects
user-toggled soft-deletes consistently with the LLM path.

---

## 13. Tool Schemas and Usage

### Schema generation

```python
from agentix.tools.schema import extract_tool_schema

def read_file(path: str, start_line: int = 0, end_line: int = -1) -> str:
    """Read a file and return its contents.

    Args:
        path: Absolute path to the file.
        start_line: First line to read (0-indexed, inclusive).
        end_line: Last line to read (-1 = until EOF).
    """
    ...

schema = extract_tool_schema(read_file)
# Returns:
# {
#   "type": "function",
#   "function": {
#     "name": "read_file",
#     "description": "Read a file and return its contents.",
#     "parameters": {
#       "type": "object",
#       "properties": {
#         "path":       {"type": "string", "description": "Absolute path to the file."},
#         "start_line": {"type": "integer", "description": "First line to read (0-indexed, inclusive)."},
#         "end_line":   {"type": "integer", "description": "Last line to read (-1 = until EOF)."}
#       },
#       "required": ["path"]
#     }
#   }
# }
```

### Registering new tools

```python
# In AgentixBridgeAdapter or after bridge creation:
bridge.register_tool_implementations(
    implementations={"my_tool": my_tool_fn},
    schemas=[extract_tool_schema(my_tool_fn)],
)
```

### Client tool schemas (reference)

| Tool | Required params | Optional params | Returns |
|------|-----------------|-----------------|---------|
| `read_file` | `path: str` | `start_line: int`, `end_line: int` | `str` (file contents) |
| `list_directory` | `path: str` | `pattern: str` | `str` (JSON array of entries) |
| `write_file` | `path: str`, `content: str` | `mode: str` ("w"\|"a") | `str` (confirmation) |
| `get_file_info` | `path: str` | — | `str` (JSON metadata) |
| `search_files` | `path: str`, `pattern: str` | — | `str` (JSON array of paths) |

### Working Memory tool schemas (reference)

| Tool | Required params | Optional params | Returns |
|------|-----------------|-----------------|---------|
| `remember_fact` | `key: str`, `value: str` | — | `str` (confirmation or error) |
| `forget_fact` | `key: str` | — | `str` (confirmation or error) |
| `list_facts` | — | — | `str` (JSON array of fact entries) |

---

## 14. Threading Model

Tkinter must run on the main thread.  LLM streaming runs on a background thread.

```
Main thread (Tkinter)
  ├── session._handle_submit()
  │     ├── reads GUI input
  │     ├── adds user message to context
  │     └── starts StreamingController.run_streaming_loop() on background thread
  │
  └── root.after() callbacks
        └── All GUI updates from background threads scheduled via
            session._safe_root_after(lambda: …)

Background thread (started by StreamingController)
  └── StreamingExecutor iterates process_prompt_generator()
        ├── AgentixBridgeAdapter.process_prompt_generator()
        │     └── AgentixBridge.process_prompt_streaming()  (sync wrapper)
        └── For each ResponseChunk:
              session._safe_root_after(lambda: gui.display_*(…))

Coordination:
  session._is_streaming    threading.Event — set while stream is active
  session._handle_interrupt() sets interrupt flag and clears event
```

`AgentixBridgeAdapter` wraps async Agentix methods with sync calls and returns
generators.  `StreamingExecutor` iterates them on the background thread.
Never call `gui.*` directly from the background thread — always use
`_safe_root_after`.

---

## 15. Persistence Layout

```
sessions/
  _logs/                           (reserved)
  <username>/
    <session_YYYY-MM-DD_HH-MM-SS>/
      context/
        <epoch>_<role>.json        one per message
      plans/
        <epoch>_<plan_id>.json     PlanRecord (JSON)
      task_nodes/
        <epoch>_<task_id>.json     TaskNodeRecord (JSON)
      task_tree.json               TaskTree index
      working_memory.json          WorkingMemory fact store
      session.log                  full session transcript (plain text)
      task_tree_export.md          generated on [Export] click

agentx.toml                        project configuration (root)
.agentx/
  agentx-instructions.md           agent identity → injected into working memory
  bootstrap-prompt.md              first prompt sent on session start (optional)
```

---

## 16. Configuration Reference

`agentx.toml` (project root):

```toml
[agentx]
ollama_host     = "localhost:11434"
ollama_model    = "gpt-oss:latest"          # chat model
screen_side     = "right"                   # window placement
theme_mode      = "dark"

[agentix]
host                              = "localhost:8000"
system_prompts_dir                = "system_prompts"
agentix_bench_classification_model = "phi4-mini:3.8b"  # neutral classification model
classify_prompts                  = true
classification_backend            = "api"   # "api" | "torch"
```

Runtime config class: `AgentixConfig` (`src/agentix/agentix_config.py`)  
GUI config class: `GUIConfig` (`src/agentx/gui/gui_config.py`)  
App config loader: `AgentXConfig` (`src/agentx/config.py`)

---

## 17. Retrieval Keywords

| Keyword | Location |
|---------|----------|
| session lifecycle, coordinator | `src/agentx/session.py` |
| mutable session data, active model | `src/agentx/session_state.py` |
| streaming, LLM stream, display logic | `src/agentx/streaming_controller.py` |
| tool routing, execute_tool | `src/agentx/tool_dispatcher.py` |
| service startup, health check, Ollama, Agentix | `src/agentx/service_manager.py` |
| GUI interface, protocol, IGUIManager | `src/agentx/igui_manager.py` |
| GUI coordinator, panels | `src/agentx/gui/gui_manager.py` |
| chat output, plan tabs, streaming display | `src/agentx/gui/chat_panel.py` |
| input box, submit, attachment bar | `src/agentx/gui/input_panel.py` |
| side panel, model selector, context tab | `src/agentx/gui/side_panel.py` |
| context rendering, message grid, working memory widget | `src/agentx/gui/context_renderer.py` |
| plan tree, task node widget, re-synthesis | `src/agentx/gui/plan_tree_widget.py` |
| settings tab, toml editor | `src/agentx/gui/settings_tab.py` |
| open file, file explorer, navigation | `src/agentx/file_explorer.py` |
| config load/save | `src/agentx/config.py` |
| agentix bridge, async to sync | `src/agentx/integration/agentix_bridge_adapter.py` |
| file tools, read_file, write_file, list_directory | `src/agentx/integration/client_tool_executor.py` |
| working memory tools, remember_fact | `src/agentx/integration/working_memory_tool_executor.py` |
| response chunk processing, GUI callback | `src/agentx/integration/response_handler.py` |
| tool loop, agentic loop, LLM chunks | `src/agentix/bridge/tool_loop.py` |
| classification, intent, next_step | `src/agentix/bridge/classify_prompt.py` |
| prompt assembly, system prompt injection | `src/agentix/bridge/prompt_assembly.py` |
| assertion check, pre/post/invariant | `src/agentix/bridge/assertion_checker.py` |
| API request, response_format, json_object | `src/agentix/query_payload.py` |
| system prompt loading, PromptLoader | `src/agentix/prompt_loader.py` |
| conversation history, to_llm_messages | `src/shared/models/context.py` |
| message dataclass, MessageRole | `src/shared/models/message.py` |
| chunk type, ResponseChunk | `src/shared/models/response.py` |
| working memory, fact store, ownership | `src/shared/models/working_memory.py` |
| plan record, task node, synthesis, assertion | `src/shared/models/task_node.py` |
| tool schema, extract_tool_schema, OpenAI schema | `src/agentix/tools/schema.py` |
| CST tools, AST tools, code analysis | `src/agentix/tools/cst_tools.py`, `ast_tools.py` |
| diagnostics, health check CLI | `agentx_diagnostics.py` |

Version: 2026-02-17
Author: GitHub Copilot Chat Assistant (for maintainers' review)

Summary
-------

This document describes the high-level architecture and main functional components of the AgentX application (branch: smolagents). It is written and structured to be assistant-friendly: it contains short module summaries, a searchable index of key files and entrypoints, recommended retrieval keywords, and suggested prompts for automated agents to reason about the repo.

Quick links
-----------

- Repository: <https://github.com/matthewapeters/agentX>
- Branch scanned: smolagents

NOTE: The repository scan used to generate this document was programmatic and limited to the top-level and several src files. The scan may be incomplete — view the repository in GitHub for the complete file tree.

Index (high-level)
-------------------

1. Entrypoints
   - src/agentx/**main**.py  — app launch wrapper
   - src/agentx/main.py      — GUI application main()
   - agentx_diagnostics.py   — CLI health checks for dependencies and services
2. Core runtime components
   - src/agentx/session.py       — AgentXSession: session lifecycle, GUI wiring, tool executors
   - src/agentx/service_manager.py — ServiceManager: manage external services (Ollama, Agentix)
   - src/agentx/config.py        — load/save configuration
3. UI & UX
   - src/agentx/gui/*            — GUI manager and configuration (GUIManager, GUIConfig)
   - src/agentx/widget_registry.py — centralized widget references and lifecycle
   - gui_manager.md              — human-focused documentation for GUI layout and behavior
4. Persistence & history
   - src/agentx/history.py       — session/history loader and GUI adapter
   - sessions/ (runtime)         — saved contexts/messages per user session (created at runtime)
5. File and code tooling
   - src/agentx/file_explorer.py — file navigation and file reading helper
   - src/agentx/integration/*    — Agentix bridge adapters, tool executors, advanced tool registry
   - src/agentx/agentx_diagnostics.py — service diagnostics CLI (root-level script)
6. Shared models and adapters
   - src/shared/*                — data models used across the app (Context, Message, ResponseChunk, etc.)
   - src/agentix/*               — agentix-specific helpers/bridges (adapter code)
7. Documentation and PoC
   - smolagentx.md               — SmolAgents integration notes (PoC plan)
   - docs/architecture.md        — (this document)

Module summaries (concise)
--------------------------

- agentx.**main** / agentx.main
  - Purpose: Launch the AgentX Tkinter GUI application. Creates AgentXSession and runs the main loop.
  - Key behavior: Calls session.perform_service_handshake() then session.layout() and mainloop(); ensures ServiceManager shutdown on exit.

- agentx.config
  - Purpose: Read and write agentx.toml configuration. Provide helper to obtain icon file paths from packaged assets.
  - Notes: Uses toml load/dump; DEFAULT_CONFIG = agentx.toml.

- agentx.session
  - Purpose: Central orchestration for a user session. Creates context, file explorer, GUI manager, service manager, Agentix adapter, tool executors, and history storage.
  - Key responsibilities:
    - Maintain Context and session folder structure
    - Initialize ServiceManager and Agentix bridge
    - Initialize GUIManager and wire callbacks for submit/interrupt/attachments
    - Provide streaming/async handling for assistant responses
  - Integration points: ollama client, Agentix bridge, ClientToolExecutor/ServerToolExecutor, AdvancedToolRegistry

- agentx.service_manager
  - Purpose: Manage external dependencies (Ollama, Agentix). Start processes if needed, perform HTTP health checks, and shutdown.
  - Important: Encapsulates host/port parsing, health endpoint checks, subprocess management and graceful termination.

- agentx.file_explorer
  - Purpose: Provide file navigation utilities (list directories, open files, navigate history). Used by GUI to display project files and allow agents or users to open files for inspection.

- agentx.history
  - Purpose: Load prior session contexts from disk, expose enabled messages and attachments to the GUI, and render history into GUI frames.

- widget_registry
  - Purpose: Central place to keep references to UI widgets for lifecycle and cleanup.

- agentx_diagnostics.py (root)
  - Purpose: CLI helper to verify environment readiness (Ollama, Agentix, python deps). Contains checks for httpx, ollama, toml, libcst, and agentix.
  - Usage: python agentx_diagnostics.py

Assistant-friendly features included in this document
----------------------------------------------------

- Short summaries (1–3 lines) for each major module to allow quick retrieval.
- Index mapping file paths to functional descriptions so an assistant can locate code to answer questions (e.g., "Where is service startup logic?").
- Suggested retrieval keywords for each module (see next section).
- Suggested prompts for multi-turn / tool-invocation workflows.

Suggested retrieval keywords (examples)
--------------------------------------

- service startup, health check -> src/agentx/service_manager.py
- session lifecycle, GUI wiring -> src/agentx/session.py
- open file, file explorer -> src/agentx/file_explorer.py
- diagnostics -> agentx_diagnostics.py
- agent integration, tool executor -> src/agentx/integration/
- config load/save -> src/agentx/config.py

Suggested prompts for an assistant (templates)
----------------------------------------------

- "Summarize the responsibilities of src/agentx/service_manager.py in 3 bullet points."
- "List functions that start subprocesses or call external services across the repo."
- "Find functions that read/write files under sessions/ and summarize their formats."
- "Generate unit tests for file_explorer.open_file to validate behavior on unreadable files."

Practical next steps to make it more assistant-friendly
-------------------------------------------------------

1. Add a docs/index.md (topic-based table of contents) with permalinks to each module and short descriptions.
2. Store short one-line summaries as module-level docstrings (where missing); they are easy to extract programmatically.
3. Add a small metadata file .repo_index.json that maps keywords -> file paths and includes concise summaries; used by search/routers.
4. Add code-level docstrings and type hints for integration adapters to improve static analysis and assist agent reasoning.

Files created/updated
---------------------

- docs/architecture.md  (created)
- smolagentx.md         (already present and updated earlier)

Actions performed
-----------------

- Programmatic review of top-level files and src/agentx components on branch `smolagents` to generate this architecture summary.
- Note: The automated scan focused on several core modules and may not cover every file in the repository. For a full index, I can run a complete file tree scan and produce a machine-readable mapping (.repo_index.json) if you’d like.

Commit
------

This document will be committed to docs/architecture.md on branch smolagents
---

Tool Pipeline (added 2026-03-08)
---------------------------------

The agentic tool-usage pipeline was implemented across Phases 1-6 of
docs/tool_usage_plan.md. The key components are:

  User prompt
    └── AgentixBridgeAdapter.process_prompt_generator()
          └── AgentixBridge.process_prompt_streaming()
                ├── classify_prompt()  →  NextStep
                ├── RESPOND_DIRECTLY   →  _stream_direct_response()
                ├── SINGLE_TOOL        →  _stream_tool_response()   ─┐
                ├── INVOKE_PLANNER     →  _stream_planned_response() ─┤
                └── ESCALATE           →_stream_direct_response()  │
                                                                      │
                    All tool routes call _run_tool_loop(max_rounds=N)─┘
                      ├── _iter_llm_chunks()  — OpenAI-compat stream
                      ├── execute_tool()      — dispatch by name
                      │     ├── CST/AST tools (agentix.tools.*)
                      │     └── File tools    (ClientToolExecutor)
                      └── ThreadPoolExecutor — parallel multi-tool rounds

Key tool pipeline files:
  src/agentix/bridge/bridge.py               — _run_tool_loop, execute_tool, register_tool_implementations
  src/agentix/tools/schema.py                — extract_tool_schema(fn) → OpenAI JSON schema
  src/shared/models/tools.py                 — ToolDefinition, ToolRegistry, ToolResponse
  src/shared/models/message.py               — to_llm_dict() with TOOL_CALL/TOOL_RESULT wire format
  src/shared/models/context.py               — add_tool_call_message(), add_tool_result_message()
  src/agentx/integration/client_tool_executor.py — file tool wrappers + get_client_tool_schemas()
  src/agentx/integration/agentix_bridge_adapter.py —_register_client_tools() on init
  src/agentx/session.py                      — _display_tool_call(),_display_tool_result()
  system_prompts/tool_use.md                 — LLM guidance for tool use

Tool message wire format (Ollama/OpenAI):
  TOOL_CALL:   {"role": "assistant", "tool_calls": [{"id": ..., "function": {"name": ..., "arguments": "..."}}]}
  TOOL_RESULT: {"role": "tool", "content": "...", "tool_call_id": "..."}

Tests added (tool pipeline):
  tests/test_tool_schema.py                        (15 tests)
  tests/test_message_wire_format.py                (24 tests)
  tests/test_client_tool_integration.py            (21 tests)
  tests/integration/test_ollama_tool_stream.py     (13 tests)

---

Hierarchical Task Execution (added 2026-03-29)
-----------------------------------------------

Implemented across Phases 1–7 of docs/hierarchical_task_execution_plan.md.

Key files:
  src/shared/models/task_node.py                — PlanRecord, PlanStep, TaskNodeRecord, SynthesisAttempt, AssertionRecord, TaskTree
  src/agentix/bridge/bridge.py                  —_create_plan,_run_plan,_run_task_node, retrigger_synthesis_streaming, replay_task_node_streaming
  src/agentx/gui/plan_tree_widget.py            — PlanTreeWidget: scrollable tree with step/sub-task nodes, synthesis, assertions
  src/agentx/gui/resynthesis_dialog.py          — ResynthesisDialog: modal UI for re-synthesis with WM hint
  system_prompts/task_execution.md              — injected by _run_task_node only (role, synthesis contract, scope rules)

Session-folder layout:
  sessions/<user>/<session>/
    context/                   ← message files (existing)
    plans/                     ← <epoch>_<plan_id>.json  (one PlanRecord each)
    task_nodes/                ← <epoch>_<task_id>.json  (one TaskNodeRecord each)
    task_tree.json             ← TaskTree index (single file, updated in-place)
    scratch/                   ← ephemeral per-task scratch files
    task_tree_export.md        ← generated by [Export] button in plan tab toolbar

task_node.json schema (TaskNodeRecord):
  {
    "plan_id":                  string,          // parent plan
    "task_id":                  string,          // unique node id
    "parent_task_id":           string | null,   // null for root steps
    "depth":                    int,             // 0 = root, max 10
    "plan_step_index":          int,             // index in parent plan.steps[]
    "task_description":         string,
    "tbd":                      bool,            // placeholder step
    "tbd_resolved_description": string | null,   // resolved at runtime
    "status":                   "pending"|"running"|"done"|"failed",
    "child_message_epochs":     float[],         // epoch refs for context messages
    "child_task_ids":           string[],        // spawned sub-task IDs
    "synthesis_epoch":          float | null,    // epoch of accepted synthesis message
    "scratch_file":             string | null,   // relative path inside scratch/
    "assertions": [
      {
        "fact":     string,
        "type":     "pre"|"post"|"invariant",
        "check":    string | null,
        "verified": bool | null,
        "error":    string | null
      }
    ],
    "synthesis_attempts": [
      { "epoch": float, "status": "accepted"|"rejected"|"pending", "rejected_epochs": float[] }
    ],
    "wm_hints_added":           bool,
    "epoch":                    float,           // creation timestamp
    "enabled":                  bool
  }

task_tree.json schema (TaskTree):
  {
    "session_id":          string,
    "created_epoch":       float,
    "last_updated_epoch":  float,
    "plans": {
      "<plan_id>": {
        "plan_id":            string,
        "plan_name":          string,
        "session_plan_index": int,
        "steps": [
          { "step_id": string, "description": string, "tbd": bool, "depends_on": string[] }
        ],
        "root_task_ids":  string[],
        "status":         "pending"|"running"|"done"|"failed",
        "epoch":          float
      }
    },
    "nodes": {
      "<task_id>": { ... }    // same schema as task_node.json above
    }
  }

Retrieval keywords:
  PlanRecord, TaskNodeRecord, TaskTree, task_tree.json, plan tab, replay, export task tree
  → src/shared/models/task_node.py, src/agentx/gui/plan_tree_widget.py, src/agentx/session.py
