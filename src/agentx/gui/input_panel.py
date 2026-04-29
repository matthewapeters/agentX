"""Input panel extracted from GUIManager.

Owns the user-text input area, attachment bar, submit/interrupt buttons, and
related state.  Follows the back-reference pattern: ``InputPanel(gui_manager)``
stores ``self._g = gui_manager`` and accesses shared state from there.
"""

from __future__ import annotations

import threading
import tkinter as tk
from typing import TYPE_CHECKING

from .context_meter_widget import ContextMeterWidget

if TYPE_CHECKING:
    from .gui_manager import GUIManager


class InputPanel:
    """Manages the bottom input area: text field, buttons, and attachment bar."""

    def __init__(self, gui_manager: "GUIManager") -> None:
        self._g = gui_manager
        self._cached_user_input: str = ""
        self.context_meter: ContextMeterWidget = ContextMeterWidget(gui_manager)

    # ── Convenience accessors ─────────────────────────────────────────────────

    @property
    def _config(self):
        return self._g.config

    @property
    def _widgets(self):
        return self._g.widgets

    @property
    def _text_font(self):
        return self._g._text_font

    # ── Panel creation ────────────────────────────────────────────────────────

    def create(self) -> None:
        """Create user input area widgets."""
        text_font = self._text_font or self._config.default_font
        enter_emoji_unicode = "^⏎"

        # Attachment bar
        self._widgets.attachments_frame = tk.Frame(self._g.root, height=2)
        self._widgets.attachments_frame.place(relx=0.001, rely=0.77, relwidth=1.0, relheight=0.03)

        # User input frame
        self._widgets.user_input = tk.Frame(self._g.root, bg=self._config.input_bg)
        self._widgets.user_input.place(relx=0.001, rely=0.80, relwidth=1.0, relheight=0.2)

        self._widgets.input_scrollbar = tk.Scrollbar(self._widgets.user_input)
        self._widgets.user_input_text = tk.Text(
            self._widgets.user_input,
            wrap=tk.WORD,
            font=text_font,
            bg=self._config.input_bg,
            fg=self._config.input_fg,
            insertbackground=self._config.input_fg,
            yscrollcommand=self._widgets.input_scrollbar.set,
        )
        original_insert = self._widgets.user_input_text.insert

        def _insert_with_cache(*args, **kwargs):
            result = original_insert(*args, **kwargs)
            try:
                self._cached_user_input = self._widgets.user_input_text.get("1.0", tk.END).strip()
            except Exception:
                pass
            return result

        self._widgets.user_input_text.insert = _insert_with_cache
        self._widgets.input_scrollbar.config(command=self._widgets.user_input_text.yview)
        self._widgets.user_input_text.place(relx=0, rely=0, relwidth=0.90, relheight=1.0)
        self._widgets.input_scrollbar.place(relx=0.90, rely=0, relheight=1.0)

        # Submit button
        self._widgets.user_submit = tk.Button(
            self._widgets.user_input,
            text=enter_emoji_unicode,
            command=self._on_submit_clicked,
        )
        self._widgets.user_submit.place(relx=0.92, rely=0, relwidth=0.07, relheight=0.25)

        # Interrupt button
        self._widgets.user_break = tk.Button(
            self._widgets.user_input,
            text="❌",
            command=self._on_interrupt_clicked,
            state=tk.DISABLED,
        )
        self._widgets.user_break.place(relx=0.92, rely=0.26, relwidth=0.07, relheight=0.25)
        # Context meter (donut chart — ARCH-04)
        self.context_meter.create(self._widgets.user_input)
        self._widgets.context_meter_canvas = self.context_meter._canvas
        # Keyboard shortcuts
        self._widgets.user_input_text.bind(
            "<Control-Return>",
            lambda _event: self._widgets.user_submit.invoke(),
        )
        self._g.root.bind_all(
            "<Control-space>",
            lambda _event: self._widgets.user_break.invoke(),
        )

    # ── Public interface ──────────────────────────────────────────────────────

    def get_user_input(self) -> str:
        """Return and clear the current text input value."""
        text_widget = self._widgets.user_input_text
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
        text_widget = self._widgets.user_input_text
        if text_widget is not None:
            text_widget.delete("1.0", tk.END)

    def get_cached_user_input(self) -> str:
        """Return the last submitted input value (thread-safe, no side effects)."""
        return self._cached_user_input or ""

    def set_streaming_state(self, is_streaming: bool) -> None:
        """Update button states for streaming vs. idle."""
        if threading.current_thread() is not threading.main_thread():
            return
        try:
            submit = self._widgets.user_submit
            interrupt = self._widgets.user_break
            if submit is not None:
                submit.config(state=tk.DISABLED if is_streaming else tk.NORMAL)
            if interrupt is not None:
                interrupt.config(state=tk.NORMAL if is_streaming else tk.DISABLED)
        except RuntimeError:
            pass

    def set_busy_state(self, is_busy: bool) -> None:
        """Update UI for non-streaming busy operations."""
        if threading.current_thread() is not threading.main_thread():
            return
        try:
            cursor = "watch" if is_busy else ""
            try:
                self._g.root.config(cursor=cursor)
            except tk.TclError:
                self._g.root.config(cursor="")
            input_text = self._widgets.user_input_text
            submit = self._widgets.user_submit
            if input_text is not None:
                input_text.config(state=tk.DISABLED if is_busy else tk.NORMAL)
            if submit is not None:
                submit.config(state=tk.DISABLED if is_busy else tk.NORMAL)
        except RuntimeError:
            pass

    def update_attachment_bar(
        self,
        current_attachments: list,
        history_attachments: list,
    ) -> None:
        """Rebuild the attachment bar widgets above the input field."""
        frame = self._widgets.attachments_frame
        if frame is None:
            return
        self._widgets.clear_attachments()
        for info in current_attachments:
            widget = self._create_attachment_widget(frame, info, is_history=False)
            self._widgets.attachment_labels.append(widget)
        for info in history_attachments:
            widget = self._create_attachment_widget(frame, info, is_history=True)
            self._widgets.attachment_labels.append(widget)

    # ── Private helpers ───────────────────────────────────────────────────────

    def _on_submit_clicked(self) -> None:
        if self._g._on_submit:
            self._g._on_submit()

    def _on_interrupt_clicked(self) -> None:
        if self._g._on_interrupt:
            self._g._on_interrupt()

    def _create_attachment_widget(self, parent: tk.Frame, info, is_history: bool = False) -> tk.Widget:
        if is_history:
            bg = self._g.COLOR_ATTACHMENT_HISTORY_BG
            icon = "📜"
            suffix = " (history)"
        else:
            bg = self._g.COLOR_ATTACHMENT_BG
            icon = "📁"
            suffix = ""

        att_frame = tk.Frame(parent, bg=bg)
        att_frame.pack(side=tk.LEFT, padx=2, pady=2)

        var = tk.BooleanVar(value=info.enabled)

        def on_toggle(v=var, att_id=info.attachment_id):
            self._g._on_attachment_toggle(att_id, v.get())

        tk.Checkbutton(
            att_frame,
            text=f"{icon} {info.display_name}{suffix}",
            variable=var,
            command=on_toggle,
            bg=bg,
            fg=self._g.COLOR_ATTACHMENT_TEXT,
            activebackground=bg,
            activeforeground=self._g.COLOR_ATTACHMENT_TEXT,
            selectcolor=bg,
        ).pack(side=tk.LEFT, padx=5, pady=2)

        return att_frame
