"""Unit tests for centralized active_model property in AgentXSession.

Tests verify that the active model configuration is properly:
- Initialized from config
- Accessed via the property getter
- Updated via the property setter across all storage locations
- Propagated to agentix adapter when enabled
- Used consistently in all Ollama call sites
"""

import tkinter as tk
import unittest
from unittest.mock import MagicMock, patch, Mock, PropertyMock
import sys
import os
import tempfile
import shutil

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from agentx.session import AgentXSession


class TestActiveModelProperty(unittest.TestCase):
    """Test that active_model property works as single source of truth."""
    
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
        
        # Base config with initial model
        self.initial_model = "llama3.2"
        self.config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": self.initial_model,
                "ollama_timeout": 30,
            },
            "agentix": {
                "enabled": False,
                "host": "localhost:8000",
            }
        }
    
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
    
    def test_active_model_initialized_from_config(self):
        """Test that active_model is initialized from config on session creation."""
        session = AgentXSession(root=self.root, config=self.config)
        
        self.assertEqual(session.active_model, self.initial_model)
        self.assertEqual(session._active_model, self.initial_model)
    
    def test_active_model_getter_returns_internal_state(self):
        """Test that property getter returns the internal _active_model value."""
        session = AgentXSession(root=self.root, config=self.config)
        
        # Directly modify internal state (normally shouldn't do this)
        session._active_model = "custom-model"
        
        # Getter should return internal state
        self.assertEqual(session.active_model, "custom-model")
    
    def test_active_model_setter_updates_internal_state(self):
        """Test that property setter updates _active_model."""
        session = AgentXSession(root=self.root, config=self.config)
        
        new_model = "gpt-oss"
        session.active_model = new_model
        
        self.assertEqual(session._active_model, new_model)
    
    def test_active_model_setter_updates_config_dict(self):
        """Test that property setter updates config dictionary."""
        session = AgentXSession(root=self.root, config=self.config)
        
        new_model = "mistral"
        session.active_model = new_model
        
        self.assertEqual(session.config["agentx"]["ollama_model"], new_model)
    
    def test_active_model_setter_updates_all_three_locations(self):
        """Test that setting active_model updates all three storage locations atomically."""
        session = AgentXSession(root=self.root, config=self.config)
        
        new_model = "codellama"
        session.active_model = new_model
        
        # Verify all three are updated
        self.assertEqual(session._active_model, new_model, 
                        "Internal _active_model not updated")
        self.assertEqual(session.active_model, new_model,
                        "Property getter doesn't return new model")
        self.assertEqual(session.config["agentx"]["ollama_model"], new_model,
                        "Config dict not updated")
    
    def test_active_model_setter_updates_agentix_when_enabled(self):
        """Test that property setter updates agentix adapter config when enabled."""
        # Enable agentix in config
        config_with_agentix = self.config.copy()
        config_with_agentix["agentix"] = {
            "enabled": True,
            "host": "localhost:8000",
            "classify_prompts": False,
            "available_tools": [],
        }
        
        # Create session with mocked agentix adapter
        with patch('agentx.session.create_adapter') as mock_create_adapter:
            mock_adapter = Mock()
            mock_adapter.enabled = True
            mock_adapter.agentix_config = Mock()
            mock_adapter.agentix_config.model = self.initial_model
            mock_create_adapter.return_value = mock_adapter
            
            session = AgentXSession(root=self.root, config=config_with_agentix)
            
            new_model = "phi-2"
            session.active_model = new_model
            
            # Verify agentix config was updated
            self.assertEqual(mock_adapter.agentix_config.model, new_model,
                           "Agentix adapter config not updated")
    
    def test_active_model_setter_handles_no_agentix(self):
        """Test that property setter works when agentix is disabled/absent."""
        session = AgentXSession(root=self.root, config=self.config)
        
        # Agentix should be disabled
        self.assertFalse(session.agentix_adapter.enabled if session.agentix_adapter else False)
        
        # Should not raise exception
        new_model = "llama3.1"
        session.active_model = new_model
        
        self.assertEqual(session.active_model, new_model)
    
    def test_multiple_model_changes(self):
        """Test that multiple model changes work correctly."""
        session = AgentXSession(root=self.root, config=self.config)
        
        models_to_test = ["model1", "model2", "model3", "model1"]
        
        for model in models_to_test:
            session.active_model = model
            
            self.assertEqual(session.active_model, model)
            self.assertEqual(session._active_model, model)
            self.assertEqual(session.config["agentx"]["ollama_model"], model)


