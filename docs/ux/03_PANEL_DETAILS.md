# AgentX — Panel Details

> **⚠️ Architecture migration (2026-06-26).** These per-panel specs (PD-01…PD-17,
> 112 affordances) describe the prior single-window split-pane GUI. AgentX is now a
> **client-server** app: the chat surface has **two panels (output + input)** and the
> former tabbed "system" panel becomes **multiple independent, separately launchable
> surfaces**. Affordance IDs are preserved where possible, but each PD's surface
> geometry and host change during M2. Treat the geometry below as **legacy** until the
> M2 migration re-homes each affordance to its standalone surface. See
> [`../architecture/00_ARCHITECTURE_RECONCILIATION.md`](../architecture/00_ARCHITECTURE_RECONCILIATION.md).

_Last updated: 2026-06-25 (v0.79.2)_

Detailed affordance specifications for each surface/widget and the hybrid
runtime surfaces that need UX traceability. Each section documents the
surface's purpose, all user-visible controls, and the callback wiring to
session logic.

Authoritative contract rule:

- This document defines required user-facing affordances independent of delivery
  technology.
- Implementations may be GUI, TUI, or hybrid, but must satisfy these UX
  behaviors without weakening the contract.

Each section should follow the component cut-sheet standard in
[04_COMPONENT_CUT_SHEET_TEMPLATE.md](04_COMPONENT_CUT_SHEET_TEMPLATE.md).

---

## PD-01: OutputSurface

**Purpose**: Left ~66% of the window; primary output and conversation history
surface. Streams agent responses, classification markers, thinking blocks, and
tool calls. Also hosts plan tabs.

### Tabs

| Tab | Created | Contents |
|-----|---------|----------|
| `Chat` | Always present | Streaming message entries (see message types below) |
| `Plan: <name>` | Added per plan | `PlanView` for that plan |

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
| Plan tab click | Navigate to plan tree | Tab selection |
| Scroll | Vertical scroll of chat history | Mouse wheel / scrollbar |
| Startup notice block | Show friendly log-file locations on startup | Orchestrator startup hook |

### Collapse / Expand Behaviour

Collapsing the user turn hides the response children (classification,
thinking, tool, assistant entries). Expanding re-shows them below the user
entry. The children always appear below the user entry regardless of how many
collapse/expand cycles occur.

### Affordance: PD-01-AF-009 — Startup log-location notice in output window

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

The output panel is read-only; the only clipboard action available is **Copy**.
The right-click popup uses a Wayland-aware fallback popup approach.
A fresh popup is created for every invocation; stale surfaces are destroyed before
each new popup is shown. The "Copy" item is always visible; if no text is selected at
the moment of right-click, "Copy" copies nothing (same as Ctrl-C with no selection).

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

---

## PD-02: InputSurface

**Purpose**: Bottom ~23% of the window; captures user text input and file
attachments. Contains an attachment bar above the text entry area.

### Layout

```
┌──────────────────────────── AgentX main window ─────────────────────────────┐
│                                                                              │
│  [OutputSurface ~66%]                [SystemSurface ~34%]                   │
│                                                                              │
├──────────────────────────────────────────────────────────────────────────────┤
│  Attachment bar (thin strip)                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐ │
│  │ [📁 file1.py ✓]  [📜 old.py (history) ✓]                              │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
├──────────────────────────────────────────────────────────────────────────────┤
│  User input area                                                             │
│  ┌──────────────────────────────────────────────────┬────┬────┬──────────┐  │
│  │ Text input (~90% width)                          │[⏎] │[❌]│ context  │  │
│  │ (multi-line input, wraps at word boundaries)     │    │    │  meter   │  │
│  │                                           ▲ scrollbar │    │ (donut)  │  │
│  └──────────────────────────────────────────────────┴────┴────┴──────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Behaviour Inventory

| ID | Affordance | Trigger | Outcome |
|----|-----------|---------|---------|
| PD-02-AF-001 | Enter key submits | `Ctrl+Enter` binding | Invokes submit action |
| PD-02-AF-002 | Shift+Enter inserts newline | `Shift+Enter` binding | Inserts `\n` into text widget |
| PD-02-AF-003 | Send disabled during streaming | `set_streaming_state(True)` | Submit button disabled |
| PD-02-AF-004 | Stop enabled during streaming | `set_streaming_state(True)` | ⚠️ **Relocated to PD-12-AF-003** — stop button moves to StatusTab; callback unchanged |
| PD-02-AF-005 | Chip renders with filename | `update_attachment_bar([info], [])` | Chip shows `display_name`; current-turn: `📁` icon, bright bg; history: `📜` icon + `" (history)"` suffix, grey bg |
| PD-02-AF-006 | Toggle chip calls callback | User clicks checkbox | `on_attachment_toggle(attachment_id, bool)` called with new enabled state |
| PD-02-AF-007 | Rebuild clears old chips | `update_attachment_bar([], [])` | All previous chip frames destroyed |
| PD-02-AF-008 | Right-click opens context popup on input widget | `Button-3` binding | Wayland-safe popup appears with conditional "Copy" and/or "Paste" items |
| PD-02-AF-009 | Input context menu shows "Copy" only when text is selected | Popup construction | "Copy" item present iff text selection exists at time of right-click |
| PD-02-AF-010 | Input context menu shows "Paste" only when clipboard is non-empty | Popup construction | "Paste" item present iff clipboard is non-empty |
| PD-02-AF-011 | "Copy" in input context menu copies selected text | User clicks "Copy" in popup | Selected text placed on system clipboard; popup dismissed |
| PD-02-AF-012 | "Paste" in input context menu replaces selection / inserts at cursor | User clicks "Paste" in popup | If selection exists, selected text deleted first; then clipboard content inserted at cursor; popup dismissed |

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

### Prompt history seeding (TUI-native)

> **TUI-native affordances** (no Tkinter precedent). The input panel keeps a
> readline-style history of prompts submitted during the current run. `↑`/`↓`
> *seed* the editable buffer with a prior prompt — they copy it in for reuse, they
> do not edit the original. The user may submit the seed as-is with Enter or edit it
> first; either way a **new** prompt is created. Source: `internal/surfaces/input`,
> wired through `internal/surfaces/chat`. Scope is the current process run only
> (in-memory, captured at submit time); persisting history across a session reload
> is a follow-up.

| ID | Affordance | Trigger | Outcome |
|----|-----------|---------|---------|
| PD-02-AF-013 | History prev seeds the input | `↑` while input-focused, idle | Buffer is replaced with the previous (older) submitted prompt; the in-progress draft is stashed on first step back |
| PD-02-AF-014 | History next walks toward the present | `↓` while input-focused, idle | Buffer is replaced with the next (newer) prompt; stepping past the newest restores the stashed draft |
| PD-02-AF-015 | Boundary flash | `↑` past the oldest prompt, or `↓` past the restored draft | The buffer does not change; the input frame flashes briefly to signal the boundary |
| PD-02-AF-016 | Esc,Esc clears a seed | `Esc` then `Esc` while idle with a seed active | The buffer returns to empty (unseeded) and history navigation resets to the present |

```gherkin
# PD-02-AF-013 — up seeds the previous prompt
GIVEN the input has submitted prompts "first" then "second"
  AND the input buffer is empty
WHEN  the user presses up
THEN  the input value is "second"
WHEN  the user presses up
THEN  the input value is "first"

# PD-02-AF-013 — the in-progress draft is stashed on the first step back
GIVEN the input has submitted prompt "first"
  AND the user has typed "draft" without submitting
WHEN  the user presses up
THEN  the input value is "first"

