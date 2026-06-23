# AgentX Documentation Consistency Audit Report

**Date**: 2026-06-23  
**Audit Scope**: All documents in `./docs/` for consistency with branch direction.

- Baseline expectation: documentation should avoid stale implementation references and avoid broken navigation paths.

**Status at audit time (historical snapshot)**: ⚠️ Significant inconsistencies were identified; findings below capture the 2026-06-23 state.

## Current Disposition

This report is a **historical baseline artifact** and is not the current gate decision document.

For current gate decisions, use the triad remediation brief at:
`.subutai/runs/2026-06-23-doc-review-triad/documentation-improvement-brief.md`.

Any architecture decisions, including Agentix references, must align to the **active branch architecture contract** as documented in `AGENTS.md` and related branch-contract sources.

---

## Executive Summary

At audit time, the documentation set contained **pervasive references** to three implementation-specific detail categories that were flagged for review:

1. **TMUX references** (11+ files) — Should be abstracted to generic multiplexer/presentation concepts
2. **Python implementation details** (9+ files) — Should describe behavior, not code structure
3. **Agentix middleware references** (2+ files) — Should be aligned with the active architecture contract
4. **Conflicting UX behaviors** — Implementation-specific concepts presented as architecture-neutral

**Recommended approach**:

- Separate *logical architecture* (what the system does) from *presentation implementation* (how users interact with it)
- Use generic terms: "pane", "window", "surface", "panel" instead of TMUX-specific concepts
- Use generic state concepts instead of Python class names
- Remove or refactor startup modes to focus on user presentation intent, not multiplexer-specific details

---

## Category 1: TMUX References (11 files)

These files reference TMUX explicitly and should be abstracted to generic multiplexer/presentation concepts.

### 🔴 **CRITICAL** — Full documents about TMUX

#### 1. `back-end migration guide (historical reference)` ⚠️

**Issue**: Entire document is a TMUX↔Zellij migration guide.  
**Current content**: "Switch backends: tmux vs zellij", keybindings, daemon config  
**Recommendation**: **DELETE** — TMUX/Zellij backend selection is implementation detail.  
**Alternative**: If multiplexer selection is a user choice, document only the user-facing effect ("Choose your preferred terminal multiplexer") without technical switching steps.

#### 2. `docs/troubleshooting.md` ⚠️

**Issue**: Entire document focused on multiplexer backend troubleshooting.  
**Lines**: Sections about "Zellij Installation & Setup", "Session Creation Issues", zellij daemon, layout files  
**Recommendation**: **REFACTOR** — Remove backend-specific troubleshooting. Keep only user-facing symptoms (e.g., "Session won't start").

---

### 🟠 **HIGH** — Architecture docs with heavy TMUX terminology

#### 3. `docs/architecture/startup_modes.md`

**Issue**: Startup modes table and switches are TMUX-centric.  
**Examples**:

- Line 1: "Single source of truth for startup modes and startup switches in AgentX"
- Lines 8-16: Table entries reference `agentx --startup-mode visible-windows`, `--user`, tmux session paths
- Lines 32-38: Flags like `--session-id`, `--attach`, `--layout` are TMUX/multiplexer-specific
- Line 48: "Runtime startup now probes `tmux -V` and `tmuxp --version`"

**Recommendation**:

- Rename sections to focus on **presentation topology** (not TMUX terms):
  - "frame-based" → "composed" or "paned" (logical concept)
  - "visible-windows" → "windowed" (generic, agnostic)
- Abstract startup switches to presentation intent:
  - Instead of `--layout-file`, describe what layout means (pane arrangement)
  - Remove TMUX-specific flags like `--session-id`, `--user`, `--attach`
  - Or mark them clearly as "[multiplexer-specific]"
- Remove `tmux -V` probing reference

#### 4. `docs/architecture/runtime_split.md`

**Issues**:

- Line ~50: "Go Core owns tmux session, pane/window orchestration"
- Line ~52: "Named tmux windows"
- Line ~55: "tmuxp overlays may reshape the layout"
- Line ~72: "The proposed startup switch is `--startup-mode visible-windows`"
- Line ~78: "Each runtime applet still uses the shared Go base architecture"

**Recommendation**:

- Replace "tmux session" with "runtime session" or "orchestration session"
- Replace "tmux windows" with "logical surfaces" or "presentation surfaces"
- Remove "tmuxp overlays" — this is an implementation detail
- Move startup-mode discussion to a generic "Presentation Topology" section
- Add abstract pane/window naming contract (e.g., "Pane role: output, system, input, logs") without TMUX terminology

