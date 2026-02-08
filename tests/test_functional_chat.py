"""
Functional test for complete chat workflow.

Tests the exact user scenario:
1. User asks a question
2. System processes through Agentix
3. Response is displayed and added to context
"""

import tkinter as tk
import unittest
from unittest.mock import Mock, patch, MagicMock
import sys
import os
import tempfile
import shutil
from pathlib import Path

# Set AGENTIX_HOME before any imports
PROJECT_ROOT = Path(__file__).resolve().parent.parent
os.environ["AGENTIX_HOME"] = str(PROJECT_ROOT)

# Add src to path
sys.path.insert(0, str(PROJECT_ROOT / "src"))

from agentx.session import AgentXSession
from shared.models.response import ResponseChunk, ChunkType


class TestChatWorkflow(unittest.TestCase):
    """Test complete chat workflow through Agentix."""
    
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
            "USERNAME": "testuser",
            "AGENTIX_HOME": str(PROJECT_ROOT)
        }.get(key, os.environ.get(key, default))
        
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
    
    def test_chat_converts_context_correctly(self):
        """
        Test that AgentX context is converted to SharedContext correctly.
        
        This tests the specific code path that was breaking:
        - session.py creates SharedContext from AgentX Context
        - AgentX messages (with string role) converted to SharedMessage (with MessageRole enum)
        - Passed to Agentix adapter correctly
        """
        config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "test-model",
                "ollama_timeout": 30,
            },
            "agentix": {
                "server_url": "http://localhost:11434",
                "classify_prompts": False,
                "debug": False,
                "available_tools": [],
            }
        }
        
        # Mock Agentix adapter to track what context it receives
        with patch('agentx.integration.agentix_bridge_adapter.create_adapter') as mock_create:
            mock_adapter = MagicMock()
            mock_adapter.get_models.return_value = [
                {"name": "test-model", "size": 1000}
            ]
            
            # Mock the streaming response
            def mock_stream(*args, **kwargs):
                """Mock streaming response with content chunks."""
                yield ResponseChunk(type=ChunkType.CONTENT, content="I")
                yield ResponseChunk(type=ChunkType.CONTENT, content=" am")
                yield ResponseChunk(type=ChunkType.CONTENT, content=" test-model")
                yield ResponseChunk(type=ChunkType.DONE, content="")
            
            mock_adapter.process_prompt_generator.side_effect = mock_stream
            mock_create.return_value = mock_adapter
            
            # Create session
            session = AgentXSession(root=self.root, config=config)
            session.gui.create_layout()
            
            # Add a message to context (simulating previous conversation)
            from agentx.message import Message
            prev_message = Message(role="assistant", content="Previous response")
            prev_message.enabled = True
            session.add_message_to_context(prev_message)
            
            # Submit a query
            session.gui.widgets.user_input_text.insert("1.0", "what model are you?")
            
            # Trigger streaming (in thread-safe way for test)
            import threading
            error = []
            
            def run_stream():
                try:
                    session.stream_ollama_response_worker()
                except Exception as e:
                    error.append(str(e))
                    import traceback
                    traceback.print_exc()
            
            thread = threading.Thread(target=run_stream)
            thread.start()
            thread.join(timeout=5)
            
            # Check for errors
            if error:
                self.fail(f"Chat workflow failed: {error[0]}")
            
            # Verify adapter was called
            self.assertTrue(mock_adapter.process_prompt_generator.called)
            
            # Verify the context passed to adapter
            call_args = mock_adapter.process_prompt_generator.call_args
            prompt = call_args[0][0]
            context = call_args[0][1]
            
            self.assertEqual(prompt, "what model are you?")
            
            # Context should be SharedContext with messages converted correctly
            from shared.models.context import Context as SharedContext
            self.assertIsInstance(context, SharedContext)
            
            # Should have previous message in context
            messages = list(context.get_enabled_messages())
            self.assertGreater(len(messages), 0)
            
            # Messages should have MessageRole enum
            for msg in messages:
                from shared.models.message import MessageRole
                self.assertIsInstance(msg.role, MessageRole)
                # Should NOT be a dict or string
                self.assertNotIsInstance(msg.role, dict)
                self.assertNotIsInstance(msg.role, str)
            
            # Verify response was added to context
            self.assertGreater(len(session.context.messages), 1)


    def test_empty_context_chat(self):
        """Test chat with no previous context (first message)."""
        config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "test-model",
                "ollama_timeout": 30,
            },
            "agentix": {
                "server_url": "http://localhost:11434",
                "classify_prompts": False,
                "debug": False,
                "available_tools": [],
            }
        }
        
        with patch('agentx.integration.agentix_bridge_adapter.create_adapter') as mock_create:
            mock_adapter = MagicMock()
            mock_adapter.get_models.return_value = []
            
            def mock_stream(*args, **kwargs):
                yield ResponseChunk(type=ChunkType.CONTENT, content="Hello!")
                yield ResponseChunk(type=ChunkType.DONE, content="")
            
            mock_adapter.process_prompt_generator.side_effect = mock_stream
            mock_create.return_value = mock_adapter
            
            session = AgentXSession(root=self.root, config=config)
            session.gui.create_layout()
            
            # Submit first query (no previous context)
            session.gui.widgets.user_input_text.insert("1.0", "hello")
            
            import threading
            def run_stream():
                session.stream_ollama_response_worker()
            
            thread = threading.Thread(target=run_stream)
            thread.start()
            thread.join(timeout=5)
            
            # Verify adapter was called with empty context
            self.assertTrue(mock_adapter.process_prompt_generator.called)
            
            call_args = mock_adapter.process_prompt_generator.call_args
            context = call_args[0][1]
            
            # New context should have no enabled messages
            messages = list(context.get_enabled_messages())
            # Should be empty (no previous messages)
            self.assertEqual(len(messages), 0)


if __name__ == '__main__':
    unittest.main()
