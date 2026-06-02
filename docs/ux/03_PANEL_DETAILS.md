# AgentX — Panel Details

_Last updated: 2026-06-01 (v0.79.2)_

Detailed affordance specifications for each GUI panel/widget and the hybrid
runtime surfaces that need UX traceability.  Each section documents the
widget's purpose, all user-visible controls, and the callback wiring to session
logic.

Authoritative contract rule:

- This document defines required user-facing affordances independent of delivery
  technology.
- Implementations may be GUI, TUI, or hybrid, but must satisfy these UX
  behaviors without weakening the contract.

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
_above_ the user prompt.

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
**Status**: ✅ Implemented and tested

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
| PD-02-AF-004 | Stop enabled during streaming | `user_break` | `set_streaming_state(True)` | `state=NORMAL` | ⚠️ **Relocated to PD-12-AF-003** — `user_break` button moves to `StatusTab`; callback unchanged |
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
| PD-02-AF-008 | `test_input_panel_context_menu.py` | `TestInputPanelRightClickPopup` |
| PD-02-AF-009 | `test_input_panel_context_menu.py` | `TestInputCopyMenuVisibility` |
| PD-02-AF-010 | `test_input_panel_context_menu.py` | `TestInputPasteMenuVisibility` |
| PD-02-AF-011 | `test_input_panel_context_menu.py` | `TestInputCopyAction` |
| PD-02-AF-012 | `test_input_panel_context_menu.py` | `TestInputPasteAction` |

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
| `Ctrl+Space` | Interrupt / stop streaming — ⚠️ **binding migrating to `StatusTab` (PD-12 implementation)**; callback unchanged |

### Button State

| State | When |
|-------|------|
| `Send` enabled | Not streaming |
| `Send` disabled | Streaming in progress |
| `Stop` enabled | Streaming in progress — ⚠️ **button relocated to StatusTab (PD-12); not present in InputPanel after PD-12 implementation** |
| `Stop` disabled | Not streaming |

> **PD-12 layout change**: When PD-12 `StatusTab` is implemented:
>
> - `user_break` button is removed from InputPanel right-column
> - `ContextMeterWidget` canvas is removed from InputPanel right-column
> - `user_submit` button shrinks to a slim right-edge strip (`relx=0.96, relwidth=0.04`)
> - `user_input_text` expands from `relwidth=0.90` to `~relwidth=0.96`
> - `Ctrl+Space` binding moves to `StatusTab` (still bound on `root`)

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

---

## PD-17: DemoMode

**Panel/Surface**: Terminal-first split-pane demo harness (`agentx --demo`)  
**Type**: CLI UX mode for pre-UAT validation  
**Primary source**: `cmd/agentx-core/main.go`, `cmd/agentx-core/demo_harness.go`, `cmd/agentx-core/demo_split_mode.go`

DemoMode is a user-visible, interactive pre-UAT flow that runs E2E test sequences and requests user feedback after every test.

In the interactive path, `agentx --demo` opens a split tmux session: the left pane shows the ordered sequence and accepts `N`/`X`, while the right pane mirrors the live AgentX core pane set (chat/context/input) so the operator can watch the actual app respond without collapsing the outer split.

### Affordance Inventory

| Affordance | ID | Expected Behavior | Status |
|-----------|----|-------------------|--------|
| `--demo` enters DemoMode | PD-17-AF-001 | Launches the split-pane demo controller and live-core mirror instead of normal interactive run | ✅ |
| Demo test sequence preview | PD-17-AF-002 | Displays ordered E2E tests with id/title before running | ✅ |
| Start selection from id/index | PD-17-AF-003 | User can choose where to start sequence (`--demo-start` or interactive pick) | ✅ |
| Per-test `N/X` user feedback loop | PD-17-AF-004 | End of each test returns control and accepts only `N` or `X` | ✅ |
| `X` failure artifact bundle | PD-17-AF-005 | Captures all panes + metadata to deterministic logs for analysis | ✅ |
| End-of-run readiness summary | PD-17-AF-006 | Prints run totals, failed step if any, and artifact paths | ✅ |

### Interaction Contract

```gherkin
# PD-17-AF-004 — feedback prompt runs per test (not end of sequence)
GIVEN demo mode is running an ordered test sequence
WHEN an individual test finishes
THEN control returns to the user immediately
 AND prompt accepts only N (next) or X (fail)

# PD-17-AF-005 — fail path captures diagnostics
GIVEN demo mode feedback prompt is visible for a completed test
WHEN the user enters X
THEN demo execution stops
 AND all panes are dumped to log artifacts
 AND artifact paths are printed to terminal

# PD-17-AF-003 — start selection
GIVEN a demo test sequence is available
WHEN the user provides --demo-start <id-or-index>
THEN demo execution begins at the selected test
 AND prior tests are listed as skipped by selection
```

### UX Notes

- DemoMode is a UX surface and must remain operator-friendly.
- Output must be clear and structured, with explicit current-test identity.
- Failure artifacts must be deterministic and easy to locate under `logs/`.
- `make demo-smoke` uses the internal headless flag so the split-pane interactive path stays user-facing while smoke coverage remains deterministic.

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

