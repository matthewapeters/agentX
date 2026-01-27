"""
Docstring for agentx.session
"""
from datetime import datetime

import os

import tkinter as tk
from typing import Any

from .ollama_client import interrupt_streaming, stream_ollama_response
from .context import Context
from .message import Message

class AgentXSession:
    """
    AgentXSession
    """
    def __init__(self, root:tk.Tk, config: dict[str, Any]):
        self.root = root
        self.config = config
        self.context = Context()
        self.user = os.getenv("USER") or os.getenv("USERNAME") or "User"
        self.start_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        self.root.title(f"{self.user} - AgentX Session - {self.start_time}")
        self.session_folder = os.path.join(os.getcwd(), "sessions", self.user,f"session_{self.start_time.replace(' ', '_').replace(':', '-')}")
        os.makedirs(self.session_folder, exist_ok=True)
        self.context_folder = os.path.join(self.session_folder, "context")
        os.makedirs(self.context_folder, exist_ok=True)

    def add_message_to_context(self, message: Message):
        """
        Adds a message to the session context.
        """
        time_added = datetime.now()
        self.context.add_message(time_added, message=message)
        message_file = os.path.join(self.context_folder, f"{time_added.timestamp()}_{message.role}.json")
        message.file = message_file
        with open(message_file, "w", encoding="utf-8") as f:
            f.write(str(message.serialize()))


    def layout(self):
        """
        Sets up the layout for the tkinter root window.
        """
        root = self.root
        config = self.config
        text_font = ("Terminal", 10)
        enter_emoji_unicode = "^⏎"

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

        # Output display with scrollbar
        root.output_display = tk.Frame(root, bg="white")
        root.output_scrollbar = tk.Scrollbar(root.output_display)
        root.output_text = tk.Text(
            root.output_display,
            wrap=tk.WORD,
            font=text_font,
            yscrollcommand=root.output_scrollbar.set,
        )
        root.output_scrollbar.config(command=root.output_text.yview)
        root.output_text.pack(side=tk.LEFT, expand=True, fill=tk.BOTH)
        root.output_scrollbar.pack(side=tk.RIGHT, fill=tk.Y)
        # Ensure selection highlighting is visible (after output_text is created)
        root.output_text.tag_config(
            "sel", background="#3399ff", foreground="#ffffff"
        )

        root.system_status = tk.Frame(root, bg="lightblue")
        root.system_status_text = tk.Text(root.system_status, wrap=tk.WORD, font=text_font)
        root.system_status_text.pack(expand=True, fill=tk.BOTH)

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
            command=lambda: stream_ollama_response(root, config),
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

        root.output_display.place(relx=0.001, rely=0.001, relwidth=0.79, relheight=0.79)
        root.system_status.place(relx=0.80, rely=0.001, relwidth=0.2, relheight=0.79)
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
        root.output_text.tag_config(
            "user_prompt", font=("Terminal", 10, "bold")
        )
        root.output_text.tag_config(
            "agent_response", font=("Terminal", 10, "normal")
        )
        root.output_text.tag_config(
            "agent_thinking", font=("Terminal", 10, "italic")
        )
        root.output_text.tag_config(
            "system_space", font=("Terminal", 10, "normal")
        )
