#!/usr/bin/env bash
# =============================================================================
# launch_vibe.sh — AgentX Vibe Coding Launcher
# =============================================================================
# Starts a tmux session with:
#   - window 0, pane 0.0: neovim with RPC socket exposed at /tmp/agentx.nvim.sock
#   - window 1 (agent-bg): persistent agent shell + ephemeral agent terminal panes
#   - window 2 (agentx-log): AgentX runtime process + its stdout/stderr logs
#   - AgentX GUI launched as a floating Tkinter window (separate from tmux)
#
# Usage:
#   ./launch_vibe.sh [start] [project_dir]
#   ./launch_vibe.sh stop
#   ./launch_vibe.sh status
#   ./launch_vibe.sh recover-editor [project_dir]
#   ./launch_vibe.sh restart [project_dir]
#
#   project_dir  Path to your project root (default: current directory)
#
# Environment overrides:
#   AGENTX_NVIM_SOCKET      Path for nvim RPC socket (default: /tmp/agentx_<SESSION_ID>.nvim.sock)
#   AGENTX_SAVES_FIFO       Path for save-notification pipe (default: /tmp/agentx_<SESSION_ID>.saves.fifo)
#   AGENTX_TMUX_SESSION     tmux session name (default: agentx) — used as SESSION_ID for socket scoping
#   AGENTX_TERMINAL_VISIBLE Default visibility for agent terminal panes (default: true)
#   AGENTX_PYTHON           Python executable to use (default: resolved from venv or PATH)
#   AGENTX_SOCKET_WAIT_LOOPS Number of nvim socket polling loops (default: 10)
#   AGENTX_SOCKET_WAIT_SEC   Seconds per socket polling loop (default: 0.5)
#
# Multi-session support:
#   Socket and FIFO paths are automatically scoped by tmux session name to prevent collisions
#   when multiple sessions are running. Stale sockets/FIFOs are detected and cleaned up on start.
#
# Requirements:
#   - tmux >= 2.9
#   - nvim (neovim) >= 0.5  (--listen flag)
#   - python >= 3.12 with AgentX installed (uv sync or pip install -e .)
# =============================================================================

set -euo pipefail

# ── Colour helpers ────────────────────────────────────────────────────────────
_red()    { printf '\033[0;31m%s\033[0m\n' "$*"; }
_green()  { printf '\033[0;32m%s\033[0m\n' "$*"; }
_yellow() { printf '\033[0;33m%s\033[0m\n' "$*"; }
_blue()   { printf '\033[0;34m%s\033[0m\n' "$*"; }
_bold()   { printf '\033[1m%s\033[0m\n' "$*"; }

_usage() {
        cat << 'USAGE'
Usage:
    ./launch_vibe.sh [start] [project_dir]
    ./launch_vibe.sh stop
    ./launch_vibe.sh status
    ./launch_vibe.sh recover-editor [project_dir]
    ./launch_vibe.sh restart [project_dir]

Commands:
    start           Start a vibe session (default command)
    stop            Gracefully stop AgentX + neovim and kill tmux session
    status          Show current session/windows/socket status
    recover-editor  Recreate or restart neovim in the editor window
    restart         Equivalent to: stop then start
USAGE
}

# ── Configuration (overridable via environment) ──────────────────────────────
TMUX_SESSION="${AGENTX_TMUX_SESSION:-agentx}"
# Scope socket and FIFO paths by session ID to avoid collisions between multiple sessions
SESSION_ID="${TMUX_SESSION}"
NVIM_SOCKET="${AGENTX_NVIM_SOCKET:-/tmp/agentx_${SESSION_ID}.nvim.sock}"
SAVES_FIFO="${AGENTX_SAVES_FIFO:-/tmp/agentx_${SESSION_ID}.saves.fifo}"
TERMINAL_VISIBLE="${AGENTX_TERMINAL_VISIBLE:-true}"
SOCKET_WAIT_LOOPS="${AGENTX_SOCKET_WAIT_LOOPS:-10}"
SOCKET_WAIT_SEC="${AGENTX_SOCKET_WAIT_SEC:-0.5}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EDITOR_WINDOW_NAME="editor"
AGENT_BG_WINDOW_NAME="agent-bg"
AGENTX_LOG_WINDOW_NAME="agentx-log"
EDITOR_PANE_ID=""
AGENT_BG_PANE_ID=""
AGENTX_LOG_PANE_ID=""

