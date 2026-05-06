"""Input panel extracted from GUIManager.

Owns the user-text input area, attachment bar, and submit button.
Follows the back-reference pattern: ``InputPanel(gui_manager)``
stores ``self._g = gui_manager`` and accesses shared state from there.

Note: The interrupt (user_break) button and ContextMeterWidget have been
relocated to ``StatusTab`` (PD-12).  The submit button now occupies a slim
right-column strip (relx=0.96, relwidth=0.04).
"""

from __future__ import annotations

import threading
import tkinter as tk
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .gui_manager import GUIManager


class InputPanel:
    """Manages the bottom input area: text field, buttons, and attachment bar."""

    # Delay (ms) before showing any right-click context popup.  Must be > the
    # physical button-press duration so the ButtonRelease fires on the widget,
    # not the popup window.  Tests may override this to 0 for speed.
    _MENU_POST_DELAY_MS: int = 100

    def __init__(self, gui_manager: "GUIManager") -> None:
        self._g = gui_manager
        self._cached_user_input: str = ""
        # Input right-click context popup (PD-02-AF-008)
        self._input_context_popup: tk.Toplevel | None = None

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
        self._widgets.user_input_text.place(relx=0, rely=0, relwidth=0.96, relheight=1.0)
        self._widgets.input_scrollbar.place(relx=0.96, rely=0, relheight=1.0)

        # Submit button (slim strip — PD-12 freed the right column)
        self._widgets.user_submit = tk.Button(
            self._widgets.user_input,
            text=enter_emoji_unicode,
            command=self._on_submit_clicked,
        )
        self._widgets.user_submit.place(relx=0.97, rely=0, relwidth=0.03, relheight=1.0)
        # Keyboard shortcuts
        self._widgets.user_input_text.bind(
            "<Control-Return>",
            lambda _event: self._widgets.user_submit.invoke(),
        )
        self._widgets.user_input_text.bind(
            "<Shift-Return>",
            self._on_shift_return,
        )
        # Note: Ctrl+Space binding moved to StatusTab (PD-12)
        # Right-click context menu on user input (PD-02-AF-008)
        self._widgets.user_input_text.bind(
            "<Button-3>",
            self._on_input_right_click,
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
        """Update submit button state for streaming vs. idle."""
        if threading.current_thread() is not threading.main_thread():
            return
        try:
            submit = self._widgets.user_submit
            if submit is not None:
                submit.config(state=tk.DISABLED if is_streaming else tk.NORMAL)
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

    def _on_shift_return(self, event: tk.Event) -> str:  # type: ignore[type-arg]
        """Insert a newline at the insertion cursor (PD-02-AF-002).

        Bound to ``<Shift-Return>`` on ``user_input_text``.  Returns ``"break"``
        to suppress Tkinter's default ``<Return>`` handling which would otherwise
        also fire after Shift is released.

        Affordance ID: PD-02-AF-002
        """
        self._widgets.user_input_text.insert(tk.INSERT, "\n")
        return "break"

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

    # ── Input right-click context menu (PD-02-AF-008..012) ───────────────────

    def _on_input_right_click(self, event: tk.Event) -> str:  # type: ignore[type-arg]
        """Schedule the input context menu popup after button release (PD-02-AF-008).

        Uses after(_MENU_POST_DELAY_MS) so Button-3 is physically released before
        the popup appears, preventing the release event from immediately dismissing it.
        See UX_LIFECYCLE.md §6 for the rationale.

        Affordance ID: PD-02-AF-008
        """
        widget = self._widgets.user_input_text
        if widget is None:
            return "break"
        x_root = widget.winfo_rootx() + event.x
        y_root = widget.winfo_rooty() + event.y
        self._dismiss_input_context_popup()
        widget.after(
            self._MENU_POST_DELAY_MS,
            lambda x=x_root, y=y_root: self._show_input_context_menu(x, y),
        )
        return "break"

    def _dismiss_input_context_popup(self, _event: object = None) -> None:
        """Destroy the input context popup if it exists (PD-02-AF-008).

        Affordance ID: PD-02-AF-008
        """
        if self._input_context_popup is not None:
            try:
                if self._input_context_popup.winfo_exists():
                    self._input_context_popup.destroy()
            except tk.TclError:
                pass
            self._input_context_popup = None

    def _clipboard_has_content(self) -> bool:
        """Return True if the system clipboard contains non-empty text (PD-02-AF-010).

        Uses try/except around clipboard_get() to handle the TclError raised
        when the clipboard is empty.

        Affordance ID: PD-02-AF-010
        """
        try:
            content = self._widgets.user_input_text.clipboard_get()
            return bool(content)
        except tk.TclError:
            return False

    def _on_input_context_copy(self, widget: tk.Text) -> None:
        """Copy selected text to clipboard and dismiss popup (PD-02-AF-011).

        Affordance ID: PD-02-AF-011
        """
        try:
            widget.event_generate("<<Copy>>")
        finally:
            self._dismiss_input_context_popup()

    def _on_input_context_paste(self, widget: tk.Text) -> None:
        """Replace selection (or insert at cursor) with clipboard content (PD-02-AF-012).

        If text is currently selected the selection is deleted first, then the
        clipboard content is inserted at the INSERT index.  This gives reliable,
        test-verifiable behaviour rather than delegating to the <<Paste>> virtual
        event whose replace-selection semantics vary across platforms.

        Affordance ID: PD-02-AF-012
        """
        try:
            clipboard_text = widget.clipboard_get()
        except tk.TclError:
            self._dismiss_input_context_popup()
            return
        try:
            # Delete selection if present; reset INSERT to the deletion point so
            # that the pasted text lands at the former selection start rather than
            # wherever Tk drifts INSERT after the delete operation.
            if widget.tag_ranges(tk.SEL):
                sel_start = widget.index(tk.SEL_FIRST)
                widget.delete(tk.SEL_FIRST, tk.SEL_LAST)
                widget.mark_set(tk.INSERT, sel_start)
            widget.insert(tk.INSERT, clipboard_text)
        finally:
            self._dismiss_input_context_popup()

    def _show_input_context_menu(self, x_root: int, y_root: int) -> None:
        """Display the user-input right-click context popup (PD-02-AF-008..012).

        Always creates a fresh tk.Toplevel(overrideredirect=True) per invocation
        so each right-click gets a new compositor surface — avoids stale Wayland
        surfaces.

        Conditional items:
        - \"Copy\"  — shown only when text is selected (SEL tag present). AF-009/011.
        - \"Paste\" — shown only when clipboard is non-empty.              AF-010/012.

        If neither item is applicable the popup is not shown.

        Affordance ID: PD-02-AF-008
        """
        widget = self._widgets.user_input_text
        if widget is None:
            return

        has_selection = bool(widget.tag_ranges(tk.SEL))
        has_clipboard = self._clipboard_has_content()

        # Nothing to show — skip popup creation entirely
        if not has_selection and not has_clipboard:
            return

        popup_bg = self._config.input_bg
        popup_fg = self._config.input_fg
        active_bg = self._config.muted_fg
        active_fg = self._config.input_bg

        popup = tk.Toplevel(widget)
        popup.withdraw()
        popup.configure(bg=popup_bg, borderwidth=0, highlightthickness=0)
        popup.overrideredirect(True)
        popup.attributes("-topmost", True)
        self._input_context_popup = popup

        frame = tk.Frame(popup, bg=popup_bg, borderwidth=1, relief="solid")
        frame.pack(fill="both", expand=True)

        def _btn(label: str, command) -> None:
            tk.Button(
                frame,
                text=label,
                anchor="w",
                relief="flat",
                bd=0,
                padx=10,
                pady=6,
                bg=popup_bg,
                fg=popup_fg,
                activebackground=active_bg,
                activeforeground=active_fg,
                highlightthickness=0,
                command=command,
            ).pack(fill="x")

        if has_selection:
            _btn("Copy", lambda w=widget: self._on_input_context_copy(w))

        if has_clipboard:
            _btn("Paste", lambda w=widget: self._on_input_context_paste(w))

        popup.bind("<Escape>", self._dismiss_input_context_popup)

        def _on_outside_click(event: "tk.Event[tk.Toplevel]") -> None:
            """Dismiss popup when user clicks outside its bounds (PD-02-AF-008)."""
            if not (0 <= event.x <= popup.winfo_width() and 0 <= event.y <= popup.winfo_height()):
                self._dismiss_input_context_popup()

        popup.bind("<ButtonPress>", _on_outside_click)

        popup.update_idletasks()
        req_w = max(popup.winfo_reqwidth(), 80)
        req_h = max(popup.winfo_reqheight(), 28)
        popup.geometry(f"{req_w}x{req_h}+{x_root}+{y_root}")
        popup.deiconify()
        popup.lift()
        popup.grab_set()
