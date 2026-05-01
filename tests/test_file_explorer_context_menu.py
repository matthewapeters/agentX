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

    def test_right_click_file_calls_post_on_file_menu(self, tmp_path):
        """
        GIVEN the tree has a file row
        WHEN the user right-clicks that row
        THEN menu.post() is called (NOT tk_popup) to display the file context menu
         AND the handler returns 'break' to stop event propagation
         AND the folder context menu is NOT posted.

        NOTE on test limitations: unit tests can only verify that the correct Tk
        method is called with the correct coordinates.  Whether the menu actually
        stays visible under a live X11 compositor (grab conflicts, WM interference)
        cannot be detected by headless unit tests and requires manual UAT.

        menu.post() is used instead of tk_popup() to avoid the Tcl grab command
        that tk_popup() sets internally.  On Linux compositors the WM cancels
        Tk's grab immediately, causing unpost().  post() has no grab at all.
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

            with (
                patch.object(fe._popup_menu, "post") as mock_file_post,
                patch.object(fe._popup_menu, "tk_popup") as mock_file_tk_popup,
                patch.object(fe._folder_popup_menu, "post") as mock_folder_post,
            ):
                result = fe._on_right_click(ev)

            mock_file_post.assert_called_once()
            mock_file_tk_popup.assert_not_called()
            mock_folder_post.assert_not_called()
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

        With menu.post() (no Tcl grab), <Button-3> press is the correct trigger.
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

    def test_right_click_directory_calls_post_on_folder_menu(self, tmp_path):
        """
        GIVEN the tree has a directory row
        WHEN the user right-clicks that row
        THEN menu.post() is called on the folder menu (NOT tk_popup)
         AND the handler returns 'break' to stop event propagation
         AND the file context menu is NOT posted.

        See test_right_click_file_calls_post_on_file_menu for the rationale for
        using post() instead of tk_popup().
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

            with (
                patch.object(fe._popup_menu, "post") as mock_file_post,
                patch.object(fe._folder_popup_menu, "post") as mock_folder_post,
                patch.object(fe._folder_popup_menu, "tk_popup") as mock_folder_tk_popup,
            ):
                result = fe._on_right_click(ev)

            mock_folder_post.assert_called_once()
            mock_folder_tk_popup.assert_not_called()
            mock_file_post.assert_not_called()
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
