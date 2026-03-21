"""Widget registry for centralized widget management."""

import tkinter as tk
from tkinter import ttk
from typing import Optional


class WidgetRegistry:
    """Manages widget references and lifecycle.

    This class serves as the single source of truth for all widget
    references in the GUI. It enables centralized cleanup and provides
    type-safe access to widgets throughout the GUIManager.
    """

    def __init__(self):
        """Initialize the widget registry."""
        # Main structure widgets
        self.root: Optional[tk.Tk] = None
        self.paned: Optional[tk.PanedWindow] = None

        # Output panel widgets
        self.output_display: Optional[tk.Frame] = None
        self.output_notebook: Optional[ttk.Notebook] = None
        self.output_tab: Optional[tk.Frame] = None
        self.output_entries_container: Optional[tk.Frame] = None
        self.output_entries_canvas: Optional[tk.Canvas] = None
        self.output_entries_scrollbar: Optional[tk.Scrollbar] = None
        self.output_entries_frame: Optional[tk.Frame] = None
        self.output_text: Optional[tk.Text] = None
        self.output_scrollbar: Optional[tk.Scrollbar] = None

        # Status panel widgets
        self.system_status: Optional[tk.Frame] = None
        self.system_notebook: Optional[ttk.Notebook] = None
        self.session_tab: Optional[tk.Frame] = None
        self.files_tab: Optional[tk.Frame] = None

        # Dynamic content widgets (replaced during updates)
        self.system_status_history: Optional[tk.Widget] = None
        self.system_status_context: Optional[tk.Widget] = None
        self.system_status_files: Optional[tk.Widget] = None

        # Input panel widgets
        self.attachments_frame: Optional[tk.Frame] = None
        self.attachment_labels: list = []
        self.user_input: Optional[tk.Frame] = None
        self.user_input_text: Optional[tk.Text] = None
        self.input_scrollbar: Optional[tk.Scrollbar] = None
        self.user_submit: Optional[tk.Button] = None
        self.user_break: Optional[tk.Button] = None

    def clear_attachments(self) -> None:
        """Destroy all attachment label widgets.

        Called before updating the attachment display to clean up
        old widgets before creating new ones.
        """
        for widget in self.attachment_labels:
            widget.destroy()
        self.attachment_labels.clear()

    def destroy_all(self) -> None:
        """Destroy all managed widgets.

        Called during cleanup to ensure all widgets are properly
        destroyed and resources are released.
        """
        # Destroy widgets in reverse creation order to respect dependencies

        # Clear attachment widgets first
        self.clear_attachments()

        # Destroy input panel widgets
        if self.user_break is not None:
            self.user_break.destroy()
        if self.user_submit is not None:
            self.user_submit.destroy()
        if self.input_scrollbar is not None:
            self.input_scrollbar.destroy()
        if self.user_input_text is not None:
            self.user_input_text.destroy()
        if self.user_input is not None:
            self.user_input.destroy()
        if self.attachments_frame is not None:
            self.attachments_frame.destroy()

        # Destroy dynamic content widgets
        if self.system_status_files is not None:
            self.system_status_files.destroy()
        if self.system_status_context is not None:
            self.system_status_context.destroy()
        if self.system_status_history is not None:
            self.system_status_history.destroy()

        # Destroy status panel widgets
        if self.files_tab is not None:
            self.files_tab.destroy()
        if self.session_tab is not None:
            self.session_tab.destroy()
        if self.system_notebook is not None:
            self.system_notebook.destroy()
        if self.system_status is not None:
            self.system_status.destroy()

        # Destroy output panel widgets
        if self.output_scrollbar is not None:
            self.output_scrollbar.destroy()
        if self.output_text is not None:
            self.output_text.destroy()
        if self.output_entries_scrollbar is not None:
            self.output_entries_scrollbar.destroy()
        if self.output_entries_canvas is not None:
            self.output_entries_canvas.destroy()
        if self.output_entries_frame is not None:
            self.output_entries_frame.destroy()
        if self.output_entries_container is not None:
            self.output_entries_container.destroy()
        if self.output_tab is not None:
            self.output_tab.destroy()
        if self.output_notebook is not None:
            self.output_notebook.destroy()
        if self.output_display is not None:
            self.output_display.destroy()

        # Destroy main structure widgets
        if self.paned is not None:
            self.paned.destroy()
        if self.root is not None:
            self.root.destroy()
