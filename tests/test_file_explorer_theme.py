"""Theme-related tests for FileExplorer widget rendering."""

import tkinter as tk

from agentx.file_explorer import FileExplorer


def _make_root() -> tk.Tk:
    root = tk.Tk()
    root.withdraw()
    return root


def test_file_explorer_dark_mode_colors():
    root = _make_root()
    try:
        parent = tk.Frame(root, bg="#333333")
        parent.pack()

        explorer = FileExplorer(start_path=".")
        frame = explorer.to_gui(
            parent,
            theme_mode="Dark Mode",
            bg="#333333",
            panel_bg="#2a2a2a",
            fg="#eeeeee",
            tree_bg="#222222",
            tree_fg="#eeeeee",
        )

        assert frame.cget("bg") == "#333333"
        assert explorer._path_label.cget("fg") == "#eeeeee"
    finally:
        root.destroy()


def test_file_explorer_light_mode_colors():
    root = _make_root()
    try:
        parent = tk.Frame(root, bg="#f0f4f8")
        parent.pack()

        explorer = FileExplorer(start_path=".")
        frame = explorer.to_gui(
            parent,
            theme_mode="Light Mode",
            bg="#ffffff",
            panel_bg="#f0f4f8",
            fg="#111827",
            tree_bg="#ffffff",
            tree_fg="#111827",
        )

        assert frame.cget("bg") == "#ffffff"
        assert explorer._path_label.cget("fg") == "#111827"
    finally:
        root.destroy()