#### 5. `docs/architecture/agentx_tui_hybrid_architecture.md`

**Issue**: Entire diagram centered on "tmux Session TUI-First Layout".  
**Lines**:

- Diagram title: "tmux Session TUI-First Layout"
- All pane descriptions reference TMUX-isms
- Diagram container is labeled "TMUX" explicitly

**Recommendation**:

- Rename diagram title to "**Runtime Presentation Layout**" or "**User-Facing Session Layout**"
- Replace "tmux Session" with "Runtime Session"
- Rename container from "TMUX" to "Presentation Layer" or "User Interface Surfaces"
- Update all pane labels to be TMUX-agnostic (remove any TMUX-specific language)

#### 6. `docs/ux/06_TUI_MIRROR.md`

**Issues**:

- Line ~11: "neovim window inside the tmux environment"
- Line ~97: "tmux Naming Contract (Hybrid Core)"
- Line ~99: "authoritative source of truth for session and window identifiers"
- Section 1.4: References to TMUX-specific behavior throughout

**Recommendation**:

- Replace "tmux environment" with "multiplexer-agnostic session"
- Rename section to "**Runtime Pane Naming Contract**"
- Make pane-naming independent of TMUX (e.g., "Pane role: output, system, input, logs")
- Document contract as "logical" not "TMUX-specific"

#### 7. `hybrid remaining work (historical reference)`

**Issues**:

- Line ~20: "dedicated system applet windows are now created at startup so users can cycle through them with tmux window navigation (`C-b n`)"
- References to TMUX keybindings and window cycling

**Recommendation**:

- Replace "tmux window navigation (`C-b n`)" with generic "multiplexer window navigation"
- Document as "Users can cycle through logical surfaces using standard multiplexer keybindings"
- Remove TMUX-specific key sequences

#### 8. `docs/architecture/applets/output_applet.md`

**Issues**:

- Line ~17: "**Ownership Boundary**" table mentions "tmux pane geometry (splits, resize, window layout) | **Core**"
- Line ~18: "tmux/core concern and is outside the applet's responsibility"

**Recommendation**:

- Replace "tmux pane geometry" with "**Presentation geometry** (pane sizing, layout, window splits)"
- Replace "tmux/core concern" with "**Presentation/core concern**"

#### 9. `docs/ux/00_INDEX.md`

**Issues**:

- Line ~30: Link to "Vibe coding (neovim + tmux integration)"
- References to TMUX in context of developer workflows

**Recommendation**:

- Replace "neovim + tmux integration" with "Terminal editor integration"
- Make vibe-coding agnostic to specific multiplexer

#### 10. `docs/ux/UX_LIFECYCLE.md`

**Issues**:

- References to testing infrastructure that may be TMUX-specific

**Recommendation**:

- Review "Headless Tkinter Testing Primer" section — if it references TMUX, abstract it
- Remove references to specific test frameworks tied to TMUX

#### 11. `docs/architecture/channel_registry.md`

**Minor issue**: References "tmux" in code links section  
**Recommendation**: Verify code links don't expose TMUX assumptions

---

## Category 2: Python Implementation References (9 files)

These files expose Python code structure, class names, and file paths that should be abstracted to behavior descriptions.

### 🔴 **CRITICAL** — Heavy Python code references

#### 1. `architecture module doc (historical reference)`

**Issues** (Lines):

- Line ~30: "Tkinter GUI (main thread)"
- Line ~32-33: Lists Python class names directly: `SidePanel`, `ChatPanel`, `InputPanel`
- Line ~50: "AgentxSession (main thread coordinator)"
- Line ~51-56: Lists Python internals: `SessionState`, `StreamingController`, `ToolDispatcher`, `Context`, `WorkingMemory`, `AgentixBridgeAdapter`
- Line ~60-75: "AgentixBridge" details and references "Ollama" as external service
- Section "2. Startup Flow" references Python entry points: `main.py`, `src/agentx/main.py`
- Multiple references to Python file structure

**Recommendation**:

- Replace "Tkinter GUI" with "**User Interface** (desktop/TUI/hybrid)"
- Replace class names with logical concepts:
  - `SidePanel` → "Navigation panel"
  - `ChatPanel` → "Chat/output surface"
  - `AgentXSession` → "Session orchestrator"
  - `StreamingController` → "Event streaming coordinator"
  - `AgentixBridgeAdapter` → Align wording with the active architecture contract
