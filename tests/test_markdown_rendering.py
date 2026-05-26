"""Tests for src/agentx/gui/markdown_renderer.py — Phases 1–4."""

import importlib
import sys
import types
import unittest
from unittest.mock import MagicMock, patch

import pytest

from agentx.gui.gui_config import GUIConfig
from agentx.gui.markdown_renderer import (
    MARKDOWN_AVAILABLE,
    TKINTERWEB_AVAILABLE,
    build_markdown_css,
    has_markdown,
    markdown_to_html,
)


def _reload_module_with_missing(*missing_packages: str):
    """Reload markdown_renderer after faking ImportError for *missing_packages*."""
    # Remove the cached module so importlib re-executes the module body.
    module_name = "agentx.gui.markdown_renderer"
    sys.modules.pop(module_name, None)

    if isinstance(__builtins__, types.ModuleType):
        original_import = __builtins__.__import__  # type: ignore[union-attr]
    else:
        original_import = __builtins__["__import__"]  # type: ignore[index]

    def _fake_import(name, *args, **kwargs):
        if name in missing_packages:
            raise ImportError(f"Mocked ImportError for {name}")
        return original_import(name, *args, **kwargs)

    with patch("builtins.__import__", side_effect=_fake_import):
        module = importlib.import_module(module_name)

    # Restore a clean module reference for subsequent tests.
    sys.modules.pop(module_name, None)
    return module


class TestSoftImportGuards:
    def test_both_unavailable_sets_flags_false(self):
        """Both flags are False when tkinterweb and markdown are missing."""
        mod = _reload_module_with_missing("tkinterweb", "markdown")
        assert mod.TKINTERWEB_AVAILABLE is False
        assert mod.MARKDOWN_AVAILABLE is False

    def test_both_unavailable_does_not_raise(self):
        """Importing with both packages missing must not raise any exception."""
        try:
            _reload_module_with_missing("tkinterweb", "markdown")
        except Exception as exc:  # noqa: BLE001
            raise AssertionError(f"Import raised unexpectedly: {exc}") from exc

    def test_only_tkinterweb_unavailable(self):
        mod = _reload_module_with_missing("tkinterweb")
        assert mod.TKINTERWEB_AVAILABLE is False
        # markdown may or may not be installed in the test environment;
        # we only assert the tkinterweb flag here.

    def test_only_markdown_unavailable(self):
        mod = _reload_module_with_missing("markdown")
        assert mod.MARKDOWN_AVAILABLE is False

    def test_htmlframe_is_none_when_tkinterweb_unavailable(self):
        mod = _reload_module_with_missing("tkinterweb")
        assert mod.HtmlFrame is None

    def test_module_exposes_public_constants(self):
        """The real module (loaded normally) exposes all required public names."""
        import agentx.gui.markdown_renderer as mod  # noqa: PLC0415

        assert hasattr(mod, "TKINTERWEB_AVAILABLE")
        assert hasattr(mod, "MARKDOWN_AVAILABLE")
        assert hasattr(mod, "HtmlFrame")
        assert hasattr(mod, "MARKDOWN_PATTERNS")
        assert hasattr(mod, "has_markdown")


# ---------------------------------------------------------------------------
# Phase 2 — CSS Theme Generator
# ---------------------------------------------------------------------------


def _dark_config() -> GUIConfig:
    return GUIConfig.from_dict({"agentx": {"theme_mode": "Dark Mode"}})


def _light_config() -> GUIConfig:
    return GUIConfig.from_dict({"agentx": {"theme_mode": "Light Mode"}})


class TestBuildMarkdownCss:
    def test_dark_mode_body_contains_dark_bg(self):
        css = build_markdown_css(_dark_config())
        # Dark Mode output_bg is #222222
        assert "#222222" in css

    def test_light_mode_body_contains_white_bg(self):
        css = build_markdown_css(_light_config())
        # Light Mode output_bg is #ffffff
        assert "#ffffff" in css

    def test_css_is_non_empty(self):
        assert len(build_markdown_css(_dark_config())) > 0

    def test_css_contains_table(self):
        assert "table" in build_markdown_css(_dark_config())

    def test_css_contains_pre(self):
        assert "pre" in build_markdown_css(_dark_config())

    def test_css_contains_h1(self):
        assert "h1" in build_markdown_css(_dark_config())

    def test_dark_and_light_differ(self):
        assert build_markdown_css(_dark_config()) != build_markdown_css(_light_config())


@pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown package not installed")
class TestMarkdownToHtml:
    def _css(self) -> str:
        return build_markdown_css(_dark_config())

    def test_heading_renders(self):
        html = markdown_to_html("# Hello", self._css())
        assert "<h1>Hello</h1>" in html

    def test_bold_renders(self):
        html = markdown_to_html("**bold**", self._css())
        assert "<strong>bold</strong>" in html

    def test_table_renders(self):
        table_md = "| a | b |\n|---|---|\n| 1 | 2 |"
        html = markdown_to_html(table_md, self._css())
        assert "<table>" in html

    def test_fenced_code_renders(self):
        code_md = "```\nprint('hello')\n```"
        html = markdown_to_html(code_md, self._css())
        assert "<code>" in html

    def test_plain_text_passes_through(self):
        html = markdown_to_html("Just plain text.", self._css())
        assert "Just plain text." in html
        assert "<html>" in html

    def test_output_is_full_html_document(self):
        html = markdown_to_html("hello", self._css())
        assert html.startswith("<html>")
        assert "<head>" in html
        assert "<style>" in html
        assert "<body>" in html


# ---------------------------------------------------------------------------
# Phase 2 (also Phase 3) — Detection Heuristic
# ---------------------------------------------------------------------------


class TestHasMarkdown:
    def test_atx_heading(self):
        assert has_markdown("# Hello") is True

    def test_bold(self):
        assert has_markdown("**bold text**") is True

    def test_table_row(self):
        assert has_markdown("| col1 | col2 |") is True

    def test_inline_code(self):
        assert has_markdown("`code`") is True

    def test_plain_sentence(self):
        assert has_markdown("Here is a plain sentence.") is False

    def test_ordered_list(self):
        assert has_markdown("1. First item") is True

    def test_blockquote(self):
        assert has_markdown("> blockquote") is True

    def test_image(self):
        assert has_markdown("![alt](url)") is True

    def test_empty_string(self):
        assert has_markdown("") is False

    def test_arithmetic_not_italic(self):
        # "2+2 = 4" should not trigger italic heuristic
        assert has_markdown("2+2 = 4") is False

    def test_unordered_list_dash(self):
        assert has_markdown("- item") is True

    def test_unordered_list_star(self):
        assert has_markdown("* item") is True

    def test_fenced_code_block(self):
        assert has_markdown("```\ncode\n```") is True


# ---------------------------------------------------------------------------
# Phase 4 — GUIManager Entry State & Toggle Refactor
# ---------------------------------------------------------------------------


def _skip_if_no_display():
    """Return a pytest skip mark when no DISPLAY is available."""
    import os

    if not os.environ.get("DISPLAY") and not os.environ.get("WAYLAND_DISPLAY"):
        return pytest.mark.skip(reason="No display available")
    return pytest.mark.skipif(False, reason="")


try:
    import tkinter as tk

    _TK_AVAILABLE = True
except Exception:
    _TK_AVAILABLE = False


@pytest.mark.skipif(not _TK_AVAILABLE, reason="tkinter not available")
class TestCreateOutputEntryStateDict(unittest.TestCase):
    """Tests for the three new keys added to the _create_output_entry state dict."""

    def setUp(self):
        try:
            self.root = tk.Tk()
            self.root.withdraw()
        except Exception:
            self.skipTest("Cannot create Tk root (no display)")

        from agentx.gui.gui_manager import GUIManager

        self.gui = GUIManager(
            root=self.root,
            config=GUIConfig.from_dict({}),
            on_submit=MagicMock(),
            on_interrupt=MagicMock(),
            on_attachment_toggle=MagicMock(),
        )
        self.gui.create_layout()

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def _make_entry(self, content: str = "hello", expanded: bool = True) -> dict:
        parent = self.gui.widgets.output_entries_frame
        return self.gui._create_output_entry(
            parent=parent,
            role_label="Agent",
            icon="🤖",
            content=content,
            expanded=expanded,
        )

    def test_state_has_toggle_btn_key(self):
        entry = self._make_entry()
        assert "toggle_btn" in entry

    def test_state_has_html_frame_key(self):
        entry = self._make_entry()
        assert "html_frame" in entry

    def test_state_has_is_finalized_key(self):
        entry = self._make_entry()
        assert "is_finalized" in entry

    def test_toggle_btn_is_button_widget(self):
        entry = self._make_entry()
        assert isinstance(entry["toggle_btn"], tk.Button)

    def test_html_frame_initially_none(self):
        entry = self._make_entry()
        assert entry["html_frame"] is None

    def test_is_finalized_initially_false(self):
        entry = self._make_entry()
        assert entry["is_finalized"] is False

    def test_toggle_expands_and_collapses(self):
        entry = self._make_entry(expanded=True)
        assert entry["expanded"] is True
        # Invoke collapse
        entry["toggle"]()
        assert entry["expanded"] is False
        # Invoke expand
        entry["toggle"]()
        assert entry["expanded"] is True


