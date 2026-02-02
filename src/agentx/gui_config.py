"""GUI configuration dataclass."""

from dataclasses import dataclass
from typing import Any, Optional


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

    # Font Configuration
    default_font: tuple = ("Terminal", 10)
    emoji_font_path: Optional[str] = None

    # Style Configuration
    output_bg: str = "white"
    status_bg: str = "lightblue"
    input_bg: str = "lightgrey"
    attachment_bg: str = "white"
    history_attachment_bg: str = "#f0f0f0"

    # Text Style Configuration
    user_prompt_font: tuple[str, int, str] = ("Terminal", 10, "bold")
    agent_response_font: tuple[str, int, str] = ("Terminal", 10, "normal")
    agent_thinking_font: tuple[str, int, str] = ("Terminal", 10, "italic")
    gray_text_font: tuple[str, int, str] = ("Terminal", 10, "italic")

    @classmethod
    def from_dict(cls, config: dict[str, Any]) -> "GUIConfig":
        """Create GUIConfig from application config dictionary.

        Args:
            config: Application configuration dictionary

        Returns:
            GUIConfig instance with values from config, or defaults
        """
        agentx = config.get("agentx", {})
        return cls(
            screen_side=agentx.get("screen_side", "right"),
            window_width_ratio=agentx.get("window_width_ratio", 0.5),
            window_height_ratio=agentx.get("window_height_ratio", 1.0),
            output_panel_ratio=agentx.get("output_panel_ratio", 0.66),
            attachment_bar_height=agentx.get("attachment_bar_height", 0.03),
            input_panel_height=agentx.get("input_panel_height", 0.2),
            default_font=tuple(agentx.get("default_font", ["Terminal", 10])),
            emoji_font_path=agentx.get("emoji_font_path", None),
            output_bg=agentx.get("output_bg", "white"),
            status_bg=agentx.get("status_bg", "lightblue"),
            input_bg=agentx.get("input_bg", "lightgrey"),
            attachment_bg=agentx.get("attachment_bg", "white"),
            history_attachment_bg=agentx.get("history_attachment_bg", "#f0f0f0"),
            user_prompt_font=tuple(
                agentx.get("user_prompt_font", ["Terminal", 10, "bold"])
            ),
            agent_response_font=tuple(
                agentx.get("agent_response_font", ["Terminal", 10, "normal"])
            ),
            agent_thinking_font=tuple(
                agentx.get("agent_thinking_font", ["Terminal", 10, "italic"])
            ),
            gray_text_font=tuple(
                agentx.get("gray_text_font", ["Terminal", 10, "italic"])
            ),
        )
