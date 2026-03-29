# Hierarchical Task Execution — Design & Implementation Plan

**Created:** 2026-03-22  
**Status:** Draft  
**Target:** AgentX `0.12.x` series

---

## How to Use This Document

- `[ ]` Not started  
- `[/]` In progress — mark this **before** beginning a step  
- `[X]` Failed / blocked — append a note on the same line explaining why  
- `[✓]` Complete — replace `[ ]` or `[/]` when the step is done and tested  

Work through phases top-to-bottom within a session. At session start, scan this
file for the first `[ ]` item and mark it `[/]` before touching any code.  
At session end, confirm every touched step is either `[✓]` or `[X]`.

---

## Motivation

AgentX current flat tool loop:

```
user prompt → _run_tool_loop(max_rounds=10) → synthesis → response
```

Problems:

1. `max_rounds=10` is exhausted on large tasks before enough context is gathered.
2. All tool-call/result messages accumulate in a single context window — causes context bloat.
3. The model has no structured way to retain a "snapshot fact" from a sub-problem
   and proceed to the next without carrying all the raw data forward.
4. No audit trail of *why* a synthesis was accepted or rejected.
5. The GUI shows tool calls inline with the response, obscuring the reasoning structure.

---

## `run_task` vs `run_subtask` — Key Distinction

These are **not** two separate tools. Understanding the distinction in roles prevents confusion:

| Concept | Who drives it | Depth | How created |
|---------|--------------|-------|-------------|
| **Plan step** (a.k.a. "task") | The *system* walks the plan and launches each step | depth 0 | Scheduler reads planner JSON output step-by-step |
| **Sub-task** | The *LLM* requests it by calling `run_subtask(...)` | depth 1 … N | Tool call from within any executing node at any depth |

`run_subtask` is the **only** tool the LLM needs. The planner output is a structured list of step *descriptions*; the system iterates that list and executes each description as a depth-0 node. Inside any node the LLM may call `run_subtask` freely, creating branches. Those branches may recurse further. The tree is fractal: the same `run_subtask` mechanism applies at every level.

**Rule:** Plan steps are *scheduled*. Sub-tasks are *requested*. Both produce `TaskNodeRecord`s and are displayed identically in the tree — only `depth` and `plan_step_index` differ.

---

## Design Goals

| Goal | Description |
|------|-------------|
| **Bounded execution** | Each node caps at `max_tool_rounds`; depth caps at `max_task_depth` (default 10). Total LLM calls bounded by `max_tool_rounds^max_task_depth` |
| **Context compression** | Sub-task results collapse to a single synthesis string before the parent sees them |
| **Durable provenance** | Every sub-task, its tool calls, its synthesis attempts, and assertion verdicts are persisted to the session folder |
| **Re-entrant synthesis** | A rejected synthesis can be retried using stored raw data — no tools re-run |
| **Scratchpad support** | Model can write intermediate results to scratch files in the session folder; parent can `read_file` them |
| **Plans as first-class entities** | Each plan is named, persisted, and accessible as a distinct element in the conversation context; clicking it opens its own named output tab |
| **Dynamic / TBD steps** | Plan steps may be marked "To Be Determined" — the system fills them in after prerequisite steps complete, enabling discovery-first workflows |
| **Transparent UI** | A dedicated named tab per plan visualises the full execution graph with branch/join topology |
| **Reproducibility** | The recorded task tree + tool results + synthesis chain is sufficient to replay or audit the entire run |

---

## Architecture Overview

```
AgentixBridge.process_prompt_streaming()
  └── _stream_planned_response()
        ├── PlanRecord created, named ("Explore Bridge"), persisted, context panel entry emitted
        └── For each plan step (or single root node if no planner):
              _run_task_node(task_id="step-1", depth=0, plan_id=...)
                ├── Round 1-N: LLM may call:
                │     ├── any file / CST tool  →  leaf execution
                │     └── run_subtask(task=..., scratch_file=...)   ← recursive branch
                │           _run_task_node(task_id="sub-1", depth=1, parent="step-1")
                │             ├── Round 1-N: leaf tools OR further run_subtask (depth 1…9)
                │             └── Synthesis → AssertionCheck → TaskNodeRecord persisted
                │                         ↑
                │           result string ─┘  (parent sees only synthesis, not raw tool calls)
                └── Synthesis → plan step result
              ↓
        Final synthesis across all step results → streamed response
```

