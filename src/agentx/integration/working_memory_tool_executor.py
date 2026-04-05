"""
Working Memory tool executor for AgentX.

Provides remember_fact, forget_fact, and list_facts tools that the agent
can invoke via the tool-call mechanism. Ownership enforcement is applied here:
- Agent tools may only create/update/remove agent-owned facts.
- Attempting to overwrite a user-owned key raises a clear error string.

Usage:
    executor = WorkingMemoryToolExecutor(working_memory)
    impls = executor.get_tool_implementations()
    schemas = get_working_memory_tool_schemas()
"""

import json
import logging
from typing import Callable

logger = logging.getLogger(__name__)

from shared.models.working_memory import FactEntry, FactOwner, FactValue, WorkingMemory


class WorkingMemoryToolExecutor:
    """
    Wraps a WorkingMemory instance and exposes it as agent-callable tools.

    The executor holds a reference to the session's live WorkingMemory so that
    mutations are immediately reflected in the GUI and persisted to disk.
    """

    def __init__(self, working_memory: WorkingMemory) -> None:
        self._wm = working_memory

    # ------------------------------------------------------------------
    # Tool implementations
    # ------------------------------------------------------------------

    def remember_fact(self, key: str, value: str) -> str:
        """
        Store or update an agent-owned fact in Working Memory.

        Adds a new fact or updates an existing agent-owned fact with the given key.
        Agent tools cannot overwrite user-owned facts — use list_facts to check what
        the user has already defined.

        Args:
            key: The fact identifier (e.g. "current_task", "detected_language").
                 Must be a simple string with no colons.
            value: The value to store. May be a plain string, a number, a JSON array,
                   or a JSON object (pass as JSON-encoded string for complex types).

        Returns:
            Confirmation string on success, or error description on failure.
        """
        if ":" in key:
            return f"Error: key must not contain ':' (got '{key}')"

        # Guard: do not overwrite user-owned facts
        user_key = f"{FactOwner.USER.value}:{key}"
        if self._wm.get(user_key) is not None:
            return (
                f"Error: '{key}' is a user-owned fact and cannot be overwritten by the agent. "
                f"Current value: {self._wm.get(user_key).value_preview()}"
            )

        # Parse JSON-encoded complex values if possible
        parsed_value: FactValue = value
        try:
            candidate = json.loads(value)
            if isinstance(candidate, (dict, list, int, float)):
                parsed_value = candidate
        except (json.JSONDecodeError, ValueError):
            pass

        self._wm.add_fact(FactOwner.AGENT, key, parsed_value)
        return f"✓ Stored: 🤖 {key} = {json.dumps(parsed_value) if not isinstance(parsed_value, str) else parsed_value}"

    def forget_fact(self, key: str) -> str:
        """
        Remove an agent-owned fact from Working Memory.

        Only agent-owned facts can be removed by this tool. User-owned facts
        are protected and must be managed by the user through the GUI.

        Args:
            key: The fact identifier to remove (without owner prefix).

        Returns:
            Confirmation string on success, or error description on failure.
        """
        if ":" in key:
            return f"Error: key must not contain ':' (got '{key}')"

        agent_key = f"{FactOwner.AGENT.value}:{key}"
        user_key = f"{FactOwner.USER.value}:{key}"

        if self._wm.get(user_key) is not None:
            return f"Error: '{key}' is a user-owned fact and cannot be removed by the agent."

        if self._wm.remove_fact(agent_key):
            return f"✓ Removed agent fact: {key}"
        return f"Error: no agent-owned fact found with key '{key}'"

    def list_facts(self) -> str:
        """
        List all facts currently stored in Working Memory.

        Returns all facts (both user-owned and agent-owned, both enabled and
        disabled) as a JSON object. Each entry includes the owner, value,
        and enabled status.

        Returns:
            JSON-encoded summary of all Working Memory facts.
        """
        all_facts = self._wm.all_facts()
        if not all_facts:
            return json.dumps({"facts": [], "total": 0})

        result = {
            "facts": [
                {
                    "compound_key": f.compound_key,
                    "owner": f.owner.value,
                    "key": f.key,
                    "value": f.value,
                    "enabled": f.enabled,
                }
                for f in all_facts
            ],
            "total": len(all_facts),
        }
        return json.dumps(result, indent=2, ensure_ascii=False)

    # ------------------------------------------------------------------
    # Registration helpers
    # ------------------------------------------------------------------

    def get_tool_implementations(self) -> dict[str, Callable]:
        """Return name → callable mapping for bridge registration."""
        return {
            "remember_fact": self.remember_fact,
            "forget_fact": self.forget_fact,
            "list_facts": self.list_facts,
        }


# ---------------------------------------------------------------------------
# Schema generation (module-level, no instance needed)
# ---------------------------------------------------------------------------


def get_working_memory_tool_schemas() -> list[dict]:
    """Return OpenAI-format tool schemas for Working Memory tools."""
    from agentix.tools.schema import SchemaGenerationError, extract_tool_schema

    # Create a temporary executor against a dummy WorkingMemory to extract schemas
    _tmp = WorkingMemoryToolExecutor(WorkingMemory())
    schemas = []
    for fn in _tmp.get_tool_implementations().values():
        try:
            schemas.append(extract_tool_schema(fn))
        except SchemaGenerationError as exc:
            logger.error("Schema generation failed for %s: %s", fn.__name__, exc)
    return schemas
