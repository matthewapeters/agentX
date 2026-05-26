"""
Tests for shared.models.working_memory — FactEntry, WorkingMemory, PromotionResult.
"""

import json
import tempfile
from datetime import datetime
from pathlib import Path

import pytest

from shared.models.working_memory import (
    FactEntry,
    FactOwner,
    PromotionResult,
    PromotionStatus,
    WorkingMemory,
)

# ---------------------------------------------------------------------------
# FactOwner
# ---------------------------------------------------------------------------


class TestFactOwner:
    def test_values(self):
        assert FactOwner.USER.value == "user"
        assert FactOwner.AGENT.value == "agent"

    def test_icons(self):
        assert FactOwner.USER.icon == "👤"
        assert FactOwner.AGENT.icon == "🤖"


# ---------------------------------------------------------------------------
# FactEntry
# ---------------------------------------------------------------------------


class TestFactEntry:
    def _entry(self, owner=FactOwner.USER, key="project", value="AgentX"):
        return FactEntry(owner=owner, key=key, value=value)

    def test_compound_key_user(self):
        e = self._entry(owner=FactOwner.USER, key="lang")
        assert e.compound_key == "user:lang"

    def test_compound_key_agent(self):
        e = self._entry(owner=FactOwner.AGENT, key="task")
        assert e.compound_key == "agent:task"

    def test_owner_icon(self):
        assert self._entry(FactOwner.USER).owner_icon == "👤"
        assert self._entry(FactOwner.AGENT).owner_icon == "🤖"

    def test_to_dict_round_trip(self):
        e = FactEntry(owner=FactOwner.USER, key="x", value=42, enabled=False)
        d = e.to_dict()
        assert d["owner"] == "user"
        assert d["key"] == "x"
        assert d["value"] == 42
        assert d["enabled"] is False
        assert "timestamp" in d

        restored = FactEntry.from_dict(d)
        assert restored.owner == FactOwner.USER
        assert restored.key == "x"
        assert restored.value == 42
        assert restored.enabled is False

    def test_to_dict_complex_value(self):
        value = {"nested": [1, 2, 3], "flag": True}
        e = FactEntry(owner=FactOwner.AGENT, key="data", value=value)
        restored = FactEntry.from_dict(e.to_dict())
        assert restored.value == value

    def test_value_preview_string(self):
        e = FactEntry(owner=FactOwner.USER, key="k", value="hello")
        assert e.value_preview() == "hello"

    def test_value_preview_truncates(self):
        e = FactEntry(owner=FactOwner.USER, key="k", value="x" * 100)
        preview = e.value_preview(max_len=20)
        assert len(preview) <= 20
        assert preview.endswith("…")

    def test_to_llm_line_string(self):
        e = FactEntry(owner=FactOwner.USER, key="project", value="AgentX")
        assert e.to_llm_line() == "👤 project: AgentX"

    def test_to_llm_line_numeric(self):
        e = FactEntry(owner=FactOwner.AGENT, key="count", value=5)
        assert e.to_llm_line() == "🤖 count: 5"

    def test_from_dict_missing_timestamp_uses_now(self):
        data = {"owner": "user", "key": "k", "value": "v", "enabled": True}
        e = FactEntry.from_dict(data)
        assert isinstance(e.timestamp, datetime)


# ---------------------------------------------------------------------------
# WorkingMemory — CRUD
# ---------------------------------------------------------------------------


class TestWorkingMemoryCRUD:
    def test_add_and_retrieve(self):
        wm = WorkingMemory()
        entry = wm.add_fact(FactOwner.USER, "project", "AgentX")
        assert entry.compound_key == "user:project"
        assert wm.get("user:project") is not None

    def test_add_overwrites_existing(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "task", "v1")
        wm.add_fact(FactOwner.AGENT, "task", "v2")
        assert wm.get("agent:task").value == "v2"
        assert len(wm) == 1

    def test_remove_existing(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.USER, "k", "v")
        assert wm.remove_fact("user:k") is True
        assert wm.get("user:k") is None

    def test_remove_nonexistent_returns_false(self):
        wm = WorkingMemory()
        assert wm.remove_fact("user:missing") is False

    def test_set_enabled_true_false(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "k", "v")
        assert wm.set_enabled("agent:k", False) is True
        assert wm.get("agent:k").enabled is False
        wm.set_enabled("agent:k", True)
        assert wm.get("agent:k").enabled is True

    def test_set_enabled_missing_returns_false(self):
        wm = WorkingMemory()
        assert wm.set_enabled("user:ghost", True) is False

    def test_bool_empty(self):
        assert not WorkingMemory()

    def test_bool_nonempty(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.USER, "k", "v")
        assert wm

    def test_len(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.USER, "a", 1)
        wm.add_fact(FactOwner.AGENT, "b", 2)
        assert len(wm) == 2


# ---------------------------------------------------------------------------
# WorkingMemory — get_enabled_facts / all_facts ordering
# ---------------------------------------------------------------------------


class TestWorkingMemoryOrdering:
    def test_enabled_facts_user_before_agent(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "z_agent", "a")
        wm.add_fact(FactOwner.USER, "a_user", "b")
        facts = wm.get_enabled_facts()
        assert facts[0].owner == FactOwner.USER
        assert facts[1].owner == FactOwner.AGENT

    def test_enabled_facts_excludes_disabled(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.USER, "visible", "yes")
        wm.add_fact(FactOwner.USER, "hidden", "no", enabled=False)
        keys = [f.key for f in wm.get_enabled_facts()]
        assert "visible" in keys
        assert "hidden" not in keys

    def test_all_facts_includes_disabled(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.USER, "visible", "yes")
        wm.add_fact(FactOwner.AGENT, "hidden", "no", enabled=False)
        assert len(wm.all_facts()) == 2


