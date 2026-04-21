"""Integration tests for ChatPanel conversation-turn rendering order.

Units under test:
  - ``agentx.gui.chat_panel.ChatPanel``
  - ``agentx.gui.gui_manager.GUIManager`` (as the host of ChatPanel)

These tests verify that within a single conversation turn the visual stacking
order of widgets is correct: the **user message** entry must appear *above*
(i.e. packed before) the **children frame** that contains classification,
thinking, tool-call, and agent-response child entries.  Without the fix,
Tkinter's ``pack`` manager renders the children frame before the user entry
because it was packed earlier, making all response widgets appear *above* the
prompt that triggered them.

Gherkin use-cases are embedded in each test's docstring.
"""

from __future__ import annotations

import tkinter as tk
import unittest
from datetime import datetime
from typing import Any
from unittest.mock import MagicMock

import pytest

from agentx.gui.gui_config import GUIConfig
from agentx.gui.gui_manager import GUIManager

# ---------------------------------------------------------------------------
# Shared fixture helpers
# ---------------------------------------------------------------------------


def _make_gui(root: tk.Tk) -> GUIManager:
    """Build a fully laid-out GUIManager attached to *root*."""
    config = GUIConfig.from_dict(
        {
            "ollama_host": "localhost",
            "ollama_model": "test-model",
            "ollama_timeout": 30,
        }
    )
    gui = GUIManager(
        root=root,
        config=config,
        on_submit=MagicMock(),
        on_interrupt=MagicMock(),
        on_attachment_toggle=MagicMock(),
    )
    gui.create_layout()
    return gui


def _classification_payload(**overrides: Any) -> dict:
    """Return a minimal classification dict, optionally overriding fields."""
    base = {
        "intent": "simple_action",
        "next_step": "respond_directly",
        "reasoning_summary": "Direct response needed.",
        "needs_clarification": False,
        "missing_fields": [],
    }
    base.update(overrides)
    return base


# ---------------------------------------------------------------------------
# Helper: pack order assertion
# ---------------------------------------------------------------------------


def _pack_index(widget: tk.Widget) -> int:
    """Return the 0-based index of *widget* among its parent's pack slaves."""
    slaves = widget.master.pack_slaves()
    return slaves.index(widget)


# ---------------------------------------------------------------------------
# Test class
# ---------------------------------------------------------------------------


