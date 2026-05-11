# TOOLS ISSUES

_Last updated: 2026-05-11 (v0.39.3.post1)_

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

## Issues

- [ ] AgentX needs a dynamic tool registry. Implement a file-backed registry of available tools (local scripts, local system tools, and built-ins) so the registry can be updated by AgentX and consumed by the UX tools enable/disable widget.
- [ ] Agent does not seem able to create or edit files. Verify end-to-end tool wiring from tool discovery to execution and add explicit diagnostics for missing tool exposure.
- [ ] AgentX needs explicit awareness of vibe-editor intents (neovim in tmux pane 0): “open file”, “open <file> in vibe editor”, “edit file”, and similar phrasings should reliably route to the editor-open capability.
- [ ] AgentX needs a tool that opens a file in the vibe editor.
- [ ] AgentX needs a tool that diffs files in the vibe editor.
- [ ] AgentX needs a tool that performs editor-assist actions in vibe editor (for example: propose edits, show symbol help, and autocomplete-style assist) while preserving terminal safety controls.
