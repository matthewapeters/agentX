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
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

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
            on_attachment_toggle=self.on_attachment_toggle,
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
            on_attachment_toggle=self.on_attachment_toggle,
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
            on_attachment_toggle=self.on_attachment_toggle,
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
            on_attachment_toggle=self.on_attachment_toggle,
        )
        self.assertEqual(gui._session_section_spacing, 8)

    def test_gui_config_defaults_to_dark_mode(self):
        """GUIConfig should default to dark mode palette."""
        self.assertEqual(self.config.theme_mode, "Dark Mode")
        self.assertEqual(self.config.output_bg, "#222222")
        self.assertEqual(self.config.agent_response_fg, "#eeeeee")

    def test_gui_config_supports_light_mode_palette(self):
        """GUIConfig should switch colors when light mode is requested."""
        cfg = GUIConfig.from_dict({"agentx": {"theme_mode": "Light Mode"}})

        self.assertEqual(cfg.theme_mode, "Light Mode")
        self.assertEqual(cfg.output_bg, "#ffffff")
        self.assertEqual(cfg.agent_response_fg, "#111827")


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
            on_attachment_toggle=MagicMock(),
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
        self.gui.display_user_message("Hello agent", attachments=[], timestamp=datetime.now())

        output_text = self.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)

        self.assertIn("User:", content)
        self.assertIn("Hello agent", content)

    def test_display_user_message_with_attachments(self):
        """Test displaying user message with attachments."""
        self.gui.display_user_message("Check this file", attachments=["test.txt", "data.csv"], timestamp=datetime.now())

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

        self.assertIn("Analyzing the request", content)

    def test_display_agent_response(self):
        """Test displaying agent response."""
        self.gui.display_agent_response("Here's the answer...")

        output_text = self.gui.widgets.output_text
        content = output_text.get("1.0", tk.END)

        self.assertIn("Here's the answer", content)

    def test_display_bootstrap_agent_response(self):
        """Bootstrap responses should render without a visible user prompt."""
        self.gui.display_bootstrap_agent_response("Hello from bootstrap")

        content = self.gui.widgets.output_text.get("1.0", tk.END)
        self.assertIn("Hello from bootstrap", content)
        self.assertNotIn("User:", content)

    def test_output_text_surface_is_visible_for_copy(self):
        """Output text mirror should be packed and readable for copy operations."""
        self.assertEqual(self.gui.widgets.output_text.winfo_manager(), "pack")
        self.assertEqual(self.gui.widgets.output_scrollbar.winfo_manager(), "pack")

    def test_output_text_is_read_only_but_selectable(self):
        """Output mirror should block typing while still allowing selection actions."""
        output = self.gui.widgets.output_text
        self.assertTrue(output.bind("<Key>"))
        self.assertTrue(output.bind("<Control-a>"))
        self.assertTrue(output.bind("<Control-c>"))

    def test_output_text_has_copy_shortcuts_bound(self):
        """Output text mirror should bind select-all and copy shortcuts."""
        output = self.gui.widgets.output_text
        self.assertTrue(output.bind("<Control-a>"))
        self.assertTrue(output.bind("<Control-c>"))

    def test_output_text_select_all_handler_creates_selection(self):
        """Select-all handler should create a real selection range in output text."""
        output = self.gui.widgets.output_text
        self.gui.display_agent_response("Mouse selection smoke test text")

        self.gui._select_all_output_text()

        sel_ranges = output.tag_ranges(tk.SEL)
        self.assertTrue(sel_ranges)

    def test_output_entries_wraplength_updates_with_resize(self):
        """Structured output labels should re-wrap as output width changes."""
        self.gui.display_user_message("A sample prompt", attachments=[], timestamp=datetime.now())
        self.gui.display_agent_response("A long response that should reflow when width changes.")

        self.gui._update_output_wraplength(320)
        self.assertTrue(self.gui._output_wrapped_labels)
        for label in self.gui._output_wrapped_labels:
            self.assertEqual(int(label.cget("wraplength")), 280)

        self.gui._update_output_wraplength(800)
        for label in self.gui._output_wrapped_labels:
            self.assertEqual(int(label.cget("wraplength")), 760)

    def test_user_collapse_hides_children_entries(self):
        """Collapsing user row should hide thinking/classification/tool child rows."""
        self.gui.display_user_message("Prompt", attachments=[], timestamp=datetime.now())
        self.gui.display_agent_thinking("(The agent is thinking...)")
        self.gui.display_agent_thinking("Reasoning details")
        self.gui.display_classification(
            {
                "intent": "simple_action",
                "next_step": "single_tool",
                "reasoning_summary": "One step",
                "needs_clarification": False,
                "missing_fields": [],
            }
        )
        self.gui.display_agent_response("[🔧 Calling tool: read_file]")

        user_entry = self.gui._current_turn_entries["user"]
        children_frame = self.gui._current_turn_children_frame
        self.assertIsNotNone(children_frame)
        self.assertEqual(children_frame.winfo_manager(), "pack")

        user_entry["toggle"]()  # collapse
        self.assertEqual(children_frame.winfo_manager(), "")

        user_entry["toggle"]()  # expand
        self.assertEqual(children_frame.winfo_manager(), "pack")

    def test_header_preview_not_driven_by_newline(self):
        """Header preview condenses newlines into spaces.

        GIVEN a user message containing embedded newlines
        WHEN the message is displayed
        THEN the header preview must replace newlines with spaces and show all
             words as a single line (the preview word-count limit is 15 words;
             a short message is never truncated regardless of panel width).
        """
        self.gui.display_user_message("Line one\nLine two\nLine three", attachments=[], timestamp=datetime.now())
        user_entry = self.gui._current_turn_entries["user"]
        header = user_entry["header_var"].get()

        # Newlines must be condensed into spaces — the full phrase must appear.
        self.assertIn("Line one Line two Line three", header)

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

    def test_display_classification_shows_all_fields(self):
        """Classification block renders intent, reasoning, clarification, and path."""
        self.gui.display_classification(
            {
                "intent": "simple_action",
                "next_step": "invoke_planner",
                "reasoning_summary": "Multi-step refactor needed.",
                "needs_clarification": True,
                "missing_fields": ["target_dir", "language"],
            }
        )

        content = self.gui.widgets.output_text.get("1.0", tk.END)

        self.assertIn("simple_action", content)
        self.assertIn("invoke_planner", content)
        self.assertIn("Multi-step refactor needed.", content)
        self.assertIn("clarification needed: yes", content)
        self.assertIn("target_dir", content)
        self.assertIn("language", content)

    def test_display_classification_suppresses_falsy_clarification(self):
        """Clarification line is absent when needs_clarification is False and missing_fields is empty."""
        self.gui.display_classification(
            {
                "intent": "conversation",
                "next_step": "respond_directly",
                "reasoning_summary": "Simple chat.",
                "needs_clarification": False,
                "missing_fields": [],
            }
        )

        content = self.gui.widgets.output_text.get("1.0", tk.END)

        self.assertIn("conversation", content)
        self.assertIn("respond_directly", content)
        self.assertNotIn("clarification needed", content)
        self.assertNotIn("missing fields", content)

    def test_display_classification_shown_only_once_per_turn(self):
        """display_classification is a one-shot per turn — second call is ignored."""
        classification = {
            "intent": "simple_action",
            "next_step": "single_tool",
            "reasoning_summary": "One tool needed.",
            "needs_clarification": False,
            "missing_fields": [],
        }
        self.gui.display_classification(classification)
        self.gui.display_classification(classification)

        content = self.gui.widgets.output_text.get("1.0", tk.END)
        # "simple_action" should appear exactly once
        self.assertEqual(content.count("simple_action"), 1)

    def test_display_spacing_resets_classification_state(self):
        """display_spacing resets state so classification can show again on next turn."""
        classification = {
            "intent": "simple_action",
            "next_step": "single_tool",
            "reasoning_summary": "One tool needed.",
            "needs_clarification": False,
            "missing_fields": [],
        }
        self.gui.display_classification(classification)
        self.gui.display_spacing()
        self.assertFalse(self.gui._agent_classification_shown)


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
            on_attachment_toggle=MagicMock(),
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


