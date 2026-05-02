"""GUI Manager implementation.

GUIManager has been decomposed into four panel classes that each own a
focused slice of state and widget creation:

    ContextRenderer  — stateless widget factory for context/history/WM rendering
    ChatPanel        — output notebook, structured entries, plan tree tabs
    InputPanel       — user text input, submit/interrupt buttons, attachment bar
    SidePanel        — system-status pane, model selector, tabs, tool checkboxes

GUIManager now acts as a thin coordinator: it wires the panels together via the
back-reference pattern (each panel holds ``self._g = gui_manager``) and exposes
the full IGUIManager interface by delegating every method to the appropriate panel.

Forwarding @property attributes maintain backward compatibility with tests that
access private attributes directly (``gui._current_turn_entries`` etc.).
"""

from __future__ import annotations

import logging
import os
import tkinter as tk
from datetime import datetime
from typing import TYPE_CHECKING, Any, Callable, Optional

from ..attachment_info import AttachmentInfo
from ..history import History
from ..igui_manager import IGUIManager
from ..widget_registry import WidgetRegistry
from .gui_config import GUIConfig

# Re-export markdown/tkinterweb availability flags at module level so that
# tests can patch them via ``agentx.gui.gui_manager.TKINTERWEB_AVAILABLE`` etc.
try:
    from .markdown_renderer import (  # type: ignore
        MARKDOWN_AVAILABLE,
        TKINTERWEB_AVAILABLE,
        HtmlFrame,
    )
except Exception:
    TKINTERWEB_AVAILABLE: bool = False
    MARKDOWN_AVAILABLE: bool = False
    HtmlFrame = None

if TYPE_CHECKING:
    from .chat_panel import ChatPanel
    from .context_renderer import ContextRenderer
    from .input_panel import InputPanel
    from .side_panel import SidePanel

logger = logging.getLogger(__name__)


