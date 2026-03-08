"""
Adapter for using AgentixBridge within AgentX's threaded model.

AgentX uses tkinter which requires all GUI updates to happen on the main thread,
while Agentix uses async/await patterns. This adapter bridges the two models.
"""

import os
import sys
from pathlib import Path
from typing import Iterator, Optional

# Add parent directories to path for imports
parent_dir = str(Path(__file__).parent.parent.parent)
if parent_dir not in sys.path:
    sys.path.insert(0, parent_dir)

# Set AGENTIX_HOME to project root for local system_prompts
PROJECT_ROOT = Path(__file__).resolve().parents[3]
os.environ["AGENTIX_HOME"] = str(PROJECT_ROOT)

from agentix.bridge import AgentixBridge
from agentix.agentix_config import AgentixConfig
from agentix.prompt_classification_response import (
    PromptClassificationResponse,
    Intent,
    NextStep,
)
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
                    "agentix": {"classify_prompts": bool, ...}
                }
        """
        self.config = config
        self.agentix_config = self._convert_config(config)
        self.bridge = AgentixBridge(self.agentix_config)
        self._register_client_tools()

    def _register_client_tools(self) -> None:
        """Register client-side file tools with the bridge.

        This makes ``read_file``, ``write_file``, ``list_directory``,
        ``get_file_info``, and ``search_files`` available to the LLM tool loop.
        The tools execute locally (within the AgentX process) but are fully
        transparent to the bridge's ``_run_tool_loop``.
        """
        try:
            from agentx.integration.client_tool_executor import (
                get_client_tool_implementations,
                get_client_tool_schemas,
            )
            impls = get_client_tool_implementations()
            schemas = get_client_tool_schemas()
            self.bridge.register_tool_implementations(impls, schemas)
        except Exception as exc:
            print(f"⚠ Could not register client tools: {exc}")

    def register_working_memory_tools(self, working_memory) -> None:
        """Register Working Memory tools with the bridge.

        Called after session creates its ``WorkingMemory`` instance so the tools
        hold a live reference to session state. Must be called before the first
        prompt is processed.

        Args:
            working_memory: The session's ``WorkingMemory`` instance.
        """
        try:
            from agentx.integration.working_memory_tool_executor import (
                WorkingMemoryToolExecutor,
                get_working_memory_tool_schemas,
            )
            executor = WorkingMemoryToolExecutor(working_memory)
            impls = executor.get_tool_implementations()
            schemas = get_working_memory_tool_schemas()
            self.bridge.register_tool_implementations(impls, schemas)
        except Exception as exc:
            print(f"⚠ Could not register working memory tools: {exc}")
    
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
        if not self.agentix_config.classify_prompts:
            return None
        
        try:
            return self.bridge.classify_prompt(prompt, context)
        except Exception as e:
            print(f"Classification error: {e}")
            return PromptClassificationResponse(
                intent=Intent.conversation,
                needs_clarification=False,
                missing_fields=[],
                reasoning_summary="Classification unavailable",
                next_step=NextStep.respond_directly,
            )
    
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

    def set_enabled_tools(self, enabled_tool_names: list[str]) -> None:
        """
        Restrict tools offered to the LLM to the given names.

        Delegates directly to the bridge so that the next call to
        ``get_available_tools()`` (inside ``_run_tool_loop``) honours the
        user's selection.

        Args:
            enabled_tool_names: Tool names the user has checked in the GUI.
        """
        try:
            self.bridge.set_enabled_tools(enabled_tool_names)
        except Exception as exc:
            print(f"⚠ Could not update enabled tools: {exc}")
    
    def _convert_config(self, agentx_config: dict) -> AgentixConfig:
        """
        Convert AgentX config dict to AgentixConfig.
        
        Maps between the two configuration formats:
        - agentx.ollama_model -> model
        - agentx.ollama_host -> ollama_host
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
            ollama_host=agentx_section.get("ollama_host", "localhost:11434"),
            classify_prompts=agentix_section.get("classify_prompts", True),
            classification_model=agentix_section.get(
                "agentix_bench_classification_model"
            ),
            classification_backend=agentix_section.get(
                "classification_backend", "ollama"
            ),
            classification_torch_model=agentix_section.get(
                "classification_torch_model"
            ),
            classification_torch_device=agentix_section.get(
                "classification_torch_device"
            ),
        )


def create_adapter(config: dict) -> AgentixBridgeAdapter:
    """
    Create Agentix bridge adapter.
    
    Agentix is always integrated.
    
    Args:
        config: AgentX configuration dictionary
        
    Returns:
        AgentixBridgeAdapter instance
    """
    try:
        return AgentixBridgeAdapter(config)
    except ImportError as e:
        print(f"⚠ Agentix import error (missing dependency): {e}")
        print("  Install missing dependencies to enable code analysis tools")
        print(f"  Command: pip install libcst")
        raise
    except Exception as e:
        print(f"⚠ Failed to initialize Agentix bridge: {e}")
        raise
