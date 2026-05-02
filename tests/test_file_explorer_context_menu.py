"""
Hermetic unit tests for FileExplorer context-menu affordances.

PD-11-AF-008 — Right-click on a file shows the file context menu
PD-11-AF-009 — Right-click on a directory shows the folder context menu
PD-11-AF-010 — Escape dismisses the context menu

Units under test:
  - FileExplorer._on_right_click()   (PD-11-AF-008, PD-11-AF-009)
  - FileExplorer._dismiss_popup_menu() (PD-11-AF-010)
  - FileExplorer._on_attach_selected()
  - FileExplorer._on_edit_selected()
  - FileExplorer._on_add_full_path_selected()
  - FileExplorer._on_add_relative_path_selected()

Bug history:
  v0.22.1: <FocusOut> binding removed — tk_popup() stole focus, immediately unposting.
  v0.22.2: Changed to <ButtonRelease-3> — avoided tk_popup() grab consuming the release.
  v0.22.5: Replaced tk_popup() with menu.post() (no grab) — but kept ButtonRelease binding.
  v0.22.6: Root cause found — Menu class <ButtonRelease> binding fires on the just-posted
           menu window (cursor is over it at x_root,y_root), calls unpost() before any
           item is active.  Fix: bind <Button-3> press; with no grab the release goes to
           the treeview (ignored) or the menu (item invoked), never causing unpost().
"""

import tkinter as tk
from unittest.mock import MagicMock, patch

import pytest

from agentx.file_explorer import FileExplorer

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_explorer(tmp_path, *, on_attach=None, on_edit=None, on_add_folder_to_memory=None):
    """Return a (root, FileExplorer, frame) triple with the widget built."""
    root = tk.Tk()
    root.withdraw()
    parent = tk.Frame(root)
    parent.pack()
    fe = FileExplorer(str(tmp_path))
    frame = fe.to_gui(
        parent,
        on_attach=on_attach,
        on_edit=on_edit,
        on_add_folder_to_memory=on_add_folder_to_memory,
    )
    return root, fe, frame


def _fake_event(*, x: int = 0, y: int = 0, x_root: int = 100, y_root: int = 200):
    """Create a minimal synthetic Tkinter event."""
    ev = MagicMock()
    ev.x = x
    ev.y = y
    ev.x_root = x_root
    ev.y_root = y_root
    return ev


