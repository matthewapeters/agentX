"""
Docstring for agentx.file_explorer
"""

import logging
import os
import tkinter as tk
from pathlib import Path
from tkinter import ttk

_logger = logging.getLogger(__name__)


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
        self._root_path = self.current_path
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

    def to_gui(
        self,
        parent_frame: tk.Frame,
        on_attach=None,
        on_edit=None,
        on_add_folder_to_memory=None,
        theme_mode: str = "Dark Mode",
        bg: str | None = None,
        panel_bg: str | None = None,
        fg: str | None = None,
        muted_fg: str | None = None,
        tree_bg: str | None = None,
        tree_fg: str | None = None,
        selection_bg: str = "#3399ff",
        selection_fg: str = "#ffffff",
    ) -> tk.Frame:
        """
        Create a GUI frame for the file explorer.

        :param parent_frame: The parent Tkinter frame to attach the file explorer GUI to.
        :param on_attach: Callback invoked with a file path when "Attach" is selected.
        :param on_edit: Callback invoked with a file path when "Edit" is selected.
        :param on_add_folder_to_memory: Callback ``(key: str, value: str) -> None`` invoked
            when a folder path is added to working memory. ``key`` is the folder name;
            ``value`` is the path string chosen by the user (full or relative).
        :return: A Tkinter frame containing the file explorer GUI.
        """
        _logger.debug("[FileExplorer] to_gui() called")
        if theme_mode == "Light Mode":
            defaults = {
                "bg": "#ffffff",
                "panel_bg": "#f0f4f8",
                "fg": "#111827",
                "muted_fg": "#4b5563",
                "tree_bg": "#ffffff",
                "tree_fg": "#111827",
            }
        else:
            defaults = {
                "bg": "#333333",
                "panel_bg": "#3a3a3a",
                "fg": "#eeeeee",
                "muted_fg": "#bbbbbb",
                "tree_bg": "#2a2a2a",
                "tree_fg": "#eeeeee",
            }

        colors = {
            "bg": bg or defaults["bg"],
            "panel_bg": panel_bg or defaults["panel_bg"],
            "fg": fg or defaults["fg"],
            "muted_fg": muted_fg or defaults["muted_fg"],
            "tree_bg": tree_bg or defaults["tree_bg"],
            "tree_fg": tree_fg or defaults["tree_fg"],
            "selection_bg": selection_bg,
            "selection_fg": selection_fg,
        }

        frame = tk.Frame(parent_frame, bg=colors["bg"])

        # Top frame for navigation controls and path display
        top_frame = tk.Frame(frame, bg=colors["panel_bg"], height=60)
        top_frame.pack(side=tk.TOP, fill=tk.X, padx=0, pady=0)
        top_frame.pack_propagate(False)

        # Navigation buttons frame
        button_frame = tk.Frame(top_frame, bg=colors["panel_bg"])
        button_frame.pack(side=tk.TOP, fill=tk.X, padx=5, pady=5)

        back_btn = tk.Button(
            button_frame,
            text="◀ Back",
            width=6,
            command=self._on_back_click,
            bg=colors["panel_bg"],
            fg=colors["fg"],
            activebackground=colors["bg"],
            activeforeground=colors["fg"],
        )
        back_btn.pack(side=tk.LEFT, padx=2)

        forward_btn = tk.Button(
            button_frame,
            text="Forward ▶",
            width=8,
            command=self._on_forward_click,
            bg=colors["panel_bg"],
            fg=colors["fg"],
            activebackground=colors["bg"],
            activeforeground=colors["fg"],
        )
        forward_btn.pack(side=tk.LEFT, padx=2)

        up_btn = tk.Button(
            button_frame,
            text="⬆ Up",
            width=5,
            command=self._on_up_click,
            bg=colors["panel_bg"],
            fg=colors["fg"],
            activebackground=colors["bg"],
            activeforeground=colors["fg"],
        )
        up_btn.pack(side=tk.LEFT, padx=2)

        home_btn = tk.Button(
            button_frame,
            text="🏠 Home",
            width=7,
            command=self._on_home_click,
            bg=colors["panel_bg"],
            fg=colors["fg"],
            activebackground=colors["bg"],
            activeforeground=colors["fg"],
        )
        home_btn.pack(side=tk.LEFT, padx=2)

        refresh_btn = tk.Button(
            button_frame,
            text="🔄 Refresh",
            width=8,
            command=self._on_refresh_click,
            bg=colors["panel_bg"],
            fg=colors["fg"],
            activebackground=colors["bg"],
            activeforeground=colors["fg"],
        )
        refresh_btn.pack(side=tk.LEFT, padx=2)

        # Current path display
        path_label = tk.Label(
            top_frame,
            text=f"📁 {self.current_path}",
            bg=colors["panel_bg"],
            fg=colors["fg"],
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
        tree_frame = tk.Frame(frame, bg=colors["bg"])
        tree_frame.pack(side=tk.TOP, fill=tk.BOTH, expand=True, padx=0, pady=0)

        # Scrollbars
        vsb = ttk.Scrollbar(tree_frame, orient=tk.VERTICAL)
        hsb = ttk.Scrollbar(tree_frame, orient=tk.HORIZONTAL)

        style = ttk.Style(frame)
        tree_style_name = f"AgentXFileExplorer{abs(id(self))}.Treeview"
        tree_heading_style = f"{tree_style_name}.Heading"
        style.configure(
            tree_style_name,
            background=colors["tree_bg"],
            fieldbackground=colors["tree_bg"],
            foreground=colors["tree_fg"],
        )
        style.map(
            tree_style_name,
            background=[("selected", colors["selection_bg"])],
            foreground=[("selected", colors["selection_fg"])],
        )
        style.configure(
            tree_heading_style,
            background=colors["panel_bg"],
            foreground=colors["fg"],
        )

        # Treeview widget
        self.tree = ttk.Treeview(
            tree_frame,
            columns=("type", "size"),
            height=15,
            yscrollcommand=vsb.set,
            xscrollcommand=hsb.set,
            style=tree_style_name,
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

        # --- Right-click popup menu for files ---
        self._popup_menu = tk.Menu(
            self.tree,
            tearoff=0,
            bg=colors["panel_bg"],
            fg=colors["fg"],
            activebackground=colors["selection_bg"],
            activeforeground=colors["selection_fg"],
        )
        self._popup_menu.add_command(label="Attach", command=self._on_attach_selected)
        self._popup_menu.add_command(label="Edit", command=self._on_edit_selected)
        self._on_attach_callback = on_attach
        self._on_edit_callback = on_edit
        _logger.debug(f"[FileExplorer] Created file menu: {id(self._popup_menu)}")

        # --- Right-click popup menu for folders ---
        self._folder_popup_menu = tk.Menu(
            self.tree,
            tearoff=0,
            bg=colors["panel_bg"],
            fg=colors["fg"],
            activebackground=colors["selection_bg"],
            activeforeground=colors["selection_fg"],
        )
        self._folder_popup_menu.add_command(label="Add full path to memory", command=self._on_add_full_path_selected)
        self._folder_popup_menu.add_command(
            label="Add relative path to memory", command=self._on_add_relative_path_selected
        )
        self._on_add_folder_to_memory_callback = on_add_folder_to_memory
        _logger.debug(f"[FileExplorer] Created folder menu: {id(self._folder_popup_menu)}")

        # Bind right-click on PRESS (Button-3), not release.
        #
        # History of this binding:
        #   v0.22.2  Used <ButtonRelease-3> because tk_popup() internally calls
        #            Tcl 'grab', which captures the subsequent ButtonRelease and
        #            sends it to the menu before the user can select anything — the
        #            menu's own <ButtonRelease> class binding then calls unpost().
        #   v0.22.5  Replaced tk_popup() with menu.post() (no grab).  However,
        #            keeping <ButtonRelease-3> introduced a new race:
        #            menu.post() creates the menu window at (x_root, y_root) — i.e.
        #            directly under the cursor — so after the handler returns the X
        #            server sends an <Enter> event to the new menu window followed by
        #            the same <ButtonRelease-3> (synthesised or queued).  The Menu
        #            class has a generic <ButtonRelease> binding (tk::MenuInvoke)
        #            which calls unpost() when no item is active.  Result: menu
        #            appears and immediately vanishes.
        #   v0.22.6  Switch back to <Button-3> (press).  With menu.post() there is
        #            no grab, so the subsequent <ButtonRelease-3> goes to whichever
        #            window the cursor is over when the button is released — either
        #            the menu (item invoked ✓) or the treeview (ignored, menu stays ✓).
        #            The press-trigger/no-grab combination is the standard Tkinter
        #            idiom for right-click context menus on Linux.
        self.tree.bind("<Button-3>", self._on_right_click)
        self.tree.bind("<Control-Button-1>", self._on_right_click)
        self.tree.bind("<Escape>", self._dismiss_popup_menu)
        # NOTE: <FocusOut> is intentionally NOT bound here.  The menu is dismissed
        # by clicking elsewhere (Tk's root ButtonPress auto-dismiss) or pressing Escape.

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

    def _dismiss_popup_menu(self, event=None):
        _logger.debug(f"[FileExplorer] _dismiss_popup_menu called (event={event})")
        _logger.debug(f"[FileExplorer]   file menu id={id(self._popup_menu)} - calling unpost()")
        self._popup_menu.unpost()
        _logger.debug(f"[FileExplorer]   folder menu id={id(self._folder_popup_menu)} - calling unpost()")
        self._folder_popup_menu.unpost()

    def _on_right_click(self, event):
        _logger.debug(
            f"[FileExplorer] _on_right_click called: x={event.x}, y={event.y}, x_root={event.x_root}, y_root={event.y_root}"
        )
        item = self.tree.identify_row(event.y)
        if not item:
            _logger.debug("[FileExplorer]   no item found, returning break")
            return "break"
        self.tree.selection_set(item)
        tags = self.tree.item(item, "tags")
        menu: tk.Menu | None = None
        if "file" in tags:
            menu = self._popup_menu
        elif "directory" in tags:
            menu = self._folder_popup_menu
        if menu is None:
            _logger.debug("[FileExplorer]   menu is None, returning break")
            return "break"
        # Use menu.post() rather than menu.tk_popup().
        # tk_popup() calls Tcl's `grab` command internally; on Linux with any
        # modern compositor (GNOME/Mutter, KWin, Wayland/XWayland) the WM already
        # holds a server-side grab for the ButtonRelease event and resolves the
        # conflict by cancelling Tk's grab, which immediately calls unpost().
        # menu.post() positions and displays the menu without setting any grab.
        # Tk's native root-window <ButtonPress> binding handles auto-dismiss when
        # the user clicks outside the menu; <Escape> is bound explicitly on the tree.
        _logger.debug(f"[FileExplorer]   posting menu {id(menu)} at ({event.x_root}, {event.y_root})")
        menu.post(event.x_root, event.y_root)
        _logger.debug(f"[FileExplorer]   menu.post() completed, returning break")
        return "break"

    def _get_selected_folder_name(self) -> str | None:
        selection = self.tree.selection()
        if not selection:
            return None
        item_text = self.tree.item(selection[0], "text")
        return item_text.split(" ", 1)[1] if " " in item_text else item_text

    def _on_add_full_path_selected(self):
        folder_name = self._get_selected_folder_name()
        if folder_name and self._on_add_folder_to_memory_callback:
            full_path = os.path.join(self.current_path, folder_name)
            self._on_add_folder_to_memory_callback(folder_name, full_path)

    def _on_add_relative_path_selected(self):
        folder_name = self._get_selected_folder_name()
        if folder_name and self._on_add_folder_to_memory_callback:
            full_path = os.path.join(self.current_path, folder_name)
            rel_path = os.path.relpath(full_path, self._root_path)
            self._on_add_folder_to_memory_callback(folder_name, rel_path)

    def _on_attach_selected(self):
        selection = self.tree.selection()
        if selection:
            item = selection[0]
            item_text = self.tree.item(item, "text")
            item_name = item_text.split(" ", 1)[1] if " " in item_text else item_text
            file_path = os.path.join(self.current_path, item_name)
            if self._on_attach_callback:
                self._on_attach_callback(file_path)

    def _on_edit_selected(self):
        selection = self.tree.selection()
        if selection:
            item = selection[0]
            item_text = self.tree.item(item, "text")
            item_name = item_text.split(" ", 1)[1] if " " in item_text else item_text
            file_path = os.path.join(self.current_path, item_name)
            if self._on_edit_callback:
                self._on_edit_callback(file_path)

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
            self._back_btn.config(state=tk.NORMAL if self.history_index > 0 else tk.DISABLED)
        if hasattr(self, "_forward_btn"):
            self._forward_btn.config(state=(tk.NORMAL if self.history_index < len(self.history) - 1 else tk.DISABLED))