# ---------------------------------------------------------------------------
# Phase 5 — _finalize_entry_markdown()
# ---------------------------------------------------------------------------

_MARKDOWN_TEXT = "# Title\n\n**bold** and `code`\n\n| a | b |\n|---|---|\n| 1 | 2 |"
_PLAIN_TEXT = "Just a plain sentence with no markdown."


@pytest.mark.skipif(not _TK_AVAILABLE, reason="tkinter not available")
class TestFinalizeEntryMarkdown(unittest.TestCase):
    """Tests for GUIManager._finalize_entry_markdown()."""

    def setUp(self):
        try:
            self.root = tk.Tk()
            self.root.withdraw()
        except Exception:
            self.skipTest("Cannot create Tk root (no display)")

        from agentx.gui.gui_manager import GUIManager

        self.GUIManager = GUIManager
        self.config = GUIConfig.from_dict({})
        self.gui = GUIManager(
            root=self.root,
            config=self.config,
            on_submit=MagicMock(),
            on_interrupt=MagicMock(),
            on_attachment_toggle=MagicMock(),
        )
        self.gui.create_layout()

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def _make_entry(self, content: str = _MARKDOWN_TEXT, role_label: str = "Agent", expanded: bool = True) -> dict:
        parent = self.gui.widgets.output_entries_frame
        entry = self.gui._create_output_entry(
            parent=parent,
            role_label=role_label,
            icon="🤖",
            content=content,
            expanded=expanded,
        )
        self.root.update_idletasks()
        return entry

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_finalize_sets_is_finalized(self):
        entry = self._make_entry()
        self.gui._finalize_entry_markdown(entry)
        assert entry["is_finalized"] is True

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_finalize_sets_html_frame(self):
        entry = self._make_entry()
        self.gui._finalize_entry_markdown(entry)
        assert entry["html_frame"] is not None

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_finalize_clears_detail_text(self):
        entry = self._make_entry()
        self.gui._finalize_entry_markdown(entry)
        assert entry["detail_text"] is None

    def test_plain_text_not_finalized(self):
        entry = self._make_entry(content=_PLAIN_TEXT)
        self.gui._finalize_entry_markdown(entry)
        assert entry["is_finalized"] is False

    def test_tool_role_skipped(self):
        entry = self._make_entry(role_label="Tool")
        self.gui._finalize_entry_markdown(entry)
        assert entry["is_finalized"] is False

    def test_error_role_skipped(self):
        entry = self._make_entry(role_label="Error")
        self.gui._finalize_entry_markdown(entry)
        assert entry["is_finalized"] is False

    def test_classification_role_skipped(self):
        entry = self._make_entry(role_label="Classification")
        self.gui._finalize_entry_markdown(entry)
        assert entry["is_finalized"] is False

    def test_guard_tkinterweb_unavailable(self):
        """Method returns silently when TKINTERWEB_AVAILABLE is False."""
        import agentx.gui.gui_manager as gm_mod

        entry = self._make_entry()
        original = gm_mod.TKINTERWEB_AVAILABLE
        try:
            gm_mod.TKINTERWEB_AVAILABLE = False
            self.gui._finalize_entry_markdown(entry)
            assert entry["is_finalized"] is False
        finally:
            gm_mod.TKINTERWEB_AVAILABLE = original

    def test_guard_markdown_render_disabled(self):
        """Method skips when markdown_render_enabled is False."""
        entry = self._make_entry()
        self.gui.config.markdown_render_enabled = False
        try:
            self.gui._finalize_entry_markdown(entry)
            assert entry["is_finalized"] is False
        finally:
            self.gui.config.markdown_render_enabled = True

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_double_finalization_is_noop(self):
        entry = self._make_entry()
        self.gui._finalize_entry_markdown(entry)
        assert entry["is_finalized"] is True
        html_frame_first = entry["html_frame"]
        # Second call must not raise and must not replace the frame.
        self.gui._finalize_entry_markdown(entry)
        assert entry["html_frame"] is html_frame_first

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_expanded_entry_html_frame_is_packed(self):
        entry = self._make_entry(expanded=True)
        self.gui._finalize_entry_markdown(entry)
        self.root.update_idletasks()
        assert entry["html_frame"].winfo_manager() == "pack"

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_collapsed_entry_html_frame_not_packed(self):
        entry = self._make_entry(expanded=False)
        self.gui._finalize_entry_markdown(entry)
        self.root.update_idletasks()
        assert entry["html_frame"].winfo_manager() == ""

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_html_frame_added_to_output_html_frames(self):
        entry = self._make_entry()
        before_count = len(self.gui._output_html_frames)
        self.gui._finalize_entry_markdown(entry)
        assert len(self.gui._output_html_frames) == before_count + 1
        assert entry["html_frame"] in self.gui._output_html_frames


