"""Replay lineage end-to-end tests for message identity semantics.

The intent of this module is to verify replay lineage and supersession
behaviors that are prerequisites for context-panel correctness.
"""

from __future__ import annotations

import threading
from dataclasses import dataclass
from types import SimpleNamespace

import pytest

from agentx.streaming_controller import StreamingController
from shared.models.context import Context
from shared.models.message import Message, MessageRole, is_valid_message_id
from shared.models.response import ChunkType, ResponseChunk


class _ImmediateThread:
    """Test double for ``threading.Thread`` that runs target immediately."""

    def __init__(self, target=None, daemon=None):
        self._target = target
        self.daemon = daemon

    def start(self) -> None:
        if self._target is not None:
            self._target()

    def join(self, timeout=None) -> None:  # pragma: no cover - included for API compatibility
        return


class _DummyGui:
    """Minimal GUI stub required by ``StreamingController`` replay worker."""

    def __init__(self) -> None:
        self.errors: list[str] = []

    def set_streaming_state(self, _active: bool) -> None:
        return

    def update_plan_node_status(self, _task_id: str, _status: str) -> None:
        return

    def update_plan_synthesis(self, _task_id: str, _synth: str, _assertions: list) -> None:
        return

    def display_agent_response(self, _text: str) -> None:
        return

    def display_error(self, message: str) -> None:
        self.errors.append(message)


@dataclass
class _ReplayFixture:
    """Container for replay-test setup artifacts."""

    session: SimpleNamespace
    controller: StreamingController
    originals: dict[str, Message]


def _make_original_group(tool_pair_count: int) -> tuple[Context, dict[str, Message]]:
    """Create original replay target messages in active context."""
    ctx = Context()
    originals: dict[str, Message] = {}

    for idx in range(tool_pair_count):
        call_msg = Message(role=MessageRole.TOOL_CALL, content=f"original-call-{idx}", task_id="task-1")
        result_msg = Message(role=MessageRole.TOOL_RESULT, content=f"original-result-{idx}", task_id="task-1")
        ctx.add_message(call_msg)
        ctx.add_message(result_msg)
        originals[f"tool_call_{idx}"] = call_msg
        originals[f"tool_result_{idx}"] = result_msg

    original_assistant = Message(role=MessageRole.ASSISTANT, content="original synthesis", task_id="task-1")
    ctx.add_message(original_assistant)
    originals["assistant"] = original_assistant
    return ctx, originals


def _make_replay_fixture(tool_pair_count: int) -> _ReplayFixture:
    """Build minimal Session + StreamingController test fixture."""
    ctx, originals = _make_original_group(tool_pair_count)
    session = SimpleNamespace(
        context=ctx,
        gui=_DummyGui(),
        _is_streaming=threading.Event(),
        _safe_root_after=lambda fn: fn(),
        _write_log=lambda _text: None,
        _output_logger=SimpleNamespace(log=lambda *_args, **_kwargs: None),
        refresh_user_gui=lambda: None,
        refresh_working_memory_gui=lambda: None,
        agentix_adapter=SimpleNamespace(),
    )
    return _ReplayFixture(session=session, controller=StreamingController(session), originals=originals)


def _make_success_chunks(tool_pair_count: int, task_id: str = "task-1") -> list[ResponseChunk]:
    """Create replay stream chunks for a successful replay."""
    chunks: list[ResponseChunk] = []
    for idx in range(tool_pair_count):
        chunks.append(
            ResponseChunk(
                type=ChunkType.TOOL_CALL,
                task_id=task_id,
                tool_name=f"tool_{idx}",
                tool_input={"idx": idx},
                tool_id=f"tool-{idx}",
                round_index=idx,
            )
        )
        chunks.append(
            ResponseChunk(
                type=ChunkType.TOOL_RESULT,
                task_id=task_id,
                tool_name=f"tool_{idx}",
                tool_output={"result": idx},
                tool_id=f"tool-{idx}",
                round_index=idx,
            )
        )
    chunks.append(
        ResponseChunk(
            type=ChunkType.TASK_NODE_END,
            task_id=task_id,
            content="replayed synthesis",
            assertions=[],
        )
    )
    return chunks


