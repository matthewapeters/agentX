import json
import os
import re
import tkinter as tk
from dataclasses import dataclass
from datetime import datetime

from .attachment import Attachment


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
        attachment_paths: list[str] = None,
        enabled: bool = True,
        file: str = None,
        epoch: float = 0.0,
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
        self.attachments: list[Attachment] = attachments or []
        self._id: int | None = None
        self._enabled = enabled
        self._file = file
        self._epoch = epoch

    @classmethod
    def from_dict(cls, data: dict, file_path: str = None) -> "Message":
        """
        Create a Message instance from a dictionary.

        :param data: Dictionary containing message data
        :param file_path: Optional file path to override the one in data
        :return: Message instance
        """
        # Convert attachments from dicts to Attachment objects if needed
        raw_attachments = data.get("attachments", [])
        attachments = []
        for a in raw_attachments:
            if isinstance(a, dict):
                attachments.append(Attachment(**a))
            else:
                attachments.append(a)
        return cls(
            role=data.get("role", "user"),
            content=data.get("content", ""),
            attachments=attachments,
            enabled=data.get("enabled", True),
            file=file_path or data.get("file"),
            epoch=data.get("epoch", 0),
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

    @property
    def ts(self) -> datetime:
        return datetime.fromtimestamp(self._epoch)

    @ts.setter
    def ts(self, value: datetime):
        self._epoch = value.timestamp()

    @file.setter
    def file(self, value: str):
        self._file = value

    def attach(self, attachment_path: str):
        """
        Attach a file to the message.

        :param attachment_path: The file path to attach.
        """
        print(f"Attaching file: {attachment_path}")
        a = Attachment(file_path=attachment_path, content_type="unknown", content="")
        # Read the file and add its content to attachments
        try:
            with open(attachment_path, "r", encoding="utf-8") as f:
                a.content = f.read()
        except Exception as e:
            a.content = f"[Could not read file: {attachment_path}]"
        self.attachments.append(a)

    def detach(self, attachment_path: str):
        """
        Detach a file from the message.

        :param attachment_path: The file path to detach.
        """
        self.attachments = [
            a for a in self.attachments if a.file_path != attachment_path
        ]

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
            "epoch": self._epoch,
            "attachments": [
                {
                    "file_path": a.file_path,
                    "content_type": a.content_type,
                    "enabled": a.enabled,
                    "content": a.content,
                }
                for a in self.attachments
            ],
        }

    def save(self, context_path: str, time_added: datetime) -> None:
        """
        save
        Use this method to save the context object to a JSON file.
        """
        message_file = os.path.join(
            context_path, f"{time_added.timestamp()}_{self.role}.json"
        )
        self.file = message_file
        with open(message_file, "w", encoding="utf-8") as f:
            f.write(json.dumps(self.serialize()))

    def llm_message_dict(self) -> dict:
        """
        Custom JSON serialization that omits the file property.
        """
        return {
            "role": self.role,
            "content": self.content,
            "attachments": [
                {
                    "file_path": a.file_path,
                    "content_type": a.content_type,
                    "enabled": a.enabled,
                    "content": a.content,
                }
                for a in self.attachments
            ],
        }

    def to_gui(self, parent):
        """
        Generate tkinter GUI representation of the message.
        :param parent: The parent widget for the frame.
        :return: tkinter Frame representing the message
        """
        message_frame = tk.Frame(parent)

        has_attachments = bool(self.attachments)
        attachment_widgets:list[tk.Frame] = []
        attachments_frame = tk.Frame(message_frame)

        columns = {
            "collapse_expand": 0,
            "enabled":1,
            "role": 2,
            "content": 3,
        }
        expand_collapse = {
            True: "▼",
            False: "▶",
        }
        roles = {
            "user": "👤",
            "assistant": "🤖",
            "system": "⚙️",
        }

        expanded_var = tk.BooleanVar(value=False)

        def toggle_expand():
            expanded = expanded_var.get()
            expanded_var.set(not expanded)
            collapse_expand_button.config(text=expand_collapse[expanded_var.get()])
            if expanded_var.get():
                # Show attachments
                attachments_frame.grid(row=1, columnspan=columns["role"], sticky="w")
            else:
                # Hide attachments
                attachments_frame.grid_remove()

        collapse_expand_button = tk.Button(
            message_frame,
            text=expand_collapse[expanded_var.get()],
            width=1,
            height=1,
            font=("Terminal", 10),
            command=toggle_expand,
        )
        # Only show collapse/expand if there are attachments
        if has_attachments:
            collapse_expand_button.grid(row=0, column=columns["collapse_expand"], sticky="w")

        enabled_var = tk.BooleanVar(value=self.enabled)

        def on_enabled_toggle():
            self.enabled = enabled_var.get()

        enabled_checkbox = tk.Checkbutton(
            message_frame, variable=enabled_var, command=on_enabled_toggle
        )
        enabled_checkbox.grid(row=0, column=columns["enabled"], sticky="w")

        role_label = tk.Label(message_frame, text=roles.get(self.role, "⚙️"))
        role_label.grid(row=0, column=columns["role"], sticky="w")

        # Content preview (first 40 chars, trimmed, no attachments)
        trimmed_content = self.content.strip()
        lines = [
            line
            for line in trimmed_content.splitlines()
        ]
        preview_text = " ".join([l.strip() for l in lines if l.strip()])
        preview = preview_text[:40] + ("..." if len(preview_text) > 40 else "")
        preview_label = tk.Label(message_frame, text=preview, anchor="w", width=50)

        preview_label.grid(row=0, column=columns["content"], sticky="w")

        if has_attachments: 
            attachments_frame.grid(row=1, column=columns["role"], sticky="w")

            for idx, att in enumerate(self.attachments):
                att_frame = tk.Frame(message_frame)
                enabled_var = tk.BooleanVar(value=att.enabled)

                def toggle(var=enabled_var, a=att):
                    a.enabled = var.get()

                enabled_checkbox = tk.Checkbutton(
                    att_frame, variable=enabled_var, command=toggle
                )
                enabled_checkbox.grid(row=0, column=0, sticky="w")
                att_label = tk.Label(
                    att_frame, text=f"📁  {att.file_path.split('/')[-1]}", anchor="w"
                )
                att_label.grid(row=0, column=1, sticky="w")
                attachment_widgets.append(att_frame)
                for idx, w in enumerate(attachment_widgets):
                    w.grid(
                        in_=attachments_frame,
                        row=1 + idx, 
                        column=columns["role"], 
                        columnspan=2, 
                        sticky="w",
                    )

            # Initial attachment frame visibility: hide if collapsed, show if expanded
            for i_ in range(2):
                toggle_expand()
        return message_frame