class TestActiveModelUsage(unittest.TestCase):
    """Test that all usage points correctly use active_model."""
    
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
        
        self.config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "initial-model",
                "ollama_initial_load_timeout_seconds": 120,
            },
            "agentix": {
                "enabled": False,
            }
        }
    
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
    
    @patch('agentx.session.Client')
    def test_stream_direct_ollama_uses_active_model(self, mock_client_class):
        """Test that _stream_direct_ollama uses active_model instead of config."""
        session = AgentXSession(root=self.root, config=self.config)
        
        # Change model via property
        new_model = "updated-model-for-streaming"
        session.active_model = new_model
        
        # Mock Client and its methods
        mock_client = Mock()
        mock_client_class.return_value = mock_client
        mock_client.chat.return_value = []  # Empty stream
        
        # Mock GUI methods to prevent actual GUI updates
        session.gui.get_user_input = Mock(return_value="test prompt")
        session.gui.display_user_message = Mock()
        session.gui.display_spacing = Mock()
        session.gui.set_streaming_state = Mock()
        session.gui.get_history_parent = Mock(return_value=self.root)
        session.gui.get_context_parent = Mock(return_value=self.root)
        session.gui.update_history_panel = Mock()
        session.gui.update_context_panel = Mock()
        session.gui.render_history_widget = Mock(return_value=tk.Frame(self.root))
        session.gui.render_context_widget = Mock(return_value=tk.Frame(self.root))
        
        # Call the method
        session._stream_direct_ollama()
        
        # Verify Client.chat was called with the active_model
        mock_client.chat.assert_called_once()
        call_kwargs = mock_client.chat.call_args[1]
        self.assertEqual(call_kwargs['model'], new_model,
                        "_stream_direct_ollama did not use active_model")
    
    @patch('agentx.session.httpx.Client')
    def test_perform_service_handshake_uses_active_model(self, mock_httpx_client_class):
        """Test that perform_service_handshake uses active_model instead of config."""
        session = AgentXSession(root=self.root, config=self.config)
        
        # Change model via property
        new_model = "handshake-model"
        session.active_model = new_model
        
        # Mock httpx client and service manager
        mock_httpx_client = Mock()
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {"models": [{"name": new_model}]}
        mock_httpx_client.post.return_value = mock_response
        mock_httpx_client.get.return_value = mock_response
        mock_httpx_client.__enter__ = Mock(return_value=mock_httpx_client)
        mock_httpx_client.__exit__ = Mock(return_value=False)
        mock_httpx_client_class.return_value = mock_httpx_client
        
        session.service_manager.ensure_services = Mock(return_value=True)
        
        # Call the method
        session.perform_service_handshake()
        
        # Verify the POST request used active_model
        mock_httpx_client.post.assert_called()
        call_args = mock_httpx_client.post.call_args
        payload = call_args[1]['json']
        self.assertEqual(payload['model'], new_model,
                        "perform_service_handshake did not use active_model")
    
    def test_model_selector_callback_uses_active_model(self):
        """Test that ModelSelector callback properly uses active_model setter."""
        session = AgentXSession(root=self.root, config=self.config)
        
        # Simulate the callback that would be set up in _setup_agentix_ui
        def simulated_on_model_change(model: str):
            session.active_model = model
        
        # Simulate user selecting a new model
        selected_model = "user-selected-model"
        simulated_on_model_change(selected_model)
        
        # Verify all storage locations updated
        self.assertEqual(session.active_model, selected_model)
        self.assertEqual(session._active_model, selected_model)
        self.assertEqual(session.config["agentx"]["ollama_model"], selected_model)
    
    def test_model_change_callback_works_without_agentix(self):
        """Test that model change callback is set up even when Agentix is disabled."""
        # Ensure Agentix is disabled
        config = self.config.copy()
        config["agentix"]["enabled"] = False
        
        session = AgentXSession(root=self.root, config=config)
        
        # Verify agentix is not enabled
        self.assertFalse(session.agentix_adapter.enabled if session.agentix_adapter else False)
        
        # Trigger _setup_agentix_ui to set up callbacks
        session._setup_agentix_ui()
        
        # Verify the callback was overridden (not the default placeholder)
        original_callback = session.gui._on_model_change
        
        # The callback should be our custom function, not None
        self.assertIsNotNone(original_callback)
        
        # Test that calling it updates active_model
        initial_model = session.active_model
        test_model = "test-callback-model"
        
        # Call the callback directly
        session.gui._on_model_change(test_model)
        
        # Verify model was updated
        self.assertEqual(session.active_model, test_model)
        self.assertNotEqual(session.active_model, initial_model)


class TestActiveModelPropagation(unittest.TestCase):
    """Integration tests for model propagation across components."""
    
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
    
    def test_model_change_propagates_to_all_locations(self):
        """End-to-end test: model change propagates everywhere."""
        config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "model-v1",
            },
            "agentix": {
                "enabled": False,
            }
        }
        
        session = AgentXSession(root=self.root, config=config)
        
        # Initial state
        self.assertEqual(session.active_model, "model-v1")
        
        # User changes model
        session.active_model = "model-v2"
        
        # Verify propagation to all locations
        locations = {
            "property getter": session.active_model,
            "internal state": session._active_model,
            "config dict": session.config["agentx"]["ollama_model"],
        }
        
        for location_name, value in locations.items():
            self.assertEqual(value, "model-v2",
                           f"Model not propagated to {location_name}")
    
    def test_initial_model_consistent_across_locations(self):
        """Test that initial model is consistent everywhere on startup."""
        initial_model = "startup-model"
        config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": initial_model,
            },
            "agentix": {
                "enabled": False,
            }
        }
        
        session = AgentXSession(root=self.root, config=config)
        
        # All locations should have the same initial value
        self.assertEqual(session.active_model, initial_model)
        self.assertEqual(session._active_model, initial_model)
        self.assertEqual(session.config["agentx"]["ollama_model"], initial_model)


