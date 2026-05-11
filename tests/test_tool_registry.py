"""Unit tests for ToolRegistry.

GIVEN a ToolRegistry with tools loaded from config
WHEN accessing tool data, toggling state, or registering new tools
THEN the registry correctly maintains tool definitions and enabled state.
"""

import json
import pytest
import tempfile
from pathlib import Path
from unittest.mock import patch, MagicMock

from src.agentx.tool_registry import ToolRegistry


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


class TestToolRegistryInit:
    """Test ToolRegistry initialization and config loading."""

    def test_load_from_config(self, temp_tools_config):
        """
        GIVEN a valid agentx_tools.toml file
        WHEN creating a ToolRegistry with that config
        THEN tools are loaded with correct names and enabled states.
        """
        registry = ToolRegistry(str(temp_tools_config))

        assert len(registry.tools) == 3
        assert "cst" in registry.tools
        assert "ast" in registry.tools
        assert "read_file" in registry.tools

        assert registry.enabled_state["cst"] is True
        assert registry.enabled_state["ast"] is False
        assert registry.enabled_state["read_file"] is True

    def test_load_missing_config_raises_error(self, tmp_path):
        """
        GIVEN a non-existent config file path
        WHEN creating a ToolRegistry
        THEN FileNotFoundError is raised.
        """
        missing_path = tmp_path / "nonexistent.toml"
        with pytest.raises(FileNotFoundError):
            ToolRegistry(str(missing_path))

    def test_load_invalid_toml_raises_error(self, tmp_path):
        """
        GIVEN a malformed TOML config file
        WHEN creating a ToolRegistry
        THEN ValueError is raised.
        """
        bad_config = tmp_path / "bad.toml"
        bad_config.write_text("[tools.cst\ndescription = incomplete")
        with pytest.raises(ValueError, match="Invalid TOML"):
            ToolRegistry(str(bad_config))


class TestGetTools:
    """Test retrieval of tool information."""

    def test_get_all_tools(self, temp_tools_config):
        """
        GIVEN a registry with mixed enabled/disabled tools
        WHEN calling get_all_tools()
        THEN all tools are returned with correct enabled state.
        """
        registry = ToolRegistry(str(temp_tools_config))
        all_tools = registry.get_all_tools()

        assert len(all_tools) == 3
        cst = next(t for t in all_tools if t["name"] == "cst")
        ast = next(t for t in all_tools if t["name"] == "ast")

        assert cst["enabled"] is True
        assert ast["enabled"] is False

    def test_get_enabled_tools(self, temp_tools_config):
        """
        GIVEN a registry with both enabled and disabled tools
        WHEN calling get_enabled_tools()
        THEN only enabled tools are returned.
        """
        registry = ToolRegistry(str(temp_tools_config))
        enabled = registry.get_enabled_tools()

        assert len(enabled) == 2
        names = {t["name"] for t in enabled}
        assert names == {"cst", "read_file"}

    def test_get_enabled_tool_names(self, temp_tools_config):
        """
        GIVEN a registry with mixed tool states
        WHEN calling get_enabled_tool_names()
        THEN list of enabled tool names is returned.
        """
        registry = ToolRegistry(str(temp_tools_config))
        names = registry.get_enabled_tool_names()

        assert names == ["cst", "read_file"]


class TestToggleTool:
    """Test enabling/disabling individual tools."""

    def test_toggle_enable_disabled_tool(self, temp_tools_config):
        """
        GIVEN a tool that is disabled
        WHEN calling toggle_tool() with enabled=True
        THEN the tool is enabled and True is returned.
        """
        registry = ToolRegistry(str(temp_tools_config))
        assert registry.enabled_state["ast"] is False

        result = registry.toggle_tool("ast", enabled=True)

        assert result is True
        assert registry.enabled_state["ast"] is True

    def test_toggle_disable_enabled_tool(self, temp_tools_config):
        """
        GIVEN a tool that is enabled
        WHEN calling toggle_tool() with enabled=False
        THEN the tool is disabled and True is returned.
        """
        registry = ToolRegistry(str(temp_tools_config))
        assert registry.enabled_state["cst"] is True

        result = registry.toggle_tool("cst", enabled=False)

        assert result is True
        assert registry.enabled_state["cst"] is False

    def test_toggle_nonexistent_tool_returns_false(self, temp_tools_config):
        """
        GIVEN a tool name that doesn't exist
        WHEN calling toggle_tool()
        THEN False is returned and state is unchanged.
        """
        registry = ToolRegistry(str(temp_tools_config))
        result = registry.toggle_tool("nonexistent", enabled=True)

        assert result is False