- Remove Ollama as a "service dependency"; replace with "Language model backend"
- Describe startup flow using logical concepts, not Python entry points

#### 2. `docs/TUI_INTEGRATION_COMPLETION_PLAN.md`

**Issues** (Heavy Python file references):

- Line ~11: `cmd/agentx-core/main.go` and `cmd/agentx-core/demo_harness.go` (Go is OK, but...)
- Line ~13: `src/agentx/integration/tui_bridge.py` ← **Python file path**
- Line ~14: `src/agentx/integration/tui_event_subscriber.py` ← **Python file path**
- Line ~15: `src/agentx/gui/gui_manager.py` ← **Python file path**
- Line ~16: `src/agentx/igui_manager.py` ← **Python file path**
- Line ~18: `src/agentx/streaming_controller.py` ← **Python file path**
- Lines ~22-28: Multiple Python test file references

**Recommendation**:

- Remove all Python file path references
- Replace with logical architectural concepts (e.g., "Event bridge", "GUI manager", "Streaming coordinator")
- If code structure is necessary for developers, move to a separate IMPLEMENTATION_GUIDE, not architecture docs
- Keep focus on **what the system does**, not **where it's written**

#### 3. `docs/event_broker_pubsub.md`

**Issues**:

- Line ~11: "EventBroker (central pub-sub hub)" — Python class name
- Line ~14-15: "Publishes events to all subscribers" — implementation detail
- Line ~20: Code example showing Python signatures (`def subscribe(...)`, `def publish(...)`)
- Line ~40-50: Architecture diagram referencing "EventBroker", "StreamingController", "GUI display calls", "TUI Event Sub." (Python naming)
- Line ~70+: "EventType enum and broker: src/agentx/event_broker.py" — **Python file path**

**Recommendation**:

- Replace "EventBroker" with "**Event Coordination Layer**"
- Replace "StreamingController" with "**Response streaming coordinator**"
- Replace "TUIEventSubscriber" with "**TUI event consumer**"
- Remove Python code signatures; describe contract in logical terms (e.g., "Event subscribers register callbacks for event types")
- Remove file path references or move to a separate implementation guide

#### 4. `docs/architecture/channel_registry.md`

**Issues**:

- Line ~1: References "EventType Enum" — Python implementation
- Line ~90+: Code links to Python files:
  - `src/agentx/event_broker.py`
  - `src/agentx/integration/tui_event_subscriber.py`
  - `src/agentx/streaming_controller.py`

**Recommendation**:

- Remove "EventType Enum" terminology; replace with "Event Types" or "Channel Types"
- Remove or drastically reduce code links to Python files
- Keep only Go code paths (since branch is Go-focused)

#### 5. `docs/markdown_rendering_plan.md`

**Issues**:

- Line ~29: "Tkinter"
- Line ~31: "tk.Text widgets"
- Line ~37: "HtmlFrame (tkinterweb renders...)"
- Line ~49: "Technology Stack" table references "tkinterweb", "markdown" (Python packages)
- Line ~56: `src/agentx/gui/markdown_renderer.py` — **Python file path**

**Recommendation**:

- Replace all Tkinter/GUI-specific terminology with generic "UI rendering"
- This document should be **DEPRECATED** if it's about Python GUI rendering
- If markdown rendering is still needed, document it as a feature, not a Python implementation detail

#### 6. `tool usage plan (historical reference)`

**Issue**: Entire document is about "agentix middleware layer"  

- Line ~3: "agentix middleware layer" repeated throughout
- References Python functions and classes: `extract_tool_schema()`, `ToolDefinition.from_callable()`, `ServerToolExecutor`, `lmstudio-python` SDK
- Line ~50: `src/agentix/tools/schema.py` — **Python file path**
- References agentix-specific stubs and implementation details

**Recommendation**:

- **DELETE or ARCHIVE** — If no longer part of the active architecture contract
- If tool execution is still needed, document the desired **user-facing behavior**, not the Python middleware

#### 7. `docs/AGENT_README.md`

**Issues**:

- Line ~5: "Use the Senior Application Architect and Senior Document Witer experts"
- Line ~9: References "Tkinter GUI" as a feature
- Line ~16-18: References GHERKIN use-cases and Python testing patterns
- Line ~21-24: "Apply quality gates" section references Python-specific concepts

**Recommendation**:

- Remove AGENT_README entirely or refactor to focus on **what AgentX does**, not development process
- Or move to a separate DEVELOPMENT.md file not in ./docs

