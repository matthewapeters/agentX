# Changelog

All notable changes to AgentX are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versioning follows [Semantic Versioning](https://semver.org/).

---

## [0.18.22-f] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10: rewrote self-referential arrow labels to remove leading underscores (`_interrupt_flag`, `_is_streaming`). Mermaid processes label text as Markdown, so `_word_` patterns are interpreted as italic markers, corrupting the token stream and causing "No diagram type detected". Replaced with plain descriptive text: `set interrupt flag = True` and `clear is_streaming event`. Also simplified the Note and partial-response label text.

---

## [0.18.22-e] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10: replaced em dash `—` with a plain hyphen `-` in the `Note` text; the Unicode em dash caused "No diagram type detected" in the VS Code Mermaid preview renderer.

---

## [0.18.22-d] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — UF-10 Mermaid parse error: added missing `participant Chat as ChatPanel` declaration and replaced `\n`-delimited `Note` text with a single-line string (inline `\n` escapes in `Note` blocks are not supported by all Mermaid versions and caused a NEWLINE parse error).

---

## [0.18.22-c] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/02_USER_FLOWS.md` — replaced `actor User` with `participant User` in all 9 Mermaid sequence diagrams (UF-01 through UF-10 excluding UF-09). The `actor` keyword is not supported by the Mermaid version bundled with VS Code's markdown preview, causing "No diagram type detected" errors.

---

## [0.18.22-b] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Fixed

- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Window Layout Mockup**: ChatPanel is now shown on the **left (~66%)** and SidePanel on the **right (~34%)**, matching the actual `PanedWindow` widget order in `chat_panel.py` and `side_panel.py`.
- `docs/ux/01_MAIN_LAYOUT.md` — removed incorrect model-selector widget from the OS title bar in the mockup (it lives inside SidePanel, not the title bar).
- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Zone Map** sash table: left pane is ChatPanel (~66%), right pane is SidePanel (~34%); was previously inverted (25%/75%).
- `docs/ux/01_MAIN_LAYOUT.md` — corrected **Component Index** `Left pane` / `Right pane` labels and percentages.
- `docs/ux/01_MAIN_LAYOUT.md` — corrected §4 SidePanel and §5 ChatPanel position strings to match actual sash proportions.
- `docs/ux/01_MAIN_LAYOUT.md` — added `screen_side` clarification note: this setting controls which side of the **monitor** the window is placed on, not the internal panel arrangement.
- `docs/ux/01_MAIN_LAYOUT.md` — added **Detail Diagram References** table beneath the mockup, providing `[→ PD-XX]` annotations on every generalised component and linking to corresponding sections in `03_PANEL_DETAILS.md` (quality gate: generalised components must carry detail diagram references).
- `docs/ux/03_PANEL_DETAILS.md` — corrected **PD-01 ChatPanel** position from "Right ~75%" to "Left ~66% (PanedWindow left pane)".
- `docs/ux/03_PANEL_DETAILS.md` — corrected **PD-03 SidePanel** position from "Left ~25%" to "Right ~34% (PanedWindow right pane)".

---

## [0.18.22-a] - 2026-04-19

### Code Changes

#### Changed

- No source code changes in this release (documentation only).

### Test Changes

#### Changed

- No test changes in this release.

### Documentation Changes

#### Added

- `docs/ux/README.md` — new UX documentation index with message-role icon legend and key UX principles.
- `docs/ux/01_MAIN_LAYOUT.md` — main window layout mockup with zone map, component index, and per-panel layout details (SidePanel, ChatPanel, InputPanel).
- `docs/ux/02_USER_FLOWS.md` — Mermaid sequence and flowchart diagrams for all 10 major user flows: basic chat, tool execution, hierarchical task execution, re-synthesis, file attachment, model switch, settings change, session history navigation, working memory management, and interrupt streaming.
- `docs/ux/03_PANEL_DETAILS.md` — per-panel affordance specifications for ChatPanel, InputPanel, SidePanel, ModelSelector, PlanTreeWidget, ResynthesisDialog, SettingsTab, ContextRenderer, CollapsibleSection, and ToolPanel.

#### Changed

- `docs/architecture.md` — full rewrite to reflect current codebase state (2026-04-19). Updated module map tables, class relationship Mermaid diagram, session decomposition (SessionState/StreamingController/ToolDispatcher), GUI decomposition (ChatPanel/InputPanel/SidePanel/ContextRenderer), classification pipeline (with phi4-mini, response_format fix, system-msg exclusion fix), tool pipeline, hierarchical task execution, working memory section, data model schemas, tool schema examples, threading model, persistence layout, configuration reference, and retrieval keywords index.
- `gui_manager.md` — replaced aspirational design doc with current-state documentation of GUIManager as a thin coordinator, describing the IGUIManager Protocol, panel decomposition, WidgetRegistry pattern, and achieved separation-of-concerns table.
- `docs/integration/01_ARCHITECTURE_OVERVIEW.md` — updated all stale module file paths (e.g. `src/agentx/gui_manager.py` → `src/agentx/gui/gui_manager.py`; `src/agentx/context.py` → `src/shared/models/context.py`). Updated component tables for both AgentX and Agentix subsystems to match current code structure. Updated data flow diagrams.

---

## [0.18.22] - 2026-04-18

### Code Changes

#### Fixed

- `src/agentix/query_payload.py` — emit `response_format: {"type": "json_object"}` (OpenAI-compat key) instead of Ollama-native `format: "json"` so classification calls receive structured JSON from all endpoints.
- `src/agentix/bridge/classify_prompt.py` — filter context to `user`/`assistant` roles only before classification (exclude `system` messages carrying working-memory identity to prevent persona contamination).
- `src/agentix/bridge/classify_prompt.py` — pass `system_prompts_dir` from `AgentixConfig` to `PromptLoader` so the classification system prompt is loaded correctly when invoked from the bridge.
- `agentx.toml` — set `agentix_bench_classification_model = "phi4-mini:3.8b"` (neutral model suitable for JSON classification; replaces agent-persona model).
- `system_prompts/prompt_classification.md` — updated to produce valid JSON output conforming to `PromptClassificationResponse` schema.

### Test Changes

#### Added

- `tests/integration/test_bootstrap_e2e.py` — 10 `@pytest.mark.live` end-to-end bootstrap tests verifying the full classification pipeline against a live Ollama instance.
