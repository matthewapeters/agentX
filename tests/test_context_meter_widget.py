"""Unit tests for ContextMeterWidget (ARCH-04).

Tests cover:
- Rendering lifecycle: create → update → canvas operations
- Arc slice generation per band category
- Ghost arc for remaining capacity (ENH-02)
- Border risk-state transitions (ENH-16)
- Center-label color changes at warning/critical thresholds
- Tooltip text generation
- Thread-safe update scheduling
- Degenerate inputs (empty breakdown, max_tokens=0, over-100% state)
"""

from __future__ import annotations

from unittest.mock import MagicMock, call, patch

import pytest

from agentx.gui.context_meter_widget import (
    _BANDS,
    _BORDER_CRITICAL,
    _BORDER_DEFAULT,
    _BORDER_WARNING,
    _CRITICAL_THRESHOLD,
    _GHOST_COLOR,
    _WARNING_THRESHOLD,
    ContextMeterWidget,
)

# ── Fixtures ──────────────────────────────────────────────────────────────────


def _make_widget() -> ContextMeterWidget:
    """Build a ContextMeterWidget with a mock GUIManager.

    Returns:
        ContextMeterWidget: Widget instance ready for unit testing.
    """
    gui = MagicMock()
    gui.config.input_bg = "#2a2a2a"
    gui.config.input_fg = "#eeeeee"
    return ContextMeterWidget(gui)


# ── Initialisation ────────────────────────────────────────────────────────────


@pytest.mark.unit
def test_initial_state_has_no_canvas() -> None:
    """GIVEN a new ContextMeterWidget
    WHEN no create() call has been made
    THEN _canvas is None and cached state is empty.
    """
    widget = _make_widget()
    assert widget._canvas is None
    assert widget._cached_max_tokens == 4096
    assert widget._cached_breakdown == {}


@pytest.mark.unit
def test_create_builds_canvas_and_places_it() -> None:
    """GIVEN a ContextMeterWidget
    WHEN create(parent) is called
    THEN a tk.Canvas is created at the correct place geometry.
    """
    widget = _make_widget()
    parent = MagicMock()
    canvas_instance = MagicMock()
    canvas_instance.winfo_width.return_value = 56
    canvas_instance.winfo_height.return_value = 60

    with patch("agentx.gui.context_meter_widget.tk.Canvas", return_value=canvas_instance) as mock_canvas:
        widget.create(parent)

    mock_canvas.assert_called_once()
    canvas_instance.place.assert_called_once_with(relx=0.92, rely=0.00, relwidth=0.07, relheight=0.24)
    assert widget._canvas is canvas_instance


# ── update() scheduling ───────────────────────────────────────────────────────


@pytest.mark.unit
def test_update_caches_values() -> None:
    """GIVEN a widget with a canvas
    WHEN update() is called
    THEN max_tokens and breakdown are cached before scheduling.
    """
    widget = _make_widget()
    widget._canvas = MagicMock()
    breakdown = {"user": 100, "assistant": 200}

    widget.update(8192, breakdown)

    assert widget._cached_max_tokens == 8192
    assert widget._cached_breakdown == breakdown


@pytest.mark.unit
def test_update_schedules_render_via_after() -> None:
    """GIVEN a widget with a canvas
    WHEN update() is called
    THEN canvas.after(0, ...) is invoked once (ARCH-06 thread safety).
    """
    widget = _make_widget()
    canvas_mock = MagicMock()
    widget._canvas = canvas_mock
    widget.update(4096, {"user": 10})
    canvas_mock.after.assert_called_once()
    args = canvas_mock.after.call_args[0]
    assert args[0] == 0  # delay must be 0


@pytest.mark.unit
def test_update_noop_when_canvas_is_none() -> None:
    """GIVEN a widget with no canvas
    WHEN update() is called
    THEN no exception is raised and values are still cached.
    """
    widget = _make_widget()
    # No exception should propagate
    widget.update(8192, {"user": 50})
    assert widget._cached_max_tokens == 8192


# ── _render() arc geometry ────────────────────────────────────────────────────


