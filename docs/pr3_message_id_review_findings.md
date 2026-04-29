# PR #3 — "Introduce Message Id" — Architecture Review Findings

**Reviewer:** Senior Application Architect  
**Branch:** `feature/message-id`  
**Date:** 2026-04-25  
**Status:** Requires changes before merge

---

## Executive Summary

The message-ID foundation introduced by this PR is architecturally sound and
largely well-implemented.  The core `Message` model changes, `Context` indexing
helpers, and the majority of the persistence path are correct.

**Three defects must be resolved before merging:**

| # | Severity | Area | Short description |
|---|---|---|---|
| [F-1](#f-1-replay-supersession-targets-context-wide-messages-by-positional-index) | **HIGH** | `streaming_controller.py` | Replay maps `cloned_from`/supersession by positional index across the whole context, not by task scope |
| [F-2](#f-2-session-wrapper-passes-refresh_gui-bool-into-synthesis_of-parameter) | **HIGH** | `session.py` + `streaming_controller.py` | Session delegate passes `refresh_gui` bool as positional arg into the `synthesis_of` parameter — corrupts the field |
| [F-3](#f-3-synthesis_of-corruption-survives-save--load-cycle) | **MEDIUM** | `message.py` | `synthesis_of` has no list-type validation on deserialization — a persisted boolean survives reload unchanged |
| [F-4](#f-4-missing-test-coverage-for-cross-turn-replay-isolation) | **MEDIUM** | test suite | Replay tests only cover single-group contexts; cross-turn contamination introduced by F-1 is undetected by current tests |

---

## F-1  Replay supersession targets context-wide messages by positional index

### Location

[src/agentx/streaming_controller.py](streaming_controller.py), lines 506–572 (`_run_replay_subtask_worker`)

### Root cause

Inside `_worker`, the lists of "original" messages to supersede are built by
scanning the **entire context** for every enabled message of each role:

```python
# streaming_controller.py — lines 506-520
original_tool_calls: list[Message] = [
    msg
    for msg in s.context.get_messages(enabled_only=False)
    if msg.role == MessageRole.TOOL_CALL and msg.enabled
]
original_tool_results: list[Message] = [
    msg
    for msg in s.context.get_messages(enabled_only=False)
    if msg.role == MessageRole.TOOL_RESULT and msg.enabled
]
original_assistants: list[Message] = [
    msg
    for msg in s.context.get_messages(enabled_only=False)
    if msg.role == MessageRole.ASSISTANT and msg.enabled
]
```

Each replay chunk then consumes these lists positionally via incrementing
counters (`original_tool_call_index`, `original_tool_result_index`).  Replay
chunk #0 maps to context-message #0, replay chunk #1 maps to context-message
# 1, and so on — regardless of which task node those context messages belong to.

### Scenario A — Two completed turns before replay

A session has two prior turns.  Turn 1 called `read_file` and `list_directory`.
Turn 2 called `search_files`.  The user then replays the task node from Turn 2.

**Context (before replay):**

```
[0]  TOOL_CALL   read_file        (Turn 1, enabled)
[1]  TOOL_RESULT read_file        (Turn 1, enabled)
[2]  TOOL_CALL   list_directory   (Turn 1, enabled)
[3]  TOOL_RESULT list_directory   (Turn 1, enabled)
[4]  ASSISTANT   "Listed files…"  (Turn 1, enabled)
[5]  TOOL_CALL   search_files     (Turn 2, enabled)   ← INTENDED replay target
[6]  TOOL_RESULT search_files     (Turn 2, enabled)   ← INTENDED replay target
[7]  ASSISTANT   "Found 3 files"  (Turn 2, enabled)   ← INTENDED replay target
```

**What currently happens:**

The replay produces two chunks: one `TOOL_CALL search_files` and one
`TOOL_RESULT search_files`.

- Replay `TOOL_CALL` chunk #0 → `original_tool_calls[0]` → **msg[0]** (`read_file`)  
  → `replay_new_call.cloned_from = msg[0].message_id`  
  → `msg[0].superseded_by = replay_new_call.message_id`  
  → msg[0] is **disabled** — Turn 1's `read_file` call is silently dropped.

- Replay `TOOL_RESULT` chunk #0 → `original_tool_results[0]` → **msg[1]** (`read_file`)  
  → same — Turn 1's `read_file` result is silently dropped.

- `TASK_NODE_END` → `original_assistants[-1]` → **msg[7]** (correct by accident
  because `[-1]` picks the last assistant).

**Expected behaviour:**

Only `msg[5]`, `msg[6]`, and `msg[7]` (Turn 2) should be superseded.
`msg[0]`–`msg[4]` (Turn 1) must be untouched.

### Scenario B — Two subtasks in the same plan, replay the first

A plan executes two steps.  Each step produced one `TOOL_CALL` + `TOOL_RESULT`
- `ASSISTANT` group.  The user replays Step 1.

**Context (before replay):**

```
[0]  TOOL_CALL   step_1_tool    (Step 1, enabled)   ← target
[1]  TOOL_RESULT step_1_tool    (Step 1, enabled)   ← target
[2]  ASSISTANT   "Step 1 done"  (Step 1, enabled)   ← target
[3]  TOOL_CALL   step_2_tool    (Step 2, enabled)   ← NOT a target
[4]  TOOL_RESULT step_2_tool    (Step 2, enabled)   ← NOT a target
[5]  ASSISTANT   "Step 2 done"  (Step 2, enabled)   ← NOT a target
```

Replay produces one `TOOL_CALL` and one `TOOL_RESULT` for Step 1.

- Replay call #0 → `original_tool_calls[0]` → **msg[0]** (correct).
- Replay result #0 → `original_tool_results[0]` → **msg[1]** (correct).
- `TASK_NODE_END` → `original_assistants[-1]` → **msg[5]** — Step 2's synthesis
  is superseded and disabled.  **LLM will never see Step 2's result again.**

### Required fix

Scope the "original" candidate lists to the task node being replayed.  The
`node` object passed into `_run_replay_subtask_worker` carries `task_id` (and
optionally `plan_id`).  All three lists should be filtered to messages whose
`task_id` field (or, where absent, whose `tool_id` overlap with the task's
previously recorded tool calls) matches the replay target.

Concretely, after loading the task-node record from `tree`, collect its known
`tool_id` values and filter accordingly:

```python
# Pseudo-code for the fix

replay_node_record = tree.nodes.get(_tid)
task_tool_ids: set[str] = set()
if replay_node_record:
    for tc_msg in s.context.get_messages(enabled_only=False):
        if (
            tc_msg.role == MessageRole.TOOL_CALL
            and tc_msg.task_id == _tid
            and tc_msg.enabled
        ):
            if tc_msg.tool_id:
                task_tool_ids.add(tc_msg.tool_id)

original_tool_calls = [
    msg for msg in s.context.get_messages(enabled_only=False)
    if msg.role == MessageRole.TOOL_CALL
    and msg.enabled
    and msg.task_id == _tid        # ← scoped to task
]
original_tool_results = [
    msg for msg in s.context.get_messages(enabled_only=False)
    if msg.role == MessageRole.TOOL_RESULT
    and msg.enabled
    and (msg.task_id == _tid or msg.tool_id in task_tool_ids)
]
original_assistants = [
    msg for msg in s.context.get_messages(enabled_only=False)
    if msg.role == MessageRole.ASSISTANT
    and msg.enabled
    and msg.task_id == _tid        # ← scoped to task
]
```

`task_id` must be stamped on `TOOL_CALL` and `TOOL_RESULT` messages when they
are first persisted via `_display_tool_call` / `_display_tool_result`.  The
`chunk.task_id` field is already carried on every `ResponseChunk` emitted by
the bridge; it just needs to be forwarded into `add_tool_call_message` /
`add_tool_result_message` and stored on the `Message`.

> **Note:** The `original_assistants[-1]` fallback for `cloned_from` on the
> replay synthesis also needs to be scoped to `task_id`.  Until `ASSISTANT`
> messages are stamped with `task_id` this can use `synthesis_of` reverse
> lookup: find the `ASSISTANT` whose `synthesis_of` intersects the set of
> `tool_result_message_id`s from the original group.

---

## F-2  Session wrapper passes `refresh_gui` bool into `synthesis_of` parameter

### Location

[src/agentx/session.py](session.py), lines 1046–1053 (`_persist_stream_messages` delegate)  
[src/agentx/streaming_controller.py](streaming_controller.py), lines 168–172 (new signature)

### Root cause

The `StreamingController._persist_stream_messages` signature was extended in
this PR to accept `synthesis_of`:

```python
# streaming_controller.py — new signature (lines 168-173)
def _persist_stream_messages(
    self,
    thinking_text: str,
    content_text: str,
    synthesis_of: list[str] | None = None,   # ← NEW parameter added here
    refresh_gui: bool = True,
) -> None:
```

The delegate in `session.py` was **not updated** to match:

```python
# session.py — unchanged delegate (lines 1046-1053)
def _persist_stream_messages(
    self,
    thinking_text: str,
    content_text: str,
    refresh_gui: bool = True,      # ← no synthesis_of param added
) -> None:
    """Delegate to StreamingController."""
    self._streaming_controller._persist_stream_messages(thinking_text, content_text, refresh_gui)
    #                                                                               ^^^^^^^^^^^
    #                                  refresh_gui (bool) lands in synthesis_of position
```

The positional argument `refresh_gui` (a `bool`) is passed as the third
positional argument to the controller, which the controller receives as
`synthesis_of`.

### Scenario C — Normal streaming response with tool calls

User submits a prompt.  The streaming loop accumulates two tool result IDs into
`stream_tool_result_ids = ["msg_aaa...", "msg_bbb..."]`, then calls:

```python
# streaming_controller.py line 436 — CORRECT path (not affected)
self._persist_stream_messages(joined_thinking, joined_content, synthesis_of=stream_tool_result_ids)
```

This path passes `synthesis_of` **by keyword** and is **unaffected** — the
controller receives the correct list.

### Scenario D — `process_prompt()` code path (test-friendly API)

`session.process_prompt()` (line 286) calls the session **wrapper**, not the
controller directly:

```python
# session.py line 286
self._persist_stream_messages(
    "".join(thinking_parts),
    "".join(content_parts),
    refresh_gui=False,          # ← goes into session wrapper's refresh_gui
)
```

The session wrapper then calls the controller positionally:

```python
self._streaming_controller._persist_stream_messages(thinking_text, content_text, refresh_gui)
# → controller receives: thinking_text=..., content_text=..., synthesis_of=False
```

Inside the controller:

```python
assistant_message.synthesis_of = synthesis_of or []
# synthesis_of is False (a falsy bool)
# False or [] → []
# synthesis_of stored as [] — tool result provenance silently dropped
```

**Effect:** Any session using the `process_prompt()` API (including integration
tests) will always persist `synthesis_of = []` even when tool results were
produced.  Tool provenance is silently lost.

### Scenario E — Default path via `_display_tool_call` delegation in session

Any call arriving via the `session._persist_stream_messages` with the default
`refresh_gui=True`:

```python
self._streaming_controller._persist_stream_messages(thinking_text, content_text, True)
# → controller receives: synthesis_of=True
```

Inside the controller:

```python
assistant_message.synthesis_of = synthesis_of or []
# synthesis_of is True (a truthy bool)
# True or [] → True
# synthesis_of stored as True — type violation
```

When this message is serialized:

```json
{
  "role": "assistant",
  "content": "...",
  "synthesis_of": true      ← boolean, not list; violates schema
}
```

On reload via `Message.from_dict`, the line:

```python
synthesis_of=data.get("synthesis_of") or [],
```

evaluates as `True or []` → `True` — the boolean is silently reloaded as `True`
without raising.  The corruption survives the full save/load cycle (see F-3).

### Required fix

Update the session wrapper's delegate signature and body to match the
controller, forwarding `synthesis_of`:

```python
# session.py — corrected delegate
def _persist_stream_messages(
    self,
    thinking_text: str,
    content_text: str,
    synthesis_of: list[str] | None = None,
    refresh_gui: bool = True,
) -> None:
    """Delegate to StreamingController."""
    self._streaming_controller._persist_stream_messages(
        thinking_text, content_text, synthesis_of=synthesis_of, refresh_gui=refresh_gui
    )
```

All arguments should be passed by keyword to the controller to prevent future
positional-drift bugs.

---

## F-3  `synthesis_of` corruption survives save / load cycle

### Location

[src/shared/models/message.py](message.py) — `from_dict` classmethod  
[src/shared/models/message.py](message.py) — `Message` dataclass field declaration

### Root cause

`synthesis_of` is declared as `list[str]` but there is no runtime type
assertion on deserialization.  `from_dict` uses:

```python
synthesis_of=data.get("synthesis_of") or [],
```

If the stored value is a non-list truthy value (e.g. `True` from F-2, or a
stray `str` from a hand-edited file), `or []` is bypassed because the value is
truthy, and the field is silently set to the non-list value.

### Scenario F — Corrupted message loaded and sent to LLM

1. F-2 causes `synthesis_of = True` to be persisted to disk.
2. Session is reloaded.  `Message.from_dict` runs `True or []` → stores `True`.
3. GUI shows the assistant message normally (field not rendered).
4. User submits next prompt.  `to_llm_dict()` is called.
5. `to_dict()` serializes `"synthesis_of": true` again — still corrupted.
6. `context.to_llm_messages()` works (does not touch `synthesis_of`).

The immediate damage is silent data corruption in the persisted JSON and in the
in-memory `synthesis_of` attribute.  No exception is raised anywhere in the
current stack.  A future context-panel feature that iterates `synthesis_of` to
display provenance links will receive a `bool` instead of a list and crash or
produce wrong output.

### Required fix

Add a type-coercion guard in `from_dict` and a validation guard in
`__post_init__`:

```python
# message.py — from_dict, synthesis_of line
raw_synthesis_of = data.get("synthesis_of")
synthesis_of = raw_synthesis_of if isinstance(raw_synthesis_of, list) else []

# message.py — __post_init__, after message_id validation
if not isinstance(self.synthesis_of, list):
    raise ValueError(
        f"synthesis_of must be a list[str], got {type(self.synthesis_of).__name__!r}"
    )
```

---

## F-4  Missing test coverage for cross-turn replay isolation

### Location

[tests/test_replay_message_lineage_e2e.py](../tests/test_replay_message_lineage_e2e.py)

### Gap

Every existing replay test constructs a context that contains **only** the
messages from the single replay target group.  The positional-index bug
described in F-1 is undetectable in these tests because there are no other
turns in the fixture context to be incorrectly targeted.

```python
# test_replay_message_lineage_e2e.py — line 86
def _make_original_group(tool_pair_count: int) -> tuple[Context, dict[str, Message]]:
    ctx = Context()
    # Only the replay-target group is ever added; no prior turns exist
    for idx in range(tool_pair_count):
        call_msg = Message(role=MessageRole.TOOL_CALL, ...)
        result_msg = Message(role=MessageRole.TOOL_RESULT, ...)
        ...
    original_assistant = Message(role=MessageRole.ASSISTANT, ...)
    ...
```

The `interleaved_replays_different_targets` test in the same file manipulates
`supersede_message` directly; it does not exercise `_run_replay_subtask_worker`
with a mixed-history context.

### Required new test cases

#### Test T-1: Replay does not supersede messages from a prior turn

**GIVEN** a context that contains one prior completed turn with its own
`TOOL_CALL` and `TOOL_RESULT`, followed by a second task-node group that is the
intended replay target.

**WHEN** `_run_replay_subtask_worker` is invoked for the second task node.

**THEN** the prior turn's `TOOL_CALL` and `TOOL_RESULT` have `superseded_by =
None` and `enabled = True`.

**AND** only the intended target messages have `superseded_by` set.

```gherkin
GIVEN context = [Turn1/TC, Turn1/TR, Turn1/ASST, Turn2/TC, Turn2/TR, Turn2/ASST]
  AND replay target = Turn2 task_id
WHEN _run_replay_subtask_worker executes
THEN Turn1/TC.superseded_by is None
 AND Turn1/TR.superseded_by is None
 AND Turn1/ASST.superseded_by is None
 AND Turn2/TC.superseded_by == new_TC.message_id
 AND Turn2/TR.superseded_by == new_TR.message_id
 AND Turn2/ASST.superseded_by == new_ASST.message_id
```

#### Test T-2: Replay does not supersede messages from a concurrent plan step

**GIVEN** a context that contains two plan steps (Step A and Step B), both
completed, where Step A is replayed.

**WHEN** `_run_replay_subtask_worker` is invoked for Step A's task node.

**THEN** Step B's `TOOL_CALL`, `TOOL_RESULT`, and `ASSISTANT` remain enabled
and have `superseded_by = None`.

```gherkin
GIVEN context = [StepA/TC, StepA/TR, StepA/ASST, StepB/TC, StepB/TR, StepB/ASST]
  AND replay target = StepA task_id
WHEN _run_replay_subtask_worker executes
THEN StepB/TC.superseded_by is None
 AND StepB/TR.superseded_by is None
 AND StepB/ASST.superseded_by is None
```

#### Test T-3: `synthesis_of` is a valid list after `_persist_stream_messages`

**GIVEN** `_persist_stream_messages` is called with `synthesis_of=["msg_aaa..."]`.

**WHEN** the controller persists the assistant message.

**THEN** `assistant_message.synthesis_of == ["msg_aaa..."]`.

**AND** `isinstance(assistant_message.synthesis_of, list) is True`.

```gherkin
GIVEN synthesis_of = ["msg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]
WHEN _persist_stream_messages(thinking_text="", content_text="answer", synthesis_of=synthesis_of)
THEN stored assistant message has synthesis_of == ["msg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]
 AND isinstance(synthesis_of, list) is True
```

#### Test T-4: Session delegate preserves `synthesis_of` through the delegation chain

**GIVEN** `AgentXSession._persist_stream_messages` is called with an explicit
`synthesis_of` list.

**WHEN** the call is delegated to `StreamingController._persist_stream_messages`.

**THEN** the controller receives the same list (not a boolean).

```gherkin
GIVEN session._persist_stream_messages called with synthesis_of=["msg_abc..."], refresh_gui=False
WHEN delegation passes arguments to controller
THEN controller receives synthesis_of=["msg_abc..."]
 AND controller receives refresh_gui=False
 AND stored message.synthesis_of == ["msg_abc..."]
```

#### Test T-5: `synthesis_of` boolean corruption is rejected at deserialization

**GIVEN** a JSON message file where `synthesis_of` has value `true` (a boolean).

**WHEN** `Message.from_dict` is called with that payload.

**THEN** `message.synthesis_of == []` (corrected to empty list, not storing `True`).

**OR** a `ValueError` is raised if strict validation is preferred.

```gherkin
GIVEN payload = {"role": "assistant", "content": "...", "message_id": "msg_abc...",
                 "synthesis_of": true}
WHEN Message.from_dict(payload)
THEN resulting message.synthesis_of == []   # coerced to empty list
 AND isinstance(message.synthesis_of, list) is True
```

---

## Secondary observation: `from_dict` / `load_from_dir` bypass `add_message` guards

### Location

[src/shared/models/context.py](context.py), `from_dict` lines 179–183 and
`load_from_dir` lines 205–216.

### Description

Both methods append `MessageEntry` objects **directly** to `self.messages`,
bypassing `add_message()`.  Each independently re-implements the duplicate-ID
check that lives in `add_message`.

This is not a current defect — both code paths guard correctly today.  The risk
is a maintenance trap: any future validation added to `add_message` (e.g. a
maximum-context-size guard, a schema version check) will silently not apply
to the load paths.

### Recommendation

Extract a private `_append_loaded_message(msg: Message) -> None` that handles
duplicate detection and direct-append, leaving `add_message` as the creation
path (with save-to-disk semantics).  Both `from_dict` and `load_from_dir` call
the private method.  This makes the distinction explicit in the code.

---

## Summary: Files to change

| File | Change required |
|---|---|
| `src/agentx/streaming_controller.py` | F-1: scope `original_tool_calls`, `original_tool_results`, `original_assistants` to `_tid`; F-1: stamp `task_id` on `TOOL_CALL`/`TOOL_RESULT` messages in `_display_tool_call` / `_display_tool_result` |
| `src/agentx/session.py` | F-2: add `synthesis_of` param to delegate `_persist_stream_messages`; pass all args by keyword |
| `src/shared/models/message.py` | F-3: add type-coercion guard in `from_dict`; add `isinstance` assertion in `__post_init__` |
| `tests/test_replay_message_lineage_e2e.py` | F-4: add T-1, T-2 with multi-turn fixture context |
| `tests/test_streaming_message_id_integration.py` | F-4: add T-3, T-4 |
| `tests/test_message_model_ids.py` | F-4: add T-5 |

---

## Appendix: Code references

| Finding | File | Line(s) | Excerpt |
|---|---|---|---|
| F-1 list construction | `streaming_controller.py` | 506–520 | `original_tool_calls: list[Message] = [...]` |
| F-1 positional indexing | `streaming_controller.py` | 529–531 | `cloned_from = original_tool_calls[original_tool_call_index]` |
| F-1 assistant scope | `streaming_controller.py` | 563 | `replay_synthesis.cloned_from = original_assistants[-1].message_id` |
| F-2 old session signature | `session.py` | 1046–1053 | `def _persist_stream_messages(self, thinking_text, content_text, refresh_gui=True)` |
| F-2 new controller signature | `streaming_controller.py` | 168–173 | `def _persist_stream_messages(self, thinking_text, content_text, synthesis_of=None, refresh_gui=True)` |
| F-2 positional pass-through | `session.py` | 1053 | `self._streaming_controller._persist_stream_messages(thinking_text, content_text, refresh_gui)` |
| F-3 unsafe coercion | `message.py` | 288 | `synthesis_of=data.get("synthesis_of") or [],` |
| F-3 no `__post_init__` guard | `message.py` | 148–155 | `__post_init__` validates `role` and `message_id` but not `synthesis_of` type |
| F-4 single-group fixture | `test_replay_message_lineage_e2e.py` | 68–85 | `_make_original_group` never adds prior-turn messages |