**Depth cap:** `max_task_depth = 10` (configurable). At `depth >= max_task_depth`, `run_subtask`
is removed from the LLM's tool list for that node. The cap is a guard rail, not an expected limit.  
**Max total LLM calls:** `max_tool_rounds^max_task_depth` — bounded but large by design.

### Dynamic / TBD Steps

A plan step may be emitted by the planner as `"tbd": true` with a dependency on a previous step:

```json
{ "step_id": "step-3", "description": "TBD — determine from findings in step-1 and step-2", "tbd": true, "depends_on": ["step-1", "step-2"] }
```

When step-3's prerequisites complete, the system re-queries the LLM with the synthesis results of
step-1 and step-2 to resolve the TBD description into a concrete task. The GUI shows TBD nodes with
a `?` indicator until they are resolved.

### Plan Naming

Each plan has a short human-readable name generated the LLM (part of the planner JSON output,
3–6 words). If the planner omits a name, the fallback is sequential: **"Plan 1"**, **"Plan 2"**,
etc., counting plans within the current session.

---

## New Persistent Artifacts per Session

```
sessions/<user>/<session_id>/
  context/
    <epoch>_task_node.json       ← NEW: one per task/sub-task node
    <epoch>_plan.json            ← NEW: one per plan (top-level record with step list)
    <epoch>_tool_call.json       ← unchanged
    <epoch>_tool_result.json     ← unchanged
    <epoch>_synthesis.json       ← NEW: versioned synthesis attempts
    <epoch>_assertion.json       ← NEW: per-field assertion verdicts
  scratch/                       ← NEW: LLM-writable scratchpad area
    step-1_output.md
    sub-1_summary.json
  task_tree.json                 ← NEW: full DAG index for plan tabs + history replay
```

### `plan.json` schema  *(new top-level record)*

```json
{
  "role": "plan",
  "plan_id": "plan-1",
  "plan_name": "Explore Bridge Architecture",
  "session_plan_index": 1,
  "steps": [
    { "step_id": "step-1", "description": "List and read all files in src/agentix/bridge", "tbd": false, "depends_on": [] },
    { "step_id": "step-2", "description": "Read src/agentix/api_client.py", "tbd": false, "depends_on": [] },
    { "step_id": "step-3", "description": "TBD — write summary based on steps 1 and 2", "tbd": true, "depends_on": ["step-1", "step-2"] }
  ],
  "root_task_ids": ["step-1", "step-2", "step-3"],
  "status": "running",
  "epoch": 1774215950.00
}
```

### `task_node.json` schema

```json
{
  "role": "task_node",
  "plan_id": "plan-1",
  "task_id": "step-1",
  "parent_task_id": null,
  "depth": 0,
  "plan_step_index": 0,
  "task_description": "List and read all files in src/agentix/bridge",
  "tbd": false,
  "tbd_resolved_description": null,
  "status": "synthesised",
  "child_message_epochs": [1774215953.12, 1774215954.93],
  "child_task_ids": ["sub-1"],
  "synthesis_epoch": 1774216000.50,
  "scratch_file": "scratch/step-1_output.md",
  "assertions": [
    { "fact": "AgentixBridge._run_tool_loop exists", "type": "exists", "verified": true },
    { "fact": "max_rounds is configurable", "type": "value",  "verified": true }
  ],
  "synthesis_attempts": [
    { "epoch": 1774216000.50, "status": "accepted", "rejected_epochs": [] }
  ],
  "wm_hints_added": [],
  "epoch": 1774215950.00,
  "enabled": true
}
```

**`status` values:** `pending | tbd | running | synthesised | invalidated | failed`

### `task_tree.json` schema

```json
{
  "session_id": "session_2026-03-22_21-45-08",
  "plans": [
    {
      "plan_id": "plan-1",
      "plan_name": "Explore Bridge Architecture",
      "session_plan_index": 1,
      "root_task_ids": ["step-1", "step-2", "step-3"],
      "plan_epoch": 1774215950.00
    }
  ],
  "nodes": {
    "step-1": { "task_id": "step-1", "plan_id": "plan-1", "children": ["sub-1"], "depth": 0 },
    "sub-1":  { "task_id": "sub-1",  "plan_id": "plan-1", "children": [],        "depth": 1 },
    "step-2": { "task_id": "step-2", "plan_id": "plan-1", "children": [],        "depth": 0 },
    "step-3": { "task_id": "step-3", "plan_id": "plan-1", "children": [],        "depth": 0, "tbd": true }
  },
  "created_epoch": 1774215950.00,
  "last_updated_epoch": 1774216000.50
}
```

---

## `run_subtask` Tool Schema

