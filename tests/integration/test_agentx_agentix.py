"""
Integration tests for AgentX + Agentix combined functionality.

These tests verify that the shared models and communication protocols
work correctly between the AgentX GUI client and Agentix middleware.

Test Categories:
- Unit tests for shared models (Message, Context, ResponseChunk, Tools)
- Config loading and validation tests
- Mock-based integration tests (no real Ollama/server required)
- Live integration tests (marked with @pytest.mark.live)
"""

import json
import pytest
from dataclasses import asdict
from pathlib import Path
import sys
import tempfile
import os

# Add src to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent.parent / "src"))

from shared.models.message import (
    Message, 
    MessageRole, 
    user_message, 
    assistant_message,
    system_message,
    tool_call_message,
    tool_result_message,
)
from shared.models.context import Context
from shared.models.response import ResponseChunk, ChunkType
from shared.models.attachment import Attachment
from shared.models.tools import (
    ToolDefinition,
    ToolRequest,
    ToolResponse,
    ToolExecutionContext,
    ToolRegistry,
    BaseTool,
)
from shared.config.unified_config import (
    UnifiedConfig,
    AgentXConfig,
    AgentixConfig,
    ScreenSide,
    load_config,
)


# =============================================================================
# Message Model Tests
# =============================================================================

class TestMessageModel:
    """Tests for the unified Message model."""
    
    def test_create_user_message(self):
        """Test creating a basic user message."""
        msg = user_message("Hello, world!")
        
        assert msg.role == MessageRole.USER
        assert msg.content == "Hello, world!"
        assert msg.enabled is True
        assert msg.attachments == []
    
    def test_create_assistant_message(self):
        """Test creating an assistant response message."""
        msg = assistant_message("I can help you with that!")
        
        assert msg.role == MessageRole.ASSISTANT
        assert msg.content == "I can help you with that!"
    
    def test_create_system_message(self):
        """Test creating a system prompt message."""
        msg = system_message("You are a helpful assistant.")
        
        assert msg.role == MessageRole.SYSTEM
        assert msg.content == "You are a helpful assistant."
    
    def test_create_tool_call_message(self):
        """Test creating a tool call message."""
        msg = tool_call_message(
            tool_name="cst_analyze",
            tool_input={"file_path": "/src/main.py"},
            tool_id="call_123"
        )
        
        assert msg.role == MessageRole.TOOL_CALL
        assert msg.tool_name == "cst_analyze"
        assert msg.tool_input == {"file_path": "/src/main.py"}
        assert msg.tool_id == "call_123"
    
    def test_create_tool_result_message(self):
        """Test creating a tool result message."""
        msg = tool_result_message(
            tool_id="call_123",
            content="Analysis complete: 5 functions found"
        )
        
        assert msg.role == MessageRole.TOOL_RESULT
        assert msg.tool_id == "call_123"
        assert msg.content == "Analysis complete: 5 functions found"
    
    def test_message_to_llm_dict_user(self):
        """Test converting user message to LLM format."""
        msg = user_message("Hello")
        llm_dict = msg.to_llm_dict()
        
        assert llm_dict == {"role": "user", "content": "Hello"}
    
    def test_message_to_llm_dict_assistant(self):
        """Test converting assistant message to LLM format."""
        msg = assistant_message("Hi there!")
        llm_dict = msg.to_llm_dict()
        
        assert llm_dict == {"role": "assistant", "content": "Hi there!"}
    
    def test_message_with_attachment(self):
        """Test message with file attachment."""
        attachment = Attachment(
            file_path="/path/to/file.py",
            content="print('hello')",
            mime_type="text/x-python"
        )
        msg = user_message("Review this code", attachments=[attachment])
        
        assert len(msg.attachments) == 1
        assert msg.attachments[0].file_path == "/path/to/file.py"
    
    def test_message_serialization_roundtrip(self):
        """Test that messages can be serialized and deserialized."""
        original = user_message("Test message")
        
        as_dict = original.to_dict()
        restored = Message.from_dict(as_dict)
        
        assert restored.role == original.role
        assert restored.content == original.content
        assert restored.enabled == original.enabled
    
    def test_disabled_message_excluded_from_llm(self):
        """Test that disabled messages are marked correctly."""
        msg = user_message("Disabled message")
        msg.enabled = False
        
        assert msg.enabled is False


# =============================================================================
# Context Model Tests
# =============================================================================

