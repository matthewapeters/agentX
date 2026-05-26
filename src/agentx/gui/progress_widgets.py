"""
Progress tracking widgets for AgentX GUI.

Displays real-time progress of tool execution with status indicators,
progress bars, and streaming result display.
"""

import threading
import tkinter as tk
from datetime import datetime
from tkinter import ttk
from typing import Any, Callable, Optional

from ..integration.streaming_executor import ProgressType, ProgressUpdate


class ProgressIndicator(tk.Frame):
    """
    Displays progress for a single operation.
    """

    def __init__(self, parent, tool_name: str, **kwargs):
        """
        Initialize progress indicator.

        Args:
            parent: Parent widget
            tool_name: Name of the tool being executed
        """
        super().__init__(parent, **kwargs)

        self.tool_name = tool_name
        self.start_time = datetime.now()

        # Tool name label
        name_frame = tk.Frame(self)
        name_frame.pack(fill=tk.X, padx=5, pady=2)

        tk.Label(name_frame, text=tool_name, font=("Arial", 10, "bold")).pack(side=tk.LEFT)

        self.status_label = tk.Label(name_frame, text="Starting...", fg="blue", font=("Arial", 9))
        self.status_label.pack(side=tk.RIGHT)

        # Progress bar
        self.progress_var = tk.DoubleVar(value=0)
        self.progress_bar = ttk.Progressbar(
            self, variable=self.progress_var, maximum=100, length=300, mode="determinate"
        )
        self.progress_bar.pack(fill=tk.X, padx=5, pady=2)

        # Details frame
        details_frame = tk.Frame(self)
        details_frame.pack(fill=tk.X, padx=5, pady=2)

        self.details_label = tk.Label(details_frame, text="0 / ? bytes", font=("Arial", 8), fg="gray")
        self.details_label.pack(side=tk.LEFT)

        self.time_label = tk.Label(details_frame, text="0s", font=("Arial", 8), fg="gray")
        self.time_label.pack(side=tk.RIGHT)

    def update_progress(self, update: ProgressUpdate):
        """Update progress display."""
        # Update status
        status_map = {
            ProgressType.STARTED.value: ("Starting...", "blue"),
            ProgressType.PROGRESS.value: ("Processing...", "blue"),
            ProgressType.CHUNK.value: ("Streaming...", "blue"),
            ProgressType.COMPLETE.value: ("Complete", "green"),
            ProgressType.ERROR.value: ("Error", "red"),
            ProgressType.CANCELLED.value: ("Cancelled", "orange"),
        }

        status_text, color = status_map.get(update.type, ("Unknown", "gray"))
        self.status_label.config(text=status_text, fg=color)

        # Update progress bar
        if update.percent is not None:
            self.progress_var.set(update.percent)

        # Update details
        if update.current is not None and update.total is not None:
            self.details_label.config(text=f"{update.current} / {update.total} bytes")

        # Update elapsed time
        elapsed = (datetime.now() - self.start_time).total_seconds()
        minutes, seconds = divmod(int(elapsed), 60)
        if minutes > 0:
            time_str = f"{minutes}m {seconds}s"
        else:
            time_str = f"{seconds}s"
        self.time_label.config(text=time_str)


class ProgressPanel(tk.Frame):
    """
    Panel showing progress for multiple concurrent operations.
    """

    def __init__(self, parent, **kwargs):
        """Initialize progress panel."""
        super().__init__(parent, **kwargs)

        # Header
        header_frame = tk.Frame(self)
        header_frame.pack(fill=tk.X, padx=5, pady=5)

        tk.Label(header_frame, text="Execution Progress", font=("Arial", 12, "bold")).pack(side=tk.LEFT)

        # Operations tracking
        self._operations: dict[str, ProgressIndicator] = {}
        self._scrollable_frame = None
        self._setup_scrollable_area()

    def _setup_scrollable_area(self):
        """Setup scrollable container for progress indicators."""
        canvas = tk.Canvas(self, height=150)
        scrollbar = ttk.Scrollbar(self, orient="vertical", command=canvas.yview)

        self._scrollable_frame = tk.Frame(canvas)
        self._scrollable_frame.bind("<Configure>", lambda e: canvas.configure(scrollregion=canvas.bbox("all")))

        canvas.create_window((0, 0), window=self._scrollable_frame, anchor="nw")
        canvas.configure(yscrollcommand=scrollbar.set)

        canvas.pack(side="left", fill="both", expand=True, padx=5, pady=5)
        scrollbar.pack(side="right", fill="y")

    def start_operation(self, operation_id: str, tool_name: str):
        """Start tracking a new operation."""
        if operation_id not in self._operations:
            indicator = ProgressIndicator(self._scrollable_frame, tool_name, relief="sunken", bd=1)
            indicator.pack(fill=tk.X, pady=2)
            self._operations[operation_id] = indicator

    def update_operation(self, operation_id: str, update: ProgressUpdate):
        """Update operation progress."""
        if operation_id in self._operations:
            self._operations[operation_id].update_progress(update)

    def remove_operation(self, operation_id: str):
        """Remove operation from tracking."""
        if operation_id in self._operations:
            self._operations[operation_id].destroy()
            del self._operations[operation_id]

    def clear_completed(self):
        """Remove all completed operations."""
        for op_id in list(self._operations.keys()):
            status = self._operations[op_id].status_label.cget("text")
            if status in ["Complete", "Error", "Cancelled"]:
                self.remove_operation(op_id)


