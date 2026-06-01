# System Panel Tab Mapping

_Last updated: 2026-05-28 (v0.84.0)_

## Purpose

Define the runtime mapping for System panel tabs (files, configuration, context,
context history, context visualizer), including source-of-truth provider and render adapter.
This document remains a current-state mapping artifact for the Go-owned system panel.

## Runtime Scope

- Surface: tmux `system` pane
- Runtime renderer (current): Go context widget in `cmd/agentx-core/context_widget.go`
- Data source boundary (current): core `GET /context` endpoint from `cmd/agentx-core/core.go`

## Tab Mapping

| System Tab | Primary Data Provider (SoT) | Render Adapter | Current Runtime Owner | Notes |
| --- | --- | --- | --- | --- |
| files | Project filesystem + context endpoint metadata (`turn_count`) | `renderContextWidget(..., "files", ...)` in `cmd/agentx-core/context_widget.go` | Go current | Uses project root listing + deterministic preview line. |
| configuration | Runtime config (`agentx.toml` + env) | `renderContextWidget(..., "configuration", ...)` in `cmd/agentx-core/context_widget.go` | Go current | Shows model/backend/host contract used by runtime. |
| context | Core context snapshot (`/context`: session_id, turns, prompt_cycle) | `renderContextWidget(..., "context", ...)` in `cmd/agentx-core/context_widget.go` | Go current | Uses latest turn summary and session metadata. |
| context history | Core persisted turn history (`/context.turns`) | `renderContextWidget(..., "context-history", ...)` + system applet host | Go current | Summarizes count + recent prompt fidelity. |
| context visualizer | Derived token-band metrics from turns + `/context.prompt_cycle` | `renderContextWidget(..., "context-visualizer", ...)` in `cmd/agentx-core/context_widget.go` | Go current | Includes consumed%, contributors, emoji meter rows, and prompt-cycle rows. |

## Provider Contract (Current)

`GET /context` response fields consumed by the System pane:

- `session_id`: session identifier used for context grouping
- `turn_count`: deterministic turn count
- `turns`: ordered prompt/response history
- `prompt_cycle`: classify/thinking/tool/respond phase state + elapsed_ms

These fields are produced by core in `cmd/agentx-core/core.go` via `ContextManager.HealthHandler()`.

## Render Contract (Current)

The `system` pane must render:

- `== CONTEXT WINDOW ==` usage summary + contributor lines
- emoji meter rows for working memory/system/user/attachments/thinking/assistant/tool/remaining
- `== PROMPT CYCLE ==` rows for classify/think/tool/respond
- no `== SESSION SNAPSHOT ==` block in the System pane

## Current Control Notes

The Go-owned system widget implementation preserves backward compatibility with the
existing full-surface system rendering contract used by parity tests.

Implemented controls:

- default mode remains `full` (renders all mapped sections)
- tab selector supports `files|configuration|context|context-history|context-visualizer|full`
- selected tab is resolved deterministically from:

 1. state file (`.agentx/system-panel-tab.txt`, override: `AGENTX_SYSTEM_PANEL_TAB_STATE_FILE`)
 2. env fallback (`AGENTX_SYSTEM_PANEL_TAB`)
 3. default (`full`)

Validation evidence:

- `tests/test_system_tab_routing_headless.sh` verifies:
  - deterministic initial tab routing (`files`)
  - stateful switch to `context-visualizer` after updating the tab-state file
