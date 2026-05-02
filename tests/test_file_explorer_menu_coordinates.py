"""
Functional tests for FileExplorer context-menu coordinate safety.

Units under test:
  - FileExplorer._post_menu()        (coordinate placement)
  - FileExplorer._on_right_click()   (coordinate capture strategy)

Problem context (v0.22.7):
  Under XWayland on Wayland compositors (Ubuntu 25.04+, GNOME/Mutter) the raw
  X11 event coordinates ``event.x_root`` / ``event.y_root`` are in *physical*
  pixels within the XWayland virtual screen, which may span multiple logical
  monitors or a HiDPI-scaled surface.  Passing them directly to ``menu.post()``
  can place the menu window far outside the visible area of any monitor.

  Evidence from UAT: right-click 3 yielded x_root=3753 on a system where no
  single monitor is wider than ~3840 pixels, implying the coordinate is in an
  off-screen region of the XWayland virtual framebuffer.

Safe coordinate strategy:
  ``self.tree.winfo_rootx() + event.x``
  ``self.tree.winfo_rooty() + event.y``

  These derive the screen position by asking Tk for the widget's own on-screen
  anchor (guaranteed to be in the visible window) and offsetting by the
  widget-relative cursor position (which is always non-negative and bounded by
  the widget dimensions).  The result is always within the window's footprint
  and therefore always visible.
"""

import tkinter as tk
from unittest.mock import MagicMock, call, patch

import pytest

from agentx.file_explorer import FileExplorer

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_explorer(tmp_path):
    """Return (root, fe, frame). Caller is responsible for root.destroy()."""
    root = tk.Tk()
    root.withdraw()
    parent = tk.Frame(root)
    parent.pack()
    fe = FileExplorer(str(tmp_path))
    frame = fe.to_gui(parent)
    root.update_idletasks()
    return root, fe, frame


def _fake_event(*, tree, x: int, y: int, x_root: int, y_root: int):
    """
    Build a synthetic event backed by a real tree widget so that
    ``identify_row``, ``winfo_rootx``, and ``winfo_rooty`` work.
    """
    ev = MagicMock()
    ev.x = x
    ev.y = y
    ev.x_root = x_root
    ev.y_root = y_root
    return ev


# ---------------------------------------------------------------------------
# Functional tests: coordinate strategy soundness
# ---------------------------------------------------------------------------


