"""
Tests for Phase 6 — Client Tool Integration.

Covers:
- Standalone wrapper functions (schema generation + execution)
- get_client_tool_schemas() produces valid OpenAI tool schemas
- AgentixBridge.register_tool_implementations() / get_available_tools()
- Client tools are callable from bridge.execute_tool()
"""

import os
import json
import tempfile
import pytest
from unittest.mock import patch, MagicMock

# ---------------------------------------------------------------------------
# Standalone wrapper function tests
# ---------------------------------------------------------------------------


class TestClientToolFunctions:
    def test_read_file_returns_contents(self, tmp_path):
        from agentx.integration.client_tool_executor import read_file

        f = tmp_path / "hello.txt"
        f.write_text("hello world")
        result = read_file(str(f))
        assert result == "hello world"

    def test_write_file_creates_file(self, tmp_path):
        from agentx.integration.client_tool_executor import write_file

        target = tmp_path / "out.txt"
        result = write_file(str(target), "test content")
        assert target.read_text() == "test content"
        assert "Wrote to" in result

    def test_write_file_append(self, tmp_path):
        from agentx.integration.client_tool_executor import write_file

        target = tmp_path / "out.txt"
        write_file(str(target), "line1\n")
        write_file(str(target), "line2\n", append=True)
        assert target.read_text() == "line1\nline2\n"

    def test_list_directory_returns_files(self, tmp_path):
        from agentx.integration.client_tool_executor import list_directory

        (tmp_path / "a.py").write_text("")
        (tmp_path / "b.txt").write_text("")
        result = list_directory(str(tmp_path))
        assert "a.py" in result
        assert "b.txt" in result

    def test_list_directory_pattern(self, tmp_path):
        from agentx.integration.client_tool_executor import list_directory

        (tmp_path / "a.py").write_text("")
        (tmp_path / "b.txt").write_text("")
        result = list_directory(str(tmp_path), pattern="*.py")
        assert "a.py" in result
        assert "b.txt" not in result

    def test_get_file_info_returns_json(self, tmp_path):
        from agentx.integration.client_tool_executor import get_file_info

        f = tmp_path / "info.txt"
        f.write_text("data")
        result = get_file_info(str(f))
        info = json.loads(result)
        assert info["is_file"] is True
        assert info["size_bytes"] == 4

    def test_search_files_finds_matches(self, tmp_path):
        from agentx.integration.client_tool_executor import search_files

        (tmp_path / "main.py").write_text("")
        (tmp_path / "test.py").write_text("")
        (tmp_path / "readme.md").write_text("")
        result = search_files(str(tmp_path), "*.py")
        assert "main.py" in result
        assert "test.py" in result
        assert "readme.md" not in result

    def test_read_file_missing_returns_error_string(self, tmp_path):
        from agentx.integration.client_tool_executor import read_file

        result = read_file(str(tmp_path / "does_not_exist.txt"))
        assert "Error" in result or "not found" in result.lower()


# ---------------------------------------------------------------------------
# Schema generation
# ---------------------------------------------------------------------------


class TestClientToolSchemas:
    def setup_method(self):
        from agentx.integration.client_tool_executor import get_client_tool_schemas

        self.schemas = get_client_tool_schemas()

    def test_returns_six_schemas(self):
        assert len(self.schemas) == 6

    def test_all_are_function_type(self):
        for s in self.schemas:
            assert s["type"] == "function"

    def test_tool_names_present(self):
        names = {s["function"]["name"] for s in self.schemas}
        assert names == {"read_file", "write_file", "list_directory", "get_file_info", "search_files", "grep_files"}

    def test_read_file_has_path_required(self):
        schema = next(s for s in self.schemas if s["function"]["name"] == "read_file")
        fn = schema["function"]
        assert "path" in fn["parameters"]["required"]
        assert "encoding" not in fn["parameters"].get("required", [])

    def test_write_file_has_path_and_content_required(self):
        schema = next(s for s in self.schemas if s["function"]["name"] == "write_file")
        fn = schema["function"]
        assert "path" in fn["parameters"]["required"]
        assert "content" in fn["parameters"]["required"]

    def test_search_files_has_path_and_pattern_required(self):
        schema = next(s for s in self.schemas if s["function"]["name"] == "search_files")
        fn = schema["function"]
        assert "path" in fn["parameters"]["required"]
        assert "pattern" in fn["parameters"]["required"]


