# AgentX — Main Window Layout

Version: 2026-04-19

---

## 1. Window Layout Mockup

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  AgentX   [phi4-mini:3.8b ▾]  [● Status: Ready]                             │
├──────────┬───────────────────────────────────────────────────────────────────┤
│ SIDE     │  CHAT AREA  (output notebook)                                     │
│ PANEL    │ ┌──────────────────────────────────────────────────────────────┐  │
│          │ │ Chat │ Plan: Step 1 │ Plan: Step 2 │                         │  │
│ ┌──────┐ │ ├──────────────────────────────────────────────────────────────┤  │
│ │Model │ │ │ 👤 alice  12:04:01                                           │  │
│ │[▾]   │ │ │ > write a function to parse JSON                             │  │
│ └──────┘ │ │                                                              │  │
│ ┌──────┐ │ │ ⚙️ Classification: complex_action → invoke_planner           │  │
│ │Sesh │ │ │                                                              │  │
│ │Files│ │ │ 🤖 AgentX                                                    │  │
│ │⚙️ Set│ │ │ > I'll break this into steps.                                │  │
│ └──────┘ │ │                                                              │  │
│          │ │ 🔧 read_file  /src/parser.py                                 │  │
│ SESSION  │ │   ▶ [expand result]                                          │  │
│ ┌──────┐ │ │                                                              │  │
│ │🧠 WM │ │ │ Step 1 complete.                                             │  │
│ │  ▶   │ │ │                                                              │  │
│ └──────┘ │ └──────────────────────────────────────────────────────────────┘  │
│ ┌──────┐ ├──────────────────────────────────────────────────────────────────│
│ │💬 Ctx│ │ 📎 README.md   📎 parser.py                          [×clear]   │
│ │  ▶   │ ├──────────────────────────────────────────────────────────────────│
│ └──────┘ │ ┌────────────────────────────────────────────────────┐ [Send]    │
│          │ │  Type your message here… (Enter to send)           │ [Stop]    │
│          │ └────────────────────────────────────────────────────┘          │
└──────────┴───────────────────────────────────────────────────────────────────┘
```

---

## 2. Zone Map

AgentX uses absolute placement (`.place(relx=…, rely=…)`) within the root window.

| Zone | `rely` | `relheight` | Class | Purpose |
|------|--------|-------------|-------|---------|
| Main paned area | 0.00 | 0.77 | `PanedWindow` | Side panel + chat panel |
| Attachment bar | 0.77 | 0.03 | `InputPanel` | File attachment chips |
| Text input area | 0.80 | 0.20 | `InputPanel` | Message text + buttons |

The main paned area is split horizontally:

| Sash | Widgets | Module |
|------|---------|--------|
| Left sash (~25%) | Model selector + Session/Files/Settings tabs | `SidePanel` |
| Right sash (~75%) | Output notebook (`Chat` tab + plan tabs) | `ChatPanel` |

---

## 3. Component Index

| Screen Region | Component Class | Source File |
|---------------|----------------|-------------|
| Window root | `AgentXSession` + `GUIManager` | `session.py`, `gui/gui_manager.py` |
| Left pane | `SidePanel` | `gui/side_panel.py` |
| Model dropdown | `ModelSelector` | `gui/model_selector.py` |
| Session/Files/Settings tabs | `SidePanel._notebook` | `gui/side_panel.py` |
| Session tab: Working Memory | `ContextRenderer.render_working_memory_widget()` | `gui/context_renderer.py` |
| Session tab: Context messages | `ContextRenderer.render_context_widget()` | `gui/context_renderer.py` |
| Files tab | `FileExplorer` | `file_explorer.py` |
| Settings tab | `SettingsTab` | `gui/settings_tab.py` |
| Center/Right output | `ChatPanel._notebook` | `gui/chat_panel.py` |
| Chat tab | `ChatPanel` streaming entries | `gui/chat_panel.py` |
| Plan tabs | `PlanTreeWidget` | `gui/plan_tree_widget.py` |
| Attachment bar | `InputPanel.attachments_frame` | `gui/input_panel.py` |
| Text input | `InputPanel._user_input` | `gui/input_panel.py` |
| Send / Stop buttons | `InputPanel._btn_submit`, `_btn_interrupt` | `gui/input_panel.py` |
| Tool toggles | `ToolPanel` (inside SidePanel Settings) | `gui/tool_panel.py` |
| Re-synthesis dialog | `ResynthesisDialog` (modal) | `gui/resynthesis_dialog.py` |

---

## 4. Layout Detail: SidePanel

```
SidePanel (left ~25% of window, full height 0–77%)
│
├── ModelSelector                 top of pane
│     [phi4-mini:3.8b          ▾]
│
└── ttk.Notebook (fills remaining height)
      ├── "Session" tab
      │     ┌── CollapsibleSection: 🧠 Working Memory (N facts)
      │     │     [👤 cwd: /Projects/myapp]
      │     │     [👤 project: myapp      ]
      │     │     [🤖 current_task: ...   ]
      │     │     [+ Add fact…]
      │     │
      │     └── CollapsibleSection: 💬 Context (N messages)
      │           [👤] alice  12:04  > write a function…  [▶]
      │           [🤖] AgentX 12:04  > I'll break this…   [▶]
      │
      ├── "Files" tab
      │     FileExplorer widget
      │     (directory tree with click-to-open)
      │
      └── "⚙️ Settings" tab
            SettingsTab widget
            ├── [agentx] section
            │     ollama_host    [ localhost:11434 ]
            │     ollama_model   [ phi4-mini:3.8b ▾]
            │     theme_mode     ( dark ) ( light )
            │     screen_side    ( right ) ( left )
            │
            ├── [agentix] section
            │     host           [ localhost:8000 ]
            │     classification_model  [ phi4-mini:3.8b ▾]
            │
            └── Tool toggles (ToolPanel)
                  ▼ Available Tools
                  [✓] cst   Concrete Syntax Tree
                  [✓] ast   Abstract Syntax Tree
