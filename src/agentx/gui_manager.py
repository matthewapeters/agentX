"""GUI Manager implementation."""

import tkinter as tk
from tkinter import ttk
from typing import Callable, Optional
from datetime import datetime

from .igui_manager import IGUIManager
from .gui_config import GUIConfig
from .widget_registry import WidgetRegistry
from .attachment_info import AttachmentInfo


class GUIManager:
    """Manages all GUI widgets and presentation logic.
    
    This class implements the IGUIManager interface and handles all
    presentation concerns, completely separated from business logic.
    """
    
    def __init__(
        self,
        root: tk.Tk,
        config: GUIConfig,
        on_submit: Callable[[], None],
        on_interrupt: Callable[[], None],
        on_attachment_toggle: Callable[[str, bool], None]
    ):
        """Initialize GUI manager.
        
        Args:
            root: The tkinter root window
            config: GUI configuration
            on_submit: Callback when user submits input
            on_interrupt: Callback when user interrupts streaming
            on_attachment_toggle: Callback when attachment enabled state changes
                                  Args: (attachment_id, enabled)
        """
        self.root = root
        self.config = config
        self.widgets = WidgetRegistry()
        
        # Store callbacks
        self._on_submit = on_submit
        self._on_interrupt = on_interrupt
        self._on_attachment_toggle = on_attachment_toggle
        
        # Cache for text font
        self._text_font: Optional[tuple] = None
    
    # Lifecycle Methods
    
    def create_layout(self) -> None:
        """Creates and arranges all GUI widgets.
        
        Called once during initialization after the root window is created.
        Sets up the complete widget hierarchy and event bindings.
        """
        # Setup fonts first (caches the result)
        self._setup_fonts()
        
        # Configure window
        self._setup_window_geometry()
        
        # Create main panels
        self._create_output_panel()
        self._create_status_panel()
        self._create_input_panel()
        
        # Configure text styles
        self._configure_text_styles()
    
    def destroy(self) -> None:
        """Cleanup and destroy all widgets.
        
        Called during application shutdown to properly release resources.
        """
        self.widgets.destroy_all()
    
    # Display Methods - Output
    
    def display_user_message(
        self,
        content: str,
        attachments: list[str],
        timestamp: datetime
    ) -> None:
        """Display a user message in the output area.
        
        Args:
            content: The message text content
            attachments: List of attachment filenames (for display only)
            timestamp: When the message was created
        """
        output = self.widgets.output_text
        if output is None:
            return
        
        # Insert message
        output.insert(tk.END, f"User: {content}\n", ("user_prompt",))
        
        # Display attachments
        if attachments:
            for filename in attachments:
                output.insert(
                    tk.END,
                    f"\n[Attached file: {filename}]\n",
                    ("gray",)
                )
        
        # Auto-scroll
        output.see(tk.END)
    
    def display_agent_thinking(self, content: str) -> None:
        """Display agent thinking content (streaming).
        
        Args:
            content: Chunk of thinking text to append
        """
        output = self.widgets.output_text
        if output is None:
            return
        
        # Check if this is the first call (no agent thinking header yet)
        current_content = output.get("1.0", tk.END)
        if "(Agent is thinking..." not in current_content:
            output.insert(tk.END, "(Agent is thinking...)\n\n", ("agent_thinking",))
        
        # Append content
        output.insert(tk.END, content, ("agent_thinking",))
        
        # Auto-scroll
        output.see(tk.END)
    
    def display_agent_response(self, content: str) -> None:
        """Display agent response content (streaming).
        
        Args:
            content: Chunk of response text to append
        """
        output = self.widgets.output_text
        if output is None:
            return
        
        # Check if this is the first call (no agent response header yet)
        current_content = output.get("1.0", tk.END)
        if "\nAgent:\n\n" not in current_content and not current_content.startswith("Agent:\n\n"):
            output.insert(tk.END, "\nAgent:\n\n", ("agent_response",))
        
        # Append content
        output.insert(tk.END, content, ("agent_response",))
        
        # Auto-scroll
        output.see(tk.END)
    
    def display_error(self, message: str) -> None:
        """Display an error message to the user.
        
        Args:
            message: Error message text
        """
        output = self.widgets.output_text
        if output is None:
            return
        
        # Insert error message with emphasis
        output.insert(tk.END, f"\n⚠️  ERROR: {message}\n\n", ("gray",))
        
        # Auto-scroll
        output.see(tk.END)
    
    def display_spacing(self) -> None:
        """Display spacing between conversation segments."""
        output = self.widgets.output_text
        if output is None:
            return
        
        # Insert spacing
        output.insert(tk.END, "\n\n", ("system_space",))
        
        # Auto-scroll
        output.see(tk.END)
    
    # Display Methods - Attachments
    
    def update_attachment_bar(
        self,
        current_attachments: list[AttachmentInfo],
        history_attachments: list[AttachmentInfo]
    ) -> None:
        """Update the attachment display bar above input.
        
        Args:
            current_attachments: Attachments on current message
            history_attachments: Enabled attachments from history
        """
        frame = self.widgets.attachments_frame
        if frame is None:
            return
        
        # Clear existing attachments
        self.widgets.clear_attachments()
        
        # Create widgets for current attachments
        for info in current_attachments:
            widget = self._create_attachment_widget(
                frame,
                info,
                is_history=False
            )
            self.widgets.attachment_labels.append(widget)
        
        # Create widgets for history attachments
        for info in history_attachments:
            widget = self._create_attachment_widget(
                frame,
                info,
                is_history=True
            )
            self.widgets.attachment_labels.append(widget)
    
    # Display Methods - Context/History
    
    def update_context_panel(self, context_widget: tk.Widget) -> None:
        """Replace context panel content with new widget.
        
        Args:
            context_widget: Fully rendered context widget from Context.to_gui()
        """
        # Destroy old widget if it exists
        if self.widgets.system_status_context is not None:
            self.widgets.system_status_context.destroy()
        
        # Pack new widget into session tab
        context_widget.pack(expand=True, fill=tk.BOTH)
        
        # Store reference
        self.widgets.system_status_context = context_widget
    
    def update_history_panel(self, history_widget: tk.Widget) -> None:
        """Replace history panel content with new widget.
        
        Args:
            history_widget: Fully rendered history widget from History.to_gui()
        """
        # Destroy old widget if it exists
        if self.widgets.system_status_history is not None:
            self.widgets.system_status_history.destroy()
        
        # Pack new widget into session tab
        history_widget.pack(expand=True, fill=tk.BOTH)
        
        # Store reference
        self.widgets.system_status_history = history_widget
    
    def update_files_panel(self, files_widget: tk.Widget) -> None:
        """Replace files panel content with new widget.
        
        Args:
            files_widget: Fully rendered file explorer from FileExplorer.to_gui()
        """
        # Destroy old widget if it exists
        if self.widgets.system_status_files is not None:
            self.widgets.system_status_files.destroy()
        
        # Pack new widget into files tab
        files_widget.pack(expand=True, fill=tk.BOTH)
        
        # Store reference
        self.widgets.system_status_files = files_widget
    
    # Input Methods
    
    def get_user_input(self) -> str:
        """Extract and clear user input text.
        
        Returns:
            Stripped input text
        """
        text_widget = self.widgets.user_input_text
        if text_widget is None:
            return ""
        
        # Get text and strip whitespace
        content = text_widget.get("1.0", tk.END).strip()
        
        # Clear the widget
        text_widget.delete("1.0", tk.END)
        
        return content
    
    def clear_user_input(self) -> None:
        """Clear the user input field."""
        text_widget = self.widgets.user_input_text
        if text_widget is not None:
            text_widget.delete("1.0", tk.END)
    
    def set_streaming_state(self, is_streaming: bool) -> None:
        """Update UI for streaming state.
        
        Args:
            is_streaming: True if streaming in progress, False if idle
        """
        submit = self.widgets.user_submit
        interrupt = self.widgets.user_break
        
        if submit is not None:
            submit.config(state=tk.DISABLED if is_streaming else tk.NORMAL)
        
        if interrupt is not None:
            interrupt.config(state=tk.NORMAL if is_streaming else tk.DISABLED)
    
    def set_busy_state(self, is_busy: bool) -> None:
        """Update UI for busy operations (non-streaming).
        
        Args:
            is_busy: True if operation in progress
        """
        # Update cursor
        cursor = "wait" if is_busy else ""
        self.root.config(cursor=cursor)
        
        # Disable/enable input controls
        input_text = self.widgets.user_input_text
        submit = self.widgets.user_submit
        
        if input_text is not None:
            input_text.config(state=tk.DISABLED if is_busy else tk.NORMAL)
        
        if submit is not None:
            submit.config(state=tk.DISABLED if is_busy else tk.NORMAL)
    
    def set_window_title(self, title: str) -> None:
        """Set the window title.
        
        Args:
            title: The new window title
        """
        self.root.title(title)
    
    # Widget Access Methods
    
    def get_root(self) -> tk.Tk:
        """Get the root window.
        
        Returns:
            The tkinter root window
        """
        return self.root
    
    def get_context_parent(self) -> tk.Widget:
        """Get parent widget for context rendering.
        
        Returns:
            The widget to use as parent for context.to_gui()
        """
        if self.widgets.session_tab is None:
            raise RuntimeError("session_tab not yet created")
        return self.widgets.session_tab
    
    def get_history_parent(self) -> tk.Widget:
        """Get parent widget for history rendering.
        
        Returns:
            The widget to use as parent for history.to_gui()
        """
        if self.widgets.session_tab is None:
            raise RuntimeError("session_tab not yet created")
        return self.widgets.session_tab
    
    def get_files_parent(self) -> tk.Widget:
        """Get parent widget for file explorer rendering.
        
        Returns:
            The widget to use as parent for file_explorer.to_gui()
        """
        if self.widgets.files_tab is None:
            raise RuntimeError("files_tab not yet created")
        return self.widgets.files_tab
    
    # Private helper methods
    
    def _setup_fonts(self) -> tuple:
        """Determine and cache text font.
        
        Returns:
            Tuple of (font_name, font_size) to use for text widgets
        """
        # Check if emoji font is available
        text_font = self.config.default_font
        
        # Try to locate the emoji font if path is specified
        if self.config.emoji_font_path:
            import os
            emoji_font_path = self.config.emoji_font_path
            if os.path.exists(emoji_font_path):
                text_font = (emoji_font_path, self.config.default_font[1])
        
        self._text_font = text_font
        return text_font
    
    def _setup_window_geometry(self) -> None:
        """Configure window size and position."""
        # Get screen dimensions
        screen_width = self.root.winfo_screenwidth()
        screen_height = self.root.winfo_screenheight()
        
        # Calculate window dimensions based on ratios from config
        window_width = int(screen_width * self.config.window_width_ratio)
        window_height = int(screen_height * self.config.window_height_ratio)
        
        # Determine screen side (left or right)
        if self.config.screen_side.lower() == "left":
            x_position = 0
        else:  # Default to "right"
            x_position = screen_width - window_width
        
        y_position = 0
        
        # Set window geometry
        self.root.geometry(f"{window_width}x{window_height}+{x_position}+{y_position}")
        self.root.title("AgentX - the Ollama Agent")
    
    def _create_output_panel(self) -> None:
        """Create output display widgets."""
        # Get or setup font
        text_font = self._text_font or self.config.default_font
        
        # Create a PanedWindow for resizable output and system frames
        self.widgets.paned = tk.PanedWindow(
            self.root,
            orient=tk.HORIZONTAL,
            sashrelief=tk.RAISED
        )
        self.widgets.paned.place(
            relx=0.001,
            rely=0.001,
            relwidth=0.99,
            relheight=0.79
        )
        
        # Output display with scrollbar
        self.widgets.output_display = tk.Frame(
            self.widgets.paned,
            bg=self.config.output_bg
        )
        
        # Create a notebook (tabbed interface) for output
        self.widgets.output_notebook = ttk.Notebook(
            self.widgets.output_display
        )
        self.widgets.output_notebook.pack(
            expand=True,
            fill=tk.BOTH,
            padx=0,
            pady=0
        )
        
        # Create Output tab
        self.widgets.output_tab = tk.Frame(
            self.widgets.output_notebook,
            bg=self.config.output_bg
        )
        self.widgets.output_notebook.add(
            self.widgets.output_tab,
            text="Output"
        )
        
        # Create output text and scrollbar in the Output tab
        self.widgets.output_scrollbar = tk.Scrollbar(
            self.widgets.output_tab
        )
        self.widgets.output_text = tk.Text(
            self.widgets.output_tab,
            wrap=tk.WORD,
            font=text_font,
            yscrollcommand=self.widgets.output_scrollbar.set,
        )
        self.widgets.output_scrollbar.config(
            command=self.widgets.output_text.yview
        )
        self.widgets.output_text.pack(
            side=tk.LEFT,
            expand=True,
            fill=tk.BOTH
        )
        self.widgets.output_scrollbar.pack(side=tk.RIGHT, fill=tk.Y)
        
        # Ensure selection highlighting is visible
        self.widgets.output_text.tag_config(
            "sel",
            background="#3399ff",
            foreground="#ffffff"
        )
        
        self.widgets.paned.add(
            self.widgets.output_display,
            stretch="always"
        )
    
    def _create_status_panel(self) -> None:
        """Create status panel with tabs."""
        self.widgets.system_status = tk.Frame(
            self.widgets.paned,
            bg=self.config.status_bg
        )
        
        # Create a notebook (tabbed interface) for system status
        self.widgets.system_notebook = ttk.Notebook(
            self.widgets.system_status
        )
        self.widgets.system_notebook.pack(
            expand=True,
            fill=tk.BOTH,
            padx=0,
            pady=0
        )
        
        # Create Session tab
        self.widgets.session_tab = tk.Frame(
            self.widgets.system_notebook,
            bg=self.config.status_bg
        )
        self.widgets.system_notebook.add(
            self.widgets.session_tab,
            text="Session"
        )
        
        # Create Files tab
        self.widgets.files_tab = tk.Frame(
            self.widgets.system_notebook,
            bg=self.config.status_bg
        )
        self.widgets.system_notebook.add(
            self.widgets.files_tab,
            text="Files"
        )
        
        # Bind tab change event to force widget updates
        def on_tab_changed(event):
            self.root.update_idletasks()
            selected_tab = self.widgets.system_notebook.select()
            if selected_tab:
                self.widgets.system_notebook.nametowidget(
                    selected_tab
                ).update_idletasks()
        
        self.widgets.system_notebook.bind("<<NotebookTabChanged>>", on_tab_changed)
        
        self.widgets.paned.add(
            self.widgets.system_status,
            stretch="always"
        )
        
        # Set the sash position to create a 2:1 split after widgets are rendered
        def set_initial_split():
            self.root.update_idletasks()
            paned_width = self.widgets.paned.winfo_width()
            if paned_width > 1:
                sash_position = int(
                    paned_width * self.config.output_panel_ratio
                )
                self.widgets.paned.sash_place(0, sash_position, 1)
        
        self.root.after(100, set_initial_split)
    
    def _create_input_panel(self) -> None:
        """Create user input area."""
        # Get or setup font
        text_font = self._text_font or self.config.default_font
        enter_emoji_unicode = "^⏎"
        
        # Add a frame for attachments display
        self.widgets.attachments_frame = tk.Frame(self.root, height=2)
        self.widgets.attachments_frame.place(
            relx=0.001,
            rely=0.77,
            relwidth=1.0,
            relheight=0.03
        )
        
        # User input with scrollbar
        self.widgets.user_input = tk.Frame(
            self.root,
            bg=self.config.input_bg
        )
        self.widgets.user_input.place(
            relx=0.001,
            rely=0.80,
            relwidth=1.0,
            relheight=0.2
        )
        
        self.widgets.input_scrollbar = tk.Scrollbar(
            self.widgets.user_input
        )
        self.widgets.user_input_text = tk.Text(
            self.widgets.user_input,
            wrap=tk.WORD,
            font=text_font,
            yscrollcommand=self.widgets.input_scrollbar.set,
        )
        self.widgets.input_scrollbar.config(
            command=self.widgets.user_input_text.yview
        )
        self.widgets.user_input_text.place(
            relx=0,
            rely=0,
            relwidth=0.90,
            relheight=1.0
        )
        self.widgets.input_scrollbar.place(relx=0.90, rely=0, relheight=1.0)
        
        # Submit button
        self.widgets.user_submit = tk.Button(
            self.widgets.user_input,
            text=enter_emoji_unicode,
            command=self._on_submit_clicked,
        )
        self.widgets.user_submit.place(
            relx=0.92,
            rely=0,
            relwidth=0.07,
            relheight=0.25
        )
        
        # Break button below the submit button
        self.widgets.user_break = tk.Button(
            self.widgets.user_input,
            text="❌",
            command=self._on_interrupt_clicked,
            state=tk.DISABLED,
        )
        self.widgets.user_break.place(
            relx=0.92,
            rely=0.26,
            relwidth=0.07,
            relheight=0.25
        )
        
        # Bind Ctrl-Enter to trigger the user_submit button
        self.widgets.user_input_text.bind(
            "<Control-Return>",
            lambda event: self.widgets.user_submit.invoke()
        )
        
        # Bind Ctrl-Space globally to trigger the user_break button
        self.root.bind_all(
            "<Control-space>",
            lambda event: self.widgets.user_break.invoke()
        )
    
    def _configure_text_styles(self) -> None:
        """Configure text widget tags for styling."""
        output = self.widgets.output_text
        if output is None:
            return
        
        # Configure text styling tags
        output.tag_config("gray", font=self.config.gray_text_font)
        output.tag_config("user_prompt", font=self.config.user_prompt_font)
        output.tag_config("agent_response", font=self.config.agent_response_font)
        output.tag_config("agent_thinking", font=self.config.agent_thinking_font)
        output.tag_config("system_space", font=self.config.default_font)
    
    def _create_attachment_widget(
        self,
        parent: tk.Frame,
        info: AttachmentInfo,
        is_history: bool = False
    ) -> tk.Widget:
        """Create a single attachment display widget.
        
        Args:
            parent: Parent frame to pack widget into
            info: AttachmentInfo containing display information
            is_history: Whether this is a history attachment
            
        Returns:
            The created widget
        """
        # Choose styling based on whether it's history
        if is_history:
            bg = self.config.history_attachment_bg
            icon = "📜"
            suffix = " (history)"
        else:
            bg = self.config.attachment_bg
            icon = "📁"
            suffix = ""
        
        # Create container frame
        att_frame = tk.Frame(parent, bg=bg)
        att_frame.pack(side=tk.LEFT, padx=2, pady=2)
        
        # Create checkbox
        var = tk.BooleanVar(value=info.enabled)
        
        def on_toggle(v=var, att_id=info.attachment_id):
            self._on_attachment_toggle(att_id, v.get())
        
        checkbox = tk.Checkbutton(
            att_frame,
            text=f"{icon} {info.display_name}{suffix}",
            variable=var,
            command=on_toggle,
            bg=bg
        )
        checkbox.pack(side=tk.LEFT, padx=5, pady=2)
        
        return att_frame
    
    def _on_submit_clicked(self) -> None:
        """Internal handler for submit button/keyboard."""
        if self._on_submit:
            self._on_submit()
    
    def _on_interrupt_clicked(self) -> None:
        """Internal handler for interrupt button/keyboard."""
        if self._on_interrupt:
            self._on_interrupt()
    
    def _on_attachment_toggle(self, attachment_id: str, enabled: bool) -> None:
        """Internal handler for attachment checkbox.
        
        Args:
            attachment_id: Unique identifier of attachment
            enabled: New enabled state
        """
        if self._on_attachment_toggle:
            self._on_attachment_toggle(attachment_id, enabled)
