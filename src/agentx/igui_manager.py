"""GUI Manager interface protocol."""

import tkinter as tk
from datetime import datetime
from typing import Any, Callable, Optional, Protocol

from .attachment_info import AttachmentInfo
from .gui.gui_config import GUIConfig


class IGUIManager(Protocol):
    """Interface for GUI implementations.

    This protocol defines the contract between the business logic layer
    (AgentXSession) and the presentation layer (GUIManager). All communication
    between layers flows through this interface.
    """

    # Configuration

    config: GUIConfig
    """The GUI configuration (theme colours, markdown flag, etc.).

    Session may read colour values for embedding into child widgets, and may
    write ``config.markdown_render_enabled`` when the setting changes.
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

    def display_user_message(self, content: str, attachments: list[str], timestamp: datetime) -> None:
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

    def display_startup_notice(self, content: str) -> None:
        """Display a non-agent startup notice in the output panel.

        Args:
            content: Friendly startup text, typically operational guidance.

        Behavior:
            - Renders a one-shot informational notice before first agent output.
            - Uses neutral/system visual styling (not assistant-response styling).
            - Auto-scrolls to ensure the notice is visible on startup.
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

    def finalize_current_turn_markdown(self) -> None:
        """Finalise all completed entries in the current turn by replacing streaming
        tk.Text widgets with rendered HtmlFrame widgets where markdown is detected.

        Called automatically by display_spacing(); exposed here for callers that need
        explicit control (e.g. bootstrap turn, plan-tab finalization).
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

    def update_working_memory_panel(self, working_memory_widget: tk.Widget, fact_count: int = 0) -> None:
        """Replace Working Memory panel content with a newly rendered widget.

        Args:
            working_memory_widget: Fully rendered widget from
                ``GUIManager.render_working_memory_widget()``.

        Behavior:
            - Replaces existing content inside the 🏛️ Working Memory section.
            - No-op if the working_memory section is not present (feature disabled).
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

    def get_settings_parent(self) -> tk.Widget:
        """Get the Settings tab frame."""
        ...

    def render_settings_tab(
        self,
        config: dict,
        on_change: Callable[[list[str], Any], None],
        models: list | None = None,
        system_prompts_dir: str = "system_prompts",
    ) -> None:
        """Build or rebuild the ⚙️ Settings tab content."""
        ...

    def populate_settings_models(self, models: list[dict]) -> None:
        """Refresh model dropdowns in the Settings tab."""
        ...

    # ── Plan tab methods ──────────────────────────────────────────────────────

    def add_plan_tab(self, plan_id: str, plan_name: str, on_export: Optional[Callable[[], None]] = None) -> tk.Widget:
        """Create a named tab in output_notebook for the given plan.

        Args:
            plan_id:   Unique plan identifier (from ``PlanRecord.plan_id``).
            plan_name: Human-readable plan name used as the tab label.
            on_export: Optional callback invoked when the Export button is clicked.

        Returns:
            The tab frame widget.
        """
        ...

    def get_plan_tab_frame(self, plan_id: str) -> "Optional[tk.Widget]":  # type: ignore[override]
        """Return the tab frame for an existing plan, or None."""
        ...

    def focus_plan_tab(self, plan_id: str) -> None:
        """Switch output notebook focus to the plan tab."""
        ...

    def add_plan_step_node(
        self, plan_id: str, task_id: str, description: str, tbd: bool, on_replay: Optional[Callable[[str], None]] = None
    ) -> None:
        """Add a root-level step row to the plan's tree widget."""
        ...

    def add_plan_subtask_node(
        self,
        task_id: str,
        parent_task_id: str,
        description: str,
        depth: int,
        on_replay: Optional[Callable[[str], None]] = None,
    ) -> None:
        """Add an indented sub-task row under its parent in the plan tree."""
        ...

    def update_plan_node_status(self, task_id: str, status: str) -> None:
        """Update the status icon for a node in the plan tree."""
        ...

    def resolve_plan_tbd_node(self, task_id: str, resolved_description: str) -> None:
        """Replace the TBD placeholder with the resolved description."""
        ...

    def add_plan_tool_call(self, task_id: str, tool_name: str, tool_input: Any) -> None:
        """Append a collapsible tool-call row to the node's details frame."""
        ...

    def add_plan_synthesis(
        self,
        task_id: str,
        synthesis_text: str,
        assertions: list,
        on_resynth: Optional[Callable] = None,
        on_add_wm_hint: Optional[Callable] = None,
    ) -> None:
        """Append synthesis text and assertion badges to the node."""
        ...

    def update_plan_synthesis(self, task_id: str, new_synthesis: str, assertions: list) -> None:
        """Replace synthesis text and badges in-place after a retrigger."""
        ...

    def mark_plan_node_invalidated(self, task_id: str) -> None:
        """Mark a node status as 'invalidated' (needs re-synthesis)."""
        ...

    def update_context_meter(self, max_tokens: int, breakdown: dict[str, int]) -> None:
        """Trigger a context-meter redraw with denominator and band data.

        Args:
            max_tokens: Active-model context-window denominator.
            breakdown: Per-band token estimates from ``Context.token_breakdown``.

        Implementations must be safe to call from non-main threads.
        """
        ...

    # Callback Registration

    def set_model_change_callback(self, callback: Callable[[str], None]) -> None:
        """Register the callback invoked whenever the active model changes.

        Args:
            callback: Receives the new model name string.

        Replaces any previously registered callback.  GUIManager also propagates
        the callback down into the ``ModelSelector`` widget so the two stay in sync.
        """
        ...

    def set_refresh_models_callback(self, callback: Callable[[], None]) -> None:
        """Register the callback invoked when the user clicks the model-list refresh button.

        Args:
            callback: Called with no arguments; should re-fetch models from
                Agentix/Ollama and call ``populate_models()`` to update the
                drop-down (PD-04-AF-004).

        Replaces any previously registered refresh callback.
        """
        ...

    def set_tool_toggle_callback(self, callback: Callable[[str, bool], None]) -> None:
        """Register the callback invoked whenever a tool is enabled/disabled.

        Args:
            callback: Receives (tool_name, enabled_flag).

        Replaces any previously registered callback.
        """
        ...

    def get_cached_user_input(self) -> str:
        """Return the most-recently submitted user input string.

        Returns:
            The cached prompt string, or an empty string if nothing has been
            submitted yet.  Used as a fallback in tests where the submit
            pipeline is bypassed.
        """
        ...
