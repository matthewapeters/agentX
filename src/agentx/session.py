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
from typing import Any, Optional, Iterator

import httpx

from .attachment_info import AttachmentInfo
from .output_logger import OutputLogger
from shared.models.context import Context
from shared.models.working_memory import FactOwner, WorkingMemory
from .file_explorer import FileExplorer
from .gui.gui_config import GUIConfig
from .config import save_config
from .gui.gui_manager import GUIManager
from .history import History
from shared.models.message import Message, MessageRole
from shared.models.response import ResponseChunk, ChunkType
from .service_manager import ServiceManager
from .integration import (
    AgentixBridgeAdapter,
    ResponseHandler,
    ClientToolExecutor,
    ServerToolExecutor,
    AdvancedToolRegistry,
)
from .integration import agentix_bridge_adapter

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

    def process_prompt(self, prompt: str) -> Iterator[ResponseChunk]:
        """Process a prompt and yield response chunks (test-friendly API)."""
        shared_context = self._build_shared_context_from_context()

        user_message = Message(role=MessageRole.USER, content=prompt)
        user_message.enabled = True
        self.context.add_message(user_message, ts=datetime.now())

        classification = None
        if self.config.get("agentix", {}).get("classify_prompts", False):
            classification = self.agentix_adapter.classify_prompt_sync(prompt, shared_context)
            if classification:
                yield ResponseChunk(
                    type=ChunkType.THINKING,
                    content=classification.reasoning_summary or "",
                    classification=classification.__dict__,
                )

        thinking_parts: list[str] = []
        content_parts: list[str] = []
        for chunk in self.agentix_adapter.process_prompt_generator(
            prompt, shared_context, classification
        ):
            if chunk.type == ChunkType.THINKING and chunk.content:
                thinking_parts.append(chunk.content)
            elif chunk.type == ChunkType.CONTENT and chunk.content:
                content_parts.append(chunk.content)
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
            client_tool_names = {
                "read_file",
                "list_directory",
                "write_file",
                "get_file_info",
                "search_files"
            }

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
            self._safe_root_after(
                lambda: self.gui.display_agent_response(
                    f"\n[🔧 Calling tool: {tool_name}]\n"
                )
            )

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
                    f"[📋 Tool result: {result[:100]}...]\n"
                    if len(result) > 100
                    else f"[📋 Tool result: {result}]\n"
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
            AttachmentInfo.from_attachment(att, is_from_history=False)
            for att in self.message.attachments
        ]

        # Convert enabled history attachments to AttachmentInfo DTOs
        history_attachments = [
            AttachmentInfo.from_attachment(att, is_from_history=True)
            for att in self.enabled_history_attachments
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
        attachment_filenames = [
            os.path.basename(att.file_path) for att in self.message.attachments
        ]
        self._safe_root_after(
            lambda: self.gui.display_user_message(
                prompt, attachment_filenames, datetime.now()
            )
        )
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
            shared_context = Context()

            # Prepend Working Memory block as a system message when enabled
            wm_config = config.get("agentx", {}).get("working_memory", {})
            if (
                wm_config.get("enabled", True)
                and wm_config.get("inject_into_context", True)
                and self.working_memory is not None
            ):
                wm_block = self.working_memory.to_llm_block()
                if wm_block:
                    wm_msg = Message(role=MessageRole.SYSTEM, content=wm_block)
                    shared_context.add_message(wm_msg, ts=datetime.now())

            # Add history messages
            history_messages = self.history.get_enabled_messages()
            for ts, msg in history_messages:
                if hasattr(msg, "message"):
                    msg = msg.message
                shared_context.add_message(msg, ts=ts)

            # Add current context messages
            for msg in self.context.messages:
                if hasattr(msg, "message"):
                    msg = msg.message
                if getattr(msg, "enabled", False):
                    shared_context.add_message(msg, ts=msg.timestamp)

            # Add current message
            self.add_message_to_context(self.message)

            # Reset message and history attachments
            self.message = Message(role="user", content="")
            self.enabled_history_attachments = []

            # Classify prompt if enabled
            classification = None
            if config.get("agentix", {}).get("classify_prompts", True):
                classification = self.agentix_adapter.classify_prompt_sync(prompt, shared_context)

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
                on_tool_result=lambda tool_name, output, round_i=None, tool_id=None: self._display_tool_result(tool_name, output, round_i, tool_id=tool_id),
                on_error=lambda msg, code: self._safe_root_after(
                    lambda: self.gui.display_error(f"{code}: {msg}")
                ),
                on_classification=self._make_classification_callback(config),
            )

            # Stream through Agentix
            for chunk in self.agentix_adapter.process_prompt_generator(
                prompt, shared_context, classification
            ):
                if not self._is_streaming.is_set():
                    break

                handler.process_chunk(chunk)

                if chunk.type == ChunkType.THINKING and chunk.content:
                    thinking_parts.append(chunk.content)
                elif chunk.type == ChunkType.CONTENT and chunk.content:
                    content_parts.append(chunk.content)

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
            self._safe_root_after(
                lambda: self.gui.display_agent_thinking(header)
            )
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

    def _display_tool_result(self, tool_name: str, output, round_index: int | None = None, tool_id: str | None = None) -> None:
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
        timeout_seconds = config["agentx"].get(
            "ollama_initial_load_timeout_seconds", 120
        )

        # Determine which services to ensure are running
        services_to_start = ["ollama", "agentix"]

        # Start services
        print(f"Ensuring services are running: {', '.join(services_to_start)}")
        all_services_started = self.service_manager.ensure_services(services_to_start, timeout=30)

        if not all_services_started:
            raise RuntimeError(
                "Agentix service did not start successfully. "
                "AgentX requires Agentix for model interactions."
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
        self._streaming_thread = threading.Thread(
            target=self.stream_ollama_response_worker, daemon=True
        )
        self._streaming_thread.start()

    def interrupt_streaming(self):
        """
        Interrupts the ongoing streaming process.
        """
        print("Interrupting streaming...")
        self._is_streaming.clear()
