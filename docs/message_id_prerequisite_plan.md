# Message ID Prerequisite Plan

Status: Approved — feature branch `feature/message-id` active
Date: 2026-04-25

## Purpose

Define exactly what must change to implement unique message IDs as a prerequisite for upcoming context-visualization and context-panel features.

## Assumptions

- No legacy sessions need to be preserved. All old test/session data will be removed before rollout.
- Backward compatibility for pre-existing session JSON is intentionally not supported.
- The feature should be implemented for all newly created and persisted messages.
- Message ID should be stable for the life of a message and never repurposed.

## Scope

In scope:

- Add a unique `message_id` to the core `Message` model.
- Persist and load `message_id` through all context serialization paths.
- Ensure every message creation path assigns an ID.
- Add lookup helpers to support clone/synthesis and context-panel actions.
- Add comprehensive tests for uniqueness, persistence, and behavior.

Out of scope:

- Migration tooling for old session files.
- Cross-session deduplication.
- Reworking task-node IDs (`plan_id`, `task_id`) outside message model integration.

## Classes And Functions To Update

## 1) Core model

File: `src/shared/models/message.py`

Required updates:

- `Message` dataclass:
  - Add `message_id: str` field.
  - Add `cloned_from: str | None = None` — set on replay-generated messages.
  - Add `superseded_by: str | None = None` — set on originals when a replay completes.
  - Add `synthesis_of: list[str] = field(default_factory=list)` — TOOL_RESULT IDs an ASSISTANT message synthesizes.
  - Ensure ID assignment in `__post_init__` when not explicitly provided.
- `Message.to_dict()`:
  - Include `message_id`, `cloned_from`, `superseded_by`, `synthesis_of` in persisted payload.
- `Message.from_dict()`:
  - Require `message_id` (no backward compatibility fallback).
  - Load `cloned_from`, `superseded_by`, `synthesis_of` with safe defaults (`None` / `[]`).
- `Message.save()`:
  - **Required:** change filename template from `{epoch}_{role}.json` to `{epoch}_{role}_{message_id}.json`.
  - This encodes the ID in the filename so messages are locatable by ID via directory glob without opening files (see "file_path ↔ message_id Mapping" section).
  - `_auto_save()` is unaffected — it reuses the already-set `self.file_path`.
- Factory helpers:
  - `user_message`, `assistant_message`, `system_message`, `thinking_message`, `tool_call_message`, `tool_result_message`.
  - No extra call-site parameters are required if `__post_init__` handles assignment.

Potential new helpers in `message.py`:

- `_new_message_id() -> str`
- `is_valid_message_id(message_id: str) -> bool`

## 2) Context persistence and lookup

File: `src/shared/models/context.py`

Required updates:

- `Context.add_message()`:
  - Validate `message.message_id` exists.
- `Context.to_dict()` and `Context.from_dict()`:
  - Ensure message IDs round-trip untouched.
- `Context.load_from_dir()`:
  - Validate every loaded message has a valid ID.
- Add helper methods to support upcoming panel operations:
  - `get_message_by_id(message_id: str) -> Message | None`
  - `require_message_by_id(message_id: str) -> Message`
  - `index_by_message_id() -> dict[str, Message]` (or cached equivalent)
  - `supersede_message(original_id: str, replacement_id: str) -> None` — sets `superseded_by` on the original and persists the change.
  - `get_ancestry(message_id: str) -> list[Message]` — follows `cloned_from` chain from the given message back to the root original; returns `[original, ..., given_message]` in chronological order.
- Enforce duplicate-ID detection at context load/add time.

## 3) Session and streaming message creation paths

File: `src/agentx/session.py`

Creation points to verify:

- `_build_shared_context()` WM system message creation.
- `process_prompt()` user message, PLAN message, TASK_NODE message creation.
- `handle_tool_call()` TOOL_CALL and TOOL_RESULT creation (legacy path).
- Any direct `Message(...)` instantiation.

