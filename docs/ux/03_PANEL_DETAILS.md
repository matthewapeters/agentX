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
| Startup notice block | Show friendly log-file locations on startup | `AgentXSession._show_startup_log_locations_notice_if_enabled()` |

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

### Affordance: PD-01-AF-009 — Startup log-location notice in output window

**Source**: `AgentXSession._show_startup_log_locations_notice_if_enabled()` +
`ChatPanel.display_startup_notice()`
(`src/agentx/session.py`, `src/agentx/gui/chat_panel.py`)  
**Test**: `tests/test_startup_log_notice.py` · `TestStartupLogNotice`

```gherkin
GIVEN startup layout initialization with default configuration
WHEN AgentX initializes the main output window
THEN a friendly startup notice is displayed before any agent response
 AND the notice includes where session and runtime log files can be found

GIVEN `agentx.show_log_locations_on_startup` is set to false in `agentx.toml`
WHEN AgentX initializes the main output window
THEN the startup log-location notice is suppressed

GIVEN startup notice display is enabled
WHEN the notice is rendered in the output window
THEN it appears as informational/system content (not as an agent response)
```

### Affordance: PD-01-AF-010 — Right-click context menu on output panel (Copy)

**Source**: `ChatPanel._show_output_context_menu()` + `ChatPanel._bind_output_text_shortcuts()`  
(`src/agentx/gui/chat_panel.py`)  
**Test**: `tests/test_chat_panel_copy_context_menu.py` · `TestOutputPanelRightClickCopy`  
**Status**: 📝 (spec only — not yet implemented)

The output panel is read-only; the only clipboard action available is **Copy**.
The right-click popup uses the same Wayland-aware `tk.Toplevel(overrideredirect=True)`
approach proven by the `FileExplorer` context menu (see UX_ISSUES.md RC11–RC14 history).
A fresh `Toplevel` is created for every invocation; stale surfaces are destroyed before
each new popup is shown.  The "Copy" item is always visible; if no text is selected at
the moment of right-click, "Copy" copies nothing (same as Ctrl-C with no selection).

```
output_text  ──<Button-3>──► after(100) ──► _show_output_context_menu(x, y)
                                                   │
                                     destroy any existing popup
                                                   │
                                 create tk.Toplevel (overrideredirect=True,
                                   bg=panel_bg, borderwidth=0)
                                   └── tk.Button "Copy"
                                         └── output_text.event_generate("<<Copy>>")
                                             dismiss popup
```

```gherkin
# PD-01-AF-010 — right-click opens popup
GIVEN the output_text widget has content
WHEN the user right-clicks anywhere on the output_text widget
THEN a popup menu appears within 200 ms
 AND the popup contains a "Copy" option

# PD-01-AF-010 — Copy with selection copies to clipboard
GIVEN the output_text widget has text and the user has selected some of it
WHEN the user right-clicks and chooses "Copy"
THEN the selected text is placed on the system clipboard
 AND the popup is dismissed

# PD-01-AF-010 — Copy with no selection is a no-op
GIVEN the output_text widget has text and no text is currently selected
WHEN the user right-clicks and chooses "Copy"
THEN the clipboard is unchanged
 AND the popup is dismissed

# PD-01-AF-010 — popup dismisses on Escape
GIVEN the output context menu popup is visible
WHEN the user presses Escape
THEN the popup is dismissed without changing the clipboard

# PD-01-AF-010 — stale popup replaced on second right-click
GIVEN a context menu popup is already visible
WHEN the user right-clicks the output panel again
THEN the first popup is destroyed
 AND a fresh popup appears at the new cursor position

# PD-01-AF-010 — popup uses themed background (no light flash)
GIVEN the active theme has a dark panel_bg colour
WHEN the output context menu popup is created
THEN the Toplevel background is set to panel_bg before it is made visible
 AND no light-coloured pre-render flash is observable
```

### Test Mapping (PD-01)

| Affordance | Test file | Test name |
|-----------|-----------|-----------|
| PD-01-AF-001..007 | `test_chat_panel_turn_rendering.py` | (see matrix) |
| PD-01-AF-009 | `test_startup_log_notice.py` | `TestStartupLogNotice` |
| PD-01-AF-010 | `test_chat_panel_copy_context_menu.py` | `TestOutputPanelRightClickCopy` |

---

## PD-02: InputPanel

**Class**: `InputPanel` (`src/agentx/gui/input_panel.py`)  
**Position**: rely=0.77 to rely=1.0 (bottom 23% of window)  
**Purpose**: Captures user text input and file attachments.

### Placement Diagram

```
┌──────────────────────────── AgentX main window ─────────────────────────────┐
│                                                                              │
│  [ChatPanel rely=0.00–0.77]          [SidePanel rely=0.00–0.77]             │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│  Attachment bar (rely=0.77, relheight=0.03)                                  │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ [📁 file1.py ✓]  [📜 old.py (history) ✓]                              │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
├──────────────────────────────────────────────────────────────────────────────┤
│  User input area (rely=0.80, relheight=0.20)                                 │
│  ┌──────────────────────────────────────────────────┬────┬────┬──────────┐  │
│  │ tk.Text (relwidth=0.90)                          │[⏎] │[❌]│ context  │  │
│  │ (multi-line input, wraps at word boundaries)     │    │    │  meter   │  │
│  │                                           ▲ scrollbar │    │ (donut)  │  │
│  └──────────────────────────────────────────────────┴────┴────┴──────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Internal Structure — Attachment Bar

Each call to `InputPanel.update_attachment_bar(current, history)` destroys all
existing chip widgets, then creates one chip per attachment in order:
current-turn chips first, then history chips.

```
attachments_frame (tk.Frame, parent=root)
  ├── att_frame (tk.Frame, bg=attachment_bg)          ← one per current-turn att
  │     └── tk.Checkbutton(text="📁 {display_name}", variable=BooleanVar(enabled))
  └── att_frame (tk.Frame, bg=history_attachment_bg)  ← one per history att
        └── tk.Checkbutton(text="📜 {display_name} (history)", variable=BooleanVar(enabled))
