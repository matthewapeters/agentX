"""
Adapter for using AgentixBridge within AgentX's threaded model.

AgentX uses tkinter which requires all GUI updates to happen on the main thread,
while Agentix uses async/await patterns. This adapter bridges the two models.
"""

import json
import logging
import os
import sys
from pathlib import Path
from typing import Callable, Iterator, Optional

# Set AGENTIX_HOME to project root for local system_prompts
PROJECT_ROOT = Path(__file__).resolve().parents[3]
os.environ["AGENTIX_HOME"] = str(PROJECT_ROOT)

from agentix.agentix_config import AgentixConfig
from agentix.bridge import AgentixBridge
from agentix.prompt_classification_response import (
    Intent,
    NextStep,
    PromptClassificationResponse,
)
from shared.models.context import Context
from shared.models.response import ChunkType, ResponseChunk

logger = logging.getLogger("agentx.adapter")


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
        self.tool_registry_manager = None  # Initialized by _register_registry_tools()
        self._register_client_tools()
        self._register_terminal_tools()
        self._register_registry_tools()

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
            logger.warning("Could not register client tools: %s", exc)

    def _register_terminal_tools(self) -> None:
        """Register tmux-backed terminal tools with the bridge.

        Terminal tools are optional and are only useful when the launcher has set
        ``AGENTX_TMUX_SESSION`` and tmux is available at runtime.
        """
        try:
            from agentx.integration.terminal_bridge import (
                configure_terminal_bridge,
                get_terminal_tool_implementations,
                get_terminal_tool_schemas,
            )

            tmux_session = os.environ.get("AGENTX_TMUX_SESSION", "agentx")
            audit_log_path = self.config.get("terminal", {}).get("audit_log_path")

            configure_terminal_bridge(
                config=self.config,
                session_id=tmux_session,
                project_roots=[str(Path.cwd())],
                audit_log_path=audit_log_path,
            )

            impls = get_terminal_tool_implementations()
            schemas = get_terminal_tool_schemas()
            self.bridge.register_tool_implementations(impls, schemas)
        except Exception as exc:
            logger.warning("Could not register terminal tools: %s", exc)

    def _register_registry_tools(self) -> None:
        """Register dynamic tool registry with the bridge.

        Initializes the ToolRegistryManager which:
        - Loads available tools from agentx_tools.toml
        - Provides built-in tools: reload_tools, register_tool
        - Exposes registry to UI (ToolPanel) for enable/disable controls

        The manager is stored as self.tool_registry_manager so the UI can
        access it to update enabled tools.
        """
        try:
            from agentx.integration.tool_registry_manager import (
                ToolRegistryManager,
            )

            def on_registry_change():
                """Callback when registry state changes - update bridge with new enabled tools."""
                enabled_names = self.tool_registry_manager.get_enabled_tool_names()
                self.bridge.set_enabled_tools(enabled_names)

            self.tool_registry_manager = ToolRegistryManager(
                config_path="agentx_tools.toml",
                on_registry_change=on_registry_change,
                bridge=self.bridge,
                config=self.config,
            )

            impls = self.tool_registry_manager.get_builtin_tool_implementations()

            # Generate tool schemas for built-in tools
            schemas = [
                {
                    "name": "reload_tools",
                    "description": "Reload available tools from configuration",
                    "parameters": {
                        "type": "object",
                        "properties": {},
                        "required": [],
                    },
                },
                {
                    "name": "register_tool",
                    "description": "Dynamically register a new tool",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "tool_name": {
                                "type": "string",
                                "description": "Unique name for the tool",
                            },
                            "description": {
                                "type": "string",
                                "description": "Human-readable description",
                            },
                            "category": {
                                "type": "string",
                                "description": "Tool category (e.g., user, filesystem)",
                            },
                            "enabled": {
                                "type": "boolean",
                                "description": "Whether tool is enabled by default",
                            },
                            "scope": {
                                "type": "string",
                                "description": "Tool scope (universal, user, project, session)",
                            },
                            "source_path": {
                                "type": "string",
                                "description": "Path to tool implementation file",
                            },
                            "runtime": {
                                "type": "string",
                                "description": "Tool runtime (e.g., python)",
                            },
                            "entrypoint": {
                                "type": "string",
                                "description": "Callable/function entrypoint (e.g., module:function)",
                            },
                            "input_schema": {
                                "type": "object",
                                "description": "JSON-schema-like input schema for the tool",
                            },
                            "output_schema": {
                                "type": "object",
                                "description": "JSON-schema-like output schema for the tool",
                            },
                        },
                        "required": ["tool_name"],
                    },
                },
                {
                    "name": "diagnose_tools",
                    "description": "Run diagnostics for tool registration and execution pipeline",
                    "parameters": {
                        "type": "object",
                        "properties": {},
                        "required": [],
                    },
                },
                {
                    "name": "open_file_in_editor",
                    "description": "Open a file in the running vibe editor (neovim)",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "file_path": {
                                "type": "string",
                                "description": "Absolute or relative path to the file to open",
                            },
                            "line": {
                                "type": "integer",
                                "description": "Optional 1-based line number for cursor placement",
                            },
                        },
                        "required": ["file_path"],
                    },
                },
                {
                    "name": "diff_files_in_editor",
                    "description": "Open a side-by-side diff between two files in the running vibe editor",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "left_file": {
                                "type": "string",
                                "description": "Path to the left-hand file in the diff view",
                            },
                            "right_file": {
                                "type": "string",
                                "description": "Path to the right-hand file in the diff view",
                            },
                        },
                        "required": ["left_file", "right_file"],
                    },
                },
                {
                    "name": "editor_action",
                    "description": "Run sandboxed editor-assist actions in the running vibe editor",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "action": {
                                "type": "string",
                                "description": "Action name: show_symbol_help | autocomplete_assist | propose_edit",
                            },
                            "file_path": {
                                "type": "string",
                                "description": "Path to target file",
                            },
                            "line": {
                                "type": "integer",
                                "description": "Optional 1-based line number",
                            },
                            "payload": {
                                "type": "string",
                                "description": "Optional payload text (used by propose_edit)",
                            },
                        },
                        "required": ["action", "file_path"],
                    },
                },
            ]

            self.bridge.register_tool_implementations(impls, schemas)
        except Exception as exc:
            logger.warning("Could not register registry tools: %s", exc)

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
            logger.warning("Could not register working memory tools: %s", exc)

    def _iter_safe(
        self, gen_factory: Callable[[], Iterator[ResponseChunk]], error_prefix: str
    ) -> Iterator[ResponseChunk]:
        """Yield all chunks from the iterator produced by *gen_factory*.

        Calling *gen_factory* and iterating its result are both wrapped in a
        single ``try`` block so that exceptions raised either at call time
        (e.g. a mock ``side_effect``) or during iteration are converted to an
        ERROR ``ResponseChunk`` rather than propagating into Tkinter.

        This is the single canonical streaming wrapper used by every generator
        method in this adapter, replacing per-method ``try / yield from / except``
        boilerplate.

        Args:
            gen_factory:  Zero-argument callable that returns an
                          ``Iterator[ResponseChunk]`` — typically a lambda over
                          a bridge streaming method call.
            error_prefix: Human-readable prefix prepended to the error message
                          in the ERROR chunk content.
        """
        try:
            yield from gen_factory()
        except Exception as e:
            yield ResponseChunk(type=ChunkType.ERROR, content=f"{error_prefix}: {str(e)}")

    def classify_prompt_sync(
        self,
        prompt: str,
        context: Context,
        working_memory: Optional["WorkingMemory"] = None,
    ) -> Optional[PromptClassificationResponse]:
        """
        Synchronously classify user prompt.

        This is a blocking call suitable for background threads.

        Args:
            prompt: User's input text
            context: Current conversation context
            working_memory: Optional WorkingMemory instance for context-aware classification

        Returns:
            PromptClassificationResponse or None if classification disabled
        """
        if not self.agentix_config.classify_prompts:
            return None

        try:
            return self.bridge.classify_prompt(prompt, context, working_memory)
        except (json.JSONDecodeError, ValueError) as e:
            # LLM returned non-JSON (conversational text or malformed output) — recoverable; fall back to defaults
            logger.warning(
                "Classification skipped: LLM returned non-JSON response",
                extra={
                    "error": str(e),
                    "prompt_preview": prompt[:200] if prompt else "",
                    "context_size": len(context.get_enabled_messages()) if context else 0,
                    "model": self.agentix_config.classification_model or self.agentix_config.model,
                },
            )
            return PromptClassificationResponse(
                intent=Intent.conversation,
                needs_clarification=False,
                missing_fields=[],
                reasoning_summary=f"Non-JSON response: {str(e)[:50]}",
                next_step=NextStep.respond_directly,
            )
        except KeyError as e:
            # Enum lookup failed — invalid intent/next_step from LLM; recoverable
            logger.warning(
                "Classification skipped: invalid enum value in LLM response",
                extra={
                    "error": str(e),
                    "prompt_preview": prompt[:200] if prompt else "",
                    "valid_intents": [i.name for i in Intent],
                    "valid_next_steps": [n.name for n in NextStep],
                },
            )
            return PromptClassificationResponse(
                intent=Intent.conversation,
                needs_clarification=False,
                missing_fields=[],
                reasoning_summary=f"Invalid enum: {str(e)[:50]}",
                next_step=NextStep.respond_directly,
            )
        except Exception as e:
            # Genuinely unexpected error — log full traceback at WARNING (still recoverable)
            logger.warning(
                "Classification unexpected error",
                extra={
                    "error": str(e),
                    "error_type": type(e).__name__,
                    "prompt_preview": prompt[:200] if prompt else "",
                },
                exc_info=True,
            )
            return PromptClassificationResponse(
                intent=Intent.conversation,
                needs_clarification=False,
                missing_fields=[],
                reasoning_summary=f"Error: {type(e).__name__}",
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
        yield from self._iter_safe(
            lambda: self.bridge.process_prompt_streaming(prompt, context, classification),
            "Error processing prompt",
        )

    def retrigger_synthesis_generator(
        self,
        node,
        context: Context,
        task_tree,
        hint: str = "",
    ) -> Iterator[ResponseChunk]:
        """Yield re-synthesis chunks for a completed task node.

        Delegates to ``AgentixBridge.retrigger_synthesis_streaming``.

        Args:
            node:       ``TaskNodeRecord`` to re-synthesise.
            context:    Current session context.
            task_tree:  The live ``TaskTree``.
            hint:       Optional user guidance injected into the prompt.

        Yields:
            ``ResponseChunk`` objects.
        """
        yield from self._iter_safe(
            lambda: self.bridge.retrigger_synthesis_streaming(node, context, task_tree, hint),
            "Error during retrigger synthesis",
        )

    def replay_task_node_generator(
        self,
        node,
        context: Context,
        task_tree,
    ) -> Iterator[ResponseChunk]:
        """Yield all chunks for a full task-node replay from scratch.

        Delegates to ``AgentixBridge.replay_task_node_streaming``.

        Args:
            node:       ``TaskNodeRecord`` to replay.
            context:    Current session context.
            task_tree:  The live ``TaskTree``.

        Yields:
            ``ResponseChunk`` objects.
        """
        yield from self._iter_safe(
            lambda: self.bridge.replay_task_node_streaming(node, context, task_tree),
            "Error during task node replay",
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
            logger.exception("Error fetching models: %s", e)
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
            logger.exception("Error fetching tools: %s", e)
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
            logger.warning("Could not update enabled tools: %s", exc)

    def invalidate_max_tokens(self) -> None:
        """Clear any cached model context length held by the Agentix bridge."""
        try:
            self.bridge.invalidate_max_tokens()
        except Exception as exc:
            logger.warning("Could not invalidate Agentix max token cache: %s", exc)

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
            classification_model=agentix_section.get("agentix_bench_classification_model"),
            classification_backend=agentix_section.get("classification_backend", "ollama"),
            classification_torch_model=agentix_section.get("classification_torch_model"),
            classification_torch_device=agentix_section.get("classification_torch_device"),
            max_tool_rounds=agentix_section.get("max_tool_rounds", 10),
            max_task_depth=agentix_section.get("max_task_depth", 10),
            max_synthesis_retries=agentix_section.get("max_synthesis_retries", 3),
            system_prompts_dir=str(PROJECT_ROOT / agentix_section.get("system_prompts_dir", "system_prompts")),
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
        logger.error("Agentix import error (missing dependency): %s", e)
        logger.error("  Install missing dependencies to enable code analysis tools")
        logger.error("  Command: pip install libcst")
        raise
    except Exception as e:
        logger.exception("Failed to initialize Agentix bridge: %s", e)
        raise