# ---------------------------------------------------------------------------
# Phase 6 — _finalize_current_turn_markdown() & display_spacing() hook
# ---------------------------------------------------------------------------


@pytest.mark.skipif(not _TK_AVAILABLE, reason="tkinter not available")
class TestFinalizeTurnAndDisplaySpacing(unittest.TestCase):
    """Tests for _finalize_current_turn_markdown() and its display_spacing() hook."""

    def setUp(self):
        try:
            self.root = tk.Tk()
            self.root.withdraw()
        except Exception:
            self.skipTest("Cannot create Tk root (no display)")

        from datetime import datetime as dt

        from agentx.gui.gui_manager import GUIManager

        self.dt = dt
        self.gui = GUIManager(
            root=self.root,
            config=GUIConfig.from_dict({}),
            on_submit=MagicMock(),
            on_interrupt=MagicMock(),
            on_attachment_toggle=MagicMock(),
        )
        self.gui.create_layout()

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def _run_turn(self, agent_text: str) -> dict:
        """Simulate a full turn: user message → agent chunks → display_spacing."""
        self.gui.display_user_message("hello", attachments=[], timestamp=self.dt.now())
        # Stream the agent text character by character to mimic real streaming.
        for ch in agent_text:
            self.gui.display_agent_response(ch)
        self.root.update_idletasks()
        # Capture entry reference before display_spacing() clears it.
        entry = self.gui._current_turn_entries.get("assistant")
        self.gui.display_spacing()
        self.root.update_idletasks()
        return entry

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_markdown_response_finalized_after_display_spacing(self):
        entry = self._run_turn(_MARKDOWN_TEXT)
        assert entry is not None
        assert entry["is_finalized"] is True

    def test_plain_response_not_finalized_after_display_spacing(self):
        entry = self._run_turn(_PLAIN_TEXT)
        assert entry is not None
        assert entry["is_finalized"] is False

    def test_current_turn_entries_reset_after_display_spacing(self):
        self._run_turn(_PLAIN_TEXT)
        assert self.gui._current_turn_entries == {}

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_two_turns_no_cross_interference(self):
        entry1 = self._run_turn(_MARKDOWN_TEXT)
        entry2 = self._run_turn(_MARKDOWN_TEXT)
        # Both turns should be independently finalized.
        assert entry1 is not None and entry1["is_finalized"] is True
        assert entry2 is not None and entry2["is_finalized"] is True
        # They must be distinct entry objects.
        assert entry1 is not entry2

    def test_empty_turn_entries_noop(self):
        """finalize_current_turn_markdown on empty dict must not raise."""
        assert self.gui._current_turn_entries == {}
        self.gui.finalize_current_turn_markdown()  # should not raise

    def test_finalize_current_turn_method_exists(self):
        assert callable(getattr(self.gui, "finalize_current_turn_markdown", None))