Registered as a normal tool via `register_tool_implementations()`. Removed from the
LLM's tool list when `depth >= max_task_depth`.

```python
def run_subtask(task: str, scratch_file: str = "") -> str:
    """
    Execute a complex sub-task using up to max_tool_rounds tool calls and return a
    synthesised summary. Use when a sub-problem requires multiple tool calls
    that would otherwise consume too many rounds in the parent context.

    The result of this sub-task is returned as a concise synthesis string.
    All raw tool call / tool result detail is preserved in the session folder
    but is NOT carried into the parent context — only the synthesis is visible
    to the parent LLM. This keeps the parent context small.

    Args:
        task: Full description of the sub-task including exactly what facts
              to return in the synthesis. Be specific and assertable.
        scratch_file: Optional relative path within the session scratch/
                      directory to write full results. The parent can read
                      this file if the inline summary is insufficient.

    Returns:
        Synthesised summary string. If scratch_file was provided the file
        is also written and can be retrieved with read_file.
    """
```

**Key behaviour difference from regular tool calls:**

| Aspect | Regular tool | `run_subtask` |
|--------|-------------|--------------|
| Context visibility | Full tool call + result in parent context | Only synthesis string visible to parent |
| Tool rounds consumed in parent | 1 (for the call itself) | 1 (the result is atomic from parent's view) |
| LLM calls consumed total | 1 | 1 to N (child has its own loop) |
| Persisted detail | Single tool_call + tool_result record | Full task_node record with nested tool calls |
| Recursion | No | Yes — child may call `run_subtask` further |

---

## Assertion Extraction & Verification

After each sub-task synthesis, a second (cheap) LLM call extracts structured
assertions from the synthesis text:

**Prompt template:**

```
From the following synthesis, extract every specific verifiable claim as JSON.
Return only: [{"fact": str, "type": "exists|value|count|regex", "check": str}]

Synthesis:
<synthesis text>
```

**Mechanical verification by type:**

| Type | Check method |
|------|-------------|
| `exists` | `os.path.exists(check)` or `grep_search(check, codebase)` |
| `value` | `check` appears literally in the named file |
| `count` | `len(results) >= int(check)` |
| `regex` | `re.search(check, target_text)` |

**On assertion failure:**  

- Persist bad synthesis as `status: "rejected"` in `synthesis_attempts`  
- Re-run synthesis with: bad synthesis as a negative example + original raw tool results  
- Up to `MAX_SYNTHESIS_RETRIES = 3` per task node  
- If all retries fail: `task_node.status = "failed"`, parent receives error string

---

## New Chunk Types

Add to `ChunkType` enum in `src/shared/models/response.py`:

```python
PLAN_START       = "plan_start"        # emitted when a new plan record is created (carries plan_id, plan_name)
TASK_NODE_START  = "task_node_start"   # emitted when a task/sub-task begins (carries task_id, depth, tbd flag)
TASK_NODE_TBD    = "task_node_tbd"     # emitted when a TBD step is resolved into a concrete description
TASK_NODE_END    = "task_node_end"     # emitted when a task/sub-task completes (carries synthesis)
ASSERTION_RESULT = "assertion_result"  # emitted after each assertion check
```

Add to `MessageRole` enum in `src/shared/models/message.py`:

```python
PLAN        = "plan"        # top-level plan record (persisted plan.json)
TASK_NODE   = "task_node"   # persisted task node record
SYNTHESIS   = "synthesis"   # versioned synthesis attempt
ASSERTION   = "assertion"   # per-assertion verdict record
```

## Plans as Context Panel Elements

Plans appear in the conversation context panel (system pane) as a new element type at the
message level — alongside tool calls and attachments, as a sibling of the `assistant` message
that contains them:

```
┌ Context ──────────────────────────────────────────────────────┐
│  👤 [User]  "Document the agentix bridge architecture"        │
│  🤖 [Assistant]  ← collapsed; has children                   │
│      📋 [Plan: Explore Bridge Architecture]  ← NEW element   │
│          clicking → opens / focuses "Explore Bridge..." tab  │
│      🔧 [tool_call: list_directory]  ← existing              │
│      📎 [attachment: bridge.py]      ← existing (if any)     │
│  🤖 [Assistant]  "Here is a summary of the bridge..."        │
└───────────────────────────────────────────────────────────────┘
```

**Rules:**

- One context element per plan (even if the plan has many steps).
- The element shows: plan icon + plan name + step count + final status badge.
- Clicking the element opens (or focuses) the corresponding named output tab.
- If a session is reloaded from history, plan elements are reconstructed from `plan.json` records and the tab is shown in read-only replay mode.
- A session may contain multiple plans (one per user prompt that triggers the planner). Each gets its own context element and its own output tab.

---

## Task Tree Tab (GUI)

Each plan gets its own dedicated tab in `output_notebook`, named after the plan.  
Tab label: **`📋 <plan_name>`** (e.g. `📋 Explore Bridge Architecture`).  
If a session has two plans, two tabs appear: `📋 Plan 1` · `📋 Explore GUI`.

The tab is created when the `PLAN_START` chunk is received. It is NOT created for
`respond_directly` or `single_tool` flows.

### Visual layout — branch/join topology

The tree renders the fractal execution structure visually. Sequential plan steps are shown
left-to-right or top-to-bottom as a main trunk. Sub-tasks branch off a step and rejoin with their
synthesised result. TBD steps show a question-mark placeholder until resolved.

```
┌ Output ─────────────────────────────────────────────────────────────┐
│  [Output] [📋 Explore Bridge Architecture ●] [📋 Plan 2]            │
├─────────────────────────────────────────────────────────────────────┤
│  Plan: Explore Bridge Architecture        [Export] [Replay]         │
│                                                                     │
│  ●─── step-1: List bridge files ──────────────────────────── [✓]   │
│  │       └─ sub-1: Read _run_tool_loop detail  [✓]                  │
│  │            🔧 read_file bridge.py                                │
│  │            💡 "_run_tool_loop manages rounds..."                 │
│  │            ✓ _run_tool_loop exists  ✓ max_rounds configurable   │
│  │            [Re-synthesise]  [Add WM hint]                       │
│  │       synthesis → "Bridge has 3 routing paths, loop at line 580"│
│  │                                                                  │
│  ●─── step-2: Read api_client.py ─────────────────────────── [✓]   │
│  │       🔧 read_file api_client.py                                 │
│  │       💡 "api_client wraps Ollama HTTP..."                       │
│  │                                                                  │
│  ●─── step-3: TBD ─────────────────────────────────────────── [?]   │
│  │       ⏳ Waiting for step-1, step-2...                           │
│  │                                                                  │
│  ●─── step-4: Write summary ───────────────────────────────── [⏳]  │
└─────────────────────────────────────────────────────────────────────┘
```

**Node connector lines** (`│` and `├─`) are drawn with canvas or nested frames to convey
parent→child branching and the join-back (synthesis) visually.  
Sub-tasks indent by `depth × 20px` and draw a branch line back up to their parent step.

**Status icons:** `✓` synthesised · `⏳` running · `○` pending · `?` TBD (not yet resolved) · `✗` failed · `↻` retrying

**"Re-synthesise" button** — opens a modal:

1. Shows the current synthesis text (editable hint field).
2. Shows assertion failures.
3. Optionally pre-populates a WM fact ("Add context hint").
4. On confirm: re-runs synthesis using stored raw tool results (no new tool calls).

**"Add WM hint" button** — opens `WorkingMemory` editor pre-filtered to the task's scope.

---

## Implementation Phases

---

### Phase 1 — Data Model & Persistence

**Goal:** New message roles, plan/task node records, and serialisation in place. No behaviour changes yet.

- [✓] Add `PLAN`, `TASK_NODE`, `SYNTHESIS`, `ASSERTION` to `MessageRole` enum in `src/shared/models/message.py`
- [✓] Add serialisation/deserialisation for these roles in `Message.to_dict()` and `Message.from_dict()`
- [✓] Add `PLAN_START`, `TASK_NODE_START`, `TASK_NODE_TBD`, `TASK_NODE_END`, `ASSERTION_RESULT` to `ChunkType` in `src/shared/models/response.py`
- [✓] Add corresponding fields to `ResponseChunk` dataclass: `plan_id`, `plan_name`, `task_id`, `parent_task_id`, `task_depth`, `tbd`, `assertions`
- [✓] Create `PlanRecord` dataclass in `src/shared/models/task_node.py` (mirrors `plan.json` schema above)
- [✓] Create `TaskNodeRecord` dataclass in the same file (mirrors `task_node.json` schema above); include `tbd`, `tbd_resolved_description`, `plan_step_index`, `child_task_ids`
- [✓] Add `save_plan()`, `load_plans()`, `save_task_node()`, `load_task_nodes()` methods to `Context`
- [✓] Create `sessions/<user>/<session_id>/scratch/` directory on session init
- [✓] Add `task_tree.json` read/write helpers to `Context`; structure supports multiple plans per session
- [✓] Write unit tests for serialisation round-trip: plan record, task node, TBD step, task_tree index

---

### Phase 2 — `run_subtask` Tool & Recursive Loop

**Goal:** The bridge can spawn and run a depth-bounded child tool loop; plans are created, named, and emitted.

- [✓] Add `depth` and `plan_id` parameters to `_run_tool_loop` in `src/agentix/bridge/bridge.py`
- [✓] Create `_run_task_node(plan_id, task_id, task_description, parent_task_id, depth, plan_step_index, tbd, context, scratch_path)` method in bridge
- [✓] `_run_task_node` emits `TASK_NODE_START` chunk at entry, `TASK_NODE_END` chunk on exit
- [✓] `_run_task_node` persists `task_node.json` and updates `task_tree.json` via `Context`
- [✓] Create `_run_plan(plan_record, context)` method — iterates plan steps, executes each as `_run_task_node(depth=0)`, resolves TBD steps after their prerequisites synthesise
- [✓] TBD resolution: after prerequisite steps complete, call LLM with synthesis results to resolve `tbd_resolved_description`; emit `TASK_NODE_TBD` chunk with resolved description
- [✓] Plan creation: `_create_plan(prompt, steps, plan_name)` — creates `PlanRecord`, persists `plan.json`, emits `PLAN_START` chunk, updates `task_tree.json`
- [✓] Update `_stream_planned_response` to: (1) call `_create_plan()` with planner output, (2) call `_run_plan()` instead of `_run_tool_loop`
- [✓] Add `plan_name` field to planner JSON schema in `system_prompts/planner_prompt.md` (3–6 word name describing the plan intent)
- [✓] Implement `run_subtask` Python function (docstring-driven schema via `extract_tool_schema`)
- [✓] Register `run_subtask` in `AgentixBridgeAdapter._register_client_tools()` — removed from tool list when `depth >= max_task_depth`
- [✓] Wire `execute_tool("run_subtask", ...)` dispatch in `bridge.execute_tool()` to call `_run_task_node`
- [✓] Update `agentx.toml` to add `max_task_depth = 10` and `max_synthesis_retries = 3` config keys
- [✓] Create `system_prompts/task_execution.md` covering: role framing, when to use `run_subtask`, synthesis contract (self-contained, assertable, 50–200 words), scratch file pattern, scope discipline, depth-limit awareness (see "System Prompts — What Goes Where" section above for full content outline)
- [✓] Wire `task_execution.md` loading into `_run_task_node` — injected as a `system` role message before round 1 of every node; NOT loaded for direct-response or single-tool paths
- [✓] Update `system_prompts/planner_prompt.md`: add `plan_name` (3–6 word string), `tbd` (bool), and `depends_on` (array of step IDs) to the step schema; add TBD resolution instructions
- [✓] Write unit tests: plan created → steps executed sequentially → TBD step resolved after deps → sub-task spawned → depth cap enforced

---

### Phase 3 — Synthesis Assertion Engine

**Goal:** After each sub-task synthesis, structured facts are extracted and mechanically verified.

- [✓] Create `src/agentix/bridge/assertion_checker.py` with `extract_assertions(synthesis_text, config)` — calls LLM with constrained JSON prompt
- [✓] Implement `verify_assertion(assertion: dict, session_path: str)` — handles `exists`, `value`, `count`, `regex` types
- [✓] Integrate assertion check into `_run_task_node`: after synthesis, call `extract_assertions` then `verify_assertion` for each
- [✓] Store assertion results in `task_node.json` `assertions` array
- [✓] Emit `ASSERTION_RESULT` chunk for each assertion (GUI can display pass/fail live)
- [✓] Implement re-synthesis loop: on failure, rebuild prompt with bad synthesis as negative example and raw tool results, retry up to `max_synthesis_retries`
- [✓] Persist each synthesis attempt in `synthesis_attempts` array with status
- [✓] Write unit tests for assertion extraction and each verification type

---

### Phase 4 — Plan Tab (GUI Shell)

**Goal:** Each plan gets its own named tab in `output_notebook` that renders the branch/join tree live.

- [✓] Add `add_plan_tab(plan_id, plan_name) -> tk.Frame` to `GUIManager` — creates a named tab in `output_notebook` using `plan_name` as label; returns the tab frame
- [✓] Add `plan_tab` frames to `WidgetRegistry` lifecycle; keyed by `plan_id`
- [✓] Add `add_plan_tab()`, `get_plan_tab_frame(plan_id)`, `focus_plan_tab(plan_id)` to `IGUIManager` protocol
- [✓] In `session.py` `_stream_via_agentix()`: on `PLAN_START` chunk, call `gui.add_plan_tab(plan_id, plan_name)`; switch focus to the new tab
- [✓] Add tab toolbar: plan name label + `[Export]` + `[Replay]` buttons
- [✓] Create `src/agentx/gui/plan_tree_widget.py` — `PlanTreeWidget` class managing the scrollable canvas or frame tree
- [✓] `PlanTreeWidget.add_step_node(plan_id, task_id, description, tbd)` — renders a main-trunk step row with connector
- [✓] `PlanTreeWidget.add_subtask_node(task_id, parent_task_id, description, depth)` — renders an indented sub-task row with branch line to parent
- [✓] `PlanTreeWidget.update_node_status(task_id, status)` — updates status icon live during streaming
- [✓] `PlanTreeWidget.resolve_tbd_node(task_id, resolved_description)` — replaces `?` placeholder with resolved description
- [✓] `PlanTreeWidget.add_tool_call_to_node(task_id, tool_name, tool_input)` — appends collapsed tool row under node
- [✓] `PlanTreeWidget.add_synthesis_to_node(task_id, synthesis_text, assertions)` — appends synthesis block with assertion badges
- [✓] Draw branch connector lines: sub-tasks indent by `depth × 20 px`; a vertical connector line runs from parent node to child; collapsed by default, expand on click
- [✓] Wire chunk handlers in `session.py`: `PLAN_START` → `add_plan_tab`, `TASK_NODE_START` → `add_step_node` or `add_subtask_node`, `TASK_NODE_TBD` → `resolve_tbd_node`, `TASK_NODE_END` → `update_node_status + add_synthesis_to_node`, `TOOL_CALL` (with task_id) → `add_tool_call_to_node`
- [✓] Switch output notebook focus to the plan tab automatically while plan is executing; no forced switch after completion

---

### Phase 5 — Re-synthesis UI

**Goal:** User can trigger synthesis retry from the Task Tree tab with optional WM hints.

- [✓] Add `[Re-synthesise]` button to each synthesis block in `TaskTreeWidget`
- [✓] Clicking opens a `ResynthesisDialog` (modal Toplevel): shows current synthesis, assertion failures, hint field
- [✓] `ResynthesisDialog` on confirm: calls `session.retrigger_synthesis(task_id, hint)` method
- [✓] Implement `session.retrigger_synthesis(task_id, hint)`: loads raw tool results from session `context/`, optionally adds hint to `WorkingMemory`, re-runs synthesis (no tool calls)
- [✓] Add `[Add WM hint]` button: opens `WorkingMemory` editor pre-filtered; on save, marks task node as `invalidated` (requires re-synthesis)
- [✓] Persist the new synthesis attempt alongside previous ones (non-destructive; all attempts kept)
- [✓] Update `task_tree.json` and `TaskTreeWidget` live to reflect new synthesis result

---

### Phase 6 — Context Panel Integration

**Goal:** The system pane context panel shows plan records as first-class elements; clicking opens the named tab.

- [✓] Add `PLAN` role rendering in `GUIManager.render_message_row()` — renders as **plan element row**: `📋 Explore Bridge Architecture  [3 steps ✓]`; placed at the same level as tool calls and attachments under the assistant message that triggered it
- [✓] Plan element row click → `gui.focus_plan_tab(plan_id)`; if reloading from history and tab doesn't exist yet → reconstruct tab from `plan.json` + `task_node.json` records (read-only replay)
- [✓] Add `TASK_NODE` role rendering — compact row: `🌿 step-1 [✓] — "List bridge files"` with child count badge and indentation based on depth; shown nested under its plan element row
- [✓] Task node rows have an `enabled` toggle (checkbox): disabled nodes are dimmed; their nested tool_call/tool_result messages are suppressed from LLM context re-submissions
- [✓] TBD task node rows render with `?` icon and italic description until resolved; update in-place on `TASK_NODE_TBD` chunk

---

### Phase 7 — Replay & Export

**Goal:** A full plan run can be replayed or exported for reproducibility.

- [ ] Add `[Export Task Tree]` button in the Task Tree tab toolbar — writes `task_tree_export.md` to session folder in human-readable form (plan → sub-tasks → tool calls → synthesis → assertions)
- [ ] Add `[Replay Sub-task]` button per node — re-runs the child loop from scratch (new tool calls), appends new synthesis attempt alongside old
- [ ] Add `open_task_tree(session_path)` to `HistoryLoader` — allows loading and inspecting task trees from prior sessions
- [ ] Document the `task_node.json` / `task_tree.json` schema in `docs/architecture.md`

---

## System Prompts — What Goes Where

This feature introduces three distinct LLM call contexts. Each needs targeted
instruction; none should receive the others' content as noise.

| Call context | Fires when | New system prompt | Rationale |
|---|---|---|---|
| **Planning phase** | Once, before any execution | Update `planner_prompt.md` | Existing prompt generates the plan JSON; just needs new schema fields (`plan_name`, `tbd`, `depends_on`). No new file. |
| **Task node execution** | Every LLM round inside every task node at any depth | **New: `task_execution.md`** | The LLM must reason about delegation, synthesis scope, and scratch files. This is irrelevant to simple direct-response or single-tool flows — injecting it there adds noise and wastes context tokens. `_run_task_node` loads it; `_run_tool_loop` for non-plan paths does not. |
| **TBD resolution** | Once per TBD step after prerequisites complete | Inline dynamic message (no file) | This call is a single narrow instruction: "given these synthesis results, write a concrete step description". Short enough to construct at call time. A file would be over-engineering. |

**Why `tool_use.md` is NOT the right place for `run_subtask` guidance:**  
`tool_use.md` is loaded for all tool-using LLM calls — including the majority that
never involve a plan. Adding task-execution semantics there would clutter every
single-tool interaction. More importantly, the LLM needs a *different orientation*
inside a task node: it should be working toward a synthesisable, assertable result
and actively deciding whether to delegate sub-problems. That orientation is
meaningless and confusing outside of a plan context.

### `system_prompts/task_execution.md` — Content Outline

This prompt is injected as a `system` message by `_run_task_node` before the first
LLM round of every node. It must cover:

1. **Role framing** — "You are executing one step in a multi-step plan. Your output
   for this step will be synthesised into a concise summary that the parent context
   will use. Only the synthesis is visible to the parent — not the raw tool calls."
2. **When to use `run_subtask`** — delegate when a sub-problem requires 3+ tool
   calls to explore independently and its result can be expressed as a compact
   fact. Do NOT use it for a single targeted lookup; call the leaf tool directly.
3. **Synthesis contract** — the synthesis must: (a) be self-contained without
   reference to the raw tool output, (b) express every key finding as an assertable
   claim (e.g. "File X exists at path Y", "Function Z takes N arguments"), (c) be
   50–200 words.
4. **Scratch file usage** — if findings are too large for inline synthesis, write
   them to `scratch_file` and reference the path in the synthesis so the parent
   can `read_file` it.
5. **Scope discipline** — you have `max_tool_rounds` rounds in this node. Do not
   try to complete the entire plan in one node. Stop when you have enough to
   synthesise the assigned step.
6. **Depth awareness** — if `run_subtask` is not in your tool list, you are at the
   depth limit; handle everything with leaf tools directly.

---

## Configuration Keys (add to `agentx.toml`)

```toml
[agentix]
max_tool_rounds       = 10     # rounds per node (existing)
max_task_depth        = 10     # max recursive sub-task depth (10 = practical max; avoid > 10)
max_synthesis_retries = 3      # retries before task_node.status = "failed"
scratch_dir           = "scratch"  # relative to session folder
```

---

## File Map — New Files

| File | Purpose |
|------|---------|
| `src/shared/models/task_node.py` | `PlanRecord` + `TaskNodeRecord` dataclasses + `task_tree.json` helpers |
| `src/agentix/bridge/assertion_checker.py` | Assertion extraction (LLM) + verification (mechanical) |
| `src/agentx/gui/plan_tree_widget.py` | `PlanTreeWidget` — branch/join tree renderer with connector lines |
| `src/agentx/gui/resynthesis_dialog.py` | Modal re-synthesis UI |
| `system_prompts/task_execution.md` | System prompt injected by `_run_task_node` only: role framing, `run_subtask` decision rules, synthesis contract, scratch file pattern, scope/depth guidance |

---

## File Map — Modified Files

| File | Change |
|------|--------|
| `src/shared/models/message.py` | Add `PLAN`, `TASK_NODE`, `SYNTHESIS`, `ASSERTION` roles |
| `src/shared/models/response.py` | Add `PLAN_START`, `TASK_NODE_START`, `TASK_NODE_TBD`, `TASK_NODE_END`, `ASSERTION_RESULT` chunk types; extend `ResponseChunk` with `plan_id`, `plan_name`, `task_id`, `task_depth`, `tbd` |
| `src/shared/models/context.py` | Add `save_plan()`, `load_plans()`, `save_task_node()`, `load_task_nodes()`, `task_tree` read/write; create `scratch/` on session init |
| `src/agentix/bridge/bridge.py` | Add `_create_plan()`, `_run_plan()`, `_run_task_node()`; add `depth`+`plan_id` to `_run_tool_loop`; register `run_subtask`; update `execute_tool` dispatch; TBD resolution logic |
| `src/agentix/integration/agentix_bridge_adapter.py` | Register `run_subtask` tool; respect `max_task_depth` config for suppression |
| `src/agentx/igui_manager.py` | Add `add_plan_tab()`, `get_plan_tab_frame()`, `focus_plan_tab()` to protocol |
| `src/agentx/gui/gui_manager.py` | Implement new `IGUIManager` methods; plan tab creation; plan + task_node element rendering in context panel |
| `src/agentx/session.py` | Handle new chunk types (`PLAN_START`, `TASK_NODE_*`, `ASSERTION_RESULT`); add `retrigger_synthesis()` |
| `system_prompts/planner_prompt.md` | Add `plan_name`, `tbd`, `depends_on` fields to step schema; add TBD resolution instructions |
| `agentx.toml` | Add `max_task_depth`, `max_synthesis_retries`, `scratch_dir` |
| `docs/architecture.md` | Document `PlanRecord`, `TaskNodeRecord`, new chunk types, `task_tree.json` schema, plan tab pattern |
| `pyproject.toml` | Version bump to `0.12.0` (new minor — backward-compatible feature) |

---

## Testing Strategy

Each phase ships with unit tests before the phase is marked complete.

| Phase | Test file | Key scenarios |
|-------|-----------|---------------|
| 1 | `tests/test_task_node_model.py` | Serialise/deserialise round-trip; `task_tree.json` index |
| 2 | `tests/test_subtask_loop.py` | Sub-task spawned → child executes → result in parent; depth cap enforced |
| 3 | `tests/test_assertion_checker.py` | Each assertion type passes/fails; re-synthesis with negative example |
| 2 | `tests/test_subtask_loop.py` | Plan created + named; steps execute sequentially; TBD resolved after deps; depth cap enforced |
| 4 | `tests/test_plan_tree_widget.py` | Step/subtask node add/update; TBD placeholder replaced; connector lines drawn; status icons |
| 5 | `tests/test_resynthesis_dialog.py` | Dialog opens; hint added to WM; synthesis attempt persisted |
| 6 | `tests/test_context_plan_render.py` | Plan element row renders; clicking focuses correct tab; disabled node suppressed from LLM context |
| 7 | `tests/test_plan_export.py` | Export markdown; replay spawns new tool calls |

---

## Open Questions

1. **Assertion extraction model** — Use the same `classification_model` (phi4-mini) or a separate cheaper call? Assertion prompts are short so phi4-mini should suffice. **Resolution: use phi4-mini; revisit if extraction quality is poor.**
2. **Scratch file path safety** — `scratch_file` argument from LLM must be sanitised to prevent path traversal. Resolve against session `scratch/` dir and reject `..` components. **Resolution: enforce in Phase 2 during `_run_task_node` argument handling.**
3. **Plan tab persistence across sessions** — When re-opening a prior session in History, plan tabs should be reconstructed from `plan.json` + `task_node.json` records. **Resolution: yes, read-only replay view; implement in Phase 6/7.**
4. **`run_subtask` visibility** — Should the user be able to disable `run_subtask` from the tool toggles panel? Proposed: yes, disabling falls back to the current flat loop. **Resolution: implement as part of Phase 2; wire into existing tool-toggle infrastructure.**
5. **Plan naming fallback** — If the LLM omits `plan_name` from the planner JSON, fall back to `"Plan N"` (sequential within session). Sequential index stored in `task_tree.json` → `plans[].session_plan_index`. **Resolution: handle in `_create_plan()`.**
6. **Multiple plans per session** — A single conversation may trigger the planner multiple times. Each plan gets its own `plan.json`, its own context element, and its own output tab. Old plan tabs remain open (not closed). **Resolution: supported by the per-`plan_id` tab keying.**
7. **TBD resolution failure** — If the LLM cannot resolve a TBD step (e.g., prerequisites returned no useful data), the step status becomes `"failed"` and the plan reports it. **Resolution: add `tbd_resolution_failed` status; plan execution continues with remaining non-TBD steps.**
