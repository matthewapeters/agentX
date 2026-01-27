"""
Docstring for agentx.session
"""
from datetime import datetime

import os

import tkinter as tk
from typing import Any

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