File: `src/agentx/streaming_controller.py`

Creation points to verify:

- `_display_tool_call()` via `add_tool_call_message()`.
- `_display_tool_result()` via `add_tool_result_message()`.
- `_persist_stream_messages()` THINKING and ASSISTANT message creation.
- `_run_replay_subtask_worker()` — currently ephemeral (GUI-only); must be updated to:
  - Persist new TOOL_CALL/TOOL_RESULT/ASSISTANT messages with new `message_id`s and `cloned_from` set.
  - Track persisted TOOL_RESULT `message_id`s during execution and set them on synthesized ASSISTANT messages via `synthesis_of`.
  - Set `superseded_by` on original messages only after the replay group is fully persisted (atomic-ish: failure before completion leaves originals active).
  - Update context meter after supersession.

Goal:

- Every creation path must result in a valid `message_id`.

## 4) UI integration touchpoints (immediate prep)

Likely file: `src/agentx/gui/gui_manager.py` and related context panel code.

Required behavior prep:

- Context panel row identity should key off `message_id`, not list position.
- Planned fields `cloned_from` and `synthesis_of` must reference `message_id` only.

## Message ID Assignment Policies

### Policy 1 — Streamed Responses: One ID Per Completed Message

A streaming LLM response **accumulates all chunks before persistence** (see `StreamingController._persist_stream_messages()`). The result is two discrete persisted messages:

| Persisted message | Role | ID count |
|---|---|---|
| Accumulated thinking text | `THINKING` | 1 `message_id` |
| Accumulated response text | `ASSISTANT` | 1 `message_id` |

**Rule:** No per-chunk IDs are assigned. The ID is generated at the moment the completed message is persisted. This matches current implementation and keeps the message graph simple for typical conversational turns.

### Policy 2 — Composite Tasks: Discrete ID Per Unit of Work

A complex (agentic) task produces a structured tree of messages. **Each discrete unit of work gets its own `message_id`**, enabling independent enable/disable toggling in the context meter and context panel.

Message units in a composite task execution:

| Message role | Description | ID policy |
|---|---|---|
| `PLAN` | Top-level plan envelope | 1 `message_id` (plan root) |
| `TASK_NODE` | Each discrete task step | 1 `message_id` per node |
| `TOOL_CALL` | Each individual tool invocation | 1 `message_id` (distinct from `tool_id`) |
| `TOOL_RESULT` | Each tool result payload | 1 `message_id` (shares `tool_id` with its `TOOL_CALL`) |
| `THINKING` | Pre-synthesis reasoning text | 1 `message_id` |
| `ASSISTANT` (synthesis) | Final synthesized response | 1 `message_id` + `synthesis_of=[tool_result_message_ids]` |

**Rule:** Every persisted record regardless of role is an independently addressable message. Enabling/disabling a `PLAN` does **not** cascade to its children — each child is toggled independently. This allows the user to emphasize the synthesis (ASSISTANT) while suppressing intermediate tool results and task nodes under memory pressure.

#### Relationship Between `message_id` and `tool_id`

These are separate concerns:

- **`tool_id`**: LLM-assigned correlation token linking a `TOOL_CALL` to its `TOOL_RESULT`. Not unique across sessions.
- **`message_id`**: Session identity — globally unique, stable, assigned at creation, never reused.

A `TOOL_CALL` and its matching `TOOL_RESULT` share the same `tool_id` but have **different `message_id` values**.

#### Replay History Chain: Single Pointer, Not a List

A replayed message carries `cloned_from: str | None` — a single pointer to its **immediate predecessor**, not an accumulated list of all ancestors.

Why not a list:

- Growing an ancestry list means every replay must copy-and-extend the previous list into the new message. That is mutable data being propagated forward through new immutable objects — fragile and creates unnecessary coupling.
- A single pointer forms a **singly-linked list** (identical to the Git commit-parent model). Full ancestry is traversable by following the chain: `msg_C.cloned_from → msg_B.cloned_from → msg_A.cloned_from → None`.
- Traversal is a context-lookup operation, not a data-storage concern.

