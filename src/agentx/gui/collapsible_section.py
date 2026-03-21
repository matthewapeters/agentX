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
        bg: str | None = None,
        fg: str | None = None,
    ):
        self.parent = parent
        self.title = title
        self.expanded = not initial_collapsed

        frame_kwargs = {"bg": bg} if bg else {}
        self.frame = tk.Frame(parent, **frame_kwargs)

        self.header = tk.Frame(self.frame, **frame_kwargs)
        self.header.pack(fill=tk.X)

        button_kwargs = {}
        if bg:
            button_kwargs["bg"] = bg
            button_kwargs["activebackground"] = bg
        if fg:
            button_kwargs["fg"] = fg
            button_kwargs["activeforeground"] = fg

        self.toggle_button = tk.Button(
            self.header,
            command=self.toggle,
            text=self.EXPAND_COLLAPSE_ICONS[self.expanded],
            width=1,
            height=1,
            font=("Terminal", 10),
            **button_kwargs,
        )
        self.toggle_button.pack(side=tk.LEFT, anchor="w")

        label_kwargs = {}
        if bg:
            label_kwargs["bg"] = bg
        if fg:
            label_kwargs["fg"] = fg

        self.title_label = tk.Label(
            self.header,
            text=title,
            font=font,
            anchor="w",
            **label_kwargs,
        )
        self.title_label.pack(side=tk.LEFT, anchor="w", padx=(4, 0))

        self.content_container = tk.Frame(self.frame, **frame_kwargs)
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