class GUIManager(IGUIManager):
    """Manages all GUI widgets and presentation logic.

    This class implements the IGUIManager interface; all heavy lifting is done
    by the four panel objects.  See the module docstring for the decomposition.
    """

    # ── Class-level color palette ─────────────────────────────────────────────
    COLOR_BG = "#222222"
    COLOR_STATUS_BG = "#333333"
    COLOR_OUTPUT_BG = "#222222"
    COLOR_INPUT_BG = "#222222"
    COLOR_SCROLLBAR = "#444444"
    COLOR_SELECTION_BG = "#3399ff"
    COLOR_SELECTION_FG = "#ffffff"
    COLOR_ERROR = "#ff4444"
    COLOR_ATTACHMENT_BG = "#444444"
    COLOR_ATTACHMENT_HISTORY_BG = "#555555"
    COLOR_ATTACHMENT_TEXT = "#eeeeee"
    COLOR_USER_PROMPT = "#eeeeee"
    COLOR_AGENT_RESPONSE = "#eeeeee"
    COLOR_AGENT_THINKING = "#cccccc"
    COLOR_AGENT_CLASSIFICATION = "#7dd3fc"
    COLOR_SYSTEM_SPACE = "#888888"

    # ── Layout / rendering constants ──────────────────────────────────────────
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

    # ── Constructor ───────────────────────────────────────────────────────────

    def __init__(
        self,
        root: tk.Tk,
        config: GUIConfig,
        on_submit: Callable[[], None],
        on_interrupt: Callable[[], None],
        on_attachment_toggle: Callable[[str, bool], None],
    ) -> None:
        self.root = root
        self.config = config
        self.widgets = WidgetRegistry()
        self._section_bg = self.config.status_bg

        # Instance-level color aliases (may differ from class defaults when
        # the config overrides the palette)
        self.COLOR_BG = self.config.status_bg
        self.COLOR_STATUS_BG = self.config.status_bg
        self.COLOR_OUTPUT_BG = self.config.output_bg
        self.COLOR_INPUT_BG = self.config.input_bg
        self.COLOR_ATTACHMENT_BG = self.config.attachment_bg
        self.COLOR_ATTACHMENT_HISTORY_BG = self.config.history_attachment_bg
        self.COLOR_ATTACHMENT_TEXT = self.config.attachment_fg
        self.COLOR_USER_PROMPT = self.config.user_prompt_fg
        self.COLOR_AGENT_RESPONSE = self.config.agent_response_fg
        self.COLOR_AGENT_THINKING = self.config.agent_thinking_fg
        self.COLOR_AGENT_CLASSIFICATION = self.config.agent_classification_fg
        self.COLOR_SYSTEM_SPACE = self.config.system_space_fg

        # Callbacks stored here because panels access them via ``self._g.*``
        self._on_submit = on_submit
        self._on_interrupt = on_interrupt
        # NOTE: storing this as an instance attribute shadows the class method
        # of the same name (defined below).  All callers that do
        # ``self._on_attachment_toggle(id, val)`` will invoke the stored
        # callable directly.
        self._on_attachment_toggle = on_attachment_toggle  # type: ignore[method-assign]

        # Cached font tuple — populated by _setup_fonts(); read by panels
        self._text_font: Optional[tuple] = None

        # Panel objects — created after all shared state is established so that
        # their ``__init__`` can safely call ``self._g.*``
        from .chat_panel import ChatPanel
        from .context_renderer import ContextRenderer
        from .input_panel import InputPanel
        from .side_panel import SidePanel

        self._context_renderer: ContextRenderer = ContextRenderer(self)
        self._chat_panel: ChatPanel = ChatPanel(self)
        self._input_panel: InputPanel = InputPanel(self)
        self._side_panel: SidePanel = SidePanel(self)

    # ── Forwarding properties (backward compat for tests) ─────────────────────

    @property
    def model_selector(self):
        return self._side_panel.model_selector

    @model_selector.setter
    def model_selector(self, value) -> None:
        self._side_panel.model_selector = value

    @property
    def _current_turn_entries(self) -> dict:
        return self._chat_panel._current_turn_entries

    @property
    def _current_turn_children_frame(self):
        return self._chat_panel._current_turn_children_frame

    @property
    def _session_sections(self) -> dict:
        return self._side_panel._session_sections

    @property
    def _settings_tab_widget(self):
        return self._side_panel._settings_tab_widget

    @property
    def _plan_trees(self) -> dict:
        return self._chat_panel._plan_trees

    @property
    def _task_to_plan(self) -> dict:
        return self._chat_panel._task_to_plan

    @property
    def _session_section_spacing(self) -> int:
        return self._side_panel._session_section_spacing

    @property
    def _agent_classification_shown(self) -> bool:
        return self._chat_panel._agent_classification_shown

    @property
    def _agent_thinking_started(self) -> bool:
        return self._chat_panel._agent_thinking_started

    @property
    def _agent_response_started(self) -> bool:
        return self._chat_panel._agent_response_started

    @property
    def _output_wrapped_labels(self) -> list:
        return self._chat_panel._output_wrapped_labels

    @property
    def _output_html_frames(self) -> list:
        return self._chat_panel._output_html_frames

    @property
    def _tool_panel_vars(self) -> dict:
        return self._side_panel._tool_panel_vars

    # ── Lifecycle ─────────────────────────────────────────────────────────────

    def create_layout(self) -> None:
        """Create and arrange all GUI widgets by delegating to panel objects."""
        self._setup_fonts()
        self._setup_window_geometry()
        self._chat_panel.create()
        self._side_panel.create()
        self._input_panel.create()
        self._chat_panel.configure_text_styles()

    def destroy(self) -> None:
        """Cleanup and destroy all widgets."""
        self.widgets.destroy_all()

    # ── Context / history rendering ───────────────────────────────────────────

    def collapse_expand_button(
        self,
        parent: tk.Widget,
        expandable_frame: tk.Widget = None,
        attachment_rows=None,
    ) -> tk.Button:
        return self._context_renderer.collapse_expand_button(parent, expandable_frame, attachment_rows)

    def render_history_widget(
        self,
        history_obj: History,
        parent,
        user_name,
        on_attachment_toggle=None,
        include_header: bool = False,
    ):
        return self._context_renderer.render_history_widget(
            history_obj, parent, user_name, on_attachment_toggle, include_header
        )

    def render_context_widget(
        self,
        context_obj,
        parent,
        on_attachment_toggle=None,
        include_header: bool = False,
        on_plan_click=None,
    ):
        return self._context_renderer.render_context_widget(
            context_obj, parent, on_attachment_toggle, include_header, on_plan_click
        )

    def render_working_memory_widget(
        self,
        working_memory,
        parent: tk.Widget,
        on_toggle: "Callable[[str, bool], None] | None" = None,
        on_delete: "Callable[[str], None] | None" = None,
        on_promote: "Callable[[str], None] | None" = None,
        on_user_add: "Callable[[str, str], None] | None" = None,
    ) -> tk.Frame:
        return self._context_renderer.render_working_memory_widget(
            working_memory, parent, on_toggle, on_delete, on_promote, on_user_add
        )

    def _render_message_to_grid(
        self,
        message_obj,
        parent_frame,
        start_row,
        on_attachment_toggle=None,
        tool_interactions=None,
        on_plan_click=None,
    ) -> int:
        return self._context_renderer._render_message_to_grid(
            message_obj, parent_frame, start_row, on_attachment_toggle, tool_interactions, on_plan_click
        )

    def _render_tool_rows(self, tool_msgs, parent_frame, start_row, parent_collapsible) -> int:
        return self._context_renderer._render_tool_rows(tool_msgs, parent_frame, start_row, parent_collapsible)

    def _render_plan_rows(
        self, plan_msgs, task_node_msgs, parent_frame, start_row, parent_collapsible, on_plan_click=None
    ) -> int:
        return self._context_renderer._render_plan_rows(
            plan_msgs, task_node_msgs, parent_frame, start_row, parent_collapsible, on_plan_click
        )

    # ── Display methods ───────────────────────────────────────────────────────

    def display_user_message(self, content: str, attachments: list[str], timestamp: datetime) -> None:
        self._chat_panel.display_user_message(content, attachments, timestamp)

    def display_agent_thinking(self, content: str) -> None:
        self._chat_panel.display_agent_thinking(content)

    def display_classification(self, classification: dict) -> None:
        self._chat_panel.display_classification(classification)

    def display_agent_response(self, content: str) -> None:
        self._chat_panel.display_agent_response(content)

    def display_bootstrap_agent_response(self, content: str) -> None:
        self._chat_panel.display_bootstrap_agent_response(content)

    def display_startup_notice(self, content: str) -> None:
        self._chat_panel.display_startup_notice(content)

    def display_error(self, message: str) -> None:
        self._chat_panel.display_error(message)

    def display_spacing(self) -> None:
        self._chat_panel.display_spacing()

    def finalize_current_turn_markdown(self) -> None:
        self._chat_panel.finalize_current_turn_markdown()

    def _select_all_output_text(self) -> None:
        self._chat_panel._select_all_output_text()

    def _update_output_wraplength(self, width: int) -> None:
        self._chat_panel._update_output_wraplength(width)

    def _set_entry_text(self, entry: dict, text: str) -> None:
        self._chat_panel._set_entry_text(entry, text)

    def _create_output_entry(self, *args, **kwargs) -> dict:
        return self._chat_panel._create_output_entry(*args, **kwargs)

    def _finalize_entry_markdown(self, entry: dict) -> None:
        self._chat_panel._finalize_entry_markdown(entry)

    # ── Attachment bar ────────────────────────────────────────────────────────

    def update_attachment_bar(
        self,
        current_attachments: list[AttachmentInfo],
        history_attachments: list[AttachmentInfo],
    ) -> None:
        self._input_panel.update_attachment_bar(current_attachments, history_attachments)

    # ── Panel update methods ──────────────────────────────────────────────────

    def update_context_panel(self, context_widget: tk.Widget) -> None:
        self._side_panel.update_context_panel(context_widget)

    def update_history_panel(self, history_widget: tk.Widget) -> None:
        self._side_panel.update_history_panel(history_widget)

    def update_working_memory_panel(self, working_memory_widget: tk.Widget, fact_count: int = 0) -> None:
        self._side_panel.update_working_memory_panel(working_memory_widget, fact_count)

    def update_files_panel(self, files_widget: tk.Widget) -> None:
        self._side_panel.update_files_panel(files_widget)

    # ── Parent accessors ──────────────────────────────────────────────────────

    def get_context_parent(self) -> tk.Widget:
        return self._side_panel.get_context_parent()

    def get_history_parent(self) -> tk.Widget:
        return self._side_panel.get_history_parent()

    def get_working_memory_parent(self) -> tk.Widget:
        return self._side_panel.get_working_memory_parent()

    def get_files_parent(self) -> tk.Widget:
        return self._side_panel.get_files_parent()

    def get_settings_parent(self) -> tk.Widget:
        return self._side_panel.get_settings_parent()

    # ── Input methods ─────────────────────────────────────────────────────────

    def get_user_input(self) -> str:
        return self._input_panel.get_user_input()

    def clear_user_input(self) -> None:
        self._input_panel.clear_user_input()

    def get_cached_user_input(self) -> str:
        return self._input_panel.get_cached_user_input()

    def set_streaming_state(self, is_streaming: bool) -> None:
        self._input_panel.set_streaming_state(is_streaming)

    def set_busy_state(self, is_busy: bool) -> None:
        self._input_panel.set_busy_state(is_busy)

    # ── State accessors ───────────────────────────────────────────────────────

    def get_root(self) -> tk.Tk:
        return self.root

    def set_window_title(self, title: str) -> None:
        self.root.title(title)

    # ── Settings tab ──────────────────────────────────────────────────────────

    def render_settings_tab(
        self,
        config: dict,
        on_change: Callable[[list[str], Any], None],
        models: Optional[list] = None,
        system_prompts_dir: str = "system_prompts",
    ) -> None:
        self._side_panel.render_settings_tab(config, on_change, models, system_prompts_dir)

    def populate_settings_models(self, models: list[dict]) -> None:
        self._side_panel.populate_settings_models(models)

    def register_system_collapsible_section(
        self,
        tab_name: str,
        section_key: str,
        title: str,
        initial_collapsed: bool = True,
        spacing: Optional[int] = None,
    ) -> tk.Widget:
        return self._side_panel.register_system_collapsible_section(
            tab_name, section_key, title, initial_collapsed, spacing
        )

    # ── Model / tool management ───────────────────────────────────────────────

    def populate_models(self, models: list[dict], initial_model: str = None) -> None:
        self._side_panel.populate_models(models, initial_model)

    def populate_tools(self, tools: list[dict]) -> None:
        self._side_panel.populate_tools(tools)

    def get_enabled_tools(self) -> list[str]:
        return self._side_panel.get_enabled_tools()

    def set_model_change_callback(self, callback: Callable[[str], None]) -> None:
        self._side_panel.set_model_change_callback(callback)

    def set_refresh_models_callback(self, callback: Callable[[], None]) -> None:
        """Register the callback invoked when the user presses the model-list refresh button (PD-04-AF-004)."""
        self._side_panel.set_refresh_models_callback(callback)

    def set_tool_toggle_callback(self, callback: Callable[[str, bool], None]) -> None:
        self._side_panel.set_tool_toggle_callback(callback)

    # ── Plan tree methods ─────────────────────────────────────────────────────

    def add_plan_tab(self, plan_id: str, plan_name: str, on_export: Optional[Callable[[], None]] = None) -> tk.Frame:
        return self._chat_panel.add_plan_tab(plan_id, plan_name, on_export)

    def get_plan_tab_frame(self, plan_id: str) -> Optional[tk.Frame]:
        return self._chat_panel.get_plan_tab_frame(plan_id)

    def focus_plan_tab(self, plan_id: str) -> None:
        self._chat_panel.focus_plan_tab(plan_id)

    def add_plan_step_node(
        self,
        plan_id: str,
        task_id: str,
        description: str,
        tbd: bool,
        on_replay: Optional[Callable[[str], None]] = None,
    ) -> None:
        self._chat_panel.add_plan_step_node(plan_id, task_id, description, tbd, on_replay)

    def add_plan_subtask_node(
        self,
        task_id: str,
        parent_task_id: str,
        description: str,
        depth: int,
        on_replay: Optional[Callable[[str], None]] = None,
    ) -> None:
        self._chat_panel.add_plan_subtask_node(task_id, parent_task_id, description, depth, on_replay)

    def update_plan_node_status(self, task_id: str, status: str) -> None:
        self._chat_panel.update_plan_node_status(task_id, status)

    def resolve_plan_tbd_node(self, task_id: str, resolved_description: str) -> None:
        self._chat_panel.resolve_plan_tbd_node(task_id, resolved_description)

    def add_plan_tool_call(self, task_id: str, tool_name: str, tool_input: Any) -> None:
        self._chat_panel.add_plan_tool_call(task_id, tool_name, tool_input)

    def add_plan_synthesis(
        self,
        task_id: str,
        synthesis_text: str,
        assertions: list,
        on_resynth=None,
        on_add_wm_hint=None,
    ) -> None:
        self._chat_panel.add_plan_synthesis(task_id, synthesis_text, assertions, on_resynth, on_add_wm_hint)

    def update_plan_synthesis(self, task_id: str, new_synthesis: str, assertions: list) -> None:
        self._chat_panel.update_plan_synthesis(task_id, new_synthesis, assertions)

    def mark_plan_node_invalidated(self, task_id: str) -> None:
        self._chat_panel.mark_plan_node_invalidated(task_id)

    def update_context_meter(self, max_tokens: int, breakdown: dict[str, int]) -> None:
        """Redraw the context meter donut chart with updated token data.

        Delegates to ``InputPanel.context_meter.update()`` which schedules
        the canvas redraw on the Tkinter main thread (ARCH-06).

        Args:
            max_tokens (int): Context-window size for the active model.
            breakdown (dict[str, int]): Per-category token estimates from
                ``Context.token_breakdown()``.
        """
        logger.debug("update_context_meter: max_tokens=%d breakdown=%s", max_tokens, breakdown)
        self._input_panel.context_meter.update(max_tokens, breakdown)

    # ── Placeholder callbacks (replaced at runtime via set_*_callback) ────────

    def _on_model_change(self, model: str) -> None:
        """Placeholder — replaced by set_model_change_callback()."""
        pass

    def _on_tool_toggle(self, tool_name: str, enabled: bool) -> None:
        """Placeholder — replaced by set_tool_toggle_callback()."""
        pass

    def _on_attachment_toggle(self, attachment_id: str, enabled: bool) -> None:
        """Class-method shadow — will only be called if no callback was passed at
        construction time (the instance attribute set in __init__ takes priority)."""
        if callable(getattr(self, "_on_attachment_toggle", None)):
            pass  # instance attribute handles it

    # ── Private helpers (stay here; used across panels via self._g) ───────────

    def _setup_fonts(self) -> tuple:
        """Determine and cache text font; result stored in self._text_font."""
        text_font = self.config.default_font
        if self.config.emoji_font_path:
            if os.path.exists(self.config.emoji_font_path):
                text_font = (self.config.emoji_font_path, self.config.default_font[1])
        self._text_font = text_font
        return text_font

    def _setup_window_geometry(self) -> None:
        """Configure window size and position from config ratios."""
        self.root.configure(bg=self.config.status_bg)
        screen_width = self.root.winfo_screenwidth()
        screen_height = self.root.winfo_screenheight()
        window_width = int(screen_width * self.config.window_width_ratio)
        window_height = int(screen_height * self.config.window_height_ratio)
        x_position = 0 if self.config.screen_side.lower() == "left" else screen_width - window_width
        self.root.geometry(f"{window_width}x{window_height}+{x_position}+0")
        self.root.title("AgentX - the Ollama Agent")
