# AgentX — Panel Details

Version: 2026-04-19

Detailed affordance specifications for each GUI panel/widget.  Each section
documents the widget's purpose, all user-visible controls, and the callback
wiring to session logic.

---

## PD-01: ChatPanel

**Class**: `ChatPanel` (`src/agentx/gui/chat_panel.py`)  
**Position**: Right ~75% of window, from top to rely=0.77  
**Purpose**: Displays the live conversation and plan-tree tabs.

### Tabs

| Tab | Created | Contents |
|-----|---------|----------|
| `Chat` | Always present | Streaming message entries (see message types below) |
| `Plan: <name>` | Added per plan | `PlanTreeWidget` for that plan |

### Message Entry Types (Chat tab)

| Entry Type | Trigger | Visual |
|------------|---------|--------|
| User message | `display_user_message()` | 👤 username + timestamp, message text, attachment chips |
| Classification | `display_classification()` | ⚙️ `intent → next_step` (greyed secondary line) |
| Thinking block | `display_agent_thinking()` | 💭 collapsible block (collapsed by default) |
| Assistant response | `display_agent_response()` | 🤖 "AgentX" header, streaming text |
| Tool call | `_display_tool_call()` | 🔧 tool_name + args, collapse/expand button |
| Tool result | `_display_tool_result()` | 📋 result text inside collapsed `CollapsibleSection` |
| Error | `display_error()` | Red-highlighted error text |

### Affordances

| Control | Action | Callback |
|---------|--------|---------|
| Tool call `▶` button | Expand/collapse tool result | In-widget toggle |
| Thinking block `▶` button | Expand/collapse reasoning | In-widget toggle |
| Plan tab click | Navigate to plan tree | Tkinter notebook selection |
| Scroll | Vertical scroll of chat history | Mouse wheel / scrollbar |

### State Fields

| Field | Type | Description |
|-------|------|-------------|
| `_current_turn_entries` | `dict[str, dict]` | Active streaming entry refs by role |
| `_current_turn_children_frame` | `tk.Frame` | Container for current turn's child widgets |
| `_plan_trees` | `dict[str, PlanTreeWidget]` | plan_id → tree widget |
| `_task_to_plan` | `dict[str, str]` | task_id → plan_id mapping |
| `_agent_thinking_started` | `bool` | True after first thinking chunk |
| `_agent_response_started` | `bool` | True after first response chunk |
| `_agent_classification_shown` | `bool` | True after classification shown |
| `_output_wrapped_labels` | `list` | Labels that need wraplength updates on resize |

---

## PD-02: InputPanel

**Class**: `InputPanel` (`src/agentx/gui/input_panel.py`)  
**Position**: rely=0.77 to rely=1.0 (bottom 23% of window)  
**Purpose**: Captures user text input and file attachments.

### Widgets

| Widget | rely / relheight | Description |
|--------|-----------------|-------------|
| Attachment bar | 0.77 / 0.03 | Chip list of current + history attachments |
| Text input | 0.80 / 0.18 | Multi-line `tk.Text` for user message |
| Send button | right side | Triggers `on_submit()` |
| Stop button | right side | Triggers `on_interrupt()` |

### Attachment Bar

- **Current attachments**: bright chip `📎 filename [×]` — click `[×]` to remove.
- **History attachments**: greyed chip (already in context, informational only).
- **Clear all**: `[✕]` button to remove all current-turn attachments.

### Keyboard Shortcuts

| Key | Behaviour |
|-----|-----------|
| `Enter` | Send message (same as Send button) |
| `Shift+Enter` | Insert newline in text area |

### Button State

| State | When |
|-------|------|
| `Send` enabled | Not streaming |
| `Send` disabled | Streaming in progress |
| `Stop` enabled | Streaming in progress |
| `Stop` disabled | Not streaming |

---

## PD-03: SidePanel

**Class**: `SidePanel` (`src/agentx/gui/side_panel.py`)  
**Position**: Left ~25% of window (PanedWindow left sash), full height 0–77%  
**Purpose**: System-status, model selection, session/file/settings management.

### Sub-widgets

| Widget | Position | Description |
|--------|----------|-------------|
| `ModelSelector` | Top of pane | Active model dropdown |
| `ttk.Notebook` | Below model selector | Three tabs: Session / Files / Settings |