```

### Behaviour Inventory

| ID | Affordance | Widget | Trigger | Outcome |
|----|-----------|--------|---------|---------|
| PD-02-AF-001 | Enter key submits | `user_input_text` | `<Control-Return>` binding | Invokes `user_submit` button |
| PD-02-AF-002 | Shift+Enter inserts newline | `user_input_text` | `<Shift-Return>` binding | Inserts `\n` into text widget |
| PD-02-AF-003 | Send disabled during streaming | `user_submit` | `set_streaming_state(True)` | `state=DISABLED` |
| PD-02-AF-004 | Stop enabled during streaming | `user_break` | `set_streaming_state(True)` | `state=NORMAL` |
| PD-02-AF-005 | Chip renders with filename | `att_frame` + `Checkbutton` | `update_attachment_bar([info], [])` | Frame packed; Checkbutton text contains `display_name`; current-turn: `📁` icon, bright bg; history: `📜` icon + `" (history)"` suffix, grey bg |
| PD-02-AF-006 | Toggle chip calls callback | `Checkbutton` (inside chip) | User clicks checkbox | `on_attachment_toggle(attachment_id, bool)` called with new enabled state |
| PD-02-AF-007 | Rebuild clears old chips | `attachments_frame` children | `update_attachment_bar([], [])` | All previous chip frames destroyed; `attachment_labels` empty |
| PD-02-AF-008 | Right-click opens context popup on input widget | `user_input_text` | `<Button-3>` binding | Wayland-safe `tk.Toplevel(overrideredirect=True)` popup appears with conditional "Copy" and/or "Paste" items |
| PD-02-AF-009 | Input context menu shows "Copy" only when text is selected | `user_input_text` | Popup construction | "Copy" item present iff `SEL` tag exists at time of right-click; absent otherwise |
| PD-02-AF-010 | Input context menu shows "Paste" only when clipboard is non-empty | `user_input_text` | Popup construction | "Paste" item present iff `clipboard_get()` succeeds (non-empty); absent otherwise (guarded with `try/except tk.TclError`) |
| PD-02-AF-011 | "Copy" in input context menu copies selected text | `user_input_text` | User clicks "Copy" in popup | `user_input_text.event_generate("<<Copy>>")` called; selected text placed on system clipboard; popup dismissed |
| PD-02-AF-012 | "Paste" in input context menu replaces selection / inserts at cursor | `user_input_text` | User clicks "Paste" in popup | If `SEL` tag exists, selected text deleted first; then clipboard content inserted at `INSERT`; popup dismissed |

### Gherkin Use-Cases

```gherkin
# PD-02-AF-002 — Shift+Enter inserts newline in empty widget
GIVEN the user_input_text widget is empty
WHEN  _on_shift_return is invoked
THEN  the widget contains a newline character

# PD-02-AF-002 — Shift+Enter inserts newline after existing text
GIVEN the user_input_text contains "hello" and the cursor is at the end
WHEN  _on_shift_return is invoked
THEN  the widget content is "hello\n"

# PD-02-AF-002 — return value suppresses default handling
GIVEN the user_input_text widget exists
WHEN  _on_shift_return is invoked
THEN  the return value is "break"

# PD-02-AF-002 — binding registered on text widget
GIVEN InputPanel.create() has been called
WHEN  we query the bindings on user_input_text
THEN  a Shift+Return binding is present

# PD-02-AF-002 — newline inserted at cursor, not at end
GIVEN the user_input_text contains "ab" and the cursor is between 'a' and 'b'
WHEN  _on_shift_return is invoked
THEN  the content is "a\nb" (newline at cursor position)

# PD-02-AF-005 — chip render (current-turn)
GIVEN an AttachmentInfo with display_name="parser.py" and is_from_history=False
WHEN  update_attachment_bar([info], []) is called
THEN  attachment_labels has 1 entry
  AND the Checkbutton text contains "parser.py"
  AND the Checkbutton text starts with the 📁 icon

# PD-02-AF-005 — chip render (history)
GIVEN an AttachmentInfo with display_name="old.txt" and is_from_history=True
WHEN  update_attachment_bar([], [info]) is called
THEN  attachment_labels has 1 entry
  AND the Checkbutton text contains "old.txt"
  AND the Checkbutton text contains "(history)"

# PD-02-AF-005 — multiple chips
GIVEN two AttachmentInfos with display_names "a.py" and "b.py"
WHEN  update_attachment_bar([info_a, info_b], []) is called
THEN  attachment_labels has 2 entries

# PD-02-AF-006 — toggle off
GIVEN a chip rendered with enabled=True and attachment_id="att-x"
WHEN  the Checkbutton is invoked (checked → unchecked)
THEN  on_attachment_toggle("att-x", False) is called exactly once

# PD-02-AF-006 — toggle on
GIVEN a chip rendered with enabled=False and attachment_id="att-y"
WHEN  the Checkbutton is invoked (unchecked → checked)
THEN  on_attachment_toggle("att-y", True) is called exactly once

# PD-02-AF-007 — rebuild empties bar
GIVEN one chip already rendered
WHEN  update_attachment_bar([], []) is called
THEN  attachment_labels is empty

# PD-02-AF-007 — rebuild replaces chips
GIVEN a chip for "old.py" already rendered
WHEN  update_attachment_bar([new_info("new.py")], []) is called
THEN  attachment_labels has 1 entry
  AND the Checkbutton text contains "new.py"

# PD-02-AF-008 — right-click opens popup
GIVEN the user_input_text widget has focus and may or may not have selected text
WHEN  the user right-clicks anywhere on user_input_text
THEN  a Wayland-safe tk.Toplevel popup appears within 200 ms
 AND  the popup contains at least one action item

# PD-02-AF-008 — stale popup replaced on second right-click
GIVEN an input context menu popup is already visible
WHEN  the user right-clicks the input widget again
THEN  the first popup is destroyed
 AND  a fresh popup appears at the new cursor position