@pytest.mark.unit
@pytest.mark.parametrize(
    "band_key,label",
    [(bk, bl) for bk, bl, _ in _BANDS],
    ids=[bk for bk, _, _ in _BANDS],
)
def test_render_draws_arc_for_each_nonzero_band(band_key: str, label: str) -> None:
    """GIVEN a breakdown with one non-zero band
    WHEN _render() is called
    THEN create_arc is called with style=PIESLICE and the correct fill color.

    Parameterized over all seven band categories (BAND-01 through BAND-07).
    """
    import tkinter as tk

    widget = _make_widget()
    canvas = MagicMock()
    canvas.winfo_width.return_value = 60
    canvas.winfo_height.return_value = 60
    widget._canvas = canvas

    expected_color = next(c for bk, _, c in _BANDS if bk == band_key)
    breakdown = {band_key: 1000}
    widget._render(max_tokens=4096, breakdown=breakdown)

    arc_calls = [c for c in canvas.create_arc.call_args_list]
    # At least one call with the expected fill color and PIESLICE style
    matching = [c for c in arc_calls if c.kwargs.get("fill") == expected_color and c.kwargs.get("style") == tk.PIESLICE]
    assert matching, f"No arc drawn for band {band_key!r} with color {expected_color!r}"


@pytest.mark.unit
def test_render_draws_ghost_arc_for_remaining_capacity() -> None:
    """GIVEN a breakdown using 50% of the context window
    WHEN _render() is called
    THEN a ghost arc with _GHOST_COLOR is drawn for the remaining 50%.
    """
    import tkinter as tk

    widget = _make_widget()
    canvas = MagicMock()
    canvas.winfo_width.return_value = 60
    canvas.winfo_height.return_value = 60
    widget._canvas = canvas

    breakdown = {"user": 2048}  # 50% of 4096
    widget._render(max_tokens=4096, breakdown=breakdown)

    ghost_calls = [
        c
        for c in canvas.create_arc.call_args_list
        if c.kwargs.get("fill") == _GHOST_COLOR and c.kwargs.get("style") == tk.PIESLICE
    ]
    assert ghost_calls, "No ghost arc drawn for remaining capacity"


@pytest.mark.unit
def test_render_no_ghost_arc_when_full() -> None:
    """GIVEN a breakdown exactly filling the context window (100%)
    WHEN _render() is called
    THEN no ghost arc is drawn (remaining fraction ~ 0).
    """
    widget = _make_widget()
    canvas = MagicMock()
    canvas.winfo_width.return_value = 60
    canvas.winfo_height.return_value = 60
    widget._canvas = canvas

    breakdown = {"user": 4096}  # 100%
    widget._render(max_tokens=4096, breakdown=breakdown)

    ghost_calls = [c for c in canvas.create_arc.call_args_list if c.kwargs.get("fill") == _GHOST_COLOR]
    assert not ghost_calls, "Ghost arc should not be drawn when context is full"


@pytest.mark.unit
def test_render_empty_breakdown_draws_only_ghost() -> None:
    """GIVEN an empty breakdown (no tokens consumed)
    WHEN _render() is called
    THEN only the ghost arc (100% remaining) is drawn; no band arcs.
    """
    import tkinter as tk

    widget = _make_widget()
    canvas = MagicMock()
    canvas.winfo_width.return_value = 60
    canvas.winfo_height.return_value = 60
    widget._canvas = canvas

    widget._render(max_tokens=4096, breakdown={})

    arc_calls = canvas.create_arc.call_args_list
    band_colors = {c for _, _, c in _BANDS}
    band_arcs = [c for c in arc_calls if c.kwargs.get("fill") in band_colors]
    ghost_arcs = [c for c in arc_calls if c.kwargs.get("fill") == _GHOST_COLOR]

    assert not band_arcs, "No band arcs should be drawn for empty breakdown"
    assert ghost_arcs, "Ghost arc should fill entire ring for empty breakdown"


# ── _render() border risk states ─────────────────────────────────────────────


