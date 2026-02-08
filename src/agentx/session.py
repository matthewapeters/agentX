"""
Docstring for agentx.session
"""

import os
import threading
import tkinter as tk
from datetime import datetime
from typing import Any, Optional, Iterator

import httpx
from ollama import Client

from .attachment_info import AttachmentInfo
from shared.models.context import Context
from .file_explorer import FileExplorer
from .gui.gui_config import GUIConfig
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
        use_agentix: Optional[bool] = None,
    ):
        if config is None:
            raise ValueError("config is required")

        self.root = root or tk.Tk()
        if root is None:
            self.root.withdraw()
        self.config = config
        if use_agentix is not None:
            self.config.setdefault("agentix", {})["enabled"] = use_agentix

        self.context = Context()
        self.file_explorer = FileExplorer(start_path=os.getcwd())
        self.user = username or os.getenv("USER") or os.getenv("USERNAME") or "User"
        self.start_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        # Title will be set by GUIManager after initialization
        base_dir = session_dir or os.getcwd()
        self.user_history_folder = os.path.join(
            base_dir,
            "sessions",
            self.user,
        )
        self.session_folder = os.path.join(
            self.user_history_folder,
            f"session_{self.start_time.replace(' ', '_').replace(':', '-')}",
        )
        os.makedirs(self.session_folder, exist_ok=True)
        self.context_folder = os.path.join(self.session_folder, "context")
        os.makedirs(self.context_folder, exist_ok=True)
        self.context.path = self.context_folder
        self._history = None  # Placeholder for History object
        self.message = Message(role="user", content="")
        self.enabled_history_attachments = []  # Track enabled attachments from history
        self._is_streaming = threading.Event()
        self._streaming_thread = None
        
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
        
        # Initialize tool executors
        self.client_tool_executor = ClientToolExecutor(base_path=os.getcwd())
        self.server_tool_executor = ServerToolExecutor(agentix_bridge=self.agentix_adapter.bridge)
        self.advanced_tools = AdvancedToolRegistry(agentix_bridge=self.agentix_adapter.bridge)
        
        # Initialize active model from config
        self._active_model = config["agentx"]["ollama_model"]

    def process_prompt(self, prompt: str) -> Iterator[ResponseChunk]:
        """Process a prompt and yield response chunks (test-friendly API)."""
        shared_context = Context()
        for entry in self.context.messages:
            msg = entry.message if hasattr(entry, "message") else entry
            if getattr(msg, "enabled", False):
                shared_context.add_message(msg, ts=msg.timestamp)

        if self.config.get("agentix", {}).get("enabled", False):
            classification = None
            if self.config.get("agentix", {}).get("classify_prompts", False):
                classification = self.agentix_adapter.classify_prompt_sync(prompt, shared_context)
                if classification:
                    yield ResponseChunk(
                        type=ChunkType.THINKING,
                        content=classification.reasoning_summary or "",
                        classification=classification.__dict__,
                    )

            for chunk in self.agentix_adapter.process_prompt_generator(
                prompt, shared_context, classification
            ):
                yield chunk
            return

        # Fallback response for non-Agentix tests without Ollama
        yield ResponseChunk(type=ChunkType.CONTENT, content="Test response")
        yield ResponseChunk(type=ChunkType.DONE, content="", done_reason="stop")

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
                if not self.config.get("agentix", {}).get("enabled", False):
                    return f"Code analysis tool '{tool_name}' not available - Agentix disabled"
                if self.server_tool_executor.is_available():
                    return self.server_tool_executor.execute(tool_name, tool_input)
                else:
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
            self.gui.display_agent_response(
                f"\n[🔧 Calling tool: {tool_name}]\n"
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
            self.gui.display_agent_response(
                f"[📋 Tool result: {result[:100]}...]\n" if len(result) > 100 else f"[📋 Tool result: {result}]\n"
            )
            
        except Exception as e:
            error_msg = f"Error handling tool call: {e}"
            self.gui.display_error(error_msg)
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
        )
        # Update via GUIManager
        self.gui.update_files_panel(files_widget)

    def add_message_to_context(self, message: Message):
        """
        Adds a message to the session context and refreshes the context GUI.
        """
        time_added = datetime.now()
        self.context.add_message(message, ts=time_added)
        if threading.current_thread() is threading.main_thread():
            self.refresh_context_gui()

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
        
        # Setup model selector and tool panel if Agentix is available
        # (Must be after layout is created so the widgets exist)
        self._setup_agentix_ui()

    def stream_ollama_response_worker(self):
        """
        Worker function that streams the response from the Ollama server and updates the output via GUIManager.
        This runs in a separate thread to keep the GUI responsive.
        
        If Agentix is enabled, routes through Agentix middleware for classification and tool support.
        Otherwise, uses direct Ollama streaming.
        """
        # Route to Agentix streaming
        self._stream_via_agentix()

    def _safe_root_after(self, callback) -> None:
        if threading.current_thread() is threading.main_thread():
            try:
                self.root.after(0, callback)
            except RuntimeError:
                pass
    
    def _stream_via_agentix(self):
        """Stream response through Agentix middleware."""
        config = self.config

        self._is_streaming.set()
        self.gui.set_streaming_state(True)
        self.refresh_user_gui()

        # Get the prompt from the user input
        prompt = self.gui.get_user_input()

        if not prompt and not self.message.attachments:
            if threading.current_thread() is not threading.main_thread():
                prompt = self.gui._cached_user_input or " "
            else:
                self.gui.display_error("No input provided.")
                self._is_streaming.clear()
                self.gui.set_streaming_state(False)
                return

        # Display the user prompt and attachments
        attachment_filenames = [
            os.path.basename(att.file_path) for att in self.message.attachments
        ]
        self.gui.display_user_message(prompt, attachment_filenames, datetime.now())

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
                if classification and config.get("agentix", {}).get("show_classification", False):
                    self.gui.display_agent_thinking(
                        f"\n[Classification: {classification.intent.name} → {classification.next_step.name}]\n"
                    )

            # Create response handler
            agent_response_message = Message(role="assistant", content="")
            thinking_shown = False
            
            handler = ResponseHandler(
                on_content=lambda text: self.gui.display_agent_response(text),
                on_thinking=lambda text: self._display_thinking(text),
                on_tool_call=lambda name, args: self.handle_tool_call(name, args),
                on_tool_result=lambda id, result: self.gui.display_agent_response(
                    f"\n[📋 Tool result: {result[:100]}...]\n" if len(result) > 100 else f"\n[📋 Tool result: {result}]\n"
                ),
                on_error=lambda msg, code: self.gui.display_error(f"{code}: {msg}"),
            )

            # Display assistant header
            self.gui.display_agent_response(
                f"\n\n{GUIManager.MESSAGE_ROLES['assistant']} ({self.active_model})\t"
            )

            # Stream through Agentix
            for chunk in self.agentix_adapter.process_prompt_generator(
                prompt, shared_context, classification
            ):
                if not self._is_streaming.is_set():
                    break
                
                handler.process_chunk(chunk)
                
                # Accumulate content for message
                if chunk.type.value == "content":
                    agent_response_message.content += chunk.content
                
                self.refresh_user_gui()

            # Complete the response
            self.gui.display_spacing()
            if agent_response_message.content:
                self.add_message_to_context(agent_response_message)
            self.refresh_user_gui()

        except Exception as e:
            import traceback
            self.gui.display_error(f"Error: {e}")
            print(f"Request error: {e}")
            traceback.print_exc()
        finally:
            self._is_streaming.clear()
            self.gui.set_streaming_state(False)
    
    def _display_thinking(self, text: str):
        """Helper to display thinking text with header on first call."""
        if not hasattr(self, '_thinking_header_shown'):
            self.gui.display_agent_thinking(
                f"\n{GUIManager.MESSAGE_ROLES['thinking']} ({self.active_model})\t(The agent is thinking...)\n"
            )
            self._thinking_header_shown = True
        self.gui.display_agent_thinking(text)
    
    def _stream_direct_ollama(self):
        """Stream response directly from Ollama (original implementation)."""
        config = self.config

        self._is_streaming.set()
        self.gui.set_streaming_state(True)  # Update GUI to streaming state

        self.refresh_user_gui()

        # Load configuration
        ollama_host = config["agentx"]["ollama_host"]
        ollama_model = self.active_model

        # Get the prompt from the user input (via GUIManager)
        prompt = self.gui.get_user_input()

        if not prompt and not self.message.attachments:
            self.gui.display_error("No input provided.")
            return

        # Build the full user message including only enabled attached file contents
        full_prompt = prompt

        # Display the user prompt and attachments
        attachment_filenames = [
            os.path.basename(att.file_path) for att in self.message.attachments
        ]
        self.gui.display_user_message(prompt, attachment_filenames, datetime.now())

        try:
            # Define the message payload
            self.message.content = full_prompt
            # Enable the message before adding to context
            self.message.enabled = True

            # Refresh context GUI to show the enabled message immediately
            self._safe_root_after(self.refresh_context_gui)

            # Add enabled history attachments to the current message
            for att in self.enabled_history_attachments:
                if att not in self.message.attachments:
                    self.message.attachments.append(att)

            agent_thinking_message = Message(role="thinking", content="")
            agent_thinking_message.enabled = False
            agent_response_message = Message(role="assistant", content="")

            self.add_message_to_context(self.message)

            # Reset the message and clear enabled history attachments
            self.message = Message(role="user", content="")
            self.enabled_history_attachments = []

            # Build LLM context from enabled messages/attachments in history (all sessions)
            llm_messages = []

            # Collect enabled messages from history
            history_messages = self.history.get_enabled_messages()
            for _, msg in history_messages:
                llm_messages.append(msg.llm_message_dict())

            # Also include enabled messages from the current context
            for msg in self.context.messages:
                if hasattr(msg, "message"):
                    msg = msg.message
                if getattr(msg, "enabled", False):
                    if hasattr(msg, "attachments"):
                        msg.attachments = [
                            a for a in msg.attachments if getattr(a, "enabled", False)
                        ]
                    llm_messages.append(msg.llm_message_dict())

            client = Client(host=f"http://{ollama_host}")
            for part in client.chat(
                model=ollama_model,
                messages=llm_messages,
                stream=True,
            ):
                if not self._is_streaming.is_set():
                    break  # Exit the loop if streaming is interrupted

                if part.message.thinking:
                    self._display_thinking(part.message.thinking)
                if part.message.content:
                    agent_response_message.content += part.message.content
                    self.gui.display_agent_response(part.message.content)

                self.refresh_user_gui()
            # After streaming is complete, add spacing
            self.gui.display_spacing()
            self.add_message_to_context(agent_response_message)
            self.refresh_user_gui()

        except Exception as e:
            import traceback

            print(f"Request error: {e}")
            traceback.print_exc()
    def perform_service_handshake(self):
        """
        Performs service startup and handshake with required services.
        
        Steps:
        1. Ensure Ollama service is running
        2. Ensure Agentix service is running (if enabled)
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
        services_to_start = ["ollama"]
        agentix_enabled = config.get("agentix", {}).get("enabled", False)
        if agentix_enabled:
            services_to_start.append("agentix")

        # Start services
        print(f"Ensuring services are running: {', '.join(services_to_start)}")
        all_services_started = self.service_manager.ensure_services(services_to_start, timeout=30)
        
        if not all_services_started:
            if agentix_enabled:
                print("⚠ Warning: Agentix service did not start successfully")
                print("  - Check that Agentix dependencies are installed (e.g., libcst)")
                print("  - Code analysis tools will be unavailable")
                print("  - Continuing with Ollama only...")
            else:
                print("Warning: Not all services started successfully, attempting to continue...")

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
                if agentix_enabled:
                    if all_services_started:
                        print(f"  ✓ Agentix: Ready (code analysis available)")
                    else:
                        print(f"  ✗ Agentix: Failed (code analysis unavailable)")
                else:
                    print(f"  • Agentix: Disabled (optional)")
                print()
                
        except httpx.RequestError as e:
            raise RuntimeError(f"Failed to connect to Ollama at {url}") from e
        if self._streaming_thread and self._streaming_thread.is_alive():
            print("Streaming already in progress")
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