COMMAND="${1:-start}"
PROJECT_DIR="${PWD}"

case "$COMMAND" in
    start|stop|status|recover-editor|restart)
        shift || true
        PROJECT_DIR="${1:-$PWD}"
        ;;
    -h|--help|help)
        _usage
        exit 0
        ;;
    *)
        # Backward compatibility: first arg can be project_dir
        COMMAND="start"
        PROJECT_DIR="$1"
        ;;
esac

if [[ -z "$PROJECT_DIR" ]]; then
    PROJECT_DIR="$PWD"
fi

# ── Helpers ──────────────────────────────────────────────────────────────────
_resolve_python() {
    if [[ -n "${AGENTX_PYTHON:-}" ]]; then
        echo "$AGENTX_PYTHON"
    elif [[ -f "$SCRIPT_DIR/.venv/bin/python" ]]; then
        echo "$SCRIPT_DIR/.venv/bin/python"
    elif [[ -f "$PROJECT_DIR/.venv/bin/python" ]]; then
        echo "$PROJECT_DIR/.venv/bin/python"
    elif command -v python3 &>/dev/null; then
        echo "python3"
    else
        return 1
    fi
}

_check_dep() {
    local cmd="$1"
    local install_hint="$2"
    if ! command -v "$cmd" &>/dev/null; then
        _red "  ✗ $cmd — not found"
        _red "    Install hint: $install_hint"
        return 1
    else
        _green "  ✓ $cmd  ($(command -v "$cmd"))"
        return 0
    fi
}

_check_start_dependencies() {
    local python_bin="$1"
    local dep_ok=true
    _check_dep tmux  "sudo apt install tmux  /  brew install tmux" || dep_ok=false
    _check_dep nvim  "sudo apt install neovim  /  brew install neovim" || dep_ok=false
    _check_dep "$python_bin" "uv sync  /  pip install -e ." || dep_ok=false
    if [[ "$dep_ok" == "false" ]]; then
        echo ""
        _red "One or more required dependencies are missing. Aborting."
        exit 1
    fi
}

_check_minimal_dependencies() {
    local need_nvim="$1"
    local dep_ok=true
    _check_dep tmux  "sudo apt install tmux  /  brew install tmux" || dep_ok=false
    if [[ "$need_nvim" == "true" ]]; then
        _check_dep nvim  "sudo apt install neovim  /  brew install neovim" || dep_ok=false
    fi
    if [[ "$dep_ok" == "false" ]]; then
        echo ""
        _red "One or more required dependencies are missing. Aborting."
        exit 1
    fi
}

_verify_nvim_version() {
    local nvim_version
    local nvim_major
    local nvim_minor
    nvim_version=$(nvim --version | head -1 | grep -oP '\d+\.\d+\.\d+' | head -1)
    nvim_major=$(echo "$nvim_version" | cut -d. -f1)
    nvim_minor=$(echo "$nvim_version" | cut -d. -f2)
    if [[ "$nvim_major" -lt 1 && "$nvim_minor" -lt 5 ]]; then
        _yellow "  ⚠ neovim $nvim_version detected — version >= 0.5 required for --listen flag"
        _yellow "    Upgrade: https://github.com/neovim/neovim/releases"
        exit 1
    fi
}

_session_exists() {
    tmux has-session -t "$TMUX_SESSION" 2>/dev/null
}

_window_exists_by_name() {
    local name="$1"
    tmux list-windows -t "$TMUX_SESSION" -F '#{window_name}' 2>/dev/null | grep -qx "$name"
}

_normalize_token() {
    local token="$1"
    # Strip CR/LF and surrounding whitespace that may appear in command substitutions
    token="${token//$'\r'/}"
    token="${token//$'\n'/}"
    token="${token#${token%%[![:space:]]*}}"
    token="${token%${token##*[![:space:]]}}"
    echo "$token"
}