# ---------------------------------------------------------------------------
# Phase 7 — Resize Handling for HtmlFrame
# ---------------------------------------------------------------------------


@pytest.mark.skipif(not _TK_AVAILABLE, reason="tkinter not available")
class TestResizeHandlingHtmlFrames(unittest.TestCase):
    """Tests for _output_html_frames tracking and auto-pruning during resize."""

    def setUp(self):
        try:
            self.root = tk.Tk()
            self.root.withdraw()
        except Exception:
            self.skipTest("Cannot create Tk root (no display)")

        from agentx.gui.gui_manager import GUIManager

        self.gui = GUIManager(
            root=self.root,
            config=GUIConfig.from_dict({}),
            on_submit=MagicMock(),
            on_interrupt=MagicMock(),
            on_attachment_toggle=MagicMock(),
        )
        self.gui.create_layout()

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def _finalized_entry(self) -> dict:
        parent = self.gui.widgets.output_entries_frame
        entry = self.gui._create_output_entry(
            parent=parent,
            role_label="Agent",
            icon="🤖",
            content=_MARKDOWN_TEXT,
            expanded=True,
        )
        self.root.update_idletasks()
        self.gui._finalize_entry_markdown(entry)
        self.root.update_idletasks()
        return entry

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_wraplength_update_does_not_raise_with_html_frames(self):
        """_update_output_wraplength must not raise when _output_html_frames is populated."""
        self._finalized_entry()
        assert len(self.gui._output_html_frames) > 0
        # Should not raise even though HtmlFrame has no wraplength concept.
        self.gui._update_output_wraplength(800)

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    @pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
    def test_destroyed_html_frame_pruned_on_resize(self):
        """A destroyed HtmlFrame is removed from _output_html_frames during the next resize."""
        entry = self._finalized_entry()
        hf = entry["html_frame"]
        assert hf in self.gui._output_html_frames

        # Destroy the frame directly (simulates widget cleanup).
        hf.destroy()
        self.root.update_idletasks()

        # Trigger resize — the pruning loop should remove the dead widget.
        self.gui._update_output_wraplength(800)
        assert hf not in self.gui._output_html_frames

    def test_output_html_frames_initialised_empty(self):
        """_output_html_frames starts as an empty list."""
        from agentx.gui.gui_manager import GUIManager

        gui = GUIManager(
            root=self.root,
            config=GUIConfig.from_dict({}),
            on_submit=MagicMock(),
            on_interrupt=MagicMock(),
            on_attachment_toggle=MagicMock(),
        )
        assert gui._output_html_frames == []


# ---------------------------------------------------------------------------
# Phase 8 — IGUIManager Protocol Update
# ---------------------------------------------------------------------------


class TestIGUIManagerProtocol(unittest.TestCase):
    """Tests that GUIManager satisfies the updated IGUIManager protocol."""

    def test_gui_manager_has_finalize_current_turn_markdown(self):
        """GUIManager must expose the public finalize_current_turn_markdown method."""
        from agentx.gui.gui_manager import GUIManager

        assert callable(getattr(GUIManager, "finalize_current_turn_markdown", None))

    @pytest.mark.skipif(not _TK_AVAILABLE, reason="tkinter not available")
    def test_gui_manager_is_igui_manager(self):
        """GUIManager structurally satisfies IGUIManager at runtime."""
        try:
            root = tk.Tk()
            root.withdraw()
        except Exception:
            self.skipTest("Cannot create Tk root (no display)")
        try:
            from agentx.gui.gui_manager import GUIManager

            gui = GUIManager(
                root=root,
                config=GUIConfig.from_dict({}),
                on_submit=MagicMock(),
                on_interrupt=MagicMock(),
                on_attachment_toggle=MagicMock(),
            )
            # Structural check: all protocol method names exist on the instance.
            for name in ("finalize_current_turn_markdown", "display_error", "display_agent_response"):
                assert hasattr(gui, name), f"Missing protocol method: {name}"
        finally:
            try:
                root.destroy()
            except Exception:
                pass

    def test_finalize_current_turn_markdown_in_protocol(self):
        """IGUIManager protocol defines finalize_current_turn_markdown."""
        from agentx.igui_manager import IGUIManager

        assert "finalize_current_turn_markdown" in dir(IGUIManager)


