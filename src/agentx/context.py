"""
Docstring for src.agentx.context
"""
from datetime import datetime
import json

import tkinter as tk

from .message import Message

class Context:
    def __init__(self):
      self.messages: list[Message] = []  # List to hold context messages

    def add_message(self, ts: datetime, message: Message) -> None:
        """
        Add a new message to the context.
        """
        self.list.append((ts, message))

    def get_messages(self):
        """
        get_messages
        
        Use this method to convert the Context object to a JSON string.
        
        Only enabled messages are included in the output.
        
        :param self: Description
        """
        return json.dumps([m for m in self.messages if m.enabled])

    def _to_gui(self, root):
        """
        _to_gui

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
            collapse_expand_button.config(text=expand_collapse[expanded_var.get()])
            if expanded:
                context_messages_frame.grid_remove()
            else:
                context_messages_frame.grid(row=1, column=1, columnspan=2, sticky="w")

        collapse_expand_button = tk.Button(
            context_frame,
            command=toggle_expand,
            text=expand_collapse[expanded_var.get()],
            width=2
        )
        collapse_expand_button.grid(row=0, column=0, sticky="w")

        context_label = tk.Label(
            context_frame,
            text=f"Context ({len(self.messages)} messages)",
            font=("Terminal", 10, "bold"),
        )
        context_label.grid(row=0, column=1, sticky="w")

        context_messages_frame = tk.Frame(context_frame)
        context_messages_frame.grid(row=1, column=1, columnspan=2, sticky="w")

        for idx, (ts, message) in enumerate(self.messages):
            m_frame = message._to_gui()
            m_frame.grid(row=idx, column=0, sticky="w")

        return context_frame
