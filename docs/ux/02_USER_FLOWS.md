# AgentX — User Flows

Version: 2026-04-19

Each section contains a Mermaid sequence or flowchart diagram documenting how
user actions map to system behaviour.

---

## UF-01: Basic Chat (Direct Response)

**Trigger**: User types a message and presses `Enter` (or clicks `Send`).

```mermaid
sequenceDiagram
    actor User
    participant InputPanel
    participant Session as AgentXSession
    participant Bridge as AgentixBridgeAdapter
    participant LLM as Ollama
    participant Chat as ChatPanel

    User->>InputPanel: types message, presses Enter
    InputPanel->>Session: on_submit()
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
    actor User
    participant Session as AgentXSession
    participant Bridge as AgentixBridgeAdapter
    participant ToolLoop as ToolLoopRunner
    participant ToolDisp as ToolDispatcher
    participant LLM as Ollama
    participant Chat as ChatPanel

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
    actor User
    participant Session as AgentXSession
    participant Bridge as AgentixBridge
    participant LLM as Ollama
    participant Chat as ChatPanel
    participant Tree as PlanTreeWidget

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
    actor User
    participant Tree as PlanTreeWidget
    participant Dialog as ResynthesisDialog
    participant Session as AgentXSession
    participant Bridge as AgentixBridge
    participant Chat as ChatPanel

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

**Trigger**: User clicks a file in the `FileExplorer` or drags one to the input area.

```mermaid
sequenceDiagram
    actor User
    participant Explorer as FileExplorer
    participant Session as AgentXSession
    participant InputPanel
    participant Context

    User->>Explorer: clicks a file in Files tab
    Explorer->>Session: on_file_open(path)
    Session->>Session: add AttachmentInfo to pending_attachments
    Session->>InputPanel: update_attachment_bar(current, history)
    InputPanel->>InputPanel: render chip: 📎 filename.py [×]
    User->>InputPanel: submits message
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
    actor User
    participant Selector as ModelSelector
    participant Session as AgentXSession
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
    actor User
    participant Settings as SettingsTab
    participant Session as AgentXSession
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
    actor User
    participant SidePanel
    participant Session as AgentXSession
    participant History
    participant Chat as ChatPanel

    User->>SidePanel: clicks prior session entry in Session tab
    SidePanel->>Session: on_session_select(session_id)
    Session->>History: load_context(session_id)
    History-->>Session: Context object
    Session->>Chat: render_history_widget(context)
    Chat->>Chat: display all historical messages (read-only)
```

---

## UF-09: Working Memory Management

**Trigger**: User interacts with a fact in the Working Memory panel.

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
    actor User
    participant InputPanel
    participant Session as AgentXSession
    participant Controller as StreamingController

    User->>InputPanel: clicks Stop button
    InputPanel->>Session: on_interrupt()
    Session->>Controller: interrupt()
    Controller->>Controller: _interrupt_flag = True
    Controller->>Controller: _is_streaming.clear()
    Note right of Controller: generator yields nothing\non next iteration;\nstream terminates gracefully
    Session->>Chat: display partial response (up to interrupt point)
    Session->>Session: persist partial message to Context
```