## PD-18: SystemAppletSuite

**Panel/Surface**: Hybrid runtime system frame and UAT-visible applet startup mode
**Type**: Runtime surface contract for the system applets and visible startup mode
**Primary source**: `cmd/agentx-core/` runtime orchestration, `docs/architecture/runtime_split.md`, `docs/ux/06_TUI_MIRROR.md`

This section defines the user-visible contract for the system applets that will
compose the system frame and for the optional visible-windows startup mode used
to validate applets before frame layout is enabled.

### Affordance Inventory

| Affordance | ID | Expected Behavior | Status |
|-----------|----|-------------------|--------|
| System frame binds by semantic title, not pane index | PD-18-AF-001 | Core resolves owned system surfaces by stable titles/roles after tmuxp overlays and reattach flows | ✅ Tested |
| Context history applet renders recent turn history | PD-18-AF-002 | The context-history surface shows ordered turns, latest prompt/response context, and deterministic truncation rules | ✅ Tested |
| Configuration applet renders runtime config | PD-18-AF-003 | The config surface shows the current runtime config and effective environment-driven overrides | ✅ Tested |
| File-selection applet renders project file navigation | PD-18-AF-004 | The files surface shows the project tree/selection summary that UAT can inspect without switching modes | ✅ Tested |
| Working-memory applet renders session facts | PD-18-AF-005 | The working-memory surface shows current facts as a read-only summary sourced from the active session directory | ✅ Tested |
| Context visualizer applet renders capacity and prompt-cycle status | PD-18-AF-006 | The context surface shows capacity metrics, prompt-cycle status, and meter rows that match core state | ✅ Tested |
| Visible startup mode exposes one window per applet for UAT | PD-18-AF-007 | Optional startup mode launches each applet in its own visible window before frame layout is introduced | ✅ Tested |

### Applet Review Contract

Each applet above must have:

1. A UX specification row in this section.
2. A traceability row in [UX_LIFECYCLE.md](UX_LIFECYCLE.md) with a matching
  affordance ID.
3. Unit tests for the applet's default state and each user-visible state change.
4. Integration or functional tests for startup, reattach, and session ownership
  behavior.
5. A reconciliation step that updates the lifecycle matrix from `📝` to `✅`
  only after implementation and testing are complete.

### Implementation Notes

- The system applet suite is a runtime surface, not a new GUI panel.
- The visible startup mode is a review-only topology to make applet presence and
  basic function observable before the final frame layout lands.
- This section intentionally mirrors the runtime split and UX lifecycle docs so
  implementation work can be reviewed against one authoritative spec chain.

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

## PD-10: ContextMeterWidget

**Class**: `ContextMeterWidget` (`src/agentx/gui/context_meter_widget.py`)
**Position**: Initially hosted in `InputPanel` right-column; relocating to `StatusTab` (PD-12) during PD-12 implementation.
**Purpose**: Donut chart showing context-window utilisation. Seven coloured arc bands represent token categories; a ghost arc shows remaining capacity. A risk border changes colour as utilisation approaches the limit.

### Layout

```
┌─── Canvas (square, configurable size) ─────────────────────┐
│                                                            │
│         ┌───────────────┐                                  │
│       ╱   [arc bands]    ╲                                 │
│      │       NN%          │   ← hole label (percentage)   │
│       ╲                  ╱                                 │
│         └───────────────┘                                  │
│                                                            │
│  [risk border: normal grey ▸ warning orange ▸ red]        │
└────────────────────────────────────────────────────────────┘
```

### Band Definitions (`_BANDS` constant)

| Index | Label | Hex colour |
|-------|-------|------------|
| 0 | Working Memory | `#0d9488` |
| 1 | System Prompts | `#6366f1` |
| 2 | User Prompts | `#3b82f6` |
| 3 | Attachments | `#f59e0b` |
| 4 | Thinking | `#a855f7` |
| 5 | Agent Response | `#22c55e` |
| 6 | Tool Calls / Results | `#f97316` |
| Ghost | Remaining capacity | `#444444` (`_GHOST_COLOR`) |

### Affordance Inventory

| Affordance | ID | Source | Status |
|-----------|----|---------|---------|
| Meter creates canvas on first `create()` call | PD-10-AF-001 | `ContextMeterWidget.create()` | ✅ |
| Arc slices sized proportionally to token counts | PD-10-AF-002 | `ContextMeterWidget._draw_arcs()` | ✅ |
| Ghost arc shows remaining capacity | PD-10-AF-003 | `ContextMeterWidget._draw_arcs()` | ✅ |
| Border turns warning-orange at 80 % utilisation | PD-10-AF-004 | `ContextMeterWidget._risk_state()` | ✅ |
| Border turns critical-red at 100 % utilisation | PD-10-AF-005 | `ContextMeterWidget._risk_state()` | ✅ |
| `update()` is thread-safe via `after()` | PD-10-AF-006 | `ContextMeterWidget.update()` | ✅ |
| `max_tokens=0` does not crash | PD-10-AF-007 | `ContextMeterWidget._draw_arcs()` | ✅ |

