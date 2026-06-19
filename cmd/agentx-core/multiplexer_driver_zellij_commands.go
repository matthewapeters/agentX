package main

// ZellijCommandMappings documents the mapping of tmux commands used in agentX
// to their zellij equivalents. This reference is for Phase 2 implementation.
//
// SESSION MANAGEMENT
// ==================
//
// Tmux: new-session -d -P -F "#{pane_id}" -s <name> -n <window> [args...]
// Zellij: action new-session --session-name <name> -- [args...]
// Context: Creates a detached session in the background and returns pane/session ID.
// Zellij: Use `zellij run` for background execution, or manage sessions via API.
//
// Tmux: attach-session -t <session_name>
// Zellij: zellij attach <session-name>
// Context: Attaches to an existing session interactively.
//
// Tmux: attach-session -r -t <session_name>
// Zellij: zellij attach <session-name> --create-background-tabs
// Context: Read-only attach (Tmux) vs. Zellij connection modes.
//
// Tmux: kill-session -t <session_name>
// Zellij: zellij delete-session <session-name>
// Context: Terminates a session and all its panes.
//
// WINDOW AND PANE MANAGEMENT
// ===========================
//
// Tmux: split-window -P -F "#{pane_id}" -h -p <percent> -t <pane_id> [args...]
// Zellij: action new-pane --direction Right --percent <percent> -- [args...]
// Context: Horizontal split. Tmux returns pane_id; Zellij manages panes via layout/action API.
//
// Tmux: split-window -P -F "#{pane_id}" -v -p <percent> -t <pane_id> [args...]
// Zellij: action new-pane --direction Down --percent <percent> -- [args...]
// Context: Vertical split.
//
// Tmux: select-pane -t <pane_id>
// Zellij: action focus-pane --pane-id <id>
// Context: Sets focus to a pane.
//
// Tmux: select-pane -t <pane_id> -T <title>
// Zellij: action rename-pane --pane-id <id> --name <title>
// Context: Sets pane title. Zellij: action rename-pane.
//
// Tmux: select-window -t <session>:<window_index>
// Zellij: action focus-tab --tab-index <index>
// Context: Selects a window/tab.
//
// QUERY AND DISPLAY
// =================
//
// Tmux: display-message -p "#{session_name}"
// Zellij: (API or env var $ZELLIJ_SESSION_NAME)
// Context: Query current session name. Zellij provides env vars in sessions.
//
// Tmux: display-message -p -t <pane_id> "#{pane_height}"
// Zellij: (API or layout inspection)
// Context: Query pane dimensions.
//
// Tmux: display-message -p -t <pane_id> "#{window_zoomed_flag}"
// Zellij: (Pane state API)
// Context: Query pane zoom state.
//
// PANE OPERATIONS
// ===============
//
// Tmux: resize-pane -t <pane_id> -y <height>
// Zellij: action resize-pane --percent <percent>
// Context: Resize pane height.
//
// Tmux: resize-pane -t <pane_id> -Z
// Zellij: action toggle-pane-fullscreen [--pane-id <id>]
// Context: Toggle zoom (fullscreen) on a pane.
//
// WINDOW OPTIONS
// ==============
//
// Tmux: set-window-option -t <window_target> window-size smallest
// Zellij: (Layout-based sizing, or layout auto-adjustment)
// Context: Set window size mode to "smallest". Zellij uses tiling and resize-unit.
//
// Tmux: select-layout -t <window_target> tiled
// Zellij: action new-pane --direction Right --percent 50 (or similar layout setup)
// Context: Apply tiled layout. Zellij has fewer built-in layouts but supports custom.
//
// INTERACTIVE VS. DETACHED
// ========================
//
// All Tmux -d (detached) flag operations should map to:
// Zellij: action new-session or action new-pane (non-interactive modes).
//
// All interactive operations (attach-session, select-pane for focus) may use:
// Zellij: CLI commands or action API depending on context.