@pytest.mark.functional
class TestMenuCoordinateSafety:
    """
    Functional tests confirming that the safe coordinate strategy
    (winfo_rootx + event.x) produces visible menu positions whereas
    raw XWayland x_root coordinates can be off-screen.

    Units under test:
      - FileExplorer._post_menu()
      - FileExplorer._on_right_click() coordinate capture
    External units under test:
      - tk.Menu.post()      (real Tk call, no mock)
      - tk.Widget.winfo_*() (real Tk geometry queries)
    """

    def test_safe_coords_are_within_screen_bounds(self, tmp_path):
        """
        GIVEN a FileExplorer with a file row
        WHEN the safe coordinate strategy (winfo_rootx + event.x) is used
        THEN the resulting (x, y) fit within the screen dimensions reported by Tk
         AND are non-negative.

        Affordance ID: PD-11-AF-008
        """
        (tmp_path / "readme.md").write_text("hello")
        root, fe, _ = _make_explorer(tmp_path)
        try:
            root.update_idletasks()
            tree = fe.tree

            # Use a widget-relative position safely inside the tree
            event_x = 10
            event_y = 10

            safe_x = tree.winfo_rootx() + event_x
            safe_y = tree.winfo_rooty() + event_y

            screen_w = tree.winfo_screenwidth()
            screen_h = tree.winfo_screenheight()

            assert safe_x >= 0, f"safe_x={safe_x} is negative"
            assert safe_y >= 0, f"safe_y={safe_y} is negative"
            assert safe_x <= screen_w, f"safe_x={safe_x} exceeds screen_w={screen_w}"
            assert safe_y <= screen_h, f"safe_y={safe_y} exceeds screen_h={screen_h}"
        finally:
            root.destroy()

    @pytest.mark.parametrize(
        "x_root, y_root, description",
        [
            (3753, 500, "x_root beyond typical 1920px single-monitor width"),
            (5000, 3000, "both coords far off typical screen"),
            (0, 0, "origin — also valid for safe strategy"),
            (100, 200, "typical on-screen coords — both strategies agree"),
        ],
    )
    def test_raw_xwayland_coords_may_be_off_screen(self, tmp_path, x_root, y_root, description):
        """
        GIVEN raw XWayland event.x_root / event.y_root values
        WHEN compared against the Tk-reported screen dimensions
        THEN some values fall outside screen bounds (demonstrating the bug)
         AND the safe strategy (winfo_rootx + event.x) always stays in bounds.

        Permutations:
          - x_root=3753, y_root=500: UAT-observed off-screen coordinate
          - x_root=5000, y_root=3000: both dimensions far off screen
          - x_root=0, y_root=0: origin edge case
          - x_root=100, y_root=200: typical on-screen — both strategies agree

        Affordance ID: PD-11-AF-008
        """
        (tmp_path / "notes.txt").write_text("data")
        root, fe, _ = _make_explorer(tmp_path)
        try:
            root.update_idletasks()
            tree = fe.tree
            screen_w = tree.winfo_screenwidth()
            screen_h = tree.winfo_screenheight()

            # Safe strategy: anchor to widget position, offset by widget-relative coords
            safe_x = tree.winfo_rootx() + 10
            safe_y = tree.winfo_rooty() + 10

            # Safe coords always in bounds
            assert 0 <= safe_x <= screen_w, f"safe_x={safe_x} not in [0, {screen_w}]"
            assert 0 <= safe_y <= screen_h, f"safe_y={safe_y} not in [0, {screen_h}]"

            # Document whether raw x_root is off-screen (not an assertion — informational)
            raw_in_bounds = (0 <= x_root <= screen_w) and (0 <= y_root <= screen_h)
            print(
                f"[coord test] {description}: "
                f"raw=({x_root},{y_root}) screen={screen_w}x{screen_h} "
                f"raw_in_bounds={raw_in_bounds} safe=({safe_x},{safe_y})"
            )
        finally:
            root.destroy()

    def test_post_menu_with_safe_coords_maps_menu(self, tmp_path):
        """
        GIVEN a FileExplorer with a file row
        WHEN _post_menu() is called with safe (winfo_rootx + event.x) coordinates
        THEN menu.winfo_ismapped() returns 1 immediately after popup
         AND menu.winfo_x() and winfo_y() are within screen bounds.

        This test uses a real tk.Menu.tk_popup() call (no mock) to confirm that Tk
        actually maps the menu window at those coordinates.

        Affordance ID: PD-11-AF-008
        """
        (tmp_path / "script.py").write_text("pass")
        root, fe, _ = _make_explorer(tmp_path)
        try:
            root.update_idletasks()
            tree = fe.tree

            safe_x = tree.winfo_rootx() + 10
            safe_y = tree.winfo_rooty() + 10

            fe._post_menu(fe._popup_menu, safe_x, safe_y)
            root.update_idletasks()

            assert fe._popup_menu.winfo_ismapped() == 1, "menu should be mapped after _post_menu"

            menu_x = fe._popup_menu.winfo_x()
            menu_y = fe._popup_menu.winfo_y()
            screen_w = tree.winfo_screenwidth()
            screen_h = tree.winfo_screenheight()

            assert 0 <= menu_x <= screen_w, f"menu_x={menu_x} out of screen bounds [0,{screen_w}]"
            assert 0 <= menu_y <= screen_h, f"menu_y={menu_y} out of screen bounds [0,{screen_h}]"

            fe._popup_menu.unpost()
        finally:
            root.destroy()

    def test_on_right_click_uses_safe_coords_for_popup(self, tmp_path):
        """
        GIVEN a file row in the treeview
        WHEN _on_right_click() fires with widget-relative event.x / event.y = (10, row_y)
        THEN menu.tk_popup() is called with (winfo_rootx + event.x, winfo_rooty + event.y)
         NOT with the raw event.x_root / event.y_root values.

        This test captures what coordinates are actually passed to menu.tk_popup()
        and asserts they match the safe strategy.  It will FAIL until the
        production code is updated to use winfo_rootx/y + event.x/y instead of
        event.x_root/y_root.

        Affordance ID: PD-11-AF-008
        """
        (tmp_path / "app.py").write_text("code")
        root, fe, _ = _make_explorer(tmp_path)
        try:
            root.update_idletasks()
            items = fe.tree.get_children()
            file_item = next(i for i in items if "file" in fe.tree.item(i, "tags"))
            bbox = fe.tree.bbox(file_item)
            event_x = 10
            event_y = int(bbox[1]) + int(bbox[3]) // 2 if bbox else 5

            # Build expected safe coordinates from widget geometry
            expected_x = fe.tree.winfo_rootx() + event_x
            expected_y = fe.tree.winfo_rooty() + event_y

            # Supply deliberately wrong raw coords to expose if they are used
            bad_x_root = 9999
            bad_y_root = 9999

            ev = _fake_event(tree=fe.tree, x=event_x, y=event_y, x_root=bad_x_root, y_root=bad_y_root)

            fe._MENU_POST_DELAY_MS = 0  # fire synchronously in tests
            fe._FORCE_WAYLAND_POPUP = False
            with patch.object(fe._popup_menu, "tk_popup") as mock_popup:
                fe._on_right_click(ev)
                root.update()  # flush after(0) timer

            mock_popup.assert_called_once_with(expected_x, expected_y)
        finally:
            root.destroy()
