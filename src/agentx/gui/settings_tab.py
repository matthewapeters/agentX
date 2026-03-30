"""Settings tab widget for interactive agentx.toml editing."""

from __future__ import annotations

import glob
import tkinter as tk
from tkinter import ttk
from typing import Any, Callable

from .collapsible_section import CollapsibleSection
from .markdown_renderer import TKINTERWEB_AVAILABLE

# System-prompt files that should be excluded from user selection (internal only).
_EXCLUDED_SYSTEM_PROMPTS = {"prompt_classification", "structured_response"}

# Keys whose changes are written to disk but do NOT take effect until the app
# is restarted. Each entry is a tuple (section, key) matching the TOML path.
_RESTART_REQUIRED: set[tuple] = {
    ("agentx", "ollama_host"),
    ("agentx", "ollama_model"),  # runtime model controlled by toolbar selector
    ("agentx", "ollama_initial_load_timeout_seconds"),
    ("agentx", "screen_side"),
    ("agentx", "theme_mode"),
    ("agentix", "host"),
    ("agentx", "working_memory", "enabled"),
    ("agentix", "classification_torch_device"),
    ("agentix", "classification_torch_model"),
}

# Keys that are greyed-out unless classification_backend == "torch".
_TORCH_ONLY_KEYS = {"classification_torch_device", "classification_torch_model"}


