"""
AgentXSession — thin coordinator that wires together the GUI, streaming,
tool execution, and session state.

The heavy lifting is delegated to three focused classes:
- ``SessionState``       — mutable session data (model, history, current message)
- ``ToolDispatcher``     — routes tool calls to client/server executors
- ``StreamingController`` — owns all LLM streaming and display logic
"""

import json
import logging
import os
import threading
import tkinter as tk
from datetime import UTC, datetime
from pathlib import Path
from tkinter import messagebox
from typing import Any, Iterator, Optional

import httpx

from agentix.prompt_classification_response import PromptClassificationResponse
from shared.models.context import Context
from shared.models.message import Message, MessageRole
from shared.models.response import ChunkType, ResponseChunk
from shared.models.working_memory import FactOwner, WorkingMemory
from shared.providers import ILLMServiceProvider, OllamaServiceProvider
from shared.providers.constants import FALLBACK_CONTEXT_WINDOW

from .attachment_info import AttachmentInfo
from .config import apply_config_defaults, normalize_context_history_session_sort, save_config, validate_config
from .event_broker import EventBroker, EventType
from .file_explorer import FileExplorer
from .gui.gui_config import GUIConfig
from .gui.gui_manager import GUIManager  # concrete class — used only for construction in __init__
from .history import History
from .igui_manager import IGUIManager, NullGUIManager
from .integration import (
    AgentixBridgeAdapter,
    ClientToolExecutor,
    ServerToolExecutor,
    TuiBridge,
    agentix_bridge_adapter,
)
from .integration.tui_event_subscriber import TUIEventSubscriber
from .integration.vim_bridge import VimBridge
from .model_metadata_store import ModelMetadataStore
from .output_logger import OutputLogger
from .service_manager import ServiceManager
from .session_state import SessionState
from .streaming_controller import StreamingController
from .tool_dispatcher import ToolDispatcher

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

        apply_config_defaults(config)
        validate_config(config)

        self._enable_gui_chat = bool(config.get("agentx", {}).get("enable_gui_chat", True))

        if root is not None:
            self.root = root
        else:
            if self._enable_gui_chat:
                self.root = tk.Tk()
            else:
                # Headless mode keeps a Tcl/Tk event loop available without creating windows.
                self.root = tk.Tk(useTk=False)

        if root is None and self._enable_gui_chat:
            self.root.withdraw()
        self.config = config

        session_started_at = datetime.now(UTC)
        session_id = f"session_{session_started_at.strftime('%Y-%m-%d_%H-%M-%S')}"
        user = username or os.getenv("USER") or os.getenv("USERNAME") or "User"
        base_dir = session_dir or os.getcwd()
        user_history_folder = os.path.join(base_dir, "sessions", user)
        session_folder = os.path.join(user_history_folder, session_id)
        os.makedirs(session_folder, exist_ok=True)
        context_folder = os.path.join(session_folder, "context")
        os.makedirs(context_folder, exist_ok=True)
        session_log_path = os.path.join(session_folder, "session.log")

        # --- SessionState: all mutable session data ---
        self._state = SessionState(
            config=config,
            session_id=session_id,
            session_folder=session_folder,
            context_folder=context_folder,
            session_log_path=session_log_path,
            user=user,
            user_history_folder=user_history_folder,
        )

        # Expose session-identity attributes directly for backward compatibility
        # (tests and other modules read these from the session instance)
        self.session_id = session_id
        self.session_folder = session_folder
        self.user = user
        self.start_time = self._state.start_time
        self.user_history_folder = user_history_folder
        self.context_folder = context_folder

        # Startup model-capacity store used by context-meter redraws.
        ollama_host = self.config.get("agentx", {}).get("ollama_host", "localhost:11434")
        self._llm_provider: ILLMServiceProvider = OllamaServiceProvider(host=ollama_host)
        self._model_store = ModelMetadataStore(
            provider=self._llm_provider,
            cache_path=Path(base_dir) / "sessions" / "_model_cache.json",
        )
        # Populate asynchronously so the Tk main-thread is not blocked at startup.
        # get_context_length() returns FALLBACK_CONTEXT_WINDOW until this completes.
        threading.Thread(target=self._model_store.populate, daemon=True).start()

        # Context lives on session directly (tests mock session.context)
        self.context = Context(path=context_folder, session_id=session_id)
        self.file_explorer = FileExplorer(start_path=os.getcwd())

        # Initialize event broker for pub-sub streaming
        self.event_broker = EventBroker()

        # VimBridge: connects to the running neovim instance via its Unix socket
        self.vim_bridge = VimBridge(config=config)

        # Per-turn streaming state (stays on session — tests set these directly)
        self._is_streaming = threading.Event()
        self._streaming_thread: Optional[threading.Thread] = None
        self._pending_prompt: Optional[str] = None
        self._last_synthesis_thread: Optional[threading.Thread] = None
        self._last_replay_thread: Optional[threading.Thread] = None

        # Session transcript log
        self._session_log_path = session_log_path
        self._session_log = open(session_log_path, "a", encoding="utf-8", buffering=1)
        self._output_logger = OutputLogger(session_folder)

        # Optional TUI output mirror bridge.
        self.tui_bridge: Optional[TuiBridge] = None
        self.tui_event_subscriber: Optional[TUIEventSubscriber] = None
        self._last_tui_context_signature: tuple[int, tuple[tuple[str, int], ...]] | None = None
        tui_cfg = config.get("tui", {})
        if bool(tui_cfg.get("enable", False)):
            tmux_session = os.getenv("AGENTX_TMUX_SESSION", "agentx")
            output_fifo = str(
                tui_cfg.get("output_fifo")
                or os.getenv("AGENTX_TUI_OUTPUT_FIFO")
                or f"/tmp/agentx_{tmux_session}.tui_output.fifo"
            )
            input_fifo = str(
                tui_cfg.get("input_fifo")
                or os.getenv("AGENTX_TUI_INPUT_FIFO")
                or f"/tmp/agentx_{tmux_session}.tui_input.fifo"
            )
            try:
                write_timeout_sec = float(tui_cfg.get("write_timeout_sec", 0.1))
            except (TypeError, ValueError):
                write_timeout_sec = 0.1

            self.tui_bridge = TuiBridge(
                output_fifo=output_fifo,
                input_fifo=input_fifo,
                on_submit=self._on_tui_submit,
                on_quit=self._on_tui_quit,
                enabled=True,
                write_timeout_sec=write_timeout_sec,
            )
            self.tui_bridge.start()

            # Create TUI event subscriber and wire to event broker
            self.tui_event_subscriber = TUIEventSubscriber(tui_bridge=self.tui_bridge)
            self.tui_event_subscriber.start()

            # Subscribe to all streaming events
            from .event_broker import EventType

            for event_type in EventType:
                self.event_broker.subscribe(event_type, self.tui_event_subscriber.handle_event, queue_size=1000)

        # Initialize service manager for external services
        self.service_manager = ServiceManager(config)

        # Initialize GUIManager
        gui_config = GUIConfig.from_dict(config)
        if self._enable_gui_chat:
            self.gui = GUIManager(
                root=self.root,
                config=gui_config,
                on_submit=self._handle_submit,
                on_interrupt=self._handle_interrupt,
                on_attachment_toggle=self._handle_attachment_toggle,
            )
        else:
            self.gui = NullGUIManager(
                root=self.root,
                config=gui_config,
                on_submit=self._handle_submit,
                on_interrupt=self._handle_interrupt,
                on_attachment_toggle=self._handle_attachment_toggle,
            )
        if hasattr(self.gui, "_on_terminal_kill_pane"):
            setattr(self.gui, "_on_terminal_kill_pane", self._handle_terminal_kill_pane)
        self.gui.set_terminal_mode_toggle_callback(self._handle_terminal_mode_toggle)

        # Set window title with session info
        self.gui.set_window_title(f"{user} - AgentX Session - {self.start_time}")

        # Initialize Agentix bridge (always integrated)
        self.agentix_adapter = create_adapter(config)
        self.agentix_adapter.agentix_config.session = session_id

        # Initialize tool executors and dispatcher
        self.client_tool_executor = ClientToolExecutor(base_path=os.getcwd())
        self.server_tool_executor = ServerToolExecutor(agentix_bridge=self.agentix_adapter.bridge)
        self._tool_dispatcher = ToolDispatcher(self.client_tool_executor, self.server_tool_executor)

        # Initialize Working Memory — loaded from session folder (or empty on new session)
        wm_config = config.get("agentx", {}).get("working_memory", {})
        if wm_config.get("enabled", True):
            self.working_memory: Optional[WorkingMemory] = WorkingMemory.load(session_folder)
            self.working_memory.set_path(session_folder)
            cwd = os.getcwd()
            self.working_memory.add_fact(FactOwner.USER, "UserName", user)
            self.working_memory.add_fact(FactOwner.USER, "cwd", cwd)
            project_name = SessionState.detect_git_project_name(cwd)
            if project_name:
                self.working_memory.add_fact(FactOwner.USER, "project", project_name)
            instructions_text = SessionState.load_agentx_instructions(cwd)
            if instructions_text is not None:
                self.working_memory.add_fact(FactOwner.USER, "agentx-instructions", instructions_text)
            self.agentix_adapter.register_working_memory_tools(self.working_memory)
        else:
            self.working_memory = None

        # Initialize active model (kept on session for backward compat with tests)
        self._active_model = config["agentx"]["ollama_model"]
        self._sync_agentix_model_capacity()

        # Per-turn message and history (reset after each sent message)
        self._history: Optional["History"] = None
        self.message: Message = Message(role="user", content="")
        self.enabled_history_attachments: list = []

        # Initialize per-turn display flags
        self._assistant_header_shown: bool = False
        self._thinking_header_shown: bool = False

        # --- StreamingController: owns the streaming loop and display logic ---
        self._streaming_controller = StreamingController(self)
        self._active_terminal_panes: set[str] = set()
        self._configure_terminal_approval_callback()
        self._update_terminal_status_strip()

    def _get_terminal_exec_mode(self) -> str:
        """Return configured terminal execution mode for status display."""
        try:
            from .integration.terminal_bridge import get_terminal_exec_mode

            return get_terminal_exec_mode(default="supervised")
        except Exception:
            return "supervised"

    def _configure_terminal_approval_callback(self) -> None:
        """Attach session approval callback to the configured terminal bridge."""
        try:
            from .integration.terminal_bridge import set_terminal_approval_callback

            set_terminal_approval_callback(self._request_terminal_approval)
        except Exception:
            pass

    def _update_terminal_status_strip(self) -> None:
        """Refresh input-panel terminal status strip. [PD-15-AF-003]"""
        self._safe_root_after(
            lambda: self.gui.set_terminal_status(
                active_panes=len(self._active_terminal_panes),
                exec_mode=self._get_terminal_exec_mode(),
            )
        )

    def _handle_terminal_kill_pane(self, pane_id: str) -> None:
        """Handle kill-pane action from tool-result UI. [PD-15-AF-004]

        Args:
            pane_id: Target tmux pane id.
        """
        try:
            from .integration.terminal_bridge import terminal_kill_pane

            result = terminal_kill_pane(pane_id)
            self._active_terminal_panes.discard(pane_id)
            self._update_terminal_status_strip()
            self._safe_root_after(lambda: self.gui.display_agent_response(f"\n[🧹 {result}]\n"))
        except Exception as exc:
            self._safe_root_after(lambda err=exc: self.gui.display_error(f"Terminal kill failed: {err}"))

    def _handle_terminal_mode_toggle(self) -> None:
        """Toggle terminal exec mode with confirmation for autonomous. [PD-15-AF-005]"""
        current = self._get_terminal_exec_mode().strip().lower()
        target = "autonomous" if current != "autonomous" else "supervised"

        if target == "autonomous":
            confirmed = messagebox.askyesno(
                "Enable Autonomous Mode",
                "Autonomous mode will execute state-changing commands without approval. Continue?",
                parent=self.root,
            )
            if not confirmed:
                return

        self._apply_terminal_exec_mode(target)

    def _apply_terminal_exec_mode(self, mode: str) -> None:
        """Apply terminal exec mode in runtime + persisted config."""
        mode = mode.strip().lower()
        if mode not in {"supervised", "autonomous"}:
            return

        try:
            from .integration.terminal_bridge import set_terminal_exec_mode

            set_terminal_exec_mode(mode)
        except Exception:
            pass

        terminal_cfg = self.config.setdefault("terminal", {})
        terminal_cfg["exec_mode"] = mode
        try:
            save_config(self.config)
        except Exception as e:
            logger.warning("Settings: failed to save terminal exec mode: %s", e)

        self._update_terminal_status_strip()

    def _show_terminal_approval_dialog(self, command: str, context: str) -> tuple[bool, str | None]:
        """Show supervised approval dialog on Tk thread. [PD-15-AF-006]"""
        dialog = tk.Toplevel(self.root)
        dialog.title("Terminal Command Approval")
        dialog.transient(self.root)
        dialog.grab_set()
        dialog.configure(bg=self.gui.config.status_bg)

        result: dict[str, object] = {"approved": False, "command": command}
        edit_enabled = tk.BooleanVar(value=False)

        tk.Label(
            dialog,
            text="Agent wants to run:",
            anchor="w",
            bg=self.gui.config.status_bg,
            fg=self.gui.config.ui_fg,
            font=("Terminal", 10, "bold"),
        ).pack(fill=tk.X, padx=12, pady=(10, 4))

        cmd_text = tk.Text(dialog, height=4, wrap=tk.WORD, font=("Terminal", 9))
        cmd_text.insert("1.0", command)
        cmd_text.config(state=tk.DISABLED)
        cmd_text.pack(fill=tk.BOTH, expand=True, padx=12, pady=(0, 8))

        if context:
            tk.Label(
                dialog,
                text=f"Context: {context}",
                anchor="w",
                justify=tk.LEFT,
                bg=self.gui.config.status_bg,
                fg=self.gui.config.muted_fg,
                font=("Terminal", 9),
            ).pack(fill=tk.X, padx=12, pady=(0, 8))

        btn_row = tk.Frame(dialog, bg=self.gui.config.status_bg)
        btn_row.pack(fill=tk.X, padx=12, pady=(0, 10))

        def _approve() -> None:
            result["approved"] = True
            result["command"] = cmd_text.get("1.0", tk.END).strip() if edit_enabled.get() else command
            dialog.destroy()

        def _edit_or_approve() -> None:
            if not edit_enabled.get():
                edit_enabled.set(True)
                cmd_text.config(state=tk.NORMAL)
                cmd_text.focus_set()
                edit_btn.config(text="Approve Edit")
                return
            _approve()

        def _reject() -> None:
            result["approved"] = False
            result["command"] = command
            dialog.destroy()

        tk.Button(btn_row, text="Approve", command=_approve).pack(side=tk.LEFT, padx=(0, 6))
        edit_btn = tk.Button(btn_row, text="Edit & Approve", command=_edit_or_approve)
        edit_btn.pack(side=tk.LEFT, padx=(0, 6))
        tk.Button(btn_row, text="Reject", command=_reject).pack(side=tk.LEFT)

        dialog.bind("<Escape>", lambda _e: _reject())
        dialog.bind("<Return>", lambda _e: _approve())
        dialog.protocol("WM_DELETE_WINDOW", _reject)

        dialog.wait_window()
        approved = bool(result.get("approved"))
        updated_command = str(result.get("command", command))
        return approved, updated_command

    def _request_terminal_approval(self, command: str, context: str) -> tuple[bool, str | None]:
        """Request command approval, marshaling to Tk main thread when needed."""
        if not self._enable_gui_chat:
            # In headless mode there is no modal approval UI.
            return False, command

        if threading.current_thread() is threading.main_thread():
            try:
                return self._show_terminal_approval_dialog(command, context)
            except Exception:
                return False, command

        done = threading.Event()
        result: dict[str, object] = {"approved": False, "command": command}

        def _run_dialog() -> None:
            try:
                approved, updated = self._show_terminal_approval_dialog(command, context)
                result["approved"] = approved
                result["command"] = updated
            except Exception:
                result["approved"] = False
                result["command"] = command
            finally:
                done.set()

        self._safe_root_after(_run_dialog)
        if not done.wait(timeout=300):
            return False, command
        return bool(result.get("approved")), str(result.get("command", command))

    def _log_classification(self, classification: Optional[PromptClassificationResponse], prompt: str) -> None:
        """Delegate to StreamingController."""
        self._streaming_controller._log_classification(classification, prompt)

    def _build_shared_context(self) -> Context:
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
                wm_msg.metadata["is_working_memory"] = True
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
        prompt = SessionState.load_bootstrap_prompt(os.getcwd())
        if not prompt:
            return

        try:
            shared_context = self._build_shared_context()
            self._sync_agentix_model_capacity()
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
                if self.tui_bridge is not None:
                    try:
                        self.tui_bridge.write_output(f"###AGENT 🤖\n{response_text}\n###DONE\n")
                    except Exception:
                        logger.debug("Failed to mirror bootstrap response to TUI output", exc_info=True)
                self._output_logger.log("bootstrap_agent", response_text)
                self.refresh_working_memory_gui()
        except Exception:
            logger.exception("Bootstrap prompt execution failed")

    def _show_startup_log_locations_notice_if_enabled(self) -> None:
        """Display startup log location guidance in the output panel when enabled."""
        show_notice = self.config.get("agentx", {}).get("show_log_locations_on_startup", True)
        if not show_notice:
            return

        base_sessions_dir = Path(self.user_history_folder).parent
        app_log_dir = base_sessions_dir / "_logs"
        app_log_path = app_log_dir / "agentx.log"
        classification_log_path = app_log_dir / "classification.jsonl"
        session_log_path = Path(self._session_log_path)
        output_log_path = Path(self.session_folder) / "output_log.jsonl"

        notice = (
            "Log files for this session:\n"
            f"- Session transcript: {session_log_path}\n"
            f"- Output stream (JSONL): {output_log_path}\n"
            f"- App runtime log: {app_log_path}\n"
            f"- Classification log: {classification_log_path}\n"
            "Note: a location may appear empty until its first write event."
        )
        self.gui.display_startup_notice(notice)
        if self.tui_bridge is not None:
            try:
                self.tui_bridge.write_output(f"###SYSTEM Startup\n{notice}\n")
            except Exception:
                logger.debug("Failed to mirror startup notice to TUI output", exc_info=True)
        self._output_logger.log("startup", notice)

    def process_prompt(self, prompt: str) -> Iterator[ResponseChunk]:
        """Process a prompt and yield response chunks (test-friendly API)."""
        shared_context = self._build_shared_context()
        self._sync_agentix_model_capacity()

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
        if model == self._active_model:
            return

        self._active_model = model
        self.config["agentx"]["ollama_model"] = model

        # Update the bridge's config
        self.agentix_adapter.agentix_config.model = model
        self._sync_agentix_model_capacity()
        self.agentix_adapter.invalidate_max_tokens()

        max_tokens, breakdown = self.context_meter_payload(model_name=model)
        self.schedule_meter_redraw(max_tokens, breakdown)

    def _sync_agentix_model_capacity(self) -> None:
        """Refresh Agentix config with the best-known context length for the active model."""
        self.agentix_adapter.agentix_config.model_max_tokens = self._model_store.get_context_length(self._active_model)

    def context_meter_payload(self, model_name: Optional[str] = None) -> tuple[int, dict[str, int]]:
        """Build denominator and token-band payload for context-meter redraws.

        Args:
            model_name: Optional model override.  When ``None`` the current
                :attr:`active_model` is used.

        Returns:
            ``(max_tokens, breakdown)`` where *max_tokens* is the context-window
            capacity and *breakdown* is a per-band token-count mapping.
        """
        # Explicit None check — an empty string is a valid (if unusual) model name.
        if model_name is None:
            model_name = self._active_model

        # get_context_length() never raises; it returns FALLBACK_CONTEXT_WINDOW on error.
        max_tokens: int = self._model_store.get_context_length(model_name)

        breakdown: dict[str, int] = {}
        try:
            breakdown = self.context.token_breakdown(model_name=model_name)
        except Exception:
            logger.exception("Failed to build context token breakdown for model '%s'", model_name)

        return max_tokens, breakdown

    def _context_meter_payload(self, model_name: Optional[str] = None) -> tuple[int, dict[str, int]]:
        """Backward-compatible wrapper for the public meter payload API."""
        return self.context_meter_payload(model_name=model_name)

    def on_context_assembled(self, shared_context: "Context") -> None:
        """Update the context meter from the fully assembled shared context.

        Called by :class:`~agentx.streaming_controller.StreamingController`
        immediately after :meth:`_build_shared_context` so the meter reflects
        working memory and history as well as the session's own messages.

        Args:
            shared_context: The assembled :class:`~shared.models.context.Context`
                ready to be sent to the LLM.
        """
        max_tokens: int = self._model_store.get_context_length(self._active_model)
        try:
            breakdown = shared_context.token_breakdown(model_name=self._active_model)
        except Exception:
            logger.exception("Failed to build context token breakdown for on_context_assembled")
            breakdown = {}
        self.schedule_meter_redraw(max_tokens, breakdown)

    def schedule_meter_redraw(self, max_tokens: int, breakdown: dict[str, int]) -> None:
        """Schedule a context-meter redraw safely on the Tk main thread."""
        self._safe_root_after(lambda: self.gui.update_context_meter(max_tokens=max_tokens, breakdown=breakdown))
        self._emit_tui_context_visualization(max_tokens=max_tokens, breakdown=breakdown)

    def _emit_tui_context_visualization(self, max_tokens: int, breakdown: dict[str, int]) -> None:
        """Emit the PD-16-AF-009 context bar block to TUI output subscribers."""
        if getattr(self, "tui_bridge", None) is None:
            return

        normalized_items = tuple(sorted((key, max(int(value), 0)) for key, value in breakdown.items()))
        signature = (max(int(max_tokens), 1), normalized_items)
        if signature == getattr(self, "_last_tui_context_signature", None):
            return

        self._last_tui_context_signature = signature
        try:
            context_block = TuiBridge.render_context_visualization(
                max_tokens=max_tokens,
                breakdown=breakdown,
                use_color=True,
            )
            broker = getattr(self, "event_broker", None)
            if broker is None:
                return
            broker.publish(EventType.AGENT_CONTENT, {"text": context_block, "is_raw_tui": True})
        except Exception:
            logger.exception("Failed to emit TUI context visualization")

    def _schedule_meter_redraw(self, max_tokens: int, breakdown: dict[str, int]) -> None:
        """Backward-compatible wrapper for the public redraw API."""
        self.schedule_meter_redraw(max_tokens=max_tokens, breakdown=breakdown)

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
                sort_order=normalize_context_history_session_sort(
                    self.config.get("agentx", {}).get("context_history_session_sort", "Ascending")
                ),
            )
        return self._history

    @history.setter
    def history(self, value: "History"):
        self._history = value

    # Callback handlers for GUIManager

    def _handle_submit(self) -> None:
        """Handle user submit button click."""
        self.stream_ollama_response()

    def _on_tui_submit(self, prompt_text: str) -> None:
        """Schedule submit handling for prompt text coming from TUI input FIFO."""
        cleaned_prompt = prompt_text.strip()
        if not cleaned_prompt:
            return

        def _submit() -> None:
            self._pending_prompt = cleaned_prompt
            self.stream_ollama_response()

        self._safe_root_after(_submit)

    def _on_tui_quit(self) -> None:
        """Schedule graceful application shutdown from TUI quit affordance. [PD-16-AF-008]"""

        def _quit() -> None:
            self.interrupt_streaming()
            try:
                if self.tui_bridge is not None:
                    self.tui_bridge.write_output("###SYSTEM Quit requested from TUI (\\q). Shutting down...\n")
            except Exception:
                logger.debug("Failed to mirror TUI quit notice", exc_info=True)
            self.root.quit()

        self._safe_root_after(_quit)

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
        max_tokens, breakdown = self._context_meter_payload(model_name=self.active_model)
        self._schedule_meter_redraw(max_tokens, breakdown)

    def _setup_agentix_ui(self) -> None:
        """Setup model selector and tool panel from Agentix."""

        # Register model-change callback through the protocol interface.
        def on_model_change(model: str):
            logger.debug("Model selector changed to: %s", model)
            self.active_model = model  # Use property setter for 3-way sync
            logger.debug("Session.active_model updated to: %s", self.active_model)

        self.gui.set_model_change_callback(on_model_change)

        def _refresh_models() -> None:
            """Reload the model list from Agentix and re-populate the dropdown (PD-04-AF-004)."""
            try:
                models = self.agentix_adapter.get_models()
                if models:
                    self.gui.populate_models(models, initial_model=self.active_model)
            except Exception as exc:
                logger.exception("Error refreshing models: %s", exc)

        self.gui.set_refresh_models_callback(_refresh_models)

        # Always populate models from Agentix (integrated and always available)
        try:
            models = self.agentix_adapter.get_models()
            if models:
                self.gui.populate_models(models, initial_model=self.active_model)
        except Exception as e:
            logger.exception("Error loading models: %s", e)

        # Populate tools from registry
        try:
            if self.agentix_adapter.tool_registry_manager:
                registry_manager = self.agentix_adapter.tool_registry_manager

                # Populate UI with all tools (enabled and disabled)
                all_tools = registry_manager.get_available_tools()
                if all_tools:
                    self.gui.populate_tools(all_tools)

                # Register tool-toggle callback to update registry and bridge
                def on_tool_toggle(tool_name: str, enabled: bool):
                    registry_manager.toggle_tool(tool_name, enabled)
                    # Bridge updates automatically via registry callback

                self.gui.set_tool_toggle_callback(on_tool_toggle)
            else:
                # Fallback if registry manager is not initialized
                tools = self.agentix_adapter.get_tools()
                if tools:
                    self.gui.populate_tools(tools)
        except Exception as e:
            logger.exception("Error loading tools: %s", e)

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
        # Keep dispatcher in sync with current executor references (tests may replace them)
        self._tool_dispatcher._client = self.client_tool_executor
        self._tool_dispatcher._server = self.server_tool_executor
        return self._tool_dispatcher.execute_tool(tool_name, tool_input)

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
            logger.exception("Error handling tool call")

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
        self._streaming_controller._run_replay_subtask_worker(node, tree, task_id)

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
        self.gui.update_history_panel(
            history_widget,
            context_count=len(self.history.sessions),
            sort_order=getattr(self.history, "sort_order", None),
        )

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
            except Exception as exc:
                logger.debug("Could not load models for settings panel: %s", exc)
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
                elif leaf == "context_history_session_sort":
                    history = History(
                        user_history_path=self.user_history_folder,
                        exclude_session=self.context_folder,
                        sort_order=str(value),
                    )
                    self.history = history
                    self._state.history = history
                    self._safe_root_after(self.refresh_context_gui)
            elif section == "terminal":
                if leaf == "exec_mode":
                    self._apply_terminal_exec_mode(str(value))
                else:
                    try:
                        from .integration.terminal_bridge import reload_terminal_config

                        reload_terminal_config(self.config)
                    except Exception:
                        pass
                    self._update_terminal_status_strip()
        except AttributeError:
            pass  # Agentix not available

    def attach_file(self, file_path: str):
        """
        Attach a file to the session context.
        :param file_path: The path to the file to be attached.
        """
        self.message.attach(file_path)
        self.refresh_user_gui()

    def _open_file_in_editor(self, file_path: str) -> None:
        """Open ``file_path`` in the running neovim instance as a new buffer.

        Called by the FileExplorer "Edit" context-menu entry (``on_edit`` callback).
        Delegates to :class:`VimBridge` which communicates via ``nvim --server``.
        If neovim is not connected, a warning is logged and nothing is sent.

        Args:
            file_path: Absolute path to the file selected in the FileExplorer.
        """

        success = self.vim_bridge.open_file_from_context(file_path)
        if not success:
            logger.warning(
                "Could not open %s in neovim — is the editor running?",
                file_path,
            )

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
            on_edit=self._open_file_in_editor,
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
        if not self._enable_gui_chat:
            return

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
        self._show_startup_log_locations_notice_if_enabled()
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
        except (RuntimeError, tk.TclError):
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
        self._streaming_controller._run_retrigger_synthesis_worker(node, tree, task_id, hint)

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
                    logger.warning("Failed to persist task-node WM hint state", exc_info=True)

        self.gui.mark_plan_node_invalidated(task_id)

    def _stream_via_agentix(self):
        """Stream response through Agentix middleware."""
        self._streaming_controller.stream_via_agentix()

    def _make_classification_callback(self, config: dict):
        """Delegate to StreamingController."""
        return self._streaming_controller._make_classification_callback(config)

    def _display_thinking(self, text: str) -> None:
        """Delegate to StreamingController."""
        self._streaming_controller._display_thinking(text)

    def _display_tool_call(self, tool_name: str, tool_input: dict, round_index: int | None = None) -> None:
        """Delegate to StreamingController."""
        self._streaming_controller._display_tool_call(tool_name, tool_input, round_index)

    def _display_tool_result(
        self, tool_name: str, output, round_index: int | None = None, tool_id: str | None = None
    ) -> None:
        """Delegate to StreamingController."""
        self._streaming_controller._display_tool_result(tool_name, output, round_index, tool_id=tool_id)

    def _display_assistant_header(self) -> None:
        """Delegate to StreamingController."""
        self._streaming_controller._display_assistant_header()

    def _handle_stream_content(self, text: str) -> None:
        """Delegate to StreamingController."""
        self._streaming_controller._handle_stream_content(text)

    def _persist_stream_messages(
        self,
        thinking_text: str,
        content_text: str,
        synthesis_of: list[str] | None = None,
        refresh_gui: bool = True,
    ) -> None:
        """Delegate to StreamingController."""
        self._streaming_controller._persist_stream_messages(
            thinking_text=thinking_text,
            content_text=content_text,
            synthesis_of=synthesis_of,
            refresh_gui=refresh_gui,
        )

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
        logger.info("Ensuring services are running: %s", ", ".join(services_to_start))
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

        logger.info("Connecting to Ollama at %s", url)

        try:
            with httpx.Client(timeout=timeout_seconds) as client:
                response = client.post(url, json=payload, headers=headers)
                response.raise_for_status()
                logger.info("Service handshake and model invocation successful.")

                # List available models
                try:
                    models_response = client.get(f"http://{ollama_host}/api/tags")
                    if models_response.status_code == 200:
                        import json

                        models_data = models_response.json()
                        models = models_data.get("models", [])
                        if models:
                            logger.info("Available Ollama models (%d):", len(models))
                            for model in models:
                                model_name = model.get("name", "unknown")
                                # Show simplified name without tag
                                display_name = model_name.split(":")[0] if ":" in model_name else model_name
                                logger.info("  %s", display_name)
                        else:
                            logger.warning("No models available in Ollama")
                except Exception as e:
                    logger.warning("Could not fetch model list: %s", e)

                # Show service status
                logger.info("Service Status:")
                logger.info("  Ollama: Ready")
                if all_services_started:
                    logger.info("  Agentix: Ready (code analysis available)")
                else:
                    logger.warning("  Agentix: Failed (code analysis unavailable)")

        except httpx.RequestError as e:
            raise RuntimeError(f"Failed to connect to Ollama at {url}") from e

    def stream_ollama_response(self) -> None:
        """Start streaming response in a background thread."""
        if self._streaming_thread and self._streaming_thread.is_alive():
            logger.warning("Streaming already in progress")
            return
        # Capture and clear input before starting the worker.
        if self._pending_prompt is None:
            self._pending_prompt = self.gui.get_user_input()
        if not self._pending_prompt and not self.message.attachments:
            self.gui.display_error("No input provided.")
            self._pending_prompt = None
            return
        max_tokens, breakdown = self._context_meter_payload(model_name=self.active_model)
        self._schedule_meter_redraw(max_tokens, breakdown)
        self._streaming_thread = threading.Thread(target=self.stream_ollama_response_worker, daemon=True)
        self._streaming_thread.start()

    def close(self) -> None:
        """Gracefully shut down background threads and release file handles.

        Safe to call multiple times; subsequent calls are no-ops for closed resources.
        Intended to be called from the window-close handler or the ``finally`` block
        in :func:`main` so that all file handles are flushed and daemon threads have
        a chance to finish their current I/O before the process exits.
        """
        # Signal the streaming worker to stop (idempotent).
        self._is_streaming.clear()

        # Give each active background thread up to 2 s to finish gracefully.
        for thread in (
            self._streaming_thread,
            self._last_synthesis_thread,
            self._last_replay_thread,
        ):
            if thread is not None and thread.is_alive():
                thread.join(timeout=2.0)

        # Flush and close the plain-text session transcript.
        try:
            self._session_log.close()
        except Exception:
            pass

        # Flush and close the structured JSON-lines log.
        self._output_logger.close()

        if self.tui_bridge is not None:
            try:
                self.tui_bridge.stop()
            except Exception:
                pass

        if self.tui_event_subscriber is not None:
            try:
                self.tui_event_subscriber.stop()
            except Exception:
                pass

        try:
            self.gui.destroy()
        except Exception:
            pass

    def interrupt_streaming(self):
        """
        Interrupts the ongoing streaming process.
        """
        logger.info("Interrupting streaming...")
        self._is_streaming.clear()
