"""Integration tests for AgentXSession and GUIManager integration.

Tests the interaction between AgentXSession business logic and
GUIManager presentation layer to ensure clean separation.
"""

import tkinter as tk
import unittest
from unittest.mock import MagicMock, patch, call
from datetime import datetime
import sys
import os
import tempfile
import shutil

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from agentx.session import AgentXSession
from agentx.gui.gui_manager import GUIManager
from agentx.gui.gui_config import GUIConfig


class TestAgentXSessionGUIIntegration(unittest.TestCase):
    """Test AgentXSession integration with GUIManager."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.root = tk.Tk()
        self.root.withdraw()
        
        # Create temporary directory for session
        self.temp_dir = tempfile.mkdtemp()
        
        # Patch os.getcwd to use temp directory
        self.patcher_getcwd = patch('os.getcwd')
        self.mock_getcwd = self.patcher_getcwd.start()
        self.mock_getcwd.return_value = self.temp_dir
        
        # Patch os.getenv for user
        self.patcher_getenv = patch('os.getenv')
        self.mock_getenv = self.patcher_getenv.start()
        self.mock_getenv.side_effect = lambda key, default=None: {
            "USER": "testuser",
            "USERNAME": "testuser"
        }.get(key, default)
        
        config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "mistral",
                "ollama_timeout": 30,
            },
            "agentix": {
                "host": "localhost:8000",
            }
        }
        
        self.session = AgentXSession(
            root=self.root,
            config=config
        )
    
    def tearDown(self):
        """Clean up after tests."""
        self.patcher_getcwd.stop()
        self.patcher_getenv.stop()
        
        try:
            self.root.destroy()
        except:
            pass
        
        # Clean up temp directory
        try:
            shutil.rmtree(self.temp_dir)
        except:
            pass
    
    def test_session_initializes_gui_manager(self):
        """Test that AgentXSession creates GUIManager."""
        self.assertIsNotNone(self.session.gui)
        self.assertIsInstance(self.session.gui, GUIManager)
    
    def test_gui_manager_stores_session_callbacks(self):
        """Test that GUIManager has callbacks from session."""
        self.assertEqual(
            self.session.gui._on_submit,
            self.session._handle_submit
        )
        self.assertEqual(
            self.session.gui._on_interrupt,
            self.session._handle_interrupt
        )
        self.assertEqual(
            self.session.gui._on_attachment_toggle,
            self.session._handle_attachment_toggle
        )
    
    def test_window_title_set_correctly(self):
        """Test that window title is set with session info."""
        title = self.root.title()
        
        self.assertIn("testuser", title)
        self.assertIn("AgentX Session", title)
        self.assertIn("2026", title)  # Year in timestamp
    
    def test_session_has_valid_configuration(self):
        """Test that session configuration is properly loaded."""
        self.assertIsNotNone(self.session.config)
        self.assertEqual(self.session.config["agentx"]["ollama_host"], "localhost:11434")
        self.assertEqual(self.session.config["agentx"]["ollama_model"], "mistral")
    
    def test_session_folders_created(self):
        """Test that session folders are created."""
        self.assertTrue(os.path.exists(self.session.session_folder))
        self.assertTrue(os.path.exists(self.session.context_folder))
    
    def test_handle_submit_callback_exists(self):
        """Test that submit callback is defined."""
        self.assertTrue(hasattr(self.session, '_handle_submit'))
        self.assertTrue(callable(self.session._handle_submit))
    
    def test_handle_interrupt_callback_exists(self):
        """Test that interrupt callback is defined."""
        self.assertTrue(hasattr(self.session, '_handle_interrupt'))
        self.assertTrue(callable(self.session._handle_interrupt))
    
    def test_handle_attachment_toggle_callback_exists(self):
        """Test that attachment toggle callback is defined."""
        self.assertTrue(hasattr(self.session, '_handle_attachment_toggle'))
        self.assertTrue(callable(self.session._handle_attachment_toggle))


class TestAgentXSessionGuiDelegation(unittest.TestCase):
    """Test that AgentXSession properly delegates to GUIManager."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.root = tk.Tk()
        self.root.withdraw()
        
        # Create temporary directory for session
        self.temp_dir = tempfile.mkdtemp()
        
        # Patch os.getcwd to use temp directory
        self.patcher_getcwd = patch('os.getcwd')
        self.mock_getcwd = self.patcher_getcwd.start()
        self.mock_getcwd.return_value = self.temp_dir
        
        # Patch os.getenv for user
        self.patcher_getenv = patch('os.getenv')
        self.mock_getenv = self.patcher_getenv.start()
        self.mock_getenv.side_effect = lambda key, default=None: {
            "USER": "testuser",
            "USERNAME": "testuser"
        }.get(key, default)
        
        config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "mistral",
                "ollama_timeout": 30,
            },
            "agentix": {
                "host": "localhost:8000",
            }
        }
        
        self.session = AgentXSession(
            root=self.root,
            config=config
        )
        
        # Create layout so widgets exist
        self.session.gui.create_layout()
    
    def tearDown(self):
        """Clean up after tests."""
        self.patcher_getcwd.stop()
        self.patcher_getenv.stop()
        
        try:
            self.root.destroy()
        except:
            pass
        
        # Clean up temp directory
        try:
            shutil.rmtree(self.temp_dir)
        except:
            pass
    
    def test_refresh_user_gui_method_exists(self):
        """Test that refresh_user_gui method exists."""
        self.assertTrue(hasattr(self.session, 'refresh_user_gui'))
        self.assertTrue(callable(self.session.refresh_user_gui))
    
    def test_refresh_context_gui_method_exists(self):
        """Test that refresh_context_gui method exists."""
        self.assertTrue(hasattr(self.session, 'refresh_context_gui'))
        self.assertTrue(callable(self.session.refresh_context_gui))
    
    def test_refresh_files_gui_method_exists(self):
        """Test that refresh_files_gui method exists."""
        self.assertTrue(hasattr(self.session, 'refresh_files_gui'))
        self.assertTrue(callable(self.session.refresh_files_gui))
    
    def test_session_message_attribute_exists(self):
        """Test that session has message attribute."""
        self.assertIsNotNone(self.session.message)
        self.assertEqual(self.session.message.role, "user")
    
    def test_session_context_attribute_exists(self):
        """Test that session has context attribute."""
        self.assertIsNotNone(self.session.context)
    
    def test_session_file_explorer_attribute_exists(self):
        """Test that session has file_explorer attribute."""
        self.assertIsNotNone(self.session.file_explorer)
    
    def test_attachment_toggle_method_callable(self):
        """Test that attachment toggle is callable."""
        # Try calling with dummy values
        try:
            self.session._handle_attachment_toggle("test-id", True)
        except Exception as e:
            # Should not raise any exception related to method not existing
            self.fail(f"_handle_attachment_toggle raised exception: {e}")