### Test Mapping

| Affordance | Test file | Test class |
|-----------|-----------|------------|
| PD-10-AF-001..007 | `tests/test_context_meter_widget.py` | Module-level pytest tests |

### Related Specs

- **PD-12-AF-011** — ContextMeterWidget is re-hosted in `StatusTab`; all above affordances unchanged.
- **PD-12: ContextKeyWidget** — companion colour-key legend reading from the same `_BANDS` constant.
- **PD-14: ContextPanelWidget** — management surface that the meter visualises; click-to-navigate links meter bands to panel rows.

### Band Source Role Mapping

Each arc segment corresponds to one or more `MessageRole` values from `Context.messages`.

| Band | Label | `MessageRole`(s) / Source | Colour |
|------|-------|---------------------------|--------|
| 0 | Working Memory | `SYSTEM` message with `metadata["is_working_memory"] = True` (ARCH-03) | `#0d9488` teal |
| 1 | System Prompts | All other `SYSTEM` messages (planner, tool-use, classification prompts) | `#6366f1` indigo |
| 2 | User Prompts | `USER` | `#3b82f6` blue |
| 3 | Attachments | Enabled `Attachment` objects across all messages | `#f59e0b` amber |
| 4 | Thinking | `THINKING` | `#a855f7` purple |
| 5 | Agent Response | `ASSISTANT` | `#22c55e` green |
| 6 | Tool Calls / Results | `TOOL_CALL` + `TOOL_RESULT` | `#f97316` orange |
| Ghost | Remaining capacity | — (ghost arc, not a role) | `#444444` dim |

> **Note (ARCH-03)**: Working Memory is injected as an ordinary `SYSTEM` message and is not yet separately tagged. Separating band 0 from band 1 requires setting `metadata["is_working_memory"] = True` in `_build_shared_context()`.

### Requirements Baseline

Baseline requirements from the original design:

| Code | Requirement | Status |
|------|-------------|--------|
| REQ-01 | Display a colour-band donut showing how much of the context window is consumed | ✅ Implemented |
| REQ-02 | Distinguish token consumption by category (WM, Attachments, User, System, Thinking, Agent, Tools) | ✅ Implemented |
| REQ-03 | Each arc is proportional to its share of `max_tokens` | ✅ Implemented |
| REQ-04 | ~~Right of text input, above Submit~~ → **Superseded**: hosted in StatusTab (PD-12) | Relocated |
| REQ-05 | Redraw when user submits a prompt | Deferred (PD-14 trigger) |
| REQ-06 | Redraw when context element enabled/disabled | Deferred (PD-14 trigger) |
| REQ-07 | Redraw after agent streaming finishes (`DONE` chunk) | Deferred |
| REQ-08 | Percentages use actual model context window, not a constant | ✅ Implemented (PRE-02) |
| REQ-09 | Band colours represent data type only; stable across risk levels | ✅ Implemented |
| REQ-10 | Capacity risk shown via border, not by re-colouring bands | ✅ Implemented |
| REQ-11 | Colour-blind-safe redundancy (text, border weight, tooltip) | ✅ Implemented |

### Token Counting Strategy

| Code | Strategy | Accuracy | Selected |
|------|----------|----------|---------|
| TOK-01 | `len(content) // 4` | ±30–50% | — |
| TOK-02 | Model-family char/token ratios (Llama ≈ 3.5, Mistral ≈ 3.8, default ≈ 4.0) | ±15–25% | **Current default** |
| TOK-03 | Ollama `/api/tokenize` endpoint | Exact | Follow-on (no extra dep) |
| TOK-04 | `tiktoken` | Exact for OpenAI models only | Rejected (wrong for Ollama) |

Upgrade path: TOK-02 → TOK-03 via an `ITokenizer` interface. See `docs/archive/agentx-agentix-integration/04_IMPROVEMENT_SUGGESTIONS.md §1.3`.

### Enrichment Backlog (Unimplemented)

| Code | Description |
|------|-------------|
| ENH-02 | Remaining-capacity ghost band always visible in arc |
| ENH-03 | Overflow hatching extends beyond 100 % boundary |
| ENH-05 | Token label `N / max_tokens tokens` alongside the donut |
| ENH-06 | Hover tooltips on arc bands: `Role · ~N tokens · X% · M messages` |
| ENH-07 | Pending-input preview arc (live keystroke estimate; debounced ~400 ms) |
| ENH-08 | Trim-warning badge at ≥ 90 % |
| ENH-09 | Click-to-navigate: clicking a band scrolls Context Panel to first matching message |
| ENH-10 | Post-completion token calibration from `prompt_eval_count` response field |

### Open Questions

| Code | Question |
|------|----------|
| Q-01 | Should disabled messages (REQ-06) trigger a meter redraw even though they contribute zero tokens? |
| Q-03 | Is ENH-07 (pending-input preview) desirable, or does per-keystroke computation introduce lag? Should recompute be debounced at ~400 ms? |
| Q-08 | Should `_max_tokens` cache be explicitly invalidated on model change, or should the meter always re-query on each redraw? |
| Q-09 | Should an `on_model_change` hook in `ModelSelector` trigger a meter redraw so the denominator updates immediately? |