# PD-02-AF-008 — popup dismisses on Escape
GIVEN the input context menu popup is visible
WHEN  the user presses Escape
THEN  the popup is dismissed without modifying the input or clipboard

# PD-02-AF-008 — popup uses themed background (no light flash)
GIVEN the active theme has a dark panel_bg colour
WHEN  the input context menu popup is created
THEN  the Toplevel background is set to panel_bg before it is made visible

# PD-02-AF-009 — Copy item present when text is selected
GIVEN the user_input_text widget contains "hello world" with "hello" selected
WHEN  the right-click popup is constructed
THEN  the popup contains a "Copy" item

# PD-02-AF-009 — Copy item absent when no text is selected
GIVEN the user_input_text widget contains "hello world" with no selection
WHEN  the right-click popup is constructed
THEN  the popup does NOT contain a "Copy" item

# PD-02-AF-010 — Paste item present when clipboard is non-empty
GIVEN the system clipboard contains the text "world"
WHEN  the right-click popup is constructed on user_input_text
THEN  the popup contains a "Paste" item

# PD-02-AF-010 — Paste item absent when clipboard is empty
GIVEN the system clipboard is empty (clipboard_get() raises TclError)
WHEN  the right-click popup is constructed on user_input_text
THEN  the popup does NOT contain a "Paste" item

# PD-02-AF-011 — Copy copies selection to clipboard
GIVEN user_input_text contains "hello world" with "hello" selected
WHEN  the user chooses "Copy" from the input context popup
THEN  the system clipboard contains "hello"
 AND  the popup is dismissed
 AND  the input text is unchanged

# PD-02-AF-012 — Paste replaces selected text
GIVEN user_input_text contains "hello world" with "hello" selected
 AND  the system clipboard contains "goodbye"
WHEN  the user chooses "Paste" from the input context popup
THEN  the input widget contains "goodbye world"
 AND  the popup is dismissed

# PD-02-AF-012 — Paste inserts at cursor when no selection
GIVEN user_input_text contains "helo world" with the cursor after "hel" (no selection)
 AND  the system clipboard contains "l"
WHEN  the user chooses "Paste" from the input context popup
THEN  the input widget contains "hello world"
 AND  the popup is dismissed
```

### Test Mapping

| Affordance | Test file | Test name |
|-----------|-----------|-----------|
| PD-02-AF-002 | `test_input_panel_keyboard.py` | `test_shift_return_inserts_newline_into_empty_widget` |
| PD-02-AF-002 | `test_input_panel_keyboard.py` | `test_shift_return_inserts_newline_after_existing_text` |
| PD-02-AF-002 | `test_input_panel_keyboard.py` | `test_shift_return_returns_break` |
| PD-02-AF-002 | `test_input_panel_keyboard.py` | `test_shift_return_binding_registered_on_input_text` |
| PD-02-AF-002 | `test_input_panel_keyboard.py` | `test_shift_return_inserts_at_cursor_not_at_end` |
| PD-02-AF-005 | `test_input_panel_attachment_chips.py` | `test_current_attachment_chip_shows_filename` |
| PD-02-AF-005 | `test_input_panel_attachment_chips.py` | `test_history_attachment_chip_shows_filename_and_history_suffix` |
| PD-02-AF-005 | `test_input_panel_attachment_chips.py` | `test_multiple_chips_rendered_in_order` |
| PD-02-AF-006 | `test_input_panel_attachment_chips.py` | `test_uncheck_calls_on_attachment_toggle_false` |
| PD-02-AF-006 | `test_input_panel_attachment_chips.py` | `test_check_after_uncheck_calls_toggle_true` |
| PD-02-AF-007 | `test_input_panel_attachment_chips.py` | `test_empty_update_clears_all_chips` |
| PD-02-AF-007 | `test_input_panel_attachment_chips.py` | `test_rebuild_replaces_existing_chips` |
| PD-02-AF-008 | `test_input_panel_context_menu.py` | `TestInputPanelRightClickPopup` — *not yet implemented* |
| PD-02-AF-009 | `test_input_panel_context_menu.py` | `TestInputCopyMenuVisibility` — *not yet implemented* |
| PD-02-AF-010 | `test_input_panel_context_menu.py` | `TestInputPasteMenuVisibility` — *not yet implemented* |
| PD-02-AF-011 | `test_input_panel_context_menu.py` | `TestInputCopyAction` — *not yet implemented* |
| PD-02-AF-012 | `test_input_panel_context_menu.py` | `TestInputPasteAction` — *not yet implemented* |

### Code / Configuration References

| Concept | Location |
|---------|---------|
| `InputPanel` class | `src/agentx/gui/input_panel.py` |
| `update_attachment_bar()` | `src/agentx/gui/input_panel.py` L171 |
| `_create_attachment_widget()` | `src/agentx/gui/input_panel.py` L211 |
| `AttachmentInfo` DTO | `src/agentx/attachment_info.py` |
| `WidgetRegistry.clear_attachments()` | `src/agentx/widget_registry.py` |
| `on_attachment_toggle` callback | `src/agentx/session.py` — `_handle_attachment_toggle()` |
| Chip colours | `GUIConfig.attachment_bg`, `GUIConfig.history_attachment_bg`, `GUIConfig.attachment_fg` |

### Keyboard Shortcuts

| Key | Behaviour |
|-----|-----------|
| `Ctrl+Enter` | Send message (same as Send button) |
| `Ctrl+Space` | Interrupt / stop streaming |

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
| Per-message row | Expand/collapse button (▶/▼), enabled checkbox, role icon, content preview |
| Tool call sub-row | 🔧 tool_name (indented, collapsible) |
| Plan sub-row | 📋 plan_name (indented, collapsible) |

#### PD-03-AF-007 — Message Enabled Checkbox

**ID**: `PD-03-AF-007`  
**Widget**: `tk.Checkbutton` in `MESSAGE_COLUMNS["enabled"]` (column 1) of each message row  
**Source**: `ContextRenderer._render_message_to_grid()`  
**Purpose**: Allow the user to exclude individual context messages from the LLM prompt without deleting them.

**Behaviour**:

| Action | Outcome |
|--------|---------|
| Checkbox rendered | Initial state reflects `message.enabled` — checked when `True`, unchecked when `False` |
| User checks/unchecks | `message.enabled` is updated in-place to the new boolean value |
| `enabled=False` message | Excluded from `Context.to_llm_messages()` on the next LLM call |

**Gherkin Use-Cases**:

```gherkin
# PD-03-AF-007 — initial state enabled
GIVEN a Message with enabled=True
WHEN  the message row is rendered into a frame via _render_message_to_grid()
THEN  a Checkbutton is present in the enabled column
  AND the Checkbutton variable reports True

