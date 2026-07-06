# Zellij — Configuration Options (vendored reference)

> **Provenance.** Extracted from <https://zellij.dev/documentation/options.html> and
> <https://zellij.dev/documentation/configuration.html> on 2026-07-03 for zellij
> **0.44.3**. Working reference for tuning the AgentX harness; not a byte-verbatim
> mirror. Re-fetch and bump the date on a zellij upgrade.

## Options relevant to a TUI harness

All are **top-level nodes** in `config.kdl` (not inside the layout — layouts cannot
embed configuration; see `creating-a-layout.md`).

| Option | Type | Default | Syntax | Why it matters here |
|--------|------|---------|--------|---------------------|
| `mouse_mode` | bool | `true` | `mouse_mode false` | On → zellij click-to-focus + selection; **Shift+drag still falls through to the terminal's native selection**. Keep on. |
| `copy_on_select` | bool | `true` | `copy_on_select false` | With mouse on, zellij copies on selection. |
| `copy_command` | string | none (OSC 52) | `copy_command "xclip -selection clipboard"` | Override clipboard integration. |
| `default_mode` | string | `"normal"` | `default_mode "locked"` | **Locked → panes hand *all* keys to the TUI** (scrollback, ESC command line, etc.); reach zellij ops via the unlock key (Ctrl+g). The main keyboard lever for a TUI harness. |
| `support_kitty_keyboard_protocol` | bool | `true` (if terminal supports) | `support_kitty_keyboard_protocol true` | Must be on for bubbletea to disambiguate **Shift+Enter** through the multiplexer; if off, AgentX falls back to Alt+Enter/Ctrl+J. |
| `scroll_buffer_size` | int | `10000` | `scroll_buffer_size 10000` | zellij's own scrollback depth. |
| `pane_frames` | bool | `true` | `pane_frames true` | Frame chrome around panes. |
| `show_startup_tips` | bool | `true` | `show_startup_tips true` | Set false to skip the startup tip pane. |
| `default_layout` | string | `"default"` | `default_layout "compact"` | Layout used when none is passed. |

`rounded_corners` and `hide_session_name` nest under `ui { pane_frames { } }`.

## Config discovery + CLI flags

`config.kdl` is discovered in this order:

1. `--config-dir <dir>` flag (dir containing `config.kdl`, `layouts/`, `themes/`, …)
2. `ZELLIJ_CONFIG_DIR` env var
3. `$HOME/.config/zellij` (Linux) / `$HOME/Library/Application Support/org.Zellij-Contributors.Zellij` (macOS)
4. system location (`/etc/zellij`)

`zellij --config <file>` points at a specific config file.

> **Unresolved:** the docs do **not** state whether `--config <file>` *replaces* the
> discovered config wholesale or *merges* with it. Treat it as **replace** until
> proven otherwise — i.e. pointing `ax` at a repo `config.kdl` would drop the user's
> global keybinds/theme unless we reproduce them. This is why the harness currently
> **documents** the recommended settings rather than force-loading its own config.

## Keybinds

`config.kdl` supports a `keybinds clear-defaults=true { … }` block that replaces the
default keymap with an explicit one (the setup this project's author already uses).
Layouts cannot carry keybinds.

## AgentX keymap vs. a default (`normal`) mode

Audit of this project's author config (zellij 0.44.3, `clear-defaults=true`,
`default_mode` unset → `normal`) against the keys the AgentX surfaces bind. In
`normal` mode the `shared_except "locked"` block is live, so those keys are
intercepted before the TUI; **locked** mode passes everything through except the
unlock key (`Ctrl+g`).

Everything AgentX needs passes through in `normal` mode — typing, Enter, Esc,
`Shift+Enter`, `Alt+Enter`/`Ctrl+J` (newline), `Ctrl+C` (quit), `PgUp`/`PgDn`,
arrows, `Ctrl+A/E/L/R`, `Ctrl+Left`/`Ctrl+Right` — **except two aliases that zellij
claims:**

| AgentX binding | Action | zellij `normal` | Mitigation |
|----------------|--------|-----------------|------------|
| `Alt+f` | word-forward in the input (`input.go`) | `Alt f` → ToggleFloatingPanes | primary `Ctrl+Right` is **not** intercepted; use it, or `unbind "Alt f"` |
| `Ctrl+o` | expand/activate element (chat/context) | `Ctrl o` → SwitchToMode "session" | primary `Enter` is **not** intercepted; use it, or `unbind "Ctrl o"` |

Both collisions are on redundant aliases, so AgentX is fully usable in `normal` mode
as-is. To reclaim the two aliases without the sledgehammer of global `default_mode
"locked"`, `unbind` just those two keys in `config.kdl`. `support_kitty_keyboard_protocol`
and `mouse_mode` are already at their (good) defaults in this config, so
`Shift+Enter` passthrough and click-to-focus + Shift+drag selection work without
changes.