Example — three replays of the same tool call:

```
msg_A (original TOOL_CALL)
  ↓ cloned_from=None
msg_B (replay 1)   cloned_from=msg_A
  ↓
msg_C (replay 2)   cloned_from=msg_B
  ↓
msg_D (replay 3)   cloned_from=msg_C
```

`msg_D` does **not** carry a list `[msg_A, msg_B, msg_C]` — it only carries `cloned_from=msg_C`. The full chain is reconstructed by `Context.get_ancestry(message_id)` which follows pointers.

---

#### File Retention: All Messages Kept on Disk

The current system already writes every message to a dedicated JSON file under `sessions/<session_id>/context/`. This behavior is preserved and extended:

- Superseded messages are **never deleted**. Their files are updated in-place (`_auto_save()` rewrites to `self.file_path`) with `superseded_by` set and `enabled=False`.
- Replayed messages write new files alongside originals in the same directory.
- The directory is therefore a complete, auditable history of every execution including all retries.

A context directory after two replays of a tool call looks like:

```
sessions/xyz/context/
  1745600100_tool_call_msg_aaa.json    ← original    superseded_by=msg_bbb
  1745600101_tool_result_msg_bbb.json  ← original    superseded_by=msg_ccc  (example)
  1745600200_tool_call_msg_ddd.json    ← replay 1    cloned_from=msg_aaa
  1745600201_tool_result_msg_eee.json  ← replay 1    cloned_from=msg_bbb
  1745600300_tool_call_msg_fff.json    ← replay 2    cloned_from=msg_ddd
  1745600301_tool_result_msg_ggg.json  ← replay 2    cloned_from=msg_eee
```

---

#### file_path ↔ message_id Mapping: Decided — ID Encoded in Filename

**Decision:** The `message_id` is encoded in every message filename.

**New filename format:** `{epoch}_{role}_{message_id}.json`

Example: `1745600100_tool_call_msg_7d4a9833d5d34d83ae79f88f0f2444ce.json`

Rationale: keeping the ID only inside the JSON body would require opening every file at cold start to reconstruct the ID→path mapping — cost grows O(N) with session size and replay history. With the ID in the filename, any message is locatable by `glob("*_msg_<id>.json")` with zero file opens. The epoch prefix preserves temporal sort order so `load_from_dir()` is unchanged.

| Concern | Resolution |
|---|---|
| Temporal ordering in `load_from_dir()` | Unchanged — epoch prefix sorts correctly |
| Lookup by `message_id` from filesystem | `glob("*_msg_<id>.json")` — no file open required |
| Cold-start index rebuild cost | O(directory listing), not O(all file contents) |
| In-memory `index_by_message_id()` | Fast path; glob on filesystem as fallback |
| `_auto_save()` for in-place updates | Unchanged — reuses `self.file_path` set on first save |
| Human filesystem inspection | Role and epoch are immediately visible in the filename |
| Replay file identification | Original vs replay chains distinguishable via `cloned_from` in JSON body |

**`Message.save()` change:** `message_id` is generated in `__post_init__` before `save()` is called. Change the filename template from:

```python
filename = f"{self.epoch}_{self.role.value}.json"
```

to:

```python
filename = f"{self.epoch}_{self.role.value}_{self.message_id}.json"
```

No change to the assignment sequence is required. `_auto_save()` is unaffected — it rewrites to the already-set `self.file_path`.

**Required new Context helper:** `get_ancestry(message_id: str) -> list[Message]` — follows `cloned_from` chain from given message back to the root original, returning `[original, replay_1, ..., given_message]` in chronological order.

---

The composite task ID design directly supports selective context inclusion:

