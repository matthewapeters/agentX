"""Status Tab — real-time streaming status panel.

Implements PD-12.  Spec: docs/ux/03_PANEL_DETAILS.md §PD-12.

The tab is the **first** tab in ``SidePanel.system_notebook`` and auto-activates
whenever a prompt is submitted.  It contains three sections stacked vertically:

1. ``ContextWindowSection`` (``tk.LabelFrame``) — re-hosts ``ContextMeterWidget``
   (relocated from ``InputPanel``) alongside a ``ContextKeyWidget`` colour-key
   legend.
2. ``PhaseStepperWidget`` (``tk.LabelFrame``) — one row per prompt-cycle phase:
   Classify, Think, Tool, Respond.  Each row shows a status icon, emoji label,
   and an elapsed timer (HH:MM:SS).
3. ``InterruptButton`` (``tk.Button``) — full-width; enabled only while streaming.

All state updates are safe to call off-screen; Tkinter defers the paint until
the tab is next rendered.

Affordances: PD-12-AF-001 through PD-12-AF-011.
"""

from __future__ import annotations

import logging
import time
import tkinter as tk
from typing import TYPE_CHECKING, Callable, Optional

from .context_meter_widget import _BANDS, _GHOST_COLOR, ContextMeterWidget

if TYPE_CHECKING:
    from .gui_manager import GUIManager

logger = logging.getLogger(__name__)

# ── Phase step definitions ────────────────────────────────────────────────────
# Each entry: (step_key, emoji, display_label)
_PHASE_STEPS: list[tuple[str, str, str]] = [
    ("classify", "\U0001f914", "Classify"),
    ("think", "\U0001f4ad", "Think"),
    ("tool", "\U0001f527", "Tool"),
    ("respond", "\u270d\ufe0f", "Respond"),
]

# Status icon characters per state
_ICON_PENDING = "\u25cb"  # ○
_ICON_RUNNING = "\u21bb"  # ↻
_ICON_DONE = "\u2713"  # ✓
_ICON_FAILED = "\u2717"  # ✗

# Elapsed placeholder when not yet started
_ELAPSED_PLACEHOLDER = "--:--:--"


def _format_elapsed(seconds: float) -> str:
    """Format an elapsed duration as HH:MM:SS.

    Args:
        seconds (float): Elapsed seconds (non-negative).

    Returns:
        str: Human-readable duration string in HH:MM:SS format.
    """
    total = int(max(0, seconds))
    hh = total // 3600
    mm = (total % 3600) // 60
    ss = total % 60
    return f"{hh:02d}:{mm:02d}:{ss:02d}"


