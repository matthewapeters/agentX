# TOOLS ISSUES

_Last updated: 2026-05-11 (v0.48.0)_

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

- [/] **P1-001**: AgentX needs a dynamic tool registry. Implement a file-backed registry of available tools (local scripts, local system tools, and built-ins) so the registry can be updated by AgentX and consumed by the UX tools enable/disable widget. This is the prerequisite for all downstream tool discovery and exposure. Built-in tools include: `read_file`, `write_file`, `list_directory`, `get_file_info`, `search_files`. Config drives which tools are enabled at runtime.

**[/] Phase 2: Tool Execution Diagnostics**

- [/] **P2-001**: Agent does not seem able to create or edit files. Verify end-to-end tool wiring from tool discovery (in registry) to execution. Add explicit diagnostics for missing tool exposure. Use `read_file`, `write_file` as test cases. Ensure agent can invoke them end-to-end. Agent must be able to self-diagnose tool pipeline health.

**[/] Phase 3: Vibe-Editor Intent Routing**

- [/] **P3-001**: AgentX needs explicit awareness of vibe-editor intents: "open file", "open <file> in vibe editor", "edit file", and similar phrasings should reliably route to the editor-open capability. Update the prompt router to map these intents to the editor tool set using deterministic pattern-based override before falling back to LLM classification.

**[ ] Phase 4–6: Editor Tools (Vibe-Coding Integration)**

- [/] **P4-001**: AgentX needs a tool that opens a file in the editor integration. Register `open_file_in_editor` in the tool registry and wire into the LLM bridge executor. Required-arg validation, success path, and editor-unavailable failure path must be covered.

- [/] **P5-001**: AgentX needs a tool that diffs files in the editor. Implement `diff_files_in_editor` with vimdiff semantics (or equivalent). Register in tool registry. Wire into executor. Cover validation, success, and failure paths.

- [/] **P6-001**: AgentX needs a tool that performs editor-assist actions (propose edits, show symbol help, autocomplete-style assist) while preserving terminal safety controls. Editor actions must route through terminal policy and supervised approval (PD-15-AF-006). Supported action allowlist, payload sanitization, and subprocess output capture are required.