- **All** — include all messages in context window calculation (PLAN + TASK_NODEs + TOOL_CALLs + TOOL_RESULTs + THINKING + ASSISTANT)
- **Synthesis only** — disable all task/tool messages; include only ASSISTANT synthesis
- **Synthesis + selective tool data** — user selects individual TOOL_RESULT IDs to keep alongside synthesis
- **Custom** — any combination enabled/disabled per `message_id`

This is the primary motivation for the granular ID strategy: under memory pressure, a user can retain the synthesized answer while discarding scaffolding messages that were only needed during execution.

### Policy 3 — Replay: Immutable Originals, Supersede Model

#### Current Behavior (Pre-message-ID)

The replay GUI affordance (`on_replay` button on plan task nodes) currently calls `_replay_subtask` → `_run_replay_subtask_worker`. The worker re-executes the task node and updates the plan-tab GUI (status → "done", synthesis text), but **does not persist any new messages to the context**. Replay is currently ephemeral and GUI-only.

#### Problem With Ephemeral Replay

Once message IDs are in place, the context panel and context meter will render messages from the persisted context. A replay that only updates the plan-tab GUI creates a split-truth problem: the plan tab shows the new result but the context history (and LLM's next-turn view) still reflects the original execution.

#### Decision: Supersede Model

Messages are **immutable after creation** — replay never modifies or deletes an existing message. Instead:

1. Replay generates a fresh execution: new TOOL_CALL(s), new TOOL_RESULT(s), new ASSISTANT synthesis.
2. Each new message gets a new `message_id`.
3. Each new TOOL_CALL sets `cloned_from = original_tool_call.message_id`.
4. Every synthesized ASSISTANT message sets `synthesis_of = [tool_result_message_ids]` (replay and non-replay flows).
5. Each original message in the replayed group is marked `superseded_by = new_message_id`.
6. The context meter automatically disables superseded messages and enables the replacement set.
7. The user can manually re-enable original messages (e.g. to compare old vs new tool output).

#### Impact on Message Fields

New fields required on `Message` (extend current model):

| Field | Type | Purpose |
|---|---|---|
| `cloned_from` | `str \| None` | `message_id` of the source message this was replayed from |
| `superseded_by` | `str \| None` | `message_id` of the replacement message; set on the original when a replay occurs |
| `synthesis_of` | `list[str]` | `message_id`s of TOOL_RESULTs this ASSISTANT message synthesizes |

`superseded_by` is set **on the original** at the moment the replay completes successfully. This means `Message` instances are mutable at one controlled point — supersession — but `message_id` and `content` remain immutable.

#### Replay Lifecycle in Context Panel

```
Original group (disabled after replay):
  [TASK_NODE  msg_aaa]  superseded_by=msg_ddd  ← grayed out
  [TOOL_CALL  msg_bbb]  superseded_by=msg_eee  ← grayed out
  [TOOL_RESULT msg_ccc] superseded_by=msg_fff  ← grayed out
  [ASSISTANT  msg_xxx]  superseded_by=msg_ggg  ← grayed out

Replayed group (enabled by default):
  [TASK_NODE  msg_ddd]  cloned_from=msg_aaa    ← active
  [TOOL_CALL  msg_eee]  cloned_from=msg_bbb    ← active
  [TOOL_RESULT msg_fff] cloned_from=msg_ccc    ← active
  [ASSISTANT  msg_ggg]  synthesis_of=[msg_fff] ← active
```

#### What Happens If Replay Output Is Different?

The new TOOL_RESULT may have different content from the original — this is expected and the primary reason replay exists. Because the original is only disabled (not deleted), the user can:

- Enable the original result to compare old vs new output side by side in the context panel.
- Choose to include both in the LLM context (e.g. to let the LLM reconcile conflicting tool results).
- Keep only the new result for a clean "fresh run" context.

The context meter reflects whichever messages are currently enabled, so the token cost of including both generations is visible.

---

## How The ID Is Determined

Recommended scheme:

- Opaque ID with stable prefix plus UUID: `msg_<uuid4_hex>`.

Example:

- `msg_7d4a9833d5d34d83ae79f88f0f2444ce`

Why this scheme:

- High uniqueness without central coordination.
- Easy to validate with a simple regex.
- Prefix supports debugging/filtering in logs and JSON.

## Should ID Encode Metadata?

Recommendation: Do not encode metadata in the ID.

Do not use:

- `message_type + GUID`
- `message_type + epoch`
- `epoch-only`

Reasoning:

- Message role can change less predictably in future refactors, but identity must remain immutable.
- Epoch-based IDs are collision-prone under concurrency and complicate clock-related edge cases.
- Encoded semantics in IDs create unnecessary coupling and parsing logic.

Use explicit fields instead:

- `role` for message type.
- `epoch/timestamp` for time.
- `message_id` as pure identity.

## Thorough Testing Plan

## Test layers

Unit tests (primary):

- File: `tests/test_message_model_ids.py` (new)
- File: `tests/test_context_message_id_indexing.py` (new)

Integration tests:

- Extend `tests/test_session_stream_context_persistence.py`
- Extend `tests/test_context_coverage_uplift.py`

Functional tests:

- Add/extend a focused session-flow test where prompt processing produces multiple message roles and IDs persist across save/load.

## Required test cases

Model-level:

- GIVEN a new `Message` with no ID WHEN constructed THEN `message_id` is generated and valid.
- GIVEN a `Message` with explicit ID WHEN constructed THEN explicit ID is preserved.
- GIVEN two rapidly created messages WHEN IDs are generated THEN IDs are unique.
- GIVEN a message WHEN `to_dict()` is called THEN serialized payload includes `message_id`.
- GIVEN serialized message data WHEN `from_dict()` is called THEN `message_id` round-trips exactly.

Context-level:

- GIVEN messages with unique IDs WHEN added to context THEN `index_by_message_id()` returns all IDs.
- GIVEN duplicate message IDs WHEN added/loaded THEN context raises a deterministic validation error.
- GIVEN saved context WHEN reloaded from directory THEN IDs are unchanged.

Session/streaming paths:

- GIVEN `process_prompt()` flow WHEN thinking/assistant/tool messages are persisted THEN every stored message has valid unique ID.
- GIVEN tool call/result via streaming controller WHEN persisted THEN tool correlation (`tool_id`) and `message_id` coexist without conflict.

Streaming policy:

- GIVEN a streaming LLM response WHEN chunks are accumulated THEN exactly one `message_id` is assigned to the THINKING message and exactly one to the ASSISTANT message.
- GIVEN two concurrent streaming responses WHEN both complete THEN their respective THINKING and ASSISTANT messages each have distinct `message_id` values.
- GIVEN a completed stream WHEN the ASSISTANT message is persisted THEN no per-chunk IDs exist in the context.

Composite task policy:

- GIVEN a complex task execution WHEN PLAN, TASK_NODE, TOOL_CALL, TOOL_RESULT, and ASSISTANT messages are created THEN each has a distinct `message_id`.
- GIVEN a TOOL_CALL and its matching TOOL_RESULT WHEN both are persisted THEN `tool_id` is shared but `message_id` values are different.
- GIVEN a composite task context WHEN the PLAN message is disabled THEN TASK_NODE and TOOL_CALL children are not automatically disabled.
- GIVEN a composite task context WHEN TOOL_CALL and TOOL_RESULT messages are disabled and ASSISTANT synthesis is enabled THEN context meter reflects only the synthesis token count.

Replay policy:

- GIVEN an original TOOL_CALL/TOOL_RESULT/ASSISTANT group WHEN replay completes successfully THEN each original message has `superseded_by` set to the corresponding new `message_id`.
- GIVEN a replayed TOOL_CALL WHEN persisted THEN `cloned_from` equals the original TOOL_CALL `message_id`.
- GIVEN a replayed ASSISTANT synthesis WHEN persisted THEN `synthesis_of` contains the replayed TOOL_RESULT `message_id`(s).
- GIVEN a replay that produces different TOOL_RESULT content WHEN the replay completes THEN original messages are disabled (not deleted) and the new group is enabled.
- GIVEN a failed replay WHEN the worker raises an exception THEN no `superseded_by` fields are set on originals (originals remain active).
- GIVEN original and replayed groups both in context WHEN the user re-enables the original TOOL_RESULT THEN the context meter reflects the combined token cost of both enabled results.

Synthesis provenance policy:

- GIVEN a non-replay composite task that uses tool results WHEN final ASSISTANT synthesis is persisted THEN `synthesis_of` contains the contributing TOOL_RESULT `message_id`(s).
- GIVEN a synthesis message with no tool inputs WHEN persisted THEN `synthesis_of` is an empty list.

Replay ancestry chain:

- GIVEN three replays of the same tool call (msg_A → msg_B → msg_C → msg_D) WHEN `get_ancestry(msg_D.message_id)` is called THEN the result is `[msg_A, msg_B, msg_C, msg_D]` in order.
- GIVEN a message with no `cloned_from` WHEN `get_ancestry()` is called THEN the result is `[message]` (single-element list — it is the root).
- GIVEN a `cloned_from` value pointing to a missing message WHEN `get_ancestry()` is called THEN a partial chain is returned up to the missing link with a logged warning (does not raise).

Filesystem and filename policy:

- GIVEN a new message WHEN `save()` is called THEN the filename is `{epoch}_{role}_{message_id}.json`.
- GIVEN a saved message WHEN `_auto_save()` is called after `superseded_by` is set THEN the file is overwritten in-place at the same path.
- GIVEN a context directory WHEN `load_from_dir()` reloads THEN temporal order matches epoch-sorted filenames regardless of `message_id` suffix.
- GIVEN a `message_id` WHEN `glob("*_{message_id}.json")` is run on the context directory THEN exactly one file is returned without opening any files.

Failure-path tests:

- GIVEN malformed ID format WHEN loading THEN validation error is raised.
- GIVEN missing ID in JSON WHEN loading THEN validation error is raised (no compatibility fallback required).
- GIVEN old session files without `message_id` WHEN loading THEN validation error is raised and session load is aborted.

## Test quality requirements

- Include pytest markers (`unit`, `integration`, `functional`) as appropriate.
- Use parameterization for ID format variants and invalid cases.
- Include Gherkin-style docstrings per project convention.
- Ensure deterministic tests for explicit-ID paths; avoid flaky uniqueness assertions by checking set cardinality across a large sample.

## Other Concerns And Decisions

- Concurrency:
  - ID generation must be local and lock-free (UUID-based generation is suitable).
- Filename collisions:
  - Resolved — `message_id` is required in the filename: `{epoch}_{role}_{message_id}.json`. See "file_path ↔ message_id Mapping" section.
- Distinguish concerns:
  - `tool_id` is call-correlation identity, not message identity.
- Validation strictness:
  - Prefer strict validation now (early pre-1.x stage) to flush bad producers quickly.
- Logging:
  - Add `message_id` to debug logs for traceability in streaming and context-panel actions.

## Suggested Implementation Sequence

1. [/] Add `message_id` field and generation/validation in `Message`.
2. [/] Add strict serialization/deserialization support in `Message` and `Context`.
3. [/] Add context ID indexing and duplicate detection helpers.
4. [/] Verify all message creation paths in session/streaming use generated IDs.
5. [ ] Add unit tests first, then integration/functional tests.
6. [ ] Update filename strategy to `{epoch}_{role}_{message_id}.json` — required, not optional.
7. [ ] Remove any pre-message-id test/session artifacts before rollout to enforce strict no-compat assumptions.

## Done Criteria

- Every message persisted by the app has a valid `message_id`.
- IDs round-trip through all persistence and payload paths.
- No duplicate IDs can exist in a single context.
- Session and streaming tests confirm all runtime-created messages carry IDs.
- Clone/synthesis prerequisite is unblocked by stable ID support.