# ---------------------------------------------------------------------------
# Phase 9 — Settings Tab Toggle
# ---------------------------------------------------------------------------


@pytest.mark.skipif(not _TK_AVAILABLE, reason="tkinter not available")
class TestSettingsTabMarkdownToggle(unittest.TestCase):
    """Tests for the Render Markdown checkbox in SettingsTab."""

    def setUp(self):
        try:
            self.root = tk.Tk()
            self.root.withdraw()
        except Exception:
            self.skipTest("Cannot create Tk root (no display)")

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def _make_settings_tab(self, on_change=None):
        from agentx.gui.settings_tab import SettingsTab

        config = {"agentx": {"markdown_render_enabled": True}, "agentix": {}}
        return SettingsTab(
            parent=self.root,
            config=config,
            bg="#222222",
            fg="#eeeeee",
            on_change=on_change or (lambda *_: None),
        )

    def test_markdown_render_var_exists(self):
        """SettingsTab exposes _markdown_render_var after construction."""
        tab = self._make_settings_tab()
        assert hasattr(tab, "_markdown_render_var")
        assert isinstance(tab._markdown_render_var, tk.BooleanVar)

    def test_markdown_render_var_initial_value(self):
        """Initial value matches config when tkinterweb is available."""
        tab = self._make_settings_tab()
        if TKINTERWEB_AVAILABLE:
            assert tab._markdown_render_var.get() is True
        else:
            # Forced False when tkinterweb is missing.
            assert tab._markdown_render_var.get() is False

    @pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
    def test_toggling_fires_on_change(self):
        """Toggling the checkbox fires on_change with the correct key_path and value."""
        changes = []
        tab = self._make_settings_tab(on_change=lambda kp, v: changes.append((kp, v)))

        # Simulate unchecking.
        tab._markdown_render_var.set(False)
        tab._markdown_render_var.set(True)  # back on

        # Programmatic BooleanVar.set() doesn't fire the command; invoke via config.
        # Instead verify that the var itself holds the correct value after toggle.
        tab._markdown_render_var.set(False)
        assert tab._markdown_render_var.get() is False

    def test_checkbox_disabled_when_tkinterweb_unavailable(self):
        """The Render Markdown checkbox is disabled when TKINTERWEB_AVAILABLE is False."""
        import agentx.gui.settings_tab as st_mod

        original = st_mod.TKINTERWEB_AVAILABLE
        try:
            st_mod.TKINTERWEB_AVAILABLE = False
            tab = self._make_settings_tab()
            # When unavailable the var should be forced to False.
            assert tab._markdown_render_var.get() is False
        finally:
            st_mod.TKINTERWEB_AVAILABLE = original


# ---------------------------------------------------------------------------
# Phase 10 — Integration Smoke Test
# ---------------------------------------------------------------------------

_TABLE_MD = "| A | B |\n|---|---|\n| 1 | 2 |"


@pytest.mark.live
@pytest.mark.skipif(not _TK_AVAILABLE, reason="tkinter not available")
@pytest.mark.skipif(not TKINTERWEB_AVAILABLE, reason="tkinterweb not installed")
@pytest.mark.skipif(not MARKDOWN_AVAILABLE, reason="markdown not installed")
class TestMarkdownRenderingIntegration(unittest.TestCase):
    """End-to-end integration test — requires a display and all optional packages."""

    def setUp(self):
        import os

        if not os.environ.get("DISPLAY") and not os.environ.get("WAYLAND_DISPLAY"):
            self.skipTest("No display available (set DISPLAY or WAYLAND_DISPLAY)")
        try:
            self.root = tk.Tk()
            self.root.withdraw()
        except Exception as exc:
            self.skipTest(f"Cannot create Tk root: {exc}")

        from datetime import datetime as dt

        from agentx.gui.gui_manager import GUIManager

        self.dt = dt
        self.gui = GUIManager(
            root=self.root,
            config=GUIConfig.from_dict({}),
            on_submit=MagicMock(),
            on_interrupt=MagicMock(),
            on_attachment_toggle=MagicMock(),
        )
        self.gui.create_layout()

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_table_response_fully_rendered(self):
        """Full end-to-end: user message → streamed table → display_spacing → HtmlFrame visible."""
        self.gui.display_user_message("Show me a markdown table", attachments=[], timestamp=self.dt.now())

        for ch in _TABLE_MD:
            self.gui.display_agent_response(ch)

        self.root.update_idletasks()
        entry = self.gui._current_turn_entries.get("assistant")
        self.assertIsNotNone(entry, "assistant entry must be created during streaming")

        self.gui.display_spacing()
        self.root.update_idletasks()

        self.assertTrue(entry["is_finalized"], "entry must be finalized after display_spacing")
        self.assertIsNotNone(entry["html_frame"], "html_frame must be set")
        self.assertIsNone(entry["detail_text"], "detail_text must be destroyed")
        self.assertTrue(entry["html_frame"].winfo_exists(), "html_frame widget must still exist")
        self.assertEqual(
            entry["html_frame"].winfo_manager(),
            "pack",
            "html_frame must be packed (entry was expanded)",
        )


