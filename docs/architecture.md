# AgentX — Architecture and Assistant-Friendly Index

Version: 2026-02-17
Author: GitHub Copilot Chat Assistant (for maintainers' review)

Summary
-------
This document describes the high-level architecture and main functional components of the AgentX application (branch: smolagents). It is written and structured to be assistant-friendly: it contains short module summaries, a searchable index of key files and entrypoints, recommended retrieval keywords, and suggested prompts for automated agents to reason about the repo.

Quick links
-----------
- Repository: https://github.com/matthewapeters/agentX
- Branch scanned: smolagents

NOTE: The repository scan used to generate this document was programmatic and limited to the top-level and several src files. The scan may be incomplete — view the repository in GitHub for the complete file tree.

Index (high-level)
-------------------
1. Entrypoints
   - src/agentx/__main__.py  — app launch wrapper
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
- agentx.__main__ / agentx.main
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
This document will be committed to docs/architecture.md on branch smolagents.
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
                └── ESCALATE           →  _stream_direct_response()  │
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
  src/agentx/integration/agentix_bridge_adapter.py — _register_client_tools() on init
  src/agentx/session.py                      — _display_tool_call(), _display_tool_result()
  system_prompts/tool_use.md                 — LLM guidance for tool use

Tool message wire format (Ollama/OpenAI):
  TOOL_CALL:   {"role": "assistant", "tool_calls": [{"id": ..., "function": {"name": ..., "arguments": "..."}}]}
  TOOL_RESULT: {"role": "tool", "content": "...", "tool_call_id": "..."}

Tests added (tool pipeline):
  tests/test_tool_schema.py                        (15 tests)
  tests/test_message_wire_format.py                (24 tests)
  tests/test_client_tool_integration.py            (21 tests)
  tests/integration/test_ollama_tool_stream.py     (13 tests)
