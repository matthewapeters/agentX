"""Tests for tool diagnostics.

GIVEN a tool diagnostics instance with bridge and registry
WHEN running diagnostic phases (registry, bridge, availability, execution)
THEN diagnostics correctly identify tool status and issues.
"""

import json
from unittest.mock import MagicMock, patch

import pytest

from src.agentx.tool_diagnostics import ToolDiagnostics, create_tool_diagnostics


class TestToolDiagnosticsInit:
    """Test ToolDiagnostics initialization."""

    def test_init_with_bridge_only(self):
        """
        GIVEN a bridge instance
        WHEN creating ToolDiagnostics
        THEN instance is created with bridge set.
        """
        bridge = MagicMock()
        diag = ToolDiagnostics(bridge)

        assert diag.bridge == bridge
        assert diag.registry_manager is None

    def test_init_with_bridge_and_registry(self):
        """
        GIVEN bridge and registry manager instances
        WHEN creating ToolDiagnostics
        THEN both are stored.
        """
        bridge = MagicMock()
        registry = MagicMock()
        diag = ToolDiagnostics(bridge, registry)

        assert diag.bridge == bridge
        assert diag.registry_manager == registry


class TestRegistryDiagnostics:
    """Test registry diagnostics phase."""

    def test_diagnose_registry_with_manager(self):
        """
        GIVEN a registry manager with tools
        WHEN running _diagnose_registry()
        THEN tool inventory is returned.
        """
        bridge = MagicMock()
        registry = MagicMock()
        registry.get_available_tools.return_value = [
            {"name": "read_file", "enabled": True, "category": "filesystem"},
            {"name": "write_file", "enabled": False, "category": "filesystem"},
        ]
        registry.get_enabled_tools.return_value = [
            {"name": "read_file", "enabled": True, "category": "filesystem"},
        ]

        diag = ToolDiagnostics(bridge, registry)
        result = diag._diagnose_registry()

        assert result["phase"] == "registry"
        assert result["status"] == "ok"
        assert result["total_tools"] == 2
        assert result["enabled_tools"] == 1

    def test_diagnose_registry_without_manager(self):
        """
        GIVEN no registry manager
        WHEN running _diagnose_registry()
        THEN status is skipped.
        """
        bridge = MagicMock()
        diag = ToolDiagnostics(bridge)
        result = diag._diagnose_registry()

        assert result["phase"] == "registry"
        assert result["status"] == "skipped"

    def test_diagnose_registry_error_handling(self):
        """
        GIVEN a registry that raises an exception
        WHEN running _diagnose_registry()
        THEN error status is returned.
        """
        bridge = MagicMock()
        registry = MagicMock()
        registry.get_available_tools.side_effect = RuntimeError("Registry load failed")

        diag = ToolDiagnostics(bridge, registry)
        result = diag._diagnose_registry()

        assert result["phase"] == "registry"
        assert result["status"] == "error"
        assert "Registry load failed" in result["error"]


class TestBridgeDiagnostics:
    """Test bridge tool registration diagnostics."""

    def test_diagnose_bridge_tools(self):
        """
        GIVEN a bridge with available tools
        WHEN running _diagnose_bridge_tools()
        THEN tool registration status is reported.
        """
        bridge = MagicMock()
        bridge.get_available_tools.return_value = [
            {"name": "read_file", "description": "Read file"},
            {"name": "write_file", "description": "Write file"},
            {"name": "reload_tools", "description": "Reload tools"},
        ]

        diag = ToolDiagnostics(bridge)
        result = diag._diagnose_bridge_tools()

        assert result["phase"] == "bridge"
        assert result["status"] == "ok"
        assert result["tools_available_to_bridge"] == 3
        assert "read_file" in result["tool_names"]

    def test_diagnose_bridge_error(self):
        """
        GIVEN a bridge that raises an exception
        WHEN running _diagnose_bridge_tools()
        THEN error is reported.
        """
        bridge = MagicMock()
        bridge.get_available_tools.side_effect = RuntimeError("Bridge error")

        diag = ToolDiagnostics(bridge)
        result = diag._diagnose_bridge_tools()

        assert result["phase"] == "bridge"
        assert result["status"] == "error"


class TestAvailabilityDiagnostics:
    """Test tool availability to LLM."""

    def test_diagnose_availability_all_tools_present(self):
        """
        GIVEN a bridge with critical tools
        WHEN running _diagnose_tool_availability()
        THEN all critical tools are reported as available.
        """
        bridge = MagicMock()
        bridge.get_available_tools.return_value = [
            {"name": "read_file"},
            {"name": "write_file"},
            {"name": "reload_tools"},
        ]

        diag = ToolDiagnostics(bridge)
        result = diag._diagnose_tool_availability()

        assert result["phase"] == "availability"
        assert result["status"] == "ok"
        assert result["missing_critical_tools"] == []

    def test_diagnose_availability_missing_critical_tools(self):
        """
        GIVEN a bridge missing critical tools
        WHEN running _diagnose_tool_availability()
        THEN missing tools are reported as warning.
        """
        bridge = MagicMock()
        bridge.get_available_tools.return_value = [
            {"name": "reload_tools"},
        ]

        diag = ToolDiagnostics(bridge)
        result = diag._diagnose_tool_availability()

        assert result["phase"] == "availability"
        assert result["status"] == "warning"
        assert "read_file" in result["missing_critical_tools"]
        assert "write_file" in result["missing_critical_tools"]

    def test_diagnose_availability_no_tools(self):
        """
        GIVEN a bridge with no tools
        WHEN running _diagnose_tool_availability()
        THEN warning is reported.
        """
        bridge = MagicMock()
        bridge.get_available_tools.return_value = []

        diag = ToolDiagnostics(bridge)
        result = diag._diagnose_tool_availability()

        assert result["phase"] == "availability"
        assert result["status"] == "warning"
        assert result["available_tools"] == 0


