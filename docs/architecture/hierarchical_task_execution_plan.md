# Hierarchical Task Execution — Design Plan

**Created:** 2026-03-22  
**Status:** Draft

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
user prompt → runToolLoop(max_rounds=10) → synthesis → response
```

Problems:

1. `max_rounds=10` is exhausted on large tasks before enough context is gathered.
2. All tool-call/result messages accumulate in a single context window — causes context bloat.
3. The model has no structured way to retain a "snapshot fact" from a sub-problem
   and proceed to the next without carrying all the raw data forward.
4. No audit trail of *why* a synthesis was accepted or rejected.
5. The output surface shows tool calls inline with the response, obscuring the reasoning structure.

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
Orchestrator.processPrompt()
  └── streamPlannedResponse()
        ├── PlanRecord created, named, persisted, context panel entry emitted
        └── For each plan step (or single root node if no planner):
              runTaskNode(task_id="step-1", depth=0, plan_id=...)
                ├── Round 1-N: LLM may call:
                │     ├── any file / search tool  →  leaf execution
                │     └── run_subtask(task=..., scratch_file=...)   ← recursive branch
                │           runTaskNode(task_id="sub-1", depth=1, parent="step-1")
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
step-1 and step-2 to resolve the TBD description into a concrete task. The output surface shows TBD nodes with
a `?` indicator until they are resolved.

### Plan Naming

Each plan has a short human-readable name generated by the LLM (part of the planner JSON output,
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

### `plan.json` schema *(new top-level record)*

```json
{
  "role": "plan",
  "plan_id": "plan-1",
  "plan_name": "Explore Bridge Architecture",
  "session_plan_index": 1,
  "steps": [
    { "step_id": "step-1", "description": "List and read all files in the bridge module", "tbd": false, "depends_on": [] },
    { "step_id": "step-2", "description": "Read the API client source", "tbd": false, "depends_on": [] },
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
  "task_description": "List and read all files in the bridge module",
  "tbd": false,
  "tbd_resolved_description": null,
  "status": "synthesised",
  "child_message_epochs": [1774215953.12, 1774215954.93],
  "child_task_ids": ["sub-1"],
  "synthesis_epoch": 1774216000.50,
  "scratch_file": "scratch/step-1_output.md",
  "assertions": [
    { "fact": "LLMBridge routing logic exists", "type": "exists", "verified": true },
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

## `run_subtask` Tool

`run_subtask` executes a complex sub-task using up to `max_tool_rounds` tool calls and returns a
synthesised summary. It is used when a sub-problem requires multiple tool calls that would otherwise
consume too many rounds in the parent context.

The result of the sub-task is returned as a concise synthesis string. All raw tool call / tool result
detail is preserved in the session folder but is NOT carried into the parent context — only the synthesis
is visible to the parent LLM. This keeps the parent context small.

Parameters:
- `task` — full description of the sub-task including exactly what facts to return in the synthesis
- `scratch_file` — optional relative path within the session `scratch/` directory to write full results

**Key behaviour difference from regular tool calls:**

| Aspect | Regular tool | `run_subtask` |
|--------|-------------|--------------|
| Context visibility | Full tool call + result in parent context | Only synthesis string visible to parent |
| Tool rounds consumed in parent | 1 (for the call itself) | 1 (the result is atomic from parent's view) |
| LLM calls consumed total | 1 | 1 to N (child has its own loop) |
| Persisted detail | Single tool_call + tool_result record | Full task_node record with nested tool calls |
| Recursion | No | Yes — child may call `run_subtask` further |

`run_subtask` is removed from the LLM's tool list when `depth >= max_task_depth`.

---

## New Chunk Types

These chunk types are emitted by the runtime during plan execution:

- `plan_start` — emitted when a new plan record is created (carries plan_id, plan_name)
- `task_node_start` — emitted when a task/sub-task begins (carries task_id, depth, tbd flag)
- `task_node_tbd` — emitted when a TBD step is resolved into a concrete description
- `task_node_end` — emitted when a task/sub-task completes (carries synthesis)
- `assertion_result` — emitted after each assertion check

New message roles for persistence:

- `plan` — top-level plan record (persisted plan.json)
- `task_node` — persisted task node record
- `synthesis` — versioned synthesis attempt
- `assertion` — per-assertion verdict record

---

## Plans as Context Panel Elements

Plans appear in the conversation context panel (system pane) as a new element type at the
message level — alongside tool calls and attachments, as a sibling of the `assistant` message
that contains them:

```
┌ Context ──────────────────────────────────────────────────────┐
│  👤 [User]  "Document the bridge architecture"                │
│  🤖 [Assistant]  ← collapsed; has children                   │
│      📋 [Plan: Explore Bridge Architecture]  ← NEW element   │
│          clicking → opens / focuses "Explore Bridge..." tab  │
│      🔧 [tool_call: list_directory]  ← existing              │
│      📎 [attachment: bridge_module]  ← existing (if any)     │
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

## Task Tree Tab

Each plan gets its own dedicated tab in the OutputSurface tabbed view, named after the plan.  
Tab label: **`📋 <plan_name>`** (e.g. `📋 Explore Bridge Architecture`).  
If a session has two plans, two tabs appear: `📋 Plan 1` · `📋 Explore Architecture`.

The tab is created when the `PLAN_START` chunk is received. It is NOT created for
`respond_directly` or `single_tool` flows.

### Visual layout — branch/join topology

The PlanView renders the fractal execution structure visually. Sequential plan steps are shown
left-to-right or top-to-bottom as a main trunk. Sub-tasks branch off a step and rejoin with their
synthesised result. TBD steps show a question-mark placeholder until resolved.

```
┌ Output ─────────────────────────────────────────────────────────────┐
│  [Output] [📋 Explore Bridge Architecture ●] [📋 Plan 2]            │
├─────────────────────────────────────────────────────────────────────┤
│  Plan: Explore Bridge Architecture        [Export] [Replay]         │
│                                                                     │
│  ●─── step-1: List bridge files ──────────────────────────── [ ]   │
│  │       └─ sub-1: Read routing detail  [ ]                         │
│  │            🔧 read_file bridge_module                            │
│  │            💡 "LLMBridge has 3 routing paths..."                 │
│  │            ✓ routing logic exists  ✓ max_rounds configurable    │
│  │            [Re-synthesise]  [Add WM hint]                       │
│  │       synthesis → "Bridge has 3 routing paths"                  │
│  │                                                                  │
│  ●─── step-2: Read API client ─────────────────────────────── [ ]   │
│  │       🔧 read_file api_client                                    │
│  │       💡 "API client wraps Ollama HTTP..."                       │
│  │                                                                  │
│  ●─── step-3: TBD ─────────────────────────────────────────── [ ]   │
│  │       ⏳ Waiting for step-1, step-2...                           │
│  │                                                                  │
│  ●─── step-4: Write summary ───────────────────────────────── [ ]  │
└─────────────────────────────────────────────────────────────────────┘
```

**Status icons:** `✓` synthesised · `⏳` running · `○` pending · `?` TBD (not yet resolved) · `✗` failed · `↻` retrying

**"Re-synthesise" button** — opens a ResynthesisDialog (modal):

1. Shows the current synthesis text (editable hint field).
2. Shows assertion failures.
3. Optionally pre-populates a working memory fact ("Add context hint").
4. On confirm: re-runs synthesis using stored raw tool results (no new tool calls).

**"Add WM hint" button** — opens the working memory editor pre-filtered to the task's scope.

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
| `exists` | path existence check or text search in codebase |
| `value` | `check` appears literally in the named file |
| `count` | result count meets or exceeds the expected value |
| `regex` | pattern match against target text |

**On assertion failure:**  

- Persist bad synthesis as `status: "rejected"` in `synthesis_attempts`  
- Re-run synthesis with: bad synthesis as a negative example + original raw tool results  
- Up to `MAX_SYNTHESIS_RETRIES = 3` per task node  
- If all retries fail: `task_node.status = "failed"`, parent receives error string

---

## Re-synthesis UI

User can trigger synthesis retry from the Task Tree tab with optional working memory hints:

- Each synthesis block in the PlanView has a `[Re-synthesise]` button.
- Clicking opens a ResynthesisDialog (modal): shows current synthesis, assertion failures, hint field.
- On confirm: loads raw tool results from session `context/`, optionally adds hint to working memory, re-runs synthesis (no tool calls).
- A `[Add WM hint]` button opens the working memory editor pre-filtered; on save, marks the task node as `invalidated` (requires re-synthesis).
- New synthesis attempts are persisted alongside previous ones (non-destructive; all attempts kept).

---

## Context Panel Integration

- Plans appear in the context panel as `plan` role elements rendered as: `📋 Explore Bridge Architecture  [3 steps ✓]`; placed at the same level as tool calls and attachments under the assistant message that triggered it.
- Clicking a plan element opens (or focuses) the corresponding named tab; reloading from history reconstructs tabs from `plan.json` + `task_node.json` records (read-only replay).
- Task node rows show: `🌿 step-1 [ ] — "List bridge files"` with child count badge and indentation based on depth.
- Task node rows have an `enabled` toggle: disabled nodes are dimmed; their nested tool_call/tool_result messages are suppressed from LLM context re-submissions.
- TBD task node rows render with `?` icon and italic description until resolved; update in-place on `task_node_tbd` chunk.

---

## Replay & Export

- `[Export Task Tree]` button in the Task Tree tab toolbar — writes `task_tree_export.md` to session folder in human-readable form (plan → sub-tasks → tool calls → synthesis → assertions).
- `[Replay Sub-task]` button per node — re-runs the child loop from scratch (new tool calls), appends new synthesis attempt alongside old.
- History loader allows loading and inspecting task trees from prior sessions.

---

## Implementation

Implementation phases for this feature are tracked in `docs/implementation/06_delivery_plan.md`.
Refer to that document for phased delivery milestones, acceptance criteria, and go/no-go gates.
Implementation details (package structure, data type definitions, test coverage requirements) are defined per the active delivery plan.

---

## Open Questions

1. **Assertion extraction model** — Use the same classification model or a separate cheaper call? Assertion prompts are short so a small model should suffice. **Resolution: use the smallest capable model; revisit if extraction quality is poor.**
2. **Scratch file path safety** — `scratch_file` argument from LLM must be sanitised to prevent path traversal. Resolve against session `scratch/` dir and reject `..` components. **Resolution: enforce during `runTaskNode` argument handling.**
3. **Plan tab persistence across sessions** — When re-opening a prior session in History, plan tabs should be reconstructed from `plan.json` + `task_node.json` records. **Resolution: yes, read-only replay view.**
4. **`run_subtask` visibility** — Should the user be able to disable `run_subtask` from the tool toggles panel? Proposed: yes, disabling falls back to the current flat loop. **Resolution: wire into existing tool-toggle infrastructure.**
5. **Plan naming fallback** — If the LLM omits `plan_name` from the planner JSON, fall back to `"Plan N"` (sequential within session). Sequential index stored in `task_tree.json` → `plans[].session_plan_index`. **Resolution: handle in plan creation logic.**
6. **Multiple plans per session** — A single conversation may trigger the planner multiple times. Each plan gets its own `plan.json`, its own context element, and its own output tab. Old plan tabs remain open (not closed). **Resolution: supported by the per-`plan_id` tab keying.**
7. **TBD resolution failure** — If the LLM cannot resolve a TBD step (e.g., prerequisites returned no useful data), the step status becomes `"failed"` and the plan reports it. **Resolution: add `tbd_resolution_failed` status; plan execution continues with remaining non-TBD steps.**
