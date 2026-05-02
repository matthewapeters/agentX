"""Unit tests for startup log-location notice behavior.

Affordance under test:
  - PD-01-AF-009: Startup output includes friendly log-file locations message,
    controlled by `agentx.show_log_locations_on_startup`.
"""

from __future__ import annotations

import tkinter as tk
import tkinter.font as tkfont
from pathlib import Path
from unittest.mock import patch

import pytest

from agentx.session import AgentXSession


@pytest.mark.unit
class TestStartupLogNotice:
    """Hermetic tests for startup log-location notice emission."""

    def _build_config(self, show_notice: bool | None = None) -> dict:
        config = {
            "agentx": {
                "ollama_host": "localhost:11434",
                "ollama_model": "mistral",
            },
            "agentix": {
                "host": "localhost:8000",
            },
        }
        if show_notice is not None:
            config["agentx"]["show_log_locations_on_startup"] = show_notice
        return config

    def _create_session(self, tmp_path: Path, config: dict) -> AgentXSession:
        root = tk.Tk()
        root.withdraw()
        return AgentXSession(root=root, config=config, username="tester", session_dir=str(tmp_path))

    def test_notice_enabled_by_default_emits_friendly_paths(self, tmp_path: Path) -> None:
        """GIVEN show_log_locations_on_startup is absent [PD-01-AF-009]
        WHEN _show_startup_log_locations_notice_if_enabled() runs
        THEN a startup notice is displayed and includes expected log locations.
        """
        session = self._create_session(tmp_path, self._build_config(show_notice=None))
        try:
            with (
                patch.object(session.gui, "display_startup_notice") as mock_notice,
                patch.object(session._output_logger, "log") as mock_log,
            ):
                session._show_startup_log_locations_notice_if_enabled()

            mock_notice.assert_called_once()
            content = mock_notice.call_args.args[0]
            assert "Session transcript:" in content
            assert "output_log.jsonl" in content
            assert "agentx.log" in content
            assert "classification.jsonl" in content
            mock_log.assert_called_once_with("startup", content)
        finally:
            session.close()
            session.root.destroy()

    def test_notice_can_be_disabled_via_agentx_toml_setting(self, tmp_path: Path) -> None:
        """GIVEN show_log_locations_on_startup is false [PD-01-AF-009]
        WHEN _show_startup_log_locations_notice_if_enabled() runs
        THEN no startup notice is rendered.
        """
        session = self._create_session(tmp_path, self._build_config(show_notice=False))
        try:
            with (
                patch.object(session.gui, "display_startup_notice") as mock_notice,
                patch.object(session._output_logger, "log") as mock_log,
            ):
                session._show_startup_log_locations_notice_if_enabled()

            mock_notice.assert_not_called()
            mock_log.assert_not_called()
        finally:
            session.close()
            session.root.destroy()

    def test_layout_displays_notice_before_bootstrap_response(self, tmp_path: Path) -> None:
        """GIVEN startup layout initialization [PD-01-AF-009]
        WHEN layout() executes
        THEN startup log notice is emitted before bootstrap prompt rendering.
        """
        session = self._create_session(tmp_path, self._build_config(show_notice=True))
        try:
            events: list[str] = []
            with (
                patch.object(session.gui, "create_layout"),
                patch.object(session, "refresh_context_gui"),
                patch.object(session, "refresh_files_gui"),
                patch.object(session, "refresh_working_memory_gui"),
                patch.object(session, "refresh_settings_gui"),
                patch.object(session, "_setup_agentix_ui"),
                patch.object(
                    session,
                    "_show_startup_log_locations_notice_if_enabled",
                    side_effect=lambda: events.append("notice"),
                ),
                patch.object(
                    session,
                    "_run_bootstrap_prompt_if_present",
                    side_effect=lambda: events.append("bootstrap"),
                ),
            ):
                session.layout()

            assert events == ["notice", "bootstrap"]
        finally:
            session.close()
            session.root.destroy()

    def test_startup_notice_icon_is_circled_i_bold_and_larger(self, tmp_path: Path) -> None:
        """GIVEN startup notice rendering [PD-01-AF-009]
        WHEN display_startup_notice() is called
        THEN icon uses ⓘ and is bold/larger than base text font.
        """
        session = self._create_session(tmp_path, self._build_config(show_notice=True))
        try:
            session.gui.create_layout()
            session.gui.display_startup_notice("Log files for this session")

            output_text = session.gui.widgets.output_text.get("1.0", tk.END)
            assert "ⓘ Startup:" in output_text

            output_entries = session.gui.widgets.output_entries_frame
            assert output_entries is not None
            icon_labels = []
            for child in output_entries.winfo_children():
                for grandchild in child.winfo_children():
                    for widget in grandchild.winfo_children():
                        if isinstance(widget, tk.Label) and widget.cget("text") == "ⓘ":
                            icon_labels.append(widget)
            assert icon_labels, "Expected startup icon label in output entries"

            icon_label = icon_labels[-1]
            icon_font = tkfont.Font(font=icon_label.cget("font"))
            icon_actual = icon_font.actual()
            text_font = session.gui._text_font or session.gui.config.default_font
            base_size = 10
            if isinstance(text_font, tuple) and len(text_font) >= 2 and isinstance(text_font[1], int):
                base_size = text_font[1]

            assert icon_actual.get("weight") == "bold"
            assert int(icon_actual.get("size", base_size)) >= base_size + 1
        finally:
            session.close()
            session.root.destroy()