_pane_exists() {
    local pane_id="$1"
    pane_id="$(_normalize_token "$pane_id")"
    [[ -n "$pane_id" ]] || return 1
    tmux list-panes -a -F '#{pane_id}' 2>/dev/null | grep -Fqx -- "$pane_id"
}

_first_pane_target() {
    local pane_id
    pane_id="$(tmux list-panes -t "$TMUX_SESSION" -F '#{pane_id}' 2>/dev/null | head -1 || true)"
    pane_id="$(_normalize_token "$pane_id")"
    echo "$pane_id"
}

_window_index_by_name() {
    local name="$1"
    tmux list-windows -t "$TMUX_SESSION" -F '#{window_index}:#{window_name}' 2>/dev/null | \
    awk -F: -v target="$name" '$2 == target {print $1; found=1; exit} END {if (!found) print ""}' || true
}

_window_target_by_name() {
    local name="$1"
    local idx
    idx="$(_window_index_by_name "$name")"
    if [[ -z "$idx" ]]; then
        echo ""
        return 1
    fi
    echo "${TMUX_SESSION}:${idx}"
}

_discover_nvim_pane_id() {
    tmux list-panes -t "$TMUX_SESSION" -F '#{pane_id} #{pane_current_command}' 2>/dev/null | \
        awk '$2 == "nvim" {print $1; exit}'
}

_editor_pane_target() {
    if [[ -n "$EDITOR_PANE_ID" ]]; then
        echo "$EDITOR_PANE_ID"
        return 0
    fi

    local pane_id
    pane_id="$(_discover_nvim_pane_id)"
    if [[ -n "$pane_id" ]]; then
        EDITOR_PANE_ID="$pane_id"
        echo "$pane_id"
        return 0
    fi

    # In this launcher the editor starts as the first pane in the session.
    # Prefer that stable position before falling back to the window name,
    # because tmux may rename the window away from "editor" after nvim starts.
    pane_id="$(_first_pane_target)"
    if [[ -n "$pane_id" ]]; then
        EDITOR_PANE_ID="$pane_id"
        echo "$pane_id"
        return 0
    fi

    local target
    target="$(_window_target_by_name "$EDITOR_WINDOW_NAME")" || true
    if [[ -n "$target" ]]; then
        pane_id="$(tmux display-message -p -t "${target}.0" '#{pane_id}' 2>/dev/null || true)"
        pane_id="$(_normalize_token "$pane_id")"
        if [[ -n "$pane_id" ]]; then
            EDITOR_PANE_ID="$pane_id"
            echo "$pane_id"
            return 0
        fi
    fi

    echo ""
}

_agent_bg_pane_target() {
    if _pane_exists "$AGENT_BG_PANE_ID"; then
        echo "$AGENT_BG_PANE_ID"
        return 0
    fi

    local pane_id
    local target
    target="$(_window_target_by_name "$AGENT_BG_WINDOW_NAME")" || true
    if [[ -n "$target" ]]; then
        pane_id="$(tmux display-message -p -t "${target}.0" '#{pane_id}' 2>/dev/null || true)"
        pane_id="$(_normalize_token "$pane_id")"
        if _pane_exists "$pane_id"; then
            AGENT_BG_PANE_ID="$pane_id"
            echo "$pane_id"
            return 0
        fi
    fi

    echo ""
}

_agentx_log_window_target() {
    if _pane_exists "$AGENTX_LOG_PANE_ID"; then
        tmux display-message -p -t "$AGENTX_LOG_PANE_ID" '#{session_name}:#{window_index}' 2>/dev/null || echo ""
        return 0
    fi

    local target
    target="$(_window_target_by_name "$AGENTX_LOG_WINDOW_NAME")" || true
    [[ -n "$target" ]] && echo "$target" || echo ""
}

