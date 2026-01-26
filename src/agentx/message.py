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

        Example:

        ▼ [ ] ⚙️  You are a helpful assistant...    # Enambled system prompt, expanded
                  📁  canned prompt1.txt            
                  📁  canned prompt2.txt
        ▼ [ ] 👤  This is the message content...    # Enabled user prompt, expanded
                  📁 filename.txt
                  📁 filename.txt
                  📁 filename.txt
        ▶ [x] 🤖  That is a great question...       # Disabled agent response, collapsed


        Explanataon: 
        Each message is a single frame with a 4x1+n grid layout.
        The first row contains:
        - A toggle button (▼ or ▶) to expand/collapse the message content (column 0)
        - A checkbox to enable/disable the message in the context (column 1)
        - A label showing the role icon (👤, 🤖, ⚙️) (column 2)
        - a label showing a preview of the message content (column 3)
        - Additional rows (n) for each attachment showing a "📁" icon and the filename of the attachments


        :return: tkinter Frame representing the message
        """
        frame = tk.Frame()

        # State for expand/collapse
        expanded_var = tk.BooleanVar(value=True)

        def toggle_expand():
            expanded = expanded_var.get()
            if expanded:
                collapse_expand_button.config(text="▶")
                # Hide attachments and content
                for w in attachment_widgets:
                    w.grid_remove()
            else:
                collapse_expand_button.config(text="▼")
                for idx, w in enumerate(attachment_widgets):
                    w.grid(row=1+idx, column=3, columnspan=2, sticky="w", padx=(30,0))
            expanded_var.set(not expanded)

        collapse_expand_button = tk.Button(
            frame, text="▼", width=2, command=toggle_expand
        )
        collapse_expand_button.grid(row=0, column=0, sticky="w")

        enabled_var = tk.BooleanVar(value=self.enabled)
        enabled_checkbox = tk.Checkbutton(frame, variable=enabled_var)
        enabled_checkbox.grid(row=0, column=1, sticky="w")

        role: str
        match self.role:
            case "user":
                role = "👤"
            case "assistant":
                role = "🤖"
            case _:
                role = "⚙️"  # default to system
        role_label = tk.Label(frame, text=role)
        role_label.grid(row=0, column=2, sticky="w")

        # Content preview (first 40 chars)
        preview = self.content[:40] + ("..." if len(self.content) > 40 else "")
        preview_label = tk.Label(frame, text=preview, anchor="w", width=50)
        preview_label.grid(row=0, column=3, sticky="w")

        # Attachments
        attachment_widgets = []
        for idx, att in enumerate(self.attachments):
            att_label = tk.Label(frame, text=f"📁  {att.split('/')[-1]}", anchor="w")
            att_label.grid(row=1+idx, column=3, columnspan=2, sticky="w", padx=(30,0))
            attachment_widgets.append(att_label)

        # Start collapsed state (all widgets hidden) to respect screen space
        expanded_var.set(False)

        return frame