# ---------------------------------------------------------------------------
# PD-11-AF-008: right-click on a file shows the file context menu
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestFileContextMenu:
    """
    Unit tests for PD-11-AF-008.

    Affordance ID: PD-11-AF-008
    Source: FileExplorer._on_right_click()
    """

    def test_right_click_file_calls_popup_on_file_menu(self, tmp_path):
        """
        GIVEN the tree has a file row
        WHEN the user right-clicks that row AND idle callbacks are flushed
        THEN menu.tk_popup() is called to display the file context menu
         AND the handler returns 'break' to stop event propagation
         AND the folder context menu is NOT posted.

        NOTE: _on_right_click() defers menu popup via after(_MENU_POST_DELAY_MS).
        In production _MENU_POST_DELAY_MS=100 ms ensures the button release fires
        on the treeview (not the menu) before the menu posts.  In tests the delay
        is set to 0 and root.update() is called to flush the timer callback.

        Unit tests can only verify the correct Tk method is called; whether the menu
        stays visible under a live compositor requires manual UAT.
        Affordance ID: PD-11-AF-008
        """
        (tmp_path / "hello.py").write_text("x")
        root, fe, _ = _make_explorer(tmp_path)
        try:
            items = fe.tree.get_children()
            file_item = next(i for i in items if "file" in fe.tree.item(i, "tags"))
            bbox = fe.tree.bbox(file_item)
            y = int(bbox[1]) + int(bbox[3]) // 2 if bbox else 5
            ev = _fake_event(y=y)

            fe._MENU_POST_DELAY_MS = 0  # fire synchronously in tests
            fe._FORCE_WAYLAND_POPUP = False
            with (
                patch.object(fe._popup_menu, "tk_popup") as mock_file_tk_popup,
                patch.object(fe._folder_popup_menu, "tk_popup") as mock_folder_tk_popup,
                patch.object(fe._popup_menu, "grab_release") as mock_file_grab_release,
            ):
                result = fe._on_right_click(ev)
                # Flush after(0) timer — this is when _post_menu() fires.
                root.update()

            mock_file_tk_popup.assert_called_once()
            mock_folder_tk_popup.assert_not_called()
            mock_file_grab_release.assert_called_once()
            assert result == "break"
        finally:
            root.destroy()

    def test_right_click_on_empty_area_does_nothing(self, tmp_path):
        """
        GIVEN the tree is empty
        WHEN right-click fires on an empty row area
        THEN no menu is posted.
        """
        root, fe, _ = _make_explorer(tmp_path)
        try:
            ev = _fake_event(y=9999)  # beyond any row
            with (
                patch.object(fe._popup_menu, "tk_popup") as mock_file_popup,
                patch.object(fe._folder_popup_menu, "tk_popup") as mock_folder_popup,
            ):
                fe._on_right_click(ev)
            mock_file_popup.assert_not_called()
            mock_folder_popup.assert_not_called()
        finally:
            root.destroy()

    def test_right_click_bound_to_button_press_not_release(self, tmp_path):
        """
        GIVEN the widget has been created
        WHEN the tree's bindings are inspected
        THEN <Button-3> (press) is bound and <ButtonRelease-3> is NOT bound.

        With delayed popup posting, <Button-3> press is the correct trigger.
        The subsequent <ButtonRelease-3> is NOT captured by any grab — it goes to
        whichever window the cursor is over at release time (menu → invoke ✓,
        treeview → ignored ✓).  Binding to the release caused a race where the
        Menu class's generic <ButtonRelease> class binding fired on the just-posted
        menu before any item was active, calling unpost() immediately.
        """
        root, fe, _ = _make_explorer(tmp_path)
        try:
            bound_events = fe.tree.bind()
            assert "<Button-3>" in bound_events
            assert "<ButtonRelease-3>" not in bound_events
        finally:
            root.destroy()

    def test_focusout_binding_not_present(self, tmp_path):
        """
        GIVEN the widget has been created
        WHEN the tree's bindings are inspected
        THEN <FocusOut> is NOT bound (would dismiss menu immediately on focus steal).
        """
        root, fe, _ = _make_explorer(tmp_path)
        try:
            bound_events = fe.tree.bind()
            assert "<FocusOut>" not in bound_events
        finally:
            root.destroy()

    def test_attach_callback_invoked_with_correct_path(self, tmp_path):
        """
        GIVEN a file row is selected
        WHEN _on_attach_selected is called
        THEN the on_attach callback receives the full path of the file.

        Affordance ID: PD-11-AF-008
        """
        (tmp_path / "readme.md").write_text("docs")
        attach_cb = MagicMock()
        root, fe, _ = _make_explorer(tmp_path, on_attach=attach_cb)
        try:
            items = fe.tree.get_children()
            file_item = next(i for i in items if "file" in fe.tree.item(i, "tags"))
            fe.tree.selection_set(file_item)
            fe._on_attach_selected()
            attach_cb.assert_called_once()
            called_path = attach_cb.call_args[0][0]
            assert called_path.endswith("readme.md")
        finally:
            root.destroy()

    def test_edit_callback_invoked_with_correct_path(self, tmp_path):
        """
        GIVEN a file row is selected
        WHEN _on_edit_selected is called
        THEN the on_edit callback receives the full path of the file.

        Affordance ID: PD-11-AF-008
        """
        (tmp_path / "main.py").write_text("code")
        edit_cb = MagicMock()
        root, fe, _ = _make_explorer(tmp_path, on_edit=edit_cb)
        try:
            items = fe.tree.get_children()
            file_item = next(i for i in items if "file" in fe.tree.item(i, "tags"))
            fe.tree.selection_set(file_item)
            fe._on_edit_selected()
            edit_cb.assert_called_once()
            called_path = edit_cb.call_args[0][0]
            assert called_path.endswith("main.py")
        finally:
            root.destroy()

    def test_attach_without_callback_does_not_raise(self, tmp_path):
        """
        GIVEN no on_attach callback was provided
        WHEN _on_attach_selected is called
        THEN no exception is raised.
        """
        (tmp_path / "x.txt").write_text("x")
        root, fe, _ = _make_explorer(tmp_path, on_attach=None)
        try:
            items = fe.tree.get_children()
            file_item = next((i for i in items if "file" in fe.tree.item(i, "tags")), None)
            if file_item:
                fe.tree.selection_set(file_item)
            fe._on_attach_selected()  # must not raise
        finally:
            root.destroy()


