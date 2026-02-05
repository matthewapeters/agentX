"""
Docstring for agentix.message
"""

from dataclasses import dataclass, field
from typing import Optional


@dataclass
class Message:
    """
    Docstring for Message
    """

    def __init__(self, role: str, content: str, attachments: list = None):
        self.role = role
        self.content = content
        self.attachments: Optional[list] = attachments
        self.filename: Optional[str] = field(
            default=None, init=False, repr=False, compare=False
        )  # Track filename without serialization
        self._exclude_from_context: bool = field(
            default=False, init=False, repr=False, compare=False
        )  # Internal use only

    def exclude_from_context(self):
        """Mark this message to be excluded from context trimming."""
        self._exclude_from_context = True
