"""Attachment display information DTO."""

import os
from dataclasses import dataclass


@dataclass
class AttachmentInfo:
    """Information needed to display an attachment in GUI.

    This is a data transfer object that encapsulates the information
    needed to render an attachment in the GUI. It's created from
    Attachment objects to separate business logic from presentation.
    """

    file_path: str
    display_name: str
    enabled: bool
    is_from_history: bool
    attachment_id: str

    @classmethod
    def from_attachment(
        cls, attachment, is_from_history: bool = False
    ) -> "AttachmentInfo":
        """Create from Attachment object.

        Args:
            attachment: An Attachment object from the business layer
            is_from_history: Whether this attachment is from history

        Returns:
            AttachmentInfo instance with display information
        """
        return cls(
            file_path=attachment.file_path,
            display_name=os.path.basename(attachment.file_path),
            enabled=attachment.enabled,
            is_from_history=is_from_history,
            attachment_id=str(id(attachment)),
        )
