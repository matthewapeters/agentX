# AgentX — User Flows

> **⚠️ Architecture migration (2026-06-26).** These flows assume the prior
> single-window split-pane GUI. AgentX is now a **client-server** app with a
> two-panel chat surface and multiple independent system surfaces; flows that cross
> the old tabbed "system" panel are being migrated during M2. See
> [`../architecture/00_ARCHITECTURE_RECONCILIATION.md`](../architecture/00_ARCHITECTURE_RECONCILIATION.md).

_Last updated: 2026-05-22 (v0.74.4.post2)_

Each section contains a Mermaid sequence or flowchart diagram documenting how
user actions map to system behaviour.

---

## UF-01: Basic Chat (Direct Response)

**Trigger**: User types a message and presses `Enter` (or clicks `Send`).

```mermaid
sequenceDiagram
    participant User
    participant InputSurface
    participant Session as Orchestrator
    participant Bridge as LLMBridge
    participant LLM as Ollama
    participant Chat as OutputSurface

    User->>InputSurface: types message, presses Enter
    InputSurface->>Session: on_submit()
    Session->>Chat: display_user_message(content, attachments, timestamp)
    Session->>Bridge: process_prompt_generator(prompt, context)
    Bridge->>LLM: POST /v1/chat/completions (classify)
    LLM-->>Bridge: PromptClassificationResponse (respond_directly)
    Bridge->>LLM: POST /v1/chat/completions (stream=True)
    loop streaming
        LLM-->>Bridge: delta chunk
        Bridge-->>Session: ResponseChunk(CONTENT, text)
        Session->>Chat: display_agent_response(text)
    end
    Bridge-->>Session: ResponseChunk(DONE)
    Session->>Chat: finalize_current_turn_markdown()
    Session->>Session: persist messages to Context
```

**Key affordances**:

- `Stop` button visible during streaming; clicking it sets `_is_streaming` to False and stops the generator.
- Response appears word-by-word (real-time streaming).
- Markdown is rendered after streaming completes.

---

## UF-02: Tool Execution (Single Tool)

**Trigger**: User asks something that requires a file or code tool.

```mermaid
sequenceDiagram
    participant User
    participant Session as Orchestrator
    participant Bridge as LLMBridge
    participant ToolLoop as ToolLoopRunner
    participant ToolDisp as ToolDispatcher
    participant LLM as Ollama
    participant Chat as OutputSurface

    User->>Session: submit prompt
    Session->>Bridge: process_prompt_generator()
    Bridge->>LLM: classify → single_tool
    Bridge->>LLM: stream (tools enabled)
    LLM-->>Bridge: ResponseChunk(TOOL_CALL, {name, args, id})
    Bridge-->>Session: TOOL_CALL chunk
    Session->>Chat: display_tool_call(name, args)
    Session->>ToolDisp: execute_tool(name, args)
    ToolDisp->>ToolDisp: route (client / server)
    ToolDisp-->>Session: result string
    Session->>Bridge: inject tool_result message
    Bridge->>LLM: continue stream with tool result
    LLM-->>Bridge: ResponseChunk(CONTENT, final text)
    Bridge-->>Session: CONTENT chunk
    Session->>Chat: display_agent_response(text)
    Bridge-->>Session: ResponseChunk(DONE)
```

**Key affordances**:

- Tool call row appears in chat as `🔧 tool_name  args…` with a collapse/expand button.
- Tool result is shown inline (collapsed by default).
- Multiple tool calls in a single round are executed in parallel (`ThreadPoolExecutor`).

---

## UF-03: Hierarchical Task Execution (Planner)

**Trigger**: User asks a complex multi-step task.

```mermaid
sequenceDiagram
    participant User
    participant Session as Orchestrator
    participant Bridge as LLMBridge
    participant LLM as Ollama
    participant Chat as OutputSurface
    participant Tree as PlanView

    User->>Session: submit prompt
    Session->>Bridge: process_prompt_generator()
    Bridge->>LLM: classify → invoke_planner
    Bridge->>LLM: create plan (JSON)
    LLM-->>Bridge: PlanRecord {steps:[…]}
    Bridge-->>Session: ResponseChunk(PLAN, plan_data)
    Session->>Chat: add_plan_tab(plan_id, name)
    Chat->>Tree: initialise tree with step nodes
    loop for each step
        Bridge-->>Session: ResponseChunk(TASK_NODE_START, task_data)
        Session->>Tree: add_plan_step_node(step)
        Tree->>Tree: set status ●
        Bridge->>LLM: run task (tool loop)
        loop tool calls
            Bridge-->>Session: TOOL_CALL / TOOL_RESULT
            Session->>Tree: add tool row
        end
        Bridge-->>Session: ResponseChunk(TASK_NODE_DONE, synthesis)
        Session->>Tree: update_plan_node_status(done, synthesis_text)
        Tree->>Tree: set status ✓
    end
    Bridge-->>Session: ResponseChunk(DONE)
```

**Key affordances**:

- Each plan step has a live status icon (○●✓✗?).
- Tool call rows under each step expand/collapse inline.
- Synthesis text visible below each completed step.
- `[Re-synth]` button per step → see UF-04.
- `[Export]` button → exports tree to `task_tree_export.md`.

---

## UF-04: Re-synthesis

**Trigger**: User clicks `[Re-synth]` on a completed plan step.

```mermaid
sequenceDiagram
    participant User
    participant Tree as PlanView
    participant Dialog as ResynthesisDialog
    participant Session as Orchestrator
    participant Bridge as LLMBridge
    participant Chat as OutputSurface

    User->>Tree: clicks [Re-synth] on a step node
    Tree->>Dialog: open ResynthesisDialog(task_id)
    Dialog->>User: show modal (optional WM hint field)
    User->>Dialog: enter hint (optional), click [Re-synthesise]
    Dialog->>Session: retrigger_synthesis_streaming(task_id, wm_hint)
    Session->>Bridge: retrigger_synthesis_streaming()
    Bridge->>Bridge: replay task messages + inject hint
    loop streaming
        Bridge-->>Session: ResponseChunk(SYNTHESIS, text)
        Session->>Tree: update synthesis text for node
    end
    Bridge-->>Session: ResponseChunk(DONE)
    Tree->>Tree: show updated synthesis text
```

---

## UF-05: File Attachment

**Trigger**: User clicks a file in the `FileBrowser` or drags one to the input area.

```mermaid
sequenceDiagram
    participant User
    participant Explorer as FileBrowser
    participant Session as Orchestrator
    participant InputSurface
    participant Context

    User->>Explorer: clicks a file in Files tab
    Explorer->>Session: on_file_open(path)
    Session->>Session: add AttachmentInfo to pending_attachments
    Session->>InputSurface: update_attachment_bar(current, history)
    InputSurface->>InputSurface: render chip: 📎 filename.py [×]
    User->>InputSurface: submits message
    Session->>Context: add_message(role=USER, attachments=[…])
    Context->>Context: persist to disk
    Note right of Context: file content injected\ninto LLM context as\nsystem attachment block
```

**Attachment bar affordances**:

- Current-turn attachments shown as chips with `[×]` remove button.
- History attachments shown greyed (cannot be removed; already in context).
- `[✕ clear all]` removes all current-turn attachments.

---

## UF-06: Model Switch

**Trigger**: User selects a different model from the toolbar dropdown.

```mermaid
sequenceDiagram
    participant User
    participant Selector as ModelSelector
    participant Session as Orchestrator
    participant State as SessionState
    participant WM as WorkingMemory

    User->>Selector: selects "llama3.2:3b" from dropdown
    Selector->>Session: on_model_change("llama3.2:3b")
    Session->>State: active_model = "llama3.2:3b"
    Session->>WM: remember_fact("agent:active_model", "llama3.2:3b")
    Session->>Session: write updated model to agentx.toml
    Note right of Session: next prompt uses\nthe selected model
```

---

## UF-07: Settings Change

**Trigger**: User edits a value in the Settings tab.

```mermaid
sequenceDiagram
    participant User
    participant Settings as SettingsSurface
    participant Session as Orchestrator
    participant Config as AgentXConfig

    User->>Settings: changes a setting (e.g. theme_mode → light)
    Settings->>Session: on_change(["agentx","theme_mode"], "light")
    Session->>Config: config.set(["agentx","theme_mode"], "light")
    Config->>Config: write agentx.toml to disk
    alt restart required (🔁 label)
        Settings->>User: show tooltip "Restart required"
    else hot-applies
        Session->>Session: apply change immediately
    end
```

---

## UF-08: Session History Navigation

**Trigger**: User clicks on a prior session in the `Session` tab.

```mermaid
sequenceDiagram
    participant User
    participant SystemSurface
    participant Session as Orchestrator
    participant History
    participant Chat as OutputSurface

    User->>SystemSurface: clicks prior session entry in Session tab
    SystemSurface->>Session: on_session_select(session_id)
    Session->>History: load_context(session_id)
    History-->>Session: Context object
    Session->>Chat: render_history_widget(context)
    Chat->>Chat: display all historical messages (read-only)
```

---

## UF-09: Working Memory Management

**Trigger**: User interacts with a fact in the Working Memory surface.

```mermaid
flowchart TD
    A[User opens Session tab] --> B[Working Memory section visible]
    B --> C{Action?}
    C -->|Toggle enable/disable| D[on_toggle key, enabled]
    C -->|Delete fact| E[on_delete key]
    C -->|Promote agent fact to user| F[on_promote key]
    C -->|Add new user fact| G[on_user_add key, value]
    D --> H[WorkingMemory.set_enabled]
    E --> I[WorkingMemory.remove_fact]
    F --> J[WorkingMemory.promote_to_user]
    G --> K[WorkingMemory.add_user_fact]
    H --> L[re-render WM widget]
    I --> L
    J --> L
    K --> L
    L --> M[persist working_memory.json]
```

