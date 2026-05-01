"""Unit tests for ResynthesisDialog affordances.

Unit under test:
  - ``agentx.gui.resynthesis_dialog.ResynthesisDialog``

These tests lock down all behaviours described in the PD-06 component cut-sheet
in ``docs/ux/03_PANEL_DETAILS.md §PD-06`` and tracked in
``docs/ux/UX_LIFECYCLE.md §4``.

All tests are hermetic: no live services, no file system I/O.  A hidden
``tk.Tk`` root is created per-test and destroyed in the fixture teardown.
The dialog is never made visible; ``_win.withdraw()`` is called immediately
after construction to keep the test process headless.
"""

from __future__ import annotations

import tkinter as tk
from unittest.mock import MagicMock, call

import pytest

from agentx.gui.resynthesis_dialog import ResynthesisDialog

# ---------------------------------------------------------------------------
# Fixture
# ---------------------------------------------------------------------------


@pytest.fixture()
def root() -> "tk.Tk":
    """Yield a hidden Tk root; destroy after each test."""
    instance = tk.Tk()
    instance.withdraw()
    yield instance
    try:
        instance.destroy()
    except Exception:
        pass


# ---------------------------------------------------------------------------
# Helper: build a dialog and immediately withdraw it to stay headless
# ---------------------------------------------------------------------------


def _make_dialog(
    root: tk.Tk,
    task_id: str = "step-42",
    synthesis_text: str = "Synthesis result.",
    failed_assertions: list | None = None,
    on_confirm: MagicMock | None = None,
    on_add_wm_hint: MagicMock | None = None,
) -> ResynthesisDialog:
    """Construct a ResynthesisDialog, withdraw it, and return it."""
    dialog = ResynthesisDialog(
        parent=root,
        task_id=task_id,
        synthesis_text=synthesis_text,
        failed_assertions=failed_assertions or [],
        on_confirm=on_confirm or MagicMock(),
        on_add_wm_hint=on_add_wm_hint,
    )
    dialog._win.withdraw()
    return dialog


# ---------------------------------------------------------------------------
# PD-06-AF-001 — Title includes task_id
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_title_includes_task_id(root: tk.Tk) -> None:
    """Dialog title must include the task_id string.

    GIVEN ResynthesisDialog is constructed with task_id="step-42"  [PD-06-AF-001]
    WHEN the dialog window is created
    THEN the window title is "Re-synthesise — step-42".
    """
    dialog = _make_dialog(root, task_id="step-42")
    assert dialog._win.title() == "Re-synthesise — step-42"


# ---------------------------------------------------------------------------
# PD-06-AF-002 — Cancel closes dialog without calling on_confirm
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_cancel_destroys_dialog_without_confirm(root: tk.Tk) -> None:
    """Cancel button must destroy the window and not call on_confirm.

    GIVEN a ResynthesisDialog with a mock on_confirm callback  [PD-06-AF-002]
    WHEN the Cancel button is invoked
    THEN on_confirm is not called
    AND the dialog window is no longer alive (winfo_exists() == 0).
    """
    on_confirm = MagicMock()
    dialog = _make_dialog(root, on_confirm=on_confirm)
    win = dialog._win

    # Find the Cancel button by its label text
    cancel_btn = _find_button(win, "Cancel")
    assert cancel_btn is not None, "Cancel button must exist"
    cancel_btn.invoke()

    on_confirm.assert_not_called()
    assert win.winfo_exists() == 0, "Dialog window must be destroyed after Cancel"


# ---------------------------------------------------------------------------
# PD-06-AF-003 — Re-synthesise calls on_confirm with hint text
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_confirm_calls_on_confirm_with_hint(root: tk.Tk) -> None:
    """Re-synthesise button must pass the hint text to on_confirm.

    GIVEN a ResynthesisDialog with a mock on_confirm callback  [PD-06-AF-003]
    AND the hint field contains "focus on error handling"
    WHEN the Re-synthesise button is invoked
    THEN on_confirm is called once with "focus on error handling"
    AND the dialog window is destroyed.
    """
    on_confirm = MagicMock()
    dialog = _make_dialog(root, on_confirm=on_confirm)
    win = dialog._win

    dialog._hint_text.insert("1.0", "focus on error handling")
    confirm_btn = _find_button(win, "Re-synthesise")
    assert confirm_btn is not None, "Re-synthesise button must exist"
    confirm_btn.invoke()

    on_confirm.assert_called_once_with("focus on error handling")
    assert win.winfo_exists() == 0, "Dialog window must be destroyed after confirm"