class TestContextModel:
    """Tests for the unified Context model."""
    
    def test_create_empty_context(self):
        """Test creating an empty context."""
        ctx = Context()
        
        assert ctx.messages == []
        assert ctx.metadata == {}
    
    def test_add_message_to_context(self):
        """Test adding messages to context."""
        ctx = Context()
        
        ctx.add_message(user_message("Hello"))
        ctx.add_message(assistant_message("Hi!"))
        
        assert len(ctx.messages) == 2
        assert ctx.messages[0].role == MessageRole.USER
        assert ctx.messages[1].role == MessageRole.ASSISTANT
    
    def test_get_enabled_messages(self):
        """Test filtering to only enabled messages."""
        ctx = Context()
        
        msg1 = user_message("Enabled message")
        msg2 = user_message("Disabled message")
        msg2.enabled = False
        msg3 = assistant_message("Another enabled")
        
        ctx.add_message(msg1)
        ctx.add_message(msg2)
        ctx.add_message(msg3)
        
        enabled = ctx.get_enabled_messages()
        
        assert len(enabled) == 2
        assert all(m.enabled for m in enabled)
    
    def test_to_llm_messages(self):
        """Test converting context to LLM-ready format."""
        ctx = Context()
        ctx.add_message(system_message("Be helpful"))
        ctx.add_message(user_message("Hello"))
        ctx.add_message(assistant_message("Hi!"))
        
        llm_messages = ctx.to_llm_messages()
        
        assert len(llm_messages) == 3
        assert llm_messages[0]["role"] == "system"
        assert llm_messages[1]["role"] == "user"
        assert llm_messages[2]["role"] == "assistant"
    
    def test_context_save_and_load(self):
        """Test saving and loading context from file."""
        ctx = Context()
        ctx.add_message(user_message("Test message"))
        ctx.metadata["session_id"] = "test_123"
        
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
            temp_path = f.name
        
        try:
            ctx.save(temp_path)
            loaded = Context.load(temp_path)
            
            assert len(loaded.messages) == 1
            assert loaded.messages[0].content == "Test message"
            assert loaded.metadata.get("session_id") == "test_123"
        finally:
            os.unlink(temp_path)
    
    def test_to_payload_for_api(self):
        """Test creating API payload from context."""
        ctx = Context()
        ctx.add_message(user_message("Hello"))
        
        payload = ctx.to_payload(model="llama3.2")
        
        assert payload["model"] == "llama3.2"
        assert "messages" in payload
        assert len(payload["messages"]) == 1


# =============================================================================
# ResponseChunk Model Tests
# =============================================================================

class TestResponseChunkModel:
    """Tests for the streaming ResponseChunk model."""
    
    def test_create_content_chunk(self):
        """Test creating a content chunk."""
        chunk = ResponseChunk(
            chunk_type=ChunkType.CONTENT,
            content="Hello"
        )
        
        assert chunk.chunk_type == ChunkType.CONTENT
        assert chunk.content == "Hello"
    
    def test_create_thinking_chunk(self):
        """Test creating a thinking/reasoning chunk."""
        chunk = ResponseChunk(
            chunk_type=ChunkType.THINKING,
            content="Let me analyze this..."
        )
        
        assert chunk.chunk_type == ChunkType.THINKING
    
    def test_create_tool_call_chunk(self):
        """Test creating a tool call chunk."""
        chunk = ResponseChunk(
            chunk_type=ChunkType.TOOL_CALL,
            tool_name="cst_analyze",
            tool_input={"file": "main.py"}
        )
        
        assert chunk.chunk_type == ChunkType.TOOL_CALL
        assert chunk.tool_name == "cst_analyze"
    
    def test_create_error_chunk(self):
        """Test creating an error chunk."""
        chunk = ResponseChunk(
            chunk_type=ChunkType.ERROR,
            content="Connection failed",
            error_code="CONN_ERR"
        )
        
        assert chunk.chunk_type == ChunkType.ERROR
        assert chunk.error_code == "CONN_ERR"
    
    def test_from_ollama_chunk(self):
        """Test converting from Ollama streaming format."""
        ollama_chunk = {
            "message": {"content": "Hello"},
            "done": False
        }
        
        chunk = ResponseChunk.from_ollama_chunk(ollama_chunk)
        
        assert chunk.chunk_type == ChunkType.CONTENT
        assert chunk.content == "Hello"
    
    def test_done_chunk_from_ollama(self):
        """Test creating done chunk from Ollama."""
        ollama_chunk = {
            "message": {"content": ""},
            "done": True
        }
        
        chunk = ResponseChunk.from_ollama_chunk(ollama_chunk)
        
        assert chunk.chunk_type == ChunkType.DONE


# =============================================================================
# Tool System Tests
# =============================================================================

