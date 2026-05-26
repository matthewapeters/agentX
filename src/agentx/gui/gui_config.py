"""GUI configuration dataclass."""

from dataclasses import dataclass
from typing import Any, ClassVar, Optional


@dataclass
class GUIConfig:
    """Configuration for GUI appearance and behavior.

    This dataclass encapsulates all GUI-related configuration,
    extracted from the main application config dictionary.
    """

    # Window Configuration
    screen_side: str = "right"
    window_width_ratio: float = 0.5
    window_height_ratio: float = 1.0

    # Layout Configuration
    output_panel_ratio: float = 0.66
    attachment_bar_height: float = 0.03
    input_panel_height: float = 0.2
    session_section_spacing: int = 8

    # Font Configuration
    default_font: tuple = ("Terminal", 10)
    emoji_font_path: Optional[str] = None

    # Style Configuration
    PALETTES: ClassVar[dict[str, dict[str, str]]] = {
        "Dark Mode": {
            "output_bg": "#222222",
            "status_bg": "#333333",
            "input_bg": "#2a2a2a",
            "attachment_fg": "#eeeeee",
            "input_fg": "#eeeeee",
            "attachment_bg": "#444444",
            "history_attachment_bg": "#555555",
            "ui_fg": "#eeeeee",
            "muted_fg": "#bbbbbb",
            "user_prompt_fg": "#ffffff",
            "agent_response_fg": "#eeeeee",
            "agent_thinking_fg": "#cccccc",
            "agent_classification_fg": "#7dd3fc",
            "system_space_fg": "#888888",
        },
        "Light Mode": {
            "output_bg": "#ffffff",
            "status_bg": "#f0f4f8",
            "input_bg": "#f7f7f7",
            "attachment_fg": "#1f2937",
            "input_fg": "#111827",
            "attachment_bg": "#e5e7eb",
            "history_attachment_bg": "#dbe4ee",
            "ui_fg": "#111827",
            "muted_fg": "#4b5563",
            "user_prompt_fg": "#111827",
            "agent_response_fg": "#111827",
            "agent_thinking_fg": "#374151",
            "agent_classification_fg": "#0f766e",
            "system_space_fg": "#6b7280",
        },
    }

    theme_mode: str = "Dark Mode"
    output_bg: str = PALETTES["Dark Mode"]["output_bg"]
    status_bg: str = PALETTES["Dark Mode"]["status_bg"]
    input_bg: str = PALETTES["Dark Mode"]["input_bg"]
    attachment_fg: str = PALETTES["Dark Mode"]["attachment_fg"]
    input_fg: str = PALETTES["Dark Mode"]["input_fg"]
    attachment_bg: str = PALETTES["Dark Mode"]["attachment_bg"]
    history_attachment_bg: str = PALETTES["Dark Mode"]["history_attachment_bg"]
    ui_fg: str = PALETTES["Dark Mode"]["ui_fg"]
    muted_fg: str = PALETTES["Dark Mode"]["muted_fg"]
    user_prompt_fg: str = PALETTES["Dark Mode"]["user_prompt_fg"]
    agent_response_fg: str = PALETTES["Dark Mode"]["agent_response_fg"]
    agent_thinking_fg: str = PALETTES["Dark Mode"]["agent_thinking_fg"]
    agent_classification_fg: str = PALETTES["Dark Mode"]["agent_classification_fg"]
    system_space_fg: str = PALETTES["Dark Mode"]["system_space_fg"]

    # Text Style Configuration
    user_prompt_font: tuple[str, int, str] = ("Terminal", 10, "bold")
    agent_response_font: tuple[str, int, str] = ("Terminal", 10, "normal")
    agent_thinking_font: tuple[str, int, str] = ("Terminal", 10, "italic")
    gray_text_font: tuple[str, int, str] = ("Terminal", 10, "italic")

    # Markdown Rendering Configuration
    markdown_render_enabled: bool = True

    @classmethod
    def from_dict(cls, config: dict[str, Any]) -> "GUIConfig":
        """Create GUIConfig from application config dictionary.

        Args:
            config: Application configuration dictionary

        Returns:
            GUIConfig instance with values from config, or defaults
        """
        agentx = config.get("agentx", {})
        theme_mode = agentx.get("theme_mode", "Dark Mode")
        palette = cls.PALETTES.get(theme_mode, cls.PALETTES["Dark Mode"])
        return cls(
            screen_side=agentx.get("screen_side", "right"),
            window_width_ratio=agentx.get("window_width_ratio", 0.5),
            window_height_ratio=agentx.get("window_height_ratio", 1.0),
            output_panel_ratio=agentx.get("output_panel_ratio", 0.66),
            attachment_bar_height=agentx.get("attachment_bar_height", 0.03),
            input_panel_height=agentx.get("input_panel_height", 0.2),
            session_section_spacing=agentx.get("session_section_spacing", 8),
            default_font=tuple(agentx.get("default_font", ["Terminal", 10])),
            emoji_font_path=agentx.get("emoji_font_path", None),
            theme_mode=theme_mode,
            output_bg=agentx.get("output_bg", palette["output_bg"]),
            status_bg=agentx.get("status_bg", palette["status_bg"]),
            input_bg=agentx.get("input_bg", palette["input_bg"]),
            attachment_fg=agentx.get("attachment_fg", palette["attachment_fg"]),
            input_fg=agentx.get("input_fg", palette["input_fg"]),
            attachment_bg=agentx.get("attachment_bg", palette["attachment_bg"]),
            history_attachment_bg=agentx.get("history_attachment_bg", palette["history_attachment_bg"]),
            ui_fg=agentx.get("ui_fg", palette["ui_fg"]),
            muted_fg=agentx.get("muted_fg", palette["muted_fg"]),
            user_prompt_fg=agentx.get("user_prompt_fg", palette["user_prompt_fg"]),
            agent_response_fg=agentx.get("agent_response_fg", palette["agent_response_fg"]),
            agent_thinking_fg=agentx.get("agent_thinking_fg", palette["agent_thinking_fg"]),
            agent_classification_fg=agentx.get("agent_classification_fg", palette["agent_classification_fg"]),
            system_space_fg=agentx.get("system_space_fg", palette["system_space_fg"]),
            user_prompt_font=tuple(agentx.get("user_prompt_font", ["Terminal", 10, "bold"])),
            agent_response_font=tuple(agentx.get("agent_response_font", ["Terminal", 10, "normal"])),
            agent_thinking_font=tuple(agentx.get("agent_thinking_font", ["Terminal", 10, "italic"])),
            gray_text_font=tuple(agentx.get("gray_text_font", ["Terminal", 10, "italic"])),
            markdown_render_enabled=agentx.get("markdown_render_enabled", True),
        )
