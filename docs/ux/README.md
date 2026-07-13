# AgentX — UX Documentation

_Last updated: 2026-05-22 (v0.74.4.post2)_

This directory documents the AgentX user interface — layout, affordances,
user flows, and per-surface detail.

---

## Contents

| File | Description |
|------|-----------|
| **[00_INDEX.md](00_INDEX.md)** | **Session entry point — reconciliation status and document map** |
| **[UX_LIFECYCLE.md](UX_LIFECYCLE.md)** | **Lifecycle rules, affordance ID scheme, full traceability matrix (not yet reconciled to the current implementation — see its banner)** |
| [04_COMPONENT_CUT_SHEET_TEMPLATE.md](04_COMPONENT_CUT_SHEET_TEMPLATE.md) | Component cut-sheet template (identity, diagrams, Gherkin, test mapping) |
| [01_MAIN_LAYOUT.md](01_MAIN_LAYOUT.md) | Window geometry, zone map, layout diagram |
| [02_USER_FLOWS.md](02_USER_FLOWS.md) | Mermaid flow diagrams for all major user interactions |
| [03_PANEL_DETAILS.md](03_PANEL_DETAILS.md) | Per-surface affordances, widgets, and interactions — reconciled to the current client-server implementation |
| [05_VIBE_CODING.md](05_VIBE_CODING.md) | Editor + terminal integration UX contract |
| [06_OUTPUT_WIDGET.md](06_OUTPUT_WIDGET.md) | Output-panel widget contract |
| [07_DEMO_MODE.md](07_DEMO_MODE.md) | Demo mode UX contract and implementation plan (not yet reconciled — see its banner) |
| [UX_ISSUES.md](UX_ISSUES.md) | Bug-tracking log for user-reported UX defects |

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