# PD-02-AF-014 — down walks back toward the present and restores the draft
GIVEN the input has submitted prompts "first" then "second"
  AND the user has typed "draft" without submitting
  AND the user presses up twice
WHEN  the user presses down
THEN  the input value is "second"
WHEN  the user presses down
THEN  the input value is "draft"

# PD-02-AF-015 — up at the oldest prompt flashes and does not change the buffer
GIVEN the input has submitted prompt "first"
  AND the user has pressed up once so the value is "first"
WHEN  the user presses up
THEN  the input value is "first"
  AND the input reports a history boundary

# PD-02-AF-015 — down past the draft flashes and does not change the buffer
GIVEN the input has submitted prompt "first"
  AND the input buffer is empty
WHEN  the user presses down
THEN  the input value is ""
  AND the input reports a history boundary

# PD-02-AF-016 — Esc,Esc returns to an empty, unseeded prompt
GIVEN the input has submitted prompt "first"
  AND the user has pressed up so the value is "first"
WHEN  the user presses esc then esc
THEN  the input value is ""

# PD-02-AF-013 — submitting appends the submitted text to history
GIVEN the input has submitted prompt "first"
  AND the user seeds "first" and edits it to "first revised" and submits
WHEN  the user presses up
THEN  the input value is "first revised"
```

### Cursor & line editing (TUI-native)

> **TUI-native affordances** (no Tkinter precedent). When the input panel is
> focused it shows a text cursor and edits relative to it: typing inserts at the
> cursor, Backspace deletes the rune before it, and a **soft-newline** inserts a
> `\n` at it. The soft-newline binding is `Alt+Enter` (or `Ctrl+J`) on any
> terminal, plus `Shift+Enter` on terminals that disambiguate modified keys; an
> empty input shows a dim hint advertising whichever key applies (detected via
> `tea.KeyboardEnhancementsMsg`). Movement keys follow readline. The buffer is treated as one logical line
> (embedded newlines are ordinary characters), so Ctrl-A/Ctrl-E address the whole
> buffer. A *word* is a maximal run of non-space runes; Alt-B lands on the start of
> the prior word and Alt-F on the start of the next word. Source:
> `internal/surfaces/input`. The cursor position is exposed as a rune index via
> `Cursor()` for testing; it is rendered as a reverse-video cell (including a
> virtual cell at end-of-line) and is shown only while the panel is focused.
>
> **Multiplexer-safe aliases.** zellij binds `Alt-f` to toggle floating panes and
> intercepts it before the app sees it, so word motion is also bound to
> `Ctrl+←`/`Ctrl+→`, which zellij does not grab by default. Both bindings invoke
> the same back-word / forward-word movement.
>
> History seeding (PD-02-AF-013/AF-014) places the cursor at the end of the seeded
> text so the user can keep typing immediately.

| ID | Affordance | Trigger | Outcome |
|----|-----------|---------|---------|
| PD-02-AF-017 | Left moves the cursor back one rune | `←` while input-focused, idle | Cursor index decremented, floored at 0 |
| PD-02-AF-018 | Right moves the cursor forward one rune | `→` while input-focused, idle | Cursor index incremented, capped at the buffer length |
| PD-02-AF-019 | Ctrl-A jumps to the start of the buffer | `Ctrl+A` | Cursor index set to 0 |
| PD-02-AF-020 | Ctrl-E jumps to the end of the buffer | `Ctrl+E` | Cursor index set to the buffer length |
| PD-02-AF-021 | Jump to the start of the prior word | `Alt+B` or `Ctrl+←` | Cursor moves left over any spaces then over non-spaces, landing on the word start |
| PD-02-AF-022 | Jump to the start of the next word | `Alt+F` or `Ctrl+→` | Cursor moves right over the current word then over spaces, landing on the next word start |
| PD-02-AF-023 | Edits act at the cursor | typing / `Backspace` / `Shift+Enter` | Rune inserted at the cursor (cursor advances); Backspace deletes the rune before the cursor; newline inserted at the cursor |
| PD-02-AF-024 | The cursor is rendered while focused | panel focused | A reverse-video cell marks the cursor position; absent when the panel is blurred |

```gherkin
# PD-02-AF-023 — typing inserts at the cursor
GIVEN a focused input panel containing "ac" with the cursor at 1
WHEN  the user types "b"
THEN  the input value is "abc"
  AND the cursor is at 2

# PD-02-AF-023 — backspace deletes the rune before the cursor
GIVEN a focused input panel containing "abc" with the cursor at 2
WHEN  the user presses backspace
THEN  the input value is "ac"
  AND the cursor is at 1

# PD-02-AF-017 / AF-018 — left and right move one rune and clamp at the edges
GIVEN a focused input panel containing "ab" with the cursor at 2
WHEN  the user presses left
THEN  the cursor is at 1
WHEN  the user presses left
THEN  the cursor is at 0
WHEN  the user presses left
THEN  the cursor is at 0
WHEN  the user presses right
THEN  the cursor is at 1

# PD-02-AF-019 / AF-020 — Ctrl-A and Ctrl-E jump to the buffer ends
GIVEN a focused input panel containing "hello world" with the cursor at 5
WHEN  the user presses ctrl+a
THEN  the cursor is at 0
WHEN  the user presses ctrl+e
THEN  the cursor is at 11

# PD-02-AF-021 — Alt-B jumps to the start of the prior word
GIVEN a focused input panel containing "foo bar baz" with the cursor at 11
WHEN  the user presses alt+b
THEN  the cursor is at 8
WHEN  the user presses alt+b
THEN  the cursor is at 4

# PD-02-AF-022 — Alt-F jumps to the start of the next word
GIVEN a focused input panel containing "foo bar baz" with the cursor at 0
WHEN  the user presses alt+f
THEN  the cursor is at 4
WHEN  the user presses alt+f
THEN  the cursor is at 8

# PD-02-AF-021 / AF-022 — Ctrl+arrow aliases move by word (multiplexer-safe)
GIVEN a focused input panel containing "foo bar" with the cursor at 0
WHEN  the user presses ctrl+right
THEN  the cursor is at 4
WHEN  the user presses ctrl+left
THEN  the cursor is at 0

# PD-02-AF-013 — seeding a prompt leaves the cursor at the end
GIVEN the input has submitted prompt "hello"
WHEN  the user presses up
THEN  the input value is "hello"
  AND the cursor is at 5

