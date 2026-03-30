"""
Docstring for agentx.session
"""

import json
import logging
import os
import subprocess
import threading
import tkinter as tk
from datetime import UTC, datetime
from typing import Any, Iterator, Optional

import httpx

from agentix.prompt_classification_response import PromptClassificationResponse
from shared.models.context import Context
from shared.models.message import Message, MessageRole
from shared.models.response import ChunkType, ResponseChunk
from shared.models.working_memory import FactOwner, WorkingMemory

from .attachment_info import AttachmentInfo
from .config import save_config
from .file_explorer import FileExplorer
from .gui.gui_config import GUIConfig
from .gui.gui_manager import GUIManager
from .history import History
from .integration import (
    AdvancedToolRegistry,
    AgentixBridgeAdapter,
    ClientToolExecutor,
    ResponseHandler,
    ServerToolExecutor,
    agentix_bridge_adapter,
)
from .output_logger import OutputLogger
from .service_manager import ServiceManager

logger = logging.getLogger(__name__)


def create_adapter(config: dict) -> agentix_bridge_adapter.AgentixBridgeAdapter:
    return agentix_bridge_adapter.create_adapter(config)


class AgentXSession:
    """
    AgentXSession
    """

    def __init__(
        self,
        root: Optional[tk.Tk] = None,
        config: Optional[dict[str, Any]] = None,
        username: Optional[str] = None,
        session_dir: Optional[str] = None,
    ):
        if config is None:
            raise ValueError("config is required")

        self.root = root or tk.Tk()
        if root is None:
            self.root.withdraw()
        self.config = config

        session_started_at = datetime.now(UTC)
        self.session_id = f"session_{session_started_at.strftime('%Y-%m-%d_%H-%M-%S')}"
        self.context = Context()
        self.file_explorer = FileExplorer(start_path=os.getcwd())
        self.user = username or os.getenv("USER") or os.getenv("USERNAME") or "User"
        self.start_time = session_started_at.strftime("%Y-%m-%d %H:%M:%S UTC")
        # Title will be set by GUIManager after initialization
        base_dir = session_dir or os.getcwd()
        self.user_history_folder = os.path.join(
            base_dir,
            "sessions",
            self.user,
        )
        self.session_folder = os.path.join(self.user_history_folder, self.session_id)
        os.makedirs(self.session_folder, exist_ok=True)
        self.context_folder = os.path.join(self.session_folder, "context")
        os.makedirs(self.context_folder, exist_ok=True)
        self.context.path = self.context_folder
        self.context.session_id = self.session_id
        # Session transcript log — mirrors everything written to the output panel
        self._session_log_path = os.path.join(self.session_folder, "session.log")
        self._session_log = open(self._session_log_path, "a", encoding="utf-8", buffering=1)  # line-buffered
        self._output_logger = OutputLogger(self.session_folder)
        self._history = None  # Placeholder for History object
        self.message = Message(role="user", content="")
        self.enabled_history_attachments = []  # Track enabled attachments from history
        self._is_streaming = threading.Event()
        self._streaming_thread = None
        self._pending_prompt: Optional[str] = None

        # Initialize service manager for external services
        self.service_manager = ServiceManager(config)

        # Initialize GUIManager
        gui_config = GUIConfig.from_dict(config)
        self.gui = GUIManager(
            root=self.root,
            config=gui_config,
            on_submit=self._handle_submit,
            on_interrupt=self._handle_interrupt,
            on_attachment_toggle=self._handle_attachment_toggle,
        )

        # Set window title with session info
        self.gui.set_window_title(f"{self.user} - AgentX Session - {self.start_time}")

        # Initialize Agentix bridge (always integrated)
        self.agentix_adapter = create_adapter(config)
        self.agentix_adapter.agentix_config.session = self.session_id

        # Initialize tool executors
        self.client_tool_executor = ClientToolExecutor(base_path=os.getcwd())
        self.server_tool_executor = ServerToolExecutor(agentix_bridge=self.agentix_adapter.bridge)
        self.advanced_tools = AdvancedToolRegistry(agentix_bridge=self.agentix_adapter.bridge)

        # Initialize Working Memory — loaded from session folder (or empty on new session)
        wm_config = config.get("agentx", {}).get("working_memory", {})
        if wm_config.get("enabled", True):
            self.working_memory: Optional[WorkingMemory] = WorkingMemory.load(self.session_folder)
            self.working_memory.set_path(self.session_folder)
            # Seed startup working directory as a user-owned fact for tool/prompt context.
            cwd = os.getcwd()
            self.working_memory.add_fact(FactOwner.USER, "UserName", self.user)
            self.working_memory.add_fact(FactOwner.USER, "cwd", cwd)
            project_name = self._detect_git_project_name(cwd)
            if project_name:
                self.working_memory.add_fact(FactOwner.USER, "project", project_name)
            instructions_text = self._load_agentx_instructions(cwd)
            if instructions_text is not None:
                self.working_memory.add_fact(
                    FactOwner.USER,
                    "agentx-instructions",
                    instructions_text,
                )
            self.agentix_adapter.register_working_memory_tools(self.working_memory)
        else:
            self.working_memory = None

        # Initialize active model from config
        self._active_model = config["agentx"]["ollama_model"]

    def _detect_git_project_name(self, cwd: str) -> Optional[str]:
        """Return repo name if cwd is inside a git worktree, otherwise None."""
        try:
            result = subprocess.run(
                ["git", "-C", cwd, "rev-parse", "--show-toplevel"],
                capture_output=True,
                text=True,
                check=False,
                timeout=2,
            )
        except (OSError, subprocess.SubprocessError):
            return None

        if result.returncode != 0:
            return None

        repo_root = (result.stdout or "").strip()
        if not repo_root:
            return None
        return os.path.basename(repo_root).lower()

    def _load_agentx_instructions(self, cwd: str) -> Optional[str]:
        """Load .agentx/agentx-instructions.md contents when present in cwd."""
        instructions_path = os.path.join(cwd, ".agentx", "agentx-instructions.md")
        if not os.path.isfile(instructions_path):
            return None
        try:
            with open(instructions_path, "r", encoding="utf-8") as f:
                content = f.read()
        except OSError:
            return None
        return content

    def _load_bootstrap_prompt(self, cwd: str) -> Optional[str]:
        """Load .agentx/bootstrap-prompt.md contents when present in cwd."""
        prompt_path = os.path.join(cwd, ".agentx", "bootstrap-prompt.md")
        if not os.path.isfile(prompt_path):
            return None
        try:
            with open(prompt_path, "r", encoding="utf-8") as f:
                content = f.read().strip()
        except OSError:
            return None
        return content or None

    def _log_classification(self, classification: Optional[PromptClassificationResponse], prompt: str) -> None:
        """
        Log classification decision to session.log.

        Args:
            classification: Classification result or None if disabled
            prompt: The user prompt that was classified
        """
        if not classification:
            self._session_log.write("🤔 intent: (classification disabled)\n")
            return

        intent_str = getattr(classification.intent, "name", classification.intent)
        next_step_str = getattr(classification.next_step, "name", classification.next_step)
        self._session_log.write(f"🤔 intent: {intent_str}\n")
        self._session_log.write(f"   reasoning: {classification.reasoning_summary}\n")

        if getattr(classification, "needs_clarification", False):
            self._session_log.write("   ⚠️  needs clarification\n")
            missing = getattr(classification, "missing_fields", [])
            if missing:
                self._session_log.write(f"   missing: {', '.join(missing)}\n")

        # Show Working Memory context if available
        if self.working_memory and self.working_memory.all_facts():
            wm_facts = self.working_memory.get_enabled_facts()
            if wm_facts:
                key_facts = [f for f in wm_facts if f.key in ("use_tools", "cwd", "project")]
                if key_facts:
                    fact_strs = [f"{f.key}={f.value}" for f in key_facts]
                    self._session_log.write(f"   🏛️  WM context: {', '.join(fact_strs)}\n")

        self._session_log.write(f"💡 path: {next_step_str}\n\n")
        self._session_log.flush()

    def _build_stream_shared_context(self) -> Context:
        """Build prompt context from working memory, history, and enabled session messages."""
        shared_context = Context()

        wm_config = self.config.get("agentx", {}).get("working_memory", {})
        if (
            wm_config.get("enabled", True)
            and wm_config.get("inject_into_context", True)
            and self.working_memory is not None
        ):
            wm_block = self.working_memory.to_llm_block()
            if wm_block:
                wm_msg = Message(role=MessageRole.SYSTEM, content=wm_block)
                shared_context.add_message(wm_msg, ts=datetime.now())

        history_messages = self.history.get_enabled_messages()
        for ts, msg in history_messages:
            if hasattr(msg, "message"):
                msg = msg.message
            shared_context.add_message(msg, ts=ts)

        for msg in self.context.messages:
            if hasattr(msg, "message"):
                msg = msg.message
            if getattr(msg, "enabled", False):
                shared_context.add_message(msg, ts=msg.timestamp)

        return shared_context

    def _run_bootstrap_prompt_if_present(self) -> None:
        """Run hidden startup bootstrap prompt and render only final assistant content."""
        prompt = self._load_bootstrap_prompt(os.getcwd())
        if not prompt:
            return

        try:
            shared_context = self._build_stream_shared_context()
            classification = None
            if self.config.get("agentix", {}).get("classify_prompts", True):
                classification = self.agentix_adapter.classify_prompt_sync(prompt, shared_context, self.working_memory)
                self._log_classification(classification, prompt)

            content_parts: list[str] = []
            for chunk in self.agentix_adapter.process_prompt_generator(
                prompt,
                shared_context,
                classification,
            ):
                if chunk.type == ChunkType.CONTENT and chunk.content:
                    content_parts.append(chunk.content)

            response_text = "".join(content_parts).strip()
            if response_text:
                self.gui.display_bootstrap_agent_response(response_text)
                self._output_logger.log("bootstrap_agent", response_text)
                self.refresh_working_memory_gui()
        except Exception:
            logger.exception("Bootstrap prompt execution failed")

    def process_prompt(self, prompt: str) -> Iterator[ResponseChunk]:
        """Process a prompt and yield response chunks (test-friendly API)."""
        shared_context = self._build_shared_context_from_context()

        user_message = Message(role=MessageRole.USER, content=prompt)
        user_message.enabled = True
        self.context.add_message(user_message, ts=datetime.now())

        classification = None
        if self.config.get("agentix", {}).get("classify_prompts", False):
            classification = self.agentix_adapter.classify_prompt_sync(prompt, shared_context, self.working_memory)
            self._log_classification(classification, prompt)
            if classification:
                yield ResponseChunk(
                    type=ChunkType.THINKING,
                    content=classification.reasoning_summary or "",
                    classification=classification.__dict__,
                )

        thinking_parts: list[str] = []
        content_parts: list[str] = []
        for chunk in self.agentix_adapter.process_prompt_generator(prompt, shared_context, classification):
            if chunk.type == ChunkType.THINKING and chunk.content:
                thinking_parts.append(chunk.content)
            elif chunk.type == ChunkType.CONTENT and chunk.content:
                content_parts.append(chunk.content)
            elif chunk.type == ChunkType.PLAN_START and chunk.plan_id:
                _plan_msg = Message(
                    role=MessageRole.PLAN,
                    content=chunk.plan_name or "Plan",
                    plan_id=chunk.plan_id,
                    plan_name=chunk.plan_name or "Plan",
                )
                self.context.add_message(_plan_msg)
            elif chunk.type == ChunkType.TASK_NODE_END and chunk.task_id:
                _node_msg = Message(
                    role=MessageRole.TASK_NODE,
                    content=chunk.content or "",
                    plan_id=chunk.plan_id or "",
                    task_id=chunk.task_id,
                    task_depth=chunk.task_depth or 0,
                )
                self.context.add_message(_node_msg)
            yield chunk

        self._persist_stream_messages(
            "".join(thinking_parts),
            "".join(content_parts),
            refresh_gui=False,
        )

    @property
    def active_model(self) -> str:
        """
        Get the currently active Ollama model.

        This is the single source of truth for which model is being used.
        All Ollama calls should use this property.

        Returns:
            Model name (e.g., "gpt-oss", "llama3.2")
        """
        return self._active_model

    @active_model.setter
    def active_model(self, model: str) -> None:
        """
        Set the active Ollama model.

        Updates both the internal state and the config dictionary
        to keep them synchronized.

        Args:
            model: Model name to use
        """
        self._active_model = model
        self.config["agentx"]["ollama_model"] = model

        # Update the bridge's config
        self.agentix_adapter.agentix_config.model = model

    @property
    def history(self) -> "History":
        """
        Docstring for history

        :param self: Description
        :return: Description
        :rtype: History
        """
        if self._history is None:
            self._history = History(
                user_history_path=self.user_history_folder,
                exclude_session=self.context_folder,
            )
        return self._history

    @history.setter
    def history(self, value: "History"):
        self._history = value

    # Callback handlers for GUIManager

    def _handle_submit(self) -> None:
        """Handle user submit button click."""
        self.stream_ollama_response()

    def _handle_interrupt(self) -> None:
        """Handle user interrupt button click."""
        self.interrupt_streaming()

    def _handle_attachment_toggle(self, attachment_id: str, enabled: bool) -> None:
        """Handle attachment checkbox toggle from GUI.

        Args:
            attachment_id: Unique identifier of attachment (from AttachmentInfo)
            enabled: New enabled state
        """
        # Find the attachment by ID
        # ID is str(id(attachment)) from AttachmentInfo.from_attachment()
        for att in self.message.attachments:
            if str(id(att)) == attachment_id:
                att.enabled = enabled
                break

        # Also check history attachments
        for att in self.enabled_history_attachments:
            if str(id(att)) == attachment_id:
                # Toggle presence in enabled list
                if enabled and att not in self.enabled_history_attachments:
                    self.enabled_history_attachments.append(att)
                elif not enabled and att in self.enabled_history_attachments:
                    self.enabled_history_attachments.remove(att)
                break

        # Refresh display
        self.refresh_user_gui()

    def _setup_agentix_ui(self) -> None:
        """Setup model selector and tool panel from Agentix."""
        # Setup model change callback by updating the ModelSelector's callback directly
        # (Since ModelSelector was already created with a reference to the old callback)
        original_callback = self.gui._on_model_change

        def on_model_change(model: str):
            print(f"Model selector changed to: {model}")
            self.active_model = model  # Use property setter for 3-way sync
            print(f"Session.active_model updated to: {self.active_model}")
            if callable(original_callback):
                original_callback(model)

        self.gui._on_model_change = on_model_change
        if self.gui.model_selector:
            self.gui.model_selector.on_model_change = on_model_change

        # Always populate models from Agentix (integrated and always available)
        try:
            models = self.agentix_adapter.get_models()
            if models:
                self.gui.populate_models(models, initial_model=self.active_model)
        except Exception as e:
            print(f"Error loading models: {e}")

        # Setup tool callbacks
        original_tool_toggle = self.gui._on_tool_toggle

        def on_tool_toggle(tool_name: str, enabled: bool):
            enabled_tools = self.gui.get_enabled_tools()
            self.config["agentix"]["available_tools"] = enabled_tools
            self.agentix_adapter.set_enabled_tools(enabled_tools)
            original_tool_toggle(tool_name, enabled)

        self.gui._on_tool_toggle = on_tool_toggle

        # Populate tools from Agentix
        try:
            tools = self.agentix_adapter.get_tools()
            if tools:
                self.gui.populate_tools(tools)
        except Exception as e:
            print(f"Error loading tools: {e}")

    def execute_tool(self, tool_name: str, tool_input: dict) -> str:
        """
        Execute a tool (either client-side or server-side).

        Routes to appropriate executor based on tool type and availability:
        - CLIENT tools: Execute via ClientToolExecutor
        - SERVER tools: Execute via ServerToolExecutor
        - CODE_ANALYSIS: Execute via ServerToolExecutor (Agentix)
        - EITHER: Try client first, fall back to server

        Args:
            tool_name: Name of the tool to execute
            tool_input: Arguments for the tool

        Returns:
            Tool execution result as string
        """
        try:
            from .integration import CodeAnalysisTool

            # Client-side tool names
            client_tool_names = {"read_file", "list_directory", "write_file", "get_file_info", "search_files"}

            # Try client-side tools first
            if tool_name in client_tool_names:
                return self.client_tool_executor.execute(tool_name, tool_input)

            # Check if it's a code analysis tool
            if CodeAnalysisTool.is_code_analysis_tool(tool_name):
                if self.server_tool_executor.is_available():
                    return self.server_tool_executor.execute(tool_name, tool_input)
                return f"Code analysis tool '{tool_name}' not available - Agentix not connected"

            # Try server-side tools
            if self.server_tool_executor.is_available():
                return self.server_tool_executor.execute(tool_name, tool_input)

            # Unknown tool
            return f"Unknown tool: {tool_name}"

        except Exception as e:
            return f"Error executing tool '{tool_name}': {str(e)}"

    def handle_tool_call(self, tool_name: str, tool_input: dict) -> None:
        """
        Handle a tool call from the LLM response.

        This method:
        1. Stores the TOOL_CALL message in context
        2. Executes the tool
        3. Stores the TOOL_RESULT message in context
        4. Displays both in the GUI

        Args:
            tool_name: Name of the tool to call
            tool_input: Arguments for the tool
        """
        try:
            # Store TOOL_CALL message
            tool_call_msg = Message(
                role=MessageRole.TOOL_CALL,
                content=f"Calling tool: {tool_name}",
            )
            tool_call_msg.tool_name = tool_name
            tool_call_msg.tool_input = tool_input
            tool_call_msg.enabled = True

            self.add_message_to_context(tool_call_msg)

            # Display tool call in GUI
            self._safe_root_after(lambda: self.gui.display_agent_response(f"\n[🔧 Calling tool: {tool_name}]\n"))

            # Execute the tool
            result = self.execute_tool(tool_name, tool_input)

            # Store TOOL_RESULT message
            tool_result_msg = Message(
                role=MessageRole.TOOL_RESULT,
                content=result,
            )
            tool_result_msg.tool_name = tool_name
            tool_result_msg.enabled = True

            self.add_message_to_context(tool_result_msg)

            # Display tool result in GUI
            self._safe_root_after(
                lambda: self.gui.display_agent_response(
                    f"[📋 Tool result: {result[:100]}...]\n" if len(result) > 100 else f"[📋 Tool result: {result}]\n"
                )
            )

        except Exception as e:
            error_msg = f"Error handling tool call: {e}"
            self._safe_root_after(lambda: self.gui.display_error(error_msg))
            print(error_msg)

    def on_history_attachment_toggle(self, attachment, enabled: bool):
        """
        Callback when a history attachment is enabled or disabled.
        Updates the enabled_history_attachments list and refreshes the attachment bar.
        """
        if enabled:
            if attachment not in self.enabled_history_attachments:
                self.enabled_history_attachments.append(attachment)
        else:
            if attachment in self.enabled_history_attachments:
                self.enabled_history_attachments.remove(attachment)
        self.refresh_user_gui()

    def _on_plan_row_click(self, plan_id: str) -> None:
        """Handle a click on a plan row in the context panel.

        If the plan tab already exists in the output notebook, focus it.
        Otherwise reconstruct the tab from persisted plan/task-node records.
        """
        tab = self.gui.get_plan_tab_frame(plan_id)
        if tab is not None:
            self.gui.focus_plan_tab(plan_id)
        else:
            self._replay_plan_tab(plan_id)

    def _replay_plan_tab(self, plan_id: str) -> None:
        """Reconstruct a read-only plan tab from persisted JSON records.

        Loads the matching PlanRecord and all associated TaskNodeRecords from
        the session directory, creates the tab, and populates it with nodes
        and their synthesis text exactly as they appeared during execution.
        """
        plans = self.context.load_plans()
        plan_record = next((p for p in plans if p.plan_id == plan_id), None)
        if plan_record is None:
            return

        self.gui.add_plan_tab(plan_id, plan_record.plan_name, on_export=lambda: self._export_task_tree(plan_id))
        self.gui.focus_plan_tab(plan_id)

        task_nodes = self.context.load_task_nodes()
        plan_nodes = [n for n in task_nodes if n.plan_id == plan_id]
        plan_nodes.sort(key=lambda n: (n.depth, n.epoch))

        for node in plan_nodes:
            _on_replay = lambda tid=node.task_id: self._replay_subtask(tid)
            if node.parent_task_id:
                self.gui.add_plan_subtask_node(
                    node.task_id, node.parent_task_id, node.task_description, node.depth, on_replay=_on_replay
                )
            else:
                self.gui.add_plan_step_node(
                    plan_id, node.task_id, node.task_description, node.tbd, on_replay=_on_replay
                )

            if node.status == "done":
                self.gui.update_plan_node_status(node.task_id, "done")

    def _export_task_tree(self, plan_id: str) -> None:
        """Export the plan's task tree to a markdown file in the session folder.

        Loads the plan and its task nodes from disk, formats them as a nested
        markdown list (plan → steps → sub-tasks with synthesis), and writes
        the result to ``<session_folder>/task_tree_export.md``.

        Args:
            plan_id: ID of the plan to export.
        """
        from shared.models.task_node import TaskNodeRecord

        plans = self.context.load_plans()
        plan = next((p for p in plans if p.plan_id == plan_id), None)
        if plan is None:
            self.gui.display_error(f"Cannot export: plan '{plan_id}' not found.")
            return

        task_nodes = self.context.load_task_nodes()
        plan_nodes = [n for n in task_nodes if n.plan_id == plan_id]

        lines: list[str] = [
            f"# Plan: {plan.plan_name}",
            f"",
            f"Status: {plan.status}  |  Plan ID: {plan_id}",
            f"",
        ]

        def _format_node(node: TaskNodeRecord, level: int) -> None:
            indent = "  " * level
            status_icon = {"done": "✓", "failed": "✗", "running": "●", "pending": "○"}.get(node.status, "?")
            desc = node.tbd_resolved_description or (
                "[TBD] " + node.task_description if node.tbd else node.task_description
            )
            lines.append(f"{indent}- [{status_icon}] {desc}")
            for a in node.assertions:
                icon = "✓" if a.verified else ("✗" if a.verified is False else "?")
                lines.append(f"{indent}  - [{icon}] {a.fact}")
            children = sorted(
                [n for n in plan_nodes if n.parent_task_id == node.task_id],
                key=lambda n: n.epoch,
            )
            for child in children:
                _format_node(child, level + 1)

        root_nodes = sorted(
            [n for n in plan_nodes if not n.parent_task_id],
            key=lambda n: n.epoch,
        )
        for root in root_nodes:
            _format_node(root, 0)

        export_path = os.path.join(self.session_folder, "task_tree_export.md")
        try:
            with open(export_path, "w", encoding="utf-8") as fh:
                fh.write("\n".join(lines) + "\n")
            self.gui.display_error(f"Task tree exported to: {export_path}")
        except Exception as exc:
            self.gui.display_error(f"Export failed: {exc}")

    def _replay_subtask(self, task_id: str) -> None:
        """Re-run a task node from scratch on a background thread.

        Loads the TaskNodeRecord and TaskTree from disk, spawns a background thread
        that calls ``AgentixBridgeAdapter.replay_task_node_generator``, and updates
        the plan tree widget live as chunks arrive.

        Args:
            task_id: The task node to replay.
        """
        if self._is_streaming.is_set():
            self.gui.display_error("Cannot replay while a response is streaming.")
            return

        tree = self.context.load_task_tree()
        if tree is None:
            self.gui.display_error(f"No task tree found — cannot replay {task_id}.")
            return
        node = tree.nodes.get(task_id)
        if node is None:
            self.gui.display_error(f"Task node '{task_id}' not found in task tree.")
            return

        self._safe_root_after(lambda tid=task_id: self.gui.update_plan_node_status(tid, "running"))

        def _worker(_node=node, _tree=tree, _tid=task_id):
            try:
                self._is_streaming.set()
                self._safe_root_after(lambda: self.gui.set_streaming_state(True))
                for chunk in self.agentix_adapter.replay_task_node_generator(_node, self.context, _tree):
                    if chunk.type == ChunkType.TASK_NODE_END and chunk.task_id == _tid:
                        _synth = chunk.content or ""
                        _asserts = chunk.assertions or []
                        self._safe_root_after(lambda tid=_tid: self.gui.update_plan_node_status(tid, "done"))
                        self._safe_root_after(
                            lambda tid=_tid, s=_synth, a=_asserts: self.gui.update_plan_synthesis(tid, s, a)
                        )
            except Exception as exc:
                logger.exception("replay_subtask worker error")
                self._safe_root_after(lambda err=exc: self.gui.display_error(f"Replay error: {err}"))
            finally:
                self._is_streaming.clear()
                self._safe_root_after(lambda: self.gui.set_streaming_state(False))
                self._safe_root_after(self.refresh_user_gui)

        threading.Thread(target=_worker, daemon=True).start()

    def refresh_context_gui(self):
        """
        Refreshes the context GUI in the Session tab of the system status notebook.
        Uses GUIManager's rendering methods for context/history.
        """
        if threading.current_thread() is not threading.main_thread():
            return
        # Render history first (collapsed by default) in the Session tab

        history_widget = self.gui.render_history_widget(
            self.history,
            self.gui.get_history_parent(),
            self.user,
            on_attachment_toggle=self.on_history_attachment_toggle,
        )
        self.gui.update_history_panel(history_widget)

        # Render current context in the Session tab
        context_widget = self.gui.render_context_widget(
            self.context,
            self.gui.get_context_parent(),
            on_attachment_toggle=self.on_history_attachment_toggle,
            on_plan_click=self._on_plan_row_click,
        )
        self.gui.update_context_panel(context_widget)

    def refresh_working_memory_gui(self):
        """Refresh the 🏛️ Working Memory panel in the Session tab."""
        if threading.current_thread() is not threading.main_thread():
            return
        if self.working_memory is None:
            return
        try:
            wm_widget = self.gui.render_working_memory_widget(
                self.working_memory,
                self.gui.get_working_memory_parent(),
                on_toggle=self._on_wm_toggle,
                on_delete=self._on_wm_delete,
                on_promote=self._on_wm_promote,
                on_user_add=self._on_wm_user_add,
            )
            self.gui.update_working_memory_panel(wm_widget, fact_count=len(self.working_memory))
        except RuntimeError:
            pass  # working_memory section not present (feature disabled in config)

    # ------------------------------------------------------------------
    # Working Memory GUI callbacks
    # ------------------------------------------------------------------

    def _on_wm_toggle(self, compound_key: str, enabled: bool) -> None:
        if self.working_memory is not None:
            self.working_memory.set_enabled(compound_key, enabled)
            self._safe_root_after(self.refresh_working_memory_gui)

    def _on_wm_delete(self, compound_key: str) -> None:
        if self.working_memory is not None:
            self.working_memory.remove_fact(compound_key)
            self._safe_root_after(self.refresh_working_memory_gui)

    def _on_wm_promote(self, compound_key: str) -> None:
        """Handle promote-to-user-owned with conflict resolution dialog."""
        if self.working_memory is None:
            return
        from shared.models.working_memory import PromotionStatus

        result = self.working_memory.promote_to_user(compound_key)
        if result.status == PromotionStatus.OK:
            self._safe_root_after(self.refresh_working_memory_gui)
        elif result.status == PromotionStatus.CONFLICT:
            self._handle_wm_promote_conflict(compound_key, result)
        # NOT_FOUND / NOT_AGENT_OWNED — silently ignore (should not happen from GUI)

    def _handle_wm_promote_conflict(self, compound_key: str, result) -> None:
        """Show conflict resolution dialog for promote operation."""
        import tkinter.simpledialog as sd

        from shared.models.working_memory import PromotionStatus

        key = compound_key.split(":", 1)[-1] if ":" in compound_key else compound_key
        existing_preview = str(result.conflicting_value)[:60]
        choice = sd.askstring(
            "Promote Conflict",
            (
                f"A user-owned fact '{key}' already exists:\n"
                f"  Current value: {existing_preview}\n\n"
                f"Options:\n"
                f"  • Type a new key name to rename\n"
                f"  • Type 'replace' to overwrite the existing user fact\n"
                f"  • Cancel to abort"
            ),
        )
        if choice is None:
            return
        if choice.strip().lower() == "replace":
            r = self.working_memory.promote_to_user(compound_key, force=True)
        else:
            new_key = choice.strip()
            if not new_key:
                return
            r = self.working_memory.promote_to_user(compound_key, new_key=new_key)
        if r.status == PromotionStatus.OK:
            self._safe_root_after(self.refresh_working_memory_gui)

    def _on_wm_user_add(self, key: str, value_str: str) -> None:
        """Handle user submitting a new user-owned fact via the add form."""
        if self.working_memory is None:
            return
        import json as _json

        from shared.models.working_memory import FactOwner

        parsed_value = value_str
        try:
            candidate = _json.loads(value_str)
            if isinstance(candidate, (dict, list, int, float)):
                parsed_value = candidate
        except (ValueError, _json.JSONDecodeError):
            pass
        self.working_memory.add_fact(FactOwner.USER, key, parsed_value)
        self._safe_root_after(self.refresh_working_memory_gui)

    # ------------------------------------------------------------------
    # Settings GUI
    # ------------------------------------------------------------------

    def refresh_settings_gui(self) -> None:
        """Build (or rebuild) the ⚙️ Settings tab content."""
        if threading.current_thread() is not threading.main_thread():
            return
        try:
            models: list[dict] = []
            try:
                models = self.agentix_adapter.get_models()
            except Exception:
                pass
            self.gui.render_settings_tab(
                config=self.config,
                on_change=self._on_setting_change,
                models=models,
                system_prompts_dir="system_prompts",
            )
        except RuntimeError:
            pass  # settings_tab not yet created

    def _on_setting_change(self, key_path: list[str], value) -> None:
        """Handle a setting change from the ⚙️ Settings tab.

        Config-only changes (key_path[0] == '__config_only__') are written to
        disk but NOT hot-applied at runtime.  All other changes are both
        written and hot-applied where possible.

        Special rule: 'ollama_model' is the startup *default* only — it must
        never overwrite the user's live model selection (managed by the toolbar
        ModelSelector).
        """
        config_only = False
        if key_path and key_path[0] == "__config_only__":
            config_only = True
            key_path = key_path[1:]

        if not key_path:
            return

        # Navigate / create nested dict structure and write the new value.
        node = self.config
        for part in key_path[:-1]:
            node = node.setdefault(part, {})
        node[key_path[-1]] = value

        # Persist to disk.
        try:
            save_config(self.config)
        except Exception as e:
            logger.warning("Settings: failed to save config: %s", e)

        if config_only:
            return

        # Hot-apply where possible.
        leaf = key_path[-1]
        section = key_path[0] if len(key_path) > 1 else None
        sub = key_path[1] if len(key_path) > 2 else None

        # NEVER change the runtime active model here — that is the toolbar's job.
        if key_path == ["agentx", "ollama_model"]:
            return

        try:
            if section == "agentix":
                agentix_cfg = self.agentix_adapter.agentix_config
                if leaf == "debug":
                    agentix_cfg.debug = bool(value)
                elif leaf == "agentix_bench_classification_model":
                    agentix_cfg.classification_model = value
                elif leaf == "available_tools":
                    self.agentix_adapter.set_enabled_tools(value)
                elif leaf == "default_system_prompts":
                    agentix_cfg.system = value
                # classify_prompts / classification_display / host — all config-dict
                # reads, no explicit adapter call needed.
            elif section == "agentx":
                if leaf == "markdown_render_enabled":
                    self.gui.config.markdown_render_enabled = bool(value)
        except AttributeError:
            pass  # Agentix not available

    def attach_file(self, file_path: str):
        """
        Attach a file to the session context.
        :param file_path: The path to the file to be attached.
        """
        self.message.attach(file_path)
        self.refresh_user_gui()

    def refresh_user_gui(self):
        """
        Refreshes the user attachment bar display.
        Now delegated to GUIManager via update_attachment_bar().
        """
        if threading.current_thread() is not threading.main_thread():
            return
        # Convert current message attachments to AttachmentInfo DTOs
        current_attachments = [
            AttachmentInfo.from_attachment(att, is_from_history=False) for att in self.message.attachments
        ]

        # Convert enabled history attachments to AttachmentInfo DTOs
        history_attachments = [
            AttachmentInfo.from_attachment(att, is_from_history=True) for att in self.enabled_history_attachments
        ]

        # Update via GUIManager
        self.gui.update_attachment_bar(current_attachments, history_attachments)

    def _on_add_folder_to_memory(self, key: str, value: str) -> None:
        """Add a folder path to working memory with the folder name as the key."""
        if self.working_memory is None:
            return
        from shared.models.working_memory import FactOwner

        self.working_memory.add_fact(FactOwner.USER, key, value)
        self._safe_root_after(self.refresh_working_memory_gui)

    def refresh_files_gui(self):
        """
        Refreshes the file explorer GUI in the Files tab.
        Now delegated to GUIManager via update_files_panel().
        """
        if threading.current_thread() is not threading.main_thread():
            return
        # Render file explorer widget
        files_widget = self.file_explorer.to_gui(
            self.gui.get_files_parent(),
            on_attach=self.attach_file,
            on_edit=None,  # You can wire up edit logic here later
            on_add_folder_to_memory=self._on_add_folder_to_memory,
            theme_mode=self.gui.config.theme_mode,
            bg=self.gui.config.status_bg,
            panel_bg=self.gui.config.input_bg,
            fg=self.gui.config.ui_fg,
            muted_fg=self.gui.config.muted_fg,
            tree_bg=self.gui.config.output_bg,
            tree_fg=self.gui.config.ui_fg,
        )
        # Update via GUIManager
        self.gui.update_files_panel(files_widget)

    def add_message_to_context(self, message: Message):
        """
        Adds a message to the session context and refreshes the context GUI.
        """
        time_added = datetime.now()
        self.context.add_message(message, ts=time_added)
        self._safe_root_after(self.refresh_context_gui)

    def layout(self):
        """
        Sets up the layout for the tkinter root window.
        Now delegated to GUIManager.
        """
        # Create all GUI widgets via GUIManager
        self.gui.create_layout()

        # Initialize dynamic content in panels
        self.refresh_context_gui()
        self.refresh_files_gui()
        self.refresh_working_memory_gui()
        self.refresh_settings_gui()

        # Setup model selector and tool panel if Agentix is available
        # (Must be after layout is created so the widgets exist)
        self._setup_agentix_ui()
        self._run_bootstrap_prompt_if_present()

    def stream_ollama_response_worker(self):
        """
        Worker function that streams the response from the Ollama server and updates the output via GUIManager.
        This runs in a separate thread to keep the GUI responsive.

        Routes through Agentix middleware for classification and tool support.
        """
        # Agentix is always integrated.
        self._stream_via_agentix()

    def _safe_root_after(self, callback) -> None:
        try:
            if threading.current_thread() is threading.main_thread():
                callback()
            else:
                self.root.after(0, callback)
        except RuntimeError:
            pass

    def _write_log(self, text: str) -> None:
        """Append text to the session transcript log (thread-safe, best-effort)."""
        try:
            self._session_log.write(text)
        except Exception:
            pass

    def retrigger_synthesis(self, task_id: str, hint: str = "") -> None:
        """Re-run synthesis for a completed task node in a background thread.

        Invoked by the ResynthesisDialog confirm handler on the Tkinter main
        thread.  Loads the TaskNodeRecord and TaskTree from disk, optionally
        injects a WM hint, then streams the new synthesis on a daemon thread
        and updates the plan tree widget live.

        Args:
            task_id: The task node to re-synthesise.
            hint:    Optional free-text guidance for the LLM.
        """
        if self._is_streaming.is_set():
            self.gui.display_error("Cannot re-synthesise while a response is streaming.")
            return

        tree = self.context.load_task_tree()
        if tree is None:
            self.gui.display_error(f"No task tree found — cannot re-synthesise {task_id}.")
            return
        node = tree.nodes.get(task_id)
        if node is None:
            self.gui.display_error(f"Task node '{task_id}' not found in task tree.")
            return

        if hint.strip() and self.working_memory is not None:
            from shared.models.working_memory import FactOwner

            self.working_memory.add_fact(FactOwner.AGENT, f"resynth_hint_{task_id}", hint)
            node.wm_hints_added = True

        self.gui.mark_plan_node_invalidated(task_id)

        def _worker(_node=node, _tree=tree, _tid=task_id, _hint=hint):
            try:
                self._is_streaming.set()
                self._safe_root_after(lambda: self.gui.set_streaming_state(True))
                for chunk in self.agentix_adapter.retrigger_synthesis_generator(_node, self.context, _tree, _hint):
                    if chunk.type == ChunkType.TASK_NODE_END and chunk.task_id == _tid:
                        _synth = chunk.content or ""
                        _asserts = chunk.assertions or []
                        self._safe_root_after(lambda tid=_tid: self.gui.update_plan_node_status(tid, "done"))
                        self._safe_root_after(
                            lambda tid=_tid, s=_synth, a=_asserts: self.gui.update_plan_synthesis(tid, s, a)
                        )
            except Exception as exc:
                logger.exception("retrigger_synthesis worker error")
                self._safe_root_after(lambda err=exc: self.gui.display_error(f"Re-synthesis error: {err}"))
            finally:
                self._is_streaming.clear()
                self._safe_root_after(lambda: self.gui.set_streaming_state(False))
                self._safe_root_after(self.refresh_user_gui)

        threading.Thread(target=_worker, daemon=True).start()

    def _add_wm_hint_for_task(self, task_id: str, key: str, value: str) -> None:
        """Store a working-memory fact and mark the task node as invalidated.

        Called from the ResynthesisDialog "Add WM hint" button on the Tkinter
        main thread.  Does not trigger re-synthesis — the user must still click
        "Re-synthesise" afterwards.

        Args:
            task_id: The task node to invalidate.
            key:     WM fact key.
            value:   WM fact value.
        """
        if self.working_memory is not None:
            self.working_memory.add_fact(FactOwner.AGENT, key, value)

        tree = self.context.load_task_tree()
        if tree is not None:
            node = tree.nodes.get(task_id)
            if node is not None:
                node.wm_hints_added = True
                try:
                    self.context.save_task_node(node)
                    self.context.save_task_tree(tree)
                except Exception:
                    pass

        self.gui.mark_plan_node_invalidated(task_id)

    def _stream_via_agentix(self):
        """Stream response through Agentix middleware."""
        config = self.config

        self._is_streaming.set()
        self._safe_root_after(lambda: self.gui.set_streaming_state(True))
        self._safe_root_after(self.refresh_user_gui)

        # Use captured prompt from submit; fall back to cached input for tests.
        prompt = self._pending_prompt or ""
        self._pending_prompt = None

        if not prompt and not self.message.attachments:
            prompt = self.gui._cached_user_input or ""
            if not prompt:
                self._safe_root_after(lambda: self.gui.display_error("No input provided."))
                self._is_streaming.clear()
                self._safe_root_after(lambda: self.gui.set_streaming_state(False))
                return

        # Display the user prompt and attachments
        attachment_filenames = [os.path.basename(att.file_path) for att in self.message.attachments]
        self._safe_root_after(lambda: self.gui.display_user_message(prompt, attachment_filenames, datetime.now()))
        self._write_log(f"\n👤 User: {prompt}\n")
        self._output_logger.log("user", prompt)

        try:
            # Prepare message
            self.message.content = prompt
            self.message.enabled = True
            self._safe_root_after(self.refresh_context_gui)

            # Add enabled history attachments
            for att in self.enabled_history_attachments:
                if att not in self.message.attachments:
                    self.message.attachments.append(att)

            # Build shared Context from history and current context
            shared_context = self._build_stream_shared_context()

            # Add current message
            self.add_message_to_context(self.message)

            # Reset message and history attachments
            self.message = Message(role="user", content="")
            self.enabled_history_attachments = []

            # Classify prompt if enabled
            classification = None
            if config.get("agentix", {}).get("classify_prompts", True):
                classification = self.agentix_adapter.classify_prompt_sync(prompt, shared_context, self.working_memory)
                self._log_classification(classification, prompt)

            # Reset per-turn display state
            self._assistant_header_shown = False
            self._thinking_header_shown = False

            # Create response handler
            thinking_parts: list[str] = []
            content_parts: list[str] = []

            handler = ResponseHandler(
                on_content=lambda text: self._handle_stream_content(text),
                on_thinking=lambda text: self._display_thinking(text),
                on_tool_call=lambda name, args, round_i=None: self._display_tool_call(name, args, round_i),
                on_tool_result=lambda tool_name, output, round_i=None, tool_id=None: self._display_tool_result(
                    tool_name, output, round_i, tool_id=tool_id
                ),
                on_error=lambda msg, code: self._safe_root_after(lambda: self.gui.display_error(f"{code}: {msg}")),
                on_classification=self._make_classification_callback(config),
            )

            # Stream through Agentix
            for chunk in self.agentix_adapter.process_prompt_generator(prompt, shared_context, classification):
                if not self._is_streaming.is_set():
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
                    _on_export = lambda pid=_pid: self._export_task_tree(pid)
                    self._safe_root_after(
                        lambda pid=_pid, pn=_pname, exp=_on_export: (
                            self.gui.add_plan_tab(pid, pn, on_export=exp),
                            self.gui.focus_plan_tab(pid),
                        )
                    )
                    # Store PLAN message so it shows in the context panel
                    _plan_msg = Message(
                        role=MessageRole.PLAN,
                        content=_pname,
                        plan_id=_pid,
                        plan_name=_pname,
                    )
                    self.context.add_message(_plan_msg)
                elif chunk.type == ChunkType.TASK_NODE_START and chunk.task_id:
                    _tid = chunk.task_id
                    _pid = chunk.plan_id or ""
                    _desc = chunk.content or chunk.task_id
                    _par = chunk.parent_task_id
                    _depth = chunk.task_depth or 0
                    _tbd = bool(chunk.tbd)
                    _on_replay = lambda tid=_tid: self._replay_subtask(tid)
                    if _par:
                        self._safe_root_after(
                            lambda tid=_tid, par=_par, desc=_desc, d=_depth, rep=_on_replay: self.gui.add_plan_subtask_node(
                                tid, par, desc, d, on_replay=rep
                            )
                        )
                    else:
                        self._safe_root_after(
                            lambda pid=_pid, tid=_tid, desc=_desc, tb=_tbd, rep=_on_replay: self.gui.add_plan_step_node(
                                pid, tid, desc, tb, on_replay=rep
                            )
                        )
                elif chunk.type == ChunkType.TASK_NODE_TBD and chunk.task_id:
                    _tid = chunk.task_id
                    _desc = chunk.content or ""
                    self._safe_root_after(lambda tid=_tid, desc=_desc: self.gui.resolve_plan_tbd_node(tid, desc))
                elif chunk.type == ChunkType.TASK_NODE_END and chunk.task_id:
                    _tid = chunk.task_id
                    _synth = chunk.content or ""
                    _asserts = chunk.assertions or []
                    _node_pid = chunk.plan_id or ""
                    _node_depth = chunk.task_depth or 0
                    self._safe_root_after(lambda tid=_tid: self.gui.update_plan_node_status(tid, "done"))

                    # Build per-task callbacks for the Re-synthesise dialog.
                    def _make_callbacks(tid=_tid):
                        on_resynth = lambda hint: self.retrigger_synthesis(tid, hint)
                        on_add_wm = lambda key, val: self._add_wm_hint_for_task(tid, key, val)
                        return on_resynth, on_add_wm

                    _on_resynth, _on_add_wm = _make_callbacks()
                    self._safe_root_after(
                        lambda tid=_tid, s=_synth, a=_asserts, cb=_on_resynth, wm=_on_add_wm: self.gui.add_plan_synthesis(
                            tid, s, a, on_resynth=cb, on_add_wm_hint=wm
                        )
                    )
                    # Store TASK_NODE message so it shows in the context panel
                    _node_msg = Message(
                        role=MessageRole.TASK_NODE,
                        content=_synth,
                        plan_id=_node_pid,
                        task_id=_tid,
                        task_depth=_node_depth,
                    )
                    self.context.add_message(_node_msg)
                elif chunk.type == ChunkType.TOOL_CALL and chunk.task_id:
                    _tid = chunk.task_id
                    _tname = chunk.tool_name or ""
                    _tinput = chunk.tool_input or {}
                    self._safe_root_after(
                        lambda tid=_tid, tn=_tname, ti=_tinput: self.gui.add_plan_tool_call(tid, tn, ti)
                    )

                self._safe_root_after(self.refresh_user_gui)

            # Complete the response
            self._safe_root_after(self.gui.display_spacing)
            joined_thinking = "".join(thinking_parts)
            joined_content = "".join(content_parts)
            self._persist_stream_messages(joined_thinking, joined_content)
            if joined_thinking:
                self._output_logger.log("thinking", joined_thinking)
            if joined_content:
                self._output_logger.log("agent", joined_content)
            self._safe_root_after(self.refresh_user_gui)

        except Exception as e:
            logger.exception("Request error during streaming")
            err_line = f"\n⚠️  ERROR: {e}\n"
            self._safe_root_after(lambda err=e: self.gui.display_error(f"Error: {err}"))
            self._write_log(err_line)
            self._output_logger.log("error", str(e))
        finally:
            self._is_streaming.clear()
            self._safe_root_after(lambda: self.gui.set_streaming_state(False))

    def _make_classification_callback(self, config: dict):
        """Build the on_classification callback respecting field-level display config."""
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
            self._safe_root_after(lambda m=filtered: self.gui.display_classification(m))
            # Mirror classification to session log
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
                self._write_log("\n".join(lines) + "\n")
            self._output_logger.log("classification", json.dumps(filtered, ensure_ascii=False))

        return _callback

    def _display_thinking(self, text: str):
        """Helper to display thinking text with header on first call."""
        if not getattr(self, "_thinking_header_shown", False):
            header = f"\n{GUIManager.MESSAGE_ROLES['thinking']} ({self.active_model})\t(The agent is thinking...)\n"
            self._safe_root_after(lambda: self.gui.display_agent_thinking(header))
            self._write_log(header)
            self._thinking_header_shown = True
        self._safe_root_after(lambda: self.gui.display_agent_thinking(text))
        self._write_log(text)

    def _display_tool_call(self, tool_name: str, tool_input: dict, round_index: int | None = None) -> None:
        """
        Display a tool call in the GUI and store it in context.

        The bridge handles tool execution; this method is display-only.
        Storing the TOOL_CALL message ensures the tool interaction is visible
        in the session history and can be re-serialized in future turns.
        """
        round_label = f" [round {round_index + 1}]" if round_index is not None else ""
        line = f"\n[🔧 Calling tool{round_label}: {tool_name}]\n"
        self._safe_root_after(lambda: self.gui.display_agent_response(line))
        self._write_log(line)
        try:
            input_text = f"{tool_name}: {json.dumps(tool_input, ensure_ascii=False)}"
        except Exception:
            input_text = f"{tool_name}: {tool_input}"
        self._output_logger.log("tool_call", input_text)
        self.context.add_tool_call_message(tool_name, tool_input)

    def _display_tool_result(
        self, tool_name: str, output, round_index: int | None = None, tool_id: str | None = None
    ) -> None:
        """
        Display a tool result in the GUI and store it in context.

        The bridge has already executed the tool and produced ``output``.
        This method records the result in context so it persists across
        sessions and is included in future LLM history.
        """
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
        self._safe_root_after(lambda: self.gui.display_agent_response(result_line))
        self._write_log(result_line)
        self._output_logger.log("tool_result", f"{tool_name}: {display_text}")
        self.context.add_tool_result_message(
            tool_name=tool_name,
            tool_output=output,
            tool_id=tool_id,
        )
        # Refresh Working Memory panel in case the tool mutated it
        self._safe_root_after(self.refresh_working_memory_gui)

    def _display_assistant_header(self) -> None:
        """Display the assistant header once per response stream."""
        if not getattr(self, "_assistant_header_shown", False):
            self._assistant_header_shown = True
            header = f"\n\n{GUIManager.MESSAGE_ROLES['assistant']} ({self.active_model})\t"
            self._safe_root_after(lambda: self.gui.display_agent_response(header))
            self._write_log(header)

    def _handle_stream_content(self, text: str) -> None:
        """Ensure header is shown before streaming content chunks."""
        self._display_assistant_header()
        self._safe_root_after(lambda: self.gui.display_agent_response(text))
        self._write_log(text)

    def _build_shared_context_from_context(self) -> Context:
        """Build shared context from currently enabled messages."""
        shared_context = Context()
        for entry in self.context.messages:
            msg = entry.message if hasattr(entry, "message") else entry
            if getattr(msg, "enabled", False):
                shared_context.add_message(msg, ts=msg.timestamp)
        return shared_context

    def _persist_stream_messages(
        self,
        thinking_text: str,
        content_text: str,
        refresh_gui: bool = True,
    ) -> None:
        """Persist streamed thinking and assistant content to context."""
        if thinking_text:
            thinking_message = Message(role=MessageRole.THINKING, content=thinking_text)
            thinking_message.enabled = False
            if refresh_gui:
                self.add_message_to_context(thinking_message)
            else:
                self.context.add_message(thinking_message, ts=datetime.now())

        if content_text:
            assistant_message = Message(role=MessageRole.ASSISTANT, content=content_text)
            assistant_message.enabled = True
            if refresh_gui:
                self.add_message_to_context(assistant_message)
            else:
                self.context.add_message(assistant_message, ts=datetime.now())

    def _stream_direct_ollama(self):
        """Deprecated compatibility shim that delegates to canonical Agentix stream path."""
        print("[AgentX] _stream_direct_ollama is deprecated; delegating to _stream_via_agentix")
        if self._pending_prompt is None:
            self._pending_prompt = self.gui.get_user_input()
        self._stream_via_agentix()

    def perform_service_handshake(self):
        """
        Performs service startup and handshake with required services.

        Steps:
        1. Ensure Ollama service is running
        2. Ensure Agentix service is running
        3. Verify Ollama model is loaded and responsive
        4. Display available models and services
        """
        config = self.config
        ollama_host = config["agentx"]["ollama_host"]
        ollama_model = self.active_model
        timeout_seconds = config["agentx"].get("ollama_initial_load_timeout_seconds", 120)

        # Determine which services to ensure are running
        services_to_start = ["ollama", "agentix"]

        # Start services
        print(f"Ensuring services are running: {', '.join(services_to_start)}")
        all_services_started = self.service_manager.ensure_services(services_to_start, timeout=30)

        if not all_services_started:
            raise RuntimeError(
                "Agentix service did not start successfully. " "AgentX requires Agentix for model interactions."
            )

        # Perform Ollama model handshake and list available models
        url = f"http://{ollama_host}/api/chat"
        headers = {"Content-Type": "application/json"}
        payload = {
            "model": ollama_model,
            "prompt": "",
        }  # Empty prompt to trigger model load

        print(f"Connecting to Ollama at {url}")

        try:
            with httpx.Client(timeout=timeout_seconds) as client:
                response = client.post(url, json=payload, headers=headers)
                response.raise_for_status()
                print("✓ Service handshake and model invocation successful.")

                # List available models
                try:
                    models_response = client.get(f"http://{ollama_host}/api/tags")
                    if models_response.status_code == 200:
                        import json

                        models_data = models_response.json()
                        models = models_data.get("models", [])
                        if models:
                            print(f"\n✓ Available Ollama models ({len(models)}):")
                            for model in models:
                                model_name = model.get("name", "unknown")
                                # Show simplified name without tag
                                display_name = model_name.split(":")[0] if ":" in model_name else model_name
                                print(f"  • {display_name}")
                        else:
                            print("\n⚠ No models available in Ollama")
                except Exception as e:
                    print(f"\n⚠ Could not fetch model list: {e}")

                # Show service status
                print()
                print("Service Status:")
                print(f"  ✓ Ollama: Ready")
                if all_services_started:
                    print("  ✓ Agentix: Ready (code analysis available)")
                else:
                    print("  ✗ Agentix: Failed (code analysis unavailable)")
                print()

        except httpx.RequestError as e:
            raise RuntimeError(f"Failed to connect to Ollama at {url}") from e

    def stream_ollama_response(self) -> None:
        """Start streaming response in a background thread."""
        if self._streaming_thread and self._streaming_thread.is_alive():
            print("Streaming already in progress")
            return
        # Capture and clear input before starting the worker.
        self._pending_prompt = self.gui.get_user_input()
        if not self._pending_prompt and not self.message.attachments:
            self.gui.display_error("No input provided.")
            self._pending_prompt = None
            return
        self._streaming_thread = threading.Thread(target=self.stream_ollama_response_worker, daemon=True)
        self._streaming_thread.start()

    def interrupt_streaming(self):
        """
        Interrupts the ongoing streaming process.
        """
        print("Interrupting streaming...")
        self._is_streaming.clear()
