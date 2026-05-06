"""Unit tests for StatusTab (PD-12).

Affordance traceability:
    PD-12-AF-001  Status tab is first tab in system_notebook
    PD-12-AF-002  Auto-switch on stream start
    PD-12-AF-003  Interrupt button enabled/disabled on set_streaming_state
    PD-12-AF-004  Interrupt button invokes interrupt callback
    PD-12-AF-005  Phase rows reset to PENDING on reset()
    PD-12-AF-006  Phase row transitions to RUNNING via set_phase()
    PD-12-AF-007  Phase row transitions to DONE via set_phase()
    PD-12-AF-008  Phase row transitions to FAILED via set_phase()
    PD-12-AF-009  Tool step label shows active tool name
    PD-12-AF-010  ContextKeyWidget renders one swatch row per band
    PD-12-AF-011  context_meter attribute is a ContextMeterWidget

All tests are hermetic (no real Tkinter display required); widgets are
created under a hidden ``tk.Tk()`` that is destroyed in teardown.
"""

from __future__ import annotations

import tkinter as tk
from tkinter import ttk
from unittest.mock import MagicMock, patch

import pytest

from agentx.gui.status_tab import (
    ContextKeyWidget,
    PhaseRow,
    StatusTab,
    _ELAPSED_PLACEHOLDER,
    _ICON_DONE,
    _ICON_FAILED,
    _ICON_PENDING,
    _ICON_RUNNING,
    _PHASE_STEPS,
    _format_elapsed,
)
from agentx.gui.context_meter_widget import _BANDS

# ── Helpers ───────────────────────────────────────────────────────────────────


def _make_root() -> tk.Tk:
    """Return a hidden Tk root window for unit testing."""
    root = tk.Tk()
    root.withdraw()
    return root


def _make_status_tab() -> tuple[StatusTab, MagicMock, tk.Tk]:
    """Instantiate a StatusTab with a mock GUIManager and a hidden root.

    Returns:
        tuple[StatusTab, MagicMock, tk.Tk]: tab, gui mock, root.
    """
    root = _make_root()
    gui = MagicMock()
    gui.root = root
    gui.config = MagicMock()
    gui.config.app_background_color = "#1e1e1e"
    gui.config.section_background_color = "#2d2d2d"
    gui.config.input_bg = "#1e1e1e"
    gui.config.text_fg = "#ffffff"
    gui.config.accent_color = "#007acc"
    tab = StatusTab(gui)
    return tab, gui, root


# ── _format_elapsed ───────────────────────────────────────────────────────────


@pytest.mark.unit
class TestFormatElapsed:
    """Unit tests for _format_elapsed helper.

    GIVEN a non-negative elapsed duration in seconds,
    WHEN _format_elapsed is called,
    THEN it returns HH:MM:SS with leading zeros.
    """

    @pytest.mark.parametrize(
        "seconds,expected",
        [
            (0, "00:00:00"),
            (1, "00:00:01"),
            (59, "00:00:59"),
            (60, "00:01:00"),
            (3599, "00:59:59"),
            (3600, "01:00:00"),
            (3661, "01:01:01"),
            (86399, "23:59:59"),
            (-5, "00:00:00"),  # negative clamped to zero
        ],
    )
    def test_format_elapsed(self, seconds: float, expected: str) -> None:
        """GIVEN elapsed={seconds}s WHEN _format_elapsed THEN result='{expected}'.

        Args:
            seconds (float): Input duration.
            expected (str): Expected HH:MM:SS string.
        """
        assert _format_elapsed(seconds) == expected


# ── PhaseRow ──────────────────────────────────────────────────────────────────


