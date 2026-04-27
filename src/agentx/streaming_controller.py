"""
StreamingController — owns all LLM streaming logic for AgentXSession.

Extracted from AgentXSession to give streaming a clear single responsibility.
Uses a back-reference to the owning session so it can read/write session state
(context, working_memory, gui, etc.) without duplicating it.

Design note: all session-owned attributes are accessed via ``self._s.<attr>``
so that tests which construct a partial session via ``AgentXSession.__new__``
and patch individual attributes still work correctly.
"""

import json
import logging
import os
import threading
from typing import TYPE_CHECKING, Callable, Optional

from shared.models.message import Message, MessageRole
from shared.models.response import ChunkType, ResponseChunk

if TYPE_CHECKING:
    from .session import AgentXSession

logger = logging.getLogger(__name__)


class StreamingController:
    """
    Owns all streaming logic that was previously scattered across
    AgentXSession: display helpers, the main Agentix streaming loop, and the
    background workers for re-synthesis and task-node replay.

    The controller holds a back-reference (``self._s``) to the owning
    ``AgentXSession`` and reads/writes session state exclusively through it so
    that unit tests can keep their ``session.foo = MagicMock()`` patterns.
    """

    def __init__(self, session: "AgentXSession") -> None:
        self._s = session

    # ------------------------------------------------------------------
    # Streaming state callbacks (named to avoid repeated inline lambdas)
    # ------------------------------------------------------------------

    def _on_stream_start(self) -> None:
        """Set the GUI streaming state to active."""
        self._s.gui.set_streaming_state(True)

    def _on_stream_end(self) -> None:
        """Set the GUI streaming state to idle."""
        self._s.gui.set_streaming_state(False)

    # ------------------------------------------------------------------
    # Thinking / content display helpers
    # ------------------------------------------------------------------

    def _display_thinking(self, text: str) -> None:
        """Display thinking text with a once-per-turn header."""
        s = self._s
        if not getattr(s, "_thinking_header_shown", False):
            header = f"\n\U0001f4ad ({s.active_model})\t(The agent is thinking...)\n"
            s._safe_root_after(lambda: s.gui.display_agent_thinking(header))
            s._write_log(header)
            s._thinking_header_shown = True
        s._safe_root_after(lambda: s.gui.display_agent_thinking(text))
        s._write_log(text)

    def _display_assistant_header(self) -> None:
        """Display the assistant header once per response stream."""
        s = self._s
        if not getattr(s, "_assistant_header_shown", False):
            s._assistant_header_shown = True
            header = f"\n\n\U0001f916 ({s.active_model})\t"
            s._safe_root_after(lambda: s.gui.display_agent_response(header))
            s._write_log(header)

    def _handle_stream_content(self, text: str) -> None:
        """Ensure header is shown before streaming content chunks."""
        self._display_assistant_header()
        self._s._safe_root_after(lambda: self._s.gui.display_agent_response(text))
        self._s._write_log(text)

    # ------------------------------------------------------------------
    # Tool call / result display helpers
    # ------------------------------------------------------------------

    def _display_tool_call(
        self,
        tool_name: str,
        tool_input: dict,
        round_index: int | None = None,
        tool_id: str | None = None,
        cloned_from: str | None = None,
        task_id: str | None = None,
    ) -> str:
        """
        Display a tool call in the GUI and store it in context.

        The bridge handles tool execution; this method is display-only.
        Storing the TOOL_CALL message ensures the tool interaction is visible
        in the session history and can be re-serialized in future turns.
        """
        s = self._s
        round_label = f" [round {round_index + 1}]" if round_index is not None else ""
        line = f"\n[🔧 Calling tool{round_label}: {tool_name}]\n"
        s._safe_root_after(lambda: s.gui.display_agent_response(line))
        s._write_log(line)
        try:
            input_text = f"{tool_name}: {json.dumps(tool_input, ensure_ascii=False)}"
        except Exception:
            input_text = f"{tool_name}: {tool_input}"
        s._output_logger.log("tool_call", input_text)
        msg = s.context.add_tool_call_message(tool_name, tool_input, tool_id=tool_id)
        dirty = False
        if cloned_from:
            msg.cloned_from = cloned_from
            dirty = True
        if task_id:
            msg.task_id = task_id
            dirty = True
        if dirty and msg.file_path:
            msg.save(os.path.dirname(msg.file_path))
        return msg.message_id

    def _display_tool_result(
        self,
        tool_name: str,
        output,
        round_index: int | None = None,
        tool_id: str | None = None,
        cloned_from: str | None = None,
        task_id: str | None = None,
    ) -> str:
        """
        Display a tool result in the GUI and store it in context.

        The bridge has already executed the tool and produced ``output``.
        This method records the result in context so it persists across
        sessions and is included in future LLM history.
        """
        s = self._s
        if isinstance(output, str):
            display_text = output
        elif output is not None:
            try:
                display_text = json.dumps(output)
            except Exception:
                display_text = str(output)
        else:
            display_text = ""

        round_label = f" [round {round_index + 1}]" if round_index is not None else ""
        preview = display_text[:100] + "..." if len(display_text) > 100 else display_text
        result_line = f"\n[📋 Tool result{round_label}: {preview}]\n"
        s._safe_root_after(lambda: s.gui.display_agent_response(result_line))
        s._write_log(result_line)
        s._output_logger.log("tool_result", f"{tool_name}: {display_text}")
        msg = s.context.add_tool_result_message(
            tool_name=tool_name,
            tool_output=output,
            tool_id=tool_id,
        )
        dirty = False
        if cloned_from:
            msg.cloned_from = cloned_from
            dirty = True
        if task_id:
            msg.task_id = task_id
            dirty = True
        if dirty and msg.file_path:
            msg.save(os.path.dirname(msg.file_path))
        s._safe_root_after(s.refresh_working_memory_gui)
        return msg.message_id

    # ------------------------------------------------------------------
    # Message persistence
    # ------------------------------------------------------------------

    def _persist_stream_messages(
        self,
        thinking_text: str,
        content_text: str,
        synthesis_of: list[str] | None = None,
        refresh_gui: bool = True,
    ) -> None:
        """Persist streamed thinking and assistant content to context."""
        from datetime import datetime

        s = self._s
        if thinking_text:
            thinking_message = Message(role=MessageRole.THINKING, content=thinking_text)
            thinking_message.enabled = False
            if refresh_gui:
                s.add_message_to_context(thinking_message)
            else:
                s.context.add_message(thinking_message, ts=datetime.now())

        if content_text:
            assistant_message = Message(role=MessageRole.ASSISTANT, content=content_text)
            assistant_message.enabled = True
            assistant_message.synthesis_of = synthesis_of or []
            if refresh_gui:
                s.add_message_to_context(assistant_message)
            else:
                s.context.add_message(assistant_message, ts=datetime.now())

    # ------------------------------------------------------------------
    # Classification logging and callback
    # ------------------------------------------------------------------

    def _log_classification(self, classification, prompt: str) -> None:
        """Log classification decision to session.log."""
        s = self._s
        if not classification:
            s._write_log("🤔 intent: (classification disabled)\n")
            return

        intent_str = getattr(classification.intent, "name", classification.intent)
        next_step_str = getattr(classification.next_step, "name", classification.next_step)
        s._write_log(f"🤔 intent: {intent_str}\n")
        s._write_log(f"   reasoning: {classification.reasoning_summary}\n")

        if getattr(classification, "needs_clarification", False):
            s._write_log("   ⚠️  needs clarification\n")
            missing = getattr(classification, "missing_fields", [])
            if missing:
                s._write_log(f"   missing: {', '.join(missing)}\n")

        if s.working_memory and s.working_memory.all_facts():
            wm_facts = s.working_memory.get_enabled_facts()
            if wm_facts:
                key_facts = [f for f in wm_facts if f.key in ("use_tools", "cwd", "project")]
                if key_facts:
                    fact_strs = [f"{f.key}={f.value}" for f in key_facts]
                    s._write_log(f"   🏛️  WM context: {', '.join(fact_strs)}\n")

        s._write_log(f"💡 path: {next_step_str}\n\n")
        s._session_log.flush()

    def _make_classification_callback(self, config: dict) -> Callable:
        """Build the on_classification callback respecting field-level display config."""
        s = self._s
        cd = config.get("agentix", {}).get("classification_display", {})
        if not cd.get("enabled", True):
            return lambda meta: None

        show_intent = cd.get("show_intent", True)
        show_reasoning = cd.get("show_reasoning", True)
        show_clarification = cd.get("show_clarification", True)
        show_next_step = cd.get("show_next_step", True)

        def _callback(meta: dict) -> None:
            filtered = {
                "intent": meta.get("intent", "") if show_intent else "",
                "reasoning_summary": meta.get("reasoning_summary", "") if show_reasoning else "",
                "needs_clarification": meta.get("needs_clarification", False) if show_clarification else False,
                "missing_fields": meta.get("missing_fields") if show_clarification else [],
                "next_step": meta.get("next_step", "") if show_next_step else "",
            }
            s._safe_root_after(lambda m=filtered: s.gui.display_classification(m))
            lines = []
            if filtered.get("intent"):
                lines.append(f"🤔 intent: {filtered['intent']}")
            if filtered.get("reasoning_summary"):
                lines.append(f"   reasoning: {filtered['reasoning_summary']}")
            if filtered.get("needs_clarification") or filtered.get("missing_fields"):
                cl = "   clarification needed: yes"
                mf = filtered.get("missing_fields") or []
                if mf:
                    cl += f"  |  missing fields: {', '.join(mf)}"
                lines.append(cl)
            if filtered.get("next_step"):
                lines.append(f"💡 path: {filtered['next_step']}")
            if lines:
                s._write_log("\n".join(lines) + "\n")
            s._output_logger.log("classification", json.dumps(filtered, ensure_ascii=False))

        return _callback

    # ------------------------------------------------------------------
    # Main Agentix streaming loop
    # ------------------------------------------------------------------

    def stream_via_agentix(self) -> None:
        """Stream response through Agentix middleware (runs on background thread)."""
        from datetime import datetime

        from .integration import ResponseHandler

        s = self._s
        config = s.config

        s._is_streaming.set()
        s._safe_root_after(self._on_stream_start)
        s._safe_root_after(s.refresh_user_gui)

        # Use captured prompt from submit; fall back to cached input for tests.
        prompt = s._pending_prompt or ""
        s._pending_prompt = None

        if not prompt and not s.message.attachments:
            prompt = s.gui.get_cached_user_input()
            if not prompt:
                s._safe_root_after(lambda: s.gui.display_error("No input provided."))
                s._is_streaming.clear()
                s._safe_root_after(self._on_stream_end)
                return

        import os

        attachment_filenames = [os.path.basename(att.file_path) for att in s.message.attachments]
        s._safe_root_after(lambda: s.gui.display_user_message(prompt, attachment_filenames, datetime.now()))
        s._write_log(f"\n👤 User: {prompt}\n")
        s._output_logger.log("user", prompt)

        try:
            s.message.content = prompt
            s.message.enabled = True
            s._safe_root_after(s.refresh_context_gui)

            for att in s.enabled_history_attachments:
                if att not in s.message.attachments:
                    s.message.attachments.append(att)

            shared_context = s._build_shared_context()
            if hasattr(s, "_model_store") and hasattr(s, "_schedule_meter_redraw"):
                max_tokens = s._model_store.get_context_length(s.active_model)
                breakdown = shared_context.token_breakdown(model_name=s.active_model)
                s._schedule_meter_redraw(max_tokens, breakdown)
            s.add_message_to_context(s.message)

            s.message = Message(role="user", content="")
            s.enabled_history_attachments = []

            classification = None
            if config.get("agentix", {}).get("classify_prompts", True):
                classification = s.agentix_adapter.classify_prompt_sync(prompt, shared_context, s.working_memory)
                self._log_classification(classification, prompt)

            s._assistant_header_shown = False
            s._thinking_header_shown = False

            thinking_parts: list[str] = []
            content_parts: list[str] = []
            stream_tool_result_ids: list[str] = []
            # Mutable box so on_tool_call/on_tool_result closures pick up the
            # task_id that was set by the most recent TASK_NODE_START chunk.
            _current_task_id: list[str | None] = [None]

            handler = ResponseHandler(
                on_content=lambda text: self._handle_stream_content(text),
                on_thinking=lambda text: self._display_thinking(text),
                on_tool_call=lambda name, args, round_i=None, tool_id=None: self._display_tool_call(
                    name, args, round_i, tool_id=tool_id, task_id=_current_task_id[0]
                ),
                on_tool_result=lambda tool_name, output, round_i=None, tool_id=None: stream_tool_result_ids.append(
                    self._display_tool_result(tool_name, output, round_i, tool_id=tool_id, task_id=_current_task_id[0])
                ),
                on_error=lambda msg, code: s._safe_root_after(lambda: s.gui.display_error(f"{code}: {msg}")),
                on_classification=self._make_classification_callback(config),
            )

            for chunk in s.agentix_adapter.process_prompt_generator(prompt, shared_context, classification):
                if not s._is_streaming.is_set():
                    break

                handler.process_chunk(chunk)

                if chunk.type == ChunkType.THINKING and chunk.content:
                    thinking_parts.append(chunk.content)
                elif chunk.type == ChunkType.CONTENT and chunk.content:
                    content_parts.append(chunk.content)

                # Plan tree chunk routing
                if chunk.type == ChunkType.PLAN_START and chunk.plan_id:
                    _pid = chunk.plan_id
                    _pname = chunk.plan_name or "Plan"
                    _on_export = lambda pid=_pid: s._export_task_tree(pid)
                    s._safe_root_after(
                        lambda pid=_pid, pn=_pname, exp=_on_export: (
                            s.gui.add_plan_tab(pid, pn, on_export=exp),
                            s.gui.focus_plan_tab(pid),
                        )
                    )
                    _plan_msg = Message(
                        role=MessageRole.PLAN,
                        content=_pname,
                        plan_id=_pid,
                        plan_name=_pname,
                    )
                    s.context.add_message(_plan_msg)
                elif chunk.type == ChunkType.TASK_NODE_START and chunk.task_id:
                    _tid = chunk.task_id
                    _current_task_id[0] = _tid  # stamp subsequent tool calls with this task
                    _pid = chunk.plan_id or ""
                    _desc = chunk.content or chunk.task_id
                    _par = chunk.parent_task_id
                    _depth = chunk.task_depth or 0
                    _tbd = bool(chunk.tbd)
                    _on_replay = lambda tid=_tid: s._replay_subtask(tid)
                    if _par:
                        s._safe_root_after(
                            lambda tid=_tid, par=_par, desc=_desc, d=_depth, rep=_on_replay: s.gui.add_plan_subtask_node(
                                tid, par, desc, d, on_replay=rep
                            )
                        )
                    else:
                        s._safe_root_after(
                            lambda pid=_pid, tid=_tid, desc=_desc, tb=_tbd, rep=_on_replay: s.gui.add_plan_step_node(
                                pid, tid, desc, tb, on_replay=rep
                            )
                        )
                elif chunk.type == ChunkType.TASK_NODE_TBD and chunk.task_id:
                    _tid = chunk.task_id
                    _desc = chunk.content or ""
                    s._safe_root_after(lambda tid=_tid, desc=_desc: s.gui.resolve_plan_tbd_node(tid, desc))
                elif chunk.type == ChunkType.TASK_NODE_END and chunk.task_id:
                    _tid = chunk.task_id
                    _synth = chunk.content or ""
                    _asserts = chunk.assertions or []
                    _node_pid = chunk.plan_id or ""
                    _node_depth = chunk.task_depth or 0
                    s._safe_root_after(lambda tid=_tid: s.gui.update_plan_node_status(tid, "done"))

                    def _make_callbacks(tid=_tid):
                        on_resynth = lambda hint: s.retrigger_synthesis(tid, hint)
                        on_add_wm = lambda key, val: s._add_wm_hint_for_task(tid, key, val)
                        return on_resynth, on_add_wm

                    _on_resynth, _on_add_wm = _make_callbacks()
                    s._safe_root_after(
                        lambda tid=_tid, synth=_synth, asserts=_asserts, cb=_on_resynth, wm=_on_add_wm: s.gui.add_plan_synthesis(
                            tid, synth, asserts, on_resynth=cb, on_add_wm_hint=wm
                        )
                    )
                    _node_msg = Message(
                        role=MessageRole.TASK_NODE,
                        content=_synth,
                        plan_id=_node_pid,
                        task_id=_tid,
                        task_depth=_node_depth,
                    )
                    s.context.add_message(_node_msg)
                elif chunk.type == ChunkType.TOOL_CALL and chunk.task_id:
                    _tid = chunk.task_id
                    _tname = chunk.tool_name or ""
                    _tinput = chunk.tool_input or {}
                    s._safe_root_after(lambda tid=_tid, tn=_tname, ti=_tinput: s.gui.add_plan_tool_call(tid, tn, ti))

                s._safe_root_after(s.refresh_user_gui)

            s._safe_root_after(s.gui.display_spacing)
            joined_thinking = "".join(thinking_parts)
            joined_content = "".join(content_parts)
            self._persist_stream_messages(joined_thinking, joined_content, synthesis_of=stream_tool_result_ids)
            if joined_thinking:
                s._output_logger.log("thinking", joined_thinking)
            if joined_content:
                s._output_logger.log("agent", joined_content)
            s._safe_root_after(s.refresh_user_gui)

        except Exception as e:
            logger.exception("Request error during streaming")
            err_line = f"\n⚠️  ERROR: {e}\n"
            s._safe_root_after(lambda err=e: s.gui.display_error(f"Error: {err}"))
            s._write_log(err_line)
            s._output_logger.log("error", str(e))
        finally:
            s._is_streaming.clear()
            if hasattr(s, "_context_meter_payload") and hasattr(s, "_schedule_meter_redraw"):
                max_tokens, breakdown = s._context_meter_payload(model_name=s.active_model)
                s._schedule_meter_redraw(max_tokens, breakdown)
            s._safe_root_after(self._on_stream_end)

    # ------------------------------------------------------------------
    # Background worker: retrigger synthesis
    # ------------------------------------------------------------------

    def _run_retrigger_synthesis_worker(self, node, tree, task_id: str, hint: str) -> None:
        """Spawn the background thread for re-synthesis.

        Called from ``AgentXSession.retrigger_synthesis`` after guard checks
        and working-memory injection have been performed.
        """
        s = self._s

        def _worker(_node=node, _tree=tree, _tid=task_id, _hint=hint):
            try:
                s._is_streaming.set()
                s._safe_root_after(self._on_stream_start)
                for chunk in s.agentix_adapter.retrigger_synthesis_generator(_node, s.context, _tree, _hint):
                    if chunk.type == ChunkType.TASK_NODE_END and chunk.task_id == _tid:
                        _synth = chunk.content or ""
                        _asserts = chunk.assertions or []
                        s._safe_root_after(lambda tid=_tid: s.gui.update_plan_node_status(tid, "done"))
                        s._safe_root_after(
                            lambda tid=_tid, synth=_synth, asserts=_asserts: s.gui.update_plan_synthesis(
                                tid, synth, asserts
                            )
                        )
            except Exception as exc:
                logger.exception("retrigger_synthesis worker error")
                s._safe_root_after(lambda err=exc: s.gui.display_error(f"Re-synthesis error: {err}"))
            finally:
                s._is_streaming.clear()
                s._safe_root_after(self._on_stream_end)
                s._safe_root_after(s.refresh_user_gui)

        s._last_synthesis_thread = threading.Thread(target=_worker, daemon=True)
        s._last_synthesis_thread.start()

    # ------------------------------------------------------------------
    # Background worker: replay task node
    # ------------------------------------------------------------------

    def _run_replay_subtask_worker(self, node, tree, task_id: str) -> None:
        """Spawn the background thread for task-node replay.

        Called from ``AgentXSession._replay_subtask`` after guard checks.
        """
        s = self._s

        def _worker(_node=node, _tree=tree, _tid=task_id):
            replay_tool_result_ids: list[str] = []
            replay_tool_call_pairs: list[tuple[str, str]] = []
            replay_tool_result_pairs: list[tuple[str, str]] = []
            replay_assistant_pair: tuple[str, str] | None = None
            original_tool_calls: list[Message] = [
                msg
                for msg in s.context.get_messages(enabled_only=False)
                if msg.role == MessageRole.TOOL_CALL and msg.enabled and msg.task_id == _tid
            ]
            original_tool_results: list[Message] = [
                msg
                for msg in s.context.get_messages(enabled_only=False)
                if msg.role == MessageRole.TOOL_RESULT and msg.enabled and msg.task_id == _tid
            ]
            original_assistants: list[Message] = [
                msg
                for msg in s.context.get_messages(enabled_only=False)
                if msg.role == MessageRole.ASSISTANT and msg.enabled and msg.task_id == _tid
            ]
            original_tool_call_index = 0
            original_tool_result_index = 0
            try:
                s._is_streaming.set()
                s._safe_root_after(self._on_stream_start)
                for chunk in s.agentix_adapter.replay_task_node_generator(_node, s.context, _tree):
                    if chunk.type == ChunkType.TOOL_CALL and chunk.tool_name:
                        cloned_from = None
                        if original_tool_call_index < len(original_tool_calls):
                            cloned_from = original_tool_calls[original_tool_call_index].message_id
                            original_tool_call_index += 1
                        replay_tool_call_id = self._display_tool_call(
                            chunk.tool_name,
                            chunk.tool_input or {},
                            chunk.round_index,
                            tool_id=chunk.tool_id,
                            cloned_from=cloned_from,
                            task_id=_tid,
                        )
                        if cloned_from:
                            replay_tool_call_pairs.append((cloned_from, replay_tool_call_id))
                    elif chunk.type == ChunkType.TOOL_RESULT and chunk.tool_name:
                        cloned_from = None
                        if original_tool_result_index < len(original_tool_results):
                            cloned_from = original_tool_results[original_tool_result_index].message_id
                            original_tool_result_index += 1
                        tool_result_id = self._display_tool_result(
                            chunk.tool_name,
                            chunk.tool_output,
                            chunk.round_index,
                            tool_id=chunk.tool_id,
                            cloned_from=cloned_from,
                            task_id=_tid,
                        )
                        replay_tool_result_ids.append(tool_result_id)
                        if cloned_from:
                            replay_tool_result_pairs.append((cloned_from, tool_result_id))

                    if chunk.type == ChunkType.TASK_NODE_END and chunk.task_id == _tid:
                        _synth = chunk.content or ""
                        _asserts = chunk.assertions or []
                        replay_synthesis = Message(role=MessageRole.ASSISTANT, content=_synth)
                        replay_synthesis.synthesis_of = replay_tool_result_ids
                        replay_synthesis.task_id = _tid
                        if original_assistants:
                            replay_synthesis.cloned_from = original_assistants[-1].message_id
                        s.context.add_message(replay_synthesis)
                        if replay_synthesis.cloned_from:
                            replay_assistant_pair = (replay_synthesis.cloned_from, replay_synthesis.message_id)

                        # Apply supersession only after replay outputs are fully persisted.
                        for original_id, replacement_id in replay_tool_call_pairs:
                            s.context.supersede_message(original_id, replacement_id)
                        for original_id, replacement_id in replay_tool_result_pairs:
                            s.context.supersede_message(original_id, replacement_id)
                        if replay_assistant_pair is not None:
                            s.context.supersede_message(replay_assistant_pair[0], replay_assistant_pair[1])

                        s._safe_root_after(lambda tid=_tid: s.gui.update_plan_node_status(tid, "done"))
                        s._safe_root_after(
                            lambda tid=_tid, synth=_synth, asserts=_asserts: s.gui.update_plan_synthesis(
                                tid, synth, asserts
                            )
                        )
            except Exception as exc:
                logger.exception("replay_subtask worker error")
                s._safe_root_after(lambda err=exc: s.gui.display_error(f"Replay error: {err}"))
            finally:
                s._is_streaming.clear()
                s._safe_root_after(self._on_stream_end)
                s._safe_root_after(s.refresh_user_gui)

        s._last_replay_thread = threading.Thread(target=_worker, daemon=True)
        s._last_replay_thread.start()
