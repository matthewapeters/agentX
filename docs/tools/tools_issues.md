# TOOLS ISSUES

_Last updated: 2026-05-11 (v0.46.0)_

This file tracks non-UX tool and tooling issues (tool registry, tool exposure, tool intent routing, and editor-tool integrations).

## How to Use This File

Use the same semaphore markers as UX issue tracking:

| Marker | Set by | Meaning |
|--------|--------|---------|
| `[ ]`  | User   | Issue reported; agent has not yet applied a fix. |
| `[/]`  | Agent  | Fix committed and all tests pass; ready for UAT. |
| `[X]`  | Either | Fix attempted but failed or blocked; needs follow-up. |

## Scope Boundary

- Keep GUI/interaction defects in `docs/ux/UX_ISSUES.md`.
- Keep tool architecture, tool registry, tool discoverability, and tool invocation defects here.

---

## Execution Sequence

This backlog is organized by dependency order. Complete items in sequence to minimize rework:

| Phase | Item | Purpose | Blocks |
|-------|------|---------|--------|
| 1 | Dynamic tool registry | Foundation for all subsequent tool discovery | Items 2–6 |
| 2 | Create/edit file wiring | Verify existing tool execution path | Items 4–6 |
| 3 | Vibe-editor intent routing | Route editor-related language to editor tools | Items 4–6 |
| 4 | Open-file tool | Agent can invoke editor | Item 5–6 |
| 5 | Diff tool | Agent can compare files in editor | Item 6 |
| 6 | Editor-assist tool | Agent can propose edits and provide help | (terminal safety required) |

---

## Issues

**[/] Phase 1: Dynamic Tool Registry (FOUNDATION)**

- [/] **P1-001**: AgentX needs a dynamic tool registry. Implement a file-backed registry of available tools (local scripts, local system tools, and built-ins) so the registry can be updated by AgentX and consumed by the UX tools enable/disable widget. This is the prerequisite for all downstream tool discovery and exposure.
  - **Implementation**: `src/agentx/tool_registry.py` — ToolRegistry class loads from `agentx_tools.toml`
  - **Manager**: `src/agentx/integration/tool_registry_manager.py` — ToolRegistryManager integrates registry with bridge and UI
  - **Built-in tools**: `reload_tools()` and `register_tool()` for dynamic tool management
  - **Integration**: Wired into `AgentixBridgeAdapter._register_registry_tools()` and session layout
  - **UI wiring**: `ToolPanel` now receives registry tools with enable/disable state
  - **Tests**: 39 unit tests + integration tests (all passing)
  - **Config**: `agentx_tools.toml` defines built-in tools (cst, ast, read_file, write_file, list_directory, get_file_info, search_files)

**[/] Phase 2: Tool Execution Diagnostics**

- [/] **P2-001**: Agent does not seem able to create or edit files. Verify end-to-end tool wiring from tool discovery (in registry) to execution. Add explicit diagnostics for missing tool exposure. Use `read_file`, `write_file` as test cases. Ensure agent can invoke them end-to-end.
  - **Implementation**: `src/agentx/tool_diagnostics.py` — ToolDiagnostics class with 4-phase suite (registry, bridge, availability, execution)
  - **Built-in tool wiring**: `diagnose_tools()` added to ToolRegistryManager and registered in bridge schemas
  - **Integration**: ToolRegistryManager now receives bridge reference for runtime diagnostics
  - **Verification**: targeted tests pass for manager + integration + diagnostics suites
  - **Status**: Ready for UAT (agent can self-diagnose tool pipeline health)

**[/] Phase 3: Vibe-Editor Intent Routing**

- [/] **P3-001**: AgentX needs explicit awareness of vibe-editor intents (neovim in tmux pane 0): "open file", "open <file> in vibe editor", "edit file", and similar phrasings should reliably route to the editor-open capability. Update prompt classification or prompt router to map these intents to the tool set.
  - **Implementation**: `src/agentix/bridge/classify_prompt.py` — added deterministic pattern-based override for explicit vibe-editor phrasing.
  - **Routing behavior**: explicit editor intent short-circuits to `intent=simple_action`, `next_step=single_tool` for reliable tool-route selection.
  - **Tests**: `tests/test_classify_prompt_bridge.py` verifies short-circuit routing and confirms non-editor prompts still flow through normal LLM classification.

**[ ] Phase 4–6: Editor Tools (Vibe-Coding Integration)**

- [/] **P4-001**: AgentX needs a tool that opens a file in the vibe editor. Expose `VimBridge.open_file()` as an agent tool schema. Register in the tool registry (P1-001). Wire into session and bridge executors.
  - **Implementation**: `src/agentx/integration/tool_registry_manager.py` — added `builtin_open_file_in_editor(file_path, line)` wired through `VimBridge.open_file_from_context()`.
  - **Bridge registration**: `src/agentx/integration/agentix_bridge_adapter.py` now exposes `open_file_in_editor` schema and registers implementation with the built-in registry tools.
  - **Registry metadata**: `src/agentx/tool_registry.py` default `system_tools` now includes `open_file_in_editor = true`.
  - **Tests**: `tests/test_tool_registry_manager.py` covers required-arg validation, success path, and editor-unavailable failure path.

- [ ] **P5-001**: AgentX needs a tool that diffs files in the vibe editor. Implement `VimBridge.diff_files()` to invoke `nvim --server` with vimdiff semantics. Register in tool registry. Wire into executor.

- [ ] **P6-001**: AgentX needs a tool that performs editor-assist actions in vibe editor (for example: propose edits, show symbol help, and autocomplete-style assist) while preserving terminal safety controls. Implement `VimBridge.editor_action()` with sandboxed key-injection and output capture. Integrate with command approval UI (PD-15-AF-006).
