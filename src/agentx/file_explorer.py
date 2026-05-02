"""
Docstring for agentx.file_explorer
"""

import logging
import os
import tkinter as tk
from pathlib import Path
from tkinter import ttk

_logger = logging.getLogger(__name__)
_to_gui_call_count = 0  # Track how many times to_gui() is called


class FileExplorer:
    """
    A file explorer widget that allows users to navigate the file system
    and explore directories and files.

    Class attributes
    ----------------
    _MENU_POST_DELAY_MS : int
        Milliseconds to wait before calling ``menu.post()`` after a right-click
        press event.  The delay ensures the ``<ButtonRelease-3>`` event fires on
        the treeview (where there is no binding) rather than on the newly-posted
        menu window.  Without the delay the menu posts while the button is still
        held; the release lands on the menu, ``tk::MenuInvoke`` finds no active
        item, and calls ``unpost()`` before the user can see anything.

        Set to ``0`` in unit tests (combined with ``root.update()``) to fire the
        callback synchronously without introducing real wall-clock latency.
    """

    _MENU_POST_DELAY_MS: int = 100
    _MENU_POST_VERIFY_DELAY_MS: int = 120
    _FORCE_WAYLAND_POPUP: bool | None = None

    def __init__(self, start_path: str = str(Path.home())):
        """
        Initialize the FileExplorer.

        :param start_path: The initial path to start exploring from.
        """
        self.current_path = os.path.abspath(start_path)
        self._root_path = self.current_path
        self.history = [self.current_path]
        self.history_index = 0
        self._menu_post_generation = 0
        self._menu_colors: dict[str, str] = {}
        self._wayland_popup: tk.Toplevel | None = None
        self._wayland_popup_frame: tk.Frame | None = None

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
        global _to_gui_call_count
        _to_gui_call_count += 1
        msg = f"[FileExplorer] to_gui() called (call #{_to_gui_call_count})"
        print(msg)
        _logger.debug(msg)
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
        self._menu_colors = colors
        msg = f"[FileExplorer] Created file menu: {id(self._popup_menu)}"
        print(msg)
        _logger.debug(msg)

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
        msg = f"[FileExplorer] Created folder menu: {id(self._folder_popup_menu)}"
        print(msg)
        _logger.debug(msg)

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
        msg = f"[FileExplorer] _dismiss_popup_menu called (event={event})"
        print(msg)
        _logger.debug(msg)
        if self._wayland_popup is not None and self._wayland_popup.winfo_exists():
            self._wayland_popup.withdraw()
        msg2 = f"[FileExplorer]   file menu id={id(self._popup_menu)} - calling unpost()"
        print(msg2)
        _logger.debug(msg2)
        self._popup_menu.unpost()
        msg3 = f"[FileExplorer]   folder menu id={id(self._folder_popup_menu)} - calling unpost()"
        print(msg3)
        _logger.debug(msg3)
        self._folder_popup_menu.unpost()

    def _on_right_click(self, event):
        """Handle right-click on the treeview to show the context menu.

        The menu is posted via ``after(_MENU_POST_DELAY_MS)`` (default 100 ms)
        so that the ``<ButtonRelease-3>`` event fires on the treeview before
        the menu window exists.  ``after_idle`` is NOT sufficient: it fires
        before the button is physically released, so the release event still
        lands on the just-posted menu window.  The Tk ``Menu`` class binding
        ``tk::MenuInvoke`` then sees no active item and calls ``unpost()``
        immediately, making the menu invisible to the user.

        With a 100 ms timer the button is guaranteed to have been released by
        the time the callback fires.  The release lands on the treeview (no
        binding → no action) and the menu posts cleanly and remains visible.
        """
        item = self.tree.identify_row(event.y)
        if not item:
            return "break"
        self.tree.selection_set(item)
        tags = self.tree.item(item, "tags")
        menu: tk.Menu | None = None
        if "file" in tags:
            menu = self._popup_menu
        elif "directory" in tags:
            menu = self._folder_popup_menu
        if menu is None:
            return "break"
        # Derive screen coordinates from the widget's own position rather than
        # the raw X11 event coordinates.  Under XWayland on Wayland compositors,
        # event.x_root / event.y_root are in the XWayland virtual-screen physical
        # pixel space, which can extend far outside any visible monitor (e.g.
        # x_root=3753 was observed in UAT on a system with no monitor wider than
        # 3840px).  winfo_rootx/y() asks Tk for the widget's screen anchor — always
        # inside the visible window — and event.x/y are widget-relative offsets
        # bounded by the widget's own dimensions.
        x_root = self.tree.winfo_rootx() + event.x
        y_root = self.tree.winfo_rooty() + event.y
        msg = f"[FileExplorer] _on_right_click: scheduling deferred post of menu {id(menu)} at ({x_root}, {y_root})"
        print(msg)
        _logger.debug(msg)

        if self._use_wayland_popup():
            # Ensure any previously shown fallback popup cannot keep stale hitboxes
            # or compositor state before showing the next right-click menu.
            self._dismiss_popup_menu()
            popup_kind = "file" if menu is self._popup_menu else "directory"
            self.tree.after(
                self._MENU_POST_DELAY_MS,
                lambda k=popup_kind, x=x_root, y=y_root: self._show_wayland_popup(k, x, y),
            )
            return "break"

        self._menu_post_generation += 1
        generation = self._menu_post_generation
        self.tree.after(
            self._MENU_POST_DELAY_MS,
            lambda m=menu, x=x_root, y=y_root, g=generation: self._post_menu(m, x, y, g),
        )
        return "break"

    def _use_wayland_popup(self) -> bool:
        """Return True when running under Wayland where Tk menus may render invisibly."""
        if self._FORCE_WAYLAND_POPUP is not None:
            return self._FORCE_WAYLAND_POPUP
        return os.getenv("XDG_SESSION_TYPE", "").lower() == "wayland"

    def _ensure_wayland_popup(self) -> None:
        """Create reusable in-app popup container for Wayland fallback mode."""
        if self._wayland_popup is not None and self._wayland_popup.winfo_exists():
            return

        popup = tk.Toplevel(self.tree)
        popup.withdraw()
        popup.overrideredirect(True)
        popup.attributes("-topmost", True)
        frame = tk.Frame(
            popup,
            bg=self._menu_colors.get("panel_bg", "#2b2b2b"),
            borderwidth=1,
            relief="solid",
        )
        frame.pack(fill="both", expand=True)
        popup.bind("<Escape>", self._dismiss_popup_menu)
        self._wayland_popup = popup
        self._wayland_popup_frame = frame

    def _destroy_wayland_popup(self) -> None:
        """Destroy Wayland popup window so next show starts with a fresh surface."""
        if self._wayland_popup is None:
            return
        if self._wayland_popup.winfo_exists():
            self._wayland_popup.destroy()
        self._wayland_popup = None
        self._wayland_popup_frame = None

    def _render_wayland_popup_buttons(self, popup_kind: str) -> None:
        """Populate Wayland popup with commands for current selection type."""
        self._ensure_wayland_popup()
        assert self._wayland_popup_frame is not None
        frame = self._wayland_popup_frame
        for child in frame.winfo_children():
            child.destroy()

        bg = self._menu_colors.get("panel_bg", "#2b2b2b")
        fg = self._menu_colors.get("fg", "#f1f1f1")
        active_bg = self._menu_colors.get("selection_bg", "#404040")
        active_fg = self._menu_colors.get("selection_fg", "#ffffff")

        def add_btn(label: str, command) -> None:
            btn = tk.Button(
                frame,
                text=label,
                anchor="w",
                relief="flat",
                bd=0,
                padx=10,
                pady=6,
                bg=bg,
                fg=fg,
                activebackground=active_bg,
                activeforeground=active_fg,
                highlightthickness=0,
                command=lambda cb=command: self._invoke_wayland_action(cb),
            )
            btn.pack(fill="x")

        if popup_kind == "file":
            add_btn("Attach", self._on_attach_selected)
            add_btn("Edit", self._on_edit_selected)
        else:
            add_btn("Add full path to memory", self._on_add_full_path_selected)
            add_btn("Add relative path to memory", self._on_add_relative_path_selected)

    def _invoke_wayland_action(self, callback) -> None:
        """Run Wayland popup action and hide popup."""
        try:
            callback()
        finally:
            self._dismiss_popup_menu()

    def _show_wayland_popup(self, popup_kind: str, x_root: int, y_root: int) -> None:
        """Show a custom in-app popup window for Wayland sessions."""
        # Re-create the popup per invocation. Reusing an overrideredirect surface
        # with repeated withdraw/deiconify can intermittently become non-visual on
        # some Wayland compositors while still accepting clicks.
        self._destroy_wayland_popup()
        self._render_wayland_popup_buttons(popup_kind)
        assert self._wayland_popup is not None
        # Realize child widgets first so we can set a stable geometry before map,
        # avoiding a transient oversized flash on some compositors.
        self._wayland_popup.update_idletasks()
        req_w = max(self._wayland_popup.winfo_reqwidth(), 120)
        req_h = max(self._wayland_popup.winfo_reqheight(), 24)
        self._wayland_popup.geometry(f"{req_w}x{req_h}+{x_root}+{y_root}")
        self._wayland_popup.deiconify()
        self._wayland_popup.lift()
        msg = (
            f"[FileExplorer] _show_wayland_popup: kind={popup_kind}, "
            f"pos=({x_root},{y_root}), size={req_w}x{req_h}, "
            f"mapped={int(self._wayland_popup.winfo_ismapped())}"
        )
        print(msg)
        _logger.debug(msg)

    def _post_menu(
        self,
        menu: tk.Menu,
        x_root: int,
        y_root: int,
        generation: int | None = None,
        *,
        is_retry: bool = False,
    ) -> None:
        """Post the context menu and verify it remained mapped.

        Under some compositor/event-order combinations, the menu can be posted and
        then immediately unposted by Tk's class-level ``<ButtonRelease>`` menu
        binding. We verify after a short delay and retry once for the active click.
        """
        if generation is None:
            generation = self._menu_post_generation
        msg = f"[FileExplorer] _post_menu: posting menu {id(menu)} at ({x_root}, {y_root})"
        print(msg)
        _logger.debug(msg)
        try:
            # tk_popup asks Tk to present the menu as a true popup with proper
            # compositor stacking/focus behavior. This is more reliable than a
            # bare post() on some Wayland/XWayland combinations.
            menu.tk_popup(x_root, y_root)
        finally:
            # Do not keep the grab after showing the menu; we only need popup
            # placement/stacking semantics from tk_popup.
            try:
                menu.grab_release()
            except tk.TclError:
                pass
        menu.lift()
        sw = menu.winfo_screenwidth()
        sh = menu.winfo_screenheight()
        mx, my = menu.winfo_x(), menu.winfo_y()
        mw, mh = menu.winfo_width(), menu.winfo_height()
        msg2 = (
            f"[FileExplorer] _post_menu: after lift() → "
            f"ismapped={menu.winfo_ismapped()}, viewable={menu.winfo_viewable()}, "
            f"menu_pos=({mx},{my}), menu_size={mw}x{mh}, screen={sw}x{sh}"
        )
        print(msg2)
        _logger.debug(msg2)

        if not is_retry:
            self.tree.after(
                self._MENU_POST_VERIFY_DELAY_MS,
                lambda m=menu, x=x_root, y=y_root, g=generation: self._verify_menu_visible(m, x, y, g),
            )

    def _verify_menu_visible(self, menu: tk.Menu, x_root: int, y_root: int, generation: int) -> None:
        """Re-post once if the current generation menu has already been unposted."""
        if generation != self._menu_post_generation:
            return
        mapped = bool(menu.winfo_ismapped())
        msg = f"[FileExplorer] _verify_menu_visible: menu {id(menu)} mapped={int(mapped)} generation={generation}"
        print(msg)
        _logger.debug(msg)
        if mapped:
            return
        msg2 = f"[FileExplorer] _verify_menu_visible: retry posting menu {id(menu)} " f"at ({x_root}, {y_root})"
        print(msg2)
        _logger.debug(msg2)
        self._post_menu(menu, x_root, y_root, generation, is_retry=True)

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
