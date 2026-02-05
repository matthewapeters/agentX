"""
Adapter for using AgentixBridge within AgentX's threaded model.

AgentX uses tkinter which requires all GUI updates to happen on the main thread,
while Agentix uses async/await patterns. This adapter bridges the two models.
"""

import sys
from pathlib import Path
from typing import Iterator, Optional

# Add parent directories to path for imports
parent_dir = str(Path(__file__).parent.parent.parent)
if parent_dir not in sys.path:
    sys.path.insert(0, parent_dir)

from agentix.bridge import AgentixBridge
from agentix.agentix_config import AgentixConfig
from agentix.prompt_classification_response import PromptClassificationResponse
from shared.models.context import Context
from shared.models.response import ResponseChunk


class AgentixBridgeAdapter:
    """
    Adapts AgentixBridge for use in AgentX's synchronous threaded model.
    
    This adapter:
    - Converts AgentX config format to AgentixConfig
    - Provides synchronous wrappers for async bridge methods
    - Handles generator-based streaming compatible with tkinter
    - Thread-safe for use in background threads
    
    Example usage:
        adapter = AgentixBridgeAdapter(config)
        
        # Classify prompt
        classification = adapter.classify_prompt_sync(prompt, context)
        
        # Stream response
        for chunk in adapter.process_prompt_generator(prompt, context, classification):
            # Update GUI with chunk
            gui.append_output(chunk.content)
    """
    
    def __init__(self, config: dict):
        """
        Initialize adapter with AgentX configuration.
        
        Args:
            config: AgentX configuration dictionary with structure:
                {
                    "agentx": {"ollama_model": str, "ollama_host": str, ...},
                    "agentix": {"enabled": bool, "classify_prompts": bool, ...}
                }
        """
        self.config = config
        self.agentix_config = self._convert_config(config)
        self.bridge = AgentixBridge(self.agentix_config)
        self._enabled = config.get("agentix", {}).get("enabled", False)
        
    @property
    def enabled(self) -> bool:
        """Check if Agentix integration is enabled."""
        return self._enabled
    
    def classify_prompt_sync(
        self, 
        prompt: str, 
        context: Context
    ) -> Optional[PromptClassificationResponse]:
        """
        Synchronously classify user prompt.
        
        This is a blocking call suitable for background threads.
        
        Args:
            prompt: User's input text
            context: Current conversation context
            
        Returns:
            PromptClassificationResponse or None if classification disabled
        """
        if not self.enabled:
            return None
        
        if not self.agentix_config.classify_prompts:
            return None
        
        try:
            return self.bridge.classify_prompt(prompt, context)
        except Exception as e:
            print(f"Classification error: {e}")
            return None
    
    def process_prompt_generator(
        self,
        prompt: str,
        context: Context,
        classification: Optional[PromptClassificationResponse] = None,
    ) -> Iterator[ResponseChunk]:
        """
        Process prompt and yield response chunks.
        
        This generator is compatible with AgentX's streaming loop.
        It yields ResponseChunk objects that can be processed by
        the response handler.
        
        Args:
            prompt: User's input text
            context: Current conversation context
            classification: Optional pre-computed classification
            
        Yields:
            ResponseChunk objects with content, tool calls, etc.
        """
        if not self.enabled:
            # Fall back to direct Ollama streaming (handled by caller)
            return
        
        try:
            # Bridge returns an iterator, so we can yield directly
            yield from self.bridge.process_prompt_streaming(
                prompt, context, classification
            )
        except Exception as e:
            # Yield error chunk
            from shared.models.response import ChunkType
            yield ResponseChunk(
                type=ChunkType.ERROR,
                content=f"Error processing prompt: {str(e)}",
                error_code="BRIDGE_ERROR",
            )
    
    def get_models(self) -> list[dict]:
        """
        Get available models from Ollama.
        
        Returns:
            List of model dictionaries with name, size, details
        """
        try:
            return self.bridge.get_available_models()
        except Exception as e:
            print(f"Error fetching models: {e}")
            return []
    
    def get_tools(self) -> list[dict]:
        """
        Get available MCP tools.
        
        Returns:
            List of tool definitions in OpenAI format
        """
        try:
            return self.bridge.get_available_tools()
        except Exception as e:
            print(f"Error fetching tools: {e}")
            return []
    
    def _convert_config(self, agentx_config: dict) -> AgentixConfig:
        """
        Convert AgentX config dict to AgentixConfig.
        
        Maps between the two configuration formats:
        - agentx.ollama_model -> model
        - agentx.ollama_host -> (not used by bridge, uses default)
        - agentix.classify_prompts -> classify_prompts
        - etc.
        
        Args:
            agentx_config: AgentX configuration dictionary
            
        Returns:
            AgentixConfig instance
        """
        agentx_section = agentx_config.get("agentx", {})
        agentix_section = agentx_config.get("agentix", {})
        
        return AgentixConfig(
            model=agentx_section.get("ollama_model", "llama3.2"),
            temperature=agentx_section.get("temperature", 0.7),
            tools=agentix_section.get("available_tools", ["cst", "ast"]),
            system=agentix_section.get("default_system_prompts", []),
            debug=agentix_section.get("debug", False),
        )


def create_adapter(config: dict) -> Optional[AgentixBridgeAdapter]:
    """
    Convenience function to create an adapter if Agentix is enabled.
    
    Args:
        config: AgentX configuration dictionary
        
    Returns:
        AgentixBridgeAdapter if enabled, None otherwise
    """
    if config.get("agentix", {}).get("enabled", False):
        return AgentixBridgeAdapter(config)
    return None
