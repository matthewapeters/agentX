"""Context Meter Widget — donut chart showing LLM context window usage by category.

Implements ARCH-04 from docs/ux/context_visualizer.md.

The widget is a ``tk.Canvas`` placed inside the input-panel button column
(``relx=0.92, rely=0.00, relwidth=0.07, relheight=0.24``) — above the Submit
button, no layout change required.

Band categories (BAND-01 through BAND-07) map to ``Context.token_breakdown()``
keys.  A dim ghost arc always fills unused capacity (ENH-02).  The outer ring
border transitions through three risk states (ENH-16): default gray, warning
red at >= 80%, critical thick red at >= 100%.  Hover tooltips (ENH-06) are
implemented as ``tk.Toplevel`` overlays with ``overrideredirect(True)``.
"""

from __future__ import annotations

import logging
import tkinter as tk
from typing import TYPE_CHECKING, Optional

if TYPE_CHECKING:
    from .gui_manager import GUIManager

logger = logging.getLogger(__name__)

# ── Band definitions ─────────────────────────────────────────────────────────
# Each entry: (breakdown_key, display_label, hex_color)
# Order determines clockwise rendering sequence from 12 o'clock.
_BANDS: list[tuple[str, str, str]] = [
    ("working_memory", "Working Memory", "#0d9488"),  # Teal    — BAND-01
    ("system", "System Prompts", "#6366f1"),  # Indigo  — BAND-02
    ("user", "User Prompts", "#3b82f6"),  # Blue    — BAND-03
    ("attachments", "Attachments", "#f59e0b"),  # Amber   — BAND-04
    ("thinking", "Thinking", "#a855f7"),  # Purple  — BAND-05
    ("assistant", "Agent Response", "#22c55e"),  # Green   — BAND-06
    ("tool", "Tool Calls / Results", "#f97316"),  # Orange  — BAND-07
]

# Ghost arc color (remaining capacity — ENH-02)
_GHOST_COLOR = "#444444"

# Risk-state border specs: (outline_color, line_width_px)
_BORDER_DEFAULT = ("#9ca3af", 1)  # default — gray
_BORDER_WARNING = ("#CC4444", 1)  # warning — thin red
_BORDER_CRITICAL = ("#FF0000", 3)  # critical — thick red

# Risk thresholds (fraction of max_tokens)
_WARNING_THRESHOLD = 0.80
_CRITICAL_THRESHOLD = 1.00

# Donut hole as a fraction of the outer diameter
_HOLE_RATIO = 0.55

# Padding between canvas edge and outer ring (pixels)
_RING_PAD = 2