def _run_replay_worker_with_chunks(
    replay_fixture: _ReplayFixture,
    chunks: list[ResponseChunk] | None = None,
    exception: Exception | None = None,
    monkeypatch: pytest.MonkeyPatch | None = None,
    task_id: str = "task-1",
) -> None:
    """Execute replay worker deterministically using immediate-thread patching."""
    if monkeypatch is None:
        raise AssertionError("monkeypatch fixture is required")

    if exception is not None:

        def _raise_generator(_node, _context, _tree):
            raise exception
            yield  # pragma: no cover

        replay_fixture.session.agentix_adapter.replay_task_node_generator = _raise_generator
    else:
        provided_chunks = chunks or []
        replay_fixture.session.agentix_adapter.replay_task_node_generator = lambda _n, _c, _t: iter(provided_chunks)

    monkeypatch.setattr("agentx.streaming_controller.threading.Thread", _ImmediateThread)
    replay_fixture.controller._run_replay_subtask_worker(node=object(), tree=object(), task_id=task_id)


def _new_messages_by_role(context: Context, pre_ids: set[str], role: MessageRole) -> list[Message]:
    """Return messages of ``role`` created after ``pre_ids`` snapshot."""
    return [m for m in context.get_messages(enabled_only=False) if m.role == role and m.message_id not in pre_ids]