@pytest.mark.integration
class TestConversationTurnRenderingOrder(unittest.TestCase):
    """ChatPanel must stack widgets so the user entry is above child entries.

    Integration units tested:
      - ``ChatPanel._ensure_turn_started``
      - ``ChatPanel._ensure_child_entry``
      - ``ChatPanel.display_user_message``
      - ``ChatPanel.display_classification``
      - ``ChatPanel.display_agent_thinking``
      - ``ChatPanel.display_agent_response``
    """

    def setUp(self) -> None:
        """Create a hidden Tk root and a fully-laid-out GUIManager."""
        self.root = tk.Tk()
        self.root.withdraw()
        self.gui = _make_gui(self.root)

    def tearDown(self) -> None:
        try:
            self.root.destroy()
        except Exception:
            pass

    # ------------------------------------------------------------------
    # Core ordering test
    # ------------------------------------------------------------------

    def test_user_entry_packed_before_children_frame_on_first_render(self) -> None:
        """User entry frame must precede the children container in pack order.

        GIVEN a fresh conversation turn is started via display_user_message
        AND a classification chunk is received immediately after
        WHEN the chat panel processes both display calls
        THEN the user entry frame's pack index must be lower than the
             children frame's pack index (user appears above children).
        """
        self.gui.display_user_message("Hello agent", attachments=[], timestamp=datetime.now())
        self.gui.display_classification(_classification_payload())

        turn_frame = self.gui._chat_panel._current_turn_frame
        children_frame = self.gui._current_turn_children_frame
        user_frame = self.gui._current_turn_entries["user"]["frame"]

        self.assertIsNotNone(turn_frame, "turn_frame must exist after display_user_message")
        self.assertIsNotNone(children_frame, "children_frame must exist after display_user_message")

        slaves = turn_frame.pack_slaves()
        self.assertIn(user_frame, slaves, "User entry frame must be a pack slave of turn_frame")
        self.assertIn(children_frame, slaves, "Children frame must be a pack slave of turn_frame")

        user_idx = slaves.index(user_frame)
        children_idx = slaves.index(children_frame)
        self.assertLess(
            user_idx,
            children_idx,
            f"User entry (index {user_idx}) must be packed BEFORE children frame "
            f"(index {children_idx}), but children frame appears first.  "
            f"Full pack-slave order: {[str(w) for w in slaves]}",
        )

    def test_thinking_entry_placed_inside_children_frame(self) -> None:
        """Thinking child entry must live inside the children frame, not the turn frame.

        GIVEN a conversation turn is started
        AND the agent emits a thinking block
        WHEN the chat panel processes the thinking chunk
        THEN the thinking entry frame's parent must be the children frame (not the turn frame).
        """
        self.gui.display_user_message("Analyse this", attachments=[], timestamp=datetime.now())
        self.gui.display_agent_thinking("(The agent is thinking...)")
        self.gui.display_agent_thinking("Considering all possibilities…")

        children_frame = self.gui._current_turn_children_frame
        thinking_entry = self.gui._current_turn_entries.get("thinking")
        self.assertIsNotNone(thinking_entry, "Thinking entry must exist after display_agent_thinking")

        thinking_frame = thinking_entry["frame"]
        self.assertEqual(
            str(thinking_frame.master),
            str(children_frame),
            "Thinking entry frame must be a child of the children frame, not the turn frame",
        )

    def test_classification_entry_placed_inside_children_frame(self) -> None:
        """Classification entry must be placed inside the children frame.

        GIVEN a conversation turn is started
        AND a classification payload is received
        WHEN the chat panel renders the classification
        THEN the classification entry frame's parent must be the children frame.
        """
        self.gui.display_user_message("What is the weather?", attachments=[], timestamp=datetime.now())
        self.gui.display_classification(_classification_payload(intent="weather_query"))

        children_frame = self.gui._current_turn_children_frame
        classification_entry = self.gui._current_turn_entries.get("classification")
        self.assertIsNotNone(classification_entry, "Classification entry must exist")

        clf_frame = classification_entry["frame"]
        self.assertEqual(
            str(clf_frame.master),
            str(children_frame),
            "Classification entry frame must be a child of the children frame",
        )

    def test_agent_response_entry_placed_inside_children_frame(self) -> None:
        """Agent response entry must be placed inside the children frame.

        GIVEN a conversation turn is started
        AND the agent streams a response
        WHEN the chat panel processes the response chunk
        THEN the assistant entry frame's parent must be the children frame.
        """
        self.gui.display_user_message("Tell me a joke", attachments=[], timestamp=datetime.now())
        self.gui.display_agent_response("Why did the chicken cross the road?")

        children_frame = self.gui._current_turn_children_frame
        assistant_entry = self.gui._current_turn_entries.get("assistant")
        self.assertIsNotNone(assistant_entry, "Assistant entry must exist after display_agent_response")

        asst_frame = assistant_entry["frame"]
        self.assertEqual(
            str(asst_frame.master),
            str(children_frame),
            "Assistant entry frame must be a child of the children frame",
        )

    # ------------------------------------------------------------------
    # Full turn sequence
    # ------------------------------------------------------------------

    def test_full_turn_sequence_pack_order(self) -> None:
        """A full turn (user → classify → think → respond) must render in correct order.

        GIVEN the user submits a prompt
        AND the bridge emits classification, thinking, and content chunks
        WHEN all display calls are processed in sequence
        THEN:
          1. turn_frame pack-slaves: [user_entry_frame, children_frame]
          2. children_frame pack-slaves contain classification, thinking, assistant
             entries in that order.
        """
        self.gui.display_user_message("Write me a poem", attachments=[], timestamp=datetime.now())
        self.gui.display_classification(_classification_payload(next_step="respond_directly"))
        self.gui.display_agent_thinking("(The agent is thinking...)")
        self.gui.display_agent_thinking("Thinking about poetry…")
        self.gui.display_agent_response("Roses are red,\nViolets are blue.")

        turn_frame = self.gui._chat_panel._current_turn_frame
        children_frame = self.gui._current_turn_children_frame
        user_frame = self.gui._current_turn_entries["user"]["frame"]

        # --- turn_frame order ---
        turn_slaves = turn_frame.pack_slaves()
        user_idx = turn_slaves.index(user_frame)
        children_idx = turn_slaves.index(children_frame)
        self.assertLess(user_idx, children_idx, "User entry must precede children frame")

        # --- children_frame order ---
        clf_entry = self.gui._current_turn_entries.get("classification")
        thinking_entry = self.gui._current_turn_entries.get("thinking")
        assistant_entry = self.gui._current_turn_entries.get("assistant")

        self.assertIsNotNone(clf_entry)
        self.assertIsNotNone(thinking_entry)
        self.assertIsNotNone(assistant_entry)

        child_slaves = children_frame.pack_slaves()
        clf_idx = child_slaves.index(clf_entry["frame"])
        thinking_idx = child_slaves.index(thinking_entry["frame"])
        asst_idx = child_slaves.index(assistant_entry["frame"])

        self.assertLess(clf_idx, thinking_idx, "Classification must precede thinking")
        self.assertLess(thinking_idx, asst_idx, "Thinking must precede agent response")

    def test_full_turn_sequence_with_tool_call(self) -> None:
        """A turn with a tool call must place the tool entry inside the children frame.

        GIVEN the user submits a prompt requiring a tool
        AND the bridge emits a TOOL_CALL chunk
        WHEN display_agent_response processes the [🔧 Calling tool: ...] line
        THEN the tool entry must be inside the children frame, not the turn frame.
        """
        self.gui.display_user_message("List files in /tmp", attachments=[], timestamp=datetime.now())
        self.gui.display_classification(_classification_payload(next_step="single_tool", intent="file_listing"))
        self.gui.display_agent_response("[🔧 Calling tool: list_directory args={'path': '/tmp'}]")

        children_frame = self.gui._current_turn_children_frame
        tool_entry = self.gui._current_turn_entries.get("tool_call")
        self.assertIsNotNone(tool_entry, "Tool call entry must be created")

        tool_frame = tool_entry["frame"]
        self.assertEqual(
            str(tool_frame.master),
            str(children_frame),
            "Tool call entry frame must be a child of the children frame",
        )

    # ------------------------------------------------------------------
    # Collapse / expand does not corrupt pack order
    # ------------------------------------------------------------------

    def test_expand_after_collapse_preserves_correct_order(self) -> None:
        """Expanding the user entry after collapse must not move children above user.

        GIVEN a full conversation turn has been rendered
        AND the user entry is collapsed (hiding the children frame)
        WHEN the user entry is expanded again
        THEN the user entry frame must still precede the children frame in pack order.
        """
        self.gui.display_user_message("Multi-step task", attachments=[], timestamp=datetime.now())
        self.gui.display_classification(_classification_payload())
        self.gui.display_agent_response("Working on it…")

        turn_frame = self.gui._chat_panel._current_turn_frame
        children_frame = self.gui._current_turn_children_frame
        user_entry = self.gui._current_turn_entries["user"]
        user_frame = user_entry["frame"]

        # Collapse and then expand
        user_entry["toggle"]()  # collapse → children_frame hidden
        self.assertEqual(children_frame.winfo_manager(), "", "Children frame must be hidden after collapse")
        user_entry["toggle"]()  # expand → children_frame visible again
        self.assertEqual(children_frame.winfo_manager(), "pack", "Children frame must be visible after expand")

        # After re-pack the order must still be correct
        slaves = turn_frame.pack_slaves()
        user_idx = slaves.index(user_frame)
        children_idx = slaves.index(children_frame)
        self.assertLess(
            user_idx,
            children_idx,
            "After expand/collapse cycle user entry must still precede children frame in pack order",
        )