# ---------------------------------------------------------------------------
# WorkingMemory — promote_to_user
# ---------------------------------------------------------------------------


class TestPromoteToUser:
    def test_promote_ok(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "task", "build")
        result = wm.promote_to_user("agent:task")
        assert result.ok
        assert wm.get("agent:task") is None
        promoted = wm.get("user:task")
        assert promoted is not None
        assert promoted.owner == FactOwner.USER
        assert promoted.value == "build"

    def test_promote_preserves_value_and_enabled(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "k", [1, 2, 3], enabled=False)
        wm.promote_to_user("agent:k")
        fact = wm.get("user:k")
        assert fact.value == [1, 2, 3]
        assert fact.enabled is False

    def test_promote_conflict_no_force(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "k", "agent_val")
        wm.add_fact(FactOwner.USER, "k", "user_val")
        result = wm.promote_to_user("agent:k")
        assert result.status == PromotionStatus.CONFLICT
        assert result.conflicting_value == "user_val"
        # Neither entry should be removed
        assert wm.get("agent:k") is not None
        assert wm.get("user:k").value == "user_val"

    def test_promote_conflict_with_force_overwrites(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "k", "new")
        wm.add_fact(FactOwner.USER, "k", "old")
        result = wm.promote_to_user("agent:k", force=True)
        assert result.ok
        assert wm.get("user:k").value == "new"
        assert wm.get("agent:k") is None

    def test_promote_with_rename(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "tmp", "val")
        result = wm.promote_to_user("agent:tmp", new_key="permanent")
        assert result.ok
        assert wm.get("user:permanent") is not None
        assert wm.get("agent:tmp") is None
        assert wm.get("user:tmp") is None

    def test_promote_rename_collision_blocked(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "src", "v")
        wm.add_fact(FactOwner.USER, "dst", "existing")
        result = wm.promote_to_user("agent:src", new_key="dst")
        assert result.status == PromotionStatus.CONFLICT
        assert result.conflicting_value == "existing"

    def test_promote_not_found(self):
        wm = WorkingMemory()
        result = wm.promote_to_user("agent:ghost")
        assert result.status == PromotionStatus.NOT_FOUND

    def test_promote_user_owned_returns_not_agent(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.USER, "k", "v")
        result = wm.promote_to_user("user:k")
        assert result.status == PromotionStatus.NOT_AGENT_OWNED

    def test_promote_total_count_unchanged(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "k", "v")
        wm.add_fact(FactOwner.USER, "other", "x")
        wm.promote_to_user("agent:k")
        assert len(wm) == 2  # agent:k became user:k; user:other unchanged


# ---------------------------------------------------------------------------
# WorkingMemory — LLM block
# ---------------------------------------------------------------------------


class TestLLMBlock:
    def test_empty_returns_empty_string(self):
        assert WorkingMemory().to_llm_block() == ""

    def test_all_disabled_returns_empty_string(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.USER, "k", "v", enabled=False)
        assert wm.to_llm_block() == ""

    def test_block_contains_fact_lines(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.USER, "project", "AgentX")
        wm.add_fact(FactOwner.AGENT, "task", "coding")
        block = wm.to_llm_block()
        assert "<working_memory>" in block
        assert "👤 project: AgentX" in block
        assert "🤖 task: coding" in block
        assert "</working_memory>" in block

    def test_block_user_facts_before_agent(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.AGENT, "z", "last")
        wm.add_fact(FactOwner.USER, "a", "first")
        block = wm.to_llm_block()
        assert block.index("👤") < block.index("🤖")


# ---------------------------------------------------------------------------
# WorkingMemory — persistence
# ---------------------------------------------------------------------------


class TestPersistence:
    def test_save_and_load_round_trip(self, tmp_path):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.USER, "project", "AgentX")
        wm.add_fact(FactOwner.AGENT, "task", [1, 2, 3], enabled=False)
        wm.save(tmp_path)

        loaded = WorkingMemory.load(tmp_path)
        assert len(loaded) == 2
        assert loaded.get("user:project").value == "AgentX"
        assert loaded.get("agent:task").value == [1, 2, 3]
        assert loaded.get("agent:task").enabled is False

    def test_load_missing_path_returns_empty(self, tmp_path):
        wm = WorkingMemory.load(tmp_path / "nonexistent")
        assert len(wm) == 0

    def test_load_corrupt_file_returns_empty(self, tmp_path):
        bad = tmp_path / "working_memory.json"
        bad.write_text("not json{{{", encoding="utf-8")
        wm = WorkingMemory.load(tmp_path)
        assert len(wm) == 0

    def test_autosave_on_add(self, tmp_path):
        wm = WorkingMemory()
        wm.set_path(tmp_path)
        wm.add_fact(FactOwner.USER, "auto", "saved")
        assert (tmp_path / "working_memory.json").exists()

    def test_autosave_on_remove(self, tmp_path):
        wm = WorkingMemory()
        wm.set_path(tmp_path)
        wm.add_fact(FactOwner.USER, "k", "v")
        wm.remove_fact("user:k")
        loaded = WorkingMemory.load(tmp_path)
        assert len(loaded) == 0

    def test_to_dict_from_dict_round_trip(self):
        wm = WorkingMemory()
        wm.add_fact(FactOwner.USER, "x", {"nested": True})
        wm.add_fact(FactOwner.AGENT, "y", 99)
        restored = WorkingMemory.from_dict(wm.to_dict())
        assert len(restored) == 2
        assert restored.get("user:x").value == {"nested": True}
        assert restored.get("agent:y").value == 99
