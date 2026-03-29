"""Modal dialog for re-triggering synthesis on a completed task node.

Opened when the user clicks ``[Re-synthesise]`` in the plan tree.
Shows the current synthesis, any assertion failures, and a free-text
hint field.  On confirm the caller-supplied ``on_confirm(hint)`` is
invoked on the Tkinter main thread.

A secondary ``[Add WM hint]`` button lets the user inject a working-memory
fact *before* triggering re-synthesis.  If provided, ``on_add_wm_hint`` is
called with ``(key, value)`` and the dialog remains open for re-synthesis.
"""

import tkinter as tk
from tkinter import messagebox
from typing import Callable, Optional


class ResynthesisDialog:
    """Modal Toplevel for re-synthesis with optional WM-hint injection.

    All widget construction happens on the calling thread, which must be
    the Tkinter main thread.

    Args:
        parent:           A live Tkinter widget (used as the dialog owner).
        task_id:          ID of the task node being re-synthesised.
        synthesis_text:   Current (accepted) synthesis text.
        failed_assertions: List of assertion dicts that have ``verified=False``.
        on_confirm:       Callable[hint: str] — called on confirm.
        on_add_wm_hint:   Optional Callable[key: str, value: str] — called when
                          user clicks *Add WM hint*.  If ``None`` the button is
                          hidden.
    """

    def __init__(
        self,
        parent: tk.Widget,
        task_id: str,
        synthesis_text: str,
        failed_assertions: list,
        on_confirm: Callable[[str], None],
        on_add_wm_hint: Optional[Callable[[str, str], None]] = None,
    ) -> None:
        self._task_id = task_id
        self._on_confirm = on_confirm
        self._on_add_wm_hint = on_add_wm_hint

        # ── Window setup ─────────────────────────────────────────────────────
        self._win = tk.Toplevel(parent)
        self._win.title(f"Re-synthesise — {task_id}")
        self._win.resizable(True, True)
        self._win.grab_set()  # modal
        self._win.transient(parent)
        self._win.configure(bg="#1e1e1e")
        self._win.geometry("640x520")

        fg = "#eeeeee"
        bg = "#1e1e1e"
        dim = "#888888"
        accent = "#7dd3fc"
        font_body = ("Courier New", 10)
        font_label = ("Courier New", 10, "bold")

        # ── Current synthesis (read-only scroll) ──────────────────────────────
        tk.Label(self._win, text="Current synthesis:", bg=bg, fg=accent, font=font_label, anchor="w").pack(
            fill=tk.X, padx=10, pady=(10, 2)
        )
        synth_frame = tk.Frame(self._win, bg=bg)
        synth_frame.pack(fill=tk.X, padx=10, pady=(0, 6))
        synth_scroll = tk.Scrollbar(synth_frame)
        synth_scroll.pack(side=tk.RIGHT, fill=tk.Y)
        self._synth_text = tk.Text(
            synth_frame,
            bg="#2a2a2a",
            fg=fg,
            font=font_body,
            height=6,
            wrap=tk.WORD,
            relief=tk.FLAT,
            yscrollcommand=synth_scroll.set,
            state=tk.NORMAL,
        )
        self._synth_text.insert("1.0", synthesis_text)
        self._synth_text.config(state=tk.DISABLED)
        self._synth_text.pack(side=tk.LEFT, fill=tk.X, expand=True)
        synth_scroll.config(command=self._synth_text.yview)

        # ── Assertion failures ────────────────────────────────────────────────
        if failed_assertions:
            tk.Label(self._win, text="Assertion failures:", bg=bg, fg="#f87171", font=font_label, anchor="w").pack(
                fill=tk.X, padx=10, pady=(4, 2)
            )
            fail_frame = tk.Frame(self._win, bg="#2a1a1a")
            fail_frame.pack(fill=tk.X, padx=10, pady=(0, 6))
            for assertion in failed_assertions:
                fact = assertion.get("fact", "")
                error = assertion.get("error", "")
                text = f"✗ {fact}" + (f": {error}" if error else "")
                tk.Label(
                    fail_frame, text=text, bg="#2a1a1a", fg="#f87171", font=font_body, anchor="w", wraplength=580
                ).pack(fill=tk.X, padx=6, pady=1)

        # ── Hint field ────────────────────────────────────────────────────────
        tk.Label(
            self._win, text="Hint for re-synthesis (optional):", bg=bg, fg=accent, font=font_label, anchor="w"
        ).pack(fill=tk.X, padx=10, pady=(4, 2))
        self._hint_text = tk.Text(
            self._win, bg="#2a2a2a", fg=fg, font=font_body, height=4, wrap=tk.WORD, relief=tk.FLAT
        )
        self._hint_text.pack(fill=tk.X, padx=10, pady=(0, 8))

        # ── WM hint sub-section (only when callback provided) ─────────────────
        if on_add_wm_hint is not None:
            wm_frame = tk.Frame(self._win, bg="#1a2030", relief=tk.FLAT)
            wm_frame.pack(fill=tk.X, padx=10, pady=(0, 6))
            tk.Label(wm_frame, text="Add working-memory fact:", bg="#1a2030", fg=dim, font=font_label, anchor="w").pack(
                fill=tk.X, padx=6, pady=(4, 2)
            )
            fields_frame = tk.Frame(wm_frame, bg="#1a2030")
            fields_frame.pack(fill=tk.X, padx=6, pady=(0, 4))
            tk.Label(fields_frame, text="Key:", bg="#1a2030", fg=dim, font=font_body, width=5, anchor="w").pack(
                side=tk.LEFT
            )
            self._wm_key_var = tk.StringVar()
            tk.Entry(
                fields_frame,
                textvariable=self._wm_key_var,
                bg="#2a2a2a",
                fg=fg,
                font=font_body,
                relief=tk.FLAT,
                width=18,
            ).pack(side=tk.LEFT, padx=(0, 8))
            tk.Label(fields_frame, text="Value:", bg="#1a2030", fg=dim, font=font_body, width=6, anchor="w").pack(
                side=tk.LEFT
            )
            self._wm_val_var = tk.StringVar()
            tk.Entry(
                fields_frame,
                textvariable=self._wm_val_var,
                bg="#2a2a2a",
                fg=fg,
                font=font_body,
                relief=tk.FLAT,
                width=28,
            ).pack(side=tk.LEFT)
            tk.Button(
                wm_frame,
                text="Add WM hint",
                bg="#2c3e50",
                fg=fg,
                font=font_body,
                relief=tk.FLAT,
                cursor="hand2",
                command=self._on_add_wm_hint_clicked,
            ).pack(pady=(0, 6))
        else:
            self._wm_key_var = tk.StringVar()
            self._wm_val_var = tk.StringVar()

        # ── Action buttons ────────────────────────────────────────────────────
        btn_frame = tk.Frame(self._win, bg=bg)
        btn_frame.pack(fill=tk.X, padx=10, pady=(4, 10))
        tk.Button(
            btn_frame,
            text="Re-synthesise",
            bg="#166534",
            fg="#ffffff",
            font=font_label,
            relief=tk.FLAT,
            cursor="hand2",
            padx=10,
            pady=4,
            command=self._on_confirm_clicked,
        ).pack(side=tk.LEFT, padx=(0, 8))
        tk.Button(
            btn_frame,
            text="Cancel",
            bg="#3a3a3a",
            fg=fg,
            font=font_body,
            relief=tk.FLAT,
            cursor="hand2",
            padx=8,
            pady=4,
            command=self._win.destroy,
        ).pack(side=tk.LEFT)

    # ── Event handlers ────────────────────────────────────────────────────────

    def _on_confirm_clicked(self) -> None:
        hint = self._hint_text.get("1.0", tk.END).strip()
        self._win.destroy()
        self._on_confirm(hint)

    def _on_add_wm_hint_clicked(self) -> None:
        if self._on_add_wm_hint is None:
            return
        key = self._wm_key_var.get().strip()
        value = self._wm_val_var.get().strip()
        if not key or not value:
            messagebox.showwarning("WM Hint", "Both key and value are required.", parent=self._win)
            return
        self._on_add_wm_hint(key, value)
        self._wm_key_var.set("")
        self._wm_val_var.set("")

    # ── Public helpers ────────────────────────────────────────────────────────

    def wait(self) -> None:
        """Block until the dialog is dismissed (convenience for tests)."""
        self._win.wait_window()