_agentx_log_pane_target() {
    if _pane_exists "$AGENTX_LOG_PANE_ID"; then
        echo "$AGENTX_LOG_PANE_ID"
        return 0
    fi

    local pane_id
    local target
    target="$(_window_target_by_name "$AGENTX_LOG_WINDOW_NAME")" || true
    if [[ -n "$target" ]]; then
        pane_id="$(tmux display-message -p -t "${target}.0" '#{pane_id}' 2>/dev/null || true)"
        pane_id="$(_normalize_token "$pane_id")"
        if _pane_exists "$pane_id"; then
            AGENTX_LOG_PANE_ID="$pane_id"
            echo "$pane_id"
            return 0
        fi
    fi

    echo ""
}

_pane_current_command() {
    local pane_target
    pane_target="$(_editor_pane_target)"
    [[ -n "$pane_target" ]] || { echo ""; return 0; }
    tmux display-message -p -t "$pane_target" '#{pane_current_command}' 2>/dev/null || echo ""
}

_cleanup_stale_sockets() {
    # Remove stale socket if it exists and no live neovim process is using it.
    if [[ -S "$NVIM_SOCKET" ]]; then
        # Try to ping the socket with a simple nvim --remote-expr call (with timeout)
        if ! timeout 1 nvim --remote-expr "1" --server "$NVIM_SOCKET" >/dev/null 2>&1; then
            _yellow "  Removing stale neovim socket: $NVIM_SOCKET"
            rm -f "$NVIM_SOCKET"
        fi
    fi
    # Always remove old FIFO to ensure fresh notifications
    if [[ -p "$SAVES_FIFO" ]]; then
        _yellow "  Removing stale save-notification FIFO: $SAVES_FIFO"
        rm -f "$SAVES_FIFO"
    fi
}

_write_nvimrc() {
    local nvimrc_path="$PROJECT_DIR/.nvimrc.agentx"
    cat > "$nvimrc_path" << NVIMRC
" AgentX vibe-coding autocommands
" Auto-generated by launch_vibe.sh — modifications will be overwritten on next launch.
" This file is sourced alongside your personal neovim config.
augroup agentx_vibe
  autocmd!
  " Notify AgentX when any buffer is written to disk.
  autocmd BufWritePost * silent! call writefile([expand('%:p')], '${SAVES_FIFO}', 'a')
augroup END
NVIMRC
    _green "  Wrote: $nvimrc_path"

    local gitignore="$PROJECT_DIR/.gitignore"
    if [[ -f "$gitignore" ]] && ! grep -qxF '.nvimrc.agentx' "$gitignore"; then
        echo '.nvimrc.agentx' >> "$gitignore"
        _green "  Added .nvimrc.agentx to .gitignore"
    fi
}

_launch_editor_in_pane_zero() {
    local nvimrc_path="$PROJECT_DIR/.nvimrc.agentx"
    local pane_target
    pane_target="$(_editor_pane_target)"
    if [[ -z "$pane_target" ]]; then
        _red "  ✗ unable to find '${EDITOR_WINDOW_NAME}' window to launch neovim."
        return 1
    fi
    [[ -S "$NVIM_SOCKET" ]] && rm -f "$NVIM_SOCKET"
    tmux send-keys -t "$pane_target" C-c
    tmux send-keys -t "$pane_target" \
        "nvim --listen '${NVIM_SOCKET}' --cmd 'source ${nvimrc_path}'" \
        Enter
    _green "  Launched neovim in '${EDITOR_WINDOW_NAME}' pane 0 (socket: $NVIM_SOCKET)"
}

_wait_for_socket() {
    local waited=0
    while [[ ! -S "$NVIM_SOCKET" && "$waited" -lt "$SOCKET_WAIT_LOOPS" ]]; do
        sleep "$SOCKET_WAIT_SEC"
        waited=$((waited + 1))
    done
}

