"""Unit tests for terminal permission-list editor controls in SettingsTab.

Units under test:
- ``SettingsTab._save_terminal_permission_lists`` [PD-15-AF-007]
- ``SettingsTab._reset_terminal_permission_lists`` [PD-15-AF-007]
"""

from __future__ import annotations

import tkinter as tk
from unittest.mock import MagicMock

import pytest

from agentx.gui.settings_tab import SettingsTab
from agentx.integration.terminal_bridge import (
    DEFAULT_ALLOW_PREFIXES,
    DEFAULT_CONFIRM_PREFIXES,
    DEFAULT_DENY_PREFIXES,
)


@pytest.fixture
def settings_tab_root() -> tk.Tk:
    """Create hidden Tk root for SettingsTab tests."""
    root = tk.Tk()
    root.withdraw()
    yield root
    root.destroy()


def _build_tab(root: tk.Tk, on_change: MagicMock) -> SettingsTab:
    """Create SettingsTab with terminal config enabled."""
    config = {
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
        "terminal": {
            "exec_mode": "supervised",
            "allow": ["ls"],
            "confirm": ["git commit"],
            "deny": ["rm "],
            "terminal_visible": True,
            "terminal_auto_close": True,
            "terminal_timeout_sec": 60,
        },
    }

    return SettingsTab(
        parent=root,
        config=config,
        bg="#222222",
        fg="#eeeeee",
        on_change=on_change,
    )


@pytest.mark.unit
def test_save_terminal_permission_lists_emits_allow_confirm_deny_updates(settings_tab_root: tk.Tk) -> None:
    """GIVEN edited allow/confirm/deny text widgets [PD-15-AF-007]

    WHEN save lists action is triggered
    THEN on_change is fired with normalized list values for all three keys.
    """
    on_change = MagicMock()
    tab = _build_tab(settings_tab_root, on_change)

    assert tab._terminal_allow_text is not None
    assert tab._terminal_confirm_text is not None
    assert tab._terminal_deny_text is not None

    tab._terminal_allow_text.delete("1.0", tk.END)
    tab._terminal_allow_text.insert("1.0", "pwd\nls\n")
    tab._terminal_confirm_text.delete("1.0", tk.END)
    tab._terminal_confirm_text.insert("1.0", "git commit\nuv add\n")
    tab._terminal_deny_text.delete("1.0", tk.END)
    tab._terminal_deny_text.insert("1.0", "rm \nsudo \n")

    tab._save_terminal_permission_lists()

    on_change.assert_any_call(["terminal", "allow"], ["pwd", "ls"])
    on_change.assert_any_call(["terminal", "confirm"], ["git commit", "uv add"])
    on_change.assert_any_call(["terminal", "deny"], ["rm ", "sudo "])


@pytest.mark.unit
def test_reset_terminal_permission_lists_restores_factory_defaults(settings_tab_root: tk.Tk) -> None:
    """GIVEN modified permission list editors [PD-15-AF-007]

    WHEN reset defaults is triggered
    THEN text editors are reset to terminal bridge defaults
     AND save callback emits default lists.
    """
    on_change = MagicMock()
    tab = _build_tab(settings_tab_root, on_change)

    tab._reset_terminal_permission_lists()

    on_change.assert_any_call(["terminal", "allow"], DEFAULT_ALLOW_PREFIXES)
    on_change.assert_any_call(["terminal", "confirm"], DEFAULT_CONFIRM_PREFIXES)
    on_change.assert_any_call(["terminal", "deny"], DEFAULT_DENY_PREFIXES)
