"""Chat output panel extracted from GUIManager.

Owns the output notebook, structured-entry display, plan-tree tabs, and all
streaming display methods (display_user_message, display_agent_response, …).

Accessed via back-reference pattern: ``ChatPanel(gui_manager)`` stores
``self._g = gui_manager`` and reads widget registry / config from there.
"""

from __future__ import annotations

import math
import threading
import tkinter as tk
import tkinter.font as tkfont
from tkinter import ttk
from typing import TYPE_CHECKING, Any, Callable, Optional

from .markdown_renderer import (
    MARKDOWN_AVAILABLE,
    TKINTERWEB_AVAILABLE,
    HtmlFrame,
    build_markdown_css,
    has_markdown,
    markdown_to_html,
)
from .plan_tree_widget import PlanTreeWidget

if TYPE_CHECKING:
    from .gui_manager import GUIManager


class ChatPanel:
    """Manages the output display notebook, chat entries, and plan tree tabs."""

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
    _FINALIZE_SKIP_ROLES = {"Tool", "Error", "Classification"}

    def __init__(self, gui_manager: "GUIManager") -> None:
        self._g = gui_manager

        # Structured output state
        self._current_turn_frame: Optional[tk.Frame] = None
        self._current_turn_children_frame: Optional[tk.Frame] = None
        self._current_turn_entries: dict[str, dict[str, Any]] = {}
        self._output_wraplength: int = 1200
        self._output_wrapped_labels: list[tk.Label] = []
        self._output_detail_text_widgets: list[tk.Text] = []
        self._output_html_frames: list = []

        # Streaming display state flags
        self._agent_thinking_started: bool = False
        self._agent_response_started: bool = False
        self._agent_classification_shown: bool = False

        # Plan tree state
        self._plan_trees: dict[str, PlanTreeWidget] = {}
        self._task_to_plan: dict[str, str] = {}

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

    @property
    def _COLOR_AGENT_RESPONSE(self) -> str:
        return self._g.COLOR_AGENT_RESPONSE

    # ── Panel creation ────────────────────────────────────────────────────────

    def create(self) -> None:
        """Create output display widgets and register them on the widget registry."""
        text_font = self._text_font or self._config.default_font

        self._widgets.paned = tk.PanedWindow(self._g.root, orient=tk.HORIZONTAL, sashrelief=tk.RAISED)
        self._widgets.paned.place(relx=0.001, rely=0.001, relwidth=0.99, relheight=0.77)

        self._widgets.output_display = tk.Frame(self._widgets.paned, bg=self._config.output_bg)
        self._widgets.output_notebook = ttk.Notebook(self._widgets.output_display)
        self._widgets.output_notebook.pack(expand=True, fill=tk.BOTH, padx=0, pady=0)

        self._widgets.output_tab = tk.Frame(self._widgets.output_notebook, bg=self._config.output_bg)
        self._widgets.output_notebook.add(self._widgets.output_tab, text="Output")

        _hidden_text_container = tk.Frame(self._widgets.output_display, bg=self._config.output_bg)

        self._widgets.output_entries_container = tk.Frame(self._widgets.output_tab, bg=self._config.output_bg)
        self._widgets.output_entries_container.pack(side=tk.LEFT, expand=True, fill=tk.BOTH)

        self._widgets.output_entries_canvas = tk.Canvas(
            self._widgets.output_entries_container,
            bg=self._config.output_bg,
            highlightthickness=0,
            borderwidth=0,
        )
        self._widgets.output_entries_scrollbar = tk.Scrollbar(
            self._widgets.output_entries_container,
            command=self._widgets.output_entries_canvas.yview,
        )
        self._widgets.output_entries_canvas.configure(yscrollcommand=self._widgets.output_entries_scrollbar.set)
        self._widgets.output_entries_scrollbar.pack(side=tk.RIGHT, fill=tk.Y)
        self._widgets.output_entries_canvas.pack(side=tk.LEFT, expand=True, fill=tk.BOTH)

        self._widgets.output_entries_frame = tk.Frame(self._widgets.output_entries_canvas, bg=self._config.output_bg)
        output_window = self._widgets.output_entries_canvas.create_window(
            (0, 0), window=self._widgets.output_entries_frame, anchor="nw"
        )

        def _on_output_frame_configure(_event):
            if self._widgets.output_entries_canvas is not None:
                self._widgets.output_entries_canvas.configure(
                    scrollregion=self._widgets.output_entries_canvas.bbox("all")
                )

        def _on_output_canvas_configure(event):
            if self._widgets.output_entries_canvas is not None:
                self._widgets.output_entries_canvas.itemconfig(output_window, width=event.width)
                self._update_output_wraplength(event.width)

        self._widgets.output_entries_frame.bind("<Configure>", _on_output_frame_configure)
        self._widgets.output_entries_canvas.bind("<Configure>", _on_output_canvas_configure)

        # Hidden text mirror (backward compat; never displayed)
        self._widgets.output_scrollbar = tk.Scrollbar(_hidden_text_container)
        output_xscrollbar = tk.Scrollbar(_hidden_text_container, orient=tk.HORIZONTAL)
        self._widgets.output_text = tk.Text(
            _hidden_text_container,
            wrap=tk.WORD,
            font=text_font,
            yscrollcommand=self._widgets.output_scrollbar.set,
            xscrollcommand=output_xscrollbar.set,
            bg=self._config.output_bg,
            fg=self._config.agent_response_fg,
            insertbackground=self._config.agent_response_fg,
        )
        self._widgets.output_scrollbar.config(command=self._widgets.output_text.yview)
        output_xscrollbar.config(command=self._widgets.output_text.xview)
        self._widgets.output_scrollbar.pack(side=tk.RIGHT, fill=tk.Y)
        output_xscrollbar.pack(side=tk.BOTTOM, fill=tk.X)
        self._widgets.output_text.pack(side=tk.LEFT, expand=True, fill=tk.BOTH)
        self._widgets.output_text.bind("<Key>", lambda _event: "break")
        self._bind_output_text_shortcuts()
        self._widgets.output_text.tag_config("sel", background="#3399ff", foreground="#ffffff")

        self._widgets.paned.add(self._widgets.output_display, stretch="always")

    def configure_text_styles(self) -> None:
        """Configure text widget tags for styling (called after create())."""
        output = self._widgets.output_text
        if output is None:
            return
        output.tag_config("gray", font=self._config.gray_text_font, foreground=self._config.muted_fg)
        output.tag_config("user_prompt", font=self._config.user_prompt_font, foreground=self._g.COLOR_USER_PROMPT)
        output.tag_config(
            "agent_response", font=self._config.agent_response_font, foreground=self._g.COLOR_AGENT_RESPONSE
        )
        output.tag_config(
            "agent_thinking", font=self._config.agent_thinking_font, foreground=self._g.COLOR_AGENT_THINKING
        )
        output.tag_config(
            "agent_classification",
            font=self._config.agent_thinking_font,
            foreground=self._g.COLOR_AGENT_CLASSIFICATION,
        )
        output.tag_config("system_space", font=self._config.default_font, foreground=self._g.COLOR_SYSTEM_SPACE)

    # ── Display methods ───────────────────────────────────────────────────────

    def display_user_message(self, content: str, attachments: list[str], timestamp) -> None:
        if threading.current_thread() is not threading.main_thread():
            return
        if self._widgets.output_text is None:
            return

        self._ensure_turn_started(content)
        self._legacy_output_insert(f"{self.MESSAGE_ROLES['user']} User: {content}\n", ("user_prompt",))
        self._agent_thinking_started = False
        self._agent_response_started = False
        self._agent_classification_shown = False

        if attachments:
            attachment_lines = []
            for filename in attachments:
                attachment_line = f"[Attached file: {filename}]"
                attachment_lines.append(attachment_line)
                self._legacy_output_insert(f"\n{attachment_line}\n", ("gray",))
            if "user" in self._current_turn_entries:
                self._append_output_entry_text(
                    self._current_turn_entries["user"],
                    "\n" + "\n".join(attachment_lines),
                )

        self._scroll_output_to_end()

    def display_agent_thinking(self, content: str) -> None:
        if threading.current_thread() is not threading.main_thread():
            return
        if self._widgets.output_text is None:
            return

        self._legacy_output_insert(content, ("agent_thinking",))

        if "(The agent is thinking...)" in content:
            self._ensure_child_entry(
                key="thinking",
                role_label="Thinking",
                icon=self.MESSAGE_ROLES["thinking"],
                initial_text="",
                expanded=False,
            )
            self._scroll_output_to_end()
            return

        entry = self._ensure_child_entry(
            key="thinking",
            role_label="Thinking",
            icon=self.MESSAGE_ROLES["thinking"],
            initial_text="",
            expanded=False,
        )
        if entry is not None:
            self._append_output_entry_text(entry, content)
        self._scroll_output_to_end()

    def display_classification(self, classification: dict) -> None:
        if threading.current_thread() is not threading.main_thread():
            return
        output = self._widgets.output_text
        if output is None or self._agent_classification_shown:
            return

        self._agent_classification_shown = True
        tag = ("agent_classification",)

        intent = classification.get("intent", "")
        reasoning = classification.get("reasoning_summary", "")
        needs_clarification = classification.get("needs_clarification", False)
        missing_fields: list = classification.get("missing_fields") or []
        next_step = classification.get("next_step", "")

        lines: list[str] = []
        if intent:
            lines.append(f"🤔 intent: {intent}")
        if reasoning:
            lines.append(f"   reasoning: {reasoning}")
        if needs_clarification or missing_fields:
            clarification_line = "   clarification needed: yes"
            if missing_fields:
                clarification_line += f"  |  missing fields: {', '.join(missing_fields)}"
            lines.append(clarification_line)
        if next_step:
            lines.append(f"💡 path: {next_step}")

        block = "\n".join(lines)
        if block:
            self._legacy_output_insert(block + "\n", tag)
            entry = self._ensure_child_entry(
                key="classification",
                role_label="Classification",
                icon="🤔",
                initial_text=block,
                expanded=False,
            )
            if entry is not None:
                self._set_entry_text(entry, block)
            self._scroll_output_to_end()

    def display_agent_response(self, content: str) -> None:
        if threading.current_thread() is not threading.main_thread():
            return
        if self._widgets.output_text is None:
            return
        if not self._agent_response_started:
            self._agent_response_started = True

        self._legacy_output_insert(content, ("agent_response",))

        if self._display_tool_line(content):
            self._scroll_output_to_end()
            return

        if f"{self.MESSAGE_ROLES['assistant']} (" in content:
            self._ensure_child_entry(
                key="assistant",
                role_label="Agent",
                icon=self.MESSAGE_ROLES["assistant"],
                initial_text="",
                expanded=True,
            )
            self._scroll_output_to_end()
            return

        entry = self._ensure_child_entry(
            key="assistant",
            role_label="Agent",
            icon=self.MESSAGE_ROLES["assistant"],
            initial_text="",
            expanded=True,
        )
        if entry is not None:
            self._append_output_entry_text(entry, content)
        self._scroll_output_to_end()

    def display_bootstrap_agent_response(self, content: str) -> None:
        if threading.current_thread() is not threading.main_thread():
            return
        if self._widgets.output_text is None:
            return

        clean_content = content.strip()
        if not clean_content:
            return

        self._legacy_output_insert(f"{self.MESSAGE_ROLES['assistant']} Agent: {clean_content}\n\n", ("agent_response",))

        if self._widgets.output_entries_frame is not None:
            container = tk.Frame(self._widgets.output_entries_frame, bg=self._config.output_bg)
            container.pack(fill=tk.X, anchor="w", pady=(4, 6))
            self._create_output_entry(
                parent=container,
                role_label="Agent",
                icon=self.MESSAGE_ROLES["assistant"],
                content=clean_content,
                expanded=True,
            )

        self._current_turn_frame = None
        self._current_turn_children_frame = None
        self._current_turn_entries = {}
        self._scroll_output_to_end()

    def display_startup_notice(self, content: str) -> None:
        """Render a one-shot informational startup notice in the output panel."""
        if threading.current_thread() is not threading.main_thread():
            return
        if self._widgets.output_text is None:
            return

        clean_content = content.strip()
        if not clean_content:
            return

        self._legacy_output_insert(f"ⓘ Startup: {clean_content}\n\n", ("gray",))

        if self._widgets.output_entries_frame is not None:
            container = tk.Frame(self._widgets.output_entries_frame, bg=self._config.output_bg)
            container.pack(fill=tk.X, anchor="w", pady=(4, 6))

            text_font = self._text_font or self._config.default_font
            base_size = 10
            if isinstance(text_font, tuple) and len(text_font) >= 2 and isinstance(text_font[1], int):
                base_size = text_font[1]
            icon_font = (text_font[0], base_size + 2, "bold") if isinstance(text_font, tuple) else ("Terminal", 12, "bold")

            header_frame = tk.Frame(container, bg=self._config.output_bg)
            header_frame.pack(fill=tk.X, anchor="w")

            icon_label = tk.Label(
                header_frame,
                text="ⓘ",
                font=icon_font,
                bg=self._config.output_bg,
                fg=self._COLOR_AGENT_RESPONSE,
                anchor="w",
            )
            icon_label.pack(side=tk.LEFT, padx=(0, 6))
            title_label = tk.Label(
                header_frame,
                text="Startup:",
                font=(text_font[0], base_size, "bold") if isinstance(text_font, tuple) else ("Terminal", 10, "bold"),
                bg=self._config.output_bg,
                fg=self._COLOR_AGENT_RESPONSE,
                anchor="w",
            )
            title_label.pack(side=tk.LEFT)

            detail_label = tk.Label(
                container,
                text=clean_content,
                bg=self._config.output_bg,
                fg=self._COLOR_AGENT_RESPONSE,
                anchor="w",
                justify=tk.LEFT,
                wraplength=self._output_wraplength,
                font=text_font,
            )
            detail_label.pack(fill=tk.X, anchor="w", padx=(24, 0))
            self._output_wrapped_labels.append(detail_label)

        self._current_turn_frame = None
        self._current_turn_children_frame = None
        self._current_turn_entries = {}
        self._scroll_output_to_end()

    def display_error(self, message: str) -> None:
        if threading.current_thread() is not threading.main_thread():
            return
        if self._widgets.output_text is None:
            return

        self._legacy_output_insert(f"\n⚠️  ERROR: {message}\n\n", ("gray",))
        entry = self._ensure_child_entry(
            key="error", role_label="Error", icon="⚠️", initial_text=message, expanded=True
        )
        if entry is not None:
            self._set_entry_text(entry, message)
        self._scroll_output_to_end()

    def display_spacing(self) -> None:
        if threading.current_thread() is not threading.main_thread():
            return
        if self._widgets.output_text is None:
            return

        self.finalize_current_turn_markdown()
        self._legacy_output_insert("\n\n", ("system_space",))
        self._current_turn_frame = None
        self._current_turn_children_frame = None
        self._current_turn_entries = {}
        self._agent_thinking_started = False
        self._agent_response_started = False
        self._agent_classification_shown = False
        self._scroll_output_to_end()

    def finalize_current_turn_markdown(self) -> None:
        for entry in self._current_turn_entries.values():
            if entry is not None:
                self._finalize_entry_markdown(entry)

    # ── Plan tab methods ──────────────────────────────────────────────────────

    def add_plan_tab(self, plan_id: str, plan_name: str, on_export: Optional[Callable[[], None]] = None) -> tk.Frame:
        if plan_id in self._plan_trees:
            return self._widgets.plan_tabs[plan_id]

        if self._widgets.output_notebook is None:
            raise RuntimeError("output_notebook not yet created")

        tab_frame = tk.Frame(self._widgets.output_notebook, bg=self._config.output_bg)
        self._widgets.output_notebook.add(tab_frame, text=f"📋 {plan_name}")
        self._widgets.plan_tabs[plan_id] = tab_frame

        toolbar = tk.Frame(tab_frame, bg=self._config.output_bg)
        toolbar.pack(fill=tk.X, side=tk.TOP, padx=4, pady=(4, 2))
        tk.Label(
            toolbar,
            text=f"📋 {plan_name}",
            bg=self._config.output_bg,
            fg=self._config.agent_response_fg,
            font=(
                self._config.default_font[0] if self._config.default_font else "Courier New",
                11,
                "bold",
            ),
            anchor="w",
        ).pack(side=tk.LEFT)
        tk.Button(
            toolbar,
            text="Replay",
            bg=self._g._section_bg,
            fg=self._config.ui_fg,
            relief=tk.FLAT,
            cursor="hand2",
            command=lambda: None,
        ).pack(side=tk.RIGHT, padx=(4, 0))
        tk.Button(
            toolbar,
            text="Export",
            bg=self._g._section_bg,
            fg=self._config.ui_fg,
            relief=tk.FLAT,
            cursor="hand2",
            command=on_export or (lambda: None),
        ).pack(side=tk.RIGHT)

        tree = PlanTreeWidget(
            parent=tab_frame,
            bg=self._config.output_bg,
            fg=self._config.agent_response_fg,
            dim_fg=self._config.system_space_fg,
            accent_fg=self._config.agent_classification_fg,
        )
        tree.get_widget().pack(expand=True, fill=tk.BOTH)
        self._plan_trees[plan_id] = tree
        return tab_frame

    def get_plan_tab_frame(self, plan_id: str) -> Optional[tk.Frame]:
        return self._widgets.plan_tabs.get(plan_id)

    def focus_plan_tab(self, plan_id: str) -> None:
        tab_frame = self._widgets.plan_tabs.get(plan_id)
        if tab_frame is not None and self._widgets.output_notebook is not None:
            self._widgets.output_notebook.select(tab_frame)

    def add_plan_step_node(
        self,
        plan_id: str,
        task_id: str,
        description: str,
        tbd: bool,
        on_replay: Optional[Callable[[str], None]] = None,
    ) -> None:
        tree = self._plan_trees.get(plan_id)
        if tree:
            self._task_to_plan[task_id] = plan_id
            tree.add_step_node(plan_id, task_id, description, tbd, on_replay=on_replay)

    def add_plan_subtask_node(
        self,
        task_id: str,
        parent_task_id: str,
        description: str,
        depth: int,
        on_replay: Optional[Callable[[str], None]] = None,
    ) -> None:
        plan_id = self._task_to_plan.get(parent_task_id)
        if plan_id is None:
            for pid, tree in self._plan_trees.items():
                if parent_task_id in tree._nodes:
                    plan_id = pid
                    break
        if plan_id is None:
            return
        tree = self._plan_trees.get(plan_id)
        if tree:
            self._task_to_plan[task_id] = plan_id
            tree.add_subtask_node(task_id, parent_task_id, description, depth, on_replay=on_replay)

    def update_plan_node_status(self, task_id: str, status: str) -> None:
        plan_id = self._task_to_plan.get(task_id)
        tree = self._plan_trees.get(plan_id) if plan_id else None
        if tree is None:
            for t in self._plan_trees.values():
                if task_id in t._nodes:
                    tree = t
                    break
        if tree:
            tree.update_node_status(task_id, status)

    def resolve_plan_tbd_node(self, task_id: str, resolved_description: str) -> None:
        plan_id = self._task_to_plan.get(task_id)
        tree = self._plan_trees.get(plan_id) if plan_id else None
        if tree:
            tree.resolve_tbd_node(task_id, resolved_description)

    def add_plan_tool_call(self, task_id: str, tool_name: str, tool_input: Any) -> None:
        plan_id = self._task_to_plan.get(task_id)
        tree = self._plan_trees.get(plan_id) if plan_id else None
        if tree:
            tree.add_tool_call_to_node(task_id, tool_name, tool_input)

    def add_plan_synthesis(
        self,
        task_id: str,
        synthesis_text: str,
        assertions: list,
        on_resynth=None,
        on_add_wm_hint=None,
    ) -> None:
        plan_id = self._task_to_plan.get(task_id)
        tree = self._plan_trees.get(plan_id) if plan_id else None
        if tree:
            tree.add_synthesis_to_node(
                task_id, synthesis_text, assertions, on_resynth=on_resynth, on_add_wm_hint=on_add_wm_hint
            )

    def update_plan_synthesis(self, task_id: str, new_synthesis: str, assertions: list) -> None:
        plan_id = self._task_to_plan.get(task_id)
        tree = self._plan_trees.get(plan_id) if plan_id else None
        if tree:
            tree.update_synthesis_on_node(task_id, new_synthesis, assertions)

    def mark_plan_node_invalidated(self, task_id: str) -> None:
        plan_id = self._task_to_plan.get(task_id)
        tree = self._plan_trees.get(plan_id) if plan_id else None
        if tree:
            tree.update_node_status(task_id, "invalidated")

    # ── Private output helpers ────────────────────────────────────────────────

    def _scroll_output_to_end(self) -> None:
        canvas = self._widgets.output_entries_canvas
        if canvas is None:
            return
        canvas.update_idletasks()
        canvas.yview_moveto(1.0)

    def _legacy_output_insert(self, text: str, tags: tuple[str, ...]) -> None:
        output = self._widgets.output_text
        if output is None:
            return
        output.insert(tk.END, text, tags)
        output.see(tk.END)

    def _header_preview(self, text: str) -> str:
        words = (text or "").replace("\r\n", "\n").split()
        if not words:
            return ""
        if len(words) <= 15:
            return " ".join(words)
        return " ".join(words[:15]) + " …"

    def _update_output_wraplength(self, canvas_width: int) -> None:
        self._output_wraplength = max(160, canvas_width - 40)
        active_labels: list[tk.Label] = []
        for label in self._output_wrapped_labels:
            try:
                label.configure(wraplength=self._output_wraplength)
                active_labels.append(label)
            except tk.TclError:
                continue
        self._output_wrapped_labels = active_labels

        active_text_widgets: list[tk.Text] = []
        for detail_text in self._output_detail_text_widgets:
            try:
                self._update_detail_text_height(detail_text)
                active_text_widgets.append(detail_text)
            except tk.TclError:
                continue
        self._output_detail_text_widgets = active_text_widgets

        active_html_frames: list = []
        for hf in self._output_html_frames:
            try:
                if not hf.winfo_exists():
                    continue
                self._update_html_frame_height(hf)
                active_html_frames.append(hf)
            except tk.TclError:
                continue
        self._output_html_frames = active_html_frames

    def _update_detail_text_height(self, detail_text: tk.Text) -> None:
        try:
            detail_text.update_idletasks()
            display_lines = detail_text.count("1.0", "end-1c", "displaylines")
            if display_lines and display_lines[0]:
                lines = max(1, int(display_lines[0]) + 1)
            else:
                text = detail_text.get("1.0", "end-1c")
                approx_chars = max(20, int(self._output_wraplength / 7))
                lines = max(1, int(math.ceil(len(text) / approx_chars)) + 1)
            detail_text.configure(height=lines)
        except (tk.TclError, ValueError, TypeError):
            return

    def _schedule_detail_text_height_update(self, detail_text: tk.Text) -> None:
        try:
            detail_text.after_idle(lambda w=detail_text: self._update_detail_text_height(w))
        except tk.TclError:
            return

    def _update_html_frame_height(self, html_frame: Any) -> None:
        try:
            html_frame.update_idletasks()
            height = html_frame.winfo_reqheight()
            if height > 1:
                html_frame.configure(height=height)
            canvas = self._widgets.output_entries_canvas
            if canvas is not None:
                canvas.configure(scrollregion=canvas.bbox("all"))
        except tk.TclError:
            return

    def _schedule_html_height_update(self, html_frame: Any) -> None:
        try:
            html_frame.after_idle(lambda hf=html_frame: self._update_html_frame_height(hf))
        except tk.TclError:
            return

    def _select_all_output_text(self, _event=None):
        output = self._widgets.output_text
        if output is None:
            return "break"
        output.tag_add(tk.SEL, "1.0", tk.END)
        output.mark_set(tk.INSERT, "1.0")
        output.see(tk.INSERT)
        return "break"

    def _copy_output_text_selection(self, _event=None):
        output = self._widgets.output_text
        if output is None:
            return "break"
        output.event_generate("<<Copy>>")
        return "break"

    def _bind_output_text_shortcuts(self) -> None:
        output = self._widgets.output_text
        if output is None:
            return
        output.bind("<Control-a>", self._select_all_output_text)
        output.bind("<Control-A>", self._select_all_output_text)
        output.bind("<Command-a>", self._select_all_output_text)
        output.bind("<Command-A>", self._select_all_output_text)
        output.bind("<Control-c>", self._copy_output_text_selection)
        output.bind("<Control-C>", self._copy_output_text_selection)
        output.bind("<Command-c>", self._copy_output_text_selection)
        output.bind("<Command-C>", self._copy_output_text_selection)

    def _finalize_entry_markdown(self, entry: dict[str, Any]) -> None:
        # Read availability flags from gui_manager module so tests can patch
        # agentx.gui.gui_manager.TKINTERWEB_AVAILABLE / MARKDOWN_AVAILABLE.
        import agentx.gui.gui_manager as _gm_mod

        if not _gm_mod.TKINTERWEB_AVAILABLE or not _gm_mod.MARKDOWN_AVAILABLE:
            return
        if not self._config.markdown_render_enabled:
            return
        if entry.get("is_finalized"):
            return
        if entry.get("role_label") in self._FINALIZE_SKIP_ROLES:
            return
        full_text = entry.get("full_text", "")
        if not has_markdown(full_text):
            return

        css = build_markdown_css(self._config)
        html = markdown_to_html(full_text, css)

        detail_text: tk.Text = entry["detail_text"]
        parent = detail_text.master

        self._output_detail_text_widgets = [w for w in self._output_detail_text_widgets if w is not detail_text]
        detail_text.destroy()
        entry["detail_text"] = None

        html_frame = _gm_mod.HtmlFrame(parent, messages_enabled=False)
        html_frame.load_html(html)

        entry["html_frame"] = html_frame
        entry["is_finalized"] = True
        self._output_html_frames.append(html_frame)

        if entry.get("expanded", True):
            html_frame.pack(fill=tk.X, anchor="w", padx=(24, 0))
        self._schedule_html_height_update(html_frame)

        toggle_btn: tk.Button = entry["toggle_btn"]

        def _html_toggle(e: dict = entry) -> None:
            e["expanded"] = not e["expanded"]
            toggle_btn.config(text=self.EXPAND_COLLAPSE_ICONS[e["expanded"]])
            if e["expanded"]:
                e["html_frame"].pack(fill=tk.X, anchor="w", padx=(24, 0))
                self._schedule_html_height_update(e["html_frame"])
            else:
                e["html_frame"].pack_forget()

        toggle_btn.config(command=_html_toggle)
        entry["toggle"] = _html_toggle

    def _create_output_entry(
        self,
        parent: tk.Widget,
        role_label: str,
        icon: str,
        content: str,
        expanded: bool,
        on_expand_changed: "Callable[[bool], None] | None" = None,
    ) -> dict[str, Any]:
        entry_frame = tk.Frame(parent, bg=self._config.output_bg)
        header_frame = tk.Frame(entry_frame, bg=self._config.output_bg)
        header_frame.pack(fill=tk.X, anchor="w")

        full_text = content or ""
        state: dict[str, Any] = {
            "frame": entry_frame,
            "header_var": tk.StringVar(),
            "expanded": expanded,
            "full_text": full_text,
            "role_label": role_label,
            "icon": icon,
        }

        toggle_btn = tk.Button(
            header_frame,
            width=1,
            height=1,
            font=("Terminal", 10),
            bd=0,
            bg=self._config.output_bg,
            fg=self._COLOR_AGENT_RESPONSE,
            activebackground=self._config.output_bg,
            activeforeground=self._COLOR_AGENT_RESPONSE,
        )
        toggle_btn.pack(side=tk.LEFT, padx=(0, 4))
        state["toggle_btn"] = toggle_btn
        state["html_frame"] = None
        state["is_finalized"] = False

        header_label = tk.Label(
            header_frame,
            textvariable=state["header_var"],
            bg=self._config.output_bg,
            fg=self._COLOR_AGENT_RESPONSE,
            anchor="w",
            justify=tk.LEFT,
            wraplength=self._output_wraplength,
        )
        header_label.pack(side=tk.LEFT, fill=tk.X, expand=True)
        self._output_wrapped_labels.append(header_label)

        text_font = self._text_font or self._config.default_font
        detail_text = tk.Text(
            entry_frame,
            wrap=tk.WORD,
            font=text_font,
            bg=self._config.output_bg,
            fg=self._COLOR_AGENT_RESPONSE,
            insertbackground=self._config.output_bg,
            borderwidth=0,
            highlightthickness=0,
            relief=tk.FLAT,
            height=max(1, full_text.count("\n") + 1) if full_text else 1,
            state=tk.NORMAL,
        )
        if full_text:
            detail_text.insert("1.0", full_text)
        detail_text.config(state=tk.DISABLED)
        detail_text.bind("<Key>", lambda _e: "break")
        detail_text.bind("<Control-a>", lambda _e, w=detail_text: (w.tag_add(tk.SEL, "1.0", tk.END), "break")[1])
        detail_text.bind("<Control-A>", lambda _e, w=detail_text: (w.tag_add(tk.SEL, "1.0", tk.END), "break")[1])
        detail_text.bind("<Command-a>", lambda _e, w=detail_text: (w.tag_add(tk.SEL, "1.0", tk.END), "break")[1])
        detail_text.bind("<Command-A>", lambda _e, w=detail_text: (w.tag_add(tk.SEL, "1.0", tk.END), "break")[1])
        detail_text.bind("<Control-c>", lambda _e, w=detail_text: w.event_generate("<<Copy>>") or "break")
        detail_text.bind("<Control-C>", lambda _e, w=detail_text: w.event_generate("<<Copy>>") or "break")
        detail_text.bind("<Command-c>", lambda _e, w=detail_text: w.event_generate("<<Copy>>") or "break")
        detail_text.bind("<Command-C>", lambda _e, w=detail_text: w.event_generate("<<Copy>>") or "break")
        detail_text.tag_config("sel", background="#3399ff", foreground="#ffffff")
        state["detail_text"] = detail_text
        self._output_detail_text_widgets.append(detail_text)

        def _on_detail_text_configure(_event) -> None:
            self._schedule_detail_text_height_update(detail_text)

        detail_text.bind("<Configure>", _on_detail_text_configure)

        def _toggle() -> None:
            state["expanded"] = not state["expanded"]
            toggle_btn.config(text=self.EXPAND_COLLAPSE_ICONS[state["expanded"]])
            if state["expanded"]:
                detail_text.pack(fill=tk.X, anchor="w", padx=(24, 0))
                self._schedule_detail_text_height_update(detail_text)
            else:
                detail_text.pack_forget()
            if on_expand_changed is not None:
                on_expand_changed(state["expanded"])

        toggle_btn.config(command=_toggle, text=self.EXPAND_COLLAPSE_ICONS[expanded])
        state["toggle"] = _toggle
        state["header_var"].set(f"{icon} {role_label}: {self._header_preview(full_text)}")

        if expanded:
            detail_text.pack(fill=tk.X, anchor="w", padx=(24, 0))
            self._schedule_detail_text_height_update(detail_text)

        entry_frame.pack(fill=tk.X, anchor="w", pady=(1, 1))
        return state

    def _append_output_entry_text(self, entry: dict[str, Any], chunk: str) -> None:
        if not chunk:
            return
        entry["full_text"] = f"{entry['full_text']}{chunk}"
        entry["header_var"].set(f"{entry['icon']} {entry['role_label']}: {self._header_preview(entry['full_text'])}")
        detail_text: tk.Text = entry["detail_text"]
        detail_text.config(state=tk.NORMAL)
        detail_text.insert(tk.END, chunk)
        detail_text.config(state=tk.DISABLED)
        self._schedule_detail_text_height_update(detail_text)

    def _ensure_turn_started(self, user_content: str) -> None:
        if self._widgets.output_entries_frame is None:
            return

        turn_frame = tk.Frame(self._widgets.output_entries_frame, bg=self._config.output_bg)
        turn_frame.pack(fill=tk.X, anchor="w", pady=(4, 6))
        children = tk.Frame(turn_frame, bg=self._config.output_bg)

        def _on_user_expand_changed(expanded: bool) -> None:
            if expanded:
                children.pack(fill=tk.X, anchor="w", padx=(22, 0))
            else:
                children.pack_forget()

        user_entry = self._create_output_entry(
            parent=turn_frame,
            role_label="User",
            icon=self.MESSAGE_ROLES["user"],
            content=user_content,
            expanded=True,
            on_expand_changed=_on_user_expand_changed,
        )

        # Pack children AFTER the user entry so Tkinter's pack manager renders
        # them below the user message.  Previously this line appeared before
        # _create_output_entry(), which caused child entries (classification,
        # thinking, agent response) to stack above the user prompt on first
        # render.  Collapsing then expanding "fixed" it accidentally because
        # pack_forget()/pack() re-inserts the frame at the end of the list.
        children.pack(fill=tk.X, anchor="w", padx=(22, 0))

        self._current_turn_frame = turn_frame
        self._current_turn_children_frame = children
        self._current_turn_entries = {"user": user_entry}

    def _ensure_child_entry(
        self,
        key: str,
        role_label: str,
        icon: str,
        initial_text: str,
        expanded: bool,
    ) -> Optional[dict[str, Any]]:
        if self._current_turn_children_frame is None:
            return None
        if key in self._current_turn_entries:
            return self._current_turn_entries[key]
        entry = self._create_output_entry(
            parent=self._current_turn_children_frame,
            role_label=role_label,
            icon=icon,
            content=initial_text,
            expanded=expanded,
        )
        self._current_turn_entries[key] = entry
        return entry

    def _set_entry_text(self, entry: dict[str, Any], text: str) -> None:
        entry["full_text"] = text
        entry["header_var"].set(f"{entry['icon']} {entry['role_label']}: {self._header_preview(text)}")
        detail_text: tk.Text = entry["detail_text"]
        detail_text.config(state=tk.NORMAL)
        detail_text.delete("1.0", tk.END)
        detail_text.insert("1.0", text)
        detail_text.config(state=tk.DISABLED)
        self._schedule_detail_text_height_update(detail_text)

    def _display_tool_line(self, line: str) -> bool:
        stripped = line.strip()
        if stripped.startswith("[🔧 Calling tool"):
            entry = self._ensure_child_entry(
                key="tool_call",
                role_label="Tool",
                icon=self.MESSAGE_ROLES["tool_call"],
                initial_text=stripped,
                expanded=False,
            )
            if entry is not None:
                if entry["full_text"]:
                    self._set_entry_text(entry, f"{entry['full_text']}\n{stripped}")
                else:
                    self._set_entry_text(entry, stripped)
            return True

        if stripped.startswith("[📋 Tool result"):
            entry = self._ensure_child_entry(
                key="tool_result",
                role_label="Tool",
                icon=self.MESSAGE_ROLES["tool_result"],
                initial_text=stripped,
                expanded=False,
            )
            if entry is not None:
                if entry["full_text"]:
                    self._set_entry_text(entry, f"{entry['full_text']}\n{stripped}")
                else:
                    self._set_entry_text(entry, stripped)
            return True

        return False
