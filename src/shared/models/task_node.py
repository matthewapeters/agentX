"""
Hierarchical task execution data models.

These models represent the plan/task tree that drives AgentX's hierarchical
task execution system.  They are persisted alongside the conversation context
in the session directory:

    sessions/<user>/<session>/
        context/            ← existing message files
        plans/              ← PlanRecord JSON files  (<epoch>_plan.json)
        task_nodes/         ← TaskNodeRecord JSON files  (<epoch>_task_node.json)
        task_tree.json      ← TaskTree index (single file, updated in-place)
        scratch/            ← ephemeral per-task scratch files

Key concepts
------------
PlanRecord   – A single execution plan (ordered list of PlanSteps).  Each step
               may be concrete or TBD (to-be-determined, resolved at runtime).
TaskNodeRecord – A single node in the task tree.  Nodes carry a reference back
               to the PlanStep they realise and track their own sub-tasks,
               assertions, and synthesis attempts.
TaskTree     – In-memory index aligning plans and nodes to a session.  Persisted
               as a single ``task_tree.json`` file so the GUI can reconstruct the
               tree without scanning individual record files.
"""

from __future__ import annotations

import json
import os
import time
from dataclasses import dataclass, field
from typing import Optional

# ---------------------------------------------------------------------------
# Leaf dataclasses (no save/load — embedded inside larger records)
# ---------------------------------------------------------------------------


@dataclass
class AssertionRecord:
    """A single verifiable fact asserted during task execution.

    Attributes:
        fact: Natural language description of the expected condition.
        type: One of ``"pre"``, ``"post"``, or ``"invariant"``.
        check: Optional snippet or description of how to verify the fact.
        verified: ``True`` if the assertion passed, ``False`` if it failed,
                  ``None`` if not yet evaluated.
        error: Failure reason if ``verified is False``.
    """

    fact: str
    type: str = "post"  # "pre" | "post" | "invariant"
    check: Optional[str] = None
    verified: Optional[bool] = None
    error: Optional[str] = None

    def to_dict(self) -> dict:
        d: dict = {"fact": self.fact, "type": self.type}
        if self.check is not None:
            d["check"] = self.check
        if self.verified is not None:
            d["verified"] = self.verified
        if self.error is not None:
            d["error"] = self.error
        return d

    @classmethod
    def from_dict(cls, data: dict) -> "AssertionRecord":
        return cls(
            fact=data["fact"],
            type=data.get("type", "post"),
            check=data.get("check"),
            verified=data.get("verified"),
            error=data.get("error"),
        )


@dataclass
class SynthesisAttempt:
    """Records one attempt to synthesise the result of a task node.

    Attributes:
        epoch: Unix timestamp (float) when the attempt was made.
        status: One of ``"accepted"``, ``"rejected"``, ``"pending"``.
        rejected_epochs: Epochs of child messages that were discarded on rejection.
    """

    epoch: float
    status: str = "pending"  # "accepted" | "rejected" | "pending"
    rejected_epochs: list[float] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "epoch": self.epoch,
            "status": self.status,
            "rejected_epochs": self.rejected_epochs,
        }

    @classmethod
    def from_dict(cls, data: dict) -> "SynthesisAttempt":
        return cls(
            epoch=data["epoch"],
            status=data.get("status", "pending"),
            rejected_epochs=data.get("rejected_epochs", []),
        )


@dataclass
class PlanStep:
    """One step inside a PlanRecord.

    Attributes:
        step_id: Unique identifier within the plan (e.g. ``"step_0"``).
        description: Human-readable description of what this step does.
        tbd: If ``True``, the description is a placeholder that must be
             resolved before the step can be executed.
        depends_on: List of ``step_id`` values that must complete first.
    """

    step_id: str
    description: str
    tbd: bool = False
    depends_on: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        d: dict = {
            "step_id": self.step_id,
            "description": self.description,
        }
        if self.tbd:
            d["tbd"] = True
        if self.depends_on:
            d["depends_on"] = self.depends_on
        return d

    @classmethod
    def from_dict(cls, data: dict) -> "PlanStep":
        return cls(
            step_id=data["step_id"],
            description=data["description"],
            tbd=data.get("tbd", False),
            depends_on=data.get("depends_on", []),
        )


