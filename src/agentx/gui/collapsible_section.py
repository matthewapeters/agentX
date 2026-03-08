"""Reusable collapsible section widget for status/system panels."""

import tkinter as tk
from typing import Optional


class CollapsibleSection:
    """Reusable collapsible container with a header and content host."""

    EXPAND_COLLAPSE_ICONS = {True: "▼", False: "▶"}

    def __init__(
        self,
        parent: tk.Widget,
        title: str,
        initial_collapsed: bool = True,
        font: tuple[str, int, str] = ("Terminal", 10, "bold"),
    ):
        self.parent = parent
        self.title = title
        self.expanded = not initial_collapsed

        self.frame = tk.Frame(parent)

        self.header = tk.Frame(self.frame)
        self.header.pack(fill=tk.X)

        self.toggle_button = tk.Button(
            self.header,
            command=self.toggle,
            text=self.EXPAND_COLLAPSE_ICONS[self.expanded],
            width=1,
            height=1,
            font=("Terminal", 10),
        )
        self.toggle_button.pack(side=tk.LEFT, anchor="w")

        self.title_label = tk.Label(
            self.header,
            text=title,
            font=font,
            anchor="w",
        )
        self.title_label.pack(side=tk.LEFT, anchor="w", padx=(4, 0))

        self.content_container = tk.Frame(self.frame)
        if self.expanded:
            self.content_container.pack(fill=tk.BOTH, expand=True)

        self._content_widget: Optional[tk.Widget] = None

    def get_widget(self) -> tk.Widget:
        """Return the section root widget for layout management."""
        return self.frame

    def toggle(self) -> None:
        """Toggle collapsed/expanded state while retaining content widget."""
        self.expanded = not self.expanded
        self.toggle_button.config(text=self.EXPAND_COLLAPSE_ICONS[self.expanded])

        if self.expanded:
            self.content_container.pack(fill=tk.BOTH, expand=True)
        else:
            self.content_container.pack_forget()

    def set_content(self, widget: tk.Widget, fill: str = tk.BOTH, expand: bool = True) -> None:
        """Replace section content with provided widget."""
        if self._content_widget is not None:
            self._content_widget.destroy()

        self._content_widget = widget
        self._content_widget.pack(fill=fill, expand=expand)

    def set_title(self, title: str) -> None:
        """Update the section header label text."""
        self.title = title
        self.title_label.config(text=title)

    def clear_content(self) -> None:
        """Clear current content while preserving section state."""
        if self._content_widget is not None:
            self._content_widget.destroy()
            self._content_widget = None

    def is_expanded(self) -> bool:
        """Get expanded state."""
        return self.expanded