#### 8. `docs/ux/UX_LIFECYCLE.md`

**Issues**:

- Line ~32: "Headless Tkinter Testing Primer" — **Python GUI testing reference**
- Line ~120+: References Python-specific testing patterns
- Lines ~140-160: Traceability matrix references Python class names (e.g., "test_chat_panel_turn_rendering.py")

**Recommendation**:

- Rename "Headless Tkinter Testing Primer" to generic "**UI Testing Primer**"
- Remove test file names or make them agnostic (e.g., "Chat output rendering tests")
- Reference test coverage by feature, not by Python file

#### 9. `docs/ux/00_INDEX.md`

**Issues**:

- Line ~40+: Status table references implementation-specific component names:
  - "PD-01 ChatPanel"
  - "PD-02 InputPanel"
  - "PD-03 SidePanel"
  - "test_chat_panel_turn_rendering.py" (Python test file)
  - "test_file_explorer_coverage.py" (Python test file)

**Recommendation**:

- Replace component names with feature descriptions:
  - "ChatPanel" → "Chat/Output feature"
  - "InputPanel" → "User input feature"
  - "SidePanel" → "Context/Navigation panel"
- Replace test file references with test categories or feature names
- Keep component IDs (PD-01, etc.) if they have meaning, but document them separately

---

## Category 3: Agentix References (2+ files)

These files reference Agentix middleware and should be reconciled with the active architecture contract.

### 🔴 **CRITICAL**

#### 1. `tool usage plan (historical reference)`

**Status**: Already marked for removal in Category 2  
**Action**: Reconcile with the active branch architecture contract; archive or refactor if out of scope

#### 2. `architecture module doc (historical reference)`

**Issue**: Line ~60: "Agentix (optional)" listed as external service  
**Recommendation**: Remove entirely; replace with "Tool execution layer" if needed

---

## Category 4: Conflicting UX Behaviors (Multiple files)

These documents present conflicting or ambiguous UX concepts.

### 🟠 **HIGH**

#### 1. **Implementation-Agnostic UX vs. Implementation Details**

**Files affected**: `docs/ux/UX_LIFECYCLE.md`, `docs/ux/00_INDEX.md`, `docs/ux/03_PANEL_DETAILS.md`

**Issue**:

- UX_LIFECYCLE.md states (line ~25): "UX requirements in this document are implementation-agnostic. Delivery technology (GUI, TUI, hybrid) is an implementation concern and must conform to UX requirements."
- BUT: Same document includes a "Owner (Go/Py)" column (line ~35-50) which directly contradicts implementation-agnostic principle
- Status table shows "test_*.py" file references, mixing spec with implementation details

**Conflict Example**:

```
| Chat (Output) | Output Pane (chat) | Go (current) | test_chat_panel_turn_rendering.py | ✅ Go-owned, tested |
```

This row conflates:

- UX requirement (Chat output)
- Implementation surface (Output Pane)
- Implementation language (Go)
- Implementation test file (Python)

**Recommendation**:

- Keep UX spec separate from implementation metadata
- If tracking implementation ownership, create a separate "Implementation Status" table
- Remove test file references from UX spec
- Replace "Owner (Go/Py)" with hidden implementation notes

#### 2. **Startup Modes: Logical vs. Multiplexer-Specific**

**Files affected**: `docs/architecture/startup_modes.md`, `docs/architecture/runtime_split.md`, `docs/hybrid remaining work (historical reference)`

**Issue**:

- "frame-based runtime" is TMUX-centric (frames don't exist in all multiplexers)
- "visible-windows" is TMUX-centric (zellij doesn't use the same concept)
- Fallback logic ("If visible-windows mode cannot be established, fall back...") assumes TMUX

**Conflict**:

- These are presented as logical presentation modes
- But they're actually multiplexer-specific implementations
- Branch claims "No reference to TMUX"

**Recommendation**:

- Define logical presentation modes:
  - "Default" → "Composed pane layout" (multiple surfaces in single window)
  - "Windowed" → "Separated surface windows" (each surface in a separate window)
- Implementation can use TMUX/Zellij specific techniques without documenting them in arch docs
- Move startup-modes.md to implementation guide or remove

#### 3. **Activity State Contract: Implementation Leak**

**Files affected**: `docs/architecture/channel_registry.md`

**Issue** (lines ~96-98):

```
- Primary transport (current): HTTP `GET /activity` on core health endpoint.
- Secondary mirror: `/context.prompt_cycle` must remain semantically consistent with `/activity.prompt_cycle`.
```

**Conflict**:

- This is an implementation detail (HTTP endpoint, file path)
- Should only document the **state contract** (what information is available), not transport/file paths

**Recommendation**:

- Replace with: "Activity state must be accessible to all applets in a query-able form (implementation-agnostic)"
- Remove HTTP endpoint and file path references
- Move technical binding details to implementation guide

#### 4. **GUI/TUI Parity Claims vs. Actual Status**

**Files affected**: Multiple (docs/architecture/runtime_split.md, docs/architecture/startup_modes.md, docs/ux/UX_LIFECYCLE.md)

**Issue**:

- Documents claim "GUI and TUI are peers" with parity matrices
- BUT: docs/architecture/runtime_split.md says GUI remains on a secondary rollout track
- AND: docs/architecture/startup_modes.md and related architecture guidance still include implementation-first framing in places

**Conflict Examples**:

- runtime and UX docs can still present mixed signals about parity versus staged rollout priority
- But text says "Keep GUI on secondary track"
- This sends conflicting signals about priority

**Recommendation**:

- Choose: Is GUI equal to TUI or secondary?
- If secondary, remove from parity tables or clearly mark as "not in current scope"
- If equal, update status to reflect equal priority

#### 5. **Presentation vs. Pane Terminology Confusion**

**Files affected**: Multiple architecture docs

**Issue**:

- "Pane" is used inconsistently:
  - Sometimes means TMUX pane (implementation)
  - Sometimes means logical surface (architecture)
  - Sometimes means Go widget (runtime)

**Conflict Example** (output_applet.md):

```
Core owns conversation data and tmux pane geometry; the applet owns pane-local view state
```

- Here "pane" is explicitly TMUX
- But should be generic "surface" or "window"

**Recommendation**:

- Define terms clearly:
  - "Surface" = logical user-facing area
  - "Pane" = only if multiplexer-specific, otherwise avoid
  - "Widget" = runtime implementation (Go native widget)
- Use consistently across all docs

---

## Category 5: Ambiguities (Clarification Needed at Audit Time)

These are unclear points that need explicit resolution:

### 1. **What is the primary user interface?**

- Documents mention "GUI", "TUI", "neovim + tmux integration", "hybrid"
- Is there a canonical user experience, or are all equally supported?
- **Clarification needed**: Explicit statement of primary UX intent

### 2. **What is "parity"?**

- Parity matrices claim feature equivalence between GUI and TUI
- But status says "TUI is primary, GUI is secondary"
- **Clarification needed**: Does parity mean feature-equivalent or behavioral-equivalent?

### 3. **Are multiplexer backends a user choice or implementation detail?**

- Documents discuss "tmux vs zellij"
- But branch direction suggests this is internal only
- **Clarification needed**: Should users need to know about backend selection?

### 4. **What is "demo mode"?**

- Multiple files reference demo mode with unclear purpose
- Is it a testing harness, a user feature, or an implementation artifact?
- **Clarification needed**: Explicit definition of demo mode's scope and audience

### 5. **Activity state visibility**

- channel_registry.md describes activity state as having multiple transports and mirrors
- This seems over-engineered for an internal concept
- **Clarification needed**: Is activity state a user-facing feature or internal plumbing?

---

## Historical Remediation Tracking State (At Audit Time)

### Phase 1: Remove/Delete (Highest Priority)

- [ ] historical back-end migration guide doc — DELETE entire file
- [ ] historical tool usage plan doc — evaluate against active branch architecture contract; archive or refactor if out of scope
- [ ] `docs/AGENT_README.md` — DELETE or move to DEVELOPMENT.md (not in docs/)

### Phase 2: Refactor Architecture Files

- [ ] historical architecture module doc — Remove Python classes, Tkinter, file paths; describe logically
- [ ] `docs/architecture/startup_modes.md` — Abstract to presentation topology, remove TMUX terms
- [ ] `docs/architecture/runtime_split.md` — Replace TMUX/pane terminology with generic concepts
- [ ] `docs/architecture/channel_registry.md` — Remove Python file links, generalize EventType naming
- [ ] `docs/architecture/agentx_tui_hybrid_architecture.md` — Rename diagram, abstract presentation layer
- [ ] `docs/event_broker_pubsub.md` — Remove Python class references, generalize architecture

### Phase 3: Refactor UX Documents

- [ ] `docs/ux/UX_LIFECYCLE.md` — Remove test file references, separate spec from implementation metadata
- [ ] `docs/ux/00_INDEX.md` — Replace component names with feature descriptions; remove test file names
- [ ] `docs/ux/06_TUI_MIRROR.md` — Replace TMUX terminology with generic multiplexer concepts
- [ ] `docs/ux/03_PANEL_DETAILS.md` — Verify no Python implementation leakage (review separately)

### Phase 4: Fix Remaining References

- [ ] `docs/troubleshooting.md` — Remove backend-specific troubleshooting
- [ ] historical hybrid remaining-work doc — Remove TMUX keybinding references (C-b n)
- [ ] `docs/architecture/applets/output_applet.md` — Replace "tmux pane geometry" with generic "surface geometry"
- [ ] `docs/architecture/applets/*.md` — Audit all applet docs for TMUX/Python leakage
- [ ] `docs/markdown_rendering_plan.md` — Archive or DELETE (Python GUI rendering detail)

### Phase 5: Clarification & Alignment

- [ ] Resolve ambiguities from Category 5 above
- [ ] Create a **TERMINOLOGY.md** file defining key concepts (pane, surface, widget, applet, etc.)
- [ ] Create separate IMPLEMENTATION.md (not in docs/) for code structure and file paths
- [ ] Update CHANGELOG.md to document this consistency pass

---

## Summary Table

| File | Category | Severity | Action |
|------|----------|----------|--------|
| historical back-end migration guide doc | TMUX | 🔴 CRITICAL | DELETE |
| `docs/troubleshooting.md` | TMUX | 🟠 HIGH | REFACTOR |
| `docs/architecture/startup_modes.md` | TMUX | 🟠 HIGH | REFACTOR |
| `docs/architecture/runtime_split.md` | TMUX | 🟠 HIGH | REFACTOR |
| `docs/architecture/agentx_tui_hybrid_architecture.md` | TMUX | 🟠 HIGH | REFACTOR |
| `docs/ux/06_TUI_MIRROR.md` | TMUX | 🟠 HIGH | REFACTOR |
| historical hybrid remaining-work doc | TMUX | 🟠 HIGH | REFACTOR |
| `docs/architecture/applets/output_applet.md` | TMUX | 🟠 HIGH | REFACTOR |
| `docs/ux/00_INDEX.md` | TMUX + Python | 🟠 HIGH | REFACTOR |
| `docs/ux/UX_LIFECYCLE.md` | Python | 🟠 HIGH | REFACTOR |
| historical architecture module doc | Python | 🔴 CRITICAL | REFACTOR |
| `docs/TUI_INTEGRATION_COMPLETION_PLAN.md` | Python | 🔴 CRITICAL | REFACTOR |
| `docs/event_broker_pubsub.md` | Python | 🟠 HIGH | REFACTOR |
| `docs/architecture/channel_registry.md` | Python | 🟠 HIGH | REFACTOR |
| `docs/markdown_rendering_plan.md` | Python | 🟠 HIGH | DELETE/ARCHIVE |
| historical tool usage plan doc | Agentix | 🔴 CRITICAL | ALIGN/ARCHIVE |
| `docs/AGENT_README.md` | Mixed | 🟠 HIGH | DELETE/MOVE |
| **UX Behavior Conflicts** | Multiple | 🟠 HIGH | CLARIFY & ALIGN |
| **Ambiguities** | Multiple | 🟡 MEDIUM | CLARIFY |

---

## Recommended Next Steps (Captured at Audit Time)

1. **Agree on terminology** — Define authoritative terms (pane, surface, widget, applet, etc.)
2. **Decide on scope** — Clarify if GUI is truly secondary or if it's equal
3. **Create TERMINOLOGY.md** — Single source of truth for all architectural terms
4. **Execute Phase 1 deletions** — Remove files that are completely out of scope
5. **Execute Phase 2-3 refactoring** — Using the remediation list above
6. **Review with specialists** — Have the application architect and senior doc writer validate alignment
7. **Update CHANGELOG.md** — Document consistency pass and breaking changes to documentation

---

## Open Clarification Questions (Captured at Audit Time)

Before the original remediation pass, the following clarification prompts were captured:

1. **What is the primary user interface?** (GUI, TUI, neovim, hybrid, or user-selectable?)
2. **Is GUI truly secondary?** (Can we remove from feature parity tables if so?)
3. **Multiplexer backend?** (Is this a user-facing choice or internal only?)
4. **What is "parity"?** (Feature-equivalent or behavioral-equivalent?)
5. **Activity state** (Is this user-facing or internal plumbing only?)
