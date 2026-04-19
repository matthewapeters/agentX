# AgentX — Main Window Layout

Version: 2026-04-19

---

## 1. Window Layout Mockup

Proportions: ChatPanel occupies the **left ~66%** of the PanedWindow; SidePanel
occupies the **right ~34%**.  The InputPanel spans the **full width** at the
bottom.  The `screen_side` setting (⚙️ Settings tab) controls which side of the
**monitor** the window is placed on — it does **not** affect the internal panel
arrangement.

```
┌────────────────────────────────────────────────────────────────────────────────────┐
│  AgentX — the Ollama Agent                            (OS window title bar)        │
├────────────────────────────────────────────────────────────┬───────────────────────┤
│  CHAT AREA  (output notebook)              [→ PD-01]       │  SIDE PANEL [→ PD-03] │
│  (~66% width, height rely 0.00–0.77)                       │  (~34% width, 0–0.77) │
│ ┌──────────────────────────────────────────────────────┐   │                       │
│ │ Chat │ Plan: Step 1 │ Plan: Step 2 │                  │   │  [phi4-mini:3.8b ▾]   │
│ ├──────────────────────────────────────────────────────┤   │         [→ PD-04]     │
│ │ 👤 alice  12:04:01                                   │   │ ┌─────────────────────┐│
│ │ > write a function to parse JSON                     │   │ │Session │ Files │ ⚙️ ││
│ │                                                      │   │ ├─────────────────────┤│
│ │ ⚙️ Classification: complex_action → invoke_planner   │   │ │ 🧠 Working Memory   ││
│ │                                                      │   │ │        ▶            ││
│ │ 🤖 AgentX                                            │   │ ├─────────────────────┤│
│ │ > I'll break this into steps.                        │   │ │ 💬 Context          ││
│ │                                                      │   │ │        ▶            ││
│ │ 🔧 read_file  /src/parser.py                         │   │ └─────────────────────┘│
│ │   ▶ [expand result]                                  │   │                       │
│ │ Step 1 complete.                                     │   │                       │
│ └──────────────────────────────────────────────────────┘   │                       │
├────────────────────────────────────────────────────────────┴───────────────────────┤
│ 📎 README.md [×]   📎 parser.py [×]                             [✕ clear all]      │
│ (Attachment Bar — full width, rely=0.77)                             [→ PD-02]     │
├────────────────────────────────────────────────────────────────────────────────────┤
│ ┌──────────────────────────────────────────────────────────────────────┐  [Send]   │
│ │  Type your message here… (Enter to send, Shift+Enter = newline)     │  [Stop]   │
│ └──────────────────────────────────────────────────────────────────────┘  [→ PD-02]│
└────────────────────────────────────────────────────────────────────────────────────┘
```

### Detail Diagram References

| Generalised Component | Detail Diagram | Detail File |
|-----------------------|---------------|-------------|
| Chat Area (output notebook) | [§5 / PD-01](#5-layout-detail-chatpanel) | [03_PANEL_DETAILS.md — PD-01](03_PANEL_DETAILS.md#pd-01-chatpanel) |
| Side Panel | [§4 / PD-03](#4-layout-detail-sidepanel) | [03_PANEL_DETAILS.md — PD-03](03_PANEL_DETAILS.md#pd-03-sidepanel) |
| Model Selector | [§4 excerpt / PD-04](#4-layout-detail-sidepanel) | [03_PANEL_DETAILS.md — PD-04](03_PANEL_DETAILS.md#pd-04-modelselector) |
| Attachment Bar + Input Panel | [§6 / PD-02](#6-layout-detail-inputpanel) | [03_PANEL_DETAILS.md — PD-02](03_PANEL_DETAILS.md#pd-02-inputpanel) |

---

## 2. Zone Map

AgentX uses absolute placement (`.place(relx=…, rely=…)`) within the root window.

| Zone | `rely` | `relheight` | Class | Purpose |
|------|--------|-------------|-------|---------|
| Main paned area | 0.00 | 0.77 | `PanedWindow` | ChatPanel (left) + SidePanel (right) |
| Attachment bar | 0.77 | 0.03 | `InputPanel` | File attachment chips (full width) |
| Text input area | 0.80 | 0.20 | `InputPanel` | Message text + buttons (full width) |

The main paned area is split horizontally (sash at `output_panel_ratio` = 0.66):

| Pane | Side | Width | Widgets | Module | Detail |
|------|------|-------|---------|--------|--------|
| Left pane (~66%) | Left | 66% | Output notebook (`Chat` tab + plan tabs) | `ChatPanel` | [PD-01](03_PANEL_DETAILS.md#pd-01-chatpanel) |
| Right pane (~34%) | Right | 34% | Model selector + Session/Files/Settings tabs | `SidePanel` | [PD-03](03_PANEL_DETAILS.md#pd-03-sidepanel) |

---

## 3. Component Index

| Screen Region | Component Class | Source File |
|---------------|----------------|-------------|
| Window root | `AgentXSession` + `GUIManager` | `session.py`, `gui/gui_manager.py` |
| Left pane (~66%) | `ChatPanel` | `gui/chat_panel.py` |
| Right pane (~34%) | `SidePanel` | `gui/side_panel.py` |
| Model dropdown | `ModelSelector` | `gui/model_selector.py` |
| Session/Files/Settings tabs | `SidePanel._notebook` | `gui/side_panel.py` |
| Session tab: Working Memory | `ContextRenderer.render_working_memory_widget()` | `gui/context_renderer.py` |
| Session tab: Context messages | `ContextRenderer.render_context_widget()` | `gui/context_renderer.py` |
| Files tab | `FileExplorer` | `file_explorer.py` |
| Settings tab | `SettingsTab` | `gui/settings_tab.py` |
| Left output area | `ChatPanel._notebook` | `gui/chat_panel.py` |
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
SidePanel (right ~34% of window, height rely 0.00–0.77) [→ PD-03]
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
            │     screen_side    ( right ) ( left )  ← window placement on monitor
            │                                        (not internal panel arrangement)
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
ChatPanel (left ~66% of window, height rely 0.00–0.77) [→ PD-01]
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
InputPanel (bottom of window, full width, rely 0.77–1.0) [→ PD-02]
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
