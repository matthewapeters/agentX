---
title: AgentX Hybrid TUI-First Architecture (Go Core + Mixed Runtime Applets)
---

```mermaid
%%{init: { 'theme': 'default', 'flowchart': { 'curve': 'linear', 'nodeSpacing': 60, 'rankSpacing': 60 }, 'themeVariables': { 'fontSize': '18px' }, 'width': 1200, 'height': 800 }}%%
flowchart TD
    subgraph TMUX [tmux Session TUI-First Layout]
        direction TB
        P1["Pane: Chat/Output\n(Runtime: Go native widget default)"]
        P2["Pane: Logs\n(Runtime: Go native widget)"]
        P3["Pane: Input\n(Runtime: Go native widget)"]
        P4["Pane: System\n(Runtime: Go context widget)\n(files/config/context/history/visualizer)"]
        P5["Pane: Tabs/Navigation\n(Runtime: Go state-file tab routing)"]
    end
    subgraph GoCore [Go Core AgentX Orchestrator]
        G1["tmux Layout Manager\n(pane/window orchestration)"]
        G2["Applet Supervisor\n(spawn/monitor runtime-managed pane handlers)"]
        G3["IPC Router\n(FIFO, socket, or pipe for each applet)"]
        G4["Session State Manager\n(context, config, plugin registry)"]
    end
    subgraph RuntimeApplets [Runtime Applets]
        A1["TUI Chat/Output\n(Go native widget, direct Go fallback)"]
        A2["TUI Input\n(Go native widget)"]
        A3["TUI Logs\n(Go native widget)"]
        A4["TUI System Tabs\n(Go context widget render pipeline)"]
        A5["TUI Tabs/Navigation\n(system-panel-tab Go state routing)"]
        A6["GUI Applet (Tkinter)\n(mouse, color, emoji, singleton, relaunchable)"]
    end
    G1 --"tmux control"--> TMUX
    G2 --"spawn/manage"--> RuntimeApplets
    G3 --"IPC"--> RuntimeApplets
    G4 --"session state"--> RuntimeApplets
    TMUX --"user input/output"--> RuntimeApplets
    A6 -. "GUI launch/close" .-> G2
    G2 -. "GUI relaunch" .-> A6
    A6 -. "context sync" .-> G4
    style TMUX fill:#f9f,stroke:#333,stroke-width:2px
    style GoCore fill:#bbf,stroke:#333,stroke-width:2px
    style RuntimeApplets fill:#bfb,stroke:#333,stroke-width:2px
    %% Emoji/color/UX notes
    classDef emoji fill:#fffbe7,stroke:#333,stroke-width:1px
    class P1,P2,P3,P4,P5,A1,A2,A3,A4,A5 emoji
    %% Singleton GUI note
    click A6 "#" "GUI is singleton, relaunchable, independent lifecycle"
```

---

**To view this diagram in VS Code:**

- Install a Mermaid extension such as "Markdown Preview Mermaid Support" or "vscode-markdown-mermaid".
- Open this file and use the preview feature (usually right-click → "Open Preview" or use the command palette).

---

## Applet Contracts

Full UX/architecture contracts for each runtime applet — including ownership
boundaries, navigation models, phased implementation plans, and test evidence
targets — are in [`docs/architecture/applets/`](applets/README.md).

The **Output applet** (`chat/output`) UX contract, interactive navigation
design, and phased implementation plan are the canonical source of truth at
[`docs/architecture/applets/output_applet.md`](applets/output_applet.md).
Key design decisions for the Output applet:

- Core owns conversation data and tmux pane geometry; the applet owns
  pane-local view state (focus position, per-turn expand/compact flag).
- Default behavior: latest-first, focus-expands — newest turn auto-expanded,
  older turns compacted to a single summary line.
- Navigation: `j/k`/arrows (turn focus), `h/l` (hierarchy/collapse), Enter/Space
  (toggle), PgUp/PgDn (scroll), Home/End (oldest/newest), `?` (help), `q` (quit).
