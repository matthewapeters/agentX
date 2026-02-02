"""Smoke tests for complete AgentX workflows.

Tests end-to-end workflows to ensure all phases work together
without regressions.
"""

import tkinter as tk
import unittest
from unittest.mock import MagicMock, patch
from datetime import datetime
import sys
import os
import tempfile
import shutil
import threading
import time

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from agentx.session import AgentXSession
from agentx.gui_manager import GUIManager
from agentx.message import Message
from agentx.context import Context


class TestAgentXWorkflow(unittest.TestCase):
    """Test complete AgentX workflows."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.root = tk.Tk()
        self.root.withdraw()
        
        # Create temporary directory for session
        self.temp_dir = tempfile.mkdtemp()
        
        # Patch os.getcwd
        self.patcher_getcwd = patch('os.getcwd')
        self.mock_getcwd = self.patcher_getcwd.start()
        self.mock_getcwd.return_value = self.temp_dir
        
        # Patch os.getenv
        self.patcher_getenv = patch('os.getenv')
        self.mock_getenv = self.patcher_getenv.start()
        self.mock_getenv.side_effect = lambda key, default=None: {
            "USER": "testuser",
            "USERNAME": "testuser"
        }.get(key, default)
        
        config = {
            "ollama_host": "localhost:11434",
            "ollama_model": "mistral",
            "ollama_timeout": 30,
        }
        
        self.session = AgentXSession(
            root=self.root,
            config=config
        )
        
        # Create layout
        self.session.gui.create_layout()
    
    def tearDown(self):
        """Clean up after tests."""
        self.patcher_getcwd.stop()
        self.patcher_getenv.stop()
        
        try:
            self.root.destroy()
        except:
            pass
        
        try:
            shutil.rmtree(self.temp_dir)
        except:
            pass
    
    def test_complete_message_display_workflow(self):
        """Test complete workflow: user input → agent response → display."""
        # Step 1: User enters text
        input_text = self.session.gui.widgets.user_input_text
        input_text.insert("1.0", "Hello, agent!")
        
        # Step 2: Get user input
        user_input = self.session.gui.get_user_input()
        self.assertEqual(user_input, "Hello, agent!")
        
        # Step 3: Display user message
        self.session.gui.display_user_message(
            user_input,
            attachments=[],
            timestamp=datetime.now()
        )
        
        # Step 4: Simulate agent thinking
        self.session.gui.display_agent_thinking("Analyzing...")
        
        # Step 5: Simulate agent response
        self.session.gui.display_agent_response("Here's my response...")
        
        # Step 6: Add spacing
        self.session.gui.display_spacing()
        
        # Verify all content in output
        output_text = self.session.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)
        
        self.assertIn("User:", content)
        self.assertIn("Hello, agent!", content)
        self.assertIn("Agent is thinking", content)
        self.assertIn("Analyzing", content)
        self.assertIn("Agent:", content)
        self.assertIn("Here's my response", content)
    
    def test_streaming_state_transitions(self):
        """Test streaming state transitions during workflow."""
        # Step 1: Start streaming
        self.session.gui.set_streaming_state(True)
        submit_btn = self.session.gui.widgets.user_submit
        interrupt_btn = self.session.gui.widgets.user_break
        
        self.assertEqual(submit_btn.cget("state"), tk.DISABLED)
        self.assertEqual(interrupt_btn.cget("state"), tk.NORMAL)
        
        # Step 2: Display streaming content
        self.session.gui.display_agent_thinking("Processing...")
        self.session.gui.display_agent_thinking(" more...")
        
        # Verify buttons still in streaming state
        self.assertEqual(submit_btn.cget("state"), tk.DISABLED)
        self.assertEqual(interrupt_btn.cget("state"), tk.NORMAL)
        
        # Step 3: End streaming
        self.session.gui.set_streaming_state(False)
        
        # Verify buttons return to normal state
        self.assertEqual(submit_btn.cget("state"), tk.NORMAL)
        self.assertEqual(interrupt_btn.cget("state"), tk.DISABLED)
    
    def test_attachment_workflow(self):
        """Test attachment display workflow."""
        # Step 1: Display user message with attachments
        self.session.gui.display_user_message(
            "Please analyze this data",
            attachments=["data.csv", "report.pdf"],
            timestamp=datetime.now()
        )
        
        # Step 2: Verify attachments displayed
        output_text = self.session.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)
        
        self.assertIn("data.csv", content)
        self.assertIn("report.pdf", content)
        
        # Step 3: Display agent response
        self.session.gui.display_agent_response("Analysis complete")
        
        # Verify both attachments and response visible
        content = output_text.get("1.0", tk.END)
        self.assertIn("data.csv", content)
        self.assertIn("report.pdf", content)
        self.assertIn("Analysis complete", content)
    
    def test_error_handling_workflow(self):
        """Test error display workflow."""
        # Step 1: Display normal message
        self.session.gui.display_user_message(
            "Connect to server",
            attachments=[],
            timestamp=datetime.now()
        )
        
        # Step 2: Display error
        self.session.gui.display_error("Connection timeout")
        
        # Step 3: Continue with recovery
        self.session.gui.display_user_message(
            "Try again",
            attachments=[],
            timestamp=datetime.now()
        )
        
        # Verify all content visible
        output_text = self.session.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)
        
        self.assertIn("Connect to server", content)
        self.assertIn("ERROR", content)
        self.assertIn("Connection timeout", content)
        self.assertIn("Try again", content)
    
    def test_busy_state_workflow(self):
        """Test busy state transitions for non-streaming operations."""
        input_widget = self.session.gui.widgets.user_input_text
        submit_btn = self.session.gui.widgets.user_submit
        
        # Step 1: Start busy operation
        self.session.gui.set_busy_state(True)
        
        # Both should be disabled
        self.assertEqual(input_widget.cget("state"), tk.DISABLED)
        self.assertEqual(submit_btn.cget("state"), tk.DISABLED)
        
        # Step 2: End busy operation
        self.session.gui.set_busy_state(False)
        
        # Both should be enabled
        self.assertEqual(input_widget.cget("state"), tk.NORMAL)
        self.assertEqual(submit_btn.cget("state"), tk.NORMAL)
    
    def test_window_title_update_workflow(self):
        """Test window title update during session."""
        # Step 1: Set initial title
        initial_title = "Test Session - 2026-02-02"
        self.session.gui.set_window_title(initial_title)
        self.assertEqual(self.root.title(), initial_title)
        
        # Step 2: Update title (e.g., after first message)
        updated_title = "Test Session - 2026-02-02 (1 message)"
        self.session.gui.set_window_title(updated_title)
        self.assertEqual(self.root.title(), updated_title)
        
        # Step 3: Update again
        updated_title2 = "Test Session - 2026-02-02 (2 messages)"
        self.session.gui.set_window_title(updated_title2)
        self.assertEqual(self.root.title(), updated_title2)
    
    def test_panel_refresh_workflow(self):
        """Test panel refresh workflow."""
        # Step 1: Get parent frames
        history_parent = self.session.gui.get_history_parent()
        context_parent = self.session.gui.get_context_parent()
        files_parent = self.session.gui.get_files_parent()
        
        self.assertIsNotNone(history_parent)
        self.assertIsNotNone(context_parent)
        self.assertIsNotNone(files_parent)
        
        # Step 2: Create test widgets
        history_widget = tk.Frame(self.root)
        tk.Label(history_widget, text="History Content").pack()
        
        context_widget = tk.Frame(self.root)
        tk.Label(context_widget, text="Context Content").pack()
        
        files_widget = tk.Frame(self.root)
        tk.Label(files_widget, text="Files Content").pack()
        
        # Step 3: Update panels
        self.session.gui.update_history_panel(history_widget)
        self.session.gui.update_context_panel(context_widget)
        self.session.gui.update_files_panel(files_widget)
        
        # Verify panels were updated
        self.assertIsNotNone(self.session.gui.widgets.system_status_history)
        self.assertIsNotNone(self.session.gui.widgets.system_status_context)
        self.assertIsNotNone(self.session.gui.widgets.system_status_files)


class TestGuiManagerRobustness(unittest.TestCase):
    """Test GUIManager robustness and error handling."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.root = tk.Tk()
        self.root.withdraw()
        
        config_dict = {
            "ollama_host": "localhost",
            "ollama_model": "test-model",
            "ollama_timeout": 30,
        }
        
        from agentx.gui_config import GUIConfig
        config = GUIConfig.from_dict(config_dict)
        
        self.gui = GUIManager(
            root=self.root,
            config=config,
            on_submit=MagicMock(),
            on_interrupt=MagicMock(),
            on_attachment_toggle=MagicMock()
        )
        
        self.gui.create_layout()
    
    def tearDown(self):
        """Clean up after tests."""
        try:
            self.root.destroy()
        except:
            pass
    
    def test_rapid_display_calls(self):
        """Test rapid display calls don't cause crashes."""
        for i in range(10):
            self.gui.display_user_message(f"Message {i}", [], datetime.now())
            self.gui.display_agent_thinking(f"Thinking {i}")
            self.gui.display_agent_response(f"Response {i}")
            self.gui.display_spacing()
    
    def test_rapid_state_changes(self):
        """Test rapid state changes don't cause crashes."""
        for i in range(10):
            self.gui.set_streaming_state(i % 2 == 0)
            self.gui.set_busy_state(i % 3 == 0)
    
    def test_long_input_handling(self):
        """Test handling of very long inputs."""
        long_text = "A" * 10000
        
        input_widget = self.gui.widgets.user_input_text
        input_widget.insert("1.0", long_text)
        
        result = self.gui.get_user_input()
        
        self.assertEqual(result, long_text)
        self.assertEqual(input_widget.get("1.0", tk.END).strip(), "")
    
    def test_special_characters_display(self):
        """Test display of special characters."""
        special_text = "Special: émojis 🎉 symbols @#$% unicode ñ"
        
        self.gui.display_user_message(special_text, [], datetime.now())
        
        output = self.gui.widgets.output_text.get("1.0", tk.END)
        self.assertIn("Special:", output)


if __name__ == '__main__':
    unittest.main()