---

## PD-13: ToolPanel

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

---

## PD-12: StatusTab

**Class**: `StatusTab` (`src/agentx/gui/status_tab.py`) — _to be created_
**Position**: First tab of `SidePanel.system_notebook` (before Session / Files / Settings)
**Purpose**: Real-time visibility into the current prompt-reply cycle — active phase,
elapsed time per step, and context window utilisation with a colour-key legend.
The tab auto-activates when the user submits a prompt and updates in-the-blind
(all widget state is written regardless of tab visibility; only paint is deferred).

> **Moved from**:
>
> - `ContextMeterWidget` (PD-10) — donut canvas formerly hosted in `InputPanel`
>   right-column (`relx=0.92, relwidth=0.07, relheight=0.24`). The donut and its
>   colour-key legend are now the upper section of this tab.
> - `InputPanel` (PD-02) — `user_break` interrupt button (`relx=0.92, rely=0.51`)
>   removed from input panel right-column and re-hosted here as the large `Interrupt`
>   button below the phase stepper.
> - `InputPanel` right-column freed: `user_submit` button shrinks to a slim strip
>   (`relx=0.96, relwidth=0.04`); text area expands to fill `relwidth=0.96`.

---

### Layout

```
┌────────────────────────── Status Tab ───────────────────────────────────┐
│                                                                         │
│  ┌──── Context Window ─────────────────────────────────────────────┐   │
│  │                                                                  │   │
│  │  ┌─── Colour Key ──────────────┐  ┌─── Donut ─────────────────┐ │   │
│  │  │  ● Working Memory  #0d9488  │  │                           │ │   │
│  │  │  ● System Prompts  #6366f1  │  │        ┌─────┐            │ │   │
│  │  │  ● User Prompts    #3b82f6  │  │     ╱         ╲           │ │   │
│  │  │  ● Attachments     #f59e0b  │  │    │     NN%   │          │ │   │
│  │  │  ● Thinking        #a855f7  │  │     ╲         ╱           │ │   │
│  │  │  ● Agent Response  #22c55e  │  │        └─────┘            │ │   │
│  │  │  ● Tool Calls      #f97316  │  │                           │ │   │
│  │  │  ░ Remaining        #444444 │  │  [risk border: gray/red]  │ │   │
│  │  └─────────────────────────────┘  └───────────────────────────┘ │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌──── Prompt Cycle Status ────────────────────────────────────────┐   │
│  │                                                                  │   │
│  │  ☐  🤔 Classify       00:00:00                                  │   │
│  │  ↻  💭 Think          00:00:07  ← running (spinner)             │   │
│  │  ○  🔧 Tool: <name>   --:--:--  ← pending                      │   │
│  │  ○  ✍️  Respond        --:--:--  ← pending                      │   │
│  │                                                                  │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │              ⛔  Interrupt  (Ctrl+Space)                         │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

### Sub-widgets

| Widget | Class | Purpose |
|--------|-------|---------|
| `ContextWindowSection` | `tk.LabelFrame` | Container holding colour-key and donut side-by-side |
| `ContextKeyWidget` | `tk.Frame` (rows of coloured circles + labels) | Colour-key legend for donut bands |
| `ContextMeterWidget` (relocated) | `tk.Canvas` | Donut chart — same class as before, re-parented |
| `PhaseStepperWidget` | `tk.Frame` | Vertical list of phase rows with status icon + label + elapsed timer |
| `PhaseRow` | internal row frame | One row per phase; holds status icon, emoji label, elapsed `tk.Label` |
| `InterruptButton` | `tk.Button` | Large full-width interrupt button; enabled only during streaming |

---

### Context Window Section (upper)

Two child frames sit side-by-side inside a `tk.LabelFrame` labelled
`"Context Window"`:

```
ContextWindowSection (tk.LabelFrame, pack fill=X, pady=4)
  ├── ContextKeyWidget  (pack side=LEFT, fill=Y, padx=8)
  └── ContextMeterWidget canvas (pack side=LEFT, fill=BOTH, expand=True)
```

#### ContextKeyWidget — Colour Key

Renders one row per band in band-definition order. Each row contains:

- A small square `tk.Canvas` (14×14 px) filled with the band colour
- A `tk.Label` with the band's display name

Bands are read from the same `_BANDS` constant in `context_meter_widget.py`
(or a shared constant imported by both) so the key is never out of sync with
the donut.  The ghost-arc (remaining capacity) is the last row, using
`_GHOST_COLOR` (`#444444`).

