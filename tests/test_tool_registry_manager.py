"""Unit tests for ToolRegistryManager.

GIVEN a ToolRegistryManager with an underlying ToolRegistry
WHEN calling manager methods or built-in tools
THEN the manager correctly delegates to registry and invokes callbacks.
"""

import json
import pytest
from pathlib import Path
from unittest.mock import MagicMock, patch

from src.agentx.integration.tool_registry_manager import ToolRegistryManager


@pytest.fixture
def temp_tools_config(tmp_path):
    """Create a temporary agentx_tools.toml for testing.

    Returns:
        Path to temporary tools config file.
    """
    config_file = tmp_path / "agentx_tools.toml"
    config_content = """
[tools.cst]
description = "Concrete Syntax Tree analysis"
category = "code-analysis"
enabled = true

[tools.ast]
description = "Abstract Syntax Tree analysis"
category = "code-analysis"
enabled = false

[tools.read_file]
description = "Read file contents"
category = "filesystem"
enabled = true
"""
    config_file.write_text(config_content)
    return config_file


class TestToolRegistryManagerInit:
    """Test ToolRegistryManager initialization."""

    def test_init_with_config_path(self, temp_tools_config):
        """
        GIVEN a valid config path
        WHEN creating a ToolRegistryManager
        THEN the registry is initialized and tools are loaded.
        """
        manager = ToolRegistryManager(str(temp_tools_config))

        assert manager.registry is not None
        assert len(manager.registry.tools) == 3

    def test_init_with_callback(self, temp_tools_config):
        """
        GIVEN a callback function
        WHEN creating a ToolRegistryManager
        THEN the callback is stored and can be invoked on changes.
        """
        callback = MagicMock()
        manager = ToolRegistryManager(str(temp_tools_config), on_registry_change=callback)

        assert manager.on_registry_change == callback
        assert not callback.called