class TestExecutionDiagnostics:
    """Test end-to-end tool execution."""

    def test_diagnose_execution_success(self):
        """
        GIVEN a bridge where write_file and read_file work
        WHEN running _diagnose_execution()
        THEN both tests report success.
        """
        bridge = MagicMock()
        test_content = "AgentX tool diagnostic test"

        def mock_execute_tool(tool_name, arguments):
            if tool_name == "write_file":
                return "ok"
            elif tool_name == "read_file":
                return test_content
            raise ValueError(f"Unknown tool: {tool_name}")

        bridge.execute_tool = MagicMock(side_effect=mock_execute_tool)

        diag = ToolDiagnostics(bridge)
        result = diag._diagnose_execution()

        assert result["phase"] == "execution"
        assert result["status"] == "ok"
        assert result["write_file_test"]["status"] == "ok"
        assert result["read_file_test"]["status"] == "ok"

    def test_diagnose_execution_write_fails(self):
        """
        GIVEN write_file fails
        WHEN running _diagnose_execution()
        THEN write failure is reported.
        """
        bridge = MagicMock()
        bridge.execute_tool.side_effect = RuntimeError("Permission denied")

        diag = ToolDiagnostics(bridge)
        result = diag._diagnose_execution()

        assert result["phase"] == "execution"
        assert result["status"] == "failed"
        assert result["write_file_test"]["status"] == "failed"
        assert "Permission denied" in result["write_file_test"]["error"]


class TestFullDiagnostic:
    """Test full diagnostic suite."""

    def test_run_full_diagnostic_healthy(self):
        """
        GIVEN a fully healthy tool pipeline
        WHEN running run_full_diagnostic()
        THEN overall status is ok.
        """
        bridge = MagicMock()
        registry = MagicMock()

        # Mock registry
        registry.get_available_tools.return_value = [
            {"name": "read_file", "enabled": True, "category": "filesystem"},
            {"name": "write_file", "enabled": True, "category": "filesystem"},
        ]
        registry.get_enabled_tools.return_value = registry.get_available_tools.return_value

        # Mock bridge
        bridge.get_available_tools.return_value = [
            {"name": "read_file"},
            {"name": "write_file"},
        ]

        def mock_execute(tool_name, args):
            if tool_name == "write_file":
                return "ok"
            return "test content"

        bridge.execute_tool = MagicMock(side_effect=mock_execute)

        diag = ToolDiagnostics(bridge, registry)
        report = diag.run_full_diagnostic()

        assert report["status"] == "ok"
        assert len(report["phases"]) == 4
        assert len(report["issues"]) == 0

    def test_run_full_diagnostic_with_issues(self):
        """
        GIVEN a pipeline with missing tools
        WHEN running run_full_diagnostic()
        THEN issues are reported.
        """
        bridge = MagicMock()
        registry = MagicMock()

        registry.get_available_tools.return_value = [
            {"name": "read_file", "enabled": True, "category": "filesystem"},
        ]
        registry.get_enabled_tools.return_value = registry.get_available_tools.return_value

        # Bridge only has one tool
        bridge.get_available_tools.return_value = [
            {"name": "read_file"},
        ]
        bridge.execute_tool.side_effect = RuntimeError("Tool execution failed")

        diag = ToolDiagnostics(bridge, registry)
        report = diag.run_full_diagnostic()

        assert report["status"] == "warning"
        assert len(report["issues"]) > 0

    def test_get_diagnostic_json(self):
        """
        GIVEN a diagnostic instance
        WHEN calling get_diagnostic_json()
        THEN valid JSON is returned.
        """
        bridge = MagicMock()
        bridge.get_available_tools.return_value = [
            {"name": "read_file"},
        ]

        diag = ToolDiagnostics(bridge)
        diag.run_full_diagnostic()
        json_output = diag.get_diagnostic_json()

        parsed = json.loads(json_output)
        assert "status" in parsed
        assert "phases" in parsed
        assert "issues" in parsed


class TestFactoryFunction:
    """Test factory function."""

    def test_create_tool_diagnostics(self):
        """
        GIVEN bridge and registry instances
        WHEN calling create_tool_diagnostics()
        THEN ToolDiagnostics instance is returned.
        """
        bridge = MagicMock()
        registry = MagicMock()

        diag = create_tool_diagnostics(bridge, registry)

        assert isinstance(diag, ToolDiagnostics)
        assert diag.bridge == bridge
        assert diag.registry_manager == registry