# PD-03-AF-007 — initial state disabled
GIVEN a Message with enabled=False
WHEN  the message row is rendered
THEN  the Checkbutton variable reports False

# PD-03-AF-007 — unchecking updates model
GIVEN a Message with enabled=True rendered in a frame
WHEN  the Checkbutton is invoked (checked → unchecked)
THEN  message.enabled is False

# PD-03-AF-007 — checking updates model
GIVEN a Message with enabled=False rendered in a frame
WHEN  the Checkbutton is invoked (unchecked → checked)
THEN  message.enabled is True
```

**Test Mapping**:

| Affordance | Test file | Test name |
|-----------|-----------|-----------|
| PD-03-AF-007 | `test_context_renderer_message_enabled.py` | `test_enabled_checkbox_initial_true` |
| PD-03-AF-007 | `test_context_renderer_message_enabled.py` | `test_enabled_checkbox_initial_false` |
| PD-03-AF-007 | `test_context_renderer_message_enabled.py` | `test_uncheck_sets_message_enabled_false` |
| PD-03-AF-007 | `test_context_renderer_message_enabled.py` | `test_check_sets_message_enabled_true` |

#### PD-03-AF-011 — Working Memory Toggle Checkbox

**ID**: `PD-03-AF-011`
**Widget**: `tk.Checkbutton` in column 0 of each fact `row_frame`
**Source**: `ContextRenderer._render_working_memory_row()`
**Purpose**: Include or exclude an individual fact from the LLM context without deleting it.

**Behaviour**:

| Action | Outcome |
|--------|---------|
| Widget rendered | Checkbutton initial state reflects `fact.enabled` |
| User checks | `on_toggle(compound_key, True)` fired |
| User unchecks | `on_toggle(compound_key, False)` fired |
| `on_toggle=None` | Invocation is silently ignored (no error) |

**Gherkin Use-Cases**:

```gherkin
# PD-03-AF-011 — initial state enabled
GIVEN a WorkingMemory with one fact (enabled=True)
WHEN  render_working_memory_widget() is called
THEN  the toggle Checkbutton variable is True

# PD-03-AF-011 — initial state disabled
GIVEN a WorkingMemory with one fact (enabled=False)
WHEN  render_working_memory_widget() is called
THEN  the toggle Checkbutton variable is False

# PD-03-AF-011 — uncheck fires on_toggle False
GIVEN a fact (enabled=True) and an on_toggle callback
WHEN  the toggle Checkbutton is invoked (unchecking it)
THEN  on_toggle is called with (compound_key, False)

# PD-03-AF-011 — check fires on_toggle True
GIVEN a fact (enabled=False) and an on_toggle callback
WHEN  the toggle Checkbutton is invoked (checking it)
THEN  on_toggle is called with (compound_key, True)

# PD-03-AF-011 — no callback does not raise
GIVEN a fact and on_toggle=None
WHEN  the toggle Checkbutton is invoked
THEN  no exception is raised
```

**Test Mapping**:

| Affordance | Test file | Test name |
|-----------|-----------|-----------|
| PD-03-AF-011 | `test_working_memory_widget_callbacks.py` | `test_toggle_initial_checked_state_matches_fact_enabled` |
| PD-03-AF-011 | `test_working_memory_widget_callbacks.py` | `test_toggle_initial_unchecked_state_matches_fact_disabled` |
| PD-03-AF-011 | `test_working_memory_widget_callbacks.py` | `test_toggle_calls_on_toggle_with_false` |
| PD-03-AF-011 | `test_working_memory_widget_callbacks.py` | `test_toggle_calls_on_toggle_with_true` |
| PD-03-AF-011 | `test_working_memory_widget_callbacks.py` | `test_toggle_no_callback_does_not_raise` |

---

#### PD-03-AF-012 — Working Memory Delete Button

**ID**: `PD-03-AF-012`
**Widget**: `tk.Button(text="✕")` in column 3 of the `row_frame` — AGENT-owned facts only
**Source**: `ContextRenderer._render_working_memory_row()`
**Purpose**: Permanently delete an agent-owned fact after user confirmation.

**Behaviour**:

| Action | Outcome |
|--------|---------|
| ✕ clicked, dialog confirmed | `on_delete(compound_key)` fired |
| ✕ clicked, dialog cancelled | `on_delete` NOT called |
| USER-owned fact | No ✕ button rendered |
| `on_delete=None` and confirmed | Silently ignored (no error) |

**Gherkin Use-Cases**:

```gherkin
# PD-03-AF-012 — delete confirmed
GIVEN a WorkingMemory with one AGENT fact and an on_delete callback
WHEN  the ✕ button is clicked and the confirmation dialog returns True
THEN  on_delete is called with the fact's compound_key

# PD-03-AF-012 — delete cancelled
GIVEN a WorkingMemory with one AGENT fact and an on_delete callback
WHEN  the ✕ button is clicked and the dialog returns False
THEN  on_delete is NOT called

# PD-03-AF-012 — absent for USER fact
GIVEN a WorkingMemory with one USER fact
WHEN  render_working_memory_widget() is called
THEN  no ✕ button is present in the row

