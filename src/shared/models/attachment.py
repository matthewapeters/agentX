"""
Unified Attachment model for AgentX and Agentix.

Attachments represent files attached to messages in the conversation context.
They are stored client-side as part of the session.
"""

import os
from dataclasses import dataclass, field
from typing import Optional


@dataclass
class Attachment:
    """
    Represents a file attachment associated with a message.

    Attachments are stored client-side in the AgentX session folder.
    When sending context to Agentix server, attachment content is included
    in the request payload.

    Attributes:
        file_path: Original path to the attached file
        content_type: MIME type or general type (e.g., "text/plain", "code/python")
        content: The actual content of the file
        enabled: Whether this attachment is included in LLM context
    """

    file_path: str
    content_type: str = "text/plain"
    content: str = ""
    enabled: bool = True
    mime_type: Optional[str] = None

    def __post_init__(self):
        if self.mime_type and self.content_type == "text/plain":
            self.content_type = self.mime_type

    @property
    def filename(self) -> str:
        """Extract filename from path."""
        return os.path.basename(self.file_path)

    @property
    def extension(self) -> str:
        """Extract file extension."""
        _, ext = os.path.splitext(self.file_path)
        return ext.lstrip(".")

    def to_dict(self) -> dict:
        """Serialize attachment for storage or transmission."""
        return {
            "file_path": self.file_path,
            "content_type": self.content_type,
            "mime_type": self.content_type,
            "content": self.content,
            "enabled": self.enabled,
        }

    @classmethod
    def from_dict(cls, data: dict) -> "Attachment":
        """Create Attachment from dictionary."""
        content_type = data.get("content_type", data.get("mime_type", "text/plain"))
        return cls(
            file_path=data.get("file_path", ""),
            content_type=content_type,
            content=data.get("content", ""),
            enabled=data.get("enabled", True),
            mime_type=data.get("mime_type"),
        )

    @classmethod
    def from_file(cls, file_path: str, enabled: bool = True) -> "Attachment":
        """
        Create Attachment by reading from a file.

        Args:
            file_path: Path to the file to attach
            enabled: Whether attachment is enabled by default

        Returns:
            Attachment with content loaded from file
        """
        content = ""
        content_type = cls._detect_content_type(file_path)

        try:
            with open(file_path, "r", encoding="utf-8") as f:
                content = f.read()
        except UnicodeDecodeError:
            content = f"[Binary file: {file_path}]"
        except FileNotFoundError:
            content = f"[File not found: {file_path}]"
        except Exception as e:
            content = f"[Could not read file: {file_path}] - {e}"

        return cls(
            file_path=file_path,
            content_type=content_type,
            content=content,
            enabled=enabled,
        )

    @staticmethod
    def _detect_content_type(file_path: str) -> str:
        """Detect content type based on file extension."""
        ext = os.path.splitext(file_path)[1].lower()

        type_map = {
            ".py": "code/python",
            ".js": "code/javascript",
            ".ts": "code/typescript",
            ".json": "application/json",
            ".yaml": "application/yaml",
            ".yml": "application/yaml",
            ".md": "text/markdown",
            ".txt": "text/plain",
            ".html": "text/html",
            ".css": "text/css",
            ".sql": "code/sql",
            ".sh": "code/shell",
            ".bash": "code/shell",
            ".toml": "application/toml",
            ".xml": "application/xml",
            ".csv": "text/csv",
        }

        return type_map.get(ext, "text/plain")

    def to_llm_format(self) -> str:
        """Format attachment content for inclusion in LLM prompt."""
        return f"\n[File: {self.filename}]\n```{self.extension}\n{self.content}\n```\n"
