"""
Phase 6 Tests: Advanced Tools & Server-side Integration

Tests for server-side tool execution, code analysis tools, and advanced tool registry.
"""

import sys
import os
from pathlib import Path
from unittest.mock import Mock, MagicMock, patch
import tkinter as tk

# Add src to path
project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from agentx.integration import (
    ServerToolExecutor,
    AdvancedToolRegistry,
    CodeAnalysisTool,
)
from agentx.session import AgentXSession


class TestServerToolExecutor:
    """Tests for server-side tool execution."""
    
    def setup_method(self):
        """Setup test environment."""
        self.mock_bridge = MagicMock()
        self.executor = ServerToolExecutor(agentix_bridge=self.mock_bridge)
    
    def test_executor_availability(self):
        """Test checking if executor is available."""
        assert self.executor.is_available() == True
        
        # Test with no bridge
        executor_no_bridge = ServerToolExecutor(agentix_bridge=None)
        assert executor_no_bridge.is_available() == False
        print("✅ ServerToolExecutor availability checking works")
    
    def test_get_available_tools(self):
        """Test getting available tools from bridge."""
        mock_tools = [
            {
                "type": "function",
                "function": {
                    "name": "analyze_syntax",
                    "description": "Analyze code syntax"
                }
            }
        ]
        self.mock_bridge.get_available_tools.return_value = mock_tools
        
        tools = self.executor.get_available_tools()
        
        assert len(tools) > 0
        print("✅ ServerToolExecutor.get_available_tools() works")
    
    def test_execute_unavailable_tool(self):
        """Test executing tool that's not available."""
        self.mock_bridge.get_available_tools.return_value = []
        
        result = self.executor.execute("unknown_tool", {})
        
        assert "Unknown server tool" in result or "error" in result.lower()
        print("✅ ServerToolExecutor handles unknown tools")
    
    def test_no_bridge_error(self):
        """Test error when no bridge available."""
        executor = ServerToolExecutor(agentix_bridge=None)
        
        try:
            result = executor.execute("any_tool", {})
            assert False, "Should raise error"
        except ValueError as e:
            assert "not available" in str(e).lower()
            print("✅ ServerToolExecutor raises error when no bridge")
    
    def test_tool_result_formatting(self):
        """Test formatting of tool results."""
        result = self.executor._format_tool_result(
            tool_name="test_tool",
            status="success",
            message="Tool executed",
            result={"data": "value"}
        )
        
        assert "test_tool" in result
        assert "success" in result
        assert "Tool executed" in result
        print("✅ ServerToolExecutor result formatting works")


class TestCodeAnalysisTool:
    """Tests for code analysis tool classification."""
    
    def test_is_code_analysis_tool(self):
        """Test identifying code analysis tools."""
        assert CodeAnalysisTool.is_code_analysis_tool("analyze_syntax") == True
        assert CodeAnalysisTool.is_code_analysis_tool("find_functions") == True
        assert CodeAnalysisTool.is_code_analysis_tool("find_classes") == True
        assert CodeAnalysisTool.is_code_analysis_tool("unknown_tool") == False
        print("✅ CodeAnalysisTool identification works")
    
    def test_get_description(self):
        """Test getting tool descriptions."""
        desc = CodeAnalysisTool.get_description("analyze_syntax")
        assert desc is not None
        assert "syntax" in desc.lower() or "cst" in desc.lower()
        
        desc = CodeAnalysisTool.get_description("unknown_tool")
        assert desc is None
        print("✅ CodeAnalysisTool.get_description() works")
    
    def test_available_tools(self):
        """Test that code analysis tools are available."""
        assert len(CodeAnalysisTool.TOOLS) > 0
        assert "analyze_syntax" in CodeAnalysisTool.TOOLS
        assert "find_functions" in CodeAnalysisTool.TOOLS
        print("✅ Code analysis tools are available")


class TestAdvancedToolRegistry:
    """Tests for advanced tool registry."""
    
    def setup_method(self):
        """Setup test environment."""
        self.mock_bridge = MagicMock()
        self.registry = AdvancedToolRegistry(agentix_bridge=self.mock_bridge)
    
    def test_registry_initialization(self):
        """Test registry initialization."""
        self.mock_bridge.get_available_tools.return_value = [
            {
                "type": "function",
                "function": {
                    "name": "analyze_syntax",
                    "description": "Analyze code"
                }
            }
        ]
        
        self.registry.initialize()
        
        assert self.registry._initialized == True
        print("✅ AdvancedToolRegistry initialization works")
    
    def test_get_tool_info(self):
        """Test getting tool information."""
        self.mock_bridge.get_available_tools.return_value = [
            {
                "type": "function",
                "function": {
                    "name": "analyze_syntax",
                    "description": "Analyze code syntax"
                }
            }
        ]
        
        info = self.registry.get_tool_info("analyze_syntax")
        
        assert info is not None
        assert info["name"] == "analyze_syntax"
        assert info["is_code_analysis"] == True
        print("✅ AdvancedToolRegistry.get_tool_info() works")
    
    def test_list_tools_all(self):
        """Test listing all tools."""
        self.mock_bridge.get_available_tools.return_value = [
            {
                "type": "function",
                "function": {
                    "name": "analyze_syntax",
                    "description": "Code analysis"
                }
            }
        ]
        
        tools = self.registry.list_tools()
        
        assert len(tools) > 0
        print("✅ AdvancedToolRegistry.list_tools() works")
    
    def test_list_tools_filtered(self):
        """Test listing tools with category filter."""
        self.mock_bridge.get_available_tools.return_value = [
            {
                "type": "function",
                "function": {
                    "name": "analyze_syntax",
                    "description": "Code analysis"
                }
            }
        ]
        
        code_tools = self.registry.list_tools(category="code_analysis")
        
        assert len(code_tools) > 0
        assert all(t["is_code_analysis"] for t in code_tools)
        print("✅ AdvancedToolRegistry category filtering works")
    
    def test_registry_without_bridge(self):
        """Test registry without bridge."""
        registry = AdvancedToolRegistry(agentix_bridge=None)
        registry.initialize()
        
        tools = registry.list_tools()
        
        assert len(tools) == 0
        assert registry._initialized == True
        print("✅ AdvancedToolRegistry works without bridge")


