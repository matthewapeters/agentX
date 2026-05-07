"""Side panel extracted from GUIManager.

Owns the system-status pane: model selector, tabbed notebook (Session / Files /
Settings), collapsible sections, and tool checkboxes.  Follows the back-reference
pattern: ``SidePanel(gui_manager)`` stores ``self._g = gui_manager``.
"""

from __future__ import annotations

import tkinter as tk
from tkinter import ttk
from typing import TYPE_CHECKING, Any, Callable, Optional

from .collapsible_section import CollapsibleSection
from .settings_tab import SettingsTab
from .status_tab import StatusTab

if TYPE_CHECKING:
    from .gui_manager import GUIManager

import logging

logger = logging.getLogger(__name__)


class SidePanel:
    """Manages the system-status side panel including tabs and collapsible sections."""

    def __init__(self, gui_manager: "GUIManager") -> None:
        self._g = gui_manager

        # Widget components – model selector is exposed as a public attribute
        # because GUIManager (and tests) reference it as ``gui.model_selector``.
        self.model_selector = None

        # Collapsible / tab state
        self._session_sections: dict[str, CollapsibleSection] = {}
        self._system_tab_frames: dict[str, tk.Frame] = {}
        self._system_tab_section_stacks: dict[str, tk.Frame] = {}
        self._session_section_spacing: int = self._g.config.session_section_spacing

        # Tool panel state
        self._tool_panel_vars: dict[str, tk.BooleanVar] = {}
        self._tool_panel_tools: Optional[list] = None

        # Settings tab state
        self._settings_tab_widget = None

        # Status tab (PD-12)
        self._status_tab: StatusTab = StatusTab(gui_manager)

    # ── Convenience accessors ─────────────────────────────────────────────────

    @property
    def _config(self):
        return self._g.config

    @property
    def _widgets(self):
        return self._g.widgets

    @property
    def _section_bg(self) -> str:
        return self._g._section_bg

    # ── Panel creation ────────────────────────────────────────────────────────

    def create(self) -> None:
        """Create the system-status side panel and add it to the paned window."""
        from .model_selector import ModelSelector

        self._widgets.system_status = tk.Frame(self._widgets.paned, bg=self._section_bg)

        # Model selector at the top
        model_frame = tk.Frame(self._widgets.system_status, bg=self._section_bg)
        model_frame.pack(fill=tk.X, padx=5, pady=5)

        self.model_selector = ModelSelector(
            parent=model_frame,
            on_model_change=self._g._on_model_change,
        )
        self.model_selector.get_widget().pack(side=tk.LEFT)

        # Tabbed notebook
        self._widgets.system_notebook = ttk.Notebook(self._widgets.system_status)
        self._widgets.system_notebook.pack(expand=True, fill=tk.BOTH, padx=0, pady=0)

        # Status tab — first tab [PD-12-AF-001]
        status_frame = self._status_tab.create(
            notebook=self._widgets.system_notebook,
            section_bg=self._section_bg,
        )
        # add() is used (not insert) because the notebook is still empty here;
        # subsequent tabs are added after, so Status is naturally at index 0.
        self._widgets.system_notebook.add(status_frame, text="⚡ Status")

        # Session tab
        self._widgets.session_tab = tk.Frame(self._widgets.system_notebook, bg=self._section_bg)
        self._widgets.system_notebook.add(self._widgets.session_tab, text="Session")

        for key, title, collapsed in [
            ("history", "History", True),
            ("tools", "Available Tools", True),
            ("working_memory", "🏛️ Working Memory", True),
            ("context", "Context", True),
        ]:
            self._register_system_collapsible_section(
                tab_name="Session",
                section_key=key,
                title=title,
                initial_collapsed=collapsed,
                spacing=self._session_section_spacing,
            )

        self._refresh_tools_section()

        # Files tab
        self._widgets.files_tab = tk.Frame(self._widgets.system_notebook, bg=self._section_bg)
        self._widgets.system_notebook.add(self._widgets.files_tab, text="Files")

        # Settings tab
        self._widgets.settings_tab = tk.Frame(self._widgets.system_notebook, bg=self._section_bg)
        self._widgets.system_notebook.add(self._widgets.settings_tab, text="⚙️ Settings")

        def _on_tab_changed(_event):
            self._g.root.update_idletasks()
            selected_tab = self._widgets.system_notebook.select()
            if selected_tab:
                self._widgets.system_notebook.nametowidget(selected_tab).update_idletasks()

        self._widgets.system_notebook.bind("<<NotebookTabChanged>>", _on_tab_changed)

        self._widgets.paned.add(self._widgets.system_status, stretch="always")

        def _set_initial_split():
            self._g.root.update_idletasks()
            paned_width = self._widgets.paned.winfo_width()
            if paned_width > 1:
                sash_position = int(paned_width * self._config.output_panel_ratio)
                self._widgets.paned.sash_place(0, sash_position, 1)

        self._g.root.after(100, _set_initial_split)

    # ── Tab / section management ──────────────────────────────────────────────

    def _get_or_create_system_tab(self, tab_name: str) -> tk.Frame:
        if self._widgets.system_notebook is None:
            raise RuntimeError("system_notebook not yet created")
        key = tab_name.lower()
        if key in self._system_tab_frames:
            return self._system_tab_frames[key]
        if key == "session" and self._widgets.session_tab is not None:
            tab_frame = self._widgets.session_tab
        elif key == "files" and self._widgets.files_tab is not None:
            tab_frame = self._widgets.files_tab
        else:
            tab_frame = tk.Frame(self._widgets.system_notebook, bg=self._section_bg)
            self._widgets.system_notebook.add(tab_frame, text=tab_name)
        self._system_tab_frames[key] = tab_frame
        return tab_frame

    def _get_or_create_system_section_stack(self, tab_name: str) -> tk.Frame:
        key = tab_name.lower()
        if key in self._system_tab_section_stacks:
            return self._system_tab_section_stacks[key]
        tab_frame = self._get_or_create_system_tab(tab_name)
        stack = tk.Frame(tab_frame, bg=self._section_bg)
        stack.pack(fill=tk.BOTH, expand=True, padx=5, pady=5)
        self._system_tab_section_stacks[key] = stack
        return stack

    def _register_system_collapsible_section(
        self,
        tab_name: str,
        section_key: str,
        title: str,
        initial_collapsed: bool = True,
        spacing: int = 8,
    ) -> CollapsibleSection:
        stack = self._get_or_create_system_section_stack(tab_name)
        section = CollapsibleSection(
            parent=stack,
            title=title,
            initial_collapsed=initial_collapsed,
            bg=self._section_bg,
            fg=self._config.ui_fg,
        )
        section.get_widget().pack(fill=tk.X, expand=False, pady=(0, spacing))
        if tab_name.lower() == "session":
            self._session_sections[section_key] = section
        return section

    def register_system_collapsible_section(
        self,
        tab_name: str,
        section_key: str,
        title: str,
        initial_collapsed: bool = True,
        spacing: Optional[int] = None,
    ) -> tk.Widget:
        """Public API: register a collapsible section and return its widget."""
        section = self._register_system_collapsible_section(
            tab_name=tab_name,
            section_key=section_key,
            title=title,
            initial_collapsed=initial_collapsed,
            spacing=self._session_section_spacing if spacing is None else spacing,
        )
        return section.get_widget()

    # ── Status tab API (PD-12) ────────────────────────────────────────────────

    def show_status_tab(self) -> None:
        """Switch the system notebook to the Status tab.

        [PD-12-AF-002] Called from StreamingController._on_stream_start().
        """
        self._status_tab.show(self._widgets.system_notebook)

    def set_status_streaming_state(self, is_streaming: bool) -> None:
        """Relay streaming state to StatusTab interrupt button.

        [PD-12-AF-003]

        Args:
            is_streaming (bool): True while LLM stream is active.
        """
        self._status_tab.set_streaming_state(is_streaming)

    def reset_status_tab(self) -> None:
        """Reset all phase rows and restart the tick loop.

        [PD-12-AF-005] Called from StreamingController._on_stream_start().
        """
        self._status_tab.reset()

    def set_status_phase(
        self,
        step_key: str,
        state: str,
        tool_name: Optional[str] = None,
        start_time: Optional[float] = None,
    ) -> None:
        """Transition a phase row in the StatusTab.

        [PD-12-AF-006] [PD-12-AF-007] [PD-12-AF-008] [PD-12-AF-009]

        Args:
            step_key (str): Phase key — classify, think, tool, respond.
            state (str): RUNNING, DONE, or FAILED.
            tool_name (Optional[str]): Tool name for the tool step label.
            start_time (Optional[float]): Background-thread timestamp anchor;
                see ``StatusTab.set_phase`` for details.
        """
        self._status_tab.set_phase(step_key, state, tool_name=tool_name, start_time=start_time)

    def stop_status_tick(self) -> None:
        """Stop the phase elapsed timer tick loop."""
        self._status_tab.stop_tick()

    @property
    def status_tab_context_meter(self) -> "StatusTab":
        """Expose the StatusTab for context meter delegation.

        Returns:
            StatusTab: The status tab instance.
        """
        return self._status_tab

    # ── Model selector ────────────────────────────────────────────────────────

    def populate_models(self, models: list[dict], initial_model: str = None) -> None:
        """Populate the model selector drop-down."""
        if self.model_selector:
            self.model_selector.populate(models, initial_model=initial_model)

    def set_model_change_callback(self, callback: Callable[[str], None]) -> None:
        """Wire the model-change callback to both GUIManager and model selector."""
        self._g._on_model_change = callback  # type: ignore[method-assign]
        if self.model_selector:
            self.model_selector.on_model_change = callback

    def set_refresh_models_callback(self, callback: Callable[[], None]) -> None:
        """Register the callback invoked when the user clicks the model-list refresh button.

        Args:
            callback (Callable[[], None]): Called with no arguments when the
                refresh button is pressed (PD-04-AF-004).
        """
        if self.model_selector:
            self.model_selector.set_refresh_callback(callback)

    # ── Tools panel ───────────────────────────────────────────────────────────

    def populate_tools(self, tools: list[dict]) -> None:
        """Populate the tools section with the given tool definitions."""
        self._tool_panel_tools = tools
        self._refresh_tools_section()

    def get_enabled_tools(self) -> list[str]:
        """Return the names of currently checked tools."""
        return [name for name, var in self._tool_panel_vars.items() if var.get()]

    def set_tool_toggle_callback(self, callback: Callable[[str, bool], None]) -> None:
        """Wire the tool-toggle callback to GUIManager."""
        self._g._on_tool_toggle = callback  # type: ignore[method-assign]

    def _parse_tool_metadata(self, tool: dict) -> tuple[str, str]:
        tool_func = tool.get("function") if isinstance(tool, dict) else None
        name = tool.get("name") if isinstance(tool, dict) else None
        if not name and isinstance(tool_func, dict):
            name = tool_func.get("name")
        if not name:
            name = "Unknown"
        description = tool.get("description", "") if isinstance(tool, dict) else ""
        if not description and isinstance(tool_func, dict):
            description = tool_func.get("description", "")
        return name, description

    def _build_tools_content_widget(self, parent: tk.Widget) -> tk.Widget:
        container = tk.Frame(parent, bg=self._section_bg)
        container.columnconfigure(0, weight=1)

        if not self._tool_panel_tools:
            tk.Label(
                container,
                text="No tools available",
                foreground=self._config.muted_fg,
                font=("", 9, "italic"),
                bg=self._section_bg,
            ).grid(row=0, column=0, sticky="w", pady=10)
            return container

        previous_enabled = {name: var.get() for name, var in self._tool_panel_vars.items()}
        self._tool_panel_vars = {}

        canvas = tk.Canvas(
            container,
            borderwidth=0,
            highlightthickness=0,
            height=160,
            bg=self._section_bg,
        )
        scrollbar = tk.Scrollbar(container, orient="vertical", command=canvas.yview)
        canvas.configure(yscrollcommand=scrollbar.set)
        canvas.grid(row=0, column=0, sticky="nsew")
        scrollbar.grid(row=0, column=1, sticky="ns")
        container.rowconfigure(0, weight=1)

        content = tk.Frame(canvas, bg=self._section_bg)
        canvas_window = canvas.create_window((0, 0), window=content, anchor="nw")

        def _on_content_configure(_event):
            canvas.configure(scrollregion=canvas.bbox("all"))

        def _on_canvas_configure(event):
            canvas.itemconfig(canvas_window, width=event.width)

        content.bind("<Configure>", _on_content_configure)
        canvas.bind("<Configure>", _on_canvas_configure)

        def _on_mousewheel(event):
            canvas.yview_scroll(int(-1 * (event.delta / 120)), "units")

        def _on_mousewheel_linux(event):
            canvas.yview_scroll(-1 if event.num == 4 else 1, "units")

        canvas.bind("<MouseWheel>", _on_mousewheel)
        canvas.bind("<Button-4>", _on_mousewheel_linux)
        canvas.bind("<Button-5>", _on_mousewheel_linux)

        for idx, tool in enumerate(self._tool_panel_tools):
            name, description = self._parse_tool_metadata(tool)
            is_enabled = previous_enabled.get(name, True)
            var = tk.BooleanVar(value=is_enabled)
            self._tool_panel_vars[name] = var

            tk.Checkbutton(
                content,
                text=name,
                variable=var,
                command=lambda n=name, v=var: self._g._on_tool_toggle(n, v.get()),
                bg=self._section_bg,
                fg=self._config.ui_fg,
                activebackground=self._section_bg,
                activeforeground=self._config.ui_fg,
                selectcolor=self._section_bg,
            ).grid(row=idx, column=0, sticky="w", pady=2, padx=(0, 5))

            if description:
                desc_text = f"- {description[:50]}..." if len(description) > 50 else f"- {description}"
                tk.Label(
                    content,
                    text=desc_text,
                    foreground=self._config.muted_fg,
                    font=("", 9),
                    bg=self._section_bg,
                ).grid(row=idx, column=1, sticky="w")

        return container

    def _refresh_tools_section(self) -> None:
        tools_section = self._session_sections.get("tools")
        if tools_section is None:
            return
        tools_content = self._build_tools_content_widget(tools_section.content_container)
        tools_section.set_content(tools_content, fill=tk.BOTH, expand=False)

    # ── Panel update methods ──────────────────────────────────────────────────

    def update_context_panel(self, context_widget: tk.Widget) -> None:
        """Replace the Context section content."""
        section = self._session_sections.get("context")
        if section is None:
            raise RuntimeError("Context section not initialized")
        section.set_content(context_widget, fill=tk.BOTH, expand=True)
        self._widgets.system_status_context = context_widget

    def update_history_panel(self, history_widget: tk.Widget) -> None:
        """Replace the History section content."""
        section = self._session_sections.get("history")
        if section is None:
            raise RuntimeError("History section not initialized")
        section.set_content(history_widget, fill=tk.BOTH, expand=False)
        self._widgets.system_status_history = history_widget

    def update_working_memory_panel(self, working_memory_widget: tk.Widget, fact_count: int = 0) -> None:
        """Replace the Working Memory section content."""
        section = self._session_sections.get("working_memory")
        if section is None:
            return
        label = "🏛️ Working Memory" if fact_count == 0 else f"🏛️ Working Memory ({fact_count})"
        section.set_title(label)
        section.set_content(working_memory_widget, fill=tk.BOTH, expand=True)

    def update_files_panel(self, files_widget: tk.Widget) -> None:
        """Replace the Files tab content."""
        if self._widgets.system_status_files is not None:
            self._widgets.system_status_files.destroy()
        files_widget.pack(expand=True, fill=tk.BOTH)
        self._widgets.system_status_files = files_widget

    # ── Parent accessors ──────────────────────────────────────────────────────

    def get_context_parent(self) -> tk.Widget:
        section = self._session_sections.get("context")
        if section is None:
            raise RuntimeError("context section not yet created")
        return section.content_container

    def get_history_parent(self) -> tk.Widget:
        section = self._session_sections.get("history")
        if section is None:
            raise RuntimeError("history section not yet created")
        return section.content_container

    def get_working_memory_parent(self) -> tk.Widget:
        section = self._session_sections.get("working_memory")
        if section is None:
            raise RuntimeError("working_memory section not yet created")
        return section.content_container

    def get_files_parent(self) -> tk.Widget:
        if self._widgets.files_tab is None:
            raise RuntimeError("files_tab not yet created")
        return self._widgets.files_tab

    def get_settings_parent(self) -> tk.Widget:
        if getattr(self._widgets, "settings_tab", None) is None:
            raise RuntimeError("settings_tab not yet created")
        return self._widgets.settings_tab

    # ── Settings tab ──────────────────────────────────────────────────────────

    def render_settings_tab(
        self,
        config: dict,
        on_change: Callable[[list[str], Any], None],
        models: Optional[list] = None,
        system_prompts_dir: str = "system_prompts",
    ) -> None:
        """Build or rebuild the Settings tab content."""
        parent = self.get_settings_parent()
        if self._settings_tab_widget is not None:
            try:
                self._settings_tab_widget.frame.destroy()
            except Exception:
                logger.debug("Could not destroy previous SettingsTab widget", exc_info=True)
        self._settings_tab_widget = SettingsTab(
            parent,
            config=config,
            on_change=on_change,
            models=models or [],
            system_prompts_dir=system_prompts_dir,
            bg=self._section_bg,
            fg=self._config.ui_fg,
            muted_fg=self._config.muted_fg,
        )
        self._settings_tab_widget.frame.pack(fill=tk.BOTH, expand=True)

    def populate_settings_models(self, models: list[dict]) -> None:
        """Refresh model dropdowns in the Settings tab."""
        if self._settings_tab_widget is not None:
            self._settings_tab_widget.populate_models(models)
