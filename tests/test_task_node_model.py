"""
Unit tests for Phase 1 hierarchical task execution data models.

Covers:
- PlanRecord round-trip serialisation
- TaskNodeRecord with TBD=True
- TaskTree indexing across multiple plans
- Context.save_plan / load_plans
- Context.save_task_node / load_task_nodes
- Context.save_task_tree / load_task_tree
- Context.get_scratch_dir creation
- Message.to_llm_dict() guard for internal roles
- Context.to_llm_messages() filters internal roles
"""

import os
import tempfile
import time

import pytest

from src.shared.models.task_node import (
    AssertionRecord,
    PlanRecord,
    PlanStep,
    SynthesisAttempt,
    TaskNodeRecord,
    TaskTree,
)
from src.shared.models.context import Context
from src.shared.models.message import Message, MessageRole

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_plan(idx: int = 0) -> PlanRecord:
    return PlanRecord(
        plan_id=f"plan_{idx}",
        plan_name=f"Test plan {idx}",
        session_plan_index=idx,
        steps=[
            PlanStep(step_id="step_0", description="Do something"),
            PlanStep(step_id="step_1", description="TBD", tbd=True),
        ],
        root_task_ids=[f"task_{idx}_0"],
        status="pending",
        epoch=1_700_000_000.0 + idx,
    )


def _make_node(plan_id: str = "plan_0", task_id: str = "task_0") -> TaskNodeRecord:
    return TaskNodeRecord(
        plan_id=plan_id,
        task_id=task_id,
        parent_task_id=None,
        depth=0,
        plan_step_index=0,
        task_description="Do something concrete",
        tbd=False,
        status="pending",
        assertions=[AssertionRecord(fact="Output is non-empty", type="post")],
        synthesis_attempts=[SynthesisAttempt(epoch=1_700_000_001.0, status="accepted")],
        epoch=1_700_000_000.0,
    )


# ---------------------------------------------------------------------------
# AssertionRecord
# ---------------------------------------------------------------------------


class TestAssertionRecord:
    def test_round_trip(self):
        a = AssertionRecord(fact="x > 0", type="pre", check="assert x > 0", verified=True)
        assert AssertionRecord.from_dict(a.to_dict()) == a

    def test_sparse_serialisation(self):
        a = AssertionRecord(fact="y is not None")
        d = a.to_dict()
        assert "check" not in d
        assert "verified" not in d
        assert "error" not in d

    def test_failed_assertion(self):
        a = AssertionRecord(fact="z == 1", verified=False, error="z was 2")
        d = a.to_dict()
        assert d["verified"] is False
        assert d["error"] == "z was 2"
        assert AssertionRecord.from_dict(d).error == "z was 2"


# ---------------------------------------------------------------------------
# SynthesisAttempt
# ---------------------------------------------------------------------------


class TestSynthesisAttempt:
    def test_round_trip(self):
        s = SynthesisAttempt(epoch=1.0, status="rejected", rejected_epochs=[2.0, 3.0])
        assert SynthesisAttempt.from_dict(s.to_dict()) == s

    def test_defaults(self):
        s = SynthesisAttempt(epoch=5.0)
        assert s.status == "pending"
        assert s.rejected_epochs == []


# ---------------------------------------------------------------------------
# PlanStep
# ---------------------------------------------------------------------------


class TestPlanStep:
    def test_round_trip(self):
        ps = PlanStep(step_id="s0", description="step desc", tbd=True, depends_on=["s1"])
        assert PlanStep.from_dict(ps.to_dict()) == ps

    def test_tbd_omitted_when_false(self):
        ps = PlanStep(step_id="s0", description="desc")
        assert "tbd" not in ps.to_dict()

    def test_depends_on_omitted_when_empty(self):
        ps = PlanStep(step_id="s0", description="desc")
        assert "depends_on" not in ps.to_dict()


# ---------------------------------------------------------------------------
# PlanRecord
# ---------------------------------------------------------------------------


class TestPlanRecord:
    def test_round_trip(self):
        p = _make_plan(0)
        p2 = PlanRecord.from_dict(p.to_dict())
        assert p2.plan_id == p.plan_id
        assert p2.plan_name == p.plan_name
        assert len(p2.steps) == 2
        assert p2.steps[1].tbd is True
        assert p2.root_task_ids == ["task_0_0"]

    def test_save_and_load(self, tmp_path):
        p = _make_plan(1)
        plans_dir = str(tmp_path / "plans")
        file_path = p.save(plans_dir)
        assert os.path.isfile(file_path)
        loaded = PlanRecord.load(file_path)
        assert loaded.plan_id == p.plan_id
        assert loaded.steps[0].step_id == "step_0"

    def test_save_creates_directory(self, tmp_path):
        p = _make_plan(2)
        nested = str(tmp_path / "deep" / "plans")
        p.save(nested)
        assert os.path.isdir(nested)