class ContextMeterWidget:
    """Donut chart widget displaying LLM context window utilisation by category.

    Follows the back-reference pattern: ``ContextMeterWidget(gui_manager)``
    stores ``self._g = gui_manager`` and reads display config from there.

    Usage::

        widget = ContextMeterWidget(gui_manager)
        widget.create(parent_frame)          # call once on main thread
        widget.update(max_tokens, breakdown) # call whenever data changes

    The ``update()`` method is safe to call from background threads; it
    schedules the actual canvas redraw via ``canvas.after(0, ...)`` per
    ARCH-06 thread-safety requirements.
    """

    def __init__(self, gui_manager: "GUIManager") -> None:
        """Initialise the widget with a back-reference to the GUI manager.

        Args:
            gui_manager (GUIManager): Owning GUI manager instance; used to read
                display configuration (colors, fonts) without coupling to the
                widget's presentation logic.
        """
        self._g = gui_manager
        self._canvas: Optional[tk.Canvas] = None
        self._tooltip: Optional[tk.Toplevel] = None

        # Cached state so tooltips can be rebuilt after resize
        self._cached_max_tokens: int = 4096
        self._cached_breakdown: dict[str, int] = {}

    # ── Convenience ──────────────────────────────────────────────────────────

    @property
    def _config(self):
        """Return the GUI configuration object from the owning manager."""
        return self._g.config

    # ── Public API ────────────────────────────────────────────────────────────

    def create(self, parent: tk.Widget) -> None:
        """Create and place the canvas inside ``parent``.

        Must be called on the Tkinter main thread after the parent frame
        exists.  Places the canvas at ``relx=0.92, rely=0.00,
        relwidth=0.07, relheight=0.24`` — matching the existing button
        column geometry, above the Submit button.

        Args:
            parent (tk.Widget): Parent frame (the ``user_input`` frame).
        """
        bg = self._config.input_bg
        self._canvas = tk.Canvas(parent, bg=bg, highlightthickness=0, cursor="hand2")
        self._canvas.place(relx=0.92, rely=0.00, relwidth=0.07, relheight=0.24)

        # Bind Configure so the donut redraws if the canvas is resized
        self._canvas.bind("<Configure>", self._on_configure)

        # Initial render with empty state
        self._render(self._cached_max_tokens, self._cached_breakdown)

    def update(self, max_tokens: int, breakdown: dict[str, int]) -> None:
        """Redraw the donut with new token data.

        Thread-safe: schedules the canvas redraw on the Tkinter main thread
        via ``canvas.after(0, ...)``.

        Args:
            max_tokens (int): Context window size for the active model
                (denominator for arc proportions).
            breakdown (dict[str, int]): Per-category token estimates from
                ``Context.token_breakdown()``.  Expected keys:
                ``working_memory``, ``system``, ``user``, ``attachments``,
                ``thinking``, ``assistant``, ``tool``.
        """
        self._cached_max_tokens = max(max_tokens, 1)
        self._cached_breakdown = dict(breakdown)
        if self._canvas is not None:
            self._canvas.after(0, lambda: self._render(self._cached_max_tokens, self._cached_breakdown))

    # ── Event handlers ────────────────────────────────────────────────────────

    def _on_configure(self, _event: tk.Event) -> None:  # type: ignore[type-arg]
        """Redraw when the canvas is resized.

        Args:
            _event (tk.Event): Tkinter ``<Configure>`` event (unused).
        """
        self._render(self._cached_max_tokens, self._cached_breakdown)

    # ── Internal rendering ────────────────────────────────────────────────────

    def _render(self, max_tokens: int, breakdown: dict[str, int]) -> None:
        """Redraw the full donut chart.

        Must be called on the Tkinter main thread.  Clears the canvas, then
        paints arc slices, ghost arc, hole overlay, center label, and border
        ring in sequence.  Binds hover tooltip events after painting.

        Args:
            max_tokens (int): Context-window denominator.
            breakdown (dict[str, int]): Token counts per band key.
        """
        canvas = self._canvas
        if canvas is None:
            return

        canvas.delete("all")

        w = canvas.winfo_width()
        h = canvas.winfo_height()
        if w < 4 or h < 4:
            # Canvas not yet laid out; retry after the next layout pass
            canvas.after(50, lambda: self._render(max_tokens, breakdown))
            return

        size = min(w, h)
        # Centre the ring within the canvas
        ox = (w - size) / 2 + _RING_PAD
        oy = (h - size) / 2 + _RING_PAD
        bbox = (ox, oy, ox + size - 2 * _RING_PAD, oy + size - 2 * _RING_PAD)

        total_tokens = max(sum(breakdown.values()), 0)
        pct = total_tokens / max_tokens if max_tokens > 0 else 0.0

        self._draw_slices(bbox, breakdown, max_tokens)
        self._draw_ghost_arc(bbox, pct)
        self._draw_hole(bbox, size)
        self._draw_center_label(w, h, pct, total_tokens, max_tokens)
        self._draw_border(bbox, pct)
        self._bind_tooltips(breakdown, max_tokens, pct, total_tokens)

    def _draw_slices(
        self,
        bbox: tuple[float, float, float, float],
        breakdown: dict[str, int],
        max_tokens: int,
    ) -> None:
        """Draw one PIESLICE arc per non-zero band category.

        Args:
            bbox (tuple[float, float, float, float]): Bounding box ``(x0, y0, x1, y1)``.
            breakdown (dict[str, int]): Token counts per band key.
            max_tokens (int): Context-window denominator.
        """
        canvas = self._canvas
        if canvas is None:
            return
        start_angle = 90.0  # 12 o'clock; clockwise with negative extent
        for band_key, _label, color in _BANDS:
            tokens = breakdown.get(band_key, 0)
            if tokens <= 0:
                continue
            fraction = min(tokens / max_tokens, 1.0) if max_tokens > 0 else 0.0
            if fraction < 1e-6:
                continue
            extent = -(fraction * 360.0)
            canvas.create_arc(
                *bbox,
                start=start_angle,
                extent=extent,
                fill=color,
                outline="",
                style=tk.PIESLICE,
                tags=(f"band_{band_key}", "slice"),
            )
            start_angle += extent

    def _draw_ghost_arc(
        self,
        bbox: tuple[float, float, float, float],
        pct: float,
    ) -> None:
        """Draw the dim ghost arc representing unused capacity (ENH-02).

        Args:
            bbox (tuple[float, float, float, float]): Bounding box ``(x0, y0, x1, y1)``.
            pct (float): Current usage as a fraction of ``max_tokens`` (0–1+).
        """
        canvas = self._canvas
        if canvas is None:
            return
        used = min(pct, 1.0)
        remaining = max(1.0 - used, 0.0)
        if remaining < 1e-6:
            return
        # Start angle is where the last slice ended, i.e. 90 - used*360
        start_angle = 90.0 - used * 360.0
        ghost_extent = -(remaining * 360.0)
        canvas.create_arc(
            *bbox,
            start=start_angle,
            extent=ghost_extent,
            fill=_GHOST_COLOR,
            outline="",
            style=tk.PIESLICE,
            tags=("ghost", "slice"),
        )

    def _draw_hole(
        self,
        bbox: tuple[float, float, float, float],
        size: float,
    ) -> None:
        """Overlay a filled oval to carve the donut hole.

        Args:
            bbox (tuple[float, float, float, float]): Outer ring bounding box.
            size (float): Pixel diameter of the outer ring area.
        """
        canvas = self._canvas
        if canvas is None:
            return
        ox, oy, ox1, oy1 = bbox
        ring_diam = size - 2 * _RING_PAD
        hole_margin = ring_diam * (1.0 - _HOLE_RATIO) / 2.0
        hx0 = ox + hole_margin
        hy0 = oy + hole_margin
        hx1 = ox1 - hole_margin
        hy1 = oy1 - hole_margin
        canvas.create_oval(hx0, hy0, hx1, hy1, fill=self._config.input_bg, outline="", tags=("hole",))

    def _draw_center_label(
        self,
        canvas_w: float,
        canvas_h: float,
        pct: float,
        total_tokens: int,
        max_tokens: int,
    ) -> None:
        """Draw the percentage label in the center of the donut hole (ENH-05).

        Color reflects risk state: default input_fg, warning red, critical
        bright red — providing non-color cue via numeric value.

        Args:
            canvas_w (float): Canvas pixel width.
            canvas_h (float): Canvas pixel height.
            pct (float): Usage fraction (0–1+).
            total_tokens (int): Sum of all band token counts.
            max_tokens (int): Context-window denominator.
        """
        canvas = self._canvas
        if canvas is None:
            return
        if pct >= _CRITICAL_THRESHOLD:
            color = "#FF0000"
        elif pct >= _WARNING_THRESHOLD:
            color = "#CC4444"
        else:
            color = self._config.input_fg

        label = f"{min(int(pct * 100), 999)}%"
        canvas.create_text(
            canvas_w / 2,
            canvas_h / 2,
            text=label,
            fill=color,
            font=("TkSmallCaptionFont", 7),
            tags=("pct_label",),
        )

    def _draw_border(
        self,
        bbox: tuple[float, float, float, float],
        pct: float,
    ) -> None:
        """Draw the outer ring border in the appropriate risk state (ENH-16).

        Args:
            bbox (tuple[float, float, float, float]): Bounding box for the ring.
            pct (float): Usage fraction (0–1+).
        """
        canvas = self._canvas
        if canvas is None:
            return
        if pct >= _CRITICAL_THRESHOLD:
            color, width = _BORDER_CRITICAL
        elif pct >= _WARNING_THRESHOLD:
            color, width = _BORDER_WARNING
        else:
            color, width = _BORDER_DEFAULT
        canvas.create_oval(*bbox, fill="", outline=color, width=width, tags=("border",))

    # ── Tooltip binding ───────────────────────────────────────────────────────

    def _bind_tooltips(
        self,
        breakdown: dict[str, int],
        max_tokens: int,
        pct: float,
        total_tokens: int,
    ) -> None:
        """Bind ``<Enter>``/``<Leave>`` tooltip events to all canvas items (ENH-06).

        Each band slice gets a role-specific tooltip; the ghost arc and hole
        get summary tooltips.

        Args:
            breakdown (dict[str, int]): Token counts per band key.
            max_tokens (int): Context-window size.
            pct (float): Usage fraction.
            total_tokens (int): Sum of all enabled token counts.
        """
        canvas = self._canvas
        if canvas is None:
            return

        for band_key, label, _color in _BANDS:
            tokens = breakdown.get(band_key, 0)
            if tokens <= 0:
                continue
            band_pct = (tokens / max_tokens * 100) if max_tokens > 0 else 0.0
            tip = f"{label}\n~{tokens:,} tokens  \u00b7  {band_pct:.1f}%\n(estimated: model-ratio)"

            tag = f"band_{band_key}"
            canvas.tag_bind(tag, "<Enter>", self._make_enter_handler(tip))
            canvas.tag_bind(tag, "<Leave>", lambda _e: self._hide_tooltip())

        # Ghost arc tooltip
        remaining = max(max_tokens - total_tokens, 0)
        rem_pct = (remaining / max_tokens * 100) if max_tokens > 0 else 0.0
        ghost_tip = f"Remaining capacity\n~{remaining:,} tokens  \u00b7  {rem_pct:.1f}% free"
        canvas.tag_bind("ghost", "<Enter>", self._make_enter_handler(ghost_tip))
        canvas.tag_bind("ghost", "<Leave>", lambda _e: self._hide_tooltip())

        # Hole + center label tooltip (overall summary)
        if pct >= _CRITICAL_THRESHOLD:
            status = "CRITICAL \u2014 trim will fire on next send"
        elif pct >= _WARNING_THRESHOLD:
            status = "WARNING \u2014 context is nearly full"
        else:
            status = "OK"
        hole_tip = (
            f"{min(int(pct * 100), 999)}% used  \u00b7  {total_tokens:,}\u00a0/\u00a0{max_tokens:,} tokens\n"
            f"Counting method: model-ratio estimate\n"
            f"Status: {status}"
        )
        for tag in ("hole", "pct_label"):
            canvas.tag_bind(tag, "<Enter>", self._make_enter_handler(hole_tip))
            canvas.tag_bind(tag, "<Leave>", lambda _e: self._hide_tooltip())

    def _make_enter_handler(self, text: str):
        """Return a closure that shows a tooltip with ``text`` on ``<Enter>``.

        Args:
            text (str): Tooltip text to display.

        Returns:
            Callable: Tkinter event handler that calls ``_show_tooltip``.
        """

        def handler(event: tk.Event) -> None:  # type: ignore[type-arg]
            self._show_tooltip(event, text)

        return handler

    def _show_tooltip(self, event: tk.Event, text: str) -> None:  # type: ignore[type-arg]
        """Display a small borderless tooltip window near the cursor.

        Args:
            event (tk.Event): The ``<Enter>`` event providing cursor position.
            text (str): Multi-line tooltip text.
        """
        self._hide_tooltip()
        canvas = self._canvas
        if canvas is None:
            return
        try:
            tw = tk.Toplevel(canvas)
            tw.overrideredirect(True)
            tw.attributes("-topmost", True)
            tk.Label(
                tw,
                text=text,
                justify=tk.LEFT,
                background="#1e293b",
                foreground="#f1f5f9",
                relief=tk.FLAT,
                bd=0,
                padx=6,
                pady=4,
                font=("TkSmallCaptionFont", 9),
            ).pack()
            tw.geometry(f"+{event.x_root + 14}+{event.y_root + 14}")
            self._tooltip = tw
        except tk.TclError as exc:
            logger.debug("ContextMeterWidget: tooltip creation failed: %s", exc)

    def _hide_tooltip(self) -> None:
        """Destroy the currently-visible tooltip window, if any."""
        if self._tooltip is not None:
            try:
                self._tooltip.destroy()
            except tk.TclError:
                pass
            self._tooltip = None
