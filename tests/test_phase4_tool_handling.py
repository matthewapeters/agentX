"""
Phase 4 Tests: Tool Call/Result Message Handling

Tests for storing and displaying tool execution messages in AgentX.
"""

import os
import sys
import tkinter as tk
from pathlib import Path
from unittest.mock import MagicMock, Mock, patch

# Add src to path
project_root = str(Path(__file__).parent.parent)
sys.path.insert(0, os.path.join(project_root, "src"))

from agentx.session import AgentXSession
from shared.models.context import Context
from shared.models.message import Message


def test_tool_call_message_storage():
    """Test that TOOL_CALL messages are stored in context."""
    # Use shared Context (Agentix format)
    from shared.models.context import Context as SharedContext
    from shared.models.message import Message as SharedMessage
    from shared.models.message import MessageRole

    context = SharedContext()

    # Create a tool call message
    tool_call_msg = SharedMessage(
        role=MessageRole.TOOL_CALL, content="Calling test_tool", tool_name="test_tool", tool_input={"param1": "value1"}
    )
    tool_call_msg.enabled = True

    # Add to context
    context.add_message(tool_call_msg)

    # Verify it's stored
    assert len(context.messages) > 0
    ts, last_msg = context.messages[-1]

    assert last_msg.role == MessageRole.TOOL_CALL
    assert last_msg.tool_name == "test_tool"
    assert last_msg.tool_input == {"param1": "value1"}
    print("✅ TOOL_CALL message storage verified")


def test_tool_result_message_storage():
    """Test that TOOL_RESULT messages are stored in context."""
    # Use shared Context (Agentix format)
    from shared.models.context import Context as SharedContext
    from shared.models.message import Message as SharedMessage
    from shared.models.message import MessageRole

    context = SharedContext()

    # Create a tool result message
    tool_result_msg = SharedMessage(
        role=MessageRole.TOOL_RESULT, content="Tool execution result", tool_name="test_tool"
    )
    tool_result_msg.enabled = True

    # Add to context
    context.add_message(tool_result_msg)

    # Verify it's stored
    assert len(context.messages) > 0
    ts, last_msg = context.messages[-1]

    assert last_msg.role == MessageRole.TOOL_RESULT
    assert last_msg.content == "Tool execution result"
    print("✅ TOOL_RESULT message storage verified")


def test_tool_execution_in_session():
    """Test tool execution method in AgentXSession."""
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
        },
    }

    # Mock the GUIManager to avoid tkinter issues
    with patch("agentx.session.GUIManager") as mock_gui_class:
        mock_gui = MagicMock()
        mock_gui_class.return_value = mock_gui

        # Create session
        session = AgentXSession(root, config)

        # Test execute_tool method
        result = session.execute_tool("test_tool", {"param": "value"})

        # Now returns "Unknown tool" due to Phase 6 routing logic
        assert "Unknown tool" in result or "error" in result.lower()
        print("✅ execute_tool() method works")


def test_gui_message_roles_updated():
    """Test that GUIManager has updated MESSAGE_ROLES."""
    from agentx.gui.gui_manager import GUIManager

    # Check that tool_call and tool_result are in MESSAGE_ROLES
    assert "tool_call" in GUIManager.MESSAGE_ROLES
    assert "tool_result" in GUIManager.MESSAGE_ROLES
    assert GUIManager.MESSAGE_ROLES["tool_call"] == "🔧"
    assert GUIManager.MESSAGE_ROLES["tool_result"] == "📋"
    print("✅ GUIManager.MESSAGE_ROLES updated with tool icons")


def test_shared_message_has_tool_fields():
    """Test that shared Message model has tool fields."""
    from shared.models.message import Message as SharedMessage
    from shared.models.message import MessageRole

    # Create a tool_call message
    msg = SharedMessage(
        role=MessageRole.TOOL_CALL, content="Tool call test", tool_name="test_tool", tool_input={"key": "value"}
    )

    assert msg.role == MessageRole.TOOL_CALL
    assert msg.tool_name == "test_tool"
    assert msg.tool_input == {"key": "value"}
    print("✅ Shared Message model has tool fields")


def test_phase4_integration():
    """Integration test for Phase 4 components."""
    # Verify all components work together
    from shared.models.context import Context as SharedContext
    from shared.models.message import Message as SharedMessage
    from shared.models.message import MessageRole

    context = SharedContext()

    # Create tool call sequence
    user_msg = SharedMessage(role=MessageRole.USER, content="Please analyze file.py")
    context.add_message(user_msg)

    tool_call = SharedMessage(
        role=MessageRole.TOOL_CALL, content="Analyzing file.py", tool_name="read_file", tool_input={"path": "file.py"}
    )
    context.add_message(tool_call)

    tool_result = SharedMessage(role=MessageRole.TOOL_RESULT, content="File contents...", tool_name="read_file")
    context.add_message(tool_result)

    assistant_msg = SharedMessage(role=MessageRole.ASSISTANT, content="I've analyzed the file.")
    context.add_message(assistant_msg)

    # Verify all messages are stored
    assert len(context.messages) == 4

    roles = [msg.role for _, msg in context.messages]
    assert roles == [MessageRole.USER, MessageRole.TOOL_CALL, MessageRole.TOOL_RESULT, MessageRole.ASSISTANT]

    print("✅ Phase 4 integration test passed")


if __name__ == "__main__":
    test_tool_call_message_storage()
    test_tool_result_message_storage()
    test_tool_execution_in_session()
    test_gui_message_roles_updated()
    test_shared_message_has_tool_fields()
    test_phase4_integration()
    print("\n🎉 All Phase 4 tests passed!")
