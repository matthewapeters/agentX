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

    def load_messages(self, messages_json: str) -> None:
        """
        load_messages

        Use this method to load messages from a JSON files into the Context object.

        :param self: Description
        :param messages_json: JSON string representing a list of messages.
        """
        save_epoch = ""
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

    def to_gui(self, root, on_attachment_toggle=None):
        """
        to_gui

        Use this method to render the Context object in a tkinter GUI.

        Example:
        ▼  Context (2 messages)     # expanded view of context
            ▼ [ ] ⚙️  You are a helpful assistant...    # Enambled system prompt, expanded
                      📁  canned prompt1.txt
                      📁  canned prompt2.txt
            ▼ [ ] 👤  This is the message content...    # Enabled user prompt, expanded
                      📁 filename.txt
                      📁 filename.txt
                      📁 filename.txt
            ▶ [x] 🤖  That is a great question...       # Disabled agent response, collapsed

        Example:
        ▶  Context (2 messages)     # collapsed view of context

        Explanation of GUI elements:

        - A toggle button (▼ or ▶) to expand/collapse the context content (row 0, column 0)
        - A label showing "Context (N messages)" where N is the number of messages in the context (row 0, column 1)
        - A frame containing the context content (row 1, column 1) that can be expanded or collapsed.



        :param self: Description
        :param root: The tkinter root or parent widget where the context will be rendered.
        :param on_attachment_toggle: Optional callback when attachment enabled state changes.
        """
        context_frame = tk.Frame(root)

        expanded_var = tk.BooleanVar(value=True)
        expand_collapse = {
            True: "▼",
            False: "▶",
        }

        def toggle_expand():
            expanded = expanded_var.get()
            expanded_var.set(not expanded)
            self.expanded = expanded_var.get()
            collapse_expand_button.config(text=expand_collapse[expanded_var.get()])
            if expanded:
                context_messages_frame.grid_remove()
            else:
                context_messages_frame.grid(row=1, column=1, columnspan=2, sticky="w")

        collapse_expand_button = tk.Button(
            context_frame,
            command=toggle_expand,
            text=expand_collapse[expanded_var.get()],
            width=1,
            height=1,
            font=("Terminal", 10),
        )
        collapse_expand_button.grid(row=0, column=0, sticky="w")

        context_label = tk.Label(
            context_frame,
            text=f"{self.session_id or 'Context'} ({len(self.messages)} messages)",
            font=("Terminal", 10, "bold"),
        )
        context_label.grid(row=0, column=1, sticky="w")

        # Normally this is True, but for history contexts we start collapsed
        context_messages_frame = tk.Frame(context_frame)
        context_messages_frame.grid(row=1, column=1, columnspan=2, sticky="w")

        for idx, (ts, message) in enumerate(self.messages):
            m_frame = message.to_gui(context_messages_frame, on_attachment_toggle=on_attachment_toggle)
            m_frame.grid(row=idx, column=0, sticky="w")

        if not self.expanded:
            toggle_expand()

        return context_frame