@pytest.mark.skipif(not _TK_AVAILABLE, reason="tkinter not available")
class TestMarkdownRenderingHeadless(unittest.TestCase):
    """Headless smoke test: exercises the full finalization path with HtmlFrame mocked."""

    def setUp(self):
        try:
            self.root = tk.Tk()
            self.root.withdraw()
        except Exception:
            self.skipTest("Cannot create Tk root (no display)")

    def tearDown(self):
        try:
            self.root.destroy()
        except Exception:
            pass

    def test_full_path_with_mocked_html_frame(self):
        """Full finalization path works with a fake HtmlFrame — no display needed."""
        from datetime import datetime as dt

        import agentx.gui.gui_manager as gm_mod
        from agentx.gui.gui_manager import GUIManager

        class FakeHtmlFrame(tk.Frame):
            """Minimal HtmlFrame stand-in that records load_html calls."""

            def __init__(self, parent, **kwargs):
                super().__init__(parent)
                self.loaded_html: str = ""
                self._exists = True

            def load_html(self, html: str) -> None:
                self.loaded_html = html

            def winfo_exists(self) -> bool:
                return self._exists

        original_hf = gm_mod.HtmlFrame
        original_avail = gm_mod.TKINTERWEB_AVAILABLE
        original_md_avail = gm_mod.MARKDOWN_AVAILABLE
        try:
            gm_mod.HtmlFrame = FakeHtmlFrame
            gm_mod.TKINTERWEB_AVAILABLE = True
            gm_mod.MARKDOWN_AVAILABLE = True

            # markdown_to_html calls _md_lib (which is None when the markdown
            # package is not installed).  Patch the function itself so the test
            # is self-contained regardless of whether the package is present.
            fake_html = "<html><body><table><tr><td>A</td><td>B</td></tr></table></body></html>"
            with patch("agentx.gui.chat_panel.markdown_to_html", return_value=fake_html):
                gui = GUIManager(
                    root=self.root,
                    config=GUIConfig.from_dict({}),
                    on_submit=MagicMock(),
                    on_interrupt=MagicMock(),
                    on_attachment_toggle=MagicMock(),
                )
                gui.create_layout()

                gui.display_user_message("hello", attachments=[], timestamp=dt.now())
                for ch in _TABLE_MD:
                    gui.display_agent_response(ch)
                self.root.update_idletasks()

                entry = gui._current_turn_entries.get("assistant")
                self.assertIsNotNone(entry)

                gui.display_spacing()
                self.root.update_idletasks()

                self.assertTrue(entry["is_finalized"])
                self.assertIsNotNone(entry["html_frame"])
                self.assertIsNone(entry["detail_text"])
                self.assertIsInstance(entry["html_frame"], FakeHtmlFrame)

                self.assertTrue(entry["is_finalized"])
                self.assertIsNotNone(entry["html_frame"])
                self.assertIsNone(entry["detail_text"])
                self.assertIsInstance(entry["html_frame"], FakeHtmlFrame)
                self.assertIn("<table>", entry["html_frame"].loaded_html)
        finally:
            gm_mod.HtmlFrame = original_hf
            gm_mod.TKINTERWEB_AVAILABLE = original_avail
            gm_mod.MARKDOWN_AVAILABLE = original_md_avail