_ensure_editor_running() {
    local cmd=""
    local attempt=1
    while [[ "$attempt" -le 3 ]]; do
        cmd="$(_pane_current_command)"
        if [[ "$cmd" == "nvim" ]]; then
            return 0
        fi
        if [[ "$attempt" -gt 1 ]]; then
            _yellow "  Editor pane command is '$cmd' (expected nvim). Relaunch attempt $attempt/3..."
            _launch_editor_in_pane_zero
            _wait_for_socket
        else
            _wait_for_socket
        fi
        attempt=$((attempt + 1))
    done
    _yellow "  ⚠ unable to confirm neovim in pane 0.0. Pane command: '$cmd'."
    return 1
}

_stop_session() {
    if ! _session_exists; then
        _yellow "No active tmux session '$TMUX_SESSION' to stop."
        return 0
    fi

    echo "Stopping session '$TMUX_SESSION'..."

    local agentx_log_pane_target
    local editor_pane_target
    agentx_log_pane_target="$(_agentx_log_pane_target)"
    editor_pane_target="$(_editor_pane_target)"

    if [[ -n "$agentx_log_pane_target" ]]; then
        tmux send-keys -t "$agentx_log_pane_target" C-c
    fi

    if [[ -n "$editor_pane_target" ]]; then
        tmux send-keys -t "$editor_pane_target" C-c
        tmux send-keys -t "$editor_pane_target" ":qa!" Enter
    fi

    tmux kill-session -t "$TMUX_SESSION"
    [[ -S "$NVIM_SOCKET" ]] && rm -f "$NVIM_SOCKET"
    [[ -p "$SAVES_FIFO" ]] && rm -f "$SAVES_FIFO"
    _green "Session '$TMUX_SESSION' stopped and cleaned up (socket and FIFO removed)."
}

_show_status() {
    echo "Session   : $TMUX_SESSION"
    echo "Project   : $PROJECT_DIR"
    echo "Socket    : $NVIM_SOCKET"
    echo "FIFO      : $SAVES_FIFO"
    if _session_exists; then
        _green "State     : RUNNING"
        echo ""
        echo "Windows:"
        tmux list-windows -t "$TMUX_SESSION" -F '  - [#{window_index}] #{window_name} (panes=#{window_panes})'
    else
        _yellow "State     : STOPPED"
    fi
    if [[ -S "$NVIM_SOCKET" ]]; then
        _green "Socket    : present"
    else
        _yellow "Socket    : missing"
    fi
}

_recover_editor() {
    if ! _session_exists; then
        _red "Cannot recover editor: tmux session '$TMUX_SESSION' is not running."
        _red "Start a session first with: ./launch_vibe.sh start '$PROJECT_DIR'"
        exit 1
    fi

    if ! _window_exists_by_name "$EDITOR_WINDOW_NAME"; then
        EDITOR_PANE_ID="$(_normalize_token "$(tmux new-window -P -F '#{pane_id}' -d -t "$TMUX_SESSION" -n "$EDITOR_WINDOW_NAME" -c "$PROJECT_DIR")")"
        tmux set-window-option -t "${TMUX_SESSION}:${EDITOR_WINDOW_NAME}" automatic-rename off 2>/dev/null || true
        _green "  Recreated '${EDITOR_WINDOW_NAME}' window."
    else
        EDITOR_PANE_ID="$(_editor_pane_target)"
    fi

    _write_nvimrc
    _launch_editor_in_pane_zero
    _ensure_editor_running || true
    _green "Editor recovered. Use Ctrl+B, w and select '${EDITOR_WINDOW_NAME}' to return to neovim."
}

