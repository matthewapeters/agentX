"""
Coverage uplift tests for src/agentx/file_explorer.py (non-GUI methods).

Targets the 55% → 90% uplift by covering:
  - list_directory: OSError on individual file (52-54) and directory (57-58)
  - change_directory: forward-history truncation (67-76)
  - open_file: OSError / UnicodeDecodeError handler (93-97)
  - navigate_back / navigate_forward: success and boundary paths
  - navigate_home: home directory navigation
  - navigate_parent: success and root boundary
"""

import os
import pytest
from pathlib import Path
from unittest.mock import patch, MagicMock

# ---------------------------------------------------------------------------
# list_directory
# ---------------------------------------------------------------------------


class TestListDirectory:
    def test_returns_files_and_dirs(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        (tmp_path / "a.py").write_text("code")
        (tmp_path / "subdir").mkdir()

        fe = FileExplorer(str(tmp_path))
        items = fe.list_directory()
        names = [i["name"] for i in items]
        assert "a.py" in names
        assert "subdir" in names

    def test_error_on_individual_file_is_skipped(self, tmp_path):
        """OSError while getting file size is caught and item skipped (lines 52-54)."""
        from agentx.file_explorer import FileExplorer

        (tmp_path / "ok.txt").write_text("ok")
        (tmp_path / "bad.txt").write_text("bad")

        fe = FileExplorer(str(tmp_path))

        original_getsize = os.path.getsize

        def flaky_getsize(path):
            if "bad.txt" in path:
                raise OSError("permission denied")
            return original_getsize(path)

        with patch("os.path.getsize", side_effect=flaky_getsize):
            items = fe.list_directory()

        names = [i["name"] for i in items]
        assert "ok.txt" in names
        assert "bad.txt" not in names

    def test_oserror_on_directory_returns_empty(self, tmp_path):
        """OSError on os.listdir returns [] (lines 57-58)."""
        from agentx.file_explorer import FileExplorer

        fe = FileExplorer(str(tmp_path))
        with patch("os.listdir", side_effect=OSError("no access")):
            result = fe.list_directory()
        assert result == []


# ---------------------------------------------------------------------------
# change_directory
# ---------------------------------------------------------------------------


class TestChangeDirectory:
    def test_successful_change(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        sub = tmp_path / "sub"
        sub.mkdir()
        fe = FileExplorer(str(tmp_path))
        assert fe.change_directory(str(sub)) is True
        assert fe.current_path == str(sub)

    def test_returns_false_for_non_directory(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        fe = FileExplorer(str(tmp_path))
        assert fe.change_directory("/nonexistent/path/xyz") is False

    def test_forward_history_truncated_on_new_navigation(self, tmp_path):
        """Covers lines 67-76 — forward history is dropped when navigating to a new dir."""
        from agentx.file_explorer import FileExplorer

        dirs = [tmp_path / f"d{i}" for i in range(3)]
        for d in dirs:
            d.mkdir()

        fe = FileExplorer(str(tmp_path))
        fe.change_directory(str(dirs[0]))
        fe.change_directory(str(dirs[1]))
        # Go back to dirs[0]
        fe.navigate_back()
        assert fe.history_index == 1  # at dirs[0] position

        # Navigate to a NEW dir — forward history (dirs[1]) should be truncated
        fe.change_directory(str(dirs[2]))
        assert len(fe.history) == 3  # initial + dirs[0] + dirs[2] (dirs[1] dropped)
        assert str(dirs[2]) in fe.history


# ---------------------------------------------------------------------------
# open_file
# ---------------------------------------------------------------------------


class TestOpenFile:
    def test_reads_file_contents(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        f = tmp_path / "hello.txt"
        f.write_text("hello world")
        fe = FileExplorer(str(tmp_path))
        assert fe.open_file(str(f)) == "hello world"

    def test_oserror_returns_empty_string(self, tmp_path):
        """OSError on open returns '' (lines 93-97)."""
        from agentx.file_explorer import FileExplorer

        fe = FileExplorer(str(tmp_path))
        with patch("builtins.open", side_effect=OSError("permission denied")):
            result = fe.open_file("/some/file.txt")
        assert result == ""

    def test_unicode_error_returns_empty_string(self, tmp_path):
        """UnicodeDecodeError on read returns ''."""
        from agentx.file_explorer import FileExplorer

        fe = FileExplorer(str(tmp_path))
        with patch("builtins.open", side_effect=UnicodeDecodeError("utf-8", b"", 0, 1, "reason")):
            result = fe.open_file("/some/binary.bin")
        assert result == ""


# ---------------------------------------------------------------------------
# navigate_back / navigate_forward
# ---------------------------------------------------------------------------


class TestNavigateBackForward:
    def _fe_with_history(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        dirs = [tmp_path / f"d{i}" for i in range(3)]
        for d in dirs:
            d.mkdir()
        fe = FileExplorer(str(tmp_path))
        fe.change_directory(str(dirs[0]))
        fe.change_directory(str(dirs[1]))
        return fe, dirs

    def test_navigate_back_success(self, tmp_path):
        """navigate_back returns True when history allows."""
        fe, dirs = self._fe_with_history(tmp_path)
        assert fe.navigate_back() is True
        assert fe.current_path == str(dirs[0])

    def test_navigate_back_at_start_returns_false(self, tmp_path):
        """navigate_back returns False when already at start."""
        from agentx.file_explorer import FileExplorer

        fe = FileExplorer(str(tmp_path))
        assert fe.navigate_back() is False

    def test_navigate_forward_success(self, tmp_path):
        """navigate_forward returns True and moves forward."""
        fe, dirs = self._fe_with_history(tmp_path)
        fe.navigate_back()
        assert fe.navigate_forward() is True
        assert fe.current_path == str(dirs[1])

    def test_navigate_forward_at_end_returns_false(self, tmp_path):
        """navigate_forward returns False when at end of history."""
        fe, dirs = self._fe_with_history(tmp_path)
        assert fe.navigate_forward() is False


# ---------------------------------------------------------------------------
# navigate_home
# ---------------------------------------------------------------------------


class TestNavigateHome:
    def test_navigates_to_home_directory(self, tmp_path):
        """navigate_home changes current_path to home (lines 127-128)."""
        from agentx.file_explorer import FileExplorer

        fe = FileExplorer(str(tmp_path))
        home = str(Path.home())
        fe.navigate_home()
        assert fe.current_path == home


# ---------------------------------------------------------------------------
# navigate_parent
# ---------------------------------------------------------------------------


class TestNavigateParent:
    def test_navigate_parent_goes_up(self, tmp_path):
        """navigate_parent returns True and moves up one level."""
        from agentx.file_explorer import FileExplorer

        sub = tmp_path / "child"
        sub.mkdir()
        fe = FileExplorer(str(sub))
        result = fe.navigate_parent()
        assert result is True
        assert fe.current_path == str(tmp_path)

    def test_navigate_parent_at_root_returns_false(self):
        """navigate_parent returns False when already at filesystem root (lines 136-139)."""
        from agentx.file_explorer import FileExplorer

        # Use "/" as root — dirname("/") == "/" so it won't navigate
        fe = FileExplorer("/")
        fe.current_path = "/"
        result = fe.navigate_parent()
        assert result is False


# ---------------------------------------------------------------------------
# get_current_path (line 84)
# ---------------------------------------------------------------------------


class TestGetCurrentPath:
    def test_returns_current_path(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        fe = FileExplorer(str(tmp_path))
        assert fe.get_current_path() == str(tmp_path)


# ---------------------------------------------------------------------------
# GUI callbacks — test via to_gui() + direct method calls
# ---------------------------------------------------------------------------


def _make_root():
    import tkinter as tk

    root = tk.Tk()
    root.withdraw()
    return root


class TestGUICallbacks:
    """Test GUI callback methods after calling to_gui() to set up widget state."""

    def test_on_back_click_navigates(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        sub = tmp_path / "child"
        sub.mkdir()
        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            # Navigate into child dir, then back
            fe.change_directory(str(sub))
            fe._on_back_click()
            assert fe.current_path == str(tmp_path)
        finally:
            root.destroy()

    def test_on_forward_click_navigates(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        sub = tmp_path / "child"
        sub.mkdir()
        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            fe.change_directory(str(sub))
            fe.navigate_back()
            fe._on_forward_click()
            assert fe.current_path == str(sub)
        finally:
            root.destroy()

    def test_on_up_click_navigates_parent(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        sub = tmp_path / "child"
        sub.mkdir()
        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(sub))
            fe.to_gui(parent)
            fe._on_up_click()
            assert fe.current_path == str(tmp_path)
        finally:
            root.destroy()

    def test_on_home_click_navigates_home(self, tmp_path):
        from agentx.file_explorer import FileExplorer
        from pathlib import Path

        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            fe._on_home_click()
            assert fe.current_path == str(Path.home())
        finally:
            root.destroy()

    def test_on_refresh_click_repopulates(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            # Should not raise
            fe._on_refresh_click()
        finally:
            root.destroy()

    def test_update_path_display(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            fe._update_path_display()
            assert str(tmp_path) in fe._path_label.cget("text")
        finally:
            root.destroy()

    def test_get_selected_folder_name_no_selection(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            # No selection → returns None
            result = fe._get_selected_folder_name()
            assert result is None
        finally:
            root.destroy()

    def test_dismiss_popup_menu(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            # Should not raise
            fe._dismiss_popup_menu()
        finally:
            root.destroy()

    def test_populate_tree_shows_items(self, tmp_path):
        from agentx.file_explorer import FileExplorer

        (tmp_path / "readme.txt").write_text("hi")
        (tmp_path / "subdir").mkdir()
        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            fe._populate_tree()
            children = fe.tree.get_children()
            assert len(children) > 0
        finally:
            root.destroy()

    def test_on_back_click_at_start_does_nothing(self, tmp_path):
        """_on_back_click at history start does not raise."""
        from agentx.file_explorer import FileExplorer

        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            # Should not navigate (at start), should not raise
            fe._on_back_click()
        finally:
            root.destroy()

    def test_on_forward_click_at_end_does_nothing(self, tmp_path):
        """_on_forward_click at end of history does not raise."""
        from agentx.file_explorer import FileExplorer

        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            fe._on_forward_click()
        finally:
            root.destroy()

    def test_on_item_double_click_navigates_into_dir(self, tmp_path):
        """_on_item_double_click changes into a directory when a dir item is selected."""
        from agentx.file_explorer import FileExplorer

        sub = tmp_path / "mydir"
        sub.mkdir()
        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            # Insert a directory item manually and select it
            item_id = fe.tree.insert("", "end", text="📁 mydir", values=("Folder", ""), tags=("directory",))
            fe.tree.selection_set(item_id)
            fe._on_item_double_click(None)
            assert fe.current_path == str(sub)
        finally:
            root.destroy()

    def test_on_item_double_click_on_file_does_not_navigate(self, tmp_path):
        """_on_item_double_click on a file does not change directory."""
        from agentx.file_explorer import FileExplorer

        (tmp_path / "readme.txt").write_text("hi")
        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            item_id = fe.tree.insert("", "end", text="📄 readme.txt", values=("File", "0 KB"), tags=("file",))
            fe.tree.selection_set(item_id)
            original_path = fe.current_path
            fe._on_item_double_click(None)
            assert fe.current_path == original_path  # no change
        finally:
            root.destroy()

    def test_get_selected_folder_name_with_selection(self, tmp_path):
        """_get_selected_folder_name returns folder name from selected item (lines 414-415)."""
        from agentx.file_explorer import FileExplorer

        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent)
            item_id = fe.tree.insert("", "end", text="📁 myfolder", values=("Folder", ""), tags=("directory",))
            fe.tree.selection_set(item_id)
            result = fe._get_selected_folder_name()
            assert result == "myfolder"
        finally:
            root.destroy()

    def test_on_edit_selected_calls_callback(self, tmp_path):
        """_on_edit_selected invokes the edit callback with the file path (lines 441-448)."""
        from agentx.file_explorer import FileExplorer

        callback = MagicMock()
        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            fe.to_gui(parent, on_edit=callback)
            item_id = fe.tree.insert("", "end", text="📄 notes.txt", values=("File", "1 KB"), tags=("file",))
            fe.tree.selection_set(item_id)
            fe._on_edit_selected()
            callback.assert_called_once()
            called_path = callback.call_args[0][0]
            assert called_path.endswith("notes.txt")
        finally:
            root.destroy()

    def test_to_gui_label_font_fallback_when_emoji_font_missing(self, tmp_path):
        """Covers line 287 — uses 'Terminal' font when NotoColorEmoji.ttf is absent."""
        from agentx.file_explorer import FileExplorer

        root = _make_root()
        try:
            import tkinter as tk

            parent = tk.Frame(root)
            parent.pack()
            fe = FileExplorer(str(tmp_path))
            with patch("os.path.exists", return_value=False):
                fe.to_gui(parent)
            # Label should use terminal font fallback — no exception
            assert fe._path_label is not None
        finally:
            root.destroy()