```
ContextKeyWidget (tk.Frame)
  ├── row (tk.Frame)  swatch(Canvas 14×14 #0d9488)  Label "Working Memory"
  ├── row (tk.Frame)  swatch(Canvas 14×14 #6366f1)  Label "System Prompts"
  ├── row (tk.Frame)  swatch(Canvas 14×14 #3b82f6)  Label "User Prompts"
  ├── row (tk.Frame)  swatch(Canvas 14×14 #f59e0b)  Label "Attachments"
  ├── row (tk.Frame)  swatch(Canvas 14×14 #a855f7)  Label "Thinking"
  ├── row (tk.Frame)  swatch(Canvas 14×14 #22c55e)  Label "Agent Response"
  ├── row (tk.Frame)  swatch(Canvas 14×14 #f97316)  Label "Tool Calls / Results"
  └── row (tk.Frame)  swatch(Canvas 14×14 #444444)  Label "Remaining"
```

#### ContextMeterWidget (re-parented)

The existing `ContextMeterWidget` class is unchanged. Its `create(parent)` call
is moved from `InputPanel` to `StatusTab`.  All existing affordances
(PD-10-AF-001 through PD-10-AF-007) and the tooltip hover behaviour are
preserved; only the host frame changes.

> **Spec cross-reference**: PD-10 (ContextMeterWidget) — relocation only; no
> behavioural change to the donut itself.

---

### Phase Stepper Section (middle)

A `tk.LabelFrame` labelled `"Prompt Cycle"` containing a `PhaseStepperWidget`.

#### Phase Row Structure

```
PhaseStepperWidget (tk.Frame, pack fill=BOTH, expand=True)
  └── PhaseRow × N  (one per phase step)
        ├── status_icon  (tk.Label, width=2)   — see Status Icon table
        ├── phase_label  (tk.Label)             — emoji + phase name
        └── elapsed_label (tk.Label, width=8)  — "HH:MM:SS" or "--:--:--"
```

#### Phase Steps (in display order)

| Step key | Emoji | Label |
|----------|-------|-------|
| `classify` | 🤔 | Classify |
| `think` | 💭 | Think |
| `tool` | 🔧 | Tool: `<name>` _(name injected at runtime)_ |
| `respond` | ✍️ | Respond |

> Tool step label is dynamic: once a tool call begins the step label updates to
> `🔧 Tool: read_file` (or whichever tool is active).  If multiple tool rounds
> occur, the same row is reused with the latest tool name.

#### Status Icons

| Icon | State | Meaning |
|------|-------|---------|
| `○` | `PENDING` | Not yet reached this step |
| `↻` | `RUNNING` | Currently executing; elapsed timer ticking |
| `✓` | `DONE` | Completed successfully |
| `✗` | `FAILED` | Ended with an error |

The icon is a `tk.Label` updated by `PhaseRow.set_state(state)`.

#### Elapsed Timer

- GUI StatusTab format: `HH:MM:SS` while running; `--:--:--` while pending;
  frozen at final elapsed when `DONE` or `FAILED`.
- Hybrid/TUI prompt-cycle format: `HH:MM:SS.mmm` while running; `--:--:--.---`
  while pending; frozen at final elapsed when `DONE` or `FAILED`.
- Implementation: `PhaseRow` records `start_time: float = time.monotonic()` when
  state transitions to `RUNNING`.  `StatusTab` drives a single `after(1000, …)`
  tick loop that calls `PhaseRow.tick()` on all `RUNNING` rows.  The tick loop
  is started by `PhaseStepperWidget.start_tick()` and cancelled by
  `PhaseStepperWidget.stop_tick()`.
- Off-screen safety: `after()` callbacks fire regardless of tab visibility.
  `tick()` updates the `tk.Label` text; the paint simply queues if the tab is
  not the active view, flushing instantly when the tab is selected.

---

### Interrupt Button (bottom)

A `tk.Button` spanning the full tab width:

```
interrupt_btn  text="⛔  Interrupt  (Ctrl+Space)"
               state=DISABLED when not streaming
               state=NORMAL   when streaming
               command → on_interrupt callback (same callback as old user_break)
```

The `Ctrl+Space` global binding in `InputPanel` is **moved** to `StatusTab`
(still bound on `root` so it works regardless of focus).

> **Spec cross-reference**: PD-02-AF-004 (`user_break` button) — this affordance
> is **relocated** to PD-12.  PD-02-AF-004 status changes to `🔁 Relocated →
> PD-12-AF-003`.

---

### Tab Navigation Behaviour

| Trigger | Action |
|---------|--------|
| User submits prompt | `system_notebook.select(status_tab_index)` — switches to Status tab |
| Stream ends | Tab remains on Status (user may want to review elapsed times) |
| User manually switches tab | No forced return; updates continue in-the-blind |

---

### Affordance Inventory

#### PD-12-AF-001 — Status tab is the first tab in the system notebook

**Source**: `SidePanel.create()` — Status tab added before Session tab
**Purpose**: Ensures the tab order is: Status → Session → Files → Settings

```gherkin
GIVEN SidePanel.create() has been called
WHEN  we query system_notebook tab names
THEN  the first tab text is "Status"
 AND  the second tab text is "Session"
```

#### PD-12-AF-002 — Auto-switch to Status tab on prompt submit

**Source**: `StreamingController._on_stream_start()` (calls `gui.show_status_tab()`)
**Purpose**: Gives the user immediate visual feedback that the system received their input

