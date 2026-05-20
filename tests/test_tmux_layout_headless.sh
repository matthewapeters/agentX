#!/usr/bin/env bash
# test_tmux_layout_headless.sh
# Headless test for AgentX tmux layout.
# Fails if panes are missing, in the wrong geometry, or contain wrong placeholders.

set -euo pipefail
SESSION="agentx_test_$$"

cleanup() {
  tmux kill-session -t "$SESSION" >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Launch session and create layout (mimic Go logic)
tmux new-session -d -s "$SESSION" -n tui-chat -x 120 -y 40
# Set chat pane title
tmux select-pane -t "$SESSION:0.0" -T chat
# Split horizontally (input at bottom, 20%) and capture pane id
INPUT_PANE="$(tmux split-window -P -F '#{pane_id}' -t "$SESSION:0.0" -v -p 20)"
# Title bottom pane as input
tmux select-pane -t "$INPUT_PANE" -T input
# Focus top pane and split vertically (context right, 20%)
CHAT_PANE="$SESSION:0.0"
CONTEXT_PANE="$(tmux split-window -P -F '#{pane_id}' -t "$CHAT_PANE" -h -p 20)"
# Title right pane as context
tmux select-pane -t "$CONTEXT_PANE" -T context
# Set placeholder text
tmux send-keys -t "$CHAT_PANE" "echo '🔶 Pane: chat (AgentX Core)'" Enter
tmux send-keys -t "$INPUT_PANE" "echo '🔶 Pane: input (AgentX Core)'" Enter
tmux send-keys -t "$CONTEXT_PANE" "echo '🔶 Pane: context (AgentX Core)'" Enter
# Create logs window (hidden)
tmux new-window -t "$SESSION:1" -n logs
# Re-select primary window so attach defaults to main UX
tmux select-window -t "$SESSION:0"

echo "Window metadata:"
WINDOWS_META="$(tmux list-windows -t "$SESSION" -F '#{window_index}:#{window_name}:#{window_active}')"
echo "$WINDOWS_META"

if ! grep -q '^0:tui-chat:1$' <<< "$WINDOWS_META"; then
  echo "FAIL: Expected active primary window 0:tui-chat"
  exit 1
fi

if ! grep -q '^1:logs:0$' <<< "$WINDOWS_META"; then
  echo "FAIL: Expected hidden logs window 1:logs"
  exit 1
fi

echo "Pane metadata:"
PANE_META="$(tmux list-panes -t "$SESSION:0" -F '#{pane_index}|#{pane_title}|#{pane_left}|#{pane_top}|#{pane_width}|#{pane_height}')"
echo "$PANE_META"

if [[ "$(wc -l <<< "$PANE_META" | awk '{print $1}')" -ne 3 ]]; then
  echo "FAIL: Expected 3 panes in primary window"
  exit 1
fi

CHAT_RECORD="$(grep '|chat|' <<< "$PANE_META" || true)"
INPUT_RECORD="$(grep '|input|' <<< "$PANE_META" || true)"
CONTEXT_RECORD="$(grep '|context|' <<< "$PANE_META" || true)"

if [[ -z "$CHAT_RECORD" || -z "$INPUT_RECORD" || -z "$CONTEXT_RECORD" ]]; then
  echo "FAIL: Missing one or more titled panes (chat/input/context)"
  exit 1
fi

IFS='|' read -r CHAT_INDEX _ CHAT_LEFT CHAT_TOP _ _ <<< "$CHAT_RECORD"
IFS='|' read -r INPUT_INDEX _ INPUT_LEFT INPUT_TOP _ _ <<< "$INPUT_RECORD"
IFS='|' read -r CONTEXT_INDEX _ CONTEXT_LEFT CONTEXT_TOP _ _ <<< "$CONTEXT_RECORD"

if [[ "$CHAT_INDEX" -ne 0 ]]; then
  echo "FAIL: Chat pane is expected at index 0"
  exit 1
fi

if [[ "$INPUT_INDEX" -ne 2 ]]; then
  echo "FAIL: Input pane is expected at index 2"
  exit 1
fi

if [[ "$CONTEXT_INDEX" -ne 1 ]]; then
  echo "FAIL: Context pane is expected at index 1"
  exit 1
fi

# Validate geometry: chat is top-left, context is top-right, input is bottom full width.
if [[ "$CHAT_LEFT" -ne 0 || "$CHAT_TOP" -ne 0 ]]; then
  echo "FAIL: Chat pane is not at top-left"
  exit 1
fi

if [[ "$CONTEXT_TOP" -ne 0 || "$CONTEXT_LEFT" -le "$CHAT_LEFT" ]]; then
  echo "FAIL: Context pane is not in top-right position"
  exit 1
fi

if [[ "$INPUT_TOP" -le 0 || "$INPUT_LEFT" -ne 0 ]]; then
  echo "FAIL: Input pane is not at the bottom"
  exit 1
fi

# Validate placeholder text
CHAT_OUT="$(tmux capture-pane -t "$CHAT_PANE" -p | grep 'Pane:')"
INPUT_OUT="$(tmux capture-pane -t "$INPUT_PANE" -p | grep 'Pane:')"
CONTEXT_OUT="$(tmux capture-pane -t "$CONTEXT_PANE" -p | grep 'Pane:')"

[[ "$CHAT_OUT" == *"chat"* ]] || { echo "FAIL: Chat pane missing chat placeholder"; exit 1; }
[[ "$INPUT_OUT" == *"input"* ]] || { echo "FAIL: Input pane missing input placeholder"; exit 1; }
[[ "$CONTEXT_OUT" == *"context"* ]] || { echo "FAIL: Context pane missing context placeholder"; exit 1; }

echo "PASS: tmux layout matches UX spec."
