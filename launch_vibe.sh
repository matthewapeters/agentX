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
#   ./launch_vibe.sh [project_dir]
#
#   project_dir  Path to your project root (default: current directory)
#
# Environment overrides:
#   AGENTX_NVIM_SOCKET      Path for nvim RPC socket (default: /tmp/agentx.nvim.sock)
#   AGENTX_SAVES_FIFO       Path for save-notification pipe (default: /tmp/agentx_saves.fifo)
#   AGENTX_TMUX_SESSION     tmux session name (default: agentx)
#   AGENTX_TERMINAL_VISIBLE Default visibility for agent terminal panes (default: true)
#   AGENTX_PYTHON           Python executable to use (default: resolved from venv or PATH)
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

# ── Configuration (overridable via environment) ───────────────────────────────
NVIM_SOCKET="${AGENTX_NVIM_SOCKET:-/tmp/agentx.nvim.sock}"
SAVES_FIFO="${AGENTX_SAVES_FIFO:-/tmp/agentx_saves.fifo}"
TMUX_SESSION="${AGENTX_TMUX_SESSION:-agentx}"
TERMINAL_VISIBLE="${AGENTX_TERMINAL_VISIBLE:-true}"
PROJECT_DIR="${1:-$PWD}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ── Resolve Python executable ─────────────────────────────────────────────────
if [[ -n "${AGENTX_PYTHON:-}" ]]; then
    PYTHON_BIN="$AGENTX_PYTHON"
elif [[ -f "$SCRIPT_DIR/.venv/bin/python" ]]; then
    PYTHON_BIN="$SCRIPT_DIR/.venv/bin/python"
elif [[ -f "$PROJECT_DIR/.venv/bin/python" ]]; then
    PYTHON_BIN="$PROJECT_DIR/.venv/bin/python"
elif command -v python3 &>/dev/null; then
    PYTHON_BIN="python3"
else
    _red "ERROR: No Python 3 executable found."
    _red "       Set AGENTX_PYTHON or activate a virtualenv before running."
    exit 1
fi

# ── Step 1: Dependency checks ─────────────────────────────────────────────────
_bold "AgentX Vibe Coding Launcher"
echo ""
echo "Checking dependencies..."

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

dep_ok=true
_check_dep tmux  "sudo apt install tmux  /  brew install tmux" || dep_ok=false
_check_dep nvim  "sudo apt install neovim  /  brew install neovim" || dep_ok=false
_check_dep "$PYTHON_BIN" "uv sync  /  pip install -e ." || dep_ok=false

if [[ "$dep_ok" == "false" ]]; then
    echo ""
    _red "One or more required dependencies are missing. Aborting."
    exit 1
fi

# Verify neovim version supports --listen
nvim_version=$(nvim --version | head -1 | grep -oP '\d+\.\d+\.\d+' | head -1)
nvim_major=$(echo "$nvim_version" | cut -d. -f1)
nvim_minor=$(echo "$nvim_version" | cut -d. -f2)
if [[ "$nvim_major" -lt 1 && "$nvim_minor" -lt 5 ]]; then
    _yellow "  ⚠ neovim $nvim_version detected — version >= 0.5 required for --listen flag"
    _yellow "    Upgrade: https://github.com/neovim/neovim/releases"
    exit 1
fi

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
echo "Python      : $(_blue "$PYTHON_BIN")"
echo ""

# ── Step 2: Handle existing tmux session ─────────────────────────────────────
if tmux has-session -t "$TMUX_SESSION" 2>/dev/null; then
    _yellow "tmux session '$TMUX_SESSION' already exists."
    echo ""
    echo "Options:"
    echo "  [r] Reattach to existing session"
    echo "  [k] Kill existing session and start fresh"
    echo "  [a] Abort"
    echo ""
    read -r -p "Choice [r/k/a]: " choice
    case "$choice" in
        r|R)
            _green "Reattaching to existing session..."
            exec tmux attach -t "$TMUX_SESSION"
            ;;
        k|K)
            _yellow "Killing existing session '$TMUX_SESSION'..."
            tmux kill-session -t "$TMUX_SESSION"
            # Clean up stale socket
            [[ -S "$NVIM_SOCKET" ]] && rm -f "$NVIM_SOCKET"
            ;;
        *)
            echo "Aborted."
            exit 0
            ;;
    esac
fi