```gherkin
GIVEN the user is on any tab in the system notebook
WHEN  the user submits a prompt
THEN  the system notebook switches to the Status tab
 AND  the Phase Stepper resets all rows to PENDING state
```

#### PD-12-AF-003 — Interrupt button enables/disables with streaming state

**Source**: `StatusTab.set_streaming_state(is_streaming)` (mirrors old PD-02-AF-004)
**Purpose**: Interrupt is only actionable when a stream is running

> **Relocated from**: PD-02-AF-004 (`user_break` button in InputPanel)

```gherkin
GIVEN streaming is not active
WHEN  StatusTab.set_streaming_state(False) is called
THEN  interrupt_btn state is DISABLED

GIVEN streaming is active
WHEN  StatusTab.set_streaming_state(True) is called
THEN  interrupt_btn state is NORMAL
```

#### PD-12-AF-004 — Interrupt button invokes on_interrupt callback

**Source**: `StatusTab` `interrupt_btn` command binding
**Purpose**: Stops the active stream via the same callback as the old Break button

```gherkin
GIVEN streaming is active and interrupt_btn is NORMAL
WHEN  the user clicks interrupt_btn
THEN  the on_interrupt callback is called exactly once

GIVEN streaming is active
WHEN  the user presses Ctrl+Space
THEN  the on_interrupt callback is called exactly once
```

#### PD-12-AF-005 — Phase rows reset at stream start

**Source**: `StatusTab.reset()` called from `StreamingController._on_stream_start()`
**Purpose**: Each new prompt cycle starts with a clean slate

```gherkin
GIVEN the stepper has rows with DONE state from a previous cycle
WHEN  StatusTab.reset() is called
THEN  all phase rows return to PENDING state
 AND  all elapsed labels show "--:--:--"
 AND  all status icons show "○"
```

#### PD-12-AF-006 — Phase row transitions to RUNNING and starts timer

**Source**: `StatusTab.set_phase(step_key, state="RUNNING", tool_name=None)`
**Purpose**: Marks a phase as in-progress and begins elapsed time display

```gherkin
GIVEN the "classify" row is in PENDING state
WHEN  set_phase("classify", "RUNNING") is called
THEN  the classify row status icon becomes "↻"
 AND  the classify elapsed label shows "00:00:00"
 AND  after ~1 second the elapsed label shows "00:00:01"
```

#### PD-12-AF-007 — Phase row transitions to DONE and freezes timer

**Source**: `StatusTab.set_phase(step_key, state="DONE")`
**Purpose**: Records final elapsed time for a completed step

```gherkin
GIVEN the "classify" row has been RUNNING for ~3 seconds
WHEN  set_phase("classify", "DONE") is called
THEN  the classify row status icon becomes "✓"
 AND  the elapsed label is frozen at the final elapsed value
 AND  the label does not change after a further tick
```

#### PD-12-AF-008 — Phase row transitions to FAILED

**Source**: `StatusTab.set_phase(step_key, state="FAILED")`
**Purpose**: Distinguishes error-terminated steps from successful ones

```gherkin
GIVEN the "think" row is RUNNING
WHEN  set_phase("think", "FAILED") is called
THEN  the think row status icon becomes "✗"
 AND  the elapsed label is frozen at the failure time
```

#### PD-12-AF-009 — Tool step label updates with active tool name

**Source**: `StatusTab.set_phase("tool", "RUNNING", tool_name="read_file")`
**Purpose**: Identifies which tool is running without opening the chat panel

```gherkin
GIVEN the tool row is in PENDING state
WHEN  set_phase("tool", "RUNNING", tool_name="read_file") is called
THEN  the tool row label shows "🔧 Tool: read_file"
```

#### PD-12-AF-010 — Colour-key legend rows match donut bands in order

**Source**: `ContextKeyWidget` reads from the same `_BANDS` constant as `ContextMeterWidget`
**Purpose**: Key and donut are guaranteed to stay in sync

```gherkin
GIVEN ContextKeyWidget has been created
WHEN  we enumerate the key rows
THEN  the row count equals len(_BANDS) + 1 (for the ghost/remaining row)
 AND  each row swatch colour matches the corresponding band colour in _BANDS order
 AND  the final row swatch colour is _GHOST_COLOR
```

#### PD-12-AF-011 — ContextMeterWidget hosted in StatusTab (relocation)

**Source**: `StatusTab.create()` calls `ContextMeterWidget.create(context_frame)`
**Purpose**: Donut retains all PD-10 affordances under new host; no functional regression

```gherkin
GIVEN StatusTab has been created
WHEN  we inspect the ContextWindowSection
THEN  a ContextMeterWidget canvas is present inside the section frame
 AND  calling update(max_tokens, breakdown) redraws the donut (same as PD-10-AF-001)
```

---

### State Fields

