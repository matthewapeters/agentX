# AgentX — Main Window Layout

> **⚠️ Architecture migration (2026-06-26).** This layout describes the prior
> single-window split-pane GUI. AgentX is now a **client-server** app: the chat
> surface (launched by `agentx`) has **two panels — output + input** — and the
> former right-hand "system" surface is now **multiple independent, separately
> launchable surfaces** the user arranges via tmux/screen/zellij. The split-pane
> geometry below is **legacy**, pending M2 migration. See
> [`../architecture/00_ARCHITECTURE_RECONCILIATION.md`](../architecture/00_ARCHITECTURE_RECONCILIATION.md).

_Last updated: 2026-05-06 (v0.22.20.post3)_

---

## 1. Window Layout Mockup

Proportions: the OutputSurface occupies the **left ~66%** of the split-pane layout; the
SystemSurface occupies the **right ~34%**. The InputSurface spans the **full width** at
the bottom. The `screen_side` setting (SettingsSurface) controls which side of the
**monitor** the window is placed on — it does **not** affect the internal surface
arrangement.

```
┌────────────────────────────────────────────────────────────────────────────────────┐
│  AgentX — the Ollama Agent                            (OS window title bar)        │
├────────────────────────────────────────────────────────┬───────────────────────────┤
│  OUTPUT SURFACE  (tabbed view)          [→ PD-01]      │  SYSTEM SURFACE [→ PD-03] │
│  (~66% width)                                          │  (~34% width)             │
│ ┌──────────────────────────────────────────────────┐   │                           │
│ │ Chat │ Plan: Step 1 │ Plan: Step 2 │              │   │  [phi4-mini:3.8b ▾]       │
│ ├──────────────────────────────────────────────────┤   │         [→ PD-04]         │
│ │ 👤 alice  12:04:01                               │   │ ┌─────────────────────────┐│
│ │ > write a function to parse JSON                 │   │ │Session │ Files │ ⚙️      ││
│ │                                                  │   │ ├─────────────────────────┤│
│ │ 🤖 AgentX                                        │   │ │ 🧠 Working Memory       ││
│ │ > I'll break this into steps.                    │   │ │        ▶                ││
│ │                                                  │   │ ├─────────────────────────┤│
│ │ 🔧 read_file  /src/parser.py                     │   │ │ 💬 Context              ││
│ │   ▶ [expand result]                              │   │ │        ▶                ││
│ │ Step 1 complete.                                 │   │ └─────────────────────────┘│
│ └──────────────────────────────────────────────────┘   │                           │
├────────────────────────────────────────────────────────┴───────────────────────────┤
│ 📎 README.md [×]   📎 parser.py [×]                             [✕ clear all]      │
│ (Attachment Bar — full width)                                        [→ PD-02]     │
├────────────────────────────────────────────────────────────────────────────────────┤
│ ┌──────────────────────────────────────────────────────────────────────┐  [Send]   │
│ │  Type your message here… (Enter to send, Shift+Enter = newline)     │  [Stop]   │
│ └──────────────────────────────────────────────────────────────────────┘  [→ PD-02]│
└────────────────────────────────────────────────────────────────────────────────────┘
```

### Detail Diagram References