_start_session() {
    local python_bin="$1"

    # Verify project dir exists
    if [[ ! -d "$PROJECT_DIR" ]]; then
        _red "ERROR: project_dir '$PROJECT_DIR' does not exist."
        exit 1
    fi

    echo ""
    echo "Project dir : $(_blue "$PROJECT_DIR")"
    echo "nvim socket : $(_blue "$NVIM_SOCKET")"
    echo "saves FIFO  : $(_blue "$SAVES_FIFO")"
    echo "tmux session: $(_blue "$TMUX_SESSION")"
    echo "Python      : $(_blue "$python_bin")"
    echo ""

    if _session_exists; then
        _yellow "tmux session '$TMUX_SESSION' already exists."
        echo ""
        echo "Options:"
        echo "  [r] Reattach to existing session"
        echo "  [k] Kill existing session and start fresh"
        echo "  [s] Stop existing session and exit"
        echo "  [a] Abort"
        echo ""
        read -r -p "Choice [r/k/s/a]: " choice
        case "$choice" in
            r|R)
                if [[ ! -S "$NVIM_SOCKET" || "$(_pane_current_command)" != "nvim" ]]; then
                    _yellow "Session is running but editor is not healthy; recovering editor before attach..."
                    _recover_editor
                fi
                _green "Reattaching to existing session..."
                exec tmux attach -t "$TMUX_SESSION"
                ;;
            k|K)
                _yellow "Killing existing session '$TMUX_SESSION'..."
                tmux kill-session -t "$TMUX_SESSION"
                [[ -S "$NVIM_SOCKET" ]] && rm -f "$NVIM_SOCKET"
                ;;
            s|S)
                _stop_session
                exit 0
                ;;
            *)
                echo "Aborted."
                exit 0
                ;;
        esac
    fi

    echo "Setting up IPC..."
    # Detect and cleanup stale sockets/FIFOs from previous sessions
    _cleanup_stale_sockets

    if [[ ! -p "$SAVES_FIFO" ]]; then
        mkfifo "$SAVES_FIFO"
        _green "  Created FIFO: $SAVES_FIFO"
    else
        _green "  FIFO exists: $SAVES_FIFO"
    fi

    _write_nvimrc

    echo ""
    echo "Creating tmux session '$TMUX_SESSION'..."
    local editor_pane_target
    editor_pane_target="$(tmux new-session -d -P -F '#{pane_id}' -s "$TMUX_SESSION" -n "$EDITOR_WINDOW_NAME" -c "$PROJECT_DIR")"
    EDITOR_PANE_ID="$(_normalize_token "$editor_pane_target")"
    # Prevent tmux from renaming the editor window after nvim starts — this keeps
    # _window_target_by_name("editor") reliable throughout the session lifetime.
    tmux set-window-option -t "${TMUX_SESSION}:${EDITOR_WINDOW_NAME}" automatic-rename off 2>/dev/null || true

    local agent_bg_pane_target
    if ! _window_exists_by_name "$AGENT_BG_WINDOW_NAME"; then
        agent_bg_pane_target="$(tmux new-window -P -F '#{pane_id}' -t "$TMUX_SESSION" -n "$AGENT_BG_WINDOW_NAME" -d -c "$PROJECT_DIR")"
    else
        agent_bg_pane_target="$(_agent_bg_pane_target)"
    fi
    AGENT_BG_PANE_ID="$agent_bg_pane_target"
    if [[ -z "$agent_bg_pane_target" ]]; then
        _red "  ✗ unable to find '${AGENT_BG_WINDOW_NAME}' window pane after creation."
        exit 1
    fi
    tmux send-keys -t "$agent_bg_pane_target" "bash" Enter

    echo ""
    echo "Launching AgentX in tmux window '${AGENTX_LOG_WINDOW_NAME}'..."
    local agentx_log_pane_target
    if ! _window_exists_by_name "$AGENTX_LOG_WINDOW_NAME"; then
        agentx_log_pane_target="$(tmux new-window -P -F '#{pane_id}' -t "$TMUX_SESSION" -n "$AGENTX_LOG_WINDOW_NAME" -d -c "$PROJECT_DIR")"
    else
        agentx_log_pane_target="$(_agentx_log_pane_target)"
    fi
    AGENTX_LOG_PANE_ID="$agentx_log_pane_target"

    local agentx_log_window_target
    agentx_log_window_target="$(tmux display-message -p -t "$agentx_log_pane_target" '#{session_name}:#{window_index}' 2>/dev/null || true)"
    if [[ -z "$agentx_log_window_target" ]]; then
        _red "  ✗ unable to find '${AGENTX_LOG_WINDOW_NAME}' window after creation."
        exit 1
    fi
    tmux send-keys -t "$agentx_log_window_target" \
        "AGENTX_NVIM_SOCKET='${NVIM_SOCKET}' AGENTX_SAVES_FIFO='${SAVES_FIFO}' AGENTX_TMUX_SESSION='${TMUX_SESSION}' AGENTX_TERMINAL_VISIBLE='${TERMINAL_VISIBLE}' '${python_bin}' -m agentx; __agentx_rc=\$?; tmux kill-session -t '${TMUX_SESSION}' >/dev/null 2>&1; exit \$__agentx_rc" \
        Enter
    _green "  AgentX launched in '${AGENTX_LOG_WINDOW_NAME}' window"

    # Launch neovim last — all windows and AgentX are stable before nvim starts,
    # so EDITOR_PANE_ID is fully valid and no window-rename race can occur.
    _launch_editor_in_pane_zero
    echo "  Waiting for neovim RPC socket..."
    _wait_for_socket
    if [[ ! -S "$NVIM_SOCKET" ]]; then
        _yellow "  ⚠ neovim socket not yet available — AgentX will retry connection automatically."
    else
        _green "  neovim socket ready"
    fi
    _ensure_editor_running || true

    local editor_idx
    local agent_bg_idx
    local agentx_log_idx
    editor_idx="$(_window_index_by_name "$EDITOR_WINDOW_NAME")"
    agent_bg_idx="$(_window_index_by_name "$AGENT_BG_WINDOW_NAME")"
    agentx_log_idx="$(_window_index_by_name "$AGENTX_LOG_WINDOW_NAME")"

    echo ""
    _bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    _bold "  Vibe Coding Session Ready"
    _bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "  Neovim     : window '${EDITOR_WINDOW_NAME}', pane 0${editor_idx:+  (Ctrl+B, ${editor_idx})}"
    echo "  AgentX GUI : floating window (alt-tab or move to monitor 1)"
    echo "  Agent bg   : window '${AGENT_BG_WINDOW_NAME}'${agent_bg_idx:+         (Ctrl+B, ${agent_bg_idx} — observe agent terminals)}"
    echo "  AgentX log : window '${AGENTX_LOG_WINDOW_NAME}'${agentx_log_idx:+        (Ctrl+B, ${agentx_log_idx} — runtime logs)}"
    echo ""
    echo "  Detach tmux   : Ctrl+B, D"
    echo "  Graceful stop : ./launch_vibe.sh stop"
    echo "  Recover editor: ./launch_vibe.sh recover-editor"
    echo ""

    exec tmux attach -t "$TMUX_SESSION"
}