| Attribute | Type | Description |
|-----------|------|-------------|
| `_phase_rows` | `dict[str, PhaseRow]` | Keyed by step key; created in `create()` |
| `_tick_id` | `str \| None` | Return value of last `after()` call; `None` when idle |
| `_on_interrupt` | `Callable[[], None]` | Callback; same reference passed to old `user_break` |
| `_context_meter` | `ContextMeterWidget` | Donut instance (relocated from InputPanel) |
| `_context_key` | `ContextKeyWidget` | Colour-key instance |

---

### IGUIManager Interface Additions

The following methods are added to `IGUIManager` (and implemented in `GUIManager`):

| Method | Purpose |
|--------|---------|
| `show_status_tab()` | Switch `system_notebook` to the Status tab |
| `set_status_phase(step_key, state, tool_name=None)` | Delegate to `StatusTab.set_phase()` |
| `reset_status_tab()` | Delegate to `StatusTab.reset()` |

`set_streaming_state(is_streaming)` is extended to also call
`StatusTab.set_streaming_state(is_streaming)`.

---

### Cross-References

| Spec | Change |
|------|--------|
| PD-02 InputPanel | `user_break` button removed from right-column; `user_submit` resized to slim strip (`relx=0.96, relwidth=0.04`); text area expands to `relwidth=0.96`; `Ctrl+Space` binding migrated to `StatusTab`. See PD-02-AF-004 → **Relocated to PD-12-AF-003**. |
| PD-10 ContextMeterWidget | `create()` call moves from `InputPanel` to `StatusTab`; all PD-10-AF-001..007 affordances unchanged. |
| PD-03 SidePanel | Status tab frame created and inserted at index 0 of `system_notebook` before Session. |
| `StreamingController` | `_on_stream_start()` gains `gui.show_status_tab()` + `gui.reset_status_tab()` calls. `_display_*` helpers gain `gui.set_status_phase()` calls at each phase transition. |

---

### Test Mapping

| Affordance | Test file | Test class/name |
|-----------|-----------|-----------------|
| PD-12-AF-001 | `test_status_tab.py` | `TestStatusTabOrder` — _📝 spec only_ |
| PD-12-AF-002 | `test_status_tab.py` | `TestStatusTabAutoSwitch` — _📝 spec only_ |
| PD-12-AF-003 | `test_status_tab.py` | `TestInterruptButtonState` — _📝 spec only_ |
| PD-12-AF-004 | `test_status_tab.py` | `TestInterruptButtonCallback` — _📝 spec only_ |
| PD-12-AF-005 | `test_status_tab.py` | `TestPhaseStepperReset` — _📝 spec only_ |
| PD-12-AF-006 | `test_status_tab.py` | `TestPhaseRowRunning` — _📝 spec only_ |
| PD-12-AF-007 | `test_status_tab.py` | `TestPhaseRowDone` — _📝 spec only_ |
| PD-12-AF-008 | `test_status_tab.py` | `TestPhaseRowFailed` — _📝 spec only_ |
| PD-12-AF-009 | `test_status_tab.py` | `TestToolStepLabel` — _📝 spec only_ |
| PD-12-AF-010 | `test_status_tab.py` | `TestContextKeyLegend` — _📝 spec only_ |
| PD-12-AF-011 | `test_status_tab.py` | `TestContextMeterRelocation` — _📝 spec only_ |

---

## PD-14: ContextPanelWidget

**Class**: `ContextPanelWidget` (planned: `src/agentx/gui/context_panel_widget.py`)
**Position**: Permanent tab ("Context") in the SidePanel display notebook — always visible, never modal.
**Purpose**: Management surface for all LLM context elements. The `ContextMeterWidget` (PD-10) shows _what_; the Context Panel shows _why_ and lets the user act: enable/disable messages, synthesise, clone/edit inline.
**Status**: 📝 Spec only — not yet implemented.

### Layout

```
SidePanel — "Context" tab
│
├── Selection Action Bar (hidden when selection = 0)
│     ┌────────────────────────────────────────────────────┐
│     │  N selected   [Disable]  [Synthesize]  [Clear]     │
│     └────────────────────────────────────────────────────┘
│
└── Scrollable row list  (tk.Text + embedded tk.Frame rows)
      ┌─[ ]──[▶]────────────────────────────────────[●]─┐
      │  sel  exp   Role icon · Name · ~N tok · X%       │
      │             Content preview (≤ 80 chars)         │
      │  ┌───────────────────────────────────────────┐   │  ← expanded only
      │  │  editable tk.Text widget                  │   │
      │  └───────────────────────────────────────────┘   │
      │  [Save]  [Discard]                                │  ← expanded only
      └───────────────────────────────────────────────────┘
```

- `[ ]` select checkbox → shows/hides Selection Action Bar
- `[▶]`/`[▼]` expand toggle → at most one row expanded at a time
- `[●]`/`[○]` enable/disable toggle → right-aligned

### Row Visual States (priority order)