class TestRegisterTool:
    """Test dynamic tool registration."""

    def test_register_new_tool(self, temp_tools_config):
        """
        GIVEN a registry with existing tools
        WHEN calling register_tool() with new tool definition
        THEN the tool is added and True is returned.
        """
        registry = ToolRegistry(str(temp_tools_config))
        assert "custom_tool" not in registry.tools

        result = registry.register_tool(
            "custom_tool",
            description="My custom tool",
            category="user",
            enabled=True,
        )

        assert result is True
        assert "custom_tool" in registry.tools
        assert registry.tools["custom_tool"]["description"] == "My custom tool"
        assert registry.enabled_state["custom_tool"] is True

    def test_register_existing_tool_returns_false(self, temp_tools_config):
        """
        GIVEN a registry with a tool named 'cst'
        WHEN calling register_tool() with name 'cst'
        THEN False is returned and the original tool is unchanged.
        """
        registry = ToolRegistry(str(temp_tools_config))
        original_desc = registry.tools["cst"]["description"]

        result = registry.register_tool(
            "cst",
            description="Modified description",
            enabled=False,
        )

        assert result is False
        assert registry.tools["cst"]["description"] == original_desc
        assert registry.enabled_state["cst"] is True

    def test_register_tool_without_optional_fields(self, temp_tools_config):
        """
        GIVEN optional fields not provided
        WHEN calling register_tool()
        THEN defaults are used (empty description, 'user' category, enabled=True).
        """
        registry = ToolRegistry(str(temp_tools_config))

        registry.register_tool("minimal_tool")

        assert registry.tools["minimal_tool"]["description"] == ""
        assert registry.tools["minimal_tool"]["category"] == "user"
        assert registry.enabled_state["minimal_tool"] is True


class TestReloadTools:
    """Test reloading tool definitions from config."""

    def test_reload_tools_resets_state(self, temp_tools_config):
        """
        GIVEN a registry with toggled tool state and registered tools
        WHEN calling reload_tools()
        THEN state is reset to config defaults and registered tools are lost.
        """
        registry = ToolRegistry(str(temp_tools_config))

        # Toggle a tool and register a new one
        registry.toggle_tool("cst", enabled=False)
        registry.register_tool("dynamic_tool")

        assert "dynamic_tool" in registry.tools
        assert registry.enabled_state["cst"] is False

        # Reload
        registry.reload_tools()

        # Should be back to config defaults
        assert "dynamic_tool" not in registry.tools
        assert registry.enabled_state["cst"] is True

    def test_reload_with_modified_config_file(self, tmp_path):
        """
        GIVEN a config file that is modified on disk
        WHEN calling reload_tools()
        THEN the new tool definitions are loaded.
        """
        config_file = tmp_path / "tools.toml"
        config_file.write_text("[tools.tool1]\ndescription = 'First'\nenabled = true\n")

        registry = ToolRegistry(str(config_file))
        assert "tool1" in registry.tools
        assert "tool2" not in registry.tools

        # Modify config file
        config_file.write_text(
            "[tools.tool1]\ndescription = 'First'\nenabled = true\n"
            "[tools.tool2]\ndescription = 'Second'\nenabled = false\n"
        )

        registry.reload_tools()

        assert "tool1" in registry.tools
        assert "tool2" in registry.tools
        assert registry.enabled_state["tool2"] is False


class TestSaveEnabledState:
    """Test persistence of enabled/disabled state."""

    def test_save_enabled_state(self, temp_tools_config, tmp_path):
        """
        GIVEN a registry with modified tool states
        WHEN calling save_enabled_state()
        THEN current enabled state is saved to file in TOML format.
        """
        registry = ToolRegistry(str(temp_tools_config))
        registry.toggle_tool("cst", enabled=False)
        registry.toggle_tool("ast", enabled=True)

        output_file = tmp_path / "saved_state.toml"
        registry.save_enabled_state(output_file)

        # Verify file exists and contains expected state
        assert output_file.exists()
        saved_content = output_file.read_text()
        assert "[tools.cst]" in saved_content
        assert 'enabled = false' in saved_content


class TestExportState:
    """Test exporting registry state."""

    def test_to_dict(self, temp_tools_config):
        """
        GIVEN a registry with mixed tool states
        WHEN calling to_dict()
        THEN a dictionary with all tools and enabled tool names is returned.
        """
        registry = ToolRegistry(str(temp_tools_config))
        state = registry.to_dict()

        assert "tools" in state
        assert "enabled_tools" in state
        assert len(state["tools"]) == 3
        assert "cst" in state["enabled_tools"]
        assert "read_file" in state["enabled_tools"]
        assert "ast" not in state["enabled_tools"]