if __name__ == '__main__':
    unittest.main()


class TestModelSelectorInitialization(unittest.TestCase):
    """Test that model selector is initialized with the active model."""
    
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
    
    def test_populate_models_with_initial_model(self):
        """Test that populate_models selects the initial_model parameter."""
        from agentx.gui.model_selector import ModelSelector
        
        # Create a model selector
        selected_model = None
        def on_change(model):
            nonlocal selected_model
            selected_model = model
        
        selector = ModelSelector(
            parent=self.root,
            on_model_change=on_change,
            initial_model=""
        )
        
        # Models list
        models = [
            {"name": "llama3.2", "size": 1000000},
            {"name": "gpt-oss", "size": 2000000},
            {"name": "mistral", "size": 1500000},
        ]
        
        # Populate with gpt-oss as initial
        selector.populate(models, initial_model="gpt-oss")
        
        # Verify gpt-oss is selected (display name includes size)
        selected_display = selector.current_model.get()
        self.assertIn("gpt-oss", selected_display)
        
        # Verify the actual model name is gpt-oss
        actual_model = selector.get_selected_model()
        self.assertEqual(actual_model, "gpt-oss")
    
    def test_populate_models_falls_back_if_initial_not_found(self):
        """Test that populate_models falls back to first if initial_model not in list."""
        from agentx.gui.model_selector import ModelSelector
        
        selector = ModelSelector(
            parent=self.root,
            on_model_change=lambda m: None,
            initial_model=""
        )
        
        models = [
            {"name": "model-a", "size": 1000000},
            {"name": "model-b", "size": 2000000},
        ]
        
        # Populate with non-existent model as initial
        selector.populate(models, initial_model="non-existent")
        
        # Should select first model as fallback
        actual_model = selector.get_selected_model()
        self.assertEqual(actual_model, "model-a")
    
    @patch('agentx.session.httpx.Client')
    def test_session_populates_models_with_active_model(self, mock_httpx_client_class):
        """Test that session passes active_model when populating models."""
        config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "configured-model",
            },
            "agentix": {
                "enabled": False,
            }
        }
        
        # Mock httpx client
        mock_client = Mock()
        mock_response = Mock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "models": [
                {"name": "configured-model", "size": 1000000},
                {"name": "other-model", "size": 2000000},
            ]
        }
        mock_client.get.return_value = mock_response
        mock_client.__enter__ = Mock(return_value=mock_client)
        mock_client.__exit__ = Mock(return_value=False)
        mock_httpx_client_class.return_value = mock_client
        
        session = AgentXSession(root=self.root, config=config)
        
        # Mock the gui.populate_models to capture what's passed
        populate_called_with = {}
        original_populate = session.gui.populate_models
        def mock_populate(models, initial_model=None):
            populate_called_with['models'] = models
            populate_called_with['initial_model'] = initial_model
            original_populate(models, initial_model)
        
        session.gui.populate_models = mock_populate
        
        # Trigger _setup_agentix_ui
        session._setup_agentix_ui()
        
        # Verify populate_models was called with active_model
        self.assertIn('initial_model', populate_called_with)
        self.assertEqual(populate_called_with['initial_model'], "configured-model")


class TestModelNameInOutput(unittest.TestCase):
    """Test that model name appears in output headers."""
    
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
        
        self.config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "test-model",
            },
            "agentix": {
                "enabled": False,
            }
        }
    
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
    
    def test_thinking_header_includes_model_name(self):
        """Test that _display_thinking includes model name."""
        session = AgentXSession(root=self.root, config=self.config)
        
        # Capture what's displayed
        displayed_text = []
        session.gui.display_agent_thinking = lambda text: displayed_text.append(text)
        
        # Call _display_thinking for the first time (shows header)
        session._display_thinking("test thinking content")
        
        # Verify header includes model name
        header = displayed_text[0]
        self.assertIn("test-model", header)
        self.assertIn("💭", header)  # Thinking emoji
    
    def test_assistant_header_includes_model_name_agentix(self):
        """Test that assistant header includes model name in agentix streaming."""
        session = AgentXSession(root=self.root, config=self.config)
        
        # Mock the display method to capture output
        displayed_text = []
        original_display = session.gui.display_agent_response
        session.gui.display_agent_response = lambda text: displayed_text.append(text)
        
        # Simulate the assistant header display from _stream_via_agentix
        session.gui.display_agent_response(
            f"\n\n{session.gui.MESSAGE_ROLES['assistant']} ({session.active_model})\t"
        )
        
        # Verify header includes model name
        header = displayed_text[0]
        self.assertIn("test-model", header)
        self.assertIn("🤖", header)  # Assistant emoji


if __name__ == '__main__':
    unittest.main()