# PD-02-AF-024 — the cursor is rendered only while focused
GIVEN a focused input panel containing "hi"
THEN  the rendered input shows a cursor cell
WHEN  the panel is blurred
THEN  the rendered input shows no cursor cell
```

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
| `Stop` enabled | Streaming in progress — ⚠️ **button relocated to StatusTab (PD-12); not present in InputSurface after PD-12 implementation** |
| `Stop` disabled | Not streaming |

> **PD-12 layout change**: When PD-12 `StatusTab` is implemented:
>
> - Stop button is removed from InputSurface right-column
> - ContextMeterWidget canvas is removed from InputSurface right-column
> - Submit button shrinks to a slim right-edge strip
> - Text input area expands to fill the freed space
> - `Ctrl+Space` binding moves to `StatusTab` (still bound globally)

---

## PD-03: SystemSurface

**Purpose**: Right ~34% of the window; provides tabbed system panels for model
selection, session context, file browsing, and settings.

### Tabs

| Widget | Position | Description |
|--------|----------|-------------|
| `ModelSelector` | Top of pane | Active model dropdown |
| Tabbed view | Below model selector | Four tabs: Status / Session / Files / Settings |

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

---

#### PD-03-AF-011 — Working Memory Toggle Checkbox

**ID**: `PD-03-AF-011`  
**Purpose**: Include or exclude an individual fact from the LLM context without deleting it.

**Behaviour**:

| Action | Outcome |
|--------|---------|
| Widget rendered | Toggle initial state reflects `fact.enabled` |
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

---

#### PD-03-AF-012 — Working Memory Delete Button

**ID**: `PD-03-AF-012`  
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

---

#### PD-03-AF-013 — Working Memory Promote Button

**ID**: `PD-03-AF-013`  
**Purpose**: Transfer ownership of an agent-written fact to the user, preventing the agent from overwriting it.

**Behaviour**:

| Action | Outcome |
|--------|---------|
| Owner icon clicked (🤖), dialog confirmed | `on_promote(compound_key)` fired |
| Owner icon clicked, dialog cancelled | `on_promote` NOT called |
| USER-owned fact | Owner icon is a static label (not clickable) |
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

---

#### PD-03-AF-014 — Working Memory Add-Fact Form

**ID**: `PD-03-AF-014`  
**Purpose**: Let the user add a new user-owned fact by entering a key and value.

**Add-fact form structure**:

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

---

#### PD-03-AF-015 — Working Memory Section Starts Collapsed

**ID**: `PD-03-AF-015`  
**Purpose**: Ensure the Working Memory section starts collapsed at startup, consistent with History, Available Tools, and Context sections.

**Behaviour**:

| Condition | Outcome |
|-----------|---------|
| SystemSurface freshly created | `working_memory` section `is_expanded()` returns `False` |
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

---

### Files Tab

`FileBrowser` widget — full detail: [PD-11](#pd-11-filebrowser).

### Settings Tab

`SettingsSurface` widget — full detail: [PD-07](#pd-07-settingssurface).

---

## PD-04: ModelSelector

**Purpose**: Top of SystemSurface; switches the active Ollama model for subsequent prompts.

| Control | Action | Effect |
|---------|--------|--------|
| Dropdown combo | Select model | `on_model_change(model_name)` → updates active model, writes to `agentx.toml` |
| `[⟳]` refresh | Reload model list | Calls Ollama `/api/tags` endpoint to refresh available models |

### Affordance: PD-04-AF-004 — Refresh button reloads model list

```gherkin
GIVEN a ModelSelector widget is rendered with a refresh callback registered
WHEN the user clicks the [⟳] button
THEN the refresh callback is invoked once
 AND the model dropdown is repopulated with the updated list from Ollama

GIVEN a ModelSelector widget is rendered with no refresh callback
WHEN the user clicks the [⟳] button
THEN no exception is raised

GIVEN a ModelSelector widget with a previous refresh callback
WHEN set_refresh_callback() is called with a new callback
THEN only the new callback is invoked on the next button click
```

---

## PD-05: PlanView

**Purpose**: Inside a plan tab in OutputSurface; live collapsible tree of plan execution state.

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

**Purpose**: Modal dialog for re-running synthesis on a specific task node,
optionally injecting a free-text hint and/or a new Working-Memory fact before
confirming.

### Layout

```
┌─────────────────────────── AgentX main window ────────────────────────────┐
│                                                                            │
│   [OutputSurface]           [SystemSurface]                               │
│                                                                            │
│        ┌──────────── ResynthesisDialog (modal) ─────────────────────────┐ │
│        │  Re-synthesise — <task_id>              640 × 520 px           │ │
│        └─────────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────────────────┘
```

The dialog is transient to its parent widget and centered on screen.

### Behaviour Inventory

| ID | Control / Trigger | Behaviour | Notes |
|----|-------------------|-----------|-------|
| PD-06-AF-001 | Window title | Title reads `"Re-synthesise — <task_id>"` | Set at construction |
| PD-06-AF-002 | `[Cancel]` button | Closes dialog; `on_confirm` is **not** called | |
| PD-06-AF-003 | `[Re-synthesise]` button | Closes dialog, then calls `on_confirm(hint.strip())` | Hint may be empty string |
| PD-06-AF-004 | WM hint section | Hidden when `on_add_wm_hint=None`; visible when provided | |
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

---

## PD-07: SettingsSurface

**Purpose**: Third tab of SystemSurface (`⚙️ Settings`); interactive `agentx.toml` editor.
All changes are persisted to disk immediately on interaction. Settings marked 🔁 require
a full app restart; a tooltip is shown on modification.

### Sections

#### 🎨 Appearance (expanded by default)

| Setting key | Label | Restart? |
|-------------|-------|----------|
| `agentx.theme_mode` | Theme mode | Yes 🔁 |
| `agentx.markdown_render_enabled` | Render Markdown | No |

#### 🤖 Ollama (expanded by default)

| Setting key | Label | Restart? |
|-------------|-------|----------|
| `agentx.ollama_host` | Host | Yes 🔁 |
| `agentx.ollama_model` | Default model | Yes 🔁 |
| `agentx.ollama_initial_load_timeout_seconds` | Load timeout (s) | Yes 🔁 |
| `agentx.screen_side` | Screen side | Yes 🔁 |

#### 🏛️ Working Memory (collapsed by default)

| Setting key | Label | Restart? |
|-------------|-------|----------|
| `agentx.working_memory.enabled` | Enabled | Yes 🔁 |
| `agentx.working_memory.inject_into_context` | Inject into LLM context | No |
| `agentx.working_memory.max_facts` | Max facts (0 = unlimited) | No |

---

### PD-07-AF-002: Section Collapse Defaults

**Affordance ID**: `PD-07-AF-002`

Each settings section uses `CollapsibleSection(initial_collapsed=...)`. The initial
expanded state of each section:

| Section title | `initial_collapsed` | Initial visible state |
|---------------|--------------------|-----------------------|
| 🎨 Appearance | `False` | Expanded (▼) |
| 🤖 Ollama | `False` | Expanded (▼) |
| 🏛️ Working Memory | `True` | Collapsed (▶) |

The two top sections are expanded by default so the most common settings are immediately
visible. The less-frequently-needed section is collapsed to reduce visual noise on first load.

**Behaviour**: User can toggle any section by clicking the ▼/▶ button; state is not persisted across restarts.

---

### PD-07-AF-003: Restart-Required Icon in Label Text

**Affordance ID**: `PD-07-AF-003`

Settings whose changes are persisted to disk but do NOT take effect until the app is
restarted carry the `🔁` icon appended to their label text.

**Restart-required fields**:

| Setting key | Label shown |
|-------------|-------------|
| `agentx.theme_mode` | `Theme mode 🔁` |
| `agentx.ollama_host` | `Host 🔁` |
| `agentx.ollama_model` | `Default model 🔁` |
| `agentx.ollama_initial_load_timeout_seconds` | `Load timeout (s) 🔁` |
| `agentx.screen_side` | `Screen side 🔁` |
| `agentx.working_memory.enabled` | `Enabled 🔁` |

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
└── ▶ 🏛️ Working Memory          (collapsed)
```

---

## PD-17: DemoMode

**Panel/Surface**: Interactive split-pane demo harness (`agentx --demo`)  
**Type**: CLI UX mode for pre-UAT validation

DemoMode is a user-visible, interactive pre-UAT flow that runs E2E test sequences and requests user feedback after every test.

In the interactive path, `agentx --demo` opens a split workspace view: the left pane shows the ordered sequence and accepts `N`/`J`/`X`, while the right pane mirrors the live AgentX pane set (output/context/input) so the operator can watch the actual app respond without collapsing the outer split.

### Affordance Inventory