# ---------------------------------------------------------------------------
# PD-11-AF-009: right-click on a directory shows the folder context menu
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestFolderContextMenu:
    """
    Unit tests for PD-11-AF-009.

    Affordance ID: PD-11-AF-009
    Source: FileExplorer._on_right_click()
    """

    def test_right_click_directory_calls_popup_on_folder_menu(self, tmp_path):
        """
        GIVEN the tree has a directory row
        WHEN the user right-clicks that row AND idle callbacks are flushed
        THEN menu.tk_popup() is called on the folder menu
         AND the handler returns 'break' to stop event propagation
         AND the file context menu is NOT posted.

        See test_right_click_file_calls_post_on_file_menu for the rationale for
        deferring menu popup via after(_MENU_POST_DELAY_MS).
        Affordance ID: PD-11-AF-009
        """
        (tmp_path / "subdir").mkdir()
        root, fe, _ = _make_explorer(tmp_path)
        try:
            items = fe.tree.get_children()
            dir_item = next(i for i in items if "directory" in fe.tree.item(i, "tags"))
            bbox = fe.tree.bbox(dir_item)
            y = int(bbox[1]) + int(bbox[3]) // 2 if bbox else 5
            ev = _fake_event(y=y)

            fe._MENU_POST_DELAY_MS = 0  # fire synchronously in tests
            fe._FORCE_WAYLAND_POPUP = False
            with (
                patch.object(fe._folder_popup_menu, "tk_popup") as mock_folder_tk_popup,
                patch.object(fe._popup_menu, "tk_popup") as mock_file_tk_popup,
                patch.object(fe._folder_popup_menu, "grab_release") as mock_folder_grab_release,
            ):
                result = fe._on_right_click(ev)
                # Flush after(0) timer — this is when _post_menu() fires.
                root.update()

            mock_folder_tk_popup.assert_called_once()
            mock_file_tk_popup.assert_not_called()
            mock_folder_grab_release.assert_called_once()
            assert result == "break"
        finally:
            root.destroy()

    def test_add_full_path_callback_invoked(self, tmp_path):
        """
        GIVEN a directory row is selected
        WHEN _on_add_full_path_selected is called
        THEN the on_add_folder_to_memory callback receives (folder_name, full_abs_path).

        Affordance ID: PD-11-AF-009
        """
        sub = tmp_path / "mylib"
        sub.mkdir()
        mem_cb = MagicMock()
        root, fe, _ = _make_explorer(tmp_path, on_add_folder_to_memory=mem_cb)
        try:
            items = fe.tree.get_children()
            dir_item = next(i for i in items if "directory" in fe.tree.item(i, "tags"))
            fe.tree.selection_set(dir_item)
            fe._on_add_full_path_selected()
            mem_cb.assert_called_once()
            key, value = mem_cb.call_args[0]
            assert key == "mylib"
            assert value == str(sub)
        finally:
            root.destroy()

    def test_add_relative_path_callback_invoked(self, tmp_path):
        """
        GIVEN a directory row is selected
        WHEN _on_add_relative_path_selected is called
        THEN the on_add_folder_to_memory callback receives (folder_name, relative_path).

        Affordance ID: PD-11-AF-009
        """
        sub = tmp_path / "utils"
        sub.mkdir()
        mem_cb = MagicMock()
        root, fe, _ = _make_explorer(tmp_path, on_add_folder_to_memory=mem_cb)
        try:
            items = fe.tree.get_children()
            dir_item = next(i for i in items if "directory" in fe.tree.item(i, "tags"))
            fe.tree.selection_set(dir_item)
            fe._on_add_relative_path_selected()
            mem_cb.assert_called_once()
            key, value = mem_cb.call_args[0]
            assert key == "utils"
            # Relative to root_path (tmp_path), this is just "utils"
            assert value == "utils"
        finally:
            root.destroy()

    def test_folder_callback_without_registration_does_not_raise(self, tmp_path):
        """
        GIVEN no on_add_folder_to_memory callback was provided
        WHEN _on_add_full_path_selected or _on_add_relative_path_selected is called
        THEN no exception is raised.
        """
        (tmp_path / "empty_dir").mkdir()
        root, fe, _ = _make_explorer(tmp_path, on_add_folder_to_memory=None)
        try:
            items = fe.tree.get_children()
            dir_item = next((i for i in items if "directory" in fe.tree.item(i, "tags")), None)
            if dir_item:
                fe.tree.selection_set(dir_item)
            fe._on_add_full_path_selected()
            fe._on_add_relative_path_selected()
        finally:
            root.destroy()