# ---------------------------------------------------------------------------
# Parametrized multi-turn test (plain pytest class — not unittest.TestCase)
# ---------------------------------------------------------------------------


@pytest.mark.integration
class TestMultipleTurnsRenderingOrder:
    """Parametrized verification that consecutive turns each have correct pack order.

    Units tested:
      - ``ChatPanel._ensure_turn_started``
      - ``ChatPanel.display_spacing`` (turn reset)
      - ``GUIManager.display_user_message`` / ``display_agent_response``
    """

    @pytest.mark.parametrize(
        "turns",
        [
            pytest.param(
                [("First question", "First answer")],
                id="single_turn",
            ),
            pytest.param(
                [
                    ("First question", "First answer"),
                    ("Second question", "Second answer"),
                ],
                id="two_turns",
            ),
            pytest.param(
                [
                    ("Question A", "Answer A"),
                    ("Question B", "Answer B"),
                    ("Question C", "Answer C"),
                ],
                id="three_turns",
            ),
        ],
    )
    def test_multiple_turns_each_have_correct_order(self, turns: list[tuple[str, str]]) -> None:
        """Each conversation turn must independently maintain correct pack order.

        GIVEN multiple conversation turns are rendered sequentially
        WHEN each turn completes (user message, classification, agent response)
        THEN for the final turn the user entry frame must precede the children
             frame in the Tkinter pack order.

        Parameterized permutations:
          - single_turn: one turn only — baseline correctness
          - two_turns: two consecutive turns — checks that turn state resets cleanly
          - three_turns: three consecutive turns — detects state bleed across turns
        """
        root = tk.Tk()
        root.withdraw()
        try:
            gui = _make_gui(root)
            for prompt, response in turns:
                gui.display_user_message(prompt, attachments=[], timestamp=datetime.now())
                gui.display_classification(_classification_payload())
                gui.display_agent_response(response)
                gui.display_spacing()

            # Re-render the last turn so we can inspect it (display_spacing clears state)
            last_prompt, last_response = turns[-1]
            gui.display_user_message(last_prompt, attachments=[], timestamp=datetime.now())
            gui.display_classification(_classification_payload())
            gui.display_agent_response(last_response)

            turn_frame = gui._chat_panel._current_turn_frame
            children_frame = gui._current_turn_children_frame
            user_frame = gui._current_turn_entries["user"]["frame"]

            slaves = turn_frame.pack_slaves()
            user_idx = slaves.index(user_frame)
            children_idx = slaves.index(children_frame)
            assert user_idx < children_idx, (
                f"User entry (index {user_idx}) must be packed BEFORE children frame "
                f"(index {children_idx}). Full order: {[str(w) for w in slaves]}"
            )
        finally:
            try:
                root.destroy()
            except Exception:
                pass