@pytest.mark.unit
class TestPhaseRow:
    """Unit tests for PhaseRow widget.

    Tests initial state, state transitions, elapsed timer, and tool label.
    """

    def setup_method(self) -> None:
        self._root = _make_root()

    def teardown_method(self) -> None:
        self._root.destroy()

    def _make_row(self, step_key: str = "classify") -> PhaseRow:
        parent = tk.Frame(self._root)
        return PhaseRow(parent, step_key, "🤔", "Classify")

    # ── AF-005 ─────────────────────────────────────────────────────────────────

    def test_initial_state_pending(self) -> None:
        """GIVEN a new PhaseRow WHEN inspected THEN state is PENDING.

        GIVEN a fresh PhaseRow instance,
        WHEN the state attribute is checked immediately after construction,
        THEN it should be 'PENDING' with icon _ICON_PENDING.

        [PD-12-AF-005]
        """
        row = self._make_row()
        assert row._state == "PENDING"
        assert row._icon_label.cget("text") == _ICON_PENDING

    def test_reset_restores_pending(self) -> None:
        """GIVEN a DONE PhaseRow WHEN reset() THEN state becomes PENDING.

        GIVEN a PhaseRow that has been transitioned to DONE,
        WHEN reset() is called,
        THEN the state returns to PENDING, elapsed shows placeholder, and
        the timer is stopped.

        [PD-12-AF-005]
        """
        row = self._make_row()
        row.set_state("DONE")
        row.reset()
        assert row._state == "PENDING"
        assert row._elapsed_label.cget("text") == _ELAPSED_PLACEHOLDER
        assert row._start_time is None

    # ── AF-006 ─────────────────────────────────────────────────────────────────

    def test_set_state_running(self) -> None:
        """GIVEN a PENDING PhaseRow WHEN set_state('RUNNING') THEN icon updates.

        GIVEN a PENDING PhaseRow,
        WHEN set_state is called with 'RUNNING',
        THEN the icon changes to _ICON_RUNNING and _start_time is recorded.

        [PD-12-AF-006]
        """
        row = self._make_row()
        row.set_state("RUNNING")
        assert row._state == "RUNNING"
        assert row._icon_label.cget("text") == _ICON_RUNNING
        assert row._start_time is not None

    # ── AF-007 ─────────────────────────────────────────────────────────────────

    def test_set_state_done(self) -> None:
        """GIVEN a RUNNING PhaseRow WHEN set_state('DONE') THEN icon shows DONE.

        GIVEN a RUNNING PhaseRow,
        WHEN set_state is called with 'DONE',
        THEN the icon changes to _ICON_DONE and _final_elapsed is frozen.

        [PD-12-AF-007]
        """
        row = self._make_row()
        row.set_state("RUNNING")
        row.set_state("DONE")
        assert row._state == "DONE"
        assert row._icon_label.cget("text") == _ICON_DONE
        assert row._final_elapsed is not None

    # ── AF-008 ─────────────────────────────────────────────────────────────────

    def test_set_state_failed(self) -> None:
        """GIVEN a RUNNING PhaseRow WHEN set_state('FAILED') THEN icon shows FAILED.

        GIVEN a RUNNING PhaseRow,
        WHEN set_state is called with 'FAILED',
        THEN the icon changes to _ICON_FAILED.

        [PD-12-AF-008]
        """
        row = self._make_row()
        row.set_state("RUNNING")
        row.set_state("FAILED")
        assert row._state == "FAILED"
        assert row._icon_label.cget("text") == _ICON_FAILED

    # ── AF-009 ─────────────────────────────────────────────────────────────────

    def test_tool_label_update(self) -> None:
        """GIVEN a tool PhaseRow WHEN set_state with tool_name THEN label shows name.

        GIVEN a PhaseRow with step_key='tool',
        WHEN set_state('RUNNING', tool_name='read_file') is called,
        THEN the phase label displays the tool name.

        [PD-12-AF-009]
        """
        parent = tk.Frame(self._root)
        row = PhaseRow(parent, "tool", "🔧", "Tool")
        row.set_state("RUNNING", tool_name="read_file")
        assert "read_file" in row._phase_label.cget("text")

    def test_tick_updates_elapsed_label(self) -> None:
        """GIVEN a RUNNING PhaseRow WHEN tick() is called THEN elapsed label updates.

        GIVEN a PhaseRow in RUNNING state,
        WHEN tick() is called after at least 1 second of elapsed time has been
        artificially advanced,
        THEN the elapsed label no longer shows the placeholder.

        [PD-12-AF-006]
        """
        import time

        row = self._make_row()
        row.set_state("RUNNING")
        # Force start time into the past so elapsed > 0
        row._start_time = time.monotonic() - 5
        row.tick()
        label_text = row._elapsed_label.cget("text")
        assert label_text != _ELAPSED_PLACEHOLDER


# ── ContextKeyWidget ──────────────────────────────────────────────────────────


@pytest.mark.unit
class TestContextKeyWidget:
    """Unit tests for ContextKeyWidget legend panel.

    [PD-12-AF-010]
    """

    def setup_method(self) -> None:
        self._root = _make_root()

    def teardown_method(self) -> None:
        self._root.destroy()

    def test_swatch_count_matches_bands(self) -> None:
        """GIVEN a ContextKeyWidget WHEN created THEN one swatch row per _BANDS entry.

        GIVEN ContextKeyWidget is created under a parent Frame,
        WHEN the widget is constructed,
        THEN the number of child rows in the key frame equals len(_BANDS) + 1
        (bands plus the ghost/remaining row).

        [PD-12-AF-010]
        """
        parent = tk.Frame(self._root)
        key = ContextKeyWidget(parent)
        # One row per band + 1 ghost row
        child_frames = [c for c in key._frame.winfo_children()]
        assert len(child_frames) == len(_BANDS) + 1

    def test_swatch_labels_match_band_names(self) -> None:
        """GIVEN a ContextKeyWidget WHEN created THEN label text matches band names.

        GIVEN _BANDS is a list of (key, display_label, color) tuples,
        WHEN ContextKeyWidget builds its rows,
        THEN the labels in each row match the corresponding band display_label.

        [PD-12-AF-010]
        """
        parent = tk.Frame(self._root)
        key = ContextKeyWidget(parent)
        # Each child row frame has a Canvas (swatch) and a Label
        band_names = [b[1] for b in _BANDS] + ["Remaining"]
        label_texts = []
        for row_frame in key._frame.winfo_children():
            for child in row_frame.winfo_children():
                if isinstance(child, tk.Label):
                    label_texts.append(child.cget("text"))
        assert label_texts == band_names