@pytest.mark.unit
@pytest.mark.parametrize(
    "pct,expected_color,expected_width,state_name",
    [
        (0.0, _BORDER_DEFAULT[0], _BORDER_DEFAULT[1], "default"),
        (0.50, _BORDER_DEFAULT[0], _BORDER_DEFAULT[1], "default-mid"),
        (0.79, _BORDER_DEFAULT[0], _BORDER_DEFAULT[1], "just-below-warning"),
        (0.80, _BORDER_WARNING[0], _BORDER_WARNING[1], "warning-threshold"),
        (0.95, _BORDER_WARNING[0], _BORDER_WARNING[1], "warning-mid"),
        (0.99, _BORDER_WARNING[0], _BORDER_WARNING[1], "just-below-critical"),
        (1.00, _BORDER_CRITICAL[0], _BORDER_CRITICAL[1], "critical-threshold"),
        (1.20, _BORDER_CRITICAL[0], _BORDER_CRITICAL[1], "over-100"),
    ],
)
def test_border_color_at_usage_percentages(
    pct: float,
    expected_color: str,
    expected_width: int,
    state_name: str,
) -> None:
    """GIVEN a context window used to a specific percentage
    WHEN _render() is called
    THEN the outer ring oval is drawn with the matching risk-state color and width.

    Tests all three ENH-16 risk state boundaries:
    - Default (< 80%): gray, 1px
    - Warning (>= 80% and < 100%): thin red, 1px
    - Critical (>= 100%): bright red, 3px
    """
    widget = _make_widget()
    canvas = MagicMock()
    canvas.winfo_width.return_value = 60
    canvas.winfo_height.return_value = 60
    widget._canvas = canvas

    tokens = int(pct * 100)
    widget._render(max_tokens=100, breakdown={"user": tokens})

    oval_calls = canvas.create_oval.call_args_list
    # The border oval is the one that has outline= and no fill
    border_calls = [c for c in oval_calls if c.kwargs.get("fill") == "" and c.kwargs.get("outline") == expected_color]
    assert border_calls, (
        f"State {state_name!r}: expected border color {expected_color!r} at "
        f"{pct*100:.0f}% usage but found: "
        f"{[c.kwargs.get('outline') for c in oval_calls]}"
    )
    assert border_calls[0].kwargs.get("width") == expected_width


# ── Center label ──────────────────────────────────────────────────────────────


@pytest.mark.unit
@pytest.mark.parametrize(
    "pct,expected_color,state_name",
    [
        (0.50, "#eeeeee", "normal"),  # input_fg
        (0.80, "#CC4444", "warning"),
        (1.00, "#FF0000", "critical"),
        (1.20, "#FF0000", "over-critical"),
    ],
)
def test_center_label_color_reflects_risk(
    pct: float,
    expected_color: str,
    state_name: str,
) -> None:
    """GIVEN a context usage fraction
    WHEN _render() is called
    THEN the center percentage label is drawn in the matching risk color.

    Normal state uses the theme's input_fg; warning/critical use their
    respective red shades for non-color cue redundancy (REQ-11).
    """
    widget = _make_widget()
    canvas = MagicMock()
    canvas.winfo_width.return_value = 60
    canvas.winfo_height.return_value = 60
    widget._canvas = canvas

    tokens = int(pct * 100)
    widget._render(max_tokens=100, breakdown={"user": tokens})

    text_calls = [c for c in canvas.create_text.call_args_list if c.kwargs.get("tags") == ("pct_label",)]
    assert text_calls, "No center pct_label text was drawn"
    assert text_calls[0].kwargs["fill"] == expected_color, (
        f"State {state_name!r}: expected color {expected_color!r}, " f"got {text_calls[0].kwargs['fill']!r}"
    )


@pytest.mark.unit
def test_center_label_text_format() -> None:
    """GIVEN 40% context usage
    WHEN _render() is called
    THEN the center label shows '40%'.
    """
    widget = _make_widget()
    canvas = MagicMock()
    canvas.winfo_width.return_value = 60
    canvas.winfo_height.return_value = 60
    widget._canvas = canvas

    widget._render(max_tokens=100, breakdown={"user": 40})

    text_calls = [c for c in canvas.create_text.call_args_list if c.kwargs.get("tags") == ("pct_label",)]
    assert text_calls[0].kwargs["text"] == "40%"


@pytest.mark.unit
def test_center_label_capped_at_999_percent() -> None:
    """GIVEN an absurdly large token count
    WHEN _render() is called
    THEN the center label is capped at '999%' to prevent display overflow.
    """
    widget = _make_widget()
    canvas = MagicMock()
    canvas.winfo_width.return_value = 60
    canvas.winfo_height.return_value = 60
    widget._canvas = canvas

    widget._render(max_tokens=4096, breakdown={"user": 99999})

    text_calls = [c for c in canvas.create_text.call_args_list if c.kwargs.get("tags") == ("pct_label",)]
    assert text_calls[0].kwargs["text"] == "999%"


# ── Degenerate inputs ─────────────────────────────────────────────────────────


@pytest.mark.unit
def test_render_does_not_crash_when_max_tokens_is_zero() -> None:
    """GIVEN max_tokens=0 (uninitialised state)
    WHEN _render() is called
    THEN no exception is raised and canvas.delete is still called.
    """
    widget = _make_widget()
    canvas = MagicMock()
    canvas.winfo_width.return_value = 60
    canvas.winfo_height.return_value = 60
    widget._canvas = canvas

    widget._render(max_tokens=0, breakdown={"user": 100})
    canvas.delete.assert_called_once_with("all")