class PhaseRow:
    """One row in the PhaseStepperWidget representing a single prompt-cycle phase.

    [PD-12-AF-005] [PD-12-AF-006] [PD-12-AF-007] [PD-12-AF-008] [PD-12-AF-009]

    Attributes:
        step_key (str): Logical key for this phase (e.g. "classify", "tool").
        _state (str): One of PENDING / RUNNING / DONE / FAILED.
        _start_time (Optional[float]): monotonic timestamp of RUNNING transition.
        _final_elapsed (Optional[float]): Frozen elapsed seconds when DONE/FAILED.
    """

    def __init__(self, parent: tk.Frame, step_key: str, emoji: str, label: str) -> None:
        """Construct a PhaseRow and pack it into parent.

        Args:
            parent (tk.Frame): Parent widget to pack into.
            step_key (str): Logical phase key.
            emoji (str): Emoji prefix for the label.
            label (str): Human-readable phase name.
        """
        self.step_key: str = step_key
        self._state: str = "PENDING"
        self._start_time: Optional[float] = None
        self._final_elapsed: Optional[float] = None
        self._base_label: str = f"{emoji} {label}"

        self._row_frame = tk.Frame(parent)
        self._row_frame.pack(fill=tk.X, pady=1)

        self._icon_label = tk.Label(self._row_frame, text=_ICON_PENDING, width=2, anchor="w")
        self._icon_label.pack(side=tk.LEFT)

        self._phase_label = tk.Label(self._row_frame, text=self._base_label, anchor="w")
        self._phase_label.pack(side=tk.LEFT, fill=tk.X, expand=True)

        self._elapsed_label = tk.Label(self._row_frame, text=_ELAPSED_PLACEHOLDER, width=8, anchor="e")
        self._elapsed_label.pack(side=tk.RIGHT)

    def reset(self) -> None:
        """Reset this row to PENDING state.

        [PD-12-AF-005]
        """
        self._state = "PENDING"
        self._start_time = None
        self._final_elapsed = None
        self._icon_label.config(text=_ICON_PENDING)
        self._phase_label.config(text=self._base_label)
        self._elapsed_label.config(text=_ELAPSED_PLACEHOLDER)

    def set_state(
        self,
        state: str,
        tool_name: Optional[str] = None,
        start_time: Optional[float] = None,
    ) -> None:
        """Transition this row to a new state.

        [PD-12-AF-006] [PD-12-AF-007] [PD-12-AF-008] [PD-12-AF-009]

        Args:
            state (str): Target state — one of RUNNING, DONE, FAILED.
            tool_name (Optional[str]): When state is RUNNING and step_key is
                ``"tool"``, update the label to show the active tool name.
            start_time (Optional[float]): ``time.monotonic()`` timestamp captured
                on the background thread when the phase began.  Passed for
                phases (e.g. ``respond``) whose RUNNING and DONE transitions are
                queued back-to-back on the Tk thread, which would otherwise give
                zero elapsed.  When provided for RUNNING the row stores it
                instead of calling ``time.monotonic()`` again; when provided for
                DONE/FAILED it is used as the phase start anchor.
        """
        self._state = state
        if state == "RUNNING":
            self._start_time = start_time if start_time is not None else time.monotonic()
            self._final_elapsed = None
            self._icon_label.config(text=_ICON_RUNNING)
            if self.step_key == "tool" and tool_name:
                self._phase_label.config(text=f"\U0001f527 Tool: {tool_name}")
        elif state == "DONE":
            anchor = self._start_time if start_time is None else start_time
            if anchor is not None:
                self._final_elapsed = time.monotonic() - anchor
            self._icon_label.config(text=_ICON_DONE)
            if self._final_elapsed is not None:
                self._elapsed_label.config(text=_format_elapsed(self._final_elapsed))
        elif state == "FAILED":
            anchor = self._start_time if start_time is None else start_time
            if anchor is not None:
                self._final_elapsed = time.monotonic() - anchor
            self._icon_label.config(text=_ICON_FAILED)
            if self._final_elapsed is not None:
                self._elapsed_label.config(text=_format_elapsed(self._final_elapsed))

    def tick(self) -> None:
        """Update the elapsed display if this row is currently RUNNING.

        Called by the StatusTab 1-second tick loop.
        """
        if self._state == "RUNNING" and self._start_time is not None:
            elapsed = time.monotonic() - self._start_time
            self._elapsed_label.config(text=_format_elapsed(elapsed))


class ContextKeyWidget:
    """Colour-key legend for the ContextMeterWidget donut.

    [PD-12-AF-010]

    Renders one labelled swatch row per band in ``_BANDS`` order (sourced from
    ``context_meter_widget._BANDS``) plus a final ghost-arc row for remaining
    capacity.  The key is always in sync with the donut because both read from
    the same constant.
    """

    def __init__(self, parent: tk.Widget) -> None:
        """Build and pack the colour-key into parent.

        Args:
            parent (tk.Widget): Container to pack rows into.
        """
        self._frame = tk.Frame(parent)
        self._frame.pack(side=tk.LEFT, fill=tk.Y, padx=6)
        for _, display_label, color in _BANDS:
            self._add_row(display_label, color)
        self._add_row("Remaining", _GHOST_COLOR)

    def _add_row(self, label: str, color: str) -> None:
        """Add a single swatch + label row.

        Args:
            label (str): Display name for this band.
            color (str): Hex colour string for the swatch.
        """
        row = tk.Frame(self._frame)
        row.pack(fill=tk.X, pady=1)
        swatch = tk.Canvas(row, width=14, height=14, highlightthickness=0)
        swatch.pack(side=tk.LEFT, padx=(0, 4))
        swatch.create_rectangle(0, 0, 14, 14, fill=color, outline="")
        tk.Label(row, text=label, anchor="w").pack(side=tk.LEFT)


