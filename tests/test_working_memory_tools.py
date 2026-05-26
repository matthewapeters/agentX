"""
Tests for WorkingMemoryToolExecutor — ownership enforcement, CRUD via tool API,
schema generation, and JSON output format.
"""

import json
import sys
import unittest
from pathlib import Path

# Ensure src/ is on path
src_path = str(Path(__file__).parent.parent / "src")
if src_path not in sys.path:
    sys.path.insert(0, src_path)

from agentx.integration.working_memory_tool_executor import (
    WorkingMemoryToolExecutor,
    get_working_memory_tool_schemas,
)
from shared.models.working_memory import FactOwner, WorkingMemory


class TestRememberFact(unittest.TestCase):
    def setUp(self):
        self.wm = WorkingMemory()
        self.ex = WorkingMemoryToolExecutor(self.wm)

    def test_store_new_string_fact(self):
        result = self.ex.remember_fact("project", "AgentX")
        self.assertIn("project", result)
        entry = self.wm.get("agent:project")
        self.assertIsNotNone(entry)
        self.assertEqual(entry.value, "AgentX")
        self.assertEqual(entry.owner, FactOwner.AGENT)

    def test_overwrite_existing_agent_fact(self):
        self.ex.remember_fact("lang", "Python")
        self.ex.remember_fact("lang", "Rust")
        entry = self.wm.get("agent:lang")
        self.assertEqual(entry.value, "Rust")

    def test_rejects_user_owned_key(self):
        self.wm.add_fact(FactOwner.USER, "project", "Important")
        result = self.ex.remember_fact("project", "SomethingElse")
        self.assertIn("Error", result)
        self.assertIn("user-owned", result)
        # User fact is unchanged
        self.assertEqual(self.wm.get("user:project").value, "Important")

    def test_rejects_key_with_colon(self):
        result = self.ex.remember_fact("bad:key", "value")
        self.assertIn("Error", result)

    def test_parses_json_list(self):
        result = self.ex.remember_fact("tags", '["a", "b", "c"]')
        self.assertIn("tags", result)
        entry = self.wm.get("agent:tags")
        self.assertEqual(entry.value, ["a", "b", "c"])

    def test_parses_json_dict(self):
        self.ex.remember_fact("meta", '{"version": 1}')
        entry = self.wm.get("agent:meta")
        self.assertEqual(entry.value, {"version": 1})

    def test_keeps_plain_string_when_not_json(self):
        self.ex.remember_fact("note", "plain text here")
        entry = self.wm.get("agent:note")
        self.assertIsInstance(entry.value, str)
        self.assertEqual(entry.value, "plain text here")

    def test_return_string_contains_checkmark(self):
        result = self.ex.remember_fact("x", "y")
        self.assertIn("✓", result)


class TestForgetFact(unittest.TestCase):
    def setUp(self):
        self.wm = WorkingMemory()
        self.ex = WorkingMemoryToolExecutor(self.wm)

    def test_removes_agent_fact(self):
        self.wm.add_fact(FactOwner.AGENT, "task", "build feature")
        result = self.ex.forget_fact("task")
        self.assertIn("✓", result)
        self.assertIsNone(self.wm.get("agent:task"))

    def test_rejects_user_owned_fact(self):
        self.wm.add_fact(FactOwner.USER, "project", "MyProject")
        result = self.ex.forget_fact("project")
        self.assertIn("Error", result)
        self.assertIn("user-owned", result)
        self.assertIsNotNone(self.wm.get("user:project"))

    def test_missing_key_returns_error(self):
        result = self.ex.forget_fact("nonexistent")
        self.assertIn("Error", result)
        self.assertIn("nonexistent", result)

    def test_rejects_key_with_colon(self):
        result = self.ex.forget_fact("bad:key")
        self.assertIn("Error", result)


class TestListFacts(unittest.TestCase):
    def setUp(self):
        self.wm = WorkingMemory()
        self.ex = WorkingMemoryToolExecutor(self.wm)

    def test_empty_returns_json_with_zero_total(self):
        result = self.ex.list_facts()
        data = json.loads(result)
        self.assertEqual(data["total"], 0)
        self.assertEqual(data["facts"], [])

    def test_returns_all_facts(self):
        self.wm.add_fact(FactOwner.USER, "name", "Alice")
        self.wm.add_fact(FactOwner.AGENT, "task", "build")
        result = self.ex.list_facts()
        data = json.loads(result)
        self.assertEqual(data["total"], 2)
        keys = {f["compound_key"] for f in data["facts"]}
        self.assertIn("user:name", keys)
        self.assertIn("agent:task", keys)

    def test_includes_disabled_facts(self):
        self.wm.add_fact(FactOwner.AGENT, "hidden", "value")
        self.wm.set_enabled("agent:hidden", False)
        result = self.ex.list_facts()
        data = json.loads(result)
        fact = next(f for f in data["facts"] if f["compound_key"] == "agent:hidden")
        self.assertFalse(fact["enabled"])

    def test_fact_entries_have_required_fields(self):
        self.wm.add_fact(FactOwner.AGENT, "k", "v")
        result = self.ex.list_facts()
        data = json.loads(result)
        fact = data["facts"][0]
        for field in ("compound_key", "owner", "key", "value", "enabled"):
            self.assertIn(field, fact)


class TestSchemaGeneration(unittest.TestCase):
    def test_schemas_generated(self):
        schemas = get_working_memory_tool_schemas()
        self.assertIsInstance(schemas, list)
        self.assertGreater(len(schemas), 0)

    def test_schema_names(self):
        schemas = get_working_memory_tool_schemas()
        names = {s["function"]["name"] for s in schemas}
        self.assertIn("remember_fact", names)
        self.assertIn("forget_fact", names)
        self.assertIn("list_facts", names)

    def test_remember_fact_schema_has_parameters(self):
        schemas = get_working_memory_tool_schemas()
        schema = next(s for s in schemas if s["function"]["name"] == "remember_fact")
        params = schema["function"]["parameters"]["properties"]
        self.assertIn("key", params)
        self.assertIn("value", params)

    def test_forget_fact_schema_has_key_param(self):
        schemas = get_working_memory_tool_schemas()
        schema = next(s for s in schemas if s["function"]["name"] == "forget_fact")
        params = schema["function"]["parameters"]["properties"]
        self.assertIn("key", params)

    def test_list_facts_schema_no_required_params(self):
        schemas = get_working_memory_tool_schemas()
        schema = next(s for s in schemas if s["function"]["name"] == "list_facts")
        required = schema["function"]["parameters"].get("required", [])
        self.assertEqual(required, [])


if __name__ == "__main__":
    unittest.main()