# ---------------------------------------------------------------------------
# PlanRecord
# ---------------------------------------------------------------------------


@dataclass
class PlanRecord:
    """A complete execution plan for a user request.

    One plan is created per top-level user interaction that triggers the
    hierarchical task system.  Plans are persisted as individual JSON files
    inside the ``plans/`` sub-directory of the session.

    Attributes:
        plan_id: Unique plan identifier (e.g. ``"plan_abc123"``).
        plan_name: Short human-readable title.
        session_plan_index: Zero-based ordinal of this plan within the session.
        steps: Ordered list of PlanStep objects.
        root_task_ids: Task IDs of the top-level nodes spawned by this plan.
        status: One of ``"pending"``, ``"running"``, ``"done"``, ``"failed"``.
        epoch: Unix timestamp (float) when this record was created.
    """

    plan_id: str
    plan_name: str
    session_plan_index: int = 0
    steps: list[PlanStep] = field(default_factory=list)
    root_task_ids: list[str] = field(default_factory=list)
    status: str = "pending"  # "pending" | "running" | "done" | "failed"
    epoch: float = field(default_factory=time.time)

    # ------------------------------------------------------------------
    # Serialisation
    # ------------------------------------------------------------------

    def to_dict(self) -> dict:
        return {
            "plan_id": self.plan_id,
            "plan_name": self.plan_name,
            "session_plan_index": self.session_plan_index,
            "steps": [s.to_dict() for s in self.steps],
            "root_task_ids": self.root_task_ids,
            "status": self.status,
            "epoch": self.epoch,
        }

    @classmethod
    def from_dict(cls, data: dict) -> "PlanRecord":
        return cls(
            plan_id=data["plan_id"],
            plan_name=data["plan_name"],
            session_plan_index=data.get("session_plan_index", 0),
            steps=[PlanStep.from_dict(s) for s in data.get("steps", [])],
            root_task_ids=data.get("root_task_ids", []),
            status=data.get("status", "pending"),
            epoch=data.get("epoch", 0.0),
        )

    # ------------------------------------------------------------------
    # Persistence
    # ------------------------------------------------------------------

    def save(self, plans_dir: str) -> str:
        """Persist this record to *plans_dir* and return the file path."""
        os.makedirs(plans_dir, exist_ok=True)
        file_path = os.path.join(plans_dir, f"{self.epoch}_{self.plan_id}.json")
        with open(file_path, "w", encoding="utf-8") as fh:
            json.dump(self.to_dict(), fh, indent=2)
        return file_path

    @classmethod
    def load(cls, file_path: str) -> "PlanRecord":
        """Load a PlanRecord from *file_path*."""
        with open(file_path, "r", encoding="utf-8") as fh:
            data = json.load(fh)
        return cls.from_dict(data)


# ---------------------------------------------------------------------------
# TaskNodeRecord
# ---------------------------------------------------------------------------


