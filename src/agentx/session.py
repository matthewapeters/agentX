"""
Docstring for agentx.session
"""

import json
import os
import threading
import tkinter as tk
from tkinter import ttk
from datetime import datetime
from typing import Any

import httpx
from ollama import Client

from .context import Context
from .file_explorer import FileExplorer
from .history import History
from .message import Message

is_streaming = threading.Event()
streaming_thread = None


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
        self.root.title(f"{self.user} - AgentX Session - {self.start_time}")
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

    @property
    def history(self) -> "History":
        """
        Docstring for history

        :param self: Description
        :return: Description
        :rtype: History
        """
        if self._history is None:
            self._history = History(user_history_path=self.user_history_folder)
        return self._history

    @history.setter
    def history(self, value: "History"):
        self._history = value

    def refresh_context_gui(self):
        """
        Refreshes the context GUI in the Session tab of the system status notebook.
        Destroys the old frames and re-renders the history and current context.
        """
        # Destroy existing frames
        if (
            hasattr(self.root, "system_status_history")
            and self.root.system_status_history
        ):
            self.root.system_status_history.destroy()
        if (
            hasattr(self.root, "system_status_context")
            and self.root.system_status_context
        ):
            self.root.system_status_context.destroy()

        # Render history first (collapsed by default) in the Session tab
        self.root.system_status_history = self.history.to_gui(
            self.root.session_tab, self.user
        )
        self.root.system_status_history.pack(expand=False, fill=tk.X, anchor=tk.N)

        # Render current context in the Session tab
        self.root.system_status_context = self.context.to_gui(self.root.session_tab)
        self.root.system_status_context.pack(expand=True, fill=tk.BOTH)

    def refresh_files_gui(self):
        """
        Refreshes the file explorer GUI in the Files tab of the system status notebook.
        Destroys the old frame and re-renders the file explorer.
        """
        # Destroy existing frame
        if hasattr(self.root, "system_status_files") and self.root.system_status_files:
            self.root.system_status_files.destroy()

        # Render file explorer in the Files tab
        self.root.system_status_files = self.file_explorer.to_gui(self.root.files_tab)
        self.root.system_status_files.pack(expand=True, fill=tk.BOTH)

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
        """
        root = self.root
        config = self.config
        text_font = ("Terminal", 10)
        enter_emoji_unicode = "^⏎"

        # Locate the font file relative to the installed package directory
        package_dir = os.path.dirname(__file__)
        emoji_font_path = os.path.join(package_dir, "fonts", "NotoColorEmoji.ttf")
        if os.path.exists(emoji_font_path):
            text_font = (emoji_font_path, 10)
        else:
            print("Warning: Emoji font not found. Falling back to default font.")
            text_font = ("Terminal", 10)

        # Get screen dimensions
        screen_width = root.winfo_screenwidth()
        screen_height = root.winfo_screenheight()

        # Determine screen side from config (default to "right")
        screen_side = config["agentx"].get("screen_side", "right").lower()
        if screen_side == "left":
            root.geometry(f"{screen_width // 2}x{screen_height}+0+0")
        else:  # Default to "right"
            root.geometry(f"{screen_width // 2}x{screen_height}+{screen_width // 2}+0")

        root.title("AgentX - the Ollama Agent")

        # Explicitly set the root title to include the font name for testing purposes
        if os.path.exists(emoji_font_path):
            root.title(f"AgentX - the Ollama Agent (Font: NotoColorEmoji)")
        else:
            root.title("AgentX - the Ollama Agent")

        # Create a PanedWindow for resizable output and system frames with 80:20 split
        root.paned = tk.PanedWindow(root, orient=tk.HORIZONTAL, sashrelief=tk.RAISED)
        root.paned.place(relx=0.001, rely=0.001, relwidth=0.99, relheight=0.79)

        # Output display with scrollbar
        root.output_display = tk.Frame(root.paned, bg="white")

        # Create a notebook (tabbed interface) for output
        root.output_notebook = ttk.Notebook(root.output_display)
        root.output_notebook.pack(expand=True, fill=tk.BOTH, padx=0, pady=0)

        # Create Output tab
        root.output_tab = tk.Frame(root.output_notebook, bg="white")
        root.output_notebook.add(root.output_tab, text="Output")

        # Create output text and scrollbar in the Output tab
        root.output_scrollbar = tk.Scrollbar(root.output_tab)
        root.output_text = tk.Text(
            root.output_tab,
            wrap=tk.WORD,
            font=text_font,
            yscrollcommand=root.output_scrollbar.set,
        )
        root.output_scrollbar.config(command=root.output_text.yview)
        root.output_text.pack(side=tk.LEFT, expand=True, fill=tk.BOTH)
        root.output_scrollbar.pack(side=tk.RIGHT, fill=tk.Y)
        # Ensure selection highlighting is visible (after output_text is created)
        root.output_text.tag_config("sel", background="#3399ff", foreground="#ffffff")

        root.system_status = tk.Frame(root.paned, bg="lightblue")
        # Create a notebook (tabbed interface) for system status
        root.system_notebook = ttk.Notebook(root.system_status)
        root.system_notebook.pack(expand=True, fill=tk.BOTH, padx=0, pady=0)

        # Create Session tab
        root.session_tab = tk.Frame(root.system_notebook, bg="lightblue")
        root.system_notebook.add(root.session_tab, text="Session")

        # Create Files tab
        root.files_tab = tk.Frame(root.system_notebook, bg="lightblue")
        root.system_notebook.add(root.files_tab, text="Files")

        # Initialize the Session tab content
        self.refresh_context_gui()

        # Initialize the Files tab content
        self.refresh_files_gui()

        # Bind tab change event to force widget updates
        def on_tab_changed(event):
            root.update_idletasks()
            # Force update of the selected tab's content
            selected_tab = root.system_notebook.select()
            if selected_tab:
                root.system_notebook.nametowidget(selected_tab).update_idletasks()

        root.system_notebook.bind("<<NotebookTabChanged>>", on_tab_changed)

        root.paned.add(root.output_display, stretch="always")
        root.paned.add(root.system_status, stretch="always")

        # Set the sash position to create an 80:20 split after widgets are rendered
        def set_initial_split():
            # Calculate 80% of the actual paned window width
            root.update_idletasks()  # Ensure widgets are rendered
            paned_width = root.paned.winfo_width()
            if paned_width > 1:  # Only set if widget is properly sized
                sash_position = int(paned_width * 0.66)  # 2:1 layout for output display
                root.paned.sash_place(0, sash_position, 1)

        root.after(100, set_initial_split)

        # User input with scrollbar
        root.user_input = tk.Frame(root, bg="lightgrey")
        root.input_scrollbar = tk.Scrollbar(root.user_input)
        root.user_input_text = tk.Text(
            root.user_input,
            wrap=tk.WORD,
            font=text_font,
            yscrollcommand=root.input_scrollbar.set,
        )
        root.input_scrollbar.config(command=root.user_input_text.yview)
        root.user_input_text.place(
            relx=0, rely=0, relwidth=0.90, relheight=1.0
        )  # Adjusted width to make space for scrollbar
        root.input_scrollbar.place(
            relx=0.90, rely=0, relheight=1.0
        )  # Positioned at the right edge of user_input_text

        root.user_submit = tk.Button(
            root.user_input,
            text=enter_emoji_unicode,
            command=lambda: self.stream_ollama_response_worker(),
        )
        root.user_submit.place(relx=0.92, rely=0, relwidth=0.07, relheight=0.25)

        # Add a break button below the submit button
        root.user_break = tk.Button(
            root.user_input,
            text="❌",
            command=interrupt_streaming,
            state=tk.DISABLED,
        )
        root.user_break.place(relx=0.92, rely=0.26, relwidth=0.07, relheight=0.25)

        root.user_input.place(relx=0.001, rely=0.80, relwidth=1.0, relheight=0.2)

        # Bind Ctrl-Enter to trigger the user_submit button
        root.user_input_text.bind(
            "<Control-Return>", lambda event: root.user_submit.invoke()
        )

        # Bind Ctrl-Space globally to trigger the user_break button
        root.bind_all("<Control-space>", lambda event: root.user_break.invoke())

        # Setup the Ollama client with the loaded configuration
        # Adds text styling tags to the output_text widget.
        root.output_text.tag_config(
            "gray", foreground="gray", font=("Terminal", 10, "italic")
        )
        root.output_text.tag_config("user_prompt", font=("Terminal", 10, "bold"))
        root.output_text.tag_config("agent_response", font=("Terminal", 10, "normal"))
        root.output_text.tag_config("agent_thinking", font=("Terminal", 10, "italic"))
        root.output_text.tag_config("system_space", font=("Terminal", 10, "normal"))

        # Update Ctrl-Enter button to use NotoColorEmoji font
        emoji_font_path = os.path.join(os.getcwd(), "fonts", "NotoColorEmoji.ttf")
        if os.path.exists(emoji_font_path):
            ctrl_enter_font = (emoji_font_path, 10)
        else:
            ctrl_enter_font = ("Terminal", 10)

        # Example usage for a button
        ctrl_enter_button = tk.Button(
            root, text=enter_emoji_unicode, font=ctrl_enter_font
        )

    def stream_ollama_response_worker(self):
        """
        Worker function that streams the response from the Ollama server and updates the output_text widget.
        This runs in a separate thread to keep the GUI responsive.
        """
        root = self.root
        config = self.config

        global is_streaming  # Ensure we use the global is_streaming instance
        is_streaming.set()
        root.user_break.config(state=tk.NORMAL)  # Enable the break button
        root.update_idletasks()

        # Load configuration
        ollama_host = config["agentx"]["ollama_host"]
        ollama_model = config["agentx"]["ollama_model"]

        # Get the prompt from the user_input_text widget
        prompt = root.user_input_text.get("1.0", tk.END).strip()
        if not prompt:
            root.output_text.insert(tk.END, "No input provided.\n")
            return

        # Display the user prompt in the output_text widget
        root.user_input_text.delete("1.0", tk.END)  # Clear the user input text
        root.output_text.insert(tk.END, f"User: {prompt}\n", ("user_prompt",))
        root.output_text.see(tk.END)  # Auto-scroll to the end
        root.update_idletasks()

        try:
            # Define the message payload
            user_message = Message(role="user", content=prompt)
            agent_thinking_message = Message(role="assistant", content="")
            agent_thinking_message.enabled = False
            agent_response_message = Message(role="assistant", content="")
            self.add_message_to_context(user_message)
            # Use the AsyncClient to stream responses
            last_channel = ""
            client = Client(host=f"http://{ollama_host}")
            for part in client.chat(
                model=ollama_model,
                messages=[
                    m[1].llm_message_dict()
                    for m in self.context.messages
                    if m[1].enabled
                ],
                stream=True,
            ):
                if not is_streaming.is_set():
                    break  # Exit the loop if streaming is interrupted
                # print(f"Received part: {part}")  # Debugging for received part
                channels = [
                    k
                    for k, v in part.message.__dict__.items()
                    if v and k not in ["role", ""]
                ]
                if channels:
                    channel = channels[0]
                    # print(f"Received channel: {channel}: {part.message[channel]}")  # Debugging for received channel
                    if channel != last_channel:
                        match channel:
                            case "thinking":
                                root.output_text.insert(
                                    tk.END, "\n", ("system_space",)
                                )  # Add spacing between different channels
                                root.output_text.insert(
                                    tk.END,
                                    "(Agent is thinking...)\n\n",
                                    ("agent_thinking",),
                                )
                                root.output_text.see(tk.END)  # Auto-scroll to the end
                            case "content":
                                self.add_message_to_context(agent_thinking_message)
                                root.output_text.insert(
                                    tk.END, "\n", ("agent_thinking",)
                                )  # end of line for thinking
                                root.output_text.insert(
                                    tk.END, "\n", ("system_space",)
                                )  # Add spacing between different channels
                                root.output_text.insert(
                                    tk.END, "Agent:\n\n", ("agent_response",)
                                )
                                root.update_idletasks()
                            case _:
                                pass  # For other channels, no special header
                    match channel:
                        case "thinking":
                            # Handle agent thinking content
                            root.output_text.insert(
                                tk.END, part.message.thinking, ("agent_thinking",)
                            )
                            agent_thinking_message.content += part.message.thinking
                            root.output_text.see(tk.END)  # Auto-scroll to the end
                            last_channel = channel
                        case "content":
                            # Handle agent response content
                            root.output_text.insert(
                                tk.END, part.message.content, ("agent_response",)
                            )
                            agent_response_message.content += part.message.content
                            root.output_text.see(tk.END)  # Auto-scroll to the end
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
                    root.update_idletasks()
            # After streaming is complete, add spacing
            root.output_text.insert(
                tk.END, "\n\n", ("system_space",)
            )  # Add spacing between different channels
            self.add_message_to_context(agent_response_message)
            root.update_idletasks()

        except Exception as e:
            import traceback

            root.output_text.insert(tk.END, f"Error: {e}\n")
            print(f"Request error: {e}")
            traceback.print_exc()
        finally:
            is_streaming.clear()
            root.user_break.config(state=tk.DISABLED)  # Disable the break button

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
        global streaming_thread
        root = self.root
        config = self.config
        if streaming_thread and streaming_thread.is_alive():
            print("Streaming already in progress")
            return
        streaming_thread = threading.Thread(
            target=self.stream_ollama_response_worker, args=(root, config), daemon=False
        )
        streaming_thread.start()


def interrupt_streaming():
    """
    Interrupts the ongoing streaming process.
    """
    global is_streaming
    print("Interrupting streaming...")
    is_streaming.clear()