| Generalised Component | Detail Diagram | Detail File |
|-----------------------|---------------|-------------|
| Output Surface (tabbed view) | [§5 / PD-01](#5-layout-detail-outputsurface) | [03_PANEL_DETAILS.md — PD-01](03_PANEL_DETAILS.md#pd-01-outputsurface) |
| System Surface | [§4 / PD-03](#4-layout-detail-systemsurface) | [03_PANEL_DETAILS.md — PD-03](03_PANEL_DETAILS.md#pd-03-systemsurface) |
| Model Selector | [§4 excerpt / PD-04](#4-layout-detail-systemsurface) | [03_PANEL_DETAILS.md — PD-04](03_PANEL_DETAILS.md#pd-04-modelselector) |
| Attachment Bar + Input Surface | [§6 / PD-02](#6-layout-detail-inputsurface) | [03_PANEL_DETAILS.md — PD-02](03_PANEL_DETAILS.md#pd-02-inputsurface) |
| Files tab (FileBrowser) | — | [03_PANEL_DETAILS.md — PD-11](03_PANEL_DETAILS.md#pd-11-filebrowser) |
| Settings tab (SettingsSurface) | — | [03_PANEL_DETAILS.md — PD-07](03_PANEL_DETAILS.md#pd-07-settingssurface-detail) |

---

## 2. Zone Map

The window is divided into three zones. The main split-pane area occupies roughly the
upper 77% of height and is divided into a left OutputSurface (~66% width) and a right
SystemSurface (~34% width). Below the main area sits a narrow attachment bar for file
chips (full width), followed by a text input row with Send/Stop controls (full width,
remaining height).

The main split-pane is divided horizontally at approximately the 66% mark:

| Pane | Side | Width | Contents | Detail |
|------|------|-------|----------|--------|
| Left pane (~66%) | Left | 66% | Tabbed output view (Chat tab + plan tabs) | [PD-01](03_PANEL_DETAILS.md#pd-01-outputsurface) |
| Right pane (~34%) | Right | 34% | Model selector + Session/Files/Settings tabs | [PD-03](03_PANEL_DETAILS.md#pd-03-systemsurface) |

---

## 3. Component Index

| Screen Region | Component |
|---------------|-----------|
| Window root | Orchestrator + SurfaceManager |
| Left pane (~66%) | OutputSurface |
| Right pane (~34%) | SystemSurface |
| Model dropdown | ModelSelector |
| Session/Files/Settings tabs | SystemSurface tabbed view |
| Session tab: Working Memory | ContextRenderer (working memory section) |
| Session tab: Context messages | ContextRenderer (context section) |
| Files tab | FileBrowser [→ PD-11](03_PANEL_DETAILS.md#pd-11-filebrowser) |
| Settings tab | SettingsSurface [→ PD-07](03_PANEL_DETAILS.md#pd-07-settingssurface-detail) |
| Left output area | OutputSurface tabbed view |
| Chat tab | OutputSurface streaming entries |
| Plan tabs | PlanView |
| Attachment bar | InputSurface attachment row |
| Text input | InputSurface message field |
| Send / Stop buttons | InputSurface submit/interrupt controls |
| Tool toggles | ToolPanel (inside SettingsSurface) |
| Re-synthesis dialog | ResynthesisDialog (modal) |

---

## 4. Layout Detail: SystemSurface

```
SystemSurface (right ~34% of window) [→ PD-03]
│
├── ModelSelector                 top of pane
│     [phi4-mini:3.8b          ▾]
│
└── tabbed view (fills remaining height)
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
      │     FileBrowser widget
      │     (directory tree with click-to-open)
      │
      └── "⚙️ Settings" tab
            SettingsSurface widget
            ├── Ollama section
            │     ollama_host    [ localhost:11434 ]
            │     ollama_model   [ phi4-mini:3.8b ▾]
            │
            └── Appearance section
                  theme_mode     ( dark ) ( light )
                  screen_side    ( right ) ( left )  ← window placement on monitor
                                                     (not internal surface arrangement)
```

---

## 5. Layout Detail: OutputSurface

```
OutputSurface (left ~66% of window) [→ PD-01]
│
└── tabbed view
      ├── "Chat" tab            (always present)
      │     Scrollable list of entries:
      │       ┌─────────────────────────────────────────┐
      │       │ 👤 alice                       12:04:01 │
      │       │ write a function to parse JSON          │
      │       │  📎 README.md                           │
      │       ├─────────────────────────────────────────┤
      │       │ 💭  [thinking block — click to expand]  │
      │       ├─────────────────────────────────────────┤
      │       │ 🤖 AgentX                      12:04:03 │
      │       │ I'll break this into steps.            │
      │       │   🔧 read_file  ▶ [result]              │
      │       └─────────────────────────────────────────┘
      │
      ├── "Plan: Parse JSON" tab  (added when a plan is created)
      │     PlanView:
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

## 6. Layout Detail: InputSurface

```
InputSurface (bottom of window, full width) [→ PD-02]
│
├── Attachment bar (top sub-row, full width)
│     📎 README.md [×]    📎 parser.py [×]         [✕ clear all]
│     (history attachments shown greyed out)
│
└── Input row (fills remaining height)
      ┌──────────────────────────────────────┐  [Send]
      │  Type your message here…             │  [Stop]
      │  (Enter = send, Shift+Enter = newline)│
      └──────────────────────────────────────┘
```