# ---------------------------------------------------------------------------
# Bridge registration
# ---------------------------------------------------------------------------


class TestBridgeClientToolRegistration:
    def _make_bridge(self):
        from agentix.bridge.bridge import AgentixBridge
        from agentix.agentix_config import AgentixConfig

        config = AgentixConfig(model="llama3.2", tools=[], debug=False)
        return AgentixBridge(config)

    def test_register_tool_implementations_adds_to_cache(self):
        bridge = self._make_bridge()
        bridge.register_tool_implementations({"my_tool": lambda x: x})
        assert "my_tool" in bridge._tool_impl_cache

    def test_register_tool_schemas_appear_in_get_available_tools(self):
        bridge = self._make_bridge()
        fake_schema = {"type": "function", "function": {"name": "my_tool", "description": "test", "parameters": {}}}
        bridge.register_tool_implementations({"my_tool": lambda: "ok"}, schemas=[fake_schema])
        tools = bridge.get_available_tools()
        names = [t["function"]["name"] for t in tools]
        assert "my_tool" in names

    def test_client_tool_schemas_in_get_available_tools_after_registration(self):
        from agentx.integration.client_tool_executor import (
            get_client_tool_implementations,
            get_client_tool_schemas,
        )

        bridge = self._make_bridge()
        bridge.register_tool_implementations(
            get_client_tool_implementations(),
            get_client_tool_schemas(),
        )
        tools = bridge.get_available_tools()
        names = {t["function"]["name"] for t in tools}
        assert "read_file" in names
        assert "write_file" in names
        assert "list_directory" in names

    def test_execute_tool_dispatches_to_registered_client_tool(self, tmp_path):
        from agentx.integration.client_tool_executor import (
            get_client_tool_implementations,
            get_client_tool_schemas,
        )

        bridge = self._make_bridge()
        bridge.register_tool_implementations(
            get_client_tool_implementations(),
            get_client_tool_schemas(),
        )
        # write a file then read it back via bridge.execute_tool
        test_file = tmp_path / "test.txt"
        test_file.write_text("bridge test")

        result = bridge.execute_tool("read_file", {"path": str(test_file)})
        assert result.success
        assert "bridge test" in str(result.output)

    def test_execute_unknown_tool_returns_error(self):
        bridge = self._make_bridge()
        result = bridge.execute_tool("nonexistent_tool", {})
        assert not result.success
        assert "nonexistent_tool" in result.error


# ---------------------------------------------------------------------------
# Adapter auto-registration
# ---------------------------------------------------------------------------


class TestAdapterRegistersClientTools:
    def test_adapter_registers_client_tools_on_init(self):
        """AgentixBridgeAdapter.__init__ should register client tools without error."""
        config = {
            "agentx": {"ollama_model": "llama3.2", "ollama_host": "localhost:11434"},
            "agentix": {"classify_prompts": False},
        }
        from agentx.integration.agentix_bridge_adapter import AgentixBridgeAdapter

        adapter = AgentixBridgeAdapter(config)
        tools = adapter.bridge.get_available_tools()
        names = {t["function"]["name"] for t in tools}
        assert "read_file" in names
        assert "search_files" in names

    def test_adapter_client_tools_are_executable(self, tmp_path):
        config = {
            "agentx": {"ollama_model": "llama3.2", "ollama_host": "localhost:11434"},
            "agentix": {"classify_prompts": False},
        }
        from agentx.integration.agentix_bridge_adapter import AgentixBridgeAdapter

        adapter = AgentixBridgeAdapter(config)
        test_file = tmp_path / "exec_test.txt"
        test_file.write_text("adapter exec test")
        result = adapter.bridge.execute_tool("read_file", {"path": str(test_file)})
        assert result.success
        assert "adapter exec test" in str(result.output)