class ResultStreamWidget(tk.Frame):
    """
    Displays streamed results in real-time.
    """

    def __init__(self, parent, **kwargs):
        """Initialize result stream widget."""
        super().__init__(parent, **kwargs)

        # Header
        header_frame = tk.Frame(self)
        header_frame.pack(fill=tk.X, padx=5, pady=5)

        tk.Label(header_frame, text="Result Stream", font=("Arial", 12, "bold")).pack(side=tk.LEFT)

        # Text area with scrollbar
        text_frame = tk.Frame(self)
        text_frame.pack(fill=tk.BOTH, expand=True, padx=5, pady=5)

        scrollbar = ttk.Scrollbar(text_frame)
        scrollbar.pack(side=tk.RIGHT, fill=tk.Y)

        self.text_widget = tk.Text(
            text_frame, height=15, yscrollcommand=scrollbar.set, state="disabled", wrap=tk.WORD, font=("Courier", 9)
        )
        self.text_widget.pack(side=tk.LEFT, fill=tk.BOTH, expand=True)
        scrollbar.config(command=self.text_widget.yview)

        # Configure tags for different types
        self.text_widget.tag_config("error", foreground="red")
        self.text_widget.tag_config("success", foreground="green")
        self.text_widget.tag_config("info", foreground="blue")
        self.text_widget.tag_config("warning", foreground="orange")

    def append_chunk(self, chunk: str, tag: str = "info"):
        """Append a chunk of result text."""
        self.text_widget.config(state="normal")
        self.text_widget.insert(tk.END, chunk, tag)
        self.text_widget.see(tk.END)  # Auto-scroll to end
        self.text_widget.config(state="disabled")

    def append_update(self, update: ProgressUpdate):
        """Append a progress update."""
        if update.type == ProgressType.CHUNK.value and update.data:
            self.append_chunk(update.data)
        elif update.type == ProgressType.ERROR.value:
            self.append_chunk(f"ERROR: {update.message}\n", "error")
        elif update.type == ProgressType.COMPLETE.value:
            self.append_chunk(f"✓ {update.message}\n", "success")
        elif update.message:
            self.append_chunk(f"→ {update.message}\n", "info")

    def clear(self):
        """Clear all text."""
        self.text_widget.config(state="normal")
        self.text_widget.delete("1.0", tk.END)
        self.text_widget.config(state="disabled")


class StreamingExecutionUI(tk.Frame):
    """
    Complete UI for streaming tool execution with progress and results.
    """

    def __init__(self, parent, **kwargs):
        """Initialize streaming execution UI."""
        super().__init__(parent, **kwargs)

        # Progress panel
        self.progress_panel = ProgressPanel(self)
        self.progress_panel.pack(fill=tk.X, padx=5, pady=5)

        # Separator
        ttk.Separator(self, orient="horizontal").pack(fill=tk.X, padx=5, pady=2)

        # Result stream
        self.result_stream = ResultStreamWidget(self)
        self.result_stream.pack(fill=tk.BOTH, expand=True, padx=5, pady=5)

        # Control buttons
        button_frame = tk.Frame(self)
        button_frame.pack(fill=tk.X, padx=5, pady=5)

        tk.Button(button_frame, text="Clear Results", command=self.clear_results).pack(side=tk.LEFT, padx=2)

        tk.Button(button_frame, text="Clear Completed", command=self.progress_panel.clear_completed).pack(
            side=tk.LEFT, padx=2
        )

    def handle_progress_update(self, operation_id: str, update: ProgressUpdate):
        """Handle a progress update."""
        # Update progress panel
        if update.type == ProgressType.STARTED.value:
            self.progress_panel.start_operation(operation_id, update.tool_name)
        else:
            self.progress_panel.update_operation(operation_id, update)

        # Update result stream
        self.result_stream.append_update(update)

    def clear_results(self):
        """Clear all results."""
        self.result_stream.clear()

    def start_operation(self, operation_id: str, tool_name: str):
        """Start a new operation."""
        self.progress_panel.start_operation(operation_id, tool_name)