# PD-03-AF-012 — no callback does not raise
GIVEN a AGENT fact and on_delete=None
WHEN  the ✕ button is clicked and confirmed
THEN  no exception is raised
```

**Test Mapping**:

| Affordance | Test file | Test name |
|-----------|-----------|-----------|
| PD-03-AF-012 | `test_working_memory_widget_callbacks.py` | `test_delete_button_calls_on_delete_when_confirmed` |
| PD-03-AF-012 | `test_working_memory_widget_callbacks.py` | `test_delete_button_not_called_when_cancelled` |
| PD-03-AF-012 | `test_working_memory_widget_callbacks.py` | `test_delete_button_absent_for_user_fact` |
| PD-03-AF-012 | `test_working_memory_widget_callbacks.py` | `test_delete_no_callback_does_not_raise` |

---

#### PD-03-AF-013 — Working Memory Promote Button

**ID**: `PD-03-AF-013`
**Widget**: `tk.Button(text=fact.owner_icon)` in column 1 of the `row_frame` — AGENT-owned facts only
**Source**: `ContextRenderer._render_working_memory_row()`, `ContextRenderer._confirm_promote()`
**Purpose**: Transfer ownership of an agent-written fact to the user, preventing the agent from overwriting it.

**Behaviour**:

| Action | Outcome |
|--------|---------|
| Owner icon clicked (🤖), dialog confirmed | `on_promote(compound_key)` fired |
| Owner icon clicked, dialog cancelled | `on_promote` NOT called |
| USER-owned fact | Owner icon is a static `tk.Label` (not clickable) |
| `on_promote=None` and confirmed | Silently ignored (no error) |

**Gherkin Use-Cases**:

```gherkin
# PD-03-AF-013 — promote confirmed
GIVEN a WorkingMemory with one AGENT fact and an on_promote callback
WHEN  the owner-icon button is clicked and the promote dialog returns True
THEN  on_promote is called with the fact's compound_key

# PD-03-AF-013 — promote cancelled
GIVEN a WorkingMemory with one AGENT fact and an on_promote callback
WHEN  the owner-icon button is clicked and the dialog returns False
THEN  on_promote is NOT called

# PD-03-AF-013 — USER fact has Label not Button
GIVEN a WorkingMemory with one USER fact
WHEN  render_working_memory_widget() is called
THEN  the owner icon is a tk.Label (not tk.Button)

# PD-03-AF-013 — no callback does not raise
GIVEN an AGENT fact and on_promote=None
WHEN  the owner-icon button is clicked and confirmed
THEN  no exception is raised
```

**Test Mapping**:

| Affordance | Test file | Test name |
|-----------|-----------|-----------|
| PD-03-AF-013 | `test_working_memory_widget_callbacks.py` | `test_promote_calls_on_promote_when_confirmed` |
| PD-03-AF-013 | `test_working_memory_widget_callbacks.py` | `test_promote_not_called_when_cancelled` |
| PD-03-AF-013 | `test_working_memory_widget_callbacks.py` | `test_user_fact_owner_icon_is_label_not_button` |
| PD-03-AF-013 | `test_working_memory_widget_callbacks.py` | `test_promote_no_callback_does_not_raise` |

---

#### PD-03-AF-014 — Working Memory Add-Fact Form

**ID**: `PD-03-AF-014`
**Widget**: Footer `add_frame` inside `render_working_memory_widget()` — always present regardless of fact count
**Source**: `ContextRenderer.render_working_memory_widget()`
**Purpose**: Let the user add a new user-owned fact by entering a key and value.

**Internal Structure**:

```
add_frame
  ├── Label  "👤 Add fact:"
  ├── Label  "key"     Entry [key_var, width=18]
  ├── Label  "value"   Entry [val_var, width=28]
  └── Button "Add 👤"
```

**Behaviour**:

| Action | Outcome |
|--------|---------|
| Key non-empty, "Add 👤" clicked | `on_user_add(key.strip(), value.strip())` fired; both entries cleared |
| Key empty, "Add 👤" clicked | `on_user_add` NOT called; entries not cleared |
| `on_user_add=None` with valid key | Silently ignored (no error) |

**Gherkin Use-Cases**:

```gherkin
# PD-03-AF-014 — submit with key and value
GIVEN a rendered WM widget with on_user_add callback
WHEN  the key Entry contains "my_key", value Entry contains "my_val"
  AND "Add 👤" is clicked
THEN  on_user_add is called with ("my_key", "my_val")

# PD-03-AF-014 — entries cleared after submit
GIVEN a rendered WM widget with on_user_add callback
WHEN  "Add 👤" is clicked with non-empty key/value
THEN  both Entry fields are empty after the call

# PD-03-AF-014 — empty key suppresses callback
GIVEN a rendered WM widget with on_user_add callback
WHEN  the key Entry is empty and "Add 👤" is clicked
THEN  on_user_add is NOT called

# PD-03-AF-014 — no callback does not raise
GIVEN a rendered WM widget with on_user_add=None
WHEN  "Add 👤" is clicked with a non-empty key
THEN  no exception is raised
```

**Test Mapping**:

| Affordance | Test file | Test name |
|-----------|-----------|-----------|
| PD-03-AF-014 | `test_working_memory_widget_callbacks.py` | `test_add_button_calls_on_user_add_with_key_and_value` |
| PD-03-AF-014 | `test_working_memory_widget_callbacks.py` | `test_add_button_clears_entries_after_submit` |
| PD-03-AF-014 | `test_working_memory_widget_callbacks.py` | `test_add_button_does_not_call_on_user_add_when_key_empty` |
| PD-03-AF-014 | `test_working_memory_widget_callbacks.py` | `test_add_button_no_callback_does_not_raise` |

---

#### PD-03-AF-015 — Working Memory Section Starts Collapsed

**ID**: `PD-03-AF-015`
**Widget**: `CollapsibleSection` keyed `"working_memory"` in `SidePanel._session_sections`
**Source**: `SidePanel.create()` in `side_panel.py`
**Purpose**: Ensure the Working Memory section starts collapsed at startup, consistent with History, Available Tools, and Context sections.

**Behaviour**:

| Condition | Outcome |
|-----------|---------|
| SidePanel freshly created | `working_memory` section `is_expanded()` returns `False` |
| User clicks section header | Section toggles (expand/collapse) normally |

**Gherkin Use-Cases**:

```gherkin
# PD-03-AF-015 — Working Memory starts collapsed
GIVEN a freshly created SidePanel
WHEN  SidePanel.create() runs
THEN  the "working_memory" CollapsibleSection is_expanded() == False

