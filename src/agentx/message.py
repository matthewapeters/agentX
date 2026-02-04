import json
import os
import re
import tkinter as tk
from dataclasses import dataclass
from datetime import datetime

from .attachment import Attachment

USER = "user"
ASSISTANT = "assistant"
SYSTEM = "system"

EXPAND_COLLAPSE_ICON = {
    True: "▼",
    False: "▶",
}

ROLES = {
    USER: "👤",
    ASSISTANT: "🤖",
    SYSTEM: "⚙️",
}

COLLAPSE_EXPAND_BUTTON = "collapse_expand"
ENABLED = "enabled"
ROLE = "role"
CONTENT = "content"

# Define columns for layout clarity
COLUMNS = {
    COLLAPSE_EXPAND_BUTTON: 0,
    ENABLED: 1,
    ROLE: 2,
    CONTENT: 3,
}


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
        if self.file:
            self.save(self.file, self.ts)
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
        if self.file:
            self.save(self.file, self.ts)

    def detach(self, attachment_path: str):
        """
        Detach a file from the message.

        :param attachment_path: The file path to detach.
        """
        self.attachments = [
            a for a in self.attachments if a.file_path != attachment_path
        ]
        if self.file:
            self.save(self.file, self.ts)

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
        if not self._epoch:
            if time_added is None:
                self.ts = datetime.now()
            else:
                self.ts = time_added
        if self.file is None:
            message_file = os.path.join(
                context_path, f"{self.ts.timestamp()}_{self.role}.json"
            )
            self.file = message_file
        with open(self.file, "w", encoding="utf-8") as f:
            f.write(json.dumps(self.serialize()))

    def llm_message_dict(self) -> dict:
        """
        Build the message dict for the LLM API.
        Includes enabled attachment content inline in the content field,
        since Ollama expects content in the 'content' field only.
        """
        # Build content with enabled attachments included
        full_content = self.content
        enabled_attachments = [a for a in self.attachments if a.enabled]
        if enabled_attachments:
            attachment_text = "\n\n--- Attached Files ---\n"
            for a in enabled_attachments:
                filename = a.file_path.split("/")[-1]
                attachment_text += f"\n[File: {filename}]\n```\n{a.content}\n```\n"
            full_content = full_content + attachment_text

        return {
            "role": self.role,
            "content": full_content,
        }