# ---------------------------------------------------------------------------
# TaskNodeRecord
# ---------------------------------------------------------------------------


class TestTaskNodeRecord:
    def test_round_trip(self):
        node = _make_node()
        node2 = TaskNodeRecord.from_dict(node.to_dict())
        assert node2.task_id == node.task_id
        assert node2.assertions[0].fact == "Output is non-empty"
        assert node2.synthesis_attempts[0].status == "accepted"

    def test_tbd_node(self):
        node = TaskNodeRecord(
            plan_id="p0",
            task_id="t_tbd",
            task_description="TBD — resolve at runtime",
            tbd=True,
            tbd_resolved_description=None,
        )
        d = node.to_dict()
        assert d["tbd"] is True
        assert "tbd_resolved_description" not in d

        node.tbd_resolved_description = "Write tests for module X"
        d2 = node.to_dict()
        assert d2["tbd_resolved_description"] == "Write tests for module X"
        node2 = TaskNodeRecord.from_dict(d2)
        assert node2.tbd_resolved_description == "Write tests for module X"

    def test_parent_child_relationship(self):
        root = TaskNodeRecord(plan_id="p", task_id="root", depth=0)
        child = TaskNodeRecord(plan_id="p", task_id="child", parent_task_id="root", depth=1)
        assert child.parent_task_id == "root"
        child2 = TaskNodeRecord.from_dict(child.to_dict())
        assert child2.parent_task_id == "root"
        assert child2.depth == 1

    def test_save_and_load(self, tmp_path):
        node = _make_node()
        nodes_dir = str(tmp_path / "task_nodes")
        file_path = node.save(nodes_dir)
        assert os.path.isfile(file_path)
        loaded = TaskNodeRecord.load(file_path)
        assert loaded.task_id == node.task_id
        assert loaded.assertions[0].type == "post"

    def test_enabled_default_true(self):
        node = TaskNodeRecord(plan_id="p", task_id="t")
        assert node.enabled is True

    def test_disabled_node_round_trip(self):
        node = TaskNodeRecord(plan_id="p", task_id="t", enabled=False)
        assert TaskNodeRecord.from_dict(node.to_dict()).enabled is False


# ---------------------------------------------------------------------------
# TaskTree
# ---------------------------------------------------------------------------


class TestTaskTree:
    def test_add_and_retrieve(self):
        tree = TaskTree(session_id="sess_1")
        p = _make_plan(0)
        n = _make_node(plan_id=p.plan_id, task_id="task_0_0")
        p.root_task_ids = ["task_0_0"]
        tree.add_plan(p)
        tree.add_node(n)

        assert tree.plans["plan_0"].plan_name == "Test plan 0"
        assert tree.nodes["task_0_0"].depth == 0
        roots = tree.get_root_nodes("plan_0")
        assert len(roots) == 1 and roots[0].task_id == "task_0_0"

    def test_round_trip(self):
        tree = TaskTree(session_id="sess_2")
        for i in range(3):
            p = _make_plan(i)
            tree.add_plan(p)
            n = _make_node(plan_id=p.plan_id, task_id=f"task_{i}_0")
            tree.add_node(n)
        d = tree.to_dict()
        tree2 = TaskTree.from_dict(d)
        assert len(tree2.plans) == 3
        assert len(tree2.nodes) == 3

    def test_save_and_load(self, tmp_path):
        tree = TaskTree(session_id="sess_3")
        tree.add_plan(_make_plan(0))
        tree.add_node(_make_node())
        session_dir = str(tmp_path)
        file_path = tree.save(session_dir)
        assert os.path.isfile(file_path)
        loaded = TaskTree.load(session_dir)
        assert loaded.session_id == "sess_3"
        assert "plan_0" in loaded.plans

    def test_get_children(self):
        tree = TaskTree(session_id="sess_4")
        root = TaskNodeRecord(plan_id="p", task_id="root", depth=0)
        c1 = TaskNodeRecord(plan_id="p", task_id="c1", parent_task_id="root", depth=1)
        c2 = TaskNodeRecord(plan_id="p", task_id="c2", parent_task_id="root", depth=1)
        for n in (root, c1, c2):
            tree.add_node(n)
        children = tree.get_children("root")
        assert {c.task_id for c in children} == {"c1", "c2"}


# ---------------------------------------------------------------------------
# Context persistence helpers
# ---------------------------------------------------------------------------


