# AgentX — UX Documentation

Version: 2026-04-19

This directory documents the AgentX user interface — layout, affordances,
user flows, and per-panel detail.

---

## Contents

| File | Description |
|------|-------------|
| [01_MAIN_LAYOUT.md](01_MAIN_LAYOUT.md) | Window geometry, zone map, layout diagram |
| [02_USER_FLOWS.md](02_USER_FLOWS.md) | Mermaid flow diagrams for all major user interactions |
| [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md) | Per-panel affordances, widgets, and interactions |

---

## Quick Reference: Message Role Icons

| Icon | Role | Description |
|------|------|-------------|
| 👤 | `user` | User-submitted message |
| 🤖 | `assistant` | LLM response |
| ⚙️ | `system` | System instruction (hidden from main chat view) |
| 💭 | `thinking` | LLM reasoning / thinking block |
| 🔧 | `tool_call` | Tool invocation sent to LLM |
| 📋 | `tool_result` | Tool result returned to LLM |
| 🛠️ | `tools` | Tool group header |
| 📋 | `plan` | Plan record (plan-tree header) |
| 🌿 | `task_node` | Task-node (individual plan step) |

---

## Quick Reference: Plan Step Status Icons

| Icon | Status | Meaning |
|------|--------|---------|
| ○ | `pending` | Not yet started |
| ● | `running` | Currently executing |
| ✓ | `done` | Completed successfully |
| ? | `needs_review` | Requires user attention |
| ✗ | `failed` | Failed or blocked |

---

## Key UX Principles

- **Streaming-first**: responses appear word-by-word in real time.
- **Non-blocking**: the interrupt button is always reachable during a stream.
- **Progressive disclosure**: thinking blocks and tool interactions are collapsed
  by default; clicking expands them inline.
- **Persistent sessions**: the full conversation, plan tree, and working memory
  survive app restart.
- **Keyboard-forward**: `Enter` sends the message; `Shift+Enter` adds a newline.
