"""
Model selector widget for selecting Ollama models.

This widget fetches available models from Ollama/Agentix and allows
the user to select which model to use for the current session.
"""

import tkinter as tk
from tkinter import ttk
from typing import Callable, Optional


class ModelSelector:
    """
    Dropdown widget for selecting Ollama model.
    
    Displays available models and calls a callback when selection changes.
    
    Example:
        def on_model_change(model: str):
            session.config["agentx"]["ollama_model"] = model
        
        selector = ModelSelector(
            parent=status_frame,
            on_model_change=on_model_change,
            initial_model="llama3.2"
        )
        selector.get_widget().pack(side=tk.LEFT)
    """
    
    def __init__(
        self,
        parent: tk.Widget,
        on_model_change: Callable[[str], None],
        initial_model: str = ""
    ):
        """
        Initialize model selector.
        
        Args:
            parent: Parent tkinter widget
            on_model_change: Callback when user selects a model
            initial_model: Initial model selection
        """
        self.parent = parent
        self.on_model_change = on_model_change
        self.current_model = tk.StringVar(value=initial_model)
        self._models = {}  # Map display name to full model info
        
        # Create frame
        self.frame = ttk.Frame(parent)
        
        # Label
        self.label = ttk.Label(self.frame, text="Model:")
        self.label.pack(side=tk.LEFT, padx=(0, 5))
        
        # Dropdown
        self.dropdown = ttk.Combobox(
            self.frame,
            textvariable=self.current_model,
            state="readonly",
            width=25
        )
        self.dropdown.bind("<<ComboboxSelected>>", self._on_selection)
        self.dropdown.pack(side=tk.LEFT)
    
    def populate(self, models: list[dict]) -> None:
        """
        Populate dropdown with available models.
        
        Args:
            models: List of model dictionaries from Ollama/Agentix
        """
        self._models.clear()
        model_names = []
        
        for model_info in models:
            name = model_info.get("name", str(model_info))
            size = model_info.get("size", 0)
            
            # Format display name with size info
            if size:
                size_str = self._format_size(size)
                display_name = f"{name} ({size_str})"
            else:
                display_name = name
            
            model_names.append(display_name)
            self._models[display_name] = name  # Store actual name
        
        # Set dropdown values
        self.dropdown["values"] = model_names
        
        # Select first if no current selection
        if not self.current_model.get() and model_names:
            self.current_model.set(model_names[0])
            self.on_model_change(self._models[model_names[0]])
    
    def _on_selection(self, event) -> None:
        """Handle model selection change."""
        selected_display = self.current_model.get()
        if selected_display in self._models:
            actual_name = self._models[selected_display]
            self.on_model_change(actual_name)
    
    def _format_size(self, size_bytes: int) -> str:
        """Format byte size to human readable format."""
        for unit in ["B", "KB", "MB", "GB"]:
            if size_bytes < 1024:
                return f"{size_bytes:.1f}{unit}"
            size_bytes /= 1024
        return f"{size_bytes:.1f}TB"
    
    def get_selected_model(self) -> str:
        """Get currently selected model name."""
        display_name = self.current_model.get()
        return self._models.get(display_name, display_name)
    
    def get_widget(self) -> tk.Widget:
        """Return the frame widget for packing."""
        return self.frame
    
    def set_enabled(self, enabled: bool) -> None:
        """Enable or disable the selector."""
        state = "readonly" if enabled else "disabled"
        self.dropdown.config(state=state)
