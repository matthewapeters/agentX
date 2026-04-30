"""Unit tests for CollapsibleSection affordances.

These tests lock down all behaviors described in the PD-09 component cut-sheet.
"""

from __future__ import annotations

import tkinter as tk

import pytest

from agentx.gui.collapsible_section import CollapsibleSection


@pytest.fixture
def root() -> tk.Tk:
    """Create a hidden Tk root for widget tests.

    Returns:
        tk.Tk: Hidden Tk root.
    """
    instance = tk.Tk()
    instance.withdraw()
    yield instance
    try:
        instance.destroy()
    except Exception:
        pass


@pytest.mark.unit
def test_initial_collapsed_state_hides_content_container(root: tk.Tk) -> None:
    """GIVEN a CollapsibleSection with initial_collapsed=True [PD-09-AF-001]
    WHEN it is created
    THEN the section is collapsed and content_container is not managed by pack.
    """
    section = CollapsibleSection(parent=root, title="WM", initial_collapsed=True)

    assert section.is_expanded() is False
    assert section.content_container.winfo_manager() == ""


@pytest.mark.unit
def test_initial_expanded_state_shows_content_container(root: tk.Tk) -> None:
    """GIVEN a CollapsibleSection with initial_collapsed=False [PD-09-AF-002]
    WHEN it is created
    THEN the section is expanded and content_container is managed by pack.
    """
    section = CollapsibleSection(parent=root, title="Context", initial_collapsed=False)

    assert section.is_expanded() is True
    assert section.content_container.winfo_manager() == "pack"


@pytest.mark.unit
def test_toggle_flips_state_and_visibility(root: tk.Tk) -> None:
    """GIVEN a CollapsibleSection [PD-09-AF-003]
    WHEN toggle() is called
    THEN expanded state flips and content_container visibility changes accordingly.
    """
    section = CollapsibleSection(parent=root, title="Section", initial_collapsed=True)

    assert section.is_expanded() is False
    assert section.content_container.winfo_manager() == ""

    section.toggle()
    assert section.is_expanded() is True
    assert section.content_container.winfo_manager() == "pack"

    section.toggle()
    assert section.is_expanded() is False
    assert section.content_container.winfo_manager() == ""


@pytest.mark.unit
def test_set_content_replaces_previous_widget(root: tk.Tk) -> None:
    """GIVEN a CollapsibleSection with existing content [PD-09-AF-004]
    WHEN set_content() is called with a new widget
    THEN the previous widget is replaced and only the new widget remains.
    """
    section = CollapsibleSection(parent=root, title="Section", initial_collapsed=False)

    first = tk.Label(section.content_container, text="first")
    second = tk.Label(section.content_container, text="second")

    section.set_content(first)
    assert section._content_widget is first
    assert first.winfo_exists() == 1

    section.set_content(second)

    assert section._content_widget is second
    assert second.winfo_exists() == 1
    assert first.winfo_exists() == 0