### Session Tab

Contains two `CollapsibleSection` widgets:

**Working Memory section** (`🧠 Working Memory (N facts)`):

| Row | Contains |
|-----|---------|
| Per-fact row | Icon (👤/🤖), key, value, toggle (☑), delete (🗑), promote (↑) |
| Footer | `[+ Add fact…]` user-add entry |

| Control | Action | Callback |
|---------|--------|---------|
| Checkbox | Toggle fact enabled/disabled | `on_toggle(key, bool)` |
| 🗑 button | Delete fact | `on_delete(key)` |
| ↑ button | Promote agent fact to user-owned | `on_promote(key)` |
| `[+ Add fact…]` | Add new user-owned fact | `on_user_add(key, value)` |

**Context section** (`💬 Context (N messages)`):

| Row | Contains |
|-----|---------|
| Per-message row | Role icon, sender, timestamp, content preview, expand/collapse |
| Tool call sub-row | 🔧 tool_name (indented, collapsible) |
| Plan sub-row | 📋 plan_name (indented, collapsible) |

### Files Tab

`FileExplorer` widget — see `src/agentx/file_explorer.py`:

| Control | Action |
|---------|--------|
| Directory tree | Navigate folders |
| File click | Add file as attachment |
| `[↑]` up-dir button | Navigate to parent directory |
| `[⟳]` refresh button | Reload current directory |

### Settings Tab

`SettingsTab` widget — interactive `agentx.toml` editor:

| Widget type | Used for |
|-------------|---------|
| `tk.Checkbutton` | Boolean settings |
| `ttk.Spinbox` | Integer settings |
| `ttk.Combobox` | Enum or model-name settings |
| `ttk.Entry` | Free-text string settings |
| `tk.Checkbutton` per value | `list[str]` flag arrays |

- **🔁** label suffix: setting requires app restart to take effect.
- Torch-specific fields (`classification_torch_device`, `classification_torch_model`)
  are greyed unless `classification_backend == "torch"`.

---

## PD-04: ModelSelector

**Class**: `ModelSelector` (`src/agentx/gui/model_selector.py`)  
**Position**: Top of SidePanel  
**Purpose**: Switch the active Ollama model for subsequent prompts.

| Control | Action | Effect |
|---------|--------|--------|
| Dropdown combo | Select model | `on_model_change(model_name)` → updates `SessionState.active_model`, writes to `agentx.toml` |
| `[⟳]` refresh | Reload model list | Calls Ollama `/api/tags` endpoint to refresh available models |

---

## PD-05: PlanTreeWidget

**Class**: `PlanTreeWidget` (`src/agentx/gui/plan_tree_widget.py`)  
**Position**: Inside a plan tab in ChatPanel  
**Purpose**: Live collapsible tree of plan execution state.

### Tree Structure

```
● Plan: <name>                           [Re-synth] [Export]
  ○ Step 1: <description>               [Re-synth]
      🔧 read_file  /src/parser.py      [▶ collapse]
         📋 [file contents…]
      ✓ Synthesis: I read the file and found…
  ● Step 2: <description>               [Re-synth]
      🔧 write_file  /src/parser.py     [▶ collapse]
```

### Node Status Icons

| Icon | Status | Colour |
|------|--------|--------|
| ○ | `pending` | Grey |
| ● | `running` | Blue |
| ✓ | `done` | Green |
| ? | `needs_review` | Orange |
| ✗ | `failed` | Red |

### Controls

| Control | Location | Action |
|---------|----------|--------|
| `[Re-synth]` | Plan root and each step | Opens `ResynthesisDialog` |
| `[Export]` | Plan root | Writes `task_tree_export.md` and opens it |
| Tool call `▶` | Tool call row | Expand/collapse tool result inline |
| Canvas scroll | Tree panel | Vertical scroll through long plans |

---

## PD-06: ResynthesisDialog

**Class**: `ResynthesisDialog` (`src/agentx/gui/resynthesis_dialog.py`)  
**Type**: Modal dialog (blocks parent window)  
**Purpose**: Re-run synthesis for a specific task node, optionally with a WM hint.