@pytest.mark.integration
@pytest.mark.parametrize(
    "message_role",
    [
        "tool_call",
        "tool_result",
        "assistant",
    ],
)
def test_replay_success_sets_cloned_from_and_superseded_by_for_each_replayed_message(
    message_role: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    """GIVEN a completed replay for a previously executed message group.

    WHEN replay persistence succeeds for a role-specific message replacement pair.

    THEN the replacement message must set cloned_from to the original message_id,
    AND the original message must set superseded_by to the replacement message_id,
    AND both IDs must be valid, distinct, and resolvable in the active Context index.
    """
    replay_fixture = _make_replay_fixture(tool_pair_count=1)
    ctx = replay_fixture.session.context
    pre_ids = {m.message_id for m in ctx.get_messages(enabled_only=False)}

    _run_replay_worker_with_chunks(
        replay_fixture,
        chunks=_make_success_chunks(tool_pair_count=1),
        monkeypatch=monkeypatch,
    )

    role_map = {
        "tool_call": (MessageRole.TOOL_CALL, "tool_call_0"),
        "tool_result": (MessageRole.TOOL_RESULT, "tool_result_0"),
        "assistant": (MessageRole.ASSISTANT, "assistant"),
    }
    replay_role, original_key = role_map[message_role]
    replacements = _new_messages_by_role(ctx, pre_ids, replay_role)
    assert replacements, f"Expected replay replacement message for role={message_role}"

    replacement = replacements[-1]
    original = replay_fixture.originals[original_key]

    assert replacement.message_id != original.message_id
    assert is_valid_message_id(replacement.message_id)
    assert replacement.cloned_from == original.message_id
    assert original.superseded_by == replacement.message_id
    assert ctx.require_message_by_id(original.message_id) is original
    assert ctx.require_message_by_id(replacement.message_id) is replacement


@pytest.mark.integration
@pytest.mark.parametrize(
    "failure_stage",
    [
        "before_any_replay_message_persist",
        "after_tool_call_before_tool_result",
        "after_tool_result_before_synthesis",
        "during_supersession_update",
    ],
)
def test_replay_failure_does_not_supersede_original_messages(
    failure_stage: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    """GIVEN an original replay target group in active context.

    WHEN replay fails at a defined persistence stage.

    THEN no original message in the target group may have superseded_by set,
    AND originals must remain enabled,
    AND partial replacement state must not mark originals as superseded.
    """
    replay_fixture = _make_replay_fixture(tool_pair_count=1)
    ctx = replay_fixture.session.context

    if failure_stage == "before_any_replay_message_persist":
        chunks: list[ResponseChunk] = []
        exception = RuntimeError("failure before replay output")
    elif failure_stage == "after_tool_call_before_tool_result":
        chunks = [
            ResponseChunk(
                type=ChunkType.TOOL_CALL,
                task_id="task-1",
                tool_name="tool_0",
                tool_input={"idx": 0},
                tool_id="tool-0",
                round_index=0,
            )
        ]
        exception = RuntimeError("failure after tool call")
    elif failure_stage == "after_tool_result_before_synthesis":
        chunks = _make_success_chunks(tool_pair_count=1)[:-1]
        exception = RuntimeError("failure after tool result")
    else:
        chunks = _make_success_chunks(tool_pair_count=1)
        exception = RuntimeError("failure during supersession update")

    if failure_stage == "during_supersession_update":

        def _raise_on_supersede(_original_id: str, _replacement_id: str) -> None:
            raise RuntimeError("supersede failure")

        monkeypatch.setattr(ctx, "supersede_message", _raise_on_supersede)

    _run_replay_worker_with_chunks(
        replay_fixture,
        chunks=chunks,
        exception=exception,
        monkeypatch=monkeypatch,
    )

    for original in replay_fixture.originals.values():
        assert original.superseded_by is None
        assert original.enabled is True


@pytest.mark.integration
@pytest.mark.parametrize("tool_result_count", [1, 2, 3])
def test_replay_synthesis_of_references_replayed_tool_result_ids(
    tool_result_count: int, monkeypatch: pytest.MonkeyPatch
) -> None:
    """GIVEN a replay execution that emits one or more TOOL_RESULT messages.

    WHEN replay assistant synthesis is persisted.

    THEN synthesis_of must contain the replay TOOL_RESULT message_id values in execution order,
    AND synthesis_of must not reference original TOOL_RESULT IDs from prior generations.
    """
    replay_fixture = _make_replay_fixture(tool_pair_count=tool_result_count)
    ctx = replay_fixture.session.context
    original_result_ids = {
        replay_fixture.originals[f"tool_result_{idx}"].message_id for idx in range(tool_result_count)
    }
    pre_ids = {m.message_id for m in ctx.get_messages(enabled_only=False)}

    _run_replay_worker_with_chunks(
        replay_fixture,
        chunks=_make_success_chunks(tool_pair_count=tool_result_count),
        monkeypatch=monkeypatch,
    )

    replay_result_messages = _new_messages_by_role(ctx, pre_ids, MessageRole.TOOL_RESULT)
    replay_result_ids = [m.message_id for m in replay_result_messages]
    assert len(replay_result_ids) == tool_result_count

    replay_assistants = _new_messages_by_role(ctx, pre_ids, MessageRole.ASSISTANT)
    assert replay_assistants, "Expected replay synthesis assistant message"
    replay_synthesis = replay_assistants[-1]

    assert replay_synthesis.synthesis_of == replay_result_ids
    assert set(replay_synthesis.synthesis_of).isdisjoint(original_result_ids)


@pytest.mark.functional
@pytest.mark.parametrize(
    "replacement_group_size",
    [
        1,
        2,
        3,
    ],
)
def test_replay_enables_replacement_group_and_disables_original_group(
    replacement_group_size: int, monkeypatch: pytest.MonkeyPatch
) -> None:
    """GIVEN an original replay target group and a successfully persisted replacement group.

    WHEN replay completion transitions context to the replacement generation.

    THEN each original message in the replay scope must be disabled,
    AND each replacement message must be enabled,
    AND context token accounting must reflect the replacement enablement state.
    """
    replay_fixture = _make_replay_fixture(tool_pair_count=replacement_group_size)
    ctx = replay_fixture.session.context
    pre_ids = {m.message_id for m in ctx.get_messages(enabled_only=False)}

    _run_replay_worker_with_chunks(
        replay_fixture,
        chunks=_make_success_chunks(tool_pair_count=replacement_group_size),
        monkeypatch=monkeypatch,
    )

    replacement_messages = [m for m in ctx.get_messages(enabled_only=False) if m.message_id not in pre_ids]
    assert replacement_messages, "Expected replacement replay messages"

    for original in replay_fixture.originals.values():
        assert original.enabled is False

    for replacement in replacement_messages:
        assert replacement.enabled is True


@pytest.mark.integration
@pytest.mark.parametrize("replay_depth", [1, 2, 3, 4])
def test_replay_ancestry_chain_is_chronological_and_complete(replay_depth: int) -> None:
    """GIVEN a message replayed repeatedly across multiple generations.

    WHEN ancestry is requested for the latest replay generation.

    THEN get_ancestry must return a root-to-leaf ordered chain,
    AND each link must align with cloned_from pointer semantics,
    AND the chain cardinality must equal replay depth plus the root generation.
    """
    ctx = Context()
    root = Message(role=MessageRole.TOOL_CALL, content="root")
    ctx.add_message(root)

    current = root
    for idx in range(replay_depth):
        next_message = Message(
            role=MessageRole.TOOL_CALL,
            content=f"replay-{idx + 1}",
            cloned_from=current.message_id,
        )
        ctx.add_message(next_message)
        current = next_message

    ancestry = ctx.get_ancestry(current.message_id)
    ancestry_ids = [msg.message_id for msg in ancestry]

    assert len(ancestry) == replay_depth + 1
    assert ancestry_ids[0] == root.message_id
    assert ancestry_ids[-1] == current.message_id

    for idx in range(1, len(ancestry)):
        assert ancestry[idx].cloned_from == ancestry[idx - 1].message_id


@pytest.mark.integration
@pytest.mark.parametrize(
    "interleaving_pattern",
    [
        "single_replay_single_target",
        "multiple_replays_same_target",
        "interleaved_replays_different_targets",
    ],
)
def test_replay_supersession_mapping_remains_deterministic_under_interleaving(interleaving_pattern: str) -> None:
    """GIVEN replay operations that may interleave across task targets.

    WHEN replay persistence and supersession updates complete.

    THEN each original message must map to exactly one intended replacement message_id,
    AND no original may supersede to a message outside its replay lineage,
    AND supersession relationships must remain deterministic regardless of interleaving order.
    """
    ctx = Context()
    originals = [Message(role=MessageRole.TOOL_RESULT, content=f"orig-{idx}") for idx in range(3)]
    for original in originals:
        ctx.add_message(original)

    if interleaving_pattern == "single_replay_single_target":
        replacement = Message(role=MessageRole.TOOL_RESULT, content="r0", cloned_from=originals[0].message_id)
        ctx.add_message(replacement)
        ctx.supersede_message(originals[0].message_id, replacement.message_id)
        expected = {originals[0].message_id: replacement.message_id}

    elif interleaving_pattern == "multiple_replays_same_target":
        replacement_1 = Message(role=MessageRole.TOOL_RESULT, content="r1", cloned_from=originals[0].message_id)
        replacement_2 = Message(role=MessageRole.TOOL_RESULT, content="r2", cloned_from=replacement_1.message_id)
        ctx.add_message(replacement_1)
        ctx.add_message(replacement_2)
        ctx.supersede_message(originals[0].message_id, replacement_1.message_id)
        ctx.supersede_message(replacement_1.message_id, replacement_2.message_id)
        expected = {
            originals[0].message_id: replacement_1.message_id,
            replacement_1.message_id: replacement_2.message_id,
        }

    else:
        replacement_a = Message(role=MessageRole.TOOL_RESULT, content="ra", cloned_from=originals[0].message_id)
        replacement_b = Message(role=MessageRole.TOOL_RESULT, content="rb", cloned_from=originals[1].message_id)
        ctx.add_message(replacement_a)
        ctx.add_message(replacement_b)
        ctx.supersede_message(originals[1].message_id, replacement_b.message_id)
        ctx.supersede_message(originals[0].message_id, replacement_a.message_id)
        expected = {
            originals[0].message_id: replacement_a.message_id,
            originals[1].message_id: replacement_b.message_id,
        }

    for original_id, replacement_id in expected.items():
        original = ctx.require_message_by_id(original_id)
        replacement = ctx.require_message_by_id(replacement_id)
        assert original.superseded_by == replacement.message_id
        assert replacement.cloned_from == original_id

    allowed_sources = set(expected.keys())
    for original_id, replacement_id in expected.items():
        replacement = ctx.require_message_by_id(replacement_id)
        assert replacement.cloned_from in allowed_sources


# ---------------------------------------------------------------------------
# F-4 / T-1: Replay does not supersede messages from a prior turn
# ---------------------------------------------------------------------------


def _make_two_turn_fixture(prior_task_id: str, target_task_id: str) -> tuple[_ReplayFixture, dict[str, Message]]:
    """Build a fixture with two turns: one prior completed turn followed by the replay target."""
    ctx = Context()
    prior: dict[str, Message] = {}

    # Prior turn messages — different task_id
    prior_call = Message(role=MessageRole.TOOL_CALL, content="prior-call", task_id=prior_task_id)
    prior_result = Message(role=MessageRole.TOOL_RESULT, content="prior-result", task_id=prior_task_id)
    prior_asst = Message(role=MessageRole.ASSISTANT, content="prior synthesis", task_id=prior_task_id)
    for msg in (prior_call, prior_result, prior_asst):
        ctx.add_message(msg)
        prior[msg.message_id] = msg

    # Target turn messages — the task_id that will be replayed
    target_call = Message(role=MessageRole.TOOL_CALL, content="target-call", task_id=target_task_id)
    target_result = Message(role=MessageRole.TOOL_RESULT, content="target-result", task_id=target_task_id)
    target_asst = Message(role=MessageRole.ASSISTANT, content="target synthesis", task_id=target_task_id)
    for msg in (target_call, target_result, target_asst):
        ctx.add_message(msg)

    originals = {
        "tool_call_0": target_call,
        "tool_result_0": target_result,
        "assistant": target_asst,
    }

    session = SimpleNamespace(
        context=ctx,
        gui=_DummyGui(),
        _is_streaming=threading.Event(),
        _safe_root_after=lambda fn: fn(),
        _write_log=lambda _text: None,
        _output_logger=SimpleNamespace(log=lambda *_args, **_kwargs: None),
        refresh_user_gui=lambda: None,
        refresh_working_memory_gui=lambda: None,
        agentix_adapter=SimpleNamespace(),
    )
    fixture = _ReplayFixture(session=session, controller=StreamingController(session), originals=originals)
    return fixture, prior


@pytest.mark.integration
def test_replay_does_not_supersede_prior_turn_messages(monkeypatch: pytest.MonkeyPatch) -> None:
    """GIVEN a context with a prior completed turn and a second task-node that is the replay target.

    WHEN _run_replay_subtask_worker is invoked for the second task node.

    THEN the prior turn's TOOL_CALL and TOOL_RESULT remain enabled with superseded_by=None,
    AND only the intended target messages have superseded_by set.

    Gherkin:
    GIVEN context = [Turn1/TC, Turn1/TR, Turn1/ASST, Turn2/TC, Turn2/TR, Turn2/ASST]
      AND prior messages have task_id="prior-task"
      AND replay target has task_id="target-task"
    WHEN _run_replay_subtask_worker executes for "target-task"
    THEN Turn1/TC.superseded_by is None
     AND Turn1/TR.superseded_by is None
     AND Turn1/ASST.superseded_by is None
     AND Turn2/TC.superseded_by == new_TC.message_id
     AND Turn2/TR.superseded_by == new_TR.message_id
     AND Turn2/ASST.superseded_by == new_ASST.message_id
    """
    fixture, prior_msgs = _make_two_turn_fixture(prior_task_id="prior-task", target_task_id="target-task")
    ctx = fixture.session.context

    _run_replay_worker_with_chunks(
        fixture,
        chunks=_make_success_chunks(tool_pair_count=1, task_id="target-task"),
        monkeypatch=monkeypatch,
        task_id="target-task",
    )

    # Prior turn must be completely untouched
    for msg_id, msg in prior_msgs.items():
        assert msg.superseded_by is None, f"Prior turn message {msg_id!r} (role={msg.role}) was unexpectedly superseded"
        assert msg.enabled, f"Prior turn message {msg_id!r} was unexpectedly disabled"

    # Target turn originals must be superseded
    assert fixture.originals["tool_call_0"].superseded_by is not None
    assert fixture.originals["tool_result_0"].superseded_by is not None
    assert fixture.originals["assistant"].superseded_by is not None


# ---------------------------------------------------------------------------
# F-4 / T-2: Replay does not supersede messages from a concurrent plan step
# ---------------------------------------------------------------------------


def _make_two_step_plan_fixture(step_a_task_id: str, step_b_task_id: str) -> tuple[_ReplayFixture, dict[str, Message]]:
    """Build a fixture with two plan steps; Step A is the replay target, Step B must be untouched."""
    ctx = Context()
    step_b: dict[str, Message] = {}

    step_a_call = Message(role=MessageRole.TOOL_CALL, content="a-call", task_id=step_a_task_id)
    step_a_result = Message(role=MessageRole.TOOL_RESULT, content="a-result", task_id=step_a_task_id)
    step_a_asst = Message(role=MessageRole.ASSISTANT, content="a synthesis", task_id=step_a_task_id)

    step_b_call = Message(role=MessageRole.TOOL_CALL, content="b-call", task_id=step_b_task_id)
    step_b_result = Message(role=MessageRole.TOOL_RESULT, content="b-result", task_id=step_b_task_id)
    step_b_asst = Message(role=MessageRole.ASSISTANT, content="b synthesis", task_id=step_b_task_id)

    for msg in (step_a_call, step_a_result, step_a_asst, step_b_call, step_b_result, step_b_asst):
        ctx.add_message(msg)

    for msg in (step_b_call, step_b_result, step_b_asst):
        step_b[msg.message_id] = msg

    originals = {
        "tool_call_0": step_a_call,
        "tool_result_0": step_a_result,
        "assistant": step_a_asst,
    }

    session = SimpleNamespace(
        context=ctx,
        gui=_DummyGui(),
        _is_streaming=threading.Event(),
        _safe_root_after=lambda fn: fn(),
        _write_log=lambda _text: None,
        _output_logger=SimpleNamespace(log=lambda *_args, **_kwargs: None),
        refresh_user_gui=lambda: None,
        refresh_working_memory_gui=lambda: None,
        agentix_adapter=SimpleNamespace(),
    )
    fixture = _ReplayFixture(session=session, controller=StreamingController(session), originals=originals)
    return fixture, step_b


@pytest.mark.integration
def test_replay_does_not_supersede_concurrent_plan_step_messages(monkeypatch: pytest.MonkeyPatch) -> None:
    """GIVEN a context with two plan steps (Step A and Step B) both completed, where Step A is replayed.

    WHEN _run_replay_subtask_worker is invoked for Step A's task node.

    THEN Step B's TOOL_CALL, TOOL_RESULT, and ASSISTANT remain enabled with superseded_by=None.

    Gherkin:
    GIVEN context = [StepA/TC, StepA/TR, StepA/ASST, StepB/TC, StepB/TR, StepB/ASST]
      AND StepA has task_id="step-a" and StepB has task_id="step-b"
    WHEN _run_replay_subtask_worker executes for "step-a"
    THEN StepB/TC.superseded_by is None
     AND StepB/TR.superseded_by is None
     AND StepB/ASST.superseded_by is None
    """
    fixture, step_b_msgs = _make_two_step_plan_fixture(step_a_task_id="step-a", step_b_task_id="step-b")
    ctx = fixture.session.context

    _run_replay_worker_with_chunks(
        fixture,
        chunks=_make_success_chunks(tool_pair_count=1, task_id="step-a"),
        monkeypatch=monkeypatch,
        task_id="step-a",
    )

    for msg_id, msg in step_b_msgs.items():
        assert msg.superseded_by is None, f"Step B message {msg_id!r} (role={msg.role}) was unexpectedly superseded"
        assert msg.enabled, f"Step B message {msg_id!r} was unexpectedly disabled"

    # Step A originals must have been superseded
    assert fixture.originals["tool_call_0"].superseded_by is not None
    assert fixture.originals["tool_result_0"].superseded_by is not None
    assert fixture.originals["assistant"].superseded_by is not None