```

---

## 5. Layout Detail: ChatPanel

```
ChatPanel (right ~75% of window, height 0–77%)
│
└── ttk.Notebook
      ├── "Chat" tab            (always present)
      │     Scrollable canvas of entries:
      │       ┌─────────────────────────────────────────┐
      │       │ 👤 alice                       12:04:01 │
      │       │ write a function to parse JSON          │
      │       │  📎 README.md                           │
      │       ├─────────────────────────────────────────┤
      │       │ ⚙️  complex_action → invoke_planner      │
      │       ├─────────────────────────────────────────┤
      │       │ 💭  [thinking block — click to expand]  │
      │       ├─────────────────────────────────────────┤
      │       │ 🤖 AgentX                      12:04:03 │
      │       │ I'll break this into steps.            │
      │       │   🔧 read_file  ▶ [result]              │
      │       └─────────────────────────────────────────┘
      │
      ├── "Plan: Parse JSON" tab  (added when a plan is created)
      │     PlanTreeWidget:
      │       ┌─────────────────────────────────────────┐
      │       │ ● Plan: Parse JSON  [Re-synth] [Export] │
      │       │   ○ Step 1: Read existing parser        │
      │       │     🔧 read_file  /src/parser.py        │
      │       │       📋 [result — click to expand]      │
      │       │   ○ Step 2: Write new implementation    │
      │       └─────────────────────────────────────────┘
      │
      └── … additional plan tabs added dynamically
```

---

## 6. Layout Detail: InputPanel

```
InputPanel (bottom of window, rely 0.77–1.0)
│
├── Attachment bar (rely 0.77, relheight 0.03)
│     📎 README.md [×]    📎 parser.py [×]         [✕ clear all]
│     (history attachments shown greyed out)
│
└── Input row (rely 0.80, relheight 0.20)
      ┌──────────────────────────────────────┐  [Send]
      │  Type your message here…             │  [Stop]
      │  (Enter = send, Shift+Enter = newline)│
      └──────────────────────────────────────┘
```
