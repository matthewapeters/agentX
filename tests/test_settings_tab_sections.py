"""
Tests for SettingsTab section collapse defaults (PD-07-AF-002)
and restart-required icon in label text (PD-07-AF-003).

Affordance IDs: PD-07-AF-002, PD-07-AF-003
"""

from __future__ import annotations

import tkinter as tk
import unittest
from unittest.mock import patch

import pytest

from agentx.gui.settings_tab import SettingsTab

# ---------------------------------------------------------------------------
# Shared helpers
# ---------------------------------------------------------------------------

_MINIMAL_CONFIG: dict = {
    "agentx": {
        "theme_mode": "Dark Mode",
        "ollama_host": "localhost:11434",
        "ollama_model": "llama3",
        "ollama_initial_load_timeout_seconds": 120,
        "screen_side": "left",
        "markdown_render_enabled": True,
        "working_memory": {
            "enabled": True,
            "inject_into_context": True,
            "max_facts": 50,
        },
    },
    "agentix": {
        "host": "localhost:8000",
        "classify_prompts": True,
        "debug": False,
        "classification_backend": "ollama",
        "classification_torch_model": "",
        "classification_torch_device": -1,
        "default_system_prompts": [],
        "classification_display": {
            "enabled": True,
            "show_intent": True,
            "show_reasoning": True,
            "show_clarification": True,
            "show_next_step": True,
        },
    },
}


def _make_settings_tab(root: tk.Tk) -> SettingsTab:
    """Construct a SettingsTab with minimal config."""
    return SettingsTab(
        parent=root,
        config=_MINIMAL_CONFIG,
        bg="#222222",
        fg="#eeeeee",
        on_change=lambda *_: None,
    )


def _collect_text_widgets(widget: tk.Widget) -> list[tk.Widget]:
    """Recursively collect all Label and Checkbutton widgets under *widget*."""
    results: list[tk.Widget] = []
    if isinstance(widget, (tk.Label, tk.Checkbutton)):
        results.append(widget)
    for child in widget.winfo_children():
        results.extend(_collect_text_widgets(child))
    return results


# ---------------------------------------------------------------------------
# PD-07-AF-002 — Section collapse defaults
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestSettingsTabSectionCollapseDefaults(unittest.TestCase):
    """
    Unit tests for PD-07-AF-002: SettingsTab section collapse defaults.

    Unit under test: SettingsTab._make_section() / CollapsibleSection.expanded

    GIVEN the SettingsTab is constructed with default config
    WHEN we inspect the CollapsibleSection objects captured during __init__
    THEN each section has the expected initial expanded/collapsed state.
    """

    def setUp(self) -> None:
        self.root = tk.Tk()
        self.root.withdraw()
        self.sections: dict[str, object] = {}

        from agentx.gui.collapsible_section import CollapsibleSection
        from agentx.gui.settings_tab import SettingsTab as _ST

        _original_make_section = _ST._make_section

        captured: dict[str, object] = self.sections

        def _capturing_make_section(
            self_tab: SettingsTab, title: str, initial_collapsed: bool = True
        ) -> CollapsibleSection:
            section = _original_make_section(self_tab, title, initial_collapsed=initial_collapsed)
            captured[title] = section
            return section

        with patch.object(_ST, "_make_section", _capturing_make_section):
            self.tab = _make_settings_tab(self.root)

    def tearDown(self) -> None:
        self.root.destroy()

    # ---- Expanded by default ------------------------------------------------

    def test_appearance_section_expanded_by_default(self) -> None:
        """
        PD-07-AF-002 — Appearance section is expanded on first load.

        GIVEN the SettingsTab is constructed
        WHEN the 🎨 Appearance section is inspected
        THEN CollapsibleSection.expanded is True (initial_collapsed=False)
        """
        section = self.sections.get("🎨 Appearance")
        self.assertIsNotNone(section, "Appearance section not captured")
        self.assertTrue(section.expanded)  # type: ignore[union-attr]

    def test_ollama_section_expanded_by_default(self) -> None:
        """
        PD-07-AF-002 — Ollama section is expanded on first load.

        GIVEN the SettingsTab is constructed
        WHEN the 🤖 Ollama section is inspected
        THEN CollapsibleSection.expanded is True (initial_collapsed=False)
        """
        section = self.sections.get("🤖 Ollama")
        self.assertIsNotNone(section, "Ollama section not captured")
        self.assertTrue(section.expanded)  # type: ignore[union-attr]

    def test_agentix_section_expanded_by_default(self) -> None:
        """
        PD-07-AF-002 — Agentix section is expanded on first load.

        GIVEN the SettingsTab is constructed
        WHEN the 🧠 Agentix section is inspected
        THEN CollapsibleSection.expanded is True (initial_collapsed=False)
        """
        section = self.sections.get("🧠 Agentix")
        self.assertIsNotNone(section, "Agentix section not captured")
        self.assertTrue(section.expanded)  # type: ignore[union-attr]

    # ---- Collapsed by default -----------------------------------------------

    def test_classification_display_section_collapsed_by_default(self) -> None:
        """
        PD-07-AF-002 — Classification Display section is collapsed on first load.

        GIVEN the SettingsTab is constructed
        WHEN the 📊 Classification Display section is inspected
        THEN CollapsibleSection.expanded is False (initial_collapsed=True)
        """
        section = self.sections.get("📊 Classification Display")
        self.assertIsNotNone(section, "Classification Display section not captured")
        self.assertFalse(section.expanded)  # type: ignore[union-attr]

    def test_working_memory_section_collapsed_by_default(self) -> None:
        """
        PD-07-AF-002 — Working Memory section is collapsed on first load.

        GIVEN the SettingsTab is constructed
        WHEN the 🏛️ Working Memory section is inspected
        THEN CollapsibleSection.expanded is False (initial_collapsed=True)
        """
        section = self.sections.get("🏛️ Working Memory")
        self.assertIsNotNone(section, "Working Memory section not captured")
        self.assertFalse(section.expanded)  # type: ignore[union-attr]