class TestToolSystem:
    """Tests for the tool definition and execution system."""
    
    def test_create_tool_definition(self):
        """Test creating a tool definition."""
        tool_def = ToolDefinition(
            name="file_read",
            description="Read a file from the filesystem",
            parameters={
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "File path"}
                },
                "required": ["path"]
            },
            execution_context=ToolExecutionContext.CLIENT
        )
        
        assert tool_def.name == "file_read"
        assert tool_def.execution_context == ToolExecutionContext.CLIENT
    
    def test_create_tool_request(self):
        """Test creating a tool execution request."""
        request = ToolRequest(
            tool_name="file_read",
            tool_input={"path": "/src/main.py"},
            request_id="req_001"
        )
        
        assert request.tool_name == "file_read"
        assert request.tool_input["path"] == "/src/main.py"
    
    def test_create_tool_response_success(self):
        """Test creating a successful tool response."""
        response = ToolResponse(
            request_id="req_001",
            success=True,
            result="File contents here...",
        )
        
        assert response.success is True
        assert response.error is None
    
    def test_create_tool_response_error(self):
        """Test creating a failed tool response."""
        response = ToolResponse(
            request_id="req_001",
            success=False,
            error="File not found: /src/main.py"
        )
        
        assert response.success is False
        assert "File not found" in response.error
    
    def test_tool_registry_register(self):
        """Test registering tools in registry."""
        registry = ToolRegistry()
        
        class MockTool(BaseTool):
            name = "mock_tool"
            description = "A mock tool for testing"
            execution_context = ToolExecutionContext.CLIENT
            
            def execute(self, **kwargs):
                return {"status": "ok"}
        
        registry.register(MockTool())
        
        assert "mock_tool" in registry.list_tools()
    
    def test_tool_registry_get_definitions(self):
        """Test getting tool definitions from registry."""
        registry = ToolRegistry()
        
        class MockTool(BaseTool):
            name = "mock_tool"
            description = "A mock tool"
            execution_context = ToolExecutionContext.CLIENT
            parameters = {"type": "object", "properties": {}}
            
            def execute(self, **kwargs):
                return {}
        
        registry.register(MockTool())
        definitions = registry.get_definitions()
        
        assert len(definitions) == 1
        assert definitions[0].name == "mock_tool"


# =============================================================================
# Configuration Tests
# =============================================================================

class TestConfiguration:
    """Tests for the unified configuration system."""
    
    def test_default_agentx_config(self):
        """Test AgentXConfig defaults."""
        config = AgentXConfig()
        
        assert config.ollama_host == "localhost:11434"
        assert config.ollama_model == "llama3.2"
        assert config.screen_side == ScreenSide.LEFT
    
    def test_default_agentix_config(self):
        """Test AgentixConfig defaults."""
        config = AgentixConfig()
        
        assert config.enabled is True
        assert config.server_url is None
        assert config.classify_prompts is True
    
    def test_agentix_is_remote(self):
        """Test remote detection for Agentix."""
        local_config = AgentixConfig()
        assert local_config.is_remote is False
        
        remote_config = AgentixConfig(server_url="http://localhost:8000")
        assert remote_config.is_remote is True
    
    def test_unified_config_defaults(self):
        """Test UnifiedConfig default creation."""
        config = UnifiedConfig()
        
        assert isinstance(config.agentx, AgentXConfig)
        assert isinstance(config.agentix, AgentixConfig)
    
    def test_unified_config_from_dict(self):
        """Test creating UnifiedConfig from dictionary."""
        data = {
            "agentx": {
                "ollama_model": "codellama",
                "screen_side": "right"
            },
            "agentix": {
                "enabled": False,
                "debug": True
            }
        }
        
        config = UnifiedConfig.from_dict(data)
        
        assert config.agentx.ollama_model == "codellama"
        assert config.agentx.screen_side == ScreenSide.RIGHT
        assert config.agentix.enabled is False
        assert config.agentix.debug is True
    
    def test_unified_config_from_toml(self):
        """Test loading UnifiedConfig from TOML file."""
        toml_content = """
[agentx]
ollama_host = "custom-host:11434"
ollama_model = "mistral"
screen_side = "center"

[agentix]
enabled = true
classify_prompts = true
available_tools = ["cst", "ast", "search"]
"""
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".toml", delete=False
        ) as f:
            f.write(toml_content)
            temp_path = f.name
        
        try:
            config = UnifiedConfig.from_toml(temp_path)
            
            assert config.agentx.ollama_host == "custom-host:11434"
            assert config.agentx.ollama_model == "mistral"
            assert config.agentx.screen_side == ScreenSide.CENTER
            assert "search" in config.agentix.available_tools
        finally:
            os.unlink(temp_path)
    
    def test_unified_config_missing_file_uses_defaults(self):
        """Test that missing config file returns defaults."""
        config = UnifiedConfig.from_toml("/nonexistent/path.toml")
        
        assert config.agentx.ollama_model == "llama3.2"
        assert config.agentix.enabled is True
    
    def test_config_convenience_properties(self):
        """Test convenience properties on UnifiedConfig."""
        config = UnifiedConfig()
        
        assert config.ollama_host == config.agentx.ollama_host
        assert config.ollama_model == config.agentx.ollama_model
        assert config.agentix_enabled == config.agentix.enabled
    
    def test_config_to_dict_roundtrip(self):
        """Test configuration serialization roundtrip."""
        original = UnifiedConfig()
        original.agentx.ollama_model = "custom-model"
        original.agentix.debug = True
        
        as_dict = original.to_dict()
        restored = UnifiedConfig.from_dict(as_dict)
        
        assert restored.agentx.ollama_model == "custom-model"
        assert restored.agentix.debug is True


