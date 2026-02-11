"""
Phase 5 Tests: Full Tool Execution Implementation

Tests for client-side and server-side tool execution.
"""

import sys
import os
import tempfile
from pathlib import Path
from unittest.mock import Mock, MagicMock, patch
import tkinter as tk

# Add src to path
project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from agentx.integration import ClientToolExecutor
from agentx.session import AgentXSession


class TestClientToolExecutor:
    """Tests for client-side tool execution."""
    
    def setup_method(self):
        """Setup test environment."""
        self.temp_dir = tempfile.TemporaryDirectory()
        self.executor = ClientToolExecutor(base_path=self.temp_dir.name)
    
    def teardown_method(self):
        """Cleanup."""
        self.temp_dir.cleanup()
    
    def test_read_file(self):
        """Test reading a file."""
        # Create a test file
        test_file = Path(self.temp_dir.name) / "test.txt"
        test_content = "Hello, World!"
        test_file.write_text(test_content)
        
        # Read file
        result = self.executor.execute("read_file", {"path": str(test_file)})
        
        assert result == test_content
        print("✅ read_file tool works")
    
    def test_read_file_not_found(self):
        """Test reading non-existent file."""
        # Use a path within the base directory that doesn't exist
        result = self.executor.execute("read_file", {"path": os.path.join(self.temp_dir.name, "nonexistent.txt")})
        assert "not found" in result.lower() or "error" in result.lower()
        print("✅ read_file error handling works")
    
    def test_write_file(self):
        """Test writing a file."""
        test_file = Path(self.temp_dir.name) / "output.txt"
        content = "Test content"
        
        result = self.executor.execute("write_file", {
            "path": str(test_file),
            "content": content
        })
        
        assert test_file.exists()
        assert test_file.read_text() == content
        assert "Wrote to file" in result
        print("✅ write_file tool works")
    
    def test_list_directory(self):
        """Test listing directory contents."""
        # Create some test files
        (Path(self.temp_dir.name) / "file1.txt").write_text("content1")
        (Path(self.temp_dir.name) / "file2.txt").write_text("content2")
        
        result = self.executor.execute("list_directory", {
            "path": self.temp_dir.name
        })
        
        assert "file1.txt" in result
        assert "file2.txt" in result
        print("✅ list_directory tool works")
    
    def test_get_file_info(self):
        """Test getting file information."""
        test_file = Path(self.temp_dir.name) / "info_test.txt"
        test_file.write_text("test content")
        
        result = self.executor.execute("get_file_info", {
            "path": str(test_file)
        })
        
        assert "is_file" in result
        assert "size_bytes" in result
        print("✅ get_file_info tool works")
    
    def test_search_files(self):
        """Test searching for files."""
        # Create some Python files
        (Path(self.temp_dir.name) / "script1.py").write_text("print('hi')")
        (Path(self.temp_dir.name) / "script2.py").write_text("print('bye')")
        (Path(self.temp_dir.name) / "other.txt").write_text("not python")
        
        result = self.executor.execute("search_files", {
            "path": self.temp_dir.name,
            "pattern": "*.py"
        })
        
        assert "script1.py" in result
        assert "script2.py" in result
        assert "other.txt" not in result
        print("✅ search_files tool works")
    
    def test_append_file(self):
        """Test appending to a file."""
        test_file = Path(self.temp_dir.name) / "append.txt"
        test_file.write_text("Line 1\n")
        
        self.executor.execute("write_file", {
            "path": str(test_file),
            "content": "Line 2\n",
            "append": True
        })
        
        content = test_file.read_text()
        assert "Line 1" in content
        assert "Line 2" in content
        print("✅ write_file append mode works")
    
    def test_path_security(self):
        """Test that path traversal is blocked."""
        # Try to access parent directory - should be blocked
        try:
            result = self.executor.execute("read_file", {
                "path": "../../etc/passwd"
            })
            # If we got here, check if it returned an error
            assert "outside allowed" in result or "error" in result.lower()
        except ValueError as e:
            # This is expected - path traversal should be blocked
            assert "outside allowed" in str(e)
        print("✅ Path security validation works")
    
    def test_unknown_tool(self):
        """Test handling unknown tool."""
        try:
            result = self.executor.execute("unknown_tool", {})
            assert False, "Should raise ValueError"
        except ValueError as e:
            assert "Unknown" in str(e)
            print("✅ Unknown tool handling works")
    
    def test_large_file_truncation(self):
        """Test that large files are truncated."""
        test_file = Path(self.temp_dir.name) / "large.txt"
        # Create file larger than 50KB
        large_content = "x" * 60000
        test_file.write_text(large_content)
        
        result = self.executor.execute("read_file", {"path": str(test_file)})
        
        assert len(result) < len(large_content)
        assert "truncated" in result.lower()
        print("✅ Large file truncation works")


