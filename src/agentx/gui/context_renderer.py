"""Context/history/working-memory widget renderer.

Stateless widget factory methods extracted from GUIManager.  All methods
receive the widgets they need as arguments and return constructed Tkinter
widgets — no persistent per-instance widget state.

Accessed via back-reference pattern: ``ContextRenderer(gui_manager)`` stores
``self._g = gui_manager`` and reads colours/config from there.
"""

from __future__ import annotations

import re
import tkinter as tk
from tkinter import messagebox as tk_messagebox
from typing import TYPE_CHECKING, Any, Callable, Optional

if TYPE_CHECKING:
    from .gui_manager import GUIManager


class ContextRenderer:
    """Renders Context, History and WorkingMemory objects as Tkinter widgets."""

    # ── Layout constants (mirrors GUIManager class attrs) ────────────────────
    EXPAND_COLLAPSE_ICONS: dict[bool, str] = {True: "▼", False: "▶"}
    MESSAGE_ROLES: dict[str, str] = {
        "user": "👤",
        "assistant": "🤖",
        "system": "⚙️",
        "thinking": "💭",
        "tool_call": "🔧",
        "tool_result": "📋",
        "tools": "🛠️",
        "plan": "📋",
        "task_node": "🌿",
    }
    MESSAGE_COLUMNS: dict[str, int] = {
        "exp_button": 0,
        "enabled": 1,
        "role": 2,
        "content": 3,
    }

    def __init__(self, gui_manager: "GUIManager") -> None:
        self._g = gui_manager

    # ── Convenience accessors ─────────────────────────────────────────────────

    @property
    def _config(self):
        return self._g.config

    @property
    def _section_bg(self) -> str:
        return self._g._section_bg

    # ── Public widget-factory methods ─────────────────────────────────────────

    def collapse_expand_button(
        self,
        parent: tk.Widget,
        expandable_frame: tk.Widget = None,
        attachment_rows=None,
    ) -> tk.Button:
        """Create a collapse/expand button."""
        if attachment_rows is None and expandable_frame is None:
            raise ValueError("Either expandable_frame or row_widgets must be provided")

        expanded_var = tk.BooleanVar(value=False)
        collapse_expand_button: tk.Button

        def toggle_expand():
            expanded = expanded_var.get()
            expanded_var.set(not expanded)
            collapse_expand_button.config(text=self.EXPAND_COLLAPSE_ICONS[expanded_var.get()])
            if expandable_frame:
                if expanded:
                    expandable_frame.grid_remove()
                else:
                    expandable_frame.grid(row=1, column=self.MESSAGE_COLUMNS["enabled"], sticky="w")
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
        history_obj,
        parent: tk.Widget,
        user_name: str,
        on_attachment_toggle=None,
        include_header: bool = False,
    ) -> tk.Frame:
        """Render a History object as a Tkinter Frame."""
        history_frame = tk.Frame(parent, bg=self._section_bg)

        if include_header:
            history_contexts_frame = tk.Frame(history_frame, bg=self._section_bg)
            btn = self.collapse_expand_button(history_frame, history_contexts_frame)
            history_label = tk.Label(
                history_frame,
                text=f"{user_name} History ({len(history_obj.sessions)} contexts)",
                font=("Terminal", 10, "bold"),
                bg=self._section_bg,
                fg=self._config.ui_fg,
            )
            btn.grid(row=0, column=self.MESSAGE_COLUMNS["exp_button"], sticky="w")
            history_label.grid(row=0, column=self.MESSAGE_COLUMNS["enabled"], sticky="w")
            history_contexts_frame.grid(row=1, column=self.MESSAGE_COLUMNS["exp_button"], sticky="nsew")
        else:
            history_contexts_frame = tk.Frame(history_frame, bg=self._section_bg)
            history_contexts_frame.pack(fill=tk.BOTH, expand=True)

        for idx, context in enumerate(history_obj.sessions):
            context.expanded = False
            c_frame = self.render_context_widget(
                context,
                history_contexts_frame,
                on_attachment_toggle=on_attachment_toggle,
                include_header=True,
            )
            if include_header:
                c_frame.grid(row=idx, column=self.MESSAGE_COLUMNS["exp_button"], sticky="nsew")
            else:
                c_frame.pack(fill=tk.X, expand=False)

        if include_header:
            history_contexts_frame.grid_remove()
        return history_frame

    def render_context_widget(
        self,
        context_obj,
        parent: tk.Widget,
        on_attachment_toggle=None,
        include_header: bool = False,
        on_plan_click=None,
    ) -> tk.Frame:
        """Render a Context object as a Tkinter Frame."""
        context_frame = tk.Frame(parent, bg=self._section_bg)
        context_messages_frame = tk.Frame(context_frame, bg=self._section_bg)

        if include_header:
            btn = self.collapse_expand_button(context_frame, context_messages_frame)
            context_label = tk.Label(
                context_frame,
                text=(
                    f"{getattr(context_obj, 'session_id', None) or 'Context'} "
                    f"({len(context_obj.messages)} messages)"
                ),
                font=("Terminal", 10, "bold"),
                bg=self._section_bg,
                fg=self._config.ui_fg,
            )
            btn.grid(row=0, column=self.MESSAGE_COLUMNS["exp_button"], sticky="w")
            context_label.grid(row=0, column=1, sticky="w")
            context_messages_frame.grid(row=1, column=self.MESSAGE_COLUMNS["enabled"], sticky="nsew")
        else:
            context_messages_frame.pack(fill=tk.BOTH, expand=True)

        context_messages_frame.columnconfigure(self.MESSAGE_COLUMNS["exp_button"], weight=0)
        context_messages_frame.columnconfigure(self.MESSAGE_COLUMNS["enabled"], weight=0)
        context_messages_frame.columnconfigure(self.MESSAGE_COLUMNS["role"], weight=0)
        context_messages_frame.columnconfigure(self.MESSAGE_COLUMNS["content"], weight=1)

        _TOOL_ROLES = {"tool_call", "tool_result", "plan", "task_node"}

        def _role_str(msg) -> str:
            r = getattr(msg, "role", "")
            return r.value if hasattr(r, "value") else str(r)

        grouped: list[tuple] = []
        for entry in context_obj.messages:
            msg = entry.message if hasattr(entry, "message") else entry
            if _role_str(msg) in _TOOL_ROLES:
                if grouped:
                    grouped[-1][1].append(msg)
            else:
                grouped.append((msg, []))

        current_row = 0
        for message, tool_msgs in grouped:
            current_row = self._render_message_to_grid(
                message,
                context_messages_frame,
                current_row,
                on_attachment_toggle,
                tool_msgs,
                on_plan_click=on_plan_click,
            )

        if include_header and not getattr(context_obj, "expanded", False):
            context_messages_frame.grid_remove()

        return context_frame

    def render_working_memory_widget(
        self,
        working_memory,
        parent: tk.Widget,
        on_toggle: "Callable[[str, bool], None] | None" = None,
        on_delete: "Callable[[str], None] | None" = None,
        on_promote: "Callable[[str], None] | None" = None,
        on_user_add: "Callable[[str, str], None] | None" = None,
    ) -> tk.Frame:
        """Render a WorkingMemory instance as a Tkinter widget tree."""
        from shared.models.working_memory import FactOwner  # noqa: F401 (triggers import check)

        outer = tk.Frame(parent, bg=self._section_bg)

        if working_memory is None or len(working_memory) == 0:
            tk.Label(
                outer,
                text="No facts stored yet.",
                bg=self._section_bg,
                fg=self._config.muted_fg,
                font=("Terminal", 9),
            ).pack(anchor="w", padx=4, pady=2)
        else:
            for fact in working_memory.all_facts():
                self._render_working_memory_row(
                    outer,
                    fact,
                    on_toggle=on_toggle,
                    on_delete=on_delete,
                    on_promote=on_promote,
                )

        sep = tk.Frame(outer, height=1, bg="#555555")
        sep.pack(fill=tk.X, padx=4, pady=(6, 2))

        add_frame = tk.Frame(outer, bg=self._section_bg)
        add_frame.pack(fill=tk.X, padx=4, pady=2)

        tk.Label(
            add_frame,
            text="👤 Add fact:",
            bg=self._section_bg,
            fg=self._config.ui_fg,
            font=("Terminal", 9),
        ).grid(row=0, column=0, sticky="w")

        key_var = tk.StringVar()
        val_var = tk.StringVar()

        tk.Label(add_frame, text="key", bg=self._section_bg, fg=self._config.ui_fg, font=("Terminal", 8)).grid(
            row=1, column=0, sticky="w"
        )
        tk.Entry(add_frame, textvariable=key_var, width=18, font=("Terminal", 9)).grid(
            row=1, column=1, sticky="ew", padx=2
        )

        tk.Label(add_frame, text="value", bg=self._section_bg, fg=self._config.ui_fg, font=("Terminal", 8)).grid(
            row=2, column=0, sticky="w"
        )
        tk.Entry(add_frame, textvariable=val_var, width=28, font=("Terminal", 9)).grid(
            row=2, column=1, sticky="ew", padx=2
        )
        add_frame.columnconfigure(1, weight=1)

        def _submit_add():
            k = key_var.get().strip()
            v = val_var.get().strip()
            if k and on_user_add:
                on_user_add(k, v)
                key_var.set("")
                val_var.set("")

        tk.Button(add_frame, text="Add 👤", font=("Terminal", 9), command=_submit_add).grid(
            row=3, column=1, sticky="e", pady=2
        )

        return outer

    # ── Private helpers ───────────────────────────────────────────────────────

    def _render_message_to_grid(
        self,
        message_obj,
        parent_frame: tk.Frame,
        start_row: int,
        on_attachment_toggle=None,
        tool_interactions: list | None = None,
        on_plan_click=None,
    ) -> int:
        """Render a Message into parent_frame's grid; returns next available row."""
        current_row = start_row
        tool_interactions = tool_interactions or []

        def _role_str_inner(m) -> str:
            r = getattr(m, "role", "")
            return r.value if hasattr(r, "value") else str(r)

        _REAL_TOOL_ROLES = {"tool_call", "tool_result"}
        real_tool_msgs = [m for m in tool_interactions if _role_str_inner(m) in _REAL_TOOL_ROLES]
        plan_msgs = [m for m in tool_interactions if _role_str_inner(m) == "plan"]
        task_node_msgs = [m for m in tool_interactions if _role_str_inner(m) == "task_node"]

        has_attachments = bool(getattr(message_obj, "attachments", []))
        has_tools = bool(real_tool_msgs)
        has_plans = bool(plan_msgs or task_node_msgs)

        # Every message is expandable — plain messages show full content in the
        # detail row; tool/attachment/plan rows are appended to the same list.
        collapsible_rows: list[list[tk.Widget]] = []
        collapse_expand_button = self.collapse_expand_button(parent=parent_frame, attachment_rows=collapsible_rows)
        collapse_expand_button.grid(row=current_row, column=self.MESSAGE_COLUMNS["exp_button"], sticky="nsew")

        enabled_var = tk.BooleanVar(value=getattr(message_obj, "enabled", True))

        def on_enabled_toggle():
            message_obj.enabled = enabled_var.get()

        enabled_checkbox = tk.Checkbutton(
            parent_frame,
            variable=enabled_var,
            command=on_enabled_toggle,
            bg=self._section_bg,
            fg=self._config.ui_fg,
            activebackground=self._section_bg,
            activeforeground=self._config.ui_fg,
            selectcolor=self._section_bg,
        )
        enabled_checkbox.grid(row=current_row, column=self.MESSAGE_COLUMNS["enabled"], sticky="nsew")

        role_value = getattr(message_obj, "role", "system")
        role_key = role_value.value if hasattr(role_value, "value") else role_value
        role_label = tk.Label(
            parent_frame,
            text=self.MESSAGE_ROLES.get(role_key, "⚙️"),
            bg=self._section_bg,
            fg=self._config.ui_fg,
        )
        role_label.grid(row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nsew")

        trimmed_content = getattr(message_obj, "content", "").strip()
        lines = [
            line
            for line in trimmed_content.splitlines()
            if not re.match(r"--- \[Attached file: .+\] ---", line) and not re.match(r"--- \[End of .+\] ---", line)
        ]
        preview_text = " ".join([ln.strip() for ln in lines if ln.strip()])
        if has_tools:
            preview_text += f"  [{len(real_tool_msgs)} tool interaction{'s' if len(real_tool_msgs) != 1 else ''}]"
        if has_plans:
            preview_text += f"  [{len(plan_msgs)} plan{'s' if len(plan_msgs) != 1 else ''}]"
        preview = preview_text[:60] + ("..." if len(preview_text) > 60 else "")
        preview_label = tk.Label(
            parent_frame,
            text=preview,
            anchor="w",
            width=50,
            bg=self._section_bg,
            fg=self._config.ui_fg,
        )
        preview_label.grid(row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew")
        current_row += 1

        # Full-content detail row — always created, starts hidden (collapsed).
        if trimmed_content:
            full_text_label = tk.Label(
                parent_frame,
                text=trimmed_content,
                anchor="nw",
                justify=tk.LEFT,
                bg=self._section_bg,
                fg=self._config.ui_fg,
                wraplength=360,
                font=("Terminal", 9),
            )
            full_text_label.grid(
                row=current_row,
                column=self.MESSAGE_COLUMNS["content"],
                sticky="nsew",
                padx=(12, 0),
                pady=(0, 2),
            )
            full_text_label.grid_remove()
            collapsible_rows.append([full_text_label])
            current_row += 1

        if has_attachments:
            for att in getattr(message_obj, "attachments", []):
                row_widgets: list[tk.Widget] = []
                att_enabled_var = tk.BooleanVar(value=getattr(att, "enabled", True))

                def toggle(var=att_enabled_var, a=att, callback=on_attachment_toggle):
                    a.enabled = var.get()
                    if callback:
                        callback(a, a.enabled)

                att_checkbox = tk.Checkbutton(
                    parent_frame,
                    variable=att_enabled_var,
                    command=toggle,
                    bg=self._section_bg,
                    fg=self._config.ui_fg,
                    activebackground=self._section_bg,
                    activeforeground=self._config.ui_fg,
                    selectcolor=self._section_bg,
                )
                att_checkbox.grid(row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nsew")
                row_widgets.append(att_checkbox)

                att_label = tk.Label(
                    parent_frame,
                    text=f"📁  {getattr(att, 'file_path', '').split('/')[-1]}",
                    anchor="w",
                    bg=self._section_bg,
                    fg=self._config.ui_fg,
                )
                att_label.grid(row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew")
                row_widgets.append(att_label)

                for widget in row_widgets:
                    widget.grid_remove()
                collapsible_rows.append(row_widgets)
                current_row += 1

        if has_tools:
            current_row = self._g._render_tool_rows(real_tool_msgs, parent_frame, current_row, collapsible_rows)

        if has_plans:
            current_row = self._g._render_plan_rows(
                plan_msgs, task_node_msgs, parent_frame, current_row, collapsible_rows, on_plan_click
            )

        return current_row

    def _render_tool_rows(
        self,
        tool_msgs: list,
        parent_frame: tk.Frame,
        start_row: int,
        parent_collapsible: list[list[tk.Widget]],
    ) -> int:
        """Render TOOL_CALL/TOOL_RESULT messages as collapsible sub-rows."""
        import json as _json

        current_row = start_row

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

            header_row_widgets: list[tk.Widget] = []
            detail_rows: list[list[tk.Widget]] = []

            tool_btn = self.collapse_expand_button(parent=parent_frame, attachment_rows=detail_rows)
            tool_btn.grid(row=current_row, column=self.MESSAGE_COLUMNS["exp_button"], sticky="nsew")
            header_row_widgets.append(tool_btn)

            tool_icon = tk.Label(parent_frame, text="🔧", anchor="w", bg=self._section_bg, fg=self._config.ui_fg)
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
                foreground=self._config.muted_fg,
                font=("", 9),
                bg=self._section_bg,
            )
            tool_label.grid(row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew")
            header_row_widgets.append(tool_label)
            current_row += 1

            if call_input is not None:
                try:
                    input_str = _json.dumps(call_input, indent=2)
                except Exception:
                    input_str = str(call_input)

                in_icon = tk.Label(parent_frame, text="📥", anchor="w", bg=self._section_bg, fg=self._config.ui_fg)
                in_icon.grid(row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nw", padx=(12, 0))
                in_text = tk.Label(
                    parent_frame,
                    text=input_str,
                    anchor="w",
                    justify="left",
                    foreground="#4a9eff",
                    font=("Courier", 8),
                    wraplength=380,
                    bg=self._section_bg,
                )
                in_text.grid(row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew")
                in_row = [in_icon, in_text]
                for w in in_row:
                    w.grid_remove()
                detail_rows.append(in_row)
                current_row += 1

            if result_msg:
                out_raw = getattr(result_msg, "tool_output", None) or getattr(result_msg, "content", "")
                if isinstance(out_raw, (dict, list)):
                    out_str = _json.dumps(out_raw, indent=2)
                else:
                    out_str = str(out_raw) if out_raw else ""

                out_icon = tk.Label(parent_frame, text="📤", anchor="w", bg=self._section_bg, fg=self._config.ui_fg)
                out_icon.grid(row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nw", padx=(12, 0))
                out_text = tk.Label(
                    parent_frame,
                    text=out_str[:800] + ("…" if len(out_str) > 800 else ""),
                    anchor="w",
                    justify="left",
                    foreground="#5cb85c",
                    font=("Courier", 8),
                    wraplength=380,
                    bg=self._section_bg,
                )
                out_text.grid(row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew")
                out_row = [out_icon, out_text]
                for w in out_row:
                    w.grid_remove()
                detail_rows.append(out_row)
                current_row += 1

            for w in header_row_widgets:
                w.grid_remove()
            parent_collapsible.append(header_row_widgets)

        return current_row

    def _render_plan_rows(
        self,
        plan_msgs: list,
        task_node_msgs: list,
        parent_frame: tk.Frame,
        start_row: int,
        parent_collapsible: list[list[tk.Widget]],
        on_plan_click=None,
    ) -> int:
        """Render PLAN and TASK_NODE messages as collapsible sub-rows."""
        current_row = start_row

        nodes_by_plan: dict[str, list] = {}
        for msg in task_node_msgs:
            pid = getattr(msg, "plan_id", None) or ""
            nodes_by_plan.setdefault(pid, []).append(msg)

        for plan_msg in plan_msgs:
            plan_id = getattr(plan_msg, "plan_id", None) or ""
            plan_name = getattr(plan_msg, "plan_name", None) or getattr(plan_msg, "content", None) or "Plan"
            plan_nodes = nodes_by_plan.get(plan_id, [])
            enabled = getattr(plan_msg, "enabled", True)

            detail_rows: list[list[tk.Widget]] = []
            header_row_widgets: list[tk.Widget] = []

            plan_expand_btn = self.collapse_expand_button(parent=parent_frame, attachment_rows=detail_rows)
            plan_expand_btn.grid(row=current_row, column=self.MESSAGE_COLUMNS["exp_button"], sticky="nsew")
            header_row_widgets.append(plan_expand_btn)

            plan_enabled_var = tk.BooleanVar(value=enabled)

            def _on_plan_enabled(var=plan_enabled_var, msg=plan_msg):
                msg.enabled = var.get()

            plan_checkbox = tk.Checkbutton(
                parent_frame,
                variable=plan_enabled_var,
                command=_on_plan_enabled,
                bg=self._section_bg,
                fg=self._config.ui_fg,
                activebackground=self._section_bg,
                activeforeground=self._config.ui_fg,
                selectcolor=self._section_bg,
            )
            plan_checkbox.grid(row=current_row, column=self.MESSAGE_COLUMNS["enabled"], sticky="nsew")
            header_row_widgets.append(plan_checkbox)

            plan_icon = tk.Label(parent_frame, text="📋", anchor="w", bg=self._section_bg, fg=self._config.ui_fg)
            plan_icon.grid(row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nsew")
            header_row_widgets.append(plan_icon)

            step_count = len(plan_nodes)
            badge = f"  [{step_count} step{'s' if step_count != 1 else ''}]" if step_count else ""
            plan_label_text = f"{plan_name}{badge}"

            if on_plan_click and plan_id:
                plan_label: tk.Widget = tk.Button(
                    parent_frame,
                    text=plan_label_text,
                    anchor="w",
                    cursor="hand2",
                    relief=tk.FLAT,
                    font=("", 10, "bold"),
                    bg=self._section_bg,
                    fg=self._config.agent_classification_fg,
                    activebackground=self._section_bg,
                    activeforeground=self._config.agent_classification_fg,
                    command=lambda pid=plan_id: on_plan_click(pid),
                )
            else:
                plan_label = tk.Label(
                    parent_frame,
                    text=plan_label_text,
                    anchor="w",
                    font=("", 10, "bold"),
                    bg=self._section_bg,
                    fg=self._config.ui_fg,
                )
            plan_label.grid(row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew")
            header_row_widgets.append(plan_label)
            current_row += 1

            for node_msg in plan_nodes:
                node_row_widgets: list[tk.Widget] = []
                depth = getattr(node_msg, "task_depth", 0) or 0
                task_id = getattr(node_msg, "task_id", "") or ""
                synth = (getattr(node_msg, "content", "") or "").strip()
                node_enabled = getattr(node_msg, "enabled", True)

                task_data = getattr(node_msg, "task_data", None) or {}
                is_tbd = bool(task_data.get("tbd", False))
                icon = "?" if is_tbd else "🌿"
                indent = 4 + depth * 8

                node_enabled_var = tk.BooleanVar(value=node_enabled)

                def _on_node_enabled(var=node_enabled_var, msg=node_msg):
                    msg.enabled = var.get()

                node_checkbox = tk.Checkbutton(
                    parent_frame,
                    variable=node_enabled_var,
                    command=_on_node_enabled,
                    bg=self._section_bg,
                    fg=self._config.ui_fg,
                    activebackground=self._section_bg,
                    activeforeground=self._config.ui_fg,
                    selectcolor=self._section_bg,
                )
                node_checkbox.grid(
                    row=current_row, column=self.MESSAGE_COLUMNS["enabled"], sticky="nsew", padx=(indent, 0)
                )
                node_row_widgets.append(node_checkbox)

                node_icon_label = tk.Label(
                    parent_frame, text=icon, anchor="w", bg=self._section_bg, fg=self._config.muted_fg
                )
                node_icon_label.grid(row=current_row, column=self.MESSAGE_COLUMNS["role"], sticky="nsew")
                node_row_widgets.append(node_icon_label)

                desc_preview = synth[:50] + ("..." if len(synth) > 50 else "")
                node_font = ("", 9, "italic") if is_tbd else ("", 9)
                node_label = tk.Label(
                    parent_frame,
                    text=f"{task_id[-6:] if task_id else '?'} — \"{desc_preview}\"",
                    anchor="w",
                    font=node_font,
                    bg=self._section_bg,
                    fg=self._config.muted_fg,
                )
                node_label.grid(row=current_row, column=self.MESSAGE_COLUMNS["content"], sticky="nsew")
                node_row_widgets.append(node_label)

                for w in node_row_widgets:
                    w.grid_remove()
                detail_rows.append(node_row_widgets)
                current_row += 1

            for w in header_row_widgets:
                w.grid_remove()
            parent_collapsible.append(header_row_widgets)

        return current_row

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
            fg=self._config.ui_fg,
            activeforeground=self._config.ui_fg,
            selectcolor=self._section_bg,
        ).grid(row=0, column=0, sticky="w")

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
                fg=self._config.ui_fg,
                font=("Terminal", 10),
            ).grid(row=0, column=1, sticky="w", padx=(0, 2))

        preview = fact.value_preview()
        tk.Label(
            row_frame,
            text=f"{fact.key}: {preview}",
            bg=self._section_bg,
            fg=self._config.ui_fg,
            font=("Terminal", 9),
            anchor="w",
            justify="left",
        ).grid(row=0, column=2, sticky="ew", padx=2)
        row_frame.columnconfigure(2, weight=1)

        if is_agent:

            def _on_delete(ck=fact.compound_key):
                if on_delete and tk_messagebox.askyesno("Delete Fact", f"Remove agent fact '{fact.key}'?"):
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
        """Show promote-to-user confirmation dialog."""
        key = compound_key.split(":", 1)[-1] if ":" in compound_key else compound_key
        if tk_messagebox.askyesno(
            "Promote Fact",
            f"Promote 🤖 '{key}' to your own 👤 fact?\n\nYou will own this fact "
            "and the agent will no longer modify it automatically.",
        ):
            if on_promote:
                on_promote(compound_key)