class TestContextTaskPersistence:
    def _context_in(self, tmp_path) -> Context:
        context_dir = str(tmp_path / "context")
        os.makedirs(context_dir, exist_ok=True)
        ctx = Context(path=context_dir, session_id="test_session")
        return ctx

    def test_save_and_load_plan(self, tmp_path):
        ctx = self._context_in(tmp_path)
        plan = _make_plan(0)
        ctx.save_plan(plan)
        loaded = ctx.load_plans()
        assert len(loaded) == 1
        assert loaded[0].plan_id == "plan_0"

    def test_load_plans_empty_when_no_dir(self, tmp_path):
        ctx = self._context_in(tmp_path)
        # plans/ directory does not exist yet
        plans = ctx.load_plans()
        assert plans == []

    def test_save_and_load_task_node(self, tmp_path):
        ctx = self._context_in(tmp_path)
        node = _make_node()
        ctx.save_task_node(node)
        loaded = ctx.load_task_nodes()
        assert len(loaded) == 1
        assert loaded[0].task_id == "task_0"

    def test_load_task_nodes_empty_when_no_dir(self, tmp_path):
        ctx = self._context_in(tmp_path)
        assert ctx.load_task_nodes() == []

    def test_save_and_load_task_tree(self, tmp_path):
        ctx = self._context_in(tmp_path)
        tree = TaskTree(session_id="test_session")
        tree.add_plan(_make_plan(0))
        tree.add_node(_make_node())
        ctx.save_task_tree(tree)
        loaded = ctx.load_task_tree()
        assert loaded is not None
        assert loaded.session_id == "test_session"
        assert "plan_0" in loaded.plans

    def test_load_task_tree_returns_none_when_missing(self, tmp_path):
        ctx = self._context_in(tmp_path)
        assert ctx.load_task_tree() is None

    def test_get_scratch_dir_creates_directory(self, tmp_path):
        ctx = self._context_in(tmp_path)
        scratch = ctx.get_scratch_dir()
        assert os.path.isdir(scratch)
        assert scratch.endswith("scratch")

    def test_get_scratch_dir_idempotent(self, tmp_path):
        ctx = self._context_in(tmp_path)
        d1 = ctx.get_scratch_dir()
        d2 = ctx.get_scratch_dir()
        assert d1 == d2

    def test_no_path_raises(self):
        ctx = Context(session_id="no_path")
        with pytest.raises(ValueError, match="no path"):
            ctx.save_plan(_make_plan())
        with pytest.raises(ValueError, match="no path"):
            ctx.save_task_node(_make_node())
        with pytest.raises(ValueError, match="no path"):
            ctx.save_task_tree(TaskTree(session_id="x"))
        with pytest.raises(ValueError, match="no path"):
            ctx.get_scratch_dir()


# ---------------------------------------------------------------------------
# Message.to_llm_dict() guard for internal roles
# ---------------------------------------------------------------------------


class TestInternalRoleGuard:
    @pytest.mark.parametrize(
        "role",
        [
            MessageRole.PLAN,
            MessageRole.TASK_NODE,
            MessageRole.SYNTHESIS,
            MessageRole.ASSERTION,
        ],
    )
    def test_to_llm_dict_raises_for_internal_role(self, role):
        msg = Message(role=role, content="test")
        with pytest.raises(ValueError, match="internal task-execution record"):
            msg.to_llm_dict()

    def test_to_llm_dict_works_for_standard_roles(self):
        for role in (MessageRole.USER, MessageRole.ASSISTANT, MessageRole.SYSTEM):
            msg = Message(role=role, content="hello")
            d = msg.to_llm_dict()
            assert "role" in d


# ---------------------------------------------------------------------------
# Context.to_llm_messages() filters internal roles
# ---------------------------------------------------------------------------


class TestContextToLlmMessages:
    def test_internal_roles_filtered(self):
        ctx = Context()
        ctx.messages = []

        def _add(role, content):
            from datetime import datetime
            from src.shared.models.context import MessageEntry

            msg = Message(role=role, content=content)
            msg.enabled = True
            ctx.messages.append(MessageEntry(timestamp=datetime.now(), message=msg))

        _add(MessageRole.USER, "hello")
        _add(MessageRole.PLAN, "plan data")
        _add(MessageRole.TASK_NODE, "node data")
        _add(MessageRole.SYNTHESIS, "synthesis")
        _add(MessageRole.ASSERTION, "assertion")
        _add(MessageRole.ASSISTANT, "world")

        llm_msgs = ctx.to_llm_messages()
        roles_sent = {m["role"] for m in llm_msgs}
        assert roles_sent == {"user", "assistant"}
        assert len(llm_msgs) == 2