# ---------------------------------------------------------------------------
# PD-11-AF-010: Escape dismisses the context menu
# ---------------------------------------------------------------------------


@pytest.mark.unit
class TestDismissContextMenu:
    """
    Unit tests for PD-11-AF-010.

    Affordance ID: PD-11-AF-010
    Source: FileExplorer._dismiss_popup_menu()
    """

    def test_escape_binding_present_on_tree(self, tmp_path):
        """
        GIVEN the widget has been created
        WHEN the tree's bindings are inspected
        THEN <Escape> (normalised by Tkinter to <Key-Escape>) is bound.
        """
        root, fe, _ = _make_explorer(tmp_path)
        try:
            bound_events = fe.tree.bind()
            # Tkinter normalises "<Escape>" → "<Key-Escape>"
            assert "<Key-Escape>" in bound_events
        finally:
            root.destroy()

    def test_dismiss_calls_unpost_on_both_menus(self, tmp_path):
        """
        GIVEN both menus exist
        WHEN _dismiss_popup_menu() is called
        THEN unpost() is called on both the file menu and the folder menu.

        Affordance ID: PD-11-AF-010
        """
        root, fe, _ = _make_explorer(tmp_path)
        try:
            with (
                patch.object(fe._popup_menu, "unpost") as mock_file_unpost,
                patch.object(fe._folder_popup_menu, "unpost") as mock_folder_unpost,
            ):
                fe._dismiss_popup_menu()
            mock_file_unpost.assert_called_once()
            mock_folder_unpost.assert_called_once()
        finally:
            root.destroy()

    def test_dismiss_with_no_event_does_not_raise(self, tmp_path):
        """
        GIVEN no context menu is open
        WHEN _dismiss_popup_menu() is called with no event argument
        THEN no exception is raised.

        Affordance ID: PD-11-AF-010
        """
        root, fe, _ = _make_explorer(tmp_path)
        try:
            fe._dismiss_popup_menu()  # event=None by default
        finally:
            root.destroy()

    def test_dismiss_with_event_does_not_raise(self, tmp_path):
        """
        GIVEN a synthetic FocusOut event
        WHEN _dismiss_popup_menu(event) is called
        THEN no exception is raised.

        Affordance ID: PD-11-AF-010
        """
        root, fe, _ = _make_explorer(tmp_path)
        try:
            ev = _fake_event()
            fe._dismiss_popup_menu(ev)
        finally:
            root.destroy()


@pytest.mark.unit
class TestMenuVisibilityRecovery:
    """
    Unit tests for recovery behavior when a context menu is immediately unposted.

    Affordance ID: PD-11-AF-008
    Source: FileExplorer._verify_menu_visible()
    """

    def test_verify_reposts_once_when_unmapped_for_active_generation(self, tmp_path):
        """
        GIVEN the current generation menu is not mapped
        WHEN _verify_menu_visible() runs
        THEN _post_menu() is called once with is_retry=True.

        Affordance ID: PD-11-AF-008
        """
        root, fe, _ = _make_explorer(tmp_path)
        try:
            fe._menu_post_generation = 5
            with (
                patch.object(fe._popup_menu, "winfo_ismapped", return_value=0),
                patch.object(fe, "_post_menu") as mock_post_menu,
            ):
                fe._verify_menu_visible(fe._popup_menu, 100, 200, generation=5)

            mock_post_menu.assert_called_once_with(fe._popup_menu, 100, 200, 5, is_retry=True)
        finally:
            root.destroy()


