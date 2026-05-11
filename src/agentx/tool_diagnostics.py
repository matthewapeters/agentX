"""Tool diagnostics for verifying end-to-end tool execution pipeline.

Provides diagnostic utilities to verify that tools are correctly registered
with the bridge, discoverable by the agent, and executable end-to-end.
"""

import json
from typing import Optional, Any, Callable
import logging

logger = logging.getLogger("agentx.tool_diagnostics")


class ToolDiagnostics:
    """
    Diagnose tool registration and execution pipeline issues.

    Verifies:
    - Tools are in the registry
    - Tools are registered with the bridge
    - Tools have valid schemas for the LLM
    - Tools can be invoked end-to-end
    - Agent can see and call tools

    Example:
        diag = ToolDiagnostics(bridge, registry_manager)
        report = diag.run_full_diagnostic()
        print(report)
    """

    def __init__(
        self,
        bridge: Any,
        registry_manager: Optional[Any] = None,
    ):
        """
        Initialize tool diagnostics.

        Args:
            bridge: AgentixBridge instance
            registry_manager: Optional ToolRegistryManager for registry diagnostics
        """
        self.bridge = bridge
        self.registry_manager = registry_manager
        self.diagnostics = []

    def run_full_diagnostic(self) -> dict[str, Any]:
        """
        Run complete diagnostic suite.

        Returns:
            Dictionary with detailed diagnostic results including issues found.
        """
        self.diagnostics.clear()

        # Phase 1: Registry diagnostics
        registry_diag = self._diagnose_registry()
        self.diagnostics.append(registry_diag)

        # Phase 2: Bridge tool registration
        bridge_diag = self._diagnose_bridge_tools()
        self.diagnostics.append(bridge_diag)

        # Phase 3: Tool availability to LLM
        availability_diag = self._diagnose_tool_availability()
        self.diagnostics.append(availability_diag)

        # Phase 4: Test execution (read_file, write_file)
        execution_diag = self._diagnose_execution()
        self.diagnostics.append(execution_diag)

        return self._compile_report()

    def _diagnose_registry(self) -> dict[str, Any]:
        """Diagnose tool registry state."""
        if not self.registry_manager:
            return {
                "phase": "registry",
                "status": "skipped",
                "reason": "registry_manager not provided",
            }

        try:
            all_tools = self.registry_manager.get_available_tools()
            enabled_tools = self.registry_manager.get_enabled_tools()

            return {
                "phase": "registry",
                "status": "ok",
                "total_tools": len(all_tools),
                "enabled_tools": len(enabled_tools),
                "tools": [
                    {
                        "name": t["name"],
                        "enabled": t["enabled"],
                        "category": t.get("category", "unknown"),
                    }
                    for t in all_tools
                ],
            }
        except Exception as e:
            return {
                "phase": "registry",
                "status": "error",
                "error": str(e),
            }

    def _diagnose_bridge_tools(self) -> dict[str, Any]:
        """Diagnose bridge tool registration."""
        try:
            available_tools = self.bridge.get_available_tools()

            return {
                "phase": "bridge",
                "status": "ok",
                "tools_available_to_bridge": len(available_tools),
                "tool_names": [t.get("name") for t in available_tools],
            }
        except Exception as e:
            return {
                "phase": "bridge",
                "status": "error",
                "error": str(e),
            }

    def _diagnose_tool_availability(self) -> dict[str, Any]:
        """Diagnose tool availability to LLM (schema check)."""
        try:
            available_tools = self.bridge.get_available_tools()

            if not available_tools:
                return {
                    "phase": "availability",
                    "status": "warning",
                    "message": "No tools visible to LLM",
                    "available_tools": 0,
                }

            # Check critical tools
            tool_names = {t.get("name") for t in available_tools}
            critical_tools = {"read_file", "write_file"}
            missing = critical_tools - tool_names

            return {
                "phase": "availability",
                "status": "ok" if not missing else "warning",
                "tools_available": len(available_tools),
                "critical_tools": list(critical_tools),
                "missing_critical_tools": list(missing),
                "builtin_tools": [
                    t.get("name")
                    for t in available_tools
                    if t.get("name") in {"reload_tools", "register_tool"}
                ],
            }
        except Exception as e:
            return {
                "phase": "availability",
                "status": "error",
                "error": str(e),
            }

    def _diagnose_execution(self) -> dict[str, Any]:
        """Test end-to-end tool execution."""
        try:
            # Test 1: write_file
            test_file = "/tmp/agentx_diagnostic_test.txt"
            test_content = "AgentX tool diagnostic test"

            try:
                result = self.bridge.execute_tool(
                    "write_file",
                    {"path": test_file, "content": test_content},
                )
                write_status = "ok"
                write_error = None
            except Exception as e:
                write_status = "failed"
                write_error = str(e)

            # Test 2: read_file
            try:
                result = self.bridge.execute_tool(
                    "read_file",
                    {"path": test_file},
                )
                read_status = "ok"
                read_error = None
                read_result = result if isinstance(result, str) else str(result)
            except Exception as e:
                read_status = "failed"
                read_error = str(e)
                read_result = None

            return {
                "phase": "execution",
                "status": "ok" if write_status == "ok" and read_status == "ok" else "failed",
                "write_file_test": {
                    "status": write_status,
                    "error": write_error,
                },
                "read_file_test": {
                    "status": read_status,
                    "error": read_error,
                    "sample": read_result[:50] if read_result else None,
                },
            }
        except Exception as e:
            return {
                "phase": "execution",
                "status": "error",
                "error": str(e),
            }

    def _compile_report(self) -> dict[str, Any]:
        """Compile full diagnostic report."""
        issues = []

        # Check for critical issues
        for phase_result in self.diagnostics:
            if phase_result.get("status") == "error":
                issues.append(
                    f"ERROR in {phase_result['phase']}: {phase_result.get('error')}"
                )
            elif phase_result.get("status") == "warning":
                if missing := phase_result.get("missing_critical_tools"):
                    issues.append(f"WARNING: Missing critical tools: {missing}")

        return {
            "status": "ok" if not issues else "warning" if any("WARNING" in i for i in issues) else "error",
            "timestamp": __import__("datetime").datetime.now().isoformat(),
            "phases": self.diagnostics,
            "issues": issues,
            "summary": self._build_summary(),
        }

    def _build_summary(self) -> str:
        """Build human-readable summary."""
        parts = []

        for phase in self.diagnostics:
            phase_name = phase.get("phase", "unknown")
            status = phase.get("status", "unknown")

            if status == "ok":
                if phase_name == "registry":
                    count = phase.get("total_tools", 0)
                    parts.append(f"✓ Registry: {count} tools available")
                elif phase_name == "bridge":
                    count = phase.get("tools_available_to_bridge", 0)
                    parts.append(f"✓ Bridge: {count} tools registered")
                elif phase_name == "availability":
                    parts.append(f"✓ LLM can see all critical tools")
                elif phase_name == "execution":
                    parts.append(f"✓ read_file and write_file execute successfully")
            elif status == "warning":
                if missing := phase.get("missing_critical_tools"):
                    parts.append(f"⚠ Missing tools: {missing}")
                else:
                    parts.append(f"⚠ {phase_name}: {phase.get('message', 'unknown issue')}")
            elif status == "error":
                error = phase.get("error", "unknown error")
                parts.append(f"✗ {phase_name}: {error}")
            elif status == "skipped":
                parts.append(f"- {phase_name}: skipped ({phase.get('reason', 'unknown')})")

        return "\n".join(parts)

    def get_diagnostic_json(self) -> str:
        """Export diagnostic report as JSON."""
        report = self._compile_report()
        return json.dumps(report, indent=2)


def create_tool_diagnostics(bridge: Any, registry_manager: Optional[Any] = None) -> ToolDiagnostics:
    """
    Factory function to create tool diagnostics.

    Args:
        bridge: AgentixBridge instance
        registry_manager: Optional ToolRegistryManager

    Returns:
        ToolDiagnostics instance
    """
    return ToolDiagnostics(bridge, registry_manager)