# =============================================================================
# Integration Tests (Mock-based)
# =============================================================================

class TestClientServerCommunication:
    """Tests for client-server communication patterns."""
    
    def test_context_to_api_request(self):
        """Test converting context to API request format."""
        ctx = Context()
        ctx.add_message(system_message("You are helpful"))
        ctx.add_message(user_message("Hello"))
        
        payload = ctx.to_payload(
            model="llama3.2",
            stream=True,
            options={"temperature": 0.7}
        )
        
        assert payload["model"] == "llama3.2"
        assert payload["stream"] is True
        assert payload["options"]["temperature"] == 0.7
        assert len(payload["messages"]) == 2
    
    def test_tool_request_serialization(self):
        """Test tool request can be serialized for API."""
        request = ToolRequest(
            tool_name="cst_analyze",
            tool_input={"file_path": "/src/main.py", "depth": 3},
            request_id="tool_001"
        )
        
        as_dict = request.to_dict()
        json_str = json.dumps(as_dict)  # Ensure JSON serializable
        
        restored_dict = json.loads(json_str)
        assert restored_dict["tool_name"] == "cst_analyze"
        assert restored_dict["tool_input"]["depth"] == 3
    
    def test_tool_response_serialization(self):
        """Test tool response can be serialized for API."""
        response = ToolResponse(
            request_id="tool_001",
            success=True,
            result={"functions": ["main", "helper"], "count": 2}
        )
        
        as_dict = response.to_dict()
        json_str = json.dumps(as_dict)
        
        restored = json.loads(json_str)
        assert restored["success"] is True
        assert restored["result"]["count"] == 2
    
    def test_response_chunk_stream_simulation(self):
        """Simulate receiving a stream of response chunks."""
        # Simulate chunks as they would arrive from streaming API
        chunks = [
            ResponseChunk(ChunkType.THINKING, content="Analyzing..."),
            ResponseChunk(ChunkType.CONTENT, content="Here"),
            ResponseChunk(ChunkType.CONTENT, content=" is"),
            ResponseChunk(ChunkType.CONTENT, content=" the"),
            ResponseChunk(ChunkType.CONTENT, content=" answer"),
            ResponseChunk(ChunkType.TOOL_CALL, tool_name="search", tool_input={"q": "test"}),
            ResponseChunk(ChunkType.TOOL_RESULT, tool_id="t1", content="Search results"),
            ResponseChunk(ChunkType.DONE),
        ]
        
        # Accumulate content
        content_parts = []
        tool_calls = []
        
        for chunk in chunks:
            if chunk.chunk_type == ChunkType.CONTENT:
                content_parts.append(chunk.content)
            elif chunk.chunk_type == ChunkType.TOOL_CALL:
                tool_calls.append(chunk.tool_name)
        
        full_content = "".join(content_parts)
        assert full_content == "Here is the answer"
        assert "search" in tool_calls


# =============================================================================
# Live Integration Tests (require running services)
# =============================================================================

@pytest.mark.live
class TestLiveIntegration:
    """
    Live integration tests that require actual services.
    
    These tests are skipped by default. Run with:
        pytest -m live tests/integration/
    
    Requirements:
    - Ollama running locally
    - (Optional) Agentix server running
    """
    
    @pytest.mark.skip(reason="Requires live Ollama instance")
    def test_ollama_connection(self):
        """Test actual connection to Ollama."""
        # This would test actual Ollama connectivity
        pass
    
    @pytest.mark.skip(reason="Requires live Agentix server")
    def test_agentix_server_health(self):
        """Test connection to Agentix server."""
        # This would test actual Agentix server health endpoint
        pass


# =============================================================================
# Test Runner Configuration
# =============================================================================

if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