@pytest.mark.unit
def test_render_schedules_retry_when_canvas_has_no_size() -> None:
    """GIVEN a canvas that reports width=0 (not yet laid out)
    WHEN _render() is called
    THEN canvas.after(50, ...) is scheduled for retry and nothing else is drawn.
    """
    widget = _make_widget()
    canvas = MagicMock()
    canvas.winfo_width.return_value = 0
    canvas.winfo_height.return_value = 0
    widget._canvas = canvas

    widget._render(max_tokens=4096, breakdown={"user": 100})

    canvas.after.assert_called_once()
    assert canvas.after.call_args[0][0] == 50
    canvas.create_arc.assert_not_called()


# ── Tooltip text content ──────────────────────────────────────────────────────


@pytest.mark.unit
@pytest.mark.parametrize(
    "band_key,tokens,max_tokens,expected_substr",
    [
        ("user", 1024, 4096, "User Prompts"),
        ("assistant", 800, 4000, "Agent Response"),
        ("working_memory", 200, 1000, "Working Memory"),
        ("tool", 300, 3000, "Tool Calls / Results"),
        ("thinking", 50, 500, "10.0%"),
    ],
    ids=["user-label", "assistant-label", "wm-label", "tool-label", "thinking-pct"],
)
def test_band_tooltip_contains_expected_text(
    band_key: str,
    tokens: int,
    max_tokens: int,
    expected_substr: str,
) -> None:
    """GIVEN a breakdown with a single non-zero band
    WHEN _bind_tooltips() is called
    THEN the tooltip text for that band contains the expected label / percentage.

    Parameterized to verify each band label and percentage formatting.
    """
    widget = _make_widget()
    canvas = MagicMock()
    widget._canvas = canvas

    breakdown = {band_key: tokens}
    total = tokens
    pct = total / max_tokens

    captured_texts: list[str] = []

    def fake_tag_bind(tag, event, handler):
        if event == "<Enter>" and tag == f"band_{band_key}":
            fake_ev = MagicMock()
            fake_ev.x_root = 0
            fake_ev.y_root = 0
            # Capture the tooltip text by intercepting _show_tooltip
            widget._show_tooltip = lambda e, t: captured_texts.append(t)
            handler(fake_ev)

    canvas.tag_bind.side_effect = fake_tag_bind

    widget._bind_tooltips(breakdown, max_tokens, pct, total)

    assert captured_texts, f"No tooltip text captured for band {band_key!r}"
    assert (
        expected_substr in captured_texts[0]
    ), f"Expected {expected_substr!r} in tooltip for {band_key!r}; got: {captured_texts[0]!r}"


@pytest.mark.unit
def test_ghost_arc_tooltip_shows_remaining_tokens() -> None:
    """GIVEN a 50% full context window
    WHEN _bind_tooltips() is called
    THEN the ghost arc tooltip contains 'Remaining capacity' and the free token count.
    """
    widget = _make_widget()
    canvas = MagicMock()
    widget._canvas = canvas

    breakdown = {"user": 2048}
    max_tokens = 4096
    total = 2048
    pct = 0.5

    captured: list[str] = []

    def fake_tag_bind(tag, event, handler):
        if event == "<Enter>" and tag == "ghost":
            widget._show_tooltip = lambda e, t: captured.append(t)
            handler(MagicMock(x_root=0, y_root=0))

    canvas.tag_bind.side_effect = fake_tag_bind
    widget._bind_tooltips(breakdown, max_tokens, pct, total)

    assert captured
    assert "Remaining capacity" in captured[0]
    assert "2,048" in captured[0]


# ── _show_tooltip / _hide_tooltip ─────────────────────────────────────────────


@pytest.mark.unit
def test_hide_tooltip_suppresses_tkerror() -> None:
    """GIVEN a tooltip Toplevel that raises TclError on destroy
    WHEN _hide_tooltip() is called
    THEN the TclError is silently swallowed and _tooltip is set to None.
    """
    import tkinter as tk

    widget = _make_widget()
    bad_toplevel = MagicMock()
    bad_toplevel.destroy.side_effect = tk.TclError("already destroyed")
    widget._tooltip = bad_toplevel

    widget._hide_tooltip()

    assert widget._tooltip is None


@pytest.mark.unit
def test_hide_tooltip_noop_when_no_tooltip() -> None:
    """GIVEN no active tooltip
    WHEN _hide_tooltip() is called
    THEN no exception is raised.
    """
    widget = _make_widget()
    widget._tooltip = None
    widget._hide_tooltip()  # must not raise
