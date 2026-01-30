"""
Docstring for agentx.file_explorer
"""

import tkinter as tk
from tkinter import ttk
import os
from pathlib import Path


class FileExplorer:
    """
    A file explorer widget that allows users to navigate the file system
    and explore directories and files.
    """

    def __init__(self, start_path: str = str(Path.home())):
        """
        Initialize the FileExplorer.

        :param start_path: The initial path to start exploring from.
        """
        self.current_path = os.path.abspath(start_path)
        self.history = [self.current_path]
        self.history_index = 0

    def list_directory(self) -> list[dict]:
        """
        List the contents of the current directory.

        :return: A list of dictionaries containing file/directory information.
        """
        try:
            items = []
            entries = os.listdir(self.current_path)
            entries.sort()

            for entry in entries:
                full_path = os.path.join(self.current_path, entry)
                is_dir = os.path.isdir(full_path)
                try:
                    size = os.path.getsize(full_path) if not is_dir else None
                    items.append(
                        {
                            "name": entry,
                            "path": full_path,
                            "is_dir": is_dir,
                            "size": size,
                        }
                    )
                except (OSError, PermissionError):
                    # Skip files we can't access
                    continue

            return items
        except (OSError, PermissionError):
            return []

    def change_directory(self, new_path: str) -> bool:
        """
        Change the current directory.

        :param new_path: The new directory path to change to.
        :return: True if successful, False otherwise.
        """
        abs_path = os.path.abspath(new_path)
        if os.path.isdir(abs_path):
            self.current_path = abs_path
            # Update history
            if self.history_index < len(self.history) - 1:
                self.history = self.history[: self.history_index + 1]
            self.history.append(abs_path)
            self.history_index = len(self.history) - 1
            return True
        return False

    def get_current_path(self) -> str:
        """
        Get the current directory path.

        :return: The current directory path.
        """
        return self.current_path

    def open_file(self, file_path: str) -> str:
        """
        Open and read the contents of a file.

        :param file_path: The path of the file to open.
        :return: The contents of the file as a string.
        """
        try:
            with open(file_path, "r", encoding="utf-8") as f:
                return f.read()
        except (OSError, UnicodeDecodeError):
            return ""

    def navigate_back(self) -> bool:
        """
        Navigate to the previous directory in history.

        :return: True if successful, False if at the start of history.
        """
        if self.history_index > 0:
            self.history_index -= 1
            self.current_path = self.history[self.history_index]
            return True
        return False

    def navigate_forward(self) -> bool:
        """
        Navigate to the next directory in history.

        :return: True if successful, False if at the end of history.
        """
        if self.history_index < len(self.history) - 1:
            self.history_index += 1
            self.current_path = self.history[self.history_index]
            return True
        return False

    def navigate_home(self):
        """
        Navigate to the home directory.
        """
        home_path = str(Path.home())
        self.change_directory(home_path)

    def navigate_parent(self) -> bool:
        """
        Navigate to the parent directory.

        :return: True if successful, False if already at root.
        """
        parent = os.path.dirname(self.current_path)
        if parent != self.current_path:  # Not at root
            return self.change_directory(parent)
        return False

    def to_gui(self, parent_frame: tk.Frame) -> tk.Frame:
        """
        Create a GUI frame for the file explorer.

        :param parent_frame: The parent Tkinter frame to attach the file explorer GUI to.
        :return: A Tkinter frame containing the file explorer GUI.
        """
        frame = tk.Frame(parent_frame, bg="white")

        # Top frame for navigation controls and path display
        top_frame = tk.Frame(frame, bg="lightgray", height=60)
        top_frame.pack(side=tk.TOP, fill=tk.X, padx=0, pady=0)
        top_frame.pack_propagate(False)

        # Navigation buttons frame
        button_frame = tk.Frame(top_frame, bg="lightgray")
        button_frame.pack(side=tk.TOP, fill=tk.X, padx=5, pady=5)

        back_btn = tk.Button(
            button_frame, text="◀ Back", width=6, command=self._on_back_click
        )
        back_btn.pack(side=tk.LEFT, padx=2)

        forward_btn = tk.Button(
            button_frame, text="Forward ▶", width=8, command=self._on_forward_click
        )
        forward_btn.pack(side=tk.LEFT, padx=2)

        up_btn = tk.Button(
            button_frame, text="⬆ Up", width=5, command=self._on_up_click
        )
        up_btn.pack(side=tk.LEFT, padx=2)

        home_btn = tk.Button(
            button_frame, text="🏠 Home", width=7, command=self._on_home_click
        )
        home_btn.pack(side=tk.LEFT, padx=2)

        refresh_btn = tk.Button(
            button_frame, text="🔄 Refresh", width=8, command=self._on_refresh_click
        )
        refresh_btn.pack(side=tk.LEFT, padx=2)

        # Current path display
        path_label = tk.Label(
            top_frame,
            text=f"📁 {self.current_path}",
            bg="lightgray",
            fg="black",
            font=("Terminal", 9),
            justify=tk.LEFT,
        )
        path_label.pack(side=tk.TOP, fill=tk.X, padx=5, pady=0)

        # Update font to use NotoColorEmoji if available
        # Locate the font file relative to the installed package directory
        package_dir = os.path.dirname(__file__)
        emoji_font_path = os.path.join(package_dir, "fonts", "NotoColorEmoji.ttf")
        if os.path.exists(emoji_font_path):
            label_font = (emoji_font_path, 9)
        else:
            label_font = ("Terminal", 9)

        path_label.config(font=label_font)

        # Store references for updating
        self._path_label = path_label
        self._back_btn = back_btn
        self._forward_btn = forward_btn
        self._parent_frame = frame

        # Create treeview for file listing
        tree_frame = tk.Frame(frame)
        tree_frame.pack(side=tk.TOP, fill=tk.BOTH, expand=True, padx=0, pady=0)

        # Scrollbars
        vsb = ttk.Scrollbar(tree_frame, orient=tk.VERTICAL)
        hsb = ttk.Scrollbar(tree_frame, orient=tk.HORIZONTAL)

        # Treeview widget
        self.tree = ttk.Treeview(
            tree_frame,
            columns=("type", "size"),
            height=15,
            yscrollcommand=vsb.set,
            xscrollcommand=hsb.set,
        )
        vsb.config(command=self.tree.yview)
        hsb.config(command=self.tree.xview)

        # Define column headings and widths
        self.tree.column("#0", width=250, minwidth=150)
        self.tree.column("type", width=80, minwidth=50)
        self.tree.column("size", width=100, minwidth=50)

        self.tree.heading("#0", text="Name", anchor=tk.W)
        self.tree.heading("type", text="Type", anchor=tk.W)
        self.tree.heading("size", text="Size", anchor=tk.W)

        # Bind events
        self.tree.bind("<Double-1>", self._on_item_double_click)

        # Pack the treeview and scrollbars
        self.tree.grid(row=0, column=0, sticky="nsew")
        vsb.grid(row=0, column=1, sticky="ns")
        hsb.grid(row=1, column=0, sticky="ew")

        tree_frame.grid_rowconfigure(0, weight=1)
        tree_frame.grid_columnconfigure(0, weight=1)

        # Populate the tree
        self._populate_tree()
        self._update_button_states()

        return frame

    def _populate_tree(self):
        """
        Populate the treeview with the contents of the current directory.
        """
        # Clear existing items
        for item in self.tree.get_children():
            self.tree.delete(item)

        # Get directory contents
        items = self.list_directory()

        # Add items to tree (directories first, then files)
        dirs = [item for item in items if item["is_dir"]]
        files = [item for item in items if not item["is_dir"]]

        for item in dirs:
            size_text = ""
            self.tree.insert(
                "",
                "end",
                text=f"📁 {item['name']}",
                values=("Folder", size_text),
                tags=("directory",),
            )

        for item in files:
            size_kb = item["size"] / 1024 if item["size"] else 0
            if size_kb > 1024:
                size_text = f"{size_kb / 1024:.1f} MB"
            else:
                size_text = f"{size_kb:.1f} KB" if size_kb > 0 else "0 KB"

            self.tree.insert(
                "",
                "end",
                text=f"📄 {item['name']}",
                values=("File", size_text),
                tags=("file",),
            )

    def _on_item_double_click(self, event):
        """
        Handle double-click on a treeview item.
        """
        selection = self.tree.selection()
        if selection:
            item = selection[0]
            item_text = self.tree.item(item, "text")
            # Remove the emoji and get the actual name
            item_name = item_text.split(" ", 1)[1] if " " in item_text else item_text

            new_path = os.path.join(self.current_path, item_name)

            if os.path.isdir(new_path):
                self.change_directory(new_path)
                self._populate_tree()
                self._update_path_display()
                self._update_button_states()

    def _on_back_click(self):
        """
        Handle back button click.
        """
        if self.navigate_back():
            self._populate_tree()
            self._update_path_display()
            self._update_button_states()

    def _on_forward_click(self):
        """
        Handle forward button click.
        """
        if self.navigate_forward():
            self._populate_tree()
            self._update_path_display()
            self._update_button_states()

    def _on_up_click(self):
        """
        Handle up (parent directory) button click.
        """
        if self.navigate_parent():
            self._populate_tree()
            self._update_path_display()
            self._update_button_states()

    def _on_home_click(self):
        """
        Handle home button click.
        """
        self.navigate_home()
        self._populate_tree()
        self._update_path_display()
        self._update_button_states()

    def _on_refresh_click(self):
        """
        Handle refresh button click.
        """
        self._populate_tree()

    def _update_path_display(self):
        """
        Update the path display label.
        """
        if hasattr(self, "_path_label"):
            self._path_label.config(text=f"📁 {self.current_path}")

    def _update_button_states(self):
        """
        Update the enabled/disabled state of navigation buttons.
        """
        if hasattr(self, "_back_btn"):
            self._back_btn.config(
                state=tk.NORMAL if self.history_index > 0 else tk.DISABLED
            )
        if hasattr(self, "_forward_btn"):
            self._forward_btn.config(
                state=(
                    tk.NORMAL
                    if self.history_index < len(self.history) - 1
                    else tk.DISABLED
                )
            )
