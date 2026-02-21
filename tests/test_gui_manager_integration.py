"""Integration tests for GUIManager functionality.

Tests the GUIManager's core display and state management operations
to ensure proper separation of concerns and clean interface.
"""

import tkinter as tk
import unittest
from unittest.mock import MagicMock, patch
from datetime import datetime
import sys
import os

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from agentx.gui.gui_manager import GUIManager
from agentx.gui.gui_config import GUIConfig


class TestGUIManagerInitialization(unittest.TestCase):
    """Test GUIManager initialization and setup."""
    
    def setUp(self):
        """Set up test fixtures."""
        self.root = tk.Tk()
        self.root.withdraw()  # Hide window during tests
        
        config_dict = {
            "ollama_host": "localhost",
            "ollama_model": "test-model",
            "ollama_timeout": 30,
        }
        self.config = GUIConfig.from_dict(config_dict)
        
        # Mock callbacks
        self.on_submit = MagicMock()
        self.on_interrupt = MagicMock()
        self.on_attachment_toggle = MagicMock()
    
    def tearDown(self):
        """Clean up after tests."""
        try:
            self.root.destroy()
        except:
            pass
    
    def test_gui_manager_creates_successfully(self):
        """Test that GUIManager initializes without errors."""
        gui = GUIManager(
            root=self.root,
            config=self.config,
            on_submit=self.on_submit,
            on_interrupt=self.on_interrupt,
            on_attachment_toggle=self.on_attachment_toggle
        )
        
        self.assertIsNotNone(gui)
        self.assertEqual(gui.root, self.root)
        self.assertEqual(gui.config, self.config)
    
    def test_gui_manager_stores_callbacks(self):
        """Test that GUIManager properly stores callback references."""
        gui = GUIManager(
            root=self.root,
            config=self.config,
            on_submit=self.on_submit,
            on_interrupt=self.on_interrupt,
            on_attachment_toggle=self.on_attachment_toggle
        )
        
        self.assertEqual(gui._on_submit, self.on_submit)
        self.assertEqual(gui._on_interrupt, self.on_interrupt)
        self.assertEqual(gui._on_attachment_toggle, self.on_attachment_toggle)
    
    def test_widget_registry_created(self):
        """Test that WidgetRegistry is properly initialized."""
        gui = GUIManager(
            root=self.root,
            config=self.config,
            on_submit=self.on_submit,
            on_interrupt=self.on_interrupt,
            on_attachment_toggle=self.on_attachment_toggle
        )
        
        self.assertIsNotNone(gui.widgets)
        # Widgets should start as None before layout
        self.assertIsNone(gui.widgets.output_text)

    def test_session_section_spacing_default(self):
        """Session section spacing should default to configured value."""
        gui = GUIManager(
            root=self.root,
            config=self.config,
            on_submit=self.on_submit,
            on_interrupt=self.on_interrupt,
            on_attachment_toggle=self.on_attachment_toggle
        )
        self.assertEqual(gui._session_section_spacing, 8)


class TestGUIManagerDisplayMethods(unittest.TestCase):
    """Test display methods of GUIManager."""
    
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
        
        # Create layout so widgets exist
        self.gui.create_layout()
    
    def tearDown(self):
        """Clean up after tests."""
        try:
            self.root.destroy()
        except:
            pass
    
    def test_display_user_message(self):
        """Test displaying user message."""
        self.gui.display_user_message(
            "Hello agent",
            attachments=[],
            timestamp=datetime.now()
        )
        
        output_text = self.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)
        
        self.assertIn("User:", content)
        self.assertIn("Hello agent", content)
    
    def test_display_user_message_with_attachments(self):
        """Test displaying user message with attachments."""
        self.gui.display_user_message(
            "Check this file",
            attachments=["test.txt", "data.csv"],
            timestamp=datetime.now()
        )
        
        output_text = self.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)
        
        self.assertIn("User:", content)
        self.assertIn("test.txt", content)
        self.assertIn("data.csv", content)
    
    def test_display_agent_thinking(self):
        """Test displaying agent thinking."""
        self.gui.display_agent_thinking("Analyzing the request...")
        
        output_text = self.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)
        
        self.assertIn("Agent is thinking", content)
        self.assertIn("Analyzing the request", content)
    
    def test_display_agent_response(self):
        """Test displaying agent response."""
        self.gui.display_agent_response("Here's the answer...")
        
        output_text = self.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)
        
        self.assertIn("Agent:", content)
        self.assertIn("Here's the answer", content)
    
    def test_display_error(self):
        """Test displaying error message."""
        self.gui.display_error("Connection failed")
        
        output_text = self.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)
        
        self.assertIn("ERROR", content)
        self.assertIn("Connection failed", content)
    
    def test_display_spacing(self):
        """Test displaying spacing."""
        # Add some content first
        self.gui.display_user_message("Test", attachments=[], timestamp=datetime.now())
        
        # Add spacing
        self.gui.display_spacing()
        
        output_text = self.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)
        
        # Should have extra newlines
        self.assertIn("\n\n", content)


