"""Integration tests for tool registry pipeline.

GIVEN AgentX initialized with a dynamic tool registry
WHEN tools are toggled, registered, or reloaded
THEN the bridge is updated and UI reflects the current state.
"""

import json
import pytest
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

from src.agentx.integration.tool_registry_manager import ToolRegistryManager


@pytest.fixture
def temp_tools_config(tmp_path):
    """Create a temporary agentx_tools.toml for testing."""
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


class TestToolRegistryBridgeIntegration:
    """Test tool registry integration with bridge."""

    def test_registry_callback_triggers_on_toggle(self, temp_tools_config):
        """
        GIVEN a registry manager with on_registry_change callback
        WHEN toggling a tool
        THEN the callback is invoked to update bridge.
        """
        callback = MagicMock()
        manager = ToolRegistryManager(str(temp_tools_config), on_registry_change=callback)

        manager.toggle_tool("ast", enabled=True)

        assert callback.called

    def test_enabled_tools_list_updated_after_toggle(self, temp_tools_config):
        """
        GIVEN initial enabled tools [cst, read_file]
        WHEN toggling ast to enabled
        THEN get_enabled_tool_names() returns [cst, read_file, ast].
        """
        manager = ToolRegistryManager(str(temp_tools_config))

        initial = manager.get_enabled_tool_names()
        assert "ast" not in initial

        manager.toggle_tool("ast", enabled=True)

        updated = manager.get_enabled_tool_names()
        assert "ast" in updated

    def test_builtin_reload_tools_resets_state(self, temp_tools_config):
        """
        GIVEN a registry with toggled state and registered tools
        WHEN calling builtin_reload_tools()
        THEN state is reset to config defaults.
        """
        manager = ToolRegistryManager(str(temp_tools_config))

        # Modify state
        manager.toggle_tool("ast", enabled=True)
        manager.registry.register_tool("dynamic_tool")

        # Verify modifications
        assert "dynamic_tool" in manager.registry.tools
        assert manager.registry.enabled_state["ast"] is True

        # Reload
        result = manager.builtin_reload_tools()

        # Verify reset
        assert "dynamic_tool" not in manager.registry.tools
        assert manager.registry.enabled_state["ast"] is False

        # Verify JSON response
        data = json.loads(result)
        assert data["status"] == "success"

    def test_builtin_register_tool_adds_to_registry(self, temp_tools_config):
        """
        GIVEN a registry
        WHEN calling builtin_register_tool()
        THEN the tool is added and available in get_available_tools().
        """
        manager = ToolRegistryManager(str(temp_tools_config))

        initial_count = len(manager.get_available_tools())

        result = manager.builtin_register_tool(
            "analyze_code",
            description="Analyze code for issues",
            category="analysis",
        )

        data = json.loads(result)
        assert data["status"] == "success"

        updated_count = len(manager.get_available_tools())
        assert updated_count == initial_count + 1

        tools = manager.get_available_tools()
        analyze_tool = next((t for t in tools if t["name"] == "analyze_code"), None)
        assert analyze_tool is not None
        assert analyze_tool["description"] == "Analyze code for issues"

    def test_tool_enabled_status_in_ui_export(self, temp_tools_config):
        """
        GIVEN a registry with mixed enabled/disabled tools
        WHEN exporting for UI via get_available_tools()
        THEN each tool includes its current enabled status.
        """
        manager = ToolRegistryManager(str(temp_tools_config))
        manager.toggle_tool("ast", enabled=True)

        tools = manager.get_available_tools()

        cst = next(t for t in tools if t["name"] == "cst")
        ast = next(t for t in tools if t["name"] == "ast")
        read_file = next(t for t in tools if t["name"] == "read_file")

        assert cst["enabled"] is True
        assert ast["enabled"] is True
        assert read_file["enabled"] is True

        # Disable one
        manager.toggle_tool("read_file", enabled=False)
        tools = manager.get_available_tools()

        read_file = next(t for t in tools if t["name"] == "read_file")
        assert read_file["enabled"] is False

    def test_consecutive_operations_maintain_state(self, temp_tools_config):
        """
        GIVEN a registry
        WHEN performing multiple toggle and register operations
        THEN state is correctly maintained across all operations.
        """
        manager = ToolRegistryManager(str(temp_tools_config))

        # Operation 1: Toggle ast to enabled
        manager.toggle_tool("ast", enabled=True)
        enabled = manager.get_enabled_tool_names()
        assert "ast" in enabled

        # Operation 2: Register new tool
        manager.builtin_register_tool("custom_tool", enabled=False)
        assert "custom_tool" in [t["name"] for t in manager.get_available_tools()]

        # Operation 3: Verify custom tool is disabled
        custom = next(t for t in manager.get_available_tools() if t["name"] == "custom_tool")
        assert custom["enabled"] is False

        # Operation 4: Toggle new tool to enabled
        manager.toggle_tool("custom_tool", enabled=True)
        enabled = manager.get_enabled_tool_names()
        assert "custom_tool" in enabled

        # Operation 5: Reload (should reset toggles to config defaults)
        manager.builtin_reload_tools()
        enabled = manager.get_enabled_tool_names()
        assert "ast" not in enabled  # Back to disabled

        # Registered tools now persist to config and remain available after reload.
        assert "custom_tool" in [t["name"] for t in manager.get_available_tools()]
        custom = next(t for t in manager.get_available_tools() if t["name"] == "custom_tool")
        assert custom["enabled"] is False