class SettingsTab:
    """
    Renders all agentx.toml settings as interactive widgets inside the
    ⚙️ Settings notebook tab.

    Each control fires ``on_change(key_path: list[str], value: Any)``
    immediately on interaction.  The caller is responsible for persisting and
    hot-applying the change.

    Widget conventions:
      - bool  → ``tk.Checkbutton``
      - int   → ``ttk.Spinbox``
      - str (model name) → ``ttk.Combobox`` (populated by caller via
        ``populate_models()``)
      - str (enum)       → ``ttk.Combobox`` with fixed choices
      - str (free text)  → ``ttk.Entry``
      - list[str] (flags) → one ``tk.Checkbutton`` per known value

    🔁 labels indicate settings that require an app restart.
    Torch-specific fields are shown greyed when ``classification_backend ≠ torch``.
    """

    RESTART_ICON = " 🔁"

    def __init__(
        self,
        parent: tk.Widget,
        config: dict,
        on_change: Callable[[list[str], Any], None],
        models: list[dict] | None = None,
        system_prompts_dir: str = "system_prompts",
        bg: str = "",
        fg: str = "",
        muted_fg: str = "gray",
    ) -> None:
        self._config = config
        self._on_change = on_change
        self._models: list[str] = self._extract_model_names(models or [])
        self._system_prompts_dir = system_prompts_dir
        self._bg = bg
        self._fg = fg
        self._muted_fg = muted_fg

        self.frame = tk.Frame(parent, bg=bg)

        # Scrollable canvas so sections don't overflow the panel.
        canvas = tk.Canvas(self.frame, borderwidth=0, highlightthickness=0, bg=bg)
        scrollbar = tk.Scrollbar(self.frame, orient="vertical", command=canvas.yview)
        canvas.configure(yscrollcommand=scrollbar.set)
        canvas.pack(side=tk.LEFT, fill=tk.BOTH, expand=True)
        scrollbar.pack(side=tk.RIGHT, fill=tk.Y)

        self._inner = tk.Frame(canvas, bg=bg)
        win_id = canvas.create_window((0, 0), window=self._inner, anchor="nw")

        def _on_inner_configure(event):
            canvas.configure(scrollregion=canvas.bbox("all"))

        def _on_canvas_configure(event):
            canvas.itemconfig(win_id, width=event.width)

        self._inner.bind("<Configure>", _on_inner_configure)
        canvas.bind("<Configure>", _on_canvas_configure)

        def _mousewheel(event):
            canvas.yview_scroll(int(-1 * (event.delta / 120)), "units")

        def _mousewheel_linux(event):
            canvas.yview_scroll(-1 if event.num == 4 else 1, "units")

        canvas.bind("<MouseWheel>", _mousewheel)
        canvas.bind("<Button-4>", _mousewheel_linux)
        canvas.bind("<Button-5>", _mousewheel_linux)

        # References to widgets we may need to update after construction.
        self._torch_widgets: list[tuple[tk.Widget, ...]] = []  # (label, widget)
        self._torch_state_var: tk.StringVar | None = None
        self._theme_mode_var: tk.StringVar | None = None
        self._model_dropdowns: list[ttk.Combobox] = []

        # Build all sections.
        self._build_appearance_section()
        self._build_ollama_section()
        self._build_agentix_section()
        self._build_classification_display_section()
        self._build_working_memory_section()

        # Apply initial torch-field greyout.
        self._apply_torch_greyout()

    # ──────────────────────────────────────────────────────────────────────────
    # Public API
    # ──────────────────────────────────────────────────────────────────────────

    def populate_models(self, models: list[dict]) -> None:
        """Refresh the models list in all model-selection dropdowns."""
        self._models = self._extract_model_names(models)
        for combo in self._model_dropdowns:
            combo["values"] = self._models

    # ──────────────────────────────────────────────────────────────────────────
    # Section builders
    # ──────────────────────────────────────────────────────────────────────────

    def _build_appearance_section(self) -> None:
        cfg = self._config.get("agentx", {})
        section = self._make_section("🎨 Appearance", initial_collapsed=False)
        g = section.content_container

        self._theme_mode_var = self._add_enum_dropdown(
            g,
            0,
            ["agentx", "theme_mode"],
            "Theme mode" + self.RESTART_ICON,
            cfg.get("theme_mode", "Dark Mode"),
            ["Dark Mode", "Light Mode"],
            hot_reload=False,
        )

        # Render Markdown toggle — disabled with an explanatory label when tkinterweb
        # is not installed so users understand why the option is greyed out.
        md_initial = cfg.get("markdown_render_enabled", True)
        self._markdown_render_var = tk.BooleanVar(value=md_initial if TKINTERWEB_AVAILABLE else False)

        def _on_markdown_toggle():
            self._fire(["agentx", "markdown_render_enabled"], self._markdown_render_var.get())

        md_cb = tk.Checkbutton(
            g,
            text="Render Markdown",
            variable=self._markdown_render_var,
            command=_on_markdown_toggle,
            anchor="w",
            bg=self._bg,
            fg=self._fg,
            activebackground=self._bg,
            activeforeground=self._fg,
            selectcolor=self._bg,
            font=("Terminal", 9),
        )
        md_cb.grid(row=1, column=0, columnspan=2, sticky="w", padx=(8, 4), pady=2)

        if not TKINTERWEB_AVAILABLE:
            md_cb.config(state=tk.DISABLED)
            tk.Label(
                g,
                text="(requires tkinterweb)",
                bg=self._bg,
                fg=self._muted_fg,
                font=("Terminal", 8, "italic"),
            ).grid(row=1, column=2, sticky="w", padx=(0, 4), pady=2)

    def _build_ollama_section(self) -> None:
        cfg = self._config.get("agentx", {})
        section = self._make_section("🤖 Ollama", initial_collapsed=False)
        g = section.content_container

        self._add_text_entry(
            g, 0, ["agentx", "ollama_host"], "Host", cfg.get("ollama_host", "localhost:11434"), restart=True
        )

        # Default model: dropdown but NOT wired to active_model at runtime.
        self._add_model_dropdown(
            g,
            1,
            ["agentx", "ollama_model"],
            "Default model" + self.RESTART_ICON,
            cfg.get("ollama_model", ""),
            hot_reload=False,
        )

        self._add_spinbox(
            g,
            2,
            ["agentx", "ollama_initial_load_timeout_seconds"],
            "Load timeout (s)",
            cfg.get("ollama_initial_load_timeout_seconds", 120),
            from_=5,
            to=600,
            restart=True,
        )

        self._add_enum_dropdown(
            g,
            3,
            ["agentx", "screen_side"],
            "Screen side" + self.RESTART_ICON,
            cfg.get("screen_side", "left"),
            ["left", "right"],
        )

    def _build_agentix_section(self) -> None:
        cfg = self._config.get("agentix", {})
        section = self._make_section("🧠 Agentix", initial_collapsed=False)
        g = section.content_container

        self._add_text_entry(
            g, 0, ["agentix", "host"], "Host" + self.RESTART_ICON, cfg.get("host", "localhost:8000"), restart=True
        )

        self._add_checkbox(g, 1, ["agentix", "classify_prompts"], "Classify prompts", cfg.get("classify_prompts", True))

        self._add_checkbox(g, 2, ["agentix", "debug"], "Debug logging", cfg.get("debug", False))

        # Classification sub-group
        self._add_separator(g, 3, "Classification")

        backend_var = self._add_enum_dropdown(
            g,
            4,
            ["agentix", "classification_backend"],
            "Backend",
            cfg.get("classification_backend", "ollama"),
            ["ollama", "torch"],
        )
        self._torch_state_var = backend_var
        backend_var.trace_add("write", lambda *_: self._apply_torch_greyout())

        self._add_model_dropdown(
            g,
            5,
            ["agentix", "agentix_bench_classification_model"],
            "Classification model",
            cfg.get("agentix_bench_classification_model", ""),
            hot_reload=True,
        )

        torch_lbl_a, torch_entry_a = self._add_text_entry(
            g,
            6,
            ["agentix", "classification_torch_model"],
            "Torch model" + self.RESTART_ICON,
            cfg.get("classification_torch_model", ""),
            restart=True,
            return_widgets=True,
        )
        self._torch_widgets.append((torch_lbl_a, torch_entry_a))

        torch_lbl_b, torch_spin_b = self._add_spinbox(
            g,
            7,
            ["agentix", "classification_torch_device"],
            "Torch device" + self.RESTART_ICON,
            cfg.get("classification_torch_device", -1),
            from_=-1,
            to=16,
            restart=True,
            return_widgets=True,
        )
        self._torch_widgets.append((torch_lbl_b, torch_spin_b))

        # System prompts multi-checkbox
        self._add_separator(g, 8, "System prompts (tool calls)")
        available = self._discover_system_prompts()
        enabled_set = set(cfg.get("default_system_prompts", []))
        row = 9
        for prompt_name in available:
            self._add_list_checkbox(
                g,
                row,
                ["agentix", "default_system_prompts"],
                prompt_name,
                prompt_name in enabled_set,
                all_values=available,
                current_enabled=enabled_set,
            )
            row += 1

    def _build_classification_display_section(self) -> None:
        cfg = self._config.get("agentix", {}).get("classification_display", {})
        section = self._make_section("📊 Classification Display", initial_collapsed=True)
        g = section.content_container

        fields = [
            ("enabled", "Show classification block"),
            ("show_intent", "Show intent"),
            ("show_reasoning", "Show reasoning"),
            ("show_clarification", "Show clarification info"),
            ("show_next_step", "Show routing path"),
        ]
        for row, (key, label) in enumerate(fields):
            self._add_checkbox(g, row, ["agentix", "classification_display", key], label, cfg.get(key, True))

    def _build_working_memory_section(self) -> None:
        cfg = self._config.get("agentx", {}).get("working_memory", {})
        section = self._make_section("🏛️ Working Memory", initial_collapsed=True)
        g = section.content_container

        self._add_checkbox(
            g,
            0,
            ["agentx", "working_memory", "enabled"],
            "Enabled" + self.RESTART_ICON,
            cfg.get("enabled", True),
            restart=True,
        )

        self._add_checkbox(
            g,
            1,
            ["agentx", "working_memory", "inject_into_context"],
            "Inject into LLM context",
            cfg.get("inject_into_context", True),
        )

        self._add_spinbox(
            g,
            2,
            ["agentx", "working_memory", "max_facts"],
            "Max facts (0 = unlimited)",
            cfg.get("max_facts", 50),
            from_=0,
            to=500,
        )

    # ──────────────────────────────────────────────────────────────────────────
    # Widget factory helpers
    # ──────────────────────────────────────────────────────────────────────────

    def _make_section(self, title: str, initial_collapsed: bool = True) -> CollapsibleSection:
        section = CollapsibleSection(
            self._inner,
            title,
            initial_collapsed=initial_collapsed,
            font=("Terminal", 10, "bold"),
            bg=self._bg,
            fg=self._fg,
        )
        section.get_widget().pack(fill=tk.X, padx=4, pady=(4, 0))
        section.content_container.columnconfigure(1, weight=1)
        return section

    def _add_label(self, parent: tk.Widget, row: int, text: str) -> tk.Label:
        lbl = tk.Label(
            parent,
            text=text,
            anchor="w",
            bg=self._bg,
            fg=self._fg,
            font=("Terminal", 9),
        )
        lbl.grid(row=row, column=0, sticky="w", padx=(8, 4), pady=2)
        return lbl

    def _add_separator(self, parent: tk.Widget, row: int, text: str) -> None:
        sep_lbl = tk.Label(
            parent, text=text, anchor="w", bg=self._bg, font=("Terminal", 9, "italic"), foreground=self._muted_fg
        )
        sep_lbl.grid(row=row, column=0, columnspan=2, sticky="w", padx=(8, 4), pady=(6, 0))

    def _add_checkbox(
        self,
        parent: tk.Widget,
        row: int,
        key_path: list[str],
        label: str,
        initial: bool,
        restart: bool = False,
    ) -> tk.BooleanVar:
        var = tk.BooleanVar(value=initial)

        def _on_change():
            self._fire(key_path, var.get())

        display = label + (self.RESTART_ICON if restart else "")
        cb = tk.Checkbutton(
            parent,
            text=display,
            variable=var,
            command=_on_change,
            anchor="w",
            bg=self._bg,
            fg=self._fg,
            activebackground=self._bg,
            activeforeground=self._fg,
            selectcolor=self._bg,
            font=("Terminal", 9),
        )
        cb.grid(row=row, column=0, columnspan=2, sticky="w", padx=(8, 4), pady=2)
        return var

    def _add_text_entry(
        self,
        parent: tk.Widget,
        row: int,
        key_path: list[str],
        label: str,
        initial: str,
        restart: bool = False,
        return_widgets: bool = False,
    ):
        lbl = self._add_label(parent, row, label)
        var = tk.StringVar(value=str(initial))

        def _commit(*_):
            self._fire(key_path, var.get())

        entry = ttk.Entry(parent, textvariable=var, width=28)
        entry.grid(row=row, column=1, sticky="ew", padx=(0, 8), pady=2)
        entry.bind("<FocusOut>", _commit)
        entry.bind("<Return>", _commit)
        if return_widgets:
            return lbl, entry
        return var

    def _add_spinbox(
        self,
        parent: tk.Widget,
        row: int,
        key_path: list[str],
        label: str,
        initial: int,
        from_: int = 0,
        to: int = 9999,
        restart: bool = False,
        return_widgets: bool = False,
    ):
        lbl = self._add_label(parent, row, label)
        var = tk.StringVar(value=str(initial))

        def _commit(*_):
            try:
                self._fire(key_path, int(var.get()))
            except ValueError:
                pass

        spin = ttk.Spinbox(parent, from_=from_, to=to, textvariable=var, width=8, command=_commit)
        spin.grid(row=row, column=1, sticky="w", padx=(0, 8), pady=2)
        spin.bind("<FocusOut>", _commit)
        spin.bind("<Return>", _commit)
        if return_widgets:
            return lbl, spin
        return var

    def _add_enum_dropdown(
        self,
        parent: tk.Widget,
        row: int,
        key_path: list[str],
        label: str,
        initial: str,
        choices: list[str],
        hot_reload: bool = True,
    ) -> tk.StringVar:
        self._add_label(parent, row, label)
        var = tk.StringVar(value=initial)

        def _on_change(*_):
            if hot_reload:
                self._fire(key_path, var.get())
            else:
                self._fire_config_only(key_path, var.get())

        combo = ttk.Combobox(parent, textvariable=var, values=choices, state="readonly", width=20)
        combo.grid(row=row, column=1, sticky="w", padx=(0, 8), pady=2)
        var.trace_add("write", _on_change)
        return var

    def _add_model_dropdown(
        self,
        parent: tk.Widget,
        row: int,
        key_path: list[str],
        label: str,
        initial: str,
        hot_reload: bool = True,
    ) -> tk.StringVar:
        self._add_label(parent, row, label)
        var = tk.StringVar(value=initial)

        def _on_change(*_):
            if hot_reload:
                self._fire(key_path, var.get())
            else:
                # Config-only: write to disk but do NOT change active runtime state.
                self._fire_config_only(key_path, var.get())

        combo = ttk.Combobox(parent, textvariable=var, values=self._models, state="readonly", width=25)
        combo.grid(row=row, column=1, sticky="w", padx=(0, 8), pady=2)
        self._model_dropdowns.append(combo)
        var.trace_add("write", _on_change)
        return var

    def _add_list_checkbox(
        self,
        parent: tk.Widget,
        row: int,
        key_path: list[str],
        value: str,
        initial: bool,
        all_values: list[str],
        current_enabled: set[str],
    ) -> tk.BooleanVar:
        enabled_set = set(current_enabled)
        var = tk.BooleanVar(value=initial)

        def _on_change():
            if var.get():
                enabled_set.add(value)
            else:
                enabled_set.discard(value)
            self._fire(key_path, [v for v in all_values if v in enabled_set])

        cb = tk.Checkbutton(
            parent,
            text=value,
            variable=var,
            command=_on_change,
            anchor="w",
            bg=self._bg,
            fg=self._fg,
            activebackground=self._bg,
            activeforeground=self._fg,
            selectcolor=self._bg,
            font=("Terminal", 9),
        )
        cb.grid(row=row, column=0, columnspan=2, sticky="w", padx=(20, 4), pady=1)
        return var

    # ──────────────────────────────────────────────────────────────────────────
    # Internal helpers
    # ──────────────────────────────────────────────────────────────────────────

    def _fire(self, key_path: list[str], value: Any) -> None:
        """Notify caller: this change should be hot-applied AND persisted."""
        try:
            self._on_change(key_path, value)
        except Exception:
            pass  # never crash the GUI

    def _fire_config_only(self, key_path: list[str], value: Any) -> None:
        """Notify caller: persist this change but do NOT hot-apply at runtime."""
        # Signal config-only by prepending a sentinel key "__config_only__".
        try:
            self._on_change(["__config_only__"] + key_path, value)
        except Exception:
            pass

    def _apply_torch_greyout(self) -> None:
        """Enable or disable torch-only fields based on current backend selection."""
        if self._torch_state_var is None:
            return
        is_torch = self._torch_state_var.get() == "torch"
        state = tk.NORMAL if is_torch else tk.DISABLED
        fg = "" if is_torch else "gray"
        for lbl, widget in self._torch_widgets:
            try:
                lbl.config(foreground=fg)
                widget.config(state=state)
            except Exception:
                pass

    def _discover_system_prompts(self) -> list[str]:
        """Return sorted list of user-selectable system prompt names."""
        patterns = [
            f"{self._system_prompts_dir}/*.md",
            f"{self._system_prompts_dir}/*.txt",
        ]
        names = []
        for pat in patterns:
            for path in glob.glob(pat):
                name = path.split("/")[-1].rsplit(".", 1)[0]
                if name not in _EXCLUDED_SYSTEM_PROMPTS:
                    names.append(name)
        return sorted(set(names))

    @staticmethod
    def _extract_model_names(models: list[dict]) -> list[str]:
        return [m.get("name", str(m)) for m in models if isinstance(m, dict)]