# PD-03-AF-015 — consistent with peer sections
GIVEN a freshly created SidePanel
WHEN  SidePanel.create() runs
THEN  history, tools, working_memory, and context are all collapsed
```

**Test Mapping**:

| Affordance | Test file | Test name |
|-----------|-----------|-----------|
| PD-03-AF-015 | `test_gui_manager_integration.py` | `test_session_sections_start_collapsed` |

---

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

### Affordance: PD-04-AF-004 — Refresh button reloads model list

**Source**: `ModelSelector._on_refresh()` (`src/agentx/gui/model_selector.py`)  
**Test**: `tests/test_model_selector_refresh.py` · `TestModelSelectorRefreshButton`

```gherkin
GIVEN a ModelSelector widget is rendered with a refresh callback registered
WHEN the user clicks the [⟳] button
THEN the refresh callback is invoked once
 AND the model dropdown is repopulated with the updated list from Agentix

GIVEN a ModelSelector widget is rendered with no refresh callback
WHEN the user clicks the [⟳] button
THEN no exception is raised

GIVEN a ModelSelector widget with a previous refresh callback
WHEN set_refresh_callback() is called with a new callback
THEN only the new callback is invoked on the next button click
```

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

### Affordance: PD-05-AF-004 — Re-synth button opens ResynthesisDialog

**Source**: `PlanTreeWidget._create_synthesis_block()` (`src/agentx/gui/plan_tree_widget.py`)  
**Test**: `tests/test_plan_tree_affordances.py` · `TestResynthButtonInSynthesisBlock`

```gherkin
GIVEN a PlanTreeWidget with a task node that has an on_resynth callback
WHEN add_synthesis_to_node() is called
THEN a Re-synth button is present in the node's details frame
 AND clicking it invokes the on_resynth callback

GIVEN a PlanTreeWidget with a task node that has no on_resynth callback
WHEN add_synthesis_to_node() is called
THEN no Re-synth button is created

GIVEN a PlanTreeWidget and a task_id that does not exist in the tree
WHEN add_synthesis_to_node() is called with an on_resynth callback
THEN no exception is raised
```

### Affordance: PD-05-AF-005 — Export button writes and opens export file

**Source**: `ChatPanel.add_plan_tab()` (`src/agentx/gui/chat_panel.py`) / `AgentXSession._export_task_tree()` (`src/agentx/session.py`)  
**Test**: `tests/test_plan_tree_affordances.py` · `TestExportButtonInPlanTab`

```gherkin
GIVEN a ChatPanel with a plan tab added via add_plan_tab()
WHEN the toolbar is inspected
THEN an Export button is present

GIVEN a ChatPanel with a plan tab and an on_export callback registered
WHEN the user clicks the Export button
THEN the on_export callback is invoked once

GIVEN a ChatPanel with a plan tab and no on_export callback
WHEN the user clicks the Export button
THEN no exception is raised
```

### Affordance: PD-05-AF-006 — Node status icon reflects task state

**Source**: `PlanTreeWidget.update_node_status()` / `_STATUS_ICONS` (`src/agentx/gui/plan_tree_widget.py`)  
**Test**: `tests/test_plan_tree_affordances.py` · `TestNodeStatusIconReflectsState`

```gherkin
GIVEN a PlanTreeWidget with a task node
WHEN update_node_status(task_id, "pending") is called
THEN the node label shows the pending icon (○)

WHEN update_node_status(task_id, "running") is called
THEN the node label shows the running icon (●)

WHEN update_node_status(task_id, "done") is called
THEN the node label shows the done icon (✓)

WHEN update_node_status(task_id, "needs_review") is called
THEN the node label shows the review icon (?)

WHEN update_node_status(task_id, "failed") is called
THEN the node label shows the failed icon (✗)

GIVEN an unknown status string
WHEN update_node_status() is called
THEN a fallback bullet icon is displayed and no exception is raised

GIVEN a task_id that does not exist in the tree
WHEN update_node_status() is called
THEN no exception is raised

