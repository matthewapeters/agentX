"""GUI Manager implementation."""

import os
import re
import threading
import tkinter as tk
from datetime import datetime
from tkinter import messagebox as tk_messagebox
from tkinter import ttk
from typing import Any, Callable, Optional

from ..attachment_info import AttachmentInfo
from .collapsible_section import CollapsibleSection
from .gui_config import GUIConfig
from .settings_tab import SettingsTab
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
    COLOR_AGENT_CLASSIFICATION = "#7dd3fc"  # sky-blue — distinct from thinking/response
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
        self._section_bg = self.root.cget("bg")

        # Store callbacks
        self._on_submit = on_submit
        self._on_interrupt = on_interrupt
        self._on_attachment_toggle = on_attachment_toggle

        # Widget components (initialized in create_layout)
        self.model_selector: Optional[ModelSelector] = None

        # Session/System collapsible section state
        self._session_sections: dict[str, CollapsibleSection] = {}
        self._system_tab_frames: dict[str, tk.Frame] = {}
        self._system_tab_section_stacks: dict[str, tk.Frame] = {}
        self._session_section_spacing: int = self.config.session_section_spacing

        # Available tools state
        self._tool_panel_vars: dict[str, tk.BooleanVar] = {}
        self._tool_panel_tools: Optional[list] = None

        # Cache for text font
        self._text_font: Optional[tuple] = None

        # Cache for thread-safe input access
        self._cached_user_input: str = ""

        # Streaming label state
        self._agent_thinking_started = False
        self._agent_response_started = False
        self._agent_classification_shown = False

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
        self,
        history_obj: History,
        parent,
        user_name,
        on_attachment_toggle=None,
        include_header: bool = False,
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

        if include_header:
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
            history_label.grid(
                row=0, column=self.MESSAGE_COLUMNS["enabled"], sticky="w"
            )
            history_contexts_frame.grid(
                row=1, column=self.MESSAGE_COLUMNS["exp_button"], sticky="nsew"
            )
        else:
            history_contexts_frame = tk.Frame(history_frame)
            history_contexts_frame.pack(fill=tk.BOTH, expand=True)

        for idx, context in enumerate(history_obj.sessions):
            # Ensure each context is collapsed by default when rendering history
            context.expanded = False
            c_frame = self.render_context_widget(
                context,
                history_contexts_frame,
                on_attachment_toggle=on_attachment_toggle,
                include_header=True,
            )
            if include_header:
                c_frame.grid(
                    row=idx, column=self.MESSAGE_COLUMNS["exp_button"], sticky="nsew"
                )
            else:
                c_frame.pack(fill=tk.X, expand=False)

        if include_header:
            history_contexts_frame.grid_remove()  # Start collapsed
        return history_frame

    def render_context_widget(
        self,
        context_obj,
        parent,
        on_attachment_toggle=None,
        include_header: bool = False,
    ):
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

        if include_header:
            collapse_expand_button = self.collapse_expand_button(
                context_frame, context_messages_frame
            )
            context_label = tk.Label(
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
        else:
            context_messages_frame.pack(fill=tk.BOTH, expand=True)

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

        # Group messages: collect TOOL_CALL/TOOL_RESULT entries as children of
        # the nearest preceding non-tool message so they render nested in the UI.
        _TOOL_ROLES = {"tool_call", "tool_result"}

        def _role_str(msg) -> str:
            r = getattr(msg, "role", "")
            return r.value if hasattr(r, "value") else str(r)

        grouped: list[tuple] = []  # (message, [tool_interaction_messages])
        for entry in context_obj.messages:
            msg = entry.message if hasattr(entry, "message") else entry
            if _role_str(msg) in _TOOL_ROLES:
                if grouped:
                    grouped[-1][1].append(msg)
                # else: orphaned tool msg with no parent — skip silently
            else:
                grouped.append((msg, []))

        # Render grouped messages into the frame's grid
        current_row = 0
        for message, tool_msgs in grouped:
            current_row = self._render_message_to_grid(
                message, context_messages_frame, current_row,
                on_attachment_toggle, tool_msgs,
            )

        # Hide messages frame if not expanded on initial render (header mode only)
        if include_header and not getattr(context_obj, "expanded", False):
            context_messages_frame.grid_remove()

        return context_frame

    def _render_message_to_grid(
        self,
        message_obj,
        parent_frame: tk.Frame,
        start_row: int,
        on_attachment_toggle=None,
        tool_interactions: list | None = None,
    ) -> int:
        """
        Render a Message object directly into the parent frame's grid.

        This method places message components (checkbox, role, content, attachments,
        and any tool call/result sub-rows) directly into the parent frame's grid
        system using MESSAGE_COLUMNS for alignment.  Tool interactions are rendered
        as collapsible nested rows below the message — collapsed by default.

        Args:
            message_obj: The Message instance to render
            parent_frame: The parent tkinter Frame with grid layout
            start_row: The starting row number in the parent grid
            on_attachment_toggle: Optional callback for attachment toggles
            tool_interactions: Optional list of TOOL_CALL / TOOL_RESULT Message
                objects that were triggered by this message.  They are rendered
                as collapsible sub-rows beneath the message row.

        Returns:
            The next available row number after this message and its sub-rows
        """
        current_row = start_row
        tool_interactions = tool_interactions or []

        has_attachments = bool(getattr(message_obj, "attachments", []))
        has_tools = bool(tool_interactions)
        is_expandable = has_attachments or has_tools

        # Rows that will be toggled by the expand/collapse button.
        # Each inner list is one visual row's widgets.
        collapsible_rows: list[list[tk.Widget]] = []

        # Forward declare for use in toggle closure
        collapse_expand_button: tk.Button

        # Column 0: Collapse/Expand button (or empty spacer)
        if is_expandable:
            collapse_expand_button = self.collapse_expand_button(
                parent=parent_frame, attachment_rows=collapsible_rows
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
        if has_tools:
            preview_text += f"  [{len(tool_interactions)} tool interaction{'s' if len(tool_interactions) != 1 else ''}]"
        preview = preview_text[:60] + ("..." if len(preview_text) > 60 else "")
        preview_label = tk.Label(parent_frame, text=preview, anchor="w", width=50)
        preview_label.grid(
            row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew"
        )

        current_row += 1

        # ── Attachment sub-rows ──────────────────────────────────────────────
        if has_attachments:
            for att in getattr(message_obj, "attachments", []):
                row_widgets: list[tk.Widget] = []

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

                for widget in row_widgets:
                    widget.grid_remove()

                collapsible_rows.append(row_widgets)
                current_row += 1

        # ── Tool interaction sub-rows ────────────────────────────────────────
        if has_tools:
            current_row = self._render_tool_rows(
                tool_interactions, parent_frame, current_row, collapsible_rows
            )

        return current_row

    def _render_tool_rows(
        self,
        tool_msgs: list,
        parent_frame: tk.Frame,
        start_row: int,
        parent_collapsible: list[list[tk.Widget]],
    ) -> int:
        """
        Render TOOL_CALL / TOOL_RESULT messages as collapsible nested sub-rows.

        Each tool call is paired with its result (matched by tool_id) and rendered
        as a header row (tool icon + name) with its own expand button revealing the
        raw input and output below.  All rows start hidden (collapsed under the
        parent message's expand button).

        Args:
            tool_msgs: Flat list of TOOL_CALL and TOOL_RESULT Message objects.
            parent_frame: The grid frame to place rows into.
            start_row: First grid row to use.
            parent_collapsible: The parent message's collapsible_rows list — each
                tool header row is appended here so the parent's toggle hides them.

        Returns:
            Next available grid row after all tool sub-rows.
        """
        import json as _json

        current_row = start_row

        # Pair tool_calls with their results by tool_id; preserve call order.
        def _role_str(m) -> str:
            r = getattr(m, "role", "")
            return r.value if hasattr(r, "value") else str(r)

        calls = [m for m in tool_msgs if _role_str(m) == "tool_call"]
        results_by_id: dict[str, object] = {}
        for m in tool_msgs:
            if _role_str(m) == "tool_result":
                key = getattr(m, "tool_id", None) or getattr(m, "tool_name", "")
                results_by_id[key] = m

        for call_msg in calls:
            call_name = getattr(call_msg, "tool_name", "") or getattr(call_msg, "content", "")
            call_input = getattr(call_msg, "tool_input", None)
            call_id = getattr(call_msg, "tool_id", None) or getattr(call_msg, "tool_name", "")
            result_msg = results_by_id.get(call_id)

            # Widgets that toggle with the parent message's expand button
            header_row_widgets: list[tk.Widget] = []
            # Widgets revealed by this tool's own expand button
            detail_rows: list[list[tk.Widget]] = []

            # ── Tool-call header row ─────────────────────────────────────────
            # col 0: per-tool expand button
            tool_btn = self.collapse_expand_button(
                parent=parent_frame, attachment_rows=detail_rows
            )
            tool_btn.grid(
                row=current_row,
                column=self.MESSAGE_COLUMNS["exp_button"],
                sticky="nsew",
            )
            header_row_widgets.append(tool_btn)

            tool_icon = tk.Label(parent_frame, text="🔧", anchor="w")
            tool_icon.grid(row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nsew")
            header_row_widgets.append(tool_icon)

            result_preview = ""
            if result_msg:
                out = getattr(result_msg, "tool_output", None) or getattr(result_msg, "content", "")
                if isinstance(out, (dict, list)):
                    out_str = _json.dumps(out)
                else:
                    out_str = str(out) if out else ""
                result_preview = f"  →  {out_str[:40]}{'...' if len(out_str) > 40 else ''}"

            tool_label = tk.Label(
                parent_frame,
                text=f"{call_name}{result_preview}",
                anchor="w",
                foreground="gray",
                font=("", 9),
            )
            tool_label.grid(
                row=current_row,
                column=self.MESSAGE_COLUMNS["content"],
                sticky="nsew",
            )
            header_row_widgets.append(tool_label)

            current_row += 1

            # ── Input detail row ─────────────────────────────────────────────
            if call_input is not None:
                try:
                    input_str = _json.dumps(call_input, indent=2)
                except Exception:
                    input_str = str(call_input)

                in_icon = tk.Label(parent_frame, text="📥", anchor="w")
                in_icon.grid(row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nw", padx=(12, 0))
                in_text = tk.Label(
                    parent_frame,
                    text=input_str,
                    anchor="w",
                    justify="left",
                    foreground="#4a9eff",
                    font=("Courier", 8),
                    wraplength=380,
                )
                in_text.grid(row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew")
                in_row = [in_icon, in_text]
                for w in in_row:
                    w.grid_remove()
                detail_rows.append(in_row)
                current_row += 1

            # ── Output detail row ────────────────────────────────────────────
            if result_msg:
                out_raw = getattr(result_msg, "tool_output", None) or getattr(result_msg, "content", "")
                if isinstance(out_raw, (dict, list)):
                    out_str = _json.dumps(out_raw, indent=2)
                else:
                    out_str = str(out_raw) if out_raw else ""

                out_icon = tk.Label(parent_frame, text="📤", anchor="w")
                out_icon.grid(row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nw", padx=(12, 0))
                out_text = tk.Label(
                    parent_frame,
                    text=out_str[:800] + ("…" if len(out_str) > 800 else ""),
                    anchor="w",
                    justify="left",
                    foreground="#5cb85c",
                    font=("Courier", 8),
                    wraplength=380,
                )
                out_text.grid(row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew")
                out_row = [out_icon, out_text]
                for w in out_row:
                    w.grid_remove()
                detail_rows.append(out_row)
                current_row += 1

            # All header + detail rows start hidden (collapsed under parent)
            for w in header_row_widgets:
                w.grid_remove()
            parent_collapsible.append(header_row_widgets)

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
        self._agent_classification_shown = False

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
            self._agent_thinking_started = True

        # Append content
        output.insert(tk.END, content, ("agent_thinking",))

        # Auto-scroll
        output.see(tk.END)

    def display_classification(self, classification: dict) -> None:
        """Display prompt classification metadata block in the output panel.

        Args:
            classification: Dict with keys: intent, next_step, reasoning_summary,
                            needs_clarification, missing_fields.
        """
        if threading.current_thread() is not threading.main_thread():
            return

        output = self.widgets.output_text
        if output is None or self._agent_classification_shown:
            return

        self._agent_classification_shown = True
        tag = ("agent_classification",)

        intent = classification.get("intent", "")
        reasoning = classification.get("reasoning_summary", "")
        needs_clarification = classification.get("needs_clarification", False)
        missing_fields: list = classification.get("missing_fields") or []
        next_step = classification.get("next_step", "")

        # 🤔 analysis block
        if intent:
            output.insert(tk.END, f"🤔 intent: {intent}\n", tag)
        if reasoning:
            output.insert(tk.END, f"   reasoning: {reasoning}\n", tag)
        if needs_clarification or missing_fields:
            clarification_line = "   clarification needed: yes"
            if missing_fields:
                clarification_line += f"  |  missing fields: {', '.join(missing_fields)}"
            output.insert(tk.END, clarification_line + "\n", tag)

        # 💡 routing decision
        if next_step:
            output.insert(tk.END, f"💡 path: {next_step}\n", tag)

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
        self._agent_classification_shown = False

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
        section = self._session_sections.get("context")
        if section is None:
            raise RuntimeError("Context section not initialized")

        section.set_content(context_widget, fill=tk.BOTH, expand=True)
        self.widgets.system_status_context = context_widget

    def update_history_panel(self, history_widget: tk.Widget) -> None:
        """Replace history panel content with new widget.

        Args:
            history_widget: Fully rendered history widget from History.to_gui()
        """
        section = self._session_sections.get("history")
        if section is None:
            raise RuntimeError("History section not initialized")

        section.set_content(history_widget, fill=tk.BOTH, expand=False)
        self.widgets.system_status_history = history_widget

    def update_working_memory_panel(self, working_memory_widget: tk.Widget, fact_count: int = 0) -> None:
        """Replace Working Memory panel content with a newly rendered widget.

        Args:
            working_memory_widget: Fully rendered working memory widget from
                ``render_working_memory_widget()``.
            fact_count: Number of facts currently in working memory; shown in
                the section header so the user gets immediate feedback.
        """
        section = self._session_sections.get("working_memory")
        if section is None:
            return  # Working Memory disabled — silently no-op
        label = "🏛️ Working Memory" if fact_count == 0 else f"🏛️ Working Memory ({fact_count})"
        section.set_title(label)
        section.set_content(working_memory_widget, fill=tk.BOTH, expand=True)

    def get_working_memory_parent(self) -> tk.Widget:
        """Get parent widget for Working Memory panel rendering.

        Returns:
            The content container of the 🏛️ Working Memory collapsible section.

        Raises:
            RuntimeError: If the section has not yet been created (layout not called).
        """
        section = self._session_sections.get("working_memory")
        if section is None:
            raise RuntimeError("working_memory section not yet created")
        return section.content_container

    def render_working_memory_widget(
        self,
        working_memory,
        parent: tk.Widget,
        on_toggle: "Callable[[str, bool], None] | None" = None,
        on_delete: "Callable[[str], None] | None" = None,
        on_promote: "Callable[[str], None] | None" = None,
        on_user_add: "Callable[[str, str], None] | None" = None,
    ) -> tk.Frame:
        """Render a WorkingMemory instance as a Tkinter widget tree.

        Produces a scrollable frame listing all facts grouped by owner with
        per-row controls:
          - enable/disable checkbox (all facts)
          - delete button (agent-owned only)
          - 🤖 promote button (agent-owned only) → confirmation dialog
          - user-add form at the bottom

        Args:
            working_memory: ``WorkingMemory`` instance to render.
            parent: Parent widget.
            on_toggle: Called with ``(compound_key, enabled)`` on checkbox change.
            on_delete: Called with ``compound_key`` when delete is confirmed.
            on_promote: Called with ``compound_key`` when promote is confirmed.
            on_user_add: Called with ``(key, value_str)`` when user submits a new fact.

        Returns:
            Frame widget ready for placement.
        """
        from shared.models.working_memory import FactOwner

        outer = tk.Frame(parent, bg=self._section_bg)

        if working_memory is None or len(working_memory) == 0:
            tk.Label(
                outer,
                text="No facts stored yet.",
                bg=self._section_bg,
                font=("Terminal", 9),
            ).pack(anchor="w", padx=4, pady=2)
        else:
            facts = working_memory.all_facts()
            for fact in facts:
                self._render_working_memory_row(
                    outer,
                    fact,
                    on_toggle=on_toggle,
                    on_delete=on_delete,
                    on_promote=on_promote,
                )

        # --- User-add form ---
        sep = tk.Frame(outer, height=1, bg="#555555")
        sep.pack(fill=tk.X, padx=4, pady=(6, 2))

        add_frame = tk.Frame(outer, bg=self._section_bg)
        add_frame.pack(fill=tk.X, padx=4, pady=2)

        tk.Label(
            add_frame, text="👤 Add fact:", bg=self._section_bg,
            font=("Terminal", 9),
        ).grid(row=0, column=0, sticky="w")

        key_var = tk.StringVar()
        val_var = tk.StringVar()

        tk.Label(
            add_frame, text="key", bg=self._section_bg,
            font=("Terminal", 8),
        ).grid(row=1, column=0, sticky="w")
        tk.Entry(add_frame, textvariable=key_var, width=18, font=("Terminal", 9)).grid(
            row=1, column=1, sticky="ew", padx=2,
        )

        tk.Label(
            add_frame, text="value", bg=self._section_bg,
            font=("Terminal", 8),
        ).grid(row=2, column=0, sticky="w")
        tk.Entry(add_frame, textvariable=val_var, width=28, font=("Terminal", 9)).grid(
            row=2, column=1, sticky="ew", padx=2,
        )
        add_frame.columnconfigure(1, weight=1)

        def _submit_add():
            k = key_var.get().strip()
            v = val_var.get().strip()
            if k and on_user_add:
                on_user_add(k, v)
                key_var.set("")
                val_var.set("")

        tk.Button(
            add_frame, text="Add 👤", font=("Terminal", 9),
            command=_submit_add,
        ).grid(row=3, column=1, sticky="e", pady=2)

        return outer

    def _render_working_memory_row(
        self,
        parent: tk.Frame,
        fact,
        on_toggle=None,
        on_delete=None,
        on_promote=None,
    ) -> None:
        """Render a single FactEntry as a grid row inside parent."""
        from shared.models.working_memory import FactOwner

        row_frame = tk.Frame(parent, bg=self._section_bg)
        row_frame.pack(fill=tk.X, padx=2, pady=1)

        is_agent = fact.owner == FactOwner.AGENT

        # Enable/disable checkbox
        enabled_var = tk.BooleanVar(value=fact.enabled)

        def _on_toggle(ck=fact.compound_key, var=enabled_var):
            if on_toggle:
                on_toggle(ck, var.get())

        tk.Checkbutton(
            row_frame,
            variable=enabled_var,
            command=_on_toggle,
            bg=self._section_bg,
            activebackground=self._section_bg,
        ).grid(row=0, column=0, sticky="w")

        # Owner icon — clickable button for agent facts (promote), plain label for user
        if is_agent:
            def _on_promote_click(ck=fact.compound_key):
                self._confirm_promote(ck, on_promote)

            tk.Button(
                row_frame,
                text=fact.owner_icon,
                font=("Terminal", 10),
                relief="flat",
                bg=self._section_bg,
                activebackground=self._section_bg,
                cursor="hand2",
                command=_on_promote_click,
            ).grid(row=0, column=1, sticky="w", padx=(0, 2))
        else:
            tk.Label(
                row_frame,
                text=fact.owner_icon,
                bg=self._section_bg,
                font=("Terminal", 10),
            ).grid(row=0, column=1, sticky="w", padx=(0, 2))

        # Key: value label
        preview = fact.value_preview()
        label_text = f"{fact.key}: {preview}"
        tk.Label(
            row_frame,
            text=label_text,
            bg=self._section_bg,
            font=("Terminal", 9),
            anchor="w",
            justify="left",
        ).grid(row=0, column=2, sticky="ew", padx=2)
        row_frame.columnconfigure(2, weight=1)

        # Delete button (agent-owned only)
        if is_agent:
            def _on_delete(ck=fact.compound_key):
                if on_delete and tk_messagebox.askyesno(
                    "Delete Fact", f"Remove agent fact '{fact.key}'?"
                ):
                    on_delete(ck)

            tk.Button(
                row_frame,
                text="✕",
                font=("Terminal", 8),
                relief="flat",
                fg="#cc4444",
                bg=self._section_bg,
                activebackground=self._section_bg,
                cursor="hand2",
                command=_on_delete,
            ).grid(row=0, column=3, sticky="e", padx=2)

    def _confirm_promote(self, compound_key: str, on_promote) -> None:
        """Show promote-to-user confirmation dialog.

        Opens a simple Yes/No dialog.  The actual conflict-resolution dialog
        (if the target user key already exists) is handled by the caller after
        receiving the callback with the compound_key — the caller calls
        ``WorkingMemory.promote_to_user()`` and checks the returned status.
        """
        key = compound_key.split(":", 1)[-1] if ":" in compound_key else compound_key
        if tk_messagebox.askyesno(
            "Promote Fact",
            f"Promote 🤖 '{key}' to your own 👤 fact?\n\nYou will own this fact and the agent will no longer modify it automatically.",
        ):
            if on_promote:
                on_promote(compound_key)

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
        section = self._session_sections.get("context")
        if section is None:
            raise RuntimeError("context section not yet created")
        return section.content_container

    def get_history_parent(self) -> tk.Widget:
        """Get parent widget for history rendering.

        Returns:
            The widget to use as parent for history.to_gui()
        """
        section = self._session_sections.get("history")
        if section is None:
            raise RuntimeError("history section not yet created")
        return section.content_container

    def get_files_parent(self) -> tk.Widget:
        """Get parent widget for file explorer rendering.

        Returns:
            The widget to use as parent for file_explorer.to_gui()
        """
        if self.widgets.files_tab is None:
            raise RuntimeError("files_tab not yet created")
        return self.widgets.files_tab

    def get_settings_parent(self) -> tk.Widget:
        """Return the Settings tab frame (parent for SettingsTab widget).

        Raises:
            RuntimeError: If the Settings tab has not yet been created.
        """
        if getattr(self.widgets, "settings_tab", None) is None:
            raise RuntimeError("settings_tab not yet created")
        return self.widgets.settings_tab

    def render_settings_tab(
        self,
        config: dict,
        on_change: "Callable[[list[str], Any], None]",
        models: list | None = None,
        system_prompts_dir: str = "system_prompts",
    ) -> None:
        """Build or rebuild the ⚙️ Settings tab content.

        Destroys any existing SettingsTab widget and replaces it with a freshly
        rendered one using the current config snapshot and model list.

        Args:
            config: The full agentx config dict (passed by reference — not copied).
            on_change: Callback fired on each control interaction.
                Signature: ``(key_path: list[str], value: Any) -> None``
                If ``key_path[0] == '__config_only__'`` the change should only be
                written to disk, not hot-applied at runtime.
            models: List of model dicts from Ollama (same format as populate_models).
            system_prompts_dir: Path to system_prompts directory for prompt discovery.
        """
        parent = self.get_settings_parent()
        # Destroy previous SettingsTab if one exists.
        if getattr(self, "_settings_tab_widget", None) is not None:
            try:
                self._settings_tab_widget.frame.destroy()
            except Exception:
                pass
        self._settings_tab_widget = SettingsTab(
            parent,
            config=config,
            on_change=on_change,
            models=models or [],
            system_prompts_dir=system_prompts_dir,
            bg=self._section_bg,
        )
        self._settings_tab_widget.frame.pack(fill=tk.BOTH, expand=True)

    def populate_settings_models(self, models: list[dict]) -> None:
        """Refresh the model dropdowns in the Settings tab after models list changes."""
        widget = getattr(self, "_settings_tab_widget", None)
        if widget is not None:
            widget.populate_models(models)

    def register_system_collapsible_section(
        self,
        tab_name: str,
        section_key: str,
        title: str,
        initial_collapsed: bool = True,
        spacing: Optional[int] = None,
    ) -> tk.Widget:
        """Register a reusable collapsible section in a system tab.

        This enables future Session/System components to share the same
        look, feel, and behavior as existing collapsible sections.
        """
        section = self._register_system_collapsible_section(
            tab_name=tab_name,
            section_key=section_key,
            title=title,
            initial_collapsed=initial_collapsed,
            spacing=self._session_section_spacing if spacing is None else spacing,
        )
        return section.get_widget()

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
            self.widgets.paned, bg=self._section_bg
        )

        # Create a frame for model selector at the top
        model_frame = tk.Frame(self.widgets.system_status, bg=self._section_bg)
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
            self.widgets.system_notebook, bg=self._section_bg
        )
        self.widgets.system_notebook.add(self.widgets.session_tab, text="Session")

        # Build ordered, reusable section stack for Session tab
        self._register_system_collapsible_section(
            tab_name="Session",
            section_key="history",
            title="History",
            initial_collapsed=True,
            spacing=self._session_section_spacing,
        )
        self._register_system_collapsible_section(
            tab_name="Session",
            section_key="tools",
            title="Available Tools",
            initial_collapsed=True,
            spacing=self._session_section_spacing,
        )
        self._register_system_collapsible_section(
            tab_name="Session",
            section_key="working_memory",
            title="🏛️ Working Memory",
            initial_collapsed=False,
            spacing=self._session_section_spacing,
        )
        self._register_system_collapsible_section(
            tab_name="Session",
            section_key="context",
            title="Context",
            initial_collapsed=True,
            spacing=self._session_section_spacing,
        )

        # Initialize Available Tools section with empty-state content
        self._refresh_tools_section()

        # Create Files tab
        self.widgets.files_tab = tk.Frame(
            self.widgets.system_notebook, bg=self._section_bg
        )
        self.widgets.system_notebook.add(self.widgets.files_tab, text="Files")

        # Create Settings tab
        self.widgets.settings_tab = tk.Frame(
            self.widgets.system_notebook, bg=self._section_bg
        )
        self.widgets.system_notebook.add(self.widgets.settings_tab, text="⚙️ Settings")

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

    def _get_or_create_system_tab(self, tab_name: str) -> tk.Frame:
        if self.widgets.system_notebook is None:
            raise RuntimeError("system_notebook not yet created")

        key = tab_name.lower()
        if key in self._system_tab_frames:
            return self._system_tab_frames[key]

        if key == "session" and self.widgets.session_tab is not None:
            tab_frame = self.widgets.session_tab
        elif key == "files" and self.widgets.files_tab is not None:
            tab_frame = self.widgets.files_tab
        else:
            tab_frame = tk.Frame(self.widgets.system_notebook, bg=self._section_bg)
            self.widgets.system_notebook.add(tab_frame, text=tab_name)

        self._system_tab_frames[key] = tab_frame
        return tab_frame

    def _get_or_create_system_section_stack(self, tab_name: str) -> tk.Frame:
        key = tab_name.lower()
        if key in self._system_tab_section_stacks:
            return self._system_tab_section_stacks[key]

        tab_frame = self._get_or_create_system_tab(tab_name)
        stack = tk.Frame(tab_frame, bg=self._section_bg)
        stack.pack(fill=tk.BOTH, expand=True, padx=5, pady=5)
        self._system_tab_section_stacks[key] = stack
        return stack

    def _register_system_collapsible_section(
        self,
        tab_name: str,
        section_key: str,
        title: str,
        initial_collapsed: bool = True,
        spacing: int = 8,
    ) -> CollapsibleSection:
        stack = self._get_or_create_system_section_stack(tab_name)
        section = CollapsibleSection(
            parent=stack,
            title=title,
            initial_collapsed=initial_collapsed,
        )
        section.get_widget().pack(fill=tk.X, expand=False, pady=(0, spacing))
        if tab_name.lower() == "session":
            self._session_sections[section_key] = section
        return section

    def _parse_tool_metadata(self, tool: dict) -> tuple[str, str]:
        tool_func = tool.get("function") if isinstance(tool, dict) else None
        name = tool.get("name") if isinstance(tool, dict) else None
        if not name and isinstance(tool_func, dict):
            name = tool_func.get("name")
        if not name:
            name = "Unknown"

        description = tool.get("description", "") if isinstance(tool, dict) else ""
        if not description and isinstance(tool_func, dict):
            description = tool_func.get("description", "")
        return name, description

    def _build_tools_content_widget(self, parent: tk.Widget) -> tk.Widget:
        container = tk.Frame(parent)
        container.columnconfigure(0, weight=1)

        if not self._tool_panel_tools:
            empty = tk.Label(
                container,
                text="No tools available",
                foreground="gray",
                font=("", 9, "italic"),
            )
            empty.grid(row=0, column=0, sticky="w", pady=10)
            return container

        previous_enabled = {
            name: var.get() for name, var in self._tool_panel_vars.items()
        }
        self._tool_panel_vars = {}

        # Scrollable canvas so the list doesn't overflow its section.
        canvas = tk.Canvas(container, borderwidth=0, highlightthickness=0, height=160)
        scrollbar = tk.Scrollbar(container, orient="vertical", command=canvas.yview)
        canvas.configure(yscrollcommand=scrollbar.set)
        canvas.grid(row=0, column=0, sticky="nsew")
        scrollbar.grid(row=0, column=1, sticky="ns")
        container.rowconfigure(0, weight=1)

        content = tk.Frame(canvas)
        canvas_window = canvas.create_window((0, 0), window=content, anchor="nw")

        def _on_content_configure(event):
            canvas.configure(scrollregion=canvas.bbox("all"))

        def _on_canvas_configure(event):
            canvas.itemconfig(canvas_window, width=event.width)

        content.bind("<Configure>", _on_content_configure)
        canvas.bind("<Configure>", _on_canvas_configure)

        # Mouse-wheel scrolling (cross-platform).
        def _on_mousewheel(event):
            canvas.yview_scroll(int(-1 * (event.delta / 120)), "units")

        def _on_mousewheel_linux(event):
            canvas.yview_scroll(-1 if event.num == 4 else 1, "units")

        canvas.bind("<MouseWheel>", _on_mousewheel)
        canvas.bind("<Button-4>", _on_mousewheel_linux)
        canvas.bind("<Button-5>", _on_mousewheel_linux)

        for idx, tool in enumerate(self._tool_panel_tools):
            name, description = self._parse_tool_metadata(tool)
            is_enabled = previous_enabled.get(name, True)
            var = tk.BooleanVar(value=is_enabled)
            self._tool_panel_vars[name] = var

            checkbox = tk.Checkbutton(
                content,
                text=name,
                variable=var,
                command=lambda n=name, v=var: self._on_tool_toggle(n, v.get()),
            )
            checkbox.grid(row=idx, column=0, sticky="w", pady=2, padx=(0, 5))

            if description:
                desc_text = (
                    f"- {description[:50]}..."
                    if len(description) > 50
                    else f"- {description}"
                )
                description_label = tk.Label(
                    content,
                    text=desc_text,
                    foreground="gray",
                    font=("", 9),
                )
                description_label.grid(row=idx, column=1, sticky="w")

        return container

    def _refresh_tools_section(self) -> None:
        tools_section = self._session_sections.get("tools")
        if tools_section is None:
            return

        tools_content = self._build_tools_content_widget(tools_section.content_container)
        tools_section.set_content(tools_content, fill=tk.BOTH, expand=False)

    def populate_tools(self, tools: list[dict]) -> None:
        """Populate tool panel with available tools."""
        self._tool_panel_tools = tools
        self._refresh_tools_section()

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
        output.tag_config(
            "agent_classification",
            font=self.config.agent_thinking_font,
            foreground=self.COLOR_AGENT_CLASSIFICATION,
        )
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
