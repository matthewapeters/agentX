"""GUI Manager interface protocol."""

import tkinter as tk
from datetime import datetime
from typing import Callable, Optional, Protocol

from .attachment_info import AttachmentInfo


class IGUIManager(Protocol):
    """Interface for GUI implementations.

    This protocol defines the contract between the business logic layer
    (AgentXSession) and the presentation layer (GUIManager). All communication
    between layers flows through this interface.
    """

    # Lifecycle Methods

    def create_layout(self) -> None:
        """Creates and arranges all GUI widgets.

        Called once during initialization after the root window is created.
        Sets up the complete widget hierarchy and event bindings.
        """
        ...

    def destroy(self) -> None:
        """Cleanup and destroy all widgets.

        Called during application shutdown to properly release resources.
        """
        ...

    # Display Methods - Output

    def display_user_message(
        self, content: str, attachments: list[str], timestamp: datetime
    ) -> None:
        """Display a user message in the output area.

        Args:
            content: The message text content
            attachments: List of attachment filenames (for display only)
            timestamp: When the message was created

        Behavior:
            - Appends formatted message to output_text widget
            - Uses 'user_prompt' text tag for styling
            - Lists attachments below message with 'gray' tag
            - Auto-scrolls to show new content
            - Does NOT modify business state
        """
        ...

    def display_agent_thinking(self, content: str) -> None:
        """Display agent thinking content (streaming).

        Args:
            content: Chunk of thinking text to append

        Behavior:
            - On first call, inserts header: "(Agent is thinking...)\n\n"
            - Appends content with 'agent_thinking' tag
            - Auto-scrolls to show new content
            - Designed for streaming: called multiple times with chunks
        """
        ...

    def display_agent_response(self, content: str) -> None:
        """Display agent response content (streaming).

        Args:
            content: Chunk of response text to append

        Behavior:
            - On first call, inserts header: "Agent:\n\n"
            - Appends content with 'agent_response' tag
            - Auto-scrolls to show new content
            - Designed for streaming: called multiple times with chunks
        """
        ...

    def display_classification(self, classification: dict) -> None:
        """Display prompt classification metadata block in the output panel.

        Args:
            classification: Dict with keys:
                intent (str): Classifier intent label (e.g. "simple_action")
                next_step (str): Routing decision (e.g. "invoke_planner")
                reasoning_summary (str): Brief explanation from the classifier
                needs_clarification (bool): Whether the prompt is ambiguous
                missing_fields (list[str]): Fields the classifier found missing

        Behavior:
            - Renders a one-shot block before any THINKING or CONTENT output
            - 🤔 prefixes the analysis group (intent, reasoning, clarification)
            - 💡 prefixes the routing decision (next_step)
            - needs_clarification / missing_fields lines suppressed when falsy
            - Uses 'agent_classification' text tag for distinct styling
            - Auto-scrolls to show the block
            - Designed to be called once per turn (not a streaming method)
        """
        ...

    def display_error(self, message: str) -> None:
        """Display an error message to the user.

        Args:
            message: Error message text

        Behavior:
            - Appends error to output_text with error styling
            - Auto-scrolls to show error
        """
        ...

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

        Behavior:
            - Destroys existing attachment widgets
            - Creates checkbox + label for each attachment
            - Current attachments: white background, 📁 icon
            - History attachments: gray background, 📜 icon, "(history)" suffix
            - Checkbox bound to on_attachment_toggle callback

        Design:
            - Caller provides all attachment state
            - GUI doesn't maintain attachment state
            - Idempotent: can be called repeatedly with full state
        """
        ...

    # Display Methods - Context/History

    def update_context_panel(self, context_widget: tk.Widget) -> None:
        """Replace context panel content with new widget.

        Args:
            context_widget: Fully rendered context widget from Context.to_gui()

        Behavior:
            - Destroys existing system_status_context widget if exists
            - Packs new widget into session_tab
            - Stores reference in widgets.system_status_context
        """
        ...

    def update_history_panel(self, history_widget: tk.Widget) -> None:
        """Replace history panel content with new widget.

        Args:
            history_widget: Fully rendered history widget from History.to_gui()

        Behavior:
            - Destroys existing system_status_history widget
            - Packs new widget into session_tab (collapsed by default)
            - Stores reference in widgets.system_status_history
        """
        ...

    def update_files_panel(self, files_widget: tk.Widget) -> None:
        """Replace files panel content with new widget.

        Args:
            files_widget: Fully rendered file explorer from FileExplorer.to_gui()

        Behavior:
            - Destroys existing system_status_files widget
            - Packs new widget into files_tab
            - Stores reference in widgets.system_status_files
        """
        ...

    # Input Methods

    def get_user_input(self) -> str:
        """Extract and clear user input text.

        Returns:
            Stripped input text

        Behavior:
            - Gets text from user_input_text widget (1.0 to END)
            - Strips whitespace
            - Clears the widget
            - Returns the text

        Design:
            Combines get and clear for atomic operation
            Prevents duplicate submissions
        """
        ...

    def clear_user_input(self) -> None:
        """Clear the user input field.

        Behavior:
            - Deletes all text from user_input_text widget

        Use Case:
            Error conditions where input should be cleared
            without processing it
        """
        ...

    # State Management

    def set_streaming_state(self, is_streaming: bool) -> None:
        """Update UI for streaming state.

        Args:
            is_streaming: True if streaming in progress, False if idle

        Behavior:
            If is_streaming:
                - Disable submit button
                - Enable break button (for interrupt)
            Else:
                - Enable submit button
                - Disable break button

        Design:
            GUI reflects state, doesn't own it
            Session manages streaming state, tells GUI to update
        """
        ...

    def set_busy_state(self, is_busy: bool) -> None:
        """Update UI for busy operations (non-streaming).

        Args:
            is_busy: True if operation in progress

        Behavior:
            - Change cursor to wait/normal
            - Disable/enable input controls
            - Could show progress indicator in future

        Use Case:
            Loading history, initializing session, etc.
        """
        ...

    # Widget Access Methods

    def get_root(self) -> tk.Tk:
        """Get the root window.

        Returns:
            The tkinter root window

        Use Case:
            - Creating dialogs (file pickers, etc.)
            - Global key bindings
            - Window-level operations

        Design:
            Minimize usage - most operations should go through
            GUIManager methods. Expose only when necessary.
        """
        ...

    def get_context_parent(self) -> tk.Widget:
        """Get parent widget for context rendering.

        Returns:
            The widget to use as parent for context.to_gui()
        """
        ...

    def get_history_parent(self) -> tk.Widget:
        """Get parent widget for history rendering.

        Returns:
            The widget to use as parent for history.to_gui()
        """
        ...

    def get_files_parent(self) -> tk.Widget:
        """Get parent widget for file explorer rendering.

        Returns:
            The widget to use as parent for file_explorer.to_gui()
        """
        ...