GIVEN a task node that transitions through multiple statuses
WHEN update_node_status() is called multiple times
THEN each call updates the icon to match the current status
```

---

## PD-06: ResynthesisDialog

**Class**: `ResynthesisDialog` (`src/agentx/gui/resynthesis_dialog.py`)  
**Type**: Modal `tk.Toplevel` (blocks parent with `grab_set()`)  
**Purpose**: Re-run synthesis for a specific task node, optionally injecting a free-text hint and/or a new Working-Memory fact before confirming.

### Placement Diagram

```
┌─────────────────────────── AgentX main window ────────────────────────────┐
│                                                                            │
│   [ChatPanel]               [SidePanel]                                   │
│                                                                            │
│        ┌──────────── ResynthesisDialog (modal Toplevel) ──────────────┐   │
│        │  Re-synthesise — <task_id>              640 × 520 px          │   │
│        └──────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────────────┘
```

The dialog is transient to its parent widget and centered on screen.

### Internal Structure Diagram

```
ResynthesisDialog._win  (tk.Toplevel, bg #1e1e1e, 640×520)
├── title_label         "Current synthesis:"  (tk.Label)
├── synth_frame         (tk.Frame)
│     ├── _synth_text   (tk.Text, read-only, 6 rows, scrollable)
│     └── synth_scroll  (tk.Scrollbar)
│
├── [assertions_label]  "Assertion failures:"  (tk.Label — only when failures > 0)
├── [fail_frame]        (tk.Frame, bg #2a1a1a — only when failures > 0)
│     └── ...labels per assertion
│
├── hint_label          "Hint for re-synthesis (optional):"  (tk.Label)
├── _hint_text          (tk.Text, 4 rows, writable)
│
├── [wm_frame]          (tk.Frame — only when on_add_wm_hint provided)
│     ├── wm_label      "Add working-memory fact:"  (tk.Label)
│     ├── fields_frame  (tk.Frame)
│     │     ├── key_label + _wm_key_var Entry
│     │     └── val_label + _wm_val_var Entry
│     └── add_wm_btn    [Add WM hint]  (tk.Button)
│
└── btn_frame           (tk.Frame)
      ├── confirm_btn   [Re-synthesise]  (tk.Button, bg #166534)
      └── cancel_btn    [Cancel]  (tk.Button, bg #3a3a3a)
```

### Behaviour Inventory

| ID | Control / Trigger | Behaviour | Notes |
|----|-------------------|-----------|-------|
| PD-06-AF-001 | Window title | Title reads `"Re-synthesise — <task_id>"` | Set in `__init__` via `self._win.title(...)` |
| PD-06-AF-002 | `[Cancel]` button | Destroys `_win`; `on_confirm` is **not** called | `command=self._win.destroy` |
| PD-06-AF-003 | `[Re-synthesise]` button | Destroys `_win`, then calls `on_confirm(hint.strip())` | Hint may be empty string |
| PD-06-AF-004 | WM hint section | Hidden when `on_add_wm_hint=None`; visible when provided | Entire `wm_frame` only packed if callback supplied |
| PD-06-AF-005 | `[Add WM hint]` button | Calls `on_add_wm_hint(key, value)`, clears fields; shows warning if key or value blank | Dialog remains open after WM hint added |

### Gherkin Use-Cases

```gherkin
# PD-06-AF-001
Scenario: Dialog title includes task ID
  Given ResynthesisDialog is constructed with task_id="step-42"
  When the dialog window is displayed
  Then the window title is "Re-synthesise — step-42"

# PD-06-AF-002
Scenario: Cancel closes dialog without calling on_confirm
  Given a ResynthesisDialog with a mock on_confirm callback
  When the Cancel button is invoked
  Then the dialog window is destroyed
  And on_confirm is not called

# PD-06-AF-003
Scenario: Re-synthesise calls on_confirm with hint text
  Given a ResynthesisDialog with a mock on_confirm callback
  And the hint field contains "focus on error handling"
  When the Re-synthesise button is invoked
  Then the dialog window is destroyed
  And on_confirm is called once with "focus on error handling"

# PD-06-AF-003 (empty hint)
Scenario: Re-synthesise with empty hint passes empty string
  Given a ResynthesisDialog with a mock on_confirm callback
  And the hint field is empty
  When the Re-synthesise button is invoked
  Then on_confirm is called once with ""

# PD-06-AF-004
Scenario: WM hint section hidden when on_add_wm_hint not provided
  Given ResynthesisDialog constructed without on_add_wm_hint
  When the dialog is displayed
  Then no "Add WM hint" button is visible in the dialog

# PD-06-AF-004 (variant)
Scenario: WM hint section visible when on_add_wm_hint provided
  Given ResynthesisDialog constructed with a mock on_add_wm_hint callback
  When the dialog is displayed
  Then the "Add WM hint" button is visible in the dialog

# PD-06-AF-005
Scenario: Add WM hint calls callback and clears fields
  Given ResynthesisDialog with on_add_wm_hint provided
  And key field contains "style" and value field contains "concise"
  When the Add WM hint button is invoked
  Then on_add_wm_hint is called with ("style", "concise")
  And the key and value fields are cleared
  And the dialog remains open (on_confirm not called)
```

### Test Mapping

| Affordance ID | Test File | Test Function |
|---------------|-----------|---------------|
| PD-06-AF-001 | `test_resynthesis_dialog.py` | `test_title_includes_task_id` |
| PD-06-AF-002 | `test_resynthesis_dialog.py` | `test_cancel_destroys_dialog_without_confirm` |
| PD-06-AF-003 | `test_resynthesis_dialog.py` | `test_confirm_calls_on_confirm_with_hint` |
| PD-06-AF-003 | `test_resynthesis_dialog.py` | `test_confirm_with_empty_hint_passes_empty_string` |
| PD-06-AF-004 | `test_resynthesis_dialog.py` | `test_wm_section_hidden_without_callback` |
| PD-06-AF-004 | `test_resynthesis_dialog.py` | `test_wm_section_visible_with_callback` |
| PD-06-AF-005 | `test_resynthesis_dialog.py` | `test_add_wm_hint_calls_callback_and_clears_fields` |

### Code and Configuration References

| Symbol | Location |
|--------|----------|
| `ResynthesisDialog.__init__` | `src/agentx/gui/resynthesis_dialog.py:18` |
| `ResynthesisDialog._on_confirm_clicked` | `src/agentx/gui/resynthesis_dialog.py:193` |
| `ResynthesisDialog._on_add_wm_hint_clicked` | `src/agentx/gui/resynthesis_dialog.py:198` |
| `ResynthesisDialog.wait` | `src/agentx/gui/resynthesis_dialog.py:209` |
| Caller (Re-synth button) | `src/agentx/gui/plan_tree_widget.py:342` |

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

---

### PD-07-AF-002: Section Collapse Defaults

**Affordance ID**: `PD-07-AF-002`
**Source**: `SettingsTab._make_section()` / `SettingsTab.__init__()` (`src/agentx/gui/settings_tab.py`)
**Tests**: `tests/test_settings_tab_sections.py::TestSettingsTabSectionCollapseDefaults`

Each settings section uses `CollapsibleSection(initial_collapsed=...)`. The initial
`expanded` state of each section is set at construction time as follows:

| Section title | `initial_collapsed` | Initial visible state |
|---------------|--------------------|-----------------------|
| 🎨 Appearance | `False` | Expanded (▼) |
| 🤖 Ollama | `False` | Expanded (▼) |
| 🧠 Agentix | `False` | Expanded (▼) |
| 📊 Classification Display | `True` | Collapsed (▶) |
| 🏛️ Working Memory | `True` | Collapsed (▶) |

The three top sections are expanded by default so the most common settings are immediately
visible. The two less-frequently-needed sections are collapsed to reduce visual noise on
first load.

**Behaviour**: `CollapsibleSection.expanded = not initial_collapsed`. User can toggle any
section by clicking the ▼/▶ button; state is not persisted across restarts.

---

### PD-07-AF-003: Restart-Required Icon in Label Text

**Affordance ID**: `PD-07-AF-003`
**Source**: `SettingsTab.RESTART_ICON` class constant; `_add_checkbox()`, `_add_text_entry()`,
`_add_spinbox()`, `_add_enum_dropdown()`, `_add_model_dropdown()` (`src/agentx/gui/settings_tab.py`)
**Tests**: `tests/test_settings_tab_sections.py::TestRestartIconInLabels`

Settings whose changes are persisted to disk but do NOT take effect until the app is
restarted carry the `🔁` icon appended to their label text.

- **Class constant**: `SettingsTab.RESTART_ICON = " 🔁"` (space + emoji)
- **`_add_checkbox`**, **`_add_text_entry`**, **`_add_spinbox`**: append `RESTART_ICON` to
  `label` when `restart=True` is passed
- **`_add_enum_dropdown`** and **`_add_model_dropdown`**: callers include `RESTART_ICON`
  explicitly in the `label` argument (these helpers have no `restart` parameter)

**Restart-required fields**:

| Setting key | Label shown |
|-------------|-------------|
| `agentx.theme_mode` | `Theme mode 🔁` |
| `agentx.ollama_host` | `Host 🔁` |
| `agentx.ollama_model` | `Default model 🔁` |
| `agentx.ollama_initial_load_timeout_seconds` | `Load timeout (s) 🔁` |
| `agentx.screen_side` | `Screen side 🔁` |
| `agentix.host` | `Host 🔁` |
| `agentx.working_memory.enabled` | `Enabled 🔁` |
| `agentix.classification_torch_model` | `Torch model 🔁` |
| `agentix.classification_torch_device` | `Torch device 🔁` |

The `_RESTART_REQUIRED` module-level set in `settings_tab.py` is the authoritative list of
keys that require restart.

---

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
| `Escape` | Any | Dismisses open context menu |

> **Platform behavior note**: Right-click is bound on `<Button-3>` and posted with a
> short timer delay to avoid immediate button-release dismissal races. Under Wayland
> sessions, FileExplorer uses a custom in-app `tk.Toplevel(overrideredirect=True)`
> fallback popup rather than native Tk menu windows when compositor behavior makes
> menus unreliable. The fallback popup must render with the active theme palette from
> the first visible frame (no default light top-level flash) to prevent visual drift in
> long usage sessions.
> The `<FocusOut>` event is also intentionally **not** used to dismiss the menu, as
> `tk_popup()` steals focus from the treeview when it opens.

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

### Affordance: PD-11-AF-008 — Right-click on a file shows file context menu

**Source**: `FileExplorer._on_right_click()` (`src/agentx/file_explorer.py`)  
**Test**: `tests/test_file_explorer_context_menu.py` · `TestFileContextMenu`

```gherkin
GIVEN the file listing is populated
WHEN the user right-clicks a file row
THEN the file context menu is posted at the cursor position
 AND the menu remains visible (is not immediately dismissed)

GIVEN the user right-clicks a file row and the menu is visible
WHEN the user presses Escape
THEN the menu is dismissed

GIVEN the app is in Wayland fallback popup mode with dark theme selected
WHEN the user right-clicks a file row
THEN the top-level popup surface uses the selected theme palette on first render
 AND no default light top-level frame is shown before buttons are painted

GIVEN the user right-clicks a file row and the menu is visible
WHEN the user clicks Attach
THEN the on_attach callback is invoked with the full path of the selected file

GIVEN the user right-clicks a file row and the menu is visible
WHEN the user clicks Edit
THEN the on_edit callback is invoked with the full path of the selected file
```

### Affordance: PD-11-AF-009 — Right-click on a directory shows folder context menu

**Source**: `FileExplorer._on_right_click()` (`src/agentx/file_explorer.py`)  
**Test**: `tests/test_file_explorer_context_menu.py` · `TestFolderContextMenu`

```gherkin
GIVEN the file listing is populated with a directory row
WHEN the user right-clicks the directory row
THEN the folder context menu is posted (not the file context menu)
 AND the menu remains visible

GIVEN the folder context menu is visible
WHEN the user clicks "Add full path to memory"
THEN the on_add_folder_to_memory callback is invoked with the folder name and its absolute path

GIVEN the folder context menu is visible
WHEN the user clicks "Add relative path to memory"
THEN the on_add_folder_to_memory callback is invoked with the folder name and its path relative to the root path
```

### Affordance: PD-11-AF-010 — Escape dismisses the context menu

**Source**: `FileExplorer._dismiss_popup_menu()` (`src/agentx/file_explorer.py`)  
**Test**: `tests/test_file_explorer_context_menu.py` · `TestDismissContextMenu`

```gherkin
GIVEN a file context menu is open
WHEN the user presses Escape
THEN both the file and folder context menus are unposted

GIVEN no context menu is open
WHEN _dismiss_popup_menu() is called
THEN no exception is raised
```

### State

| Attribute | Type | Description |
|-----------|------|-------------|
| `current_path` | `str` | Absolute path currently displayed |
| `history` | `list[str]` | Navigation history stack |
| `history_index` | `int` | Current position in history stack |

### Related User Flow

See [UF-05: File Attachment](02_USER_FLOWS.md#uf-05-file-attachment) for the end-to-end flow from clicking a file to it appearing as an attachment chip.
See [UF-11: File Explorer Navigation](02_USER_FLOWS.md#uf-11-file-explorer-navigation) for directory browsing and folder-to-memory flows.
See [UF-12: File Explorer Context Popup Rendering](02_USER_FLOWS.md#uf-12-file-explorer-context-popup-rendering) for popup visibility and first-frame palette guarantees.
