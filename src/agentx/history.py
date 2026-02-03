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

    def __init__(self, user_history_path: str, exclude_session: str = None):
        """
        Docstring for __init__

        :param self: Description
        :param user_session_path: Description
        :type user_session_path: str
        :param exclude_session: Path to the current session folder to exclude from history
        :type exclude_session: str
        """
        self.sessions = []

        # Load the list of contexts from the user session path
        # each folder under the user session path represent a context
        # each file under the context folder represent a message
        # add each context to self.records alphabetically

        # if not os.path.exists(user_history_path):
        #    return

        # Get all folders under user_history_path
        try:
            print("Loading history from:", user_history_path)
            context_folders = [
                d
                for d in os.listdir(user_history_path)
                if os.path.isdir(os.path.join(user_history_path, d))
            ]
        except OSError as e:
            print(
                f"Could not access user history path: {user_history_path}. Error: {e }"
            )
            return

        # Sort alphabetically
        context_folders.sort()
        # print("Found context folders:", context_folders)

        # Load each context
        for context_folder_name in context_folders:
            context_folder_path = os.path.join(
                user_history_path, context_folder_name, "context"
            )
            # Skip the current session folder if specified
            session_folder = os.path.join(user_history_path, context_folder_name)
            if exclude_session and os.path.normpath(session_folder) == os.path.normpath(
                exclude_session
            ):
                print("Skipping current session folder in history:", exclude_session)
                continue

            context = Context()
            context.session_id = context_folder_name
            context.path = context_folder_path

            # Load all message files from this context folder
            try:
                #  print("  Loading context from:", context_folder_path)
                context.load_messages()
            except OSError as e:
                print(
                    f"Could not load messages from context folder: {context_folder_name}. Error: {e}"
                )
                continue

            # Add context to history if it contains messages
            if context.messages:
                # start with contexts collapsed
                context.expanded = False
                self.sessions.append(context)

    def get_enabled_messages(self) -> list:
        """
        Collect all enabled messages from history sessions with their enabled attachments.
        Returns a list of tuples: (timestamp, message)
        """
        enabled_messages = []
        for context in self.sessions:
            for ts, message in context.messages:
                if getattr(message, "enabled", False):
                    # Filter attachments to only include enabled ones
                    if hasattr(message, "attachments"):
                        message.attachments = [
                            a
                            for a in message.attachments
                            if getattr(a, "enabled", False)
                        ]
                    enabled_messages.append((ts, message))
        return enabled_messages

    def to_gui(
        self, parent_frame: tk.Frame, user_name: str, on_attachment_toggle=None
    ) -> tk.Frame:
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

        Only messages and attachments with their checkboxes enabled will be included in the LLM context.

        :param parent_frame: Description
        :type parent_frame: tk.Frame
        :param user_name: The name of the user
        :type user_name: str
        :param on_attachment_toggle: Optional callback when attachment enabled state changes.
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
            text=f"{user_name} History ({len(self.sessions)} contexts)",
            font=("Terminal", 10, "bold"),
        )
        history_label.grid(row=0, column=1, sticky="w")

        history_contexts_frame = tk.Frame(history_frame)
        history_contexts_frame.grid(row=1, column=1, columnspan=2, sticky="w")
        history_contexts_frame.grid_remove()  # Start collapsed

        for idx, context in enumerate(self.sessions):
            c_frame = context.to_gui(
                history_contexts_frame, on_attachment_toggle=on_attachment_toggle
            )
            c_frame.grid(row=idx, column=0, sticky="w", padx=(20, 0))

        return history_frame
