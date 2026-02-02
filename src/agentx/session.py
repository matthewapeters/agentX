"""
Docstring for agentx.session
"""

import os
import threading
import tkinter as tk
from datetime import datetime
from tkinter import ttk
from typing import Any

import httpx
from ollama import Client

from .attachment_info import AttachmentInfo
from .context import Context
from .file_explorer import FileExplorer
from .gui_config import GUIConfig
from .gui_manager import GUIManager
from .history import History
from .message import Message


class AgentXSession:
    """
    AgentXSession
    """

    def __init__(self, root: tk.Tk, config: dict[str, Any]):
        self.root = root
        self.config = config
        self.context = Context()
        self.file_explorer = FileExplorer(start_path=os.getcwd())
        self.user = os.getenv("USER") or os.getenv("USERNAME") or "User"
        self.start_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        # Title will be set by GUIManager after initialization
        self.user_history_folder = os.path.join(
            os.getcwd(),
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
        
        # Initialize GUIManager
        gui_config = GUIConfig.from_dict(config)
        self.gui = GUIManager(
            root=root,
            config=gui_config,
            on_submit=self._handle_submit,
            on_interrupt=self._handle_interrupt,
            on_attachment_toggle=self._handle_attachment_toggle
        )
        
        # Set window title with session info
        self.gui.set_window_title(
            f"{self.user} - AgentX Session - {self.start_time}"
        )

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
        Destroys the old frames and re-renders the history and current context.
        """
        # Render history first (collapsed by default) in the Session tab
        history_widget = self.history.to_gui(
            self.gui.get_history_parent(),
            self.user,
            on_attachment_toggle=self.on_history_attachment_toggle,
        )
        self.gui.update_history_panel(history_widget)
        
        # Render current context in the Session tab
        context_widget = self.context.to_gui(
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
        self.context.add_message(ts=time_added, message=message)
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

    def stream_ollama_response_worker(self):
        """
        Worker function that streams the response from the Ollama server and updates the output via GUIManager.
        This runs in a separate thread to keep the GUI responsive.
        """
        config = self.config

        self._is_streaming.set()
        self.gui.set_streaming_state(True)  # Update GUI to streaming state

        self.refresh_user_gui()

        # Load configuration
        ollama_host = config["agentx"]["ollama_host"]
        ollama_model = config["agentx"]["ollama_model"]

        # Get the prompt from the user input (via GUIManager)
        prompt = self.gui.get_user_input()
        
        if not prompt and not self.message.attachments:
            self.gui.display_error("No input provided.")
            return

        # Build the full user message including only enabled attached file contents
        full_prompt = prompt

        # Display the user prompt and attachments
        attachment_filenames = [os.path.basename(att.file_path) for att in self.message.attachments]
        self.gui.display_user_message(prompt, attachment_filenames, datetime.now())

        try:
            # Define the message payload
            self.message.content = full_prompt
            # Enable the message before adding to context
            self.message.enabled = True

            # Refresh context GUI to show the enabled message immediately
            self.root.after(0, self.refresh_context_gui)

            # Add enabled history attachments to the current message
            for att in self.enabled_history_attachments:
                if att not in self.message.attachments:
                    self.message.attachments.append(att)

            agent_thinking_message = Message(role="assistant", content="")
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
            for _, msg in self.context.messages:
                if getattr(msg, "enabled", False):
                    if hasattr(msg, "attachments"):
                        msg.attachments = [
                            a for a in msg.attachments if getattr(a, "enabled", False)
                        ]
                    llm_messages.append(msg.llm_message_dict())

            last_channel = ""
            client = Client(host=f"http://{ollama_host}")
            for part in client.chat(
                model=ollama_model,
                messages=llm_messages,
                stream=True,
            ):
                if not self._is_streaming.is_set():
                    break  # Exit the loop if streaming is interrupted
                # print(f"Received part: {part}")  # Debugging for received part
                channels = [
                    k
                    for k, v in part.message.__dict__.items()
                    if v and k not in ["role", ""]
                ]
                if channels:
                    channel = channels[0]
                    if channel != last_channel:
                        match channel:
                            case "thinking":
                                # First thinking block - header handled by display method
                                pass
                            case "content":
                                # Transition from thinking to content
                                self.add_message_to_context(agent_thinking_message)
                            case _:
                                pass
                    match channel:
                        case "thinking":
                            self.gui.display_agent_thinking(part.message.thinking)
                            agent_thinking_message.content += part.message.thinking
                            last_channel = channel
                        case "content":
                            self.gui.display_agent_response(part.message.content)
                            agent_response_message.content += part.message.content
                            last_channel = channel
                        case "tool_name":
                            # Handle tool_name (currently pass)
                            print(
                                f"Tool name received: {part.message.tool_name}"
                            )  # Debugging for tool_name
                            last_channel = channel
                        case "tool_calls":
                            # Handle tool_calls (currently pass)
                            print(
                                f"Tool calls received: {part.message.tool_calls}"
                            )  # Debugging for tool_calls
                            last_channel = channel
                        case "images":
                            # Handle images (currently pass)
                            print(
                                f"Images received: {part.message.images}"
                            )  # Debugging for images
                            last_channel = channel
                        case _:
                            print(
                                f"Unknown channel received: {channel}"
                            )  # Debugging for unknown channels
                            last_channel = channel
                    self.refresh_user_gui()
            # After streaming is complete, add spacing
            self.gui.display_spacing()
            self.add_message_to_context(agent_response_message)
            self.refresh_user_gui()

        except Exception as e:
            import traceback

            self.gui.display_error(f"Error: {e}")
            print(f"Request error: {e}")
            traceback.print_exc()
        finally:
            self._is_streaming.clear()
            self.gui.set_streaming_state(False)  # Update GUI to idle state

    def perform_service_handshake(self):
        """
        Performs a handshake with the Ollama server and ensures the model is loaded.
        """
        config = self.config
        ollama_host = config["agentx"]["ollama_host"]
        ollama_model = config["agentx"]["ollama_model"]
        timeout_seconds = config["agentx"].get(
            "ollama_initial_load_timeout_seconds", 120
        )

        url = f"http://{ollama_host}/api/chat"
        headers = {"Content-Type": "application/json"}
        payload = {
            "model": ollama_model,
            "prompt": "",
        }  # Empty prompt to trigger model load

        try:
            with httpx.Client(timeout=timeout_seconds) as client:
                response = client.post(url, json=payload, headers=headers)
                response.raise_for_status()
                print("Service handshake and model invocation successful.")
        except httpx.RequestError as e:
            raise RuntimeError(
                f"Failed to perform service handshake and model invocation: {e}"
            )

    def stream_ollama_response(self):
        """
        Initiates streaming response in a separate thread to keep the GUI responsive.
        """
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