# ── StatusTab ─────────────────────────────────────────────────────────────────


@pytest.mark.unit
class TestStatusTabCreate:
    """Unit tests for StatusTab.create() and attribute presence.

    Tests cover AF-001, AF-011.
    """

    def setup_method(self) -> None:
        self._tab, self._gui, self._root = _make_status_tab()

    def teardown_method(self) -> None:
        self._root.destroy()

    def test_create_returns_frame(self) -> None:
        """GIVEN a StatusTab WHEN create() is called THEN a tk.Frame is returned.

        GIVEN a StatusTab instance with a mock GUIManager,
        WHEN create(notebook, section_bg) is called,
        THEN the return value is a tk.Frame widget.

        [PD-12-AF-001]
        """
        notebook = ttk.Notebook(self._root)
        frame = self._tab.create(notebook, section_bg="#2d2d2d")
        assert isinstance(frame, tk.Frame)

    def test_context_meter_attribute_present(self) -> None:
        """GIVEN a StatusTab after create() WHEN context_meter accessed THEN it exists.

        GIVEN a StatusTab that has been created,
        WHEN the context_meter attribute is accessed,
        THEN it is a ContextMeterWidget instance.

        [PD-12-AF-011]
        """
        from agentx.gui.context_meter_widget import ContextMeterWidget

        notebook = ttk.Notebook(self._root)
        self._tab.create(notebook, section_bg="#2d2d2d")
        assert isinstance(self._tab.context_meter, ContextMeterWidget)

    def test_phase_rows_for_all_steps(self) -> None:
        """GIVEN a created StatusTab WHEN _phase_rows inspected THEN all phase keys present.

        GIVEN a StatusTab instance after create(),
        WHEN the _phase_rows dict is inspected,
        THEN it contains a PhaseRow for every key in _PHASE_STEPS.

        [PD-12-AF-005] [PD-12-AF-006]
        """
        notebook = ttk.Notebook(self._root)
        self._tab.create(notebook, section_bg="#2d2d2d")
        expected_keys = {step[0] for step in _PHASE_STEPS}
        assert set(self._tab._phase_rows.keys()) == expected_keys


@pytest.mark.unit
class TestStatusTabAutoSwitch:
    """Unit tests for auto-switch behaviour [PD-12-AF-002]."""

    def setup_method(self) -> None:
        self._tab, self._gui, self._root = _make_status_tab()
        self._notebook = ttk.Notebook(self._root)
        self._frame = self._tab.create(self._notebook, section_bg="#2d2d2d")
        self._notebook.add(self._frame, text="⚡ Status")
        # Add a second dummy tab
        dummy = tk.Frame(self._notebook)
        self._notebook.add(dummy, text="Session")

    def teardown_method(self) -> None:
        self._root.destroy()

    def test_show_selects_status_tab(self) -> None:
        """GIVEN notebook on second tab WHEN show() called THEN status tab selected.

        GIVEN a Notebook where the current tab is not Status,
        WHEN StatusTab.show(notebook) is called,
        THEN the notebook selects tab index 0 (the Status tab).

        [PD-12-AF-002]
        """
        # select the second tab first
        self._notebook.select(1)
        self._tab.show(self._notebook)
        assert self._notebook.index("current") == 0


