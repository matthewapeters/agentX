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
    
    def __init__(
        self,
        parent: tk.Widget,
        on_tool_toggle: Callable[[str, bool], None]
    ):
        """
        Initialize tool panel.
        
        Args:
            parent: Parent tkinter widget
            on_tool_toggle: Callback when tool is toggled
        """
        self.parent = parent
        self.on_tool_toggle = on_tool_toggle
        self.tool_vars: dict[str, tk.BooleanVar] = {}
        self.tool_info: dict[str, dict] = {}
        
        # Create frame
        self.frame = ttk.LabelFrame(parent, text="Available Tools", height=120)
        self.frame.pack_propagate(False)
        
        # Container for tool entries
        self.tools_container = ttk.Frame(self.frame)
        self.tools_container.pack(fill=tk.BOTH, expand=True, padx=5, pady=5)
        
        # Empty state message
        self.empty_label = ttk.Label(
            self.tools_container,
            text="No tools available",
            foreground="gray"
        )
        self.empty_label.pack(pady=10)
    
    def populate(self, tools: list[dict]) -> None:
        """
        Populate panel with tool definitions.
        
        Args:
            tools: List of tool dictionaries with name, description
        """
        # Clear existing widgets
        for widget in self.tools_container.winfo_children():
            widget.destroy()
        
        self.tool_vars.clear()
        self.tool_info.clear()
        
        if not tools:
            self.empty_label.pack(pady=10)
            return
        
        # Create checkboxes for each tool
        for tool in tools:
            name = tool.get("name", "Unknown")
            description = tool.get("description", "")
            
            # Create variable (enabled by default)
            var = tk.BooleanVar(value=True)
            self.tool_vars[name] = var
            self.tool_info[name] = tool
            
            # Create row frame
            row = ttk.Frame(self.tools_container)
            row.pack(fill=tk.X, anchor=tk.W, pady=2)
            
            # Checkbox
            checkbox = ttk.Checkbutton(
                row,
                text=name,
                variable=var,
                command=lambda n=name, v=var: self._on_tool_toggle(n, v.get())
            )
            checkbox.pack(side=tk.LEFT)
            
            # Description
            if description:
                desc_label = ttk.Label(
                    row,
                    text=f"  - {description}",
                    foreground="gray",
                    font=("", 9)
                )
                desc_label.pack(side=tk.LEFT, fill=tk.X)
    
    def _on_tool_toggle(self, tool_name: str, enabled: bool) -> None:
        """Handle tool toggle event."""
        self.on_tool_toggle(tool_name, enabled)
    
    def get_enabled_tools(self) -> list[str]:
        """
        Get list of enabled tool names.
        
        Returns:
            List of tool names that are currently enabled
        """
        return [
            name for name, var in self.tool_vars.items()
            if var.get()
        ]
    
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