| Priority | State | Visual treatment |
|----------|-------|-----------------|
| 1 | Streaming | Animated pulse border; all controls disabled |
| 2 | Expanded (edit active) | Elevated background; editor + [Save]/[Discard] shown; submit blocked globally |
| 3 | Selected | Selection-tint overlay; checkbox checked |
| 4a | Synthesis source | Greyed + italic + annotation `→ synthesised as #<id>`; toggle locked |
| 4b | Clone source | Greyed + italic + annotation `→ cloned as #<id>`; toggle locked |
| 5 | Disabled (user-toggled) | Greyed; token count struck through; toggle shows `○` |
| — | Enabled (normal) | Default appearance; toggle shows `●` |

### Live-Update Behaviour

| Phase | Panel behaviour |
|-------|----------------|
| Idle | Full affordances active |
| Streaming | List frozen at last-computed state; edits blocked; action bar disabled |
| `DONE` chunk received | Atomic rebuild from updated `Context.messages`; meter recomputed simultaneously |
| Inline edit active | Edit row open; all other rows read-only; submit button blocked (ARCH-14) |

### Synthesise Flow (ENH-14)

```
User selects N rows → [Synthesize]
  → "Synthesising N items…" spinner in action bar
  → Background LLM call: compression prompt + selected message contents
  → On completion:
      a. New SYNTHESIS-role Message (enabled=True, synthesis_of=[id1,…])
      b. Source rows: enabled=False; toggle locked; annotation added
      c. Synthesis row inserted at position of last source in list
      d. Selection cleared; action bar hidden; meter recomputed
```

Open (Q-11): whether step (b) is gated by a preview/approval step before originals are disabled.

### Clone / Edit Flow (ENH-15)

```
User clicks [▶] on any row → row expands; submit disabled
  [Save]:    new Message (same role, edited content, cloned_from=<original_id>)
             original: enabled=False; toggle locked; annotation added
             clone row appears below original; submit re-enabled; meter recomputed
  [Discard]: editor content abandoned; row collapses; no state change; submit re-enabled
```

### System-Injected Rows

Working Memory and system prompt files are not `Context.messages` objects — they are generated at send-time. Both appear as synthetic **read-only** rows with a `⚙` icon:

| Synthetic row | Disable mechanism | Edit mechanism |
|---------------|-------------------|----------------|
| ⚙ Working Memory | `session_config["suppress_working_memory"] = True` | Link to WM management UI |
| ⚙ `<prompt_file>.md` | `session_config["suppressed_system_prompts"].add(filename)` | Creates a `SYSTEM`-role Message override in `Context.messages`; replaces synthetic row |

Requires ARCH-03 (tagging WM SYSTEM message with `metadata["is_working_memory"]`).

### Architecture Notes

| Code | Item |
|------|------|
| ARCH-11 | `ContextPanelWidget` uses `tk.Text` as a scrollable container; per-row `tk.Frame` embedded via `window_create`. |
| ARCH-12 | Six-dimension row state bitmask; priority rules as in row-state table above. |
| ARCH-13 | Panel list frozen during streaming; atomic rebuild on `DONE` chunk. |
| ARCH-14 | `edit_active` flag on widget → disables submit button + applies amber border to input area. |

### Affordance Inventory

| Affordance | ID | Source | Status |
|-----------|-----|---------|--------|
| Row enable/disable toggle | PD-14-AF-001 | `ContextPanelWidget._on_toggle()` | 📝 |
| Row expand/collapse | PD-14-AF-002 | `ContextPanelWidget._on_expand()` | 📝 |
| Inline edit Save creates clone | PD-14-AF-003 | `ContextPanelWidget._on_save()` | 📝 |
| Inline edit Discard reverts row | PD-14-AF-004 | `ContextPanelWidget._on_discard()` | 📝 |
| Row select checkbox shows action bar | PD-14-AF-005 | `ContextPanelWidget._on_select()` | 📝 |
| [Disable] disables all selected rows | PD-14-AF-006 | `ContextPanelWidget._on_bulk_disable()` | 📝 |
| [Synthesize] runs synthesis LLM call | PD-14-AF-007 | `ContextPanelWidget._on_synthesize()` | 📝 |
| [Clear] deselects all rows | PD-14-AF-008 | `ContextPanelWidget._on_clear_selection()` | 📝 |
| List frozen during streaming | PD-14-AF-009 | `ContextPanelWidget.freeze()` | 📝 |
| Atomic rebuild on DONE | PD-14-AF-010 | `ContextPanelWidget.rebuild()` | 📝 |
| Click-to-navigate (ENH-09): band click scrolls panel | PD-14-AF-011 | `ContextPanelWidget.scroll_to_role()` | 📝 |
| WM row disable sets session flag | PD-14-AF-012 | `ContextPanelWidget._on_toggle_wm()` | 📝 |
| System-prompt row disable sets session flag | PD-14-AF-013 | `ContextPanelWidget._on_toggle_sysprompt()` | 📝 |

### Open Questions

| Code | Question |
|------|----------|
| Q-11 | Should synthesis (ENH-14) require a preview/approval step before originals are disabled? |
| Q-12 | Where does a synthesised message appear in the list: (a) replace last source position, (b) bottom of selection range, or (c) dedicated Synthesis section? |
| Q-13 | Should the synthesis compression instruction be a fixed internal prompt or user-editable per invocation? |