class TestGUIManagerSettingsTheme(unittest.TestCase):
    """Test theme setting presence in the GUI settings panel."""

    def setUp(self):
        self.root = tk.Tk()
        self.root.withdraw()
        self.config_dict = {
            "agentx": {
                "theme_mode": "Light Mode",
                "ollama_host": "localhost:11434",
                "ollama_model": "test-model",
            },
            "agentix": {
                "host": "localhost:8000",
            },
        }
        self.config = GUIConfig.from_dict(self.config_dict)
        self.gui = GUIManager(
            root=self.root,
            config=self.config,
            on_submit=MagicMock(),
            on_interrupt=MagicMock(),
            on_attachment_toggle=MagicMock(),
        )
        self.gui.create_layout()

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_settings_tab_reflects_theme_mode(self):
        self.gui.render_settings_tab(
            config=self.config_dict,
            on_change=MagicMock(),
            models=[],
            system_prompts_dir="system_prompts",
        )

        self.assertEqual(self.gui._settings_tab_widget._theme_mode_var.get(), "Light Mode")


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
            on_attachment_toggle=MagicMock(),
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
            on_attachment_toggle=MagicMock(),
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
        self.assertEqual(section_keys, ["history", "tools", "working_memory", "context"])

    def test_session_sections_start_collapsed(self):
        """Session sections should be collapsed initially at application start."""
        self.assertFalse(self.gui._session_sections["history"].is_expanded())
        self.assertFalse(self.gui._session_sections["tools"].is_expanded())
        self.assertFalse(self.gui._session_sections["context"].is_expanded())


if __name__ == "__main__":
    unittest.main()