# ---------------------------------------------------------------------------
# PD-07-AF-003 — Restart-required icon in label text
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestRestartIconInLabels(unittest.TestCase):
    """
    Unit tests for PD-07-AF-003: Restart-required fields display 🔁 icon.

    Unit under test: SettingsTab.RESTART_ICON / _add_checkbox / _add_text_entry /
                     _add_spinbox / _add_enum_dropdown / _add_model_dropdown

    GIVEN the SettingsTab is constructed
    WHEN we inspect Label / Checkbutton widget text throughout the widget tree
    THEN restart-required fields carry the 🔁 suffix and non-restart fields do not.
    """

    def setUp(self) -> None:
        self.root = tk.Tk()
        self.root.withdraw()
        self.tab = _make_settings_tab(self.root)
        self._all_texts = [w.cget("text") for w in _collect_text_widgets(self.tab.frame)]

    def tearDown(self) -> None:
        self.root.destroy()

    # ---- RESTART_ICON constant ----------------------------------------------

    def test_restart_icon_constant_value(self) -> None:
        """
        PD-07-AF-003 — RESTART_ICON class constant has the expected value.

        GIVEN the SettingsTab class
        WHEN the RESTART_ICON constant is read
        THEN it equals ' 🔁'
        """
        self.assertEqual(SettingsTab.RESTART_ICON, " 🔁")

    # ---- Fields that REQUIRE restart ----------------------------------------

    def test_theme_mode_carries_restart_icon(self) -> None:
        """
        PD-07-AF-003 — Theme mode label carries 🔁 icon (agentx.theme_mode).

        GIVEN the SettingsTab is constructed
        WHEN the 'Theme mode' label is located
        THEN its text contains the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if "Theme mode" in t]
        self.assertTrue(matching, "No label found containing 'Theme mode'")
        self.assertTrue(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Theme mode' label {matching!r} is missing RESTART_ICON",
        )

    def test_ollama_host_carries_restart_icon(self) -> None:
        """
        PD-07-AF-003 — Ollama Host label carries 🔁 icon (agentx.ollama_host).

        GIVEN the SettingsTab is constructed
        WHEN the Host labels are located
        THEN at least one contains the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if "Host" in t]
        self.assertTrue(matching, "No label found containing 'Host'")
        self.assertTrue(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"No 'Host' label in {matching!r} contains RESTART_ICON",
        )

    def test_load_timeout_carries_restart_icon(self) -> None:
        """
        PD-07-AF-003 — Load timeout label carries 🔁 icon
        (agentx.ollama_initial_load_timeout_seconds).

        GIVEN the SettingsTab is constructed
        WHEN the 'Load timeout' label is located
        THEN its text contains the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if "Load timeout" in t]
        self.assertTrue(matching, "No label found containing 'Load timeout'")
        self.assertTrue(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Load timeout' label {matching!r} is missing RESTART_ICON",
        )

    def test_screen_side_carries_restart_icon(self) -> None:
        """
        PD-07-AF-003 — Screen side label carries 🔁 icon (agentx.screen_side).

        GIVEN the SettingsTab is constructed
        WHEN the 'Screen side' label is located
        THEN its text contains the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if "Screen side" in t]
        self.assertTrue(matching, "No label found containing 'Screen side'")
        self.assertTrue(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Screen side' label {matching!r} is missing RESTART_ICON",
        )

    def test_default_model_carries_restart_icon(self) -> None:
        """
        PD-07-AF-003 — Default model label carries 🔁 icon (agentx.ollama_model).

        GIVEN the SettingsTab is constructed
        WHEN the 'Default model' label is located
        THEN its text contains the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if "Default model" in t]
        self.assertTrue(matching, "No label found containing 'Default model'")
        self.assertTrue(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Default model' label {matching!r} is missing RESTART_ICON",
        )

    def test_working_memory_enabled_carries_restart_icon(self) -> None:
        """
        PD-07-AF-003 — Working Memory 'Enabled' label carries 🔁 icon
        (agentx.working_memory.enabled).

        GIVEN the SettingsTab is constructed
        WHEN the 'Enabled' Checkbutton text is located
        THEN its text contains the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if "Enabled" in t]
        self.assertTrue(matching, "No label found containing 'Enabled'")
        self.assertTrue(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Enabled' label {matching!r} is missing RESTART_ICON",
        )

    def test_torch_model_carries_restart_icon(self) -> None:
        """
        PD-07-AF-003 — Torch model label carries 🔁 icon
        (agentix.classification_torch_model).

        GIVEN the SettingsTab is constructed
        WHEN the 'Torch model' label is located
        THEN its text contains 🔁.
        """
        matching = [t for t in self._all_texts if "Torch model" in t]
        self.assertTrue(matching, "No label found containing 'Torch model'")
        self.assertTrue(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Torch model' label {matching!r} is missing RESTART_ICON",
        )

    def test_torch_device_carries_restart_icon(self) -> None:
        """
        PD-07-AF-003 — Torch device label carries 🔁 icon
        (agentix.classification_torch_device).

        GIVEN the SettingsTab is constructed
        WHEN the 'Torch device' label is located
        THEN its text contains 🔁.
        """
        matching = [t for t in self._all_texts if "Torch device" in t]
        self.assertTrue(matching, "No label found containing 'Torch device'")
        self.assertTrue(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Torch device' label {matching!r} is missing RESTART_ICON",
        )

    # ---- Fields that do NOT require restart ---------------------------------

    def test_classify_prompts_has_no_restart_icon(self) -> None:
        """
        PD-07-AF-003 — 'Classify prompts' does NOT carry the 🔁 icon.

        GIVEN the SettingsTab is constructed
        WHEN the 'Classify prompts' label is located
        THEN its text does NOT contain the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if "Classify prompts" in t]
        self.assertTrue(matching, "No label found containing 'Classify prompts'")
        self.assertFalse(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Classify prompts' label {matching!r} unexpectedly contains RESTART_ICON",
        )

    def test_debug_logging_has_no_restart_icon(self) -> None:
        """
        PD-07-AF-003 — 'Debug logging' does NOT carry the 🔁 icon.

        GIVEN the SettingsTab is constructed
        WHEN the 'Debug logging' label is located
        THEN its text does NOT contain the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if "Debug logging" in t]
        self.assertTrue(matching, "No label found containing 'Debug logging'")
        self.assertFalse(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Debug logging' label {matching!r} unexpectedly contains RESTART_ICON",
        )

    def test_backend_has_no_restart_icon(self) -> None:
        """
        PD-07-AF-003 — 'Backend' does NOT carry the 🔁 icon.

        GIVEN the SettingsTab is constructed
        WHEN the 'Backend' label is located
        THEN its text does NOT contain the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if t == "Backend"]
        self.assertTrue(matching, "No label with exact text 'Backend' found")
        self.assertFalse(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Backend' label {matching!r} unexpectedly contains RESTART_ICON",
        )

    def test_inject_into_llm_context_has_no_restart_icon(self) -> None:
        """
        PD-07-AF-003 — 'Inject into LLM context' does NOT carry the 🔁 icon.

        GIVEN the SettingsTab is constructed
        WHEN the 'Inject into LLM context' Checkbutton is located
        THEN its text does NOT contain the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if "Inject into LLM context" in t]
        self.assertTrue(matching, "No label found containing 'Inject into LLM context'")
        self.assertFalse(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Inject into LLM context' label {matching!r} unexpectedly contains RESTART_ICON",
        )

    def test_max_facts_has_no_restart_icon(self) -> None:
        """
        PD-07-AF-003 — 'Max facts' does NOT carry the 🔁 icon.

        GIVEN the SettingsTab is constructed
        WHEN the 'Max facts' label is located
        THEN its text does NOT contain the RESTART_ICON suffix.
        """
        matching = [t for t in self._all_texts if "Max facts" in t]
        self.assertTrue(matching, "No label found containing 'Max facts'")
        self.assertFalse(
            any(SettingsTab.RESTART_ICON in t for t in matching),
            f"'Max facts' label {matching!r} unexpectedly contains RESTART_ICON",
        )
