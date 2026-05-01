"""
Model selector widget for selecting Ollama models.

This widget fetches available models from Ollama/Agentix and allows
the user to select which model to use for the current session.
"""

import logging
import tkinter as tk
from tkinter import ttk
from typing import Callable, Optional

logger = logging.getLogger(__name__)


class ModelSelector:
    """
    Dropdown widget for selecting Ollama model.

    Displays available models and calls a callback when selection changes.
    An optional ``on_refresh`` callback may be supplied so the host can
    reload the model list from Ollama/Agentix on demand (PD-04-AF-004).

    Example::

        def on_model_change(model: str):
            session.active_model = model

        def on_refresh():
            models = agentix_adapter.get_models()
            selector.populate(models)

        selector = ModelSelector(
            parent=status_frame,
            on_model_change=on_model_change,
            initial_model="llama3.2",
            on_refresh=on_refresh,
        )
        selector.get_widget().pack(side=tk.LEFT)
    """

    def __init__(
        self,
        parent: tk.Widget,
        on_model_change: Callable[[str], None],
        initial_model: str = "",
        on_refresh: Optional[Callable[[], None]] = None,
    ):
        """
        Initialize model selector.

        Args:
            parent (tk.Widget): Parent tkinter widget.
            on_model_change (Callable[[str], None]): Callback when user selects a model.
            initial_model (str): Initial model selection.
            on_refresh (Optional[Callable[[], None]]): Optional callback invoked when the
                user clicks the ``[⟳]`` refresh button (PD-04-AF-004).  When ``None`` the
                button is still rendered but does nothing until a callback is registered
                via :meth:`set_refresh_callback`.
        """
        self.parent = parent
        self.on_model_change = on_model_change
        self._on_refresh_callback: Optional[Callable[[], None]] = on_refresh
        self.current_model = tk.StringVar(master=parent, value=initial_model)
        self._models: dict[str, str] = {}  # Map display name to full model info

        # Create frame
        self.frame = ttk.Frame(parent)

        # Label
        self.label = ttk.Label(self.frame, text="Model:")
        self.label.pack(side=tk.LEFT, padx=(0, 5))

        # Dropdown
        self.dropdown = ttk.Combobox(self.frame, textvariable=self.current_model, state="readonly", width=25)
        self.dropdown.bind("<<ComboboxSelected>>", self._on_selection)
        self.dropdown.pack(side=tk.LEFT)

        # Refresh button (PD-04-AF-004)
        self.refresh_btn = ttk.Button(self.frame, text="⟳", width=3, command=self._on_refresh)
        self.refresh_btn.pack(side=tk.LEFT, padx=(4, 0))

    def populate(self, models: list[dict], initial_model: str = None) -> None:
        """
        Populate dropdown with available models.

        Args:
            models: List of model dictionaries from Ollama/Agentix
            initial_model: Model to select initially (if present in list)
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

        # Try to select initial_model if provided, otherwise select first
        selected = False
        if initial_model:
            # Find the display name for this model
            # Try multiple matching strategies to handle Ollama tags (e.g., "gpt-oss" vs "gpt-oss:latest")

            # Strategy 1: Exact match
            for display_name, actual_name in self._models.items():
                if actual_name == initial_model:
                    self.current_model.set(display_name)
                    selected = True
                    break

            # Strategy 2: Try with ":latest" tag if initial_model has no tag
            if not selected and ":" not in initial_model:
                initial_with_tag = f"{initial_model}:latest"
                for display_name, actual_name in self._models.items():
                    if actual_name == initial_with_tag:
                        self.current_model.set(display_name)
                        selected = True
                        break

            # Strategy 3: Match base name (before colon) if initial_model has no tag
            if not selected and ":" not in initial_model:
                for display_name, actual_name in self._models.items():
                    # Extract base name from actual_name (e.g., "gpt-oss:latest" -> "gpt-oss")
                    base_name = actual_name.split(":")[0] if ":" in actual_name else actual_name
                    if base_name == initial_model:
                        self.current_model.set(display_name)
                        selected = True
                        break

            if not selected:
                logger.warning("Model '%s' not found in available models", initial_model)

        # If initial_model not found or not provided, select first if no current selection
        if not selected and not self.current_model.get() and model_names:
            self.current_model.set(model_names[0])
            self.on_model_change(self._models[model_names[0]])

    def set_refresh_callback(self, callback: Callable[[], None]) -> None:
        """Register or replace the refresh callback (PD-04-AF-004).

        Args:
            callback (Callable[[], None]): Called when the user clicks ``[⟳]``.
        """
        self._on_refresh_callback = callback

    def _on_refresh(self) -> None:
        """Handle refresh button click — invoke the registered callback if any (PD-04-AF-004).

        Affordance ID: PD-04-AF-004
        """
        if self._on_refresh_callback is not None:
            self._on_refresh_callback()

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