class TestGUIManagerStateMethods(unittest.TestCase):
    """Test state management methods of GUIManager."""
    
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
        
        # Create layout
        self.gui.create_layout()
    
    def tearDown(self):
        """Clean up after tests."""
        try:
            self.root.destroy()
        except:
            pass
    
    def test_set_window_title(self):
        """Test setting window title."""
        title = "Test Title - Session 2026-01-01"
        self.gui.set_window_title(title)
        
        self.assertEqual(self.root.title(), title)
    
    def test_set_streaming_state_true(self):
        """Test setting streaming state to active."""
        self.gui.set_streaming_state(True)
        
        submit_btn = self.gui.widgets.user_submit
        interrupt_btn = self.gui.widgets.user_break
        
        # Submit should be disabled, interrupt should be enabled
        self.assertEqual(submit_btn.cget("state"), tk.DISABLED)
        self.assertEqual(interrupt_btn.cget("state"), tk.NORMAL)
    
    def test_set_streaming_state_false(self):
        """Test setting streaming state to idle."""
        self.gui.set_streaming_state(True)  # First set to streaming
        self.gui.set_streaming_state(False)  # Then set to idle
        
        submit_btn = self.gui.widgets.user_submit
        interrupt_btn = self.gui.widgets.user_break
        
        # Submit should be enabled, interrupt should be disabled
        self.assertEqual(submit_btn.cget("state"), tk.NORMAL)
        self.assertEqual(interrupt_btn.cget("state"), tk.DISABLED)
    
    def test_set_busy_state_true(self):
        """Test setting busy state to true."""
        self.gui.set_busy_state(True)
        
        input_text = self.gui.widgets.user_input_text
        submit_btn = self.gui.widgets.user_submit
        
        # Both should be disabled
        self.assertEqual(input_text.cget("state"), tk.DISABLED)
        self.assertEqual(submit_btn.cget("state"), tk.DISABLED)
    
    def test_set_busy_state_false(self):
        """Test setting busy state to false."""
        self.gui.set_busy_state(True)  # First set to busy
        self.gui.set_busy_state(False)  # Then set to not busy
        
        input_text = self.gui.widgets.user_input_text
        submit_btn = self.gui.widgets.user_submit
        
        # Both should be enabled
        self.assertEqual(input_text.cget("state"), tk.NORMAL)
        self.assertEqual(submit_btn.cget("state"), tk.NORMAL)


class TestGUIManagerInputMethods(unittest.TestCase):
    """Test input handling methods of GUIManager."""
    
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
        
        # Create layout
        self.gui.create_layout()
    
    def tearDown(self):
        """Clean up after tests."""
        try:
            self.root.destroy()
        except:
            pass
    
    def test_get_user_input(self):
        """Test retrieving user input."""
        input_text = self.gui.widgets.user_input_text
        input_text.insert("1.0", "Hello, Agent!")
        
        result = self.gui.get_user_input()
        
        self.assertEqual(result, "Hello, Agent!")
        # Input should be cleared after retrieval
        remaining = input_text.get("1.0", tk.END).strip()
        self.assertEqual(remaining, "")
    
    def test_get_user_input_with_whitespace(self):
        """Test that get_user_input strips whitespace."""
        input_text = self.gui.widgets.user_input_text
        input_text.insert("1.0", "  Hello  \n  ")
        
        result = self.gui.get_user_input()
        
        self.assertEqual(result, "Hello")
    
    def test_clear_user_input(self):
        """Test clearing user input."""
        input_text = self.gui.widgets.user_input_text
        input_text.insert("1.0", "Some text")
        
        self.gui.clear_user_input()
        
        remaining = input_text.get("1.0", tk.END).strip()
        self.assertEqual(remaining, "")


class TestGUIManagerPanelMethods(unittest.TestCase):
    """Test panel management methods of GUIManager."""
    
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
        
        # Create layout
        self.gui.create_layout()
    
    def tearDown(self):
        """Clean up after tests."""
        try:
            self.root.destroy()
        except:
            pass
    
    def test_get_history_parent(self):
        """Test getting history parent frame."""
        parent = self.gui.get_history_parent()
        
        self.assertIsNotNone(parent)
        self.assertIsInstance(parent, tk.Frame)
    
    def test_get_context_parent(self):
        """Test getting context parent frame."""
        parent = self.gui.get_context_parent()
        
        self.assertIsNotNone(parent)
        self.assertIsInstance(parent, tk.Frame)
    
    def test_get_files_parent(self):
        """Test getting files parent frame."""
        parent = self.gui.get_files_parent()
        
        self.assertIsNotNone(parent)
        self.assertIsInstance(parent, tk.Frame)
    
    def test_update_history_panel(self):
        """Test updating history panel."""
        # Create a test widget
        test_frame = tk.Frame(self.root)
        test_label = tk.Label(test_frame, text="Test History")
        test_label.pack()
        
        # Update panel
        self.gui.update_history_panel(test_frame)
        
        # Verify panel was updated (widget should exist)
        history_parent = self.gui.get_history_parent()
        self.assertIsNotNone(history_parent)
    
    def test_update_context_panel(self):
        """Test updating context panel."""
        # Create a test widget
        test_frame = tk.Frame(self.root)
        test_label = tk.Label(test_frame, text="Test Context")
        test_label.pack()
        
        # Update panel
        self.gui.update_context_panel(test_frame)
        
        # Verify panel was updated
        context_parent = self.gui.get_context_parent()
        self.assertIsNotNone(context_parent)
    
    def test_update_files_panel(self):
        """Test updating files panel."""
        # Create a test widget
        test_frame = tk.Frame(self.root)
        test_label = tk.Label(test_frame, text="Test Files")
        test_label.pack()
        
        # Update panel
        self.gui.update_files_panel(test_frame)
        
        # Verify panel was updated
        files_parent = self.gui.get_files_parent()
        self.assertIsNotNone(files_parent)

    def test_session_sections_have_required_order(self):
        """Session tab should host sections in History, Available Tools, Context order."""
        section_keys = list(self.gui._session_sections.keys())
        self.assertEqual(section_keys, ["history", "tools", "context"])

    def test_session_sections_start_collapsed(self):
        """Session sections should be collapsed initially at application start."""
        self.assertFalse(self.gui._session_sections["history"].is_expanded())
        self.assertFalse(self.gui._session_sections["tools"].is_expanded())
        self.assertFalse(self.gui._session_sections["context"].is_expanded())


if __name__ == '__main__':
    unittest.main()