@pytest.mark.unit
def test_confirm_with_empty_hint_passes_empty_string(root: tk.Tk) -> None:
    """Re-synthesise with blank hint must call on_confirm with empty string.

    GIVEN a ResynthesisDialog with a mock on_confirm callback  [PD-06-AF-003]
    AND the hint field is empty
    WHEN the Re-synthesise button is invoked
    THEN on_confirm is called once with "".
    """
    on_confirm = MagicMock()
    dialog = _make_dialog(root, on_confirm=on_confirm)

    # Leave hint field empty
    confirm_btn = _find_button(dialog._win, "Re-synthesise")
    assert confirm_btn is not None
    confirm_btn.invoke()

    on_confirm.assert_called_once_with("")


# ---------------------------------------------------------------------------
# PD-06-AF-004 — WM hint section visibility
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_wm_section_hidden_without_callback(root: tk.Tk) -> None:
    """WM hint section must be absent when on_add_wm_hint is not provided.

    GIVEN ResynthesisDialog constructed without on_add_wm_hint  [PD-06-AF-004]
    WHEN the dialog is displayed
    THEN no widget with text "Add WM hint" exists in the dialog.
    """
    dialog = _make_dialog(root, on_add_wm_hint=None)
    btn = _find_button(dialog._win, "Add WM hint")
    assert btn is None, "Add WM hint button must NOT be present when callback is None"


@pytest.mark.unit
def test_wm_section_visible_with_callback(root: tk.Tk) -> None:
    """WM hint section must be present when on_add_wm_hint is provided.

    GIVEN ResynthesisDialog constructed with a mock on_add_wm_hint callback  [PD-06-AF-004]
    WHEN the dialog is displayed
    THEN a button labelled "Add WM hint" exists in the dialog.
    """
    dialog = _make_dialog(root, on_add_wm_hint=MagicMock())
    btn = _find_button(dialog._win, "Add WM hint")
    assert btn is not None, "Add WM hint button must be present when callback is provided"


# ---------------------------------------------------------------------------
# PD-06-AF-005 — Add WM hint calls callback and clears fields
# ---------------------------------------------------------------------------


@pytest.mark.unit
def test_add_wm_hint_calls_callback_and_clears_fields(root: tk.Tk) -> None:
    """Add WM hint must invoke callback with key+value and clear both fields.

    GIVEN ResynthesisDialog with on_add_wm_hint provided  [PD-06-AF-005]
    AND key field contains "style" and value field contains "concise"
    WHEN the Add WM hint button is invoked
    THEN on_add_wm_hint is called with ("style", "concise")
    AND both key and value fields are cleared
    AND the dialog remains open (on_confirm not called).
    """
    on_confirm = MagicMock()
    on_add_wm_hint = MagicMock()
    dialog = _make_dialog(root, on_confirm=on_confirm, on_add_wm_hint=on_add_wm_hint)

    dialog._wm_key_var.set("style")
    dialog._wm_val_var.set("concise")

    add_btn = _find_button(dialog._win, "Add WM hint")
    assert add_btn is not None
    add_btn.invoke()

    on_add_wm_hint.assert_called_once_with("style", "concise")
    assert dialog._wm_key_var.get() == "", "Key field must be cleared after Add WM hint"
    assert dialog._wm_val_var.get() == "", "Value field must be cleared after Add WM hint"
    on_confirm.assert_not_called()
    assert dialog._win.winfo_exists() == 1, "Dialog must remain open after Add WM hint"


# ---------------------------------------------------------------------------
# Private helper: recursive button finder
# ---------------------------------------------------------------------------


def _find_button(widget: tk.Widget, label: str) -> "tk.Button | None":
    """Recursively search *widget* tree for a tk.Button with the given label.

    Returns the first match or ``None``.
    """
    if isinstance(widget, tk.Button) and widget.cget("text") == label:
        return widget
    for child in widget.winfo_children():
        result = _find_button(child, label)
        if result is not None:
            return result
    return None
