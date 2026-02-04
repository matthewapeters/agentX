"""
Docstring for src.agentx.context
"""

import json
import tkinter as tk
from datetime import datetime
from glob import glob

from .message import Message


class Context:
    """
    Context class to hold a list of messages.
    """

    def __init__(self):
        self.messages: list[Message] = []  # List to hold context messages
        self.session_id: str | None = None  # Optional session ID
        self.path: str | None = None  # Optional path for context storage
        self.expanded: bool = True  # Whether the context is expanded in the GUI

    def add_message(self, ts: datetime, message: Message) -> None:
        """
        Add a new message to the context.
        """
        if message.file is None:
            message.save(self.path, ts)
        self.messages.append((ts, message))

    def get_messages(self):
        """
        get_messages

        Use this method to convert the Context object to a JSON string.

        Only enabled messages are included in the output.

        :param self: Description
        """
        return json.dumps([m.serialize() for ts, m in self.messages if m.enabled])

    def load_messages(self) -> None:
        """
        load_messages

        Use this method to load messages from a JSON files into the Context object.

        :param self: Description
        """
        g = glob(self.path + "/*.json")
        g.sort()
        for f in g:
            with open(f, "r", encoding="utf-8") as file:
                message = Message.from_dict(json.loads(file.read()), file_path=f)
                # Default all loaded messages and attachments to disabled
                message.enabled = False
                for att in getattr(message, "attachments", []):
                    if hasattr(att, "enabled"):
                        att.enabled = False
                self.messages.append((message.ts, message))