@pytest.mark.unit
class TestWaylandPopupFallback:
    """Unit tests for Wayland-specific custom popup fallback behavior."""

    def test_right_click_uses_wayland_popup_when_enabled(self, tmp_path):
        """
        GIVEN forced Wayland popup mode
        WHEN right-clicking a file row
        THEN _show_wayland_popup() is called and _post_menu() is not.

        Affordance ID: PD-11-AF-008
        """
        (tmp_path / "hello.py").write_text("x")
        root, fe, _ = _make_explorer(tmp_path)
        try:
            fe._FORCE_WAYLAND_POPUP = True
            fe._MENU_POST_DELAY_MS = 0
            items = fe.tree.get_children()
            file_item = next(i for i in items if "file" in fe.tree.item(i, "tags"))
            bbox = fe.tree.bbox(file_item)
            y = int(bbox[1]) + int(bbox[3]) // 2 if bbox else 5
            ev = _fake_event(x=10, y=y)

            with (
                patch.object(fe, "_show_wayland_popup") as mock_show_wayland,
                patch.object(fe, "_post_menu") as mock_post_menu,
            ):
                result = fe._on_right_click(ev)
                root.update()

            mock_show_wayland.assert_called_once()
            mock_post_menu.assert_not_called()
            assert result == "break"
        finally:
            root.destroy()

    def test_dismiss_hides_wayland_popup(self, tmp_path):
        """
        GIVEN a visible Wayland popup window
        WHEN _dismiss_popup_menu() is called
        THEN the popup is withdrawn.

        Affordance ID: PD-11-AF-010
        """
        (tmp_path / "hello.py").write_text("x")
        root, fe, _ = _make_explorer(tmp_path)
        try:
            fe._FORCE_WAYLAND_POPUP = True
            fe._ensure_wayland_popup()
            assert fe._wayland_popup is not None
            fe._wayland_popup.deiconify()
            root.update_idletasks()
            assert fe._wayland_popup.winfo_ismapped() == 1
            fe._dismiss_popup_menu()
            root.update_idletasks()
            assert fe._wayland_popup.winfo_ismapped() == 0
        finally:
            root.destroy()

    def test_show_wayland_popup_recreates_window_each_time(self, tmp_path):
        """
        GIVEN Wayland fallback popup mode is active
        WHEN _show_wayland_popup() is called for consecutive right-clicks
        THEN a fresh popup toplevel is created each time to avoid stale compositor state.

        Affordance ID: PD-11-AF-008
        """
        (tmp_path / "hello.py").write_text("x")
        root, fe, _ = _make_explorer(tmp_path)
        try:
            fe._FORCE_WAYLAND_POPUP = True
            fe._show_wayland_popup("file", 100, 120)
            assert fe._wayland_popup is not None
            first_popup = fe._wayland_popup
            assert first_popup.winfo_exists() == 1

            fe._show_wayland_popup("file", 140, 160)
            assert fe._wayland_popup is not None
            second_popup = fe._wayland_popup

            assert second_popup is not first_popup
            assert second_popup.winfo_exists() == 1
        finally:
            root.destroy()

    def test_wayland_popup_uses_theme_background_on_toplevel(self, tmp_path):
        """
        GIVEN Wayland fallback popup mode with a dark panel background
        WHEN _ensure_wayland_popup() creates the popup window
        THEN the toplevel background matches the themed panel color immediately.

        Affordance ID: PD-11-AF-008
        """
        (tmp_path / "hello.py").write_text("x")
        root, fe, _ = _make_explorer(tmp_path)
        try:
            fe._FORCE_WAYLAND_POPUP = True
            fe._menu_colors["panel_bg"] = "#1f1f1f"
            fe._ensure_wayland_popup()
            assert fe._wayland_popup is not None
            assert fe._wayland_popup.cget("bg") == "#1f1f1f"
        finally:
            root.destroy()

    def test_verify_does_nothing_when_generation_is_stale(self, tmp_path):
        """
        GIVEN a stale generation from an older click
        WHEN _verify_menu_visible() runs
        THEN no repost is attempted.

        Affordance ID: PD-11-AF-008
        """
        root, fe, _ = _make_explorer(tmp_path)
        try:
            fe._menu_post_generation = 8
            with patch.object(fe, "_post_menu") as mock_post_menu:
                fe._verify_menu_visible(fe._popup_menu, 100, 200, generation=7)
            mock_post_menu.assert_not_called()
        finally:
            root.destroy()