def test_session_execute_tool_integration():
    """Test execute_tool method in session."""
    # Create a mock root window
    root = MagicMock(spec=tk.Tk)
    
    # Create minimal config
    config = {
        "agentx": {
            "ollama_model": "llama3.2",
            "ollama_host": "http://localhost:11434",
        },
        "agentix": {
            "host": "localhost:8000",
        }
    }
    
    # Mock the GUIManager
    with patch('agentx.session.GUIManager') as mock_gui_class:
        mock_gui = MagicMock()
        mock_gui_class.return_value = mock_gui
        
        # Create session
        session = AgentXSession(root, config)
        
        # Use an existing file in the project
        test_file = Path("/Users/mpeters/starbucks/projects/agentX/README.md")
        if test_file.exists():
            # Execute read_file tool via session
            result = session.execute_tool("read_file", {"path": str(test_file)})
            
            # Result should contain file content
            assert len(result) > 0
            print("✅ Session.execute_tool() works with ClientToolExecutor")
        else:
            print("✅ Session.execute_tool() skipped (test file not found)")


def test_phase5_integration():
    """Full integration test for Phase 5."""
    print("\n" + "="*50)
    print("Phase 5 Integration Test")
    print("="*50)
    
    # Test 1: Tool execution flow
    with tempfile.TemporaryDirectory() as temp_dir:
        executor = ClientToolExecutor(base_path=temp_dir)
        
        # Create test file
        test_file = Path(temp_dir) / "data.txt"
        test_file.write_text("Original content")
        
        # Read it
        content = executor.execute("read_file", {"path": str(test_file)})
        assert content == "Original content"
        
        # List directory
        listing = executor.execute("list_directory", {"path": temp_dir})
        assert "data.txt" in listing
        
        # Get info
        info = executor.execute("get_file_info", {"path": str(test_file)})
        assert "Original content" not in info  # Should be metadata, not content
        assert "size_bytes" in info
        
        # Search files
        search = executor.execute("search_files", {
            "path": temp_dir,
            "pattern": "*.txt"
        })
        assert "data.txt" in search
    
    print("✅ Phase 5 integration test passed")


def test_client_tools_support():
    """Verify all client-side tools are supported."""
    with tempfile.TemporaryDirectory() as temp_dir:
        executor = ClientToolExecutor(base_path=temp_dir)
        
        # Create a test file for read operations
        test_file = Path(temp_dir) / "test.txt"
        test_file.write_text("test content")
        
        # Verify all client tools can be called
        client_tools = {
            "read_file": {"path": str(test_file)},
            "list_directory": {"path": temp_dir},
            "get_file_info": {"path": str(test_file)},
            "search_files": {"path": temp_dir, "pattern": "*"},
        }
        
        for tool_name, args in client_tools.items():
            try:
                # Just verify it doesn't crash on unknown tool error
                result = executor.execute(tool_name, args)
                assert result is not None
                print(f"✅ {tool_name} is supported")
            except ValueError as e:
                if "Unknown" in str(e):
                    assert False, f"Tool {tool_name} not supported"
                # Other errors are OK (e.g., file not found)


if __name__ == "__main__":
    print("\n" + "="*50)
    print("Running Phase 5 Tool Execution Tests")
    print("="*50 + "\n")
    
    # Test ClientToolExecutor
    print("Testing ClientToolExecutor...")
    tester = TestClientToolExecutor()
    
    tester.setup_method()
    tester.test_read_file()
    tester.teardown_method()
    
    tester.setup_method()
    tester.test_read_file_not_found()
    tester.teardown_method()
    
    tester.setup_method()
    tester.test_write_file()
    tester.teardown_method()
    
    tester.setup_method()
    tester.test_list_directory()
    tester.teardown_method()
    
    tester.setup_method()
    tester.test_get_file_info()
    tester.teardown_method()
    
    tester.setup_method()
    tester.test_search_files()
    tester.teardown_method()
    
    tester.setup_method()
    tester.test_append_file()
    tester.teardown_method()
    
    tester.setup_method()
    tester.test_path_security()
    tester.teardown_method()
    
    tester.setup_method()
    tester.test_unknown_tool()
    tester.teardown_method()
    
    tester.setup_method()
    tester.test_large_file_truncation()
    tester.teardown_method()
    
    # Test session integration
    print("\nTesting Session Integration...")
    test_session_execute_tool_integration()
    
    # Test phase 5 integration
    test_phase5_integration()
    
    # Test client tools
    print("\nVerifying Client Tools Support...")
    test_client_tools_support()
    
    print("\n" + "="*50)
    print("🎉 All Phase 5 tests passed!")
    print("="*50)