---

## UF-10: Interrupt Streaming

**Trigger**: User clicks `Stop` while the LLM is streaming.

```mermaid
sequenceDiagram
    participant User
    participant InputSurface
    participant Session as Orchestrator
    participant Controller as StreamingController
    participant Chat as OutputSurface

    User->>InputSurface: clicks Stop button
    InputSurface->>Session: on_interrupt()
    Session->>Controller: interrupt()
    Controller->>Controller: set interrupt flag = True
    Controller->>Controller: clear is_streaming event
    Note right of Controller: generator stops on next iteration
    Session->>Chat: display partial response
    Session->>Session: persist partial message to Context
```

---

## UF-11: File Explorer Navigation

**Trigger**: User opens the `Files` tab in the SystemSurface and browses the local filesystem.

```mermaid
sequenceDiagram
    participant User
    participant SystemSurface
    participant FileBrowser
    participant Session as Orchestrator
    participant InputSurface

    User->>SystemSurface: selects Files tab
    SystemSurface->>FileBrowser: render file tree at current path
    User->>FileBrowser: double-clicks a directory
    FileBrowser->>FileBrowser: change_directory(path)
    FileBrowser->>FileBrowser: update history stack
    FileBrowser->>FileBrowser: refresh tree listing
    FileBrowser->>FileBrowser: update path label

    alt User attaches a file
        User->>FileBrowser: right-clicks file, selects Attach
        FileBrowser->>Session: on_attach(path)
        Session->>InputSurface: add attachment chip
    else User edits a file
        User->>FileBrowser: right-clicks file, selects Edit
        FileBrowser->>Session: on_edit(path)
        Session->>Session: load file into chat context
    else User pins a folder to Working Memory
        User->>FileBrowser: right-clicks directory
        User->>FileBrowser: selects Add full path to memory
        FileBrowser->>Session: on_add_folder_to_memory(key, path)
        Session->>Session: store fact in working_memory
    else User navigates history
        User->>FileBrowser: clicks Back or Forward
        FileBrowser->>FileBrowser: navigate_back() or navigate_forward()
        FileBrowser->>FileBrowser: refresh tree listing
    end
```

---

## UF-12: File Explorer Context Popup Rendering

**Trigger**: User right-clicks a file or directory row in the `Files` tab.

```mermaid
sequenceDiagram
    participant User
    participant Explorer as FileBrowser
    participant Popup as ContextPopup
    participant Surface as SurfaceManager

    User->>Explorer: right-click row
    Explorer->>Explorer: compute safe coordinates
    Explorer->>Popup: create themed context popup
    Popup->>Surface: render popup at target coordinates
    Popup-->>User: visible context actions (Attach/Edit or folder actions)
```

**Main-window UX invariants**:

- Right-click context actions must be visually present before they are actionable.
- The context popup must use the selected theme palette on first render (no flash before theme applies).
- Popup rendering behavior must remain consistent across repeated right-click cycles.

---

## UF-13: Startup Log Locations Notice

**Trigger**: AgentX completes startup layout before first agent response.

```mermaid
sequenceDiagram
    participant User
    participant Session as Orchestrator
    participant Config as agentx.toml
    participant Chat as OutputSurface

    Session->>Config: read agentx.show_log_locations_on_startup
    alt config is true or absent
        Session->>Chat: display_startup_notice(log locations)
        Chat-->>User: friendly startup message in output window
    else config is false
        Session->>Session: skip startup notice
    end
    Session->>Session: proceed with bootstrap/normal agent flow
```

**Behavior goals**:

- Show users where logs are written without requiring filesystem discovery.
- Display before first assistant response so operational context is immediately visible.
- Allow suppression via `agentx.show_log_locations_on_startup=false`.

---

## UF-14: Demo Mode (Per-Test UAT Review Loop)

**Trigger**: UAT operator launches `agentx --demo`.

```mermaid
sequenceDiagram
    participant User as UAT Operator
    participant CLI as agentx --demo
    participant Harness as DemoHarness
    participant E2E as E2E Runner
    participant App as ApplicationSession
    participant Logs as logs/demo/<session>

    User->>CLI: starts demo mode
    CLI->>Harness: initialize demo manifest
    Harness-->>User: print ordered test sequence
    User->>Harness: choose start test (id/index)
    loop for each selected test
        Harness->>E2E: execute test scenario in visible terminal
        E2E->>App: interact with application surfaces
        E2E-->>Harness: pass/fail result
        Harness-->>User: prompt [N] next or [X] fail
        alt user enters N
            Harness->>Harness: record accepted test
        else user enters X
            Harness->>App: capture surface snapshots and metadata
            Harness->>Logs: write artifact bundle
            Harness-->>User: print failure summary and artifact paths
            break
        end
    end
    Harness-->>User: print end-of-run readiness summary
```

**Behavior goals**:

- feedback is collected after each test, not after the full sequence.
- operator can start from any test to review newly added scenarios quickly.
- failure path always produces full surface-capture artifacts for agent analysis.