```
┌─────────────────────────────────────────────────┐
│  Re-synthesise: Step 1                           │
│                                                  │
│  Optional working-memory hint:                   │
│  ┌────────────────────────────────────────────┐  │
│  │  focus on error handling patterns          │  │
│  └────────────────────────────────────────────┘  │
│                                                  │
│            [Cancel]  [Re-synthesise]              │
└─────────────────────────────────────────────────┘
```

| Control | Action |
|---------|--------|
| Hint text field | Optional free-text hint injected into re-synthesis prompt |
| `[Re-synthesise]` | Calls `session.retrigger_synthesis_streaming(task_id, hint)` |
| `[Cancel]` | Dismisses dialog, no action |

---

## PD-07: SettingsTab (Detail)

**Class**: `SettingsTab` (`src/agentx/gui/settings_tab.py`)  
**Position**: Third tab of SidePanel notebook  
**Purpose**: Interactive `agentx.toml` editor.

### Key Sections

| Section | Settings | Widget Type |
|---------|----------|-------------|
| `[agentx]` | `ollama_host`, `ollama_model`, `screen_side`, `theme_mode` | Entry / Combobox |
| `[agentx]` | `ollama_initial_load_timeout_seconds` | Spinbox |
| `[agentix]` | `host`, `classify_prompts`, `classification_backend` | Entry / Checkbox / Combobox |
| `[agentix]` | `agentix_bench_classification_model` | Combobox (populated from Ollama) |
| `[agentix]` | `classification_torch_device`, `classification_torch_model` | Entry (greyed if backend≠torch) |
| `working_memory` | `enabled` | Checkbox (restart required) |

Settings marked 🔁 require a full app restart to take effect; a tooltip is
shown when the user modifies them.

---

## PD-08: ContextRenderer

**Class**: `ContextRenderer` (`src/agentx/gui/context_renderer.py`)  
**Type**: Stateless widget factory (no persistent state)  
**Purpose**: Constructs the context/history/working-memory sub-widgets shown in the Session tab.

### Factory Methods

| Method | Output |
|--------|--------|
| `render_context_widget(context, parent, …)` | Scrollable grid of message rows for the current session |
| `render_history_widget(history, parent, …)` | Same, but for a historical session (read-only) |
| `render_working_memory_widget(wm, parent, …)` | Working Memory fact grid with control buttons |
| `collapse_expand_button(parent, expandable_frame)` | A `▶/▼` toggle button wired to show/hide a frame |
| `_render_message_to_grid(msg, parent, row, …)` | Renders a single message row into a grid |
| `_render_tool_rows(tool_msgs, …)` | Renders tool_call + tool_result row pairs |
| `_render_plan_rows(plan_msgs, task_msgs, …)` | Renders plan and task_node rows |

### Message Columns Layout

| Column | Index | Content |
|--------|-------|---------|
| `exp_button` | 0 | Collapse/expand toggle |
| `enabled` | 1 | Message-enabled checkbox |
| `role` | 2 | Role icon (👤🤖⚙️💭🔧📋) |
| `content` | 3 | Truncated message content |

---

## PD-09: CollapsibleSection

**Class**: `CollapsibleSection` (`src/agentx/gui/collapsible_section.py`)  
**Type**: Reusable container widget  
**Purpose**: Wraps any widget in an expand/collapse header.

```
▼ Section Title (N items)     ← click to collapse
  ┌────────────────────────┐
  │ … child widgets …      │
  └────────────────────────┘

▶ Section Title (N items)     ← click to expand
```

Used in:

- SidePanel Session tab: Working Memory section, Context section
- SettingsTab: each configuration group

---

## PD-10: ToolPanel

**Class**: `ToolPanel` (`src/agentx/gui/tool_panel.py`)  
**Position**: Inside SettingsTab  
**Purpose**: Enable/disable individual tools per session.

```
▼ Available Tools
  [✓] cst   Concrete Syntax Tree analysis
  [✓] ast   Abstract Syntax Tree analysis
  [ ] my_custom_tool   ...
```

| Control | Action | Callback |
|---------|--------|---------|
| Checkbox per tool | Toggle tool enabled | `on_tool_toggle(tool_name, enabled)` |
| `▼/▶` header | Expand/collapse panel | In-widget toggle |

Disabled tools are passed as `_disabled_tools` to `ToolLoopRunner` and excluded
from the `tools=[…]` array in the API request.
