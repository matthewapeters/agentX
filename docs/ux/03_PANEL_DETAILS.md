# AgentX — Panel Details

Version: 2026-04-19 (updated 2026-04-19 — conversation-turn widget hierarchy documented)

Detailed affordance specifications for each GUI panel/widget.  Each section
documents the widget's purpose, all user-visible controls, and the callback
wiring to session logic.

Each section should follow the component cut-sheet standard in
[04_COMPONENT_CUT_SHEET_TEMPLATE.md](04_COMPONENT_CUT_SHEET_TEMPLATE.md).

---

## PD-01: ChatPanel

**Class**: `ChatPanel` (`src/agentx/gui/chat_panel.py`)  
**Position**: Left ~66% of window (PanedWindow left pane), height rely 0.00–0.77

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
| `_current_turn_frame` | `tk.Frame` | Outer container for the active turn (owns user entry + children frame) |
| `_current_turn_entries` | `dict[str, dict]` | Active streaming entry refs by role |
| `_current_turn_children_frame` | `tk.Frame` | Container for current turn's child widgets |
| `_plan_trees` | `dict[str, PlanTreeWidget]` | plan_id → tree widget |
| `_task_to_plan` | `dict[str, str]` | task_id → plan_id mapping |
| `_agent_thinking_started` | `bool` | True after first thinking chunk |
| `_agent_response_started` | `bool` | True after first response chunk |
| `_agent_classification_shown` | `bool` | True after classification shown |
| `_output_wrapped_labels` | `list` | Labels that need wraplength updates on resize |

### Conversation-Turn Widget Hierarchy

Each user submission creates a **turn frame** that owns exactly two direct children,
packed in this order (top → bottom):

```
turn_frame (tk.Frame, parent: output_entries_frame)
  ├── user_entry_frame  ← packed FIRST  (👤 user message + collapse toggle)
  └── children_frame    ← packed SECOND (22 px left-indent)
        ├── classification_entry_frame  (🤔, collapsed)
        ├── thinking_entry_frame        (💭, collapsed)
        ├── tool_call_entry_frame       (🔧, collapsed)   — if tool used
        └── assistant_entry_frame       (🤖, expanded)
```

**Critical invariant**: `children_frame` must be packed into `turn_frame` **after**
`user_entry_frame`.  Tkinter's `pack` geometry manager renders slaves in the order they
were packed; packing `children_frame` first would cause all response widgets to appear
*above* the user prompt.

> **Bug history (fixed 2026-04-19):** `_ensure_turn_started()` previously called
> `children.pack(...)` before `_create_output_entry()`, which packed the user entry
> frame.  This reversed the visual order on first render.  Collapsing then expanding the
> user entry accidentally "fixed" the order because `pack_forget()` + `pack()` appends
> the frame to the end of the slave list.  The fix was to defer `children.pack()` until
> after the user entry frame has been packed.

### Collapse / Expand Behaviour

When the user clicks the `▶/▼` toggle on the user entry:

| Action | Effect on `children_frame` |
|--------|--------------------------|
| Collapse (▼ → ▶) | `children_frame.pack_forget()` — hidden |
| Expand (▶ → ▼) | `children_frame.pack(...)` — re-appended after user entry |

Because the user entry was packed first, `children_frame` always re-appears **below**
the user entry after re-packing, regardless of how many collapse/expand cycles occur.

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
**Position**: Right ~34% of window (PanedWindow right pane), height rely 0.00–0.77

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