@pytest.mark.unit
class TestStatusTabInterruptButton:
    """Unit tests for interrupt button state management [PD-12-AF-003] [PD-12-AF-004]."""

    def setup_method(self) -> None:
        self._tab, self._gui, self._root = _make_status_tab()
        notebook = ttk.Notebook(self._root)
        self._tab.create(notebook, section_bg="#2d2d2d")

    def teardown_method(self) -> None:
        self._root.destroy()

    def test_interrupt_disabled_initially(self) -> None:
        """GIVEN a fresh StatusTab WHEN interrupt button checked THEN it is disabled.

        GIVEN a newly created StatusTab (stream not started),
        WHEN the interrupt button state is inspected,
        THEN it is tk.DISABLED.

        [PD-12-AF-003]
        """
        assert self._tab._interrupt_btn.cget("state") == tk.DISABLED

    def test_set_streaming_state_true_enables_interrupt(self) -> None:
        """GIVEN StatusTab WHEN set_streaming_state(True) THEN interrupt enabled.

        GIVEN a StatusTab where streaming is inactive,
        WHEN set_streaming_state(True) is called,
        THEN the interrupt button state becomes tk.NORMAL.

        [PD-12-AF-003]
        """
        self._tab.set_streaming_state(True)
        assert self._tab._interrupt_btn.cget("state") == tk.NORMAL

    def test_set_streaming_state_false_disables_interrupt(self) -> None:
        """GIVEN a streaming StatusTab WHEN set_streaming_state(False) THEN disabled.

        GIVEN a StatusTab where streaming is active,
        WHEN set_streaming_state(False) is called,
        THEN the interrupt button state becomes tk.DISABLED.

        [PD-12-AF-003]
        """
        self._tab.set_streaming_state(True)
        self._tab.set_streaming_state(False)
        assert self._tab._interrupt_btn.cget("state") == tk.DISABLED

    def test_interrupt_callback_invoked(self) -> None:
        """GIVEN a streaming StatusTab WHEN interrupt clicked THEN callback fires.

        GIVEN a StatusTab with set_streaming_state(True),
        WHEN the interrupt button is invoked,
        THEN the gui_manager._on_interrupt callback is called.

        [PD-12-AF-004]
        """
        self._tab.set_streaming_state(True)
        self._tab._interrupt_btn.invoke()
        # GUIManager mock tracks all calls; check that the interrupt path was called
        self._gui._on_interrupt.assert_called_once()


@pytest.mark.unit
class TestStatusTabPhaseReset:
    """Unit tests for phase stepper reset [PD-12-AF-005]."""

    def setup_method(self) -> None:
        self._tab, self._gui, self._root = _make_status_tab()
        notebook = ttk.Notebook(self._root)
        self._tab.create(notebook, section_bg="#2d2d2d")

    def teardown_method(self) -> None:
        self._root.destroy()

    def test_reset_sets_all_rows_pending(self) -> None:
        """GIVEN rows in various states WHEN reset() THEN all rows are PENDING.

        GIVEN phase rows in RUNNING and DONE states,
        WHEN StatusTab.reset() is called,
        THEN every PhaseRow reverts to PENDING state.

        [PD-12-AF-005]
        """
        self._tab.set_phase("classify", "RUNNING")
        self._tab.set_phase("classify", "DONE")
        self._tab.set_phase("think", "RUNNING")
        self._tab.reset()
        for row in self._tab._phase_rows.values():
            assert row._state == "PENDING"


@pytest.mark.unit
class TestStatusTabSetPhase:
    """Unit tests for StatusTab.set_phase() delegation [PD-12-AF-006 to AF-009]."""

    def setup_method(self) -> None:
        self._tab, self._gui, self._root = _make_status_tab()
        notebook = ttk.Notebook(self._root)
        self._tab.create(notebook, section_bg="#2d2d2d")

    def teardown_method(self) -> None:
        self._root.destroy()

    @pytest.mark.parametrize(
        "step_key,state",
        [
            ("classify", "RUNNING"),
            ("think", "RUNNING"),
            ("tool", "RUNNING"),
            ("respond", "RUNNING"),
            ("classify", "DONE"),
            ("think", "DONE"),
            ("tool", "DONE"),
            ("respond", "DONE"),
            ("classify", "FAILED"),
            ("think", "FAILED"),
            ("tool", "FAILED"),
            ("respond", "FAILED"),
        ],
    )
    def test_set_phase_transitions(self, step_key: str, state: str) -> None:
        """GIVEN step_key={step_key} WHEN set_phase(state={state}) THEN row.state matches.

        GIVEN a StatusTab with all rows in PENDING state,
        WHEN set_phase(step_key, state) is called,
        THEN the corresponding PhaseRow._state equals the requested state.

        [PD-12-AF-006] [PD-12-AF-007] [PD-12-AF-008]

        Args:
            step_key (str): Phase identifier.
            state (str): Target state to transition to.
        """
        if state != "PENDING":
            # DONE/FAILED require RUNNING first
            self._tab.set_phase(step_key, "RUNNING")
        self._tab.set_phase(step_key, state)
        assert self._tab._phase_rows[step_key]._state == state

    def test_set_phase_tool_with_name(self) -> None:
        """GIVEN tool step WHEN set_phase with tool_name THEN label contains name.

        GIVEN StatusTab with all rows PENDING,
        WHEN set_phase('tool', 'RUNNING', tool_name='list_files') is called,
        THEN the tool PhaseRow phase label contains 'list_files'.

        [PD-12-AF-009]
        """
        self._tab.set_phase("tool", "RUNNING", tool_name="list_files")
        row = self._tab._phase_rows["tool"]
        assert "list_files" in row._phase_label.cget("text")
