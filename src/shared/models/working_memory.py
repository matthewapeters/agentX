"""
shared.models.working_memory

Per-session key-value fact store (Working Memory) for AgentX.

Facts have a two-part compound key: "{owner}:{key}", where owner is "user" or "agent".
Ownership rules:
  - user-owned facts: only the user may create or mutate; agent may read but not write.
  - agent-owned facts: the agent may add, update, or disable autonomously; the user
    may promote them to user-owned (changing ownership in-place, no cloning).

Persistence: one JSON file per session at {session_dir}/working_memory.json.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from pathlib import Path
from typing import Any, Optional

# ---------------------------------------------------------------------------
# Types
# ---------------------------------------------------------------------------

FactValue = str | int | float | list[Any] | dict[Any, Any]


class FactOwner(str, Enum):
    USER = "user"
    AGENT = "agent"

    @property
    def icon(self) -> str:
        return "👤" if self == FactOwner.USER else "🤖"


class PromotionStatus(Enum):
    OK = "ok"
    CONFLICT = "conflict"
    NOT_FOUND = "not_found"
    NOT_AGENT_OWNED = "not_agent_owned"


@dataclass
class PromotionResult:
    status: PromotionStatus
    # Populated on CONFLICT — the value already stored under the target user key
    conflicting_value: Optional[FactValue] = None

    @property
    def ok(self) -> bool:
        return self.status == PromotionStatus.OK

    @property
    def conflict(self) -> bool:
        return self.status == PromotionStatus.CONFLICT


# ---------------------------------------------------------------------------
# FactEntry
# ---------------------------------------------------------------------------

@dataclass
class FactEntry:
    """A single fact in Working Memory."""

    owner: FactOwner
    key: str
    value: FactValue
    enabled: bool = True
    timestamp: datetime = field(default_factory=datetime.now)

    # ------------------------------------------------------------------
    # Properties
    # ------------------------------------------------------------------

    @property
    def compound_key(self) -> str:
        """Canonical dict key: "{owner}:{key}"."""
        return f"{self.owner.value}:{self.key}"

    @property
    def owner_icon(self) -> str:
        return self.owner.icon

    # ------------------------------------------------------------------
    # Serialisation
    # ------------------------------------------------------------------

    def to_dict(self) -> dict:
        return {
            "owner": self.owner.value,
            "key": self.key,
            "value": self.value,
            "enabled": self.enabled,
            "timestamp": self.timestamp.isoformat(),
        }

    @classmethod
    def from_dict(cls, data: dict) -> "FactEntry":
        return cls(
            owner=FactOwner(data["owner"]),
            key=data["key"],
            value=data["value"],
            enabled=data.get("enabled", True),
            timestamp=datetime.fromisoformat(data["timestamp"]) if "timestamp" in data else datetime.now(),
        )

    # ------------------------------------------------------------------
    # Display helpers
    # ------------------------------------------------------------------

    def value_preview(self, max_len: int = 60) -> str:
        """Short displayable representation of the value."""
        raw = json.dumps(self.value) if not isinstance(self.value, str) else self.value
        return raw if len(raw) <= max_len else raw[:max_len - 1] + "…"

    def to_llm_line(self) -> str:
        """Single line for the LLM context block."""
        val = json.dumps(self.value) if not isinstance(self.value, str) else self.value
        return f"{self.owner_icon} {self.key}: {val}"


# ---------------------------------------------------------------------------
# WorkingMemory
# ---------------------------------------------------------------------------

class WorkingMemory:
    """
    Per-session key-value fact store.

    Internally: dict[compound_key, FactEntry]
    compound_key = "{owner}:{key}"

    Ownership rules enforced here (not in tools — tools call these methods):
      - Only user-facing code (GUI, explicit user instruction) may create user-owned facts.
      - Agent tools may only write agent-owned facts.
      - Promotion (agent → user) is a destructive key rename performed atomically.
    """

    PANEL_ICON = "🏛️"
    PANEL_LABEL = "Working Memory"

    def __init__(self) -> None:
        self._facts: dict[str, FactEntry] = {}
        self._path: Optional[Path] = None  # Set by session after load

    # ------------------------------------------------------------------
    # CRUD
    # ------------------------------------------------------------------

    def add_fact(self, owner: FactOwner, key: str, value: FactValue, *, enabled: bool = True) -> FactEntry:
        """Add or update a fact. Returns the stored FactEntry."""
        entry = FactEntry(owner=owner, key=key, value=value, enabled=enabled)
        self._facts[entry.compound_key] = entry
        self._autosave()
        return entry

    def remove_fact(self, compound_key: str) -> bool:
        """Remove a fact by compound key. Returns True if it existed."""
        existed = compound_key in self._facts
        if existed:
            del self._facts[compound_key]
            self._autosave()
        return existed

    def set_enabled(self, compound_key: str, enabled: bool) -> bool:
        """Toggle enabled state for a fact. Returns True if the fact was found."""
        if compound_key not in self._facts:
            return False
        self._facts[compound_key].enabled = enabled
        self._autosave()
        return True

    def get(self, compound_key: str) -> Optional[FactEntry]:
        return self._facts.get(compound_key)

    def get_enabled_facts(self) -> list[FactEntry]:
        """Return all enabled facts, user-owned first, then agent-owned, alpha within each group."""
        user_facts = sorted(
            (f for f in self._facts.values() if f.enabled and f.owner == FactOwner.USER),
            key=lambda f: f.key,
        )
        agent_facts = sorted(
            (f for f in self._facts.values() if f.enabled and f.owner == FactOwner.AGENT),
            key=lambda f: f.key,
        )
        return user_facts + agent_facts

    def all_facts(self) -> list[FactEntry]:
        """All facts regardless of enabled state, user-owned first."""
        user_facts = sorted(
            (f for f in self._facts.values() if f.owner == FactOwner.USER), key=lambda f: f.key
        )
        agent_facts = sorted(
            (f for f in self._facts.values() if f.owner == FactOwner.AGENT), key=lambda f: f.key
        )
        return user_facts + agent_facts

    def __len__(self) -> int:
        return len(self._facts)

    def __bool__(self) -> bool:
        return bool(self._facts)

    # ------------------------------------------------------------------
    # Promote agent-owned → user-owned
    # ------------------------------------------------------------------

    def promote_to_user(
        self,
        compound_key: str,
        new_key: Optional[str] = None,
        force: bool = False,
    ) -> PromotionResult:
        """
        Promote an agent-owned fact to user-owned (in-place key rename).

        Args:
            compound_key: Must be an "agent:{key}" entry.
            new_key:       Override the key name for the promoted fact (optional).
            force:         If True, overwrite any existing user-owned fact with the
                           same target key instead of returning CONFLICT.

        Returns:
            PromotionResult — check .status for OK / CONFLICT / NOT_FOUND / NOT_AGENT_OWNED.
        """
        entry = self._facts.get(compound_key)
        if entry is None:
            return PromotionResult(PromotionStatus.NOT_FOUND)
        if entry.owner != FactOwner.AGENT:
            return PromotionResult(PromotionStatus.NOT_AGENT_OWNED)

        target_key = new_key or entry.key
        target_compound = f"{FactOwner.USER.value}:{target_key}"

        if target_compound in self._facts and not force:
            return PromotionResult(
                PromotionStatus.CONFLICT,
                conflicting_value=self._facts[target_compound].value,
            )

        # Atomic rename: remove agent entry, insert user entry
        del self._facts[compound_key]
        promoted = FactEntry(
            owner=FactOwner.USER,
            key=target_key,
            value=entry.value,
            enabled=entry.enabled,
            timestamp=entry.timestamp,
        )
        self._facts[target_compound] = promoted
        self._autosave()
        return PromotionResult(PromotionStatus.OK)

    # ------------------------------------------------------------------
    # LLM context injection
    # ------------------------------------------------------------------

    def to_llm_block(self) -> str:
        """
        Format enabled facts as a structured block for LLM system context injection.

        Returns empty string if no enabled facts exist (caller should skip injection).
        """
        enabled = self.get_enabled_facts()
        if not enabled:
            return ""
        lines = [f"{self.PANEL_ICON} [WORKING MEMORY]"]
        for fact in enabled:
            lines.append(f"  {fact.to_llm_line()}")
        lines.append("[END WORKING MEMORY]")
        return "\n".join(lines)

    # ------------------------------------------------------------------
    # Persistence
    # ------------------------------------------------------------------

    def save(self, path: Path | str) -> None:
        """Persist to {path}/working_memory.json."""
        target = Path(path) / "working_memory.json"
        target.parent.mkdir(parents=True, exist_ok=True)
        data = {ck: entry.to_dict() for ck, entry in self._facts.items()}
        target.write_text(json.dumps(data, indent=2, ensure_ascii=False), encoding="utf-8")

    @classmethod
    def load(cls, path: Path | str) -> "WorkingMemory":
        """Load from {path}/working_memory.json. Returns empty WorkingMemory if absent."""
        wm = cls()
        target = Path(path) / "working_memory.json"
        if not target.exists():
            return wm
        try:
            raw = json.loads(target.read_text(encoding="utf-8"))
            for ck, entry_data in raw.items():
                entry = FactEntry.from_dict(entry_data)
                wm._facts[entry.compound_key] = entry
        except Exception:
            # Corrupt file — start fresh; do not crash the session
            pass
        return wm

    def set_path(self, path: Path | str) -> None:
        """Bind a session path so that mutations auto-save."""
        self._path = Path(path)

    def _autosave(self) -> None:
        if self._path is not None:
            self.save(self._path)

    # ------------------------------------------------------------------
    # Dict-like serialisation (for testing / inspection)
    # ------------------------------------------------------------------

    def to_dict(self) -> dict:
        return {ck: entry.to_dict() for ck, entry in self._facts.items()}

    @classmethod
    def from_dict(cls, data: dict) -> "WorkingMemory":
        wm = cls()
        for entry_data in data.values():
            entry = FactEntry.from_dict(entry_data)
            wm._facts[entry.compound_key] = entry
        return wm