class TestGuiManagerCleanInterface(unittest.TestCase):
    """Test that GUIManager provides clean interface without GUI violations."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.root = tk.Tk()
        self.root.withdraw()
        
        config_dict = {
            "ollama_host": "localhost",
            "ollama_model": "test-model",
            "ollama_timeout": 30,
        }
        self.config = GUIConfig.from_dict(config_dict)
        
        self.gui = GUIManager(
            root=self.root,
            config=self.config,
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
    
    def test_display_methods_never_fail(self):
        """Test that all display methods handle errors gracefully."""
        # These should not raise exceptions even if called multiple times
        self.gui.display_user_message("Test", attachments=[], timestamp=datetime.now())
        self.gui.display_user_message("Test2", attachments=[], timestamp=datetime.now())
        
        self.gui.display_agent_thinking("Thinking...")
        self.gui.display_agent_thinking("More thinking...")
        
        self.gui.display_agent_response("Response...")
        self.gui.display_agent_response("More response...")
        
        self.gui.display_spacing()
        self.gui.display_spacing()
        
        self.gui.display_error("Error 1")
        self.gui.display_error("Error 2")
    
    def test_state_methods_idempotent(self):
        """Test that state methods can be called multiple times safely."""
        # These should not raise exceptions
        self.gui.set_streaming_state(True)
        self.gui.set_streaming_state(True)
        self.gui.set_streaming_state(False)
        self.gui.set_streaming_state(False)
        
        self.gui.set_busy_state(True)
        self.gui.set_busy_state(True)
        self.gui.set_busy_state(False)
        self.gui.set_busy_state(False)
    
    def test_window_title_method_works(self):
        """Test that window title method actually changes title."""
        original_title = self.root.title()
        
        new_title = "New Test Title"
        self.gui.set_window_title(new_title)
        
        self.assertEqual(self.root.title(), new_title)
        self.assertNotEqual(self.root.title(), original_title)
    
    def test_input_methods_handle_empty_input(self):
        """Test that input methods handle empty input gracefully."""
        # Input should be empty initially
        result = self.gui.get_user_input()
        self.assertEqual(result, "")
        
        # Clear empty input should not fail
        self.gui.clear_user_input()
        
        # Get again should still be empty
        result = self.gui.get_user_input()
        self.assertEqual(result, "")


if __name__ == '__main__':
    unittest.main()