| Affordance | ID | Expected Behavior | Status |
|-----------|----|-------------------|--------|
| `--demo` enters DemoMode | PD-17-AF-001 | Launches the split-pane demo controller and live-core mirror instead of normal interactive run | ✅ |
| Demo test sequence preview | PD-17-AF-002 | Displays ordered E2E tests with id/title before running | ✅ |
| Start selection from id/index | PD-17-AF-003 | User can choose where to start sequence (`--demo-start` or interactive pick) | ✅ |
| Per-test `N/J/X` user feedback loop | PD-17-AF-004 | End of each test returns control and accepts only `N`, `J <num>`, or `X` | ✅ |
| `X` failure artifact bundle | PD-17-AF-005 | Captures all surfaces + metadata to deterministic logs for analysis | ✅ |
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
- The `--demo-headless` internal flag enables automated smoke coverage without presenting the split-surface control interface.

---

## PD-18: SystemAppletSuite

**Panel/Surface**: Hybrid runtime system frame and UAT-visible applet startup mode  
**Type**: Runtime surface contract for the system applets and visible startup mode

This section defines the user-visible contract for the system applets that will
compose the system frame and for the optional visible-windows startup mode used
to validate applets before frame layout is enabled.

### Affordance Inventory

| Affordance | ID | Expected Behavior | Status |
|-----------|----|-------------------|--------|
| System frame binds by semantic title, not surface index | PD-18-AF-001 | Core resolves owned system surfaces by stable titles/roles after session reattach flows | ✅ Tested |
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
- Context-history keyboard behavior is owned by an applet model with explicit
  history node interfaces (user/session/turn IDs with parent links).
- `Tab` drills in one level and moves focus to the expanded target.
- `Shift-Tab` backs out one level and collapses the exited node.
- Inside `context-history`, `Space` performs history node peek/expand: it
  toggles the focused node branch visibility without moving focus.
- `Enter` is action-only: enable/disable element, commit cell value and advance
  to the next cell, or save a Working Memory pair.
- `PgUp/PgDn` scrolls wrapped text content only when the active row is an
  expanded `current-context` text entry; otherwise it pages row selection by 5
  rows (including `context-history` and `working-memory`).
- Boundary behavior is explicit: pressing `Down` when only a single user history
  row exists is a no-op and must not auto-descend into sessions.
- Runtime-default note: the Go TUI context applet defaults (`current-context`
  expanded; `context-history` and `working-memory` collapsed) are runtime-
  specific and may differ from legacy GUI Session tab defaults.

---

## PD-08: ContextRenderer

**Purpose**: Stateless widget factory that constructs the context/history/working-memory
sub-widgets shown in the Session tab.

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

**Purpose**: Reusable container that wraps any widget in an expand/collapse header.
Used in SystemSurface Session tab (Working Memory and Context sections) and in
SettingsSurface (each configuration group).

```
▼ Section Title (N items)     ← click to collapse
  ┌────────────────────────┐
  │ … child widgets …      │
  └────────────────────────┘

▶ Section Title (N items)     ← click to expand
```

### Behaviour Inventory

| Affordance ID | Trigger | Expected behaviour | Edge cases |
|---------------|---------|-------------------|------------|
| PD-09-AF-001 | Constructor with `initial_collapsed=True` | Starts collapsed (container not visible) | Empty content is allowed |
| PD-09-AF-002 | Constructor with `initial_collapsed=False` | Starts expanded (container visible) | No content set yet |
| PD-09-AF-003 | User click / `toggle()` | Flips expanded state and icon (`▶/▼`), shows or hides container | Repeated toggles remain stable |
| PD-09-AF-004 | `set_content(widget)` | Replaces previous content widget, destroys old one | First assignment has no prior widget |

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

---

## PD-10: ContextMeterWidget

**Purpose**: Donut chart showing context-window utilisation. Seven coloured arc bands
represent token categories; a ghost arc shows remaining capacity. A risk border changes
colour as utilisation approaches the limit. Hosted in StatusTab (PD-12).

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

| Affordance | ID | Status |
|-----------|----|---------|
| Meter creates canvas on first `create()` call | PD-10-AF-001 | ✅ |
| Arc slices sized proportionally to token counts | PD-10-AF-002 | ✅ |
| Ghost arc shows remaining capacity | PD-10-AF-003 | ✅ |
| Border turns warning-orange at 80 % utilisation | PD-10-AF-004 | ✅ |
| Border turns critical-red at 100 % utilisation | PD-10-AF-005 | ✅ |
| `update()` is thread-safe via deferred scheduling | PD-10-AF-006 | ✅ |
| `max_tokens=0` does not crash | PD-10-AF-007 | ✅ |

### Related Specs

- **PD-12-AF-011** — ContextMeterWidget is hosted in `StatusTab`; all above affordances unchanged.
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

> **Note (ARCH-03)**: Working Memory is injected as an ordinary `SYSTEM` message and is not yet separately tagged. Separating band 0 from band 1 requires setting `metadata["is_working_memory"] = True` in the context-build step.

### Requirements Baseline

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

Upgrade path: TOK-02 → TOK-03 via a tokenizer interface.

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

**Purpose**: Inside SettingsSurface; enables/disables individual tools per session.

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

Disabled tools are excluded from the `tools=[…]` array in the LLM API request.

---

## PD-11: FileBrowser

**Purpose**: Second tab (`Files`) of SystemSurface; browse the local filesystem,
attach files to the current message, open files for editing, and pin folder paths
to Working Memory.

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

### TUI Parity Requirements (Authoritative)

The Files user experience is implementation-agnostic and applies to GUI, TUI,
or hybrid delivery. A runtime applet implementation must satisfy these
requirements before parity can be marked complete.

| Requirement ID | Requirement |
|----------------|-------------|
| `PD-11-AF-011` | Large directory lists must remain navigable within the visible terminal viewport; selected row must stay visible while moving up/down. |
| `PD-11-AF-012` | Files surface must support accelerated navigation for long lists (`PageUp`, `PageDown`, `Home`, `End`) or equivalent commands with clear discoverability in-widget. |
| `PD-11-AF-013` | Files surface must provide deterministic overflow status (for example `showing X-Y of Z`) so users can orient themselves when content exceeds viewport height. |
| `PD-11-AF-014` | Parity sign-off for Files requires executable evidence for both small-list and overflow-list behavior, not only summary rendering assertions. |
| `PD-11-AF-015` | TUI files applet must support arrow-key navigation (`Up`/`Down` minimum) with behavior equivalent to row navigation controls. |
| `PD-11-AF-016` | TUI files applet must support `Space` as soft-select toggle semantics (alias to check/uncheck or context-select behavior) with clear on-screen state indication. |
| `PD-11-AF-017` | TUI files applet must support `Return` as hard-select activation semantics (alias to primary action/left-click behavior). |
| `PD-11-AF-018` | If any required affordance has no obvious TUI implementation path, resolution must be escalated to user for case-by-case contract decision before sign-off. |

These requirements are additive to `PD-11-AF-008..010` and `UF-11` / `UF-12`.

Implementation guidance note:

- TUI file navigation should follow established operator patterns seen in
  mature terminal file managers (for example dual-mode select/activate
  behavior), while preserving this UX contract as authoritative.

### File Context Menu (right-click on a file)

| Item | Action | Callback |
|------|--------|---------|
| Attach | Add file as attachment chip in InputSurface | `on_attach(path)` |
| Edit | Open file content for editing/viewing | `on_edit(path)` |

### Folder Context Menu (right-click on a directory)

| Item | Action | Callback |
|------|--------|---------|
| Add full path to memory | Saves `folder_name → /abs/path` as a Working Memory fact | `on_add_folder_to_memory(key, full_path)` |
| Add relative path to memory | Saves `folder_name → relative/path` as a Working Memory fact | `on_add_folder_to_memory(key, rel_path)` |