# ── Step 3: Create named pipe for save notifications ──────────────────────────
echo "Setting up IPC..."
if [[ ! -p "$SAVES_FIFO" ]]; then
    mkfifo "$SAVES_FIFO"
    _green "  Created FIFO: $SAVES_FIFO"
else
    _green "  FIFO exists: $SAVES_FIFO"
fi

# ── Step 4: Write .nvimrc.agentx into project root ────────────────────────────
NVIMRC_PATH="$PROJECT_DIR/.nvimrc.agentx"
cat > "$NVIMRC_PATH" << NVIMRC
" AgentX vibe-coding autocommands
" Auto-generated by launch_vibe.sh — modifications will be overwritten on next launch.
" This file is sourced alongside your personal neovim config.
augroup agentx_vibe
  autocmd!
  " Notify AgentX when any buffer is written to disk.
  autocmd BufWritePost * silent! call writefile([expand('%:p')], '${SAVES_FIFO}', 'a')
augroup END
NVIMRC
_green "  Wrote: $NVIMRC_PATH"

# Add to .gitignore if not already there
GITIGNORE="$PROJECT_DIR/.gitignore"
if [[ -f "$GITIGNORE" ]] && ! grep -qxF '.nvimrc.agentx' "$GITIGNORE"; then
    echo '.nvimrc.agentx' >> "$GITIGNORE"
    _green "  Added .nvimrc.agentx to .gitignore"
fi

# ── Step 5: Create tmux session ───────────────────────────────────────────────
echo ""
echo "Creating tmux session '$TMUX_SESSION'..."
tmux new-session -d -s "$TMUX_SESSION" -c "$PROJECT_DIR"

# Create a background window for hidden agent terminal panes (window 1)
tmux new-window -t "$TMUX_SESSION:1" -n "agent-bg" -d
_green "  Created tmux session with background window"

# ── Step 6: Launch neovim in pane 0.0 ────────────────────────────────────────
# Remove stale socket if it exists
[[ -S "$NVIM_SOCKET" ]] && rm -f "$NVIM_SOCKET"

tmux send-keys -t "${TMUX_SESSION}:0.0" \
    "nvim --listen '${NVIM_SOCKET}' --cmd 'source ${NVIMRC_PATH}'" \
    Enter
_green "  Launched neovim in pane 0.0 (socket: $NVIM_SOCKET)"

# Wait briefly for the socket to appear (nvim starts asynchronously)
echo "  Waiting for neovim RPC socket..."
waited=0
while [[ ! -S "$NVIM_SOCKET" && $waited -lt 10 ]]; do
    sleep 0.5
    waited=$((waited + 1))
done
if [[ ! -S "$NVIM_SOCKET" ]]; then
    _yellow "  ⚠ neovim socket not yet available — AgentX will retry connection automatically."
else
    _green "  neovim socket ready"
fi

# ── Step 7: Launch AgentX in a dedicated tmux window (window 2) ──────────────
# AgentX is launched inside tmux so its stdout/stderr go to window 2 (agentx-log)
# and never bleed into the neovim pane.
echo ""
echo "Launching AgentX in tmux window 2 (agentx-log)..."

tmux new-window -t "${TMUX_SESSION}:2" -n "agentx-log" -d -c "$PROJECT_DIR"

tmux send-keys -t "${TMUX_SESSION}:2" \
    "AGENTX_NVIM_SOCKET='${NVIM_SOCKET}' AGENTX_SAVES_FIFO='${SAVES_FIFO}' AGENTX_TMUX_SESSION='${TMUX_SESSION}' AGENTX_TERMINAL_VISIBLE='${TERMINAL_VISIBLE}' '${PYTHON_BIN}' -m agentx" \
    Enter
_green "  AgentX launched in window 2 (Ctrl+B, 2 to view logs)"

# ── Step 8: Attach tmux session ───────────────────────────────────────────────
echo ""
_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
_bold "  Vibe Coding Session Ready"
_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Neovim     : window 0, pane 0.0  (Ctrl+B, 0)"
echo "  AgentX GUI : floating window (alt-tab or move to monitor 1)"
echo "  Agent bg   : window 1           (Ctrl+B, 1 — observe agent terminals)"
echo "  AgentX log : window 2           (Ctrl+B, 2 — runtime logs)"
echo ""
echo "  Detach tmux: Ctrl+B, D"
echo "  Quit       : :qa in neovim, then Ctrl+B, D to detach"
echo ""

exec tmux attach -t "$TMUX_SESSION"
