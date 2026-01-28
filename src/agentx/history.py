import json
import os
import tkinter as tk
from datetime import datetime

from .context import Context
from .message import Message


class History:
    """
    Docstring for History
    """

    def __init__(self, user_session_path: str):
        """
        Docstring for __init__

        :param self: Description
        :param user_session_path: Description
        :type user_session_path: str
        """
        self.records = []

        # Load the list of contexts from the user session path
        # each folder under the user session path represent a context
        # each file under the context folder represent a message
        # add each context to self.records alphabetically

        if not os.path.exists(user_session_path):
            return

        # Get all folders under user_session_path
        try:
            context_folders = [
                d
                for d in os.listdir(user_session_path)
                if os.path.isdir(os.path.join(user_session_path, d))
            ]
        except OSError:
            return

        # Sort alphabetically
        context_folders.sort()

        # Load each context
        for context_folder_name in context_folders:
            context_folder_path = os.path.join(user_session_path, context_folder_name)
            context = Context()

            # Load all message files from this context folder
            try:
                message_files = [
                    f for f in os.listdir(context_folder_path) if f.endswith(".json")
                ]
            except OSError:
                continue

            # Sort message files by name (which includes timestamp)
            message_files.sort()

            # Load each message
            for message_file in message_files:
                message_file_path = os.path.join(context_folder_path, message_file)
                with open(message_file_path, "r", encoding="utf-8") as f:
                    message_data = json.load(f)

                    # Extract timestamp from filename (e.g., "1769529115.019496_user.json")
                    timestamp_str = message_file.split("_")[0]
                    ts = datetime.fromtimestamp(float(timestamp_str))

                    # Create Message object and add to context
                    message = Message.from_dict(message_data, message_file_path)
                    context.add_message(ts=ts, message=message)

            # Add context to records
            self.records.append(context)

    def to_gui(self, parent_frame: tk.Frame, user_name: str) -> tk.Frame:
        """
        Docstring for to_gui

        Laytout the user's history of contexts and messages in a GUI frame.
        The frame should allow expanding/collapsing each context to show/hide its messages.
        each message should be represented using its own to_gui method.

        Example layout:

        [v] {user name} History
              [v] Context 1
                    <Message 1 GUI>
                    <Message 1 GUI>
              [>] Context 1

        Explanation:
        - [v] or [>] indicates whether the context is expanded or collapsed.
        - row 0 is always displayed as the header with the user's name and "History".

        :param parent_frame: Description
        :type parent_frame: tk.Frame
        :param user_name: The name of the user
        :type user_name: str
        :return: tk.Frame
        """
        history_frame = tk.Frame(parent_frame)

        expanded_var = tk.BooleanVar(value=False)
        expand_collapse = {
            True: "▼",
            False: "▶",
        }

        def toggle_expand():
            expanded = expanded_var.get()
            expanded_var.set(not expanded)
            collapse_expand_button.config(text=expand_collapse[expanded_var.get()])
            if expanded:
                history_contexts_frame.grid_remove()
            else:
                history_contexts_frame.grid(row=1, column=1, columnspan=2, sticky="w")

        collapse_expand_button = tk.Button(
            history_frame,
            command=toggle_expand,
            text=expand_collapse[expanded_var.get()],
            width=1,
            height=1,
            font=("Terminal", 10),
        )
        collapse_expand_button.grid(row=0, column=0, sticky="w")

        history_label = tk.Label(
            history_frame,
            text=f"{user_name} History ({len(self.records)} contexts)",
            font=("Terminal", 10, "bold"),
        )
        history_label.grid(row=0, column=1, sticky="w")

        history_contexts_frame = tk.Frame(history_frame)
        history_contexts_frame.grid(row=1, column=1, columnspan=2, sticky="w")
        history_contexts_frame.grid_remove()  # Start collapsed

        for idx, context in enumerate(self.records):
            c_frame = context.to_gui(history_contexts_frame)
            c_frame.grid(row=idx, column=0, sticky="w", padx=(20, 0))

        return history_frame
