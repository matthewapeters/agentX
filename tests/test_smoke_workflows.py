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
from agentx.gui.gui_manager import GUIManager
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
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "mistral",
                "ollama_timeout": 30,
            },
            "agentix": {
                "server_url": "http://localhost:11434",
                "classify_prompts": False,
                "debug": False,
            }
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
    
    def test_context_conversion_for_agentix(self):
        """Test that AgentX context is correctly converted to SharedContext for Agentix."""
        # Step 1: Add messages to AgentX context
        msg1 = Message(role="user", content="Hello")
        msg1.enabled = True
        msg2 = Message(role="assistant", content="Hi there")
        msg2.enabled = True
        
        from datetime import datetime
        self.session.context.add_message(datetime.now(), msg1)
        self.session.context.add_message(datetime.now(), msg2)
        
        # Step 2: Convert to SharedContext (simulate what _stream_via_agentix does)
        from shared.models.context import Context as SharedContext
        from shared.models.message import Message as SharedMessage, MessageRole
        
        shared_context = SharedContext()
        
        # Convert context messages (simulate lines 493-503 of session.py)
        for _, msg in self.session.context.messages:
            if getattr(msg, "enabled", False):
                # This is the actual conversion code from session.py
                shared_msg = SharedMessage(
                    role=MessageRole[msg.role.upper()] if hasattr(MessageRole, msg.role.upper()) else MessageRole.USER,
                    content=msg.content,
                    enabled=msg.enabled
                )
                shared_context.add_message(shared_msg)
        
        # Step 3: Verify SharedContext has the messages
        shared_messages = list(shared_context.get_enabled_messages())
        self.assertEqual(len(shared_messages), 2)
        
        # Step 4: Verify messages have correct types
        for msg in shared_messages:
            self.assertIsInstance(msg.role, MessageRole)
            self.assertNotIsInstance(msg.role, dict)
            # MessageRole is a str Enum, so isinstance(msg.role, str) is True
            # Just verify it's the enum type we expect
            self.assertTrue(isinstance(msg.role.value, str))
            self.assertTrue(hasattr(msg, 'to_dict'))
        
        # Step 5: Verify to_dict() works
        for msg in shared_messages:
            msg_dict = msg.to_dict()
            self.assertIsInstance(msg_dict, dict)
            self.assertIn("role", msg_dict)
            self.assertIn("content", msg_dict)
            self.assertIsInstance(msg_dict["role"], str)  # Should be enum.value
    
    def test_multi_turn_conversation_context(self):
        """
        Test that multi-turn conversations maintain correct context.
        
        This tests the scenario where:
        1. User asks first question
        2. Assistant responds (message added to context)
        3. User asks second question
        4. Context from first turn should be correctly passed to second turn
        """
        from datetime import datetime
        
        # Turn 1: User asks a question
        msg1_user = Message(role="user", content="What is 2+2?")
        msg1_user.enabled = True
        self.session.context.add_message(datetime.now(), msg1_user)
        
        # Turn 1: Assistant responds
        msg1_assistant = Message(role="assistant", content="The answer is 4.")
        msg1_assistant.enabled = True
        self.session.context.add_message(datetime.now(), msg1_assistant)
        
        # Verify context has 2 messages
        self.assertEqual(len(self.session.context.messages), 2)
        
        # Turn 2: Convert to SharedContext (simulating second query)
        from shared.models.context import Context as SharedContext
        from shared.models.message import Message as SharedMessage, MessageRole
        
        shared_context = SharedContext()
        
        # This is the exact conversion code from session.py _stream_via_agentix
        for _, msg in self.session.context.messages:
            # Verify msg is a Message object, not a dict
            self.assertNotIsInstance(msg, dict, 
                f"Found dict in context.messages instead of Message object: {msg}")
            self.assertTrue(hasattr(msg, 'role'),
                f"Message object missing 'role' attribute: {type(msg)}")
            self.assertTrue(hasattr(msg, 'content'),
                f"Message object missing 'content' attribute: {type(msg)}")
            self.assertTrue(hasattr(msg, 'enabled'),
                f"Message object missing 'enabled' attribute: {type(msg)}")
            
            if getattr(msg, "enabled", False):
                # This should not raise AttributeError
                shared_msg = SharedMessage(
                    role=MessageRole[msg.role.upper()] if hasattr(MessageRole, msg.role.upper()) else MessageRole.USER,
                    content=msg.content,
                    enabled=msg.enabled
                )
                shared_context.add_message(shared_msg)
        
        # Verify SharedContext has both messages
        shared_messages = list(shared_context.get_enabled_messages())
        self.assertEqual(len(shared_messages), 2)
        
        # Verify all messages can be converted to dicts for Agentix
        for msg in shared_messages:
            msg_dict = msg.to_dict()
            self.assertIn("role", msg_dict)
            self.assertIn("content", msg_dict)
            self.assertIsInstance(msg_dict["role"], str)
    
    def test_thinking_and_response_display(self):
        """
        Test that thinking is displayed and responses are added to context.
        
        This addresses the user's concern: "thinking is not displayed, and message  
        from LLM not received or added to context."
        """
        from unittest.mock import Mock, patch, MagicMock
        from shared.models.response import ResponseChunk, ChunkType
        
        # Mock the Agentix adapter to return chunks
        with patch.object(self.session.agentix_adapter, 'process_prompt_generator') as mock_gen:
            # Simulate a response with thinking, content, and done
            def mock_stream(*args, **kwargs):
                yield ResponseChunk(type=ChunkType.THINKING, content="Let me think...")
                yield ResponseChunk(type=ChunkType.CONTENT, content="I am ")
                yield ResponseChunk(type=ChunkType.CONTENT, content="gpt-oss")
                yield ResponseChunk(type=ChunkType.DONE, content="")
            
            mock_gen.side_effect = mock_stream
            
            # Set up user input
            self.session.gui.widgets.user_input_text.insert("1.0", "what model are you?")
            
            # Capture display calls
            thinking_displayed = []
            content_displayed = []
            
            original_thinking = self.session.gui.display_agent_thinking
            original_content = self.session.gui.display_agent_response
            
            def capture_thinking(text):
                thinking_displayed.append(text)
                original_thinking(text)
            
            def capture_content(text):
                content_displayed.append(text)
                original_content(text)
            
            self.session.gui.display_agent_thinking = capture_thinking
            self.session.gui.display_agent_response = capture_content
            
            # Get initial context size
            initial_context_size = len(self.session.context.messages)
            
            # Trigger streaming
            self.session.stream_ollama_response_worker()
            
            # Verify thinking was displayed
            self.assertGreater(len(thinking_displayed), 0, 
                "Thinking should be displayed")
            self.assertTrue(any("Let me think" in t for t in thinking_displayed),
                "Thinking content should include 'Let me think...'")
            
            # Verify content was displayed
            self.assertGreater(len(content_displayed), 0,
                "Content should be displayed")
            full_content = "".join(content_displayed)
            self.assertIn("gpt-oss", full_content,
                "Response should include 'gpt-oss'")
            
            # Verify response was added to context
            final_context_size = len(self.session.context.messages)
            self.assertGreater(final_context_size, initial_context_size,
                "Response message should be added to context")
            
            # Verify the last message is the assistant's response
            _, last_msg = self.session.context.messages[-1]
            self.assertEqual(last_msg.role, "assistant",
                "Last message should be from assistant")
            self.assertIn("gpt-oss", last_msg.content,
                "Last message should contain the response content")
    
    def test_history_messages_are_not_dicts(self):
        """
        Test that messages loaded from history are Message objects, not dicts.
        
        This addresses the error: "'dict' object has no attribute 'role'"
        which suggests somewhere messages are being stored/loaded as dicts.
        """
        import tempfile
        import json
        from datetime import datetime
        
        # Create a temporary session directory
        with tempfile.TemporaryDirectory() as tmpdir:
            session_path = os.path.join(tmpdir, "test_session")
            os.makedirs(session_path)
            
            # Save a message to disk (simulating previous session)
            test_message_data = {
                "role": "user",
                "content": "test content",
                "enabled": True,
                "file": None,
                "epoch": datetime.now().timestamp(),
                "attachments": []
            }
            
            msg_file = os.path.join(session_path, "test_message.json")
            with open(msg_file, 'w') as f:
                json.dump(test_message_data, f)
            
            # Load messages from disk (simulating session restoration)
            test_context = Context()
            test_context.path = session_path
            test_context.load_messages()
            
            # Verify messages are Message objects, not dicts
            self.assertEqual(len(test_context.messages), 1)
            _, msg = test_context.messages[0]
            
            self.assertIsInstance(msg, Message,
                f"Message should be Message object, got {type(msg)}")
            self.assertNotIsInstance(msg, dict,
                "Message should not be a dict")
            self.assertTrue(hasattr(msg, 'role'),
                f"Message should have 'role' attribute")
            self.assertTrue(hasattr(msg, 'content'),
                f"Message should have 'content' attribute")
            
            # Verify we can access role as attribute (not dict key)
            self.assertEqual(msg.role, "user")
            self.assertEqual(msg.content, "test content")
            
            # Verify conversion to SharedMessage works
            from shared.models.message import Message as SharedMessage, MessageRole
            
            # This should not raise AttributeError
            shared_msg = SharedMessage(
                role=MessageRole[msg.role.upper()],
                content=msg.content,
                enabled=msg.enabled
            )
            self.assertIsInstance(shared_msg.role, MessageRole)
    
    def test_model_selection_updates_active_model(self):
        """
        Test that selecting a model in the dropdown updates session.active_model.
        
        This addresses the issue where selecting qwen3-coder shows (gpt-oss) in output.
        """
        # Verify initial model
        initial_model = self.session.active_model
        self.assertEqual(initial_model, "mistral")  # From config
        
        # Populate with multiple models
        models = [
            {"name": "mistral", "size": 1000},
            {"name": "qwen3-coder", "size": 2000},
            {"name": "llama3.2", "size": 3000},
        ]
        self.session.gui.populate_models(models, initial_model="mistral")
        
        # Setup the callback wrapper (this is what _setup_agentix_ui does)
        if self.session.gui.model_selector:
            original_callback = self.session.gui.model_selector.on_model_change
            def on_model_change(model: str):
                self.session.active_model = model
                original_callback(model)
            self.session.gui.model_selector.on_model_change = on_model_change
        
        # Simulate user selecting a different model
        # Get the model selector widget
        model_selector = self.session.gui.model_selector
        dropdown = model_selector.dropdown
        
        # Simulate selection change by directly calling _on_selection  
        # (event_generate doesn't work in tests)
        # Use the actual display name from _models
        qwen_display_name = [k for k in model_selector._models.keys() if 'qwen3-coder' in k][0]
        dropdown.set(qwen_display_name)
        model_selector._on_selection(None)  # Trigger the selection handler
        
        # Verify active_model was updated
        self.assertEqual(self.session.active_model, "qwen3-coder",
            "Session.active_model should be updated when model selector changes")
        
        # Verify config was updated
        self.assertEqual(self.session.config["agentx"]["ollama_model"], "qwen3-coder",
            "Config should be updated when model selector changes")
        
        # Verify agentix config was updated
        self.assertEqual(self.session.agentix_adapter.agentix_config.model, "qwen3-coder",
            "Agentix config should be updated when model selector changes")
    
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
        
        from agentx.gui.gui_config import GUIConfig
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
