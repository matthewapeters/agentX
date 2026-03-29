"""
Programmatic API bridge for Agentix.

This module provides a clean programmatic interface to Agentix functionality
that can be consumed by AgentX GUI without going through the CLI.
"""

import glob
import json
import logging
import os
import sys
import time
import uuid
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
from shared.models.task_node import AssertionRecord, PlanRecord, PlanStep, SynthesisAttempt, TaskNodeRecord, TaskTree
from agentix.bridge.assertion_checker import extract_assertions, verify_assertion
from shared.models.message import Message
from shared.models.response import ChunkType, ResponseChunk
from shared.models.tools import ToolResponse
from .classify_prompt import classify_prompt as classifier

logger = logging.getLogger("agentix.bridge")


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
        working_memory: Optional["WorkingMemory"] = None,
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
            working_memory: Optional WorkingMemory instance for context-aware classification

        Returns:
            PromptClassificationResponse with intent and next_step
        """
        return classifier(
            self.config,
            prompt,
            context,
            self._context_to_history(context),
            self._get_max_tokens(),
            working_memory=working_memory,
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

        # Emit classification as a stream chunk so the GUI can display it
        if classification:
            yield ResponseChunk(
                type=ChunkType.CLASSIFICATION,
                classification={
                    "intent": classification.intent.name,
                    "next_step": classification.next_step.name,
                    "reasoning_summary": classification.reasoning_summary,
                    "needs_clarification": classification.needs_clarification,
                    "missing_fields": classification.missing_fields,
                },
            )

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
            result = [t for t in result if t.get("function", {}).get("name") not in self._disabled_tools]

        return result

    def set_enabled_tools(self, enabled_tool_names: list[str]) -> None:
        """
        Restrict the tools offered to the LLM to the given names.

        Call this when the user toggles tools in the GUI.  Passing an empty
        list disables all tools; passing ``None`` re-enables all tools.

        Args:
            enabled_tool_names: Tool names the user wants enabled.
        """
        all_names = {t.get("function", {}).get("name") for t in (self._extra_tool_schemas or [])}
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

    def retrigger_synthesis_streaming(
        self,
        node: TaskNodeRecord,
        context: Context,
        task_tree: TaskTree,
        hint: str = "",
    ) -> Iterator[ResponseChunk]:
        """Re-run synthesis for a completed node without re-running tool calls.

        Reconstructs the message thread for this node (using
        ``child_message_epochs`` to filter), appends a synthesis instruction
        with an optional user hint, streams a new synthesis, re-runs assertion
        checking, appends a non-destructive ``SynthesisAttempt``, persists the
        updated node and tree, and yields a ``TASK_NODE_END`` chunk with the
        new synthesis text and assertions.

        Args:
            node:       ``TaskNodeRecord`` to re-synthesise.
            context:    Current session context (provides stored messages).
            task_tree:  Live ``TaskTree`` index (updated in-place).
            hint:       Optional free-text guidance from the user.

        Yields:
            ``ResponseChunk`` objects (CONTENT, THINKING, ASSERTION_RESULT,
            TASK_NODE_END).
        """
        # Reconstruct the tool-call/result messages for this task node.
        all_msgs = context.get_messages(enabled_only=False)
        epoch_set: set[float] = set(node.child_message_epochs) if node.child_message_epochs else set()

        if epoch_set:
            task_messages: list[dict] = []
            for msg in all_msgs:
                msg_epoch = getattr(msg, "epoch", None)
                if msg_epoch is not None and msg_epoch in epoch_set:
                    try:
                        task_messages.append(msg.to_llm_dict())
                    except Exception:
                        pass
        else:
            # Fallback: use all LLM-visible messages.
            task_messages = context.to_llm_messages()

        hint_fragment = f"\n\nAdditional guidance from the user: {hint}" if hint.strip() else ""
        synthesis_instruction = (
            "Based on the tool calls and results above, write a concise, self-contained "
            "synthesis (50–200 words) that summarises the key findings as assertable facts. "
            "Do not call any tools." + hint_fragment
        )
        synthesis_messages = task_messages + [{"role": "user", "content": synthesis_instruction}]

        # Stream the new synthesis.
        new_synthesis = ""
        for chunk in self._iter_llm_chunks(synthesis_messages):
            if chunk.type in (ChunkType.TOOL_CALL, ChunkType.DONE):
                continue
            yield chunk
            if chunk.type == ChunkType.CONTENT and chunk.content:
                new_synthesis += chunk.content

        if not new_synthesis:
            new_synthesis = "(no synthesis produced)"

        # Re-run assertions on the new synthesis.
        attempt_epoch = time.time()
        node.assertions = []
        try:
            assertions = extract_assertions(new_synthesis, self._iter_llm_chunks)
            for a in assertions:
                verify_assertion(a, os.getcwd())
                node.assertions.append(a)
                yield ResponseChunk(
                    type=ChunkType.ASSERTION_RESULT,
                    plan_id=node.plan_id,
                    task_id=node.task_id,
                    assertions=[a.to_dict()],
                )
            failed = [a for a in assertions if a.verified is False]
            attempt_status = "rejected" if failed else "accepted"
        except Exception as exc:
            logger.debug("Assertion loop error during retrigger (non-fatal): %s", exc)
            attempt_status = "accepted"

        node.synthesis_attempts.append(SynthesisAttempt(epoch=attempt_epoch, status=attempt_status))
        node.status = "done"
        node.synthesis_epoch = attempt_epoch

        try:
            context.save_task_node(node)
            task_tree.nodes[node.task_id] = node
            context.save_task_tree(task_tree)
        except Exception as exc:
            logger.debug("Could not persist re-synthesised node %s: %s", node.task_id, exc)

        yield ResponseChunk(
            type=ChunkType.TASK_NODE_END,
            plan_id=node.plan_id,
            task_id=node.task_id,
            parent_task_id=node.parent_task_id,
            task_depth=node.depth,
            content=new_synthesis,
            assertions=[a.to_dict() for a in node.assertions],
        )

    def replay_task_node_streaming(
        self,
        node: "TaskNodeRecord",
        context: "Context",
        task_tree: "TaskTree",
    ) -> "Iterator[ResponseChunk]":
        """Re-run a task node completely from scratch — new tool loop + synthesis.

        Excludes the node's original child messages from the initial context so
        the LLM starts fresh, then delegates to ``_run_task_node``.  A new
        ``TaskNodeRecord`` epoch is **not** created — the existing ``task_id``
        is reused so the GUI updates the same row in-place.

        Args:
            node:       ``TaskNodeRecord`` to replay.
            context:    Current session context (provides base conversation history).
            task_tree:  Live ``TaskTree`` index (updated in-place).

        Yields:
            ``ResponseChunk`` objects exactly as ``_run_task_node`` does.
        """
        excluded: set[float] = set(node.child_message_epochs or [])
        base_messages: list[dict] = []
        for msg in context.get_messages(enabled_only=False):
            epoch = getattr(msg, "epoch", None)
            if epoch is not None and epoch in excluded:
                continue
            try:
                base_messages.append(msg.to_llm_dict())
            except Exception:
                pass

        description = node.tbd_resolved_description or node.task_description
        yield from self._run_task_node(
            plan_id=node.plan_id,
            task_id=node.task_id,
            task_description=description,
            parent_task_id=node.parent_task_id,
            depth=node.depth,
            plan_step_index=node.plan_step_index,
            tbd=False,
            context=context,
            task_tree=task_tree,
            initial_messages=base_messages,
        )

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
        # run_subtask must be intercepted inline inside _run_task_node BEFORE
        # execute_tool is called. If it reaches here it means the tool was invoked
        # outside the hierarchical task engine; return a clear error so the LLM
        # can handle it gracefully.
        if tool_name == "run_subtask":
            return ToolResponse.error_response(
                "run_subtask cannot be called directly. It is intercepted by the "
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

    def _stream_direct_response(
        self,
        prompt: str,
        context: Context,
    ) -> Iterator[ResponseChunk]:
        """
        Stream a direct LLM response without tools.

        No tool schemas are passed to the LLM.  If the model unexpectedly
        returns a TOOL_CALL chunk (e.g. from a fine-tuned model that ignores
        the absence of tool schemas), the entire buffered response is discarded
        and the request is transparently re-routed through the full tool loop
        (max 3 rounds) so the user still gets a coherent answer and any unknown
        tool name is reported as an error result rather than leaving the
        response hanging.

        Args:
            prompt: User prompt
            context: Conversation context

        Yields:
            Content chunks from LLM
        """
        history = self._context_to_history(context)
        messages = [msg.to_llm_dict() for msg in history]
        messages.append({"role": "user", "content": prompt})

        # Buffer chunks so we can detect a rogue TOOL_CALL before yielding anything.
        buffered: list[ResponseChunk] = []
        has_tool_calls = False
        for chunk in self._iter_llm_chunks(messages):
            if chunk.type == ChunkType.TOOL_CALL:
                has_tool_calls = True
            buffered.append(chunk)

        if has_tool_calls:
            # LLM attempted tool use in direct-response mode; fall back so the
            # tool executor can handle (and report) unknown / valid tools.
            yield from self._run_tool_loop(prompt, context, max_rounds=3)
            return

        yield from buffered

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
        yield from self._run_tool_loop(prompt, context, max_rounds=10)

    def _stream_planned_response(
        self,
        prompt: str,
        context: Context,
    ) -> Iterator[ResponseChunk]:
        """
        Stream a multi-step planned response using the hierarchical task engine.

        Calls the planner LLM to obtain a structured plan then executes each
        step via _run_task_node (depth 0), forwarding all emitted chunks to the
        caller.  Falls back silently to _run_tool_loop if the planner fails.
        """
        try:
            existing_plans = context.load_plans() or []
        except Exception:
            existing_plans = []

        plan, task_tree = self._create_plan(prompt, context, session_plan_index=len(existing_plans))

        if plan is None or task_tree is None:
            max_rounds = getattr(self.config, "max_tool_rounds", 10)
            yield from self._run_tool_loop(prompt, context, max_rounds=max_rounds)
            return

        yield ResponseChunk(
            type=ChunkType.PLAN_START,
            plan_id=plan.plan_id,
            plan_name=plan.plan_name,
            content=plan.plan_name,
        )

        history = self._context_to_history(context)
        base_messages: list[dict] = [msg.to_llm_dict() for msg in history]
        try:
            from agentix.context.prompts import get_system_prompt

            sys_content = get_system_prompt(self.config)
            if sys_content:
                base_messages.insert(0, {"role": "system", "content": sys_content})
        except Exception:
            pass

        plan.status = "running"
        try:
            context.save_plan(plan)
        except Exception:
            pass

        yield from self._run_plan(plan, context, task_tree, base_messages, prompt)

        plan.status = "done"
        try:
            context.save_plan(plan)
        except Exception:
            pass

        yield ResponseChunk(type=ChunkType.DONE, done_reason="stop")

    # ── Hierarchical task execution helpers (Phase 2) ─────────────────────────

    def _load_prompt_file(self, name: str) -> "Optional[str]":
        """Load a system prompt by name (without extension) from SYSTEM_PROMPTS_DIR."""
        try:
            from agentix.constants import SYSTEM_PROMPTS_DIR

            matches = glob.glob(f"{SYSTEM_PROMPTS_DIR}{name}.*")
            if not matches:
                return None
            with open(matches[0], "r", encoding="utf-8") as fh:
                return fh.read()
        except Exception:
            return None

    @staticmethod
    def _extract_plan_json(raw: str) -> dict:
        """Extract a JSON object from an LLM response that may contain surrounding text."""
        raw = raw.strip()
        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            pass
        start = raw.find("{")
        end = raw.rfind("}")
        if start != -1 and end > start:
            try:
                return json.loads(raw[start : end + 1])
            except json.JSONDecodeError:
                pass
        raise ValueError(f"No valid JSON found in planner response (length={len(raw)})")

    def _create_plan(
        self,
        prompt: str,
        context: "Context",
        session_plan_index: int = 0,
    ) -> "tuple[Optional[PlanRecord], Optional[TaskTree]]":
        """
        Call the planner LLM to produce a structured plan, then persist it.

        Returns (PlanRecord, TaskTree) on success, (None, None) on failure.
        """
        planner_text = self._load_prompt_file("planner_prompt")
        if not planner_text:
            logger.warning("planner_prompt.md not found; falling back to tool loop")
            return None, None

        history = self._context_to_history(context)
        messages: list[dict] = [msg.to_llm_dict() for msg in history]
        messages.insert(0, {"role": "system", "content": planner_text})
        messages.append({"role": "user", "content": prompt})

        raw_content = ""
        for chunk in self._iter_llm_chunks(messages, tools=None):
            if chunk.type == ChunkType.CONTENT:
                raw_content += chunk.content

        try:
            plan_data = self._extract_plan_json(raw_content)
        except (ValueError, json.JSONDecodeError) as exc:
            logger.warning("Planner JSON parse failed: %s", exc)
            return None, None

        plan_name = plan_data.get("plan_name", f"Plan {session_plan_index + 1}")
        steps_data = plan_data.get("steps", [])

        if not steps_data:
            logger.warning("Planner returned empty steps list")
            return None, None

        steps = [
            PlanStep(
                step_id=s.get("id", f"step_{i}"),
                description=s.get("description") or str(s.get("inputs", "")),
                tbd=s.get("tbd", False),
                depends_on=s.get("depends_on", []),
            )
            for i, s in enumerate(steps_data)
        ]

        plan_id = f"plan_{uuid.uuid4().hex[:8]}"
        plan = PlanRecord(
            plan_id=plan_id,
            plan_name=plan_name,
            session_plan_index=session_plan_index,
            steps=steps,
            root_task_ids=[s.step_id for s in steps if not s.depends_on],
            status="pending",
            epoch=time.time(),
        )

        task_tree = TaskTree(
            session_id=context.session_id or "",
            created_epoch=time.time(),
            last_updated_epoch=time.time(),
        )
        task_tree.add_plan(plan)

        try:
            context.save_plan(plan)
            context.save_task_tree(task_tree)
        except Exception as exc:
            logger.debug("Could not persist plan: %s", exc)

        return plan, task_tree

    def _resolve_tbd_step(
        self,
        step: "PlanStep",
        dep_syntheses: "list[str]",
        plan_name: str,
    ) -> str:
        """Ask the LLM to resolve a TBD step using prerequisite syntheses."""
        context_block = "\n".join(f"Result from {dep}: {synth}" for dep, synth in zip(step.depends_on, dep_syntheses))
        resolve_prompt = (
            f"You are executing plan '{plan_name}'."
            f" Step '{step.step_id}' is TBD. Its placeholder: \"{step.description}\""
            f"\n\nPrerequisite results:\n{context_block}"
            "\n\nWrite a single concrete description (<=15 words) for what this step should do."
            " Output ONLY the description string — no JSON, no explanation."
        )
        resolved = ""
        for chunk in self._iter_llm_chunks([{"role": "user", "content": resolve_prompt}], tools=None):
            if chunk.type == ChunkType.CONTENT:
                resolved += chunk.content
        return resolved.strip().strip('"').strip("'") or step.description

    def _run_task_node(
        self,
        *,
        plan_id: str,
        task_id: str,
        task_description: str,
        parent_task_id: "Optional[str]" = None,
        depth: int = 0,
        plan_step_index: int = 0,
        tbd: bool = False,
        context: "Context",
        task_tree: "TaskTree",
        initial_messages: "list[dict]",
        max_rounds: int = 10,
    ) -> "Iterator[ResponseChunk]":
        """Execute a single task node, recursing into sub-tasks via run_subtask."""
        max_task_depth: int = getattr(self.config, "max_task_depth", 10)

        yield ResponseChunk(
            type=ChunkType.TASK_NODE_START,
            plan_id=plan_id,
            task_id=task_id,
            parent_task_id=parent_task_id,
            task_depth=depth,
            tbd=tbd,
            content=task_description,
        )

        node = TaskNodeRecord(
            plan_id=plan_id,
            task_id=task_id,
            parent_task_id=parent_task_id,
            depth=depth,
            plan_step_index=plan_step_index,
            task_description=task_description,
            tbd=tbd,
            status="running",
            epoch=time.time(),
        )
        try:
            context.save_task_node(node)
            task_tree.add_node(node)
            context.save_task_tree(task_tree)
        except Exception as exc:
            logger.debug("Could not persist task node %s: %s", task_id, exc)

        # Build messages with task_execution prompt injected at position 0
        messages: list[dict] = list(initial_messages)
        task_exec_text = self._load_prompt_file("task_execution")
        if task_exec_text:
            task_exec_text = task_exec_text.replace("{depth}", str(depth)).replace("{max_depth}", str(max_task_depth))
            messages.insert(0, {"role": "system", "content": task_exec_text})
        if not messages or messages[-1].get("content") != task_description:
            messages.append({"role": "user", "content": task_description})

        available_tools = self.get_available_tools()
        max_task_depth: int = getattr(self.config, "max_task_depth", 10)
        if depth >= max_task_depth:
            available_tools = [t for t in available_tools if t.get("function", {}).get("name") != "run_subtask"]
        if depth >= max_task_depth:
            available_tools = [t for t in available_tools if t.get("function", {}).get("name") != "run_subtask"]

        any_tools_called = False
        got_content = False
        synthesis_text = ""

        for round_index in range(max_rounds):
            tool_calls_this_round: list[ResponseChunk] = []
            content_chunks: list[ResponseChunk] = []

            for chunk in self._iter_llm_chunks(messages, tools=available_tools or None):
                if chunk.type == ChunkType.TOOL_CALL:
                    tool_calls_this_round.append(chunk)
                elif chunk.type in (ChunkType.CONTENT, ChunkType.THINKING):
                    content_chunks.append(chunk)
                    yield chunk
                    if chunk.type == ChunkType.CONTENT and chunk.content:
                        got_content = True
                        synthesis_text += chunk.content
                elif chunk.type == ChunkType.DONE:
                    pass
                else:
                    yield chunk

            if not tool_calls_this_round:
                break

            any_tools_called = True
            got_content = False
            synthesis_text = ""

            assistant_msg: dict = {
                "role": "assistant",
                "content": "".join(c.content for c in content_chunks),
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

            subtask_calls = [tc for tc in tool_calls_this_round if tc.tool_name == "run_subtask"]
            regular_calls = [tc for tc in tool_calls_this_round if tc.tool_name != "run_subtask"]
            tool_result_messages: list[dict] = []

            for tc in subtask_calls:
                yield ResponseChunk(
                    type=ChunkType.TOOL_CALL,
                    tool_name=tc.tool_name,
                    tool_input=tc.tool_input,
                    tool_id=tc.tool_id,
                    round_index=round_index,
                    plan_id=plan_id,
                    task_id=task_id,
                )
                sub_args = tc.tool_input or {}
                sub_task_id = f"subtask_{uuid.uuid4().hex[:8]}"
                sub_description = sub_args.get("task", "")
                sub_synthesis = ""
                for sub_chunk in self._run_task_node(
                    plan_id=plan_id,
                    task_id=sub_task_id,
                    task_description=sub_description,
                    parent_task_id=task_id,
                    depth=depth + 1,
                    plan_step_index=0,
                    tbd=False,
                    context=context,
                    task_tree=task_tree,
                    initial_messages=[],
                    max_rounds=max_rounds,
                ):
                    yield sub_chunk
                    if sub_chunk.type == ChunkType.TASK_NODE_END:
                        sub_synthesis = sub_chunk.content or ""
                node.child_task_ids.append(sub_task_id)
                yield ResponseChunk(
                    type=ChunkType.TOOL_RESULT,
                    tool_name=tc.tool_name,
                    tool_output=sub_synthesis,
                    tool_id=tc.tool_id,
                    round_index=round_index,
                    plan_id=plan_id,
                    task_id=task_id,
                )
                tool_result_messages.append(
                    {
                        "role": "tool",
                        "tool_call_id": tc.tool_id or f"call_{sub_task_id}",
                        "content": sub_synthesis,
                    }
                )

            if regular_calls:
                tc_list = list(regular_calls)
                with ThreadPoolExecutor(max_workers=min(len(regular_calls), 4)) as pool:
                    futures = {
                        pool.submit(self.execute_tool, tc.tool_name, tc.tool_input or {}, tc.tool_id): tc
                        for tc in regular_calls
                    }
                    for future in as_completed(futures):
                        tc = futures[future]
                        yield ResponseChunk(
                            type=ChunkType.TOOL_CALL,
                            tool_name=tc.tool_name,
                            tool_input=tc.tool_input,
                            tool_id=tc.tool_id,
                            round_index=round_index,
                            plan_id=plan_id,
                            task_id=task_id,
                        )
                        result: ToolResponse = future.result()
                        yield ResponseChunk(
                            type=ChunkType.TOOL_RESULT,
                            tool_name=tc.tool_name,
                            tool_output=result.output if result.success else result.error,
                            tool_id=tc.tool_id,
                            round_index=round_index,
                            plan_id=plan_id,
                            task_id=task_id,
                        )
                        tool_result_messages.append(
                            {
                                "role": "tool",
                                "tool_call_id": tc.tool_id or f"call_{tc_list.index(tc)}",
                                "content": result.to_llm_format(),
                            }
                        )

            messages.extend(tool_result_messages)

        # Guaranteed synthesis when tools were used but no final text was produced
        if any_tools_called and not got_content:
            synthesis_messages = messages + [
                {
                    "role": "user",
                    "content": (
                        "You have gathered sufficient information through your tool calls. "
                        "Now write your complete synthesis for this task. "
                        "Do not call any more tools."
                    ),
                }
            ]
            for chunk in self._iter_llm_chunks(synthesis_messages):
                if chunk.type == ChunkType.TOOL_CALL:
                    continue
                if chunk.type == ChunkType.DONE:
                    continue
                yield chunk
                if chunk.type == ChunkType.CONTENT and chunk.content:
                    synthesis_text += chunk.content
                    got_content = True
            if not synthesis_text:
                synthesis_text = "(no synthesis produced)"

        # Assertion extraction and re-synthesis loop (Phase 3)
        final_synthesis = synthesis_text
        try:
            max_retries: int = getattr(self.config, "max_synthesis_retries", 3)
            for attempt_num in range(max_retries + 1):
                attempt_epoch = time.time()
                assertions = extract_assertions(final_synthesis, self._iter_llm_chunks)
                failed_assertions: list[AssertionRecord] = []

                for a in assertions:
                    verify_assertion(a, os.getcwd())
                    node.assertions.append(a)
                    yield ResponseChunk(
                        type=ChunkType.ASSERTION_RESULT,
                        plan_id=plan_id,
                        task_id=task_id,
                        assertions=[a.to_dict()],
                    )
                    if a.verified is False:
                        failed_assertions.append(a)

                if not failed_assertions:
                    node.synthesis_attempts.append(SynthesisAttempt(epoch=attempt_epoch, status="accepted"))
                    break

                node.synthesis_attempts.append(SynthesisAttempt(epoch=attempt_epoch, status="rejected"))
                if attempt_num >= max_retries:
                    break

                failure_details = "\n".join(f"- {a.fact}: {a.error or 'check failed'}" for a in failed_assertions)
                resynth_messages = messages + [
                    {
                        "role": "user",
                        "content": (
                            "Your previous synthesis had the following assertion failures:\n"
                            f"{failure_details}\n\n"
                            f"Previous (rejected) synthesis:\n{final_synthesis}\n\n"
                            "Please write a corrected synthesis that addresses the above "
                            "failures. Keep it self-contained and 50-200 words. "
                            "Do not call any tools."
                        ),
                    }
                ]
                retry_text = ""
                for chunk in self._iter_llm_chunks(resynth_messages):
                    if chunk.type in (ChunkType.TOOL_CALL, ChunkType.DONE):
                        continue
                    yield chunk
                    if chunk.type == ChunkType.CONTENT and chunk.content:
                        retry_text += chunk.content
                if retry_text:
                    final_synthesis = retry_text

        except Exception as exc:
            logger.debug("Assertion loop error (non-fatal): %s", exc)

        node.status = "done"
        node.synthesis_epoch = time.time()
        try:
            context.save_task_node(node)
            task_tree.nodes[task_id] = node
            context.save_task_tree(task_tree)
        except Exception as exc:
            logger.debug("Could not persist completed task node %s: %s", task_id, exc)

        yield ResponseChunk(
            type=ChunkType.TASK_NODE_END,
            plan_id=plan_id,
            task_id=task_id,
            parent_task_id=parent_task_id,
            task_depth=depth,
            content=final_synthesis,
        )

    def _run_plan(
        self,
        plan: "PlanRecord",
        context: "Context",
        task_tree: "TaskTree",
        base_messages: "list[dict]",
        original_prompt: str,
    ) -> "Iterator[ResponseChunk]":
        """Execute all plan steps, resolving TBD steps at runtime, then synthesise."""
        max_rounds: int = getattr(self.config, "max_tool_rounds", 10)
        step_syntheses: dict[str, str] = {}

        for i, step in enumerate(plan.steps):
            # Skip steps whose dependencies are not yet satisfied
            unsatisfied = [d for d in step.depends_on if d not in step_syntheses]
            if unsatisfied:
                logger.warning(
                    "Step %s deps not yet completed: %s; skipping",
                    step.step_id,
                    unsatisfied,
                )
                continue

            effective_description = step.description

            # Resolve TBD step description using predecessor syntheses
            if step.tbd:
                dep_synths = [step_syntheses.get(d, "") for d in step.depends_on]
                resolved = self._resolve_tbd_step(step, dep_synths, plan.plan_name)
                effective_description = resolved
                yield ResponseChunk(
                    type=ChunkType.TASK_NODE_TBD,
                    plan_id=plan.plan_id,
                    task_id=step.step_id,
                    content=resolved,
                )
                try:
                    context.save_plan(plan)
                except Exception:
                    pass

            # Build per-step message set (inject dep context if present)
            step_messages = list(base_messages)
            if step.depends_on:
                dep_context = "\n".join(
                    f"Result from {dep}: {step_syntheses[dep]}" for dep in step.depends_on if dep in step_syntheses
                )
                if dep_context:
                    step_messages.append(
                        {
                            "role": "system",
                            "content": f"Context from completed prerequisite steps:\n{dep_context}",
                        }
                    )

            step_synthesis = ""
            for chunk in self._run_task_node(
                plan_id=plan.plan_id,
                task_id=step.step_id,
                task_description=effective_description,
                parent_task_id=None,
                depth=0,
                plan_step_index=i,
                tbd=step.tbd,
                context=context,
                task_tree=task_tree,
                initial_messages=step_messages,
                max_rounds=max_rounds,
            ):
                yield chunk
                if chunk.type == ChunkType.TASK_NODE_END and chunk.task_id == step.step_id:
                    step_synthesis = chunk.content or ""

            step_syntheses[step.step_id] = step_synthesis

        # Final cross-step synthesis emitted as regular CONTENT stream
        if step_syntheses:
            combined = "\n\n".join(f"Step {sid}:\n{synth}" for sid, synth in step_syntheses.items())
            final_messages = list(base_messages) + [
                {"role": "user", "content": original_prompt},
                {
                    "role": "assistant",
                    "content": ("I have gathered the following information through my research:\n\n" + combined),
                },
                {
                    "role": "user",
                    "content": (
                        "Based on all the information above, please provide your complete "
                        "and final answer to my original request."
                    ),
                },
            ]
            for chunk in self._iter_llm_chunks(final_messages):
                if chunk.type == ChunkType.TOOL_CALL:
                    continue
                if chunk.type == ChunkType.DONE:
                    continue
                yield chunk

    def _run_tool_loop(
        self,
        prompt: str,
        context: Context,
        max_rounds: int = 10,
        depth: int = 0,
        plan_id: Optional[str] = None,
        task_id: Optional[str] = None,
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

        After at most max_rounds of tool calling a guaranteed synthesis round is
        run with no tool schemas so the LLM always produces a user-facing
        content response.  This is separate from the max_rounds cap so a
        LLM that retries an errored tool in the "final" round still gets a
        chance to synthesise.

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

        # Prepend configured system prompt(s) before history so the LLM sees
        # tool-use instructions (e.g. "use WM cwd for list_directory") first.
        if self.config.system:
            from agentix.context.prompts import get_system_prompt

            sys_content = get_system_prompt(self.config)
            if sys_content:
                messages.insert(0, {"role": "system", "content": sys_content})

        messages.append({"role": "user", "content": prompt})

        any_tools_called = False
        got_content = False

        for round_index in range(max_rounds):
            tool_calls_this_round: list[ResponseChunk] = []
            final_content_chunks: list[ResponseChunk] = []

            # Stream this round (always with tools — synthesis handled below)
            for chunk in self._iter_llm_chunks(messages, tools=available_tools or None):
                if chunk.type == ChunkType.TOOL_CALL:
                    tool_calls_this_round.append(chunk)
                elif chunk.type == ChunkType.DONE:
                    if not tool_calls_this_round:
                        yield chunk
                elif chunk.type in (ChunkType.CONTENT, ChunkType.THINKING):
                    final_content_chunks.append(chunk)
                    yield chunk
                    if chunk.type == ChunkType.CONTENT and chunk.content:
                        got_content = True
                else:
                    yield chunk

            # No tool calls → LLM gave a direct answer; we're done
            if not tool_calls_this_round:
                break

            any_tools_called = True
            got_content = False  # reset — content before tool calls doesn't count as synthesis

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
                    tool_result_messages.append(
                        {
                            "role": "tool",
                            "tool_call_id": tc.tool_id or f"call_{tc_list.index(tc)}",
                            "content": result.to_llm_format(),
                        }
                    )

            messages.extend(tool_result_messages)

        # ── Guaranteed synthesis step ────────────────────────────────────────
        # If tool calls were made and the loop exited without producing content
        # (e.g. max_rounds exhausted, or LLM retried a tool in the last round),
        # run one final LLM call with NO tool schemas to force a text response.
        if any_tools_called and not got_content:
            # Append a directive so the model synthesises instead of calling tools.
            # Fine-tuned models sometimes call tools even without tool schemas;
            # the explicit instruction reliably steers them toward a text answer.
            synthesis_messages = messages + [
                {
                    "role": "user",
                    "content": (
                        "You have gathered enough information through your tool calls. "
                        "Now write your complete response to the original request. "
                        "Do not call any more tools."
                    ),
                }
            ]
            for chunk in self._iter_llm_chunks(synthesis_messages):
                # Don't forward unexpected tool-call chunks from the synthesis
                # phase: they will not be executed and would leave orphaned
                # tool-call nodes in the GUI.
                if chunk.type == ChunkType.TOOL_CALL:
                    continue
                yield chunk
                if chunk.type == ChunkType.CONTENT and chunk.content:
                    got_content = True
            if not got_content:
                # Absolute fallback so the user is never left with silence
                yield ResponseChunk(type=ChunkType.CONTENT, content="\n")

        yield ResponseChunk(type=ChunkType.DONE, done_reason="stop")


def run_subtask(task: str, scratch_file: Optional[str] = None) -> str:
    """Request execution of a focused sub-task with its own bounded tool loop.

    Creates a child task node that runs within a fresh context window using the
    task_execution system prompt. On completion its synthesis (50-200 words) is
    returned as the result of this tool call.

    Use run_subtask when:
    - Investigation of a sub-problem would pollute the current context window.
    - A focused, bounded scope produces a cleaner result than continued tool calls.
    - Intermediate results need to be passed via a scratch file.

    Do NOT use run_subtask for simple tool calls — call the tool directly instead.

    Args:
        task: Complete, self-contained description of the sub-task including
              relevant file paths, scope, and success criteria. The sub-task has
              NO access to the parent task's context window.
        scratch_file: Optional relative path within the session scratch directory
                      for passing large intermediate data between tasks.

    Returns:
        Synthesis text (50-200 words) summarising the sub-task result.
    """
    raise NotImplementedError(
        "run_subtask must be intercepted by _run_task_node's tool loop before "
        "execute_tool is reached. This function exists solely for schema extraction."
    )


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
