---
title: AgentX Hybrid TUI-First Architecture (Go Core + Python Applets)
---

```mermaid
%%{init: { 'theme': 'default', 'flowchart': { 'curve': 'linear', 'nodeSpacing': 60, 'rankSpacing': 60 }, 'themeVariables': { 'fontSize': '18px' }, 'width': 1200, 'height': 800 }}%%
flowchart TD
    subgraph TMUX [tmux Session TUI-First Layout]
        direction TB
        P1["Pane: Chat/Output\n(Python applet, e.g. rich/textual, color+emoji)"]
        P2["Pane: System/Logs\n(Python applet, e.g. tail, status, color+emoji)"]
        P3["Pane: Input\n(Python applet, prompt_toolkit, color+emoji)"]
        P4["Pane: Context Visualizer\n(Python applet, text-based, color+emoji)"]
        P5["Pane: Tabs/Navigation\n(Python applet, text-based, color+emoji)"]
    end
    subgraph GoCore [Go Core AgentX Orchestrator]
        G1["tmux Layout Manager\n(pane/window orchestration)"]
        G2["Applet Supervisor\n(spawn, monitor, restart Python applets)"]
        G3["IPC Router\n(FIFO, socket, or pipe for each applet)"]
        G4["Session State Manager\n(context, config, plugin registry)"]
    end
    subgraph PythonApplets [Python Applets]
        A1["TUI Chat/Output Applet\n(rich/textual, LLM, color+emoji)"]
        A2["TUI Input Applet\n(prompt_toolkit, color+emoji)"]
        A3["TUI System/Logs Applet\n(tail, status, color+emoji)"]
        A4["TUI Context Visualizer\n(text-based, color+emoji)"]
        A5["TUI Tabs/Navigation Applet\n(text-based, color+emoji)"]
        A6["GUI Applet (Tkinter)\n(mouse, color, emoji, singleton, relaunchable)"]
    end
    G1 --"tmux control"--> TMUX
    G2 --"spawn/manage"--> PythonApplets
    G3 --"IPC"--> PythonApplets
    G4 --"session state"--> PythonApplets
    TMUX --"user input/output"--> PythonApplets
    A6 -. "GUI launch/close" .-> G2
    G2 -. "GUI relaunch" .-> A6
    A6 -. "context sync" .-> G4
    style TMUX fill:#f9f,stroke:#333,stroke-width:2px
    style GoCore fill:#bbf,stroke:#333,stroke-width:2px
    style PythonApplets fill:#bfb,stroke:#333,stroke-width:2px
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