class StatusTab:
    """Status Tab widget — PD-12.

    Hosts the ContextMeterWidget (re-parented from InputPanel), a colour-key
    legend, the PhaseStepperWidget, and the Interrupt button.

    [PD-12-AF-001] Tab is the first in system_notebook.
    [PD-12-AF-002] Auto-switch via ``show()`` called from StreamingController.
    [PD-12-AF-003] Interrupt button state tracks streaming.
    [PD-12-AF-004] Interrupt button invokes on_interrupt callback.
    [PD-12-AF-011] ContextMeterWidget is hosted here.

    Attributes:
        context_meter (ContextMeterWidget): Donut chart widget; exposed so
            GUIManager.update_context_meter() can delegate to it.
    """

    def __init__(self, gui_manager: "GUIManager") -> None:
        """Initialise StatusTab with a reference to the owning GUIManager.

        Args:
            gui_manager (GUIManager): Owning GUI manager; provides root, config,
                and on_interrupt callback.
        """
        self._g = gui_manager
        self._tab_frame: Optional[tk.Frame] = None
        self._phase_rows: dict[str, PhaseRow] = {}
        self._tick_id: Optional[str] = None
        self.context_meter: ContextMeterWidget = ContextMeterWidget(gui_manager)
        self._interrupt_btn: Optional[tk.Button] = None

    # ── Creation ──────────────────────────────────────────────────────────────

    def create(self, notebook: "tk.ttk.Notebook", section_bg: str) -> tk.Frame:
        """Build all sub-widgets and return the tab frame.

        [PD-12-AF-001] Caller inserts this frame as the first notebook tab.
        [PD-12-AF-011] ContextMeterWidget is created here.

        Args:
            notebook (ttk.Notebook): The system notebook to host the tab.
            section_bg (str): Background colour from GUI config.

        Returns:
            tk.Frame: The tab frame (to be passed to notebook.insert(0, ...)).
        """
        from tkinter import ttk  # noqa: F401 — already imported by side_panel but safe here

        self._tab_frame = tk.Frame(notebook, bg=section_bg)

        # ── Section 1: Context Window ──────────────────────────────────────
        ctx_frame = tk.LabelFrame(self._tab_frame, text="Context Window", padx=4, pady=4)
        ctx_frame.pack(fill=tk.X, padx=6, pady=(6, 2))

        # Legend on the left; donut fills the remaining space on the right.
        # The canvas <Configure> binding redraws at the actual pixel size so
        # the donut scales dynamically — no hard-coded diameter needed.
        ContextKeyWidget(ctx_frame)
        donut_frame = tk.Frame(ctx_frame)
        donut_frame.pack(side=tk.LEFT, fill=tk.BOTH, expand=True)
        self.context_meter.create(donut_frame, relx=0, rely=0, relwidth=1.0, relheight=1.0)

        # ── Section 2: Prompt Cycle phases ────────────────────────────────
        phase_frame = tk.LabelFrame(self._tab_frame, text="Prompt Cycle", padx=4, pady=4)
        phase_frame.pack(fill=tk.BOTH, expand=True, padx=6, pady=2)

        for step_key, emoji, label in _PHASE_STEPS:
            row = PhaseRow(phase_frame, step_key, emoji, label)
            self._phase_rows[step_key] = row

        # ── Section 3: Interrupt button ────────────────────────────────────
        self._interrupt_btn = tk.Button(
            self._tab_frame,
            text="\u26d4  Interrupt  (Ctrl+Space)",
            state=tk.DISABLED,
            command=self._on_interrupt_clicked,
        )
        self._interrupt_btn.pack(fill=tk.X, padx=6, pady=(2, 6))

        # Global Ctrl+Space binding (moved from InputPanel — PD-12-AF-003)
        self._g.root.bind_all(
            "<Control-space>",
            lambda _event: self._on_interrupt_clicked(),
        )

        return self._tab_frame

    # ── Public API ────────────────────────────────────────────────────────────

    def show(self, notebook: "tk.ttk.Notebook") -> None:
        """Switch the system notebook to the Status tab.

        [PD-12-AF-002] Called by StreamingController._on_stream_start().

        Args:
            notebook (ttk.Notebook): The system notebook widget.
        """
        if self._tab_frame is not None:
            try:
                notebook.select(self._tab_frame)
            except tk.TclError:
                logger.debug("StatusTab.show: tab not yet registered in notebook")

    def set_streaming_state(self, is_streaming: bool) -> None:
        """Enable/disable the interrupt button to match streaming state.

        [PD-12-AF-003]

        Args:
            is_streaming (bool): True while an LLM stream is active.
        """
        if self._interrupt_btn is None:
            return
        try:
            self._interrupt_btn.config(state=tk.NORMAL if is_streaming else tk.DISABLED)
        except tk.TclError:
            pass

    def reset(self) -> None:
        """Reset all phase rows to PENDING and restart the tick loop.

        [PD-12-AF-005] Called by StreamingController._on_stream_start().
        """
        for row in self._phase_rows.values():
            row.reset()
        self._stop_tick()
        self._start_tick()

    def set_phase(
        self,
        step_key: str,
        state: str,
        tool_name: Optional[str] = None,
        start_time: Optional[float] = None,
    ) -> None:
        """Transition a phase row to a new state.

        [PD-12-AF-006] [PD-12-AF-007] [PD-12-AF-008] [PD-12-AF-009]

        Args:
            step_key (str): One of "classify", "think", "tool", "respond".
            state (str): Target state — RUNNING, DONE, or FAILED.
            tool_name (Optional[str]): Tool name injected into the tool-step
                label when state is RUNNING and step_key is "tool".
            start_time (Optional[float]): Background-thread ``time.monotonic()``
                timestamp to anchor the elapsed timer accurately when RUNNING
                and DONE are queued back-to-back.
        """
        row = self._phase_rows.get(step_key)
        if row is None:
            logger.warning("StatusTab.set_phase: unknown step_key %r", step_key)
            return
        row.set_state(state, tool_name=tool_name, start_time=start_time)

    def stop_tick(self) -> None:
        """Stop the elapsed timer tick loop (called when streaming ends)."""
        self._stop_tick()

    # ── Private helpers ───────────────────────────────────────────────────────

    def _on_interrupt_clicked(self) -> None:
        """Invoke the on_interrupt callback if one is registered.

        [PD-12-AF-004]
        """
        if self._g._on_interrupt:
            self._g._on_interrupt()

    def _start_tick(self) -> None:
        """Schedule the 1-second elapsed timer tick loop."""
        self._tick()

    def _stop_tick(self) -> None:
        """Cancel the pending tick callback if one is scheduled."""
        if self._tick_id is not None:
            try:
                self._g.root.after_cancel(self._tick_id)
            except tk.TclError:
                pass
            self._tick_id = None

    def _tick(self) -> None:
        """Update elapsed labels for all RUNNING rows and reschedule."""
        for row in self._phase_rows.values():
            row.tick()
        self._tick_id = self._g.root.after(1000, self._tick)
