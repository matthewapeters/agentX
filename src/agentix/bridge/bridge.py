"""
Programmatic API bridge for Agentix.

This module provides a clean programmatic interface to Agentix functionality
that can be consumed by AgentX GUI without going through the CLI.
"""
import json
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Iterator, Optional

from agentix.agentix_config import AgentixConfig
from agentix.api_client import query_api_streaming
from agentix.models import get_model, get_models
from agentix.prompt_classification_response import (
    NextStep,
    PromptClassificationResponse,
)

# Direct imports to avoid circular dependencies
from agentix.tools import extract_cst_tools
from agentix.tools.describe_tools import to_openai_tools
from shared.models.context import Context
from shared.models.message import Message
from shared.models.response import ChunkType, ResponseChunk
from shared.models.tools import ToolResponse
from .classify_prompt import classify_prompt as classifier


class AgentixBridge:
    """
    Bridge between AgentX GUI and Agentix middleware.

    Provides a programmatic interface to:
    - Prompt classification
    - Model management
    - Tool discovery
    - Streaming response generation

    Example usage:
        config = AgentixConfig(model="llama3.2", debug=False)
        bridge = AgentixBridge(config)

        # Classify prompt
        classification = bridge.classify_prompt("Read file.py", context)

        # Process with streaming
        for chunk in bridge.process_prompt_streaming(prompt, context, classification):
            print(chunk.content)
    """

    def __init__(self, config: AgentixConfig):
        """
        Initialize AgentixBridge with configuration.

        Args:
            config: AgentixConfig instance with model, tools, etc.
        """
        self.config = config
        self._model_cache: Optional[list[dict]] = None
        self._max_tokens: Optional[int] = None
        # Extra tool implementations registered externally (e.g. client-side file tools)
        self._tool_impl_cache: dict[str, callable] = {}
        # Extra OpenAI-format schemas for tools registered externally
        self._extra_tool_schemas: list[dict] = []
        # Tool names the user has disabled via the GUI; None means "all enabled"
        self._disabled_tools: Optional[set[str]] = None

    def classify_prompt(
        self,
        prompt: str,
        context: Context,
    ) -> PromptClassificationResponse:
        """
        Classify user intent before processing.

        Analyzes the user's prompt to determine:
        - Intent type (conversation, simple_action, complex_action, safety_issue)
        - Whether clarification is needed
        - Next step to take (respond_directly, single_tool, invoke_planner, escalate)

        Args:
            prompt: User's input text
            context: Current conversation context

        Returns:
            PromptClassificationResponse with intent and next_step
        """
        return classifier(
            self.config,
            prompt,
            context,
            self._context_to_history(context),
            self._get_max_tokens(),
        )

    def process_prompt_streaming(
        self,
        prompt: str,
        context: Context,
        classification: Optional[PromptClassificationResponse] = None,
    ) -> Iterator[ResponseChunk]:
        """
        Process prompt through appropriate handler with streaming.

        This routes the request based on classification:
        - respond_directly: Direct LLM response
        - single_tool: Execute one tool then respond
        - invoke_planner: Multi-step planning
        - escalate: Human intervention required

        Args:
            prompt: User's input text
            context: Current conversation context
            classification: Optional pre-computed classification

        Yields:
            ResponseChunk objects for GUI rendering
        """
        # Auto-classify if not provided and classification is enabled
        # Note: Classification is transparent to the user - logged but not displayed
        if classification is None and self.config.classify_prompts:

            print("[Agentix] Classifying prompt...", file=sys.stderr)
            classification = self.classify_prompt(prompt, context)
            if self.config.debug and classification:
                print(
                    f"[Agentix] Classification: intent={classification.intent.name}, "
                    f"next_step={classification.next_step.name}, "
                    f"reasoning={classification.reasoning_summary}",
                    file=sys.stderr,
                )

        # Route based on next_step (if classification provided) or default to direct response
        # Route based on next_step (if classification provided) or default to direct response
        if classification:
            match classification.next_step:
                case NextStep.respond_directly:
                    yield from self._stream_direct_response(prompt, context)

                case NextStep.single_tool:
                    yield from self._stream_tool_response(prompt, context, classification)

                case NextStep.invoke_planner:
                    yield from self._stream_planned_response(prompt, context)

                case NextStep.escalate:
                    yield ResponseChunk(
                        type=ChunkType.ERROR,
                        content="This request requires human assistance or involves safety concerns.",
                    )
        else:
            # No classification - default to direct response
            yield from self._stream_direct_response(prompt, context)

    def get_available_models(self) -> list[dict]:
        """
        Fetch available models from Ollama.

        Returns cached results if available.

        Returns:
            List of model dictionaries with name, size, details
        """
        if self._model_cache is None:
            # Don't filter - we want all models for the UI dropdown
            self._model_cache = get_models(self.config, filter_by_model=False)
        return self._model_cache

    def get_available_tools(self) -> list[dict]:
        """
        Return available MCP tools with metadata.

        Tools are defined in Agentix and include:
        - CST (Concrete Syntax Tree) analysis
        - AST (Abstract Syntax Tree) analysis
        - Code search and manipulation
        - Any tools registered via ``register_tool_implementations()``

        If ``set_enabled_tools()`` has been called, only the named tools are
        returned; disabled tools are omitted so the LLM never sees them.

        Returns:
            List of tool definitions in OpenAI tools format
        """

        tools = []
        for tool_name in self.config.tools or []:
            if tool_name == "cst":
                cst_tools = extract_cst_tools()
                tools.extend(cst_tools)
            elif tool_name == "ast":
                # AST tools would be added here
                pass

        result = to_openai_tools(tools) if tools else []
        result.extend(self._extra_tool_schemas)

        if self._disabled_tools is not None:
            result = [
                t for t in result
                if t.get("function", {}).get("name") not in self._disabled_tools
            ]

        return result

    def set_enabled_tools(self, enabled_tool_names: list[str]) -> None:
        """
        Restrict the tools offered to the LLM to the given names.

        Call this when the user toggles tools in the GUI.  Passing an empty
        list disables all tools; passing ``None`` re-enables all tools.

        Args:
            enabled_tool_names: Tool names the user wants enabled.
        """
        all_names = {
            t.get("function", {}).get("name")
            for t in (self._extra_tool_schemas or [])
        }
        # Determine which tools to disable (those known but not in the enabled list)
        self._disabled_tools = all_names - set(enabled_tool_names)

    def register_tool_implementations(
        self,
        impls: "dict[str, callable]",
        schemas: "list[dict] | None" = None,
    ) -> None:
        """
        Register additional tool implementations with this bridge.

        Registered tools are available to ``execute_tool()`` and, when
        ``schemas`` are provided, also appear in ``get_available_tools()``
        so the LLM sees them in its tool list.

        Args:
            impls: Mapping of tool name → callable.  The callable is invoked
                   with keyword arguments extracted from the LLM response.
            schemas: Optional list of OpenAI-format tool schemas
                     (``{"type": "function", "function": {...}}``).  When
                     omitted the tools are executable but not advertised to
                     the LLM.
        """
        self._tool_impl_cache.update(impls)
        if schemas:
            self._extra_tool_schemas.extend(schemas)

    def _context_to_history(self, context: Context) -> list[Message]:
        """
        Convert AgentX Context to Agentix history format.

        Args:
            context: AgentX Context with messages

        Returns:
            List of enabled Message dictionaries (Agentix format)
        """
        return list(context.get_enabled_messages())

    def _get_max_tokens(self) -> int:
        """
        Get max tokens for current model.

        Caches the result after first call.

        Returns:
            Maximum token count for model
        """
        if self._max_tokens is None:
            self._max_tokens = get_model(self.config)
        return self._max_tokens

    def execute_tool(self, tool_name: str, arguments: dict, tool_id: Optional[str] = None) -> ToolResponse:
        """
        Execute a named tool with the given arguments.

        Looks up the tool in the registered tool implementations, calls it, and
        returns a ToolResponse.  Exceptions are caught and returned as error
        responses (error-as-result pattern — the agentic loop never crashes due
        to a single tool failure).

        Args:
            tool_name: Name of the tool to invoke.
            arguments: Keyword arguments to pass to the tool implementation.
            tool_id: Optional tool call ID from the LLM for correlation.

        Returns:
            ToolResponse with success=True and output, or success=False and error.
        """
        impl = self._get_tool_implementations().get(tool_name)
        if impl is None:
            return ToolResponse.error_response(
                f"Unknown tool: '{tool_name}'. Available tools: {list(self._get_tool_implementations())}",
                request_id=tool_id,
            )
        try:
            result = impl(**arguments)
            return ToolResponse.success_response(result, request_id=tool_id)
        except TypeError as exc:
            return ToolResponse.error_response(
                f"Invalid arguments for tool '{tool_name}': {exc}",
                request_id=tool_id,
            )
        except Exception as exc:
            return ToolResponse.error_response(str(exc), request_id=tool_id)

    def _get_tool_implementations(self) -> dict[str, callable]:
        """
        Return a mapping of tool name → callable for all tools available through
        this bridge.  Results are cached after the first call.
        """
        if not self._tool_impl_cache:
            # Register CST tools by importing the module and binding functions
            try:
                import agentix.tools.cst_tools as cst_mod
                import inspect
                for name, fn in inspect.getmembers(cst_mod, inspect.isfunction):
                    if not name.startswith("_"):
                        self._tool_impl_cache[name] = fn
            except Exception:
                pass
            # Register AST tools
            try:
                import agentix.tools.ast_tools as ast_mod
                import inspect
                for name, fn in inspect.getmembers(ast_mod, inspect.isfunction):
                    if not name.startswith("_"):
                        self._tool_impl_cache[name] = fn
            except Exception:
                pass
        return self._tool_impl_cache

    def _iter_llm_chunks(
        self,
        messages: list[dict],
        tools: Optional[list[dict]] = None,
    ) -> Iterator[ResponseChunk]:
        """
        Low-level streaming iterator over the OpenAI-compat Ollama endpoint.

        Handles all chunk types from the ``/v1/chat/completions`` streaming
        format:
        - content delta  → CONTENT chunk
        - reasoning/thinking delta → THINKING chunk
        - tool_calls deltas → accumulated, emitted as TOOL_CALL chunk(s) on
          ``finish_reason == "tool_calls"``
        - finish_reason "stop" → DONE chunk

        OpenAI streaming tool-call format (arguments arrive incrementally)::

            {"choices": [{"delta": {"tool_calls": [
                {"index": 0, "id": "call_abc", "type": "function",
                 "function": {"name": "read_file", "arguments": ""}}
            ]}, "finish_reason": null}]}
            {"choices": [{"delta": {"tool_calls": [
                {"index": 0, "function": {"arguments": "{\"path\":"}}
            ]}}]}
            {"choices": [{"delta": {"tool_calls": [
                {"index": 0, "function": {"arguments": " \"/tmp/x\"}"}}
            ]}}]}
            {"choices": [{"delta": {}, "finish_reason": "tool_calls"}]}

        Args:
            messages: Full chat history in OpenAI message format.
            tools: Optional list of OpenAI-format tool schemas to include.

        Yields:
            ResponseChunk objects.
        """
        payload: dict = {
            "model": self.config.model,
            "messages": messages,
            "temperature": self.config.temperature,
        }
        if tools:
            payload["tools"] = tools
            payload["tool_choice"] = "auto"

        # Accumulate tool call fragments keyed by index.
        # Structure: {index: {"id": str, "name": str, "arguments": str}}
        pending_tool_calls: dict[int, dict] = {}

        try:
            for chunk in query_api_streaming(self.config, payload):
                if chunk.get("error"):
                    yield ResponseChunk(type=ChunkType.ERROR, content=chunk["error"])
                    return

                choices = chunk.get("choices", [])
                if not choices:
                    if chunk.get("done"):
                        yield ResponseChunk(type=ChunkType.DONE, done_reason="stop")
                    continue

                choice = choices[0]
                delta = choice.get("delta", {})
                finish_reason = choice.get("finish_reason")

                # ── Thinking / reasoning ──────────────────────────────────
                reasoning = delta.get("reasoning") or delta.get("thinking")
                if reasoning:
                    yield ResponseChunk(type=ChunkType.THINKING, content=reasoning)

                # ── Regular content ───────────────────────────────────────
                content = delta.get("content", "")
                if content:
                    yield ResponseChunk(type=ChunkType.CONTENT, content=content)

                # ── Tool call deltas (accumulate fragments) ───────────────
                for tc_delta in delta.get("tool_calls", []):
                    idx = tc_delta.get("index", 0)
                    if idx not in pending_tool_calls:
                        pending_tool_calls[idx] = {"id": "", "name": "", "arguments": ""}
                    entry = pending_tool_calls[idx]
                    if tc_delta.get("id"):
                        entry["id"] = tc_delta["id"]
                    fn = tc_delta.get("function", {})
                    if fn.get("name"):
                        entry["name"] += fn["name"]
                    if fn.get("arguments"):
                        entry["arguments"] += fn["arguments"]

                # ── Finish: emit accumulated tool calls or DONE ───────────
                if finish_reason == "tool_calls":
                    for entry in sorted(pending_tool_calls.values(), key=lambda e: list(pending_tool_calls.values()).index(e)):
                        try:
                            parsed_args = json.loads(entry["arguments"]) if entry["arguments"] else {}
                        except json.JSONDecodeError:
                            parsed_args = {"_raw": entry["arguments"]}
                        yield ResponseChunk(
                            type=ChunkType.TOOL_CALL,
                            tool_name=entry["name"],
                            tool_input=parsed_args,
                            tool_id=entry["id"] or None,
                        )
                    pending_tool_calls.clear()

                elif finish_reason:
                    yield ResponseChunk(type=ChunkType.DONE, done_reason=finish_reason)
                    return

                if chunk.get("done"):
                    yield ResponseChunk(type=ChunkType.DONE, done_reason="stop")
                    return

                # Top-level thinking (some providers)
                top_thinking = chunk.get("thinking") or chunk.get("reasoning")
                if top_thinking:
                    yield ResponseChunk(type=ChunkType.THINKING, content=top_thinking)

        except Exception as exc:
            yield ResponseChunk(type=ChunkType.ERROR, content=f"Error generating response: {exc}")

    def _stream_direct_response(
        self,
        prompt: str,
        context: Context,
    ) -> Iterator[ResponseChunk]:
        """
        Stream a direct LLM response without tools.

        Args:
            prompt: User prompt
            context: Conversation context

        Yields:
            Content chunks from LLM
        """
        history = self._context_to_history(context)
        messages = [msg.to_llm_dict() for msg in history]
        messages.append({"role": "user", "content": prompt})
        yield from self._iter_llm_chunks(messages)

    def _stream_tool_response(
        self,
        prompt: str,
        context: Context,
        classification: PromptClassificationResponse,
    ) -> Iterator[ResponseChunk]:
        """
        Stream a response that may involve one or more tool calls.

        Sends the available tool schemas alongside the prompt so the LLM can
        choose which tool to invoke.  Tool calls detected in the stream are
        executed immediately (in parallel when multiple arrive in one round)
        and their results are fed back to the LLM for a final answer — the
        core single-round agentic loop.

        Args:
            prompt: User prompt
            context: Conversation context
            classification: Classification result (unused directly; tools chosen by LLM)

        Yields:
            TOOL_CALL, TOOL_RESULT, CONTENT, THINKING, DONE chunks.
        """
        yield from self._run_tool_loop(prompt, context, max_rounds=1)

    def _stream_planned_response(
        self,
        prompt: str,
        context: Context,
    ) -> Iterator[ResponseChunk]:
        """
        Stream a multi-step planned response using the tool loop.

        Uses the full configurable max_rounds so the LLM can chain multiple
        tool calls before producing a final answer.

        Args:
            prompt: User prompt
            context: Conversation context

        Yields:
            Planning, tool call, tool result, and content chunks.
        """
        max_rounds = getattr(self.config, "max_tool_rounds", 10)
        yield from self._run_tool_loop(prompt, context, max_rounds=max_rounds)

    def _run_tool_loop(
        self,
        prompt: str,
        context: Context,
        max_rounds: int = 10,
    ) -> Iterator[ResponseChunk]:
        """
        Core agentic tool loop.

        Each round:
        1. Submits the current message history (with tool schemas) to the LLM.
        2. Streams content/thinking chunks to the caller immediately.
        3. Collects any TOOL_CALL chunks emitted during the stream.
        4. If no tool calls arrived → done (LLM produced a final answer).
        5. Executes all tool calls in parallel via ThreadPoolExecutor.
        6. Appends assistant-with-tool-calls message and tool-result messages
           to the running history.
        7. Yields TOOL_RESULT chunks and loops.

        Inspired by the MIT-licensed lmstudio-python multi-round loop
        (sync_api.py:1326-1510).

        Args:
            prompt: User's original prompt.
            context: Conversation context (read-only; history is built locally).
            max_rounds: Maximum number of LLM→tool→LLM cycles.

        Yields:
            ResponseChunk objects of all types.
        """
        available_tools = self.get_available_tools()

        # Build initial message list from context + current prompt
        history = self._context_to_history(context)
        messages: list[dict] = [msg.to_llm_dict() for msg in history]
        messages.append({"role": "user", "content": prompt})

        # max_rounds = max tool-calling rounds; one extra final round forces a direct answer
        for round_index in range(max_rounds + 1):
            is_final_round = round_index == max_rounds
            tools_for_round = (available_tools or None) if not is_final_round else None

            tool_calls_this_round: list[ResponseChunk] = []
            final_content_chunks: list[ResponseChunk] = []

            # Stream this round; collect tool calls, pass content/thinking straight through
            for chunk in self._iter_llm_chunks(messages, tools=tools_for_round):
                if chunk.type == ChunkType.TOOL_CALL:
                    tool_calls_this_round.append(chunk)
                elif chunk.type == ChunkType.DONE:
                    if not tool_calls_this_round:
                        yield chunk
                    # Don't yield DONE yet if we have tool calls to process
                elif chunk.type in (ChunkType.CONTENT, ChunkType.THINKING):
                    final_content_chunks.append(chunk)
                    yield chunk
                else:
                    yield chunk

            # No tool calls → LLM produced a final answer; we're done
            if not tool_calls_this_round:
                break

            # Build the assistant message that records the tool call requests
            assistant_msg: dict = {
                "role": "assistant",
                "content": "".join(c.content for c in final_content_chunks),
                "tool_calls": [
                    {
                        "id": tc.tool_id or f"call_{i}",
                        "type": "function",
                        "function": {
                            "name": tc.tool_name,
                            "arguments": json.dumps(tc.tool_input or {}),
                        },
                    }
                    for i, tc in enumerate(tool_calls_this_round)
                ],
            }
            messages.append(assistant_msg)

            # Execute all tool calls in parallel; emit TOOL_CALL then TOOL_RESULT chunks
            tool_result_messages: list[dict] = []
            with ThreadPoolExecutor(max_workers=min(len(tool_calls_this_round), 4)) as pool:
                futures = {
                    pool.submit(
                        self.execute_tool,
                        tc.tool_name,
                        tc.tool_input or {},
                        tc.tool_id,
                    ): tc
                    for tc in tool_calls_this_round
                }
                tc_list = list(futures.values())
                for future in as_completed(futures):
                    tc = futures[future]
                    yield ResponseChunk(
                        type=tc.type,
                        tool_name=tc.tool_name,
                        tool_input=tc.tool_input,
                        tool_id=tc.tool_id,
                        round_index=round_index,
                    )

                    result: ToolResponse = future.result()
                    yield ResponseChunk(
                        type=ChunkType.TOOL_RESULT,
                        tool_name=tc.tool_name,
                        tool_output=result.output if result.success else result.error,
                        tool_id=tc.tool_id,
                        round_index=round_index,
                    )
                    tool_result_messages.append({
                        "role": "tool",
                        "tool_call_id": tc.tool_id or f"call_{tc_list.index(tc)}",
                        "content": result.to_llm_format(),
                    })

            messages.extend(tool_result_messages)

        yield ResponseChunk(type=ChunkType.DONE, done_reason="stop")


# Convenience function for quick usage
def create_bridge(
    model: str = "llama3.2",
    tools: Optional[list[str]] = None,
    debug: bool = False,
) -> AgentixBridge:
    """
    Create an AgentixBridge with sensible defaults.

    Args:
        model: Model name (default: llama3.2)
        tools: List of tool names (default: ["cst", "ast"])
        debug: Enable debug output

    Returns:
        Configured AgentixBridge instance
    """
    config = AgentixConfig(
        model=model,
        tools=tools or ["cst", "ast"],
        debug=debug,
    )
    return AgentixBridge(config)
