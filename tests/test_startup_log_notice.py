"""Unit tests for startup log-location notice behavior.

Affordance under test:
  - PD-01-AF-009: Startup output includes friendly log-file locations message,
    controlled by `agentx.show_log_locations_on_startup`.
"""

from __future__ import annotations

import tkinter as tk
import tkinter.font as tkfont
from pathlib import Path
from unittest.mock import Mock, patch

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
        with patch("agentx.model_metadata_store.ModelMetadataStore.populate"):
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

    def test_notice_enabled_mirrors_to_tui_output_when_bridge_present(self, tmp_path: Path) -> None:
        """GIVEN show_log_locations_on_startup is enabled [PD-01-AF-009]
        WHEN _show_startup_log_locations_notice_if_enabled() runs with a TUI bridge
        THEN the startup notice is mirrored to the TUI output as a system record.
        """
        session = self._create_session(tmp_path, self._build_config(show_notice=True))
        try:
            session.tui_bridge = Mock()
            with (
                patch.object(session.gui, "display_startup_notice") as mock_notice,
                patch.object(session._output_logger, "log") as mock_log,
            ):
                session._show_startup_log_locations_notice_if_enabled()

            mock_notice.assert_called_once()
            content = mock_notice.call_args.args[0]
            session.tui_bridge.write_output.assert_called_once_with(f"###SYSTEM Startup\n{content}\n")
            mock_log.assert_called_once_with("startup", content)
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
            # icon_text is now a tk.Text widget (selectable) — not a tk.Label
            icon_texts = []
            for child in output_entries.winfo_children():
                for grandchild in child.winfo_children():
                    for widget in grandchild.winfo_children():
                        if isinstance(widget, tk.Text) and widget.get("1.0", tk.END).strip() == "ⓘ":
                            icon_texts.append(widget)
            assert icon_texts, "Expected startup icon Text widget in output entries"

            icon_text_widget = icon_texts[-1]
            icon_font = tkfont.Font(font=icon_text_widget.cget("font"))
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

    def test_startup_notice_detail_is_selectable_text_with_context_binding(self, tmp_path: Path) -> None:
        """GIVEN startup notice rendering [PD-01-AF-009]
        WHEN display_startup_notice() creates the detail body
        THEN detail is a disabled Text widget containing content and bound for right-click copy.
        """
        session = self._create_session(tmp_path, self._build_config(show_notice=True))
        try:
            session.gui.create_layout()
            message = "Log files for this session"
            session.gui.display_startup_notice(message)

            detail_widgets = session.gui._chat_panel._output_detail_text_widgets
            assert detail_widgets, "Expected startup detail Text widget to be tracked"
            detail_widget = detail_widgets[-1]
            assert detail_widget.get("1.0", tk.END).strip() == message
            assert str(detail_widget.cget("state")) == "disabled"
            assert "<Button-3>" in detail_widget.bind()
        finally:
            session.close()
            session.root.destroy()
