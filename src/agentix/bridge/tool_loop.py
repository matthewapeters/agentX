"""
Core agentic tool-loop logic, extracted from AgentixBridge for independent
testability and a cleaner separation of concerns.

``ToolLoopRunner`` owns:
- Tool registry (implementations + OpenAI schemas)
- The low-level LLM streaming primitive (``_iter_llm_chunks``)
- Tool execution (``execute_tool``)
- History conversion utility (``_context_to_history``)

``AgentixBridge`` uses ``ToolLoopRunner`` via composition and exposes delegation
wrappers so the existing public API is unchanged for callers.
"""

import json
import logging
from typing import Iterator, Optional

from agentix.agentix_config import AgentixConfig
from agentix.api_client import query_api_streaming
from agentix.models import get_model
from agentix.tools import extract_cst_tools
from agentix.tools.describe_tools import to_openai_tools
from shared.models.context import Context
from shared.models.message import Message
from shared.models.response import ChunkType, ResponseChunk
from shared.models.tools import ToolResponse

logger = logging.getLogger("agentix.bridge.tool_loop")

# ── Tool name constants ────────────────────────────────────────────────────────
# Single source of truth for special tool names referenced in dispatch logic.
SUBTASK_TOOL_NAME = "run_subtask"
CST_TOOL_FAMILY = "cst"


class ToolLoopRunner:
    """
    Encapsulates the core agentic tool-loop logic.

    This class handles low-level LLM streaming and tool execution so that
    AgentixBridge can focus on higher-level coordination and its public API
    surface.  Instantiate directly in tests to verify tool-loop behaviour
    in isolation from the full bridge.

    Args:
        config: Runtime configuration (model name, temperature, tools, …).
    """

    def __init__(self, config: AgentixConfig) -> None:
        self.config = config
        self._max_tokens: Optional[int] = None
        # Callable implementations keyed by tool name (populated lazily + via register_tool_implementations)
        self._tool_impl_cache: dict[str, callable] = {}
        # Extra OpenAI-format schemas for externally registered tools
        self._extra_tool_schemas: list[dict] = []
        # Tool names the user has disabled via the GUI; None means "all enabled"
        self._disabled_tools: Optional[set[str]] = None

    # ── Tool registry ────────────────────────────────────────────────────────

    def get_available_tools(self) -> list[dict]:
        """
        Return available tools with metadata in OpenAI tools format.

        If ``set_enabled_tools()`` has been called only the named tools are
        returned; disabled tools are omitted so the LLM never sees them.
        """
        tools = []
        for tool_name in self.config.tools or []:
            if tool_name == CST_TOOL_FAMILY:
                tools.extend(extract_cst_tools())

        result = to_openai_tools(tools) if tools else []
        result.extend(self._extra_tool_schemas)

        if self._disabled_tools is not None:
            result = [t for t in result if t.get("function", {}).get("name") not in self._disabled_tools]

        return result

    def set_enabled_tools(self, enabled_tool_names: list[str]) -> None:
        """
        Restrict the tools offered to the LLM to the given names.

        Passing an empty list disables all tools; passing ``None`` re-enables
        all tools.
        """
        all_names = {t.get("function", {}).get("name") for t in (self._extra_tool_schemas or [])}
        self._disabled_tools = all_names - set(enabled_tool_names)

    def register_tool_implementations(
        self,
        impls: "dict[str, callable]",
        schemas: "list[dict] | None" = None,
    ) -> None:
        """
        Register additional tool implementations.

        Args:
            impls: Mapping of tool name → callable.
            schemas: Optional list of OpenAI-format tool schemas.
        """
        self._tool_impl_cache.update(impls)
        if schemas:
            self._extra_tool_schemas.extend(schemas)

    def _get_tool_implementations(self) -> dict[str, callable]:
        """
        Return a mapping of tool name → callable for all available tools.
        Results are cached after the first call.
        """
        if not self._tool_impl_cache:
            try:
                import inspect

                import agentix.tools.cst_tools as cst_mod

                for name, fn in inspect.getmembers(cst_mod, inspect.isfunction):
                    if not name.startswith("_"):
                        self._tool_impl_cache[name] = fn
            except Exception:
                pass
            try:
                import inspect

                import agentix.tools.ast_tools as ast_mod

                for name, fn in inspect.getmembers(ast_mod, inspect.isfunction):
                    if not name.startswith("_"):
                        self._tool_impl_cache[name] = fn
            except Exception:
                pass
        return self._tool_impl_cache

    # ── LLM client ───────────────────────────────────────────────────────────

    def _get_max_tokens(self) -> int:
        """Return max tokens for the current model (cached)."""
        if self._max_tokens is None:
            self._max_tokens = get_model(self.config, max_tokens=self.config.model_max_tokens)
        return self._max_tokens

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
                    for entry in sorted(
                        pending_tool_calls.values(), key=lambda e: list(pending_tool_calls.values()).index(e)
                    ):
                        try:
                            parsed_args = json.loads(entry["arguments"]) if entry["arguments"] else {}
                        except json.JSONDecodeError as e:
                            logger.warning(
                                f"Tool call arguments failed JSON parsing: {e}",
                                extra={
                                    "tool_name": entry.get("name"),
                                    "tool_id": entry.get("id"),
                                    "error_pos": e.pos,
                                    "arguments_repr": repr(entry["arguments"]),
                                    "arguments_length": len(entry["arguments"]) if entry["arguments"] else 0,
                                    "arguments_content": entry["arguments"],
                                },
                            )
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

    # ── Tool execution ───────────────────────────────────────────────────────

    def execute_tool(self, tool_name: str, arguments: dict, tool_id: Optional[str] = None) -> ToolResponse:
        """
        Execute a named tool with the given arguments.

        Looks up the tool in the registered implementations, calls it, and
        returns a ``ToolResponse``.  Exceptions are caught and returned as
        error responses (error-as-result pattern — the agentic loop never
        crashes due to a single tool failure).

        Args:
            tool_name: Name of the tool to invoke.
            arguments: Keyword arguments to pass to the tool implementation.
            tool_id: Optional tool call ID from the LLM for correlation.

        Returns:
            ToolResponse with success=True and output, or success=False and error.
        """
        # run_subtask must be intercepted inline inside _run_task_node BEFORE
        # execute_tool is called. If it reaches here it means the tool was invoked
        # outside the hierarchical task engine; return a clear error so the LLM
        # can handle it gracefully.
        if tool_name == SUBTASK_TOOL_NAME:
            return ToolResponse.error_response(
                f"{SUBTASK_TOOL_NAME} cannot be called directly. It is intercepted by the "
                "hierarchical task engine and re-routed to a recursive _run_task_node "
                "call. Ensure you are inside a planned response flow.",
                request_id=tool_id,
            )

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

    # ── Utility ──────────────────────────────────────────────────────────────

    def _context_to_history(self, context: Context) -> list[Message]:
        """Convert AgentX Context to Agentix history format."""
        return list(context.get_enabled_messages())
