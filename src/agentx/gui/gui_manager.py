"""GUI Manager implementation."""

import os
import re
import threading
import tkinter as tk
from datetime import datetime
from tkinter import ttk
from typing import Callable, Optional

from ..attachment_info import AttachmentInfo
from .gui_config import GUIConfig
from ..history import History
from ..igui_manager import IGUIManager
from ..widget_registry import WidgetRegistry
from ..integration import ModelSelector


class GUIManager(IGUIManager):
    """Manages all GUI widgets and presentation logic.

    This class implements the IGUIManager interface and handles all
    presentation concerns, completely separated from business logic.
    """

    # Unified color palette
    COLOR_BG = "#222222"
    COLOR_STATUS_BG = "#333333"
    COLOR_OUTPUT_BG = "#222222"
    COLOR_INPUT_BG = "#222222"
    COLOR_SCROLLBAR = "#444444"
    COLOR_SELECTION_BG = "#3399ff"
    COLOR_SELECTION_FG = "#ffffff"
    COLOR_ERROR = "#ff4444"
    COLOR_ATTACHMENT_BG = "#444444"
    COLOR_ATTACHMENT_HISTORY_BG = "#555555"
    COLOR_ATTACHMENT_TEXT = "#eeeeee"
    COLOR_USER_PROMPT = "#eeeeee"
    COLOR_AGENT_RESPONSE = "#eeeeee"
    COLOR_AGENT_THINKING = "#cccccc"
    COLOR_SYSTEM_SPACE = "#888888"

    def __init__(
        self,
        root: tk.Tk,
        config: GUIConfig,
        on_submit: Callable[[], None],
        on_interrupt: Callable[[], None],
        on_attachment_toggle: Callable[[str, bool], None],
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

        # Widget components (initialized in create_layout)
        self.model_selector: Optional[ModelSelector] = None
        
        # Tool panel state (inlined implementation)
        self._tool_panel_frame: Optional[tk.Frame] = None
        self._tool_panel_tools_container: Optional[tk.Frame] = None
        self._tool_panel_vars: dict = {}
        self._tool_panel_expanded: bool = True
        self._tool_panel_tools: Optional[list] = None

        # Cache for text font
        self._text_font: Optional[tuple] = None

        # Cache for thread-safe input access
        self._cached_user_input: str = ""

        # Streaming label state
        self._agent_thinking_started = False
        self._agent_response_started = False

    # Layout constants for UI rendering
    EXPAND_COLLAPSE_ICONS = {True: "▼", False: "▶"}
    MESSAGE_ROLES = {
        "user": "👤",
        "assistant": "🤖",
        "system": "⚙️",
        "thinking": "💭",
        "tool_call": "🔧",
        "tool_result": "📋",
        "tools": "🛠️",
    }
    MESSAGE_COLUMNS = {
        "exp_button": 0,
        "enabled": 1,
        "role": 2,
        "content": 3,
    }

    def collapse_expand_button(
        self,
        parent: tk.Widget,
        expandable_frame: tk.Widget = None,
        attachment_rows=None,
    ) -> tk.Button:
        """Create a collapse/expand button.

        Args:
            parent: The parent tkinter widget

        Returns:
            A tkinter Button configured as a collapse/expand toggle
        """
        if attachment_rows is None and expandable_frame is None:
            raise ValueError("Either expandable_frame or row_widgets must be provided")

        expanded_var = tk.BooleanVar(value=False)

        collapse_expand_button: tk.Button

        def toggle_expand():
            expanded = expanded_var.get()
            expanded_var.set(not expanded)
            collapse_expand_button.config(
                text=self.EXPAND_COLLAPSE_ICONS[expanded_var.get()]
            )
            if expandable_frame:
                if expanded:
                    expandable_frame.grid_remove()
                else:
                    # widget that will be expanded/collapsed indented by one column
                    expandable_frame.grid(
                        row=1, column=self.MESSAGE_COLUMNS["enabled"], sticky="w"
                    )
            if attachment_rows:
                for row_widgets in attachment_rows:
                    for widget in row_widgets:
                        if expanded_var.get():
                            widget.grid()
                        else:
                            widget.grid_remove()

        collapse_expand_button = tk.Button(
            parent,
            command=toggle_expand,
            text=self.EXPAND_COLLAPSE_ICONS[expanded_var.get()],
            width=1,
            height=1,
            font=("Terminal", 10),
        )
        return collapse_expand_button

    def render_history_widget(
        self, history_obj: History, parent, user_name, on_attachment_toggle=None
    ):
        """
        Render a History object as a tkinter widget (Frame), replicating History.to_gui logic.
        Args:
            history_obj: The History instance to render
            parent: The parent tkinter widget
            user_name: The name of the user (for label)
            on_attachment_toggle: Optional callback for attachment toggles
        Returns:
            tkinter Frame representing the history
        """
        history_frame = tk.Frame(parent)
        history_contexts_frame = tk.Frame(history_frame)
        collapse_expand_button = self.collapse_expand_button(
            history_frame, history_contexts_frame
        )
        history_label = tk.Label(
            history_frame,
            text=f"{user_name} History ({len(history_obj.sessions)} contexts)",
            font=("Terminal", 10, "bold"),
        )

        collapse_expand_button.grid(
            row=0, column=self.MESSAGE_COLUMNS["exp_button"], sticky="w"
        )
        history_label.grid(row=0, column=self.MESSAGE_COLUMNS["enabled"], sticky="w")

        # context widgets indented by one column
        # columns: | indent | context widgets... |
        history_contexts_frame.grid(
            row=1, column=self.MESSAGE_COLUMNS["exp_button"], sticky="nsew"
        )
        for idx, context in enumerate(history_obj.sessions):
            # Ensure each context is collapsed by default when rendering history
            context.expanded = False
            c_frame = self.render_context_widget(
                context,
                history_contexts_frame,
                on_attachment_toggle=on_attachment_toggle,
            )
            # Stack tightly, no vertical padding, align to top
            # column 0 of history_contexts_frame which is in history_frame column 1 (indent)
            c_frame.grid(
                row=idx, column=self.MESSAGE_COLUMNS["exp_button"], sticky="nsew"
            )

        history_contexts_frame.grid_remove()  # Start collapsed
        return history_frame

    def render_context_widget(self, context_obj, parent, on_attachment_toggle=None):
        """
        Render a Context object as a tkinter widget (Frame), replicating Context.to_gui logic.
        Args:
            context_obj: The Context instance to render
            parent: The parent tkinter widget
            on_attachment_toggle: Optional callback for attachment toggles
        Returns:
            tkinter Frame representing the context
        """
        context_frame = tk.Frame(parent)
        context_messages_frame = tk.Frame(context_frame)

        # Create a sub-frame for expand/collapse button and label, so label is always left-aligned
        # header_frame = tk.Frame(context_frame)
        collapse_expand_button = self.collapse_expand_button(
            context_frame, context_messages_frame
        )
        context_label = tk.Label(
            # header_frame,
            context_frame,
            text=(
                f"{getattr(context_obj, 'session_id', None) or 'Context'} "
                f"({len(context_obj.messages)} messages)"
            ),
            font=("Terminal", 10, "bold"),
        )
        collapse_expand_button.grid(
            row=0, column=self.MESSAGE_COLUMNS["exp_button"], sticky="w"
        )
        context_label.grid(row=0, column=1, sticky="w")
        context_messages_frame.grid(
            row=1, column=self.MESSAGE_COLUMNS["enabled"], sticky="nsew"
        )

        # Configure column 0 as indent, then message columns
        context_messages_frame.columnconfigure(
            self.MESSAGE_COLUMNS["exp_button"], weight=0
        )
        context_messages_frame.columnconfigure(
            self.MESSAGE_COLUMNS["enabled"], weight=0
        )
        context_messages_frame.columnconfigure(self.MESSAGE_COLUMNS["role"], weight=0)
        context_messages_frame.columnconfigure(
            self.MESSAGE_COLUMNS["content"], weight=1
        )

        # Render messages directly into the frame's grid
        current_row = 0
        for message in context_obj.messages:
            current_row = self._render_message_to_grid(
                message, context_messages_frame, current_row, on_attachment_toggle
            )

        # Hide messages frame if not expanded on initial render
        if not getattr(context_obj, "expanded", False):
            context_messages_frame.grid_remove()

        return context_frame

    def _render_message_to_grid(
        self,
        message_obj,
        parent_frame: tk.Frame,
        start_row: int,
        on_attachment_toggle=None,
    ) -> int:
        """
        Render a Message object directly into the parent frame's grid.

        This method places message components (checkbox, role, content, attachments)
        directly into the parent frame's grid system using MESSAGE_COLUMNS for alignment.
        This ensures consistent column widths across all messages.

        Args:
            message_obj: The Message instance to render
            parent_frame: The parent tkinter Frame with grid layout
            start_row: The starting row number in the parent grid
            on_attachment_toggle: Optional callback for attachment toggles

        Returns:
            The next available row number after this message and its attachments
        """
        current_row = start_row

        has_attachments = bool(getattr(message_obj, "attachments", []))

        # Track widgets for show/hide on expand/collapse
        attachment_rows: list[list[tk.Widget]] = []

        # Forward declare for use in toggle_expand closure
        collapse_expand_button: tk.Button

        # Column 1: Collapse/Expand button (or empty space), indented by one column
        if has_attachments:
            collapse_expand_button = self.collapse_expand_button(
                parent=parent_frame, attachment_rows=attachment_rows
            )
            collapse_expand_button.grid(
                row=current_row,
                column=self.MESSAGE_COLUMNS["exp_button"],
                sticky="nsew",
            )
        else:
            empty_label = tk.Label(parent_frame, width=2)
            empty_label.grid(
                row=current_row,
                column=self.MESSAGE_COLUMNS["exp_button"],
                sticky="nsew",
            )

        # Column 1: Enabled checkbox
        enabled_var = tk.BooleanVar(value=getattr(message_obj, "enabled", True))

        def on_enabled_toggle():
            message_obj.enabled = enabled_var.get()

        enabled_checkbox = tk.Checkbutton(
            parent_frame, variable=enabled_var, command=on_enabled_toggle
        )
        enabled_checkbox.grid(
            row=current_row, column=self.MESSAGE_COLUMNS["enabled"], sticky="nsew"
        )

        # Column 2: Role icon
        role_value = getattr(message_obj, "role", "system")
        role_key = role_value.value if hasattr(role_value, "value") else role_value
        role_label = tk.Label(
            parent_frame,
            text=self.MESSAGE_ROLES.get(role_key, "⚙️"),
        )
        role_label.grid(
            row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nsew"
        )

        # Column 3: Content preview
        trimmed_content = getattr(message_obj, "content", "").strip()
        lines = [
            line
            for line in trimmed_content.splitlines()
            if not re.match(r"--- \[Attached file: .+\] ---", line)
            and not re.match(r"--- \[End of .+\] ---", line)
        ]
        preview_text = " ".join([l.strip() for l in lines if l.strip()])
        preview = preview_text[:40] + ("..." if len(preview_text) > 40 else "")
        preview_label = tk.Label(parent_frame, text=preview, anchor="w", width=50)
        preview_label.grid(
            row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew"
        )

        current_row += 1

        # Render attachments directly in the parent grid
        if has_attachments:
            for att in getattr(message_obj, "attachments", []):
                row_widgets: list[tk.Widget] = []

                # Column 2: Attachment enabled checkbox (in role column for visual indentation)
                att_enabled_var = tk.BooleanVar(value=getattr(att, "enabled", True))

                def toggle(var=att_enabled_var, a=att, callback=on_attachment_toggle):
                    a.enabled = var.get()
                    if callback:
                        callback(a, a.enabled)

                att_checkbox = tk.Checkbutton(
                    parent_frame, variable=att_enabled_var, command=toggle
                )
                att_checkbox.grid(
                    row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nsew"
                )
                row_widgets.append(att_checkbox)

                # Column 3: Attachment label
                att_label = tk.Label(
                    parent_frame,
                    text=f"📁  {getattr(att, 'file_path', '').split('/')[-1]}",
                    anchor="w",
                )
                att_label.grid(
                    row=current_row,
                    column=self.MESSAGE_COLUMNS["content"],
                    sticky="nsew",
                )
                row_widgets.append(att_label)

                # Start with attachments hidden
                for widget in row_widgets:
                    widget.grid_remove()

                attachment_rows.append(row_widgets)
                current_row += 1

        return current_row

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
        self, content: str, attachments: list[str], timestamp: datetime
    ) -> None:
        """Display a user message in the output area.

        Args:
            content: The message text content
            attachments: List of attachment filenames (for display only)
            timestamp: When the message was created
        """
        if threading.current_thread() is not threading.main_thread():
            return

        output = self.widgets.output_text
        if output is None:
            return

        # Insert message with role emoji
        output.insert(
            tk.END, f"{self.MESSAGE_ROLES['user']} User: {content}\n", ("user_prompt",)
        )

        # Reset agent display state for a new turn
        self._agent_thinking_started = False
        self._agent_response_started = False

        # Display attachments
        if attachments:
            for filename in attachments:
                output.insert(tk.END, f"\n[Attached file: {filename}]\n", ("gray",))

        # Auto-scroll
        output.see(tk.END)

    def display_agent_thinking(self, content: str) -> None:
        """Display agent thinking content (streaming).

        Args:
            content: Chunk of thinking text to append
        """
        if threading.current_thread() is not threading.main_thread():
            return

        output = self.widgets.output_text
        if output is None:
            return

        if not self._agent_thinking_started:
            output.insert(tk.END, "Agent is thinking: ", ("agent_thinking",))
            self._agent_thinking_started = True

        # Append content
        output.insert(tk.END, content, ("agent_thinking",))

        # Auto-scroll
        output.see(tk.END)

    def display_agent_response(self, content: str) -> None:
        """Display agent response content (streaming).

        Args:
            content: Chunk of response text to append
        """
        if threading.current_thread() is not threading.main_thread():
            return

        output = self.widgets.output_text
        if output is None:
            return
        if not self._agent_response_started:
            output.insert(tk.END, "Agent: ", ("agent_response",))
            self._agent_response_started = True

        # Append content
        output.insert(tk.END, content, ("agent_response",))

        # Auto-scroll
        output.see(tk.END)

    def display_error(self, message: str) -> None:
        """Display an error message to the user.

        Args:
            message: Error message text
        """
        if threading.current_thread() is not threading.main_thread():
            return

        output = self.widgets.output_text
        if output is None:
            return

        # Insert error message with emphasis
        output.insert(tk.END, f"\n⚠️  ERROR: {message}\n\n", ("gray",))

        # Auto-scroll
        output.see(tk.END)

    def display_spacing(self) -> None:
        """Display spacing between conversation segments."""
        if threading.current_thread() is not threading.main_thread():
            return

        output = self.widgets.output_text
        if output is None:
            return

        # Insert spacing
        output.insert(tk.END, "\n\n", ("system_space",))

        # Reset agent display state for next turn
        self._agent_thinking_started = False
        self._agent_response_started = False

        # Auto-scroll
        output.see(tk.END)

    # Display Methods - Attachments

    def update_attachment_bar(
        self,
        current_attachments: list[AttachmentInfo],
        history_attachments: list[AttachmentInfo],
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
            widget = self._create_attachment_widget(frame, info, is_history=False)
            self.widgets.attachment_labels.append(widget)

        # Create widgets for history attachments
        for info in history_attachments:
            widget = self._create_attachment_widget(frame, info, is_history=True)
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
        history_widget.pack(expand=False, fill=tk.X)

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
        if threading.current_thread() is not threading.main_thread():
            return self._cached_user_input

        try:
            content = text_widget.get("1.0", tk.END).strip()
            self._cached_user_input = content
            text_widget.delete("1.0", tk.END)
            return content
        except RuntimeError:
            return self._cached_user_input

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
        def _apply():
            submit = self.widgets.user_submit
            interrupt = self.widgets.user_break

            if submit is not None:
                submit.config(state=tk.DISABLED if is_streaming else tk.NORMAL)

            if interrupt is not None:
                interrupt.config(state=tk.NORMAL if is_streaming else tk.DISABLED)

        if threading.current_thread() is not threading.main_thread():
            return
        try:
            _apply()
        except RuntimeError:
            pass

    def set_busy_state(self, is_busy: bool) -> None:
        """Update UI for busy operations (non-streaming).

        Args:
            is_busy: True if operation in progress
        """
        def _apply():
            # Update cursor
            cursor = "watch" if is_busy else ""
            try:
                self.root.config(cursor=cursor)
            except tk.TclError:
                self.root.config(cursor="")

            # Disable/enable input controls
            input_text = self.widgets.user_input_text
            submit = self.widgets.user_submit

            if input_text is not None:
                input_text.config(state=tk.DISABLED if is_busy else tk.NORMAL)

            if submit is not None:
                submit.config(state=tk.DISABLED if is_busy else tk.NORMAL)

        if threading.current_thread() is not threading.main_thread():
            return
        try:
            _apply()
        except RuntimeError:
            pass

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
            self.root, orient=tk.HORIZONTAL, sashrelief=tk.RAISED
        )
        self.widgets.paned.place(relx=0.001, rely=0.001, relwidth=0.99, relheight=0.77)

        # Output display with scrollbar
        self.widgets.output_display = tk.Frame(
            self.widgets.paned, bg=self.config.output_bg
        )

        # Create a notebook (tabbed interface) for output
        self.widgets.output_notebook = ttk.Notebook(self.widgets.output_display)
        self.widgets.output_notebook.pack(expand=True, fill=tk.BOTH, padx=0, pady=0)

        # Create Output tab
        self.widgets.output_tab = tk.Frame(
            self.widgets.output_notebook, bg=self.config.output_bg
        )
        self.widgets.output_notebook.add(self.widgets.output_tab, text="Output")

        # Create output text and scrollbar in the Output tab
        self.widgets.output_scrollbar = tk.Scrollbar(self.widgets.output_tab)
        self.widgets.output_text = tk.Text(
            self.widgets.output_tab,
            wrap=tk.WORD,
            font=text_font,
            yscrollcommand=self.widgets.output_scrollbar.set,
        )
        self.widgets.output_scrollbar.config(command=self.widgets.output_text.yview)
        self.widgets.output_text.pack(side=tk.LEFT, expand=True, fill=tk.BOTH)
        self.widgets.output_scrollbar.pack(side=tk.RIGHT, fill=tk.Y)

        # Ensure selection highlighting is visible
        self.widgets.output_text.tag_config(
            "sel", background="#3399ff", foreground="#ffffff"
        )

        self.widgets.paned.add(self.widgets.output_display, stretch="always")

    def _create_status_panel(self) -> None:
        """Create status panel with tabs."""
        self.widgets.system_status = tk.Frame(
            self.widgets.paned, bg=self.config.status_bg
        )

        # Create a frame for model selector at the top
        model_frame = tk.Frame(self.widgets.system_status, bg=self.config.status_bg)
        model_frame.pack(fill=tk.X, padx=5, pady=5)
        
        # Add model selector
        self.model_selector = ModelSelector(
            parent=model_frame,
            on_model_change=self._on_model_change,
        )
        self.model_selector.get_widget().pack(side=tk.LEFT)

        # Create a notebook (tabbed interface) for system status
        self.widgets.system_notebook = ttk.Notebook(self.widgets.system_status)
        self.widgets.system_notebook.pack(expand=True, fill=tk.BOTH, padx=0, pady=0)

        # Create Session tab
        self.widgets.session_tab = tk.Frame(
            self.widgets.system_notebook, bg=self.config.status_bg
        )
        self.widgets.system_notebook.add(self.widgets.session_tab, text="Session")
        
        # Add tool panel to session tab (rendered directly here)
        self._tool_panel_frame = None
        self._tool_panel_vars = {}
        self._tool_panel_expanded = True
        self._create_tool_panel(self.widgets.session_tab)

        # Create Files tab
        self.widgets.files_tab = tk.Frame(
            self.widgets.system_notebook, bg=self.config.status_bg
        )
        self.widgets.system_notebook.add(self.widgets.files_tab, text="Files")

        # Bind tab change event to force widget updates
        def on_tab_changed(event):
            self.root.update_idletasks()
            selected_tab = self.widgets.system_notebook.select()
            if selected_tab:
                self.widgets.system_notebook.nametowidget(
                    selected_tab
                ).update_idletasks()

        self.widgets.system_notebook.bind("<<NotebookTabChanged>>", on_tab_changed)

        self.widgets.paned.add(self.widgets.system_status, stretch="always")

        # Set the sash position to create a 2:1 split after widgets are rendered
        def set_initial_split():
            self.root.update_idletasks()
            paned_width = self.widgets.paned.winfo_width()
            if paned_width > 1:
                sash_position = int(paned_width * self.config.output_panel_ratio)
                self.widgets.paned.sash_place(0, sash_position, 1)

        self.root.after(100, set_initial_split)

    def _create_tool_panel(self, parent):
        """Create the tool panel UI in the given parent."""
        if self._tool_panel_frame:
            self._tool_panel_frame.destroy()
        self._tool_panel_frame = tk.Frame(parent)
        self._tool_panel_frame.pack(fill=tk.BOTH, expand=True, padx=5, pady=5)

        # Header with expand/collapse
        header = tk.Frame(self._tool_panel_frame)
        header.pack(fill=tk.X, padx=5, pady=(5, 0))
        btn = tk.Button(
            header,
            text="▼" if self._tool_panel_expanded else "▶",
            width=2,
            font=("Terminal", 10),
            command=self._toggle_tool_panel_expand
        )
        btn.pack(side=tk.LEFT)
        label = tk.Label(
            header,
            text="Available Tools",
            font=("Terminal", 10, "bold")
        )
        label.pack(side=tk.LEFT, padx=(5, 0))

        # Collapsible container
        self._tool_panel_tools_container = tk.Frame(self._tool_panel_frame)
        if self._tool_panel_expanded:
            self._tool_panel_tools_container.pack(fill=tk.BOTH, expand=True, padx=5, pady=5)

        # If no tools, show empty
        if not hasattr(self, '_tool_panel_tools') or not self._tool_panel_tools:
            empty = tk.Label(
                self._tool_panel_tools_container,
                text="No tools available",
                foreground="gray",
                font=("", 9, "italic")
            )
            empty.grid(row=0, column=0, sticky="w", pady=10)
            return

        # Render tool checkboxes
        self._tool_panel_vars = {}
        for idx, tool in enumerate(self._tool_panel_tools):
            name = tool.get("name", "Unknown")
            description = tool.get("description", "")
            var = tk.BooleanVar(value=True)
            self._tool_panel_vars[name] = var
            cb = tk.Checkbutton(
                self._tool_panel_tools_container,
                text=name,
                variable=var,
                command=lambda n=name, v=var: self._on_tool_toggle(n, v.get())
            )
            cb.grid(row=idx, column=0, sticky="w", pady=2, padx=(0, 5))
            if description:
                desc_text = f"- {description[:50]}..." if len(description) > 50 else f"- {description}"
                desc = tk.Label(
                    self._tool_panel_tools_container,
                    text=desc_text,
                    foreground="gray",
                    font=("", 9)
                )
                desc.grid(row=idx, column=1, sticky="w")

    def _toggle_tool_panel_expand(self):
        self._tool_panel_expanded = not self._tool_panel_expanded
        self._create_tool_panel(self.widgets.session_tab)

    def populate_tools(self, tools: list[dict]) -> None:
        """Populate tool panel with available tools."""
        self._tool_panel_tools = tools
        self._create_tool_panel(self.widgets.session_tab)

    def get_enabled_tools(self) -> list[str]:
        """Get list of currently enabled tools."""
        if hasattr(self, '_tool_panel_vars'):
            return [name for name, var in self._tool_panel_vars.items() if var.get()]
        return []

    def _create_input_panel(self) -> None:
        """Create user input area."""
        # Get or setup font
        text_font = self._text_font or self.config.default_font
        enter_emoji_unicode = "^⏎"

        # Add a frame for attachments display
        self.widgets.attachments_frame = tk.Frame(self.root, height=2)
        self.widgets.attachments_frame.place(
            relx=0.001, rely=0.77, relwidth=1.0, relheight=0.03
        )

        # User input with scrollbar
        self.widgets.user_input = tk.Frame(self.root, bg=self.config.input_bg)
        self.widgets.user_input.place(
            relx=0.001, rely=0.80, relwidth=1.0, relheight=0.2
        )

        self.widgets.input_scrollbar = tk.Scrollbar(self.widgets.user_input)
        self.widgets.user_input_text = tk.Text(
            self.widgets.user_input,
            wrap=tk.WORD,
            font=text_font,
            yscrollcommand=self.widgets.input_scrollbar.set,
        )
        original_insert = self.widgets.user_input_text.insert

        def insert_with_cache(*args, **kwargs):
            result = original_insert(*args, **kwargs)
            try:
                self._cached_user_input = self.widgets.user_input_text.get("1.0", tk.END).strip()
            except Exception:
                pass
            return result

        self.widgets.user_input_text.insert = insert_with_cache
        self.widgets.input_scrollbar.config(command=self.widgets.user_input_text.yview)
        self.widgets.user_input_text.place(relx=0, rely=0, relwidth=0.90, relheight=1.0)
        self.widgets.input_scrollbar.place(relx=0.90, rely=0, relheight=1.0)

        # Submit button
        self.widgets.user_submit = tk.Button(
            self.widgets.user_input,
            text=enter_emoji_unicode,
            command=self._on_submit_clicked,
        )
        self.widgets.user_submit.place(relx=0.92, rely=0, relwidth=0.07, relheight=0.25)

        # Break button below the submit button
        self.widgets.user_break = tk.Button(
            self.widgets.user_input,
            text="❌",
            command=self._on_interrupt_clicked,
            state=tk.DISABLED,
        )
        self.widgets.user_break.place(
            relx=0.92, rely=0.26, relwidth=0.07, relheight=0.25
        )

        # Bind Ctrl-Enter to trigger the user_submit button
        self.widgets.user_input_text.bind(
            "<Control-Return>", lambda event: self.widgets.user_submit.invoke()
        )

        # Bind Ctrl-Space globally to trigger the user_break button
        self.root.bind_all(
            "<Control-space>", lambda event: self.widgets.user_break.invoke()
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
        self, parent: tk.Frame, info: AttachmentInfo, is_history: bool = False
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
            bg = self.COLOR_ATTACHMENT_HISTORY_BG
            icon = "📜"
            suffix = " (history)"
        else:
            bg = self.COLOR_ATTACHMENT_BG
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
            bg=bg,
        )
        checkbox.pack(side=tk.LEFT, padx=5, pady=2)
        
        return att_frame
    
    # Callbacks for model selector and tool panel
    
    def _on_model_change(self, model: str) -> None:
        """Handle model selection change."""
        # This is a placeholder - the actual handler should be set by AgentXSession
        pass
    
    def _on_tool_toggle(self, tool_name: str, enabled: bool) -> None:
        """Handle tool toggle."""
        # This is a placeholder - the actual handler should be set by AgentXSession
        pass
    
    def populate_models(self, models: list[dict], initial_model: str = None) -> None:
        """Populate model selector with available models.
        
        Args:
            models: List of model dictionaries
            initial_model: Model to select initially (if present in list)
        """
        if self.model_selector:
            self.model_selector.populate(models, initial_model=initial_model)
    
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