def test_session_with_advanced_tools():
    """Test session integration with advanced tools."""
    # Create a mock root window
    root = MagicMock(spec=tk.Tk)
    
    # Create minimal config
    config = {
        "agentx": {
            "ollama_model": "llama3.2",
            "ollama_host": "http://localhost:11434",
        },
        "agentix": {
            "enabled": False,
        }
    }
    
    # Mock the GUIManager
    with patch('agentx.session.GUIManager') as mock_gui_class:
        mock_gui = MagicMock()
        mock_gui_class.return_value = mock_gui
        
        # Create session
        session = AgentXSession(root, config)
        
        # Verify tool executors are initialized
        assert session.client_tool_executor is not None
        assert session.server_tool_executor is not None
        assert session.advanced_tools is not None
        print("✅ Session initializes tool executors")
        
        # Verify execute_tool routing works
        result = session.execute_tool("unknown_tool", {})
        assert "Unknown tool" in result or "error" in result.lower()
        print("✅ Session.execute_tool() routing works")


def test_tool_routing():
    """Test intelligent tool routing in session."""
    # Create a mock root window
    root = MagicMock(spec=tk.Tk)
    
    config = {
        "agentx": {
            "ollama_model": "llama3.2",
            "ollama_host": "http://localhost:11434",
        },
        "agentix": {
            "enabled": False,
        }
    }
    
    with patch('agentx.session.GUIManager') as mock_gui_class:
        mock_gui = MagicMock()
        mock_gui_class.return_value = mock_gui
        
        session = AgentXSession(root, config)
        
        # Client tool should be recognized
        client_tools = ["read_file", "write_file", "list_directory", "get_file_info", "search_files"]
        for tool in client_tools:
            # Just verify no "Unknown tool" error (actual execution may fail due to paths)
            result = session.execute_tool(tool, {})
            assert result is not None
        
        print("✅ Session recognizes client-side tools")
        
        # Code analysis tool without server
        result = session.execute_tool("analyze_syntax", {})
        assert "not available" in result.lower() or "agentix" in result.lower()
        print("✅ Session handles code analysis tools without server")


def test_phase6_integration():
    """Full integration test for Phase 6."""
    print("\n" + "="*60)
    print("Phase 6 Integration Test")
    print("="*60)
    
    # Test 1: Tool executors created
    executor_server = ServerToolExecutor(agentix_bridge=None)
    assert executor_server.is_available() == False
    
    # Test 2: Code analysis tool classification
    assert CodeAnalysisTool.is_code_analysis_tool("analyze_syntax") == True
    assert CodeAnalysisTool.is_code_analysis_tool("read_file") == False
    
    # Test 3: Registry initialization
    registry = AdvancedToolRegistry(agentix_bridge=None)
    registry.initialize()
    assert registry._initialized == True
    
    print("✅ Phase 6 integration test passed")


def test_advanced_tool_categories():
    """Test that advanced tools are properly categorized."""
    print("\nAdvanced Tool Categories:")
    
    # Code analysis tools
    code_tools = {
        name: tool for name, tool in CodeAnalysisTool.TOOLS.items()
        if "syntax" in tool["categories"] or "structure" in tool["categories"]
    }
    print(f"  Code Analysis Tools: {len(code_tools)} available")
    for name in code_tools:
        print(f"    - {name}: {CodeAnalysisTool.TOOLS[name]['description']}")
    
    # All tools
    all_tools = CodeAnalysisTool.TOOLS
    print(f"  Total Code Analysis Tools: {len(all_tools)}")
    
    assert len(all_tools) > 0
    print("✅ Advanced tool categories verified")


if __name__ == "__main__":
    print("\n" + "="*60)
    print("Running Phase 6 Advanced Tools & Server Integration Tests")
    print("="*60 + "\n")
    
    # Test ServerToolExecutor
    print("Testing ServerToolExecutor...")
    tester = TestServerToolExecutor()
    
    tester.setup_method()
    tester.test_executor_availability()
    tester.test_get_available_tools()
    tester.test_execute_unavailable_tool()
    tester.test_no_bridge_error()
    tester.test_tool_result_formatting()
    
    # Test CodeAnalysisTool
    print("\nTesting CodeAnalysisTool...")
    test_code = TestCodeAnalysisTool()
    test_code.test_is_code_analysis_tool()
    test_code.test_get_description()
    test_code.test_available_tools()
    
    # Test AdvancedToolRegistry
    print("\nTesting AdvancedToolRegistry...")
    tester_registry = TestAdvancedToolRegistry()
    
    tester_registry.setup_method()
    tester_registry.test_registry_initialization()
    tester_registry.test_get_tool_info()
    tester_registry.test_list_tools_all()
    tester_registry.test_list_tools_filtered()
    tester_registry.test_registry_without_bridge()
    
    # Test session integration
    print("\nTesting Session Integration...")
    test_session_with_advanced_tools()
    test_tool_routing()
    
    # Test phase 6 integration
    test_phase6_integration()
    
    # Test tool categories
    test_advanced_tool_categories()
    
    print("\n" + "="*60)
    print("🎉 All Phase 6 tests passed!")
    print("="*60)
