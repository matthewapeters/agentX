from dataclasses import dataclass

import tkinter as tk

@dataclass
class Message:
    """
    Docstring for src.agentx.message
    """
    def __init__(self, 
                 role: str, 
                 content: str, 
                 attachments: list[str] = None, 
                 enabled: bool = True, 
                 file: str = None):
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

    def to_json(self) -> dict:
        """
        Custom JSON serialization that omits the file property.
        """
        return {
            "role": self.role,
            "content": self.content,
            "attachments": "",
        }
    

    def _to_gui(self):
        """
        Generage tkkinter GUI representation of the message:

        enabled renders a checkbox showing if the message is enabled
        the role will be represented as either "👤" for userm or "🤖" for assistant or "⚙️" for system
        a short preview of the content will be shown as a label
        if there are attachments,
        the attachments will be represented in the GUI by "📁" followed by the filename

        [ ] ⚙️  You are a helpful assistant... 
                📁  canned prompt1.txt
                📁  canned prompt2.txt
        [ ] 👤  This is the message content...
                📁 filename.txt
                📁 filename.txt
                📁 filename.txt
        [x] 🤖  That is a great question...


        :return: tkinter Frame representing the message
        """
        frame = tk.Frame()
        enabled_var = tk.BooleanVar(value=self.enabled)
        enabled_checkbox = tk.Checkbutton(frame, text="Enabled", variable=enabled_var)
        enabled_checkbox.pack(side=tk.LEFT)

        role: str 
        match self.role:
            case "user":
                role = "👤"
            case "assistant":
                role = "🤖"
            case _:
                role = "⚙️" # default to system

        role_label = tk.Label(frame, text=role)
        role_label.pack(side=tk.LEFT)

        content_text = tk.Text(frame, wrap=tk.WORD, height=4, width=50)
        content_text.insert(tk.END, self.content)
        content_text.pack(side=tk.LEFT)

        return frame