"""Plan tree widget for displaying hierarchical task execution in the GUI.

Provides a scrollable, collapsible tree of plan steps, sub-tasks, tool
calls, and synthesis text with assertion badges — all updated live as
``ResponseChunk`` events arrive during streaming.
"""

import json
import tkinter as tk
from typing import Any

_INDENT_PX = 20  # horizontal pixels per depth level

_STATUS_ICONS: dict[str, str] = {
    "pending": "○",
    "running": "●",
    "done": "✓",
    "tbd": "?",
    "failed": "✗",
}


class PlanTreeWidget:
    """Scrollable tree view for a single plan's task nodes.

    Displays main-trunk steps and recursively nested sub-tasks, each with
    collapsible tool-call rows and a synthesis block with assertion badges.

    All mutating methods must be called on the Tkinter main thread.
    The caller (``GUIManager``) is responsible for scheduling via
    ``root.after()`` / ``_safe_root_after()``.
    """

    def __init__(
        self,
        parent: tk.Widget,
        bg: str = "#222222",
        fg: str = "#eeeeee",
        dim_fg: str = "#888888",
        accent_fg: str = "#7dd3fc",
        font: tuple = ("Courier New", 10),
    ) -> None:
        self._bg = bg
        self._fg = fg
        self._dim_fg = dim_fg
        self._accent_fg = accent_fg
        self._font = font

        # Outer container — pack via get_widget().
        self._outer = tk.Frame(parent, bg=bg)

        # Scrollable canvas + content frame.
        self._canvas = tk.Canvas(self._outer, bg=bg, highlightthickness=0, borderwidth=0)
        self._scrollbar = tk.Scrollbar(self._outer, command=self._canvas.yview)
        self._canvas.configure(yscrollcommand=self._scrollbar.set)
        self._scrollbar.pack(side=tk.RIGHT, fill=tk.Y)
        self._canvas.pack(side=tk.LEFT, expand=True, fill=tk.BOTH)

        self._content = tk.Frame(self._canvas, bg=bg)
        self._window = self._canvas.create_window((0, 0), window=self._content, anchor="nw")

        self._content.bind("<Configure>", self._on_frame_configure)
        self._canvas.bind("<Configure>", self._on_canvas_configure)
        self._canvas.bind("<Enter>", self._bind_mousewheel)
        self._canvas.bind("<Leave>", self._unbind_mousewheel)

        # task_id → node-info dict (see _add_node for keys).
        self._nodes: dict[str, dict[str, Any]] = {}

    # ── Public API ────────────────────────────────────────────────────────────

    def get_widget(self) -> tk.Widget:
        """Return the outer frame to pack inside the plan tab."""
        return self._outer

    def add_step_node(self, plan_id: str, task_id: str, description: str, tbd: bool) -> None:
        """Add a root-level (depth=0) plan step row to the tree."""
        self._add_node(task_id, description, depth=0, tbd=tbd, parent=self._content, initially_expanded=True)

    def add_subtask_node(self, task_id: str, parent_task_id: str, description: str, depth: int) -> None:
        """Add an indented sub-task row under its parent's details frame."""
        parent_node = self._nodes.get(parent_task_id)
        parent_frame = parent_node["details_frame"] if parent_node else self._content
        self._add_node(task_id, description, depth=depth, tbd=False, parent=parent_frame, initially_expanded=False)

    def update_node_status(self, task_id: str, status: str) -> None:
        """Update the status icon (○ ● ✓ etc.) for an existing node."""
        node = self._nodes.get(task_id)
        if node:
            node["status_label"].config(text=_STATUS_ICONS.get(status, "●"))

    def resolve_tbd_node(self, task_id: str, resolved_description: str) -> None:
        """Replace the TBD placeholder text with the resolved description."""
        node = self._nodes.get(task_id)
        if node:
            node["desc_label"].config(text=resolved_description, font=self._font, fg=self._fg)

    def add_tool_call_to_node(self, task_id: str, tool_name: str, tool_input: Any) -> None:
        """Append a collapsible tool-call row to the node's details frame."""
        node = self._nodes.get(task_id)
        if node:
            self._create_tool_row(node["details_frame"], tool_name, tool_input)

    def add_synthesis_to_node(self, task_id: str, synthesis_text: str, assertions: list) -> None:
        """Append synthesis text and assertion badges to the node's details frame."""
        node = self._nodes.get(task_id)
        if node:
            self._create_synthesis_block(node["details_frame"], synthesis_text, assertions)

    # ── Internal helpers ──────────────────────────────────────────────────────

    def _add_node(
        self,
        task_id: str,
        description: str,
        depth: int,
        tbd: bool,
        parent: tk.Widget,
        initially_expanded: bool,
    ) -> None:
        indent = depth * _INDENT_PX + 4
        container = tk.Frame(parent, bg=self._bg)
        container.pack(fill=tk.X, expand=False, padx=(indent, 0), pady=(2, 0))

        # Header row: connector glyph · status icon · description · toggle btn
        row = tk.Frame(container, bg=self._bg)
        row.pack(fill=tk.X, expand=False)

        connector = "├─" if depth > 0 else "●─"
        tk.Label(row, text=connector, bg=self._bg, fg=self._dim_fg, font=self._font).pack(side=tk.LEFT)

        icon = _STATUS_ICONS["tbd"] if tbd else _STATUS_ICONS["pending"]
        status_lbl = tk.Label(row, text=icon, bg=self._bg, fg=self._accent_fg, font=self._font, width=2)
        status_lbl.pack(side=tk.LEFT)

        desc_font = (self._font[0], self._font[1], "italic") if tbd else self._font
        desc_color = self._dim_fg if tbd else self._fg
        desc_text = f"[TBD] {description}" if tbd else description
        desc_lbl = tk.Label(
            row,
            text=desc_text,
            bg=self._bg,
            fg=desc_color,
            font=desc_font,
            anchor="w",
            justify=tk.LEFT,
            wraplength=600,
        )
        desc_lbl.pack(side=tk.LEFT, fill=tk.X, expand=True, padx=(4, 0))

        # Details frame (tool calls + synthesis + sub-tasks): expand/collapse.
        details_frame = tk.Frame(container, bg=self._bg)
        toggle_text = tk.StringVar(value="▾" if initially_expanded else "▸")
        if initially_expanded:
            details_frame.pack(fill=tk.X, expand=False, padx=(8, 0))

        def _toggle(df=details_frame, tv=toggle_text):
            if tv.get() == "▸":
                df.pack(fill=tk.X, expand=False, padx=(8, 0))
                tv.set("▾")
                self._update_scroll()
            else:
                df.pack_forget()
                tv.set("▸")

        toggle_btn = tk.Button(
            row,
            textvariable=toggle_text,
            bg=self._bg,
            fg=self._dim_fg,
            relief=tk.FLAT,
            bd=0,
            command=_toggle,
            font=self._font,
            cursor="hand2",
        )
        toggle_btn.pack(side=tk.RIGHT, padx=(0, 4))

        self._nodes[task_id] = {
            "container": container,
            "row": row,
            "status_label": status_lbl,
            "desc_label": desc_lbl,
            "details_frame": details_frame,
            "toggle_var": toggle_text,
        }
        self._update_scroll()

    def _create_tool_row(self, parent: tk.Widget, tool_name: str, tool_input: Any) -> None:
        """Create a collapsed tool-call row with an expand/collapse toggle."""
        outer = tk.Frame(parent, bg=self._bg)
        outer.pack(fill=tk.X, expand=False, pady=(1, 0))

        header = tk.Frame(outer, bg=self._bg)
        header.pack(fill=tk.X)

        detail_frame = tk.Frame(outer, bg=self._bg)
        # detail_frame NOT packed initially — collapsed by default.

        toggle_lbl = tk.Label(header, text="▸", bg=self._bg, fg=self._dim_fg, font=self._font)
        toggle_lbl.pack(side=tk.LEFT)

        def _toggle_tool():
            if not detail_frame.winfo_ismapped():
                detail_frame.pack(fill=tk.X, padx=(16, 0))
                toggle_lbl.config(text="▾")
                self._update_scroll()
            else:
                detail_frame.pack_forget()
                toggle_lbl.config(text="▸")

        toggle_lbl.bind("<Button-1>", lambda _e: _toggle_tool())

        tk.Label(header, text="🔧", bg=self._bg, fg=self._accent_fg, font=self._font).pack(side=tk.LEFT, padx=(2, 0))
        tk.Label(header, text=tool_name, bg=self._bg, fg=self._fg, font=self._font, anchor="w").pack(
            side=tk.LEFT, padx=(4, 0)
        )

        try:
            input_str = json.dumps(tool_input, ensure_ascii=False, indent=2)
        except Exception:
            input_str = str(tool_input)

        height = min(8, input_str.count("\n") + 2)
        detail_text = tk.Text(
            detail_frame,
            bg=self._bg,
            fg=self._dim_fg,
            font=self._font,
            height=height,
            wrap=tk.WORD,
            relief=tk.FLAT,
            state=tk.NORMAL,
        )
        detail_text.insert("1.0", input_str)
        detail_text.config(state=tk.DISABLED)
        detail_text.pack(fill=tk.X, expand=False, padx=(4, 0))
        self._update_scroll()

    def _create_synthesis_block(self, parent: tk.Widget, synthesis_text: str, assertions: list) -> None:
        """Create the synthesis text block with assertion badge row."""
        block = tk.Frame(parent, bg=self._bg)
        block.pack(fill=tk.X, expand=False, pady=(4, 2))

        tk.Label(
            block,
            text="📝 Synthesis:",
            bg=self._bg,
            fg=self._accent_fg,
            font=(self._font[0], self._font[1], "bold"),
            anchor="w",
        ).pack(fill=tk.X)

        height = max(3, synthesis_text.count("\n") + 2)
        synth_text = tk.Text(
            block,
            bg=self._bg,
            fg=self._fg,
            font=self._font,
            height=height,
            wrap=tk.WORD,
            relief=tk.FLAT,
            state=tk.NORMAL,
        )
        synth_text.insert("1.0", synthesis_text)
        synth_text.config(state=tk.DISABLED)
        synth_text.pack(fill=tk.X, expand=False)

        if assertions:
            badge_frame = tk.Frame(block, bg=self._bg)
            badge_frame.pack(fill=tk.X, pady=(2, 0))
            for assertion in assertions:
                self._create_assertion_badge(badge_frame, assertion)

        self._update_scroll()

    def _create_assertion_badge(self, parent: tk.Widget, assertion: dict) -> None:
        """Render a single pass/fail/unknown assertion badge."""
        verified = assertion.get("verified")
        fact = assertion.get("fact", "")
        error = assertion.get("error") or ""

        if verified is True:
            icon, color = "✓", "#4ade80"
        elif verified is False:
            icon, color = "✗", "#f87171"
        else:
            icon, color = "?", "#fbbf24"

        short_fact = fact[:50] + ("..." if len(fact) > 50 else "")
        label_text = f" {icon} {short_fact} "
        if error and verified is False:
            label_text = f" {icon} {short_fact}: {error[:30]} "

        tk.Label(
            parent,
            text=label_text,
            bg=color,
            fg="#111111",
            font=(self._font[0], max(8, self._font[1] - 1)),
            relief=tk.FLAT,
            padx=4,
            pady=1,
        ).pack(side=tk.LEFT, padx=(0, 4))

    # ── Scroll helpers ────────────────────────────────────────────────────────

    def _on_frame_configure(self, _event) -> None:
        self._canvas.configure(scrollregion=self._canvas.bbox("all"))

    def _on_canvas_configure(self, event) -> None:
        self._canvas.itemconfig(self._window, width=event.width)

    def _update_scroll(self) -> None:
        try:
            self._content.update_idletasks()
            self._canvas.configure(scrollregion=self._canvas.bbox("all"))
            self._canvas.yview_moveto(1.0)
        except tk.TclError:
            pass

    def _bind_mousewheel(self, _event) -> None:
        self._canvas.bind_all("<MouseWheel>", self._on_mousewheel)
        self._canvas.bind_all("<Button-4>", self._on_mousewheel)
        self._canvas.bind_all("<Button-5>", self._on_mousewheel)

    def _unbind_mousewheel(self, _event) -> None:
        self._canvas.unbind_all("<MouseWheel>")
        self._canvas.unbind_all("<Button-4>")
        self._canvas.unbind_all("<Button-5>")

    def _on_mousewheel(self, event) -> None:
        if event.num == 4:
            self._canvas.yview_scroll(-1, "units")
        elif event.num == 5:
            self._canvas.yview_scroll(1, "units")
        else:
            self._canvas.yview_scroll(int(-1 * (event.delta / 120)), "units")