### Affordance: PD-11-AF-008 — Right-click on a file shows file context menu

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

```gherkin
GIVEN a file context menu is open
WHEN the user presses Escape
THEN both the file and folder context menus are unposted

GIVEN no context menu is open
WHEN _dismiss_popup_menu() is called
THEN no exception is raised
```

### Related User Flow

See [UF-05: File Attachment](02_USER_FLOWS.md#uf-05-file-attachment) for the end-to-end flow from clicking a file to it appearing as an attachment chip.
See [UF-11: File Explorer Navigation](02_USER_FLOWS.md#uf-11-file-explorer-navigation) for directory browsing and folder-to-memory flows.
See [UF-12: File Explorer Context Popup Rendering](02_USER_FLOWS.md#uf-12-file-explorer-context-popup-rendering) for popup visibility and first-frame palette guarantees.

---

## PD-12: StatusTab

**Purpose**: First tab of SystemSurface's system tab container (before Session / Files /
Settings). Provides real-time visibility into the current prompt-reply cycle — active
phase, elapsed time per step, and context window utilisation with a colour-key legend.
The tab auto-activates when the user submits a prompt and updates in-the-blind
(all widget state is written regardless of tab visibility; only paint is deferred).

> **Moved from**:
>
> - `ContextMeterWidget` (PD-10) — donut canvas formerly hosted in InputSurface.
>   The donut and its colour-key legend are now the upper section of this tab.
> - `InputSurface` — interrupt button removed from input surface right-column
>   and re-hosted here as the large `Interrupt` button below the phase stepper.
> - InputSurface freed: submit button shrinks to a slim strip; text area expands to fill.

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

### Context Window Section (upper)

Two child frames sit side-by-side inside a labelled frame `"Context Window"`:
a colour-key legend on the left, and the ContextMeterWidget donut on the right.

#### ContextKeyWidget — Colour Key

Renders one row per band in band-definition order. Each row contains:

- A small colour swatch
- A label with the band's display name

Bands are read from the same `_BANDS` constant as `ContextMeterWidget`
so the key is never out of sync with the donut. The ghost-arc (remaining capacity)
is the last row, using `_GHOST_COLOR` (`#444444`).

#### ContextMeterWidget (re-parented)

The existing `ContextMeterWidget` class is unchanged. Its `create(parent)` call
is moved from InputSurface to `StatusTab`. All existing affordances
(PD-10-AF-001 through PD-10-AF-007) and the tooltip hover behaviour are
preserved; only the host frame changes.

> **Spec cross-reference**: PD-10 (ContextMeterWidget) — relocation only; no
> behavioural change to the donut itself.

---

### Phase Stepper Section (middle)

A labelled frame `"Prompt Cycle"` containing a `PhaseStepperWidget`.

#### Phase Steps (in display order)

| Step key | Emoji | Label |
|----------|-------|-------|
| `classify` | 🤔 | Classify |
| `think` | 💭 | Think |
| `tool` | 🔧 | Tool: `<name>` _(name injected at runtime)_ |
| `respond` | ✍️ | Respond |

> Tool step label is dynamic: once a tool call begins the step label updates to
> `🔧 Tool: read_file` (or whichever tool is active). If multiple tool rounds
> occur, the same row is reused with the latest tool name.

#### Status Icons

| Icon | State | Meaning |
|------|-------|---------|
| `○` | `PENDING` | Not yet reached this step |
| `↻` | `RUNNING` | Currently executing; elapsed timer ticking |
| `✓` | `DONE` | Completed successfully |
| `✗` | `FAILED` | Ended with an error |

#### Elapsed Timer

- Format while running: `HH:MM:SS` (TUI: `HH:MM:SS.mmm`)
- Format while pending: `--:--:--` (TUI: `--:--:--.---`)
- Frozen at final elapsed when `DONE` or `FAILED`
- Updates fire regardless of tab visibility; paint queues if tab is not active

---

### Interrupt Button (bottom)

A full-width button spanning the tab:

```
text="⛔  Interrupt  (Ctrl+Space)"
state=disabled when not streaming
state=normal   when streaming
```

The `Ctrl+Space` global binding is moved from InputSurface to `StatusTab`
(still bound globally so it works regardless of focus).

> **Spec cross-reference**: PD-02-AF-004 (`user_break` button) — this affordance
> is **relocated** to PD-12. PD-02-AF-004 status changes to `🔁 Relocated →
> PD-12-AF-003`.

---

### Tab Navigation Behaviour

| Trigger | Action |
|---------|--------|
| User submits prompt | Switch to Status tab |
| Stream ends | Tab remains on Status (user may want to review elapsed times) |
| User manually switches tab | No forced return; updates continue in-the-blind |

---

### Affordance Inventory

#### PD-12-AF-001 — Status tab is the first tab in the system notebook

**Purpose**: Ensures the tab order is: Status → Session → Files → Settings

```gherkin
GIVEN SidePanel.create() has been called
WHEN  we query system_notebook tab names
THEN  the first tab text is "Status"
 AND  the second tab text is "Session"
```

#### PD-12-AF-002 — Auto-switch to Status tab on prompt submit

**Purpose**: Gives the user immediate visual feedback that the system received their input

```gherkin
GIVEN the user is on any tab in the system notebook
WHEN  the user submits a prompt
THEN  the system notebook switches to the Status tab
 AND  the Phase Stepper resets all rows to PENDING state
```

#### PD-12-AF-003 — Interrupt button enables/disables with streaming state

**Purpose**: Interrupt is only actionable when a stream is running

> **Relocated from**: PD-02-AF-004 (`user_break` button in InputSurface)

```gherkin
GIVEN streaming is not active
WHEN  StatusTab.set_streaming_state(False) is called
THEN  interrupt_btn state is DISABLED

GIVEN streaming is active
WHEN  StatusTab.set_streaming_state(True) is called
THEN  interrupt_btn state is NORMAL
```

#### PD-12-AF-004 — Interrupt button invokes on_interrupt callback

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

**Purpose**: Each new prompt cycle starts with a clean slate

```gherkin
GIVEN the stepper has rows with DONE state from a previous cycle
WHEN  StatusTab.reset() is called
THEN  all phase rows return to PENDING state
 AND  all elapsed labels show "--:--:--"
 AND  all status icons show "○"
```

#### PD-12-AF-006 — Phase row transitions to RUNNING and starts timer

**Purpose**: Marks a phase as in-progress and begins elapsed time display

```gherkin
GIVEN the "classify" row is in PENDING state
WHEN  set_phase("classify", "RUNNING") is called
THEN  the classify row status icon becomes "↻"
 AND  the classify elapsed label shows "00:00:00"
 AND  after ~1 second the elapsed label shows "00:00:01"
```

#### PD-12-AF-007 — Phase row transitions to DONE and freezes timer

**Purpose**: Records final elapsed time for a completed step

```gherkin
GIVEN the "classify" row has been RUNNING for ~3 seconds
WHEN  set_phase("classify", "DONE") is called
THEN  the classify row status icon becomes "✓"
 AND  the elapsed label is frozen at the final elapsed value
 AND  the label does not change after a further tick
```

#### PD-12-AF-008 — Phase row transitions to FAILED

**Purpose**: Distinguishes error-terminated steps from successful ones

```gherkin
GIVEN the "think" row is RUNNING
WHEN  set_phase("think", "FAILED") is called
THEN  the think row status icon becomes "✗"
 AND  the elapsed label is frozen at the failure time
```

#### PD-12-AF-009 — Tool step label updates with active tool name

**Purpose**: Identifies which tool is running without opening the output surface

```gherkin
GIVEN the tool row is in PENDING state
WHEN  set_phase("tool", "RUNNING", tool_name="read_file") is called
THEN  the tool row label shows "🔧 Tool: read_file"
```

#### PD-12-AF-010 — Colour-key legend rows match donut bands in order

**Purpose**: Key and donut are guaranteed to stay in sync

```gherkin
GIVEN ContextKeyWidget has been created
WHEN  we enumerate the key rows
THEN  the row count equals len(_BANDS) + 1 (for the ghost/remaining row)
 AND  each row swatch colour matches the corresponding band colour in _BANDS order
 AND  the final row swatch colour is _GHOST_COLOR
```

#### PD-12-AF-011 — ContextMeterWidget hosted in StatusTab (relocation)

**Purpose**: Donut retains all PD-10 affordances under new host; no functional regression

```gherkin
GIVEN StatusTab has been created
WHEN  we inspect the ContextWindowSection
THEN  a ContextMeterWidget canvas is present inside the section frame
 AND  calling update(max_tokens, breakdown) redraws the donut (same as PD-10-AF-001)
```

---

### SurfaceManager Interface Additions

The following methods are added to the `SurfaceManager` interface:

| Method | Purpose |
|--------|---------|
| `show_status_tab()` | Switch system tab container to the Status tab |
| `set_status_phase(step_key, state, tool_name=None)` | Delegate to `StatusTab.set_phase()` |
| `reset_status_tab()` | Delegate to `StatusTab.reset()` |

`set_streaming_state(is_streaming)` is extended to also call
`StatusTab.set_streaming_state(is_streaming)`.

---

### Cross-References

| Spec | Change |
|------|--------|
| PD-02 InputSurface | Stop button removed from input surface right-column; submit button resizes to slim strip; text area expands; `Ctrl+Space` binding migrated to `StatusTab`. See PD-02-AF-004 → **Relocated to PD-12-AF-003**. |
| PD-10 ContextMeterWidget | `create()` call moves from InputSurface to `StatusTab`; all PD-10-AF-001..007 affordances unchanged. |
| PD-03 SystemSurface | Status tab frame created and inserted at index 0 of system tab container before Session. |
| StreamingController | `_on_stream_start()` gains `show_status_tab()` + `reset_status_tab()` calls. Display helpers gain `set_status_phase()` calls at each phase transition. |

---

## PD-14: ContextPanelWidget

**Purpose**: Permanent tab ("Context") in the SystemSurface display tab container —
always visible, never modal. Management surface for all LLM context elements. The
`ContextMeterWidget` (PD-10) shows _what_; the Context Panel shows _why_ and lets
the user act: enable/disable messages, synthesise, clone/edit inline.  
**Status**: 📝 Spec only — not yet implemented.

### Layout

```
SystemSurface — "Context" tab
│
├── Selection Action Bar (hidden when selection = 0)
│     ┌────────────────────────────────────────────────────┐
│     │  N selected   [Disable]  [Synthesize]  [Clear]     │
│     └────────────────────────────────────────────────────┘
│
└── Scrollable row list
      ┌─[ ]──[▶]────────────────────────────────────[●]─┐
      │  sel  exp   Role icon · Name · ~N tok · X%       │
      │             Content preview (≤ 80 chars)         │
      │  ┌───────────────────────────────────────────┐   │  ← expanded only
      │  │  editable text widget                     │   │
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

### Affordance Inventory

| Affordance | ID | Status |
|-----------|-----|--------|
| Row enable/disable toggle | PD-14-AF-001 | 📝 |
| Row expand/collapse | PD-14-AF-002 | 📝 |
| Inline edit Save creates clone | PD-14-AF-003 | 📝 |
| Inline edit Discard reverts row | PD-14-AF-004 | 📝 |
| Row select checkbox shows action bar | PD-14-AF-005 | 📝 |
| [Disable] disables all selected rows | PD-14-AF-006 | 📝 |
| [Synthesize] runs synthesis LLM call | PD-14-AF-007 | 📝 |
| [Clear] deselects all rows | PD-14-AF-008 | 📝 |
| List frozen during streaming | PD-14-AF-009 | 📝 |
| Atomic rebuild on DONE | PD-14-AF-010 | 📝 |
| Click-to-navigate (ENH-09): band click scrolls panel | PD-14-AF-011 | 📝 |
| WM row disable sets session flag | PD-14-AF-012 | 📝 |
| System-prompt row disable sets session flag | PD-14-AF-013 | 📝 |

### Open Questions

| Code | Question |
|------|----------|
| Q-11 | Should synthesis (ENH-14) require a preview/approval step before originals are disabled? |
| Q-12 | Where does a synthesised message appear in the list: (a) replace last source position, (b) bottom of selection range, or (c) dedicated Synthesis section? |
| Q-13 | Should the synthesis compression instruction be a fixed internal prompt or user-editable per invocation? |

---

## PD-WM — Working-Memory Editor (TUI)

> **TUI surface (M2, SS-6).** The first **read-write** peer surface, launched as a
> separate process (`agentx surface launch working-memory`). It lists the session's
> working-memory facts and lets the user curate what folds into the agent's context.
> It re-authors, for the TUI, the legacy GUI working-memory affordances (PD-14). See
> `docs/implementation/02_surface_orchestration_http.md` (Working-Memory CRUD SS-6).

### Behaviour

Working memory is a document (`working_memory.json`), not an event stream, so the
surface reads on attach (`GET /working-memory`), polls (~2s) for live refresh, and
mutates through dedicated token-gated endpoints. Each fact renders as
`<cursor> <●/○> key = value` (● enabled / ○ disabled; agent-owned facts are tagged).
Mutations persist and take effect on the **next** prompt's assembled context (only
enabled facts fold in). It is read-write but single-purpose: no prompt input.

#### Scroll & Collapse

**🚧 Planned** (see the "Improve WM layouts" implementation plan). The fact list
is hosted in a scrollable viewport (`bubbles/v2/viewport`, the same primitive
`PD-01`'s TUI output panel uses — `docs/ux/06_OUTPUT_WIDGET.md`), with the
rightmost column reserved as a transcript-scrollbar gutter shown whenever facts
overflow the panel height. This surface fully mirrors the output panel's
established scroll/collapse taxonomy rather than inventing a new one, **including
its keybinding split** — `↑/↓`/`j`/`k` are repurposed from cursor movement (their
prior WM-only meaning) to **inner-scrolling the selected fact's value**, and
`PgUp`/`PgDn` become the cursor-movement keys (moving the selection exactly one
fact per press, matching the output panel's own `PgUp`/`PgDn` → `SelectUp`/
`SelectDown` behavior, not a true page jump). This is a **breaking change** to
this surface's previously-shipped `↑/↓` cursor binding, chosen deliberately for
cross-surface consistency over preserving the old binding.

A fact whose value word-wraps to more than one line (the `tree .`/multi-line
case this fixes) renders **collapsed by default**: only its first wrapped line
plus a `… (+N lines)` hint. Pressing **Enter** on the selected fact toggles it
between collapsed and expanded (a no-op on a single-line fact); expanded, the
value is capped to a per-fact row budget with its own scrollbar and inner-scroll
via `↑/↓`/`j`/`k`, exactly like an over-cap output-panel widget body. Moving the
selection (`PgUp`/`PgDn`) auto-scrolls the outer viewport to keep the newly
selected fact visible, the same `scrollSelectedIntoView` behavior the output
panel already has — no separate "jump to selection" key is needed. The owner/pin
annotation (` (agent)` / ` (pin ▶ live, 4s)`) always stays anchored to the fact's
first rendered row, never buried inside a wrapped or windowed body.

#### Pin (static / live)

A **pinned** fact (`Owner == pin`) is one created by the context surface's Pin
affordance (`p` on a selected tool-result, PD-CTX-AF-012) rather than typed by
hand — the durable, curated counterpart to the context surface's plain
enable/disable (PD-CTX-AF-011, which is session-scoped and applies to the exact
past event, not a copy). A pinned fact carries its source tool + args, so it can
be **re-run**, and one of two states:

- **static** (default at pin time): a frozen snapshot. The value never changes
  until the user edits or deletes it, exactly like a hand-typed fact.
- **live**: re-run before every turn's context assembly (`refreshLiveFacts`), so
  the value is always current — the mechanism for something like `tree .` or
  `date` that should never go stale. Only a tool that currently evaluates to
  policy `Allow` (no blacklist hit, no pending approval) can be set live; this
  prevents an unattended re-run of something that would otherwise need a human's
  sign-off, and avoids interrupting a turn on an approval prompt.

**Toggle key `l`** flips a pinned fact between live (▶) and static (⏸) — the
play/pause affordance — and immediately re-runs it once when switching to live,
so the action visibly does something rather than waiting for the next turn. It is
a no-op on a non-pinned fact. Each pinned fact's row shows its state and age
(`▶ live, 4s` / `⏸ static, 2m`) — `Age()` is measured from the last successful
refresh (live) or from when it was pinned (static), giving the user (and the
model, via the same annotation folded into the assembled context — see
`docs/implementation/03_configuration_and_storage.md`) a sense of how current the
value is.

**Unpinning** is the existing delete affordance (`d`) — no separate command. It
removes the WM fact only; it does **not** re-enable the source context element
(PD-CTX-AF-011/012) — that stays as the user left it. If the content is wanted
back in context, the user re-enables the original element directly in the
context surface.

### Affordance Inventory

| Affordance | ID | Status |
|-----------|-----|--------|
| List facts with enabled/disabled markers | PD-WM-AF-001 | ✅ |
| Navigate the selection cursor (`PgUp`/`PgDn`, one fact per press) | PD-WM-AF-002 | 🚧 (was `↑/↓`/`j`/`k` — see PD-WM-AF-010) |
| Toggle enable/disable (space) | PD-WM-AF-003 | ✅ |
| Delete the selected fact (d) — also the unpin affordance for a pinned fact | PD-WM-AF-004 | ✅ |
| Add a fact (a → `key=value`, enter) | PD-WM-AF-005 | ✅ |
| Edit the selected value (e) / cancel (esc) | PD-WM-AF-006 | ✅ |
| A pinned fact's row shows its static/live state and age | PD-WM-AF-007 | ✅ |
| Toggle a pinned fact live/static (`l`), refreshing once immediately on live | PD-WM-AF-008 | ✅ |
| Setting a fact live is refused when its source tool is not currently policy-`Allow` | PD-WM-AF-009 | ✅ |
| Inner-scroll the selected fact's value (`↑/↓`, `j`/`k`) | PD-WM-AF-010 | 🚧 |
| Expand/collapse the selected fact's multi-line value (Enter) | PD-WM-AF-011 | 🚧 |
| Outer viewport auto-scrolls to keep the selection visible | PD-WM-AF-012 | 🚧 |
| Transcript-style scrollbar in the reserved right gutter when facts overflow | PD-WM-AF-013 | 🚧 |

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: A multi-line value collapses by default (PD-WM-AF-011)

- GIVEN a working-memory surface with a fact whose value word-wraps to more than
  one line (e.g. a pinned `tree .` snapshot)
- WHEN the fact list renders
- THEN the fact shows only its first wrapped line plus a `… (+N lines)` hint,
  and its owner/pin annotation stays on that first line

Use-case: Expand/collapse the selected fact (PD-WM-AF-011)

- GIVEN a working-memory surface with a collapsed multi-line fact selected
- WHEN the user presses Enter
- THEN the fact expands to show its full value, capped to a per-fact row budget
  with its own scrollbar when the value exceeds it
- WHEN the user presses Enter again
- THEN the fact re-collapses to its first-line preview

Use-case: Enter on a single-line fact is a no-op (PD-WM-AF-011)

- GIVEN a working-memory surface with a single-line-value fact selected
- WHEN the user presses Enter
- THEN nothing changes (there is nothing to expand)

Use-case: Inner-scroll an expanded, over-cap fact (PD-WM-AF-010)

- GIVEN a working-memory surface with an expanded fact whose value exceeds the
  per-fact row budget
- WHEN the user presses `↓`/`j`
- THEN the fact's body window scrolls down by one row and a scrollbar thumb
  reflects the new position

Use-case: Cursor movement auto-scrolls the outer list (PD-WM-AF-002 / PD-WM-AF-012)

- GIVEN a working-memory surface with more facts than fit in the panel height,
  and the selection at the bottom edge of the visible window
- WHEN the user presses `PgDn`
- THEN the selection moves to the next fact and the outer viewport scrolls just
  enough to keep it visible

Use-case: Transcript scrollbar reflects overflow (PD-WM-AF-013)

- GIVEN a working-memory surface whose facts (rendered, with any expansions)
  exceed the panel height
- WHEN the fact list renders
- THEN the reserved right gutter column shows a proportional scrollbar thumb
- GIVEN all facts fit within the panel height
- WHEN the fact list renders
- THEN the gutter column is blank (no thumb)

### Deferred (later slices)

Inline edit→clone, multi-select action bar, synthesize-via-LLM, system-prompt row
toggle, and click-to-navigate (the remainder of legacy PD-14).

## PD-CTX — Context Surface (TUI)

> **TUI surface (M2, SS-3).** A read-only peer surface launched as a separate
> process (`agentx surface launch context`) that attaches over the transport and
> mirrors the session. It supersedes, for the TUI, the legacy GUI context affordances
> (PD-03 SystemSurface — Context, PD-08 ContextRenderer), which described the
> retired single-window GUI. See `docs/build-plan/06_system_surfaces_backlog.md`.

### Behaviour

The surface seeds from the durable event log on attach (the full prior session),
then resumes the live stream by cursor and appends new events (SS-1). It is a
**navigable summary**: every element renders **collapsed by default** (titled border
+ preview), so the surface reads as a scannable list of conversation elements, not a
full transcript — expand one with Enter to read it.

It deals only in **complete conversation elements**: it never receives the live
`agent_delta` stream (that is the chat window's job); an agent turn appears as one
finished `agent_response` element. Streaming is watched in the main window.

Its **primary affordance is enable/disable** (not read-only): selecting a
user-prompt, agent-response, tool-call, or tool-result element and pressing
**space** toggles whether that element participates in the agent's upcoming
context. The toggle is sent to the orchestrator, which applies it in memory
(effective on the next prompt) and persists it in the element's event file. Each
toggleable element carries an **enabled checkbox to the left of its role emoji** —
`[x]` when it is in context, `[ ]` when disabled (re-authoring PD-03-AF-007's
message-enabled checkbox). The checkbox is deliberately independent of the
selection border, so navigation and context-membership read as separate cues.
Thinking/classification/system-prompt/approval elements are display-only and not
toggleable, so they carry no checkbox. A one-line processing-state indicator sits
at the bottom. Quitting (`Ctrl-C`/`q`) marks the surface stopped.

A 🔧 tool-call or 📋 tool-result element (the flat, untagged `single_tool`-cycle
kind — a call folded into a plan step's Task node is display-only, unaffected)
starts **unchecked**: tool output normally scopes to the turn that produced it, so
by default it does not carry forward. Checking it is the same enable/disable
toggle as a user/agent element — nothing special about the word — applied to a
content class that starts off: its text folds into every subsequent turn's
assembled context until it is unchecked again. This is a session-scoped, one-off
inclusion of *that specific past event*; it is **not** the durable/curated
mechanism (that's Pin, below). An enabled tool element's bytes are counted under
the visualizer's `tools` 🔧 band (PD-CTXVIZ), which is otherwise always zero.

A selected 📋 tool-result element (only `tool_result`, not `tool_call` — the
result is the useful content) also has a distinct affordance: **`p` pins it to
working memory** (PD-WM). Pinning copies the element into a durable WM fact and
disables the source element here (`SetEventEnabled(ordinal, false)`), so the same
output is never represented twice — once as a raw context element, once as a WM
fact. Pin is a one-way handoff *out of* the context surface: the copy it creates
is managed from the working-memory surface from then on (static/live, age,
delete-to-unpin — PD-WM), not here. Unpinning does not restore the source
element's checkbox; if the user wants the raw event back in context, they
re-check it manually. See PD-WM's "Pin" affordances for the full design —
enable/disable here is the session-scoped mechanism, Pin is the durable one, and
they are deliberately different features that happen to share a source event.

### Affordance Inventory

| Affordance | ID | Status |
|-----------|-----|--------|
| Seed render: durable history on attach | PD-CTX-AF-001 | ✅ |
| Live tail: resumed complete events append after the seed cursor | PD-CTX-AF-002 | ✅ |
| Every element collapsed by default (navigable summary) | PD-CTX-AF-003 | ✅ |
| Navigation keys (scroll, page, select, expand/collapse) | PD-CTX-AF-004 | ✅ |
| Processing-state line reflects state · phase | PD-CTX-AF-005 | ✅ |
| Enable/disable the selected element (space) → context inclusion | PD-CTX-AF-006 | ✅ |
| Enabled checkbox (`[x]`/`[ ]`) left of the emoji, independent of selection | PD-CTX-AF-007 | ✅ |
| User/agent/flat-tool-call/flat-tool-result elements toggle; others are display-only | PD-CTX-AF-008 | ✅ |
| Complete agent responses only (no live `agent_delta` stream) | PD-CTX-AF-009 | ✅ |
| Title strip (`context · <session>`) via the surface host | PD-CTX-AF-010 | ✅ |
| Enabling a tool-call/tool-result folds its text into every subsequent turn's context (not just the turn that produced it), until disabled | PD-CTX-AF-011 | ✅ |
| Pin (`p`) a selected tool-result to working memory; disables it here (PD-WM) | PD-CTX-AF-012 | ✅ |

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Seed then live (PD-CTX-AF-001 / PD-CTX-AF-002)

- GIVEN a session with a recorded exchange
- WHEN a context surface attaches
- THEN it renders the prior exchange as collapsed elements, and complete events
  stream in thereafter

Use-case: Enable/disable an element (PD-CTX-AF-006)

- GIVEN a context surface with a selected agent-response element
- WHEN the user presses space
- THEN the element's checkbox flips (`[x]`→`[ ]`) and the toggle is sent to the
  orchestrator (excluded from the next assembled context)

Use-case: Non-toggleable element (PD-CTX-AF-008)

- GIVEN a context surface with a selected thinking element
- WHEN the user presses space
- THEN nothing is toggled (thinking never enters context)

Use-case: Enable a tool-result element (PD-CTX-AF-011)

- GIVEN a context surface with a selected, unchecked tool-result element from an
  earlier turn (e.g. a `tree .` listing)
- WHEN the user presses space
- THEN the checkbox flips to `[x]` and the toggle is sent to the orchestrator; the
  result's text folds into the assembled context on the next prompt and every
  prompt after, until the element is unchecked again

Use-case: Pin a tool-result to working memory (PD-CTX-AF-012)

- GIVEN a context surface with a selected tool-result element
- WHEN the user presses `p`
- THEN a working-memory fact is created from the element's text and the source
  element's checkbox is unchecked (excluded from the next assembled context) — the
  content now folds into context through the WM fact instead

Use-case: Collapsed by default (PD-CTX-AF-003)

- GIVEN a context surface
- WHEN any element arrives
- THEN it renders collapsed until the user expands it

## PD-CTXVIZ — Context Visualizer (TUI)

> **TUI surface (M2, SS-7).** A read-only peer surface launched as a separate
> process (`agentx surface launch context-visualizer`) that polls the assembled
> context window's composition and renders it as a budget meter. It re-authors the
> legacy GUI ContextMeterWidget (PD-10) and ContextKeyWidget (PD-12) for the TUI.
> See `docs/build-plan/06_system_surfaces_backlog.md`.

### Behaviour

The surface polls `GET /context` (every 2 s) for a per-content-class breakdown of
the standing context window and draws one bar per class in PD-10 band order, using
the app's content emoji so it reads consistently with the output widgets: working
memory 🧠, instructions 📜, user 👤, attachments 📎, thinking 💭, assistant 🤖,
tools 🔧. A remaining-capacity ghost band (`░`) and a total line complete the meter,
all measured against the model's context window — read from Ollama's `/api/show`
(`<architecture>.context_length`), the same window the runtime requests as
`num_ctx`. Token figures are a `chars ÷ 4` estimate (Ollama exposes no universal
local tokenizer), labelled "est.". When the model reports no context length the
meter drops the percentages and the ghost band.

It is **strictly read-only**: it holds an event stream only for connection presence
(SS-4) and performs no writes. The enable/disable-a-turn management affordance lives
on the context pane (PD-CTX); the meter only *hints* at what to prune. Classes not
yet fed into the assembled context (attachments, thinking today) render as zero
rather than being hidden, so the legend stays complete. `tools` renders as zero
too, until the context pane enables a tool-call/tool-result (PD-CTX-AF-011) —
from then on its bar reflects those bytes, same as any other class. A tool
element *pinned* to working memory (PD-CTX-AF-012 / PD-WM) instead counts under
`working-memory`, not `tools` — Pin hands the content off to WM entirely.
Quitting (`Ctrl-C`/`q`) marks the surface stopped.

### Affordance Inventory

| Affordance | ID | Status |
|-----------|-----|--------|
| Per-class bars in band order with content emoji | PD-CTXVIZ-AF-001 | ✅ |
| Bars sized against the model's context window | PD-CTXVIZ-AF-002 | ✅ |
| Remaining-capacity ghost band | PD-CTXVIZ-AF-003 | ✅ |
| Total line: est. tokens / window (percent) · model | PD-CTXVIZ-AF-004 | ✅ |
| Near-limit (≥80%) / full (≥100%) annotation | PD-CTXVIZ-AF-005 | ✅ |
| Graceful degrade when the window is unknown | PD-CTXVIZ-AF-006 | ✅ |
| Strictly read-only: no mutation affordance | PD-CTXVIZ-AF-007 | ✅ |
| Live refresh via poll (agent turns, WM edits appear) | PD-CTXVIZ-AF-008 | ✅ |

### Behavior contracts (GIVEN/WHEN/THEN)

Use-case: Per-class meter (PD-CTXVIZ-AF-001 / PD-CTXVIZ-AF-002)

- GIVEN a context breakdown with working-memory and user contributions and a known window
- WHEN the visualizer renders
- THEN it shows a bar per content class and a remaining band against the window

Use-case: Window unknown (PD-CTXVIZ-AF-006)

- GIVEN a breakdown whose model reports no context length
- WHEN the visualizer renders
- THEN it shows "window unknown" and omits the percentages and ghost band

Use-case: Read-only (PD-CTXVIZ-AF-007)

- GIVEN a context visualizer
- WHEN a mutation key (e.g. `a`) is pressed
- THEN nothing changes — no editor opens; management is directed to the context pane
