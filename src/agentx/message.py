import tkinter as tk
from dataclasses import dataclass


@dataclass
class Message:
    """
    Docstring for src.agentx.message
    """

    def __init__(
        self,
        role: str,
        content: str,
        attachments: list[str] = None,
        enabled: bool = True,
        file: str = None,
    ):
        """
        Message

        :param role: The role of the message (e.g., "user", "assistant", "system").
        :param content: The content of the message.
        :param attachments: List of attachment file paths associated with the message.
        :param enabled: Flag indicating if the message is enabled in the context.
        :param file: The file path from which the message was loaded, if applicable.
        """
        self.role = role
        self.content = content
        self.attachments: list[str] = attachments or []
        self._enabled = enabled
        self._file = file

    @classmethod
    def from_dict(cls, data: dict, file_path: str = None) -> "Message":
        """
        Create a Message instance from a dictionary.

        :param data: Dictionary containing message data
        :param file_path: Optional file path to override the one in data
        :return: Message instance
        """
        return cls(
            role=data.get("role", "user"),
            content=data.get("content", ""),
            attachments=data.get("attachments", []),
            enabled=data.get("enabled", True),
            file=file_path or data.get("file"),
        )

    @property
    def enabled(self) -> bool:
        return self._enabled

    @enabled.setter
    def enabled(self, value: bool):
        self._enabled = value

    @property
    def file(self) -> str:
        return self._file

    @file.setter
    def file(self, value: str):
        self._file = value

    def attach(self, attachment_path: str):
        """
        Attach a file to the message.

        :param attachment_path: The file path to attach.
        """
        self.attachments.append(attachment_path)

    def detach(self, attachment_path: str):
        """
        Detach a file from the message.

        :param attachment_path: The file path to detach.
        """
        if attachment_path in self.attachments:
            self.attachments.remove(attachment_path)

    def serialize(self) -> dict:
        """
        serialize

        Use this method to convet the Message object to a dictionary for writing to context JSON file.

        :param self: Description
        :return: Description
        :rtype: dict
        """
        return {
            "role": self.role,
            "content": self.content,
            "enabled": self.enabled,
            "file": self.file,
        }

    def llm_message_dict(self) -> dict:
        """
        Custom JSON serialization that omits the file property.
        """
        mj = {
            "role": self.role,
            "content": self.content,
            "attachments": self.attachments,
        }
        return mj

    def to_gui(self, parent):
        """
        Generate tkinter GUI representation of the message.
        :param parent: The parent widget for the frame.
        :return: tkinter Frame representing the message
        """
        frame = tk.Frame(parent)

        # State for expand/collapse
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
                for w in attachment_widgets:
                    w.grid_remove()
            else:
                for idx, w in enumerate(attachment_widgets):
                    w.grid(
                        row=1 + idx, column=3, columnspan=2, sticky="w", padx=(30, 0)
                    )

        collapse_expand_button = tk.Button(
            frame,
            text=expand_collapse[expanded_var.get()],
            width=1,
            height=1,
            font=("Terminal", 10),
            command=toggle_expand,
        )
        collapse_expand_button.grid(row=0, column=0, sticky="w")

        enabled_var = tk.BooleanVar(value=self.enabled)

        def on_enabled_toggle():
            self.enabled = enabled_var.get()

        enabled_checkbox = tk.Checkbutton(
            frame, variable=enabled_var, command=on_enabled_toggle
        )
        enabled_checkbox.grid(row=0, column=1, sticky="w")

        roles = {
            "user": "👤",
            "assistant": "🤖",
            "system": "⚙️",
        }
        role_label = tk.Label(frame, text=roles.get(self.role, "⚙️"))
        role_label.grid(row=0, column=2, sticky="w")

        # Content preview (first 40 chars)
        preview = self.content[:40] + ("..." if len(self.content) > 40 else "")
        preview_label = tk.Label(frame, text=preview, anchor="w", width=50)
        preview_label.grid(row=0, column=3, sticky="w")

        # Attachments
        attachment_widgets = []
        for idx, att in enumerate(self.attachments):
            att_label = tk.Label(frame, text=f"📁  {att.split('/')[-1]}", anchor="w")
            att_label.grid(
                row=1 + idx, column=3, columnspan=2, sticky="w", padx=(30, 0)
            )
            attachment_widgets.append(att_label)

        return frame