`FileExplorer` widget — full detail: [PD-11](#pd-11-fileexplorer).

### Settings Tab

`SettingsTab` widget — full detail: [PD-07](#pd-07-settingstab-detail).

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
**Position**: Third tab of SidePanel notebook (`⚙️ Settings`)
**Purpose**: Interactive `agentx.toml` editor. All changes are persisted to disk immediately on interaction. Settings marked 🔁 require a full app restart; a tooltip is shown on modification.

### Widget Conventions

| Value type | Widget | Notes |
|------------|--------|-------|
| `bool` | `tk.Checkbutton` | Fires immediately on toggle |
| `int` | `ttk.Spinbox` | Fires on value change |
| `str` (enum) | `ttk.Combobox` (fixed choices) | Fires on selection |
| `str` (model name) | `ttk.Combobox` (populated at runtime) | Refreshed via `populate_models()` |
| `str` (free text) | `ttk.Entry` | Fires on focus-out or Enter |
| `list[str]` (flags) | One `tk.Checkbutton` per known value | Fires on each toggle |

### Sections

#### 🎨 Appearance (expanded by default)

| Setting key | Label | Widget | Restart? |
|-------------|-------|--------|----------|
| `agentx.theme_mode` | Theme mode | Combobox: `Dark Mode` / `Light Mode` | Yes 🔁 |
| `agentx.markdown_render_enabled` | Render Markdown | Checkbutton (greyed if `tkinterweb` not installed) | No |

#### 🤖 Ollama (expanded by default)

| Setting key | Label | Widget | Restart? |
|-------------|-------|--------|----------|
| `agentx.ollama_host` | Host | Entry | Yes 🔁 |
| `agentx.ollama_model` | Default model | Combobox (from `/api/tags`) | Yes 🔁 |
| `agentx.ollama_initial_load_timeout_seconds` | Load timeout (s) | Spinbox 5–600 | Yes 🔁 |
| `agentx.screen_side` | Screen side | Combobox: `left` / `right` | Yes 🔁 |

#### 🧠 Agentix (expanded by default)

| Setting key | Label | Widget | Restart? |
|-------------|-------|--------|----------|
| `agentix.host` | Host | Entry | Yes 🔁 |
| `agentix.classify_prompts` | Classify prompts | Checkbutton | No |
| `agentix.debug` | Debug logging | Checkbutton | No |
| `agentix.classification_backend` | Backend | Combobox: `ollama` / `torch` | No |
| `agentix.agentix_bench_classification_model` | Classification model | Combobox (from `/api/tags`) | No (hot-reload) |
| `agentix.classification_torch_model` | Torch model | Entry (greyed unless backend=torch) | Yes 🔁 |
| `agentix.classification_torch_device` | Torch device | Spinbox −1–16 (greyed unless backend=torch) | Yes 🔁 |
| `agentix.default_system_prompts` | System prompts | One Checkbutton per discovered `.md` file | No |

#### 📊 Classification Display (collapsed by default)

| Setting key | Label | Widget |
|-------------|-------|--------|
| `agentix.classification_display.enabled` | Show classification block | Checkbutton |
| `agentix.classification_display.show_intent` | Show intent | Checkbutton |
| `agentix.classification_display.show_reasoning` | Show reasoning | Checkbutton |
| `agentix.classification_display.show_clarification` | Show clarification info | Checkbutton |
| `agentix.classification_display.show_next_step` | Show routing path | Checkbutton |

#### 🏛️ Working Memory (collapsed by default)

| Setting key | Label | Widget | Restart? |
|-------------|-------|--------|----------|
| `agentx.working_memory.enabled` | Enabled | Checkbutton | Yes 🔁 |
| `agentx.working_memory.inject_into_context` | Inject into LLM context | Checkbutton | No |
| `agentx.working_memory.max_facts` | Max facts (0 = unlimited) | Spinbox 0–500 | No |

### ASCII Mockup

```
⚙️ Settings tab (scrollable)
│
├── ▼ 🎨 Appearance
│     Theme mode:          [ Dark Mode    ▾]  🔁
│     [✓] Render Markdown
│
├── ▼ 🤖 Ollama
│     Host:                [ localhost:11434  ]  🔁
│     Default model:       [ phi4-mini:3.8b ▾]  🔁
│     Load timeout (s):    [  120  ↑↓ ]  🔁
│     Screen side:         [ right ▾]  🔁
│
├── ▼ 🧠 Agentix
│     Host:                [ localhost:8000   ]  🔁
│     [✓] Classify prompts
│     [ ] Debug logging
│     ── Classification ──────────────────────
│     Backend:             [ ollama ▾]
│     Classification model:[ phi4-mini:3.8b ▾]
│     Torch model:         [ (greyed)         ]  🔁
│     Torch device:        [ (greyed)  ↑↓ ]  🔁
│     ── System prompts ─────────────────────
│     [✓] planner_prompt
│     [✓] python_coder
│     [✓] tool_use
│
├── ▶ 📊 Classification Display   (collapsed)
│
└── ▶ 🏛️ Working Memory          (collapsed)
```

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

### Placement Diagram (Context)

```text
MainWindow
  └── SidePanel (PD-03)
       └── Session tab
            ├── CollapsibleSection("Working Memory")   [PD-09]
            └── CollapsibleSection("Context")          [PD-09]

MainWindow
  └── SidePanel (PD-03)
       └── Settings tab
            └── SettingsTab (PD-07)
                 └── CollapsibleSection(<settings group>) [PD-09]
```

### Internal Structure Diagram (Labeled Sub-Components)

```text
CollapsibleSection
  ├── frame
  │    ├── header
  │    │    ├── toggle_button
  │    │    └── title_label
  │    └── content_container
  │         └── _content_widget (optional, replaced by set_content)
```

### Behaviour Inventory

| Affordance ID | Sub-component | Trigger | Expected behaviour | Edge cases |
|---------------|---------------|---------|--------------------|------------|
| PD-09-AF-001 | `content_container` | Constructor with `initial_collapsed=True` | Starts collapsed (container not packed) | Empty content is allowed |
| PD-09-AF-002 | `content_container` | Constructor with `initial_collapsed=False` | Starts expanded (container packed) | No content set yet |
| PD-09-AF-003 | `toggle_button` | User click / `toggle()` | Flips expanded state and icon (`▶/▼`), packs or forgets container | Repeated toggles remain stable |
| PD-09-AF-004 | `_content_widget` | `set_content(widget)` | Replaces previous content widget, destroys old one | First assignment has no prior widget |

### Gherkin Use-Cases (Complete)

#### Scenario: Starts collapsed when requested `[PD-09-AF-001]`

GIVEN a `CollapsibleSection` created with `initial_collapsed=True`
WHEN the section is instantiated
THEN `is_expanded()` is `False` and `content_container` has no pack manager.

#### Scenario: Starts expanded when requested `[PD-09-AF-002]`

GIVEN a `CollapsibleSection` created with `initial_collapsed=False`
WHEN the section is instantiated
THEN `is_expanded()` is `True` and `content_container` is packed.

#### Scenario: Toggle changes visibility and state `[PD-09-AF-003]`

GIVEN a collapsed `CollapsibleSection`
WHEN `toggle()` is called
THEN the section becomes expanded and `content_container` becomes packed.

GIVEN the same section now expanded
WHEN `toggle()` is called again
THEN the section becomes collapsed and `content_container` is hidden.

#### Scenario: set_content replaces previous widget `[PD-09-AF-004]`

GIVEN a `CollapsibleSection` with an existing content widget
WHEN `set_content()` is called with a new widget
THEN the previous widget is destroyed and only the new widget remains.

### Test Mapping

| Affordance ID | Test file | Test class | Test function | Status |
|---------------|-----------|------------|---------------|--------|
| PD-09-AF-001 | `tests/test_collapsible_section.py` | Module-level pytest tests | `test_initial_collapsed_state_hides_content_container` | Passing |
| PD-09-AF-002 | `tests/test_collapsible_section.py` | Module-level pytest tests | `test_initial_expanded_state_shows_content_container` | Passing |
| PD-09-AF-003 | `tests/test_collapsible_section.py` | Module-level pytest tests | `test_toggle_flips_state_and_visibility` | Passing |
| PD-09-AF-004 | `tests/test_collapsible_section.py` | Module-level pytest tests | `test_set_content_replaces_previous_widget` | Passing |

### Code and Configuration References

- Source implementation:
  - `src/agentx/gui/collapsible_section.py:CollapsibleSection.__init__`
  - `src/agentx/gui/collapsible_section.py:CollapsibleSection.toggle`
  - `src/agentx/gui/collapsible_section.py:CollapsibleSection.set_content`
- Configuration keys consumed:
  - None directly (style args are passed from parent widgets)
- Runtime lookups / external dependencies:
  - None (pure Tkinter widget behavior)
- Data/state dependencies:
  - `expanded`, `_content_widget`, `content_container`, `toggle_button`

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

---

## PD-11: FileExplorer

**Class**: `FileExplorer` (`src/agentx/file_explorer.py`)
**Position**: Second tab (`Files`) of SidePanel notebook
**Purpose**: Browse the local filesystem, attach files to the current message, open files for editing, and pin folder paths to Working Memory.

### Layout

```
Files tab
│
├── Navigation bar (top strip)
│     [ ◀ Back ]  [ Forward ▶ ]  [ ⬆ Up ]  [ 🏠 Home ]  [ 🔄 Refresh ]
│     📁 /Projects/agentX/src/agentx
│
└── File listing (fills remaining height)
      ┌─────────────────────────────────────┬──────────┬──────────┐
      │ Name                                │ Type     │ Size     │
      ├─────────────────────────────────────┼──────────┼──────────┤
      │ 📁 gui/                             │ dir      │          │
      │ 📁 bridge/                          │ dir      │          │
      │ 📄 session.py                       │ .py      │ 48.2 KB  │
      │ 📄 file_explorer.py                 │ .py      │ 12.1 KB  │
      └─────────────────────────────────────┴──────────┴──────────┘
```

### Navigation Controls

| Control | Action | Callback |
|---------|--------|---------|
| `◀ Back` | Navigate to previous directory in history | `navigate_back()` |
| `Forward ▶` | Navigate to next directory in history | `navigate_forward()` |
| `⬆ Up` | Navigate to parent directory | `navigate_parent()` |
| `🏠 Home` | Navigate to user home directory | `navigate_home()` |
| `🔄 Refresh` | Reload current directory listing | `_populate_tree()` |

- `◀ Back` and `Forward ▶` are greyed when at the start/end of the navigation history.
- The path label below the buttons always shows the full absolute path of the current directory.

### Tree Columns

| Column | Width | Content |
|--------|-------|---------|
| Name | 250 px | File/folder name with icon |
| Type | 80 px | Extension (e.g. `.py`) or `dir` |
| Size | 100 px | File size in human-readable form; blank for directories |

### Interactions

| Interaction | Target | Action |
|-------------|--------|--------|
| Double-click | Directory row | Enter that directory (`change_directory()`) |
| Double-click | File row | Opens file for editing (`on_edit` callback) |
| Right-click (or Ctrl+click) | File row | Shows file context menu |
| Right-click (or Ctrl+click) | Directory row | Shows folder context menu |
| `Escape` / focus lost | Any | Dismisses open context menu |

### File Context Menu (right-click on a file)

| Item | Action | Callback |
|------|--------|---------|
| Attach | Add file as attachment chip in InputPanel | `on_attach(path)` |
| Edit | Open file content for editing/viewing | `on_edit(path)` |

### Folder Context Menu (right-click on a directory)

| Item | Action | Callback |
|------|--------|---------|
| Add full path to memory | Saves `folder_name → /abs/path` as a Working Memory fact | `on_add_folder_to_memory(key, full_path)` |
| Add relative path to memory | Saves `folder_name → relative/path` as a Working Memory fact | `on_add_folder_to_memory(key, rel_path)` |

### State

| Attribute | Type | Description |
|-----------|------|-------------|
| `current_path` | `str` | Absolute path currently displayed |
| `history` | `list[str]` | Navigation history stack |
| `history_index` | `int` | Current position in history stack |

### Related User Flow

See [UF-05: File Attachment](02_USER_FLOWS.md#uf-05-file-attachment) for the end-to-end flow from clicking a file to it appearing as an attachment chip.
See [UF-11: File Explorer Navigation](02_USER_FLOWS.md#uf-11-file-explorer-navigation) for directory browsing and folder-to-memory flows.