class TestGetTools:
    """Test tool retrieval methods."""

    def test_get_available_tools(self, temp_tools_config):
        """
        GIVEN a manager with mixed enabled/disabled tools
        WHEN calling get_available_tools()
        THEN all tools are returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        tools = manager.get_available_tools()

        assert len(tools) == 3
        names = {t["name"] for t in tools}
        assert names == {"cst", "ast", "read_file"}

    def test_get_enabled_tools(self, temp_tools_config):
        """
        GIVEN a manager with mixed tool states
        WHEN calling get_enabled_tools()
        THEN only enabled tools are returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        tools = manager.get_enabled_tools()

        assert len(tools) == 2
        names = {t["name"] for t in tools}
        assert names == {"cst", "read_file"}

    def test_get_enabled_tool_names(self, temp_tools_config):
        """
        GIVEN a manager with mixed tool states
        WHEN calling get_enabled_tool_names()
        THEN list of enabled tool names is returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        names = manager.get_enabled_tool_names()

        assert names == ["cst", "read_file"]


class TestToggleTool:
    """Test toggling tools and callback invocation."""

    def test_toggle_tool_invokes_callback(self, temp_tools_config):
        """
        GIVEN a manager with an on_registry_change callback
        WHEN calling toggle_tool()
        THEN the callback is invoked.
        """
        callback = MagicMock()
        manager = ToolRegistryManager(str(temp_tools_config), on_registry_change=callback)

        manager.toggle_tool("cst", enabled=False)

        assert callback.called

    def test_toggle_tool_updates_state(self, temp_tools_config):
        """
        GIVEN a tool that is enabled
        WHEN calling toggle_tool() with enabled=False
        THEN the tool's state is updated and True is returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        assert manager.registry.enabled_state["cst"] is True

        result = manager.toggle_tool("cst", enabled=False)

        assert result is True
        assert manager.registry.enabled_state["cst"] is False

    def test_toggle_nonexistent_tool_returns_false(self, temp_tools_config):
        """
        GIVEN a tool name that doesn't exist
        WHEN calling toggle_tool()
        THEN False is returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        result = manager.toggle_tool("nonexistent", enabled=True)

        assert result is False


class TestBuiltinReloadTools:
    """Test built-in reload_tools implementation."""

    def test_reload_tools_returns_json(self, temp_tools_config):
        """
        GIVEN a manager
        WHEN calling builtin_reload_tools()
        THEN valid JSON with status and tools is returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        result = manager.builtin_reload_tools()

        data = json.loads(result)
        assert data["status"] == "success"
        assert data["tools_loaded"] == 3
        assert isinstance(data["tools"], list)

    def test_reload_tools_invokes_callback(self, temp_tools_config):
        """
        GIVEN a manager with on_registry_change callback
        WHEN calling builtin_reload_tools()
        THEN the callback is invoked.
        """
        callback = MagicMock()
        manager = ToolRegistryManager(str(temp_tools_config), on_registry_change=callback)

        manager.builtin_reload_tools()

        assert callback.called

    def test_reload_tools_resets_state(self, temp_tools_config):
        """
        GIVEN a manager with modified tool state
        WHEN calling builtin_reload_tools()
        THEN the state is reset to config defaults.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        manager.toggle_tool("ast", enabled=True)
        manager.registry.register_tool("dynamic_tool")

        assert manager.registry.enabled_state["ast"] is True
        assert "dynamic_tool" in manager.registry.tools

        manager.builtin_reload_tools()

        assert manager.registry.enabled_state["ast"] is False
        assert "dynamic_tool" not in manager.registry.tools


class TestBuiltinRegisterTool:
    """Test built-in register_tool implementation."""

    def test_register_tool_success(self, temp_tools_config):
        """
        GIVEN a manager with existing tools
        WHEN calling builtin_register_tool() with a new tool name
        THEN JSON with success status and tool list is returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        result = manager.builtin_register_tool("my_tool", description="Test tool", category="user")

        data = json.loads(result)
        assert data["status"] == "success"
        assert data["tool_registered"] == "my_tool"
        assert isinstance(data["tools"], list)

    def test_register_tool_duplicate_fails(self, temp_tools_config):
        """
        GIVEN a manager with existing tool 'cst'
        WHEN calling builtin_register_tool() with name 'cst'
        THEN JSON with error status is returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        result = manager.builtin_register_tool("cst", description="Duplicate")

        data = json.loads(result)
        assert data["status"] == "error"
        assert "already exists" in data["message"]

    def test_register_tool_missing_name_fails(self, temp_tools_config):
        """
        GIVEN a manager
        WHEN calling builtin_register_tool() with empty tool_name
        THEN JSON with error status is returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        result = manager.builtin_register_tool("", description="Test")

        data = json.loads(result)
        assert data["status"] == "error"

    def test_register_tool_invokes_callback(self, temp_tools_config):
        """
        GIVEN a manager with on_registry_change callback
        WHEN calling builtin_register_tool() successfully
        THEN the callback is invoked.
        """
        callback = MagicMock()
        manager = ToolRegistryManager(str(temp_tools_config), on_registry_change=callback)

        manager.builtin_register_tool("new_tool")

        assert callback.called

    def test_register_tool_with_defaults(self, temp_tools_config):
        """
        GIVEN optional parameters not provided
        WHEN calling builtin_register_tool() with only tool_name
        THEN the tool is registered with default values.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        manager.builtin_register_tool("minimal_tool")

        tool = manager.registry.tools["minimal_tool"]
        assert tool["description"] == ""
        assert tool["category"] == "user"
        assert manager.registry.enabled_state["minimal_tool"] is True

    def test_register_external_tool_persists_metadata(self, temp_tools_config):
        """
        GIVEN a user-created script and complete tool metadata
        WHEN calling builtin_register_tool()
        THEN metadata is persisted to TOML and visible after reload.
        """
        manager = ToolRegistryManager(str(temp_tools_config))

        result = manager.builtin_register_tool(
            "extract_todos",
            description="Extract TODO comments",
            category="analysis",
            enabled=True,
            scope="project",
            source_path="tools/extract_todos.py",
            runtime="python",
            entrypoint="main",
            input_schema={"type": "object", "required": ["path"]},
            output_schema={"type": "object", "required": ["todos"]},
        )

        data = json.loads(result)
        assert data["status"] == "success"

        reloaded = ToolRegistryManager(str(temp_tools_config))
        tool = reloaded.registry.tools["extract_todos"]
        assert tool["scope"] == "project"
        assert tool["source_path"] == "tools/extract_todos.py"
        assert tool["runtime"] == "python"
        assert tool["entrypoint"] == "main"
        assert tool["input_schema"]["required"] == ["path"]
        assert tool["output_schema"]["required"] == ["todos"]


class TestGetBuiltinToolImplementations:
    """Test retrieval of built-in tool implementations."""

    def test_get_builtin_implementations(self, temp_tools_config):
        """
        GIVEN a manager
        WHEN calling get_builtin_tool_implementations()
        THEN dictionary with reload_tools, register_tool, and diagnose_tools is returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        impls = manager.get_builtin_tool_implementations()

        assert "reload_tools" in impls
        assert "register_tool" in impls
        assert "diagnose_tools" in impls
        assert callable(impls["reload_tools"])
        assert callable(impls["register_tool"])
        assert callable(impls["diagnose_tools"])


class TestBuiltinDiagnoseTools:
    """Test built-in diagnose_tools implementation."""

    def test_diagnose_tools_requires_bridge(self, temp_tools_config):
        """
        GIVEN a manager without bridge
        WHEN calling builtin_diagnose_tools()
        THEN JSON with error status is returned.
        """
        manager = ToolRegistryManager(str(temp_tools_config))

        result = manager.builtin_diagnose_tools()
        data = json.loads(result)

        assert data["status"] == "error"
        assert "Bridge is not configured" in data["message"]

    def test_diagnose_tools_returns_report(self, temp_tools_config):
        """
        GIVEN a manager with bridge and registry tools
        WHEN calling builtin_diagnose_tools()
        THEN JSON report with phases is returned.
        """
        bridge = MagicMock()
        bridge.get_available_tools.return_value = [
            {"name": "read_file"},
            {"name": "write_file"},
            {"name": "diagnose_tools"},
        ]

        def execute_tool(tool_name, arguments):
            if tool_name == "write_file":
                return "ok"
            if tool_name == "read_file":
                return "AgentX tool diagnostic test"
            raise ValueError("unexpected tool")

        bridge.execute_tool.side_effect = execute_tool

        manager = ToolRegistryManager(str(temp_tools_config), bridge=bridge)
        result = manager.builtin_diagnose_tools()
        data = json.loads(result)

        assert "status" in data
        assert "phases" in data
        assert len(data["phases"]) == 4