_bold "AgentX Vibe Coding Launcher"
echo ""

case "$COMMAND" in
    start)
        PYTHON_BIN="$(_resolve_python || true)"
        if [[ -z "${PYTHON_BIN:-}" ]]; then
            _red "ERROR: No Python 3 executable found."
            _red "       Set AGENTX_PYTHON or activate a virtualenv before running."
            exit 1
        fi
        echo "Checking dependencies..."
        _check_start_dependencies "$PYTHON_BIN"
        _verify_nvim_version
        _start_session "$PYTHON_BIN"
        ;;
    stop)
        echo "Checking dependencies..."
        _check_minimal_dependencies false
        _stop_session
        ;;
    status)
        echo "Checking dependencies..."
        _check_minimal_dependencies false
        _show_status
        ;;
    recover-editor)
        echo "Checking dependencies..."
        _check_minimal_dependencies true
        _verify_nvim_version
        _recover_editor
        ;;
    restart)
        PYTHON_BIN="$(_resolve_python || true)"
        if [[ -z "${PYTHON_BIN:-}" ]]; then
            _red "ERROR: No Python 3 executable found."
            _red "       Set AGENTX_PYTHON or activate a virtualenv before running."
            exit 1
        fi
        echo "Checking dependencies..."
        _check_start_dependencies "$PYTHON_BIN"
        _verify_nvim_version
        _stop_session
        _start_session "$PYTHON_BIN"
        ;;
    *)
        _red "Unknown command: $COMMAND"
        _usage
        exit 1
        ;;
esac
