"""
Tool panel widget for displaying and toggling available tools.

Shows available MCP tools (CST, AST, etc.) with enable/disable toggles
so users can control which tools the LLM can use.
"""

import tkinter as tk
from tkinter import ttk
from typing import Callable, Optional


class ToolPanel:
    """
    Display available tools with enable/disable toggles.

    Example:
        def on_tool_toggle(tool_name: str, enabled: bool):
            # Update session config with enabled tools
            enabled_tools = tool_panel.get_enabled_tools()
            session.config["agentix"]["available_tools"] = enabled_tools

        panel = ToolPanel(
            parent=session_frame,
            on_tool_toggle=on_tool_toggle
        )
        panel.populate([
            {"name": "cst", "description": "Concrete Syntax Tree analysis"},
            {"name": "ast", "description": "Abstract Syntax Tree analysis"},
        ])
        panel.get_widget().pack(fill=tk.X)
    """

    def __init__(self, parent: tk.Widget, on_tool_toggle: Callable[[str, bool], None]):
        """
        Initialize tool panel with header and collapsible tool list.

        Args:
            parent: Parent tkinter widget
            on_tool_toggle: Callback when tool is toggled
        """
        self.parent = parent
        self.on_tool_toggle = on_tool_toggle
        self.tool_vars: dict[str, tk.BooleanVar] = {}
        self.tool_info: dict[str, dict] = {}
        self.expanded_var = tk.BooleanVar(value=True)
        self._last_tools = None  # For state persistence

        # Main frame
        self.frame = ttk.Frame(parent)

        # Header with expand/collapse button and label
        header_frame = ttk.Frame(self.frame)
        header_frame.grid(row=0, column=0, sticky="ew", padx=5, pady=(5, 0))
        self.collapse_expand_btn = tk.Button(
            header_frame, text="▼", width=2, command=self._toggle_expand, font=("Terminal", 10)  # Expanded by default
        )
        self.collapse_expand_btn.grid(row=0, column=0, sticky="w")
        self.header_label = ttk.Label(header_frame, text="Available Tools", font=("Terminal", 10, "bold"))
        self.header_label.grid(row=0, column=1, sticky="w", padx=(5, 0))
        header_frame.columnconfigure(1, weight=1)

        # Collapsible container for tool entries
        self.tools_container = ttk.Frame(self.frame)
        self.tools_container.grid(row=1, column=0, sticky="nsew", padx=5, pady=5)
        self.frame.rowconfigure(1, weight=1)
        self.frame.columnconfigure(0, weight=1)

        # Empty state message
        self.empty_label = ttk.Label(
            self.tools_container, text="No tools available", foreground="gray", font=("", 9, "italic")
        )
        self.empty_label.grid(row=0, column=0, sticky="w", pady=10)

        # Start expanded
        self.tools_container.grid(row=1, column=0, sticky="nsew", padx=5, pady=5)

    def _toggle_expand(self):
        expanded = self.expanded_var.get()
        self.expanded_var.set(not expanded)
        if expanded:
            self.tools_container.grid_remove()
            self.collapse_expand_btn.config(text="▶")
        else:
            self.tools_container.grid(row=1, column=0, sticky="nsew", padx=5, pady=5)
            self.collapse_expand_btn.config(text="▼")

    def populate(self, tools: list[dict]) -> None:
        """
        Refresh the tool panel with new tool definitions.
        Destroys and recreates all tool widgets, preserving expand/collapse state.
        Args:
            tools: List of tool dictionaries with name, description
        """
        # Remember expand/collapse state and restore after refresh
        expanded = self.expanded_var.get()
        self._last_tools = tools

        # Clear existing widgets and state (preserve empty_label)
        for widget in self.tools_container.winfo_children():
            if widget is not self.empty_label:
                widget.destroy()
        self.tool_vars.clear()
        self.tool_info.clear()

        if not tools:
            self.empty_label.grid(row=0, column=0, sticky="w", pady=10)
            return
        else:
            self.empty_label.grid_remove()

        # Create checkboxes for each tool
        for idx, tool in enumerate(tools):
            name = tool.get("name", "Unknown")
            description = tool.get("description", "")
            var = tk.BooleanVar(value=True)
            self.tool_vars[name] = var
            self.tool_info[name] = tool
            # Row for each tool: checkbox and description
            checkbox = ttk.Checkbutton(
                self.tools_container,
                text=name,
                variable=var,
                command=lambda n=name, v=var: self._on_tool_toggle(n, v.get()),
            )
            checkbox.grid(row=idx, column=0, sticky="w", pady=2, padx=(0, 5))
            if description:
                desc_text = f"- {description[:50]}..." if len(description) > 50 else f"- {description}"
                desc_label = ttk.Label(self.tools_container, text=desc_text, foreground="gray", font=("", 9))
                desc_label.grid(row=idx, column=1, sticky="w")
                # Tooltip for accessibility
                self._add_tooltip(desc_label, description)

    def _add_tooltip(self, widget, text):
        """Add a simple tooltip to a widget."""

        def on_enter(event):
            self._show_tooltip(widget, text)

        def on_leave(event):
            self._hide_tooltip()

        widget.bind("<Enter>", on_enter)
        widget.bind("<Leave>", on_leave)

    def _show_tooltip(self, widget, text):
        if hasattr(self, "_tooltip") and self._tooltip:
            self._tooltip.destroy()
        x = widget.winfo_rootx() + 20
        y = widget.winfo_rooty() + 20
        self._tooltip = tk.Toplevel(widget)
        self._tooltip.wm_overrideredirect(True)
        self._tooltip.wm_geometry(f"+{x}+{y}")
        label = tk.Label(self._tooltip, text=text, background="#ffffe0", relief="solid", borderwidth=1, font=("", 9))
        label.pack(ipadx=2)

    def _hide_tooltip(self):
        if hasattr(self, "_tooltip") and self._tooltip:
            self._tooltip.destroy()
            self._tooltip = None

    def _on_tool_toggle(self, tool_name: str, enabled: bool) -> None:
        """Handle tool toggle event and propagate state immediately."""
        # Update the variable state immediately (in case called programmatically)
        if tool_name in self.tool_vars:
            self.tool_vars[tool_name].set(enabled)
        # Call the provided callback to update session config/Agentix
        if callable(self.on_tool_toggle):
            self.on_tool_toggle(tool_name, enabled)

    def get_enabled_tools(self) -> list[str]:
        """
        Get list of enabled tool names.

        Returns:
            List of tool names that are currently enabled
        """
        return [name for name, var in self.tool_vars.items() if var.get()]

    def set_tool_enabled(self, tool_name: str, enabled: bool) -> None:
        """
        Enable or disable a specific tool.

        Args:
            tool_name: Name of tool
            enabled: Whether to enable or disable
        """
        if tool_name in self.tool_vars:
            self.tool_vars[tool_name].set(enabled)

    def get_widget(self) -> tk.Widget:
        """Return the frame widget for packing."""
        return self.frame
