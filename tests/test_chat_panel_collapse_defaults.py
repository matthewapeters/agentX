"""Unit tests for ChatPanel entry collapse/expand defaults.

Unit under test:
  - ``agentx.gui.chat_panel.ChatPanel``

These tests lock down the three default-visibility affordances for chat-turn
child entries (PD-01-AF-005, PD-01-AF-006, PD-01-AF-007) as specified in
``docs/ux/03_PANEL_DETAILS.md §PD-01`` and tracked in
``docs/ux/UX_LIFECYCLE.md §4``.
"""

from __future__ import annotations

import tkinter as tk
from datetime import datetime
from unittest.mock import MagicMock

import pytest

from agentx.gui.gui_config import GUIConfig
from agentx.gui.gui_manager import GUIManager

# ---------------------------------------------------------------------------
# Fixture helpers
# ---------------------------------------------------------------------------


@pytest.fixture()
def gui() -> "GUIManager":
    """Yield a fully laid-out GUIManager attached to a hidden Tk root.

    The root is destroyed after each test to keep tests hermetic.
    """
    root = tk.Tk()
    root.withdraw()
    config = GUIConfig.from_dict(
        {
            "ollama_host": "localhost",
            "ollama_model": "test-model",
            "ollama_timeout": 30,
        }
    )
    instance = GUIManager(
        root=root,
        config=config,
        on_submit=MagicMock(),
        on_interrupt=MagicMock(),
        on_attachment_toggle=MagicMock(),
    )
    instance.create_layout()
    yield instance
    try:
        root.destroy()
    except Exception:
        pass


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _start_turn(gui: GUIManager) -> None:
    """Send a user message so that a conversation turn is initialised."""
    gui.display_user_message("Hello", attachments=[], timestamp=datetime.now())


def _is_detail_visible(entry: dict) -> bool:
    """Return True if the entry's detail_text widget is currently packed."""
    return entry["detail_text"].winfo_manager() == "pack"


# ---------------------------------------------------------------------------
# PD-01-AF-005 — Thinking block collapsed by default
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_thinking_entry_collapsed_by_default(gui: GUIManager) -> None:
    """Thinking block must be rendered collapsed when first created.

    GIVEN a conversation turn has started via display_user_message  [PD-01-AF-005]
    WHEN display_agent_thinking is called with a thinking chunk
    THEN the resulting thinking entry has expanded=False
    AND  the detail_text widget is NOT visible (not managed by pack).
    """
    _start_turn(gui)

    gui.display_agent_thinking("(The agent is thinking...)")
    gui.display_agent_thinking("Considering all edge-cases carefully.")

    entries = gui._current_turn_entries
    assert "thinking" in entries, "thinking entry must be created after display_agent_thinking"

    thinking_entry = entries["thinking"]
    assert thinking_entry["expanded"] is False, "PD-01-AF-005: thinking entry must start collapsed (expanded=False)"
    assert not _is_detail_visible(
        thinking_entry
    ), "PD-01-AF-005: detail_text of thinking entry must not be visible on creation"


# ---------------------------------------------------------------------------
# PD-01-AF-006 — Tool call collapsed by default
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_tool_call_entry_collapsed_by_default(gui: GUIManager) -> None:
    """Tool call block must be rendered collapsed when first created.

    GIVEN a conversation turn has started via display_user_message  [PD-01-AF-006]
    WHEN display_agent_response is called with a '[🔧 Calling tool' line
    THEN the resulting tool_call entry has expanded=False
    AND  the detail_text widget is NOT visible (not managed by pack).
    """
    _start_turn(gui)

    # The assistant header line is required to initialise the assistant entry
    # before the tool-call line arrives, matching the real streaming order.
    gui.display_agent_response(f"\U0001f916 AgentX (12:00):\n")
    gui.display_agent_response('[🔧 Calling tool: read_file({"path": "/tmp/x"})]')

    entries = gui._current_turn_entries
    assert "tool_call" in entries, "tool_call entry must be created after a '[🔧 Calling tool' line"

    tool_call_entry = entries["tool_call"]
    assert tool_call_entry["expanded"] is False, "PD-01-AF-006: tool_call entry must start collapsed (expanded=False)"
    assert not _is_detail_visible(
        tool_call_entry
    ), "PD-01-AF-006: detail_text of tool_call entry must not be visible on creation"


# ---------------------------------------------------------------------------
# PD-01-AF-007 — Assistant response expanded by default
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_assistant_response_entry_expanded_by_default(gui: GUIManager) -> None:
    """Assistant response block must be rendered expanded when first created.

    GIVEN a conversation turn has started via display_user_message  [PD-01-AF-007]
    WHEN display_agent_response is called with an assistant header line
    AND  subsequent content chunks are streamed
    THEN the resulting assistant entry has expanded=True
    AND  the detail_text widget IS visible (managed by pack).
    """
    _start_turn(gui)

    gui.display_agent_response(f"\U0001f916 AgentX (12:00):\n")
    gui.display_agent_response("Here is my answer to your question.")

    entries = gui._current_turn_entries
    assert "assistant" in entries, "assistant entry must be created after display_agent_response"

    assistant_entry = entries["assistant"]
    assert assistant_entry["expanded"] is True, "PD-01-AF-007: assistant entry must start expanded (expanded=True)"
    assert _is_detail_visible(
        assistant_entry
    ), "PD-01-AF-007: detail_text of assistant entry must be visible on creation"