@dataclass
class TaskNodeRecord:
    """A single node in the execution task tree.

    Each TaskNodeRecord corresponds to one unit of work — either a top-level
    goal or a sub-task spawned dynamically.  Nodes are persisted as individual
    JSON files inside the ``task_nodes/`` sub-directory of the session.

    Attributes:
        plan_id: ID of the parent PlanRecord.
        task_id: Unique identifier for this node (e.g. ``"task_abc123"``).
        parent_task_id: ID of the parent task, or ``None`` for root tasks.
        depth: Nesting depth (root = 0, max = 10 per design spec).
        plan_step_index: Index into the parent plan's ``steps`` list.
        task_description: What this task must accomplish.
        tbd: If ``True``, the description needs runtime resolution.
        tbd_resolved_description: Concrete description once TBD resolved.
        status: One of ``"pending"``, ``"running"``, ``"done"``, ``"failed"``.
        child_message_epochs: Epochs of messages produced directly by this task.
        child_task_ids: IDs of sub-tasks spawned by this task.
        synthesis_epoch: Epoch of the accepted synthesis message, or ``None``.
        scratch_file: Path to the scratch file for working memory.
        assertions: Assertions to verify before/after this task.
        synthesis_attempts: History of synthesis attempts.
        wm_hints_added: Whether working-memory hints have been injected.
        epoch: Unix timestamp (float) when this record was created.
        enabled: Whether this node is active (supports soft-delete).
    """

    plan_id: str
    task_id: str
    parent_task_id: Optional[str] = None
    depth: int = 0
    plan_step_index: int = 0
    task_description: str = ""
    tbd: bool = False
    tbd_resolved_description: Optional[str] = None
    status: str = "pending"  # "pending" | "running" | "done" | "failed"
    child_message_epochs: list[float] = field(default_factory=list)
    child_task_ids: list[str] = field(default_factory=list)
    synthesis_epoch: Optional[float] = None
    scratch_file: Optional[str] = None
    assertions: list[AssertionRecord] = field(default_factory=list)
    synthesis_attempts: list[SynthesisAttempt] = field(default_factory=list)
    wm_hints_added: bool = False
    epoch: float = field(default_factory=time.time)
    enabled: bool = True

    # ------------------------------------------------------------------
    # Serialisation
    # ------------------------------------------------------------------

    def to_dict(self) -> dict:
        d: dict = {
            "plan_id": self.plan_id,
            "task_id": self.task_id,
            "depth": self.depth,
            "plan_step_index": self.plan_step_index,
            "task_description": self.task_description,
            "tbd": self.tbd,
            "status": self.status,
            "child_message_epochs": self.child_message_epochs,
            "child_task_ids": self.child_task_ids,
            "assertions": [a.to_dict() for a in self.assertions],
            "synthesis_attempts": [s.to_dict() for s in self.synthesis_attempts],
            "wm_hints_added": self.wm_hints_added,
            "epoch": self.epoch,
            "enabled": self.enabled,
        }
        if self.parent_task_id is not None:
            d["parent_task_id"] = self.parent_task_id
        if self.tbd_resolved_description is not None:
            d["tbd_resolved_description"] = self.tbd_resolved_description
        if self.synthesis_epoch is not None:
            d["synthesis_epoch"] = self.synthesis_epoch
        if self.scratch_file is not None:
            d["scratch_file"] = self.scratch_file
        return d

    @classmethod
    def from_dict(cls, data: dict) -> "TaskNodeRecord":
        return cls(
            plan_id=data["plan_id"],
            task_id=data["task_id"],
            parent_task_id=data.get("parent_task_id"),
            depth=data.get("depth", 0),
            plan_step_index=data.get("plan_step_index", 0),
            task_description=data.get("task_description", ""),
            tbd=data.get("tbd", False),
            tbd_resolved_description=data.get("tbd_resolved_description"),
            status=data.get("status", "pending"),
            child_message_epochs=data.get("child_message_epochs", []),
            child_task_ids=data.get("child_task_ids", []),
            synthesis_epoch=data.get("synthesis_epoch"),
            scratch_file=data.get("scratch_file"),
            assertions=[AssertionRecord.from_dict(a) for a in data.get("assertions", [])],
            synthesis_attempts=[SynthesisAttempt.from_dict(s) for s in data.get("synthesis_attempts", [])],
            wm_hints_added=data.get("wm_hints_added", False),
            epoch=data.get("epoch", 0.0),
            enabled=data.get("enabled", True),
        )

    # ------------------------------------------------------------------
    # Persistence
    # ------------------------------------------------------------------

    def save(self, task_nodes_dir: str) -> str:
        """Persist this record to *task_nodes_dir* and return the file path."""
        os.makedirs(task_nodes_dir, exist_ok=True)
        file_path = os.path.join(task_nodes_dir, f"{self.epoch}_{self.task_id}.json")
        with open(file_path, "w", encoding="utf-8") as fh:
            json.dump(self.to_dict(), fh, indent=2)
        return file_path

    @classmethod
    def load(cls, file_path: str) -> "TaskNodeRecord":
        """Load a TaskNodeRecord from *file_path*."""
        with open(file_path, "r", encoding="utf-8") as fh:
            data = json.load(fh)
        return cls.from_dict(data)


# ---------------------------------------------------------------------------
# TaskTree (session-level index)
# ---------------------------------------------------------------------------


@dataclass
class TaskTree:
    """Session-level index of all plans and task nodes.

    Persisted as a single ``task_tree.json`` in the session root directory
    (sibling of ``context/``).  Updated in-place whenever a plan or node
    changes status so the GUI can reconstruct the tree without scanning
    individual record files.

    Attributes:
        session_id: The session this tree belongs to.
        plans: Map of plan_id → PlanRecord.
        nodes: Map of task_id → TaskNodeRecord.
        created_epoch: When the tree was first created.
        last_updated_epoch: When the tree was last written.
    """

    session_id: str
    plans: dict[str, PlanRecord] = field(default_factory=dict)
    nodes: dict[str, TaskNodeRecord] = field(default_factory=dict)
    created_epoch: float = field(default_factory=time.time)
    last_updated_epoch: float = field(default_factory=time.time)

    # ------------------------------------------------------------------
    # Convenience
    # ------------------------------------------------------------------

    def add_plan(self, plan: PlanRecord) -> None:
        """Register a plan in the index."""
        self.plans[plan.plan_id] = plan
        self.last_updated_epoch = time.time()

    def add_node(self, node: TaskNodeRecord) -> None:
        """Register a task node in the index."""
        self.nodes[node.task_id] = node
        self.last_updated_epoch = time.time()

    def get_children(self, task_id: str) -> list[TaskNodeRecord]:
        """Return direct child nodes for *task_id* in insertion order."""
        return [n for n in self.nodes.values() if n.parent_task_id == task_id]

    def get_root_nodes(self, plan_id: str) -> list[TaskNodeRecord]:
        """Return root-level task nodes for *plan_id*."""
        plan = self.plans.get(plan_id)
        if plan is None:
            return []
        return [self.nodes[tid] for tid in plan.root_task_ids if tid in self.nodes]

    # ------------------------------------------------------------------
    # Serialisation
    # ------------------------------------------------------------------

    def to_dict(self) -> dict:
        return {
            "session_id": self.session_id,
            "plans": {pid: p.to_dict() for pid, p in self.plans.items()},
            "nodes": {tid: n.to_dict() for tid, n in self.nodes.items()},
            "created_epoch": self.created_epoch,
            "last_updated_epoch": self.last_updated_epoch,
        }

    @classmethod
    def from_dict(cls, data: dict) -> "TaskTree":
        tree = cls(
            session_id=data["session_id"],
            created_epoch=data.get("created_epoch", 0.0),
            last_updated_epoch=data.get("last_updated_epoch", 0.0),
        )
        for pid, pd in data.get("plans", {}).items():
            tree.plans[pid] = PlanRecord.from_dict(pd)
        for tid, nd in data.get("nodes", {}).items():
            tree.nodes[tid] = TaskNodeRecord.from_dict(nd)
        return tree

    # ------------------------------------------------------------------
    # Persistence
    # ------------------------------------------------------------------

    def save(self, session_dir: str) -> str:
        """Persist the tree to ``<session_dir>/task_tree.json``."""
        os.makedirs(session_dir, exist_ok=True)
        self.last_updated_epoch = time.time()
        file_path = os.path.join(session_dir, "task_tree.json")
        with open(file_path, "w", encoding="utf-8") as fh:
            json.dump(self.to_dict(), fh, indent=2)
        return file_path

    @classmethod
    def load(cls, session_dir: str) -> "TaskTree":
        """Load the tree from ``<session_dir>/task_tree.json``."""
        file_path = os.path.join(session_dir, "task_tree.json")
        with open(file_path, "r", encoding="utf-8") as fh:
            data = json.load(fh)
        return cls.from_dict(data)
